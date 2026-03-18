use std::{
    fs,
    fs::OpenOptions,
    io::Write,
    path::PathBuf,
    sync::{Arc, Mutex},
};
use rfd::FileDialog;
use rusqlite::functions::FunctionFlags;

pub const DB_FILENAME: &str = "Rekenraam-data.rekenraam";

#[derive(Clone, Default)]
pub struct AuditUserHandle {
    user: Arc<Mutex<Option<String>>>,
}

impl AuditUserHandle {
    pub fn set(&self, user: Option<&str>) {
        let cleaned = user
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(|value| value.to_string());
        if let Ok(mut guard) = self.user.lock() {
            *guard = cleaned;
        }
    }
}

pub fn register_audit_user(conn: &rusqlite::Connection) -> Result<AuditUserHandle, rusqlite::Error> {
    let handle = AuditUserHandle::default();
    let user_ref = handle.user.clone();
    conn.create_scalar_function("audit_user", 0, FunctionFlags::SQLITE_UTF8, move |_ctx| {
        let value = user_ref.lock().ok().and_then(|guard| guard.clone());
        Ok(value)
    })?;
    Ok(handle)
}

pub fn set_audit_user(handle: &AuditUserHandle, user: Option<&str>) {
    handle.set(user);
}

pub fn os_login_user() -> Option<String> {
    let candidates = ["USERNAME", "USER", "LOGNAME"];
    for key in candidates.iter() {
        if let Ok(value) = std::env::var(key) {
            let trimmed = value.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
    }
    None
}

pub fn normalize_db_path(path: &PathBuf) -> PathBuf {
    if path.is_dir() {
        path.join(DB_FILENAME)
    } else {
        path.to_path_buf()
    }
}

pub fn storage_config_file() -> Option<PathBuf> {
    let mut path = dirs_next::data_local_dir()?;
    path.push("rekenraam");
    fs::create_dir_all(&path).ok()?;
    path.push("storage-path.txt");
    Some(path)
}

pub fn default_storage_dir() -> PathBuf {
    if cfg!(target_os = "windows") {
        if let Some(mut d) = dirs_next::document_dir() {
            d.push("rekenraam");
            return d;
        }
    }
    let mut home = dirs_next::home_dir().unwrap_or_else(|| PathBuf::from("."));
    home.push("rekenraam");
    home
}

pub fn load_storage_path() -> Option<PathBuf> {
    let cfg = storage_config_file()?;
    let content = fs::read_to_string(cfg).ok()?;
    let trimmed = content.trim();
    if trimmed.is_empty() {
        return None;
    }
    // Return the configured path even if it doesn't exist yet.
    // We can create the DB file on-demand; if the location is invalid, `db_accessible`
    // will fail and the app will prompt the user to pick a new place.
    Some(PathBuf::from(trimmed))
}

pub fn save_storage_path(path: &PathBuf) {
    if let Some(cfg) = storage_config_file() {
        if let Some(parent) = cfg.parent() {
            let _ = fs::create_dir_all(parent);
        }
        if let Ok(mut f) = fs::File::create(cfg) {
            let _ = f.write_all(path.to_string_lossy().as_bytes());
        }
    }
}

pub fn create_db_placeholder(path: &PathBuf) {
    let db = normalize_db_path(path);
    if let Some(parent) = db.parent() {
        let _ = fs::create_dir_all(parent);
    }
    if !db.exists() {
        let _ = fs::File::create(db);
    }
}

pub fn db_accessible(path: &PathBuf) -> bool {
    let db = normalize_db_path(path);
    if db.exists() {
        OpenOptions::new().read(true).write(true).open(&db).is_ok()
    } else {
        if let Some(parent) = db.parent() {
            if fs::create_dir_all(parent).is_err() {
                return false;
            }
        }
        OpenOptions::new().create(true).write(true).open(&db).is_ok()
    }
}

#[allow(dead_code)]
pub fn resolve_accessible_db(mut path: PathBuf) -> PathBuf {
    if db_accessible(&path) {
        return path;
    }

    // Allow selecting either an existing DB file or a folder.
    if let Some(picked) = FileDialog::new()
        .set_title("Locate a Rekenraam database file")
        .add_filter("Rekenraam database", &["rekenraam"])
        .pick_file()
    {
        if db_accessible(&picked) {
            return picked;
        }
        path = picked;
    } else if let Some(picked) = FileDialog::new()
        .set_title("Locate a storage folder")
        .pick_folder()
    {
        if db_accessible(&picked) {
            return picked;
        }
        path = picked;
    }

    let default = default_storage_dir();
    let chosen = if fs::create_dir_all(&default).is_ok() {
        default
    } else {
        path
    };
    create_db_placeholder(&chosen);
    chosen
}

const MIGRATIONS: &[(i32, &str)] = &[
    (1, include_str!("../migrations/V1__init.sql")),
];

pub fn migration_versions() -> Vec<i32> {
    MIGRATIONS.iter().map(|(version, _)| *version).collect()
}

pub fn latest_migration_version() -> i32 {
    MIGRATIONS
        .iter()
        .map(|(version, _)| *version)
        .max()
        .unwrap_or(0)
}

fn run_migrations(conn: &mut rusqlite::Connection) -> Result<(), rusqlite::Error> {
    conn.execute_batch("PRAGMA foreign_keys = ON;")?;
    // ensure migrations table
    conn.execute_batch("CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);")?;

    let applied: Vec<i32> = {
        let mut stmt = conn.prepare("SELECT version FROM schema_migrations ORDER BY version")?;
        let rows = stmt.query_map([], |row| row.get::<_, i32>(0))?;
        let mut v = Vec::new();
        for r in rows {
            v.push(r?);
        }
        v
    };

    for (version, sql) in MIGRATIONS.iter() {
        if applied.contains(version) {
            continue;
        }

        let tx = conn.transaction()?;
        tx.execute_batch(sql)?;
        tx.execute("INSERT INTO schema_migrations (version) VALUES (?1)", [version])?;
        tx.commit()?;
    }

    Ok(())
}

pub fn open_and_migrate(
    path: &PathBuf,
) -> Result<(rusqlite::Connection, PathBuf, AuditUserHandle), rusqlite::Error> {
    let db = normalize_db_path(path);
    let mut conn = rusqlite::Connection::open(&db)?;
    // Enable recommended pragmas for durability and foreign key support.
    // `journal_mode = WAL` improves concurrency; `synchronous = NORMAL` is a reasonable default.
    conn.execute_batch(
        "PRAGMA journal_mode = WAL;
         PRAGMA synchronous = NORMAL;
         PRAGMA foreign_keys = ON;
         -- Memory-mapped I/O for faster sequential reads (256 MB)
         PRAGMA mmap_size = 268435456;
         -- 64 MB page cache (default is ~2 MB)
         PRAGMA cache_size = -64000;
         -- Avoid immediate SQLITE_BUSY errors when a writer holds the lock
         PRAGMA busy_timeout = 5000;
         -- Keep temp tables and indexes in memory rather than on disk
         PRAGMA temp_store = MEMORY;",
    )?;
    let audit_user = register_audit_user(&conn)?;
    if let Some(user) = os_login_user() {
        set_audit_user(&audit_user, Some(&user));
    }
    run_migrations(&mut conn)?;
        conn.execute_batch(
                "CREATE TABLE IF NOT EXISTS app_runtime_session (
                        id TEXT PRIMARY KEY,
                        started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
                    );
                    CREATE TABLE IF NOT EXISTS session_undo_stack (
                        seq INTEGER PRIMARY KEY AUTOINCREMENT,
                        session_id TEXT NOT NULL,
                        table_name TEXT NOT NULL,
                        row_id INTEGER NOT NULL,
                        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
                    );
                    CREATE TABLE IF NOT EXISTS session_redo_stack (
                        seq INTEGER PRIMARY KEY AUTOINCREMENT,
                        session_id TEXT NOT NULL,
                        table_name TEXT NOT NULL,
                        row_id INTEGER NOT NULL,
                        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
                    );
                    CREATE TABLE IF NOT EXISTS session_reverts (
                        session_id TEXT NOT NULL,
                        table_name TEXT NOT NULL,
                        row_id INTEGER NOT NULL,
                        reverted_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
                        PRIMARY KEY(session_id, table_name, row_id)
                    );
                    DELETE FROM app_runtime_session;
                    INSERT INTO app_runtime_session (id, started_at)
                    VALUES (lower(hex(randomblob(16))), strftime('%Y-%m-%dT%H:%M:%fZ','now'));
                    DELETE FROM session_undo_stack;
                    DELETE FROM session_redo_stack;
                    DELETE FROM session_reverts;",
        )?;
    Ok((conn, db, audit_user))
}

#[cfg(test)]
mod tests {
    use super::*;
    use rusqlite::params;
    use std::fs;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn create_temp_dir() -> PathBuf {
        let start = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis();
        let mut temp = std::env::temp_dir();
        temp.push(format!("rekenraam_db_test_{start}"));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");
        temp
    }

    #[test]
    fn test_report_cache_invalidation_by_book_state() {
        let temp = create_temp_dir();
        let (conn, _db_path, _audit_user) = open_and_migrate(&temp).expect("open and migrate");

        let book_id: i64 = conn
            .query_row("SELECT id FROM books WHERE name='Personal' LIMIT 1", [], |row| row.get(0))
            .expect("book id");

        let initial_seq: i64 = conn
            .query_row(
                "SELECT change_seq FROM book_state WHERE book_id = ?1",
                [book_id],
                |row| row.get(0),
            )
            .expect("initial change_seq");

        conn.execute(
            "INSERT INTO report_cache (book_id, report_type, params_hash, params_json, as_of_seq, payload_json)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![book_id, "test_report", "hash1", "{}", initial_seq, "{\"ok\":true}"],
        )
        .expect("insert report_cache");

        let cached_seq: i64 = conn
            .query_row(
                "SELECT as_of_seq FROM report_cache WHERE book_id = ?1 AND report_type = 'test_report'",
                [book_id],
                |row| row.get(0),
            )
            .expect("cache as_of_seq");
        assert_eq!(cached_seq, initial_seq);

        let category_name = format!("Test Category {initial_seq}");
        conn.execute(
            "INSERT INTO categories (book_id, parent_id, name, kind, created_at)
             VALUES (?1, NULL, ?2, 'expense', strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![book_id, category_name],
        )
        .expect("insert category");

        let updated_seq: i64 = conn
            .query_row(
                "SELECT change_seq FROM book_state WHERE book_id = ?1",
                [book_id],
                |row| row.get(0),
            )
            .expect("updated change_seq");

        assert!(updated_seq > initial_seq, "change_seq should increment");
        assert!(cached_seq < updated_seq, "cached report should be stale");

        let _ = fs::remove_file(normalize_db_path(&temp));
        let _ = fs::remove_dir_all(&temp);
    }
}

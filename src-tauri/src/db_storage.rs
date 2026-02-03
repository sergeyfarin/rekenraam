use std::{fs, path::PathBuf, time::{SystemTime, UNIX_EPOCH}};

use serde::{Deserialize, Serialize};
use rusqlite::{params, OptionalExtension};
use tauri::{async_runtime, command, AppHandle, Manager, State};
use rfd::FileDialog;
use tokio::time::{sleep, Duration};

use crate::db::{
    db_accessible, latest_migration_version, load_storage_path, migration_versions,
    normalize_db_path, open_and_migrate, save_storage_path,
};
use crate::fx_refresh;
use crate::state::{BackupSchedulerState, DbState, FxSchedulerState};

const SINGLE_BOOK_ID: i64 = 1;

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct BackupSettings {
    pub enabled: bool,
    pub interval_minutes: u64,
    pub retention_count: u64,
    pub backup_path: Option<String>,
    pub backup_on_close: bool,
}

impl Default for BackupSettings {
    fn default() -> Self {
        Self {
            enabled: false,
            interval_minutes: 60,
            retention_count: 60,
            backup_path: None,
            backup_on_close: true,
        }
    }
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct BackupInfo {
    pub path: String,
    pub size_bytes: u64,
    pub modified_unix: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct DbStats {
    pub path: String,
    pub size_bytes: u64,
    pub modified_unix: Option<i64>,
    pub writable: bool,
    pub journal_mode: Option<String>,
    pub foreign_keys: bool,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct MigrationStatus {
    pub current_version: i32,
    pub latest_version: i32,
    pub pending_versions: Vec<i32>,
}

fn backup_settings_file() -> Option<PathBuf> {
    let mut path = dirs_next::data_local_dir()?;
    path.push("rekenraam");
    fs::create_dir_all(&path).ok()?;
    path.push("backup-settings.json");
    Some(path)
}

fn load_backup_settings_file() -> Option<BackupSettings> {
    let path = backup_settings_file()?;
    let content = fs::read_to_string(path).ok()?;
    serde_json::from_str(&content).ok()
}

fn save_backup_settings_file(settings: &BackupSettings) -> Result<(), String> {
    let path = backup_settings_file().ok_or_else(|| "backup config path not available".to_string())?;
    let serialized = serde_json::to_string_pretty(settings).map_err(|e| e.to_string())?;
    fs::write(path, serialized).map_err(|e| e.to_string())
}

fn load_backup_settings_db(conn: &rusqlite::Connection) -> Result<Option<BackupSettings>, String> {
    let row = conn
        .query_row(
            "SELECT enabled, interval_minutes, retention_count, backup_path, backup_on_close
             FROM backup_settings WHERE book_id = ?1",
            [SINGLE_BOOK_ID],
            |r| {
                let enabled: i64 = r.get(0)?;
                let interval_minutes: i64 = r.get(1)?;
                let retention_count: i64 = r.get(2)?;
                let backup_on_close: i64 = r.get(4)?;
                Ok(BackupSettings {
                    enabled: enabled != 0,
                    interval_minutes: interval_minutes.max(0) as u64,
                    retention_count: retention_count.max(0) as u64,
                    backup_path: r.get(3)?,
                    backup_on_close: backup_on_close != 0,
                })
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;
    Ok(row)
}

fn save_backup_settings_db(
    conn: &rusqlite::Connection,
    settings: &BackupSettings,
) -> Result<(), String> {
    conn.execute(
        "INSERT INTO backup_settings
            (book_id, enabled, interval_minutes, retention_count, backup_path, backup_on_close, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
         ON CONFLICT(book_id) DO UPDATE SET
            enabled = excluded.enabled,
            interval_minutes = excluded.interval_minutes,
            retention_count = excluded.retention_count,
            backup_path = excluded.backup_path,
            backup_on_close = excluded.backup_on_close,
            updated_at = excluded.updated_at",
        params![
            SINGLE_BOOK_ID,
            settings.enabled as i64,
            settings.interval_minutes as i64,
            settings.retention_count as i64,
            settings.backup_path,
            settings.backup_on_close as i64,
        ],
    )
    .map_err(|e| e.to_string())?;
    Ok(())
}

pub fn load_backup_settings(db: Option<&DbState>) -> BackupSettings {
    if let Some(db_state) = db {
        if let Ok(guard) = db_state.inner.lock() {
            if let Some(conn) = guard.conn.as_ref() {
                if let Ok(Some(settings)) = load_backup_settings_db(conn) {
                    return settings;
                }

                if let Some(file_settings) = load_backup_settings_file() {
                    let _ = save_backup_settings_db(conn, &file_settings);
                    let _ = backup_settings_file().and_then(|p| fs::remove_file(p).ok());
                    return file_settings;
                }
            }
        }
    }

    load_backup_settings_file().unwrap_or_default()
}

fn save_backup_settings(db: &DbState, settings: &BackupSettings) -> Result<(), String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    save_backup_settings_db(conn, settings)?;
    let _ = save_backup_settings_file(settings);
    Ok(())
}

fn default_backup_dir(db_path: &PathBuf) -> PathBuf {
    db_path
        .parent()
        .map(|p| p.join("backups"))
        .unwrap_or_else(|| db_path.join("backups"))
}

fn resolve_backup_dir(settings: &BackupSettings, db_path: &PathBuf) -> PathBuf {
    if let Some(path) = settings.backup_path.as_ref() {
        PathBuf::from(path)
    } else {
        default_backup_dir(db_path)
    }
}

fn backup_filename() -> String {
    let ts = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis())
        .unwrap_or(0);
    format!("rekenraam-backup-{ts}.rekenraam")
}

fn resolve_backup_target(path: Option<PathBuf>, settings: &BackupSettings, db_path: &PathBuf) -> PathBuf {
    if let Some(p) = path {
        if p.is_dir() || p.extension().is_none() {
            p.join(backup_filename())
        } else {
            p
        }
    } else {
        let dir = resolve_backup_dir(settings, db_path);
        dir.join(backup_filename())
    }
}

fn prune_backups(backup_dir: &PathBuf, retention_count: u64) {
    if retention_count == 0 {
        return;
    }

    let mut entries: Vec<(PathBuf, SystemTime)> = match fs::read_dir(backup_dir) {
        Ok(read_dir) => read_dir
            .filter_map(|entry| entry.ok())
            .filter_map(|entry| {
                let path = entry.path();
                if path.is_file() {
                    let modified = entry.metadata().and_then(|m| m.modified()).unwrap_or(SystemTime::UNIX_EPOCH);
                    Some((path, modified))
                } else {
                    None
                }
            })
            .collect(),
        Err(_) => return,
    };

    entries.sort_by_key(|(_, modified)| *modified);

    if entries.len() <= retention_count as usize {
        return;
    }

    let remove_count = entries.len().saturating_sub(retention_count as usize);
    for (path, _) in entries.into_iter().take(remove_count) {
        let _ = fs::remove_file(path);
    }
}

fn checkpoint_and_close(db: &State<DbState>) -> Result<PathBuf, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let db_path = guard
        .db_path
        .clone()
        .ok_or_else(|| "db not initialized".to_string())?;

    if let Some(conn) = guard.conn.as_ref() {
        let _ = conn.execute("PRAGMA wal_checkpoint(TRUNCATE)", []);
    }

    guard.conn = None;
    Ok(db_path)
}

fn open_and_set_storage(
    db: &State<DbState>,
    storage_path: &PathBuf,
    open_path: &PathBuf,
) -> Result<String, String> {
    let (conn, effective_db_path) =
        open_and_migrate(open_path).map_err(|e| format!("Migration failed: {e}"))?;

    save_storage_path(storage_path);

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    guard.db_path = Some(effective_db_path.clone());
    guard.conn = Some(conn);

    Ok(normalize_db_path(&effective_db_path)
        .to_string_lossy()
        .to_string())
}

#[command]
pub fn validate_and_set_storage_location(
    app: AppHandle,
    scheduler: State<BackupSchedulerState>,
    fx_scheduler: State<FxSchedulerState>,
    db: State<DbState>,
    path: String,
) -> Result<String, String> {
    let input = PathBuf::from(path);

    if !db_accessible(&input) {
        return Err("Storage location is not accessible for read/write.".to_string());
    }

    let (conn, effective_db_path) =
        open_and_migrate(&input).map_err(|e| format!("Migration failed: {e}"))?;

    save_storage_path(&input);

    {
        let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
        guard.db_path = Some(effective_db_path.clone());
        guard.conn = Some(conn);
    }

    let settings = load_backup_settings(Some(&db));
    restart_backup_scheduler(app.clone(), scheduler, settings);
    fx_refresh::restart_fx_scheduler(app, fx_scheduler);

    Ok(normalize_db_path(&effective_db_path)
        .to_string_lossy()
        .to_string())
}

#[command]
pub fn get_storage_location() -> Option<String> {
    load_storage_path().map(|p| normalize_db_path(&p).to_string_lossy().to_string())
}

#[command]
pub fn pick_storage_file() -> Option<String> {
    FileDialog::new()
        .add_filter("Rekenraam database", &["rekenraam"])
        .pick_file()
        .map(|p| p.to_string_lossy().to_string())
}

#[command]
pub fn pick_storage_folder() -> Option<String> {
    FileDialog::new()
        .pick_folder()
        .map(|p| p.to_string_lossy().to_string())
}

#[command]
pub fn pick_backup_folder() -> Option<String> {
    FileDialog::new()
        .pick_folder()
        .map(|p| p.to_string_lossy().to_string())
}

#[command]
pub fn pick_backup_file() -> Option<String> {
    FileDialog::new()
        .add_filter("Rekenraam backup", &["rekenraam", "bak", "sqlite", "db"])
        .pick_file()
        .map(|p| p.to_string_lossy().to_string())
}

#[command]
pub fn get_backup_settings(db: State<DbState>) -> BackupSettings {
    load_backup_settings(Some(&db))
}

#[command]
pub fn set_backup_settings(
    app: AppHandle,
    scheduler: State<BackupSchedulerState>,
    db: State<DbState>,
    settings: BackupSettings,
) -> Result<BackupSettings, String> {
    save_backup_settings(&db, &settings)?;
    restart_backup_scheduler(app, scheduler, settings.clone());
    Ok(settings)
}

fn create_backup_internal(
    db: &DbState,
    settings: &BackupSettings,
    dest_path: Option<PathBuf>,
) -> Result<String, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let db_path = guard
        .db_path
        .clone()
        .ok_or_else(|| "db not initialized".to_string())?;

    let target = resolve_backup_target(dest_path, settings, &db_path);
    let target_str = target.to_string_lossy().to_string();
    if let Some(parent) = target.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }

    let mut vacuumed = false;
    if let Some(conn) = guard.conn.as_ref() {
        let _ = conn.execute("PRAGMA wal_checkpoint(TRUNCATE)", []);
        if conn.execute("VACUUM INTO ?1", params![target_str]).is_ok() {
            vacuumed = true;
        }
    }

    drop(guard);

    if !vacuumed {
        fs::copy(&db_path, &target).map_err(|e| e.to_string())?;
    }

    let backup_dir = target.parent().map(|p| p.to_path_buf()).unwrap_or_else(|| db_path.clone());
    prune_backups(&backup_dir, settings.retention_count);

    Ok(target.to_string_lossy().to_string())
}

#[command]
pub fn create_backup(db: State<DbState>, dest_path: Option<String>) -> Result<String, String> {
    let settings = load_backup_settings(Some(&db));
    let dest = dest_path.map(PathBuf::from);
    create_backup_internal(&db, &settings, dest)
}

#[command]
pub fn list_backups(db: State<DbState>, path: Option<String>) -> Result<Vec<BackupInfo>, String> {
    let settings = load_backup_settings(Some(&db));
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let db_path = guard
        .db_path
        .clone()
        .ok_or_else(|| "db not initialized".to_string())?;
    drop(guard);

    let backup_dir = match path {
        Some(p) => PathBuf::from(p),
        None => resolve_backup_dir(&settings, &db_path),
    };

    let read_dir = match fs::read_dir(&backup_dir) {
        Ok(read_dir) => read_dir,
        Err(_) => return Ok(Vec::new()),
    };
    let mut items = Vec::new();
    for entry in read_dir.flatten() {
        let path = entry.path();
        if !path.is_file() {
            continue;
        }
        let metadata = entry.metadata().ok();
        let size = metadata.as_ref().map(|m| m.len()).unwrap_or(0);
        let modified = metadata
            .and_then(|m| m.modified().ok())
            .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
            .map(|d| d.as_secs() as i64);
        items.push(BackupInfo {
            path: path.to_string_lossy().to_string(),
            size_bytes: size,
            modified_unix: modified,
        });
    }

    items.sort_by(|a, b| b.modified_unix.cmp(&a.modified_unix));
    Ok(items)
}

pub fn restart_backup_scheduler(
    app: AppHandle,
    scheduler: State<BackupSchedulerState>,
    settings: BackupSettings,
) {
    if let Ok(mut guard) = scheduler.task.lock() {
        if let Some(task) = guard.take() {
            task.abort();
        }

        if !settings.enabled {
            return;
        }

        let interval_minutes = settings.interval_minutes.max(1);
        let handle = async_runtime::spawn(async move {
            loop {
                sleep(Duration::from_secs(interval_minutes * 60)).await;
                let db_state = app.state::<DbState>();
                let _ = create_backup_internal(&db_state, &settings, None);
            }
        });

        *guard = Some(handle);
    }
}

pub fn backup_on_close(app: &AppHandle) {
    let db_state = app.state::<DbState>();
    let settings = load_backup_settings(Some(&db_state));
    if !settings.backup_on_close {
        return;
    }

    let db_state = app.state::<DbState>();
    let _ = create_backup_internal(&db_state, &settings, None);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::{open_and_migrate, normalize_db_path};
    use crate::state::DbStateInner;
    use rusqlite::params;
    use std::sync::Mutex;
    use std::time::Duration;
    use tauri::async_runtime;
    use tokio::time::sleep;

    fn create_temp_dir(name: &str) -> PathBuf {
        let mut temp = std::env::temp_dir();
        let ts = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_millis())
            .unwrap_or(0);
        temp.push(format!("rekenraam_test_{name}_{ts}"));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");
        temp
    }

    fn insert_account(conn: &rusqlite::Connection, name: &str) {
        let book_id: i64 = conn
            .query_row("SELECT id FROM books WHERE name='Personal' LIMIT 1", [], |row| row.get(0))
            .expect("book id");
        let commodity_id: i64 = conn
            .query_row("SELECT id FROM commodities WHERE book_id = ?1 AND symbol='USD' LIMIT 1", [book_id], |row| row.get(0))
            .expect("commodity id");
        conn.execute(
            "INSERT INTO accounts (book_id, type, name, commodity_id) VALUES (?1, 'cash', ?2, ?3)",
            params![book_id, name, commodity_id],
        )
        .expect("insert account");
    }

    fn account_exists(db_path: &PathBuf, name: &str) -> bool {
        let (conn, _path) = open_and_migrate(db_path).expect("open db");
        let count: i64 = conn
            .query_row("SELECT COUNT(*) FROM accounts WHERE name = ?1", [name], |row| row.get(0))
            .unwrap_or(0);
        count > 0
    }

    #[test]
    fn test_backup_creation_and_retention() {
        let temp = create_temp_dir("backup_retention");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");
        insert_account(&conn, "BackupRetentionAccount");

        let db_state = DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path.clone()),
                conn: Some(conn),
            }),
        };

        let backups_dir = temp.join("backups");
        let settings = BackupSettings {
            enabled: true,
            interval_minutes: 60,
            retention_count: 2,
            backup_path: Some(backups_dir.to_string_lossy().to_string()),
            backup_on_close: true,
        };

        let _ = create_backup_internal(&db_state, &settings, None).expect("backup 1");
        let _ = create_backup_internal(&db_state, &settings, None).expect("backup 2");
        let _ = create_backup_internal(&db_state, &settings, None).expect("backup 3");

        let entries: Vec<_> = fs::read_dir(&backups_dir)
            .expect("read backups dir")
            .filter_map(|e| e.ok())
            .filter(|e| e.path().is_file())
            .collect();

        assert!(entries.len() <= 2, "retention should keep only two backups");
        assert!(!entries.is_empty(), "expected at least one backup");
        let sample = entries.first().expect("backup entry").path();
        assert!(account_exists(&sample, "BackupRetentionAccount"));

        let _ = fs::remove_file(normalize_db_path(&temp));
        let _ = fs::remove_dir_all(&temp);
    }

    #[test]
    fn test_restore_from_backup_file() {
        let temp = create_temp_dir("restore");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");
        insert_account(&conn, "AccountBeforeBackup");

        let db_state = DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path.clone()),
                conn: Some(conn),
            }),
        };

        let settings = BackupSettings::default();
        let backup_path = create_backup_internal(&db_state, &settings, None).expect("backup");

        {
            let mut guard = db_state.inner.lock().expect("lock db");
            if let Some(conn) = guard.conn.as_ref() {
                insert_account(conn, "AccountAfterBackup");
            }
            guard.conn = None;
        }

        let target = normalize_db_path(&temp);
        let _ = fs::remove_file(&target);
        fs::copy(&backup_path, &target).expect("restore copy");

        assert!(account_exists(&target, "AccountBeforeBackup"));
        assert!(!account_exists(&target, "AccountAfterBackup"));

        let _ = fs::remove_file(&target);
        let _ = fs::remove_dir_all(&temp);
    }

    #[test]
    fn test_copy_preserves_data() {
        let temp = create_temp_dir("copy_preserves");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");
        insert_account(&conn, "CopyPreserveAccount");

        let source = normalize_db_path(&db_path);
        let _ = conn.execute("PRAGMA wal_checkpoint(TRUNCATE)", []);
        drop(conn);
        let dest_dir = create_temp_dir("copy_dest");
        let dest = normalize_db_path(&dest_dir);

        fs::copy(&source, &dest).expect("copy db");
        assert!(account_exists(&dest, "CopyPreserveAccount"));

        let _ = fs::remove_file(&source);
        let _ = fs::remove_dir_all(&temp);
        let _ = fs::remove_file(&dest);
        let _ = fs::remove_dir_all(&dest_dir);
    }

    #[test]
    fn test_backup_scheduler_timing() {
        let temp = create_temp_dir("backup_scheduler");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");

        let db_state = DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path.clone()),
                conn: Some(conn),
            }),
        };

        let backups_dir = temp.join("backups");
        let settings = BackupSettings {
            enabled: true,
            interval_minutes: 1,
            retention_count: 10,
            backup_path: Some(backups_dir.to_string_lossy().to_string()),
            backup_on_close: true,
        };

        async_runtime::block_on(async {
            for _ in 0..2 {
                let _ = create_backup_internal(&db_state, &settings, None).expect("backup");
                sleep(Duration::from_millis(20)).await;
            }
        });

        let entries: Vec<_> = fs::read_dir(&backups_dir)
            .expect("read backups dir")
            .filter_map(|e| e.ok())
            .filter(|e| e.path().is_file())
            .collect();

        assert!(entries.len() >= 2, "expected multiple backups");

        let _ = fs::remove_file(normalize_db_path(&temp));
        let _ = fs::remove_dir_all(&temp);
    }

    #[test]
    fn test_integrity_check_negative_case() {
        let temp = create_temp_dir("integrity_negative");
        let (_conn, _db_path) = open_and_migrate(&temp).expect("open and migrate");
        drop(_conn);

        let db_file = normalize_db_path(&temp);
        let mut bytes = fs::read(&db_file).expect("read db file");
        if bytes.len() >= 16 {
            for i in 0..16 {
                bytes[i] = 0;
            }
        } else if !bytes.is_empty() {
            let idx = bytes.len() / 2;
            bytes[idx] = bytes[idx].wrapping_add(1);
        }
        fs::write(&db_file, bytes).expect("write corrupted db");

        match rusqlite::Connection::open(&db_file) {
            Ok(conn) => {
                match conn.query_row("PRAGMA integrity_check", [], |row| row.get::<_, String>(0)) {
                    Ok(result) => {
                        assert_ne!(result.to_lowercase(), "ok", "expected integrity check to fail");
                    }
                    Err(_) => {
                        // integrity_check itself failed due to corruption.
                    }
                }
            }
            Err(_) => {
                // Opening failed due to corruption; treat as expected failure.
            }
        }

        let _ = fs::remove_file(&db_file);
        let _ = fs::remove_dir_all(&temp);
    }

    #[test]
    fn test_backup_settings_db_roundtrip() {
        let temp = create_temp_dir("backup_settings_db");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");

        let settings = BackupSettings {
            enabled: true,
            interval_minutes: 30,
            retention_count: 7,
            backup_path: Some("C:/backups".to_string()),
            backup_on_close: false,
        };

        save_backup_settings_db(&conn, &settings).expect("save settings db");
        let loaded = load_backup_settings_db(&conn)
            .expect("load settings db")
            .expect("settings row");

        assert_eq!(loaded.enabled, settings.enabled);
        assert_eq!(loaded.interval_minutes, settings.interval_minutes);
        assert_eq!(loaded.retention_count, settings.retention_count);
        assert_eq!(loaded.backup_path, settings.backup_path);
        assert_eq!(loaded.backup_on_close, settings.backup_on_close);

        drop(conn);
        let _ = fs::remove_file(normalize_db_path(&db_path));
        let _ = fs::remove_dir_all(&temp);
    }

    #[test]
    fn test_load_backup_settings_from_db_state() {
        let temp = create_temp_dir("backup_settings_state");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");

        let settings = BackupSettings {
            enabled: true,
            interval_minutes: 15,
            retention_count: 3,
            backup_path: Some(temp.join("custom").to_string_lossy().to_string()),
            backup_on_close: true,
        };

        save_backup_settings_db(&conn, &settings).expect("save settings db");

        let db_state = DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path.clone()),
                conn: Some(conn),
            }),
        };

        let loaded = load_backup_settings(Some(&db_state));
        assert_eq!(loaded.enabled, settings.enabled);
        assert_eq!(loaded.interval_minutes, settings.interval_minutes);
        assert_eq!(loaded.retention_count, settings.retention_count);
        assert_eq!(loaded.backup_path, settings.backup_path);
        assert_eq!(loaded.backup_on_close, settings.backup_on_close);

        let _ = fs::remove_file(normalize_db_path(&temp));
        let _ = fs::remove_dir_all(&temp);
    }
}

#[command]
pub fn create_new_storage(
    app: AppHandle,
    scheduler: State<BackupSchedulerState>,
    fx_scheduler: State<FxSchedulerState>,
    db: State<DbState>,
    path: String,
) -> Result<String, String> {
    let input = PathBuf::from(path);
    let db_path = normalize_db_path(&input);

    if db_path.exists() {
        return Err("Database file already exists.".to_string());
    }

    if let Some(parent) = db_path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }

    if !db_accessible(&db_path) {
        return Err("Storage location is not accessible for read/write.".to_string());
    }

    let (conn, effective_db_path) =
        open_and_migrate(&db_path).map_err(|e| format!("Migration failed: {e}"))?;

    save_storage_path(&db_path);

    {
        let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
        guard.db_path = Some(effective_db_path.clone());
        guard.conn = Some(conn);
    }

    let settings = load_backup_settings(Some(&db));
    restart_backup_scheduler(app.clone(), scheduler, settings);
    fx_refresh::restart_fx_scheduler(app, fx_scheduler);

    Ok(normalize_db_path(&effective_db_path)
        .to_string_lossy()
        .to_string())
}

#[command]
pub fn move_storage(db: State<DbState>, path: String) -> Result<String, String> {
    let input = PathBuf::from(path);
    let dest_db_path = normalize_db_path(&input);

    if dest_db_path.exists() {
        return Err("Destination already exists.".to_string());
    }

    if let Some(parent) = dest_db_path.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }

    let src_db_path = checkpoint_and_close(&db)?;

    if src_db_path == dest_db_path {
        let _ = open_and_set_storage(&db, &input, &input);
        return Err("Destination matches current database.".to_string());
    }

    if fs::rename(&src_db_path, &dest_db_path).is_err() {
        fs::copy(&src_db_path, &dest_db_path).map_err(|e| e.to_string())?;
        fs::remove_file(&src_db_path).map_err(|e| e.to_string())?;
    }

    open_and_set_storage(&db, &input, &input)
}

#[command]
pub fn copy_storage(db: State<DbState>, path: String) -> Result<String, String> {
    let input = PathBuf::from(path);
    let dest_db_path = normalize_db_path(&input);

    if dest_db_path.exists() {
        return Err("Destination already exists.".to_string());
    }

    if let Some(parent) = dest_db_path.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }

    let src_db_path = checkpoint_and_close(&db)?;

    if src_db_path == dest_db_path {
        let _ = open_and_set_storage(&db, &input, &input);
        return Err("Destination matches current database.".to_string());
    }

    if let Err(e) = fs::copy(&src_db_path, &dest_db_path) {
        let _ = open_and_set_storage(&db, &src_db_path, &src_db_path);
        return Err(format!("Copy failed: {e}"));
    }

    open_and_set_storage(&db, &input, &input)
}

#[command]
pub fn restore_from_backup(db: State<DbState>, backup_path: String) -> Result<String, String> {
    let backup = PathBuf::from(backup_path);
    if !backup.exists() || backup.is_dir() {
        return Err("Backup file not found.".to_string());
    }

    let config_path = load_storage_path();
    let open_path = match config_path.as_ref() {
        Some(path) => path.clone(),
        None => {
            let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
            guard
                .db_path
                .clone()
                .ok_or_else(|| "db not initialized".to_string())?
        }
    };

    let target_db_path = normalize_db_path(&open_path);

    if target_db_path == backup {
        return Err("Backup matches current database.".to_string());
    }

    let _ = checkpoint_and_close(&db)?;

    if target_db_path.exists() {
        fs::remove_file(&target_db_path).map_err(|e| e.to_string())?;
    }

    if let Err(e) = fs::copy(&backup, &target_db_path) {
        let _ = open_and_set_storage(&db, &open_path, &open_path);
        return Err(format!("Restore failed: {e}"));
    }

    open_and_set_storage(&db, &open_path, &open_path)
}

#[command]
pub fn get_db_path(db: State<DbState>) -> Option<String> {
    let guard = db.inner.lock().ok()?;
    guard
        .db_path
        .as_ref()
        .map(|p| p.to_string_lossy().to_string())
}

#[command]
pub fn db_health(db: State<DbState>) -> Result<String, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    conn.query_row("SELECT 1", [], |_row| Ok(())).map_err(|e| e.to_string())?;
    Ok("ok".to_string())
}

#[command]
pub fn get_schema_version(db: State<DbState>) -> Result<i32, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let version_opt: Option<i32> = conn
        .query_row(
            "SELECT MAX(version) FROM schema_migrations",
            [],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    Ok(version_opt.unwrap_or(0))
}

#[command]
pub fn get_migration_status(db: State<DbState>) -> Result<MigrationStatus, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare("SELECT version FROM schema_migrations")
        .map_err(|e| e.to_string())?;
    let applied_iter = stmt
        .query_map([], |row| row.get::<_, i32>(0))
        .map_err(|e| e.to_string())?;

    let mut applied = Vec::new();
    for version in applied_iter {
        if let Ok(v) = version {
            applied.push(v);
        }
    }

    let current_version = applied.iter().copied().max().unwrap_or(0);
    let latest_version = latest_migration_version();
    let mut pending_versions: Vec<i32> = migration_versions()
        .into_iter()
        .filter(|v| !applied.contains(v))
        .collect();
    pending_versions.sort();

    Ok(MigrationStatus {
        current_version,
        latest_version,
        pending_versions,
    })
}

#[command]
pub fn db_integrity_check(db: State<DbState>) -> Result<String, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let result: String = conn
        .query_row("PRAGMA integrity_check", [], |row| row.get(0))
        .map_err(|e| e.to_string())?;
    Ok(result)
}

#[command]
pub fn db_vacuum(db: State<DbState>) -> Result<String, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    conn.execute_batch("VACUUM").map_err(|e| e.to_string())?;
    Ok("ok".to_string())
}

#[command]
pub fn db_stats(db: State<DbState>) -> Result<DbStats, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let db_path = guard
        .db_path
        .clone()
        .ok_or_else(|| "db not initialized".to_string())?;

    let metadata = fs::metadata(&db_path).map_err(|e| e.to_string())?;
    let size_bytes = metadata.len();
    let modified_unix = metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
        .map(|d| d.as_secs() as i64);

    let writable = !metadata.permissions().readonly();
    let journal_mode: Option<String> = conn
        .query_row("PRAGMA journal_mode", [], |row| row.get(0))
        .ok();
    let foreign_keys: i64 = conn
        .query_row("PRAGMA foreign_keys", [], |row| row.get(0))
        .unwrap_or(0);

    Ok(DbStats {
        path: db_path.to_string_lossy().to_string(),
        size_bytes,
        modified_unix,
        writable,
        journal_mode,
        foreign_keys: foreign_keys != 0,
    })
}

#[command]
pub fn greet(name: &str) -> String {
    format!("Hello, {}! You've been greeted from Rust!", name)
}

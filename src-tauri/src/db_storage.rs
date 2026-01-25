use std::path::PathBuf;

use tauri::{command, State};

use crate::db::{
    db_accessible, load_storage_path, normalize_db_path, open_and_migrate, save_storage_path,
};
use crate::state::DbState;

#[command]
pub fn validate_and_set_storage_location(path: String) -> Result<String, String> {
    let input = PathBuf::from(path);

    if !db_accessible(&input) {
        return Err("Storage location is not accessible for read/write.".to_string());
    }

    let (_conn, effective_db_path) =
        open_and_migrate(&input).map_err(|e| format!("Migration failed: {e}"))?;

    save_storage_path(&input);

    Ok(normalize_db_path(&effective_db_path)
        .to_string_lossy()
        .to_string())
}

#[command]
pub fn get_storage_location() -> Option<String> {
    load_storage_path().map(|p| normalize_db_path(&p).to_string_lossy().to_string())
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
pub fn greet(name: &str) -> String {
    format!("Hello, {}! You've been greeted from Rust!", name)
}

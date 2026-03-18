use std::{path::PathBuf, sync::Mutex};
use tauri::async_runtime::JoinHandle;

use crate::db::AuditUserHandle;

/// Write connection state — holds the single writable connection + metadata.
/// Guarded by its own Mutex so writes serialize.
#[derive(Default)]
pub struct DbStateInner {
    pub db_path: Option<PathBuf>,
    pub conn: Option<rusqlite::Connection>,
    pub audit_user: Option<AuditUserHandle>,
}

/// Database state managed by Tauri.
///
/// Two separate mutexes enable concurrent reads and writes (SQLite WAL mode):
/// - `inner` — write connection; all mutating commands use this.
/// - `read_conn` — read-only connection (`PRAGMA query_only = ON`);
///   all `list_*` / `get_*` commands should prefer this so they don't
///   block writers.
///
/// Both connections must be opened/closed together in DB lifecycle commands.
#[derive(Default)]
pub struct DbState {
    pub inner: Mutex<DbStateInner>,
    pub read_conn: Mutex<Option<rusqlite::Connection>>,
}

#[derive(Default)]
pub struct BackupSchedulerState {
    pub task: Mutex<Option<JoinHandle<()>>>,
}

#[derive(Default)]
pub struct FxSchedulerState {
    pub task: Mutex<Option<JoinHandle<()>>>,
}

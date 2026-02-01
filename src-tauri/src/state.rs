use std::{path::PathBuf, sync::Mutex};
use tauri::async_runtime::JoinHandle;

#[derive(Default)]
pub struct DbStateInner {
    pub db_path: Option<PathBuf>,
    pub conn: Option<rusqlite::Connection>,
}

#[derive(Default)]
pub struct DbState {
    pub inner: Mutex<DbStateInner>,
}

#[derive(Default)]
pub struct BackupSchedulerState {
    pub task: Mutex<Option<JoinHandle<()>>>,
}

#[derive(Default)]
pub struct FxSchedulerState {
    pub task: Mutex<Option<JoinHandle<()>>>,
}
use std::{path::PathBuf, sync::Mutex};

#[derive(Default)]
pub struct DbStateInner {
    pub db_path: Option<PathBuf>,
    pub conn: Option<rusqlite::Connection>,
}

#[derive(Default)]
pub struct DbState {
    pub inner: Mutex<DbStateInner>,
}
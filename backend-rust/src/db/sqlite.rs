use std::{path::Path, str::FromStr};

use sqlx::{
    sqlite::{SqliteConnectOptions, SqliteJournalMode, SqlitePoolOptions},
    SqlitePool,
};

static MIGRATOR: sqlx::migrate::Migrator = sqlx::migrate!("./migrations");

pub async fn connect(database_url: &str) -> Result<SqlitePool, sqlx::Error> {
    ensure_parent_dir(database_url);

    let options = SqliteConnectOptions::from_str(database_url)?
        .create_if_missing(true)
        .journal_mode(SqliteJournalMode::Wal);

    SqlitePoolOptions::new()
        .max_connections(5)
        .connect_with(options)
        .await
}

pub async fn migrate(pool: &SqlitePool) -> Result<(), sqlx::migrate::MigrateError> {
    MIGRATOR.run(pool).await
}

fn ensure_parent_dir(database_url: &str) {
    let Some(path) = sqlite_path(database_url) else {
        return;
    };
    let Some(parent) = Path::new(&path).parent() else {
        return;
    };

    if !parent.as_os_str().is_empty() {
        std::fs::create_dir_all(parent).expect("failed to create SQLite parent directory");
    }
}

fn sqlite_path(database_url: &str) -> Option<String> {
    database_url
        .strip_prefix("sqlite://")
        .or_else(|| database_url.strip_prefix("sqlite:"))
        .filter(|path| !path.is_empty() && *path != ":memory:")
        .map(ToOwned::to_owned)
}

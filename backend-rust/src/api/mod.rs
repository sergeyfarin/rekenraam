mod hello;

use axum::{routing::get, Router};
use sqlx::SqlitePool;

#[derive(Clone)]
pub struct AppState {
    pub db: SqlitePool,
}

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/healthz", get(hello::healthz))
        .route("/api/hello", get(hello::hello))
        .with_state(state)
}

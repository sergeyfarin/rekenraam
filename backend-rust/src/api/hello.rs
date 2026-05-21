use axum::{extract::State, http::StatusCode, response::IntoResponse, Json};
use serde::Serialize;

use crate::{api::AppState, app};

pub async fn healthz() -> StatusCode {
    StatusCode::NO_CONTENT
}

pub async fn hello(State(state): State<AppState>) -> Result<Json<HelloResponse>, ApiError> {
    let message = app::hello::message(&state.db).await?;
    Ok(Json(HelloResponse { message }))
}

#[derive(Serialize)]
pub struct HelloResponse {
    message: String,
}

pub struct ApiError(sqlx::Error);

impl From<sqlx::Error> for ApiError {
    fn from(err: sqlx::Error) -> Self {
        Self(err)
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> axum::response::Response {
        eprintln!("request failed: {}", self.0);
        StatusCode::INTERNAL_SERVER_ERROR.into_response()
    }
}

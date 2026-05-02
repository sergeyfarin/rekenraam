use std::{env, net::SocketAddr};

use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::{IntoResponse, Response},
    routing::get,
    Json, Router,
};
use serde::Serialize;
use sqlx::{postgres::PgPoolOptions, PgPool};
use tower_http::{cors::CorsLayer, trace::TraceLayer};
use tracing::{error, info};

mod books;

static MIGRATOR: sqlx::migrate::Migrator = sqlx::migrate!();

#[derive(Clone)]
struct AppState {
    pool: PgPool,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    service: &'static str,
    database: &'static str,
    schema_version: String,
}

#[derive(Serialize)]
struct ErrorResponse {
    status: &'static str,
    message: String,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    dotenvy::dotenv().ok();
    init_tracing();

    let host = env::var("APP_HOST").unwrap_or_else(|_| "0.0.0.0".to_string());
    let port = env::var("APP_PORT")
        .ok()
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(8080);
    let database_url = env::var("DATABASE_URL")
        .map_err(|_| "DATABASE_URL must be set for apps/api")?;

    let pool = PgPoolOptions::new()
        .max_connections(10)
        .connect(&database_url)
        .await?;

    MIGRATOR.run(&pool).await?;

    let schema_version = load_schema_version(&pool).await?;

    let state = AppState { pool };
    let app = Router::new()
        .route("/health", get(health))
        .route("/api/v1/books", get(list_books))
        .route("/api/v1/books/{slug}", get(get_book_by_slug))
        .layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http())
        .with_state(state);

    let addr: SocketAddr = format!("{host}:{port}").parse()?;
    let listener = tokio::net::TcpListener::bind(addr).await?;

    info!(address = %addr, schema_version = %schema_version, "rekenraam api listening");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    Ok(())
}

fn init_tracing() {
    let env_filter = tracing_subscriber::EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| "info,tower_http=info".into());

    tracing_subscriber::fmt()
        .with_env_filter(env_filter)
        .with_target(false)
        .compact()
        .init();
}

async fn health(State(state): State<AppState>) -> Result<Json<HealthResponse>, AppError> {
    let schema_version = load_schema_version(&state.pool)
        .await
        .map_err(AppError::database)?;

    Ok(Json(HealthResponse {
        status: "ok",
        service: "rekenraam-api",
        database: "ok",
        schema_version,
    }))
}

async fn list_books(State(state): State<AppState>) -> Result<Json<Vec<BookSummary>>, AppError> {
    let books = books::list_books(&state.pool)
    .await
    .map_err(AppError::database)?;

    Ok(Json(books))
}

async fn get_book_by_slug(
    State(state): State<AppState>,
    Path(slug): Path<String>,
) -> Result<Json<BookSummary>, AppError> {
    let book = books::get_book_by_slug(&state.pool, &slug)
        .await
        .map_err(AppError::database)?
        .ok_or_else(|| AppError::not_found(format!("book not found for slug '{slug}'")))?;

    Ok(Json(book))
}

async fn load_schema_version(pool: &PgPool) -> Result<String, sqlx::Error> {
    sqlx::query_scalar::<_, String>(
        "SELECT schema_version FROM app_schema_state WHERE id = TRUE",
    )
    .fetch_one(pool)
    .await
}

async fn shutdown_signal() {
    let ctrl_c = async {
        if let Err(err) = tokio::signal::ctrl_c().await {
            error!(error = %err, "failed to listen for ctrl-c");
        }
    };

    #[cfg(unix)]
    let terminate = async {
        match tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate()) {
            Ok(mut signal) => {
                signal.recv().await;
            }
            Err(err) => {
                error!(error = %err, "failed to listen for terminate signal");
            }
        }
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}

struct AppError {
    status: StatusCode,
    message: String,
}

impl AppError {
    fn not_found(message: String) -> Self {
        Self {
            status: StatusCode::NOT_FOUND,
            message,
        }
    }

    fn database(error: sqlx::Error) -> Self {
        Self {
            status: StatusCode::SERVICE_UNAVAILABLE,
            message: format!("database unavailable: {error}"),
        }
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        (
            self.status,
            Json(ErrorResponse {
                status: "error",
                message: self.message,
            }),
        )
            .into_response()
    }
}

use books::BookSummary;
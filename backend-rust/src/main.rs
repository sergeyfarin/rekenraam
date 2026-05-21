mod api;
mod app;
mod config;
mod db;
mod web;

use tokio::net::TcpListener;

#[tokio::main]
async fn main() {
    let cfg = config::Config::load();
    let pool = db::sqlite::connect(&cfg.database_url)
        .await
        .expect("failed to connect to SQLite");

    db::sqlite::migrate(&pool)
        .await
        .expect("failed to run SQLite migrations");

    let listener = TcpListener::bind(cfg.http_addr)
        .await
        .expect("failed to bind HTTP listener");
    let addr = listener.local_addr().expect("failed to read listener addr");

    let app = api::router(api::AppState { db: pool }).fallback_service(web::handler());

    println!("rekenraam backend-rust listening on {addr}");
    axum::serve(listener, app)
        .await
        .expect("HTTP server failed");
}

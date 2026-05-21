use tower_http::services::ServeDir;

pub fn handler() -> ServeDir {
    ServeDir::new("dist")
}

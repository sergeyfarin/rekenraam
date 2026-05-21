use std::{env, net::SocketAddr};

pub struct Config {
    pub http_addr: SocketAddr,
    pub database_url: String,
}

impl Config {
    pub fn load() -> Self {
        Self {
            http_addr: http_addr(),
            database_url: env_or("DATABASE_URL", "sqlite://var/dev.sqlite"),
        }
    }
}

fn http_addr() -> SocketAddr {
    let raw = env_or("HTTP_ADDR", ":18888");
    let normalized = if raw.starts_with(':') {
        format!("0.0.0.0{raw}")
    } else {
        raw
    };

    normalized
        .parse()
        .expect("HTTP_ADDR must be an address like :18888 or 127.0.0.1:18888")
}

fn env_or(key: &str, fallback: &str) -> String {
    env::var(key).unwrap_or_else(|_| fallback.to_string())
}

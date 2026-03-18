use serde::{Deserialize, Serialize};

/// Structured application error returned by all Tauri commands.
///
/// Serializes as a tagged JSON object so the frontend can distinguish error types
/// and show contextual messages:
///
/// ```json
/// {"type":"Validation","data":{"field":"name","message":"Name is required"}}
/// {"type":"NotFound","data":{"entity":"Account","id":"42"}}
/// {"type":"Conflict","data":{"message":"Account has transactions"}}
/// {"type":"Database","data":{"message":"..."}}
/// {"type":"Internal","data":{"message":"..."}}
/// ```
#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(tag = "type", content = "data")]
pub enum AppError {
    Validation { field: String, message: String },
    NotFound { entity: String, id: String },
    Conflict { message: String },
    Database { message: String },
    Internal { message: String },
}

impl AppError {
    pub fn validation(field: impl Into<String>, message: impl Into<String>) -> Self {
        AppError::Validation { field: field.into(), message: message.into() }
    }

    pub fn not_found(entity: impl Into<String>, id: impl ToString) -> Self {
        AppError::NotFound { entity: entity.into(), id: id.to_string() }
    }

    pub fn conflict(message: impl Into<String>) -> Self {
        AppError::Conflict { message: message.into() }
    }

    pub fn db(message: impl Into<String>) -> Self {
        AppError::Database { message: message.into() }
    }

    pub fn internal(message: impl Into<String>) -> Self {
        AppError::Internal { message: message.into() }
    }
}

impl std::fmt::Display for AppError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AppError::Validation { field, message } => {
                write!(f, "Validation error on {field}: {message}")
            }
            AppError::NotFound { entity, id } => write!(f, "{entity} not found: {id}"),
            AppError::Conflict { message } => write!(f, "Conflict: {message}"),
            AppError::Database { message } => write!(f, "Database error: {message}"),
            AppError::Internal { message } => write!(f, "Internal error: {message}"),
        }
    }
}

impl std::error::Error for AppError {}

impl From<rusqlite::Error> for AppError {
    fn from(e: rusqlite::Error) -> Self {
        AppError::Database { message: e.to_string() }
    }
}

/// Allows `?` to propagate plain String errors (from legacy internal helpers)
/// as `AppError::Internal`.
impl From<String> for AppError {
    fn from(s: String) -> Self {
        AppError::Internal { message: s }
    }
}

/// Convenience: convert `&str` errors.
impl From<&str> for AppError {
    fn from(s: &str) -> Self {
        AppError::Internal { message: s.to_string() }
    }
}

impl From<serde_json::Error> for AppError {
    fn from(e: serde_json::Error) -> Self {
        AppError::Internal { message: e.to_string() }
    }
}

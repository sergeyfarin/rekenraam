use serde::{Deserialize, Serialize};

/// Generic paginated result returned by unbounded list commands.
///
/// `list_transactions` uses infinite scroll with its own `has_more` field.
/// All other list commands (accounts, payees, categories, tags, etc.) that
/// were previously returning raw `Vec<T>` should be migrated to this type
/// so the frontend can detect when a list was truncated.
#[derive(Debug, Serialize, Deserialize)]
pub struct PaginatedResult<T> {
    pub items: Vec<T>,
    /// Total count of matching rows (before limit/offset).
    pub total_count: i64,
    /// True when there are more rows beyond the current page.
    pub has_more: bool,
}

impl<T> PaginatedResult<T> {
    pub fn new(items: Vec<T>, total_count: i64, limit: i64, offset: i64) -> Self {
        let has_more = (offset + items.len() as i64) < total_count;
        let _ = limit; // kept for caller clarity
        PaginatedResult { items, total_count, has_more }
    }

    /// Convenience: wrap a full unbounded result (no pagination applied).
    pub fn all(items: Vec<T>) -> Self {
        let total_count = items.len() as i64;
        PaginatedResult { items, total_count, has_more: false }
    }
}

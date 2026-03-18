/// Session helpers shared across db_accounts, db_transactions, and db_commodities.
///
/// These operate on the SQLite undo/redo stack and are called within write transactions.
/// Return types are `Result<_, String>` for compatibility with legacy internal helpers;
/// callers that return `AppError` convert via `?` using `impl From<String> for AppError`.

use rusqlite::params;

pub fn current_session_id(conn: &rusqlite::Connection) -> Result<String, String> {
    conn.query_row("SELECT id FROM app_runtime_session LIMIT 1", [], |row| row.get(0))
        .map_err(|e| e.to_string())
}

pub fn clear_redo_stack(conn: &rusqlite::Connection, session_id: &str) -> Result<(), String> {
    conn.execute("DELETE FROM session_redo_stack WHERE session_id = ?1", [session_id])
        .map_err(|e| e.to_string())?;
    Ok(())
}

pub fn record_insert_change(
    conn: &rusqlite::Connection,
    session_id: &str,
    table_name: &str,
    row_id: i64,
) -> Result<(), String> {
    conn.execute(
        "INSERT INTO session_undo_stack (session_id, table_name, row_id) VALUES (?1, ?2, ?3)",
        params![session_id, table_name, row_id],
    )
    .map_err(|e| e.to_string())?;
    conn.execute(
        "DELETE FROM session_reverts WHERE session_id = ?1 AND table_name = ?2 AND row_id = ?3",
        params![session_id, table_name, row_id],
    )
    .map_err(|e| e.to_string())?;
    Ok(())
}

use std::collections::HashSet;

use chrono::{DateTime, NaiveDate, Utc};
use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::state::DbState;

const SINGLE_BOOK_ID: i64 = 1;

#[derive(Serialize, Deserialize, Debug)]
pub struct Transaction {
    pub id: i64,
    pub book_id: i64,
    pub previous_tx_id: Option<i64>,
    pub txn_date: String,
    pub happened_at_utc: String,
    pub posted_at_utc: Option<String>,
    pub payee_id: Option<i64>,
    pub memo: Option<String>,
    pub status: String,
    pub reference: Option<String>,
    pub import_id: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Split {
    pub id: i64,
    pub tx_id: i64,
    pub account_id: i64,
    pub commodity_id: i64,
    pub amount_minor: i64,
    pub category_id: Option<i64>,
    pub tag_id: Option<i64>,
    pub person_id: Option<i64>,
    pub project_id: Option<i64>,
    pub share_bps: Option<i64>,
    pub memo: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct TransactionWithSplits {
    pub transaction: Transaction,
    pub splits: Vec<Split>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CreateSplit {
    pub account_id: i64,
    pub commodity_id: i64,
    pub amount_minor: i64,
    pub category_id: Option<i64>,
    pub tag_id: Option<i64>,
    pub person_id: Option<i64>,
    pub project_id: Option<i64>,
    pub share_bps: Option<i64>,
    pub memo: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CreateTransactionInput {
    pub book_id: i64,
    pub txn_date: String,
    pub happened_at_utc: Option<String>,
    pub posted_at_utc: Option<String>,
    pub payee_id: Option<i64>,
    pub memo: Option<String>,
    pub status: Option<String>,
    pub reference: Option<String>,
    pub import_id: Option<String>,
    pub splits: Vec<CreateSplit>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct UpdateTransactionInput {
    pub id: i64,
    pub book_id: i64,
    pub txn_date: String,
    pub happened_at_utc: Option<String>,
    pub posted_at_utc: Option<String>,
    pub payee_id: Option<i64>,
    pub memo: Option<String>,
    pub status: Option<String>,
    pub reference: Option<String>,
    pub import_id: Option<String>,
    pub splits: Vec<CreateSplit>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ListTransactionsFilter {
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub payee_id: Option<i64>,
    pub date_from: Option<String>,
    pub date_to: Option<String>,
    pub search: Option<String>,
    pub limit: Option<i64>,
    pub offset: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct RegisterEntry {
    pub tx_id: i64,
    pub txn_date: String,
    pub payee_id: Option<i64>,
    pub payee_name: Option<String>,
    pub memo: Option<String>,
    pub status: String,
    pub split_id: i64,
    pub account_id: i64,
    pub account_name: String,
    pub amount_minor: i64,
    pub commodity_id: i64,
    pub category_id: Option<i64>,
    pub category_name: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct RegisterEntryWithBalance {
    pub tx_id: i64,
    pub txn_date: String,
    pub payee_id: Option<i64>,
    pub payee_name: Option<String>,
    pub memo: Option<String>,
    pub status: String,
    pub split_id: i64,
    pub account_id: i64,
    pub account_name: String,
    pub amount_minor: i64,
    pub commodity_id: i64,
    pub category_id: Option<i64>,
    pub category_name: Option<String>,
    pub running_balance_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PostingEntry {
    pub tx_id: i64,
    pub txn_date: String,
    pub payee_id: Option<i64>,
    pub payee_name: Option<String>,
    pub memo: Option<String>,
    pub status: String,
    pub split_id: i64,
    pub account_id: i64,
    pub amount_minor: i64,
    pub commodity_id: i64,
    pub category_id: Option<i64>,
    pub category_name: Option<String>,
}

fn map_transaction_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Transaction> {
    Ok(Transaction {
        id: row.get(0)?,
        book_id: row.get(1)?,
        previous_tx_id: row.get(2)?,
        txn_date: row.get(3)?,
        happened_at_utc: row.get(4)?,
        posted_at_utc: row.get(5)?,
        payee_id: row.get(6)?,
        memo: row.get(7)?,
        status: row.get(8)?,
        reference: row.get(9)?,
        import_id: row.get(10)?,
        created_at: row.get(11)?,
        updated_at: row.get(12)?,
    })
}

fn map_split_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Split> {
    Ok(Split {
        id: row.get(0)?,
        tx_id: row.get(1)?,
        account_id: row.get(2)?,
        commodity_id: row.get(3)?,
        amount_minor: row.get(4)?,
        category_id: row.get(5)?,
        tag_id: row.get(6)?,
        person_id: row.get(7)?,
        project_id: row.get(8)?,
        share_bps: row.get(9)?,
        memo: row.get(10)?,
        created_at: row.get(11)?,
        updated_at: row.get(12)?,
    })
}

fn map_register_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<RegisterEntry> {
    Ok(RegisterEntry {
        tx_id: row.get(0)?,
        txn_date: row.get(1)?,
        payee_id: row.get(2)?,
        payee_name: row.get(3)?,
        memo: row.get(4)?,
        status: row.get(5)?,
        split_id: row.get(6)?,
        account_id: row.get(7)?,
        account_name: row.get(8)?,
        amount_minor: row.get(9)?,
        commodity_id: row.get(10)?,
        category_id: row.get(11)?,
        category_name: row.get(12)?,
    })
}

fn map_register_balance_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<RegisterEntryWithBalance> {
    Ok(RegisterEntryWithBalance {
        tx_id: row.get(0)?,
        txn_date: row.get(1)?,
        payee_id: row.get(2)?,
        payee_name: row.get(3)?,
        memo: row.get(4)?,
        status: row.get(5)?,
        split_id: row.get(6)?,
        account_id: row.get(7)?,
        account_name: row.get(8)?,
        amount_minor: row.get(9)?,
        commodity_id: row.get(10)?,
        category_id: row.get(11)?,
        category_name: row.get(12)?,
        running_balance_minor: row.get(13)?,
    })
}

fn map_posting_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<PostingEntry> {
    Ok(PostingEntry {
        tx_id: row.get(0)?,
        txn_date: row.get(1)?,
        payee_id: row.get(2)?,
        payee_name: row.get(3)?,
        memo: row.get(4)?,
        status: row.get(5)?,
        split_id: row.get(6)?,
        account_id: row.get(7)?,
        amount_minor: row.get(8)?,
        commodity_id: row.get(9)?,
        category_id: row.get(10)?,
        category_name: row.get(11)?,
    })
}

fn ensure_txn_date_format(txn_date: &str) -> Result<(), String> {
    NaiveDate::parse_from_str(txn_date.trim(), "%Y-%m-%d")
        .map(|_| ())
        .map_err(|_| "txn_date must be in YYYY-MM-DD format".to_string())
}

fn normalize_date_or_utc_timestamp(value: &str, field_name: &str) -> Result<String, String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return Err(format!("{field_name} cannot be empty"));
    }

    if NaiveDate::parse_from_str(trimmed, "%Y-%m-%d").is_ok() {
        return Ok(trimmed.to_string());
    }

    let parsed = DateTime::parse_from_rfc3339(trimmed)
        .map_err(|_| format!("{field_name} must be YYYY-MM-DD or a valid ISO-8601 UTC timestamp"))?;
    Ok(parsed
        .with_timezone(&Utc)
        .format("%Y-%m-%dT%H:%M:%S%.3fZ")
        .to_string())
}

fn normalize_optional_date_or_utc_timestamp(
    value: Option<&str>,
    field_name: &str,
) -> Result<Option<String>, String> {
    if let Some(raw) = value {
        let trimmed = raw.trim();
        if trimmed.is_empty() {
            return Ok(None);
        }
        return normalize_date_or_utc_timestamp(trimmed, field_name).map(Some);
    }
    Ok(None)
}

fn effective_happened_at_utc(txn_date: &str, happened_at_utc: Option<&str>) -> Result<String, String> {
    ensure_txn_date_format(txn_date)?;
    if let Some(value) = happened_at_utc {
        let trimmed = value.trim();
        if !trimmed.is_empty() {
            return normalize_date_or_utc_timestamp(trimmed, "happened_at_utc");
        }
    }
    Ok(txn_date.to_string())
}

fn current_session_id(conn: &rusqlite::Connection) -> Result<String, String> {
    conn.query_row("SELECT id FROM app_runtime_session LIMIT 1", [], |row| row.get(0))
        .map_err(|e| e.to_string())
}

fn clear_redo_stack(conn: &rusqlite::Connection, session_id: &str) -> Result<(), String> {
    conn.execute("DELETE FROM session_redo_stack WHERE session_id = ?1", [session_id])
        .map_err(|e| e.to_string())?;
    Ok(())
}

fn record_insert_change(
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

#[command]
pub fn list_account_register(
    db: State<DbState>,
    account_id: i64,
    limit: Option<i64>,
    offset: Option<i64>,
) -> Result<Vec<RegisterEntry>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = limit.unwrap_or(200).max(1);
    let offset = offset.unwrap_or(0).max(0);

    let mut stmt = conn
        .prepare(
            "SELECT
                t.id,
                t.txn_date,
                t.payee_id,
                p.name,
                t.memo,
                t.status,
                s.id,
                s.account_id,
                a.name,
                s.amount_minor,
                s.commodity_id,
                s.category_id,
                c.name
             FROM splits s
             JOIN transactions t ON t.id = s.tx_id
             JOIN accounts a ON a.id = s.account_id
             LEFT JOIN payees p ON p.id = t.payee_id
             LEFT JOIN categories c ON c.id = s.category_id
             WHERE s.account_id = ?1
                             AND t.status != 'void'
                             AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'transactions'
                                     AND sr.row_id = t.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
             ORDER BY t.txn_date DESC, t.id DESC, s.id ASC
             LIMIT ?2 OFFSET ?3",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![account_id, limit, offset], map_register_row)
        .map_err(|e| e.to_string())?;

    let mut entries = Vec::new();
    for row in rows {
        entries.push(row.map_err(|e| e.to_string())?);
    }

    Ok(entries)
}

#[command]
pub fn list_account_register_with_balance(
    db: State<DbState>,
    account_id: i64,
    limit: Option<i64>,
    offset: Option<i64>,
) -> Result<Vec<RegisterEntryWithBalance>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = limit.unwrap_or(200).max(1);
    let offset = offset.unwrap_or(0).max(0);

    let mut stmt = conn
        .prepare(
            "WITH base AS (
                SELECT
                    t.id AS tx_id,
                    t.txn_date,
                    t.payee_id,
                    p.name AS payee_name,
                    t.memo,
                    t.status,
                    s.id AS split_id,
                    s.account_id,
                    a.name AS account_name,
                    s.amount_minor,
                    s.commodity_id,
                    s.category_id,
                    c.name AS category_name,
                    SUM(s.amount_minor) OVER (
                        ORDER BY t.txn_date ASC, t.id ASC, s.id ASC
                    ) AS running_balance_minor
                FROM splits s
                JOIN transactions t ON t.id = s.tx_id
                JOIN accounts a ON a.id = s.account_id
                LEFT JOIN payees p ON p.id = t.payee_id
                LEFT JOIN categories c ON c.id = s.category_id
                WHERE s.account_id = ?1
                                    AND t.status != 'void'
                                    AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                                    AND NOT EXISTS (
                                        SELECT 1 FROM session_reverts sr
                                        WHERE sr.table_name = 'transactions'
                                            AND sr.row_id = t.id
                                            AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                                    )
            )
            SELECT
                tx_id,
                txn_date,
                payee_id,
                payee_name,
                memo,
                status,
                split_id,
                account_id,
                account_name,
                amount_minor,
                commodity_id,
                category_id,
                category_name,
                running_balance_minor
            FROM base
            ORDER BY txn_date DESC, tx_id DESC, split_id ASC
            LIMIT ?2 OFFSET ?3",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![account_id, limit, offset], map_register_balance_row)
        .map_err(|e| e.to_string())?;

    let mut entries = Vec::new();
    for row in rows {
        entries.push(row.map_err(|e| e.to_string())?);
    }

    Ok(entries)
}

#[command]
pub fn list_postings(
    db: State<DbState>,
    account_id: i64,
    date_from: Option<String>,
    date_to: Option<String>,
    limit: Option<i64>,
    offset: Option<i64>,
) -> Result<Vec<PostingEntry>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = limit.unwrap_or(200).max(1);
    let offset = offset.unwrap_or(0).max(0);

    let mut sql = String::from(
        "SELECT
            t.id,
            t.txn_date,
            t.payee_id,
            p.name,
            t.memo,
            t.status,
            s.id,
            s.account_id,
            s.amount_minor,
            s.commodity_id,
            s.category_id,
            c.name
         FROM splits s
         JOIN transactions t ON t.id = s.tx_id
         LEFT JOIN payees p ON p.id = t.payee_id
         LEFT JOIN categories c ON c.id = s.category_id
                 WHERE s.account_id = ?1
                     AND t.status != 'void'
                     AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                     AND NOT EXISTS (
                         SELECT 1 FROM session_reverts sr
                         WHERE sr.table_name = 'transactions'
                             AND sr.row_id = t.id
                             AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                     )",
    );
    let mut params: Vec<Value> = vec![Value::from(account_id)];

    if let Some(date_from) = date_from {
        sql.push_str(" AND t.txn_date >= ?");
        params.push(Value::from(date_from));
    }
    if let Some(date_to) = date_to {
        sql.push_str(" AND t.txn_date <= ?");
        params.push(Value::from(date_to));
    }

    sql.push_str(" ORDER BY t.txn_date DESC, t.id DESC, s.id ASC LIMIT ? OFFSET ?");
    params.push(Value::from(limit));
    params.push(Value::from(offset));

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), map_posting_row)
        .map_err(|e| e.to_string())?;

    let mut entries = Vec::new();
    for row in rows {
        entries.push(row.map_err(|e| e.to_string())?);
    }

    Ok(entries)
}

fn fetch_transaction_with_splits(
    conn: &rusqlite::Connection,
    id: i64,
) -> Result<TransactionWithSplits, String> {
    let transaction = conn
        .query_row(
                        "SELECT id, book_id, previous_tx_id, txn_date, happened_at_utc, posted_at_utc,
                                        payee_id, memo, status, reference, import_id, created_at, updated_at
                         FROM transactions
                         WHERE id = ?1
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'transactions'
                                     AND sr.row_id = transactions.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )",
            [id],
            map_transaction_row,
        )
        .map_err(|e| e.to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at, updated_at
             FROM splits WHERE tx_id = ?1 ORDER BY id ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([id], map_split_row)
        .map_err(|e| e.to_string())?;

    let mut splits = Vec::new();
    for row in rows {
        splits.push(row.map_err(|e| e.to_string())?);
    }

    Ok(TransactionWithSplits { transaction, splits })
}

fn check_account_locks(
    conn: &rusqlite::Connection,
    txn_date: &str,
    account_ids: &HashSet<i64>,
) -> Result<(), String> {
    let mut locked = Vec::new();
    for account_id in account_ids {
        let lock_date: Option<String> = conn
            .query_row(
                "SELECT ab.as_of_date
                 FROM account_balancings ab
                 WHERE ab.account_id = ?1
                   AND ab.voided_at IS NULL
                   AND ab.as_of_date >= ?2
                   AND NOT EXISTS (
                       SELECT 1 FROM account_balancings newer
                       WHERE newer.previous_account_balancing_id = ab.id
                   )
                 ORDER BY as_of_date ASC LIMIT 1",
                params![account_id, txn_date],
                |row| row.get(0),
            )
            .optional()
            .map_err(|e| e.to_string())?;

        if let Some(date) = lock_date {
            locked.push((*account_id, date));
        }
    }

    if !locked.is_empty() {
        let details = locked
            .iter()
            .map(|(account_id, date)| format!("{account_id} (balanced through {date})"))
            .collect::<Vec<_>>()
            .join(", ");
        return Err(format!(
            "cannot post transaction dated {} to locked accounts: {}",
            txn_date, details
        ));
    }

    Ok(())
}

#[command]
pub fn create_transaction_with_splits(
    db: State<DbState>,
    input: CreateTransactionInput,
) -> Result<TransactionWithSplits, String> {
    if input.splits.len() < 2 {
        return Err("transaction must have at least two splits".to_string());
    }

    let sum: i64 = input.splits.iter().map(|s| s.amount_minor).sum();
    if sum != 0 {
        return Err("splits must balance to zero".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let book_id = SINGLE_BOOK_ID;

    let mut accounts = HashSet::new();
    for split in &input.splits {
        accounts.insert(split.account_id);
    }
    check_account_locks(&tx, &input.txn_date, &accounts)?;

    ensure_txn_date_format(&input.txn_date)?;
    let status = input.status.unwrap_or_else(|| "uncleared".to_string());
    let happened_at_utc = effective_happened_at_utc(&input.txn_date, input.happened_at_utc.as_deref())?;
    let posted_at_utc = normalize_optional_date_or_utc_timestamp(input.posted_at_utc.as_deref(), "posted_at_utc")?;

    tx.execute(
        "INSERT INTO transactions (
                book_id, previous_tx_id, txn_date, happened_at_utc, posted_at_utc,
                payee_id, memo, status, reference, import_id, session_id, created_at, updated_at
         )
         VALUES (
                ?1, NULL, ?2, ?3, ?4,
                ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.txn_date,
            happened_at_utc,
            posted_at_utc,
            input.payee_id,
            input.memo,
            status,
            input.reference,
            input.import_id,
            session_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let tx_id = tx.last_insert_rowid();
    clear_redo_stack(&tx, &session_id)?;
    record_insert_change(&tx, &session_id, "transactions", tx_id)?;

    for split in &input.splits {
        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                tx_id,
                split.account_id,
                split.commodity_id,
                split.amount_minor,
                split.category_id,
                split.tag_id,
                split.person_id,
                split.project_id,
                split.share_bps,
                split.memo,
            ],
        )
        .map_err(|e| e.to_string())?;
    }

    tx.commit().map_err(|e| e.to_string())?;
    fetch_transaction_with_splits(conn, tx_id)
}

#[command]
pub fn get_transaction_with_splits(
    db: State<DbState>,
    id: i64,
) -> Result<Option<TransactionWithSplits>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let tx_opt: Option<i64> = conn
        .query_row(
            "SELECT id FROM transactions WHERE id = ?1",
            [id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if tx_opt.is_none() {
        return Ok(None);
    }

    fetch_transaction_with_splits(conn, id).map(Some)
}

#[command]
pub fn list_transactions(
    db: State<DbState>,
    filter: ListTransactionsFilter,
) -> Result<Vec<TransactionWithSplits>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let limit = filter.limit.unwrap_or(200).max(1);
    let offset = filter.offset.unwrap_or(0).max(0);

    let mut sql = String::from(
        "SELECT DISTINCT t.id
         FROM transactions t
         LEFT JOIN splits s ON s.tx_id = t.id
         LEFT JOIN payees p ON p.id = t.payee_id
                 WHERE t.book_id = ?
                     AND t.status != 'void'
                     AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                     AND NOT EXISTS (
                         SELECT 1 FROM session_reverts sr
                         WHERE sr.table_name = 'transactions'
                             AND sr.row_id = t.id
                             AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                     )",
    );

    let mut params: Vec<Value> = vec![Value::from(book_id)];

    if let Some(account_id) = filter.account_id {
        sql.push_str(" AND s.account_id = ?");
        params.push(Value::from(account_id));
    }

    if let Some(payee_id) = filter.payee_id {
        sql.push_str(" AND t.payee_id = ?");
        params.push(Value::from(payee_id));
    }

    if let Some(date_from) = filter.date_from {
        sql.push_str(" AND t.txn_date >= ?");
        params.push(Value::from(date_from));
    }

    if let Some(date_to) = filter.date_to {
        sql.push_str(" AND t.txn_date <= ?");
        params.push(Value::from(date_to));
    }

    if let Some(search) = filter.search {
        let like = format!("%{}%", search);
        sql.push_str(" AND (t.memo LIKE ? OR t.reference LIKE ? OR p.name LIKE ?)");
        params.push(Value::from(like.clone()));
        params.push(Value::from(like.clone()));
        params.push(Value::from(like));
    }

    sql.push_str(" ORDER BY t.txn_date DESC, t.id DESC LIMIT ? OFFSET ?");
    params.push(Value::from(limit));
    params.push(Value::from(offset));

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params.iter()), |row| row.get::<_, i64>(0))
        .map_err(|e| e.to_string())?;

    let mut results = Vec::new();
    for row in rows {
        let id = row.map_err(|e| e.to_string())?;
        results.push(fetch_transaction_with_splits(conn, id)?);
    }

    Ok(results)
}

#[command]
pub fn update_transaction_with_splits(
    db: State<DbState>,
    input: UpdateTransactionInput,
) -> Result<TransactionWithSplits, String> {
    if input.splits.len() < 2 {
        return Err("transaction must have at least two splits".to_string());
    }

    let sum: i64 = input.splits.iter().map(|s| s.amount_minor).sum();
    if sum != 0 {
        return Err("splits must balance to zero".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let book_id = SINGLE_BOOK_ID;

    let existing_book: Option<i64> = tx
        .query_row(
                        "SELECT book_id
                         FROM transactions
                         WHERE id = ?1
                             AND status != 'void'
                             AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = transactions.id)
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'transactions'
                                     AND sr.row_id = transactions.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )",
            [input.id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    match existing_book {
        Some(existing) if existing == book_id => {}
        Some(_) => return Err("transaction does not belong to book".to_string()),
        None => return Err("transaction not found".to_string()),
    }

    let mut accounts = HashSet::new();
    for split in &input.splits {
        accounts.insert(split.account_id);
    }
    check_account_locks(&tx, &input.txn_date, &accounts)?;

    ensure_txn_date_format(&input.txn_date)?;
    let status = input.status.unwrap_or_else(|| "uncleared".to_string());
    let happened_at_utc = effective_happened_at_utc(&input.txn_date, input.happened_at_utc.as_deref())?;
    let posted_at_utc = normalize_optional_date_or_utc_timestamp(input.posted_at_utc.as_deref(), "posted_at_utc")?;

    tx.execute(
        "INSERT INTO transactions (
                book_id, previous_tx_id, txn_date, happened_at_utc, posted_at_utc,
                payee_id, memo, status, reference, import_id, session_id, created_at, updated_at
         )
         VALUES (
                ?1, ?2, ?3, ?4, ?5,
                ?6, ?7, ?8, ?9, ?10, ?11, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.id,
            input.txn_date,
            happened_at_utc,
            posted_at_utc,
            input.payee_id,
            input.memo,
            status,
            input.reference,
            input.import_id,
            session_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_tx_id = tx.last_insert_rowid();
    clear_redo_stack(&tx, &session_id)?;
    record_insert_change(&tx, &session_id, "transactions", new_tx_id)?;

    for split in &input.splits {
        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                new_tx_id,
                split.account_id,
                split.commodity_id,
                split.amount_minor,
                split.category_id,
                split.tag_id,
                split.person_id,
                split.project_id,
                split.share_bps,
                split.memo,
            ],
        )
        .map_err(|e| e.to_string())?;
    }

    tx.commit().map_err(|e| e.to_string())?;
    fetch_transaction_with_splits(conn, new_tx_id)
}

#[command]
pub fn delete_transaction(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let txn_date: Option<String> = tx
        .query_row(
                        "SELECT txn_date
                         FROM transactions
                         WHERE id = ?1
                             AND status != 'void'
                             AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = transactions.id)
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'transactions'
                                     AND sr.row_id = transactions.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )",
            [id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let txn_date = match txn_date {
        Some(date) => date,
        None => return Ok(false),
    };

    let mut stmt = tx
        .prepare("SELECT DISTINCT account_id FROM splits WHERE tx_id = ?1")
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([id], |row| row.get::<_, i64>(0))
        .map_err(|e| e.to_string())?;

    let mut accounts = HashSet::new();
    for row in rows {
        accounts.insert(row.map_err(|e| e.to_string())?);
    }
    drop(stmt);
    check_account_locks(&tx, &txn_date, &accounts)?;

    let (payee_id, memo, reference, import_id, happened_at_utc, posted_at_utc): (
        Option<i64>,
        Option<String>,
        Option<String>,
        Option<String>,
        String,
        Option<String>,
    ) = tx
        .query_row(
            "SELECT payee_id, memo, reference, import_id, happened_at_utc, posted_at_utc
             FROM transactions WHERE id = ?1",
            [id],
            |row| {
                Ok((
                    row.get(0)?,
                    row.get(1)?,
                    row.get(2)?,
                    row.get(3)?,
                    row.get(4)?,
                    row.get(5)?,
                ))
            },
        )
        .map_err(|e| e.to_string())?;

    tx.execute(
        "INSERT INTO transactions (
            book_id, previous_tx_id, txn_date, happened_at_utc, posted_at_utc,
            payee_id, memo, status, reference, import_id, session_id, created_at, updated_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5,
            ?6, ?7, 'void', ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            SINGLE_BOOK_ID,
            id,
            txn_date,
            happened_at_utc,
            posted_at_utc,
            payee_id,
            memo,
            reference,
            import_id,
            session_id,
        ],
    )
    .map_err(|e| e.to_string())?;
    let tombstone_id = tx.last_insert_rowid();
    clear_redo_stack(&tx, &session_id)?;
    record_insert_change(&tx, &session_id, "transactions", tombstone_id)?;

    tx.commit().map_err(|e| e.to_string())?;
    Ok(true)
}

#[allow(dead_code)]
#[command]
pub fn undo_active_session_change(db: State<DbState>) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let entry: Option<(i64, String, i64)> = tx
        .query_row(
            "SELECT seq, table_name, row_id
             FROM session_undo_stack
             WHERE session_id = ?1
             ORDER BY seq DESC
             LIMIT 1",
            [session_id.as_str()],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((seq, table_name, row_id)) = entry else {
        return Ok(false);
    };

    tx.execute("DELETE FROM session_undo_stack WHERE seq = ?1", [seq])
        .map_err(|e| e.to_string())?;
    tx.execute(
        "INSERT INTO session_redo_stack (session_id, table_name, row_id) VALUES (?1, ?2, ?3)",
        params![session_id, table_name, row_id],
    )
    .map_err(|e| e.to_string())?;
    tx.execute(
        "INSERT OR IGNORE INTO session_reverts (session_id, table_name, row_id) VALUES (?1, ?2, ?3)",
        params![session_id, table_name, row_id],
    )
    .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;
    Ok(true)
}

#[allow(dead_code)]
#[command]
pub fn redo_active_session_change(db: State<DbState>) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let entry: Option<(i64, String, i64)> = tx
        .query_row(
            "SELECT seq, table_name, row_id
             FROM session_redo_stack
             WHERE session_id = ?1
             ORDER BY seq DESC
             LIMIT 1",
            [session_id.as_str()],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((seq, table_name, row_id)) = entry else {
        return Ok(false);
    };

    tx.execute("DELETE FROM session_redo_stack WHERE seq = ?1", [seq])
        .map_err(|e| e.to_string())?;
    tx.execute(
        "INSERT INTO session_undo_stack (session_id, table_name, row_id) VALUES (?1, ?2, ?3)",
        params![session_id, table_name, row_id],
    )
    .map_err(|e| e.to_string())?;
    tx.execute(
        "DELETE FROM session_reverts WHERE session_id = ?1 AND table_name = ?2 AND row_id = ?3",
        params![session_id, table_name, row_id],
    )
    .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;
    Ok(true)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::open_and_migrate;
    use crate::state::DbStateInner;
    use rusqlite::params;
    use std::fs;
    use std::sync::{atomic::{AtomicUsize, Ordering}, Mutex};
    use std::time::{SystemTime, UNIX_EPOCH};

    static TEST_COUNTER: AtomicUsize = AtomicUsize::new(0);

    fn create_temp_db() -> DbState {
        let start = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis();
        let counter = TEST_COUNTER.fetch_add(1, Ordering::SeqCst);
        let mut temp = std::env::temp_dir();
        temp.push(format!("rekenraam_txn_test_{start}_{counter}"));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");

        let (conn, db_path, audit_user) = open_and_migrate(&temp).expect("open and migrate");
        DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path),
                conn: Some(conn),
                audit_user: Some(audit_user),
            }),
        }
    }

    fn as_state<'a>(db: &'a DbState) -> State<'a, DbState> {
        unsafe { std::mem::transmute::<&'a DbState, State<'a, DbState>>(db) }
    }

    fn lookup_account_id(conn: &rusqlite::Connection, name: &str) -> i64 {
        conn.query_row(
            "SELECT id FROM accounts WHERE name = ?1 LIMIT 1",
            [name],
            |row| row.get(0),
        )
        .expect("account id")
    }

    fn lookup_usd_id(conn: &rusqlite::Connection) -> i64 {
        conn.query_row(
            "SELECT id FROM commodities WHERE symbol = 'USD' LIMIT 1",
            [],
            |row| row.get(0),
        )
        .expect("usd id")
    }

    fn create_expense_account(conn: &rusqlite::Connection) -> i64 {
        let book_id: i64 = conn
            .query_row("SELECT id FROM books WHERE name='Personal' LIMIT 1", [], |row| row.get(0))
            .expect("book id");
        let commodity_id = lookup_usd_id(conn);
        conn.execute(
            "INSERT INTO accounts (book_id, type, name, commodity_id) VALUES (?1, 'expense', 'Test Expense', ?2)",
            params![book_id, commodity_id],
        )
        .expect("insert expense account");
        conn.last_insert_rowid()
    }

    #[test]
    fn test_create_update_register() {
        let db_state = create_temp_db();
        let (checking_id, expense_id, commodity_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let checking_id = lookup_account_id(conn, "Checking Account");
            let expense_id = create_expense_account(conn);
            let commodity_id = lookup_usd_id(conn);
            (checking_id, expense_id, commodity_id)
        };

        let created = create_transaction_with_splits(
            as_state(&db_state),
            CreateTransactionInput {
                book_id: 1,
                txn_date: "2024-02-01".to_string(),
                happened_at_utc: None,
                posted_at_utc: None,
                payee_id: None,
                memo: Some("Groceries".to_string()),
                status: Some("cleared".to_string()),
                reference: None,
                import_id: None,
                splits: vec![
                    CreateSplit {
                        account_id: checking_id,
                        commodity_id,
                        amount_minor: -5000,
                        category_id: None,
                        tag_id: None,
                        person_id: None,
                        project_id: None,
                        share_bps: None,
                        memo: None,
                    },
                    CreateSplit {
                        account_id: expense_id,
                        commodity_id,
                        amount_minor: 5000,
                        category_id: None,
                        tag_id: None,
                        person_id: None,
                        project_id: None,
                        share_bps: None,
                        memo: None,
                    },
                ],
            },
        )
        .expect("create transaction");
        assert_eq!(created.splits.len(), 2);
        assert_eq!(created.transaction.happened_at_utc, "2024-02-01");

        let register = list_account_register_with_balance(
            as_state(&db_state),
            checking_id,
            None,
            None,
        )
        .expect("register with balance");
        assert_eq!(register.len(), 1);
        assert_eq!(register[0].running_balance_minor, -5000);

        let updated = update_transaction_with_splits(
            as_state(&db_state),
            UpdateTransactionInput {
                id: created.transaction.id,
                book_id: 1,
                txn_date: "2024-02-01".to_string(),
                happened_at_utc: None,
                posted_at_utc: None,
                payee_id: None,
                memo: Some("Groceries updated".to_string()),
                status: Some("cleared".to_string()),
                reference: None,
                import_id: None,
                splits: vec![
                    CreateSplit {
                        account_id: checking_id,
                        commodity_id,
                        amount_minor: -7000,
                        category_id: None,
                        tag_id: None,
                        person_id: None,
                        project_id: None,
                        share_bps: None,
                        memo: None,
                    },
                    CreateSplit {
                        account_id: expense_id,
                        commodity_id,
                        amount_minor: 7000,
                        category_id: None,
                        tag_id: None,
                        person_id: None,
                        project_id: None,
                        share_bps: None,
                        memo: None,
                    },
                ],
            },
        )
        .expect("update transaction");
        assert_eq!(updated.splits.len(), 2);

        let register = list_account_register_with_balance(
            as_state(&db_state),
            checking_id,
            None,
            None,
        )
        .expect("register with balance updated");
        assert_eq!(register.len(), 1);
        assert_eq!(register[0].running_balance_minor, -7000);
    }

    #[test]
    fn test_normalize_date_or_utc_timestamp() {
        assert_eq!(
            normalize_date_or_utc_timestamp("2024-02-01", "happened_at_utc").expect("date value"),
            "2024-02-01"
        );

        assert_eq!(
            normalize_date_or_utc_timestamp("2024-02-01T10:00:00+02:00", "posted_at_utc")
                .expect("timestamp value"),
            "2024-02-01T08:00:00.000Z"
        );
    }
}

#[command]
pub fn ensure_currency_trading_balances(
    db: State<DbState>,
    tx_id: i64,
) -> Result<i64, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let mut adjustments: Vec<(i64, i64)> = Vec::new();
    {
        let mut stmt = tx
            .prepare(
                "SELECT commodity_id, COALESCE(SUM(amount_minor), 0) AS net_amount
                 FROM splits WHERE tx_id = ?1
                 GROUP BY commodity_id
                 HAVING net_amount != 0",
            )
            .map_err(|e| e.to_string())?;

        let rows = stmt
            .query_map([tx_id], |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)))
            .map_err(|e| e.to_string())?;

        for row in rows {
            adjustments.push(row.map_err(|e| e.to_string())?);
        }
    }

    if adjustments.is_empty() {
        return Ok(0);
    }

    let inserted = {
        let mut inserted = 0;
        for (commodity_id, net_amount) in adjustments {
            let trading_account_id: i64 = tx
                .query_row(
                    "SELECT id FROM accounts
                     WHERE book_id = ?1 AND commodity_id = ?2 AND type = 'equity' AND name = 'Currency Trading'
                     LIMIT 1",
                    params![book_id, commodity_id],
                    |row| row.get(0),
                )
                .optional()
                .map_err(|e| e.to_string())?
                .unwrap_or(0);

            let trading_account_id = if trading_account_id == 0 {
                tx.execute(
                    "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, is_closed, created_at, updated_at)
                     VALUES (?1, NULL, 'equity', 'Currency Trading', ?2, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![book_id, commodity_id],
                )
                .map_err(|e| e.to_string())?;
                tx.last_insert_rowid()
            } else {
                trading_account_id
            };

            tx.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx_id, trading_account_id, commodity_id, -net_amount],
            )
            .map_err(|e| e.to_string())?;

            inserted += 1;
        }
        inserted
    };

    tx.commit().map_err(|e| e.to_string())?;
    Ok(inserted)
}

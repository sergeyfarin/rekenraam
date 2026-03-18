use std::collections::HashSet;

use chrono::{DateTime, NaiveDate, Utc};
use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::error::AppError;
use crate::session::{current_session_id, clear_redo_stack, record_insert_change};
use crate::state::DbState;
use crate::validation::{validate_tx_status, validate_memo, validate_amount_minor, escape_like};

const SINGLE_BOOK_ID: i64 = 1;

#[derive(Serialize, Deserialize, Debug)]
pub struct Transaction {
    pub id: i64,
    pub book_id: i64,
    pub previous_tx_id: Option<i64>,
    #[serde(rename = "txn_date")]
    pub occurred_date: String,
    pub occurred_at_utc: Option<String>,
    pub occurred_tz: Option<String>,
    pub posted_date: String,
    pub posted_at_utc: Option<String>,
    pub posted_tz: Option<String>,
    pub payee_id: Option<i64>,
    pub memo: Option<String>,
    pub status: String,
    pub reference: Option<String>,
    pub import_id: Option<String>,
    pub created_at: String,
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
    #[serde(rename = "txn_date")]
    pub occurred_date: String,
    pub occurred_at_utc: Option<String>,
    pub occurred_tz: Option<String>,
    pub posted_date: Option<String>,
    pub posted_at_utc: Option<String>,
    pub posted_tz: Option<String>,
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
    #[serde(rename = "txn_date")]
    pub occurred_date: String,
    pub occurred_at_utc: Option<String>,
    pub occurred_tz: Option<String>,
    pub posted_date: Option<String>,
    pub posted_at_utc: Option<String>,
    pub posted_tz: Option<String>,
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
    pub status: Option<String>,
    pub amount_min: Option<i64>,
    pub amount_max: Option<i64>,
    pub sort_by: Option<String>,  // "date" | "payee" | "amount" | "status"
    pub sort_dir: Option<String>, // "asc" | "desc"
    pub limit: Option<i64>,
    pub offset: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct RegisterEntry {
    pub tx_id: i64,
    #[serde(rename = "txn_date")]
    pub occurred_date: String,
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
    #[serde(rename = "txn_date")]
    pub occurred_date: String,
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
    #[serde(rename = "txn_date")]
    pub occurred_date: String,
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
        occurred_date: row.get(3)?,
        occurred_at_utc: row.get(4)?,
        occurred_tz: row.get(5)?,
        posted_date: row.get(6)?,
        posted_at_utc: row.get(7)?,
        posted_tz: row.get(8)?,
        payee_id: row.get(9)?,
        memo: row.get(10)?,
        status: row.get(11)?,
        reference: row.get(12)?,
        import_id: row.get(13)?,
        created_at: row.get(14)?,
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
    })
}

fn map_register_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<RegisterEntry> {
    Ok(RegisterEntry {
        tx_id: row.get(0)?,
        occurred_date: row.get(1)?,
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
        occurred_date: row.get(1)?,
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
        occurred_date: row.get(1)?,
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

fn ensure_occurred_date_format(occurred_date: &str) -> Result<(), String> {
    NaiveDate::parse_from_str(occurred_date.trim(), "%Y-%m-%d")
        .map(|_| ())
        .map_err(|_| "occurred_date must be in YYYY-MM-DD format".to_string())
}

fn ensure_posted_date_format(posted_date: &str) -> Result<(), String> {
    NaiveDate::parse_from_str(posted_date.trim(), "%Y-%m-%d")
        .map(|_| ())
        .map_err(|_| "posted_date must be in YYYY-MM-DD format".to_string())
}

fn normalize_utc_timestamp(value: &str, field_name: &str) -> Result<String, String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return Err(format!("{field_name} cannot be empty"));
    }

    let parsed = DateTime::parse_from_rfc3339(trimmed)
        .map_err(|_| format!("{field_name} must be a valid ISO-8601 UTC timestamp"))?;
    Ok(parsed
        .with_timezone(&Utc)
        .format("%Y-%m-%dT%H:%M:%S%.3fZ")
        .to_string())
}

fn normalize_optional_utc_timestamp(
    value: Option<&str>,
    field_name: &str,
) -> Result<Option<String>, String> {
    if let Some(raw) = value {
        let trimmed = raw.trim();
        if trimmed.is_empty() {
            return Ok(None);
        }
        return normalize_utc_timestamp(trimmed, field_name).map(Some);
    }
    Ok(None)
}

fn resolve_tz(
    value: Option<&str>,
    field_name: &str,
    required_when_timestamp: bool,
) -> Result<Option<String>, String> {
    let normalized = value
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(|v| v.to_string());
    if required_when_timestamp && normalized.is_none() {
        return Err(format!("{field_name} is required when timestamp is set"));
    }
    Ok(normalized)
}

#[command]
pub fn list_account_register(
    db: State<DbState>,
    account_id: i64,
    limit: Option<i64>,
    offset: Option<i64>,
) -> Result<Vec<RegisterEntry>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = limit.unwrap_or(200).max(1);
    let offset = offset.unwrap_or(0).max(0);

    let mut stmt = conn
        .prepare(
            "SELECT
                t.id,
                t.occurred_date,
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
             ORDER BY t.occurred_date DESC, COALESCE(t.occurred_at_utc, t.posted_at_utc) DESC, t.id DESC, s.id ASC
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
) -> Result<Vec<RegisterEntryWithBalance>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = limit.unwrap_or(200).max(1);
    let offset = offset.unwrap_or(0).max(0);

    let mut stmt = conn
        .prepare(
            "WITH base AS (
                SELECT
                    t.id AS tx_id,
                    t.occurred_date,
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
                        ORDER BY t.occurred_date ASC, COALESCE(t.occurred_at_utc, t.posted_at_utc) ASC, t.id ASC, s.id ASC
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
                occurred_date,
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
            ORDER BY occurred_date DESC, tx_id DESC, split_id ASC
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
) -> Result<Vec<PostingEntry>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = limit.unwrap_or(200).max(1);
    let offset = offset.unwrap_or(0).max(0);

    let mut sql = String::from(
        "SELECT
            t.id,
            t.occurred_date,
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
        sql.push_str(" AND t.occurred_date >= ?");
        params.push(Value::from(date_from));
    }
    if let Some(date_to) = date_to {
        sql.push_str(" AND t.occurred_date <= ?");
        params.push(Value::from(date_to));
    }

    sql.push_str(" ORDER BY t.occurred_date DESC, COALESCE(t.occurred_at_utc, t.posted_at_utc) DESC, t.id DESC, s.id ASC LIMIT ? OFFSET ?");
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
                        "SELECT id, book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
                                        payee_id, memo, status, reference, import_id, created_at
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
            "SELECT id, tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at
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
    occurred_date: &str,
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
                params![account_id, occurred_date],
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
            occurred_date, details
        ));
    }

    Ok(())
}

#[command]
pub fn create_transaction_with_splits(
    db: State<DbState>,
    input: CreateTransactionInput,
) -> Result<TransactionWithSplits, AppError> {
    if input.splits.len() < 2 {
        return Err(AppError::validation("splits", "transaction must have at least two splits"));
    }

    if let Some(ref s) = input.status {
        validate_tx_status(s)?;
    }
    if let Some(ref m) = input.memo {
        validate_memo(m)?;
    }
    for split in &input.splits {
        validate_amount_minor(split.amount_minor)?;
        if let Some(ref m) = split.memo {
            validate_memo(m)?;
        }
    }

    let sum: i64 = input.splits.iter().map(|s| s.amount_minor).sum();
    if sum != 0 {
        return Err(AppError::validation("splits", "splits must balance to zero"));
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
    check_account_locks(&tx, &input.occurred_date, &accounts)?;

    ensure_occurred_date_format(&input.occurred_date)?;
    let status = input.status.unwrap_or_else(|| "uncleared".to_string());
    let occurred_at_utc = normalize_optional_utc_timestamp(input.occurred_at_utc.as_deref(), "occurred_at_utc")?;
    let occurred_tz = match occurred_at_utc.as_ref() {
        Some(_) => {
            let tz = input
                .occurred_tz
                .as_deref()
                .map(str::trim)
                .filter(|v| !v.is_empty())
                .ok_or_else(|| "occurred_tz is required when occurred_at_utc is set".to_string())?;
            Some(tz.to_string())
        }
        None => input
            .occurred_tz
            .as_deref()
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .map(|v| v.to_string()),
    };
    let posted_at_utc = normalize_optional_utc_timestamp(input.posted_at_utc.as_deref(), "posted_at_utc")?;
    let posted_tz = resolve_tz(input.posted_tz.as_deref(), "posted_tz", posted_at_utc.is_some())?;
    let posted_date = input
        .posted_date
        .as_deref()
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(|v| v.to_string())
        .or_else(|| posted_at_utc.as_ref().and_then(|value| value.get(0..10).map(|v| v.to_string())))
        .unwrap_or_else(|| input.occurred_date.clone());
    ensure_posted_date_format(&posted_date)?;

    tx.execute(
        "INSERT INTO transactions (
                book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
              payee_id, memo, status, reference, import_id, session_id, created_at
         )
         VALUES (
                ?1, NULL, ?2, ?3, ?4, ?5, ?6, ?7,
              ?8, ?9, ?10, ?11, ?12, ?13, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.occurred_date,
            occurred_at_utc,
            occurred_tz,
            posted_date,
            posted_at_utc,
            posted_tz,
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
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
    Ok(fetch_transaction_with_splits(conn, tx_id)?)
}

#[command]
pub fn get_transaction_with_splits(
    db: State<DbState>,
    id: i64,
) -> Result<Option<TransactionWithSplits>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

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

    Ok(Some(fetch_transaction_with_splits(conn, id)?))
}

#[command]
pub fn list_transactions(
    db: State<DbState>,
    filter: ListTransactionsFilter,
) -> Result<Vec<TransactionWithSplits>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let limit = filter.limit.unwrap_or(50).max(1);
    let offset = filter.offset.unwrap_or(0).max(0);

    // Status filter: default excludes void; if caller passes "void" show only void; "all" shows everything
    let status_filter = filter.status.as_deref().unwrap_or("active");

    let mut sql = String::from(
        "SELECT DISTINCT t.id
         FROM transactions t
         LEFT JOIN splits s ON s.tx_id = t.id
         LEFT JOIN payees p ON p.id = t.payee_id
                 WHERE t.book_id = ?
                     AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                     AND NOT EXISTS (
                         SELECT 1 FROM session_reverts sr
                         WHERE sr.table_name = 'transactions'
                             AND sr.row_id = t.id
                             AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                     )",
    );

    let mut params: Vec<Value> = vec![Value::from(book_id)];

    // Status filtering
    match status_filter {
        "all" => {} // no extra filter
        "void" => {
            sql.push_str(" AND t.status = 'void'");
        }
        _ => {
            // "active" = exclude void
            sql.push_str(" AND t.status != 'void'");
        }
    }

    if let Some(account_id) = filter.account_id {
        sql.push_str(" AND s.account_id = ?");
        params.push(Value::from(account_id));
    }

    if let Some(payee_id) = filter.payee_id {
        sql.push_str(" AND t.payee_id = ?");
        params.push(Value::from(payee_id));
    }

    if let Some(date_from) = filter.date_from {
        sql.push_str(" AND t.occurred_date >= ?");
        params.push(Value::from(date_from));
    }

    if let Some(date_to) = filter.date_to {
        sql.push_str(" AND t.occurred_date <= ?");
        params.push(Value::from(date_to));
    }

    if let Some(search) = filter.search {
        let like = format!("%{}%", escape_like(&search));
        sql.push_str(" AND (t.memo LIKE ? ESCAPE '\\' OR t.reference LIKE ? ESCAPE '\\' OR p.name LIKE ? ESCAPE '\\')");
        params.push(Value::from(like.clone()));
        params.push(Value::from(like.clone()));
        params.push(Value::from(like));
    }

    // Amount range filter: join needs to be handled carefully with DISTINCT — use subquery
    if filter.amount_min.is_some() || filter.amount_max.is_some() {
        sql.push_str(" AND EXISTS (SELECT 1 FROM splits sa WHERE sa.tx_id = t.id");
        if let Some(min) = filter.amount_min {
            sql.push_str(" AND ABS(sa.amount_minor) >= ?");
            params.push(Value::from(min));
        }
        if let Some(max) = filter.amount_max {
            sql.push_str(" AND ABS(sa.amount_minor) <= ?");
            params.push(Value::from(max));
        }
        sql.push_str(")");
    }

    // Sort order
    let order_clause = match filter.sort_by.as_deref().unwrap_or("date") {
        "payee" => {
            let dir = if filter.sort_dir.as_deref() == Some("asc") { "ASC" } else { "DESC" };
            format!("p.name {} NULLS LAST, t.occurred_date DESC, t.id DESC", dir)
        }
        "status" => {
            let dir = if filter.sort_dir.as_deref() == Some("asc") { "ASC" } else { "DESC" };
            format!("t.status {}, t.occurred_date DESC, t.id DESC", dir)
        }
        _ => {
            // default: date
            let dir = if filter.sort_dir.as_deref() == Some("asc") { "ASC" } else { "DESC" };
            format!("t.occurred_date {}, COALESCE(t.occurred_at_utc, t.posted_at_utc) {}, t.id {}", dir, dir, dir)
        }
    };
    sql.push_str(&format!(" ORDER BY {} LIMIT ? OFFSET ?", order_clause));
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
) -> Result<TransactionWithSplits, AppError> {
    if input.splits.len() < 2 {
        return Err(AppError::validation("splits", "transaction must have at least two splits"));
    }

    if let Some(ref s) = input.status {
        validate_tx_status(s)?;
    }
    if let Some(ref m) = input.memo {
        validate_memo(m)?;
    }
    for split in &input.splits {
        validate_amount_minor(split.amount_minor)?;
        if let Some(ref m) = split.memo {
            validate_memo(m)?;
        }
    }

    let sum: i64 = input.splits.iter().map(|s| s.amount_minor).sum();
    if sum != 0 {
        return Err(AppError::validation("splits", "splits must balance to zero"));
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
        Some(_) => return Err(AppError::conflict("transaction does not belong to book")),
        None => return Err(AppError::not_found("transaction", input.id.to_string())),
    }

    let mut accounts = HashSet::new();
    for split in &input.splits {
        accounts.insert(split.account_id);
    }
    check_account_locks(&tx, &input.occurred_date, &accounts)?;

    ensure_occurred_date_format(&input.occurred_date)?;
    let status = input.status.unwrap_or_else(|| "uncleared".to_string());
    let occurred_at_utc = normalize_optional_utc_timestamp(input.occurred_at_utc.as_deref(), "occurred_at_utc")?;
    let occurred_tz = match occurred_at_utc.as_ref() {
        Some(_) => {
            let tz = input
                .occurred_tz
                .as_deref()
                .map(str::trim)
                .filter(|v| !v.is_empty())
                .ok_or_else(|| "occurred_tz is required when occurred_at_utc is set".to_string())?;
            Some(tz.to_string())
        }
        None => input
            .occurred_tz
            .as_deref()
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .map(|v| v.to_string()),
    };
    let posted_at_utc = normalize_optional_utc_timestamp(input.posted_at_utc.as_deref(), "posted_at_utc")?;
    let posted_tz = resolve_tz(input.posted_tz.as_deref(), "posted_tz", posted_at_utc.is_some())?;
    let posted_date = input
        .posted_date
        .as_deref()
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(|v| v.to_string())
        .or_else(|| posted_at_utc.as_ref().and_then(|value| value.get(0..10).map(|v| v.to_string())))
        .unwrap_or_else(|| input.occurred_date.clone());
    ensure_posted_date_format(&posted_date)?;

    tx.execute(
        "INSERT INTO transactions (
                book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
              payee_id, memo, status, reference, import_id, session_id, created_at
         )
         VALUES (
                ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8,
              ?9, ?10, ?11, ?12, ?13, ?14, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.id,
            input.occurred_date,
            occurred_at_utc,
            occurred_tz,
            posted_date,
            posted_at_utc,
            posted_tz,
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
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
    Ok(fetch_transaction_with_splits(conn, new_tx_id)?)
}

#[command]
pub fn delete_transaction(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let occurred_date: Option<String> = tx
        .query_row(
                        "SELECT occurred_date
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

    let occurred_date = match occurred_date {
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
    check_account_locks(&tx, &occurred_date, &accounts)?;

    let (payee_id, memo, reference, import_id, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz): (
        Option<i64>,
        Option<String>,
        Option<String>,
        Option<String>,
        Option<String>,
        Option<String>,
        String,
        Option<String>,
        Option<String>,
    ) = tx
        .query_row(
            "SELECT payee_id, memo, reference, import_id, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz
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
                    row.get(6)?,
                    row.get(7)?,
                    row.get(8)?,
                ))
            },
        )
        .map_err(|e| e.to_string())?;

    tx.execute(
        "INSERT INTO transactions (
            book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
                payee_id, memo, status, reference, import_id, session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8,
                ?9, ?10, 'void', ?11, ?12, ?13, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            SINGLE_BOOK_ID,
            id,
            occurred_date,
            occurred_at_utc,
            occurred_tz,
            posted_date,
            posted_at_utc,
            posted_tz,
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
pub fn undo_active_session_change(db: State<DbState>) -> Result<bool, AppError> {
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
pub fn redo_active_session_change(db: State<DbState>) -> Result<bool, AppError> {
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
    use crate::db::{open_and_migrate, open_read_conn};
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
        let read_conn = open_read_conn(&db_path).expect("open read conn");
        DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path),
                conn: Some(conn),
                audit_user: Some(audit_user),
            }),
            read_conn: Mutex::new(Some(read_conn)),
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
                occurred_date: "2024-02-01".to_string(),
                occurred_at_utc: None,
                occurred_tz: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
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
        assert_eq!(created.transaction.occurred_at_utc, None);

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
                occurred_date: "2024-02-01".to_string(),
                occurred_at_utc: None,
                occurred_tz: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
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
            normalize_utc_timestamp("2024-02-01T10:00:00+02:00", "posted_at_utc")
                .expect("timestamp value"),
            "2024-02-01T08:00:00.000Z"
        );
    }
}

#[command]
pub fn ensure_currency_trading_balances(
    db: State<DbState>,
    tx_id: i64,
) -> Result<i64, AppError> {
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
                    "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, is_closed, created_at)
                     VALUES (?1, NULL, 'equity', 'Currency Trading', ?2, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![book_id, commodity_id],
                )
                .map_err(|e| e.to_string())?;
                tx.last_insert_rowid()
            } else {
                trading_account_id
            };

            tx.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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

// ── Payee defaults ────────────────────────────────────────────────────────────

#[derive(Serialize, Deserialize, Debug)]
pub struct PayeeDefaults {
    pub category_id: Option<i64>,
    pub memo: Option<String>,
}

/// Return the category and memo from the most recent non-void transaction for
/// the given payee.  Optionally restricted to a specific account.
#[command]
pub fn get_payee_defaults(
    db: State<DbState>,
    payee_id: i64,
    account_id: Option<i64>,
) -> Result<PayeeDefaults, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    // Look for the most recent qualifying split: category set, payee matches, not void/superseded.
    let row: Option<(Option<i64>, Option<String>)> = if let Some(acct) = account_id {
        conn.query_row(
            "SELECT s.category_id, t.memo
             FROM splits s
             JOIN transactions t ON t.id = s.tx_id
             WHERE t.book_id = ?1
               AND t.payee_id = ?2
               AND s.account_id = ?3
               AND s.category_id IS NOT NULL
               AND t.status NOT IN ('void')
               AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
             ORDER BY t.occurred_date DESC, t.id DESC
             LIMIT 1",
            params![book_id, payee_id, acct],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?
    } else {
        conn.query_row(
            "SELECT s.category_id, t.memo
             FROM splits s
             JOIN transactions t ON t.id = s.tx_id
             WHERE t.book_id = ?1
               AND t.payee_id = ?2
               AND s.category_id IS NOT NULL
               AND t.status NOT IN ('void')
               AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
             ORDER BY t.occurred_date DESC, t.id DESC
             LIMIT 1",
            params![book_id, payee_id],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?
    };

    Ok(match row {
        Some((category_id, memo)) => PayeeDefaults { category_id, memo },
        None => PayeeDefaults { category_id: None, memo: None },
    })
}

// ── Duplicate transaction ─────────────────────────────────────────────────────

/// Create a copy of an existing transaction with today's date and status=uncleared.
#[command]
pub fn duplicate_transaction(
    db: State<DbState>,
    id: i64,
    today: String,
) -> Result<TransactionWithSplits, AppError> {
    ensure_occurred_date_format(&today)?;

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    // Fetch the source transaction with its splits
    let source = {
        let tx_row: Option<(Option<i64>, Option<String>, i64)> = conn.query_row(
            "SELECT payee_id, memo, book_id FROM transactions WHERE id = ?1 AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = transactions.id)",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)),
        ).optional().map_err(|e| e.to_string())?;

        match tx_row {
            None => return Err(AppError::not_found("transaction", id.to_string())),
            Some(row) => row,
        }
    };
    let (payee_id, memo, _book_id) = source;

    let splits: Vec<(i64, i64, i64, Option<i64>, Option<i64>, Option<i64>, Option<i64>, Option<i64>, Option<String>)> = {
        let mut stmt = conn
            .prepare(
                "SELECT account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo
                 FROM splits WHERE tx_id = ?1 ORDER BY id ASC",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map([id], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, i64>(2)?,
                    row.get::<_, Option<i64>>(3)?,
                    row.get::<_, Option<i64>>(4)?,
                    row.get::<_, Option<i64>>(5)?,
                    row.get::<_, Option<i64>>(6)?,
                    row.get::<_, Option<i64>>(7)?,
                    row.get::<_, Option<String>>(8)?,
                ))
            })
            .map_err(|e| e.to_string())?;
        let mut v = Vec::new();
        for r in rows { v.push(r.map_err(|e| e.to_string())?); }
        v
    };

    if splits.is_empty() {
        return Err(AppError::conflict("source transaction has no splits"));
    }

    let book_id = SINGLE_BOOK_ID;
    let db_tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&db_tx)?;

    let account_ids: HashSet<i64> = splits.iter().map(|s| s.0).collect();
    check_account_locks(&db_tx, &today, &account_ids)?;

    db_tx.execute(
        "INSERT INTO transactions (book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz, payee_id, memo, status, reference, import_id, session_id, created_at)
         VALUES (?1, NULL, ?2, NULL, NULL, ?2, NULL, NULL, ?3, ?4, 'uncleared', NULL, NULL, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, today, payee_id, memo, session_id],
    ).map_err(|e| e.to_string())?;
    let new_tx_id = db_tx.last_insert_rowid();

    for (account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, split_memo) in &splits {
        db_tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![new_tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, split_memo],
        ).map_err(|e| e.to_string())?;
    }

    record_insert_change(&db_tx, &session_id, "transactions", new_tx_id)?;
    clear_redo_stack(&db_tx, &session_id)?;
    db_tx.commit().map_err(|e| e.to_string())?;

    let result = fetch_transaction_with_splits(conn, new_tx_id)?;
    Ok(result)
}

// ── Bulk operations ───────────────────────────────────────────────────────────

/// Void multiple transactions by ID. Skips locked or already-void transactions
/// and returns the count voided.
#[command]
pub fn bulk_void_transactions(
    db: State<DbState>,
    ids: Vec<i64>,
) -> Result<usize, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let book_id = SINGLE_BOOK_ID;
    let mut voided = 0;

    for id in ids {
        // Fetch the current transaction
        let row: Option<(String, Option<i64>, Option<String>, Option<String>)> = conn
            .query_row(
                "SELECT occurred_date, payee_id, memo, reference
                 FROM transactions WHERE id = ?1 AND book_id = ?2 AND status != 'void'
                   AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = transactions.id)",
                params![id, book_id],
                |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)),
            )
            .optional()
            .map_err(|e| e.to_string())?;

        let (occurred_date, payee_id, memo, reference) = match row {
            None => continue, // skip: not found, already void, or superseded
            Some(r) => r,
        };

        // Get splits
        let split_rows: Vec<(i64, i64, i64, Option<i64>, Option<i64>, Option<i64>, Option<i64>, Option<i64>, Option<String>)> = {
            let mut stmt = conn
                .prepare("SELECT account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo FROM splits WHERE tx_id = ?1 ORDER BY id ASC")
                .map_err(|e| e.to_string())?;
            let rows = stmt.query_map([id], |row| Ok((
                row.get(0)?, row.get(1)?, row.get(2)?,
                row.get(3)?, row.get(4)?, row.get(5)?, row.get(6)?, row.get(7)?, row.get(8)?,
            ))).map_err(|e| e.to_string())?;
            let mut v = Vec::new();
            for r in rows { v.push(r.map_err(|e| e.to_string())?); }
            v
        };

        // Check locks
        let account_ids: HashSet<i64> = split_rows.iter().map(|s| s.0).collect();
        if check_account_locks(conn, &occurred_date, &account_ids).is_err() {
            continue; // skip locked
        }

        let db_tx = conn.transaction().map_err(|e| e.to_string())?;
        let session_id = current_session_id(&db_tx)?;

        db_tx.execute(
            "INSERT INTO transactions (book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz, payee_id, memo, status, reference, import_id, session_id, created_at)
             VALUES (?1, ?2, ?3, NULL, NULL, ?3, NULL, NULL, ?4, ?5, 'void', ?6, NULL, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![book_id, id, occurred_date, payee_id, memo, reference, session_id],
        ).map_err(|e| e.to_string())?;
        let new_tx_id = db_tx.last_insert_rowid();

        for (account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, split_memo) in &split_rows {
            db_tx.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, memo, created_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![new_tx_id, account_id, commodity_id, amount_minor, category_id, tag_id, person_id, project_id, share_bps, split_memo],
            ).map_err(|e| e.to_string())?;
        }

        record_insert_change(&db_tx, &session_id, "transactions", new_tx_id)?;
        clear_redo_stack(&db_tx, &session_id)?;
        db_tx.commit().map_err(|e| e.to_string())?;
        voided += 1;
    }

    Ok(voided)
}

/// Delete multiple transactions by ID. Skips locked transactions.
#[command]
pub fn bulk_delete_transactions(
    db: State<DbState>,
    ids: Vec<i64>,
) -> Result<usize, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let book_id = SINGLE_BOOK_ID;
    let mut deleted = 0;

    for id in ids {
        // Verify it exists and is not locked
        let row: Option<String> = conn
            .query_row(
                "SELECT occurred_date FROM transactions WHERE id = ?1 AND book_id = ?2
                   AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = transactions.id)",
                params![id, book_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|e| e.to_string())?;

        let occurred_date = match row {
            None => continue,
            Some(d) => d,
        };

        let split_account_ids: Vec<i64> = {
            let mut stmt = conn.prepare("SELECT DISTINCT account_id FROM splits WHERE tx_id = ?1").map_err(|e| e.to_string())?;
            let rows = stmt.query_map([id], |row| row.get(0)).map_err(|e| e.to_string())?;
            let mut v = Vec::new();
            for r in rows { v.push(r.map_err(|e| e.to_string())?); }
            v
        };
        let account_set: HashSet<i64> = split_account_ids.into_iter().collect();
        if check_account_locks(conn, &occurred_date, &account_set).is_err() {
            continue;
        }

        let db_tx = conn.transaction().map_err(|e| e.to_string())?;
        let session_id = current_session_id(&db_tx)?;
        db_tx.execute("DELETE FROM splits WHERE tx_id = ?1", [id]).map_err(|e| e.to_string())?;
        db_tx.execute("DELETE FROM transactions WHERE id = ?1", [id]).map_err(|e| e.to_string())?;
        clear_redo_stack(&db_tx, &session_id)?;
        db_tx.commit().map_err(|e| e.to_string())?;
        deleted += 1;
    }

    Ok(deleted)
}

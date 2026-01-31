#[path = "db_accounts.rs"]
pub mod db_accounts;
#[path = "db_commodities.rs"]
pub mod db_commodities;
#[path = "db_storage.rs"]
pub mod db_storage;
#[path = "db_transactions.rs"]
pub mod db_transactions;

pub use db_accounts::*;
pub use db_commodities::*;
pub use db_storage::*;
pub use db_transactions::*;

/*
use std::{collections::HashSet, path::PathBuf};

use serde::{Deserialize, Serialize};
use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use tauri::{command, State};

use crate::db::{
    db_accessible, load_storage_path, normalize_db_path, open_and_migrate, save_storage_path,
};
use crate::state::DbState;

#[derive(Serialize, Deserialize, Debug)]
pub struct Account {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub account_type: String,
    pub name: String,
    pub commodity_id: i64,
    pub institution: Option<String>,
    pub number_last4: Option<String>,
    pub is_closed: bool,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountCreate {
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub account_type: String,
    pub name: String,
    pub commodity_id: i64,
    pub institution: Option<String>,
    pub number_last4: Option<String>,
    pub is_closed: Option<bool>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountUpdate {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub account_type: String,
    pub name: String,
    pub commodity_id: i64,
    pub institution: Option<String>,
    pub number_last4: Option<String>,
    pub is_closed: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Category {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub name: String,
    pub kind: String,
    pub color: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Payee {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub metadata: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

fn map_account_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Account> {
    let is_closed: i64 = row.get(8)?;
    Ok(Account {
        id: row.get(0)?,
        book_id: row.get(1)?,
        parent_id: row.get(2)?,
        account_type: row.get(3)?,
        name: row.get(4)?,
        commodity_id: row.get(5)?,
        institution: row.get(6)?,
        number_last4: row.get(7)?,
        is_closed: is_closed != 0,
        created_at: row.get(9)?,
        updated_at: row.get(10)?,
    })
}

fn map_account_balancing_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<AccountBalancing> {
    Ok(AccountBalancing {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        as_of_date: row.get(3)?,
        balance_minor: row.get(4)?,
        memo: row.get(5)?,
        created_at: row.get(6)?,
        voided_at: row.get(7)?,
        void_reason: row.get(8)?,
    })
}
#[command]
pub fn validate_and_set_storage_location(path: String) -> Result<String, String> {
    let input = PathBuf::from(path);

    if !db_accessible(&input) {
        return Err("Storage location is not accessible for read/write.".to_string());
    }

    let (_conn, effective_db_path) =
        open_and_migrate(&input).map_err(|e| format!("Migration failed: {e}"))?;

    save_storage_path(&input);

    Ok(normalize_db_path(&effective_db_path).to_string_lossy().to_string())
}

fn map_category_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Category> {
    Ok(Category {
        id: row.get(0)?,
        book_id: row.get(1)?,
        parent_id: row.get(2)?,
        name: row.get(3)?,
        kind: row.get(4)?,
        color: row.get(5)?,
        created_at: row.get(6)?,
        updated_at: row.get(7)?,
    })
}

fn map_payee_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Payee> {
    Ok(Payee {
        id: row.get(0)?,
        book_id: row.get(1)?,
        name: row.get(2)?,
        kind: row.get(3)?,
        metadata: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
    })
}

#[command]
pub fn get_storage_location() -> Option<String> {
    load_storage_path()
        .map(|p| normalize_db_path(&p).to_string_lossy().to_string())
}

#[command]
pub fn get_db_path(db: State<DbState>) -> Option<String> {
    let guard = db.inner.lock().ok()?;
    guard
        .db_path
        .as_ref()
        .map(|p| p.to_string_lossy().to_string())
}

#[command]
pub fn db_health(db: State<DbState>) -> Result<String, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    conn.query_row("SELECT 1", [], |_row| Ok(())).map_err(|e| e.to_string())?;
    Ok("ok".to_string())
}

#[command]
pub fn get_schema_version(db: State<DbState>) -> Result<i32, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let version_opt: Option<i32> = conn
        .query_row(
            "SELECT MAX(version) FROM schema_migrations",
            [],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    Ok(version_opt.unwrap_or(0))
}

#[command]
pub fn greet(name: &str) -> String {
    format!("Hello, {}! You've been greeted from Rust!", name)
}

#[command]
pub fn create_account(db: State<DbState>, input: AccountCreate) -> Result<Account, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let is_closed = if input.is_closed.unwrap_or(false) { 1 } else { 0 };

    conn.execute(
        "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, institution, number_last4, is_closed, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            input.book_id,
            input.parent_id,
            input.account_type,
            input.name,
            input.commodity_id,
            input.institution,
            input.number_last4,
            is_closed,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let account = conn
        .query_row(
            "SELECT id, book_id, parent_id, type, name, commodity_id, institution, number_last4, is_closed, created_at, updated_at
             FROM accounts WHERE id = ?1",
            [id],
            map_account_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(account)
}

#[command]
pub fn get_account(db: State<DbState>, id: i64) -> Result<Option<Account>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, parent_id, type, name, commodity_id, institution, number_last4, is_closed, created_at, updated_at
             FROM accounts WHERE id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let mut rows = stmt.query([id]).map_err(|e| e.to_string())?;
    if let Some(row) = rows.next().map_err(|e| e.to_string())? {
        Ok(Some(map_account_row(row).map_err(|e| e.to_string())?))
    } else {
        Ok(None)
    }
}

#[command]
pub fn list_accounts(db: State<DbState>, book_id: i64) -> Result<Vec<Account>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, parent_id, type, name, commodity_id, institution, number_last4, is_closed, created_at, updated_at
             FROM accounts WHERE book_id = ?1 ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_account_row)
        .map_err(|e| e.to_string())?;

    let mut accounts = Vec::new();
    for row in rows {
        accounts.push(row.map_err(|e| e.to_string())?);
    }

    Ok(accounts)
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountBalance {
    pub account_id: i64,
    pub balance_minor: i64,
    pub native_balance_minor: i64,
    pub price_missing: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountBalancing {
    pub id: i64,
    pub book_id: i64,
    pub account_id: i64,
    pub as_of_date: String,
    pub balance_minor: i64,
    pub memo: Option<String>,
    pub created_at: String,
    pub voided_at: Option<String>,
    pub void_reason: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountBalancingCreate {
    pub book_id: i64,
    pub account_id: i64,
    pub as_of_date: String,
    pub balance_minor: i64,
    pub memo: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountBalancingUnlockInput {
    pub account_id: i64,
    pub from_date: String,
    pub reason: Option<String>,
    pub confirm: bool,
}

#[command]
pub fn list_account_balances(db: State<DbState>, book_id: i64) -> Result<Vec<AccountBalance>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let base_commodity_id: i64 = conn
        .query_row(
            "SELECT base_commodity_id FROM books WHERE id = ?1",
            [book_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    let mut price_stmt = conn
        .prepare(
            "SELECT commodity_id, price_minor, as_of_date
             FROM commodity_prices
             WHERE book_id = ?1 AND quote_commodity_id = ?2
             ORDER BY as_of_date DESC, id DESC",
        )
        .map_err(|e| e.to_string())?;

    let price_rows = price_stmt
        .query_map(params![book_id, base_commodity_id], |row| {
            Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?))
        })
        .map_err(|e| e.to_string())?;

    let mut latest_prices: std::collections::HashMap<i64, i64> = std::collections::HashMap::new();
    for row in price_rows {
        let (commodity_id, price_minor) = row.map_err(|e| e.to_string())?;
        latest_prices.entry(commodity_id).or_insert(price_minor);
    }

    let mut stmt = conn
        .prepare(
            "SELECT a.id, a.commodity_id, c.scale, COALESCE(SUM(s.amount_minor), 0)
             FROM accounts a
             JOIN commodities c ON c.id = a.commodity_id
             LEFT JOIN splits s ON s.account_id = a.id
             LEFT JOIN transactions t ON t.id = s.tx_id
             WHERE a.book_id = ?1
             GROUP BY a.id, a.commodity_id, c.scale",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, i64>(3)?,
            ))
        })
        .map_err(|e| e.to_string())?;

    let mut balances = Vec::new();
    for row in rows {
        let (account_id, commodity_id, scale, native_balance_minor) =
            row.map_err(|e| e.to_string())?;

        if commodity_id == base_commodity_id {
            balances.push(AccountBalance {
                account_id,
                balance_minor: native_balance_minor,
                native_balance_minor,
                price_missing: false,
            });
            continue;
        }

        if let Some(price_minor) = latest_prices.get(&commodity_id) {
            let scale_factor: i128 = 10i128.pow(scale as u32);
            let value_minor = (native_balance_minor as i128 * *price_minor as i128) / scale_factor;
            balances.push(AccountBalance {
                account_id,
                balance_minor: value_minor as i64,
                native_balance_minor,
                price_missing: false,
            });
        } else {
            balances.push(AccountBalance {
                account_id,
                balance_minor: 0,
                native_balance_minor,
                price_missing: true,
            });
        }
    }

    Ok(balances)
}

#[command]
pub fn create_account_balancing(
    db: State<DbState>,
    input: AccountBalancingCreate,
) -> Result<AccountBalancing, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let account_book_id: Option<i64> = tx
        .query_row(
            "SELECT book_id FROM accounts WHERE id = ?1",
            [input.account_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    match account_book_id {
        Some(book_id) if book_id == input.book_id => {}
        Some(_) => return Err("account does not belong to book".to_string()),
        None => return Err("account not found".to_string()),
    }

    let latest: Option<String> = tx
        .query_row(
            "SELECT MAX(as_of_date) FROM account_balancings WHERE account_id = ?1 AND voided_at IS NULL",
            [input.account_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if let Some(max_date) = latest {
        if input.as_of_date <= max_date {
            return Err("balancing date must be after last active balancing".to_string());
        }
    }

    tx.execute(
        "INSERT INTO account_balancings (book_id, account_id, as_of_date, balance_minor, memo, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            input.book_id,
            input.account_id,
            input.as_of_date,
            input.balance_minor,
            input.memo,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = tx.last_insert_rowid();
    let balancing = tx
        .query_row(
            "SELECT id, book_id, account_id, as_of_date, balance_minor, memo, created_at, voided_at, void_reason
             FROM account_balancings WHERE id = ?1",
            [id],
            map_account_balancing_row,
        )
        .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;
    Ok(balancing)
}

#[command]
pub fn list_account_balancings(
    db: State<DbState>,
    account_id: i64,
) -> Result<Vec<AccountBalancing>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, as_of_date, balance_minor, memo, created_at, voided_at, void_reason
             FROM account_balancings WHERE account_id = ?1
             ORDER BY as_of_date DESC, id DESC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([account_id], map_account_balancing_row)
        .map_err(|e| e.to_string())?;

    let mut balancings = Vec::new();
    for row in rows {
        balancings.push(row.map_err(|e| e.to_string())?);
    }

    Ok(balancings)
}

#[command]
pub fn unlock_account_balancings(
    db: State<DbState>,
    input: AccountBalancingUnlockInput,
) -> Result<i64, String> {
    if !input.confirm {
        return Err("unlock not confirmed".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let rows = tx
        .execute(
            "UPDATE account_balancings
             SET voided_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
                 void_reason = ?1
             WHERE account_id = ?2 AND voided_at IS NULL AND as_of_date >= ?3",
            params![input.reason, input.account_id, input.from_date],
        )
        .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;
    Ok(rows as i64)
}

#[command]
pub fn list_categories(db: State<DbState>, book_id: i64) -> Result<Vec<Category>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, parent_id, name, kind, color, created_at, updated_at
             FROM categories WHERE book_id = ?1 ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_category_row)
        .map_err(|e| e.to_string())?;

    let mut categories = Vec::new();
    for row in rows {
        categories.push(row.map_err(|e| e.to_string())?);
    }

    Ok(categories)
}

#[command]
pub fn list_payees(db: State<DbState>, book_id: i64) -> Result<Vec<Payee>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, name, kind, metadata, created_at, updated_at
             FROM payees WHERE book_id = ?1 ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_payee_row)
        .map_err(|e| e.to_string())?;

    let mut payees = Vec::new();
    for row in rows {
        payees.push(row.map_err(|e| e.to_string())?);
    }

    Ok(payees)
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
pub fn list_corporate_actions(
    db: State<DbState>,
    book_id: i64,
    commodity_id: Option<i64>,
) -> Result<Vec<CorporateAction>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    if let Some(commodity_id) = commodity_id {
        let mut stmt = conn
            .prepare(
                "SELECT id, book_id, commodity_id, kind, ratio_num, ratio_den, effective_date, memo, tx_id, created_at
                 FROM corporate_actions WHERE book_id = ?1 AND commodity_id = ?2
                 ORDER BY effective_date DESC, id DESC",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(params![book_id, commodity_id], map_corporate_action_row)
            .map_err(|e| e.to_string())?;
        let mut actions = Vec::new();
        for row in rows {
            actions.push(row.map_err(|e| e.to_string())?);
        }
        return Ok(actions);
    }

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, commodity_id, kind, ratio_num, ratio_den, effective_date, memo, tx_id, created_at
             FROM corporate_actions WHERE book_id = ?1
             ORDER BY effective_date DESC, id DESC",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([book_id], map_corporate_action_row)
        .map_err(|e| e.to_string())?;
    let mut actions = Vec::new();
    for row in rows {
        actions.push(row.map_err(|e| e.to_string())?);
    }

    Ok(actions)
}

#[command]
pub fn create_price_source(
    db: State<DbState>,
    input: PriceSourceCreate,
) -> Result<PriceSource, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute(
        "INSERT INTO price_sources (name, kind, provider, base_url, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![input.name, input.kind, input.provider, input.base_url],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let source = conn
        .query_row(
            "SELECT id, name, kind, provider, base_url, created_at, updated_at
             FROM price_sources WHERE id = ?1",
            [id],
            map_price_source_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(source)
}

#[command]
pub fn list_price_sources(db: State<DbState>) -> Result<Vec<PriceSource>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, name, kind, provider, base_url, created_at, updated_at
             FROM price_sources ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([], map_price_source_row)
        .map_err(|e| e.to_string())?;

    let mut sources = Vec::new();
    for row in rows {
        sources.push(row.map_err(|e| e.to_string())?);
    }

    Ok(sources)
}

#[command]
pub fn update_price_source(
    db: State<DbState>,
    input: PriceSourceUpdate,
) -> Result<PriceSource, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute(
            "UPDATE price_sources
             SET name = ?2, kind = ?3, provider = ?4, base_url = ?5,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![input.id, input.name, input.kind, input.provider, input.base_url],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("price source not found".to_string());
    }

    let source = conn
        .query_row(
            "SELECT id, name, kind, provider, base_url, created_at, updated_at
             FROM price_sources WHERE id = ?1",
            [input.id],
            map_price_source_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(source)
}

#[command]
pub fn delete_price_source(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let rows = conn
        .execute("DELETE FROM price_sources WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;
    Ok(rows > 0)
}

#[command]
pub fn create_commodity_price_source(
    db: State<DbState>,
    input: CommodityPriceSourceCreate,
) -> Result<CommodityPriceSource, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let is_primary = if input.is_primary { 1 } else { 0 };

    conn.execute(
        "INSERT INTO commodity_price_sources (commodity_id, source_id, symbol, name_override, is_primary, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            input.commodity_id,
            input.source_id,
            input.symbol,
            input.name_override,
            is_primary,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let mapping = conn
        .query_row(
            "SELECT id, commodity_id, source_id, symbol, name_override, is_primary, created_at, updated_at
             FROM commodity_price_sources WHERE id = ?1",
            [id],
            map_commodity_price_source_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(mapping)
}

#[command]
pub fn list_commodity_price_sources(
    db: State<DbState>,
    commodity_id: i64,
) -> Result<Vec<CommodityPriceSource>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, commodity_id, source_id, symbol, name_override, is_primary, created_at, updated_at
             FROM commodity_price_sources WHERE commodity_id = ?1
             ORDER BY is_primary DESC, symbol ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([commodity_id], map_commodity_price_source_row)
        .map_err(|e| e.to_string())?;

    let mut mappings = Vec::new();
    for row in rows {
        mappings.push(row.map_err(|e| e.to_string())?);
    }

    Ok(mappings)
}

#[command]
pub fn update_commodity_price_source(
    db: State<DbState>,
    input: CommodityPriceSourceUpdate,
) -> Result<CommodityPriceSource, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let is_primary = if input.is_primary { 1 } else { 0 };

    let rows = conn
        .execute(
            "UPDATE commodity_price_sources
             SET commodity_id = ?2, source_id = ?3, symbol = ?4, name_override = ?5, is_primary = ?6,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![
                input.id,
                input.commodity_id,
                input.source_id,
                input.symbol,
                input.name_override,
                is_primary,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("commodity price source not found".to_string());
    }

    let mapping = conn
        .query_row(
            "SELECT id, commodity_id, source_id, symbol, name_override, is_primary, created_at, updated_at
             FROM commodity_price_sources WHERE id = ?1",
            [input.id],
            map_commodity_price_source_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(mapping)
}

#[command]
pub fn delete_commodity_price_source(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let rows = conn
        .execute("DELETE FROM commodity_price_sources WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;
    Ok(rows > 0)
}

#[command]
pub fn create_commodity_price(
    db: State<DbState>,
    input: CommodityPriceCreate,
) -> Result<CommodityPrice, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let is_manual = if input.is_manual { 1 } else { 0 };

    conn.execute(
        "INSERT INTO commodity_prices (book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            input.book_id,
            input.commodity_id,
            input.quote_commodity_id,
            input.price_minor,
            input.as_of_date,
            input.source,
            input.source_id,
            is_manual,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let price = conn
        .query_row(
            "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
             FROM commodity_prices WHERE id = ?1",
            [id],
            map_commodity_price_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(price)
}

#[command]
pub fn list_commodity_prices(
    db: State<DbState>,
    book_id: i64,
    commodity_id: Option<i64>,
    quote_commodity_id: Option<i64>,
    limit: Option<i64>,
) -> Result<Vec<CommodityPrice>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let limit = limit.unwrap_or(200).max(1);
    if let (Some(commodity_id), Some(quote_id)) = (commodity_id, quote_commodity_id) {
        let mut stmt = conn
            .prepare(
                "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
                 FROM commodity_prices WHERE book_id = ?1 AND commodity_id = ?2 AND quote_commodity_id = ?3
                 ORDER BY as_of_date DESC, id DESC LIMIT ?4",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(
                params![book_id, commodity_id, quote_id, limit],
                map_commodity_price_row,
            )
            .map_err(|e| e.to_string())?;
        let mut prices = Vec::new();
        for row in rows {
            prices.push(row.map_err(|e| e.to_string())?);
        }
        return Ok(prices);
    }

    if let Some(commodity_id) = commodity_id {
        let mut stmt = conn
            .prepare(
                "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
                 FROM commodity_prices WHERE book_id = ?1 AND commodity_id = ?2
                 ORDER BY as_of_date DESC, id DESC LIMIT ?3",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(params![book_id, commodity_id, limit], map_commodity_price_row)
            .map_err(|e| e.to_string())?;
        let mut prices = Vec::new();
        for row in rows {
            prices.push(row.map_err(|e| e.to_string())?);
        }
        return Ok(prices);
    }

    if let Some(quote_id) = quote_commodity_id {
        let mut stmt = conn
            .prepare(
                "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
                 FROM commodity_prices WHERE book_id = ?1 AND quote_commodity_id = ?2
                 ORDER BY as_of_date DESC, id DESC LIMIT ?3",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(params![book_id, quote_id, limit], map_commodity_price_row)
            .map_err(|e| e.to_string())?;
        let mut prices = Vec::new();
        for row in rows {
            prices.push(row.map_err(|e| e.to_string())?);
        }
        return Ok(prices);
    }

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
             FROM commodity_prices WHERE book_id = ?1
             ORDER BY as_of_date DESC, id DESC LIMIT ?2",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params![book_id, limit], map_commodity_price_row)
        .map_err(|e| e.to_string())?;
    let mut prices = Vec::new();
    for row in rows {
        prices.push(row.map_err(|e| e.to_string())?);
    }

    Ok(prices)
}

#[command]
pub fn update_commodity_price(
    db: State<DbState>,
    input: CommodityPriceUpdate,
) -> Result<CommodityPrice, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let is_manual = if input.is_manual { 1 } else { 0 };

    let rows = conn
        .execute(
            "UPDATE commodity_prices
             SET book_id = ?2, commodity_id = ?3, quote_commodity_id = ?4, price_minor = ?5, as_of_date = ?6,
                 source = ?7, source_id = ?8, is_manual = ?9
             WHERE id = ?1",
            params![
                input.id,
                input.book_id,
                input.commodity_id,
                input.quote_commodity_id,
                input.price_minor,
                input.as_of_date,
                input.source,
                input.source_id,
                is_manual,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("commodity price not found".to_string());
    }

    let price = conn
        .query_row(
            "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
             FROM commodity_prices WHERE id = ?1",
            [input.id],
            map_commodity_price_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(price)
}

#[command]
pub fn delete_commodity_price(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let rows = conn
        .execute("DELETE FROM commodity_prices WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;
    Ok(rows > 0)
}

#[command]
pub fn apply_corporate_action_split_merge(
    db: State<DbState>,
    input: ApplyCorporateActionInput,
) -> Result<CorporateActionResult, String> {
    if input.ratio_num <= 0 || input.ratio_den <= 0 {
        return Err("ratio must be positive".to_string());
    }

    let kind = if input.ratio_num >= input.ratio_den {
        "split"
    } else {
        "merge"
    };

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let (action_id, transaction_id) = {
        let result: Result<(i64, Option<i64>), String> = (|| {
        let tx = conn.transaction().map_err(|e| e.to_string())?;

        let equity_account_id: i64 = tx
            .query_row(
                "SELECT id FROM accounts
                 WHERE book_id = ?1 AND commodity_id = ?2 AND type = 'equity' AND name = 'Corporate Actions'
                 LIMIT 1",
                params![input.book_id, input.commodity_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|e| e.to_string())?
            .unwrap_or_else(|| 0);

        let equity_account_id = if equity_account_id == 0 {
            tx.execute(
                "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, is_closed, created_at, updated_at)
                 VALUES (?1, NULL, 'equity', 'Corporate Actions', ?2, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![input.book_id, input.commodity_id],
            )
            .map_err(|e| e.to_string())?;
            tx.last_insert_rowid()
        } else {
            equity_account_id
        };

        let adjustments: Vec<(i64, i64)> = {
            let mut stmt = tx
                .prepare(
                    "SELECT a.id, COALESCE(SUM(s.amount_minor), 0) AS balance
                     FROM accounts a
                     LEFT JOIN splits s ON s.account_id = a.id
                     WHERE a.book_id = ?1 AND a.commodity_id = ?2
                     GROUP BY a.id
                     HAVING balance != 0",
                )
                .map_err(|e| e.to_string())?;

            let rows = stmt
                .query_map(params![input.book_id, input.commodity_id], |row| {
                    let account_id: i64 = row.get(0)?;
                    let balance: i64 = row.get(1)?;
                    Ok((account_id, balance))
                })
                .map_err(|e| e.to_string())?;

            let mut adjustments: Vec<(i64, i64)> = Vec::new();
            for row in rows {
                let (account_id, balance) = row.map_err(|e| e.to_string())?;
                let numerator = (balance as i128) * (input.ratio_num as i128);
                if numerator % (input.ratio_den as i128) != 0 {
                    return Err("split/merge ratio produces fractional shares".to_string());
                }
                let new_balance = (numerator / (input.ratio_den as i128)) as i64;
                let delta = new_balance - balance;
                if delta != 0 {
                    adjustments.push((account_id, delta));
                }
            }
            adjustments
        };

        let mut transaction_id: Option<i64> = None;
        if !adjustments.is_empty() {
            let memo = input
                .memo
                .clone()
                .unwrap_or_else(|| format!("Corporate action: {} {}/{}", kind, input.ratio_num, input.ratio_den));

            tx.execute(
                "INSERT INTO transactions (book_id, txn_date, memo, status, created_at, updated_at)
                 VALUES (?1, ?2, ?3, 'cleared', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![input.book_id, input.effective_date, memo],
            )
            .map_err(|e| e.to_string())?;

            let tx_id = tx.last_insert_rowid();
            transaction_id = Some(tx_id);

            let mut total_delta: i64 = 0;
            for (account_id, delta) in &adjustments {
                total_delta += *delta;
                tx.execute(
                    "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at, updated_at)
                     VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![tx_id, account_id, input.commodity_id, *delta],
                )
                .map_err(|e| e.to_string())?;
            }

            tx.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx_id, equity_account_id, input.commodity_id, -total_delta],
            )
            .map_err(|e| e.to_string())?;
        }

        tx.execute(
            "INSERT INTO corporate_actions (book_id, commodity_id, kind, ratio_num, ratio_den, effective_date, memo, tx_id)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
            params![
                input.book_id,
                input.commodity_id,
                kind,
                input.ratio_num,
                input.ratio_den,
                input.effective_date,
                input.memo,
                transaction_id,
            ],
        )
        .map_err(|e| e.to_string())?;

        let action_id = tx.last_insert_rowid();

        tx.commit().map_err(|e| e.to_string())?;

            Ok((action_id, transaction_id))
        })();
        result?
    };

    let action = conn
        .query_row(
            "SELECT id, book_id, commodity_id, kind, ratio_num, ratio_den, effective_date, memo, tx_id, created_at
             FROM corporate_actions WHERE id = ?1",
            [action_id],
            map_corporate_action_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(CorporateActionResult {
        action,
        transaction_id,
    })
}

#[command]
pub fn update_account(db: State<DbState>, input: AccountUpdate) -> Result<Account, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let is_closed = if input.is_closed { 1 } else { 0 };

    let rows = conn
        .execute(
            "UPDATE accounts
             SET book_id = ?2,
                 parent_id = ?3,
                 type = ?4,
                 name = ?5,
                 commodity_id = ?6,
                 institution = ?7,
                 number_last4 = ?8,
                 is_closed = ?9,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![
                input.id,
                input.book_id,
                input.parent_id,
                input.account_type,
                input.name,
                input.commodity_id,
                input.institution,
                input.number_last4,
                is_closed,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("account not found".to_string());
    }

    let account = conn
        .query_row(
            "SELECT id, book_id, parent_id, type, name, commodity_id, institution, number_last4, is_closed, created_at, updated_at
             FROM accounts WHERE id = ?1",
            [input.id],
            map_account_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(account)
}

#[command]
pub fn delete_account(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute("DELETE FROM accounts WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(rows > 0)
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Transaction {
    pub id: i64,
    pub book_id: i64,
    pub txn_date: String,
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
    pub memo: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CreateTransactionInput {
    pub book_id: i64,
    pub txn_date: String,
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
pub struct CorporateAction {
    pub id: i64,
    pub book_id: i64,
    pub commodity_id: i64,
    pub kind: String,
    pub ratio_num: i64,
    pub ratio_den: i64,
    pub effective_date: String,
    pub memo: Option<String>,
    pub tx_id: Option<i64>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ApplyCorporateActionInput {
    pub book_id: i64,
    pub commodity_id: i64,
    pub ratio_num: i64,
    pub ratio_den: i64,
    pub effective_date: String,
    pub memo: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CorporateActionResult {
    pub action: CorporateAction,
    pub transaction_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PriceSource {
    pub id: i64,
    pub name: String,
    pub kind: String,
    pub provider: Option<String>,
    pub base_url: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PriceSourceCreate {
    pub name: String,
    pub kind: String,
    pub provider: Option<String>,
    pub base_url: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PriceSourceUpdate {
    pub id: i64,
    pub name: String,
    pub kind: String,
    pub provider: Option<String>,
    pub base_url: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityPriceSource {
    pub id: i64,
    pub commodity_id: i64,
    pub source_id: i64,
    pub symbol: String,
    pub name_override: Option<String>,
    pub is_primary: bool,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityPriceSourceCreate {
    pub commodity_id: i64,
    pub source_id: i64,
    pub symbol: String,
    pub name_override: Option<String>,
    pub is_primary: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityPriceSourceUpdate {
    pub id: i64,
    pub commodity_id: i64,
    pub source_id: i64,
    pub symbol: String,
    pub name_override: Option<String>,
    pub is_primary: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityPrice {
    pub id: i64,
    pub book_id: i64,
    pub commodity_id: i64,
    pub quote_commodity_id: i64,
    pub price_minor: i64,
    pub as_of_date: String,
    pub source: Option<String>,
    pub source_id: Option<i64>,
    pub is_manual: bool,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityPriceCreate {
    pub book_id: i64,
    pub commodity_id: i64,
    pub quote_commodity_id: i64,
    pub price_minor: i64,
    pub as_of_date: String,
    pub source: Option<String>,
    pub source_id: Option<i64>,
    pub is_manual: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityPriceUpdate {
    pub id: i64,
    pub book_id: i64,
    pub commodity_id: i64,
    pub quote_commodity_id: i64,
    pub price_minor: i64,
    pub as_of_date: String,
    pub source: Option<String>,
    pub source_id: Option<i64>,
    pub is_manual: bool,
}

fn map_transaction_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Transaction> {
    Ok(Transaction {
        id: row.get(0)?,
        book_id: row.get(1)?,
        txn_date: row.get(2)?,
        payee_id: row.get(3)?,
        memo: row.get(4)?,
        status: row.get(5)?,
        reference: row.get(6)?,
        import_id: row.get(7)?,
        created_at: row.get(8)?,
        updated_at: row.get(9)?,
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
        memo: row.get(6)?,
        created_at: row.get(7)?,
        updated_at: row.get(8)?,
    })
}

fn fetch_transaction_with_splits(
    conn: &rusqlite::Connection,
    id: i64,
) -> Result<TransactionWithSplits, String> {
    let transaction = conn
        .query_row(
            "SELECT id, book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at
             FROM transactions WHERE id = ?1",
            [id],
            map_transaction_row,
        )
        .map_err(|e| e.to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at
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

#[command]
pub fn list_transactions(
    db: State<DbState>,
    filter: ListTransactionsFilter,
) -> Result<Vec<TransactionWithSplits>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let limit = filter.limit.unwrap_or(200).max(1);
    let offset = filter.offset.unwrap_or(0).max(0);

    let mut sql = String::from(
        "SELECT DISTINCT t.id
         FROM transactions t
         LEFT JOIN splits s ON s.tx_id = t.id
         LEFT JOIN payees p ON p.id = t.payee_id
         WHERE t.book_id = ?",
    );

    let mut params: Vec<Value> = vec![Value::from(filter.book_id)];

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

    let existing_book: Option<i64> = tx
        .query_row(
            "SELECT book_id FROM transactions WHERE id = ?1",
            [input.id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    match existing_book {
        Some(book_id) if book_id == input.book_id => {}
        Some(_) => return Err("transaction does not belong to book".to_string()),
        None => return Err("transaction not found".to_string()),
    }

    let mut accounts = HashSet::new();
    for split in &input.splits {
        accounts.insert(split.account_id);
    }
    check_account_locks(&tx, &input.txn_date, &accounts)?;

    let status = input.status.unwrap_or_else(|| "uncleared".to_string());

    tx.execute(
        "UPDATE transactions
         SET txn_date = ?2,
             payee_id = ?3,
             memo = ?4,
             status = ?5,
             reference = ?6,
             import_id = ?7,
             updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
         WHERE id = ?1",
        params![
            input.id,
            input.txn_date,
            input.payee_id,
            input.memo,
            status,
            input.reference,
            input.import_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    tx.execute("DELETE FROM splits WHERE tx_id = ?1", [input.id])
        .map_err(|e| e.to_string())?;

    for split in &input.splits {
        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                input.id,
                split.account_id,
                split.commodity_id,
                split.amount_minor,
                split.category_id,
                split.memo,
            ],
        )
        .map_err(|e| e.to_string())?;
    }

    tx.commit().map_err(|e| e.to_string())?;
    fetch_transaction_with_splits(conn, input.id)
}

#[command]
pub fn delete_transaction(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let txn_date: Option<String> = tx
        .query_row(
            "SELECT txn_date FROM transactions WHERE id = ?1",
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

    let rows = tx
        .execute("DELETE FROM transactions WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;
    Ok(rows > 0)
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
                "SELECT as_of_date FROM account_balancings
                 WHERE account_id = ?1 AND voided_at IS NULL AND as_of_date >= ?2
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

    let mut locked = Vec::new();
    let mut seen = HashSet::new();
    for split in &input.splits {
        if !seen.insert(split.account_id) {
            continue;
        }

        let lock_date: Option<String> = tx
            .query_row(
                "SELECT as_of_date FROM account_balancings
                 WHERE account_id = ?1 AND voided_at IS NULL AND as_of_date >= ?2
                 ORDER BY as_of_date ASC LIMIT 1",
                params![split.account_id, input.txn_date],
                |row| row.get(0),
            )
            .optional()
            .map_err(|e| e.to_string())?;

        if let Some(date) = lock_date {
            locked.push((split.account_id, date));
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
            input.txn_date, details
        ));
    }

    let status = input.status.unwrap_or_else(|| "uncleared".to_string());

    tx.execute(
        "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            input.book_id,
            input.txn_date,
            input.payee_id,
            input.memo,
            status,
            input.reference,
            input.import_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let tx_id = tx.last_insert_rowid();

    for split in &input.splits {
        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                tx_id,
                split.account_id,
                split.commodity_id,
                split.amount_minor,
                split.category_id,
                split.memo,
            ],
        )
        .map_err(|e| e.to_string())?;
    }

    tx.commit().map_err(|e| e.to_string())?;

    let transaction = conn
        .query_row(
            "SELECT id, book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at
             FROM transactions WHERE id = ?1",
            [tx_id],
            map_transaction_row,
        )
        .map_err(|e| e.to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at
             FROM splits WHERE tx_id = ?1 ORDER BY id ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([tx_id], map_split_row)
        .map_err(|e| e.to_string())?;

    let mut splits = Vec::new();
    for row in rows {
        splits.push(row.map_err(|e| e.to_string())?);
    }

    Ok(TransactionWithSplits { transaction, splits })
}

#[command]
pub fn get_transaction_with_splits(
    db: State<DbState>,
    id: i64,
) -> Result<Option<TransactionWithSplits>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut tx_stmt = conn
        .prepare(
            "SELECT id, book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at
             FROM transactions WHERE id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let mut tx_rows = tx_stmt.query([id]).map_err(|e| e.to_string())?;
    let transaction = if let Some(row) = tx_rows.next().map_err(|e| e.to_string())? {
        map_transaction_row(row).map_err(|e| e.to_string())?
    } else {
        return Ok(None);
    };

    let mut split_stmt = conn
        .prepare(
            "SELECT id, tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at
             FROM splits WHERE tx_id = ?1 ORDER BY id ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = split_stmt
        .query_map([id], map_split_row)
        .map_err(|e| e.to_string())?;

    let mut splits = Vec::new();
    for row in rows {
        splits.push(row.map_err(|e| e.to_string())?);
    }

    Ok(Some(TransactionWithSplits { transaction, splits }))
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

fn map_corporate_action_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<CorporateAction> {
    Ok(CorporateAction {
        id: row.get(0)?,
        book_id: row.get(1)?,
        commodity_id: row.get(2)?,
        kind: row.get(3)?,
        ratio_num: row.get(4)?,
        ratio_den: row.get(5)?,
        effective_date: row.get(6)?,
        memo: row.get(7)?,
        tx_id: row.get(8)?,
        created_at: row.get(9)?,
    })
}

fn map_price_source_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<PriceSource> {
    Ok(PriceSource {
        id: row.get(0)?,
        name: row.get(1)?,
        kind: row.get(2)?,
        provider: row.get(3)?,
        base_url: row.get(4)?,
        created_at: row.get(5)?,
        updated_at: row.get(6)?,
    })
}

fn map_commodity_price_source_row(
    row: &rusqlite::Row<'_>,
) -> rusqlite::Result<CommodityPriceSource> {
    let is_primary: i64 = row.get(5)?;
    Ok(CommodityPriceSource {
        id: row.get(0)?,
        commodity_id: row.get(1)?,
        source_id: row.get(2)?,
        symbol: row.get(3)?,
        name_override: row.get(4)?,
        is_primary: is_primary != 0,
        created_at: row.get(6)?,
        updated_at: row.get(7)?,
    })
}

fn map_commodity_price_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<CommodityPrice> {
    let is_manual: i64 = row.get(8)?;
    Ok(CommodityPrice {
        id: row.get(0)?,
        book_id: row.get(1)?,
        commodity_id: row.get(2)?,
        quote_commodity_id: row.get(3)?,
        price_minor: row.get(4)?,
        as_of_date: row.get(5)?,
        source: row.get(6)?,
        source_id: row.get(7)?,
        is_manual: is_manual != 0,
        created_at: row.get(9)?,
    })
}
*/
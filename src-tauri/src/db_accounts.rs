use std::collections::HashMap;

use rusqlite::{params, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

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

    let mut latest_prices: HashMap<i64, i64> = HashMap::new();
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

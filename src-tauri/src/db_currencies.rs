use rusqlite::{params, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::state::DbState;

const SINGLE_BOOK_ID: i64 = 1;

// ============================================================================
// Currency Types
// ============================================================================

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct Currency {
    pub id: i64,
    pub book_id: i64,
    pub symbol: Option<String>,
    pub display_symbol: Option<String>,
    pub name: String,
    pub scale: i64,
    pub is_active: bool,
    pub is_default: bool,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CurrencyCreate {
    pub book_id: i64,
    pub symbol: String,
    pub display_symbol: Option<String>,
    pub name: String,
    pub scale: Option<i64>,
    pub is_active: Option<bool>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CurrencyUpdate {
    pub id: i64,
    pub symbol: Option<String>,
    pub display_symbol: Option<String>,
    pub name: Option<String>,
    pub is_active: Option<bool>,
}

// ============================================================================
// FX Rate Types
// ============================================================================

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct FxRateDaily {
    pub id: i64,
    pub book_id: i64,
    pub from_currency_id: i64,
    pub from_currency_symbol: Option<String>,
    pub to_currency_id: i64,
    pub to_currency_symbol: Option<String>,
    pub rate_date: String,
    pub rate: f64,
    pub source: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateDailyCreate {
    pub book_id: i64,
    pub from_currency_id: i64,
    pub to_currency_id: i64,
    pub rate_date: String,
    pub rate: f64,
    pub source: Option<String>,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct FxRateOfficial {
    pub id: i64,
    pub book_id: i64,
    pub from_currency_id: i64,
    pub from_currency_symbol: Option<String>,
    pub to_currency_id: i64,
    pub to_currency_symbol: Option<String>,
    pub period_type: String,
    pub period_year: i64,
    pub period_month: Option<i64>,
    pub rate: f64,
    pub source_name: String,
    pub source_url: Option<String>,
    pub source_date: Option<String>,
    pub notes: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateOfficialCreate {
    pub book_id: i64,
    pub from_currency_id: i64,
    pub to_currency_id: i64,
    pub period_type: String,
    pub period_year: i64,
    pub period_month: Option<i64>,
    pub rate: f64,
    pub source_name: String,
    pub source_url: Option<String>,
    pub source_date: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateOfficialUpdate {
    pub id: i64,
    pub rate: Option<f64>,
    pub source_name: Option<String>,
    pub source_url: Option<String>,
    pub source_date: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct FxRateSource {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub country_code: Option<String>,
    pub website_url: Option<String>,
    pub notes: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateSourceCreate {
    pub book_id: i64,
    pub name: String,
    pub country_code: Option<String>,
    pub website_url: Option<String>,
    pub notes: Option<String>,
}

// ============================================================================
// Row Mappers
// ============================================================================

fn map_currency_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Currency> {
    Ok(Currency {
        id: row.get(0)?,
        book_id: row.get(1)?,
        symbol: row.get(2)?,
        display_symbol: row.get(3)?,
        name: row.get(4)?,
        scale: row.get(5)?,
        is_active: row.get::<_, i64>(6)? != 0,
        is_default: row.get::<_, i64>(7)? != 0,
        created_at: row.get(8)?,
        updated_at: row.get(9)?,
    })
}

fn map_fx_rate_daily_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<FxRateDaily> {
    Ok(FxRateDaily {
        id: row.get(0)?,
        book_id: row.get(1)?,
        from_currency_id: row.get(2)?,
        from_currency_symbol: row.get(3)?,
        to_currency_id: row.get(4)?,
        to_currency_symbol: row.get(5)?,
        rate_date: row.get(6)?,
        rate: row.get(7)?,
        source: row.get(8)?,
        created_at: row.get(9)?,
    })
}

fn map_fx_rate_official_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<FxRateOfficial> {
    Ok(FxRateOfficial {
        id: row.get(0)?,
        book_id: row.get(1)?,
        from_currency_id: row.get(2)?,
        from_currency_symbol: row.get(3)?,
        to_currency_id: row.get(4)?,
        to_currency_symbol: row.get(5)?,
        period_type: row.get(6)?,
        period_year: row.get(7)?,
        period_month: row.get(8)?,
        rate: row.get(9)?,
        source_name: row.get(10)?,
        source_url: row.get(11)?,
        source_date: row.get(12)?,
        notes: row.get(13)?,
        created_at: row.get(14)?,
        updated_at: row.get(15)?,
    })
}

fn map_fx_rate_source_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<FxRateSource> {
    Ok(FxRateSource {
        id: row.get(0)?,
        book_id: row.get(1)?,
        name: row.get(2)?,
        country_code: row.get(3)?,
        website_url: row.get(4)?,
        notes: row.get(5)?,
        created_at: row.get(6)?,
        updated_at: row.get(7)?,
    })
}

// ============================================================================
// Currency Commands
// ============================================================================

#[command]
pub fn list_currencies(db: State<DbState>, book_id: i64, active_only: Option<bool>) -> Result<Vec<Currency>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let sql = if active_only.unwrap_or(false) {
        "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
         FROM commodities WHERE book_id = ?1 AND kind = 'currency' AND is_active = 1 ORDER BY is_default DESC, symbol ASC"
    } else {
        "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
         FROM commodities WHERE book_id = ?1 AND kind = 'currency' ORDER BY is_default DESC, is_active DESC, symbol ASC"
    };

    let mut stmt = conn.prepare(sql).map_err(|e| e.to_string())?;
    let rows = stmt.query_map([book_id], map_currency_row).map_err(|e| e.to_string())?;

    let mut currencies = Vec::new();
    for row in rows {
        currencies.push(row.map_err(|e| e.to_string())?);
    }
    Ok(currencies)
}

#[command]
pub fn get_default_currency(db: State<DbState>, book_id: i64) -> Result<Option<Currency>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
             FROM commodities WHERE book_id = ?1 AND kind = 'currency' AND is_default = 1 LIMIT 1",
        )
        .map_err(|e| e.to_string())?;

    let mut rows = stmt.query([book_id]).map_err(|e| e.to_string())?;
    if let Some(row) = rows.next().map_err(|e| e.to_string())? {
        Ok(Some(map_currency_row(row).map_err(|e| e.to_string())?))
    } else {
        Ok(None)
    }
}

#[command]
pub fn create_currency(db: State<DbState>, input: CurrencyCreate) -> Result<Currency, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let book_id = SINGLE_BOOK_ID;

    let scale = input.scale.unwrap_or(2);
    let is_active = if input.is_active.unwrap_or(true) { 1 } else { 0 };

    conn.execute(
        "INSERT INTO commodities (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at)
         VALUES (?1, 'currency', ?2, ?3, ?4, ?5, ?6, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.symbol, input.display_symbol, input.name, scale, is_active],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
             FROM commodities WHERE id = ?1",
            [id],
            map_currency_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(currency)
}

#[command]
pub fn update_currency(db: State<DbState>, input: CurrencyUpdate) -> Result<Currency, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let mut updates = vec!["updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![];

    if let Some(symbol) = &input.symbol {
        params_vec.push(Box::new(symbol.clone()));
        updates.push(format!("symbol = ?{}", params_vec.len()));
    }
    if let Some(display_symbol) = &input.display_symbol {
        params_vec.push(Box::new(display_symbol.clone()));
        updates.push(format!("display_symbol = ?{}", params_vec.len()));
    }
    if let Some(name) = &input.name {
        params_vec.push(Box::new(name.clone()));
        updates.push(format!("name = ?{}", params_vec.len()));
    }
    if let Some(is_active) = input.is_active {
        params_vec.push(Box::new(if is_active { 1i64 } else { 0i64 }));
        updates.push(format!("is_active = ?{}", params_vec.len()));
    }

    params_vec.push(Box::new(input.id));
    let sql = format!(
        "UPDATE commodities SET {} WHERE id = ?{} AND kind = 'currency'",
        updates.join(", "),
        params_vec.len()
    );

    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    conn.execute(&sql, params_refs.as_slice()).map_err(|e| e.to_string())?;

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
             FROM commodities WHERE id = ?1",
            [input.id],
            map_currency_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(currency)
}

#[command]
pub fn set_default_currency(db: State<DbState>, book_id: i64, currency_id: i64) -> Result<Currency, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    // Clear existing default
    conn.execute(
        "UPDATE commodities SET is_default = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
         WHERE book_id = ?1 AND kind = 'currency' AND is_default = 1",
        [book_id],
    )
    .map_err(|e| e.to_string())?;

    // Set new default (also ensure it's active)
    conn.execute(
        "UPDATE commodities SET is_default = 1, is_active = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
         WHERE id = ?1 AND kind = 'currency'",
        [currency_id],
    )
    .map_err(|e| e.to_string())?;

    // Also update book's base_commodity_id
    conn.execute(
        "UPDATE books SET base_commodity_id = ?1 WHERE id = ?2",
        [currency_id, book_id],
    )
    .map_err(|e| e.to_string())?;

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
             FROM commodities WHERE id = ?1",
            [currency_id],
            map_currency_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(currency)
}

#[command]
pub fn toggle_currency_active(db: State<DbState>, currency_id: i64) -> Result<Currency, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    // Check if it's the default - can't deactivate default
    let is_default: i64 = conn
        .query_row(
            "SELECT is_default FROM commodities WHERE id = ?1",
            [currency_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    if is_default != 0 {
        return Err("Cannot deactivate the default currency".to_string());
    }

    conn.execute(
        "UPDATE commodities SET is_active = 1 - is_active, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
         WHERE id = ?1 AND kind = 'currency'",
        [currency_id],
    )
    .map_err(|e| e.to_string())?;

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
             FROM commodities WHERE id = ?1",
            [currency_id],
            map_currency_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(currency)
}

// ============================================================================
// FX Daily Rate Commands
// ============================================================================

#[command]
pub fn list_fx_rates_daily(
    db: State<DbState>,
    book_id: i64,
    from_currency_id: Option<i64>,
    to_currency_id: Option<i64>,
    start_date: Option<String>,
    end_date: Option<String>,
    limit: Option<i64>,
) -> Result<Vec<FxRateDaily>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut conditions = vec!["r.book_id = ?1".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![Box::new(book_id)];

    if let Some(from_id) = from_currency_id {
        params_vec.push(Box::new(from_id));
        conditions.push(format!("r.from_currency_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("r.to_currency_id = ?{}", params_vec.len()));
    }
    if let Some(start) = start_date {
        params_vec.push(Box::new(start));
        conditions.push(format!("r.rate_date >= ?{}", params_vec.len()));
    }
    if let Some(end) = end_date {
        params_vec.push(Box::new(end));
        conditions.push(format!("r.rate_date <= ?{}", params_vec.len()));
    }

    let limit_clause = if let Some(l) = limit {
        format!(" LIMIT {}", l)
    } else {
        " LIMIT 1000".to_string()
    };

    let sql = format!(
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.rate_date, r.rate, r.source, r.created_at
         FROM fx_rates_daily r
         LEFT JOIN commodities fc ON r.from_currency_id = fc.id
         LEFT JOIN commodities tc ON r.to_currency_id = tc.id
         WHERE {}
         ORDER BY r.rate_date DESC{}",
        conditions.join(" AND "),
        limit_clause
    );

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    let rows = stmt.query_map(params_refs.as_slice(), map_fx_rate_daily_row).map_err(|e| e.to_string())?;

    let mut rates = Vec::new();
    for row in rows {
        rates.push(row.map_err(|e| e.to_string())?);
    }
    Ok(rates)
}

#[command]
pub fn create_fx_rate_daily(db: State<DbState>, input: FxRateDailyCreate) -> Result<FxRateDaily, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT OR REPLACE INTO fx_rates_daily (book_id, from_currency_id, to_currency_id, rate_date, rate, source, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.from_currency_id, input.to_currency_id, input.rate_date, input.rate, input.source],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let rate = conn
        .query_row(
            "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                    r.rate_date, r.rate, r.source, r.created_at
             FROM fx_rates_daily r
             LEFT JOIN commodities fc ON r.from_currency_id = fc.id
             LEFT JOIN commodities tc ON r.to_currency_id = tc.id
             WHERE r.id = ?1",
            [id],
            map_fx_rate_daily_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(rate)
}

#[command]
pub fn delete_fx_rate_daily(db: State<DbState>, id: i64) -> Result<(), String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute("DELETE FROM fx_rates_daily WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(())
}

#[command]
pub fn get_fx_rate_for_date(
    db: State<DbState>,
    book_id: i64,
    from_currency_id: i64,
    to_currency_id: i64,
    date: String,
) -> Result<Option<f64>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    // First try exact date, then find closest earlier date
    let rate: Option<f64> = conn
        .query_row(
            "SELECT rate FROM fx_rates_daily
             WHERE book_id = ?1 AND from_currency_id = ?2 AND to_currency_id = ?3 AND rate_date <= ?4
             ORDER BY rate_date DESC LIMIT 1",
            params![book_id, from_currency_id, to_currency_id, date],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(rate)
}

// ============================================================================
// FX Official Rate Commands
// ============================================================================

#[command]
pub fn list_fx_rates_official(
    db: State<DbState>,
    book_id: i64,
    from_currency_id: Option<i64>,
    to_currency_id: Option<i64>,
    period_type: Option<String>,
    period_year: Option<i64>,
) -> Result<Vec<FxRateOfficial>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut conditions = vec!["r.book_id = ?1".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![Box::new(book_id)];

    if let Some(from_id) = from_currency_id {
        params_vec.push(Box::new(from_id));
        conditions.push(format!("r.from_currency_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("r.to_currency_id = ?{}", params_vec.len()));
    }
    if let Some(pt) = period_type {
        params_vec.push(Box::new(pt));
        conditions.push(format!("r.period_type = ?{}", params_vec.len()));
    }
    if let Some(py) = period_year {
        params_vec.push(Box::new(py));
        conditions.push(format!("r.period_year = ?{}", params_vec.len()));
    }

    let sql = format!(
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.period_type, r.period_year, r.period_month, r.rate,
                r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
         FROM fx_rates_official r
         LEFT JOIN commodities fc ON r.from_currency_id = fc.id
         LEFT JOIN commodities tc ON r.to_currency_id = tc.id
         WHERE {}
         ORDER BY r.period_year DESC, r.period_month DESC NULLS LAST",
        conditions.join(" AND ")
    );

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    let rows = stmt.query_map(params_refs.as_slice(), map_fx_rate_official_row).map_err(|e| e.to_string())?;

    let mut rates = Vec::new();
    for row in rows {
        rates.push(row.map_err(|e| e.to_string())?);
    }
    Ok(rates)
}

#[command]
pub fn create_fx_rate_official(db: State<DbState>, input: FxRateOfficialCreate) -> Result<FxRateOfficial, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT OR REPLACE INTO fx_rates_official
         (book_id, from_currency_id, to_currency_id, period_type, period_year, period_month, rate,
          source_name, source_url, source_date, notes, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11,
                 strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.from_currency_id,
            input.to_currency_id,
            input.period_type,
            input.period_year,
            input.period_month,
            input.rate,
            input.source_name,
            input.source_url,
            input.source_date,
            input.notes
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let rate = conn
        .query_row(
            "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                    r.period_type, r.period_year, r.period_month, r.rate,
                    r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
             FROM fx_rates_official r
             LEFT JOIN commodities fc ON r.from_currency_id = fc.id
             LEFT JOIN commodities tc ON r.to_currency_id = tc.id
             WHERE r.id = ?1",
            [id],
            map_fx_rate_official_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(rate)
}

#[command]
pub fn update_fx_rate_official(db: State<DbState>, input: FxRateOfficialUpdate) -> Result<FxRateOfficial, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let mut updates = vec!["updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![];

    if let Some(rate) = input.rate {
        params_vec.push(Box::new(rate));
        updates.push(format!("rate = ?{}", params_vec.len()));
    }
    if let Some(source_name) = &input.source_name {
        params_vec.push(Box::new(source_name.clone()));
        updates.push(format!("source_name = ?{}", params_vec.len()));
    }
    if let Some(source_url) = &input.source_url {
        params_vec.push(Box::new(source_url.clone()));
        updates.push(format!("source_url = ?{}", params_vec.len()));
    }
    if let Some(source_date) = &input.source_date {
        params_vec.push(Box::new(source_date.clone()));
        updates.push(format!("source_date = ?{}", params_vec.len()));
    }
    if let Some(notes) = &input.notes {
        params_vec.push(Box::new(notes.clone()));
        updates.push(format!("notes = ?{}", params_vec.len()));
    }

    params_vec.push(Box::new(input.id));
    let sql = format!(
        "UPDATE fx_rates_official SET {} WHERE id = ?{}",
        updates.join(", "),
        params_vec.len()
    );

    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    conn.execute(&sql, params_refs.as_slice()).map_err(|e| e.to_string())?;

    let rate = conn
        .query_row(
            "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                    r.period_type, r.period_year, r.period_month, r.rate,
                    r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
             FROM fx_rates_official r
             LEFT JOIN commodities fc ON r.from_currency_id = fc.id
             LEFT JOIN commodities tc ON r.to_currency_id = tc.id
             WHERE r.id = ?1",
            [input.id],
            map_fx_rate_official_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(rate)
}

#[command]
pub fn delete_fx_rate_official(db: State<DbState>, id: i64) -> Result<(), String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute("DELETE FROM fx_rates_official WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(())
}

#[command]
pub fn get_official_rate_for_period(
    db: State<DbState>,
    book_id: i64,
    from_currency_id: i64,
    to_currency_id: i64,
    period_type: String,
    period_year: i64,
    period_month: Option<i64>,
) -> Result<Option<FxRateOfficial>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let sql = if period_month.is_some() {
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.period_type, r.period_year, r.period_month, r.rate,
                r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
         FROM fx_rates_official r
         LEFT JOIN commodities fc ON r.from_currency_id = fc.id
         LEFT JOIN commodities tc ON r.to_currency_id = tc.id
         WHERE r.book_id = ?1 AND r.from_currency_id = ?2 AND r.to_currency_id = ?3
           AND r.period_type = ?4 AND r.period_year = ?5 AND r.period_month = ?6
         LIMIT 1"
    } else {
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.period_type, r.period_year, r.period_month, r.rate,
                r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
         FROM fx_rates_official r
         LEFT JOIN commodities fc ON r.from_currency_id = fc.id
         LEFT JOIN commodities tc ON r.to_currency_id = tc.id
         WHERE r.book_id = ?1 AND r.from_currency_id = ?2 AND r.to_currency_id = ?3
           AND r.period_type = ?4 AND r.period_year = ?5 AND r.period_month IS NULL
         LIMIT 1"
    };

    let result = if let Some(month) = period_month {
        conn.query_row(
            sql,
            params![book_id, from_currency_id, to_currency_id, period_type, period_year, month],
            map_fx_rate_official_row,
        )
    } else {
        conn.query_row(
            sql,
            params![book_id, from_currency_id, to_currency_id, period_type, period_year],
            map_fx_rate_official_row,
        )
    };

    match result {
        Ok(rate) => Ok(Some(rate)),
        Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
        Err(e) => Err(e.to_string()),
    }
}

// ============================================================================
// FX Rate Source Commands
// ============================================================================

#[command]
pub fn list_fx_rate_sources(db: State<DbState>, book_id: i64) -> Result<Vec<FxRateSource>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, name, country_code, website_url, notes, created_at, updated_at
             FROM fx_rate_sources WHERE book_id = ?1 ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt.query_map([book_id], map_fx_rate_source_row).map_err(|e| e.to_string())?;

    let mut sources = Vec::new();
    for row in rows {
        sources.push(row.map_err(|e| e.to_string())?);
    }
    Ok(sources)
}

#[command]
pub fn create_fx_rate_source(db: State<DbState>, input: FxRateSourceCreate) -> Result<FxRateSource, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO fx_rate_sources (book_id, name, country_code, website_url, notes, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.name, input.country_code, input.website_url, input.notes],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let source = conn
        .query_row(
            "SELECT id, book_id, name, country_code, website_url, notes, created_at, updated_at
             FROM fx_rate_sources WHERE id = ?1",
            [id],
            map_fx_rate_source_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(source)
}

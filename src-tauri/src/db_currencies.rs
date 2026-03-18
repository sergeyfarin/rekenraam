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
    pub source_id: Option<i64>,
    pub is_derived: bool,
    pub derived_via_currency_id: Option<i64>,
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
    pub source_id: Option<i64>,
    pub is_derived: Option<bool>,
    pub derived_via_currency_id: Option<i64>,
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

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct FxRateSettings {
    pub book_id: i64,
    pub base_currency_id: i64,
    pub base_currency_symbol: Option<String>,
    pub default_source_id: Option<i64>,
    pub default_source_name: Option<String>,
    pub refresh_enabled: bool,
    pub refresh_hour_utc: i64,
    pub refresh_minute_utc: i64,
    pub max_backfill_days: i64,
    pub weekend_policy: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateSettingsUpdate {
    pub base_currency_id: Option<i64>,
    pub default_source_id: Option<i64>,
    pub refresh_enabled: Option<bool>,
    pub refresh_hour_utc: Option<i64>,
    pub refresh_minute_utc: Option<i64>,
    pub max_backfill_days: Option<i64>,
    pub weekend_policy: Option<String>,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct FxRateSourceAssignment {
    pub id: i64,
    pub book_id: i64,
    pub from_currency_id: i64,
    pub from_currency_symbol: Option<String>,
    pub to_currency_id: i64,
    pub to_currency_symbol: Option<String>,
    pub source_id: i64,
    pub source_name: Option<String>,
    pub effective_from: String,
    pub effective_to: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateSourceAssignmentCreate {
    pub from_currency_id: i64,
    pub to_currency_id: i64,
    pub source_id: i64,
    pub effective_from: String,
    pub effective_to: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateSourceAssignmentUpdate {
    pub id: i64,
    pub source_id: Option<i64>,
    pub effective_from: Option<String>,
    pub effective_to: Option<String>,
}

#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct FxRateRefreshState {
    pub id: i64,
    pub book_id: i64,
    pub from_currency_id: i64,
    pub from_currency_symbol: Option<String>,
    pub to_currency_id: i64,
    pub to_currency_symbol: Option<String>,
    pub source_id: i64,
    pub source_name: Option<String>,
    pub last_success_date: Option<String>,
    pub last_attempt_at: Option<String>,
    pub last_error: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FxRateRefreshStateUpsert {
    pub from_currency_id: i64,
    pub to_currency_id: i64,
    pub source_id: i64,
    pub last_success_date: Option<String>,
    pub last_attempt_at: Option<String>,
    pub last_error: Option<String>,
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
        source_id: row.get(9)?,
        is_derived: row.get::<_, i64>(10)? != 0,
        derived_via_currency_id: row.get(11)?,
        created_at: row.get(12)?,
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

fn map_fx_rate_settings_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<FxRateSettings> {
    Ok(FxRateSettings {
        book_id: row.get(0)?,
        base_currency_id: row.get(1)?,
        base_currency_symbol: row.get(2)?,
        default_source_id: row.get(3)?,
        default_source_name: row.get(4)?,
        refresh_enabled: row.get::<_, i64>(5)? != 0,
        refresh_hour_utc: row.get(6)?,
        refresh_minute_utc: row.get(7)?,
        max_backfill_days: row.get(8)?,
        weekend_policy: row.get(9)?,
        created_at: row.get(10)?,
        updated_at: row.get(11)?,
    })
}

fn map_fx_rate_source_assignment_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<FxRateSourceAssignment> {
    Ok(FxRateSourceAssignment {
        id: row.get(0)?,
        book_id: row.get(1)?,
        from_currency_id: row.get(2)?,
        from_currency_symbol: row.get(3)?,
        to_currency_id: row.get(4)?,
        to_currency_symbol: row.get(5)?,
        source_id: row.get(6)?,
        source_name: row.get(7)?,
        effective_from: row.get(8)?,
        effective_to: row.get(9)?,
        created_at: row.get(10)?,
        updated_at: row.get(11)?,
    })
}

fn map_fx_rate_refresh_state_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<FxRateRefreshState> {
    Ok(FxRateRefreshState {
        id: row.get(0)?,
        book_id: row.get(1)?,
        from_currency_id: row.get(2)?,
        from_currency_symbol: row.get(3)?,
        to_currency_id: row.get(4)?,
        to_currency_symbol: row.get(5)?,
        source_id: row.get(6)?,
        source_name: row.get(7)?,
        last_success_date: row.get(8)?,
        last_attempt_at: row.get(9)?,
        last_error: row.get(10)?,
        created_at: row.get(11)?,
        updated_at: row.get(12)?,
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
        "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
                 FROM current_commodities c
                 WHERE c.book_id = ?1
                     AND c.kind = 'currency'
                     AND c.is_active = 1
                 ORDER BY c.is_default DESC, c.symbol ASC"
    } else {
        "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
                 FROM current_commodities c
                 WHERE c.book_id = ?1
                     AND c.kind = 'currency'
                 ORDER BY c.is_default DESC, c.is_active DESC, c.symbol ASC"
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
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
                         FROM current_commodities c
                         WHERE c.book_id = ?1
                             AND c.kind = 'currency'
                             AND c.is_default = 1
                         LIMIT 1",
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

        let duplicate_exists: Option<i64> = conn
                .query_row(
                        "SELECT 1 FROM current_commodities c
                         WHERE c.book_id = ?1
                             AND c.kind = 'currency'
                             AND c.symbol = ?2
                         LIMIT 1",
                        params![book_id, input.symbol],
                        |row| row.get(0),
                )
                .optional()
                .map_err(|e| e.to_string())?;
        if duplicate_exists.is_some() {
                return Err("currency with this symbol already exists".to_string());
        }

    conn.execute(
        "INSERT INTO commodities (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, created_at)
         VALUES (?1, 'currency', ?2, ?3, ?4, ?5, ?6, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.symbol, input.display_symbol, input.name, scale, is_active],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
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

    let source = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default
                         FROM current_commodities c
             WHERE c.id = ?1
                             AND c.kind = 'currency'",
            [input.id],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, Option<String>>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, i64>(5)?,
                    row.get::<_, i64>(6)?,
                    row.get::<_, i64>(7)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let (source_id, book_id, source_symbol, source_display_symbol, source_name, source_scale, source_is_active, source_is_default) =
        source.ok_or_else(|| "currency not found or not current".to_string())?;

    let new_symbol = input.symbol.or(source_symbol);
    let new_display_symbol = input.display_symbol.or(source_display_symbol);
    let new_name = input.name.unwrap_or(source_name);
    let new_is_active = input
        .is_active
        .map(|value| if value { 1i64 } else { 0i64 })
        .unwrap_or(source_is_active);

    conn.execute(
        "INSERT INTO commodities
          (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at)
         VALUES
          (?1, 'currency', ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            new_symbol,
            new_display_symbol,
            new_name,
            source_scale,
            new_is_active,
            source_is_default,
            source_id
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
             FROM commodities WHERE id = ?1",
            [new_id],
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

    let target = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default
                         FROM current_commodities c
             WHERE c.id = ?1
                             AND c.kind = 'currency'",
            [currency_id],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, Option<String>>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, i64>(5)?,
                    row.get::<_, i64>(6)?,
                    row.get::<_, i64>(7)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?
        .ok_or_else(|| "currency not found or not current".to_string())?;

    let current_default = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default
                         FROM current_commodities c
             WHERE c.book_id = ?1
               AND c.kind = 'currency'
               AND c.is_default = 1
             LIMIT 1",
            [book_id],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, Option<String>>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, i64>(5)?,
                    row.get::<_, i64>(6)?,
                    row.get::<_, i64>(7)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if let Some((default_id, default_book_id, default_symbol, default_display_symbol, default_name, default_scale, default_is_active, _)) = current_default.clone() {
        if default_id != currency_id {
            conn.execute(
                "INSERT INTO commodities
                  (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at)
                 VALUES
                  (?1, 'currency', ?2, ?3, ?4, ?5, ?6, 0, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![
                    default_book_id,
                    default_symbol,
                    default_display_symbol,
                    default_name,
                    default_scale,
                    default_is_active,
                    default_id
                ],
            )
            .map_err(|e| e.to_string())?;
        }
    }

    let mut effective_currency_id = target.0;
    let need_target_version = target.6 == 0 || target.7 == 0 || current_default.map(|d| d.0) != Some(currency_id);
    if need_target_version {
        conn.execute(
            "INSERT INTO commodities
              (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at)
             VALUES
              (?1, 'currency', ?2, ?3, ?4, ?5, 1, 1, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![target.1, target.2, target.3, target.4, target.5, target.0],
        )
        .map_err(|e| e.to_string())?;
        effective_currency_id = conn.last_insert_rowid();
    }

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
             FROM commodities WHERE id = ?1",
            [effective_currency_id],
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
    let source = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default
                         FROM current_commodities c
             WHERE c.id = ?1
                             AND c.kind = 'currency'",
            [currency_id],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, Option<String>>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, i64>(5)?,
                    row.get::<_, i64>(6)?,
                    row.get::<_, i64>(7)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?
        .ok_or_else(|| "currency not found or not current".to_string())?;

    let is_active = source.6;
    let is_default = source.7;

    if is_default != 0 && is_active != 0 {
        return Err("Cannot deactivate the default currency".to_string());
    }

    if is_active != 0 {
        let in_use: i64 = conn
            .query_row(
                "SELECT
                    (EXISTS(SELECT 1 FROM accounts WHERE commodity_id = ?1)
                     OR EXISTS(SELECT 1 FROM splits WHERE commodity_id = ?1)
                     OR EXISTS(SELECT 1 FROM lots WHERE commodity_id = ?1))",
                [currency_id],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?;

        if in_use != 0 {
            return Err("Cannot deactivate currency in use by accounts or transactions".to_string());
        }
    }

    let toggled_is_active = if is_active == 0 { 1 } else { 0 };

    conn.execute(
        "INSERT INTO commodities
          (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at)
         VALUES
          (?1, 'currency', ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            source.1,
            source.2,
            source.3,
            source.4,
            source.5,
            toggled_is_active,
            source.7,
            source.0
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at
             FROM commodities WHERE id = ?1",
            [new_id],
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
        conditions.push(format!("r.commodity_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("r.quote_commodity_id = ?{}", params_vec.len()));
    }
    if let Some(start) = start_date {
        params_vec.push(Box::new(start));
        conditions.push(format!("r.price_date >= ?{}", params_vec.len()));
    }
    if let Some(end) = end_date {
        params_vec.push(Box::new(end));
        conditions.push(format!("r.price_date <= ?{}", params_vec.len()));
    }

    let limit_clause = if let Some(l) = limit {
        format!(" LIMIT {}", l)
    } else {
        " LIMIT 1000".to_string()
    };

    let sql = format!(
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
              r.rate_date, r.rate, r.source, r.source_id, r.is_derived, r.derived_via_currency_id, r.created_at
          FROM (
             SELECT id, book_id,
                 commodity_id AS from_currency_id,
                 quote_commodity_id AS to_currency_id,
                 price_date AS rate_date,
                 price_value AS rate,
                 source_name AS source,
                 source_id,
                 is_derived,
                 derived_via_commodity_id AS derived_via_currency_id,
                 created_at,
                 commodity_id,
                 quote_commodity_id,
                 price_date
             FROM current_price_observations
             WHERE observation_kind = 'fx_daily'
          ) r
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

    let is_derived = input.is_derived.unwrap_or(false) as i64;

    conn.execute(
        "INSERT INTO price_observations
         (book_id, commodity_id, quote_commodity_id, observation_kind, price_value, price_date,
          source_name, source_id, is_manual, is_derived, derived_via_commodity_id, created_at)
         VALUES (?1, ?2, ?3, 'fx_daily', ?4, ?5, ?6, ?7, 0, ?8, ?9, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.from_currency_id,
            input.to_currency_id,
            input.rate,
            input.rate_date,
            input.source,
            input.source_id,
            is_derived,
            input.derived_via_currency_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let rate = conn
        .query_row(
             "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                  r.rate_date, r.rate, r.source, r.source_id, r.is_derived, r.derived_via_currency_id, r.created_at
              FROM (
              SELECT id, book_id,
                  commodity_id AS from_currency_id,
                  quote_commodity_id AS to_currency_id,
                  price_date AS rate_date,
                  price_value AS rate,
                  source_name AS source,
                  source_id,
                  is_derived,
                  derived_via_commodity_id AS derived_via_currency_id,
                  created_at
              FROM price_observations
              WHERE observation_kind = 'fx_daily'
              ) r
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
    let _ = db;
    let _ = id;
    Err("FX rates are append-only; insert a superseding observation instead".to_string())
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
            "SELECT price_value FROM current_price_observations
             WHERE book_id = ?1 AND observation_kind = 'fx_daily' AND commodity_id = ?2 AND quote_commodity_id = ?3 AND price_date <= ?4
             ORDER BY price_date DESC, created_at DESC LIMIT 1",
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
        conditions.push(format!("r.commodity_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("r.quote_commodity_id = ?{}", params_vec.len()));
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
          FROM (
             SELECT id, book_id,
                 commodity_id AS from_currency_id,
                 quote_commodity_id AS to_currency_id,
                 period_type,
                 period_year,
                 period_month,
                 price_value AS rate,
                 source_name,
                 source_url,
                 source_date,
                 triangulation_path_json AS notes,
                 created_at,
                 created_at AS updated_at,
                 commodity_id,
                 quote_commodity_id
             FROM current_price_observations
             WHERE observation_kind = 'fx_official'
          ) r
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
        "INSERT INTO price_observations
         (book_id, commodity_id, quote_commodity_id, observation_kind, price_value, price_date,
          period_type, period_year, period_month, source_name, source_url, source_date,
          triangulation_path_json, is_manual, is_derived, created_at)
         VALUES (?1, ?2, ?3, 'fx_official', ?4, COALESCE(?5, printf('%04d-%02d-01', ?6, COALESCE(?7, 1))),
                 ?8, ?9, ?10, ?11, ?12, ?13, ?14, 0, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.from_currency_id,
            input.to_currency_id,
            input.rate,
            input.source_date,
            input.period_year,
            input.period_month,
            input.period_type,
            input.period_year,
            input.period_month,
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
              FROM (
              SELECT id, book_id,
                  commodity_id AS from_currency_id,
                  quote_commodity_id AS to_currency_id,
                  period_type,
                  period_year,
                  period_month,
                  price_value AS rate,
                  source_name,
                  source_url,
                  source_date,
                  triangulation_path_json AS notes,
                  created_at,
                  created_at AS updated_at
              FROM price_observations
              WHERE observation_kind = 'fx_official'
              ) r
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

    let current = conn
        .query_row(
            "SELECT commodity_id, quote_commodity_id, period_type, period_year, period_month, price_value, source_name, source_url, source_date, triangulation_path_json
             FROM current_price_observations
             WHERE id = ?1 AND observation_kind = 'fx_official'",
            [input.id],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, i64>(3)?,
                    row.get::<_, Option<i64>>(4)?,
                    row.get::<_, f64>(5)?,
                    row.get::<_, String>(6)?,
                    row.get::<_, Option<String>>(7)?,
                    row.get::<_, Option<String>>(8)?,
                    row.get::<_, Option<String>>(9)?,
                ))
            },
        )
        .map_err(|e| e.to_string())?;

    conn.execute(
        "INSERT INTO price_observations
         (book_id, commodity_id, quote_commodity_id, observation_kind, price_value, price_date,
          period_type, period_year, period_month, source_name, source_url, source_date,
          triangulation_path_json, supersedes_observation_id, created_at)
         VALUES (?1, ?2, ?3, 'fx_official', ?4, COALESCE(?5, printf('%04d-%02d-01', ?6, COALESCE(?7, 1))),
                 ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            SINGLE_BOOK_ID,
            current.0,
            current.1,
            input.rate.unwrap_or(current.5),
            input.source_date.clone().or(current.8.clone()),
            current.3,
            current.4,
            current.2,
            current.3,
            current.4,
            input.source_name.unwrap_or(current.6),
            input.source_url.or(current.7),
            input.source_date.or(current.8),
            input.notes.or(current.9),
            input.id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();

    let rate = conn
        .query_row(
            "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                    r.period_type, r.period_year, r.period_month, r.rate,
                    r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
                 FROM (
                     SELECT id, book_id,
                              commodity_id AS from_currency_id,
                              quote_commodity_id AS to_currency_id,
                              period_type,
                              period_year,
                              period_month,
                              price_value AS rate,
                              source_name,
                              source_url,
                              source_date,
                              triangulation_path_json AS notes,
                              created_at,
                              created_at AS updated_at
                     FROM price_observations
                     WHERE observation_kind = 'fx_official'
                 ) r
             LEFT JOIN commodities fc ON r.from_currency_id = fc.id
             LEFT JOIN commodities tc ON r.to_currency_id = tc.id
             WHERE r.id = ?1",
                [new_id],
            map_fx_rate_official_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(rate)
}

#[command]
pub fn delete_fx_rate_official(db: State<DbState>, id: i64) -> Result<(), String> {
    let _ = db;
    let _ = id;
    Err("Official FX rates are append-only; insert a superseding observation instead".to_string())
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
             FROM (
                SELECT id, book_id,
                   commodity_id AS from_currency_id,
                   quote_commodity_id AS to_currency_id,
                   period_type,
                   period_year,
                   period_month,
                   price_value AS rate,
                   source_name,
                   source_url,
                   source_date,
                   triangulation_path_json AS notes,
                   created_at,
                   created_at AS updated_at
                FROM current_price_observations
                WHERE observation_kind = 'fx_official'
             ) r
         LEFT JOIN commodities fc ON r.from_currency_id = fc.id
         LEFT JOIN commodities tc ON r.to_currency_id = tc.id
             WHERE r.book_id = ?1 AND r.from_currency_id = ?2 AND r.to_currency_id = ?3
           AND r.period_type = ?4 AND r.period_year = ?5 AND r.period_month = ?6
         LIMIT 1"
    } else {
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.period_type, r.period_year, r.period_month, r.rate,
                r.source_name, r.source_url, r.source_date, r.notes, r.created_at, r.updated_at
             FROM (
                SELECT id, book_id,
                   commodity_id AS from_currency_id,
                   quote_commodity_id AS to_currency_id,
                   period_type,
                   period_year,
                   period_month,
                   price_value AS rate,
                   source_name,
                   source_url,
                   source_date,
                   triangulation_path_json AS notes,
                   created_at,
                   created_at AS updated_at
                FROM current_price_observations
                WHERE observation_kind = 'fx_official'
             ) r
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
            "SELECT id, ?1 AS book_id, name, provider AS country_code, base_url AS website_url, NULL AS notes, created_at, created_at AS updated_at
             FROM price_sources ORDER BY name ASC",
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
        "INSERT INTO price_sources (name, kind, provider, base_url, created_at)
         VALUES (?1, 'provider', ?2, ?3, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![input.name, input.country_code, input.website_url],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let source = conn
        .query_row(
            "SELECT id, ?2 AS book_id, name, provider AS country_code, base_url AS website_url, NULL AS notes, created_at, created_at AS updated_at
             FROM price_sources WHERE id = ?1",
            params![id, book_id],
            map_fx_rate_source_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(source)
}

// ============================================================================
// FX Settings + Source Assignments + Refresh State
// ============================================================================

#[command]
pub fn get_fx_rate_settings(db: State<DbState>, book_id: i64) -> Result<Option<FxRateSettings>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let row = conn
        .query_row(
            "SELECT p.book_id, b.base_commodity_id, c.symbol, p.default_source_id, src.name,
                    p.refresh_enabled, p.refresh_hour_utc, p.refresh_minute_utc, p.max_backfill_days,
                    p.weekend_policy, p.created_at, p.created_at AS updated_at
             FROM current_pricing_policies p
             JOIN current_book_base_currency_history b ON b.book_id = p.book_id
             LEFT JOIN commodities c ON b.base_commodity_id = c.id
             LEFT JOIN price_sources src ON p.default_source_id = src.id
             WHERE p.book_id = ?1
             LIMIT 1",
            [book_id],
            map_fx_rate_settings_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(row)
}

#[command]
pub fn set_fx_rate_settings(db: State<DbState>, input: FxRateSettingsUpdate) -> Result<FxRateSettings, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let existing: Option<(i64, String, String, i64, i64, i64, i64, String, Option<i64>, i64, i64)> = conn
        .query_row(
            "SELECT p.id, p.name, p.mode, p.refresh_enabled, p.refresh_hour_utc, p.refresh_minute_utc,
                    p.max_backfill_days, p.weekend_policy, p.default_source_id,
                    b.id, b.base_commodity_id
             FROM current_pricing_policies p
             JOIN current_book_base_currency_history b ON b.book_id = p.book_id
             WHERE p.book_id = ?1
             LIMIT 1",
            [SINGLE_BOOK_ID],
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
                    row.get(9)?,
                    row.get(10)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if let Some((
        policy_id,
        policy_name,
        policy_mode,
        refresh_enabled_old,
        refresh_hour_old,
        refresh_minute_old,
        max_backfill_days_old,
        weekend_policy_old,
        default_source_id_old,
        base_history_id,
        current_base_currency_id,
    )) = existing
    {
        let next_default_source_id = input.default_source_id.or(default_source_id_old);
        let next_refresh_enabled = input.refresh_enabled.unwrap_or(refresh_enabled_old != 0) as i64;
        let next_refresh_hour_utc = input.refresh_hour_utc.unwrap_or(refresh_hour_old);
        let next_refresh_minute_utc = input.refresh_minute_utc.unwrap_or(refresh_minute_old);
        let next_max_backfill_days = input.max_backfill_days.unwrap_or(max_backfill_days_old);
        let next_weekend_policy = input.weekend_policy.unwrap_or_else(|| weekend_policy_old.clone());

        let policy_changed = next_default_source_id != default_source_id_old
            || next_refresh_enabled != refresh_enabled_old
            || next_refresh_hour_utc != refresh_hour_old
            || next_refresh_minute_utc != refresh_minute_old
            || next_max_backfill_days != max_backfill_days_old
            || next_weekend_policy != weekend_policy_old;

        if policy_changed {
            conn.execute(
                "INSERT INTO pricing_policies
                    (previous_pricing_policy_id, book_id, name, mode, refresh_enabled, refresh_hour_utc,
                     refresh_minute_utc, max_backfill_days, weekend_policy, staleness_max_days,
                     triangulation_max_hops, rounding_mode, prefer_official_fx, default_source_id, created_at)
                 SELECT ?1, book_id, ?2, ?3, ?4, ?5, ?6, ?7, ?8, staleness_max_days,
                        triangulation_max_hops, rounding_mode, prefer_official_fx, ?9,
                        strftime('%Y-%m-%dT%H:%M:%fZ','now')
                 FROM pricing_policies
                 WHERE id = ?1",
                params![
                    policy_id,
                    policy_name,
                    policy_mode,
                    next_refresh_enabled,
                    next_refresh_hour_utc,
                    next_refresh_minute_utc,
                    next_max_backfill_days,
                    next_weekend_policy,
                    next_default_source_id,
                ],
            )
            .map_err(|e| e.to_string())?;
        }

        if let Some(new_base_id) = input.base_currency_id {
            if new_base_id != current_base_currency_id {
                conn.execute(
                    "INSERT INTO book_base_currency_history
                        (previous_book_base_currency_history_id, book_id, base_commodity_id, effective_from, effective_to, created_at)
                     VALUES (?1, ?2, ?3, date('now'), NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![base_history_id, SINGLE_BOOK_ID, new_base_id],
                )
                .map_err(|e| e.to_string())?;
            }
        }
    } else {
        let base_currency_id = if let Some(id) = input.base_currency_id {
            id
        } else {
            conn.query_row(
                                "SELECT c.id
                                 FROM current_commodities c
                                 WHERE c.book_id = ?1
                                     AND c.kind = 'currency'
                                     AND c.is_default = 1
                                 LIMIT 1",
                [SINGLE_BOOK_ID],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?
        };

        let refresh_enabled = input.refresh_enabled.unwrap_or(true) as i64;
        let refresh_hour_utc = input.refresh_hour_utc.unwrap_or(4);
        let refresh_minute_utc = input.refresh_minute_utc.unwrap_or(0);
        let max_backfill_days = input.max_backfill_days.unwrap_or(370);
        let weekend_policy = input.weekend_policy.unwrap_or_else(|| "skip".to_string());

        conn.execute(
            "INSERT INTO pricing_policies
                (previous_pricing_policy_id, book_id, name, mode, refresh_enabled, refresh_hour_utc, refresh_minute_utc, max_backfill_days, weekend_policy, staleness_max_days, triangulation_max_hops, rounding_mode, prefer_official_fx, default_source_id, created_at)
             VALUES (?1, ?2, 'Default policy', 'latest_corrected', ?3, ?4, ?5, ?6, ?7, 7, 2, 'bankers', 0, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                Option::<i64>::None,
                SINGLE_BOOK_ID,
                refresh_enabled,
                refresh_hour_utc,
                refresh_minute_utc,
                max_backfill_days,
                weekend_policy,
                input.default_source_id,
            ],
        )
        .map_err(|e| e.to_string())?;

        conn.execute(
            "INSERT INTO book_base_currency_history (previous_book_base_currency_history_id, book_id, base_commodity_id, effective_from, effective_to, created_at)
             VALUES (?1, ?2, ?3, date('now'), NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![Option::<i64>::None, SINGLE_BOOK_ID, base_currency_id],
        )
        .map_err(|e| e.to_string())?;
    }

    let updated = conn
        .query_row(
            "SELECT p.book_id, b.base_commodity_id, c.symbol, p.default_source_id, src.name,
                    p.refresh_enabled, p.refresh_hour_utc, p.refresh_minute_utc, p.max_backfill_days,
                    p.weekend_policy, p.created_at, p.created_at AS updated_at
             FROM current_pricing_policies p
             JOIN current_book_base_currency_history b ON b.book_id = p.book_id
             LEFT JOIN commodities c ON b.base_commodity_id = c.id
             LEFT JOIN price_sources src ON p.default_source_id = src.id
             WHERE p.book_id = ?1
             LIMIT 1",
            [SINGLE_BOOK_ID],
            map_fx_rate_settings_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(updated)
}

#[command]
pub fn list_fx_rate_source_assignments(
    db: State<DbState>,
    book_id: i64,
    from_currency_id: Option<i64>,
    to_currency_id: Option<i64>,
    on_date: Option<String>,
) -> Result<Vec<FxRateSourceAssignment>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut conditions = vec!["a.book_id = ?1".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![Box::new(book_id)];

    if let Some(from_id) = from_currency_id {
        params_vec.push(Box::new(from_id));
        conditions.push(format!("a.commodity_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("a.quote_commodity_id = ?{}", params_vec.len()));
    }
    if let Some(date) = on_date {
        let date_start = date.clone();
        params_vec.push(Box::new(date_start));
        conditions.push(format!("a.effective_from <= ?{}", params_vec.len()));
        params_vec.push(Box::new(date));
        conditions.push(format!("(a.effective_to IS NULL OR a.effective_to >= ?{})", params_vec.len()));
    }

    let sql = format!(
        "SELECT a.id, a.book_id, a.from_currency_id, fc.symbol, a.to_currency_id, tc.symbol,
                a.source_id, s.name, a.effective_from, a.effective_to, a.created_at, a.updated_at
          FROM (
             SELECT id, book_id,
                 commodity_id AS from_currency_id,
                 quote_commodity_id AS to_currency_id,
                 source_id,
                 effective_from,
                 effective_to,
                 created_at,
                 updated_at,
                 commodity_id,
                 quote_commodity_id
             FROM current_pricing_source_assignments
          ) a
          LEFT JOIN commodities fc ON a.from_currency_id = fc.id
          LEFT JOIN commodities tc ON a.to_currency_id = tc.id
          LEFT JOIN price_sources s ON a.source_id = s.id
         WHERE {}
         ORDER BY a.effective_from DESC",
        conditions.join(" AND ")
    );

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    let rows = stmt
        .query_map(params_refs.as_slice(), map_fx_rate_source_assignment_row)
        .map_err(|e| e.to_string())?;

    let mut assignments = Vec::new();
    for row in rows {
        assignments.push(row.map_err(|e| e.to_string())?);
    }
    Ok(assignments)
}

#[command]
pub fn create_fx_rate_source_assignment(
    db: State<DbState>,
    input: FxRateSourceAssignmentCreate,
) -> Result<FxRateSourceAssignment, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute(
        "INSERT INTO pricing_source_assignments
            (previous_pricing_source_assignment_id, book_id, commodity_id, quote_commodity_id, source_id, priority, effective_from, effective_to, created_at, updated_at)
         VALUES (NULL, ?1, ?2, ?3, ?4, 100, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            SINGLE_BOOK_ID,
            input.from_currency_id,
            input.to_currency_id,
            input.source_id,
            input.effective_from,
            input.effective_to,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let assignment = conn
        .query_row(
            "SELECT a.id, a.book_id, a.from_currency_id, fc.symbol, a.to_currency_id, tc.symbol,
                    a.source_id, s.name, a.effective_from, a.effective_to, a.created_at, a.updated_at
                  FROM (
                  SELECT id, book_id,
                      commodity_id AS from_currency_id,
                      quote_commodity_id AS to_currency_id,
                      source_id,
                      effective_from,
                      effective_to,
                      created_at,
                      updated_at
                  FROM current_pricing_source_assignments
                  ) a
                  LEFT JOIN commodities fc ON a.from_currency_id = fc.id
                  LEFT JOIN commodities tc ON a.to_currency_id = tc.id
                  LEFT JOIN price_sources s ON a.source_id = s.id
             WHERE a.id = ?1",
            [id],
            map_fx_rate_source_assignment_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(assignment)
}

#[command]
pub fn update_fx_rate_source_assignment(
    db: State<DbState>,
    input: FxRateSourceAssignmentUpdate,
) -> Result<FxRateSourceAssignment, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let current: Option<(i64, i64, i64, i64, i64, String, Option<String>)> = conn
        .query_row(
            "SELECT id, book_id, commodity_id, quote_commodity_id, source_id, effective_from, effective_to
             FROM current_pricing_source_assignments
             WHERE id = ?1",
            [input.id],
            |row| {
                Ok((
                    row.get(0)?,
                    row.get(1)?,
                    row.get(2)?,
                    row.get(3)?,
                    row.get(4)?,
                    row.get(5)?,
                    row.get(6)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let (id, book_id, commodity_id, quote_commodity_id, source_id_old, effective_from_old, effective_to_old) =
        current.ok_or_else(|| "assignment not found".to_string())?;

    let next_source_id = input.source_id.unwrap_or(source_id_old);
    let next_effective_from = input.effective_from.unwrap_or(effective_from_old);
    let next_effective_to = input.effective_to.or(effective_to_old);

    conn.execute(
        "INSERT INTO pricing_source_assignments
            (previous_pricing_source_assignment_id, book_id, commodity_id, quote_commodity_id, source_id, priority, effective_from, effective_to, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, 100, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            id,
            book_id,
            commodity_id,
            quote_commodity_id,
            next_source_id,
            next_effective_from,
            next_effective_to,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();

    let assignment = conn
        .query_row(
            "SELECT a.id, a.book_id, a.from_currency_id, fc.symbol, a.to_currency_id, tc.symbol,
                    a.source_id, s.name, a.effective_from, a.effective_to, a.created_at, a.updated_at
                  FROM (
                  SELECT id, book_id,
                      commodity_id AS from_currency_id,
                      quote_commodity_id AS to_currency_id,
                      source_id,
                      effective_from,
                      effective_to,
                      created_at,
                      updated_at
                  FROM current_pricing_source_assignments
                  ) a
                  LEFT JOIN commodities fc ON a.from_currency_id = fc.id
                  LEFT JOIN commodities tc ON a.to_currency_id = tc.id
                  LEFT JOIN price_sources s ON a.source_id = s.id
             WHERE a.id = ?1",
            [new_id],
            map_fx_rate_source_assignment_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(assignment)
}

#[command]
pub fn delete_fx_rate_source_assignment(db: State<DbState>, id: i64) -> Result<(), String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let closed_rows = conn
        .execute(
            "INSERT INTO pricing_source_assignments
                (previous_pricing_source_assignment_id, book_id, commodity_id, quote_commodity_id, source_id, priority, effective_from, effective_to, created_at, updated_at)
             SELECT id, book_id, commodity_id, quote_commodity_id, source_id, priority, effective_from,
                    CASE
                      WHEN effective_to IS NOT NULL THEN effective_to
                      WHEN effective_from > date('now') THEN effective_from
                      ELSE date('now')
                    END,
                    strftime('%Y-%m-%dT%H:%M:%fZ','now'),
                    strftime('%Y-%m-%dT%H:%M:%fZ','now')
             FROM current_pricing_source_assignments
             WHERE id = ?1",
            [id],
        )
        .map_err(|e| e.to_string())?;

    if closed_rows == 0 {
        return Err("assignment not found".to_string());
    }

    Ok(())
}

#[command]
pub fn list_fx_rate_refresh_state(
    db: State<DbState>,
    book_id: i64,
    from_currency_id: Option<i64>,
    to_currency_id: Option<i64>,
    source_id: Option<i64>,
) -> Result<Vec<FxRateRefreshState>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut conditions = vec!["r.book_id = ?1".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![Box::new(book_id)];

    if let Some(from_id) = from_currency_id {
        params_vec.push(Box::new(from_id));
        conditions.push(format!("r.commodity_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("r.quote_commodity_id = ?{}", params_vec.len()));
    }
    if let Some(source_id) = source_id {
        params_vec.push(Box::new(source_id));
        conditions.push(format!("r.source_id = ?{}", params_vec.len()));
    }

    let sql = format!(
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.source_id, s.name, r.last_success_date, r.last_attempt_at, r.last_error,
                r.created_at, r.updated_at
          FROM (
             SELECT id, book_id,
                 commodity_id AS from_currency_id,
                 quote_commodity_id AS to_currency_id,
                 source_id,
                 last_success_date,
                 last_attempt_at,
                 last_error,
                 created_at,
                 updated_at,
                 commodity_id,
                 quote_commodity_id
             FROM pricing_refresh_state
          ) r
          LEFT JOIN commodities fc ON r.from_currency_id = fc.id
          LEFT JOIN commodities tc ON r.to_currency_id = tc.id
          LEFT JOIN price_sources s ON r.source_id = s.id
         WHERE {}
         ORDER BY r.from_currency_id, r.to_currency_id",
        conditions.join(" AND ")
    );

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    let rows = stmt
        .query_map(params_refs.as_slice(), map_fx_rate_refresh_state_row)
        .map_err(|e| e.to_string())?;

    let mut states = Vec::new();
    for row in rows {
        states.push(row.map_err(|e| e.to_string())?);
    }
    Ok(states)
}

#[command]
pub fn upsert_fx_rate_refresh_state(
    db: State<DbState>,
    input: FxRateRefreshStateUpsert,
) -> Result<FxRateRefreshState, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute(
          "INSERT INTO pricing_refresh_state
                (book_id, commodity_id, quote_commodity_id, source_id, last_success_date, last_attempt_at, last_error, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))
            ON CONFLICT(book_id, commodity_id, quote_commodity_id, source_id) DO UPDATE SET
            last_success_date = excluded.last_success_date,
            last_attempt_at = excluded.last_attempt_at,
            last_error = excluded.last_error,
            updated_at = excluded.updated_at",
        params![
            SINGLE_BOOK_ID,
            input.from_currency_id,
            input.to_currency_id,
            input.source_id,
            input.last_success_date,
            input.last_attempt_at,
            input.last_error,
        ],
    )
    .map_err(|e| e.to_string())?;

    let state = conn
        .query_row(
            "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                    r.source_id, s.name, r.last_success_date, r.last_attempt_at, r.last_error,
                    r.created_at, r.updated_at
                  FROM (
                  SELECT id, book_id,
                      commodity_id AS from_currency_id,
                      quote_commodity_id AS to_currency_id,
                      source_id,
                      last_success_date,
                      last_attempt_at,
                      last_error,
                      created_at,
                      updated_at
                  FROM pricing_refresh_state
                  ) r
                  LEFT JOIN commodities fc ON r.from_currency_id = fc.id
                  LEFT JOIN commodities tc ON r.to_currency_id = tc.id
                  LEFT JOIN price_sources s ON r.source_id = s.id
                  WHERE r.book_id = ?1 AND r.from_currency_id = ?2 AND r.to_currency_id = ?3 AND r.source_id = ?4
             LIMIT 1",
            params![
                SINGLE_BOOK_ID,
                input.from_currency_id,
                input.to_currency_id,
                input.source_id
            ],
            map_fx_rate_refresh_state_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(state)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::open_and_migrate;
    use crate::state::{DbState, DbStateInner};
    use rusqlite::Connection;
    use std::fs;
    use std::path::PathBuf;
    use std::sync::Mutex;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn create_temp_dir(name: &str) -> PathBuf {
        let start = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis();
        let mut temp = std::env::temp_dir();
        temp.push(format!("rekenraam_fx_{name}_{start}"));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");
        temp
    }

    fn open_test_db() -> (Connection, PathBuf) {
        let temp = create_temp_dir("db");
        let (conn, path, _audit_user) = open_and_migrate(&temp).expect("open and migrate");
        (conn, path)
    }

    fn create_db_state() -> DbState {
        let temp = create_temp_dir("state");
        let (conn, db_path, audit_user) = open_and_migrate(&temp).expect("open and migrate");
        DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path),
                conn: Some(conn),
                audit_user: Some(audit_user),
            }),
            read_conn: Default::default(),
        }
    }

    fn as_state<'a>(db: &'a DbState) -> State<'a, DbState> {
        unsafe { std::mem::transmute::<&'a DbState, State<'a, DbState>>(db) }
    }

    #[test]
    fn test_fx_settings_and_assignments_are_append_only() {
        let db_state = create_db_state();

        let (eur_id, usd_id, ecb_id, hmrc_id, policy_before, assignment_before) = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");
            let eur_id: i64 = conn
                .query_row(
                    "SELECT id FROM commodities WHERE symbol='EUR' AND kind='currency' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("eur id");
            let usd_id: i64 = conn
                .query_row(
                    "SELECT id FROM commodities WHERE symbol='USD' AND kind='currency' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("usd id");
            let ecb_id: i64 = conn
                .query_row(
                    "SELECT id FROM price_sources WHERE name='ECB' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("ecb id");
            let hmrc_id: i64 = conn
                .query_row(
                    "SELECT id FROM price_sources WHERE name='HMRC' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("hmrc id");
            let policy_before: i64 = conn
                .query_row("SELECT COUNT(*) FROM pricing_policies", [], |row| row.get(0))
                .expect("policy count");
            let assignment_before: i64 = conn
                .query_row("SELECT COUNT(*) FROM pricing_source_assignments", [], |row| row.get(0))
                .expect("assignment count");
            (eur_id, usd_id, ecb_id, hmrc_id, policy_before, assignment_before)
        };

        let settings = set_fx_rate_settings(
            as_state(&db_state),
            FxRateSettingsUpdate {
                base_currency_id: None,
                default_source_id: Some(ecb_id),
                refresh_enabled: Some(true),
                refresh_hour_utc: Some(6),
                refresh_minute_utc: Some(15),
                max_backfill_days: Some(120),
                weekend_policy: Some("skip".to_string()),
            },
        )
        .expect("set fx settings");
        assert_eq!(settings.refresh_hour_utc, 6);
        assert_eq!(settings.refresh_minute_utc, 15);
        assert_eq!(settings.max_backfill_days, 120);

        let policy_after: i64 = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");
            conn.query_row("SELECT COUNT(*) FROM pricing_policies", [], |row| row.get(0))
                .expect("policy count after")
        };
        assert_eq!(policy_after, policy_before + 1);

        let created = create_fx_rate_source_assignment(
            as_state(&db_state),
            FxRateSourceAssignmentCreate {
                from_currency_id: eur_id,
                to_currency_id: usd_id,
                source_id: ecb_id,
                effective_from: "2025-01-01".to_string(),
                effective_to: None,
            },
        )
        .expect("create assignment");

        let updated = update_fx_rate_source_assignment(
            as_state(&db_state),
            FxRateSourceAssignmentUpdate {
                id: created.id,
                source_id: Some(hmrc_id),
                effective_from: None,
                effective_to: None,
            },
        )
        .expect("update assignment");
        assert_ne!(updated.id, created.id);
        assert_eq!(updated.source_id, hmrc_id);

        delete_fx_rate_source_assignment(as_state(&db_state), updated.id).expect("close assignment");

        let (assignment_after, current_effective_to_is_set): (i64, i64) = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");
            let assignment_after: i64 = conn
                .query_row("SELECT COUNT(*) FROM pricing_source_assignments", [], |row| row.get(0))
                .expect("assignment count after");
            let current_effective_to_is_set: i64 = conn
                .query_row(
                    "SELECT COUNT(*) FROM current_pricing_source_assignments
                     WHERE book_id = 1 AND commodity_id = ?1 AND quote_commodity_id = ?2 AND effective_to IS NOT NULL",
                    params![eur_id, usd_id],
                    |row| row.get(0),
                )
                .expect("current closed assignment check");
            (assignment_after, current_effective_to_is_set)
        };

        assert_eq!(assignment_after, assignment_before + 3);
        assert_eq!(current_effective_to_is_set, 1);
    }

    #[test]
    fn test_base_currency_change_creates_history_revision_chain() {
        let db_state = create_db_state();

        let (usd_id, eur_id, history_before, current_history_before, current_base_before) = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");

            let usd_id: i64 = conn
                .query_row(
                    "SELECT id FROM commodities WHERE symbol='USD' AND kind='currency' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("usd id");
            let eur_id: i64 = conn
                .query_row(
                    "SELECT id FROM commodities WHERE symbol='EUR' AND kind='currency' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("eur id");
            let history_before: i64 = conn
                .query_row("SELECT COUNT(*) FROM book_base_currency_history", [], |row| row.get(0))
                .expect("history count before");
            let (current_history_before, current_base_before): (i64, i64) = conn
                .query_row(
                    "SELECT id, base_commodity_id FROM current_book_base_currency_history WHERE book_id = 1 LIMIT 1",
                    [],
                    |row| Ok((row.get(0)?, row.get(1)?)),
                )
                .expect("current history before");

            (usd_id, eur_id, history_before, current_history_before, current_base_before)
        };

        assert_eq!(current_base_before, usd_id);

        let updated = set_fx_rate_settings(
            as_state(&db_state),
            FxRateSettingsUpdate {
                base_currency_id: Some(eur_id),
                default_source_id: None,
                refresh_enabled: None,
                refresh_hour_utc: None,
                refresh_minute_utc: None,
                max_backfill_days: None,
                weekend_policy: None,
            },
        )
        .expect("set base currency to eur");
        assert_eq!(updated.base_currency_id, eur_id);

        let (history_after, current_history_after, current_base_after, previous_link): (i64, i64, i64, i64) = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");

            let history_after: i64 = conn
                .query_row("SELECT COUNT(*) FROM book_base_currency_history", [], |row| row.get(0))
                .expect("history count after");
            let (current_history_after, current_base_after): (i64, i64) = conn
                .query_row(
                    "SELECT id, base_commodity_id FROM current_book_base_currency_history WHERE book_id = 1 LIMIT 1",
                    [],
                    |row| Ok((row.get(0)?, row.get(1)?)),
                )
                .expect("current history after");
            let previous_link: i64 = conn
                .query_row(
                    "SELECT previous_book_base_currency_history_id
                     FROM book_base_currency_history
                     WHERE id = ?1",
                    [current_history_after],
                    |row| row.get(0),
                )
                .expect("previous link");

            (history_after, current_history_after, current_base_after, previous_link)
        };

        assert_eq!(history_after, history_before + 1);
        assert_ne!(current_history_after, current_history_before);
        assert_eq!(current_base_after, eur_id);
        assert_eq!(previous_link, current_history_before);
    }

    #[test]
    fn test_base_currency_noop_does_not_create_history_revision() {
        let db_state = create_db_state();

        let (usd_id, history_before, current_history_before, policy_before) = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");

            let usd_id: i64 = conn
                .query_row(
                    "SELECT id FROM commodities WHERE symbol='USD' AND kind='currency' LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("usd id");
            let history_before: i64 = conn
                .query_row("SELECT COUNT(*) FROM book_base_currency_history", [], |row| row.get(0))
                .expect("history count before");
            let current_history_before: i64 = conn
                .query_row(
                    "SELECT id FROM current_book_base_currency_history WHERE book_id = 1 LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("current history before");
            let policy_before: i64 = conn
                .query_row("SELECT COUNT(*) FROM pricing_policies", [], |row| row.get(0))
                .expect("policy count before");

            (usd_id, history_before, current_history_before, policy_before)
        };

        let updated = set_fx_rate_settings(
            as_state(&db_state),
            FxRateSettingsUpdate {
                base_currency_id: Some(usd_id),
                default_source_id: None,
                refresh_enabled: None,
                refresh_hour_utc: None,
                refresh_minute_utc: None,
                max_backfill_days: None,
                weekend_policy: None,
            },
        )
        .expect("set same base currency");
        assert_eq!(updated.base_currency_id, usd_id);

        let (history_after, current_history_after, policy_after): (i64, i64, i64) = {
            let guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_ref().expect("conn");

            let history_after: i64 = conn
                .query_row("SELECT COUNT(*) FROM book_base_currency_history", [], |row| row.get(0))
                .expect("history count after");
            let current_history_after: i64 = conn
                .query_row(
                    "SELECT id FROM current_book_base_currency_history WHERE book_id = 1 LIMIT 1",
                    [],
                    |row| row.get(0),
                )
                .expect("current history after");
            let policy_after: i64 = conn
                .query_row("SELECT COUNT(*) FROM pricing_policies", [], |row| row.get(0))
                .expect("policy count after");

            (history_after, current_history_after, policy_after)
        };

        assert_eq!(history_after, history_before);
        assert_eq!(current_history_after, current_history_before);
        assert_eq!(policy_after, policy_before);
    }

    #[test]
    fn test_fx_schema_columns_exist() {
        let (conn, _path) = open_test_db();

        let mut columns = Vec::new();
        let mut stmt = conn
            .prepare("PRAGMA table_info(price_observations)")
            .expect("prepare pragma");
        let rows = stmt
            .query_map([], |row| row.get::<_, String>(1))
            .expect("query pragma");
        for r in rows {
            columns.push(r.expect("column"));
        }

        assert!(columns.contains(&"source_id".to_string()));
        assert!(columns.contains(&"is_derived".to_string()));
        assert!(columns.contains(&"derived_via_commodity_id".to_string()));

        let settings_exists: i64 = conn
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='pricing_policies'",
                [],
                |row| row.get(0),
            )
            .expect("settings table");
        assert_eq!(settings_exists, 1);

        let refresh_exists: i64 = conn
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='pricing_refresh_state'",
                [],
                |row| row.get(0),
            )
            .expect("refresh table");
        assert_eq!(refresh_exists, 1);
    }

    #[test]
    fn test_fx_settings_seeded() {
        let (conn, _path) = open_test_db();

        let count: i64 = conn
            .query_row("SELECT COUNT(*) FROM pricing_policies", [], |row| row.get(0))
            .expect("settings count");
        assert!(count >= 1);
    }

    #[test]
    fn test_fx_refresh_state_roundtrip() {
        let (conn, _path) = open_test_db();

        let from_id: i64 = conn
            .query_row(
                "SELECT id FROM commodities WHERE symbol='EUR' AND kind='currency' LIMIT 1",
                [],
                |row| row.get(0),
            )
            .expect("eur id");
        let to_id: i64 = conn
            .query_row(
                "SELECT id FROM commodities WHERE symbol='USD' AND kind='currency' LIMIT 1",
                [],
                |row| row.get(0),
            )
            .expect("usd id");
        let source_id: i64 = conn
            .query_row(
                "SELECT id FROM price_sources WHERE name='ECB' LIMIT 1",
                [],
                |row| row.get(0),
            )
            .expect("source id");

        conn.execute(
            "INSERT INTO pricing_refresh_state
             (book_id, commodity_id, quote_commodity_id, source_id, last_success_date, last_attempt_at, last_error)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
            params![SINGLE_BOOK_ID, from_id, to_id, source_id, "2025-12-30", "2025-12-31T00:00:00Z", "oops"],
        )
        .expect("insert refresh state");

        let last_success: String = conn
            .query_row(
                "SELECT last_success_date FROM pricing_refresh_state WHERE source_id = ?1",
                [source_id],
                |row| row.get(0),
            )
            .expect("read refresh state");

        assert_eq!(last_success, "2025-12-30");
    }
}

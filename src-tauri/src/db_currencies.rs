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
        "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
                 FROM current_commodities c
                 WHERE c.book_id = ?1
                     AND c.kind = 'currency'
                     AND c.is_active = 1
                 ORDER BY c.is_default DESC, c.symbol ASC"
    } else {
        "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
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
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
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
          (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at, updated_at)
         VALUES
          (?1, 'currency', ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
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
                  (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at, updated_at)
                 VALUES
                  (?1, 'currency', ?2, ?3, ?4, ?5, ?6, 0, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
              (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at, updated_at)
             VALUES
              (?1, 'currency', ?2, ?3, ?4, ?5, 1, 1, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![target.1, target.2, target.3, target.4, target.5, target.0],
        )
        .map_err(|e| e.to_string())?;
        effective_currency_id = conn.last_insert_rowid();
    }

    let currency = conn
        .query_row(
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
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
          (book_id, kind, symbol, display_symbol, name, scale, is_active, is_default, previous_commodity_id, created_at, updated_at)
         VALUES
          (?1, 'currency', ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
            "SELECT id, book_id, symbol, display_symbol, name, scale, is_active, is_default, created_at, updated_at
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
                r.rate_date, r.rate, r.source, r.source_id, r.is_derived, r.derived_via_currency_id, r.created_at
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

    let is_derived = input.is_derived.unwrap_or(false) as i64;

    conn.execute(
        "INSERT OR REPLACE INTO fx_rates_daily (book_id, from_currency_id, to_currency_id, rate_date, rate, source, source_id, is_derived, derived_via_currency_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.from_currency_id,
            input.to_currency_id,
            input.rate_date,
            input.rate,
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
            "SELECT s.book_id, s.base_currency_id, c.symbol, s.default_source_id, src.name,
                    s.refresh_enabled, s.refresh_hour_utc, s.refresh_minute_utc, s.max_backfill_days,
                    s.weekend_policy, s.created_at, s.updated_at
             FROM fx_rate_settings s
             LEFT JOIN commodities c ON s.base_currency_id = c.id
             LEFT JOIN fx_rate_sources src ON s.default_source_id = src.id
             WHERE s.book_id = ?1
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

    let existing: Option<FxRateSettings> = conn
        .query_row(
            "SELECT s.book_id, s.base_currency_id, c.symbol, s.default_source_id, src.name,
                    s.refresh_enabled, s.refresh_hour_utc, s.refresh_minute_utc, s.max_backfill_days,
                    s.weekend_policy, s.created_at, s.updated_at
             FROM fx_rate_settings s
             LEFT JOIN commodities c ON s.base_currency_id = c.id
             LEFT JOIN fx_rate_sources src ON s.default_source_id = src.id
             WHERE s.book_id = ?1
             LIMIT 1",
            [SINGLE_BOOK_ID],
            map_fx_rate_settings_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if let Some(current) = existing {
        let mut updates = vec!["updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')".to_string()];
        let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![];

        if let Some(base_id) = input.base_currency_id {
            params_vec.push(Box::new(base_id));
            updates.push(format!("base_currency_id = ?{}", params_vec.len()));
        }
        if let Some(default_source_id) = input.default_source_id {
            params_vec.push(Box::new(default_source_id));
            updates.push(format!("default_source_id = ?{}", params_vec.len()));
        }
        if let Some(enabled) = input.refresh_enabled {
            params_vec.push(Box::new(enabled as i64));
            updates.push(format!("refresh_enabled = ?{}", params_vec.len()));
        }
        if let Some(hour) = input.refresh_hour_utc {
            params_vec.push(Box::new(hour));
            updates.push(format!("refresh_hour_utc = ?{}", params_vec.len()));
        }
        if let Some(minute) = input.refresh_minute_utc {
            params_vec.push(Box::new(minute));
            updates.push(format!("refresh_minute_utc = ?{}", params_vec.len()));
        }
        if let Some(days) = input.max_backfill_days {
            params_vec.push(Box::new(days));
            updates.push(format!("max_backfill_days = ?{}", params_vec.len()));
        }
        if let Some(policy) = input.weekend_policy {
            params_vec.push(Box::new(policy));
            updates.push(format!("weekend_policy = ?{}", params_vec.len()));
        }

        params_vec.push(Box::new(current.book_id));
        let sql = format!("UPDATE fx_rate_settings SET {} WHERE book_id = ?{}", updates.join(", "), params_vec.len());
        let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
        conn.execute(&sql, params_refs.as_slice()).map_err(|e| e.to_string())?;
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
            "INSERT INTO fx_rate_settings
                (book_id, base_currency_id, default_source_id, refresh_enabled, refresh_hour_utc, refresh_minute_utc, max_backfill_days, weekend_policy, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                SINGLE_BOOK_ID,
                base_currency_id,
                input.default_source_id,
                refresh_enabled,
                refresh_hour_utc,
                refresh_minute_utc,
                max_backfill_days,
                weekend_policy
            ],
        )
        .map_err(|e| e.to_string())?;
    }

    let updated = conn
        .query_row(
            "SELECT s.book_id, s.base_currency_id, c.symbol, s.default_source_id, src.name,
                    s.refresh_enabled, s.refresh_hour_utc, s.refresh_minute_utc, s.max_backfill_days,
                    s.weekend_policy, s.created_at, s.updated_at
             FROM fx_rate_settings s
             LEFT JOIN commodities c ON s.base_currency_id = c.id
             LEFT JOIN fx_rate_sources src ON s.default_source_id = src.id
             WHERE s.book_id = ?1
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
        conditions.push(format!("a.from_currency_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("a.to_currency_id = ?{}", params_vec.len()));
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
         FROM fx_rate_source_assignments a
         LEFT JOIN commodities fc ON a.from_currency_id = fc.id
         LEFT JOIN commodities tc ON a.to_currency_id = tc.id
         LEFT JOIN fx_rate_sources s ON a.source_id = s.id
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
        "INSERT INTO fx_rate_source_assignments
            (book_id, from_currency_id, to_currency_id, source_id, effective_from, effective_to, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
             FROM fx_rate_source_assignments a
             LEFT JOIN commodities fc ON a.from_currency_id = fc.id
             LEFT JOIN commodities tc ON a.to_currency_id = tc.id
             LEFT JOIN fx_rate_sources s ON a.source_id = s.id
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

    let mut updates = vec!["updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')".to_string()];
    let mut params_vec: Vec<Box<dyn rusqlite::ToSql>> = vec![];

    if let Some(source_id) = input.source_id {
        params_vec.push(Box::new(source_id));
        updates.push(format!("source_id = ?{}", params_vec.len()));
    }
    if let Some(effective_from) = input.effective_from {
        params_vec.push(Box::new(effective_from));
        updates.push(format!("effective_from = ?{}", params_vec.len()));
    }
    if let Some(effective_to) = input.effective_to {
        params_vec.push(Box::new(effective_to));
        updates.push(format!("effective_to = ?{}", params_vec.len()));
    }

    params_vec.push(Box::new(input.id));
    let sql = format!(
        "UPDATE fx_rate_source_assignments SET {} WHERE id = ?{}",
        updates.join(", "),
        params_vec.len()
    );

    let params_refs: Vec<&dyn rusqlite::ToSql> = params_vec.iter().map(|p| p.as_ref()).collect();
    conn.execute(&sql, params_refs.as_slice()).map_err(|e| e.to_string())?;

    let assignment = conn
        .query_row(
            "SELECT a.id, a.book_id, a.from_currency_id, fc.symbol, a.to_currency_id, tc.symbol,
                    a.source_id, s.name, a.effective_from, a.effective_to, a.created_at, a.updated_at
             FROM fx_rate_source_assignments a
             LEFT JOIN commodities fc ON a.from_currency_id = fc.id
             LEFT JOIN commodities tc ON a.to_currency_id = tc.id
             LEFT JOIN fx_rate_sources s ON a.source_id = s.id
             WHERE a.id = ?1",
            [input.id],
            map_fx_rate_source_assignment_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(assignment)
}

#[command]
pub fn delete_fx_rate_source_assignment(db: State<DbState>, id: i64) -> Result<(), String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute("DELETE FROM fx_rate_source_assignments WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

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
        conditions.push(format!("r.from_currency_id = ?{}", params_vec.len()));
    }
    if let Some(to_id) = to_currency_id {
        params_vec.push(Box::new(to_id));
        conditions.push(format!("r.to_currency_id = ?{}", params_vec.len()));
    }
    if let Some(source_id) = source_id {
        params_vec.push(Box::new(source_id));
        conditions.push(format!("r.source_id = ?{}", params_vec.len()));
    }

    let sql = format!(
        "SELECT r.id, r.book_id, r.from_currency_id, fc.symbol, r.to_currency_id, tc.symbol,
                r.source_id, s.name, r.last_success_date, r.last_attempt_at, r.last_error,
                r.created_at, r.updated_at
         FROM fx_rate_refresh_state r
         LEFT JOIN commodities fc ON r.from_currency_id = fc.id
         LEFT JOIN commodities tc ON r.to_currency_id = tc.id
         LEFT JOIN fx_rate_sources s ON r.source_id = s.id
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
        "INSERT INTO fx_rate_refresh_state
            (book_id, from_currency_id, to_currency_id, source_id, last_success_date, last_attempt_at, last_error, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))
         ON CONFLICT(book_id, from_currency_id, to_currency_id, source_id) DO UPDATE SET
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
             FROM fx_rate_refresh_state r
             LEFT JOIN commodities fc ON r.from_currency_id = fc.id
             LEFT JOIN commodities tc ON r.to_currency_id = tc.id
             LEFT JOIN fx_rate_sources s ON r.source_id = s.id
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
    use rusqlite::Connection;
    use std::fs;
    use std::path::PathBuf;
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

    #[test]
    fn test_fx_schema_columns_exist() {
        let (conn, _path) = open_test_db();

        let mut columns = Vec::new();
        let mut stmt = conn
            .prepare("PRAGMA table_info(fx_rates_daily)")
            .expect("prepare pragma");
        let rows = stmt
            .query_map([], |row| row.get::<_, String>(1))
            .expect("query pragma");
        for r in rows {
            columns.push(r.expect("column"));
        }

        assert!(columns.contains(&"source_id".to_string()));
        assert!(columns.contains(&"is_derived".to_string()));
        assert!(columns.contains(&"derived_via_currency_id".to_string()));

        let settings_exists: i64 = conn
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='fx_rate_settings'",
                [],
                |row| row.get(0),
            )
            .expect("settings table");
        assert_eq!(settings_exists, 1);

        let refresh_exists: i64 = conn
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='fx_rate_refresh_state'",
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
            .query_row("SELECT COUNT(*) FROM fx_rate_settings", [], |row| row.get(0))
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
                "SELECT id FROM fx_rate_sources WHERE name='ECB' LIMIT 1",
                [],
                |row| row.get(0),
            )
            .expect("source id");

        conn.execute(
            "INSERT INTO fx_rate_refresh_state
             (book_id, from_currency_id, to_currency_id, source_id, last_success_date, last_attempt_at, last_error)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
            params![SINGLE_BOOK_ID, from_id, to_id, source_id, "2025-12-30", "2025-12-31T00:00:00Z", "oops"],
        )
        .expect("insert refresh state");

        let last_success: String = conn
            .query_row(
                "SELECT last_success_date FROM fx_rate_refresh_state WHERE source_id = ?1",
                [source_id],
                |row| row.get(0),
            )
            .expect("read refresh state");

        assert_eq!(last_success, "2025-12-30");
    }
}

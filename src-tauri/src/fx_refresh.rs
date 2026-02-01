use chrono::{Datelike, NaiveDate, Utc, Weekday};
use rusqlite::{params, OptionalExtension};
use std::collections::HashMap;
use std::time::Duration;
use tauri::{async_runtime, command, AppHandle, Manager, State};
use tokio::time::sleep;

use crate::fx_rates::{provider_by_name, FxRatePoint, ProviderRequest};
use crate::state::{DbState, FxSchedulerState};

const SINGLE_BOOK_ID: i64 = 1;

#[derive(Clone, Debug, serde::Serialize)]
pub struct FxRefreshSummary {
    pub pairs_total: usize,
    pub pairs_success: usize,
    pub pairs_failed: usize,
    pub rates_inserted: usize,
    pub derived_inserted: usize,
    pub last_error: Option<String>,
}

#[derive(Clone, Debug)]
struct FxSettingsRow {
    base_currency_id: i64,
    base_symbol: String,
    default_source_id: Option<i64>,
    refresh_enabled: bool,
    refresh_hour_utc: i64,
    refresh_minute_utc: i64,
    max_backfill_days: i64,
    weekend_policy: String,
}

#[derive(Clone, Debug)]
struct PairTask {
    from_id: i64,
    from_symbol: String,
    to_id: i64,
    source_id: i64,
    source_name: String,
    start_date: String,
    end_date: String,
}

#[command]
pub async fn refresh_fx_rates_now(db: State<'_, DbState>) -> Result<FxRefreshSummary, String> {
    refresh_fx_rates_internal(&db).await
}

#[command]
pub fn restart_fx_rate_scheduler(app: AppHandle, scheduler: State<FxSchedulerState>) {
    restart_fx_scheduler(app, scheduler);
}

pub fn restart_fx_scheduler(app: AppHandle, scheduler: State<FxSchedulerState>) {
    if let Ok(mut guard) = scheduler.task.lock() {
        if let Some(task) = guard.take() {
            task.abort();
        }

        let settings = {
            let db_state = app.state::<DbState>();
            match load_fx_settings(&db_state) {
                Ok(Some(s)) => s,
                _ => return,
            }
        };

        if !settings.refresh_enabled {
            return;
        }

        let app_handle = app.clone();
        let handle = async_runtime::spawn(async move {
            loop {
                let delay = next_refresh_delay(&settings);
                sleep(delay).await;
                let db_state = app_handle.state::<DbState>();
                let _ = refresh_fx_rates_internal(&db_state).await;
            }
        });

        *guard = Some(handle);
    }
}

fn next_refresh_delay(settings: &FxSettingsRow) -> Duration {
    let now = Utc::now();
    let today = now.date_naive();
    let scheduled_today = today
        .and_hms_opt(settings.refresh_hour_utc as u32, settings.refresh_minute_utc as u32, 0)
        .unwrap_or_else(|| today.and_hms_opt(4, 0, 0).unwrap());

    let next = if now.naive_utc() < scheduled_today {
        scheduled_today
    } else {
        scheduled_today + chrono::Duration::days(1)
    };

    let diff = next - now.naive_utc();
    diff.to_std().unwrap_or(Duration::from_secs(60))
}

async fn refresh_fx_rates_internal(db: &DbState) -> Result<FxRefreshSummary, String> {
    let (settings, tasks, symbol_to_id) = prepare_refresh_tasks(db)?;
    let mut summary = FxRefreshSummary {
        pairs_total: tasks.len(),
        pairs_success: 0,
        pairs_failed: 0,
        rates_inserted: 0,
        derived_inserted: 0,
        last_error: None,
    };

    for task in tasks {
        match refresh_pair(db, &settings, &task, &symbol_to_id).await {
            Ok((inserted, derived)) => {
                summary.pairs_success += 1;
                summary.rates_inserted += inserted;
                summary.derived_inserted += derived;
            }
            Err(err) => {
                summary.pairs_failed += 1;
                let err_msg = err.clone();
                summary.last_error = Some(err_msg.clone());
                let _ = record_refresh_error(db, &task, &err_msg);
            }
        }
    }

    Ok(summary)
}

fn prepare_refresh_tasks(
    db: &DbState,
) -> Result<(FxSettingsRow, Vec<PairTask>, HashMap<String, i64>), String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let settings = load_fx_settings_row(conn)?
        .ok_or_else(|| "FX settings not configured".to_string())?;

    let mut symbol_to_id = HashMap::new();
    let mut active_currencies: Vec<(i64, String)> = Vec::new();

    let mut stmt = conn
        .prepare(
            "SELECT id, symbol, is_active FROM commodities WHERE book_id = ?1 AND kind = 'currency'",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([SINGLE_BOOK_ID], |row| {
            let id: i64 = row.get(0)?;
            let symbol: String = row.get(1)?;
            let is_active: i64 = row.get(2)?;
            Ok((id, symbol, is_active != 0))
        })
        .map_err(|e| e.to_string())?;

    for row in rows {
        let (id, symbol, is_active) = row.map_err(|e| e.to_string())?;
        symbol_to_id.insert(symbol.clone(), id);
        if is_active {
            active_currencies.push((id, symbol));
        }
    }

    symbol_to_id.insert(settings.base_symbol.clone(), settings.base_currency_id);

    let today = adjust_for_weekend(&settings.weekend_policy, Utc::now().date_naive());
    let today_str = today.format("%Y-%m-%d").to_string();

    let mut tasks = Vec::new();
    for (from_id, from_symbol) in active_currencies {
        if from_id == settings.base_currency_id {
            continue;
        }
        let (source_id, source_name) = resolve_source_for_pair(conn, from_id, settings.base_currency_id, &today_str, settings.default_source_id)?;
        let last_success = get_last_success_date(conn, from_id, settings.base_currency_id, source_id)?;

        let start_date = if let Some(last) = last_success {
            let last_date = NaiveDate::parse_from_str(&last, "%Y-%m-%d").map_err(|e| e.to_string())?;
            let next = last_date + chrono::Duration::days(1);
            next.format("%Y-%m-%d").to_string()
        } else {
            let start = today - chrono::Duration::days(settings.max_backfill_days.max(1));
            start.format("%Y-%m-%d").to_string()
        };

        if start_date > today_str {
            continue;
        }

        tasks.push(PairTask {
            from_id,
            from_symbol,
            to_id: settings.base_currency_id,
            source_id,
            source_name,
            start_date,
            end_date: today_str.clone(),
        });
    }

    Ok((settings, tasks, symbol_to_id))
}

async fn refresh_pair(
    db: &DbState,
    settings: &FxSettingsRow,
    task: &PairTask,
    symbol_to_id: &HashMap<String, i64>,
) -> Result<(usize, usize), String> {
    let provider = provider_by_name(&task.source_name)
        .ok_or_else(|| format!("No provider for source '{}'", task.source_name))?;

    let (points, derived) = fetch_rates_for_pair(settings, task, provider.as_ref()).await?;

    let mut inserted = 0usize;
    let mut derived_inserted = 0usize;

    let now = Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Millis, true);

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let tx = conn.transaction().map_err(|e| e.to_string())?;

    for rate in points.iter() {
        if let (Some(from_id), Some(to_id)) = (
            symbol_to_id.get(&rate.base).copied(),
            symbol_to_id.get(&rate.quote).copied(),
        ) {
            tx.execute(
                "INSERT OR REPLACE INTO fx_rates_daily
                 (book_id, from_currency_id, to_currency_id, rate_date, rate, source, source_id, is_derived, derived_via_currency_id, created_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![
                    SINGLE_BOOK_ID,
                    from_id,
                    to_id,
                    rate.date,
                    rate.rate,
                    rate.source_name,
                    task.source_id,
                    rate.is_derived as i64,
                    rate.derived_via.as_ref().and_then(|s| symbol_to_id.get(s).copied()),
                ],
            )
            .map_err(|e| e.to_string())?;
            inserted += 1;
        }
    }

    for rate in derived.iter() {
        if let (Some(from_id), Some(to_id)) = (
            symbol_to_id.get(&rate.base).copied(),
            symbol_to_id.get(&rate.quote).copied(),
        ) {
            tx.execute(
                "INSERT OR REPLACE INTO fx_rates_daily
                 (book_id, from_currency_id, to_currency_id, rate_date, rate, source, source_id, is_derived, derived_via_currency_id, created_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![
                    SINGLE_BOOK_ID,
                    from_id,
                    to_id,
                    rate.date,
                    rate.rate,
                    rate.source_name,
                    task.source_id,
                    1i64,
                    rate.derived_via.as_ref().and_then(|s| symbol_to_id.get(s).copied()),
                ],
            )
            .map_err(|e| e.to_string())?;
            derived_inserted += 1;
        }
    }

    tx.execute(
        "INSERT INTO fx_rate_refresh_state
         (book_id, from_currency_id, to_currency_id, source_id, last_success_date, last_attempt_at, last_error, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))
         ON CONFLICT(book_id, from_currency_id, to_currency_id, source_id) DO UPDATE SET
            last_success_date = excluded.last_success_date,
            last_attempt_at = excluded.last_attempt_at,
            last_error = NULL,
            updated_at = excluded.updated_at",
        params![
            SINGLE_BOOK_ID,
            task.from_id,
            task.to_id,
            task.source_id,
            task.end_date,
            now,
        ],
    )
    .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;

    Ok((inserted, derived_inserted))
}

fn record_refresh_error(db: &DbState, task: &PairTask, err: &str) -> Result<(), String> {
    let now = Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Millis, true);
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    conn.execute(
        "INSERT INTO fx_rate_refresh_state
         (book_id, from_currency_id, to_currency_id, source_id, last_success_date, last_attempt_at, last_error, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, NULL, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))
         ON CONFLICT(book_id, from_currency_id, to_currency_id, source_id) DO UPDATE SET
            last_attempt_at = excluded.last_attempt_at,
            last_error = excluded.last_error,
            updated_at = excluded.updated_at",
        params![
            SINGLE_BOOK_ID,
            task.from_id,
            task.to_id,
            task.source_id,
            now,
            err,
        ],
    )
    .map_err(|e| e.to_string())?;
    Ok(())
}

async fn fetch_rates_for_pair(
    settings: &FxSettingsRow,
    task: &PairTask,
    provider: &dyn crate::fx_rates::FxRateProvider,
) -> Result<(Vec<FxRatePoint>, Vec<FxRatePoint>), String> {
    let source_name = task.source_name.clone();
    let mut points = Vec::new();
    let mut derived = Vec::new();

    let base_symbol = settings.base_symbol.clone();
    let quote_symbol = task.from_symbol.clone();

    let provider_name = provider.name();

    let (req_base, symbols, derived_via) = if provider_name.contains("FRED") && base_symbol != "USD" {
        ("USD".to_string(), vec![base_symbol.clone(), quote_symbol.clone()], Some("USD".to_string()))
    } else if provider_name.contains("Bank of Canada") && base_symbol != "CAD" {
        ("CAD".to_string(), vec![base_symbol.clone(), quote_symbol.clone()], Some("CAD".to_string()))
    } else {
        (base_symbol.clone(), vec![quote_symbol.clone()], None)
    };

    let req = ProviderRequest {
        base: req_base.clone(),
        symbols,
        start_date: task.start_date.clone(),
        end_date: task.end_date.clone(),
    };

    let mut fetched = provider.fetch_daily_rates(req).await?;
    for r in fetched.iter_mut() {
        r.source_name = source_name.clone();
    }
    points.extend(fetched);

    if let Some(via) = derived_via {
        derived = derive_from_via(points.as_slice(), &via, &base_symbol, &quote_symbol);
    }

    Ok((points, derived))
}

fn derive_from_via(points: &[FxRatePoint], via: &str, base: &str, quote: &str) -> Vec<FxRatePoint> {
    let mut via_to_base: HashMap<String, f64> = HashMap::new();
    let mut via_to_quote: HashMap<String, f64> = HashMap::new();

    for r in points.iter() {
        if r.base == via && r.quote == base {
            via_to_base.insert(r.date.clone(), r.rate);
        }
        if r.base == via && r.quote == quote {
            via_to_quote.insert(r.date.clone(), r.rate);
        }
    }

    let mut derived = Vec::new();
    for (date, quote_rate) in via_to_quote {
        if let Some(base_rate) = via_to_base.get(&date) {
            let rate = quote_rate / base_rate;
            derived.push(FxRatePoint {
                date: date.clone(),
                base: base.to_string(),
                quote: quote.to_string(),
                rate,
                source_name: format!("derived:{}", via),
                source_url: None,
                is_derived: true,
                derived_via: Some(via.to_string()),
            });
        }
    }

    derived
}

fn adjust_for_weekend(policy: &str, date: NaiveDate) -> NaiveDate {
    match policy {
        "skip" => match date.weekday() {
            Weekday::Sat => date - chrono::Duration::days(1),
            Weekday::Sun => date - chrono::Duration::days(2),
            _ => date,
        },
        "fill_previous" => match date.weekday() {
            Weekday::Sat => date - chrono::Duration::days(1),
            Weekday::Sun => date - chrono::Duration::days(2),
            _ => date,
        },
        _ => date,
    }
}

fn load_fx_settings(db: &DbState) -> Result<Option<FxSettingsRow>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    load_fx_settings_row(conn)
}

fn load_fx_settings_row(conn: &rusqlite::Connection) -> Result<Option<FxSettingsRow>, String> {
    conn
        .query_row(
            "SELECT s.base_currency_id, c.symbol, s.default_source_id,
                    s.refresh_enabled, s.refresh_hour_utc, s.refresh_minute_utc, s.max_backfill_days, s.weekend_policy
             FROM fx_rate_settings s
             LEFT JOIN commodities c ON s.base_currency_id = c.id
             WHERE s.book_id = ?1
             LIMIT 1",
            [SINGLE_BOOK_ID],
            |row| {
                let base_currency_id: i64 = row.get(0)?;
                let base_symbol: String = row.get(1)?;
                Ok(FxSettingsRow {
                    base_currency_id,
                    base_symbol,
                    default_source_id: row.get(2)?,
                    refresh_enabled: row.get::<_, i64>(3)? != 0,
                    refresh_hour_utc: row.get(4)?,
                    refresh_minute_utc: row.get(5)?,
                    max_backfill_days: row.get(6)?,
                    weekend_policy: row.get(7)?,
                })
            },
        )
        .optional()
        .map_err(|e| e.to_string())
}

fn resolve_source_for_pair(
    conn: &rusqlite::Connection,
    from_id: i64,
    to_id: i64,
    on_date: &str,
    default_source_id: Option<i64>,
) -> Result<(i64, String), String> {
    let assignment: Option<(i64, String)> = conn
        .query_row(
            "SELECT a.source_id, s.name
             FROM fx_rate_source_assignments a
             JOIN fx_rate_sources s ON a.source_id = s.id
             WHERE a.book_id = ?1
               AND a.from_currency_id = ?2
               AND a.to_currency_id = ?3
               AND a.effective_from <= ?4
               AND (a.effective_to IS NULL OR a.effective_to >= ?4)
             ORDER BY a.effective_from DESC
             LIMIT 1",
            params![SINGLE_BOOK_ID, from_id, to_id, on_date],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if let Some(a) = assignment {
        return Ok(a);
    }

    let default_id = default_source_id.ok_or_else(|| "No default FX source configured".to_string())?;
    let name: String = conn
        .query_row(
            "SELECT name FROM fx_rate_sources WHERE id = ?1",
            [default_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    Ok((default_id, name))
}

fn get_last_success_date(
    conn: &rusqlite::Connection,
    from_id: i64,
    to_id: i64,
    source_id: i64,
) -> Result<Option<String>, String> {
    conn
        .query_row(
            "SELECT last_success_date FROM fx_rate_refresh_state
             WHERE book_id = ?1 AND from_currency_id = ?2 AND to_currency_id = ?3 AND source_id = ?4",
            params![SINGLE_BOOK_ID, from_id, to_id, source_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::open_and_migrate;
    use crate::state::DbStateInner;
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
        temp.push(format!("rekenraam_fx_refresh_{name}_{start}"));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");
        temp
    }

    fn create_db_state() -> DbState {
        let temp = create_temp_dir("db");
        let (conn, db_path) = open_and_migrate(&temp).expect("open and migrate");
        DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path),
                conn: Some(conn),
            }),
        }
    }

    #[test]
    fn test_adjust_for_weekend() {
        let sat = NaiveDate::from_ymd_opt(2025, 1, 4).unwrap();
        let sun = NaiveDate::from_ymd_opt(2025, 1, 5).unwrap();
        let fri = NaiveDate::from_ymd_opt(2025, 1, 3).unwrap();

        assert_eq!(adjust_for_weekend("skip", sat), fri);
        assert_eq!(adjust_for_weekend("skip", sun), fri);
        assert_eq!(adjust_for_weekend("skip", fri), fri);
    }

    #[test]
    fn test_prepare_refresh_tasks_uses_active_currencies_and_last_success() {
        let db = create_db_state();
        let guard = db.inner.lock().expect("lock");
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
        let source_id: i64 = conn
            .query_row(
                "SELECT id FROM fx_rate_sources WHERE name='ECB' LIMIT 1",
                [],
                |row| row.get(0),
            )
            .expect("source id");

        conn.execute(
            "UPDATE commodities SET is_active = 1 WHERE id = ?1",
            [eur_id],
        )
        .expect("activate eur");

        let today = adjust_for_weekend("skip", Utc::now().date_naive());
        let last_success = (today - chrono::Duration::days(2)).format("%Y-%m-%d").to_string();

        conn.execute(
            "INSERT INTO fx_rate_refresh_state (book_id, from_currency_id, to_currency_id, source_id, last_success_date, last_attempt_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![SINGLE_BOOK_ID, eur_id, usd_id, source_id, last_success, "2025-01-01T00:00:00Z"],
        )
        .expect("insert refresh state");

        drop(guard);

        let (_settings, tasks, _symbol_map) = prepare_refresh_tasks(&db).expect("prepare");
        assert!(!tasks.is_empty());

        let task = tasks.iter().find(|t| t.from_id == eur_id).expect("eur task");
        let expected_start = (today - chrono::Duration::days(1)).format("%Y-%m-%d").to_string();
        assert_eq!(task.start_date, expected_start);
    }
}

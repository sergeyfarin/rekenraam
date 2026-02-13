use std::collections::HashMap;

use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::state::DbState;

const SINGLE_BOOK_ID: i64 = 1;

#[derive(Serialize, Deserialize, Debug)]
pub struct Commodity {
    pub id: i64,
    pub book_id: i64,
    pub kind: String,
    pub symbol: Option<String>,
    pub name: String,
    pub scale: i64,
    pub metadata: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CommodityRenameInput {
    pub id: i64,
    pub book_id: i64,
    pub symbol: Option<String>,
    pub name: String,
    pub metadata: Option<String>,
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

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct DividendIncomeCategory {
    pub id: i64,
    pub book_id: i64,
    pub category_id: i64,
    pub commodity_id: Option<i64>,
    pub tax_withheld_minor: Option<i64>,
    pub notes: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct DividendIncomeCategoryCreate {
    pub book_id: i64,
    pub category_id: i64,
    pub commodity_id: Option<i64>,
    pub tax_withheld_minor: Option<i64>,
    pub notes: Option<String>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct DividendIncomeCategoryUpdate {
    pub id: i64,
    pub book_id: i64,
    pub category_id: i64,
    pub commodity_id: Option<i64>,
    pub tax_withheld_minor: Option<i64>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct DividendInput {
    pub book_id: i64,
    pub txn_date: String,
    pub cash_account_id: i64,
    pub income_account_id: i64,
    pub amount_minor: i64,
    pub memo: Option<String>,
    pub payee_id: Option<i64>,
    pub status: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct DividendResult {
    pub transaction_id: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReinvestDividendInput {
    pub book_id: i64,
    pub txn_date: String,
    pub commodity_id: i64,
    pub investment_account_id: i64,
    pub cash_account_id: i64,
    pub income_account_id: i64,
    pub quantity_minor: i64,
    pub cash_amount_minor: i64,
    pub memo: Option<String>,
    pub payee_id: Option<i64>,
    pub status: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReinvestDividendResult {
    pub dividend_transaction_id: i64,
    pub buy_transaction_id: i64,
    pub lot_id: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct BuyCommodityInput {
    pub book_id: i64,
    pub txn_date: String,
    pub commodity_id: i64,
    pub investment_account_id: i64,
    pub cash_account_id: i64,
    pub quantity_minor: i64,
    pub cash_amount_minor: i64,
    pub memo: Option<String>,
    pub payee_id: Option<i64>,
    pub status: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct SellLotAllocationInput {
    pub lot_id: i64,
    pub quantity_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct SellCommodityInput {
    pub book_id: i64,
    pub txn_date: String,
    pub commodity_id: i64,
    pub investment_account_id: i64,
    pub cash_account_id: i64,
    pub quantity_minor: i64,
    pub cash_amount_minor: i64,
    pub lot_strategy: String,
    pub lot_allocations: Option<Vec<SellLotAllocationInput>>,
    pub allow_short: bool,
    pub memo: Option<String>,
    pub payee_id: Option<i64>,
    pub status: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct TradeAllocation {
    pub lot_id: i64,
    pub quantity_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct TradeResult {
    pub transaction_id: i64,
    pub allocations: Vec<TradeAllocation>,
    pub lot_id: Option<i64>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct LotHoldingPeriod {
    pub lot_id: i64,
    pub account_id: i64,
    pub commodity_id: i64,
    pub opened_date: Option<String>,
    pub balance_minor: i64,
    pub holding_days: Option<i64>,
    pub holding_term: Option<String>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct SellGainsValidationResult {
    pub tx_id: i64,
    pub proceeds_minor: i64,
    pub cost_basis_minor: i64,
    pub gain_loss_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PositionLot {
    pub lot_id: i64,
    pub opened_date: Option<String>,
    pub quantity_minor: i64,
    pub cost_basis_minor: i64,
    pub remaining_cost_basis_minor: i64,
    pub converted_value_minor: Option<i64>,
    pub converted_cost_basis_minor: Option<i64>,
    pub price_missing: Option<bool>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Position {
    pub account_id: i64,
    pub account_name: String,
    pub account_type: String,
    pub commodity_id: i64,
    pub commodity_name: String,
    pub commodity_scale: i64,
    pub balance_minor: i64,
    pub lots: Vec<PositionLot>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ConvertedPosition {
    pub account_id: i64,
    pub account_name: String,
    pub account_type: String,
    pub commodity_id: i64,
    pub commodity_name: String,
    pub commodity_scale: i64,
    pub balance_minor: i64,
    pub value_minor: i64,
    pub price_missing: bool,
    pub lots: Vec<PositionLot>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct AccountPositionTotal {
    pub account_id: i64,
    pub account_name: String,
    pub account_type: String,
    pub total_value_minor: i64,
    pub price_missing: bool,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct AccountCommodityPositionTotal {
    pub account_id: i64,
    pub account_name: String,
    pub account_type: String,
    pub commodity_id: i64,
    pub commodity_name: String,
    pub commodity_scale: i64,
    pub total_value_minor: i64,
    pub price_missing: bool,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct RealizedGainEntry {
    pub tx_id: i64,
    pub txn_date: String,
    pub commodity_id: i64,
    pub quantity_minor: i64,
    pub proceeds_minor: i64,
    pub quote_commodity_id: Option<i64>,
    pub cost_basis_minor: i64,
    pub gain_loss_minor: i64,
    pub proceeds_missing: bool,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct UnrealizedGainEntry {
    pub account_id: i64,
    pub account_name: String,
    pub account_type: String,
    pub commodity_id: i64,
    pub commodity_name: String,
    pub value_minor: i64,
    pub cost_basis_minor: i64,
    pub unrealized_gain_minor: i64,
    pub price_missing: bool,
}

fn map_commodity_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Commodity> {
    Ok(Commodity {
        id: row.get(0)?,
        book_id: row.get(1)?,
        kind: row.get(2)?,
        symbol: row.get(3)?,
        name: row.get(4)?,
        scale: row.get(5)?,
        metadata: row.get(6)?,
        created_at: row.get(7)?,
        updated_at: row.get(8)?,
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

#[allow(dead_code)]
fn map_dividend_income_category_row(
    row: &rusqlite::Row<'_>,
) -> rusqlite::Result<DividendIncomeCategory> {
    Ok(DividendIncomeCategory {
        id: row.get(0)?,
        book_id: row.get(1)?,
        category_id: row.get(2)?,
        commodity_id: row.get(3)?,
        tax_withheld_minor: row.get(4)?,
        notes: row.get(5)?,
        created_at: row.get(6)?,
        updated_at: row.get(7)?,
    })
}

fn insert_transaction_header(
    conn: &rusqlite::Transaction<'_>,
    book_id: i64,
    txn_date: &str,
    memo: Option<String>,
    payee_id: Option<i64>,
    status: Option<String>,
    reference: Option<String>,
    import_id: Option<String>,
) -> Result<i64, String> {
    let status = status.unwrap_or_else(|| "cleared".to_string());
    conn.execute(
        "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, txn_date, payee_id, memo, status, reference, import_id],
    )
    .map_err(|e| e.to_string())?;

    Ok(conn.last_insert_rowid())
}

fn insert_split(
    conn: &rusqlite::Transaction<'_>,
    tx_id: i64,
    account_id: i64,
    commodity_id: i64,
    amount_minor: i64,
    category_id: Option<i64>,
    memo: Option<String>,
) -> Result<i64, String> {
    conn.execute(
        "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![tx_id, account_id, commodity_id, amount_minor, category_id, memo],
    )
    .map_err(|e| e.to_string())?;
    Ok(conn.last_insert_rowid())
}

fn list_lot_balances(
    conn: &rusqlite::Transaction<'_>,
    account_id: i64,
    commodity_id: i64,
) -> Result<Vec<(i64, Option<String>, i64)>, String> {
    let mut stmt = conn
        .prepare(
            "SELECT l.id, l.opened_date, COALESCE(SUM(sla.quantity_minor), 0) AS balance
             FROM lots l
             LEFT JOIN split_lot_allocations sla ON sla.lot_id = l.id
             WHERE l.account_id = ?1 AND l.commodity_id = ?2
             GROUP BY l.id
             ORDER BY l.opened_date ASC, l.id ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![account_id, commodity_id], |row| {
            Ok((row.get::<_, i64>(0)?, row.get::<_, Option<String>>(1)?, row.get::<_, i64>(2)?))
        })
        .map_err(|e| e.to_string())?;

    let mut lots = Vec::new();
    for row in rows {
        lots.push(row.map_err(|e| e.to_string())?);
    }
    Ok(lots)
}

fn account_commodity_id(
    conn: &rusqlite::Transaction<'_>,
    account_id: i64,
) -> Result<i64, String> {
    let commodity_id = conn
        .query_row(
            "SELECT commodity_id FROM accounts WHERE id = ?1",
            [account_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;
    Ok(commodity_id)
}

fn account_booking_policy(
    conn: &rusqlite::Transaction<'_>,
    account_id: i64,
) -> Result<String, String> {
    let policy: Option<String> = conn
        .query_row(
            "SELECT booking_policy FROM accounts WHERE id = ?1",
            [account_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;
    Ok(policy.unwrap_or_else(|| "fifo".to_string()).to_lowercase())
}


fn plan_average_allocations(
    lots: &[(i64, Option<String>, i64)],
    quantity_minor: i64,
) -> Vec<(i64, i64)> {
    let mut allocations: Vec<(i64, i64)> = Vec::new();
    let total_balance: i64 = lots.iter().map(|(_, _, balance)| *balance).sum();
    if total_balance <= 0 || quantity_minor <= 0 {
        return allocations;
    }

    let mut base_allocs: Vec<(i64, i64, i64)> = Vec::new();
    let mut allocated: i64 = 0;

    for (lot_id, _opened_date, balance) in lots {
        let numerator = (quantity_minor as i128) * (*balance as i128);
        let alloc = (numerator / (total_balance as i128)) as i64;
        let remainder = (numerator % (total_balance as i128)) as i64;
        base_allocs.push((*lot_id, alloc, remainder));
        allocated += alloc;
    }

    let mut remaining = quantity_minor - allocated;
    base_allocs.sort_by(|a, b| b.2.cmp(&a.2));

    for (idx, (lot_id, alloc, _rem)) in base_allocs.iter_mut().enumerate() {
        let bump = if remaining > 0 { 1 } else { 0 };
        if bump == 1 {
            remaining -= 1;
        }
        let final_alloc = *alloc + bump;
        if final_alloc > 0 {
            allocations.push((*lot_id, final_alloc));
        } else if idx == 0 && remaining < 0 {
            // no-op; safeguard
        }
    }

    allocations
}

#[command]
pub fn list_commodities(db: State<DbState>, book_id: i64) -> Result<Vec<Commodity>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, kind, symbol, name, scale, metadata, created_at, updated_at
             FROM commodities WHERE book_id = ?1 ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_commodity_row)
        .map_err(|e| e.to_string())?;

    let mut commodities = Vec::new();
    for row in rows {
        commodities.push(row.map_err(|e| e.to_string())?);
    }

    Ok(commodities)
}

#[command]
pub fn get_commodity(db: State<DbState>, id: i64) -> Result<Option<Commodity>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, kind, symbol, name, scale, metadata, created_at, updated_at
             FROM commodities WHERE id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let mut rows = stmt.query([id]).map_err(|e| e.to_string())?;
    if let Some(row) = rows.next().map_err(|e| e.to_string())? {
        Ok(Some(map_commodity_row(row).map_err(|e| e.to_string())?))
    } else {
        Ok(None)
    }
}

#[command]
pub fn rename_commodity(db: State<DbState>, input: CommodityRenameInput) -> Result<Commodity, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let rows = conn
        .execute(
            "UPDATE commodities
             SET symbol = ?2, name = ?3, metadata = ?4,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1 AND book_id = ?5",
            params![input.id, input.symbol, input.name, input.metadata, book_id],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("commodity not found".to_string());
    }

    let commodity = conn
        .query_row(
            "SELECT id, book_id, kind, symbol, name, scale, metadata, created_at, updated_at
             FROM commodities WHERE id = ?1",
            [input.id],
            map_commodity_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(commodity)
}

#[command]
pub fn list_corporate_actions(
    db: State<DbState>,
    book_id: i64,
    commodity_id: Option<i64>,
) -> Result<Vec<CorporateAction>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

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
    let book_id = SINGLE_BOOK_ID;
    let (action_id, transaction_id) = {
        let result: Result<(i64, Option<i64>), String> = (|| {
            let tx = conn.transaction().map_err(|e| e.to_string())?;

            let equity_account_id: i64 = tx
                .query_row(
                    "SELECT id FROM accounts
                 WHERE book_id = ?1 AND commodity_id = ?2 AND type = 'equity' AND name = 'Corporate Actions'
                 LIMIT 1",
                    params![book_id, input.commodity_id],
                    |row| row.get(0),
                )
                .optional()
                .map_err(|e| e.to_string())?
                .unwrap_or_else(|| 0);

            let equity_account_id = if equity_account_id == 0 {
                tx.execute(
                    "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, is_closed, created_at, updated_at)
                 VALUES (?1, NULL, 'equity', 'Corporate Actions', ?2, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![book_id, input.commodity_id],
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
                    .query_map(params![book_id, input.commodity_id], |row| {
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
                    params![book_id, input.effective_date, memo],
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
                    book_id,
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
pub fn create_dividend_transaction(
    db: State<DbState>,
    input: DividendInput,
) -> Result<DividendResult, String> {
    if input.amount_minor <= 0 {
        return Err("dividend amount must be positive".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let tx_id = insert_transaction_header(
        &tx,
        book_id,
        &input.txn_date,
        input.memo.clone(),
        input.payee_id,
        input.status,
        None,
        None,
    )?;

    let cash_commodity_id = account_commodity_id(&tx, input.cash_account_id)?;
    let income_commodity_id = account_commodity_id(&tx, input.income_account_id)?;

    insert_split(
        &tx,
        tx_id,
        input.cash_account_id,
        cash_commodity_id,
        input.amount_minor,
        None,
        None,
    )?;
    insert_split(
        &tx,
        tx_id,
        input.income_account_id,
        income_commodity_id,
        -input.amount_minor,
        None,
        None,
    )?;

    tx.commit().map_err(|e| e.to_string())?;

    Ok(DividendResult {
        transaction_id: tx_id,
    })
}

#[command]
pub fn create_reinvest_dividend(
    db: State<DbState>,
    input: ReinvestDividendInput,
) -> Result<ReinvestDividendResult, String> {
    if input.quantity_minor <= 0 || input.cash_amount_minor <= 0 {
        return Err("quantity and cash amount must be positive".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let dividend_tx_id = insert_transaction_header(
        &tx,
        book_id,
        &input.txn_date,
        input.memo.clone(),
        input.payee_id,
        input.status.clone(),
        None,
        None,
    )?;

    let cash_commodity_id = account_commodity_id(&tx, input.cash_account_id)?;
    let income_commodity_id = account_commodity_id(&tx, input.income_account_id)?;

    insert_split(
        &tx,
        dividend_tx_id,
        input.cash_account_id,
        cash_commodity_id,
        input.cash_amount_minor,
        None,
        None,
    )?;
    insert_split(
        &tx,
        dividend_tx_id,
        input.income_account_id,
        income_commodity_id,
        -input.cash_amount_minor,
        None,
        None,
    )?;

    let buy_tx_id = insert_transaction_header(
        &tx,
        book_id,
        &input.txn_date,
        input.memo.clone(),
        input.payee_id,
        input.status,
        None,
        None,
    )?;

    let investment_commodity_id = account_commodity_id(&tx, input.investment_account_id)?;
    if investment_commodity_id != input.commodity_id {
        return Err("investment account commodity does not match input commodity".to_string());
    }

    let security_split_id = insert_split(
        &tx,
        buy_tx_id,
        input.investment_account_id,
        input.commodity_id,
        input.quantity_minor,
        None,
        None,
    )?;

    insert_split(
        &tx,
        buy_tx_id,
        input.cash_account_id,
        cash_commodity_id,
        -input.cash_amount_minor,
        None,
        None,
    )?;

    tx.execute(
        "INSERT INTO lots (book_id, account_id, commodity_id, opened_date, cost_basis_minor, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.investment_account_id,
            input.commodity_id,
            input.txn_date,
            input.cash_amount_minor,
        ],
    )
    .map_err(|e| e.to_string())?;

    let lot_id = tx.last_insert_rowid();

    tx.execute(
        "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
         VALUES (?1, ?2, ?3)",
        params![security_split_id, lot_id, input.quantity_minor],
    )
    .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;

    Ok(ReinvestDividendResult {
        dividend_transaction_id: dividend_tx_id,
        buy_transaction_id: buy_tx_id,
        lot_id,
    })
}

#[command]
pub fn buy_commodity(
    db: State<DbState>,
    input: BuyCommodityInput,
) -> Result<TradeResult, String> {
    if input.quantity_minor <= 0 || input.cash_amount_minor <= 0 {
        return Err("quantity and cash amount must be positive".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let tx_id = insert_transaction_header(
        &tx,
        book_id,
        &input.txn_date,
        input.memo,
        input.payee_id,
        input.status,
        None,
        None,
    )?;

    let investment_commodity_id = account_commodity_id(&tx, input.investment_account_id)?;
    if investment_commodity_id != input.commodity_id {
        return Err("investment account commodity does not match input commodity".to_string());
    }

    let cash_commodity_id = account_commodity_id(&tx, input.cash_account_id)?;

    let security_split_id = insert_split(
        &tx,
        tx_id,
        input.investment_account_id,
        input.commodity_id,
        input.quantity_minor,
        None,
        None,
    )?;

    insert_split(
        &tx,
        tx_id,
        input.cash_account_id,
        cash_commodity_id,
        -input.cash_amount_minor,
        None,
        None,
    )?;

    tx.execute(
        "INSERT INTO lots (book_id, account_id, commodity_id, opened_date, cost_basis_minor, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.investment_account_id,
            input.commodity_id,
            input.txn_date,
            input.cash_amount_minor,
        ],
    )
    .map_err(|e| e.to_string())?;

    let lot_id = tx.last_insert_rowid();

    tx.execute(
        "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
         VALUES (?1, ?2, ?3)",
        params![security_split_id, lot_id, input.quantity_minor],
    )
    .map_err(|e| e.to_string())?;

    tx.commit().map_err(|e| e.to_string())?;

    Ok(TradeResult {
        transaction_id: tx_id,
        allocations: vec![TradeAllocation {
            lot_id,
            quantity_minor: input.quantity_minor,
        }],
        lot_id: Some(lot_id),
    })
}

#[command]
pub fn sell_commodity(
    db: State<DbState>,
    input: SellCommodityInput,
) -> Result<TradeResult, String> {
    if input.quantity_minor <= 0 || input.cash_amount_minor <= 0 {
        return Err("quantity and cash amount must be positive".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let tx_id = insert_transaction_header(
        &tx,
        book_id,
        &input.txn_date,
        input.memo,
        input.payee_id,
        input.status,
        None,
        None,
    )?;

    let investment_commodity_id = account_commodity_id(&tx, input.investment_account_id)?;
    if investment_commodity_id != input.commodity_id {
        return Err("investment account commodity does not match input commodity".to_string());
    }

    let cash_commodity_id = account_commodity_id(&tx, input.cash_account_id)?;

    let security_split_id = insert_split(
        &tx,
        tx_id,
        input.investment_account_id,
        input.commodity_id,
        -input.quantity_minor,
        None,
        None,
    )?;

    insert_split(
        &tx,
        tx_id,
        input.cash_account_id,
        cash_commodity_id,
        input.cash_amount_minor,
        None,
        None,
    )?;

    let policy = account_booking_policy(&tx, input.investment_account_id)?;
    let mut requested = input.lot_strategy.trim().to_lowercase();
    if requested.is_empty() || requested == "default" {
        requested = policy.clone();
    }

    if policy == "strict" {
        if requested != "custom" {
            return Err("strict booking requires custom lot allocations".to_string());
        }
    } else if requested != "fifo" && requested != "lifo" && requested != "custom" && requested != "average" {
        return Err("lot strategy must be fifo, lifo, average, or custom".to_string());
    }

    let strategy = requested;

    if strategy == "custom" && policy == "strict" && input.allow_short {
        return Err("strict booking does not allow short allocations".to_string());
    }
    let mut allocations: Vec<TradeAllocation> = Vec::new();

    if strategy == "custom" {
        let custom = input.lot_allocations.ok_or_else(|| "custom allocations required".to_string())?;
        let total: i64 = custom.iter().map(|a| a.quantity_minor).sum();
        if total != input.quantity_minor {
            return Err("custom allocations must sum to quantity".to_string());
        }

        for alloc in custom {
            if alloc.quantity_minor <= 0 {
                return Err("allocation quantities must be positive".to_string());
            }

            let balance: Option<i64> = tx
                .query_row(
                    "SELECT COALESCE(SUM(quantity_minor), 0) FROM split_lot_allocations WHERE lot_id = ?1",
                    [alloc.lot_id],
                    |row| row.get(0),
                )
                .optional()
                .map_err(|e| e.to_string())?;

            let balance = balance.unwrap_or(0);
            if alloc.quantity_minor > balance && !input.allow_short {
                return Err("insufficient lot quantity for custom allocation".to_string());
            }

            tx.execute(
                "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
                 VALUES (?1, ?2, ?3)",
                params![security_split_id, alloc.lot_id, -alloc.quantity_minor],
            )
            .map_err(|e| e.to_string())?;

            allocations.push(TradeAllocation {
                lot_id: alloc.lot_id,
                quantity_minor: alloc.quantity_minor,
            });
        }
    } else if strategy == "average" {
        let mut lots = list_lot_balances(&tx, input.investment_account_id, input.commodity_id)?;
        lots.retain(|(_, _, balance)| *balance > 0);

        let avg_allocs = plan_average_allocations(&lots, input.quantity_minor);
        let mut remaining: i64 = input.quantity_minor;

        for (lot_id, alloc_qty) in avg_allocs {
            if alloc_qty <= 0 {
                continue;
            }
            tx.execute(
                "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
                 VALUES (?1, ?2, ?3)",
                params![security_split_id, lot_id, -alloc_qty],
            )
            .map_err(|e| e.to_string())?;
            allocations.push(TradeAllocation {
                lot_id,
                quantity_minor: alloc_qty,
            });
            remaining -= alloc_qty;
        }

        if remaining > 0 {
            if !input.allow_short {
                return Err("insufficient lots available for sale".to_string());
            }

            tx.execute(
                "INSERT INTO lots (book_id, account_id, commodity_id, opened_date, notes, cost_basis_minor, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, 'Short sale', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![book_id, input.investment_account_id, input.commodity_id, input.txn_date],
            )
            .map_err(|e| e.to_string())?;

            let lot_id = tx.last_insert_rowid();
            tx.execute(
                "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
                 VALUES (?1, ?2, ?3)",
                params![security_split_id, lot_id, -remaining],
            )
            .map_err(|e| e.to_string())?;

            allocations.push(TradeAllocation {
                lot_id,
                quantity_minor: remaining,
            });
        }
    } else {
        let mut remaining = input.quantity_minor;
        let mut lots = list_lot_balances(&tx, input.investment_account_id, input.commodity_id)?;

        lots.retain(|(_, _, balance)| *balance > 0);
        if strategy == "lifo" {
            lots.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| b.0.cmp(&a.0)));
        }

        for (lot_id, _opened_date, balance) in &lots {
            if remaining <= 0 {
                break;
            }

            let take = std::cmp::min(remaining, *balance);
            tx.execute(
                "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
                 VALUES (?1, ?2, ?3)",
                params![security_split_id, lot_id, -take],
            )
            .map_err(|e| e.to_string())?;
            allocations.push(TradeAllocation {
                lot_id: *lot_id,
                quantity_minor: take,
            });
            remaining -= take;
        }

        if remaining > 0 {
            if !input.allow_short {
                return Err("insufficient lots available for sale".to_string());
            }

            tx.execute(
                "INSERT INTO lots (book_id, account_id, commodity_id, opened_date, notes, cost_basis_minor, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, 'Short sale', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![book_id, input.investment_account_id, input.commodity_id, input.txn_date],
            )
            .map_err(|e| e.to_string())?;

            let lot_id = tx.last_insert_rowid();
            tx.execute(
                "INSERT INTO split_lot_allocations (split_id, lot_id, quantity_minor)
                 VALUES (?1, ?2, ?3)",
                params![security_split_id, lot_id, -remaining],
            )
            .map_err(|e| e.to_string())?;

            allocations.push(TradeAllocation {
                lot_id,
                quantity_minor: remaining,
            });
        }
    }

    tx.commit().map_err(|e| e.to_string())?;

    Ok(TradeResult {
        transaction_id: tx_id,
        allocations,
        lot_id: None,
    })
}

#[allow(dead_code)]
#[command]
pub fn list_lots_with_holding_period(
    db: State<DbState>,
    account_id: i64,
    commodity_id: i64,
    as_of_date: Option<String>,
) -> Result<Vec<LotHoldingPeriod>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT
                l.id,
                l.account_id,
                l.commodity_id,
                l.opened_date,
                COALESCE(SUM(sla.quantity_minor), 0) AS balance_minor,
                CASE
                    WHEN l.opened_date IS NULL THEN NULL
                    ELSE CAST(julianday(COALESCE(?1, date('now'))) - julianday(l.opened_date) AS INTEGER)
                END AS holding_days,
                CASE
                    WHEN l.opened_date IS NULL THEN NULL
                    WHEN (julianday(COALESCE(?1, date('now'))) - julianday(l.opened_date)) >= 365 THEN 'long'
                    ELSE 'short'
                END AS holding_term
             FROM lots l
             LEFT JOIN split_lot_allocations sla ON sla.lot_id = l.id
             WHERE l.account_id = ?2 AND l.commodity_id = ?3
             GROUP BY l.id
             ORDER BY l.opened_date ASC, l.id ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![as_of_date, account_id, commodity_id], |row| {
            Ok(LotHoldingPeriod {
                lot_id: row.get(0)?,
                account_id: row.get(1)?,
                commodity_id: row.get(2)?,
                opened_date: row.get(3)?,
                balance_minor: row.get(4)?,
                holding_days: row.get(5)?,
                holding_term: row.get(6)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut lots = Vec::new();
    for row in rows {
        lots.push(row.map_err(|e| e.to_string())?);
    }

    Ok(lots)
}

#[allow(dead_code)]
#[command]
pub fn validate_sell_gains(db: State<DbState>, tx_id: i64) -> Result<SellGainsValidationResult, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let proceeds: i64 = conn
        .query_row(
            "SELECT COALESCE(SUM(CASE WHEN amount_minor > 0 THEN amount_minor ELSE 0 END), 0)
             FROM splits WHERE tx_id = ?1",
            [tx_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT sla.lot_id, ABS(sla.quantity_minor) AS qty
             FROM split_lot_allocations sla
             JOIN splits s ON s.id = sla.split_id
             WHERE s.tx_id = ?1 AND sla.quantity_minor < 0",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([tx_id], |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)))
        .map_err(|e| e.to_string())?;

    let mut cost_basis: i64 = 0;
    for row in rows {
        let (lot_id, qty) = row.map_err(|e| e.to_string())?;
        let lot_cost: i64 = conn
            .query_row(
                "SELECT cost_basis_minor FROM lots WHERE id = ?1",
                [lot_id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;

        let total_qty: i64 = conn
            .query_row(
                "SELECT COALESCE(SUM(CASE WHEN quantity_minor > 0 THEN quantity_minor ELSE 0 END), 0)
                 FROM split_lot_allocations WHERE lot_id = ?1",
                [lot_id],
                |r| r.get(0),
            )
            .map_err(|e| e.to_string())?;

        if total_qty <= 0 {
            continue;
        }

        let alloc_cost = ((lot_cost as i128) * (qty as i128)) / (total_qty as i128);
        cost_basis += alloc_cost as i64;
    }

    Ok(SellGainsValidationResult {
        tx_id,
        proceeds_minor: proceeds,
        cost_basis_minor: cost_basis,
        gain_loss_minor: proceeds - cost_basis,
    })
}

#[command]
pub fn get_positions(
    db: State<DbState>,
    book_id: i64,
    as_of_date: Option<String>,
) -> Result<Vec<Position>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT
                a.id AS account_id,
                a.name,
                a.type,
                c.id AS commodity_id,
                c.name,
                c.scale,
                COALESCE(SUM(s.amount_minor), 0) AS balance_minor
             FROM accounts a
             JOIN splits s ON s.account_id = a.id
             JOIN transactions t ON t.id = s.tx_id
             JOIN commodities c ON c.id = s.commodity_id
             WHERE a.book_id = ?1
               AND (?2 IS NULL OR t.txn_date <= ?2)
             GROUP BY a.id, s.commodity_id
             HAVING balance_minor != 0
             ORDER BY a.name ASC, c.name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_date], |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, String>(4)?,
                row.get::<_, i64>(5)?,
                row.get::<_, i64>(6)?,
            ))
        })
        .map_err(|e| e.to_string())?;

    let mut base_positions: Vec<(i64, String, String, i64, String, i64, i64)> = Vec::new();
    for row in rows {
        base_positions.push(row.map_err(|e| e.to_string())?);
    }

    let mut lot_stmt = conn
        .prepare(
            "SELECT
                l.id,
                l.account_id,
                l.commodity_id,
                l.opened_date,
                l.cost_basis_minor,
                COALESCE(SUM(sla.quantity_minor), 0) AS balance_minor,
                COALESCE(SUM(CASE WHEN sla.quantity_minor > 0 THEN sla.quantity_minor ELSE 0 END), 0) AS total_positive
             FROM lots l
             LEFT JOIN split_lot_allocations sla ON sla.lot_id = l.id
             LEFT JOIN splits s ON s.id = sla.split_id
             LEFT JOIN transactions t ON t.id = s.tx_id
             WHERE l.book_id = ?1
               AND (?2 IS NULL OR t.txn_date <= ?2 OR t.txn_date IS NULL)
             GROUP BY l.id
             HAVING balance_minor != 0
             ORDER BY l.opened_date ASC, l.id ASC",
        )
        .map_err(|e| e.to_string())?;

    let lot_rows = lot_stmt
        .query_map(params![book_id, as_of_date], |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, Option<String>>(3)?,
                row.get::<_, i64>(4)?,
                row.get::<_, i64>(5)?,
                row.get::<_, i64>(6)?,
            ))
        })
        .map_err(|e| e.to_string())?;

    let mut lot_map: HashMap<(i64, i64), Vec<PositionLot>> = HashMap::new();
    for row in lot_rows {
        let (lot_id, account_id, commodity_id, opened_date, cost_basis_minor, balance_minor, total_positive) =
            row.map_err(|e| e.to_string())?;
        if balance_minor == 0 {
            continue;
        }
        let remaining_cost_basis_minor = if total_positive > 0 {
            let numer = (cost_basis_minor as i128) * (balance_minor as i128);
            (numer / (total_positive as i128)) as i64
        } else {
            0
        };

        lot_map
            .entry((account_id, commodity_id))
            .or_default()
            .push(PositionLot {
                lot_id,
                opened_date,
                quantity_minor: balance_minor,
                cost_basis_minor,
                remaining_cost_basis_minor,
                converted_value_minor: None,
                converted_cost_basis_minor: None,
                price_missing: None,
            });
    }

    let mut positions = Vec::new();
    for (account_id, account_name, account_type, commodity_id, commodity_name, commodity_scale, balance_minor) in
        base_positions
    {
        let lots = lot_map.remove(&(account_id, commodity_id)).unwrap_or_default();
        positions.push(Position {
            account_id,
            account_name,
            account_type,
            commodity_id,
            commodity_name,
            commodity_scale,
            balance_minor,
            lots,
        });
    }

    Ok(positions)
}

#[command]
pub fn convert_positions(
    db: State<DbState>,
    base_commodity_id: i64,
    as_of_date: Option<String>,
) -> Result<Vec<ConvertedPosition>, String> {
    let prices: HashMap<i64, i64> = {
        let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
        let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

        let mut price_sql = String::from(
            "SELECT cp.commodity_id, cp.price_minor
             FROM commodity_prices cp
             JOIN (
                SELECT commodity_id, MAX(as_of_date) AS max_date
                FROM commodity_prices
                WHERE quote_commodity_id = ?",
        );

        let mut price_params: Vec<Value> = vec![Value::from(base_commodity_id)];
        if let Some(date) = &as_of_date {
            price_sql.push_str(" AND as_of_date <= ?");
            price_params.push(Value::from(date.clone()));
        }
        price_sql.push_str(" GROUP BY commodity_id) latest
             ON latest.commodity_id = cp.commodity_id AND latest.max_date = cp.as_of_date
             WHERE cp.quote_commodity_id = ?");
        price_params.push(Value::from(base_commodity_id));

        let mut stmt = conn.prepare(&price_sql).map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(params_from_iter(price_params), |row| {
                Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?))
            })
            .map_err(|e| e.to_string())?;

        let mut prices: HashMap<i64, i64> = HashMap::new();
        for row in rows {
            let (commodity_id, price_minor) = row.map_err(|e| e.to_string())?;
            prices.insert(commodity_id, price_minor);
        }

        prices
    };

    let positions = get_positions(db, SINGLE_BOOK_ID, as_of_date)?;
    let mut converted = Vec::new();

    for p in positions {
        if p.commodity_id == base_commodity_id {
            let lots = p
                .lots
                .into_iter()
                .map(|mut lot| {
                    lot.converted_value_minor = Some(lot.quantity_minor);
                    lot.converted_cost_basis_minor = Some(lot.remaining_cost_basis_minor);
                    lot.price_missing = Some(false);
                    lot
                })
                .collect();
            converted.push(ConvertedPosition {
                account_id: p.account_id,
                account_name: p.account_name,
                account_type: p.account_type,
                commodity_id: p.commodity_id,
                commodity_name: p.commodity_name,
                commodity_scale: p.commodity_scale,
                balance_minor: p.balance_minor,
                value_minor: p.balance_minor,
                price_missing: false,
                lots,
            });
            continue;
        }

        if let Some(price_minor) = prices.get(&p.commodity_id) {
            let scale_factor: i128 = 10i128.pow(p.commodity_scale as u32);
            let value_minor = ((p.balance_minor as i128) * (*price_minor as i128)) / scale_factor;
            let lots = p
                .lots
                .into_iter()
                .map(|mut lot| {
                    let lot_value = ((lot.quantity_minor as i128) * (*price_minor as i128)) / scale_factor;
                    let lot_cost_value =
                        ((lot.remaining_cost_basis_minor as i128) * (*price_minor as i128)) / scale_factor;
                    lot.converted_value_minor = Some(lot_value as i64);
                    lot.converted_cost_basis_minor = Some(lot_cost_value as i64);
                    lot.price_missing = Some(false);
                    lot
                })
                .collect();
            converted.push(ConvertedPosition {
                account_id: p.account_id,
                account_name: p.account_name,
                account_type: p.account_type,
                commodity_id: p.commodity_id,
                commodity_name: p.commodity_name,
                commodity_scale: p.commodity_scale,
                balance_minor: p.balance_minor,
                value_minor: value_minor as i64,
                price_missing: false,
                lots,
            });
        } else {
            let lots = p
                .lots
                .into_iter()
                .map(|mut lot| {
                    lot.converted_value_minor = None;
                    lot.converted_cost_basis_minor = None;
                    lot.price_missing = Some(true);
                    lot
                })
                .collect();
            converted.push(ConvertedPosition {
                account_id: p.account_id,
                account_name: p.account_name,
                account_type: p.account_type,
                commodity_id: p.commodity_id,
                commodity_name: p.commodity_name,
                commodity_scale: p.commodity_scale,
                balance_minor: p.balance_minor,
                value_minor: 0,
                price_missing: true,
                lots,
            });
        }
    }

    Ok(converted)
}

#[allow(dead_code)]
#[command]
pub fn list_account_position_totals(
    db: State<DbState>,
    base_commodity_id: i64,
    as_of_date: Option<String>,
) -> Result<Vec<AccountPositionTotal>, String> {
    let positions = convert_positions(db, base_commodity_id, as_of_date)?;
    let mut map: HashMap<i64, AccountPositionTotal> = HashMap::new();

    for p in positions {
        let entry = map.entry(p.account_id).or_insert(AccountPositionTotal {
            account_id: p.account_id,
            account_name: p.account_name.clone(),
            account_type: p.account_type.clone(),
            total_value_minor: 0,
            price_missing: false,
        });
        entry.total_value_minor += p.value_minor;
        if p.price_missing {
            entry.price_missing = true;
        }
    }

    let mut totals: Vec<AccountPositionTotal> = map.into_values().collect();
    totals.sort_by(|a, b| a.account_name.cmp(&b.account_name));
    Ok(totals)
}

#[allow(dead_code)]
#[command]
pub fn list_account_position_totals_by_commodity(
    db: State<DbState>,
    base_commodity_id: i64,
    as_of_date: Option<String>,
) -> Result<Vec<AccountCommodityPositionTotal>, String> {
    let positions = convert_positions(db, base_commodity_id, as_of_date)?;
    let mut map: HashMap<(i64, i64), AccountCommodityPositionTotal> = HashMap::new();

    for p in positions {
        let entry = map
            .entry((p.account_id, p.commodity_id))
            .or_insert(AccountCommodityPositionTotal {
                account_id: p.account_id,
                account_name: p.account_name.clone(),
                account_type: p.account_type.clone(),
                commodity_id: p.commodity_id,
                commodity_name: p.commodity_name.clone(),
                commodity_scale: p.commodity_scale,
                total_value_minor: 0,
                price_missing: false,
            });
        entry.total_value_minor += p.value_minor;
        if p.price_missing {
            entry.price_missing = true;
        }
    }

    let mut totals: Vec<AccountCommodityPositionTotal> = map.into_values().collect();
    totals.sort_by(|a, b| {
        a.account_name
            .cmp(&b.account_name)
            .then_with(|| a.commodity_name.cmp(&b.commodity_name))
    });
    Ok(totals)
}

#[allow(dead_code)]
#[command]
pub fn realized_gains_report(
    db: State<DbState>,
    date_from: Option<String>,
    date_to: Option<String>,
) -> Result<Vec<RealizedGainEntry>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut sql = String::from(
        "SELECT t.id, t.txn_date, s.commodity_id, sla.lot_id, ABS(sla.quantity_minor) AS qty
         FROM split_lot_allocations sla
         JOIN splits s ON s.id = sla.split_id
         JOIN transactions t ON t.id = s.tx_id
         WHERE sla.quantity_minor < 0",
    );
    let mut params: Vec<Value> = Vec::new();
    if let Some(date_from) = date_from {
        sql.push_str(" AND t.txn_date >= ?");
        params.push(Value::from(date_from));
    }
    if let Some(date_to) = date_to {
        sql.push_str(" AND t.txn_date <= ?");
        params.push(Value::from(date_to));
    }
    sql.push_str(" ORDER BY t.txn_date ASC, t.id ASC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, i64>(4)?,
            ))
        })
        .map_err(|e| e.to_string())?;

    let mut allocations: Vec<(i64, String, i64, i64, i64)> = Vec::new();
    for row in rows {
        allocations.push(row.map_err(|e| e.to_string())?);
    }

    let mut results: Vec<RealizedGainEntry> = Vec::new();
    for (tx_id, txn_date, commodity_id, lot_id, qty) in allocations {
        let lot_cost: i64 = conn
            .query_row(
                "SELECT cost_basis_minor FROM lots WHERE id = ?1",
                [lot_id],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?;

        let total_qty: i64 = conn
            .query_row(
                "SELECT COALESCE(SUM(CASE WHEN quantity_minor > 0 THEN quantity_minor ELSE 0 END), 0)
                 FROM split_lot_allocations WHERE lot_id = ?1",
                [lot_id],
                |row| row.get(0),
            )
            .map_err(|e| e.to_string())?;

        let cost_basis_minor = if total_qty > 0 {
            let numer = (lot_cost as i128) * (qty as i128);
            (numer / (total_qty as i128)) as i64
        } else {
            0
        };

        let mut cash_stmt = conn
            .prepare(
                "SELECT commodity_id, SUM(amount_minor) AS amt
                 FROM splits
                 WHERE tx_id = ?1 AND commodity_id != ?2 AND amount_minor > 0
                 GROUP BY commodity_id",
            )
            .map_err(|e| e.to_string())?;
        let cash_rows = cash_stmt
            .query_map(params![tx_id, commodity_id], |row| {
                Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?))
            })
            .map_err(|e| e.to_string())?;

        let mut cash: Vec<(i64, i64)> = Vec::new();
        for row in cash_rows {
            cash.push(row.map_err(|e| e.to_string())?);
        }

        let (proceeds_minor, quote_commodity_id, proceeds_missing) = if cash.len() == 1 {
            (cash[0].1, Some(cash[0].0), false)
        } else {
            (0, None, true)
        };

        results.push(RealizedGainEntry {
            tx_id,
            txn_date,
            commodity_id,
            quantity_minor: qty,
            proceeds_minor,
            quote_commodity_id,
            cost_basis_minor,
            gain_loss_minor: proceeds_minor - cost_basis_minor,
            proceeds_missing,
        });
    }

    Ok(results)
}

#[allow(dead_code)]
#[command]
pub fn unrealized_gains_report(
    db: State<DbState>,
    base_commodity_id: i64,
    as_of_date: Option<String>,
) -> Result<Vec<UnrealizedGainEntry>, String> {
    let positions = convert_positions(db, base_commodity_id, as_of_date)?;
    let mut results = Vec::new();

    for p in positions {
        let mut cost_basis_minor: i64 = 0;
        for lot in &p.lots {
            if let Some(cost) = lot.converted_cost_basis_minor {
                cost_basis_minor += cost;
            }
        }

        let unrealized_gain_minor = p.value_minor - cost_basis_minor;
        results.push(UnrealizedGainEntry {
            account_id: p.account_id,
            account_name: p.account_name,
            account_type: p.account_type,
            commodity_id: p.commodity_id,
            commodity_name: p.commodity_name,
            value_minor: p.value_minor,
            cost_basis_minor,
            unrealized_gain_minor,
            price_missing: p.price_missing,
        });
    }

    Ok(results)
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
pub fn add_implicit_price(db: State<DbState>, tx_id: i64) -> Result<Option<CommodityPrice>, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let (txn_date, _book_id): (String, i64) = conn
        .query_row(
            "SELECT txn_date, book_id FROM transactions WHERE id = ?1",
            [tx_id],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .map_err(|e| e.to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT commodity_id, SUM(amount_minor) AS amount_minor
             FROM splits WHERE tx_id = ?1
             GROUP BY commodity_id
             HAVING amount_minor != 0",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([tx_id], |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)))
        .map_err(|e| e.to_string())?;

    let mut totals: Vec<(i64, i64)> = Vec::new();
    for row in rows {
        totals.push(row.map_err(|e| e.to_string())?);
    }

    if totals.len() != 2 {
        return Ok(None);
    }

    let base_commodity_id: Option<i64> = conn
        .query_row(
            "SELECT base_commodity_id FROM books WHERE id = ?1",
            [SINGLE_BOOK_ID],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let (commodity_id, commodity_amount, quote_commodity_id, quote_amount) =
        if let Some(base_id) = base_commodity_id {
            if totals[0].0 == base_id {
                (totals[1].0, totals[1].1, totals[0].0, totals[0].1)
            } else if totals[1].0 == base_id {
                (totals[0].0, totals[0].1, totals[1].0, totals[1].1)
            } else {
                (totals[0].0, totals[0].1, totals[1].0, totals[1].1)
            }
        } else {
            (totals[0].0, totals[0].1, totals[1].0, totals[1].1)
        };

    let scale: i64 = conn
        .query_row(
            "SELECT scale FROM commodities WHERE id = ?1",
            [commodity_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    let denom = commodity_amount.abs();
    if denom == 0 {
        return Ok(None);
    }

    let scale_factor: i128 = 10i128.pow(scale as u32);
    let numerator = (quote_amount.abs() as i128) * scale_factor;
    let price_minor = (numerator / (denom as i128)) as i64;

    conn.execute(
        "INSERT OR IGNORE INTO commodity_prices (book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, 'implicit', NULL, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            SINGLE_BOOK_ID,
            commodity_id,
            quote_commodity_id,
            price_minor,
            txn_date,
        ],
    )
    .map_err(|e| e.to_string())?;

    let price = conn
        .query_row(
            "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
             FROM commodity_prices
             WHERE book_id = ?1 AND commodity_id = ?2 AND quote_commodity_id = ?3 AND as_of_date = ?4",
            params![SINGLE_BOOK_ID, commodity_id, quote_commodity_id, txn_date],
            map_commodity_price_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(price)
}

#[command]
pub fn get_latest_price(
    db: State<DbState>,
    commodity_id: i64,
    quote_commodity_id: i64,
) -> Result<Option<CommodityPrice>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let price = conn
        .query_row(
            "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
             FROM commodity_prices
             WHERE book_id = ?1 AND commodity_id = ?2 AND quote_commodity_id = ?3
             ORDER BY as_of_date DESC, id DESC LIMIT 1",
            params![SINGLE_BOOK_ID, commodity_id, quote_commodity_id],
            map_commodity_price_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(price)
}

#[command]
pub fn get_price_on_date(
    db: State<DbState>,
    commodity_id: i64,
    quote_commodity_id: i64,
    as_of_date: String,
) -> Result<Option<CommodityPrice>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let price = conn
        .query_row(
            "SELECT id, book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at
             FROM commodity_prices
             WHERE book_id = ?1 AND commodity_id = ?2 AND quote_commodity_id = ?3 AND as_of_date <= ?4
             ORDER BY as_of_date DESC, id DESC LIMIT 1",
            params![SINGLE_BOOK_ID, commodity_id, quote_commodity_id, as_of_date],
            map_commodity_price_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(price)
}

#[command]
pub fn create_commodity_price(
    db: State<DbState>,
    input: CommodityPriceCreate,
) -> Result<CommodityPrice, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let is_manual = if input.is_manual { 1 } else { 0 };

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO commodity_prices (book_id, commodity_id, quote_commodity_id, price_minor, as_of_date, source, source_id, is_manual, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
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
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;
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

    let book_id = SINGLE_BOOK_ID;

    let rows = conn
        .execute(
            "UPDATE commodity_prices
             SET book_id = ?2, commodity_id = ?3, quote_commodity_id = ?4, price_minor = ?5, as_of_date = ?6,
                 source = ?7, source_id = ?8, is_manual = ?9
             WHERE id = ?1",
            params![
                input.id,
                book_id,
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

#[allow(dead_code)]
#[command]
pub fn list_dividend_income_categories(
    db: State<DbState>,
    book_id: i64,
    commodity_id: Option<i64>,
) -> Result<Vec<DividendIncomeCategory>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    if let Some(commodity_id) = commodity_id {
        let mut stmt = conn
            .prepare(
                "SELECT id, book_id, category_id, commodity_id, tax_withheld_minor, notes, created_at, updated_at
                 FROM dividend_income_categories
                 WHERE book_id = ?1 AND commodity_id = ?2
                 ORDER BY category_id ASC",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map(params![book_id, commodity_id], map_dividend_income_category_row)
            .map_err(|e| e.to_string())?;
        let mut items = Vec::new();
        for row in rows {
            items.push(row.map_err(|e| e.to_string())?);
        }
        return Ok(items);
    }

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, category_id, commodity_id, tax_withheld_minor, notes, created_at, updated_at
             FROM dividend_income_categories
             WHERE book_id = ?1
             ORDER BY category_id ASC",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([book_id], map_dividend_income_category_row)
        .map_err(|e| e.to_string())?;
    let mut items = Vec::new();
    for row in rows {
        items.push(row.map_err(|e| e.to_string())?);
    }

    Ok(items)
}

#[allow(dead_code)]
#[command]
pub fn create_dividend_income_category(
    db: State<DbState>,
    input: DividendIncomeCategoryCreate,
) -> Result<DividendIncomeCategory, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO dividend_income_categories (book_id, category_id, commodity_id, tax_withheld_minor, notes, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.category_id,
            input.commodity_id,
            input.tax_withheld_minor,
            input.notes,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let item = conn
        .query_row(
            "SELECT id, book_id, category_id, commodity_id, tax_withheld_minor, notes, created_at, updated_at
             FROM dividend_income_categories WHERE id = ?1",
            [id],
            map_dividend_income_category_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[allow(dead_code)]
#[command]
pub fn get_dividend_income_category(
    db: State<DbState>,
    id: i64,
) -> Result<Option<DividendIncomeCategory>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, category_id, commodity_id, tax_withheld_minor, notes, created_at, updated_at
             FROM dividend_income_categories WHERE id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let mut rows = stmt.query([id]).map_err(|e| e.to_string())?;
    if let Some(row) = rows.next().map_err(|e| e.to_string())? {
        Ok(Some(map_dividend_income_category_row(row).map_err(|e| e.to_string())?))
    } else {
        Ok(None)
    }
}

#[allow(dead_code)]
#[command]
pub fn update_dividend_income_category(
    db: State<DbState>,
    input: DividendIncomeCategoryUpdate,
) -> Result<DividendIncomeCategory, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let rows = conn
        .execute(
            "UPDATE dividend_income_categories
             SET book_id = ?2,
                 category_id = ?3,
                 commodity_id = ?4,
                 tax_withheld_minor = ?5,
                 notes = ?6,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![
                input.id,
                book_id,
                input.category_id,
                input.commodity_id,
                input.tax_withheld_minor,
                input.notes,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("dividend income category not found".to_string());
    }

    let item = conn
        .query_row(
            "SELECT id, book_id, category_id, commodity_id, tax_withheld_minor, notes, created_at, updated_at
             FROM dividend_income_categories WHERE id = ?1",
            [input.id],
            map_dividend_income_category_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[allow(dead_code)]
#[command]
pub fn delete_dividend_income_category(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let rows = conn
        .execute("DELETE FROM dividend_income_categories WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;
    Ok(rows > 0)
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
        temp.push(format!("rekenraam_commodities_test_{start}_{counter}"));
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

    fn get_usd_id(conn: &rusqlite::Connection) -> i64 {
        conn.query_row("SELECT id FROM commodities WHERE symbol='USD' LIMIT 1", [], |row| row.get(0))
            .expect("usd id")
    }

    fn create_stock_commodity(conn: &rusqlite::Connection) -> i64 {
        let book_id: i64 = conn
            .query_row("SELECT id FROM books WHERE name='Personal' LIMIT 1", [], |row| row.get(0))
            .expect("book id");
        conn.execute(
            "INSERT INTO commodities (book_id, kind, symbol, name, scale)
             VALUES (?1, 'stock', 'ACME', 'ACME Corp', 2)",
            params![book_id],
        )
        .expect("insert commodity");
        conn.last_insert_rowid()
    }

    fn create_investment_account(conn: &rusqlite::Connection, commodity_id: i64) -> i64 {
        let book_id: i64 = conn
            .query_row("SELECT id FROM books WHERE name='Personal' LIMIT 1", [], |row| row.get(0))
            .expect("book id");
        conn.execute(
            "INSERT INTO accounts (book_id, type, name, commodity_id)
             VALUES (?1, 'investment', 'Brokerage', ?2)",
            params![book_id, commodity_id],
        )
        .expect("insert investment account");
        conn.last_insert_rowid()
    }

    fn get_checking_account(conn: &rusqlite::Connection) -> i64 {
        conn.query_row("SELECT id FROM accounts WHERE name='Checking Account' LIMIT 1", [], |row| row.get(0))
            .expect("checking id")
    }

    #[test]
    fn test_prices_positions_and_gains() {
        let db_state = create_temp_db();

        let (stock_id, investment_account_id, cash_account_id, usd_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let stock_id = create_stock_commodity(conn);
            let investment_account_id = create_investment_account(conn, stock_id);
            let cash_account_id = get_checking_account(conn);
            let usd_id = get_usd_id(conn);
            (stock_id, investment_account_id, cash_account_id, usd_id)
        };

        let buy = buy_commodity(
            as_state(&db_state),
            BuyCommodityInput {
                book_id: 1,
                txn_date: "2024-01-10".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 1000,
                cash_amount_minor: 100_000,
                memo: Some("buy".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("buy commodity");
        assert!(buy.transaction_id > 0);

        let _price = create_commodity_price(
            as_state(&db_state),
            CommodityPriceCreate {
                book_id: 1,
                commodity_id: stock_id,
                quote_commodity_id: usd_id,
                price_minor: 20_000,
                as_of_date: "2024-01-10".to_string(),
                source: Some("manual".to_string()),
                source_id: None,
                is_manual: true,
            },
        )
        .expect("create price");

        let positions = get_positions(as_state(&db_state), 1, None).expect("positions");
        assert!(positions.iter().any(|p| p.account_id == investment_account_id));

        let converted = convert_positions(as_state(&db_state), usd_id, None)
            .expect("convert positions");
        let inv = converted
            .iter()
            .find(|p| p.account_id == investment_account_id)
            .expect("investment position");
        assert_eq!(inv.value_minor, 200_000);

        let sell = sell_commodity(
            as_state(&db_state),
            SellCommodityInput {
                book_id: 1,
                txn_date: "2024-02-10".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 500,
                cash_amount_minor: 70_000,
                lot_strategy: "fifo".to_string(),
                lot_allocations: None,
                allow_short: false,
                memo: Some("sell".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("sell commodity");

        let gains = validate_sell_gains(as_state(&db_state), sell.transaction_id)
            .expect("validate gains");
        assert_eq!(gains.cost_basis_minor, 50_000);
        assert_eq!(gains.proceeds_minor, 70_000);
        assert_eq!(gains.gain_loss_minor, 20_000);
    }

    #[test]
    fn test_corporate_actions_and_reports() {
        let db_state = create_temp_db();

        let (stock_id, investment_account_id, cash_account_id, usd_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let stock_id = create_stock_commodity(conn);
            let investment_account_id = create_investment_account(conn, stock_id);
            let cash_account_id = get_checking_account(conn);
            let usd_id = get_usd_id(conn);
            (stock_id, investment_account_id, cash_account_id, usd_id)
        };

        let buy = buy_commodity(
            as_state(&db_state),
            BuyCommodityInput {
                book_id: 1,
                txn_date: "2024-03-01".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 1000,
                cash_amount_minor: 50_000,
                memo: Some("buy".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("buy commodity");

        let action = apply_corporate_action_split_merge(
            as_state(&db_state),
            ApplyCorporateActionInput {
                book_id: 1,
                commodity_id: stock_id,
                ratio_num: 2,
                ratio_den: 1,
                effective_date: "2024-03-10".to_string(),
                memo: Some("2-for-1".to_string()),
            },
        )
        .expect("apply corporate action");
        assert!(action.transaction_id.is_some());

        let actions = list_corporate_actions(as_state(&db_state), 1, Some(stock_id))
            .expect("list corporate actions");
        assert_eq!(actions.len(), 1);

        let positions = get_positions(as_state(&db_state), 1, None)
            .expect("positions after split");
        let pos = positions
            .iter()
            .find(|p| p.account_id == investment_account_id)
            .expect("position exists");
        assert_eq!(pos.balance_minor, 2000);

        let _price = create_commodity_price(
            as_state(&db_state),
            CommodityPriceCreate {
                book_id: 1,
                commodity_id: stock_id,
                quote_commodity_id: usd_id,
                price_minor: 40_000,
                as_of_date: "2024-03-10".to_string(),
                source: Some("manual".to_string()),
                source_id: None,
                is_manual: true,
            },
        )
        .expect("create price");

        let sell = sell_commodity(
            as_state(&db_state),
            SellCommodityInput {
                book_id: 1,
                txn_date: "2024-03-15".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 1000,
                cash_amount_minor: 60_000,
                lot_strategy: "fifo".to_string(),
                lot_allocations: None,
                allow_short: false,
                memo: Some("sell".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("sell commodity");
        let gains = realized_gains_report(as_state(&db_state), None, None)
            .expect("realized gains");
        assert!(gains.iter().any(|g| g.tx_id == sell.transaction_id));

        let unrealized = unrealized_gains_report(as_state(&db_state), usd_id, None)
            .expect("unrealized gains");
        assert!(unrealized.iter().any(|u| u.account_id == investment_account_id));

        // ensure buy tx remains usable
        let _ = validate_sell_gains(as_state(&db_state), buy.transaction_id)
            .expect("validate gains for buy tx");
    }

    #[test]
    fn test_dividend_income_categories_crud() {
        let db_state = create_temp_db();

        let (category_id, usd_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let usd_id = get_usd_id(conn);
            conn.execute(
                "INSERT INTO categories (book_id, parent_id, name, kind, created_at, updated_at)
                 VALUES (1, NULL, 'Dividends', 'income', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert category");
            let category_id = conn.last_insert_rowid();
            (category_id, usd_id)
        };

        let item = create_dividend_income_category(
            as_state(&db_state),
            DividendIncomeCategoryCreate {
                book_id: 1,
                category_id,
                commodity_id: Some(usd_id),
                tax_withheld_minor: Some(100),
                notes: Some("withholding".to_string()),
            },
        )
        .expect("create dividend income category");

        let list = list_dividend_income_categories(as_state(&db_state), 1, Some(usd_id))
            .expect("list dividend income categories");
        assert_eq!(list.len(), 1);

        let fetched = get_dividend_income_category(as_state(&db_state), item.id)
            .expect("get dividend income category")
            .expect("exists");
        assert_eq!(fetched.category_id, category_id);

        let updated = update_dividend_income_category(
            as_state(&db_state),
            DividendIncomeCategoryUpdate {
                id: item.id,
                book_id: 1,
                category_id,
                commodity_id: Some(usd_id),
                tax_withheld_minor: Some(150),
                notes: Some("updated".to_string()),
            },
        )
        .expect("update dividend income category");
        assert_eq!(updated.tax_withheld_minor, Some(150));

        let duplicate_err = create_dividend_income_category(
            as_state(&db_state),
            DividendIncomeCategoryCreate {
                book_id: 1,
                category_id,
                commodity_id: Some(usd_id),
                tax_withheld_minor: None,
                notes: None,
            },
        )
        .expect_err("duplicate should fail");
        assert!(duplicate_err.to_lowercase().contains("unique") || duplicate_err.to_lowercase().contains("constraint"));

        let deleted = delete_dividend_income_category(as_state(&db_state), item.id)
            .expect("delete dividend income category");
        assert!(deleted);
    }

    #[test]
    fn test_price_sources_crud() {
        let db_state = create_temp_db();

        let alpha = create_price_source(
            as_state(&db_state),
            PriceSourceCreate {
                name: "Alpha".to_string(),
                kind: "provider".to_string(),
                provider: Some("AlphaFeed".to_string()),
                base_url: Some("https://alpha.example".to_string()),
            },
        )
        .expect("create price source");

        let beta = create_price_source(
            as_state(&db_state),
            PriceSourceCreate {
                name: "Beta".to_string(),
                kind: "manual".to_string(),
                provider: None,
                base_url: None,
            },
        )
        .expect("create second price source");

        let list = list_price_sources(as_state(&db_state)).expect("list price sources");
        assert!(list.len() >= 2);
        assert!(list.iter().any(|s| s.name == "Alpha"));
        assert!(list.iter().any(|s| s.name == "Beta"));

        let updated = update_price_source(
            as_state(&db_state),
            PriceSourceUpdate {
                id: alpha.id,
                name: "Alpha Prime".to_string(),
                kind: "provider".to_string(),
                provider: Some("AlphaFeed".to_string()),
                base_url: Some("https://alpha.example/v2".to_string()),
            },
        )
        .expect("update price source");
        assert_eq!(updated.name, "Alpha Prime");

        let duplicate_err = create_price_source(
            as_state(&db_state),
            PriceSourceCreate {
                name: "Alpha Prime".to_string(),
                kind: "provider".to_string(),
                provider: None,
                base_url: None,
            },
        )
        .expect_err("duplicate should fail");
        assert!(
            duplicate_err.to_lowercase().contains("unique")
                || duplicate_err.to_lowercase().contains("constraint")
        );

        let deleted = delete_price_source(as_state(&db_state), beta.id)
            .expect("delete price source");
        assert!(deleted);

        let list_after = list_price_sources(as_state(&db_state)).expect("list after delete");
        assert!(list_after.iter().any(|s| s.id == alpha.id));
    }

    #[test]
    fn test_commodity_price_sources_crud() {
        let db_state = create_temp_db();

        let commodity_id = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            create_stock_commodity(conn)
        };

        let source = create_price_source(
            as_state(&db_state),
            PriceSourceCreate {
                name: "Provider".to_string(),
                kind: "provider".to_string(),
                provider: Some("Feed".to_string()),
                base_url: Some("https://feed.example".to_string()),
            },
        )
        .expect("create source");

        let primary = create_commodity_price_source(
            as_state(&db_state),
            CommodityPriceSourceCreate {
                commodity_id,
                source_id: source.id,
                symbol: "ZZZ".to_string(),
                name_override: Some("ACME Override".to_string()),
                is_primary: true,
            },
        )
        .expect("create commodity price source");
        assert!(primary.is_primary);

        let secondary = create_commodity_price_source(
            as_state(&db_state),
            CommodityPriceSourceCreate {
                commodity_id,
                source_id: source.id,
                symbol: "AAA".to_string(),
                name_override: None,
                is_primary: false,
            },
        )
        .expect("create secondary mapping");

        let list = list_commodity_price_sources(as_state(&db_state), commodity_id)
            .expect("list commodity price sources");
        assert_eq!(list.len(), 2);
        assert_eq!(list[0].id, primary.id);
        assert_eq!(list[1].id, secondary.id);

        let updated = update_commodity_price_source(
            as_state(&db_state),
            CommodityPriceSourceUpdate {
                id: secondary.id,
                commodity_id,
                source_id: source.id,
                symbol: "BBB".to_string(),
                name_override: Some("Alt".to_string()),
                is_primary: true,
            },
        )
        .expect("update commodity price source");
        assert!(updated.is_primary);
        assert_eq!(updated.symbol, "BBB");

        let duplicate_err = create_commodity_price_source(
            as_state(&db_state),
            CommodityPriceSourceCreate {
                commodity_id,
                source_id: source.id,
                symbol: "BBB".to_string(),
                name_override: None,
                is_primary: false,
            },
        )
        .expect_err("duplicate should fail");
        assert!(
            duplicate_err.to_lowercase().contains("unique")
                || duplicate_err.to_lowercase().contains("constraint")
        );

        let deleted = delete_commodity_price_source(as_state(&db_state), primary.id)
            .expect("delete commodity price source");
        assert!(deleted);

        let list_after = list_commodity_price_sources(as_state(&db_state), commodity_id)
            .expect("list after delete");
        assert_eq!(list_after.len(), 1);
        assert_eq!(list_after[0].id, secondary.id);
    }

    #[test]
    fn test_booking_policy_override_for_investments() {
        let db_state = create_temp_db();

        let (stock_id, investment_account_id, cash_account_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let stock_id = create_stock_commodity(conn);
            let investment_account_id = create_investment_account(conn, stock_id);
            let cash_account_id = get_checking_account(conn);
            (stock_id, investment_account_id, cash_account_id)
        };

        let first_buy = buy_commodity(
            as_state(&db_state),
            BuyCommodityInput {
                book_id: 1,
                txn_date: "2024-01-01".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 1000,
                cash_amount_minor: 10_000,
                memo: Some("buy1".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("buy commodity 1");

        let second_buy = buy_commodity(
            as_state(&db_state),
            BuyCommodityInput {
                book_id: 1,
                txn_date: "2024-02-01".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 1000,
                cash_amount_minor: 12_000,
                memo: Some("buy2".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("buy commodity 2");

        let sell = sell_commodity(
            as_state(&db_state),
            SellCommodityInput {
                book_id: 1,
                txn_date: "2024-03-01".to_string(),
                commodity_id: stock_id,
                investment_account_id,
                cash_account_id,
                quantity_minor: 1000,
                cash_amount_minor: 15_000,
                lot_strategy: "lifo".to_string(),
                lot_allocations: None,
                allow_short: false,
                memo: Some("sell".to_string()),
                payee_id: None,
                status: Some("cleared".to_string()),
            },
        )
        .expect("sell commodity");

        assert_eq!(sell.allocations.len(), 1);
        assert_eq!(sell.allocations[0].lot_id, second_buy.lot_id.unwrap());
        assert_ne!(sell.allocations[0].lot_id, first_buy.lot_id.unwrap());
    }
}
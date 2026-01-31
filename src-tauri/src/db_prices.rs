// Deprecated: functionality moved to db_commodities.rs

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

use std::collections::HashSet;

use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::state::DbState;

const SINGLE_BOOK_ID: i64 = 1;

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

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportRule {
    pub id: i64,
    pub book_id: i64,
    pub rule_kind: String,
    pub match_type: String,
    pub match_text: String,
    pub priority: i64,
    pub amount_min_minor: Option<i64>,
    pub amount_max_minor: Option<i64>,
    pub date_from: Option<String>,
    pub date_to: Option<String>,
    pub match_account_id: Option<i64>,
    pub target_account_id: Option<i64>,
    pub target_category_id: Option<i64>,
    pub target_payee_id: Option<i64>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportRuleCreate {
    pub book_id: i64,
    pub rule_kind: String,
    pub match_type: Option<String>,
    pub match_text: String,
    pub priority: Option<i64>,
    pub amount_min_minor: Option<i64>,
    pub amount_max_minor: Option<i64>,
    pub date_from: Option<String>,
    pub date_to: Option<String>,
    pub match_account_id: Option<i64>,
    pub target_account_id: Option<i64>,
    pub target_category_id: Option<i64>,
    pub target_payee_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportSession {
    pub id: i64,
    pub book_id: i64,
    pub source: Option<String>,
    pub status: String,
    pub started_at: String,
    pub committed_at: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportSessionStartInput {
    pub book_id: i64,
    pub source: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportDraft {
    pub payee_name: Option<String>,
    pub memo: Option<String>,
    pub txn_date: Option<String>,
    pub amount_minor: Option<i64>,
    pub reference: Option<String>,
    pub import_id: Option<String>,
    pub currency_code: Option<String>,
    pub account_id: Option<i64>,
    pub category_id: Option<i64>,
    pub payee_id: Option<i64>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct ImportMatchResult {
    pub draft: ImportDraft,
    pub matched_tx_id: Option<i64>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct ImportBatchResult {
    pub created_tx_ids: Vec<i64>,
    pub matched_tx_ids: Vec<i64>,
    pub skipped: i64,
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

fn map_import_rule_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<ImportRule> {
    Ok(ImportRule {
        id: row.get(0)?,
        book_id: row.get(1)?,
        rule_kind: row.get(2)?,
        match_type: row.get(3)?,
        match_text: row.get(4)?,
        priority: row.get(5)?,
        amount_min_minor: row.get(6)?,
        amount_max_minor: row.get(7)?,
        date_from: row.get(8)?,
        date_to: row.get(9)?,
        match_account_id: row.get(10)?,
        target_account_id: row.get(11)?,
        target_category_id: row.get(12)?,
        target_payee_id: row.get(13)?,
        created_at: row.get(14)?,
        updated_at: row.get(15)?,
    })
}

fn map_import_session_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<ImportSession> {
    Ok(ImportSession {
        id: row.get(0)?,
        book_id: row.get(1)?,
        source: row.get(2)?,
        status: row.get(3)?,
        started_at: row.get(4)?,
        committed_at: row.get(5)?,
        notes: row.get(6)?,
    })
}

fn parse_amount_to_minor(raw: &str, scale: i64) -> Result<i64, String> {
    let mut s = raw.trim().replace(',', ".");
    let mut negative = false;
    if s.starts_with('(') && s.ends_with(')') {
        negative = true;
        s = s.trim_start_matches('(').trim_end_matches(')').to_string();
    }
    if s.starts_with('-') {
        negative = true;
        s = s.trim_start_matches('-').to_string();
    }

    let parts: Vec<&str> = s.split('.').collect();
    let whole = parts.get(0).unwrap_or(&"0").trim();
    let frac = parts.get(1).unwrap_or(&"0").trim();

    let mut frac_str = frac.to_string();
    while frac_str.len() < (scale as usize) {
        frac_str.push('0');
    }
    if frac_str.len() > (scale as usize) {
        frac_str = frac_str[..(scale as usize)].to_string();
    }

    let whole_val: i64 = whole.parse().unwrap_or(0);
    let frac_val: i64 = frac_str.parse().unwrap_or(0);
    let minor = whole_val * 10_i64.pow(scale as u32) + frac_val;
    Ok(if negative { -minor } else { minor })
}

fn format_ymd(y: i32, m: i32, d: i32) -> String {
    format!("{:04}-{:02}-{:02}", y, m, d)
}

fn parse_date_mmddyy(raw: &str) -> Option<String> {
    let cleaned = raw.trim().replace('\\', "/").replace('-', "/");
    let parts: Vec<&str> = cleaned.split('/').collect();
    if parts.len() != 3 {
        return None;
    }
    let p1: i32 = parts[0].parse().ok()?;
    let p2: i32 = parts[1].parse().ok()?;
    let mut y: i32 = parts[2].trim_matches('"').trim_matches('\'').parse().ok()?;
    if y < 100 {
        y = if y < 70 { 2000 + y } else { 1900 + y };
    }
    let (m, d) = if p1 > 12 { (p2, p1) } else { (p1, p2) };
    Some(format_ymd(y, m, d))
}

fn parse_ofx_date(raw: &str) -> Option<String> {
    let digits: String = raw.chars().take(8).collect();
    if digits.len() != 8 {
        return None;
    }
    let y: i32 = digits[0..4].parse().ok()?;
    let m: i32 = digits[4..6].parse().ok()?;
    let d: i32 = digits[6..8].parse().ok()?;
    Some(format_ymd(y, m, d))
}

fn parse_mt940_date(raw: &str) -> Option<String> {
    if raw.len() < 6 {
        return None;
    }
    let y: i32 = raw[0..2].parse().ok()?;
    let m: i32 = raw[2..4].parse().ok()?;
    let d: i32 = raw[4..6].parse().ok()?;
    let year = if y < 70 { 2000 + y } else { 1900 + y };
    Some(format_ymd(year, m, d))
}

fn parse_qif(content: &str) -> Result<Vec<ImportDraft>, String> {
    let mut results = Vec::new();
    let mut current = ImportDraft {
        payee_name: None,
        memo: None,
        txn_date: None,
        amount_minor: None,
        reference: None,
        import_id: None,
        currency_code: None,
        account_id: None,
        category_id: None,
        payee_id: None,
    };

    for line in content.lines() {
        let line = line.trim_end();
        if line == "^" {
            if current.txn_date.is_some() || current.amount_minor.is_some() {
                results.push(current);
            }
            current = ImportDraft {
                payee_name: None,
                memo: None,
                txn_date: None,
                amount_minor: None,
                reference: None,
                import_id: None,
                currency_code: None,
                account_id: None,
                category_id: None,
                payee_id: None,
            };
            continue;
        }
        if line.starts_with('D') {
            current.txn_date = parse_date_mmddyy(&line[1..]);
        } else if line.starts_with('T') {
            current.amount_minor = Some(parse_amount_to_minor(&line[1..], 2)?);
        } else if line.starts_with('P') {
            current.payee_name = Some(line[1..].trim().to_string());
        } else if line.starts_with('M') {
            current.memo = Some(line[1..].trim().to_string());
        } else if line.starts_with('N') {
            current.reference = Some(line[1..].trim().to_string());
        }
    }

    Ok(results)
}

fn parse_ofx(content: &str) -> Result<Vec<ImportDraft>, String> {
    let mut results = Vec::new();
    let mut currency_code: Option<String> = None;
    for line in content.lines() {
        let l = line.trim();
        if l.starts_with("<CURDEF>") {
            currency_code = Some(l.replace("<CURDEF>", "").trim().to_string());
        }
    }

    let mut in_txn = false;
    let mut current = ImportDraft {
        payee_name: None,
        memo: None,
        txn_date: None,
        amount_minor: None,
        reference: None,
        import_id: None,
        currency_code: currency_code.clone(),
        account_id: None,
        category_id: None,
        payee_id: None,
    };

    for line in content.lines() {
        let l = line.trim();
        if l.starts_with("<STMTTRN>") {
            in_txn = true;
            current = ImportDraft {
                payee_name: None,
                memo: None,
                txn_date: None,
                amount_minor: None,
                reference: None,
                import_id: None,
                currency_code: currency_code.clone(),
                account_id: None,
                category_id: None,
                payee_id: None,
            };
            continue;
        }
        if l.starts_with("</STMTTRN>") {
            in_txn = false;
            let has_data = current.txn_date.is_some() || current.amount_minor.is_some();
            if has_data {
                results.push(current);
            }
            current = ImportDraft {
                payee_name: None,
                memo: None,
                txn_date: None,
                amount_minor: None,
                reference: None,
                import_id: None,
                currency_code: currency_code.clone(),
                account_id: None,
                category_id: None,
                payee_id: None,
            };
            continue;
        }
        if !in_txn {
            continue;
        }
        if l.starts_with("<DTPOSTED>") {
            current.txn_date = parse_ofx_date(l.replace("<DTPOSTED>", "").trim());
        } else if l.starts_with("<TRNAMT>") {
            current.amount_minor = Some(parse_amount_to_minor(&l.replace("<TRNAMT>", ""), 2)?);
        } else if l.starts_with("<NAME>") {
            current.payee_name = Some(l.replace("<NAME>", "").trim().to_string());
        } else if l.starts_with("<MEMO>") {
            current.memo = Some(l.replace("<MEMO>", "").trim().to_string());
        } else if l.starts_with("<FITID>") {
            current.import_id = Some(l.replace("<FITID>", "").trim().to_string());
        }
    }

    Ok(results)
}

fn parse_hbci_mt940(content: &str) -> Result<Vec<ImportDraft>, String> {
    let mut results = Vec::new();
    let mut current: Option<ImportDraft> = None;
    for line in content.lines() {
        let l = line.trim();
        if l.starts_with(":61:") {
            if let Some(draft) = current.take() {
                results.push(draft);
            }
            let body = &l[4..];
            let date = parse_mt940_date(body).unwrap_or_else(|| "".to_string());
            let amount_part = body
                .chars()
                .skip(6)
                .collect::<String>();
            let is_debit = amount_part.contains('D');
            let amt_str = amount_part
                .split(|c| c == 'C' || c == 'D')
                .nth(1)
                .unwrap_or("");
            let mut amount_minor = parse_amount_to_minor(amt_str, 2).unwrap_or(0);
            if is_debit {
                amount_minor = -amount_minor.abs();
            }
            current = Some(ImportDraft {
                payee_name: None,
                memo: None,
                txn_date: if date.is_empty() { None } else { Some(date) },
                amount_minor: Some(amount_minor),
                reference: None,
                import_id: None,
                currency_code: None,
                account_id: None,
                category_id: None,
                payee_id: None,
            });
        } else if l.starts_with(":86:") {
            if let Some(draft) = current.as_mut() {
                draft.memo = Some(l[4..].trim().to_string());
            }
        }
    }
    if let Some(draft) = current {
        results.push(draft);
    }
    Ok(results)
}

#[command]
pub fn parse_import_file(format: String, content: String) -> Result<Vec<ImportDraft>, String> {
    let fmt = format.to_lowercase();
    let detected = if fmt == "auto" {
        if content.contains("<OFX>") {
            "ofx"
        } else if content.contains("!Type:") {
            "qif"
        } else if content.contains(":61:") {
            "hbci"
        } else {
            return Err("unable to detect import format".to_string());
        }
    } else {
        fmt.as_str()
    };

    match detected {
        "qif" => parse_qif(&content),
        "ofx" => parse_ofx(&content),
        "hbci" => parse_hbci_mt940(&content),
        _ => Err("unsupported import format".to_string()),
    }
}

fn find_matching_tx(
    conn: &rusqlite::Connection,
    account_id: i64,
    draft: &ImportDraft,
) -> Result<Option<i64>, String> {
    if let Some(import_id) = draft.import_id.as_ref() {
        let tx_id: Option<i64> = conn
            .query_row(
                "SELECT t.id
                 FROM transactions t
                 JOIN splits s ON s.tx_id = t.id
                 WHERE s.account_id = ?1 AND t.import_id = ?2
                 LIMIT 1",
                params![account_id, import_id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|e| e.to_string())?;
        if tx_id.is_some() {
            return Ok(tx_id);
        }
    }

    if let (Some(txn_date), Some(amount_minor)) = (&draft.txn_date, draft.amount_minor) {
        let mut sql = String::from(
            "SELECT t.id
             FROM transactions t
             JOIN splits s ON s.tx_id = t.id
             LEFT JOIN payees p ON p.id = t.payee_id
             WHERE s.account_id = ?1 AND t.txn_date = ?2 AND s.amount_minor = ?3",
        );
        let mut params: Vec<Value> = vec![Value::from(account_id), Value::from(txn_date.clone()), Value::from(amount_minor)];
        if let Some(payee) = draft.payee_name.as_ref() {
            sql.push_str(" AND p.name LIKE ?");
            params.push(Value::from(format!("%{}%", payee)));
        } else if let Some(memo) = draft.memo.as_ref() {
            sql.push_str(" AND t.memo LIKE ?");
            params.push(Value::from(format!("%{}%", memo)));
        }
        sql.push_str(" ORDER BY t.id DESC LIMIT 1");

        let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
        let tx_id: Option<i64> = stmt
            .query_row(params_from_iter(params), |row| row.get(0))
            .optional()
            .map_err(|e| e.to_string())?;
        return Ok(tx_id);
    }

    Ok(None)
}

#[command]
pub fn match_import_transactions(
    db: State<DbState>,
    account_id: i64,
    drafts: Vec<ImportDraft>,
) -> Result<Vec<ImportMatchResult>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut results = Vec::new();
    for draft in drafts {
        let match_id = find_matching_tx(conn, account_id, &draft)?;
        results.push(ImportMatchResult {
            draft,
            matched_tx_id: match_id,
        });
    }

    Ok(results)
}

#[command]
pub fn import_transactions(
    db: State<DbState>,
    account_id: i64,
    drafts: Vec<ImportDraft>,
    create_payees: bool,
) -> Result<ImportBatchResult, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let account_commodity_id: i64 = tx
        .query_row(
            "SELECT commodity_id FROM accounts WHERE id = ?1",
            [account_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    let imbalance_account_id: i64 = tx
        .query_row(
            "SELECT id FROM accounts
             WHERE book_id = ?1 AND commodity_id = ?2 AND type = 'equity' AND name = 'Imbalance-Import'
             LIMIT 1",
            params![SINGLE_BOOK_ID, account_commodity_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?
        .unwrap_or(0);

    let imbalance_account_id = if imbalance_account_id == 0 {
        tx.execute(
            "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, is_closed, created_at, updated_at)
             VALUES (?1, NULL, 'equity', 'Imbalance-Import', ?2, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![SINGLE_BOOK_ID, account_commodity_id],
        )
        .map_err(|e| e.to_string())?;
        tx.last_insert_rowid()
    } else {
        imbalance_account_id
    };

    let mut created = Vec::new();
    let mut matched = Vec::new();
    let mut skipped: i64 = 0;

    for draft in drafts {
        let target_account_id = draft.account_id.unwrap_or(account_id);
        let match_id = find_matching_tx(&tx, target_account_id, &draft)?;
        if let Some(id) = match_id {
            matched.push(id);
            continue;
        }

        let txn_date = match draft.txn_date.clone() {
            Some(d) => d,
            None => {
                skipped += 1;
                continue;
            }
        };
        let amount_minor = match draft.amount_minor {
            Some(a) => a,
            None => {
                skipped += 1;
                continue;
            }
        };

        let payee_id = if let Some(pid) = draft.payee_id {
            Some(pid)
        } else if let Some(name) = draft.payee_name.as_ref() {
            let existing: Option<i64> = tx
                .query_row(
                    "SELECT id FROM payees WHERE book_id = ?1 AND name = ?2",
                    params![SINGLE_BOOK_ID, name],
                    |row| row.get(0),
                )
                .optional()
                .map_err(|e| e.to_string())?;
            if let Some(id) = existing {
                Some(id)
            } else if create_payees {
                tx.execute(
                    "INSERT INTO payees (book_id, name, kind, created_at, updated_at)
                     VALUES (?1, ?2, 'payee', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![SINGLE_BOOK_ID, name],
                )
                .map_err(|e| e.to_string())?;
                Some(tx.last_insert_rowid())
            } else {
                None
            }
        } else {
            None
        };

        tx.execute(
            "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, 'uncleared', ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                SINGLE_BOOK_ID,
                txn_date,
                payee_id,
                draft.memo,
                draft.reference,
                draft.import_id,
            ],
        )
        .map_err(|e| e.to_string())?;

        let tx_id = tx.last_insert_rowid();

        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                tx_id,
                target_account_id,
                account_commodity_id,
                amount_minor,
                draft.category_id,
                draft.memo,
            ],
        )
        .map_err(|e| e.to_string())?;

        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![tx_id, imbalance_account_id, account_commodity_id, -amount_minor],
        )
        .map_err(|e| e.to_string())?;

        created.push(tx_id);
    }

    tx.commit().map_err(|e| e.to_string())?;

    Ok(ImportBatchResult {
        created_tx_ids: created,
        matched_tx_ids: matched,
        skipped,
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
         WHERE s.account_id = ?1",
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

#[command]
pub fn create_import_rule(
    db: State<DbState>,
    input: ImportRuleCreate,
) -> Result<ImportRule, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;
    let rule_kind = input.rule_kind.to_lowercase();
    if rule_kind != "payee"
        && rule_kind != "memo"
        && rule_kind != "amount"
        && rule_kind != "date"
        && rule_kind != "account"
    {
        return Err("rule_kind must be payee, memo, amount, date, or account".to_string());
    }
    let match_type = input
        .match_type
        .unwrap_or_else(|| "contains".to_string())
        .to_lowercase();
    if match_type != "contains" && match_type != "equals" {
        return Err("match_type must be contains or equals".to_string());
    }
    if let (Some(min), Some(max)) = (input.amount_min_minor, input.amount_max_minor) {
        if min > max {
            return Err("amount_min_minor must be <= amount_max_minor".to_string());
        }
    }
    let priority = input.priority.unwrap_or(100);


    conn.execute(
        "INSERT INTO import_rules (book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            rule_kind,
            match_type,
            input.match_text,
            priority,
            input.amount_min_minor,
            input.amount_max_minor,
            input.date_from,
            input.date_to,
            input.match_account_id,
            input.target_account_id,
            input.target_category_id,
            input.target_payee_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let rule = conn
        .query_row(
            "SELECT id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at, updated_at
             FROM import_rules WHERE id = ?1",
            [id],
            map_import_rule_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(rule)
}

#[command]
pub fn list_import_rules(db: State<DbState>, book_id: i64) -> Result<Vec<ImportRule>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at, updated_at
             FROM import_rules WHERE book_id = ?1 ORDER BY priority ASC, id ASC",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([book_id], map_import_rule_row)
        .map_err(|e| e.to_string())?;

    let mut rules = Vec::new();
    for row in rows {
        rules.push(row.map_err(|e| e.to_string())?);
    }

    Ok(rules)
}

#[command]
pub fn delete_import_rule(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute("DELETE FROM import_rules WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(rows > 0)
}

#[command]
pub fn start_import_session(
    db: State<DbState>,
    input: ImportSessionStartInput,
) -> Result<ImportSession, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO import_sessions (book_id, source, status, started_at, notes)
         VALUES (?1, ?2, 'started', strftime('%Y-%m-%dT%H:%M:%fZ','now'), ?3)",
        params![book_id, input.source, input.notes],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let session = conn
        .query_row(
            "SELECT id, book_id, source, status, started_at, committed_at, notes
             FROM import_sessions WHERE id = ?1",
            [id],
            map_import_session_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(session)
}

#[command]
pub fn commit_import_session(db: State<DbState>, session_id: i64) -> Result<ImportSession, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute(
            "UPDATE import_sessions
             SET status = 'committed', committed_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            [session_id],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("import session not found".to_string());
    }

    let session = conn
        .query_row(
            "SELECT id, book_id, source, status, started_at, committed_at, notes
             FROM import_sessions WHERE id = ?1",
            [session_id],
            map_import_session_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(session)
}

#[command]
pub fn apply_import_rules(
    db: State<DbState>,
    drafts: Vec<ImportDraft>,
) -> Result<Vec<ImportDraft>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at, updated_at
             FROM import_rules WHERE book_id = ?1 ORDER BY priority ASC, id ASC",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([SINGLE_BOOK_ID], map_import_rule_row)
        .map_err(|e| e.to_string())?;

    let mut rules = Vec::new();
    for row in rows {
        rules.push(row.map_err(|e| e.to_string())?);
    }

    let mut results = Vec::new();
    for mut draft in drafts {
        for rule in &rules {
            let mut matched = false;
            match rule.rule_kind.as_str() {
                "payee" | "memo" => {
                    let haystack = match rule.rule_kind.as_str() {
                        "payee" => draft.payee_name.as_deref().unwrap_or(""),
                        _ => draft.memo.as_deref().unwrap_or(""),
                    };
                    let needle = rule.match_text.as_str();
                    matched = if rule.match_type == "equals" {
                        haystack.eq_ignore_ascii_case(needle)
                    } else {
                        haystack.to_lowercase().contains(&needle.to_lowercase())
                    };
                }
                "amount" => {
                    if let Some(amount) = draft.amount_minor {
                        let min_ok = rule.amount_min_minor.map(|min| amount >= min).unwrap_or(true);
                        let max_ok = rule.amount_max_minor.map(|max| amount <= max).unwrap_or(true);
                        matched = min_ok && max_ok;
                    }
                }
                "date" => {
                    if let Some(txn_date) = draft.txn_date.as_deref() {
                        let from_ok = rule.date_from.as_deref().map(|d| txn_date >= d).unwrap_or(true);
                        let to_ok = rule.date_to.as_deref().map(|d| txn_date <= d).unwrap_or(true);
                        matched = from_ok && to_ok;
                    }
                }
                "account" => {
                    if let (Some(match_account_id), Some(account_id)) = (rule.match_account_id, draft.account_id) {
                        matched = match_account_id == account_id;
                    }
                }
                _ => {}
            }

            if matched {
                if rule.target_account_id.is_some() {
                    draft.account_id = rule.target_account_id;
                }
                if rule.target_category_id.is_some() {
                    draft.category_id = rule.target_category_id;
                }
                if rule.target_payee_id.is_some() {
                    draft.payee_id = rule.target_payee_id;
                }
            }
        }
        results.push(draft);
    }

    Ok(results)
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

    let book_id = SINGLE_BOOK_ID;

    let mut accounts = HashSet::new();
    for split in &input.splits {
        accounts.insert(split.account_id);
    }
    check_account_locks(&tx, &input.txn_date, &accounts)?;

    let status = input.status.unwrap_or_else(|| "uncleared".to_string());

    tx.execute(
        "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
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
         WHERE t.book_id = ?",
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

    let book_id = SINGLE_BOOK_ID;

    let existing_book: Option<i64> = tx
        .query_row(
            "SELECT book_id FROM transactions WHERE id = ?1",
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

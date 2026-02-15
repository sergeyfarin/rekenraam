use csv::ReaderBuilder;
use chrono::{Duration, NaiveDate};
use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::state::DbState;

const SINGLE_BOOK_ID: i64 = 1;
const VOID_SESSION_PREFIX: &str = "void:";

fn current_session_id(conn: &rusqlite::Connection) -> Result<String, String> {
    conn.query_row("SELECT id FROM app_runtime_session LIMIT 1", [], |row| row.get(0))
        .map_err(|e| e.to_string())
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
    pub file_name: Option<String>,
    pub file_hash: Option<String>,
    pub file_size: Option<i64>,
    pub status: String,
    pub started_at: String,
    pub committed_at: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportSessionStartInput {
    pub book_id: i64,
    pub source: Option<String>,
    pub file_name: Option<String>,
    pub file_hash: Option<String>,
    pub file_size: Option<i64>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportRunInput {
    pub account_id: i64,
    pub drafts: Vec<ImportDraft>,
    pub create_payees: bool,
    pub mode: Option<ImportMode>,
    pub source: Option<String>,
    pub file_name: Option<String>,
    pub file_hash: Option<String>,
    pub file_size: Option<i64>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportRunResult {
    pub session: ImportSession,
    pub batch: ImportBatchResult,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportDraft {
    pub payee_name: Option<String>,
    pub memo: Option<String>,
    pub txn_date: Option<String>,
    pub happened_at_utc: Option<String>,
    pub posted_date: Option<String>,
    pub posted_at_utc: Option<String>,
    pub posted_tz: Option<String>,
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
    pub updated_tx_ids: Vec<i64>,
    pub skipped: i64,
    pub review_matches: Option<Vec<ImportMatchResult>>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ImportSessionTransactionInfo {
    pub session_id: i64,
    pub tx_id: i64,
    pub action: String,
    pub created_at: String,
    pub source: Option<String>,
    pub file_name: Option<String>,
    pub file_hash: Option<String>,
    pub file_size: Option<i64>,
    pub started_at: String,
    pub committed_at: Option<String>,
}

#[derive(Serialize, Deserialize, Debug, Clone, Default)]
pub struct ImportLocaleOptions {
    pub decimal_separator: Option<String>,
    pub thousands_separator: Option<String>,
    pub date_format: Option<String>,
    pub csv_delimiter: Option<String>,
}

#[derive(Serialize, Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ImportMode {
    AlwaysCreate,
    MatchAndEnrich,
    Deduplicate,
    Review,
    UpdateOnly,
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
    })
}

fn map_import_session_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<ImportSession> {
    Ok(ImportSession {
        id: row.get(0)?,
        book_id: row.get(1)?,
        source: row.get(2)?,
        file_name: row.get(3)?,
        file_hash: row.get(4)?,
        file_size: row.get(5)?,
        status: row.get(6)?,
        started_at: row.get(7)?,
        committed_at: row.get(8)?,
        notes: row.get(9)?,
    })
}

fn parse_amount_to_minor_with_locale(
    raw: &str,
    scale: i64,
    locale: Option<&ImportLocaleOptions>,
) -> Result<i64, String> {
    let mut s = raw.trim().replace(' ', "");
    let mut negative = false;
    if s.starts_with('(') && s.ends_with(')') {
        negative = true;
        s = s.trim_start_matches('(').trim_end_matches(')').to_string();
    }
    if s.starts_with('-') {
        negative = true;
        s = s.trim_start_matches('-').to_string();
    }
    if s.starts_with('+') {
        s = s.trim_start_matches('+').to_string();
    }

    if let Some(locale) = locale {
        if let Some(thousands) = locale.thousands_separator.as_deref() {
            s = s.replace(thousands, "");
        }
    }

    let dec_pos = if let Some(locale) = locale {
        locale
            .decimal_separator
            .as_deref()
            .and_then(|sep| s.rfind(sep))
    } else {
        None
    }
    .or_else(|| {
        let last_dot = s.rfind('.');
        let last_comma = s.rfind(',');
        match (last_dot, last_comma) {
            (Some(d), Some(c)) => Some(d.max(c)),
            (Some(d), None) => Some(d),
            (None, Some(c)) => Some(c),
            (None, None) => None,
        }
    });

    let (whole_raw, frac_raw) = match dec_pos {
        Some(pos) => (&s[..pos], &s[pos + 1..]),
        None => (s.as_str(), ""),
    };

    let whole: String = whole_raw.chars().filter(|c| c.is_ascii_digit()).collect();
    let frac: String = frac_raw.chars().filter(|c| c.is_ascii_digit()).collect();

    let whole = if whole.is_empty() { "0" } else { &whole };
    let mut frac_str = frac;
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

fn parse_date_ymd(raw: &str) -> Option<String> {
    let cleaned = raw.trim().replace('\\', "/").replace('-', "/");
    let parts: Vec<&str> = cleaned.split('/').collect();
    if parts.len() != 3 {
        return None;
    }
    let y: i32 = parts[0].parse().ok()?;
    let m: i32 = parts[1].parse().ok()?;
    let d: i32 = parts[2].parse().ok()?;
    if y < 1000 {
        return None;
    }
    Some(format_ymd(y, m, d))
}

fn parse_csv_date(raw: &str, locale: Option<&ImportLocaleOptions>) -> Option<String> {
    parse_date_with_format(raw, locale)
        .or_else(|| parse_date_ymd(raw))
        .or_else(|| parse_date_mmddyy(raw))
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

fn parse_ofx_datetime_utc(raw: &str) -> Option<String> {
    let trimmed = raw.trim();
    let digits: String = trimmed.chars().filter(|c| c.is_ascii_digit()).collect();
    if digits.len() < 14 {
        return None;
    }

    let y: i32 = digits[0..4].parse().ok()?;
    let m: u32 = digits[4..6].parse().ok()?;
    let d: u32 = digits[6..8].parse().ok()?;
    let hh: u32 = digits[8..10].parse().ok()?;
    let mm: u32 = digits[10..12].parse().ok()?;
    let ss: u32 = digits[12..14].parse().ok()?;

    let mut dt = NaiveDate::from_ymd_opt(y, m, d)?.and_hms_opt(hh, mm, ss)?;

    let offset_hours = trimmed
        .find('[')
        .and_then(|start| trimmed[start + 1..].find(']').map(|end| (start, end)))
        .and_then(|(start, end)| {
            let tz = &trimmed[start + 1..start + 1 + end];
            let sign = if tz.starts_with('+') {
                1_i64
            } else if tz.starts_with('-') {
                -1_i64
            } else {
                return None;
            };
            let hours_str: String = tz[1..]
                .chars()
                .take_while(|c| c.is_ascii_digit())
                .collect();
            let hours: i64 = hours_str.parse().ok()?;
            Some(sign * hours)
        });

    let offset_hours = offset_hours?;
    dt -= Duration::hours(offset_hours);

    Some(format!("{}Z", dt.format("%Y-%m-%dT%H:%M:%S.000")))
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

fn parse_date_with_format(raw: &str, locale: Option<&ImportLocaleOptions>) -> Option<String> {
    let fmt = locale?.date_format.as_deref()?;
    let cleaned = raw.trim().replace('\r', "").replace('\n', "");
    let date_part = cleaned
        .split('T')
        .next()
        .and_then(|s| s.split_whitespace().next())?
        .trim();

    let split_parts = |value: &str| -> Vec<String> {
        value
            .split(|c: char| !c.is_ascii_alphanumeric())
            .filter(|s| !s.is_empty())
            .map(|s| s.to_string())
            .collect()
    };

    let fmt_tokens = split_parts(fmt);
    let value_tokens = split_parts(date_part);
    if fmt_tokens.len() != value_tokens.len() {
        return None;
    }

    let mut year: Option<i32> = None;
    let mut month: Option<i32> = None;
    let mut day: Option<i32> = None;

    for (token, value) in fmt_tokens.iter().zip(value_tokens.iter()) {
        let t = token.to_ascii_uppercase();
        if t.contains('Y') {
            let mut y: i32 = value.parse().ok()?;
            if y < 100 {
                y = if y < 70 { 2000 + y } else { 1900 + y };
            }
            year = Some(y);
        } else if t.contains('M') {
            if t.contains("MMM") {
                let val = value.to_ascii_lowercase();
                let m = match val.as_str() {
                    "jan" | "january" => 1,
                    "feb" | "february" => 2,
                    "mar" | "march" => 3,
                    "apr" | "april" => 4,
                    "may" => 5,
                    "jun" | "june" => 6,
                    "jul" | "july" => 7,
                    "aug" | "august" => 8,
                    "sep" | "sept" | "september" => 9,
                    "oct" | "october" => 10,
                    "nov" | "november" => 11,
                    "dec" | "december" => 12,
                    _ => return None,
                };
                month = Some(m);
            } else {
                month = Some(value.parse().ok()?);
            }
        } else if t.contains('D') {
            day = Some(value.parse().ok()?);
        }
    }

    Some(format_ymd(year?, month?, day?))
}

fn parse_qif(content: &str, locale: Option<&ImportLocaleOptions>) -> Result<Vec<ImportDraft>, String> {
    let mut results = Vec::new();
    let mut current = ImportDraft {
        payee_name: None,
        memo: None,
        txn_date: None,
        happened_at_utc: None,
        posted_date: None,
        posted_at_utc: None,
        posted_tz: None,
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
                happened_at_utc: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
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
            current.txn_date = parse_csv_date(&line[1..], locale).or_else(|| parse_date_mmddyy(&line[1..]));
        } else if line.starts_with('T') {
            current.amount_minor = Some(parse_amount_to_minor_with_locale(&line[1..], 2, locale)?);
        } else if line.starts_with('P') {
            current.payee_name = Some(line[1..].trim().to_string());
        } else if line.starts_with('M') {
            current.memo = Some(line[1..].trim().to_string());
        } else if line.starts_with('N') {
            current.reference = Some(line[1..].trim().to_string());
        }
    }

    if current.txn_date.is_some() || current.amount_minor.is_some() {
        results.push(current);
    }

    Ok(results)
}

fn parse_ofx(content: &str, locale: Option<&ImportLocaleOptions>) -> Result<Vec<ImportDraft>, String> {
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
        happened_at_utc: None,
        posted_date: None,
        posted_at_utc: None,
        posted_tz: None,
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
                happened_at_utc: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
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
                happened_at_utc: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
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
            let raw = l.replace("<DTPOSTED>", "");
            let raw = raw.trim().to_string();
            current.txn_date = parse_ofx_date(&raw);
            current.posted_date = parse_ofx_date(&raw);
            current.posted_at_utc = parse_ofx_datetime_utc(&raw);
            if current.posted_at_utc.is_some() {
                current.posted_tz = Some("Etc/UTC".to_string());
            }
            if current.happened_at_utc.is_none() {
                current.happened_at_utc = current.posted_at_utc.clone();
            }
        } else if l.starts_with("<TRNAMT>") {
            current.amount_minor = Some(parse_amount_to_minor_with_locale(
                &l.replace("<TRNAMT>", ""),
                2,
                locale,
            )?);
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

fn parse_hbci_mt940(
    content: &str,
    locale: Option<&ImportLocaleOptions>,
) -> Result<Vec<ImportDraft>, String> {
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
            let amount_part = body.chars().skip(6).collect::<String>();
            let is_debit = amount_part.contains('D');
            let amt_str = amount_part
                .split(|c| c == 'C' || c == 'D')
                .nth(1)
                .unwrap_or("");
            let mut amount_minor =
                parse_amount_to_minor_with_locale(amt_str, 2, locale).unwrap_or(0);
            if is_debit {
                amount_minor = -amount_minor.abs();
            }
            current = Some(ImportDraft {
                payee_name: None,
                memo: None,
                txn_date: if date.is_empty() { None } else { Some(date) },
                happened_at_utc: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
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

fn parse_csv(
    content: &str,
    locale: Option<&ImportLocaleOptions>,
) -> Result<Vec<ImportDraft>, String> {
    let mut builder = ReaderBuilder::new();
    builder.has_headers(true).flexible(true);
    if let Some(locale) = locale {
        if let Some(delim) = locale.csv_delimiter.as_deref() {
            let delim = delim.chars().next().unwrap_or(',') as u8;
            builder.delimiter(delim);
        }
    }
    let mut reader = builder.from_reader(content.as_bytes());

    let headers = reader
        .headers()
        .map_err(|e| e.to_string())?
        .iter()
        .map(|h| h.trim().to_lowercase())
        .collect::<Vec<String>>();

    if headers.is_empty() {
        return Err("csv header row is required".to_string());
    }

    let idx = |names: &[&str]| -> Option<usize> {
        names
            .iter()
            .find_map(|name| headers.iter().position(|h| h == name))
    };

    let date_idx = idx(&["date", "txn_date", "transaction_date"]);
    let amount_idx = idx(&["amount", "amount_minor"]);
    let payee_idx = idx(&["payee", "payee_name"]);
    let memo_idx = idx(&["memo", "notes"]);
    let reference_idx = idx(&["reference", "ref"]);
    let import_id_idx = idx(&["import_id", "fitid"]);
    let currency_idx = idx(&["currency", "currency_code"]);

    if date_idx.is_none() && amount_idx.is_none() {
        return Err("csv must include at least date or amount column".to_string());
    }

    let mut results = Vec::new();
    for record in reader.records() {
        let record = record.map_err(|e| e.to_string())?;
        let get = |idx: Option<usize>| -> Option<String> {
            idx.and_then(|i| record.get(i))
                .map(|v| v.trim())
                .filter(|v| !v.is_empty())
                .map(|v| v.to_string())
        };

        let raw_date = get(date_idx);
        let raw_amount = get(amount_idx);
        let txn_date = raw_date.as_deref().and_then(|val| parse_csv_date(val, locale));
        let amount_minor = match raw_amount.as_deref() {
            Some(val) => Some(parse_amount_to_minor_with_locale(val, 2, locale)?),
            None => None,
        };

        let draft = ImportDraft {
            payee_name: get(payee_idx),
            memo: get(memo_idx),
            txn_date,
            happened_at_utc: None,
            posted_date: None,
            posted_at_utc: None,
            posted_tz: None,
            amount_minor,
            reference: get(reference_idx),
            import_id: get(import_id_idx),
            currency_code: get(currency_idx),
            account_id: None,
            category_id: None,
            payee_id: None,
        };

        if draft.txn_date.is_some() || draft.amount_minor.is_some() {
            results.push(draft);
        }
    }

    Ok(results)
}

fn looks_like_csv(content: &str) -> bool {
    let first_line = content.lines().find(|l| !l.trim().is_empty());
    let Some(line) = first_line else {
        return false;
    };
    let lower = line.to_lowercase();
    line.contains(',') && lower.contains("date") && lower.contains("amount")
}

#[command]
pub fn parse_import_file(format: String, content: String) -> Result<Vec<ImportDraft>, String> {
    parse_import_file_with_locale(format, content, None)
}

#[command]
pub fn parse_import_file_with_locale(
    format: String,
    content: String,
    locale: Option<ImportLocaleOptions>,
) -> Result<Vec<ImportDraft>, String> {
    let fmt = format.to_lowercase();
    let detected = if fmt == "auto" {
        if content.contains("<OFX>") {
            "ofx"
        } else if content.contains("!Type:") {
            "qif"
        } else if content.contains(":61:") {
            "hbci"
        } else if looks_like_csv(&content) {
            "csv"
        } else {
            return Err("unable to detect import format".to_string());
        }
    } else {
        fmt.as_str()
    };

    let locale_ref = locale.as_ref();
    match detected {
        "qif" => parse_qif(&content, locale_ref),
        "ofx" => parse_ofx(&content, locale_ref),
        "hbci" => parse_hbci_mt940(&content, locale_ref),
        "csv" => parse_csv(&content, locale_ref),
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
             WHERE s.account_id = ?1 AND t.occurred_date = ?2 AND s.amount_minor = ?3",
        );
        let mut params: Vec<Value> = vec![
            Value::from(account_id),
            Value::from(txn_date.clone()),
            Value::from(amount_minor),
        ];
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

fn resolve_payee_id(
    conn: &rusqlite::Connection,
    draft: &ImportDraft,
    create_payees: bool,
) -> Result<Option<i64>, String> {
    if let Some(pid) = draft.payee_id {
        return Ok(Some(pid));
    }
    if let Some(name) = draft.payee_name.as_ref() {
        let existing: Option<i64> = conn
            .query_row(
                                "SELECT p.id
                                 FROM payees p
                                 WHERE p.book_id = ?1
                                     AND p.name = ?2
                                     AND NOT EXISTS (SELECT 1 FROM payees newer WHERE newer.previous_payee_id = p.id)
                                 ORDER BY p.id DESC
                                 LIMIT 1",
                params![SINGLE_BOOK_ID, name],
                |row| row.get(0),
            )
            .optional()
            .map_err(|e| e.to_string())?;
        if let Some(id) = existing {
            return Ok(Some(id));
        }
        if create_payees {
            let session_id: String = conn
                .query_row("SELECT id FROM app_runtime_session LIMIT 1", [], |row| row.get(0))
                .map_err(|e| e.to_string())?;
            conn.execute(
                "INSERT INTO payees (
                    book_id, name, kind, previous_payee_id,
                          session_id, created_at
                 )
                 VALUES (
                    ?1, ?2, 'payee', NULL,
                          ?3,
                      strftime('%Y-%m-%dT%H:%M:%fZ','now')
                 )",
                params![SINGLE_BOOK_ID, name, session_id],
            )
            .map_err(|e| e.to_string())?;
            return Ok(Some(conn.last_insert_rowid()));
        }
    }
    Ok(None)
}

fn enrich_existing_transaction(
    conn: &rusqlite::Connection,
    tx_id: i64,
    draft: &ImportDraft,
    create_payees: bool,
    import_session_id: Option<i64>,
) -> Result<bool, String> {
    let (payee_id, memo, reference, import_id, existing_session_id): (
        Option<i64>,
        Option<String>,
        Option<String>,
        Option<String>,
        Option<i64>,
    ) = conn
        .query_row(
            "SELECT payee_id, memo, reference, import_id, import_session_id FROM transactions WHERE id = ?1",
            [tx_id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?)),
        )
        .map_err(|e| e.to_string())?;

    let new_payee_id = if payee_id.is_none() {
        resolve_payee_id(conn, draft, create_payees)?
    } else {
        None
    };
    let new_memo = if memo.as_deref().unwrap_or("").is_empty() {
        draft.memo.clone()
    } else {
        None
    };
    let new_reference = if reference.as_deref().unwrap_or("").is_empty() {
        draft.reference.clone()
    } else {
        None
    };
    let new_import_id = if import_id.as_deref().unwrap_or("").is_empty() {
        draft.import_id.clone()
    } else {
        None
    };
    let new_session_id = if existing_session_id.is_none() {
        import_session_id
    } else {
        None
    };

    if new_payee_id.is_some()
        || new_memo.is_some()
        || new_reference.is_some()
        || new_import_id.is_some()
        || new_session_id.is_some()
    {
        let session_id: String = conn
            .query_row("SELECT id FROM app_runtime_session LIMIT 1", [], |row| row.get(0))
            .map_err(|e| e.to_string())?;
        let (book_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz, status): (
            i64,
            String,
            Option<String>,
            Option<String>,
            String,
            Option<String>,
            Option<String>,
            String,
        ) = conn
            .query_row(
                "SELECT book_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz, status FROM transactions WHERE id = ?1",
                [tx_id],
                |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?, row.get(5)?, row.get(6)?, row.get(7)?)),
            )
            .map_err(|e| e.to_string())?;

        let resolved_payee_id = new_payee_id.or(payee_id);
        let resolved_memo = new_memo.or(memo);
        let resolved_reference = new_reference.or(reference);
        let resolved_import_id = new_import_id.or(import_id);
        let resolved_import_session_id = new_session_id.or(existing_session_id);

        conn.execute(
            "INSERT INTO transactions (
                book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
                payee_id, memo, status, reference, import_id, import_session_id,
                     session_id, created_at
             )
             VALUES (
                ?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8,
                ?9, ?10, ?11, ?12, ?13, ?14,
                     ?15, strftime('%Y-%m-%dT%H:%M:%fZ','now')
             )",
            params![
                book_id,
                tx_id,
                occurred_date,
                occurred_at_utc,
                occurred_tz,
                posted_date,
                posted_at_utc,
                posted_tz,
                resolved_payee_id,
                resolved_memo,
                status,
                resolved_reference,
                resolved_import_id,
                resolved_import_session_id,
                session_id,
            ],
        )
        .map_err(|e| e.to_string())?;
        let new_tx_id = conn.last_insert_rowid();
        conn.execute("DELETE FROM session_redo_stack WHERE session_id = ?1", [session_id.as_str()])
            .map_err(|e| e.to_string())?;
        conn.execute(
            "INSERT INTO session_undo_stack (session_id, table_name, row_id) VALUES (?1, 'transactions', ?2)",
            params![session_id, new_tx_id],
        )
        .map_err(|e| e.to_string())?;
        return Ok(true);
    }

    Ok(false)
}

fn record_import_session_tx(
    conn: &rusqlite::Connection,
    session_id: i64,
    tx_id: i64,
    action: &str,
) -> Result<(), String> {
    conn.execute(
        "INSERT OR IGNORE INTO import_session_transactions (session_id, tx_id, action)
         VALUES (?1, ?2, ?3)",
        params![session_id, tx_id, action],
    )
    .map_err(|e| e.to_string())?;
    Ok(())
}

fn get_account_commodity_symbol(
    conn: &rusqlite::Connection,
    account_id: i64,
) -> Result<Option<String>, String> {
    conn.query_row(
        "SELECT c.symbol
         FROM accounts a
         JOIN commodities c ON c.id = a.commodity_id
         WHERE a.id = ?1",
        [account_id],
        |row| row.get(0),
    )
    .optional()
    .map_err(|e| e.to_string())
}

fn check_account_locks(
    conn: &rusqlite::Connection,
    txn_date: &str,
    account_ids: &std::collections::HashSet<i64>,
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

fn check_transaction_locks(conn: &rusqlite::Connection, tx_id: i64) -> Result<(), String> {
    let txn_date: Option<String> = conn
        .query_row(
            "SELECT occurred_date FROM transactions WHERE id = ?1",
            [tx_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some(txn_date) = txn_date else {
        return Err("transaction not found".to_string());
    };

    let mut stmt = conn
        .prepare("SELECT DISTINCT account_id FROM splits WHERE tx_id = ?1")
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([tx_id], |row| row.get::<_, i64>(0))
        .map_err(|e| e.to_string())?;

    let mut accounts = std::collections::HashSet::new();
    for row in rows {
        accounts.insert(row.map_err(|e| e.to_string())?);
    }

    check_account_locks(conn, &txn_date, &accounts)
}

#[command]
pub fn list_import_session_transactions(
    db: State<DbState>,
    session_id: i64,
) -> Result<Vec<ImportSessionTransactionInfo>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT ist.session_id, ist.tx_id, ist.action, ist.created_at,
                    s.source, s.file_name, s.file_hash, s.file_size, s.started_at, s.committed_at
             FROM import_session_transactions ist
             JOIN import_sessions s ON s.id = ist.session_id
             WHERE ist.session_id = ?1
             ORDER BY ist.id ASC",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([session_id], |row| {
            Ok(ImportSessionTransactionInfo {
                session_id: row.get(0)?,
                tx_id: row.get(1)?,
                action: row.get(2)?,
                created_at: row.get(3)?,
                source: row.get(4)?,
                file_name: row.get(5)?,
                file_hash: row.get(6)?,
                file_size: row.get(7)?,
                started_at: row.get(8)?,
                committed_at: row.get(9)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut results = Vec::new();
    for row in rows {
        results.push(row.map_err(|e| e.to_string())?);
    }

    Ok(results)
}

#[command]
pub fn list_transaction_import_sessions(
    db: State<DbState>,
    tx_id: i64,
) -> Result<Vec<ImportSessionTransactionInfo>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT ist.session_id, ist.tx_id, ist.action, ist.created_at,
                    s.source, s.file_name, s.file_hash, s.file_size, s.started_at, s.committed_at
             FROM import_session_transactions ist
             JOIN import_sessions s ON s.id = ist.session_id
             WHERE ist.tx_id = ?1
             ORDER BY ist.id ASC",
        )
        .map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map([tx_id], |row| {
            Ok(ImportSessionTransactionInfo {
                session_id: row.get(0)?,
                tx_id: row.get(1)?,
                action: row.get(2)?,
                created_at: row.get(3)?,
                source: row.get(4)?,
                file_name: row.get(5)?,
                file_hash: row.get(6)?,
                file_size: row.get(7)?,
                started_at: row.get(8)?,
                committed_at: row.get(9)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut results = Vec::new();
    for row in rows {
        results.push(row.map_err(|e| e.to_string())?);
    }

    Ok(results)
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
    mode: Option<ImportMode>,
    import_session_id: Option<i64>,
) -> Result<ImportBatchResult, String> {
    let mode = mode.unwrap_or(ImportMode::MatchAndEnrich);

    if mode == ImportMode::Review {
        let matches = match_import_transactions(db.clone(), account_id, drafts)?;
        let matched_tx_ids = matches
            .iter()
            .filter_map(|m| m.matched_tx_id)
            .collect::<Vec<_>>();

        if let Some(session_id) = import_session_id {
            let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
            let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
            for tx_id in &matched_tx_ids {
                record_import_session_tx(conn, session_id, *tx_id, "validated")?;
            }
        }

        return Ok(ImportBatchResult {
            created_tx_ids: Vec::new(),
            matched_tx_ids,
            updated_tx_ids: Vec::new(),
            skipped: 0,
            review_matches: Some(matches),
        });
    }

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
            "INSERT INTO accounts (book_id, parent_id, type, name, commodity_id, is_closed, created_at)
             VALUES (?1, NULL, 'equity', 'Imbalance-Import', ?2, 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![SINGLE_BOOK_ID, account_commodity_id],
        )
        .map_err(|e| e.to_string())?;
        tx.last_insert_rowid()
    } else {
        imbalance_account_id
    };

    let mut created = Vec::new();
    let mut matched = Vec::new();
    let mut updated = Vec::new();
    let mut skipped: i64 = 0;

    for draft in drafts {
        let target_account_id = draft.account_id.unwrap_or(account_id);
        if let Some(code) = draft.currency_code.as_ref() {
            let symbol = get_account_commodity_symbol(&tx, target_account_id)?;
            if let Some(symbol) = symbol {
                if !symbol.eq_ignore_ascii_case(code) {
                    skipped += 1;
                    continue;
                }
            }
        }
        let match_id = find_matching_tx(&tx, target_account_id, &draft)?;
        if let Some(id) = match_id {
            match mode {
                ImportMode::AlwaysCreate => {}
                ImportMode::Deduplicate => {
                    if let Some(session_id) = import_session_id {
                        record_import_session_tx(&tx, session_id, id, "validated")?;
                    }
                    matched.push(id);
                    continue;
                }
                ImportMode::MatchAndEnrich => {
                    check_transaction_locks(&tx, id)?;
                    if enrich_existing_transaction(&tx, id, &draft, create_payees, import_session_id)? {
                        if let Some(session_id) = import_session_id {
                            record_import_session_tx(&tx, session_id, id, "updated")?;
                        }
                        updated.push(id);
                    } else if let Some(session_id) = import_session_id {
                        record_import_session_tx(&tx, session_id, id, "validated")?;
                    }
                    matched.push(id);
                    continue;
                }
                ImportMode::UpdateOnly => {
                    check_transaction_locks(&tx, id)?;
                    if enrich_existing_transaction(&tx, id, &draft, create_payees, import_session_id)? {
                        if let Some(session_id) = import_session_id {
                            record_import_session_tx(&tx, session_id, id, "updated")?;
                        }
                        updated.push(id);
                    } else if let Some(session_id) = import_session_id {
                        record_import_session_tx(&tx, session_id, id, "validated")?;
                    }
                    matched.push(id);
                    continue;
                }
                ImportMode::Review => {}
            }
        } else if mode == ImportMode::UpdateOnly {
            skipped += 1;
            continue;
        }

        let txn_date = match draft.txn_date.clone() {
            Some(d) => d,
            None => {
                skipped += 1;
                continue;
            }
        };
        let occurred_at_utc = draft.happened_at_utc.clone();
        let occurred_tz = occurred_at_utc
            .as_ref()
            .map(|_| "Etc/UTC".to_string());
        let posted_at_utc = draft.posted_at_utc.clone();
        let posted_tz = if posted_at_utc.is_some() {
            draft.posted_tz.clone().or_else(|| Some("Etc/UTC".to_string()))
        } else {
            draft.posted_tz.clone()
        };
        let posted_date = draft
            .posted_date
            .clone()
            .or_else(|| posted_at_utc.as_ref().and_then(|value| value.get(0..10).map(|v| v.to_string())))
            .unwrap_or_else(|| txn_date.clone());
        let amount_minor = match draft.amount_minor {
            Some(a) => a,
            None => {
                skipped += 1;
                continue;
            }
        };

        let mut accounts = std::collections::HashSet::new();
        accounts.insert(target_account_id);
        accounts.insert(imbalance_account_id);
        check_account_locks(&tx, &txn_date, &accounts)?;

        let payee_id = resolve_payee_id(&tx, &draft, create_payees)?;

        tx.execute(
                "INSERT INTO transactions (
                     book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
                     payee_id, memo, status, reference, import_id, import_session_id, created_at
                 )
                 VALUES (
                     ?1, NULL, ?2, ?3, ?4, ?5, ?6, ?7,
                     ?8, ?9, 'uncleared', ?10, ?11, ?12, strftime('%Y-%m-%dT%H:%M:%fZ','now')
                 )",
            params![
                SINGLE_BOOK_ID,
                txn_date,
                occurred_at_utc,
                occurred_tz,
                posted_date,
                posted_at_utc,
                posted_tz,
                payee_id,
                draft.memo,
                draft.reference,
                draft.import_id,
                import_session_id,
            ],
        )
        .map_err(|e| e.to_string())?;

        let tx_id = tx.last_insert_rowid();

        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
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
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
             VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![tx_id, imbalance_account_id, account_commodity_id, -amount_minor],
        )
        .map_err(|e| e.to_string())?;

        created.push(tx_id);
        if let Some(session_id) = import_session_id {
            record_import_session_tx(&tx, session_id, tx_id, "created")?;
        }
    }

    tx.commit().map_err(|e| e.to_string())?;

    Ok(ImportBatchResult {
        created_tx_ids: created,
        matched_tx_ids: matched,
        updated_tx_ids: updated,
        skipped,
        review_matches: None,
    })
}

#[command]
pub fn create_import_rule(
    db: State<DbState>,
    input: ImportRuleCreate,
) -> Result<ImportRule, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

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
        "INSERT INTO import_rules (
            book_id, rule_kind, match_type, match_text, priority,
            amount_min_minor, amount_max_minor, date_from, date_to,
            match_account_id, target_account_id, target_category_id, target_payee_id,
                previous_import_rule_id, session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5,
            ?6, ?7, ?8, ?9,
            ?10, ?11, ?12, ?13,
                NULL, ?14, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
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
            session_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let rule = conn
        .query_row(
            "SELECT id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at
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
            "SELECT id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at
                         FROM import_rules
                         WHERE book_id = ?1
                            AND (import_rules.session_id IS NULL OR import_rules.session_id NOT LIKE 'void:%')
                             AND NOT EXISTS (
                                     SELECT 1 FROM import_rules newer
                                     WHERE newer.previous_import_rule_id = import_rules.id
                             )
                         ORDER BY priority ASC, id ASC",
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

    let source: Option<(i64, i64, String, String, String, i64, Option<i64>, Option<i64>, Option<String>, Option<String>, Option<i64>, Option<i64>, Option<i64>, Option<i64>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_import_rule_id
                 FROM import_rules a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_import_rule_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM import_rules a
                 JOIN chain c ON a.previous_import_rule_id = c.id
             )
             SELECT id, book_id, rule_kind, match_type, match_text, priority,
                    amount_min_minor, amount_max_minor, date_from, date_to,
                    match_account_id, target_account_id, target_category_id, target_payee_id
             FROM import_rules
             WHERE id IN chain
               AND (import_rules.session_id IS NULL OR import_rules.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM import_rules newer
                   WHERE newer.previous_import_rule_id = import_rules.id
                     AND newer.id IN chain
               )
             LIMIT 1",
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
                    row.get(9)?,
                    row.get(10)?,
                    row.get(11)?,
                    row.get(12)?,
                    row.get(13)?,
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO import_rules (
            book_id, rule_kind, match_type, match_text, priority,
            amount_min_minor, amount_max_minor, date_from, date_to,
            match_account_id, target_account_id, target_category_id, target_payee_id,
            previous_import_rule_id, session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5,
            ?6, ?7, ?8, ?9,
            ?10, ?11, ?12, ?13,
            ?14, ?15, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            rule_kind,
            match_type,
            match_text,
            priority,
            amount_min_minor,
            amount_max_minor,
            date_from,
            date_to,
            match_account_id,
            target_account_id,
            target_category_id,
            target_payee_id,
            current_id,
            format!("{}delete", VOID_SESSION_PREFIX),
        ],
    )
    .map_err(|e| e.to_string())?;

    Ok(true)
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
        "INSERT INTO import_sessions (book_id, source, file_name, file_hash, file_size, status, started_at, notes)
         VALUES (?1, ?2, ?3, ?4, ?5, 'started', strftime('%Y-%m-%dT%H:%M:%fZ','now'), ?6)",
        params![
            book_id,
            input.source,
            input.file_name,
            input.file_hash,
            input.file_size,
            input.notes,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let session = conn
        .query_row(
            "SELECT id, book_id, source, file_name, file_hash, file_size, status, started_at, committed_at, notes
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
            "SELECT id, book_id, source, file_name, file_hash, file_size, status, started_at, committed_at, notes
             FROM import_sessions WHERE id = ?1",
            [session_id],
            map_import_session_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(session)
}

#[command]
pub fn import_transactions_with_session(
    db: State<DbState>,
    input: ImportRunInput,
) -> Result<ImportRunResult, String> {
    let session = start_import_session(
        db.clone(),
        ImportSessionStartInput {
            book_id: SINGLE_BOOK_ID,
            source: input.source,
            file_name: input.file_name,
            file_hash: input.file_hash,
            file_size: input.file_size,
            notes: input.notes,
        },
    )?;

    let batch = import_transactions(
        db.clone(),
        input.account_id,
        input.drafts,
        input.create_payees,
        input.mode,
        Some(session.id),
    )?;

    let committed = commit_import_session(db, session.id)?;

    Ok(ImportRunResult {
        session: committed,
        batch,
    })
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
            "SELECT id, book_id, rule_kind, match_type, match_text, priority, amount_min_minor, amount_max_minor, date_from, date_to, match_account_id, target_account_id, target_category_id, target_payee_id, created_at
                         FROM import_rules
                         WHERE book_id = ?1
                            AND (import_rules.session_id IS NULL OR import_rules.session_id NOT LIKE 'void:%')
                             AND NOT EXISTS (
                                     SELECT 1 FROM import_rules newer
                                     WHERE newer.previous_import_rule_id = import_rules.id
                             )
                         ORDER BY priority ASC, id ASC",
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
                        let from_ok = rule
                            .date_from
                            .as_deref()
                            .map(|d| txn_date >= d)
                            .unwrap_or(true);
                        let to_ok = rule
                            .date_to
                            .as_deref()
                            .map(|d| txn_date <= d)
                            .unwrap_or(true);
                        matched = from_ok && to_ok;
                    }
                }
                "account" => {
                    if let (Some(match_account_id), Some(account_id)) =
                        (rule.match_account_id, draft.account_id)
                    {
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::open_and_migrate;
    use crate::state::DbStateInner;
    use rusqlite::params;
    use std::fs;
    use std::sync::{
        atomic::{AtomicUsize, Ordering},
        Mutex,
    };
    use std::time::{SystemTime, UNIX_EPOCH};

    static TEST_COUNTER: AtomicUsize = AtomicUsize::new(0);

    fn create_temp_db() -> DbState {
        let start = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis();
        let counter = TEST_COUNTER.fetch_add(1, Ordering::SeqCst);
        let mut temp = std::env::temp_dir();
        temp.push(format!("rekenraam_import_test_{start}_{counter}"));
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

    #[test]
    fn test_import_rules_and_sessions() {
        let db_state = create_temp_db();

        let err = create_import_rule(
            as_state(&db_state),
            ImportRuleCreate {
                book_id: 1,
                rule_kind: "payee".to_string(),
                match_type: Some("invalid".to_string()),
                match_text: "Amazon".to_string(),
                priority: None,
                amount_min_minor: None,
                amount_max_minor: None,
                date_from: None,
                date_to: None,
                match_account_id: None,
                target_account_id: None,
                target_category_id: None,
                target_payee_id: None,
            },
        )
        .expect_err("invalid match_type should error");
        assert!(err.contains("match_type"));

        let rule = create_import_rule(
            as_state(&db_state),
            ImportRuleCreate {
                book_id: 1,
                rule_kind: "memo".to_string(),
                match_type: Some("contains".to_string()),
                match_text: "Coffee".to_string(),
                priority: Some(10),
                amount_min_minor: None,
                amount_max_minor: None,
                date_from: None,
                date_to: None,
                match_account_id: None,
                target_account_id: None,
                target_category_id: None,
                target_payee_id: None,
            },
        )
        .expect("create import rule");

        let rules = list_import_rules(as_state(&db_state), 1).expect("list rules");
        assert!(rules.iter().any(|r| r.id == rule.id));

        let deleted = delete_import_rule(as_state(&db_state), rule.id).expect("delete rule");
        assert!(deleted);

        let session = start_import_session(
            as_state(&db_state),
            ImportSessionStartInput {
                book_id: 1,
                source: Some("qif".to_string()),
                file_name: None,
                file_hash: None,
                file_size: None,
                notes: Some("test".to_string()),
            },
        )
        .expect("start session");
        assert_eq!(session.status, "started");

        let committed = commit_import_session(as_state(&db_state), session.id)
            .expect("commit session");
        assert_eq!(committed.status, "committed");
    }

    #[test]
    fn test_import_rule_kinds_create() {
        let db_state = create_temp_db();

        let kinds = ["payee", "memo", "amount", "date", "account"];
        for (idx, kind) in kinds.iter().enumerate() {
            let rule = create_import_rule(
                as_state(&db_state),
                ImportRuleCreate {
                    book_id: 1,
                    rule_kind: kind.to_string(),
                    match_type: Some("contains".to_string()),
                    match_text: format!("rule-{idx}"),
                    priority: None,
                    amount_min_minor: None,
                    amount_max_minor: None,
                    date_from: None,
                    date_to: None,
                    match_account_id: None,
                    target_account_id: None,
                    target_category_id: None,
                    target_payee_id: None,
                },
            )
            .expect("create import rule kind");
            assert_eq!(rule.rule_kind, *kind);
        }
    }

    #[test]
    fn test_parse_import_formats_and_matching() {
        let qif = "!Type:Bank\nD01/02/2024\nT-12.34\nPTest Store\nMNote\n^";
        let qif_drafts = parse_import_file("qif".to_string(), qif.to_string())
            .expect("parse qif");
        assert_eq!(qif_drafts.len(), 1);
        assert_eq!(qif_drafts[0].amount_minor, Some(-1234));

        let ofx = "<OFX>\n<STMTTRN>\n<DTPOSTED>20240102\n<TRNAMT>-45.67\n<NAME>OFX Payee\n<MEMO>Memo\n<FITID>123\n</STMTTRN>\n";
        let ofx_drafts = parse_import_file("ofx".to_string(), ofx.to_string())
            .expect("parse ofx");
        assert_eq!(ofx_drafts.len(), 1);
        assert_eq!(ofx_drafts[0].amount_minor, Some(-4567));

        let hbci = ":61:240102C12,34NTRFNONREF\n:86:HB memo";
        let hbci_drafts = parse_import_file("hbci".to_string(), hbci.to_string())
            .expect("parse hbci");
        assert_eq!(hbci_drafts.len(), 1);
        assert_eq!(hbci_drafts[0].amount_minor, Some(1234));

        let csv = "date,amount,payee,memo,reference,import_id,currency\n2024-01-05,-10.50,Cafe,Latte,REF1,IMP1,USD";
        let csv_drafts = parse_import_file("csv".to_string(), csv.to_string())
            .expect("parse csv");
        assert_eq!(csv_drafts.len(), 1);
        assert_eq!(csv_drafts[0].amount_minor, Some(-1050));
        assert_eq!(csv_drafts[0].payee_name.as_deref(), Some("Cafe"));

        let csv_semicolon = "date;amount;payee\n2024-01-05;-10,50;Cafe";
        let csv_locale = parse_import_file_with_locale(
            "csv".to_string(),
            csv_semicolon.to_string(),
            Some(ImportLocaleOptions {
                decimal_separator: Some(",".to_string()),
                thousands_separator: Some(".".to_string()),
                date_format: Some("YMD".to_string()),
                csv_delimiter: Some(";".to_string()),
            }),
        )
        .expect("parse csv with locale");
        assert_eq!(csv_locale.len(), 1);
        assert_eq!(csv_locale[0].amount_minor, Some(-1050));
        assert_eq!(csv_locale[0].payee_name.as_deref(), Some("Cafe"));

        let auto_csv = parse_import_file("auto".to_string(), csv.to_string())
            .expect("auto detect csv");
        assert_eq!(auto_csv.len(), 1);

        let auto_drafts = parse_import_file("auto".to_string(), ofx.to_string())
            .expect("auto detect");
        assert_eq!(auto_drafts.len(), 1);

        let db_state = create_temp_db();
        let checking_id = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            lookup_account_id(conn, "Checking Account")
        };

        let draft = ImportDraft {
            payee_name: Some("Match Payee".to_string()),
            memo: None,
            txn_date: Some("2024-04-01".to_string()),
            happened_at_utc: None,
            posted_date: None,
            posted_at_utc: None,
            posted_tz: None,
            amount_minor: Some(-2500),
            reference: None,
            import_id: Some("match-1".to_string()),
            currency_code: None,
            account_id: None,
            category_id: None,
            payee_id: None,
        };

        let draft_match = ImportDraft {
            payee_name: Some("Match Payee".to_string()),
            memo: None,
            txn_date: Some("2024-04-01".to_string()),
            happened_at_utc: None,
            posted_date: None,
            posted_at_utc: None,
            posted_tz: None,
            amount_minor: Some(-2500),
            reference: None,
            import_id: Some("match-1".to_string()),
            currency_code: None,
            account_id: None,
            category_id: None,
            payee_id: None,
        };

        let created = import_transactions(
            as_state(&db_state),
            checking_id,
            vec![draft],
            false,
            Some(ImportMode::AlwaysCreate),
            None,
        )
        .expect("import transactions");
        assert_eq!(created.created_tx_ids.len(), 1);
        assert_eq!(created.skipped, 0);

        let matches = match_import_transactions(
            as_state(&db_state),
            checking_id,
            vec![draft_match],
        )
        .expect("match imports");
        assert_eq!(matches.len(), 1);
        assert!(matches[0].matched_tx_id.is_some());
    }

    #[test]
    fn test_import_negative_paths_and_duplicates() {
        let db_state = create_temp_db();
        let (checking_id, commodity_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let checking_id = lookup_account_id(conn, "Checking Account");
            let commodity_id = lookup_usd_id(conn);
            (checking_id, commodity_id)
        };

        let drafts = vec![
            ImportDraft {
                payee_name: Some("Missing Amount".to_string()),
                memo: None,
                txn_date: Some("2024-06-01".to_string()),
                happened_at_utc: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
                amount_minor: None,
                reference: None,
                import_id: None,
                currency_code: None,
                account_id: None,
                category_id: None,
                payee_id: None,
            },
            ImportDraft {
                payee_name: Some("Missing Date".to_string()),
                memo: None,
                txn_date: None,
                happened_at_utc: None,
                posted_date: None,
                posted_at_utc: None,
                posted_tz: None,
                amount_minor: Some(-1234),
                reference: None,
                import_id: None,
                currency_code: None,
                account_id: None,
                category_id: None,
                payee_id: None,
            },
        ];

        let result = import_transactions(
            as_state(&db_state),
            checking_id,
            drafts,
            false,
            Some(ImportMode::AlwaysCreate),
            None,
        )
        .expect("import transactions");
        assert_eq!(result.created_tx_ids.len(), 0);
        assert_eq!(result.skipped, 2);

        let tx_id = {
            let mut guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_mut().expect("conn");
            conn.execute(
                "INSERT INTO transactions (book_id, occurred_date, payee_id, memo, status, reference, import_id, created_at)
                 VALUES (1, '2024-06-05', NULL, 'Duplicate Memo', 'cleared', NULL, 'dup-1', strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert tx");
            let tx_id = conn.last_insert_rowid();
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -5000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx_id, checking_id, commodity_id],
            )
            .expect("insert split");
            tx_id
        };

        let match_by_import_id = ImportDraft {
            payee_name: None,
            memo: Some("Duplicate Memo".to_string()),
            txn_date: Some("2024-06-05".to_string()),
            happened_at_utc: None,
            posted_date: None,
            posted_at_utc: None,
            posted_tz: None,
            amount_minor: Some(-5000),
            reference: None,
            import_id: Some("dup-1".to_string()),
            currency_code: None,
            account_id: None,
            category_id: None,
            payee_id: None,
        };

        let matches = match_import_transactions(
            as_state(&db_state),
            checking_id,
            vec![match_by_import_id],
        )
        .expect("match by import_id");
        assert_eq!(matches[0].matched_tx_id, Some(tx_id));

        let match_by_fields = ImportDraft {
            payee_name: None,
            memo: Some("Duplicate Memo".to_string()),
            txn_date: Some("2024-06-05".to_string()),
            happened_at_utc: None,
            posted_date: None,
            posted_at_utc: None,
            posted_tz: None,
            amount_minor: Some(-5000),
            reference: None,
            import_id: None,
            currency_code: None,
            account_id: None,
            category_id: None,
            payee_id: None,
        };

        let matches = match_import_transactions(
            as_state(&db_state),
            checking_id,
            vec![match_by_fields],
        )
        .expect("match by fields");
        assert_eq!(matches[0].matched_tx_id, Some(tx_id));
    }
}

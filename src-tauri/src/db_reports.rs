use std::collections::HashMap;

use rusqlite::{params, params_from_iter, types::{Value, ValueRef}, OptionalExtension};
use serde::{Deserialize, Serialize};
use serde_json::Value as JsonValue;
use sha2::Digest;
use tauri::{command, State};

use crate::state::DbState;

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportDefinition {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub query_type: String,
    pub query_text: String,
    pub params_schema: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportDefinitionCreate {
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub query_type: String,
    pub query_text: String,
    pub params_schema: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportDefinitionUpdate {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub query_type: String,
    pub query_text: String,
    pub params_schema: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportRunResult {
    pub definition_id: i64,
    pub params_hash: String,
    pub as_of_seq: i64,
    pub result_json: JsonValue,
    pub cached: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct BuiltinReport {
    pub name: String,
    pub description: String,
    pub query_type: String,
    pub query_text: String,
    pub params_schema: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportAccountBalancesInput {
    pub book_id: i64,
    pub date_to: Option<String>,
    pub account_ids: Option<Vec<i64>>,
    pub include_closed: Option<bool>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountBalanceRow {
    pub account_id: i64,
    pub account_name: String,
    pub commodity_id: i64,
    pub balance_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportCashflowInput {
    pub book_id: i64,
    pub date_from: Option<String>,
    pub date_to: Option<String>,
    pub group_by: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CashflowRow {
    pub period_start: String,
    pub inflow_minor: i64,
    pub outflow_minor: i64,
    pub net_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportCategorySpendInput {
    pub book_id: i64,
    pub date_from: Option<String>,
    pub date_to: Option<String>,
    pub category_ids: Option<Vec<i64>>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CategorySpendRow {
    pub category_id: i64,
    pub category_name: String,
    pub total_minor: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ReportPayeeTotalsInput {
    pub book_id: i64,
    pub date_from: Option<String>,
    pub date_to: Option<String>,
    pub payee_ids: Option<Vec<i64>>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PayeeTotalRow {
    pub payee_id: i64,
    pub payee_name: String,
    pub total_minor: i64,
}

#[derive(Debug)]
struct ParamSchemaSpec {
    required: Vec<String>,
    allowed: Vec<String>,
    param_order: Vec<String>,
    types: HashMap<String, String>,
    enums: HashMap<String, Vec<JsonValue>>,
    date_fields: Vec<String>,
}

#[derive(Debug)]
struct TemplateFilter {
    param: String,
    clause: String,
}

const DEFAULT_REPORT_ROW_LIMIT: usize = 1000;
const MAX_REPORT_ROW_LIMIT: usize = 10_000;

fn map_report_definition_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<ReportDefinition> {
    Ok(ReportDefinition {
        id: row.get(0)?,
        book_id: row.get(1)?,
        name: row.get(2)?,
        kind: row.get(3)?,
        query_type: row.get(4)?,
        query_text: row.get(5)?,
        params_schema: row.get(6)?,
        created_at: row.get(7)?,
        updated_at: row.get(8)?,
    })
}

fn canonicalize_json(value: &JsonValue) -> JsonValue {
    match value {
        JsonValue::Object(map) => {
            let mut keys: Vec<String> = map.keys().cloned().collect();
            keys.sort();
            let mut new_map = serde_json::Map::new();
            for key in keys {
                let val = map.get(&key).unwrap();
                new_map.insert(key, canonicalize_json(val));
            }
            JsonValue::Object(new_map)
        }
        JsonValue::Array(items) => {
            JsonValue::Array(items.iter().map(canonicalize_json).collect())
        }
        _ => value.clone(),
    }
}

fn hash_params(value: &JsonValue) -> Result<String, String> {
    let canonical = canonicalize_json(value);
    let json = serde_json::to_string(&canonical).map_err(|e| e.to_string())?;
    let digest = sha2::Sha256::digest(json.as_bytes());
    let mut hex = String::new();
    for b in digest {
        hex.push_str(&format!("{:02x}", b));
    }
    Ok(hex)
}

fn parse_param_schema(schema: Option<&str>) -> Result<Option<ParamSchemaSpec>, String> {
    let Some(schema_str) = schema else {
        return Ok(None);
    };
    let value: JsonValue = serde_json::from_str(schema_str)
        .map_err(|e| format!("invalid params_schema json: {e}"))?;
    let obj = value.as_object().ok_or_else(|| "params_schema must be a JSON object".to_string())?;

    let required = obj
        .get("required")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    let mut allowed = obj
        .get("allowed")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    if allowed.is_empty() && !required.is_empty() {
        allowed = required.clone();
    }

    let param_order = obj
        .get("param_order")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    let types = obj
        .get("types")
        .and_then(|v| v.as_object())
        .map(|map| {
            map.iter()
                .filter_map(|(k, v)| v.as_str().map(|s| (k.clone(), s.to_string())))
                .collect::<HashMap<_, _>>()
        })
        .unwrap_or_default();

    let enums = obj
        .get("enums")
        .and_then(|v| v.as_object())
        .map(|map| {
            map.iter()
                .map(|(k, v)| {
                    let values = v
                        .as_array()
                        .map(|arr| arr.clone())
                        .unwrap_or_default();
                    (k.clone(), values)
                })
                .collect::<HashMap<_, _>>()
        })
        .unwrap_or_default();

    let date_fields = obj
        .get("date_fields")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    Ok(Some(ParamSchemaSpec {
        required,
        allowed,
        param_order,
        types,
        enums,
        date_fields,
    }))
}

fn is_valid_ymd(value: &str) -> bool {
    if value.len() != 10 {
        return false;
    }
    let bytes = value.as_bytes();
    if bytes[4] != b'-' || bytes[7] != b'-' {
        return false;
    }
    value[0..4].chars().all(|c| c.is_ascii_digit())
        && value[5..7].chars().all(|c| c.is_ascii_digit())
        && value[8..10].chars().all(|c| c.is_ascii_digit())
}

fn check_type(value: &JsonValue, type_spec: &str) -> bool {
    if type_spec.starts_with("array:") {
        let subtype = type_spec.trim_start_matches("array:");
        let Some(arr) = value.as_array() else {
            return false;
        };
        return arr.iter().all(|v| check_type(v, subtype));
    }

    match type_spec {
        "string" => value.is_string(),
        "number" => value.is_number(),
        "integer" => value.as_i64().is_some(),
        "boolean" => value.is_boolean(),
        "array" => value.is_array(),
        "object" => value.is_object(),
        _ => true,
    }
}

fn validate_params(params: &JsonValue, schema: &ParamSchemaSpec) -> Result<(), String> {
    let obj = params.as_object().ok_or_else(|| "params must be a JSON object".to_string())?;

    for key in &schema.required {
        if !obj.contains_key(key) || obj.get(key).is_none() || obj.get(key) == Some(&JsonValue::Null) {
            return Err(format!("missing required param: {key}"));
        }
    }

    if !schema.allowed.is_empty() {
        for key in obj.keys() {
            if !schema.allowed.contains(key) {
                return Err(format!("unknown param: {key}"));
            }
        }
    }

    for (key, type_spec) in &schema.types {
        if let Some(value) = obj.get(key) {
            if value.is_null() {
                continue;
            }
            if !check_type(value, type_spec) {
                return Err(format!("invalid param type for {key}"));
            }
        }
    }

    for (key, allowed_values) in &schema.enums {
        if let Some(value) = obj.get(key) {
            if value.is_null() {
                continue;
            }
            if !allowed_values.iter().any(|v| v == value) {
                return Err(format!("invalid param value for {key}"));
            }
        }
    }

    for key in &schema.date_fields {
        if let Some(value) = obj.get(key) {
            if value.is_null() {
                continue;
            }
            let Some(date) = value.as_str() else {
                return Err(format!("invalid date param for {key}"));
            };
            if !is_valid_ymd(date) {
                return Err(format!("invalid date format for {key}"));
            }
        }
    }

    Ok(())
}

fn json_to_sql_value(value: &JsonValue) -> Result<Value, String> {
    match value {
        JsonValue::Null => Ok(Value::Null),
        JsonValue::Bool(b) => Ok(Value::Integer(if *b { 1 } else { 0 })),
        JsonValue::Number(num) => {
            if let Some(i) = num.as_i64() {
                Ok(Value::Integer(i))
            } else if let Some(f) = num.as_f64() {
                Ok(Value::Real(f))
            } else {
                Err("unsupported number value".to_string())
            }
        }
        JsonValue::String(s) => Ok(Value::Text(s.clone())),
        JsonValue::Array(_) | JsonValue::Object(_) => {
            serde_json::to_string(value)
                .map(Value::Text)
                .map_err(|e| e.to_string())
        }
    }
}

fn build_param_values(params: &JsonValue, order: &[String]) -> Result<Vec<Value>, String> {
    if order.is_empty() {
        return Ok(Vec::new());
    }
    let obj = params.as_object().ok_or_else(|| "params must be a JSON object".to_string())?;
    let mut values = Vec::new();
    for key in order {
        let value = obj.get(key).unwrap_or(&JsonValue::Null);
        values.push(json_to_sql_value(value)?);
    }
    Ok(values)
}

fn is_select_query(sql: &str) -> bool {
    let trimmed = sql.trim_start().to_lowercase();
    trimmed.starts_with("select") || trimmed.starts_with("with")
}

fn apply_row_limit(sql: &str, limit: usize) -> String {
    let trimmed = sql.trim().trim_end_matches(';');
    let lower = trimmed.to_lowercase();
    if lower.contains(" limit ") {
        trimmed.to_string()
    } else {
        format!("{trimmed} LIMIT {limit}")
    }
}

fn resolve_row_limit(params: &JsonValue) -> usize {
    let mut limit = std::env::var("REKENRAAM_REPORT_ROW_LIMIT")
        .ok()
        .and_then(|v| v.parse::<usize>().ok())
        .unwrap_or(DEFAULT_REPORT_ROW_LIMIT);

    if let Some(custom) = params
        .get("limit")
        .and_then(|v| v.as_u64())
        .map(|v| v as usize)
    {
        if custom > 0 {
            limit = custom;
        }
    }

    limit.clamp(1, MAX_REPORT_ROW_LIMIT)
}

fn row_to_json(row: &rusqlite::Row<'_>, col_names: &[String]) -> Result<JsonValue, String> {
    let mut map = serde_json::Map::new();
    for (idx, name) in col_names.iter().enumerate() {
        let value = match row.get_ref(idx).map_err(|e| e.to_string())? {
            ValueRef::Null => JsonValue::Null,
            ValueRef::Integer(i) => JsonValue::Number(i.into()),
            ValueRef::Real(f) => serde_json::Number::from_f64(f)
                .map(JsonValue::Number)
                .unwrap_or(JsonValue::Null),
            ValueRef::Text(t) => JsonValue::String(String::from_utf8_lossy(t).to_string()),
            ValueRef::Blob(b) => JsonValue::String(format!("[blob:{} bytes]", b.len())),
        };
        map.insert(name.clone(), value);
    }
    Ok(JsonValue::Object(map))
}

fn get_change_seq(conn: &rusqlite::Connection, book_id: i64) -> Result<i64, String> {
    let seq: i64 = conn
        .query_row(
            "SELECT change_seq FROM book_state WHERE book_id = ?1",
            [book_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;
    Ok(seq)
}

fn template_to_sql_and_filters(template_json: &JsonValue) -> Result<(String, Vec<String>, Vec<TemplateFilter>), String> {
    let obj = template_json
        .as_object()
        .ok_or_else(|| "template must be JSON object".to_string())?;
    let sql = obj
        .get("sql")
        .and_then(|v| v.as_str())
        .ok_or_else(|| "template.sql is required".to_string())?
        .to_string();

    let param_order = obj
        .get("param_order")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    let filters = obj
        .get("filters")
        .and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|item| {
                    let obj = item.as_object()?;
                    let param = obj.get("param")?.as_str()?.to_string();
                    let clause = obj.get("clause")?.as_str()?.to_string();
                    Some(TemplateFilter { param, clause })
                })
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    Ok((sql, param_order, filters))
}

#[command]
pub fn list_builtin_reports() -> Result<Vec<BuiltinReport>, String> {
    let mut items = Vec::new();

    let balances_schema = "{\"required\":[\"book_id\"],\"allowed\":[\"book_id\",\"date_to\",\"account_ids\",\"include_closed\",\"limit\"],\"types\":{\"book_id\":\"integer\",\"date_to\":\"string\",\"account_ids\":\"array:integer\",\"include_closed\":\"boolean\",\"limit\":\"integer\"},\"date_fields\":[\"date_to\"]}";
    let balances_template = r#"{"sql":"SELECT a.id AS account_id, a.name AS account_name, a.commodity_id, COALESCE(SUM(s.amount_minor),0) AS balance_minor FROM accounts a LEFT JOIN splits s ON s.account_id = a.id LEFT JOIN transactions t ON t.id = s.tx_id AND (?2 IS NULL OR t.txn_date <= ?2) WHERE a.book_id = ?1 AND (?3 = 1 OR a.is_closed = 0) GROUP BY a.id, a.name, a.commodity_id ORDER BY a.name ASC","param_order":["book_id","date_to","include_closed"],"filters":[{"param":"account_ids","clause":"AND a.id IN (SELECT value FROM json_each(?4))"}]}"#;
    items.push(BuiltinReport {
        name: "account_balances".to_string(),
        description: "Account balances as of a date".to_string(),
        query_type: "template".to_string(),
        query_text: balances_template.to_string(),
        params_schema: Some(balances_schema.to_string()),
    });

    let cashflow_schema = "{\"required\":[\"book_id\"],\"allowed\":[\"book_id\",\"date_from\",\"date_to\",\"group_by\",\"limit\"],\"types\":{\"book_id\":\"integer\",\"date_from\":\"string\",\"date_to\":\"string\",\"group_by\":\"string\",\"limit\":\"integer\"},\"enums\":{\"group_by\":[\"month\",\"quarter\",\"year\",\"day\"]},\"date_fields\":[\"date_from\",\"date_to\"]}";
    let cashflow_template = r#"{"sql":"SELECT substr(t.txn_date,1,7) || '-01' AS period_start, SUM(CASE WHEN s.amount_minor > 0 THEN s.amount_minor ELSE 0 END) AS inflow_minor, SUM(CASE WHEN s.amount_minor < 0 THEN -s.amount_minor ELSE 0 END) AS outflow_minor, SUM(s.amount_minor) AS net_minor FROM transactions t JOIN splits s ON s.tx_id = t.id LEFT JOIN categories c ON c.id = s.category_id WHERE t.book_id = ?1 AND (?2 IS NULL OR t.txn_date >= ?2) AND (?3 IS NULL OR t.txn_date <= ?3) AND c.kind IN ('income','expense') GROUP BY period_start ORDER BY period_start ASC","param_order":["book_id","date_from","date_to"]}"#;
    items.push(BuiltinReport {
        name: "cashflow".to_string(),
        description: "Cashflow totals grouped by period".to_string(),
        query_type: "template".to_string(),
        query_text: cashflow_template.to_string(),
        params_schema: Some(cashflow_schema.to_string()),
    });

    let category_schema = "{\"required\":[\"book_id\"],\"allowed\":[\"book_id\",\"date_from\",\"date_to\",\"category_ids\",\"limit\"],\"types\":{\"book_id\":\"integer\",\"date_from\":\"string\",\"date_to\":\"string\",\"category_ids\":\"array:integer\",\"limit\":\"integer\"},\"date_fields\":[\"date_from\",\"date_to\"]}";
    let category_template = r#"{"sql":"SELECT c.id AS category_id, c.name AS category_name, COALESCE(SUM(s.amount_minor),0) AS total_minor FROM categories c LEFT JOIN splits s ON s.category_id = c.id LEFT JOIN transactions t ON t.id = s.tx_id WHERE c.book_id = ?1 AND c.kind = 'expense' AND (?2 IS NULL OR t.txn_date >= ?2) AND (?3 IS NULL OR t.txn_date <= ?3) GROUP BY c.id, c.name ORDER BY total_minor DESC, c.name ASC","param_order":["book_id","date_from","date_to"],"filters":[{"param":"category_ids","clause":"AND c.id IN (SELECT value FROM json_each(?4))"}]}"#;
    items.push(BuiltinReport {
        name: "category_spend".to_string(),
        description: "Spend totals by category".to_string(),
        query_type: "template".to_string(),
        query_text: category_template.to_string(),
        params_schema: Some(category_schema.to_string()),
    });

    let payee_schema = "{\"required\":[\"book_id\"],\"allowed\":[\"book_id\",\"date_from\",\"date_to\",\"payee_ids\",\"limit\"],\"types\":{\"book_id\":\"integer\",\"date_from\":\"string\",\"date_to\":\"string\",\"payee_ids\":\"array:integer\",\"limit\":\"integer\"},\"date_fields\":[\"date_from\",\"date_to\"]}";
    let payee_template = r#"{"sql":"SELECT p.id AS payee_id, p.name AS payee_name, COALESCE(SUM(s.amount_minor),0) AS total_minor FROM payees p JOIN transactions t ON t.payee_id = p.id JOIN splits s ON s.tx_id = t.id WHERE p.book_id = ?1 AND (?2 IS NULL OR t.txn_date >= ?2) AND (?3 IS NULL OR t.txn_date <= ?3) GROUP BY p.id, p.name ORDER BY total_minor DESC, p.name ASC","param_order":["book_id","date_from","date_to"],"filters":[{"param":"payee_ids","clause":"AND p.id IN (SELECT value FROM json_each(?4))"}]}"#;
    items.push(BuiltinReport {
        name: "payee_totals".to_string(),
        description: "Totals by payee".to_string(),
        query_type: "template".to_string(),
        query_text: payee_template.to_string(),
        params_schema: Some(payee_schema.to_string()),
    });

    Ok(items)
}

#[command]
pub fn create_report_definition(
    db: State<DbState>,
    input: ReportDefinitionCreate,
) -> Result<ReportDefinition, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    conn.execute(
        "INSERT INTO report_definitions (book_id, name, kind, query_type, query_text, params_schema, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            input.book_id,
            input.name,
            input.kind,
            input.query_type,
            input.query_text,
            input.params_schema,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let item = conn
        .query_row(
            "SELECT id, book_id, name, kind, query_type, query_text, params_schema, created_at, updated_at
             FROM report_definitions WHERE id = ?1",
            [id],
            map_report_definition_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[command]
pub fn update_report_definition(
    db: State<DbState>,
    input: ReportDefinitionUpdate,
) -> Result<ReportDefinition, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute(
            "UPDATE report_definitions
             SET name = ?2, kind = ?3, query_type = ?4, query_text = ?5, params_schema = ?6,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![
                input.id,
                input.name,
                input.kind,
                input.query_type,
                input.query_text,
                input.params_schema,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err("report definition not found".to_string());
    }

    let item = conn
        .query_row(
            "SELECT id, book_id, name, kind, query_type, query_text, params_schema, created_at, updated_at
             FROM report_definitions WHERE id = ?1",
            [input.id],
            map_report_definition_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[command]
pub fn delete_report_definition(db: State<DbState>, id: i64) -> Result<bool, String> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let rows = conn
        .execute("DELETE FROM report_definitions WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;
    Ok(rows > 0)
}

#[command]
pub fn get_report_definition(db: State<DbState>, id: i64) -> Result<Option<ReportDefinition>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let item = conn
        .query_row(
            "SELECT id, book_id, name, kind, query_type, query_text, params_schema, created_at, updated_at
             FROM report_definitions WHERE id = ?1",
            [id],
            map_report_definition_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;
    Ok(item)
}

#[command]
pub fn list_report_definitions(db: State<DbState>, book_id: i64) -> Result<Vec<ReportDefinition>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, name, kind, query_type, query_text, params_schema, created_at, updated_at
             FROM report_definitions WHERE book_id = ?1 ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_report_definition_row)
        .map_err(|e| e.to_string())?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row.map_err(|e| e.to_string())?);
    }

    Ok(items)
}

#[command]
pub fn run_report(
    db: State<DbState>,
    definition_id: i64,
    params_json: JsonValue,
) -> Result<ReportRunResult, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let definition: ReportDefinition = conn
        .query_row(
            "SELECT id, book_id, name, kind, query_type, query_text, params_schema, created_at, updated_at
             FROM report_definitions WHERE id = ?1",
            [definition_id],
            map_report_definition_row,
        )
        .map_err(|e| e.to_string())?;

    let schema = parse_param_schema(definition.params_schema.as_deref())?;
    let param_order = schema.as_ref().map(|s| s.param_order.clone()).unwrap_or_default();
    if let Some(schema) = &schema {
        validate_params(&params_json, schema)?;
    }

    let params_hash = hash_params(&params_json)?;
    let as_of_seq = get_change_seq(conn, definition.book_id)?;

    let cached_json: Option<String> = conn
        .query_row(
            "SELECT result_json FROM report_runs
             WHERE book_id = ?1 AND definition_id = ?2 AND params_hash = ?3 AND as_of_seq = ?4",
            params![definition.book_id, definition_id, params_hash, as_of_seq],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    if let Some(cached) = cached_json {
        let parsed: JsonValue = serde_json::from_str(&cached).map_err(|e| e.to_string())?;
        return Ok(ReportRunResult {
            definition_id,
            params_hash,
            as_of_seq,
            result_json: parsed,
            cached: true,
        });
    }

    let (sql, template_param_order, template_filters) = if definition.query_type == "template" {
        let template: JsonValue = serde_json::from_str(&definition.query_text)
            .map_err(|e| format!("invalid template JSON: {e}"))?;
        let (sql, template_order, filters) = template_to_sql_and_filters(&template)?;
        (sql, template_order, filters)
    } else {
        (definition.query_text.clone(), Vec::new(), Vec::new())
    };

    if !is_select_query(&sql) {
        return Err("only SELECT queries are allowed in reports".to_string());
    }

    let mut order = if !template_param_order.is_empty() {
        template_param_order
    } else {
        param_order
    };

    let mut sql_with_filters = sql;
    if !template_filters.is_empty() {
        for filter in template_filters {
            let include = params_json
                .get(&filter.param)
                .map(|v| !v.is_null())
                .unwrap_or(false);
            if include {
                sql_with_filters.push(' ');
                sql_with_filters.push_str(&filter.clause);
                order.push(filter.param);
            }
        }
    }

    let limit = resolve_row_limit(&params_json);
    let sql_with_limit = apply_row_limit(&sql_with_filters, limit);
    let param_values = build_param_values(&params_json, &order)?;

    let mut stmt = conn.prepare(&sql_with_limit).map_err(|e| e.to_string())?;
    let col_names = stmt
        .column_names()
        .iter()
        .map(|s| s.to_string())
        .collect::<Vec<_>>();

    let rows = stmt
        .query_map(params_from_iter(param_values), |row| Ok(row_to_json(row, &col_names)))
        .map_err(|e| e.to_string())?;

    let mut result_rows = Vec::new();
    for row in rows {
        result_rows.push(row.map_err(|e| e.to_string())??);
    }

    let result_json = JsonValue::Array(result_rows);
    let serialized = serde_json::to_string(&result_json).map_err(|e| e.to_string())?;

    conn.execute(
        "INSERT INTO report_runs (book_id, definition_id, params_hash, as_of_seq, result_json, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![definition.book_id, definition_id, params_hash, as_of_seq, serialized],
    )
    .map_err(|e| e.to_string())?;

    Ok(ReportRunResult {
        definition_id,
        params_hash,
        as_of_seq,
        result_json,
        cached: false,
    })
}

#[command]
pub fn report_account_balances(
    db: State<DbState>,
    input: ReportAccountBalancesInput,
) -> Result<Vec<AccountBalanceRow>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let include_closed = input.include_closed.unwrap_or(false);
    let mut sql = String::from(
        "SELECT a.id, a.name, a.commodity_id, COALESCE(SUM(s.amount_minor), 0) AS balance_minor
         FROM accounts a
         LEFT JOIN splits s ON s.account_id = a.id
         LEFT JOIN transactions t ON t.id = s.tx_id AND (?2 IS NULL OR t.txn_date <= ?2)
         WHERE a.book_id = ?1 AND (?3 = 1 OR a.is_closed = 0)",
    );
    let mut params: Vec<Value> = vec![
        Value::from(input.book_id),
        match input.date_to.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
        Value::from(if include_closed { 1 } else { 0 }),
    ];

    if let Some(ids) = input.account_ids.as_ref() {
        if !ids.is_empty() {
            let placeholders = vec!["?"; ids.len()].join(",");
            sql.push_str(&format!(" AND a.id IN ({placeholders})"));
            for id in ids {
                params.push(Value::from(*id));
            }
        }
    }

    sql.push_str(" GROUP BY a.id, a.name, a.commodity_id ORDER BY a.name ASC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), |row| {
            Ok(AccountBalanceRow {
                account_id: row.get(0)?,
                account_name: row.get(1)?,
                commodity_id: row.get(2)?,
                balance_minor: row.get(3)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut result = Vec::new();
    for row in rows {
        result.push(row.map_err(|e| e.to_string())?);
    }

    Ok(result)
}

#[command]
pub fn report_cashflow(
    db: State<DbState>,
    input: ReportCashflowInput,
) -> Result<Vec<CashflowRow>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let group_by = input.group_by.unwrap_or_else(|| "month".to_string());
    let period_expr = match group_by.as_str() {
        "year" => "substr(t.txn_date,1,4) || '-01-01'",
        "quarter" => "printf('%s-%02d-01', substr(t.txn_date,1,4), ((cast(substr(t.txn_date,6,2) as int)-1)/3)*3+1)",
        "day" => "t.txn_date",
        _ => "substr(t.txn_date,1,7) || '-01'",
    };

    let sql = format!(
        "SELECT {period_expr} AS period_start,
                SUM(CASE WHEN s.amount_minor > 0 THEN s.amount_minor ELSE 0 END) AS inflow_minor,
                SUM(CASE WHEN s.amount_minor < 0 THEN -s.amount_minor ELSE 0 END) AS outflow_minor,
                SUM(s.amount_minor) AS net_minor
         FROM transactions t
         JOIN splits s ON s.tx_id = t.id
         LEFT JOIN categories c ON c.id = s.category_id
         WHERE t.book_id = ?1
           AND (?2 IS NULL OR t.txn_date >= ?2)
           AND (?3 IS NULL OR t.txn_date <= ?3)
           AND c.kind IN ('income','expense')
         GROUP BY period_start
         ORDER BY period_start ASC"
    );

    let params: Vec<Value> = vec![
        Value::from(input.book_id),
        match input.date_from.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
        match input.date_to.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
    ];

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), |row| {
            Ok(CashflowRow {
                period_start: row.get(0)?,
                inflow_minor: row.get(1)?,
                outflow_minor: row.get(2)?,
                net_minor: row.get(3)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut result = Vec::new();
    for row in rows {
        result.push(row.map_err(|e| e.to_string())?);
    }

    Ok(result)
}

#[command]
pub fn report_category_spend(
    db: State<DbState>,
    input: ReportCategorySpendInput,
) -> Result<Vec<CategorySpendRow>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut sql = String::from(
        "SELECT c.id, c.name, COALESCE(SUM(s.amount_minor), 0) AS total_minor
         FROM categories c
         LEFT JOIN splits s ON s.category_id = c.id
         LEFT JOIN transactions t ON t.id = s.tx_id
         WHERE c.book_id = ?1 AND c.kind = 'expense'
           AND (?2 IS NULL OR t.txn_date >= ?2)
           AND (?3 IS NULL OR t.txn_date <= ?3)",
    );

    let mut params: Vec<Value> = vec![
        Value::from(input.book_id),
        match input.date_from.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
        match input.date_to.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
    ];

    if let Some(ids) = input.category_ids.as_ref() {
        if !ids.is_empty() {
            let placeholders = vec!["?"; ids.len()].join(",");
            sql.push_str(&format!(" AND c.id IN ({placeholders})"));
            for id in ids {
                params.push(Value::from(*id));
            }
        }
    }

    sql.push_str(" GROUP BY c.id, c.name ORDER BY total_minor DESC, c.name ASC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), |row| {
            Ok(CategorySpendRow {
                category_id: row.get(0)?,
                category_name: row.get(1)?,
                total_minor: row.get(2)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut result = Vec::new();
    for row in rows {
        result.push(row.map_err(|e| e.to_string())?);
    }

    Ok(result)
}

#[command]
pub fn report_payee_totals(
    db: State<DbState>,
    input: ReportPayeeTotalsInput,
) -> Result<Vec<PayeeTotalRow>, String> {
    let guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut sql = String::from(
        "SELECT p.id, p.name, COALESCE(SUM(s.amount_minor), 0) AS total_minor
         FROM payees p
         JOIN transactions t ON t.payee_id = p.id
         JOIN splits s ON s.tx_id = t.id
         WHERE p.book_id = ?1
           AND (?2 IS NULL OR t.txn_date >= ?2)
           AND (?3 IS NULL OR t.txn_date <= ?3)",
    );

    let mut params: Vec<Value> = vec![
        Value::from(input.book_id),
        match input.date_from.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
        match input.date_to.clone() {
            Some(v) => Value::from(v),
            None => Value::Null,
        },
    ];

    if let Some(ids) = input.payee_ids.as_ref() {
        if !ids.is_empty() {
            let placeholders = vec!["?"; ids.len()].join(",");
            sql.push_str(&format!(" AND p.id IN ({placeholders})"));
            for id in ids {
                params.push(Value::from(*id));
            }
        }
    }

    sql.push_str(" GROUP BY p.id, p.name ORDER BY total_minor DESC, p.name ASC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), |row| {
            Ok(PayeeTotalRow {
                payee_id: row.get(0)?,
                payee_name: row.get(1)?,
                total_minor: row.get(2)?,
            })
        })
        .map_err(|e| e.to_string())?;

    let mut result = Vec::new();
    for row in rows {
        result.push(row.map_err(|e| e.to_string())?);
    }

    Ok(result)
}

#[command]
pub fn prune_report_runs(
    db: State<DbState>,
    book_id: i64,
    retain_per_definition: i64,
) -> Result<i64, String> {
    if retain_per_definition < 1 {
        return Err("retain_per_definition must be >= 1".to_string());
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute(
            "DELETE FROM report_runs
             WHERE id IN (
               SELECT id FROM (
                 SELECT id,
                        ROW_NUMBER() OVER (PARTITION BY definition_id ORDER BY created_at DESC) AS rn
                 FROM report_runs
                 WHERE book_id = ?1
               ) WHERE rn > ?2
             )",
            params![book_id, retain_per_definition],
        )
        .map_err(|e| e.to_string())?;

    Ok(rows as i64)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::open_and_migrate;
    use crate::state::DbStateInner;
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
        temp.push(format!("rekenraam_reports_test_{start}_{counter}"));
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

    fn lookup_id(conn: &rusqlite::Connection, table: &str, name: &str) -> i64 {
        let sql = format!("SELECT id FROM {table} WHERE name = ?1 LIMIT 1");
        conn.query_row(&sql, [name], |row| row.get(0)).expect("lookup")
    }

    fn lookup_usd(conn: &rusqlite::Connection) -> i64 {
        conn.query_row("SELECT id FROM commodities WHERE symbol='USD' LIMIT 1", [], |row| row.get(0))
            .expect("usd")
    }

    fn insert_transaction(
        conn: &rusqlite::Connection,
        txn_date: &str,
        payee_id: i64,
        checking_id: i64,
        cash_id: i64,
        grocery_category_id: i64,
        usd_id: i64,
        amount_minor: i64,
    ) {
        conn.execute(
            "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
             VALUES (1, ?1, ?2, 'memo', 'cleared', NULL, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![txn_date, payee_id],
        )
        .expect("insert tx");
        let tx_id = conn.last_insert_rowid();

        conn.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, 'groceries', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![tx_id, checking_id, usd_id, -amount_minor, grocery_category_id],
        )
        .expect("insert split1");

        conn.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, NULL, 'counter', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![tx_id, cash_id, usd_id, amount_minor],
        )
        .expect("insert split2");
    }

    #[test]
    fn test_report_definition_crud_and_cache() {
        let db_state = create_temp_db();

        let definition = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Test Report".to_string(),
                kind: "custom".to_string(),
                query_type: "sql".to_string(),
                query_text: "SELECT 1 AS value".to_string(),
                params_schema: Some("{\"required\":[],\"allowed\":[]}".to_string()),
            },
        )
        .expect("create report definition");

        let list = list_report_definitions(as_state(&db_state), 1).expect("list definitions");
        assert!(list.iter().any(|d| d.id == definition.id));

        let fetched = get_report_definition(as_state(&db_state), definition.id)
            .expect("get report definition")
            .expect("exists");
        assert_eq!(fetched.name, "Test Report");

        let updated = update_report_definition(
            as_state(&db_state),
            ReportDefinitionUpdate {
                id: definition.id,
                book_id: 1,
                name: "Test Report Updated".to_string(),
                kind: "custom".to_string(),
                query_type: "sql".to_string(),
                query_text: "SELECT 2 AS value".to_string(),
                params_schema: Some("{\"required\":[],\"allowed\":[]}".to_string()),
            },
        )
        .expect("update report definition");
        assert_eq!(updated.name, "Test Report Updated");

        let run1 = run_report(
            as_state(&db_state),
            updated.id,
            JsonValue::Object(serde_json::Map::new()),
        )
        .expect("run report");
        assert!(!run1.cached);

        let run2 = run_report(
            as_state(&db_state),
            updated.id,
            JsonValue::Object(serde_json::Map::new()),
        )
        .expect("run report cached");
        assert!(run2.cached);

        {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            conn.execute(
                "INSERT INTO categories (book_id, parent_id, name, kind, created_at, updated_at)
                 VALUES (1, NULL, 'Cache Bump', 'expense', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert category");
        }

        let run3 = run_report(
            as_state(&db_state),
            updated.id,
            JsonValue::Object(serde_json::Map::new()),
        )
        .expect("run report after change");
        assert!(!run3.cached);

        let deleted = delete_report_definition(as_state(&db_state), updated.id)
            .expect("delete report definition");
        assert!(deleted);
    }

    #[test]
    fn test_run_report_rejects_non_select() {
        let db_state = create_temp_db();

        let definition = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Bad Report".to_string(),
                kind: "custom".to_string(),
                query_type: "sql".to_string(),
                query_text: "DELETE FROM transactions".to_string(),
                params_schema: Some("{\"required\":[],\"allowed\":[]}".to_string()),
            },
        )
        .expect("create report definition");

        let err = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(serde_json::Map::new()),
        )
        .expect_err("non-select rejected");
        assert!(err.to_lowercase().contains("select"));
    }

    #[test]
    fn test_template_report_params() {
        let db_state = create_temp_db();

        let template_json = r#"{"sql":"SELECT ?1 AS value, ?2 AS label","param_order":["amount","label"]}"#;
        let definition = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Template Report".to_string(),
                kind: "custom".to_string(),
                query_type: "template".to_string(),
                query_text: template_json.to_string(),
                params_schema: Some("{\"required\":[\"amount\",\"label\"],\"allowed\":[\"amount\",\"label\"]}".to_string()),
            },
        )
        .expect("create template definition");

        let mut params = serde_json::Map::new();
        params.insert("amount".to_string(), JsonValue::Number(123.into()));
        params.insert("label".to_string(), JsonValue::String("ok".to_string()));

        let run = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(params),
        )
        .expect("run template report");

        let rows = run.result_json.as_array().expect("rows array");
        assert_eq!(rows.len(), 1);
        let row = rows[0].as_object().expect("row object");
        assert_eq!(row.get("value"), Some(&JsonValue::Number(123.into())));
        assert_eq!(row.get("label"), Some(&JsonValue::String("ok".to_string())));
    }

    #[test]
    fn test_builtin_reports() {
        let db_state = create_temp_db();

        let (checking_id, cash_id, grocery_id, payee_id) = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            let checking_id = lookup_id(conn, "accounts", "Checking Account");
            let cash_id = lookup_id(conn, "accounts", "Cash");
            let grocery_id = lookup_id(conn, "categories", "Groceries");

            conn.execute(
                "INSERT INTO payees (book_id, name, kind, created_at, updated_at)
                 VALUES (1, 'Test Store', 'payee', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert payee");
            let payee_id = conn.last_insert_rowid();

            insert_transaction(
                conn,
                "2024-01-05",
                payee_id,
                checking_id,
                cash_id,
                grocery_id,
                lookup_usd(conn),
                1000,
            );
            (checking_id, cash_id, grocery_id, payee_id)
        };

        let balances = report_account_balances(
            as_state(&db_state),
            ReportAccountBalancesInput {
                book_id: 1,
                date_to: Some("2024-01-31".to_string()),
                account_ids: Some(vec![checking_id, cash_id]),
                include_closed: Some(true),
            },
        )
        .expect("account balances");
        assert_eq!(balances.len(), 2);

        let cashflow = report_cashflow(
            as_state(&db_state),
            ReportCashflowInput {
                book_id: 1,
                date_from: Some("2024-01-01".to_string()),
                date_to: Some("2024-01-31".to_string()),
                group_by: Some("month".to_string()),
            },
        )
        .expect("cashflow");
        assert_eq!(cashflow.len(), 1);
        assert_eq!(cashflow[0].outflow_minor, 1000);

        let category_spend = report_category_spend(
            as_state(&db_state),
            ReportCategorySpendInput {
                book_id: 1,
                date_from: Some("2024-01-01".to_string()),
                date_to: Some("2024-01-31".to_string()),
                category_ids: Some(vec![grocery_id]),
            },
        )
        .expect("category spend");
        assert_eq!(category_spend.len(), 1);

        let payee_totals = report_payee_totals(
            as_state(&db_state),
            ReportPayeeTotalsInput {
                book_id: 1,
                date_from: Some("2024-01-01".to_string()),
                date_to: Some("2024-01-31".to_string()),
                payee_ids: Some(vec![payee_id]),
            },
        )
        .expect("payee totals");
        assert_eq!(payee_totals.len(), 1);
        assert_eq!(payee_totals[0].payee_id, payee_id);
    }

    #[test]
    fn test_row_limit_and_param_validation() {
        let db_state = create_temp_db();

        let definition = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Limit Report".to_string(),
                kind: "custom".to_string(),
                query_type: "sql".to_string(),
                query_text: "WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 20) SELECT x AS value FROM cnt".to_string(),
                params_schema: Some("{\"required\":[\"limit\"],\"allowed\":[\"limit\"],\"types\":{\"limit\":\"integer\"}}".to_string()),
            },
        )
        .expect("create limit report");

        let mut params = serde_json::Map::new();
        params.insert("limit".to_string(), JsonValue::Number(5.into()));
        let run = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(params),
        )
        .expect("run limited report");
        assert_eq!(run.result_json.as_array().unwrap().len(), 5);

        let mut bad_map = serde_json::Map::new();
        bad_map.insert("limit".to_string(), JsonValue::String("nope".to_string()));
        let bad_params = JsonValue::Object(bad_map);
        let err = run_report(as_state(&db_state), definition.id, bad_params)
            .expect_err("type validation");
        assert!(err.contains("invalid param type"));
    }

    #[test]
    fn test_invalid_params_schema_and_enum_validation() {
        let db_state = create_temp_db();

        let definition = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Schema Report".to_string(),
                kind: "custom".to_string(),
                query_type: "sql".to_string(),
                query_text: "SELECT 1 AS value".to_string(),
                params_schema: Some("not-json".to_string()),
            },
        )
        .expect("create schema report");

        let err = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(serde_json::Map::new()),
        )
        .expect_err("invalid schema");
        assert!(err.to_lowercase().contains("json"));

        let definition2 = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Enum Report".to_string(),
                kind: "custom".to_string(),
                query_type: "sql".to_string(),
                query_text: "SELECT 1 AS value".to_string(),
                params_schema: Some("{\"required\":[\"group_by\"],\"allowed\":[\"group_by\"],\"types\":{\"group_by\":\"string\"},\"enums\":{\"group_by\":[\"month\",\"year\"]}}".to_string()),
            },
        )
        .expect("create enum report");

        let mut bad = serde_json::Map::new();
        bad.insert("group_by".to_string(), JsonValue::String("week".to_string()));
        let err = run_report(
            as_state(&db_state),
            definition2.id,
            JsonValue::Object(bad),
        )
        .expect_err("enum validation");
        assert!(err.contains("invalid param value"));
    }

    #[test]
    fn test_template_filters_and_prune_runs() {
        let db_state = create_temp_db();

        let template = r#"{"sql":"SELECT t.txn_date AS txn_date FROM transactions t WHERE t.book_id = ?1","param_order":["book_id"],"filters":[{"param":"date_from","clause":"AND t.txn_date >= ?"}]}"#;
        let definition = create_report_definition(
            as_state(&db_state),
            ReportDefinitionCreate {
                book_id: 1,
                name: "Filter Report".to_string(),
                kind: "custom".to_string(),
                query_type: "template".to_string(),
                query_text: template.to_string(),
                params_schema: Some("{\"required\":[\"book_id\"],\"allowed\":[\"book_id\",\"date_from\"],\"types\":{\"book_id\":\"integer\",\"date_from\":\"string\"},\"date_fields\":[\"date_from\"]}".to_string()),
            },
        )
        .expect("create filter report");

        {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            conn.execute(
                "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
                 VALUES (1, '2024-01-01', NULL, NULL, 'cleared', NULL, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert tx1");
            conn.execute(
                "INSERT INTO transactions (book_id, txn_date, payee_id, memo, status, reference, import_id, created_at, updated_at)
                 VALUES (1, '2024-02-01', NULL, NULL, 'cleared', NULL, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert tx2");
        }

        let mut params = serde_json::Map::new();
        params.insert("book_id".to_string(), JsonValue::Number(1.into()));
        params.insert("date_from".to_string(), JsonValue::String("2024-02-01".to_string()));
        let run = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(params),
        )
        .expect("run filter report");
        assert_eq!(run.result_json.as_array().unwrap().len(), 1);

        let mut params1 = serde_json::Map::new();
        params1.insert("book_id".to_string(), JsonValue::Number(1.into()));
        let _ = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(params1),
        )
        .expect("run report 1");

        {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            conn.execute(
                "INSERT INTO categories (book_id, parent_id, name, kind, created_at, updated_at)
                 VALUES (1, NULL, 'Prune Bump', 'expense', strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert category");
        }

        let mut params2 = serde_json::Map::new();
        params2.insert("book_id".to_string(), JsonValue::Number(1.into()));
        let _ = run_report(
            as_state(&db_state),
            definition.id,
            JsonValue::Object(params2),
        )
        .expect("run report 2");

        let pruned = prune_report_runs(as_state(&db_state), 1, 1)
            .expect("prune report runs");
        assert!(pruned >= 1);
    }
}

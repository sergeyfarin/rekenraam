use std::collections::HashMap;

use chrono::NaiveDate;
use rusqlite::{params, params_from_iter, types::Value, OptionalExtension};
use serde::{Deserialize, Serialize};
use tauri::{command, State};

use crate::error::AppError;
use crate::session::{current_session_id, clear_redo_stack, record_insert_change};
use crate::state::DbState;
use crate::validation::{validate_account_type, validate_name};

const SINGLE_BOOK_ID: i64 = 1;
const BALANCING_TX_MARK: &str = "balance_adjustment";
const VOID_SESSION_PREFIX: &str = "void:";

fn resolve_current_account_id(conn: &rusqlite::Connection, account_id: i64) -> Result<Option<i64>, String> {
    conn.query_row(
        "WITH RECURSIVE chain(id) AS (
             SELECT ?1
             UNION ALL
             SELECT a.id
             FROM accounts a
             JOIN chain c ON a.previous_account_id = c.id
         )
         SELECT c.id
         FROM chain c
         WHERE NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = c.id)
         LIMIT 1",
        [account_id],
        |row| row.get(0),
    )
    .optional()
    .map_err(|e| e.to_string())
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Account {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub account_type: String,
    pub name: String,
    pub commodity_id: i64,
    pub institution_id: Option<i64>,
    pub institution_name: Option<String>,
    pub country_id: Option<i64>,
    pub country_name: Option<String>,
    pub number_last4: Option<String>,
    pub is_closed: bool,
    pub is_hidden: bool,
    pub is_system: bool,
    pub system_role: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountCreate {
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub account_type: String,
    pub name: String,
    pub commodity_id: i64,
    pub institution_id: Option<i64>,
    pub country_id: Option<i64>,
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
    pub institution_id: Option<i64>,
    pub country_id: Option<i64>,
    pub number_last4: Option<String>,
    pub is_closed: bool,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ProfitLossSystemAccounts {
    pub income_summary_account_id: i64,
    pub expense_summary_account_id: i64,
    pub retained_earnings_account_id: i64,
    pub commodity_id: i64,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FiscalYearCloseInput {
    pub close_date: String,
    pub memo: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct FiscalYearCloseResult {
    pub tx_id: Option<i64>,
    pub closed_accounts_count: i64,
    pub retained_earnings_delta_minor: i64,
    pub close_date: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Country {
    pub id: i64,
    pub book_id: i64,
    pub code: String,
    pub name: String,
    pub default_currency_id: Option<i64>,
    pub currency_code: Option<String>,
    pub currency_name: Option<String>,
    pub currency_symbol: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CountryCreate {
    pub book_id: i64,
    pub code: String,
    pub name: String,
    pub default_currency_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CountryUpdate {
    pub id: i64,
    pub book_id: i64,
    pub code: String,
    pub name: String,
    pub default_currency_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Institution {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub country_id: Option<i64>,
    pub country_name: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct InstitutionCreate {
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub country_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct InstitutionUpdate {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub country_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Category {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub previous_category_id: Option<i64>,
    pub name: String,
    pub kind: String,
    pub color: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CategoryCreate {
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub name: String,
    pub kind: String,
    pub color: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct CategoryUpdate {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub name: String,
    pub kind: String,
    pub color: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Payee {
    pub id: i64,
    pub book_id: i64,
    pub previous_payee_id: Option<i64>,
    pub name: String,
    pub kind: String,
    pub metadata: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PayeeCreate {
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub metadata: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PayeeUpdate {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub kind: String,
    pub metadata: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Tag {
    pub id: i64,
    pub book_id: i64,
    pub previous_tag_id: Option<i64>,
    pub name: String,
    pub color: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct TagCreate {
    pub book_id: i64,
    pub name: String,
    pub color: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct TagUpdate {
    pub id: i64,
    pub book_id: i64,
    pub name: String,
    pub color: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Person {
    pub id: i64,
    pub book_id: i64,
    pub previous_person_id: Option<i64>,
    pub name: String,
    pub role: String,
    pub metadata: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct PersonCreate {
    pub book_id: i64,
    pub name: String,
    pub role: String,
    pub metadata: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Project {
    pub id: i64,
    pub book_id: i64,
    pub previous_project_id: Option<i64>,
    pub name: String,
    pub status: String,
    pub metadata: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct ProjectCreate {
    pub book_id: i64,
    pub name: String,
    pub status: String,
    pub metadata: Option<String>,
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

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountBookingPolicyUpdate {
    pub account_id: i64,
    pub booking_policy: String,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct BalanceConstraint {
    pub id: i64,
    pub book_id: i64,
    pub account_id: i64,
    pub min_balance_minor: Option<i64>,
    pub max_balance_minor: Option<i64>,
    pub sign_rule: String,
    pub created_at: String,
    pub updated_at: String,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct BalanceConstraintCreate {
    pub book_id: i64,
    pub account_id: i64,
    pub min_balance_minor: Option<i64>,
    pub max_balance_minor: Option<i64>,
    pub sign_rule: Option<String>,
}

#[allow(dead_code)]
#[derive(Serialize, Deserialize, Debug)]
pub struct AccountTreeNode {
    pub id: i64,
    pub parent_id: Option<i64>,
    pub name: String,
    pub account_type: String,
    pub commodity_id: i64,
    pub commodity_name: String,
    pub commodity_scale: i64,
    pub institution_name: Option<String>,
    pub country_name: Option<String>,
    pub balance_minor: i64,
    pub rollup_balance_minor: i64,
    pub children: Vec<AccountTreeNode>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountDirective {
    pub id: i64,
    pub book_id: i64,
    pub account_id: i64,
    pub directive_type: String,
    pub directive_date: String,
    pub note: Option<String>,
    pub metadata: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct AccountDirectiveCreate {
    pub book_id: i64,
    pub account_id: i64,
    pub directive_date: String,
    pub note: Option<String>,
    pub metadata: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct BalanceCheck {
    pub id: i64,
    pub book_id: i64,
    pub account_id: i64,
    pub as_of_date: String,
    pub balance_minor: i64,
    pub memo: Option<String>,
    pub status: String,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct BalanceCheckCreate {
    pub book_id: i64,
    pub account_id: i64,
    pub as_of_date: String,
    pub balance_minor: i64,
    pub memo: Option<String>,
    pub create_balancing_tx: Option<bool>,
    pub offset_account_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct BalanceAdjustment {
    pub id: i64,
    pub book_id: i64,
    pub balance_check_id: Option<i64>,
    pub account_id: i64,
    pub offset_account_id: i64,
    pub as_of_date: String,
    pub target_balance_minor: i64,
    pub actual_balance_minor: i64,
    pub adjustment_minor: i64,
    pub tx_id: i64,
    pub mark: String,
    pub memo: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct BalanceAdjustmentCreate {
    pub book_id: i64,
    pub balance_check_id: Option<i64>,
    pub account_id: i64,
    pub offset_account_id: i64,
    pub as_of_date: String,
    pub target_balance_minor: i64,
    pub memo: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Note {
    pub id: i64,
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub note: String,
    pub note_date: Option<String>,
    pub created_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct NoteCreate {
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub note: String,
    pub note_date: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct NoteUpdate {
    pub id: i64,
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub note: String,
    pub note_date: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct NoteFilter {
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Event {
    pub id: i64,
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub event_type: String,
    pub event_date: String,
    pub description: Option<String>,
    pub metadata: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct EventCreate {
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub event_type: String,
    pub event_date: String,
    pub description: Option<String>,
    pub metadata: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct EventUpdate {
    pub id: i64,
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub event_type: String,
    pub event_date: String,
    pub description: Option<String>,
    pub metadata: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct EventFilter {
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct Document {
    pub id: i64,
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub doc_type: Option<String>,
    pub title: Option<String>,
    pub uri: Option<String>,
    pub mime_type: Option<String>,
    pub notes: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct DocumentCreate {
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub doc_type: Option<String>,
    pub title: Option<String>,
    pub uri: Option<String>,
    pub mime_type: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct DocumentUpdate {
    pub id: i64,
    pub book_id: i64,
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
    pub doc_type: Option<String>,
    pub title: Option<String>,
    pub uri: Option<String>,
    pub mime_type: Option<String>,
    pub notes: Option<String>,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct DocumentFilter {
    pub account_id: Option<i64>,
    pub tx_id: Option<i64>,
}

fn map_account_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Account> {
    let is_closed: i64 = row.get(11)?;
    let is_hidden: i64 = row.get(12)?;
    let is_system: i64 = row.get(13)?;
    Ok(Account {
        id: row.get(0)?,
        book_id: row.get(1)?,
        parent_id: row.get(2)?,
        account_type: row.get(3)?,
        name: row.get(4)?,
        commodity_id: row.get(5)?,
        institution_id: row.get(6)?,
        institution_name: row.get(7)?,
        country_id: row.get(8)?,
        country_name: row.get(9)?,
        number_last4: row.get(10)?,
        is_closed: is_closed != 0,
        is_hidden: is_hidden != 0,
        is_system: is_system != 0,
        system_role: row.get(14)?,
        created_at: row.get(15)?,
    })
}

fn validate_close_date(value: &str) -> Result<(), String> {
    let bytes = value.as_bytes();
    if bytes.len() != 10 || bytes[4] != b'-' || bytes[7] != b'-' {
        return Err("close_date must be in YYYY-MM-DD format".to_string());
    }
    for (index, byte) in bytes.iter().enumerate() {
        if index == 4 || index == 7 {
            continue;
        }
        if !byte.is_ascii_digit() {
            return Err("close_date must be in YYYY-MM-DD format".to_string());
        }
    }

    NaiveDate::parse_from_str(value, "%Y-%m-%d")
        .map_err(|_| "close_date must be a valid calendar date in YYYY-MM-DD format".to_string())?;

    Ok(())
}

fn resolve_base_commodity_id(conn: &rusqlite::Connection, book_id: i64) -> Result<i64, String> {
    conn.query_row(
        "SELECT id
         FROM current_commodities c
         WHERE c.book_id = ?1
           AND c.kind = 'currency'
           AND c.is_default = 1
         LIMIT 1",
        [book_id],
        |row| row.get(0),
    )
    .map_err(|e| e.to_string())
}

fn ensure_profit_loss_system_accounts_internal(
    conn: &rusqlite::Connection,
    book_id: i64,
) -> Result<ProfitLossSystemAccounts, String> {
    let commodity_id = resolve_base_commodity_id(conn, book_id)?;

    conn.execute(
        "INSERT INTO accounts (
            book_id, type, name, commodity_id, is_closed, is_hidden, is_system, system_role,
            previous_account_id, session_id, effective_at, lifecycle_event, created_at
        )
        SELECT
            ?1, 'income', 'System Income Summary', ?2, 0, 1, 1, 'income_summary',
            NULL, NULL, date('now'), 'open', strftime('%Y-%m-%dT%H:%M:%fZ','now')
        WHERE NOT EXISTS (
            SELECT 1 FROM accounts a
            WHERE a.book_id = ?1
              AND a.system_role = 'income_summary'
              AND NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = a.id)
        )",
        params![book_id, commodity_id],
    )
    .map_err(|e| e.to_string())?;

    conn.execute(
        "INSERT INTO accounts (
            book_id, type, name, commodity_id, is_closed, is_hidden, is_system, system_role,
            previous_account_id, session_id, effective_at, lifecycle_event, created_at
        )
        SELECT
            ?1, 'expense', 'System Expense Summary', ?2, 0, 1, 1, 'expense_summary',
            NULL, NULL, date('now'), 'open', strftime('%Y-%m-%dT%H:%M:%fZ','now')
        WHERE NOT EXISTS (
            SELECT 1 FROM accounts a
            WHERE a.book_id = ?1
              AND a.system_role = 'expense_summary'
              AND NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = a.id)
        )",
        params![book_id, commodity_id],
    )
    .map_err(|e| e.to_string())?;

    conn.execute(
        "INSERT INTO accounts (
            book_id, type, name, commodity_id, is_closed, is_hidden, is_system, system_role,
            previous_account_id, session_id, effective_at, lifecycle_event, created_at
        )
        SELECT
            ?1, 'equity', 'Retained Earnings', ?2, 0, 1, 1, 'retained_earnings',
            NULL, NULL, date('now'), 'open', strftime('%Y-%m-%dT%H:%M:%fZ','now')
        WHERE NOT EXISTS (
            SELECT 1 FROM accounts a
            WHERE a.book_id = ?1
              AND a.system_role = 'retained_earnings'
              AND NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = a.id)
        )",
        params![book_id, commodity_id],
    )
    .map_err(|e| e.to_string())?;

    let income_summary_account_id: i64 = conn
        .query_row(
                        "SELECT id
                         FROM accounts
                         WHERE book_id = ?1 AND system_role = 'income_summary'
                             AND NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = accounts.id)
                         LIMIT 1",
            [book_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;
    let expense_summary_account_id: i64 = conn
        .query_row(
                        "SELECT id
                         FROM accounts
                         WHERE book_id = ?1 AND system_role = 'expense_summary'
                             AND NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = accounts.id)
                         LIMIT 1",
            [book_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;
    let retained_earnings_account_id: i64 = conn
        .query_row(
                        "SELECT id
                         FROM accounts
                         WHERE book_id = ?1 AND system_role = 'retained_earnings'
                             AND NOT EXISTS (SELECT 1 FROM accounts newer WHERE newer.previous_account_id = accounts.id)
                         LIMIT 1",
            [book_id],
            |row| row.get(0),
        )
        .map_err(|e| e.to_string())?;

    Ok(ProfitLossSystemAccounts {
        income_summary_account_id,
        expense_summary_account_id,
        retained_earnings_account_id,
        commodity_id,
    })
}

fn map_country_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Country> {
    Ok(Country {
        id: row.get(0)?,
        book_id: row.get(1)?,
        code: row.get(2)?,
        name: row.get(3)?,
        default_currency_id: row.get(4)?,
        currency_code: row.get(5)?,
        currency_name: row.get(6)?,
        currency_symbol: row.get(7)?,
        created_at: row.get(8)?,
    })
}

fn map_institution_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Institution> {
    Ok(Institution {
        id: row.get(0)?,
        book_id: row.get(1)?,
        name: row.get(2)?,
        kind: row.get(3)?,
        country_id: row.get(4)?,
        country_name: row.get(5)?,
        created_at: row.get(6)?,
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
        previous_category_id: row.get(3)?,
        name: row.get(4)?,
        kind: row.get(5)?,
        color: row.get(6)?,
        created_at: row.get(7)?,
    })
}

fn map_payee_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Payee> {
    Ok(Payee {
        id: row.get(0)?,
        book_id: row.get(1)?,
        previous_payee_id: row.get(2)?,
        name: row.get(3)?,
        kind: row.get(4)?,
        metadata: row.get(5)?,
        created_at: row.get(6)?,
    })
}

fn map_tag_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Tag> {
    Ok(Tag {
        id: row.get(0)?,
        book_id: row.get(1)?,
        previous_tag_id: row.get(2)?,
        name: row.get(3)?,
        color: row.get(4)?,
        created_at: row.get(5)?,
    })
}

fn map_person_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Person> {
    Ok(Person {
        id: row.get(0)?,
        book_id: row.get(1)?,
        previous_person_id: row.get(2)?,
        name: row.get(3)?,
        role: row.get(4)?,
        metadata: row.get(5)?,
        created_at: row.get(6)?,
    })
}

fn map_project_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Project> {
    Ok(Project {
        id: row.get(0)?,
        book_id: row.get(1)?,
        previous_project_id: row.get(2)?,
        name: row.get(3)?,
        status: row.get(4)?,
        metadata: row.get(5)?,
        created_at: row.get(6)?,
    })
}

fn map_account_directive_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<AccountDirective> {
    Ok(AccountDirective {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        directive_type: row.get(3)?,
        directive_date: row.get(4)?,
        note: row.get(5)?,
        metadata: row.get(6)?,
        created_at: row.get(7)?,
    })
}

fn map_balance_check_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<BalanceCheck> {
    Ok(BalanceCheck {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        as_of_date: row.get(3)?,
        balance_minor: row.get(4)?,
        memo: row.get(5)?,
        status: row.get(6)?,
        created_at: row.get(7)?,
    })
}

fn map_balance_adjustment_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<BalanceAdjustment> {
    Ok(BalanceAdjustment {
        id: row.get(0)?,
        book_id: row.get(1)?,
        balance_check_id: row.get(2)?,
        account_id: row.get(3)?,
        offset_account_id: row.get(4)?,
        as_of_date: row.get(5)?,
        target_balance_minor: row.get(6)?,
        actual_balance_minor: row.get(7)?,
        adjustment_minor: row.get(8)?,
        tx_id: row.get(9)?,
        mark: row.get(10)?,
        memo: row.get(11)?,
        created_at: row.get(12)?,
    })
}

fn get_account_balance_minor(conn: &rusqlite::Connection, account_id: i64) -> Result<i64, String> {
    // Must match the standard void/supersede/revert filter used throughout the codebase.
    // Without these filters the balance includes voided and superseded transactions,
    // which produces incorrect totals for balance checks, adjustments, and closing validation.
    conn.query_row(
        "SELECT COALESCE(SUM(s.amount_minor), 0)
         FROM splits s
         JOIN transactions t ON t.id = s.tx_id
         WHERE s.account_id = ?1
           AND t.status != 'void'
           AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
           AND NOT EXISTS (
               SELECT 1 FROM session_reverts sr
               WHERE sr.table_name = 'transactions'
                 AND sr.row_id = t.id
                 AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
           )",
        [account_id],
        |row| row.get(0),
    )
    .map_err(|e| e.to_string())
}

fn get_account_commodity_id(conn: &rusqlite::Connection, account_id: i64) -> Result<i64, String> {
    conn.query_row(
        "SELECT commodity_id FROM accounts WHERE id = ?1",
        [account_id],
        |row| row.get(0),
    )
    .map_err(|e| e.to_string())
}

fn create_marked_balance_adjustment(
    conn: &mut rusqlite::Connection,
    balance_check_id: Option<i64>,
    account_id: i64,
    offset_account_id: i64,
    as_of_date: &str,
    target_balance_minor: i64,
    memo: Option<&str>,
) -> Result<Option<BalanceAdjustment>, String> {
    let actual_balance_minor = get_account_balance_minor(conn, account_id)?;
    let adjustment_minor = target_balance_minor - actual_balance_minor;
    if adjustment_minor == 0 {
        return Ok(None);
    }

    let account_commodity_id = get_account_commodity_id(conn, account_id)?;
    let offset_commodity_id = get_account_commodity_id(conn, offset_account_id)?;
    if account_commodity_id != offset_commodity_id {
        return Err("account and offset account must share the same commodity".to_string());
    }

    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;
    let tx_memo = memo
        .map(|m| format!("[BALANCING] {}", m))
        .unwrap_or_else(|| "[BALANCING] Balance adjustment".to_string());
    let occurred_at_utc = format!("{}T00:00:00.000Z", as_of_date);

    tx.execute(
        "INSERT INTO transactions (
                book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
            payee_id, memo, status, reference, import_id, session_id, created_at
         )
         VALUES (
                ?1, NULL, ?2, ?3, ?4, date('now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'), 'Etc/UTC',
                NULL, ?5, 'cleared', ?6, NULL, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            SINGLE_BOOK_ID,
            as_of_date,
            occurred_at_utc,
            "Etc/UTC",
            tx_memo,
            BALANCING_TX_MARK,
            session_id
        ],
    )
    .map_err(|e| e.to_string())?;

    let tx_id = tx.last_insert_rowid();
    clear_redo_stack(&tx, &session_id)?;
    record_insert_change(&tx, &session_id, "transactions", tx_id)?;

    tx.execute(
        "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at)
         VALUES (?1, ?2, ?3, ?4, NULL, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![tx_id, account_id, account_commodity_id, adjustment_minor],
    )
    .map_err(|e| e.to_string())?;

    tx.execute(
        "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at)
         VALUES (?1, ?2, ?3, ?4, NULL, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![tx_id, offset_account_id, account_commodity_id, -adjustment_minor],
    )
    .map_err(|e| e.to_string())?;

    tx.execute(
        "INSERT INTO balance_adjustments (
            book_id, balance_check_id, account_id, offset_account_id, as_of_date,
            target_balance_minor, actual_balance_minor, adjustment_minor, tx_id, mark, memo, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5,
            ?6, ?7, ?8, ?9, ?10, ?11, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            SINGLE_BOOK_ID,
            balance_check_id,
            account_id,
            offset_account_id,
            as_of_date,
            target_balance_minor,
            actual_balance_minor,
            adjustment_minor,
            tx_id,
            BALANCING_TX_MARK,
            memo,
        ],
    )
    .map_err(|e| e.to_string())?;

    let adjustment_id = tx.last_insert_rowid();
    tx.commit().map_err(|e| e.to_string())?;

    let adjustment = conn
        .query_row(
            "SELECT id, book_id, balance_check_id, account_id, offset_account_id, as_of_date,
                    target_balance_minor, actual_balance_minor, adjustment_minor, tx_id, mark, memo, created_at
             FROM balance_adjustments WHERE id = ?1",
            [adjustment_id],
            map_balance_adjustment_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(Some(adjustment))
}

fn map_note_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Note> {
    Ok(Note {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        tx_id: row.get(3)?,
        note: row.get(4)?,
        note_date: row.get(5)?,
        created_at: row.get(6)?,
    })
}

fn map_event_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Event> {
    Ok(Event {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        tx_id: row.get(3)?,
        event_type: row.get(4)?,
        event_date: row.get(5)?,
        description: row.get(6)?,
        metadata: row.get(7)?,
        created_at: row.get(8)?,
        updated_at: row.get(9)?,
    })
}

fn map_document_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<Document> {
    Ok(Document {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        tx_id: row.get(3)?,
        doc_type: row.get(4)?,
        title: row.get(5)?,
        uri: row.get(6)?,
        mime_type: row.get(7)?,
        notes: row.get(8)?,
        created_at: row.get(9)?,
        updated_at: row.get(10)?,
    })
}

#[allow(dead_code)]
fn map_balance_constraint_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<BalanceConstraint> {
    Ok(BalanceConstraint {
        id: row.get(0)?,
        book_id: row.get(1)?,
        account_id: row.get(2)?,
        min_balance_minor: row.get(3)?,
        max_balance_minor: row.get(4)?,
        sign_rule: row.get(5)?,
        created_at: row.get(6)?,
        updated_at: row.get(7)?,
    })
}

#[command]
pub fn create_account(db: State<DbState>, input: AccountCreate) -> Result<Account, AppError> {
    validate_account_type(&input.account_type)?;
    validate_name(&input.name)?;

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let is_closed = if input.is_closed.unwrap_or(false) { 1 } else { 0 };
    let lifecycle_event = if is_closed == 1 { "close" } else { "open" };

    conn.execute(
        "INSERT INTO accounts (book_id, parent_id, previous_account_id, session_id, type, name, commodity_id, institution_id, country_id, number_last4, is_closed, is_hidden, is_system, system_role, effective_at, lifecycle_event, created_at)
         VALUES (?1, ?2, NULL, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, 0, 0, NULL, date('now'), ?11, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.parent_id,
            session_id,
            input.account_type,
            input.name,
            input.commodity_id,
            input.institution_id,
            input.country_id,
            input.number_last4,
            is_closed,
            lifecycle_event,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "accounts", id)?;
    let account = conn
        .query_row(
            "SELECT a.id, a.book_id, a.parent_id, a.type, a.name, a.commodity_id,
                    a.institution_id, i.name, a.country_id, c.name,
                    a.number_last4, a.is_closed, a.is_hidden, a.is_system, a.system_role, a.created_at
             FROM accounts a
             LEFT JOIN institutions i ON a.institution_id = i.id
             LEFT JOIN countries c ON a.country_id = c.id
             WHERE a.id = ?1",
            [id],
            map_account_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(account)
}

#[command]
pub fn get_account(
    db: State<DbState>,
    id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Option<Account>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let account = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_account_id
                 FROM accounts a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_account_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM accounts a
                 JOIN chain c ON a.previous_account_id = c.id
             )
             SELECT a.id, a.book_id, a.parent_id, a.type, a.name, a.commodity_id,
                    a.institution_id, i.name, a.country_id, c.name,
                      a.number_last4, a.is_closed, a.is_hidden, a.is_system, a.system_role, a.created_at
             FROM accounts a
             LEFT JOIN institutions i ON a.institution_id = i.id
             LEFT JOIN countries c ON a.country_id = c.id
             WHERE a.id IN chain
                             AND (?2 IS NULL OR date(a.effective_at) <= date(?2))
               AND NOT EXISTS (
                   SELECT 1 FROM accounts newer
                   WHERE newer.previous_account_id = a.id
                     AND newer.id IN chain
                                         AND (?2 IS NULL OR date(newer.effective_at) <= date(?2))
               )
             LIMIT 1",
            params![id, as_of_timestamp],
            map_account_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(account)
}

#[command]
pub fn list_accounts(
    db: State<DbState>,
    book_id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Vec<Account>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT a.id, a.book_id, a.parent_id, a.type, a.name, a.commodity_id,
                    a.institution_id, i.name, a.country_id, c.name,
                    a.number_last4, a.is_closed, a.is_hidden, a.is_system, a.system_role, a.created_at
             FROM accounts a
             LEFT JOIN institutions i ON a.institution_id = i.id
             LEFT JOIN countries c ON a.country_id = c.id
                             WHERE a.book_id = ?1
                                                                 AND (?2 IS NULL OR date(a.effective_at) <= date(?2))
                                 AND a.is_hidden = 0
                                                                 AND NOT EXISTS (
                                                                         SELECT 1 FROM accounts newer
                                                                         WHERE newer.previous_account_id = a.id
                                                                             AND (?2 IS NULL OR date(newer.effective_at) <= date(?2))
                                                                 )
                                 AND NOT EXISTS (
                                         SELECT 1 FROM session_reverts sr
                                         WHERE sr.table_name = 'accounts'
                                             AND sr.row_id = a.id
                                             AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                                 )
                             ORDER BY a.name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_timestamp], map_account_row)
        .map_err(|e| e.to_string())?;

    let mut accounts = Vec::new();
    for row in rows {
        accounts.push(row.map_err(|e| e.to_string())?);
    }

    Ok(accounts)
}

#[command]
pub fn update_account(db: State<DbState>, input: AccountUpdate) -> Result<Account, AppError> {
    validate_account_type(&input.account_type)?;
    validate_name(&input.name)?;

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let current_id = resolve_current_account_id(conn, input.id)?
        .ok_or_else(|| "account not found".to_string())?;

    let source: Option<(String, i64, i64, Option<String>)> = conn
        .query_row(
            "SELECT booking_policy, is_hidden, is_system, system_role
             FROM accounts WHERE id = ?1",
            [current_id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let (booking_policy, is_hidden, is_system, system_role) =
        source.ok_or_else(|| "account not found".to_string())?;
    if is_system == 1 {
        return Err(AppError::conflict("system accounts cannot be updated"));
    }

    let is_closed = if input.is_closed { 1 } else { 0 };
    conn.execute(
        "INSERT INTO accounts (
            book_id, parent_id, previous_account_id, session_id,
            type, name, commodity_id, booking_policy, institution_id, country_id, number_last4,
                is_hidden, is_system, system_role, is_closed,
                effective_at, lifecycle_event, lifecycle_note, lifecycle_metadata,
                created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4,
            ?5, ?6, ?7, ?8, ?9, ?10, ?11,
                ?12, ?13, ?14, ?15,
                date('now'), 'update', NULL, NULL,
                strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.parent_id,
            current_id,
            session_id,
            input.account_type,
            input.name,
            input.commodity_id,
            booking_policy,
            input.institution_id,
            input.country_id,
            input.number_last4,
            is_hidden,
            is_system,
            system_role,
            is_closed,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "accounts", new_id)?;

    let account = conn
        .query_row(
            "SELECT a.id, a.book_id, a.parent_id, a.type, a.name, a.commodity_id,
                    a.institution_id, i.name, a.country_id, c.name,
                    a.number_last4, a.is_closed, a.is_hidden, a.is_system, a.system_role, a.created_at
             FROM accounts a
             LEFT JOIN institutions i ON a.institution_id = i.id
             LEFT JOIN countries c ON a.country_id = c.id
             WHERE a.id = ?1",
            [new_id],
            map_account_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(account)
}

#[command]
pub fn delete_account(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let current_id = resolve_current_account_id(conn, id)?;
    if current_id.is_none() {
        return Ok(false);
    }
    let current_id = current_id.unwrap();

    let is_system = conn
        .query_row("SELECT is_system FROM accounts WHERE id = ?1", [current_id], |row| row.get::<_, i64>(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if is_system == Some(1) {
        return Err(AppError::conflict("system accounts cannot be deleted"));
    }

    let source: (i64, Option<i64>, String, String, i64, String, Option<i64>, Option<i64>, Option<String>, i64, i64, Option<String>) = conn
        .query_row(
            "SELECT book_id, parent_id, type, name, commodity_id, booking_policy, institution_id, country_id, number_last4, is_system, is_hidden, system_role
             FROM accounts WHERE id = ?1",
            [current_id],
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
                ))
            },
        )
        .map_err(|e| e.to_string())?;

    conn.execute(
        "INSERT INTO accounts (
            book_id, parent_id, previous_account_id, session_id,
            type, name, commodity_id, booking_policy, institution_id, country_id, number_last4,
            is_hidden, is_system, system_role, is_closed,
            effective_at, lifecycle_event, lifecycle_note, lifecycle_metadata,
            created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4,
            ?5, ?6, ?7, ?8, ?9, ?10, ?11,
            1, ?12, ?13, 1,
            date('now'), 'close', 'deleted', NULL,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            source.0,
            source.1,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
            source.2,
            source.3,
            source.4,
            source.5,
            source.6,
            source.7,
            source.8,
            source.9,
            source.11,
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "accounts", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn ensure_profit_loss_system_accounts(
    db: State<DbState>,
    book_id: i64,
) -> Result<ProfitLossSystemAccounts, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;
    Ok(ensure_profit_loss_system_accounts_internal(conn, book_id)?)
}

#[command]
pub fn close_fiscal_year(
    db: State<DbState>,
    input: FiscalYearCloseInput,
) -> Result<FiscalYearCloseResult, AppError> {
    validate_close_date(&input.close_date)?;

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;
    let system_accounts = ensure_profit_loss_system_accounts_internal(&tx, book_id)?;
    let reference = format!("year-close:{}", input.close_date);

    let existing_close_id: Option<i64> = tx
        .query_row(
            "SELECT t.id
             FROM transactions t
             WHERE t.book_id = ?1
               AND t.reference = ?2
                             AND t.status != 'void'
               AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
               AND NOT EXISTS (
                 SELECT 1 FROM session_reverts sr
                 WHERE sr.table_name = 'transactions'
                   AND sr.row_id = t.id
                   AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
               )
             LIMIT 1",
            params![book_id, reference],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;
    if existing_close_id.is_some() {
        return Err(AppError::conflict("fiscal year already closed for close_date"));
    }

    let mut account_adjustments: Vec<(i64, i64, String)> = Vec::new();
    let mut retained_earnings_delta_minor: i64 = 0;
    {
        let mut stmt = tx
            .prepare(
                                "SELECT a.id, a.type, a.commodity_id, COALESCE(b.balance_minor, 0) AS balance_minor
                                 FROM accounts a
                                 LEFT JOIN (
                                         SELECT s.account_id, SUM(s.amount_minor) AS balance_minor
                                         FROM splits s
                                         JOIN transactions t ON t.id = s.tx_id
                                         WHERE t.book_id = ?1
                                             AND t.occurred_date <= ?2
                                             AND t.status != 'void'
                                             AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                                             AND NOT EXISTS (
                                                     SELECT 1 FROM session_reverts sr
                                                     WHERE sr.table_name = 'transactions'
                                                         AND sr.row_id = t.id
                                                         AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                                             )
                                         GROUP BY s.account_id
                                 ) b ON b.account_id = a.id
                                 WHERE a.book_id = ?1
                                     AND a.type IN ('income', 'expense')
                                     AND a.is_system = 0
                                     AND COALESCE(b.balance_minor, 0) != 0",
            )
            .map_err(|e| e.to_string())?;

        let rows = stmt
            .query_map(params![book_id, input.close_date], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, i64>(2)?,
                    row.get::<_, i64>(3)?,
                ))
            })
            .map_err(|e| e.to_string())?;

        for row in rows {
            let (account_id, account_type, commodity_id, balance_minor) = row.map_err(|e| e.to_string())?;
            if commodity_id != system_accounts.commodity_id {
                return Err(AppError::internal(format!(
                    "account {} uses commodity {} but fiscal close requires base commodity {}",
                    account_id, commodity_id, system_accounts.commodity_id
                )));
            }
            account_adjustments.push((account_id, -balance_minor, account_type));
            retained_earnings_delta_minor += balance_minor;
        }
    }

    if account_adjustments.is_empty() {
        tx.commit().map_err(|e| e.to_string())?;
        return Ok(FiscalYearCloseResult {
            tx_id: None,
            closed_accounts_count: 0,
            retained_earnings_delta_minor: 0,
            close_date: input.close_date,
        });
    }

    let mut splits: Vec<(i64, i64)> = Vec::new();
    let mut income_summary_balance: i64 = 0;
    let mut expense_summary_balance: i64 = 0;

    for (account_id, adjustment_minor, account_type) in &account_adjustments {
        splits.push((*account_id, *adjustment_minor));
        let summary_amount = -*adjustment_minor;
        if account_type == "income" {
            splits.push((system_accounts.income_summary_account_id, summary_amount));
            income_summary_balance += summary_amount;
        } else {
            splits.push((system_accounts.expense_summary_account_id, summary_amount));
            expense_summary_balance += summary_amount;
        }
    }

    if income_summary_balance != 0 {
        splits.push((system_accounts.income_summary_account_id, -income_summary_balance));
    }
    if expense_summary_balance != 0 {
        splits.push((system_accounts.expense_summary_account_id, -expense_summary_balance));
    }
    if retained_earnings_delta_minor != 0 {
        splits.push((
            system_accounts.retained_earnings_account_id,
            retained_earnings_delta_minor,
        ));
    }

    let split_sum: i64 = splits.iter().map(|(_, amount)| *amount).sum();
    if split_sum != 0 {
        return Err(AppError::internal("fiscal close failed: generated splits are unbalanced"));
    }

    let session_id = current_session_id(&tx)?;
    let occurred_at_utc = format!("{}T23:59:59.000Z", input.close_date);
    let memo = input
        .memo
        .unwrap_or_else(|| format!("Fiscal year close {}", input.close_date));

    tx.execute(
        "INSERT INTO transactions (
                book_id, previous_tx_id, occurred_date, occurred_at_utc, occurred_tz, posted_date, posted_at_utc, posted_tz,
            payee_id, memo, status, reference, import_id, session_id, created_at
         )
         VALUES (
                ?1, NULL, ?2, ?3, ?4, date('now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'), 'Etc/UTC',
                NULL, ?5, 'cleared', ?6, NULL, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.close_date,
            occurred_at_utc,
            "Etc/UTC",
            memo,
            reference,
            session_id
        ],
    )
    .map_err(|e| e.to_string())?;

    let tx_id = tx.last_insert_rowid();
    clear_redo_stack(&tx, &session_id)?;
    record_insert_change(&tx, &session_id, "transactions", tx_id)?;

    for (account_id, amount_minor) in splits {
        if amount_minor == 0 {
            continue;
        }
        tx.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, category_id, memo, created_at)
             VALUES (?1, ?2, ?3, ?4, NULL, NULL, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![
                tx_id,
                account_id,
                system_accounts.commodity_id,
                amount_minor,
            ],
        )
        .map_err(|e| e.to_string())?;
    }

    tx.commit().map_err(|e| e.to_string())?;
    Ok(FiscalYearCloseResult {
        tx_id: Some(tx_id),
        closed_accounts_count: account_adjustments.len() as i64,
        retained_earnings_delta_minor,
        close_date: input.close_date,
    })
}

fn validate_institution_kind(kind: &str) -> Result<(), String> {
    match kind {
        "bank" | "broker" | "credit_union" | "other" => Ok(()),
        _ => Err("institution kind must be bank, broker, credit_union, or other".to_string()),
    }
}

#[command]
pub fn list_countries(db: State<DbState>, book_id: i64) -> Result<Vec<Country>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT c.id, c.book_id, c.code, c.name, c.default_currency_id,
                    cur.code, cur.name, cur.symbol, c.created_at
             FROM countries c
             LEFT JOIN currencies cur ON c.default_currency_id = cur.id
                         WHERE c.book_id = ?1
                            AND (c.session_id IS NULL OR c.session_id NOT LIKE 'void:%')
                             AND NOT EXISTS (
                                     SELECT 1 FROM countries newer
                                     WHERE newer.previous_country_id = c.id
                             )
                             AND NOT EXISTS (
                                     SELECT 1 FROM session_reverts sr
                                     WHERE sr.table_name = 'countries'
                                         AND sr.row_id = c.id
                                         AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY c.name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_country_row)
        .map_err(|e| e.to_string())?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row.map_err(|e| e.to_string())?);
    }

    Ok(items)
}

#[command]
pub fn create_country(db: State<DbState>, input: CountryCreate) -> Result<Country, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO countries (book_id, code, name, default_currency_id, previous_country_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, NULL, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.code, input.name, input.default_currency_id, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "countries", id)?;
    let item = conn
        .query_row(
            "SELECT c.id, c.book_id, c.code, c.name, c.default_currency_id,
                    cur.code, cur.name, cur.symbol, c.created_at
             FROM countries c
             LEFT JOIN currencies cur ON c.default_currency_id = cur.id
             WHERE c.id = ?1",
            [id],
            map_country_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[command]
pub fn get_country(db: State<DbState>, id: i64) -> Result<Option<Country>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let item = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_country_id
                 FROM countries a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_country_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM countries a
                 JOIN chain c ON a.previous_country_id = c.id
             )
             SELECT c.id, c.book_id, c.code, c.name, c.default_currency_id,
                      cur.code, cur.name, cur.symbol, c.created_at
             FROM countries c
             LEFT JOIN currencies cur ON c.default_currency_id = cur.id
             WHERE c.id IN chain
                             AND (c.session_id IS NULL OR c.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM countries newer
                   WHERE newer.previous_country_id = c.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            map_country_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;
    Ok(item)
}

#[command]
pub fn update_country(db: State<DbState>, input: CountryUpdate) -> Result<Country, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let exists: Option<i64> = conn
        .query_row("SELECT id FROM countries WHERE id = ?1", [input.id], |row| row.get(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if exists.is_none() {
        return Err(AppError::not_found("country", "requested"));
    }

    conn.execute(
        "INSERT INTO countries (book_id, code, name, default_currency_id, previous_country_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.code, input.name, input.default_currency_id, input.id, session_id],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "countries", new_id)?;

    let item = conn
        .query_row(
            "SELECT c.id, c.book_id, c.code, c.name, c.default_currency_id,
                    cur.code, cur.name, cur.symbol, c.created_at
             FROM countries c
             LEFT JOIN currencies cur ON c.default_currency_id = cur.id
             WHERE c.id = ?1",
            [new_id],
            map_country_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[command]
pub fn delete_country(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let source: Option<(i64, i64, String, String, Option<i64>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_country_id
                 FROM countries a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_country_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM countries a
                 JOIN chain c ON a.previous_country_id = c.id
             )
             SELECT c.id, c.book_id, c.code, c.name, c.default_currency_id
             FROM countries c
             WHERE c.id IN chain
               AND (c.session_id IS NULL OR c.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM countries newer
                   WHERE newer.previous_country_id = c.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, code, name, default_currency_id)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO countries (book_id, code, name, default_currency_id, previous_country_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            code,
            name,
            default_currency_id,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "countries", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn list_institutions(db: State<DbState>, book_id: i64) -> Result<Vec<Institution>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT i.id, i.book_id, i.name, i.kind, i.country_id, c.name, i.created_at
             FROM institutions i
             LEFT JOIN countries c ON i.country_id = c.id
                         WHERE i.book_id = ?1
                            AND (i.session_id IS NULL OR i.session_id NOT LIKE 'void:%')
                             AND NOT EXISTS (
                                     SELECT 1 FROM institutions newer
                                     WHERE newer.previous_institution_id = i.id
                             )
                             AND NOT EXISTS (
                                     SELECT 1 FROM session_reverts sr
                                     WHERE sr.table_name = 'institutions'
                                         AND sr.row_id = i.id
                                         AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY i.name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], map_institution_row)
        .map_err(|e| e.to_string())?;

    let mut items = Vec::new();
    for row in rows {
        items.push(row.map_err(|e| e.to_string())?);
    }

    Ok(items)
}

#[command]
pub fn create_institution(db: State<DbState>, input: InstitutionCreate) -> Result<Institution, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let kind = input.kind.to_lowercase();
    validate_institution_kind(&kind)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO institutions (book_id, name, kind, country_id, previous_institution_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, NULL, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.name, kind, input.country_id, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "institutions", id)?;
    let item = conn
        .query_row(
            "SELECT i.id, i.book_id, i.name, i.kind, i.country_id, c.name, i.created_at
             FROM institutions i
             LEFT JOIN countries c ON i.country_id = c.id
             WHERE i.id = ?1",
            [id],
            map_institution_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[command]
pub fn get_institution(db: State<DbState>, id: i64) -> Result<Option<Institution>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let item = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_institution_id
                 FROM institutions a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_institution_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM institutions a
                 JOIN chain c ON a.previous_institution_id = c.id
             )
             SELECT i.id, i.book_id, i.name, i.kind, i.country_id, c.name, i.created_at
             FROM institutions i
             LEFT JOIN countries c ON i.country_id = c.id
             WHERE i.id IN chain
                             AND (i.session_id IS NULL OR i.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM institutions newer
                   WHERE newer.previous_institution_id = i.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            map_institution_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;
    Ok(item)
}

#[command]
pub fn update_institution(db: State<DbState>, input: InstitutionUpdate) -> Result<Institution, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let kind = input.kind.to_lowercase();
    validate_institution_kind(&kind)?;

    let book_id = SINGLE_BOOK_ID;

    let exists: Option<i64> = conn
        .query_row("SELECT id FROM institutions WHERE id = ?1", [input.id], |row| row.get(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if exists.is_none() {
        return Err(AppError::not_found("institution", "requested"));
    }

    conn.execute(
        "INSERT INTO institutions (book_id, name, kind, country_id, previous_institution_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.name, kind, input.country_id, input.id, session_id],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "institutions", new_id)?;

    let item = conn
        .query_row(
            "SELECT i.id, i.book_id, i.name, i.kind, i.country_id, c.name, i.created_at
             FROM institutions i
             LEFT JOIN countries c ON i.country_id = c.id
             WHERE i.id = ?1",
            [new_id],
            map_institution_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(item)
}

#[command]
pub fn delete_institution(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let source: Option<(i64, i64, String, String, Option<i64>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_institution_id
                 FROM institutions a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_institution_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM institutions a
                 JOIN chain c ON a.previous_institution_id = c.id
             )
             SELECT i.id, i.book_id, i.name, i.kind, i.country_id
             FROM institutions i
             WHERE i.id IN chain
               AND (i.session_id IS NULL OR i.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM institutions newer
                   WHERE newer.previous_institution_id = i.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, name, kind, country_id)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO institutions (book_id, name, kind, country_id, previous_institution_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            name,
            kind,
            country_id,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "institutions", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn list_account_balances(db: State<DbState>, book_id: i64) -> Result<Vec<AccountBalance>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let base_commodity_id = resolve_base_commodity_id(conn, book_id)?;

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
            // Use a filtered subquery so voided and superseded transactions are
            // excluded from balances displayed on the dashboard and account tree.
            "SELECT a.id, a.commodity_id, c.scale, COALESCE(b.balance_minor, 0)
             FROM accounts a
             JOIN commodities c ON c.id = a.commodity_id
             LEFT JOIN (
                 SELECT s.account_id, SUM(s.amount_minor) AS balance_minor
                 FROM splits s
                 JOIN transactions t ON t.id = s.tx_id
                 WHERE t.book_id = ?1
                   AND t.status != 'void'
                   AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
                   AND NOT EXISTS (
                       SELECT 1 FROM session_reverts sr
                       WHERE sr.table_name = 'transactions'
                         AND sr.row_id = t.id
                         AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                   )
                 GROUP BY s.account_id
             ) b ON b.account_id = a.id
             WHERE a.book_id = ?1",
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
) -> Result<AccountBalancing, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let account_book_id: Option<i64> = tx
        .query_row(
            "SELECT book_id FROM accounts WHERE id = ?1",
            [input.account_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    match account_book_id {
        Some(account_book_id) if account_book_id == book_id => {}
        Some(_) => return Err(AppError::conflict("account does not belong to book")),
        None => return Err(AppError::not_found("account", "requested")),
    }

    let latest: Option<String> = tx
        .query_row(
            "SELECT MAX(ab.as_of_date)
             FROM account_balancings ab
             WHERE ab.account_id = ?1
               AND ab.voided_at IS NULL
               AND NOT EXISTS (
                   SELECT 1 FROM account_balancings newer
                   WHERE newer.previous_account_balancing_id = ab.id
               )",
            [input.account_id],
            |row| row.get::<_, Option<String>>(0),
        )
        .optional()
        .map_err(|e| e.to_string())?
        .flatten();

    if let Some(max_date) = latest {
        if input.as_of_date <= max_date {
            return Err(AppError::validation("balance_date", "balancing date must be after last active balancing"));
        }
    }

    tx.execute(
        "INSERT INTO account_balancings (
            book_id, account_id, as_of_date, balance_minor, memo, voided_at, void_reason,
                previous_account_balancing_id, session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5, NULL, NULL,
                NULL, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.account_id,
            input.as_of_date,
            input.balance_minor,
            input.memo,
            current_session_id(&tx)?,
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
) -> Result<Vec<AccountBalancing>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, as_of_date, balance_minor, memo, created_at, voided_at, void_reason
             FROM account_balancings
             WHERE account_id = ?1
               AND NOT EXISTS (
                   SELECT 1 FROM account_balancings newer
                   WHERE newer.previous_account_balancing_id = account_balancings.id
               )
               AND NOT EXISTS (
                   SELECT 1 FROM session_reverts sr
                   WHERE sr.table_name = 'account_balancings'
                     AND sr.row_id = account_balancings.id
                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
               )
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
) -> Result<i64, AppError> {
    if !input.confirm {
        return Err(AppError::conflict("unlock not confirmed"));
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let tx = conn.transaction().map_err(|e| e.to_string())?;
    let session_id = current_session_id(&tx)?;

    let ids = {
        let mut select_stmt = tx
            .prepare(
                "SELECT id
                 FROM account_balancings ab
                 WHERE ab.account_id = ?1
                   AND ab.voided_at IS NULL
                   AND ab.as_of_date >= ?2
                   AND NOT EXISTS (
                       SELECT 1 FROM account_balancings newer
                       WHERE newer.previous_account_balancing_id = ab.id
                   )",
            )
            .map_err(|e| e.to_string())?;

        let ids_iter = select_stmt
            .query_map(params![input.account_id, input.from_date], |row| row.get::<_, i64>(0))
            .map_err(|e| e.to_string())?;
        let mut ids = Vec::new();
        for id in ids_iter {
            ids.push(id.map_err(|e| e.to_string())?);
        }
        ids
    };

    let mut inserted = 0i64;
    for id in ids {
        tx.execute(
            "INSERT INTO account_balancings (
                book_id, account_id, as_of_date, balance_minor, memo, voided_at, void_reason,
                     previous_account_balancing_id, session_id, created_at
             )
             SELECT
                book_id, account_id, as_of_date, balance_minor, memo,
                strftime('%Y-%m-%dT%H:%M:%fZ','now'), ?1,
                id, ?2,
                     strftime('%Y-%m-%dT%H:%M:%fZ','now')
             FROM account_balancings
             WHERE id = ?3",
            params![input.reason, session_id, id],
        )
        .map_err(|e| e.to_string())?;

        let new_id = tx.last_insert_rowid();
        clear_redo_stack(&tx, &session_id)?;
        record_insert_change(&tx, &session_id, "account_balancings", new_id)?;
        inserted += 1;
    }

    tx.commit().map_err(|e| e.to_string())?;
    Ok(inserted)
}

#[command]
pub fn list_categories(
    db: State<DbState>,
    book_id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Vec<Category>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
                        "SELECT id, book_id, parent_id, previous_category_id, name, kind, color,
                                        created_at
                         FROM categories
                         WHERE book_id = ?1
                            AND (categories.session_id IS NULL OR categories.session_id NOT LIKE 'void:%')
                             AND (?2 IS NULL OR categories.created_at <= ?2)
                             AND NOT EXISTS (
                                 SELECT 1 FROM categories newer
                                 WHERE newer.previous_category_id = categories.id
                                   AND (?2 IS NULL OR newer.created_at <= ?2)
                             )
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'categories'
                                     AND sr.row_id = categories.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_timestamp], map_category_row)
        .map_err(|e| e.to_string())?;

    let mut categories = Vec::new();
    for row in rows {
        categories.push(row.map_err(|e| e.to_string())?);
    }

    Ok(categories)
}

#[command]
pub fn create_category(db: State<DbState>, input: CategoryCreate) -> Result<Category, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO categories (
            book_id, parent_id, name, kind, color, previous_category_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5, NULL,
                ?6,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.parent_id, input.name, input.kind, input.color, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "categories", id)?;
    let category = conn
        .query_row(
            "SELECT id, book_id, parent_id, previous_category_id, name, kind, color,
                    created_at
             FROM categories WHERE id = ?1",
            [id],
            map_category_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(category)
}

#[command]
pub fn get_category(
    db: State<DbState>,
    id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Option<Category>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let category = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_category_id
                 FROM categories a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_category_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM categories a
                 JOIN chain c ON a.previous_category_id = c.id
             )
             SELECT id, book_id, parent_id, previous_category_id, name, kind, color,
                      created_at
             FROM categories
             WHERE id IN chain
                             AND (categories.session_id IS NULL OR categories.session_id NOT LIKE 'void:%')
               AND (?2 IS NULL OR created_at <= ?2)
               AND NOT EXISTS (
                   SELECT 1 FROM categories newer
                   WHERE newer.previous_category_id = categories.id
                     AND newer.id IN chain
                     AND (?2 IS NULL OR newer.created_at <= ?2)
               )
             LIMIT 1",
            params![id, as_of_timestamp],
            map_category_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(category)
}

#[command]
pub fn update_category(db: State<DbState>, input: CategoryUpdate) -> Result<Category, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let exists: Option<i64> = conn
        .query_row("SELECT id FROM categories WHERE id = ?1", [input.id], |row| row.get(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if exists.is_none() {
        return Err(AppError::not_found("category", "requested"));
    }

    conn.execute(
        "INSERT INTO categories (
            book_id, parent_id, name, kind, color, previous_category_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5, ?6,
                ?7,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            input.parent_id,
            input.name,
            input.kind,
            input.color,
            input.id,
            session_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "categories", new_id)?;

    let category = conn
        .query_row(
            "SELECT id, book_id, parent_id, previous_category_id, name, kind, color,
                    created_at
             FROM categories WHERE id = ?1",
            [new_id],
            map_category_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(category)
}

#[command]
pub fn delete_category(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let source: Option<(i64, i64, Option<i64>, String, String, Option<String>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_category_id
                 FROM categories a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_category_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM categories a
                 JOIN chain c ON a.previous_category_id = c.id
             )
             SELECT id, book_id, parent_id, name, kind, color
             FROM categories
             WHERE id IN chain
               AND (categories.session_id IS NULL OR categories.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM categories newer
                   WHERE newer.previous_category_id = categories.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?, row.get(5)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, parent_id, name, kind, color)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO categories (
            book_id, parent_id, name, kind, color, previous_category_id,
            session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5, ?6,
            ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            parent_id,
            name,
            kind,
            color,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "categories", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn list_payees(
    db: State<DbState>,
    book_id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Vec<Payee>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
                        "SELECT id, book_id, previous_payee_id, name, kind, metadata,
                                        created_at
                         FROM payees
                         WHERE book_id = ?1
                            AND (payees.session_id IS NULL OR payees.session_id NOT LIKE 'void:%')
                             AND (?2 IS NULL OR payees.created_at <= ?2)
                             AND NOT EXISTS (
                                 SELECT 1 FROM payees newer
                                 WHERE newer.previous_payee_id = payees.id
                                   AND (?2 IS NULL OR newer.created_at <= ?2)
                             )
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'payees'
                                     AND sr.row_id = payees.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_timestamp], map_payee_row)
        .map_err(|e| e.to_string())?;

    let mut payees = Vec::new();
    for row in rows {
        payees.push(row.map_err(|e| e.to_string())?);
    }

    Ok(payees)
}

#[command]
pub fn create_payee(db: State<DbState>, input: PayeeCreate) -> Result<Payee, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO payees (
            book_id, name, kind, metadata, previous_payee_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, NULL,
                ?5,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.name, input.kind, input.metadata, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "payees", id)?;
    let payee = conn
        .query_row(
            "SELECT id, book_id, previous_payee_id, name, kind, metadata,
                    created_at
             FROM payees WHERE id = ?1",
            [id],
            map_payee_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(payee)
}

#[command]
pub fn get_payee(
    db: State<DbState>,
    id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Option<Payee>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let payee = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_payee_id
                 FROM payees a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_payee_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM payees a
                 JOIN chain c ON a.previous_payee_id = c.id
             )
             SELECT id, book_id, previous_payee_id, name, kind, metadata,
                      created_at
             FROM payees
             WHERE id IN chain
                             AND (payees.session_id IS NULL OR payees.session_id NOT LIKE 'void:%')
               AND (?2 IS NULL OR created_at <= ?2)
               AND NOT EXISTS (
                   SELECT 1 FROM payees newer
                   WHERE newer.previous_payee_id = payees.id
                     AND newer.id IN chain
                     AND (?2 IS NULL OR newer.created_at <= ?2)
               )
             LIMIT 1",
            params![id, as_of_timestamp],
            map_payee_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(payee)
}

#[command]
pub fn update_payee(db: State<DbState>, input: PayeeUpdate) -> Result<Payee, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let exists: Option<i64> = conn
        .query_row("SELECT id FROM payees WHERE id = ?1", [input.id], |row| row.get(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if exists.is_none() {
        return Err(AppError::not_found("payee", "requested"));
    }

    conn.execute(
        "INSERT INTO payees (
            book_id, name, kind, metadata, previous_payee_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5,
                ?6,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.name, input.kind, input.metadata, input.id, session_id],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "payees", new_id)?;

    let payee = conn
        .query_row(
            "SELECT id, book_id, previous_payee_id, name, kind, metadata,
                    created_at
             FROM payees WHERE id = ?1",
            [new_id],
            map_payee_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(payee)
}

#[command]
pub fn delete_payee(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let source: Option<(i64, i64, String, String, Option<String>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_payee_id
                 FROM payees a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_payee_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM payees a
                 JOIN chain c ON a.previous_payee_id = c.id
             )
             SELECT id, book_id, name, kind, metadata
             FROM payees
             WHERE id IN chain
               AND (payees.session_id IS NULL OR payees.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM payees newer
                   WHERE newer.previous_payee_id = payees.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, name, kind, metadata)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO payees (
            book_id, name, kind, metadata, previous_payee_id,
            session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, ?5,
            ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            name,
            kind,
            metadata,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "payees", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn list_tags(
    db: State<DbState>,
    book_id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Vec<Tag>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
                        "SELECT id, book_id, previous_tag_id, name, color,
                                        created_at
                         FROM tags
                         WHERE book_id = ?1
                            AND (tags.session_id IS NULL OR tags.session_id NOT LIKE 'void:%')
                             AND (?2 IS NULL OR tags.created_at <= ?2)
                             AND NOT EXISTS (
                                 SELECT 1 FROM tags newer
                                 WHERE newer.previous_tag_id = tags.id
                                   AND (?2 IS NULL OR newer.created_at <= ?2)
                             )
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'tags'
                                     AND sr.row_id = tags.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_timestamp], map_tag_row)
        .map_err(|e| e.to_string())?;

    let mut tags = Vec::new();
    for row in rows {
        tags.push(row.map_err(|e| e.to_string())?);
    }

    Ok(tags)
}

#[command]
pub fn create_tag(db: State<DbState>, input: TagCreate) -> Result<Tag, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO tags (
            book_id, name, color, previous_tag_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, NULL,
                ?4,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.name, input.color, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "tags", id)?;
    let tag = conn
        .query_row(
            "SELECT id, book_id, previous_tag_id, name, color,
                    created_at
             FROM tags WHERE id = ?1",
            [id],
            map_tag_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(tag)
}

#[command]
pub fn get_tag(
    db: State<DbState>,
    id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Option<Tag>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let tag = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_tag_id
                 FROM tags a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_tag_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM tags a
                 JOIN chain c ON a.previous_tag_id = c.id
             )
             SELECT id, book_id, previous_tag_id, name, color,
                      created_at
             FROM tags
             WHERE id IN chain
                             AND (tags.session_id IS NULL OR tags.session_id NOT LIKE 'void:%')
               AND (?2 IS NULL OR created_at <= ?2)
               AND NOT EXISTS (
                   SELECT 1 FROM tags newer
                   WHERE newer.previous_tag_id = tags.id
                     AND newer.id IN chain
                     AND (?2 IS NULL OR newer.created_at <= ?2)
               )
             LIMIT 1",
            params![id, as_of_timestamp],
            map_tag_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(tag)
}

#[command]
pub fn update_tag(db: State<DbState>, input: TagUpdate) -> Result<Tag, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let exists: Option<i64> = conn
        .query_row("SELECT id FROM tags WHERE id = ?1", [input.id], |row| row.get(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if exists.is_none() {
        return Err(AppError::not_found("tag", "requested"));
    }

    conn.execute(
        "INSERT INTO tags (
            book_id, name, color, previous_tag_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4,
                ?5,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.name, input.color, input.id, session_id],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "tags", new_id)?;

    let tag = conn
        .query_row(
            "SELECT id, book_id, previous_tag_id, name, color,
                    created_at
             FROM tags WHERE id = ?1",
            [new_id],
            map_tag_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(tag)
}

#[command]
pub fn delete_tag(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let source: Option<(i64, i64, String, Option<String>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_tag_id
                 FROM tags a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_tag_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM tags a
                 JOIN chain c ON a.previous_tag_id = c.id
             )
             SELECT id, book_id, name, color
             FROM tags
             WHERE id IN chain
               AND (tags.session_id IS NULL OR tags.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM tags newer
                   WHERE newer.previous_tag_id = tags.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, name, color)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO tags (
            book_id, name, color, previous_tag_id,
            session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4,
            ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            name,
            color,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "tags", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn list_people(
    db: State<DbState>,
    book_id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Vec<Person>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
                        "SELECT id, book_id, previous_person_id, name, role, metadata,
                                        created_at
                         FROM people
                         WHERE book_id = ?1
                             AND (?2 IS NULL OR people.created_at <= ?2)
                             AND NOT EXISTS (
                                 SELECT 1 FROM people newer
                                 WHERE newer.previous_person_id = people.id
                                   AND (?2 IS NULL OR newer.created_at <= ?2)
                             )
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'people'
                                     AND sr.row_id = people.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_timestamp], map_person_row)
        .map_err(|e| e.to_string())?;

    let mut people = Vec::new();
    for row in rows {
        people.push(row.map_err(|e| e.to_string())?);
    }

    Ok(people)
}

#[command]
pub fn create_person(db: State<DbState>, input: PersonCreate) -> Result<Person, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO people (
            book_id, name, role, metadata, previous_person_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, NULL,
                ?5,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.name, input.role, input.metadata, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "people", id)?;
    let person = conn
        .query_row(
            "SELECT id, book_id, previous_person_id, name, role, metadata,
                    created_at
             FROM people WHERE id = ?1",
            [id],
            map_person_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(person)
}

#[command]
pub fn list_projects(
    db: State<DbState>,
    book_id: i64,
    as_of_timestamp: Option<String>,
) -> Result<Vec<Project>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
                        "SELECT id, book_id, previous_project_id, name, status, metadata,
                                        created_at
                         FROM projects
                         WHERE book_id = ?1
                             AND (?2 IS NULL OR projects.created_at <= ?2)
                             AND NOT EXISTS (
                                 SELECT 1 FROM projects newer
                                 WHERE newer.previous_project_id = projects.id
                                   AND (?2 IS NULL OR newer.created_at <= ?2)
                             )
                             AND NOT EXISTS (
                                 SELECT 1 FROM session_reverts sr
                                 WHERE sr.table_name = 'projects'
                                     AND sr.row_id = projects.id
                                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                             )
                         ORDER BY name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![book_id, as_of_timestamp], map_project_row)
        .map_err(|e| e.to_string())?;

    let mut projects = Vec::new();
    for row in rows {
        projects.push(row.map_err(|e| e.to_string())?);
    }

    Ok(projects)
}

#[command]
pub fn create_project(db: State<DbState>, input: ProjectCreate) -> Result<Project, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO projects (
            book_id, name, status, metadata, previous_project_id,
                session_id, created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4, NULL,
                ?5,
            strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![book_id, input.name, input.status, input.metadata, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "projects", id)?;
    let project = conn
        .query_row(
            "SELECT id, book_id, previous_project_id, name, status, metadata,
                    created_at
             FROM projects WHERE id = ?1",
            [id],
            map_project_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(project)
}

#[command]
pub fn get_account_booking_policy(db: State<DbState>, account_id: i64) -> Result<String, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let current_id = resolve_current_account_id(conn, account_id)?
        .ok_or_else(|| "account not found".to_string())?;

    let account_type: Option<String> = conn
        .query_row(
            "SELECT type FROM accounts WHERE id = ?1",
            [current_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let account_type = account_type.ok_or_else(|| "account not found".to_string())?;
    if account_type != "investment" {
        return Err(AppError::validation("booking_policy", "booking policy only applies to investment accounts"));
    }

    let policy: Option<String> = conn
        .query_row(
            "SELECT booking_policy FROM accounts WHERE id = ?1",
            [current_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    Ok(policy.unwrap_or_else(|| "fifo".to_string()))
}

#[command]
pub fn set_account_booking_policy(
    db: State<DbState>,
    input: AccountBookingPolicyUpdate,
) -> Result<String, AppError> {
    let policy = input.booking_policy.to_lowercase();
    if policy != "fifo" && policy != "lifo" && policy != "strict" && policy != "average" {
        return Err(AppError::validation("booking_policy", "booking policy must be fifo, lifo, strict, or average"));
    }

    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let current_id = resolve_current_account_id(conn, input.account_id)?
        .ok_or_else(|| "account not found".to_string())?;

    let source: Option<(
        i64,
        Option<i64>,
        String,
        String,
        i64,
        Option<i64>,
        Option<i64>,
        Option<String>,
        i64,
        i64,
        Option<String>,
        i64,
    )> = conn
        .query_row(
            "SELECT book_id, parent_id, type, name, commodity_id, institution_id, country_id, number_last4,
                    is_hidden, is_system, system_role, is_closed
             FROM accounts WHERE id = ?1",
            [current_id],
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
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let (
        book_id,
        parent_id,
        account_type,
        name,
        commodity_id,
        institution_id,
        country_id,
        number_last4,
        is_hidden,
        is_system,
        system_role,
        is_closed,
    ) = source.ok_or_else(|| "account not found".to_string())?;

    if is_system == 1 {
        return Err(AppError::conflict("system accounts cannot be updated"));
    }

    if account_type != "investment" {
        return Err(AppError::validation("booking_policy", "booking policy only applies to investment accounts"));
    }

    conn.execute(
        "INSERT INTO accounts (
            book_id, parent_id, previous_account_id, session_id,
            type, name, commodity_id, booking_policy, institution_id, country_id, number_last4,
                is_hidden, is_system, system_role, is_closed,
                effective_at, lifecycle_event, lifecycle_note, lifecycle_metadata,
                created_at
         )
         VALUES (
            ?1, ?2, ?3, ?4,
            ?5, ?6, ?7, ?8, ?9, ?10, ?11,
                ?12, ?13, ?14, ?15,
                date('now'), 'update', NULL, NULL,
                strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            parent_id,
            current_id,
            session_id,
            account_type,
            name,
            commodity_id,
            policy,
            institution_id,
            country_id,
            number_last4,
            is_hidden,
            is_system,
            system_role,
            is_closed,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "accounts", new_id)?;

    Ok(policy)
}

#[allow(dead_code)]
#[command]
pub fn validate_account_closing(db: State<DbState>, account_id: i64) -> Result<bool, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let is_closed: Option<i64> = conn
        .query_row(
            "SELECT is_closed FROM accounts WHERE id = ?1",
            [account_id],
            |row| row.get(0),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let is_closed = match is_closed {
        Some(v) => v != 0,
        None => return Err(AppError::not_found("account", "requested")),
    };

    if !is_closed {
        return Ok(false);
    }

    let balance: i64 = get_account_balance_minor(conn, account_id)?;

    if balance != 0 {
        return Err(AppError::conflict("account closing validation failed: balance is not zero"));
    }

    Ok(true)
}

#[allow(dead_code)]
#[command]
pub fn create_balance_constraint(
    db: State<DbState>,
    input: BalanceConstraintCreate,
) -> Result<BalanceConstraint, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;
    let sign_rule = input
        .sign_rule
        .unwrap_or_else(|| "any".to_string())
        .to_lowercase();
    if sign_rule != "any" && sign_rule != "nonnegative" && sign_rule != "nonpositive" {
        return Err(AppError::validation("sign_rule", "sign_rule must be any, nonnegative, or nonpositive"));
    }
    if let (Some(min), Some(max)) = (input.min_balance_minor, input.max_balance_minor) {
        if min > max {
            return Err(AppError::validation("min_balance_minor", "min_balance_minor must be <= max_balance_minor"));
        }
    }

    conn.execute(
        "INSERT INTO balance_constraints (book_id, account_id, min_balance_minor, max_balance_minor, sign_rule, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.account_id,
            input.min_balance_minor,
            input.max_balance_minor,
            sign_rule,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let constraint = conn
        .query_row(
            "SELECT id, book_id, account_id, min_balance_minor, max_balance_minor, sign_rule, created_at, updated_at
             FROM balance_constraints WHERE id = ?1",
            [id],
            map_balance_constraint_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(constraint)
}

#[allow(dead_code)]
#[command]
pub fn list_balance_constraints(
    db: State<DbState>,
    account_id: i64,
) -> Result<Vec<BalanceConstraint>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, min_balance_minor, max_balance_minor, sign_rule, created_at, updated_at
             FROM balance_constraints WHERE account_id = ?1
             ORDER BY id DESC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([account_id], map_balance_constraint_row)
        .map_err(|e| e.to_string())?;

    let mut constraints = Vec::new();
    for row in rows {
        constraints.push(row.map_err(|e| e.to_string())?);
    }

    Ok(constraints)
}

#[allow(dead_code)]
#[command]
pub fn delete_balance_constraint(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute("DELETE FROM balance_constraints WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(rows > 0)
}

#[allow(dead_code)]
#[command]
pub fn validate_balance_constraints(db: State<DbState>, account_id: i64) -> Result<bool, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let balance: i64 = get_account_balance_minor(conn, account_id)?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, min_balance_minor, max_balance_minor, sign_rule, created_at, updated_at
             FROM balance_constraints WHERE account_id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([account_id], map_balance_constraint_row)
        .map_err(|e| e.to_string())?;

    for row in rows {
        let constraint = row.map_err(|e| e.to_string())?;
        if let Some(min) = constraint.min_balance_minor {
            if balance < min {
                return Err(AppError::conflict("balance constraint violated: below minimum"));
            }
        }
        if let Some(max) = constraint.max_balance_minor {
            if balance > max {
                return Err(AppError::conflict("balance constraint violated: above maximum"));
            }
        }
        if constraint.sign_rule == "nonnegative" && balance < 0 {
            return Err(AppError::conflict("balance constraint violated: must be nonnegative"));
        }
        if constraint.sign_rule == "nonpositive" && balance > 0 {
            return Err(AppError::conflict("balance constraint violated: must be nonpositive"));
        }
    }

    Ok(true)
}

#[allow(dead_code)]
#[command]
pub fn get_account_tree(db: State<DbState>, book_id: i64) -> Result<Vec<AccountTreeNode>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;
    let _ = book_id;
    let book_id = SINGLE_BOOK_ID;

    let mut stmt = conn
        .prepare(
            "SELECT a.id, a.parent_id, a.name, a.type, a.commodity_id, c.name, c.scale,
                i.name, co.name
             FROM accounts a
             JOIN commodities c ON c.id = a.commodity_id
             LEFT JOIN institutions i ON a.institution_id = i.id
             LEFT JOIN countries co ON a.country_id = co.id
             WHERE a.book_id = ?1 AND a.type NOT IN ('income', 'expense')
             ORDER BY a.name ASC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([book_id], |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, Option<i64>>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, String>(3)?,
                row.get::<_, i64>(4)?,
                row.get::<_, String>(5)?,
                row.get::<_, i64>(6)?,
                row.get::<_, Option<String>>(7)?,
                row.get::<_, Option<String>>(8)?,
            ))
        })
        .map_err(|e| e.to_string())?;

    let mut accounts: Vec<(i64, Option<i64>, String, String, i64, String, i64, Option<String>, Option<String>)> = Vec::new();
    for row in rows {
        accounts.push(row.map_err(|e| e.to_string())?);
    }

    let mut balance_stmt = conn
        .prepare(
            // Exclude voided and superseded transactions so the account tree
            // shows correct balances that match what the user can actually spend.
            "SELECT s.account_id, COALESCE(SUM(s.amount_minor), 0) AS balance_minor
             FROM splits s
             JOIN transactions t ON t.id = s.tx_id
             WHERE t.status != 'void'
               AND NOT EXISTS (SELECT 1 FROM transactions newer WHERE newer.previous_tx_id = t.id)
               AND NOT EXISTS (
                   SELECT 1 FROM session_reverts sr
                   WHERE sr.table_name = 'transactions'
                     AND sr.row_id = t.id
                     AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
               )
             GROUP BY s.account_id",
        )
        .map_err(|e| e.to_string())?;
    let balance_rows = balance_stmt
        .query_map([], |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)))
        .map_err(|e| e.to_string())?;

    let mut balances: HashMap<i64, i64> = HashMap::new();
    for row in balance_rows {
        let (account_id, balance_minor) = row.map_err(|e| e.to_string())?;
        balances.insert(account_id, balance_minor);
    }

    let mut children_map: HashMap<Option<i64>, Vec<i64>> = HashMap::new();
    let mut account_map: HashMap<i64, (Option<i64>, String, String, i64, String, i64, Option<String>, Option<String>)> = HashMap::new();

    for (id, parent_id, name, account_type, commodity_id, commodity_name, commodity_scale, institution_name, country_name) in &accounts {
        account_map.insert(
            *id,
            (
                *parent_id,
                name.clone(),
                account_type.clone(),
                *commodity_id,
                commodity_name.clone(),
                *commodity_scale,
                institution_name.clone(),
                country_name.clone(),
            ),
        );
        children_map.entry(*parent_id).or_default().push(*id);
    }

    fn build_tree(
        account_id: i64,
        account_map: &HashMap<i64, (Option<i64>, String, String, i64, String, i64, Option<String>, Option<String>)>,
        children_map: &HashMap<Option<i64>, Vec<i64>>,
        balances: &HashMap<i64, i64>,
    ) -> AccountTreeNode {
        let (parent_id, name, account_type, commodity_id, commodity_name, commodity_scale, institution_name, country_name) =
            account_map.get(&account_id).cloned().unwrap();
        let balance_minor = *balances.get(&account_id).unwrap_or(&0);

        let mut children = Vec::new();
        let mut rollup_balance_minor = balance_minor;
        if let Some(child_ids) = children_map.get(&Some(account_id)) {
            for child_id in child_ids {
                let child = build_tree(*child_id, account_map, children_map, balances);
                rollup_balance_minor += child.rollup_balance_minor;
                children.push(child);
            }
        }

        AccountTreeNode {
            id: account_id,
            parent_id,
            name,
            account_type,
            commodity_id,
            commodity_name,
            commodity_scale,
            institution_name,
            country_name,
            balance_minor,
            rollup_balance_minor,
            children,
        }
    }

    let mut roots = Vec::new();
    if let Some(root_ids) = children_map.get(&None) {
        for root_id in root_ids {
            roots.push(build_tree(*root_id, &account_map, &children_map, &balances));
        }
    }

    Ok(roots)
}

fn create_account_lifecycle_directive_internal(
    conn: &rusqlite::Connection,
    directive_type: &str,
    input: AccountDirectiveCreate,
) -> Result<AccountDirective, String> {
    let current_id = resolve_current_account_id(conn, input.account_id)?
        .ok_or_else(|| "account not found".to_string())?;

    let source: Option<(
        i64,
        Option<i64>,
        String,
        String,
        i64,
        String,
        Option<i64>,
        Option<i64>,
        Option<String>,
        i64,
        i64,
        Option<String>,
    )> = conn
        .query_row(
            "SELECT book_id, parent_id, type, name, commodity_id, booking_policy, institution_id, country_id,
                    number_last4, is_hidden, is_system, system_role
             FROM accounts WHERE id = ?1",
            [current_id],
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
                ))
            },
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let (
        book_id,
        parent_id,
        account_type,
        name,
        commodity_id,
        booking_policy,
        institution_id,
        country_id,
        number_last4,
        is_hidden,
        is_system,
        system_role,
    ) = source.ok_or_else(|| "account not found".to_string())?;

    let is_closed = match directive_type {
        "close" => 1,
        "open" | "reopen" => 0,
        _ => return Err("directive type must be open, close, or reopen".to_string()),
    };

    conn.execute(
        "INSERT INTO accounts (
            book_id, parent_id, previous_account_id, session_id,
            type, name, commodity_id, booking_policy, institution_id, country_id, number_last4,
            is_hidden, is_system, system_role, is_closed,
            effective_at, lifecycle_event, lifecycle_note, lifecycle_metadata,
                created_at
         )
         VALUES (
            ?1, ?2, ?3, NULL,
            ?4, ?5, ?6, ?7, ?8, ?9, ?10,
            ?11, ?12, ?13, ?14,
            ?15, ?16, ?17, ?18,
                strftime('%Y-%m-%dT%H:%M:%fZ','now')
         )",
        params![
            book_id,
            parent_id,
            current_id,
            account_type,
            name,
            commodity_id,
            booking_policy,
            institution_id,
            country_id,
            number_last4,
            is_hidden,
            is_system,
            system_role,
            is_closed,
            input.directive_date,
            directive_type,
            input.note,
            input.metadata,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    conn.query_row(
        "SELECT id, book_id, ?2 as account_id, lifecycle_event, effective_at, lifecycle_note, lifecycle_metadata, created_at
         FROM accounts WHERE id = ?1",
        params![id, input.account_id],
        map_account_directive_row,
    )
    .map_err(|e| e.to_string())
}

#[command]
pub fn create_account_open(
    db: State<DbState>,
    input: AccountDirectiveCreate,
) -> Result<AccountDirective, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    Ok(create_account_lifecycle_directive_internal(conn, "open", input)?)
}

#[command]
pub fn create_account_close(
    db: State<DbState>,
    input: AccountDirectiveCreate,
) -> Result<AccountDirective, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    Ok(create_account_lifecycle_directive_internal(conn, "close", input)?)
}

#[command]
pub fn create_account_reopen(
    db: State<DbState>,
    input: AccountDirectiveCreate,
) -> Result<AccountDirective, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    Ok(create_account_lifecycle_directive_internal(conn, "reopen", input)?)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::{open_and_migrate, open_read_conn};
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
        temp.push(format!("rekenraam_accounts_test_{start}_{counter}"));
        let _ = fs::remove_dir_all(&temp);
        fs::create_dir_all(&temp).expect("create temp dir");

        let (conn, db_path, audit_user) = open_and_migrate(&temp).expect("open and migrate");
        let read_conn = open_read_conn(&db_path).expect("open read conn");
        DbState {
            inner: Mutex::new(DbStateInner {
                db_path: Some(db_path),
                conn: Some(conn),
                audit_user: Some(audit_user),
            }),
            read_conn: Mutex::new(Some(read_conn)),
        }
    }

    fn as_state<'a>(db: &'a DbState) -> State<'a, DbState> {
        unsafe { std::mem::transmute::<&'a DbState, State<'a, DbState>>(db) }
    }

    fn get_usd_commodity_id(db: &DbState) -> i64 {
        let guard = db.inner.lock().expect("lock db");
        let conn = guard.conn.as_ref().expect("conn");
        conn.query_row(
            "SELECT id FROM commodities WHERE symbol = 'USD' LIMIT 1",
            [],
            |row| row.get(0),
        )
        .expect("usd commodity")
    }

    #[test]
    fn test_countries_and_institutions_crud() {
        let db_state = create_temp_db();

        let country = create_country(
            as_state(&db_state),
            CountryCreate {
                book_id: 1,
                code: "ZZ".to_string(),
                name: "Testland".to_string(),
                default_currency_id: None,
            },
        )
        .expect("create country");

        let countries = list_countries(as_state(&db_state), 1).expect("list countries");
        assert!(countries.iter().any(|c| c.id == country.id));

        let updated_country = update_country(
            as_state(&db_state),
            CountryUpdate {
                id: country.id,
                book_id: 1,
                code: "ZZ".to_string(),
                name: "Testland Republic".to_string(),
                default_currency_id: None,
            },
        )
        .expect("update country");
        assert_eq!(updated_country.name, "Testland Republic");

        let institution = create_institution(
            as_state(&db_state),
            InstitutionCreate {
                book_id: 1,
                name: "Broker One".to_string(),
                kind: "broker".to_string(),
                country_id: Some(updated_country.id),
            },
        )
        .expect("create institution");

        let institutions = list_institutions(as_state(&db_state), 1).expect("list institutions");
        assert!(institutions.iter().any(|i| i.id == institution.id));

        let updated_institution = update_institution(
            as_state(&db_state),
            InstitutionUpdate {
                id: institution.id,
                book_id: 1,
                name: "Broker Two".to_string(),
                kind: "broker".to_string(),
                country_id: Some(updated_country.id),
            },
        )
        .expect("update institution");
        assert_eq!(updated_institution.name, "Broker Two");

        let deleted_inst = delete_institution(as_state(&db_state), institution.id)
            .expect("delete institution");
        assert!(deleted_inst);

        let deleted_country = delete_country(as_state(&db_state), updated_country.id)
            .expect("delete country");
        assert!(deleted_country);
    }

    #[test]
    fn test_account_crud() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let country = list_countries(as_state(&db_state), 1)
            .expect("list countries")
            .into_iter()
            .find(|c| c.code == "US")
            .expect("US country");

        let institution = create_institution(
            as_state(&db_state),
            InstitutionCreate {
                book_id: 1,
                name: "Test Bank".to_string(),
                kind: "bank".to_string(),
                country_id: Some(country.id),
            },
        )
        .expect("create institution");

        let created = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "cash".to_string(),
                name: "Test Cash".to_string(),
                commodity_id,
                institution_id: Some(institution.id),
                country_id: Some(country.id),
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create account");

        let fetched = get_account(as_state(&db_state), created.id, None)
            .expect("get account")
            .expect("account exists");
        assert_eq!(fetched.name, "Test Cash");
        assert_eq!(fetched.institution_name.as_deref(), Some("Test Bank"));
        assert_eq!(fetched.country_name.as_deref(), Some("United States of America"));

        let updated = update_account(
            as_state(&db_state),
            AccountUpdate {
                id: created.id,
                book_id: 1,
                parent_id: None,
                account_type: "cash".to_string(),
                name: "Renamed Cash".to_string(),
                commodity_id,
                institution_id: Some(institution.id),
                country_id: Some(country.id),
                number_last4: None,
                is_closed: false,
            },
        )
        .expect("update account");
        assert_eq!(updated.name, "Renamed Cash");

        let list = list_accounts(as_state(&db_state), 1, None).expect("list accounts");
        assert!(list.iter().any(|a| a.id == updated.id));

        let deleted = delete_account(as_state(&db_state), created.id).expect("delete account");
        assert!(deleted);
    }

    #[test]
    fn test_account_tree_excludes_income_expense_rollup() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let parent = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "asset".to_string(),
                name: "Parent Asset".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create parent account");

        let child = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: Some(parent.id),
                account_type: "asset".to_string(),
                name: "Child Asset".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create child account");

        let income = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: Some(parent.id),
                account_type: "income".to_string(),
                name: "Income Account".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create income account");

        let expense = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: Some(parent.id),
                account_type: "expense".to_string(),
                name: "Expense Account".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create expense account");

        let equity = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "equity".to_string(),
                name: "Equity".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create equity account");

        {
            let mut guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_mut().expect("conn");

            let insert_tx = |conn: &rusqlite::Connection| -> i64 {
                conn.execute(
                    "INSERT INTO transactions (book_id, occurred_date, status, created_at)
                     VALUES (1, '2024-06-01', 'cleared', strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    [],
                )
                .expect("insert tx");
                conn.last_insert_rowid()
            };

            let tx1 = insert_tx(conn);
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, 1000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx1, parent.id, commodity_id],
            )
            .expect("insert parent split");
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -1000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx1, equity.id, commodity_id],
            )
            .expect("insert equity split 1");

            let tx2 = insert_tx(conn);
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, 2000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx2, child.id, commodity_id],
            )
            .expect("insert child split");
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -2000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx2, equity.id, commodity_id],
            )
            .expect("insert equity split 2");

            let tx3 = insert_tx(conn);
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, 3000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx3, income.id, commodity_id],
            )
            .expect("insert income split");
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -3000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx3, equity.id, commodity_id],
            )
            .expect("insert equity split 3");

            let tx4 = insert_tx(conn);
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -4000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx4, expense.id, commodity_id],
            )
            .expect("insert expense split");
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, 4000, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx4, equity.id, commodity_id],
            )
            .expect("insert equity split 4");
        }

        let tree = get_account_tree(as_state(&db_state), 1).expect("get account tree");

        fn collect_nodes<'a>(nodes: &'a [AccountTreeNode], out: &mut Vec<&'a AccountTreeNode>) {
            for node in nodes {
                out.push(node);
                collect_nodes(&node.children, out);
            }
        }

        let mut all_nodes = Vec::new();
        collect_nodes(&tree, &mut all_nodes);

        assert!(!all_nodes.iter().any(|n| n.account_type == "income"));
        assert!(!all_nodes.iter().any(|n| n.account_type == "expense"));

        let parent_node = all_nodes
            .iter()
            .find(|n| n.id == parent.id)
            .expect("parent node");
        assert_eq!(parent_node.rollup_balance_minor, 3000);
    }

    #[test]
    fn test_account_balancing_flow() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "checking".to_string(),
                name: "Balancing Account".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create account");

        let balancing = create_account_balancing(
            as_state(&db_state),
            AccountBalancingCreate {
                book_id: 1,
                account_id: account.id,
                as_of_date: "2024-01-01".to_string(),
                balance_minor: 1000,
                memo: Some("opening".to_string()),
            },
        )
        .expect("create balancing");
        assert_eq!(balancing.balance_minor, 1000);

        let err = create_account_balancing(
            as_state(&db_state),
            AccountBalancingCreate {
                book_id: 1,
                account_id: account.id,
                as_of_date: "2023-12-31".to_string(),
                balance_minor: 900,
                memo: None,
            },
        )
        .expect_err("should reject earlier balancing date");
        assert!(err.to_string().contains("balancing date"));

        let listed = list_account_balancings(as_state(&db_state), account.id)
            .expect("list balancings");
        assert_eq!(listed.len(), 1);

        let unlocked = unlock_account_balancings(
            as_state(&db_state),
            AccountBalancingUnlockInput {
                account_id: account.id,
                from_date: "2024-01-01".to_string(),
                reason: Some("test".to_string()),
                confirm: true,
            },
        )
        .expect("unlock balancings");
        assert!(unlocked >= 1);
    }

    #[test]
    fn test_booking_policy_and_constraints_and_directives() {
        let db_state = create_temp_db();

        let commodity_id = get_usd_commodity_id(&db_state);
        let account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "investment".to_string(),
                name: "Policy Account".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create account");

        let policy = set_account_booking_policy(
            as_state(&db_state),
            AccountBookingPolicyUpdate {
                account_id: account.id,
                booking_policy: "lifo".to_string(),
            },
        )
        .expect("set booking policy");
        assert_eq!(policy, "lifo");

        let fetched = get_account_booking_policy(as_state(&db_state), account.id)
            .expect("get booking policy");
        assert_eq!(fetched, "lifo");

        let checking = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "checking".to_string(),
                name: "Non Investment".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create checking account");

        let err = set_account_booking_policy(
            as_state(&db_state),
            AccountBookingPolicyUpdate {
                account_id: checking.id,
                booking_policy: "fifo".to_string(),
            },
        )
        .expect_err("non-investment booking policy should fail");
        assert!(err.to_string().contains("investment"));

        let constraint = create_balance_constraint(
            as_state(&db_state),
            BalanceConstraintCreate {
                book_id: 1,
                account_id: account.id,
                min_balance_minor: Some(0),
                max_balance_minor: None,
                sign_rule: Some("nonnegative".to_string()),
            },
        )
        .expect("create constraint");
        let constraints = list_balance_constraints(as_state(&db_state), account.id)
            .expect("list constraints");
        assert_eq!(constraints.len(), 1);
        assert_eq!(constraints[0].id, constraint.id);

        {
            let mut guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_mut().expect("conn");
            conn.execute(
                "INSERT INTO transactions (book_id, occurred_date, status, created_at)
                 VALUES (1, '2024-02-02', 'cleared', strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert tx");
            let tx_id = conn.last_insert_rowid();

            // balancing account to keep splits zero-sum
            conn.execute(
                "INSERT INTO accounts (book_id, type, name, commodity_id)
                 VALUES (1, 'income', 'Offset Account', ?1)",
                params![commodity_id],
            )
            .expect("insert offset account");
            let offset_id = conn.last_insert_rowid();

            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -500, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx_id, account.id, commodity_id],
            )
            .expect("insert negative split");
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, 500, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![tx_id, offset_id, commodity_id],
            )
            .expect("insert offset split");
        }

        let err = validate_balance_constraints(as_state(&db_state), account.id)
            .expect_err("constraint should fail on negative balance");
        assert!(err.to_string().contains("constraint"));

        let opened = create_account_open(
            as_state(&db_state),
            AccountDirectiveCreate {
                book_id: 1,
                account_id: account.id,
                directive_date: "2024-01-01".to_string(),
                note: Some("open".to_string()),
                metadata: None,
            },
        )
        .expect("create open directive");
        assert_eq!(opened.directive_type, "open");

        let closed = create_account_close(
            as_state(&db_state),
            AccountDirectiveCreate {
                book_id: 1,
                account_id: account.id,
                directive_date: "2024-12-31".to_string(),
                note: Some("close".to_string()),
                metadata: None,
            },
        )
        .expect("create close directive");
        assert_eq!(closed.directive_type, "close");

        let reopened = create_account_reopen(
            as_state(&db_state),
            AccountDirectiveCreate {
                book_id: 1,
                account_id: account.id,
                directive_date: "2025-01-01".to_string(),
                note: Some("reopen".to_string()),
                metadata: None,
            },
        )
        .expect("create reopen directive");
        assert_eq!(reopened.directive_type, "reopen");

        let directives = list_account_directives(as_state(&db_state), account.id)
            .expect("list directives");
        assert_eq!(directives.len(), 4);

        let check = create_balance_check(
            as_state(&db_state),
            BalanceCheckCreate {
                book_id: 1,
                account_id: account.id,
                as_of_date: "2024-02-02".to_string(),
                balance_minor: -500,
                memo: Some("check".to_string()),
                create_balancing_tx: Some(false),
                offset_account_id: None,
            },
        )
        .expect("create balance check");
        assert_eq!(check.balance_minor, -500);

        let checks = list_balance_checks(as_state(&db_state), account.id)
            .expect("list balance checks");
        assert_eq!(checks.len(), 1);

        let pad = create_balance_adjustment(
            as_state(&db_state),
            BalanceAdjustmentCreate {
                book_id: 1,
                balance_check_id: Some(check.id),
                account_id: account.id,
                offset_account_id: account.id,
                as_of_date: "2024-02-03".to_string(),
                target_balance_minor: 0,
                memo: Some("pad".to_string()),
            },
        )
        .expect("create balance adjustment");
        assert_eq!(pad.target_balance_minor, 0);

        let pads = list_balance_adjustments(as_state(&db_state), account.id)
            .expect("list balance adjustments");
        assert_eq!(pads.len(), 1);

        let deleted = delete_balance_constraint(as_state(&db_state), constraint.id)
            .expect("delete constraint");
        assert!(deleted);
    }

    #[test]
    fn test_notes_events_documents_crud() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "checking".to_string(),
                name: "Notes Account".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create account");

        let tx_id = {
            let mut guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_mut().expect("conn");
            conn.execute(
                "INSERT INTO transactions (book_id, occurred_date, status, created_at)
                 VALUES (1, '2024-05-01', 'cleared', strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert tx");
            conn.last_insert_rowid()
        };

        let note = create_note(
            as_state(&db_state),
            NoteCreate {
                book_id: 1,
                account_id: Some(account.id),
                tx_id: Some(tx_id),
                note: "note text".to_string(),
                note_date: Some("2024-05-01".to_string()),
            },
        )
        .expect("create note");

        let notes = list_notes(
            as_state(&db_state),
            NoteFilter {
                account_id: Some(account.id),
                tx_id: Some(tx_id),
            },
        )
        .expect("list notes");
        assert_eq!(notes.len(), 1);

        let fetched = get_note(as_state(&db_state), note.id)
            .expect("get note")
            .expect("note exists");
        assert_eq!(fetched.note, "note text");

        let updated = update_note(
            as_state(&db_state),
            NoteUpdate {
                id: note.id,
                book_id: 1,
                account_id: Some(account.id),
                tx_id: Some(tx_id),
                note: "note updated".to_string(),
                note_date: Some("2024-05-02".to_string()),
            },
        )
        .expect("update note");
        assert_eq!(updated.note, "note updated");

        let deleted = delete_note(as_state(&db_state), note.id).expect("delete note");
        assert!(deleted);

        let event = create_event(
            as_state(&db_state),
            EventCreate {
                book_id: 1,
                account_id: Some(account.id),
                tx_id: Some(tx_id),
                event_type: "audit".to_string(),
                event_date: "2024-05-01".to_string(),
                description: Some("event".to_string()),
                metadata: None,
            },
        )
        .expect("create event");

        let events = list_events(
            as_state(&db_state),
            EventFilter {
                account_id: Some(account.id),
                tx_id: Some(tx_id),
            },
        )
        .expect("list events");
        assert_eq!(events.len(), 1);

        let updated_event = update_event(
            as_state(&db_state),
            EventUpdate {
                id: event.id,
                book_id: 1,
                account_id: Some(account.id),
                tx_id: Some(tx_id),
                event_type: "audit".to_string(),
                event_date: "2024-05-02".to_string(),
                description: Some("event updated".to_string()),
                metadata: Some("{}".to_string()),
            },
        )
        .expect("update event");
        assert_eq!(updated_event.description.as_deref(), Some("event updated"));

        let deleted_event = delete_event(as_state(&db_state), event.id).expect("delete event");
        assert!(deleted_event);

        let doc = create_document(
            as_state(&db_state),
            DocumentCreate {
                book_id: 1,
                account_id: Some(account.id),
                tx_id: Some(tx_id),
                doc_type: Some("statement".to_string()),
                title: Some("Statement".to_string()),
                uri: Some("file:///tmp/statement.pdf".to_string()),
                mime_type: Some("application/pdf".to_string()),
                notes: Some("doc notes".to_string()),
            },
        )
        .expect("create document");

        let docs = list_documents(
            as_state(&db_state),
            DocumentFilter {
                account_id: Some(account.id),
                tx_id: Some(tx_id),
            },
        )
        .expect("list documents");
        assert_eq!(docs.len(), 1);

        let fetched_doc = get_document(as_state(&db_state), doc.id)
            .expect("get document")
            .expect("doc exists");
        assert_eq!(fetched_doc.title.as_deref(), Some("Statement"));

        let updated_doc = update_document(
            as_state(&db_state),
            DocumentUpdate {
                id: doc.id,
                book_id: 1,
                account_id: Some(account.id),
                tx_id: Some(tx_id),
                doc_type: Some("statement".to_string()),
                title: Some("Statement Updated".to_string()),
                uri: Some("file:///tmp/statement-v2.pdf".to_string()),
                mime_type: Some("application/pdf".to_string()),
                notes: Some("doc notes updated".to_string()),
            },
        )
        .expect("update document");
        assert_eq!(updated_doc.title.as_deref(), Some("Statement Updated"));

        let deleted_doc = delete_document(as_state(&db_state), doc.id).expect("delete document");
        assert!(deleted_doc);
    }

    #[test]
    fn test_close_fiscal_year_excludes_future_void_and_reverted_transactions() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let income_account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "income".to_string(),
                name: "Sales Income".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create income account");

        let offset_equity = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "equity".to_string(),
                name: "Opening Equity".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create offset equity account");

        {
            let mut guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_mut().expect("conn");

            let insert_tx = |conn: &rusqlite::Connection, occurred_date: &str, status: &str| -> i64 {
                conn.execute(
                    "INSERT INTO transactions (book_id, occurred_date, status, created_at)
                     VALUES (1, ?1, ?2, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![occurred_date, status],
                )
                .expect("insert tx");
                conn.last_insert_rowid()
            };

            let insert_balanced_pair = |conn: &rusqlite::Connection, tx_id: i64, amount_minor: i64| {
                conn.execute(
                    "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                     VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![tx_id, income_account.id, commodity_id, amount_minor],
                )
                .expect("insert income split");
                conn.execute(
                    "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                     VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                    params![tx_id, offset_equity.id, commodity_id, -amount_minor],
                )
                .expect("insert offset split");
            };

            let included_tx = insert_tx(conn, "2024-12-31", "cleared");
            insert_balanced_pair(conn, included_tx, 1000);

            let future_tx = insert_tx(conn, "2025-01-01", "cleared");
            insert_balanced_pair(conn, future_tx, 2000);

            let void_tx = insert_tx(conn, "2024-12-31", "void");
            insert_balanced_pair(conn, void_tx, 3000);

            let reverted_tx = insert_tx(conn, "2024-12-31", "cleared");
            insert_balanced_pair(conn, reverted_tx, 4000);

            let session_id = current_session_id(conn).expect("current session id");
            conn.execute(
                "INSERT INTO session_reverts (session_id, table_name, row_id)
                 VALUES (?1, 'transactions', ?2)",
                params![session_id, reverted_tx],
            )
            .expect("mark tx as reverted in current session");
        }

        let close_result = close_fiscal_year(
            as_state(&db_state),
            FiscalYearCloseInput {
                close_date: "2024-12-31".to_string(),
                memo: Some("Year End Close".to_string()),
            },
        )
        .expect("close fiscal year");

        assert_eq!(close_result.closed_accounts_count, 1);
        assert_eq!(close_result.retained_earnings_delta_minor, 1000);

        let system_accounts = ensure_profit_loss_system_accounts(as_state(&db_state), 1)
            .expect("ensure system accounts");

        let close_tx_id = close_result.tx_id.expect("close tx id");
        let retained_earnings_split_minor = {
            let guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_ref().expect("conn");
            conn.query_row(
                "SELECT amount_minor
                 FROM splits
                 WHERE tx_id = ?1 AND account_id = ?2",
                params![close_tx_id, system_accounts.retained_earnings_account_id],
                |row| row.get::<_, i64>(0),
            )
            .expect("retained earnings split")
        };
        assert_eq!(retained_earnings_split_minor, 1000);
    }

    #[test]
    fn test_close_fiscal_year_same_date_twice_returns_already_closed_error() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let income_account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "income".to_string(),
                name: "Recurring Income".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create income account");

        let offset_equity = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "equity".to_string(),
                name: "Offset Equity".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create offset equity account");

        {
            let mut guard = db_state.inner.lock().expect("lock db");
            let conn = guard.conn.as_mut().expect("conn");
            conn.execute(
                "INSERT INTO transactions (book_id, occurred_date, status, created_at)
                 VALUES (1, '2024-12-31', 'cleared', strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [],
            )
            .expect("insert source tx");
            let source_tx_id = conn.last_insert_rowid();

            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, 1500, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![source_tx_id, income_account.id, commodity_id],
            )
            .expect("insert income split");
            conn.execute(
                "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
                 VALUES (?1, ?2, ?3, -1500, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                params![source_tx_id, offset_equity.id, commodity_id],
            )
            .expect("insert offset split");
        }

        let first_close = close_fiscal_year(
            as_state(&db_state),
            FiscalYearCloseInput {
                close_date: "2024-12-31".to_string(),
                memo: Some("Year End Close".to_string()),
            },
        )
        .expect("first close should succeed");
        assert!(first_close.tx_id.is_some());

        let second_close_err = close_fiscal_year(
            as_state(&db_state),
            FiscalYearCloseInput {
                close_date: "2024-12-31".to_string(),
                memo: Some("Year End Close Retry".to_string()),
            },
        )
        .expect_err("second close on same date should fail");

        assert!(
            second_close_err.to_string().contains("fiscal year already closed for close_date"),
            "unexpected error: {second_close_err}"
        );
    }

    // ── Balance filter tests ───────────────────────────────────────────────────
    // Verify that get_account_balance_minor (and, transitively, all balance
    // queries that call it) excludes voided and superseded transactions.

    fn insert_raw_tx(conn: &rusqlite::Connection, book_id: i64, status: &str) -> i64 {
        conn.execute(
            "INSERT INTO transactions (book_id, occurred_date, status, created_at)
             VALUES (?1, '2024-01-15', ?2, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![book_id, status],
        )
        .expect("insert raw tx");
        conn.last_insert_rowid()
    }

    fn insert_raw_split(conn: &rusqlite::Connection, tx_id: i64, account_id: i64, commodity_id: i64, amount: i64) {
        conn.execute(
            "INSERT INTO splits (tx_id, account_id, commodity_id, amount_minor, created_at)
             VALUES (?1, ?2, ?3, ?4, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
            params![tx_id, account_id, commodity_id, amount],
        )
        .expect("insert raw split");
    }

    #[test]
    fn test_balance_excludes_voided_transactions() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "checking".to_string(),
                name: "Balance Void Test".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create account");

        {
            let mut guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_mut().expect("conn");

            // Cleared transaction — should be included in balance.
            let tx_cleared = insert_raw_tx(conn, 1, "cleared");
            insert_raw_split(conn, tx_cleared, account.id, commodity_id, 1000);

            // Voided transaction — must NOT be included in balance.
            let tx_void = insert_raw_tx(conn, 1, "void");
            insert_raw_split(conn, tx_void, account.id, commodity_id, 500);

            let balance = get_account_balance_minor(conn, account.id)
                .expect("get balance");
            assert_eq!(
                balance, 1000,
                "voided transaction (500) must not be counted; expected 1000 got {balance}"
            );
        }
    }

    #[test]
    fn test_balance_excludes_superseded_transactions() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let account = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "checking".to_string(),
                name: "Balance Supersede Test".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        )
        .expect("create account");

        {
            let mut guard = db_state.inner.lock().expect("lock");
            let conn = guard.conn.as_mut().expect("conn");

            // Original transaction: 1000.
            let tx_original = insert_raw_tx(conn, 1, "cleared");
            insert_raw_split(conn, tx_original, account.id, commodity_id, 1000);

            // Revised (superseding) transaction: 2000.
            // Inserting a new transaction pointing to tx_original via previous_tx_id
            // makes tx_original "superseded" (another tx has previous_tx_id = tx_original).
            conn.execute(
                "INSERT INTO transactions (book_id, occurred_date, status, previous_tx_id, created_at)
                 VALUES (1, '2024-01-15', 'cleared', ?1, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
                [tx_original],
            )
            .expect("insert revised tx");
            let tx_revised = conn.last_insert_rowid();
            insert_raw_split(conn, tx_revised, account.id, commodity_id, 2000);

            let balance = get_account_balance_minor(conn, account.id)
                .expect("get balance");
            assert_eq!(
                balance, 2000,
                "superseded original (1000) must not be counted; expected 2000 got {balance}"
            );
        }
    }

    #[test]
    fn test_validate_account_type_rejects_typo() {
        let err = crate::validation::validate_account_type("epxense");
        assert!(err.is_err(), "typo 'epxense' should be rejected");
    }

    #[test]
    fn test_create_account_rejects_invalid_type() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let result = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "bank_account".to_string(), // invalid
                name: "Bad Account".to_string(),
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        );
        assert!(result.is_err(), "invalid account_type should be rejected");
        let msg = result.unwrap_err();
        assert!(msg.to_string().contains("invalid account_type"), "error should mention account_type, got: {msg}");
    }

    #[test]
    fn test_create_account_rejects_blank_name() {
        let db_state = create_temp_db();
        let commodity_id = get_usd_commodity_id(&db_state);

        let result = create_account(
            as_state(&db_state),
            AccountCreate {
                book_id: 1,
                parent_id: None,
                account_type: "checking".to_string(),
                name: "   ".to_string(), // blank
                commodity_id,
                institution_id: None,
                country_id: None,
                number_last4: None,
                is_closed: None,
            },
        );
        assert!(result.is_err(), "blank name should be rejected");
    }
}

#[command]
pub fn list_account_directives(
    db: State<DbState>,
    account_id: i64,
) -> Result<Vec<AccountDirective>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_account_id
                 FROM accounts a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_account_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM accounts a
                 JOIN chain c ON a.previous_account_id = c.id
             )
             SELECT a.id, a.book_id, ?2 as account_id, a.lifecycle_event, a.effective_at,
                    a.lifecycle_note, a.lifecycle_metadata, a.created_at
             FROM accounts a
             WHERE a.id IN chain
               AND a.lifecycle_event IN ('open', 'close', 'reopen')
             ORDER BY a.effective_at DESC, a.id DESC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map(params![account_id, account_id], map_account_directive_row)
        .map_err(|e| e.to_string())?;

    let mut directives = Vec::new();
    for row in rows {
        directives.push(row.map_err(|e| e.to_string())?);
    }

    Ok(directives)
}

#[command]
pub fn create_balance_check(
    db: State<DbState>,
    input: BalanceCheckCreate,
) -> Result<BalanceCheck, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let actual_balance_minor = get_account_balance_minor(conn, input.account_id)?;
    let mut status = if actual_balance_minor == input.balance_minor {
        "matched".to_string()
    } else {
        "unbalanced".to_string()
    };

    conn.execute(
        "INSERT INTO balance_checks (book_id, account_id, as_of_date, balance_minor, memo, status, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.account_id,
            input.as_of_date,
            input.balance_minor,
            input.memo,
            status,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    if input.create_balancing_tx.unwrap_or(false) {
        let offset_account_id = input
            .offset_account_id
            .ok_or_else(|| "offset_account_id is required when create_balancing_tx is true".to_string())?;

        let maybe_adjustment = create_marked_balance_adjustment(
            conn,
            Some(id),
            input.account_id,
            offset_account_id,
            &input.as_of_date,
            input.balance_minor,
            input.memo.as_deref(),
        )?;

        if maybe_adjustment.is_some() {
            status = "balanced".to_string();
            conn.execute(
                "UPDATE balance_checks SET status = ?2 WHERE id = ?1",
                params![id, status],
            )
            .map_err(|e| e.to_string())?;
        }
    }

    let check = conn
        .query_row(
            "SELECT id, book_id, account_id, as_of_date, balance_minor, memo, status, created_at
             FROM balance_checks WHERE id = ?1",
            [id],
            map_balance_check_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(check)
}

#[command]
pub fn list_balance_checks(
    db: State<DbState>,
    account_id: i64,
) -> Result<Vec<BalanceCheck>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, as_of_date, balance_minor, memo, status, created_at
             FROM balance_checks WHERE account_id = ?1
             ORDER BY as_of_date DESC, id DESC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([account_id], map_balance_check_row)
        .map_err(|e| e.to_string())?;

    let mut checks = Vec::new();
    for row in rows {
        checks.push(row.map_err(|e| e.to_string())?);
    }

    Ok(checks)
}

#[command]
pub fn create_balance_adjustment(
    db: State<DbState>,
    input: BalanceAdjustmentCreate,
) -> Result<BalanceAdjustment, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let adjustment = create_marked_balance_adjustment(
        conn,
        input.balance_check_id,
        input.account_id,
        input.offset_account_id,
        &input.as_of_date,
        input.target_balance_minor,
        input.memo.as_deref(),
    )?
    .ok_or_else(|| "no adjustment transaction required: account already at target balance".to_string())?;

    Ok(adjustment)
}

#[command]
pub fn list_balance_adjustments(
    db: State<DbState>,
    account_id: i64,
) -> Result<Vec<BalanceAdjustment>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, balance_check_id, account_id, offset_account_id, as_of_date,
                    target_balance_minor, actual_balance_minor, adjustment_minor, tx_id, mark, memo, created_at
             FROM balance_adjustments WHERE account_id = ?1
             ORDER BY as_of_date DESC, id DESC",
        )
        .map_err(|e| e.to_string())?;

    let rows = stmt
        .query_map([account_id], map_balance_adjustment_row)
        .map_err(|e| e.to_string())?;

    let mut directives = Vec::new();
    for row in rows {
        directives.push(row.map_err(|e| e.to_string())?);
    }

    Ok(directives)
}

#[command]
pub fn create_note(db: State<DbState>, input: NoteCreate) -> Result<Note, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO notes (book_id, account_id, tx_id, note, note_date, previous_note_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, NULL, ?6, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![book_id, input.account_id, input.tx_id, input.note, input.note_date, session_id],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "notes", id)?;
    let note = conn
        .query_row(
            "SELECT id, book_id, account_id, tx_id, note, note_date, created_at
             FROM notes WHERE id = ?1",
            [id],
            map_note_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(note)
}

#[command]
pub fn list_notes(db: State<DbState>, filter: NoteFilter) -> Result<Vec<Note>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut sql = String::from(
        "SELECT id, book_id, account_id, tx_id, note, note_date, created_at
                 FROM notes
                 WHERE book_id = ?
                    AND (notes.session_id IS NULL OR notes.session_id NOT LIKE 'void:%')
                     AND NOT EXISTS (
                             SELECT 1 FROM notes newer
                             WHERE newer.previous_note_id = notes.id
                     )
                     AND NOT EXISTS (
                             SELECT 1 FROM session_reverts sr
                             WHERE sr.table_name = 'notes'
                                 AND sr.row_id = notes.id
                                 AND sr.session_id = (SELECT id FROM app_runtime_session LIMIT 1)
                     )",
    );
    let mut params: Vec<Value> = vec![Value::from(SINGLE_BOOK_ID)];

    if let Some(account_id) = filter.account_id {
        sql.push_str(" AND account_id = ?");
        params.push(Value::from(account_id));
    }
    if let Some(tx_id) = filter.tx_id {
        sql.push_str(" AND tx_id = ?");
        params.push(Value::from(tx_id));
    }

    sql.push_str(" ORDER BY note_date DESC, id DESC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), map_note_row)
        .map_err(|e| e.to_string())?;

    let mut notes = Vec::new();
    for row in rows {
        notes.push(row.map_err(|e| e.to_string())?);
    }

    Ok(notes)
}

#[command]
pub fn get_note(db: State<DbState>, id: i64) -> Result<Option<Note>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let note = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_note_id
                 FROM notes a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_note_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM notes a
                 JOIN chain c ON a.previous_note_id = c.id
             )
             SELECT id, book_id, account_id, tx_id, note, note_date, created_at
             FROM notes
             WHERE id IN chain
                             AND (notes.session_id IS NULL OR notes.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM notes newer
                   WHERE newer.previous_note_id = notes.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            map_note_row,
        )
        .optional()
        .map_err(|e| e.to_string())?;
    Ok(note)
}

#[command]
pub fn update_note(db: State<DbState>, input: NoteUpdate) -> Result<Note, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let book_id = SINGLE_BOOK_ID;

    let exists: Option<i64> = conn
        .query_row("SELECT id FROM notes WHERE id = ?1", [input.id], |row| row.get(0))
        .optional()
        .map_err(|e| e.to_string())?;
    if exists.is_none() {
        return Err(AppError::not_found("note", "requested"));
    }

    conn.execute(
        "INSERT INTO notes (book_id, account_id, tx_id, note, note_date, previous_note_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.account_id,
            input.tx_id,
            input.note,
            input.note_date,
            input.id,
            session_id,
        ],
    )
    .map_err(|e| e.to_string())?;

    let new_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "notes", new_id)?;

    let note = conn
        .query_row(
            "SELECT id, book_id, account_id, tx_id, note, note_date, created_at
             FROM notes WHERE id = ?1",
            [new_id],
            map_note_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(note)
}

#[command]
pub fn delete_note(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;
    let session_id = current_session_id(conn)?;

    let source: Option<(i64, i64, Option<i64>, Option<i64>, String, Option<String>)> = conn
        .query_row(
            "WITH RECURSIVE chain(id) AS (
                 SELECT ?1
                 UNION
                 SELECT a.previous_note_id
                 FROM notes a
                 JOIN chain c ON a.id = c.id
                 WHERE a.previous_note_id IS NOT NULL
                 UNION
                 SELECT a.id
                 FROM notes a
                 JOIN chain c ON a.previous_note_id = c.id
             )
             SELECT id, book_id, account_id, tx_id, note, note_date
             FROM notes
             WHERE id IN chain
               AND (notes.session_id IS NULL OR notes.session_id NOT LIKE 'void:%')
               AND NOT EXISTS (
                   SELECT 1 FROM notes newer
                   WHERE newer.previous_note_id = notes.id
                     AND newer.id IN chain
               )
             LIMIT 1",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?, row.get(4)?, row.get(5)?)),
        )
        .optional()
        .map_err(|e| e.to_string())?;

    let Some((current_id, book_id, account_id, tx_id, note, note_date)) = source else {
        return Ok(false);
    };

    conn.execute(
        "INSERT INTO notes (book_id, account_id, tx_id, note, note_date, previous_note_id, session_id, created_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            account_id,
            tx_id,
            note,
            note_date,
            current_id,
            format!("{}{}", VOID_SESSION_PREFIX, session_id),
        ],
    )
    .map_err(|e| e.to_string())?;

    let tombstone_id = conn.last_insert_rowid();
    clear_redo_stack(conn, &session_id)?;
    record_insert_change(conn, &session_id, "notes", tombstone_id)?;
    Ok(true)
}

#[command]
pub fn create_event(db: State<DbState>, input: EventCreate) -> Result<Event, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO events (book_id, account_id, tx_id, event_type, event_date, description, metadata, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.account_id,
            input.tx_id,
            input.event_type,
            input.event_date,
            input.description,
            input.metadata,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let event = conn
        .query_row(
            "SELECT id, book_id, account_id, tx_id, event_type, event_date, description, metadata, created_at, updated_at
             FROM events WHERE id = ?1",
            [id],
            map_event_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(event)
}

#[command]
pub fn list_events(db: State<DbState>, filter: EventFilter) -> Result<Vec<Event>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut sql = String::from(
        "SELECT id, book_id, account_id, tx_id, event_type, event_date, description, metadata, created_at, updated_at
         FROM events WHERE book_id = ?",
    );
    let mut params: Vec<Value> = vec![Value::from(SINGLE_BOOK_ID)];

    if let Some(account_id) = filter.account_id {
        sql.push_str(" AND account_id = ?");
        params.push(Value::from(account_id));
    }
    if let Some(tx_id) = filter.tx_id {
        sql.push_str(" AND tx_id = ?");
        params.push(Value::from(tx_id));
    }

    sql.push_str(" ORDER BY event_date DESC, id DESC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), map_event_row)
        .map_err(|e| e.to_string())?;

    let mut events = Vec::new();
    for row in rows {
        events.push(row.map_err(|e| e.to_string())?);
    }

    Ok(events)
}

#[command]
pub fn get_event(db: State<DbState>, id: i64) -> Result<Option<Event>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, tx_id, event_type, event_date, description, metadata, created_at, updated_at
             FROM events WHERE id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let mut rows = stmt.query([id]).map_err(|e| e.to_string())?;
    if let Some(row) = rows.next().map_err(|e| e.to_string())? {
        Ok(Some(map_event_row(row).map_err(|e| e.to_string())?))
    } else {
        Ok(None)
    }
}

#[command]
pub fn update_event(db: State<DbState>, input: EventUpdate) -> Result<Event, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let rows = conn
        .execute(
            "UPDATE events
             SET book_id = ?2,
                 account_id = ?3,
                 tx_id = ?4,
                 event_type = ?5,
                 event_date = ?6,
                 description = ?7,
                 metadata = ?8,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![
                input.id,
                book_id,
                input.account_id,
                input.tx_id,
                input.event_type,
                input.event_date,
                input.description,
                input.metadata,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err(AppError::not_found("event", "requested"));
    }

    let event = conn
        .query_row(
            "SELECT id, book_id, account_id, tx_id, event_type, event_date, description, metadata, created_at, updated_at
             FROM events WHERE id = ?1",
            [input.id],
            map_event_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(event)
}

#[command]
pub fn delete_event(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute("DELETE FROM events WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(rows > 0)
}

#[command]
pub fn create_document(db: State<DbState>, input: DocumentCreate) -> Result<Document, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    conn.execute(
        "INSERT INTO documents (book_id, account_id, tx_id, doc_type, title, uri, mime_type, notes, created_at, updated_at)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'))",
        params![
            book_id,
            input.account_id,
            input.tx_id,
            input.doc_type,
            input.title,
            input.uri,
            input.mime_type,
            input.notes,
        ],
    )
    .map_err(|e| e.to_string())?;

    let id = conn.last_insert_rowid();
    let doc = conn
        .query_row(
            "SELECT id, book_id, account_id, tx_id, doc_type, title, uri, mime_type, notes, created_at, updated_at
             FROM documents WHERE id = ?1",
            [id],
            map_document_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(doc)
}

#[command]
pub fn list_documents(db: State<DbState>, filter: DocumentFilter) -> Result<Vec<Document>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut sql = String::from(
        "SELECT id, book_id, account_id, tx_id, doc_type, title, uri, mime_type, notes, created_at, updated_at
         FROM documents WHERE book_id = ?",
    );
    let mut params: Vec<Value> = vec![Value::from(SINGLE_BOOK_ID)];

    if let Some(account_id) = filter.account_id {
        sql.push_str(" AND account_id = ?");
        params.push(Value::from(account_id));
    }
    if let Some(tx_id) = filter.tx_id {
        sql.push_str(" AND tx_id = ?");
        params.push(Value::from(tx_id));
    }

    sql.push_str(" ORDER BY id DESC");

    let mut stmt = conn.prepare(&sql).map_err(|e| e.to_string())?;
    let rows = stmt
        .query_map(params_from_iter(params), map_document_row)
        .map_err(|e| e.to_string())?;

    let mut docs = Vec::new();
    for row in rows {
        docs.push(row.map_err(|e| e.to_string())?);
    }

    Ok(docs)
}

#[command]
pub fn get_document(db: State<DbState>, id: i64) -> Result<Option<Document>, AppError> {
    let guard = db.read_conn.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.as_ref().ok_or_else(|| "db not initialized".to_string())?;

    let mut stmt = conn
        .prepare(
            "SELECT id, book_id, account_id, tx_id, doc_type, title, uri, mime_type, notes, created_at, updated_at
             FROM documents WHERE id = ?1",
        )
        .map_err(|e| e.to_string())?;

    let mut rows = stmt.query([id]).map_err(|e| e.to_string())?;
    if let Some(row) = rows.next().map_err(|e| e.to_string())? {
        Ok(Some(map_document_row(row).map_err(|e| e.to_string())?))
    } else {
        Ok(None)
    }
}

#[command]
pub fn update_document(db: State<DbState>, input: DocumentUpdate) -> Result<Document, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let book_id = SINGLE_BOOK_ID;

    let rows = conn
        .execute(
            "UPDATE documents
             SET book_id = ?2,
                 account_id = ?3,
                 tx_id = ?4,
                 doc_type = ?5,
                 title = ?6,
                 uri = ?7,
                 mime_type = ?8,
                 notes = ?9,
                 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
             WHERE id = ?1",
            params![
                input.id,
                book_id,
                input.account_id,
                input.tx_id,
                input.doc_type,
                input.title,
                input.uri,
                input.mime_type,
                input.notes,
            ],
        )
        .map_err(|e| e.to_string())?;

    if rows == 0 {
        return Err(AppError::not_found("document", "requested"));
    }

    let doc = conn
        .query_row(
            "SELECT id, book_id, account_id, tx_id, doc_type, title, uri, mime_type, notes, created_at, updated_at
             FROM documents WHERE id = ?1",
            [input.id],
            map_document_row,
        )
        .map_err(|e| e.to_string())?;

    Ok(doc)
}

#[command]
pub fn delete_document(db: State<DbState>, id: i64) -> Result<bool, AppError> {
    let mut guard = db.inner.lock().map_err(|_| "db state lock poisoned".to_string())?;
    let conn = guard.conn.as_mut().ok_or_else(|| "db not initialized".to_string())?;

    let rows = conn
        .execute("DELETE FROM documents WHERE id = ?1", [id])
        .map_err(|e| e.to_string())?;

    Ok(rows > 0)
}

use serde::Serialize;
use sqlx::{FromRow, PgPool};

#[derive(Serialize, FromRow)]
pub struct AccountSummary {
    pub id: i64,
    pub book_id: i64,
    pub parent_id: Option<i64>,
    pub account_type: String,
    pub name: String,
    pub currency_code: String,
    pub is_closed: bool,
    pub is_hidden: bool,
}

pub async fn list_accounts(pool: &PgPool) -> Result<Vec<AccountSummary>, sqlx::Error> {
    sqlx::query_as::<_, AccountSummary>(
        "SELECT id, book_id, parent_id, account_type, name, currency_code, is_closed, is_hidden
         FROM accounts
         WHERE is_hidden = FALSE
         ORDER BY book_id, name",
    )
    .fetch_all(pool)
    .await
}
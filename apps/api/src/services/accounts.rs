use sqlx::PgPool;

use crate::accounts::{self, AccountSummary};

pub async fn list_accounts(pool: &PgPool) -> Result<Vec<AccountSummary>, sqlx::Error> {
    accounts::list_accounts(pool).await
}
use serde::Serialize;
use sqlx::{FromRow, PgPool};

#[derive(Serialize, FromRow)]
pub struct BookSummary {
    pub id: i64,
    pub slug: String,
    pub name: String,
    pub base_currency_code: String,
}

pub async fn list_books(pool: &PgPool) -> Result<Vec<BookSummary>, sqlx::Error> {
    sqlx::query_as::<_, BookSummary>(
        "SELECT id, slug, name, base_currency_code FROM books ORDER BY id",
    )
    .fetch_all(pool)
    .await
}

pub async fn get_book_by_slug(pool: &PgPool, slug: &str) -> Result<Option<BookSummary>, sqlx::Error> {
    sqlx::query_as::<_, BookSummary>(
        "SELECT id, slug, name, base_currency_code FROM books WHERE slug = $1",
    )
    .bind(slug)
    .fetch_optional(pool)
    .await
}
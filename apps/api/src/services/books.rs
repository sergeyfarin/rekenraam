use sqlx::PgPool;

use crate::books::{self, BookSummary};

pub async fn list_books(pool: &PgPool) -> Result<Vec<BookSummary>, sqlx::Error> {
    books::list_books(pool).await
}

pub async fn get_book_by_slug(pool: &PgPool, slug: &str) -> Result<Option<BookSummary>, sqlx::Error> {
    books::get_book_by_slug(pool, slug).await
}
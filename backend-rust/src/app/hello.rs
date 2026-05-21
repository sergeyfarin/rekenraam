use sqlx::SqlitePool;

pub async fn message(db: &SqlitePool) -> Result<String, sqlx::Error> {
    let value = sqlx::query_scalar::<_, String>("SELECT value FROM app_info WHERE key = ?")
        .bind("hello")
        .fetch_one(db)
        .await?;

    Ok(value)
}

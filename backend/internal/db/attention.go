package db

import (
	"context"
	"database/sql"
	"fmt"
)

type AttentionRepository struct{ database *sql.DB }

func NewAttentionRepository(database *sql.DB) *AttentionRepository {
	return &AttentionRepository{database: database}
}

type AttentionRecord struct {
	DividendReviewCount *int
	Count               int
	Status              string
}

// Attention reads are independent: a broken source must not erase the others.
// Each query counts identities directly, never a capped first page or joined
// posting rows. Latest-run sources deliberately retire failures after recovery.
func (r *AttentionRepository) Read(ctx context.Context, bookID int64, source string) (AttentionRecord, error) {
	if source == "imports" {
		// Batch identities still count once in the header; the view's row count is
		// separate and uses exactly the review queue's membership, including repair.
		var record AttentionRecord
		var dividends int
		err := r.database.QueryRowContext(ctx, `WITH dividend_reviews AS (SELECT s.id,s.batch_id `+importDividendReviewFromSQL+`)
 SELECT count(*),CASE WHEN count(CASE WHEN b.status='failed' THEN 1 END)>0 THEN 'failed' ELSE '' END,
 (SELECT count(*) FROM dividend_reviews)
 FROM import_batches b WHERE b.book_id=? AND (
 (b.status IN ('previewing','partially_committed','failed') AND (b.status='failed' OR EXISTS (
 SELECT 1 FROM import_staged_rows s WHERE s.batch_id=b.id AND s.book_id=b.book_id AND s.commit_status IN ('pending','failed') AND s.dedupe_status NOT IN ('duplicate','excluded'))))
 OR EXISTS (SELECT 1 FROM dividend_reviews d WHERE d.batch_id=b.id))`, bookID, 0, 0, 0, 0, bookID).Scan(&record.Count, &record.Status, &dividends)
		if err != nil {
			return AttentionRecord{}, fmt.Errorf("read import attention: %w", err)
		}
		record.DividendReviewCount = &dividends
		return record, nil
	}
	queries := map[string]string{
		"drafts":             `SELECT count(*), '' FROM transactions t JOIN current_transaction_versions v ON v.transaction_id=t.id WHERE t.book_id=:book_id AND t.deleted_at IS NULL AND v.status='draft'`,
		"transaction_review": `SELECT count(*), '' FROM transactions t JOIN current_transaction_versions v ON v.transaction_id=t.id WHERE t.book_id=:book_id AND t.deleted_at IS NULL AND v.status='posted' AND v.needs_review=1`,
		"recurring_blocked":  `SELECT count(*), '' FROM recurring_occurrences WHERE book_id=:book_id AND status='blocked'`,
		"suggestions":        `SELECT count(DISTINCT provider_event_id), CASE WHEN count(CASE WHEN status='failed' THEN 1 END)>0 THEN 'failed' ELSE '' END FROM investment_event_suggestions WHERE book_id=:book_id AND status IN ('suggested','failed')`,
		"connections":        `SELECT count(CASE WHEN last_fetch_status='failed' THEN 1 END), CASE WHEN count(CASE WHEN last_fetch_status='fetching' THEN 1 END)>0 THEN 'running' ELSE '' END FROM import_connections WHERE book_id=:book_id`,
		"refresh": `WITH latest_runs AS (
 SELECT source_id, status FROM market_data_ingest_runs
 WHERE id IN (SELECT max(id) FROM market_data_ingest_runs WHERE book_id=:book_id GROUP BY source_id)
 ) SELECT count(*), CASE WHEN EXISTS(SELECT 1 FROM latest_runs WHERE status='running') THEN 'running' ELSE '' END FROM (
 SELECT source_id FROM pricing_refresh_state WHERE book_id=:book_id AND coalesce(last_error,'') <> ''
 UNION SELECT source_id FROM latest_runs WHERE status IN ('failed','partial')
 )`,
		"backups":    `SELECT 0, status FROM backup_runs WHERE book_id=:book_id ORDER BY id DESC LIMIT 1`,
		"self_check": `SELECT 0, status FROM self_check_runs WHERE book_id=:book_id ORDER BY id DESC LIMIT 1`,
	}
	query, ok := queries[source]
	if !ok {
		return AttentionRecord{}, fmt.Errorf("unknown attention source %q", source)
	}
	var record AttentionRecord
	err := r.database.QueryRowContext(ctx, query, sql.Named("book_id", bookID)).Scan(&record.Count, &record.Status)
	if err == sql.ErrNoRows {
		return AttentionRecord{Status: "never"}, nil
	}
	if err != nil {
		return AttentionRecord{}, fmt.Errorf("read attention source %s: %w", source, err)
	}
	return record, nil
}

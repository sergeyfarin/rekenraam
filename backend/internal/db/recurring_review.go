package db

import (
	"context"
	"database/sql"
	"fmt"
	"rekenraam/backend/internal/exact"
)

type RecurringDuePosting struct {
	CommodityID   int64
	CommodityCode string
	QuantityValue exact.Coefficient
	QuantityScale int
}

type RecurringDueRecord struct {
	RecurringOccurrenceRecord
	TemplateName     string
	TemplateEnabled  bool
	TemplateArchived bool
	Description      string
	PayeeName        string
	Postings         []RecurringDuePosting
}

// RecurringDue reads a whole page and its current posting amounts in one SQL
// snapshot. Archived templates do not hide drafts that still need review.
func (r *RecurringRepository) RecurringDue(ctx context.Context, bookID int64, afterDate string, afterID int64, limit int) ([]RecurringDueRecord, error) {
	rows, err := r.database.QueryContext(ctx, `WITH page AS (
 SELECT o.*, template.name AS template_name, template.enabled,
 template.archived_at IS NOT NULL AS archived,
 COALESCE(tv.description, template.description) AS description,
 CASE WHEN o.status = 'generated' THEN COALESCE(pv.name, tv.payee_name, '') ELSE COALESCE(pv.name, template.payee_name, '') END AS payee_name,
 tv.id AS version_id
 FROM recurring_occurrences o
 JOIN recurring_templates template ON template.id = o.template_id AND template.book_id = o.book_id
 LEFT JOIN transactions t ON t.id = o.transaction_id AND t.book_id = o.book_id
 LEFT JOIN current_transaction_versions tv ON tv.transaction_id = t.id
 LEFT JOIN current_payee_versions pv ON pv.payee_id = CASE WHEN o.status = 'generated' THEN tv.payee_id ELSE template.payee_id END
 WHERE o.book_id = ? AND (o.status = 'blocked' OR (o.status = 'generated' AND tv.status = 'draft' AND t.deleted_at IS NULL))
 AND (o.occurrence_date > ? OR (o.occurrence_date = ? AND o.id > ?))
 ORDER BY o.occurrence_date, o.id LIMIT ?
 ), amounts AS (
 SELECT p.id AS occurrence_id, v.commodity_id, v.quantity_value, v.quantity_scale
 FROM page p JOIN journal_entries e ON e.transaction_version_id = p.version_id
 JOIN posting_versions v ON v.journal_entry_id = e.id WHERE p.status = 'generated'
 UNION ALL
 SELECT p.id, v.commodity_id, v.quantity_value, v.quantity_scale
 FROM page p JOIN recurring_template_postings v ON v.template_id = p.template_id WHERE p.status = 'blocked'
 )
 SELECT p.id, p.book_id, p.template_id, p.occurrence_date, p.status, p.transaction_id,
 p.error_summary, p.skip_reason, p.materialized_at, p.created_at, p.updated_at,
 p.last_audit_event_id, p.template_name, p.enabled, p.archived, p.description, p.payee_name,
 a.commodity_id, c.code, a.quantity_value, a.quantity_scale
 FROM page p LEFT JOIN amounts a ON a.occurrence_id = p.id
 LEFT JOIN commodities c ON c.id = a.commodity_id AND c.book_id = p.book_id
 ORDER BY p.occurrence_date, p.id, a.commodity_id`, bookID, afterDate, afterDate, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("read recurring due: %w", err)
	}
	defer rows.Close()
	result := make([]RecurringDueRecord, 0)
	for rows.Next() {
		var item RecurringDueRecord
		var commodityID, scale sql.NullInt64
		var code, value sql.NullString
		if err := rows.Scan(&item.ID, &item.BookID, &item.TemplateID, &item.OccurrenceDate, &item.Status, &item.TransactionID,
			&item.ErrorSummary, &item.SkipReason, &item.MaterializedAt, &item.CreatedAt, &item.UpdatedAt,
			&item.LastAuditEventID, &item.TemplateName, &item.TemplateEnabled, &item.TemplateArchived, &item.Description, &item.PayeeName,
			&commodityID, &code, &value, &scale); err != nil {
			return nil, fmt.Errorf("scan recurring due: %w", err)
		}
		if len(result) == 0 || result[len(result)-1].ID != item.ID {
			item.Postings = make([]RecurringDuePosting, 0)
			result = append(result, item)
		}
		if commodityID.Valid {
			coefficient, err := exact.Parse(value.String)
			if err != nil {
				return nil, fmt.Errorf("read recurring amount: %w", err)
			}
			last := &result[len(result)-1]
			last.Postings = append(last.Postings, RecurringDuePosting{CommodityID: commodityID.Int64, CommodityCode: code.String, QuantityValue: coefficient, QuantityScale: int(scale.Int64)})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring due: %w", err)
	}
	return result, nil
}

func (r *RecurringRepository) RecurringOccurrencesInRange(ctx context.Context, bookID, templateID int64, from, to string) ([]RecurringOccurrenceRecord, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT id, book_id, template_id, occurrence_date, status, transaction_id,
 error_summary, skip_reason, materialized_at, created_at, updated_at, last_audit_event_id
 FROM recurring_occurrences WHERE book_id = ? AND template_id = ? AND occurrence_date BETWEEN ? AND ?
 ORDER BY occurrence_date, id`, bookID, templateID, from, to)
	if err != nil {
		return nil, fmt.Errorf("read recurring range: %w", err)
	}
	defer rows.Close()
	return scanRecurringOccurrences(rows)
}

// SkipRecurringOccurrence claims a date or transitions the captured blocked
// attempt. The template and occurrence guards are in the same write transaction.
func (r *RecurringRepository) SkipRecurringOccurrence(ctx context.Context, revision int64, occurrence CreateRecurringOccurrenceParams, expectedAuditID *int64, audit AuditEventParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recurring skip: %w", err)
	}
	defer rollbackTx(ctx, tx)
	result, err := tx.ExecContext(ctx, `UPDATE recurring_templates SET revision = revision
 WHERE book_id = ? AND id = ? AND revision = ?`, occurrence.BookID, occurrence.TemplateID, revision)
	if err != nil {
		return fmt.Errorf("guard recurring skip: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRecurringTemplateConflict
	}
	if err := guardRecurringOccurrence(ctx, tx, occurrence, expectedAuditID); err != nil {
		return err
	}
	auditID, err := insertAuditEvent(ctx, tx, audit)
	if err != nil {
		return err
	}
	if expectedAuditID == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO recurring_occurrences
  (book_id, template_id, occurrence_date, status, error_summary, skip_reason, materialized_at, created_at, updated_at, last_audit_event_id)
  VALUES (?, ?, ?, 'skipped', '', ?, ?, ?, ?, ?)`, occurrence.BookID, occurrence.TemplateID, occurrence.OccurrenceDate,
			occurrence.SkipReason, occurrence.MaterializedAt, occurrence.MaterializedAt, occurrence.MaterializedAt, auditID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE recurring_occurrences SET status = 'skipped', error_summary = '', skip_reason = ?, updated_at = ?, last_audit_event_id = ?
  WHERE book_id = ? AND template_id = ? AND occurrence_date = ?`, occurrence.SkipReason, occurrence.MaterializedAt, auditID,
			occurrence.BookID, occurrence.TemplateID, occurrence.OccurrenceDate)
	}
	if err != nil {
		return fmt.Errorf("write recurring skip: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recurring skip: %w", err)
	}
	return nil
}

// A retry/skip can consume only the blocked attempt the caller read. A second
// retry (even another failure) receives a new audit ID, so stale callers lose.
func guardRecurringOccurrence(ctx context.Context, tx *sql.Tx, occurrence CreateRecurringOccurrenceParams, expectedAuditID *int64) error {
	var status string
	var auditID sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT status, last_audit_event_id FROM recurring_occurrences
 WHERE book_id = ? AND template_id = ? AND occurrence_date = ?`, occurrence.BookID, occurrence.TemplateID, occurrence.OccurrenceDate).Scan(&status, &auditID)
	if err == sql.ErrNoRows && expectedAuditID == nil {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read recurring identity: %w", err)
	}
	if expectedAuditID != nil && err == nil && status == "blocked" && auditID.Valid && auditID.Int64 == *expectedAuditID {
		return nil
	}
	return ErrRecurringOccurrenceExists
}

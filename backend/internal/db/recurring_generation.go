package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MaterializeRecurringOccurrence guards the captured template revision before
// writing either a draft and its identity, or a terminal validation failure.
// There is no committed state in which only one half of generation exists.
func (r *RecurringRepository) MaterializeRecurringOccurrence(ctx context.Context, revision int64, occurrence CreateRecurringOccurrenceParams, draft *CreateTransactionParams, audit AuditEventParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recurring generation: %w", err)
	}
	defer rollbackTx(ctx, tx)
	if err := guardRecurringGeneration(ctx, tx, occurrence.BookID, occurrence.TemplateID, revision); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM recurring_occurrences WHERE template_id = ? AND occurrence_date = ?)`, occurrence.TemplateID, occurrence.OccurrenceDate).Scan(&exists); err != nil {
		return fmt.Errorf("check recurring identity: %w", err)
	}
	if exists {
		return ErrRecurringOccurrenceExists
	}
	var auditID int64
	if draft != nil {
		if draft.BookID != occurrence.BookID || draft.Spec.Status != "draft" || draft.OriginType != "scheduled" || draft.Spec.TransactionDate != occurrence.OccurrenceDate {
			return fmt.Errorf("invalid recurring draft contract")
		}
		record, id, err := createTransactionWithAuditTx(ctx, tx, *draft)
		if err != nil {
			return err
		}
		auditID = id
		occurrence.Status, occurrence.TransactionID = "generated", &record.ID
	} else {
		occurrence.Status, occurrence.TransactionID = "blocked", nil
		auditID, err = insertAuditEvent(ctx, tx, audit)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO recurring_occurrences
		(book_id, template_id, occurrence_date, status, transaction_id, error_summary,
		 skip_reason, materialized_at, created_at, updated_at, last_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		occurrence.BookID, occurrence.TemplateID, occurrence.OccurrenceDate, occurrence.Status,
		nullableOptionalInt64(occurrence.TransactionID), occurrence.ErrorSummary, occurrence.SkipReason,
		occurrence.MaterializedAt, occurrence.MaterializedAt, occurrence.MaterializedAt, auditID)
	if isRecurringOccurrenceConflict(err) {
		return ErrRecurringOccurrenceExists
	}
	if err != nil {
		return fmt.Errorf("insert generated occurrence: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recurring generation: %w", err)
	}
	return nil
}

func guardRecurringGeneration(ctx context.Context, tx *sql.Tx, bookID, templateID, revision int64) error {
	// Acquire SQLite's writer lock before taking a read snapshot. A SELECT
	// followed by an INSERT can fail to upgrade a stale WAL snapshot when two
	// independently opened pools generate concurrently. This no-op update
	// both acquires the lock and checks the captured template atomically.
	result, err := tx.ExecContext(ctx, `UPDATE recurring_templates SET revision = revision
		WHERE book_id = ? AND id = ? AND revision = ? AND enabled = 1 AND archived_at IS NULL`, bookID, templateID, revision)
	if err != nil {
		return fmt.Errorf("guard recurring template revision: %w", err)
	}
	matched, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read recurring guard result: %w", err)
	}
	if matched == 0 {
		return ErrRecurringTemplateConflict
	}
	return nil
}

// AdvanceRecurringGeneration only accepts the revision whose window the
// caller fully handled. A concurrent edit, archive, or tick forces a re-read.
func (r *RecurringRepository) AdvanceRecurringGeneration(ctx context.Context, bookID, templateID, revision int64, through, at string) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recurring watermark: %w", err)
	}
	defer rollbackTx(ctx, tx)
	if err := guardRecurringGeneration(ctx, tx, bookID, templateID, revision); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE recurring_templates SET generate_from = ?, updated_at = ?, revision = revision + 1
		WHERE book_id = ? AND id = ? AND generate_from < ?`, through, at, bookID, templateID, through); err != nil {
		return fmt.Errorf("advance recurring watermark: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recurring watermark: %w", err)
	}
	return nil
}

func (r *RecurringRepository) CurrentBookOwnerID(ctx context.Context, bookID int64) (int64, error) {
	var ownerID int64
	if err := r.database.QueryRowContext(ctx, `SELECT owner_user_id FROM books WHERE id = ?`, bookID).Scan(&ownerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("read recurring book owner: %w", err)
	}
	return ownerID, nil
}

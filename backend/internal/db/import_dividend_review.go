package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrImportDividendReviewUnavailable = errors.New("import dividend review is no longer available")

// Shared membership for both the queue and its attention count (#447).
const importDividendReviewFromSQL = ` FROM import_staged_rows s
 JOIN import_batches b ON b.id = s.batch_id AND b.book_id = s.book_id
 LEFT JOIN import_commit_identities i ON i.book_id = s.book_id AND i.dedupe_fingerprint = s.dedupe_fingerprint
 LEFT JOIN transactions t ON t.id = (SELECT e.transaction_id FROM import_commit_identity_effects e
   WHERE e.identity_id = i.id AND e.transaction_id IS NOT NULL ORDER BY e.effect_seq LIMIT 1)
 LEFT JOIN current_transaction_versions v ON v.transaction_id = t.id
 WHERE s.book_id = ? AND s.id > ? AND (? = 0 OR s.batch_id = ?)
 AND b.source_kind = 'trading212' AND json_extract(s.raw_json, '$.kind') = 'trading212_dividend'
 AND b.status NOT IN ('discarded', 'rolled_back')
 AND ((i.id IS NULL AND s.commit_status IN ('pending','failed') AND s.dedupe_status NOT IN ('duplicate','excluded')
       AND (b.status IN ('previewing','partially_committed') OR (b.status='failed' AND s.commit_status='failed')))
   OR (i.id IS NOT NULL AND i.source_kind = 'trading212' AND v.status = 'posted'
       AND v.transaction_kind = 'ordinary' AND t.deleted_at IS NULL
       AND NOT EXISTS (SELECT 1 FROM investment_operation_journal_links l JOIN transaction_versions lv ON lv.id = l.transaction_version_id WHERE lv.transaction_id = t.id)
       AND (? > 0 OR s.id = (SELECT MIN(original.id) FROM import_staged_rows original WHERE original.committed_identity_id = i.id AND original.commit_status = 'committed'))))`

// ListImportDividendReviewRows composes the queue from durable source identities.
// A historical identity is offered once globally, or in its requested batch.
func (r *ImportRepository) ListImportDividendReviewRows(ctx context.Context, bookID, batchID, afterID int64, limit int) ([]ImportStagedRowRecord, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT `+importStagedRowSelect+`
`+importDividendReviewFromSQL+`
 ORDER BY s.id LIMIT ?`, bookID, afterID, batchID, batchID, batchID, limit)
	if err != nil {
		return nil, fmt.Errorf("list dividend review rows: %w", err)
	}
	defer rows.Close()
	result := []ImportStagedRowRecord{}
	for rows.Next() {
		row, err := scanImportStagedRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// CheckPendingDividendRowInTx pins the owner's reviewed resolution until the
// named writer atomically records its financial effects and identity.
func (r *ImportRepository) CheckPendingDividendRowInTx(ctx context.Context, tx *sql.Tx, row ImportStagedRowRecord, expectedResolution string) error {
	var valid bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM import_staged_rows s JOIN import_batches b ON b.id=s.batch_id
 WHERE s.id=? AND s.batch_id=? AND s.book_id=? AND s.dedupe_fingerprint=? AND s.resolution_json=?
 AND s.commit_status IN ('pending','failed') AND s.dedupe_status NOT IN ('duplicate','excluded')
 AND b.status IN ('previewing','partially_committed','failed') AND b.source_kind='trading212'
 AND json_extract(s.raw_json,'$.kind')='trading212_dividend')`, row.ID, row.BatchID, row.BookID, row.DedupeFingerprint, expectedResolution).Scan(&valid)
	if err != nil {
		return fmt.Errorf("check reviewed dividend row: %w", err)
	}
	if !valid {
		return ErrImportDividendReviewUnavailable
	}
	return nil
}

func (r *ImportRepository) SaveDividendResolutionInTx(ctx context.Context, tx *sql.Tx, rowID int64, resolution string) error {
	_, err := tx.ExecContext(ctx, `UPDATE import_staged_rows SET resolution_json=? WHERE id=?`, resolution, rowID)
	return err
}

// FinishDividendReviewInTx shares the named command's audit and rollback.
func (r *ImportRepository) FinishDividendReviewInTx(ctx context.Context, tx *sql.Tx, batchID, transactionID int64, now string) error {
	var auditID int64
	if err := tx.QueryRowContext(ctx, `SELECT change_audit_event_id FROM current_transaction_versions WHERE transaction_id=?`, transactionID).Scan(&auditID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_batches SET status=CASE WHEN EXISTS(SELECT 1 FROM import_staged_rows s WHERE s.batch_id=import_batches.id AND s.commit_status IN ('pending','failed')) THEN 'partially_committed' ELSE 'committed' END
 WHERE id=? AND status IN ('previewing','partially_committed','failed')`, batchID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO import_batch_events(batch_id,event_kind,detail_json,audit_event_id,created_at) SELECT id,status,?,?,? FROM import_batches WHERE id=?`, fmt.Sprintf(`{"action":"dividend_reviewed","transaction_id":%d}`, transactionID), auditID, now, batchID)
	return err
}

type RepairImportDividendParams struct {
	RowID, BatchID, IdentityID, TransactionID, ExpectedVersionID int64
	ResolutionJSON                                               string
	Journal                                                      CreateTransactionParams
	CheckpointCandidates                                         []PeriodScopedCheckpointRef
}

// RepairImportDividend appends a classification version, never a second payment.
// The old journal and source identity/effect remain immutable history.
func (r *ImportRepository) RepairImportDividend(ctx context.Context, p RepairImportDividendParams, preview bool) (TransactionRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, err
	}
	defer rollbackTx(ctx, tx)
	j := p.Journal
	if j.Spec.InvestmentOperationKind != "dividend" || j.Spec.TransactionKind != "investment" || j.Spec.Status != "posted" || p.ExpectedVersionID <= 0 {
		return TransactionRecord{}, ErrImportDividendReviewUnavailable
	}
	if _, err = readBookForUpdate(ctx, tx, j.BookID); err != nil {
		return TransactionRecord{}, err
	}
	current, err := transactionByID(ctx, tx, j.BookID, p.TransactionID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if err = requireExpectedVersionTx(p.ExpectedVersionID, current); err != nil {
		return TransactionRecord{}, err
	}
	if current.Status != "posted" || current.TransactionKind != "ordinary" || current.DeletedAt.Valid || current.TransactionDate != j.Spec.TransactionDate {
		return TransactionRecord{}, ErrImportDividendReviewUnavailable
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM import_staged_rows s
 JOIN import_batches b ON b.id=s.batch_id JOIN import_commit_identities i ON i.book_id=s.book_id AND i.dedupe_fingerprint=s.dedupe_fingerprint
 JOIN import_commit_identity_effects e ON e.identity_id=i.id AND e.transaction_id=?
 WHERE s.id=? AND s.batch_id=? AND i.id=? AND i.book_id=? AND i.source_kind='trading212' AND b.source_kind='trading212'
 AND b.status NOT IN ('discarded','rolled_back') AND json_extract(s.raw_json,'$.kind')='trading212_dividend'
 AND NOT EXISTS(SELECT 1 FROM investment_operation_journal_links l JOIN transaction_versions v ON v.id=l.transaction_version_id WHERE v.transaction_id=?))`, p.TransactionID, p.RowID, p.BatchID, p.IdentityID, j.BookID, p.TransactionID).Scan(&valid)
	if err != nil {
		return TransactionRecord{}, err
	}
	if !valid {
		return TransactionRecord{}, ErrImportDividendReviewUnavailable
	}
	if err = requireAccountRuleDependenciesTx(ctx, tx, j.Spec, j.AccountRuleDependencies); err != nil {
		return TransactionRecord{}, err
	}
	auditID, err := insertAuditEvent(ctx, tx, AuditEventParams{BookID: j.BookID, ActorUserID: j.ActorUserID, AuthSessionID: j.AuthSessionID, OccurredAt: j.CreatedAt, RequestID: j.RequestID, OriginType: "import", Operation: "import.dividend.repair", Reason: j.ChangeReason, MetadataJSON: j.AuditMetadataJSON})
	if err != nil {
		return TransactionRecord{}, err
	}
	repository := &TransactionRepository{}
	record, err := repository.insertTransactionVersion(ctx, tx, insertTransactionVersionParams{BookID: j.BookID, TransactionID: p.TransactionID, VersionSeq: current.VersionSeq + 1, SupersedesVersionID: sql.NullInt64{Int64: current.VersionID, Valid: true}, Spec: j.Spec, ReplaceTags: true, RecordedAt: j.CreatedAt, ChangedByUserID: j.ActorUserID, ChangeReason: j.ChangeReason, ChangeAuditEventID: auditID, RequestID: j.RequestID, TransactionDaySequence: current.TransactionDaySequence})
	if err != nil {
		return TransactionRecord{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_operations(book_id,operation_kind,event_date,created_at,created_audit_event_id) VALUES (?,'dividend',?,?,?)`, j.BookID, j.Spec.TransactionDate, j.CreatedAt, auditID)
	if err != nil {
		return TransactionRecord{}, err
	}
	operationID, err := result.LastInsertId()
	if err != nil {
		return TransactionRecord{}, err
	}
	if err = recordInvestmentFoundationTx(ctx, tx, j, record, auditID, operationID); err != nil {
		return TransactionRecord{}, err
	}
	if preview {
		record.InvalidatedCheckpointRefs, err = periodScopedCheckpointInvalidationRefs(ctx, tx, j.BookID, p.CheckpointCandidates)
		if err != nil {
			return TransactionRecord{}, err
		}
	}
	record.InvalidatedCheckpointIDs, err = enforceCheckpointBoundaryTx(ctx, tx, checkpointBoundaryParams{BookID: j.BookID, Candidates: p.CheckpointCandidates, ReconciliationOverride: j.ReconciliationOverride, ReviewedCheckpointIDs: j.ReviewedCheckpointIDs, ActorUserID: j.ActorUserID, AuditEventID: auditID, OccurredAt: j.CreatedAt, Reason: j.ChangeReason})
	if err != nil {
		return TransactionRecord{}, err
	}
	if preview {
		return record, nil
	}
	// The original transaction effect is unique and stays in place. The new
	// operation refers to its new journal version through its primary link.
	_, err = tx.ExecContext(ctx, `INSERT INTO import_commit_identity_effects(identity_id,effect_seq,operation_id) SELECT ?,COALESCE(MAX(effect_seq),0)+1,? FROM import_commit_identity_effects WHERE identity_id=?`, p.IdentityID, operationID, p.IdentityID)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("record repaired dividend source operation: %w", err)
	}
	if err = r.SaveDividendResolutionInTx(ctx, tx, p.RowID, p.ResolutionJSON); err != nil {
		return TransactionRecord{}, err
	}
	if err = r.FinishDividendReviewInTx(ctx, tx, p.BatchID, p.TransactionID, j.CreatedAt); err != nil {
		return TransactionRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return TransactionRecord{}, err
	}
	return record, nil
}

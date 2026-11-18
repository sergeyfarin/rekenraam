package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReconciliationPolicyRecord is the book's stored reconciliation window
// (#482). ErrNotFound means the book never saved one and the default applies.
type ReconciliationPolicyRecord struct {
	BookID             int64
	OutOfDateAfterDays int
	UpdatedAt          string
}

type SaveReconciliationPolicyParams struct {
	BookID             int64
	OutOfDateAfterDays int
	ActorUserID        int64
	AuthSessionID      int64
	RequestID          string
	OriginType         string
	Operation          string
	RecordedAt         string
	ChangeReason       string
}

// ReconciliationAccountFact is what judging one account's reconciliation age
// needs: its latest active checkpoint's statement date (empty when it has
// none) and whether a current posted transaction posts to it.
type ReconciliationAccountFact struct {
	AccountID int64
	// Name and Code are empty when the account has none (a starter account
	// is labelled by its built-in key).
	Name                string
	Code                string
	LatestStatementDate string
	HasPostedPostings   bool
}

func (r *TransactionRepository) GetReconciliationPolicy(ctx context.Context, bookID int64) (ReconciliationPolicyRecord, error) {
	var record ReconciliationPolicyRecord
	err := r.database.QueryRowContext(ctx, `
		SELECT book_id, out_of_date_after_days, updated_at
		FROM reconciliation_policies
		WHERE book_id = ?
	`, bookID).Scan(&record.BookID, &record.OutOfDateAfterDays, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ReconciliationPolicyRecord{}, ErrNotFound
	}
	if err != nil {
		return ReconciliationPolicyRecord{}, fmt.Errorf("read reconciliation policy: %w", err)
	}
	return record, nil
}

// SaveReconciliationPolicy writes the window and its audit event in one
// transaction.
func (r *TransactionRepository) SaveReconciliationPolicy(ctx context.Context, params SaveReconciliationPolicyParams) (ReconciliationPolicyRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationPolicyRecord{}, fmt.Errorf("begin save reconciliation policy: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ActorUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return ReconciliationPolicyRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO reconciliation_policies (
			book_id, out_of_date_after_days, created_at, created_by_user_id,
			updated_at, updated_by_user_id, created_audit_event_id, updated_audit_event_id
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (book_id) DO UPDATE SET
			out_of_date_after_days = excluded.out_of_date_after_days,
			updated_at = excluded.updated_at,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_audit_event_id = excluded.updated_audit_event_id
	`, params.BookID, params.OutOfDateAfterDays, params.RecordedAt, params.ActorUserID,
		params.RecordedAt, params.ActorUserID, auditEventID, auditEventID); err != nil {
		return ReconciliationPolicyRecord{}, fmt.Errorf("save reconciliation policy: %w", err)
	}
	var record ReconciliationPolicyRecord
	if err := tx.QueryRowContext(ctx, `
		SELECT book_id, out_of_date_after_days, updated_at
		FROM reconciliation_policies
		WHERE book_id = ?
	`, params.BookID).Scan(&record.BookID, &record.OutOfDateAfterDays, &record.UpdatedAt); err != nil {
		return ReconciliationPolicyRecord{}, fmt.Errorf("read saved reconciliation policy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationPolicyRecord{}, fmt.Errorf("commit save reconciliation policy: %w", err)
	}
	committed = true
	return record, nil
}

// ReconciliationAccountFacts reads every account that can be reconciled, with
// its latest active checkpoint and whether it has posted activity, in one
// statement so both facts describe one state of the book. An account can be
// reconciled when it is an active, posting, non-system asset or liability
// outside the investment subledger (holdings are reconciled through their
// investment commands, as the Reconcile picker's default list says). Voided
// and invalidated checkpoints never count; neither do drafts, voided versions
// or deleted transactions.
func (r *TransactionRepository) ReconciliationAccountFacts(ctx context.Context, bookID int64) ([]ReconciliationAccountFact, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			a.id,
			COALESCE(av.name, ''),
			COALESCE(av.code, ''),
			COALESCE((
				SELECT MAX(rc.statement_date)
				FROM reconciliation_checkpoints rc
				WHERE rc.book_id = a.book_id AND rc.account_id = a.id AND rc.status = 'active'
			), ''),
			EXISTS (
				SELECT 1
				FROM posting_versions pv
				JOIN current_transaction_versions tv ON tv.id = pv.transaction_version_id
				JOIN transactions t ON t.id = tv.transaction_id
				WHERE pv.account_id = a.id AND tv.status = 'posted' AND t.deleted_at IS NULL
			)
		FROM accounts a
		JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id
		JOIN account_kinds ak ON ak.code = av.account_kind
		WHERE a.book_id = ?
			AND a.system_role IS NULL
			AND av.status = 'active'
			AND av.allows_postings = 1
			AND av.account_class IN ('asset', 'liability')
			AND ak.base_kind != 'security_holding'
		ORDER BY a.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read reconciliation account facts: %w", err)
	}
	defer rows.Close()
	facts := []ReconciliationAccountFact{}
	for rows.Next() {
		var fact ReconciliationAccountFact
		if err := rows.Scan(&fact.AccountID, &fact.Name, &fact.Code, &fact.LatestStatementDate, &fact.HasPostedPostings); err != nil {
			return nil, fmt.Errorf("scan reconciliation account fact: %w", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation account facts: %w", err)
	}
	return facts, nil
}

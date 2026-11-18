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

// ReconciliationAccountFact is an account that can be reconciled and, per
// commodity it has posted activity in or an active checkpoint for, what
// judging that commodity's reconciliation age needs. Commodities is empty when
// the account has neither.
type ReconciliationAccountFact struct {
	AccountID int64
	// Name and Code are empty when the account has none; StarterKey and
	// CurrencyCode label a starter account as everywhere else.
	Name         string
	Code         string
	StarterKey   string
	CurrencyCode string
	Commodities  []ReconciliationCommodityFact
}

type ReconciliationCommodityFact struct {
	CommodityID   int64
	CommodityCode string
	// LatestStatementDate is the latest active checkpoint dated on or before
	// today; empty when there is none.
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

// ReconciliationAccountFacts reads, in one snapshot and on the given
// owner-local today, every account that can be reconciled with its posted
// activity and latest active checkpoint per commodity. An account can be
// reconciled when, on today, it is an active, posting, non-system asset or
// liability outside the investment subledger (holdings are reconciled through
// their investment commands, as the Reconcile picker's default list says).
// Activity is a current posted, undeleted transaction dated on or before
// today; a checkpoint counts only while active and dated on or before today,
// so a statement entered for a later date never vouches for today.
//
// accountID > 0 reads that one account only, through its own postings, so a
// form that needs one account's commodities does not scan the book (#594
// review); 0 reads every account.
func (r *TransactionRepository) ReconciliationAccountFacts(ctx context.Context, bookID int64, today string, accountID int64) ([]ReconciliationAccountFact, error) {
	ctx = WithReferenceDate(ctx, today)
	facts := []ReconciliationAccountFact{}
	err := r.WithLedgerSnapshot(ctx, func(snapshot *LedgerSnapshotReader) error {
		tx := snapshot.transaction
		byID := map[int64]int{}
		rows, err := tx.QueryContext(ctx, `
			SELECT a.id, COALESCE(av.name, ''), COALESCE(av.code, ''),
				COALESCE(json_extract(av.metadata_json, '$.starter_key'), ''),
				COALESCE(json_extract(av.metadata_json, '$.currency_code'), '')
			FROM accounts a
			JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id
			JOIN account_kinds ak ON ak.code = av.account_kind
			WHERE a.book_id = ?
				AND a.system_role IS NULL
				AND av.status = 'active'
				AND av.allows_postings = 1
				AND av.account_class IN ('asset', 'liability')
				AND ak.base_kind != 'security_holding'
				AND (? = 0 OR a.id = ?)
			ORDER BY a.id
		`, bookID, accountID, accountID)
		if err != nil {
			return fmt.Errorf("read reconcilable accounts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var fact ReconciliationAccountFact
			if err := rows.Scan(&fact.AccountID, &fact.Name, &fact.Code, &fact.StarterKey, &fact.CurrencyCode); err != nil {
				return fmt.Errorf("scan reconcilable account: %w", err)
			}
			fact.Commodities = []ReconciliationCommodityFact{}
			byID[fact.AccountID] = len(facts)
			facts = append(facts, fact)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate reconcilable accounts: %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(facts) == 0 {
			return nil
		}
		// Book-wide, drive from the current transaction versions; for one
		// account, from its own postings (posting_versions_account_idx).
		posted := `
				SELECT DISTINCT pv.account_id, pv.commodity_id
				FROM current_transaction_versions tv
				JOIN transactions t ON t.id = tv.transaction_id
				JOIN journal_entries je ON je.transaction_version_id = tv.id
				JOIN posting_versions pv ON pv.journal_entry_id = je.id
				WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL AND je.entry_date <= ?`
		postedArgs := []any{bookID, today}
		if accountID > 0 {
			posted = `
				SELECT DISTINCT pv.account_id, pv.commodity_id
				FROM posting_versions pv
				JOIN current_transaction_versions tv ON tv.id = pv.transaction_version_id
				JOIN transactions t ON t.id = tv.transaction_id
				JOIN journal_entries je ON je.id = pv.journal_entry_id
				WHERE pv.account_id = ? AND tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL AND je.entry_date <= ?`
			postedArgs = []any{accountID, bookID, today}
		}
		args := append(postedArgs, bookID, today, accountID, accountID)
		pairs, err := tx.QueryContext(ctx, `
			WITH posted AS (`+posted+`
			),
			checked AS (
				SELECT account_id, commodity_id, MAX(statement_date) AS statement_date
				FROM reconciliation_checkpoints
				WHERE book_id = ? AND status = 'active' AND statement_date <= ? AND (? = 0 OR account_id = ?)
				GROUP BY account_id, commodity_id
			),
			pairs AS (
				SELECT account_id, commodity_id FROM posted
				UNION
				SELECT account_id, commodity_id FROM checked
			)
			SELECT p.account_id, p.commodity_id, c.code, COALESCE(k.statement_date, ''),
				EXISTS (SELECT 1 FROM posted x WHERE x.account_id = p.account_id AND x.commodity_id = p.commodity_id)
			FROM pairs p
			JOIN commodities c ON c.id = p.commodity_id
			LEFT JOIN checked k ON k.account_id = p.account_id AND k.commodity_id = p.commodity_id
			ORDER BY p.account_id, p.commodity_id
		`, args...)
		if err != nil {
			return fmt.Errorf("read reconciliation commodities: %w", err)
		}
		defer pairs.Close()
		for pairs.Next() {
			var accountID int64
			var fact ReconciliationCommodityFact
			if err := pairs.Scan(&accountID, &fact.CommodityID, &fact.CommodityCode, &fact.LatestStatementDate, &fact.HasPostedPostings); err != nil {
				return fmt.Errorf("scan reconciliation commodity: %w", err)
			}
			if index, ok := byID[accountID]; ok {
				facts[index].Commodities = append(facts[index].Commodities, fact)
			}
		}
		return pairs.Err()
	})
	if err != nil {
		return nil, err
	}
	return facts, nil
}

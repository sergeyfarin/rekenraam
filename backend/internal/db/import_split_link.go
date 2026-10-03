package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// A provider split row (Trading 212 STOCK_SPLIT) carries no verified ratio or
// entitlement, so it never posts. The user records the split manually and
// links the staged row to it: the row's dedupe identity then names the
// existing split operation, so neither a retry nor a re-fetch can post it
// again (T-122).

// ErrSplitLinkUnavailable means the staged row or split can no longer be
// linked: the row was committed or is not a provider split, or the split is
// not effective, belongs to another security, or is already linked.
var ErrSplitLinkUnavailable = errors.New("staged split row cannot be linked to this split")

// SplitLinkCandidate is an effective recorded split of a security, with the
// staged-row identity already linked to it, if any.
type SplitLinkCandidate struct {
	OperationID      int64
	TransactionID    int64
	HoldingAccountID int64
	EffectiveOn      string
	RatioNumerator   int64
	RatioDenominator int64
	Linked           bool
}

// ListSplitLinkCandidates returns the effective splits of one security, newest
// first, marking those already linked to a committed source row.
func (r *ImportRepository) ListSplitLinkCandidates(ctx context.Context, bookID, commodityID int64) ([]SplitLinkCandidate, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT f.operation_id, v.transaction_id, f.account_id, f.effective_on,
			f.ratio_numerator, f.ratio_denominator,
			EXISTS (SELECT 1 FROM import_commit_identity_effects e WHERE e.operation_id = f.operation_id)
		FROM investment_split_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE f.book_id = ? AND f.commodity_id = ?
		ORDER BY f.effective_on DESC, f.operation_id DESC`, bookID, commodityID)
	if err != nil {
		return nil, fmt.Errorf("read split link candidates: %w", err)
	}
	defer rows.Close()
	var candidates []SplitLinkCandidate
	for rows.Next() {
		var candidate SplitLinkCandidate
		if err := rows.Scan(&candidate.OperationID, &candidate.TransactionID, &candidate.HoldingAccountID,
			&candidate.EffectiveOn, &candidate.RatioNumerator, &candidate.RatioDenominator, &candidate.Linked); err != nil {
			return nil, fmt.Errorf("scan split link candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate split link candidates: %w", err)
	}
	return candidates, nil
}

// LinkStagedRowToSplitParams names the staged provider row and the recorded
// split it evidences. CommodityID is the security the row resolved to.
type LinkStagedRowToSplitParams struct {
	BookID            int64
	BatchID           int64
	RowID             int64
	DedupeFingerprint string
	SourceKind        string
	OperationID       int64
	CommodityID       int64
	ActorUserID       int64
	AuthSessionID     int64
	RequestID         string
	Now               string
}

// LinkStagedRowToSplit admits the row's dedupe identity with the split
// operation as its only effect and marks the row committed, under one audit
// event. No journal or lot row is written, so nothing can post twice.
func (r *ImportRepository) LinkStagedRowToSplit(ctx context.Context, params LinkStagedRowToSplitParams) (int64, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin split link: %w", err)
	}
	defer rollbackTx(ctx, tx)
	var batchID int64
	var commitStatus, fingerprint string
	err = tx.QueryRowContext(ctx, `SELECT batch_id, commit_status, dedupe_fingerprint FROM import_staged_rows
		WHERE id = ? AND book_id = ?`, params.RowID, params.BookID).Scan(&batchID, &commitStatus, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSplitLinkUnavailable
	}
	if err != nil {
		return 0, fmt.Errorf("read staged split row: %w", err)
	}
	if batchID != params.BatchID || commitStatus == "committed" || fingerprint != params.DedupeFingerprint {
		return 0, ErrSplitLinkUnavailable
	}
	var accountID, transactionID int64
	err = tx.QueryRowContext(ctx, `
		SELECT f.account_id, v.transaction_id FROM investment_split_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE f.operation_id = ? AND f.book_id = ? AND f.commodity_id = ?
			AND NOT EXISTS (SELECT 1 FROM import_commit_identity_effects e WHERE e.operation_id = o.id)`,
		params.OperationID, params.BookID, params.CommodityID).Scan(&accountID, &transactionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSplitLinkUnavailable
	}
	if err != nil {
		return 0, fmt.Errorf("read split to link: %w", err)
	}
	if _, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: params.BookID, ActorUserID: params.ActorUserID, AuthSessionID: params.AuthSessionID,
		OccurredAt: params.Now, RequestID: params.RequestID, OriginType: "import",
		Operation: "import.row.link_split",
		MetadataJSON: fmt.Sprintf(`{"batch_id":%d,"row_id":%d,"operation_id":%d}`,
			params.BatchID, params.RowID, params.OperationID),
	}); err != nil {
		return 0, err
	}
	identityID, err := r.CreateCommitIdentityWithEffects(ctx, tx, CreateImportCommitIdentityParams{
		BookID: params.BookID, DedupeFingerprint: params.DedupeFingerprint,
		CommittedTransactionID: transactionID, SourceKind: params.SourceKind,
		AccountID: accountID, CreatedAt: params.Now,
	}, []CreateImportCommitEffectParams{{
		OperationID:   sql.NullInt64{Int64: params.OperationID, Valid: true},
		TransactionID: sql.NullInt64{Int64: transactionID, Valid: true},
	}})
	if errors.Is(err, ErrCommitIdentityConflict) {
		return 0, ErrSplitLinkUnavailable
	}
	if err != nil {
		return 0, err
	}
	if err := commitImportStagedRowExec(ctx, tx, CommitImportStagedRowParams{
		RowID: params.RowID, CommitStatus: "committed",
		CommittedIdentityID:    sql.NullInt64{Int64: identityID, Valid: true},
		CommittedTransactionID: sql.NullInt64{Int64: transactionID, Valid: true},
	}); err != nil {
		if errors.Is(err, ErrImportStagedRowAlreadyCommitted) {
			return 0, ErrSplitLinkUnavailable
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit split link: %w", err)
	}
	return identityID, nil
}

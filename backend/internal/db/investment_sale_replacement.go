package db

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvestmentSaleNotLatest = errors.New("investment sale has a later position operation")

// ReplaceLatestSale commits an inverse journal, a corrected sale, its fresh
// disposal decision, and the resulting lot projection as one audited command.
// Later position intents are fenced until replacement can replay them too.
func (r *InvestmentRepository) ReplaceLatestSale(ctx context.Context, expected SaleOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, disposalParams DisposeLotsParams,
) (TransactionRecord, TransactionRecord, []LotDisposalRecord, DisposalDecisionRecord, error) {
	var noInverse, noReplacement TransactionRecord
	var noDecision DisposalDecisionRecord
	if expected.OperationID <= 0 || inverseParams.BookID <= 0 ||
		inverseParams.BookID != replacementParams.BookID || inverseParams.BookID != disposalParams.BookID ||
		inverseParams.ActorUserID != replacementParams.ActorUserID || inverseParams.ActorUserID != disposalParams.ActorUserID ||
		inverseParams.Spec.InvestmentOperationKind != "" ||
		inverseParams.Spec.TransactionKind != "investment" || inverseParams.Spec.Status != "posted" ||
		replacementParams.Spec.InvestmentOperationKind != "sell" ||
		replacementParams.Spec.TransactionKind != "investment" || replacementParams.Spec.Status != "posted" ||
		inverseParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.Spec.TransactionDate != expected.EventDate ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" ||
		replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid ||
		inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid ||
		replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		disposalParams.AccountID != expected.AccountID ||
		disposalParams.CommodityID != expected.CommodityID ||
		disposalParams.CostCommodityID != expected.CostCommodityID {
		return noInverse, noReplacement, nil, noDecision, fmt.Errorf("%w: sale replacement is incomplete", ErrInvalidDisposalParams)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, fmt.Errorf("begin sale replacement: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	current, err := checkSaleOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, inverseParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	if len(intents) == 0 || intents[len(intents)-1].OperationID != current.OperationID {
		return noInverse, noReplacement, nil, noDecision, ErrInvestmentSaleNotLatest
	}
	if _, err := readBookForUpdate(ctx, tx, inverseParams.BookID); err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	for _, params := range []CreateTransactionParams{inverseParams, replacementParams} {
		if err := requireAccountRuleDependenciesTx(ctx, tx, params.Spec, params.AccountRuleDependencies); err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID: inverseParams.BookID, ActorUserID: inverseParams.ActorUserID,
		AuthSessionID: inverseParams.AuthSessionID, OccurredAt: inverseParams.CreatedAt,
		RequestID: inverseParams.RequestID, OriginType: inverseParams.OriginType,
		Operation: inverseParams.Operation, Reason: inverseParams.ChangeReason,
	})
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	inverse, err := insertTransactionWithAuditEventTx(ctx, tx, inverseParams, auditEventID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	replacement, err := insertTransactionWithAuditEventTx(ctx, tx, replacementParams, auditEventID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	operationID, err := investmentOperationIDTx(ctx, tx, replacementParams.BookID, replacement.ID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		VALUES (?, ?, ?, 2, 'reversal')`, replacementParams.BookID, operationID, inverse.VersionID); err != nil {
		return noInverse, noReplacement, nil, noDecision, fmt.Errorf("link replacement inverse: %w", err)
	}
	intents, err = investmentReplayIntentsQuery(ctx, tx, replacementParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, replacementParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, intents)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	if err := persistInvestmentReplayProjectionTx(ctx, tx, replacementParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID,
		operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt,
		intents, projection); err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	disposalParams.TransactionID = replacement.ID
	disposalParams.CreatedAt = replacementParams.CreatedAt
	disposals, err := disposeLotsWithAuditTx(ctx, tx, disposalParams, auditEventID, false)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	decision, err := createDisposalDecisionTx(ctx, tx, replacement, disposalParams, disposals, auditEventID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	if err := voidTradePricesForVersionTx(ctx, tx, inverseParams, current.TransactionVersionID, auditEventID); err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	inverse.InvalidatedCheckpointIDs, err = invalidateCreateTransactionCheckpointsTx(ctx, tx, inverseParams, auditEventID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	replacement.InvalidatedCheckpointIDs, err = invalidateCreateTransactionCheckpointsTx(ctx, tx, replacementParams, auditEventID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	if err := tx.Commit(); err != nil {
		return noInverse, noReplacement, nil, noDecision, fmt.Errorf("commit sale replacement: %w", err)
	}
	committed = true
	return inverse, replacement, disposals, decision, nil
}

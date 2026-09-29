package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ReplaceSale commits an inverse journal, a corrected sale, its fresh
// disposal decision, and the resulting lot projection as one audited command.
// An older sale is evaluated at its original order slot and later decisions
// receive effective revisions in the same write transaction.
func (r *InvestmentRepository) ReplaceSale(ctx context.Context, expected SaleOperationRecord,
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
	current, err := checkSaleOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected, true)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, inverseParams.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	saleIndex := -1
	for index, intent := range intents {
		if intent.OperationID == current.OperationID && intent.Kind == "disposal" {
			saleIndex = index
			break
		}
	}
	if saleIndex < 0 {
		return noInverse, noReplacement, nil, noDecision, ErrNotFound
	}
	latestRewriteDate, err := latestPositionRewriteDateTx(ctx, tx, inverseParams.BookID,
		current.AccountID, current.CommodityID)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	// A later sale that was subsequently reversed is absent from effective
	// intents but its immutable dated event still makes the ordinary disposal
	// guard reject a backdated insert. Use chronological replay in that case.
	olderSale := saleIndex < len(intents)-1 || latestRewriteDate > current.EventDate
	sourceIntents := intents
	sourceDecisionID := intents[saleIndex].DecisionID
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
	disposalParams.TransactionID = replacement.ID
	disposalParams.CreatedAt = replacementParams.CreatedAt
	var disposals []LotDisposalRecord
	var decision DisposalDecisionRecord
	if olderSale {
		proposedIntents, err := proposedSaleReplayIntents(sourceIntents, current.OperationID, disposalParams)
		if err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
		proposed, err := simulateInvestmentReplayTx(ctx, tx, replacementParams.BookID,
			current.AccountID, current.CommodityID, current.CostCommodityID, proposedIntents)
		if err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
		var historical *InvestmentReplayDisposal
		for index := range proposed.Disposals {
			if proposed.Disposals[index].DecisionID == sourceDecisionID {
				historical = &proposed.Disposals[index]
				break
			}
		}
		if historical == nil {
			return noInverse, noReplacement, nil, noDecision,
				fmt.Errorf("%w: corrected sale has no simulated allocation", ErrInvalidDisposalParams)
		}
		disposals, err = insertHistoricalSaleDisposalsTx(ctx, tx, disposalParams,
			historical.Allocations, auditEventID)
		if err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
		decision, err = createDisposalDecisionTx(ctx, tx, replacement, disposalParams, disposals, auditEventID)
		if err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
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
	if !olderSale {
		disposals, err = disposeLotsWithAuditTx(ctx, tx, disposalParams, auditEventID, false)
		if err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
		decision, err = createDisposalDecisionTx(ctx, tx, replacement, disposalParams, disposals, auditEventID)
		if err != nil {
			return noInverse, noReplacement, nil, noDecision, err
		}
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

// These events are immutable evidence of the corrected sale at its historical
// date. The current lot projection comes from full chronological replay, so
// inserting the evidence must not mutate the present-day lot balances.
func insertHistoricalSaleDisposalsTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams,
	allocations []InvestmentReplayAllocation, auditEventID int64,
) ([]LotDisposalRecord, error) {
	if len(allocations) == 0 || params.TransactionID <= 0 || auditEventID <= 0 {
		return nil, fmt.Errorf("%w: historical sale allocation is incomplete", ErrInvalidDisposalParams)
	}
	disposals := make([]LotDisposalRecord, 0, len(allocations))
	for _, allocation := range allocations {
		if allocation.LotID <= 0 || allocation.QuantityValue.Sign() <= 0 ||
			allocation.CostBasisValue < 0 {
			return nil, fmt.Errorf("%w: historical sale allocation is invalid", ErrInvalidDisposalParams)
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_events
			(book_id, lot_id, event_kind, transaction_id, event_date, quantity_value,
			 quantity_scale, cost_basis_value, cost_basis_scale, cost_basis_method,
			 metadata_json, created_at, created_by_user_id, created_audit_event_id)
			VALUES (?, ?, 'disposal', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			params.BookID, allocation.LotID, params.TransactionID, params.EventDate,
			allocation.QuantityValue.Negated(), allocation.QuantityScale,
			-allocation.CostBasisValue, allocation.CostBasisScale,
			params.CostBasisMethod, params.MetadataJSON, params.CreatedAt,
			params.ActorUserID, auditEventID)
		if err != nil {
			return nil, fmt.Errorf("insert historical sale lot event: %w", err)
		}
		eventID, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("read historical sale lot event id: %w", err)
		}
		disposals = append(disposals, LotDisposalRecord{
			EventID: eventID, LotID: allocation.LotID,
			QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale,
			CostBasisValue: allocation.CostBasisValue, CostBasisScale: allocation.CostBasisScale,
			ProceedsValue: allocation.ProceedsValue, ProceedsScale: allocation.ProceedsScale,
			CostCommodityID: params.CostCommodityID,
		})
	}
	return disposals, nil
}

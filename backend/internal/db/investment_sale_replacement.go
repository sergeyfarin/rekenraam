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
	return r.ReplaceSaleWithPostWrite(ctx, expected, inverseParams, replacementParams, disposalParams, nil)
}

// ReplaceSaleWithPostWrite accepts source evidence in the same transaction as
// the journal, disposal replay, prices, audit and reconciliation invalidation.
func (r *InvestmentRepository) ReplaceSaleWithPostWrite(ctx context.Context, expected SaleOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, disposalParams DisposeLotsParams,
	postWrite func(*sql.Tx, int64, int64) error,
) (TransactionRecord, TransactionRecord, []LotDisposalRecord, DisposalDecisionRecord, error) {
	return r.replaceSale(ctx, expected, inverseParams, replacementParams, disposalParams, postWrite, false)
}

// PreviewSaleReplacement runs the complete replacement writer — inverse,
// corrected disposal, dependent replay, prices and checkpoints — and rolls
// back. Source acceptance never runs and no temporary IDs escape (T-126).
func (r *InvestmentRepository) PreviewSaleReplacement(ctx context.Context, expected SaleOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, disposalParams DisposeLotsParams,
) (SimulatedInvestmentWrite, error) {
	inverse, replacement, _, _, err := r.replaceSale(ctx, expected, inverseParams, replacementParams, disposalParams, nil, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(inverse, replacement), nil
}

func (r *InvestmentRepository) replaceSale(ctx context.Context, expected SaleOperationRecord,
	inverseParams, replacementParams CreateTransactionParams, disposalParams DisposeLotsParams,
	postWrite func(*sql.Tx, int64, int64) error, preview bool,
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
		disposalParams.EventDate != replacementParams.Spec.TransactionDate ||
		replacementParams.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacementParams.InvestmentCorrectionMode != "replace" ||
		replacementParams.InvestmentCorrectionReason == "" ||
		!inverseParams.CorrectionOfTransactionID.Valid ||
		inverseParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		!replacementParams.CorrectionOfTransactionID.Valid ||
		replacementParams.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		disposalParams.AccountID <= 0 || disposalParams.CommodityID <= 0 || disposalParams.CostCommodityID <= 0 {
		return noInverse, noReplacement, nil, noDecision, fmt.Errorf("%w: sale replacement is incomplete", ErrInvalidDisposalParams)
	}
	var current SaleOperationRecord
	var sourceIntents []InvestmentReplayIntent
	var sourceDecisionID, operationID int64
	var olderSale bool
	// A corrected date, holding, instrument or cost currency (T-116) removes
	// the sale from its source slot and admits the replacement at the new
	// date as a backdated disposal would be (T-117).
	source := investmentReplayPositionKey{expected.AccountID, expected.CommodityID, expected.CostCommodityID}
	target := investmentReplayPositionKey{disposalParams.AccountID, disposalParams.CommodityID, disposalParams.CostCommodityID}
	moved := source != target || disposalParams.EventDate != expected.EventDate
	write := executeInvestmentJournalsWithGuardTx[saleReplacementEffects]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[saleReplacementEffects]
	}
	var acceptSource func(*sql.Tx, []TransactionRecord, int64) error
	if postWrite != nil {
		acceptSource = func(tx *sql.Tx, _ []TransactionRecord, auditEventID int64) error {
			return postWrite(tx, operationID, auditEventID)
		}
	}
	journals, effects, err := write(ctx, r.database,
		[]CreateTransactionParams{inverseParams, replacementParams},
		func(tx *sql.Tx) error {
			var err error
			current, err = checkSaleOperationForCorrectionTx(ctx, tx, inverseParams.BookID, expected)
			if err != nil || moved {
				return err
			}
			intents, err := investmentReplayIntentsQuery(ctx, tx, inverseParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID, "long")
			if err != nil {
				return err
			}
			saleIndex := -1
			for index, intent := range intents {
				if intent.OperationID == current.OperationID && intent.Kind == "disposal" {
					saleIndex = index
					break
				}
			}
			if saleIndex < 0 {
				return ErrNotFound
			}
			latestRewriteDate, err := latestPositionRewriteDateTx(ctx, tx, inverseParams.BookID,
				current.AccountID, current.CommodityID)
			if err != nil {
				return err
			}
			// A later sale that was subsequently reversed is absent from effective
			// intents but its immutable dated event still makes the ordinary disposal
			// guard reject a backdated insert. Use chronological replay in that case.
			olderSale = saleIndex < len(intents)-1 || latestRewriteDate > current.EventDate
			sourceIntents = intents
			sourceDecisionID = intents[saleIndex].DecisionID
			return nil
		}, func(tx *sql.Tx, journals []TransactionRecord, auditEventID int64) (saleReplacementEffects, error) {
			inverse, replacement := journals[0], journals[1]
			var err error
			operationID, err = investmentOperationIDTx(ctx, tx, replacementParams.BookID, replacement.ID)
			if err != nil {
				return saleReplacementEffects{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		VALUES (?, ?, ?, 2, 'reversal')`, replacementParams.BookID, operationID, inverse.VersionID); err != nil {
				return saleReplacementEffects{}, fmt.Errorf("link replacement inverse: %w", err)
			}
			disposalParams.TransactionID = replacement.ID
			disposalParams.CreatedAt = replacementParams.CreatedAt
			var disposals []LotDisposalRecord
			var decision DisposalDecisionRecord
			if moved {
				if source != target {
					if err := replayCorrectedPositionTx(ctx, tx, replacementParams.BookID, source,
						operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt); err != nil {
						return saleReplacementEffects{}, err
					}
				}
				disposals, decision, err = disposeBehindLaterRewriteTx(ctx, tx, replacement, disposalParams, auditEventID)
				if err != nil {
					return saleReplacementEffects{}, err
				}
				if err := voidTradePricesForVersionTx(ctx, tx, inverseParams, current.TransactionVersionID, auditEventID); err != nil {
					return saleReplacementEffects{}, err
				}
				return saleReplacementEffects{disposals: disposals, decision: decision}, nil
			}
			if olderSale {
				proposedIntents, err := proposedSaleReplayIntents(sourceIntents, current.OperationID, disposalParams)
				if err != nil {
					return saleReplacementEffects{}, err
				}
				proposed, err := simulateInvestmentReplayTx(ctx, tx, replacementParams.BookID,
					current.AccountID, current.CommodityID, current.CostCommodityID, proposedIntents)
				if err != nil {
					return saleReplacementEffects{}, err
				}
				var historical *InvestmentReplayDisposal
				for index := range proposed.Disposals {
					if proposed.Disposals[index].DecisionID == sourceDecisionID {
						historical = &proposed.Disposals[index]
						break
					}
				}
				if historical == nil {
					return saleReplacementEffects{},
						fmt.Errorf("%w: corrected sale has no simulated allocation", ErrInvalidDisposalParams)
				}
				disposals, err = insertHistoricalSaleDisposalsTx(ctx, tx, disposalParams,
					historical.Allocations, auditEventID)
				if err != nil {
					return saleReplacementEffects{}, err
				}
				decision, err = createDisposalDecisionTx(ctx, tx, replacement, disposalParams, disposals, auditEventID)
				if err != nil {
					return saleReplacementEffects{}, err
				}
			}
			intents, err := investmentReplayIntentsQuery(ctx, tx, replacementParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID, "long")
			if err != nil {
				return saleReplacementEffects{}, err
			}
			projection, err := simulateInvestmentReplayTx(ctx, tx, replacementParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID, intents)
			if err != nil {
				return saleReplacementEffects{}, err
			}
			if err := persistInvestmentReplayProjectionTx(ctx, tx, replacementParams.BookID,
				current.AccountID, current.CommodityID, current.CostCommodityID,
				operationID, auditEventID, replacementParams.ActorUserID, replacementParams.CreatedAt,
				intents, projection); err != nil {
				return saleReplacementEffects{}, err
			}
			if !olderSale {
				disposals, err = disposeLotsWithAuditTx(ctx, tx, disposalParams, auditEventID, false)
				if err != nil {
					return saleReplacementEffects{}, err
				}
				decision, err = createDisposalDecisionTx(ctx, tx, replacement, disposalParams, disposals, auditEventID)
				if err != nil {
					return saleReplacementEffects{}, err
				}
			}
			if err := voidTradePricesForVersionTx(ctx, tx, inverseParams, current.TransactionVersionID, auditEventID); err != nil {
				return saleReplacementEffects{}, err
			}
			return saleReplacementEffects{disposals: disposals, decision: decision}, nil
		}, acceptSource)
	if err != nil {
		return noInverse, noReplacement, nil, noDecision, err
	}
	return journals[0], journals[1], effects.disposals, effects.decision, nil
}

type saleReplacementEffects struct {
	disposals []LotDisposalRecord
	decision  DisposalDecisionRecord
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

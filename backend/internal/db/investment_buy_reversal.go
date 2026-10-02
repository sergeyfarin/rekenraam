package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReverseBuy posts an exact inverse and removes the acquisition from the
// effective long-position replay. Later sales keep their original decisions
// and receive new effective allocations, or the entire command rolls back.
func (r *InvestmentRepository) ReverseBuy(ctx context.Context, params CreateTransactionParams, expected BuyOperationRecord) (TransactionRecord, error) {
	return r.reverseBuy(ctx, params, expected, false)
}

// PreviewBuyReversal runs the reversal writer, dependent replay, price
// retirement and checkpoint invalidation, then rolls back (T-126).
func (r *InvestmentRepository) PreviewBuyReversal(ctx context.Context, params CreateTransactionParams, expected BuyOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseBuy(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseBuy(ctx context.Context, params CreateTransactionParams, expected BuyOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: buy reversal is incomplete", ErrInvalidDisposalParams)
	}
	var current BuyOperationRecord
	guard := func(tx *sql.Tx) error {
		var err error
		current, err = checkBuyOperationForCorrectionTx(ctx, tx, params.BookID, expected)
		return err
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
		if err != nil {
			return struct{}{}, err
		}
		intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID,
			current.AccountID, current.CommodityID, current.CostCommodityID, "long")
		if err != nil {
			return struct{}{}, err
		}
		projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID,
			current.AccountID, current.CommodityID, current.CostCommodityID, intents)
		if err != nil {
			if errors.Is(err, ErrInsufficientLots) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvestmentCorrectionDependency) {
				return struct{}{}, fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)
			}
			return struct{}{}, err
		}
		if err := persistInvestmentReplayProjectionTx(ctx, tx, params.BookID,
			current.AccountID, current.CommodityID, current.CostCommodityID,
			operationID, auditEventID, params.ActorUserID, params.CreatedAt,
			intents, projection); err != nil {
			return struct{}{}, err
		}
		if err := voidTradePricesForVersionTx(ctx, tx, params, current.TransactionVersionID, auditEventID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	}
	var transaction TransactionRecord
	var err error
	if preview {
		transaction, _, err = previewInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect)
	} else {
		transaction, _, err = executeInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect, nil)
	}
	return transaction, err
}

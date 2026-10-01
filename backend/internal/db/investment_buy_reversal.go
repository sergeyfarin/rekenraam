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
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: buy reversal is incomplete", ErrInvalidDisposalParams)
	}
	var current BuyOperationRecord
	transaction, _, err := executeInvestmentWriteWithGuardTx(ctx, r.database, params,
		func(tx *sql.Tx) error {
			var err error
			current, err = checkBuyOperationForCorrectionTx(ctx, tx, params.BookID, expected)
			return err
		},
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
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
		}, nil)
	return transaction, err
}

// SimulateBuyReversal checks dependent long-position intents without a durable
// journal, audit event, revision, or lot change. The write repeats this check.
func (r *InvestmentRepository) SimulateBuyReversal(ctx context.Context, bookID int64, expected BuyOperationRecord) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin buy reversal preview: %w", err)
	}
	defer rollbackTx(ctx, tx)
	current, err := checkBuyOperationForCorrectionTx(ctx, tx, bookID, expected)
	if err != nil {
		return err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, bookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return err
	}
	filtered := make([]InvestmentReplayIntent, 0, len(intents))
	found := false
	for _, intent := range intents {
		if intent.OperationID == current.OperationID && intent.Kind == "opening" {
			found = true
			continue
		}
		filtered = append(filtered, intent)
	}
	if !found {
		return ErrNotFound
	}
	_, err = simulateInvestmentReplayTx(ctx, tx, bookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, filtered)
	if errors.Is(err, ErrInsufficientLots) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvestmentCorrectionDependency) {
		return fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)
	}
	return err
}

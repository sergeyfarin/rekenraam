package db

import (
	"context"
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
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return TransactionRecord{}, fmt.Errorf("begin buy reversal: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()
	current, err := checkBuyOperationForCorrectionTx(ctx, tx, params.BookID, expected, false)
	if err != nil {
		return TransactionRecord{}, err
	}
	transaction, auditEventID, err := createTransactionWithAuditTx(ctx, tx, params)
	if err != nil {
		return TransactionRecord{}, err
	}
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return TransactionRecord{}, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, "long")
	if err != nil {
		return TransactionRecord{}, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID, intents)
	if err != nil {
		if errors.Is(err, ErrInsufficientLots) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvestmentCorrectionDependency) {
			return TransactionRecord{}, fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)
		}
		return TransactionRecord{}, err
	}
	if err := persistInvestmentReplayProjectionTx(ctx, tx, params.BookID,
		current.AccountID, current.CommodityID, current.CostCommodityID,
		operationID, auditEventID, params.ActorUserID, params.CreatedAt,
		intents, projection); err != nil {
		return TransactionRecord{}, err
	}
	if err := voidTradePricesForVersionTx(ctx, tx, params, current.TransactionVersionID, auditEventID); err != nil {
		return TransactionRecord{}, err
	}
	transaction.InvalidatedCheckpointIDs, err = invalidateCreateTransactionCheckpointsTx(ctx, tx, params, auditEventID)
	if err != nil {
		return TransactionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransactionRecord{}, fmt.Errorf("commit buy reversal: %w", err)
	}
	committed = true
	return transaction, nil
}

// SimulateBuyReversal checks dependent long-position intents without a durable
// journal, audit event, revision, or lot change. The write repeats this check.
func (r *InvestmentRepository) SimulateBuyReversal(ctx context.Context, bookID int64, expected BuyOperationRecord) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin buy reversal preview: %w", err)
	}
	defer rollbackTx(ctx, tx)
	current, err := checkBuyOperationForCorrectionTx(ctx, tx, bookID, expected, false)
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

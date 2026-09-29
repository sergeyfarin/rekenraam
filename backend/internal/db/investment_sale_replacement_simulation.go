package db

import (
	"context"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
)

// SimulateSaleReplacement checks a proposed corrected sale at the original
// operation's chronological slot. Later sales are replayed against that new
// quantity and election. The savepoint restores all simulated writes before
// the snapshot closes; the command repeats this check in its write transaction.
func (r *InvestmentRepository) SimulateSaleReplacement(ctx context.Context,
	expected SaleOperationRecord, proposed DisposeLotsParams,
) (InvestmentReplayProjection, error) {
	if proposed.BookID <= 0 ||
		proposed.AccountID != expected.AccountID ||
		proposed.CommodityID != expected.CommodityID ||
		proposed.CostCommodityID != expected.CostCommodityID ||
		proposed.EventDate != expected.EventDate {
		return InvestmentReplayProjection{}, fmt.Errorf("%w: replacement position differs from source sale", ErrInvalidDisposalParams)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("begin sale replacement simulation: %w", err)
	}
	defer rollbackTx(ctx, tx)
	if _, err := checkSaleOperationForCorrectionTx(ctx, tx, proposed.BookID, expected); err != nil {
		return InvestmentReplayProjection{}, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, proposed.BookID,
		expected.AccountID, expected.CommodityID, expected.CostCommodityID, "long")
	if err != nil {
		return InvestmentReplayProjection{}, err
	}
	intents, err = proposedSaleReplayIntents(intents, expected.OperationID, proposed)
	if err != nil {
		return InvestmentReplayProjection{}, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, proposed.BookID,
		expected.AccountID, expected.CommodityID, expected.CostCommodityID, intents)
	if err != nil {
		return InvestmentReplayProjection{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("close sale replacement simulation: %w", err)
	}
	return projection, nil
}

// proposedSaleReplayIntents replaces only source economics. It keeps the
// original decision ID for correlation in this temporary projection; a
// committed replacement receives a fresh immutable decision ID.
func proposedSaleReplayIntents(intents []InvestmentReplayIntent, operationID int64,
	proposed DisposeLotsParams,
) ([]InvestmentReplayIntent, error) {
	if operationID <= 0 || proposed.QuantityValue.Sign() <= 0 || proposed.QuantityScale < 0 ||
		proposed.ProceedsScale < 0 ||
		proposed.CostBasisMethod == "" {
		return nil, fmt.Errorf("%w: replacement disposal is incomplete", ErrInvalidDisposalParams)
	}
	updated := slices.Clone(intents)
	found := false
	for index := range updated {
		if updated[index].OperationID != operationID {
			continue
		}
		if found || updated[index].Kind != "disposal" ||
			updated[index].EventDate != proposed.EventDate {
			return nil, fmt.Errorf("%w: source sale has an ambiguous replay intent", ErrInvalidDisposalParams)
		}
		found = true
		updated[index].QuantityValue = proposed.QuantityValue
		updated[index].QuantityScale = proposed.QuantityScale
		updated[index].AmountValue = exact.New(proposed.ProceedsValue)
		updated[index].AmountScale = proposed.ProceedsScale
		updated[index].CostBasisMethod = proposed.CostBasisMethod
		updated[index].DecisionSource = proposed.DecisionSource
		updated[index].SpecificLots = slices.Clone(proposed.Allocations)
	}
	if !found {
		return nil, ErrNotFound
	}
	return updated, nil
}

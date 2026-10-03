package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
)

// disposeBehindLaterRewriteTx admits a sale or write-off dated before a later
// depletion of the same position (T-117). The ordinary path refuses it,
// because the current projection already reflects that later event. Here the
// new disposal joins the effective intents at its own dated slot — after
// same-day events already entered — and the whole position replays: its
// allocations come from that chronology, and every later decision keeps its
// recorded method, provenance and explicit elections while receiving an
// effective revision. A later decision that can no longer be satisfied is
// named; nothing is written. Gain changes reach the caller's GainImpactPolicy
// because the writer compares effective disposals around this effect.
func disposeBehindLaterRewriteTx(ctx context.Context, tx *sql.Tx, transaction TransactionRecord, params DisposeLotsParams, auditEventID int64) ([]LotDisposalRecord, DisposalDecisionRecord, error) {
	if params.CostBasisMethod == "" {
		params.CostBasisMethod = "fifo"
	}
	if !validCostBasisMethods[params.CostBasisMethod] {
		return nil, DisposalDecisionRecord{}, fmt.Errorf("%w: cost basis method %q is not supported", ErrInvalidDisposalParams, params.CostBasisMethod)
	}
	if params.CostBasisMethod != "specific_lot" && len(params.Allocations) > 0 {
		return nil, DisposalDecisionRecord{}, fmt.Errorf("%w: explicit lot allocations are only permitted for specific_lot cost basis method", ErrInvalidDisposalParams)
	}
	if !isDisposalCalendarDate(params.EventDate) || params.QuantityValue.Sign() <= 0 || params.TransactionID <= 0 {
		return nil, DisposalDecisionRecord{}, ErrInvalidDisposalParams
	}
	costCommodityID, err := historicalDisposalCostCommodityTx(ctx, tx, params)
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	params.CostCommodityID = costCommodityID
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, "long")
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	// The proposed decision has no ID yet; zero marks it in the projection.
	proposed := append(slices.Clone(intents), InvestmentReplayIntent{
		OperationID: operationID, OrderOperationID: operationID, EffectSeq: 1,
		EventDate: params.EventDate, Kind: "disposal",
		QuantityValue: params.QuantityValue, QuantityScale: params.QuantityScale,
		AmountValue: exact.New(params.ProceedsValue), AmountScale: params.ProceedsScale,
		CostBasisMethod: params.CostBasisMethod, DecisionSource: params.DecisionSource,
		SpecificLots: slices.Clone(params.Allocations), TransactionID: transaction.ID,
		AuditEventID: auditEventID, CreatedByUserID: params.ActorUserID, CreatedAt: params.CreatedAt,
	})
	simulated, err := simulateInvestmentReplayTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, proposed)
	if err != nil {
		// The new disposal's own failure (too few lots, an ineligible elected
		// lot, a method switch) is reported as itself, not as a dependency.
		var dependency *InvestmentReplayDependencyError
		if errors.As(err, &dependency) && dependency.OperationID == operationID {
			return nil, DisposalDecisionRecord{}, dependency.Cause
		}
		return nil, DisposalDecisionRecord{}, err
	}
	var allocations []InvestmentReplayAllocation
	for _, disposal := range simulated.Disposals {
		if disposal.DecisionID == 0 {
			allocations = disposal.Allocations
		}
	}
	if len(allocations) == 0 {
		return nil, DisposalDecisionRecord{}, fmt.Errorf("%w: backdated disposal has no simulated allocation", ErrInvalidDisposalParams)
	}
	disposals, err := insertHistoricalSaleDisposalsTx(ctx, tx, params, allocations, auditEventID)
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	decision, err := createDisposalDecisionTx(ctx, tx, transaction, params, disposals, auditEventID)
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	// Replay again from the committed intents, now including the new decision,
	// and install the projection and later decisions' effective revisions.
	intents, err = investmentReplayIntentsQuery(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, "long")
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, intents)
	if err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	if err := persistInvestmentReplayProjectionTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, operationID, auditEventID, params.ActorUserID,
		params.CreatedAt, intents, projection); err != nil {
		return nil, DisposalDecisionRecord{}, err
	}
	return disposals, decision, nil
}

// historicalDisposalCostCommodityTx resolves the cost currency of a backdated
// disposal from the lots held on its date, not from today's open lots: a lot
// a later sale closed was still held then.
func historicalDisposalCostCommodityTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams) (int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT cost_commodity_id FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND position_side = 'long'
			AND opened_on <= ? AND (? = 0 OR cost_commodity_id = ?)
		ORDER BY cost_commodity_id`, params.BookID, params.AccountID, params.CommodityID,
		params.EventDate, params.CostCommodityID, params.CostCommodityID)
	if err != nil {
		return 0, fmt.Errorf("read backdated disposal cost commodities: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scan backdated disposal cost commodity: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate backdated disposal cost commodities: %w", err)
	}
	switch len(ids) {
	case 0:
		return 0, ErrInsufficientLots
	case 1:
		return ids[0], nil
	default:
		return 0, fmt.Errorf("%w: cost commodity is required when a position has lots in multiple currencies", ErrInvalidDisposalParams)
	}
}

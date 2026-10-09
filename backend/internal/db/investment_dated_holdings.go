package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// DatedHolding is one long position as a command entered for asOf would find
// it (#166): after every effective event dated on or before asOf, because a
// new entry takes the last same-day slot. A holding a later sale closed is
// still listed with the units it held then.
type DatedHolding struct {
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	QuantityValue   exact.Coefficient
	QuantityScale   int
	// MethodFamily is the position's current method-family lock, which the
	// transfer writers apply at commit; empty when none is held today.
	MethodFamily string
	Lots         []DatedHoldingLot
}

// DatedHoldingLot is one lot open at the slot with its remaining quantity
// and basis then. Unknown basis leaves the amount unused.
type DatedHoldingLot struct {
	LotID                   int64
	OpenedOn                string
	QuantityValue           exact.Coefficient
	QuantityScale           int
	BasisKnowledge          string
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
}

// DatedLongHoldings replays every long position up to asOf in one rolled-back
// read, so a selector makes one request rather than one per holding. It is
// discovery only: each command's writer rechecks availability, replay,
// gain acknowledgement and checkpoints at commit.
func (r *InvestmentRepository) DatedLongHoldings(ctx context.Context, bookID int64, asOf string) ([]DatedHolding, error) {
	if !isDisposalCalendarDate(asOf) {
		return nil, fmt.Errorf("%w: dated holdings need a calendar date", ErrInvalidDisposalParams)
	}
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin dated holdings read: %w", err)
	}
	defer rollbackTx(ctx, tx)
	rows, err := tx.QueryContext(ctx, `
		SELECT l.account_id, l.commodity_id, l.cost_commodity_id, MAX(l.operation_id IS NULL),
			COALESCE((SELECT state.method_family FROM investment_position_basis_state state
				WHERE state.book_id = l.book_id AND state.account_id = l.account_id
					AND state.commodity_id = l.commodity_id AND state.cost_commodity_id = l.cost_commodity_id
					AND state.position_side = 'long'), '')
		FROM investment_lots l
		WHERE l.book_id = ? AND l.position_side = 'long' AND l.opened_on <= ?
		GROUP BY l.account_id, l.commodity_id, l.cost_commodity_id
		ORDER BY l.account_id, l.commodity_id, l.cost_commodity_id`, bookID, asOf)
	if err != nil {
		return nil, fmt.Errorf("read dated holding positions: %w", err)
	}
	type position struct {
		key       investmentReplayPositionKey
		unmodeled bool
		family    string
	}
	var positions []position
	for rows.Next() {
		var p position
		if err := rows.Scan(&p.key.accountID, &p.key.commodityID, &p.key.costCommodityID, &p.unmodeled, &p.family); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan dated holding position: %w", err)
		}
		positions = append(positions, p)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("iterate dated holding positions: %w", err)
	}
	holdings := make([]DatedHolding, 0, len(positions))
	for _, p := range positions {
		// A lot without an opening fact cannot be replayed (T-133); its
		// position is not offered for dated entry.
		if p.unmodeled {
			continue
		}
		holding, ok, err := datedLongHoldingTx(ctx, tx, bookID, p.key, asOf)
		if err != nil {
			return nil, err
		}
		if ok {
			holding.MethodFamily = p.family
			holdings = append(holdings, holding)
		}
	}
	return holdings, nil
}

func datedLongHoldingTx(ctx context.Context, tx *sql.Tx, bookID int64, key investmentReplayPositionKey, asOf string) (DatedHolding, bool, error) {
	intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, key.accountID, key.commodityID, key.costCommodityID, PositionSideLong)
	if err != nil {
		return DatedHolding{}, false, err
	}
	before := make([]InvestmentReplayIntent, 0, len(intents))
	opened := make(map[int64]string)
	for _, intent := range intents {
		if intent.EventDate > asOf {
			continue
		}
		before = append(before, intent)
		if intent.Kind == "opening" {
			opened[intent.LotID] = intent.EventDate
		}
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, bookID, key.accountID, key.commodityID, key.costCommodityID, before)
	if err != nil {
		return DatedHolding{}, false, fmt.Errorf("replay dated holding %d/%d/%d: %w", key.accountID, key.commodityID, key.costCommodityID, err)
	}
	holding := DatedHolding{AccountID: key.accountID, CommodityID: key.commodityID, CostCommodityID: key.costCommodityID}
	total := exact.NewScaledInt()
	for _, lot := range projection.Lots {
		if lot.RemainingQuantityValue.Sign() <= 0 || opened[lot.LotID] == "" {
			continue
		}
		total.AddCoefficient(lot.RemainingQuantityValue, lot.RemainingQuantityScale)
		holding.Lots = append(holding.Lots, DatedHoldingLot{LotID: lot.LotID, OpenedOn: opened[lot.LotID],
			QuantityValue: lot.RemainingQuantityValue, QuantityScale: lot.RemainingQuantityScale,
			BasisKnowledge:          normalizedBasisKnowledge(lot.BasisKnowledge),
			RemainingCostBasisValue: lot.RemainingCostBasisValue, RemainingCostBasisScale: lot.RemainingCostBasisScale})
	}
	if total.Sign() <= 0 {
		return DatedHolding{}, false, nil
	}
	quantity, err := total.Coefficient()
	if err != nil {
		return DatedHolding{}, false, err
	}
	holding.QuantityValue, holding.QuantityScale = quantity, total.Scale()
	return holding, true, nil
}

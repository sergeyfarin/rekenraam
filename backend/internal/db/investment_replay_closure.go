package db

import (
	"cmp"
	"context"
	"fmt"
	"slices"
)

// InvestmentReplayPosition is one long position that replay rebuilds, with
// the earliest date from which its effective intents may differ. Intents
// before AffectedFrom are unchanged, so their replay output is too.
type InvestmentReplayPosition struct {
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	AffectedFrom    string
}

// InvestmentReplayClosure returns the affected-position dependency closure
// that ADR 0013's cross-position replay refinement (T-124) selects instead of
// a whole-book rebuild. Starting from the positions a command changes, it
// follows internal-transfer lot links from source to destination: a source
// whose replay may change from date D can change the carried basis of every
// transfer it makes on or after D, which changes the destination's opening
// from that transfer date. The walk repeats to a fixed point, so chains and
// cycles (A→B, later B→A) settle on each position's earliest affected date.
//
// Every recorded transfer is followed, including corrected and reversed ones,
// so the closure covers the union of the edges before and after a command
// that adds, replaces or removes a transfer. Including an unchanged position
// costs only a no-op replay; omitting a changed one would leave stale state.
// Positions with no transfer path to a seed are excluded, because none of
// their replay inputs can differ.
//
// This is the scope primitive only. Replaying the closure as one merged dated
// stream, and the guarded bridge adjustment at the book boundary, are follow-up
// work; until then the replay itself still refuses a changed carried basis.
func (r *InvestmentRepository) InvestmentReplayClosure(ctx context.Context, bookID int64, seeds []InvestmentReplayPosition) ([]InvestmentReplayPosition, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replay closure snapshot: %w", err)
	}
	defer rollbackTx(ctx, tx)
	closure, err := investmentReplayClosureQuery(ctx, tx, bookID, seeds)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("close replay closure snapshot: %w", err)
	}
	return closure, nil
}

type investmentReplayPositionKey struct {
	accountID, commodityID, costCommodityID int64
}

type investmentReplayTransferEdge struct {
	effectiveOn string
	destination investmentReplayPositionKey
}

func investmentReplayClosureQuery(ctx context.Context, reader queryer, bookID int64, seeds []InvestmentReplayPosition) ([]InvestmentReplayPosition, error) {
	if bookID <= 0 {
		return nil, fmt.Errorf("%w: replay closure requires a book", ErrInvalidDisposalParams)
	}
	affected := make(map[investmentReplayPositionKey]string, len(seeds))
	var pending []investmentReplayPositionKey
	visit := func(key investmentReplayPositionKey, from string) {
		if current, seen := affected[key]; seen && current <= from {
			return
		}
		affected[key] = from
		pending = append(pending, key)
	}
	for _, seed := range seeds {
		if seed.AccountID <= 0 || seed.CommodityID <= 0 || seed.CostCommodityID <= 0 || !isDisposalCalendarDate(seed.AffectedFrom) {
			return nil, fmt.Errorf("%w: replay closure seed needs a position and date", ErrInvalidDisposalParams)
		}
		visit(investmentReplayPositionKey{seed.AccountID, seed.CommodityID, seed.CostCommodityID}, seed.AffectedFrom)
	}
	if len(pending) == 0 {
		return nil, nil
	}

	// The transfer graph is small next to the ledger; read it once rather
	// than once per visited position.
	rows, err := reader.QueryContext(ctx, `
		SELECT DISTINCT f.source_account_id, f.commodity_id, x.cost_commodity_id,
			f.destination_account_id, f.effective_on
		FROM investment_transfer_facts f
		JOIN investment_transfer_lot_links x ON x.operation_id = f.operation_id
		WHERE f.book_id = ? AND f.transfer_kind = 'internal' AND x.cost_commodity_id IS NOT NULL
		ORDER BY f.effective_on, f.source_account_id, f.destination_account_id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read replay closure transfers: %w", err)
	}
	edges := make(map[investmentReplayPositionKey][]investmentReplayTransferEdge)
	for rows.Next() {
		var source investmentReplayPositionKey
		var edge investmentReplayTransferEdge
		if err := rows.Scan(&source.accountID, &source.commodityID, &source.costCommodityID,
			&edge.destination.accountID, &edge.effectiveOn); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan replay closure transfer: %w", err)
		}
		edge.destination.commodityID, edge.destination.costCommodityID = source.commodityID, source.costCommodityID
		edges[source] = append(edges[source], edge)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate replay closure transfers: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close replay closure transfers: %w", err)
	}

	for len(pending) > 0 {
		key := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		from := affected[key]
		for _, edge := range edges[key] {
			// A same-day transfer may precede the changed intent in its slot
			// order; including it is conservative and costs a no-op replay.
			if edge.effectiveOn >= from {
				visit(edge.destination, edge.effectiveOn)
			}
		}
	}

	closure := make([]InvestmentReplayPosition, 0, len(affected))
	for key, from := range affected {
		closure = append(closure, InvestmentReplayPosition{AccountID: key.accountID,
			CommodityID: key.commodityID, CostCommodityID: key.costCommodityID, AffectedFrom: from})
	}
	slices.SortFunc(closure, func(a, b InvestmentReplayPosition) int {
		return cmp.Or(cmp.Compare(a.AffectedFrom, b.AffectedFrom), cmp.Compare(a.AccountID, b.AccountID),
			cmp.Compare(a.CommodityID, b.CommodityID), cmp.Compare(a.CostCommodityID, b.CostCommodityID))
	})
	return closure, nil
}

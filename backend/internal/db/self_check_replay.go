package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Replay equivalence (T-134). Commands replay only the positions they touch
// (ADR 0013 closure), so a projection or revision that is internally
// consistent but no longer what the recorded intents produce — for example
// after a writer bug that skipped a dependent replay — passes every other
// check. This verifier replays each long position from its effective intents
// inside a rolled-back savepoint and compares the result with what is stored.
// Checking every position against the stored cross-position inputs (the
// effective transfer links) verifies the whole book's fixed point: a source
// whose link is stale produces a transfer revision, and a destination
// replayed from a stale link disagrees with its own stored state.

// Replay-equivalence mismatch kinds.
const (
	ReplayMismatchLotState     = "lot_state"
	ReplayMismatchDisposal     = "disposal"
	ReplayMismatchSplit        = "split"
	ReplayMismatchTransferLink = "transfer_link"
	ReplayMismatchMethodFamily = "method_family"
	ReplayMismatchRefused      = "replay_refused"
)

// InvestmentReplayMismatch names one stored fact that the position's replay
// does not reproduce. ReferenceID is the lot, disposal decision or operation.
type InvestmentReplayMismatch struct {
	AccountID       int64
	CommodityID     int64
	CostCommodityID int64
	Kind            string
	ReferenceID     int64
}

// InvestmentReplayEquivalence is the verifier's whole result. Skipped counts
// positions with a lot that has no opening operation (T-133); they cannot be
// replayed and are not reported as mismatches.
type InvestmentReplayEquivalence struct {
	Positions  int
	Skipped    int
	Mismatches []InvestmentReplayMismatch
}

// InvestmentReplayEquivalence replays every long position of the book. Each
// position runs in its own write transaction that is always rolled back, so
// the write lock is held for one position at a time and nothing is stored.
func (r *SelfCheckRepository) InvestmentReplayEquivalence(ctx context.Context, bookID int64) (InvestmentReplayEquivalence, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT account_id, commodity_id, cost_commodity_id,
			MAX(operation_id IS NULL)
		FROM investment_lots WHERE book_id = ? AND position_side = 'long'
		GROUP BY account_id, commodity_id, cost_commodity_id
		ORDER BY account_id, commodity_id, cost_commodity_id`, bookID)
	if err != nil {
		return InvestmentReplayEquivalence{}, fmt.Errorf("read replay-equivalence positions: %w", err)
	}
	type position struct {
		key       investmentReplayPositionKey
		unmodeled bool
	}
	var positions []position
	for rows.Next() {
		var p position
		if err := rows.Scan(&p.key.accountID, &p.key.commodityID, &p.key.costCommodityID, &p.unmodeled); err != nil {
			rows.Close()
			return InvestmentReplayEquivalence{}, fmt.Errorf("scan replay-equivalence position: %w", err)
		}
		positions = append(positions, p)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return InvestmentReplayEquivalence{}, fmt.Errorf("iterate replay-equivalence positions: %w", err)
	}
	result := InvestmentReplayEquivalence{Positions: len(positions)}
	for _, p := range positions {
		if p.unmodeled {
			result.Skipped++
			continue
		}
		mismatches, err := r.positionReplayEquivalence(ctx, bookID, p.key)
		if err != nil {
			return InvestmentReplayEquivalence{}, err
		}
		result.Mismatches = append(result.Mismatches, mismatches...)
	}
	return result, nil
}

func (r *SelfCheckRepository) positionReplayEquivalence(ctx context.Context, bookID int64, key investmentReplayPositionKey) ([]InvestmentReplayMismatch, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin replay-equivalence position: %w", err)
	}
	defer rollbackTx(ctx, tx)
	mismatch := func(kind string, referenceID int64) InvestmentReplayMismatch {
		return InvestmentReplayMismatch{AccountID: key.accountID, CommodityID: key.commodityID,
			CostCommodityID: key.costCommodityID, Kind: kind, ReferenceID: referenceID}
	}
	stored, err := storedInvestmentProjectionQuery(ctx, tx, bookID, key)
	if err != nil {
		return nil, err
	}
	intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, key.accountID, key.commodityID, key.costCommodityID, "long")
	var projection InvestmentReplayProjection
	if err == nil {
		projection, err = simulateInvestmentReplayTx(ctx, tx, bookID, key.accountID, key.commodityID, key.costCommodityID, intents)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		// History the stored projection claims to describe cannot be read back
		// into intents or replayed. That is a finding about the book, named by
		// its dependent operation when replay knows it; the other checks name
		// the damaged rows.
		var dependency *InvestmentReplayDependencyError
		errors.As(err, &dependency)
		var operationID int64
		if dependency != nil {
			operationID = dependency.OperationID
		}
		return []InvestmentReplayMismatch{mismatch(ReplayMismatchRefused, operationID)}, nil
	}

	var found []InvestmentReplayMismatch
	replayedLots := make(map[int64]InvestmentReplayLotState, len(projection.Lots))
	for _, lot := range projection.Lots {
		replayedLots[lot.LotID] = lot
	}
	for _, lot := range stored.lots {
		replayed, ok := replayedLots[lot.lotID]
		if !ok || lot.basis == nil || replayed.Status != lot.status ||
			exact.ScaledIntFromCoefficient(replayed.RemainingQuantityValue, replayed.RemainingQuantityScale).Cmp(lot.quantity) != 0 ||
			exact.ScaledIntFromInt64(replayed.RemainingCostBasisValue, replayed.RemainingCostBasisScale).Cmp(lot.basis) != 0 {
			found = append(found, mismatch(ReplayMismatchLotState, lot.lotID))
		}
		delete(replayedLots, lot.lotID)
	}
	for lotID := range replayedLots {
		found = append(found, mismatch(ReplayMismatchLotState, lotID))
	}

	replayedDisposals := make(map[int64][]InvestmentReplayAllocation, len(projection.Disposals))
	for _, disposal := range projection.Disposals {
		replayedDisposals[disposal.DecisionID] = disposal.Allocations
	}
	for _, decisionID := range stored.decisionOrder {
		if !sameReplayAllocations(stored.allocations[decisionID], replayedDisposals[decisionID]) {
			found = append(found, mismatch(ReplayMismatchDisposal, decisionID))
		}
		delete(replayedDisposals, decisionID)
	}
	for decisionID := range replayedDisposals {
		found = append(found, mismatch(ReplayMismatchDisposal, decisionID))
	}

	for _, split := range projection.Splits {
		effects, _, _, err := effectiveSplitEffectsQuery(ctx, tx, bookID, split.OperationID, key.costCommodityID)
		if err != nil {
			return nil, err
		}
		if canonicalSplitEffects(effects) != canonicalSplitEffects(split.Effects) {
			found = append(found, mismatch(ReplayMismatchSplit, split.OperationID))
		}
	}
	reported := make(map[int64]bool)
	for _, revision := range projection.TransferRevisions {
		if !reported[revision.OperationID] {
			reported[revision.OperationID] = true
			found = append(found, mismatch(ReplayMismatchTransferLink, revision.OperationID))
		}
	}
	if projection.MethodFamily != stored.methodFamily {
		found = append(found, mismatch(ReplayMismatchMethodFamily, 0))
	}
	return found, nil
}

// storedReplayLot and storedReplayAllocation hold stored amounts as scaled
// values, so damaged or out-of-range rows compare unequal instead of failing
// the run. A nil basis is unknown or unreadable.
type storedReplayLot struct {
	lotID    int64
	status   string
	quantity *exact.ScaledInt
	basis    *exact.ScaledInt
}

type storedReplayAllocation struct {
	lotID                     int64
	quantity, basis, proceeds *exact.ScaledInt
}

type storedInvestmentProjection struct {
	lots          []storedReplayLot
	decisionOrder []int64
	allocations   map[int64][]storedReplayAllocation
	methodFamily  string
}

// storedInvestmentProjectionQuery reads what a position currently claims:
// its lot projection, each effective disposal's current allocation set (the
// latest revision, else the original snapshot) and its method-family lock.
func storedInvestmentProjectionQuery(ctx context.Context, tx *sql.Tx, bookID int64, key investmentReplayPositionKey) (storedInvestmentProjection, error) {
	stored := storedInvestmentProjection{allocations: make(map[int64][]storedReplayAllocation)}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, status, remaining_quantity_value, remaining_quantity_scale,
			remaining_cost_basis_value, remaining_cost_basis_scale, basis_knowledge = 'known'
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'
		ORDER BY id`, bookID, key.accountID, key.commodityID, key.costCommodityID)
	if err != nil {
		return storedInvestmentProjection{}, fmt.Errorf("read stored lot projection: %w", err)
	}
	for rows.Next() {
		var lot storedReplayLot
		var status, quantity, basis sql.NullString
		var quantityScale, basisScale sql.NullInt64
		var known sql.NullBool
		if err := rows.Scan(&lot.lotID, &status, &quantity, &quantityScale, &basis, &basisScale, &known); err != nil {
			rows.Close()
			return storedInvestmentProjection{}, fmt.Errorf("scan stored lot projection: %w", err)
		}
		lot.status = status.String
		lot.quantity = storedScaled(quantity, quantityScale)
		if known.Bool {
			lot.basis = storedScaled(basis, basisScale)
		}
		if lot.quantity == nil {
			lot.status = ""
			lot.quantity = exact.NewScaledInt()
		}
		stored.lots = append(stored.lots, lot)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return storedInvestmentProjection{}, fmt.Errorf("iterate stored lot projection: %w", err)
	}

	rows, err = tx.QueryContext(ctx, `
		SELECT d.id, a.lot_id, a.quantity_value, a.quantity_scale, a.cost_basis_value, a.cost_basis_scale,
			a.proceeds_value, a.proceeds_scale
		FROM investment_disposal_decisions d
		JOIN effective_investment_operations o ON o.id = d.operation_id
		LEFT JOIN latest_investment_disposal_revisions revision ON revision.decision_id = d.id
		JOIN (
			SELECT decision_id, 0 AS revision_id, allocation_seq, lot_id, quantity_value, quantity_scale,
				cost_basis_value, cost_basis_scale, proceeds_value, proceeds_scale
			FROM investment_disposal_allocations
			UNION ALL
			SELECT r.decision_id, r.id, a.allocation_seq, a.lot_id, a.quantity_value, a.quantity_scale,
				a.cost_basis_value, a.cost_basis_scale, a.proceeds_value, a.proceeds_scale
			FROM investment_disposal_revision_allocations a JOIN investment_disposal_revisions r ON r.id = a.revision_id
		) a ON a.decision_id = d.id AND a.revision_id = COALESCE(revision.id, 0)
		WHERE d.book_id = ? AND d.account_id = ? AND d.commodity_id = ? AND d.cost_commodity_id = ?
			AND d.position_side = 'long'
		ORDER BY d.id, a.allocation_seq`, bookID, key.accountID, key.commodityID, key.costCommodityID)
	if err != nil {
		return storedInvestmentProjection{}, fmt.Errorf("read stored disposal allocations: %w", err)
	}
	for rows.Next() {
		var decisionID int64
		var allocation storedReplayAllocation
		var quantity, basis, proceeds sql.NullString
		var quantityScale, basisScale, proceedsScale sql.NullInt64
		if err := rows.Scan(&decisionID, &allocation.lotID, &quantity, &quantityScale,
			&basis, &basisScale, &proceeds, &proceedsScale); err != nil {
			rows.Close()
			return storedInvestmentProjection{}, fmt.Errorf("scan stored disposal allocation: %w", err)
		}
		allocation.quantity = storedScaled(quantity, quantityScale)
		allocation.basis = storedScaled(basis, basisScale)
		allocation.proceeds = storedScaled(proceeds, proceedsScale)
		if _, seen := stored.allocations[decisionID]; !seen {
			stored.decisionOrder = append(stored.decisionOrder, decisionID)
		}
		stored.allocations[decisionID] = append(stored.allocations[decisionID], allocation)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return storedInvestmentProjection{}, fmt.Errorf("iterate stored disposal allocations: %w", err)
	}

	err = tx.QueryRowContext(ctx, `SELECT method_family FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'`,
		bookID, key.accountID, key.commodityID, key.costCommodityID).Scan(&stored.methodFamily)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return storedInvestmentProjection{}, fmt.Errorf("read stored method-family state: %w", err)
	}
	return stored, nil
}

// storedScaled reads a stored coefficient, or nil when it is missing or not
// a canonical exact value.
func storedScaled(value sql.NullString, scale sql.NullInt64) *exact.ScaledInt {
	if !value.Valid || !scale.Valid {
		return nil
	}
	parsed, err := exact.Parse(value.String)
	if err != nil {
		return nil
	}
	return exact.ScaledIntFromCoefficient(parsed, int(scale.Int64))
}

// sameReplayAllocations compares two allocation sets in order, exactly and
// scale-aware: lot, quantity, basis and proceeds.
func sameReplayAllocations(stored []storedReplayAllocation, replayed []InvestmentReplayAllocation) bool {
	if len(stored) != len(replayed) {
		return false
	}
	for index := range stored {
		a, b := stored[index], replayed[index]
		if a.lotID != b.LotID || a.quantity == nil || a.basis == nil || a.proceeds == nil ||
			a.quantity.Cmp(exact.ScaledIntFromCoefficient(b.QuantityValue, b.QuantityScale)) != 0 ||
			a.basis.Cmp(exact.ScaledIntFromInt64(b.CostBasisValue, b.CostBasisScale)) != 0 ||
			a.proceeds.Cmp(exact.ScaledIntFromInt64(b.ProceedsValue, b.ProceedsScale)) != 0 {
			return false
		}
	}
	return true
}

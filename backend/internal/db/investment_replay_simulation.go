package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
)

// InvestmentReplayProjection is a proposed effective position state. It is
// computed inside a savepoint and returned after that savepoint is rolled
// back; simulated lot-event IDs are never exposed as durable records.
type InvestmentReplayProjection struct {
	Lots         []InvestmentReplayLotState
	Disposals    []InvestmentReplayDisposal
	Splits       []InvestmentReplaySplit
	MethodFamily string
	// TransferRevisions are internal-transfer links whose replayed depletion
	// carries a different basis than their effective amount. Persisting the
	// projection appends them and replays each destination (T-132).
	TransferRevisions []InvestmentReplayTransferRevision
}

// InvestmentReplayTransferRevision is one link's replayed depletion: the
// source lot it now takes from and the basis it carries.
type InvestmentReplayTransferRevision struct {
	OperationID    int64
	LinkSeq        int
	SourceLotID    int64
	CostBasisValue int64
	CostBasisScale int
}

// InvestmentReplaySplit is a split's per-lot effect at its replay slot.
// Subject marks the split the current command is creating.
type InvestmentReplaySplit struct {
	OperationID int64
	Subject     bool
	Effects     []SplitLotEffect
}

type InvestmentReplayLotState struct {
	LotID                   int64
	Status                  string
	RemainingQuantityValue  exact.Coefficient
	RemainingQuantityScale  int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
}

type InvestmentReplayDisposal struct {
	DecisionID  int64
	Allocations []InvestmentReplayAllocation
}

type InvestmentReplayAllocation struct {
	LotID          int64
	QuantityValue  exact.Coefficient
	QuantityScale  int
	CostBasisValue int64
	CostBasisScale int
	ProceedsValue  int64
	ProceedsScale  int
}

// InvestmentReplayDependencyError identifies the later decision that a
// corrected opening can no longer satisfy. Callers surface this as a named
// conflict while the enclosing write rolls back every attempted effect.
type InvestmentReplayDependencyError struct {
	OperationID int64
	DecisionID  int64
	Cause       error
}

func (e *InvestmentReplayDependencyError) Error() string {
	return fmt.Sprintf("replay dependent operation %d decision %d: %v", e.OperationID, e.DecisionID, e.Cause)
}

func (e *InvestmentReplayDependencyError) Unwrap() error { return e.Cause }

// simulateInvestmentReplayTx reuses the posted disposal algorithm against a
// temporary projection. Original lot events and decisions remain untouched.
// The caller may later persist the returned state and allocation revision in
// its own write transaction, after dependency and reconciliation checks.
func simulateInvestmentReplayTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64, intents []InvestmentReplayIntent) (InvestmentReplayProjection, error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT investment_replay_simulation`); err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("start investment replay simulation: %w", err)
	}
	projection, simulationErr := runInvestmentReplayTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, intents)
	_, rollbackErr := tx.ExecContext(ctx, `ROLLBACK TO investment_replay_simulation`)
	if rollbackErr != nil {
		abortErr := tx.Rollback()
		return InvestmentReplayProjection{}, errors.Join(simulationErr,
			fmt.Errorf("restore projection after investment replay: %w", rollbackErr), abortErr)
	}
	_, releaseErr := tx.ExecContext(ctx, `RELEASE investment_replay_simulation`)
	if releaseErr != nil {
		abortErr := tx.Rollback()
		return InvestmentReplayProjection{}, errors.Join(simulationErr,
			fmt.Errorf("release investment replay savepoint: %w", releaseErr), abortErr)
	}
	if simulationErr != nil {
		return InvestmentReplayProjection{}, simulationErr
	}
	return projection, nil
}

func runInvestmentReplayTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64, intents []InvestmentReplayIntent) (InvestmentReplayProjection, error) {
	if bookID <= 0 || accountID <= 0 || commodityID <= 0 || costCommodityID <= 0 {
		return InvestmentReplayProjection{}, fmt.Errorf("%w: replay position key is incomplete", ErrInvalidDisposalParams)
	}
	var unmodeledLotID int64
	err := tx.QueryRowContext(ctx, `
		SELECT l.id FROM investment_lots l
		WHERE l.book_id = ? AND l.account_id = ? AND l.commodity_id = ?
			AND l.cost_commodity_id = ? AND l.position_side = 'long' AND l.operation_id IS NULL
		LIMIT 1`, bookID, accountID, commodityID, costCommodityID).Scan(&unmodeledLotID)
	if err == nil {
		return InvestmentReplayProjection{}, fmt.Errorf("%w: lot %d lacks an immutable opening fact", ErrInvalidDisposalParams, unmodeledLotID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return InvestmentReplayProjection{}, fmt.Errorf("check replay opening coverage: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_state
		(lot_id, book_id, status, remaining_quantity_value, remaining_quantity_scale,
		remaining_cost_basis_value, remaining_cost_basis_scale, updated_at, updated_by_user_id, updated_audit_event_id)
		SELECT id, book_id, 'closed', '0', quantity_scale, '0', cost_basis_scale,
		created_at, created_by_user_id, created_audit_event_id FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = 'long'
		ON CONFLICT(lot_id) DO UPDATE SET basis_knowledge = 'known', status = 'closed', remaining_quantity_value = '0',
		remaining_quantity_scale = excluded.remaining_quantity_scale, remaining_cost_basis_value = '0',
		remaining_cost_basis_scale = excluded.remaining_cost_basis_scale`, bookID, accountID, commodityID, costCommodityID); err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("reset replay lot projection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = 'long'`,
		bookID, accountID, commodityID, costCommodityID); err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("reset replay basis-method state: %w", err)
	}
	projection := InvestmentReplayProjection{}
	ordered := slices.Clone(intents)
	sortInvestmentReplayIntents(ordered)
	for _, intent := range ordered {
		switch intent.Kind {
		case "opening":
			basis, err := exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale).Int64()
			if err != nil || basis < 0 || intent.QuantityValue.Sign() <= 0 {
				return InvestmentReplayProjection{}, fmt.Errorf("%w: replay opening lot %d has invalid quantity or basis", ErrInvestmentBasisRange, intent.LotID)
			}
			result, err := tx.ExecContext(ctx, `
				UPDATE investment_lot_state SET basis_knowledge = 'known', status = 'open',
					remaining_quantity_value = ?, remaining_quantity_scale = ?,
					remaining_cost_basis_value = ?, remaining_cost_basis_scale = ?
				WHERE lot_id IN (SELECT id FROM investment_lots WHERE id = ? AND book_id = ? AND account_id = ? AND commodity_id = ?
					AND cost_commodity_id = ? AND position_side = 'long' AND opened_on = ?)`, intent.QuantityValue, intent.QuantityScale, basis, intent.AmountScale,
				intent.LotID, bookID, accountID, commodityID, costCommodityID, intent.EventDate)
			if err != nil {
				return InvestmentReplayProjection{}, fmt.Errorf("activate replay opening lot %d: %w", intent.LotID, err)
			}
			changed, err := result.RowsAffected()
			if err != nil || changed != 1 {
				return InvestmentReplayProjection{}, fmt.Errorf("%w: replay opening lot %d does not match this position and date", ErrInvalidDisposalParams, intent.LotID)
			}
			if err := requirePositionBasisRangeTx(ctx, tx, bookID, accountID, commodityID, costCommodityID); err != nil {
				return InvestmentReplayProjection{}, fmt.Errorf("replay opening lot %d: %w", intent.LotID, err)
			}
		case "disposal":
			params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
				CommodityID: commodityID, CostCommodityID: costCommodityID,
				ProceedsScale: intent.AmountScale,
				TransactionID: intent.TransactionID, EventDate: intent.EventDate,
				QuantityValue: intent.QuantityValue, QuantityScale: intent.QuantityScale,
				CostBasisMethod: intent.CostBasisMethod, Allocations: intent.SpecificLots,
				CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID,
				MetadataJSON: "{}"}
			if !intent.AmountValue.BigInt().IsInt64() {
				return InvestmentReplayProjection{}, ErrInvestmentBasisRange
			}
			params.ProceedsValue = intent.AmountValue.BigInt().Int64()
			allocations, err := disposeLotsWithAuditTx(ctx, tx, params, intent.AuditEventID, true)
			if err != nil {
				return InvestmentReplayProjection{}, &InvestmentReplayDependencyError{
					OperationID: intent.OperationID, DecisionID: intent.DecisionID, Cause: err,
				}
			}
			disposal := InvestmentReplayDisposal{DecisionID: intent.DecisionID}
			for _, allocation := range allocations {
				disposal.Allocations = append(disposal.Allocations, InvestmentReplayAllocation{
					LotID: allocation.LotID, QuantityValue: allocation.QuantityValue,
					QuantityScale: allocation.QuantityScale, CostBasisValue: allocation.CostBasisValue,
					CostBasisScale: allocation.CostBasisScale, ProceedsValue: allocation.ProceedsValue,
					ProceedsScale: allocation.ProceedsScale,
				})
			}
			projection.Disposals = append(projection.Disposals, disposal)
		case "transfer_out":
			params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
				CommodityID: commodityID, CostCommodityID: costCommodityID,
				TransactionID: intent.TransactionID, EventDate: intent.EventDate,
				EventKind: "transfer_out", MetadataJSON: "{}",
				CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID}
			allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
			if err != nil {
				return InvestmentReplayProjection{}, err
			}
			moved, err := disposeLotTx(ctx, tx, params, intent.LotID,
				intent.QuantityValue, intent.QuantityScale, intent.AuditEventID, allocationScale)
			if err != nil {
				return InvestmentReplayProjection{}, &InvestmentReplayDependencyError{
					OperationID: intent.OperationID, Cause: fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)}
			}
			// The quantity is fixed by the transfer; the basis it carries, and
			// the successor lot of a corrected acquisition, follow history.
			if moved.LotID != intent.RecordedLotID || exact.ScaledIntFromInt64(moved.CostBasisValue, moved.CostBasisScale).Cmp(
				exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale)) != 0 {
				projection.TransferRevisions = append(projection.TransferRevisions, InvestmentReplayTransferRevision{
					OperationID: intent.OperationID, LinkSeq: intent.LinkSeq, SourceLotID: moved.LotID,
					CostBasisValue: moved.CostBasisValue, CostBasisScale: moved.CostBasisScale})
			}
		case "pooled_transfer_out":
			params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
				CommodityID: commodityID, CostCommodityID: costCommodityID,
				TransactionID: intent.TransactionID, EventDate: intent.EventDate,
				QuantityValue: intent.QuantityValue, QuantityScale: intent.QuantityScale,
				MetadataJSON: "{}", CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID}
			moved, err := pooledTransferOutTx(ctx, tx, params, intent.AuditEventID)
			if err == nil && !pooledTransferLineageReproduced(moved, intent.PooledLinks) {
				// Each destination lot is tied to one source lot and its original
				// date; a pool that now depletes other lots or quantities would
				// move different units, which no basis revision can express.
				err = errors.New("transfer source lots changed")
			}
			if err != nil {
				return InvestmentReplayProjection{}, &InvestmentReplayDependencyError{
					OperationID: intent.OperationID, Cause: fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)}
			}
			for index, link := range intent.PooledLinks {
				depletion := moved[index]
				if depletion.LotID != link.RecordedLotID || exact.ScaledIntFromInt64(depletion.CostBasisValue, depletion.CostBasisScale).Cmp(
					exact.ScaledIntFromCoefficient(link.CostBasisValue, link.CostBasisScale)) != 0 {
					projection.TransferRevisions = append(projection.TransferRevisions, InvestmentReplayTransferRevision{
						OperationID: intent.OperationID, LinkSeq: link.LinkSeq, SourceLotID: depletion.LotID,
						CostBasisValue: depletion.CostBasisValue, CostBasisScale: depletion.CostBasisScale})
				}
			}
		case "split":
			effects, err := splitEffectsForPositionTx(ctx, tx, bookID, accountID, commodityID, costCommodityID,
				intent.EventDate, intent.RatioNumerator, intent.RatioDenominator)
			if err == nil {
				err = applySplitEffectsTx(ctx, tx, bookID, effects, intent.CreatedAt, intent.CreatedByUserID, intent.AuditEventID)
			}
			if err != nil {
				return InvestmentReplayProjection{}, &InvestmentReplayDependencyError{OperationID: intent.OperationID, Cause: err}
			}
			// The posted split journal moved exactly its recorded delta. Until a
			// split's journal can be revised, history that changes how many
			// shares it multiplied is refused with the split named.
			if !intent.SplitIsSubject && sumSplitEffects(effects).Cmp(
				exact.ScaledIntFromCoefficient(intent.QuantityValue, intent.QuantityScale)) != 0 {
				return InvestmentReplayProjection{}, &InvestmentReplayDependencyError{OperationID: intent.OperationID,
					Cause: fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, ErrSplitQuantityDependency)}
			}
			projection.Splits = append(projection.Splits, InvestmentReplaySplit{
				OperationID: intent.OperationID, Subject: intent.SplitIsSubject, Effects: effects})
		default:
			return InvestmentReplayProjection{}, fmt.Errorf("%w: replay intent kind %q is unsupported", ErrInvalidDisposalParams, intent.Kind)
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT method_family FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = 'long'`,
		bookID, accountID, commodityID, costCommodityID).Scan(&projection.MethodFamily)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return InvestmentReplayProjection{}, fmt.Errorf("read replay method-family state: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, status, remaining_quantity_value, remaining_quantity_scale,
			remaining_cost_basis_value, remaining_cost_basis_scale
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = 'long'
		ORDER BY id`, bookID, accountID, commodityID, costCommodityID)
	if err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("read replay lot projection: %w", err)
	}
	for rows.Next() {
		var lot InvestmentReplayLotState
		if err := rows.Scan(&lot.LotID, &lot.Status, &lot.RemainingQuantityValue,
			&lot.RemainingQuantityScale, (*knownInvestmentBasis)(&lot.RemainingCostBasisValue),
			&lot.RemainingCostBasisScale); err != nil {
			rows.Close()
			return InvestmentReplayProjection{}, fmt.Errorf("scan replay lot projection: %w", err)
		}
		projection.Lots = append(projection.Lots, lot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return InvestmentReplayProjection{}, fmt.Errorf("iterate replay lot projection: %w", err)
	}
	if err := rows.Close(); err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("close replay lot projection: %w", err)
	}
	return projection, nil
}

// pooledTransferLineageReproduced reports whether a replayed pool depletion
// took the same quantities from the same source lots as the committed links.
// Its basis may differ; that difference becomes a link revision.
func pooledTransferLineageReproduced(moved []LotDisposalRecord, links []InvestmentReplayTransferLink) bool {
	if len(moved) != len(links) {
		return false
	}
	for index, link := range links {
		depletion := moved[index]
		if depletion.LotID != link.LotID ||
			exact.ScaledIntFromCoefficient(depletion.QuantityValue, depletion.QuantityScale).Cmp(
				exact.ScaledIntFromCoefficient(link.QuantityValue, link.QuantityScale)) != 0 {
			return false
		}
	}
	return true
}

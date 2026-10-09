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
	// SubjectTransferOut is the subject transfer's source depletions at its
	// replay slot, in effect order; SubjectTransferMethod is the method its
	// lot events record (average_cost for a pool, empty for selected lots).
	SubjectTransferOut    []LotDisposalRecord
	SubjectTransferMethod string
	// CapitalReturns are returns of capital whose replayed effects differ
	// from their effective ones; persisting appends a revision (T-148).
	CapitalReturns []InvestmentReplayCapitalReturn
	// SubjectCapitalReturn is the effect set of the return of capital the
	// command is recording, at its replay slot.
	SubjectCapitalReturn []CapitalReturnEffect
	// PositionSide is the side this projection replayed; empty is long. A
	// short position replays only its openings and covers (#175).
	PositionSide string
}

// InvestmentReplayTransferRevision is one link's replayed depletion: the
// source lot it now takes from and the basis it carries. A pooled_lot link
// (T-135) has no SourceLotID; it records every source depletion and the
// latest original acquisition date among them instead.
type InvestmentReplayTransferRevision struct {
	OperationID    int64
	LinkSeq        int
	SourceLotID    int64
	CostBasisValue int64
	CostBasisScale int
	// BasisKnowledge is the link's knowledge, which a revision never changes
	// (T-145); unknown leaves CostBasis unused.
	BasisKnowledge string
	PooledLot      bool
	// ExternalOut marks an outbound transfer's link: it has no destination to
	// replay; its basis change posts a dated bridge adjustment (T-143).
	ExternalOut bool
	// pooled_lot only.
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
	Depletions            []InvestmentReplayTransferLink
}

// InvestmentReplaySplit is a split's per-lot effect at its replay slot.
// Subject marks the split the current command is creating.
type InvestmentReplaySplit struct {
	OperationID int64
	Subject     bool
	Effects     []SplitLotEffect
}

type InvestmentReplayLotState struct {
	BasisKnowledge          string
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
	BasisKnowledge string // Empty means known; unknown leaves CostBasis unused.
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
	return simulateInvestmentReplaySideTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, PositionSideLong, intents)
}

// simulateInvestmentReplaySideTx replays one side of a position (#175).
func simulateInvestmentReplaySideTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64, side string, intents []InvestmentReplayIntent) (InvestmentReplayProjection, error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT investment_replay_simulation`); err != nil {
		return InvestmentReplayProjection{}, fmt.Errorf("start investment replay simulation: %w", err)
	}
	projection, simulationErr := runInvestmentReplayTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, side, intents)
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

func runInvestmentReplayTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64, side string, intents []InvestmentReplayIntent) (InvestmentReplayProjection, error) {
	if !validPositionSide(side) {
		return InvestmentReplayProjection{}, fmt.Errorf("%w: replay position side %q is invalid", ErrInvalidDisposalParams, side)
	}
	if err := resetInvestmentReplayPositionTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, side); err != nil {
		return InvestmentReplayProjection{}, err
	}
	projection := InvestmentReplayProjection{PositionSide: side}
	ordered := slices.Clone(intents)
	sortInvestmentReplayIntents(ordered)
	for _, intent := range ordered {
		if err := applyInvestmentReplayIntentTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, side, intent, &projection); err != nil {
			return InvestmentReplayProjection{}, err
		}
	}
	if err := finishInvestmentReplayPositionTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, side, &projection); err != nil {
		return InvestmentReplayProjection{}, err
	}
	return projection, nil
}

// resetInvestmentReplayPositionTx clears one position's lot and method-family
// projection so its intents can rebuild it from the first opening.
func resetInvestmentReplayPositionTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64, side string) error {
	if bookID <= 0 || accountID <= 0 || commodityID <= 0 || costCommodityID <= 0 {
		return fmt.Errorf("%w: replay position key is incomplete", ErrInvalidDisposalParams)
	}
	var unmodeledLotID int64
	err := tx.QueryRowContext(ctx, `
		SELECT l.id FROM investment_lots l
		WHERE l.book_id = ? AND l.account_id = ? AND l.commodity_id = ?
			AND l.cost_commodity_id = ? AND l.position_side = ? AND l.operation_id IS NULL
		LIMIT 1`, bookID, accountID, commodityID, costCommodityID, side).Scan(&unmodeledLotID)
	if err == nil {
		return fmt.Errorf("%w: lot %d lacks an immutable opening fact", ErrInvalidDisposalParams, unmodeledLotID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check replay opening coverage: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_lot_state
		(lot_id, book_id, status, remaining_quantity_value, remaining_quantity_scale,
		remaining_cost_basis_value, remaining_cost_basis_scale, updated_at, updated_by_user_id, updated_audit_event_id, basis_knowledge)
		SELECT id, book_id, 'closed', '0', quantity_scale,
		CASE WHEN opening_basis_knowledge = 'known' THEN '0' ELSE NULL END, cost_basis_scale,
		created_at, created_by_user_id, created_audit_event_id, opening_basis_knowledge FROM investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ? AND cost_commodity_id = ? AND position_side = ?
		ON CONFLICT(lot_id) DO UPDATE SET basis_knowledge = excluded.basis_knowledge, status = 'closed', remaining_quantity_value = '0',
		remaining_quantity_scale = excluded.remaining_quantity_scale, remaining_cost_basis_value = excluded.remaining_cost_basis_value,
		remaining_cost_basis_scale = excluded.remaining_cost_basis_scale`, bookID, accountID, commodityID, costCommodityID, side); err != nil {
		return fmt.Errorf("reset replay lot projection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = ?`,
		bookID, accountID, commodityID, costCommodityID, side); err != nil {
		return fmt.Errorf("reset replay basis-method state: %w", err)
	}
	return nil
}

// applyInvestmentReplayIntentTx replays one intent against its position's
// projection and records its output in projection.
func applyInvestmentReplayIntentTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64,
	side string, intent InvestmentReplayIntent, projection *InvestmentReplayProjection) error {
	if side == PositionSideShort && intent.Kind != "opening" && intent.Kind != "disposal" {
		return fmt.Errorf("%w: replay intent kind %q has no short-side contract", ErrInvalidDisposalParams, intent.Kind)
	}
	switch intent.Kind {
	case "opening":
		knowledge := normalizedBasisKnowledge(intent.BasisKnowledge)
		basis, err := exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale).Int64()
		if err != nil || basis < 0 || intent.QuantityValue.Sign() <= 0 ||
			(knowledge != InvestmentBasisKnown && knowledge != InvestmentBasisUnknown) ||
			(knowledge == InvestmentBasisUnknown && (basis != 0 || intent.AmountScale != 0)) {
			return fmt.Errorf("%w: replay opening lot %d has invalid quantity or basis", ErrInvestmentBasisRange, intent.LotID)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE investment_lot_state SET basis_knowledge = ?, status = 'open',
				remaining_quantity_value = ?, remaining_quantity_scale = ?,
				remaining_cost_basis_value = ?, remaining_cost_basis_scale = ?
			WHERE lot_id IN (SELECT id FROM investment_lots WHERE id = ? AND book_id = ? AND account_id = ? AND commodity_id = ?
				AND cost_commodity_id = ? AND position_side = ? AND opened_on = ?)`, knowledge, intent.QuantityValue, intent.QuantityScale, nullableBasisValue(basis, knowledge), nullableBasisScale(intent.AmountScale, knowledge),
			intent.LotID, bookID, accountID, commodityID, costCommodityID, side, intent.EventDate)
		if err != nil {
			return fmt.Errorf("activate replay opening lot %d: %w", intent.LotID, err)
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return fmt.Errorf("%w: replay opening lot %d does not match this position and date", ErrInvalidDisposalParams, intent.LotID)
		}
		if err := requirePositionBasisRangeQueryTx(ctx, tx, bookID, accountID, commodityID, costCommodityID, true, side); err != nil {
			return fmt.Errorf("replay opening lot %d: %w", intent.LotID, err)
		}
	case "disposal":
		params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
			CommodityID: commodityID, CostCommodityID: costCommodityID,
			ProceedsScale: intent.AmountScale,
			TransactionID: intent.TransactionID, EventDate: intent.EventDate,
			QuantityValue: intent.QuantityValue, QuantityScale: intent.QuantityScale,
			CostBasisMethod: intent.CostBasisMethod, Allocations: intent.SpecificLots,
			CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID,
			MetadataJSON: "{}", PositionSide: side,
			AdmitUnknownBasis: side == PositionSideLong && (intent.AdmitUnknownBasis || intent.OperationKind == "sell")}
		if !intent.AmountValue.BigInt().IsInt64() {
			return ErrInvestmentBasisRange
		}
		params.ProceedsValue = intent.AmountValue.BigInt().Int64()
		allocations, err := disposeLotsWithAuditTx(ctx, tx, params, intent.AuditEventID, true)
		if err != nil {
			return &InvestmentReplayDependencyError{
				OperationID: intent.OperationID, DecisionID: intent.DecisionID, Cause: err,
			}
		}
		disposal := InvestmentReplayDisposal{DecisionID: intent.DecisionID}
		for _, allocation := range allocations {
			disposal.Allocations = append(disposal.Allocations, InvestmentReplayAllocation{
				LotID: allocation.LotID, QuantityValue: allocation.QuantityValue,
				QuantityScale: allocation.QuantityScale, CostBasisValue: allocation.CostBasisValue,
				CostBasisScale: allocation.CostBasisScale, ProceedsValue: allocation.ProceedsValue,
				ProceedsScale: allocation.ProceedsScale, BasisKnowledge: allocation.BasisKnowledge,
			})
		}
		projection.Disposals = append(projection.Disposals, disposal)
	case "transfer_out":
		params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
			CommodityID: commodityID, CostCommodityID: costCommodityID,
			TransactionID: intent.TransactionID, EventDate: intent.EventDate,
			EventKind: "transfer_out", MetadataJSON: "{}", AdmitUnknownBasis: true,
			CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID}
		allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
		if err != nil {
			return err
		}
		moved, err := disposeLotTx(ctx, tx, params, intent.LotID,
			intent.QuantityValue, intent.QuantityScale, intent.AuditEventID, allocationScale)
		if err == nil {
			// The writer locks a source it moves lots out of to individual
			// lots; replay must leave the same lock (found by T-134).
			err = updatePositionMethodFamilyTx(ctx, tx, params, "specific_lot", intent.AuditEventID)
		}
		if err != nil {
			return replayTransferError(intent, err)
		}
		if intent.TransferIsSubject {
			projection.SubjectTransferOut = append(projection.SubjectTransferOut, moved)
			return nil
		}
		// The quantity is fixed by the transfer; the basis it carries, and
		// the successor lot of a corrected acquisition, follow history. Its
		// knowledge may only become known (a resolution reaching it).
		knowledge, resolved, err := transferKnowledgeAfterReplay(intent.BasisKnowledge, moved.BasisKnowledge)
		if err != nil {
			return replayTransferError(intent, err)
		}
		if moved.LotID != intent.RecordedLotID || resolved || knowledge == InvestmentBasisKnown &&
			exact.ScaledIntFromInt64(moved.CostBasisValue, moved.CostBasisScale).Cmp(
				exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale)) != 0 {
			projection.TransferRevisions = append(projection.TransferRevisions, InvestmentReplayTransferRevision{
				OperationID: intent.OperationID, LinkSeq: intent.LinkSeq, SourceLotID: moved.LotID,
				CostBasisValue: moved.CostBasisValue, CostBasisScale: moved.CostBasisScale,
				BasisKnowledge: knowledge, ExternalOut: intent.ExternalOut})
		}
	case "pooled_transfer_out":
		params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
			CommodityID: commodityID, CostCommodityID: costCommodityID,
			TransactionID: intent.TransactionID, EventDate: intent.EventDate,
			QuantityValue: intent.QuantityValue, QuantityScale: intent.QuantityScale, AdmitUnknownBasis: true,
			MetadataJSON: "{}", CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID}
		moved, err := pooledTransferOutTx(ctx, tx, params, intent.AuditEventID)
		if err == nil && intent.TransferIsSubject {
			projection.SubjectTransferOut, projection.SubjectTransferMethod = moved, "average_cost"
			return nil
		}
		if err == nil && intent.ExternalOut && !pooledTransferLineageReproduced(moved, intent.PooledLinks) {
			// An outbound pool's links are fixed per source lot; a basis change
			// is revised, a change of which lots it took is not expressible.
			err = ErrExternalTransferLotsChanged
		}
		if err == nil && !pooledTransferLineageReproduced(moved, intent.PooledLinks) {
			// Each destination lot is tied to one source lot and its original
			// date; a pool that now depletes other lots or quantities would
			// move different units, which no basis revision can express.
			// A pooled_lot transfer has no such tie (T-135).
			err = errors.New("transfer source lots changed; record it as one pooled lot to let history move them")
		}
		if err != nil {
			return replayTransferError(intent, err)
		}
		for index, link := range intent.PooledLinks {
			depletion := moved[index]
			knowledge, resolved, err := transferKnowledgeAfterReplay(link.BasisKnowledge, depletion.BasisKnowledge)
			if err != nil {
				return replayTransferError(intent, err)
			}
			if depletion.LotID != link.RecordedLotID || resolved || knowledge == InvestmentBasisKnown &&
				exact.ScaledIntFromInt64(depletion.CostBasisValue, depletion.CostBasisScale).Cmp(
					exact.ScaledIntFromCoefficient(link.CostBasisValue, link.CostBasisScale)) != 0 {
				projection.TransferRevisions = append(projection.TransferRevisions, InvestmentReplayTransferRevision{
					OperationID: intent.OperationID, LinkSeq: link.LinkSeq, SourceLotID: depletion.LotID,
					CostBasisValue: depletion.CostBasisValue, CostBasisScale: depletion.CostBasisScale,
					BasisKnowledge: knowledge, ExternalOut: intent.ExternalOut})
			}
		}
	case "pooled_lot_transfer_out":
		params := DisposeLotsParams{BookID: bookID, AccountID: accountID,
			CommodityID: commodityID, CostCommodityID: costCommodityID,
			TransactionID: intent.TransactionID, EventDate: intent.EventDate,
			QuantityValue: intent.QuantityValue, QuantityScale: intent.QuantityScale, AdmitUnknownBasis: true,
			MetadataJSON: "{}", CreatedAt: intent.CreatedAt, ActorUserID: intent.CreatedByUserID}
		moved, err := pooledTransferOutTx(ctx, tx, params, intent.AuditEventID)
		if err == nil && intent.TransferIsSubject {
			projection.SubjectTransferOut, projection.SubjectTransferMethod = moved, "average_cost"
			return nil
		}
		var totals pooledTransferTotals
		if err == nil {
			totals, err = pooledTransferTotalsTx(ctx, tx, bookID, moved)
		}
		if err == nil && exact.ScaledIntFromCoefficient(totals.quantityValue, totals.quantityScale).Cmp(
			exact.ScaledIntFromCoefficient(intent.QuantityValue, intent.QuantityScale)) != 0 {
			err = errors.New("pooled transfer quantity changed")
		}
		if err == nil {
			_, _, err = transferKnowledgeAfterReplay(intent.BasisKnowledge, totals.knowledge)
		}
		if err != nil {
			return replayTransferError(intent, err)
		}
		// The destination lot and quantity are fixed; its basis, date and
		// the source lots the pool took them from follow history.
		if revision, changed := pooledLotTransferRevision(intent, moved, totals); changed {
			projection.TransferRevisions = append(projection.TransferRevisions, revision)
		}
	case "split":
		effects, err := splitEffectsForPositionTx(ctx, tx, bookID, accountID, commodityID, costCommodityID,
			intent.EventDate, intent.RatioNumerator, intent.RatioDenominator)
		if err == nil {
			err = applySplitEffectsTx(ctx, tx, bookID, effects, intent.CreatedAt, intent.CreatedByUserID, intent.AuditEventID)
		}
		if err != nil {
			return &InvestmentReplayDependencyError{OperationID: intent.OperationID, Cause: err}
		}
		// History may change how many shares the split multiplied; persisting
		// the projection posts that difference as an adjustment journal.
		projection.Splits = append(projection.Splits, InvestmentReplaySplit{
			OperationID: intent.OperationID, Subject: intent.SplitIsSubject, Effects: effects})
	case "capital_return":
		// The basis action at its slot follows history: changed effects are
		// revised (T-148). A slot with no entitled lot, or an unknown basis,
		// cannot express the receipt and names the operation.
		effects, err := capitalReturnEffectsTx(ctx, tx, bookID, accountID, commodityID, costCommodityID,
			intent.EventDate, exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale),
			intent.CapitalReturnEntitledLots, intent.CapitalReturnEntitlements)
		if err == nil {
			switch {
			case intent.CapitalReturnIsSubject:
				projection.SubjectCapitalReturn = effects
			case !sameCapitalReturnEffects(effects, intent.CapitalReturnEffects):
				projection.CapitalReturns = append(projection.CapitalReturns,
					InvestmentReplayCapitalReturn{OperationID: intent.OperationID, Effects: effects})
			}
		}
		if err == nil {
			err = applyCapitalReturnEffectsTx(ctx, tx, bookID, effects, intent.CreatedAt, intent.CreatedByUserID, intent.AuditEventID)
		}
		if err != nil {
			if intent.CapitalReturnIsSubject {
				return err
			}
			return &InvestmentReplayDependencyError{OperationID: intent.OperationID,
				Cause: fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)}
		}
	default:
		return fmt.Errorf("%w: replay intent kind %q is unsupported", ErrInvalidDisposalParams, intent.Kind)
	}
	return nil
}

// finishInvestmentReplayPositionTx reads one replayed position's resulting
// lot state and method family into projection.
func finishInvestmentReplayPositionTx(ctx context.Context, tx *sql.Tx, bookID, accountID, commodityID, costCommodityID int64,
	side string, projection *InvestmentReplayProjection) error {
	err := tx.QueryRowContext(ctx, `SELECT method_family FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = ?`,
		bookID, accountID, commodityID, costCommodityID, side).Scan(&projection.MethodFamily)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read replay method-family state: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, status, remaining_quantity_value, remaining_quantity_scale,
			remaining_cost_basis_value, remaining_cost_basis_scale, basis_knowledge
		FROM current_investment_lots
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = ?
		ORDER BY id`, bookID, accountID, commodityID, costCommodityID, side)
	if err != nil {
		return fmt.Errorf("read replay lot projection: %w", err)
	}
	for rows.Next() {
		var lot InvestmentReplayLotState
		var basis, scale sql.NullInt64
		if err := rows.Scan(&lot.LotID, &lot.Status, &lot.RemainingQuantityValue,
			&lot.RemainingQuantityScale, &basis, &scale, &lot.BasisKnowledge); err != nil {
			rows.Close()
			return fmt.Errorf("scan replay lot projection: %w", err)
		}
		lot.RemainingCostBasisValue, lot.RemainingCostBasisScale, err = projectedBasis(basis, scale, lot.BasisKnowledge)
		if err != nil {
			rows.Close()
			return fmt.Errorf("read replay lot basis: %w", err)
		}
		projection.Lots = append(projection.Lots, lot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate replay lot projection: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close replay lot projection: %w", err)
	}
	return nil
}

// replayTransferError names a transfer replay cannot reproduce as a
// dependency of the command. The subject transfer of a replacement is the
// command's own move, so its shortfall is returned as itself.
func replayTransferError(intent InvestmentReplayIntent, err error) error {
	if intent.TransferIsSubject {
		return err
	}
	return &InvestmentReplayDependencyError{
		OperationID: intent.OperationID, Cause: fmt.Errorf("%w: %w", ErrInvestmentCorrectionDependency, err)}
}

// pooledLotTransferRevision compares a replayed pooled_lot depletion with the
// transfer's effective one and returns the revision to append when anything
// differs: a source lot, a quantity, a basis or the original date.
func pooledLotTransferRevision(intent InvestmentReplayIntent, moved []LotDisposalRecord, totals pooledTransferTotals) (InvestmentReplayTransferRevision, bool) {
	known := totals.knowledge == InvestmentBasisKnown
	revision := InvestmentReplayTransferRevision{OperationID: intent.OperationID, LinkSeq: intent.LinkSeq,
		CostBasisValue: totals.basisValue, CostBasisScale: totals.basisScale, BasisKnowledge: totals.knowledge, PooledLot: true,
		OriginalDateKnowledge: totals.originalKnowledge, OriginalAcquiredOn: totals.originalDate}
	changed := len(moved) != len(intent.PooledDepletions) ||
		totals.knowledge != normalizedBasisKnowledge(intent.BasisKnowledge) ||
		totals.originalKnowledge != intent.OriginalDateKnowledge || totals.originalDate != intent.OriginalAcquiredOn ||
		known && exact.ScaledIntFromInt64(totals.basisValue, totals.basisScale).Cmp(
			exact.ScaledIntFromCoefficient(intent.AmountValue, intent.AmountScale)) != 0
	for index, depletion := range moved {
		knowledge := normalizedBasisKnowledge(depletion.BasisKnowledge)
		revision.Depletions = append(revision.Depletions, InvestmentReplayTransferLink{
			LotID: depletion.LotID, QuantityValue: depletion.QuantityValue, QuantityScale: depletion.QuantityScale,
			CostBasisValue: exact.New(depletion.CostBasisValue), CostBasisScale: depletion.CostBasisScale,
			BasisKnowledge: knowledge})
		if changed {
			continue
		}
		effective := intent.PooledDepletions[index]
		changed = depletion.LotID != effective.LotID || knowledge != normalizedBasisKnowledge(effective.BasisKnowledge) ||
			exact.ScaledIntFromCoefficient(depletion.QuantityValue, depletion.QuantityScale).Cmp(
				exact.ScaledIntFromCoefficient(effective.QuantityValue, effective.QuantityScale)) != 0 ||
			knowledge == InvestmentBasisKnown && exact.ScaledIntFromInt64(depletion.CostBasisValue, depletion.CostBasisScale).Cmp(
				exact.ScaledIntFromCoefficient(effective.CostBasisValue, effective.CostBasisScale)) != 0
	}
	return revision, changed
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

// ErrTransferBasisKnowledgeChanged refuses a history change that would turn a
// transfer's known basis unknown. Unknown may become known when a sourced
// resolution (or a corrected acquisition) reaches it (T-145).
var ErrTransferBasisKnowledgeChanged = errors.New("a transfer's known basis would become unknown")

// transferKnowledgeAfterReplay returns the knowledge replay produced and
// whether it resolved a recorded unknown.
func transferKnowledgeAfterReplay(recorded, replayed string) (string, bool, error) {
	recorded, replayed = normalizedBasisKnowledge(recorded), normalizedBasisKnowledge(replayed)
	switch {
	case recorded == replayed:
		return replayed, false, nil
	case recorded == InvestmentBasisUnknown:
		return replayed, true, nil
	default:
		return "", false, ErrTransferBasisKnowledgeChanged
	}
}

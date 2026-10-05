package db

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"rekenraam/backend/internal/exact"
)

// propagateInvestmentTransferRevisionsTx carries changed internal-transfer
// basis to the destination positions, within the caller's transaction
// (ADR 0013 cross-position replay refinement, T-132).
//
// The only input one position gives another is a link's carried basis and,
// for a pooled_lot link (T-135), its original acquisition date: destination
// lots and quantities of a transfer are fixed, and a source_lots replay that
// would change which lots it takes is refused with the transfer named. A
// source's depletion at a transfer's slot depends only on that source's
// earlier history, so the dependency closure is replayed once as the merged
// dated stream the ADR describes (T-140): every position's intents in one
// causal order, each transfer handing its new basis to its destination lot
// before that lot opens. Chains and cycles (A→B, later B→A) settle in that
// single pass whatever their depth, and each affected position is persisted
// once from its final inputs.
//
// Every write joins the caller's transaction. Any refusal (an impossible
// disposal, an invalid election, a split quantity dependency, a basis range
// overflow) rolls back the triggering command with the dependent operation
// named. A position in a cycle through the caller's own position is persisted
// again here; each persist appends its effective revision, and the last one
// is current.
func propagateInvestmentTransferRevisionsTx(ctx context.Context, tx *sql.Tx, bookID,
	causedByOperationID, auditEventID, actorUserID int64, createdAt string,
	revisions []InvestmentReplayTransferRevision) error {
	if len(revisions) == 0 {
		return nil
	}
	affected := make(map[investmentReplayPositionKey]bool)
	if err := recordTransferRevisionsTx(ctx, tx, bookID, causedByOperationID, auditEventID, actorUserID,
		createdAt, revisions, affected); err != nil {
		return err
	}
	if len(affected) == 0 {
		// Only outbound links moved: nothing in the book replays from them.
		return nil
	}
	// Every position downstream of a changed link, whatever its date: the
	// pass then never meets a destination outside the positions it replays.
	seeds := make([]InvestmentReplayPosition, 0, len(affected))
	for key := range affected {
		seeds = append(seeds, InvestmentReplayPosition{AccountID: key.accountID, CommodityID: key.commodityID,
			CostCommodityID: key.costCommodityID, AffectedFrom: "0001-01-01"})
	}
	closure, err := investmentReplayClosureQuery(ctx, tx, bookID, seeds)
	if err != nil {
		return err
	}
	keys := make([]investmentReplayPositionKey, 0, len(closure))
	for _, position := range closure {
		keys = append(keys, investmentReplayPositionKey{position.AccountID, position.CommodityID, position.CostCommodityID})
	}
	sortInvestmentReplayPositionKeys(keys)
	pass, err := simulateInvestmentReplayClosureTx(ctx, tx, bookID, causedByOperationID, auditEventID, createdAt, keys)
	if err != nil {
		return err
	}
	if err := recordTransferRevisionsTx(ctx, tx, bookID, causedByOperationID, auditEventID, actorUserID,
		createdAt, pass.revisions, affected); err != nil {
		return err
	}
	persisted := make([]investmentReplayPositionKey, 0, len(affected))
	for key := range affected {
		persisted = append(persisted, key)
	}
	sortInvestmentReplayPositionKeys(persisted)
	for _, key := range persisted {
		projection, replayed := pass.projections[key]
		if !replayed {
			return fmt.Errorf("%w: transfer destination is outside the replayed closure", ErrInvalidDisposalParams)
		}
		// The pass read each position's inputs before its own revisions; the
		// persisted intents are the final ones it effectively replayed.
		intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, key.accountID,
			key.commodityID, key.costCommodityID, "long")
		if err != nil {
			return err
		}
		if err := persistInvestmentReplayPositionTx(ctx, tx, bookID, key.accountID, key.commodityID,
			key.costCommodityID, causedByOperationID, auditEventID, actorUserID, createdAt,
			intents, projection); err != nil {
			return err
		}
	}
	return nil
}

// recordTransferRevisionsTx appends each changed link's revision. An internal
// link adds its destination to affected; an outbound link's basis change is
// summed per transfer and cost currency and posted as one dated bridge
// adjustment, T −Δ and E +Δ, under the causing command's audit event (T-143).
// The original bridge and link stay immutable evidence.
func recordTransferRevisionsTx(ctx context.Context, tx *sql.Tx, bookID, causedByOperationID, auditEventID,
	actorUserID int64, createdAt string, revisions []InvestmentReplayTransferRevision,
	affected map[investmentReplayPositionKey]bool,
) error {
	type bridgeKey struct{ operationID, costCommodityID int64 }
	deltas := make(map[bridgeKey]*exact.ScaledInt)
	var order []bridgeKey
	for _, revision := range revisions {
		if revision.ExternalOut {
			var prior exact.Coefficient
			var priorScale int
			var costID int64
			if err := tx.QueryRowContext(ctx, `SELECT carried_basis_value, carried_basis_scale, cost_commodity_id
				FROM effective_investment_transfer_links WHERE operation_id = ? AND link_seq = ?
					AND basis_knowledge = 'known'`, revision.OperationID, revision.LinkSeq).Scan(
				&prior, &priorScale, &costID); err != nil {
				return fmt.Errorf("read outbound transfer link basis: %w", err)
			}
			key := bridgeKey{revision.OperationID, costID}
			if deltas[key] == nil {
				deltas[key] = exact.NewScaledInt()
				order = append(order, key)
			}
			deltas[key].AddInt64(revision.CostBasisValue, revision.CostBasisScale)
			deltas[key].SubScaled(exact.ScaledIntFromCoefficient(prior, priorScale))
		}
		destination, _, err := appendTransferLinkRevisionTx(ctx, tx, bookID, causedByOperationID,
			auditEventID, createdAt, revision)
		if err != nil {
			return err
		}
		if !revision.ExternalOut {
			affected[destination] = true
		}
	}
	for _, key := range order {
		if deltas[key].Sign() == 0 {
			continue
		}
		if _, err := postTransferBridgeJournalTx(ctx, tx, bookID, key.operationID, key.costCommodityID,
			deltas[key], causedByOperationID, auditEventID, actorUserID, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func sortInvestmentReplayPositionKeys(keys []investmentReplayPositionKey) {
	slices.SortFunc(keys, func(a, b investmentReplayPositionKey) int {
		return cmp.Or(cmp.Compare(a.accountID, b.accountID), cmp.Compare(a.commodityID, b.commodityID),
			cmp.Compare(a.costCommodityID, b.costCommodityID))
	})
}

type investmentReplayClosurePass struct {
	projections map[investmentReplayPositionKey]InvestmentReplayProjection
	// revisions are the links the pass changed, in causal order.
	revisions []InvestmentReplayTransferRevision
}

// simulateInvestmentReplayClosureTx replays the positions as one merged dated
// stream inside a savepoint and rolls every write back. A changed link's
// revision is appended inside the pass, so the destination's lot order sees
// its original date, and its destination lot opens at the new basis.
func simulateInvestmentReplayClosureTx(ctx context.Context, tx *sql.Tx, bookID, causedByOperationID,
	auditEventID int64, createdAt string, keys []investmentReplayPositionKey) (investmentReplayClosurePass, error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT investment_replay_closure`); err != nil {
		return investmentReplayClosurePass{}, fmt.Errorf("start investment replay closure: %w", err)
	}
	pass, passErr := runInvestmentReplayClosureTx(ctx, tx, bookID, causedByOperationID, auditEventID, createdAt, keys)
	if _, err := tx.ExecContext(ctx, `ROLLBACK TO investment_replay_closure`); err != nil {
		abortErr := tx.Rollback()
		return investmentReplayClosurePass{}, errors.Join(passErr,
			fmt.Errorf("restore projection after investment replay closure: %w", err), abortErr)
	}
	if _, err := tx.ExecContext(ctx, `RELEASE investment_replay_closure`); err != nil {
		abortErr := tx.Rollback()
		return investmentReplayClosurePass{}, errors.Join(passErr,
			fmt.Errorf("release investment replay closure savepoint: %w", err), abortErr)
	}
	return pass, passErr
}

func runInvestmentReplayClosureTx(ctx context.Context, tx *sql.Tx, bookID, causedByOperationID,
	auditEventID int64, createdAt string, keys []investmentReplayPositionKey) (investmentReplayClosurePass, error) {
	type positionIntent struct {
		key    investmentReplayPositionKey
		intent InvestmentReplayIntent
	}
	pass := investmentReplayClosurePass{projections: make(map[investmentReplayPositionKey]InvestmentReplayProjection, len(keys))}
	projections := make(map[investmentReplayPositionKey]*InvestmentReplayProjection, len(keys))
	var stream []positionIntent
	for _, key := range keys {
		intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, key.accountID, key.commodityID, key.costCommodityID, "long")
		if err != nil {
			return pass, err
		}
		if err := resetInvestmentReplayPositionTx(ctx, tx, bookID, key.accountID, key.commodityID, key.costCommodityID); err != nil {
			return pass, err
		}
		projections[key] = &InvestmentReplayProjection{}
		for _, intent := range intents {
			stream = append(stream, positionIntent{key, intent})
		}
	}
	// Each position's intents keep their own order; across positions the same
	// date, correction-root slot and effect order puts a transfer's source
	// depletion before the destination lot it opens.
	slices.SortStableFunc(stream, func(a, b positionIntent) int {
		return compareInvestmentReplayIntents(a.intent, b.intent)
	})
	carried := make(map[int64]InvestmentReplayTransferRevision) // destination lot → revised link
	opened := make(map[int64]bool)
	for _, item := range stream {
		intent, projection := item.intent, projections[item.key]
		if intent.Kind == "opening" {
			if revision, revised := carried[intent.LotID]; revised {
				intent.AmountValue, intent.AmountScale = exact.New(revision.CostBasisValue), revision.CostBasisScale
			}
			opened[intent.LotID] = true
		}
		before := len(projection.TransferRevisions)
		if err := applyInvestmentReplayIntentTx(ctx, tx, bookID, item.key.accountID, item.key.commodityID,
			item.key.costCommodityID, intent, projection); err != nil {
			return pass, err
		}
		for _, revision := range projection.TransferRevisions[before:] {
			if revision.ExternalOut {
				// Nothing in the book opens from an outbound link; its revision
				// and bridge adjustment are written after the pass.
				pass.revisions = append(pass.revisions, revision)
				continue
			}
			destination, lotID, err := appendTransferLinkRevisionTx(ctx, tx, bookID, causedByOperationID,
				auditEventID, createdAt, revision)
			if err != nil {
				return pass, err
			}
			if projections[destination] == nil || opened[lotID] {
				return pass, fmt.Errorf("%w: transfer operation %d reached its destination out of causal order",
					ErrInvalidDisposalParams, revision.OperationID)
			}
			carried[lotID] = revision
			pass.revisions = append(pass.revisions, revision)
		}
	}
	for _, key := range keys {
		projection := projections[key]
		if err := finishInvestmentReplayPositionTx(ctx, tx, bookID, key.accountID, key.commodityID,
			key.costCommodityID, projection); err != nil {
			return pass, err
		}
		pass.projections[key] = *projection
	}
	return pass, nil
}

// appendTransferLinkRevisionTx records one link's new effective depletion and
// returns the destination position and lot that must replay from it.
func appendTransferLinkRevisionTx(ctx context.Context, tx *sql.Tx, bookID, causedByOperationID,
	auditEventID int64, createdAt string, revision InvestmentReplayTransferRevision) (investmentReplayPositionKey, int64, error) {
	if revision.OperationID <= 0 || revision.LinkSeq <= 0 || revision.CostBasisValue < 0 ||
		(revision.SourceLotID > 0) == revision.PooledLot || (revision.PooledLot && len(revision.Depletions) == 0) {
		return investmentReplayPositionKey{}, 0, fmt.Errorf("%w: transfer link revision is incomplete", ErrInvalidDisposalParams)
	}
	var destination investmentReplayPositionKey
	var destinationLotID int64
	var external bool
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(d.id, 0), COALESCE(d.account_id, 0),
			COALESCE(d.commodity_id, 0), COALESCE(d.cost_commodity_id, 0), f.transfer_kind = 'external_out'
		FROM investment_transfer_lot_links x
		JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
		LEFT JOIN investment_lots d ON d.id = x.destination_lot_id
		WHERE x.operation_id = ? AND x.link_seq = ? AND f.book_id = ?
			AND f.transfer_kind IN ('internal', 'external_out')`,
		revision.OperationID, revision.LinkSeq, bookID).Scan(&destinationLotID,
		&destination.accountID, &destination.commodityID, &destination.costCommodityID, &external); err != nil {
		return investmentReplayPositionKey{}, 0, fmt.Errorf("read revised transfer link destination: %w", err)
	}
	// An outbound link has no destination to replay (T-143).
	if external != revision.ExternalOut || external == (destinationLotID > 0) {
		return investmentReplayPositionKey{}, 0, fmt.Errorf("%w: transfer link revision does not match its transfer kind", ErrInvalidDisposalParams)
	}
	var priorID sql.NullInt64
	priorSeq := 1
	err := tx.QueryRowContext(ctx, `SELECT id, revision_seq FROM latest_investment_transfer_link_revisions
		WHERE book_id = ? AND operation_id = ? AND link_seq = ?`,
		bookID, revision.OperationID, revision.LinkSeq).Scan(&priorID, &priorSeq)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return investmentReplayPositionKey{}, 0, fmt.Errorf("read current transfer link revision: %w", err)
	}
	var originalKnowledge, originalDate sql.NullString
	if revision.PooledLot {
		originalKnowledge = sql.NullString{String: revision.OriginalDateKnowledge, Valid: true}
		originalDate = sql.NullString{String: revision.OriginalAcquiredOn, Valid: revision.OriginalAcquiredOn != ""}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_link_revisions (
		book_id, operation_id, link_seq, revision_seq, caused_by_operation_id, supersedes_revision_id,
		source_lot_id, carried_basis_value, carried_basis_scale, original_date_knowledge, original_acquired_on,
		created_at, created_audit_event_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		bookID, revision.OperationID, revision.LinkSeq, priorSeq+1, causedByOperationID, priorID,
		nullablePositiveInt64(revision.SourceLotID), exact.New(revision.CostBasisValue), revision.CostBasisScale,
		originalKnowledge, originalDate, createdAt, auditEventID)
	if err != nil {
		return investmentReplayPositionKey{}, 0, fmt.Errorf("append transfer link revision: %w", err)
	}
	revisionID, err := result.LastInsertId()
	if err != nil {
		return investmentReplayPositionKey{}, 0, fmt.Errorf("read transfer link revision id: %w", err)
	}
	for index, depletion := range revision.Depletions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_link_revision_depletions (
			revision_id, depletion_seq, book_id, source_lot_id, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, revisionID, index+1, bookID, depletion.LotID,
			depletion.QuantityValue, depletion.QuantityScale, depletion.CostBasisValue, depletion.CostBasisScale); err != nil {
			return investmentReplayPositionKey{}, 0, fmt.Errorf("append transfer revision depletion: %w", err)
		}
	}
	return destination, destinationLotID, nil
}

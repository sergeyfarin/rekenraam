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

// maxInvestmentTransferPropagationRounds bounds the fixed-point walk. Each
// round crosses one more transfer; carried basis only flows forward in time,
// so a real book settles in at most one round per transfer in its longest
// chain. Hitting the bound means the inputs are inconsistent, not that the
// book is large.
const maxInvestmentTransferPropagationRounds = 256

// propagateInvestmentTransferRevisionsTx carries changed internal-transfer
// basis to the destination positions, within the caller's transaction
// (ADR 0013 cross-position replay refinement, T-132).
//
// The only input one position gives another is a link's carried basis and,
// for a pooled_lot link (T-135), its original acquisition date: destination
// lots and quantities of a transfer are fixed, and a source_lots replay that
// would change which lots it takes is refused with the transfer named. So the
// merged dated stream the ADR describes is computed exactly by replaying each
// destination from its links' effective inputs and repeating until no link
// changes. A source's depletion at a transfer's slot depends only on that
// source's earlier history, so a cycle (A→B, later B→A) settles: the return
// leg's new basis cannot reach back before it. Destinations reached this way
// are a subset of InvestmentReplayClosure — only links that actually moved.
//
// Every write joins the caller's transaction. Any refusal (an impossible
// disposal, an invalid election, a split quantity dependency, a basis range
// overflow) rolls back the triggering command with the dependent operation
// named. A position in a cycle may be replayed twice in one command; each
// replay appends its effective revision, and the last one is current.
func propagateInvestmentTransferRevisionsTx(ctx context.Context, tx *sql.Tx, bookID,
	causedByOperationID, auditEventID, actorUserID int64, createdAt string,
	revisions []InvestmentReplayTransferRevision) error {
	pending := revisions
	for round := 0; len(pending) > 0; round++ {
		if round == maxInvestmentTransferPropagationRounds {
			return fmt.Errorf("%w: internal transfer basis did not settle", ErrInvalidDisposalParams)
		}
		destinations := make(map[investmentReplayPositionKey]bool)
		for _, revision := range pending {
			destination, err := appendTransferLinkRevisionTx(ctx, tx, bookID, causedByOperationID,
				auditEventID, createdAt, revision)
			if err != nil {
				return err
			}
			destinations[destination] = true
		}
		keys := make([]investmentReplayPositionKey, 0, len(destinations))
		for key := range destinations {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b investmentReplayPositionKey) int {
			return cmp.Or(cmp.Compare(a.accountID, b.accountID), cmp.Compare(a.commodityID, b.commodityID),
				cmp.Compare(a.costCommodityID, b.costCommodityID))
		})
		var next []InvestmentReplayTransferRevision
		for _, key := range keys {
			intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, key.accountID,
				key.commodityID, key.costCommodityID, "long")
			if err != nil {
				return err
			}
			projection, err := simulateInvestmentReplayTx(ctx, tx, bookID, key.accountID,
				key.commodityID, key.costCommodityID, intents)
			if err != nil {
				return err
			}
			if err := persistInvestmentReplayPositionTx(ctx, tx, bookID, key.accountID, key.commodityID,
				key.costCommodityID, causedByOperationID, auditEventID, actorUserID, createdAt,
				intents, projection); err != nil {
				return err
			}
			next = append(next, projection.TransferRevisions...)
		}
		pending = next
	}
	return nil
}

// appendTransferLinkRevisionTx records one link's new effective depletion and
// returns the destination position that must replay from it.
func appendTransferLinkRevisionTx(ctx context.Context, tx *sql.Tx, bookID, causedByOperationID,
	auditEventID int64, createdAt string, revision InvestmentReplayTransferRevision) (investmentReplayPositionKey, error) {
	if revision.OperationID <= 0 || revision.LinkSeq <= 0 || revision.CostBasisValue < 0 ||
		(revision.SourceLotID > 0) == revision.PooledLot || (revision.PooledLot && len(revision.Depletions) == 0) {
		return investmentReplayPositionKey{}, fmt.Errorf("%w: transfer link revision is incomplete", ErrInvalidDisposalParams)
	}
	var destination investmentReplayPositionKey
	if err := tx.QueryRowContext(ctx, `SELECT d.account_id, d.commodity_id, d.cost_commodity_id
		FROM investment_transfer_lot_links x
		JOIN investment_transfer_facts f ON f.operation_id = x.operation_id
		JOIN investment_lots d ON d.id = x.destination_lot_id
		WHERE x.operation_id = ? AND x.link_seq = ? AND f.book_id = ? AND f.transfer_kind = 'internal'`,
		revision.OperationID, revision.LinkSeq, bookID).Scan(
		&destination.accountID, &destination.commodityID, &destination.costCommodityID); err != nil {
		return investmentReplayPositionKey{}, fmt.Errorf("read revised transfer link destination: %w", err)
	}
	var priorID sql.NullInt64
	priorSeq := 1
	err := tx.QueryRowContext(ctx, `SELECT id, revision_seq FROM latest_investment_transfer_link_revisions
		WHERE book_id = ? AND operation_id = ? AND link_seq = ?`,
		bookID, revision.OperationID, revision.LinkSeq).Scan(&priorID, &priorSeq)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return investmentReplayPositionKey{}, fmt.Errorf("read current transfer link revision: %w", err)
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
		return investmentReplayPositionKey{}, fmt.Errorf("append transfer link revision: %w", err)
	}
	revisionID, err := result.LastInsertId()
	if err != nil {
		return investmentReplayPositionKey{}, fmt.Errorf("read transfer link revision id: %w", err)
	}
	for index, depletion := range revision.Depletions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_link_revision_depletions (
			revision_id, depletion_seq, book_id, source_lot_id, quantity_value, quantity_scale,
			cost_basis_value, cost_basis_scale
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, revisionID, index+1, bookID, depletion.LotID,
			depletion.QuantityValue, depletion.QuantityScale, depletion.CostBasisValue, depletion.CostBasisScale); err != nil {
			return investmentReplayPositionKey{}, fmt.Errorf("append transfer revision depletion: %w", err)
		}
	}
	return destination, nil
}

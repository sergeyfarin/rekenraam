package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Internal transfer basis allocation (T-123). An individual-lot source moves
// the selected lots with their own remaining basis. An average-cost source
// moves a quantity out of its dated pool: the pool rate prices the carried
// basis, the final touched lot absorbs the exact integer remainder, and the
// surviving source lots are redistributed at the same rate — the algorithm a
// sale uses, so source plus destination basis is conserved exactly.
const (
	InternalTransferSelectedLots    = "selected_lots"
	InternalTransferAverageCostPool = "average_cost_pool"
)

type CreateInternalTransferParams struct {
	BookID               int64
	SourceAccountID      int64
	DestinationAccountID int64
	CommodityID          int64
	CostCommodityID      int64
	EffectiveOn          string
	// Allocations selects source lots. A pooled transfer leaves it empty and
	// sets PooledQuantity instead.
	Allocations         []LotAllocation
	PooledQuantityValue exact.Coefficient
	PooledQuantityScale int
	SourceEvidenceJSON  string
	// SourceCostBasisMethod and SourceMethodSource are the resolved default.
	// An open position's method-family lock overrides them inside the writer.
	SourceCostBasisMethod string
	SourceMethodSource    DisposalDecisionSource
}

// InternalTransferLink is one source lot's carried quantity and basis.
// DestinationLotID is zero in a preview, whose IDs are never durable.
type InternalTransferLink struct {
	SourceLotID           int64
	DestinationLotID      int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	CarriedBasisValue     int64
	CarriedBasisScale     int
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

type InternalTransferResult struct {
	BasisAllocation   string
	CostBasisMethod   string
	ResolutionTier    string
	Links             []InternalTransferLink
	DestinationLotIDs []int64
}

var (
	ErrAverageCostTransferRequiresPoolAllocation = errors.New("internal transfer from an average-cost position requires pooled basis allocation")
	ErrPooledTransferRequiresAverageCost         = errors.New("pooled basis allocation requires an average-cost source position")
)

// InternalTransferAllocation is the allocation an internal transfer applies
// for a position's method-family lock (empty before its first depletion) and
// its resolved default method. The lock wins.
func InternalTransferAllocation(lockFamily, defaultMethod string) string {
	family := lockFamily
	if family == "" {
		family = methodFamily(defaultMethod)
	}
	if family == "average_cost" {
		return InternalTransferAverageCostPool
	}
	return InternalTransferSelectedLots
}

// internalTransferPolicy is the allocation the writer applies. An open
// position's method-family lock wins over the current default, so a default
// changed after the first average-cost sale cannot move pooled basis as if it
// belonged to individual lots, nor the reverse.
type internalTransferPolicy struct {
	allocation string
	method     string
	source     DisposalDecisionSource
}

func internalTransferPolicyTx(ctx context.Context, tx *sql.Tx, transfer CreateInternalTransferParams) (internalTransferPolicy, error) {
	if !validCostBasisMethods[transfer.SourceCostBasisMethod] {
		return internalTransferPolicy{}, fmt.Errorf("%w: internal transfer source cost-basis method is required", ErrInvalidDisposalParams)
	}
	var family string
	err := tx.QueryRowContext(ctx, `SELECT method_family FROM investment_position_basis_state
		WHERE book_id = ? AND account_id = ? AND commodity_id = ?
			AND cost_commodity_id = ? AND position_side = 'long'`,
		transfer.BookID, transfer.SourceAccountID, transfer.CommodityID,
		transfer.CostCommodityID).Scan(&family)
	if errors.Is(err, sql.ErrNoRows) {
		family = ""
	} else if err != nil {
		return internalTransferPolicy{}, fmt.Errorf("read internal transfer source basis method: %w", err)
	}
	policy := internalTransferPolicy{method: transfer.SourceCostBasisMethod, source: transfer.SourceMethodSource}
	if family != "" && family != methodFamily(policy.method) {
		policy.source = DisposalDecisionSource{ResolutionTier: "position_lock"}
		policy.method = "specific_lot"
		if family == "average_cost" {
			policy.method = "average_cost"
		}
	}
	policy.allocation = InternalTransferAllocation(family, transfer.SourceCostBasisMethod)
	pooled := len(transfer.Allocations) == 0
	switch {
	case policy.allocation == InternalTransferAverageCostPool && !pooled:
		return internalTransferPolicy{}, ErrAverageCostTransferRequiresPoolAllocation
	case policy.allocation == InternalTransferSelectedLots && pooled:
		return internalTransferPolicy{}, ErrPooledTransferRequiresAverageCost
	}
	return policy, nil
}

func (r *InvestmentRepository) CreateInternalTransfer(ctx context.Context, journal CreateTransactionParams, transfer CreateInternalTransferParams) (TransactionRecord, InternalTransferResult, error) {
	return r.createInternalTransfer(ctx, journal, transfer, false)
}

// SimulateInternalTransfer runs the complete transfer writer, then rolls
// back. The carried amounts are what the same commit would write; destination
// lot IDs are cleared because they were never durable.
func (r *InvestmentRepository) SimulateInternalTransfer(ctx context.Context, journal CreateTransactionParams, transfer CreateInternalTransferParams) (SimulatedInvestmentWrite, InternalTransferResult, error) {
	transaction, result, err := r.createInternalTransfer(ctx, journal, transfer, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, InternalTransferResult{}, err
	}
	result.DestinationLotIDs = nil
	for index := range result.Links {
		result.Links[index].DestinationLotID = 0
	}
	return simulatedInvestmentWrite(transaction), result, nil
}

// createInternalTransfer keeps both security postings, every source depletion,
// linked destination lot, reconciliation invalidation and audit in one commit.
func (r *InvestmentRepository) createInternalTransfer(ctx context.Context, journal CreateTransactionParams, transfer CreateInternalTransferParams, preview bool) (TransactionRecord, InternalTransferResult, error) {
	write := executeInvestmentWriteTx[InternalTransferResult]
	if preview {
		write = previewInvestmentWriteTx[InternalTransferResult]
	}
	return write(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (InternalTransferResult, error) {
			if transfer.BookID <= 0 || transfer.SourceAccountID <= 0 || transfer.DestinationAccountID <= 0 ||
				transfer.SourceAccountID == transfer.DestinationAccountID ||
				(len(transfer.Allocations) == 0) != (transfer.PooledQuantityValue.Sign() > 0) {
				return InternalTransferResult{}, ErrInvalidDisposalParams
			}
			if err := requirePositionEventInOrderTx(ctx, tx, transfer.BookID, transfer.SourceAccountID,
				transfer.CommodityID, transfer.EffectiveOn, "an internal transfer"); err != nil {
				return InternalTransferResult{}, err
			}
			if err := requirePositionEventInOrderTx(ctx, tx, transfer.BookID, transfer.DestinationAccountID,
				transfer.CommodityID, transfer.EffectiveOn, "an internal transfer"); err != nil {
				return InternalTransferResult{}, err
			}
			policy, err := internalTransferPolicyTx(ctx, tx, transfer)
			if err != nil {
				return InternalTransferResult{}, err
			}
			operationID, err := investmentOperationIDTx(ctx, tx, transfer.BookID, transaction.ID)
			if err != nil {
				return InternalTransferResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_facts
				(operation_id, book_id, transfer_kind, effective_on, commodity_id,
				 source_account_id, destination_account_id, source_evidence_json, created_audit_event_id,
				 basis_allocation, cost_basis_method, method_resolution_tier,
				 method_account_version_id, method_profile_version_id)
				VALUES (?, ?, 'internal', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, transfer.BookID,
				transfer.EffectiveOn, transfer.CommodityID, transfer.SourceAccountID,
				transfer.DestinationAccountID, transfer.SourceEvidenceJSON, auditEventID,
				policy.allocation, policy.method, policy.source.ResolutionTier,
				nullablePositiveInt64(policy.source.AccountVersionID),
				nullablePositiveInt64(policy.source.ProfileVersionID)); err != nil {
				return InternalTransferResult{}, fmt.Errorf("record internal transfer fact: %w", err)
			}
			params := DisposeLotsParams{BookID: transfer.BookID, AccountID: transfer.SourceAccountID,
				CommodityID: transfer.CommodityID, CostCommodityID: transfer.CostCommodityID,
				TransactionID: transaction.ID, EventDate: transfer.EffectiveOn,
				EventKind: "transfer_out", MetadataJSON: transfer.SourceEvidenceJSON,
				CreatedAt: journal.CreatedAt, ActorUserID: journal.ActorUserID}
			var moved []LotDisposalRecord
			if policy.allocation == InternalTransferAverageCostPool {
				params.QuantityValue, params.QuantityScale = transfer.PooledQuantityValue, transfer.PooledQuantityScale
				moved, err = pooledTransferOutTx(ctx, tx, params, auditEventID)
			} else {
				moved, err = selectedLotsTransferOutTx(ctx, tx, params, transfer.Allocations, auditEventID)
			}
			if err != nil {
				return InternalTransferResult{}, err
			}
			result := InternalTransferResult{BasisAllocation: policy.allocation, CostBasisMethod: policy.method,
				ResolutionTier: policy.source.ResolutionTier}
			for index, depletion := range moved {
				link, err := openInternalTransferDestinationTx(ctx, tx, transfer, transaction, journal,
					operationID, auditEventID, index+1, depletion)
				if err != nil {
					return InternalTransferResult{}, err
				}
				result.Links = append(result.Links, link)
				result.DestinationLotIDs = append(result.DestinationLotIDs, link.DestinationLotID)
			}
			// Any move fixes the source position's method family, even before
			// its first sale; a fully moved position releases the lock.
			if err := updatePositionMethodFamilyTx(ctx, tx, params, policy.method, auditEventID); err != nil {
				return InternalTransferResult{}, fmt.Errorf("save internal transfer source basis method: %w", err)
			}
			return result, nil
		}, nil)
}

// selectedLotsTransferOutTx depletes each chosen lot at its own remaining
// basis. Valid only for an individual-lot position.
func selectedLotsTransferOutTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, allocations []LotAllocation, auditEventID int64) ([]LotDisposalRecord, error) {
	allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	moved := make([]LotDisposalRecord, 0, len(allocations))
	seen := make(map[int64]bool, len(allocations))
	for _, allocation := range allocations {
		if allocation.LotID <= 0 || seen[allocation.LotID] || allocation.QuantityValue.Sign() <= 0 {
			return nil, ErrInvalidDisposalParams
		}
		seen[allocation.LotID] = true
		depletion, err := disposeLotTx(ctx, tx, params, allocation.LotID,
			allocation.QuantityValue, allocation.QuantityScale, auditEventID, allocationScale)
		if err != nil {
			return nil, err
		}
		moved = append(moved, depletion)
	}
	if err := requirePositionBasisRangeTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID); err != nil {
		return nil, err
	}
	return moved, nil
}

// pooledTransferOutTx moves params.Quantity out of the dated average-cost
// pool. Commit and replay share it, so a replayed transfer reproduces its
// carried basis or names the transfer as a changed dependency.
func pooledTransferOutTx(ctx context.Context, tx *sql.Tx, params DisposeLotsParams, auditEventID int64) ([]LotDisposalRecord, error) {
	params.EventKind = "transfer_out"
	params.CostBasisMethod = "average_cost"
	if params.QuantityValue.Sign() <= 0 || !isDisposalCalendarDate(params.EventDate) {
		return nil, ErrInvalidDisposalParams
	}
	if err := enforcePositionMethodFamilyTx(ctx, tx, params, params.CostBasisMethod); err != nil {
		return nil, err
	}
	allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
	if err != nil {
		return nil, err
	}
	moved, err := disposeAverageCostTx(ctx, tx, params, auditEventID, allocationScale)
	if err != nil {
		return nil, err
	}
	if err := requirePositionBasisRangeTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID); err != nil {
		return nil, err
	}
	if err := updatePositionMethodFamilyTx(ctx, tx, params, params.CostBasisMethod, auditEventID); err != nil {
		return nil, err
	}
	return moved, nil
}

// openInternalTransferDestinationTx opens one destination lot for a source
// depletion and links them. The destination opens on the transfer date with
// the carried basis; the link keeps the source lot and original acquisition
// date. The destination's own method state is not changed: its lots join an
// average-cost pool there, or stay individual lots, like any other opening.
func openInternalTransferDestinationTx(ctx context.Context, tx *sql.Tx, transfer CreateInternalTransferParams,
	transaction TransactionRecord, journal CreateTransactionParams, operationID, auditEventID int64,
	linkSeq int, depletion LotDisposalRecord,
) (InternalTransferLink, error) {
	source, err := investmentLotByIDTx(ctx, tx, transfer.BookID, depletion.LotID)
	if err != nil {
		return InternalTransferLink{}, err
	}
	originalKnowledge, originalDate, err := internalTransferOriginalDateTx(ctx, tx, source)
	if err != nil {
		return InternalTransferLink{}, err
	}
	if err := linkLotEffectTx(ctx, tx, operationID, depletion.EventID); err != nil {
		return InternalTransferLink{}, err
	}
	destination, err := createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
		BookID: transfer.BookID, AccountID: transfer.DestinationAccountID,
		CommodityID: transfer.CommodityID, OpenedOn: transfer.EffectiveOn,
		SourceTransactionID: transaction.ID, QuantityValue: depletion.QuantityValue,
		QuantityScale: depletion.QuantityScale, CostBasisValue: depletion.CostBasisValue,
		CostBasisScale: depletion.CostBasisScale, CostCommodityID: transfer.CostCommodityID,
		MetadataJSON: `{"source":"internal_transfer"}`, EventKind: "transfer_in",
		CreatedAt: journal.CreatedAt, CreatedByUserID: journal.ActorUserID,
	}, auditEventID, false)
	if err != nil {
		return InternalTransferLink{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
		(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
		 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
		 original_date_knowledge, original_acquired_on, source_evidence_json)
		VALUES (?, ?, ?, ?, ?, ?, 'known', ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		operationID, linkSeq, depletion.LotID, destination.ID,
		depletion.QuantityValue, depletion.QuantityScale, exact.New(depletion.CostBasisValue),
		depletion.CostBasisScale, transfer.CostCommodityID, originalKnowledge, originalDate,
		transfer.SourceEvidenceJSON); err != nil {
		return InternalTransferLink{}, fmt.Errorf("link internal transfer lots: %w", err)
	}
	return InternalTransferLink{SourceLotID: depletion.LotID, DestinationLotID: destination.ID,
		QuantityValue: depletion.QuantityValue, QuantityScale: depletion.QuantityScale,
		CarriedBasisValue: depletion.CostBasisValue, CarriedBasisScale: depletion.CostBasisScale,
		OriginalDateKnowledge: originalKnowledge, OriginalAcquiredOn: originalDate}, nil
}

func internalTransferOriginalDateTx(ctx context.Context, tx *sql.Tx, source InvestmentLotRecord) (string, string, error) {
	var knowledge string
	var original sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT original_date_knowledge, original_acquired_on
		FROM investment_transfer_lot_links WHERE destination_lot_id = ?`, source.ID).Scan(&knowledge, &original)
	if errors.Is(err, sql.ErrNoRows) {
		return "known", source.OpenedOn, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read source lot original acquisition date: %w", err)
	}
	return knowledge, original.String, nil
}

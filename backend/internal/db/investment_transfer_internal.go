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

// Destination lineage of an internal transfer (T-135, ADR 0013). An
// individual-lot source always carries source lots. An average-cost source
// defaults to one pooled destination lot, dated by the latest original
// acquisition date among the units moved, which replay may revise without
// changing the lot; carrying source lots there is opt-in, and a replay that
// would change which lots its FIFO lineage depletes is refused.
const (
	InternalTransferSourceLots = "source_lots"
	InternalTransferPooledLot  = "pooled_lot"
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
	// DestinationLineage is source_lots or pooled_lot. Empty means the
	// allocation's default: source lots for selected lots, a pooled lot for a
	// pooled quantity.
	DestinationLineage string
}

// InternalTransferLink is one source lot's carried quantity and basis, or a
// pooled lot's whole move (SourceLotID zero). DestinationLotID is zero in a
// preview, whose IDs are never durable.
type InternalTransferLink struct {
	SourceLotID           int64
	DestinationLotID      int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	CarriedBasisValue     int64
	CarriedBasisScale     int
	BasisKnowledge        string // unknown leaves CarriedBasis unused
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

type InternalTransferResult struct {
	BasisAllocation    string
	DestinationLineage string
	CostBasisMethod    string
	ResolutionTier     string
	Links              []InternalTransferLink
	DestinationLotIDs  []int64
}

var (
	ErrAverageCostTransferRequiresPoolAllocation = errors.New("internal transfer from an average-cost position requires pooled basis allocation")
	ErrPooledTransferRequiresAverageCost         = errors.New("pooled basis allocation requires an average-cost source position")
	ErrInvalidTransferDestinationLineage         = errors.New("internal transfer destination lineage is invalid for its allocation")
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
	lineage    string
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
	switch {
	case transfer.DestinationLineage == "" && pooled:
		policy.lineage = InternalTransferPooledLot
	case transfer.DestinationLineage == "" || transfer.DestinationLineage == InternalTransferSourceLots:
		policy.lineage = InternalTransferSourceLots
	case transfer.DestinationLineage == InternalTransferPooledLot && pooled:
		policy.lineage = InternalTransferPooledLot
	default:
		return internalTransferPolicy{}, ErrInvalidTransferDestinationLineage
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
			if err := insertInternalTransferFactTx(ctx, tx, transfer, policy, operationID, auditEventID); err != nil {
				return InternalTransferResult{}, err
			}
			params := DisposeLotsParams{BookID: transfer.BookID, AccountID: transfer.SourceAccountID,
				CommodityID: transfer.CommodityID, CostCommodityID: transfer.CostCommodityID,
				TransactionID: transaction.ID, EventDate: transfer.EffectiveOn,
				EventKind: "transfer_out", MetadataJSON: transfer.SourceEvidenceJSON,
				CreatedAt: journal.CreatedAt, ActorUserID: journal.ActorUserID,
				// Unknown basis moves as unknown: the destination inherits it.
				AdmitUnknownBasis: true}
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
			result, err := openInternalTransferDestinationsTx(ctx, tx, transfer, policy, transaction, journal,
				operationID, auditEventID, moved, false)
			if err != nil {
				return InternalTransferResult{}, err
			}
			// Any move fixes the source position's method family, even before
			// its first sale; a fully moved position releases the lock.
			if err := updatePositionMethodFamilyTx(ctx, tx, params, policy.method, auditEventID); err != nil {
				return InternalTransferResult{}, fmt.Errorf("save internal transfer source basis method: %w", err)
			}
			return result, nil
		}, nil)
}

// insertInternalTransferFactTx records the transfer's immutable terms and
// the allocation policy that applied.
func insertInternalTransferFactTx(ctx context.Context, tx *sql.Tx, transfer CreateInternalTransferParams,
	policy internalTransferPolicy, operationID, auditEventID int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_facts
		(operation_id, book_id, transfer_kind, effective_on, commodity_id,
		 source_account_id, destination_account_id, source_evidence_json, created_audit_event_id,
		 basis_allocation, cost_basis_method, method_resolution_tier,
		 method_account_version_id, method_profile_version_id, destination_lineage)
		VALUES (?, ?, 'internal', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, transfer.BookID,
		transfer.EffectiveOn, transfer.CommodityID, transfer.SourceAccountID,
		transfer.DestinationAccountID, transfer.SourceEvidenceJSON, auditEventID,
		policy.allocation, policy.method, policy.source.ResolutionTier,
		nullablePositiveInt64(policy.source.AccountVersionID),
		nullablePositiveInt64(policy.source.ProfileVersionID), policy.lineage); err != nil {
		return fmt.Errorf("record internal transfer fact: %w", err)
	}
	return nil
}

// openInternalTransferDestinationsTx opens the destination lots for a
// source depletion and links them: one pooled lot, or one lot per depleted
// source lot. replayAdmission lets a correction open them behind later
// destination events, which its replay then orders.
func openInternalTransferDestinationsTx(ctx context.Context, tx *sql.Tx, transfer CreateInternalTransferParams,
	policy internalTransferPolicy, transaction TransactionRecord, journal CreateTransactionParams,
	operationID, auditEventID int64, moved []LotDisposalRecord, replayAdmission bool,
) (InternalTransferResult, error) {
	result := InternalTransferResult{BasisAllocation: policy.allocation, DestinationLineage: policy.lineage,
		CostBasisMethod: policy.method, ResolutionTier: policy.source.ResolutionTier}
	if policy.lineage == InternalTransferPooledLot {
		link, err := openPooledTransferDestinationTx(ctx, tx, transfer, transaction, journal,
			operationID, auditEventID, moved, replayAdmission)
		if err != nil {
			return InternalTransferResult{}, err
		}
		result.Links = []InternalTransferLink{link}
		result.DestinationLotIDs = []int64{link.DestinationLotID}
		return result, nil
	}
	for index, depletion := range moved {
		link, err := openInternalTransferDestinationTx(ctx, tx, transfer, transaction, journal,
			operationID, auditEventID, index+1, depletion, replayAdmission)
		if err != nil {
			return InternalTransferResult{}, err
		}
		result.Links = append(result.Links, link)
		result.DestinationLotIDs = append(result.DestinationLotIDs, link.DestinationLotID)
	}
	return result, nil
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
	if err := requirePositionBasisRangeQueryTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, params.AdmitUnknownBasis, PositionSideLong); err != nil {
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
	if err := requirePositionBasisRangeQueryTx(ctx, tx, params.BookID, params.AccountID,
		params.CommodityID, params.CostCommodityID, params.AdmitUnknownBasis, PositionSideLong); err != nil {
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
	linkSeq int, depletion LotDisposalRecord, replayAdmission bool,
) (InternalTransferLink, error) {
	source, err := investmentLotByIDTx(ctx, tx, transfer.BookID, depletion.LotID)
	if err != nil {
		return InternalTransferLink{}, err
	}
	originalKnowledge, originalDate, err := internalTransferOriginalDateTx(ctx, tx, source.ID, source.OpenedOn)
	if err != nil {
		return InternalTransferLink{}, err
	}
	if err := linkLotEffectTx(ctx, tx, operationID, depletion.EventID); err != nil {
		return InternalTransferLink{}, err
	}
	// An unknown depletion carries no amount into the destination opening.
	carriedValue, carriedScale := depletion.CostBasisValue, depletion.CostBasisScale
	if normalizedBasisKnowledge(depletion.BasisKnowledge) == InvestmentBasisUnknown {
		carriedValue, carriedScale = 0, 0
	}
	destination, err := createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
		BookID: transfer.BookID, AccountID: transfer.DestinationAccountID,
		CommodityID: transfer.CommodityID, OpenedOn: transfer.EffectiveOn,
		SourceTransactionID: transaction.ID, QuantityValue: depletion.QuantityValue,
		QuantityScale: depletion.QuantityScale, CostBasisValue: carriedValue,
		CostBasisScale: carriedScale, CostCommodityID: transfer.CostCommodityID,
		OpeningBasisKnowledge: normalizedBasisKnowledge(depletion.BasisKnowledge),
		MetadataJSON:          `{"source":"internal_transfer"}`, EventKind: "transfer_in",
		CreatedAt: journal.CreatedAt, CreatedByUserID: journal.ActorUserID,
	}, auditEventID, replayAdmission)
	if err != nil {
		return InternalTransferLink{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
		(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
		 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
		 original_date_knowledge, original_acquired_on, source_evidence_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		operationID, linkSeq, depletion.LotID, destination.ID,
		depletion.QuantityValue, depletion.QuantityScale, normalizedBasisKnowledge(depletion.BasisKnowledge),
		nullableBasisValue(depletion.CostBasisValue, depletion.BasisKnowledge),
		nullableBasisScale(depletion.CostBasisScale, depletion.BasisKnowledge), transfer.CostCommodityID, originalKnowledge, originalDate,
		transfer.SourceEvidenceJSON); err != nil {
		return InternalTransferLink{}, fmt.Errorf("link internal transfer lots: %w", err)
	}
	return InternalTransferLink{SourceLotID: depletion.LotID, DestinationLotID: destination.ID,
		QuantityValue: depletion.QuantityValue, QuantityScale: depletion.QuantityScale,
		CarriedBasisValue: depletion.CostBasisValue, CarriedBasisScale: depletion.CostBasisScale,
		BasisKnowledge:        normalizedBasisKnowledge(depletion.BasisKnowledge),
		OriginalDateKnowledge: originalKnowledge, OriginalAcquiredOn: originalDate}, nil
}

// openPooledTransferDestinationTx opens the single destination lot of a
// pooled_lot transfer (T-135). Every source depletion is linked to the
// operation as an effect; the one link carries their total quantity and basis
// and the latest original acquisition date among them.
func openPooledTransferDestinationTx(ctx context.Context, tx *sql.Tx, transfer CreateInternalTransferParams,
	transaction TransactionRecord, journal CreateTransactionParams, operationID, auditEventID int64,
	moved []LotDisposalRecord, replayAdmission bool,
) (InternalTransferLink, error) {
	if len(moved) == 0 {
		return InternalTransferLink{}, ErrInvalidDisposalParams
	}
	for _, depletion := range moved {
		if err := linkLotEffectTx(ctx, tx, operationID, depletion.EventID); err != nil {
			return InternalTransferLink{}, err
		}
	}
	pooled, err := pooledTransferTotalsTx(ctx, tx, transfer.BookID, moved)
	if err != nil {
		return InternalTransferLink{}, err
	}
	destination, err := createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
		BookID: transfer.BookID, AccountID: transfer.DestinationAccountID,
		CommodityID: transfer.CommodityID, OpenedOn: transfer.EffectiveOn,
		SourceTransactionID: transaction.ID, QuantityValue: pooled.quantityValue,
		QuantityScale: pooled.quantityScale, CostBasisValue: pooled.basisValue,
		CostBasisScale: pooled.basisScale, CostCommodityID: transfer.CostCommodityID,
		OpeningBasisKnowledge: pooled.knowledge,
		MetadataJSON:          `{"source":"internal_transfer"}`, EventKind: "transfer_in",
		CreatedAt: journal.CreatedAt, CreatedByUserID: journal.ActorUserID,
	}, auditEventID, replayAdmission)
	if err != nil {
		return InternalTransferLink{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
		(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
		 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
		 original_date_knowledge, original_acquired_on, source_evidence_json)
		VALUES (?, 1, NULL, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		operationID, destination.ID, pooled.quantityValue, pooled.quantityScale, pooled.knowledge,
		nullableBasisValue(pooled.basisValue, pooled.knowledge), nullableBasisScale(pooled.basisScale, pooled.knowledge), transfer.CostCommodityID,
		pooled.originalKnowledge, pooled.originalDate, transfer.SourceEvidenceJSON); err != nil {
		return InternalTransferLink{}, fmt.Errorf("link pooled internal transfer lot: %w", err)
	}
	return InternalTransferLink{DestinationLotID: destination.ID,
		QuantityValue: pooled.quantityValue, QuantityScale: pooled.quantityScale,
		CarriedBasisValue: pooled.basisValue, CarriedBasisScale: pooled.basisScale, BasisKnowledge: pooled.knowledge,
		OriginalDateKnowledge: pooled.originalKnowledge, OriginalAcquiredOn: pooled.originalDate}, nil
}

// pooledTransferTotals is what a pooled lot carries from its depletions.
type pooledTransferTotals struct {
	quantityValue     exact.Coefficient
	quantityScale     int
	basisValue        int64
	basisScale        int
	knowledge         string // unknown when any depletion is unknown
	originalKnowledge string
	originalDate      string
}

// pooledTransferTotalsTx sums a pool depletion exactly and dates it by the
// latest original acquisition date among the lots it took from. One unknown
// date makes the latest unknown. Commit and replay share it.
func pooledTransferTotalsTx(ctx context.Context, tx *sql.Tx, bookID int64, moved []LotDisposalRecord) (pooledTransferTotals, error) {
	quantity, basis := exact.NewScaledInt(), exact.NewScaledInt()
	totals := pooledTransferTotals{originalKnowledge: "known", knowledge: InvestmentBasisKnown}
	for _, depletion := range moved {
		quantity.AddCoefficient(depletion.QuantityValue, depletion.QuantityScale)
		if normalizedBasisKnowledge(depletion.BasisKnowledge) == InvestmentBasisUnknown {
			totals.knowledge = InvestmentBasisUnknown
		} else {
			basis.AddInt64(depletion.CostBasisValue, depletion.CostBasisScale)
		}
		source, err := investmentLotByIDTx(ctx, tx, bookID, depletion.LotID)
		if err != nil {
			return pooledTransferTotals{}, err
		}
		knowledge, date, err := internalTransferOriginalDateTx(ctx, tx, source.ID, source.OpenedOn)
		if err != nil {
			return pooledTransferTotals{}, err
		}
		if knowledge != "known" {
			totals.originalKnowledge = "unknown"
		} else if date > totals.originalDate {
			totals.originalDate = date
		}
	}
	if totals.originalKnowledge != "known" {
		totals.originalDate = ""
	}
	var err error
	if totals.quantityValue, err = quantity.Coefficient(); err != nil {
		return pooledTransferTotals{}, err
	}
	totals.quantityScale = quantity.Scale()
	if totals.knowledge == InvestmentBasisUnknown {
		return totals, nil // a partial sum is not the carried basis
	}
	if totals.basisValue, err = basis.Int64(); err != nil {
		return pooledTransferTotals{}, ErrInvestmentBasisRange
	}
	totals.basisScale = basis.Scale()
	return totals, nil
}

// internalTransferOriginalDateTx is a lot's effective original acquisition
// date: its own opening, or what the transfer that opened it currently carries.
func internalTransferOriginalDateTx(ctx context.Context, tx *sql.Tx, lotID int64, openedOn string) (string, string, error) {
	var knowledge string
	var original sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT original_date_knowledge, original_acquired_on
		FROM effective_investment_transfer_links WHERE destination_lot_id = ?`, lotID).Scan(&knowledge, &original)
	if errors.Is(err, sql.ErrNoRows) {
		return "known", openedOn, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read source lot original acquisition date: %w", err)
	}
	return knowledge, original.String, nil
}

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

type CreateInternalTransferParams struct {
	BookID                int64
	SourceAccountID       int64
	DestinationAccountID  int64
	CommodityID           int64
	CostCommodityID       int64
	EffectiveOn           string
	Allocations           []LotAllocation
	SourceEvidenceJSON    string
	SourceCostBasisMethod string
}

type InternalTransferResult struct {
	DestinationLotIDs []int64
}

var ErrAverageCostTransferRequiresPoolAllocation = errors.New("internal transfer from an average-cost position requires pooled basis allocation")

func requireInternalTransferBasisMethodTx(ctx context.Context, tx *sql.Tx, transfer CreateInternalTransferParams) error {
	if !validCostBasisMethods[transfer.SourceCostBasisMethod] {
		return fmt.Errorf("%w: internal transfer source cost-basis method is required", ErrInvalidDisposalParams)
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
		return fmt.Errorf("read internal transfer source basis method: %w", err)
	}
	if family == "average_cost" || methodFamily(transfer.SourceCostBasisMethod) == "average_cost" {
		return ErrAverageCostTransferRequiresPoolAllocation
	}
	return nil
}

// PreviewInternalTransferLots runs the same depletion and destination opening
// primitives as the writer inside a transaction that is always rolled back.
// The journal/reconciliation preview is computed separately by the service.
func (r *InvestmentRepository) PreviewInternalTransferLots(ctx context.Context, transfer CreateInternalTransferParams, actorUserID int64) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin internal transfer lot preview: %w", err)
	}
	defer rollbackTx(ctx, tx)
	if err := requirePositionEventInOrderTx(ctx, tx, transfer.BookID, transfer.SourceAccountID,
		transfer.CommodityID, transfer.EffectiveOn, "an internal transfer"); err != nil {
		return err
	}
	if err := requirePositionEventInOrderTx(ctx, tx, transfer.BookID, transfer.DestinationAccountID,
		transfer.CommodityID, transfer.EffectiveOn, "an internal transfer"); err != nil {
		return err
	}
	if err := requireInternalTransferBasisMethodTx(ctx, tx, transfer); err != nil {
		return err
	}
	params := DisposeLotsParams{BookID: transfer.BookID, AccountID: transfer.SourceAccountID,
		CommodityID: transfer.CommodityID, CostCommodityID: transfer.CostCommodityID,
		EventDate: transfer.EffectiveOn, EventKind: "transfer_out", MetadataJSON: transfer.SourceEvidenceJSON,
		CreatedAt: "1970-01-01T00:00:00Z", ActorUserID: actorUserID}
	allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
	if err != nil {
		return err
	}
	seen := make(map[int64]bool, len(transfer.Allocations))
	for _, allocation := range transfer.Allocations {
		if allocation.LotID <= 0 || seen[allocation.LotID] || allocation.QuantityValue.Sign() <= 0 {
			return ErrInvalidDisposalParams
		}
		seen[allocation.LotID] = true
		var auditEventID int64
		if err := tx.QueryRowContext(ctx, `SELECT created_audit_event_id FROM investment_lots
			WHERE book_id = ? AND id = ?`, transfer.BookID, allocation.LotID).Scan(&auditEventID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("read source lot audit for transfer preview: %w", err)
		}
		moved, err := disposeLotTx(ctx, tx, params, allocation.LotID,
			allocation.QuantityValue, allocation.QuantityScale, auditEventID, allocationScale)
		if err != nil {
			return err
		}
		_, err = createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
			BookID: transfer.BookID, AccountID: transfer.DestinationAccountID,
			CommodityID: transfer.CommodityID, OpenedOn: transfer.EffectiveOn,
			QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale,
			CostBasisValue: moved.CostBasisValue, CostBasisScale: moved.CostBasisScale,
			CostCommodityID: transfer.CostCommodityID, MetadataJSON: `{"source":"internal_transfer_preview"}`,
			EventKind: "transfer_in", CreatedAt: params.CreatedAt, CreatedByUserID: actorUserID,
		}, auditEventID, false)
		if err != nil {
			return err
		}
	}
	return requirePositionBasisRangeTx(ctx, tx, transfer.BookID, transfer.SourceAccountID,
		transfer.CommodityID, transfer.CostCommodityID)
}

// CreateInternalTransfer keeps both security postings, every source depletion,
// linked destination lot, reconciliation invalidation and audit in one commit.
func (r *InvestmentRepository) CreateInternalTransfer(ctx context.Context, journal CreateTransactionParams, transfer CreateInternalTransferParams) (TransactionRecord, InternalTransferResult, error) {
	return executeInvestmentWriteTx(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (InternalTransferResult, error) {
			if transfer.BookID <= 0 || transfer.SourceAccountID <= 0 || transfer.DestinationAccountID <= 0 ||
				transfer.SourceAccountID == transfer.DestinationAccountID || len(transfer.Allocations) == 0 {
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
			if err := requireInternalTransferBasisMethodTx(ctx, tx, transfer); err != nil {
				return InternalTransferResult{}, err
			}
			operationID, err := investmentOperationIDTx(ctx, tx, transfer.BookID, transaction.ID)
			if err != nil {
				return InternalTransferResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_facts
				(operation_id, book_id, transfer_kind, effective_on, commodity_id,
				 source_account_id, destination_account_id, source_evidence_json, created_audit_event_id)
				VALUES (?, ?, 'internal', ?, ?, ?, ?, ?, ?)`, operationID, transfer.BookID,
				transfer.EffectiveOn, transfer.CommodityID, transfer.SourceAccountID,
				transfer.DestinationAccountID, transfer.SourceEvidenceJSON, auditEventID); err != nil {
				return InternalTransferResult{}, fmt.Errorf("record internal transfer fact: %w", err)
			}
			params := DisposeLotsParams{BookID: transfer.BookID, AccountID: transfer.SourceAccountID,
				CommodityID: transfer.CommodityID, CostCommodityID: transfer.CostCommodityID,
				TransactionID: transaction.ID, EventDate: transfer.EffectiveOn,
				EventKind: "transfer_out", MetadataJSON: transfer.SourceEvidenceJSON,
				CreatedAt: journal.CreatedAt, ActorUserID: journal.ActorUserID}
			allocationScale, err := positionBasisAllocationScaleTx(ctx, tx, params)
			if err != nil {
				return InternalTransferResult{}, err
			}
			result := InternalTransferResult{DestinationLotIDs: make([]int64, 0, len(transfer.Allocations))}
			seen := make(map[int64]bool, len(transfer.Allocations))
			for index, allocation := range transfer.Allocations {
				if allocation.LotID <= 0 || seen[allocation.LotID] || allocation.QuantityValue.Sign() <= 0 {
					return InternalTransferResult{}, ErrInvalidDisposalParams
				}
				seen[allocation.LotID] = true
				source, err := investmentLotByIDTx(ctx, tx, transfer.BookID, allocation.LotID)
				if err != nil {
					return InternalTransferResult{}, err
				}
				if source.AccountID != transfer.SourceAccountID || source.CommodityID != transfer.CommodityID ||
					source.CostCommodityID != transfer.CostCommodityID || source.Status != "open" {
					return InternalTransferResult{}, ErrNotFound
				}
				originalKnowledge, originalDate, err := internalTransferOriginalDateTx(ctx, tx, source)
				if err != nil {
					return InternalTransferResult{}, err
				}
				moved, err := disposeLotTx(ctx, tx, params, allocation.LotID,
					allocation.QuantityValue, allocation.QuantityScale, auditEventID, allocationScale)
				if err != nil {
					return InternalTransferResult{}, err
				}
				if err := linkLotEffectTx(ctx, tx, operationID, moved.EventID); err != nil {
					return InternalTransferResult{}, err
				}
				destination, err := createLotWithAuditTx(ctx, tx, CreateInvestmentLotParams{
					BookID: transfer.BookID, AccountID: transfer.DestinationAccountID,
					CommodityID: transfer.CommodityID, OpenedOn: transfer.EffectiveOn,
					SourceTransactionID: transaction.ID, QuantityValue: allocation.QuantityValue,
					QuantityScale: allocation.QuantityScale, CostBasisValue: moved.CostBasisValue,
					CostBasisScale: moved.CostBasisScale, CostCommodityID: transfer.CostCommodityID,
					MetadataJSON: `{"source":"internal_transfer"}`, EventKind: "transfer_in",
					CreatedAt: journal.CreatedAt, CreatedByUserID: journal.ActorUserID,
				}, auditEventID, false)
				if err != nil {
					return InternalTransferResult{}, err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
					(operation_id, link_seq, source_lot_id, destination_lot_id, quantity_value, quantity_scale,
					 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
					 original_date_knowledge, original_acquired_on, source_evidence_json)
					VALUES (?, ?, ?, ?, ?, ?, 'known', ?, ?, ?, ?, NULLIF(?, ''), ?)`,
					operationID, index+1, allocation.LotID, destination.ID,
					allocation.QuantityValue, allocation.QuantityScale, exact.New(moved.CostBasisValue),
					moved.CostBasisScale, transfer.CostCommodityID, originalKnowledge, originalDate,
					transfer.SourceEvidenceJSON); err != nil {
					return InternalTransferResult{}, fmt.Errorf("link internal transfer lots: %w", err)
				}
				result.DestinationLotIDs = append(result.DestinationLotIDs, destination.ID)
			}
			if err := requirePositionBasisRangeTx(ctx, tx, transfer.BookID, transfer.SourceAccountID,
				transfer.CommodityID, transfer.CostCommodityID); err != nil {
				return InternalTransferResult{}, err
			}
			// A lot-specific move fixes the source position's method family even
			// before its first sale. A fully moved position releases that lock.
			if err := updatePositionMethodFamilyTx(ctx, tx, params, transfer.SourceCostBasisMethod, auditEventID); err != nil {
				return InternalTransferResult{}, fmt.Errorf("save internal transfer source basis method: %w", err)
			}
			return result, nil
		}, nil)
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

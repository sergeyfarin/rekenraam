package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// An external outbound transfer moves a long holding out of the book (slice 5,
// ADR 0013). Its source depletion follows the same allocation as an internal
// transfer: selected lots for an individual-lot source, a dated average-cost
// pool quantity otherwise. The basis it carries out, b, is a replay output of
// that depletion, never a broker figure, so it is computed inside the write
// and posted as a separate bridge journal (T −b, E +b) linked to the
// operation as 'transfer_bridge'. The writer's combined checkpoint guard nets
// it with the security journal.

// ErrExternalTransferBasisChanged refuses a history change that would move the
// basis an outbound transfer already carried out of the book, until dated
// bridge adjustments ship.
var ErrExternalTransferBasisChanged = errors.New("an outbound transfer's carried basis would change")

type CreateExternalTransferOutParams struct {
	BookID          int64
	SourceAccountID int64
	CommodityID     int64
	CostCommodityID int64
	EffectiveOn     string
	// Allocations selects source lots; a pooled transfer leaves it empty and
	// sets PooledQuantity instead.
	Allocations           []LotAllocation
	PooledQuantityValue   exact.Coefficient
	PooledQuantityScale   int
	SourceEvidenceJSON    string
	SourceCostBasisMethod string
	SourceMethodSource    DisposalDecisionSource
	Memo                  string
}

// ExternalTransferOutLink is one source lot's depletion carried out of the book.
type ExternalTransferOutLink struct {
	SourceLotID           int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	CarriedBasisValue     int64
	CarriedBasisScale     int
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

type ExternalTransferOutResult struct {
	BasisAllocation string
	CostBasisMethod string
	ResolutionTier  string
	Links           []ExternalTransferOutLink
	// Basis is the exact total carried out, the bridge amount.
	BasisValue int64
	BasisScale int
}

func (r *InvestmentRepository) CreateExternalTransferOut(ctx context.Context, journal CreateTransactionParams, transfer CreateExternalTransferOutParams) (TransactionRecord, ExternalTransferOutResult, error) {
	return r.createExternalTransferOut(ctx, journal, transfer, false)
}

// SimulateExternalTransferOut runs the complete writer, bridge included, then
// rolls back.
func (r *InvestmentRepository) SimulateExternalTransferOut(ctx context.Context, journal CreateTransactionParams, transfer CreateExternalTransferOutParams) (SimulatedInvestmentWrite, ExternalTransferOutResult, error) {
	transaction, result, err := r.createExternalTransferOut(ctx, journal, transfer, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, ExternalTransferOutResult{}, err
	}
	return simulatedInvestmentWrite(transaction), result, nil
}

func (r *InvestmentRepository) createExternalTransferOut(ctx context.Context, journal CreateTransactionParams, transfer CreateExternalTransferOutParams, preview bool) (TransactionRecord, ExternalTransferOutResult, error) {
	write := executeInvestmentWriteTx[ExternalTransferOutResult]
	if preview {
		write = previewInvestmentWriteTx[ExternalTransferOutResult]
	}
	return write(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (ExternalTransferOutResult, error) {
			return writeExternalTransferOutTx(ctx, tx, journal, transfer, transaction, auditEventID)
		}, nil)
}

func writeExternalTransferOutTx(ctx context.Context, tx *sql.Tx, journal CreateTransactionParams,
	transfer CreateExternalTransferOutParams, transaction TransactionRecord, auditEventID int64,
) (ExternalTransferOutResult, error) {
	if transfer.BookID <= 0 || transfer.SourceAccountID <= 0 || transfer.CostCommodityID <= 0 ||
		(len(transfer.Allocations) == 0) != (transfer.PooledQuantityValue.Sign() > 0) {
		return ExternalTransferOutResult{}, ErrInvalidDisposalParams
	}
	// Backdated admission through replay, with its dated bridge adjustments,
	// is a later slice; until then a later depletion refuses by name.
	if err := requirePositionEventInOrderTx(ctx, tx, transfer.BookID, transfer.SourceAccountID,
		transfer.CommodityID, transfer.EffectiveOn, "an outbound transfer"); err != nil {
		return ExternalTransferOutResult{}, err
	}
	policy, err := internalTransferPolicyTx(ctx, tx, CreateInternalTransferParams{
		BookID: transfer.BookID, SourceAccountID: transfer.SourceAccountID, CommodityID: transfer.CommodityID,
		CostCommodityID: transfer.CostCommodityID, Allocations: transfer.Allocations,
		PooledQuantityValue: transfer.PooledQuantityValue, PooledQuantityScale: transfer.PooledQuantityScale,
		SourceCostBasisMethod: transfer.SourceCostBasisMethod, SourceMethodSource: transfer.SourceMethodSource,
	})
	if err != nil {
		return ExternalTransferOutResult{}, err
	}
	operationID, err := investmentOperationIDTx(ctx, tx, transfer.BookID, transaction.ID)
	if err != nil {
		return ExternalTransferOutResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_facts
		(operation_id, book_id, transfer_kind, effective_on, commodity_id,
		 source_account_id, source_evidence_json, created_audit_event_id,
		 basis_allocation, cost_basis_method, method_resolution_tier,
		 method_account_version_id, method_profile_version_id)
		VALUES (?, ?, 'external_out', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, transfer.BookID,
		transfer.EffectiveOn, transfer.CommodityID, transfer.SourceAccountID, transfer.SourceEvidenceJSON,
		auditEventID, policy.allocation, policy.method, policy.source.ResolutionTier,
		nullablePositiveInt64(policy.source.AccountVersionID),
		nullablePositiveInt64(policy.source.ProfileVersionID)); err != nil {
		return ExternalTransferOutResult{}, fmt.Errorf("record outbound transfer fact: %w", err)
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
		return ExternalTransferOutResult{}, err
	}
	result := ExternalTransferOutResult{BasisAllocation: policy.allocation, CostBasisMethod: policy.method,
		ResolutionTier: policy.source.ResolutionTier}
	basis := exact.NewScaledInt()
	for index, depletion := range moved {
		if err := linkLotEffectTx(ctx, tx, operationID, depletion.EventID); err != nil {
			return ExternalTransferOutResult{}, err
		}
		source, err := investmentLotByIDTx(ctx, tx, transfer.BookID, depletion.LotID)
		if err != nil {
			return ExternalTransferOutResult{}, err
		}
		originalKnowledge, originalDate, err := internalTransferOriginalDateTx(ctx, tx, source.ID, source.OpenedOn)
		if err != nil {
			return ExternalTransferOutResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_transfer_lot_links
			(operation_id, link_seq, source_lot_id, quantity_value, quantity_scale,
			 basis_knowledge, carried_basis_value, carried_basis_scale, cost_commodity_id,
			 original_date_knowledge, original_acquired_on, source_evidence_json)
			VALUES (?, ?, ?, ?, ?, 'known', ?, ?, ?, ?, NULLIF(?, ''), ?)`,
			operationID, index+1, depletion.LotID, depletion.QuantityValue, depletion.QuantityScale,
			exact.New(depletion.CostBasisValue), depletion.CostBasisScale, transfer.CostCommodityID,
			originalKnowledge, originalDate, transfer.SourceEvidenceJSON); err != nil {
			return ExternalTransferOutResult{}, fmt.Errorf("link outbound transfer lot: %w", err)
		}
		basis.AddInt64(depletion.CostBasisValue, depletion.CostBasisScale)
		result.Links = append(result.Links, ExternalTransferOutLink{SourceLotID: depletion.LotID,
			QuantityValue: depletion.QuantityValue, QuantityScale: depletion.QuantityScale,
			CarriedBasisValue: depletion.CostBasisValue, CarriedBasisScale: depletion.CostBasisScale,
			OriginalDateKnowledge: originalKnowledge, OriginalAcquiredOn: originalDate})
	}
	if result.BasisValue, err = basis.Int64(); err != nil {
		return ExternalTransferOutResult{}, ErrInvestmentBasisRange
	}
	result.BasisScale = basis.Scale()
	if basis.Sign() > 0 {
		if _, err := postExternalTransferBridgeJournalTx(ctx, tx, transfer, operationID, basis,
			auditEventID, journal.ActorUserID, journal.CreatedAt); err != nil {
			return ExternalTransferOutResult{}, err
		}
	}
	// Any move fixes the source position's method family; a full move
	// releases it, exactly as for an internal transfer.
	if err := updatePositionMethodFamilyTx(ctx, tx, params, policy.method, auditEventID); err != nil {
		return ExternalTransferOutResult{}, fmt.Errorf("save outbound transfer source basis method: %w", err)
	}
	return result, nil
}

// postExternalTransferBridgeJournalTx posts the basis an outbound transfer
// carries out of the book, T −b and E +b in its cost currency, dated to the
// transfer and linked to its operation as 'transfer_bridge'.
func postExternalTransferBridgeJournalTx(ctx context.Context, tx *sql.Tx, transfer CreateExternalTransferOutParams,
	operationID int64, basis *exact.ScaledInt, auditEventID, actorUserID int64, createdAt string) (int64, error) {
	var tradingID, equityID int64
	if err := tx.QueryRowContext(ctx, `SELECT trading.id, equity.id FROM accounts trading
		JOIN accounts equity ON equity.book_id = trading.book_id
			AND equity.system_role = 'external_investment_transfer_equity'
		WHERE trading.book_id = ? AND trading.system_role = 'commodity_trading'`,
		transfer.BookID).Scan(&tradingID, &equityID); err != nil {
		return 0, fmt.Errorf("read outbound transfer bridge accounts: %w", err)
	}
	value, err := basis.Coefficient()
	if err != nil {
		return 0, err
	}
	posting := func(key string, account int64, quantity exact.Coefficient) PostingSpec {
		return PostingSpec{LineKey: key, AccountID: account, QuantityValue: quantity, QuantityScale: basis.Scale(),
			CommodityID: transfer.CostCommodityID, ReconciliationStatus: "uncleared", Memo: transfer.Memo, MetadataJSON: "{}"}
	}
	record, err := insertTransactionWithAuditEventTx(ctx, tx, CreateTransactionParams{
		BookID: transfer.BookID, ActorUserID: actorUserID, CreatedAt: createdAt,
		Spec: TransactionSpec{
			Status: "posted", TransactionKind: "investment", TransactionDate: transfer.EffectiveOn,
			Description:  transfer.Memo,
			MetadataJSON: fmt.Sprintf(`{"transfer_bridge_of_operation_id":%d}`, operationID),
			JournalEntries: []JournalEntrySpec{{EntryDate: transfer.EffectiveOn, EntryKind: "investment",
				Memo: transfer.Memo, MetadataJSON: "{}",
				Postings: []PostingSpec{
					posting("transfer-bridge-trading", tradingID, value.Negated()),
					posting("transfer-bridge-equity", equityID, value),
				}}},
		},
	}, auditEventID)
	if err != nil {
		return 0, fmt.Errorf("post outbound transfer bridge journal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
		(book_id, operation_id, transaction_version_id, link_seq, role)
		SELECT ?, ?, ?, COALESCE(MAX(link_seq), 0) + 1, 'transfer_bridge'
		FROM investment_operation_journal_links WHERE operation_id = ?`,
		transfer.BookID, operationID, record.VersionID, operationID); err != nil {
		return 0, fmt.Errorf("link outbound transfer bridge journal: %w", err)
	}
	return record.VersionID, nil
}

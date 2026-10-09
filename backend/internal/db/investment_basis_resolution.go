package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Sourced basis resolution (T-145, boundary 4). An external transfer in that
// recorded unknown basis is resolved by appending an audited fact: a
// basis_resolution operation whose journal posts the complete omitted bridge
// (T +b, E -b) dated to the transfer. The original link and lot stay unknown
// evidence; the effective link reads the resolution, and replay of the
// dependency closure resolves every quantity already sold or transferred as
// well as those still held. Outbound transfers reached by replay post their
// complete omitted bridge when their last unknown link becomes known.

var (
	// ErrTransferBasisNotUnknown refuses a resolution of a link whose basis is
	// already known, or already resolved.
	ErrTransferBasisNotUnknown = errors.New("the transfer's basis is not unknown")
	// ErrTransferBasisResolved refuses a correction of an inbound transfer whose
	// unknown basis has a resolution: the resolution is pinned to that transfer's
	// lot, quantity and cost currency and cannot silently follow a replacement.
	ErrTransferBasisResolved = errors.New("the transfer's basis has a sourced resolution")
)

type ResolveTransferBasisParams struct {
	BookID              int64
	TransferOperationID int64
	BasisValue          exact.Coefficient
	BasisScale          int
	SourceEvidenceJSON  string
}

// UnknownTransferBasis is an unknown inbound link a resolution may pin.
type UnknownTransferBasis struct {
	TransferOperationID int64
	LinkSeq             int
	LotID               int64
	AccountID           int64
	CommodityID         int64
	CostCommodityID     int64
	QuantityValue       exact.Coefficient
	QuantityScale       int
	EffectiveOn         string
}

// UnknownTransferBasisForOperation reads the effective external transfer in
// that still carries unknown basis. It is read again inside the write.
func (r *InvestmentRepository) UnknownTransferBasisForOperation(ctx context.Context, bookID, operationID int64) (UnknownTransferBasis, error) {
	return unknownTransferBasisQuery(ctx, r.database, bookID, operationID)
}

func unknownTransferBasisQuery(ctx context.Context, reader queryer, bookID, operationID int64) (UnknownTransferBasis, error) {
	rows, err := reader.QueryContext(ctx, `
		SELECT x.operation_id, x.link_seq, l.id, l.account_id, l.commodity_id, x.cost_commodity_id,
			x.quantity_value, x.quantity_scale, f.effective_on, x.basis_knowledge
		FROM investment_transfer_facts f
		JOIN effective_investment_operations o ON o.id = f.operation_id
		JOIN effective_investment_transfer_links x ON x.operation_id = f.operation_id
		JOIN investment_lots l ON l.id = x.destination_lot_id
		WHERE f.book_id = ? AND f.operation_id = ? AND f.transfer_kind = 'external_in'
		ORDER BY x.link_seq`, bookID, operationID)
	if err != nil {
		return UnknownTransferBasis{}, fmt.Errorf("read unknown transfer basis: %w", err)
	}
	defer rows.Close()
	var found []UnknownTransferBasis
	var unknown bool
	for rows.Next() {
		var basis UnknownTransferBasis
		var knowledge string
		if err := rows.Scan(&basis.TransferOperationID, &basis.LinkSeq, &basis.LotID, &basis.AccountID,
			&basis.CommodityID, &basis.CostCommodityID, &basis.QuantityValue, &basis.QuantityScale,
			&basis.EffectiveOn, &knowledge); err != nil {
			return UnknownTransferBasis{}, fmt.Errorf("scan unknown transfer basis: %w", err)
		}
		unknown = knowledge == InvestmentBasisUnknown
		found = append(found, basis)
	}
	if err := rows.Err(); err != nil {
		return UnknownTransferBasis{}, fmt.Errorf("iterate unknown transfer basis: %w", err)
	}
	if len(found) == 0 {
		return UnknownTransferBasis{}, ErrNotFound
	}
	if len(found) != 1 || !unknown {
		return UnknownTransferBasis{}, ErrTransferBasisNotUnknown
	}
	return found[0], nil
}

// ResolveTransferBasis appends the resolution and its bridge journal, then
// replays the resolved holding and every position its lots reached, all under
// one audit event and the caller's gain-impact policy.
func (r *InvestmentRepository) ResolveTransferBasis(ctx context.Context, journal CreateTransactionParams, params ResolveTransferBasisParams) (TransactionRecord, error) {
	transaction, _, err := executeInvestmentWriteTx(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
			return struct{}{}, resolveTransferBasisTx(ctx, tx, journal, params, transaction, auditEventID)
		}, nil)
	return transaction, err
}

// SimulateTransferBasisResolution runs the complete resolution, replay and
// gain comparison, then rolls back.
func (r *InvestmentRepository) SimulateTransferBasisResolution(ctx context.Context, journal CreateTransactionParams, params ResolveTransferBasisParams) (SimulatedInvestmentWrite, error) {
	transaction, _, err := previewInvestmentWriteTx(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
			return struct{}{}, resolveTransferBasisTx(ctx, tx, journal, params, transaction, auditEventID)
		}, nil)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func resolveTransferBasisTx(ctx context.Context, tx *sql.Tx, journal CreateTransactionParams, params ResolveTransferBasisParams,
	transaction TransactionRecord, auditEventID int64) error {
	if params.BookID <= 0 || params.TransferOperationID <= 0 || params.BasisValue.Sign() <= 0 ||
		params.BasisScale < 0 || params.BasisScale > 12 {
		return fmt.Errorf("%w: a resolution needs a positive known basis at a money scale", ErrInvalidDisposalParams)
	}
	// Every pinned fact is read again inside the write: the transfer must
	// still be effective, unknown and unresolved at commit.
	target, err := unknownTransferBasisQuery(ctx, tx, params.BookID, params.TransferOperationID)
	if err != nil {
		return err
	}
	if journal.Spec.TransactionDate != target.EffectiveOn {
		return fmt.Errorf("%w: a resolution is dated to its transfer", ErrInvalidDisposalParams)
	}
	operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
	if err != nil {
		return err
	}
	evidence := params.SourceEvidenceJSON
	if evidence == "" {
		evidence = "{}"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO investment_basis_resolutions
		(operation_id, book_id, transfer_operation_id, link_seq, lot_id, quantity_value, quantity_scale,
		 cost_commodity_id, basis_value, basis_scale, source_evidence_json, created_audit_event_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operationID, params.BookID, target.TransferOperationID,
		target.LinkSeq, target.LotID, target.QuantityValue, target.QuantityScale, target.CostCommodityID,
		params.BasisValue, params.BasisScale, evidence, auditEventID); err != nil {
		return fmt.Errorf("record transfer basis resolution: %w", err)
	}
	// The whole opening is resolved: replay revises the units already sold or
	// moved on as well as those still held, across the dependency closure.
	return replayCorrectedPositionTx(ctx, tx, params.BookID,
		investmentReplayPositionKey{target.AccountID, target.CommodityID, target.CostCommodityID},
		operationID, auditEventID, journal.ActorUserID, journal.CreatedAt)
}

// TransferBasisResolved reports whether an inbound transfer's unknown basis
// has an effective sourced resolution.
func (r *InvestmentRepository) TransferBasisResolved(ctx context.Context, bookID, operationID int64) (bool, error) {
	var resolved bool
	if err := r.database.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM investment_basis_resolutions r
		JOIN effective_investment_operations o ON o.id = r.operation_id
		WHERE r.book_id = ? AND r.transfer_operation_id = ?)`, bookID, operationID).Scan(&resolved); err != nil {
		return false, fmt.Errorf("read transfer basis resolution: %w", err)
	}
	return resolved, nil
}

// transferBasisResolvedTx reports whether an inbound transfer has an effective
// sourced resolution, which pins it against correction.
func transferBasisResolvedTx(ctx context.Context, reader *sql.Tx, bookID, operationID int64) (bool, error) {
	var resolved bool
	if err := reader.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM investment_basis_resolutions r
		JOIN effective_investment_operations o ON o.id = r.operation_id
		WHERE r.book_id = ? AND r.transfer_operation_id = ?)`, bookID, operationID).Scan(&resolved); err != nil {
		return false, fmt.Errorf("read transfer basis resolution: %w", err)
	}
	return resolved, nil
}

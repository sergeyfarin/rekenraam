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

// BasisResolutionOperationRecord is a transfer's effective resolution as a
// correction command reads it before the write; the guard reads it again
// inside (#168).
type BasisResolutionOperationRecord struct {
	OperationID           int64
	TransactionID         int64
	TransactionVersionID  int64
	CurrentVersionID      int64
	EventDate             string
	TransferOperationID   int64
	TransferTransactionID int64
	LinkSeq               int
	LotID                 int64
	AccountID             int64
	CommodityID           int64
	CostCommodityID       int64
	BasisValue            exact.Coefficient
	BasisScale            int
	SourceEvidenceJSON    string
	AlreadyCorrected      bool
}

// EffectiveTransferBasisResolution reads the effective resolution pinned to an
// inbound transfer operation, or ErrNotFound.
func (r *InvestmentRepository) EffectiveTransferBasisResolution(ctx context.Context, bookID, transferOperationID int64) (BasisResolutionOperationRecord, error) {
	return effectiveTransferBasisResolutionQuery(ctx, r.database, bookID, transferOperationID)
}

func effectiveTransferBasisResolutionQuery(ctx context.Context, reader saleOperationReader, bookID, transferOperationID int64) (BasisResolutionOperationRecord, error) {
	return basisResolutionOperationQuery(ctx, reader, bookID, `r.transfer_operation_id = ?
		AND EXISTS (SELECT 1 FROM effective_investment_operations effective WHERE effective.id = o.id)`, transferOperationID)
}

func basisResolutionOperationQuery(ctx context.Context, reader saleOperationReader, bookID int64, predicate string, argument int64) (BasisResolutionOperationRecord, error) {
	var record BasisResolutionOperationRecord
	var corrected int
	err := reader.QueryRowContext(ctx, `SELECT o.id, linked_version.transaction_id, link.transaction_version_id,
		current.id, o.event_date, r.transfer_operation_id, transfer_version.transaction_id, r.link_seq, r.lot_id,
		l.account_id, l.commodity_id, r.cost_commodity_id, r.basis_value, r.basis_scale, r.source_evidence_json,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_basis_resolutions r ON r.operation_id = o.id
		JOIN investment_lots l ON l.id = r.lot_id
		JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
		JOIN investment_operation_journal_links transfer_link ON transfer_link.operation_id = r.transfer_operation_id
			AND transfer_link.role = 'primary'
		JOIN transaction_versions transfer_version ON transfer_version.id = transfer_link.transaction_version_id
		WHERE o.book_id = ? AND o.operation_kind = 'basis_resolution' AND `+predicate,
		bookID, argument).Scan(&record.OperationID, &record.TransactionID, &record.TransactionVersionID,
		&record.CurrentVersionID, &record.EventDate, &record.TransferOperationID, &record.TransferTransactionID,
		&record.LinkSeq, &record.LotID, &record.AccountID, &record.CommodityID, &record.CostCommodityID,
		&record.BasisValue, &record.BasisScale, &record.SourceEvidenceJSON, &corrected)
	if errors.Is(err, sql.ErrNoRows) {
		return BasisResolutionOperationRecord{}, ErrNotFound
	}
	if err != nil {
		return BasisResolutionOperationRecord{}, fmt.Errorf("read basis resolution operation: %w", err)
	}
	record.AlreadyCorrected = corrected != 0
	return record, nil
}

// checkBasisResolutionForCorrectionTx re-reads the transfer's effective
// resolution inside the write: it must still be the one read, unchanged and
// posted as read.
func checkBasisResolutionForCorrectionTx(ctx context.Context, tx *sql.Tx, bookID int64, expected BasisResolutionOperationRecord) error {
	current, err := effectiveTransferBasisResolutionQuery(ctx, tx, bookID, expected.TransferOperationID)
	if errors.Is(err, ErrNotFound) || (err == nil && current.OperationID != expected.OperationID) {
		return ErrInvestmentOperationAlreadyCorrected
	}
	if err != nil {
		return err
	}
	if current != expected {
		return ErrInvestmentSaleChanged
	}
	return checkInvestmentSourceJournalTx(ctx, tx, bookID, current.TransactionID,
		current.EventDate, current.TransactionVersionID, current.CurrentVersionID)
}

func (record BasisResolutionOperationRecord) position() investmentReplayPositionKey {
	return investmentReplayPositionKey{record.AccountID, record.CommodityID, record.CostCommodityID}
}

// ReverseTransferBasisResolution posts the exact inverse of a resolution's
// bridge as a reversal operation. The link reads unknown again, and replay
// revises every position the lot reached; a dependent that cannot return to
// unknown basis (a known outbound or onward link) refuses with its operation
// named (#168).
func (r *InvestmentRepository) ReverseTransferBasisResolution(ctx context.Context, params CreateTransactionParams, expected BasisResolutionOperationRecord) (TransactionRecord, error) {
	return r.reverseTransferBasisResolution(ctx, params, expected, false)
}

// PreviewTransferBasisResolutionReversal runs the reversal and its replay,
// then rolls back.
func (r *InvestmentRepository) PreviewTransferBasisResolutionReversal(ctx context.Context, params CreateTransactionParams, expected BasisResolutionOperationRecord) (SimulatedInvestmentWrite, error) {
	transaction, err := r.reverseTransferBasisResolution(ctx, params, expected, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func (r *InvestmentRepository) reverseTransferBasisResolution(ctx context.Context, params CreateTransactionParams, expected BasisResolutionOperationRecord, preview bool) (TransactionRecord, error) {
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 || expected.AccountID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		!params.CorrectionOfTransactionID.Valid || params.CorrectionOfTransactionID.Int64 != expected.TransactionID {
		return TransactionRecord{}, fmt.Errorf("%w: basis resolution reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		return checkBasisResolutionForCorrectionTx(ctx, tx, params.BookID, expected)
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := investmentOperationIDTx(ctx, tx, params.BookID, transaction.ID)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, replayCorrectedPositionTx(ctx, tx, params.BookID, expected.position(),
			operationID, auditEventID, params.ActorUserID, params.CreatedAt)
	}
	var transaction TransactionRecord
	var err error
	if preview {
		transaction, _, err = previewInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect)
	} else {
		transaction, _, err = executeInvestmentWriteWithGuardTx(ctx, r.database, params, guard, effect, nil)
	}
	return transaction, err
}

type BasisResolutionReplacementRecord struct {
	Inverse     TransactionRecord
	Replacement TransactionRecord
}

// ReplaceTransferBasisResolution atomically cancels a resolution's bridge and
// appends its successor: a new fact pinned to the same link, lot, quantity and
// cost currency with its own complete bridge at the transfer date. Replay of
// the dependency closure revises sales, onward links and outbound bridges.
func (r *InvestmentRepository) ReplaceTransferBasisResolution(ctx context.Context, expected BasisResolutionOperationRecord,
	inverse, replacement CreateTransactionParams, params ResolveTransferBasisParams,
) (BasisResolutionReplacementRecord, error) {
	return r.replaceTransferBasisResolution(ctx, expected, inverse, replacement, params, false)
}

func (r *InvestmentRepository) SimulateTransferBasisResolutionReplacement(ctx context.Context, expected BasisResolutionOperationRecord,
	inverse, replacement CreateTransactionParams, params ResolveTransferBasisParams,
) (SimulatedInvestmentWrite, error) {
	record, err := r.replaceTransferBasisResolution(ctx, expected, inverse, replacement, params, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(record.Inverse, record.Replacement), nil
}

func (r *InvestmentRepository) replaceTransferBasisResolution(ctx context.Context, expected BasisResolutionOperationRecord,
	inverse, replacement CreateTransactionParams, params ResolveTransferBasisParams, preview bool,
) (BasisResolutionReplacementRecord, error) {
	if expected.OperationID <= 0 || inverse.BookID != params.BookID || inverse.BookID != replacement.BookID ||
		inverse.ActorUserID != replacement.ActorUserID || inverse.ActorUserID <= 0 ||
		inverse.Spec.InvestmentOperationKind != "" || inverse.Spec.TransactionKind != "investment" || inverse.Spec.Status != "posted" ||
		inverse.Spec.TransactionDate != expected.EventDate || replacement.Spec.TransactionKind != "investment" ||
		replacement.Spec.InvestmentOperationKind != "basis_resolution" || replacement.Spec.Status != "posted" ||
		replacement.Spec.TransactionDate != expected.EventDate || replacement.InvestmentCorrectionOfOperationID != expected.OperationID ||
		replacement.InvestmentCorrectionMode != "replace" || replacement.InvestmentCorrectionReason == "" ||
		!inverse.CorrectionOfTransactionID.Valid || inverse.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
		replacement.CorrectionOfTransactionID != inverse.CorrectionOfTransactionID ||
		params.TransferOperationID != expected.TransferOperationID {
		return BasisResolutionReplacementRecord{}, fmt.Errorf("%w: basis resolution replacement is incomplete", ErrInvalidDisposalParams)
	}
	write := executeInvestmentJournalsWithGuardTx[struct{}]
	if preview {
		write = previewInvestmentJournalsWithGuardTx[struct{}]
	}
	journals, _, err := write(ctx, r.database, []CreateTransactionParams{inverse, replacement},
		func(tx *sql.Tx) error {
			return checkBasisResolutionForCorrectionTx(ctx, tx, params.BookID, expected)
		},
		func(tx *sql.Tx, journals []TransactionRecord, auditID int64) (struct{}, error) {
			opID, err := investmentOperationIDTx(ctx, tx, params.BookID, journals[1].ID)
			if err != nil {
				return struct{}{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
				(book_id, operation_id, transaction_version_id, link_seq, role) VALUES (?, ?, ?, 2, 'reversal')`,
				params.BookID, opID, journals[0].VersionID); err != nil {
				return struct{}{}, fmt.Errorf("link basis resolution inverse: %w", err)
			}
			// With its predecessor superseded the link reads unknown again,
			// so the successor is pinned exactly as a first resolution is.
			return struct{}{}, resolveTransferBasisTx(ctx, tx, replacement, params, journals[1], auditID)
		}, nil)
	if err != nil {
		return BasisResolutionReplacementRecord{}, err
	}
	return BasisResolutionReplacementRecord{Inverse: journals[0], Replacement: journals[1]}, nil
}

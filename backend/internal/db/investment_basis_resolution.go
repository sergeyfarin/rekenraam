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
// one audit event and the caller's gain-impact policy. A known zero has no
// bridge: its header is audit-only and the operation is journal-free (#168).
func (r *InvestmentRepository) ResolveTransferBasis(ctx context.Context, journal CreateTransactionParams, params ResolveTransferBasisParams) (TransactionRecord, error) {
	transaction, _, err := executeInvestmentWriteTx(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
			operationID, err := basisResolutionOperationTx(ctx, tx, journal, transaction, auditEventID)
			if err != nil {
				return struct{}{}, err
			}
			return struct{}{}, resolveTransferBasisTx(ctx, tx, journal, params, operationID, auditEventID)
		}, nil)
	return transaction, err
}

// basisResolutionOperationTx is the operation a resolution's header names: a
// journal-free operation for an audit-only header, else the journal's own.
func basisResolutionOperationTx(ctx context.Context, tx *sql.Tx, header CreateTransactionParams,
	transaction TransactionRecord, auditEventID int64) (int64, error) {
	if header.AuditOnly {
		return insertJournalFreeInvestmentOperationTx(ctx, tx, header, auditEventID)
	}
	return investmentOperationIDTx(ctx, tx, header.BookID, transaction.ID)
}

// SimulateTransferBasisResolution runs the complete resolution, replay and
// gain comparison, then rolls back.
func (r *InvestmentRepository) SimulateTransferBasisResolution(ctx context.Context, journal CreateTransactionParams, params ResolveTransferBasisParams) (SimulatedInvestmentWrite, error) {
	transaction, _, err := previewInvestmentWriteTx(ctx, r.database, journal,
		func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
			operationID, err := basisResolutionOperationTx(ctx, tx, journal, transaction, auditEventID)
			if err != nil {
				return struct{}{}, err
			}
			return struct{}{}, resolveTransferBasisTx(ctx, tx, journal, params, operationID, auditEventID)
		}, nil)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	return simulatedInvestmentWrite(transaction), nil
}

func resolveTransferBasisTx(ctx context.Context, tx *sql.Tx, header CreateTransactionParams, params ResolveTransferBasisParams,
	operationID, auditEventID int64) error {
	if params.BookID <= 0 || params.TransferOperationID <= 0 || params.BasisValue.Sign() < 0 ||
		params.BasisScale < 0 || params.BasisScale > 12 {
		return fmt.Errorf("%w: a resolution needs a nonnegative known basis at a money scale", ErrInvalidDisposalParams)
	}
	// A known zero posts no bridge, so only it is journal-free (#168).
	if (params.BasisValue.Sign() == 0) != header.AuditOnly {
		return fmt.Errorf("%w: only a known-zero resolution is journal-free", ErrInvalidDisposalParams)
	}
	// Every pinned fact is read again inside the write: the transfer must
	// still be effective, unknown and unresolved at commit.
	target, err := unknownTransferBasisQuery(ctx, tx, params.BookID, params.TransferOperationID)
	if err != nil {
		return err
	}
	if header.Spec.TransactionDate != target.EffectiveOn {
		return fmt.Errorf("%w: a resolution is dated to its transfer", ErrInvalidDisposalParams)
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
		operationID, auditEventID, header.ActorUserID, header.CreatedAt)
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
// inside (#168). A journal-free known zero has zero transaction and versions.
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
	err := reader.QueryRowContext(ctx, `SELECT o.id, COALESCE(linked_version.transaction_id, 0),
		COALESCE(link.transaction_version_id, 0), COALESCE(current.id, 0), o.event_date, r.transfer_operation_id, transfer_version.transaction_id, r.link_seq, r.lot_id,
		l.account_id, l.commodity_id, r.cost_commodity_id, r.basis_value, r.basis_scale, r.source_evidence_json,
		EXISTS(SELECT 1 FROM investment_operations successor WHERE successor.correction_of_operation_id = o.id)
		FROM investment_operations o
		JOIN investment_basis_resolutions r ON r.operation_id = o.id
		JOIN investment_lots l ON l.id = r.lot_id
		LEFT JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.book_id = o.book_id AND link.role = 'primary'
		LEFT JOIN transaction_versions linked_version ON linked_version.id = link.transaction_version_id
		LEFT JOIN current_transaction_versions current ON current.transaction_id = linked_version.transaction_id
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
	if current.TransactionID == 0 {
		return nil // a journal-free known zero has no journal to recheck
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
	// A journal-free zero is reversed by a journal-free reversal; a bridged
	// resolution by the inverse of its bridge.
	journalFree := expected.TransactionID == 0
	if params.BookID <= 0 || params.ActorUserID <= 0 || expected.OperationID <= 0 || expected.AccountID <= 0 ||
		params.Spec.InvestmentOperationKind != "reversal" || params.Spec.TransactionKind != "investment" ||
		params.Spec.Status != "posted" || params.Spec.TransactionDate != expected.EventDate ||
		params.InvestmentCorrectionOfOperationID != expected.OperationID ||
		params.InvestmentCorrectionMode != "reverse" || params.InvestmentCorrectionReason == "" ||
		params.AuditOnly != journalFree || params.CorrectionOfTransactionID.Valid == journalFree ||
		(!journalFree && params.CorrectionOfTransactionID.Int64 != expected.TransactionID) {
		return TransactionRecord{}, fmt.Errorf("%w: basis resolution reversal is incomplete", ErrInvalidDisposalParams)
	}
	guard := func(tx *sql.Tx) error {
		return checkBasisResolutionForCorrectionTx(ctx, tx, params.BookID, expected)
	}
	effect := func(tx *sql.Tx, transaction TransactionRecord, auditEventID int64) (struct{}, error) {
		operationID, err := basisResolutionOperationTx(ctx, tx, params, transaction, auditEventID)
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
	Inverse     TransactionRecord // zero when the predecessor was a journal-free zero
	Replacement TransactionRecord // zero when the successor is a journal-free zero
}

// BasisResolutionReplacement is the write a replacement makes. Inverse is the
// inverse of the predecessor's bridge, nil for a journal-free zero
// predecessor. Successor is the new bridge journal, or an audit-only header
// for a known-zero successor, which is journal-free (#168).
type BasisResolutionReplacement struct {
	Inverse   *CreateTransactionParams
	Successor CreateTransactionParams
}

// ReplaceTransferBasisResolution atomically cancels a resolution's bridge and
// appends its successor: a new fact pinned to the same link, lot, quantity and
// cost currency with its own complete bridge at the transfer date. Replay of
// the dependency closure revises sales, onward links and outbound bridges.
func (r *InvestmentRepository) ReplaceTransferBasisResolution(ctx context.Context, expected BasisResolutionOperationRecord,
	write BasisResolutionReplacement, params ResolveTransferBasisParams,
) (BasisResolutionReplacementRecord, error) {
	return r.replaceTransferBasisResolution(ctx, expected, write, params, false)
}

func (r *InvestmentRepository) SimulateTransferBasisResolutionReplacement(ctx context.Context, expected BasisResolutionOperationRecord,
	write BasisResolutionReplacement, params ResolveTransferBasisParams,
) (SimulatedInvestmentWrite, error) {
	record, err := r.replaceTransferBasisResolution(ctx, expected, write, params, true)
	if err != nil {
		return SimulatedInvestmentWrite{}, err
	}
	// The command's first journal carries the gain impact.
	var journals []TransactionRecord
	for _, journal := range []TransactionRecord{record.Inverse, record.Replacement} {
		if journal.ID != 0 {
			journals = append(journals, journal)
		}
	}
	return simulatedInvestmentWrite(journals...), nil
}

func (r *InvestmentRepository) replaceTransferBasisResolution(ctx context.Context, expected BasisResolutionOperationRecord,
	write BasisResolutionReplacement, params ResolveTransferBasisParams, preview bool,
) (BasisResolutionReplacementRecord, error) {
	successor := write.Successor
	predecessorJournalFree := expected.TransactionID == 0
	if expected.OperationID <= 0 || successor.BookID != params.BookID || successor.ActorUserID <= 0 ||
		successor.Spec.TransactionKind != "investment" || successor.Spec.InvestmentOperationKind != "basis_resolution" ||
		successor.Spec.Status != "posted" || successor.Spec.TransactionDate != expected.EventDate ||
		successor.InvestmentCorrectionOfOperationID != expected.OperationID ||
		successor.InvestmentCorrectionMode != "replace" || successor.InvestmentCorrectionReason == "" ||
		params.TransferOperationID != expected.TransferOperationID ||
		(write.Inverse == nil) != predecessorJournalFree || (write.Inverse == nil && successor.AuditOnly) {
		return BasisResolutionReplacementRecord{}, fmt.Errorf("%w: basis resolution replacement is incomplete", ErrInvalidDisposalParams)
	}
	var journals []CreateTransactionParams
	if inverse := write.Inverse; inverse != nil {
		if inverse.BookID != params.BookID || inverse.ActorUserID != successor.ActorUserID ||
			inverse.Spec.InvestmentOperationKind != "" || inverse.Spec.TransactionKind != "investment" ||
			inverse.Spec.Status != "posted" || inverse.Spec.TransactionDate != expected.EventDate ||
			!inverse.CorrectionOfTransactionID.Valid || inverse.CorrectionOfTransactionID.Int64 != expected.TransactionID ||
			(!successor.AuditOnly && successor.CorrectionOfTransactionID != inverse.CorrectionOfTransactionID) {
			return BasisResolutionReplacementRecord{}, fmt.Errorf("%w: basis resolution replacement is incomplete", ErrInvalidDisposalParams)
		}
		journals = append(journals, *inverse)
	}
	if !successor.AuditOnly {
		journals = append(journals, successor)
	}
	run := executeInvestmentJournalsWithGuardTx[struct{}]
	if preview {
		run = previewInvestmentJournalsWithGuardTx[struct{}]
	}
	records, _, err := run(ctx, r.database, journals,
		func(tx *sql.Tx) error {
			return checkBasisResolutionForCorrectionTx(ctx, tx, params.BookID, expected)
		},
		func(tx *sql.Tx, records []TransactionRecord, auditID int64) (struct{}, error) {
			var successorJournal TransactionRecord
			if !successor.AuditOnly {
				successorJournal = records[len(records)-1]
			}
			opID, err := basisResolutionOperationTx(ctx, tx, successor, successorJournal, auditID)
			if err != nil {
				return struct{}{}, err
			}
			if write.Inverse != nil {
				if _, err := tx.ExecContext(ctx, `INSERT INTO investment_operation_journal_links
					(book_id, operation_id, transaction_version_id, link_seq, role) VALUES (?, ?, ?, 2, 'reversal')`,
					params.BookID, opID, records[0].VersionID); err != nil {
					return struct{}{}, fmt.Errorf("link basis resolution inverse: %w", err)
				}
			}
			// With its predecessor superseded the link reads unknown again,
			// so the successor is pinned exactly as a first resolution is.
			return struct{}{}, resolveTransferBasisTx(ctx, tx, successor, params, opID, auditID)
		}, nil)
	if err != nil {
		return BasisResolutionReplacementRecord{}, err
	}
	var record BasisResolutionReplacementRecord
	if write.Inverse != nil {
		record.Inverse = records[0]
	}
	if !successor.AuditOnly {
		record.Replacement = records[len(records)-1]
	}
	return record, nil
}

// BasisResolutionHistoryRecord is one resolution fact pinned to a transfer,
// with how it stands now: effective, superseded by a replacement, or
// reversed (#168). TransactionID is zero for a journal-free known zero.
type BasisResolutionHistoryRecord struct {
	OperationID        int64
	TransactionID      int64
	BasisValue         exact.Coefficient
	BasisScale         int
	SourceEvidenceJSON string
	CorrectionMode     string
	Reason             string
	CreatedAt          string
	Status             string
	// ReversalReason is why a reversed resolution was withdrawn.
	ReversalReason string
}

// TransferBasisResolutionHistory lists every resolution fact pinned to an
// inbound transfer operation, oldest first.
func (r *InvestmentRepository) TransferBasisResolutionHistory(ctx context.Context, bookID, transferOperationID int64) ([]BasisResolutionHistoryRecord, error) {
	rows, err := r.database.QueryContext(ctx, `SELECT o.id, COALESCE(v.transaction_id, 0), r.basis_value, r.basis_scale,
		r.source_evidence_json, COALESCE(o.correction_mode, ''), COALESCE(a.reason, ''), o.created_at,
		CASE WHEN EXISTS (SELECT 1 FROM effective_investment_operations e WHERE e.id = o.id) THEN 'effective'
			WHEN EXISTS (SELECT 1 FROM investment_operations s WHERE s.correction_of_operation_id = o.id
				AND s.correction_mode = 'reverse') THEN 'reversed'
			ELSE 'superseded' END,
		COALESCE((SELECT s.correction_reason FROM investment_operations s
			WHERE s.correction_of_operation_id = o.id AND s.correction_mode = 'reverse'), '')
		FROM investment_basis_resolutions r
		JOIN investment_operations o ON o.id = r.operation_id
		JOIN audit_events a ON a.id = o.created_audit_event_id
		LEFT JOIN investment_operation_journal_links l ON l.operation_id = o.id AND l.role = 'primary'
		LEFT JOIN transaction_versions v ON v.id = l.transaction_version_id
		WHERE r.book_id = ? AND r.transfer_operation_id = ?
		ORDER BY o.id`, bookID, transferOperationID)
	if err != nil {
		return nil, fmt.Errorf("read basis resolution history: %w", err)
	}
	defer rows.Close()
	var history []BasisResolutionHistoryRecord
	for rows.Next() {
		var record BasisResolutionHistoryRecord
		if err := rows.Scan(&record.OperationID, &record.TransactionID, &record.BasisValue, &record.BasisScale,
			&record.SourceEvidenceJSON, &record.CorrectionMode, &record.Reason, &record.CreatedAt, &record.Status, &record.ReversalReason); err != nil {
			return nil, fmt.Errorf("scan basis resolution history: %w", err)
		}
		history = append(history, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate basis resolution history: %w", err)
	}
	return history, nil
}

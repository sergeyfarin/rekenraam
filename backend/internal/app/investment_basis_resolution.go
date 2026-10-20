package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ResolveTransferBasisInput resolves the unknown basis an external transfer in
// recorded (T-145). The basis is the transfer's total sourced basis in its own
// cost currency; it is never converted from another currency.
type ResolveTransferBasisInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	// TransactionID is the effective transfer-in transaction.
	TransactionID      int64
	BasisValue         int64
	BasisScale         int
	SourceEvidenceJSON string
	Reason             string
	// ReconciliationOverride lets the dated bridge invalidate affected
	// checkpoints; GainImpactAcknowledgement echoes the preview token for the
	// sales and transfers the resolution revises.
	ReconciliationOverride    CheckpointOverride
	GainImpactAcknowledgement string
}

func (s *InvestmentService) resolveTransferBasisWrite(ctx context.Context, input ResolveTransferBasisInput) (db.CreateTransactionParams, db.ResolveTransferBasisParams, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "owner and transfer id are required"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolution reason is required"}
	}
	operation, err := s.repository.TransferOperationByTransactionID(ctx, BookID, input.TransactionID)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, mapTransferCorrectionError(err)
	}
	if operation.TransferKind != "external_in" || operation.AlreadyCorrected {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ErrInvestmentTransferBasisNotUnknown
	}
	target, err := s.repository.UnknownTransferBasisForOperation(ctx, BookID, operation.OperationID)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, mapTransferCorrectionError(err)
	}
	return s.transferBasisResolutionJournal(ctx, input, operation.OperationID, target.CostCommodityID, target.EffectiveOn)
}

// transferBasisResolutionJournal prepares a resolution's complete bridge
// journal for a transfer's pinned cost currency and date. A known zero has no
// bridge: it gets an audit-only header and a journal-free operation (#168).
func (s *InvestmentService) transferBasisResolutionJournal(ctx context.Context, input ResolveTransferBasisInput,
	transferOperationID, costCommodityID int64, effectiveOn string,
) (db.CreateTransactionParams, db.ResolveTransferBasisParams, error) {
	if input.BasisValue < 0 || input.BasisScale < 0 || input.BasisScale > 12 {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolved basis must be nonnegative at a valid money scale"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil || reason == "" {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolution reason is required"}
	}
	evidence, err := cleanBasisResolutionEvidence(input.SourceEvidenceJSON)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	params := db.ResolveTransferBasisParams{BookID: BookID, TransferOperationID: transferOperationID,
		BasisValue: exact.New(input.BasisValue), BasisScale: input.BasisScale, SourceEvidenceJSON: evidence}
	if input.BasisValue == 0 {
		return s.journalFreeInvestmentHeader(input.OwnerUserID, input.AuthSessionID, input.RequestID,
			"investment.basis_resolution", reason, "basis_resolution", effectiveOn, input.ReconciliationOverride), params, nil
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, fmt.Errorf("resolve commodity trading account: %w", err)
	}
	equityID, err := s.repository.ExternalInvestmentTransferEquityAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "external investment transfer equity account is required"}
		}
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	// The complete omitted inbound bridge, dated to the transfer.
	postings := []PostingInput{
		{AccountID: tradingID, CommodityID: costCommodityID, QuantityValue: exact.New(input.BasisValue), QuantityScale: input.BasisScale},
		{AccountID: equityID, CommodityID: costCommodityID, QuantityValue: exact.New(-input.BasisValue), QuantityScale: input.BasisScale},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OriginType: "browser_api", Operation: "investment.basis_resolution", ChangeReason: reason,
		ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "basis_resolution",
			TransactionDate: effectiveOn, Description: reason, MetadataJSON: evidence,
			JournalEntries: []JournalEntryInput{{EntryDate: effectiveOn, EntryKind: "investment", Memo: reason, Postings: postings}},
		},
	}, nil)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	return journal, params, nil
}

// journalFreeInvestmentHeader is the audit-only header of a journal-free
// investment command (#168): who, when and why, and the operation kind and
// date its effect records, with no journal.
func (s *InvestmentService) journalFreeInvestmentHeader(ownerUserID, authSessionID int64, requestID, operation, reason,
	kind, eventDate string, reconciliationOverride CheckpointOverride,
) db.CreateTransactionParams {
	return db.CreateTransactionParams{
		BookID: BookID, ActorUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID,
		OriginType: "browser_api", Operation: operation, ChangeReason: reason,
		CreatedAt:              s.transactionService.now().UTC().Format(time.RFC3339),
		ReconciliationOverride: reconciliationOverride.Allowed, ReviewedCheckpointIDs: reconciliationOverride.Reviewed,
		InvalidateCheckpointReason: reason, AuditOnly: true,
		Spec: db.TransactionSpec{Status: "posted", TransactionKind: "investment",
			InvestmentOperationKind: kind, TransactionDate: eventDate},
	}
}

// ResolveTransferBasisImpact runs the complete resolution, replay and gain
// comparison, then rolls back, so a preview reports what the commit would do.
func (s *InvestmentService) ResolveTransferBasisImpact(ctx context.Context, input ResolveTransferBasisInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	journal, params, err := s.resolveTransferBasisWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateTransferBasisResolution(ctx, journal, params)
	if err != nil {
		return ReconciliationImpact{}, mapTransferCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// ResolveTransferBasis records the sourced basis of an unknown inbound
// transfer. Every sale and transfer its lot reached is revised by replay; the
// changed gains need the preview's acknowledgement.
func (s *InvestmentService) ResolveTransferBasis(ctx context.Context, input ResolveTransferBasisInput) (*Transaction, error) {
	journal, params, err := s.resolveTransferBasisWrite(ctx, input)
	if err != nil {
		return nil, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, err := s.repository.ResolveTransferBasis(ctx, journal, params)
	if err != nil {
		return nil, mapTransferCorrectionError(err)
	}
	enriched, err := s.transactionService.enrichRecords(ctx, transaction)
	if err != nil {
		return nil, err
	}
	return optionalTransaction(enriched[0]), nil
}

var (
	// ErrInvestmentTransferBasisNotResolved refuses correcting the resolution
	// of a transfer that has no effective one.
	ErrInvestmentTransferBasisNotResolved        = errors.New("the transfer has no effective basis resolution")
	ErrInvestmentBasisResolutionAlreadyCorrected = errors.New("basis resolution already corrected")
	ErrInvestmentBasisResolutionChanged          = errors.New("basis resolution changed")
)

// CorrectTransferBasisResolutionInput reverses or replaces the effective
// sourced resolution of an external transfer in (#168). TransactionID is the
// transfer's, so a correction always addresses whichever resolution is
// effective. A replacement keeps the pinned transfer, lot, quantity and cost
// currency; only the sourced basis and its evidence change.
type CorrectTransferBasisResolutionInput struct {
	OwnerUserID                   int64
	AuthSessionID                 int64
	RequestID                     string
	TransactionID                 int64
	Reason                        string
	ReconciliationOverride        CheckpointOverride
	GainImpactAcknowledgement     string
	ExpectedResolutionOperationID int64
	ExpectedProvenanceRevision    *int
	PreserveSourceMetadata        bool
	// BasisValue, BasisScale and SourceEvidenceJSON are the replacement's;
	// a reversal ignores them.
	BasisValue         int64
	BasisScale         int
	SourceEvidenceJSON string
}

// ReplaceTransferBasisResolutionResult has no Inverse when the replaced
// resolution was a journal-free zero, and no Replacement when the successor is
// one (#168). Both are nil for a provenance-only correction (#197).
type ReplaceTransferBasisResolutionResult struct {
	Inverse               *Transaction
	Replacement           *Transaction
	TransferTransactionID int64
}

func (s *InvestmentService) basisResolutionCorrectionTarget(ctx context.Context, input CorrectTransferBasisResolutionInput) (db.BasisResolutionOperationRecord, string, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.BasisResolutionOperationRecord{}, "", ValidationError{Message: "owner and basis resolution id are required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil || reason == "" {
		return db.BasisResolutionOperationRecord{}, "", ValidationError{Message: "basis resolution correction reason is required"}
	}
	transfer, err := s.repository.TransferOperationByTransactionID(ctx, BookID, input.TransactionID)
	if err != nil {
		return db.BasisResolutionOperationRecord{}, "", mapTransferCorrectionError(err)
	}
	if transfer.TransferKind != "external_in" || transfer.AlreadyCorrected {
		return db.BasisResolutionOperationRecord{}, "", ErrInvestmentTransferBasisNotResolved
	}
	operation, err := s.repository.EffectiveTransferBasisResolution(ctx, BookID, transfer.OperationID)
	if errors.Is(err, db.ErrNotFound) {
		return db.BasisResolutionOperationRecord{}, "", ErrInvestmentTransferBasisNotResolved
	}
	if err != nil {
		return db.BasisResolutionOperationRecord{}, "", err
	}
	return operation, reason, nil
}

func (s *InvestmentService) prepareBasisResolutionReversal(ctx context.Context, input CorrectTransferBasisResolutionInput) (db.BasisResolutionOperationRecord, db.CreateTransactionParams, error) {
	operation, reason, err := s.basisResolutionCorrectionTarget(ctx, input)
	if err != nil {
		return operation, db.CreateTransactionParams{}, err
	}
	if operation.TransactionID == 0 {
		// A journal-free known zero is reversed without a journal.
		params := s.journalFreeInvestmentHeader(input.OwnerUserID, input.AuthSessionID, input.RequestID,
			"investment.basis_resolution.reverse", reason, "reversal", operation.EventDate, input.ReconciliationOverride)
		params.InvestmentCorrectionOfOperationID = operation.OperationID
		params.InvestmentCorrectionMode = "reverse"
		params.InvestmentCorrectionReason = reason
		return operation, params, nil
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, ErrInvestmentBasisResolutionChanged
	}
	spec := invertedInvestmentTransactionSpec(original)
	spec.InvestmentOperationKind = "reversal"
	spec.Description = reason
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OriginType: "browser_api", Operation: "investment.basis_resolution.reverse", ChangeReason: reason,
		ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed, CorrectionOfTransactionID: &operation.TransactionID,
		Spec: spec,
	}, nil)
	if err != nil {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = reason
	return operation, params, nil
}

// ReverseTransferBasisResolution withdraws a sourced resolution: its bridge is
// inverted and the transfer's basis reads unknown again. Sales it resolved
// become unresolved under the preview's acknowledgement; a known outbound or
// onward link it reached refuses with that operation named, never silently
// becoming unknown (#168).
func (s *InvestmentService) ReverseTransferBasisResolution(ctx context.Context, input CorrectTransferBasisResolutionInput) (*Transaction, error) {
	operation, params, err := s.prepareBasisResolutionReversal(ctx, input)
	if err != nil {
		return nil, err
	}
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	record, err := s.repository.ReverseTransferBasisResolution(ctx, params, operation)
	if err != nil {
		return nil, mapBasisResolutionCorrectionError(err)
	}
	enriched, err := s.transactionService.enrichRecords(ctx, record)
	if err != nil {
		return nil, err
	}
	return optionalTransaction(enriched[0]), nil
}

func (s *InvestmentService) ReverseTransferBasisResolutionImpact(ctx context.Context, input CorrectTransferBasisResolutionInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	operation, params, err := s.prepareBasisResolutionReversal(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	params.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.PreviewTransferBasisResolutionReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapBasisResolutionCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareBasisResolutionReplacement(ctx context.Context, input CorrectTransferBasisResolutionInput) (db.BasisResolutionOperationRecord, db.BasisResolutionReplacement, db.ResolveTransferBasisParams, error) {
	operation, reason, err := s.basisResolutionCorrectionTarget(ctx, input)
	if err != nil {
		return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, err
	}
	if input.ExpectedResolutionOperationID != 0 && (input.ExpectedResolutionOperationID != operation.OperationID ||
		input.ExpectedProvenanceRevision == nil || *input.ExpectedProvenanceRevision != operation.ProvenanceRevision) {
		return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, ErrInvestmentBasisResolutionChanged
	}
	if input.BasisValue < 0 || input.BasisScale < 0 || input.BasisScale > 12 {
		return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "a resolution needs a nonnegative known basis at a money scale"}
	}
	evidence, err := cleanBasisResolutionEvidence(input.SourceEvidenceJSON)
	if err != nil {
		return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, err
	}
	if input.PreserveSourceMetadata {
		evidence, err = mergeBasisResolutionEvidence(operation.SourceEvidenceJSON, evidence)
		if err != nil {
			return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, err
		}
	}
	if exact.ScaledIntFromCoefficient(exact.New(input.BasisValue), input.BasisScale).Cmp(
		exact.ScaledIntFromCoefficient(operation.BasisValue, operation.BasisScale)) == 0 {
		if sameBasisResolutionEvidence(evidence, operation.SourceEvidenceJSON) {
			return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "the replacement repeats the effective resolution"}
		}
		if input.ExpectedResolutionOperationID <= 0 || input.ExpectedProvenanceRevision == nil {
			return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "a provenance correction requires the expected resolution and evidence revision"}
		}
		header := s.journalFreeInvestmentHeader(input.OwnerUserID, input.AuthSessionID, input.RequestID,
			"investment.basis_resolution.provenance", reason, "", operation.EventDate, input.ReconciliationOverride)
		header.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
		return operation, db.BasisResolutionReplacement{Successor: header, ProvenanceOnly: true}, db.ResolveTransferBasisParams{
			BookID: BookID, TransferOperationID: operation.TransferOperationID,
			BasisValue: operation.BasisValue, BasisScale: operation.BasisScale, SourceEvidenceJSON: evidence,
		}, nil
	}
	_, inverse, err := s.prepareBasisResolutionReversal(ctx, input)
	if err != nil {
		return operation, db.BasisResolutionReplacement{}, db.ResolveTransferBasisParams{}, err
	}
	successor, params, err := s.transferBasisResolutionJournal(ctx, ResolveTransferBasisInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		BasisValue: input.BasisValue, BasisScale: input.BasisScale, SourceEvidenceJSON: evidence,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
	}, operation.TransferOperationID, operation.CostCommodityID, operation.EventDate)
	if err != nil {
		return operation, db.BasisResolutionReplacement{}, params, err
	}
	write := db.BasisResolutionReplacement{Successor: successor}
	write.Successor.Operation = "investment.basis_resolution.replace"
	write.Successor.InvestmentCorrectionOfOperationID, write.Successor.InvestmentCorrectionMode = operation.OperationID, "replace"
	write.Successor.InvestmentCorrectionReason = inverse.InvestmentCorrectionReason
	write.Successor.CreatedAt = inverse.CreatedAt
	// The command's first journal carries the gain policy: the inverse of a
	// bridged predecessor, else the successor's bridge.
	write.Successor.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	if operation.TransactionID != 0 {
		inverse.Spec.InvestmentOperationKind = ""
		inverse.InvestmentCorrectionOfOperationID, inverse.InvestmentCorrectionMode = 0, ""
		inverse.InvestmentCorrectionReason = ""
		inverse.Operation = write.Successor.Operation
		inverse.GainImpact, write.Successor.GainImpact = write.Successor.GainImpact, nil
		if !write.Successor.AuditOnly {
			write.Successor.CorrectionOfTransactionID = inverse.CorrectionOfTransactionID
		}
		write.Inverse = &inverse
	}
	return operation, write, params, nil
}

// ReplaceTransferBasisResolution corrects a wrong sourced basis: the old
// bridge is inverted, the successor posts its complete bridge at the transfer
// date, and replay revises every sale, onward link and outbound bridge the
// lot reached. The original resolution stays as superseded evidence (#168).
// At equal exact basis, changed evidence instead appends an audit-only
// provenance revision without changing economic operation lineage (#197).
func (s *InvestmentService) ReplaceTransferBasisResolution(ctx context.Context, input CorrectTransferBasisResolutionInput) (ReplaceTransferBasisResolutionResult, error) {
	operation, write, params, err := s.prepareBasisResolutionReplacement(ctx, input)
	if err != nil {
		return ReplaceTransferBasisResolutionResult{}, err
	}
	record, err := s.repository.ReplaceTransferBasisResolution(ctx, operation, write, params)
	if err != nil {
		return ReplaceTransferBasisResolutionResult{}, mapBasisResolutionCorrectionError(err)
	}
	enriched, err := s.transactionService.enrichRecords(ctx, record.Inverse, record.Replacement)
	if err != nil {
		return ReplaceTransferBasisResolutionResult{}, err
	}
	return ReplaceTransferBasisResolutionResult{Inverse: optionalTransaction(enriched[0]), Replacement: optionalTransaction(enriched[1]),
		TransferTransactionID: operation.TransferTransactionID}, nil
}

func (s *InvestmentService) ReplaceTransferBasisResolutionImpact(ctx context.Context, input CorrectTransferBasisResolutionInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	input.GainImpactAcknowledgement = ""
	operation, write, params, err := s.prepareBasisResolutionReplacement(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulateTransferBasisResolutionReplacement(ctx, operation, write, params)
	if err != nil {
		return ReconciliationImpact{}, mapBasisResolutionCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func mapBasisResolutionCorrectionError(err error) error {
	switch {
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentBasisResolutionAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentBasisResolutionChanged
	}
	return mapTransferCorrectionError(err)
}

// optionalTransaction is a journal a command may not post: nil for the zero
// transaction a journal-free command returns. Posted transactions are enriched
// before this pure optional-value conversion.
func optionalTransaction(transaction Transaction) *Transaction {
	if transaction.ID == 0 {
		return nil
	}
	return &transaction
}

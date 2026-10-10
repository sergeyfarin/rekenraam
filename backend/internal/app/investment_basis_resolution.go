package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	ReconciliationOverride    bool
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
// journal for a transfer's pinned cost currency and date.
func (s *InvestmentService) transferBasisResolutionJournal(ctx context.Context, input ResolveTransferBasisInput,
	transferOperationID, costCommodityID int64, effectiveOn string,
) (db.CreateTransactionParams, db.ResolveTransferBasisParams, error) {
	if input.BasisValue < 0 || input.BasisScale < 0 || input.BasisScale > 12 {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolved basis must be nonnegative at a valid money scale"}
	}
	if input.BasisValue == 0 {
		// A resolution posts its bridge; a known zero posts none, so it is
		// recorded by correcting the transfer with an explicit zero basis.
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "a zero basis is recorded by correcting the transfer, not by resolution"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil || reason == "" {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "resolution reason is required"}
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
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
		ReconciliationOverride: input.ReconciliationOverride,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "basis_resolution",
			TransactionDate: effectiveOn, Description: reason, MetadataJSON: evidence,
			JournalEntries: []JournalEntryInput{{EntryDate: effectiveOn, EntryKind: "investment", Memo: reason, Postings: postings}},
		},
	}, nil)
	if err != nil {
		return db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	return journal, db.ResolveTransferBasisParams{BookID: BookID, TransferOperationID: transferOperationID,
		BasisValue: exact.New(input.BasisValue), BasisScale: input.BasisScale, SourceEvidenceJSON: evidence}, nil
}

// ResolveTransferBasisImpact runs the complete resolution, replay and gain
// comparison, then rolls back, so a preview reports what the commit would do.
func (s *InvestmentService) ResolveTransferBasisImpact(ctx context.Context, input ResolveTransferBasisInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
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
func (s *InvestmentService) ResolveTransferBasis(ctx context.Context, input ResolveTransferBasisInput) (Transaction, error) {
	journal, params, err := s.resolveTransferBasisWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, err := s.repository.ResolveTransferBasis(ctx, journal, params)
	if err != nil {
		return Transaction{}, mapTransferCorrectionError(err)
	}
	return toTransaction(transaction), nil
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
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
	// BasisValue, BasisScale and SourceEvidenceJSON are the replacement's;
	// a reversal ignores them.
	BasisValue         int64
	BasisScale         int
	SourceEvidenceJSON string
}

type ReplaceTransferBasisResolutionResult struct {
	Inverse                Transaction
	Replacement            Transaction
	CorrectedTransactionID int64
	TransferTransactionID  int64
}

func (s *InvestmentService) prepareBasisResolutionReversal(ctx context.Context, input CorrectTransferBasisResolutionInput) (db.BasisResolutionOperationRecord, db.CreateTransactionParams, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, ValidationError{Message: "owner and basis resolution id are required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil || reason == "" {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, ValidationError{Message: "basis resolution correction reason is required"}
	}
	transfer, err := s.repository.TransferOperationByTransactionID(ctx, BookID, input.TransactionID)
	if err != nil {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, mapTransferCorrectionError(err)
	}
	if transfer.TransferKind != "external_in" || transfer.AlreadyCorrected {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, ErrInvestmentTransferBasisNotResolved
	}
	operation, err := s.repository.EffectiveTransferBasisResolution(ctx, BookID, transfer.OperationID)
	if errors.Is(err, db.ErrNotFound) {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, ErrInvestmentTransferBasisNotResolved
	}
	if err != nil {
		return db.BasisResolutionOperationRecord{}, db.CreateTransactionParams{}, err
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
		ReconciliationOverride: input.ReconciliationOverride, CorrectionOfTransactionID: &operation.TransactionID,
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
func (s *InvestmentService) ReverseTransferBasisResolution(ctx context.Context, input CorrectTransferBasisResolutionInput) (Transaction, error) {
	operation, params, err := s.prepareBasisResolutionReversal(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	record, err := s.repository.ReverseTransferBasisResolution(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapBasisResolutionCorrectionError(err)
	}
	return toTransaction(record), nil
}

func (s *InvestmentService) ReverseTransferBasisResolutionImpact(ctx context.Context, input CorrectTransferBasisResolutionInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
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

func (s *InvestmentService) prepareBasisResolutionReplacement(ctx context.Context, input CorrectTransferBasisResolutionInput) (db.BasisResolutionOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.ResolveTransferBasisParams, error) {
	operation, inverse, err := s.prepareBasisResolutionReversal(ctx, input)
	if err != nil {
		return operation, inverse, db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, err
	}
	if input.BasisScale >= 0 && exact.ScaledIntFromCoefficient(exact.New(input.BasisValue), input.BasisScale).Cmp(
		exact.ScaledIntFromCoefficient(operation.BasisValue, operation.BasisScale)) == 0 {
		return operation, inverse, db.CreateTransactionParams{}, db.ResolveTransferBasisParams{}, ValidationError{Message: "the replacement repeats the effective resolution"}
	}
	journal, params, err := s.transferBasisResolutionJournal(ctx, ResolveTransferBasisInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		BasisValue: input.BasisValue, BasisScale: input.BasisScale, SourceEvidenceJSON: input.SourceEvidenceJSON,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
	}, operation.TransferOperationID, operation.CostCommodityID, operation.EventDate)
	if err != nil {
		return operation, inverse, journal, params, err
	}
	inverse.Spec.InvestmentOperationKind = ""
	inverse.InvestmentCorrectionOfOperationID, inverse.InvestmentCorrectionMode = 0, ""
	inverse.InvestmentCorrectionReason = ""
	inverse.Operation = "investment.basis_resolution.replace"
	journal.Operation = inverse.Operation
	journal.CorrectionOfTransactionID = inverse.CorrectionOfTransactionID
	journal.InvestmentCorrectionOfOperationID, journal.InvestmentCorrectionMode = operation.OperationID, "replace"
	journal.InvestmentCorrectionReason = inverse.ChangeReason
	journal.CreatedAt = inverse.CreatedAt
	inverse.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	journal.GainImpact = nil
	return operation, inverse, journal, params, nil
}

// ReplaceTransferBasisResolution corrects a wrong sourced basis: the old
// bridge is inverted, the successor posts its complete bridge at the transfer
// date, and replay revises every sale, onward link and outbound bridge the
// lot reached. The original resolution stays as superseded evidence (#168).
func (s *InvestmentService) ReplaceTransferBasisResolution(ctx context.Context, input CorrectTransferBasisResolutionInput) (ReplaceTransferBasisResolutionResult, error) {
	operation, inverse, replacement, params, err := s.prepareBasisResolutionReplacement(ctx, input)
	if err != nil {
		return ReplaceTransferBasisResolutionResult{}, err
	}
	record, err := s.repository.ReplaceTransferBasisResolution(ctx, operation, inverse, replacement, params)
	if err != nil {
		return ReplaceTransferBasisResolutionResult{}, mapBasisResolutionCorrectionError(err)
	}
	return ReplaceTransferBasisResolutionResult{Inverse: toTransaction(record.Inverse), Replacement: toTransaction(record.Replacement),
		CorrectedTransactionID: operation.TransactionID, TransferTransactionID: operation.TransferTransactionID}, nil
}

func (s *InvestmentService) ReplaceTransferBasisResolutionImpact(ctx context.Context, input CorrectTransferBasisResolutionInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, inverse, replacement, params, err := s.prepareBasisResolutionReplacement(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	inverse.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateTransferBasisResolutionReplacement(ctx, operation, inverse, replacement, params)
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

package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"rekenraam/backend/internal/db"
)

// Share exchange correction (#179) follows the transfer correction pattern
// (ADR 0013 *Transfer Correction Refinement*): a reversal posts the exact
// inverse journal and replays the old holding, then the new one; a
// replacement also records corrected terms at the replaced exchange's slot.
// Originals stay as evidence; a destination sale or onward transfer of
// removed units refuses by name, and changed gains need the shared
// acknowledgement.

var (
	ErrShareExchangeNotFound         = errors.New("share exchange operation not found")
	ErrShareExchangeAlreadyCorrected = errors.New("share exchange already corrected")
)

// ReverseShareExchangeInput reverses a posted share exchange.
type ReverseShareExchangeInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// ReplaceShareExchangeInput corrects a posted exchange's date, ratio, new
// instrument, receiving holding or source evidence. The exchanged holding and
// its old instrument stay those of the exchange.
type ReplaceShareExchangeInput struct {
	OwnerUserID                 int64
	AuthSessionID               int64
	RequestID                   string
	TransactionID               int64
	Reason                      string
	EffectiveOn                 string
	DestinationHoldingAccountID int64
	DestinationCommodityID      int64
	RatioNumerator              int64
	RatioDenominator            int64
	SourceEvidenceJSON          string
	Memo                        string
	ReconciliationOverride      bool
	GainImpactAcknowledgement   string
}

type ReplaceShareExchangeResult struct {
	Inverse                Transaction
	Replacement            Transaction
	Plan                   ShareExchangePlan
	CorrectedTransactionID int64
}

// ReverseShareExchange posts the inverse journal and replays both holdings
// without the exchange.
func (s *InvestmentService) ReverseShareExchange(ctx context.Context, input ReverseShareExchangeInput) (Transaction, error) {
	operation, params, err := s.prepareShareExchangeReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseShareExchange(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapShareExchangeCorrectionError(err)
	}
	return toTransaction(record), nil
}

// ReverseShareExchangeReconciliationImpact runs the reversal writer and rolls
// back: the checkpoints and gains it would change.
func (s *InvestmentService) ReverseShareExchangeReconciliationImpact(ctx context.Context, input ReverseShareExchangeInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareShareExchangeReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewShareExchangeReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapShareExchangeCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareShareExchangeReversalWrite(ctx context.Context, input ReverseShareExchangeInput) (db.ShareExchangeOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.shareExchangeInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, "investment.share_exchange.reverse")
	if err != nil {
		return db.ShareExchangeOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.ShareExchangeOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

// shareExchangeInversePlan pins the effective exchange and plans its exact
// inverse journal. The writer rechecks the same facts in its transaction.
func (s *InvestmentService) shareExchangeInversePlan(ctx context.Context, ownerUserID, authSessionID int64, requestID string,
	transactionID int64, rawReason string, override bool, auditOperation string) (db.ShareExchangeOperationRecord, CreateTransactionInput, error) {
	fail := func(err error) (db.ShareExchangeOperationRecord, CreateTransactionInput, error) {
		return db.ShareExchangeOperationRecord{}, CreateTransactionInput{}, err
	}
	if ownerUserID <= 0 || transactionID <= 0 {
		return fail(ValidationError{Message: "owner and exchange id are required"})
	}
	if strings.TrimSpace(rawReason) == "" {
		return fail(ValidationError{Message: "exchange correction reason is required"})
	}
	reason, err := cleanChangeReason(rawReason, "")
	if err != nil {
		return fail(err)
	}
	operation, err := s.repository.ShareExchangeOperationByTransactionID(ctx, BookID, transactionID)
	if err != nil {
		return fail(mapShareExchangeCorrectionError(err))
	}
	if operation.AlreadyCorrected {
		return fail(ErrShareExchangeAlreadyCorrected)
	}
	if operation.ImportedLineage {
		linked, err := s.repository.HasCommittedImportSource(ctx, BookID, operation.OperationID)
		if err != nil {
			return fail(err)
		}
		if !linked {
			return fail(ErrInvestmentImportedTransfer)
		}
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return fail(err)
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" ||
		original.TransactionKind != "investment" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate {
		return fail(ErrShareExchangeChanged)
	}
	return operation, CreateTransactionInput{
		OwnerUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID, OriginType: "browser_api",
		Operation: auditOperation, CorrectionOfTransactionID: &operation.TransactionID,
		Spec: invertedInvestmentTransactionSpec(original), ChangeReason: reason, ReconciliationOverride: override,
	}, nil
}

// ReplaceShareExchange reverses the exchange and records corrected terms as
// its successor at the same correction-root slot, under one audit event.
func (s *InvestmentService) ReplaceShareExchange(ctx context.Context, input ReplaceShareExchangeInput) (ReplaceShareExchangeResult, error) {
	operation, inverse, replacement, exchange, err := s.prepareShareExchangeReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceShareExchangeResult{}, err
	}
	record, err := s.repository.ReplaceShareExchange(ctx, operation, inverse, replacement, exchange)
	if err != nil {
		return ReplaceShareExchangeResult{}, mapShareExchangeCorrectionError(err)
	}
	plan, err := toShareExchangePlan(exchange, record.Plan)
	if err != nil {
		return ReplaceShareExchangeResult{}, err
	}
	return ReplaceShareExchangeResult{Inverse: toTransaction(record.Inverse), Replacement: toTransaction(record.Replacement),
		Plan: plan, CorrectedTransactionID: operation.TransactionID}, nil
}

// PreviewShareExchangeReplacement runs the replacement writer and every
// replay, then rolls back: the plan the replacement would carry and its impact.
func (s *InvestmentService) PreviewShareExchangeReplacement(ctx context.Context, input ReplaceShareExchangeInput) (ShareExchangePreview, error) {
	input.ReconciliationOverride = true
	operation, inverse, replacement, exchange, err := s.prepareShareExchangeReplacementWrite(ctx, input)
	if err != nil {
		return ShareExchangePreview{}, err
	}
	inverse.GainImpact = gainImpactPolicy("")
	simulated, planned, err := s.repository.SimulateShareExchangeReplacement(ctx, operation, inverse, replacement, exchange)
	if err != nil {
		return ShareExchangePreview{}, mapShareExchangeCorrectionError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return ShareExchangePreview{}, err
	}
	plan, err := toShareExchangePlan(exchange, planned)
	if err != nil {
		return ShareExchangePreview{}, err
	}
	return ShareExchangePreview{Plan: plan, Impact: impact}, nil
}

func (s *InvestmentService) prepareShareExchangeReplacementWrite(ctx context.Context, input ReplaceShareExchangeInput) (
	db.ShareExchangeOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateShareExchangeParams, error) {
	fail := func(err error) (db.ShareExchangeOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateShareExchangeParams, error) {
		return db.ShareExchangeOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, db.CreateShareExchangeParams{}, err
	}
	const auditOperation = "investment.share_exchange.replace"
	operation, plannedInverse, err := s.shareExchangeInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, auditOperation)
	if err != nil {
		return fail(err)
	}
	inverse, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plannedInverse, nil)
	if err != nil {
		return fail(err)
	}
	// The compound command's first journal carries the disclosure policy.
	inverse.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	replacement, exchange, err := s.prepareShareExchangeWriteWith(ctx, ShareExchangeInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		EffectiveOn: input.EffectiveOn, HoldingAccountID: operation.AccountID,
		DestinationHoldingAccountID: input.DestinationHoldingAccountID,
		CommodityID:                 operation.CommodityID, DestinationCommodityID: input.DestinationCommodityID,
		RatioNumerator: input.RatioNumerator, RatioDenominator: input.RatioDenominator,
		SourceEvidenceJSON: input.SourceEvidenceJSON, Memo: input.Memo, ChangeReason: plannedInverse.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
	}, operation.OperationID, auditOperation)
	if err != nil {
		return fail(err)
	}
	if exchange.EffectiveOn == operation.EventDate && exchange.RatioNumerator == operation.RatioNumerator &&
		exchange.RatioDenominator == operation.RatioDenominator &&
		exchange.DestinationCommodityID == operation.DestinationCommodityID &&
		exchange.DestinationAccountID == operation.DestinationAccountID && strings.TrimSpace(input.SourceEvidenceJSON) == "" {
		return fail(ValidationError{Message: "an exchange replacement must change the date, ratio, new instrument, receiving holding or source evidence"})
	}
	replacement.CorrectionOfTransactionID = sql.NullInt64{Int64: operation.TransactionID, Valid: true}
	replacement.InvestmentCorrectionOfOperationID = operation.OperationID
	replacement.InvestmentCorrectionMode = "replace"
	replacement.InvestmentCorrectionReason = plannedInverse.ChangeReason
	replacement.CreatedAt = inverse.CreatedAt
	replacement.GainImpact = nil
	return operation, inverse, replacement, exchange, nil
}

// mapShareExchangeCorrectionError adds the correction fences to the exchange
// writer's own refusals. A later operation the correction breaks is named.
func mapShareExchangeCorrectionError(err error) error {
	// A dependency wraps the cause a later operation met (often a missing
	// lot), so it is named before any fence is matched.
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) || errors.Is(err, db.ErrInvestmentCorrectionDependency) {
		return mapShareExchangeError(err)
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrShareExchangeNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrShareExchangeAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedTransfer
	case errors.Is(err, db.ErrShareExchangeOperationChanged), errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrShareExchangeChanged
	}
	return mapShareExchangeError(err)
}

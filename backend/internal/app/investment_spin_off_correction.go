package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Spin-off correction (#183) follows the share exchange correction (#179) and
// ADR 0013 *Transfer Correction Refinement*: a reversal posts the exact
// inverse journal and replays the parent, then the new holding; a
// replacement also records corrected terms at the replaced spin-off's slot.
// Originals stay as evidence; a disposal or onward transfer of removed new
// units refuses by name, and changed gains need the shared acknowledgement.

var (
	ErrSpinOffNotFound         = errors.New("spin-off operation not found")
	ErrSpinOffAlreadyCorrected = errors.New("spin-off already corrected")
)

// ReverseSpinOffInput reverses a posted spin-off.
type ReverseSpinOffInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// ReplaceSpinOffInput corrects a posted spin-off's date, ratio, basis
// fraction, new instrument, receiving holding or source evidence. The parent
// holding and instrument stay those of the spin-off.
type ReplaceSpinOffInput struct {
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
	BasisFractionValue          exact.Coefficient
	BasisFractionScale          int
	SourceEvidenceJSON          string
	Memo                        string
	ReconciliationOverride      bool
	GainImpactAcknowledgement   string
}

type ReplaceSpinOffResult struct {
	Inverse                Transaction
	Replacement            Transaction
	Plan                   SpinOffPlan
	CorrectedTransactionID int64
}

// ReverseSpinOff posts the inverse journal and replays both holdings without
// the spin-off.
func (s *InvestmentService) ReverseSpinOff(ctx context.Context, input ReverseSpinOffInput) (Transaction, error) {
	operation, params, err := s.prepareSpinOffReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseSpinOff(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapSpinOffCorrectionError(err)
	}
	return toTransaction(record), nil
}

// ReverseSpinOffReconciliationImpact runs the reversal writer and rolls back:
// the checkpoints and gains it would change.
func (s *InvestmentService) ReverseSpinOffReconciliationImpact(ctx context.Context, input ReverseSpinOffInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareSpinOffReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewSpinOffReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapSpinOffCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareSpinOffReversalWrite(ctx context.Context, input ReverseSpinOffInput) (db.SpinOffOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.spinOffInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, "investment.spin_off.reverse")
	if err != nil {
		return db.SpinOffOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.SpinOffOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

// spinOffInversePlan pins the effective spin-off and plans its exact inverse
// journal. The writer rechecks the same facts in its transaction.
func (s *InvestmentService) spinOffInversePlan(ctx context.Context, ownerUserID, authSessionID int64, requestID string,
	transactionID int64, rawReason string, override bool, auditOperation string) (db.SpinOffOperationRecord, CreateTransactionInput, error) {
	fail := func(err error) (db.SpinOffOperationRecord, CreateTransactionInput, error) {
		return db.SpinOffOperationRecord{}, CreateTransactionInput{}, err
	}
	if ownerUserID <= 0 || transactionID <= 0 {
		return fail(ValidationError{Message: "owner and spin-off id are required"})
	}
	if strings.TrimSpace(rawReason) == "" {
		return fail(ValidationError{Message: "spin-off correction reason is required"})
	}
	reason, err := cleanChangeReason(rawReason, "")
	if err != nil {
		return fail(err)
	}
	operation, err := s.repository.SpinOffOperationByTransactionID(ctx, BookID, transactionID)
	if err != nil {
		return fail(mapSpinOffCorrectionError(err))
	}
	if operation.AlreadyCorrected {
		return fail(ErrSpinOffAlreadyCorrected)
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
		return fail(ErrSpinOffChanged)
	}
	return operation, CreateTransactionInput{
		OwnerUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID, OriginType: "browser_api",
		Operation: auditOperation, CorrectionOfTransactionID: &operation.TransactionID,
		Spec: invertedInvestmentTransactionSpec(original), ChangeReason: reason, ReconciliationOverride: override,
	}, nil
}

// ReplaceSpinOff reverses the spin-off and records corrected terms as its
// successor at the same correction-root slot, under one audit event.
func (s *InvestmentService) ReplaceSpinOff(ctx context.Context, input ReplaceSpinOffInput) (ReplaceSpinOffResult, error) {
	operation, inverse, replacement, spinOff, err := s.prepareSpinOffReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceSpinOffResult{}, err
	}
	record, err := s.repository.ReplaceSpinOff(ctx, operation, inverse, replacement, spinOff)
	if err != nil {
		return ReplaceSpinOffResult{}, mapSpinOffCorrectionError(err)
	}
	plan, err := spinOffPlanOf(spinOff.RatioNumerator, spinOff.RatioDenominator, spinOff.BasisFractionValue,
		spinOff.BasisFractionScale, record.Plan, true)
	if err != nil {
		return ReplaceSpinOffResult{}, err
	}
	return ReplaceSpinOffResult{Inverse: toTransaction(record.Inverse), Replacement: toTransaction(record.Replacement),
		Plan: plan, CorrectedTransactionID: operation.TransactionID}, nil
}

// PreviewSpinOffReplacement runs the replacement writer and every replay,
// then rolls back: the plan the replacement would carry and its impact.
func (s *InvestmentService) PreviewSpinOffReplacement(ctx context.Context, input ReplaceSpinOffInput) (SpinOffPreview, error) {
	input.ReconciliationOverride = true
	operation, inverse, replacement, spinOff, err := s.prepareSpinOffReplacementWrite(ctx, input)
	if err != nil {
		return SpinOffPreview{}, err
	}
	inverse.GainImpact = gainImpactPolicy("")
	simulated, planned, err := s.repository.SimulateSpinOffReplacement(ctx, operation, inverse, replacement, spinOff)
	if err != nil {
		return SpinOffPreview{}, mapSpinOffCorrectionError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return SpinOffPreview{}, err
	}
	plan, err := spinOffPlanOf(spinOff.RatioNumerator, spinOff.RatioDenominator, spinOff.BasisFractionValue,
		spinOff.BasisFractionScale, planned, true)
	if err != nil {
		return SpinOffPreview{}, err
	}
	return SpinOffPreview{Plan: plan, Impact: impact}, nil
}

func (s *InvestmentService) prepareSpinOffReplacementWrite(ctx context.Context, input ReplaceSpinOffInput) (
	db.SpinOffOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateSpinOffParams, error) {
	fail := func(err error) (db.SpinOffOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateSpinOffParams, error) {
		return db.SpinOffOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, db.CreateSpinOffParams{}, err
	}
	const auditOperation = "investment.spin_off.replace"
	operation, plannedInverse, err := s.spinOffInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
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
	// Omitted evidence keeps the recorded evidence.
	evidence := input.SourceEvidenceJSON
	if strings.TrimSpace(evidence) == "" {
		evidence = operation.SourceEvidenceJSON
	}
	replacement, spinOff, err := s.prepareSpinOffWriteWith(ctx, SpinOffInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		EffectiveOn: input.EffectiveOn, HoldingAccountID: operation.AccountID,
		DestinationHoldingAccountID: input.DestinationHoldingAccountID,
		CommodityID:                 operation.CommodityID, DestinationCommodityID: input.DestinationCommodityID,
		RatioNumerator: input.RatioNumerator, RatioDenominator: input.RatioDenominator,
		BasisFractionValue: input.BasisFractionValue, BasisFractionScale: input.BasisFractionScale,
		SourceEvidenceJSON: evidence, Memo: input.Memo, ChangeReason: plannedInverse.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
	}, operation.OperationID, auditOperation)
	if err != nil {
		return fail(err)
	}
	if spinOff.EffectiveOn == operation.EventDate && spinOff.RatioNumerator == operation.RatioNumerator &&
		spinOff.RatioDenominator == operation.RatioDenominator &&
		spinOff.BasisFractionValue == operation.BasisFractionValue && spinOff.BasisFractionScale == operation.BasisFractionScale &&
		spinOff.DestinationCommodityID == operation.DestinationCommodityID &&
		spinOff.DestinationAccountID == operation.DestinationAccountID &&
		sameSourceEvidence(spinOff.SourceEvidenceJSON, operation.SourceEvidenceJSON) {
		return fail(ValidationError{Message: "a spin-off replacement must change the date, ratio, basis fraction, new instrument, receiving holding or source evidence"})
	}
	replacement.CorrectionOfTransactionID = sql.NullInt64{Int64: operation.TransactionID, Valid: true}
	replacement.InvestmentCorrectionOfOperationID = operation.OperationID
	replacement.InvestmentCorrectionMode = "replace"
	replacement.InvestmentCorrectionReason = plannedInverse.ChangeReason
	replacement.CreatedAt = inverse.CreatedAt
	replacement.GainImpact = nil
	return operation, inverse, replacement, spinOff, nil
}

// sameSourceEvidence reports whether corrected evidence is the recorded
// evidence as a JSON object (the correction form pre-fills it).
func sameSourceEvidence(corrected, recorded string) bool {
	var left, right map[string]any
	if json.Unmarshal([]byte(corrected), &left) != nil || json.Unmarshal([]byte(recorded), &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

// mapSpinOffCorrectionError adds the correction fences to the spin-off
// writer's own refusals. A later operation the correction breaks is named.
func mapSpinOffCorrectionError(err error) error {
	// A dependency wraps the cause a later operation met (often a missing
	// lot), so it is named before any fence is matched.
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) || errors.Is(err, db.ErrInvestmentCorrectionDependency) {
		return mapSpinOffError(err)
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrSpinOffNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrSpinOffAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedTransfer
	case errors.Is(err, db.ErrSpinOffOperationChanged), errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrSpinOffChanged
	}
	return mapSpinOffError(err)
}

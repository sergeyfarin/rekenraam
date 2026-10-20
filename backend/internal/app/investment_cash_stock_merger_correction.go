package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// Cash-and-stock merger correction (#184) follows the share exchange (#179)
// and spin-off (#183) corrections and ADR 0013 *Transfer Correction
// Refinement*: a reversal posts the exact inverse of the whole merger journal
// (security legs and cash leg) and replays the old holding, then the new one;
// a replacement also records corrected terms at the replaced merger's slot.
// Originals stay as evidence; a disposal or onward transfer of removed new
// units, or a cash in lieu settling the merger, refuses by name, and changed
// gains (the merger's own cash result included) need the shared
// acknowledgement.

var (
	ErrCashStockMergerNotFound         = errors.New("cash-and-stock merger operation not found")
	ErrCashStockMergerAlreadyCorrected = errors.New("cash-and-stock merger already corrected")
)

// ReverseCashStockMergerInput reverses a posted merger.
type ReverseCashStockMergerInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    CheckpointOverride
	GainImpactAcknowledgement string
}

// ReplaceCashStockMergerInput corrects a posted merger's dates, ratio,
// election, cash, new instrument, receiving holding or source evidence. The
// merged holding and its old instrument stay those of the merger.
type ReplaceCashStockMergerInput struct {
	OwnerUserID                 int64
	AuthSessionID               int64
	RequestID                   string
	TransactionID               int64
	Reason                      string
	EffectiveOn                 string
	PayableOn                   string
	DestinationHoldingAccountID int64
	DestinationCommodityID      int64
	RatioNumerator              int64
	RatioDenominator            int64
	CashAccountID               int64
	CurrencyID                  int64
	CashValue                   int64
	CashScale                   int
	CashBasisFractionValue      exact.Coefficient
	CashBasisFractionScale      int
	SourceEvidenceJSON          string
	SourceEvidenceEditsJSON     string
	Memo                        string
	ReconciliationOverride      CheckpointOverride
	GainImpactAcknowledgement   string
}

type ReplaceCashStockMergerResult struct {
	Inverse                Transaction
	Replacement            Transaction
	Plan                   CashStockMergerPlan
	CorrectedTransactionID int64
}

// ReverseCashStockMerger posts the inverse journal and replays both holdings
// without the merger or its cash decision.
func (s *InvestmentService) ReverseCashStockMerger(ctx context.Context, input ReverseCashStockMergerInput) (Transaction, error) {
	operation, params, err := s.prepareCashStockMergerReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseCashStockMerger(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapCashStockMergerCorrectionError(err)
	}
	return s.transactionService.enrichOne(ctx, toTransaction(record))
}

// ReverseCashStockMergerReconciliationImpact runs the reversal writer and
// rolls back: the checkpoints and gains it would change.
func (s *InvestmentService) ReverseCashStockMergerReconciliationImpact(ctx context.Context, input ReverseCashStockMergerInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	operation, params, err := s.prepareCashStockMergerReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewCashStockMergerReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapCashStockMergerCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareCashStockMergerReversalWrite(ctx context.Context, input ReverseCashStockMergerInput) (db.CashStockMergerOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.cashStockMergerInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.Reason, input.ReconciliationOverride, "investment.cash_and_stock_merger.reverse")
	if err != nil {
		return db.CashStockMergerOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.CashStockMergerOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

// cashStockMergerInversePlan pins the effective merger and plans its exact
// inverse journal: both entries, each on its own date. The writer rechecks
// the same facts in its transaction.
func (s *InvestmentService) cashStockMergerInversePlan(ctx context.Context, ownerUserID, authSessionID int64, requestID string,
	transactionID int64, rawReason string, override CheckpointOverride, auditOperation string) (db.CashStockMergerOperationRecord, CreateTransactionInput, error) {
	fail := func(err error) (db.CashStockMergerOperationRecord, CreateTransactionInput, error) {
		return db.CashStockMergerOperationRecord{}, CreateTransactionInput{}, err
	}
	if ownerUserID <= 0 || transactionID <= 0 {
		return fail(ValidationError{Message: "owner and merger id are required"})
	}
	if strings.TrimSpace(rawReason) == "" {
		return fail(ValidationError{Message: "merger correction reason is required"})
	}
	reason, err := cleanChangeReason(rawReason, "")
	if err != nil {
		return fail(err)
	}
	operation, err := s.repository.CashStockMergerOperationByTransactionID(ctx, BookID, transactionID)
	if err != nil {
		return fail(mapCashStockMergerCorrectionError(err))
	}
	if operation.AlreadyCorrected {
		return fail(ErrCashStockMergerAlreadyCorrected)
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
		Spec: invertedInvestmentTransactionSpec(original), ChangeReason: reason, ReconciliationOverride: override.Allowed, ReconciliationOverrideCheckpointIDs: override.Reviewed,
	}, nil
}

// ReplaceCashStockMerger reverses the merger and records corrected terms as
// its successor at the same correction-root slot, under one audit event.
func (s *InvestmentService) ReplaceCashStockMerger(ctx context.Context, input ReplaceCashStockMergerInput) (ReplaceCashStockMergerResult, error) {
	operation, inverse, replacement, merger, err := s.prepareCashStockMergerReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceCashStockMergerResult{}, err
	}
	record, err := s.repository.ReplaceCashStockMerger(ctx, operation, inverse, replacement, merger)
	if err != nil {
		return ReplaceCashStockMergerResult{}, mapCashStockMergerCorrectionError(err)
	}
	plan, err := toCashStockMergerPlan(merger, record.Plan)
	if err != nil {
		return ReplaceCashStockMergerResult{}, err
	}
	enriched, err := s.transactionService.enrichRecords(ctx, record.Inverse, record.Replacement)
	if err != nil {
		return ReplaceCashStockMergerResult{}, err
	}
	return ReplaceCashStockMergerResult{Inverse: enriched[0], Replacement: enriched[1],
		Plan: plan, CorrectedTransactionID: operation.TransactionID}, nil
}

// PreviewCashStockMergerReplacement runs the replacement writer and every
// replay, then rolls back: the plan the replacement would carry and its
// impact.
func (s *InvestmentService) PreviewCashStockMergerReplacement(ctx context.Context, input ReplaceCashStockMergerInput) (CashStockMergerPreview, error) {
	input.ReconciliationOverride = unboundOverride
	operation, inverse, replacement, merger, err := s.prepareCashStockMergerReplacementWrite(ctx, input)
	if err != nil {
		return CashStockMergerPreview{}, err
	}
	inverse.GainImpact = gainImpactPolicy("")
	simulated, planned, err := s.repository.SimulateCashStockMergerReplacement(ctx, operation, inverse, replacement, merger)
	if err != nil {
		return CashStockMergerPreview{}, mapCashStockMergerCorrectionError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return CashStockMergerPreview{}, err
	}
	plan, err := toCashStockMergerPlan(merger, planned)
	if err != nil {
		return CashStockMergerPreview{}, err
	}
	return CashStockMergerPreview{Plan: plan, Impact: impact}, nil
}

func (s *InvestmentService) prepareCashStockMergerReplacementWrite(ctx context.Context, input ReplaceCashStockMergerInput) (
	db.CashStockMergerOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateCashStockMergerParams, error) {
	fail := func(err error) (db.CashStockMergerOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateCashStockMergerParams, error) {
		return db.CashStockMergerOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, db.CreateCashStockMergerParams{}, err
	}
	const auditOperation = "investment.cash_and_stock_merger.replace"
	operation, plannedInverse, err := s.cashStockMergerInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
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
	// Omitted evidence keeps the recorded evidence; edits apply to its exact
	// recorded tokens (#311).
	evidence, err := correctedSourceEvidence(operation.SourceEvidenceJSON, input.SourceEvidenceJSON, input.SourceEvidenceEditsJSON)
	if err != nil {
		return fail(err)
	}
	replacement, merger, err := s.prepareCashStockMergerWriteWith(ctx, CashStockMergerInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		EffectiveOn: input.EffectiveOn, PayableOn: input.PayableOn, HoldingAccountID: operation.AccountID,
		DestinationHoldingAccountID: input.DestinationHoldingAccountID,
		CommodityID:                 operation.CommodityID, DestinationCommodityID: input.DestinationCommodityID,
		RatioNumerator: input.RatioNumerator, RatioDenominator: input.RatioDenominator,
		CashAccountID: input.CashAccountID, CurrencyID: input.CurrencyID, CashValue: input.CashValue, CashScale: input.CashScale,
		CashBasisFractionValue: input.CashBasisFractionValue, CashBasisFractionScale: input.CashBasisFractionScale,
		SourceEvidenceJSON: evidence, Memo: input.Memo, ChangeReason: plannedInverse.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
	}, operation.OperationID, auditOperation)
	if err != nil {
		return fail(err)
	}
	exchange := merger.Exchange
	sameCash := exact.ScaledIntFromInt64(merger.CashValue, merger.CashScale).Cmp(
		exact.ScaledIntFromCoefficient(operation.CashValue, operation.CashScale)) == 0
	if exchange.EffectiveOn == operation.EventDate && merger.PayableOn == operation.PayableOn &&
		exchange.RatioNumerator == operation.RatioNumerator && exchange.RatioDenominator == operation.RatioDenominator &&
		merger.CashBasisFractionValue == operation.CashBasisFractionValue &&
		merger.CashBasisFractionScale == operation.CashBasisFractionScale &&
		exchange.DestinationCommodityID == operation.DestinationCommodityID &&
		exchange.DestinationAccountID == operation.DestinationAccountID &&
		input.CashAccountID == operation.CashAccountID && merger.CashCommodityID == operation.CashCommodityID && sameCash &&
		sameSourceEvidence(exchange.SourceEvidenceJSON, operation.SourceEvidenceJSON) {
		return fail(ValidationError{Message: "a merger replacement must change the dates, ratio, election, cash, new instrument, receiving holding or source evidence"})
	}
	replacement.CorrectionOfTransactionID = sql.NullInt64{Int64: operation.TransactionID, Valid: true}
	replacement.InvestmentCorrectionOfOperationID = operation.OperationID
	replacement.InvestmentCorrectionMode = "replace"
	replacement.InvestmentCorrectionReason = plannedInverse.ChangeReason
	replacement.CreatedAt = inverse.CreatedAt
	replacement.GainImpact = nil
	return operation, inverse, replacement, merger, nil
}

// mapCashStockMergerCorrectionError adds the correction fences to the merger
// writer's own refusals. A later operation the correction breaks is named.
func mapCashStockMergerCorrectionError(err error) error {
	// A dependency wraps the cause a later operation met (often a missing
	// lot), so it is named before any fence is matched.
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) || errors.Is(err, db.ErrInvestmentCorrectionDependency) {
		return mapCashStockMergerError(err)
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrCashStockMergerNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrCashStockMergerAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedTransfer
	case errors.Is(err, db.ErrCashStockMergerOperationChanged), errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrShareExchangeChanged
	}
	return mapCashStockMergerError(err)
}

package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

var (
	ErrInvestmentSplitNotFound         = errors.New("investment split operation not found")
	ErrInvestmentSplitAlreadyCorrected = errors.New("investment split already corrected")
	ErrInvestmentImportedSplit         = errors.New("split linked to an import row no longer names its committed source")
)

// ReverseInvestmentSplitInput reverses a posted split (T-129).
type ReverseInvestmentSplitInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	OperationID               int64
	Reason                    string
	ReconciliationOverride    CheckpointOverride
	GainImpactAcknowledgement string
}

// ReplaceInvestmentSplitInput corrects a posted split's date, ratio or source
// evidence. The holding account and security stay those of the split.
type ReplaceInvestmentSplitInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	OperationID               int64
	Reason                    string
	EffectiveOn               string
	RatioNumerator            int64
	RatioDenominator          int64
	SourceEvidenceJSON        string
	SourceEvidenceEditsJSON   string
	Memo                      string
	ReconciliationOverride    CheckpointOverride
	GainImpactAcknowledgement string
}

type ReplaceInvestmentSplitResult struct {
	Inverse                Transaction
	Replacement            Transaction
	Plan                   InvestmentSplitPlan
	CorrectedTransactionID int64
	CorrectedOperationID   int64
	OperationID            int64
}

// ReverseSplit posts the exact inverse of everything the split's journals
// currently move and replays the holding without it. A later sale the
// unsplit holding cannot satisfy refuses the whole command, naming the sale.
type InvestmentSplitReversalResult struct {
	Transaction            Transaction
	CorrectedOperationID   int64
	CorrectedTransactionID int64
}

func (s *InvestmentService) ReverseSplit(ctx context.Context, input ReverseInvestmentSplitInput) (Transaction, error) {
	result, err := s.ReverseSplitWithIdentity(ctx, input)
	return result.Transaction, err
}

func (s *InvestmentService) ReverseSplitWithIdentity(ctx context.Context, input ReverseInvestmentSplitInput) (InvestmentSplitReversalResult, error) {
	operation, params, err := s.prepareSplitReversalWrite(ctx, input)
	if err != nil {
		return InvestmentSplitReversalResult{}, err
	}
	record, err := s.repository.ReverseSplit(ctx, params, operation)
	if err != nil {
		return InvestmentSplitReversalResult{}, mapSplitCorrectionError(err)
	}
	transaction, err := s.transactionService.enrichOne(ctx, toTransaction(record))
	if err != nil {
		return InvestmentSplitReversalResult{}, err
	}
	return InvestmentSplitReversalResult{Transaction: transaction, CorrectedOperationID: operation.OperationID,
		CorrectedTransactionID: operation.TransactionID}, nil
}

// ReverseSplitReconciliationImpact runs the reversal writer in a rolled-back
// transaction and reports its checkpoints and revised gains.
func (s *InvestmentService) ReverseSplitReconciliationImpact(ctx context.Context, input ReverseInvestmentSplitInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	operation, params, err := s.prepareSplitReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewSplitReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapSplitCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareSplitReversalWrite(ctx context.Context, input ReverseInvestmentSplitInput) (db.SplitOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.splitInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.OperationID, input.Reason, input.ReconciliationOverride, "investment.split.reverse")
	if err != nil {
		return db.SplitOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.prepareSplitInverseHeader(ctx, operation, planned)
	if err != nil {
		return db.SplitOperationRecord{}, db.CreateTransactionParams{}, err
	}
	linkCorrection(&params, operation.OperationID, "reverse", planned.ChangeReason)
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

// splitInversePlan reads the split, applies the correction fences and plans a
// journal dated to the split that negates its current journal delta: the
// primary journal plus every adjustment (T-129).
func (s *InvestmentService) splitInversePlan(ctx context.Context, ownerUserID, authSessionID int64, requestID string,
	transactionID, operationID int64, rawReason string, override CheckpointOverride, auditOperation string) (db.SplitOperationRecord, CreateTransactionInput, error) {
	reason, err := splitCorrection.admit(ownerUserID, (transactionID <= 0) != (operationID <= 0), rawReason)
	if err != nil {
		return db.SplitOperationRecord{}, CreateTransactionInput{}, err
	}
	var operation db.SplitOperationRecord
	if operationID > 0 {
		operation, err = s.repository.SplitOperationByID(ctx, BookID, operationID)
	} else {
		operation, err = s.repository.SplitOperationByTransactionID(ctx, BookID, transactionID)
	}
	if err != nil {
		return db.SplitOperationRecord{}, CreateTransactionInput{}, mapSplitCorrectionError(err)
	}
	original, err := s.verifyCorrectionTarget(ctx, splitCorrection, correctionTarget{
		OperationID: operation.OperationID, TransactionID: operation.TransactionID, CurrentVersionID: operation.CurrentVersionID,
		EventDate: operation.EventDate, AlreadyCorrected: operation.AlreadyCorrected, ImportedLineage: operation.ImportedLineage,
	})
	if err != nil {
		return db.SplitOperationRecord{}, CreateTransactionInput{}, err
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		return db.SplitOperationRecord{}, CreateTransactionInput{}, fmt.Errorf("resolve commodity trading account: %w", err)
	}
	memo := ""
	if len(original.JournalEntries) > 0 {
		memo = original.JournalEntries[0].Memo
	}
	return operation, correctionInverseInput(ownerUserID, authSessionID, requestID, auditOperation,
		splitPrimaryTransactionID(operation.TransactionID), TransactionInput{
			Status: "posted", TransactionKind: "investment", TransactionDate: operation.EventDate,
			Description: original.Description,
			JournalEntries: []JournalEntryInput{{EntryDate: operation.EventDate, EntryKind: "investment", Memo: memo,
				Postings: []PostingInput{
					{AccountID: operation.AccountID, CommodityID: operation.CommodityID,
						QuantityValue: operation.JournalDeltaValue.Negated(), QuantityScale: operation.JournalDeltaScale, Memo: memo},
					{AccountID: tradingID, CommodityID: operation.CommodityID,
						QuantityValue: operation.JournalDeltaValue, QuantityScale: operation.JournalDeltaScale, Memo: memo},
				}}},
		}, reason, override), nil
}

// ReplaceSplit reverses the split and records corrected terms as its
// successor at the same correction-root slot, replaying once.
func (s *InvestmentService) ReplaceSplit(ctx context.Context, input ReplaceInvestmentSplitInput) (ReplaceInvestmentSplitResult, error) {
	operation, inverse, replacement, split, plan, err := s.prepareSplitReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceInvestmentSplitResult{}, err
	}
	record, err := s.repository.ReplaceSplit(ctx, operation, inverse, replacement, split)
	if err != nil {
		return ReplaceInvestmentSplitResult{}, mapSplitCorrectionError(err)
	}
	enriched, err := s.transactionService.enrichRecords(ctx, record.Inverse, record.Replacement)
	if err != nil {
		return ReplaceInvestmentSplitResult{}, err
	}
	return ReplaceInvestmentSplitResult{
		Inverse: enriched[0], Replacement: enriched[1],
		Plan:                   toInvestmentSplitPlan(plan.RatioNumerator, plan.RatioDenominator, record.Plan),
		CorrectedTransactionID: operation.TransactionID, CorrectedOperationID: operation.OperationID, OperationID: record.Plan.OperationID,
	}, nil
}

// PreviewSplitReplacement returns the corrected split's per-lot plan plus
// the checkpoints and gains the replacement writer would change.
func (s *InvestmentService) PreviewSplitReplacement(ctx context.Context, input ReplaceInvestmentSplitInput) (InvestmentSplitPreview, error) {
	input.ReconciliationOverride = unboundOverride
	operation, inverse, replacement, split, plan, err := s.prepareSplitReplacementWrite(ctx, input)
	if err != nil {
		return InvestmentSplitPreview{}, err
	}
	inverse.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateSplitReplacement(ctx, operation, inverse, replacement, split)
	if err != nil {
		return InvestmentSplitPreview{}, mapSplitCorrectionError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return InvestmentSplitPreview{}, err
	}
	return InvestmentSplitPreview{Plan: plan, Impact: impact}, nil
}

func (s *InvestmentService) prepareSplitReplacementWrite(ctx context.Context, input ReplaceInvestmentSplitInput) (
	db.SplitOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateSplitParams, InvestmentSplitPlan, error) {
	fail := func(err error) (db.SplitOperationRecord, db.CreateTransactionParams, db.CreateTransactionParams, db.CreateSplitParams, InvestmentSplitPlan, error) {
		return db.SplitOperationRecord{}, db.CreateTransactionParams{}, db.CreateTransactionParams{}, db.CreateSplitParams{}, InvestmentSplitPlan{}, err
	}
	operation, plannedInverse, err := s.splitInversePlan(ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID,
		input.TransactionID, input.OperationID, input.Reason, input.ReconciliationOverride, "investment.split.replace")
	if err != nil {
		return fail(err)
	}
	inverse, err := s.prepareSplitInverseHeader(ctx, operation, plannedInverse)
	if err != nil {
		return fail(err)
	}
	inverse.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	// Omitted evidence keeps the recorded evidence instead of becoming {};
	// edits apply to its exact recorded tokens (#312).
	evidence, err := correctedSourceEvidence(operation.SourceEvidenceJSON, input.SourceEvidenceJSON, input.SourceEvidenceEditsJSON)
	if err != nil {
		return fail(err)
	}
	replacement, split, plan, err := s.prepareSplitWriteWith(ctx, InvestmentSplitInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		Operation: "investment.split.replace", EffectiveOn: input.EffectiveOn,
		HoldingAccountID: operation.AccountID, CommodityID: operation.CommodityID,
		RatioNumerator: input.RatioNumerator, RatioDenominator: input.RatioDenominator,
		SourceEvidenceJSON: evidence, Memo: input.Memo, ChangeReason: plannedInverse.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
	}, operation.OperationID)
	if err != nil {
		return fail(err)
	}
	if split.RatioNumerator == operation.RatioNumerator && split.RatioDenominator == operation.RatioDenominator &&
		split.EffectiveOn == operation.EventDate && sameSourceEvidence(split.SourceEvidenceJSON, operation.SourceEvidenceJSON) {
		return fail(ValidationError{Message: "a split replacement must change the date, ratio or source evidence"})
	}
	if replacement.AuditOnly {
		inverse.AuditMetadataJSON = replacement.Spec.MetadataJSON
	}
	replacement.CorrectionOfTransactionID = sql.NullInt64{Int64: operation.TransactionID, Valid: operation.TransactionID > 0}
	linkCorrection(&replacement, operation.OperationID, "replace", plannedInverse.ChangeReason)
	return operation, inverse, replacement, split, plan, nil
}

func splitPrimaryTransactionID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

func (s *InvestmentService) prepareSplitInverseHeader(ctx context.Context, operation db.SplitOperationRecord, planned CreateTransactionInput) (db.CreateTransactionParams, error) {
	if operation.JournalDeltaValue.Sign() != 0 {
		return s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	}
	header := s.journalFreeInvestmentHeader(planned.OwnerUserID, planned.AuthSessionID, planned.RequestID,
		planned.Operation, planned.ChangeReason, planned.Spec.InvestmentOperationKind, operation.EventDate,
		CheckpointOverride{Allowed: planned.ReconciliationOverride, Reviewed: planned.ReconciliationOverrideCheckpointIDs})
	header.CorrectionOfTransactionID = sql.NullInt64{Int64: operation.TransactionID, Valid: operation.TransactionID > 0}
	return header, nil
}

func mapSplitCorrectionError(err error) error {
	if dependency, ok := errors.AsType[*db.InvestmentReplayDependencyError](err); ok {
		return InvestmentSplitDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentSplitNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentSplitAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentImportedCorrection):
		return ErrInvestmentImportedSplit
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentSplitChanged
	}
	return mapInvestmentSplitError(err)
}

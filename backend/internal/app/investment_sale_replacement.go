package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

var ErrInvestmentSaleDependency = errors.New("corrected investment sale cannot satisfy a dependent disposal")

type InvestmentSaleDependencyError struct {
	OperationID int64
	DecisionID  int64
}

func (e InvestmentSaleDependencyError) Error() string {
	return fmt.Sprintf("%s: later operation %d decision %d",
		ErrInvestmentSaleDependency, e.OperationID, e.DecisionID)
}

func (e InvestmentSaleDependencyError) Unwrap() error { return ErrInvestmentSaleDependency }

type ReplaceInvestmentSaleInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	Replacement            InvestmentTradeInput
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted (T-126).
	GainImpactAcknowledgement string
}

type ReplaceInvestmentSaleResult struct {
	CorrectedTransactionID int64
	Inverse                Transaction
	Replacement            InvestmentTradeResult
}

// ReplaceSale corrects an effective manual or source-linked imported long sale and replays dependent
// long-position decisions under one audited repository transaction.
func (s *InvestmentService) ReplaceSale(ctx context.Context, input ReplaceInvestmentSaleInput) (ReplaceInvestmentSaleResult, error) {
	return s.replaceSaleWithPostWriteOrigin(ctx, input, "browser_api", "investment.sale.replace", nil)
}

func (s *InvestmentService) replaceSaleWithPostWriteOrigin(ctx context.Context, input ReplaceInvestmentSaleInput,
	originType, operationCode string, postWrite func(*sql.Tx, int64, int64) error,
) (ReplaceInvestmentSaleResult, error) {
	prepared, err := s.prepareSaleReplacementWrite(ctx, input, originType, operationCode)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	inverse, replacementRecord, disposals, decision, err := s.repository.ReplaceSaleWithPostWrite(ctx, prepared.Operation,
		prepared.Inverse, prepared.Replacement, prepared.Disposal, postWrite)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, mapReplaceSaleError(err, prepared.Operation.OperationID)
	}
	committedDecision := toDisposalDecision(decision)
	return ReplaceInvestmentSaleResult{
		CorrectedTransactionID: prepared.Operation.TransactionID,
		Inverse:                toTransaction(inverse),
		Replacement: InvestmentTradeResult{Transaction: toTransaction(replacementRecord),
			Allocations: toInvestmentLotDisposals(disposals), DisposalDecision: &committedDecision},
	}, nil
}

type preparedSaleReplacementWrite struct {
	Operation   db.SaleOperationRecord
	Inverse     db.CreateTransactionParams
	Replacement db.CreateTransactionParams
	Disposal    db.DisposeLotsParams
}

// Preview and commit freeze identical inverse, replacement and disposal facts.
func (s *InvestmentService) prepareSaleReplacementWrite(ctx context.Context, input ReplaceInvestmentSaleInput,
	originType, operationCode string,
) (preparedSaleReplacementWrite, error) {
	operation, inversePlan, err := s.reverseSalePlan(ctx, ReverseInvestmentSaleInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: input.TransactionID,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
	})
	if err != nil {
		return preparedSaleReplacementWrite{}, err
	}
	// The date, holding account, instrument and cost currency may change
	// (T-116); the writer replays the source and the new position together.
	replacement := input.Replacement
	if err := validateSaleReplacementElections(replacement); err != nil {
		return preparedSaleReplacementWrite{}, err
	}
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.OriginType = originType
	replacement.Operation = operationCode
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	replacement.WriteOff = false
	inversePlan.OriginType = originType
	inversePlan.Operation = operationCode
	inversePlan.Spec.InvestmentOperationKind = ""
	inverseParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return preparedSaleReplacementWrite{}, err
	}
	replacementParams, disposalParams, err := s.prepareSellWrite(ctx, replacement)
	if err != nil {
		return preparedSaleReplacementWrite{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	// The compound command's first journal carries the disclosure policy.
	inverseParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	replacementParams.GainImpact = nil
	return preparedSaleReplacementWrite{Operation: operation, Inverse: inverseParams,
		Replacement: replacementParams, Disposal: disposalParams}, nil
}

// ReplaceSaleReconciliationImpact runs the complete replacement writer in a
// rolled-back transaction: the same inverse, corrected disposal, dependent
// replay, prices and checkpoint invalidation as commit (T-126).
func (s *InvestmentService) ReplaceSaleReconciliationImpact(ctx context.Context, input ReplaceInvestmentSaleInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	prepared, err := s.prepareSaleReplacementWrite(ctx, input, "browser_api", "investment.sale.replace")
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewSaleReplacement(ctx, prepared.Operation, prepared.Inverse,
		prepared.Replacement, prepared.Disposal)
	if err != nil {
		return ReconciliationImpact{}, mapReplaceSaleError(err, prepared.Operation.OperationID)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func mergeInvestmentCorrectionImpacts(inverseImpact, replacementImpact ReconciliationImpact) ReconciliationImpact {
	seen := make(map[int64]bool)
	for _, checkpoint := range inverseImpact.AffectedCheckpoints {
		seen[checkpoint.CheckpointID] = true
	}
	for _, checkpoint := range replacementImpact.AffectedCheckpoints {
		if !seen[checkpoint.CheckpointID] {
			inverseImpact.AffectedCheckpoints = append(inverseImpact.AffectedCheckpoints, checkpoint)
			seen[checkpoint.CheckpointID] = true
		}
	}
	return inverseImpact
}

func validateSaleReplacementElections(replacement InvestmentTradeInput) error {
	if replacement.CostBasisMethod == "" {
		return ValidationError{Message: "replacement cost-basis method is required"}
	}
	for index, charge := range replacement.Charges {
		if charge.Treatment == "" {
			return ValidationError{Message: fmt.Sprintf("replacement charge %d treatment is required", index+1)}
		}
	}
	return nil
}

func mapReplaceSaleError(err error, sourceOperationID int64) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		if dependency.OperationID == sourceOperationID {
			return ErrInvestmentLotsInsufficient
		}
		return InvestmentSaleDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	if errors.Is(err, db.ErrInsufficientLots) {
		return ErrInvestmentLotsInsufficient
	}
	if errors.Is(err, db.ErrOutOfOrderPositionEvent) {
		return ErrInvestmentEventOutOfOrder
	}
	return fmt.Errorf("replace investment sale: %w", mapReverseSaleError(err))
}

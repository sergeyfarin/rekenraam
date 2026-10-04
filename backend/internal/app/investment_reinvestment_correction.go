package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

var (
	ErrInvestmentReinvestmentNotFound         = errors.New("reinvested dividend operation not found")
	ErrInvestmentReinvestmentAlreadyCorrected = errors.New("reinvested dividend already corrected")
	ErrInvestmentReinvestmentChanged          = errors.New("reinvested dividend changed")
)

// A reinvested dividend is an acquisition paid for by income instead of cash
// (T-115). Its reversal and replacement reuse the acquisition writer: the
// inverse journal and replayed lot projection commit together, dependent
// disposals replay under their recorded methods and elections, the trade
// price is retired, checkpoints are guarded and changed gains need the exact
// acknowledgement. The original journal and opening lot stay as history.

type ReverseReinvestedDividendInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted.
	GainImpactAcknowledgement string
}

type ReplaceReinvestedDividendInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	Replacement               ReinvestedDividendInput
	GainImpactAcknowledgement string
}

type ReplaceReinvestedDividendResult struct {
	Inverse                Transaction
	Replacement            InvestmentTradeResult
	CorrectedTransactionID int64
}

func (s *InvestmentService) ReverseReinvestedDividend(ctx context.Context, input ReverseReinvestedDividendInput) (Transaction, error) {
	operation, params, err := s.prepareReinvestmentReversalWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.repository.ReverseBuy(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapReinvestmentCorrectionError(err)
	}
	return toTransaction(record), nil
}

// ReverseReinvestedDividendReconciliationImpact runs the reversal writer and
// its dependent replay in a rolled-back transaction.
func (s *InvestmentService) ReverseReinvestedDividendReconciliationImpact(ctx context.Context, input ReverseReinvestedDividendInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareReinvestmentReversalWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.PreviewBuyReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapReinvestmentCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareReinvestmentReversalWrite(ctx context.Context, input ReverseReinvestedDividendInput) (db.BuyOperationRecord, db.CreateTransactionParams, error) {
	operation, planned, err := s.acquisitionCorrectionPlan(ctx, acquisitionCorrectionRequest{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionID: input.TransactionID, Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		OperationKind: "reinvested_dividend", Operation: "investment.reinvested_dividend.reverse", Noun: "reinvested dividend",
	})
	if err != nil {
		return db.BuyOperationRecord{}, db.CreateTransactionParams{}, err
	}
	planned.Spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, planned, nil)
	if err != nil {
		return db.BuyOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = planned.ChangeReason
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	return operation, params, nil
}

func (s *InvestmentService) ReplaceReinvestedDividend(ctx context.Context, input ReplaceReinvestedDividendInput) (ReplaceReinvestedDividendResult, error) {
	prepared, err := s.prepareReinvestmentReplacementWrite(ctx, input)
	if err != nil {
		return ReplaceReinvestedDividendResult{}, err
	}
	record, err := s.repository.ReplaceBuy(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot)
	if err != nil {
		return ReplaceReinvestedDividendResult{}, mapReinvestmentCorrectionError(err)
	}
	lotID := record.Lot.ID
	return ReplaceReinvestedDividendResult{
		Inverse:                toTransaction(record.Inverse),
		Replacement:            InvestmentTradeResult{Transaction: toTransaction(record.Replacement), LotID: &lotID},
		CorrectedTransactionID: prepared.Operation.TransactionID,
	}, nil
}

// ReplaceReinvestedDividendReconciliationImpact runs the whole replacement
// writer — inverse, corrected opening, replay, price retirement and
// checkpoint invalidation — and rolls it back.
func (s *InvestmentService) ReplaceReinvestedDividendReconciliationImpact(ctx context.Context, input ReplaceReinvestedDividendInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	prepared, err := s.prepareReinvestmentReplacementWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	simulated, err := s.repository.SimulateBuyReplacement(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Lot)
	if err != nil {
		return ReconciliationImpact{}, mapReinvestmentCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// Preview and commit freeze identical journals and lot facts.
func (s *InvestmentService) prepareReinvestmentReplacementWrite(ctx context.Context, input ReplaceReinvestedDividendInput) (preparedBuyReplacementWrite, error) {
	const operationCode = "investment.reinvested_dividend.replace"
	operation, inversePlan, err := s.acquisitionCorrectionPlan(ctx, acquisitionCorrectionRequest{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionID: input.TransactionID, Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
		OperationKind: "reinvested_dividend", Operation: operationCode, Noun: "reinvested dividend",
	})
	if err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	replacement := input.Replacement
	if replacement.TransactionDate != operation.EventDate ||
		replacement.HoldingAccountID != operation.AccountID ||
		replacement.CommodityID != operation.CommodityID ||
		replacement.CashCommodityID != operation.CostCommodityID {
		return preparedBuyReplacementWrite{}, ValidationError{Message: "replacement must keep the reinvested dividend date, holding account, instrument and currency"}
	}
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	replacement.Status = "posted"
	replacement.GainImpactAcknowledgement = ""
	inverseParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	replacementParams, lotParams, err := s.prepareReinvestmentWriteAs(ctx, replacement, operationCode)
	if err != nil {
		return preparedBuyReplacementWrite{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	// The compound command's first journal carries the disclosure policy.
	inverseParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	replacementParams.GainImpact = nil
	return preparedBuyReplacementWrite{Operation: operation, Inverse: inverseParams, Replacement: replacementParams, Lot: lotParams}, nil
}

func mapReinvestmentCorrectionError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.Is(err, db.ErrInvestmentCorrectionDependency) && errors.As(err, &dependency) {
		return InvestmentBuyDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentReinvestmentNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentReinvestmentAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentReinvestmentChanged
	case errors.Is(err, db.ErrInvestmentCorrectionDependency):
		return ErrInvestmentBuyDependency
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("correct reinvested dividend: %w", mapTransactionDBError(err))
	}
}

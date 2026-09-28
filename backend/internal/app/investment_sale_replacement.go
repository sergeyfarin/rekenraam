package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

var ErrInvestmentSaleNotLatest = errors.New("investment sale has a later position operation")

type ReplaceInvestmentSaleInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	Replacement            InvestmentTradeInput
}

type ReplaceInvestmentSaleResult struct {
	Inverse     Transaction
	Replacement InvestmentTradeResult
}

// ReplaceLatestSale is the first bounded replacement path. The original sale
// must be the position's latest lot-affecting intent, so its replacement can
// be disposed through the existing guarded engine after restoring the prior
// effective lot projection. A later dependency needs full chronological replay
// of a newly created disposal and remains a separate slice.
func (s *InvestmentService) ReplaceLatestSale(ctx context.Context, input ReplaceInvestmentSaleInput) (ReplaceInvestmentSaleResult, error) {
	operation, inversePlan, err := s.reverseSalePlan(ctx, ReverseInvestmentSaleInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: input.TransactionID,
		Reason: input.Reason, ReconciliationOverride: input.ReconciliationOverride,
	})
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	replacement := input.Replacement
	if err := validateSaleReplacementPosition(replacement, operation); err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	if err := validateSaleReplacementElections(replacement); err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	replacement.OwnerUserID = input.OwnerUserID
	replacement.AuthSessionID = input.AuthSessionID
	replacement.RequestID = input.RequestID
	replacement.OriginType = "browser_api"
	replacement.Operation = "investment.sale.replace"
	replacement.ChangeReason = inversePlan.ChangeReason
	replacement.ReconciliationOverride = input.ReconciliationOverride
	replacement.WriteOff = false
	inversePlan.Operation = "investment.sale.replace"
	inversePlan.Spec.InvestmentOperationKind = ""
	inverseParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	replacementParams, disposalParams, err := s.prepareSellWrite(ctx, replacement)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	replacementParams.CorrectionOfTransactionID = inverseParams.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverseParams.CreatedAt
	inverse, replacementRecord, disposals, decision, err := s.repository.ReplaceLatestSale(ctx, operation, inverseParams, replacementParams, disposalParams)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, mapReplaceSaleError(err)
	}
	committedDecision := toDisposalDecision(decision)
	return ReplaceInvestmentSaleResult{
		Inverse: toTransaction(inverse),
		Replacement: InvestmentTradeResult{Transaction: toTransaction(replacementRecord),
			Allocations: toInvestmentLotDisposals(disposals), DisposalDecision: &committedDecision},
	}, nil
}

func (s *InvestmentService) ReplaceLatestSaleReconciliationImpact(ctx context.Context, input ReplaceInvestmentSaleInput) (ReconciliationImpact, error) {
	operation, inversePlan, err := s.reverseSalePlan(ctx, ReverseInvestmentSaleInput{
		OwnerUserID: input.OwnerUserID, TransactionID: input.TransactionID, Reason: input.Reason,
	})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := validateSaleReplacementPosition(input.Replacement, operation); err != nil {
		return ReconciliationImpact{}, err
	}
	if err := validateSaleReplacementElections(input.Replacement); err != nil {
		return ReconciliationImpact{}, err
	}
	intents, err := s.repository.ListInvestmentReplayIntents(ctx, BookID,
		operation.AccountID, operation.CommodityID, operation.CostCommodityID, "long")
	if err != nil {
		return ReconciliationImpact{}, fmt.Errorf("check replacement position order: %w", err)
	}
	if len(intents) == 0 || intents[len(intents)-1].OperationID != operation.OperationID {
		return ReconciliationImpact{}, ErrInvestmentSaleNotLatest
	}
	replacement := input.Replacement
	replacement.OwnerUserID = input.OwnerUserID
	replacement.WriteOff = false
	plan, err := s.sellPlan(ctx, replacement)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	inverseImpact, err := s.transactionService.investmentReconciliationImpactForCreate(ctx,
		CreateReconciliationImpactInput{OwnerUserID: input.OwnerUserID, Spec: inversePlan.Spec})
	if err != nil {
		return ReconciliationImpact{}, err
	}
	replacementImpact, err := s.transactionService.investmentReconciliationImpactForCreate(ctx,
		CreateReconciliationImpactInput{OwnerUserID: input.OwnerUserID, Spec: plan.Create.Spec})
	if err != nil {
		return ReconciliationImpact{}, err
	}
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
	return inverseImpact, nil
}

func validateSaleReplacementPosition(replacement InvestmentTradeInput, operation db.SaleOperationRecord) error {
	if replacement.TransactionDate != operation.EventDate ||
		replacement.HoldingAccountID != operation.AccountID ||
		replacement.CommodityID != operation.CommodityID ||
		replacement.CashCommodityID != operation.CostCommodityID {
		return ValidationError{Message: "replacement must keep the sale date, holding account, instrument and cost currency"}
	}
	return nil
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

func mapReplaceSaleError(err error) error {
	if errors.Is(err, db.ErrInvestmentSaleNotLatest) {
		return ErrInvestmentSaleNotLatest
	}
	if errors.Is(err, db.ErrInsufficientLots) {
		return ErrInvestmentLotsInsufficient
	}
	if errors.Is(err, db.ErrOutOfOrderPositionEvent) {
		return ErrInvestmentEventOutOfOrder
	}
	return fmt.Errorf("replace investment sale: %w", mapReverseSaleError(err))
}

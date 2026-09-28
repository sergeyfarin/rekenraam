package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type InternalTransferInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	EffectiveOn            string
	SourceAccountID        int64
	DestinationAccountID   int64
	CommodityID            int64
	CostCommodityID        int64
	Allocations            []InvestmentLotAllocationInput
	SourceEvidenceJSON     string
	Memo                   string
	ChangeReason           string
	ReconciliationOverride bool
}

type InternalTransferResult struct {
	Transaction       Transaction
	DestinationLotIDs []int64
}

func (s *InvestmentService) internalTransferPlan(ctx context.Context, input InternalTransferInput) (investmentTransactionPlan, db.CreateInternalTransferParams, error) {
	if input.OwnerUserID <= 0 {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "owner user is required"}
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "transfer effective date")
	if err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	if input.SourceAccountID <= 0 || input.DestinationAccountID <= 0 || input.SourceAccountID == input.DestinationAccountID {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "different source and destination holding accounts are required"}
	}
	if input.CostCommodityID <= 0 || len(input.Allocations) == 0 {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "basis currency and source lots are required"}
	}
	allocations := make([]db.LotAllocation, 0, len(input.Allocations))
	seen := make(map[int64]bool, len(input.Allocations))
	total := exact.NewScaledInt()
	for index, allocation := range input.Allocations {
		if allocation.LotID <= 0 || seen[allocation.LotID] || allocation.QuantityValue.Sign() <= 0 ||
			allocation.QuantityScale < 0 || allocation.QuantityScale > exact.MaxCryptoScale {
			return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: fmt.Sprintf("source lot allocation %d is invalid or repeated", index+1)}
		}
		seen[allocation.LotID] = true
		total.AddCoefficient(allocation.QuantityValue, allocation.QuantityScale)
		allocations = append(allocations, db.LotAllocation{
			LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale,
		})
	}
	quantity, err := total.Coefficient()
	if err != nil || quantity.Sign() <= 0 {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "total transfer quantity is outside the exact range"}
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.SourceAccountID, date, holdingRole, dependencies); err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	if _, err := s.accountInRole(ctx, input.DestinationAccountID, date, holdingRole, dependencies); err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "transferred commodity", false); err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CostCommodityID, date, "basis commodity", true); err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, err
	}
	plan := investmentTransactionPlan{
		Date: date, MetadataJSON: evidence, AccountRuleDependencies: dependencies,
		Create: CreateTransactionInput{
			OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
			RequestID: input.RequestID, OriginType: "browser_api",
			Operation: "investment.internal_transfer", ChangeReason: input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride,
			Spec: TransactionInput{
				Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "internal_transfer",
				TransactionDate: date, Description: memo,
				JournalEntries: []JournalEntryInput{{EntryDate: date, EntryKind: "investment", Memo: memo,
					Postings: []PostingInput{
						{AccountID: input.SourceAccountID, CommodityID: input.CommodityID,
							QuantityValue: quantity.Negated(), QuantityScale: total.Scale(), Memo: memo},
						{AccountID: input.DestinationAccountID, CommodityID: input.CommodityID,
							QuantityValue: quantity, QuantityScale: total.Scale(), Memo: memo},
					}}},
			},
		},
	}
	transfer := db.CreateInternalTransferParams{
		BookID: BookID, SourceAccountID: input.SourceAccountID,
		DestinationAccountID: input.DestinationAccountID, CommodityID: input.CommodityID,
		CostCommodityID: input.CostCommodityID, EffectiveOn: date,
		Allocations: allocations, SourceEvidenceJSON: evidence,
	}
	return plan, transfer, nil
}

func (s *InvestmentService) PreviewInternalTransferReconciliationImpact(ctx context.Context, input InternalTransferInput) (ReconciliationImpact, error) {
	plan, transfer, err := s.internalTransferPlan(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	if err := s.repository.PreviewInternalTransferLots(ctx, transfer, input.OwnerUserID); err != nil {
		return ReconciliationImpact{}, mapInternalTransferError(err)
	}
	return s.reconciliationImpactForPlan(ctx, plan)
}

func (s *InvestmentService) InternalTransfer(ctx context.Context, input InternalTransferInput) (InternalTransferResult, error) {
	plan, transfer, err := s.internalTransferPlan(ctx, input)
	if err != nil {
		return InternalTransferResult{}, err
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return InternalTransferResult{}, err
	}
	transaction, result, err := s.repository.CreateInternalTransfer(ctx, journal, transfer)
	if err != nil {
		return InternalTransferResult{}, mapInternalTransferError(err)
	}
	return InternalTransferResult{Transaction: toTransaction(transaction), DestinationLotIDs: result.DestinationLotIDs}, nil
}

func mapInternalTransferError(err error) error {
	switch {
	case errors.Is(err, db.ErrOutOfOrderPositionEvent):
		return err
	case errors.Is(err, db.ErrInsufficientLots), errors.Is(err, db.ErrNotFound):
		return ErrInvestmentLotsInsufficient
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("internal investment transfer: %w", mapTransactionDBError(err))
	}
}

package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// InternalTransferInput moves a holding between two of the book's accounts.
// An individual-lot source names Allocations; an average-cost source names a
// total Quantity and the writer allocates it from the dated pool (T-123).
// DestinationLineage (T-135) chooses, for a pooled quantity only, between one
// pooled destination lot (the default) and one lot per depleted source lot.
type InternalTransferInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	EffectiveOn               string
	SourceAccountID           int64
	DestinationAccountID      int64
	CommodityID               int64
	CostCommodityID           int64
	Allocations               []InvestmentLotAllocationInput
	QuantityValue             exact.Coefficient
	QuantityScale             int
	DestinationLineage        string
	SourceEvidenceJSON        string
	Memo                      string
	ChangeReason              string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// InternalTransferLink is one source lot's carried quantity and basis, or a
// pooled lot's whole move (SourceLotID zero). DestinationLotID is zero in a
// preview.
type InternalTransferLink struct {
	SourceLotID           int64
	DestinationLotID      int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	CarriedBasisValue     int64
	CarriedBasisScale     int
	BasisKnowledge        string // unknown leaves CarriedBasis unused
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

// InternalTransferPlan is what the writer allocated: the applied method, how
// it resolved, and every carried link.
type InternalTransferPlan struct {
	BasisAllocation    string
	DestinationLineage string
	CostBasisMethod    string
	ResolutionTier     string
	Links              []InternalTransferLink
}

type InternalTransferPreview struct {
	Plan   InternalTransferPlan
	Impact ReconciliationImpact
}

type InternalTransferResult struct {
	Transaction       Transaction
	Plan              InternalTransferPlan
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
	pooled := len(input.Allocations) == 0
	if input.CostCommodityID <= 0 || (pooled && input.QuantityValue.Sign() <= 0) {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "basis currency and source lots or a pooled quantity are required"}
	}
	if !pooled && input.QuantityValue != "" {
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "send source lots or a pooled quantity, not both"}
	}
	switch input.DestinationLineage {
	case "", db.InternalTransferSourceLots:
	case db.InternalTransferPooledLot:
		if !pooled {
			return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "a pooled destination lot needs a pooled quantity"}
		}
	default:
		return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "destination lineage must be source_lots or pooled_lot"}
	}
	allocations := make([]db.LotAllocation, 0, len(input.Allocations))
	seen := make(map[int64]bool, len(input.Allocations))
	total := exact.NewScaledInt()
	if pooled {
		if input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale {
			return investmentTransactionPlan{}, db.CreateInternalTransferParams{}, ValidationError{Message: "pooled transfer quantity scale is invalid"}
		}
		total.AddCoefficient(input.QuantityValue, input.QuantityScale)
	}
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
	method, methodSource, err := s.resolveCostBasisMethod(ctx, input.SourceAccountID, "")
	if err != nil {
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
		SourceCostBasisMethod: method, SourceMethodSource: methodSource,
		DestinationLineage: input.DestinationLineage,
	}
	if pooled {
		transfer.PooledQuantityValue, transfer.PooledQuantityScale = quantity, total.Scale()
	}
	return plan, transfer, nil
}

// PreviewInternalTransfer runs the complete transfer writer, including the
// pooled allocation, checkpoint invalidation and gain comparison, then rolls
// back. The plan shows exactly what a commit would carry.
func (s *InvestmentService) PreviewInternalTransfer(ctx context.Context, input InternalTransferInput) (InternalTransferPreview, error) {
	input.ReconciliationOverride = true
	plan, transfer, err := s.internalTransferPlan(ctx, input)
	if err != nil {
		return InternalTransferPreview{}, err
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return InternalTransferPreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, result, err := s.repository.SimulateInternalTransfer(ctx, journal, transfer)
	if err != nil {
		return InternalTransferPreview{}, mapInternalTransferError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return InternalTransferPreview{}, err
	}
	return InternalTransferPreview{Plan: toInternalTransferPlan(result), Impact: impact}, nil
}

func (s *InvestmentService) PreviewInternalTransferReconciliationImpact(ctx context.Context, input InternalTransferInput) (ReconciliationImpact, error) {
	preview, err := s.PreviewInternalTransfer(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return preview.Impact, nil
}

// InternalTransfer commits the journal, source depletions, destination lots
// and links under one audit event. A change to any committed disposal's gain
// needs the preview's acknowledgement (T-114).
func (s *InvestmentService) InternalTransfer(ctx context.Context, input InternalTransferInput) (InternalTransferResult, error) {
	plan, transfer, err := s.internalTransferPlan(ctx, input)
	if err != nil {
		return InternalTransferResult{}, err
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return InternalTransferResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, result, err := s.repository.CreateInternalTransfer(ctx, journal, transfer)
	if err != nil {
		return InternalTransferResult{}, mapInternalTransferError(err)
	}
	return InternalTransferResult{Transaction: toTransaction(transaction), Plan: toInternalTransferPlan(result),
		DestinationLotIDs: result.DestinationLotIDs}, nil
}

func toInternalTransferPlan(result db.InternalTransferResult) InternalTransferPlan {
	plan := InternalTransferPlan{BasisAllocation: result.BasisAllocation, DestinationLineage: result.DestinationLineage,
		CostBasisMethod: result.CostBasisMethod,
		ResolutionTier:  result.ResolutionTier, Links: make([]InternalTransferLink, 0, len(result.Links))}
	for _, link := range result.Links {
		plan.Links = append(plan.Links, InternalTransferLink(link))
	}
	return plan
}

func mapInternalTransferError(err error) error {
	switch {
	case errors.Is(err, db.ErrOutOfOrderPositionEvent):
		return err
	case errors.Is(err, db.ErrInsufficientLots), errors.Is(err, db.ErrNotFound):
		return ErrInvestmentLotsInsufficient
	case errors.Is(err, db.ErrAverageCostTransferRequiresPoolAllocation),
		errors.Is(err, db.ErrPooledTransferRequiresAverageCost),
		errors.Is(err, db.ErrGainImpactAcknowledgementRequired),
		errors.Is(err, db.ErrGainImpactAcknowledgementStale):
		return err
	case errors.Is(err, db.ErrInvalidTransferDestinationLineage):
		return ValidationError{Message: err.Error()}
	case errors.Is(err, db.ErrUnknownInvestmentBasis):
		return ValidationError{Message: "a holding with unresolved basis cannot be moved yet"}
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("internal investment transfer: %w", mapTransactionDBError(err))
	}
}

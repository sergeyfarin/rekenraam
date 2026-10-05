package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ExternalTransferOutInput moves a long holding out of the book (slice 5). An
// individual-lot source names Allocations; an average-cost source names a
// total Quantity and the writer takes it from the dated pool. The basis
// carried out is computed by the writer from that depletion, never supplied.
type ExternalTransferOutInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	EffectiveOn               string
	SourceAccountID           int64
	CommodityID               int64
	CostCommodityID           int64
	Allocations               []InvestmentLotAllocationInput
	QuantityValue             exact.Coefficient
	QuantityScale             int
	SourceEvidenceJSON        string
	Memo                      string
	ChangeReason              string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// ExternalTransferOutPlan is what the writer took: the applied method, how it
// resolved, every depleted lot and the exact basis bridged out of the book.
type ExternalTransferOutPlan struct {
	BasisAllocation string
	CostBasisMethod string
	ResolutionTier  string
	Links           []ExternalTransferOutLink
	BasisValue      int64
	BasisScale      int
}

type ExternalTransferOutLink struct {
	SourceLotID           int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	CarriedBasisValue     int64
	CarriedBasisScale     int
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

type ExternalTransferOutPreview struct {
	Plan   ExternalTransferOutPlan
	Impact ReconciliationImpact
}

type ExternalTransferOutResult struct {
	Transaction Transaction
	Plan        ExternalTransferOutPlan
}

func (s *InvestmentService) externalTransferOutWrite(ctx context.Context, input ExternalTransferOutInput) (db.CreateTransactionParams, db.CreateExternalTransferOutParams, error) {
	if input.OwnerUserID <= 0 {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "owner user is required"}
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "transfer effective date")
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	if input.SourceAccountID <= 0 {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "source holding account is required"}
	}
	pooled := len(input.Allocations) == 0
	if input.CostCommodityID <= 0 || (pooled && input.QuantityValue.Sign() <= 0) {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "basis currency and source lots or a pooled quantity are required"}
	}
	if !pooled && input.QuantityValue != "" {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "send source lots or a pooled quantity, not both"}
	}
	allocations := make([]db.LotAllocation, 0, len(input.Allocations))
	seen := make(map[int64]bool, len(input.Allocations))
	total := exact.NewScaledInt()
	if pooled {
		if input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale {
			return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "pooled transfer quantity scale is invalid"}
		}
		total.AddCoefficient(input.QuantityValue, input.QuantityScale)
	}
	for index, allocation := range input.Allocations {
		if allocation.LotID <= 0 || seen[allocation.LotID] || allocation.QuantityValue.Sign() <= 0 ||
			allocation.QuantityScale < 0 || allocation.QuantityScale > exact.MaxCryptoScale {
			return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: fmt.Sprintf("source lot allocation %d is invalid or repeated", index+1)}
		}
		seen[allocation.LotID] = true
		total.AddCoefficient(allocation.QuantityValue, allocation.QuantityScale)
		allocations = append(allocations, db.LotAllocation{
			LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale,
		})
	}
	quantity, err := total.Coefficient()
	if err != nil || quantity.Sign() <= 0 {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "total transfer quantity is outside the exact range"}
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.SourceAccountID, date, holdingRole, dependencies); err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "transferred commodity", false); err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CostCommodityID, date, "basis commodity", true); err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, fmt.Errorf("resolve commodity trading account: %w", err)
	}
	// The bridge journal posts to the transfer equity account inside the
	// writer; require it here so a misconfigured book fails before writing.
	if _, err := s.repository.ExternalInvestmentTransferEquityAccountID(ctx, BookID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, ValidationError{Message: "external investment transfer equity account is required"}
		}
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, fmt.Errorf("resolve external investment transfer equity account: %w", err)
	}
	method, methodSource, err := s.resolveCostBasisMethod(ctx, input.SourceAccountID, "")
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	create := CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: "browser_api",
		Operation: "investment.external_transfer_out", ChangeReason: input.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "external_transfer_out",
			TransactionDate: date, Description: memo,
			JournalEntries: []JournalEntryInput{{EntryDate: date, EntryKind: "investment", Memo: memo,
				Postings: []PostingInput{
					{AccountID: input.SourceAccountID, CommodityID: input.CommodityID,
						QuantityValue: quantity.Negated(), QuantityScale: total.Scale(), Memo: memo},
					{AccountID: tradingID, CommodityID: input.CommodityID,
						QuantityValue: quantity, QuantityScale: total.Scale(), Memo: memo},
				}}},
		},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, create, dependencies)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateExternalTransferOutParams{}, err
	}
	transfer := db.CreateExternalTransferOutParams{
		BookID: BookID, SourceAccountID: input.SourceAccountID, CommodityID: input.CommodityID,
		CostCommodityID: input.CostCommodityID, EffectiveOn: date, Allocations: allocations,
		SourceEvidenceJSON: evidence, SourceCostBasisMethod: method, SourceMethodSource: methodSource,
	}
	if pooled {
		transfer.PooledQuantityValue, transfer.PooledQuantityScale = quantity, total.Scale()
	}
	return journal, transfer, nil
}

// PreviewExternalTransferOut runs the complete writer, including the bridge
// journal and checkpoint netting, then rolls back. The plan is exactly what a
// commit would carry out.
func (s *InvestmentService) PreviewExternalTransferOut(ctx context.Context, input ExternalTransferOutInput) (ExternalTransferOutPreview, error) {
	input.ReconciliationOverride = true
	journal, transfer, err := s.externalTransferOutWrite(ctx, input)
	if err != nil {
		return ExternalTransferOutPreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, result, err := s.repository.SimulateExternalTransferOut(ctx, journal, transfer)
	if err != nil {
		return ExternalTransferOutPreview{}, mapExternalTransferOutError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return ExternalTransferOutPreview{}, err
	}
	return ExternalTransferOutPreview{Plan: toExternalTransferOutPlan(result), Impact: impact}, nil
}

// ExternalTransferOut commits the security journal, source depletions, links,
// bridge journal and checkpoint invalidations under one audit event.
func (s *InvestmentService) ExternalTransferOut(ctx context.Context, input ExternalTransferOutInput) (ExternalTransferOutResult, error) {
	journal, transfer, err := s.externalTransferOutWrite(ctx, input)
	if err != nil {
		return ExternalTransferOutResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, result, err := s.repository.CreateExternalTransferOut(ctx, journal, transfer)
	if err != nil {
		return ExternalTransferOutResult{}, mapExternalTransferOutError(err)
	}
	return ExternalTransferOutResult{Transaction: toTransaction(transaction), Plan: toExternalTransferOutPlan(result)}, nil
}

// mapExternalTransferOutError names a later decision a backdated outbound
// transfer makes impossible, as a backdated sale does (T-143).
func mapExternalTransferOutError(err error) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentSaleDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	return mapInternalTransferError(err)
}

func toExternalTransferOutPlan(result db.ExternalTransferOutResult) ExternalTransferOutPlan {
	plan := ExternalTransferOutPlan{BasisAllocation: result.BasisAllocation, CostBasisMethod: result.CostBasisMethod,
		ResolutionTier: result.ResolutionTier, BasisValue: result.BasisValue, BasisScale: result.BasisScale,
		Links: make([]ExternalTransferOutLink, 0, len(result.Links))}
	for _, link := range result.Links {
		plan.Links = append(plan.Links, ExternalTransferOutLink(link))
	}
	return plan
}

func (s *InvestmentService) PreviewExternalTransferOutReconciliationImpact(ctx context.Context, input ExternalTransferOutInput) (ReconciliationImpact, error) {
	preview, err := s.PreviewExternalTransferOut(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return preview.Impact, nil
}

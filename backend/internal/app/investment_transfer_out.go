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
	// BasisKnowledge is unknown when any link is unknown; then nothing was
	// bridged and BasisValue/Scale are unused.
	BasisKnowledge string
}

type ExternalTransferOutLink struct {
	SourceLotID           int64
	QuantityValue         exact.Coefficient
	QuantityScale         int
	CarriedBasisValue     int64
	CarriedBasisScale     int
	BasisKnowledge        string // unknown leaves CarriedBasis unused
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
		BasisKnowledge: normalizedKnowledge(result.BasisKnowledge),
		Links:          make([]ExternalTransferOutLink, 0, len(result.Links))}
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

// ReplaceInvestmentTransferOutInput corrects a posted outbound transfer.
// Replacement is the full corrected transfer: date, source holding and lots
// or quantity may change; the security and basis currency stay its own.
type ReplaceInvestmentTransferOutInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
	Replacement               ExternalTransferOutInput
}

type ReplaceInvestmentTransferOutResult struct {
	Inverse                Transaction
	Replacement            ExternalTransferOutResult
	CorrectedTransactionID int64
}

type preparedTransferOutReplacement struct {
	operation   db.TransferOperationRecord
	inverse     db.CreateTransactionParams
	replacement db.CreateTransactionParams
	transfer    db.CreateExternalTransferOutParams
}

// ReplaceTransferOut posts the inverse (security legs and net bridge) and a
// corrected outbound transfer at the replaced transfer's slot under one audit
// event, replaying every source either transfer depleted (T-144).
func (s *InvestmentService) ReplaceTransferOut(ctx context.Context, input ReplaceInvestmentTransferOutInput) (ReplaceInvestmentTransferOutResult, error) {
	prepared, err := s.prepareTransferOutReplacement(ctx, input)
	if err != nil {
		return ReplaceInvestmentTransferOutResult{}, err
	}
	record, err := s.repository.ReplaceExternalTransferOut(ctx, prepared.operation, prepared.inverse, prepared.replacement, prepared.transfer)
	if err != nil {
		return ReplaceInvestmentTransferOutResult{}, mapTransferReplacementError(err)
	}
	return ReplaceInvestmentTransferOutResult{
		Inverse: toTransaction(record.Inverse),
		Replacement: ExternalTransferOutResult{Transaction: toTransaction(record.Replacement),
			Plan: toExternalTransferOutPlan(record.Result)},
		CorrectedTransactionID: prepared.operation.TransactionID,
	}, nil
}

// PreviewTransferOutReplacement runs the replacement writer and every replay,
// then rolls back.
func (s *InvestmentService) PreviewTransferOutReplacement(ctx context.Context, input ReplaceInvestmentTransferOutInput) (ExternalTransferOutPreview, error) {
	input.ReconciliationOverride = true
	prepared, err := s.prepareTransferOutReplacement(ctx, input)
	if err != nil {
		return ExternalTransferOutPreview{}, err
	}
	simulated, result, err := s.repository.SimulateExternalTransferOutReplacement(ctx, prepared.operation,
		prepared.inverse, prepared.replacement, prepared.transfer)
	if err != nil {
		return ExternalTransferOutPreview{}, mapTransferReplacementError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return ExternalTransferOutPreview{}, err
	}
	return ExternalTransferOutPreview{Plan: toExternalTransferOutPlan(result), Impact: impact}, nil
}

func (s *InvestmentService) prepareTransferOutReplacement(ctx context.Context, input ReplaceInvestmentTransferOutInput) (preparedTransferOutReplacement, error) {
	operation, inversePlan, err := s.transferCorrectionPlan(ctx, input.OwnerUserID, input.TransactionID, input.Reason)
	if err != nil {
		return preparedTransferOutReplacement{}, err
	}
	if operation.TransferKind != "external_out" {
		return preparedTransferOutReplacement{}, ErrInvestmentTransferNotFound
	}
	replacement := input.Replacement
	replacement.OwnerUserID, replacement.AuthSessionID, replacement.RequestID =
		input.OwnerUserID, input.AuthSessionID, input.RequestID
	replacement.ChangeReason, replacement.ReconciliationOverride = inversePlan.ChangeReason, input.ReconciliationOverride
	if replacement.CommodityID != operation.CommodityID || replacement.CostCommodityID != operation.CostCommodityID {
		return preparedTransferOutReplacement{}, ValidationError{Message: "a transfer replacement keeps the security and its basis currency"}
	}
	replacementParams, transfer, err := s.externalTransferOutWrite(ctx, replacement)
	if err != nil {
		return preparedTransferOutReplacement{}, err
	}
	inversePlan.AuthSessionID, inversePlan.RequestID = input.AuthSessionID, input.RequestID
	inversePlan.Operation = "investment.transfer.replace"
	inversePlan.ReconciliationOverride = input.ReconciliationOverride
	inversePlan.Spec.InvestmentOperationKind = ""
	inverse, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, inversePlan, nil)
	if err != nil {
		return preparedTransferOutReplacement{}, err
	}
	replacementParams.Operation = "investment.transfer.replace"
	replacementParams.CorrectionOfTransactionID = inverse.CorrectionOfTransactionID
	replacementParams.InvestmentCorrectionOfOperationID = operation.OperationID
	replacementParams.InvestmentCorrectionMode = "replace"
	replacementParams.InvestmentCorrectionReason = inversePlan.ChangeReason
	replacementParams.CreatedAt = inverse.CreatedAt
	// The compound command's first journal carries the disclosure policy.
	inverse.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	replacementParams.GainImpact = nil
	return preparedTransferOutReplacement{operation: operation, inverse: inverse,
		replacement: replacementParams, transfer: transfer}, nil
}

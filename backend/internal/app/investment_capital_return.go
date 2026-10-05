package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-146: a return of capital posts its cash on the payment date and reduces
// the basis of every lot open on the effective date, per share. Its receipt
// currency is the position's cost currency: no implicit FX. The reduction and
// any unresolved excess are writer outputs, never inputs.

var ErrCapitalReturnNoHoldings = errors.New("no holdings are open on the return of capital effective date")

type CapitalReturnInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	HoldingAccountID          int64
	CommodityID               int64
	CashAccountID             int64
	CurrencyID                int64
	EffectiveOn               string
	PaymentOn                 string
	AmountValue               exact.Coefficient
	AmountScale               int
	SourceEvidenceJSON        string
	Memo                      string
	ChangeReason              string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
	// EntitledLotIDs names the lots the corporate action entitles; each takes
	// its whole remaining quantity. Empty applies the per-share rule (T-148).
	EntitledLotIDs []int64
}

type CapitalReturnEffect struct {
	LotID                 int64
	EntitledQuantityValue exact.Coefficient
	EntitledQuantityScale int
	AllocatedValue        exact.Coefficient
	AllocatedScale        int
	ReductionValue        exact.Coefficient
	ReductionScale        int
	ExcessValue           exact.Coefficient
	ExcessScale           int
}

type CapitalReturnPreview struct {
	Effects []CapitalReturnEffect
	Impact  ReconciliationImpact
}

type CapitalReturnResult struct {
	Transaction Transaction
	Effects     []CapitalReturnEffect
}

func (s *InvestmentService) capitalReturnWrite(ctx context.Context, input CapitalReturnInput) (db.CreateTransactionParams, db.CreateCapitalReturnParams, error) {
	if input.OwnerUserID <= 0 {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, ValidationError{Message: "owner user is required"}
	}
	effective, err := cleanRequiredDate(input.EffectiveOn, "return of capital effective date")
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	payment, err := cleanRequiredDate(input.PaymentOn, "return of capital payment date")
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	if effective > payment {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, ValidationError{Message: "the effective date cannot follow the payment date"}
	}
	if input.AmountValue.Sign() <= 0 || input.AmountScale < 0 || input.AmountScale > 12 {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, ValidationError{Message: "return of capital amount must be positive at a valid money scale"}
	}
	if input.CurrencyID <= 0 || input.CommodityID <= 0 {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, ValidationError{Message: "security and currency are required"}
	}
	seen := make(map[int64]bool, len(input.EntitledLotIDs))
	for _, lotID := range input.EntitledLotIDs {
		if lotID <= 0 || seen[lotID] {
			return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, ValidationError{Message: "entitled lots must be distinct lot ids"}
		}
		seen[lotID] = true
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, effective, holdingRole, dependencies); err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	if _, err := s.accountInRole(ctx, input.CashAccountID, payment, settlementRole, dependencies); err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, effective, "security", false); err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	if err := s.requireCommodityKind(ctx, input.CurrencyID, payment, "receipt currency", true); err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, fmt.Errorf("resolve commodity trading account: %w", err)
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	create := CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OriginType: "browser_api", Operation: "investment.return_of_capital", ChangeReason: input.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "return_of_capital",
			TransactionDate: payment, Description: memo,
			JournalEntries: []JournalEntryInput{{EntryDate: payment, EntryKind: "investment", Memo: memo,
				Postings: []PostingInput{
					{AccountID: input.CashAccountID, CommodityID: input.CurrencyID,
						QuantityValue: input.AmountValue, QuantityScale: input.AmountScale, Memo: memo},
					{AccountID: tradingID, CommodityID: input.CurrencyID,
						QuantityValue: input.AmountValue.Negated(), QuantityScale: input.AmountScale, Memo: memo},
				}}},
		},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, create, dependencies)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateCapitalReturnParams{}, err
	}
	return journal, db.CreateCapitalReturnParams{
		BookID: BookID, AccountID: input.HoldingAccountID, CommodityID: input.CommodityID,
		CostCommodityID: input.CurrencyID, CashAccountID: input.CashAccountID,
		EffectiveOn: effective, PaymentOn: payment, AmountValue: input.AmountValue, AmountScale: input.AmountScale,
		SourceEvidenceJSON: evidence, EntitledLotIDs: input.EntitledLotIDs,
	}, nil
}

// PreviewCapitalReturn runs the complete writer and rolls back: the per-lot
// reduction and excess a commit would record, and its impact.
func (s *InvestmentService) PreviewCapitalReturn(ctx context.Context, input CapitalReturnInput) (CapitalReturnPreview, error) {
	input.ReconciliationOverride = true
	journal, params, err := s.capitalReturnWrite(ctx, input)
	if err != nil {
		return CapitalReturnPreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, result, err := s.repository.SimulateCapitalReturn(ctx, journal, params)
	if err != nil {
		return CapitalReturnPreview{}, mapCapitalReturnError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return CapitalReturnPreview{}, err
	}
	return CapitalReturnPreview{Effects: toCapitalReturnEffects(result.Effects), Impact: impact}, nil
}

// CapitalReturn records the receipt and the basis action under one audit
// event.
func (s *InvestmentService) CapitalReturn(ctx context.Context, input CapitalReturnInput) (CapitalReturnResult, error) {
	journal, params, err := s.capitalReturnWrite(ctx, input)
	if err != nil {
		return CapitalReturnResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, result, err := s.repository.CreateCapitalReturn(ctx, journal, params)
	if err != nil {
		return CapitalReturnResult{}, mapCapitalReturnError(err)
	}
	return CapitalReturnResult{Transaction: toTransaction(transaction), Effects: toCapitalReturnEffects(result.Effects)}, nil
}

func toCapitalReturnEffects(effects []db.CapitalReturnEffect) []CapitalReturnEffect {
	out := make([]CapitalReturnEffect, 0, len(effects))
	for _, effect := range effects {
		out = append(out, CapitalReturnEffect{LotID: effect.LotID,
			EntitledQuantityValue: effect.EntitledQuantityValue, EntitledQuantityScale: effect.EntitledQuantityScale,
			AllocatedValue: effect.AllocatedValue, AllocatedScale: effect.AllocatedScale,
			ReductionValue: effect.ReductionValue, ReductionScale: effect.ReductionScale,
			ExcessValue: effect.ExcessValue, ExcessScale: effect.ExcessScale})
	}
	return out
}

func mapCapitalReturnError(err error) error {
	switch {
	case errors.Is(err, db.ErrCapitalReturnNoHoldings):
		return ErrCapitalReturnNoHoldings
	case errors.Is(err, db.ErrCapitalReturnEntitlementUnavailable):
		return ValidationError{Message: "an entitled lot is not open in this holding on the effective date"}
	case errors.Is(err, db.ErrUnknownInvestmentBasis):
		return ValidationError{Message: "a holding with unresolved basis cannot take a return of capital yet"}
	case errors.Is(err, db.ErrOutOfOrderPositionEvent),
		errors.Is(err, db.ErrGainImpactAcknowledgementRequired),
		errors.Is(err, db.ErrGainImpactAcknowledgementStale):
		return err
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	default:
		return fmt.Errorf("return of capital: %w", mapTransactionDBError(err))
	}
}

var (
	ErrInvestmentCapitalReturnNotFound         = errors.New("return of capital operation not found")
	ErrInvestmentCapitalReturnAlreadyCorrected = errors.New("return of capital already corrected")
	ErrInvestmentCapitalReturnChanged          = errors.New("return of capital changed")
)

type ReverseInvestmentCapitalReturnInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

// ReverseCapitalReturn posts the exact inverse of the receipt and removes the
// basis action from effective history; the holding replays, so later
// disposals' gains change under the preview's acknowledgement (T-148).
func (s *InvestmentService) ReverseCapitalReturn(ctx context.Context, input ReverseInvestmentCapitalReturnInput) (Transaction, error) {
	operation, params, err := s.prepareCapitalReturnReversal(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	params.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	record, err := s.repository.ReverseCapitalReturn(ctx, params, operation)
	if err != nil {
		return Transaction{}, mapCapitalReturnCorrectionError(err)
	}
	return toTransaction(record), nil
}

func (s *InvestmentService) ReverseCapitalReturnReconciliationImpact(ctx context.Context, input ReverseInvestmentCapitalReturnInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	operation, params, err := s.prepareCapitalReturnReversal(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	params.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.PreviewCapitalReturnReversal(ctx, params, operation)
	if err != nil {
		return ReconciliationImpact{}, mapCapitalReturnCorrectionError(err)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) prepareCapitalReturnReversal(ctx context.Context, input ReverseInvestmentCapitalReturnInput) (db.CapitalReturnOperationRecord, db.CreateTransactionParams, error) {
	if input.OwnerUserID <= 0 || input.TransactionID <= 0 {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, ValidationError{Message: "owner and return of capital id are required"}
	}
	reason, err := cleanChangeReason(input.Reason, "")
	if err != nil || reason == "" {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, ValidationError{Message: "return of capital correction reason is required"}
	}
	operation, err := s.repository.CapitalReturnOperationByTransactionID(ctx, BookID, input.TransactionID)
	if err != nil {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, mapCapitalReturnCorrectionError(err)
	}
	if operation.AlreadyCorrected {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, ErrInvestmentCapitalReturnAlreadyCorrected
	}
	original, err := s.transactionService.Transaction(ctx, operation.TransactionID)
	if err != nil {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, err
	}
	if original.VersionID != operation.CurrentVersionID || original.Status != "posted" || original.DeletedAt != "" ||
		original.TransactionDate != operation.EventDate {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, ErrInvestmentCapitalReturnChanged
	}
	spec := invertedInvestmentTransactionSpec(original)
	spec.InvestmentOperationKind = "reversal"
	params, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		OriginType: "browser_api", Operation: "investment.return_of_capital.reverse", ChangeReason: reason,
		ReconciliationOverride: input.ReconciliationOverride, CorrectionOfTransactionID: &operation.TransactionID,
		Spec: spec,
	}, nil)
	if err != nil {
		return db.CapitalReturnOperationRecord{}, db.CreateTransactionParams{}, err
	}
	params.InvestmentCorrectionOfOperationID = operation.OperationID
	params.InvestmentCorrectionMode = "reverse"
	params.InvestmentCorrectionReason = reason
	return operation, params, nil
}

func mapCapitalReturnCorrectionError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrInvestmentCapitalReturnNotFound
	case errors.Is(err, db.ErrInvestmentOperationAlreadyCorrected):
		return ErrInvestmentCapitalReturnAlreadyCorrected
	case errors.Is(err, db.ErrInvestmentSaleChanged):
		return ErrInvestmentCapitalReturnChanged
	}
	return mapCapitalReturnError(err)
}

package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-147: cash in lieu of a split's fractional share, and since #181 of the
// fraction a share exchange, cash-and-stock merger or spin-off leaves in its
// destination holding. It is an ordinary long disposal of the fraction under
// the holding's election, with the cash as proceeds: the security legs post
// on the disposal date and the cash on the payment date. The realized result
// comes from replay at the disposal slot; it is never a dividend. The
// operation it settles is pinned and rechecked.

var (
	ErrCashInLieuSplitNotFound = errors.New("the split, exchange or spin-off this cash in lieu settles was not found or is no longer effective")
	ErrCashInLieuNotFraction   = errors.New("cash in lieu settles a fraction of one share")

	ErrInvestmentCashInLieuNotFound         = errors.New("cash in lieu operation not found")
	ErrInvestmentCashInLieuAlreadyCorrected = errors.New("cash in lieu already corrected")
	ErrInvestmentCashInLieuChanged          = errors.New("cash in lieu changed")

	// cashInLieuCorrectionFamily reverses and replaces a cash in lieu through
	// the shared disposal correction writers (T-150).
	cashInLieuCorrectionFamily = disposalCorrectionFamily{kind: "cash_in_lieu", noun: "cash in lieu",
		reverseOperation: "investment.cash_in_lieu.reverse", notFound: ErrInvestmentCashInLieuNotFound,
		alreadyCorrected: ErrInvestmentCashInLieuAlreadyCorrected, changed: ErrInvestmentCashInLieuChanged}
)

type CashInLieuInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	// SettledTransactionID is the posted split, share exchange, cash-and-stock
	// merger or spin-off whose fraction this disposes.
	SettledOperationID        int64
	SettledTransactionID      int64
	DisposalOn                string
	PaymentOn                 string
	QuantityValue             exact.Coefficient
	QuantityScale             int
	CashAccountID             int64
	CurrencyID                int64
	ProceedsValue             int64
	ProceedsScale             int
	CostBasisMethod           string
	LotAllocations            []InvestmentLotAllocationInput
	Memo                      string
	ChangeReason              string
	ReconciliationOverride    CheckpointOverride
	GainImpactAcknowledgement string
}

func (s *InvestmentService) cashInLieuTrade(ctx context.Context, input CashInLieuInput) (InvestmentTradeInput, int64, string, error) {
	if (input.SettledTransactionID <= 0) == (input.SettledOperationID <= 0) {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "the operation this cash in lieu settles is required"}
	}
	disposal, err := cleanRequiredDate(input.DisposalOn, "disposal date")
	if err != nil {
		return InvestmentTradeInput{}, 0, "", err
	}
	payment, err := cleanRequiredDate(input.PaymentOn, "payment date")
	if err != nil {
		return InvestmentTradeInput{}, 0, "", err
	}
	if payment < disposal {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "the payment date cannot precede the disposal date"}
	}
	if input.QuantityScale < 0 || input.QuantityScale > 24 || input.ProceedsScale < 0 || input.ProceedsScale > 12 {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "cash in lieu scales exceed the supported range"}
	}
	quantity := exact.ScaledIntFromCoefficient(input.QuantityValue, input.QuantityScale)
	if quantity.Sign() <= 0 || quantity.Cmp(exact.ScaledIntFromInt64(1, 0)) >= 0 {
		return InvestmentTradeInput{}, 0, "", ErrCashInLieuNotFraction
	}
	if input.ProceedsValue <= 0 {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "cash in lieu amount must be positive"}
	}
	var settled db.CashInLieuSettledRecord
	if input.SettledOperationID > 0 {
		settled, err = s.repository.CashInLieuSettledOperationByID(ctx, BookID, input.SettledOperationID)
	} else {
		settled, err = s.repository.CashInLieuSettledOperation(ctx, BookID, input.SettledTransactionID)
	}
	if errors.Is(err, db.ErrNotFound) {
		return InvestmentTradeInput{}, 0, "", ErrCashInLieuSplitNotFound
	}
	if err != nil {
		return InvestmentTradeInput{}, 0, "", fmt.Errorf("read operation for cash in lieu: %w", err)
	}
	if settled.AlreadyCorrected {
		return InvestmentTradeInput{}, 0, "", ErrCashInLieuSplitNotFound
	}
	if disposal < settled.EventDate {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "cash in lieu cannot be disposed before the operation it settles"}
	}
	return InvestmentTradeInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionDate: disposal, SettlementDate: payment, CommodityID: settled.CommodityID,
		HoldingAccountID: settled.AccountID, CashAccountID: input.CashAccountID, CashCommodityID: input.CurrencyID,
		QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale,
		CashAmountValue: input.ProceedsValue, CashAmountScale: input.ProceedsScale,
		CostBasisMethod: input.CostBasisMethod, LotAllocations: input.LotAllocations,
		Memo: input.Memo, ChangeReason: input.ChangeReason, ReconciliationOverride: input.ReconciliationOverride,
		GainImpactAcknowledgement: input.GainImpactAcknowledgement,
		Operation:                 "investment.cash_in_lieu", CashInLieu: true,
	}, settled.OperationID, payment, nil
}

// PreviewCashInLieu runs the disposal writer and rolls back: the lots the
// fraction takes, its basis and the impact.
type CashInLieuPreviewResult struct {
	InvestmentTradeResult
	RealizedGainValue exact.Coefficient
	RealizedGainScale int
}

func cashInLieuPreviewResult(allocations []db.LotDisposalRecord, record db.DisposalDecisionRecord) (CashInLieuPreviewResult, error) {
	decision := toDisposalDecision(record)
	gain := exact.ScaledIntFromInt64(decision.ProceedsValue, decision.ProceedsScale)
	if decision.BasisKnowledge == db.InvestmentBasisUnknown {
		return CashInLieuPreviewResult{InvestmentTradeResult: InvestmentTradeResult{Allocations: toInvestmentLotDisposals(allocations), DisposalDecision: &decision}}, nil
	}
	gain.SubScaled(exact.ScaledIntFromCoefficient(decision.DisposedBasisValue, decision.DisposedBasisScale))
	value, err := gain.Coefficient()
	if err != nil {
		return CashInLieuPreviewResult{}, LedgerOverflowError{CommodityID: decision.CostCommodityID}
	}
	return CashInLieuPreviewResult{InvestmentTradeResult: InvestmentTradeResult{Allocations: toInvestmentLotDisposals(allocations), DisposalDecision: &decision}, RealizedGainValue: value, RealizedGainScale: gain.Scale()}, nil
}

func (s *InvestmentService) PreviewCashInLieu(ctx context.Context, input CashInLieuInput) (CashInLieuPreviewResult, ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	trade, settledOperationID, payment, err := s.cashInLieuTrade(ctx, input)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, err
	}
	journal, disposal, err := s.prepareSellWrite(ctx, trade)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, allocations, decision, err := s.repository.SimulateCashInLieu(ctx, journal, disposal, settledOperationID, payment)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, mapCashInLieuPreviewError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, err
	}
	result, err := cashInLieuPreviewResult(allocations, decision)
	return result, impact, err
}

func mapCashInLieuPreviewError(err error) error {
	if errors.Is(err, db.ErrCashInLieuSplitUnavailable) {
		return ErrCashInLieuSplitNotFound
	}
	return cashInLieuCorrectionFamily.mapError(mapDisposalWriteError(err, false))
}

// CashInLieu records the fraction's disposal and its cash under one audit
// event, linked to the operation it settles.
func (s *InvestmentService) CashInLieu(ctx context.Context, input CashInLieuInput) (InvestmentTradeResult, error) {
	trade, settledOperationID, payment, err := s.cashInLieuTrade(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	result, err := s.sell(ctx, trade, db.CashInLieuFactWriter(ctx, BookID, settledOperationID, payment))
	if errors.Is(err, db.ErrCashInLieuSplitUnavailable) {
		return InvestmentTradeResult{}, ErrCashInLieuSplitNotFound
	}
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	result.Transaction, err = s.transactionService.enrichOne(ctx, result.Transaction)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	return result, nil
}

// ReverseCashInLieu posts the inverse of a cash in lieu's journal and removes
// its disposal from effective history; the holding replays (T-150).
func (s *InvestmentService) ReverseCashInLieu(ctx context.Context, input ReverseInvestmentSaleInput) (Transaction, error) {
	return s.reverseDisposal(ctx, input, cashInLieuCorrectionFamily)
}

func (s *InvestmentService) ReverseCashInLieuReconciliationImpact(ctx context.Context, input ReverseInvestmentSaleInput) (ReconciliationImpact, error) {
	return s.reverseDisposalReconciliationImpact(ctx, input, cashInLieuCorrectionFamily)
}

// ReplaceCashInLieuInput corrects a cash in lieu. Replacement carries the
// corrected disposal and payment; the operation it settles stays the same.
type ReplaceCashInLieuInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    CheckpointOverride
	GainImpactAcknowledgement string
	Replacement               CashInLieuInput
}

func (s *InvestmentService) cashInLieuReplacement(ctx context.Context, input ReplaceCashInLieuInput) (ReplaceInvestmentSaleInput, int64, string, error) {
	settledID, settledTransactionID, err := s.repository.CashInLieuSettledIDs(ctx, BookID, input.TransactionID)
	if errors.Is(err, db.ErrNotFound) {
		return ReplaceInvestmentSaleInput{}, 0, "", ErrInvestmentCashInLieuNotFound
	}
	if err != nil {
		return ReplaceInvestmentSaleInput{}, 0, "", err
	}
	replacement := input.Replacement
	if replacement.SettledTransactionID != 0 && replacement.SettledTransactionID != settledTransactionID || replacement.SettledOperationID != 0 && replacement.SettledOperationID != settledID {
		return ReplaceInvestmentSaleInput{}, 0, "", ValidationError{Message: "a cash in lieu replacement must keep the operation it settles"}
	}
	replacement.OwnerUserID, replacement.SettledTransactionID, replacement.SettledOperationID = input.OwnerUserID, 0, settledID
	trade, settledOperationID, payment, err := s.cashInLieuTrade(ctx, replacement)
	if err != nil {
		return ReplaceInvestmentSaleInput{}, 0, "", err
	}
	// A replacement names its election; default to the holding's own.
	if trade.CostBasisMethod == "" {
		if trade.CostBasisMethod, _, err = s.resolveCostBasisMethod(ctx, trade.HoldingAccountID, ""); err != nil {
			return ReplaceInvestmentSaleInput{}, 0, "", err
		}
	}
	return ReplaceInvestmentSaleInput{OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, TransactionID: input.TransactionID, Reason: input.Reason,
		ReconciliationOverride: input.ReconciliationOverride, Replacement: trade,
		GainImpactAcknowledgement: input.GainImpactAcknowledgement}, settledOperationID, payment, nil
}

// ReplaceCashInLieu posts the inverse and the corrected disposal at the
// replaced slot under one audit event, linking the successor to the same
// settled operation.
func (s *InvestmentService) ReplaceCashInLieu(ctx context.Context, input ReplaceCashInLieuInput) (ReplaceInvestmentSaleResult, error) {
	sale, settledOperationID, payment, err := s.cashInLieuReplacement(ctx, input)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	result, err := s.replaceDisposal(ctx, sale, "browser_api", "investment.cash_in_lieu.replace",
		db.CashInLieuReplacementFactWriter(ctx, BookID, settledOperationID, payment), cashInLieuCorrectionFamily)
	if errors.Is(err, db.ErrCashInLieuSplitUnavailable) {
		return ReplaceInvestmentSaleResult{}, ErrCashInLieuSplitNotFound
	}
	return result, err
}

func (s *InvestmentService) ReplaceCashInLieuReconciliationImpact(ctx context.Context, input ReplaceCashInLieuInput) (ReconciliationImpact, error) {
	_, impact, err := s.PreviewCashInLieuReplacement(ctx, input)
	return impact, err
}

func (s *InvestmentService) PreviewCashInLieuReplacement(ctx context.Context, input ReplaceCashInLieuInput) (CashInLieuPreviewResult, ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	sale, settledOperationID, payment, err := s.cashInLieuReplacement(ctx, input)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, err
	}
	prepared, err := s.prepareSaleReplacementWrite(ctx, sale, "browser_api", "investment.cash_in_lieu.replace", cashInLieuCorrectionFamily)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, err
	}
	simulated, allocations, decision, err := s.repository.SimulateCashInLieuReplacement(ctx, prepared.Operation, prepared.Inverse, prepared.Replacement, prepared.Disposal, settledOperationID, payment)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, mapCashInLieuPreviewError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return CashInLieuPreviewResult{}, ReconciliationImpact{}, err
	}
	result, err := cashInLieuPreviewResult(allocations, decision)
	return result, impact, err
}

func (s *InvestmentService) CashInLieuAvailableLots(ctx context.Context, ownerUserID, settledTransactionID, currencyID int64, disposalOn string, replacingTransactionID int64) ([]db.InvestmentTradeCorrectionAvailableLot, error) {
	return s.cashInLieuAvailableLots(ctx, ownerUserID, settledTransactionID, 0, currencyID, disposalOn, replacingTransactionID)
}

func (s *InvestmentService) CashInLieuAvailableLotsForOperation(ctx context.Context, ownerUserID, operationID, currencyID int64, disposalOn string, replacingTransactionID int64) ([]db.InvestmentTradeCorrectionAvailableLot, error) {
	return s.cashInLieuAvailableLots(ctx, ownerUserID, 0, operationID, currencyID, disposalOn, replacingTransactionID)
}

func (s *InvestmentService) cashInLieuAvailableLots(ctx context.Context, ownerUserID, settledTransactionID, operationID, currencyID int64, disposalOn string, replacingTransactionID int64) ([]db.InvestmentTradeCorrectionAvailableLot, error) {
	if ownerUserID <= 0 || currencyID <= 0 {
		return nil, ValidationError{Message: "owner and currency are required"}
	}
	date, err := cleanRequiredDate(disposalOn, "disposal date")
	if err != nil {
		return nil, err
	}
	var settled db.CashInLieuSettledRecord
	if operationID > 0 {
		settled, err = s.repository.CashInLieuSettledOperationByID(ctx, BookID, operationID)
	} else {
		settled, err = s.repository.CashInLieuSettledOperation(ctx, BookID, settledTransactionID)
	}
	if errors.Is(err, db.ErrNotFound) || settled.AlreadyCorrected {
		return nil, ErrCashInLieuSplitNotFound
	}
	if err != nil {
		return nil, err
	}
	if date < settled.EventDate {
		return nil, ValidationError{Message: "cash in lieu cannot be disposed before the operation it settles"}
	}
	lots, err := s.repository.CashInLieuAvailableLots(ctx, BookID, settled, currencyID, date, replacingTransactionID)
	if err != nil {
		return nil, mapCashInLieuPreviewError(err)
	}
	return lots, nil
}

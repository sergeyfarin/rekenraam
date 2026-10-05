package app

import (
	"context"
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-147: cash in lieu of a split's fractional share. It is an ordinary long
// disposal of the fraction under the holding's election, with the cash as
// proceeds: the security legs post on the disposal date and the cash on the
// payment date. The realized result comes from replay at the disposal slot;
// it is never a dividend. The split it settles is pinned and rechecked.

var (
	ErrCashInLieuSplitNotFound = errors.New("the split this cash in lieu settles was not found or is no longer effective")
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
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	SplitTransactionID        int64
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
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
}

func (s *InvestmentService) cashInLieuTrade(ctx context.Context, input CashInLieuInput) (InvestmentTradeInput, int64, string, error) {
	if input.SplitTransactionID <= 0 {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "the split this cash in lieu settles is required"}
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
	quantity := exact.ScaledIntFromCoefficient(input.QuantityValue, input.QuantityScale)
	if quantity.Sign() <= 0 || quantity.Cmp(exact.ScaledIntFromInt64(1, 0)) >= 0 {
		return InvestmentTradeInput{}, 0, "", ErrCashInLieuNotFraction
	}
	if input.ProceedsValue <= 0 {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "cash in lieu amount must be positive"}
	}
	split, err := s.repository.SplitOperationByTransactionID(ctx, BookID, input.SplitTransactionID)
	if errors.Is(err, db.ErrNotFound) {
		return InvestmentTradeInput{}, 0, "", ErrCashInLieuSplitNotFound
	}
	if err != nil {
		return InvestmentTradeInput{}, 0, "", fmt.Errorf("read split for cash in lieu: %w", err)
	}
	if split.AlreadyCorrected {
		return InvestmentTradeInput{}, 0, "", ErrCashInLieuSplitNotFound
	}
	if disposal < split.EventDate {
		return InvestmentTradeInput{}, 0, "", ValidationError{Message: "cash in lieu cannot be disposed before its split"}
	}
	return InvestmentTradeInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionDate: disposal, SettlementDate: payment, CommodityID: split.CommodityID,
		HoldingAccountID: split.AccountID, CashAccountID: input.CashAccountID, CashCommodityID: input.CurrencyID,
		QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale,
		CashAmountValue: input.ProceedsValue, CashAmountScale: input.ProceedsScale,
		CostBasisMethod: input.CostBasisMethod, LotAllocations: input.LotAllocations,
		Memo: input.Memo, ChangeReason: input.ChangeReason, ReconciliationOverride: input.ReconciliationOverride,
		GainImpactAcknowledgement: input.GainImpactAcknowledgement,
		Operation:                 "investment.cash_in_lieu", CashInLieu: true,
	}, split.OperationID, payment, nil
}

// PreviewCashInLieu runs the disposal writer and rolls back: the lots the
// fraction takes, its basis and the impact.
func (s *InvestmentService) PreviewCashInLieu(ctx context.Context, input CashInLieuInput) (InvestmentTradeResult, ReconciliationImpact, error) {
	trade, _, _, err := s.cashInLieuTrade(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, ReconciliationImpact{}, err
	}
	simulated, disposals, err := s.simulateDisposal(ctx, trade)
	if err != nil {
		return InvestmentTradeResult{}, ReconciliationImpact{}, err
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return InvestmentTradeResult{}, ReconciliationImpact{}, err
	}
	return InvestmentTradeResult{Allocations: toInvestmentLotDisposals(disposals)}, impact, nil
}

// CashInLieu records the fraction's disposal and its cash under one audit
// event, linked to the split it settles.
func (s *InvestmentService) CashInLieu(ctx context.Context, input CashInLieuInput) (InvestmentTradeResult, error) {
	trade, splitOperationID, payment, err := s.cashInLieuTrade(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	result, err := s.sell(ctx, trade, db.CashInLieuFactWriter(ctx, BookID, splitOperationID, payment))
	if errors.Is(err, db.ErrCashInLieuSplitUnavailable) {
		return InvestmentTradeResult{}, ErrCashInLieuSplitNotFound
	}
	return result, err
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
// corrected disposal and payment; the split it settles stays the same.
type ReplaceCashInLieuInput struct {
	OwnerUserID               int64
	AuthSessionID             int64
	RequestID                 string
	TransactionID             int64
	Reason                    string
	ReconciliationOverride    bool
	GainImpactAcknowledgement string
	Replacement               CashInLieuInput
}

func (s *InvestmentService) cashInLieuReplacement(ctx context.Context, input ReplaceCashInLieuInput) (ReplaceInvestmentSaleInput, int64, string, error) {
	splitTransactionID, err := s.repository.CashInLieuSplitTransactionID(ctx, BookID, input.TransactionID)
	if errors.Is(err, db.ErrNotFound) {
		return ReplaceInvestmentSaleInput{}, 0, "", ErrInvestmentCashInLieuNotFound
	}
	if err != nil {
		return ReplaceInvestmentSaleInput{}, 0, "", err
	}
	replacement := input.Replacement
	replacement.OwnerUserID, replacement.SplitTransactionID = input.OwnerUserID, splitTransactionID
	trade, splitOperationID, payment, err := s.cashInLieuTrade(ctx, replacement)
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
		GainImpactAcknowledgement: input.GainImpactAcknowledgement}, splitOperationID, payment, nil
}

// ReplaceCashInLieu posts the inverse and the corrected disposal at the
// replaced slot under one audit event, linking the successor to the same
// split.
func (s *InvestmentService) ReplaceCashInLieu(ctx context.Context, input ReplaceCashInLieuInput) (ReplaceInvestmentSaleResult, error) {
	sale, splitOperationID, payment, err := s.cashInLieuReplacement(ctx, input)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	result, err := s.replaceDisposal(ctx, sale, "browser_api", "investment.cash_in_lieu.replace",
		db.CashInLieuReplacementFactWriter(ctx, BookID, splitOperationID, payment), cashInLieuCorrectionFamily)
	if errors.Is(err, db.ErrCashInLieuSplitUnavailable) {
		return ReplaceInvestmentSaleResult{}, ErrCashInLieuSplitNotFound
	}
	return result, err
}

func (s *InvestmentService) ReplaceCashInLieuReconciliationImpact(ctx context.Context, input ReplaceCashInLieuInput) (ReconciliationImpact, error) {
	sale, _, _, err := s.cashInLieuReplacement(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.replaceDisposalReconciliationImpact(ctx, sale, "investment.cash_in_lieu.replace", cashInLieuCorrectionFamily)
}

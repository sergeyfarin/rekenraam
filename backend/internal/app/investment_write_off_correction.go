package app

import (
	"context"
)

// T-118: a write-off is a zero-proceeds disposal operation. It is corrected
// by its own commands, never by sale correction, but shares the disposal
// reversal and replacement writers: the original journal, decision and lot
// events stay immutable evidence, dependent disposals replay under their
// recorded elections, changed gains need the exact acknowledgement and a
// reconciled balance needs the explicit override.

// ReverseWriteOff posts the exact inverse of a write-off and restores its
// units to the effective position, replaying later disposals.
func (s *InvestmentService) ReverseWriteOff(ctx context.Context, input ReverseInvestmentSaleInput) (Transaction, error) {
	return s.reverseDisposal(ctx, input, writeOffCorrectionFamily)
}

// ReverseWriteOffReconciliationImpact runs the reversal writer and rolls back.
func (s *InvestmentService) ReverseWriteOffReconciliationImpact(ctx context.Context, input ReverseInvestmentSaleInput) (ReconciliationImpact, error) {
	return s.reverseDisposalReconciliationImpact(ctx, input, writeOffCorrectionFamily)
}

type ReplaceInvestmentWriteOffInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionID          int64
	Reason                 string
	ReconciliationOverride bool
	// Replacement is the full corrected write-off. Its own Reason is why the
	// holding is worthless; the correction Reason is why the record changed.
	Replacement               InvestmentWriteOffInput
	GainImpactAcknowledgement string
}

// ReplaceWriteOff posts the inverse and a corrected zero-proceeds write-off
// under one audit event. Quantity, elections, date and holding may change.
func (s *InvestmentService) ReplaceWriteOff(ctx context.Context, input ReplaceInvestmentWriteOffInput) (ReplaceInvestmentSaleResult, error) {
	sale, err := writeOffReplacementAsSale(input)
	if err != nil {
		return ReplaceInvestmentSaleResult{}, err
	}
	return s.replaceDisposal(ctx, sale, "browser_api", "investment.write_off.replace", nil, writeOffCorrectionFamily)
}

// ReplaceWriteOffReconciliationImpact runs the replacement writer and rolls back.
func (s *InvestmentService) ReplaceWriteOffReconciliationImpact(ctx context.Context, input ReplaceInvestmentWriteOffInput) (ReconciliationImpact, error) {
	sale, err := writeOffReplacementAsSale(input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.replaceDisposalReconciliationImpact(ctx, sale, "investment.write_off.replace", writeOffCorrectionFamily)
}

func writeOffReplacementAsSale(input ReplaceInvestmentWriteOffInput) (ReplaceInvestmentSaleInput, error) {
	replacement := input.Replacement
	replacement.OwnerUserID = input.OwnerUserID
	reason, err := validateWriteOffInput(replacement)
	if err != nil {
		return ReplaceInvestmentSaleInput{}, err
	}
	trade := replacement.asTradeInput()
	trade.Memo = defaultString(replacement.Memo, reason)
	trade.PayeeID = replacement.PayeeID
	return ReplaceInvestmentSaleInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
		TransactionID: input.TransactionID, Reason: input.Reason,
		ReconciliationOverride: input.ReconciliationOverride, Replacement: trade,
		GainImpactAcknowledgement: input.GainImpactAcknowledgement,
	}, nil
}

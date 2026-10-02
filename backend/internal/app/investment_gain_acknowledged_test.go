package app

import "context"

// These helpers act like a client that reviews a command's preview and then
// accepts exactly the gain changes it disclosed (T-126). Tests about other
// behavior use them; the named gain-impact tests call commands directly so
// missing and stale acknowledgements stay pinned. A preview error leaves the
// acknowledgement empty and lets the command report its own refusal.

func acknowledgedReverseSale(ctx context.Context, s *InvestmentService, input ReverseInvestmentSaleInput) (Transaction, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReverseSaleReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReverseSale(ctx, input)
}

func acknowledgedReverseBuy(ctx context.Context, s *InvestmentService, input ReverseInvestmentBuyInput) (Transaction, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReverseBuyReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReverseBuy(ctx, input)
}

func acknowledgedReplaceSale(ctx context.Context, s *InvestmentService, input ReplaceInvestmentSaleInput) (ReplaceInvestmentSaleResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReplaceSaleReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReplaceSale(ctx, input)
}

func acknowledgedReplaceBuy(ctx context.Context, s *InvestmentService, input ReplaceInvestmentBuyInput) (ReplaceInvestmentBuyResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReplaceBuyReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReplaceBuy(ctx, input)
}

func acknowledgedReinvestedDividend(ctx context.Context, s *InvestmentService, input ReinvestedDividendInput) (InvestmentTradeResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.ReinvestedDividendReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.ReinvestedDividend(ctx, input)
}

func acknowledgedCorrectTrading212Buy(ctx context.Context, s *ImportService, input CorrectTrading212BuyInput) (ReplaceInvestmentBuyResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.Trading212BuyCorrectionReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.CorrectTrading212Buy(ctx, input)
}

func acknowledgedCorrectTrading212Sale(ctx context.Context, s *ImportService, input CorrectTrading212SaleInput) (ReplaceInvestmentSaleResult, error) {
	if input.GainImpactAcknowledgement == "" {
		if impact, err := s.Trading212SaleCorrectionReconciliationImpact(ctx, input); err == nil && impact.GainImpact != nil {
			input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
		}
	}
	return s.CorrectTrading212Sale(ctx, input)
}

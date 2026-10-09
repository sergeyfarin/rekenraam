package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ErrInvestmentPositionSideConflict refuses a lot opening that would make a
// holding long and short of one instrument over overlapping dates (#173). It
// is the repository's sentinel under this package's name, so the explanation
// naming the conflicting side and date reaches the user intact.
var ErrInvestmentPositionSideConflict = db.ErrPositionSideConflict

// A named short sale borrows units and sells them; a cover buys them back and
// closes short lots (ADR 0013, operation plan *Short positions (#103)*). The
// journal is the familiar sale or buy: an opening posts H −q and the sale's
// cash legs, a cover posts H +q and the buy's. What makes them shorts is the
// subledger: an opening creates a short lot whose basis columns hold the
// exact opening proceeds, and a cover consumes only short lots, recording the
// allocated opening proceeds as its disposed amount and its signed clearing
// amount as proceeds. Their sum is the result: 100.00 opened, 70.00 covered,
// +30.00. Entry is in date order only until correction and replay land (#175).

// ShortSale opens a named short position.
func (s *InvestmentService) ShortSale(ctx context.Context, input InvestmentTradeInput) (InvestmentTradeResult, error) {
	transactionParams, lotParams, err := s.prepareShortSaleWrite(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	// Nothing replays behind a short entry yet, so the change set is always
	// empty; the shared policy still refuses a stale echoed token.
	transactionParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transactionRecord, lot, err := s.repository.CreateTransactionAndLot(ctx, transactionParams, lotParams)
	if err != nil {
		return InvestmentTradeResult{}, shortDependency(mapInvestmentOpeningWriteError(err, "short sale"))
	}
	return InvestmentTradeResult{Transaction: toTransaction(transactionRecord), LotID: &lot.ID}, nil
}

// ShortCover buys back borrowed units and closes short lots.
func (s *InvestmentService) ShortCover(ctx context.Context, input InvestmentTradeInput) (InvestmentTradeResult, error) {
	transactionParams, disposalParams, err := s.prepareShortCoverWrite(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	transactionParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transactionRecord, disposals, decision, err := s.repository.CreateTransactionAndDisposeLotsWithDecision(ctx, transactionParams, disposalParams)
	if err != nil {
		return InvestmentTradeResult{}, mapShortCoverWriteError(err)
	}
	committed := toDisposalDecision(decision)
	return InvestmentTradeResult{Transaction: toTransaction(transactionRecord), Allocations: toInvestmentLotDisposals(disposals), DisposalDecision: &committed}, nil
}

// PreviewShortCover runs the complete cover writer and rolls back, returning
// the lots it would consume and the result it would realize.
func (s *InvestmentService) PreviewShortCover(ctx context.Context, input InvestmentTradeInput) (SellPreviewResult, error) {
	_, disposals, decision, economics, err := s.simulateShortCover(ctx, input)
	if err != nil {
		return SellPreviewResult{}, err
	}
	gain, err := shortCoverResult(decision)
	if err != nil {
		return SellPreviewResult{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
	}
	gainValue, err := gain.Int64()
	if err != nil {
		return SellPreviewResult{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
	}
	return SellPreviewResult{
		CostBasisMethod:    decision.CostBasisMethod,
		DisposalDecision:   toDisposalDecision(decision),
		Allocations:        toInvestmentLotDisposals(disposals),
		RealizedGain:       gainValue,
		RealizedGainScale:  gain.Scale(),
		BasisKnowledge:     db.InvestmentBasisKnown,
		CashAmountValue:    input.CashAmountValue,
		CashAmountScale:    input.CashAmountScale,
		GrossAmountValue:   input.GrossAmountValue,
		GrossAmountScale:   input.GrossAmountScale,
		NetSettlementValue: economics.NetValue,
		NetSettlementScale: economics.NetScale,
		SettlementDate:     economics.SettlementDate,
		Charges:            input.Charges,
	}, nil
}

// shortCoverResult is allocated opening proceeds plus the signed (negative)
// covering amount, added at whichever scale is deeper (T-101).
func shortCoverResult(decision db.DisposalDecisionRecord) (*exact.ScaledInt, error) {
	if decision.BasisKnowledge != "" && decision.BasisKnowledge != db.InvestmentBasisKnown {
		return nil, errors.New("short cover with unknown opening proceeds")
	}
	result := exact.ScaledIntFromInt64(decision.ProceedsValue, decision.ProceedsScale)
	result.AddScaled(exact.ScaledIntFromCoefficient(decision.DisposedBasisValue, decision.DisposedBasisScale))
	return result, nil
}

func (s *InvestmentService) simulateShortCover(ctx context.Context, input InvestmentTradeInput) (db.SimulatedInvestmentWrite, []db.LotDisposalRecord, db.DisposalDecisionRecord, tradeEconomics, error) {
	input.ReconciliationOverride = true
	plan, err := s.shortPlan(ctx, input, false)
	if err != nil {
		return db.SimulatedInvestmentWrite{}, nil, db.DisposalDecisionRecord{}, tradeEconomics{}, err
	}
	transactionParams, disposalParams, err := s.shortCoverWriteFromPlan(ctx, input, plan)
	if err != nil {
		return db.SimulatedInvestmentWrite{}, nil, db.DisposalDecisionRecord{}, tradeEconomics{}, err
	}
	transactionParams.GainImpact = gainImpactPolicy("")
	simulated, disposals, decision, err := s.repository.SimulateTransactionAndDisposeLots(ctx, transactionParams, disposalParams)
	if err != nil {
		return db.SimulatedInvestmentWrite{}, nil, db.DisposalDecisionRecord{}, tradeEconomics{}, mapShortCoverWriteError(err)
	}
	return simulated, disposals, decision, *plan.TradeEconomics, nil
}

func mapShortCoverWriteError(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		// A specific_lot election naming a long, closed or foreign lot.
		return ValidationError{Message: "short cover lot allocations must name open short lots of this holding"}
	}
	return shortDependency(mapDisposalWriteError(err, false))
}

// shortPlan builds the journal a short opening (sale economics) or cover
// (buy economics) posts, with the operation kind and attribution of each.
func (s *InvestmentService) shortPlan(ctx context.Context, input InvestmentTradeInput, opening bool) (investmentTransactionPlan, error) {
	input.WriteOff, input.CashInLieu = false, false
	if err := validateTradeInput(input); err != nil {
		return investmentTransactionPlan{}, err
	}
	if opening && (len(input.LotAllocations) > 0 || strings.TrimSpace(input.CostBasisMethod) != "") {
		return investmentTransactionPlan{}, ValidationError{Message: "a short sale opens a lot and takes no cost basis method or lot allocations"}
	}
	dependencies := newAccountRuleDependencies()
	if err := s.validateTradeRoles(ctx, input, dependencies); err != nil {
		return investmentTransactionPlan{}, err
	}
	// A cover pays like a buy; an opening receives like a sale.
	economics, err := s.calculateTradeEconomics(ctx, input, !opening, dependencies)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	if opening && economics.ClearingValue <= 0 {
		return investmentTransactionPlan{}, ValidationError{Message: "short sale charges leave no positive opening proceeds"}
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return investmentTransactionPlan{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	kind, operation := "short_cover", "investment.short_cover"
	if opening {
		kind, operation = "short_sale", "investment.short_sale"
	}
	return investmentTransactionPlan{
		TradeEconomics:          &economics,
		AccountRuleDependencies: dependencies,
		MetadataJSON:            metadataJSON,
		Date:                    input.TransactionDate,
		Create: CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             defaultString(input.OriginType, "browser_api"),
			Operation:              defaultString(input.Operation, operation),
			ChangeReason:           input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride,
			Spec: TransactionInput{
				Status:                  defaultString(strings.TrimSpace(input.Status), "posted"),
				TransactionKind:         "investment",
				InvestmentOperationKind: kind,
				TransactionDate:         input.TransactionDate,
				PayeeID:                 input.PayeeID,
				Description:             memo,
				ExternalRefHint:         input.ExternalRefHint,
				MetadataJSON:            metadataJSON,
				JournalEntries:          tradeJournalEntries(input, tradingAccountID, memo, economics, !opening),
			},
		},
	}, nil
}

// prepareShortTransaction freezes the journal, source components and implied
// price shared by both short commands.
func (s *InvestmentService) prepareShortTransaction(ctx context.Context, input InvestmentTradeInput, plan investmentTransactionPlan) (db.CreateTransactionParams, error) {
	transactionParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	transactionParams.InvestmentComponents = plan.TradeEconomics.components(input.CashCommodityID)
	transactionParams.InvestmentSettlementDate = plan.TradeEconomics.SettlementDate
	if s.pricingService != nil && !plan.TradeEconomics.PriceUnavailable {
		transactionParams.TradeImpliedPrice, err = tradePriceSpec(input.CommodityID, input.CashCommodityID,
			input.TransactionDate, input.QuantityValue, input.QuantityScale,
			plan.TradeEconomics.PriceValue, plan.TradeEconomics.PriceScale, plan.TradeEconomics.PriceApproximate)
		if err != nil {
			return db.CreateTransactionParams{}, err
		}
	}
	return transactionParams, nil
}

func (s *InvestmentService) prepareShortSaleWrite(ctx context.Context, input InvestmentTradeInput) (db.CreateTransactionParams, db.CreateInvestmentLotParams, error) {
	plan, err := s.shortPlan(ctx, input, true)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
	}
	transactionParams, err := s.prepareShortTransaction(ctx, input, plan)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
	}
	return transactionParams, db.CreateInvestmentLotParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		OpenedOn:        input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		CostBasisValue:  plan.TradeEconomics.ClearingValue,
		CostBasisScale:  plan.TradeEconomics.ClearingScale,
		CostCommodityID: input.CashCommodityID,
		PositionSide:    db.PositionSideShort,
		MetadataJSON:    plan.MetadataJSON,
		CreatedAt:       s.now().UTC().Format(time.RFC3339),
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      defaultString(input.OriginType, "browser_api"),
		Operation:       "investment.lot.create",
		ChangeReason:    "opened short lot from short sale",
		EventKind:       "acquisition",
	}, nil
}

func (s *InvestmentService) prepareShortCoverWrite(ctx context.Context, input InvestmentTradeInput) (db.CreateTransactionParams, db.DisposeLotsParams, error) {
	plan, err := s.shortPlan(ctx, input, false)
	if err != nil {
		return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
	}
	return s.shortCoverWriteFromPlan(ctx, input, plan)
}

func (s *InvestmentService) shortCoverWriteFromPlan(ctx context.Context, input InvestmentTradeInput, plan investmentTransactionPlan) (db.CreateTransactionParams, db.DisposeLotsParams, error) {
	transactionParams, err := s.prepareShortTransaction(ctx, input, plan)
	if err != nil {
		return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
	}
	method, source, err := s.resolveCostBasisMethod(ctx, input.HoldingAccountID, input.CostBasisMethod)
	if err != nil {
		return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
	}
	allocations := make([]db.LotAllocation, 0, len(input.LotAllocations))
	for _, allocation := range input.LotAllocations {
		allocations = append(allocations, db.LotAllocation{LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	return transactionParams, db.DisposeLotsParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		CostCommodityID: input.CashCommodityID,
		// Signed: the covering cost leaves as a negative clearing amount.
		ProceedsValue:   plan.TradeEconomics.ClearingValue,
		ProceedsScale:   plan.TradeEconomics.ClearingScale,
		EventDate:       input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		Allocations:     allocations,
		CostBasisMethod: method,
		DecisionSource:  source,
		PositionSide:    db.PositionSideShort,
		CreatedAt:       s.now().UTC().Format(time.RFC3339),
		ActorUserID:     input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      defaultString(input.OriginType, "browser_api"),
		Operation:       "investment.lot.dispose",
		ChangeReason:    "covered short lots",
		MetadataJSON:    plan.MetadataJSON,
	}, nil
}

// shortSaleReconciliationImpact runs the opening writer and rolls back.
func (s *InvestmentService) shortSaleReconciliationImpact(ctx context.Context, input InvestmentTradeInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = true
	transactionParams, lotParams, err := s.prepareShortSaleWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	impact, err := s.openingReconciliationImpact(ctx, transactionParams, lotParams, "short sale")
	return impact, shortDependency(err)
}

func (s *InvestmentService) shortCoverReconciliationImpact(ctx context.Context, input InvestmentTradeInput) (ReconciliationImpact, error) {
	simulated, _, _, _, err := s.simulateShortCover(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

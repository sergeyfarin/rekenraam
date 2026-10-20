package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// CashStockMergerInput records a cash-and-stock merger (#181): the whole long
// holding of CommodityID becomes DestinationCommodityID at the exact ratio, as
// in a share exchange, and the holder also receives Cash on PayableOn. The
// election CashBasisFraction (0 to 1 inclusive) is the share of each lot's
// basis the cash recovers; the rest carries into the new lot. The app does not
// choose it: it is a jurisdiction's rule, recorded with its source.
type CashStockMergerInput struct {
	OwnerUserID                 int64
	AuthSessionID               int64
	RequestID                   string
	EffectiveOn                 string
	PayableOn                   string
	HoldingAccountID            int64
	DestinationHoldingAccountID int64
	CommodityID                 int64
	DestinationCommodityID      int64
	RatioNumerator              int64
	RatioDenominator            int64
	CashAccountID               int64
	CurrencyID                  int64
	CashValue                   int64
	CashScale                   int
	CashBasisFractionValue      exact.Coefficient
	CashBasisFractionScale      int
	SourceEvidenceJSON          string
	Memo                        string
	ChangeReason                string
	ReconciliationOverride      CheckpointOverride
	// GainImpactAcknowledgement accepts the restated gains of later
	// decisions a backdated merger revises (#184); an in-order merger revises
	// none.
	GainImpactAcknowledgement string
}

// CashStockMergerLink is one lot's move: the exchange link, which carries the
// basis left after the cash, the realized basis and the lot's cash share.
type CashStockMergerLink struct {
	ShareExchangeLink
	RealizedBasisValue int64
	RealizedBasisScale int
	CashValue          int64
	CashScale          int
}

// CashStockMergerTotal is one cost currency's totals. An unknown lot leaves
// the bases and the result unknown; UnknownLots counts them.
type CashStockMergerTotal struct {
	ShareExchangeBasisTotal
	CashValue          exact.Coefficient
	CashScale          int
	RealizedBasisValue exact.Coefficient
	RealizedBasisScale int
	ResultValue        exact.Coefficient
	ResultScale        int
}

type CashStockMergerPlan struct {
	RatioNumerator           int64
	RatioDenominator         int64
	CashBasisFractionValue   exact.Coefficient
	CashBasisFractionScale   int
	Links                    []CashStockMergerLink
	SourceQuantityValue      exact.Coefficient
	SourceQuantityScale      int
	DestinationQuantityValue exact.Coefficient
	DestinationQuantityScale int
	Totals                   []CashStockMergerTotal
}

type CashStockMergerPreview struct {
	Plan   CashStockMergerPlan
	Impact ReconciliationImpact
}

type CashStockMergerResult struct {
	Transaction Transaction
	Plan        CashStockMergerPlan
}

func (s *InvestmentService) prepareCashStockMergerWrite(ctx context.Context, input CashStockMergerInput) (db.CreateTransactionParams, db.CreateCashStockMergerParams, error) {
	return s.prepareCashStockMergerWriteWith(ctx, input, 0, "investment.cash_and_stock_merger")
}

// prepareCashStockMergerWriteWith plans a merger journal; a replacement (#184)
// names the merger it corrects, whose correction-root slot the plan replays.
func (s *InvestmentService) prepareCashStockMergerWriteWith(ctx context.Context, input CashStockMergerInput,
	replacesOperationID int64, auditOperation string) (db.CreateTransactionParams, db.CreateCashStockMergerParams, error) {
	fail := func(err error) (db.CreateTransactionParams, db.CreateCashStockMergerParams, error) {
		return db.CreateTransactionParams{}, db.CreateCashStockMergerParams{}, err
	}
	if input.OwnerUserID <= 0 {
		return fail(ValidationError{Message: "owner user is required"})
	}
	date, err := cleanRequiredDate(input.EffectiveOn, "merger effective date")
	if err != nil {
		return fail(err)
	}
	payable, err := cleanRequiredDate(input.PayableOn, "cash payable date")
	if err != nil {
		return fail(err)
	}
	if payable < date {
		return fail(ValidationError{Message: "the cash payable date cannot precede the merger effective date"})
	}
	if input.RatioNumerator <= 0 || input.RatioDenominator <= 0 ||
		input.RatioNumerator > investmentSplitRatioMax || input.RatioDenominator > investmentSplitRatioMax {
		return fail(ValidationError{Message: "merger ratio must be two positive whole numbers"})
	}
	if input.CommodityID <= 0 || input.DestinationCommodityID <= 0 || input.CommodityID == input.DestinationCommodityID {
		return fail(ValidationError{Message: "a merger needs two different instruments"})
	}
	if input.CashValue <= 0 || input.CashScale < 0 || input.CashScale > 12 {
		return fail(ValidationError{Message: "merger cash must be a positive amount; without cash record a share exchange"})
	}
	fractionValue, fractionScale, err := cleanCashBasisFraction(input.CashBasisFractionValue, input.CashBasisFractionScale)
	if err != nil {
		return fail(err)
	}
	divisor := new(big.Int).GCD(nil, nil, big.NewInt(input.RatioNumerator), big.NewInt(input.RatioDenominator)).Int64()
	numerator, denominator := input.RatioNumerator/divisor, input.RatioDenominator/divisor
	destinationAccountID := input.DestinationHoldingAccountID
	if destinationAccountID == 0 {
		destinationAccountID = input.HoldingAccountID
	}
	dependencies := newAccountRuleDependencies()
	if _, err := s.accountInRole(ctx, input.HoldingAccountID, date, holdingRole, dependencies); err != nil {
		return fail(err)
	}
	if destinationAccountID != input.HoldingAccountID {
		if _, err := s.accountInRole(ctx, destinationAccountID, date, holdingRole, dependencies); err != nil {
			return fail(err)
		}
	}
	if _, err := s.accountInRole(ctx, input.CashAccountID, payable, settlementRole, dependencies); err != nil {
		return fail(err)
	}
	if err := s.requireCommodityKind(ctx, input.CommodityID, date, "merged instrument", false); err != nil {
		return fail(err)
	}
	if err := s.requireCommodityKind(ctx, input.DestinationCommodityID, date, "new instrument", false); err != nil {
		return fail(err)
	}
	if err := s.requireCommodityKind(ctx, input.CurrencyID, payable, "cash currency", true); err != nil {
		return fail(err)
	}
	tradingID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fail(ValidationError{Message: "commodity trading system account is required"})
		}
		return fail(fmt.Errorf("resolve commodity trading account: %w", err))
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return fail(err)
	}
	evidence, err := cleanSizedJSONObject(input.SourceEvidenceJSON, "source evidence", investmentJSONMaxBytes)
	if err != nil {
		return fail(err)
	}
	merger := db.CreateCashStockMergerParams{
		Exchange: db.CreateShareExchangeParams{
			BookID: BookID, AccountID: input.HoldingAccountID, DestinationAccountID: destinationAccountID,
			CommodityID: input.CommodityID, DestinationCommodityID: input.DestinationCommodityID, EffectiveOn: date,
			RatioNumerator: numerator, RatioDenominator: denominator, SourceEvidenceJSON: evidence,
			ReplacesOperationID: replacesOperationID,
		},
		CashCommodityID: input.CurrencyID, CashValue: input.CashValue, CashScale: input.CashScale, PayableOn: payable,
		CashBasisFractionValue: fractionValue, CashBasisFractionScale: fractionScale,
	}
	planned, err := s.repository.PlanCashStockMerger(ctx, merger, input.OwnerUserID)
	if err != nil {
		return fail(mapCashStockMergerError(err))
	}
	merger.Exchange.ExpectedSourceQuantityValue, merger.Exchange.ExpectedSourceQuantityScale = planned.SourceQuantityValue, planned.SourceQuantityScale
	merger.Exchange.ExpectedDestinationQuantityValue, merger.Exchange.ExpectedDestinationQuantityScale = planned.DestinationQuantityValue, planned.DestinationQuantityScale
	cash := exact.New(input.CashValue)
	// The exchange's four instrument legs on the effective date; the cash on
	// its payable date, balanced in its currency through commodity_trading.
	create := CreateTransactionInput{
		OwnerUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID,
		RequestID: input.RequestID, OriginType: "browser_api",
		Operation: auditOperation, ChangeReason: input.ChangeReason,
		ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
		Spec: TransactionInput{
			Status: "posted", TransactionKind: "investment", InvestmentOperationKind: "cash_and_stock_merger",
			TransactionDate: date, Description: memo,
			JournalEntries: []JournalEntryInput{
				{EntryDate: date, EntryKind: "investment", Memo: memo, Postings: []PostingInput{
					{AccountID: input.HoldingAccountID, CommodityID: input.CommodityID,
						QuantityValue: planned.SourceQuantityValue.Negated(), QuantityScale: planned.SourceQuantityScale, Memo: memo},
					{AccountID: tradingID, CommodityID: input.CommodityID,
						QuantityValue: planned.SourceQuantityValue, QuantityScale: planned.SourceQuantityScale, Memo: memo},
					{AccountID: destinationAccountID, CommodityID: input.DestinationCommodityID,
						QuantityValue: planned.DestinationQuantityValue, QuantityScale: planned.DestinationQuantityScale, Memo: memo},
					{AccountID: tradingID, CommodityID: input.DestinationCommodityID,
						QuantityValue: planned.DestinationQuantityValue.Negated(), QuantityScale: planned.DestinationQuantityScale, Memo: memo},
				}},
				{EntryDate: payable, EntryKind: "investment", Memo: memo, Postings: []PostingInput{
					{AccountID: input.CashAccountID, CommodityID: input.CurrencyID,
						QuantityValue: cash, QuantityScale: input.CashScale, Memo: memo},
					{AccountID: tradingID, CommodityID: input.CurrencyID,
						QuantityValue: cash.Negated(), QuantityScale: input.CashScale, Memo: memo},
				}},
			},
		},
	}
	journal, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, create, dependencies)
	if err != nil {
		return fail(err)
	}
	return journal, merger, nil
}

// cleanCashBasisFraction returns the election in lowest decimal terms: from 0
// to 1 inclusive, with at most 12 decimal places.
func cleanCashBasisFraction(value exact.Coefficient, scale int) (exact.Coefficient, int, error) {
	invalid := ValidationError{Message: "cash basis fraction must be a decimal from 0 to 1, with at most 12 decimal places"}
	if value == "" || scale < 0 || scale > 24 {
		return "", 0, invalid
	}
	if _, err := exact.Parse(string(value)); err != nil {
		return "", 0, invalid
	}
	normalized := exact.ScaledIntFromCoefficient(value, scale).Normalized()
	coefficient, err := normalized.Coefficient()
	if err != nil || normalized.Scale() > 12 || coefficient.Sign() < 0 ||
		coefficient.BigInt().Cmp(exact.Pow10(normalized.Scale())) > 0 {
		return "", 0, invalid
	}
	return coefficient, normalized.Scale(), nil
}

// PreviewCashStockMerger runs the complete merger writer, including
// checkpoint invalidation, then rolls back.
func (s *InvestmentService) PreviewCashStockMerger(ctx context.Context, input CashStockMergerInput) (CashStockMergerPreview, error) {
	input.ReconciliationOverride = unboundOverride
	journal, merger, err := s.prepareCashStockMergerWrite(ctx, input)
	if err != nil {
		return CashStockMergerPreview{}, err
	}
	journal.GainImpact = gainImpactPolicy("")
	simulated, plan, err := s.repository.SimulateCashStockMerger(ctx, journal, merger)
	if err != nil {
		return CashStockMergerPreview{}, mapCashStockMergerError(err)
	}
	impact, err := s.simulatedReconciliationImpact(ctx, simulated)
	if err != nil {
		return CashStockMergerPreview{}, err
	}
	out, err := toCashStockMergerPlan(merger, plan)
	if err != nil {
		return CashStockMergerPreview{}, err
	}
	return CashStockMergerPreview{Plan: out, Impact: impact}, nil
}

func (s *InvestmentService) PreviewCashStockMergerReconciliationImpact(ctx context.Context, input CashStockMergerInput) (ReconciliationImpact, error) {
	preview, err := s.PreviewCashStockMerger(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return preview.Impact, nil
}

// CashStockMerger commits the journal, the exchange fact with its election,
// the realizations, depletions, new lots, links and the cash decision under
// one audit event.
func (s *InvestmentService) CashStockMerger(ctx context.Context, input CashStockMergerInput) (CashStockMergerResult, error) {
	journal, merger, err := s.prepareCashStockMergerWrite(ctx, input)
	if err != nil {
		return CashStockMergerResult{}, err
	}
	journal.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transaction, plan, err := s.repository.CreateCashStockMerger(ctx, journal, merger)
	if err != nil {
		return CashStockMergerResult{}, mapCashStockMergerError(err)
	}
	out, err := toCashStockMergerPlan(merger, plan)
	if err != nil {
		return CashStockMergerResult{}, err
	}
	enriched, err := s.transactionService.enrichOne(ctx, toTransaction(transaction))
	if err != nil {
		return CashStockMergerResult{}, err
	}
	return CashStockMergerResult{Transaction: enriched, Plan: out}, nil
}

func toCashStockMergerPlan(merger db.CreateCashStockMergerParams, plan db.CashStockMergerPlan) (CashStockMergerPlan, error) {
	out := CashStockMergerPlan{RatioNumerator: merger.Exchange.RatioNumerator, RatioDenominator: merger.Exchange.RatioDenominator,
		CashBasisFractionValue: merger.CashBasisFractionValue, CashBasisFractionScale: merger.CashBasisFractionScale,
		SourceQuantityValue: plan.SourceQuantityValue, SourceQuantityScale: plan.SourceQuantityScale,
		DestinationQuantityValue: plan.DestinationQuantityValue, DestinationQuantityScale: plan.DestinationQuantityScale,
		Links: make([]CashStockMergerLink, 0, len(plan.Links))}
	exchangeLinks := make([]ShareExchangeLink, 0, len(plan.Links))
	type sums struct{ cash, realized *exact.ScaledInt }
	byCurrency := map[int64]*sums{}
	for _, link := range plan.Links {
		converted := CashStockMergerLink{ShareExchangeLink: ShareExchangeLink(link.ShareExchangeLink),
			RealizedBasisValue: link.RealizedBasisValue, RealizedBasisScale: link.RealizedBasisScale,
			CashValue: link.CashValue, CashScale: link.CashScale}
		out.Links = append(out.Links, converted)
		exchangeLinks = append(exchangeLinks, converted.ShareExchangeLink)
		current, ok := byCurrency[link.CostCommodityID]
		if !ok {
			current = &sums{cash: exact.NewScaledInt(), realized: exact.NewScaledInt()}
			byCurrency[link.CostCommodityID] = current
		}
		current.cash.AddInt64(link.CashValue, link.CashScale)
		if link.BasisKnowledge == db.InvestmentBasisKnown {
			current.realized.AddInt64(link.RealizedBasisValue, link.RealizedBasisScale)
		}
	}
	carried, err := shareExchangeBasisTotals(exchangeLinks)
	if err != nil {
		return CashStockMergerPlan{}, err
	}
	for _, exchangeTotal := range carried {
		current := byCurrency[exchangeTotal.CostCommodityID]
		total := CashStockMergerTotal{ShareExchangeBasisTotal: exchangeTotal, CashScale: current.cash.Scale()}
		if total.CashValue, err = current.cash.Coefficient(); err != nil {
			return CashStockMergerPlan{}, LedgerOverflowError{CommodityID: exchangeTotal.CostCommodityID}
		}
		if exchangeTotal.BasisKnowledge == db.InvestmentBasisKnown {
			if total.RealizedBasisValue, err = current.realized.Coefficient(); err != nil {
				return CashStockMergerPlan{}, LedgerOverflowError{CommodityID: exchangeTotal.CostCommodityID}
			}
			total.RealizedBasisScale = current.realized.Scale()
			result := exact.NewScaledInt()
			result.AddScaled(current.cash)
			result.SubScaled(current.realized)
			if total.ResultValue, err = result.Coefficient(); err != nil {
				return CashStockMergerPlan{}, LedgerOverflowError{CommodityID: exchangeTotal.CostCommodityID}
			}
			total.ResultScale = result.Scale()
		}
		out.Totals = append(out.Totals, total)
	}
	return out, nil
}

func mapCashStockMergerError(err error) error {
	if errors.Is(err, db.ErrCashStockMergerCurrency) {
		return ValidationError{Message: err.Error()}
	}
	return mapShareExchangeError(err)
}

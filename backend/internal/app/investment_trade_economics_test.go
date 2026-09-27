package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func tradeMoney(value int64) *int64 { return &value }

func TestExactTradeEconomicsFeeTreatmentAndGrossPrice(t *testing.T) {
	for _, test := range []struct {
		name      string
		treatment string
		basis     int64
		proceeds  int64
		gain      int64
	}{
		{"capitalized", "clearing_included", 10200, 11800, 1600},
		{"expensed", "separately_expensed", 10000, 12000, 2000},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			feeAccountID := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			var accountID *int64
			if test.treatment == "separately_expensed" {
				accountID = &feeAccountID
			}
			base := InvestmentTradeInput{OwnerUserID: f.ownerUserID, CommodityID: f.stockCommodityID,
				HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
				SettlementDate: "2026-01-03"}
			buy := base
			buy.TransactionDate = "2026-01-01"
			buy.GrossAmountValue = tradeMoney(-10000)
			buy.GrossAmountScale = 2
			buy.NetSettlementValue = tradeMoney(-10200)
			buy.NetSettlementScale = 2
			buy.Charges = []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200,
				AmountScale: 2, CommodityID: f.eurCommodityID, Treatment: test.treatment, ChargeAccountID: accountID}}
			bought, err := f.investmentService.Buy(ctx, buy)
			require.NoError(t, err)
			require.Len(t, bought.Transaction.JournalEntries, 2)
			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Len(t, lots, 1)
			assertMoneyValue(t, test.basis, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
			var price int64
			var approximate int
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT price_value, is_approximate FROM price_observations WHERE source_transaction_version_id = ?`, bought.Transaction.VersionID).Scan(&price, &approximate))
			require.EqualValues(t, 0, approximate)
			require.EqualValues(t, 10000000000, price, "unit price must use gross 100, excluding commission")

			sell := base
			sell.TransactionDate = "2026-02-01"
			sell.SettlementDate = "2026-02-03"
			sell.GrossAmountValue = tradeMoney(12000)
			sell.GrossAmountScale = 2
			sell.NetSettlementValue = tradeMoney(11800)
			sell.NetSettlementScale = 2
			sell.Charges = buy.Charges
			preview, err := f.investmentService.PreviewSell(ctx, sell)
			require.NoError(t, err)
			assertMoneyValue(t, test.gain, 2, preview.RealizedGain, preview.RealizedGainScale)
			sold, err := f.investmentService.Sell(ctx, sell)
			require.NoError(t, err)
			require.NotNil(t, sold.DisposalDecision)
			require.Len(t, sold.DisposalDecision.Allocations, 1)
			assertMoneyValue(t, test.proceeds, 2, sold.DisposalDecision.Allocations[0].ProceedsValue, sold.DisposalDecision.Allocations[0].ProceedsScale)
			assertMoneyValue(t, test.proceeds, 2, preview.DisposalDecision.Allocations[0].ProceedsValue, preview.DisposalDecision.Allocations[0].ProceedsScale)
			gains, err := f.investmentService.ListRealizedGains(ctx, GainsReportParams{})
			require.NoError(t, err)
			require.Len(t, gains, 1)
			assertMoneyValue(t, test.proceeds, 2, gains[0].ProceedsValue, gains[0].ProceedsScale)
			assertMoneyValue(t, test.gain, 2, gains[0].RealizedGainValue, gains[0].RealizedGainScale)
		})
	}
}

func TestManualAndGrossPricesOutrankLaterApproximateTradeOnSameDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	_, err := pricing.CreatePrice(ctx, PriceObservationInput{OwnerUserID: f.ownerUserID,
		BaseCommodityID: f.stockCommodityID, QuoteCommodityID: f.eurCommodityID,
		QuoteType: "manual", PriceValue: 11000000000, PriceScale: 8,
		ValuationDate: "2026-01-01", IsManual: true, MetadataJSON: `{}`})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		CashAmountValue: 10000, CashAmountScale: 2})
	require.NoError(t, err)
	positions, err := f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, int64(11000000000), *positions[0].LatestPriceValue)
	require.False(t, positions[0].LatestPriceApproximate)

	// On a date without a manual quote, a gross-derived quote outranks a
	// later-arriving net-only estimate.
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		GrossAmountValue: tradeMoney(-12000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-12000), NetSettlementScale: 2})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		CashAmountValue: 13000, CashAmountScale: 2})
	require.NoError(t, err)
	positions, err = f.investmentService.Positions(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(12000000000), *positions[0].LatestPriceValue)
	require.False(t, positions[0].LatestPriceApproximate)
}

func TestExactTradeEconomicsRejectsUnbalancedNet(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-10100), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200, AmountScale: 2, CommodityID: f.eurCommodityID}}}
	_, err := f.investmentService.Buy(context.Background(), input)
	require.ErrorContains(t, err, "gross plus charges does not equal net settlement")
	require.Zero(t, f.transactionCount(t))
}

func TestForeignTradeFeeKeepsCostCurrencyBasisAndPostsOwnCashLegs(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	result, err := f.database.ExecContext(ctx, `INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id) VALUES (1, 'USD', 'currency', 1, '2026-01-01T00:00:00Z', 1)`)
	require.NoError(t, err)
	usdID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `INSERT INTO commodity_versions (commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test', 'active', 'USD', '$', 'US dollar', 2, 6)`, usdID)
	require.NoError(t, err)
	foreignCash := seedTestAccount(t, f.database, "active", true)
	feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(1), GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-10000), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -500, AmountScale: 2,
			CommodityID: usdID, ChargeAccountID: &feeAccount, CashAccountID: &foreignCash, PaidOn: "2026-01-04"}}}
	trade, err := f.investmentService.Buy(ctx, input)
	require.NoError(t, err)
	require.Len(t, trade.Transaction.JournalEntries, 2)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	assertMoneyValue(t, 10000, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
	var cashLegs, expenseLegs int
	for _, entry := range trade.Transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.CommodityID != usdID {
				continue
			}
			if posting.AccountID == foreignCash && posting.QuantityValue.String() == "-500" {
				cashLegs++
			}
			if posting.AccountID == feeAccount && posting.QuantityValue.String() == "500" {
				expenseLegs++
			}
		}
	}
	require.Equal(t, 1, cashLegs)
	require.Equal(t, 1, expenseLegs)
	var treatment, tier string
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT charge_treatment, resolution_tier FROM investment_operation_components WHERE component_kind = 'charge'`).Scan(&treatment, &tier))
	require.Equal(t, "separately_expensed", treatment)
	require.Equal(t, "fallback", tier)
}

func TestSeparatelyPaidSameCurrencyCommissionPostsOnPaymentDate(t *testing.T) {
	for _, test := range []struct {
		treatment string
		basis     int64
	}{
		{"clearing_included", 10200}, {"separately_expensed", 10000},
	} {
		t.Run(test.treatment, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
			var accountID *int64
			if test.treatment == "separately_expensed" {
				accountID = &feeAccount
			}
			trade, err := f.investmentService.Buy(ctx, InvestmentTradeInput{
				OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
				HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
				CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
				GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
				NetSettlementValue: tradeMoney(-10000), NetSettlementScale: 2,
				Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200,
					AmountScale: 2, CommodityID: f.eurCommodityID, Treatment: test.treatment,
					ChargeAccountID: accountID, CashAccountID: &f.cashAccountID, PaidOn: "2026-01-03"}},
			})
			require.NoError(t, err)
			require.Len(t, trade.Transaction.JournalEntries, 2)
			require.Equal(t, "2026-01-03", trade.Transaction.JournalEntries[1].EntryDate)
			lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			assertMoneyValue(t, test.basis, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
			var cashAccountID int64
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT cash_account_id FROM investment_operation_components WHERE component_kind = 'charge'`).Scan(&cashAccountID))
			require.Equal(t, f.cashAccountID, cashAccountID)
			var settlementCount int
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM investment_operation_components WHERE component_kind = 'net_settlement'`).Scan(&settlementCount))
			require.Equal(t, 2, settlementCount, "main settlement and separately paid fee retain distinct source cash facts")
		})
	}
}

func TestTradeFeePolicySnapshotsAccountVersionAndOldTradeDoesNotChange(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	feeAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	result, err := f.database.ExecContext(ctx, `INSERT INTO investment_fee_policies (book_id, account_id, charge_kind, created_at) VALUES (1, ?, 'commission', '2026-01-01T00:00:00Z')`, f.holdingAccountID)
	require.NoError(t, err)
	policyID, err := result.LastInsertId()
	require.NoError(t, err)
	result, err = f.database.ExecContext(ctx, `INSERT INTO investment_fee_policy_versions (policy_id, version_seq, effective_from, treatment, charge_account_id, recorded_at) VALUES (?, 1, '2026-01-01', 'separately_expensed', ?, '2026-01-01T00:00:00Z')`, policyID, feeAccount)
	require.NoError(t, err)
	versionID, err := result.LastInsertId()
	require.NoError(t, err)
	trade, err := f.investmentService.Buy(ctx, InvestmentTradeInput{OwnerUserID: f.ownerUserID,
		TransactionDate: "2026-01-02", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(1),
		GrossAmountValue: tradeMoney(-10000), GrossAmountScale: 2,
		NetSettlementValue: tradeMoney(-10200), NetSettlementScale: 2,
		Charges: []InvestmentTradeChargeInput{{Kind: "commission", AmountValue: -200, AmountScale: 2, CommodityID: f.eurCommodityID}}})
	require.NoError(t, err)
	var treatment, tier string
	var storedVersionID int64
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT charge_treatment, resolution_tier, fee_policy_version_id FROM investment_operation_components WHERE component_kind = 'charge'`).Scan(&treatment, &tier, &storedVersionID))
	require.Equal(t, "separately_expensed", treatment)
	require.Equal(t, "account", tier)
	require.Equal(t, versionID, storedVersionID)
	_, err = f.database.ExecContext(ctx, `INSERT INTO investment_fee_policy_versions (policy_id, version_seq, effective_from, treatment, recorded_at) VALUES (?, 2, '2026-01-03', 'clearing_included', '2026-01-03T00:00:00Z')`, policyID)
	require.NoError(t, err)
	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assertMoneyValue(t, 10000, 2, lots[0].CostBasisValue, lots[0].CostBasisScale)
	require.NotNil(t, trade.LotID)
}

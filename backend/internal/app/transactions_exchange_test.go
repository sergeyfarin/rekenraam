package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #481: a transfer between two of the owner's accounts in different
// currencies. The request names the exact amount that left and the exact
// amount that arrived; the service adds the commodity_trading legs.

type exchangeFixture struct {
	postedOnFixture
	usdCommodityID   int64
	usdAccountID     int64
	tradingAccountID int64
	feesAccountID    int64
}

func newExchangeFixture(t *testing.T) exchangeFixture {
	t.Helper()
	f := newPostedOnFixture(t)
	var tradingAccountID int64
	require.NoError(t, f.database.QueryRowContext(context.Background(),
		`SELECT id FROM accounts WHERE system_role = 'commodity_trading'`).Scan(&tradingAccountID))
	return exchangeFixture{
		postedOnFixture:  f,
		usdCommodityID:   seedTestCurrencyCommodity(t, f.database, "USD"),
		usdAccountID:     seedTestAccount(t, f.database, "active", true),
		tradingAccountID: tradingAccountID,
		feesAccountID:    seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense"),
	}
}

// exchange is 100.00 EUR out of cash and 108.00 USD into the dollar account,
// with extra postings appended (a fee) and posting days on each side.
func (f exchangeFixture) exchange(t *testing.T, sentOn, arrivedOn string, extra ...PostingInput) (Transaction, error) {
	t.Helper()
	postings := append([]PostingInput{
		{AccountID: f.cashAccountID, PostedOn: sentOn, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
		{AccountID: f.usdAccountID, PostedOn: arrivedOn, CommodityID: f.usdCommodityID, QuantityValue: exact.New(10800), QuantityScale: 2},
	}, extra...)
	return f.create(t, TransactionInput{
		TransactionKind: "transfer",
		TransactionDate: "2026-06-01",
		JournalEntries:  []JournalEntryInput{{EntryKind: "exchange", Postings: postings}},
	})
}

func TestExchangeBalancesEachCurrencyThroughCommodityTrading(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	created, err := f.exchange(t, "", "")
	require.NoError(t, err)

	require.Len(t, created.JournalEntries, 1)
	assert.Equal(t, "exchange", created.JournalEntries[0].EntryKind)
	assert.Equal(t, []datedPosting{
		{"2026-06-01", f.cashAccountID, "-10000"},
		{"2026-06-01", f.usdAccountID, "10800"},
		{"2026-06-01", f.tradingAccountID, "10000"},
		{"2026-06-01", f.tradingAccountID, "-10800"},
	}, journalShape(created), "both amounts exactly as entered; one trading leg per currency")
	assert.Equal(t, "transfer", created.TransactionKind)
}

func TestExchangeFeeOnTopPostsOnItsCategoryOutsideTheExchange(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	// 100.00 EUR transferred plus a 2.50 EUR fee: cash pays 102.50.
	created, err := f.create(t, TransactionInput{
		TransactionDate: "2026-06-01",
		JournalEntries: []JournalEntryInput{{EntryKind: "exchange", Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10250), QuantityScale: 2},
			{AccountID: f.usdAccountID, CommodityID: f.usdCommodityID, QuantityValue: exact.New(10800), QuantityScale: 2},
			{AccountID: f.feesAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(250), QuantityScale: 2},
		}}},
	})
	require.NoError(t, err)

	assert.Equal(t, []datedPosting{
		{"2026-06-01", f.cashAccountID, "-10250"},
		{"2026-06-01", f.usdAccountID, "10800"},
		{"2026-06-01", f.feesAccountID, "250"},
		{"2026-06-01", f.tradingAccountID, "10000"},
		{"2026-06-01", f.tradingAccountID, "-10800"},
	}, journalShape(created), "the exchange carries only the transferred 100.00; the fee is its own expense")
}

func TestExchangeFeeChargedOnTheArrivingSide(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	// The receiving bank keeps 1.00 USD of the 108.00 that arrived.
	created, err := f.create(t, TransactionInput{
		TransactionDate: "2026-06-01",
		JournalEntries: []JournalEntryInput{{EntryKind: "exchange", Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
			{AccountID: f.usdAccountID, CommodityID: f.usdCommodityID, QuantityValue: exact.New(10700), QuantityScale: 2},
			{AccountID: f.feesAccountID, CommodityID: f.usdCommodityID, QuantityValue: exact.New(100), QuantityScale: 2},
		}}},
	})
	require.NoError(t, err)

	assert.Contains(t, journalShape(created), datedPosting{"2026-06-01", f.tradingAccountID, "-10800"}, "the exchange converts the full 108.00 that arrived")
}

func TestExchangeWithPostingDaysBridgesEachCurrencyAndConvertsOnTheTransactionDate(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	created, err := f.exchange(t, "2026-06-01", "2026-06-03")
	require.NoError(t, err)

	// Dated on the send day: the conversion happens then, so the dollars are
	// what is in transit until they arrive.
	assert.Equal(t, []datedPosting{
		{"2026-06-01", f.cashAccountID, "-10000"},
		{"2026-06-01", f.tradingAccountID, "10000"},
		{"2026-06-01", f.tradingAccountID, "-10800"},
		{"2026-06-01", f.clearingAccountID, "10800"},
		{"2026-06-03", f.usdAccountID, "10800"},
		{"2026-06-03", f.clearingAccountID, "-10800"},
	}, journalShape(created))
	for _, entry := range created.JournalEntries {
		assert.Equal(t, "exchange", entry.EntryKind, "every dated entry keeps the exchange kind")
	}

	netWorth, err := f.transactionService.NetWorth(context.Background(), NetWorthInput{AsOf: "2026-06-02"})
	require.NoError(t, err)
	usd, found := amountFor(t, netWorth.Totals, f.usdCommodityID)
	require.True(t, found, "the dollars in transit still count")
	requireScaled(t, 10800, 2, exact.ScaledIntFromCoefficient(usd.QuantityValue, usd.QuantityScale), "dollars in transit")
}

func TestExchangeRefusalsWriteNothing(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	secondEURAccountID := seedTestAccount(t, f.database, "active", true)
	eur := func(accountID int64, value int64) PostingInput {
		return PostingInput{AccountID: accountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(value), QuantityScale: 2}
	}
	usd := func(accountID int64, value int64) PostingInput {
		return PostingInput{AccountID: accountID, CommodityID: f.usdCommodityID, QuantityValue: exact.New(value), QuantityScale: 2}
	}

	cases := map[string][]PostingInput{
		"same currency":           {eur(f.cashAccountID, -100), eur(secondEURAccountID, 100)},
		"both amounts leave":      {eur(f.cashAccountID, -100), usd(f.usdAccountID, -108)},
		"one account":             {eur(f.cashAccountID, -100), eur(f.feesAccountID, 100)},
		"three accounts":          {eur(f.cashAccountID, -100), usd(f.usdAccountID, 54), usd(seedTestAccount(t, f.database, "active", true), 54)},
		"a named trading leg":     {eur(f.cashAccountID, -100), usd(f.usdAccountID, 108), eur(f.tradingAccountID, 100), usd(f.tradingAccountID, -108)},
		"a named clearing leg":    {eur(f.cashAccountID, -100), usd(f.usdAccountID, 108), eur(f.clearingAccountID, 0)},
		"two fees":                {eur(f.cashAccountID, -104), usd(f.usdAccountID, 108), eur(f.feesAccountID, 2), eur(f.feesAccountID, 2)},
		"a negative fee":          {eur(f.cashAccountID, -98), usd(f.usdAccountID, 108), eur(f.feesAccountID, -2)},
		"a fee in a third ccy":    {eur(f.cashAccountID, -100), usd(f.usdAccountID, 108), {AccountID: f.feesAccountID, CommodityID: seedTestCurrencyCommodity(t, f.database, "GBP"), QuantityValue: exact.New(1), QuantityScale: 2}},
		"a fee above the arrival": {eur(f.cashAccountID, -100), usd(f.usdAccountID, -1), usd(f.feesAccountID, 109)},
		"a fee above the payment": {eur(f.cashAccountID, -100), usd(f.usdAccountID, 108), eur(f.feesAccountID, 150)},
		"an income leg":           {eur(f.cashAccountID, -100), usd(f.usdAccountID, 108), eur(f.incomeAccountID, 1)},
	}
	for name, postings := range cases {
		_, err := f.create(t, TransactionInput{
			TransactionDate: "2026-06-01",
			JournalEntries:  []JournalEntryInput{{EntryKind: "exchange", Postings: postings}},
		})
		var validation ValidationError
		assert.ErrorAs(t, err, &validation, name)
	}

	_, err := f.create(t, TransactionInput{
		TransactionDate: "2026-06-01",
		JournalEntries: []JournalEntryInput{
			{EntryKind: "exchange", Postings: []PostingInput{eur(f.cashAccountID, -100), usd(f.usdAccountID, 108)}},
			{EntryKind: "exchange", Postings: []PostingInput{eur(f.cashAccountID, -100), usd(f.usdAccountID, 108)}},
		},
	})
	var validation ValidationError
	assert.ErrorAs(t, err, &validation, "an exchange is one journal entry")

	var count int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&count))
	assert.Zero(t, count)
}

func TestExchangeEditRebuildsTheTradingLegs(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()

	created, err := f.exchange(t, "", "")
	require.NoError(t, err)

	// The editor resends the two account postings and the exchange kind,
	// never the trading legs; a corrected arrival rebuilds them.
	spec := transactionInputFromTransaction(created)
	spec.JournalEntries[0].Postings = spec.JournalEntries[0].Postings[:2]
	spec.JournalEntries[0].Postings[1].QuantityValue = exact.New(10750)
	updated, err := f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		ExpectedVersionID: created.VersionID, OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		TransactionID: created.ID, Spec: spec, ChangeReason: "bank statement",
	})
	require.NoError(t, err)

	assert.Equal(t, []datedPosting{
		{"2026-06-01", f.cashAccountID, "-10000"},
		{"2026-06-01", f.usdAccountID, "10750"},
		{"2026-06-01", f.tradingAccountID, "10000"},
		{"2026-06-01", f.tradingAccountID, "-10750"},
	}, journalShape(updated))
}

func TestExchangeMoveDayKeepsTheExchangeAndBridgesTheArrival(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	created, err := f.exchange(t, "", "")
	require.NoError(t, err)

	moved, err := f.moveDay(t, created, f.usdAccountID, "later", false)
	require.NoError(t, err)

	assert.Equal(t, []datedPosting{
		{"2026-06-01", f.cashAccountID, "-10000"},
		{"2026-06-01", f.tradingAccountID, "10000"},
		{"2026-06-01", f.tradingAccountID, "-10800"},
		{"2026-06-01", f.clearingAccountID, "10800"},
		{"2026-06-02", f.usdAccountID, "10800"},
		{"2026-06-02", f.clearingAccountID, "-10800"},
	}, journalShape(moved), "the trading legs are rebuilt once, not carried and doubled")
}

func TestExchangePassesSelfCheck(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()

	_, err := f.create(t, TransactionInput{
		TransactionDate: "2026-06-01",
		JournalEntries: []JournalEntryInput{{EntryKind: "exchange", Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10250), QuantityScale: 2},
			{AccountID: f.usdAccountID, PostedOn: "2026-06-03", CommodityID: f.usdCommodityID, QuantityValue: exact.New(10800), QuantityScale: 2},
			{AccountID: f.feesAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(250), QuantityScale: 2},
		}}},
	})
	require.NoError(t, err)

	run, err := NewSelfCheckService(db.NewSelfCheckRepository(f.database, f.database)).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	for _, result := range run.Results {
		assert.NotEqual(t, "fail", result.Status, "%s: %s", result.CheckID, result.Summary)
	}
	assert.Zero(t, run.FailedCheckCount)
}

func TestExchangeCashflowShowsBothSidesAsTransfersAndKeepsTheConversionDifferenceThere(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	// 1 USD = 0.92 EUR on the day: 108.00 USD is worth 99.36 EUR.
	_, err := pricing.CreatePrice(ctx, PriceObservationInput{OwnerUserID: f.ownerUserID, BaseCommodityID: f.usdCommodityID, QuoteCommodityID: f.eurCommodityID, QuoteType: "official_fixing", AdjustmentBasis: "raw", ValuationDate: "2026-06-01", PriceValue: 92, PriceScale: 2})
	require.NoError(t, err)

	_, err = f.create(t, TransactionInput{
		TransactionDate: "2026-06-01",
		JournalEntries: []JournalEntryInput{{EntryKind: "exchange", Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10250), QuantityScale: 2},
			{AccountID: f.usdAccountID, CommodityID: f.usdCommodityID, QuantityValue: exact.New(10800), QuantityScale: 2},
			{AccountID: f.feesAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(250), QuantityScale: 2},
		}}},
	})
	require.NoError(t, err)

	report, err := f.transactionService.Cashflow(ctx, CashflowInput{
		StartDate: "2026-06-01", EndDate: "2026-06-30", Bucket: "month",
		Reporting: &ReportingCurrencyInput{CommodityID: f.eurCommodityID},
	})
	require.NoError(t, err)
	require.Len(t, report.Buckets, 1)
	bucket := report.Buckets[0]

	scaled := func(amount BalanceQuantity) *exact.ScaledInt {
		return exact.ScaledIntFromCoefficient(amount.QuantityValue, amount.QuantityScale)
	}
	out, found := amountFor(t, bucket.TransferOut, f.eurCommodityID)
	require.True(t, found)
	requireScaled(t, 10000, 2, scaled(out), "the euros that left are a transfer out")
	in, found := amountFor(t, bucket.TransferIn, f.usdCommodityID)
	require.True(t, found)
	requireScaled(t, 10800, 2, scaled(in), "the dollars that arrived are a transfer in")
	fee, found := amountFor(t, bucket.Outflow, f.eurCommodityID)
	require.True(t, found)
	requireScaled(t, 250, 2, scaled(fee), "only the fee is spending")
	_, usdSpent := amountFor(t, bucket.Outflow, f.usdCommodityID)
	assert.False(t, usdSpent)

	require.NotNil(t, bucket.ConvertedTransferNet)
	requireScaled(t, -64, 2, scaled(*bucket.ConvertedTransferNet), "the conversion difference stays inside transfers (net)")
	require.NotNil(t, bucket.ConvertedOutflow)
	requireScaled(t, 250, 2, scaled(*bucket.ConvertedOutflow), "the conversion difference is never spending")
}

func (f exchangeFixture) usdRate(t *testing.T, date string, value int64) {
	t.Helper()
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	_, err := pricing.CreatePrice(context.Background(), PriceObservationInput{OwnerUserID: f.ownerUserID, BaseCommodityID: f.usdCommodityID, QuoteCommodityID: f.eurCommodityID, QuoteType: "official_fixing", AdjustmentBasis: "raw", ValuationDate: date, PriceValue: value, PriceScale: 2})
	require.NoError(t, err)
}

func TestExchangeConversionComparesTheDealWithTheMarketRateOfTheDay(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))
	f.usdRate(t, "2026-06-01", 92) // 1 USD = 0.92 EUR

	created, err := f.exchange(t, "", "")
	require.NoError(t, err)
	conversion, err := f.transactionService.ExchangeConversion(ctx, created)
	require.NoError(t, err)
	require.NotNil(t, conversion)

	assert.Equal(t, "2026-06-01", conversion.ConversionDate)
	assert.Equal(t, ExchangeQuantity{CommodityID: f.eurCommodityID, QuantityValue: exact.New(10000), QuantityScale: 2}, conversion.Sent)
	assert.Equal(t, ExchangeQuantity{CommodityID: f.usdCommodityID, QuantityValue: exact.New(10800), QuantityScale: 2}, conversion.Received)
	assert.Equal(t, "1080000", conversion.ImpliedRate.QuantityValue.String(), "1.080000 USD per EUR")
	require.NotNil(t, conversion.Market)
	assert.Equal(t, "1086957", conversion.Market.Rate.QuantityValue.String(), "1 / 0.92, half-up at six places")
	assert.Equal(t, "9936", conversion.Market.MarketValue.QuantityValue.String(), "108.00 USD is worth 99.36 EUR")
	assert.Equal(t, "64", conversion.Market.Cost.QuantityValue.String(), "0.64 EUR less than the market rate")
	assert.Equal(t, 2, conversion.Market.Cost.QuantityScale)
	assert.Equal(t, "64", conversion.Market.CostPercent.String(), "0.64 %")
	assert.Equal(t, "2026-06-01", conversion.Market.ObservationDate)
	assert.False(t, conversion.Market.Stale)
}

func TestExchangeConversionWithoutAMarketRateSaysSoAndShowsTheLaterRateWhenItArrives(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))

	created, err := f.exchange(t, "", "")
	require.NoError(t, err)
	conversion, err := f.transactionService.ExchangeConversion(ctx, created)
	require.NoError(t, err)
	require.NotNil(t, conversion)
	assert.Nil(t, conversion.Market, "no rate is invented")
	assert.NotEmpty(t, conversion.MarketGap)
	assert.Equal(t, "1080000", conversion.ImpliedRate.QuantityValue.String(), "the deal itself is always known")

	// A rate observed three days earlier is inside the staleness window: used,
	// and marked stale.
	f.usdRate(t, "2026-05-29", 93)
	conversion, err = f.transactionService.ExchangeConversion(ctx, created)
	require.NoError(t, err)
	require.NotNil(t, conversion.Market)
	assert.Equal(t, "2026-05-29", conversion.Market.ObservationDate)
	assert.True(t, conversion.Market.Stale)
	assert.Equal(t, "-44", conversion.Market.Cost.QuantityValue.String(), "108.00 × 0.93 = 100.44: better than that market rate")

	// The day's own rate, arriving later, replaces it on the next read.
	f.usdRate(t, "2026-06-01", 92)
	conversion, err = f.transactionService.ExchangeConversion(ctx, created)
	require.NoError(t, err)
	assert.Equal(t, "2026-06-01", conversion.Market.ObservationDate)
	assert.Equal(t, "64", conversion.Market.Cost.QuantityValue.String())
}

func TestExchangeConversionIsNilForOtherTransactions(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)

	created, err := f.create(t, TransactionInput{
		TransactionDate: "2026-06-01",
		JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-100), QuantityScale: 2},
			{AccountID: f.savingsAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(100), QuantityScale: 2},
		}}},
	})
	require.NoError(t, err)
	conversion, err := f.transactionService.ExchangeConversion(context.Background(), created)
	require.NoError(t, err)
	assert.Nil(t, conversion)
}

func TestExchangeConversionUsesThePairStoredTheOtherWayRound(t *testing.T) {
	t.Parallel()
	f := newExchangeFixture(t)
	ctx := context.Background()
	f.transactionService.SetPricingRepository(db.NewPricingRepository(f.database))
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	// Only EUR priced in USD: 1 EUR = 1.09 USD.
	_, err := pricing.CreatePrice(ctx, PriceObservationInput{OwnerUserID: f.ownerUserID, BaseCommodityID: f.eurCommodityID, QuoteCommodityID: f.usdCommodityID, QuoteType: "official_fixing", AdjustmentBasis: "raw", ValuationDate: "2026-06-01", PriceValue: 109, PriceScale: 2})
	require.NoError(t, err)

	created, err := f.exchange(t, "", "")
	require.NoError(t, err)
	conversion, err := f.transactionService.ExchangeConversion(ctx, created)
	require.NoError(t, err)
	require.NotNil(t, conversion.Market)
	assert.Equal(t, "1090000", conversion.Market.Rate.QuantityValue.String(), "the stored rate exactly, not one re-derived from a rounded value")
	assert.Equal(t, "9908", conversion.Market.MarketValue.QuantityValue.String(), "108.00 ÷ 1.09 = 99.0826… → 99.08 EUR")
	assert.Equal(t, "92", conversion.Market.Cost.QuantityValue.String())
}

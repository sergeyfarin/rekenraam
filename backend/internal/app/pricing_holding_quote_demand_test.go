package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"
)

// #532: a EUR book buys a USD-quoted stock with EUR cash. The ledger posts
// EUR and the stock, never USD, yet valuation chains the holding through USD
// (#281), so the holding itself must demand the USD rate.

// quoteInstrument gives the fixture's TEST commodity an instrument version
// quoted in the given currency.
func quoteInstrument(t *testing.T, f homeFixture, quoteCommodityID int64) {
	t.Helper()
	ctx := context.Background()
	result, err := f.database.ExecContext(ctx, `INSERT INTO investment_instruments (book_id, commodity_id, created_at, created_by_user_id)
		VALUES (?, ?, '2026-01-01T00:00:00Z', 1)`, BookID, f.stockCommodityID)
	require.NoError(t, err)
	instrumentID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `INSERT INTO investment_instrument_versions (instrument_id, version_seq, effective_from, recorded_at,
		changed_by_user_id, change_reason, status, instrument_type, display_name, quote_commodity_id, trading_commodity_id, quantity_scale, price_scale)
		VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', 'stock', 'Test Security', ?, ?, 0, 2)`,
		instrumentID, quoteCommodityID, quoteCommodityID)
	require.NoError(t, err)
}

func (f homeFixture) buyTenWithEUR(t *testing.T, date string) Transaction {
	t.Helper()
	result, err := f.investmentService.Buy(context.Background(), InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date,
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 50000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	return result.Transaction
}

func demandFor(t *testing.T, f homeFixture, commodityID int64) (db.PricingCurrencyDemandRecord, bool) {
	t.Helper()
	demand, err := f.pricing.fxDemand(context.Background())
	require.NoError(t, err)
	for _, record := range demand {
		if record.CommodityID == commodityID {
			return record, true
		}
	}
	return db.PricingCurrencyDemandRecord{}, false
}

func pendingFXCoverageFor(t *testing.T, f homeFixture, currencyID int64) []string {
	t.Helper()
	rows, err := f.database.Query(`SELECT json_extract(payload_json, '$.start_date') FROM background_work_items
		WHERE kind = 'pricing.fx_coverage' AND status = 'pending'
		  AND json_extract(payload_json, '$.currency_id') = ? ORDER BY id`, currencyID)
	require.NoError(t, err)
	defer rows.Close()
	starts := []string{}
	for rows.Next() {
		var start string
		require.NoError(t, rows.Scan(&start))
		starts = append(starts, start)
	}
	require.NoError(t, rows.Err())
	return starts
}

func TestFXDemandIncludesTheQuoteCurrencyOfAHeldInstrument(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	quoteInstrument(t, f, usd)

	_, found := demandFor(t, f, usd)
	require.False(t, found, "an instrument nobody holds demands no rate")

	f.buyTenWithEUR(t, "2026-09-28")

	record, found := demandFor(t, f, usd)
	require.True(t, found, "the held instrument's quote currency is demanded")
	assert.Equal(t, db.PricingCurrencyDemandRecord{CommodityID: usd, Code: "USD", StartDate: "2026-09-28"}, record,
		"the obligation starts at the holding's first posted date")

	demand, err := f.pricing.fxDemand(ctx)
	require.NoError(t, err)
	targets, err := f.pricing.fxRefreshTargets(ctx, demand)
	require.NoError(t, err)
	assert.Equal(t, []string{"EUR/USD", "USD/EUR"}, fxTargetCodes(targets),
		"the quote currency's pair to the default currency is a refresh target")

	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"the posting outbox wakes FX coverage for the trade that created the demand")
}

// An instrument with no quote currency recorded is still valued through every
// currency it is priced in, so an observation names the leg. The obligation
// starts at the holding, not at the observation.
func TestFXDemandIncludesAHeldCommodityObservationCurrency(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	f.buyTenWithEUR(t, "2026-09-28")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd))

	f.priceIn(t, f.stockCommodityID, usd, "2026-10-15", "60")

	record, found := demandFor(t, f, usd)
	require.True(t, found)
	assert.Equal(t, "2026-09-28", record.StartDate)

	// A later posting of the holding wakes coverage for the observed currency.
	f.buyTenWithEUR(t, "2026-10-01")
	assert.Equal(t, []string{"2026-10-01"}, pendingFXCoverageFor(t, f, usd))
}

func TestFXDemandIgnoresDraftAndVoidedHoldings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	quoteInstrument(t, f, usd)

	holdStock := func(status string) Transaction {
		transaction, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
			OwnerUserID: 1, OriginType: "internal", Spec: TransactionInput{TransactionDate: "2026-09-20", Status: status,
				JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
					{AccountID: f.cashAccountID, CommodityID: f.stockCommodityID, QuantityValue: exact.New(10), QuantityScale: 0},
					{AccountID: f.equity, CommodityID: f.stockCommodityID, QuantityValue: exact.New(-10), QuantityScale: 0},
				}}}}})
		require.NoError(t, err)
		return transaction
	}

	holdStock("draft")
	_, found := demandFor(t, f, usd)
	assert.False(t, found, "a draft holding demands no rate")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd), "a draft does not wake FX coverage")

	posted := holdStock("posted")
	_, found = demandFor(t, f, usd)
	require.True(t, found)

	_, err := f.transactionService.VoidTransaction(ctx, VoidTransactionInput{OwnerUserID: f.ownerUserID,
		OriginType: "browser_api", TransactionID: posted.ID, ChangeReason: "entered in error"})
	require.NoError(t, err)
	_, found = demandFor(t, f, usd)
	assert.False(t, found, "a voided transaction leaves no holding demand")
}

// Historical demand is wider than current exposure: selling the whole holding
// keeps the rate obligation from the first posted date.
func TestFXDemandKeepsASoldHoldingsQuoteCurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	quoteInstrument(t, f, usd)
	f.buyTenWithEUR(t, "2026-09-28")

	_, err := f.investmentService.Sell(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-10-05",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 55000, CashAmountScale: 2,
	})
	require.NoError(t, err)

	record, found := demandFor(t, f, usd)
	require.True(t, found)
	assert.Equal(t, "2026-09-28", record.StartDate)
}

// recordingFXProvider answers every EUR/USD request at 1 and records the
// requested dates, so a test can see how far back a sweep reaches.
type recordingFXProvider struct{ dates *[]string }

func (p recordingFXProvider) Code() string { return "frankfurter" }
func (p recordingFXProvider) Name() string { return "recording" }
func (p recordingFXProvider) Capabilities() marketdata.ProviderCapabilities {
	return marketdata.ProviderCapabilities{FXRates: true, HistoricalFXRates: true}
}
func (p recordingFXProvider) FetchFXRates(ctx context.Context, request marketdata.FXRateRequest) ([]marketdata.FXRateObservation, error) {
	*p.dates = append(*p.dates, request.Date)
	return fakeFXProvider{code: "frankfurter", rates: map[string]fakeFXRate{"EUR/USD": {value: 1}, "USD/EUR": {value: 1}}}.FetchFXRates(ctx, request)
}

// PR #539 review: the holding trigger reads every instrument version, so an
// obsolete version's quote currency still wakes coverage at the holding's
// date. That wakeup is only a hint: the sweep must take the start date from
// FXDemand, never reach back to the hint's earlier date for a currency
// demanded later by something else.
func TestFXHoldingWakeupCannotBackdateOtherDemand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	quoteInstrument(t, f, usd)
	_, err := f.database.Exec(`INSERT INTO investment_instrument_versions (instrument_id, version_seq, effective_from, recorded_at,
		changed_by_user_id, change_reason, status, instrument_type, display_name, quote_commodity_id, trading_commodity_id, quantity_scale, price_scale)
		SELECT instrument_id, 2, effective_from, '2026-10-15T00:00:00Z', 1, 'correct mistaken quote currency', status, instrument_type,
			display_name, ?, ?, quantity_scale, price_scale
		FROM investment_instrument_versions WHERE version_seq = 1`, f.eurCommodityID, f.eurCommodityID)
	require.NoError(t, err)
	f.move(t, "2026-10-15", f.cashAccountID, usd, 100)
	f.buyTenWithEUR(t, "2026-09-28")

	demand, found := demandFor(t, f, usd)
	require.True(t, found)
	require.Equal(t, "2026-10-15", demand.StartDate, "USD is demanded only by the later USD balance")
	var raw string
	require.NoError(t, f.database.QueryRow(`SELECT payload_json FROM background_work_items
		WHERE kind = 'pricing.fx_coverage' AND json_extract(payload_json, '$.currency_id') = ?
		  AND json_extract(payload_json, '$.start_date') = '2026-09-28'`, usd).Scan(&raw),
		"the obsolete version's currency still wakes coverage at the holding's date")
	var payload fxCoverageWorkPayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))

	dates := []string{}
	service := NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry(recordingFXProvider{dates: &dates}))
	service.SetNowForTest(func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) })
	_, err = f.database.Exec(`UPDATE market_data_sources SET status = 'disabled' WHERE kind = 'provider' AND code <> 'frankfurter'`)
	require.NoError(t, err)
	source := marketDataSourceIDByCode(t, f.database, "frankfurter")
	_, err = service.SavePolicy(ctx, PricingPolicyInput{OwnerUserID: 1, DefaultSourceID: &source, MaxBackfillDays: 1})
	require.NoError(t, err)

	_, _, err = service.runFXCoverage(ctx, 1, payload)
	require.NoError(t, err)
	require.NotEmpty(t, dates, "the wakeup still covers the demanded dates")
	for _, date := range dates {
		assert.GreaterOrEqual(t, date, demand.StartDate, "a holding hint must not extend an unrelated USD obligation")
	}
}

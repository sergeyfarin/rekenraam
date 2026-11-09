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
	createTestInstrument(t, f, &quoteCommodityID)
}

func createTestInstrument(t *testing.T, f homeFixture, quoteCommodityID *int64) int64 {
	t.Helper()
	return createTestInstrumentEffective(t, f, quoteCommodityID, "2026-01-01")
}

func createTestInstrumentEffective(t *testing.T, f homeFixture, quoteCommodityID *int64, effectiveFrom string) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := f.database.ExecContext(ctx, `INSERT INTO investment_instruments (book_id, commodity_id, created_at, created_by_user_id)
		VALUES (?, ?, '2026-01-01T00:00:00Z', 1)`, BookID, f.stockCommodityID)
	require.NoError(t, err)
	instrumentID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `INSERT INTO investment_instrument_versions (instrument_id, version_seq, effective_from, recorded_at,
		changed_by_user_id, change_reason, status, instrument_type, display_name, quote_commodity_id, trading_commodity_id, quantity_scale, price_scale)
		VALUES (?, 1, ?, '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', 'stock', 'Test Security', ?, ?, 0, 2)`,
		instrumentID, effectiveFrom, quoteCommodityID, quoteCommodityID)
	require.NoError(t, err)
	return instrumentID
}

func updateTestInstrumentQuote(t *testing.T, f homeFixture, instrumentID, quoteCommodityID int64, effectiveFrom string) {
	t.Helper()
	_, err := f.investmentService.UpdateInstrument(context.Background(), InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.update",
		InstrumentID: instrumentID, InstrumentType: "stock", DisplayName: "Test Security",
		QuoteCommodityID: &quoteCommodityID, QuantityScale: 0, PriceScale: 2,
		EffectiveFrom: effectiveFrom, ChangeReason: "test fixture",
	})
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
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"the first observation in a new currency wakes coverage from the holding's first posted date")

	// A later posting of the holding does not duplicate the pending sweep.
	f.buyTenWithEUR(t, "2026-10-01")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd))
}

// #532's posting trigger owns the observation half of the UNION: an
// observation recorded while the commodity was not held still wakes coverage
// when the first buy posts it.
func TestFXPostingWakesACurrencyNamedOnlyByAnEarlierObservation(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")

	f.priceIn(t, f.stockCommodityID, usd, "2026-09-01", "60")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd), "a watchlist observation wakes nothing")

	f.buyTenWithEUR(t, "2026-09-28")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"the posting trigger wakes the currency named only by the earlier observation")
}

// #540: a quote currency can arrive after the holding's last posting. Saving
// it on the instrument must wake FX coverage from the holding's first posted
// date. Duplicate hints are left to the pending-work coalescing.
func TestFXSavingAQuoteCurrencyOnAHeldInstrumentWakesCoverage(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	gbp := seedTestCurrencyCommodity(t, f.database, "GBP")
	instrumentID := createTestInstrument(t, f, nil)
	f.buyTenWithEUR(t, "2026-09-28")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd), "the instrument has no quote currency yet")

	updateTestInstrumentQuote(t, f, instrumentID, usd, "2026-10-01")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"the new quote currency wakes coverage from the holding's first posted date")

	f.priceIn(t, f.stockCommodityID, usd, "2026-10-15", "60")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"the first observation in the same currency coalesces with the pending holding wakeup")

	updateTestInstrumentQuote(t, f, instrumentID, gbp, "2026-10-03")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, gbp),
		"changing the quote currency wakes the newly demanded currency")
}

// #540 review: FXDemand selects the version in effect today, not the highest
// version_seq. A backdated version must not hide the wakeup for a later version
// that actually becomes current.
func TestFXBackdatedInstrumentVersionStillWakesTheCurrentCurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	gbp := seedTestCurrencyCommodity(t, f.database, "GBP")
	instrument, err := f.investmentService.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.create",
		CommodityCode: "FXBACKDATED", InstrumentType: "stock", DisplayName: "FX backdated instrument",
		QuoteCommodityID: &gbp, TradingCommodityID: &gbp, QuantityScale: 0, PriceScale: 2,
	})
	require.NoError(t, err)
	holding, err := f.investmentService.CreateHoldingAccount(ctx, HoldingAccountInput{
		OwnerUserID: f.ownerUserID, InstrumentID: instrument.ID,
		Name: "FX holding", OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	_, err = f.investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-09-28",
		CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		QuantityValue: exact.New(10), CashAmountValue: 50000, CashAmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.database.Exec(`DELETE FROM background_work_items WHERE kind = 'pricing.fx_coverage'`)
	require.NoError(t, err)

	updateHeldInstrumentQuote(t, f, instrument.ID, "Forward USD", usd, "2026-06-01")
	updateHeldInstrumentQuote(t, f, instrument.ID, "March GBP correction", gbp, "2026-03-01")
	_, found := demandFor(t, f, gbp)
	require.False(t, found, "the backdated GBP version is not in effect")
	_, err = f.database.Exec(`UPDATE background_work_items SET status = 'completed', completed_at = updated_at
		WHERE kind = 'pricing.fx_coverage' AND json_extract(payload_json, '$.currency_id') = ?`, gbp)
	require.NoError(t, err)

	updateHeldInstrumentQuote(t, f, instrument.ID, "Current GBP", gbp, "2026-10-04")
	record, found := demandFor(t, f, gbp)
	require.True(t, found, "the effective version now demands GBP")
	assert.Equal(t, "2026-09-28", record.StartDate)
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, gbp),
		"the version that becomes current wakes coverage even though an obsolete version already named GBP")
}

func updateHeldInstrumentQuote(t *testing.T, f homeFixture, instrumentID int64, name string, quoteCommodityID int64, effectiveFrom string) {
	t.Helper()
	current, err := f.investmentService.Instrument(context.Background(), instrumentID)
	require.NoError(t, err)
	_, err = f.investmentService.UpdateInstrument(context.Background(), InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "investment.instrument.update",
		InstrumentID: instrumentID, CommodityCode: current.CommodityCode, InstrumentType: current.InstrumentType,
		DisplayName: name, Symbol: current.Symbol, ExchangeCode: current.ExchangeCode, MIC: current.MIC,
		Issuer: current.Issuer, CountryCode: current.CountryCode, QuoteCommodityID: &quoteCommodityID,
		TradingCommodityID: &quoteCommodityID, QuantityScale: current.QuantityScale, PriceScale: current.PriceScale,
		IdentifiersJSON: current.IdentifiersJSON, MetadataJSON: current.MetadataJSON,
		EffectiveFrom: effectiveFrom, ChangeReason: "test fixture",
	})
	require.NoError(t, err)
}

// #540: quote refreshes are routine, so only the first non-voided observation
// of a held commodity in a new currency may enqueue a sweep.
func TestFXHeldCommodityObservationOnlyWakesANewCurrencyOnce(t *testing.T) {
	t.Parallel()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	f.buyTenWithEUR(t, "2026-09-28")

	f.price(t, f.stockCommodityID, "2026-09-29", "50")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd), "a quote in the default currency demands no rate")

	f.priceIn(t, f.stockCommodityID, usd, "2026-10-15", "60")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"the first observation in a new currency wakes coverage from the holding's first posted date")

	f.priceIn(t, f.stockCommodityID, usd, "2026-10-16", "61")
	assert.Equal(t, []string{"2026-09-28"}, pendingFXCoverageFor(t, f, usd),
		"later observations while the sweep is pending enqueue nothing new")

	_, err := f.database.Exec(`UPDATE background_work_items SET status = 'completed', completed_at = updated_at
		WHERE kind = 'pricing.fx_coverage' AND json_extract(payload_json, '$.currency_id') = ?`, usd)
	require.NoError(t, err)
	f.priceIn(t, f.stockCommodityID, usd, "2026-10-17", "62")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd),
		"a routine observation after the sweep completed does not enqueue another one")
}

// #540: the new wakeups obey the posted-only rule. A watchlist instrument and
// a commodity held only in a draft or voided transaction demand nothing.
func TestFXHoldingQuoteWakeupsIgnoreUnpostedHoldings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	f.move(t, "2026-09-10", f.cashAccountID, f.eurCommodityID, 100000)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	gbp := seedTestCurrencyCommodity(t, f.database, "GBP")
	jpy := seedTestCurrencyCommodity(t, f.database, "JPY")
	cad := seedTestCurrencyCommodity(t, f.database, "CAD")
	instrumentID := createTestInstrument(t, f, nil)

	updateTestInstrumentQuote(t, f, instrumentID, usd, "2026-10-01")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd), "a watchlist instrument wakes nothing")

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
	updateTestInstrumentQuote(t, f, instrumentID, gbp, "2026-10-02")
	f.priceIn(t, f.stockCommodityID, usd, "2026-10-15", "60")
	assert.Empty(t, pendingFXCoverageFor(t, f, gbp), "a draft holding wakes no quote-currency sweep")
	assert.Empty(t, pendingFXCoverageFor(t, f, usd), "a draft holding wakes no observation-currency sweep")

	posted := holdStock("posted")
	_, err := f.database.Exec(`DELETE FROM background_work_items WHERE kind = 'pricing.fx_coverage'`)
	require.NoError(t, err)
	_, err = f.transactionService.VoidTransaction(ctx, VoidTransactionInput{OwnerUserID: f.ownerUserID,
		OriginType: "browser_api", TransactionID: posted.ID, ChangeReason: "entered in error"})
	require.NoError(t, err)

	updateTestInstrumentQuote(t, f, instrumentID, jpy, "2026-10-03")
	f.priceIn(t, f.stockCommodityID, cad, "2026-10-16", "9000")
	assert.Empty(t, pendingFXCoverageFor(t, f, jpy), "a voided holding wakes no quote-currency sweep")
	assert.Empty(t, pendingFXCoverageFor(t, f, cad),
		"a voided holding wakes no observation-currency sweep, even in a currency no version names")
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

// recordingFXProvider answers every EUR/USD request at 1 and records each
// requested pair and date, so a test can see how far back a sweep reaches.
type recordingFXProvider struct{ requests *[]string }

func (p recordingFXProvider) Code() string { return "frankfurter" }
func (p recordingFXProvider) Name() string { return "recording" }
func (p recordingFXProvider) Capabilities() marketdata.ProviderCapabilities {
	return marketdata.ProviderCapabilities{FXRates: true, HistoricalFXRates: true}
}
func (p recordingFXProvider) FetchFXRates(ctx context.Context, request marketdata.FXRateRequest) ([]marketdata.FXRateObservation, error) {
	for _, quote := range request.QuoteCodes {
		*p.requests = append(*p.requests, request.BaseCode+"/"+quote+" "+request.Date)
	}
	return fakeFXProvider{code: "frankfurter", rates: map[string]fakeFXRate{"EUR/USD": {value: 1}, "USD/EUR": {value: 1}}}.FetchFXRates(ctx, request)
}

// PR #539 review: the holding trigger reads every instrument version, so an
// obsolete version's quote currency still wakes coverage at the holding's
// date. That wakeup is only a hint: the sweep must take the start date from
// FXDemand, never reach back to the hint's earlier date for a currency
// demanded later by something else — in the first chunk or in any
// continuation the chunk budget hands on, for either pair direction.
func TestFXHoldingWakeupCannotBackdateOtherDemand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newHomeFixture(t)
	usd := seedTestCurrencyCommodity(t, f.database, "USD")
	instrumentID := createTestInstrument(t, f, &usd)
	updateTestInstrumentQuote(t, f, instrumentID, f.eurCommodityID, "2026-06-01")
	f.move(t, "2026-10-14", f.cashAccountID, usd, 100)
	f.buyTenWithEUR(t, "2026-09-28")

	demand, found := demandFor(t, f, usd)
	require.True(t, found)
	require.Equal(t, "2026-10-14", demand.StartDate, "USD is demanded only by the later USD balance")
	var raw string
	require.NoError(t, f.database.QueryRow(`SELECT payload_json FROM background_work_items
		WHERE kind = 'pricing.fx_coverage' AND json_extract(payload_json, '$.currency_id') = ?
		  AND json_extract(payload_json, '$.start_date') = '2026-09-28'`, usd).Scan(&raw),
		"the obsolete version's currency still wakes coverage at the holding's date")
	var payload fxCoverageWorkPayload
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))

	requests := []string{}
	service := NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry(recordingFXProvider{requests: &requests}))
	service.SetNowForTest(func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) })
	_, err := f.database.Exec(`UPDATE market_data_sources SET status = 'disabled' WHERE kind = 'provider' AND code <> 'frankfurter'`)
	require.NoError(t, err)
	source := marketDataSourceIDByCode(t, f.database, "frankfurter")
	// One provider attempt per chunk: every later date and the second pair
	// direction are reached only through continuations.
	_, err = service.SavePolicy(ctx, PricingPolicyInput{OwnerUserID: 1, DefaultSourceID: &source, MaxBackfillDays: 1})
	require.NoError(t, err)

	chunks := 0
	for ; chunks < 20; chunks++ {
		_, successors, err := service.runFXCoverage(ctx, 1, payload)
		require.NoError(t, err)
		if len(successors) == 0 {
			break
		}
		require.Len(t, successors, 1)
		payload = fxCoverageWorkPayload{}
		require.NoError(t, json.Unmarshal([]byte(successors[0].PayloadJSON), &payload))
		assert.Empty(t, payload.StartDate, "a continuation must not carry the hint's date as a start")
	}
	require.Less(t, chunks, 20, "the sweep finishes")
	assert.Greater(t, chunks, 1, "the budget forces continuations")
	assert.ElementsMatch(t, []string{"EUR/USD 2026-10-14", "EUR/USD 2026-10-15", "USD/EUR 2026-10-14", "USD/EUR 2026-10-15"}, requests,
		"both directions are covered from the demand date, never from the hint's 2026-09-28")
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPriceCoefficientsRemainExactAboveBrowserSafeInteger(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity,
		PriceValue: 9007199254740993, PriceScale: 18, BaseQuantityValue: 9007199254740995, BaseQuantityScale: 18,
		ValuationDate: "2026-06-12", IsManual: true, Derivation: json.RawMessage(`{"legs":[{"price_value":9007199254740993}]}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	assert.JSONEq(t, `"9007199254740993"`, string(body["price_value"]))
	assert.JSONEq(t, `"9007199254740995"`, string(body["base_quantity_value"]))
	var derivationText string
	require.NoError(t, json.Unmarshal(body["derivation_text"], &derivationText))
	assert.Equal(t, `{"legs":[{"price_value":9007199254740993}]}`, derivationText)
}

func TestCreatePriceRejectsUnreviewedSupersession(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	old := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, PriceValue: 100, PriceScale: 2,
		ValuationDate: "2026-06-12", IsManual: true, Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created priceObservationResponse
	require.NoError(t, json.Unmarshal(old.Body.Bytes(), &created))
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, PriceValue: 110, PriceScale: 2,
		ValuationDate: "2026-06-12", IsManual: true, Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`), SupersedesObservationID: &created.ID,
	}, http.StatusBadRequest)
}

func manualPricingInput(f pricingAPITestFixture) manualPriceRequest {
	return manualPriceRequest{BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, ValuationDate: "2026-06-12", PriceValue: "9007199254740993", PriceScale: 18, BaseQuantityValue: "1", Reason: "statement fixing", Reference: "statement line 4", ObservedAt: "2026-06-12T12:00:00Z"}
}

func TestManualPriceAndCorrectionHTTPPreserveExactProvenanceAndNoLedgerWrites(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	var auditsBefore, postingsBefore int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditsBefore))
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM posting_versions`).Scan(&postingsBefore))
	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", manualPricingInput(f), http.StatusCreated)
	var original priceObservationResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &original))
	assert.Equal(t, "9007199254740993", original.PriceValue)
	require.NotNil(t, original.SourceID)
	assert.True(t, original.IsManual)
	preview := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/prices/"+itoa(original.ID)+"/impact", nil, http.StatusOK)
	var impact priceImpactResponse
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &impact))
	require.Len(t, impact.Prices, 1)
	correction := manualPricingInput(f)
	correction.PriceValue = "9007199254740995"
	correction.Reason = "correct statement line"
	correction.ExpectedImpact = impact.Token
	res = doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+itoa(original.ID)+"/correct", correction, http.StatusCreated)
	var result correctPriceResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	assert.Equal(t, "9007199254740995", result.Replacement.PriceValue)
	require.NotNil(t, result.Replacement.SupersedesObservationID)
	assert.Equal(t, original.ID, *result.Replacement.SupersedesObservationID)
	require.Len(t, result.Retired, 1)
	assert.Equal(t, original.PriceValue, result.Retired[0].PriceValue)
	var auditsAfter, postingsAfter int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditsAfter))
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM posting_versions`).Scan(&postingsAfter))
	assert.Equal(t, auditsBefore+2, auditsAfter)
	assert.Equal(t, postingsBefore, postingsAfter)
	var sourceKind string
	require.NoError(t, database.QueryRow(`SELECT kind FROM market_data_sources WHERE id=?`, *result.Replacement.SourceID).Scan(&sourceKind))
	assert.Equal(t, "manual", sourceKind)
}

func TestManualPriceInvalidInputAndMutationProtection(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	var before int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&before))
	for _, value := range []string{"0", "-1", "01", "1.5", "1e3", ""} {
		input := manualPricingInput(f)
		input.PriceValue = value
		doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusBadRequest)
	}
	input := manualPricingInput(f)
	input.Reason = " "
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusBadRequest)
	input = manualPricingInput(f)
	input.BaseCommodityID = input.QuoteCommodityID
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusBadRequest)
	doPricingRequest(t, handler, nil, "", http.MethodGet, "/api/v1/pricing/workspace", nil, http.StatusUnauthorized)
	doPricingRequest(t, handler, nil, "", http.MethodGet, "/api/v1/pricing/history?base_commodity_id=1&quote_commodity_id=2", nil, http.StatusUnauthorized)
	doPricingRequest(t, handler, nil, "", http.MethodGet, "/api/v1/pricing/prices/1/impact", nil, http.StatusUnauthorized)
	doPricingRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/pricing/manual", manualPricingInput(f), http.StatusForbidden)
	doPricingRequest(t, handler, f.sessionCookie, "", http.MethodPost, "/api/v1/pricing/prices/1/correct", manualPricingInput(f), http.StatusForbidden)
	var after int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after)
}

func TestPriceWorkspaceDatedCoverageExcludesFutureAndVoidedPrices(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	input := manualPricingInput(f)
	input.ValuationDate = "2026-06-20"
	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusCreated)
	var future priceObservationResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &future))
	get := func() pricingWorkspaceResponse {
		res := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/workspace?as_of=2026-06-12", nil, http.StatusOK)
		var workspace pricingWorkspaceResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &workspace))
		return workspace
	}
	workspace := get()
	assert.Equal(t, 3, workspace.StalenessMaxDays, "the default pricing policy")
	assert.Equal(t, 7, workspace.ValuationWindowDays, "the reports' window, which also bounds coverage")
	require.Len(t, workspace.Coverage, 1)
	assert.Equal(t, "missing", workspace.Coverage[0].Status)
	assert.Nil(t, workspace.Coverage[0].Observation)
	input.ValuationDate = "2026-06-01"
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusCreated)
	workspace = get()
	assert.Equal(t, "stale", workspace.Coverage[0].Status)
	require.NotNil(t, workspace.Coverage[0].AgeDays)
	assert.Equal(t, 11, *workspace.Coverage[0].AgeDays)
	input.ValuationDate = "2026-06-12"
	res = doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusCreated)
	var current priceObservationResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &current))
	workspace = get()
	assert.Equal(t, "usable", workspace.Coverage[0].Status)
	assert.Equal(t, current.ID, workspace.Coverage[0].Observation.ID)
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+itoa(current.ID)+"/void", priceObservationVoidRequest{VoidReason: "withdraw"}, http.StatusOK)
	workspace = get()
	assert.Equal(t, "stale", workspace.Coverage[0].Status)
	doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/workspace?as_of=2026-02-30", nil, http.StatusBadRequest)
}

func TestPriceImpactStaleHTTPAndOverflowLeaveEvidenceUnchanged(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	input := manualPricingInput(f)
	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusCreated)
	var root priceObservationResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &root))
	preview := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/prices/"+itoa(root.ID)+"/impact", nil, http.StatusOK)
	var impact priceImpactResponse
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &impact))
	// A real new derived observation joins the previewed dependency closure.
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, PriceValue: 123, PriceScale: 2, ValuationDate: "2026-06-12", IsDerived: true, Derivation: json.RawMessage(`{"legs":[{"observation_id":` + itoa(root.ID) + `}]}`), Metadata: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`)}, http.StatusCreated)
	input.ExpectedImpact = impact.Token
	refusal := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+itoa(root.ID)+"/correct", input, http.StatusConflict)
	assert.Contains(t, refusal.Body.String(), "PRICE_IMPACT_STALE")
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+itoa(root.ID)+"/void", priceObservationVoidRequest{VoidReason: "withdraw", ExpectedImpact: impact.Token}, http.StatusConflict)
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM price_observations WHERE voided_at IS NOT NULL`).Scan(&count))
	assert.Zero(t, count)
	input = manualPricingInput(f)
	input.PriceValue = "9223372036854775808"
	overflow := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusUnprocessableEntity)
	assert.Contains(t, overflow.Body.String(), "LEDGER_OVERFLOW")
	input = manualPricingInput(f)
	input.BaseCommodityID = 999999
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", input, http.StatusBadRequest)
}

// A derived price must name sources the void cascade can follow (#104): a leg
// pointing at no observation is a validation error, and nothing is written.
func TestCreateDerivedPriceRejectsALegNamingNoObservation(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	source := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, PriceValue: 100, PriceScale: 2,
		ValuationDate: "2026-06-12", IsManual: true, Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created priceObservationResponse
	require.NoError(t, json.Unmarshal(source.Body.Bytes(), &created))
	var before int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&before))

	derived := func(legID int64) priceObservationRequest {
		return priceObservationRequest{
			BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, PriceValue: 101, PriceScale: 2,
			ValuationDate: "2026-06-13", IsDerived: true,
			Derivation:     json.RawMessage(fmt.Sprintf(`{"legs":[{"observation_id":%d}]}`, legID)),
			SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
		}
	}
	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", derived(created.ID+1000), http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "VALIDATION_FAILED")
	var after int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM price_observations`).Scan(&after))
	assert.Equal(t, before, after)

	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", derived(created.ID), http.StatusCreated)
	var links int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM price_observation_dependencies WHERE source_observation_id = ?`, created.ID).Scan(&links))
	assert.Equal(t, 1, links)
}

// R13 slice 7: the manual entry route records a total-return benchmark
// series. It feeds only a benchmark declared total-return, never the
// portfolio's own valuation, and a correction cannot relabel its basis.
func TestManualTotalReturnPriceFeedsOnlyATotalReturnBenchmark(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "TRI")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 4000), http.StatusCreated)
	entry := func(date, price, basis string) manualPriceRequest {
		return manualPriceRequest{BaseCommodityID: instrument.CommodityID, QuoteCommodityID: f.commodityID, ValuationDate: date,
			PriceValue: price, PriceScale: 0, BaseQuantityValue: "1", AdjustmentBasis: basis, Reason: "index factsheet"}
	}
	var opening priceObservationResponse
	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", entry("2026-01-31", "100", "total_return"), http.StatusCreated)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &opening))
	assert.Equal(t, "total_return", opening.AdjustmentBasis)
	assert.Equal(t, "manual", opening.QuoteType)
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", entry("2026-02-01", "110", "total_return"), http.StatusCreated)

	path := fmt.Sprintf("/api/v1/investments/returns?from=2026-02-01&to=2026-02-01&reporting_currency_id=%d&benchmark_commodity_id=%d", f.commodityID, instrument.CommodityID)
	read := func(basis string) investmentReturnsResponse {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, path+"&benchmark_basis="+basis, nil, http.StatusOK)
		var body investmentReturnsResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		require.NotNil(t, body.Benchmark)
		return body
	}
	total := read("total_return")
	require.NotNil(t, total.Benchmark.TimeWeighted.Value, "100 → 110 on the declared basis")
	assert.Equal(t, "0.100000", *total.Benchmark.TimeWeighted.Value)
	// The trade-implied price still values the holding; 110 is not a raw quote.
	require.NotNil(t, total.TimeWeighted.Value)
	assert.Equal(t, "0.000000", *total.TimeWeighted.Value)
	price := read("price_return")
	assert.Nil(t, price.Benchmark.TimeWeighted.Value, "total-return observations never price a price-return benchmark")

	// A correction keeps the basis it replaces and refuses another.
	preview := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/prices/"+itoa(opening.ID)+"/impact", nil, http.StatusOK)
	var impact priceImpactResponse
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &impact))
	relabel := entry("2026-01-31", "100", "raw")
	relabel.ExpectedImpact = impact.Token
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+itoa(opening.ID)+"/correct", relabel, http.StatusBadRequest)
	correction := entry("2026-01-31", "88", "")
	correction.ExpectedImpact = impact.Token
	res = doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices/"+itoa(opening.ID)+"/correct", correction, http.StatusCreated)
	var corrected correctPriceResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &corrected))
	assert.Equal(t, "total_return", corrected.Replacement.AdjustmentBasis)
	total = read("total_return")
	require.NotNil(t, total.Benchmark.TimeWeighted.Value)
	assert.Equal(t, "0.250000", *total.Benchmark.TimeWeighted.Value, "88 → 110")

	// Only raw and total-return are recordable, and a currency is never total-return.
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", entry("2026-02-02", "100", "split_adjusted"), http.StatusBadRequest)
	gbp := createCurrencyForSession(t, handler, f.sessionCookie, f.csrfToken, `{"code":"GBP"}`)
	currency := entry("2026-02-02", "1", "total_return")
	currency.BaseCommodityID = gbp.ID
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/manual", currency, http.StatusBadRequest)
}

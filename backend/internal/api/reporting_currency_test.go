package api

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reporting-currency selector: one currency, a named method, and
// per-commodity exact totals kept in every response. Conversion is additive —
// these tests check that as hard as they check the arithmetic.

type reportingFixture struct {
	exportFixture
	eurID int64
}

func newReportingFixture(t *testing.T, handler http.Handler) reportingFixture {
	t.Helper()

	base := newExportFixture(t, handler)
	eur := createCurrencyForSession(t, handler, base.sessionCookie, base.csrfToken, `{"code":"EUR","name":"Euro"}`)
	return reportingFixture{exportFixture: base, eurID: eur.ID}
}

// recordRate stores a USD→EUR observation: price per one unit of the base.
func recordRate(t *testing.T, handler http.Handler, f reportingFixture, date string, priceValue int64, priceScale int) {
	t.Helper()

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices",
		map[string]any{
			"base_commodity_id":  f.usdID,
			"quote_commodity_id": f.eurID,
			"valuation_date":     date,
			"price_value":        priceValue,
			"price_scale":        priceScale,
			"quote_type":         "manual",
			"is_manual":          true,
		}, http.StatusCreated)
}

func readSpending(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, query string) spendingResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/spending"+query, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var response spendingResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	return response
}

// A flow is converted at the date it happened, then summed — not at one
// range-end rate, which would misattribute everything before a mid-period move.
func TestSpendingConvertsEachPostingAtItsOwnEntryDate(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	// The rate halves mid-period: 1 USD = 1.00 EUR on the 1st, 0.50 on the 20th.
	recordRate(t, handler, f, "2026-06-01", 1000000, 6)
	recordRate(t, handler, f, "2026-06-20", 500000, 6)

	// 100.00 spent on each side of the move.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, -10000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-21",
		posting(f.checking.ID, -10000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
	), http.StatusCreated)

	report := readSpending(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&reporting_currency_id="+strconvFormatInt(f.eurID))

	require.NotNil(t, report.ConvertedTotal)
	// 100.00 at 1.00 plus 100.00 at 0.50 = 150.00, not 200.00 (all at the first
	// rate) and not 100.00 (all at the last).
	assert.Equal(t, "15000", report.ConvertedTotal.QuantityValue.String())
	assert.Equal(t, 2, report.ConvertedTotal.QuantityScale)
	assert.Equal(t, f.eurID, report.ConvertedTotal.CommodityID)

	// The exact per-commodity total is untouched: conversion is additive.
	require.Len(t, report.CommodityTotals, 1)
	assert.Equal(t, f.usdID, report.CommodityTotals[0].CommodityID)
	assert.Equal(t, "20000", report.CommodityTotals[0].QuantityValue.String())
}

// Asking for no reporting currency must leave the response exactly as it was.
func TestSpendingWithoutAReportingCurrencyIsUnchanged(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, -10000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
	), http.StatusCreated)

	report := readSpending(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category")

	assert.Nil(t, report.ConvertedTotal, "no reporting currency means no converted figure")
	assert.Nil(t, report.Valuation)
	require.NotEmpty(t, report.Groups)
	assert.Nil(t, report.Groups[0].Converted)
	require.Len(t, report.CommodityTotals, 1)
}

// A date with no observation falls back to the nearest earlier one inside the
// window, and the response says the rate was stale rather than pretending.
func TestSpendingFallsBackToTheNearestEarlierRateAndSaysSo(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	recordRate(t, handler, f, "2026-06-01", 800000, 6)

	// Spent four days later: no observation that day, one inside the window.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(f.checking.ID, -10000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
	), http.StatusCreated)

	report := readSpending(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&reporting_currency_id="+strconvFormatInt(f.eurID))

	require.NotNil(t, report.ConvertedTotal)
	assert.Equal(t, "8000", report.ConvertedTotal.QuantityValue.String())

	require.NotNil(t, report.Valuation)
	assert.True(t, report.Valuation.Complete)
	assert.Equal(t, "observed_on_or_before", report.Valuation.Method)
	assert.Equal(t, 7, report.Valuation.MaxStalenessDays)
	require.Len(t, report.Valuation.RatesUsed, 1)
	assert.Equal(t, "2026-06-01", report.Valuation.RatesUsed[0].ObservationDate)
	assert.Equal(t, "2026-06-05", report.Valuation.RatesUsed[0].RequestedDate)
	assert.True(t, report.Valuation.RatesUsed[0].Stale, "a rate from another day must be labelled as one")
}

// Nothing in the window: the figure is omitted and the commodity is named. A
// partial total that looks whole is the one outcome that must not happen.
func TestSpendingOmitsTheConvertedTotalWhenARateIsMissing(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	// An observation far outside the staleness window.
	recordRate(t, handler, f, "2026-01-02", 900000, 6)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-05",
		posting(f.checking.ID, -10000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
	), http.StatusCreated)

	report := readSpending(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&reporting_currency_id="+strconvFormatInt(f.eurID))

	assert.Nil(t, report.ConvertedTotal, "an incomplete total is not reported as a number")
	require.NotEmpty(t, report.Groups)
	assert.Nil(t, report.Groups[0].Converted)

	require.NotNil(t, report.Valuation)
	assert.False(t, report.Valuation.Complete)
	require.Len(t, report.Valuation.Gaps, 1)
	assert.Equal(t, f.usdID, report.Valuation.Gaps[0].CommodityID)
	assert.Equal(t, "no_observation_in_window", report.Valuation.Gaps[0].Reason)
	assert.Equal(t, "2026-01-02", report.Valuation.Gaps[0].NearestObservationDate,
		"'no rate' and 'no rate recently' are different problems and need different fixes")

	// The exact figures are still there: a missing rate costs the conversion,
	// not the report.
	require.Len(t, report.CommodityTotals, 1)
	assert.Equal(t, "10000", report.CommodityTotals[0].QuantityValue.String())
}

// A voided observation is a statement the book withdrew, and must not drive a
// headline figure (T-37 is the prerequisite this feature was waiting on).
func TestSpendingIgnoresVoidedRates(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	recordRate(t, handler, f, "2026-06-01", 1000000, 6)
	res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodGet,
		"/api/v1/pricing/prices?base_commodity_id="+strconvFormatInt(f.usdID)+"&quote_commodity_id="+strconvFormatInt(f.eurID),
		nil, http.StatusOK)
	var listed struct {
		Prices []struct {
			ID int64 `json:"id"`
		} `json:"prices"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &listed))
	require.NotEmpty(t, listed.Prices)

	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/pricing/prices/"+strconvFormatInt(listed.Prices[0].ID)+"/void",
		map[string]any{"void_reason": "wrong rate", "change_reason": "corrected by hand"}, http.StatusOK)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, -10000, 2, f.usdID),
		posting(f.groceries.ID, 10000, 2, f.usdID),
	), http.StatusCreated)

	report := readSpending(t, handler, f.sessionCookie,
		"?start_date=2026-06-01&end_date=2026-06-30&group_by=category&reporting_currency_id="+strconvFormatInt(f.eurID))

	assert.Nil(t, report.ConvertedTotal, "a withdrawn rate must not convert anything")
	require.NotNil(t, report.Valuation)
	assert.False(t, report.Valuation.Complete)
}

func TestSpendingRejectsAnInvalidReportingCurrency(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	for query, want := range map[string]string{
		"&reporting_currency_id=98765": "reporting currency is invalid",
		"&reporting_currency_id=abc":   "reporting_currency_id",
		"&max_staleness_days=-1&reporting_currency_id=" + strconvFormatInt(f.eurID): "max_staleness_days",
	} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category"+query, nil)
		req.AddCookie(f.sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)

		require.Equalf(t, http.StatusBadRequest, res.Code, "query %q: %s", query, res.Body.String())
		assert.Contains(t, res.Body.String(), want, query)
	}
}

// R2's headline guarantee is that net_movement = operating_net + transfer_net
// is an identity, not an approximation. Conversion must not quietly turn it
// into one: per-posting rounding is not additive, so the converted net movement
// is derived from the converted parts rather than accumulated separately.
func TestCashflowConvertedIdentityHoldsExactly(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)
	savings := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "Savings", "asset", "savings", f.usdID, 2)

	// A rate with an awkward third decimal, so per-posting rounding actually
	// bites rather than dividing evenly.
	recordRate(t, handler, f, "2026-06-01", 333333, 6)

	// Amounts chosen so the parts round differently from their sum.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-02",
		posting(f.checking.ID, 333, 2, f.usdID),
		posting(f.salary.ID, -333, 2, f.usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-03",
		posting(f.checking.ID, -167, 2, f.usdID),
		posting(f.groceries.ID, 167, 2, f.usdID),
	), http.StatusCreated)
	// A transfer out of the cash scope: financing movement, never spending.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-04",
		posting(f.checking.ID, -99, 2, f.usdID),
		posting(savings.ID, 99, 2, f.usdID),
	), http.StatusCreated)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/cashflow?start_date=2026-06-01&end_date=2026-06-30&bucket=month"+
			"&account_id="+strconvFormatInt(f.checking.ID)+
			"&reporting_currency_id="+strconvFormatInt(f.eurID), nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var report cashflowResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &report))
	require.NotEmpty(t, report.Buckets)

	bucket := report.Buckets[0]
	require.NotNil(t, bucket.ConvertedOperatingNet, "the fixture must convert cleanly")
	require.NotNil(t, bucket.ConvertedTransferNet)
	require.NotNil(t, bucket.ConvertedNetMovement)

	operating, ok := new(big.Int).SetString(bucket.ConvertedOperatingNet.QuantityValue.String(), 10)
	require.True(t, ok)
	transfer, ok := new(big.Int).SetString(bucket.ConvertedTransferNet.QuantityValue.String(), 10)
	require.True(t, ok)
	net, ok := new(big.Int).SetString(bucket.ConvertedNetMovement.QuantityValue.String(), 10)
	require.True(t, ok)

	assert.Equal(t, new(big.Int).Add(operating, transfer).String(), net.String(),
		"converted net movement must equal operating plus transfer, exactly")

	// The two legs behind each net are reported as well, so a reader can see
	// what the net is made of without a second request — and so the same
	// identity is checkable one level down.
	require.NotNil(t, bucket.ConvertedInflow)
	require.NotNil(t, bucket.ConvertedOutflow)
	require.NotNil(t, bucket.ConvertedTransferIn)
	require.NotNil(t, bucket.ConvertedTransferOut)

	difference := func(left, right *balanceQuantityResponse) string {
		leftValue, ok := new(big.Int).SetString(left.QuantityValue.String(), 10)
		require.True(t, ok)
		rightValue, ok := new(big.Int).SetString(right.QuantityValue.String(), 10)
		require.True(t, ok)
		return new(big.Int).Sub(leftValue, rightValue).String()
	}
	assert.Equal(t, difference(bucket.ConvertedInflow, bucket.ConvertedOutflow), operating.String(),
		"converted operating net must equal converted inflow minus outflow, exactly")
	assert.Equal(t, difference(bucket.ConvertedTransferIn, bucket.ConvertedTransferOut), transfer.String(),
		"converted transfer net must equal converted transfer in minus out, exactly")

	// And the exact per-commodity identity is untouched.
	require.NotEmpty(t, bucket.NetMovement)
	assert.Equal(t, f.usdID, bucket.NetMovement[0].CommodityID)
}

// Net worth is a stock: each bucket converts at its own end date, so a series
// spanning a rate change shows the change.
func TestNetWorthConvertsEachBucketAtItsOwnEndDate(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newReportingFixture(t, handler)

	recordRate(t, handler, f, "2026-06-30", 1000000, 6)
	recordRate(t, handler, f, "2026-07-31", 500000, 6)

	// 200.00 held from June onwards; nothing else changes.
	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 20000, 2, f.usdID),
		posting(f.salary.ID, -20000, 2, f.usdID),
	), http.StatusCreated)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/reports/net-worth?start_date=2026-06-01&end_date=2026-07-31&bucket=month"+
			"&reporting_currency_id="+strconvFormatInt(f.eurID), nil)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var report netWorthSeriesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &report))
	require.Len(t, report.Buckets, 2)

	require.NotNil(t, report.Buckets[0].Converted)
	require.NotNil(t, report.Buckets[1].Converted)
	// Same holding, different rate: the converted series moves even though the
	// exact one does not.
	assert.Equal(t, "20000", report.Buckets[0].Converted.QuantityValue.String())
	assert.Equal(t, "10000", report.Buckets[1].Converted.QuantityValue.String())

	assert.Equal(t, report.Buckets[0].Totals[0].QuantityValue.String(),
		report.Buckets[1].Totals[0].QuantityValue.String(),
		"the exact holding is unchanged; only its valuation moved")
}

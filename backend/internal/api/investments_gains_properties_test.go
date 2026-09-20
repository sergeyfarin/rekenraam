package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Verify the public response, not only the service arithmetic: a correctly
// calculated coefficient is still the wrong money if its scale is omitted or
// the total groups representations rather than currencies.
func TestFinancialGainsHTTPPreservesScaleAndSumsOneTotalPerCurrency(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "PRECISION")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	for i, trade := range []struct {
		cost          int64
		costScale     int
		proceeds      int64
		proceedsScale int
	}{{1099, 2, 11, 0}, {10, 0, 30, 0}, {1102, 2, 1100, 2}} {
		buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", trade.cost)
		buy.TransactionDate = fmt.Sprintf("2026-02-%02d", 2*i+1)
		buy.CashAmountScale = trade.costScale
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
		sale := buy
		sale.TransactionDate = fmt.Sprintf("2026-02-%02d", 2*i+2)
		sale.CashAmountValue = trade.proceeds
		sale.CashAmountScale = trade.proceedsScale
		doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	}
	for _, tc := range []struct {
		name, query, total string
		count              int
	}{{"whole period", "", "19.99", 3}, {"only first gain", "?from=2026-02-02&to=2026-02-02", "0.01", 1}, {"loss only", "?from=2026-02-06&to=2026-02-06", "-0.02", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains"+tc.query, nil, http.StatusOK)
			var result investmentGainsResponse
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
			require.Len(t, result.Realized, tc.count)
			require.Len(t, result.RealizedTotals, 1)
			total := result.RealizedTotals[0]
			require.Equal(t, f.commodityID, total.CostCommodityID)
			got, ok := new(big.Rat).SetString(fmt.Sprintf("%de-%d", total.TotalGainValue, total.TotalGainScale))
			require.True(t, ok)
			want, ok := new(big.Rat).SetString(tc.total)
			require.True(t, ok)
			require.Zero(t, got.Cmp(want))
			expected := map[string]string{"2026-02-02": "0.01", "2026-02-04": "20", "2026-02-06": "-0.02"}
			for _, e := range result.Realized {
				got, ok := new(big.Rat).SetString(fmt.Sprintf("%de-%d", e.RealizedGainValue, e.RealizedGainScale))
				require.True(t, ok)
				want, ok := new(big.Rat).SetString(expected[e.DisposalDate])
				require.True(t, ok)
				require.Zero(t, got.Cmp(want))
			}
			var raw struct {
				Realized []map[string]json.RawMessage `json:"realized"`
			}
			require.NoError(t, json.Unmarshal(res.Body.Bytes(), &raw))
			for _, entry := range raw.Realized {
				require.Contains(t, entry, "realized_gain_scale", "zero scale must be explicit too")
			}
		})
	}
	// A second currency must remain a second total, never be added numerically
	// to the first one. The fixture's original reporting currency is USD.
	euro := createCurrencyForSession(t, handler, f.sessionCookie, f.csrfToken, `{"code":"EUR","name":"Euro"}`)
	euroCash := createLedgerAccount(t, handler, f.sessionCookie, f.csrfToken, "EUR cash", "asset", "checking", euro.ID, 2)
	other := createInstrumentForSession(t, handler, f, "OTHER")
	otherHolding := createHoldingAccountForSession(t, handler, f, other.ID)
	buy := tradeRequestBody(f, otherHolding.ID, other.CommodityID, "1", 1000)
	buy.TransactionDate = "2026-02-07"
	buy.CashAccountID, buy.CashCommodityID = euroCash.ID, euro.ID
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	sale := buy
	sale.TransactionDate, sale.CashAmountValue = "2026-02-08", 900
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	var result investmentGainsResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.Len(t, result.RealizedTotals, 2)
	currencies := map[int64]currencyResponse{}
	for _, currency := range result.Currencies {
		currencies[currency.ID] = currency
	}
	require.Equal(t, "EUR", currencies[euro.ID].Code)
	require.Equal(t, 2, currencies[euro.ID].StandardScale)
	require.Equal(t, "USD", currencies[f.commodityID].Code)
	require.Equal(t, 2, currencies[f.commodityID].StandardScale)
	wantByCurrency := map[int64]*big.Rat{f.commodityID: big.NewRat(1999, 100), euro.ID: big.NewRat(-1, 1)}
	for _, total := range result.RealizedTotals {
		want, exists := wantByCurrency[total.CostCommodityID]
		require.True(t, exists, "unknown or duplicated currency total")
		got, ok := new(big.Rat).SetString(fmt.Sprintf("%de-%d", total.TotalGainValue, total.TotalGainScale))
		require.True(t, ok)
		require.Zero(t, got.Cmp(want))
		delete(wantByCurrency, total.CostCommodityID)
	}
	require.Empty(t, wantByCurrency)

}

// A representability limit is a rejected command, not a successful write that
// turns the gains endpoint into a 500 on the next read.
func TestFinancialUnrepresentableAcquisitionIsValidationError(t *testing.T) {
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "RANGE")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "3", 1000)
	buy.TransactionDate = "2026-02-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusCreated)
	sale := buy
	sale.TransactionDate = "2026-02-02"
	sale.QuantityValue = "1"
	sale.CashAmountValue = 500
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)
	before := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	buy.TransactionDate = "2026-02-03"
	buy.QuantityValue = "1"
	buy.CashAmountValue = 1_000_000_000_000_000
	rejected := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", buy, http.StatusBadRequest)
	require.Contains(t, rejected.Body.String(), "VALIDATION_FAILED")
	after := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	require.JSONEq(t, before.Body.String(), after.Body.String())
}

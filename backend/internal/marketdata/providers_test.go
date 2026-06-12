package marketdata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECBReferenceRatesProviderParsesDailyXML(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/eurofxref-daily.xml", r.URL.Path)
		return `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
  <Cube>
    <Cube time="2026-06-12">
      <Cube currency="USD" rate="1.1567"/>
      <Cube currency="JPY" rate="185.30"/>
    </Cube>
  </Cube>
</gesmes:Envelope>`
	})

	provider := NewECBReferenceRatesProvider(client, "https://example.test")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "EUR", QuoteCodes: []string{"USD"}})
	require.NoError(t, err)

	require.Len(t, rates, 1)
	assert.Equal(t, ECBReferenceRatesCode, rates[0].SourceCode)
	assert.Equal(t, "EUR", rates[0].BaseCode)
	assert.Equal(t, "USD", rates[0].QuoteCode)
	assert.Equal(t, int64(11567), rates[0].PriceValue)
	assert.Equal(t, 4, rates[0].PriceScale)
	assert.Equal(t, "2026-06-12", rates[0].ValuationDate)
	assert.Equal(t, "official_fixing", rates[0].QuoteType)
}

func TestECBReferenceRatesRejectsNonEURBase(t *testing.T) {
	provider := NewECBReferenceRatesProvider(nil, "")
	_, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"EUR"}})
	require.ErrorIs(t, err, ErrUnsupportedPair)
}

func TestFrankfurterProviderParsesRates(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/rates", r.URL.Path)
		assert.Equal(t, "USD", r.URL.Query().Get("base"))
		assert.Equal(t, "EUR,GBP", r.URL.Query().Get("quotes"))
		return `{"base":"USD","date":"2026-06-12","rates":{"EUR":0.8645,"GBP":0.74612}}`
	})

	provider := NewFrankfurterProvider(client, "https://example.test")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"EUR", "GBP"}})
	require.NoError(t, err)

	require.Len(t, rates, 2)
	assert.Equal(t, FrankfurterCode, rates[0].SourceCode)
	assert.Equal(t, "USD", rates[0].BaseCode)
	assert.Equal(t, "2026-06-12", rates[0].ValuationDate)
	assert.Equal(t, int64(8645), rates[0].PriceValue)
	assert.Equal(t, 4, rates[0].PriceScale)
}

func TestExchangeRateAPIOpenAccessProviderParsesRates(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/latest/USD", r.URL.Path)
		return `{
			"result":"success",
			"base_code":"USD",
			"time_last_update_utc":"Fri, 12 Jun 2026 00:06:37 +0000",
			"rates":{"USD":1,"EUR":0.8645,"ZAR":17.7723}
		}`
	})

	provider := NewExchangeRateAPIOpenAccessProvider(client, "https://example.test")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"ZAR"}})
	require.NoError(t, err)

	require.Len(t, rates, 1)
	assert.Equal(t, ExchangeRateAPIOpenAccessCode, rates[0].SourceCode)
	assert.Equal(t, "ZAR", rates[0].QuoteCode)
	assert.Equal(t, int64(177723), rates[0].PriceValue)
	assert.Equal(t, 4, rates[0].PriceScale)
	assert.Equal(t, "2026-06-12T00:06:37Z", rates[0].SourcePublishedAt)
	assert.Equal(t, "2026-06-12", rates[0].ValuationDate)
}

func TestOpenExchangeRatesProviderRequiresAppID(t *testing.T) {
	provider := NewOpenExchangeRatesProvider(nil, "", "")
	_, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"EUR"}})
	require.ErrorIs(t, err, ErrMissingSecret)
}

func TestOpenExchangeRatesProviderParsesRates(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/latest.json", r.URL.Path)
		assert.Equal(t, "secret", r.URL.Query().Get("app_id"))
		return `{"timestamp":1781222400,"base":"USD","rates":{"EUR":0.8645,"JPY":185.30}}`
	})

	provider := NewOpenExchangeRatesProvider(client, "https://example.test", "secret")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"EUR"}})
	require.NoError(t, err)

	require.Len(t, rates, 1)
	assert.Equal(t, OpenExchangeRatesFreeCode, rates[0].SourceCode)
	assert.Equal(t, "EUR", rates[0].QuoteCode)
	assert.Equal(t, int64(8645), rates[0].PriceValue)
	assert.Equal(t, "2026-06-12T00:00:00Z", rates[0].SourcePublishedAt)
}

func TestDefaultRegistryContainsBuiltInFXProviders(t *testing.T) {
	registry := DefaultRegistry("secret")
	for _, code := range []string{ECBReferenceRatesCode, FrankfurterCode, ExchangeRateAPIOpenAccessCode, OpenExchangeRatesFreeCode} {
		provider, ok := registry.FXRateProvider(code)
		require.True(t, ok, code)
		assert.True(t, provider.Capabilities().FXRates)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testHTTPClient(handler func(*http.Request) string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := handler(request)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    request,
		}, nil
	})}
}

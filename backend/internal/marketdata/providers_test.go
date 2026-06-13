package marketdata

import (
	"bytes"
	"context"
	"errors"
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
	assert.Equal(t, []string{"EUR", "GBP"}, fxQuoteCodes(rates))
	eur := requireFXQuote(t, rates, "EUR")
	assert.Equal(t, FrankfurterCode, eur.SourceCode)
	assert.Equal(t, "USD", eur.BaseCode)
	assert.Equal(t, "2026-06-12", eur.ValuationDate)
	assert.Equal(t, int64(8645), eur.PriceValue)
	assert.Equal(t, 4, eur.PriceScale)
	gbp := requireFXQuote(t, rates, "GBP")
	assert.Equal(t, int64(74612), gbp.PriceValue)
	assert.Equal(t, 5, gbp.PriceScale)
}

func TestFrankfurterProviderUsesRequestedDateAndDeduplicatesQuotes(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/rates", r.URL.Path)
		assert.Equal(t, "2026-06-01", r.URL.Query().Get("date"))
		assert.Equal(t, "EUR,JPY", r.URL.Query().Get("quotes"))
		return `{"base":"USD","date":"2026-06-01","rates":{"JPY":145.32,"EUR":0.91234}}`
	})

	provider := NewFrankfurterProvider(client, "https://example.test")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{
		BaseCode:   " usd ",
		QuoteCodes: []string{"eur", "JPY", "EUR", " "},
		Date:       "2026-06-01",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"EUR", "JPY"}, fxQuoteCodes(rates))
	assert.Equal(t, int64(91234), requireFXQuote(t, rates, "EUR").PriceValue)
	assert.Equal(t, int64(14532), requireFXQuote(t, rates, "JPY").PriceValue)
}

func TestFrankfurterProviderRejectsMalformedRates(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing date", body: `{"base":"USD","rates":{"EUR":0.9}}`},
		{name: "zero rate", body: `{"base":"USD","date":"2026-06-12","rates":{"EUR":0}}`},
		{name: "non numeric rate", body: `{"base":"USD","date":"2026-06-12","rates":{"EUR":"nope"}}`},
		{name: "invalid json", body: `{`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := NewFrankfurterProvider(testHTTPClient(func(*http.Request) string {
				return test.body
			}), "https://example.test")

			_, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"EUR"}})
			require.Error(t, err)
		})
	}
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

func TestExchangeRateAPIOpenAccessProviderRejectsErrorResponse(t *testing.T) {
	provider := NewExchangeRateAPIOpenAccessProvider(testHTTPClient(func(*http.Request) string {
		return `{"result":"error","error-type":"unsupported-code"}`
	}), "https://example.test")

	_, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "XXX", QuoteCodes: []string{"EUR"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported-code")
}

func TestExchangeRateAPIOpenAccessProviderFiltersBaseAndQuotes(t *testing.T) {
	provider := NewExchangeRateAPIOpenAccessProvider(testHTTPClient(func(*http.Request) string {
		return `{
			"result":"success",
			"base_code":"usd",
			"time_last_update_utc":"2026-06-12T00:06:37Z",
			"rates":{"USD":1,"EUR":0.8645,"ZAR":17.7723}
		}`
	}), "https://example.test")

	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"eur"}})
	require.NoError(t, err)

	require.Len(t, rates, 1)
	assert.Equal(t, "EUR", rates[0].QuoteCode)
	assert.Equal(t, "2026-06-12T00:06:37Z", rates[0].SourcePublishedAt)
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

func TestOpenExchangeRatesProviderRejectsNonUSDBase(t *testing.T) {
	provider := NewOpenExchangeRatesProvider(nil, "", "secret")
	_, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "EUR", QuoteCodes: []string{"USD"}})
	require.ErrorIs(t, err, ErrUnsupportedPair)
}

func TestOpenExchangeRatesProviderUsesHistoricalPath(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/historical/2026-06-01.json", r.URL.Path)
		assert.Equal(t, "secret", r.URL.Query().Get("app_id"))
		return `{"timestamp":1780272000,"base":"USD","rates":{"EUR":0.91}}`
	})

	provider := NewOpenExchangeRatesProvider(client, "https://example.test", "secret")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "USD", QuoteCodes: []string{"EUR"}, Date: "2026-06-01"})
	require.NoError(t, err)

	require.Len(t, rates, 1)
	assert.Equal(t, "2026-06-01", rates[0].ValuationDate)
}

func TestECBReferenceRatesProviderUsesHistoricalFileForRequestedDate(t *testing.T) {
	client := testHTTPClient(func(r *http.Request) string {
		assert.Equal(t, "/eurofxref-hist-90d.xml", r.URL.Path)
		return `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
  <Cube>
    <Cube time="2026-06-12"><Cube currency="USD" rate="1.1567"/></Cube>
    <Cube time="2026-06-01"><Cube currency="USD" rate="1.1200"/></Cube>
  </Cube>
</gesmes:Envelope>`
	})

	provider := NewECBReferenceRatesProvider(client, "https://example.test")
	rates, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "EUR", QuoteCodes: []string{"USD"}, Date: "2026-06-01"})
	require.NoError(t, err)

	require.Len(t, rates, 1)
	assert.Equal(t, "2026-06-01", rates[0].ValuationDate)
	assert.Equal(t, int64(11200), rates[0].PriceValue)
}

func TestECBReferenceRatesProviderRejectsMissingHistoricalDate(t *testing.T) {
	provider := NewECBReferenceRatesProvider(testHTTPClient(func(*http.Request) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
  <Cube><Cube time="2026-06-12"><Cube currency="USD" rate="1.1567"/></Cube></Cube>
</gesmes:Envelope>`
	}), "https://example.test")

	_, err := provider.FetchFXRates(context.Background(), FXRateRequest{BaseCode: "EUR", QuoteCodes: []string{"USD"}, Date: "2026-06-01"})
	require.Error(t, err)
}

func TestMarketDataHTTPRejectsNonSuccessStatus(t *testing.T) {
	client := testHTTPClientStatus(http.StatusTooManyRequests, "429 Too Many Requests", func(*http.Request) string {
		return `rate limited`
	})

	_, err := getBytes(context.Background(), client, "https://example.test/rates")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429 Too Many Requests")
}

func TestMarketDataHTTPPropagatesTransportError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}

	_, err := getBytes(context.Background(), client, "https://example.test/rates")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network down")
}

func TestDecimalToScaledRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0", "-1.2", "not-a-number", "1e309"} {
		t.Run(value, func(t *testing.T) {
			_, _, err := decimalToScaled(value)
			require.Error(t, err)
		})
	}
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
	return testHTTPClientStatus(http.StatusOK, "200 OK", handler)
}

func testHTTPClientStatus(statusCode int, status string, handler func(*http.Request) string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := handler(request)
		return &http.Response{
			StatusCode: statusCode,
			Status:     status,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Request:    request,
		}, nil
	})}
}

func fxQuoteCodes(rates []FXRateObservation) []string {
	codes := make([]string, 0, len(rates))
	for _, rate := range rates {
		codes = append(codes, rate.QuoteCode)
	}
	return codes
}

func requireFXQuote(t *testing.T, rates []FXRateObservation, quoteCode string) FXRateObservation {
	t.Helper()
	for _, rate := range rates {
		if rate.QuoteCode == quoteCode {
			return rate
		}
	}
	require.Failf(t, "FX quote not found", "quote=%s", quoteCode)
	return FXRateObservation{}
}

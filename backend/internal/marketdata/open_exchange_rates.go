package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const OpenExchangeRatesFreeCode = "open_exchange_rates_free"

type OpenExchangeRatesProvider struct {
	client  *http.Client
	baseURL string
	appID   string
}

func NewOpenExchangeRatesProvider(client *http.Client, baseURL string, appID string) *OpenExchangeRatesProvider {
	return &OpenExchangeRatesProvider{
		client:  httpClient(client),
		baseURL: trimBaseURL(baseURL, "https://openexchangerates.org/api"),
		appID:   strings.TrimSpace(appID),
	}
}

func (p *OpenExchangeRatesProvider) Code() string {
	return OpenExchangeRatesFreeCode
}

func (p *OpenExchangeRatesProvider) Name() string {
	return "Open Exchange Rates Free"
}

func (p *OpenExchangeRatesProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{FXRates: true}
}

func (p *OpenExchangeRatesProvider) FetchFXRates(ctx context.Context, request FXRateRequest) ([]FXRateObservation, error) {
	if p.appID == "" {
		return nil, ErrMissingSecret
	}
	base, quotes := normalizedCodes(request.BaseCode, request.QuoteCodes)
	if base == "" {
		base = "USD"
	}
	if base != "USD" {
		return nil, fmt.Errorf("%w: Open Exchange Rates free plan uses USD base", ErrUnsupportedPair)
	}
	values := url.Values{}
	values.Set("app_id", p.appID)
	path := "/latest.json"
	if strings.TrimSpace(request.Date) != "" {
		path = "/historical/" + strings.TrimSpace(request.Date) + ".json"
	}
	body, err := getBytes(ctx, p.client, p.baseURL+path+"?"+values.Encode())
	if err != nil {
		return nil, err
	}
	return parseOpenExchangeRates(body, quotes)
}

type openExchangeRatesResponse struct {
	Base      string                 `json:"base"`
	Timestamp int64                  `json:"timestamp"`
	Rates     map[string]json.Number `json:"rates"`
}

func parseOpenExchangeRates(body []byte, quoteCodes []string) ([]FXRateObservation, error) {
	var response openExchangeRatesResponse
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("parse Open Exchange Rates: %w", err)
	}
	base := strings.ToUpper(strings.TrimSpace(response.Base))
	if base == "" {
		base = "USD"
	}
	publishedAt := ""
	valuationDate := ""
	if response.Timestamp > 0 {
		publishedAt = time.Unix(response.Timestamp, 0).UTC().Format(time.RFC3339)
		valuationDate = publishedAt[:10]
	}
	quoteFilter := quoteSet(quoteCodes)
	observations := make([]FXRateObservation, 0, len(response.Rates))
	for quoteCode, rate := range response.Rates {
		quote := strings.ToUpper(strings.TrimSpace(quoteCode))
		if quote == "" || quote == base || (quoteFilter != nil && !quoteFilter[quote]) {
			continue
		}
		value, scale, err := decimalToScaled(rate.String())
		if err != nil {
			return nil, fmt.Errorf("parse Open Exchange Rates %s rate: %w", quote, err)
		}
		observations = append(observations, FXRateObservation{
			SourceCode:            OpenExchangeRatesFreeCode,
			BaseCode:              base,
			QuoteCode:             quote,
			QuoteType:             "official_fixing",
			AdjustmentBasis:       "not_applicable",
			PriceValue:            value,
			PriceScale:            scale,
			BaseQuantityValue:     1,
			BaseQuantityScale:     0,
			ValuationDate:         valuationDate,
			SourcePublishedAt:     publishedAt,
			ProviderSeriesID:      "OXR:" + base + ":" + quote,
			ProviderObservationID: "OXR:" + valuationDate + ":" + base + ":" + quote,
			MetadataJSON:          `{"provider":"Open Exchange Rates Free","requires_secret":true}`,
			RawJSON:               "{}",
		})
	}
	return observations, nil
}

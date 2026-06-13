package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const ExchangeRateAPIOpenAccessCode = "exchangerate_api_open_access"

type ExchangeRateAPIOpenAccessProvider struct {
	client  *http.Client
	baseURL string
}

func NewExchangeRateAPIOpenAccessProvider(client *http.Client, baseURL string) *ExchangeRateAPIOpenAccessProvider {
	return &ExchangeRateAPIOpenAccessProvider{
		client:  httpClient(client),
		baseURL: trimBaseURL(baseURL, "https://open.er-api.com/v6"),
	}
}

func (p *ExchangeRateAPIOpenAccessProvider) Code() string {
	return ExchangeRateAPIOpenAccessCode
}

func (p *ExchangeRateAPIOpenAccessProvider) Name() string {
	return "ExchangeRate-API Open Access"
}

func (p *ExchangeRateAPIOpenAccessProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{FXRates: true}
}

func (p *ExchangeRateAPIOpenAccessProvider) FetchFXRates(ctx context.Context, request FXRateRequest) ([]FXRateObservation, error) {
	base, quotes := normalizedCodes(request.BaseCode, request.QuoteCodes)
	if base == "" {
		base = "USD"
	}
	body, err := getBytes(ctx, p.client, p.baseURL+"/latest/"+base)
	if err != nil {
		return nil, err
	}
	return parseExchangeRateAPIRates(body, quotes)
}

type exchangeRateAPIResponse struct {
	Result            string                 `json:"result"`
	ErrorType         string                 `json:"error-type"`
	BaseCode          string                 `json:"base_code"`
	TimeLastUpdateUTC string                 `json:"time_last_update_utc"`
	Rates             map[string]json.Number `json:"rates"`
}

func parseExchangeRateAPIRates(body []byte, quoteCodes []string) ([]FXRateObservation, error) {
	var response exchangeRateAPIResponse
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("parse ExchangeRate-API rates: %w", err)
	}
	if response.Result != "success" {
		return nil, fmt.Errorf("ExchangeRate-API error: %s", response.ErrorType)
	}
	base := strings.ToUpper(strings.TrimSpace(response.BaseCode))
	if base == "" {
		return nil, fmt.Errorf("ExchangeRate-API response is missing base_code")
	}
	valuationDate := ""
	publishedAt := parseHTTPDate(response.TimeLastUpdateUTC)
	if publishedAt != "" {
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
			return nil, fmt.Errorf("parse ExchangeRate-API %s rate: %w", quote, err)
		}
		observations = append(observations, FXRateObservation{
			SourceCode:            ExchangeRateAPIOpenAccessCode,
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
			ProviderSeriesID:      "ERAPI:" + base + ":" + quote,
			ProviderObservationID: "ERAPI:" + valuationDate + ":" + base + ":" + quote,
			MetadataJSON:          `{"provider":"ExchangeRate-API Open Access","attribution_required":true}`,
			RawJSON:               "{}",
		})
	}
	sortFXRateObservations(observations)
	return observations, nil
}

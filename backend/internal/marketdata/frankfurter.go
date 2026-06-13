package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const FrankfurterCode = "frankfurter"

type FrankfurterProvider struct {
	client  *http.Client
	baseURL string
}

func NewFrankfurterProvider(client *http.Client, baseURL string) *FrankfurterProvider {
	return &FrankfurterProvider{
		client:  httpClient(client),
		baseURL: trimBaseURL(baseURL, "https://api.frankfurter.dev/v2"),
	}
}

func (p *FrankfurterProvider) Code() string {
	return FrankfurterCode
}

func (p *FrankfurterProvider) Name() string {
	return "Frankfurter"
}

func (p *FrankfurterProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{FXRates: true}
}

func (p *FrankfurterProvider) FetchFXRates(ctx context.Context, request FXRateRequest) ([]FXRateObservation, error) {
	base, quotes := normalizedCodes(request.BaseCode, request.QuoteCodes)
	if base == "" {
		base = "EUR"
	}
	values := url.Values{}
	values.Set("base", base)
	if len(quotes) > 0 {
		values.Set("quotes", strings.Join(quotes, ","))
	}
	if strings.TrimSpace(request.Date) != "" {
		values.Set("date", strings.TrimSpace(request.Date))
	}
	body, err := getBytes(ctx, p.client, p.baseURL+"/rates?"+values.Encode())
	if err != nil {
		return nil, err
	}
	return parseFrankfurterRates(body, base)
}

type frankfurterRatesResponse struct {
	Base  string                 `json:"base"`
	Date  string                 `json:"date"`
	Rates map[string]json.Number `json:"rates"`
}

func parseFrankfurterRates(body []byte, fallbackBase string) ([]FXRateObservation, error) {
	var response frankfurterRatesResponse
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("parse Frankfurter rates: %w", err)
	}
	base := strings.ToUpper(strings.TrimSpace(response.Base))
	if base == "" {
		base = strings.ToUpper(strings.TrimSpace(fallbackBase))
	}
	if base == "" || response.Date == "" {
		return nil, fmt.Errorf("Frankfurter response is missing base or date")
	}
	observations := make([]FXRateObservation, 0, len(response.Rates))
	for quoteCode, rate := range response.Rates {
		quote := strings.ToUpper(strings.TrimSpace(quoteCode))
		value, scale, err := decimalToScaled(rate.String())
		if err != nil {
			return nil, fmt.Errorf("parse Frankfurter %s rate: %w", quote, err)
		}
		observations = append(observations, FXRateObservation{
			SourceCode:            FrankfurterCode,
			BaseCode:              base,
			QuoteCode:             quote,
			QuoteType:             "official_fixing",
			AdjustmentBasis:       "not_applicable",
			PriceValue:            value,
			PriceScale:            scale,
			BaseQuantityValue:     1,
			BaseQuantityScale:     0,
			ValuationDate:         response.Date,
			ProviderSeriesID:      "FRANKFURTER:" + base + ":" + quote,
			ProviderObservationID: "FRANKFURTER:" + response.Date + ":" + base + ":" + quote,
			MetadataJSON:          `{"provider":"Frankfurter"}`,
			RawJSON:               "{}",
		})
	}
	sortFXRateObservations(observations)
	return observations, nil
}

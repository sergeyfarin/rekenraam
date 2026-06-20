package marketdata

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const ECBReferenceRatesCode = "ecb_euro_reference_rates"

type ECBReferenceRatesProvider struct {
	client  *http.Client
	baseURL string
}

func NewECBReferenceRatesProvider(client *http.Client, baseURL string) *ECBReferenceRatesProvider {
	return &ECBReferenceRatesProvider{
		client:  httpClient(client),
		baseURL: trimBaseURL(baseURL, "https://www.ecb.europa.eu/stats/eurofxref"),
	}
}

func (p *ECBReferenceRatesProvider) Code() string {
	return ECBReferenceRatesCode
}

func (p *ECBReferenceRatesProvider) Name() string {
	return "ECB euro foreign exchange reference rates"
}

func (p *ECBReferenceRatesProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{FXRates: true, HistoricalFXRates: true}
}

func (p *ECBReferenceRatesProvider) FetchFXRates(ctx context.Context, request FXRateRequest) ([]FXRateObservation, error) {
	base, quotes := normalizedCodes(request.BaseCode, request.QuoteCodes)
	if base == "" {
		base = "EUR"
	}
	if base != "EUR" {
		return nil, fmt.Errorf("%w: ECB reference rates are quoted against EUR", ErrUnsupportedPair)
	}
	path := "/eurofxref-daily.xml"
	if strings.TrimSpace(request.Date) != "" {
		path = "/eurofxref-hist-90d.xml"
	}
	body, err := getBytes(ctx, p.client, p.baseURL+path)
	if err != nil {
		return nil, err
	}
	return parseECBReferenceRates(body, strings.TrimSpace(request.Date), quotes)
}

type ecbEnvelope struct {
	Cubes []ecbCube `xml:"Cube>Cube"`
}

type ecbCube struct {
	Time  string        `xml:"time,attr"`
	Rates []ecbCubeRate `xml:"Cube"`
}

type ecbCubeRate struct {
	Currency string `xml:"currency,attr"`
	Rate     string `xml:"rate,attr"`
}

func parseECBReferenceRates(body []byte, requestedDate string, quoteCodes []string) ([]FXRateObservation, error) {
	var envelope ecbEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse ECB reference rates: %w", err)
	}
	quoteFilter := quoteSet(quoteCodes)
	var selected ecbCube
	for _, cube := range envelope.Cubes {
		if cube.Time == "" {
			continue
		}
		if requestedDate == "" || cube.Time == requestedDate {
			selected = cube
			break
		}
	}
	if selected.Time == "" {
		return nil, fmt.Errorf("ECB reference rate date not found: %s", requestedDate)
	}
	observations := make([]FXRateObservation, 0, len(selected.Rates))
	for _, rate := range selected.Rates {
		quote := strings.ToUpper(strings.TrimSpace(rate.Currency))
		if quote == "" || (quoteFilter != nil && !quoteFilter[quote]) {
			continue
		}
		value, scale, err := decimalToScaled(rate.Rate)
		if err != nil {
			return nil, fmt.Errorf("parse ECB %s rate: %w", quote, err)
		}
		observations = append(observations, FXRateObservation{
			SourceCode:            ECBReferenceRatesCode,
			BaseCode:              "EUR",
			QuoteCode:             quote,
			QuoteType:             "official_fixing",
			AdjustmentBasis:       "not_applicable",
			PriceValue:            value,
			PriceScale:            scale,
			BaseQuantityValue:     1,
			BaseQuantityScale:     0,
			ValuationDate:         selected.Time,
			ProviderSeriesID:      "ECB:EUR:" + quote,
			ProviderObservationID: "ECB:" + selected.Time + ":EUR:" + quote,
			MetadataJSON:          `{"provider":"ECB","base":"EUR"}`,
			RawJSON:               "{}",
		})
	}
	return observations, nil
}

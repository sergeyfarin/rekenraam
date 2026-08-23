package marketdata

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

var (
	ErrUnsupportedPair = errors.New("market data provider does not support the requested currency pair")
	ErrMissingSecret   = errors.New("market data provider requires a missing operator secret")
)

type ProviderCapabilities struct {
	FXRates           bool
	HistoricalFXRates bool
}

type Provider interface {
	Code() string
	Name() string
	Capabilities() ProviderCapabilities
}

type FXRateProvider interface {
	Provider
	FetchFXRates(ctx context.Context, request FXRateRequest) ([]FXRateObservation, error)
}

type FXRateRequest struct {
	BaseCode   string
	QuoteCodes []string
	Date       string
}

type FXRateObservation struct {
	SourceCode            string
	BaseCode              string
	QuoteCode             string
	QuoteType             string
	AdjustmentBasis       string
	PriceValue            int64
	PriceScale            int
	BaseQuantityValue     int64
	BaseQuantityScale     int
	ValuationDate         string
	ObservedAt            string
	SourcePublishedAt     string
	ProviderSeriesID      string
	ProviderObservationID string
	MetadataJSON          string
	RawJSON               string
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{providers: map[string]Provider{}}
	for _, provider := range providers {
		registry.Register(provider)
	}
	return registry
}

func DefaultRegistry(openExchangeRatesAppID string) *Registry {
	return NewRegistry(
		NewECBReferenceRatesProvider(nil, ""),
		NewFrankfurterProvider(nil, ""),
		NewExchangeRateAPIOpenAccessProvider(nil, ""),
		NewOpenExchangeRatesProvider(nil, "", openExchangeRatesAppID),
	)
}

func (r *Registry) Register(provider Provider) {
	if provider == nil {
		return
	}
	r.providers[provider.Code()] = provider
}

func (r *Registry) Provider(code string) (Provider, bool) {
	provider, ok := r.providers[strings.TrimSpace(code)]
	return provider, ok
}

func (r *Registry) FXRateProvider(code string) (FXRateProvider, bool) {
	provider, ok := r.Provider(code)
	if !ok {
		return nil, false
	}
	fxProvider, ok := provider.(FXRateProvider)
	return fxProvider, ok
}

func normalizedCodes(baseCode string, quoteCodes []string) (string, []string) {
	base := strings.ToUpper(strings.TrimSpace(baseCode))
	quotes := make([]string, 0, len(quoteCodes))
	seen := map[string]bool{}
	for _, quote := range quoteCodes {
		quote = strings.ToUpper(strings.TrimSpace(quote))
		if quote == "" || seen[quote] {
			continue
		}
		seen[quote] = true
		quotes = append(quotes, quote)
	}
	return base, quotes
}

func decimalToScaled(value string) (int64, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, fmt.Errorf("decimal value is empty")
	}
	rat, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, 0, fmt.Errorf("decimal value is invalid: %s", value)
	}
	if rat.Sign() <= 0 {
		return 0, 0, fmt.Errorf("decimal value must be positive")
	}
	parts := strings.Split(value, ".")
	scale := 0
	if len(parts) == 2 {
		scale = len(parts[1])
	}
	scaled := new(big.Rat).Mul(rat, new(big.Rat).SetInt(pow10(scale)))
	if !scaled.IsInt() {
		return 0, 0, fmt.Errorf("decimal value cannot be scaled exactly: %s", value)
	}
	numerator := scaled.Num()
	if !numerator.IsInt64() {
		return 0, 0, fmt.Errorf("decimal value overflows int64")
	}
	return numerator.Int64(), scale, nil
}

func pow10(exp int) *big.Int {
	result := big.NewInt(1)
	ten := big.NewInt(10)
	for range exp {
		result.Mul(result, ten)
	}
	return result
}

func parseHTTPDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func quoteSet(quoteCodes []string) map[string]bool {
	if len(quoteCodes) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, quote := range quoteCodes {
		quote = strings.ToUpper(strings.TrimSpace(quote))
		if quote != "" {
			set[quote] = true
		}
	}
	return set
}

func sortFXRateObservations(observations []FXRateObservation) {
	slices.SortFunc(observations, func(left FXRateObservation, right FXRateObservation) int {
		return strings.Compare(left.QuoteCode, right.QuoteCode)
	})
}

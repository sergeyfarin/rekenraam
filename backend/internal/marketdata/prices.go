package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/exact"
)

// PriceProvider is the sole quote-provider contract. Adapters return facts and
// never receive database IDs or write financial state.
type PriceProvider interface {
	Provider
	FetchPrices(context.Context, PriceRequest) ([]PriceObservation, error)
}

type PriceRequest struct {
	InstrumentType       string
	ProviderInstrumentID string
	QuoteCode            string
	MIC                  string
}

type PriceObservation struct {
	ProviderInstrumentID  string
	QuoteCode             string
	QuoteType             string
	AdjustmentBasis       string
	PriceValue            int64
	PriceScale            int
	ValuationDate         string
	ObservedAt            string
	ProviderObservationID string
	RawJSON               string
}

func (r *Registry) PriceProvider(code string) (PriceProvider, bool) {
	provider, ok := r.Provider(code)
	if !ok {
		return nil, false
	}
	prices, ok := provider.(PriceProvider)
	return prices, ok
}

// QuoteError contains no URL, credential or remote response text. It is safe
// to persist in operator health and work-item errors.
type QuoteError struct {
	Status     int
	RetryAfter time.Duration
	Terminal   bool
}

func (e *QuoteError) Error() string {
	return fmt.Sprintf("quote provider request failed (status %d)", e.Status)
}

func quoteJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("invalid quote provider request")
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	// Never forward operator credentials to a redirect destination.
	copyClient := *httpClient(client)
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copyClient.Do(req)
	if err != nil {
		return errors.New("quote provider connection failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		delay := time.Duration(0)
		if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
			delay = time.Duration(min(seconds, 86400)) * time.Second
		} else if date, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			delay = max(time.Duration(0), min(time.Until(date), 24*time.Hour))
		}
		return &QuoteError{Status: response.StatusCode, RetryAfter: delay, Terminal: response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != 408 && response.StatusCode != 429}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(body) > 2<<20 {
		return errors.New("invalid quote provider response size")
	}
	if err := json.Unmarshal(body, result); err != nil {
		return errors.New("invalid quote provider JSON")
	}
	return nil
}

var priceDecimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,2})?$`)

// JSON numbers stay decimal strings, including scientific notation. Refuse
// precision/range loss rather than round an unrepresentable quote into a fact.
func quoteDecimal(value string) (int64, int, error) {
	if len(value) > 128 || !priceDecimalPattern.MatchString(value) {
		return 0, 0, errors.New("invalid quote decimal")
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == 'e' || r == 'E' })
	exponent := 0
	if len(parts) == 2 {
		exponent, _ = strconv.Atoi(parts[1])
		if exponent < -24 || exponent > 24 {
			return 0, 0, errors.New("invalid quote exponent")
		}
	}
	mantissa := parts[0]
	scale := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		scale = len(mantissa) - dot - 1
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	scale -= exponent
	for scale > 0 && strings.HasSuffix(mantissa, "0") {
		mantissa = strings.TrimSuffix(mantissa, "0")
		scale--
	}
	if scale < 0 {
		mantissa += strings.Repeat("0", -scale)
		scale = 0
	}
	if scale > 18 {
		return 0, 0, errors.New("quote exceeds supported price precision")
	}
	coefficient, ok := new(big.Int).SetString(mantissa, 10)
	if !ok || coefficient.Sign() <= 0 {
		return 0, 0, errors.New("quote must be positive")
	}
	value64, err := exact.ScaledIntFromBig(coefficient, scale).Int64()
	return value64, scale, err
}

type CoinGeckoProvider struct {
	client          *http.Client
	baseURL, apiKey string
}

func NewCoinGeckoProvider(client *http.Client, baseURL, apiKey string) *CoinGeckoProvider {
	return &CoinGeckoProvider{client, trimBaseURL(baseURL, "https://api.coingecko.com/api/v3"), apiKey}
}
func (p *CoinGeckoProvider) Code() string { return "coingecko" }
func (p *CoinGeckoProvider) Name() string { return "CoinGecko" }
func (p *CoinGeckoProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Prices: true}
}
func (p *CoinGeckoProvider) FetchPrices(ctx context.Context, request PriceRequest) ([]PriceObservation, error) {
	if p.apiKey == "" {
		return nil, ErrMissingSecret
	}
	if request.InstrumentType != "crypto" || request.ProviderInstrumentID == "" {
		return nil, ErrUnsupportedPair
	}
	query := url.Values{"ids": {request.ProviderInstrumentID}, "vs_currencies": {strings.ToLower(request.QuoteCode)}, "include_last_updated_at": {"true"}, "precision": {"full"}}
	var data map[string]map[string]json.Number
	if err := quoteJSON(ctx, p.client, p.baseURL+"/simple/price?"+query.Encode(), map[string]string{"x-cg-demo-api-key": p.apiKey}, &data); err != nil {
		return nil, err
	}
	row := data[request.ProviderInstrumentID]
	stamp, err := row["last_updated_at"].Int64()
	if err != nil || stamp <= 0 {
		return nil, errors.New("quote provider returned no observation timestamp")
	}
	value, scale, err := quoteDecimal(row[strings.ToLower(request.QuoteCode)].String())
	if err != nil {
		return nil, err
	}
	at := time.Unix(stamp, 0).UTC()
	raw, _ := json.Marshal(row)
	return []PriceObservation{{ProviderInstrumentID: request.ProviderInstrumentID, QuoteCode: strings.ToUpper(request.QuoteCode), QuoteType: "spot_mid", AdjustmentBasis: "raw", PriceValue: value, PriceScale: scale, ValuationDate: at.Format(time.DateOnly), ObservedAt: at.Format(time.RFC3339), ProviderObservationID: request.ProviderInstrumentID + ":" + strings.ToLower(request.QuoteCode) + ":" + strconv.FormatInt(stamp, 10), RawJSON: string(raw)}}, nil
}

type TwelveDataProvider struct {
	client          *http.Client
	baseURL, apiKey string
}

func NewTwelveDataProvider(client *http.Client, baseURL, apiKey string) *TwelveDataProvider {
	return &TwelveDataProvider{client, trimBaseURL(baseURL, "https://api.twelvedata.com"), apiKey}
}
func (p *TwelveDataProvider) Code() string { return "twelve_data" }
func (p *TwelveDataProvider) Name() string { return "Twelve Data" }
func (p *TwelveDataProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Prices: true, Dividends: true}
}
func (p *TwelveDataProvider) FetchPrices(ctx context.Context, request PriceRequest) ([]PriceObservation, error) {
	if p.apiKey == "" {
		return nil, ErrMissingSecret
	}
	if request.ProviderInstrumentID == "" || (request.InstrumentType != "crypto" && request.MIC == "") {
		return nil, ErrUnsupportedPair
	}
	query := url.Values{"symbol": {request.ProviderInstrumentID}, "interval": {"1day"}, "outputsize": {"2"}, "adjust": {"none"}, "apikey": {p.apiKey}}
	if request.MIC != "" {
		query.Set("mic_code", request.MIC)
	}
	var data struct {
		Code   int    `json:"code"`
		Status string `json:"status"`
		Meta   struct {
			Symbol   string `json:"symbol"`
			Currency string `json:"currency"`
			MIC      string `json:"mic_code"`
			TimeZone string `json:"exchange_timezone"`
		} `json:"meta"`
		Values []struct {
			Date  string `json:"datetime"`
			Close string `json:"close"`
		} `json:"values"`
	}
	if err := quoteJSON(ctx, p.client, p.baseURL+"/time_series?"+query.Encode(), nil, &data); err != nil {
		return nil, err
	}
	if data.Status == "error" {
		return nil, &QuoteError{Status: data.Code, Terminal: data.Code >= 400 && data.Code < 500 && data.Code != 429}
	}
	if !strings.EqualFold(data.Meta.Currency, request.QuoteCode) || data.Meta.Symbol != request.ProviderInstrumentID || request.MIC != "" && data.Meta.MIC != request.MIC {
		return nil, errors.New("quote provider returned a different instrument, market or currency")
	}
	location, err := time.LoadLocation(data.Meta.TimeZone)
	if err != nil || data.Meta.TimeZone == "" {
		return nil, errors.New("quote provider returned no valid market timezone")
	}
	today := time.Now().In(location).Format(time.DateOnly)
	selected := -1
	for i, row := range data.Values {
		if _, err := time.Parse(time.DateOnly, row.Date); err == nil && row.Date < today && (selected < 0 || row.Date > data.Values[selected].Date) {
			selected = i
		}
	}
	if selected < 0 {
		return nil, errors.New("quote provider returned no completed daily close")
	}
	row := data.Values[selected]
	if _, err := time.Parse(time.DateOnly, row.Date); err != nil {
		return nil, errors.New("quote provider returned an invalid date")
	}
	value, scale, err := quoteDecimal(row.Close)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(data)
	return []PriceObservation{{ProviderInstrumentID: request.ProviderInstrumentID, QuoteCode: data.Meta.Currency, QuoteType: "close", AdjustmentBasis: "raw", PriceValue: value, PriceScale: scale, ValuationDate: row.Date, ProviderObservationID: request.ProviderInstrumentID + ":" + data.Meta.MIC + ":" + row.Date, RawJSON: string(raw)}}, nil
}

type YahooFinanceProvider struct {
	client  *http.Client
	baseURL string
}

func NewYahooFinanceProvider(client *http.Client, baseURL string) *YahooFinanceProvider {
	return &YahooFinanceProvider{client, trimBaseURL(baseURL, "https://query1.finance.yahoo.com")}
}
func (p *YahooFinanceProvider) Code() string { return "yahoo_finance" }
func (p *YahooFinanceProvider) Name() string { return "Yahoo Finance (unofficial)" }
func (p *YahooFinanceProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Prices: true}
}
func (p *YahooFinanceProvider) FetchPrices(ctx context.Context, request PriceRequest) ([]PriceObservation, error) {
	if request.ProviderInstrumentID == "" {
		return nil, ErrUnsupportedPair
	}
	var data struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol               string `json:"symbol"`
					Currency             string `json:"currency"`
					TimeZone             string `json:"exchangeTimezoneName"`
					CurrentTradingPeriod struct {
						Regular struct {
							End int64 `json:"end"`
						} `json:"regular"`
					} `json:"currentTradingPeriod"`
				} `json:"meta"`
				Events struct {
					Splits map[string]struct {
						Date int64 `json:"date"`
					} `json:"splits"`
				} `json:"events"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []json.Number `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		} `json:"chart"`
	}
	endpoint := p.baseURL + "/v8/finance/chart/" + url.PathEscape(request.ProviderInstrumentID) + "?interval=1d&range=5d&events=splits"
	if err := quoteJSON(ctx, p.client, endpoint, nil, &data); err != nil {
		return nil, err
	}
	if len(data.Chart.Result) != 1 || len(data.Chart.Error) > 0 && string(data.Chart.Error) != "null" {
		return nil, errors.New("unofficial quote provider returned no chart")
	}
	result := data.Chart.Result[0]
	if result.Meta.Symbol != request.ProviderInstrumentID || result.Meta.Currency != request.QuoteCode {
		return nil, errors.New("quote provider returned a different instrument or currency (minor-unit currencies are unsupported)")
	}
	if len(result.Indicators.Quote) != 1 {
		return nil, errors.New("quote provider returned no raw closes")
	}
	location, err := time.LoadLocation(result.Meta.TimeZone)
	if err != nil || result.Meta.TimeZone == "" {
		return nil, errors.New("quote provider returned no valid market timezone")
	}
	now := time.Now()
	closes := result.Indicators.Quote[0].Close
	for i, stamp := range slices.Backward(result.Timestamp) {
		if i >= len(closes) || closes[i] == "" {
			continue
		}
		barDate := time.Unix(stamp, 0).In(location).Format(time.DateOnly)
		today := now.In(location).Format(time.DateOnly)
		if barDate > today || (barDate == today && (result.Meta.CurrentTradingPeriod.Regular.End <= 0 || now.Unix() < result.Meta.CurrentTradingPeriod.Regular.End)) {
			continue
		}
		for _, split := range result.Events.Splits {
			if split.Date > stamp {
				return nil, errors.New("unofficial close crosses a split; use another source or a manual quote")
			}
		}
		value, scale, err := quoteDecimal(closes[i].String())
		if err != nil {
			return nil, err
		}

		if stamp <= 0 {
			return nil, errors.New("quote provider returned an invalid timestamp")
		}
		at := time.Unix(stamp, 0).UTC()
		raw, _ := json.Marshal(map[string]any{"symbol": result.Meta.Symbol, "currency": result.Meta.Currency, "timestamp": stamp, "close": closes[i], "unofficial": true})
		return []PriceObservation{{ProviderInstrumentID: request.ProviderInstrumentID, QuoteCode: result.Meta.Currency, QuoteType: "close", AdjustmentBasis: "raw", PriceValue: value, PriceScale: scale, ValuationDate: at.In(location).Format(time.DateOnly), ProviderObservationID: request.ProviderInstrumentID + ":" + strconv.FormatInt(stamp, 10), RawJSON: string(raw)}}, nil
	}
	return nil, errors.New("quote provider returned no raw close")
}

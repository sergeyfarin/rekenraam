package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

const MassiveCode = "massive"

type MassiveSplit struct {
	ID          string
	Date        string
	Kind        string
	Numerator   int64
	Denominator int64
	ShareFIGI   string
	Conflict    bool
	RawJSON     string
}

type MassiveProvider struct {
	client          *http.Client
	baseURL, apiKey string
}

func NewMassiveProvider(client *http.Client, baseURL, apiKey string) *MassiveProvider {
	return &MassiveProvider{client, trimBaseURL(baseURL, "https://api.massive.com"), apiKey}
}

func (*MassiveProvider) Code() string { return MassiveCode }
func (*MassiveProvider) Name() string { return "Massive" }
func (*MassiveProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Splits: true}
}

var massiveTicker = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.-]{0,31}$`)

func ValidMassiveTicker(symbol string) bool { return massiveTicker.MatchString(symbol) }

func MassiveListingMIC(mic string) bool {
	switch mic {
	case "XNAS", "XNYS", "XASE", "ARCX", "BATS":
		return true
	}
	return false
}

type massiveSplitRow struct {
	ID     string      `json:"id"`
	Ticker string      `json:"ticker"`
	Date   string      `json:"execution_date"`
	Kind   string      `json:"adjustment_type"`
	From   json.Number `json:"split_from"`
	To     json.Number `json:"split_to"`
}

func (p *MassiveProvider) FetchSplits(ctx context.Context, symbol, mic, from, through string) ([]MassiveSplit, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("%w: set MASSIVE_API_KEY", ErrMissingSecret)
	}
	if !ValidMassiveTicker(symbol) || !MassiveListingMIC(mic) {
		return nil, fmt.Errorf("%w: Massive requires an explicit supported US listing", ErrUnsupportedPair)
	}
	headers := map[string]string{"Authorization": "Bearer " + p.apiKey}
	query := url.Values{"ticker": {symbol}, "execution_date.gte": {from}, "execution_date.lte": {through}, "limit": {"1000"}}
	var page struct {
		Status  string            `json:"status"`
		NextURL string            `json:"next_url"`
		Results []json.RawMessage `json:"results"`
	}
	if err := quoteJSON(ctx, p.client, p.baseURL+"/stocks/v1/splits?"+query.Encode(), headers, &page); err != nil {
		return nil, err
	}
	// One ticker's bounded history fits the endpoint's maximum page. A
	// provider-supplied URL must never become a credential destination.
	if page.Status != "OK" || page.NextURL != "" {
		return nil, errors.New("Massive returned an incomplete split response")
	}
	if len(page.Results) == 0 {
		return nil, nil
	}
	figi, err := p.listingFIGI(ctx, symbol, mic, through, headers)
	if err != nil {
		return nil, err
	}
	listings := map[string]string{through: figi}
	result := make([]MassiveSplit, 0, len(page.Results))
	splitCounts := make(map[string]int)
	for _, raw := range page.Results {
		var row massiveSplitRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, errors.New("invalid Massive split record")
		}
		_, dateErr := time.Parse(time.DateOnly, row.Date)
		if row.ID == "" || len(row.ID) > 128 || row.Ticker != symbol || dateErr != nil || row.Date < from || row.Date > through {
			return nil, errors.New("Massive split response does not match the requested listing and dates")
		}
		if row.Kind == "forward_split" || row.Kind == "reverse_split" {
			splitCounts[row.Date]++
		}
		numerator, denominator := massiveSplitRatio(row)
		evidence, _ := json.Marshal(row)
		observed := MassiveSplit{ID: row.ID, Date: row.Date, Kind: row.Kind, Numerator: numerator,
			Denominator: denominator, RawJSON: string(evidence)}
		if numerator > 0 {
			eventFIGI, cached := listings[row.Date]
			if !cached {
				eventFIGI, err = p.listingFIGI(ctx, symbol, mic, row.Date, headers)
				// Ticker reuse, an inactive historical listing or missing
				// identity leaves evidence without a postable ratio.
				if err != nil && !errors.Is(err, ErrUnsupportedPair) {
					return nil, err
				}
				listings[row.Date] = eventFIGI
			}
			if eventFIGI == figi {
				observed.ShareFIGI = figi
			}
		}
		result = append(result, observed)
	}
	for index := range result {
		result[index].Conflict = splitCounts[result[index].Date] > 1
	}
	return result, nil
}

func (p *MassiveProvider) listingFIGI(ctx context.Context, symbol, mic, date string, headers map[string]string) (string, error) {
	var response struct {
		Status  string `json:"status"`
		Results struct {
			Ticker string `json:"ticker"`
			MIC    string `json:"primary_exchange"`
			Market string `json:"market"`
			Locale string `json:"locale"`
			FIGI   string `json:"share_class_figi"`
		} `json:"results"`
	}
	query := url.Values{"date": {date}}
	if err := quoteJSON(ctx, p.client, p.baseURL+"/v3/reference/tickers/"+url.PathEscape(symbol)+"?"+query.Encode(), headers, &response); err != nil {
		if failure, ok := errors.AsType[*QuoteError](err); ok && failure.Status == http.StatusNotFound {
			return "", ErrUnsupportedPair
		}
		return "", err
	}
	listing := response.Results
	if response.Status != "OK" || listing.Ticker != symbol || listing.MIC != mic || listing.Market != "stocks" ||
		listing.Locale != "us" || listing.FIGI == "" || len(listing.FIGI) > 32 {
		return "", fmt.Errorf("%w: Massive could not verify the assigned US listing", ErrUnsupportedPair)
	}
	return listing.FIGI, nil
}

func massiveSplitRatio(row massiveSplitRow) (int64, int64) {
	if row.Kind != "forward_split" && row.Kind != "reverse_split" {
		return 0, 0
	}
	from, fromScale, fromErr := quoteDecimal(string(row.From))
	to, toScale, toErr := quoteDecimal(string(row.To))
	if fromErr != nil || toErr != nil {
		return 0, 0
	}
	ratio := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(to), pow10(fromScale)),
		new(big.Int).Mul(big.NewInt(from), pow10(toScale)))
	bound := big.NewInt(1_000_000_000)
	if ratio.Num().Cmp(bound) > 0 || ratio.Denom().Cmp(bound) > 0 {
		return 0, 0
	}
	numerator, denominator := ratio.Num().Int64(), ratio.Denom().Int64()
	if (row.Kind == "forward_split" && numerator <= denominator) ||
		(row.Kind == "reverse_split" && numerator >= denominator) {
		return 0, 0
	}
	return numerator, denominator
}

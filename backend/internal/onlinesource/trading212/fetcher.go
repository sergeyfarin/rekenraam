package trading212

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://live.trading212.com/api/v0"

// historyPath is the cash/transaction history endpoint. Verified 2026-07-03
// against the published Trading 212 OpenAPI spec (docs.trading212.com) — the
// original Slice 2 path (`/history/transactions`, missing the `/equity`
// segment) was an unverified guess that 404s against the real API. Isolated
// here so a path or field rename is a one-line fix.
const historyPath = "/equity/history/transactions"

// accountSummaryPath is a lightweight authenticated endpoint used only to
// validate an API key (Probe). Deliberately not historyPath: probing
// shouldn't require the `history:transactions` scope just to confirm the key
// works.
const accountSummaryPath = "/equity/account/summary"

// ordersPath and dividendsPath (B-T212-INVST/Slice 4b) were verified
// 2026-07-03 against the published Trading 212 OpenAPI spec, the same pass
// that caught historyPath's missing /equity segment — these were never
// guessed against an unverified assumption the way the original Slice 2
// cash-history path was.
const ordersPath = "/equity/history/orders"
const dividendsPath = "/equity/history/dividends"

// maxPages bounds a single Fetch call so a misbehaving cursor (or an
// adversarial nextPagePath) can't loop forever; the durable queue's
// continuation payload (Slice 3) is the mechanism for fetching more than
// this many pages in one logical refresh (FetchResult.HasMore signals it).
// A var, not a const, so fetcher_test.go can exercise the >maxPages path
// without an actual 50-page fake server.
var maxPages = 50

// Fetcher talks HTTP to the Trading 212 public API. It holds no secret and
// no DB handle: the caller supplies the API key per call and persists
// nothing here.
type Fetcher struct {
	client  *http.Client
	baseURL string
}

// SetMaxPagesForTest overrides maxPages for the duration of a test and
// returns a restore function. Lets callers outside this package (the
// app-level fetch worker tests) exercise the pagination-continuation path
// without standing up a real 50+ page fake server.
func SetMaxPagesForTest(n int) (restore func()) {
	old := maxPages
	maxPages = n
	return func() { maxPages = old }
}

func NewFetcher(client *http.Client, baseURL string) *Fetcher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Fetcher{client: client, baseURL: baseURL}
}

// Probe makes one lightweight authenticated call to confirm the API key is
// valid, without paging through history. Used by ImportConnectionService at
// connection create/rotate time.
func (f *Fetcher) Probe(ctx context.Context, apiKey string) error {
	// The response shape doesn't matter — Probe only cares whether the
	// request succeeds or the provider rejects the key — so any item type
	// works here; json.RawMessage avoids committing to a schema for a
	// response this code never actually reads.
	_, _, _, _, err := fetchRawPage[json.RawMessage](ctx, f, apiKey, accountSummaryPath, accountSummaryPath)
	return err
}

// Fetch pages through transaction history and returns all movements seen
// plus the new incremental cursor.
//
// cursor is the incremental boundary: pagination stops once a movement
// strictly older than cursor is reached (empty = full history, never stops
// early). It is fixed for a whole logical fetch — pass the same value again
// on every continuation call (see HasMore/NextPageToken below); it is not
// something to advance chunk-to-chunk.
//
// resumeFrom is which page to start turning from: empty starts at page 1
// (the provider's newest data). Pass FetchResult.NextPageToken here on a
// follow-up call to continue exactly where the previous call's page budget
// (maxPages) cut it off, without re-scanning everything already seen.
func (f *Fetcher) Fetch(ctx context.Context, apiKey string, cursor string, resumeFrom string) (FetchResult, error) {
	items, maxSeen, hasMore, nextToken, err := fetchPaginated(
		ctx, apiKey, historyPath, resumeFrom, cursor,
		func(m Movement) string { return m.Timestamp },
		f.fetchTransactionsPage,
	)
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{Movements: items, Cursor: maxSeen, HasMore: hasMore, NextPageToken: nextToken}, nil
}

// FetchOrders pages through order-fill history exactly like Fetch, but
// against /equity/history/orders and returning []OrderFill. See Fetch's doc
// comment for what cursor/resumeFrom mean — identical contract, reusing the
// same maxPages/continuation machinery (T-14) rather than duplicating it.
func (f *Fetcher) FetchOrders(ctx context.Context, apiKey string, cursor string, resumeFrom string) (OrdersFetchResult, error) {
	items, maxSeen, hasMore, nextToken, err := fetchPaginated(
		ctx, apiKey, ordersPath, resumeFrom, cursor,
		func(o OrderFill) string { return o.FilledAt },
		f.fetchOrdersPage,
	)
	if err != nil {
		return OrdersFetchResult{}, err
	}
	return OrdersFetchResult{Fills: items, Cursor: maxSeen, HasMore: hasMore, NextPageToken: nextToken}, nil
}

// FetchDividends pages through dividend history exactly like Fetch, but
// against /equity/history/dividends and returning []Dividend.
func (f *Fetcher) FetchDividends(ctx context.Context, apiKey string, cursor string, resumeFrom string) (DividendsFetchResult, error) {
	items, maxSeen, hasMore, nextToken, err := fetchPaginated(
		ctx, apiKey, dividendsPath, resumeFrom, cursor,
		func(d Dividend) string { return d.PaidOn },
		f.fetchDividendsPage,
	)
	if err != nil {
		return DividendsFetchResult{}, err
	}
	return DividendsFetchResult{Dividends: items, Cursor: maxSeen, HasMore: hasMore, NextPageToken: nextToken}, nil
}

// fetchOnePage is the shape every endpoint-specific page fetcher
// (fetchTransactionsPage/fetchOrdersPage/fetchDividendsPage) implements:
// one HTTP GET decoded into that endpoint's item type, the next page path
// (empty if exhausted), and shouldRetry=true with a delay if the caller
// should back off and retry this same page (429 within-call throttling).
type fetchOnePage[T any] func(ctx context.Context, apiKey string, path string) (items []T, nextPagePath string, retryAfter time.Duration, shouldRetry bool, err error)

// fetchPaginated is the shared pagination/incremental-cursor/continuation
// engine behind Fetch, FetchOrders, and FetchDividends. It is generic over
// the item type so the three endpoints (which only differ in JSON shape and
// which field carries the item's timestamp) share one page-budget,
// same-timestamp-rescan, and continuation-token implementation rather than
// tripling it — see docs/plans/trading212-import-plan.md Slice 4b, which calls
// this machinery out by name as something to reuse, not duplicate.
func fetchPaginated[T any](
	ctx context.Context,
	apiKey string,
	startPath string,
	resumeFrom string,
	cursor string,
	timestampOf func(T) string,
	fetchPage fetchOnePage[T],
) (items []T, maxSeen string, hasMore bool, nextPageToken string, err error) {
	nextPath := startPath
	if resumeFrom != "" {
		nextPath = resumeFrom
	}
	maxSeen = cursor

	for page := 0; page < maxPages; page++ {
		if nextPath == "" {
			break
		}
		pageItems, next, retryAfter, shouldRetry, pageErr := fetchPage(ctx, apiKey, nextPath)
		if shouldRetry {
			select {
			case <-ctx.Done():
				return nil, "", false, "", ctx.Err()
			case <-time.After(retryAfter):
			}
			continue // retry the same page
		}
		if pageErr != nil {
			return nil, "", false, "", pageErr
		}

		stop := false
		for _, item := range pageItems {
			ts := timestampOf(item)
			// Strictly less-than: an item whose timestamp equals the cursor
			// is re-scanned rather than dropped — see Fetch's doc comment
			// for why (two items can legitimately share a cursor value;
			// dedupe keys on provider ID, not timestamp, so re-scanning is
			// idempotent and safe).
			if cursor != "" && ts < cursor {
				stop = true
				continue
			}
			items = append(items, item)
			if ts > maxSeen {
				maxSeen = ts
			}
		}
		if stop {
			nextPath = ""
			break
		}
		nextPath = next
	}

	// nextPath is non-empty here only if the loop ran out of page budget
	// while the provider still had a next page to offer — both "natural"
	// exit paths (exhaustion, incremental stop) explicitly clear it above.
	return items, maxSeen, nextPath != "", nextPath, nil
}

// paginatedResponse is the common wire shape for every Trading 212 history
// endpoint used here: {"items": [...], "nextPagePath": "..."}. Verified
// 2026-07-03 against the published OpenAPI spec.
type paginatedResponse[T any] struct {
	Items        []T    `json:"items"`
	NextPagePath string `json:"nextPagePath"`
}

// rawTransactionItem mirrors one /equity/history/transactions item.
type rawTransactionItem struct {
	ID        json.Number `json:"id"`
	Type      string      `json:"type"`
	DateTime  string      `json:"dateTime"`
	Amount    json.Number `json:"amount"`
	Currency  string      `json:"currency"`
	Reference string      `json:"reference"`
	Notes     string      `json:"notes"`
}

// rawInstrument mirrors the shared "Instrument" schema (ticker/isin/name/currency).
type rawInstrument struct {
	Ticker   string `json:"ticker"`
	ISIN     string `json:"isin"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// rawFillWalletImpact mirrors Fill.walletImpact — netValue is the fill's
// actual net cash effect (fees/taxes already netted by the provider).
type rawFillWalletImpact struct {
	Currency string      `json:"currency"`
	NetValue json.Number `json:"netValue"`
}

// rawFill mirrors one order's "fill" object (/equity/history/orders).
type rawFill struct {
	ID           json.Number         `json:"id"`
	FilledAt     string              `json:"filledAt"`
	Price        json.Number         `json:"price"`
	Quantity     json.Number         `json:"quantity"`
	WalletImpact rawFillWalletImpact `json:"walletImpact"`
}

// rawOrder mirrors one order's "order" object (/equity/history/orders).
type rawOrder struct {
	ID         json.Number   `json:"id"`
	Ticker     string        `json:"ticker"`
	Instrument rawInstrument `json:"instrument"`
	Side       string        `json:"side"`
	Currency   string        `json:"currency"`
}

// rawHistoricalOrder mirrors one /equity/history/orders item: {order, fill}.
type rawHistoricalOrder struct {
	Order rawOrder `json:"order"`
	Fill  rawFill  `json:"fill"`
}

// rawDividendItem mirrors one /equity/history/dividends item.
type rawDividendItem struct {
	Ticker     string        `json:"ticker"`
	Instrument rawInstrument `json:"instrument"`
	Quantity   json.Number   `json:"quantity"`
	Amount     json.Number   `json:"amount"`
	Currency   string        `json:"currency"`
	PaidOn     string        `json:"paidOn"`
	Reference  string        `json:"reference"`
	Type       string        `json:"type"`
}

func (f *Fetcher) fetchTransactionsPage(ctx context.Context, apiKey string, path string) ([]Movement, string, time.Duration, bool, error) {
	raw, next, retryAfter, shouldRetry, err := fetchRawPage[rawTransactionItem](ctx, f, apiKey, path, historyPath)
	if err != nil || shouldRetry {
		return nil, next, retryAfter, shouldRetry, err
	}
	out := make([]Movement, 0, len(raw))
	for _, item := range raw {
		id := item.Reference
		if id == "" {
			id = item.ID.String()
		}
		out = append(out, Movement{
			ID:        id,
			Type:      item.Type,
			Timestamp: item.DateTime,
			Amount:    item.Amount.String(),
			Currency:  item.Currency,
			Notes:     item.Notes,
		})
	}
	return out, next, retryAfter, shouldRetry, nil
}

func (f *Fetcher) fetchOrdersPage(ctx context.Context, apiKey string, path string) ([]OrderFill, string, time.Duration, bool, error) {
	raw, next, retryAfter, shouldRetry, err := fetchRawPage[rawHistoricalOrder](ctx, f, apiKey, path, ordersPath)
	if err != nil || shouldRetry {
		return nil, next, retryAfter, shouldRetry, err
	}
	out := make([]OrderFill, 0, len(raw))
	for _, item := range raw {
		ticker := item.Order.Ticker
		if ticker == "" {
			ticker = item.Order.Instrument.Ticker
		}
		out = append(out, OrderFill{
			FillID:           item.Fill.ID.String(),
			OrderID:          item.Order.ID.String(),
			Ticker:           ticker,
			ISIN:             item.Order.Instrument.ISIN,
			Side:             item.Order.Side,
			Quantity:         item.Fill.Quantity.String(),
			Price:            item.Fill.Price.String(),
			Currency:         item.Order.Currency,
			FilledAt:         item.Fill.FilledAt,
			NetValue:         item.Fill.WalletImpact.NetValue.String(),
			NetValueCurrency: item.Fill.WalletImpact.Currency,
		})
	}
	return out, next, retryAfter, shouldRetry, nil
}

func (f *Fetcher) fetchDividendsPage(ctx context.Context, apiKey string, path string) ([]Dividend, string, time.Duration, bool, error) {
	raw, next, retryAfter, shouldRetry, err := fetchRawPage[rawDividendItem](ctx, f, apiKey, path, dividendsPath)
	if err != nil || shouldRetry {
		return nil, next, retryAfter, shouldRetry, err
	}
	out := make([]Dividend, 0, len(raw))
	for _, item := range raw {
		ticker := item.Ticker
		if ticker == "" {
			ticker = item.Instrument.Ticker
		}
		out = append(out, Dividend{
			Reference: item.Reference,
			Ticker:    ticker,
			ISIN:      item.Instrument.ISIN,
			Quantity:  item.Quantity.String(),
			Amount:    item.Amount.String(),
			Currency:  item.Currency,
			PaidOn:    item.PaidOn,
			Type:      item.Type,
		})
	}
	return out, next, retryAfter, shouldRetry, nil
}

// fetchRawPage performs one HTTP GET against path (defaulting to
// defaultPath when empty) and decodes the response as
// paginatedResponse[T] — the HTTP/auth/status/size-limit mechanics shared by
// every history endpoint. shouldRetry=true with a delay means the caller
// should back off and retry this same page (429 within-call throttling; the
// durable queue's own retry/backoff handles cross-restart retries).
func fetchRawPage[T any](ctx context.Context, f *Fetcher, apiKey string, path string, defaultPath string) (items []T, nextPagePath string, retryAfter time.Duration, shouldRetry bool, err error) {
	if path == "" {
		path = defaultPath
	}
	// nextPagePath after the first page comes from the provider's own
	// response. A relative path is the only shape the documented API
	// returns; refuse anything else so a malicious/misconfigured provider
	// (or MITM'd/compromised response) can't redirect the Authorization
	// header carrying the user's real Trading 212 API key to an arbitrary host.
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return nil, "", 0, false, fmt.Errorf("trading212: refusing to follow absolute nextPagePath %q; only paths relative to the configured base URL are trusted with the API key", path)
	}
	url := f.baseURL + path

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", 0, false, fmt.Errorf("trading212: build request: %w", err)
	}
	request.Header.Set("Authorization", apiKey)
	request.Header.Set("Accept", "application/json")

	response, err := f.client.Do(request)
	if err != nil {
		return nil, "", 0, false, fmt.Errorf("trading212: request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, "", 0, false, ErrUnauthorized
	}
	if response.StatusCode == http.StatusTooManyRequests {
		delay, ok := parseRetryAfter(response.Header.Get("Retry-After"))
		if !ok {
			return nil, "", 0, false, ErrRateLimited
		}
		return nil, "", delay, true, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", 0, false, fmt.Errorf("trading212: unexpected status %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return nil, "", 0, false, fmt.Errorf("trading212: read response: %w", err)
	}

	var parsed paginatedResponse[T]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", 0, false, fmt.Errorf("trading212: parse response: %w", err)
	}

	return parsed.Items, parsed.NextPagePath, 0, false, nil
}

// parseRetryAfter reads the Retry-After header (seconds, per RFC 9110).
// ok is false if the header is absent or malformed, so the caller can tell
// "no header" apart from a valid zero-second value.
func parseRetryAfter(value string) (delay time.Duration, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

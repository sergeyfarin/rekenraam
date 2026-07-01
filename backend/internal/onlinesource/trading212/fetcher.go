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

// historyPath is the cash/transaction history endpoint. Confirmed against
// the live API on first implementation; isolated here so a path or field
// rename is a one-line fix.
const historyPath = "/history/transactions"

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
	_, _, _, _, err := f.fetchPage(ctx, apiKey, "")
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
	var all []Movement
	nextPath := historyPath
	if resumeFrom != "" {
		nextPath = resumeFrom
	}
	maxSeen := cursor

	for page := 0; page < maxPages; page++ {
		if nextPath == "" {
			break
		}
		items, next, retryAfter, shouldRetry, err := f.fetchPage(ctx, apiKey, nextPath)
		if shouldRetry {
			select {
			case <-ctx.Done():
				return FetchResult{}, ctx.Err()
			case <-time.After(retryAfter):
			}
			continue // retry the same page
		}
		if err != nil {
			return FetchResult{}, err
		}

		stop := false
		for _, item := range items {
			// Strictly less-than: a movement whose timestamp equals the
			// cursor is re-scanned rather than dropped. The cursor is a
			// single timestamp, not a per-movement watermark, so two
			// movements can legitimately share it; treating "<=" as already
			// seen silently drops whichever one didn't make it into the
			// batch that first advanced the cursor to that instant. Provider
			// IDs (not timestamps) are the dedupe key, so re-scanning
			// already-known movements here is idempotent and safe.
			if cursor != "" && item.Timestamp < cursor {
				stop = true
				continue
			}
			all = append(all, item)
			if item.Timestamp > maxSeen {
				maxSeen = item.Timestamp
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
	return FetchResult{Movements: all, Cursor: maxSeen, HasMore: nextPath != "", NextPageToken: nextPath}, nil
}

// transactionHistoryResponse mirrors the Trading 212 /history/transactions
// payload. Field names are assumptions documented in the import plan —
// verify against the live API and adjust only this struct if they drift.
type transactionHistoryResponse struct {
	Items []struct {
		ID        json.Number `json:"id"`
		Type      string      `json:"type"`
		DateTime  string      `json:"dateTime"`
		Amount    json.Number `json:"amount"`
		Currency  string      `json:"currency"`
		Reference string      `json:"reference"`
		Notes     string      `json:"notes"`
	} `json:"items"`
	NextPagePath string `json:"nextPagePath"`
}

// fetchPage performs one HTTP GET and returns the page's movements, the next
// page path (empty if exhausted), and shouldRetry=true with a delay if the
// caller should back off and retry this same page (429 within-call
// throttling; the durable queue's own retry/backoff handles cross-restart
// retries).
func (f *Fetcher) fetchPage(ctx context.Context, apiKey string, path string) (movements []Movement, nextPagePath string, retryAfter time.Duration, shouldRetry bool, err error) {
	if path == "" {
		path = historyPath
	}
	// nextPagePath after the first page comes from the provider's own
	// response (transactionHistoryResponse.NextPagePath). A relative path is
	// the only shape the documented API returns; refuse anything else so a
	// malicious/misconfigured provider (or MITM'd/compromised response)
	// can't redirect the Authorization header carrying the user's real
	// Trading 212 API key to an arbitrary host.
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

	var parsed transactionHistoryResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", 0, false, fmt.Errorf("trading212: parse response: %w", err)
	}

	out := make([]Movement, 0, len(parsed.Items))
	for _, item := range parsed.Items {
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

	return out, parsed.NextPagePath, 0, false, nil
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

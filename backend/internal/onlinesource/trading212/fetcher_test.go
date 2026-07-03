package trading212

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// TestFetchHitsRealHistoryPath locks in the corrected endpoint path — the
// original Slice 2 path (`/history/transactions`, missing `/equity`) 404s
// against the real Trading 212 API; this guards against that regressing.
func TestFetchHitsRealHistoryPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/transactions" {
			t.Fatalf("expected /equity/history/transactions, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{"items": []map[string]any{}, "nextPagePath": ""})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	if _, err := fetcher.Fetch(context.Background(), "test-key", "", ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
}

func TestFetchSinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Fatalf("expected Authorization header test-key, got %q", got)
		}
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "100.00", "currency": "EUR", "reference": "ref-1"},
				{"id": 2, "type": "DIVIDEND", "dateTime": "2024-01-02T00:00:00Z", "amount": "1.23", "currency": "EUR", "reference": "ref-2"},
			},
			"nextPagePath": "",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("expected 2 movements, got %d", len(result.Movements))
	}
	if result.Cursor != "2024-01-02T00:00:00Z" {
		t.Fatalf("expected cursor to be max timestamp, got %q", result.Cursor)
	}
	if result.Movements[0].ID != "ref-1" || result.Movements[0].Amount != "100.00" {
		t.Fatalf("unexpected first movement: %+v", result.Movements[0])
	}
}

func TestFetchPaginatesAcrossPages(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.RawQuery == "cursor=page2" {
			writeJSON(w, map[string]any{
				"items": []map[string]any{
					{"id": 2, "type": "DIVIDEND", "dateTime": "2024-01-02T00:00:00Z", "amount": "1.23", "currency": "EUR", "reference": "ref-2"},
				},
				"nextPagePath": "",
			})
			return
		}
		if r.URL.Path == "/equity/history/transactions" {
			writeJSON(w, map[string]any{
				"items": []map[string]any{
					{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "100.00", "currency": "EUR", "reference": "ref-1"},
				},
				"nextPagePath": "/equity/history/transactions?cursor=page2",
			})
			return
		}
		t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", calls)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("expected 2 movements across pages, got %d", len(result.Movements))
	}
}

func TestFetchReportsHasMoreWhenPageBudgetExhausted(t *testing.T) {
	old := maxPages
	maxPages = 3
	t.Cleanup(func() { maxPages = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		if page == "" {
			page = "0"
		}
		n, _ := strconv.Atoi(page)
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				{"id": n, "type": "DEPOSIT", "dateTime": fmt.Sprintf("2024-01-%02dT00:00:00Z", n+1), "amount": "1.00", "currency": "EUR", "reference": fmt.Sprintf("ref-%d", n)},
			},
			// Always another page: this provider never naturally exhausts,
			// so hitting maxPages is the only thing that can stop Fetch.
			"nextPagePath": fmt.Sprintf("/history/transactions?p=%d", n+1),
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !result.HasMore {
		t.Fatal("expected HasMore=true when the provider still has pages beyond the budget")
	}
	if len(result.Movements) != maxPages {
		t.Fatalf("expected exactly %d movements (the page budget), got %d", maxPages, len(result.Movements))
	}
}

func TestFetchHasMoreFalseWhenProviderExhaustsNaturally(t *testing.T) {
	old := maxPages
	maxPages = 3
	t.Cleanup(func() { maxPages = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"items":        []map[string]any{{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "1.00", "currency": "EUR", "reference": "ref-1"}},
			"nextPagePath": "",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.HasMore {
		t.Fatal("expected HasMore=false when the provider naturally exhausts before the page budget")
	}
}

func TestFetchHasMoreFalseWhenIncrementalCursorStops(t *testing.T) {
	old := maxPages
	maxPages = 3
	t.Cleanup(func() { maxPages = old })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			// Strictly before the cursor below, so the incremental stop
			// fires (not the same-timestamp re-scan from the "<" fix).
			"items": []map[string]any{
				{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "1.00", "currency": "EUR", "reference": "ref-1"},
			},
			// Would keep paging forever if not for the incremental stop.
			"nextPagePath": "/history/transactions?p=1",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "2024-01-01T00:00:01Z", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.HasMore {
		t.Fatal("expected HasMore=false when the incremental cursor stopped pagination, not the page budget")
	}
}

func TestFetchRefusesAbsoluteNextPagePath(t *testing.T) {
	trapCalled := false
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trapCalled = true
		if got := r.Header.Get("Authorization"); got == "test-key" {
			t.Fatal("API key was sent to the untrusted absolute nextPagePath host")
		}
	}))
	defer trap.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "100.00", "currency": "EUR", "reference": "ref-1"},
			},
			// A malicious/compromised/misconfigured provider response
			// pointing pagination at an attacker-controlled absolute URL.
			"nextPagePath": trap.URL + "/steal",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	_, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err == nil {
		t.Fatal("expected Fetch to reject the absolute nextPagePath, got nil error")
	}
	if trapCalled {
		t.Fatal("the untrusted absolute nextPagePath host was contacted")
	}
}

func TestFetchIncrementalStopsAtCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "100.00", "currency": "EUR", "reference": "ref-1"},
				{"id": 2, "type": "DIVIDEND", "dateTime": "2024-01-02T00:00:00Z", "amount": "1.23", "currency": "EUR", "reference": "ref-2"},
				{"id": 3, "type": "FEE", "dateTime": "2024-01-03T00:00:00Z", "amount": "-0.50", "currency": "EUR", "reference": "ref-3"},
			},
			"nextPagePath": "",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "2024-01-02T00:00:00Z", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// ref-2 shares the cursor's exact timestamp and is re-scanned (not
	// dropped) — see TestFetchIncludesMovementsAtExactCursorTimestamp for why
	// "<=" would silently lose same-timestamp movements. ref-1 (strictly
	// before the cursor) is still excluded.
	if len(result.Movements) != 2 || result.Movements[0].ID != "ref-2" || result.Movements[1].ID != "ref-3" {
		t.Fatalf("expected movements at or after cursor, got %+v", result.Movements)
	}
	if result.Cursor != "2024-01-03T00:00:00Z" {
		t.Fatalf("expected new cursor to advance, got %q", result.Cursor)
	}
}

// TestFetchIncludesMovementsAtExactCursorTimestamp guards against a real
// regression: an incremental fetch whose cursor is set to the timestamp
// shared by two distinct movements (e.g. a prior fetch's page boundary split
// them, or the provider timestamp resolution is coarser than actual event
// order) must not silently drop the one that wasn't recorded when the cursor
// was first advanced to that instant.
func TestFetchIncludesMovementsAtExactCursorTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-02T00:00:00Z", "amount": "50.00", "currency": "EUR", "reference": "ref-same-timestamp"},
			},
			"nextPagePath": "",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "2024-01-02T00:00:00Z", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(result.Movements) != 1 || result.Movements[0].ID != "ref-same-timestamp" {
		t.Fatalf("expected the same-timestamp movement to be re-scanned, not dropped, got %+v", result.Movements)
	}
}

func TestFetchHonorsRetryAfter(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, map[string]any{
			"items":        []map[string]any{{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "1.00", "currency": "EUR", "reference": "ref-1"}},
			"nextPagePath": "",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected a retry after 429, got %d calls", calls)
	}
	if len(result.Movements) != 1 {
		t.Fatalf("expected fetch to succeed after retry, got %+v", result)
	}
}

func TestFetchRateLimitedWithoutRetryAfterReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	_, err := fetcher.Fetch(context.Background(), "test-key", "", "")
	if err != ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestFetchUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	_, err := fetcher.Fetch(context.Background(), "bad-key", "", "")
	if err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestProbeUsesAccountSummaryEndpointAndSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/account/summary" {
			t.Fatalf("expected probe to hit the account summary endpoint, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	if err := fetcher.Probe(context.Background(), "test-key"); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestProbeUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	if err := fetcher.Probe(context.Background(), "bad-key"); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestFetchRespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	fetcher := NewFetcher(server.Client(), server.URL)
	_, err := fetcher.Fetch(ctx, "test-key", "", "")
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- FetchOrders / FetchDividends (B-T212-INVST / Slice 4b) ---

func TestFetchOrdersHitsRealPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/orders" {
			t.Fatalf("expected /equity/history/orders, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{"items": []map[string]any{}, "nextPagePath": ""})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	if _, err := fetcher.FetchOrders(context.Background(), "test-key", "", ""); err != nil {
		t.Fatalf("FetchOrders: %v", err)
	}
}

func TestFetchDividendsHitsRealPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/equity/history/dividends" {
			t.Fatalf("expected /equity/history/dividends, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{"items": []map[string]any{}, "nextPagePath": ""})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	if _, err := fetcher.FetchDividends(context.Background(), "test-key", "", ""); err != nil {
		t.Fatalf("FetchDividends: %v", err)
	}
}

// writeGoldenOrderJSON mirrors the real /equity/history/orders response
// shape verified 2026-07-03 against the published OpenAPI spec: a
// {order, fill} pair per item, not a flat object.
func writeGoldenOrderJSON(w http.ResponseWriter, nextPagePath string) {
	writeJSON(w, map[string]any{
		"items": []map[string]any{
			{
				"order": map[string]any{
					"id": 111, "ticker": "AAPL_US_EQ", "side": "BUY", "currency": "USD",
					"instrument": map[string]any{"ticker": "AAPL_US_EQ", "isin": "US0378331005", "name": "Apple Inc.", "currency": "USD"},
				},
				"fill": map[string]any{
					"id": 222, "filledAt": "2024-02-01T10:00:00Z", "price": "150.25", "quantity": "2",
					"walletImpact": map[string]any{"currency": "USD", "netValue": "-300.75"},
				},
			},
		},
		"nextPagePath": nextPagePath,
	})
}

func TestFetchOrdersParsesRealShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGoldenOrderJSON(w, "")
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.FetchOrders(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("FetchOrders: %v", err)
	}
	if len(result.Fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(result.Fills))
	}
	fill := result.Fills[0]
	if fill.FillID != "222" || fill.OrderID != "111" {
		t.Fatalf("unexpected ids: %+v", fill)
	}
	if fill.Ticker != "AAPL_US_EQ" || fill.ISIN != "US0378331005" {
		t.Fatalf("unexpected instrument fields: %+v", fill)
	}
	if fill.Side != "BUY" || fill.Quantity != "2" || fill.Price != "150.25" || fill.Currency != "USD" {
		t.Fatalf("unexpected order/fill fields: %+v", fill)
	}
	if fill.NetValue != "-300.75" {
		t.Fatalf("expected wallet impact net value passed through raw, got %q", fill.NetValue)
	}
	if fill.FilledAt != "2024-02-01T10:00:00Z" {
		t.Fatalf("unexpected filledAt: %q", fill.FilledAt)
	}
	if result.Cursor != "2024-02-01T10:00:00Z" {
		t.Fatalf("expected cursor to track filledAt, got %q", result.Cursor)
	}
}

func TestFetchOrdersIncrementalCursorStopsOnFilledAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"items": []map[string]any{
				{
					"order": map[string]any{"id": 1, "ticker": "AAPL_US_EQ", "side": "BUY", "currency": "USD", "instrument": map[string]any{"ticker": "AAPL_US_EQ", "isin": "US0378331005"}},
					"fill":  map[string]any{"id": 1, "filledAt": "2024-01-01T00:00:00Z", "price": "1", "quantity": "1", "walletImpact": map[string]any{"netValue": "-1"}},
				},
			},
			"nextPagePath": "/equity/history/orders?p=1",
		})
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.FetchOrders(context.Background(), "test-key", "2024-01-01T00:00:01Z", "")
	if err != nil {
		t.Fatalf("FetchOrders: %v", err)
	}
	if len(result.Fills) != 0 {
		t.Fatalf("expected the fill strictly before the cursor to stop pagination without being included, got %d", len(result.Fills))
	}
	if result.HasMore {
		t.Fatal("expected HasMore=false when the incremental cursor stopped pagination")
	}
}

// writeGoldenDividendJSON mirrors the real /equity/history/dividends response shape.
func writeGoldenDividendJSON(w http.ResponseWriter, nextPagePath string) {
	writeJSON(w, map[string]any{
		"items": []map[string]any{
			{
				"ticker": "AAPL_US_EQ", "quantity": "10", "amount": "4.32", "currency": "EUR",
				"paidOn": "2024-03-01T00:00:00Z", "reference": "div-ref-1", "type": "DIVIDEND",
				"instrument": map[string]any{"ticker": "AAPL_US_EQ", "isin": "US0378331005", "name": "Apple Inc.", "currency": "USD"},
			},
		},
		"nextPagePath": nextPagePath,
	})
}

func TestFetchDividendsParsesRealShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGoldenDividendJSON(w, "")
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.FetchDividends(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("FetchDividends: %v", err)
	}
	if len(result.Dividends) != 1 {
		t.Fatalf("expected 1 dividend, got %d", len(result.Dividends))
	}
	div := result.Dividends[0]
	if div.Reference != "div-ref-1" || div.Ticker != "AAPL_US_EQ" || div.ISIN != "US0378331005" {
		t.Fatalf("unexpected identity fields: %+v", div)
	}
	if div.Quantity != "10" || div.Amount != "4.32" || div.Currency != "EUR" {
		t.Fatalf("unexpected amount fields: %+v", div)
	}
	if div.Type != "DIVIDEND" || div.PaidOn != "2024-03-01T00:00:00Z" {
		t.Fatalf("unexpected type/paidOn: %+v", div)
	}
	if result.Cursor != "2024-03-01T00:00:00Z" {
		t.Fatalf("expected cursor to track paidOn, got %q", result.Cursor)
	}
}

func TestFetchDividendsPaginatesAcrossPages(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.RawQuery == "cursor=page2" {
			writeJSON(w, map[string]any{
				"items": []map[string]any{
					{"ticker": "MSFT_US_EQ", "quantity": "1", "amount": "1.00", "currency": "EUR", "paidOn": "2024-03-02T00:00:00Z", "reference": "div-ref-2", "type": "DIVIDEND"},
				},
				"nextPagePath": "",
			})
			return
		}
		writeGoldenDividendJSON(w, "/equity/history/dividends?cursor=page2")
	}))
	defer server.Close()

	fetcher := NewFetcher(server.Client(), server.URL)
	result, err := fetcher.FetchDividends(context.Background(), "test-key", "", "")
	if err != nil {
		t.Fatalf("FetchDividends: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", calls)
	}
	if len(result.Dividends) != 2 {
		t.Fatalf("expected 2 dividends across pages, got %d", len(result.Dividends))
	}
}

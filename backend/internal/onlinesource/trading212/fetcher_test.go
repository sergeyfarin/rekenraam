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
		if r.URL.Path == "/history/transactions" {
			writeJSON(w, map[string]any{
				"items": []map[string]any{
					{"id": 1, "type": "DEPOSIT", "dateTime": "2024-01-01T00:00:00Z", "amount": "100.00", "currency": "EUR", "reference": "ref-1"},
				},
				"nextPagePath": "/history/transactions?cursor=page2",
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

func TestProbeUsesHistoryEndpointAndSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/history/transactions" {
			t.Fatalf("expected probe to hit history endpoint, got %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{"items": []map[string]any{}, "nextPagePath": ""})
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

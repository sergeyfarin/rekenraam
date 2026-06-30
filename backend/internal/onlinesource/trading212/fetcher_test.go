package trading212

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	result, err := fetcher.Fetch(context.Background(), "test-key", "")
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
	result, err := fetcher.Fetch(context.Background(), "test-key", "")
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
	result, err := fetcher.Fetch(context.Background(), "test-key", "2024-01-02T00:00:00Z")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(result.Movements) != 1 || result.Movements[0].ID != "ref-3" {
		t.Fatalf("expected only movements after cursor, got %+v", result.Movements)
	}
	if result.Cursor != "2024-01-03T00:00:00Z" {
		t.Fatalf("expected new cursor to advance, got %q", result.Cursor)
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
	result, err := fetcher.Fetch(context.Background(), "test-key", "")
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
	_, err := fetcher.Fetch(context.Background(), "test-key", "")
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
	_, err := fetcher.Fetch(context.Background(), "bad-key", "")
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
	_, err := fetcher.Fetch(ctx, "test-key", "")
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Package trading212 isolates the Trading 212 public API's HTTP shape from
// the import pipeline. It has no DB access and no ledger knowledge: it turns
// an API key + cursor into a stable internal []Movement, which the
// app.Trading212Adapter then turns into canonical StagedRows.
package trading212

import "errors"

var (
	// ErrUnauthorized means the provider rejected the API key (401/403).
	ErrUnauthorized = errors.New("trading212: provider rejected the API key")
	// ErrRateLimited means the provider returned 429 after the fetcher's own
	// in-call backoff was exhausted. The caller (durable queue) should retry later.
	ErrRateLimited = errors.New("trading212: rate limited by provider")
)

// Movement is the stable internal shape produced by Fetch, independent of
// the exact provider JSON field names (isolated here per the import plan's
// risk note — a field rename is a one-line fix in history.go, not a
// pipeline change).
type Movement struct {
	ID        string // provider transaction id / reference — the strong dedupe key
	Type      string // "DEPOSIT" | "WITHDRAWAL" | "DIVIDEND" | "INTEREST" | "FEE" | "CARD" | "BUY" | "SELL" | ...
	Timestamp string // RFC3339, as provided
	Amount    string // raw decimal string, sign per movement direction
	Currency  string // ISO 4217 code
	Notes     string // free-text description/counterparty, when present
}

// FetchResult is what Fetch returns: the movements seen plus the cursor to
// pass to the next incremental fetch.
type FetchResult struct {
	Movements []Movement
	Cursor    string // max movement timestamp seen this call; empty if no movements
	// HasMore is true only when Fetch stopped because it exhausted maxPages
	// while the provider still had more pages to offer — never when it
	// stopped naturally (provider reported no next page) or hit the
	// incremental boundary. NextPageToken is the exact page to pass as
	// resumeFrom on a follow-up Fetch call to continue toward older history.
	//
	// This is deliberately a *different* continuation mechanism from the
	// incremental boundary argument: the boundary says "stop once you reach
	// data I already have", which is fixed for a whole logical fetch and
	// would immediately (and wrongly) re-trigger on page 2 of a follow-up
	// call if used as a resume point, because every Fetch call restarts
	// pagination at page 1. NextPageToken instead says "carry on turning
	// pages from exactly here", independent of the boundary.
	HasMore       bool
	NextPageToken string
}

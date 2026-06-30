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
	Cursor    string // max movement timestamp seen; empty if no movements
}

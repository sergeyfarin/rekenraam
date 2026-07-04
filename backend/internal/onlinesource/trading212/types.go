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
	ID   string // provider transaction id / reference — the strong dedupe key
	Type string // real /equity/history/transactions enum: "WITHDRAW" | "DEPOSIT" | "FEE" | "TRANSFER"
	// (verified 2026-07-03 against the published OpenAPI spec — dividends
	// and BUY/SELL order fills are never emitted by this endpoint; they come
	// from /equity/history/dividends and /equity/history/orders respectively,
	// see B-T212-INVST/Slice 4b).
	Timestamp string // RFC3339, as provided
	Amount    string // raw decimal string, sign per movement direction
	Currency  string // ISO 4217 code
	Notes     string // not present on the real payload today; kept for forward-compat, always empty in practice
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

// OrderFill is the stable internal shape produced by FetchOrders — one per
// execution event (a partially-filled order produces multiple fills, each
// with its own FillID). Verified 2026-07-03 against the published OpenAPI
// spec for GET /equity/history/orders (B-T212-INVST/Slice 4b).
type OrderFill struct {
	FillID   string // fill.id — the actual execution event; the strong dedupe key
	OrderID  string // order.id — links partial fills of the same order together
	Ticker   string // e.g. "AAPL_US_EQ"
	ISIN     string
	Side     string // "BUY" | "SELL"
	Quantity string // decimal string, shares filled in this fill
	Price    string // decimal string, fill price, denominated in Currency
	Currency string // ISO 4217, the order's own trading currency (for Price)
	FilledAt string // RFC3339
	// NetValue is fill.walletImpact.netValue: the actual net cash effect of
	// this fill (fees/taxes already netted in by the provider) — this is
	// what the ledger's cash leg must move, not quantity*price recomputed
	// from scratch, per "no floating point" (never re-derive money via
	// arithmetic on decimal strings in Go).
	//
	// NetValueCurrency (walletImpact.currency) is NOT necessarily the same
	// as Currency: a multi-currency Trading 212 account can trade an
	// instrument priced in one currency while its wallet settles in
	// another (the provider converts internally) — using Currency for
	// NetValue would silently mislabel the cash leg's currency whenever
	// they differ. Always pair NetValue with NetValueCurrency, never Currency.
	NetValue         string
	NetValueCurrency string
}

// OrdersFetchResult mirrors FetchResult for the orders endpoint.
type OrdersFetchResult struct {
	Fills         []OrderFill
	Cursor        string
	HasMore       bool
	NextPageToken string
}

// Dividend is the stable internal shape produced by FetchDividends.
// Verified 2026-07-03 against the published OpenAPI spec for
// GET /equity/history/dividends (B-T212-INVST/Slice 4b).
type Dividend struct {
	Reference string // strong dedupe key
	Ticker    string
	ISIN      string
	Quantity  string // decimal string
	// Amount is in the account's primary currency — the actual cash
	// credited, distinct from GrossAmountPerShare (instrument currency).
	Amount   string
	Currency string // account's primary currency
	PaidOn   string // RFC3339
	Type     string // real enum, e.g. "DIVIDEND", "INTEREST", ...
}

// DividendsFetchResult mirrors FetchResult for the dividends endpoint.
type DividendsFetchResult struct {
	Dividends     []Dividend
	Cursor        string
	HasMore       bool
	NextPageToken string
}

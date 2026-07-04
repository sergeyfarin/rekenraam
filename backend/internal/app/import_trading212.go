package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"rekenraam/backend/internal/onlinesource/trading212"
)

// trading212FetchPayload is the JSON shape the fetch worker (Slice 3) writes
// into RawInput.Bytes: the connection id (for fingerprint scoping) plus
// whichever endpoint's items this work item's stage fetched. The adapter
// never talks HTTP — it only unmarshals this envelope. Exactly one of
// Movements/OrderFills/Dividends is populated per call (the fetch worker
// processes one stage — transactions, orders, or dividends — per work
// item), but Parse doesn't assume that; it processes whichever are present.
type trading212FetchPayload struct {
	ConnectionID int64                 `json:"connection_id"`
	Movements    []trading212Movement  `json:"movements,omitempty"`
	OrderFills   []trading212OrderFill `json:"order_fills,omitempty"`
	Dividends    []trading212Dividend  `json:"dividends,omitempty"`
}

// trading212Movement mirrors trading212.Movement; duplicated here (rather
// than importing internal/onlinesource/trading212) so the adapter's input
// contract is just JSON, matching every other SourceAdapter and keeping
// internal/app free of the provider HTTP package.
type trading212Movement struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Notes     string `json:"notes"`
}

// trading212OrderFill mirrors trading212.OrderFill (B-T212-INVST/Slice 4b).
type trading212OrderFill struct {
	FillID   string `json:"fill_id"`
	OrderID  string `json:"order_id"`
	Ticker   string `json:"ticker"`
	ISIN     string `json:"isin"`
	Side     string `json:"side"`
	Quantity string `json:"quantity"`
	Price    string `json:"price"` // denominated in Currency (the instrument's trading currency)
	Currency string `json:"currency"`
	FilledAt string `json:"filled_at"`
	// NetValue is denominated in NetValueCurrency, which is NOT always the
	// same as Currency for a multi-currency account — see trading212.OrderFill's
	// doc comment. Always pair the two.
	NetValue         string `json:"net_value"`
	NetValueCurrency string `json:"net_value_currency"`
}

// trading212Dividend mirrors trading212.Dividend (B-T212-INVST/Slice 4b).
type trading212Dividend struct {
	Reference string `json:"reference"`
	Ticker    string `json:"ticker"`
	ISIN      string `json:"isin"`
	Quantity  string `json:"quantity"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	PaidOn    string `json:"paid_on"`
	Type      string `json:"type"`
}

// Raw map keys used by the investment commit-path branch (import_service.go
// CommitImportBatch) to recognize and route Trading 212 order-fill/dividend
// rows. "kind" distinguishes these from plain cash-movement rows (which have
// no "kind" key at all — checked with a plain map lookup, not a type field
// on StagedRow, to avoid a schema change to the generic pipeline).
const (
	trading212RawKindOrderFill = "trading212_order_fill"
	trading212RawKindDividend  = "trading212_dividend"
)

// Raw map keys shared by both row kinds and set by the fetch worker's
// resolution pass (import_fetch_worker.go) once instrument/holding-account
// lookups run — never by Parse itself, which has no DB access.
const (
	rawKeyKind                = "kind"
	rawKeyTicker              = "ticker"
	rawKeyISIN                = "isin"
	rawKeySide                = "side"
	rawKeyQuantity            = "quantity"
	rawKeyPrice               = "price"
	rawKeyOrderID             = "order_id"
	rawKeyResolvedCommodityID = "resolved_commodity_id"
	rawKeyResolvedHoldingID   = "resolved_holding_account_id"
)

// cashMovementTypes are the real `type` enum values the Trading 212
// `/equity/history/transactions` endpoint documents (verified 2026-07-03
// against the published OpenAPI spec at docs.trading212.com — the original
// Slice 2 list was an unverified guess and included values, e.g. DIVIDEND,
// INTEREST, CARD_*, TRANSFER_IN/OUT, that this endpoint never actually
// emits; dividends live entirely on the separate /equity/history/dividends
// endpoint, see B-T212-INVST). Anything outside this set (in practice, only
// BUY/SELL order fills once the orders endpoint is wired in Slice 4b) is
// staged but flagged needs_attention.
var cashMovementTypes = map[string]bool{
	"WITHDRAW": true,
	"DEPOSIT":  true,
	"FEE":      true,
	"TRANSFER": true,
}

// Trading212Adapter implements SourceAdapter for Trading 212 cash-movement
// history. It never talks HTTP — Parse only unmarshals the JSON envelope the
// fetch worker (Slice 3) writes into RawInput.Bytes. Detect always reports
// ConfidenceNone: online sources are never auto-detected from an uploaded
// file, only explicitly selected via connection_id.
type Trading212Adapter struct{}

func (a *Trading212Adapter) Kind() string { return "trading212" }

func (a *Trading212Adapter) Detect(input RawInput) Confidence {
	return ConfidenceNone
}

func (a *Trading212Adapter) Parse(ctx context.Context, input RawInput, profile *ImportProfile) (ParseResult, error) {
	var payload trading212FetchPayload
	if err := json.Unmarshal(input.Bytes, &payload); err != nil {
		return ParseResult{}, fmt.Errorf("parse trading212 fetch payload: %w", err)
	}
	if payload.ConnectionID <= 0 {
		return ParseResult{}, fmt.Errorf("trading212 fetch payload missing connection_id")
	}

	var result ParseResult
	addRow := func(row StagedRow, warning string) {
		rowIndex := len(result.Rows)
		result.Rows = append(result.Rows, row)
		if warning != "" {
			result.Warnings = append(result.Warnings, ParseWarning{RowIndex: rowIndex, Message: warning})
		}
		if row.Date != "" {
			if result.Meta.DateFrom == "" || row.Date < result.Meta.DateFrom {
				result.Meta.DateFrom = row.Date
			}
			if result.Meta.DateTo == "" || row.Date > result.Meta.DateTo {
				result.Meta.DateTo = row.Date
			}
		}
		if row.CommodityHint != "" && !containsString(result.Meta.CurrencyHints, row.CommodityHint) {
			result.Meta.CurrencyHints = append(result.Meta.CurrencyHints, row.CommodityHint)
		}
	}

	for i, m := range payload.Movements {
		row, warning := trading212MovementToStagedRow(m, payload.ConnectionID, i)
		addRow(row, warning)
	}
	// No ParseWarning here for order fills/dividends: whether these need
	// manual attention depends on instrument/holding-account resolution,
	// which requires DB access Parse doesn't have. The fetch worker's
	// resolution pass (import_fetch_worker.go, runs after Parse) appends a
	// warning only for rows it couldn't fully resolve.
	for i, o := range payload.OrderFills {
		addRow(trading212OrderFillToStagedRow(o, payload.ConnectionID, i), "")
	}
	for i, d := range payload.Dividends {
		addRow(trading212DividendToStagedRow(d, payload.ConnectionID, i), "")
	}

	return result, nil
}

func trading212MovementToStagedRow(m trading212Movement, connectionID int64, occurrence int) (StagedRow, string) {
	raw := map[string]string{
		"id":        m.ID,
		"type":      m.Type,
		"timestamp": m.Timestamp,
		"amount":    m.Amount,
		"currency":  m.Currency,
		"notes":     m.Notes,
	}

	date := m.Timestamp
	if len(date) >= 10 {
		date = date[:10]
	}

	warning := ""
	if !cashMovementTypes[m.Type] {
		warning = fmt.Sprintf("movement type %q is not a cash movement; review before committing", m.Type)
	}

	fp := buildTrading212Fingerprint(connectionID, m.ID, occurrence)

	return StagedRow{
		DedupeFingerprint: fp,
		Date:              date,
		Amount:            m.Amount,
		CommodityHint:     strings.ToUpper(strings.TrimSpace(m.Currency)),
		PayeeHint:         m.Notes,
		Memo:              m.Type,
		ExternalRef:       m.ID,
		Raw:               raw,
	}, warning
}

// trading212OrderFillToStagedRow maps one order fill to a StagedRow. Amount/
// CommodityHint are always populated from NetValue/Currency (the fill's
// actual cash effect) so the row commits correctly as a plain cash
// transaction via the generic path if investment resolution never succeeds
// (no cash_account_id configured, no instrument match) — the Raw fields
// below are what lets CommitImportBatch upgrade it to a real Buy/Sell
// instead, once the fetch worker's resolution pass (import_fetch_worker.go)
// has filled in rawKeyResolvedCommodityID/rawKeyResolvedHoldingID.
func trading212OrderFillToStagedRow(o trading212OrderFill, connectionID int64, occurrence int) StagedRow {
	raw := map[string]string{
		rawKeyKind:     trading212RawKindOrderFill,
		rawKeyOrderID:  o.OrderID,
		rawKeyTicker:   o.Ticker,
		rawKeyISIN:     o.ISIN,
		rawKeySide:     o.Side,
		rawKeyQuantity: o.Quantity,
		rawKeyPrice:    o.Price,
		"net_value":    o.NetValue,
	}

	date := o.FilledAt
	if len(date) >= 10 {
		date = date[:10]
	}

	fp := buildTrading212OrderFingerprint(connectionID, o.FillID, occurrence)

	// Amount/CommodityHint use NetValue/NetValueCurrency (the wallet's
	// actual cash effect), NOT Price/Currency (the instrument's trading
	// currency) — they can differ on a multi-currency account. This is
	// what lets the row commit correctly as a plain cash transaction via
	// the generic path if investment resolution never succeeds.
	return StagedRow{
		DedupeFingerprint: fp,
		Date:              date,
		Amount:            o.NetValue,
		CommodityHint:     strings.ToUpper(strings.TrimSpace(o.NetValueCurrency)),
		PayeeHint:         o.Ticker,
		Memo:              strings.TrimSpace(o.Side + " " + o.Ticker),
		ExternalRef:       o.FillID,
		Raw:               raw,
	}
}

// trading212DividendToStagedRow maps one dividend to a StagedRow. Same
// deferred-resolution shape as order fills: Amount/CommodityHint alone are
// enough to commit as a plain cash transaction; the Raw fields let
// CommitImportBatch upgrade it to InvestmentService.Dividend once resolved.
func trading212DividendToStagedRow(d trading212Dividend, connectionID int64, occurrence int) StagedRow {
	raw := map[string]string{
		rawKeyKind:      trading212RawKindDividend,
		rawKeyTicker:    d.Ticker,
		rawKeyISIN:      d.ISIN,
		rawKeyQuantity:  d.Quantity,
		"dividend_type": d.Type,
	}

	date := d.PaidOn
	if len(date) >= 10 {
		date = date[:10]
	}

	fp := buildTrading212DividendFingerprint(connectionID, d.Reference, occurrence)

	return StagedRow{
		DedupeFingerprint: fp,
		Date:              date,
		Amount:            d.Amount,
		CommodityHint:     strings.ToUpper(strings.TrimSpace(d.Currency)),
		PayeeHint:         d.Ticker,
		Memo:              "Dividend: " + d.Ticker,
		ExternalRef:       d.Reference,
		Raw:               raw,
	}
}

// buildTrading212OrderFingerprint mirrors buildTrading212Fingerprint but
// namespaces order fills separately from cash movements (a different
// endpoint's provider ids, even though a collision is not realistically
// expected — cheap insurance against ever conflating the two).
func buildTrading212OrderFingerprint(connectionID int64, fillID string, occurrence int) string {
	if fillID == "" {
		return fmt.Sprintf("trading212|%d|order|noref|%d", connectionID, occurrence)
	}
	return fmt.Sprintf("trading212|%d|order|%s", connectionID, fillID)
}

// buildTrading212DividendFingerprint mirrors buildTrading212OrderFingerprint
// for dividends.
func buildTrading212DividendFingerprint(connectionID int64, reference string, occurrence int) string {
	if reference == "" {
		return fmt.Sprintf("trading212|%d|dividend|noref|%d", connectionID, occurrence)
	}
	return fmt.Sprintf("trading212|%d|dividend|%s", connectionID, reference)
}

// buildTrading212Fingerprint builds the pre-hash fingerprint seed: connection
// scoped so two connections (even to the same provider) can't collide, and
// including the provider transaction id so re-fetch overlap is idempotent.
// When the provider id is blank (shouldn't happen against the live API but
// guarded for malformed fixtures), occurrence keeps rows distinguishable
// instead of silently colliding into one fingerprint.
func buildTrading212Fingerprint(connectionID int64, providerID string, occurrence int) string {
	if providerID == "" {
		return fmt.Sprintf("trading212|%d|noref|%d", connectionID, occurrence)
	}
	return fmt.Sprintf("trading212|%d|%s", connectionID, providerID)
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// Trading212Prober implements ConnectionProber against the live Trading 212
// API, closing T-11 (the connection CRUD previously accepted any non-blank
// string as a valid key). It is only consulted for sourceKind="trading212";
// other source kinds fall through to NoOpProber until they have a real
// provider integration.
type Trading212Prober struct {
	client  *http.Client
	baseURL string
}

func NewTrading212Prober(client *http.Client) *Trading212Prober {
	return &Trading212Prober{client: client}
}

func (p *Trading212Prober) Probe(ctx context.Context, sourceKind string, apiKey string, configJSON string) error {
	if sourceKind != "trading212" {
		return nil
	}
	fetcher := trading212.NewFetcher(p.client, p.baseURL)
	if err := fetcher.Probe(ctx, apiKey); err != nil {
		if errors.Is(err, trading212.ErrUnauthorized) {
			return ErrProviderUnauthorized
		}
		return fmt.Errorf("trading212 probe: %w", err)
	}
	return nil
}

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
// into RawInput.Bytes: the connection id (for fingerprint scoping) plus the
// movements the trading212.Fetcher produced. The adapter never talks HTTP —
// it only unmarshals this envelope.
type trading212FetchPayload struct {
	ConnectionID int64                `json:"connection_id"`
	Movements    []trading212Movement `json:"movements"`
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

// cashMovementTypes are Trading 212 movement types that map directly to a
// single cash leg. Anything else (BUY/SELL order fills) is staged but
// flagged needs_attention — see B-T212-INVST.
var cashMovementTypes = map[string]bool{
	"DEPOSIT":          true,
	"WITHDRAWAL":       true,
	"DIVIDEND":         true,
	"INTEREST":         true,
	"FEE":              true,
	"CARD_CREDIT":      true,
	"CARD_DEBIT":       true,
	"CARD_TOPUP":       true,
	"TRANSFER_IN":      true,
	"TRANSFER_OUT":     true,
	"LENDING_INTEREST": true,
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
	for i, m := range payload.Movements {
		row, warning := trading212MovementToStagedRow(m, payload.ConnectionID, i)
		result.Rows = append(result.Rows, row)
		if warning != "" {
			result.Warnings = append(result.Warnings, ParseWarning{RowIndex: i, Message: warning})
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
	client *http.Client
}

func NewTrading212Prober(client *http.Client) *Trading212Prober {
	return &Trading212Prober{client: client}
}

func (p *Trading212Prober) Probe(ctx context.Context, sourceKind string, apiKey string, configJSON string) error {
	if sourceKind != "trading212" {
		return nil
	}
	baseURL := trading212BaseURLFromConfig(configJSON)
	fetcher := trading212.NewFetcher(p.client, baseURL)
	if err := fetcher.Probe(ctx, apiKey); err != nil {
		if errors.Is(err, trading212.ErrUnauthorized) {
			return ErrProviderUnauthorized
		}
		return fmt.Errorf("trading212 probe: %w", err)
	}
	return nil
}

// trading212BaseURLFromConfig reads config_json.base_url, the documented
// override for the demo/sandbox endpoint and tests. Malformed or absent
// config falls back to the fetcher's default (the live API).
func trading212BaseURLFromConfig(configJSON string) string {
	if configJSON == "" {
		return ""
	}
	var cfg struct {
		BaseURL string `json:"base_url"`
	}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return ""
	}
	return cfg.BaseURL
}

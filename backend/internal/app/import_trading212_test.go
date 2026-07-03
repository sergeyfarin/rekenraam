package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func goldenTrading212Payload(t *testing.T) []byte {
	t.Helper()
	payload := trading212FetchPayload{
		ConnectionID: 7,
		Movements: []trading212Movement{
			{ID: "ref-1", Type: "DEPOSIT", Timestamp: "2024-01-01T08:00:00Z", Amount: "1000.00", Currency: "eur", Notes: "Bank transfer"},
			{ID: "ref-2", Type: "TRANSFER", Timestamp: "2024-01-05T12:00:00Z", Amount: "4.32", Currency: "EUR", Notes: "Internal transfer"},
			{ID: "ref-3", Type: "FEE", Timestamp: "2024-01-06T09:00:00Z", Amount: "-1.00", Currency: "EUR", Notes: "Custody fee"},
			{ID: "ref-4", Type: "WITHDRAW", Timestamp: "2024-01-10T10:00:00Z", Amount: "-200.00", Currency: "EUR", Notes: "Bank transfer"},
			{ID: "ref-5", Type: "BUY", Timestamp: "2024-01-11T14:00:00Z", Amount: "-150.00", Currency: "EUR", Notes: "MSFT order fill"},
		},
	}
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	return b
}

func TestTrading212Adapter_Kind(t *testing.T) {
	assert.Equal(t, "trading212", (&Trading212Adapter{}).Kind())
}

func TestTrading212Adapter_DetectAlwaysNone(t *testing.T) {
	a := &Trading212Adapter{}
	assert.Equal(t, ConfidenceNone, a.Detect(RawInput{Filename: "anything.json"}))
	assert.Equal(t, ConfidenceNone, a.Detect(RawInput{}))
}

func TestTrading212Adapter_ParseGoldenPayload(t *testing.T) {
	a := &Trading212Adapter{}
	result, err := a.Parse(context.Background(), RawInput{Bytes: goldenTrading212Payload(t)}, nil)
	require.NoError(t, err)
	require.Len(t, result.Rows, 5)

	deposit := result.Rows[0]
	assert.Equal(t, "2024-01-01", deposit.Date)
	assert.Equal(t, "1000.00", deposit.Amount)
	assert.Equal(t, "EUR", deposit.CommodityHint)
	assert.Equal(t, "ref-1", deposit.ExternalRef)
	assert.Equal(t, "Bank transfer", deposit.PayeeHint)

	transfer := result.Rows[1]
	assert.Equal(t, "2024-01-05", transfer.Date)
	assert.Equal(t, "4.32", transfer.Amount)

	fee := result.Rows[2]
	assert.Equal(t, "-1.00", fee.Amount)

	withdrawal := result.Rows[3]
	assert.Equal(t, "-200.00", withdrawal.Amount)

	// BUY is not a cash movement type — staged but flagged needs_attention.
	buy := result.Rows[4]
	assert.Equal(t, "ref-5", buy.ExternalRef)
	require.Len(t, result.Warnings, 1)
	assert.Equal(t, 4, result.Warnings[0].RowIndex)
	assert.Contains(t, result.Warnings[0].Message, "BUY")

	assert.Equal(t, "2024-01-01", result.Meta.DateFrom)
	assert.Equal(t, "2024-01-11", result.Meta.DateTo)
	assert.Contains(t, result.Meta.CurrencyHints, "EUR")
}

// TestTrading212Adapter_CashMovementTypesMatchRealAPIEnum guards against
// regressing to the Slice 2 guessed enum (WITHDRAWAL, DIVIDEND, INTEREST,
// CARD_*, TRANSFER_IN/OUT, LENDING_INTEREST): the real
// /equity/history/transactions endpoint only ever emits WITHDRAW, DEPOSIT,
// FEE, TRANSFER (verified against the published OpenAPI spec). Anything else
// — including the old guessed values — must be flagged needs_attention, not
// silently treated as a plain cash leg.
func TestTrading212Adapter_CashMovementTypesMatchRealAPIEnum(t *testing.T) {
	a := &Trading212Adapter{}
	payload, err := json.Marshal(trading212FetchPayload{
		ConnectionID: 1,
		Movements: []trading212Movement{
			{ID: "ref-1", Type: "WITHDRAW", Timestamp: "2024-01-01T00:00:00Z", Amount: "-1.00", Currency: "EUR"},
			{ID: "ref-2", Type: "DEPOSIT", Timestamp: "2024-01-01T00:00:00Z", Amount: "1.00", Currency: "EUR"},
			{ID: "ref-3", Type: "FEE", Timestamp: "2024-01-01T00:00:00Z", Amount: "-1.00", Currency: "EUR"},
			{ID: "ref-4", Type: "TRANSFER", Timestamp: "2024-01-01T00:00:00Z", Amount: "1.00", Currency: "EUR"},
			{ID: "ref-5", Type: "WITHDRAWAL", Timestamp: "2024-01-01T00:00:00Z", Amount: "-1.00", Currency: "EUR"},
			{ID: "ref-6", Type: "DIVIDEND", Timestamp: "2024-01-01T00:00:00Z", Amount: "1.00", Currency: "EUR"},
		},
	})
	require.NoError(t, err)

	result, err := a.Parse(context.Background(), RawInput{Bytes: payload}, nil)
	require.NoError(t, err)
	require.Len(t, result.Rows, 6)

	warningRows := make(map[int]bool)
	for _, w := range result.Warnings {
		warningRows[w.RowIndex] = true
	}
	for i := 0; i < 4; i++ {
		assert.False(t, warningRows[i], "row %d (real cash-movement type) must not be flagged", i)
	}
	for i := 4; i < 6; i++ {
		assert.True(t, warningRows[i], "row %d (not a real /equity/history/transactions type) must be flagged needs_attention", i)
	}
}

func TestTrading212Adapter_ParseRejectsMissingConnectionID(t *testing.T) {
	a := &Trading212Adapter{}
	payload := trading212FetchPayload{Movements: []trading212Movement{{ID: "ref-1", Type: "DEPOSIT", Amount: "1.00", Currency: "EUR"}}}
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = a.Parse(context.Background(), RawInput{Bytes: b}, nil)
	assert.Error(t, err)
}

func TestTrading212Adapter_ParseRejectsInvalidJSON(t *testing.T) {
	a := &Trading212Adapter{}
	_, err := a.Parse(context.Background(), RawInput{Bytes: []byte("not json")}, nil)
	assert.Error(t, err)
}

func TestTrading212Adapter_FingerprintsStableAcrossReparse(t *testing.T) {
	a := &Trading212Adapter{}
	payload := goldenTrading212Payload(t)

	result1, err := a.Parse(context.Background(), RawInput{Bytes: payload}, nil)
	require.NoError(t, err)
	result2, err := a.Parse(context.Background(), RawInput{Bytes: payload}, nil)
	require.NoError(t, err)

	require.Len(t, result1.Rows, len(result2.Rows))
	for i := range result1.Rows {
		assert.Equal(t, result1.Rows[i].DedupeFingerprint, result2.Rows[i].DedupeFingerprint, "row %d fingerprint must be stable across re-parse", i)
	}
}

func TestTrading212Adapter_FingerprintsAreConnectionScoped(t *testing.T) {
	a := &Trading212Adapter{}
	mk := func(connectionID int64) []byte {
		b, err := json.Marshal(trading212FetchPayload{
			ConnectionID: connectionID,
			Movements:    []trading212Movement{{ID: "ref-1", Type: "DEPOSIT", Timestamp: "2024-01-01T00:00:00Z", Amount: "1.00", Currency: "EUR"}},
		})
		require.NoError(t, err)
		return b
	}

	resultA, err := a.Parse(context.Background(), RawInput{Bytes: mk(1)}, nil)
	require.NoError(t, err)
	resultB, err := a.Parse(context.Background(), RawInput{Bytes: mk(2)}, nil)
	require.NoError(t, err)

	assert.NotEqual(t, resultA.Rows[0].DedupeFingerprint, resultB.Rows[0].DedupeFingerprint)
}

// --- Trading212Prober ---

func TestTrading212Prober_SkipsNonTrading212SourceKind(t *testing.T) {
	prober := NewTrading212Prober(nil)
	err := prober.Probe(context.Background(), "qif", "anything", "")
	assert.NoError(t, err)
}

func TestTrading212Prober_ValidKeySucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"nextPagePath":""}`))
	}))
	defer server.Close()

	prober := NewTrading212Prober(server.Client())
	configJSON, err := json.Marshal(map[string]string{"base_url": server.URL})
	require.NoError(t, err)
	err = prober.Probe(context.Background(), "trading212", "good-key", string(configJSON))
	assert.NoError(t, err)
}

func TestTrading212Prober_InvalidKeyReturnsProviderUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	prober := NewTrading212Prober(server.Client())
	configJSON, err := json.Marshal(map[string]string{"base_url": server.URL})
	require.NoError(t, err)
	err = prober.Probe(context.Background(), "trading212", "bad-key", string(configJSON))
	assert.True(t, errors.Is(err, ErrProviderUnauthorized))
}

func TestTrading212BaseURLFromConfig_MalformedOrEmptyFallsBackToDefault(t *testing.T) {
	assert.Equal(t, "", trading212BaseURLFromConfig(""))
	assert.Equal(t, "", trading212BaseURLFromConfig("{not json"))
	assert.Equal(t, "", trading212BaseURLFromConfig("{}"))
}

func TestTrading212BaseURLFromConfig_ReadsOverride(t *testing.T) {
	assert.Equal(t, "https://demo.trading212.com/api/v0", trading212BaseURLFromConfig(`{"base_url":"https://demo.trading212.com/api/v0"}`))
}

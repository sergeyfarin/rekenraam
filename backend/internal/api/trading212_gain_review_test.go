package api

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

// T-128: API evidence for the two gain-review families that had only service
// tests — imported acquisitions and Trading 212 source revisions. Rows come
// from a provider stub through the real fetch worker, so each test drives
// the same path as a user: connection, online import, preview, commit.

// trading212StubFill is one executed order fill the stub serves.
type trading212StubFill struct {
	ID       int64
	Side     string
	Quantity string
	Price    string
	NetValue string
	FilledAt string
}

// trading212Stub serves the Trading 212 endpoints the fetcher calls. Fills
// are keyed by API key, so a test can revise a fill (same id, new values)
// between imports exactly as the broker would.
type trading212Stub struct {
	mu       sync.Mutex
	ticker   string
	isin     string
	currency string
	fills    map[int64]trading212StubFill
}

func newTrading212Stub(t *testing.T, ticker string, isin string, currency string) (*trading212Stub, string) {
	t.Helper()
	stub := &trading212Stub{ticker: ticker, isin: isin, currency: currency, fills: map[int64]trading212StubFill{}}
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)
	return stub, server.URL
}

func (s *trading212Stub) setFill(fill trading212StubFill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fills[fill.ID] = fill
}

func (s *trading212Stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != trading212StubAPIKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	items := []map[string]any{}
	if r.URL.Path == "/equity/history/orders" {
		s.mu.Lock()
		fills := make([]trading212StubFill, 0, len(s.fills))
		for _, fill := range s.fills {
			fills = append(fills, fill)
		}
		s.mu.Unlock()
		// Newest first, as the provider pages.
		slices.SortFunc(fills, func(a, b trading212StubFill) int { return strings.Compare(b.FilledAt, a.FilledAt) })
		for _, fill := range fills {
			items = append(items, map[string]any{
				"order": map[string]any{
					"status": "FILLED", "id": fill.ID + 1000, "ticker": s.ticker, "side": fill.Side, "currency": s.currency,
					"instrument": map[string]any{"ticker": s.ticker, "isin": s.isin, "name": "Stub " + s.ticker, "currency": s.currency},
				},
				"fill": map[string]any{
					"type": "TRADE", "id": fill.ID, "filledAt": fill.FilledAt,
					"price": json.Number(fill.Price), "quantity": json.Number(fill.Quantity),
					"walletImpact": map[string]any{"currency": s.currency, "netValue": json.Number(fill.NetValue)},
				},
			})
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "nextPagePath": nil})
}

const trading212StubAPIKey = "stub-trading212-key"

type trading212APIFixture struct {
	handler http.Handler
	investmentAPITestFixture
	connectionID int64
}

func newTrading212APIFixture(t *testing.T, stubURL string) trading212APIFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var workerDone <-chan struct{}
	handler, _ := newImportConnectionsTestHandlerWith(t, app.NewTrading212Prober(nil, stubURL), func(importService *app.ImportService) {
		importService.SetTrading212BaseURL(stubURL)
		workerDone = importService.StartBackgroundWorker(ctx, nil)
	})
	t.Cleanup(func() {
		cancel()
		<-workerDone
	})
	f := bootstrapInvestmentAPITest(t, handler)
	conn := createImportConnectionForSession(t, handler, f.sessionCookie, f.csrfToken, `{
		"source": "trading212", "display_name": "Stub ISA", "api_key": "`+trading212StubAPIKey+`",
		"cash_account_id": `+strconv.FormatInt(f.cashAccount.ID, 10)+`}`, http.StatusCreated)
	return trading212APIFixture{handler: handler, investmentAPITestFixture: f, connectionID: conn.ID}
}

// importFromStub runs a full online import and waits for the worker to stage
// it, returning the batch and its rows keyed by provider order id (the stub
// numbers each order 1000 above its fill).
func (f trading212APIFixture) importFromStub(t *testing.T) (int64, map[string]importStagedRowResponse) {
	t.Helper()
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/imports",
		startOnlineImportRequest{Source: "trading212", ConnectionID: f.connectionID}, http.StatusAccepted)
	var started startOnlineImportResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&started))
	batchPath := "/api/v1/imports/" + strconv.FormatInt(started.Batch.ID, 10)

	var batch getImportBatchResponse
	require.Eventually(t, func() bool {
		res := doInvestmentRequest(t, f.handler, f.sessionCookie, "", http.MethodGet, batchPath, nil, http.StatusOK)
		batch = getImportBatchResponse{}
		require.NoError(t, json.NewDecoder(res.Body).Decode(&batch))
		var meta struct {
			FetchStatus string `json:"fetch_status"`
		}
		require.NoError(t, json.Unmarshal([]byte(batch.Batch.SourceMetaJSON), &meta))
		require.NotEqual(t, "failed", meta.FetchStatus, batch.Batch.SourceMetaJSON)
		return meta.FetchStatus == "ready"
	}, 10*time.Second, 20*time.Millisecond, "the started fetch wakes the worker")
	require.Nil(t, batch.NextCursor)

	rows := map[string]importStagedRowResponse{}
	for _, row := range batch.Rows {
		var raw map[string]string
		require.NoError(t, json.Unmarshal([]byte(row.RawJSON), &raw))
		rows[raw["order_id"]] = row
	}
	return started.Batch.ID, rows
}

func (f trading212APIFixture) stagedRow(t *testing.T, batchID int64, rowID int64) importStagedRowResponse {
	t.Helper()
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, "", http.MethodGet, "/api/v1/imports/"+strconv.FormatInt(batchID, 10), nil, http.StatusOK)
	var batch getImportBatchResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&batch))
	for _, row := range batch.Rows {
		if row.ID == rowID {
			return row
		}
	}
	require.FailNow(t, "staged row not found")
	return importStagedRowResponse{}
}

func (f trading212APIFixture) previewCommit(t *testing.T, batchID int64) previewCommitResponse {
	t.Helper()
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/imports/"+strconv.FormatInt(batchID, 10)+"/preview-commit", nil, http.StatusOK)
	var preview previewCommitResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&preview))
	return preview
}

func (f trading212APIFixture) commit(t *testing.T, batchID int64, acknowledgements []rowGainImpactAcknowledgementRequest) commitImportBatchResponse {
	t.Helper()
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/imports/"+strconv.FormatInt(batchID, 10)+"/commit",
		commitImportBatchRequest{GainImpactAcknowledgements: acknowledgements}, http.StatusOK)
	var result commitImportBatchResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	return result
}

// realizedGains returns each realized gain for the account's sales as an
// exact decimal string, oldest disposal first.
func (f trading212APIFixture) realizedGains(t *testing.T) []string {
	t.Helper()
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, "", http.MethodGet, "/api/v1/investments/gains", nil, http.StatusOK)
	var gains investmentGainsResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &gains))
	out := make([]string, 0, len(gains.Realized))
	for _, gain := range gains.Realized {
		value := new(big.Rat).SetFrac(big.NewInt(int64(gain.RealizedGainValue)), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(gain.RealizedGainScale)), nil))
		out = append(out, gain.DisposalDate+" "+value.FloatString(2))
	}
	slices.Sort(out)
	return out
}

func (f trading212APIFixture) expectConflict(t *testing.T, path string, body any, code string) {
	t.Helper()
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, body, http.StatusConflict)
	var envelope errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&envelope))
	assert.Equal(t, code, envelope.Error.Code)
}

// seedBuyAndSale imports and commits a 10-share buy at 200.00 and a 5-share
// sale for 150.00: a 50.00 gain on the FIFO basis of 100.00.
func (f trading212APIFixture) seedBuyAndSale(t *testing.T, stub *trading212Stub) {
	t.Helper()
	stub.setFill(trading212StubFill{ID: 201, Side: "BUY", Quantity: "10", Price: "20.00", NetValue: "-200.00", FilledAt: "2026-02-01T10:00:00Z"})
	stub.setFill(trading212StubFill{ID: 301, Side: "SELL", Quantity: "5", Price: "30.00", NetValue: "150.00", FilledAt: "2026-03-01T10:00:00Z"})
	batchID, _ := f.importFromStub(t)
	require.Empty(t, f.previewCommit(t, batchID).GainImpacts, "a chronological first import changes no committed gain")
	result := f.commit(t, batchID, nil)
	require.Equal(t, "committed", result.Status)
	require.Equal(t, 2, result.CommittedCount)
	require.Equal(t, []string{"2026-03-01 50.00"}, f.realizedGains(t))
}

func TestTrading212ImportHoldsBackdatedBuyForGainReviewAPI(t *testing.T) {
	t.Parallel()
	stub, stubURL := newTrading212Stub(t, "GAINR_US_EQ", "US0000000128", "USD")
	f := newTrading212APIFixture(t, stubURL)
	f.seedBuyAndSale(t, stub)

	// A backdated cheaper buy arrives later: FIFO now sells it first.
	stub.setFill(trading212StubFill{ID: 101, Side: "BUY", Quantity: "10", Price: "2.00", NetValue: "-20.00", FilledAt: "2026-01-01T10:00:00Z"})
	batchID, rows := f.importFromStub(t)
	backdated := rows["1101"]
	require.Equal(t, "new", backdated.DedupeStatus)

	preview := f.previewCommit(t, batchID)
	require.Len(t, preview.GainImpacts, 1)
	impact := preview.GainImpacts[0]
	assert.Equal(t, backdated.ID, impact.RowID)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	assert.Equal(t, "revised", change.ChangeKind)
	assert.Equal(t, "2026-03-01", change.Before.DisposalDate)
	require.NotNil(t, change.After)
	assertGainCoefficient(t, 5000, 2, change.Before.RealizedGainValue, change.Before.RealizedGainScale)
	assertGainCoefficient(t, 14000, 2, change.After.RealizedGainValue, change.After.RealizedGainScale)
	require.NotEmpty(t, impact.GainImpact.Acknowledgement)

	// Without an acknowledgement the row is held pending, never skipped.
	// The re-fetched February buy and March sale are already in the ledger
	// and are skipped as duplicates.
	held := f.commit(t, batchID, nil)
	assert.Equal(t, "partially_committed", held.Status)
	assert.Equal(t, []int64{backdated.ID}, held.GainReviewRowIDs)
	assert.Zero(t, held.CommittedCount)
	assert.Equal(t, 2, held.SkippedCount)
	assert.Zero(t, held.FailedCount)
	assert.Equal(t, "pending", f.stagedRow(t, batchID, backdated.ID).CommitStatus, "a held row is never skipped")
	assert.Equal(t, []string{"2026-03-01 50.00"}, f.realizedGains(t), "a held row changes no gain")

	// A token from another change set holds it again.
	stale := f.commit(t, batchID, []rowGainImpactAcknowledgementRequest{{RowID: backdated.ID, Acknowledgement: "stale"}})
	assert.Equal(t, "partially_committed", stale.Status)
	assert.Equal(t, []int64{backdated.ID}, stale.GainReviewRowIDs)

	// The partially committed batch is previewed again and commits with the
	// per-row token.
	retry := f.previewCommit(t, batchID)
	require.Len(t, retry.GainImpacts, 1)
	assert.Equal(t, impact.GainImpact.Acknowledgement, retry.GainImpacts[0].GainImpact.Acknowledgement,
		"nothing changed in between, so the change set and its token are the same")
	done := f.commit(t, batchID, []rowGainImpactAcknowledgementRequest{{RowID: backdated.ID, Acknowledgement: retry.GainImpacts[0].GainImpact.Acknowledgement}})
	assert.Equal(t, 1, done.CommittedCount)
	assert.Equal(t, 2, done.SkippedCount)
	assert.Empty(t, done.GainReviewRowIDs)
	assert.Equal(t, "committed", f.stagedRow(t, batchID, backdated.ID).CommitStatus)
	assert.Equal(t, []string{"2026-03-01 140.00"}, f.realizedGains(t))
}

func TestTrading212BuySourceRevisionRequiresGainAcknowledgementAPI(t *testing.T) {
	t.Parallel()
	stub, stubURL := newTrading212Stub(t, "REVB_US_EQ", "US0000001128", "USD")
	f := newTrading212APIFixture(t, stubURL)
	f.seedBuyAndSale(t, stub)

	// The broker revises the buy's net value: the sale's basis halves.
	stub.setFill(trading212StubFill{ID: 201, Side: "BUY", Quantity: "10", Price: "10.00", NetValue: "-100.00", FilledAt: "2026-02-01T10:00:00Z"})
	batchID, rows := f.importFromStub(t)
	revised := rows["1201"]
	require.True(t, revised.SourceChanged)
	require.True(t, revised.SourceBuyOperation)

	path := "/api/v1/imports/" + strconv.FormatInt(batchID, 10) + "/rows/" + strconv.FormatInt(revised.ID, 10) + "/correct-buy"
	request := correctTrading212SourceRequest{Reason: "broker revised net value"}
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	assert.Equal(t, "revised", change.ChangeKind)
	require.NotNil(t, change.After)
	assertGainCoefficient(t, 10000, 2, change.Before.DisposedBasisValue, change.Before.DisposedBasisScale)
	assertGainCoefficient(t, 5000, 2, change.After.DisposedBasisValue, change.After.DisposedBasisScale)

	f.expectConflict(t, path, request, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	request.GainImpactAcknowledgement = "stale"
	f.expectConflict(t, path, request, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE")
	assert.Equal(t, []string{"2026-03-01 50.00"}, f.realizedGains(t), "refused corrections change nothing")

	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	assert.Equal(t, []string{"2026-03-01 100.00"}, f.realizedGains(t))
}

func TestTrading212SaleSourceRevisionRequiresGainAcknowledgementAPI(t *testing.T) {
	t.Parallel()
	stub, stubURL := newTrading212Stub(t, "REVS_US_EQ", "US0000002128", "USD")
	f := newTrading212APIFixture(t, stubURL)
	f.seedBuyAndSale(t, stub)

	// The broker revises the sale's proceeds: its own gain is replaced.
	stub.setFill(trading212StubFill{ID: 301, Side: "SELL", Quantity: "5", Price: "50.00", NetValue: "250.00", FilledAt: "2026-03-01T10:00:00Z"})
	batchID, rows := f.importFromStub(t)
	revised := rows["1301"]
	require.True(t, revised.SourceChanged)
	require.True(t, revised.SourceSaleOperation)

	path := "/api/v1/imports/" + strconv.FormatInt(batchID, 10) + "/rows/" + strconv.FormatInt(revised.ID, 10) + "/correct-sale"
	request := correctTrading212SourceRequest{Reason: "broker revised proceeds"}
	res := doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost, path+"/reconciliation-impact", request, http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&impact))
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	change := impact.GainImpact.Changes[0]
	assert.Equal(t, "replaced", change.ChangeKind)
	require.NotNil(t, change.After)
	assertGainCoefficient(t, 5000, 2, change.Before.RealizedGainValue, change.Before.RealizedGainScale)
	assertGainCoefficient(t, 15000, 2, change.After.RealizedGainValue, change.After.RealizedGainScale)

	f.expectConflict(t, path, request, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	request.GainImpactAcknowledgement = "stale"
	f.expectConflict(t, path, request, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE")
	assert.Equal(t, []string{"2026-03-01 50.00"}, f.realizedGains(t), "refused corrections change nothing")

	request.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	doInvestmentRequest(t, f.handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	assert.Equal(t, []string{"2026-03-01 150.00"}, f.realizedGains(t))
}

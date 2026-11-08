package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #304: an investment or import write's reconciliation override is bound to
// the checkpoint set the owner reviewed, as a generic ledger write's is (#296).

func TestInvestmentOverrideReviewedAgainstOneCheckpointIsRefusedOnceTheSetGrows(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "BIND304")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	opening := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000)
	opening.TransactionDate = "2026-01-01"
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", opening, http.StatusCreated).Body).Decode(&bought))
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID,
		postingByAccount(t, bought.Transaction, holding.ID), "2026-02-01")
	holdingCheckpoint := activeCheckpointID(t, handler, f.sessionCookie, holding.ID)

	// The owner previews a backdated buy: it reaches the holding statement only.
	backdated := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 50000)
	backdated.TransactionDate = "2026-01-15"
	var impact reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/buy/reconciliation-impact", backdated, http.StatusOK).Body).Decode(&impact))
	require.Equal(t, []int64{holdingCheckpoint}, impactIDs(impact))

	// Another tab then records a February buy and finishes the March
	// statement (12 units), which the backdated buy would change too.
	february := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 20000)
	february.TransactionDate = "2026-02-10"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", february, http.StatusCreated)
	started := startReconciliationForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, `{
		"commodity_id":`+strconvFormatInt(instrument.CommodityID)+`,"source_kind":"statement","statement_date":"2026-03-01",
		"statement_balance_value":"12","statement_balance_scale":0}`, http.StatusCreated)
	candidates := make([]int64, 0, len(started.Candidates))
	for _, candidate := range started.Candidates {
		candidates = append(candidates, candidate.PostingID)
	}
	previewReconciliationForSession(t, handler, f.sessionCookie, f.csrfToken, started.ID,
		`{"posting_version_ids":`+idList(candidates...)+`}`, http.StatusOK)
	finishReconciliationForSession(t, handler, f.sessionCookie, f.csrfToken, started.ID, `{"change_reason":"March statement"}`, http.StatusOK)
	var laterCheckpoint int64
	for _, checkpoint := range listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID) {
		if checkpoint.SessionID != nil && *checkpoint.SessionID == started.ID {
			laterCheckpoint = checkpoint.ID
		}
	}
	require.NotZero(t, laterCheckpoint)

	// An override without its reviewed set is not accepted at all.
	unbound := backdated
	unbound.ReconciliationOverride = true
	refusedUnbound := sendInvestmentJSON(t, handler, f, "/api/v1/investments/buy", unbound,
		`,"reconciliation_override_checkpoint_ids":null`, "")
	require.Equal(t, http.StatusBadRequest, refusedUnbound.Code, refusedUnbound.Body.String())

	// The override acknowledged one checkpoint; the write would now invalidate
	// two. It is refused, names the current set, and invalidates nothing.
	stale := sendInvestmentJSON(t, handler, f, "/api/v1/investments/buy", unbound,
		`"reconciliation_override_checkpoint_ids":null`, `"reconciliation_override_checkpoint_ids":`+idList(holdingCheckpoint))
	require.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
	var refusal errorResponse
	require.NoError(t, json.NewDecoder(stale.Body).Decode(&refusal))
	assert.Equal(t, "RECONCILIATION_IMPACT_STALE", refusal.Error.Code)
	assert.ElementsMatch(t, []int64{holdingCheckpoint, laterCheckpoint}, refusal.Error.CheckpointIDs)
	for _, checkpoint := range listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID) {
		assert.Equal(t, "active", checkpoint.Status, "nothing was invalidated: checkpoint %d", checkpoint.ID)
	}

	// Reviewed against the current set, the same override proceeds.
	bound := sendInvestmentJSON(t, handler, f, "/api/v1/investments/buy", unbound,
		`"reconciliation_override_checkpoint_ids":null`, `"reconciliation_override_checkpoint_ids":`+idList(laterCheckpoint, holdingCheckpoint))
	require.Equal(t, http.StatusCreated, bound.Code, bound.Body.String())
	for _, checkpoint := range listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID) {
		assert.Equal(t, "invalidated", checkpoint.Status, "checkpoint %d", checkpoint.ID)
	}
}

// Every browser investment and import write that can override the checkpoint
// guard must name the reviewed set; a request type that forgot the shared
// fields would accept a bare override and stay unbound. The batch commit is
// the one deliberate exception until it has a preview (#305).
func TestEveryInvestmentAndImportOverrideMustNameItsReviewedCheckpoints(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	paths := []string{
		"/api/v1/investments/buy", "/api/v1/investments/sell", "/api/v1/investments/short-sale", "/api/v1/investments/short-cover",
		"/api/v1/investments/transfers/external/in", "/api/v1/investments/transfers/external/out", "/api/v1/investments/transfers/internal",
		"/api/v1/investments/cash-in-lieu", "/api/v1/investments/return-of-capital", "/api/v1/investments/splits",
		"/api/v1/investments/share-exchanges", "/api/v1/investments/cash-and-stock-mergers", "/api/v1/investments/spin-offs",
		"/api/v1/investments/write-off", "/api/v1/investments/dividend", "/api/v1/investments/payment-in-lieu",
		"/api/v1/investments/reinvested-dividend",
		"/api/v1/imports/1/rows/1/review-dividend", "/api/v1/imports/1/rows/1/correct-buy", "/api/v1/imports/1/rows/1/correct-sale",
		"/api/v1/investments/operations/1/reverse-split", "/api/v1/investments/operations/1/replace-split",
	}
	for _, action := range []string{
		"replace-cash-in-lieu", "reverse-cash-in-lieu", "reverse-spin-off", "replace-spin-off",
		"reverse-cash-and-stock-merger", "replace-cash-and-stock-merger", "reverse-share-exchange", "replace-share-exchange",
		"reverse-transfer", "resolve-basis", "replace-basis-resolution", "reverse-basis-resolution", "replace-transfer",
		"replace-transfer-out", "replace-transfer-in", "reverse-split", "replace-split", "reverse-sale", "reverse-buy",
		"replace-sale", "replace-buy", "reverse-short-sale", "replace-short-sale", "reverse-short-cover", "replace-short-cover",
		"reverse-write-off", "replace-write-off", "replace-return-of-capital", "reverse-return-of-capital", "reverse-dividend",
		"replace-dividend", "reverse-payment-in-lieu", "replace-payment-in-lieu", "reverse-reinvested-dividend",
		"replace-reinvested-dividend",
	} {
		paths = append(paths, "/api/v1/investments/transactions/1/"+action)
	}
	bodies := map[string]string{}
	for _, path := range paths {
		bodies[path] = `{"reconciliation_override":true}`
	}
	// A provider proposal's override travels inside its completion.
	bodies["/api/v1/investments/event-suggestions/1/accept"] = `{"completion":{"reconciliation_override":true}}`
	for path, body := range bodies {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfTokenHeader, f.csrfToken)
		setSameOrigin(req)
		req.AddCookie(f.sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, http.StatusBadRequest, res.Code, "%s: %s", path, res.Body.String())
		assert.Contains(t, res.Body.String(), "reconciliation_override_checkpoint_ids", path)
	}
}

// sendInvestmentJSON posts body with one literal substitution applied to its
// encoding and no automatic override binding.
func sendInvestmentJSON(t *testing.T, handler http.Handler, f investmentAPITestFixture, path string, body any, old, replacement string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	raw := strings.Replace(string(encoded), old, replacement, 1)
	if replacement == "" {
		require.NotEqual(t, string(encoded), raw, "the substitution must apply")
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, f.csrfToken)
	setSameOrigin(req)
	req.AddCookie(f.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

// A correction appends an inverse and a replacement under one command; both
// journals carry the one reviewed set into the combined guard.
func TestInvestmentCorrectionOverrideIsBoundAcrossItsJournals(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "BIND304C")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	opening := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000)
	opening.TransactionDate = "2026-01-01"
	var bought investmentTradeResponse
	require.NoError(t, json.NewDecoder(doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/investments/buy", opening, http.StatusCreated).Body).Decode(&bought))
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, holding.ID, instrument.CommodityID,
		postingByAccount(t, bought.Transaction, holding.ID), "2026-02-01")
	checkpoint := activeCheckpointID(t, handler, f.sessionCookie, holding.ID)

	replacement := tradeRequestBody(f, holding.ID, instrument.CommodityID, "8", 80000)
	replacement.TransactionDate = "2026-01-01"
	request := investmentBuyReplacementRequest{Reason: "broker corrected the fill", Replacement: replacement,
		ReconciliationOverride: true}
	path := "/api/v1/investments/transactions/" + strconvFormatInt(bought.Transaction.ID) + "/replace-buy"

	// Reviewed against no checkpoint, the correction may not invalidate one.
	request.ReconciliationOverrideCheckpointIDs = []int64{}
	stale := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusConflict)
	assert.Contains(t, stale.Body.String(), "RECONCILIATION_IMPACT_STALE")
	assert.Equal(t, checkpoint, activeCheckpointID(t, handler, f.sessionCookie, holding.ID), "nothing was invalidated")

	request.ReconciliationOverrideCheckpointIDs = []int64{checkpoint}
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, path, request, http.StatusCreated)
	checkpoints := listCheckpointsForSession(t, handler, f.sessionCookie, holding.ID)
	require.Len(t, checkpoints, 1)
	assert.Equal(t, "invalidated", checkpoints[0].Status)
}

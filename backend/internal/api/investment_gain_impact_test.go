package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestBuyGainImpactAcknowledgementContract(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "GAIN")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy",
		tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 20000), http.StatusCreated)
	sale := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 15000)
	sale.TransactionDate = "2026-03-01"
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/sell", sale, http.StatusCreated)

	backdated := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 2000)
	backdated.TransactionDate = "2026-01-01"
	previewRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/buy/reconciliation-impact", backdated, http.StatusOK)
	var preview reconciliationImpactResponse
	require.NoError(t, json.NewDecoder(previewRes.Body).Decode(&preview))
	require.NotNil(t, preview.GainImpact)
	require.Len(t, preview.GainImpact.Changes, 1)
	change := preview.GainImpact.Changes[0]
	assert.Equal(t, "revised", change.ChangeKind)
	assert.Equal(t, "2026-03-01", change.Before.DisposalDate)
	assert.Equal(t, "known", change.Before.BasisKnowledge)
	require.NotNil(t, change.After)
	assertGainCoefficient(t, 5000, 2, change.Before.RealizedGainValue, change.Before.RealizedGainScale)
	assertGainCoefficient(t, 14000, 2, change.After.RealizedGainValue, change.After.RealizedGainScale)

	expectCode := func(body investmentTradeRequest, code string) {
		t.Helper()
		res := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", body, http.StatusConflict)
		var envelope errorResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&envelope))
		assert.Equal(t, code, envelope.Error.Code)
	}
	expectCode(backdated, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED")
	backdated.GainImpactAcknowledgement = "stale"
	expectCode(backdated, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE")
	backdated.GainImpactAcknowledgement = preview.GainImpact.Acknowledgement
	doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/investments/buy", backdated, http.StatusCreated)

	// A buy that changes no committed disposal reports an empty set.
	later := tradeRequestBody(f, holding.ID, instrument.CommodityID, "1", 100)
	later.TransactionDate = "2026-04-01"
	emptyRes := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodPost,
		"/api/v1/investments/buy/reconciliation-impact", later, http.StatusOK)
	var empty map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(emptyRes.Body).Decode(&empty))
	assert.JSONEq(t, `{"changes":[],"acknowledgement":""}`, string(empty["gain_impact"]))
}

func assertGainCoefficient(t *testing.T, want int64, wantScale int, value *exact.Coefficient, scale *int) {
	t.Helper()
	require.NotNil(t, value)
	require.NotNil(t, scale)
	assert.Zero(t, exact.ScaledIntFromCoefficient(*value, *scale).Cmp(exact.ScaledIntFromInt64(want, wantScale)))
}

// The import preview and commit always carry the gain-review arrays (T-126),
// so a client never mistakes an absent list for "nothing to review".
func TestImportCommitGainReviewWireShape(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID, checking, groceries := bootstrapImportAPITest(t, handler)
	started := startQIFImportForSession(t, handler, sessionCookie, csrfToken, "bank.qif",
		qifRow("06/01/26", "-42.50", "Grocery Store"), http.StatusCreated)
	patchImportBatchForSession(t, handler, sessionCookie, csrfToken, started.Batch.ID,
		resolutionPatchBody(t, checking.ID, commodityID, groceries.ID, started.Rows[0].ID), http.StatusNoContent)
	batchPath := "/api/v1/imports/" + strconv.FormatInt(started.Batch.ID, 10)

	previewReq := httptest.NewRequest(http.MethodPost, batchPath+"/preview-commit", nil)
	previewReq.AddCookie(sessionCookie)
	previewRes := httptest.NewRecorder()
	handler.ServeHTTP(previewRes, previewReq)
	require.Equal(t, http.StatusOK, previewRes.Code, previewRes.Body.String())
	var preview map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(previewRes.Body.Bytes(), &preview))
	assert.JSONEq(t, `[]`, string(preview["gain_impacts"]))

	commitReq := httptest.NewRequest(http.MethodPost, batchPath+"/commit",
		strings.NewReader(`{"gain_impact_acknowledgements":[{"row_id":1,"acknowledgement":"unused"}]}`))
	commitReq.Header.Set("Content-Type", "application/json")
	commitReq.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(commitReq)
	commitReq.AddCookie(sessionCookie)
	commitRes := httptest.NewRecorder()
	handler.ServeHTTP(commitRes, commitReq)
	require.Equal(t, http.StatusOK, commitRes.Code, commitRes.Body.String())
	var commit map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(commitRes.Body.Bytes(), &commit))
	assert.JSONEq(t, `[]`, string(commit["gain_review_row_ids"]))
	assert.JSONEq(t, `"committed"`, string(commit["status"]))
}

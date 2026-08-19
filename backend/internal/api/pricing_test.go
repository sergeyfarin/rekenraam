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

// --- Bootstrap ---

type pricingAPITestFixture struct {
	sessionCookie *http.Cookie
	csrfToken     string
	usdCommodity  int64
	eurCommodity  int64
}

func bootstrapPricingAPITest(t *testing.T, handler http.Handler) pricingAPITestFixture {
	t.Helper()
	sessionCookie, csrfToken, usdCommodity := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"EUR"}`)
	return pricingAPITestFixture{sessionCookie: sessionCookie, csrfToken: csrfToken, usdCommodity: usdCommodity, eurCommodity: eur.ID}
}

func doPricingRequest(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body any, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrfToken != "" {
		req.Header.Set(csrfTokenHeader, csrfToken)
		setSameOrigin(req)
	}
	if sessionCookie != nil {
		req.AddCookie(sessionCookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

// --- Price creation ---

func TestCreatePrice_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, QuoteType: "manual",
		PriceValue: 10800, PriceScale: 4, ValuationDate: "2026-06-12", IsManual: true,
		Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created priceObservationResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	assert.Equal(t, int64(10800), created.PriceValue)
	assert.True(t, created.IsManual)

	listRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/prices?base_commodity_id="+itoa(f.eurCommodity)+"&quote_commodity_id="+itoa(f.usdCommodity), nil, http.StatusOK)
	var listed priceObservationsResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Prices, 1)
}

func TestCreatePrice_ValidationFailureReturnsBadRequest(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, QuoteType: "manual",
		PriceValue: 0, PriceScale: 4, ValuationDate: "2026-06-12",
	}, http.StatusBadRequest)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

// --- Policy round-trip ---

func TestPricingPolicy_GetAndSave_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	got := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/policy", nil, http.StatusOK)
	var defaultPolicy pricingPolicyResponse
	require.NoError(t, json.NewDecoder(got.Body).Decode(&defaultPolicy))
	assert.True(t, defaultPolicy.RefreshEnabled, "the in-memory default policy has refresh enabled")

	saveRes := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/pricing/policy", pricingPolicyRequest{
		RefreshEnabled: true, RefreshHourUTC: 5, RefreshMinuteUTC: 30,
		TriangulationMaxHops: 1, RoundingMode: "half_up", WeekendPolicy: "skip",
	}, http.StatusOK)
	var saved pricingPolicyResponse
	require.NoError(t, json.NewDecoder(saveRes.Body).Decode(&saved))
	assert.Equal(t, 5, saved.RefreshHourUTC)
	assert.Equal(t, 30, saved.RefreshMinuteUTC)

	got2 := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/policy", nil, http.StatusOK)
	var refetched pricingPolicyResponse
	require.NoError(t, json.NewDecoder(got2.Body).Decode(&refetched))
	assert.Equal(t, saved.ID, refetched.ID)
	assert.Equal(t, 5, refetched.RefreshHourUTC)
}

func TestSavePricingPolicy_ValidationFailureReturnsBadRequest(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut, "/api/v1/pricing/policy", pricingPolicyRequest{
		RefreshHourUTC: 30, RoundingMode: "half_up", WeekendPolicy: "skip",
	}, http.StatusBadRequest)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

// --- Source assignments, refresh runs, source health, sources ---

func TestPricingSourceAssignments_CreateUpdateList_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	sourcesRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/sources", nil, http.StatusOK)
	var sources marketDataSourcesResponse
	require.NoError(t, json.NewDecoder(sourcesRes.Body).Decode(&sources))
	require.NotEmpty(t, sources.Sources)
	manualSourceID := sources.Sources[0].ID

	createRes := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/source-assignments", pricingSourceAssignmentRequest{
		CommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, SourceID: manualSourceID,
		Priority: 10, Status: "active", EffectiveFrom: "2026-01-01",
	}, http.StatusCreated)
	var created pricingSourceAssignmentResponse
	require.NoError(t, json.NewDecoder(createRes.Body).Decode(&created))

	updateRes := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/pricing/source-assignments/"+itoa(created.ID), pricingSourceAssignmentRequest{
		CommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, SourceID: manualSourceID,
		Priority: 20, Status: "active", EffectiveFrom: "2026-01-01",
	}, http.StatusOK)
	var updated pricingSourceAssignmentResponse
	require.NoError(t, json.NewDecoder(updateRes.Body).Decode(&updated))
	assert.Equal(t, 20, updated.Priority)

	listRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/source-assignments", nil, http.StatusOK)
	var listed pricingSourceAssignmentsResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&listed))
	require.Len(t, listed.Assignments, 1)
}

func TestPricingRefreshRunsAndSourceHealth_HTTP(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	runsRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/refresh/runs", nil, http.StatusOK)
	var runs pricingRefreshRunsResponse
	require.NoError(t, json.NewDecoder(runsRes.Body).Decode(&runs))
	assert.Empty(t, runs.Runs)

	healthRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/pricing/source-health", nil, http.StatusOK)
	var health pricingSourceHealthListResponse
	require.NoError(t, json.NewDecoder(healthRes.Body).Decode(&health))
	assert.Empty(t, health.Sources)
	assert.Empty(t, health.FailedBackgroundWork)
}

func TestRetryPricingBackgroundWork_UnknownWorkIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/background-work/999999/retry", nil, http.StatusNotFound)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "NOT_FOUND", body.Error.Code)
}

// --- Authentication / CSRF ---

func TestPricingEndpoints_RequireAuthentication(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"sources", http.MethodGet, "/api/v1/pricing/sources"},
		{"prices", http.MethodGet, "/api/v1/pricing/prices"},
		{"create price", http.MethodPost, "/api/v1/pricing/prices"},
		{"policy", http.MethodGet, "/api/v1/pricing/policy"},
		{"save policy", http.MethodPut, "/api/v1/pricing/policy"},
		{"source assignments", http.MethodGet, "/api/v1/pricing/source-assignments"},
		{"refresh runs", http.MethodGet, "/api/v1/pricing/refresh/runs"},
		{"source health", http.MethodGet, "/api/v1/pricing/source-health"},
		{"retry background work", http.MethodPost, "/api/v1/pricing/background-work/1/retry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusUnauthorized, res.Code, tc.name)
		})
	}
}

func TestPricingMutations_RequireCSRFToken(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"create price", http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, PriceValue: 100, PriceScale: 2, ValuationDate: "2026-06-12"}},
		{"save policy", http.MethodPut, "/api/v1/pricing/policy", pricingPolicyRequest{RoundingMode: "half_up", WeekendPolicy: "skip"}},
		{"create source assignment", http.MethodPost, "/api/v1/pricing/source-assignments", pricingSourceAssignmentRequest{CommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, SourceID: 1, Status: "active", EffectiveFrom: "2026-01-01"}},
		{"run refresh", http.MethodPost, "/api/v1/pricing/refresh/run", pricingRefreshRunRequest{}},
		{"retry background work", http.MethodPost, "/api/v1/pricing/background-work/1/retry", struct{}{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.body)
			require.NoError(t, err)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(string(encoded)))
			req.Header.Set("Content-Type", "application/json")
			setSameOrigin(req)
			req.AddCookie(f.sessionCookie)
			// Deliberately omit the CSRF token header.
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusForbidden, res.Code, tc.name)
		})
	}
}

// --- Error mapping ---

func TestPricingSourceAssignment_UnknownAssignmentIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPatch, "/api/v1/pricing/source-assignments/999999", pricingSourceAssignmentRequest{
		CommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, SourceID: 1, Status: "active", EffectiveFrom: "2026-01-01",
	}, http.StatusNotFound)
	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "NOT_FOUND", body.Error.Code)
}

func itoa(v int64) string {
	return strconvFormatInt(v)
}

// --- Voiding (T-37) ---

// TestVoidPrice_RetiresObservationWithoutDeletingIt covers the half of the
// "correct a price by superseding or voiding" invariant that had no
// implementation: every read filtered on voided_at, but nothing could set it.
func TestVoidPrice_RetiresObservationWithoutDeletingIt(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	createPrice := func(value int64, date string) priceObservationResponse {
		res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
			BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, QuoteType: "manual",
			PriceValue: value, PriceScale: 4, ValuationDate: date, IsManual: true,
			Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
		}, http.StatusCreated)
		var created priceObservationResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
		return created
	}

	poisoned := createPrice(999999, "2026-06-12")
	good := createPrice(10800, "2026-06-13")

	listPath := "/api/v1/pricing/prices?base_commodity_id=" + itoa(f.eurCommodity) + "&quote_commodity_id=" + itoa(f.usdCommodity)
	listRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, listPath, nil, http.StatusOK)
	var before priceObservationsResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&before))
	require.Len(t, before.Prices, 2)

	voidRes := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/pricing/prices/"+itoa(poisoned.ID)+"/void",
		voidPriceRequest{VoidReason: "provider published a bad rate"}, http.StatusOK)
	var voided priceObservationResponse
	require.NoError(t, json.NewDecoder(voidRes.Body).Decode(&voided))
	assert.Equal(t, poisoned.ID, voided.ID)
	assert.NotEmpty(t, voided.VoidedAt)
	assert.Equal(t, "provider published a bad rate", voided.VoidReason)
	// The observation is retired, not rewritten: its price is untouched.
	assert.Equal(t, int64(999999), voided.PriceValue)

	afterRes := doPricingRequest(t, handler, f.sessionCookie, "", http.MethodGet, listPath, nil, http.StatusOK)
	var after priceObservationsResponse
	require.NoError(t, json.NewDecoder(afterRes.Body).Decode(&after))
	require.Len(t, after.Prices, 1)
	assert.Equal(t, good.ID, after.Prices[0].ID)
}

func TestVoidPrice_RequiresAReasonAndRejectsRepeats(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)

	res := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/pricing/prices", priceObservationRequest{
		BaseCommodityID: f.eurCommodity, QuoteCommodityID: f.usdCommodity, QuoteType: "manual",
		PriceValue: 10800, PriceScale: 4, ValuationDate: "2026-06-12", IsManual: true,
		Derivation: json.RawMessage(`{}`), SeriesMetadata: json.RawMessage(`{}`), Metadata: json.RawMessage(`{}`),
	}, http.StatusCreated)
	var created priceObservationResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&created))
	voidPath := "/api/v1/pricing/prices/" + itoa(created.ID) + "/void"

	// A reason is required rather than defaulted: "why" is the whole value of
	// the record, and inventing it for the user would make the audit log lie.
	blank := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, voidPath,
		voidPriceRequest{VoidReason: "   "}, http.StatusBadRequest)
	var blankBody errorResponse
	require.NoError(t, json.NewDecoder(blank.Body).Decode(&blankBody))
	assert.Equal(t, "VALIDATION_FAILED", blankBody.Error.Code)

	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, voidPath,
		voidPriceRequest{VoidReason: "duplicate of a later fixing"}, http.StatusOK)

	// A second void is a conflict, not a silent success: two clicks disagreeing
	// about a price's state is worth surfacing.
	repeat := doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, voidPath,
		voidPriceRequest{VoidReason: "again"}, http.StatusConflict)
	var repeatBody errorResponse
	require.NoError(t, json.NewDecoder(repeat.Body).Decode(&repeatBody))
	assert.Equal(t, "CONFLICT", repeatBody.Error.Code)

	// Unknown observations are a 404, and the request still needs a CSRF token.
	doPricingRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/pricing/prices/999999/void", voidPriceRequest{VoidReason: "nope"}, http.StatusNotFound)
	doPricingRequest(t, handler, f.sessionCookie, "", http.MethodPost, voidPath,
		voidPriceRequest{VoidReason: "no csrf"}, http.StatusForbidden)
}

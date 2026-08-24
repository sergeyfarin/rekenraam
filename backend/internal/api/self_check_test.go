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

func readSelfCheckStatus(t *testing.T, handler http.Handler, sessionCookie *http.Cookie) selfCheckStatusResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/self-check", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var status selfCheckStatusResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &status))
	return status
}

// "Never checked" is an answer a screen can show. An empty results table would
// look like a pass.
func TestSelfCheckStatusSaysWhenNothingHasRunYet(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	status := readSelfCheckStatus(t, handler, f.sessionCookie)
	assert.False(t, status.HasRun)
	assert.Nil(t, status.LatestRun)
	assert.True(t, status.ReadOnly, "the screen needs to be able to say this check changes nothing")
}

// A run on a real book passes every check and explains each one.
func TestRunSelfCheckPassesOnALiveBookAndExplainsEachCheck(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	createTransactionForSession(t, handler, f.sessionCookie, f.csrfToken, balancedBody("2026-06-01",
		posting(f.checking.ID, 200000, 2, f.usdID),
		posting(f.salary.ID, -200000, 2, f.usdID),
	), http.StatusCreated)

	body := mutateBackup(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/maintenance/self-check", ``, http.StatusOK)

	var run selfCheckRunResponse
	require.NoError(t, json.Unmarshal(body, &run))
	assert.Equal(t, "passed", run.Status)
	assert.Zero(t, run.FailedCheckCount)
	assert.Equal(t, "manual", run.Trigger)
	require.NotEmpty(t, run.Results)

	byID := map[string]selfCheckResultResponse{}
	for _, result := range run.Results {
		byID[result.CheckID] = result
		assert.NotEmptyf(t, result.Explanation, "%s must explain what it checks", result.CheckID)
		assert.NotEmptyf(t, result.NextStep, "%s must say what to do about a failure", result.CheckID)
		assert.NotNil(t, result.Sample, "an absent sample is [] rather than null")
	}

	for _, expected := range []string{
		"entry_balance", "transaction_balance", "book_balance", "version_integrity",
		"lot_reconciliation", "checkpoint_integrity", "account_version_coverage",
		"sqlite_integrity", "attachments",
	} {
		assert.Containsf(t, byID, expected, "the run must include %s", expected)
	}
	assert.Equal(t, "not_applicable", byID["attachments"].Status,
		"a reserved slot reports not_applicable rather than passing a check nobody ran")

	// And the run is readable afterwards without re-running it.
	status := readSelfCheckStatus(t, handler, f.sessionCookie)
	require.True(t, status.HasRun)
	require.NotNil(t, status.LatestRun)
	assert.Equal(t, run.ID, status.LatestRun.ID)
	assert.Len(t, status.LatestRun.Results, len(run.Results))
}

func TestSelfCheckEndpointsRequireAuthenticationAndSameOrigin(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/self-check", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusUnauthorized, res.Code)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/self-check", strings.NewReader(``))
	setSameOrigin(req)
	req.AddCookie(f.sessionCookie)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusForbidden, res.Code, "a missing CSRF token is refused even with a session")
}

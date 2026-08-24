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

func readBackupStatus(t *testing.T, handler http.Handler, sessionCookie *http.Cookie) backupStatusResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/backups", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var status backupStatusResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &status))
	return status
}

// A book that has never configured a backup still has a schedule, because the
// promise is "backed up nightly", not "backed up once you find the setting".
func TestBackupStatusServesDefaultsAndTheKeyNotice(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	status := readBackupStatus(t, handler, f.sessionCookie)

	assert.True(t, status.Policy.Enabled)
	assert.Equal(t, 3, status.Policy.HourLocal)
	assert.Equal(t, 15, status.Policy.MinuteLocal)
	assert.Equal(t, 14, status.Policy.RetentionCount)
	assert.NotEmpty(t, status.Policy.TimeZone)
	assert.NotEmpty(t, status.Directory)
	assert.NotEmpty(t, status.NextRunAt, "a schedule nobody can see is not a schedule")
	assert.Empty(t, status.Runs)
	assert.Nil(t, status.LastSuccess)

	// The part of the protection story a database backup cannot carry, served
	// with the status rather than left to the docs.
	assert.False(t, status.KeyNotice.IncludedInBackup)
	assert.Equal(t, "REKENRAAM_SECRET_KEY", status.KeyNotice.EnvironmentVariable)
	assert.Contains(t, status.KeyNotice.Consequence, "unusable connection credentials")
}

func TestBackupPolicyUpdateKeepsOmittedFields(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	updated := mutateBackup(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut,
		"/api/v1/maintenance/backups/policy", `{"hour_local":22}`, http.StatusOK)

	var policy backupPolicyResponse
	require.NoError(t, json.Unmarshal(updated, &policy))
	assert.Equal(t, 22, policy.HourLocal)
	assert.Equal(t, 15, policy.MinuteLocal, "an omitted field keeps its value rather than being zeroed")
	assert.Equal(t, 14, policy.RetentionCount)
	assert.True(t, policy.Enabled)

	// And a rejected value says why.
	refused := mutateBackup(t, handler, f.sessionCookie, f.csrfToken, http.MethodPut,
		"/api/v1/maintenance/backups/policy", `{"retention_count":0}`, http.StatusBadRequest)
	assert.Contains(t, string(refused), "retention must keep at least one backup")
}

// Requesting a backup reports accepted work, never a completed copy.
func TestRequestBackupReportsAcceptedWorkNotACompletedBackup(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	body := mutateBackup(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/maintenance/backups", ``, http.StatusAccepted)

	var run backupRunResponse
	require.NoError(t, json.Unmarshal(body, &run))
	assert.Equal(t, "manual", run.Trigger)
	assert.Equal(t, "pending", run.Status, "202 means queued; claiming otherwise would be the claim ADR 0010 forbids")
	assert.False(t, run.Verified)
	assert.NotEmpty(t, run.TargetPath)

	status := readBackupStatus(t, handler, f.sessionCookie)
	require.Len(t, status.Runs, 1)
	assert.Equal(t, run.ID, status.Runs[0].ID)
	assert.Nil(t, status.LastSuccess, "nothing has succeeded yet")
}

func TestBackupEndpointsRequireAuthenticationAndSameOrigin(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/backups", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusUnauthorized, res.Code)

	// A mutation without the CSRF token is refused even with a session.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/backups", strings.NewReader(``))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	req.AddCookie(f.sessionCookie)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusForbidden, res.Code)
}

func TestRetryBackupRunRejectsAnUnknownRun(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	f := newBundleFixture(t, handler)

	body := mutateBackup(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost,
		"/api/v1/maintenance/backups/98765/retry", ``, http.StatusNotFound)
	assert.Contains(t, string(body), "NOT_FOUND")
}

func mutateBackup(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) []byte {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())
	return res.Body.Bytes()
}

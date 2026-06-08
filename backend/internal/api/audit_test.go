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

func TestAuditEventsFilterByTargetAndIncludeRowChanges(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Audited Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`,
		"quantity_scale_override":2,
		"allows_postings":true,
		"opened_on":"2026-01-01",
		"change_reason":"audit test"
	}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-events?target_type=account&target_id="+strconvFormatInt(account.ID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equalf(t, http.StatusOK, res.Code, "response body: %s", res.Body.String())

	var response auditEventsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	require.NotEmpty(t, response.Events)
	assert.Equal(t, "account.create", response.Events[0].Operation)
	require.NotEmpty(t, response.Events[0].Targets)
	assert.Equal(t, "account", response.Events[0].Targets[0].TargetType)
	assert.Equal(t, account.ID, response.Events[0].Targets[0].TargetID)
	require.NotEmpty(t, response.Events[0].Changes)
	foundAccountSnapshot := false
	for _, change := range response.Events[0].Changes {
		if change.TableName == "accounts" && strings.Contains(string(change.After), "Audited Checking") {
			foundAccountSnapshot = true
		}
	}
	assert.True(t, foundAccountSnapshot)
}

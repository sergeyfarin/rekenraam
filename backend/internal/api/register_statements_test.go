package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #595. The register always carries a statements array — empty, never null —
// so a client can place statement rows without a nil check.
func TestAccountRegisterAlwaysReturnsStatementsArray(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)

	for _, suffix := range []string{"", "?status=voided"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(checking.ID)+"/register"+suffix, nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code)

		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
		assert.JSONEq(t, `[]`, string(body["statements"]), suffix)
		assert.JSONEq(t, `[]`, string(body["entries"]), suffix)
	}
}

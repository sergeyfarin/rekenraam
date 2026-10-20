package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rekenraam/backend/internal/app"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestPersonalAccessTokenGrantsMatchOpenAPI(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..", "api", "openapi")
	raw, err := os.ReadFile(filepath.Join(root, "openapi.yaml"))
	require.NoError(t, err)
	var spec struct {
		Paths map[string]struct {
			Ref string `yaml:"$ref"`
		} `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	declared := map[string]string{}
	for path, ref := range spec.Paths {
		raw, err := os.ReadFile(filepath.Join(root, ref.Ref))
		require.NoError(t, err)
		var operation struct {
			Get struct {
				Scope     string                `yaml:"x-personal-access-token-scope"`
				Security  []map[string][]string `yaml:"security"`
				Responses map[string]any        `yaml:"responses"`
			} `yaml:"get"`
		}
		require.NoError(t, yaml.Unmarshal(raw, &operation))
		bearer := false
		for _, alternative := range operation.Get.Security {
			if _, ok := alternative["PersonalAccessToken"]; ok {
				bearer = true
			}
		}
		if operation.Get.Scope == "" {
			assert.False(t, bearer, "Bearer must have a declared grant: %s", path)
			continue
		}
		assert.True(t, bearer, "declared grant must admit Bearer: %s", path)
		assert.Contains(t, operation.Get.Responses, "401", "invalid credential refusal: %s", path)
		assert.Contains(t, operation.Get.Responses, "403", "scope refusal: %s", path)
		declared["GET "+path] = operation.Get.Scope
	}
	assert.Equal(t, personalAccessTokenRouteScopes, declared, "runtime grants and public API contract must match exactly")
}

func tokenAPIRequest(handler http.Handler, method, path, body string, cookie *http.Cookie, csrf, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrf != "" {
		req.Header.Set(csrfTokenHeader, csrf)
		setSameOrigin(req)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func createTokenAPI(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf string, scopes []string) (app.PersonalAccessToken, string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": "Synthetic API client", "password": "test-password", "scopes": scopes})
	require.NoError(t, err)
	response := tokenAPIRequest(handler, "POST", "/api/v1/auth/tokens", string(body), cookie, csrf, "")
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var result struct {
		Token      string                  `json:"token"`
		Credential app.PersonalAccessToken `json:"credential"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	return result.Credential, result.Token
}

func TestPersonalAccessTokenAPIReadScopesAndDefaultDeny(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, currency := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Token checking", "asset", "checking", currency, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Token expense","category_type":"expense"}`)
	transaction := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-06-07", posting(checking.ID, -2500, 2, currency), posting(category.ID, 2500, 2, currency)), http.StatusCreated)
	_, ledgerToken := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead})
	_, reportToken := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeReportsRead})
	_, bothToken := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead, app.TokenScopeReportsRead})

	// Compare actual consumers with the owner session for EVERY granted route.
	// Missing optional reference IDs can return 404, but may not be auth refusals.
	for pattern, scope := range personalAccessTokenRouteScopes {
		t.Run(pattern, func(t *testing.T) {
			path := strings.TrimPrefix(pattern, "GET ")
			for _, parameter := range []string{"institution_id", "account_id", "category_id", "tag_id", "payee_id", "transaction_id"} {
				path = strings.ReplaceAll(path, "{"+parameter+"}", "1")
			}
			owner := tokenAPIRequest(handler, "GET", path, "", cookie, "", "")
			require.NotEqual(t, http.StatusInternalServerError, owner.Code, owner.Body.String())
			allowed, denied := ledgerToken, reportToken
			if scope == app.TokenScopeReportsRead {
				allowed, denied = reportToken, ledgerToken
			}
			for _, token := range []string{allowed, bothToken} {
				got := tokenAPIRequest(handler, "GET", path, "", nil, "", "Bearer "+token)
				require.Equal(t, owner.Code, got.Code, got.Body.String())
				assert.JSONEq(t, owner.Body.String(), got.Body.String())
				assert.Empty(t, got.Result().Cookies(), "token authentication never creates cookies")
			}
			got := tokenAPIRequest(handler, "GET", path, "", cookie, csrf, "Bearer "+denied)
			require.Equal(t, http.StatusForbidden, got.Code, "a valid cookie cannot widen token scope")
		})
	}
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/session"}, {"GET", "/api/v1/auth/events"}, {"GET", "/api/v1/auth/mfa"},
		{"GET", "/api/v1/auth/tokens"}, {"POST", "/api/v1/auth/tokens"}, {"DELETE", "/api/v1/auth/tokens/1"},
		{"GET", "/api/v1/settings/preferences"}, {"GET", "/api/v1/setup/status"}, {"POST", "/api/v1/auth/login"},
		{"GET", "/api/v1/maintenance/backups"}, {"GET", "/api/v1/exports/bundle.zip"}, {"GET", "/api/v1/investments/positions"},
		{"POST", "/api/v1/transactions"}, {"PATCH", fmt.Sprintf("/api/v1/transactions/%d", transaction.ID)},
		{"POST", fmt.Sprintf("/api/v1/transactions/%d/void", transaction.ID)}, {"DELETE", fmt.Sprintf("/api/v1/transactions/%d", transaction.ID)},
		{"GET", "/api/v1/accounts/1/register/extra"}, {"GET", "/api/v1/not-added-yet"},
		{"OPTIONS", "/api/v1/accounts"}, {"GET", "/app/settings/security"},
	} {
		t.Run("denies "+tc.method+" "+tc.path, func(t *testing.T) {
			got := tokenAPIRequest(handler, tc.method, tc.path, `{}`, cookie, csrf, "Bearer "+bothToken)
			require.Equal(t, http.StatusForbidden, got.Code, got.Body.String())
		})
	}
	var versions int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM transaction_versions WHERE transaction_id=?`, transaction.ID).Scan(&versions))
	assert.Equal(t, 1, versions, "every refused mutation preserves the posted journal")
	read := readTransactionForSession(t, handler, cookie, transaction.ID, http.StatusOK)
	assert.Equal(t, "posted", read.Status)
	head := tokenAPIRequest(handler, "HEAD", "/api/v1/accounts", "", nil, "", "Bearer "+ledgerToken)
	assert.Equal(t, http.StatusOK, head.Code)
}

func TestPersonalAccessTokenAPIHeaderPrecedenceAndManagementProtection(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)
	csrf := authSessionCSRFToken(t, handler, cookie)
	body := `{"name":"Synthetic client","password":"test-password","scopes":["ledger:read"]}`
	for _, expiry := range []string{"null", "1.5", `"30"`, "0", "366"} {
		invalid := strings.TrimSuffix(body, "}") + `,"expires_in_days":` + expiry + "}"
		got := tokenAPIRequest(handler, "POST", "/api/v1/auth/tokens", invalid, cookie, csrf, "")
		require.Equal(t, http.StatusBadRequest, got.Code, "expiry must match its bounded integer contract: %s", expiry)
	}
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		csrf   string
	}{
		{"no session", nil, ""}, {"no csrf", cookie, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := tokenAPIRequest(handler, "POST", "/api/v1/auth/tokens", body, tc.cookie, tc.csrf, "")
			assert.Contains(t, []int{401, 403}, response.Code)
		})
	}
	// A valid CSRF token without Origin still refuses browser creation.
	req := httptest.NewRequest("POST", "/api/v1/auth/tokens", strings.NewReader(body))
	req.AddCookie(cookie)
	req.Header.Set(csrfTokenHeader, csrf)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	assert.Equal(t, 403, response.Code)
	badPassword := tokenAPIRequest(handler, "POST", "/api/v1/auth/tokens", strings.Replace(body, "test-password", "wrong-password", 1), cookie, csrf, "")
	assert.Equal(t, 401, badPassword.Code)
	credential, token := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead})
	for _, headers := range [][]string{{""}, {"Basic abc"}, {"Bearer"}, {"Bearer invalid"}, {"Bearer " + token + ", ignored"}, {"Bearer " + token, "Bearer " + token}, {"Bearer " + token + " trailing"}} {
		req := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
		req.AddCookie(cookie)
		for _, header := range headers {
			req.Header.Add("Authorization", header)
		}
		got := httptest.NewRecorder()
		handler.ServeHTTP(got, req)
		require.Equal(t, 401, got.Code, "invalid auth never falls back to the valid session")
		assert.Contains(t, got.Body.String(), "UNAUTHENTICATED")
	}
	list := tokenAPIRequest(handler, "GET", "/api/v1/auth/tokens", "", cookie, "", "")
	require.Equal(t, 200, list.Code)
	assert.NotContains(t, list.Body.String(), token)
	assert.NotContains(t, list.Body.String(), "token_hash")
	queryOnly := tokenAPIRequest(handler, "GET", "/api/v1/accounts?access_token="+token, "", nil, "", "")
	assert.Equal(t, 401, queryOnly.Code, "URL tokens are not authentication")
	// Expire a real issued token, then check the real HTTP middleware.
	_, err := database.Exec(`UPDATE personal_access_tokens SET created_at='2020-01-01T00:00:00Z',expires_at='2020-02-01T00:00:00Z' WHERE id=?`, credential.ID)
	require.NoError(t, err)
	expired := tokenAPIRequest(handler, "GET", "/api/v1/accounts", "", cookie, csrf, "Bearer "+token)
	assert.Equal(t, 401, expired.Code)
	revokedCredential, revokedToken := createTokenAPI(t, handler, cookie, csrf, []string{app.TokenScopeLedgerRead})
	path := fmt.Sprintf("/api/v1/auth/tokens/%d", revokedCredential.ID)
	assert.Equal(t, 403, tokenAPIRequest(handler, "DELETE", path, "", cookie, "", "").Code)
	assert.Equal(t, 204, tokenAPIRequest(handler, "DELETE", path, "", cookie, csrf, "").Code)
	assert.Equal(t, 401, tokenAPIRequest(handler, "GET", "/api/v1/accounts", "", nil, "", "Bearer "+revokedToken).Code)
	assert.Equal(t, 404, tokenAPIRequest(handler, "DELETE", path, "", cookie, csrf, "").Code)
	var id int64
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT user_id FROM personal_access_tokens WHERE id=?`, credential.ID).Scan(&id))
	assert.Equal(t, int64(1), id)
}

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAccountsRequiresAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestCreateAccountPersistsCommodityInstitutionAndMetadata(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	institution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Example Bank",
		"kind":"bank",
		"country_code":"NL"
	}`)

	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Everyday Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"institution_id":`+strconvFormatInt(institution.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`,
		"quantity_scale_override":2,
		"number_last4":"1234",
		"metadata":{"statement_day":15},
		"effective_from":"2020-01-15",
		"change_reason":"Imported from legacy account list"
	}`)

	assert.Equal(t, "Everyday Checking", account.Name)
	assert.Equal(t, "asset", account.AccountClass)
	assert.Equal(t, "checking", account.AccountKind)
	assert.Equal(t, "active", account.Status)
	assert.Equal(t, "NL", account.CountryCode)
	require.NotNil(t, account.InstitutionID)
	assert.Equal(t, institution.ID, *account.InstitutionID)
	require.NotNil(t, account.DefaultCommodityID)
	assert.Equal(t, currencyID, *account.DefaultCommodityID)
	require.NotNil(t, account.QuantityScaleOverride)
	assert.Equal(t, 2, *account.QuantityScaleOverride)
	assert.Equal(t, "1234", account.NumberLast4)
	assert.Equal(t, `{"statement_day":15}`, string(account.Metadata))
	assert.Equal(t, "2020-01-15", account.EffectiveFrom)
	assert.Equal(t, "Imported from legacy account list", account.ChangeReason)
}

func TestUpdateAccountCreatesAppendOnlyVersionAndClearsOptionalFields(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Old Savings",
		"account_class":"asset",
		"account_kind":"savings",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`,
		"quantity_scale_override":2,
		"number_last4":"9876",
		"comment_markdown":"old note",
		"metadata":{"old":true}
	}`)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+strconvFormatInt(account.ID), strings.NewReader(`{
		"name":"Renamed Savings",
		"account_class":"asset",
		"account_kind":"savings",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`,
		"effective_from":"2021-02-03",
		"change_reason":"Corrected account name"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body accountResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, account.ID, body.ID)
	assert.Equal(t, "Renamed Savings", body.Name)
	assert.Nil(t, body.QuantityScaleOverride)
	assert.Equal(t, "", body.NumberLast4)
	assert.Equal(t, "", body.CommentMarkdown)
	assert.Equal(t, "{}", string(body.Metadata))
	assert.Equal(t, "2021-02-03", body.EffectiveFrom)
	assert.Equal(t, "Corrected account name", body.ChangeReason)

	var versionCount int
	err := database.QueryRowContext(context.Background(), `
		SELECT COUNT(1)
		FROM account_versions
		WHERE account_id = ?
	`, account.ID).Scan(&versionCount)
	require.NoError(t, err)
	assert.Equal(t, 2, versionCount)

	versions := listAccountsForSession(t, handler, sessionCookie, "/"+strconvFormatInt(account.ID)+"/versions")
	require.Len(t, versions.Accounts, 2)
	assert.Equal(t, "Renamed Savings", versions.Accounts[0].Name)
	assert.Equal(t, "Corrected account name", versions.Accounts[0].ChangeReason)
	assert.Equal(t, "Old Savings", versions.Accounts[1].Name)
}

func TestAccountLifecycleRequiresCloseBeforeArchiveAndRestoresClosed(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Lifecycle Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/archive", http.StatusConflict)

	closed := mutateAccountWithBody(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/close", `{
		"effective_from":"2022-05-01",
		"change_reason":"Closed after card replacement"
	}`, http.StatusOK)
	assert.Equal(t, "closed", closed.Status)
	assert.Equal(t, "2022-05-01", closed.EffectiveFrom)
	assert.Equal(t, "Closed after card replacement", closed.ChangeReason)

	archived := mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/archive", http.StatusOK)
	assert.Equal(t, "archived", archived.Status)

	defaultList := listAccountsForSession(t, handler, sessionCookie, "")
	assert.Empty(t, defaultList.Accounts)

	archivedList := listAccountsForSession(t, handler, sessionCookie, "?status=archived")
	require.Len(t, archivedList.Accounts, 1)
	assert.Equal(t, account.ID, archivedList.Accounts[0].ID)

	restored := mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/restore", http.StatusOK)
	assert.Equal(t, "closed", restored.Status)

	reopened := mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/reopen", http.StatusOK)
	assert.Equal(t, "active", reopened.Status)
}

func TestAccountValidationRejectsPostingAssetWithoutCommodityAndNonObjectMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "posting asset without commodity",
			body: `{"name":"No Commodity","account_class":"asset","account_kind":"checking"}`,
		},
		{
			name: "metadata array",
			body: `{"name":"Bad Metadata","account_class":"income","metadata":[1,2]}`,
		},
		{
			name: "metadata string",
			body: `{"name":"Bad Metadata","account_class":"income","metadata":"string"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newSetupTestHandler(t)
			sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfTokenHeader, csrfToken)
			setSameOrigin(req)
			req.AddCookie(sessionCookie)
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusBadRequest, res.Code)
		})
	}
}

func TestAccountParentRulesRejectCyclesAndClassMismatch(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	parent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Brokerage",
		"account_class":"asset",
		"account_kind":"investment",
		"allows_postings":false
	}`)
	child := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Brokerage Cash",
		"account_class":"asset",
		"account_kind":"brokerage_cash",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)

	patchAccount(t, handler, sessionCookie, csrfToken, parent.ID, `{
		"name":"Brokerage",
		"account_class":"asset",
		"account_kind":"investment",
		"parent_account_id":`+strconvFormatInt(child.ID)+`,
		"allows_postings":false
	}`, http.StatusBadRequest)

	patchAccount(t, handler, sessionCookie, csrfToken, child.ID, `{
		"name":"Wrong Parent",
		"account_class":"liability",
		"account_kind":"loan",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`, http.StatusBadRequest)
}

func TestAccountParentRulesRejectArchivedParent(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	parent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Old Parent",
		"account_class":"asset",
		"account_kind":"investment",
		"allows_postings":false
	}`)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(parent.ID)+"/close", http.StatusOK)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(parent.ID)+"/archive", http.StatusOK)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(`{
		"name":"Child",
		"account_class":"asset",
		"account_kind":"checking",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestUpdateAccountRevalidatesPostingCommodityRequirement(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Container",
		"account_class":"asset",
		"account_kind":"investment",
		"allows_postings":false
	}`)

	patchAccount(t, handler, sessionCookie, csrfToken, account.ID, `{
		"name":"Container",
		"account_class":"asset",
		"account_kind":"investment",
		"allows_postings":true
	}`, http.StatusBadRequest)
}

func TestSystemAccountSetupCreatesRolesAndProtectsThem(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/system-accounts", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body completeSystemAccountsSetupResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Len(t, body.Accounts, 5)
	assert.Equal(t, setupStepResponse{Key: "system_accounts", Status: "completed"}, body.Setup.Steps[3])

	roles := make(map[string]bool, len(body.Accounts))
	for _, account := range body.Accounts {
		assert.True(t, account.IsSystem)
		assert.Equal(t, "active", account.Status)
		assert.Equal(t, "equity", account.AccountClass)
		roles[account.SystemRole] = true
	}
	assert.True(t, roles["opening_balance"])
	assert.True(t, roles["imbalance_import"])
	assert.True(t, roles["retained_earnings"])
	assert.True(t, roles["income_summary"])
	assert.True(t, roles["expense_summary"])

	patchAccount(t, handler, sessionCookie, csrfToken, body.Accounts[0].ID, `{}`, http.StatusConflict)

	var completedAt sql.NullString
	err := database.QueryRowContext(context.Background(), `
		SELECT completed_at
		FROM setup_steps
		WHERE step_key = 'system_accounts'
	`).Scan(&completedAt)
	require.NoError(t, err)
	assert.True(t, completedAt.Valid)
}

func TestSystemAccountSetupCannotRunTwice(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusConflict)
}

func TestAccountMutationsRequireAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/accounts", body: `{"name":"No Auth","account_class":"income"}`},
		{name: "update", method: http.MethodPatch, path: "/api/v1/accounts/1", body: `{"name":"No Auth","account_class":"income"}`},
		{name: "close", method: http.MethodPost, path: "/api/v1/accounts/1/close"},
		{name: "reopen", method: http.MethodPost, path: "/api/v1/accounts/1/reopen"},
		{name: "archive", method: http.MethodPost, path: "/api/v1/accounts/1/archive"},
		{name: "restore", method: http.MethodPost, path: "/api/v1/accounts/1/restore"},
		{name: "system setup", method: http.MethodPost, path: "/api/v1/setup/system-accounts"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newSetupTestHandler(t)
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusUnauthorized, res.Code)
		})
	}
}

func setupAccountAPITest(t *testing.T, handler http.Handler) (*http.Cookie, string, int64) {
	t.Helper()

	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	currencySetup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{
		{Code: "USD"},
	})

	return sessionCookie, csrfToken, currencySetup.DefaultCurrency.ID
}

func createAccountForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) accountResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var response accountResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func patchAccount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, accountID int64, body string, wantStatus int) accountResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/"+strconvFormatInt(accountID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response accountResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}

	return response
}

func listAccountsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) accountsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response accountsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))

	return response
}

func mutateAccount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) accountResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response accountResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}

	return response
}

func mutateAccountWithBody(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) accountResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response accountResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}

	return response
}

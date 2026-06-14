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

	handler, database := newSetupTestHandler(t)
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
		"opened_on":"2019-12-01",
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
	assert.Equal(t, "2019-12-01", account.OpenedOn)
	assert.Equal(t, "", account.ClosedOn)
	assert.Equal(t, "2020-01-15", account.EffectiveFrom)
	assert.Equal(t, "Imported from legacy account list", account.ChangeReason)

	var (
		operation        string
		originType       string
		actorUserID      int64
		hasAuthSessionID int
		hasRequestID     int
		reason           string
	)
	err := database.QueryRowContext(context.Background(), `
		SELECT
			ae.operation,
			ae.origin_type,
			ae.actor_user_id,
			ae.auth_session_id IS NOT NULL,
			ae.request_id IS NOT NULL,
			ae.reason
		FROM accounts a
		JOIN account_versions av ON av.account_id = a.id
		JOIN audit_events ae ON ae.id = av.change_audit_event_id
		WHERE a.id = ?
		  AND a.created_audit_event_id = av.change_audit_event_id
	`, account.ID).Scan(&operation, &originType, &actorUserID, &hasAuthSessionID, &hasRequestID, &reason)
	require.NoError(t, err)
	assert.Equal(t, "account.create", operation)
	assert.Equal(t, "browser_api", originType)
	assert.Equal(t, int64(1), actorUserID)
	assert.Equal(t, 1, hasAuthSessionID)
	assert.Equal(t, 1, hasRequestID)
	assert.Equal(t, "Imported from legacy account list", reason)
}

func TestCreateAccountSupportsUserFacingGroupedKinds(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)

	otherMoney := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Stored Value",
		"account_class":"asset",
		"account_kind":"other_money_account",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)
	assert.Equal(t, "asset", otherMoney.AccountClass)
	assert.Equal(t, "other_money_account", otherMoney.AccountKind)

	fundHolding := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Mutual Fund",
		"account_class":"asset",
		"account_kind":"fund_holding",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)
	assert.Equal(t, "asset", fundHolding.AccountClass)
	assert.Equal(t, "fund_holding", fundHolding.AccountKind)
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
		"metadata":{"old":true},
		"opened_on":"2020-01-01"
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
	assert.Equal(t, account.OpenedOn, body.OpenedOn)
	assert.Equal(t, "", body.ClosedOn)
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

	var operation string
	err = database.QueryRowContext(context.Background(), `
		SELECT audit_events.operation
		FROM account_versions
		JOIN audit_events ON audit_events.id = account_versions.change_audit_event_id
		WHERE account_versions.account_id = ?
		ORDER BY account_versions.version_seq DESC
		LIMIT 1
	`, account.ID).Scan(&operation)
	require.NoError(t, err)
	assert.Equal(t, "account.update", operation)

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
		"default_commodity_id":`+strconvFormatInt(currencyID)+`,
		"opened_on":"2020-01-01"
	}`)

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/archive", http.StatusConflict)

	closed := mutateAccountWithBody(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/close", `{
		"closed_on":"2022-05-01",
		"change_reason":"Closed after card replacement"
	}`, http.StatusOK)
	assert.Equal(t, "closed", closed.Status)
	assert.Equal(t, "2022-05-01", closed.EffectiveFrom)
	assert.Equal(t, "2022-05-01", closed.ClosedOn)
	assert.Equal(t, "Closed after card replacement", closed.ChangeReason)

	archived := mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/archive", http.StatusOK)
	assert.Equal(t, "archived", archived.Status)
	assert.Equal(t, "2022-05-01", archived.ClosedOn)

	defaultList := listAccountsForSession(t, handler, sessionCookie, "")
	assert.Empty(t, defaultList.Accounts)

	archivedList := listAccountsForSession(t, handler, sessionCookie, "?status=archived")
	require.Len(t, archivedList.Accounts, 1)
	assert.Equal(t, account.ID, archivedList.Accounts[0].ID)

	restored := mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/restore", http.StatusOK)
	assert.Equal(t, "closed", restored.Status)
	assert.Equal(t, "2022-05-01", restored.ClosedOn)

	reopened := mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/reopen", http.StatusOK)
	assert.Equal(t, "active", reopened.Status)
	assert.Equal(t, "", reopened.ClosedOn)
}

func TestAccountCloseRejectsDateBeforeOpenedOn(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"New Account",
		"account_class":"asset",
		"account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`,
		"opened_on":"2024-01-01"
	}`)

	mutateAccountWithBody(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(account.ID)+"/close", `{
		"closed_on":"2023-12-31"
	}`, http.StatusBadRequest)
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
		{
			name: "category-like kind rejected",
			body: `{"name":"Salary","account_class":"income","account_kind":"salary"}`,
		},
		{
			name: "user-created equity rejected",
			body: `{"name":"Manual Adjustment","account_class":"equity","account_kind":"equity"}`,
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

func TestAccountValidationAllowsTransientAccountsWithoutDefaultCommodity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		accountClass string
		accountKind  string
	}{
		{
			name:         "receivable",
			body:         `{"name":"Expected refund","account_class":"asset","account_kind":"receivable"}`,
			accountClass: "asset",
			accountKind:  "receivable",
		},
		{
			name:         "payable",
			body:         `{"name":"Pending payment","account_class":"liability","account_kind":"payable"}`,
			accountClass: "liability",
			accountKind:  "payable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newSetupTestHandler(t)
			sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

			account := createAccountForSession(t, handler, sessionCookie, csrfToken, test.body)

			assert.Equal(t, test.accountClass, account.AccountClass)
			assert.Equal(t, test.accountKind, account.AccountKind)
			assert.Nil(t, account.DefaultCommodityID)
			assert.True(t, account.AllowsPostings)
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
		"account_kind":"brokerage",
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
		"account_kind":"brokerage",
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

func TestAccountParentDerivesInstitutionOnCreateAndUpdate(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	parentInstitution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Parent Bank",
		"kind":"bank",
		"country_code":"NL"
	}`)
	otherInstitution := createInstitutionForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Other Bank",
		"kind":"bank",
		"country_code":"BE"
	}`)
	parent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Brokerage",
		"account_class":"asset",
		"account_kind":"brokerage",
		"institution_id":`+strconvFormatInt(parentInstitution.ID)+`,
		"allows_postings":false
	}`)

	child := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Brokerage Cash",
		"account_class":"asset",
		"account_kind":"brokerage_cash",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"institution_id":`+strconvFormatInt(otherInstitution.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)
	require.NotNil(t, child.InstitutionID)
	assert.Equal(t, parentInstitution.ID, *child.InstitutionID)
	assert.Equal(t, "NL", child.CountryCode)

	standalone := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Standalone Cash",
		"account_class":"asset",
		"account_kind":"checking",
		"institution_id":`+strconvFormatInt(otherInstitution.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)
	require.NotNil(t, standalone.InstitutionID)
	assert.Equal(t, otherInstitution.ID, *standalone.InstitutionID)

	updated := patchAccount(t, handler, sessionCookie, csrfToken, standalone.ID, `{
		"name":"Standalone Cash",
		"account_class":"asset",
		"account_kind":"checking",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"institution_id":`+strconvFormatInt(otherInstitution.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`, http.StatusOK)
	require.NotNil(t, updated.InstitutionID)
	assert.Equal(t, parentInstitution.ID, *updated.InstitutionID)
	assert.Equal(t, "NL", updated.CountryCode)
}

func TestAccountParentRulesRejectArchivedParent(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	parent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Old Parent",
		"account_class":"asset",
		"account_kind":"brokerage",
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
		"account_kind":"brokerage",
		"allows_postings":false
	}`)

	patchAccount(t, handler, sessionCookie, csrfToken, account.ID, `{
		"name":"Container",
		"account_class":"asset",
		"account_kind":"brokerage",
		"allows_postings":true
	}`, http.StatusBadRequest)
}

func TestCreateAccountCanIntroduceCatalogCurrencyOnSave(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Euro Cash",
		"account_class":"asset",
		"account_kind":"cash",
		"default_commodity_code":"EUR"
	}`)

	require.NotNil(t, account.DefaultCommodityID)

	currencies := listCurrenciesForSession(t, handler, sessionCookie)
	assert.Equal(t, []string{"EUR", "USD"}, currencyCodes(currencies.Currencies))
}

func TestUpdateAccountRejectsCurrencyChangeAfterPostingsExist(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	income := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Salary",
		"category_type":"income"
	}`)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, 10000, 2, commodityID),
		posting(income.ID, -10000, 2, commodityID),
	), http.StatusCreated)

	patchAccount(t, handler, sessionCookie, csrfToken, checking.ID, `{
		"name":"Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"default_commodity_code":"EUR",
		"quantity_scale_override":2
	}`, http.StatusConflict)
}

func TestSystemAccountSetupCreatesRolesAndProtectsThem(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/system-accounts", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var body completeSystemAccountsSetupResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Len(t, body.Accounts, 8)
	assert.Equal(t, setupStepResponse{Key: "system_accounts", Status: "completed"}, body.Setup.Steps[3])

	expectedRoles := map[string]struct {
		accountClass string
		accountKind  string
	}{
		"opening_balance":    {accountClass: "equity", accountKind: "equity"},
		"import_imbalance":   {accountClass: "equity", accountKind: "equity"},
		"retained_earnings":  {accountClass: "equity", accountKind: "equity"},
		"unassigned_income":  {accountClass: "income", accountKind: "income"},
		"unassigned_expense": {accountClass: "expense", accountKind: "expense"},
		"transfer_clearing":  {accountClass: "asset", accountKind: "receivable"},
		"commodity_trading":  {accountClass: "equity", accountKind: "equity"},
	}
	seenRoles := make(map[string]bool, len(expectedRoles))
	var starterCashAccounts []accountResponse
	for _, account := range body.Accounts {
		assert.Equal(t, "active", account.Status)

		if !account.IsSystem {
			starterCashAccounts = append(starterCashAccounts, account)
			continue
		}

		assert.Nil(t, account.DefaultCommodityID)
		assert.Equal(t, "0001-01-01", account.OpenedOn)
		assert.Equal(t, "0001-01-01", account.EffectiveFrom)

		expected, ok := expectedRoles[account.SystemRole]
		require.True(t, ok, "unexpected system role %q", account.SystemRole)
		assert.Equal(t, expected.accountClass, account.AccountClass)
		assert.Equal(t, expected.accountKind, account.AccountKind)
		seenRoles[account.SystemRole] = true
	}
	assert.Equal(t, map[string]bool{
		"opening_balance":    true,
		"import_imbalance":   true,
		"retained_earnings":  true,
		"unassigned_income":  true,
		"unassigned_expense": true,
		"transfer_clearing":  true,
		"commodity_trading":  true,
	}, seenRoles)
	require.Len(t, starterCashAccounts, 1)
	assert.False(t, starterCashAccounts[0].IsSystem)
	assert.Empty(t, starterCashAccounts[0].SystemRole)
	assert.Equal(t, "cash:USD", starterCashAccounts[0].Code)
	assert.Empty(t, starterCashAccounts[0].Name)
	assert.Equal(t, "asset", starterCashAccounts[0].AccountClass)
	assert.Equal(t, "cash", starterCashAccounts[0].AccountKind)
	require.NotNil(t, starterCashAccounts[0].DefaultCommodityID)
	assert.Equal(t, currencyID, *starterCashAccounts[0].DefaultCommodityID)

	patchAccount(t, handler, sessionCookie, csrfToken, body.Accounts[0].ID, `{}`, http.StatusConflict)

	defaultList := listAccountsForSession(t, handler, sessionCookie, "")
	require.Len(t, defaultList.Accounts, 1)
	assert.Equal(t, "cash:USD", defaultList.Accounts[0].Code)

	systemList := listAccountsForSession(t, handler, sessionCookie, "?include_system=true")
	require.Len(t, systemList.Accounts, 8)
	var systemCount int
	for _, account := range systemList.Accounts {
		if account.IsSystem {
			assert.NotEmpty(t, account.SystemRole)
			systemCount++
		}
	}
	assert.Equal(t, 7, systemCount)

	var completedAt sql.NullString
	var auditEventID sql.NullInt64
	var operation string
	err := database.QueryRowContext(context.Background(), `
		SELECT setup_steps.completed_at, setup_steps.completed_audit_event_id, audit_events.operation
		FROM setup_steps
		JOIN audit_events ON audit_events.id = setup_steps.completed_audit_event_id
		WHERE setup_steps.step_key = 'system_accounts'
	`).Scan(&completedAt, &auditEventID, &operation)
	require.NoError(t, err)
	assert.True(t, completedAt.Valid)
	assert.True(t, auditEventID.Valid)
	assert.Equal(t, "system_accounts.setup", operation)
}

func TestSystemAccountSetupCannotRunTwice(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusConflict)
}

func TestSystemAccountSetupCreatesStarterCashForDefaultCurrency(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Personal")
	currencySetup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "USD", []setupCurrencySelectionRequest{{Code: "EUR"}})

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	accounts := listAccountsForSession(t, handler, sessionCookie, "").Accounts
	require.Len(t, accounts, 1)

	require.Len(t, currencySetup.Currencies, 1)
	account := accounts[0]
	assert.False(t, account.IsSystem)
	assert.Equal(t, "asset", account.AccountClass)
	assert.Equal(t, "cash", account.AccountKind)
	assert.Equal(t, "cash:USD", account.Code)
	require.NotNil(t, account.DefaultCommodityID)
	assert.Equal(t, currencySetup.DefaultCurrency.ID, *account.DefaultCommodityID)
}

func TestSystemAccountSetupRerunWithoutCompletedStepKeepsAuditEventReferenced(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	_, err := database.ExecContext(context.Background(), `
		UPDATE setup_steps
		SET completed_at = NULL, completed_audit_event_id = NULL
		WHERE step_key = 'system_accounts'
	`)
	require.NoError(t, err)

	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	var unreferencedSetupEvents int
	err = database.QueryRowContext(context.Background(), `
		SELECT COUNT(1)
		FROM audit_events
		WHERE operation = 'system_accounts.setup'
		  AND id NOT IN (
			-- A setup rerun may find all system accounts already present; in
			-- that case setup_steps is the intentional audit-event anchor.
			SELECT completed_audit_event_id
			FROM setup_steps
			WHERE completed_audit_event_id IS NOT NULL
		  )
		  AND id NOT IN (
			SELECT change_audit_event_id
			FROM account_versions
			WHERE change_audit_event_id IS NOT NULL
		  )
	`).Scan(&unreferencedSetupEvents)
	require.NoError(t, err)
	assert.Equal(t, 0, unreferencedSetupEvents)
}

func TestDeleteUnusedAccountRemovesErroneousAccount(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	account := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Typo Checking",
		"account_class":"asset",
		"account_kind":"checking",
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)

	mutateAccountNoResponse(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/accounts/"+strconvFormatInt(account.ID), http.StatusNoContent)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(account.ID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestDeleteAccountRejectsPostingAndChildReferences(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking With Posting", "asset", "checking", currencyID, 2)
	income := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Salary", "income", "income", currencyID, 2)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-12",
		posting(checking.ID, 10000, 2, currencyID),
		posting(income.ID, -10000, 2, currencyID),
	), http.StatusCreated)

	mutateAccountNoResponse(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/accounts/"+strconvFormatInt(checking.ID), http.StatusConflict)

	parent := createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Parent",
		"account_class":"asset",
		"account_kind":"brokerage",
		"allows_postings":false
	}`)
	createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Child",
		"account_class":"asset",
		"account_kind":"checking",
		"parent_account_id":`+strconvFormatInt(parent.ID)+`,
		"default_commodity_id":`+strconvFormatInt(currencyID)+`
	}`)

	mutateAccountNoResponse(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/accounts/"+strconvFormatInt(parent.ID), http.StatusConflict)
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
		{name: "delete", method: http.MethodDelete, path: "/api/v1/accounts/1"},
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

func mutateAccountNoResponse(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())
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

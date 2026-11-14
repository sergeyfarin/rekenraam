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

// The #269 reference-data audit. Expected values are derived by hand from the
// account and category design notes, not from the code under test: an as-of
// read takes, per account, the version with the greatest effective_from on or
// before the read date, and a range read rolls categories up through the tree
// as of the range's end ("descendants resolve per reporting date because
// parent links are versioned").

func TestHistoricalReadsFollowAccountAndCategoryVersions(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	usdText := strconvFormatInt(usd)
	savings := createAccountForSession(t, handler, cookie, csrf, `{
		"name":"Savings","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"quantity_scale_override":2,"opened_on":"2025-01-01"
	}`)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Salary","category_type":"income"}`)
	food := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Food","category_type":"expense"}`)
	personal := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Personal","category_type":"expense"}`)
	coffee := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Coffee","category_type":"expense","parent_category_id":`+strconvFormatInt(food.ID)+`}`)

	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2025-11-15",
		posting(savings.ID, 100000, 2, usd), posting(salary.ID, -100000, 2, usd)), http.StatusCreated)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2025-11-20",
		posting(savings.ID, -450, 2, usd), posting(coffee.ID, 450, 2, usd)), http.StatusCreated)

	// Both changes are made after the postings exist and take effect on
	// 2025-12-01: a rename is descriptive, and a category move is how a
	// category's place in the tree changes over time. (The test handler pins
	// the category service's clock to 2026-01-01, hence the 2025 dates.)
	patchAccount(t, handler, cookie, csrf, savings.ID, `{
		"name":"Reserve","account_class":"asset","account_kind":"savings",
		"default_commodity_id":`+usdText+`,"quantity_scale_override":2,"effective_from":"2025-12-01"
	}`, http.StatusOK)
	patchCategory(t, handler, cookie, csrf, coffee.ID, `{
		"name":"Coffee","parent_category_id":`+strconvFormatInt(personal.ID)+`,"effective_from":"2025-12-01"
	}`, http.StatusOK)

	// 1000.00 in, 4.50 out: 995.50 under whichever name applied on the date.
	for _, read := range []struct{ asOf, name string }{
		{"2025-11-20", "Savings"},
		{"2025-11-30", "Savings"},
		{"2025-12-01", "Reserve"},
		{"2026-01-01", "Reserve"},
	} {
		balances := readAccountBalancesForSession(t, handler, cookie, "?as_of="+read.asOf)
		account := balanceAccountByID(t, balances.Accounts, savings.ID)
		assert.Equal(t, read.name, account.Name, read.asOf)
		require.Len(t, account.DirectBalances, 1, read.asOf)
		assert.Equal(t, "99550", account.DirectBalances[0].QuantityValue.String(), read.asOf)
		assert.Equal(t, 2, account.DirectBalances[0].QuantityScale, read.asOf)
	}
	current := readAccountForSession(t, handler, cookie, savings.ID)
	assert.Equal(t, "Reserve", current.Name)
	versions := listAccountsForSession(t, handler, cookie, "/"+strconvFormatInt(savings.ID)+"/versions")
	require.Len(t, versions.Accounts, 2)

	// The 4.50 spent on 2025-11-20 rolls up to Food while Coffee sits under
	// Food, and to Personal once the range ends after the move.
	type subtree struct{ food, personal, coffee string }
	for _, read := range []struct {
		beforeDate string
		want       subtree
	}{
		{"2025-11-30", subtree{food: "450", personal: "", coffee: "450"}},
		{"2025-12-31", subtree{food: "", personal: "450", coffee: "450"}},
	} {
		totals := readCategoryTotalsForSession(t, handler, cookie, "?category_type=expense&after_date=2025-11-01&before_date="+read.beforeDate)
		got := subtree{
			food:     categorySubtreeValue(t, totals.Categories, food.ID),
			personal: categorySubtreeValue(t, totals.Categories, personal.ID),
			coffee:   categorySubtreeValue(t, totals.Categories, coffee.ID),
		}
		assert.Equal(t, read.want, got, read.beforeDate)
	}
}

// Every structural refusal in the reference-data surface answers with a stable
// error code, not only a status, so a client can tell the reasons apart.
func TestReferenceDataRefusalsCarryStableErrorCodes(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, usd := setupAccountAPITest(t, handler)
	mutateAccount(t, handler, cookie, csrf, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	setup := mutateCategoriesSetup(t, handler, cookie, csrf, http.StatusCreated)
	other := categoryByCode(t, setup.Categories, "expense_other")
	system := listAccountsForSession(t, handler, cookie, "?include_system=true")
	clearing := strconvFormatInt(accountBySystemRole(t, system.Accounts, "transfer_clearing").ID)
	usdText := strconvFormatInt(usd)

	parent := createAccountForSession(t, handler, cookie, csrf, `{"name":"Parent","account_class":"asset","account_kind":"other_asset","allows_postings":false,"opened_on":"2026-01-01"}`)
	child := createAccountForSession(t, handler, cookie, csrf, `{"name":"Child","account_class":"asset","account_kind":"savings","default_commodity_id":`+usdText+`,"parent_account_id":`+strconvFormatInt(parent.ID)+`,"opened_on":"2026-01-01"}`)
	used := createLedgerAccount(t, handler, cookie, csrf, "Used", "asset", "checking", usd, 2)
	salary := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Salary","category_type":"income"}`)
	createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-06-07",
		posting(used.ID, 100, 2, usd), posting(salary.ID, -100, 2, usd)), http.StatusCreated)
	food := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Food","category_type":"expense"}`)
	groceries := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Groceries","category_type":"expense","parent_category_id":`+strconvFormatInt(food.ID)+`}`)
	parentText, childText, usedText := strconvFormatInt(parent.ID), strconvFormatInt(child.ID), strconvFormatInt(used.ID)
	foodText, groceriesText := strconvFormatInt(food.ID), strconvFormatInt(groceries.ID)

	for _, refusal := range []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"account cycle", http.MethodPatch, "/api/v1/accounts/" + parentText,
			`{"name":"Parent","account_class":"asset","account_kind":"other_asset","allows_postings":false,"parent_account_id":` + childText + `}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"account self parent", http.MethodPatch, "/api/v1/accounts/" + parentText,
			`{"name":"Parent","account_class":"asset","account_kind":"other_asset","allows_postings":false,"parent_account_id":` + parentText + `}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"account parent of another class", http.MethodPost, "/api/v1/accounts",
			`{"name":"Card","account_class":"liability","account_kind":"credit_card","default_commodity_id":` + usdText + `,"parent_account_id":` + parentText + `}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"user equity account", http.MethodPost, "/api/v1/accounts",
			`{"name":"Equity","account_class":"equity","account_kind":"equity"}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"future effective date", http.MethodPatch, "/api/v1/accounts/" + childText,
			`{"name":"Child","account_class":"asset","account_kind":"savings","default_commodity_id":` + usdText + `,"parent_account_id":` + parentText + `,"effective_from":"2999-01-01"}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"system account edit", http.MethodPatch, "/api/v1/accounts/" + clearing,
			`{"name":"Clearing","account_class":"asset","account_kind":"receivable"}`,
			http.StatusConflict, "CONFLICT"},
		{"system account close", http.MethodPost, "/api/v1/accounts/" + clearing + "/close", `{}`,
			http.StatusConflict, "CONFLICT"},
		{"system account delete", http.MethodDelete, "/api/v1/accounts/" + clearing, "",
			http.StatusConflict, "CONFLICT"},
		{"structure after postings", http.MethodPatch, "/api/v1/accounts/" + usedText,
			`{"name":"Used","account_class":"asset","account_kind":"savings","default_commodity_id":` + usdText + `,"quantity_scale_override":2}`,
			http.StatusConflict, "CONFLICT"},
		{"precision after postings", http.MethodPatch, "/api/v1/accounts/" + usedText,
			`{"name":"Used","account_class":"asset","account_kind":"checking","default_commodity_id":` + usdText + `,"quantity_scale_override":4}`,
			http.StatusConflict, "CONFLICT"},
		{"precision above the commodity maximum", http.MethodPatch, "/api/v1/accounts/" + childText,
			`{"name":"Child","account_class":"asset","account_kind":"savings","default_commodity_id":` + usdText + `,"parent_account_id":` + parentText + `,"quantity_scale_override":7}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"used account delete", http.MethodDelete, "/api/v1/accounts/" + usedText, "",
			http.StatusConflict, "CONFLICT"},
		{"lifecycle archive while active", http.MethodPost, "/api/v1/accounts/" + childText + "/archive", `{}`,
			http.StatusConflict, "CONFLICT"},
		{"category cycle", http.MethodPatch, "/api/v1/categories/" + foodText,
			`{"name":"Food","parent_category_id":` + groceriesText + `}`,
			http.StatusConflict, "REFERENCE_DATED_CONFLICT"},
		{"category type change", http.MethodPatch, "/api/v1/categories/" + foodText,
			`{"name":"Food","category_type":"income"}`,
			http.StatusBadRequest, "VALIDATION_FAILED"},
		{"category duplicate sibling", http.MethodPost, "/api/v1/categories",
			`{"name":"groceries","category_type":"expense","parent_category_id":` + foodText + `}`,
			http.StatusConflict, "CONFLICT"},
		{"category disable with active child", http.MethodPost, "/api/v1/categories/" + foodText + "/disable", `{}`,
			http.StatusConflict, "REFERENCE_DATED_CONFLICT"},
		{"built-in category edit", http.MethodPatch, "/api/v1/categories/" + strconvFormatInt(other.ID),
			`{"name":"Miscellaneous"}`, http.StatusConflict, "CONFLICT"},
		{"system account as a category", http.MethodPatch, "/api/v1/categories/" + clearing,
			`{"name":"Clearing"}`, http.StatusNotFound, "NOT_FOUND"},
		{"duplicate payee", http.MethodPost, "/api/v1/payees", `{"name":"salary"}`,
			http.StatusConflict, "CONFLICT"},
		{"duplicate tag", http.MethodPost, "/api/v1/tags", `{"name":"trip","kind":"project"}`,
			http.StatusConflict, "CONFLICT"},
	} {
		if refusal.name == "duplicate payee" {
			createPayeeForSession(t, handler, cookie, csrf, `{"name":"Salary"}`)
		}
		if refusal.name == "duplicate tag" {
			created := referenceDataRequest(handler, cookie, csrf, http.MethodPost, "/api/v1/tags", `{"name":"Trip","kind":"project"}`)
			require.Equal(t, http.StatusCreated, created.Code)
		}
		res := referenceDataRequest(handler, cookie, csrf, refusal.method, refusal.path, refusal.body)
		require.Equalf(t, refusal.status, res.Code, "%s: %s", refusal.name, res.Body.String())
		assert.Equal(t, refusal.code, apiErrorCode(t, res), refusal.name)
	}
}

func referenceDataRequest(handler http.Handler, cookie *http.Cookie, csrf, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(csrfTokenHeader, csrf)
	setSameOrigin(req)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func readAccountForSession(t *testing.T, handler http.Handler, cookie *http.Cookie, accountID int64) accountResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(accountID), nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	var account accountResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&account))
	return account
}

func balanceAccountByID(t *testing.T, accounts []accountBalanceResponse, accountID int64) accountBalanceResponse {
	t.Helper()
	for _, account := range accounts {
		if account.AccountID == accountID {
			return account
		}
	}
	require.FailNowf(t, "account missing", "account %d not in balances", accountID)
	return accountBalanceResponse{}
}

func categorySubtreeValue(t *testing.T, categories []categoryTotalResponse, categoryID int64) string {
	t.Helper()
	for _, category := range categories {
		if category.CategoryID != categoryID {
			continue
		}
		if len(category.SubtreeTotals) == 0 {
			return ""
		}
		require.Len(t, category.SubtreeTotals, 1)
		return category.SubtreeTotals[0].QuantityValue.String()
	}
	require.FailNowf(t, "category missing", "category %d not in totals", categoryID)
	return ""
}

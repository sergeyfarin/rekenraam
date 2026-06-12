package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCategoriesSetupSeedsBuiltInsWithoutEnglishCanonicalNames(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	response := mutateCategoriesSetup(t, handler, sessionCookie, csrfToken, http.StatusCreated)

	require.Len(t, response.Categories, 55)
	assert.Equal(t, setupStepResponse{Key: "categories", Status: "completed"}, response.Setup.Steps[4])

	food := categoryByCode(t, response.Categories, "expense_food")
	groceries := categoryByCode(t, response.Categories, "expense_food_groceries")
	salary := categoryByCode(t, response.Categories, "income_salary_wages")

	assert.True(t, food.IsBuiltin)
	assert.Equal(t, "expense.food", food.BuiltinKey)
	assert.Empty(t, food.Name)
	assert.False(t, food.AllowsPostings)
	assert.True(t, groceries.AllowsPostings)
	require.NotNil(t, groceries.ParentCategoryID)
	assert.Equal(t, food.ID, *groceries.ParentCategoryID)
	assert.Equal(t, "income", salary.CategoryType)
	assert.Nil(t, salary.ParentCategoryID)

	var storedName sql.NullString
	require.NoError(t, database.QueryRow(`
		SELECT av.name
		FROM accounts a
		JOIN current_account_versions av ON av.account_id = a.id
		WHERE av.code = 'expense_food'
	`).Scan(&storedName))
	assert.False(t, storedName.Valid)

	var operation string
	require.NoError(t, database.QueryRow(`
		SELECT audit_events.operation
		FROM setup_steps
		JOIN audit_events ON audit_events.id = setup_steps.completed_audit_event_id
		WHERE setup_steps.step_key = 'categories'
	`).Scan(&operation))
	assert.Equal(t, "categories.setup", operation)
}

func TestCategoryCreateUpdateDisableRestoreAndDeleteUnused(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	parent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Groceries",
		"category_type":"expense",
		"allows_postings":false
	}`)
	child := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Dairy",
		"category_type":"expense",
		"parent_category_id":`+strconvFormatInt(parent.ID)+`
	}`)

	assert.Equal(t, "active", child.Status)
	require.NotNil(t, child.ParentCategoryID)
	assert.Equal(t, parent.ID, *child.ParentCategoryID)
	assert.True(t, child.AllowsPostings)
	assert.False(t, child.IsBuiltin)

	read := readCategoryForSession(t, handler, sessionCookie, child.ID, http.StatusOK)
	assert.Equal(t, child.ID, read.ID)
	assert.Equal(t, "Dairy", read.Name)

	patchCategory(t, handler, sessionCookie, csrfToken, child.ID, `{"name":"Bread"}`, http.StatusOK)
	createCategoryForSessionStatus(t, handler, sessionCookie, csrfToken, `{
		"name":"Bread",
		"category_type":"expense",
		"parent_category_id":`+strconvFormatInt(parent.ID)+`
	}`, http.StatusConflict)

	mutateCategory(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/categories/"+strconvFormatInt(parent.ID)+"/disable", `{}`, http.StatusConflict)
	mutateCategory(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/categories/"+strconvFormatInt(child.ID)+"/disable", `{}`, http.StatusOK)
	restored := mutateCategory(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/categories/"+strconvFormatInt(child.ID)+"/restore", `{}`, http.StatusOK)
	assert.Equal(t, "active", restored.Status)

	mutateCategoryNoBody(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/categories/"+strconvFormatInt(child.ID), http.StatusNoContent)
	readCategoryForSession(t, handler, sessionCookie, child.ID, http.StatusNotFound)
}

func TestCategoryDeleteRejectsCategoryUsedByPosting(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, currencyID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", currencyID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Groceries",
		"category_type":"expense"
	}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-12",
		posting(checking.ID, -10000, 2, currencyID),
		posting(expense.ID, 10000, 2, currencyID),
	), http.StatusCreated)

	mutateCategoryNoBody(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/categories/"+strconvFormatInt(expense.ID), http.StatusConflict)
	read := readCategoryForSession(t, handler, sessionCookie, expense.ID, http.StatusOK)
	assert.Equal(t, expense.ID, read.ID)
}

func TestCategoryValidationRejectsParentMismatchCycleAndAccountFields(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	incomeParent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Side Income",
		"category_type":"income",
		"allows_postings":false
	}`)
	expenseParent := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Food",
		"category_type":"expense",
		"allows_postings":false
	}`)
	expenseChild := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Snacks",
		"category_type":"expense",
		"parent_category_id":`+strconvFormatInt(expenseParent.ID)+`
	}`)

	createCategoryForSessionStatus(t, handler, sessionCookie, csrfToken, `{
		"name":"Wrong Parent",
		"category_type":"expense",
		"parent_category_id":`+strconvFormatInt(incomeParent.ID)+`
	}`, http.StatusBadRequest)
	createCategoryForSessionStatus(t, handler, sessionCookie, csrfToken, `{
		"name":"Invalid Kind",
		"category_type":"asset"
	}`, http.StatusBadRequest)
	createCategoryForSessionStatus(t, handler, sessionCookie, csrfToken, `{
		"name":"Bank-ish",
		"category_type":"expense",
		"default_commodity_id":1
	}`, http.StatusBadRequest)
	patchCategory(t, handler, sessionCookie, csrfToken, expenseParent.ID, `{
		"parent_category_id":`+strconvFormatInt(expenseChild.ID)+`
	}`, http.StatusBadRequest)
	patchCategory(t, handler, sessionCookie, csrfToken, expenseChild.ID, `{
		"category_type":"income"
	}`, http.StatusBadRequest)
}

func TestBuiltInCategoryCanBeRenamedDisabledButNotDeleted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	setup := mutateCategoriesSetup(t, handler, sessionCookie, csrfToken, http.StatusCreated)
	other := categoryByCode(t, setup.Categories, "expense_other")

	renamed := patchCategory(t, handler, sessionCookie, csrfToken, other.ID, `{
		"name":"Miscellaneous"
	}`, http.StatusOK)
	assert.True(t, renamed.IsBuiltin)
	assert.Equal(t, "expense.other", renamed.BuiltinKey)
	assert.Equal(t, "Miscellaneous", renamed.Name)
	cleared := patchCategory(t, handler, sessionCookie, csrfToken, other.ID, `{
		"name":""
	}`, http.StatusOK)
	assert.True(t, cleared.IsBuiltin)
	assert.Equal(t, "expense.other", cleared.BuiltinKey)
	assert.Empty(t, cleared.Name)

	mutateCategoryNoBody(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/categories/"+strconvFormatInt(other.ID), http.StatusConflict)
	disabled := mutateCategory(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/categories/"+strconvFormatInt(other.ID)+"/disable", `{}`, http.StatusOK)
	assert.Equal(t, "archived", disabled.Status)
	list := listCategoriesForSession(t, handler, sessionCookie, "")
	assert.NotContains(t, categoryCodes(list.Categories), "expense_other")
	list = listCategoriesForSession(t, handler, sessionCookie, "?include_archived=true")
	assert.Contains(t, categoryCodes(list.Categories), "expense_other")
}

func TestCategoryMutationsRequireAuthentication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "setup", method: http.MethodPost, path: "/api/v1/setup/categories"},
		{name: "create", method: http.MethodPost, path: "/api/v1/categories", body: `{"name":"No Auth","category_type":"expense"}`},
		{name: "update", method: http.MethodPatch, path: "/api/v1/categories/1", body: `{"name":"No Auth"}`},
		{name: "disable", method: http.MethodPost, path: "/api/v1/categories/1/disable"},
		{name: "restore", method: http.MethodPost, path: "/api/v1/categories/1/restore"},
		{name: "delete", method: http.MethodDelete, path: "/api/v1/categories/1"},
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

func mutateCategoriesSetup(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, wantStatus int) completeCategoriesSetupResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/categories", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response completeCategoriesSetupResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func createCategoryForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) categoryResponse {
	t.Helper()
	return createCategoryForSessionStatus(t, handler, sessionCookie, csrfToken, body, http.StatusCreated)
}

func createCategoryForSessionStatus(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string, wantStatus int) categoryResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response categoryResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func patchCategory(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, categoryID int64, body string, wantStatus int) categoryResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/categories/"+strconvFormatInt(categoryID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response categoryResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func mutateCategory(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) categoryResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response categoryResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func mutateCategoryNoBody(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)
}

func readCategoryForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, categoryID int64, wantStatus int) categoryResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories/"+strconvFormatInt(categoryID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response categoryResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listCategoriesForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) categoriesResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response categoriesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func categoryByCode(t *testing.T, categories []categoryResponse, code string) categoryResponse {
	t.Helper()
	for _, category := range categories {
		if category.Code == code {
			return category
		}
	}
	require.FailNowf(t, "category not found", "code %q", code)
	return categoryResponse{}
}

func categoryCodes(categories []categoryResponse) []string {
	codes := make([]string, 0, len(categories))
	for _, category := range categories {
		codes = append(codes, category.Code)
	}
	return codes
}

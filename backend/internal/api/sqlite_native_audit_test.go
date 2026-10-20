package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeSQLiteAuditSearchAndJSONConsumers(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Native audit checking", "asset", "checking", currencyID, 2)
	categoryName := `Native audit "quoted" category'; SELECT load_extension('synthetic'); --`
	body, err := json.Marshal(map[string]string{"name": categoryName, "category_type": "expense"})
	require.NoError(t, err)
	category := createCategoryForSession(t, handler, cookie, csrf, string(body))
	listed := listCategoriesForSession(t, handler, cookie, "?q="+url.QueryEscape("Native audit"))
	require.Len(t, listed.Categories, 1)
	assert.Equal(t, category.ID, listed.Categories[0].ID)
	assert.Equal(t, categoryName, listed.Categories[0].Name)
	assert.Equal(t, "expense", listed.Categories[0].CategoryType, "the real category reader extracts its type from JSON")

	var created []transactionResponse
	for _, description := range []string{"AuditAlpha OR AuditBeta", "AuditAlpha routine"} {
		body := balancedBody("2026-06-07", posting(checking.ID, -2500, 2, currencyID), posting(category.ID, 2500, 2, currencyID))
		var spec map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(body), &spec))
		spec["description"], err = json.Marshal(description)
		require.NoError(t, err)
		encoded, err := json.Marshal(spec)
		require.NoError(t, err)
		created = append(created, createTransactionForSession(t, handler, cookie, csrf, string(encoded), http.StatusCreated))
	}

	// Positive control: raw FTS grammar would broaden this to both rows.
	var rawMatches int
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT count(*) FROM transaction_search WHERE transaction_search MATCH ?`, "AuditAlpha OR missing").Scan(&rawMatches))
	require.Equal(t, 2, rawMatches)
	for _, tc := range []struct {
		name, query string
		want        []int64
	}{
		{"ordinary", "AuditAlpha", []int64{created[0].ID, created[1].ID}},
		{"literal_operator", "AuditAlpha OR AuditBeta", []int64{created[0].ID}},
		{"quoted_tokens", `AuditAlpha" OR "AuditBeta`, []int64{created[0].ID}},
		{"no_fts_operator_injection", "AuditAlpha OR missing", nil},
		{"no_sql_statement_injection", `'; DELETE FROM transactions; --`, nil},
		{"no_column_grammar", "description:AuditAlpha", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := listTransactionsForSession(t, handler, cookie, "?q="+url.QueryEscape(tc.query))
			var ids []int64
			for _, transaction := range response.Transactions {
				ids = append(ids, transaction.ID)
			}
			assert.ElementsMatch(t, tc.want, ids)
		})
	}
	for _, transaction := range created {
		assert.Equal(t, transaction, readTransactionForSession(t, handler, cookie, transaction.ID, http.StatusOK), "search must not modify any transaction field")
	}
}

func TestNativeSQLiteAuditSearchRejectsNULBeforeFTS(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, currencyID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "NUL audit checking", "asset", "checking", currencyID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"NUL audit expense","category_type":"expense"}`)
	transaction := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-06-07", posting(checking.ID, -2500, 2, currencyID), posting(category.ID, 2500, 2, currencyID)), http.StatusCreated)
	for _, query := range []string{"\x00", "AuditAlpha\x00 OR missing", "AuditAlpha\x00"} {
		t.Run(url.QueryEscape(query), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions?q="+url.QueryEscape(query), nil)
			req.AddCookie(cookie)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
			assert.Contains(t, res.Body.String(), `"code":"VALIDATION_FAILED"`)
			assert.False(t, strings.Contains(res.Body.String(), "unterminated"), "native parser details must not escape")
		})
	}
	assert.Equal(t, transaction, readTransactionForSession(t, handler, cookie, transaction.ID, http.StatusOK))
}

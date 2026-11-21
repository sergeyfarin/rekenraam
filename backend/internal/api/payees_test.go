package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayeeDirectoryUsageMatchesTransactionDrillDown(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodity := setupAccountAPITest(t, handler)
	cash := createLedgerAccount(t, handler, cookie, csrf, "Checking", "asset", "checking", commodity, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Groceries","category_type":"expense"}`)
	payee := createPayeeForSession(t, handler, cookie, csrf, `{"name":"Кофе 100%"}`)
	unused := createPayeeForSession(t, handler, cookie, csrf, `{"name":"Unused"}`)
	payeeBody := func(day string) string {
		body := balancedBody(day, posting(cash.ID, -100, 2, commodity), posting(expense.ID, 100, 2, commodity))
		return strings.Replace(body, "{", fmt.Sprintf(`{"payee_id":%d,`, payee.ID), 1)
	}
	txPath := func(day string) string {
		tx := createTransactionForSession(t, handler, cookie, csrf, payeeBody(day), http.StatusCreated)
		return fmt.Sprintf("/api/v1/transactions/%d", tx.ID)
	}
	txPath("2026-06-07")
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, txPath("2026-06-08")+"/void", `{"change_reason":"duplicate"}`, http.StatusOK)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, txPath("2026-06-09")+"/soft-delete", `{"change_reason":"duplicate"}`, http.StatusOK)
	draftDay := "2026-06-10"
	draft := generateRecurringDraft(t, handler, database, cookie, draftDay, cash.ID, expense.ID, commodity, 100)
	draft = mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, fmt.Sprintf("/api/v1/transactions/%d", draft.ID), payeeBody(draftDay), http.StatusOK)
	require.Equal(t, "draft", draft.Status)
	require.Equal(t, &payee.ID, draft.PayeeID)
	mutatePayee(t, handler, cookie, csrf, http.MethodPost, fmt.Sprintf("/api/v1/payees/%d/archive", payee.ID), `{}`, http.StatusOK)

	active := readPayeeDirectory(t, handler, cookie, "")
	require.Len(t, active.Payees, 1)
	assert.Equal(t, payeeDirectoryRow{ID: unused.ID, Name: unused.Name, Status: "active"}, active.Payees[0])
	archived := readPayeeDirectory(t, handler, cookie, "?status=archived&q="+url.QueryEscape("КОФЕ 100%"))
	require.Len(t, archived.Payees, 1)
	lastUsed := "2026-06-08"
	assert.Equal(t, payeeDirectoryRow{ID: payee.ID, Name: payee.Name, Status: "archived", TransactionCount: 2, LastUsedOn: &lastUsed}, archived.Payees[0])
	linked := listTransactionsForSession(t, handler, cookie, fmt.Sprintf("?payee_id=%d", payee.ID))
	assert.Len(t, linked.Transactions, 2)
	drafts := listTransactionsForSession(t, handler, cookie, fmt.Sprintf("?status=draft&payee_id=%d", payee.ID))
	require.Len(t, drafts.Transactions, 1)
	assert.Equal(t, draft.ID, drafts.Transactions[0].ID)
	assert.Len(t, readPayeeDirectory(t, handler, cookie, "?status=all").Payees, 2)
	assert.Empty(t, readPayeeDirectory(t, handler, cookie, "?status=all&q=_").Payees)
}

func TestPayeeDirectoryPagesPastTwoThousand(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	for i := range 2001 {
		createPayeeForSession(t, handler, cookie, csrf, fmt.Sprintf(`{"name":"Vendor %04d"}`, i))
	}

	index := 0
	cursor := ""
	for {
		page := readPayeeDirectory(t, handler, cookie, "?cursor="+url.QueryEscape(cursor))
		require.NotEmpty(t, page.Payees)
		require.LessOrEqual(t, len(page.Payees), 50)
		for _, row := range page.Payees {
			require.Equal(t, fmt.Sprintf("Vendor %04d", index), row.Name)
			index++
		}
		if page.NextCursor == "" {
			break
		}
		require.NotEqual(t, cursor, page.NextCursor)
		cursor = page.NextCursor
	}
	require.Equal(t, 2001, index)
	matches := readPayeeDirectory(t, handler, cookie, "?q=vendor+2000")
	require.Len(t, matches.Payees, 1)
	assert.Equal(t, "Vendor 2000", matches.Payees[0].Name)
	for _, query := range []string{"?status=invalid", "?cursor=invalid"} {
		doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/payees/directory"+query, nil, http.StatusBadRequest)
	}
}

func readPayeeDirectory(t *testing.T, handler http.Handler, cookie *http.Cookie, query string) payeeDirectoryResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payees/directory"+query, nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var result payeeDirectoryResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.NotNil(t, result.Payees)
	return result
}

func TestPayeeCreateUpdateArchiveRestoreAndRead(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	payee := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Albert Heijn"}`)
	assert.Equal(t, "active", payee.Status)
	assert.Equal(t, "Albert Heijn", payee.Name)

	read := readPayeeForSession(t, handler, sessionCookie, payee.ID, http.StatusOK)
	assert.Equal(t, payee.ID, read.ID)

	updated := mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/payees/"+strconvFormatInt(payee.ID), `{"name":"AH"}`, http.StatusOK)
	assert.Equal(t, "AH", updated.Name)

	archived := mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(payee.ID)+"/archive", `{"change_reason":"cleanup"}`, http.StatusOK)
	assert.Equal(t, "archived", archived.Status)

	list := listPayeesForSession(t, handler, sessionCookie, "")
	assert.Empty(t, list.Payees)

	restored := mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(payee.ID)+"/restore", `{}`, http.StatusOK)
	assert.Equal(t, "active", restored.Status)
}

func readPayeeForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, payeeID int64, wantStatus int) payeeResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payees/"+strconvFormatInt(payeeID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response payeeResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listPayeesForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) payeesResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payees"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response payeesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

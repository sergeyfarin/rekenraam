package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/testdb"
)

// TestAccountLabelPartsReachEveryAccountRead pins the starter label parts on
// every read that names asset accounts without a full account record (#597
// sweep, bug class 13). The unnamed starter cash account (code cash:USD,
// metadata starter_key "cash", currency_code "USD") reads as the localized
// "Cash USD" only when its read carries those parts; without them the
// screens fell back to the raw code. A named account carries nulls. A
// reconciliation checkpoint carries the same parts.
func TestAccountLabelPartsReachEveryAccountRead(t *testing.T) {
	t.Parallel()
	// The test handler pins the forecast and budget clocks to 2026-09-07;
	// seed the starter account before that, as a real book would have it.
	database, databaseURL := testdb.Open(t)
	instance := newTestInstanceOnDatabase(t, database, databaseURL, HandlerOptions{})
	instance.services.Account.SetNowForTest(func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) })
	handler := instance.handler
	f := bootstrapInvestmentAPITest(t, handler)

	accounts := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/accounts", nil, http.StatusOK)
	var list struct {
		Accounts []accountResponse `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(accounts.Body.Bytes(), &list))
	var starterID int64
	for _, account := range list.Accounts {
		if account.Code == "cash:USD" {
			starterID = account.ID
		}
	}
	require.NotZero(t, starterID, "the book seeds an unnamed USD starter cash account")

	session := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, "/api/v1/auth/session", nil, http.StatusOK)
	var owner struct {
		OwnerToday string `json:"owner_today"`
	}
	require.NoError(t, json.Unmarshal(session.Body.Bytes(), &owner))
	created := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/transactions", map[string]any{
		"status": "posted", "transaction_date": owner.OwnerToday,
		"journal_entries": []map[string]any{{"entry_date": owner.OwnerToday, "postings": []map[string]any{
			{"account_id": f.cashAccount.ID, "commodity_id": f.commodityID, "quantity_value": "-2500", "quantity_scale": 2},
			{"account_id": starterID, "commodity_id": f.commodityID, "quantity_value": "2500", "quantity_scale": 2},
		}}},
	}, http.StatusCreated)

	// Each read: the rows it returns for the two accounts, keyed by account ID.
	type read struct {
		name              string
		path              string
		rows              func(body []byte) []map[string]any
		id, starter, curr string
	}
	rowsAt := func(keys ...string) func(body []byte) []map[string]any {
		return func(body []byte) []map[string]any {
			var node any
			require.NoError(t, json.Unmarshal(body, &node))
			for _, key := range keys {
				node = node.(map[string]any)[key]
			}
			rows := []map[string]any{}
			for _, row := range node.([]any) {
				rows = append(rows, row.(map[string]any))
			}
			return rows
		}
	}
	postings := func(body []byte) []map[string]any {
		var transaction struct {
			JournalEntries []struct {
				Postings []map[string]any `json:"postings"`
			} `json:"journal_entries"`
		}
		require.NoError(t, json.Unmarshal(body, &transaction))
		require.Len(t, transaction.JournalEntries, 1)
		return transaction.JournalEntries[0].Postings
	}
	registerPostings := func(body []byte) []map[string]any {
		rows := []map[string]any{}
		for _, entry := range rowsAt("entries")(body) {
			rows = append(rows, entry["posting"].(map[string]any))
		}
		return rows
	}
	month := owner.OwnerToday[:8] + "01"
	reads := []read{
		{name: "allocation", path: fmt.Sprintf("/api/v1/investments/allocation?from=%s&to=%s&reporting_currency_id=%d", owner.OwnerToday, owner.OwnerToday, f.commodityID),
			rows: rowsAt("accounts"), id: "id", starter: "starter_key", curr: "currency_code"},
		{name: "forecast", path: "/api/v1/forecasts/balances?horizon_days=2", rows: rowsAt("scope", "account_options"),
			id: "id", starter: "starter_key", curr: "currency_code"},
		{name: "budgets", path: "/api/v1/budgets/month?period_start=" + month, rows: rowsAt("accounts"),
			id: "id", starter: "starter_key", curr: "currency_code"},
		{name: "search", path: "/api/v1/search/catalog", rows: rowsAt("items"),
			id: "id", starter: "starter_key", curr: "currency_code"},
	}
	for _, r := range reads {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, r.path, nil, http.StatusOK)
		assertStarterLabelParts(t, r.name, r.rows(res.Body.Bytes()), r.id, r.starter, r.curr, starterID, f.cashAccount.ID)
	}
	// A register lists its own account's postings: read both.
	registers := []map[string]any{}
	for _, id := range []int64{starterID, f.cashAccount.ID} {
		res := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/register", id), nil, http.StatusOK)
		registers = append(registers, registerPostings(res.Body.Bytes())...)
	}
	assertStarterLabelParts(t, "register", registers, "account_id", "account_starter_key", "account_currency_code", starterID, f.cashAccount.ID)
	assertStarterLabelParts(t, "created transaction", postings(created.Body.Bytes()), "account_id", "account_starter_key", "account_currency_code", starterID, f.cashAccount.ID)
	var transaction struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &transaction))
	stored := doInvestmentRequest(t, handler, f.sessionCookie, "", http.MethodGet, fmt.Sprintf("/api/v1/transactions/%d", transaction.ID), nil, http.StatusOK)
	assertStarterLabelParts(t, "transaction read", postings(stored.Body.Bytes()), "account_id", "account_starter_key", "account_currency_code", starterID, f.cashAccount.ID)

	// A reconciliation confirmation names the checkpoint's account. It sent a
	// server-composed account_label, cash:USD for the starter account, which
	// every confirmation showed verbatim.
	var createdTransaction transactionResponse
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdTransaction))
	var starterPosting postingResponse
	for _, candidate := range createdTransaction.JournalEntries[0].Postings {
		if candidate.AccountID == starterID {
			starterPosting = candidate
		}
	}
	require.NotZero(t, starterPosting.ID)
	reconcilePostingForSession(t, handler, f.sessionCookie, f.csrfToken, starterID, f.commodityID, starterPosting, owner.OwnerToday)
	today, err := time.Parse(time.DateOnly, owner.OwnerToday)
	require.NoError(t, err)
	backdated := today.AddDate(0, 0, -1).Format(time.DateOnly)
	impact := doInvestmentRequest(t, handler, f.sessionCookie, f.csrfToken, http.MethodPost, "/api/v1/transactions/reconciliation-impact", map[string]any{
		"status": "posted", "transaction_date": backdated,
		"journal_entries": []map[string]any{{"entry_date": backdated, "postings": []map[string]any{
			{"account_id": f.cashAccount.ID, "commodity_id": f.commodityID, "quantity_value": "-100", "quantity_scale": 2},
			{"account_id": starterID, "commodity_id": f.commodityID, "quantity_value": "100", "quantity_scale": 2},
		}}},
	}, http.StatusOK)
	checkpoints := rowsAt("affected_checkpoints")(impact.Body.Bytes())
	require.Len(t, checkpoints, 1, "the backdated move falls inside the starter account's checkpoint")
	checkpoint := checkpoints[0]
	assert.Equal(t, float64(starterID), checkpoint["account_id"])
	assert.NotContains(t, checkpoint, "account_label", "no server-composed label")
	assert.Equal(t, "cash", checkpoint["account_starter_key"])
	assert.Equal(t, "USD", checkpoint["account_currency_code"])
	assert.Equal(t, "cash:USD", checkpoint["account_code"])
	for _, key := range []string{"account_name", "account_system_role", "account_builtin_key"} {
		assert.Contains(t, checkpoint, key)
		assert.Nil(t, checkpoint[key], "checkpoint %s for the unnamed starter account", key)
	}
}

func assertStarterLabelParts(t *testing.T, read string, rows []map[string]any, idKey, starterKey, currencyKey string, starterID, namedID int64) {
	t.Helper()
	found := map[int64]map[string]any{}
	for _, row := range rows {
		if id, ok := row[idKey].(float64); ok && (int64(id) == starterID || int64(id) == namedID) {
			found[int64(id)] = row
		}
	}
	starter, named := found[starterID], found[namedID]
	require.NotNil(t, starter, "%s lists the starter cash account", read)
	require.NotNil(t, named, "%s lists the named account", read)
	assert.Equal(t, "cash", starter[starterKey], "%s starter key", read)
	assert.Equal(t, "USD", starter[currencyKey], "%s starter currency", read)
	for _, key := range []string{starterKey, currencyKey} {
		assert.Contains(t, named, key, "%s sends %s", read, key)
		assert.Nil(t, named[key], "%s %s for a named account", read, key)
	}
}

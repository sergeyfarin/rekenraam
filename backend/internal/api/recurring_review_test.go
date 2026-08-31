package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecurringReviewAPIReadsSkipAndSecurity(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	input := recurringAPIInput(t, handler, cookie, csrf, commodityID)
	template := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/recurring/templates", recurringJSON(t, input), http.StatusCreated))
	require.NotNil(t, template.NextDueOn)
	date := *template.NextDueOn
	path := "/api/v1/recurring/templates/" + strconvFormatInt(template.ID)
	body := `{"occurrence_date":"` + date + `","reason":"Skip this month"}`
	var before, after int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	recurringAPIRequest(t, handler, nil, "", http.MethodGet, "/api/v1/recurring/due", "", http.StatusUnauthorized)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/due?limit=201", "", http.StatusBadRequest)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/due?cursor=bad", "", http.StatusBadRequest)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/occurrences?from=0000-01-01&to="+date, "", http.StatusBadRequest)
	recurringAPIRequest(t, handler, cookie, "", http.MethodPost, path+"/skip", body, http.StatusForbidden)
	recurringAPIRequest(t, handler, cookie, "", http.MethodPost, path+"/retry", `{"occurrence_date":"`+date+`"}`, http.StatusForbidden)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, path+"/skip", `{"occurrence_date":"`+date+`","reason":""}`, http.StatusBadRequest)
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/occurrences?from="+date+"&to="+date, "", http.StatusOK)
	var planned recurringOccurrencesResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &planned))
	require.Len(t, planned.Occurrences, 1)
	assert.Nil(t, planned.Occurrences[0].ID)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, path+"/skip", body, http.StatusNoContent)
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/occurrences?from="+date+"&to="+date, "", http.StatusOK)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &planned))
	require.Len(t, planned.Occurrences, 1)
	assert.Equal(t, "skipped", planned.Occurrences[0].Status)
	assert.Equal(t, "Skip this month", planned.Occurrences[0].SkipReason)
	updated := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path, "", http.StatusOK))
	require.NotNil(t, updated.NextDueOn)
	assert.Greater(t, *updated.NextDueOn, date)
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, path+"/retry", `{"occurrence_date":"`+date+`"}`, http.StatusConflict)
	assert.Contains(t, res.Body.String(), "RECURRING_OCCURRENCE_ALREADY_MATERIALIZED")
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, path+"/run-now", `{}`, http.StatusNotFound)
}

func TestRecurringDueAPIReflectsDraftEditsAndPosting(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Due checking", "asset", "checking", commodityID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Due expense","category_type":"expense"}`)
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-06-07", account.ID, category.ID, commodityID, 10000)
	// An incomplete edit remains visible and must not be misrepresented as balanced.
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(draft.ID), balancedBody("2026-06-07", posting(account.ID, -9007199254740993, 2, commodityID), posting(category.ID, 10000, 2, commodityID)), http.StatusOK)
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/due", "", http.StatusOK)
	var due recurringDueResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &due))
	require.Len(t, due.Items, 1)
	require.Len(t, due.Items[0].Amounts, 1)
	assert.Equal(t, draft.ID, *due.Items[0].TransactionID)
	assert.Equal(t, "-9007199254740993", due.Items[0].Amounts[0].CreditValue.String())
	assert.Equal(t, "10000", due.Items[0].Amounts[0].DebitValue.String())
	path := "/api/v1/transactions/" + strconvFormatInt(draft.ID)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/post/reconciliation-impact", "", http.StatusBadRequest)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, path, balancedBody("2026-06-07", posting(account.ID, -10000, 2, commodityID), posting(category.ID, 10000, 2, commodityID)), http.StatusOK)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/post/reconciliation-impact", "", http.StatusOK)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, path+"/post", `{}`, http.StatusOK)
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/due", "", http.StatusOK)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &due))
	assert.Empty(t, due.Items)
}

func TestRecurringPostPreviewUsesStoredSameDayPositionsAndNamesCheckpoints(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Same day checking", "asset", "checking", commodityID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Same day expense","category_type":"expense"}`)
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-06-08", account.ID, category.ID, commodityID, 5000)
	posted := createTransactionForSession(t, handler, cookie, csrf, balancedBody("2026-06-08", posting(account.ID, -10000, 2, commodityID), posting(category.ID, 10000, 2, commodityID)), http.StatusCreated)
	reconcilePostingForSession(t, handler, cookie, csrf, account.ID, commodityID, posted.JournalEntries[0].Postings[0], "2026-06-08")
	before := listReconciliationCheckpointsForSession(t, handler, cookie, account.ID, "")
	path := "/api/v1/transactions/" + strconvFormatInt(draft.ID)
	var beforeAudit, afterAudit int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&beforeAudit))
	// The edit preview stays empty: promotion must use its own stored-position check.
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, path+"/reconciliation-impact", balancedBody("2026-06-08", posting(account.ID, -5000, 2, commodityID), posting(category.ID, 5000, 2, commodityID)), http.StatusOK)
	var impact reconciliationImpactResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &impact))
	assert.Empty(t, impact.AffectedCheckpoints)
	recurringAPIRequest(t, handler, nil, "", http.MethodGet, path+"/post/reconciliation-impact", "", http.StatusUnauthorized)
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/post/reconciliation-impact", "", http.StatusOK)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &impact))
	require.NotEmpty(t, impact.AffectedCheckpoints)
	assert.Equal(t, "Same day checking", impact.AffectedCheckpoints[0].AccountLabel)
	assert.NotEmpty(t, impact.AffectedCheckpoints[0].CommodityCode)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&afterAudit))
	assert.Equal(t, beforeAudit, afterAudit)
	assert.Equal(t, before, listReconciliationCheckpointsForSession(t, handler, cookie, account.ID, ""))
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, path+"/post", `{}`, http.StatusConflict)
	promoted := mutateTransaction(t, handler, cookie, csrf, http.MethodPost, path+"/post", `{"reconciliation_override":true}`, http.StatusOK)
	ids := make([]int64, 0)
	for _, cp := range impact.AffectedCheckpoints {
		ids = append(ids, cp.CheckpointID)
	}
	assert.ElementsMatch(t, ids, promoted.InvalidatedCheckpointIDs)
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path+"/post/reconciliation-impact", "", http.StatusOK)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &impact))
	assert.Empty(t, impact.AffectedCheckpoints)
}

func TestDiscardEditedGeneratedDraftPreservesAuditAndSkippedIdentity(t *testing.T) {
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	account := createLedgerAccount(t, handler, cookie, csrf, "Edited draft checking", "asset", "checking", commodityID, 2)
	category := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Edited draft expense","category_type":"expense"}`)
	draft := generateRecurringDraft(t, handler, database, cookie, "2026-06-07", account.ID, category.ID, commodityID, 5000)
	path := "/api/v1/transactions/" + strconvFormatInt(draft.ID)
	for _, amount := range []int64{6000, 7000} {
		mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, path, balancedBody("2026-06-07", posting(account.ID, -amount, 2, commodityID), posting(category.ID, amount, 2, commodityID)), http.StatusOK)
	}
	var before, after int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	mutateTransactionNoBody(t, handler, cookie, csrf, http.MethodDelete, path, http.StatusNoContent)
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before+1, after)
	for _, table := range []string{"transactions", "transaction_versions", "journal_entries", "posting_versions"} {
		var count int
		require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		assert.Zero(t, count, table)
	}
	var status string
	require.NoError(t, database.QueryRow(`SELECT status FROM recurring_occurrences`).Scan(&status))
	assert.Equal(t, "skipped", status)
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/due", "", http.StatusOK)
	var page recurringDueResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &page))
	assert.Empty(t, page.Items)
	rows, err := database.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	defer rows.Close()
	assert.False(t, rows.Next())
	require.NoError(t, rows.Err())
}

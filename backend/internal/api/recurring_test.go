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

func recurringAPIRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfTokenHeader, csrf)
		setSameOrigin(req)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, status, res.Code, res.Body.String())
	return res
}

func recurringAPIInput(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf string, commodityID int64) map[string]any {
	t.Helper()
	checking := createLedgerAccount(t, handler, cookie, csrf, "Recurring checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Recurring rent","category_type":"expense"}`)
	return map[string]any{"name": "Rent", "frequency": "monthly", "day_of_month": 1, "starts_on": "2026-01-01",
		"description": "Keep description", "note_markdown": "Keep note", "lead_days": 7,
		"postings": []map[string]any{
			{"account_id": checking.ID, "commodity_id": commodityID, "quantity_value": "-9007199254740993", "quantity_scale": 2},
			{"account_id": expense.ID, "commodity_id": commodityID, "quantity_value": "9007199254740993", "quantity_scale": 2},
		}}
}

func recurringJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
func recurringResponse(t *testing.T, res *httptest.ResponseRecorder) recurringTemplateResponse {
	t.Helper()
	var result recurringTemplateResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	return result
}

func TestRecurringTemplateAPICreatePatchReadArchive(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	input := recurringAPIInput(t, handler, cookie, csrf, commodityID)
	payee := createPayeeForSession(t, handler, cookie, csrf, `{"name":"Example landlord"}`)
	input["payee_id"] = payee.ID
	tag := createTagForSession(t, handler, cookie, csrf, `{"name":"Recurring test","kind":"custom"}`)
	input["tag_ids"] = []int64{tag.ID}
	input["ends_on"], input["max_occurrences"] = "2030-12-31", 100
	created := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/recurring/templates", recurringJSON(t, input), http.StatusCreated))
	assert.True(t, created.Enabled)
	assert.Equal(t, "ordinary", created.TransactionKind)
	assert.Equal(t, "Example landlord", created.PayeeName)
	require.Len(t, created.Postings, 2)
	assert.Equal(t, "-9007199254740993", created.Postings[0].QuantityValue.String())
	assert.NotEmpty(t, created.Postings[0].LineKey)
	path := "/api/v1/recurring/templates/" + strconvFormatInt(created.ID)
	updated := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPatch, path, `{"name":"Updated rent"}`, http.StatusOK))
	assert.Equal(t, created.Postings, updated.Postings)
	assert.Equal(t, created.Enabled, updated.Enabled)
	assert.Equal(t, created.LeadDays, updated.LeadDays)
	assert.Equal(t, created.PayeeID, updated.PayeeID)
	assert.Equal(t, []int64{tag.ID}, updated.TagIDs)
	assert.Equal(t, created.EndsOn, updated.EndsOn)
	assert.Equal(t, created.GenerateFrom, updated.GenerateFrom)
	assert.Equal(t, created.Revision+1, updated.Revision)
	read := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, path, "", http.StatusOK))
	assert.Equal(t, updated, read)
	cleared := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPatch, path,
		`{"enabled":false,"lead_days":0,"payee_id":null,"ends_on":null,"max_occurrences":null,"tag_ids":[],"description":"","note_markdown":""}`, http.StatusOK))
	assert.False(t, cleared.Enabled)
	assert.Zero(t, cleared.LeadDays)
	assert.Nil(t, cleared.PayeeID)
	assert.Empty(t, cleared.PayeeName)
	assert.Nil(t, cleared.EndsOn)
	assert.Nil(t, cleared.MaxOccurrences)
	assert.Nil(t, cleared.NextDueOn)
	assert.Empty(t, cleared.Description)
	assert.Empty(t, cleared.NoteMarkdown)
	assert.NotNil(t, cleared.TagIDs)
	assert.Empty(t, cleared.TagIDs)
	linked := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPatch, path, `{"payee_name":"example landlord"}`, http.StatusOK))
	assert.Equal(t, &payee.ID, linked.PayeeID, "known names resolve through the shared matcher")
	archived := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, path+"/archive", `{}`, http.StatusOK))
	require.NotNil(t, archived.ArchivedAt)
	refused := recurringAPIRequest(t, handler, cookie, csrf, http.MethodPatch, path, `{"name":"No"}`, http.StatusConflict)
	assert.Contains(t, refused.Body.String(), "RECURRING_TEMPLATE_ARCHIVED")
	var list recurringTemplatesResponse
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/templates", "", http.StatusOK)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &list))
	assert.Empty(t, list.Templates)
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/templates?include_archived=true", "", http.StatusOK)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &list))
	require.Len(t, list.Templates, 1)
	var transactionCount int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&transactionCount))
	assert.Zero(t, transactionCount, "template CRUD must not create ledger rows")
}

func TestRecurringTemplateAPIPatchClearsObsoleteScheduleFields(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	created := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/recurring/templates", recurringJSON(t, recurringAPIInput(t, handler, cookie, csrf, commodityID)), http.StatusCreated))
	path := "/api/v1/recurring/templates/" + strconvFormatInt(created.ID)
	bad := recurringAPIRequest(t, handler, cookie, csrf, http.MethodPatch, path, `{"frequency":"weekly","by_weekday":0}`, http.StatusBadRequest)
	assert.Contains(t, bad.Body.String(), "RECURRING_SCHEDULE_INVALID")
	updated := recurringResponse(t, recurringAPIRequest(t, handler, cookie, csrf, http.MethodPatch, path, `{"frequency":"weekly","by_weekday":0,"day_of_month":null}`, http.StatusOK))
	assert.Nil(t, updated.DayOfMonth)
	require.NotNil(t, updated.ByWeekday)
	assert.Zero(t, *updated.ByWeekday, "Sunday must not be treated as omission")
}

func TestRecurringTemplateAPIRejectsUnbalancedAndInvalidInputWithoutWrites(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, commodityID := setupAccountAPITest(t, handler)
	input := recurringAPIInput(t, handler, cookie, csrf, commodityID)
	input["postings"].([]map[string]any)[0]["quantity_value"] = "-1"
	res := recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/recurring/templates", recurringJSON(t, input), http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "RECURRING_TEMPLATE_UNBALANCED")
	input["frequency"] = "nonsense"
	res = recurringAPIRequest(t, handler, cookie, csrf, http.MethodPost, "/api/v1/recurring/templates", recurringJSON(t, input), http.StatusBadRequest)
	assert.Contains(t, res.Body.String(), "RECURRING_SCHEDULE_INVALID")
	var count int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM recurring_templates`).Scan(&count))
	assert.Zero(t, count)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/templates/999999", "", http.StatusNotFound)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/templates/invalid", "", http.StatusBadRequest)
	recurringAPIRequest(t, handler, cookie, csrf, http.MethodGet, "/api/v1/recurring/templates?include_archived=invalid", "", http.StatusBadRequest)
}

func TestRecurringTemplateAPIRequiresAuthenticationAndCSRF(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, _, _ := setupAccountAPITest(t, handler)
	recurringAPIRequest(t, handler, nil, "", http.MethodGet, "/api/v1/recurring/templates", "", http.StatusUnauthorized)
	recurringAPIRequest(t, handler, nil, "", http.MethodGet, "/api/v1/recurring/templates/1", "", http.StatusUnauthorized)
	for _, path := range []string{"/api/v1/recurring/templates", "/api/v1/recurring/templates/1", "/api/v1/recurring/templates/1/archive"} {
		method := http.MethodPost
		if path == "/api/v1/recurring/templates/1" {
			method = http.MethodPatch
		}
		recurringAPIRequest(t, handler, cookie, "", method, path, `{}`, http.StatusForbidden)
	}
}

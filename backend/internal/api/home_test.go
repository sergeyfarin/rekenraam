package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomeReadAuthenticationShapeAndPurity(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/home", nil))
	require.Equal(t, http.StatusUnauthorized, res.Code)

	cookie, _, _ := setupAccountAPITest(t, handler)
	var before, after int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&before))
	res = doInvestmentRequest(t, handler, cookie, "", http.MethodGet, "/api/v1/home", nil, http.StatusOK)
	assert.Equal(t, "no-store", res.Header().Get("Cache-Control"))
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&after))
	assert.Equal(t, before, after, "reading Home writes nothing")

	var raw map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &raw))
	for _, field := range []string{"as_of", "start_boundary", "reporting_currency", "empty_book", "headlines", "widgets", "commodities"} {
		assert.Contains(t, raw, field)
	}
	var result homeResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.Len(t, result.Headlines, 2)
	assert.Equal(t, "net_worth", result.Headlines[0].ID)
	assert.Equal(t, "cash", result.Headlines[1].ID)
	for _, headline := range raw["headlines"].([]any) {
		item := headline.(map[string]any)
		for _, field := range []string{"value", "start_value", "change", "unavailable", "missing", "stale"} {
			assert.Contains(t, item, field, "null and empty are sent, never omitted")
		}
	}
	widgets := raw["widgets"].(map[string]any)
	for widget, fields := range map[string][]string{
		"spending":        {"start_date", "end_date", "total", "commodity_totals", "categories", "category_count"},
		"budgets":         {"period_start", "period_end", "commodity_id", "other_commodity_count", "total", "categories", "category_count"},
		"accounts":        {"as_of", "open_count", "accounts"},
		"recurring":       {"today", "active_count", "draft_count", "blocked_count", "upcoming"},
		"net_worth_trend": {"start_date", "end_date", "points", "change"},
	} {
		item := widgets[widget].(map[string]any)
		assert.Equal(t, "ok", item["status"], widget)
		summary, ok := item["summary"].(map[string]any)
		require.True(t, ok, "%s has a summary", widget)
		for _, field := range fields {
			assert.Contains(t, summary, field, "%s.%s is sent, never omitted", widget, field)
		}
	}
}

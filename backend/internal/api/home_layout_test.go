package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/app"
)

func TestHomeLayoutTwoTabsCannotOverwriteAndResetAdvancesRevision(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	const path = "/api/v1/home/layout"
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	cookie, csrf, _ := setupAccountAPITest(t, handler)
	read := func() app.HomeLayoutState {
		t.Helper()
		response := doInvestmentRequest(t, handler, cookie, "", http.MethodGet, path, nil, http.StatusOK)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		var state app.HomeLayoutState
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &state))
		return state
	}
	write := func(revision int64, layout *app.HomeLayout, status int) *httptest.ResponseRecorder {
		t.Helper()
		return doInvestmentRequest(t, handler, cookie, csrf, http.MethodPut, path,
			map[string]any{"expected_revision": revision, "layout": layout}, status)
	}

	var auditBefore, auditAfter, count int
	require.NoError(t, database.QueryRow("SELECT count(*) FROM audit_events").Scan(&auditBefore))
	tabOne, tabTwo := read(), read()
	require.Zero(t, tabOne.Revision)
	require.Equal(t, tabOne, tabTwo)
	require.Equal(t, app.DefaultHomeLayout(), tabOne.Layout)
	require.NoError(t, database.QueryRow("SELECT count(*) FROM home_layouts").Scan(&count))
	assert.Zero(t, count, "reading the virtual default writes no row")

	tabOne.Layout.Headline = app.HomeHeadlineCash
	tabOne.Layout.Widgets = []string{"cashflow", "spending"}
	write(tabOne.Revision, &tabOne.Layout, http.StatusOK)
	tabTwo.Layout.ExploreOpen = false
	stale := write(tabTwo.Revision, &tabTwo.Layout, http.StatusConflict)
	assert.Contains(t, stale.Body.String(), `"code":"CONFLICT"`)
	latest := read()
	assert.Equal(t, int64(1), latest.Revision)
	assert.Equal(t, tabOne.Layout, latest.Layout)

	reset := write(latest.Revision, nil, http.StatusOK)
	var result app.HomeLayoutState
	require.NoError(t, json.Unmarshal(reset.Body.Bytes(), &result))
	assert.Equal(t, int64(2), result.Revision)
	assert.Equal(t, latest.Defaults, result.Layout)
	write(latest.Revision, &latest.Layout, http.StatusConflict)
	assert.Equal(t, result, read())
	require.NoError(t, database.QueryRow("SELECT count(*) FROM audit_events").Scan(&auditAfter))
	assert.Equal(t, auditBefore, auditAfter, "customization writes only Home layout")
}

func TestHomeLayoutRejectsIncompleteDocumentsAndInvalidOptions(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	const path = "/api/v1/home/layout"
	for _, test := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing boolean", func(document map[string]any) { delete(document, "show_review") }},
		{"unknown field", func(document map[string]any) { document["extra"] = true }},
		{"future version", func(document map[string]any) { document["version"] = 99 }},
		{"missing new transaction", func(document map[string]any) { document["shortcuts"] = []app.HomeLayoutShortcut{{ID: "transfer"}} }},
		{"duplicate widget", func(document map[string]any) { document["widgets"] = []string{"budgets", "budgets"} }},
		{"unsupported size", func(document map[string]any) {
			document["widget_options"] = map[string]any{"budgets": map[string]string{"size": "wide"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			content, err := json.Marshal(app.DefaultHomeLayout())
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(content, &document))
			test.edit(document)
			doInvestmentRequest(t, handler, cookie, csrf, http.MethodPut, path,
				map[string]any{"expected_revision": 0, "layout": document}, http.StatusBadRequest)
		})
	}
	doInvestmentRequest(t, handler, cookie, csrf, http.MethodPut, path, map[string]any{"expected_revision": 0}, http.StatusBadRequest)
	doInvestmentRequest(t, handler, cookie, csrf, http.MethodPut, path, map[string]any{"layout": nil}, http.StatusBadRequest)
	doInvestmentRequest(t, handler, cookie, "", http.MethodPut, path, map[string]any{"expected_revision": 0, "layout": nil}, http.StatusForbidden)
	doInvestmentRequest(t, handler, cookie, csrf, http.MethodPut, path, map[string]any{"expected_revision": 0, "layout": nil}, http.StatusOK)
}

func TestHomeLayoutNormalizesRetiredKeysOnWrite(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	layout := app.DefaultHomeLayout()
	layout.Shortcuts = append(layout.Shortcuts, app.HomeLayoutShortcut{ID: "retired_route"})
	layout.Widgets = []string{"retired_widget"}
	layout.WidgetOptions["retired_widget"] = app.HomeWidgetOptions{Size: "large"}
	response := doInvestmentRequest(t, handler, cookie, csrf, http.MethodPut, "/api/v1/home/layout",
		map[string]any{"expected_revision": 0, "layout": layout}, http.StatusOK)
	var state app.HomeLayoutState
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &state))
	assert.Equal(t, app.DefaultHomeLayout(), state.Layout)
}

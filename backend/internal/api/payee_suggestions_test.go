package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayeeSuggestionsSearchPastTheLegacyListLimit(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	_, err := database.Exec(`
		WITH RECURSIVE numbers(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM numbers WHERE n < 1999)
		INSERT INTO payees (id, book_id, created_at, created_by_user_id)
		SELECT n + 1, books.id, '2020-01-01T00:00:00Z', books.owner_user_id FROM numbers CROSS JOIN books;
		INSERT INTO payee_versions (
			payee_id, version_seq, effective_from, recorded_at, changed_by_user_id, change_reason, status, name, normalized_name
		)
		SELECT id, 1, '2020-01-01', created_at, created_by_user_id, 'seed', 'active',
			printf('Vendor %04d', id - 1), printf('vendor %04d', id - 1)
		FROM payees
	`)
	require.NoError(t, err)
	target := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Vendor 2000"}`)

	legacy := listPayeesForSession(t, handler, sessionCookie, "")
	require.Len(t, legacy.Payees, 2000)
	assert.Equal(t, "Vendor 1999", legacy.Payees[1999].Name)

	type suggestion struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Exact bool   `json:"exact"`
	}
	get := func(query string, wantStatus int) []suggestion {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/payees/suggestions?q="+url.QueryEscape(query), nil)
		req.AddCookie(sessionCookie)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, wantStatus, res.Code, res.Body.String())
		var result struct {
			Payees []suggestion `json:"payees"`
		}
		if wantStatus == http.StatusOK {
			require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
			require.NotNil(t, result.Payees)
			require.LessOrEqual(t, len(result.Payees), 5)
		}
		return result.Payees
	}

	exact := get("  VENDOR   2000 ", http.StatusOK)
	require.NotEmpty(t, exact)
	assert.Equal(t, suggestion{target.ID, target.Name, true}, exact[0])
	for _, query := range []string{"Vendr 2000", "2000 Vendor", "Vendor 200"} {
		near := get(query, http.StatusOK)
		assert.Contains(t, near, suggestion{target.ID, target.Name, false}, query)
	}
	assert.Empty(t, get("ZZZZZZZZZZ", http.StatusOK))
	get(" ", http.StatusBadRequest)
	get(strings.Repeat("x", 201), http.StatusBadRequest)

	mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(target.ID)+"/archive", `{}`, http.StatusOK)
	for _, candidate := range get(target.Name, http.StatusOK) {
		assert.NotEqual(t, target.ID, candidate.ID)
		assert.False(t, candidate.Exact)
	}
	assert.Equal(t, "archived", readPayeeForSession(t, handler, sessionCookie, target.ID, http.StatusOK).Status)
}

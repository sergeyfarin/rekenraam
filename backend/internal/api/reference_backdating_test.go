package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
)

func TestReferenceBackdatedLifecycleReturnsCodedConflict(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"category", "payee", "institution"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			handler, database := newSetupTestHandler(t)
			cookie, csrf, _ := setupAccountAPITest(t, handler)
			var path, archive, restore string
			switch kind {
			case "category":
				v := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Backdated category","category_type":"expense"}`)
				path, archive, restore = fmt.Sprintf("/api/v1/categories/%d", v.ID), "disable", "restore"
			case "payee":
				v := createPayeeForSession(t, handler, cookie, csrf, `{"name":"Backdated payee","effective_from":"2024-01-01"}`)
				path, archive, restore = fmt.Sprintf("/api/v1/payees/%d", v.ID), "archive", "restore"
			case "institution":
				v := createInstitutionForSession(t, handler, cookie, csrf, `{"name":"Backdated institution","kind":"bank","effective_from":"2024-01-01"}`)
				path, archive, restore = fmt.Sprintf("/api/v1/institutions/%d", v.ID), "archive", "restore"
			}
			mutate := func(method, url, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, url, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set(csrfTokenHeader, csrf)
				setSameOrigin(req)
				req.AddCookie(cookie)
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				return res
			}
			if kind != "category" {
				body := `{"name":"Before existence","kind":"bank","effective_from":"2023-01-01"}`
				if kind == "payee" {
					body = `{"name":"Before existence","effective_from":"2023-01-01"}`
				}
				res := mutate(http.MethodPatch, path, body)
				require.Equal(t, http.StatusConflict, res.Code, res.Body.String())
				assert.Contains(t, res.Body.String(), `"code":"REFERENCE_VERSION_BACKDATED"`)
			}
			res := mutate(http.MethodPost, path+"/"+archive, `{"effective_from":"2025-06-01"}`)
			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
			for _, action := range []string{archive, restore} {
				var before, after int
				require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&before))
				res := mutate(http.MethodPost, path+"/"+action, `{"effective_from":"2025-03-01"}`)
				require.Equal(t, http.StatusConflict, res.Code, res.Body.String())
				assert.JSONEq(t, `{"error":{"code":"REFERENCE_VERSION_BACKDATED","message":"reference version is backdated: reference lifecycle or structural changes must take effect on or after 2025-06-01","conflict_date":"2025-06-01"}}`, res.Body.String())
				require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM audit_events").Scan(&after))
				assert.Equal(t, before, after)
			}
			if kind == "category" {
				res := mutate(http.MethodPatch, path, `{"allows_postings":false,"effective_from":"2025-03-01"}`)
				require.Equal(t, http.StatusConflict, res.Code, res.Body.String())
				assert.Contains(t, res.Body.String(), `"code":"REFERENCE_VERSION_BACKDATED"`)
			}
			res = mutate(http.MethodPost, path+"/"+restore, `{"effective_from":"2025-06-01"}`)
			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
			assert.Contains(t, res.Body.String(), `"status":"active"`)
		})
	}
}

// A stale base version (a version added between the service's read and its
// write transaction) is a retryable 409 CONFLICT, distinct from the coded
// backdating refusal.
func TestReferenceStaleBaseVersionReturnsConflict(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.DiscardHandler)
	for name, write := range map[string]func(http.ResponseWriter, *http.Request, *slog.Logger, string, error){
		"category":    writeCategoryServiceError,
		"payee":       writePayeeServiceError,
		"institution": writeInstitutionServiceError,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := httptest.NewRecorder()
			write(res, httptest.NewRequest(http.MethodPatch, "/", nil), logger, "update", app.ErrReferenceVersionChanged)
			require.Equal(t, http.StatusConflict, res.Code)
			assert.Contains(t, res.Body.String(), `"code":"CONFLICT"`)
		})
	}
}

// A dated edit refused because of a day between its date and today names the
// rule, the day and the other record, so the editor can explain a cause that
// today's category list does not show (#552).
func TestReferenceDatedConflictNamesTheRuleDateAndRecord(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	mine := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Mine","category_type":"expense"}`)
	other := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Other","category_type":"expense"}`)
	otherPath := fmt.Sprintf("/api/v1/categories/%d", other.ID)
	for _, body := range []string{`{"name":"Taken","effective_from":"2025-09-01"}`, `{"name":"Other again","effective_from":"2025-10-01"}`} {
		res := referenceDataRequest(handler, cookie, csrf, http.MethodPatch, otherPath, body)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	}
	res := referenceDataRequest(handler, cookie, csrf, http.MethodPatch, fmt.Sprintf("/api/v1/categories/%d", mine.ID), `{"name":"Taken","effective_from":"2025-08-01"}`)
	require.Equal(t, http.StatusConflict, res.Code, res.Body.String())
	assert.JSONEq(t, `{"error":{"code":"REFERENCE_DATED_CONFLICT","message":"category already exists on 2025-09-01","conflict_rule":"category_name_taken","conflict_date":"2025-09-01","conflict_name":"Taken"}}`, res.Body.String())
}

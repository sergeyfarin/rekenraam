package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOversizedJSONRequestUsesDocumentedError(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner",
		strings.NewReader(`{"username":"`+strings.Repeat("x", 1<<20)+`","password":"test-password"}`))
	request.Header.Set("Content-Type", "application/json")
	setSameOrigin(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	var body errorResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "REQUEST_TOO_LARGE", body.Error.Code)
	assert.Equal(t, "request body must not exceed 1 MB", body.Error.Message)
	spec, err := os.ReadFile("../../../api/openapi/components/schemas/errors.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(spec), "        - "+body.Error.Code+"\n", "the real refusal must be representable by the generated client")
	var owners int
	require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM users").Scan(&owners))
	assert.Zero(t, owners, "an oversized body cannot create an owner")
}

// Every mapped operation is exercised through the real router, authentication,
// CSRF/Origin checks and decoder. The oversized whitespace prefix is valid JSON
// whitespace, so it cannot fail early on a particular request's field schema.
func TestCappedJSONOperationRefusals(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	connectionHandler, connectionDatabase := newImportConnectionsTestHandler(t, nil)
	connectionCookie, connectionCSRF, _ := setupAccountAPITest(t, connectionHandler)
	for pattern := range cappedJSONOperations(t) {
		t.Run(pattern, func(t *testing.T) {
			parts := strings.SplitN(pattern, " ", 2)
			handler, database, cookie, csrf := handler, database, cookie, csrf
			if strings.HasPrefix(parts[1], "/api/v1/import-connections") {
				handler, database, cookie, csrf = connectionHandler, connectionDatabase, connectionCookie, connectionCSRF
			}
			path := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(parts[1], "1")
			for _, unknownLength := range []bool{false, true} {
				request := httptest.NewRequest(parts[0], path, strings.NewReader(strings.Repeat(" ", (1<<20)+1)+`{}`))
				if unknownLength {
					request.ContentLength = -1
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(csrfTokenHeader, csrf)
				request.AddCookie(cookie)
				setSameOrigin(request)
				var before, after int64
				require.NoError(t, database.QueryRow("SELECT total_changes()").Scan(&before))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, response.Body.String())
				assert.JSONEq(t, `{"error":{"code":"REQUEST_TOO_LARGE","message":"request body must not exceed 1 MB"}}`, response.Body.String())
				require.NoError(t, database.QueryRow("SELECT total_changes()").Scan(&after))
				assert.Equal(t, before, after, "refusal must perform no SQLite writes (including policy, auth, audit or financial rows)")
			}
		})
	}
}

func TestJSONBodyLimitTrailingWhitespace(t *testing.T) {
	t.Parallel()
	for _, size := range []int{1 << 20, (1 << 20) + 1} {
		for _, unknownLength := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/unknown_length_%t", size, unknownLength), func(t *testing.T) {
				handler, database := newSetupTestHandler(t)
				body := `{"username":"limit-control","password":"test-password"}`
				body += strings.Repeat(" ", size-len(body))
				request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				if unknownLength {
					request.ContentLength = -1
				}
				setSameOrigin(request)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				var owners int
				require.NoError(t, database.QueryRow("SELECT COUNT(*) FROM users").Scan(&owners))
				if size == 1<<20 {
					require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
					assert.Equal(t, 1, owners)
				} else {
					require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, response.Body.String())
					assert.Contains(t, response.Body.String(), `"code":"REQUEST_TOO_LARGE"`)
					assert.Zero(t, owners)
				}
			})
		}
	}
}

func TestBackupPolicyStrictJSONRefusals(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	for _, scenario := range []struct {
		name, body, contentType string
		status                  int
	}{
		{"oversized", `{"enabled":true}` + strings.Repeat(" ", 1<<20), "application/json", http.StatusRequestEntityTooLarge},
		{"unknown_field", `{"enabled":true,"invented":1}`, "application/json", http.StatusBadRequest},
		{"multiple_objects", `{"enabled":true} {}`, "application/json", http.StatusBadRequest},
		{"wrong_type", `{"enabled":true}`, "text/plain", http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := requestBodyBackupPolicy(t, handler, cookie)
			var changesBefore, changesAfter int64
			require.NoError(t, database.QueryRow("SELECT total_changes()").Scan(&changesBefore))
			request := httptest.NewRequest(http.MethodPut, "/api/v1/maintenance/backups/policy", strings.NewReader(scenario.body))
			request.Header.Set("Content-Type", scenario.contentType)
			request.Header.Set(csrfTokenHeader, csrf)
			request.AddCookie(cookie)
			setSameOrigin(request)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, scenario.status, response.Code, response.Body.String())
			require.NoError(t, database.QueryRow("SELECT total_changes()").Scan(&changesAfter))
			assert.Equal(t, changesBefore, changesAfter)
			assert.JSONEq(t, before, requestBodyBackupPolicy(t, handler, cookie))
		})
	}
}

func requestBodyBackupPolicy(t *testing.T, handler http.Handler, cookie *http.Cookie) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/backups", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	return response.Body.String()
}

func TestOptionalJSONBodyLimitBoundary(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	for _, size := range []int{0, 1 << 20, (1 << 20) + 1} {
		for _, unknownLength := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/unknown_length_%t", size, unknownLength), func(t *testing.T) {
				institution := createInstitutionForSession(t, handler, cookie, csrf, `{"name":"body boundary bank","kind":"bank"}`)
				body := ""
				if size > 0 {
					body = `{}` + strings.Repeat(" ", size-2)
				}
				request := httptest.NewRequest(http.MethodPost, "/api/v1/institutions/"+strconvFormatInt(institution.ID)+"/archive", strings.NewReader(body))
				if unknownLength {
					request.ContentLength = -1
				}
				if size > 0 {
					request.Header.Set("Content-Type", "application/json")
				}
				request.Header.Set(csrfTokenHeader, csrf)
				request.AddCookie(cookie)
				setSameOrigin(request)
				var before, after int64
				require.NoError(t, database.QueryRow("SELECT total_changes()").Scan(&before))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if size > 1<<20 {
					require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, response.Body.String())
					require.NoError(t, database.QueryRow("SELECT total_changes()").Scan(&after))
					assert.Equal(t, before, after)
				} else {
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				}
			})
		}
	}
}

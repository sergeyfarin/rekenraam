package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

// --- Bootstrap ---

// importConnectionsTestSecretKey is a fixed 32-byte key for secretbox in
// tests — production reads this from REKENRAAM_SECRET_KEY (base64), but the
// service takes raw bytes directly.
func importConnectionsTestSecretKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

// newImportConnectionsTestHandler wires the same stack as
// newSetupTestHandler plus a real ImportConnectionService (nil in that
// helper, so the import-connections routes aren't registered there — see
// health.go's `if services.ImportConnection != nil` guard). The prober is
// swappable so tests can simulate a rejected or failing provider key.
func newImportConnectionsTestHandler(t *testing.T, prober app.ConnectionProber) (http.Handler, *sql.DB) {
	t.Helper()

	database, err := db.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "rekenraam.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
	})
	require.NoError(t, db.Migrate(context.Background(), database))

	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	authRepository := db.NewAuthRepository(database)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService := app.NewAuthService(authRepository, logger)
	settingsService := app.NewSettingsService(db.NewSettingsRepository(database))
	bookService := app.NewBookService(db.NewBookRepository(database), setupService)
	commodityRepository := db.NewCommodityRepository(database)
	currencyService := app.NewCurrencyService(commodityRepository, setupService)
	currencyService.SetNowForTest(func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	institutionRepository := db.NewInstitutionRepository(database)
	institutionService := app.NewInstitutionService(institutionRepository)
	accountRepository := db.NewAccountRepository(database)
	accountService := app.NewAccountService(accountRepository, institutionRepository, setupService)
	tagService := app.NewTagService(db.NewTagRepository(database))
	categoryService := app.NewCategoryService(db.NewCategoryRepository(database), setupService)
	categoryService.SetNowForTest(func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	})
	payeeRepository := db.NewPayeeRepository(database)
	payeeService := app.NewPayeeService(payeeRepository, accountRepository)
	transactionService := app.NewTransactionService(db.NewTransactionRepository(database), payeeRepository, accountRepository, commodityRepository)
	pricingService := app.NewPricingService(db.NewPricingRepository(database))
	investmentService := app.NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, pricingService)
	connectionService := app.NewImportConnectionService(db.NewImportConnectionRepository(database), accountService, importConnectionsTestSecretKey(), prober)
	importService := app.NewImportService(db.NewImportRepository(database), transactionService, accountRepository, connectionService, db.NewBackgroundWorkRepository(database), investmentService)

	handler := NewHandler(logger, http.NotFoundHandler(), Services{
		Setup:            setupService,
		Auth:             authService,
		Settings:         settingsService,
		Book:             bookService,
		Currency:         currencyService,
		Institution:      institutionService,
		Account:          accountService,
		Tag:              tagService,
		Category:         categoryService,
		Payee:            payeeService,
		Transaction:      transactionService,
		Pricing:          pricingService,
		Investment:       investmentService,
		Import:           importService,
		ImportConnection: connectionService,
	}, HandlerOptions{})

	return handler, database
}

// --- Request helpers ---

func createImportConnectionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string, wantStatus int) importConnectionResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-connections", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response importConnectionResponse
	if wantStatus == http.StatusCreated {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listImportConnectionsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, wantStatus int) listImportConnectionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/import-connections", nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response listImportConnectionsResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func updateImportConnectionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, connID int64, body string, wantStatus int) (importConnectionResponse, *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/import-connections/"+strconv.FormatInt(connID, 10), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())

	var response importConnectionResponse
	if wantStatus == http.StatusOK {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response, res
}

func deleteImportConnectionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, connID int64, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/import-connections/"+strconv.FormatInt(connID, 10), nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

func refreshImportConnectionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, connID int64, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-connections/"+strconv.FormatInt(connID, 10)+"/refresh", nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	require.Equal(t, wantStatus, res.Code, res.Body.String())
	return res
}

// rejectingProber simulates a provider that always rejects the API key,
// exercising the PROVIDER_ERROR mapping.
type rejectingProber struct{}

func (rejectingProber) Probe(_ context.Context, _ string, _ string, _ string) error {
	return app.ErrProviderUnauthorized
}

// --- Lifecycle ---

func TestImportConnectionLifecycle_CreateListUpdateRotateDelete(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	created := createImportConnectionForSession(t, handler, sessionCookie, csrfToken, `{
		"source":"trading212",
		"display_name":"ISA",
		"api_key":"initial-secret-key-aaaa"
	}`, http.StatusCreated)
	assert.Equal(t, "trading212", created.SourceKind)
	assert.Equal(t, "ISA", created.DisplayName)
	assert.NotEmpty(t, created.KeyHint)
	assert.NotContains(t, created.KeyHint, "initial-secret-key-aaaa")

	list := listImportConnectionsForSession(t, handler, sessionCookie, http.StatusOK)
	require.Len(t, list.Connections, 1)
	assert.Equal(t, created.ID, list.Connections[0].ID)

	renamed, _ := updateImportConnectionForSession(t, handler, sessionCookie, csrfToken, created.ID, `{"display_name":"ISA Renamed"}`, http.StatusOK)
	assert.Equal(t, "ISA Renamed", renamed.DisplayName)
	assert.Equal(t, created.KeyHint, renamed.KeyHint, "renaming must not rotate the key")

	rotated, _ := updateImportConnectionForSession(t, handler, sessionCookie, csrfToken, created.ID, `{"display_name":"ISA Renamed","api_key":"rotated-secret-key-bbbb"}`, http.StatusOK)
	assert.NotEqual(t, created.KeyHint, rotated.KeyHint, "rotating the key must change the hint")
	assert.NotContains(t, rotated.KeyHint, "rotated-secret-key-bbbb")

	refreshImportConnectionForSession(t, handler, sessionCookie, csrfToken, created.ID, http.StatusAccepted)

	deleteImportConnectionForSession(t, handler, sessionCookie, csrfToken, created.ID, http.StatusNoContent)
	afterDelete := listImportConnectionsForSession(t, handler, sessionCookie, http.StatusOK)
	assert.Empty(t, afterDelete.Connections)
}

// --- Secret hygiene ---

// TestImportConnectionResponses_NeverExposeTheRawKey scans the raw response
// bytes (not decoded fields) for create/list/update, so a future field
// rename can't accidentally leak the plaintext key past this test.
func TestImportConnectionResponses_NeverExposeTheRawKey(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	const secretKey = "super-secret-trading212-key-xyz"

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/import-connections", strings.NewReader(`{
		"source":"trading212","display_name":"ISA","api_key":"`+secretKey+`"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(createReq)
	createReq.AddCookie(sessionCookie)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	require.Equal(t, http.StatusCreated, createRes.Code)
	assert.NotContains(t, createRes.Body.String(), secretKey)

	var created importConnectionResponse
	require.NoError(t, json.Unmarshal(createRes.Body.Bytes(), &created))

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/import-connections", nil)
	listReq.AddCookie(sessionCookie)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	require.Equal(t, http.StatusOK, listRes.Code)
	assert.NotContains(t, listRes.Body.String(), secretKey)

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/import-connections/"+strconv.FormatInt(created.ID, 10), strings.NewReader(`{"display_name":"ISA 2"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(updateReq)
	updateReq.AddCookie(sessionCookie)
	updateRes := httptest.NewRecorder()
	handler.ServeHTTP(updateRes, updateReq)
	require.Equal(t, http.StatusOK, updateRes.Code)
	assert.NotContains(t, updateRes.Body.String(), secretKey)
}

func TestCreateImportConnection_ProviderRejectionDoesNotLeakKeyInError(t *testing.T) {
	t.Parallel()

	handler, database := newImportConnectionsTestHandler(t, rejectingProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	const secretKey = "rejected-secret-key-value"

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-connections", strings.NewReader(`{
		"source":"trading212","display_name":"ISA","api_key":"`+secretKey+`"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadGateway, res.Code)
	assert.NotContains(t, res.Body.String(), secretKey)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "PROVIDER_ERROR", body.Error.Code)

	var count int
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM import_connections`).Scan(&count))
	assert.Equal(t, 0, count, "a rejected probe must not persist a connection")
}

// --- Error mapping ---

func TestCreateImportConnection_DuplicateNameReturnsConflict(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)

	body := `{"source":"trading212","display_name":"ISA","api_key":"first-secret-key-value"}`
	createImportConnectionForSession(t, handler, sessionCookie, csrfToken, body, http.StatusCreated)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import-connections", strings.NewReader(`{"source":"trading212","display_name":"ISA","api_key":"second-secret-key-value"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusConflict, res.Code)
	var errBody errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
	assert.Equal(t, "CONFLICT", errBody.Error.Code)
}

func TestImportConnectionEndpoints_UnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	const missingConnectionID = 999999

	updateImportConnectionForSession(t, handler, sessionCookie, csrfToken, missingConnectionID, `{"display_name":"whatever"}`, http.StatusNotFound)
	deleteImportConnectionForSession(t, handler, sessionCookie, csrfToken, missingConnectionID, http.StatusNotFound)
	refreshImportConnectionForSession(t, handler, sessionCookie, csrfToken, missingConnectionID, http.StatusNotFound)
}

// --- Authentication / CSRF ---

func TestImportConnectionEndpoints_RequireAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"list", http.MethodGet, "/api/v1/import-connections"},
		{"create", http.MethodPost, "/api/v1/import-connections"},
		{"update", http.MethodPatch, "/api/v1/import-connections/1"},
		{"delete", http.MethodDelete, "/api/v1/import-connections/1"},
		{"refresh", http.MethodPost, "/api/v1/import-connections/1/refresh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusUnauthorized, res.Code, tc.name)
		})
	}
}

func TestImportConnectionMutations_RequireCSRFToken(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	created := createImportConnectionForSession(t, handler, sessionCookie, csrfToken, `{"source":"trading212","display_name":"ISA","api_key":"a-secret-key-value"}`, http.StatusCreated)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"create", http.MethodPost, "/api/v1/import-connections", `{"source":"trading212","display_name":"Other","api_key":"another-secret-key"}`},
		{"update", http.MethodPatch, "/api/v1/import-connections/" + strconv.FormatInt(created.ID, 10), `{"display_name":"renamed"}`},
		{"delete", http.MethodDelete, "/api/v1/import-connections/" + strconv.FormatInt(created.ID, 10), ""},
		{"refresh", http.MethodPost, "/api/v1/import-connections/" + strconv.FormatInt(created.ID, 10) + "/refresh", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			setSameOrigin(req)
			req.AddCookie(sessionCookie)
			// Deliberately omit the CSRF token header.
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			assert.Equal(t, http.StatusForbidden, res.Code, tc.name)

			var body errorResponse
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Equal(t, "CSRF_INVALID", body.Error.Code, tc.name)
		})
	}
}

// --- PATCH omission semantics (this repo's recurring bug class: a plain
// bool overwriting an unset field on partial update) ---

func TestUpdateImportConnection_OmittedAutoRefreshPreservesExistingValue(t *testing.T) {
	t.Parallel()

	handler, _ := newImportConnectionsTestHandler(t, app.NoOpProber{})
	sessionCookie, csrfToken, _ := setupAccountAPITest(t, handler)
	created := createImportConnectionForSession(t, handler, sessionCookie, csrfToken, `{"source":"trading212","display_name":"ISA","api_key":"a-secret-key-value"}`, http.StatusCreated)
	require.False(t, created.AutoRefreshEnabled, "connections default to auto-refresh disabled until explicitly enabled")

	enabled, _ := updateImportConnectionForSession(t, handler, sessionCookie, csrfToken, created.ID, `{"display_name":"ISA","auto_refresh_enabled":true}`, http.StatusOK)
	assert.True(t, enabled.AutoRefreshEnabled)

	renamedOnly, _ := updateImportConnectionForSession(t, handler, sessionCookie, csrfToken, created.ID, `{"display_name":"ISA Renamed"}`, http.StatusOK)
	assert.True(t, renamedOnly.AutoRefreshEnabled, "omitting auto_refresh_enabled on a later PATCH must not silently disable it")
}

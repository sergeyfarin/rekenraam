package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
)

func TestAuthSessionReturnsUnauthenticatedWithoutCookie(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body authSessionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.False(t, body.Authenticated)
	assert.Nil(t, body.User)
	assert.Empty(t, body.CSRFToken)
}

func newAuthHandlerForDatabase(database *sql.DB) http.Handler {
	return newAuthHandlerForDatabaseWithOptions(database, HandlerOptions{})
}

func newAuthHandlerForDatabaseWithOptions(database *sql.DB, options HandlerOptions) http.Handler {
	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	authRepository := db.NewAuthRepository(database)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService := app.NewAuthService(authRepository, logger)
	settingsService := app.NewSettingsService(db.NewSettingsRepository(database))
	bookService := app.NewBookService(db.NewBookRepository(database), setupService)
	commodityRepository := db.NewCommodityRepository(database)
	currencyService := app.NewCurrencyService(commodityRepository, setupService)
	institutionRepository := db.NewInstitutionRepository(database)
	institutionService := app.NewInstitutionService(institutionRepository)
	accountRepository := db.NewAccountRepository(database)
	accountService := app.NewAccountService(accountRepository, institutionRepository, setupService)
	tagService := app.NewTagService(db.NewTagRepository(database))
	categoryService := app.NewCategoryService(db.NewCategoryRepository(database), setupService)
	payeeRepository := db.NewPayeeRepository(database)
	payeeService := app.NewPayeeService(payeeRepository, accountRepository)
	transactionService := app.NewTransactionService(db.NewTransactionRepository(database), payeeRepository, accountRepository, commodityRepository)
	pricingService := app.NewPricingService(db.NewPricingRepository(database))
	investmentService := app.NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, pricingService)
	importService := app.NewImportService(db.NewImportRepository(database), transactionService, accountRepository)

	return NewHandler(logger, http.NotFoundHandler(), Services{
		Setup:       setupService,
		Auth:        authService,
		Settings:    settingsService,
		Book:        bookService,
		Currency:    currencyService,
		Institution: institutionService,
		Account:     accountService,
		Tag:         tagService,
		Category:    categoryService,
		Payee:       payeeService,
		Transaction: transactionService,
		Pricing:     pricingService,
		Investment:  investmentService,
		Import:      importService,
	}, options)
}

func TestLoginRequiresSetupBeforeOwnerExists(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusConflict, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "SETUP_REQUIRED", body.Error.Code)
	assert.Equal(t, "owner setup is required before login", body.Error.Message)
}

func TestAuthSessionReturnsAuthenticatedUserForValidCookie(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body authSessionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.True(t, body.Authenticated)
	assert.NotEmpty(t, body.CSRFToken)
	if assert.NotNil(t, body.User) {
		assert.Equal(t, int64(1), body.User.ID)
		assert.Equal(t, "owner", body.User.Username)
	}
}

func TestAuthSessionReturnsUnauthenticatedForExpiredCookie(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)

	_, err := database.ExecContext(context.Background(), `
		UPDATE auth_sessions
		SET expires_at = '2020-01-01T00:00:00Z'
		WHERE token_hash IS NOT NULL
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body authSessionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.False(t, body.Authenticated)
	assert.Nil(t, body.User)
	assert.Empty(t, body.CSRFToken)
}

func TestAuthSessionReturnsUnauthenticatedForRevokedCookie(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)

	_, err := database.ExecContext(context.Background(), `
		UPDATE auth_sessions
		SET revoked_at = '2026-05-30T00:00:00Z'
		WHERE token_hash IS NOT NULL
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body authSessionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.False(t, body.Authenticated)
	assert.Nil(t, body.User)
	assert.Empty(t, body.CSRFToken)
}

func TestLogoutClearsSessionAndRevokesCookie(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)
	csrfToken := authSessionCSRFToken(t, handler, cookie)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "http://example.com")
	req.Host = "example.com"
	req.Header.Set(csrfTokenHeader, csrfToken)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)

	response := res.Result()
	responseCookies := response.Cookies()
	require.Len(t, responseCookies, 1)
	assert.Equal(t, sessionCookieName, responseCookies[0].Name)
	assert.Empty(t, responseCookies[0].Value)
	assert.Equal(t, -1, responseCookies[0].MaxAge)

	var sessionCount int
	err := database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NULL`).Scan(&sessionCount)
	require.NoError(t, err)
	assert.Equal(t, 0, sessionCount)

	var revokedCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NOT NULL`).Scan(&revokedCount)
	require.NoError(t, err)
	assert.Equal(t, 1, revokedCount)
}

func TestLogoutAcceptsTrustedForwardedOrigin(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandlerWithOptions(t, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")},
	})
	cookie := bootstrapOwner(t, handler)
	csrfToken := authSessionCSRFToken(t, handler, cookie)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Host = "127.0.0.1:16888"
	req.RemoteAddr = "127.0.0.1:45678"
	req.Header.Set("Origin", "http://127.0.0.1:1888")
	req.Header.Set("X-Forwarded-Host", "127.0.0.1:1888")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set(csrfTokenHeader, csrfToken)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)

	var sessionCount int
	err := database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions WHERE revoked_at IS NULL`).Scan(&sessionCount)
	require.NoError(t, err)
	assert.Equal(t, 0, sessionCount)
}

func TestLogoutRejectsMissingOrigin(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)
	csrfToken := authSessionCSRFToken(t, handler, cookie)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set(csrfTokenHeader, csrfToken)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusForbidden, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CSRF_INVALID", body.Error.Code)

	var sessionCount int
	err := database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions`).Scan(&sessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, sessionCount)
}

func TestLogoutRejectsInvalidCSRFToken(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "http://example.com")
	req.Host = "example.com"
	req.Header.Set(csrfTokenHeader, "invalid-token")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusForbidden, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CSRF_INVALID", body.Error.Code)

	var sessionCount int
	err := database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM auth_sessions`).Scan(&sessionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, sessionCount)
}

func authSessionCSRFToken(t *testing.T, handler http.Handler, cookie *http.Cookie) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body authSessionResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.NotEmpty(t, body.CSRFToken)
	return body.CSRFToken
}

func bootstrapOwner(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)
	response := res.Result()
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

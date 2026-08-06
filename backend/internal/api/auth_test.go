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
	importService := app.NewImportService(db.NewImportRepository(database), transactionService, accountRepository, nil, nil, nil)

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
	return sessionCookieFrom(t, response.Cookies())
}

// --- Authentication event visibility (S-07) ---

func TestAuthenticationEvents_HTTP(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)

	failed := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong"}`))
	failed.Header.Set("Content-Type", "application/json")
	setSameOrigin(failed)
	failedRes := httptest.NewRecorder()
	handler.ServeHTTP(failedRes, failed)
	require.Equal(t, http.StatusUnauthorized, failedRes.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/events", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())

	var body authenticationEventsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.NotEmpty(t, body.Events)
	assert.Equal(t, "login_failed", body.Events[0].EventType)
	assert.Equal(t, "invalid_credentials", body.Events[0].FailureReason)
	assert.Equal(t, 1, body.FailedLast24h)
	// httptest's RemoteAddr is 192.0.2.1:1234; the resolved client IP must be
	// recorded, not left blank.
	assert.NotEmpty(t, body.Events[0].ClientIP)
}

func TestAuthenticationEvents_RequireAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	// The log names client IPs and attempted usernames — it must never be
	// readable without a session.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/events", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestAuthenticationEvents_RejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/events?limit=0", nil)
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
}

// sessionCookieFrom picks the session cookie out of a response by name.
// Setup and login also set the approved-device cookie (S-04), so
// "the one cookie" is no longer a safe assumption.
func sessionCookieFrom(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName || cookie.Name == secureSessionCookieName {
			return cookie
		}
	}
	t.Fatalf("no session cookie in %v", cookies)
	return nil
}

func trustedDeviceCookieFrom(cookies []*http.Cookie) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == trustedDeviceCookieName || cookie.Name == secureTrustedDeviceCookieName {
			return cookie
		}
	}
	return nil
}

// --- Approved devices (S-04) ---

func TestSetupAndLoginIssueTrustedDeviceCookie_HTTP(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	// A fresh install must not be briefly lockout-vulnerable: the setup
	// device is approved immediately.
	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup/owner", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	setupReq.Header.Set("Content-Type", "application/json")
	setSameOrigin(setupReq)
	setupRes := httptest.NewRecorder()
	handler.ServeHTTP(setupRes, setupReq)
	require.Equal(t, http.StatusCreated, setupRes.Code)

	deviceCookie := trustedDeviceCookieFrom(setupRes.Result().Cookies())
	require.NotNil(t, deviceCookie, "owner setup must approve the device it ran on")
	assert.True(t, deviceCookie.HttpOnly, "the device cookie must not be readable from script")
	assert.Equal(t, http.SameSiteStrictMode, deviceCookie.SameSite)
	assert.Equal(t, int(app.TrustedDeviceLifetime.Seconds()), deviceCookie.MaxAge)

	// The device cookie is a throttle-scope selector, never a credential: on
	// its own it authenticates nothing.
	sessionReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	sessionReq.AddCookie(deviceCookie)
	sessionRes := httptest.NewRecorder()
	handler.ServeHTTP(sessionRes, sessionReq)
	require.Equal(t, http.StatusOK, sessionRes.Code)
	var session authSessionResponse
	require.NoError(t, json.NewDecoder(sessionRes.Body).Decode(&session))
	assert.False(t, session.Authenticated, "the device cookie must never authenticate a request")

	// Logging in from a device that already holds a valid approval reuses it.
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	setSameOrigin(loginReq)
	loginReq.AddCookie(deviceCookie)
	loginRes := httptest.NewRecorder()
	handler.ServeHTTP(loginRes, loginReq)
	require.Equal(t, http.StatusOK, loginRes.Code)
	assert.Nil(t, trustedDeviceCookieFrom(loginRes.Result().Cookies()),
		"an already-approved device must not be handed a fresh cookie on every login")
}

func TestTrustedDevices_ListAndRevoke_HTTP(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie := bootstrapOwner(t, handler)
	csrfToken := authSessionCSRFToken(t, handler, cookie)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/trusted-devices", nil)
	listReq.AddCookie(cookie)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	require.Equal(t, http.StatusOK, listRes.Code, listRes.Body.String())

	var devices trustedDevicesResponse
	require.NoError(t, json.NewDecoder(listRes.Body).Decode(&devices))
	require.Len(t, devices.Devices, 1)

	revokeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/trusted-devices/"+itoa(devices.Devices[0].ID), nil)
	revokeReq.AddCookie(cookie)
	setSameOrigin(revokeReq)
	revokeReq.Header.Set(csrfTokenHeader, csrfToken)
	revokeRes := httptest.NewRecorder()
	handler.ServeHTTP(revokeRes, revokeReq)
	require.Equal(t, http.StatusNoContent, revokeRes.Code)

	missingReq := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/trusted-devices/999999", nil)
	missingReq.AddCookie(cookie)
	setSameOrigin(missingReq)
	missingReq.Header.Set(csrfTokenHeader, csrfToken)
	missingRes := httptest.NewRecorder()
	handler.ServeHTTP(missingRes, missingReq)
	require.Equal(t, http.StatusNotFound, missingRes.Code)
}

func TestTrustedDevices_RequireAuthentication(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/auth/trusted-devices"},
		{http.MethodDelete, "/api/v1/auth/trusted-devices/1"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, http.StatusUnauthorized, res.Code, tc.path)
	}
}

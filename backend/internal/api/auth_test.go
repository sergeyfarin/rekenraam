package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/netip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

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

func TestLoginSetsSessionCookieAndReturnsOwner(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body loginResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, int64(1), body.User.ID)
	assert.Equal(t, "owner", body.User.Username)

	response := res.Result()
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "UNAUTHENTICATED", body.Error.Code)
	assert.Equal(t, "invalid username or password", body.Error.Message)
}

func TestLoginRateLimitsRepeatedFailures(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	for attempt := 0; attempt < 4; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "RATE_LIMITED", body.Error.Code)
	assert.Equal(t, "too many login attempts, try again later", body.Error.Message)
}

func TestLoginThrottlePersistsAcrossHandlerRebuild(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	for attempt := 0; attempt < 5; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)
	}

	rebuiltHandler := newAuthHandlerForDatabase(database)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	rebuiltHandler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "RATE_LIMITED", body.Error.Code)
}

func TestLoginRateLimitsRepeatedFailuresFromSameIPAcrossDifferentUsernames(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	for attempt := 0; attempt < 4; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-`+string(rune('a'+attempt))+`","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-e","password":"wrong-password"}`))
	blockedReq.Header.Set("Content-Type", "application/json")
	blockedReq.RemoteAddr = "198.51.100.10:1234"
	blockedRes := httptest.NewRecorder()

	handler.ServeHTTP(blockedRes, blockedReq)

	require.Equal(t, http.StatusTooManyRequests, blockedRes.Code)

	rebuiltHandler := newAuthHandlerForDatabase(database)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.10:1234"
	res := httptest.NewRecorder()

	rebuiltHandler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "RATE_LIMITED", body.Error.Code)
}

func TestLoginIgnoresForwardedClientIPHeadersByDefault(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	for attempt := 0; attempt < 4; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100.20")
		req.RemoteAddr = "203.0.113.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	req.RemoteAddr = "198.51.100.20:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
}

func TestLoginUsesForwardedClientIPHeadersWhenTrusted(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandlerWithOptions(t, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
	})
	bootstrapOwner(t, handler)

	for attempt := 0; attempt < 4; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100.30")
		req.RemoteAddr = "203.0.113.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
	blockedReq.Header.Set("Content-Type", "application/json")
	blockedReq.Header.Set("X-Forwarded-For", "198.51.100.30")
	blockedReq.RemoteAddr = "203.0.113.10:1234"
	blockedRes := httptest.NewRecorder()

	handler.ServeHTTP(blockedRes, blockedReq)

	require.Equal(t, http.StatusTooManyRequests, blockedRes.Code)

	rebuiltHandler := newAuthHandlerForDatabaseWithOptions(database, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.30")
	req.RemoteAddr = "203.0.113.10:1234"
	res := httptest.NewRecorder()

	rebuiltHandler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)
}

func TestLoginIgnoresForwardedClientIPHeadersWhenPeerNotAllowlisted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandlerWithOptions(t, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})
	bootstrapOwner(t, handler)

	for attempt := 0; attempt < 4; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100.40")
		req.RemoteAddr = "203.0.113.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "198.51.100.40")
	req.RemoteAddr = "198.51.100.40:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
}

func TestSuccessfulLoginClearsPersistentThrottleState(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	blockedUntil := "2000-01-01T00:00:00Z"
	_, err := database.ExecContext(context.Background(), `
		INSERT INTO login_throttles (scope_type, scope_key, failed_attempts, blocked_until, updated_at)
		VALUES ('username', 'owner', 4, ?, '2000-01-01T00:00:00Z')
	`, blockedUntil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.11:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var throttleCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM login_throttles WHERE scope_type = 'username' AND scope_key = 'owner'`).Scan(&throttleCount)
	require.NoError(t, err)
	assert.Equal(t, 0, throttleCount)
}

func TestLoginRehashesLegacyPasswordHashOnSuccess(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	legacyHash := legacyPasswordHash(t, "test-password")
	_, err := database.ExecContext(context.Background(), `
		UPDATE users
		SET password_hash = ?
		WHERE username = 'owner'
	`, legacyHash)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var passwordHash string
	err = database.QueryRowContext(context.Background(), `SELECT password_hash FROM users WHERE username = ?`, "owner").Scan(&passwordHash)
	require.NoError(t, err)
	assert.Contains(t, passwordHash, "$argon2id$")
	assert.Contains(t, passwordHash, "$v=19$m=19456,t=2,p=1$")
	assert.NotEqual(t, legacyHash, passwordHash)
}

func TestLoginValidatesRequest(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"empty username", `{"username":"","password":"test-password"}`, "username is required"},
		{"whitespace username", `{"username":"  ","password":"test-password"}`, "username is required"},
		{"empty password", `{"username":"owner","password":""}`, "password is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusBadRequest, res.Code)

			var body errorResponse
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
			assert.Equal(t, tc.message, body.Error.Message)
		})
	}
}

func TestSuccessfulLoginPreservesIPThrottleState(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	_, err := database.ExecContext(context.Background(), `
		INSERT INTO login_throttles (scope_type, scope_key, failed_attempts, blocked_until, updated_at)
		VALUES ('client_ip', '198.51.100.50', 3, NULL, '2000-01-01T00:00:00Z')
	`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.50:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var ipThrottleCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM login_throttles WHERE scope_type = 'client_ip' AND scope_key = '198.51.100.50'`).Scan(&ipThrottleCount)
	require.NoError(t, err)
	assert.Equal(t, 1, ipThrottleCount, "IP throttle should be preserved after successful login")

	var usernameThrottleCount int
	err = database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM login_throttles WHERE scope_type = 'username' AND scope_key = 'owner'`).Scan(&usernameThrottleCount)
	require.NoError(t, err)
	assert.Equal(t, 0, usernameThrottleCount, "username throttle should be cleared after successful login")
}

func TestLoginSetsSessionCookieWithMaxAge(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	cookies := res.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.Equal(t, int(app.SessionLifetime.Seconds()), cookies[0].MaxAge)
}

func TestLoginSetsSecureSessionCookieWhenTrustedProxyReportsHTTPS(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandlerWithOptions(t, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
	})
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "203.0.113.10:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	cookies := res.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure)
}

func legacyPasswordHash(t *testing.T, password string) string {
	t.Helper()

	salt := []byte("0123456789abcdef")
	hash := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, 32)
	return "$argon2id$v=19$m=8192,t=1,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}

func newAuthHandlerForDatabase(database *sql.DB) http.Handler {
	return newAuthHandlerForDatabaseWithOptions(database, HandlerOptions{})
}

func newAuthHandlerForDatabaseWithOptions(database *sql.DB, options HandlerOptions) http.Handler {
	setupRepository := db.NewSetupRepository(database)
	setupService := app.NewSetupService(setupRepository)
	authRepository := db.NewAuthRepository(database)
	authService := app.NewAuthService(authRepository)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return NewHandler(logger, http.NotFoundHandler(), setupService, authService, options)
}

func TestLoginRequiresSetupBeforeOwnerExists(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
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
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)
	response := res.Result()
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}
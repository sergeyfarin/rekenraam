package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

	"rekenraam/backend/internal/app"
)

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
	setSameOrigin(req)
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
	setSameOrigin(req)
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
			setSameOrigin(req)
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
	setSameOrigin(req)
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
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	sessionCookie := sessionCookieFrom(t, res.Result().Cookies())
	assert.Equal(t, sessionCookieName, sessionCookie.Name)
	assert.Equal(t, int(app.SessionLifetime.Seconds()), sessionCookie.MaxAge)
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
	req.Header.Set("Origin", "https://example.com")
	req.RemoteAddr = "203.0.113.10:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	sessionCookie := sessionCookieFrom(t, res.Result().Cookies())
	assert.Equal(t, secureSessionCookieName, sessionCookie.Name)
	assert.True(t, sessionCookie.Secure)
}

func TestLoginRejectsCrossOriginRequest(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://attacker.example")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusForbidden, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "CSRF_INVALID", body.Error.Code)
}

func TestLoginRejectsNonJSONContentType(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "text/plain")
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "VALIDATION_FAILED", body.Error.Code)
	assert.Equal(t, "content type must be application/json", body.Error.Message)
}

func legacyPasswordHash(t *testing.T, password string) string {
	t.Helper()

	salt := []byte("0123456789abcdef")
	hash := argon2.IDKey([]byte(password), salt, 1, 8*1024, 1, 32)
	return "$argon2id$v=19$m=8192,t=1,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}

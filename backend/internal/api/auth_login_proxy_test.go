package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginIgnoresForwardedClientIPHeadersByDefault(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	for range 4 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		setSameOrigin(req)
		req.Header.Set("X-Forwarded-For", "198.51.100.20")
		req.RemoteAddr = "203.0.113.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
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

	for range 4 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		setSameOrigin(req)
		req.Header.Set("X-Forwarded-For", "198.51.100.30")
		req.RemoteAddr = "203.0.113.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
	blockedReq.Header.Set("Content-Type", "application/json")
	setSameOrigin(blockedReq)
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
	setSameOrigin(req)
	req.Header.Set("X-Forwarded-For", "198.51.100.30")
	req.RemoteAddr = "203.0.113.10:1234"
	res := httptest.NewRecorder()

	rebuiltHandler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)
}

func TestLoginUsesRightmostUntrustedForwardedClientIPWhenTrusted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandlerWithOptions(t, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{
			netip.MustParsePrefix("203.0.113.0/24"),
			netip.MustParsePrefix("198.51.100.0/24"),
		},
	})
	bootstrapOwner(t, handler)

	for attempt := range 5 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		setSameOrigin(req)
		req.Header.Set("X-Forwarded-For", "192.0.2.8, 198.51.100.9, 203.0.113.10")
		req.RemoteAddr = "203.0.113.20:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if attempt < 4 {
			require.Equal(t, http.StatusUnauthorized, res.Code)
		} else {
			require.Equal(t, http.StatusTooManyRequests, res.Code)
		}
	}

	// The spoofed leftmost address has not accumulated a throttle. The first
	// untrusted hop from the right (192.0.2.8) has.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	req.Header.Set("X-Forwarded-For", "198.51.100.77, 192.0.2.8, 203.0.113.10")
	req.RemoteAddr = "203.0.113.20:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)
}

func TestLoginIgnoresForwardedClientIPHeadersWhenPeerNotAllowlisted(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandlerWithOptions(t, HandlerOptions{
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})
	bootstrapOwner(t, handler)

	for range 4 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-user","password":"wrong-password"}`))
		req.Header.Set("Content-Type", "application/json")
		setSameOrigin(req)
		req.Header.Set("X-Forwarded-For", "198.51.100.40")
		req.RemoteAddr = "203.0.113.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	req.Header.Set("X-Forwarded-For", "198.51.100.40")
	req.RemoteAddr = "198.51.100.40:1234"
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
}

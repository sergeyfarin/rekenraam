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

	for attempt := 0; attempt < 4; attempt++ {
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

	for attempt := 0; attempt < 4; attempt++ {
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

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddlewareAddsRequestIDAndLogsRequest(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	var seenRequestID string
	handler := withRequestID(withSecurityHeaders(HandlerOptions{}, withRequestLogging(logger, withRecovery(logger,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenRequestID = RequestIDFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}),
	))))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)

	requestID := res.Header().Get(requestIDHeader)
	require.NotEmpty(t, requestID)
	assert.Equal(t, requestID, seenRequestID)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, requestID)
	assert.Contains(t, logs.String(), `"msg":"request completed"`)
	assert.Contains(t, logs.String(), `"request_id":"`+requestID+`"`)
	assert.Contains(t, logs.String(), `"status":204`)
	assert.Equal(t, "nosniff", res.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", res.Header().Get("X-Frame-Options"))
	assert.Contains(t, res.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
}

func TestMiddlewareAlwaysReplacesCallerRequestIDWithFreshServerUUID(t *testing.T) {
	t.Parallel()

	handler := withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, w.Header().Get(requestIDHeader), RequestIDFromContext(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}))

	serverIDs := make([]string, 0, 2)
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set(requestIDHeader, "caller-reused-id")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		require.Equal(t, http.StatusNoContent, res.Code)
		serverIDs = append(serverIDs, res.Header().Get(requestIDHeader))
	}

	assert.NotEqual(t, "caller-reused-id", serverIDs[0])
	assert.NotEqual(t, serverIDs[0], serverIDs[1])
	for _, requestID := range serverIDs {
		assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, requestID)
	}
}

func TestRecoveryMiddlewareReturnsAPIErrorEnvelope(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	handler := withRequestID(withSecurityHeaders(HandlerOptions{}, withRequestLogging(logger, withRecovery(logger,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}),
	))))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Header().Get("Content-Type"), "application/json")

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "INTERNAL_ERROR", body.Error.Code)
	assert.Equal(t, "internal server error", body.Error.Message)

	requestID := res.Header().Get(requestIDHeader)
	require.NotEmpty(t, requestID)
	assert.Contains(t, logs.String(), `"msg":"panic recovered"`)
	assert.Contains(t, logs.String(), `"request_id":"`+requestID+`"`)
	assert.Contains(t, logs.String(), `"status":500`)
}

func TestSecurityMiddlewareDisablesAuthResponseCaching(t *testing.T) {
	t.Parallel()

	handler := withSecurityHeaders(HandlerOptions{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)
	assert.Equal(t, "no-store", res.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", res.Header().Get("Pragma"))
}

// The app must never load fonts, icons, scripts, styles or images from a third
// party (docs/conventions.md, Security And Deployment Conventions): every CSP
// source is same-origin, inline or data:, never a host or scheme wildcard.
func TestSecurityMiddlewareCSPLoadsOnlySameOriginResources(t *testing.T) {
	t.Parallel()

	handler := withSecurityHeaders(HandlerOptions{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	allowed := map[string]bool{"'self'": true, "'none'": true, "'unsafe-inline'": true, "data:": true}
	directives := map[string]bool{}
	for directive := range strings.SplitSeq(res.Header().Get("Content-Security-Policy"), ";") {
		fields := strings.Fields(directive)
		if len(fields) == 0 {
			continue
		}
		directives[fields[0]] = true
		for _, source := range fields[1:] {
			assert.True(t, allowed[source], "%s allows non-local source %q", fields[0], source)
		}
	}
	for _, name := range []string{"default-src", "script-src", "style-src", "font-src", "img-src", "connect-src"} {
		assert.True(t, directives[name], "CSP must set %s explicitly", name)
	}
}

func TestSecurityMiddlewareSetsHSTSOnlyForHTTPS(t *testing.T) {
	t.Parallel()

	handler := withSecurityHeaders(HandlerOptions{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("no HSTS over plain HTTP", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Empty(t, res.Header().Get("Strict-Transport-Security"))
	})

	t.Run("HSTS set when TLS active", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		req.TLS = &tls.ConnectionState{}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		assert.Equal(t, "max-age=63072000; includeSubDomains", res.Header().Get("Strict-Transport-Security"))
	})
}

func TestNativeSQLiteBusyReturnsRetryableAPIError(t *testing.T) {
	t.Parallel()
	for _, code := range []sqlite.ErrNo{sqlite.ErrBusy, sqlite.ErrLocked} {
		res := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/transactions", nil)
		writeServiceInternalError(res, request, nil, "test", fmt.Errorf("wrapped: %w", sqlite.Error{Code: code}))
		require.Equal(t, http.StatusServiceUnavailable, res.Code)
		var body errorResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
		assert.Equal(t, "RESOURCE_BUSY", body.Error.Code)
		assert.NotContains(t, res.Body.String(), "wrapped")
	}
}

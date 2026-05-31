package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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

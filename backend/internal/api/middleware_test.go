package api

import (
	"bytes"
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
	handler := withRequestID(withRequestLogging(logger, withRecovery(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRequestID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))))

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
}

func TestRecoveryMiddlewareReturnsAPIErrorEnvelope(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	handler := withRequestID(withRequestLogging(logger, withRecovery(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Header().Get("Content-Type"), "application/json")

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "INTERNAL_SERVER_ERROR", body.Error.Code)
	assert.Equal(t, "internal server error", body.Error.Message)

	requestID := res.Header().Get(requestIDHeader)
	require.NotEmpty(t, requestID)
	assert.Contains(t, logs.String(), `"msg":"panic recovered"`)
	assert.Contains(t, logs.String(), `"request_id":"`+requestID+`"`)
	assert.Contains(t, logs.String(), `"status":500`)
}

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginSetsSessionCookieAndReturnsOwner(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body loginResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, int64(1), body.User.ID)
	assert.Equal(t, "owner", body.User.Username)

	response := res.Result()
	cookies := response.Cookies()
	sessionCookie := sessionCookieFrom(t, cookies)
	assert.Equal(t, sessionCookieName, sessionCookie.Name)
	assert.NotEmpty(t, sessionCookie.Value)
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	bootstrapOwner(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
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
		setSameOrigin(req)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
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
		setSameOrigin(req)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)
	}

	rebuiltHandler := newAuthHandlerForDatabase(database)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
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
		setSameOrigin(req)
		req.RemoteAddr = "198.51.100.10:1234"
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusUnauthorized, res.Code)
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"missing-e","password":"wrong-password"}`))
	blockedReq.Header.Set("Content-Type", "application/json")
	setSameOrigin(blockedReq)
	blockedReq.RemoteAddr = "198.51.100.10:1234"
	blockedRes := httptest.NewRecorder()

	handler.ServeHTTP(blockedRes, blockedReq)

	require.Equal(t, http.StatusTooManyRequests, blockedRes.Code)

	rebuiltHandler := newAuthHandlerForDatabase(database)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	setSameOrigin(req)
	req.RemoteAddr = "198.51.100.10:1234"
	res := httptest.NewRecorder()

	rebuiltHandler.ServeHTTP(res, req)

	require.Equal(t, http.StatusTooManyRequests, res.Code)

	var body errorResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "RATE_LIMITED", body.Error.Code)
}

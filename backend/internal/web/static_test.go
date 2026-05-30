package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerServesStaticIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "<!doctype html>")
}

func TestHandlerFallsBackToIndexForFrontendRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts/42", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "<!doctype html>")
}

func TestHandlerReturnsNotFoundForMissingAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_app/missing.js", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

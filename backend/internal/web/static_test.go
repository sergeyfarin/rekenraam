package web

import (
	"io/fs"
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

func TestHandlerServesExistingAsset(t *testing.T) {
	dist, err := fs.Sub(files, "dist")
	require.NoError(t, err)

	entries, err := fs.ReadDir(dist, "_app/immutable/entry")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	assetPath := "/_app/immutable/entry/" + entries[0].Name()
	req := httptest.NewRequest(http.MethodGet, assetPath, nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.NotEmpty(t, res.Body.String())
}

func TestHandlerReturnsNotFoundForMissingAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_app/missing.js", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

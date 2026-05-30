package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testHandler() http.Handler {
	return HandlerForFS(fstest.MapFS{
		"index.html":                  {Data: []byte("<!doctype html><html><body>app</body></html>")},
		"robots.txt":                  {Data: []byte("User-agent: *\nDisallow:\n")},
		"_app/immutable/entry/app.js": {Data: []byte("console.log('app');")},
	})
}

func TestHandlerServesStaticIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	testHandler().ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "<!doctype html>")
}

func TestHandlerFallsBackToIndexForFrontendRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts/42", nil)
	res := httptest.NewRecorder()

	testHandler().ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "<!doctype html>")
}

func TestHandlerServesExistingAsset(t *testing.T) {
	assetPath := "/_app/immutable/entry/app.js"
	req := httptest.NewRequest(http.MethodGet, assetPath, nil)
	res := httptest.NewRecorder()

	testHandler().ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	assert.NotEmpty(t, res.Body.String())
}

func TestHandlerReturnsNotFoundForMissingAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_app/missing.js", nil)
	res := httptest.NewRecorder()

	testHandler().ServeHTTP(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestHandlerReturnsNotFoundWhenIndexIsMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	HandlerForFS(fstest.MapFS{}).ServeHTTP(res, req)

	require.Equal(t, http.StatusNotFound, res.Code)
}

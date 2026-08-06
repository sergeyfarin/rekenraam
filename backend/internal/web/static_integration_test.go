package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const embeddedWebTestEnv = "REKENRAAM_RUN_EMBEDDED_WEB_TESTS"

func TestHandlerServesEmbeddedBuildOutput(t *testing.T) {
	if os.Getenv(embeddedWebTestEnv) != "1" {
		t.Skip("embedded build output verification is only run from the integrated build path")
	}

	handler := Handler()
	dist, err := fs.Sub(files, "dist")
	require.NoError(t, err)

	t.Run("serves index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusOK, res.Code)
		assert.Contains(t, strings.ToLower(res.Body.String()), "<!doctype html>")
	})

	t.Run("falls back for frontend route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/accounts/42", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusOK, res.Code)
		assert.Contains(t, strings.ToLower(res.Body.String()), "<!doctype html>")
	})

	t.Run("serves embedded asset", func(t *testing.T) {
		assetPath := findEmbeddedAssetPath(t, dist)
		req := httptest.NewRequest(http.MethodGet, "/"+assetPath, nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusOK, res.Code)
		assert.NotEmpty(t, res.Body.String())
	})

	t.Run("returns not found for missing asset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_app/missing.js", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		require.Equal(t, http.StatusNotFound, res.Code)
	})
}

func findEmbeddedAssetPath(t *testing.T, filesystem fs.FS) string {
	t.Helper()

	var assetPath string
	err := fs.WalkDir(filesystem, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(path, "_app/") {
			assetPath = path
			return fs.SkipAll
		}

		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, assetPath)

	return assetPath
}

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Equal(t, "ok", body["status"])
}

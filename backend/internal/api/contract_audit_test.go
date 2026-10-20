package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureContractAudit is opt-in evidence collection for scripts/audit-api-contract.py.
// It wraps only synthetic test instances, never production traffic. The runner
// owns a private temporary directory and deletes responses (including test
// credentials) after validation; published evidence contains no response bodies.
func captureContractAudit(t *testing.T, handler http.Handler) http.Handler {
	t.Helper()
	directory := os.Getenv("REKENRAAM_CONTRACT_AUDIT_DIR")
	if directory == "" {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		record := struct {
			Test        string `json:"test"`
			Method      string `json:"method"`
			Path        string `json:"path"`
			Status      int    `json:"status"`
			ContentType string `json:"content_type"`
			Body        []byte `json:"body"`
		}{t.Name(), r.Method, r.URL.Path, response.Code, response.Header().Get("Content-Type"), response.Body.Bytes()}
		raw, err := json.Marshal(record)
		require.NoError(t, err)
		file, err := os.CreateTemp(directory, "response-*.json")
		require.NoError(t, err)
		_, err = file.Write(raw)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		for key, values := range response.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(response.Code)
		if response.Body.Len() > 0 {
			_, err = w.Write(response.Body.Bytes())
			require.NoError(t, err)
		}
	})
}

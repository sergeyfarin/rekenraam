package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHello(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/hello", nil)
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["message"] != "hello from rekenraam backend" {
		t.Fatalf("unexpected message %q", body["message"])
	}
}

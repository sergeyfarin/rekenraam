package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesStaticIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "<!doctype html>") {
		t.Fatalf("expected frontend index response")
	}
}

func TestHandlerFallsBackToIndexForFrontendRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts/42", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "<!doctype html>") {
		t.Fatalf("expected frontend index fallback response")
	}
}

func TestHandlerReturnsNotFoundForMissingAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_app/missing.js", nil)
	res := httptest.NewRecorder()

	Handler().ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", res.Code)
	}
}

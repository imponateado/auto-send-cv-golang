package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterRoutes_CORSPreflight(t *testing.T) {
	mux := http.NewServeMux()
	h := RegisterRoutes(mux, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/vacancies", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin '*', got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("expected Access-Control-Allow-Methods header to be set")
	}
}

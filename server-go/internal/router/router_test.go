package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthWithoutDatabase(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Fatalf("GET /health: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLoginWithoutDatabase(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/login", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured login to return 503, got %d", response.Code)
	}
}

func TestMeWithoutDatabase(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured authentication to return 503, got %d", response.Code)
	}
}

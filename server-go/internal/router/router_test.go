package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthWithoutDatabase(t *testing.T) {
	response := httptest.NewRecorder()
	New().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Fatalf("GET /health: status=%d body=%s", response.Code, response.Body.String())
	}
}

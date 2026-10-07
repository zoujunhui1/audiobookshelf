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

func TestRefreshWithoutDatabase(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/refresh", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured refresh to return 503, got %d", response.Code)
	}
}

func TestLogoutWithoutDatabase(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/logout", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured logout to return 503, got %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "refresh_token" || cookies[0].MaxAge != -1 {
		t.Fatal("unconfigured logout must still clear the refresh cookie")
	}
}

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMissingCredentialsAndRateLimit(t *testing.T) {
	r := gin.New()
	// Invalid requests must be rejected before invoking the auth service.
	r.POST("/login", Login(nil, 1, time.Minute, "Too many authentication requests"))
	for i, expected := range []int{http.StatusBadRequest, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"alice"}`))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != expected {
			t.Fatalf("attempt %d returned %d, want %d", i, res.Code, expected)
		}
	}
}

func TestMalformedCredentials(t *testing.T) {
	r := gin.New()
	r.POST("/login", Login(nil, 0, time.Minute, ""))
	for _, body := range []string{`{`, `{}`, `{"username":null,"password":"p"}`, `{"username":123,"password":"p"}`} {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("malformed credentials returned %d", res.Code)
		}
	}
}

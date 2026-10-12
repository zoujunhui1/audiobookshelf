package router

import (
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"audiobookshelf-go/internal/proxy"
)

func TestExplicitRoutesAndFallback(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthcheck" {
			return
		}
		w.Header().Set("X-Backend", "node")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(r.Method + " " + r.RequestURI))
	}))
	defer backend.Close()
	u, _ := url.Parse(backend.URL)
	node, err := proxy.NewNode(proxy.NodeOptions{Port: u.Port(), Dir: t.TempDir(), Executable: "must-not-start", StartupTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	login := func(c *gin.Context) { c.String(200, "Go login") }
	router := New(node, login, "/audiobookshelf")
	frontend := httptest.NewServer(router)
	defer frontend.Close()
	for _, tc := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/health", 200, `{"status":"ok"}`},
		{"POST", "/health", 202, "POST /health"},
		{"GET", "/health/", 202, "GET /health/"},
		{"POST", "/login", 200, "Go login"},
		{"POST", "/audiobookshelf/login", 200, "Go login"},
		{"GET", "/login", 202, "GET /login"},
		{"GET", "/api/items?q=1", 202, "GET /api/items?q=1"},
		{"OPTIONS", "/api/items", 202, "OPTIONS /api/items"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, frontend.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := frontend.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.status || string(body) != tc.body {
				t.Fatalf("got %d %q, want %d %q", res.StatusCode, body, tc.status, tc.body)
			}
		})
	}
}

func TestHealthDoesNotStartNode(t *testing.T) {
	node, err := proxy.NewNode(proxy.NodeOptions{Port: "3333", Dir: t.TempDir(), Executable: "must-not-start", StartupTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	res := httptest.NewRecorder()
	New(node, func(c *gin.Context) {}, "").ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("Go health depends on Node: %d", res.Code)
	}
}

package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestForwardPreservesRequestAndResponse(t *testing.T) {
	node, _ := externalNode(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthcheck" {
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.RequestURI != "/api/items/a%2Fb?q=a%20b" || string(body) != `{"value":1}` {
			t.Errorf("request changed: %s %s %s", r.Method, r.RequestURI, body)
		}
		if r.Header.Get("Authorization") != "Bearer test" || r.Header.Get("Cookie") != "refresh_token=test" || r.Host != "public.example" {
			t.Error("authentication headers or host changed")
		}
		if r.Header.Get("X-Forwarded-Host") != "public.example" || r.Header.Get("X-Forwarded-Proto") != "http" || r.Header.Get("X-Forwarded-For") == "" {
			t.Error("missing forwarding headers")
		}
		w.Header().Add("Set-Cookie", "refresh_token=new; HttpOnly; Path=/")
		w.Header().Add("Set-Cookie", "other=1; Path=/")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"ok":true}`)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/items/a%2Fb?q=a%20b", strings.NewReader(`{"value":1}`))
	req.Host = "public.example"
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Cookie", "refresh_token=test")
	res := httptest.NewRecorder()
	Forward(node).ServeHTTP(res, req)
	if res.Code != http.StatusCreated || res.Body.String() != `{"ok":true}` || len(res.Header().Values("Set-Cookie")) != 2 {
		t.Fatalf("response changed: %d %s %v", res.Code, res.Body.String(), res.Header())
	}
}

func TestForwardWebSocketUpgrade(t *testing.T) {
	node, _ := externalNode(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthcheck" {
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		rw.Flush()
		data := make([]byte, 4)
		if _, err := io.ReadFull(rw, data); err == nil {
			conn.Write(data)
		}
	}))
	server := httptest.NewServer(Forward(node))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(conn, "GET /socket.io/?transport=websocket HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n", u.Host)
	reader := bufio.NewReader(conn)
	res, err := http.ReadResponse(reader, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade failed: %v %v", res, err)
	}
	conn.Write([]byte("ping"))
	data := make([]byte, 4)
	if _, err := io.ReadFull(reader, data); err != nil || string(data) != "ping" {
		t.Fatalf("upgrade tunnel failed: %q %v", data, err)
	}
}

func TestStartupFailureReturnsUnavailable(t *testing.T) {
	node, _ := ownedNode(t, time.Second)
	node.options.Executable = t.TempDir() + "/missing-node"
	res := httptest.NewRecorder()
	Forward(node).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/items", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", res.Code)
	}
}

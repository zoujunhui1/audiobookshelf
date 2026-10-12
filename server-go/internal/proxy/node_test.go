package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test binary stands in for Node, so lifecycle tests need neither npm nor
// the application's database, and cannot modify existing user data.
func TestNodeHelper(t *testing.T) {
	if os.Getenv("PROXY_TEST_CHILD") != "1" {
		return
	}
	port := ""
	for i, arg := range os.Args {
		if arg == "--port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	f, err := os.OpenFile(os.Getenv("PROXY_TEST_STARTS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(2)
	}
	f.WriteString("start\n")
	f.Close()
	time.Sleep(150 * time.Millisecond)
	if os.Getenv("PROXY_TEST_HANG") == "1" {
		time.Sleep(time.Hour)
	}
	err = http.ListenAndServe("127.0.0.1:"+port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func ownedNode(t *testing.T, timeout time.Duration) (*Node, string) {
	t.Helper()
	dir := t.TempDir()
	starts := filepath.Join(dir, "starts")
	t.Setenv("PROXY_TEST_CHILD", "1")
	t.Setenv("PROXY_TEST_STARTS", starts)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "node")
	quoted := "'" + strings.ReplaceAll(executable, "'", "'\"'\"'") + "'"
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec "+quoted+" -test.run=TestNodeHelper -- \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	node, err := NewNode(NodeOptions{Port: port, Dir: dir, Executable: script, StartupTimeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(node.Close)
	return node, starts
}

func externalNode(t *testing.T, handler http.Handler) (*Node, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	node, err := NewNode(NodeOptions{Port: u.Port(), Dir: t.TempDir(), Executable: "must-not-be-started", StartupTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(node.Close)
	return node, server
}

func TestConcurrentStartupAndRestart(t *testing.T) {
	node, starts := ownedNode(t, 5*time.Second)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- node.EnsureRunning(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(starts)
	if string(data) != "start\n" {
		t.Fatalf("expected one startup, got %q", data)
	}
	cmd := node.cmd
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// Wait for Wait() to publish exit without consuming the manager's result.
	deadline := time.Now().Add(3 * time.Second)
	for len(node.done) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := node.EnsureRunning(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(starts)
	if string(data) != "start\nstart\n" {
		t.Fatalf("expected one restart, got %q", data)
	}
	node.Close()
	if err := node.EnsureRunning(context.Background()); err == nil {
		t.Fatal("closed manager accepted request")
	}
}

func TestCanceledWaiterDoesNotCancelStartup(t *testing.T) {
	node, starts := ownedNode(t, 5*time.Second)
	result := make(chan error, 1)
	go func() { result <- node.EnsureRunning(context.Background()) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(starts); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := node.EnsureRunning(ctx); err == nil {
		t.Fatal("canceled waiter succeeded")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestStartupTimeoutStopsChild(t *testing.T) {
	t.Setenv("PROXY_TEST_HANG", "1")
	node, _ := ownedNode(t, 500*time.Millisecond)
	if err := node.EnsureRunning(context.Background()); err == nil {
		t.Fatal("unready child accepted")
	}
	if node.cmd != nil {
		t.Fatal("timed out child was left running")
	}
}

func TestExistingBackendIsNotStopped(t *testing.T) {
	node, server := externalNode(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err := node.EnsureRunning(context.Background()); err != nil {
		t.Fatal(err)
	}
	if node.cmd != nil {
		t.Fatal("started a duplicate backend")
	}
	node.Close()
	res, err := http.Get(server.URL)
	if err != nil {
		t.Fatal("external backend stopped:", err)
	}
	res.Body.Close()
}

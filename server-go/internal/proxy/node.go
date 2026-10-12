package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type NodeOptions struct {
	Port           string
	Dir            string
	Executable     string
	StartupTimeout time.Duration
}

// Node manages one local backend. The gate serializes checks and startup while
// allowing queued requests to cancel. Only processes started here are stopped.
type Node struct {
	target  *url.URL
	options NodeOptions
	client  *http.Client
	gate    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	done    chan error
}

func NewNode(options NodeOptions) (*Node, error) {
	if options.StartupTimeout <= 0 {
		return nil, errors.New("Node startup timeout must be positive")
	}
	dir, err := filepath.Abs(options.Dir)
	if err != nil {
		return nil, err
	}
	options.Dir = dir
	ctx, cancel := context.WithCancel(context.Background())
	return &Node{
		target:  &url.URL{Scheme: "http", Host: "127.0.0.1:" + options.Port},
		options: options,
		client: &http.Client{
			Timeout:       time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		gate:   make(chan struct{}, 1),
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// EnsureRunning checks readiness before every forward. An interrupted request
// does not stop a starting backend; the next request can continue waiting for it.
func (n *Node) EnsureRunning(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, n.options.StartupTimeout)
	defer cancel()
	select {
	case n.gate <- struct{}{}:
		defer func() { <-n.gate }()
	case <-ctx.Done():
		return ctx.Err()
	case <-n.ctx.Done():
		return errors.New("Node manager is closed")
	}
	if n.ctx.Err() != nil {
		return errors.New("Node manager is closed")
	}
	// Reap an exited owned process before deciding whether it needs restarting.
	if n.cmd != nil {
		select {
		case err := <-n.done:
			log.Printf("Node backend exited: %v", err)
			n.cmd, n.done = nil, nil
		default:
		}
	}
	if n.ready(ctx) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if n.cmd == nil {
		// Match npm start, with explicit host/port so inherited dev configuration
		// cannot move the backend away from the proxy target.
		cmd := exec.Command(n.options.Executable, "dist-server/index.js", "--host", "127.0.0.1", "--port", n.options.Port)
		cmd.Dir = n.options.Dir
		cmd.Env = append(os.Environ(), "NODE_ENV=production")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start Node backend: %w", err)
		}
		n.cmd = cmd
		n.done = make(chan error, 1)
		done := n.done
		go func() { done <- cmd.Wait() }()
		log.Printf("started Node backend (pid %d) at %s", cmd.Process.Pid, n.target)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-n.done:
			n.cmd, n.done = nil, nil
			return fmt.Errorf("Node exited before becoming ready: %v", err)
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				n.stopOwned()
			}
			return fmt.Errorf("wait for Node readiness: %w", ctx.Err())
		case <-n.ctx.Done():
			return errors.New("Node manager is closed")
		case <-ticker.C:
			if n.ready(ctx) {
				return nil
			}
		}
	}
}

func (n *Node) ready(ctx context.Context) bool {
	u := *n.target
	u.Path = "/healthcheck"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false
	}
	res, err := n.client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func (n *Node) stopOwned() {
	if n.cmd != nil {
		// Node handles SIGINT for graceful shutdown. Bound the wait for a hung
		// backend, and always reap the child process.
		_ = n.cmd.Process.Signal(os.Interrupt)
		select {
		case <-n.done:
		case <-time.After(3 * time.Second):
			_ = n.cmd.Process.Kill()
			<-n.done
		}
		n.cmd, n.done = nil, nil
	}
}

func (n *Node) Close() {
	n.cancel()
	n.gate <- struct{}{}
	defer func() { <-n.gate }()
	n.stopOwned()
	n.client.CloseIdleConnections()
}

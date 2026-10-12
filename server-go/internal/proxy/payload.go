package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LoginPayload reuses Node's existing user/settings serialization. Credentials
// and token generation belong entirely to Go; this calls /api/authorize.
func (n *Node) LoginPayload(ctx context.Context, accessToken string) (map[string]any, error) {
	if err := n.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	u := *n.target
	u.Path = "/api/authorize"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Node authorize returned HTTP %d", res.StatusCode)
	}
	var payload map[string]any
	decoder := json.NewDecoder(io.LimitReader(res.Body, 64<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

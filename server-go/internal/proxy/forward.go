package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
)

// Forward preserves the incoming path, query, body and application headers.
// ReverseProxy also handles streaming responses and WebSocket upgrades.
func Forward(node *Node) http.Handler {
	forward := &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(node.target)
			req.Out.Host = req.In.Host
			req.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("forward to Node: %v", err)
			http.Error(w, "Node backend unavailable", http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := node.EnsureRunning(r.Context()); err != nil {
			log.Printf("ensure Node running: %v", err)
			http.Error(w, "Node backend unavailable", http.StatusServiceUnavailable)
			return
		}
		forward.ServeHTTP(w, r)
	})
}

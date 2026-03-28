// Package proxy implements a transparent HTTP proxy that intercepts MCP tools/call requests.
// It sits between the dws binary and the real MCP server, recording all tools/call arguments
// for later comparison without modifying the traffic.
package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// CapturedCall holds the arguments from a single tools/call MCP request.
type CapturedCall struct {
	ToolName  string
	Arguments map[string]any
}

// Proxy is a transparent HTTP proxy that intercepts MCP tools/call requests.
// All requests are forwarded to the real MCP server unchanged; tools/call
// argument payloads are additionally recorded for inspection.
type Proxy struct {
	server    *http.Server
	listener  net.Listener
	targetURL *url.URL
	mu        sync.Mutex
	calls     []CapturedCall
}

// New creates and starts a proxy that forwards all traffic to targetURL.
// The proxy listens on a random local port; call URL() to get the address.
func New(targetURL string) (*Proxy, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	proxy := &Proxy{
		targetURL: parsed,
		listener:  listener,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", proxy.handle)
	proxy.server = &http.Server{Handler: mux}
	go proxy.server.Serve(listener) //nolint:errcheck
	return proxy, nil
}

// URL returns the local proxy address (e.g., "http://127.0.0.1:PORT").
func (p *Proxy) URL() string {
	return "http://" + p.listener.Addr().String()
}

// DrainCalls returns and clears all captured tools/call arguments since the last drain.
func (p *Proxy) DrainCalls() []CapturedCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	calls := p.calls
	p.calls = nil
	return calls
}

// Close shuts down the proxy server.
func (p *Proxy) Close() {
	p.server.Close()
}

func (p *Proxy) handle(responseWriter http.ResponseWriter, request *http.Request) {
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(responseWriter, "read body failed", http.StatusBadGateway)
		return
	}
	request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if handled := p.tryCaptureMCPCall(responseWriter, bodyBytes); handled {
		return
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(p.targetURL)
	reverseProxy.ServeHTTP(responseWriter, request)
}

// mcpRequest is the minimal structure needed to identify a tools/call MCP request.
type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"params"`
}

func (p *Proxy) tryCaptureMCPCall(responseWriter http.ResponseWriter, body []byte) bool {
	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	if req.Method != "tools/call" {
		return false
	}
	p.mu.Lock()
	p.calls = append(p.calls, CapturedCall{
		ToolName:  req.Params.Name,
		Arguments: req.Params.Arguments,
	})
	p.mu.Unlock()

	responseWriter.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(responseWriter).Encode(map[string]any{
		"jsonrpc": firstNonEmpty(req.JSONRPC, "2.0"),
		"id":      req.ID,
		"result": map[string]any{
			"content": map[string]any{
				"probe":    true,
				"toolName": req.Params.Name,
			},
		},
	})
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

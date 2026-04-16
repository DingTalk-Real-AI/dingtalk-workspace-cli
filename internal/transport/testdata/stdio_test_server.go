// Minimal MCP stdio server for integration tests.
// Implements initialize, tools/list, tools/call over newline-delimited JSON-RPC.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			writeResp(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}

		switch req.Method {
		case "initialize":
			writeResp(response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "test-server", "version": "0.0.1"},
			}})

		case "tools/list":
			writeResp(response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
				"tools": []map[string]any{
					{
						"name":        "test_echo",
						"description": "Echo the input message",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]any{"type": "string", "description": "Message to echo"},
							},
							"required": []string{"message"},
						},
					},
					{
						"name":        "test_add",
						"description": "Add two numbers",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"a": map[string]any{"type": "integer"},
								"b": map[string]any{"type": "integer"},
							},
							"required": []string{"a", "b"},
						},
					},
				},
			}})

		case "tools/call":
			handleCall(req.ID, req.Params)

		case "notifications/initialized":
			// no response
			continue

		default:
			writeResp(response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}})
		}
	}
}

func handleCall(id, params json.RawMessage) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		writeResp(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid params"}})
		return
	}

	switch p.Name {
	case "test_echo":
		msg, _ := p.Arguments["message"].(string)
		writeResp(response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("Echo: %s", msg)}},
		}})
	case "test_add":
		a, _ := p.Arguments["a"].(float64)
		b, _ := p.Arguments["b"].(float64)
		writeResp(response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%.0f", a+b)}},
		}})
	default:
		writeResp(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32601, Message: "unknown tool: " + p.Name}})
	}
}

func writeResp(resp response) {
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

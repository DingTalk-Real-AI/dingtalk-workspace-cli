// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillgen"
)

type docsServerExpectation struct {
	File               string
	ServerID           string
	ServerName         string
	ServerStatus       string
	Command            string
	InitializeResponse any
	ToolsListResponse  any
}

func main() {
	var docsDir string
	var outputPath string
	var strict bool

	flag.StringVar(&docsDir, "docs-dir", "docs/mcp", "Directory containing docs/mcp protocol fixtures")
	flag.StringVar(&outputPath, "output", "skills/generated/docs/schema/catalog.json", "Output catalog snapshot path")
	flag.BoolVar(&strict, "strict", true, "Fail if no runtime servers can be discovered")
	flag.Parse()

	expectations, err := loadDocsMCPExpectations(docsDir)
	if err != nil {
		fail(fmt.Errorf("load docs expectations: %w", err))
	}
	if len(expectations) == 0 {
		fail(fmt.Errorf("no docs/mcp expectations found in %s", docsDir))
	}

	gateway := newDocsMCPGateway(expectations)
	defer gateway.Close()

	cacheRoot, err := os.MkdirTemp("", "dws-docs-snapshot-cache-*")
	if err != nil {
		fail(fmt.Errorf("create temporary cache directory: %w", err))
	}
	defer func() {
		_ = os.RemoveAll(cacheRoot)
	}()

	service := discovery.NewService(
		market.NewClient(gateway.URL, gateway.Client()),
		transport.NewClient(gateway.Client()),
		cache.NewStore(cacheRoot),
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		fail(fmt.Errorf("discover servers: %w", err))
	}
	runtimeServers, failures := service.DiscoverAllRuntime(context.Background(), servers)
	if strict && len(runtimeServers) == 0 {
		if len(failures) > 0 {
			fail(fmt.Errorf("discover runtime failed: %w", failures[0].Err))
		}
		fail(fmt.Errorf("discover runtime produced no servers"))
	}

	catalog := ir.BuildCatalog(runtimeServers)
	data, err := json.MarshalIndent(skillgen.CatalogSnapshotPayload(catalog), "", "  ")
	if err != nil {
		fail(fmt.Errorf("encode catalog snapshot: %w", err))
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fail(fmt.Errorf("create output directory: %w", err))
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		fail(fmt.Errorf("write snapshot: %w", err))
	}

	_, _ = fmt.Fprintf(
		os.Stderr,
		"generated docs snapshot: output=%s products=%d runtime_ok=%d runtime_fail=%d\n",
		outputPath,
		len(catalog.Products),
		len(runtimeServers),
		len(failures),
	)
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate-docs-snapshot: %v\n", err)
	os.Exit(1)
}

func loadDocsMCPExpectations(dir string) ([]docsServerExpectation, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	out := make([]docsServerExpectation, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}

		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, fmt.Errorf("decode %s: %w", file, err)
		}

		server := mapValue(payload["server"])
		serverID := stringValue(server["id"])
		if serverID == "" {
			serverID = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		}
		command := firstPrefix(server["prefix"])
		if command == "" {
			command = serverID
		}

		out = append(out, docsServerExpectation{
			File:               filepath.Base(file),
			ServerID:           serverID,
			ServerName:         stringValue(server["name"]),
			ServerStatus:       normalizeStatus(stringValue(server["status"])),
			Command:            command,
			InitializeResponse: firstSuccessfulInitializeResponse(payload),
			ToolsListResponse:  mergedSuccessfulMethodResponse(payload, "tools/list"),
		})
	}

	return out, nil
}

func normalizeStatus(status string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		return "active"
	}
	return status
}

func firstSuccessfulInitializeResponse(payload map[string]any) any {
	initialize := mapValue(payload["initialize"])
	attempts, ok := sliceValue(initialize["attempts"])
	if !ok || len(attempts) == 0 {
		return nil
	}
	var fallback any
	for _, raw := range attempts {
		attempt := mapValue(raw)
		response := attempt["response"]
		if fallback == nil && response != nil {
			fallback = response
		}
		if isSuccessfulJSONRPCResponse(response) {
			return response
		}
	}
	return fallback
}

func mergedSuccessfulMethodResponse(payload map[string]any, method string) any {
	methods := mapValue(payload["methods"])
	methodPayload := mapValue(methods[method])
	pages, ok := sliceValue(methodPayload["pages"])
	if !ok || len(pages) == 0 {
		return nil
	}

	var fallback any
	var firstSuccess any
	mergedTools := make([]any, 0)

	for _, raw := range pages {
		page := mapValue(raw)
		response := page["response"]
		if fallback == nil && response != nil {
			fallback = response
		}
		if !isSuccessfulJSONRPCResponse(response) {
			continue
		}
		if firstSuccess == nil {
			firstSuccess = response
		}
		result := mapValue(mapValue(response)["result"])
		tools, ok := sliceValue(result["tools"])
		if ok {
			mergedTools = append(mergedTools, tools...)
		}
	}

	if firstSuccess == nil {
		return fallback
	}

	firstSuccessMap := mapValue(firstSuccess)
	if len(mergedTools) == 0 {
		return firstSuccessMap
	}

	merged := copyMap(firstSuccessMap)
	merged["result"] = map[string]any{
		"tools": mergedTools,
	}
	delete(merged, "error")
	return merged
}

func isSuccessfulJSONRPCResponse(value any) bool {
	response := mapValue(value)
	if len(response) == 0 {
		return false
	}
	if errValue, ok := response["error"]; ok && errValue != nil {
		return false
	}
	_, hasResult := response["result"]
	return hasResult
}

func newDocsMCPGateway(expectations []docsServerExpectation) *httptest.Server {
	fixtures := make(map[string]docsServerExpectation, len(expectations))
	for _, expected := range expectations {
		fixtures[expected.ServerID] = expected
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		rows := make([]any, 0, len(expectations))
		for idx, expected := range expectations {
			endpoint := endpointForServer(server.URL, expected.ServerID)
			row := map[string]any{
				"server": map[string]any{
					"$schema":     "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
					"name":        firstNonEmpty(expected.ServerName, expected.ServerID),
					"description": firstNonEmpty(expected.ServerName, expected.ServerID),
					"remotes": []map[string]any{
						{
							"type": "streamable-http",
							"url":  endpoint,
						},
					},
				},
				"_meta": map[string]any{
					"com.dingtalk.mcp.registry/metadata": map[string]any{
						"status":      expected.ServerStatus,
						"updatedAt":   "2026-03-22T00:00:00Z",
						"publishedAt": "2026-03-22T00:00:00Z",
						"mcpId":       idx + 1,
					},
					"com.dingtalk.mcp.registry/cli": map[string]any{
						"id":      expected.ServerID,
						"command": firstNonEmpty(expected.Command, expected.ServerID),
					},
				},
			}
			rows = append(rows, row)
		}

		payload := map[string]any{
			"metadata": map[string]any{
				"count": len(rows),
			},
			"servers": rows,
		}
		writeJSON(w, payload)
	})

	mux.HandleFunc("/server/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		serverID := strings.TrimPrefix(r.URL.Path, "/server/")
		fixture, ok := fixtures[serverID]
		if !ok {
			http.NotFound(w, r)
			return
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		method := stringValue(req["method"])
		switch method {
		case "initialize":
			writeJSON(w, valueOrFallback(fixture.InitializeResponse, map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
					"serverInfo":      map[string]any{"name": fixture.ServerID, "version": "1.0.0"},
				},
			}))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeJSON(w, valueOrFallback(fixture.ToolsListResponse, map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"result":  map[string]any{"tools": []any{}},
			}))
		default:
			http.Error(w, "unexpected json-rpc method", http.StatusBadRequest)
		}
	})

	return server
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func valueOrFallback(value, fallback any) any {
	if value == nil {
		return fallback
	}
	return value
}

func copyMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func firstNonEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func endpointForServer(baseURL, serverID string) string {
	return strings.TrimRight(baseURL, "/") + "/server/" + serverID
}

func mapValue(value any) map[string]any {
	out, _ := value.(map[string]any)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func sliceValue(value any) ([]any, bool) {
	out, ok := value.([]any)
	if !ok {
		return nil, false
	}
	return out, true
}

func firstPrefix(value any) string {
	values, ok := sliceValue(value)
	if !ok {
		return ""
	}
	for _, raw := range values {
		if v := strings.TrimSpace(stringValue(raw)); v != "" {
			return v
		}
	}
	return ""
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/ir"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/spf13/pflag"
)

type docsToolExpectation struct {
	Name             string
	Flags            []string
	UnsupportedProps []string
}

type docsServerExpectation struct {
	File               string
	ServerID           string
	ServerName         string
	ServerStatus       string
	Command            string
	InitializeResponse any
	ToolsListResponse  any
	Tools              []docsToolExpectation
}

func TestMCPProtocolDocsCoverageViaEndpointGeneratedCLI(t *testing.T) {
	t.Parallel()

	expectations, err := loadDocsMCPExpectations(filepath.Join("..", "..", "docs", "mcp"))
	if err != nil {
		t.Fatalf("loadDocsMCPExpectations() error = %v", err)
	}
	if len(expectations) == 0 {
		t.Skip("no docs/mcp expectations loaded; skipping")
	}

	mock := newDocsMCPGateway(t, expectations)
	defer mock.Close()

	service := discovery.NewService(
		market.NewClient(mock.URL, mock.Client()),
		transport.NewClient(mock.Client()),
		cache.NewStore(t.TempDir()),
	)

	servers, err := service.DiscoverServers(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServers() error = %v", err)
	}

	runtimeServers, failures := service.DiscoverAllRuntime(context.Background(), servers)

	expectedFailureKeys := expectedRuntimeFailureKeys(mock.URL, expectations)
	if len(failures) != len(expectedFailureKeys) {
		t.Fatalf("DiscoverAllRuntime() failures len = %d, want %d", len(failures), len(expectedFailureKeys))
	}
	unexpectedFailures := make([]string, 0)
	for _, failure := range failures {
		if _, ok := expectedFailureKeys[failure.ServerKey]; !ok {
			unexpectedFailures = append(unexpectedFailures, fmt.Sprintf("%s (%v)", failure.ServerKey, failure.Err))
		}
	}
	if len(unexpectedFailures) > 0 {
		sort.Strings(unexpectedFailures)
		t.Fatalf("unexpected runtime failures: %s", strings.Join(unexpectedFailures, "; "))
	}

	catalog := ir.BuildCatalog(runtimeServers)
	productsByServerKey := make(map[string]ir.CanonicalProduct, len(catalog.Products))
	for _, product := range catalog.Products {
		productsByServerKey[product.ServerKey] = product
	}

	missingProducts := make([]string, 0)
	missingCommands := make([]string, 0)
	missingFlags := make([]string, 0)
	missingPayloadFallback := make([]string, 0)
	traversedCommands := 0
	expectedCommandCount := 0
	unsupportedParamTools := 0

	for _, expected := range expectations {
		if len(expected.Tools) == 0 {
			continue
		}
		expectedCommandCount += len(expected.Tools)

		serverKey := market.ServerKey(market.NormalizeEndpoint(endpointForServer(mock.URL, expected.ServerID)))
		product, ok := productsByServerKey[serverKey]
		if !ok {
			missingProducts = append(missingProducts, expected.ServerID)
			continue
		}

		for _, tool := range expected.Tools {
			toolCommandName := tool.Name
			if canonicalTool, found := product.FindTool(tool.Name); found {
				if cliName := strings.TrimSpace(canonicalTool.CLIName); cliName != "" {
					toolCommandName = cliName
				}
			}

			flags, helpOutput, inspectErr := inspectToolCommand(catalog, product.ID, toolCommandName)
			if inspectErr != nil {
				missingCommands = append(missingCommands, fmt.Sprintf("%s.%s (%v)", product.ID, toolCommandName, inspectErr))
				continue
			}
			traversedCommands++

			for _, expectedFlag := range tool.Flags {
				if _, ok := flags[expectedFlag]; !ok {
					missingFlags = append(missingFlags, fmt.Sprintf("%s.%s --%s", product.ID, toolCommandName, expectedFlag))
					continue
				}
				if !strings.Contains(helpOutput, "--"+expectedFlag) {
					missingFlags = append(missingFlags, fmt.Sprintf("%s.%s --%s(help)", product.ID, toolCommandName, expectedFlag))
				}
			}
			if len(tool.UnsupportedProps) > 0 {
				unsupportedParamTools++
				if _, ok := flags["json"]; !ok {
					missingPayloadFallback = append(missingPayloadFallback, fmt.Sprintf("%s.%s --json", product.ID, toolCommandName))
				}
				if _, ok := flags["params"]; !ok {
					missingPayloadFallback = append(missingPayloadFallback, fmt.Sprintf("%s.%s --params", product.ID, toolCommandName))
				}
				if !strings.Contains(helpOutput, "--json") {
					missingPayloadFallback = append(missingPayloadFallback, fmt.Sprintf("%s.%s --json(help)", product.ID, toolCommandName))
				}
				if !strings.Contains(helpOutput, "--params") {
					missingPayloadFallback = append(missingPayloadFallback, fmt.Sprintf("%s.%s --params(help)", product.ID, toolCommandName))
				}
			}
		}
	}

	if len(missingProducts) > 0 {
		sort.Strings(missingProducts)
		t.Fatalf("missing products for docs/mcp coverage: %s", strings.Join(missingProducts, ", "))
	}
	if len(missingCommands) > 0 {
		sort.Strings(missingCommands)
		t.Fatalf("missing commands for docs/mcp coverage: %s", strings.Join(missingCommands, "; "))
	}
	if len(missingFlags) > 0 {
		sort.Strings(missingFlags)
		t.Fatalf("missing parameter flags for docs/mcp coverage: %s", strings.Join(missingFlags, "; "))
	}
	if len(missingPayloadFallback) > 0 {
		sort.Strings(missingPayloadFallback)
		t.Fatalf("missing payload fallback flags for complex params: %s", strings.Join(missingPayloadFallback, "; "))
	}

	t.Logf("docs/mcp coverage ok: servers=%d runtime_ok=%d runtime_fail=%d commands=%d traversed=%d tools_with_complex_params=%d", len(expectations), len(runtimeServers), len(failures), expectedCommandCount, traversedCommands, unsupportedParamTools)
}

func inspectToolCommand(catalog ir.Catalog, productID, toolCommand string) (map[string]struct{}, string, error) {
	root := cli.NewMCPCommand(context.Background(), cli.StaticLoader{Catalog: catalog}, executor.EchoRunner{}, nil)

	toolCmd, _, err := root.Find([]string{productID, toolCommand})
	if err != nil {
		return nil, "", err
	}
	if toolCmd == root {
		return nil, "", fmt.Errorf("tool command not found")
	}

	flags := make(map[string]struct{})
	toolCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		flags[flag.Name] = struct{}{}
	})

	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{productID, toolCommand, "--help"})
	if err := root.Execute(); err != nil {
		return nil, output.String(), err
	}
	return flags, output.String(), nil
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
		if len(server) == 0 {
			// Skip aggregate market snapshots such as docs/mcp/server.json.
			continue
		}
		serverID := stringValue(server["id"])
		if serverID == "" {
			serverID = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		}
		command := firstPrefix(server["prefix"])
		if command == "" {
			command = serverID
		}

		initializeResponse := firstInitializeResponse(payload)
		toolsListResponse := firstMethodResponse(payload, "tools/list")
		tools := extractToolsFromToolsListResponse(toolsListResponse)

		out = append(out, docsServerExpectation{
			File:               filepath.Base(file),
			ServerID:           serverID,
			ServerName:         stringValue(server["name"]),
			ServerStatus:       normalizeStatus(stringValue(server["status"])),
			Command:            command,
			InitializeResponse: initializeResponse,
			ToolsListResponse:  toolsListResponse,
			Tools:              tools,
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

func firstInitializeResponse(payload map[string]any) any {
	initialize := mapValue(payload["initialize"])
	attempts, ok := sliceValue(initialize["attempts"])
	if !ok || len(attempts) == 0 {
		return nil
	}
	attempt := mapValue(attempts[0])
	return attempt["response"]
}

func firstMethodResponse(payload map[string]any, method string) any {
	methods := mapValue(payload["methods"])
	methodPayload := mapValue(methods[method])
	pages, ok := sliceValue(methodPayload["pages"])
	if !ok || len(pages) == 0 {
		return nil
	}
	firstPage := mapValue(pages[0])
	return firstPage["response"]
}

func extractToolsFromToolsListResponse(response any) []docsToolExpectation {
	responseMap := mapValue(response)
	result := mapValue(responseMap["result"])
	toolsRaw, ok := sliceValue(result["tools"])
	if !ok {
		return nil
	}

	tools := make([]docsToolExpectation, 0, len(toolsRaw))
	for _, item := range toolsRaw {
		tool := mapValue(item)
		name := stringValue(tool["name"])
		if name == "" {
			name = stringValue(tool["toolName"])
		}
		if name == "" {
			continue
		}

		inputSchema := mapValue(tool["inputSchema"])
		specs := cli.BuildFlagSpecs(inputSchema, nil)
		flagByProperty := make(map[string]string, len(specs))
		for _, spec := range specs {
			flagByProperty[spec.PropertyName] = spec.FlagName
		}

		properties := mapValue(inputSchema["properties"])
		flags := make([]string, 0, len(flagByProperty))
		unsupported := make([]string, 0)
		for property := range properties {
			if flag, exists := flagByProperty[property]; exists {
				flags = append(flags, flag)
				continue
			}
			unsupported = append(unsupported, property)
		}
		sort.Strings(flags)
		sort.Strings(unsupported)

		tools = append(tools, docsToolExpectation{
			Name:             name,
			Flags:            flags,
			UnsupportedProps: unsupported,
		})
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

func expectedRuntimeFailureKeys(baseURL string, expectations []docsServerExpectation) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, expected := range expectations {
		if len(expected.Tools) > 0 {
			continue
		}
		endpoint := endpointForServer(baseURL, expected.ServerID)
		keys[market.ServerKey(market.NormalizeEndpoint(endpoint))] = struct{}{}
	}
	return keys
}

func endpointForServer(baseURL, serverID string) string {
	return strings.TrimRight(baseURL, "/") + "/server/" + serverID
}

func newDocsMCPGateway(t *testing.T, expectations []docsServerExpectation) *httptest.Server {
	t.Helper()

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
		_ = json.NewEncoder(w).Encode(payload)
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

func firstNonEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
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

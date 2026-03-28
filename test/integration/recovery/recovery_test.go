package recovery_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
)

func TestRecoveryClosedLoopDoesNotRecursivelyCaptureExecuteFailures(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	t.Setenv("DWS_TRUSTED_DOMAINS", "*")

	server, calls := newRecoveryRuntimeServer(t)
	defer server.Close()

	t.Setenv("DINGTALK_AITABLE_MCP_URL", server.URL+"/server/aitable")
	t.Setenv("DINGTALK_DEVDOC_MCP_URL", server.URL+"/server/devdoc")
	t.Setenv(cli.CatalogFixtureEnv, writeRecoveryCatalogFixture(t, server.URL))

	root := app.NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"aitable", "base", "get", "--base-id", "BASE_MISSING", "-f", "json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute(aitable base get) error = nil, want captured runtime failure")
	}

	store := recovery.NewStore(configDir)
	last, err := store.LoadLastError()
	if err != nil {
		t.Fatalf("LoadLastError() error = %v", err)
	}
	if last.EventID == "" {
		t.Fatal("expected captured event id")
	}
	if got := strings.Join(last.Context.CommandPath, " "); got != "aitable base get" {
		t.Fatalf("captured command path = %q, want \"aitable base get\"", got)
	}

	eventsPath := filepath.Join(configDir, "recovery", "recovery_events.jsonl")
	beforeExecute, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile(recovery_events.jsonl) error = %v", err)
	}
	if got := countPhase(beforeExecute, "captured"); got != 1 {
		t.Fatalf("captured phase count before execute = %d, want 1", got)
	}

	root = app.NewRootCommand()
	var executeOut bytes.Buffer
	root.SetOut(&executeOut)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"recovery", "execute", "--event-id", last.EventID, "-f", "json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(recovery execute) error = %v", err)
	}

	var bundle recovery.RecoveryBundle
	if err := json.Unmarshal(executeOut.Bytes(), &bundle); err != nil {
		t.Fatalf("json.Unmarshal(bundle) error = %v\noutput:\n%s", err, executeOut.String())
	}
	if bundle.EventID != last.EventID {
		t.Fatalf("bundle.EventID = %q, want %q", bundle.EventID, last.EventID)
	}
	if bundle.Status != "analysis_failed" {
		t.Fatalf("bundle.Status = %q, want analysis_failed (doc_search=%#v probes=%#v)", bundle.Status, bundle.DocSearch, bundle.ProbeResults)
	}
	if bundle.DocSearch.Status != "error" {
		t.Fatalf("bundle.DocSearch.Status = %q, want error", bundle.DocSearch.Status)
	}

	foundProbe := false
	for _, probe := range bundle.ProbeResults {
		if probe.Name != "aitable_base_catalog_probe" {
			continue
		}
		foundProbe = true
		if probe.Status != "success" {
			t.Fatalf("probe.Status = %q, want success", probe.Status)
		}
		if probe.ToolName != "list_bases" {
			t.Fatalf("probe.ToolName = %q, want list_bases", probe.ToolName)
		}
	}
	if !foundProbe {
		t.Fatalf("bundle.ProbeResults = %#v, want aitable_base_catalog_probe", bundle.ProbeResults)
	}

	afterExecute, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile(recovery_events.jsonl after execute) error = %v", err)
	}
	if got := countPhase(afterExecute, "captured"); got != 1 {
		t.Fatalf("captured phase count after execute = %d, want 1", got)
	}
	if got := countPhase(afterExecute, "analyzed"); got != 1 {
		t.Fatalf("analyzed phase count after execute = %d, want 1", got)
	}

	executionFile := filepath.Join(configDir, "execution.json")
	if err := os.WriteFile(executionFile, []byte(`{"actions":["inspect_bundle","handoff"],"attempts":[{"command_summary":"dws recovery execute --event-id EVT --format json","result":"failed","error_summary":"probe list_bases failed","source":"agent_analysis"}],"result":"handoff","error_summary":"aitable base catalog probe still failing"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(execution.json) error = %v", err)
	}

	root = app.NewRootCommand()
	var finalizeOut bytes.Buffer
	root.SetOut(&finalizeOut)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"recovery", "finalize",
		"--event-id", last.EventID,
		"--outcome", "handoff",
		"--execution-file", executionFile,
		"-f", "json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(recovery finalize) error = %v", err)
	}
	if !strings.Contains(finalizeOut.String(), `"execution_recorded": true`) {
		t.Fatalf("finalize output missing execution_recorded:\n%s", finalizeOut.String())
	}

	afterFinalize, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile(recovery_events.jsonl after finalize) error = %v", err)
	}
	if got := countPhase(afterFinalize, "captured"); got != 1 {
		t.Fatalf("captured phase count after finalize = %d, want 1", got)
	}
	if got := countPhase(afterFinalize, "finalized"); got != 1 {
		t.Fatalf("finalized phase count after finalize = %d, want 1", got)
	}

	if got := calls.getBase.Load(); got != 1 {
		t.Fatalf("get_base calls = %d, want 1", got)
	}
	if got := calls.listBases.Load(); got != 1 {
		t.Fatalf("list_bases calls = %d, want 1", got)
	}
	if got := calls.docSearch.Load(); got < 1 {
		t.Fatalf("search_open_platform_docs calls = %d, want at least 1", got)
	}
}

type recoveryRuntimeCalls struct {
	getBase   atomic.Int32
	listBases atomic.Int32
	docSearch atomic.Int32
}

func newRecoveryRuntimeServer(t *testing.T) (*httptest.Server, *recoveryRuntimeCalls) {
	t.Helper()

	calls := &recoveryRuntimeCalls{}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	mux.HandleFunc("/server/aitable", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch req["method"] {
		case "initialize":
			writeJSONRPCResult(t, w, req["id"], map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "aitable", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			writeJSONRPCResult(t, w, req["id"], map[string]any{
				"tools": []map[string]any{
					{
						"name":        "get_base",
						"title":       "Get Base",
						"description": "Get base",
						"inputSchema": map[string]any{"type": "object"},
					},
					{
						"name":        "list_bases",
						"title":       "List Bases",
						"description": "List bases",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			name, _ := params["name"].(string)
			switch name {
			case "get_base":
				calls.getBase.Add(1)
				writeJSONRPCResult(t, w, req["id"], map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "unexpected upstream failure while reading base"},
					},
					"isError": true,
				})
			case "list_bases":
				calls.listBases.Add(1)
				writeJSONRPCResult(t, w, req["id"], map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": `{"items":[{"baseId":"BASE_001","name":"项目台账"}]}`},
					},
				})
			default:
				t.Fatalf("unexpected aitable tool %q", name)
			}
		default:
			t.Fatalf("unexpected aitable method %q", req["method"])
		}
	})

	mux.HandleFunc("/server/devdoc", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch req["method"] {
		case "initialize":
			writeJSONRPCResult(t, w, req["id"], map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "devdoc", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
		case "tools/list":
			writeJSONRPCResult(t, w, req["id"], map[string]any{
				"tools": []map[string]any{
					{
						"name":        "search_open_platform_docs",
						"title":       "Search Docs",
						"description": "Search docs",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			name, _ := params["name"].(string)
			if name != "search_open_platform_docs" {
				t.Fatalf("unexpected devdoc tool %q", name)
			}
			calls.docSearch.Add(1)
			http.Error(w, "upstream doc search failed", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected devdoc method %q", req["method"])
		}
	})

	return server, calls
}

func writeJSONRPCResult(t *testing.T, w http.ResponseWriter, id any, result map[string]any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}); err != nil {
		t.Fatalf("json.NewEncoder() error = %v", err)
	}
}

func writeRecoveryCatalogFixture(t *testing.T, baseURL string) string {
	t.Helper()

	payload := map[string]any{
		"products": []any{
			map[string]any{
				"id":           "aitable",
				"display_name": "AI表格",
				"server_key":   "aitable-fixture",
				"endpoint":     baseURL + "/server/aitable",
				"tools": []any{
					map[string]any{
						"rpc_name":       "get_base",
						"canonical_path": "aitable.get_base",
					},
					map[string]any{
						"rpc_name":       "list_bases",
						"canonical_path": "aitable.list_bases",
					},
				},
			},
			map[string]any{
				"id":           "devdoc",
				"display_name": "开放平台文档",
				"server_key":   "devdoc-fixture",
				"endpoint":     baseURL + "/server/devdoc",
				"tools": []any{
					map[string]any{
						"rpc_name":       "search_open_platform_docs",
						"canonical_path": "devdoc.search_open_platform_docs",
					},
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(catalog.json) error = %v", err)
	}
	return path
}

func countPhase(data []byte, phase string) int {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, `"phase":"`+phase+`"`) {
			count++
		}
	}
	return count
}

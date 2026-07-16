package mock_mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

const mockMCPSmokeHelperEnv = "DWS_MOCK_MCP_SMOKE_HELPER"

type recordedToolCall struct {
	path          string
	method        string
	authorization string
	jsonrpc       string
	tool          string
	arguments     map[string]any
	err           error
}

// TestCLIHelperProcess runs the production CLI entrypoint with real os.Args.
// The parent test supplies only isolated temp directories, a loopback endpoint,
// and a synthetic token accepted by the local fake server.
func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv(mockMCPSmokeHelperEnv) != "1" {
		return
	}

	marker := -1
	for i, arg := range os.Args {
		if arg == "--" {
			marker = i
			break
		}
	}
	if marker < 0 {
		fmt.Fprintln(os.Stderr, "Mock MCP smoke helper: missing -- argument marker")
		os.Exit(2)
	}
	os.Args = append([]string{"dws"}, os.Args[marker+1:]...)
	os.Exit(app.Execute())
}

func TestMockMCPSmoke_CLIRoutesSerializedArgumentsAndPrintsJSON(t *testing.T) {
	var requestsMu sync.Mutex
	var requests []recordedToolCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := recordedToolCall{
			path:          r.URL.Path,
			method:        r.Method,
			authorization: r.Header.Get("Authorization"),
		}

		var envelope struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Method  string `json:"method"`
			Params  struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			call.err = err
		} else {
			call.jsonrpc = envelope.JSONRPC
			call.tool = envelope.Params.Name
			call.arguments = envelope.Params.Arguments
			if envelope.Method != "tools/call" {
				call.err = fmt.Errorf("JSON-RPC method = %q, want tools/call", envelope.Method)
			}
		}
		requestsMu.Lock()
		requests = append(requests, call)
		requestsMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      envelope.ID,
			"result": map[string]any{
				"content": []map[string]any{{
					"type": "text",
					"text": `{"success":true,"result":[{"userId":"mock-user-1","name":"Local Mock"}]}`,
				}},
			},
		})
	}))
	defer server.Close()

	env := isolatedCLIEnv(t, map[string]string{
		"DINGTALK_CONTACT_MCP_URL": server.URL + "/mcp/contact",
	})
	args := []string{
		"--token", "ci-smoke-token",
		"--format", "json",
		"contact", "user", "get",
		"--ids", "user-001,user-002",
	}
	stdout, stderr, err := runCLI(t, env, args...)
	if err != nil {
		t.Fatalf("dws %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}

	requestsMu.Lock()
	recorded := append([]recordedToolCall(nil), requests...)
	requestsMu.Unlock()
	if len(recorded) != 1 {
		t.Fatalf("local fake MCP server received %d requests, want exactly one tools/call: %#v", len(recorded), recorded)
	}
	call := recorded[0]
	if call.err != nil {
		t.Fatal(call.err)
	}
	if call.path != "/mcp/contact" {
		t.Fatalf("request path = %q, want /mcp/contact", call.path)
	}
	if call.method != http.MethodPost {
		t.Fatalf("HTTP method = %q, want POST", call.method)
	}
	if call.authorization != "Bearer ci-smoke-token" {
		t.Fatalf("Authorization = %q, want synthetic smoke token", call.authorization)
	}
	if call.jsonrpc != "2.0" {
		t.Fatalf("jsonrpc = %q, want 2.0", call.jsonrpc)
	}
	if call.tool != "get_user_info_by_user_ids" {
		t.Fatalf("tool = %q, want get_user_info_by_user_ids", call.tool)
	}
	wantArgs := map[string]any{"user_id_list": []any{"user-001", "user-002"}}
	if !reflect.DeepEqual(call.arguments, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", call.arguments, wantArgs)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("CLI returned non-JSON stdout: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if payload["success"] != true {
		t.Fatalf("CLI success = %#v, want true; payload=%#v", payload["success"], payload)
	}
	result, ok := payload["result"].([]any)
	if !ok || len(result) != 1 {
		t.Fatalf("CLI result = %#v, want one mock user", payload["result"])
	}
	user, _ := result[0].(map[string]any)
	if user["userId"] != "mock-user-1" {
		t.Fatalf("CLI userId = %#v, want mock-user-1; payload=%#v", user["userId"], payload)
	}
}

func isolatedCLIEnv(t *testing.T, extra map[string]string) []string {
	t.Helper()

	root := t.TempDir()
	controlled := map[string]string{
		"HOME":                     root,
		"USERPROFILE":              root,
		"DWS_CONFIG_DIR":           filepath.Join(root, "config"),
		"DWS_KEYCHAIN_DIR":         filepath.Join(root, "keychain"),
		"DWS_DISABLE_KEYCHAIN":     "1",
		"HTTP_PROXY":               "http://127.0.0.1:1",
		"HTTPS_PROXY":              "http://127.0.0.1:1",
		"http_proxy":               "http://127.0.0.1:1",
		"https_proxy":              "http://127.0.0.1:1",
		"NO_PROXY":                 "127.0.0.1,localhost,::1",
		"no_proxy":                 "127.0.0.1,localhost,::1",
		mockMCPSmokeHelperEnv:      "1",
		"DWS_ALLOW_HTTP_ENDPOINTS": "1",
		"DWS_TRUSTED_DOMAINS":      "127.0.0.1,localhost,::1",
	}
	for key, value := range extra {
		controlled[key] = value
	}

	env := make([]string, 0, len(controlled)+8)
	for _, key := range []string{"PATH", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "TZ", "SYSTEMROOT"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	for key, value := range controlled {
		env = append(env, key+"="+value)
	}
	sort.Strings(env)
	return env
}

func runCLI(t *testing.T, env []string, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	processArgs := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], processArgs...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("dws %s timed out: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), ctx.Err(), stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String(), err
}

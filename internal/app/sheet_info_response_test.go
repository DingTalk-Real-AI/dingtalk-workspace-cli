package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// Exercise the real command, runner, HTTP transport, adapter and error renderer.
// A helper-only caller would miss the typed-nil map converted to text "null".
func TestCrossPlatformCoverageSheetInfoHTTPResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content []map[string]any
		success bool
	}{
		{"null", []map[string]any{{"type": "text", "text": "null"}}, false},
		{"array", []map[string]any{{"type": "text", "text": "[]"}}, false},
		{"plain text", []map[string]any{{"type": "text", "text": "service busy"}}, false},
		{"empty text", []map[string]any{{"type": "text", "text": ""}}, false},
		{"empty content", []map[string]any{}, false},
		{"no text", []map[string]any{{"type": "image", "data": "fixture"}}, false},
		{"normal object", []map[string]any{{"type": "text", "text": `{"sheetId":"s1","lastNonEmptyRow":1,"lastNonEmptyColumn":2}`}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				calls.Add(1)
				if request.Method != "tools/call" || request.Params.Name != "get_sheet" || request.Params.Arguments["nodeId"] != "n1" || request.Params.Arguments["sheetId"] != "Sheet1" {
					t.Errorf("unexpected request: %#v", request)
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": request.ID,
					"result": map[string]any{"content": tc.content},
				}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Cleanup(CloseFileLogger)
			t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
			t.Setenv("DWS_TRUSTED_DOMAINS", "127.0.0.1")
			t.Setenv("DINGTALK_SHEET_MCP_URL", server.URL)
			args := []string{"--token", "synthetic-sheet-token", "sheet", "info", "--node", "n1", "--sheet-id", "Sheet1", "--format", "json"}
			testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
			helpers.InitDepsForTest(t, helpers.GetCaller())
			root := NewRootCommand()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			helpers.GetFormatter().SetWriters(&stdout, &stderr)
			root.SetArgs(args)
			leaf, err := root.ExecuteC()
			if calls.Load() != 1 {
				t.Fatalf("HTTP calls=%d, want one get_sheet", calls.Load())
			}
			if tc.success {
				if err != nil {
					t.Fatal(err)
				}
				var got struct {
					SheetID       string `json:"sheetId"`
					NonEmptyRange struct {
						Range    string `json:"range"`
						LastCell string `json:"lastCell"`
					} `json:"nonEmptyRange"`
				}
				if e := json.Unmarshal(stdout.Bytes(), &got); e != nil || got.SheetID != "s1" || got.NonEmptyRange.Range != "A1:C2" || got.NonEmptyRange.LastCell != "C2" || stderr.Len() != 0 {
					t.Fatalf("normal response changed: stdout=%s stderr=%s error=%v", &stdout, &stderr, e)
				}
				return
			}
			var failure *apperrors.Error
			if !errors.As(err, &failure) || failure.Category != apperrors.CategoryAPI || failure.Reason != helpers.CodeMCPToolError || apperrors.ExitCode(err) != 1 || failure.Operation != "sheet/get_sheet" || stdout.Len() != 0 {
				t.Fatalf("invalid response accepted or misclassified: error=%v stdout=%s", err, &stdout)
			}
			if err := printExecutionError(leaf, &stdout, &stderr, err); err != nil {
				t.Fatal(err)
			}
			// sheet info retains its legacy error contract: one JSON error on stderr.
			var wire struct {
				Error struct {
					Code      int    `json:"code"`
					Category  string `json:"category"`
					Reason    string `json:"reason"`
					Operation string `json:"operation"`
				} `json:"error"`
			}
			if e := json.Unmarshal(stderr.Bytes(), &wire); e != nil || wire.Error.Code != 1 || wire.Error.Category != "api" || wire.Error.Reason != helpers.CodeMCPToolError || wire.Error.Operation != "sheet/get_sheet" || stdout.Len() != 0 || strings.Contains(stderr.String(), "internal panic") {
				t.Fatalf("invalid CLI error: stdout=%s stderr=%s error=%v", &stdout, &stderr, e)
			}
		})
	}
}

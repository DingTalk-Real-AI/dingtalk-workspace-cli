package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// A server that ignores suggestion may perform an ordinary edit and return its
// ordinary receipt. The client must preserve it, without retrying or requiring
// suggestionId/applied fields. New sequence/range formats have separate receipt
// requirements; these fixtures deliberately use the existing simple formats.
func TestCrossPlatformCoverageDocSuggestionLegacyReceiptsAndAgentSource(t *testing.T) {
	for _, command := range []struct {
		path string
		args []string
		tool string
	}{
		{"read", []string{"doc", "read", "--node", "node"}, "get_document_content"},
		{"insert", []string{"doc", "block", "insert", "--node", "node", "--content", "body"}, "insert_document_block"},
		{"update", []string{"doc", "block", "update", "--node", "node", "--block-id", "block", "--content", "body"}, "update_document_block"},
		{"delete", []string{"doc", "block", "delete", "--node", "node", "--block-id", "block", "--yes"}, "delete_document_block"},
	} {
		for _, selector := range []string{"", "--suggestion", "--suggestion=true", "--suggestion=false"} {
			t.Run(command.path+selector, func(t *testing.T) {
				capture, err := os.CreateTemp(t.TempDir(), "doc-rollout-output-*.log")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = capture.Close() })
				testseam.Swap(t, &os.Stdout, capture)
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				// Cleanup is LIFO: close the runtime log before removing its directory.
				t.Cleanup(CloseFileLogger)
				t.Setenv("DO_NOT_TRACK", "1")
				t.Setenv("DINGTALK_AGENT", " Future-Agent ")
				t.Setenv("DINGTALK_DOC_MCP_URL", "https://mcp-gw.dingtalk.com/server/doc-rollout-test")
				args := append([]string{}, command.args...)
				if selector != "" {
					args = append(args, selector)
				}
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				helpers.InitDepsForTest(t, helpers.GetCaller())
				previousProfile := authpkg.RuntimeProfile()
				authpkg.SetRuntimeProfile("")
				t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })
				testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{})
				prefetchDone := make(chan struct{})
				testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) {
					defer close(prefetchDone)
					return "test-token", nil
				})
				// Cleanup runs in reverse registration order. Join the background
				// reader before testseam restores its process-global function.
				t.Cleanup(func() {
					select {
					case <-prefetchDone:
					case <-time.After(5 * time.Second):
						t.Error("token prefetch did not finish before restoring its stub")
					}
				})
				testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
					return AccessTokenSnapshot{AccessToken: "test-token"}, nil
				})
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					var request struct {
						ID     any    `json:"id"`
						Method string `json:"method"`
						Params struct {
							Name      string         `json:"name"`
							Arguments map[string]any `json:"arguments"`
						} `json:"params"`
					}
					if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
						return nil, err
					}
					calls++
					if request.Method != "tools/call" || request.Params.Name != command.tool {
						t.Errorf("unexpected request: %s %s", request.Method, request.Params.Name)
					}
					a := request.Params.Arguments
					wantSuggestion := selector == "--suggestion" || selector == "--suggestion=true"
					value, exists := a["suggestion"]
					if exists != wantSuggestion || exists && value != true {
						t.Errorf("suggestion presence/value = %t/%v; selector=%q", exists, value, selector)
					}
					if command.tool == "get_document_content" {
						if _, exists := a["extraInfo"]; exists {
							t.Errorf("read acquired write source: %#v", a)
						}
					} else {
						extra, _ := a["extraInfo"].(map[string]any)
						if extra["agentName"] != "Future-Agent" || len(extra) != 1 {
							t.Errorf("write lost Agent source: %#v", a)
						}
					}
					text := `{"success":true,"nodeId":"node","blockId":"block","marker":"ordinary-reply"}`
					body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}})
					if err != nil {
						return nil, err
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body)), Request: req}, nil
				})}
				testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
					return &runtimeRunner{transport: transport.NewClient(client), globalFlags: flags, auditSink: audit.NopSink{}}
				})
				root := NewRootCommand()
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				helpers.GetFormatter().SetWriters(&stdout, &stderr)
				root.SetArgs(args)
				if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
					t.Fatal(err)
				}
				if err := root.Execute(); err != nil {
					t.Fatalf("ordinary receipt rejected: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
				}
				captured, err := os.ReadFile(capture.Name())
				if err != nil {
					t.Fatal(err)
				}
				stdout.Write(captured)
				if calls != 1 || !strings.Contains(stdout.String(), "ordinary-reply") {
					t.Fatalf("calls=%d stdout=%s stderr=%s; want one RPC and retained ordinary receipt", calls, stdout.String(), stderr.String())
				}
			})
		}
	}
}

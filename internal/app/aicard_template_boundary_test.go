// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const aicardBoundaryTemplate = `[{"version":"v1.0","createSurface":{"surfaceId":"s","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{},"components":[{"id":"root","component":"Text","text":"Report"}]}}]`

type aicardBoundaryCaller struct{ calls int }

func (*aicardBoundaryCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return nil, errors.New("unexpected ordinary tool invocation")
}
func (*aicardBoundaryCaller) Format() string { return "json" }
func (*aicardBoundaryCaller) DryRun() bool   { return false }
func (*aicardBoundaryCaller) Fields() string { return "" }
func (*aicardBoundaryCaller) JQ() string     { return "" }
func (c *aicardBoundaryCaller) InvokeAicardTemplate(context.Context, string, string, map[string]any) (publishedmcp.ValidatedInvocationResult, error) {
	c.calls++
	return publishedmcp.ValidatedInvocationResult{Result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": aicardBoundaryTemplate}}}, nil
}

func TestCrossPlatformCoverageAicardTemplateRootOutputDestinations(t *testing.T) {
	for _, name := range []string{"same", "normalized", "relative", "symlink-parent", "case-variant", "different", "existing", "dry-run"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", filepath.Join(dir, "config"))
			// The root command opens <config>/logs/dws.log; close it before TempDir
			// cleanup, which Windows cannot do while the file is open.
			t.Cleanup(CloseFileLogger)
			path := filepath.Join(dir, "card.json")
			out := path
			conflict, wantCalls := true, 0
			switch name {
			case "normalized":
				out = dir + string(os.PathSeparator) + "." + string(os.PathSeparator) + "card.json"
			case "relative":
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				out, err = filepath.Rel(cwd, path)
				if err != nil {
					t.Skipf("relative paths cross volumes: %v", err)
				}
			case "symlink-parent":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(dir, alias); err != nil {
					t.Skipf("directory symlinks unavailable: %v", err)
				}
				out = filepath.Join(alias, "card.json")
			case "case-variant":
				if err := os.WriteFile(filepath.Join(dir, "SensitivityProbe"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				_, statErr := os.Stat(filepath.Join(dir, "sensitivityprobe"))
				conflict = statErr == nil
				out, wantCalls = filepath.Join(dir, "CARD.JSON"), 1
			case "different":
				out, conflict, wantCalls = filepath.Join(dir, "result.json"), false, 1
			case "existing":
				if err := os.WriteFile(path, []byte("preserve-existing"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			root := NewRootCommand(t.Context())
			caller := &aicardBoundaryCaller{}
			helpers.InitDepsForTest(t, caller)
			var stream bytes.Buffer
			root.SetOut(&stream)
			root.SetErr(&stream)
			args := []string{"aicard", "template", "get", "--mcp-id", "fixture-service", "--template-id", "t", "--version", "1", "--out", path, "--output", out, "--format", "json"}
			if name == "dry-run" {
				args = append(args, "--dry-run")
			}
			root.SetArgs(args)
			_, err := corecmd.ExecuteContextCForTest(root, t.Context())
			if caller.calls != wantCalls {
				t.Fatalf("calls=%d want=%d err=%v", caller.calls, wantCalls, err)
			}
			if conflict {
				if err == nil || apperrors.ExitCode(err) != 3 {
					t.Fatalf("conflict must fail validation: %v", err)
				}
				if name == "existing" {
					data, readErr := os.ReadFile(path)
					if readErr != nil || string(data) != "preserve-existing" {
						t.Fatalf("existing file changed: %q %v", data, readErr)
					}
				} else if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("conflict published a file: %v", statErr)
				}
				return
			}
			data, readErr := os.ReadFile(path)
			if err != nil || readErr != nil || string(data) != aicardBoundaryTemplate {
				t.Fatalf("template changed: %v %v %q", err, readErr, data)
			}
			data, readErr = os.ReadFile(out)
			var receipt map[string]any
			if readErr != nil || json.Unmarshal(data, &receipt) != nil || receipt["ok"] != true {
				t.Fatalf("missing separate receipt: %v %s", readErr, data)
			}
		})
	}
}

func aicardBoundaryAuth(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "local-test-token"}, nil
	})
	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
}

func TestCrossPlatformCoverageAicardTemplateRootDiscoveryTimeout(t *testing.T) {
	for _, shortParent := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured", true: "shorter-parent"}[shortParent], func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Cleanup(CloseFileLogger)
			t.Setenv("DINGTALK_MCP_META_MCP_URL", "https://example.test/meta")
			aicardBoundaryAuth(t)
			var original *runtimeRunner
			testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
				original = newCommandRunnerWithFlags(flags).(*runtimeRunner)
				return original
			})
			observed := false
			stop := errors.New("stop before network")
			testseam.Swap(t, &runnerCallTool, func(client *transport.Client, ctx context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
				observed = true
				deadline, ok := ctx.Deadline()
				if !ok || original.globalFlags.Timeout != 90 {
					t.Fatal("configured operation deadline was lost")
				}
				remaining := time.Until(deadline)
				if diff := client.HTTPClient.Timeout - remaining; diff < -time.Second || diff > time.Second {
					t.Fatalf("HTTP timeout=%s remaining=%s", client.HTTPClient.Timeout, remaining)
				}
				if client.HTTPClient.Transport.(*http.Transport).ResponseHeaderTimeout != client.HTTPClient.Timeout {
					t.Fatal("response-header timeout was not adapted")
				}
				if !shortParent && client.HTTPClient.Timeout < 85*time.Second {
					t.Fatal("discovery retained the default timeout")
				}
				if shortParent && client.HTTPClient.Timeout > 2*time.Second {
					t.Fatal("discovery extended its parent deadline")
				}
				return transport.ToolCallResult{}, stop
			})
			ctx := t.Context()
			if shortParent {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
			}
			root := NewRootCommand(ctx)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"aicard", "template", "get", "--mcp-id", "fixture-service", "--template-id", "t", "--version", "1", "--out", filepath.Join(t.TempDir(), "card.json"), "--timeout", "90", "--token", "local-test-token"})
			if _, err := corecmd.ExecuteContextCForTest(root, ctx); err == nil || !observed {
				t.Fatalf("discovery was not observed: %v", err)
			}
			if original.transport.HTTPClient.Timeout != 30*time.Second || original.transport.HTTPClient.Transport.(*http.Transport).ResponseHeaderTimeout != 20*time.Second {
				t.Fatal("shared transport defaults were mutated")
			}
		})
	}
}

func TestCrossPlatformCoverageAicardTemplateSlowDiscovery(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch {
		case request.URL.Path == "/meta":
			select {
			case <-time.After(200 * time.Millisecond):
			case <-request.Context().Done():
				return
			}
			body, _ := json.Marshal(map[string]any{"mcpURL": server.URL + "/template"})
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": string(body)}}}
		case rpc.Method == "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "get_aicard_template", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"templateId": map[string]any{"type": "string"}, "version": map[string]any{"type": "string"}}}}}}
		default:
			result = map[string]any{"structuredContent": map[string]any{"success": true, "file": aicardBoundaryTemplate}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
	}))
	defer server.Close()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Cleanup(CloseFileLogger)
	t.Setenv("DINGTALK_MCP_META_MCP_URL", server.URL+"/meta")
	aicardBoundaryAuth(t)
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
		runner := newCommandRunnerWithFlags(flags).(*runtimeRunner)
		base := runner.transport.WithHTTPTimeout(100 * time.Millisecond).WithMaxRetries(0)
		base.HTTPClient.Transport.(*http.Transport).TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		base.TrustedDomains = []string{"127.0.0.1"}
		runner.transport = base
		return runner
	})
	path := filepath.Join(t.TempDir(), "card.json")
	root := NewRootCommand(t.Context())
	var stream bytes.Buffer
	root.SetOut(&stream)
	root.SetErr(&stream)
	root.SetArgs([]string{"aicard", "template", "get", "--mcp-id", "fixture-service", "--template-id", "t", "--version", "1", "--out", path, "--timeout", "10", "--token", "local-test-token", "--format", "json"})
	_, err := corecmd.ExecuteContextCForTest(root, t.Context())
	data, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || string(data) != aicardBoundaryTemplate {
		t.Fatalf("slow discovery failed: %v %v %s", err, readErr, stream.String())
	}
	if !strings.Contains(stream.String(), `"ok": true`) {
		t.Fatalf("missing success receipt: %s", stream.String())
	}
}

func TestCrossPlatformCoverageAicardDeliverySummaryCapability(t *testing.T) {
	root := NewRootCommand(t.Context())
	leaf, remaining, err := root.Find([]string{"chat", "message", "send-a2ui-card"})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("chat delivery command is unavailable: %v %v", remaining, err)
	}
	for _, name := range []string{"summary", "support-forward"} {
		if leaf.Flags().Lookup(name) == nil {
			t.Fatalf("DWS host lacks --%s required by the generated AICard Skill; integrate compatible chat delivery first", name)
		}
	}
}

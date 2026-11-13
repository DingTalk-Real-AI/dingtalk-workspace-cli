// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestCrossPlatformCoverageMindSchemaAndSafetyDeliveredInFullAndCompact(t *testing.T) {
	for _, path := range []string{"mind create", "mind node list", "mind node create", "mind node update", "mind node delete"} {
		t.Run(path, func(t *testing.T) {
			var full *contract.ResultSpec
			for _, compact := range []bool{false, true} {
				root := NewRootCommand()
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				args := []string{"schema", "--cli-path", path, "--format", "json"}
				if compact {
					args = append(args, "--compact")
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("compact=%v: %v; %s", compact, err, stderr.String())
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				var result contract.ResultSpec
				if err := json.Unmarshal(payload["result"], &result); err != nil {
					t.Fatal(err)
				}
				normalized, err := contract.NormalizeResultSpec(&result, path)
				if err != nil {
					t.Fatal(err)
				}
				if compact {
					if !reflect.DeepEqual(full, normalized) {
						t.Fatal("compact result differs from full result")
					}
				} else {
					full = normalized
				}
				var schema map[string]any
				if err := json.Unmarshal(result.DataSchema, &schema); err != nil {
					t.Fatal(err)
				}
				props := schema["properties"].(map[string]any)
				if path == "mind create" && props["message"] == nil {
					t.Fatal("creation message absent from schema")
				}
				if props["nodeId"] == nil {
					t.Fatal("file identity absent from result schema")
				}
				if path == "mind node list" && (props["items"] == nil || props["truncated"] == nil || props["hasMore"] != nil || props["nextToken"] != nil) {
					t.Fatal("query data schema loses pages/truncation or duplicates pagination")
				}
				var safety struct {
					Confirmation string `json:"confirmation"`
					Effect       string `json:"effect"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &safety); err != nil {
					t.Fatal(err)
				}
				if path == "mind node list" {
					if safety.Confirmation != "not_required" || safety.Effect != "read" {
						t.Fatalf("query safety %#v", safety)
					}
				} else if safety.Confirmation != "user_required" {
					t.Fatalf("write confirmation missing: %#v", safety)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMindAndWikiCreateSelectionDistinguishesInitialContent(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, path := range []string{"mind create", "wiki node create"} {
			args := []string{"--cli-path", path}
			if compact {
				args = append(args, "--compact")
			}
			payload := executeShortcutSchemaQuery(t, args...)
			summary, _ := payload["agent_summary"].(string)
			avoid := strings.Join(schemaContractStringSlice(payload["avoid_when"]), " ")
			if path == "mind create" {
				if !strings.Contains(summary, "远端调用前校验") || !strings.Contains(avoid, "wiki node create --type amind") || !strings.Contains(avoid, "空白脑图") {
					t.Fatalf("mind routing compact=%v: summary=%q avoid=%q", compact, summary, avoid)
				}
			} else if !strings.Contains(summary, "空白脑图文件") || !strings.Contains(summary, "不带初始化内容") || !strings.Contains(avoid, "mind create --source") || !strings.Contains(avoid, "OpenNodes") {
				t.Fatalf("wiki routing compact=%v: summary=%q avoid=%q", compact, summary, avoid)
			}
		}
	}
}

// Resolve the approved server through the real runner; intercept only the final
// transport call. No live credentials, gateway calls or writes are performed.
func TestCrossPlatformCoverageMindRuntimeEndpoint(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DINGTALK_MIND_MCP_URL", "")
	testseam.Swap(t, &dynamicEndpoints, map[string]string{})
	testseam.Swap(t, &dynamicProducts, map[string]bool{})
	testseam.Swap(t, &dynamicAliases, map[string]string{})
	testseam.Swap(t, &dynamicToolEndpoints, map[string]string{})
	testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{})
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "offline-test-token"}, nil
	})
	restore := authpkg.PushMCPBaseURLOverride("")
	t.Cleanup(restore)
	runner := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{Timeout: 1}}
	for _, stage := range []string{"cold", "injected", "pre_override"} {
		t.Run(stage, func(t *testing.T) {
			if stage != "cold" {
				injectStaticServers()
			}
			want := "https://mcp-gw.dingtalk.com/server/mind"
			if stage == "pre_override" {
				restore := authpkg.PushMCPBaseURLOverride("https://pre-mcp.dingtalk.com")
				t.Cleanup(restore)
				want = "https://pre-mcp-gw.dingtalk.com/server/mind"
			}
			for _, tool := range []string{"create_mind", "list_mind_nodes", "create_mind_node", "update_mind_node", "delete_mind_node"} {
				t.Run(tool, func(t *testing.T) {
					calls := 0
					testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, _ context.Context, endpoint, gotTool string, _ map[string]any) (transport.ToolCallResult, error) {
						calls++
						if endpoint != want || gotTool != tool {
							t.Errorf("dispatch %s/%s, want %s/%s", endpoint, gotTool, want, tool)
						}
						return transport.ToolCallResult{Content: map[string]any{"success": true}}, nil
					})
					if _, err := runner.runSingle(context.Background(), executor.NewHelperInvocation("mind-endpoint-test", "mind", tool, nil), false); err != nil {
						t.Fatal(err)
					}
					if calls != 1 {
						t.Fatalf("got %d calls, want one", calls)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageMindRunnerPreservesRejectedBusinessDiagnostics(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DINGTALK_MIND_MCP_URL", "")
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "offline-test-token"}, nil
	})
	restore := authpkg.PushMCPBaseURLOverride("")
	t.Cleanup(restore)
	for _, category := range []string{"VALIDATION", "DOWNSTREAM", "TIMEOUT"} {
		t.Run(category, func(t *testing.T) {
			testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
				return transport.ToolCallResult{Content: map[string]any{"success": false, "errorCode": "mind.protected", "errorMessage": "rejected", "errorStage": "processContent", "errorCategory": category, "logId": "trace-runner"}}, nil
			})
			runner := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{Timeout: 1}}
			_, err := runner.runSingle(context.Background(), executor.NewHelperInvocation("mind-error-test", "mind", "update_mind_node", nil), false)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.FailureStage != "processContent" || typed.ServerDiag.TraceID != "trace-runner" || typed.Details["error_category"] != category || typed.Retryable {
				t.Fatalf("lost runner diagnostics: %#v", err)
			}
		})
	}
}

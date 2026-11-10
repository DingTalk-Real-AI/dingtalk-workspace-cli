// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package app

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

type a2uiForwardRequestRunner struct{ runner *runtimeRunner }

func (r *a2uiForwardRequestRunner) Run(ctx context.Context, invocation executor.Invocation) (executor.Result, error) {
	return r.runner.executeInvocation(ctx, "https://a2ui-forward-test.invalid", invocation)
}

// Exercise the installed root, flag parser and production runner.
// Only credentials, endpoint resolution and the final network exchange are fixtures.
func TestCrossPlatformCoverageA2UIForwardRootRequests(t *testing.T) {
	for _, target := range []struct{ flag, value, property string }{
		{"--conversation-id", "fixture-group", "openConversationId"},
		{"--open-dingtalk-id", "DAAAAAAAAAAAiE", "receiverOpenDingTalkId"},
	} {
		for _, choice := range []struct {
			name, flag string
			want       bool
		}{
			{"omitted", "", true},
			{"true", "--support-forward=true", true},
			{"shorthand", "--support-forward", true},
			{"false", "--support-forward=false", false},
		} {
			t.Run(target.flag+"/"+choice.name, func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				t.Setenv("DO_NOT_TRACK", "1")
				t.Cleanup(CloseFileLogger)
				helpers.InitDepsForTest(t, helpers.GetCaller())
				testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result { return runtimecontext.Result{} })
				testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
					return AccessTokenSnapshot{AccessToken: "synthetic-fixture-token"}, nil
				})
				calls := 0
				testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, _ context.Context, _, tool string, params map[string]any) (transport.ToolCallResult, error) {
					calls++
					if tool != "create_and_send_a2ui_card" || params["supportForward"] != choice.want || params[target.property] != target.value {
						t.Fatalf("unexpected tools/call: tool=%s params=%#v", tool, params)
					}
					if _, nested := params["cardProperty"]; nested {
						t.Fatal("supportForward must stay a top-level MCP parameter")
					}
					return transport.ToolCallResult{Content: map[string]any{"success": true, "result": map[string]any{"bizId": "fixture-biz"}}}, nil
				})
				testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
					return &a2uiForwardRequestRunner{&runtimeRunner{transport: transport.NewClient(nil), globalFlags: flags, auditSink: audit.NopSink{}}}
				})
				args := []string{"chat", "message", "send-a2ui-card", target.flag, target.value, "--content", `["{\"version\":\"v1.0\"}"]`, "--yes", "--format=json"}
				if choice.flag != "" {
					args = append(args, choice.flag)
				}
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root := NewRootCommand(t.Context())
				var stream bytes.Buffer
				root.SetOut(&stream)
				root.SetErr(&stream)
				root.SetArgs(args)
				if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), args); err != nil {
					t.Fatal(err)
				}
				if err := root.Execute(); err != nil || calls != 1 {
					t.Fatalf("err=%v calls=%d output=%s", err, calls, &stream)
				}
			})
		}
	}
}

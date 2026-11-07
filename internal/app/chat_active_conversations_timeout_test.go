// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Keep the real runner's per-request timeout boundary. Only credentials and
// the final network exchange are fixtures; no real account or network is used.
type activeConversationTimeoutCaller struct{ runner *runtimeRunner }

func (c *activeConversationTimeoutCaller) CallTool(ctx context.Context, product, tool string, params map[string]any) (*edition.ToolResult, error) {
	result, err := c.runner.executeInvocation(ctx, "https://pagination-test.invalid", executor.Invocation{CanonicalProduct: product, Tool: tool, Params: params})
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result.Response["content"])
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(encoded)}}}, nil
}
func (c *activeConversationTimeoutCaller) CallReadTool(ctx context.Context, product, tool string, params map[string]any) (*edition.ToolResult, error) {
	return c.CallTool(ctx, product, tool, params)
}
func (*activeConversationTimeoutCaller) Format() string { return "json" }
func (*activeConversationTimeoutCaller) DryRun() bool   { return false }
func (*activeConversationTimeoutCaller) Fields() string { return "" }
func (*activeConversationTimeoutCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageChatActiveConversationsTotalTimeoutBoundsRunner(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result { return runtimecontext.Result{} })
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "synthetic-fixture-token"}, nil
	})
	calls := 0
	var deadlines []time.Time
	testseam.Swap(t, &runnerCallTool, func(_ *transport.Client, ctx context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return transport.ToolCallResult{}, fmt.Errorf("request missing deadline")
		}
		deadlines = append(deadlines, deadline)
		calls++
		if calls == 1 {
			return transport.ToolCallResult{Content: map[string]any{"result": map[string]any{"conversationMessagesList": []any{}, "hasMore": true, "nextCursor": "A"}}}, nil
		}
		<-ctx.Done()
		return transport.ToolCallResult{}, ctx.Err()
	})
	flags := &GlobalFlags{Timeout: 10}
	caller := &activeConversationTimeoutCaller{&runtimeRunner{transport: transport.NewClient(nil), globalFlags: flags, auditSink: audit.NopSink{}}}
	// Exclude one-time catalog/scanner initialization from this deadline
	// propagation test; the production budget still includes that work.
	if _, err := caller.CallTool(context.Background(), "chat", "search_messages_by_time_range", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	calls, deadlines = 0, nil
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(chat.ActiveConversations))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--start=2026-09-07", "--end=2026-09-08", "--page-delay=100", "--total-timeout=1"})
	started := time.Now()
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted {
		t.Fatalf("emit code=%d emitted=%v err=%v", code, emitted, err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || code != 7 || elapsed < time.Second || elapsed > 5*time.Second || !deadlines[0].Equal(deadlines[1]) {
		t.Fatalf("budget reset or partial lost: calls=%d elapsed=%s code=%d deadlines=%v result=%#v", calls, elapsed, code, deadlines, result)
	}
	batch := result["data"].(map[string]any)["succeeded"].([]any)[0].(map[string]any)
	if batch["complete"] != false || batch["pagesFetched"] != float64(1) || result["meta"].(map[string]any)["pagination"].(map[string]any)["next_token"] != "A" {
		t.Fatalf("timeout lost valid progress: %#v", result)
	}
}

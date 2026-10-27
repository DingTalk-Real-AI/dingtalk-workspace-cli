// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageAitableInvocationTransportIsolatesTracing(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "test-placeholder"}, nil
	})
	testseam.Swap(t, &runnerCaptureRuntimeFailure, func(executor.Invocation, error, error) {})
	ids := make(chan string, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		ids <- client.ExecutionId
		<-release
		return transport.ToolCallResult{Content: map[string]any{"status": "success", "data": map[string]any{"baseId": "b"}}}, nil
	})
	base := transport.NewClient(nil)
	base.ExecutionId = "base-trace"
	runner := &runtimeRunner{transport: base, auditSink: audit.NopSink{}, globalFlags: &GlobalFlags{Timeout: 1}}
	var callers sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			_, err := runner.executeInvocation(context.Background(), "https://example.test", executor.Invocation{CanonicalProduct: "aitable", Tool: "get_base", Params: map[string]any{"baseId": "b"}})
			errors <- err
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		var id string
		select {
		case id = <-ids:
		case err := <-errors:
			t.Fatalf("invocation exited before transport: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("invocation did not reach transport")
		}
		if id == "" || id == "base-trace" || seen[id] {
			t.Errorf("request trace not isolated: %q", id)
		}
		seen[id] = true
	}
	releaseOnce.Do(func() { close(release) })
	callers.Wait()
	for i := 0; i < 4; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if base.ExecutionId != "base-trace" || base.FileLogger != nil {
		t.Fatal("base transport mutated")
	}
}

// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageDelegatorRunnerSnapshot(t *testing.T) {
	scope := &delegatorInvocation{}
	first := delegatorSnapshot{openDingtalkID: "first"}
	scope.snapshot.Store(&first)
	r := &runtimeRunner{globalFlags: &GlobalFlags{delegatorInvocation: scope}}
	parent, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	captured := r.withDelegatorContext(parent)
	if got := captured.Value(delegatorContextKey{}); got != first {
		t.Fatalf("snapshot=%#v", got)
	}
	deadline, ok := captured.Deadline()
	wantDeadline, _ := parent.Deadline()
	if !ok || deadline != wantDeadline {
		t.Fatal("runner replaced request deadline")
	}
	cancel()
	if !errors.Is(captured.Err(), context.Canceled) {
		t.Fatal("runner lost request cancellation")
	}
	second := delegatorSnapshot{openDingtalkID: "second"}
	scope.snapshot.Store(&second)
	if got := r.withDelegatorContext(captured).Value(delegatorContextKey{}); got != first {
		t.Fatalf("captured request switched identity: %#v", got)
	}
	empty := context.WithValue(context.Background(), delegatorContextKey{}, delegatorSnapshot{})
	if r.withDelegatorContext(empty) != empty {
		t.Fatal("explicit empty snapshot was replaced by scope identity")
	}
	// 同次执行中的并发子请求只能读取副本，不能改写后续请求的快照。
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if got := r.withDelegatorContext(context.Background()).Value(delegatorContextKey{}); got != second {
					t.Errorf("concurrent snapshot=%#v", got)
				}
			}
		}()
	}
	wg.Wait()
	scope.clear()
	if got := r.withDelegatorContext(nil).Value(delegatorContextKey{}); got != (delegatorSnapshot{}) {
		t.Fatalf("cleared scope retained identity: %#v", got)
	}
	for _, standalone := range []*runtimeRunner{nil, {}, {globalFlags: &GlobalFlags{}}} {
		if standalone.withDelegatorContext(parent) != parent {
			t.Fatal("standalone runner changed context")
		}
	}
}

func TestCrossPlatformCoverageDelegatorRunnerLifecycle(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	var runtime *runtimeRunner
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
		runtime = &runtimeRunner{globalFlags: flags}
		return runtime
	})
	mode := ""
	seen := delegatorSnapshot{}
	hook := func(stage string) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, _ []string) error {
			if mode == stage+" panic" {
				panic("fixture panic")
			}
			if mode == stage+" error" {
				return errors.New("fixture error")
			}
			if stage == "run" || stage == "post" {
				seen = runtime.withDelegatorContext(context.Background()).Value(delegatorContextKey{}).(delegatorSnapshot)
			}
			return nil
		}
	}
	leaf := &cobra.Command{Use: "leaf", Args: cobra.NoArgs, PreRunE: hook("pre"), RunE: hook("run"), PostRunE: hook("post")}
	leaf.Flags().String("principal-user-id", "", "")
	leaf.Flags().String("required", "", "")
	_ = leaf.MarkFlagRequired("required")
	group := &cobra.Command{Use: "delegator-lifecycle"}
	group.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := cmd.Root().PersistentPreRunE(cmd, args); err != nil {
			return err
		}
		return hook("persistent pre")(cmd, args)
	}
	group.PersistentPostRunE = hook("persistent post")
	group.AddCommand(leaf)
	root := newRootCommandWithAssembly(context.Background(), nil, func(root *cobra.Command) { root.AddCommand(group) })
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, tc := range []struct {
		name  string
		extra []string
		bad   bool
	}{
		{name: "success"},
		{name: "help", extra: []string{"--help"}},
		{name: "flag error", extra: []string{"--unknown-flag"}, bad: true},
		{name: "args error", extra: []string{"unexpected"}, bad: true},
		{name: "required error", bad: true},
		{name: "protocol error", extra: []string{"--principal-user-id=legacy"}, bad: true},
		{name: "output error", bad: true},
		{name: "persistent pre error", bad: true},
		{name: "pre error", bad: true},
		{name: "run error", bad: true},
		{name: "post error", bad: true},
		{name: "persistent post error", bad: true},
		{name: "persistent pre panic", bad: true},
		{name: "pre panic", bad: true},
		{name: "run panic", bad: true},
		{name: "post panic", bad: true},
		{name: "persistent post panic", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode = tc.name
			leaf.Flags().Lookup("required").Changed = false
			if help := leaf.Flags().Lookup("help"); help != nil {
				_ = help.Value.Set("false")
				help.Changed = false
			}
			args := []string{"delegator-lifecycle", "leaf", "--delegator-open-dingtalk-id=current"}
			if mode != "required error" {
				args = append(args, "--required=ok")
			}
			if mode == "output error" {
				args = append(args, "--output="+filepath.Join(t.TempDir(), "result.json"))
				testseam.Swap(t, &rootCreateTemp, func(string, string) (*os.File, error) {
					return nil, errors.New("fixture output error")
				})
				t.Cleanup(func() { _ = root.PersistentFlags().Set("output", "") })
			}
			root.SetArgs(append(args, tc.extra...))
			failed := false
			func() {
				defer func() {
					if recover() != nil {
						failed = true
					}
				}()
				failed = root.Execute() != nil
			}()
			if failed != tc.bad {
				t.Fatalf("failed=%v, want %v", failed, tc.bad)
			}
			if snapshot := runtime.globalFlags.delegatorInvocation.snapshot.Load(); snapshot != nil {
				t.Fatalf("execution retained scope: %#v", *snapshot)
			}
			if mode == "success" && seen.openDingtalkID != "current" {
				t.Fatal("post-run did not retain current identity")
			}
		})
	}
	mode = ""
	root.SetArgs([]string{"delegator-lifecycle", "leaf", "--required=ok"})
	if err := root.Execute(); err != nil || seen != (delegatorSnapshot{}) {
		t.Fatalf("next ordinary invocation inherited identity: %#v, %v", seen, err)
	}
}

func TestCrossPlatformCoverageDelegatorRunnerRetrySnapshot(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	originalHooks := edition.Get()
	t.Cleanup(func() { edition.Override(originalHooks) })
	edition.Override(&edition.Hooks{ClassifyToolResult: func(content map[string]any) error {
		if content["expired"] == true {
			return &authretry.AuthRefreshRequired{Cause: errors.New("expired")}
		}
		return nil
	}})
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "fixture-actor"}, nil
	})
	testseam.Swap(t, &runnerForceRefreshRejectedAccessToken, func(context.Context, string, string) (string, error) {
		return "fixture-refreshed", nil
	})
	scope := &delegatorInvocation{}
	identity := delegatorSnapshot{openDingtalkID: "original"}
	scope.snapshot.Store(&identity)
	calls := 0
	testseam.Swap(t, &runnerCallTool, func(tc *transport.Client, ctx context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		calls++
		if tc.ExtraHeaders["delegator-open-dingtalk-id"] != "original" {
			t.Fatal("retry reread changed invocation scope")
		}
		if calls == 1 {
			other := delegatorSnapshot{openDingtalkID: "other"}
			scope.snapshot.Store(&other)
			return transport.ToolCallResult{Content: map[string]any{"expired": true}}, nil
		}
		if !IsAuthRetrying(ctx) {
			t.Fatal("missing auth retry marker")
		}
		return transport.ToolCallResult{Content: map[string]any{"success": true}}, nil
	})
	r := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{delegatorInvocation: scope}, auditSink: audit.NopSink{}}
	if _, err := r.executeInvocation(context.Background(), "https://mcp-gw.dingtalk.com/mcp", executor.Invocation{CanonicalProduct: "doc", Tool: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want initial and auth retry", calls)
	}
}

func TestCrossPlatformCoverageDelegatorRunnerReadOnlyClone(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	const endpoint = "https://pre-mcp-gw.dingtalk.com/server/doc"
	t.Setenv("DINGTALK_DOC_MCP_URL", endpoint)
	waitPrefetch := stubDelegatorBusinessAuth(t)
	var headers []http.Header
	client := newDelegatorBusinessClient(t, endpoint, "fixture_read", `{"success":true}`, func(header http.Header) {
		headers = append(headers, header)
	})
	scope := &delegatorInvocation{}
	identity := delegatorSnapshot{openDingtalkID: "read-only"}
	scope.snapshot.Store(&identity)
	r := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{DryRun: true, delegatorInvocation: scope}, auditSink: audit.NopSink{}}
	inv := executor.NewHelperInvocation("fixture", "doc", "fixture_read", nil)
	if _, err := r.Run(context.Background(), inv); err != nil || len(headers) != 0 {
		t.Fatalf("dry-run made a request: %v, calls=%d", err, len(headers))
	}
	if _, err := r.RunReadOnly(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	waitPrefetch(t)
	if len(headers) != 1 || !r.globalFlags.DryRun {
		t.Fatal("read-only clone changed outer dry-run or request count")
	}
	assertDelegatorBusinessHeaders(t, "read-only clone", headers[0], map[string]string{"delegator-open-dingtalk-id": "read-only"})
	scope.clear()
	if _, err := r.RunReadOnly(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	waitPrefetch(t)
	if len(headers) != 2 {
		t.Fatalf("calls=%d", len(headers))
	}
	assertDelegatorBusinessHeaders(t, "cleared clone", headers[1], nil)
}

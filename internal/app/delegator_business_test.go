// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/spf13/cobra"
)

// 保留真实命令树、旧 Background helper、runtimeRunner.Run 和 HTTP Header 构造。
// 仅固定认证与网关响应，确保框架补齐身份，且整个测试不会访问业务网络。
func TestCrossPlatformCoverageDelegatorBusinessRealCommands(t *testing.T) {
	waitPrefetch := stubDelegatorBusinessAuth(t)
	for _, tc := range []struct {
		name    string
		product string
		args    []string
		tool    string
		body    string
	}{
		{
			name: "doc read legacy background", product: "doc",
			args: []string{"doc", "read", "--node=fixture-node"}, tool: "get_document_content",
			body: `{"success":true,"nodeId":"fixture-node","markdown":"fixture document"}`,
		},
		{
			name: "calendar legacy background", product: "calendar",
			args: []string{"calendar", "event", "get", "--id=fixture-event"}, tool: "get_calendar_detail",
			body: `{"success":true,"eventId":"fixture-event"}`,
		},
		{
			name: "todo legacy background", product: "todo",
			args: []string{"todo", "task", "get", "--task-id=fixture-task"}, tool: "get_todo_detail",
			body: `{"success":true,"result":{"todoDetailModel":{"taskId":"fixture-task"}}}`,
		},
		{
			name: "wiki legacy background", product: "wiki",
			args: []string{"wiki", "feed", "list", "--workspace=fixture-workspace"}, tool: "list_workspace_feeds",
			body: `{"success":true,"feeds":[]}`,
		},
		{
			name: "doc fetch shortcut", product: "doc",
			args: []string{"doc", "+fetch", "--node=fixture-node"}, tool: "get_document_content",
			body: `{"success":true,"nodeId":"fixture-node","markdown":"fixture document"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDepsForTest(t, helpers.GetCaller())
			endpoint := "https://pre-mcp-gw.dingtalk.com/server/" + tc.product
			t.Setenv("DINGTALK_"+strings.ToUpper(tc.product)+"_MCP_URL", endpoint)
			var headers []http.Header
			client := newDelegatorBusinessClient(t, endpoint, tc.tool, tc.body, func(header http.Header) {
				headers = append(headers, header)
			})
			// 必须返回实际 runtimeRunner；替代 Runner 直接 executeInvocation 会绕过待测绑定入口。
			testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
				return &runtimeRunner{transport: client, globalFlags: flags, auditSink: audit.NopSink{}}
			})
			root := NewRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			// 复用同一命令树连续执行，普通调用不能继承之前的委托身份。
			for _, identity := range []struct {
				name  string
				flags []string
				want  map[string]string
			}{
				{
					name: "all",
					flags: []string{
						"--delegator-user-id=fixture-user", "--delegator-corp-id=fixture-corp",
						"--delegator-open-dingtalk-id=fixture-openid",
					},
					want: map[string]string{
						"delegator-user-id": "fixture-user", "delegator-corp-id": "fixture-corp",
						"delegator-open-dingtalk-id": "fixture-openid",
					},
				},
				{name: "absent after delegated"},
				{
					name:  "pair",
					flags: []string{"--delegator-user-id=pair-user", "--delegator-corp-id=pair-corp"},
					want:  map[string]string{"delegator-user-id": "pair-user", "delegator-corp-id": "pair-corp"},
				},
				{name: "absent after pair"},
				{
					name:  "openid",
					flags: []string{"--delegator-open-dingtalk-id=only-openid"},
					want:  map[string]string{"delegator-open-dingtalk-id": "only-openid"},
				},
				{
					name:  "invalid pair with openid after delegated",
					flags: []string{"--delegator-user-id=incomplete", "--delegator-open-dingtalk-id=invalid-openid"},
				},
				{name: "absent after invalid"},
			} {
				headers = nil
				args := append([]string(nil), tc.args...)
				args = append(args, "--format=json")
				args = append(args, identity.flags...)
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("identity=%s execute: %v", identity.name, err)
				}
				if len(headers) != 1 {
					t.Fatalf("identity=%s HTTP calls=%d, want 1", identity.name, len(headers))
				}
				waitPrefetch(t)
				assertDelegatorBusinessHeaders(t, identity.name, headers[0], identity.want)
			}
		})
	}
}

// 两棵树先同时构建，再交替执行旧 helper；HTTP owner 能区分身份正确但错用了另一棵树 caller 的情况。
func TestCrossPlatformCoverageDelegatorBusinessSequentialRoots(t *testing.T) {
	waitPrefetch := stubDelegatorBusinessAuth(t)
	helpers.InitDepsForTest(t, helpers.GetCaller())
	const endpoint = "https://pre-mcp-gw.dingtalk.com/server/doc"
	t.Setenv("DINGTALK_DOC_MCP_URL", endpoint)
	var calls []struct {
		owner  int
		header http.Header
	}
	created := 0
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
		owner := created
		created++
		client := newDelegatorBusinessClient(t, endpoint, "get_document_content", `{"success":true,"markdown":"fixture document"}`, func(header http.Header) {
			calls = append(calls, struct {
				owner  int
				header http.Header
			}{owner, header})
		})
		return &runtimeRunner{transport: client, globalFlags: flags, auditSink: audit.NopSink{}}
	})
	roots := []*cobra.Command{NewRootCommand(), NewRootCommand()}
	if created != len(roots) {
		t.Fatalf("created %d runners, want %d", created, len(roots))
	}
	for _, root := range roots {
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
	}
	for _, tc := range []struct {
		name  string
		owner int
		flags []string
		want  map[string]string
	}{
		{
			name: "A delegated", owner: 0,
			flags: []string{"--delegator-user-id=a-user", "--delegator-corp-id=a-corp", "--delegator-open-dingtalk-id=a-openid"},
			want:  map[string]string{"delegator-user-id": "a-user", "delegator-corp-id": "a-corp", "delegator-open-dingtalk-id": "a-openid"},
		},
		{
			name: "B delegated", owner: 1,
			flags: []string{"--delegator-user-id=b-user", "--delegator-corp-id=b-corp", "--delegator-open-dingtalk-id=b-openid"},
			want:  map[string]string{"delegator-user-id": "b-user", "delegator-corp-id": "b-corp", "delegator-open-dingtalk-id": "b-openid"},
		},
		{name: "A absent after B", owner: 0},
		{
			name: "B openid", owner: 1,
			flags: []string{"--delegator-open-dingtalk-id=only-b-openid"},
			want:  map[string]string{"delegator-open-dingtalk-id": "only-b-openid"},
		},
		{name: "A invalid after B", owner: 0, flags: []string{"--delegator-user-id=incomplete"}},
		{name: "B absent after openid", owner: 1},
		{name: "A absent after invalid", owner: 0},
	} {
		calls = nil
		args := append([]string{"doc", "read", "--node=fixture-node", "--format=json"}, tc.flags...)
		testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
		roots[tc.owner].SetArgs(args)
		if err := roots[tc.owner].Execute(); err != nil {
			t.Fatalf("%s execute: %v", tc.name, err)
		}
		if len(calls) != 1 {
			t.Fatalf("%s HTTP calls=%d, want 1", tc.name, len(calls))
		}
		waitPrefetch(t)
		if calls[0].owner != tc.owner {
			t.Errorf("%s HTTP owner=%d, want %d", tc.name, calls[0].owner, tc.owner)
		}
		assertDelegatorBusinessHeaders(t, tc.name, calls[0].header, tc.want)
	}
}

func stubDelegatorBusinessAuth(t *testing.T) func(*testing.T) {
	t.Helper()
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "fixture-actor-token"}, nil
	})
	prefetched := make(chan struct{}, 1)
	testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) {
		prefetched <- struct{}{}
		return "fixture-actor-token", nil
	})
	return func(t *testing.T) {
		t.Helper()
		// 等预取调用进入 stub 后再恢复 seam，避免异步 goroutine 落到真实认证入口。
		select {
		case <-prefetched:
		case <-time.After(5 * time.Second):
			t.Fatal("runtime token prefetch did not use the fixture")
		}
	}
}

func newDelegatorBusinessClient(t *testing.T, endpoint, tool, body string, observe func(http.Header)) *transport.Client {
	t.Helper()
	return transport.NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != endpoint {
			return nil, fmt.Errorf("unexpected endpoint %s", req.URL)
		}
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(req.Body).Decode(&rpc); err != nil {
			return nil, err
		}
		if rpc.Method != "tools/call" || rpc.Params.Name != tool {
			return nil, fmt.Errorf("unexpected MCP call %s/%s; want tools/call/%s", rpc.Method, rpc.Params.Name, tool)
		}
		observe(req.Header.Clone())
		for _, name := range []string{
			"delegator-user-id", "delegator-corp-id", "delegator-open-dingtalk-id", "delegator-uid",
			"delegatorUserId", "delegatorCorpId", "delegatorOpenDingtalkId", "delegatorUid",
		} {
			if _, exists := rpc.Params.Arguments[name]; exists {
				t.Errorf("delegation leaked into business arguments: %s", name)
			}
		}
		raw, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": rpc.ID,
			"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": body}}},
		})
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(string(raw))), Request: req,
		}, nil
	})})
}

func assertDelegatorBusinessHeaders(t *testing.T, identity string, header http.Header, want map[string]string) {
	t.Helper()
	for _, name := range delegatorTestNames {
		if got := header.Get(name); got != want[name] {
			t.Errorf("identity=%s header %s=%q, want %q", identity, name, got, want[name])
		}
	}
	if header.Get("delegator-uid") != "" {
		t.Errorf("identity=%s gateway-only UID header was sent", identity)
	}
	if got := header.Get("Authorization"); got != "Bearer fixture-actor-token" {
		t.Errorf("identity=%s actor Authorization=%q, want fixture actor token", identity, got)
	}
}

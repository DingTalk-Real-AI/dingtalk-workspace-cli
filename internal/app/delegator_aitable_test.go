// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// 仅固定服务发现结果；真实命令、适配器、Header 组装和 HTTP 请求构建全部保留。
type delegatorAITableRunner struct{ runtime *runtimeRunner }

func (r *delegatorAITableRunner) Run(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
	return r.runtime.executeInvocation(ctx, "https://pre-mcp-gw.dingtalk.com/server/"+inv.CanonicalProduct, inv)
}

// 辅助只读通道同样固定端点，保留真实 helper 和 HTTP Header 构造。
func (r *delegatorAITableRunner) RunReadOnly(ctx context.Context, inv executor.Invocation) (executor.Result, error) {
	inv.DryRun = false
	return r.Run(ctx, inv)
}

func (*delegatorAITableRunner) ResolveToolProduct(_ context.Context, products []string, _ string) (string, error) {
	return products[len(products)-1], nil
}

func TestCrossPlatformCoverageDelegatorAITableRealCommands(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "fixture-actor-token"}, nil
	})
	for _, tc := range []struct {
		name     string
		args     []string
		tools    []string
		dryTools []string
	}{
		{"recent bases", []string{"base", "list", "--limit=10"}, []string{"list_bases"}, nil},
		{"search recent fallback", []string{"base", "search"}, []string{"list_bases"}, nil},
		{"search", []string{"base", "search", "--query=fixture"}, []string{"search_bases"}, nil},
		{"helper read", []string{"form", "list", "--base-id=b", "--table-id=t"}, []string{"list_form_views"}, nil},
		{"helper write", []string{"workflow", "enable", "--base-id=b", "--workflow-id=f"}, []string{"enable_workflow"}, nil},
		{"view block", []string{"view", "update", "name", "--base-id=b", "--table-id=t", "--view-id=v", "--name=fixture"}, []string{"update_view"}, nil},
		{"direct write", []string{"base", "create", "--name=fixture"}, []string{"create_base"}, nil},
		{"workflow publish", []string{"workflow", "create", "--base-id=b", `--dsl={"name":"fixture"}`}, []string{"create_workflow"}, nil},
		{"pagination", []string{"record", "query", "--base-id=b", "--table-id=t", "--all"}, []string{"query_records", "query_records"}, nil},
		{"view preflight pagination", []string{"record", "query", "--base-id=b", "--table-id=t", "--view-id=v", "--all"}, []string{"get_views", "query_records", "query_records"}, nil},
		{"shortcut data", []string{"+base-list", "--limit=10"}, []string{"list_bases"}, []string{"list_bases"}},
		{"shortcut passthrough", []string{"+base-get", "--base-id=b"}, []string{"get_base"}, nil},
		{"shortcut multiple reads", []string{"+base-schema-snapshot", "--base-id=b"}, []string{"get_base", "get_tables", "get_fields", "get_views"}, []string{"get_base", "get_tables", "get_fields", "get_views"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helpers.InitDepsForTest(t, helpers.GetCaller())
			var headers []http.Header
			var tools []string
			client := transport.NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				var rpc struct {
					ID     any `json:"id"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(req.Body).Decode(&rpc); err != nil {
					return nil, err
				}
				headers = append(headers, req.Header.Clone())
				tools = append(tools, rpc.Params.Name)
				for _, name := range []string{"delegator-user-id", "delegator-corp-id", "delegator-open-dingtalk-id", "delegator-uid", "delegatorUserId", "delegatorCorpId", "delegatorOpenDingtalkId", "delegatorUid"} {
					if _, exists := rpc.Params.Arguments[name]; exists {
						t.Errorf("delegation leaked into business arguments: %s", name)
					}
				}
				body := `{"success":true,"data":{"bases":[],"views":[],"flowId":"f","valid":true}}`
				switch rpc.Params.Name {
				case "get_base":
					body = `{"success":true,"data":{"baseId":"b","tables":[{"tableId":"t"}]}}`
				case "get_tables":
					body = `{"success":true,"data":{"tables":[{"tableId":"t"}]}}`
				case "get_fields":
					body = `{"success":true,"data":{"fields":[]}}`
				case "get_views":
					body = `{"success":true,"data":{"views":[{"viewId":"v","viewType":"Grid","baseId":"b","tableId":"t","filter":[],"sort":[]}]}}`
				case "query_records":
					body = `{"success":true,"data":{"records":[{"recordId":"one","cells":{}}],"hasMore":true,"nextCursor":"page-two"}}`
					if rpc.Params.Arguments["cursor"] == "page-two" {
						body = `{"success":true,"data":{"records":[{"recordId":"two","cells":{}}],"hasMore":false,"nextCursor":""}}`
					}
				}
				raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": body}}}})
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: req}, nil
			})})
			testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
				return &delegatorAITableRunner{runtime: &runtimeRunner{transport: client, globalFlags: flags, auditSink: audit.NopSink{}}}
			})
			root := NewRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			// 同一真实命令树切换输入组合，逐次验证 HTTP 值及后续普通调用的清理。
			for _, identity := range []struct {
				name  string
				flags []string
				want  map[string]string
			}{
				{"all", []string{"--delegator-user-id=fixture-user", "--delegator-corp-id=fixture-corp", "--delegator-open-dingtalk-id=fixture-openid"}, map[string]string{"delegator-user-id": "fixture-user", "delegator-corp-id": "fixture-corp", "delegator-open-dingtalk-id": "fixture-openid"}},
				{"absent after all", nil, nil},
				{"pair", []string{"--delegator-user-id=pair-user", "--delegator-corp-id=pair-corp"}, map[string]string{"delegator-user-id": "pair-user", "delegator-corp-id": "pair-corp"}},
				{"absent after pair", nil, nil},
				{"openid", []string{"--delegator-open-dingtalk-id=only-openid"}, map[string]string{"delegator-open-dingtalk-id": "only-openid"}},
				{"incomplete pair with openid", []string{"--delegator-user-id=incomplete", "--delegator-open-dingtalk-id=invalid-combination"}, nil},
				{"absent after invalid", nil, nil},
			} {
				headers, tools = nil, nil
				args := append([]string{"aitable"}, tc.args...)
				args = append(args, "--format=json", "--yes")
				args = append(args, identity.flags...)
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("identity=%s execute: %v", identity.name, err)
				}
				if !reflect.DeepEqual(tools, tc.tools) {
					t.Fatalf("calls=%v, want %v", tools, tc.tools)
				}
				assertDelegatorAITableHeaders(t, identity.name, headers, identity.want)
			}
			// dry-run 按具体命令契约验证：请求预览无 HTTP；允许的辅助读仍继承身份。
			headers, tools = nil, nil
			args := append([]string{"aitable"}, tc.args...)
			args = append(args, "--format=json", "--dry-run", "--delegator-open-dingtalk-id=fixture-openid")
			testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("dry-run: %v", err)
			}
			if !reflect.DeepEqual(tools, tc.dryTools) {
				t.Fatalf("dry-run calls=%v, want %v", tools, tc.dryTools)
			}
			assertDelegatorAITableHeaders(t, "dry-run", headers, map[string]string{"delegator-open-dingtalk-id": "fixture-openid"})
		})
	}
}

func assertDelegatorAITableHeaders(t *testing.T, identity string, headers []http.Header, want map[string]string) {
	t.Helper()
	for index, h := range headers {
		for _, name := range delegatorTestNames {
			if h.Get(name) != want[name] {
				t.Errorf("identity=%s request=%d header %s=%q, want %q", identity, index, name, h.Get(name), want[name])
			}
		}
		if h.Get("delegator-uid") != "" || h.Get("Authorization") != "Bearer fixture-actor-token" {
			t.Error("actor token changed or gateway-only identity was sent")
		}
	}
}

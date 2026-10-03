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

func (*delegatorAITableRunner) ResolveToolProduct(_ context.Context, products []string, _ string) (string, error) {
	return products[len(products)-1], nil
}

func TestCrossPlatformCoverageDelegatorAITableRealCommands(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "fixture-actor-token"}, nil
	})
	for _, tc := range []struct {
		name  string
		args  []string
		tools []string
	}{
		{"recent bases", []string{"base", "list", "--limit=10"}, []string{"list_bases"}},
		{"search recent fallback", []string{"base", "search"}, []string{"list_bases"}},
		{"search", []string{"base", "search", "--query=fixture"}, []string{"search_bases"}},
		{"helper read", []string{"form", "list", "--base-id=b", "--table-id=t"}, []string{"list_form_views"}},
		{"helper write", []string{"workflow", "enable", "--base-id=b", "--workflow-id=f"}, []string{"enable_workflow"}},
		{"view block", []string{"view", "update", "name", "--base-id=b", "--table-id=t", "--view-id=v", "--name=fixture"}, []string{"update_view"}},
		{"direct write", []string{"base", "create", "--name=fixture"}, []string{"create_base"}},
		{"workflow publish", []string{"workflow", "create", "--base-id=b", `--dsl={"name":"fixture"}`}, []string{"create_workflow"}},
		{"pagination", []string{"record", "query", "--base-id=b", "--table-id=t", "--all"}, []string{"query_records", "query_records"}},
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
				for _, name := range delegatorTestNames {
					if _, exists := rpc.Params.Arguments[name]; exists {
						t.Errorf("delegation leaked into business arguments: %s", name)
					}
				}
				body := `{"success":true,"data":{"bases":[],"views":[],"flowId":"f","valid":true}}`
				if rpc.Params.Name == "query_records" {
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
			// 同一真实命令树连续执行，验证有身份及随后无身份的请求互不污染。
			for _, delegated := range []bool{true, false} {
				headers, tools = nil, nil
				args := append([]string{"aitable"}, tc.args...)
				args = append(args, "--format=json", "--yes")
				if delegated {
					args = append(args, "--delegator-user-id=fixture-user", "--delegator-corp-id=fixture-corp", "--delegator-open-dingtalk-id=fixture-openid")
				}
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("delegated=%t execute: %v", delegated, err)
				}
				if !reflect.DeepEqual(tools, tc.tools) {
					t.Fatalf("calls=%v, want %v", tools, tc.tools)
				}
				for _, h := range headers {
					for name, value := range map[string]string{"delegator-user-id": "fixture-user", "delegator-corp-id": "fixture-corp", "delegator-open-dingtalk-id": "fixture-openid"} {
						if !delegated {
							value = ""
						}
						if h.Get(name) != value {
							t.Errorf("delegated=%t header %s=%q, want %q", delegated, name, h.Get(name), value)
						}
					}
					if h.Get("delegator-uid") != "" || h.Get("Authorization") != "Bearer fixture-actor-token" {
						t.Error("actor token changed or gateway-only identity was sent")
					}
				}
			}
			// 带委托参数的 dry-run 仍不得产生业务 HTTP 请求。
			headers = nil
			args := append([]string{"aitable"}, tc.args...)
			args = append(args, "--format=json", "--dry-run", "--delegator-open-dingtalk-id=fixture-openid")
			testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("dry-run: %v", err)
			}
			if len(headers) != 0 {
				t.Fatal("dry-run sent a business HTTP request")
			}
		})
	}
}

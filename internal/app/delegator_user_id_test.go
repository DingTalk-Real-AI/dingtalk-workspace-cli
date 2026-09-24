// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/requestmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/authretry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var delegatorTestNames = []string{"delegator-user-id", "delegator-corp-id", "delegator-open-dingtalk-id"}

func TestCrossPlatformCoverageDelegatorGlobalHiddenFlags(t *testing.T) {
	for _, path := range [][]string{nil, {"doc", "+fetch"}, {"chat", "+send"}, {"calendar"}, {"todo"}} {
		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		args := append([]string{"--delegator-user-id=u"}, path...)
		args = append(args, "--delegator-corp-id", "corp-B", "--delegator-open-dingtalk-id=o", "--help")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		for _, name := range delegatorTestNames {
			flag := root.PersistentFlags().Lookup(name)
			if flag == nil || !flag.Hidden || flag.Value.Type() != "string" {
				t.Fatalf("invalid hidden flag %s", name)
			}
			value, err := root.PersistentFlags().GetString(name)
			if err != nil || value != "" || flag.Changed {
				t.Fatalf("help retained %s: %q %v", name, value, err)
			}
			if strings.Contains(out.String(), name) {
				t.Fatalf("help leaked %s", name)
			}
		}
	}
	root := NewRootCommand()
	for canonical, tool := range deliverySchemaAllToolsForHelpFlagTest(t, root) {
		raw, err := json.Marshal(tool)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range delegatorTestNames {
			if bytes.Contains(raw, []byte(name)) {
				t.Errorf("%s publishes %s", canonical, name)
			}
		}
	}
}

func TestCrossPlatformCoverageDelegatorCompactSchemaHidden(t *testing.T) {
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"schema", "--cli-path", "doc +fetch", "--compact", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatal("compact schema did not return JSON")
	}
	for _, name := range delegatorTestNames {
		if strings.Contains(out.String(), name) {
			t.Fatalf("compact schema leaked %s", name)
		}
	}
}

func TestCrossPlatformCoverageDelegatorInputMatrix(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want delegatorSnapshot
	}{
		{"absent", nil, delegatorSnapshot{}},
		{"pair", []string{"--delegator-user-id=u", "--delegator-corp-id=corp-B"}, delegatorSnapshot{userID: "u", corpID: "corp-B"}},
		{"openid", []string{"--delegator-open-dingtalk-id=o"}, delegatorSnapshot{openDingtalkID: "o"}},
		{"all", []string{"--delegator-user-id=u", "--delegator-corp-id=c", "--delegator-open-dingtalk-id=o"}, delegatorSnapshot{"u", "c", "o"}},
		{"same duplicate", []string{"--delegator-open-dingtalk-id=o", "--delegator-open-dingtalk-id=o"}, delegatorSnapshot{openDingtalkID: "o"}},
		{"different duplicate", []string{"--delegator-open-dingtalk-id=o", "--delegator-open-dingtalk-id=p"}, delegatorSnapshot{}},
		{"no corp", []string{"--delegator-user-id=u"}, delegatorSnapshot{}},
		{"no user", []string{"--delegator-corp-id=c"}, delegatorSnapshot{}},
		{"partial with openid", []string{"--delegator-user-id=u", "--delegator-open-dingtalk-id=o"}, delegatorSnapshot{}},
		{"empty with openid", []string{"--delegator-user-id=", "--delegator-corp-id=c", "--delegator-open-dingtalk-id=o"}, delegatorSnapshot{}},
	}
	for _, raw := range []string{"", " ", " u", "u ", "u v", "u\tv", "u\nv", "u\rv", "u\x00v", "u\x7fv", "u\u0085v", "u\u2003v", "u\xffv"} {
		tests = append(tests, struct {
			name string
			args []string
			want delegatorSnapshot
		}{"invalid", []string{"--delegator-open-dingtalk-id=" + raw}, delegatorSnapshot{}})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "dws"}
			bindPersistentFlags(root, &GlobalFlags{})
			if err := root.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			got, supplied := consumeDelegatorFlags(root)
			if got != tc.want || supplied != (len(tc.args) > 0) {
				t.Fatalf("got %#v supplied=%v; want %#v", got, supplied, tc.want)
			}
			if got, supplied := consumeDelegatorFlags(root); got != (delegatorSnapshot{}) || supplied {
				t.Fatalf("retained %#v", got)
			}
		})
	}
	// 普通 Cobra 根或未采用本实现的同名参数不会被解释为委托身份。
	root := &cobra.Command{Use: "other"}
	root.PersistentFlags().String("delegator-user-id", "foreign", "")
	if got, supplied := consumeDelegatorFlags(root); got != (delegatorSnapshot{}) || supplied {
		t.Fatal(got)
	}
}

func TestCrossPlatformCoverageDelegatorReusableRoot(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	var contexts []context.Context
	leaf := &cobra.Command{Use: "delegator-fixture", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		contexts = append(contexts, cmd.Context())
		return nil
	}}
	leaf.Flags().String("principal-user-id", "", "")
	root := newRootCommandWithAssembly(context.Background(), nil, func(root *cobra.Command) { root.AddCommand(leaf) })
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	cases := []struct {
		args []string
		bad  bool
		want delegatorSnapshot
	}{
		{[]string{"--delegator-open-dingtalk-id=one"}, false, delegatorSnapshot{openDingtalkID: "one"}},
		{nil, false, delegatorSnapshot{}},
		{[]string{"--delegator-user-id=u", "--delegator-corp-id=corp-B"}, false, delegatorSnapshot{userID: "u", corpID: "corp-B"}},
		{[]string{"--delegator-user-id=incomplete"}, false, delegatorSnapshot{}},
		{[]string{"--delegator-open-dingtalk-id=help", "--help"}, false, delegatorSnapshot{}},
		{nil, false, delegatorSnapshot{}},
		{[]string{"--delegator-open-dingtalk-id=error", "--unknown-flag"}, true, delegatorSnapshot{}},
		{nil, false, delegatorSnapshot{}},
		{nil, false, delegatorSnapshot{}},
		{[]string{"--principal-user-id=u", "--delegator-user-id=u", "--delegator-corp-id=c"}, true, delegatorSnapshot{}},
		{[]string{"--principal-user-id=u"}, false, delegatorSnapshot{}},
		{[]string{"--delegator-open-dingtalk-id=after-legacy"}, false, delegatorSnapshot{openDingtalkID: "after-legacy"}},
		{[]string{"--principal-user-id=u", "--delegator-user-id="}, true, delegatorSnapshot{}},
		{nil, false, delegatorSnapshot{}},
	}
	for _, tc := range cases {
		before := len(contexts)
		// Cobra 的 help 布尔值不自动复位；此处仅清理通用 Help 状态。
		if help := leaf.Flags().Lookup("help"); help != nil {
			_ = help.Value.Set("false")
			help.Changed = false
		}
		root.SetArgs(append([]string{"delegator-fixture"}, tc.args...))
		err := root.Execute()
		if (err != nil) != tc.bad {
			t.Fatalf("%v: error=%v", tc.args, err)
		}
		if tc.bad && len(contexts) != before {
			t.Fatal("failed invocation executed")
		}
		if len(contexts) > before {
			got, _ := contexts[len(contexts)-1].Value(delegatorContextKey{}).(delegatorSnapshot)
			if got != tc.want {
				t.Fatalf("%v: got %#v want %#v", tc.args, got, tc.want)
			}
		}
	}
	if got := contexts[0].Value(delegatorContextKey{}).(delegatorSnapshot); got.openDingtalkID != "one" {
		t.Fatal("later invocation mutated old context")
	}
	// 真实 doc 前置链必须保留新旧协议冲突检查，且在业务调用前失败。
	root.SetArgs([]string{"doc", "+fetch", "--node", "node-fixture", "--principal-user-id=u", "--delegator-open-dingtalk-id=o"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "不能与") {
		t.Fatalf("doc conflict: %v", err)
	}
}

func TestCrossPlatformCoverageDelegatorHeaderBoundary(t *testing.T) {
	snapshot := delegatorSnapshot{"u", "corp-B", "o"}
	ctx := context.WithValue(context.Background(), delegatorContextKey{}, snapshot)
	for _, tc := range []struct {
		endpoint, product string
		excluded, allow   bool
	}{
		{"https://mcp-gw.dingtalk.com/v1/mcp", "doc", false, true},
		{"https://pre-mcp-gw.dingtalk.com/v1/mcp", "doc", false, true},
		{"https://mcp-gw.dingtalk.io:443/v1/mcp", "doc", false, true},
		{"https://pre-mcp-gw.dingtalk.io/v1/mcp", "doc", false, true},
		{"https://mcp-gw.dingtalk.com/v1/mcp", "doc", true, false},
		{"https://mcp-gw.dingtalk.com/v1/mcp", mcpMetaServerID, false, false},
		{"https://plugin.example.test/mcp", "doc", false, false},
		{"https://mcp-gw.dingtalk.com.evil.test/mcp", "doc", false, false},
		{"http://mcp-gw.dingtalk.com/mcp", "doc", false, false},
		{"https://mcp-gw.dingtalk.com:8443/mcp", "doc", false, false},
		{"https://user@mcp-gw.dingtalk.com/mcp", "doc", false, false},
		{"://bad", "doc", false, false},
	} {
		headers := map[string]string{"Delegator-User-Id": "forged", "DELEGATOR-UID": "forged", "X-Other": "keep"}
		got := applyDelegatorHeaders(ctx, headers, tc.endpoint, executor.Invocation{CanonicalProduct: tc.product}, tc.excluded)
		for _, name := range delegatorTestNames {
			if (got[name] != "") != tc.allow {
				t.Fatalf("%s %s: %#v", tc.endpoint, tc.product, got)
			}
		}
		if got["DELEGATOR-UID"] != "" || got["Delegator-User-Id"] != "" || got["X-Other"] != "keep" {
			t.Fatal(got)
		}
	}
	if got := applyDelegatorHeaders(nil, nil, "https://mcp-gw.dingtalk.com", executor.Invocation{}, false); got != nil {
		t.Fatal(got)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"one", "two", "three"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c := context.WithValue(context.Background(), delegatorContextKey{}, delegatorSnapshot{openDingtalkID: id})
			for i := 0; i < 20; i++ {
				if got := applyDelegatorHeaders(c, nil, "https://mcp-gw.dingtalk.com", executor.Invocation{}, false); got[requestmeta.DelegatorOpenDingtalkIDHeader] != id {
					t.Error("identity crossed contexts")
				}
			}
		}(id)
	}
	wg.Wait()
}

func TestCrossPlatformCoverageDelegatorHTTPExecution(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DINGTALK_DOC_MCP_URL", "https://mcp-gw.dingtalk.com/v1/mcp")
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "actor-token"}, nil
	})
	type received struct {
		headers http.Header
		params  map[string]any
	}
	var calls []received
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		calls = append(calls, received{r.Header.Clone(), req.Params})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":{"content":{"success":true}}}`)
	}))
	defer server.Close()
	local, _ := url.Parse(server.URL)
	client := transport.NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Host = local.Host
		u.Scheme = local.Scheme
		clone.URL = &u
		return server.Client().Transport.RoundTrip(clone)
	})})
	r := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{}}
	inv := executor.NewHelperInvocation("fixture", "doc", "fixture_tool", map[string]any{"nodeId": "node", "userId": "business-target"})
	var ctx context.Context
	root := newRootCommandWithAssembly(context.Background(), nil, func(root *cobra.Command) {
		root.AddCommand(&cobra.Command{Use: "delegator-http-fixture", RunE: func(cmd *cobra.Command, _ []string) error {
			ctx = cmd.Context()
			for i := 0; i < 2; i++ {
				if _, err := r.executeInvocation(ctx, "https://mcp-gw.dingtalk.com/v1/mcp", inv); err != nil {
					return err
				}
			}
			return nil
		}})
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"delegator-http-fixture", "--delegator-user-id=delegator", "--delegator-corp-id=corp-B", "--delegator-open-dingtalk-id=openid"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	// 辅助读允许联网，普通 dry-run 必须继续零请求。
	r.globalFlags.DryRun = true
	if _, err := r.Run(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatal("dry-run sent request")
	}
	if _, err := r.RunReadOnly(ctx, inv); err != nil {
		t.Fatal(err)
	}
	r.globalFlags.DryRun = false
	if len(calls) != 3 {
		t.Fatalf("read helper calls=%d", len(calls))
	}
	for _, call := range calls {
		if call.headers.Get("delegator-user-id") != "delegator" || call.headers.Get("delegator-corp-id") != "corp-B" || call.headers.Get("delegator-open-dingtalk-id") != "openid" || call.headers.Get("delegator-uid") != "" {
			t.Fatal(call.headers)
		}
		if call.headers.Get("Authorization") != "Bearer actor-token" {
			t.Fatal("actor token changed")
		}
		args, _ := call.params["arguments"].(map[string]any)
		if len(args) != 2 || args["userId"] != "business-target" {
			t.Fatalf("business arguments changed: %#v", args)
		}
	}
	// 登录辅助请求即使持有同一个 context，也不得携带委托身份。
	if _, err := r.RunWithToken(ctx, inv, "login-token"); err != nil {
		t.Fatal(err)
	}
	for _, name := range delegatorTestNames {
		if calls[len(calls)-1].headers.Get(name) != "" {
			t.Fatal("login leaked delegation")
		}
	}
	// 插件拥有自己的端点/凭据，即使配置的是同一个网关，也不继承委托字段。
	testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{"delegator-plugin": {ExtraHeaders: map[string]string{"Delegator-Uid": "forged", "delegator-user-id": "forged"}}})
	for _, product := range []string{"delegator-plugin", mcpMetaServerID} {
		other := inv
		other.CanonicalProduct = product
		if _, err := r.executeInvocation(ctx, "https://mcp-gw.dingtalk.com/v1/mcp", other); err != nil {
			t.Fatal(err)
		}
		for _, name := range append(append([]string{}, delegatorTestNames...), "delegator-uid") {
			if calls[len(calls)-1].headers.Get(name) != "" {
				t.Fatalf("%s leaked %s", product, name)
			}
		}
	}
	// 业务错误不得触发省略身份后的第二次调用。
	count := 0
	testseam.Swap(t, &runnerCallTool, func(tc *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		count++
		if tc.ExtraHeaders["delegator-user-id"] != "delegator" {
			t.Error("error call lost identity")
		}
		return transport.ToolCallResult{}, errors.New("business rejected")
	})
	if _, err := r.executeInvocation(ctx, "https://mcp-gw.dingtalk.com/v1/mcp", inv); err == nil || count != 1 {
		t.Fatalf("error=%v calls=%d", err, count)
	}
}

func TestCrossPlatformCoverageDelegatorReservedAndRedacted(t *testing.T) {
	original := edition.Get()
	t.Cleanup(func() { edition.Override(original) })
	edition.Override(&edition.Hooks{MergeHeaders: func(h map[string]string) map[string]string { h["Delegator-User-Id"] = "forged"; return h }, EnterpriseCredentialHeaders: func(h map[string]string) map[string]string { h["DELEGATOR-UID"] = "forged"; return h }})
	for _, headers := range []map[string]string{resolveIdentityHeaders(), MCPIdentityHeaders(), resolveMCPRequestHeadersForInvocation(executor.Invocation{CanonicalProduct: mcpMetaServerID})} {
		for name := range headers {
			if requestmeta.IsDelegatorHeader(name) {
				t.Fatal("shared headers leaked identity")
			}
		}
	}
	raw := map[string]string{"Delegator-User-Id": "u", "delegator-corp-id": "c", "DELEGATOR-OPEN-DINGTALK-ID": "o", "Delegator-Uid": "uid", "X-Plugin": "keep"}
	copy := maps.Clone(raw)
	got := pluginRequestHeaders(&PluginAuth{ExtraHeaders: raw})
	if len(got) != 1 || got["X-Plugin"] != "keep" || !maps.Equal(raw, copy) {
		t.Fatal(got)
	}
	for _, name := range append(append([]string{}, delegatorTestNames...), "delegator-uid") {
		if !logging.IsSensitiveKey(strings.ToUpper(name)) {
			t.Fatalf("not redacted: %s", name)
		}
	}
	for _, name := range delegatorTestNames {
		if got := SanitizeCommand([]string{"dws", "--" + name, "secret-canary", "--" + name + "=secret-canary"}); strings.Contains(got, "secret-canary") {
			t.Fatal(got)
		}
	}
}

func TestCrossPlatformCoverageDelegatorAuthRetry(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	original := edition.Get()
	t.Cleanup(func() { edition.Override(original) })
	edition.Override(&edition.Hooks{ClassifyToolResult: func(content map[string]any) error {
		if content["expired"] == true {
			return &authretry.AuthRefreshRequired{Cause: errors.New("expired")}
		}
		return nil
	}})
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		return AccessTokenSnapshot{AccessToken: "actor"}, nil
	})
	refreshes := 0
	testseam.Swap(t, &runnerForceRefreshRejectedAccessToken, func(context.Context, string, string) (string, error) { refreshes++; return "refreshed", nil })
	calls := 0
	testseam.Swap(t, &runnerCallTool, func(tc *transport.Client, ctx context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		calls++
		if tc.ExtraHeaders["delegator-open-dingtalk-id"] != "original" {
			t.Fatal("auth retry changed identity")
		}
		if calls == 1 {
			return transport.ToolCallResult{Content: map[string]any{"expired": true}}, nil
		}
		if !IsAuthRetrying(ctx) {
			t.Fatal("missing retry marker")
		}
		return transport.ToolCallResult{Content: map[string]any{"success": true}}, nil
	})
	ctx := context.WithValue(context.Background(), delegatorContextKey{}, delegatorSnapshot{openDingtalkID: "original"})
	r := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{}, auditSink: audit.NopSink{}}
	if _, err := r.executeInvocation(ctx, "https://mcp-gw.dingtalk.com/mcp", executor.Invocation{CanonicalProduct: "doc", Tool: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || refreshes != 1 {
		t.Fatalf("calls=%d refreshes=%d", calls, refreshes)
	}
}

func TestCrossPlatformCoverageDelegatorRequiredValidationCleanup(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	leaf := &cobra.Command{Use: "delegator-required", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().String("required", "", "")
	_ = leaf.MarkFlagRequired("required")
	leaf.Flags().String("principal-user-id", "", "")
	root := newRootCommandWithAssembly(context.Background(), nil, func(root *cobra.Command) { root.AddCommand(leaf) })
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"delegator-required", "--principal-user-id=u", "--delegator-open-dingtalk-id=o", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if v, _ := leaf.Flags().GetString("principal-user-id"); v != "" {
		t.Fatal("help retained old identity")
	}
	_ = leaf.Flags().Set("help", "false")
	root.SetArgs([]string{"delegator-required", "--delegator-open-dingtalk-id=o"})
	if err := root.Execute(); err == nil {
		t.Fatal("missing required flag accepted")
	}
	root.SetArgs([]string{"delegator-required", "--required=ok"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := leaf.Context().Value(delegatorContextKey{}).(delegatorSnapshot); got != (delegatorSnapshot{}) {
		t.Fatal("validation failure retained context")
	}
}

// 使用真实 doc 命令和旧装饰器，阻断在鉴权请求处，禁止任何线上请求。
type delegatorLegacyFixtureRunner struct{ calls []executor.Invocation }

func (r *delegatorLegacyFixtureRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, inv)
	if inv.Tool == "check_capability" {
		return executor.Result{}, errors.New("fixture capability rejection")
	}
	return executor.Result{Invocation: inv, Response: map[string]any{"content": map[string]any{}}}, nil
}

func TestCrossPlatformCoverageDelegatorLegacyDocCheckPreserved(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	fixture := &delegatorLegacyFixtureRunner{}
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(*GlobalFlags) executor.Runner { return fixture })
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"doc", "+fetch", "--node", "node-fixture", "--principal-user-id=legacy-user"})
	if err := root.Execute(); err == nil {
		t.Fatal("legacy authorization unexpectedly succeeded")
	}
	checks := 0
	for _, inv := range fixture.calls {
		if inv.Tool == "check_capability" {
			checks++
			if inv.CanonicalProduct != "drive-internal" || inv.Params["userId"] != "legacy-user" {
				t.Fatalf("wrong legacy check: %#v", inv)
			}
		} else if inv.CanonicalProduct == "doc" {
			t.Fatalf("business executed before check: %#v", inv)
		}
	}
	if checks != 1 {
		t.Fatalf("legacy capability checks=%d calls=%#v", checks, fixture.calls)
	}
}

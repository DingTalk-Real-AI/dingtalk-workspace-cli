// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package smart

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type smartMailCursorCall struct {
	tool string
	args map[string]any
}

type smartMailCursorCaller struct {
	responses map[string]string
	errors    map[string]error
	calls     []smartMailCursorCall
}

func (caller *smartMailCursorCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.calls = append(caller.calls, smartMailCursorCall{tool: tool, args: maps.Clone(args)})
	if err := caller.errors[tool]; err != nil {
		return nil, err
	}
	text, ok := caller.responses[tool]
	if !ok {
		return nil, fmt.Errorf("unexpected smart Mail tool: %s", tool)
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (*smartMailCursorCaller) Format() string { return "json" }
func (*smartMailCursorCaller) DryRun() bool   { return false }
func (*smartMailCursorCaller) Fields() string { return "" }
func (*smartMailCursorCaller) JQ() string     { return "" }

func runSmartMailDeclaration(t *testing.T, declaration shortcut.Shortcut, caller *smartMailCursorCaller, args ...string) error {
	t.Helper()
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageSmartMailContractsAreUnifiedAndTyped(t *testing.T) {
	for _, declaration := range []struct {
		name    string
		rollout output.RolloutState
		result  bool
	}{
		{"search", SearchMail.OutputRollout, SearchMail.Contract.Result != nil},
		{"find-user", FindMailUser.OutputRollout, FindMailUser.Contract.Result != nil},
		{"triage", TriageMail.OutputRollout, TriageMail.Contract.Result != nil},
	} {
		if declaration.rollout != output.RolloutUnifiedActive || !declaration.result {
			t.Fatalf("%s is not unified with Result", declaration.name)
		}
	}
	for _, declaration := range []*shortcut.Shortcut{&SearchMail, &FindMailUser, &TriageMail} {
		if declaration.Contract.Pagination == nil || declaration.Contract.Pagination.CursorParameter != "cursor" {
			t.Fatalf("%s missing cursor Pagination", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageSmartMailCompatibilityReadsMatchRuntimeBoundary(t *testing.T) {
	for _, declaration := range []*shortcut.Shortcut{&UnreadMail, &RecentMail} {
		if declaration.OutputRollout != output.RolloutLegacyOnly || declaration.Contract.Result != nil || declaration.Contract.Pagination != nil {
			t.Errorf("%s unavailable runtime still publishes unified result/pagination", declaration.Command)
		}
		if declaration.Contract.Interface == nil || declaration.Contract.Interface.Availability != contract.InterfaceAvailable || declaration.Contract.Interface.Reason == "" {
			t.Errorf("%s missing Schema-compatible non-public interface", declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageSmartMailHistoricalStringSizeAndRuntimeDefault(t *testing.T) {
	for _, declaration := range []*shortcut.Shortcut{&SearchMail, &UnreadMail} {
		found := false
		for _, flag := range declaration.Flags {
			if flag.Name != "size" {
				continue
			}
			found = true
			if flag.Type != shortcut.FlagString || flag.Default != "" {
				t.Fatalf("%s --size type/default=%q/%q, want string/empty", declaration.Command, flag.Type, flag.Default)
			}
		}
		if !found {
			t.Fatalf("%s missing --size", declaration.Command)
		}
		cmd := corecmd.New(shortcut.FromShortcut(*declaration))
		if flag := cmd.Flags().Lookup("size"); flag == nil || flag.Value.Type() != "string" || flag.DefValue != "" {
			t.Fatalf("%s Cobra --size=%#v, want historical string with empty default", declaration.Command, flag)
		}
	}

	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		wantSize    string
	}{
		{name: "search default", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, wantSize: "20"},
		{name: "search explicit", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", "100"}, wantSize: "100"},
		{name: "unread default", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid"}, wantSize: "20"},
		{name: "unread explicit", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", "1"}, wantSize: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: map[string]string{"search_emails": `{"success":true,"messages":[],"hasMore":false,"nextCursor":""}`}}
			if err := runSmartMailDeclaration(t, tc.declaration, caller, tc.args...); err != nil {
				t.Fatalf("compatible execution failed: %v", err)
			}
			if len(caller.calls) != 1 || caller.calls[0].args["size"] != tc.wantSize {
				t.Fatalf("calls=%#v, want one call with size=%q", caller.calls, tc.wantSize)
			}
		})
	}

	for _, declaration := range []shortcut.Shortcut{SearchMail, UnreadMail} {
		caller := &smartMailCursorCaller{responses: map[string]string{}}
		helpers.InitDepsForTest(t, caller)
		values := map[string]string{"email": "mail@example.invalid", "size": "bad"}
		if declaration.Command == "+search-mail" {
			values["query"] = "fixture"
		}
		rt := smartMailRuntimeForCoverage(t, declaration, values)
		if err := declaration.Execute(rt); err == nil {
			t.Fatalf("%s direct Execute accepted malformed string size", declaration.Command)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("%s direct Execute reached %d remote calls", declaration.Command, len(caller.calls))
		}
	}
}

func TestCrossPlatformCoverageSmartMailZeroHitSentinelIsNarrow(t *testing.T) {
	fixture := map[string]any{
		"success": "true", "total": "0", "nextCursor": "$",
		"messages": []any{map[string]any{"ccRecipients": nil, "toRecipients": nil}},
	}
	items, err := smartMailSearchRows(fixture, "mail/search_emails")
	if err != nil || len(items) != 0 {
		t.Fatalf("reviewed zero-hit sentinel must normalize to empty: items=%v err=%v", items, err)
	}
	for name, bad := range map[string]map[string]any{
		"unexpected field": {"success": "true", "total": "0", "nextCursor": "$", "messages": []any{map[string]any{"subject": nil}}},
		"non-null field":   {"success": "true", "total": "0", "nextCursor": "$", "messages": []any{map[string]any{"ccRecipients": "x"}}},
		"multiple rows":    {"success": "true", "total": "0", "nextCursor": "$", "messages": []any{map[string]any{"ccRecipients": nil}, map[string]any{"toRecipients": nil}}},
	} {
		if _, err := smartMailSearchRows(bad, "mail/search_emails"); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
}

func TestCrossPlatformCoverageSmartMailMalformedCollectionsFail(t *testing.T) {
	for name, fixture := range map[string]map[string]any{
		"missing success":    {"messages": []any{}},
		"missing collection": {"success": "true", "total": "0", "nextCursor": "$"},
		"wrong type":         {"success": "true", "messages": map[string]any{}},
		"bad item":           {"success": "true", "messages": []any{"bad"}},
	} {
		if _, err := smartMailCollection(fixture, "mail/search_emails", "messages"); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
}

func TestCrossPlatformCoverageSmartMailNestedSendersFailClosed(t *testing.T) {
	if _, err := searchMailFrom(map[string]any{"from": []any{"bad"}}); err == nil {
		t.Fatal("malformed message from field must fail")
	}
	if _, err := recentMailSenders(map[string]any{"senders": []any{"bad"}}); err == nil {
		t.Fatal("non-object conversation sender must fail")
	}
	if _, err := recentMailSenders(map[string]any{"senders": []any{map[string]any{"name": ""}}}); err == nil {
		t.Fatal("sender without identity must fail")
	}
}

func TestCrossPlatformCoverageSmartMailPaginationOutputAndIdentitySchema(t *testing.T) {
	declaration := SearchMail
	cmd := corecmd.New(shortcut.FromShortcut(declaration))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	rt := shortcut.RuntimeContextForTest(cmd, declaration)
	if err := smartMailOutputPage(rt, "messages", []map[string]any{{"messageId": "message-1"}}, false, "cursor-2"); err != nil {
		t.Fatal(err)
	}
	if code, emitted, err := output.EmitStoredResult(cmd); err != nil || !emitted || code != 0 {
		t.Fatalf("emit code=%d emitted=%v err=%v", code, emitted, err)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
		Meta struct {
			Pagination *struct {
				EndpointExhausted bool   `json:"endpoint_exhausted"`
				NextToken         string `json:"next_token"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"complete", "hasMore", "nextCursor"} {
		if _, exists := envelope.Data[field]; exists {
			t.Fatalf("pagination field %s leaked into business data", field)
		}
	}
	if envelope.Meta.Pagination == nil || envelope.Meta.Pagination.EndpointExhausted || envelope.Meta.Pagination.NextToken != "cursor-2" {
		t.Fatalf("pagination=%+v", envelope.Meta.Pagination)
	}

	for _, tc := range []struct {
		declaration *shortcut.Shortcut
		collection  string
		identity    string
	}{
		{&SearchMail, "messages", "messageId"},
		{&TriageMail, "messages", "messageId"},
		{&FindMailUser, "users", "email"},
	} {
		if len(tc.declaration.Contract.Result.SensitivePaths) == 0 {
			t.Fatalf("%s missing sensitive_paths", tc.declaration.Command)
		}
		var schema map[string]any
		if err := json.Unmarshal(tc.declaration.Contract.Result.DataSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", tc.declaration.Command, err)
		}
		properties := schema["properties"].(map[string]any)
		if minimum := properties["count"].(map[string]any)["minimum"]; minimum != float64(0) {
			t.Fatalf("%s count minimum=%v, want 0", tc.declaration.Command, minimum)
		}
		items := properties[tc.collection].(map[string]any)["items"].(map[string]any)
		identity := items["properties"].(map[string]any)[tc.identity].(map[string]any)
		if identity["description"] == "" {
			t.Fatalf("%s missing stable identity", tc.declaration.Command)
		}
	}
}

func TestCrossPlatformCoverageSmartMailCursorPassthroughAndStall(t *testing.T) {
	tests := []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		tool        string
		response    string
	}{
		{name: "search", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", "20", "--cursor", "cursor-1"}, tool: "search_emails", response: `{"success":true,"messages":[{"id":"message-1","from":"sender@example.invalid"}],"hasMore":true,"nextCursor":"cursor-2"}`},
		{name: "unread", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", "20", "--cursor", "cursor-1"}, tool: "search_emails", response: `{"success":true,"messages":[{"id":"message-1","from":"sender@example.invalid"}],"hasMore":true,"nextCursor":"cursor-2"}`},
		{name: "triage", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--limit", "20", "--cursor", "cursor-1"}, tool: "search_emails", response: `{"success":true,"messages":[{"id":"message-1","from":"sender@example.invalid"}],"hasMore":true,"nextCursor":"cursor-2"}`},
		{name: "find user", declaration: FindMailUser, args: []string{"--query", "fixture", "--limit", "20", "--cursor", "cursor-1"}, tool: "search_mail_users", response: `{"success":true,"users":[{"id":"user-1","email":"user@example.invalid"}],"hasMore":true,"nextCursor":"cursor-2"}`},
		{name: "recent", declaration: RecentMail, args: []string{"--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "20", "--cursor", "cursor-1"}, tool: "list_mailbox_threads", response: `{"success":true,"result":{"conversations":[{"id":"thread-1","senders":[{"email":"sender@example.invalid"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: map[string]string{tc.tool: tc.response}}
			if err := runSmartMailDeclaration(t, tc.declaration, caller, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 1 || caller.calls[0].args["cursor"] != "cursor-1" {
				t.Fatalf("cursor not passed through: %#v", caller.calls)
			}
		})
	}
	caller := &smartMailCursorCaller{responses: map[string]string{"search_mail_users": `{"success":true,"users":[{"id":"user-1","email":"user@example.invalid"}],"hasMore":true,"nextCursor":"cursor-1"}`}}
	if err := runSmartMailDeclaration(t, FindMailUser, caller, "--query", "fixture", "--cursor", "cursor-1"); err == nil {
		t.Fatal("repeated cursor unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageSmartMailLimitValidationHasZeroCalls(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
	}{
		{name: "search blank", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", " "}},
		{name: "search nonnumeric", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", "bad"}},
		{name: "search zero", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", "0"}},
		{name: "search negative", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", "-1"}},
		{name: "search high", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--size", "101"}},
		{name: "unread blank", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", " "}},
		{name: "unread nonnumeric", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", "bad"}},
		{name: "unread zero", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", "0"}},
		{name: "unread negative", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", "-1"}},
		{name: "unread high", declaration: UnreadMail, args: []string{"--email", "mail@example.invalid", "--size", "101"}},
		{name: "triage zero", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture", "--limit", "0"}},
		{name: "find high", declaration: FindMailUser, args: []string{"--query", "fixture", "--limit", "101"}},
		{name: "recent negative", declaration: RecentMail, args: []string{"--email", "mail@example.invalid", "--folder", "folder-1", "--limit", "-1"}},
		{name: "search blank query", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", " "}},
		{name: "find blank query", declaration: FindMailUser, args: []string{"--query", " "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: map[string]string{}}
			if err := runSmartMailDeclaration(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatal("invalid limit unexpectedly succeeded")
			}
			if len(caller.calls) != 0 {
				t.Fatalf("invalid limit reached remote: %d", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageFindMailUserProjectionMatchesResultIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		user      map[string]any
		wantKey   string
		absentKey string
	}{
		{name: "id only", user: map[string]any{"id": "user-1", "email": ""}, wantKey: "id", absentKey: "email"},
		{name: "email only", user: map[string]any{"id": "", "email": "user@example.invalid"}, wantKey: "email", absentKey: "id"},
		{name: "numeric id only", user: map[string]any{"id": float64(7), "email": "  "}, wantKey: "id", absentKey: "email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projected, err := findMailUserProjection(tc.user)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := projected[tc.wantKey]; !ok {
				t.Fatalf("projection missing %s: %#v", tc.wantKey, projected)
			}
			if _, ok := projected[tc.absentKey]; ok {
				t.Fatalf("projection retained empty %s: %#v", tc.absentKey, projected)
			}
		})
	}

	caller := &smartMailCursorCaller{responses: map[string]string{
		"search_mail_users": `{"success":true,"users":[{"id":"","email":""}],"hasMore":false,"nextCursor":""}`,
	}}
	if err := runSmartMailDeclaration(t, FindMailUser, caller, "--query", "fixture"); err == nil {
		t.Fatal("identity-free user unexpectedly satisfied runtime Result contract")
	}
	for name, user := range map[string]map[string]any{
		"malformed email with valid id": {"id": "user-1", "email": 7},
		"malformed id with valid email": {"id": map[string]any{"value": "user-1"}, "email": "user@example.invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := findMailUserProjection(user); err == nil {
				t.Fatal("malformed optional identity unexpectedly passed projection")
			}
		})
	}
}

func TestCrossPlatformCoverageSmartMailWhitespaceIdentityFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration shortcut.Shortcut
		args        []string
		tool        string
		response    string
	}{
		{name: "search message id", declaration: SearchMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, tool: "search_emails", response: `{"success":true,"messages":[{"id":"   ","from":"sender@example.invalid"}],"hasMore":false,"nextCursor":""}`},
		{name: "triage message id", declaration: TriageMail, args: []string{"--email", "mail@example.invalid", "--query", "fixture"}, tool: "search_emails", response: `{"success":true,"messages":[{"id":"   ","from":"sender@example.invalid"}],"hasMore":false,"nextCursor":""}`},
		{name: "find user identity", declaration: FindMailUser, args: []string{"--query", "fixture"}, tool: "search_mail_users", response: `{"success":true,"users":[{"id":"   ","email":"   "}],"hasMore":false,"nextCursor":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &smartMailCursorCaller{responses: map[string]string{tc.tool: tc.response}}
			if err := runSmartMailDeclaration(t, tc.declaration, caller, tc.args...); err == nil {
				t.Fatal("whitespace stable identity unexpectedly succeeded")
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls=%d, want one response validation call", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageFindMailUserKeepsStableQueryProperty(t *testing.T) {
	for _, parameter := range FindMailUser.Contract.Parameters {
		if parameter.Name == "query" {
			if parameter.Property != "query" {
				t.Fatalf("query property = %q, want stable composite property query", parameter.Property)
			}
			return
		}
	}
	t.Fatal("FindMailUser query parameter declaration is missing")
}

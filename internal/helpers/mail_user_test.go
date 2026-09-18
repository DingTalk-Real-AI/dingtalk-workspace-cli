// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type mailUserGetCaller struct {
	response string
	dryRun   bool
	calls    int
	product  string
	tool     string
	args     map[string]any
	respond  func(context.Context, string, map[string]any) (string, error)
}

func (c *mailUserGetCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	c.product, c.tool, c.args = product, tool, args
	if c.respond != nil {
		text, err := c.respond(ctx, tool, args)
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, nil
}
func (*mailUserGetCaller) Format() string { return "json" }
func (c *mailUserGetCaller) DryRun() bool { return c.dryRun }
func (*mailUserGetCaller) Fields() string { return "" }
func (*mailUserGetCaller) JQ() string     { return "" }

func executeMailUserGet(t *testing.T, caller *mailUserGetCaller, args ...string) ([]byte, error) {
	t.Helper()
	return executeMailUserLookup(t, caller, "get", args...)
}

func executeMailUserLookup(t *testing.T, caller *mailUserGetCaller, command string, args ...string) ([]byte, error) {
	t.Helper()
	data, _, err := executeMailUserLookupWithExit(t, caller, command, args...)
	return data, err
}

func executeMailUserLookupWithExit(t *testing.T, caller *mailUserGetCaller, command string, args ...string) ([]byte, int, error) {
	t.Helper()
	return executeMailUserLookupWithContext(t, context.Background(), caller, command, args...)
}

func executeMailUserLookupWithContext(t *testing.T, ctx context.Context, caller *mailUserGetCaller, command string, args ...string) ([]byte, int, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(caller)
	var stdout, stderr bytes.Buffer
	root := newMailCommand()
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.PersistentFlags().Bool("dry-run", caller.dryRun, "preview")
	root.PersistentFlags().String("format", "json", "output format")
	root.PersistentFlags().String("fields", "", "selected fields")
	root.PersistentFlags().String("jq", "", "output query")
	root.SetArgs(append([]string{"user", command}, args...))
	ctx, _ = output.WithResultStore(ctx)
	cmd, err := root.ExecuteContextC(ctx)
	code := 0
	if err == nil {
		code, _, err = output.EmitStoredResult(cmd)
	}
	return stdout.Bytes(), code, err
}

func TestCrossPlatformCoverageMailUserBatchGetMappingAndResult(t *testing.T) {
	for _, result := range []string{
		`{"users":[{"uid":9223372036854775806,"orgId":456,"staffId":"staff-789","name":"张三","orgEmail":"Alice+tag@Example.com"}],"notFoundOrgEmails":["missing@example.com"]}`,
		`{"users":[],"notFoundOrgEmails":["Alice+tag@Example.com","missing@example.com"]}`,
		`{"users":[{"uid":789,"orgEmail":"Alice+tag@Example.com"},{"uid":456,"orgEmail":"missing@example.com"}],"notFoundOrgEmails":[]}`,
	} {
		caller := &mailUserGetCaller{response: `{"success":true,"result":` + result + `}`}
		got, err := executeMailUserLookup(t, caller, "batch-get",
			"--org-emails", " Alice+tag@Example.com ,alice+tag@example.com", "--org-emails", "missing@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 || caller.product != "mail" || caller.tool != "batch_get_users_by_org_emails" {
			t.Fatalf("unexpected batch dispatch: %+v", caller)
		}
		wantArgs := map[string]any{"orgEmails": []string{"Alice+tag@Example.com", "alice+tag@example.com", "missing@example.com"}}
		if !reflect.DeepEqual(caller.args, wantArgs) {
			t.Fatalf("batch must preserve addresses for server-side deduplication: %#v", caller.args)
		}
		var envelope struct {
			OK      bool   `json:"ok"`
			Outcome string `json:"outcome"`
			Data    struct {
				Result json.RawMessage `json:"result"`
			} `json:"data"`
		}
		if err := json.Unmarshal(got, &envelope); err != nil {
			t.Fatal(err)
		}
		if !envelope.OK || envelope.Outcome != "success" || !reflect.DeepEqual(mailLookupTestJSON(t, envelope.Data.Result), mailLookupTestJSON(t, []byte(result))) {
			t.Fatalf("batch result or integer IDs changed: %s", got)
		}
	}
}

func TestCrossPlatformCoverageMailUserBatchGetBoundsAndDryRun(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		for _, args := range [][]string{
			nil, {"--org-emails", ""}, {"--org-emails", " , "},
			{"--org-emails", strings.Repeat("same@example.com,", 100) + "same@example.com"},
			{"--org-emails", "a@example.com", "--uid", "123"},
			{"--org-emails", "a@example.com", "--org-id", "456"},
		} {
			caller := &mailUserGetCaller{dryRun: dryRun}
			if _, err := executeMailUserLookup(t, caller, "batch-get", args...); err == nil || caller.calls != 0 {
				t.Fatalf("args=%v dryRun=%v error=%v calls=%d", args, dryRun, err, caller.calls)
			}
		}
	}
	for _, count := range []int{1, 100} {
		caller := &mailUserGetCaller{dryRun: true}
		addresses := strings.Repeat("a@example.com,", count-1) + "a@example.com"
		got, err := executeMailUserLookup(t, caller, "batch-get", "--org-emails", addresses, "--dry-run")
		if err != nil || caller.calls != 0 {
			t.Fatalf("count=%d error=%v calls=%d", count, err, caller.calls)
		}
		var preview struct {
			Data struct {
				Tool      string `json:"tool"`
				Executed  bool   `json:"executed"`
				Arguments struct {
					OrgEmails []string `json:"orgEmails"`
				} `json:"arguments"`
			} `json:"data"`
		}
		if err := json.Unmarshal(got, &preview); err != nil {
			t.Fatal(err)
		}
		if preview.Data.Executed || preview.Data.Tool != "batch_get_users_by_org_emails" || len(preview.Data.Arguments.OrgEmails) != count {
			t.Fatalf("invalid batch preview: %s", got)
		}
	}
}

func TestCrossPlatformCoverageMailUserBatchGetFailure(t *testing.T) {
	caller := &mailUserGetCaller{response: `{"success":false,"errorCode":"noPermission","errorMsg":"Not a member"}`}
	got, err := executeMailUserLookup(t, caller, "batch-get", "--org-emails", "a@example.com,invalid")
	if err == nil || !strings.Contains(err.Error(), "noPermission") || len(got) != 0 || caller.calls != 1 {
		t.Fatalf("batch failure must not become not-found results: output=%s error=%v calls=%d", got, err, caller.calls)
	}
}

func TestCrossPlatformCoverageMailUserBatchGetRejectsMalformedResults(t *testing.T) {
	for _, result := range []string{
		"", `null`, `{}`, `[]`, `true`,
		`{"users":[],"notFoundOrgEmails":null}`,
		`{"users":null,"notFoundOrgEmails":[]}`,
		`{"users":[],"notFoundOrgEmails":{}}`,
		`{"users":{},"notFoundOrgEmails":[]}`,
		`{"users":[null],"notFoundOrgEmails":[]}`,
		`{"users":[1],"notFoundOrgEmails":[]}`,
		`{"users":[],"notFoundOrgEmails":[null]}`,
		`{"users":[],"notFoundOrgEmails":[123]}`,
	} {
		t.Run(result, func(t *testing.T) {
			response := `{"success":true}`
			if result != "" {
				response = `{"success":true,"result":` + result + `}`
			}
			caller := &mailUserGetCaller{response: response}
			got, code, err := executeMailUserLookupWithExit(t, caller, "batch-get", "--org-emails", "a@example.com")
			envelope := mailLookupTestJSON(t, got).(map[string]any)
			if err != nil || code != 1 || envelope["outcome"] != "failure" || caller.calls != 1 {
				t.Fatalf("malformed response=%s output=%s error=%v calls=%d", response, got, err, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMailUserLookupFilteredUIDPrecision(t *testing.T) {
	for _, tc := range []struct{ command, flag, result, query string }{
		{"get", "--org-email", `{"uid":9223372036854775806}`, ".data.result.uid"},
		{"batch-get", "--org-emails", `{"users":[{"uid":9223372036854775806,"orgEmail":"a@example.com"}],"notFoundOrgEmails":[]}`, ".data.result.users[0].uid"},
	} {
		for _, filter := range [][]string{{"--jq", tc.query}, {"--fields", "result"}, {"--format", "ndjson"}} {
			caller := &mailUserGetCaller{response: `{"success":true,"result":` + tc.result + `}`}
			args := append([]string{tc.flag, "a@example.com"}, filter...)
			got, err := executeMailUserLookup(t, caller, tc.command, args...)
			if err != nil || caller.calls != 1 || !strings.Contains(string(got), "9223372036854775806") {
				t.Fatalf("%s %v: output=%s error=%v calls=%d", tc.command, filter, got, err, caller.calls)
			}
		}
	}
}

func TestCrossPlatformCoverageMailUserGetResultAndMapping(t *testing.T) {
	for _, result := range []string{
		`{"uid":9223372036854775806,"orgId":456,"staffId":"staff-789","name":"张三","orgEmail":"ZhangSan+tag@Example.com"}`,
		`null`, `{}`, `{"uid":null}`, `{"uid":null,"orgId":null,"staffId":null,"name":null,"orgEmail":null}`,
		`{"uid":9223372036854775806,"orgId":9223372036854775805,"extra":{"future":true}}`, "",
	} {
		t.Run(result, func(t *testing.T) {
			response := `{"success":true,"result":` + result + `}`
			if result == "" {
				response = `{"success":true}`
			}
			caller := &mailUserGetCaller{response: response}
			got, err := executeMailUserGet(t, caller, "--org-email", " ZhangSan+tag@Example.com ")
			if err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.product != "mail" || caller.tool != "get_user_by_org_email" {
				t.Fatalf("unexpected dispatch: %+v", caller)
			}
			if !reflect.DeepEqual(caller.args, map[string]any{"orgEmail": "ZhangSan+tag@Example.com"}) {
				t.Fatalf("MCP arguments must contain only the target orgEmail: %#v", caller.args)
			}
			var envelope struct {
				OK      bool            `json:"ok"`
				Outcome string          `json:"outcome"`
				Data    json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(got, &envelope); err != nil {
				t.Fatal(err)
			}
			var compact bytes.Buffer
			if err := json.Compact(&compact, envelope.Data); err != nil {
				t.Fatal(err)
			}
			var want, actual map[string]json.RawMessage
			_ = json.Unmarshal([]byte(response), &want)
			_ = json.Unmarshal(compact.Bytes(), &actual)
			if !envelope.OK || envelope.Outcome != "success" || !reflect.DeepEqual(actual, want) {
				t.Fatalf("lookup result changed: %s", got)
			}
		})
	}
}

func TestCrossPlatformCoverageMailUserGetValidationAndDryRun(t *testing.T) {
	for _, args := range [][]string{
		nil, {"--org-email", ""}, {"--org-email", "  "},
		{"--org-email", "a@example.com", "--uid", "123"},
		{"--org-email", "a@example.com", "--org-id", "456"},
	} {
		caller := &mailUserGetCaller{}
		if _, err := executeMailUserGet(t, caller, args...); err == nil || caller.calls != 0 {
			t.Fatalf("args=%v error=%v calls=%d", args, err, caller.calls)
		}
	}
	caller := &mailUserGetCaller{dryRun: true}
	got, err := executeMailUserGet(t, caller, "--org-email", "a@example.com", "--dry-run")
	if err != nil || caller.calls != 0 || !strings.Contains(string(got), `"executed": false`) || !strings.Contains(string(got), `"orgEmail": "a@example.com"`) {
		t.Fatalf("dry run: output=%s error=%v calls=%d", got, err, caller.calls)
	}
}

func TestCrossPlatformCoverageMailUserGetFailures(t *testing.T) {
	for _, response := range []string{
		`{"success":false,"errorCode":"noPermission","errorMsg":"Not a member"}`,
		`{"result":null}`, `{"success":"true"}`, `null`, `[]`, `invalid`,
		`{"success":true,"result":[]}`, `{"success":true,"result":true}`,
		`{"success":true,"result":123}`, `{"success":true,"result":"employee"}`,
		`{"success":true,"result":{"uid":"123"}}`, `{"success":true,"result":{"uid":false}}`,
		`{"success":true,"result":{"uid":1.5}}`, `{"success":true,"result":{"uid":9223372036854775808}}`,
		`{"success":true,"result":{"uid":123,"orgId":"456"}}`,
		`{"success":true,"result":{"uid":123,"staffId":456}}`,
		`{"success":true,"result":{"uid":123,"name":[]}}`,
		`{"success":true,"result":{"uid":123,"orgEmail":true}}`,
	} {
		caller := &mailUserGetCaller{response: response}
		got, err := executeMailUserGet(t, caller, "--org-email", "a@example.com")
		if err == nil || caller.calls != 1 || len(got) != 0 {
			t.Fatalf("response=%s output=%s error=%v calls=%d", response, got, err, caller.calls)
		}
		if strings.Contains(response, "noPermission") && !strings.Contains(err.Error(), "noPermission") {
			t.Fatalf("business error was lost: %v", err)
		}
	}
}

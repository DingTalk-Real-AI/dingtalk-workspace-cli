// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type minutesTemplateTagCall struct {
	server, tool string
	args         map[string]any
}

type minutesTemplateTagCaller struct {
	calls        []minutesTemplateTagCall
	text         string
	result       *edition.ToolResult
	err          error
	dry          bool
	customResult bool
}

func (c *minutesTemplateTagCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	copy := make(map[string]any, len(args))
	for key, value := range args {
		copy[key] = value
	}
	c.calls = append(c.calls, minutesTemplateTagCall{server, tool, copy})
	if c.customResult || c.err != nil {
		return c.result, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.text}}}, nil
}
func (*minutesTemplateTagCaller) Format() string { return "json" }
func (c *minutesTemplateTagCaller) DryRun() bool { return c.dry }
func (*minutesTemplateTagCaller) Fields() string { return "" }
func (*minutesTemplateTagCaller) JQ() string     { return "" }

type minutesTemplateTagExecution struct {
	stdout, stderr string
	env            map[string]any
	code           int
	err            error
}

func executeMinutesTemplateTag(t *testing.T, caller *minutesTemplateTagCaller, input string, args ...string) minutesTemplateTagExecution {
	t.Helper()
	var stdout, stderr bytes.Buffer
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: NewFormatterWithWriters(&stdout, &stderr)})
	root := newMinutesCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(args)
	ctx, _ := output.WithResultStore(context.Background())
	executed, err := corecmd.ExecuteContextCForTest(root, ctx)
	result := minutesTemplateTagExecution{err: err}
	if err == nil {
		var emitted bool
		result.code, emitted, result.err = output.EmitStoredResult(executed)
		if result.err == nil && !emitted {
			t.Fatal("command did not store a unified result")
		}
	} else {
		result.code = apperrors.ExitCode(err)
	}
	result.stdout, result.stderr = stdout.String(), stderr.String()
	raw := result.stdout
	if raw == "" {
		raw = result.stderr
	}
	if raw != "" {
		// Projection flags intentionally produce a non-envelope value.
		_ = json.Unmarshal([]byte(raw), &result.env)
	}
	return result
}

func minutesTemplateTagReceipt(data string) string { return `{"success":true,"result":` + data + `}` }

func requireMinutesTemplateTagOutcome(t *testing.T, got minutesTemplateTagExecution, outcome string, code int) {
	t.Helper()
	if got.err != nil || got.code != code || got.env["outcome"] != outcome || got.env["ok"] != (outcome == "success") {
		t.Fatalf("outcome=%s/%d: err=%v code=%d stdout=%s stderr=%s", outcome, code, got.err, got.code, got.stdout, got.stderr)
	}
	if outcome == "failure" {
		// EmitStoredResult preserves the framework's JSON stdout contract;
		// human-readable formats route failures to stderr instead.
		if got.stdout == "" || got.stderr != "" || got.env["error"] == nil || got.env["data"] != nil {
			t.Fatalf("failure channel or data/error exclusivity: %+v", got)
		}
	} else if got.stderr != "" || got.env["error"] != nil {
		t.Fatalf("success/partial channel or error: %+v", got)
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagRouting(t *testing.T) {
	cases := []struct {
		name, tool, data string
		args             []string
		want             map[string]any
	}{
		{"system", "list_system_templates", `{"items":[{"templateId":"official","title":"系统","brief":""}],"hasMore":false}`, []string{"template", "list-system"}, map[string]any{"maxResults": 20, "locale": "zh_CN"}},
		{"custom", "list_custom_templates", `{"items":[],"hasMore":false}`, []string{"template", "list-custom", "--limit", "10", "--cursor", " opaque/cursor "}, map[string]any{"maxResults": 10, "nextToken": " opaque/cursor "}},
		{"create", "create_custom_template", `{"success":true,"templateKey":"key","title":"保存标题","version":0}`, []string{"template", "create", "--title", "标题", "--prompt", "  分析\n\n结论  ", "--brief-intro", "简介", "--yes"}, map[string]any{"title": "标题", "prompt": "  分析\n\n结论  ", "briefIntro": "简介"}},
		{"update", "update_custom_template", `{"success":true,"templateKey":"key","title":"保存标题","version":0}`, []string{"template", "update", "--template-key", "key", "--title", "标题", "--yes"}, map[string]any{"templateKey": "key", "title": "标题"}},
		{"delete", "delete_custom_template", `{"success":true,"templateKey":"key","wasDefault":null}`, []string{"template", "delete", "--template-key", "key", "--yes"}, map[string]any{"templateKey": "key"}},
		{"default", "set_default_template", `{"success":true,"templateId":"official","templateKey":null}`, []string{"template", "set-default", "--template-id", "official", "--locale", "en_US", "--yes"}, map[string]any{"templateId": "official", "locale": "en_US"}},
		{"create-tag", "create_minutes_tag", `{"success":true,"tagId":"tag","displayName":"保存名称"}`, []string{"tag", "create", "--display-name", "名称", "--yes"}, map[string]any{"displayName": "名称"}},
		{"delete-tag", "delete_minutes_tag", `{"success":true,"tagId":"tag"}`, []string{"tag", "delete", "--tag-id", "tag", "--yes"}, map[string]any{"tagId": "tag"}},
		{"add", "add_tags_to_minutes", `{"success":true,"taskUuid":"task","results":[{"tagId":"b","status":"APPLIED"},{"tagId":"a","status":"UNCHANGED"}]}`, []string{"tag", "add", "--id", "task", "--tag-ids", " b ,a,b ", "--locale", "en_US", "--yes"}, map[string]any{"taskUuid": "task", "tagIds": []string{"b", "a"}, "locale": "en_US"}},
		{"remove", "remove_tags_from_minutes", `{"success":true,"taskUuid":"task","results":[{"tagId":"a","status":"UNCHANGED"}]}`, []string{"tag", "remove", "--id", "task", "--tag-ids", "a", "--yes"}, map[string]any{"taskUuid": "task", "tagIds": []string{"a"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(tc.data)}
			got := executeMinutesTemplateTag(t, caller, "", tc.args...)
			requireMinutesTemplateTagOutcome(t, got, "success", 0)
			if len(caller.calls) != 1 {
				t.Fatalf("calls=%#v", caller.calls)
			}
			call := caller.calls[0]
			if call.server != "minutes" || call.tool != tc.tool || !reflect.DeepEqual(call.args, tc.want) {
				t.Fatalf("call=%#v want=minutes/%s %#v", call, tc.tool, tc.want)
			}
			if tc.name == "create" || tc.name == "update" {
				data := got.env["data"].(map[string]any)
				if data["version"] != float64(0) || data["title"] != "保存标题" {
					t.Fatalf("saved DTO lost: %#v", data)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagConfirmationAndDryRun(t *testing.T) {
	for index, args := range [][]string{
		{"template", "create", "--title", "标题", "--prompt", "提示词"},
		{"template", "update", "--template-key", "key", "--brief-intro", ""},
		{"template", "delete", "--template-key", "key"},
		{"template", "set-default", "--template-key", "key"},
		{"tag", "create", "--display-name", "名称"},
		{"tag", "delete", "--tag-id", "tag"},
		{"tag", "add", "--id", "task", "--tag-ids", "a"},
		{"tag", "remove", "--id", "task", "--tag-ids", "a"},
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			caller := &minutesTemplateTagCaller{}
			got := executeMinutesTemplateTag(t, caller, "", args...)
			if !apperrors.IsConfirmationRequired(got.err) || got.code != 3 || len(caller.calls) != 0 {
				t.Fatalf("confirmation: %+v calls=%#v", got, caller.calls)
			}
			got = executeMinutesTemplateTag(t, caller, "", append(append([]string{}, args...), "--dry-run")...)
			requireMinutesTemplateTagOutcome(t, got, "success", 0)
			if len(caller.calls) != 0 || got.env["dry_run"] != true || got.env["data"].(map[string]any)["executed"] != false {
				t.Fatalf("dry run: %+v calls=%#v", got, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"extra positional", []string{"template", "list-system", "unexpected"}},
		{"unknown flag", []string{"template", "list-system", "--unexpected"}},
		{"bad integer", []string{"template", "list-system", "--limit", "x"}},
		{"create title omitted", []string{"template", "create", "--prompt", "prompt"}},
		{"create prompt omitted", []string{"template", "create", "--title", "title"}},
		{"update key omitted", []string{"template", "update", "--title", "title"}},
		{"delete key omitted", []string{"template", "delete"}},
		{"tag name omitted", []string{"tag", "create"}},
		{"tag delete ID omitted", []string{"tag", "delete"}},
		{"tag add task omitted", []string{"tag", "add", "--tag-ids", "a"}},
		{"tag remove task omitted", []string{"tag", "remove", "--tag-ids", "a"}},
		{"default neither", []string{"template", "set-default"}},
		{"default both", []string{"template", "set-default", "--template-id", "official", "--template-key", "key"}},
		{"default diy", []string{"template", "set-default", "--template-id", "diy"}},
		{"default empty alternate", []string{"template", "set-default", "--template-id", "official", "--template-key", ""}},
		{"update absent", []string{"template", "update", "--template-key", "key"}},
		{"tag ids missing", []string{"tag", "add", "--id", "task"}},
		{"tag ids empty item", []string{"tag", "add", "--id", "task", "--tag-ids", "a,,b"}},
		{"tag ids whitespace item", []string{"tag", "remove", "--id", "task", "--tag-ids", "a,  ,b"}},
		{"tag ids malformed CSV", []string{"tag", "add", "--id", "task", "--tag-ids", "\"unclosed"}},
		{"tag ids multiline", []string{"tag", "remove", "--id", "task", "--tag-ids", "a\nb"}},
		{"tag ids raw21 duplicate", []string{"tag", "add", "--id", "task", "--tag-ids", strings.TrimSuffix(strings.Repeat("a,", 21), ",")}},
		{"tag name CJK51", []string{"tag", "create", "--display-name", strings.Repeat("字", 51)}},
		{"tag name emoji26", []string{"tag", "create", "--display-name", strings.Repeat("😀", 26)}},
		{"title151", []string{"template", "create", "--title", strings.Repeat("字", 151), "--prompt", "prompt"}},
	}
	for _, tc := range []struct {
		name   string
		prefix []string
		field  string
	}{
		{"create title", []string{"template", "create", "--prompt", "prompt"}, "title"},
		{"create prompt", []string{"template", "create", "--title", "title"}, "prompt"},
		{"update key", []string{"template", "update", "--title", "title"}, "template-key"},
		{"update title", []string{"template", "update", "--template-key", "key"}, "title"},
		{"update prompt", []string{"template", "update", "--template-key", "key"}, "prompt"},
		{"delete key", []string{"template", "delete"}, "template-key"},
		{"default official", []string{"template", "set-default"}, "template-id"},
		{"default custom", []string{"template", "set-default"}, "template-key"},
		{"create tag", []string{"tag", "create"}, "display-name"},
		{"delete tag", []string{"tag", "delete"}, "tag-id"},
		{"add task", []string{"tag", "add", "--tag-ids", "a"}, "id"},
		{"remove task", []string{"tag", "remove", "--tag-ids", "a"}, "id"},
		{"tag IDs", []string{"tag", "add", "--id", "task"}, "tag-ids"},
	} {
		for _, value := range []string{"", "   "} {
			args := append(append([]string{}, tc.prefix...), "--"+tc.field, value)
			cases = append(cases, struct {
				name string
				args []string
			}{tc.name + fmt.Sprintf("=%q", value), args})
		}
	}
	for _, kind := range []string{"list-system", "list-custom"} {
		maximum := "51"
		if kind == "list-custom" {
			maximum = "11"
		}
		for _, limit := range []string{"0", "-1", maximum} {
			cases = append(cases, struct {
				name string
				args []string
			}{kind + " limit " + limit, []string{"template", kind, "--limit", limit}})
		}
		for _, cursor := range []string{"", "  "} {
			cases = append(cases, struct {
				name string
				args []string
			}{kind + fmt.Sprintf(" cursor=%q", cursor), []string{"template", kind, "--cursor", cursor}})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{}
			got := executeMinutesTemplateTag(t, caller, "", tc.args...)
			var typed *apperrors.Error
			if got.err == nil || !errors.As(got.err, &typed) || typed.Category != apperrors.CategoryValidation || got.code != 3 || apperrors.IsConfirmationRequired(got.err) || len(caller.calls) != 0 {
				t.Fatalf("local validation=%+v typed=%#v calls=%#v", got, typed, caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagInputPresenceAndBoundaries(t *testing.T) {
	const prompt = "  原样 保留\n\n行动项😀  \n"
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		args  []string
		input string
		want  map[string]any
	}{
		{"system1", []string{"template", "list-system", "--limit", "1"}, "", map[string]any{"maxResults": float64(1), "locale": "zh_CN"}},
		{"system50", []string{"template", "list-system", "--limit", "50"}, "", map[string]any{"maxResults": float64(50), "locale": "zh_CN"}},
		{"custom1", []string{"template", "list-custom", "--limit", "1"}, "", map[string]any{"maxResults": float64(1)}},
		{"custom10", []string{"template", "list-custom", "--limit", "10"}, "", map[string]any{"maxResults": float64(10)}},
		{"custom default", []string{"template", "list-custom"}, "", map[string]any{"maxResults": float64(5)}},
		{"create omit brief", []string{"template", "create", "--title", "标题", "--prompt", prompt}, "", map[string]any{"title": "标题", "prompt": prompt}},
		{"create empty brief", []string{"template", "create", "--title", "标题", "--prompt", prompt, "--brief-intro", ""}, "", map[string]any{"title": "标题", "prompt": prompt}},
		{"update empty brief", []string{"template", "update", "--template-key", "key", "--brief-intro", ""}, "", map[string]any{"templateKey": "key", "briefIntro": ""}},
		{"update only prompt", []string{"template", "update", "--template-key", "key", "--prompt", prompt}, "", map[string]any{"templateKey": "key", "prompt": prompt}},
		{"create file prompt", []string{"template", "create", "--title", "标题", "--prompt", "@" + promptPath}, "", map[string]any{"title": "标题", "prompt": prompt}},
		{"update stdin prompt", []string{"template", "update", "--template-key", "key", "--prompt", "-"}, prompt, map[string]any{"templateKey": "key", "prompt": prompt}},
		{"title150", []string{"template", "create", "--title", strings.Repeat("字", 150), "--prompt", "prompt"}, "", map[string]any{"title": strings.Repeat("字", 150), "prompt": "prompt"}},
		{"tag CJK50", []string{"tag", "create", "--display-name", strings.Repeat("字", 50)}, "", map[string]any{"displayName": strings.Repeat("字", 50)}},
		{"tag emoji25", []string{"tag", "create", "--display-name", strings.Repeat("😀", 25)}, "", map[string]any{"displayName": strings.Repeat("😀", 25)}},
		{"tag20 duplicate", []string{"tag", "add", "--id", "task", "--tag-ids", strings.TrimSuffix(strings.Repeat("a,", 20), ",")}, "", map[string]any{"taskUuid": "task", "tagIds": []any{"a"}}},
		{"tag NBSP opaque ID", []string{"tag", "remove", "--id", "task", "--tag-ids", " a ,\u00a0b\u00a0,a "}, "", map[string]any{"taskUuid": "task", "tagIds": []any{"a", "\u00a0b\u00a0"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{}
			args := append(append([]string{}, tc.args...), "--dry-run")
			got := executeMinutesTemplateTag(t, caller, tc.input, args...)
			requireMinutesTemplateTagOutcome(t, got, "success", 0)
			if len(caller.calls) != 0 {
				t.Fatalf("dry-run called backend: %#v", caller.calls)
			}
			data := got.env["data"].(map[string]any)
			if !reflect.DeepEqual(data["arguments"], tc.want) {
				t.Fatalf("args=%#v want=%#v", data["arguments"], tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagPagination(t *testing.T) {
	cases := []struct {
		name, command, body, cursor string
		valid, exhausted            bool
		count                       int
		next                        string
	}{
		{"system nonempty", "list-system", `{"items":[{"templateId":"s","title":"系统","brief":""}],"hasMore":true,"nextToken":" opaque== "}`, "", true, false, 1, " opaque== "},
		{"custom empty continues", "list-custom", `{"items":[],"hasMore":true,"nextToken":"next"}`, "previous", true, false, 0, "next"},
		{"empty terminal missing token", "list-system", `{"items":[],"hasMore":false}`, "", true, true, 0, ""},
		{"empty terminal null token", "list-custom", `{"items":[],"hasMore":false,"nextToken":null}`, "", true, true, 0, ""},
		{"custom prompt and version0", "list-custom", `{"items":[{"templateKey":"key","title":"自定义","prompt":"  首行\n\n第二行😀  ","version":0}],"hasMore":false}`, "", true, true, 1, ""},
		{"missing items", "list-system", `{"hasMore":false}`, "", false, false, 0, ""},
		{"null items", "list-system", `{"items":null,"hasMore":false}`, "", false, false, 0, ""},
		{"wrong item", "list-system", `{"items":[true],"hasMore":false}`, "", false, false, 0, ""},
		{"missing hasMore", "list-custom", `{"items":[]}`, "", false, false, 0, ""},
		{"typed hasMore", "list-custom", `{"items":[],"hasMore":"false"}`, "", false, false, 0, ""},
		{"missing next", "list-system", `{"items":[],"hasMore":true}`, "", false, false, 0, ""},
		{"null next", "list-system", `{"items":[],"hasMore":true,"nextToken":null}`, "", false, false, 0, ""},
		{"empty next", "list-system", `{"items":[],"hasMore":true,"nextToken":""}`, "", false, false, 0, ""},
		{"blank next", "list-system", `{"items":[],"hasMore":true,"nextToken":"  "}`, "", false, false, 0, ""},
		{"typed next", "list-system", `{"items":[],"hasMore":true,"nextToken":1}`, "", false, false, 0, ""},
		{"same next", "list-system", `{"items":[],"hasMore":true,"nextToken":"same"}`, "same", false, false, 0, ""},
		{"contradictory next", "list-system", `{"items":[],"hasMore":false,"nextToken":"next"}`, "", false, false, 0, ""},
		{"system missing brief", "list-system", `{"items":[{"templateId":"s","title":"系统"}],"hasMore":false}`, "", false, false, 0, ""},
		{"system blank identity", "list-system", `{"items":[{"templateId":" ","title":"系统","brief":""}],"hasMore":false}`, "", false, false, 0, ""},
		{"custom wrong version", "list-custom", `{"items":[{"templateKey":"key","title":"自定义","prompt":"提示","version":"0"}],"hasMore":false}`, "", false, false, 0, ""},
		{"custom negative version", "list-custom", `{"items":[{"templateKey":"key","title":"自定义","prompt":"提示","version":-1}],"hasMore":false}`, "", false, false, 0, ""},
		{"custom fractional version", "list-custom", `{"items":[{"templateKey":"key","title":"自定义","prompt":"提示","version":0.5}],"hasMore":false}`, "", false, false, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(tc.body)}
			args := []string{"template", tc.command}
			if tc.cursor != "" {
				args = append(args, "--cursor", tc.cursor)
			}
			got := executeMinutesTemplateTag(t, caller, "", args...)
			if len(caller.calls) != 1 {
				t.Fatalf("requests=%d", len(caller.calls))
			}
			if !tc.valid {
				requireMinutesTemplateTagOutcome(t, got, "failure", 1)
				return
			}
			requireMinutesTemplateTagOutcome(t, got, "success", 0)
			meta := got.env["meta"].(map[string]any)
			pagination := meta["pagination"].(map[string]any)
			if meta["count"] != float64(tc.count) || pagination["endpoint_exhausted"] != tc.exhausted {
				t.Fatalf("meta=%#v", meta)
			}
			if tc.next != "" {
				if pagination["next_token"] != tc.next {
					t.Fatalf("pagination=%#v", pagination)
				}
			} else if _, ok := pagination["next_token"]; ok {
				t.Fatalf("terminal token=%#v", pagination)
			}
			data := got.env["data"].(map[string]any)
			if len(data) != 1 || data["items"] == nil {
				t.Fatalf("page transport fields leaked into data=%#v", data)
			}
			if tc.name == "custom prompt and version0" {
				item := data["items"].([]any)[0].(map[string]any)
				if item["version"] != float64(0) || item["prompt"] != "  首行\n\n第二行😀  " {
					t.Fatalf("item=%#v", item)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagNullableAndSavedReceipts(t *testing.T) {
	for _, wasDefault := range []string{"true", "false", "null", ""} {
		t.Run("wasDefault="+wasDefault, func(t *testing.T) {
			field := ""
			if wasDefault != "" {
				field = `,"wasDefault":` + wasDefault
			}
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":true,"templateKey":"key"` + field + `}`)}
			got := executeMinutesTemplateTag(t, caller, "", "template", "delete", "--template-key", "key", "--yes")
			requireMinutesTemplateTagOutcome(t, got, "success", 0)
			data := got.env["data"].(map[string]any)
			value, exists := data["wasDefault"]
			if !exists || ((wasDefault == "" || wasDefault == "null") && value != nil) || (wasDefault == "true" && value != true) || (wasDefault == "false" && value != false) {
				t.Fatalf("nullable value=%#v", data)
			}
		})
	}
	for _, keyField := range []string{"", `,"templateKey":null`} {
		t.Run("official"+keyField, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":true,"templateId":"official"` + keyField + `}`)}
			got := executeMinutesTemplateTag(t, caller, "", "template", "set-default", "--template-id", "official", "--yes")
			requireMinutesTemplateTagOutcome(t, got, "success", 0)
			data := got.env["data"].(map[string]any)
			value, exists := data["templateKey"]
			if !exists || value != nil {
				t.Fatalf("nullable key=%#v", data)
			}
		})
	}
	t.Run("personal default", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":true,"templateId":"diy","templateKey":"key"}`)}
		got := executeMinutesTemplateTag(t, caller, "", "template", "set-default", "--template-key", "key", "--yes")
		requireMinutesTemplateTagOutcome(t, got, "success", 0)
		if !reflect.DeepEqual(caller.calls[0].args, map[string]any{"templateKey": "key"}) {
			t.Fatalf("request=%#v", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageMinutesTemplateTagMalformedWriteReceipts(t *testing.T) {
	cases := []struct {
		name, body string
		args       []string
	}{
		{"nonJSON", "not JSON", []string{"tag", "create", "--display-name", "名称"}},
		{"nil envelope", "null", []string{"tag", "create", "--display-name", "名称"}},
		{"multiple JSON", `{} {}`, []string{"tag", "create", "--display-name", "名称"}},
		{"missing success", `{"result":{}}`, []string{"tag", "create", "--display-name", "名称"}},
		{"string success", `{"success":"true","result":{}}`, []string{"tag", "create", "--display-name", "名称"}},
		{"missing result", `{"success":true}`, []string{"tag", "create", "--display-name", "名称"}},
		{"null result", `{"success":true,"result":null}`, []string{"tag", "create", "--display-name", "名称"}},
		{"tag missing saved name", minutesTemplateTagReceipt(`{"success":true,"tagId":"tag"}`), []string{"tag", "create", "--display-name", "名称"}},
		{"tag business success string", minutesTemplateTagReceipt(`{"success":"true","tagId":"tag","displayName":"名称"}`), []string{"tag", "create", "--display-name", "名称"}},
		{"template wrong version", minutesTemplateTagReceipt(`{"success":true,"templateKey":"key","title":"标题","version":"0"}`), []string{"template", "create", "--title", "标题", "--prompt", "提示"}},
		{"template negative version", minutesTemplateTagReceipt(`{"success":true,"templateKey":"key","title":"标题","version":-1}`), []string{"template", "create", "--title", "标题", "--prompt", "提示"}},
		{"template mismatched update", minutesTemplateTagReceipt(`{"success":true,"templateKey":"other","title":"标题","version":0}`), []string{"template", "update", "--template-key", "key", "--title", "标题"}},
		{"template mismatched delete", minutesTemplateTagReceipt(`{"success":true,"templateKey":"other"}`), []string{"template", "delete", "--template-key", "key"}},
		{"template typed wasDefault", minutesTemplateTagReceipt(`{"success":true,"templateKey":"key","wasDefault":"false"}`), []string{"template", "delete", "--template-key", "key"}},
		{"official empty key", minutesTemplateTagReceipt(`{"success":true,"templateId":"official","templateKey":""}`), []string{"template", "set-default", "--template-id", "official"}},
		{"official mismatched ID", minutesTemplateTagReceipt(`{"success":true,"templateId":"other","templateKey":null}`), []string{"template", "set-default", "--template-id", "official"}},
		{"personal wrong ID", minutesTemplateTagReceipt(`{"success":true,"templateId":"official","templateKey":"key"}`), []string{"template", "set-default", "--template-key", "key"}},
		{"personal wrong key", minutesTemplateTagReceipt(`{"success":true,"templateId":"diy","templateKey":"other"}`), []string{"template", "set-default", "--template-key", "key"}},
		{"tag mismatch delete", minutesTemplateTagReceipt(`{"success":true,"tagId":"other"}`), []string{"tag", "delete", "--tag-id", "tag"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: tc.body}
			got := executeMinutesTemplateTag(t, caller, "", append(append([]string{}, tc.args...), "--yes")...)
			requireMinutesTemplateTagOutcome(t, got, "failure", 1)
			if len(caller.calls) != 1 {
				t.Fatalf("malformed response replayed: %#v", caller.calls)
			}
			errInfo := got.env["error"].(map[string]any)
			if errInfo["subtype"] != "write_outcome_unknown" || errInfo["retryable"] == true || !strings.Contains(fmt.Sprint(errInfo["hint"]), "不自动重试") {
				t.Fatalf("unsafe failure=%#v", errInfo)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagRelationLedger(t *testing.T) {
	cases := []struct {
		name, statuses, success, outcome string
		code                             int
	}{
		{"applied", "APPLIED", "true", "success", 0},
		{"unchanged", "UNCHANGED", "true", "success", 0},
		{"mixed complete", "APPLIED,UNCHANGED", "true", "success", 0},
		{"applied partial", "APPLIED,FAILED,NOT_PROCESSED", "false", "partial_failure", 7},
		{"unchanged partial", "UNCHANGED,FAILED,NOT_PROCESSED", "false", "partial_failure", 7},
		{"failed and stopped", "FAILED,NOT_PROCESSED", "false", "failure", 1},
		{"failed only", "FAILED", "false", "failure", 1},
		{"false with all applied", "APPLIED,UNCHANGED", "false", "failure", 1},
		{"true with failure", "APPLIED,FAILED", "true", "failure", 1},
		{"unprocessed first", "NOT_PROCESSED,FAILED", "false", "failure", 1},
		{"two failures", "FAILED,FAILED", "false", "failure", 1},
		{"applied after failure", "FAILED,APPLIED", "false", "failure", 1},
		{"unknown status", "UNKNOWN", "true", "failure", 1},
	}
	for _, action := range []string{"add", "remove"} {
		for _, tc := range cases {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				statuses := strings.Split(tc.statuses, ",")
				ids := make([]string, len(statuses))
				items := make([]string, len(statuses))
				for i, status := range statuses {
					ids[i] = fmt.Sprintf("tag%d", i)
					items[i] = fmt.Sprintf(`{"tagId":%q,"status":%q}`, ids[i], status)
					if status == "FAILED" {
						items[i] = fmt.Sprintf(`{"tagId":%q,"status":"FAILED","errorCode":"SYSTEM_ERROR","errorMessage":"写入结果无法确认"}`, ids[i])
					}
				}
				body := `{"success":` + tc.success + `,"taskUuid":"task","results":[` + strings.Join(items, ",") + `]}`
				caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(body)}
				got := executeMinutesTemplateTag(t, caller, "", "tag", action, "--id", "task", "--tag-ids", strings.Join(ids, ","), "--yes")
				requireMinutesTemplateTagOutcome(t, got, tc.outcome, tc.code)
				if len(caller.calls) != 1 {
					t.Fatalf("ledger replayed: %#v", caller.calls)
				}
				if tc.outcome == "partial_failure" {
					data := got.env["data"].(map[string]any)
					succeeded := data["succeeded"].([]any)
					failed := data["failed"].([]any)
					unknown := data["unknown"].([]any)
					if data["total"] != float64(3) || len(succeeded) != 1 || len(failed) != 1 || len(unknown) != 1 {
						t.Fatalf("partial partition=%#v", data)
					}
					first := succeeded[0].(map[string]any)
					if first["id"] != "tag0" || first["taskUuid"] != "task" || first["status"] != statuses[0] {
						t.Fatalf("success evidence=%#v", first)
					}
					unknownItem := unknown[0].(map[string]any)
					if unknownItem["id"] != "tag1" || !strings.Contains(fmt.Sprint(unknownItem["reason"]), "写入结果无法确认") {
						t.Fatalf("unknown evidence=%#v", unknownItem)
					}
					failedItem := failed[0].(map[string]any)
					errInfo := failedItem["error"].(map[string]any)
					if failedItem["id"] != "tag2" || errInfo["subtype"] != "not_processed" || errInfo["execution_started"] != false || errInfo["rpc_data"].(map[string]any)["status"] != "NOT_PROCESSED" {
						t.Fatalf("not processed evidence=%#v", failedItem)
					}
				} else if tc.name == "failed only" || tc.name == "failed and stopped" {
					errInfo := got.env["error"].(map[string]any)
					receipt := errInfo["details"].(map[string]any)["receipt"].(map[string]any)
					if receipt["taskUuid"] != "task" || len(receipt["results"].([]any)) != len(statuses) {
						t.Fatalf("all-failed receipt lost=%#v", errInfo)
					}
				}
			})
		}
	}
	for _, body := range []string{
		`{"success":true,"taskUuid":"other","results":[{"tagId":"a","status":"APPLIED"}]}`,
		`{"success":true,"taskUuid":"task","results":[]}`,
		`{"success":true,"taskUuid":"task","results":[{"tagId":"other","status":"APPLIED"}]}`,
		`{"success":true,"taskUuid":"task","results":[true]}`,
		`{"success":false,"taskUuid":"task","results":[{"tagId":"a","status":"FAILED"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(body)}
			got := executeMinutesTemplateTag(t, caller, "", "tag", "add", "--id", "task", "--tag-ids", "a", "--yes")
			requireMinutesTemplateTagOutcome(t, got, "failure", 1)
			if len(caller.calls) != 1 {
				t.Fatalf("calls=%#v", caller.calls)
			}
		})
	}
	for _, body := range []string{
		`{"success":true,"taskUuid":"task","results":[{"tagId":"a","status":"APPLIED"},{"tagId":"a","status":"APPLIED"}]}`,
		`{"success":true,"taskUuid":"task","results":[{"tagId":"b","status":"APPLIED"},{"tagId":"a","status":"APPLIED"}]}`,
		`{"success":true,"taskUuid":"task","results":[{"tagId":"a","status":"APPLIED"},{"tagId":"extra","status":"APPLIED"}]}`,
	} {
		t.Run("invalid ordered ledger "+body, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(body)}
			got := executeMinutesTemplateTag(t, caller, "", "tag", "add", "--id", "task", "--tag-ids", "a,b", "--yes")
			requireMinutesTemplateTagOutcome(t, got, "failure", 1)
			if len(caller.calls) != 1 {
				t.Fatalf("invalid ledger replayed=%#v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesTemplateTagTransportAndServiceFailures(t *testing.T) {
	t.Run("deadline classified once", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{err: context.DeadlineExceeded}
		got := executeMinutesTemplateTag(t, caller, "", "template", "create", "--title", "标题", "--prompt", "提示", "--yes")
		var legacy *CLIError
		if !errors.As(got.err, &legacy) || legacy.Code != CodeNetworkTimeout || got.code != 1 || !errors.Is(got.err, context.DeadlineExceeded) || !strings.Contains(legacy.Suggestion, "不自动重试") || len(caller.calls) != 1 {
			t.Fatalf("timeout=%+v legacy=%#v calls=%#v", got, legacy, caller.calls)
		}
	})
	t.Run("typed error no retry state", func(t *testing.T) {
		after := int64(12)
		next := time.Now().Add(time.Minute)
		original := &apperrors.Error{Category: apperrors.CategoryAPI, Reason: "network_timeout", Message: "超时", Retryable: true, RetryableSet: true, RetryAfterSeconds: &after, NextRetryAt: &next, Actions: []string{"retry"}, Cause: context.DeadlineExceeded}
		caller := &minutesTemplateTagCaller{err: original}
		got := executeMinutesTemplateTag(t, caller, "", "tag", "delete", "--tag-id", "tag", "--yes")
		var typed *apperrors.Error
		if !errors.As(got.err, &typed) || typed.Category != original.Category || typed.Reason != original.Reason || typed.Retryable || !typed.RetryableSet || typed.RetryAfterSeconds != nil || typed.NextRetryAt != nil || len(typed.Actions) != 0 || !errors.Is(got.err, context.DeadlineExceeded) || len(caller.calls) != 1 {
			t.Fatalf("typed timeout=%+v typed=%#v", got, typed)
		}
		if !original.Retryable || original.RetryAfterSeconds == nil || len(original.Actions) != 1 {
			t.Fatal("caller-owned error mutated")
		}
	})
	for _, body := range []string{
		`{"success":false,"errorCode":"INVALID_PARAM","errorMsg":"参数不合法"}`,
		`{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"系统失败"}`,
		`{"success":false,"errorCode":"SERVICE_UNAVAILABLE","errorMsg":"暂时不可用"}`,
	} {
		t.Run(body, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: body}
			got := executeMinutesTemplateTag(t, caller, "", "tag", "create", "--display-name", "名称", "--yes")
			var legacy *CLIError
			if got.err == nil || !errors.As(got.err, &legacy) || legacy.Code != CodeMCPToolError || got.code != 1 || legacy.ServerCode == "" || len(caller.calls) != 1 {
				t.Fatalf("service failure=%+v legacy=%#v calls=%#v", got, legacy, caller.calls)
			}
		})
	}
	t.Run("permission preserved", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{text: `{"success":false,"errorCode":"NO_PERMISSION","errorMsg":"无权限"}`}
		got := executeMinutesTemplateTag(t, caller, "", "tag", "delete", "--tag-id", "tag", "--yes")
		var legacy *CLIError
		if !errors.As(got.err, &legacy) || legacy.Code != CodeAuthPermission || legacy.ServerCode != "NO_PERMISSION" || got.code != 4 || len(caller.calls) != 1 {
			t.Fatalf("permission=%+v legacy=%#v", got, legacy)
		}
	})
	for _, tc := range []struct {
		name   string
		result *edition.ToolResult
	}{
		{"nil tool result", nil},
		{"empty tool content", &edition.ToolResult{}},
		{"non-text content", &edition.ToolResult{Content: []edition.ContentBlock{{Type: "image", Text: "ignored"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &minutesTemplateTagCaller{customResult: true, result: tc.result}
			got := executeMinutesTemplateTag(t, caller, "", "tag", "delete", "--tag-id", "tag", "--yes")
			if got.code == 0 || len(caller.calls) != 1 {
				t.Fatalf("empty MCP reply falsely succeeded or replayed=%+v calls=%#v", got, caller.calls)
			}
			if got.err == nil {
				requireMinutesTemplateTagOutcome(t, got, "failure", 1)
			}
		})
	}
	t.Run("caller dry run", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{dry: true}
		got := executeMinutesTemplateTag(t, caller, "", "tag", "create", "--display-name", "名称", "--yes")
		requireMinutesTemplateTagOutcome(t, got, "success", 0)
		if len(caller.calls) != 0 {
			t.Fatalf("dry-run caller invoked=%#v", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageMinutesTemplateTagProjectionDoesNotReplay(t *testing.T) {
	for _, flags := range [][]string{{"--fields", "tagId"}, {"--jq", ".data.tagId"}, {"--format", "raw"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":true,"tagId":"tag","displayName":"保存名称"}`)}
			args := append([]string{"tag", "create", "--display-name", "名称", "--yes"}, flags...)
			got := executeMinutesTemplateTag(t, caller, "", args...)
			if got.err != nil || got.code != 0 || len(caller.calls) != 1 || !strings.Contains(got.stdout, "tag") || got.stderr != "" {
				t.Fatalf("projection=%+v calls=%#v", got, caller.calls)
			}
		})
	}
	t.Run("partial jq", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":false,"taskUuid":"task","results":[{"tagId":"a","status":"APPLIED"},{"tagId":"b","status":"FAILED","errorCode":"SYSTEM_ERROR","errorMessage":"结果未知"}]}`)}
		got := executeMinutesTemplateTag(t, caller, "", "tag", "add", "--id", "task", "--tag-ids", "a,b", "--yes", "--jq", ".data.unknown[0].id")
		if got.err != nil || got.code != 7 || len(caller.calls) != 1 || !strings.Contains(got.stdout, "b") || got.stderr != "" {
			t.Fatalf("partial projection=%+v calls=%#v", got, caller.calls)
		}
	})
	t.Run("failure bypasses projections", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":true,"tagId":"other"}`)}
		got := executeMinutesTemplateTag(t, caller, "", "tag", "delete", "--tag-id", "tag", "--yes", "--jq", "empty", "--fields", "nonexistent")
		requireMinutesTemplateTagOutcome(t, got, "failure", 1)
		if len(caller.calls) != 1 {
			t.Fatalf("failure replayed=%#v", caller.calls)
		}
	})
	t.Run("human failure stderr", func(t *testing.T) {
		caller := &minutesTemplateTagCaller{text: minutesTemplateTagReceipt(`{"success":true,"tagId":"other"}`)}
		got := executeMinutesTemplateTag(t, caller, "", "tag", "delete", "--tag-id", "tag", "--yes", "--format", "table")
		if got.err != nil || got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, "Error:") || len(caller.calls) != 1 {
			t.Fatalf("human failure=%+v", got)
		}
	})
}

func TestCrossPlatformCoverageMinutesTemplateTagClassifiedErrors(t *testing.T) {
	for _, category := range []apperrors.Category{apperrors.CategoryAPI, apperrors.CategoryAuth, apperrors.CategoryValidation} {
		t.Run(string(category), func(t *testing.T) {
			original := &apperrors.Error{Category: category, Reason: "known_reason", Message: "参数 text does not choose classification", Cause: context.Canceled}
			caller := &minutesTemplateTagCaller{err: original}
			got := executeMinutesTemplateTag(t, caller, "", "template", "update", "--template-key", "key", "--title", "标题", "--yes")
			var typed *apperrors.Error
			if !errors.As(got.err, &typed) || typed.Category != category || typed.Reason != "known_reason" || got.code != original.ExitCode() || !errors.Is(got.err, context.Canceled) || len(caller.calls) != 1 {
				t.Fatalf("classification=%+v typed=%#v", got, typed)
			}
		})
	}
	t.Run("read keeps typed timeout guidance", func(t *testing.T) {
		original := &apperrors.Error{Category: apperrors.CategoryAPI, Reason: "network_timeout", Message: "timeout", Retryable: true, RetryableSet: true, Cause: context.DeadlineExceeded}
		caller := &minutesTemplateTagCaller{err: original}
		got := executeMinutesTemplateTag(t, caller, "", "template", "list-custom")
		if got.err != original || got.code != 1 || len(caller.calls) != 1 {
			t.Fatalf("read timeout=%+v calls=%#v", got, caller.calls)
		}
	})
}

func TestCrossPlatformCoverageMinutesTemplateTagDefensiveBoundaries(t *testing.T) {
	t.Run("Java trim removes control-only ID", func(t *testing.T) {
		// NUL is not whitespace to strings.TrimSpace, but Java String.trim
		// removes it. The normalized ID must still be checked for emptiness.
		for _, tc := range []struct {
			raw      string
			position int
		}{{"\x00", 1}, {"a,a,\x00", 3}} {
			value, err := minutesTagIDs(tc.raw)
			var typed *apperrors.Error
			if value != nil || !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 {
				t.Fatalf("control-only ID accepted: raw=%q value=%#v err=%v", tc.raw, value, err)
			}
			if want := fmt.Sprintf("--tag-ids第%d项为空白", tc.position); typed.Message != want {
				t.Fatalf("raw=%q message=%q, want %q", tc.raw, typed.Message, want)
			}
		}
	})
	t.Run("unclassified error identity retained", func(t *testing.T) {
		original := errors.New("opaque transport error")
		if got := minutesMCPNonRetryableError(original); got != original {
			t.Fatalf("bare error replaced: got=%v original=%v", got, original)
		}
	})
	t.Run("undeclared write tool rejected", func(t *testing.T) {
		if err := validateMinutesMCPWrite("undeclared_minutes_tool", map[string]any{"success": true}, nil); err == nil {
			t.Fatal("undeclared write tool accepted")
		}
	})
	t.Run("duplicate normalized ledger fails closed", func(t *testing.T) {
		// The CLI transform prevents this input. Retain the mapper's own
		// cross-channel identity check if its caller violates that boundary.
		data := map[string]any{
			"success":  false,
			"taskUuid": "task",
			"results": []any{
				map[string]any{"tagId": "a", "status": "APPLIED"},
				map[string]any{"tagId": "a", "status": "FAILED", "errorCode": "SYSTEM_ERROR", "errorMessage": "写入结果未知"},
			},
		}
		result, err := minutesMCPTagRelation("add_tags_to_minutes", data, map[string]any{"taskUuid": "task", "tagIds": []string{"a", "a"}})
		if err != nil || result == nil || result.Outcome() != output.OutcomeFailure || result.ExitCode() != 1 {
			t.Fatalf("duplicate ledger accepted: result=%#v err=%v", result, err)
		}
		envelope, err := output.EnvelopeFromResult(result)
		if err != nil || envelope.Error == nil || envelope.Data != nil || envelope.Error.Subtype != "write_outcome_unknown" {
			t.Fatalf("invalid defensive failure envelope=%#v err=%v", envelope, err)
		}
	})
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func TestCrossPlatformCoverageAitableCommentDispatch(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		tool     string
		wantArgs map[string]any
	}{
		{
			name:     "list with cursor",
			args:     []string{"comment", "list", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--limit", "25", "--cursor", "next-1"},
			tool:     "list_comments",
			wantArgs: map[string]any{"baseId": "base-1", "tableId": "table-1", "recordId": "record-1", "pageSize": 25, "nextToken": "next-1"},
		},
		{
			name:     "list uses default page size",
			args:     []string{"comment", "list", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1"},
			tool:     "list_comments",
			wantArgs: map[string]any{"baseId": "base-1", "tableId": "table-1", "recordId": "record-1", "pageSize": 50},
		},
		{
			name: "create plain text",
			args: []string{"comment", "create", "--base", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--content", " 请确认\n第二行 "},
			tool: "create_comment",
			wantArgs: map[string]any{
				"baseId": "base-1", "tableId": "table-1", "recordId": "record-1",
				"richContent": []any{map[string]any{"type": "text", "text": " 请确认\n第二行 "}},
			},
		},
		{
			name: "reply maps reply key",
			args: []string{"comment", "reply", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--topic-id", "topic-1", "--comment-key", "comment-1", "--content", "已确认"},
			tool: "reply_comment",
			wantArgs: map[string]any{
				"baseId": "base-1", "tableId": "table-1", "recordId": "record-1", "topicId": "topic-1", "replyCommentKey": "comment-1",
				"richContent": []any{map[string]any{"type": "text", "text": "已确认"}},
			},
		},
		{
			name: "reply keeps scoped compatibility alias",
			args: []string{"comment", "reply", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--topic-id", "topic-1", "--reply-comment-key", "comment-1", "--content", "已确认"},
			tool: "reply_comment",
			wantArgs: map[string]any{
				"baseId": "base-1", "tableId": "table-1", "recordId": "record-1", "topicId": "topic-1", "replyCommentKey": "comment-1",
				"richContent": []any{map[string]any{"type": "text", "text": "已确认"}},
			},
		},
		{
			name: "update rich content",
			args: []string{"comment", "update", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--topic-id", "topic-1", "--comment-key", "comment-1", "--rich-content", `[{"type":"text","text":"请看 "},{"type":"mention","userId":"user-1","corpId":"corp-1"},{"type":"image","url":"/core/api/resources/res_1/detail","width":800,"height":600}]`},
			tool: "update_comment",
			wantArgs: map[string]any{
				"baseId": "base-1", "tableId": "table-1", "recordId": "record-1", "topicId": "topic-1", "commentKey": "comment-1",
				"richContent": []map[string]any{
					{"type": "text", "text": "请看 "},
					{"type": "mention", "userId": "user-1", "corpId": "corp-1"},
					{"type": "image", "url": "/core/api/resources/res_1/detail", "width": json.Number("800"), "height": json.Number("600")},
				},
			},
		},
		{
			name: "empty text node is valid beside a mention",
			args: []string{"comment", "create", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--rich-content", `[{"type":"text","text":""},{"type":"mention","userId":"user-1"}]`},
			tool: "create_comment",
			wantArgs: map[string]any{
				"baseId": "base-1", "tableId": "table-1", "recordId": "record-1",
				"richContent": []map[string]any{{"type": "text", "text": ""}, {"type": "mention", "userId": "user-1"}},
			},
		},
		{
			name:     "delete",
			args:     []string{"comment", "delete", "--base-id", "base-1", "--table-id", "table-1", "--record-id", "record-1", "--topic-id", "topic-1", "--comment-key", "comment-1", "--yes"},
			tool:     "delete_comment",
			wantArgs: map[string]any{"baseId": "base-1", "tableId": "table-1", "recordId": "record-1", "topicId": "topic-1", "commentKey": "comment-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			if err := runAitableCoverageCommand(t, caller, test.args...); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(caller.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(caller.calls))
			}
			call := caller.calls[0]
			if call.tool != test.tool {
				t.Fatalf("call = %s/%s, want tool %s", call.server, call.tool, test.tool)
			}
			if !reflect.DeepEqual(call.args, test.wantArgs) {
				t.Fatalf("args = %#v, want %#v", call.args, test.wantArgs)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableCommentValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing content", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r"}, wantErr: "请指定 --content、--rich-content 之一"},
		{name: "two content forms", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--content", "x", "--rich-content", `[{"type":"text","text":"y"}]`}, wantErr: "只能指定其一"},
		{name: "blank content", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--content", "  "}, wantErr: "请指定 --content、--rich-content 之一"},
		{name: "bad page size", args: []string{"comment", "list", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--limit", "101"}, wantErr: "1-100"},
		{name: "unknown node", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--rich-content", `[{"type":"link","url":"https://example.com"}]`}, wantErr: "只允许 text、mention 或 image"},
		{name: "external image URL", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--rich-content", `[{"type":"image","url":"https://example.com/a.png"}]`}, wantErr: "/core/api/resources/"},
		{name: "whitespace only rich content", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--rich-content", `[{"type":"text","text":"  \n"}]`}, wantErr: "必须包含非空文本、mention 或 image"},
		{name: "fractional image dimension", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--rich-content", `[{"type":"image","url":"/core/api/resources/image-1/detail","width":1.5}]`}, wantErr: "必须是 1-20000 的整数"},
		{name: "incompatible text fields", args: []string{"comment", "create", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--rich-content", `[{"type":"text","text":"x","userId":"u"}]`}, wantErr: "text 节点不能包含 userId"},
		{name: "reply alias is scoped", args: []string{"comment", "update", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--topic-id", "p", "--reply-comment-key", "c", "--content", "x"}, wantErr: "unknown flag: --reply-comment-key"},
		{name: "delete confirmation", args: []string{"comment", "delete", "--base-id", "b", "--table-id", "t", "--record-id", "r", "--topic-id", "p", "--comment-key", "c"}, wantErr: "--yes"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			err := runAitableCoverageCommand(t, caller, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageAitableCommentRichContentLimits(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "trailing value", raw: `[{"type":"text","text":"x"}] []`, wantErr: "只能包含一个 JSON 数组"},
		{name: "text utf16 limit", raw: `[{"type":"text","text":"` + strings.Repeat("😀", 5001) + `"}]`, wantErr: "10000 个 UTF-16 字符"},
		{name: "mention count", raw: repeatedAitableCommentNodes(`{"type":"mention","userId":"u"}`, 21), wantErr: "mention 节点不能超过 20 个"},
		{name: "image count", raw: repeatedAitableCommentNodes(`{"type":"image","url":"/core/api/resources/image-1/detail"}`, 10), wantErr: "image 节点不能超过 9 个"},
		{name: "node count", raw: repeatedAitableCommentNodes(`{"type":"text","text":"x"}`, 101), wantErr: "节点数必须在 1-100 之间"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseAitableCommentRichContent(test.raw)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func repeatedAitableCommentNodes(node string, count int) string {
	return "[" + strings.TrimSuffix(strings.Repeat(node+",", count), ",") + "]"
}

func TestCrossPlatformCoverageAitableCommentRetryPolicy(t *testing.T) {
	if !isAitableReadRetryTool("list_comments") {
		t.Fatal("list_comments must use the reviewed read retry policy")
	}
	for _, tool := range []string{"create_comment", "reply_comment", "update_comment", "delete_comment"} {
		if isAitableReadRetryTool(tool) {
			t.Fatalf("write tool %s must not be retried", tool)
		}
		caller := &aitableTestCaller{errors: []error{errors.New("timeout: retryable: true")}}
		installAitableDeps(t, caller)
		if err := callAitableCommentTool(newAitableCommentCommand(), tool, map[string]any{}); err == nil {
			t.Fatalf("%s returned nil error", tool)
		}
		if len(caller.calls) != 1 {
			t.Fatalf("%s calls = %d, want 1", tool, len(caller.calls))
		}
	}
}

func TestCrossPlatformCoverageAitableCommentContracts(t *testing.T) {
	root := newAitableCommand()
	tests := []struct {
		path, rpc, effect, confirmation, idempotency string
	}{
		{"aitable comment list", "list_comments", "read", "not_required", "idempotent"},
		{"aitable comment create", "create_comment", "write", "not_required", "non_idempotent"},
		{"aitable comment reply", "reply_comment", "write", "not_required", "non_idempotent"},
		{"aitable comment update", "update_comment", "write", "not_required", "unknown"},
		{"aitable comment delete", "delete_comment", "destructive", "user_required", "unknown"},
	}
	for _, test := range tests {
		leaf := findCLIPath(root, test.path)
		if leaf == nil {
			t.Fatalf("missing leaf %q", test.path)
		}
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Interface == nil || final.Interface.Ref == nil {
			t.Fatalf("%s missing interface contract", test.path)
		}
		if final.Interface.Ref.ProductID != "aitable" || final.Interface.Ref.RPCName != test.rpc {
			t.Fatalf("%s interface = %#v", test.path, final.Interface)
		}
		if final.Safety.Effect != test.effect || final.Safety.Confirmation != test.confirmation || final.Safety.Idempotency != test.idempotency {
			t.Fatalf("%s safety = %#v", test.path, final.Safety)
		}
		if final.Result == nil || len(final.Result.DataSchema) == 0 {
			t.Fatalf("%s missing result schema", test.path)
		}
	}
}

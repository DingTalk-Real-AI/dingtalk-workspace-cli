// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageActiveConversationsPageShapeAndCursorEdges(t *testing.T) {
	tests := []struct {
		name       string
		collection any
		cursor     any
		wantCursor string
		wantReason string
	}{
		{name: "collection object", collection: map[string]any{}, cursor: "next", wantReason: "malformed_collection"},
		{name: "nonterminal null collection", cursor: "next", wantReason: "malformed_collection"},
		{name: "collection scalar item", collection: []any{"unexpected"}, cursor: "next", wantReason: "malformed_item"},
		{name: "collection null item", collection: []any{nil}, cursor: "next", wantReason: "malformed_item"},
		{name: "integer JSON cursor", collection: []any{}, cursor: json.Number("42"), wantCursor: "42"},
		{name: "fractional JSON cursor", collection: []any{}, cursor: json.Number("42.5"), wantReason: "invalid_pagination"},
		{name: "overflow JSON cursor", collection: []any{}, cursor: json.Number("9223372036854775808"), wantReason: "invalid_pagination"},
		{name: "literal null cursor", collection: []any{}, cursor: " NuLl ", wantReason: "missing_next_cursor"},
		{name: "nil string cursor", collection: []any{}, cursor: "<nil>", wantReason: "missing_next_cursor"},
		{name: "nonfinite cursor", collection: []any{}, cursor: math.Inf(1), wantReason: "invalid_pagination"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			groups, more, cursor, err := parseActiveConversationPage(map[string]any{
				"result": map[string]any{
					"conversationMessagesList": tc.collection,
					"hasMore":                  true,
					"nextCursor":               tc.cursor,
				},
			})
			if tc.wantReason != "" {
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Reason != tc.wantReason || groups != nil {
					t.Fatalf("groups=%#v error=%v, want no accepted page and reason %s", groups, err, tc.wantReason)
				}
				return
			}
			if err != nil || !more || cursor != tc.wantCursor || groups == nil || len(groups) != 0 {
				t.Fatalf("groups=%#v more=%t cursor=%q error=%v", groups, more, cursor, err)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsRejectsMalformedMessages(t *testing.T) {
	for name, group := range map[string]map[string]any{
		"missing messages":  {},
		"nonarray messages": {"messages": map[string]any{"createTime": 1788746400000}},
		"scalar message":    {"messages": []any{"unexpected"}},
		"null message":      {"messages": []any{nil}},
		"empty message":     {"messages": []any{map[string]any{}}},
	} {
		t.Run(name, func(t *testing.T) {
			messages, err := activeConversationMessages(group)
			if err == nil || messages != nil {
				t.Fatalf("malformed messages were accepted: messages=%#v error=%v", messages, err)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsFillsMetadataWithoutRegressingLatestTime(t *testing.T) {
	caller := &larkAlignmentCaller{sequenceResponses: map[string][]string{
		activeConversationsOperation: {
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid-b","messages":[{"createTime":"2026-09-07T12:00:00+08:00"}]},{"openConversationId":"cid-a","title":"Alice","singleChat":true,"messages":[{"createTime":"2026-09-07T12:00:00+08:00"}]}],"hasMore":true,"nextCursor":"cursor-2"}}`,
			`{"result":{"conversationMessagesList":[{"openConversationId":"cid-b","title":"Recovered group","singleChat":false,"messages":[{"createTime":"2026-09-07T11:00:00+08:00"}]},{"openConversationId":"cid-b","title":"Stale name","singleChat":false,"messages":[{"createTime":"2026-09-07T10:00:00+08:00"}]}],"hasMore":false}}`,
		},
	}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08"})
	if err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	rows := data["conversations"].([]any)
	if len(rows) != 2 || data["unknownTypeCount"] != float64(0) || data["complete"] != true {
		t.Fatalf("aggregate=%#v", data)
	}
	first, second := rows[0].(map[string]any), rows[1].(map[string]any)
	if first["conversationId"] != "cid-a" || second["conversationId"] != "cid-b" {
		t.Fatalf("equal timestamps must sort by stable conversation ID: %#v", rows)
	}
	if second["name"] != "Recovered group" || second["nameKnown"] != true || second["type"] != "group" || second["latestMessageTime"] != "2026-09-07T12:00:00+08:00" {
		t.Fatalf("metadata fill regressed the verified latest message or name: %#v", second)
	}
}

func TestCrossPlatformCoverageActiveConversationsTimestampRepresentations(t *testing.T) {
	const seconds int64 = 1788746400
	const milliseconds int64 = 1788746400123
	tests := []struct {
		name  string
		value any
		want  time.Time
	}{
		{name: "numeric seconds string", value: " 1788746400 ", want: time.Unix(seconds, 0)},
		{name: "numeric milliseconds string", value: "1788746400123", want: time.UnixMilli(milliseconds)},
		{name: "JSON integer seconds", value: json.Number("1788746400"), want: time.Unix(seconds, 0)},
		{name: "JSON integer milliseconds", value: json.Number("1788746400123"), want: time.UnixMilli(milliseconds)},
		{name: "int seconds", value: int(seconds), want: time.Unix(seconds, 0)},
		{name: "int32 seconds", value: int32(seconds), want: time.Unix(seconds, 0)},
		{name: "int64 milliseconds", value: milliseconds, want: time.UnixMilli(milliseconds)},
		{name: "float32 exact seconds", value: float32(1024), want: time.Unix(1024, 0)},
		{name: "float64 milliseconds", value: float64(milliseconds), want: time.UnixMilli(milliseconds)},
		{name: "fractional message timestamp", value: "2026-09-07T10:00:00.123+08:00", want: time.UnixMilli(milliseconds)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseActiveConversationTimestamp(tc.value)
			if !ok || !got.Equal(tc.want) {
				t.Fatalf("timestamp(%#v)=%s valid=%t, want %s", tc.value, got.Format(time.RFC3339Nano), ok, tc.want.Format(time.RFC3339Nano))
			}
		})
	}
	for name, value := range map[string]any{
		"fractional JSON number": json.Number("1.5"),
		"overflow JSON number":   json.Number("9223372036854775808"),
		"zero integer":           int64(0),
		"negative integer":       int64(-1),
		"zero numeric string":    "0",
		"fractional float":       1.5,
		"negative float":         -1.0,
		"not a number":           math.NaN(),
		"positive infinity":      math.Inf(1),
		"negative infinity":      math.Inf(-1),
		"float overflow":         float64(math.MaxInt64) * 2,
		"missing timestamp":      nil,
		"boolean timestamp":      true,
		"object timestamp":       map[string]any{"seconds": seconds},
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := parseActiveConversationTimestamp(value); ok || !got.IsZero() {
				t.Fatalf("invalid timestamp(%#v)=%s valid=%t", value, got.Format(time.RFC3339Nano), ok)
			}
		})
	}
}

func TestCrossPlatformCoverageActiveConversationsBlankCursorStartsAtFirstPage(t *testing.T) {
	caller := &larkAlignmentCaller{responses: map[string]string{
		activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":false}}`,
	}}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08", "cursor": "   "})
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["cursor"] != "0" || envelope["data"].(map[string]any)["complete"] != true {
		t.Fatalf("blank cursor did not resolve to a complete first-page query: calls=%#v envelope=%#v", caller.calls, envelope)
	}
}

func TestCrossPlatformCoverageActiveConversationsExecuteRejectsInvalidPaginationState(t *testing.T) {
	caller := &larkAlignmentCaller{}
	helpers.InitDepsForTest(t, caller)
	cmd := corecmd.New(shortcut.FromShortcut(ActiveConversations))
	for name, value := range map[string]string{"start": "2026-09-07", "end": "2026-09-08", "page-limit": "0"} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	rt := shortcut.RuntimeContextForTest(cmd, ActiveConversations)
	if err := ActiveConversations.Validate(rt); err == nil {
		t.Fatal("normal command validation must reject a zero page limit")
	}
	// Directly exercise the defensive Execute boundary: a caller bypassing
	// Validate must not publish a resumable result without a continuation token.
	err := ActiveConversations.Execute(rt)
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "invalid_pagination" || len(caller.calls) != 0 {
		t.Fatalf("invalid execution state was accepted: error=%v calls=%#v", err, caller.calls)
	}
}

type activeConversationsInvalidDiagnosticCaller struct {
	*larkAlignmentCaller
	err   error
	calls int
}

func (c *activeConversationsInvalidDiagnosticCaller) CallTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	if c.calls > 1 {
		return nil, c.err
	}
	return c.larkAlignmentCaller.CallTool(ctx, product, tool, args)
}

func TestCrossPlatformCoverageActiveConversationsRejectsInvalidPartialDiagnostics(t *testing.T) {
	// A malformed typed error must not escape as an invalid unified result.
	// Construct it directly because normal error options reject negative delay.
	negativeDelay := int64(-1)
	typed := apperrors.NewAPI("malformed upstream retry metadata").(*apperrors.Error)
	typed.RetryAfterSeconds = &negativeDelay
	caller := &activeConversationsInvalidDiagnosticCaller{
		larkAlignmentCaller: &larkAlignmentCaller{responses: map[string]string{
			activeConversationsOperation: `{"result":{"conversationMessagesList":[],"hasMore":true,"nextCursor":"cursor-2"}}`,
		}},
		err: typed,
	}
	envelope, err := executeActiveConversationsForTest(t, caller, map[string]string{"start": "2026-09-07", "end": "2026-09-08"})
	if err == nil || !strings.Contains(err.Error(), "retry_after_seconds must be non-negative") || envelope != nil || caller.calls != 2 {
		t.Fatalf("invalid partial diagnostic was published: envelope=%#v error=%v calls=%d", envelope, err, caller.calls)
	}
}

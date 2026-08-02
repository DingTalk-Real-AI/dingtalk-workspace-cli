// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package chatmsg

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageQuotedResourcesAndScalarVariants(t *testing.T) {
	quoted := QuotedMessage(map[string]any{
		"quotedMessage": map[string]any{
			"openMessageId": "msg-quoted",
			"msgType":       "file",
			"content":       `{"mediaId":"@quoted-file"}`,
		},
	})
	if quoted["messageType"] != "file" || len(quoted["resourceRefs"].([]map[string]any)) != 1 {
		t.Fatalf("quoted projection = %#v", quoted)
	}
	if firstMessageValue(map[string]any{"a": " ", "b": "value"}, "a", "b") != "value" {
		t.Fatal("blank string did not fall through")
	}
	if Resources(nil) != nil {
		t.Fatal("nil message returned resources")
	}
	resources := Resources(map[string]any{
		"attachments": []map[string]any{
			{"resourceType": "mediaId", "resourceId": "@file-a"},
			{"resourceType": "fileId", "resourceId": "drive-file"},
			{"mediaId": 42, "fileId": 42},
		},
		"content": `[文件] report.txt fileId: drive-file`,
	})
	if len(resources) != 2 ||
		resources[0]["resourceId"] != "@file-a" ||
		resources[1]["resourceId"] != "drive-file" ||
		resources[1]["type"] != "fileId" {
		t.Fatalf("resources = %#v", resources)
	}
	fileDownload := resources[1]["download"].(map[string]any)
	fileArguments := fileDownload["arguments"].(map[string]any)
	if fileDownload["ready"] != true ||
		fileDownload["shortcut"] != "+messages-resource-download" ||
		fileArguments["type"] != "fileId" ||
		fileArguments["resource-id"] != "drive-file" {
		t.Fatalf("file download = %#v", fileDownload)
	}
	if got := resourceIDScalar(42); got != "" {
		t.Fatalf("non-string resource ID = %q", got)
	}
}

func TestCrossPlatformCoverageReactionShapeVariants(t *testing.T) {
	got := Reactions(map[string]any{
		"reactions": []map[string]any{
			{"emoji": " ", "count": 0},
			{"emojiName": "赞", "replyUsers": []string{"u1"}, "replyCount": "1"},
			{"reactionType": "笑", "operators": []any{"u2"}, "reactionCount": json.Number("2")},
		},
	})
	counts := got["counts"].([]map[string]any)
	details := got["details"].([]map[string]any)
	if len(counts) != 2 || len(details) != 2 {
		t.Fatalf("reactions = %#v", got)
	}
	if firstReactionValue(map[string]any{"a": " ", "b": "ok"}, "a", "b") != "ok" {
		t.Fatal("blank reaction value did not fall through")
	}
	if users := reactionUsers(map[string]any{"users": []string{"u1", "u2"}}); !reflect.DeepEqual(users, []any{"u1", "u2"}) {
		t.Fatalf("users = %#v", users)
	}
	for _, tc := range []struct {
		value any
		want  any
	}{
		{int(1), int(1)},
		{int32(2), int32(2)},
		{int64(3), int64(3)},
		{float32(4), float32(4)},
		{float64(5), float64(5)},
		{json.Number("6"), json.Number("6")},
		{"7", "7"},
	} {
		if got := reactionCount(map[string]any{"count": tc.value}, 9); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("reactionCount(%T) = %#v, want %#v", tc.value, got, tc.want)
		}
	}
	if got := Reactions(map[string]any{"reactions": []any{"invalid", map[string]any{}}}); got != nil {
		t.Fatalf("empty reaction rows = %#v", got)
	}
}

func TestCrossPlatformCoveragePaginationVariants(t *testing.T) {
	payload := map[string]any{}
	ApplyMessagePagination(payload, map[string]any{"hasMore": true}, []map[string]any{{}}, "older")
	if _, ok := payload["nextPage"]; ok {
		t.Fatalf("missing time produced next page: %#v", payload)
	}
	if Pagination(nil) != nil {
		t.Fatal("nil pagination was non-nil")
	}
	page := Pagination(map[string]any{"data": map[string]any{
		"has_more":   true,
		"next_token": int64(8),
	}})
	if page["hasMore"] != true || page["nextCursor"] != int64(8) {
		t.Fatalf("page = %#v", page)
	}
	for _, tc := range []struct {
		value any
		want  bool
	}{
		{nil, false},
		{" ", false},
		{"0", false},
		{"cursor", true},
		{int(0), false},
		{int(1), true},
		{int64(0), false},
		{int64(1), true},
		{float64(0), false},
		{float64(1), true},
		{true, true},
	} {
		if got := paginationValuePresent(tc.value); got != tc.want {
			t.Errorf("paginationValuePresent(%#v) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

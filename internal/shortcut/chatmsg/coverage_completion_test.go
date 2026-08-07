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
			{"resourceType": "mediaId", "resourceId": "@file-a", "name": "photo.png"},
			{"resourceType": "fileId", "resourceId": "drive-file", "fileName": "canonical-report.txt"},
			{"mediaId": 42, "fileId": 42},
		},
		"content": `[文件] report.txt fileId: drive-file`,
	})
	if len(resources) != 2 ||
		resources[0]["resourceId"] != "@file-a" ||
		resources[0]["name"] != "photo.png" ||
		resources[1]["resourceId"] != "drive-file" ||
		resources[1]["type"] != "fileId" ||
		resources[1]["name"] != "canonical-report.txt" {
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

func TestCrossPlatformCoverageResourcesExtractsLegacyFileNameWithoutGuessing(t *testing.T) {
	resources := Resources(map[string]any{
		"name":                "sender-name-must-not-leak",
		"openMessageId":       "msg-1",
		"openConversationId":  "cid-1",
		"content":             `[文件] 项目最终报告 2026.pdf fileId: drive-file 注意：如需下载使用旧命令`,
		"unrelatedAttachment": map[string]any{"mediaId": "@image-without-name"},
	})
	if len(resources) != 2 {
		t.Fatalf("resources = %#v", resources)
	}
	if resources[0]["resourceId"] != "@image-without-name" {
		t.Fatalf("first resource = %#v", resources[0])
	}
	if _, leaked := resources[0]["name"]; leaked {
		t.Fatalf("message sender name leaked into media resource: %#v", resources[0])
	}
	if resources[1]["resourceId"] != "drive-file" ||
		resources[1]["name"] != "项目最终报告 2026.pdf" {
		t.Fatalf("file resource = %#v", resources[1])
	}
}

func TestCrossPlatformCoverageProjectionRemovesOnlyLegacyResourceDownloadHint(t *testing.T) {
	legacy := `[文件] 项目最终报告 2026.pdf fileId: drive-file 注意：如需下载使用dws drive download命令下载`
	row := ProjectMessageV1(map[string]any{"content": legacy}, false)
	if row["text"] != `[文件] 项目最终报告 2026.pdf fileId: drive-file` {
		t.Fatalf("projected text = %#v", row["text"])
	}
	resources := row["resourceRefs"].([]map[string]any)
	if len(resources) != 1 ||
		resources[0]["name"] != "项目最终报告 2026.pdf" ||
		resources[0]["download"].(map[string]any)["shortcut"] != "+messages-resource-download" {
		t.Fatalf("projected resources = %#v", resources)
	}
	mediaRow := ProjectMessageV1(map[string]any{
		"openMessageId":      "msg-media",
		"openConversationId": "cid-media",
		"content":            `[图片消息](mediaId=@media) 注意：如需下载使用dws chat message download-media命令下载`,
	}, false)
	if mediaRow["text"] != `[图片消息](mediaId=@media)` {
		t.Fatalf("projected media text = %#v", mediaRow["text"])
	}

	ordinary := `团队规范：注意：如需下载使用dws drive download命令下载`
	if got := ProjectMessageV1(map[string]any{"content": ordinary}, false)["text"]; got != ordinary {
		t.Fatalf("ordinary text was rewritten: got %#v, want %q", got, ordinary)
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

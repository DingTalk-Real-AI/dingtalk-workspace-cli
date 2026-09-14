package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// ---------- formatFeedTime unit tests ----------

func TestCrossPlatformCoverageFormatFeedTimeInvalidJSON(t *testing.T) {
	// JSON 解析失败 → 原样返回原始字符串
	raw := "not-json"
	got := formatFeedTime(raw)
	s, ok := got.(string)
	if !ok || s != raw {
		t.Fatalf("formatFeedTime(invalid) = %v (%T), want raw string %q", got, got, raw)
	}
}

func TestCrossPlatformCoverageFormatFeedTimeEmptyInput(t *testing.T) {
	// 空字符串 → JSON 解析失败 → 原样返回
	got := formatFeedTime("")
	s, ok := got.(string)
	if !ok || s != "" {
		t.Fatalf("formatFeedTime(\"\") = %v (%T), want \"\"", got, got)
	}
}

func TestCrossPlatformCoverageFormatFeedTimeNoFeedsKey(t *testing.T) {
	// 合法 JSON 但没有 feeds 字段 → 返回解析后的 map
	raw := `{"status":"ok","count":0}`
	got := formatFeedTime(raw)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("formatFeedTime(no feeds) = %T, want map[string]any", got)
	}
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
}

func TestCrossPlatformCoverageFormatFeedTimeFeedsNotArray(t *testing.T) {
	// feeds 不是数组 → 返回解析后的 map
	raw := `{"feeds":"unexpected-string"}`
	got := formatFeedTime(raw)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("formatFeedTime(feeds string) = %T, want map[string]any", got)
	}
	if m["feeds"] != "unexpected-string" {
		t.Fatalf("feeds = %v, want unexpected-string", m["feeds"])
	}
}

func TestCrossPlatformCoverageFormatFeedTimeFeedsNull(t *testing.T) {
	// feeds 为 null → 类型断言失败 → 返回解析后的 map
	raw := `{"feeds":null}`
	got := formatFeedTime(raw)
	_, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("formatFeedTime(feeds null) = %T, want map[string]any", got)
	}
}

func TestCrossPlatformCoverageFormatFeedTimeNonMapItem(t *testing.T) {
	// feeds 数组元素不是 object → continue 跳过
	raw := `{"feeds":["string-item", 42, null]}`
	got := formatFeedTime(raw)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	feeds := m["feeds"].([]any)
	if len(feeds) != 3 {
		t.Fatalf("feeds length = %d, want 3", len(feeds))
	}
}

func TestCrossPlatformCoverageFormatFeedTimeTimeMissingOrInvalid(t *testing.T) {
	// time 字段缺失或不是 float64 或 <= 0 → 跳过格式化
	raw := `{"feeds":[
		{"id":"a"},
		{"id":"b","time":"not-a-number"},
		{"id":"c","time":0},
		{"id":"d","time":-1000}
	]}`
	got := formatFeedTime(raw)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	feeds := m["feeds"].([]any)
	for i, f := range feeds {
		item := f.(map[string]any)
		if _, hasTimeMs := item["timeMs"]; hasTimeMs {
			t.Fatalf("feed[%d] should not have timeMs, got %v", i, item)
		}
	}
}

func TestCrossPlatformCoverageFormatFeedTimeNormalPath(t *testing.T) {
	// 正常路径：time 保留原始毫秒时间戳，timeFormatted 被格式化为北京时间
	// 1750067400000 ms = 2025-06-16 17:50 Beijing time (UTC+8)
	tsMs := float64(1750067400000)
	raw := buildFeedJSON(t, []map[string]any{
		{"time": tsMs, "type": float64(1), "id": "f1"},
	})

	got := formatFeedTime(raw)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	feeds := m["feeds"].([]any)
	if len(feeds) != 1 {
		t.Fatalf("feeds length = %d, want 1", len(feeds))
	}
	item := feeds[0].(map[string]any)

	// time 保留原始毫秒时间戳不变
	if ms, ok := item["time"].(float64); !ok || ms != tsMs {
		t.Fatalf("time = %v (%T), want original ms %v", item["time"], item["time"], tsMs)
	}

	// timeFormatted 被格式化为北京时间字符串（硬编码期望值）
	timeStr, ok := item["timeFormatted"].(string)
	if !ok {
		t.Fatalf("timeFormatted = %T, want string", item["timeFormatted"])
	}
	if timeStr != "2025-06-16 17:50" {
		t.Fatalf("timeFormatted = %q, want %q", timeStr, "2025-06-16 17:50")
	}

	// typeLabel 被填充
	if item["typeLabel"] != "更新文档" {
		t.Fatalf("typeLabel = %v, want 更新文档", item["typeLabel"])
	}
}

func TestCrossPlatformCoverageFormatFeedTimeMultipleFeeds(t *testing.T) {
	// 多个 feed，混合有效/无效时间戳
	ts1 := float64(1750067400000)
	ts2 := float64(1700000000000)
	raw := buildFeedJSON(t, []map[string]any{
		{"time": ts1, "type": float64(0), "id": "f1"},
		{"time": ts2, "type": float64(12), "id": "f2"},
		{"id": "f3"}, // 没有 time → 跳过
	})

	got := formatFeedTime(raw)
	m := got.(map[string]any)
	feeds := m["feeds"].([]any)

	// 前两个有时间戳
	for i := 0; i < 2; i++ {
		item := feeds[i].(map[string]any)
		if _, ok := item["time"].(float64); !ok {
			t.Fatalf("feed[%d] time should remain float64, got %T", i, item["time"])
		}
		if _, ok := item["timeFormatted"].(string); !ok {
			t.Fatalf("feed[%d] timeFormatted not string", i)
		}
	}

	// 第三个没有时间戳
	item3 := feeds[2].(map[string]any)
	if _, hasTimeFormatted := item3["timeFormatted"]; hasTimeFormatted {
		t.Fatalf("feed[2] should not have timeFormatted")
	}
}

// ---------- enrichFeedFields unit tests ----------

func TestCrossPlatformCoverageEnrichFeedFieldsPreservesAllFields(t *testing.T) {
	// enrichFeedFields 不删除任何字段，仅新增 typeLabel
	item := map[string]any{
		"parentDoc": map[string]any{"name": "parent"},
		"userInfo":  map[string]any{"nick": "user1"},
		"id":        "f1",
		"type":      float64(1),
	}
	enrichFeedFields(item)
	if _, ok := item["parentDoc"]; !ok {
		t.Fatal("parentDoc should be preserved")
	}
	if _, ok := item["userInfo"]; !ok {
		t.Fatal("userInfo should be preserved")
	}
	if item["id"] != "f1" {
		t.Fatal("id should be preserved")
	}
	if item["typeLabel"] != "更新文档" {
		t.Fatalf("typeLabel = %v, want 更新文档", item["typeLabel"])
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsTypeLabelKnown(t *testing.T) {
	// 已知 type → 添加 typeLabel
	for typeNum, expectedLabel := range feedTypeLabels {
		item := map[string]any{"type": float64(typeNum)}
		enrichFeedFields(item)
		if item["typeLabel"] != expectedLabel {
			t.Fatalf("type %d: typeLabel = %v, want %q", typeNum, item["typeLabel"], expectedLabel)
		}
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsTypeLabelUnknown(t *testing.T) {
	// 未知 type → 不添加 typeLabel
	item := map[string]any{"type": float64(999)}
	enrichFeedFields(item)
	if _, ok := item["typeLabel"]; ok {
		t.Fatal("typeLabel should not be set for unknown type 999")
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsTypeNotFloat(t *testing.T) {
	// type 不是 float64 → 不添加 typeLabel
	item := map[string]any{"type": "string-type"}
	enrichFeedFields(item)
	if _, ok := item["typeLabel"]; ok {
		t.Fatal("typeLabel should not be set for non-float type")
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsUsersPreserved(t *testing.T) {
	// users 数组中的 user 对象所有字段保留不变
	item := map[string]any{
		"users": []any{
			map[string]any{"nick": "Alice", "avatarUrl": "http://img", "userId": "u1"},
			map[string]any{"nick": "Bob", "email": "bob@test"},
		},
	}
	enrichFeedFields(item)
	users := item["users"].([]any)
	u0 := users[0].(map[string]any)
	if u0["avatarUrl"] != "http://img" {
		t.Fatalf("users[0].avatarUrl should be preserved, got %v", u0["avatarUrl"])
	}
	if u0["userId"] != "u1" {
		t.Fatalf("users[0].userId should be preserved, got %v", u0["userId"])
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsUsersNonMapItem(t *testing.T) {
	// users 数组中有非 map 元素 → 不 panic
	item := map[string]any{
		"users": []any{"not-a-map", 42, nil},
	}
	enrichFeedFields(item) // 不应 panic
	users := item["users"].([]any)
	if len(users) != 3 {
		t.Fatalf("users length changed: %d", len(users))
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsContentPreserved(t *testing.T) {
	// content 和 content.doc 所有字段保留不变
	item := map[string]any{
		"content": map[string]any{
			"doc": map[string]any{
				"name":      "测试文档",
				"extension": "adoc",
				"docKey":    "dk123",
				"url":       "http://example",
			},
			"dentryKey":    "entry1",
			"workspaceKey": "ws1",
		},
	}
	enrichFeedFields(item)
	content := item["content"].(map[string]any)
	if content["dentryKey"] != "entry1" {
		t.Fatalf("content.dentryKey = %v, want entry1", content["dentryKey"])
	}
	if content["workspaceKey"] != "ws1" {
		t.Fatalf("content.workspaceKey = %v, want ws1", content["workspaceKey"])
	}
	doc := content["doc"].(map[string]any)
	if doc["name"] != "测试文档" {
		t.Fatalf("doc.name = %v, want 测试文档", doc["name"])
	}
	if doc["docKey"] != "dk123" {
		t.Fatalf("doc.docKey = %v, want dk123", doc["docKey"])
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsNoContentNoUsers(t *testing.T) {
	// 没有 content / users / type → 所有字段保留
	item := map[string]any{
		"id":        "f1",
		"parentDoc": map[string]any{"x": 1},
	}
	enrichFeedFields(item)
	if _, ok := item["parentDoc"]; !ok {
		t.Fatal("parentDoc should be preserved")
	}
	if item["id"] != "f1" {
		t.Fatal("id should be preserved")
	}
}

func TestCrossPlatformCoverageEnrichFeedFieldsEmptyItem(t *testing.T) {
	// 空 map → 不 panic
	item := map[string]any{}
	enrichFeedFields(item)
	if len(item) != 0 {
		t.Fatalf("empty item changed: %v", item)
	}
}

// ---------- RunE integration: feed list with formatFeedTime ----------

func TestCrossPlatformCoverageWikiFeedListFormatIntegration(t *testing.T) {
	// 通过 scriptedToolCaller 返回真实结构的 feed JSON，验证 RunE 中
	// callMCPToolReturnText → formatFeedTime → PrintJSON 完整路径（仅 --format json）
	caller := &scriptedToolCaller{format: "json"}
	ts := float64(1750067400000)
	feedPayload := map[string]any{
		"feeds": []any{
			map[string]any{
				"time":      ts,
				"type":      float64(0),
				"id":        "feed1",
				"parentDoc": map[string]any{"name": "parent"},
				"userInfo":  map[string]any{"nick": "user1", "avatarUrl": "http://img"},
				"users": []any{
					map[string]any{"nick": "Alice", "userId": "u1", "avatarUrl": "http://avatar"},
				},
				"content": map[string]any{
					"doc": map[string]any{
						"name":      "测试文档",
						"extension": "adoc",
						"docKey":    "dk123",
					},
					"dentryKey":    "entry1",
					"workspaceKey": "ws1",
				},
			},
			map[string]any{
				"time": float64(1700000000000),
				"type": float64(7),
				"id":   "feed2",
			},
		},
	}
	payloadBytes, err := json.Marshal(feedPayload)
	if err != nil {
		t.Fatal(err)
	}
	caller.steps = []scriptedToolStep{{text: string(payloadBytes)}}

	var out bytes.Buffer
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(caller)
	deps.Out.w = &out
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})

	root := newWikiCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(os.Stdin)

	args := []string{"feed", "list", "--workspace", "ws1", "--exclude-file", "--format", "json"}
	root.SetArgs(args)
	os.Args = append([]string{"dws", "wiki"}, args...)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// 验证输出是合法 JSON 且包含格式化后的 feed
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out.String())
	}
	feeds, ok := parsed["feeds"].([]any)
	if !ok || len(feeds) != 2 {
		t.Fatalf("feeds length unexpected: %v", parsed["feeds"])
	}

	// 第一个 feed 验证 time 保留、timeFormatted 格式化、所有原始字段保留
	f0 := feeds[0].(map[string]any)
	// time 保留原始毫秒时间戳
	if f0["time"] != ts {
		t.Fatalf("time = %v, want %v", f0["time"], ts)
	}
	// timeFormatted 被填充
	if _, ok := f0["timeFormatted"].(string); !ok {
		t.Fatalf("timeFormatted should be string, got %T", f0["timeFormatted"])
	}
	// 所有原始字段保留（不删除任何字段）
	if _, ok := f0["parentDoc"]; !ok {
		t.Fatal("parentDoc should be preserved")
	}
	if _, ok := f0["userInfo"]; !ok {
		t.Fatal("userInfo should be preserved")
	}
	if f0["typeLabel"] != "创建文档" {
		t.Fatalf("typeLabel = %v, want 创建文档", f0["typeLabel"])
	}

	// users 保留验证：avatarUrl / userId 均保留
	if users, ok := f0["users"].([]any); ok {
		u0 := users[0].(map[string]any)
		if u0["avatarUrl"] != "http://avatar" {
			t.Fatalf("users[0].avatarUrl should be preserved, got %v", u0["avatarUrl"])
		}
		if u0["userId"] != "u1" {
			t.Fatalf("users[0].userId should be preserved, got %v", u0["userId"])
		}
	}

	// content 结构保留验证
	if content, ok := f0["content"].(map[string]any); ok {
		if content["dentryKey"] != "entry1" {
			t.Fatalf("content.dentryKey should be preserved, got %v", content["dentryKey"])
		}
		if content["workspaceKey"] != "ws1" {
			t.Fatalf("content.workspaceKey should be preserved, got %v", content["workspaceKey"])
		}
		doc := content["doc"].(map[string]any)
		if doc["docKey"] != "dk123" {
			t.Fatalf("content.doc.docKey should be preserved, got %v", doc["docKey"])
		}
	}
}

func TestCrossPlatformCoverageWikiFeedListRawFormat(t *testing.T) {
	// --format raw: 输出原始 MCP 文本，不经过 formatFeedTime 后处理
	rawPayload := `{"feeds":[{"time":1750067400000,"type":1}]}`
	caller := &scriptedToolCaller{
		format: "raw",
		steps:  []scriptedToolStep{{text: rawPayload}},
	}

	var out bytes.Buffer
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(caller)
	deps.Out.w = &out
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})

	root := newWikiCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(os.Stdin)

	args := []string{"feed", "list", "--workspace", "ws1", "--format", "raw"}
	root.SetArgs(args)
	os.Args = append([]string{"dws", "wiki"}, args...)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// raw 输出应包含原始文本，不经过 formatFeedTime 格式化
	got := out.String()
	if !bytes.Contains([]byte(got), []byte(`"feeds"`)) {
		t.Fatalf("raw output should contain original payload, got: %s", got)
	}
}

func TestCrossPlatformCoverageWikiFeedListTableFormatMatchesDispatcher(t *testing.T) {
	// --format table 回归：与共享 dispatcher (callMCPToolInternalOpts) 契约一致，
	// 输出原始紧凑 MCP 文本，不做 pretty 化，也不注入 timeFormatted / typeLabel
	rawPayload := `{"feeds":[{"time":1750067400000,"type":1,"id":"f1"}]}`
	caller := &scriptedToolCaller{
		format: "table",
		steps:  []scriptedToolStep{{text: rawPayload}},
	}

	var out bytes.Buffer
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(caller)
	deps.Out.w = &out
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})

	root := newWikiCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(os.Stdin)

	args := []string{"feed", "list", "--workspace", "ws1", "--format", "table"}
	root.SetArgs(args)
	os.Args = append([]string{"dws", "wiki"}, args...)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// 与共享 dispatcher 一致：输出 = 原始 MCP 文本原文
	if got := strings.TrimSpace(out.String()); got != rawPayload {
		t.Fatalf("table output should equal raw MCP payload,\ngot:  %s\nwant: %s", got, rawPayload)
	}
	// 不注入任何增强字段
	for _, injected := range []string{"timeFormatted", "typeLabel"} {
		if strings.Contains(out.String(), injected) {
			t.Fatalf("table output should not inject %s, got: %s", injected, out.String())
		}
	}
}

func TestCrossPlatformCoverageWikiFeedListEmptyResponse(t *testing.T) {
	// MCP 返回空结果 → formatFeedTime 处理空字符串 → 不报错
	caller := &scriptedToolCaller{} // 无 steps → 空结果

	var out bytes.Buffer
	oldDeps := deps
	oldArgs := os.Args
	InitDeps(caller)
	deps.Out.w = &out
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = oldDeps
		os.Args = oldArgs
	})

	root := newWikiCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetIn(os.Stdin)

	args := []string{"feed", "list", "--workspace", "ws1"}
	root.SetArgs(args)
	os.Args = append([]string{"dws", "wiki"}, args...)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

// ---------- helpers ----------

func buildFeedJSON(t *testing.T, feeds []map[string]any) string {
	t.Helper()
	payload := map[string]any{"feeds": feeds}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

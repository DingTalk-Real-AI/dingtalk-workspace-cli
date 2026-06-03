// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestEnrichReportListContent_InboxStripsContent — inbox callers must never
// surface 日志内容 either in agentDisplayColumns or per-row maps; the privacy
// contract is enforced by the column set and the row builder simultaneously.
func TestEnrichReportListContent_InboxStripsContent(t *testing.T) {
	content := map[string]any{
		"result": map[string]any{
			"list": []any{
				map[string]any{
					"reportId":        "R-1",
					"reportName":      "周报",
					"creatorName":     "张三",
					"createTime":      float64(1747900800000), // 2025-05-22T16:00:00 UTC; locale-dependent display
					"readStatus":      false,
					"content":         "本周完成 enrichment helpers",
					"dingtalkOpenUrl": "https://example.dingtalk.com/r/1",
				},
			},
			"hasMore":    false,
			"nextCursor": float64(0),
		},
	}
	got := EnrichReportListContent(content, false)
	if got["agentDisplayContentIncluded"] != false {
		t.Fatalf("inbox includeContent should be false, got %v", got["agentDisplayContentIncluded"])
	}
	cols, _ := got["agentDisplayColumns"].([]string)
	want := []string{"日期", "标题", "发送人", "状态", "钉钉链接"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("inbox columns mismatch:\n got=%v\nwant=%v", cols, want)
	}
	md, _ := got["agentDisplayMarkdown"].(string)
	if !strings.HasPrefix(md, "| 日期 | 标题 | 发送人 | 状态 | 钉钉链接 |") {
		t.Fatalf("inbox markdown header missing, got: %q", md)
	}
	if strings.Contains(md, "日志内容") {
		t.Fatalf("inbox markdown must NOT contain 日志内容 column, got: %q", md)
	}
	// result list rows must not carry 日志内容 either — the test suite has
	// `assert all("日志内容" not in item for item in data["result"])`.
	wrapper, _ := got["result"].(map[string]any)
	if wrapper == nil {
		t.Fatalf("inbox result should be wrapper map preserving hasMore, got: %T", got["result"])
	}
	list, _ := wrapper["list"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 row in result.list, got %d", len(list))
	}
	row, _ := list[0].(map[string]any)
	if _, present := row["日志内容"]; present {
		t.Fatalf("inbox row must not contain 日志内容 key, got: %v", row)
	}
	if got["count"] != 1 {
		t.Fatalf("count should be 1, got %v", got["count"])
	}
	// hasMore / nextCursor preserved from the original wrapper.
	if wrapper["hasMore"] != false {
		t.Fatalf("hasMore not preserved: %v", wrapper["hasMore"])
	}
}

// TestEnrichReportListContent_OutboxKeepsContent — outbox callers (author ==
// self) get the 日志内容 column and per-row body.
func TestEnrichReportListContent_OutboxKeepsContent(t *testing.T) {
	content := map[string]any{
		"result": map[string]any{
			"list": []any{
				map[string]any{
					"reportId":        "R-9",
					"reportName":      "日报",
					"creatorName":     "李四",
					"createTime":      float64(1747900800000),
					"readStatus":      true,
					"content":         "完成 outbox enrichment 测试",
					"dingtalkOpenUrl": "https://example.dingtalk.com/r/9",
				},
			},
		},
	}
	got := EnrichReportListContent(content, true)
	if got["agentDisplayContentIncluded"] != true {
		t.Fatalf("outbox includeContent should be true")
	}
	cols, _ := got["agentDisplayColumns"].([]string)
	want := []string{"日期", "标题", "发送人", "状态", "日志内容", "钉钉链接"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("outbox columns mismatch:\n got=%v\nwant=%v", cols, want)
	}
	md, _ := got["agentDisplayMarkdown"].(string)
	if !strings.HasPrefix(md, "| 日期 | 标题 | 发送人 | 状态 | 日志内容 | 钉钉链接 |") {
		t.Fatalf("outbox markdown header wrong, got: %q", md)
	}
	if !strings.Contains(md, "[在钉钉中查看日志](") {
		t.Fatalf("outbox markdown must contain REPORT_LINK_MARKER prefix when row has a URL, got: %q", md)
	}
	if !strings.Contains(md, "完成 outbox enrichment 测试") {
		t.Fatalf("outbox markdown must contain the 日志内容 cell text, got: %q", md)
	}
}

// TestEnrichReportListContent_EmptyListStillEmitsSchema — even on a zero-row
// response the agent-display fields must be present so downstream tooling
// has a stable schema. count=0 / markdown is header-only.
func TestEnrichReportListContent_EmptyListStillEmitsSchema(t *testing.T) {
	content := map[string]any{
		"result": map[string]any{
			"list": []any{},
		},
	}
	got := EnrichReportListContent(content, false)
	if got["count"] != 0 {
		t.Fatalf("count should be 0, got %v", got["count"])
	}
	md, _ := got["agentDisplayMarkdown"].(string)
	if !strings.HasPrefix(md, "| 日期 | 标题 | 发送人 | 状态 | 钉钉链接 |") {
		t.Fatalf("empty inbox should still emit header row, got: %q", md)
	}
	// Should not panic / lose schema fields.
	for _, k := range []string{"agentDisplayContentIncluded", "agentDisplayColumns", "agentDisplayMarkdown", "success"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("missing key %q on empty list response", k)
		}
	}
}

// TestEnrichReportListContent_NilOrUnknownPassthrough — nil / non-list maps
// must not be mutated; the agent layer treats absence of agentDisplay* as
// "no enrichment available", which is the safe fallback.
func TestEnrichReportListContent_NilOrUnknownPassthrough(t *testing.T) {
	if got := EnrichReportListContent(nil, true); got != nil {
		t.Fatalf("nil input should return nil")
	}
	// No list-shaped key — we still attach the agent display schema so
	// downstream callers always see a uniform shape, but the rows are empty.
	src := map[string]any{"unrelated": "value"}
	got := EnrichReportListContent(src, false)
	if got["unrelated"] != "value" {
		t.Fatalf("unrelated keys must be preserved")
	}
	if got["count"] != 0 {
		t.Fatalf("missing list -> count should be 0")
	}
	// The input map must not be mutated in place.
	if _, present := src["agentDisplayColumns"]; present {
		t.Fatalf("EnrichReportListContent mutated the input map")
	}
}

// TestEnrichCapturedReportListJSON_PassthroughForDryRun — dry-run output
// (the {invocation, response} envelope) must round-trip verbatim through
// the wrapper; enrichment only applies to unwrapped MCP content responses.
func TestEnrichCapturedReportListJSON_PassthroughForDryRun(t *testing.T) {
	raw := []byte(`{
		"invocation": {"kind":"compat_invocation"},
		"response":   {"dry_run": true}
	}`)
	if _, ok := enrichCapturedReportListJSON(raw, false); ok {
		t.Fatalf("dry-run envelope must fall through to passthrough")
	}
}

// TestEnrichCapturedReportListJSON_HandlesContentShape — when output.
// WriteCommandPayload unwraps the Result and emits the raw `content` map,
// the wrapper must detect that shape and enrich it.
func TestEnrichCapturedReportListJSON_HandlesContentShape(t *testing.T) {
	raw := []byte(`{
		"success": true,
		"result": {
			"list": [
				{
					"reportId": "X-1",
					"reportName": "周报",
					"creatorName": "王五",
					"createTime": 1747900800000,
					"readStatus": true,
					"dingtalkOpenUrl": "https://example.dingtalk.com/r/x1"
				}
			]
		}
	}`)
	got, ok := enrichCapturedReportListJSON(raw, true)
	if !ok {
		t.Fatalf("content shape should be recognised")
	}
	root, _ := got.(map[string]any)
	if root["agentDisplayContentIncluded"] != true {
		t.Fatalf("outbox enrichment should mark contentIncluded=true")
	}
	cols, _ := root["agentDisplayColumns"].([]string)
	if len(cols) != 6 {
		t.Fatalf("outbox columns should have 6 entries, got %d (%v)", len(cols), cols)
	}
}

// TestEnrichCapturedReportListJSON_MalformedJSON — invalid JSON must not
// panic and must fall through so the caller writes the raw bytes back.
func TestEnrichCapturedReportListJSON_MalformedJSON(t *testing.T) {
	if _, ok := enrichCapturedReportListJSON([]byte(`not json`), false); ok {
		t.Fatalf("malformed JSON must fall through to passthrough")
	}
	if _, ok := enrichCapturedReportListJSON([]byte(``), false); ok {
		t.Fatalf("empty buffer must fall through to passthrough")
	}
}

// TestEnrichReportListContent_MarkdownEscapesPipes — embedded pipes in user
// content must be replaced with full-width "｜" so the markdown table is
// not broken at the renderer.
func TestEnrichReportListContent_MarkdownEscapesPipes(t *testing.T) {
	content := map[string]any{
		"result": map[string]any{
			"list": []any{
				map[string]any{
					"reportId":    "R-pipe",
					"reportName":  "标题|含管道",
					"creatorName": "张|三",
					"createTime":  float64(1747900800000),
					"readStatus":  false,
				},
			},
		},
	}
	got := EnrichReportListContent(content, false)
	md, _ := got["agentDisplayMarkdown"].(string)
	if strings.Contains(strings.Split(md, "\n")[2], "|含管道") {
		// The first data row is line index 2 (header, divider, data).
		t.Fatalf("raw '|' must be replaced with full-width '｜' in cell, got: %q", md)
	}
	if !strings.Contains(md, "标题｜含管道") {
		t.Fatalf("expected escaped full-width pipe in title cell, got: %q", md)
	}
}

// TestEnrichReportListContent_JSONNumberTimestamps — json.Number variants of
// the timestamp keys must still produce a formatted 日期 cell.
func TestEnrichReportListContent_JSONNumberTimestamps(t *testing.T) {
	decoded := map[string]any{}
	dec := json.NewDecoder(strings.NewReader(`{
		"result": {
			"list": [{
				"reportId": "N-1",
				"reportName": "N报",
				"createTime": 1747900800000
			}]
		}
	}`))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := EnrichReportListContent(decoded, false)
	wrapper, _ := got["result"].(map[string]any)
	list, _ := wrapper["list"].([]any)
	row, _ := list[0].(map[string]any)
	date, _ := row["日期"].(string)
	if date == "" {
		t.Fatalf("json.Number timestamp should still yield a formatted 日期, got empty")
	}
}

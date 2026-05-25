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

// Package helpers — readable-output enrichment for `report inbox list` and
// `report outbox list`.
//
// What this file adds:
//
//   - EnrichReportListContent: pure function that walks an MCP `content`
//     response, finds the report items list, and overlays five extra fields
//     the agent layer relies on:
//     success
//     count
//     agentDisplayContentIncluded   (bool — inbox false, outbox true)
//     agentDisplayColumns           ([]string — wukong-aligned column set)
//     agentDisplayMarkdown          (string — markdown table)
//     The function never mutates the input map in-place; it returns a fresh
//     map so callers (including tests) can compare before/after safely.
//
//   - AttachReportListReadableEnrichment: post-merge hook that finds the
//     dynamic-built `report inbox list` / `report outbox list` leaves in the
//     merged command tree and wraps their RunE to apply EnrichReportListContent
//     to the JSON payload before it is written to stdout. Behaviour for other
//     formats (--format raw / table / csv etc.) is preserved verbatim —
//     enrichment only fires when the output is JSON.
//
// Why a post-merge hook (mirroring AttachReportLegacyInboxAlias):
//
//	the envelope already publishes `report inbox` and `report outbox` as
//	groups with a `list` leaf each; the open-source CLI cannot add a same-
//	named helper leaf (MergeHardcodedLeaves would reject it as a shape
//	mismatch with the envelope). Wrapping the existing leaf's RunE keeps the
//	envelope as the single source of truth for flags/schema while letting us
//	layer wukong-equivalent enrichment on top.
package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// ── public constants (mirrors wukong/products/report.go) ────────────────

// reportInboxColumns / reportOutboxColumns must stay byte-identical to the
// REPORT_*_COLUMNS constants in dws-wukong/auto-test/cli_to_mcp/testcases/
// report/test_90_report_param_regression.py — drift breaks the param-
// regression assertions. The only difference between the two is the "日志内容"
// column, which inbox elides (privacy: never echo inbox bodies back as
// agent-visible content) and outbox keeps (the user is the author).
var (
	reportInboxColumns  = []string{"日期", "标题", "发送人", "状态", "钉钉链接"}
	reportOutboxColumns = []string{"日期", "标题", "发送人", "状态", "日志内容", "钉钉链接"}
)

// reportDingtalkLinkText is the visible label used inside the markdown link
// in the "钉钉链接" column. The wukong upstream also uses this literal; tests
// (REPORT_LINK_MARKER) match on the prefix "[在钉钉中查看日志](" so any change
// here must be mirrored in test_90_report_param_regression.py.
const reportDingtalkLinkText = "在钉钉中查看日志"

// ── EnrichReportListContent: pure transform ─────────────────────────────

// EnrichReportListContent overlays wukong-style agent-display fields on the
// MCP `content` map of a report list response. It is a pure function:
//   - input map is never mutated;
//   - on any structural mismatch (nil, no list, etc.) it returns the input
//     untouched so the caller's behaviour is byte-stable.
//
// includeContent controls whether the "日志内容" column appears in
// agentDisplayColumns / agentDisplayMarkdown and whether each result row
// retains the 日志内容 key. Pass true for outbox (sender == self, content
// safe to echo), false for inbox.
func EnrichReportListContent(content map[string]any, includeContent bool) map[string]any {
	if content == nil {
		return content
	}
	items, found := findReportItemsForEnrichment(content)
	if !found {
		// Still attach display fields so the agent layer has a stable
		// schema to read from even on empty / malformed list responses;
		// rows will be empty and markdown table will only carry headers.
		items = nil
	}
	rows := make([]map[string]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, reportRowForItem(item, includeContent))
	}
	sortReportRowsByDate(rows)

	columns := reportInboxColumns
	if includeContent {
		columns = reportOutboxColumns
	}
	markdown := reportRenderMarkdownTable(columns, rows)

	// Build a shallow copy so callers never see in-place mutation. We
	// preserve every key the upstream MCP server returned (notably
	// hasMore / nextCursor / cursor at the result-wrapper level) by
	// copying them into the new map verbatim.
	out := make(map[string]any, len(content)+5)
	for k, v := range content {
		out[k] = v
	}
	// Replace `result` with a typed list of rows so the agent can iterate
	// over a stable shape. The original `result` body is kept under the
	// rebuilt key so paginator fields like nextCursor still ride at the
	// expected level when MCP nested them there.
	rebuiltResult := buildReportResultPayload(content, rows)
	if rebuiltResult != nil {
		out["result"] = rebuiltResult
	} else {
		// No nested wrapper to preserve — emit rows directly under result
		// so callers can read result[] uniformly.
		out["result"] = rowsAsAnySlice(rows)
	}

	out["count"] = len(rows)
	if _, present := out["success"]; !present {
		out["success"] = true
	}
	out["agentDisplayContentIncluded"] = includeContent
	out["agentDisplayColumns"] = columns
	out["agentDisplayMarkdown"] = markdown
	return out
}

// findReportItemsForEnrichment locates the list of report items inside an
// MCP `content` map. The wukong upstream uses a permissive search across the
// well-known keys ("list", "items", "data", "reports", "records",
// "report_list", "reportList", "result"); we mirror that so envelope
// renames don't silently drop enrichment.
func findReportItemsForEnrichment(root map[string]any) ([]map[string]any, bool) {
	for _, key := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList", "result"} {
		value, ok := root[key]
		if !ok {
			continue
		}
		if items, found := reportItemsFromValueForEnrichment(value); found {
			return items, true
		}
	}
	return nil, false
}

func reportItemsFromValueForEnrichment(v any) ([]map[string]any, bool) {
	switch x := v.(type) {
	case []any:
		items := make([]map[string]any, 0, len(x))
		for _, raw := range x {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			items = append(items, item)
		}
		return items, true
	case map[string]any:
		// Recurse into nested wrappers (e.g. {result: {list: [...]}}).
		for _, key := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList"} {
			value, ok := x[key]
			if !ok {
				continue
			}
			if items, found := reportItemsFromValueForEnrichment(value); found {
				return items, true
			}
		}
	}
	return nil, false
}

// buildReportResultPayload rebuilds the `result` field so that, when the
// upstream MCP wrapped items under `result.list`, the rebuilt payload keeps
// the wrapper (preserving sibling fields like nextCursor / hasMore) while
// swapping the list contents for the enriched rows. When no wrapper exists,
// returns nil and the caller emits rows directly.
func buildReportResultPayload(content map[string]any, rows []map[string]string) any {
	wrapper, ok := content["result"].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(wrapper))
	for k, v := range wrapper {
		out[k] = v
	}
	for _, key := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList"} {
		if _, found := wrapper[key]; found {
			out[key] = rowsAsAnySlice(rows)
			return out
		}
	}
	// Wrapper had no recognised list key — the list must live somewhere
	// else; we still publish rows at out["list"] so agents have a uniform
	// place to read.
	out["list"] = rowsAsAnySlice(rows)
	return out
}

func rowsAsAnySlice(rows []map[string]string) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		copyRow := make(map[string]any, len(row))
		for k, v := range row {
			copyRow[k] = v
		}
		out = append(out, copyRow)
	}
	return out
}

// reportRowForItem extracts the four base columns (date / title / sender /
// status) plus the dingtalk markdown link from a single item map. The
// "日志内容" column is injected only when includeContent=true; inbox callers
// pass false to enforce the privacy contract (no inbox bodies leaked back).
func reportRowForItem(item map[string]any, includeContent bool) map[string]string {
	row := map[string]string{
		"日期":   reportItemDate(item),
		"标题":   reportItemTitle(item),
		"发送人":  reportItemSender(item),
		"状态":   reportItemStatus(item),
		"钉钉链接": reportItemMarkdownLink(item),
	}
	if includeContent {
		row["日志内容"] = reportItemContent(item)
	}
	return row
}

func reportItemDate(item map[string]any) string {
	for _, key := range []string{"createTime", "gmtCreate", "sendTime", "time", "modifiedTime", "日期"} {
		if ms := reportMillisFromValue(item[key]); ms > 0 {
			return time.UnixMilli(ms).Local().Format("2006-01-02 15:04")
		}
		if s := reportStringFromValue(item[key]); s != "" {
			return s
		}
	}
	return ""
}

func reportItemTitle(item map[string]any) string {
	for _, key := range []string{"report_name", "reportName", "title", "summary", "report_template_name", "templateName", "标题"} {
		if s := reportStringFromValue(item[key]); s != "" {
			return s
		}
	}
	if sender := reportItemSender(item); sender != "" {
		return sender + "的日志"
	}
	return "日志"
}

func reportItemSender(item map[string]any) string {
	for _, key := range []string{"creatorName", "senderName", "userName", "creator", "sender", "发送人"} {
		if s := reportStringFromValue(item[key]); s != "" {
			return s
		}
	}
	return ""
}

func reportItemStatus(item map[string]any) string {
	for _, key := range []string{"readStatus", "isRead", "hasRead", "read", "状态"} {
		v, ok := item[key]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case bool:
			if x {
				return "已读"
			}
			return "未读"
		case string:
			lower := strings.TrimSpace(strings.ToLower(x))
			switch lower {
			case "true", "read", "1", "已读":
				return "已读"
			case "false", "unread", "0", "未读":
				return "未读"
			default:
				return strings.TrimSpace(x)
			}
		case float64:
			if x == 1 {
				return "已读"
			}
			if x == 0 {
				return "未读"
			}
		}
	}
	return ""
}

// reportItemMarkdownLink prefers an upstream-supplied markdown link
// ("dingtalkOpenMarkdownLink"); otherwise it composes one from any URL field
// it can find on the item. As a last resort it emits the plain label "查看
// 详情" so the column is never empty (tests assert the column header always
// exists and the marker shows when count>0; an empty cell would still
// satisfy markdown but would degrade the user-facing render).
func reportItemMarkdownLink(item map[string]any) string {
	if s := reportStringFromValue(item["dingtalkOpenMarkdownLink"]); s != "" {
		return s
	}
	if s := reportStringFromValue(item["钉钉链接"]); s != "" {
		return s
	}
	for _, key := range []string{"url", "dingtalkOpenUrl", "openUrl", "webUrl"} {
		if s := reportStringFromValue(item[key]); s != "" {
			return fmt.Sprintf("[%s](%s)", reportDingtalkLinkText, s)
		}
	}
	if link, ok := item["dingtalkOpenLink"].(map[string]any); ok {
		if s := reportStringFromValue(link["url"]); s != "" {
			return fmt.Sprintf("[%s](%s)", reportDingtalkLinkText, s)
		}
	}
	return "查看详情"
}

func reportItemContent(item map[string]any) string {
	for _, key := range []string{"report_content", "reportContent", "content", "summary", "日志内容"} {
		v, ok := item[key]
		if !ok {
			continue
		}
		if s := reportFlattenContent(v); s != "" {
			return reportCompactCell(s, 600)
		}
	}
	return ""
}

func reportFlattenContent(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := reportFlattenContent(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "；")
	case map[string]any:
		label := reportFirstString(x, "key", "field_name", "fieldName", "name", "title")
		value := reportFirstString(x, "value", "content", "text", "plainText", "markdown")
		if value == "" {
			return ""
		}
		if label != "" {
			return label + "：" + value
		}
		return value
	}
	return ""
}

func reportFirstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := reportStringFromValue(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func reportStringFromValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return ""
}

// reportMillisFromValue normalises numeric / numeric-string timestamps to
// milliseconds. Second-precision values (< 1e11) are scaled up. Non-numeric
// inputs return 0 so the caller can fall back to a string formatter.
func reportMillisFromValue(v any) int64 {
	switch x := v.(type) {
	case float64:
		return normaliseReportTimestamp(int64(x))
	case int64:
		return normaliseReportTimestamp(x)
	case int:
		return normaliseReportTimestamp(int64(x))
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0
		}
		return normaliseReportTimestamp(n)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return normaliseReportTimestamp(n)
		}
	}
	return 0
}

func normaliseReportTimestamp(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	if ts < 100000000000 {
		ts *= 1000
	}
	return ts
}

func reportCompactCell(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "|", "｜")
	s = strings.Join(strings.Fields(s), " ")
	if maxRunes <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

// sortReportRowsByDate sorts rows by date string descending so the most
// recent log appears first — matches the wukong upstream's user-visible
// ordering. String compare is safe because dates are formatted as
// "2006-01-02 15:04" (lexicographic order == chronological order).
func sortReportRowsByDate(rows []map[string]string) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i]["日期"] > rows[j]["日期"]
	})
}

// reportRenderMarkdownTable composes a standard GFM markdown table from the
// given column header and row maps. Cell values are sanitised so that
// embedded pipes do not break the table; long whitespace runs are squashed.
func reportRenderMarkdownTable(columns []string, rows []map[string]string) string {
	var b strings.Builder
	b.WriteString("| ")
	b.WriteString(strings.Join(columns, " | "))
	b.WriteString(" |")
	b.WriteString("\n")
	dividers := make([]string, len(columns))
	for i := range dividers {
		dividers[i] = "---"
	}
	b.WriteString("| ")
	b.WriteString(strings.Join(dividers, " | "))
	b.WriteString(" |")
	for _, row := range rows {
		b.WriteString("\n")
		cells := make([]string, 0, len(columns))
		for _, col := range columns {
			cells = append(cells, reportCellForMarkdown(row[col]))
		}
		b.WriteString("| ")
		b.WriteString(strings.Join(cells, " | "))
		b.WriteString(" |")
	}
	return b.String()
}

func reportCellForMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", "｜")
}

// ── AttachReportListReadableEnrichment: post-merge hook ─────────────────

// AttachReportListReadableEnrichment wires EnrichReportListContent into the
// envelope-built `report inbox list` and `report outbox list` leaves.
//
// Mechanism (decorator over the leaf's existing RunE):
//  1. Replace leaf.RunE with a closure that delegates to the original RunE.
//  2. Before delegating, redirect cmd.SetOut() to an in-memory buffer when
//     the resolved output format is JSON (the only format the agent display
//     schema cares about).
//  3. After the original RunE returns, unmarshal the captured bytes, locate
//     the MCP `content` payload, call EnrichReportListContent, and write
//     the enriched JSON to the leaf's original stdout.
//  4. For non-JSON formats (raw / table / csv / ...) we never replace stdout,
//     so behaviour stays byte-for-byte identical to the unwrapped envelope.
//
// `runner` is accepted for API symmetry with AttachReportLegacyInboxAlias —
// the wrapper itself does not invoke runner; the wrapped RunE already does.
// Keeping the parameter avoids a churn on app/legacy.go if we ever need to
// dispatch a sibling tool from inside the wrapper.
func AttachReportListReadableEnrichment(commands []*cobra.Command, runner executor.Runner) {
	_ = runner // reserved — see comment above
	for _, top := range commands {
		if top == nil || top.Name() != "report" {
			continue
		}
		wrapReportListLeaf(top, "inbox", false)
		wrapReportListLeaf(top, "outbox", true)
	}
}

func wrapReportListLeaf(reportCmd *cobra.Command, groupName string, includeContent bool) {
	var group *cobra.Command
	for _, child := range reportCmd.Commands() {
		if child != nil && child.Name() == groupName {
			group = child
			break
		}
	}
	if group == nil {
		return
	}
	var leaf *cobra.Command
	for _, child := range group.Commands() {
		if child != nil && child.Name() == "list" {
			leaf = child
			break
		}
	}
	if leaf == nil {
		return
	}
	originalRunE := leaf.RunE
	if originalRunE == nil {
		return
	}
	leaf.RunE = func(cmd *cobra.Command, args []string) error {
		// Resolve the format the user asked for. Enrichment only applies
		// when the eventual writer would emit JSON — other formats stay
		// untouched so `--format table`, `--format raw`, etc. remain
		// byte-stable against the envelope.
		fmtChoice := output.ResolveFormat(cmd, output.FormatJSON)
		if fmtChoice != output.FormatJSON {
			return originalRunE(cmd, args)
		}

		originalStdout := cmd.OutOrStdout()
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		runErr := originalRunE(cmd, args)
		// Restore the leaf's original writer regardless of whether the
		// run succeeded — if it failed we still want any partial bytes
		// (rare, defensive) flushed back to the caller.
		cmd.SetOut(originalStdout)
		if runErr != nil {
			// Pass through any captured bytes the inner RunE produced
			// before failing so the caller's logs / stderr show the
			// same diagnostic context as the un-wrapped envelope.
			if buf.Len() > 0 {
				_, _ = originalStdout.Write(buf.Bytes())
			}
			return runErr
		}
		enriched, ok := enrichCapturedReportListJSON(buf.Bytes(), includeContent)
		if !ok {
			// Captured payload did not match the expected shape (e.g.
			// dry-run output, non-content envelope, malformed JSON) —
			// echo the original bytes verbatim so the caller's behaviour
			// is unchanged for those code paths.
			_, _ = originalStdout.Write(buf.Bytes())
			return nil
		}
		return output.WriteJSON(originalStdout, enriched)
	}
}

// enrichCapturedReportListJSON parses bytes the wrapped RunE wrote to its
// captured stdout buffer and, if the payload is the unwrapped MCP `content`
// map (the shape produced by output.WriteCommandPayload for successful
// compat_invocation Results), applies EnrichReportListContent and returns
// the enriched payload. Returns ok=false for any shape the function does
// not recognise so the caller falls back to passthrough.
func enrichCapturedReportListJSON(raw []byte, includeContent bool) (any, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return nil, false
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return nil, false
	}
	// Case A: payload is the MCP `content` map directly (the post-unwrap
	// shape that output.WriteCommandPayload emits for successful
	// compat_invocation results in --format json). Enrich in place.
	if _, hasResult := root["result"]; hasResult || hasReportListKey(root) {
		return EnrichReportListContent(root, includeContent), true
	}
	// Case B: dry-run / error envelopes carry {invocation, response}. We
	// leave those untouched — the agent-display schema only applies to
	// successful list responses; dry-run is for inspection.
	return nil, false
}

// hasReportListKey reports whether the map looks like an MCP list response
// even if `result` is absent (some servers return `list` at the top level).
func hasReportListKey(m map[string]any) bool {
	for _, k := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

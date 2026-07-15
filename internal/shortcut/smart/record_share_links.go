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

package smart

import (
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// RecordShareLinks: get share links for MANY aitable (多维表) records in one
// command, transparently working around the tool's per-call cap.
//
// The atomic tool get_record_share_url accepts a recordIds array but caps a
// single call at 20 (see the +record-share-url 1:1 shortcut and
// internal/helpers/aitable.go). To share more than 20 records a user has to
// split the list by hand and stitch the results. This shortcut dedups the
// requested recordIds (preserving order), chunks them into batches of ≤20, fans
// each batch out to get_record_share_url and merges every returned
// {recordId, shareUrl} into one projected list — a failing batch is recorded and
// does not abort the rest.
//
// Cross-server note: get_record_share_url runs on the "aitable-helper" MCP
// server (not "aitable"), so the calls go through CallMCPData with an explicit
// product, mirroring helpers.callAitableHelperTool.
//
//	dws aitable +record-share-links --base B --table T --record-ids rec1,rec2,…,rec50
//	dws aitable +record-share-links --base B --table T --record-ids rec1 --view-id viw_VIP
var RecordShareLinks = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+record-share-links",
	Product:     "aitable",
	Description: "批量（可 >20 条）获取多维表记录分享链接：去重+分片+合并",
	Intent: "当你要一次性拿到很多条多维表记录的分享链接、数量可能超过底层工具单次 20 条上限时使用；" +
		"内部先对 --record-ids 去重（保持顺序），再按每批 ≤20 条切片，逐批调用 get_record_share_url（在 aitable-helper 服务上），" +
		"最后把各批返回的 {recordId, shareUrl} 合并成一个列表；某一批失败会记录错误但不影响其余批。" +
		"这是只读操作，只生成/获取分享链接、不修改记录。可选 --view-id 生成带视图上下文的链接。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "base", Type: shortcut.FlagString, Desc: "Base ID（记录所属 base）", Required: true},
		{Name: "table", Type: shortcut.FlagString, Desc: "Table ID（记录所属数据表）", Required: true},
		{Name: "record-ids", Type: shortcut.FlagStringSlice, Desc: "记录 ID 列表，可 >20（自动去重+分片，必填）", Required: true},
		{Name: "view-id", Type: shortcut.FlagString, Desc: "视图 ID：生成带视图上下文的链接（可选）", Required: false},
	},
	Tips: []string{
		`dws aitable +record-share-links --base B --table T --record-ids rec1,rec2,rec3`,
		`dws aitable +record-share-links --base B --table T --record-ids rec1 --view-id viw_VIP`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Dedup while preserving first-seen order.
		ids := dedupStrings(rt.StrSlice("record-ids"))
		if len(ids) == 0 {
			return apperrors.NewValidation("--record-ids 去重后为空")
		}

		baseID := rt.Str("base")
		tableID := rt.Str("table")
		viewID := rt.Str("view-id")

		items := make([]map[string]any, 0, len(ids))
		var batchErrors []map[string]any
		for start := 0; start < len(ids); start += recordShareBatchSize {
			end := start + recordShareBatchSize
			if end > len(ids) {
				end = len(ids)
			}
			chunk := ids[start:end]
			params := map[string]any{
				"baseId":    baseID,
				"tableId":   tableID,
				"recordIds": chunk,
			}
			if viewID != "" {
				params["viewId"] = viewID
			}
			// get_record_share_url lives on the aitable-helper server.
			data, err := rt.CallMCPData("aitable-helper", "get_record_share_url", params)
			if err != nil {
				batchErrors = append(batchErrors, map[string]any{
					"recordIds": chunk,
					"error":     err.Error(),
				})
				continue
			}
			items = append(items, recordShareItems(data)...)
		}

		out := map[string]any{
			"base":    baseID,
			"table":   tableID,
			"total":   len(ids),
			"batches": (len(ids) + recordShareBatchSize - 1) / recordShareBatchSize,
			"items":   items,
		}
		if len(batchErrors) > 0 {
			out["errors"] = batchErrors
		}
		return rt.Output(out)
	},
}

// recordShareBatchSize is the per-call cap enforced by get_record_share_url.
const recordShareBatchSize = 20

// dedupStrings removes duplicate and blank entries, preserving first-seen order.
func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// recordShareItems defensively unwraps the {recordId, shareUrl} list from a
// get_record_share_url response. Per helpers the payload is data.items, but the
// gateway may also flatten it to a top-level items — probe both.
func recordShareItems(data map[string]any) []map[string]any {
	containers := []any{data["items"]}
	if d, ok := data["data"].(map[string]any); ok {
		containers = append(containers, d["items"])
	}
	for _, c := range containers {
		list, ok := c.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(list))
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				out = append(out, map[string]any{
					"recordId": firstNonEmpty(m, "recordId", "record_id", "id"),
					"shareUrl": firstAny(m, "shareUrl", "share_url", "url"),
				})
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// firstNonEmpty returns the first non-empty string value among the candidate keys.
func firstNonEmpty(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// firstAny returns the first present (possibly null) value among the candidate
// keys — shareUrl is intentionally kept even when null to signal a failed row.
func firstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func init() {
	shortcut.Register(RecordShareLinks)
}

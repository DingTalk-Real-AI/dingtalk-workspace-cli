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
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// MyInitiated: list the OA approval instances *I* have initiated (submitted), in
// one step.
//
// Steps:
//
//  1. call get_submitted_instances on the OA server — the exact tool + parameter
//     names (pageNumber / pageSize as float64, optional query) used by
//     helpers.approvalSubmittedListCmd ("dws oa approval list-submitted"). page
//     and limit default to the same first-page values (page 1, limit 20).
//
//  2. defensively locate the instance list inside the response (probing common
//     container keys, incl. a nested result/data object) and project each item
//     down to {title, businessId, status, processInstanceId} with multiple
//     candidate keys per field. When no recognisable list is found we print the
//     raw payload so nothing is silently lost.
//
//  3. print via rt.Output so it honours --format / --jq / --fields.
//
// Read-only: it only lists and reshapes my submitted approvals, it never
// approves, rejects, revokes or mutates anything.
//
//	dws oa +my-initiated
//	dws oa +my-initiated --query 报销
//	dws oa +my-initiated --page 2 --limit 50
var MyInitiated = shortcut.Shortcut{
	Service:     "oa",
	Command:     "+my-initiated",
	Product:     "oa",
	Description: "列出我发起（提交）的审批单据",
	Intent: "当你想快速看清自己发起（提交）过哪些 OA 审批单据、方便跟进它们的进展时使用；" +
		"内部直接拉取当前用户已发起的审批实例列表（等价于 dws oa approval list-submitted），" +
		"再在本地把每条单据投影成标题、单号(businessId)、状态和审批实例 ID(processInstanceId) 四个关键字段。" +
		"可用 --query 按关键字过滤、--page/--limit 翻页（默认第 1 页、每页 20 条）。" +
		"这是纯只读操作，只做列表与本地投影，不会同意、拒绝、撤销或修改任何审批单据；若没有发起过审批则返回空列表。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "关键字搜索（可选）", Required: false},
		{Name: "page", Type: shortcut.FlagInt, Desc: "分页页码（可选，默认 1）", Default: "1", Required: false},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页大小（可选，默认 20）", Default: "20", Required: false},
	},
	Tips: []string{
		`dws oa +my-initiated`,
		`dws oa +my-initiated --query 报销`,
		`dws oa +my-initiated --page 2 --limit 50`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — list my submitted approval instances. pageNumber/pageSize are
		// float64 and query is optional, mirroring helpers.approvalSubmittedListCmd.
		page := rt.Int("page")
		if page <= 0 {
			page = 1
		}
		limit := rt.Int("limit")
		if limit <= 0 {
			limit = 20
		}
		params := map[string]any{
			"pageNumber": float64(page),
			"pageSize":   float64(limit),
		}
		if q := strings.TrimSpace(rt.Str("query")); q != "" {
			params["query"] = q
		}

		data, err := rt.CallMCPData("oa", "get_submitted_instances", params)
		if err != nil {
			return err
		}

		// Step 2 — project the instance list; fall back to the raw payload when we
		// cannot locate a recognisable list.
		items := myInitiatedItems(data)
		if len(items) == 0 {
			return rt.Output(data)
		}
		results := make([]map[string]any, 0, len(items))
		for _, m := range items {
			results = append(results, map[string]any{
				"title":             myInitiatedTitle(m),
				"businessId":        myInitiatedBusinessID(m),
				"status":            myInitiatedStatus(m),
				"processInstanceId": myInitiatedInstanceID(m),
			})
		}

		// Step 3 — print the projected list.
		return rt.Output(map[string]any{"initiated": results})
	},
}

// myInitiatedItems locates the instance list inside a get_submitted_instances
// response, probing common container keys at the top level and nested under a
// result/data object. Returns nil when no list is found.
func myInitiatedItems(data map[string]any) []map[string]any {
	if data == nil {
		return nil
	}
	keys := []string{"list", "instances", "processInstances", "items", "data", "records", "result", "rows"}
	for _, key := range keys {
		if arr, ok := data[key].([]any); ok {
			return myInitiatedToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "instances", "processInstances", "items", "data", "records", "rows"} {
				if arr, ok := inner[k2].([]any); ok {
					return myInitiatedToMaps(arr)
				}
			}
		}
	}
	return nil
}

func myInitiatedToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// myInitiatedTitle reads an instance's human-readable title, tolerating the
// common title keys the gateway may use.
func myInitiatedTitle(m map[string]any) any {
	for _, key := range []string{"title", "subject", "name", "processName", "formName", "instanceTitle"} {
		if v := myInitiatedString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// myInitiatedBusinessID reads an instance's business / order number, preferring
// the customer-visible business id and falling back to related identifiers.
func myInitiatedBusinessID(m map[string]any) any {
	for _, key := range []string{"businessId", "bizId", "orderNo", "orderNumber", "number", "serialNumber"} {
		if v := myInitiatedString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// myInitiatedStatus reads an instance's current status/result, tolerating both
// the running status and the final result keys.
func myInitiatedStatus(m map[string]any) any {
	for _, key := range []string{"status", "statusText", "processStatus", "instanceStatus", "result", "approvalResult"} {
		if v := myInitiatedString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// myInitiatedInstanceID reads an instance's processInstanceId, tolerating the
// common id keys.
func myInitiatedInstanceID(m map[string]any) any {
	for _, key := range []string{"processInstanceId", "instanceId", "processInstanceID", "id", "bizId"} {
		if v := myInitiatedString(m[key]); v != "" {
			return v
		}
	}
	return nil
}

// myInitiatedString coerces a scalar JSON value to a trimmed string, returning
// "" for nil / non-scalar / empty values.
func myInitiatedString(v any) string {
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		// approval ids/numbers may arrive as JSON numbers; render without a
		// trailing decimal point when integral.
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func init() {
	shortcut.Register(MyInitiated)
}

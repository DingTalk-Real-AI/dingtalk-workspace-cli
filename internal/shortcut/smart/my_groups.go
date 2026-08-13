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
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

const (
	myGroupsDefaultPageLimit = 50
	myGroupsHardPageLimit    = 500
)

// MyGroups: list the groups I've joined and project just the key fields
// (会话id / 名称 / 群主 / 人数 / 类型) into a clean, composed payload — instead of
// paging through `chat group list-all` and squinting at the raw MCP response.
//
// Steps:
//  1. list my groups via list_my_groups_pagination (im server); param `limit`
//     is copied verbatim from chat.go's `chat group list-all` call site.
//  2. defensively project each group's key fields (field names probed across
//     several candidate keys, since the gateway shape isn't guaranteed);
//  3. optionally keep only groups whose type matches --type (Go-side filter —
//     the underlying tool has no server-side type parameter).
//
// Read-only: it never modifies any group or membership.
//
//	dws chat +my-groups
//	dws chat +my-groups --type group
var MyGroups = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+my-groups",
	Product:     "chat",
	Description: "列出我加入的群，可按类型过滤并投影关键字段",
	Intent: "当你想快速看一眼自己都加入了哪些群、以及每个群的会话ID、名称、群主和人数，而不想翻分页或盯着原始返回时使用；" +
		"内部分页拉取你加入的群列表，把每个群防御式地投影成 会话id / 名称 / 群主 / 人数 / 类型 等关键字段，输出成干净的结果。" +
		"可选 --type 在本地按群类型过滤（底层接口本身不带类型参数，故为客户端过滤）。这是只读操作，不会改动任何群或成员关系。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_my_groups",
			CanonicalPath:  "chat.shortcut_my_groups",
			CLIPath:        "chat +my-groups",
			PrimaryCLIPath: "chat +my-groups",
		},
		Description: "列出我加入的群，可按类型过滤并投影关键字段",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出我加入的群，可按类型过滤并投影关键字段",
			UseWhen:      []string{"当你想快速看一眼自己都加入了哪些群、以及每个群的会话ID、名称、群主和人数，而不想翻分页或盯着原始返回时使用；内部分页拉取你加入的群列表，把每个群防御式地投影成 会话id / 名称 / 群主 / 人数 / 类型 等关键字段，输出成干净的结果。可选 --type 在本地按群类型过滤（底层接口本身不带类型参数，故为客户端过滤）。这是只读操作，不会改动任何群或成员关系。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples: []string{
				"dws chat +my-groups",
				"dws chat +my-groups --type group",
			},
		},
	},
	Flags: append([]shortcut.Flag{
		{Name: "type", Type: shortcut.FlagString, Desc: "按群类型过滤（可选，如返回中的 groupType/conversationType，大小写不敏感）", Required: false},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "每页返回数量（默认 200）；--limit 必须在 1-200 之间", Default: "200"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传上次的 nextCursor"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "沿 nextCursor 自动读取全部已加入群；--page-limit 仅与 --page-all 一起使用且范围 1-500；--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlFlags()...),
	Constraints: append([]shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-200 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlConstraints()...),
	Tips: []string{
		`dws chat +my-groups`,
		`dws chat +my-groups --type group`,
		`dws chat +my-groups --page-all --page-limit 50`,
	},
	Validate: validateMyGroups,
	Execute:  executeMyGroups,
}

func validateMyGroups(rt *shortcut.RuntimeContext) error {
	if limit := rt.Int("limit"); limit < 1 || limit > 200 {
		return apperrors.NewValidation("--limit 必须在 1-200 之间")
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > myGroupsHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	if err := shortcut.ValidateAutoPageControls(rt); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	return nil
}

func executeMyGroups(rt *shortcut.RuntimeContext) error {
	// Step 1 — list my groups. `limit` mirrors chat.go's list_my_groups_pagination
	// call site (chat group list-all); pass a generous page size.
	params := map[string]any{"limit": rt.Int("limit")}
	if cursor := strings.TrimSpace(rt.Str("cursor")); cursor != "" && cursor != "0" {
		params["cursor"] = cursor
	}
	if rt.Bool("page-all") {
		payload, err := readAllMyGroups(rt, params)
		if outputErr := rt.Output(payload); outputErr != nil {
			return outputErr
		}
		return err
	}
	data, err := rt.CallMCPData("im", "list_my_groups_pagination", params)
	if err != nil {
		return err
	}
	payload := myGroupsPayload(rt, myGroupsExtract(data))
	chatmsg.ApplyPagination(payload, data)
	payload["pagesFetched"] = 1
	if payload["complete"] == true {
		payload["stopReason"] = "source_complete"
	} else {
		payload["stopReason"] = "single_page"
	}
	return rt.Output(payload)
}

func myGroupsPayload(rt *shortcut.RuntimeContext, groups []map[string]any) map[string]any {
	typeFilter := strings.TrimSpace(rt.Str("type"))
	projected := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		row := myGroupsProject(group)
		if typeFilter != "" {
			groupType, _ := row["type"].(string)
			if !strings.EqualFold(strings.TrimSpace(groupType), typeFilter) {
				continue
			}
		}
		projected = append(projected, row)
	}
	return map[string]any{"count": len(projected), "groups": projected}
}

func readAllMyGroups(rt *shortcut.RuntimeContext, baseParams map[string]any) (map[string]any, error) {
	pageLimit := defaultChatPageLimit(rt.Int("page-limit"), myGroupsDefaultPageLimit)
	cursorValue := baseParams["cursor"]
	cursorKey := myGroupsCursorString(cursorValue)
	if cursorKey == "" {
		cursorKey = "0"
	}
	seenCursors := map[string]bool{cursorKey: true}
	seenGroups := map[string]bool{}
	allGroups := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	complete := false
	hasMore := false
	stopReason := "source_complete"
	truncatedByPageLimit := false
	truncatedByResultLimit := false
	eligibleCount := 0
	var nextCursor any

	for pagesFetched < pageLimit {
		if pagesFetched > 0 {
			if err := shortcut.WaitAutoPageDelay(rt); err != nil {
				failures = append(failures, map[string]any{
					"page": pagesFetched + 1, "stage": "delay", "cursor": cursorKey, "error": err.Error(),
				})
				stopReason = "delay_interrupted"
				break
			}
		}
		pageSize, _ := baseParams["limit"].(int)
		params := map[string]any{"limit": shortcut.AutoPageRequestSize(rt, pageSize, eligibleCount)}
		if cursorKey != "0" {
			params["cursor"] = cursorValue
		}
		data, err := rt.CallMCPData("im", "list_my_groups_pagination", params)
		if err != nil {
			if pagesFetched == 0 {
				return nil, err
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "cursor": cursorKey, "error": err.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		pageGroups := myGroupsExtract(data)
		overflowOnPage := false
		for _, group := range pageGroups {
			id := myGroupsStr(group, "openConversationId", "openConversationID", "conversationId", "openCid", "cid", "id")
			if id != "" && seenGroups[id] {
				continue
			}
			if id != "" {
				seenGroups[id] = true
			}
			if maxItems := rt.Int("max-items"); maxItems > 0 && myGroupsMatchesFilter(rt, group) && eligibleCount >= maxItems {
				truncatedByResultLimit = true
				overflowOnPage = true
				continue
			}
			allGroups = append(allGroups, group)
			if myGroupsMatchesFilter(rt, group) {
				eligibleCount++
			}
		}

		page := chatmsg.Pagination(data)
		pageHasMore, known := page["hasMore"].(bool)
		if !known {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "我的群列表下层未返回可靠的 hasMore，无法证明结果完整",
			})
			stopReason = "pagination_error"
			break
		}
		hasMore = pageHasMore
		if overflowOnPage {
			hasMore = true
			nextCursor = nil
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "我的群列表下层返回的匹配条数超过请求的剩余额度，无法生成不跳项的安全续页游标",
			})
			stopReason = "pagination_error"
			break
		}
		if !hasMore {
			complete = true
			nextCursor = nil
			stopReason = "source_complete"
			break
		}
		nextCursor = page["nextCursor"]
		nextKey := myGroupsCursorString(nextCursor)
		if nextKey == "" || seenCursors[nextKey] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "我的群列表下层返回 hasMore=true，但 nextCursor 缺失、无效或未前进",
			})
			stopReason = "pagination_error"
			break
		}
		seenCursors[nextKey] = true
		cursorKey = nextKey
		cursorValue = nextCursor
		if maxItems := rt.Int("max-items"); maxItems > 0 && eligibleCount >= maxItems {
			truncatedByResultLimit = true
			stopReason = "result_limit"
			break
		}
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit && !truncatedByResultLimit {
		truncatedByPageLimit = true
		stopReason = "page_limit"
	}

	payload := myGroupsPayload(rt, allGroups)
	payload["pagesFetched"] = pagesFetched
	payload["paginationKnown"] = true
	payload["complete"] = complete && len(failures) == 0
	payload["hasMore"] = hasMore
	payload["stopReason"] = stopReason
	payload["truncatedByPageLimit"] = truncatedByPageLimit
	payload["truncatedByResultLimit"] = truncatedByResultLimit
	payload["failedCount"] = len(failures)
	payload["failures"] = failures
	payload["partial"] = len(failures) > 0 && len(allGroups) > 0
	chatmsg.ApplyTruncation(payload)
	if hasMore && nextCursor != nil {
		payload["nextCursor"] = nextCursor
	}
	if len(failures) == 0 {
		return payload, nil
	}
	return payload, apperrors.NewAPI(
		fmt.Sprintf("我的群列表分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
		apperrors.WithOperation("im/list_my_groups_pagination"),
		apperrors.WithReason("my_groups_incomplete"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(true),
		apperrors.WithHint("请根据 failures 和 nextCursor 重试"),
	)
}

func myGroupsMatchesFilter(rt *shortcut.RuntimeContext, group map[string]any) bool {
	typeFilter := strings.TrimSpace(rt.Str("type"))
	if typeFilter == "" {
		return true
	}
	groupType, _ := myGroupsProject(group)["type"].(string)
	return strings.EqualFold(strings.TrimSpace(groupType), typeFilter)
}

func myGroupsCursorString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" || text == "0" {
		return ""
	}
	return text
}

// myGroupsExtract walks a list_my_groups_pagination response and returns its
// group entries. The gateway wraps the list under one of several common
// container keys, so we probe them (and one nested level) before giving up.
func myGroupsExtract(data map[string]any) []map[string]any {
	for _, key := range []string{"result", "list", "groups", "groupList", "items", "data", "records", "conversations"} {
		if arr, ok := data[key].([]any); ok {
			return myGroupsToMaps(arr)
		}
		if inner, ok := data[key].(map[string]any); ok {
			for _, k2 := range []string{"list", "groups", "groupList", "items", "records", "result", "conversations"} {
				if arr, ok := inner[k2].([]any); ok {
					return myGroupsToMaps(arr)
				}
			}
		}
	}
	return nil
}

func myGroupsToMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// myGroupsProject reshapes a single group into the projected key fields, probing
// multiple candidate keys for each because the response shape isn't guaranteed.
func myGroupsProject(m map[string]any) map[string]any {
	row := map[string]any{}
	if v := myGroupsStr(m, "openConversationId", "openConversationID", "conversationId", "openCid", "cid", "id"); v != "" {
		row["conversationId"] = v
	}
	if v := myGroupsStr(m, "name", "groupName", "title", "conversationName", "chatName"); v != "" {
		row["name"] = v
	}
	if v := myGroupsStr(m, "ownerUserId", "owner", "ownerId", "ownerOpenDingTalkId", "ownerOpenId", "groupOwnerId"); v != "" {
		row["owner"] = v
	}
	if v, ok := myGroupsInt(m, "memberCount", "memberNum", "memberSize", "userCount", "totalMember", "count"); ok {
		row["memberCount"] = v
	}
	if v := myGroupsStr(m, "groupType", "conversationType", "type", "chatType"); v != "" {
		row["type"] = v
	}
	return row
}

func myGroupsStr(m map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := m[key].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		}
	}
	return ""
}

func myGroupsInt(m map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		switch v := m[key].(type) {
		case float64:
			return int64(v), true
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

func init() {
	shortcut.Register(MyGroups)
}

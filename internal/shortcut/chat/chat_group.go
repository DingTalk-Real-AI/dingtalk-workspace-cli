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

package chat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

const (
	chatSearchHardPageLimit  = 500
	chatSearchMaxWindowSize  = 100
	chatListAllHardPageLimit = 500
)

// ChatSearch searches groups by keyword (search_groups on the im server).
var ChatSearch = shortcut.Shortcut{
	Service:                  "chat",
	Command:                  "+chat-search",
	Aliases:                  []string{"+chat-group-search", "+search-group"},
	SinglePositionalAliasFor: "query",
	Product:                  "im",
	Description:              "按关键词分页搜索群聊，支持有界自动翻页和完整性检查",
	Intent:                   "当你只记得群名称关键词、需要拿到群 openConversationId 以便发消息或管理该群时使用；默认读取一页，明确要求全部候选时加 --page-all，并用 --page-limit 保持有界。结果按 openConversationId 去重，并公开 complete、hasMore、nextCursor、stopReason 和 failures，避免把截断或失败结果误当完整候选集。",
	Risk:                     shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_search",
			CanonicalPath:  "chat.shortcut_chat_search",
			CLIPath:        "chat +chat-search",
			PrimaryCLIPath: "chat +chat-search",
		},
		Description: "按关键词分页搜索群聊，支持有界自动翻页和完整性检查",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按关键词分页搜索群聊，支持有界自动翻页和完整性检查",
			UseWhen:      []string{"当你只记得群名称关键词、需要拿到群 openConversationId 以便发消息或管理该群时使用；默认读取一页，明确要求全部候选时加 --page-all，并用 --page-limit 保持有界。结果按 openConversationId 去重，并公开 complete、hasMore、nextCursor、stopReason 和 failures，避免把截断或失败结果误当完整候选集。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-search --query \"项目冲刺\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "query", Type: shortcut.FlagString, Desc: "群名称关键词"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "--query 的别名", Hidden: true},
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "每页返回数量；显式页大小必须在 1-100 之间"},
		{Name: "page-size", Type: shortcut.FlagInt, Desc: "--limit 的 Lark 对齐别名；显式页大小必须在 1-100 之间"},
		{Name: "size", Type: shortcut.FlagInt, Desc: "--limit 的旧版别名", Hidden: true},
		{Name: "cursor", Type: shortcut.FlagString, Default: "0", Desc: "分页游标，翻页传 nextCursor"},
		{Name: "page-token", Type: shortcut.FlagString, Desc: "--cursor 的 Lark 对齐别名"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "自动读取全部群搜索分页；--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已设置免打扰的群聊"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"query", "keyword"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"limit", "page-size", "size"}},
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"cursor", "page-token"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit", "page-size"}, Description: "显式页大小必须在 1-100 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	},
	Tips: []string{
		`dws chat +chat-search --query "项目冲刺"`,
		`dws chat +chat-search --query "项目" --page-size 100 --page-all --page-limit 20`,
	},
	Validate: validateChatSearch,
	Execute:  executeChatSearch,
}

func validateChatSearch(rt *shortcut.RuntimeContext) error {
	if size := chatSearchPageSize(rt); size < 1 || size > 100 {
		return apperrors.NewValidation("--limit/--page-size/--size 必须在 1-100 之间")
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > chatSearchHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	return nil
}

func chatSearchPageSize(rt *shortcut.RuntimeContext) int {
	if rt.Changed("page-size") {
		return rt.Int("page-size")
	}
	if rt.Changed("size") {
		return rt.Int("size")
	}
	return rt.Int("limit")
}

func chatSearchStartCursor(rt *shortcut.RuntimeContext) string {
	if rt.Changed("page-token") {
		if value := strings.TrimSpace(rt.Str("page-token")); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(rt.Str("cursor")); value != "" {
		return value
	}
	return "0"
}

func chatSearchRequestParams(query string, pageSize int, cursor string, excludeMuted bool) map[string]any {
	params := map[string]any{
		"keyword": query,
		"limit":   pageSize,
		"cursor":  cursor,
	}
	if excludeMuted {
		params["excludeMuted"] = true
	}
	return params
}

func executeChatSearch(rt *shortcut.RuntimeContext) error {
	query := strings.TrimSpace(rt.StrFirst("query", "keyword"))
	pageSize := chatSearchPageSize(rt)
	requestPageSize := pageSize
	pageLimit := 1
	if rt.Bool("page-all") {
		pageLimit = rt.Int("page-limit")
	}
	cursor := chatSearchStartCursor(rt)
	if rt.DryRun() {
		return rt.CallMCP("search_groups", chatSearchRequestParams(query, pageSize, cursor, rt.Bool("exclude-muted")))
	}
	initialCursor := cursor
	seenCursors := map[string]bool{cursor: true}
	seenChats := map[string]bool{}
	chats := make([]map[string]any, 0)
	failures := make([]map[string]any, 0)
	pagesFetched := 0
	paginationKnown := true
	complete := false
	hasMore := false
	nextCursor := ""
	stopReason := "source_complete"
	truncatedByPageLimit := false
	maxWindowProbeUsed := false
	completionEvidence := ""

	for pagesFetched < pageLimit {
		params := chatSearchRequestParams(query, requestPageSize, cursor, rt.Bool("exclude-muted"))
		data, err := rt.CallMCPData("im", "search_groups", params)
		if err != nil {
			if pagesFetched == 0 {
				return err
			}
			failures = append(failures, map[string]any{
				"page": pagesFetched + 1, "stage": "read", "cursor": cursor, "error": err.Error(),
			})
			stopReason = "read_failure"
			break
		}
		pagesFetched++
		pageItems := chatSearchItems(data)
		for _, chat := range pageItems {
			id := strings.TrimSpace(fmt.Sprint(chat["openConversationId"]))
			if id != "" && id != "<nil>" && seenChats[id] {
				continue
			}
			if id != "" && id != "<nil>" {
				seenChats[id] = true
			}
			chats = append(chats, chat)
		}

		page := chatmsg.Pagination(data)
		pageHasMore, hasMoreKnown := page["hasMore"].(bool)
		nextCursor = chatSearchCursorString(page["nextCursor"])
		if !hasMoreKnown {
			switch {
			case nextCursor != "":
				pageHasMore = true
			case len(pageItems) < requestPageSize:
				paginationKnown = false
				complete = true
				hasMore = false
				stopReason = "legacy_short_page"
			default:
				paginationKnown = false
				failures = append(failures, map[string]any{
					"page": pagesFetched, "stage": "pagination",
					"error": "群搜索返回满页结果但缺少 hasMore/nextCursor，无法证明结果完整",
				})
				stopReason = "pagination_error"
			}
			if complete || len(failures) > 0 {
				break
			}
		}
		hasMore = pageHasMore
		if !hasMore {
			// A full first page with hasMore=false/no cursor is the live legacy
			// shape that can hide additional rows. Once a prior page supplied a
			// valid continuation cursor, its terminal hasMore=false is trustworthy.
			fullPageWithoutCursor := len(pageItems) >= requestPageSize && nextCursor == "" &&
				(pagesFetched == 1 || (maxWindowProbeUsed && pagesFetched == 2))
			if fullPageWithoutCursor {
				paginationKnown = false
				if rt.Bool("page-all") && initialCursor == "0" && !maxWindowProbeUsed && requestPageSize < chatSearchMaxWindowSize && pagesFetched < pageLimit {
					maxWindowProbeUsed = true
					requestPageSize = chatSearchMaxWindowSize
					cursor = initialCursor
					stopReason = "max_window_probe"
					continue
				}
				if !rt.Bool("page-all") {
					complete = false
					stopReason = "single_page_full_untrusted"
					break
				}
				if pagesFetched >= pageLimit && requestPageSize < chatSearchMaxWindowSize {
					truncatedByPageLimit = true
					complete = false
					stopReason = "page_limit"
					break
				}
				failures = append(failures, map[string]any{
					"page": pagesFetched, "stage": "pagination",
					"error": "群搜索在最大窗口仍返回满页且没有可用 nextCursor，无法证明结果完整",
				})
				complete = false
				stopReason = "pagination_error"
				break
			}
			complete = true
			nextCursor = ""
			if maxWindowProbeUsed {
				paginationKnown = false
				completionEvidence = "max_window_short_page"
				stopReason = "max_window_short_page"
			} else {
				completionEvidence = "backend_has_more_false_short_page"
				stopReason = "source_complete"
			}
			break
		}
		if nextCursor == "" || seenCursors[nextCursor] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "群搜索返回 hasMore=true，但 nextCursor 缺失或未前进",
			})
			stopReason = "pagination_error"
			break
		}
		if !rt.Bool("page-all") {
			stopReason = "single_page"
			break
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}

	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit {
		truncatedByPageLimit = rt.Bool("page-all")
		if truncatedByPageLimit {
			stopReason = "page_limit"
		}
	}
	payload := map[string]any{
		"query":                query,
		"count":                len(chats),
		"chats":                chats,
		"pagesFetched":         pagesFetched,
		"paginationKnown":      paginationKnown,
		"complete":             complete && len(failures) == 0,
		"hasMore":              hasMore,
		"nextCursor":           nextCursor,
		"stopReason":           stopReason,
		"truncatedByPageLimit": truncatedByPageLimit,
		"failedCount":          len(failures),
		"failures":             failures,
		"partial":              len(failures) > 0 && len(chats) > 0,
		"requestedPageSize":    pageSize,
		"effectivePageSize":    requestPageSize,
	}
	if completionEvidence != "" {
		payload["completionEvidence"] = completionEvidence
	}
	if rt.Bool("exclude-muted") {
		payload["filter"] = map[string]any{"excludeMuted": true}
	}
	if err := rt.Output(payload); err != nil {
		return err
	}
	if len(failures) > 0 {
		return apperrors.NewAPI(
			fmt.Sprintf("群搜索分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
			apperrors.WithOperation("im/search_groups"),
			apperrors.WithReason("chat_search_incomplete"),
			apperrors.WithOrigin("mcp_gateway"),
			apperrors.WithFailureStage("pagination"),
			apperrors.WithExecutionStarted(true),
			apperrors.WithRetryable(true),
			apperrors.WithHint("请根据 failures 和 nextCursor 重试"),
		)
	}
	return nil
}

func chatSearchItems(data map[string]any) []map[string]any {
	if data == nil {
		return []map[string]any{}
	}
	scopes := []map[string]any{data}
	for _, key := range []string{"result", "data"} {
		if nested, ok := data[key].(map[string]any); ok {
			scopes = append(scopes, nested)
		}
	}
	for _, scope := range scopes {
		for _, key := range []string{"groups", "chats", "items", "list", "result"} {
			if raw, ok := scope[key].([]any); ok {
				return projectChatSearchItems(raw)
			}
		}
	}
	return []map[string]any{}
}

func projectChatSearchItems(raw []any) []map[string]any {
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected := make(map[string]any, len(group)+1)
		for key, value := range group {
			projected[key] = value
		}
		if _, exists := projected["openConversationId"]; !exists {
			for _, key := range []string{"conversationId", "chatId", "id"} {
				if value, ok := group[key]; ok {
					projected["openConversationId"] = value
					break
				}
			}
		}
		if _, exists := projected["name"]; !exists {
			for _, key := range []string{"title", "conversationName"} {
				if value, ok := group[key]; ok {
					projected["name"] = value
					break
				}
			}
		}
		out = append(out, projected)
	}
	return out
}

func chatSearchCursorString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" || text == "0" {
		return ""
	}
	return text
}

// ChatMembersList lists members of a group (get_group_members, chat server).
// ChatMembersGet batch-queries member detail by ids (list_group_member_by_ids, im).
var ChatMembersGet = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-members-get",
	Product:     "im",
	Description: "根据成员 openDingTalkId 批量查询群成员详情",
	Intent:      "当你已有若干成员的 openDingTalkId、需要批量获取他们在该群内的详情（群昵称、角色等）时使用；只读，需传群 openConversationId 和成员 openDingTalkId 列表。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "成员 openDingTalkId 列表", Required: true},
	},
	Tips: []string{`dws chat +chat-members-get --id <openConversationId> --users odid1,odid2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("list_group_member_by_ids", map[string]any{
			"openConversationId":    rt.Str("id"),
			"cid":                   rt.Str("id"),
			"memberOpenDingTalkIds": rt.StrSlice("users"),
		})
	},
}

// ChatMemberAdd adds members to a group (add_group_member, chat server).
// ChatMemberRemove removes members from a group (remove_group_member, chat server).
// ChatUpdateName renames a group (update_group_name, chat server).
// ChatTransferOwner transfers group ownership (transfer_group_owner, im).
var ChatTransferOwner = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-transfer-owner",
	Product:     "im",
	Description: "转让群主",
	Intent:      "当你要把群主身份转让给他人时使用；会实际变更群主（自己不再是群主），需传群 openConversationId 和新群主的 userId 或 openDingTalkId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "new-owner", Type: shortcut.FlagString, Desc: "新群主 userId 或 openDingTalkId", Required: true},
	},
	Tips: []string{`dws chat +chat-transfer-owner --group <openConversationId> --new-owner <openDingTalkId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		newOwner := rt.Str("new-owner")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"cid":                rt.Str("group"),
		}
		if isOpenID(newOwner) {
			params["newOwnerOpenDingTalkId"] = newOwner
		} else {
			params["newOwnerUid"] = newOwner
		}
		return rt.CallMCP("transfer_group_owner", params)
	},
}

// ChatInviteURL gets the group invite url (get_group_invite_url, im).
var ChatInviteURL = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-invite-url",
	Product:     "im",
	Description: "获取群邀请链接",
	Intent:      "当你想拿到一条群邀请链接分享给别人加群时使用；--group 可传群 openConversationId 或群名，多命中会安全停止。可用 --expires-seconds 设置有效期（0 表示永久）。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_invite_url",
			CanonicalPath:  "chat.shortcut_chat_invite_url",
			CLIPath:        "chat +chat-invite-url",
			PrimaryCLIPath: "chat +chat-invite-url",
		},
		Description: "获取群邀请链接",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "获取群邀请链接",
			UseWhen:      []string{"当你想拿到一条群邀请链接分享给别人加群时使用；--group 可传群 openConversationId 或群名，多命中会安全停止。可用 --expires-seconds 设置有效期（0 表示永久）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-invite-url --group <openConversationId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId；兼容直接传群名并唯一解析"},
		{Name: "chat-query", Type: shortcut.FlagString, Desc: "--group 的旧版自然名称入口", Hidden: true},
		{Name: "group-query", Type: shortcut.FlagString, Desc: "--chat-query 的兼容别名", Hidden: true},
		{Name: "expires-seconds", Type: shortcut.FlagInt, Desc: "链接有效期（秒），0 表示永久"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"group", "chat-query", "group-query"}},
	},
	Tips: []string{
		`dws chat +chat-invite-url --group <openConversationId>`,
		`dws chat +chat-invite-url --group "项目群"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupID, err := resolveStableOrNamedChat(rt)
		if err != nil {
			return err
		}
		params := map[string]any{
			"openConversationId": groupID,
			"cid":                groupID,
		}
		if rt.Changed("expires-seconds") {
			params["expiresSeconds"] = rt.Int("expires-seconds")
		}
		return rt.CallMCP("get_group_invite_url", params)
	},
}

// ChatQuit quits a group (quit_group, im).
var ChatQuit = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-quit",
	Product:     "im",
	Description: "退出群聊",
	Intent:      "当你想让当前用户主动退出某个群时使用；会实际退群，需传群 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-quit --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("quit_group", map[string]any{"openConversationId": rt.Str("group")})
	},
}

// ChatUpdateIcon updates the group icon (update_group_icon, im).
var ChatUpdateIcon = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-icon",
	Product:     "im",
	Description: "更新群头像",
	Intent:      "当你想更换群头像时使用；会实际更新群头像，需传群 openConversationId 和已上传头像的 mediaId（以 @ 开头）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "icon-media-id", Type: shortcut.FlagString, Desc: "群头像 mediaId（以 @ 开头）", Required: true},
	},
	Tips: []string{`dws chat +chat-update-icon --group <openConversationId> --icon-media-id <mediaId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_icon", map[string]any{
			"openConversationId": rt.Str("group"),
			"iconMediaId":        rt.Str("icon-media-id"),
		})
	},
}

// ChatUpdateSettings updates a group setting (update_group_settings, im).
var ChatUpdateSettings = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-settings",
	Product:     "im",
	Description: "更新群设置（settingKey + status）",
	Intent:      "当你想调整群的某项开关设置（如是否可被搜索 searchable、是否仅管理员可@所有人 onlyAdminCanAtAll）时使用；会实际修改群设置，需传 settingKey 和 status（0关/1开）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "setting-key", Type: shortcut.FlagString, Desc: "群设置项 key，如 searchable / onlyAdminCanAtAll", Required: true},
		{Name: "status", Type: shortcut.FlagInt, Desc: "设置值：0=关闭，1=开启", Required: true},
	},
	Tips: []string{`dws chat +chat-update-settings --group <openConversationId> --setting-key searchable --status 1`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_settings", map[string]any{
			"openConversationId": rt.Str("group"),
			"settingKey":         rt.Str("setting-key"),
			"status":             rt.Int("status"),
		})
	},
}

// ChatDismiss dismisses (destroys) a group (dismiss_group, im).
var ChatDismiss = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-dismiss",
	Product:     "im",
	Description: "解散群聊（不可逆，需群主权限）",
	Intent:      "当你要彻底解散一个群时使用；会实际销毁群聊，不可逆且需群主权限，仅需传群 openConversationId，操作前务必确认。",
	Risk:        shortcut.RiskHighWrite,
	Safety: contract.SafetySpec{
		Effect: "destructive", Risk: "high",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_dismiss",
			CanonicalPath:  "chat.shortcut_chat_dismiss",
			CLIPath:        "chat +chat-dismiss",
			PrimaryCLIPath: "chat +chat-dismiss",
		},
		Description: "解散群聊（不可逆，需群主权限）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "解散群聊（不可逆，需群主权限）",
			UseWhen:      []string{"当你要彻底解散一个群时使用；会实际销毁群聊，不可逆且需群主权限，仅需传群 openConversationId，操作前务必确认。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-dismiss --group <openConversationId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-dismiss --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("dismiss_group", map[string]any{"openConversationId": rt.Str("group")})
	},
}

// ChatSetHistory sets new-member history visibility (update_show_history_msg_option, im).
var ChatSetHistory = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-set-history",
	Product:     "im",
	Description: "设置新成员入群可查看历史消息范围",
	Intent:      "当你想控制新成员入群后能看到多少历史消息时使用；会实际修改群配置，需传群 openConversationId 和范围（FORBIDDEN 不可见 / RECENT_100 最近100条 / ALL 全部）。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_set_history",
			CanonicalPath:  "chat.shortcut_chat_set_history",
			CLIPath:        "chat +chat-set-history",
			PrimaryCLIPath: "chat +chat-set-history",
		},
		Description: "设置新成员入群可查看历史消息范围",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "设置新成员入群可查看历史消息范围",
			UseWhen:      []string{"当你想控制新成员入群后能看到多少历史消息时使用；会实际修改群配置，需传群 openConversationId 和范围（FORBIDDEN 不可见 / RECENT_100 最近100条 / ALL 全部）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-set-history --group <openConversationId> --option RECENT_100"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "option", Type: shortcut.FlagString, Desc: "可见范围", Required: true, Enum: []string{"FORBIDDEN", "RECENT_100", "ALL"}},
	},
	Tips: []string{`dws chat +chat-set-history --group <openConversationId> --option RECENT_100`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_show_history_msg_option", map[string]any{
			"openConversationId": rt.Str("group"),
			"option":             rt.Str("option"),
		})
	},
}

// ChatUpdateNick sets the caller's in-group nickname (update_group_nick, im).
var ChatUpdateNick = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-nick",
	Product:     "im",
	Description: "设置当前用户在群内的群昵称",
	Intent:      "当你想设置当前用户在某个群里显示的群昵称时使用；会实际更新本人在该群的昵称，需传群 openConversationId 和昵称。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_update_nick",
			CanonicalPath:  "chat.shortcut_chat_update_nick",
			CLIPath:        "chat +chat-update-nick",
			PrimaryCLIPath: "chat +chat-update-nick",
		},
		Description: "设置当前用户在群内的群昵称",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "设置当前用户在群内的群昵称",
			UseWhen:      []string{"当你想设置当前用户在某个群里显示的群昵称时使用；会实际更新本人在该群的昵称，需传群 openConversationId 和昵称。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-update-nick --group <openConversationId> --nick \"我的群昵称\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "nick", Type: shortcut.FlagString, Desc: "个人群昵称", Required: true},
	},
	Tips: []string{`dws chat +chat-update-nick --group <openConversationId> --nick "我的群昵称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_group_nick", map[string]any{
			"openConversationId": rt.Str("group"),
			"nick":               rt.Str("nick"),
		})
	},
}

// ChatUpdateAlias sets the caller's private alias for a group (update_user_group_alias, im).
var ChatUpdateAlias = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-update-alias",
	Product:     "im",
	Description: "设置群备注（仅自己可见）",
	Intent:      "当你想给某个群设置仅自己可见的备注名以便区分同名群时使用；会实际保存本人对该群的备注，需传群 openConversationId 和备注标题。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_update_alias",
			CanonicalPath:  "chat.shortcut_chat_update_alias",
			CLIPath:        "chat +chat-update-alias",
			PrimaryCLIPath: "chat +chat-update-alias",
		},
		Description: "设置群备注（仅自己可见）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "设置群备注（仅自己可见）",
			UseWhen:      []string{"当你想给某个群设置仅自己可见的备注名以便区分同名群时使用；会实际保存本人对该群的备注，需传群 openConversationId 和备注标题。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-update-alias --group <openConversationId> --alias-title \"项目A群\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "alias-title", Type: shortcut.FlagString, Desc: "群备注标题", Required: true},
	},
	Tips: []string{`dws chat +chat-update-alias --group <openConversationId> --alias-title "项目A群"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_user_group_alias", map[string]any{
			"openConversationId": rt.Str("group"),
			"aliasTitle":         rt.Str("alias-title"),
		})
	},
}

// ChatListMine lists groups the caller owns/administers (list_owned_or_admin_groups, im).
var ChatListMine = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list-mine",
	Product:     "im",
	Description: "拉取我创建/管理的群",
	Intent:      "当你想查看自己作为群主或管理员在管理哪些群时使用；只读分页返回，可用 --role OWNER/ADMIN 按角色过滤。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_list_mine",
			CanonicalPath:  "chat.shortcut_chat_list_mine",
			CLIPath:        "chat +chat-list-mine",
			PrimaryCLIPath: "chat +chat-list-mine",
		},
		Description: "拉取我创建/管理的群",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "拉取我创建/管理的群",
			UseWhen:      []string{"当你想查看自己作为群主或管理员在管理哪些群时使用；只读分页返回，可用 --role OWNER/ADMIN 按角色过滤。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-list-mine --role OWNER"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "role", Type: shortcut.FlagString, Desc: "角色过滤", Enum: []string{"OWNER", "ADMIN"}},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "最多返回群数量，不传返回全部"},
		{Name: "exclude-muted", Type: shortcut.FlagBool, Desc: "排除已设置免打扰的群聊"},
	},
	Tips: []string{`dws chat +chat-list-mine --role OWNER`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Changed("role") {
			params["roleFilter"] = rt.Str("role")
		}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Bool("exclude-muted") {
			params["excludeMuted"] = true
		}
		data, err := rt.CallMCPData("im", "list_owned_or_admin_groups", params)
		if err != nil {
			return err
		}
		groups := chatListMineProject(data)
		payload := map[string]any{"count": len(groups), "groups": groups}
		chatmsg.ApplyPagination(payload, data)
		return rt.Output(payload)
	},
}

// chatListMineProject reshapes list_owned_or_admin_groups into a clean group
// list ({openConversationId, name, role, ownerUserId}) — output-projection
// clean output projection. List container and per-item field names are probed
// defensively across candidate keys so shape drift yields an empty list rather
// than a crash or fabricated data.
func chatListMineProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openConversationId", "openconversation_id", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "groupName", "title"); ok {
			row["name"] = v
		}
		if v, ok := chatGroupFirst(m, "role", "roleType", "memberRole"); ok {
			row["role"] = v
		}
		if v, ok := chatGroupFirst(m, "ownerUserId", "ownerId", "owner"); ok {
			row["ownerUserId"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ChatListAll paginates all groups the caller joined (list_my_groups_pagination, im).
var ChatListAll = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list-all",
	Product:     "im",
	Description: "分页拉取我加入的所有群列表",
	Intent:      "当你想遍历当前用户加入的所有群做统计或批量操作时使用；只读分页返回全部已加入的群列表。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_list_all",
			CanonicalPath:  "chat.shortcut_chat_list_all",
			CLIPath:        "chat +chat-list-all",
			PrimaryCLIPath: "chat +chat-list-all",
		},
		Description: "分页拉取我加入的所有群列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "分页拉取我加入的所有群列表",
			UseWhen:      []string{"当你想遍历当前用户加入的所有群做统计或批量操作时使用；只读分页返回全部已加入的群列表。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-list-all --limit 50"},
		},
	},
	Flags: append([]shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Default: "100", Desc: "每页返回数量；--limit 必须在 1-200 之间"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传 nextCursor"},
		{Name: "page-all", Type: shortcut.FlagBool, Desc: "沿 nextCursor 自动读取全部已加入群；--page-limit 仅与 --page-all 一起使用且范围 1-500；--max-items/--page-delay 仅与 --page-all 一起使用；值必须大于等于 0"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlFlags()...),
	Constraints: append([]shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"limit"}, Description: "--limit 必须在 1-200 之间"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-all", "page-limit"}, Description: "--page-limit 仅与 --page-all 一起使用且范围 1-500"},
	}, shortcut.AutoPageControlConstraints()...),
	Tips: []string{
		`dws chat +chat-list-all --limit 50`,
		`dws chat +chat-list-all --limit 200 --page-all --page-limit 50`,
	},
	Validate: validateChatListAll,
	Execute:  executeChatListAll,
}

func validateChatListAll(rt *shortcut.RuntimeContext) error {
	if limit := rt.Int("limit"); limit < 1 || limit > 200 {
		return apperrors.NewValidation("--limit 必须在 1-200 之间")
	}
	if !rt.Bool("page-all") && rt.Changed("page-limit") {
		return apperrors.NewValidation("--page-limit 仅与 --page-all 一起使用")
	}
	if rt.Bool("page-all") {
		if limit := rt.Int("page-limit"); limit < 1 || limit > chatListAllHardPageLimit {
			return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
		}
	}
	if err := shortcut.ValidateAutoPageControls(rt); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	return nil
}

func executeChatListAll(rt *shortcut.RuntimeContext) error {
	params := map[string]any{}
	if rt.Int("limit") > 0 {
		params["limit"] = rt.Int("limit")
	}
	if c := rt.Str("cursor"); c != "" && c != "0" {
		params["cursor"] = c
	}
	if rt.Bool("page-all") {
		payload, err := readAllChatListAll(rt, params)
		if outputErr := rt.Output(payload); outputErr != nil {
			return outputErr
		}
		return err
	}
	data, err := rt.CallMCPData("im", "list_my_groups_pagination", params)
	if err != nil {
		return err
	}
	groups := chatListAllProject(data)
	payload := map[string]any{"count": len(groups), "groups": groups}
	chatmsg.ApplyPagination(payload, data)
	payload["pagesFetched"] = 1
	if payload["complete"] == true {
		payload["stopReason"] = "source_complete"
	} else {
		payload["stopReason"] = "single_page"
	}
	return rt.Output(payload)
}

func readAllChatListAll(rt *shortcut.RuntimeContext, baseParams map[string]any) (map[string]any, error) {
	pageLimit := rt.Int("page-limit")
	cursorValue := baseParams["cursor"]
	cursorKey := chatListAllCursorString(cursorValue)
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
		params := map[string]any{"limit": shortcut.AutoPageRequestSize(rt, pageSize, len(allGroups))}
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
		pageGroups := chatListAllProject(data)
		overflowOnPage := false
		for _, group := range pageGroups {
			id := strings.TrimSpace(fmt.Sprint(group["openConversationId"]))
			if id == "<nil>" {
				id = ""
			}
			if id != "" && seenGroups[id] {
				continue
			}
			if id != "" {
				seenGroups[id] = true
			}
			if maxItems := rt.Int("max-items"); maxItems > 0 && len(allGroups) >= maxItems {
				truncatedByResultLimit = true
				overflowOnPage = true
				continue
			}
			allGroups = append(allGroups, group)
		}

		page := chatmsg.Pagination(data)
		pageHasMore, known := page["hasMore"].(bool)
		if !known {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "已加入群列表下层未返回可靠的 hasMore，无法证明结果完整",
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
				"error": "已加入群列表下层返回条数超过请求的剩余额度，无法生成不跳项的安全续页游标",
			})
			stopReason = "pagination_error"
			break
		}
		if !hasMore {
			complete = !truncatedByResultLimit
			nextCursor = nil
			if truncatedByResultLimit {
				hasMore = true
				stopReason = "result_limit"
			} else {
				stopReason = "source_complete"
			}
			break
		}
		nextCursor = page["nextCursor"]
		nextKey := chatListAllCursorString(nextCursor)
		if nextKey == "" || seenCursors[nextKey] {
			failures = append(failures, map[string]any{
				"page": pagesFetched, "stage": "pagination",
				"error": "已加入群列表下层返回 hasMore=true，但 nextCursor 缺失、无效或未前进",
			})
			stopReason = "pagination_error"
			break
		}
		seenCursors[nextKey] = true
		cursorKey = nextKey
		cursorValue = nextCursor
		if maxItems := rt.Int("max-items"); maxItems > 0 && len(allGroups) >= maxItems {
			truncatedByResultLimit = true
			stopReason = "result_limit"
			break
		}
	}
	if !complete && hasMore && len(failures) == 0 && pagesFetched >= pageLimit && !truncatedByResultLimit {
		truncatedByPageLimit = true
		stopReason = "page_limit"
	}

	payload := map[string]any{
		"count": len(allGroups), "groups": allGroups,
		"pagesFetched": pagesFetched, "paginationKnown": true,
		"complete": complete && len(failures) == 0, "hasMore": hasMore,
		"stopReason": stopReason, "truncatedByPageLimit": truncatedByPageLimit,
		"truncatedByResultLimit": truncatedByResultLimit,
		"failedCount":            len(failures), "failures": failures,
		"partial": len(failures) > 0 && len(allGroups) > 0,
	}
	chatmsg.ApplyTruncation(payload)
	if hasMore && nextCursor != nil {
		payload["nextCursor"] = nextCursor
	}
	if len(failures) == 0 {
		return payload, nil
	}
	return payload, apperrors.NewAPI(
		fmt.Sprintf("已加入群列表分页未完成：成功读取 %d 页，存在 %d 个失败项", pagesFetched, len(failures)),
		apperrors.WithOperation("im/list_my_groups_pagination"),
		apperrors.WithReason("chat_list_all_incomplete"),
		apperrors.WithOrigin("mcp_gateway"),
		apperrors.WithFailureStage("pagination"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(true),
		apperrors.WithHint("请根据 failures 和 nextCursor 重试"),
	)
}

func chatListAllCursorString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" || text == "0" {
		return ""
	}
	return text
}

// chatListAllProject reshapes list_my_groups_pagination into a clean group list
// ({openConversationId, name}) — clean output projection. List
// container and per-item field names are probed defensively across candidate
// keys so shape drift yields an empty list rather than a crash or fabricated
// data.
func chatListAllProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openConversationId", "openconversation_id", "conversationId", "id"); ok {
			row["openConversationId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "groupName", "title"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// chatGroupResolveList locates the list payload inside a group-list response,
// tolerating a bare top-level array or nesting under common envelope keys.
func chatGroupResolveList(data map[string]any) []any {
	if data == nil {
		return []any{}
	}
	// "bots" and "roles" are probed too: chatBotsProject and
	// chatRoleListProject reuse this resolver, while their backing tools nest
	// lists under result.bots and result.roles. Without those keys the
	// shortcuts silently return empty despite the group having bots or roles.
	// Group listings key on groups/conversations, which are probed first, so
	// adding these sibling containers is harmless to them.
	for _, key := range []string{"result", "data", "list", "items", "groups", "conversations", "bots", "roles"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return arr
		}
		if inner, ok := v.(map[string]any); ok {
			for _, ik := range []string{"list", "items", "groups", "conversations", "bots", "roles", "result", "data"} {
				if arr, ok := inner[ik].([]any); ok {
					return arr
				}
			}
		}
	}
	return []any{}
}

// chatGroupFirst returns the first present candidate key's value.
func chatGroupFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

// ChatListJoinRequests paginates join-validation records (list_apply_join_group_records, im).
var ChatListJoinRequests = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-list-join-requests",
	Product:     "im",
	Description: "分页拉取入群验证记录",
	Intent:      "当你作为群主/管理员想查看待处理的入群申请时使用；只读分页返回入群验证记录（含 recordId、申请人与邀请人 ID），供后续用 chat-audit-join 审批。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_list_join_requests",
			CanonicalPath:  "chat.shortcut_chat_list_join_requests",
			CLIPath:        "chat +chat-list-join-requests",
			PrimaryCLIPath: "chat +chat-list-join-requests",
		},
		Description: "分页拉取入群验证记录",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "分页拉取入群验证记录",
			UseWhen:      []string{"当你作为群主/管理员想查看待处理的入群申请时使用；只读分页返回入群验证记录（含 recordId、申请人与邀请人 ID），供后续用 chat-audit-join 审批。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-list-join-requests --limit 30"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "limit", Type: shortcut.FlagInt, Default: "20", Desc: "单页数量（最大 50）"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标，翻页传 nextCursor"},
	},
	Tips: []string{`dws chat +chat-list-join-requests --limit 30`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{}
		if rt.Int("limit") > 0 {
			params["limit"] = rt.Int("limit")
		}
		if rt.Changed("cursor") {
			params["cursor"] = rt.Str("cursor")
		}
		return rt.CallMCP("list_apply_join_group_records", params)
	},
}

// ChatAuditJoin audits a join-validation record (audit_join_group, im).
var ChatAuditJoin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-audit-join",
	Product:     "im",
	Description: "审批入群验证（通过/拒绝/删除/忽略/拉黑）",
	Intent:      "当你要处理某条入群申请时使用；会实际执行通过/拒绝/删除/忽略/拉黑动作，需传群 openConversationId、recordId、申请人与邀请人 userId 及 status。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "record-id", Type: shortcut.FlagInt, Desc: "申请记录 ID", Required: true},
		{Name: "applicant", Type: shortcut.FlagString, Desc: "申请人 userId", Required: true},
		{Name: "inviter", Type: shortcut.FlagString, Desc: "邀请人 userId", Required: true},
		{Name: "status", Type: shortcut.FlagString, Desc: "审批动作", Required: true, Enum: []string{"AuditApprove", "AuditDelete", "AuditIgnore", "AuditRefuse", "AuditBlock"}},
		{Name: "description", Type: shortcut.FlagString, Desc: "审批说明"},
	},
	Tips: []string{`dws chat +chat-audit-join --group <openConversationId> --record-id 123 --applicant <userId> --inviter <userId> --status AuditApprove`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"applyRecordId":      rt.Int("record-id"),
			"applicantUid":       rt.Str("applicant"),
			"inviterUid":         rt.Str("inviter"),
			"status":             rt.Str("status"),
		}
		if rt.Changed("description") {
			params["auditDescription"] = rt.Str("description")
		}
		return rt.CallMCP("audit_join_group", params)
	},
}

// ChatGetByID looks up a group by numeric group id (get_conv_info_by_group_id, im).
var ChatGetByID = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-get-by-id",
	Product:     "im",
	Description: "根据群号获取群聊信息",
	Intent:      "当你只知道群号（数字）、需要换取群 openConversationId 及群信息时使用；只读，需传 --group-id。",
	Risk:        shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group-id", Type: shortcut.FlagInt, Desc: "群号（数字类型）", Required: true},
	},
	Tips: []string{`dws chat +chat-get-by-id --group-id 12345678`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("get_conv_info_by_group_id", map[string]any{"groupId": rt.Int("group-id")})
	},
}

// ChatAddBot adds a custom robot to a group (add_robot_to_group, bot).
var ChatAddBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-add-bot",
	Product:     "bot",
	Description: "将机器人添加到群中",
	Intent:      "当你想把某个机器人添加进群（比如让日报机器人进群播报）时使用；会实际把机器人加入群聊，需传机器人 robotCode 和群 openConversationId。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "robot-code", Type: shortcut.FlagString, Desc: "机器人 Code", Required: true},
		{Name: "id", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-add-bot --robot-code <robotCode> --id <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("add_robot_to_group", map[string]any{
			"robotCode":          rt.Str("robot-code"),
			"openConversationId": rt.Str("id"),
		})
	},
}

// ChatBots lists robots in a group (list_group_bots, bot).
var ChatBots = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-bots",
	Product:     "bot",
	Description: "查看群内所有机器人",
	Intent:      "当你想查看某个群里已添加了哪些机器人时使用；--group 可传群 openConversationId 或群名，多命中会安全停止。只读返回机器人列表（含 openBotId，供后续移除）。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_bots",
			CanonicalPath:  "chat.shortcut_chat_bots",
			CLIPath:        "chat +chat-bots",
			PrimaryCLIPath: "chat +chat-bots",
		},
		Description: "查看群内所有机器人",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查看群内所有机器人",
			UseWhen:      []string{"当你想查看某个群里已添加了哪些机器人时使用；--group 可传群 openConversationId 或群名，多命中会安全停止。只读返回机器人列表（含 openBotId，供后续移除）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-bots --group <openConversationId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId；兼容直接传群名并唯一解析"},
		{Name: "chat-query", Type: shortcut.FlagString, Desc: "--group 的旧版自然名称入口", Hidden: true},
		{Name: "group-query", Type: shortcut.FlagString, Desc: "--chat-query 的兼容别名", Hidden: true},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintMutuallyExclusive, Flags: []string{"group", "chat-query", "group-query"}},
	},
	Tips: []string{
		`dws chat +chat-bots --group <openConversationId>`,
		`dws chat +chat-bots --group "项目群"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupID, err := resolveStableOrNamedChat(rt)
		if err != nil {
			return err
		}
		data, err := rt.CallMCPData("bot", "list_group_bots", map[string]any{"openConversationId": groupID})
		if err != nil {
			return err
		}
		bots := chatBotsProject(data)
		return rt.Output(map[string]any{"count": len(bots), "bots": bots})
	},
}

// resolveStableOrNamedChat gives read-only group shortcuts one safe target
// contract. Stable cid values bypass search; natural names always go through
// the shared exact-match, full-pagination and ambiguity rules.
func resolveStableOrNamedChat(rt *shortcut.RuntimeContext) (string, error) {
	resolved, err := targetresolver.ResolveChatTarget(
		rt,
		strings.TrimSpace(rt.Str("group")),
		strings.TrimSpace(rt.StrFirst("chat-query", "group-query")),
	)
	if err != nil {
		return "", err
	}
	return resolved.Selected.OpenConversationID, nil
}

// chatBotsProject reshapes list_group_bots into a clean bot list
// ({openBotId, name}) — clean output projection. List container and
// per-item field names are probed defensively across candidate keys so shape
// drift yields an empty list rather than a crash or fabricated data.
func chatBotsProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openBotId", "open_bot_id", "botId", "robotCode", "id"); ok {
			row["openBotId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "botName", "nick", "title"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ChatRemoveBot removes a robot from a group (remove_robot_in_group, bot).
var ChatRemoveBot = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-remove-bot",
	Product:     "bot",
	Description: "从群内移除机器人",
	Intent:      "当你想把某个机器人从群里移除时使用；会实际移除机器人，不可逆，需传群 openConversationId 和机器人 openBotId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "id", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "bot-id", Type: shortcut.FlagString, Desc: "机器人 openBotId", Required: true},
	},
	Tips: []string{`dws chat +chat-remove-bot --id <openConversationId> --bot-id <openBotId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_robot_in_group", map[string]any{
			"openConversationId": rt.Str("id"),
			"openBotId":          rt.Str("bot-id"),
		})
	},
}

// ChatSetAdmin sets/unsets group admins (update_conv_member_roles, im).
var ChatSetAdmin = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-set-admin",
	Product:     "im",
	Description: "设置 / 取消群管理员",
	Intent:      "当你想把某些成员设为或取消群管理员时使用；会实际变更成员角色，需传群 openConversationId 和成员 userId/openDingTalkId 列表，加 --off 取消管理员。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_set_admin",
			CanonicalPath:  "chat.shortcut_chat_set_admin",
			CLIPath:        "chat +chat-set-admin",
			PrimaryCLIPath: "chat +chat-set-admin",
		},
		Description: "设置 / 取消群管理员",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "设置 / 取消群管理员",
			UseWhen:      []string{"当你想把某些成员设为或取消群管理员时使用；会实际变更成员角色，需传群 openConversationId 和成员 userId/openDingTalkId 列表，加 --off 取消管理员。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-set-admin --group <openConversationId> --users userId1,userId2"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "成员 userId 或 openDingTalkId 列表", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "取消管理员（不传则设为管理员）"},
	},
	Tips: []string{`dws chat +chat-set-admin --group <openConversationId> --users userId1,userId2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		userIDs, openIDs := splitIDs(rt.StrSlice("users"))
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"admin":              !rt.Bool("off"),
		}
		if len(userIDs) > 0 {
			params["uids"] = userIDs
		}
		if len(openIDs) > 0 {
			params["openDingTalkIds"] = openIDs
		}
		return rt.CallMCP("update_conv_member_roles", params)
	},
}

// ChatMute mutes/unmutes the whole group (set_group_mute, im).
var ChatMute = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-mute",
	Product:     "im",
	Description: "全员禁言 / 取消全员禁言",
	Intent:      "当你想对整个群开启或取消全员禁言时使用；会实际切换群的全员禁言状态，需传群 openConversationId，加 --off 取消禁言。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_mute",
			CanonicalPath:  "chat.shortcut_chat_mute",
			CLIPath:        "chat +chat-mute",
			PrimaryCLIPath: "chat +chat-mute",
		},
		Description: "全员禁言 / 取消全员禁言",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "全员禁言 / 取消全员禁言",
			UseWhen:      []string{"当你想对整个群开启或取消全员禁言时使用；会实际切换群的全员禁言状态，需传群 openConversationId，加 --off 取消禁言。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-mute --group <openConversationId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "off", Type: shortcut.FlagBool, Desc: "取消全员禁言（不传则开启禁言）"},
	},
	Tips: []string{`dws chat +chat-mute --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("set_group_mute", map[string]any{
			"openConversationId": rt.Str("group"),
			"mute":               !rt.Bool("off"),
		})
	},
}

// ChatMuteMember mutes/unmutes specific members (set_group_member_mute_list, im).
var ChatMuteMember = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-mute-member",
	Product:     "im",
	Description: "指定群成员禁言 / 取消禁言",
	Intent:      "当你想只禁言或解禁群里的指定成员时使用；会实际把成员加入或移出禁言名单，需传群 openConversationId 和成员列表，禁言时还需 --mute-time（毫秒）。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "users", Type: shortcut.FlagStringSlice, Desc: "成员 userId 或 openDingTalkId 列表", Required: true},
		{Name: "mute-time", Type: shortcut.FlagInt, Desc: "禁言时长（毫秒），如 300000/3600000/86400000/604800000/2592000000"},
		{Name: "off", Type: shortcut.FlagBool, Desc: "移出禁言名单（不传则加入禁言名单）"},
	},
	Tips: []string{`dws chat +chat-mute-member --group <openConversationId> --users userId1 --mute-time 3600000`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		userIDs, openIDs := splitIDs(rt.StrSlice("users"))
		off := rt.Bool("off")
		if len(userIDs) > 0 {
			resolved, err := resolveMuteMemberOpenIDs(rt, rt.Str("group"), userIDs)
			if err != nil {
				return err
			}
			openIDs = append(openIDs, resolved...)
			userIDs = nil
		}
		deduplicatedOpenIDs := make([]string, 0, len(openIDs))
		for _, openID := range openIDs {
			deduplicatedOpenIDs = appendUniqueShortcutString(deduplicatedOpenIDs, openID)
		}
		openIDs = deduplicatedOpenIDs
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"cid":                rt.Str("group"),
			"mute":               !off,
		}
		if len(userIDs) > 0 {
			params["uids"] = userIDs
		}
		if len(openIDs) > 0 {
			params["openDingTalkIds"] = openIDs
		}
		if !off {
			if rt.Int("mute-time") <= 0 {
				return fmt.Errorf("--mute-time 为禁言时必填（毫秒）")
			}
			params["muteTime"] = rt.Int("mute-time")
		}
		return rt.CallMCP("set_group_member_mute_list", params)
	},
}

// resolveMuteMemberOpenIDs mirrors the native group-mute-member compatibility
// path. The service currently rejects its documented uids input with
// "uids is required", while openDingTalkIds succeeds. Resolve userIds through
// the directory and the target group's member list so the Shortcut's advertised
// mixed-ID contract remains executable instead of forwarding a known-broken
// parameter shape.
func resolveMuteMemberOpenIDs(rt *shortcut.RuntimeContext, groupID string, userIDs []string) ([]string, error) {
	contacts, err := rt.CallMCPData("contact", "get_user_info_by_user_ids", map[string]any{
		"user_id_list": userIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("解析禁言成员 userId 失败: %w", err)
	}

	requested := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		requested[userID] = struct{}{}
	}
	nameByUserID := make(map[string]string, len(userIDs))
	for _, item := range shortcutMapSlice(contacts["result"]) {
		employee, _ := item["orgEmployeeModel"].(map[string]any)
		userID := shortcutString(employee, "orgUserId", "userId")
		if _, ok := requested[userID]; !ok {
			continue
		}
		nameByUserID[userID] = shortcutString(employee, "orgUserName", "name")
	}
	for _, userID := range userIDs {
		if strings.TrimSpace(nameByUserID[userID]) == "" {
			return nil, fmt.Errorf("无法从通讯录解析成员 userId %q 的姓名；请改传 openDingTalkId", userID)
		}
	}

	openIDsByName := map[string][]string{}
	cursor := "0"
	for page := 0; page < 50; page++ {
		members, callErr := rt.CallMCPData("chat", "get_group_members", map[string]any{
			"openconversation_id": groupID,
			"cursor":              cursor,
		})
		if callErr != nil {
			return nil, fmt.Errorf("读取目标群成员以解析 userId 失败: %w", callErr)
		}
		result, _ := members["result"].(map[string]any)
		for _, member := range shortcutMapSlice(result["list"]) {
			name := shortcutString(member, "memberEmpName", "empName", "name")
			openID := shortcutString(member, "openDingtalkId", "openDingTalkId", "memberDingtalkId")
			if name == "" || openID == "" {
				continue
			}
			openIDsByName[name] = appendUniqueShortcutString(openIDsByName[name], openID)
		}
		hasMore, _ := result["hasMore"].(bool)
		if !hasMore {
			break
		}
		next := shortcutString(result, "nextCursor", "cursor")
		if next == "" || next == cursor {
			return nil, fmt.Errorf("群成员分页返回 hasMore=true 但缺少可继续的 cursor")
		}
		cursor = next
		if page == 49 {
			return nil, fmt.Errorf("群成员分页超过 50 页，无法安全解析禁言目标")
		}
	}

	resolved := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		name := nameByUserID[userID]
		matches := openIDsByName[name]
		if len(matches) != 1 {
			return nil, fmt.Errorf(
				"群内姓名 %q 对应 %d 个成员，无法把 userId %q 唯一解析为 openDingTalkId；请直接传 openDingTalkId",
				name, len(matches), userID)
		}
		resolved = append(resolved, matches[0])
	}
	return resolved, nil
}

func shortcutMapSlice(value any) []map[string]any {
	items, _ := value.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			out = append(out, mapped)
		}
	}
	return out
}

func shortcutString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		switch typed := value[key].(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				return text
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		}
	}
	return ""
}

func appendUniqueShortcutString(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

// ── group-role: 群身份管理 (im) ──────────────────────────────

// ChatRoleList lists custom group roles (list_custom_group_roles, im).
var ChatRoleList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-list",
	Product:     "im",
	Description: "拉取会话的群身份列表",
	Intent:      "当你想查看某群自定义的群身份（如'班长''值日'）都有哪些时使用；需传群 openConversationId，只读返回群身份列表及 openRoleId。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_role_list",
			CanonicalPath:  "chat.shortcut_chat_role_list",
			CLIPath:        "chat +chat-role-list",
			PrimaryCLIPath: "chat +chat-role-list",
		},
		Description: "拉取会话的群身份列表",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "拉取会话的群身份列表",
			UseWhen:      []string{"当你想查看某群自定义的群身份（如'班长''值日'）都有哪些时使用；需传群 openConversationId，只读返回群身份列表及 openRoleId。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-role-list --group <openConversationId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
	},
	Tips: []string{`dws chat +chat-role-list --group <openConversationId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData("im", "list_custom_group_roles", map[string]any{"openConversationId": rt.Str("group")})
		if err != nil {
			return err
		}
		roles := chatRoleListProject(data)
		return rt.Output(map[string]any{"count": len(roles), "roles": roles})
	},
}

// chatRoleListProject reshapes list_custom_group_roles into a clean group-role
// list ({openRoleId, name}) — clean output projection. List
// container and per-item field names are probed defensively across candidate
// keys so shape drift yields an empty list rather than a crash or fabricated
// data.
func chatRoleListProject(data map[string]any) []map[string]any {
	raw := chatGroupResolveList(data)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{}
		if v, ok := chatGroupFirst(m, "openRoleId", "open_role_id", "roleId", "id"); ok {
			row["openRoleId"] = v
		}
		if v, ok := chatGroupFirst(m, "name", "roleName", "title"); ok {
			row["name"] = v
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// ChatRoleAdd adds a custom group role (add_custom_group_role, im).
var ChatRoleAdd = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-add",
	Product:     "im",
	Description: "添加群身份",
	Intent:      "当你想在群里新增一个自定义群身份/头衔时使用；会实际创建群身份，需传群 openConversationId 和身份名称。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_role_add",
			CanonicalPath:  "chat.shortcut_chat_role_add",
			CLIPath:        "chat +chat-role-add",
			PrimaryCLIPath: "chat +chat-role-add",
		},
		Description: "添加群身份",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "添加群身份",
			UseWhen:      []string{"当你想在群里新增一个自定义群身份/头衔时使用；会实际创建群身份，需传群 openConversationId 和身份名称。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-role-add --group <openConversationId> --name \"管理员\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "群身份名称", Required: true},
	},
	Tips: []string{`dws chat +chat-role-add --group <openConversationId> --name "管理员"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("add_custom_group_role", map[string]any{
			"openConversationId": rt.Str("group"),
			"name":               rt.Str("name"),
		})
	},
}

// ChatRoleUpdate renames a custom group role (update_custom_group_role, im).
var ChatRoleUpdate = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-update",
	Product:     "im",
	Description: "更新群身份名称",
	Intent:      "当你想重命名已有的群身份时使用；会实际更新身份名称，需传群 openConversationId、身份 openRoleId 和新名称。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_role_update",
			CanonicalPath:  "chat.shortcut_chat_role_update",
			CLIPath:        "chat +chat-role-update",
			PrimaryCLIPath: "chat +chat-role-update",
		},
		Description: "更新群身份名称",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "更新群身份名称",
			UseWhen:      []string{"当你想重命名已有的群身份时使用；会实际更新身份名称，需传群 openConversationId、身份 openRoleId 和新名称。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-role-update --group <openConversationId> --role-id <openRoleId> --name \"新名称\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "群身份 openRoleId", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "群身份新名称", Required: true},
	},
	Tips: []string{`dws chat +chat-role-update --group <openConversationId> --role-id <openRoleId> --name "新名称"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_custom_group_role", map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleId":         rt.Str("role-id"),
			"name":               rt.Str("name"),
		})
	},
}

// ChatRoleRemove deletes a custom group role (remove_custom_group_role, im).
var ChatRoleRemove = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-remove",
	Product:     "im",
	Description: "删除群身份",
	Intent:      "当你想删除某个自定义群身份时使用；会实际删除群身份，不可逆，需传群 openConversationId 和身份 openRoleId。",
	Risk:        shortcut.RiskHighWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "role-id", Type: shortcut.FlagString, Desc: "群身份 openRoleId", Required: true},
	},
	Tips: []string{`dws chat +chat-role-remove --group <openConversationId> --role-id <openRoleId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("remove_custom_group_role", map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleId":         rt.Str("role-id"),
		})
	},
}

// ChatRoleSetUser overwrites a user's group roles (set_custom_user_roles, im).
var ChatRoleSetUser = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-set-user",
	Product:     "im",
	Description: "设置用户的群身份（覆盖该用户的全部群身份）",
	Intent:      "当你想为某成员整体设定其在群内的身份时使用；会实际改写该用户的群身份集合（覆盖其原有全部身份），需传群、用户和 openRoleId 列表（传空则清除全部）。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_role_set_user",
			CanonicalPath:  "chat.shortcut_chat_role_set_user",
			CLIPath:        "chat +chat-role-set-user",
			PrimaryCLIPath: "chat +chat-role-set-user",
		},
		Description: "设置用户的群身份（覆盖该用户的全部群身份）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "设置用户的群身份（覆盖该用户的全部群身份）",
			UseWhen:      []string{"当你想为某成员整体设定其在群内的身份时使用；会实际改写该用户的群身份集合（覆盖其原有全部身份），需传群、用户和 openRoleId 列表（传空则清除全部）。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-role-set-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "用户 userId 或 openDingTalkId", Required: true},
		{Name: "role-ids", Type: shortcut.FlagStringSlice, Desc: "群身份 openRoleId 列表（空则清除全部）", Required: true},
	},
	Tips: []string{`dws chat +chat-role-set-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		user := rt.Str("user")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleIds":        rt.StrSlice("role-ids"),
		}
		if isOpenID(user) {
			params["openDingTalkId"] = user
		} else {
			params["userId"] = user
		}
		return rt.CallMCP("set_custom_user_roles", params)
	},
}

// ChatRoleRemoveUser removes specific roles from a user (remove_custom_user_roles, im).
var ChatRoleRemoveUser = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-remove-user",
	Product:     "im",
	Description: "移除用户的指定群身份",
	Intent:      "当你只想撤销某成员的部分群身份、保留其余时使用；会实际移除指定的群身份，需传群、用户和要移除的 openRoleId 列表。",
	Risk:        shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "用户 userId 或 openDingTalkId", Required: true},
		{Name: "role-ids", Type: shortcut.FlagStringSlice, Desc: "要移除的群身份 openRoleId 列表", Required: true},
	},
	Tips: []string{`dws chat +chat-role-remove-user --group <openConversationId> --user <userId> --role-ids roleId1`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		user := rt.Str("user")
		params := map[string]any{
			"openConversationId": rt.Str("group"),
			"openRoleIds":        rt.StrSlice("role-ids"),
		}
		if isOpenID(user) {
			params["openDingTalkId"] = user
		} else {
			params["userId"] = user
		}
		return rt.CallMCP("remove_custom_user_roles", params)
	},
}

// ChatRoleQueryUser queries a member's group roles (query_custom_user_roles, im).
var ChatRoleQueryUser = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-role-query-user",
	Product:     "im",
	Description: "查询群成员的群身份",
	Intent:      "当你想查看某个群成员当前拥有哪些群身份时使用；只读，需传群 openConversationId 和用户 userId 或 openDingTalkId。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_role_query_user",
			CanonicalPath:  "chat.shortcut_chat_role_query_user",
			CLIPath:        "chat +chat-role-query-user",
			PrimaryCLIPath: "chat +chat-role-query-user",
		},
		Description: "查询群成员的群身份",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "查询群成员的群身份",
			UseWhen:      []string{"当你想查看某个群成员当前拥有哪些群身份时使用；只读，需传群 openConversationId 和用户 userId 或 openDingTalkId。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +chat-role-query-user --group <openConversationId> --user <userId>"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群 openConversationId", Required: true},
		{Name: "user", Type: shortcut.FlagString, Desc: "用户 userId 或 openDingTalkId", Required: true},
	},
	Tips: []string{`dws chat +chat-role-query-user --group <openConversationId> --user <userId>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		user := rt.Str("user")
		params := map[string]any{"openConversationId": rt.Str("group")}
		if isOpenID(user) {
			params["openDingTalkId"] = user
		} else {
			params["userId"] = user
		}
		return rt.CallMCP("query_custom_user_roles", params)
	},
}

func init() {
	shortcut.Register(withReviewedChatShortcutContracts(
		ChatSearch,
		ChatMembersGet,
		ChatTransferOwner,
		ChatInviteURL,
		ChatQuit,
		ChatUpdateIcon,
		ChatUpdateSettings,
		ChatDismiss,
		ChatSetHistory,
		ChatUpdateNick,
		ChatUpdateAlias,
		ChatListMine,
		ChatListAll,
		ChatListJoinRequests,
		ChatAuditJoin,
		ChatGetByID,
		ChatAddBot,
		ChatBots,
		ChatRemoveBot,
		ChatSetAdmin,
		ChatMute,
		ChatMuteMember,
		ChatRoleList,
		ChatRoleAdd,
		ChatRoleUpdate,
		ChatRoleRemove,
		ChatRoleSetUser,
		ChatRoleRemoveUser,
		ChatRoleQueryUser,
	)...)
}

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
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

const (
	groupMembersDefaultPageLimit = 50
	groupMembersHardPageLimit    = 500
	groupMembersContractVersion  = "im.group-members.v1"
)

// GroupMembers: list a group's members by its NAME, no openConversationId juggling.
//
// Steps: search groups by name → resolve to a single openConversationId
// (disambiguate on multiple matches, never guess) → list that group's members.
// Replaces `chat search --query <群名>` (copy openConversationId) →
// `chat group members --id <openConversationId>`.
//
// Note: the group lookup uses `search_groups` (im server, keyword search over
// group NAMES) — NOT `search_common_groups`, which searches by member
// nicknames and cannot locate a group by its title.
//
//	dws chat +group-members --group 项目冲刺
var GroupMembers = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+group-members",
	Product:     "chat",
	Description: "按群名唯一解析后全量列出用户成员并公开分页完整性",
	Intent: "当你只知道群的名字、想看看这个群里有哪些成员，而不想先手动查群 ID 时使用；" +
		"内部先按群名搜索群聊解析出唯一 openConversationId，再拉取该群的成员列表。" +
		"群名匹配到多个群时会列出候选让你区分、绝不自行假定。用户成员会自动翻页、稳定 ID 去重，并公开 complete/hasMore/nextCursor；--page-limit 保证有界。只读，不改动任何数据。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_group_members",
			CanonicalPath:  "chat.shortcut_group_members",
			CLIPath:        "chat +group-members",
			PrimaryCLIPath: "chat +group-members",
		},
		Description: "按群名唯一解析后全量列出用户成员并公开分页完整性",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按群名唯一解析后全量列出用户成员并公开分页完整性",
			UseWhen:      []string{"当你只知道群的名字、想看看这个群里有哪些成员，而不想先手动查群 ID 时使用；内部先按群名搜索群聊解析出唯一 openConversationId，再拉取该群的成员列表。群名匹配到多个群时会列出候选让你区分、绝不自行假定。用户成员会自动翻页、稳定 ID 去重，并公开 complete/hasMore/nextCursor；--page-limit 保证有界。只读，不改动任何数据。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +group-members --group 项目冲刺"},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称（搜群关键词，用群名里连续的核心词）", Required: true},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "最大用户成员页数；--page-limit 必须在 1-500 之间"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须在 1-500 之间"},
	},
	Tips:     []string{`dws chat +group-members --group 项目冲刺`},
	Validate: validateGroupMembersPageLimit,
	Execute: func(rt *shortcut.RuntimeContext) error {
		resolved, err := targetresolver.ResolveChat(rt, rt.Str("group"))
		if err != nil {
			return err
		}
		result, err := collectGroupUserMembers(rt, resolved.Selected.OpenConversationID, rt.Int("page-limit"))
		if err != nil {
			return err
		}
		return rt.Output(result.payload(resolved.Selected.OpenConversationID))
	},
}

// ChatMembersList aligns the recent lark-cli IM member-list experience with
// DingTalk's two lower tools: user members come from get_group_members and bots
// come from list_group_bots. Callers may provide a stable conversation ID or a
// human group name; name resolution remains ambiguity-safe.
var ChatMembersList = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+chat-members-list",
	Aliases:     []string{"+chat-group-members"},
	Product:     "chat",
	Description: "列出群成员并把用户与机器人分桶（支持群名语义解析）",
	Intent: "当你要完整查看一个群的参与者，并需要区分真人用户和机器人时使用；" +
		"--group 可传群名或 openConversationId，也可用 --conversation-id 显式传稳定 ID、用 --chat-query 显式按群名解析。" +
		"默认同时返回 users/bots 两个桶，也可用 --member-types 只取 user 或 bot；" +
		"用户桶自动翻页并按稳定 ID 去重，结果用 buckets、complete、hasMore、nextCursor 和 failures 证明完整性；--page-limit 保证有界。机器人桶按下层全量列表投影。",
	Risk: shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low",
		Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_chat_members_list",
			CanonicalPath:  "chat.shortcut_chat_members_list",
			CLIPath:        "chat +chat-members-list",
			PrimaryCLIPath: "chat +chat-members-list",
			Aliases:        []string{"chat +chat-group-members"},
		},
		Description: "列出群成员并把用户与机器人分桶（支持群名语义解析）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed composite member adapter: the executable CLI safely resolves a group, paginates and deduplicates user members, lists bots, and publishes per-bucket completeness and failures.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "列出群成员并把用户与机器人分桶（支持群名语义解析）",
			UseWhen: []string{"当你要完整查看一个群的参与者，并需要区分真人用户和机器人时使用；" +
				"--group 可传群名或 openConversationId，也可用 --conversation-id 显式传稳定 ID、用 --chat-query 显式按群名解析。" +
				"默认同时返回 users/bots 两个桶，也可用 --member-types 只取 user 或 bot；" +
				"用户桶自动翻页并按稳定 ID 去重，结果用 buckets、complete、hasMore、nextCursor 和 failures 证明完整性；--page-limit 保证有界。机器人桶按下层全量列表投影。"},
			AvoidWhen: []string{"只需要用户成员且已有群名时可使用 +group-members；需要原始单页响应时使用底层原子命令"},
			Examples: []string{
				"dws chat +chat-members-list --group \"项目冲刺\"",
				"dws chat +chat-members-list --conversation-id <openConversationId> --member-types user,bot",
			},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称或 openConversationId"},
		{Name: "conversation-id", Type: shortcut.FlagString, Desc: "显式群 openConversationId"},
		{Name: "chat-query", Type: shortcut.FlagString, Desc: "按群名解析唯一 openConversationId"},
		{Name: "chat", Type: shortcut.FlagString, Desc: "--conversation-id 的兼容别名", Hidden: true},
		{Name: "open-conversation-id", Type: shortcut.FlagString, Desc: "--conversation-id 的兼容别名", Hidden: true},
		{Name: "member-types", Type: shortcut.FlagStringSlice, Desc: "成员类型；--member-types 仅接受 user/bot；不传则同时返回"},
		{Name: "page-limit", Type: shortcut.FlagInt, Default: "50", Desc: "用户成员桶最大页数；--page-limit 必须在 1-500 之间"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"group", "conversation-id", "chat-query", "chat", "open-conversation-id"}},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"member-types"}, Description: "--member-types 仅接受 user/bot"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"page-limit"}, Description: "--page-limit 必须在 1-500 之间"},
	},
	Tips: []string{
		`dws chat +chat-members-list --group "项目冲刺"`,
		`dws chat +chat-members-list --conversation-id <openConversationId> --member-types user,bot`,
	},
	Validate: validateGroupMembersPageLimit,
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupID := strings.TrimSpace(rt.StrFirst("conversation-id", "chat", "open-conversation-id"))
		if groupID == "" {
			resolved, err := targetresolver.ResolveChatTarget(rt, rt.Str("group"), rt.Str("chat-query"))
			if err != nil {
				return err
			}
			groupID = resolved.Selected.OpenConversationID
		}

		wantUsers, wantBots, err := resolveMemberTypes(rt.StrSlice("member-types"))
		if err != nil {
			return err
		}

		payload := map[string]any{
			"contractVersion": groupMembersContractVersion,
			"conversationId":  groupID,
			"users":           []map[string]any{},
			"bots":            []map[string]any{},
		}
		buckets := map[string]any{}
		var userErr, botErr error
		var userResult groupUserMembersResult
		if wantUsers {
			userResult, userErr = collectGroupUserMembers(rt, groupID, rt.Int("page-limit"))
			if userErr == nil {
				payload["users"] = userResult.members
				buckets["users"] = userResult.bucketPayload()
			}
		}
		if wantBots {
			var data map[string]any
			data, botErr = rt.CallMCPData("bot", "list_group_bots", map[string]any{
				"openConversationId": groupID,
			})
			if botErr == nil {
				bots := groupBotProject(data)
				payload["bots"] = bots
				buckets["bots"] = map[string]any{
					"complete": true,
					"count":    len(bots),
				}
			}
		}
		if wantUsers && userErr != nil && !wantBots {
			return userErr
		}
		if wantBots && botErr != nil && !wantUsers {
			return botErr
		}
		if userErr != nil && botErr != nil {
			return fmt.Errorf("读取用户成员失败: %v；读取机器人失败: %v", userErr, botErr)
		}
		failures := make([]map[string]any, 0, 1+len(userResult.failures))
		failures = append(failures, userResult.failures...)
		if userErr != nil {
			failures = append(failures, map[string]any{"bucket": "users", "stage": "read", "error": userErr.Error()})
		}
		if botErr != nil {
			failures = append(failures, map[string]any{"bucket": "bots", "stage": "read", "error": botErr.Error()})
		}
		if len(failures) > 0 {
			payload["errors"] = failures
			payload["partial"] = true
		} else {
			payload["partial"] = false
		}

		users, _ := payload["users"].([]map[string]any)
		bots, _ := payload["bots"].([]map[string]any)
		payload["counts"] = map[string]any{
			"users": len(users),
			"bots":  len(bots),
			"total": len(users) + len(bots),
		}
		payload["buckets"] = buckets
		payload["failedCount"] = len(failures)
		payload["failures"] = failures
		payload["complete"] = len(failures) == 0 && (!wantUsers || userResult.complete)
		payload["hasMore"] = wantUsers && userResult.hasMore
		if wantUsers && userResult.nextCursor != "" {
			payload["nextCursor"] = userResult.nextCursor
		}
		return rt.Output(payload)
	},
}

func validateGroupMembersPageLimit(rt *shortcut.RuntimeContext) error {
	if limit := rt.Int("page-limit"); limit < 1 || limit > groupMembersHardPageLimit {
		return apperrors.NewValidation("--page-limit 必须在 1-500 之间")
	}
	return nil
}

type groupUserMembersResult struct {
	members      []map[string]any
	pagesFetched int
	complete     bool
	hasMore      bool
	nextCursor   string
	stopReason   string
	failures     []map[string]any
}

func collectGroupUserMembers(rt *shortcut.RuntimeContext, groupID string, pageLimit int) (groupUserMembersResult, error) {
	pageLimit = defaultChatPageLimit(pageLimit, groupMembersDefaultPageLimit)
	result := groupUserMembersResult{
		members:    []map[string]any{},
		failures:   []map[string]any{},
		stopReason: "source_complete",
	}
	cursor := "0"
	seenCursors := map[string]bool{cursor: true}
	seenIDs := map[string]bool{}
	for result.pagesFetched < pageLimit {
		data, err := rt.CallMCPData("chat", "get_group_members", map[string]any{
			"openconversation_id": groupID,
			"cursor":              cursor,
		})
		if err != nil {
			if result.pagesFetched > 0 {
				result.failures = append(result.failures, map[string]any{
					"bucket": "users",
					"page":   result.pagesFetched + 1,
					"stage":  "read",
					"error":  err.Error(),
				})
				result.stopReason = "read_failure"
				return result, nil
			}
			return result, err
		}
		result.pagesFetched++
		for _, member := range groupMemberProject(data) {
			stableID := strings.TrimSpace(fmt.Sprint(member["openDingtalkId"]))
			if stableID != "" && stableID != "<nil>" && seenIDs[stableID] {
				continue
			}
			if stableID != "" && stableID != "<nil>" {
				seenIDs[stableID] = true
			}
			result.members = append(result.members, member)
		}
		page := chatmsg.Pagination(data)
		hasMore, known := page["hasMore"].(bool)
		if !known {
			result.failures = append(result.failures, map[string]any{
				"bucket": "users",
				"page":   result.pagesFetched,
				"stage":  "pagination",
				"error":  "群成员下层未返回可靠的 hasMore，无法证明用户桶完整",
			})
			result.stopReason = "pagination_error"
			return result, nil
		}
		result.hasMore = hasMore
		if !hasMore {
			result.complete = true
			result.stopReason = "source_complete"
			return result, nil
		}
		nextCursor := strings.TrimSpace(fmt.Sprint(page["nextCursor"]))
		if nextCursor == "" || nextCursor == "<nil>" || seenCursors[nextCursor] {
			result.failures = append(result.failures, map[string]any{
				"bucket": "users",
				"page":   result.pagesFetched,
				"stage":  "pagination",
				"error":  "群成员 hasMore=true 但 nextCursor 缺失或停滞",
			})
			result.stopReason = "pagination_error"
			return result, nil
		}
		result.nextCursor = nextCursor
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
	result.stopReason = "page_limit"
	return result, nil
}

func (result groupUserMembersResult) bucketPayload() map[string]any {
	payload := map[string]any{
		"count":        len(result.members),
		"pagesFetched": result.pagesFetched,
		"complete":     result.complete && len(result.failures) == 0,
		"hasMore":      result.hasMore,
		"partial":      len(result.failures) > 0 && len(result.members) > 0,
		"stopReason":   result.stopReason,
		"failedCount":  len(result.failures),
		"failures":     result.failures,
	}
	if result.nextCursor != "" {
		payload["nextCursor"] = result.nextCursor
	}
	return payload
}

func (result groupUserMembersResult) payload(groupID string) map[string]any {
	payload := result.bucketPayload()
	payload["contractVersion"] = groupMembersContractVersion
	payload["conversationId"] = groupID
	payload["members"] = result.members
	return payload
}

func resolveMemberTypes(raw []string) (bool, bool, error) {
	if len(raw) == 0 {
		return true, true, nil
	}
	var users, bots bool
	for _, value := range raw {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "user", "users":
			users = true
		case "bot", "bots":
			bots = true
		case "":
			continue
		default:
			return false, false, apperrors.NewValidation(
				fmt.Sprintf("--member-types 只支持 user,bot，收到 %q", value))
		}
	}
	if !users && !bots {
		return false, false, apperrors.NewValidation("--member-types 至少包含 user 或 bot")
	}
	return users, bots, nil
}

func groupBotProject(data map[string]any) []map[string]any {
	var items []any
	for _, root := range []map[string]any{data, groupMemberChildMap(data, "result")} {
		if root == nil {
			continue
		}
		for _, key := range []string{"bots", "robots", "list", "items"} {
			if value, ok := root[key].([]any); ok {
				items = value
				break
			}
		}
		if items != nil {
			break
		}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		bot, ok := item.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{
			"name":           groupMemberFirst(bot, "name", "robotName", "botName"),
			"openDingtalkId": groupMemberFirst(bot, "botOpenDingTalkId", "openDingTalkId"),
			"openBotId":      groupMemberFirst(bot, "openBotId", "botId"),
			"robotCode":      groupMemberFirst(bot, "robotCode", "code"),
		}
		out = append(out, row)
	}
	return out
}

// groupMemberProject reshapes a get_group_members response into a clean
// {name, nick, role, openDingtalkId} list. The real payload wraps members in
// result.list[]; probe a few container shapes defensively.
func groupMemberProject(data map[string]any) []map[string]any {
	var items []any
	for _, root := range []map[string]any{data, groupMemberChildMap(data, "result")} {
		if root == nil {
			continue
		}
		for _, key := range []string{"list", "members", "memberList", "items", "records", "data"} {
			if arr, ok := root[key].([]any); ok {
				items = arr
				break
			}
		}
		if items != nil {
			break
		}
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := map[string]any{
			"name":           groupMemberFirst(m, "memberEmpName", "empName", "name", "userName", "staffName"),
			"nick":           groupMemberFirst(m, "memberNick", "nick", "groupNick", "memberGroupNick"),
			"role":           groupMemberFirst(m, "memberRoleDesc", "roleDesc", "role"),
			"openDingtalkId": groupMemberFirst(m, "openDingtalkId", "openDingTalkId", "memberDingtalkId"),
		}
		out = append(out, row)
	}
	return out
}

func groupMemberChildMap(data map[string]any, key string) map[string]any {
	if m, ok := data[key].(map[string]any); ok {
		return m
	}
	return nil
}

// groupMemberFirst returns the first non-empty string among the candidate keys.
func groupMemberFirst(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func init() {
	shortcut.Register(GroupMembers, ChatMembersList)
}

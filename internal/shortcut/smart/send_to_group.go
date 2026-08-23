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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

// SendToGroup: message a group by its name or stable openConversationId.
//
// Steps: normalize a stable ID, or search a name and resolve it to a single
// openConversationId (disambiguate on multiple matches, never guess), then
// send a markdown message.
// Replaces `chat search --query <群名>` (copy openConversationId) →
// `chat +messages-send --group <openConversationId>`.
//
// Note: the group lookup uses `search_groups` (im server, keyword search over
// group NAMES) — NOT `search_common_groups`, which searches by member nicknames
// and cannot locate a group by its title.
//
//	dws chat +send-to-group --group 项目冲刺 --content "今天 5 点前提交进度"
var SendToGroup = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+send-to-group",
	Product:     "chat",
	Description: "按群名或 openConversationId 直接给群发消息",
	Intent:      "当你有群名或 openConversationId、想直接往该群发送简单文本或 Markdown 时使用；稳定 ID 不进入搜索，群名则必须唯一解析，零命中或多候选都会在发送前停止。会真实发出群消息。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_send_to_group",
			CanonicalPath:  "chat.shortcut_send_to_group",
			CLIPath:        "chat +send-to-group",
			PrimaryCLIPath: "chat +send-to-group",
		},
		Description: "按群名或 openConversationId 直接给群发消息",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按群名或 openConversationId 直接给群发消息",
			UseWhen:      []string{"当你有群名或 openConversationId、想直接往该群发送简单文本或 Markdown 时使用；稳定 ID 不进入搜索，群名则必须唯一解析，零命中或多候选都会在发送前停止。会真实发出群消息。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +send-to-group --group 项目冲刺 --content \"今天 5 点前提交进度\""},
		},
		Parameters: []contract.ParamDecl{renamedRequiredParam("content", "text")},
	},
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称或 openConversationId", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "消息内容（支持 Markdown）", Required: true, Aliases: []string{"text"}},
		shortcut.AIMessageTagFlag(),
	},
	Tips: []string{`dws chat +send-to-group --group 项目冲刺 --content "今天 5 点前提交进度"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupName := rt.Str("group")
		text := rt.StrFirst("text", "content")

		resolved, err := targetresolver.ResolveChatTarget(rt, groupName, "")
		if err != nil {
			return err
		}
		return chatshortcut.ExecuteResolvedUserMarkdown(
			rt,
			chatshortcut.ResolvedUserMessageTarget{
				GroupID: resolved.Selected.OpenConversationID,
			},
			text,
		)
	},
}

// sendGroupMatch is a single group candidate resolved from a name search.
type sendGroupMatch struct {
	id   string
	name string
}

// extractGroupsForSend pulls {openConversationId, title} out of a search_groups
// response. The result may be a bare list, or nested under
// result/result.items/result.groups (field names per chat search's real
// response shape), and the name field may be "title" or "name".
func extractGroupsForSend(data map[string]any) []sendGroupMatch {
	chats := targetresolver.ExtractChats(data)
	out := make([]sendGroupMatch, 0, len(chats))
	for _, chat := range chats {
		out = append(out, sendGroupMatch{id: chat.OpenConversationID, name: chat.Name})
	}
	return out
}

// preferExactGroupMatches keeps name-based routing ambiguity-safe while
// avoiding a common false ambiguity from substring search. If the server
// returns exactly one group whose title equals the requested name, that exact
// group wins over prefix/suffix matches. Duplicate rows for the same
// openConversationId are collapsed before selection.
func preferExactGroupMatches(groups []sendGroupMatch, query string) []sendGroupMatch {
	chats := make([]targetresolver.Chat, 0, len(groups))
	for _, group := range groups {
		chats = append(chats, targetresolver.Chat{OpenConversationID: group.id, Name: group.name})
	}
	selected := targetresolver.PreferExactChats(chats, query)
	out := make([]sendGroupMatch, 0, len(selected))
	for _, chat := range selected {
		out = append(out, sendGroupMatch{id: chat.OpenConversationID, name: chat.Name})
	}
	return out
}

func sendGroupLabels(groups []sendGroupMatch) []string {
	chats := make([]targetresolver.Chat, 0, len(groups))
	for _, group := range groups {
		chats = append(chats, targetresolver.Chat{OpenConversationID: group.id, Name: group.name})
	}
	return targetresolver.ChatLabels(chats)
}

func init() {
	shortcut.Register(SendToGroup)
}

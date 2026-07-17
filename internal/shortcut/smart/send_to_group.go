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
	"encoding/json"
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// SendToGroup: message a group by its NAME, no openConversationId juggling.
//
// Steps: search groups by name → resolve to a single openConversationId
// (disambiguate on multiple matches, never guess) → send a markdown message.
// Replaces `chat search --query <群名>` (copy openConversationId) →
// `chat +messages-send --group <openConversationId>`.
//
// Note: the group lookup uses `search_groups` (im server, keyword search over
// group NAMES) — NOT `search_common_groups`, which searches by member nicknames
// and cannot locate a group by its title.
//
//	dws chat +send-to-group --group 项目冲刺 --text "今天 5 点前提交进度"
var SendToGroup = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+send-to-group",
	Product:     "chat",
	Description: "按群名直接给群发消息（自动搜群解析 openConversationId）",
	Intent: "当你只知道群的名字、想直接往这个群里发一条消息而不想先手动查群 ID 时使用；" +
		"内部先按群名搜索群聊解析出唯一 openConversationId 再发送，群名匹配到多个群时会列出候选让你区分、绝不自行假定。会真实发出群消息。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称（搜群关键词，用群名里连续的核心词）", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "消息内容（支持 Markdown）", Required: true},
		shortcut.AIMessageTagFlag(),
	},
	Tips: []string{`dws chat +send-to-group --group 项目冲刺 --text "今天 5 点前提交进度"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupName := rt.Str("group")
		text := rt.Str("text")

		// Step 1 — search groups by name (keyword) on the im server.
		data, err := rt.CallMCPData("im", "search_groups", map[string]any{
			"keyword": groupName,
			"limit":   10,
			"cursor":  "0",
		})
		if err != nil {
			return err
		}
		groups := extractGroupsForSend(data)
		switch {
		case len(groups) == 0:
			return apperrors.NewValidation(fmt.Sprintf(
				"没找到名字匹配 %q 的群；换用群名里连续的核心词再试。", groupName))
		case len(groups) > 1:
			return apperrors.NewValidation(fmt.Sprintf(
				"%q 匹配到 %d 个群：%s。请用更精确的群名，或直接用 dws chat +messages-send --group <openConversationId> 指定群。",
				groupName, len(groups), strings.Join(sendGroupLabels(groups), "、")))
		}

		// Step 2 — send the markdown message to the unique group.
		content, _ := json.Marshal(map[string]string{"title": text, "text": text})
		return rt.CallMCP("send_personal_message", rt.AddAIMessageTag(map[string]any{
			"openConversationId": groups[0].id,
			"msgType":            "markdown",
			"content":            string(content),
		}))
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
	var list []any
	switch inner := data["result"].(type) {
	case []any:
		list = inner
	case map[string]any:
		if v, ok := inner["items"].([]any); ok {
			list = v
		} else if v, ok := inner["groups"].([]any); ok {
			list = v
		}
	}
	if list == nil {
		if v, ok := data["items"].([]any); ok {
			list = v
		}
	}

	var out []sendGroupMatch
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["openConversationId"].(string)
		if id == "" {
			id, _ = m["id"].(string)
		}
		if id == "" {
			continue
		}
		name, _ := m["title"].(string)
		if name == "" {
			name, _ = m["name"].(string)
		}
		out = append(out, sendGroupMatch{id: id, name: name})
	}
	return out
}

func sendGroupLabels(groups []sendGroupMatch) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		name := g.name
		if name == "" {
			name = "(未命名群)"
		}
		out = append(out, fmt.Sprintf("%s(%s)", name, g.id))
	}
	return out
}

func init() {
	shortcut.Register(SendToGroup)
}

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

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
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
	Description: "按群名列出群成员（自动搜群解析 openConversationId）",
	Intent: "当你只知道群的名字、想看看这个群里有哪些成员，而不想先手动查群 ID 时使用；" +
		"内部先按群名搜索群聊解析出唯一 openConversationId，再拉取该群的成员列表。" +
		"群名匹配到多个群时会列出候选让你区分、绝不自行假定。只读，不改动任何数据。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "group", Type: shortcut.FlagString, Desc: "群名称（搜群关键词，用群名里连续的核心词）", Required: true},
	},
	Tips: []string{`dws chat +group-members --group 项目冲刺`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		groupName := rt.Str("group")

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
				"%q 匹配到 %d 个群：%s。请用更精确的群名，或直接用 dws chat group members --id <openConversationId> 指定群。",
				groupName, len(groups), strings.Join(sendGroupLabels(groups), "、")))
		}

		// Step 2 — list the members of the unique group. The param key
		// (openconversation_id) is copied verbatim from chat.go's
		// get_group_members call site. Project the verbose raw member records
		// (memberAvatarMediaId, memberDingtalkId, …) down to the useful fields.
		mdata, err := rt.CallMCPData("chat", "get_group_members", map[string]any{
			"openconversation_id": groups[0].id,
		})
		if err != nil {
			return err
		}
		members := groupMemberProject(mdata)
		if len(members) == 0 {
			// Unrecognised shape — fall back to the raw payload rather than
			// hiding data.
			return rt.Output(mdata)
		}
		return rt.Output(map[string]any{"count": len(members), "members": members})
	},
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
	shortcut.Register(GroupMembers)
}

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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// Lookup: resolve a person by NAME and return their full profile in one step.
//
// Steps: search contacts by name → resolve to a single userId (disambiguate on
// multiple matches) → fetch full detail. Replaces `contact +search-user` (copy
// userId) → `contact +get-user --ids <id>`.
//
//	dws contact +lookup --name 张三
var Lookup = shortcut.Shortcut{
	Service:     "contact",
	Command:     "+lookup",
	Product:     "contact",
	Description: "按姓名查询某人的完整资料（自动解析 userId 后取详情）",
	Intent: "当你只知道对方姓名、想一步拿到其完整资料（部门、职位、联系方式等）而不想先搜 userId 再查详情时使用；" +
		"内部先按姓名搜通讯录解析出唯一 userId，再取详情，姓名匹配到多人时会列出候选让你区分。只读操作。",
	Risk: shortcut.RiskRead,
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "姓名/花名", Required: true},
	},
	Tips: []string{`dws contact +lookup --name 张三`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		// Step 1 — resolve the name to a unique userId.
		user, err := resolveUser(rt, rt.Str("name"))
		if err != nil {
			return err
		}

		// Step 2 — fetch and print the full profile of the resolved user.
		return rt.CallMCP("get_user_info_by_user_ids", map[string]any{
			"user_id_list": []string{user.userID},
		})
	},
}

func init() {
	shortcut.Register(Lookup)
}

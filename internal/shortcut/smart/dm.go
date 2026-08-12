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

// Package smart holds genuine multi-step / intelligent shortcuts — commands that
// orchestrate several MCP calls or resolve names to IDs, so they are NOT a 1:1
// wrapper over a single tool. This is the "shortcut as a real capability" layer,
// distinct from the 1:1 ergonomic wrappers under the per-service packages.
package smart

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

// DM: message a person by NAME, no ID juggling.
//
// Steps: resolve name → single user (disambiguate on multiple matches) → send
// a single-chat message via openDingTalkId. Replaces `contact +search-user`
// (copy openDingTalkId) → `chat +messages-send --open-dingtalk-id <id>`.
//
//	dws chat +dm --to 张三 --text "周报发我一下"
var DM = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+dm",
	Product:     "chat",
	Description: "按姓名直接给某人发单聊消息（自动解析唯一 openDingTalkId）",
	Intent: "当你只知道对方姓名、想直接发一条单聊消息而不想先查 userId 时使用；" +
		"内部先按姓名搜通讯录解析出唯一用户，并用其 openDingTalkId 发送，姓名匹配到多人时会列出候选让你区分。会真实发出消息。",
	Risk: shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_dm",
			CanonicalPath:  "chat.shortcut_dm",
			CLIPath:        "chat +dm",
			PrimaryCLIPath: "chat +dm",
		},
		Description: "按姓名直接给某人发单聊消息（自动解析唯一 openDingTalkId）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按姓名直接给某人发单聊消息（自动解析唯一 openDingTalkId）",
			UseWhen:      []string{"当你只知道对方姓名、想直接发一条单聊消息而不想先查 userId 时使用；内部先按姓名搜通讯录解析出唯一用户，并用其 openDingTalkId 发送，姓名匹配到多人时会列出候选让你区分。会真实发出消息。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +dm --to 张三 --text \"周报发我一下\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagString, Desc: "收件人姓名/花名", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "消息内容（支持 Markdown）", Required: true},
		shortcut.AIMessageTagFlag(),
	},
	Tips: []string{`dws chat +dm --to 张三 --text "周报发我一下"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		text := rt.Str("text")

		resolved, err := targetresolver.ResolveUser(
			rt,
			rt.Str("to"),
			targetresolver.IdentityOpenDingTalkID,
		)
		if err != nil {
			return err
		}
		return chatshortcut.ExecuteResolvedUserMarkdown(
			rt,
			chatshortcut.ResolvedUserMessageTarget{
				OpenDingTalkID: resolved.Selected.OpenDingTalkID,
			},
			text,
		)
	},
}

func init() {
	shortcut.Register(DM)
}

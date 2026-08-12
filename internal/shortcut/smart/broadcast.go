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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/targetresolver"
)

// Broadcast: send the SAME single-chat message to several people by NAME.
//
// Steps: split the --to name CSV → resolve each name to a unique user →
// send an individual single-chat message to each user's openDingTalkId. Names that
// fail to resolve (unknown / ambiguous) are collected and reported at the end
// without aborting delivery to the others. Replaces manually running
// `chat +dm` once per recipient.
//
//	dws chat +broadcast --to "张三,李四,王五" --text "今晚 8 点上线，请留意群公告"
var Broadcast = shortcut.Shortcut{
	Service:     "chat",
	Command:     "+broadcast",
	Product:     "chat",
	Description: "按姓名逐一给多个人群发同一条单聊消息（自动解析 userId、逐个发送）",
	Intent: "当你想把同一条通知一次性单聊发给多位同事、但只知道他们的姓名不想逐个查 userId 时使用；" +
		"内部把姓名列表逐个解析成唯一用户后，用 openDingTalkId 对每个人单独发一条单聊消息，并汇总成功/失败人数。" +
		"某个姓名匹配不到人或匹配到多人时，会跳过该人并在结尾报出，不影响其他人收到消息。会真实发出多条消息。",
	Risk: shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium",
		Confirmation: "user_required", Idempotency: "unknown",
	},
	Contract: corecmd.ContractDecl{
		Identity: contract.ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "shortcut_broadcast",
			CanonicalPath:  "chat.shortcut_broadcast",
			CLIPath:        "chat +broadcast",
			PrimaryCLIPath: "chat +broadcast",
		},
		Description: "按姓名逐一给多个人群发同一条单聊消息（自动解析 userId、逐个发送）",
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "Reviewed built-in shortcut adapter: the executable CLI owns validation, optional multi-step orchestration, output projection, and confirmation; the complete command contract is not represented by one pinned MCP interface_ref.",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: "按姓名逐一给多个人群发同一条单聊消息（自动解析 userId、逐个发送）",
			UseWhen:      []string{"当你想把同一条通知一次性单聊发给多位同事、但只知道他们的姓名不想逐个查 userId 时使用；内部把姓名列表逐个解析成唯一用户后，用 openDingTalkId 对每个人单独发一条单聊消息，并汇总成功/失败人数。某个姓名匹配不到人或匹配到多人时，会跳过该人并在结尾报出，不影响其他人收到消息。会真实发出多条消息。"},
			AvoidWhen:    []string{"需要该 Shortcut 未公开的底层参数、原始响应或不同执行语义时，改用对应原子命令"},
			Examples:     []string{"dws chat +broadcast --to \"张三,李四,王五\" --text \"今晚 8 点上线，请留意\""},
		},
	},
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagStringSlice, Desc: "收件人姓名/花名，逗号分隔的多个人", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "消息内容（支持 Markdown），所有人收到同一条", Required: true},
		shortcut.AIMessageTagFlag(),
	},
	Tips: []string{`dws chat +broadcast --to "张三,李四,王五" --text "今晚 8 点上线，请留意"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		text := rt.Str("text")
		names := rt.StrSlice("to")
		if len(names) == 0 {
			return apperrors.NewValidation("--to 至少要包含一个姓名")
		}

		content, _ := json.Marshal(map[string]string{"title": text, "text": text})

		var (
			sent   []string
			failed []string
			plans  []map[string]any
		)
		for _, raw := range names {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}

			// Step 1 — resolve this name to a unique userId. On failure
			// (unknown / ambiguous) record it and keep going.
			resolved, err := targetresolver.ResolveEnterpriseUser(rt, name, targetresolver.IdentityAny)
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s（%s）", name, err.Error()))
				continue
			}
			user := resolved.Selected
			targetArgs := map[string]any{}
			if user.OpenDingTalkID != "" {
				targetArgs["receiverOpenDingTalkId"] = user.OpenDingTalkID
			} else {
				targetArgs["receiverUserId"] = user.UserID
			}
			recipient := user.Name
			if recipient == "" {
				recipient = name
			}
			messageArgs := rt.AddAIMessageTag(map[string]any{
				"msgType": "markdown",
				"content": string(content),
			})
			for key, value := range targetArgs {
				messageArgs[key] = value
			}

			// Step 2 — send the single-chat message to this recipient. Under
			// --dry-run we still resolve names (a read) but never send: record the
			// resolved recipient as "would send" and move on.
			if rt.DryRun() {
				plans = append(plans, map[string]any{
					"recipient": recipient,
					"tool":      "send_personal_message",
					"arguments": messageArgs,
				})
				sent = append(sent, recipient)
				continue
			}
			if _, err := rt.CallMCPWriteData("chat", "send_personal_message", messageArgs); err != nil {
				failed = append(failed, fmt.Sprintf("%s（发送失败：%s）", name, err.Error()))
				continue
			}
			sent = append(sent, recipient)
		}

		// Summarize via rt.Output (structured, honours --format/--jq/--fields)
		// instead of fmt.Printf, which ignored the output flags.
		if len(sent) == 0 {
			return apperrors.NewValidation("没有任何人收到消息，请检查姓名是否正确")
		}
		result := map[string]any{
			"sentCount":   len(sent),
			"failedCount": len(failed),
			"sent":        sent,
		}
		if len(failed) > 0 {
			result["failed"] = failed
		}
		if rt.DryRun() {
			result["dry_run"] = true
			result["executed"] = false
			result["preview_kind"] = "plan"
			result["tool"] = "send_personal_message"
			result["actionCount"] = len(plans)
			result["actions"] = plans
		}
		return rt.Output(result)
	},
}

func init() {
	shortcut.Register(Broadcast)
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/outboundcall"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const (
	outboundCallServer     = "outbound-call"
	outboundCallCreateTool = "create_dingtalk_audio_talk_call"
	outboundCallDetailTool = "outbound_call_detail"
)

func newOutboundCallCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: outboundCallServer,
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{contract.SkillDocumentation("AI 语音外呼指南", "dingtalk-misc", "references/outbound-call.md")},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "由 AI 助手代打钉钉语音通话或钉钉语音电话，创建外呼任务，按 taskId 查询通话状态和听记链接",
			UseWhen: []string{
				"需要发起钉钉语音通话或钉钉语音电话，让助手代为通知、提醒、询问或确认事项时",
				"用户说“用钉钉给张三打电话”“帮我打个钉钉语音”或“外呼一下李四”，需要向指定人员发起通话时",
				"需要创建 AI 语音外呼任务，或查询已有外呼任务状态与听记链接时",
			},
			AvoidWhen: []string{
				"发送电话 DING 或短信 DING 使用 ding",
				"开启或关闭通话纪要自动开启使用 voip；监听来电或通话邀请使用 event",
				"用户明确要求本人手动进行语音或视频通话、打开拨号界面时，本产品不提供人工通话控制",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use: outboundCallServer, Short: "AI 语音外呼",
		Hidden: true,
		Long:   "创建钉钉 AI 语音外呼任务、查询任务详情。", RunE: groupRunE,
	})
	root.AddCommand(newOutboundCallCreateCommand(), newOutboundCallDetailCommand())
	return root
}

func newOutboundCallCreateCommand() *cobra.Command {
	// Help 与 Schema 共用示例，任务内两处开场白与 --opening 逐字一致。
	const openingExample = "XX，你好！我是张三的 AI 助手，想和你确认一下明天上午九点的会议，请问方便参加吗？"
	const taskExample = `【角色】

你是张三的 AI 助手，代张三提醒XX参加会议并确认是否方便参加，不冒充张三本人。语气亲切、友好、自然。

【任务流程】

按照“问好、说明来意并提问 → 根据答复继续确认 → 礼貌结束通话”的流程与对方沟通。

节点一：使用开场白“` + openingExample + `”后等待对方回答，不连续播报后续内容，也不重复开场白。

节点二：明确能参加或不能参加时进入节点三；答复不清楚时围绕是否参加作必要澄清，每次提问后等待回答。

节点三：完成任务且对方无新问题时，礼貌结束通话，不擅自承诺改期或其他安排。

异常处理：对方不方便或拒绝继续时停止推进，礼貌结束；询问未知信息时如实说明不确定，引导向张三确认。

【话术示例】

开场：“` + openingExample + `”

能参加：“好的，已了解您方便参加，谢谢您，再见。”

不能参加：“好的，已了解您无法参加。其他安排请向张三确认，谢谢您，再见。”

不方便或拒绝继续：“好的，我就不打扰了，再见。”

询问未知信息：“这个信息我不太确定，请向张三确认。”

【重要注意事项】`
	const createExample = "dws outbound-call create --callee 'USER_ID' --task '" + taskExample + "' --opening '" + openingExample + "' --dry-run"

	// Parsed once before confirmation; dispatch uses this exact invocation's payload.
	var request outboundcall.CreateRequest
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "创建 AI 语音外呼任务",
		Long:    "创建钉钉 AI 语音外呼任务，每次仅支持一名被呼叫人。必须同时提供 --callee、--task、--opening。callee 填写被呼叫人的钉钉 userId。生成 task 前先读取 AI 语音外呼指南的填写规则与四栏目模板；Help/Schema 仅用于确认命令契约，不能替代任务编写指引。示例中的 USER_ID、张三、XX 和会议时间均需按用户已提供或已核实的信息替换。任务说明和开场白分别映射到服务端 prompt、greeting 字段。输入上限 1 MiB。创建会真实呼叫用户，需要确认；先用 --dry-run 预览。",
		Example: createExample,
		Server:  outboundCallServer, Tool: outboundCallCreateTool,
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
		Flags: []LeafFlag{
			{Name: "callee", Usage: "被呼叫人的钉钉 userId，原样发送给服务端", Required: true, MarkRequired: true},
			{Name: "task", Usage: "完整外呼任务说明，必须按顺序填写【角色】、【任务流程】、【话术示例】、【重要注意事项】，标签独立成行、段落间留空行，保留实际换行并作为一个参数值传入，不能仅传一句任务目标。角色说明 AI 助手身份、目标和语气；任务流程先写总览，再写节点与异常处理；话术示例按场景填写；无额外注意事项时保留标签、正文留空", Required: true, MarkRequired: true},
			{Name: "opening", Usage: "外呼开场白，包含问好、明确的 AI 助手身份和来电目的，以任务相关问句结尾；须与 --task 的任务流程节点一、话术示例中的两处开场白逐字一致，不得冒充真人", Required: true, MarkRequired: true},
		},
		Validate: func(cmd *cobra.Command, _ []string) error {
			request = outboundcall.CreateRequest{}
			parsed, err := outboundcall.Single(MustGetStringFlag(cmd, "callee"), MustGetStringFlag(cmd, "task"), MustGetStringFlag(cmd, "opening"))
			if err == nil {
				request = parsed
			}
			return err
		},
		ResultCall: func(cmd *cobra.Command, tool string, _ map[string]any) (output.CommandResult, error) {
			return outboundCallResult(cmd, tool, request.Arguments())
		},
		PostMount: func(cmd *cobra.Command) { cmd.Args = cobra.NoArgs },
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: outboundCallServer, Name: "create", CanonicalPath: "outbound-call.create", CLIPath: "outbound-call create", PrimaryCLIPath: "outbound-call create"},
			Description: "创建钉钉 AI 语音外呼任务",
			Interface:   outboundCallInterface(outboundCallCreateTool),
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":["object","array","string","number","boolean"],"description":"原样保留的非 null JSON 业务回执；不承诺服务端未声明的字段结构。可识别单一 taskId 时以 pending 返回，并在 meta.operation 提供查询指令。回执不代表通话完成。"}`),
			},
			Parameters: []contract.ParamDecl{
				{Name: "callee", Property: "calleeInfos[0].callee", InterfaceType: "string"},
				{Name: "task", Property: "calleeInfos[0].prompt", InterfaceType: "string"},
				{Name: "opening", Property: "calleeInfos[0].greeting", InterfaceType: "string"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "由 AI 助手代打钉钉语音通话或钉钉语音电话，按 userId、任务说明和开场白创建外呼；只有姓名时先用 contact 确认 userId，每次仅一人",
				UseWhen: []string{
					"用户要求发起钉钉语音通话或钉钉语音电话，向指定人员通知、提醒、询问或确认事项时",
					"用户说“用钉钉给张三打电话”，需要创建一通新的语音通话时",
					"用户说“外呼一下李四”，需要创建一通新的语音通话时",
					"用户要求让 AI 通过语音外呼指定用户，且已明确被呼叫人的钉钉 userId 和通话内容时",
				},
				AvoidWhen: []string{
					"已有 taskId 只需查看状态或听记时使用 outbound-call detail；电话 DING 使用 ding",
					"开启或关闭通话纪要自动开启使用 voip；监听来电或通话邀请使用 event",
					"用户明确要求本人手动进行语音或视频通话、打开拨号界面时，本命令不提供人工通话控制",
				},
				Examples: []string{createExample},
			},
		},
	})
}

func newOutboundCallDetailCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use: "detail", Short: "查询外呼任务详情",
		Long:    "按 taskId 查询钉钉 AI 外呼任务详情，包括服务端返回的通话状态、开始和结束时间、听记链接。查询成功不代表通话成功，callStatus 等业务字段保持原值。",
		Example: "  dws outbound-call detail --task-id TASK_ID --format json",
		Server:  outboundCallServer, Tool: outboundCallDetailTool,
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Flags:         []LeafFlag{{Name: "task-id", Usage: "外呼任务 taskId", Bind: "taskId", Required: true, MarkRequired: true, Trim: true}},
		ResultCall:    outboundCallResult,
		PostMount:     func(cmd *cobra.Command) { cmd.Args = cobra.NoArgs },
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: outboundCallServer, Name: "detail", CanonicalPath: "outbound-call.detail", CLIPath: "outbound-call detail", PrimaryCLIPath: "outbound-call detail"},
			Description: "按 taskId 查询钉钉 AI 外呼任务详情",
			Interface:   outboundCallInterface(outboundCallDetailTool),
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":["object","array","string","number","boolean"],"description":"原样保留的非 null JSON 业务返回；不承诺服务端未声明的字段结构，callStatus 等业务值不做本地解释。success 仅表示查询成功。"}`),
			},
			Parameters: []contract.ParamDecl{{Name: "task-id", Property: "taskId", InterfaceType: "string"}},
			Selection: contract.SelectionSpec{
				AgentSummary: "使用已有 taskId 查询 AI 外呼任务的通话状态、时间和听记链接",
				UseWhen:      []string{"已持有外呼 taskId，需要查看通话进度、结果或听记链接时"},
				AvoidWhen:    []string{"尚未创建外呼任务、需要发起新的通话时使用 outbound-call create"},
				Examples:     []string{"dws outbound-call detail --task-id TASK_ID --format json"},
			},
		},
	})
}

func outboundCallInterface(tool string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{Mode: contract.InterfaceModeMCP, Availability: contract.InterfaceAvailable,
		Ref: &contract.InterfaceRefSpec{ProductID: outboundCallServer, RPCName: strings.TrimSpace(tool)}}
}

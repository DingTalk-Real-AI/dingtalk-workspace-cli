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
			AgentSummary: "创建钉钉 AI 语音外呼任务，按 taskId 查询通话状态和听记链接",
			UseWhen:      []string{"需要通过 AI 语音向指定用户发起外呼，或查询已有外呼任务状态与听记时"},
			AvoidWhen:    []string{"发送电话 DING 或短信 DING 使用 ding；仅查询人员信息使用 contact"},
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
	// Parsed once before confirmation; dispatch uses this exact invocation's payload.
	var request outboundcall.CreateRequest
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "创建 AI 语音外呼任务",
		Long:    "创建钉钉 AI 语音外呼任务，每次仅支持一名被呼叫人。必须同时提供 --callee、--task、--opening。callee 填写被呼叫人的钉钉 userId，示例中的 USER_ID 需替换为真实 userId。任务说明和开场白分别映射到服务端 prompt、greeting 字段。输入上限 1 MiB。创建会真实呼叫用户，需要确认；先用 --dry-run 预览。",
		Example: "  dws outbound-call create --callee 'USER_ID' --task '提醒XX明天九点开会，并询问是否能参加' --opening 'XX, 你好，我是会议提醒助手，来向你确认明天的会议安排。明天早上九点开会，请问能准时参加吗？' --dry-run",
		Server:  outboundCallServer, Tool: outboundCallCreateTool,
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "non_idempotent"},
		Flags: []LeafFlag{
			{Name: "callee", Usage: "被呼叫人的钉钉 userId，原样发送给服务端", Required: true, MarkRequired: true},
			{Name: "task", Usage: "本次外呼需要完成的事项与要求，包括需要告知的信息、需要询问或确认的问题", Required: true, MarkRequired: true},
			{Name: "opening", Usage: "外呼开场白，用于向被呼叫人问好，并说明身份和来电目的。", Required: true, MarkRequired: true},
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
				AgentSummary: "按被呼叫人的钉钉 userId、任务说明和开场白创建 AI 外呼；每次仅支持一名被呼叫人",
				UseWhen:      []string{"用户要求让 AI 通过语音外呼指定用户，且已明确被呼叫人的钉钉 userId 和通话内容时"},
				AvoidWhen:    []string{"尚未确定被呼叫人的钉钉 userId 时，先使用 contact 查询人员信息", "已有 taskId 只需查看状态或听记时使用 outbound-call detail；电话 DING 使用 ding"},
				Examples:     []string{"dws outbound-call create --callee 'USER_ID' --task '提醒XX明天九点开会，并询问是否能参加' --opening 'XX, 你好，我是会议提醒助手，来向你确认明天的会议安排。明天早上九点开会，请问能准时参加吗？' --dry-run"},
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

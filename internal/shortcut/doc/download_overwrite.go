// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const downloadOverwriteCommand = "+download-overwrite"

var DownloadOverwrite = shortcut.Shortcut{
	Service: "doc", Command: downloadOverwriteCommand, Product: productDoc,
	Description:   "确认后下载正文媒体或封面，允许原子替换已有普通文件",
	Intent:        "用户明确需要覆盖本地已有文件时使用；source=media下载正文资源，source=cover下载封面。默认不覆盖或临时预览使用+media-download、+media-preview或+resource-download。",
	Risk:          shortcut.RiskWrite,
	Safety:        contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	OutputRollout: output.RolloutUnifiedActive,
	Contract:      downloadOverwriteContract(),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Required: true, Desc: "文档ID或URL"},
		{Name: "source", Type: shortcut.FlagString, Default: "media", Enum: []string{"media", "cover"}, Desc: "media下载正文媒体，cover下载当前封面；media必须提供附件UUID；cover不能提供resource-id"},
		{Name: "resource-id", Type: shortcut.FlagString, Desc: "media必须提供附件UUID；cover不能提供resource-id"},
		{Name: "output", Shorthand: "o", Type: shortcut.FlagString, Required: true, Desc: "output必须为工作目录内相对路径；不替换符号链接或非普通文件；允许原子覆盖普通文件"},
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"source", "resource-id"}, Description: "media必须提供附件UUID；cover不能提供resource-id"},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"output"}, Description: "output必须为工作目录内相对路径；不替换符号链接或非普通文件"},
	},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Str("source") == "media" {
			if err := validateDocResourceID(rt.Str("resource-id")); err != nil {
				return err
			}
		} else if rt.Changed("resource-id") {
			return apperrors.NewValidation("source=cover不能提供resource-id")
		}
		if err := localio.ValidateOutput(rt.Str("output")); err != nil {
			return apperrors.NewValidation(err.Error())
		}
		return nil
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if rt.DryRun() {
			return rt.Output(map[string]any{"executed": false, "preview_kind": "plan", "nodeId": rt.Str("node"), "source": rt.Str("source"), "localPath": rt.Str("output")})
		}
		if rt.Str("source") == "cover" {
			return executeResourceDownload(rt)
		}
		return executeMediaDownload(rt)
	},
}

func downloadOverwriteContract() corecmd.ContractDecl {
	d := docContract(downloadOverwriteCommand, "确认后覆盖下载文档资源", "明确要替换本地文件时使用；普通下载使用默认不覆盖的原入口", []string{"dws doc +download-overwrite --node <DOC_ID> --source cover --output ./cover.png"})
	d.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false}
	d.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"executed":{"type":"boolean","description":"是否已执行本地下载发布"},"preview_kind":{"type":"string","description":"dry-run计划类型"},"nodeId":{"type":"string","description":"资源所属文档"},"source":{"type":"string","description":"资源来源media或cover"},"resourceId":{"type":"string","description":"正文媒体资源UUID"},"localPath":{"type":"string","description":"工作目录内的输出相对路径"},"sizeBytes":{"type":"integer","description":"下载文件字节数"},"verified":{"type":"boolean","description":"正文媒体下载非空校验结果"}}}`)}
	return d
}

func outputDocDownload(rt *shortcut.RuntimeContext, operation string, data map[string]any) error {
	if rt.Command().Name() == downloadOverwriteCommand {
		data["executed"] = true
		data["nodeId"] = rt.Str("node")
		data["source"] = rt.Str("source")
		return rt.Output(data)
	}
	return rt.Output(docEnvelope(operation, data))
}

func init() { registerDocShortcuts(DownloadOverwrite) }

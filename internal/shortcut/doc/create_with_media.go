// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const createWithMediaCommand = "+create-with-media"

var CreateWithMedia = func() shortcut.Shortcut {
	s := Create
	s.Command = createWithMediaCommand
	s.Description = "确认后创建文档并按顺序上传本地图片和附件"
	s.Intent = "需要在新文档正文末尾追加本地图片或附件时使用；确认前不创建文档或上传文件，所有路径先校验；失败保留已创建文档和媒体进度。只创建正文使用+create，已有文档插入媒体使用+media-insert。"
	s.Safety.Confirmation = "user_required"
	s.Flags = append(append([]shortcut.Flag(nil), Create.Flags...), shortcut.Flag{Name: "media-files", Type: shortcut.FlagStringSlice, Required: true, Desc: "media-files必须为1到20个工作目录内非空普通文件的相对路径，不能重复；按顺序追加，不改写正文内相对链接"})
	s.Constraints = []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"media-files"}, Description: "media-files必须为1到20个工作目录内非空普通文件的相对路径，不能重复"}}
	s.Validate = func(rt *shortcut.RuntimeContext) error {
		_, err := validateDocCreateMedia(rt)
		return err
	}
	s.Tips = []string{"dws doc +create-with-media --name 项目资料 --media-files ./report.pdf"}
	s.Contract = docContract(s.Command, s.Description, s.Intent, s.Tips,
		contract.ParamDecl{Name: "folder", Property: "folderId"},
		contract.ParamDecl{Name: "workspace", Property: "workspaceId"})
	s.Contract.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false}
	s.Contract.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"executed":{"type":"boolean","description":"是否执行创建和媒体上传"},"preview_kind":{"type":"string","description":"dry-run计划类型"},"nodeId":{"type":"string","description":"已创建文档ID"},"result":{"type":"object","description":"底层创建回执"},"verified":{"type":"boolean","description":"初始正文或文档已独立读回核对"},"verification":{"type":"object","description":"初始内容读回证据"},"media":{"type":"array","description":"按输入顺序排列的媒体插入回执","items":{"type":"object"}},"mediaPlacement":{"type":"string","description":"媒体位置，append_in_order表示按序追加"},"steps":{"type":"array","description":"已完成步骤","items":{"type":"object"}},"warnings":{"type":"array","description":"内容转换和验证边界说明","items":{"type":"string"}},"create":{"type":"object","description":"dry-run创建请求"},"docFormat":{"type":"string","description":"初始正文格式"},"contentBytes":{"type":"integer","description":"初始正文长度"},"mediaFiles":{"type":"array","description":"已校验的本地媒体路径","items":{"type":"string"}},"chunkPlan":{"type":"object","description":"长Markdown分片计划"}}}`), SensitivePaths: []string{"create.markdown"}}
	s.OutputRollout = output.RolloutUnifiedActive
	return s
}()

func outputCreatedDoc(rt *shortcut.RuntimeContext, envelope map[string]any) error {
	if rt.Command().Name() != createWithMediaCommand {
		return rt.Output(envelope)
	}
	data := envelope["data"].(map[string]any)
	data["executed"] = !rt.DryRun()
	data["steps"] = append([]map[string]any{}, envelope["steps"].([]map[string]any)...)
	data["warnings"] = envelope["warnings"]
	if rt.DryRun() {
		data["preview_kind"] = "plan"
		delete(data, "previewKind")
	}
	return rt.Output(data)
}

func init() { registerDocShortcuts(CreateWithMedia) }

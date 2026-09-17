// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var AttachmentDownload = shortcut.Shortcut{Service: "aitable", Command: "+record-download-attachment", Product: serverMain, Description: "按记录单元格中的准确 resourceId 下载附件，校验字节数并返回 SHA256", Intent: "已有 Base/表/记录/附件字段和 resourceId 时；先重新读取该附件的签名地址，完整下载后才发布，不覆盖本地已有文件。", Risk: shortcut.RiskRead, Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, Contract: aitableCompositeContractWithResult("+record-download-attachment", "按准确附件 ID 下载并验证字节数", "下载已知记录附件并交付真实本地文件时", "只有孤立 fileToken 不能定位所属记录；上传用 +attachment-put；移除用 +attachment-remove", `dws aitable +record-download-attachment --base-id B --table-id T --record-id R --field-id F --resource-id A --output attachment.bin`, &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"resourceId":{"type":"string","description":"实际下载的附件资源 ID"},"output":{"type":"string","description":"工作目录内相对路径"},"sizeBytes":{"type":"integer","description":"经校验的实际字节数"},"sha256":{"type":"string","description":"实际下载字节的 SHA256"}},"required":["resourceId","output","sizeBytes","sha256"]}`)}), Flags: []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}, {Name: "table-id", Type: shortcut.FlagString, Desc: "Table ID", Required: true}, {Name: "record-id", Type: shortcut.FlagString, Desc: "准确记录 ID", Required: true}, {Name: "field-id", Type: shortcut.FlagString, Desc: "附件字段 ID", Required: true}, {Name: "resource-id", Type: shortcut.FlagString, Desc: "同一附件单元格中的准确 resourceId", Required: true}, {Name: "output", Type: shortcut.FlagString, Desc: "工作目录内相对路径，不覆盖已有文件", Required: true}}, Execute: func(rt *shortcut.RuntimeContext) error {
	cwd, err := aitableWorkingDirectory()
	if err != nil {
		return err
	}
	if _, _, err = localio.ResolveOutputPath(cwd, rt.Str("output"), "", "attachment.bin"); err != nil {
		return err
	}
	_, items, err := readAttachmentCell(rt, rt.Str("base-id"), rt.Str("table-id"), rt.Str("record-id"), rt.Str("field-id"))
	if err != nil {
		return err
	}
	item, err := exactAttachmentForDownload(items, rt.Str("resource-id"))
	if err != nil {
		return err
	}
	size, ok := numericInt64(item["size"])
	if !ok || size < 0 {
		return fmt.Errorf("attachment lacks valid byte size")
	}
	url := stringValue(item, "url")
	if _, err = localio.ValidateDownloadURL(url); err != nil {
		return fmt.Errorf("attachment lacks a usable HTTPS download URL")
	}
	if rt.DryRun() {
		return rt.Output(map[string]any{"executed": false, "resourceId": rt.Str("resource-id"), "sizeBytes": size})
	}
	f, err := downloadAITableAttachment(rt.Command().Context(), url, localio.DownloadOptions{BaseDir: cwd, Output: rt.Str("output"), PreferredName: attachmentName(item), ExpectedSize: &size})
	if err != nil {
		return err
	}
	return rt.Output(map[string]any{"resourceId": rt.Str("resource-id"), "output": f.RelativePath, "sizeBytes": f.SizeBytes, "sha256": f.SHA256})
}}

func exactAttachmentForDownload(items []map[string]any, id string) (map[string]any, error) {
	var selected map[string]any
	for _, item := range items {
		if attachmentResourceID(item) == id {
			if selected != nil {
				return nil, fmt.Errorf("attachment resourceId is ambiguous")
			}
			selected = item
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("attachment resourceId is absent from the selected cell")
	}
	return selected, nil
}
func init() { shortcut.Register(withAITableParityAliases(AttachmentDownload)) }

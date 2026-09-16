// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0
package doc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const docMediaUploadIntent = "只上传绑定到在线文字文档的图片或附件资源时使用；下载核对SHA256后返回资源ID，不插入正文。需要插入正文用 doc +media-insert；电子表格媒体用 sheet media-upload，普通文件入库用 drive +upload。"

var MediaUpload = shortcut.Shortcut{
	Service: "doc", Command: "+media-upload", Product: productDoc,
	Description: "上传同文档可复用媒体并下载校验字节，不插入正文",
	Intent:      docMediaUploadIntent,
	Risk:        shortcut.RiskWrite, Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	OutputRollout: output.RolloutUnifiedActive, Contract: mediaUploadContract(),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Required: true, Desc: "资源所属文档ID或URL"},
		{Name: "file", Type: shortcut.FlagString, Required: true, Desc: "file必须为工作目录内的相对文件路径"},
		{Name: "name", Type: shortcut.FlagString, Desc: "可选资源文件名"},
		{Name: "mime-type", Type: shortcut.FlagString, Desc: "可选MIME类型，默认按扩展名"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"file"}, Description: "file必须为工作目录内的相对文件路径"}},
	Validate:    func(rt *shortcut.RuntimeContext) error { return validateWorkspaceInputPath("file", rt.Str("file")) },
	Execute:     executeMediaUpload,
}

func mediaUploadContract() corecmd.ContractDecl {
	d := docContract("+media-upload", "上传可复用媒体并校验字节", docMediaUploadIntent, []string{`dws doc +media-upload --node <DOC_ID> --file ./image.png`})
	d.Selection.AvoidWhen = append(d.Selection.AvoidWhen, "电子表格媒体上传使用 sheet media-upload；普通文件入库存储使用 drive +upload；本命令的资源ID仅供所属文字文档复用")
	d.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewPlan, RemoteReads: false}
	d.Result = &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","properties":{"preview_kind":{"type":"string","description":"dry-run的计划类型plan"},"nodeId":{"type":"string","description":"资源所属文档"},"resourceId":{"type":"string","description":"已上传资源ID"},"resourceUrl":{"type":"string","description":"同文档可引用的资源URL"},"fileName":{"type":"string","description":"资源名称"},"mimeType":{"type":"string","description":"资源MIME类型"},"size":{"type":"integer","description":"资源字节数"},"sha256":{"type":"string","description":"上传前与下载后相同的SHA256"},"verified":{"type":"boolean","description":"是否独立下载核对字节"},"executed":{"type":"boolean","description":"是否执行上传"},"inserted":{"type":"boolean","description":"是否插入正文，本入口恒false"}}}`), SensitivePaths: []string{"resourceUrl"}}
	return d
}
func hashDocFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func executeMediaUpload(rt *shortcut.RuntimeContext) error {
	if rt.DryRun() {
		return rt.Output(map[string]any{"nodeId": rt.Str("node"), "executed": false, "preview_kind": "plan", "inserted": false, "verified": false})
	}
	expected, err := hashDocFile(rt.Str("file"))
	if err != nil {
		return err
	}
	return helpers.RunDocMediaUploadWithResult(rt.Command(), func(data map[string]any) error {
		resource := nestedString(data, "resourceId")
		fail := func(cause error) error {
			return docPartialWriteError("doc.media_upload", "doc_media_upload_verification_failed", "verify", "资源可能已上传，但独立字节验证未通过；不要直接重试上传", cause, data, nil, map[string]any{"available": false, "reason": "verify resource under the same document before retrying"})
		}
		if err := validateDocResourceID(resource); err != nil {
			return fail(err)
		}
		receipt, err := rt.CallMCPData(productDoc, "download_doc_attachment", map[string]any{"nodeId": rt.Str("node"), "resourceId": resource})
		if err != nil {
			return fail(err)
		}
		dir, err := docMkdirTemp("", "dws-doc-upload-verify-")
		if err != nil {
			return fail(err)
		}
		defer os.RemoveAll(dir)
		result, err := downloadResolvedResource(rt, receipt, dir, ".")
		if err != nil {
			return fail(err)
		}
		got, err := hashDocFile(result.AbsolutePath)
		if err != nil {
			return fail(err)
		}
		if got != expected {
			return fail(apperrors.NewAPI("上传资源SHA256与原文件不一致"))
		}
		data["sha256"] = got
		data["verified"] = true
		data["executed"] = true
		data["inserted"] = false
		return rt.Output(data)
	})
}
func init() { registerDocShortcuts(MediaUpload) }

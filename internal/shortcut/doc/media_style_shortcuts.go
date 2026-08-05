// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var MediaList = shortcut.Shortcut{
	Service: "doc", Command: "+media-list", Product: productDoc,
	Description: "列出文档正文中的图片和附件资源",
	Intent:      "当用户要发现文档内可下载或可定位的图片、附件及其 blockId/resourceId 时使用；只读取并投影媒体节点。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+media-list", "列出文档正文中的图片和附件资源",
		"当用户要发现文档内可下载或可定位的图片、附件及其 blockId/resourceId 时使用；只读取并投影媒体节点。",
		[]string{`dws doc +media-list --node <DOC_ID>`}),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true}},
	Tips:  []string{`dws doc +media-list --node <DOC_ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		data, err := rt.CallMCPData(productDoc, "list_document_blocks", map[string]any{"nodeId": rt.Str("node"), "format": "element"})
		if err != nil {
			return err
		}
		items := collectMediaItems(data)
		return rt.Output(map[string]any{"count": len(items), "media": items})
	},
}

var MediaInsert = shortcut.Shortcut{
	Service: "doc", Command: "+media-insert", Product: productDoc,
	Description: "上传本地图片或文件并插入文档正文",
	Intent:      "当用户要把本地图片或附件作为正文 block 插入在线文档时使用；组合本地校验、上传凭证、OSS PUT 和插块，失败时不会伪造完整回滚。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: withDryRun(docContract("+media-insert", "上传本地图片或文件并插入文档正文",
		"当用户要把本地图片或附件作为正文 block 插入在线文档时使用；组合本地校验、上传凭证、OSS PUT 和插块，失败时不会伪造完整回滚。",
		[]string{`dws doc +media-insert --node <DOC_ID> --file ./report.pdf`, `dws doc +media-insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where after`}), contract.DryRunPreviewPlan, false),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "file", Type: shortcut.FlagString, Desc: "本地文件路径", Required: true},
		{Name: "name", Type: shortcut.FlagString, Desc: "显示名称"},
		{Name: "mime-type", Type: shortcut.FlagString, Desc: "MIME 类型"},
		{Name: "index", Type: shortcut.FlagInt, Desc: "顶层插入索引"},
		{Name: "where", Type: shortcut.FlagString, Desc: "相对参考块的位置", Enum: []string{"before", "after"}},
		{Name: "ref-block", Type: shortcut.FlagString, Desc: "参考 block ID"},
	},
	Tips:    []string{`dws doc +media-insert --node <DOC_ID> --file ./report.pdf`, `dws doc +media-insert --node <DOC_ID> --file ./image.png --ref-block <BLOCK_ID> --where after`},
	Execute: func(rt *shortcut.RuntimeContext) error { return helpers.RunDocMediaInsertShortcut(rt.Command()) },
}

var MediaDownload = shortcut.Shortcut{
	Service: "doc", Command: "+media-download", Product: productDoc,
	Description: "安全下载文档正文附件到工作目录",
	Intent:      "当用户已从 media-list 或 block 数据拿到 resourceId，要把正文附件保存到本地时使用；默认拒绝覆盖并原子发布文件。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+media-download", "安全下载文档正文附件到工作目录",
		"当用户已从 media-list 或 block 数据拿到 resourceId，要把正文附件保存到本地时使用；默认拒绝覆盖并原子发布文件。",
		[]string{`dws doc +media-download --node <DOC_ID> --resource-id <RESOURCE_ID> --output ./downloads/`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "resource-id", Type: shortcut.FlagString, Desc: "附件 resourceId", Required: true},
		{Name: "output", Type: shortcut.FlagString, Default: ".", Desc: "工作目录内相对路径（文件或目录）"},
		{Name: "overwrite", Type: shortcut.FlagBool, Desc: "允许覆盖已有文件"},
	},
	Validate:    func(rt *shortcut.RuntimeContext) error { return localio.ValidateOutput(rt.Str("output")) },
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"output"}, Description: "--output 必须是工作目录内相对路径；默认 no-clobber"}},
	Tips:        []string{`dws doc +media-download --node <DOC_ID> --resource-id <RESOURCE_ID> --output ./downloads/`},
	Execute:     executeMediaDownload,
}

var MediaPreview = shortcut.Shortcut{
	Service: "doc", Command: "+media-preview", Product: productDoc,
	Description: "下载正文媒体到受控临时目录并返回预览路径",
	Intent:      "当用户要临时查看文档附件或图片内容而不指定持久保存路径时使用；下载到独立临时目录并返回 artifact 路径。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+media-preview", "下载正文媒体到受控临时目录并返回预览路径",
		"当用户要临时查看文档附件或图片内容而不指定持久保存路径时使用；下载到独立临时目录并返回 artifact 路径。",
		[]string{`dws doc +media-preview --node <DOC_ID> --resource-id <RESOURCE_ID>`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "resource-id", Type: shortcut.FlagString, Desc: "附件 resourceId", Required: true},
	},
	Tips: []string{`dws doc +media-preview --node <DOC_ID> --resource-id <RESOURCE_ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.media_preview", map[string]any{"executed": false, "nodeId": rt.Str("node"), "resourceId": rt.Str("resource-id"), "output": "managed_temp_dir"}))
		}
		data, err := rt.CallMCPData(productDoc, "download_doc_attachment", map[string]any{"nodeId": rt.Str("node"), "resourceId": rt.Str("resource-id")})
		if err != nil {
			return err
		}
		dir, err := docMkdirTemp("", "dws-doc-preview-*")
		if err != nil {
			return err
		}
		result, err := downloadResolvedResource(rt, data, dir, ".", false)
		if err != nil {
			_ = docRemoveAll(dir)
			return err
		}
		return rt.Output(docEnvelope("doc.media_preview", map[string]any{"previewPath": result.AbsolutePath, "sizeBytes": result.SizeBytes}))
	},
}

var ResourceUpdate = shortcut.Shortcut{
	Service: "doc", Command: "+resource-update", Product: productDoc,
	Description: "从本地图片或 HTTPS URL 设置文档封面",
	Intent:      "当用户要设置或替换文档顶部封面图时使用；本地图片会先上传，HTTPS URL 由服务端转存。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: withDryRun(docContract("+resource-update", "从本地图片或 HTTPS URL 设置文档封面",
		"当用户要设置或替换文档顶部封面图时使用；本地图片会先上传，HTTPS URL 由服务端转存。",
		[]string{`dws doc +resource-update --node <DOC_ID> --image https://example.com/cover.png`, `dws doc +resource-update --node <DOC_ID> --file ./cover.png`}), contract.DryRunPreviewRequest, false),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "image", Type: shortcut.FlagString, Desc: "HTTPS 封面图片 URL"},
		{Name: "file", Type: shortcut.FlagString, Desc: "本地封面图片"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"image", "file"}, Description: "--image 与 --file 必须且只能提供一个"}},
	Tips:        []string{`dws doc +resource-update --node <DOC_ID> --image https://example.com/cover.png`, `dws doc +resource-update --node <DOC_ID> --file ./cover.png`},
	Execute:     func(rt *shortcut.RuntimeContext) error { return helpers.RunDocResourceUpdateShortcut(rt.Command()) },
}

var ResourceDownload = shortcut.Shortcut{
	Service: "doc", Command: "+resource-download", Product: productDoc,
	Description: "读取并安全下载当前文档封面",
	Intent:      "当用户要把当前文档封面保存到本地时使用；先读 style，必要时用 resourceId 换临时链接，再按安全本地下载策略保存。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+resource-download", "读取并安全下载当前文档封面",
		"当用户要把当前文档封面保存到本地时使用；先读 style，必要时用 resourceId 换临时链接，再按安全本地下载策略保存。",
		[]string{`dws doc +resource-download --node <DOC_ID> --output ./cover.png`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "output", Type: shortcut.FlagString, Default: ".", Desc: "工作目录内相对路径（文件或目录）"},
		{Name: "overwrite", Type: shortcut.FlagBool, Desc: "允许覆盖已有文件"},
	},
	Validate:    func(rt *shortcut.RuntimeContext) error { return localio.ValidateOutput(rt.Str("output")) },
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"output"}, Description: "--output 必须是工作目录内相对路径；默认 no-clobber"}},
	Tips:        []string{`dws doc +resource-download --node <DOC_ID> --output ./cover.png`},
	Execute:     executeResourceDownload,
}

var ResourceDelete = shortcut.Shortcut{
	Service: "doc", Command: "+resource-delete", Product: productDoc,
	Description: "幂等清除文档封面",
	Intent:      "当用户明确要移除文档当前封面时使用；发送 cover clear，重复执行保持无封面状态。",
	Risk:        shortcut.RiskHighWrite,
	Safety:      contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: docContract("+resource-delete", "幂等清除文档封面",
		"当用户明确要移除文档当前封面时使用；发送 cover clear，重复执行保持无封面状态。",
		[]string{`dws doc +resource-delete --node <DOC_ID>`}),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true}},
	Tips:  []string{`dws doc +resource-delete --node <DOC_ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node"), "cover": map[string]any{"action": "clear"}}
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.resource_delete", map[string]any{"executed": false, "params": params}))
		}
		return rt.CallMCP("update_document_style", params)
	},
}

var BackgroundUpdate = shortcut.Shortcut{
	Service: "doc", Command: "+background-update", Product: productDoc,
	Description: "设置文档 #RRGGBB 背景纯色",
	Intent:      "当用户要设置在线文档背景纯色时使用；只接受 #RRGGBB，不支持背景图片。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+background-update", "设置文档 #RRGGBB 背景纯色",
		"当用户要设置在线文档背景纯色时使用；只接受 #RRGGBB，不支持背景图片。",
		[]string{`dws doc +background-update --node <DOC_ID> --color "#E8F2FE"`}),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true}, {Name: "color", Type: shortcut.FlagString, Desc: "#RRGGBB 背景色", Required: true}},
	Tips:  []string{`dws doc +background-update --node <DOC_ID> --color "#E8F2FE"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		color := rt.Str("color")
		if len(color) != 7 || color[0] != '#' {
			return apperrors.NewValidation("--color 必须是 #RRGGBB")
		}
		for _, char := range color[1:] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
				return apperrors.NewValidation("--color 必须是 #RRGGBB")
			}
		}
		return nil
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"color"}, Description: "#RRGGBB"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return rt.CallMCP("update_document_style", map[string]any{"nodeId": rt.Str("node"), "background": map[string]any{"action": "set", "backgroundColor": rt.Str("color")}})
	},
}

var BackgroundDelete = shortcut.Shortcut{
	Service: "doc", Command: "+background-delete", Product: productDoc,
	Description: "清除文档背景色",
	Intent:      "当用户明确要恢复文档默认背景、移除当前背景色时使用；执行 background clear 并要求确认。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
	Contract: docContract("+background-delete", "清除文档背景色",
		"当用户明确要恢复文档默认背景、移除当前背景色时使用；执行 background clear 并要求确认。",
		[]string{`dws doc +background-delete --node <DOC_ID>`}),
	Flags: []shortcut.Flag{{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true}},
	Tips:  []string{`dws doc +background-delete --node <DOC_ID>`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node"), "background": map[string]any{"action": "clear"}}
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.background_delete", map[string]any{"executed": false, "params": params}))
		}
		return rt.CallMCP("update_document_style", params)
	},
}

func executeMediaDownload(rt *shortcut.RuntimeContext) error {
	if rt.DryRun() {
		return rt.Output(docEnvelope("doc.media_download", map[string]any{"executed": false, "nodeId": rt.Str("node"), "resourceId": rt.Str("resource-id"), "output": rt.Str("output")}))
	}
	data, err := rt.CallMCPData(productDoc, "download_doc_attachment", map[string]any{"nodeId": rt.Str("node"), "resourceId": rt.Str("resource-id")})
	if err != nil {
		return err
	}
	cwd, err := docGetwd()
	if err != nil {
		return err
	}
	result, err := downloadResolvedResource(rt, data, cwd, rt.Str("output"), rt.Bool("overwrite"))
	if err != nil {
		return err
	}
	return rt.Output(docEnvelope("doc.media_download", map[string]any{"resourceId": rt.Str("resource-id"), "localPath": result.RelativePath, "sizeBytes": result.SizeBytes}))
}

func executeResourceDownload(rt *shortcut.RuntimeContext) error {
	style, err := rt.CallMCPData(productDoc, "get_document_style", map[string]any{"nodeId": rt.Str("node")})
	if err != nil {
		return err
	}
	if rt.DryRun() {
		return rt.Output(docEnvelope("doc.resource_download", map[string]any{"executed": false, "styleResolved": true, "output": rt.Str("output")}))
	}
	resourceURL := nestedStringDeep(style, "imageUrl", "resourceUrl", "downloadUrl", "url")
	resourceID := nestedStringDeep(style, "resourceId")
	data := style
	if resourceURL == "" && resourceID != "" {
		data, err = rt.CallMCPData(productDoc, "download_doc_attachment", map[string]any{"nodeId": rt.Str("node"), "resourceId": resourceID})
		if err != nil {
			return err
		}
	} else if resourceURL != "" {
		data = map[string]any{"downloadUrl": resourceURL}
	}
	if nestedStringDeep(data, "downloadUrl", "resourceUrl", "imageUrl", "url") == "" {
		return apperrors.NewAPI("当前文档没有可下载的封面，或 style 响应缺少资源地址")
	}
	cwd, err := docGetwd()
	if err != nil {
		return err
	}
	result, err := downloadResolvedResource(rt, data, cwd, rt.Str("output"), rt.Bool("overwrite"))
	if err != nil {
		return err
	}
	return rt.Output(docEnvelope("doc.resource_download", map[string]any{"localPath": result.RelativePath, "sizeBytes": result.SizeBytes}))
}

func downloadResolvedResource(rt *shortcut.RuntimeContext, data map[string]any, baseDir, output string, overwrite bool) (localio.DownloadResult, error) {
	resourceURL := nestedStringDeep(data, "downloadUrl", "resourceUrl", "imageUrl", "url")
	if resourceURL == "" {
		return localio.DownloadResult{}, apperrors.NewAPI("附件下载响应缺少 downloadUrl/resourceUrl")
	}
	headers := map[string]string{}
	if raw := nestedMap(data)["headers"]; raw != nil {
		if values, ok := raw.(map[string]any); ok {
			for key, value := range values {
				if text, ok := value.(string); ok {
					headers[key] = text
				}
			}
		}
	}
	return docDownload(rt.Command().Context(), resourceURL, localio.DownloadOptions{BaseDir: baseDir, Output: output, PreferredName: nestedStringDeep(data, "fileName", "name"), Overwrite: overwrite, Headers: headers})
}

func collectMediaItems(value any) []map[string]any {
	var out []map[string]any
	var walk func(any, string)
	walk = func(current any, inheritedID string) {
		switch typed := current.(type) {
		case map[string]any:
			blockID := blockIdentity(typed, inheritedID)
			resourceID := fmt.Sprint(typed["resourceId"])
			resourceURL := ""
			for _, key := range []string{"resourceUrl", "src", "imageUrl", "downloadUrl"} {
				if text, ok := typed[key].(string); ok && text != "" {
					resourceURL = text
					break
				}
			}
			if (resourceID != "" && resourceID != "<nil>") || resourceURL != "" {
				row := map[string]any{"blockId": blockID}
				if resourceID != "" && resourceID != "<nil>" {
					row["resourceId"] = resourceID
				}
				if resourceURL != "" {
					row["resourceUrl"] = resourceURL
				}
				for _, key := range []string{"name", "type", "mimeType", "viewType"} {
					if value, ok := typed[key]; ok {
						row[key] = value
					}
				}
				out = append(out, row)
			}
			for _, child := range typed {
				walk(child, blockID)
			}
		case []any:
			for _, child := range typed {
				walk(child, inheritedID)
			}
		}
	}
	walk(value, "")
	return out
}

func nestedStringDeep(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		for _, child := range typed {
			if found := nestedStringDeep(child, keys...); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := nestedStringDeep(child, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func init() {
	shortcut.Register(MediaList, MediaInsert, MediaDownload, MediaPreview, ResourceUpdate, ResourceDownload, ResourceDelete, BackgroundUpdate, BackgroundDelete)
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/localio"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var (
	docGetwd        = os.Getwd
	docEvalSymlinks = filepath.EvalSymlinks
	docRel          = filepath.Rel
	docReadFile     = os.ReadFile
	docMkdirTemp    = os.MkdirTemp
	docRemoveAll    = os.RemoveAll
	docDownload     = localio.Download
)

var Create = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+create",
	Product:     productDoc,
	Description: "从 Markdown 或 JSONML 创建在线文字文档",
	Intent:      "当用户要新建钉钉在线文字文档，并可同时写入 Markdown/JSONML 初始内容、指定文件夹或知识库位置时使用；不会用于普通文件上传或其他在线对象类型。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown",
	},
	Contract: docContract(
		"+create", "从 Markdown 或 JSONML 创建在线文字文档",
		"当用户要新建钉钉在线文字文档，并可同时写入 Markdown/JSONML 初始内容、指定文件夹或知识库位置时使用；不会用于普通文件上传或其他在线对象类型。",
		[]string{`dws doc +create --name "项目周报" --content "# 本周进展"`, `dws doc +create --name "模板" --content @body.json --doc-format jsonml`},
		contract.ParamDecl{Name: "folder", Property: "folderId"},
		contract.ParamDecl{Name: "workspace", Property: "workspaceId"},
	),
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "新文档名称", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "内容字面量、@工作目录相对文件或 - 表示 stdin"},
		{Name: "doc-format", Type: shortcut.FlagString, Default: "markdown", Desc: "内容格式", Enum: []string{"markdown", "jsonml"}},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文档文件夹 ID"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Tips: []string{`dws doc +create --name "项目周报" --content "# 本周进展"`, `dws doc +create --name "模板" --content @body.json --doc-format jsonml`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		content, err := readShortcutContent(rt, "content")
		if err != nil {
			return err
		}
		format := rt.Str("doc-format")
		if format == "jsonml" && content != "" {
			content, err = validateJSONML(content)
			if err != nil {
				return err
			}
		}
		params := map[string]any{"name": rt.Str("name")}
		if rt.Str("folder") != "" {
			params["folderId"] = rt.Str("folder")
		}
		if rt.Str("workspace") != "" {
			params["workspaceId"] = rt.Str("workspace")
		}
		if format == "markdown" && content != "" {
			params["markdown"] = content
		}
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.create", map[string]any{"executed": false, "previewKind": "plan", "create": params, "docFormat": format, "contentBytes": len(content)}))
		}
		created, err := rt.CallMCPWriteData(productDoc, "create_document", params)
		if err != nil {
			return err
		}
		nodeID := nestedString(created, "nodeId", "documentId", "id")
		steps := []map[string]any{{"name": "create_document", "status": "success"}}
		if format == "jsonml" && content != "" {
			if nodeID == "" {
				return docPartialWriteError(
					"doc.create", "doc_create_missing_node_id", "resolve_created_document",
					"创建文档成功但响应缺少 nodeId；JSONML 尚未写入，请先在钉钉中定位新文档，不要直接重试",
					nil,
					map[string]any{"nodeId": "", "docFormat": format},
					append(steps, map[string]any{"name": "write_jsonml", "status": "not_started"}),
					map[string]any{"available": false, "reason": "create_document did not return nodeId; locate the new document in DingTalk"},
				)
			}
			if _, err := rt.CallMCPWriteData(productDoc, "update_document", map[string]any{"nodeId": nodeID, "format": "jsonml", "jsonml": content, "mode": "overwrite"}); err != nil {
				return docPartialWriteError(
					"doc.create", "doc_create_initial_content_failed", "write_jsonml",
					fmt.Sprintf("文档已创建但 JSONML 写入失败（nodeId=%s）；不要直接重试创建", nodeID),
					err,
					map[string]any{"nodeId": nodeID, "docFormat": format},
					append(steps, map[string]any{"name": "write_jsonml", "status": "failed"}),
					map[string]any{"available": true, "action": "delete_created_document", "nodeId": nodeID, "reason": "remove the empty document before retrying create"},
				)
			}
			steps = append(steps, map[string]any{"name": "write_jsonml", "status": "success"})
		}
		return rt.Output(docEnvelope("doc.create", map[string]any{"nodeId": nodeID, "result": created}, steps...))
	},
}

var Fetch = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+fetch",
	Product:     productDoc,
	Description: "读取完整或局部文档内容，并按 detail 控制保真度",
	Intent:      "当用户要读取在线文字文档正文，或需要 block ID、JSONML、outline/range/section/keyword/tags 局部内容用于精确编辑和评论时使用；非最新历史 revision 会明确拒绝。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract(
		"+fetch", "读取完整或局部文档内容，并按 detail 控制保真度",
		"当用户要读取在线文字文档正文，或需要 block ID、JSONML、outline/range/section/keyword/tags 局部内容用于精确编辑和评论时使用；非最新历史 revision 会明确拒绝。",
		[]string{`dws doc +fetch --node <DOC_ID>`, `dws doc +fetch --node <DOC_ID> --detail with-ids --scope keyword --keyword "结论"`},
		contract.ParamDecl{Name: "node", Property: "nodeId"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "detail", Type: shortcut.FlagString, Default: "simple", Desc: "输出细节", Enum: []string{"simple", "with-ids", "full"}},
		{Name: "scope", Type: shortcut.FlagString, Default: "full", Desc: "读取范围；keyword 时 --keyword 不能为空", Enum: []string{"full", "outline", "range", "section", "keyword", "tags"}},
		{Name: "start-block-id", Type: shortcut.FlagString, Desc: "range/section 起始块 ID"},
		{Name: "end-block-id", Type: shortcut.FlagString, Desc: "range 结束块 ID"},
		{Name: "keyword", Type: shortcut.FlagString, Desc: "keyword 范围搜索词，不能为空，支持 foo|bar"},
		{Name: "tags", Type: shortcut.FlagStringSlice, Desc: "tags 范围的 JSONML tag"},
		{Name: "context-before", Type: shortcut.FlagInt, Desc: "关键词命中前的上下文字符数"},
		{Name: "context-after", Type: shortcut.FlagInt, Desc: "关键词命中后的上下文字符数"},
		{Name: "max-depth", Type: shortcut.FlagInt, Desc: "outline/section 最大深度"},
		{Name: "revision", Type: shortcut.FlagInt, Desc: "只接受当前最新版；历史 revision 暂不支持"},
	},
	Tips: []string{`dws doc +fetch --node <DOC_ID>`, `dws doc +fetch --node <DOC_ID> --detail with-ids --scope keyword --keyword "结论"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Changed("revision") {
			return apperrors.NewValidation("HISTORICAL_READ_UNSUPPORTED: 当前接口不能读取指定历史 revision")
		}
		if rt.Str("scope") == "keyword" && rt.Str("keyword") == "" {
			return apperrors.NewValidation("--scope keyword 时必须提供 --keyword")
		}
		return nil
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"scope", "keyword"}, Description: "--scope keyword 时 --keyword 不能为空"}},
	Execute: func(rt *shortcut.RuntimeContext) error {
		format := "markdown"
		if rt.Str("detail") != "simple" || rt.Str("scope") != "full" {
			format = "jsonml"
		}
		params := map[string]any{"nodeId": rt.Str("node"), "format": format}
		scope := rt.Str("scope")
		if scope != "keyword" && scope != "full" {
			params["scope"] = scope
		}
		if value := rt.Str("start-block-id"); value != "" {
			params["startBlockId"] = value
		}
		if value := rt.Str("end-block-id"); value != "" {
			params["endBlockId"] = value
		}
		if rt.Changed("tags") {
			params["tags"] = rt.StrSlice("tags")
		}
		if rt.Changed("max-depth") {
			params["maxDepth"] = rt.Int("max-depth")
		}
		data, err := rt.CallMCPData(productDoc, "get_document_content", params)
		if err != nil {
			return err
		}
		if scope == "keyword" {
			return rt.Output(projectKeywordMatches(data, rt.Str("keyword"), rt.Int("context-before"), rt.Int("context-after")))
		}
		return rt.Output(data)
	},
}

var Inspect = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+inspect",
	Product:     productDoc,
	Description: "聚合文档元信息，并按需附带样式、权限、历史、媒体和评论",
	Intent:      "当用户需要在一次调用中了解文档类型、标题、链接和可选的协作/样式/历史/媒体/评论状态，而不是读取正文时使用。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+inspect", "聚合文档元信息，并按需附带样式、权限、历史、媒体和评论",
		"当用户需要在一次调用中了解文档类型、标题、链接和可选的协作/样式/历史/媒体/评论状态，而不是读取正文时使用。",
		[]string{`dws doc +inspect --node <DOC_ID>`, `dws doc +inspect --node <DOC_ID> --include-style --include-permissions --include-comments`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "include-style", Type: shortcut.FlagBool, Desc: "附带封面和背景"},
		{Name: "include-permissions", Type: shortcut.FlagBool, Desc: "附带权限列表"},
		{Name: "include-history", Type: shortcut.FlagBool, Desc: "附带最近历史版本"},
		{Name: "include-media", Type: shortcut.FlagBool, Desc: "附带正文媒体列表"},
		{Name: "include-comments", Type: shortcut.FlagBool, Desc: "附带评论列表"},
	},
	Tips: []string{`dws doc +inspect --node <DOC_ID>`, `dws doc +inspect --node <DOC_ID> --include-style --include-permissions --include-comments`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		node := rt.Str("node")
		result := map[string]any{}
		info, err := rt.CallMCPData(productDoc, "get_document_info", map[string]any{"nodeId": node})
		if err != nil {
			return err
		}
		result["document"] = info
		reads := []struct {
			flag, key, product, tool string
			params                   map[string]any
		}{
			{"include-style", "style", productDoc, "get_document_style", map[string]any{"nodeId": node}},
			{"include-permissions", "permissions", productDoc, "list_permission", map[string]any{"nodeId": node}},
			{"include-history", "history", productDoc, "list_doc_versions", map[string]any{"nodeId": node}},
			{"include-media", "media", productDoc, "list_document_blocks", map[string]any{"nodeId": node, "format": "jsonml"}},
			{"include-comments", "comments", productComment, "list_comments", map[string]any{"nodeId": node}},
		}
		for _, read := range reads {
			if !rt.Bool(read.flag) {
				continue
			}
			value, callErr := rt.CallMCPData(read.product, read.tool, read.params)
			if callErr != nil {
				return callErr
			}
			result[read.key] = value
		}
		return rt.Output(docEnvelope("doc.inspect", result, map[string]any{"name": "inspect", "status": "success"}))
	},
}

var Update = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+update",
	Product:     productDoc,
	Description: "追加、覆盖或按 block 精确更新文档内容",
	Intent:      "当用户要修改已有在线文字文档时使用；支持整篇 append/overwrite、block 插入/替换/删除，以及受限的唯一纯文本 str_replace，所有模式统一经过静态确认门禁。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docContract("+update", "追加、覆盖或按 block 精确更新文档内容",
		"当用户要修改已有在线文字文档时使用；支持整篇 append/overwrite、block 插入/替换/删除，以及受限的唯一纯文本 str_replace，所有模式统一经过静态确认门禁。",
		[]string{`dws doc +update --node <DOC_ID> --command append --content "补充说明"`, `dws doc +update --node <DOC_ID> --command block_replace --block-id <BLOCK_ID> --content "新内容"`},
		contract.ParamDecl{Name: "doc", Property: "node"},
		contract.ParamDecl{Name: "text", Property: "content"}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true, Aliases: []string{"doc"}, AliasesVisible: true},
		{Name: "command", Type: shortcut.FlagString, Desc: "更新动作；不能为空", Enum: []string{"append", "overwrite", "block_insert_after", "block_replace", "block_delete", "str_replace", "block_copy_insert_after"}},
		{Name: "content", Type: shortcut.FlagString, Desc: "内容字面量、@相对文件或 - 表示 stdin；相关动作要求时不能为空", Aliases: []string{"text"}, AliasesVisible: true},
		{Name: "doc-format", Type: shortcut.FlagString, Default: "markdown", Desc: "内容格式", Enum: []string{"markdown", "jsonml"}},
		{Name: "block-id", Type: shortcut.FlagString, Desc: "目标或源 block ID；相关动作要求时不能为空"},
		{Name: "after-block-id", Type: shortcut.FlagString, Desc: "插入位置参考 block ID"},
		{Name: "old", Type: shortcut.FlagString, Desc: "str_replace 原文字，不能为空"},
		{Name: "new", Type: shortcut.FlagString, Desc: "str_replace 新文字；--old 不能为空，新值可为空但参数必须显式提供"},
		{Name: "expected-revision", Type: shortcut.FlagInt, Desc: "best-effort 乐观 revision 检查"},
	},
	Tips: []string{`dws doc +update --node <DOC_ID> --command append --content "补充说明"`, `dws doc +update --node <DOC_ID> --command block_replace --block-id <BLOCK_ID> --content "新内容"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		command := rt.Str("command")
		if command == "" {
			return apperrors.NewValidation("缺少 --command")
		}
		if rt.StrFirst("node", "doc") == "" {
			return apperrors.NewValidation("缺少 --node")
		}
		if command == "append" || command == "overwrite" || command == "block_insert_after" || command == "block_replace" {
			if rt.StrFirst("content", "text") == "" {
				return apperrors.NewValidation("该更新动作的 --content 不能为空")
			}
		}
		if strings.HasPrefix(command, "block_") && command != "block_insert_after" && rt.Str("block-id") == "" {
			return apperrors.NewValidation("该 block 操作必须提供 --block-id")
		}
		if command == "str_replace" && (rt.Str("old") == "" || !rt.Changed("new")) {
			return apperrors.NewValidation("--command str_replace 必须同时提供 --old 和 --new")
		}
		return nil
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"command", "content", "block-id", "old", "new"}, Description: "依 command 校验，所需文本参数不能为空"}},
	Execute:     executeUpdate,
}

var CheckpointUpdate = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+checkpoint-update",
	Product:     productDoc,
	Description: "先保存可回滚版本，再更新并读回验证",
	Intent:      "当用户要进行重要追加或整篇覆盖，并希望自动创建恢复点、执行更新、再读回确认时使用；任一步失败都会返回已经完成的步骤。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
	Contract: docContract("+checkpoint-update", "先保存可回滚版本，再更新并读回验证",
		"当用户要进行重要追加或整篇覆盖，并希望自动创建恢复点、执行更新、再读回确认时使用；任一步失败都会返回已经完成的步骤。",
		[]string{`dws doc +checkpoint-update --node <DOC_ID> --mode append --content @section.md`, `dws doc +checkpoint-update --node <DOC_ID> --mode overwrite --content @document.md`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "mode", Type: shortcut.FlagString, Default: "append", Desc: "更新模式", Enum: []string{"append", "overwrite"}},
		{Name: "content", Type: shortcut.FlagString, Desc: "内容字面量、@相对文件或 - 表示 stdin", Required: true},
	},
	Tips: []string{`dws doc +checkpoint-update --node <DOC_ID> --mode append --content @section.md`, `dws doc +checkpoint-update --node <DOC_ID> --mode overwrite --content @document.md`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		content, err := readShortcutContent(rt, "content")
		if err != nil {
			return err
		}
		plan := map[string]any{"nodeId": rt.Str("node"), "mode": rt.Str("mode"), "contentBytes": len(content), "steps": []string{"save_doc_version", "update_document", "get_document_content"}}
		if rt.DryRun() {
			plan["executed"] = false
			return rt.Output(docEnvelope("doc.checkpoint_update", plan))
		}
		steps := []map[string]any{}
		checkpoint, err := rt.CallMCPWriteData(productDoc, "save_doc_version", map[string]any{"nodeId": rt.Str("node")})
		if err != nil {
			return err
		}
		steps = append(steps, map[string]any{"name": "checkpoint", "status": "success"})
		if _, err := rt.CallMCPWriteData(productDoc, "update_document", map[string]any{"nodeId": rt.Str("node"), "markdown": content, "mode": rt.Str("mode")}); err != nil {
			return checkpointPartialWriteError(rt.Str("node"), checkpoint, "update", "doc_checkpoint_update_failed", err,
				append(steps, map[string]any{"name": "update", "status": "failed"}, map[string]any{"name": "verify", "status": "not_started"}))
		}
		steps = append(steps, map[string]any{"name": "update", "status": "success"})
		verified, err := rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": rt.Str("node"), "format": "markdown"})
		if err != nil {
			return checkpointPartialWriteError(rt.Str("node"), checkpoint, "verify", "doc_checkpoint_verification_failed", err,
				append(steps, map[string]any{"name": "verify", "status": "failed"}))
		}
		steps = append(steps, map[string]any{"name": "verify", "status": "success"})
		return rt.Output(docEnvelope("doc.checkpoint_update", map[string]any{"verified": verified}, steps...))
	},
}

var Export = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+export",
	Product:     productDoc,
	Description: "提交、轮询并安全下载在线文档导出文件",
	Intent:      "当用户要把在线文档导出成 docx、markdown 或 PDF 并保存到工作目录时使用；自动完成 job 提交、轮询与 no-clobber 原子下载。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+export", "提交、轮询并安全下载在线文档导出文件",
		"当用户要把在线文档导出成 docx、markdown 或 PDF 并保存到工作目录时使用；自动完成 job 提交、轮询与 no-clobber 原子下载。",
		[]string{`dws doc +export --node <DOC_ID> --export-format docx --output ./exports/`, `dws doc +export --node <DOC_ID> --export-format markdown --output ./document.md`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "export-format", Type: shortcut.FlagString, Default: "docx", Desc: "导出格式", Enum: []string{"docx", "markdown", "pdf"}},
		{Name: "output", Type: shortcut.FlagString, Default: ".", Desc: "工作目录内相对路径（文件或目录）"},
		{Name: "max-polls", Type: shortcut.FlagInt, Default: "30", Desc: "最大轮询次数"},
	},
	Tips:        []string{`dws doc +export --node <DOC_ID> --export-format docx --output ./exports/`, `dws doc +export --node <DOC_ID> --export-format markdown --output ./document.md`},
	Validate:    func(rt *shortcut.RuntimeContext) error { return localio.ValidateOutput(rt.Str("output")) },
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"output"}, Description: "--output 必须是工作目录内相对路径；默认 no-clobber"}},
	Execute:     executeExport,
}

var Import = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+import",
	Product:     productDoc,
	Description: "上传本地文件并等待转换成在线文档对象；白名单外格式自动改走文件上传原样入库",
	Intent:      "当用户要把工作区内的 doc/docx/xls/xlsx/md/txt/xmind/mark 文件导入为钉钉在线对象，并可指定目标文件夹或知识库时使用。白名单外格式（html/pdf 等）不报错，自动按原文件上传入库，结果带 fallback=upload、converted=false 标记。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
	Contract: docContract("+import", "上传本地文件并等待转换成在线文档对象；白名单外格式自动改走文件上传原样入库",
		"当用户要把工作区内的 doc/docx/xls/xlsx/md/txt/xmind/mark 文件导入为钉钉在线对象，并可指定目标文件夹或知识库时使用。白名单外格式（html/pdf 等）不报错，自动按原文件上传入库，结果带 fallback=upload、converted=false 标记。",
		[]string{`dws doc +import --file ./report.docx --folder <FOLDER_ID>`, `dws doc +import --file ./notes.md --workspace <WORKSPACE_ID> --name "会议纪要"`}),
	Flags: []shortcut.Flag{
		{Name: "file", Type: shortcut.FlagString, Desc: "本地文件路径", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
		{Name: "name", Type: shortcut.FlagString, Desc: "导入后名称"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintAtLeastOne, Flags: []string{"folder", "workspace"}, Description: "--folder 与 --workspace 至少提供一个导入目标"}},
	Tips:        []string{`dws doc +import --file ./report.docx --folder <FOLDER_ID>`, `dws doc +import --file ./notes.md --workspace <WORKSPACE_ID> --name "会议纪要"`},
	Execute:     func(rt *shortcut.RuntimeContext) error { return helpers.RunDocImportShortcut(rt.Command()) },
}

func executeUpdate(rt *shortcut.RuntimeContext) error {
	command := rt.Str("command")
	contentFlag := "content"
	if rt.Str("content") == "" && rt.Str("text") != "" {
		contentFlag = "text"
	}
	content, err := readShortcutContent(rt, contentFlag)
	if err != nil {
		return err
	}
	if rt.Str("doc-format") == "jsonml" && content != "" {
		content, err = validateJSONML(content)
		if err != nil {
			return err
		}
	}
	nodeID := rt.StrFirst("node", "doc")
	currentRevision := 0
	if rt.Changed("expected-revision") {
		current, revisionErr := rt.CallMCPData(productDoc, "get_document_content", map[string]any{"nodeId": nodeID, "format": "jsonml"})
		if revisionErr != nil {
			return revisionErr
		}
		var found bool
		currentRevision, found = nestedRevision(current)
		if !found {
			return apperrors.NewAPI("REVISION_CONFLICT: 服务响应缺少当前 revision，无法安全执行乐观更新")
		}
		if expected := rt.Int("expected-revision"); currentRevision != expected {
			return apperrors.NewValidation(fmt.Sprintf("REVISION_CONFLICT: 期望 revision %d，当前为 %d", expected, currentRevision))
		}
	}
	plan := map[string]any{"nodeId": nodeID, "command": command, "blockId": rt.Str("block-id"), "afterBlockId": rt.Str("after-block-id"), "contentBytes": len(content)}
	if rt.Changed("expected-revision") {
		plan["expectedRevision"] = rt.Int("expected-revision")
		plan["currentRevision"] = currentRevision
		plan["optimisticCheck"] = "best_effort"
	}
	if rt.DryRun() {
		plan["executed"] = false
		return rt.Output(docEnvelope("doc.update", plan))
	}
	node := nodeID
	switch command {
	case "append", "overwrite":
		params := map[string]any{"nodeId": node, "mode": command}
		if rt.Str("doc-format") == "jsonml" {
			if command == "append" {
				return apperrors.NewValidation("JSONML 当前不支持 append")
			}
			params["format"], params["jsonml"] = "jsonml", content
		} else {
			params["markdown"] = content
		}
		return rt.CallMCP("update_document", params)
	case "block_insert_after":
		params := map[string]any{"nodeId": node, "referenceBlockId": rt.Str("after-block-id"), "where": "after"}
		if rt.Str("doc-format") == "jsonml" {
			params["format"], params["jsonml"] = "jsonml", content
		} else {
			params["element"] = map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": content}}
		}
		return rt.CallMCP("insert_document_block", params)
	case "block_replace":
		params := map[string]any{"nodeId": node, "blockId": rt.Str("block-id")}
		if rt.Str("doc-format") == "jsonml" {
			params["format"], params["jsonml"] = "jsonml", content
		} else {
			params["element"] = map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": content}}
		}
		return rt.CallMCP("update_document_block", params)
	case "block_delete":
		return rt.CallMCP("delete_document_block", map[string]any{"nodeId": node, "blockId": rt.Str("block-id")})
	case "str_replace":
		return executePlainTextReplace(rt, node)
	case "block_copy_insert_after":
		return executeBlockCopy(rt, node)
	default:
		return apperrors.NewValidation(fmt.Sprintf("不支持的 update command %q", command))
	}
}

func nestedRevision(value any) (int, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if normalized == "revision" || normalized == "version" || normalized == "versionnumber" {
				switch number := child.(type) {
				case float64:
					if number >= 0 && number == float64(int(number)) {
						return int(number), true
					}
				case json.Number:
					parsed, err := number.Int64()
					if err == nil && parsed >= 0 {
						return int(parsed), true
					}
				case string:
					var parsed int
					if _, err := fmt.Sscan(strings.TrimSpace(number), &parsed); err == nil && parsed >= 0 {
						return parsed, true
					}
				}
			}
			if revision, ok := nestedRevision(child); ok {
				return revision, true
			}
		}
	case []any:
		for _, child := range typed {
			if revision, ok := nestedRevision(child); ok {
				return revision, true
			}
		}
	}
	return 0, false
}

func executePlainTextReplace(rt *shortcut.RuntimeContext, nodeID string) error {
	data, err := rt.CallMCPData(productDoc, "list_document_blocks", map[string]any{"nodeId": nodeID, "format": "element"})
	if err != nil {
		return err
	}
	oldText := rt.Str("old")
	type match struct{ blockID, text string }
	matches := []match{}
	var walk func(any, string)
	walk = func(value any, inheritedID string) {
		switch typed := value.(type) {
		case map[string]any:
			blockID := blockIdentity(typed, inheritedID)
			for key, child := range typed {
				if key == "text" {
					if text, ok := child.(string); ok && strings.Contains(text, oldText) && blockID != "" {
						matches = append(matches, match{blockID: blockID, text: text})
					}
				}
				walk(child, blockID)
			}
		case []any:
			for _, child := range typed {
				walk(child, inheritedID)
			}
		}
	}
	walk(data, "")
	if len(matches) != 1 {
		return apperrors.NewValidation(fmt.Sprintf("UNSAFE_RICH_TEXT_REPLACE: 需要唯一普通文本块匹配，实际 %d 处", len(matches)))
	}
	updated := strings.Replace(matches[0].text, oldText, rt.Str("new"), 1)
	return rt.CallMCP("update_document_block", map[string]any{"nodeId": nodeID, "blockId": matches[0].blockID, "element": map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": updated}}})
}

func executeBlockCopy(rt *shortcut.RuntimeContext, nodeID string) error {
	data, err := rt.CallMCPData(productDoc, "list_document_blocks", map[string]any{"nodeId": nodeID, "blockId": rt.Str("block-id"), "format": "element"})
	if err != nil {
		return err
	}
	block := findBlock(data, rt.Str("block-id"))
	if block == nil {
		return apperrors.NewValidation("DOCUMENT_NOT_FOUND: 未找到要复制的 block")
	}
	if containsResourceReference(block) {
		return apperrors.NewValidation("UNSUPPORTED_RESOURCE_TYPE: 含资源引用的 block 暂不支持复制")
	}
	stripBlockIDs(block)
	return rt.CallMCP("insert_document_block", map[string]any{"nodeId": nodeID, "referenceBlockId": rt.Str("after-block-id"), "where": "after", "element": block})
}

func executeExport(rt *shortcut.RuntimeContext) error {
	plan := map[string]any{"nodeId": rt.Str("node"), "exportFormat": rt.Str("export-format"), "output": rt.Str("output")}
	if rt.DryRun() {
		plan["executed"] = false
		plan["steps"] = []string{"submit_export_job", "query_export_job", "safe_atomic_download"}
		return rt.Output(docEnvelope("doc.export", plan))
	}
	submit, err := rt.CallMCPData(productDoc, "submit_export_job", map[string]any{"nodeId": rt.Str("node"), "exportFormat": rt.Str("export-format")})
	if err != nil {
		return err
	}
	jobID := nestedString(submit, "jobId", "jobID")
	if jobID == "" {
		return apperrors.NewAPI("导出任务响应缺少 jobId")
	}
	maxPolls := rt.Int("max-polls")
	if maxPolls <= 0 {
		maxPolls = 30
	}
	var query map[string]any
	for attempt := 1; attempt <= maxPolls; attempt++ {
		query, err = rt.CallMCPData(productDoc, "query_export_job", map[string]any{"jobId": jobID})
		if err != nil {
			return err
		}
		status := strings.ToUpper(nestedString(query, "status"))
		if status == "SUCCESS" {
			break
		}
		if status != "PROCESSING" {
			return apperrors.NewAPI(fmt.Sprintf("导出任务失败 (jobId=%s, status=%s): %s", jobID, status, nestedString(query, "message")))
		}
		if attempt == maxPolls {
			return apperrors.NewAPI(fmt.Sprintf("导出任务超时 (jobId=%s)，可用 doc +export-get 恢复查询", jobID))
		}
		timer := time.NewTimer(time.Duration(min(attempt, 5)) * time.Second)
		select {
		case <-rt.Command().Context().Done():
			timer.Stop()
			return rt.Command().Context().Err()
		case <-timer.C:
		}
	}
	downloadURL := nestedString(query, "downloadUrl", "resourceUrl")
	if downloadURL == "" {
		return apperrors.NewAPI(fmt.Sprintf("导出成功但响应缺少 downloadUrl (jobId=%s)", jobID))
	}
	cwd, err := docGetwd()
	if err != nil {
		return err
	}
	ext := map[string]string{"docx": ".docx", "markdown": ".md", "pdf": ".pdf"}[rt.Str("export-format")]
	preferred := "document" + ext
	result, err := docDownload(rt.Command().Context(), downloadURL, localio.DownloadOptions{BaseDir: cwd, Output: rt.Str("output"), PreferredName: preferred})
	if err != nil {
		return err
	}
	return rt.Output(docEnvelope("doc.export", map[string]any{"jobId": jobID, "localPath": result.RelativePath, "sizeBytes": result.SizeBytes},
		map[string]any{"name": "submit", "status": "success"}, map[string]any{"name": "poll", "status": "success"}, map[string]any{"name": "download", "status": "success"}))
}

func projectKeywordMatches(data map[string]any, rawQuery string, before, after int) map[string]any {
	queries := stringSliceNonEmpty(strings.Split(rawQuery, "|"))
	if before <= 0 {
		before = 80
	}
	if after <= 0 {
		after = 120
	}
	matches := []map[string]any{}
	appendTextMatch := func(text, blockID string) {
		textRunes := []rune(text)
		foldedText := foldRunes(textRunes)
		for _, query := range queries {
			foldedQuery := foldRunes([]rune(query))
			index := indexRunes(foldedText, foldedQuery)
			if index < 0 {
				continue
			}
			start, end := max(0, index-before), min(len(textRunes), index+len(foldedQuery)+after)
			matches = append(matches, map[string]any{
				"blockId": blockID, "topBlockId": blockID, "parentBlockPath": []string{},
				"content": string(textRunes[start:end]), "truncated": start > 0 || end < len(textRunes),
			})
			return
		}
	}
	var walk func(any, string)
	walk = func(value any, inheritedID string) {
		switch typed := value.(type) {
		case map[string]any:
			blockID := blockIdentity(typed, inheritedID)
			for key, child := range typed {
				if key == "jsonml" {
					if raw, ok := child.(string); ok {
						var decoded any
						if json.Unmarshal([]byte(raw), &decoded) == nil {
							walk(decoded, blockID)
							continue
						}
					}
				}
				if key == "text" {
					if text, ok := child.(string); ok {
						appendTextMatch(text, blockID)
						continue
					}
				}
				walk(child, blockID)
			}
		case []any:
			blockID := inheritedID
			start := 0
			if len(typed) >= 2 {
				if _, isTag := typed[0].(string); isTag {
					start = 2
					if attrs, ok := typed[1].(map[string]any); ok {
						blockID = blockIdentity(attrs, blockID)
					}
				}
			}
			for _, child := range typed[start:] {
				walk(child, blockID)
			}
		case string:
			appendTextMatch(typed, inheritedID)
		}
	}
	walk(data, "")
	return map[string]any{"count": len(matches), "matches": matches}
}

func foldRunes(value []rune) []rune {
	folded := make([]rune, len(value))
	for index, char := range value {
		folded[index] = unicode.ToLower(char)
	}
	return folded
}

func indexRunes(value, target []rune) int {
	if len(target) == 0 || len(target) > len(value) {
		return -1
	}
	for start := 0; start+len(target) <= len(value); start++ {
		matched := true
		for offset := range target {
			if value[start+offset] != target[offset] {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func checkpointPartialWriteError(nodeID string, checkpoint map[string]any, stage, reason string, cause error, steps []map[string]any) error {
	data := map[string]any{"nodeId": nodeID, "checkpointSaved": true}
	compensation := map[string]any{
		"available": true,
		"action":    "revert_to_checkpoint",
		"nodeId":    nodeID,
		"reason":    "a checkpoint was saved before the update started",
	}
	if version, ok := nestedRevision(checkpoint); ok {
		data["checkpointVersion"] = version
		compensation["version"] = version
	}
	return docPartialWriteError(
		"doc.checkpoint_update", reason, stage,
		fmt.Sprintf("checkpoint-update 在 %s 阶段失败；恢复点已保存，nodeId=%s，请勿直接重试整个复合命令", stage, nodeID),
		cause, data, steps, compensation,
	)
}

func findBlock(value any, target string) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if blockIdentity(typed, "") == target {
			copy := map[string]any{}
			for key, value := range typed {
				copy[key] = value
			}
			return copy
		}
		for _, child := range typed {
			if found := findBlock(child, target); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findBlock(child, target); found != nil {
				return found
			}
		}
	}
	return nil
}

func containsResourceReference(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if (key == "resourceId" || key == "resourceUrl" || key == "src") && fmt.Sprint(child) != "" {
				return true
			}
			if containsResourceReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsResourceReference(child) {
				return true
			}
		}
	}
	return false
}

func stripBlockIDs(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"blockId", "id", "uuid"} {
			delete(typed, key)
		}
		for _, child := range typed {
			stripBlockIDs(child)
		}
	case []any:
		for _, child := range typed {
			stripBlockIDs(child)
		}
	}
}

func init() {
	_ = json.Valid
	_ = filepath.Separator
	shortcut.Register(Create, Fetch, Inspect, Update, CheckpointUpdate, Export, Import)
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/docresolver"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	docGetwd        = os.Getwd
	docEvalSymlinks = filepath.EvalSymlinks
	docRel          = filepath.Rel
	docReadFile     = os.ReadFile
	docMkdirTemp    = os.MkdirTemp
	docRemoveAll    = os.RemoveAll
	docDownload     = localio.Download
	docVerifySleep  = time.Sleep
	docVerifyDelays = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	docMarkdown     = goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	docMarkdownConvert = func(source []byte, writer io.Writer) error {
		return docMarkdown.Convert(source, writer)
	}
)

const (
	docBlockReadPageSize = 50
	docBlockReadMaxItems = 5000
	docMarkdownVerifyMax = 2 * 1024 * 1024
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
		{Name: "content", Type: shortcut.FlagString, Desc: docContentInputDescription},
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
		contentChunks := []string{content}
		if format == "markdown" && content != "" {
			contentChunks = splitDocMarkdown(content, 10000)
			params["markdown"] = contentChunks[0]
		}
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.create", map[string]any{"executed": false, "previewKind": "plan", "create": params, "docFormat": format, "contentBytes": len(content)}))
		}
		created, err := rt.CallMCPWriteData(productDoc, "create_document", params)
		if err != nil {
			return docUnknownWriteError("doc.create", "create_document", "", err)
		}
		nodeID := nestedString(created, "nodeId", "documentId", "id")
		steps := []map[string]any{{"name": "create_document", "status": "success"}}
		if nodeID == "" {
			return docPartialWriteError(
				"doc.create", "doc_create_missing_node_id", "resolve_created_document",
				"创建文档成功但响应缺少 nodeId；无法验证新文档，请先在钉钉中定位，不要直接重试",
				nil,
				map[string]any{"nodeId": "", "docFormat": format, "verified": false},
				append(steps, map[string]any{"name": "verify", "status": "not_started"}),
				map[string]any{"available": false, "reason": "create_document did not return nodeId; locate the new document in DingTalk"},
			)
		}
		if format == "jsonml" && content != "" {
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
		if format == "markdown" && len(contentChunks) > 1 {
			for index, chunk := range contentChunks[1:] {
				stepName := fmt.Sprintf("append_chunk_%d", index+2)
				if _, err := rt.CallMCPWriteData(productDoc, "update_document", map[string]any{"nodeId": nodeID, "markdown": chunk, "mode": "append"}); err != nil {
					return docPartialWriteError(
						"doc.create", "doc_create_chunk_commit_unknown", stepName,
						fmt.Sprintf("文档已创建，但第 %d/%d 个内容分片失败或提交状态未知；请先回读，不要重试整个创建", index+2, len(contentChunks)),
						err,
						map[string]any{"nodeId": nodeID, "chunksWritten": index + 1, "chunksTotal": len(contentChunks), "verified": false},
						append(steps, map[string]any{"name": stepName, "status": "unknown"}),
						map[string]any{"available": false, "reason": "inspect the current document and resume only confirmed missing content"},
					)
				}
				steps = append(steps, map[string]any{"name": stepName, "status": "success"})
			}
		}
		verifyTool := "get_document_info"
		verifyParams := map[string]any{"nodeId": nodeID}
		if content != "" {
			verifyTool = "get_document_content"
			verifyParams["format"] = format
		}
		verification, err := readDocVerification(rt, verifyTool, verifyParams, func(data map[string]any) bool {
			return content == "" || verifyUpdatedDocumentContent(data, content, "overwrite", format)
		})
		if err != nil {
			return docVerificationError("doc.create", "verify", nodeID, err, append(steps, map[string]any{"name": "verify", "status": "failed"}))
		}
		if content != "" && !verifyUpdatedDocumentContent(verification, content, "overwrite", format) {
			return docVerificationError("doc.create", "verify", nodeID, fmt.Errorf("回读结果与完整初始内容不一致"), append(steps, map[string]any{"name": "verify", "status": "failed"}))
		}
		steps = append(steps, map[string]any{"name": "verify", "status": "success"})
		return rt.Output(docEnvelope("doc.create", map[string]any{"nodeId": nodeID, "result": created, "verified": true, "verification": verification}, steps...))
	},
}

const fetchTargetConstraint = "--node 与 --query 必须且只能提供一个"

var Fetch = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+fetch",
	Product:     productDoc,
	Description: "读取完整或局部文档内容，并按 detail 控制保真度",
	Intent:      "当用户要按 node/URL 直接读取在线文字文档，或只知道唯一标题并希望一次完成解析和读取时使用；支持 outline/range/section/keyword/tags 局部内容用于精确编辑和评论。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract(
		"+fetch", "读取完整或局部文档内容，并按 detail 控制保真度",
		"当用户要按 node/URL 直接读取在线文字文档，或只知道唯一标题并希望一次完成解析和读取时使用；支持 outline/range/section/keyword/tags 局部内容用于精确编辑和评论。",
		[]string{`dws doc +fetch --node <DOC_ID>`, `dws doc +fetch --query "项目周报" --scope keyword --keyword "结论"`},
		contract.ParamDecl{Name: "node", Property: "nodeId"},
		contract.ParamDecl{Name: "query", Property: "keyword"},
	),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL；" + fetchTargetConstraint},
		{Name: "query", Type: shortcut.FlagString, Desc: "文档标题或关键词；跨页唯一解析后读取；" + fetchTargetConstraint},
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
	Tips: []string{`dws doc +fetch --node <DOC_ID>`, `dws doc +fetch --query "项目周报" --scope keyword --keyword "结论"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		if rt.Changed("revision") {
			return apperrors.NewValidation("HISTORICAL_READ_UNSUPPORTED: 当前接口不能读取指定历史 revision")
		}
		if rt.Str("scope") == "keyword" && rt.Str("keyword") == "" {
			return apperrors.NewValidation("--scope keyword 时必须提供 --keyword")
		}
		return nil
	},
	Constraints: []shortcut.Constraint{
		{Kind: shortcut.ConstraintCustom, Flags: []string{"node", "query"}, Description: fetchTargetConstraint},
		{Kind: shortcut.ConstraintCustom, Flags: []string{"scope", "keyword"}, Description: "--scope keyword 时 --keyword 不能为空"},
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		target, err := docresolver.Resolve(rt, rt.Str("node"), rt.Str("query"))
		if err != nil {
			return err
		}
		format := "markdown"
		if rt.Str("detail") != "simple" || rt.Str("scope") != "full" {
			format = "jsonml"
		}
		params := map[string]any{"nodeId": target.Selected.CanonicalID, "format": format}
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
		content := any(data)
		if scope == "keyword" {
			content = projectKeywordMatches(data, rt.Str("keyword"), rt.Int("context-before"), rt.Int("context-after"))
		}
		return rt.Output(map[string]any{
			"contractVersion": "doc.content.v1",
			"status":          "success",
			"complete":        true,
			"target":          target.Selected,
			"content":         content,
		})
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
		steps := []map[string]any{{"name": "get_document_info", "status": "success"}}
		failures := []map[string]any{}
		for _, read := range reads {
			if !rt.Bool(read.flag) {
				continue
			}
			value, callErr := rt.CallMCPReadData(read.product, read.tool, read.params)
			if callErr != nil {
				failures = append(failures, map[string]any{"tool": read.tool, "error": callErr.Error()})
				steps = append(steps, map[string]any{"name": read.tool, "status": "failed"})
				continue
			}
			result[read.key] = value
			steps = append(steps, map[string]any{"name": read.tool, "status": "success"})
		}
		if len(failures) > 0 {
			return apperrors.NewAPI(
				"文档聚合检查只完成了部分读取",
				apperrors.WithOperation("doc.inspect"),
				apperrors.WithReason("doc_inspect_partial"),
				apperrors.WithFailureStage("optional_reads"),
				apperrors.WithExecutionStarted(false),
				apperrors.WithRetryable(true),
				apperrors.WithDetails(map[string]any{
					"contractVersion": "doc.operation.v1",
					"status":          "partial_success",
					"complete":        false,
					"data":            result,
					"steps":           steps,
					"failures":        failures,
				}),
			)
		}
		return rt.Output(docEnvelope("doc.inspect", result, steps...))
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
		contract.ParamDecl{Name: "node", Property: "node"},
		contract.ParamDecl{Name: "command", Property: "command"},
		contract.ParamDecl{Name: "content", Property: "content"},
		contract.ParamDecl{Name: "doc-format", Property: "docFormat"},
		contract.ParamDecl{Name: "block-id", Property: "blockId"},
		contract.ParamDecl{Name: "after-block-id", Property: "afterBlockId"},
		contract.ParamDecl{Name: "old", Property: "old"},
		contract.ParamDecl{Name: "new", Property: "new"},
		contract.ParamDecl{Name: "expected-revision", Property: "expectedRevision"},
		contract.ParamDecl{Name: "doc", Property: "node"},
		contract.ParamDecl{Name: "text", Property: "content"}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true, Aliases: []string{"doc"}, AliasesVisible: true},
		{Name: "command", Type: shortcut.FlagString, Desc: "更新动作；不能为空", Enum: []string{"append", "overwrite", "block_insert_after", "block_replace", "block_delete", "str_replace", "block_copy_insert_after"}},
		{Name: "content", Type: shortcut.FlagString, Desc: docRequiredContentInputDescription, Aliases: []string{"text"}, AliasesVisible: true},
		{Name: "doc-format", Type: shortcut.FlagString, Default: "markdown", Desc: "内容格式", Enum: []string{"markdown", "jsonml"}},
		{Name: "block-id", Type: shortcut.FlagString, Desc: "目标或源 block ID；相关动作要求时不能为空"},
		{Name: "after-block-id", Type: shortcut.FlagString, Desc: "插入位置参考 block ID；相关动作要求时不能为空"},
		{Name: "old", Type: shortcut.FlagString, Desc: "str_replace 原文字，不能为空"},
		{Name: "new", Type: shortcut.FlagString, Desc: "str_replace 新文字；--old 不能为空，新值可为空但参数必须显式提供"},
		{Name: "expected-revision", Type: shortcut.FlagInt, Desc: "仅 overwrite+jsonml：传给服务端执行原子 revision 条件写"},
	},
	Tips: []string{`dws doc +update --node <DOC_ID> --command append --content "补充说明"`, `dws doc +update --node <DOC_ID> --command block_replace --block-id <BLOCK_ID> --content "新内容"`},
	Validate: func(rt *shortcut.RuntimeContext) error {
		command := rt.Str("command")
		if rt.StrFirst("node", "doc") == "" {
			return apperrors.NewValidation("缺少 --node")
		}
		if command == "" {
			return apperrors.NewValidation("缺少 --command")
		}
		switch command {
		case "append", "overwrite", "block_insert_after", "block_replace":
			if rt.StrFirst("content", "text") == "" {
				return apperrors.NewValidation("该更新动作的 --content 不能为空")
			}
		}
		switch command {
		case "block_replace", "block_delete", "block_copy_insert_after":
			if rt.Str("block-id") == "" {
				return apperrors.NewValidation("该 block 操作必须提供 --block-id")
			}
		}
		switch command {
		case "block_insert_after", "block_copy_insert_after":
			if rt.Str("after-block-id") == "" {
				return apperrors.NewValidation("该 block 操作必须提供 --after-block-id")
			}
		}
		if command == "str_replace" && (rt.Str("old") == "" || !rt.Changed("new")) {
			return apperrors.NewValidation("--command str_replace 必须同时提供 --old 和 --new")
		}
		if command == "append" && rt.Str("doc-format") == "jsonml" {
			return apperrors.NewValidation("JSONML 当前不支持 append")
		}
		if rt.Changed("expected-revision") && (command != "overwrite" || rt.Str("doc-format") != "jsonml") {
			return apperrors.NewValidation("--expected-revision 仅支持 --command overwrite --doc-format jsonml；其他写入接口没有服务端原子 revision 契约")
		}
		return nil
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"command", "content", "block-id", "after-block-id", "old", "new"}, Description: "依 command 校验，所需文本或 block 参数不能为空"}},
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
		{Name: "content", Type: shortcut.FlagString, Desc: docContentInputDescription, Required: true},
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
		verification, err := readDocVerification(rt, "get_document_content", map[string]any{"nodeId": rt.Str("node"), "format": "markdown"}, func(data map[string]any) bool {
			return verifyUpdatedDocumentContent(data, content, rt.Str("mode"), "markdown")
		})
		if err != nil {
			return checkpointPartialWriteError(rt.Str("node"), checkpoint, "verify", "doc_checkpoint_verification_failed", err,
				append(steps, map[string]any{"name": "verify", "status": "failed"}))
		}
		if !verifyUpdatedDocumentContent(verification, content, rt.Str("mode"), "markdown") {
			return checkpointPartialWriteError(rt.Str("node"), checkpoint, "verify", "doc_checkpoint_verification_failed", fmt.Errorf("回读结果未匹配预期变更"),
				append(steps, map[string]any{"name": "verify", "status": "failed"}))
		}
		steps = append(steps, map[string]any{"name": "verify", "status": "success"})
		return rt.Output(docEnvelope("doc.checkpoint_update", map[string]any{"nodeId": rt.Str("node"), "verified": true, "verification": verification}, steps...))
	},
}

var Export = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+export",
	Product:     productDoc,
	Description: "提交、轮询并安全下载在线文档导出文件",
	Intent:      "当用户要把在线文档导出成 docx、markdown 或 PDF 并保存到工作目录时使用；自动完成 job 提交、轮询与 no-clobber 原子下载；失败后保留 jobId 通过 +export-get 恢复，不改用 curl 或本地生成。",
	Risk:        shortcut.RiskRead,
	Safety:      contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
	Contract: docContract("+export", "提交、轮询并安全下载在线文档导出文件",
		"当用户要把在线文档导出成 docx、markdown 或 PDF 并保存到工作目录时使用；自动完成 job 提交、轮询与 no-clobber 原子下载；失败后保留 jobId 通过 +export-get 恢复，不改用 curl 或本地生成。",
		[]string{`dws doc +export --node <DOC_ID> --export-format docx --output ./exports/`, `dws doc +export --node <DOC_ID> --export-format markdown --output ./document.md`}),
	Flags: []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "export-format", Type: shortcut.FlagString, Default: "docx", Desc: "导出格式；省略时默认为 docx，不能用全局 --format 代替", Enum: []string{"docx", "markdown", "pdf"}},
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
	Intent:      "当用户要把工作目录内的 doc/docx/xls/xlsx/md/txt/xmind/mark 相对路径文件导入为钉钉在线对象，并可指定目标文件夹或知识库时使用；白名单外格式（html/pdf 等）自动按原文件上传入库，结果带 fallback=upload、converted=false 标记。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
	Contract: docContract("+import", "上传本地文件并等待转换成在线文档对象；白名单外格式自动改走文件上传原样入库",
		"当用户要把工作目录内的 doc/docx/xls/xlsx/md/txt/xmind/mark 相对路径文件导入为钉钉在线对象，并可指定目标文件夹或知识库时使用；白名单外格式（html/pdf 等）自动按原文件上传入库，结果带 fallback=upload、converted=false 标记。",
		[]string{`dws doc +import --file ./report.docx`, `dws doc +import --file ./notes.md --workspace <WORKSPACE_ID> --name "会议纪要"`}),
	Flags: []shortcut.Flag{
		{Name: "file", Type: shortcut.FlagString, Desc: "工作目录内已存在文件的相对路径", Required: true},
		{Name: "folder", Type: shortcut.FlagString, Desc: "可选目标文件夹 ID；folder/workspace 都省略时导入默认根目录"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "可选目标知识库 ID；folder/workspace 都省略时导入默认根目录"},
		{Name: "name", Type: shortcut.FlagString, Desc: "导入后名称"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"file"}, Description: "--file 必须是工作目录内已存在且不通过符号链接逃逸的相对路径"}},
	Tips:        []string{`dws doc +import --file ./report.docx`, `dws doc +import --file ./notes.md --workspace <WORKSPACE_ID> --name "会议纪要"`},
	Validate:    func(rt *shortcut.RuntimeContext) error { return validateWorkspaceInputPath("file", rt.Str("file")) },
	Execute: func(rt *shortcut.RuntimeContext) error {
		if err := helpers.RunDocImportShortcut(rt.Command()); err != nil {
			return docUnknownWriteError("doc.import", "import", "", err)
		}
		return nil
	},
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
	plan := map[string]any{"nodeId": nodeID, "command": command, "blockId": rt.Str("block-id"), "afterBlockId": rt.Str("after-block-id"), "contentBytes": len(content)}
	if rt.Changed("expected-revision") {
		plan["expectedRevision"] = rt.Int("expected-revision")
		plan["optimisticCheck"] = "server_enforced"
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
			if rt.Changed("expected-revision") {
				params["revision"] = rt.Int("expected-revision")
			}
		} else {
			params["markdown"] = content
		}
		return executeVerifiedDocContentMutation(rt, params, node, content, command, rt.Str("doc-format"))
	case "block_insert_after":
		verificationFormat := blockVerificationFormat(rt.Str("doc-format"))
		params := map[string]any{"nodeId": node, "referenceBlockId": rt.Str("after-block-id"), "where": "after"}
		if rt.Str("doc-format") == "jsonml" {
			params["format"], params["jsonml"] = "jsonml", content
		} else {
			params["element"] = map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": content}}
		}
		referenceBlockID := rt.Str("after-block-id")
		return executeVerifiedDocMutation(rt, "doc.update", "insert_document_block", params, node,
			"list_document_blocks", map[string]any{"nodeId": node, "format": verificationFormat, "__allBlocks": true},
			func(result, data map[string]any) bool {
				return verifyInsertedBlock(result, data, referenceBlockID, content, rt.Str("doc-format"))
			})
	case "block_replace":
		blockID := rt.Str("block-id")
		verificationFormat := blockVerificationFormat(rt.Str("doc-format"))
		params := map[string]any{"nodeId": node, "blockId": rt.Str("block-id")}
		if rt.Str("doc-format") == "jsonml" {
			params["format"], params["jsonml"] = "jsonml", content
		} else {
			params["element"] = map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": content}}
		}
		return executeVerifiedDocMutation(rt, "doc.update", "update_document_block", params, node,
			"list_document_blocks", map[string]any{"nodeId": node, "format": verificationFormat, "__allBlocks": true},
			func(_, data map[string]any) bool {
				return blockContentEquals(data, blockID, content, rt.Str("doc-format"))
			})
	case "block_delete":
		blockID := rt.Str("block-id")
		return executeVerifiedDocMutation(rt, "doc.update", "delete_document_block", map[string]any{"nodeId": node, "blockId": blockID}, node,
			"list_document_blocks", map[string]any{"nodeId": node, "format": "element", "__allBlocks": true},
			func(_, data map[string]any) bool { return findBlock(data, blockID) == nil })
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
	data, err := readAllDocumentBlocks(rt, map[string]any{"nodeId": nodeID, "format": "element"})
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
	blockID := matches[0].blockID
	return executeVerifiedDocMutation(rt, "doc.update", "update_document_block",
		map[string]any{"nodeId": nodeID, "blockId": blockID, "element": map[string]any{"blockType": "paragraph", "paragraph": map[string]any{"text": updated}}}, nodeID,
		"list_document_blocks", map[string]any{"nodeId": nodeID, "format": "element", "__allBlocks": true},
		func(_, data map[string]any) bool { return blockContentEquals(data, blockID, updated, "markdown") })
}

func executeBlockCopy(rt *shortcut.RuntimeContext, nodeID string) error {
	data, err := readAllDocumentBlocks(rt, map[string]any{"nodeId": nodeID, "format": "element"})
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
	expectedContent := canonicalBlockContent(block, "markdown")
	stripBlockIDs(block)
	referenceBlockID := rt.Str("after-block-id")
	return executeVerifiedDocMutation(rt, "doc.update", "insert_document_block",
		map[string]any{"nodeId": nodeID, "referenceBlockId": referenceBlockID, "where": "after", "element": block}, nodeID,
		"list_document_blocks", map[string]any{"nodeId": nodeID, "format": "element", "__allBlocks": true},
		func(result, data map[string]any) bool {
			return verifyInsertedCanonicalBlock(result, data, referenceBlockID, expectedContent, "markdown")
		})
}

func executeVerifiedDocMutation(
	rt *shortcut.RuntimeContext,
	operation, tool string,
	params map[string]any,
	nodeID, verifyTool string,
	verifyParams map[string]any,
	verify func(map[string]any, map[string]any) bool,
) error {
	steps := []map[string]any{{"name": tool, "status": "started"}}
	result, err := rt.CallMCPWriteData(productDoc, tool, params)
	if err != nil {
		return docUnknownWriteError(operation, tool, nodeID, err)
	}
	steps[0]["status"] = "success"
	verification, err := readDocVerification(rt, verifyTool, verifyParams, func(data map[string]any) bool {
		return verify == nil || verify(result, data)
	})
	if err != nil {
		return docVerificationError(operation, "verify", nodeID, err, append(steps, map[string]any{"name": "verify", "status": "failed"}))
	}
	if verify != nil && !verify(result, verification) {
		return docVerificationError(operation, "verify", nodeID, fmt.Errorf("回读结果未匹配预期变更"), append(steps, map[string]any{"name": "verify", "status": "failed"}))
	}
	steps = append(steps, map[string]any{"name": "verify", "status": "success"})
	return rt.Output(docEnvelope(operation, map[string]any{
		"nodeId":       nodeID,
		"verified":     true,
		"result":       result,
		"verification": verification,
	}, steps...))
}

func executeVerifiedDocContentMutation(rt *shortcut.RuntimeContext, firstParams map[string]any, nodeID, content, mode, format string) error {
	chunks := []string{content}
	if format == "markdown" {
		chunks = splitDocMarkdown(content, 10000)
		firstParams["markdown"] = chunks[0]
	}
	steps := make([]map[string]any, 0, len(chunks)+1)
	for index, chunk := range chunks {
		params := firstParams
		if index > 0 {
			params = map[string]any{"nodeId": nodeID, "mode": "append", "markdown": chunk}
		}
		stepName := "update_document"
		if len(chunks) > 1 {
			stepName = fmt.Sprintf("write_chunk_%d", index+1)
		}
		result, err := rt.CallMCPWriteData(productDoc, "update_document", params)
		if err != nil {
			if index == 0 {
				return docUnknownWriteError("doc.update", stepName, nodeID, err)
			}
			return docPartialWriteError(
				"doc.update", "doc_update_chunk_commit_unknown", stepName,
				fmt.Sprintf("文档已写入 %d/%d 个分片，但当前分片失败或提交状态未知；请先回读，不要重放已完成分片", index, len(chunks)),
				err,
				map[string]any{"nodeId": nodeID, "mode": mode, "chunksWritten": index, "chunksTotal": len(chunks), "lastResult": result, "verified": false},
				append(steps, map[string]any{"name": stepName, "status": "unknown"}),
				map[string]any{"available": false, "reason": "inspect current content before resuming from a confirmed missing boundary"},
			)
		}
		steps = append(steps, map[string]any{"name": stepName, "status": "success"})
	}
	verification, err := readDocVerification(rt, "get_document_content", map[string]any{"nodeId": nodeID, "format": format}, func(data map[string]any) bool {
		return verifyUpdatedDocumentContent(data, content, mode, format)
	})
	if err != nil {
		return docVerificationError("doc.update", "verify", nodeID, err, append(steps, map[string]any{"name": "verify", "status": "failed"}))
	}
	if !verifyUpdatedDocumentContent(verification, content, mode, format) {
		return docVerificationError("doc.update", "verify", nodeID, fmt.Errorf("回读结果未包含预期内容"), append(steps, map[string]any{"name": "verify", "status": "failed"}))
	}
	steps = append(steps, map[string]any{"name": "verify", "status": "success"})
	return rt.Output(docEnvelope("doc.update", map[string]any{
		"nodeId": nodeID, "mode": mode, "chunksWritten": len(chunks), "verified": true, "verification": verification,
	}, steps...))
}

func readDocVerification(rt *shortcut.RuntimeContext, tool string, rawParams map[string]any, verify func(map[string]any) bool) (map[string]any, error) {
	params := cloneMap(rawParams)
	allBlocks, _ := params["__allBlocks"].(bool)
	delete(params, "__allBlocks")
	var last map[string]any
	var lastErr error
	for attempt := 0; attempt <= len(docVerifyDelays); attempt++ {
		var data map[string]any
		var err error
		if allBlocks && tool == "list_document_blocks" {
			data, err = readAllDocumentBlocks(rt, params)
		} else {
			data, err = rt.CallMCPData(productDoc, tool, params)
		}
		if err != nil {
			lastErr = err
		} else {
			last = data
			lastErr = nil
			if verify == nil || verify(data) {
				return data, nil
			}
		}
		if attempt < len(docVerifyDelays) {
			docVerifySleep(docVerifyDelays[attempt])
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return last, nil
}

func readAllDocumentBlocks(rt *shortcut.RuntimeContext, base map[string]any) (map[string]any, error) {
	all := make([]any, 0, docBlockReadPageSize)
	seenPages := map[string]bool{}
	for start := 0; start < docBlockReadMaxItems; start += docBlockReadPageSize {
		params := cloneMap(base)
		params["startIndex"] = start
		params["endIndex"] = start + docBlockReadPageSize - 1
		page, err := rt.CallMCPData(productDoc, "list_document_blocks", params)
		if err != nil {
			return nil, err
		}
		blocks, ok := documentBlockEntries(page)
		if !ok {
			return nil, fmt.Errorf("list_document_blocks 回读缺少 blocks 数组")
		}
		encoded, _ := json.Marshal(blocks)
		pageKey := string(encoded)
		if pageKey != "[]" && seenPages[pageKey] {
			return nil, fmt.Errorf("list_document_blocks 分页停滞，无法证明回读完整")
		}
		seenPages[pageKey] = true
		all = append(all, blocks...)
		hasMore, known, _ := docPageState(page)
		if known && !hasMore {
			return map[string]any{"blocks": all, "hasMore": false, "totalCount": len(all)}, nil
		}
		if total, ok := nestedNonNegativeInt(page, "totalCount", "total_count"); ok && len(all) >= total {
			return map[string]any{"blocks": all, "hasMore": false, "totalCount": total}, nil
		}
		if !known && len(blocks) < docBlockReadPageSize {
			return map[string]any{"blocks": all, "hasMore": false, "totalCount": len(all)}, nil
		}
		if len(blocks) == 0 {
			return nil, fmt.Errorf("list_document_blocks 声明仍有下一页但当前页为空，无法证明回读完整")
		}
	}
	return nil, fmt.Errorf("list_document_blocks 超过 %d 个块，无法在安全上限内完成回读", docBlockReadMaxItems)
}

func documentBlockEntries(value any) ([]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"blocks", "items"} {
			if blocks, ok := typed[key].([]any); ok {
				return blocks, true
			}
		}
		if encoded, ok := typed["jsonml"].(string); ok {
			var decoded any
			if json.Unmarshal([]byte(encoded), &decoded) == nil {
				blocks := orderedJSONMLBlocks(decoded)
				values := make([]any, len(blocks))
				for index := range blocks {
					values[index] = blocks[index]
				}
				return values, true
			}
		}
		for _, key := range []string{"result", "data"} {
			if nested, ok := typed[key]; ok {
				if blocks, found := documentBlockEntries(nested); found {
					return blocks, true
				}
			}
		}
	}
	return nil, false
}

func nestedNonNegativeInt(value any, keys ...string) (int, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, ok := typed[key]; ok {
				switch number := raw.(type) {
				case float64:
					if number >= 0 && number == float64(int(number)) {
						return int(number), true
					}
				case int:
					if number >= 0 {
						return number, true
					}
				}
			}
		}
		for _, key := range []string{"result", "data"} {
			if result, ok := nestedNonNegativeInt(typed[key], keys...); ok {
				return result, true
			}
		}
	}
	return 0, false
}

func splitDocMarkdown(content string, maxRunes int) []string {
	if maxRunes <= 0 {
		return []string{content}
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return []string{content}
	}
	chunks := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			end = len(runes)
		} else {
			for split := end; split > start; split-- {
				if runes[split-1] == '\n' {
					end = split
					break
				}
			}
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}

func containsText(value any, needle string) bool {
	if needle == "" {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, needle)
	case map[string]any:
		for _, child := range typed {
			if containsText(child, needle) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsText(child, needle) {
				return true
			}
		}
	}
	return false
}

func verifyUpdatedDocumentContent(value any, expected, mode, format string) bool {
	expectedRaw := expected
	expected = normalizeDocumentContentForVerification(expectedRaw, format)
	for _, candidate := range documentContentCandidates(value, format) {
		actualRaw := candidate
		actual := normalizeDocumentContentForVerification(actualRaw, format)
		if mode == "overwrite" {
			if actual == expected || (format == "markdown" && stripReadbackDocumentTitle(actual) == expected) {
				return true
			}
			if format == "markdown" && (markdownSemanticallyEquivalent(actualRaw, expectedRaw) ||
				markdownSemanticallyEquivalent(stripReadbackDocumentTitle(actualRaw), expectedRaw)) {
				return true
			}
			continue
		}
		if actual == expected || strings.HasSuffix(actual, "\n"+expected) {
			return true
		}
		if format == "markdown" && markdownSemanticallyEndsWith(actualRaw, expectedRaw) {
			return true
		}
	}
	return false
}

func markdownSemanticallyEquivalent(left, right string) bool {
	leftFingerprint, leftOK := markdownSemanticFingerprint(left)
	rightFingerprint, rightOK := markdownSemanticFingerprint(right)
	return leftOK && rightOK && leftFingerprint == rightFingerprint
}

func markdownSemanticallyEndsWith(content, suffix string) bool {
	contentFingerprint, contentOK := markdownSemanticFingerprint(content)
	suffixFingerprint, suffixOK := markdownSemanticFingerprint(suffix)
	return contentOK && suffixOK && strings.HasSuffix(contentFingerprint, suffixFingerprint)
}

func markdownSemanticFingerprint(source string) (string, bool) {
	if len(source) > docMarkdownVerifyMax {
		return "", false
	}
	var rendered bytes.Buffer
	if err := docMarkdownConvert([]byte(source), &rendered); err != nil {
		return "", false
	}
	return rendered.String(), true
}

func stripReadbackDocumentTitle(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		return content
	}
	lines = lines[1:]
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

func verifyInsertedBlock(result, data map[string]any, referenceBlockID, expected, format string) bool {
	return verifyInsertedCanonicalBlock(result, data, referenceBlockID, normalizeDocumentContentForVerification(expected, format), format)
}

func blockVerificationFormat(format string) string {
	if format == "jsonml" {
		return "jsonml"
	}
	return "element"
}

func verifyInsertedCanonicalBlock(result, data map[string]any, referenceBlockID, expected, format string) bool {
	if insertedID := nestedString(result, "blockId", "elementId", "id"); insertedID != "" {
		if blockContentEquals(data, insertedID, expected, format) {
			return true
		}
	}
	blocks := orderedCanonicalBlocks(data, format)
	for index, block := range blocks {
		if canonicalBlockIdentity(block, format) != referenceBlockID || index+1 >= len(blocks) {
			continue
		}
		return canonicalBlockContent(blocks[index+1], format) == expected
	}
	return false
}

func blockContentEquals(data map[string]any, blockID, expected, format string) bool {
	block := findCanonicalBlock(data, blockID, format)
	if block == nil {
		return false
	}
	return canonicalBlockContent(block, format) == normalizeDocumentContentForVerification(expected, format)
}

func canonicalBlockContent(value any, format string) string {
	if values, ok := value.(map[string]any); ok {
		if element, ok := values["element"].(map[string]any); ok {
			value = element
		}
	}
	if format == "jsonml" {
		if values, ok := value.(map[string]any); ok {
			if encoded, ok := values["jsonml"].(string); ok {
				return normalizeJSONMLForVerification(encoded)
			}
		}
		if encoded, ok := value.(string); ok {
			return normalizeJSONMLForVerification(encoded)
		}
		if encoded, err := json.Marshal(value); err == nil {
			return normalizeJSONMLForVerification(string(encoded))
		}
	}
	texts := make([]string, 0, 4)
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if text, ok := typed["text"].(string); ok {
				texts = append(texts, text)
				return
			}
			for key, child := range typed {
				if key == "id" || key == "blockId" || key == "uuid" || key == "blockType" {
					continue
				}
				walk(child)
			}
		case []any:
			start := 0
			if len(typed) > 0 {
				if _, isTag := typed[0].(string); isTag {
					start = 1
				}
			}
			for _, child := range typed[start:] {
				walk(child)
			}
		case string:
			texts = append(texts, typed)
		}
	}
	walk(value)
	return normalizeMarkdownForVerification(strings.Join(texts, "\n"))
}

func orderedCanonicalBlocks(value any, format string) []any {
	if format == "jsonml" {
		blocks := orderedJSONMLBlocks(value)
		result := make([]any, len(blocks))
		for index := range blocks {
			result[index] = blocks[index]
		}
		return result
	}
	blocks := orderedDocumentBlocks(value)
	result := make([]any, len(blocks))
	for index := range blocks {
		result[index] = blocks[index]
	}
	return result
}

func canonicalBlockIdentity(value any, format string) string {
	if format == "jsonml" {
		if element, ok := value.([]any); ok {
			return jsonMLBlockIdentity(element)
		}
		return ""
	}
	if block, ok := value.(map[string]any); ok {
		return blockIdentity(block, "")
	}
	return ""
}

func findCanonicalBlock(value any, target, format string) any {
	if format == "jsonml" {
		block := findJSONMLBlock(value, target)
		if block == nil {
			return nil
		}
		return block
	}
	block := findBlock(value, target)
	if block == nil {
		return nil
	}
	return block
}

func orderedJSONMLBlocks(value any) [][]any {
	blocks := [][]any{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if encoded, ok := child.(string); ok && isJSONMLPayloadKey(key) {
					var decoded any
					if json.Unmarshal([]byte(encoded), &decoded) == nil {
						walk(decoded)
					}
					continue
				}
				walk(child)
			}
		case []any:
			if jsonMLBlockIdentity(typed) != "" {
				blocks = append(blocks, typed)
			}
			start := 0
			if len(typed) > 0 {
				if _, ok := typed[0].(string); ok {
					start = 1
				}
			}
			for _, child := range typed[start:] {
				walk(child)
			}
		}
	}
	walk(value)
	return blocks
}

func findJSONMLBlock(value any, target string) []any {
	for _, block := range orderedJSONMLBlocks(value) {
		if jsonMLBlockIdentity(block) == target {
			return block
		}
	}
	return nil
}

func jsonMLBlockIdentity(element []any) string {
	if len(element) < 2 {
		return ""
	}
	attributes, ok := element[1].(map[string]any)
	if !ok {
		return ""
	}
	return nestedString(attributes, "uuid", "blockId", "elementId", "id")
}

func isJSONMLPayloadKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	return normalized == "jsonml" || normalized == "content"
}

func orderedDocumentBlocks(value any) []map[string]any {
	blocks := []map[string]any{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if element, ok := typed["element"].(map[string]any); ok && blockIdentity(element, "") != "" {
				blocks = append(blocks, element)
				return
			}
			if blockIdentity(typed, "") != "" {
				blocks = append(blocks, typed)
				return
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return blocks
}

func documentContentCandidates(value any, format string) []string {
	wanted := map[string]bool{"content": true}
	if format == "jsonml" {
		wanted["jsonml"] = true
	} else {
		wanted["markdown"] = true
	}
	var candidates []string
	var walk func(any, bool)
	walk = func(current any, root bool) {
		switch typed := current.(type) {
		case string:
			if root {
				candidates = append(candidates, typed)
			}
		case map[string]any:
			for key, child := range typed {
				normalizedKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
				if text, ok := child.(string); ok && wanted[normalizedKey] {
					candidates = append(candidates, text)
				}
				walk(child, false)
			}
		case []any:
			for _, child := range typed {
				walk(child, false)
			}
		}
	}
	walk(value, true)
	return candidates
}

func normalizeDocumentContentForVerification(raw, format string) string {
	if format == "jsonml" {
		return normalizeJSONMLForVerification(raw)
	}
	return normalizeMarkdownForVerification(raw)
}

func normalizeMarkdownForVerification(raw string) string {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	lines := make([]string, 0, strings.Count(raw, "\n")+1)
	inFence := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			lines = append(lines, trimmed)
			continue
		}
		if inFence {
			lines = append(lines, strings.TrimRight(line, " \t"))
			continue
		}
		line = trimmed
		if line == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			for index := range parts {
				parts[index] = strings.Join(strings.Fields(parts[index]), " ")
			}
			line = strings.Join(parts, "|")
		} else {
			line = strings.Join(strings.Fields(line), " ")
		}
		lines = append(lines, line)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func normalizeJSONMLForVerification(raw string) string {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return normalizeMarkdownForVerification(raw)
	}
	var normalize func(any) any
	normalize = func(current any) any {
		switch typed := current.(type) {
		case []any:
			if len(typed) == 0 {
				return []any{}
			}
			tag, isElement := typed[0].(string)
			if !isElement {
				out := make([]any, 0, len(typed))
				for _, child := range typed {
					out = append(out, normalize(child))
				}
				return out
			}
			start := 1
			attrs := map[string]any{}
			if len(typed) > 1 {
				if declared, ok := typed[1].(map[string]any); ok {
					attrs, _ = normalize(declared).(map[string]any)
					start = 2
				}
			}
			children := make([]any, 0, len(typed)-start)
			for _, child := range typed[start:] {
				normalized := normalize(child)
				if normalized != nil {
					children = append(children, normalized)
				}
			}
			if strings.EqualFold(tag, "span") && isGeneratedTextSpan(attrs) {
				if len(children) == 1 {
					return children[0]
				}
				return children
			}
			out := []any{strings.ToLower(tag), attrs}
			out = append(out, children...)
			return out
		case map[string]any:
			out := make(map[string]any, len(typed))
			for key, child := range typed {
				normalizedKey := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
				if normalizedKey == "uuid" || normalizedKey == "blockid" || normalizedKey == "elementid" || normalizedKey == "index" {
					continue
				}
				out[key] = normalize(child)
			}
			return out
		case string:
			return strings.ReplaceAll(strings.ReplaceAll(typed, "\r\n", "\n"), "\r", "\n")
		}
		return current
	}
	// normalize only receives values decoded by encoding/json, so the resulting
	// tree is always JSON-marshalable.
	encoded, _ := json.Marshal(normalize(value))
	return string(encoded)
}

func isGeneratedTextSpan(attrs map[string]any) bool {
	if len(attrs) == 0 {
		return true
	}
	if len(attrs) != 1 {
		return false
	}
	value, ok := attrs["data-type"].(string)
	return ok && (value == "text" || value == "leaf")
}

func executeExport(rt *shortcut.RuntimeContext) error {
	plan := map[string]any{"nodeId": rt.Str("node"), "exportFormat": rt.Str("export-format"), "output": rt.Str("output")}
	if rt.DryRun() {
		plan["executed"] = false
		plan["steps"] = []string{"submit_export_job", "query_export_job", "safe_atomic_download"}
		return rt.Output(docEnvelope("doc.export", plan))
	}
	submit, err := rt.CallMCPWriteData(productDoc, "submit_export_job", map[string]any{"nodeId": rt.Str("node"), "exportFormat": rt.Str("export-format")})
	if err != nil {
		return docUnknownWriteError("doc.export", "submit_export_job", rt.Str("node"), err)
	}
	jobID := nestedString(submit, "jobId", "jobID")
	if jobID == "" {
		return docExportRecoveryError("doc_export_missing_job_id", "submit", "导出任务已提交但响应缺少 jobId；禁止重新提交", "", nil)
	}
	maxPolls := rt.Int("max-polls")
	if maxPolls <= 0 {
		maxPolls = 30
	}
	var query map[string]any
	for attempt := 1; attempt <= maxPolls; attempt++ {
		query, err = rt.CallMCPData(productDoc, "query_export_job", map[string]any{"jobId": jobID})
		if err != nil {
			return docExportRecoveryError("doc_export_poll_failed", "poll", "导出任务轮询失败；请使用现有 jobId 恢复查询，不要重新提交", jobID, err)
		}
		status := strings.ToUpper(nestedString(query, "status"))
		if status == "SUCCESS" {
			break
		}
		if !docExportStatusPollable(status) {
			return docExportRecoveryError("doc_export_job_failed", "poll", fmt.Sprintf("导出任务失败 (status=%s): %s", status, nestedString(query, "message")), jobID, nil)
		}
		if attempt == maxPolls {
			return docExportRecoveryError("doc_export_poll_timeout", "poll", "导出任务仍在处理中；请使用现有 jobId 恢复查询，不要重新提交", jobID, nil)
		}
		timer := time.NewTimer(time.Duration(min(attempt, 5)) * time.Second)
		select {
		case <-rt.Command().Context().Done():
			timer.Stop()
			return docExportRecoveryError("doc_export_poll_cancelled", "poll", "导出等待被中断；请使用现有 jobId 恢复查询，不要重新提交", jobID, rt.Command().Context().Err())
		case <-timer.C:
		}
	}
	downloadURL := nestedString(query, "downloadUrl", "resourceUrl")
	if downloadURL == "" {
		return docExportRecoveryError("doc_export_missing_download_url", "download", "导出任务已完成但响应缺少下载地址；请使用现有 jobId 重新查询", jobID, nil)
	}
	cwd, err := docGetwd()
	if err != nil {
		return err
	}
	ext := map[string]string{"docx": ".docx", "markdown": ".md", "pdf": ".pdf"}[rt.Str("export-format")]
	preferred := "document" + ext
	result, err := docDownload(rt.Command().Context(), downloadURL, localio.DownloadOptions{BaseDir: cwd, Output: rt.Str("output"), PreferredName: preferred})
	if err != nil {
		return docExportRecoveryError("doc_export_download_failed", "download", fmt.Sprintf("导出任务已完成但安全下载失败（output=%s）", rt.Str("output")), jobID, err)
	}
	return rt.Output(docEnvelope("doc.export", map[string]any{"jobId": jobID, "localPath": result.RelativePath, "sizeBytes": result.SizeBytes},
		map[string]any{"name": "submit", "status": "success"}, map[string]any{"name": "poll", "status": "success"}, map[string]any{"name": "download", "status": "success"}))
}

func docExportStatusPollable(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "INIT", "PROCESSING":
		return true
	default:
		return false
	}
}

func executeExportGet(rt *shortcut.RuntimeContext) error {
	jobID := rt.Str("job-id")
	query, err := rt.CallMCPData(productDoc, "query_export_job", map[string]any{"jobId": jobID})
	if err != nil {
		return docExportRecoveryError("doc_export_query_failed", "query", "查询导出任务失败；保留 jobId 后停止", jobID, err)
	}
	status := strings.ToUpper(nestedString(query, "status"))
	if status == "" {
		status = "UNKNOWN"
	}
	if status == "FAILED" || status == "CANCELLED" {
		return docExportRecoveryError("doc_export_job_failed", "query", fmt.Sprintf("导出任务状态为 %s: %s", status, nestedString(query, "message")), jobID, nil)
	}
	if rt.Str("output") == "" || status != "SUCCESS" {
		return rt.Output(map[string]any{
			"contractVersion": "doc.operation.v1",
			"ok":              true,
			"status":          strings.ToLower(status),
			"complete":        status == "SUCCESS",
			"operation":       "doc.export_get",
			"data":            map[string]any{"jobId": jobID, "result": query},
		})
	}
	downloadURL := nestedString(query, "downloadUrl", "resourceUrl")
	if downloadURL == "" {
		return docExportRecoveryError("doc_export_missing_download_url", "download", "导出任务已完成但响应缺少下载地址", jobID, nil)
	}
	cwd, err := docGetwd()
	if err != nil {
		return err
	}
	result, err := docDownload(rt.Command().Context(), downloadURL, localio.DownloadOptions{
		BaseDir: cwd, Output: rt.Str("output"), PreferredName: "document",
	})
	if err != nil {
		return docExportRecoveryError("doc_export_download_failed", "download", fmt.Sprintf("导出任务已完成但安全下载失败（output=%s）", rt.Str("output")), jobID, err)
	}
	return rt.Output(docEnvelope("doc.export_get", map[string]any{
		"jobId": jobID, "localPath": result.RelativePath, "sizeBytes": result.SizeBytes, "verified": result.SizeBytes > 0,
	}, map[string]any{"name": "query", "status": "success"}, map[string]any{"name": "download", "status": "success"}))
}

func docExportRecoveryError(reason, stage, message, jobID string, cause error) error {
	status := "incomplete"
	if jobID == "" {
		status = "unknown"
	}
	details := map[string]any{
		"contractVersion": "doc.operation.v1",
		"status":          status,
		"jobId":           jobID,
		"stage":           stage,
	}
	options := []apperrors.Option{
		apperrors.WithOperation("doc.export"),
		apperrors.WithReason(reason),
		apperrors.WithFailureStage(stage),
		apperrors.WithRetryable(false),
		apperrors.WithDetails(details),
	}
	if jobID != "" {
		options = append(options,
			apperrors.WithExecutionStarted(true),
			apperrors.WithActions(
				fmt.Sprintf("dws doc +export-get --job-id %s", jobID),
				"需要保存文件时给 +export-get 追加工作目录内的 --output 相对路径",
				"不要重新提交导出，不要 curl 临时下载地址，不要安装本地转换依赖",
			),
		)
	} else {
		options = append(options,
			apperrors.WithExecutionStarted(true),
			apperrors.WithActions("先检查文档空间中是否已有导出任务；不要直接重新提交"),
		)
	}
	if cause != nil {
		options = append(options, apperrors.WithCause(cause))
	}
	return apperrors.NewAPI(message, options...)
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

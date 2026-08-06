// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var (
	legacyVersionSave   shortcut.Shortcut
	legacyVersionList   shortcut.Shortcut
	legacyVersionRevert shortcut.Shortcut
)

func canonicalizeHistoryShortcuts() {
	legacyVersionSave = VersionSave
	legacyVersionList = VersionList
	legacyVersionRevert = VersionRevert

	VersionSave.Command = "+history-save"
	VersionSave.Aliases = nil
	VersionSave.Description = "手动保存当前文档版本快照"
	VersionSave.Intent = "当用户要在重要修改前后手动建立一个可回滚的文档历史快照时使用；保存快照本身无需交互确认。"
	VersionSave.Safety = contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"}
	VersionSave.Contract = docContract("+history-save", VersionSave.Description, VersionSave.Intent, []string{`dws doc +history-save --node <DOC_ID>`})
	VersionSave.Tips = []string{`dws doc +history-save --node <DOC_ID>`}

	VersionList.Command = "+history-list"
	VersionList.Aliases = nil
	VersionList.Description = "分页列出文档历史版本"
	VersionList.Intent = "当用户要查看文档已有版本、选择回滚目标或审计版本时间线时使用；返回版本号和分页游标。"
	VersionList.Contract = docContract("+history-list", VersionList.Description, VersionList.Intent, []string{`dws doc +history-list --node <DOC_ID>`, `dws doc +history-list --node <DOC_ID> --page-size 20`})
	VersionList.Flags = []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "page-size", Type: shortcut.FlagInt, Desc: "每页版本数量"},
		{Name: "page-token", Type: shortcut.FlagString, Desc: "分页游标"},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "--page-size 的兼容别名"},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "--page-token 的兼容别名"},
	}
	VersionList.Tips = []string{`dws doc +history-list --node <DOC_ID>`, `dws doc +history-list --node <DOC_ID> --page-size 20`}
	VersionList.Execute = func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if size := rt.IntFirst("page-size", "limit"); size > 0 {
			params["maxResults"] = size
		}
		if token := rt.StrFirst("page-token", "cursor"); token != "" {
			params["nextCursor"] = token
		}
		return rt.CallMCP("list_doc_versions", params)
	}

	VersionRevert.Command = "+history-revert"
	VersionRevert.Aliases = nil
	VersionRevert.Description = "预检并回滚文档到指定历史版本"
	VersionRevert.Intent = "当用户明确要把整篇文档恢复到某个历史版本时使用；先确认目标版本存在，再执行高风险回滚并读回验证。"
	VersionRevert.Contract = docContract("+history-revert", VersionRevert.Description, VersionRevert.Intent, []string{`dws doc +history-revert --node <DOC_ID> --version 3`})
	VersionRevert.Tips = []string{`dws doc +history-revert --node <DOC_ID> --version 3`}
	VersionRevert.Execute = executeHistoryRevert

	TemplateList.Description = "浏览当前用户可用的 MY/PUBLIC 文档模板"
	TemplateList.Intent = "当用户要浏览自己的或公开的文档模板并获取 templateId 时使用；若已知名称可改用 template-search。"
	TemplateList.Contract = docContract("+template-list", TemplateList.Description, TemplateList.Intent, []string{`dws doc +template-list --source PUBLIC`})
	TemplateSearch.Description = "按名称检索文档模板"
	TemplateSearch.Intent = "当用户知道模板名称关键词、要快速定位唯一 templateId 后继续创建文档时使用。"
	TemplateSearch.Contract = docContract("+template-search", TemplateSearch.Description, TemplateSearch.Intent, []string{`dws doc +template-search --query "周报"`})
}

func canonicalizeCommentShortcuts() {
	// Preserve the historical confirmation contract for comment writes. The
	// richer canonical implementations must not silently weaken that gate.
	CommentCreate.Safety = contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"}
	CommentReply.Safety = contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"}
	CommentCreate.Description = "创建全文评论，或按 selection 创建划词评论"
	CommentCreate.Intent = "当用户要对整篇文档留言，或针对文档中唯一匹配的一段文字创建精确划词评论时使用；已知 block/start/end 时也可直接走高级通道。"
	CommentCreate.Contract = docContract("+comment-create", CommentCreate.Description, CommentCreate.Intent, []string{`dws doc +comment-create --node <DOC_ID> --content "请补充数据来源"`, `dws doc +comment-create --node <DOC_ID> --selection "计划下周发布" --content "请确认日期"`})
	CommentCreate.Flags = []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "content", Type: shortcut.FlagString, Desc: "评论文字内容", Required: true},
		{Name: "selection", Type: shortcut.FlagString, Desc: "完整文字或 前缀...后缀 selection；使用时不能为空且必须唯一匹配"},
		{Name: "block-id", Type: shortcut.FlagString, Desc: "高级通道 block ID；使用时不能为空且须与 start/end 一起提供"},
		{Name: "start", Type: shortcut.FlagInt, Desc: "块内起始字符偏移；高级通道参数不能为空且须一起提供"},
		{Name: "end", Type: shortcut.FlagInt, Desc: "块内结束字符偏移；高级通道参数不能为空且须一起提供"},
		{Name: "selected-text", Type: shortcut.FlagString, Desc: "高级通道引用原文"},
		{Name: "mention", Type: shortcut.FlagStringSlice, Desc: "被 @ 的用户 uid 列表"},
	}
	CommentCreate.Constraints = []shortcut.Constraint{{Kind: shortcut.ConstraintCustom, Flags: []string{"selection", "block-id", "start", "end"}, Description: "selection/高级通道参数不能为空；selection 必须唯一匹配，block-id/start/end 必须一起提供"}}
	CommentCreate.Validate = validateCommentCreate
	CommentCreate.Execute = executeCommentCreate
	CommentCreate.Tips = []string{`dws doc +comment-create --node <DOC_ID> --content "请补充数据来源"`, `dws doc +comment-create --node <DOC_ID> --selection "计划下周发布" --content "请确认日期"`}
}

func executeHistoryRevert(rt *shortcut.RuntimeContext) error {
	nodeID := rt.Str("node")
	target := rt.Int("version")
	versions, err := rt.CallMCPData(productDoc, "list_doc_versions", map[string]any{"nodeId": nodeID})
	if err != nil {
		return err
	}
	if !containsVersion(versions, target) {
		return apperrors.NewValidation(fmt.Sprintf("目标版本 %d 不存在，已停止回滚", target))
	}
	if rt.DryRun() {
		return rt.Output(docEnvelope("doc.history_revert", map[string]any{"executed": false, "nodeId": nodeID, "version": target, "preflight": "version_exists"}))
	}
	if _, err := rt.CallMCPWriteData(productDoc, "revert_doc_version", map[string]any{"nodeId": nodeID, "version": target}); err != nil {
		return err
	}
	current, err := rt.CallMCPData(productDoc, "get_document_info", map[string]any{"nodeId": nodeID})
	if err != nil {
		return docPartialWriteError(
			"doc.history_revert", "doc_history_revert_verification_failed", "verify", fmt.Sprintf("版本 %d 已回滚，但读回验证失败（nodeId=%s）；不要直接重试回滚", target, nodeID), err,
			map[string]any{"nodeId": nodeID, "version": target, "reverted": true},
			[]map[string]any{
				{"name": "preflight", "status": "success"},
				{"name": "revert", "status": "success"},
				{"name": "verify", "status": "failed"},
			},
			map[string]any{"available": false, "reason": "the requested revert completed; verify the current document before any further write"},
		)
	}
	return rt.Output(docEnvelope("doc.history_revert", map[string]any{"version": target, "current": current},
		map[string]any{"name": "preflight", "status": "success"},
		map[string]any{"name": "revert", "status": "success"},
		map[string]any{"name": "verify", "status": "success"}))
}

func containsVersion(value any, target int) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if normalized == "version" || normalized == "versionnumber" || normalized == "revision" {
				switch number := child.(type) {
				case float64:
					if int(number) == target && number == float64(target) {
						return true
					}
				case string:
					parsed, err := strconv.Atoi(strings.TrimSpace(number))
					if err == nil && parsed == target {
						return true
					}
				}
			}
			if containsVersion(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsVersion(child, target) {
				return true
			}
		}
	}
	return false
}

var CreateFromTemplate = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+create-from-template",
	Product:     productDoc,
	Description: "按 templateId 直达或搜索消歧后创建文档",
	Intent:      "当用户要基于文档模板创建新文档时使用；可直接给 template-id，或给 query 搜索且只在唯一命中时继续创建。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
	Contract: docContract("+create-from-template", "按 templateId 直达或搜索消歧后创建文档",
		"当用户要基于文档模板创建新文档时使用；可直接给 template-id，或给 query 搜索且只在唯一命中时继续创建。",
		[]string{`dws doc +create-from-template --template-id <TEMPLATE_ID> --name "我的周报"`, `dws doc +create-from-template --query "会议纪要" --name "项目例会"`}),
	Flags: []shortcut.Flag{
		{Name: "template-id", Type: shortcut.FlagString, Desc: "模板 ID"},
		{Name: "query", Type: shortcut.FlagString, Desc: "模板搜索名称"},
		{Name: "source", Type: shortcut.FlagString, Desc: "模板来源", Enum: []string{"MY", "PUBLIC"}},
		{Name: "name", Type: shortcut.FlagString, Desc: "新文档名称"},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"template-id", "query"}, Description: "--template-id 与 --query 必须且只能提供一个"}},
	Tips:        []string{`dws doc +create-from-template --template-id <TEMPLATE_ID> --name "我的周报"`, `dws doc +create-from-template --query "会议纪要" --name "项目例会"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		templateID := rt.Str("template-id")
		if templateID == "" {
			params := map[string]any{"searchName": rt.Str("query")}
			if rt.Str("source") != "" {
				params["templateSource"] = rt.Str("source")
			}
			found, err := rt.CallMCPData(productDoc, "search_doc_templates", params)
			if err != nil {
				return err
			}
			ids := collectTemplateIDs(found)
			if len(ids) != 1 {
				return apperrors.NewValidation(fmt.Sprintf("模板搜索需要唯一命中，实际 %d 个候选: %v", len(ids), ids))
			}
			templateID = ids[0]
		}
		params := map[string]any{"templateId": templateID}
		for flag, property := range map[string]string{"name": "name", "folder": "folderId", "workspace": "workspaceId"} {
			if value := rt.Str(flag); value != "" {
				params[property] = value
			}
		}
		if rt.DryRun() {
			return rt.Output(docEnvelope("doc.create_from_template", map[string]any{"executed": false, "params": params}))
		}
		result, err := rt.CallMCPWriteData(productDoc, "apply_doc_template", params)
		if err != nil {
			return err
		}
		return rt.Output(docEnvelope("doc.create_from_template", result, map[string]any{"name": "apply_template", "status": "success"}))
	},
}

func collectTemplateIDs(value any) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if strings.EqualFold(key, "templateId") || strings.EqualFold(key, "template_id") {
					if id, ok := child.(string); ok && strings.TrimSpace(id) != "" && !seen[id] {
						seen[id] = true
						out = append(out, id)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func init() {
	shortcut.Register(CreateFromTemplate)
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var (
	compatHistorySave   shortcut.Shortcut
	compatHistoryList   shortcut.Shortcut
	compatHistoryRevert shortcut.Shortcut
)

func canonicalizeHistoryShortcuts() {
	VersionSave.Command = "+version-save"
	VersionSave.Aliases = nil
	VersionSave.Description = "手动保存当前文档版本快照"
	VersionSave.Intent = "当用户要求保存、创建或建立当前文档版本快照时使用；只保存快照，不更新正文。"
	VersionSave.Safety = contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"}
	VersionSave.Contract = versionSaveContract()
	VersionSave.Tips = []string{`dws doc +version-save --node <DOC_ID>`}

	VersionList.Command = "+version-list"
	VersionList.Aliases = nil
	VersionList.Description = "分页列出文档历史版本"
	VersionList.Intent = "当用户要查看文档已有版本、选择回滚目标或审计版本时间线时使用；返回版本号和分页游标。"
	VersionList.Contract = versionListContract()
	VersionList.Flags = []shortcut.Flag{
		{Name: "node", Type: shortcut.FlagString, Desc: "文档 ID 或 URL", Required: true},
		{Name: "limit", Type: shortcut.FlagInt, Desc: "返回版本数量上限", Aliases: []string{"page-size"}},
		{Name: "cursor", Type: shortcut.FlagString, Desc: "分页游标", Aliases: []string{"page-token"}},
	}
	VersionList.Tips = []string{`dws doc +version-list --node <DOC_ID>`, `dws doc +version-list --node <DOC_ID> --limit 20`}
	VersionList.Execute = func(rt *shortcut.RuntimeContext) error {
		params := map[string]any{"nodeId": rt.Str("node")}
		if size := rt.IntFirst("limit", "page-size"); size > 0 {
			params["maxResults"] = size
		}
		if token := rt.StrFirst("cursor", "page-token"); token != "" {
			params["nextCursor"] = token
		}
		return rt.CallMCP("list_doc_versions", params)
	}

	VersionRevert.Command = "+version-revert"
	VersionRevert.Aliases = nil
	VersionRevert.Description = "预检并回滚文档到指定历史版本"
	VersionRevert.Intent = "当用户明确要把整篇文档恢复到某个历史版本时使用；先确认目标版本存在，再执行高风险回滚并读回验证。"
	VersionRevert.Contract = versionRevertContract()
	VersionRevert.Tips = []string{`dws doc +version-revert --node <DOC_ID> --version 3`}
	VersionRevert.Execute = executeHistoryRevert

	compatHistorySave = compatibilityHistoryShortcut(VersionSave, "+history-save", "+version-save")
	compatHistoryList = compatibilityHistoryShortcut(VersionList, "+history-list", "+version-list")
	compatHistoryRevert = compatibilityHistoryShortcut(VersionRevert, "+history-revert", "+version-revert")

	TemplateList.Description = "浏览当前用户可用的 MY/PUBLIC 文档模板"
	TemplateList.Intent = "当用户没有明确模板名称或关键词，只要浏览自己的或公开模板并获取 templateId 时使用。"
	TemplateList.Contract = templateListContract()
	TemplateSearch.Description = "按名称或关键词检索文档模板"
	TemplateSearch.Intent = "当用户已提供明确模板名称或关键词时使用；返回结构化候选和 resolved/not_found/selection_required 状态，零命中或多候选时停止创建。"
	TemplateSearch.Contract = templateSearchContract()
}

func versionSaveContract() corecmd.ContractDecl {
	decl := docContract("+version-save", VersionSave.Description, VersionSave.Intent, []string{`dws doc +version-save --node <DOC_ID>`})
	decl.Selection.AvoidWhen = []string{
		"只查看历史版本时使用 doc +version-list；需要恢复历史内容时使用 doc +version-revert",
		"需要保存恢复点后继续修改正文时使用 doc +checkpoint-update",
	}
	return decl
}

func versionListContract() corecmd.ContractDecl {
	decl := docContract("+version-list", VersionList.Description, VersionList.Intent, []string{`dws doc +version-list --node <DOC_ID>`, `dws doc +version-list --node <DOC_ID> --limit 20`})
	decl.Selection.AvoidWhen = []string{
		"需要创建当前版本快照时使用 doc +version-save",
		"已经明确目标版本并要求恢复正文时使用 doc +version-revert",
	}
	return decl
}

func versionRevertContract() corecmd.ContractDecl {
	decl := docContract("+version-revert", VersionRevert.Description, VersionRevert.Intent, []string{`dws doc +version-revert --node <DOC_ID> --version 3`})
	decl.Selection.AvoidWhen = []string{
		"只需要创建当前版本快照时使用 doc +version-save",
		"尚未确定目标版本号时先使用 doc +version-list",
	}
	return decl
}

func compatibilityHistoryShortcut(source shortcut.Shortcut, command, canonical string) shortcut.Shortcut {
	result := source
	result.Command = command
	result.Aliases = nil
	result.Description = fmt.Sprintf("兼容入口：%s", source.Description)
	result.Intent = fmt.Sprintf("仅当调用方明确使用既有 doc %s 路径时兼容执行。", command)
	result.Contract = docContract(command, result.Description, result.Intent, []string{fmt.Sprintf("dws doc %s --node <DOC_ID>", command)})
	result.Contract.Selection.AvoidWhen = []string{fmt.Sprintf("新的 Agent 任务统一使用 doc %s；不要因自然语言版本意图选择本兼容入口", canonical)}
	result.Tips = []string{fmt.Sprintf("dws doc %s --node <DOC_ID>", command)}
	if command == "+history-revert" {
		result.Contract.Selection.Examples = []string{`dws doc +history-revert --node <DOC_ID> --version 3`}
		result.Tips = []string{`dws doc +history-revert --node <DOC_ID> --version 3`}
	}
	return result
}

func templateSearchContract() corecmd.ContractDecl {
	decl := docContract("+template-search", TemplateSearch.Description, TemplateSearch.Intent, []string{`dws doc +template-search --query "周报"`})
	decl.Selection.AvoidWhen = []string{
		"没有明确模板名称或关键词、只是浏览可用模板时使用 doc +template-list",
		"已经拿到唯一 templateId 时不要重复搜索，直接使用 doc +create-from-template --template-id",
		"不要为了预览多个候选而逐个创建文档；多候选必须让用户选择",
	}
	return decl
}

func templateListContract() corecmd.ContractDecl {
	decl := docContract("+template-list", TemplateList.Description, TemplateList.Intent, []string{`dws doc +template-list --source PUBLIC`})
	decl.Selection.AvoidWhen = []string{
		"已经有明确模板名称或关键词时使用 doc +template-search --query",
		"已经拿到 templateId 且要创建文档时使用 doc +create-from-template --template-id",
	}
	return decl
}

func createFromTemplateContract() corecmd.ContractDecl {
	decl := docContract("+create-from-template", "使用已选定的 templateId 创建文档",
		"当模板搜索已经唯一解析或用户明确提供 templateId 时使用；只创建一次并返回稳定 nodeId。",
		[]string{`dws doc +create-from-template --template-id <TEMPLATE_ID> --name "我的周报"`})
	decl.Selection.AvoidWhen = []string{
		"只有模板名称或关键词时先使用 doc +template-search；零命中或多候选时停止并让用户选择",
		"没有模板名称、关键词或 templateId，只想浏览可用模板时使用 doc +template-list",
		"不要通过创建多个候选文档来预览模板，也不要反复改写关键词自动重试",
	}
	return decl
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
	Description: "使用已选定的 templateId 创建文档",
	Intent:      "当模板搜索已经唯一解析或用户明确提供 templateId 时使用；只创建一次并返回稳定 nodeId。",
	Risk:        shortcut.RiskWrite,
	Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "unknown"},
	Contract:    createFromTemplateContract(),
	Flags: []shortcut.Flag{
		{Name: "template-id", Type: shortcut.FlagString, Desc: "模板 ID"},
		{Name: "query", Type: shortcut.FlagString, Desc: "兼容入口：先搜索且仅唯一命中时创建；新的 Agent 流程应先调用 +template-search"},
		{Name: "source", Type: shortcut.FlagString, Desc: "模板来源", Enum: []string{"MY", "PUBLIC"}},
		{Name: "name", Type: shortcut.FlagString, Desc: "新文档名称"},
		{Name: "folder", Type: shortcut.FlagString, Desc: "目标文件夹 ID"},
		{Name: "workspace", Type: shortcut.FlagString, Desc: "目标知识库 ID"},
	},
	Constraints: []shortcut.Constraint{{Kind: shortcut.ConstraintExactlyOne, Flags: []string{"template-id", "query"}, Description: "--template-id 与 --query 必须且只能提供一个"}},
	Tips:        []string{`dws doc +create-from-template --template-id <TEMPLATE_ID> --name "我的周报"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		templateID := rt.Str("template-id")
		if templateID == "" {
			candidates, err := searchTemplateCandidatesForCreate(rt, rt.Str("query"), rt.Str("source"))
			if err != nil {
				return err
			}
			if len(candidates) != 1 {
				status := "selection_required"
				message := fmt.Sprintf("模板搜索返回 %d 个候选，必须先选择唯一 templateId", len(candidates))
				actions := []string{"运行 dws doc +template-search 查看结构化候选", "让用户选择后使用 --template-id 创建一次"}
				if len(candidates) == 0 {
					status = "not_found"
					message = "模板搜索没有命中；已停止创建"
					actions = []string{"检查模板来源或请用户提供更准确的模板名", "不要自动变换关键词循环搜索"}
				}
				return apperrors.NewValidation(message,
					apperrors.WithOperation("doc.create_from_template"),
					apperrors.WithReason("template_"+status),
					apperrors.WithExecutionStarted(false),
					apperrors.WithRetryable(false),
					apperrors.WithActions(actions...),
					apperrors.WithDetails(map[string]any{
						"contractVersion": "doc.template-selection.v1",
						"status":          status,
						"query":           rt.Str("query"),
						"source":          rt.Str("source"),
						"candidates":      candidates,
					}),
				)
			}
			templateID = candidates[0]["templateId"].(string)
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
			return docUnknownWriteError("doc.create_from_template", "apply_template", "", err)
		}
		return rt.Output(docEnvelope("doc.create_from_template", map[string]any{"templateId": templateID, "result": result}, map[string]any{"name": "apply_template", "status": "success"}))
	},
}

func searchTemplateCandidatesForCreate(rt *shortcut.RuntimeContext, query, source string) ([]map[string]any, error) {
	const (
		pageSize = 2
		maxPages = 20
	)
	cursor := ""
	seenCursors := map[string]bool{}
	seenCandidates := map[string]bool{}
	candidates := make([]map[string]any, 0, pageSize)
	for page := 1; page <= maxPages; page++ {
		params := map[string]any{"searchName": query, "maxResults": pageSize}
		if source != "" {
			params["templateSource"] = source
		}
		if cursor != "" {
			params["nextCursor"] = cursor
		}
		found, err := rt.CallMCPData(productDoc, "search_doc_templates", params)
		if err != nil {
			return nil, err
		}
		pageCandidates := collectTemplateCandidates(found)
		for _, candidate := range pageCandidates {
			id, _ := candidate["templateId"].(string)
			if id == "" || seenCandidates[id] {
				continue
			}
			seenCandidates[id] = true
			candidates = append(candidates, candidate)
		}

		hasMore, hasMoreKnown, next := docPageState(found)
		next = strings.TrimSpace(next)
		if len(candidates) > 1 {
			return sortedTemplateCandidates(candidates), nil
		}
		complete := hasMoreKnown && !hasMore
		if !hasMoreKnown && next == "" && len(pageCandidates) < pageSize {
			complete = true
		}
		if complete {
			return sortedTemplateCandidates(candidates), nil
		}
		if next == "" {
			return nil, templatePaginationError("missing_next_cursor", page, cursor, candidates)
		}
		if next == cursor || seenCursors[next] {
			return nil, templatePaginationError("stalled_cursor", page, next, candidates)
		}
		seenCursors[next] = true
		cursor = next
	}
	return nil, templatePaginationError("max_pages", maxPages, cursor, candidates)
}

func templatePaginationError(reason string, page int, cursor string, candidates []map[string]any) error {
	return apperrors.NewAPI(
		"模板搜索分页未完整结束；为避免基于非唯一候选创建文档，已停止执行",
		apperrors.WithOperation("doc.create_from_template"),
		apperrors.WithReason("template_pagination_"+reason),
		apperrors.WithFailureStage("template_search"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithRetryable(false),
		apperrors.WithActions("使用 dws doc +template-search 分页查看候选", "选择唯一 templateId 后重新执行创建"),
		apperrors.WithDetails(map[string]any{
			"contractVersion": "doc.template-selection.v1",
			"status":          "pagination_incomplete",
			"reason":          reason,
			"page":            page,
			"nextCursor":      cursor,
			"candidates":      sortedTemplateCandidates(candidates),
		}),
	)
}

func sortedTemplateCandidates(candidates []map[string]any) []map[string]any {
	out := append([]map[string]any(nil), candidates...)
	sort.SliceStable(out, func(i, j int) bool {
		leftName, _ := out[i]["name"].(string)
		rightName, _ := out[j]["name"].(string)
		if leftName != rightName {
			return leftName < rightName
		}
		leftID, _ := out[i]["templateId"].(string)
		rightID, _ := out[j]["templateId"].(string)
		return leftID < rightID
	})
	return out
}

func collectTemplateCandidates(value any) []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			id := directTemplateString(typed, "templateId", "template_id")
			if id != "" && !seen[id] {
				seen[id] = true
				candidate := map[string]any{"templateId": id}
				if name := directTemplateString(typed, "templateName", "name", "title"); name != "" {
					candidate["name"] = name
				}
				if source := directTemplateString(typed, "templateSource", "source"); source != "" {
					candidate["source"] = source
				}
				if description := directTemplateString(typed, "description", "summary"); description != "" {
					candidate["description"] = description
				}
				out = append(out, candidate)
			}
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return sortedTemplateCandidates(out)
}

func directTemplateString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
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

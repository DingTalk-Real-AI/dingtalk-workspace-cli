// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func newWhiteboardTemplateCommand() *cobra.Command {
	root := newDeepGroupCommand(&cobra.Command{
		Use: "template", Short: "独立白板模板管理", RunE: groupRunE,
	})
	personal := newDeepGroupCommand(&cobra.Command{
		Use: "personal", Short: "个人独立白板模板", RunE: groupRunE,
	})
	team := newDeepGroupCommand(&cobra.Command{
		Use: "team", Short: "团队独立白板模板", RunE: groupRunE,
	})
	personal.AddCommand(
		newWhiteboardTemplateSaveCommand(whiteboardcore.TemplateScopePersonal),
		newWhiteboardTemplateListCommand(whiteboardcore.TemplateScopePersonal),
		newWhiteboardTemplateCreateCommand(whiteboardcore.TemplateScopePersonal),
	)
	team.AddCommand(
		newWhiteboardTemplateSaveCommand(whiteboardcore.TemplateScopeTeam),
		newWhiteboardTemplateListCommand(whiteboardcore.TemplateScopeTeam),
		newWhiteboardTemplateCreateCommand(whiteboardcore.TemplateScopeTeam),
	)
	root.AddCommand(personal, team)
	return root
}

func newWhiteboardTemplateSaveCommand(scope whiteboardcore.TemplateScope) *cobra.Command {
	tool := whiteboardcore.PersonalTemplateSaveTool
	if scope == whiteboardcore.TemplateScopeTeam {
		tool = whiteboardcore.TeamTemplateSaveTool
	}
	path := "whiteboard template " + string(scope) + " save"
	spec := LeafSpec{
		Use: "save", Short: "保存独立白板为" + whiteboardTemplateScopeLabel(scope) + "模板",
		Long:    "将一份独立 .adraw 白板保存为" + whiteboardTemplateScopeLabel(scope) + "模板。类型固定为 DRAW(9)，不支持文档内嵌白板。执行前先预览并取得用户确认。",
		Example: "  dws " + path + " --node <WHITEBOARD_NODE_ID> --name \"项目复盘模板\" --request-id wb-tpl-save-001 --format json",
		Server:  whiteboardServerID, Tool: tool, OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "node", Usage: "源独立白板节点 ID/URL（必填）", Bind: "sourceNodeId", Required: true, MarkRequired: true, Trim: true},
			{Name: "name", Usage: "模板名称（必填，允许重名）", Bind: "name", Required: true, MarkRequired: true, Trim: true},
			{Name: "language", Usage: "模板语言，如 zh_CN、en_US；未传或空白默认 zh_CN", Bind: "language", Default: "zh_CN", ArgDefault: "zh_CN", Transform: func(raw string) (any, error) {
				value := strings.TrimSpace(raw)
				if value == "" {
					value = "zh_CN"
				}
				return value, nil
			}},
			{Name: "request-id", Usage: "1-128 字符稳定幂等请求 ID（必填）", Bind: "requestId", Required: true, MarkRequired: true, Trim: true, Transform: validateWhiteboardTemplateRequestID},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "whiteboard", Name: tool, CanonicalPath: "whiteboard." + tool, CLIPath: path, PrimaryCLIPath: path},
			Description: "经用户确认后把独立白板保存为" + whiteboardTemplateScopeLabel(scope) + "模板",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: true},
			Interface:   whiteboardTemplateCompositeInterface(tool),
			Selection: contract.SelectionSpec{
				AgentSummary: "把已有独立白板保存为" + whiteboardTemplateScopeLabel(scope) + "模板",
				UseWhen:      []string{"用户要把已有独立 .adraw 白板保存为" + whiteboardTemplateScopeLabel(scope) + "模板时"},
				AvoidWhen:    []string{"文档内嵌白板不支持直接保存模板；查询模板使用同 scope 的 list；从模板创建白板使用 create"},
				Examples:     []string{"dws " + path + " --node <WHITEBOARD_NODE_ID> --name \"项目复盘模板\" --request-id wb-tpl-save-001 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "sourceNodeId", Required: boolPtr(true)},
				{Name: "name", Property: "name", Required: boolPtr(true)},
				{Name: "language", Property: "language", Required: boolPtr(false)},
				{Name: "request-id", Property: "requestId", Required: boolPtr(true)},
			},
			Result: whiteboardTemplateSaveResultSpec(),
		},
		ResultCall: whiteboardTemplateResultCall,
	}
	if scope == whiteboardcore.TemplateScopeTeam {
		spec.Flags = append(spec.Flags, LeafFlag{Name: "template-workspace", Usage: "目标团队知识库加密 ID；未传默认源白板所属知识库，需要目标知识库编辑权限", Bind: "templateWorkspaceId", Trim: true, OmitEmpty: true})
		spec.Contract.Parameters = append(spec.Contract.Parameters, contract.ParamDecl{Name: "template-workspace", Property: "templateWorkspaceId", Required: boolPtr(false)})
	}
	return NewLeafCommand(spec)
}

func newWhiteboardTemplateListCommand(scope whiteboardcore.TemplateScope) *cobra.Command {
	tool := whiteboardcore.PersonalTemplateListTool
	if scope == whiteboardcore.TemplateScopeTeam {
		tool = whiteboardcore.TeamTemplateListTool
	}
	path := "whiteboard template " + string(scope) + " list"
	flags := []LeafFlag{
		// Preserve an explicit unfiltered query on the wire instead of
		// silently dropping the caller's empty string.
		{Name: "query", Usage: "可选模板名称关键词；提供时在当前 scope 内搜索", Bind: "query", Trim: true},
		{Name: "limit", Usage: "单页返回数量，默认 20，最大 50", Kind: LeafInt, Bind: "maxResults", Default: "20", ArgDefault: "20"},
		{Name: "cursor", Usage: "分页游标；首次不传，续页原样回填 next_token", Bind: "nextCursor", Trim: true, OmitEmpty: true},
		{Name: "page-all", Usage: "有界读取全部分页", Kind: LeafBool, Bind: "pageAll"},
		{Name: "max-pages", Usage: "--page-all 最大页数，默认 10，最大 100", Kind: LeafInt, Bind: "maxPages", Default: "10", ArgDefault: "10"},
	}
	params := []contract.ParamDecl{
		{Name: "query", Property: "query", Required: boolPtr(false)},
		{Name: "limit", Property: "maxResults", Required: boolPtr(false)},
		{Name: "cursor", Property: "nextCursor", Required: boolPtr(false)},
		{Name: "page-all", Required: boolPtr(false)},
		{Name: "max-pages", Required: boolPtr(false)},
	}
	if scope == whiteboardcore.TemplateScopeTeam {
		flags = append([]LeafFlag{{Name: "template-workspace", Usage: "团队模板所属 Workspace ID（必填）", Bind: "templateWorkspaceId", Required: true, MarkRequired: true, Trim: true}}, flags...)
		params = append([]contract.ParamDecl{{Name: "template-workspace", Property: "templateWorkspaceId", Required: boolPtr(true)}}, params...)
	}
	return NewLeafCommand(LeafSpec{
		Use: "list", Short: "查询" + whiteboardTemplateScopeLabel(scope) + "独立白板模板",
		Long:    "分页列出" + whiteboardTemplateScopeLabel(scope) + "独立白板模板；提供 --query 时在相同 scope 内按名称搜索，不跨 scope 回退。",
		Example: "  dws " + path + whiteboardTemplateWorkspaceExample(scope) + " --limit 20 --format json",
		Server:  whiteboardServerID, Tool: tool, OutputRollout: output.RolloutUnifiedActive,
		Flags:    flags,
		Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Validate: validateWhiteboardTemplateList,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "whiteboard", Name: tool, CanonicalPath: "whiteboard." + tool, CLIPath: path, PrimaryCLIPath: path},
			Description: "分页查询" + whiteboardTemplateScopeLabel(scope) + "独立白板模板",
			Interface:   whiteboardTemplateCompositeInterface(tool),
			Selection: contract.SelectionSpec{
				AgentSummary: "浏览或按关键词查询" + whiteboardTemplateScopeLabel(scope) + "独立白板模板",
				UseWhen:      []string{"需要取得" + whiteboardTemplateScopeLabel(scope) + "白板模板 templateId，或按名称查找模板时"},
				AvoidWhen:    []string{"保存模板使用 save；已有 templateId 并要新建白板时使用 create；不得改查另一个 scope"},
				Examples:     []string{"dws " + path + whiteboardTemplateWorkspaceExample(scope) + " --limit 20 --format json"},
			},
			Parameters: params,
			Result:     whiteboardTemplateListResultSpec(),
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
		},
		ResultCall: whiteboardTemplateResultCall,
	})
}

func newWhiteboardTemplateCreateCommand(scope whiteboardcore.TemplateScope) *cobra.Command {
	tool := whiteboardcore.PersonalTemplateCreateTool
	if scope == whiteboardcore.TemplateScopeTeam {
		tool = whiteboardcore.TeamTemplateCreateTool
	}
	path := "whiteboard template " + string(scope) + " create"
	flags := []LeafFlag{
		{Name: "template-id", Usage: "白板模板稳定 ID（必填）", Bind: "templateId", Required: true, MarkRequired: true, Trim: true},
		{Name: "name", Usage: "新白板名称；未提供时使用模板标题", Bind: "name", Trim: true, OmitEmpty: true},
		{Name: "folder", Usage: "新白板目标文件夹节点 ID/URL", Bind: "folderId", Trim: true, OmitEmpty: true},
		{Name: "workspace", Usage: "新白板目标知识库 ID/URL", Bind: "workspaceId", Trim: true, OmitEmpty: true},
		{Name: "request-id", Usage: "1-128 字符稳定幂等请求 ID（必填）", Bind: "requestId", Required: true, MarkRequired: true, Trim: true, Transform: validateWhiteboardTemplateRequestID},
	}
	params := []contract.ParamDecl{
		{Name: "template-id", Property: "templateId", Required: boolPtr(true)},
		{Name: "name", Property: "name", Required: boolPtr(false)},
		{Name: "folder", Property: "folderId", Required: boolPtr(false)},
		{Name: "workspace", Property: "workspaceId", Required: boolPtr(false)},
		{Name: "request-id", Property: "requestId", Required: boolPtr(true)},
	}
	if scope == whiteboardcore.TemplateScopeTeam {
		flags = append([]LeafFlag{{Name: "template-workspace", Usage: "团队模板所属 Workspace ID（必填）", Bind: "templateWorkspaceId", Required: true, MarkRequired: true, Trim: true}}, flags...)
		params = append([]contract.ParamDecl{{Name: "template-workspace", Property: "templateWorkspaceId", Required: boolPtr(true)}}, params...)
	}
	return NewLeafCommand(LeafSpec{
		Use: "create", Short: "从" + whiteboardTemplateScopeLabel(scope) + "模板创建独立白板",
		Long:    "从指定" + whiteboardTemplateScopeLabel(scope) + " DRAW(9) 模板幂等创建独立 .adraw 白板；不会回退到公开模板或另一 scope。",
		Example: "  dws " + path + whiteboardTemplateWorkspaceExample(scope) + " --template-id <TEMPLATE_ID> --name \"项目复盘\" --request-id wb-tpl-create-001 --format json",
		Server:  whiteboardServerID, Tool: tool, OutputRollout: output.RolloutUnifiedActive,
		Flags:       flags,
		Constraints: []LeafConstraint{{Kind: LeafMutuallyExclusive, Flags: []string{"folder", "workspace"}}},
		Safety:      contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "whiteboard", Name: tool, CanonicalPath: "whiteboard." + tool, CLIPath: path, PrimaryCLIPath: path},
			Description: "从" + whiteboardTemplateScopeLabel(scope) + "模板幂等创建独立白板",
			DryRun:      &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: true},
			Interface:   whiteboardTemplateCompositeInterface(tool),
			Selection: contract.SelectionSpec{
				AgentSummary: "使用明确的" + whiteboardTemplateScopeLabel(scope) + "模板创建一份新独立白板",
				UseWhen:      []string{"已有当前 scope 的 templateId，需要在文件夹、知识库或我的文档创建独立白板时"},
				AvoidWhen:    []string{"还没有 templateId 时先使用同 scope 的 list；创建空白白板使用普通文件创建；不得跨 scope 猜测模板"},
				Examples:     []string{"dws " + path + whiteboardTemplateWorkspaceExample(scope) + " --template-id <TEMPLATE_ID> --name \"项目复盘\" --request-id wb-tpl-create-001 --format json"},
			},
			Parameters: params,
			Result:     whiteboardTemplateCreateResultSpec(),
		},
		ResultCall: whiteboardTemplateResultCall,
	})
}

func whiteboardTemplateScopeLabel(scope whiteboardcore.TemplateScope) string {
	if scope == whiteboardcore.TemplateScopeTeam {
		return "团队"
	}
	return "个人"
}

func whiteboardTemplateWorkspaceExample(scope whiteboardcore.TemplateScope) string {
	if scope == whiteboardcore.TemplateScopeTeam {
		return " --template-workspace <TEAM_WORKSPACE_ID>"
	}
	return ""
}

func whiteboardTemplateCompositeInterface(tool string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
		Reason: "CLI 固定白板类型和模板 scope，执行 dry-run/分页/回执校验后调用 whiteboard 服务的 " + tool}
}

func validateWhiteboardTemplateRequestID(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if err := whiteboardcore.ValidateTemplateRequestID(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func validateWhiteboardTemplateList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if err := whiteboardcore.ValidateTemplatePageSize(limit); err != nil {
		return err
	}
	cursor, _ := cmd.Flags().GetString("cursor")
	if err := whiteboardcore.ValidateTemplateCursor(cursor); err != nil {
		return err
	}
	maxPages, _ := cmd.Flags().GetInt("max-pages")
	if err := whiteboardcore.ValidateTemplateMaxPages(maxPages); err != nil {
		return err
	}
	if cmd.Flags().Changed("max-pages") {
		pageAll, _ := cmd.Flags().GetBool("page-all")
		if !pageAll {
			return fmt.Errorf("--max-pages 只能与 --page-all 一起使用")
		}
	}
	return nil
}

func whiteboardTemplateResultCall(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if tool == whiteboardcore.PersonalTemplateListTool || tool == whiteboardcore.TeamTemplateListTool {
		return callWhiteboardTemplateListResult(cmd, tool, args)
	}
	callArgs := cloneWhiteboardTemplateArgs(args)
	if deps.Caller.DryRun() {
		callArgs["dryRun"] = true
		response, err := callWhiteboardTemplatePreview(cmd, tool, callArgs)
		if err != nil {
			return nil, err
		}
		result := unwrapWhiteboardResult(response)
		if result == nil {
			return nil, invalidWhiteboardTemplateResult(tool, whiteboardTemplateDiagnostic(fmt.Errorf("dry-run response must be an object"), response, result))
		}
		if executed, present := result["executed"]; !present || executed != false {
			return nil, invalidWhiteboardTemplateResult(tool, whiteboardTemplateDiagnostic(fmt.Errorf("dry-run response executed must be present and false"), response, result))
		}
		if err := validateWhiteboardTemplateDryRunResult(result, args, tool); err != nil {
			return nil, invalidWhiteboardTemplateResult(tool, whiteboardTemplateDiagnostic(err, response, result))
		}
		return output.Success(result, output.WithDryRun()), nil
	}
	response, err := callWhiteboardToolResult(cmd, tool, callArgs)
	if err != nil {
		return nil, err
	}
	result := unwrapWhiteboardResult(response)
	switch tool {
	case whiteboardcore.PersonalTemplateSaveTool, whiteboardcore.TeamTemplateSaveTool:
		if err := validateWhiteboardTemplateSaveResult(result, args, tool); err != nil {
			return nil, invalidWhiteboardTemplateResult(tool, err)
		}
	case whiteboardcore.PersonalTemplateCreateTool, whiteboardcore.TeamTemplateCreateTool:
		if err := validateStandaloneWhiteboardCreateResponse(response, whiteboardString(args["requestId"])); err != nil {
			return nil, err
		}
		if err := validateWhiteboardTemplateCreateResult(result, args, tool); err != nil {
			return nil, invalidWhiteboardTemplateResult(tool, err)
		}
	}
	return output.Success(result), nil
}

func validateWhiteboardTemplateDryRunResult(result map[string]any, args map[string]any, tool string) error {
	if err := requireWhiteboardTemplateType(result["resourceType"], result["type"]); err != nil {
		return err
	}
	wantScope := whiteboardTemplateScopeForTool(tool)
	gotScope := strings.TrimSpace(whiteboardString(result["scope"]))
	if tool == whiteboardcore.PersonalTemplateCreateTool || tool == whiteboardcore.TeamTemplateCreateTool {
		gotScope = strings.TrimSpace(whiteboardString(result["templateScope"]))
	}
	if gotScope != wantScope {
		return fmt.Errorf("dry-run scope %q does not match %q", gotScope, wantScope)
	}
	if wantScope == "team" {
		got := strings.TrimSpace(whiteboardString(result["templateWorkspaceId"]))
		want := strings.TrimSpace(whiteboardString(args["templateWorkspaceId"]))
		// Omitted team-save destination defaults to the source; explicit destinations must match.
		if got == "" || (want != "" && got != want) {
			return fmt.Errorf("dry-run templateWorkspaceId %q does not match the request", got)
		}
	}
	return nil
}

func callWhiteboardTemplateListResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	pageAll, _ := args["pageAll"].(bool)
	maxPages := whiteboardTemplateInt(args["maxPages"], 10)
	cursor := strings.TrimSpace(whiteboardString(args["nextCursor"]))
	all := make([]any, 0)
	pages := 0
	exhausted := false
	for {
		request := cloneWhiteboardTemplateArgs(args)
		delete(request, "pageAll")
		delete(request, "maxPages")
		if cursor == "" {
			delete(request, "nextCursor")
		} else {
			request["nextCursor"] = cursor
		}
		response, err := callWhiteboardToolResult(cmd, tool, request)
		if err != nil {
			return nil, err
		}
		result := unwrapWhiteboardResult(response)
		items, next, hasMore, err := validateWhiteboardTemplatePage(result, tool, args)
		if err != nil {
			return nil, invalidWhiteboardTemplateResult(tool, err)
		}
		all = append(all, items...)
		pages++
		exhausted = !hasMore
		if exhausted || !pageAll {
			cursor = next
			break
		}
		if next == "" || next == cursor {
			return nil, invalidWhiteboardTemplateResult(tool, fmt.Errorf("pagination did not advance"))
		}
		if pages >= maxPages {
			return nil, invalidWhiteboardTemplateResult(tool, fmt.Errorf("分页在 --max-pages=%d 内未耗尽，nextCursor=%s", maxPages, next))
		}
		cursor = next
	}
	data := map[string]any{"scope": whiteboardTemplateScopeForTool(tool), "templates": all}
	if query := strings.TrimSpace(whiteboardString(args["query"])); query != "" {
		data["query"] = query
	}
	meta := &output.Meta{Count: output.NewCount(len(all)), Pagination: &output.Pagination{
		EndpointExhausted: exhausted, Pages: pages, Items: len(all), NextToken: cursor,
	}}
	return output.Success(data, output.WithMeta(meta)), nil
}

func validateWhiteboardTemplatePage(result map[string]any, tool string, args map[string]any) ([]any, string, bool, error) {
	if result == nil {
		return nil, "", false, fmt.Errorf("response result must be an object")
	}
	rawItems, ok := result["templates"]
	if !ok {
		rawItems = result["items"]
	}
	items, ok := rawItems.([]any)
	if !ok {
		return nil, "", false, fmt.Errorf("response templates must be an array")
	}
	scope := whiteboardTemplateScopeForTool(tool)
	wantWorkspace := strings.TrimSpace(whiteboardString(args["templateWorkspaceId"]))
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, "", false, fmt.Errorf("templates[%d] must be an object", i)
		}
		if strings.TrimSpace(whiteboardString(item["templateId"])) == "" {
			return nil, "", false, fmt.Errorf("templates[%d] missing templateId", i)
		}
		if err := requireWhiteboardTemplateType(item["resourceType"], item["type"]); err != nil {
			return nil, "", false, fmt.Errorf("templates[%d]: %w", i, err)
		}
		if got := strings.TrimSpace(whiteboardString(item["scope"])); got != "" && got != scope {
			return nil, "", false, fmt.Errorf("templates[%d] scope %q does not match %q", i, got, scope)
		}
		if scope == "team" {
			got := strings.TrimSpace(whiteboardString(item["templateWorkspaceId"]))
			if got != "" && got != wantWorkspace {
				return nil, "", false, fmt.Errorf("templates[%d] workspace %q does not match request", i, got)
			}
		}
	}
	next := strings.TrimSpace(whiteboardString(result["nextCursor"]))
	hasMore, _ := result["hasMore"].(bool)
	if hasMore && next == "" {
		return nil, "", false, fmt.Errorf("hasMore=true requires nextCursor")
	}
	return items, next, hasMore, nil
}

func validateWhiteboardTemplateSaveResult(result map[string]any, args map[string]any, tool string) error {
	if result == nil {
		return fmt.Errorf("response result must be an object")
	}
	if strings.TrimSpace(whiteboardString(result["templateId"])) == "" {
		return fmt.Errorf("response missing templateId")
	}
	if got, want := strings.TrimSpace(whiteboardString(result["requestId"])), strings.TrimSpace(whiteboardString(args["requestId"])); got == "" || got != want {
		return fmt.Errorf("response requestId %q does not match request %q", got, want)
	}
	if err := requireWhiteboardTemplateType(result["resourceType"], result["type"]); err != nil {
		return err
	}
	wantScope := whiteboardTemplateScopeForTool(tool)
	if got := strings.TrimSpace(whiteboardString(result["scope"])); got != wantScope {
		return fmt.Errorf("response scope %q does not match %q", got, wantScope)
	}
	if wantScope == "team" && strings.TrimSpace(whiteboardString(result["templateWorkspaceId"])) == "" {
		return fmt.Errorf("team save response missing templateWorkspaceId")
	}
	if wantScope == "team" {
		want := strings.TrimSpace(whiteboardString(args["templateWorkspaceId"]))
		got := strings.TrimSpace(whiteboardString(result["templateWorkspaceId"]))
		if want != "" && got != want {
			return fmt.Errorf("response templateWorkspaceId %q does not match request %q", got, want)
		}
	}
	return nil
}

func validateWhiteboardTemplateCreateResult(result map[string]any, args map[string]any, tool string) error {
	if result == nil {
		return fmt.Errorf("response result must be an object")
	}
	if got, want := strings.TrimSpace(whiteboardString(result["templateId"])), strings.TrimSpace(whiteboardString(args["templateId"])); got == "" || got != want {
		return fmt.Errorf("response templateId %q does not match request %q", got, want)
	}
	if err := requireWhiteboardTemplateType(result["resourceType"], result["type"]); err != nil {
		return err
	}
	wantScope := whiteboardTemplateScopeForTool(tool)
	if got := strings.TrimSpace(whiteboardString(result["templateScope"])); got != wantScope {
		return fmt.Errorf("response templateScope %q does not match %q", got, wantScope)
	}
	if wantScope == "team" {
		got := strings.TrimSpace(whiteboardString(result["templateWorkspaceId"]))
		want := strings.TrimSpace(whiteboardString(args["templateWorkspaceId"]))
		if got == "" || got != want {
			return fmt.Errorf("response templateWorkspaceId %q does not match request %q", got, want)
		}
	}
	if raw, exists := result["verified"]; exists {
		verified, ok := raw.(bool)
		if !ok || !verified {
			return fmt.Errorf("verified must be true when present")
		}
	}
	return nil
}

func requireWhiteboardTemplateType(values ...any) error {
	for _, raw := range values {
		if raw == nil {
			continue
		}
		if whiteboardTemplateInt(raw, -1) == whiteboardcore.TemplateType {
			return nil
		}
		return fmt.Errorf("resourceType must be %d", whiteboardcore.TemplateType)
	}
	return fmt.Errorf("response missing resourceType")
}

func whiteboardTemplateInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func whiteboardTemplateScopeForTool(tool string) string {
	if tool == whiteboardcore.TeamTemplateSaveTool || tool == whiteboardcore.TeamTemplateListTool || tool == whiteboardcore.TeamTemplateCreateTool {
		return "team"
	}
	return "personal"
}

func cloneWhiteboardTemplateArgs(args map[string]any) map[string]any {
	copy := make(map[string]any, len(args))
	for key, value := range args {
		copy[key] = value
	}
	return copy
}

// Report only known field types, never raw payloads, board content or credentials.
func whiteboardTemplateDiagnostic(err error, response, result map[string]any) error {
	return fmt.Errorf("%w; response types: result=%T resultJson=%T data=%T; business types: executed=%T dryRun=%T scope=%T resourceType=%T success=%T", err,
		response["result"], response["resultJson"], response["data"],
		result["executed"], result["dryRun"], result["scope"], result["resourceType"], result["success"])
}

func invalidWhiteboardTemplateResult(tool string, err error) error {
	return &CLIError{Code: CodeMCPToolError, Message: "白板模板服务返回了不符合约定的结果",
		Suggestion: "不要跨个人/团队 scope 重试；保留 request-id、响应和 trace 信息排查",
		Operation:  whiteboardServerID + "/" + tool, Cause: err}
}

func whiteboardTemplateSaveResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"独立白板保存模板结果","properties":{"templateId":{"type":"string","description":"新模板稳定 ID"},"name":{"type":"string","description":"模板名称"},"scope":{"type":"string","description":"模板所有权域"},"templateWorkspaceId":{"type":"string","description":"团队模板所属 Workspace ID"},"resourceType":{"type":"integer","description":"固定为独立白板模板类型 9"},"sourceNodeId":{"type":"string","description":"源独立白板节点 ID"},"requestId":{"type":"string","description":"服务端回显的幂等请求 ID"},"idempotentReplay":{"type":"boolean","description":"是否命中幂等重放"}},"required":["templateId","scope","resourceType","requestId"],"additionalProperties":true}`),
		SensitivePaths: []string{"templateId", "templateWorkspaceId", "sourceNodeId", "requestId"},
	}
}

func whiteboardTemplateListResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema:     json.RawMessage(`{"type":"object","description":"独立白板模板查询结果；续页信息位于 meta.pagination","properties":{"scope":{"type":"string","description":"模板所有权域"},"query":{"type":"string","description":"本次名称搜索关键词"},"templates":{"type":"array","description":"白板模板列表","items":{"type":"object","description":"白板模板摘要","properties":{"templateId":{"type":"string","description":"模板稳定 ID"},"name":{"type":"string","description":"模板名称"},"description":{"type":"string","description":"模板描述"},"coverUrl":{"type":"string","description":"模板封面 URL"},"scope":{"type":"string","description":"模板所有权域"},"templateWorkspaceId":{"type":"string","description":"团队模板所属 Workspace ID"},"resourceType":{"type":"integer","description":"固定为独立白板模板类型 9"}},"required":["templateId","resourceType"],"additionalProperties":true}}},"required":["scope","templates"],"additionalProperties":true}`),
		SensitivePaths: []string{"templates.templateId", "templates.templateWorkspaceId"},
	}
}

func whiteboardTemplateCreateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"从固定所有权域模板幂等创建独立白板的结果",
			"properties":{
				"templateId":{"type":"string","description":"实际使用的模板稳定 ID"},
				"templateScope":{"type":"string","description":"模板所有权域，personal 或 team"},
				"templateWorkspaceId":{"type":"string","description":"团队模板所属 Workspace ID"},
				"resourceType":{"type":"integer","description":"模板资源类型，固定为独立白板 9"},
				"verified":{"type":"boolean","description":"服务端是否完成 scope、类型和 Workspace 校验"},
				"nodeId":{"type":"string","description":"新建独立白板节点 ID"},
				"folderId":{"type":"string","description":"实际父目录 ID"},
				"name":{"type":"string","description":"白板名称"},
				"docUrl":{"type":"string","description":"桌面端访问链接"},
				"mobileUrl":{"type":"string","description":"移动端访问链接"},
				"contentType":{"type":"string","description":"网关投影该字段时固定为 WBD"},
				"extension":{"type":"string","description":"白板扩展名，通常为 adraw"},
				"status":{"type":"string","description":"创建状态"},
				"initializationMode":{"type":"string","description":"初始化方式"},
				"revision":{"type":"integer","description":"初始白板 revision"},
				"requestId":{"type":"string","description":"服务端回显的请求幂等 ID"},
				"idempotentReplay":{"type":"boolean","description":"是否命中幂等重放"},
				"requestMatched":{"type":"boolean","description":"历史幂等请求是否与本次参数一致"},
				"message":{"type":"string","description":"服务端结果说明"}
			},
			"required":["templateId","templateScope","resourceType","requestId","nodeId","revision"],
			"additionalProperties":true
		}`),
		SensitivePaths: []string{"templateId", "templateWorkspaceId", "nodeId", "folderId"},
	}
}

func callWhiteboardTemplatePreview(cmd *cobra.Command, tool string, args map[string]any) (map[string]any, error) {
	if err := whiteboardcore.ValidateTemplatePreviewCall(whiteboardServerID, tool, args); err != nil {
		return nil, err
	}
	caller, ok := deps.Caller.(edition.WhiteboardTemplatePreviewCaller)
	if !ok {
		return nil, fmt.Errorf("当前运行时不支持白板模板服务端预检；请升级 CLI，不会执行实际写入")
	}
	result, err := caller.CallWhiteboardTemplatePreview(cmd.Context(), tool, args)
	text, err := parseMCPToolTextResult(whiteboardServerID, tool, result, err)
	if err != nil {
		return nil, err
	}
	return decodeWhiteboardToolResult(tool, text)
}

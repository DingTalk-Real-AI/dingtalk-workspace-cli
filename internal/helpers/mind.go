// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.

package helpers

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	mindcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/mind"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublicNamed("mind", func() Handler { return wukongHandler{name: "mind", buildFn: newMindCommand} })
}

func newMindCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "mind",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("脑图操作与 OpenNodes 指南", "dingtalk-misc", "references/mind.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "携带 OpenNodes 初始内容创建独立脑图，并查询、创建、修改或删除脑图中的主题节点",
			UseWhen:      []string{"用户需要创建带内容的脑图，或读取、增删改脑图主题时"},
			AvoidWhen:    []string{"只在知识库创建空白脑图文件时使用 wiki node create --type amind；该命令不带初始化内容", "文档正文使用 doc；自由画布使用 whiteboard", "听记内置思维导图的生成及任务状态查询使用 minutes 对应命令；本产品仅操作独立脑图文件"},
		},
	})
	root := newGroupCommand(&cobra.Command{Use: "mind", Short: "钉钉脑图", RunE: groupRunE})
	nodes := newGroupCommand(&cobra.Command{Use: "node", Short: "查询和编辑脑图主题节点", RunE: groupRunE})
	nodes.AddCommand(newMindNodeListCommand(), newMindNodeCreateCommand(), newMindNodeUpdateCommand(), newMindNodeDeleteCommand())
	root.AddCommand(newMindCreateCommand(), nodes)
	return root
}

func mindInterface() *contract.InterfaceSpec {
	return &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "CLI 校验 mind-v1 开放参数并核验 MCP 业务回执；复用已注册的 mind MCP Server 和公共认证调用链"}
}

func newMindCreateCommand() *cobra.Command {
	const example = `dws mind create --name "项目计划" --source '{"schemaVersion":"1.0","catalogVersion":"mind-v1","pages":[{"id":"p","title":"计划","nodes":[{"id":"r","type":"topic","text":"项目计划"}]}]}' --request-id mind-create-001 --format json`
	return NewLeafCommand(LeafSpec{
		Example: example,
		Use:     "create", Short: "携带 OpenNodes 初始内容创建脑图", Long: "携带 OpenNodes 初始内容创建脑图，必须提供 source；只在知识库创建空白脑图文件请用 wiki node create --type amind。source 是 mind-v1 JSON 字符串或本地 JSON 文件，CLI 会在远端调用前校验字段、类型、版本、ID、树结构及大小限制，不合法则本地报参数错误。返回 READY 且 requestedContentApplied=true 才表示完成。恢复必须复用 request-id、名称、目录和原始内容。folder 与 workspace 同时提供时 folder 优先。",
		Server: "mind", Tool: "create_mind", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "name", Bind: "name", Usage: "脑图名称，最多 128 个 UTF-16 单元（必填）", Required: true, MarkRequired: true, Transform: validateMindName},
			{Name: "source", Bind: "source", Usage: "OpenNodes mind-v1 JSON 或 JSON 文件路径；远端调用前完成格式和树结构校验（必填）", Required: true, MarkRequired: true, Trim: true, Transform: loadMindSource},
			{Name: "folder", Bind: "folderId", Usage: "目标文件夹节点 ID/URL", Trim: true, OmitEmpty: true},
			{Name: "workspace", Bind: "workspaceId", Usage: "目标知识库 ID/URL", Trim: true, OmitEmpty: true},
			{Name: "request-id", Bind: "requestId", Usage: "稳定幂等键；恢复必须复用（必填）", Required: true, MarkRequired: true, Trim: true, Transform: validateMindRequestID},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "mind", Name: "create", CanonicalPath: "mind.create", CLIPath: "mind create", PrimaryCLIPath: "mind create"},
			Description: "携带 OpenNodes 初始内容创建脑图", Interface: mindInterface(),
			DryRun:    &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Selection: contract.SelectionSpec{Examples: []string{example}, AgentSummary: "携带 OpenNodes 初始内容创建脑图，必须提供 source；只在知识库创建空白脑图文件请用 wiki node create --type amind。source 是 mind-v1 JSON 字符串或本地 JSON 文件，CLI 会在远端调用前校验字段、类型、版本、ID、树结构及大小限制，不合法则本地报参数错误。返回 READY 且 requestedContentApplied=true 才表示完成。恢复必须复用 request-id、名称、目录和原始内容。folder 与 workspace 同时提供时 folder 优先。", UseWhen: []string{"携带 OpenNodes 初始页面及主题内容创建脑图时"}, AvoidWhen: []string{"仅在知识库创建空白脑图文件、不需要初始化内容时，使用 wiki node create --type amind", "其他类型的文档内容使用对应产品命令"}},
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "name", InterfaceType: "string"},
				{Name: "source", Property: "source", InterfaceType: "string"},
				{Name: "folder", Property: "folderId", InterfaceType: "string"},
				{Name: "workspace", Property: "workspaceId", InterfaceType: "string"},
				{Name: "request-id", Property: "requestId", InterfaceType: "string"},
			},
			Result: mindCreateResultSpec(),
		},
		Validate: validateMindCommand, ResultCall: callMindResult,
	})
}

func newMindNodeListCommand() *cobra.Command {
	const example = `dws mind node list --node-id FILE_ID --format json`
	return NewLeafCommand(LeafSpec{
		Example: example,
		Use:     "list", Short: "分页查询脑图主题节点", Long: "ROOT 查询根主题，省略 topic-id；next-token 可省略或为空，非空时拒绝。NODE、CHILDREN、SUBTREE 要求 page-id 和 topic-id。depth 包含起点层，默认 8。只读取一页；续页复用相同身份、查询参数、depth 和 max-results。truncated=true 不能视为完整树。",
		Server: "mind", Tool: "list_mind_nodes", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "node-id", Bind: "nodeId", Usage: "脑图文件 ID，不是主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "page-id", Bind: "pageId", Usage: "真实页面 ID", RequiredWhen: "mode is NODE, CHILDREN or SUBTREE", Trim: true, OmitEmpty: true},
			{Name: "topic-id", Bind: "topicId", Usage: "真实主题 ID；ROOT 模式省略", RequiredWhen: "mode is NODE, CHILDREN or SUBTREE", Trim: true, OmitEmpty: true},
			{Name: "mode", Bind: "mode", Usage: "ROOT/NODE/CHILDREN/SUBTREE", Default: "ROOT", Enum: []string{"ROOT", "NODE", "CHILDREN", "SUBTREE"}},
			{Name: "depth", Bind: "depth", Usage: "查询层数 1-8，包含起点", Default: "8", Transform: parseMindDepth},
			{Name: "max-results", Bind: "maxResults", Usage: "单页最多主题数 1-200", Default: "200", Transform: parseMindLimit},
			{Name: "next-token", Bind: "nextToken", Usage: "上页返回的 mindc1_ 分页游标；空字符串按首次查询处理", OmitEmpty: true},
		},
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "mind", Name: "list_nodes", CanonicalPath: "mind.list_nodes", CLIPath: "mind node list", PrimaryCLIPath: "mind node list"},
			Description: "分页查询脑图主题节点", Interface: mindInterface(),
			DryRun:    &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Selection: contract.SelectionSpec{Examples: []string{example}, AgentSummary: "ROOT 查询根主题，省略 topic-id；next-token 可省略或为空，非空时拒绝。NODE、CHILDREN、SUBTREE 要求 page-id 和 topic-id。depth 包含起点层，默认 8。只读取一页；续页复用相同身份、查询参数、depth 和 max-results。truncated=true 不能视为完整树。", UseWhen: []string{"分页查询脑图主题节点时"}, AvoidWhen: []string{"其他类型的文档内容使用对应产品命令"}},
			Parameters: []contract.ParamDecl{
				{Name: "node-id", Property: "nodeId", InterfaceType: "string"},
				{Name: "page-id", Property: "pageId", InterfaceType: "string"},
				{Name: "topic-id", Property: "topicId", InterfaceType: "string"},
				{Name: "mode", Property: "mode", InterfaceType: "string"},
				{Name: "depth", Property: "depth", InterfaceType: "integer"},
				{Name: "max-results", Property: "maxResults", InterfaceType: "integer"},
				{Name: "next-token", Property: "nextToken", InterfaceType: "string"},
			},
			Result:     mindQueryResultSpec(),
			Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "next-token"},
		},
		Validate: validateMindCommand, ResultCall: callMindResult,
	})
}

func newMindNodeCreateCommand() *cobra.Command {
	const example = `dws mind node create --node-id FILE_ID --page-id PAGE_ID --parent-topic-id ROOT_TOPIC_ID --topic '{"id":"task-1","type":"topic","text":"第一项任务"}' --request-id mind-node-create-001 --format json`
	return NewLeafCommand(LeafSpec{
		Example: example,
		Use:     "create", Short: "在脑图指定父主题下创建主题", Long: "topic 为 JSON 对象或文件，字段仅 id（临时主题 ID）、type（固定 topic）、text。parent-topic-id 和 before-topic-id 使用真实主题 ID；返回 nodeIdMap 将临时 ID 映射为真实主题 ID。仅 APPLIED 且 applied=true 表示完成。",
		Server: "mind", Tool: "create_mind_node", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "node-id", Bind: "nodeId", Usage: "脑图文件 ID，不是主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "page-id", Bind: "pageId", Usage: "真实页面 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "request-id", Bind: "requestId", Usage: "稳定幂等键；恢复必须复用（必填）", Required: true, MarkRequired: true, Trim: true, Transform: validateMindRequestID},
			{Name: "parent-topic-id", Bind: "parentTopicId", Usage: "真实父主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "before-topic-id", Bind: "beforeTopicId", Usage: "插入到指定真实兄弟主题之前", Trim: true, OmitEmpty: true},
			{Name: "topic", Bind: "node", Usage: "主题 JSON 或文件：id、type、text（必填）", Required: true, MarkRequired: true, Trim: true, Transform: loadMindTopic},
			{Name: "expected-version", Bind: "expectedVersion", Usage: "可选的期望版本；十进制字符串", Trim: true, OmitEmpty: true, Transform: validateMindVersion},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "mind", Name: "create_node", CanonicalPath: "mind.create_node", CLIPath: "mind node create", PrimaryCLIPath: "mind node create"},
			Description: "在脑图指定父主题下创建主题", Interface: mindInterface(),
			DryRun:    &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Selection: contract.SelectionSpec{Examples: []string{example}, AgentSummary: "topic 为 JSON 对象或文件，字段仅 id（临时主题 ID）、type（固定 topic）、text。parent-topic-id 和 before-topic-id 使用真实主题 ID；返回 nodeIdMap 将临时 ID 映射为真实主题 ID。仅 APPLIED 且 applied=true 表示完成。", UseWhen: []string{"在脑图指定父主题下创建主题时"}, AvoidWhen: []string{"其他类型的文档内容使用对应产品命令"}},
			Parameters: []contract.ParamDecl{
				{Name: "node-id", Property: "nodeId", InterfaceType: "string"},
				{Name: "page-id", Property: "pageId", InterfaceType: "string"},
				{Name: "request-id", Property: "requestId", InterfaceType: "string"},
				{Name: "parent-topic-id", Property: "parentTopicId", InterfaceType: "string"},
				{Name: "before-topic-id", Property: "beforeTopicId", InterfaceType: "string"},
				{Name: "topic", Property: "node", InterfaceType: "string"},
				{Name: "expected-version", Property: "expectedVersion", InterfaceType: "string"},
			},
			Result: mindMutationResultSpec(),
		},
		Validate: validateMindCommand, ResultCall: callMindResult,
	})
}

func newMindNodeUpdateCommand() *cobra.Command {
	const example = `dws mind node update --node-id FILE_ID --page-id PAGE_ID --topic-id TOPIC_ID --text "修改后的主题" --request-id mind-node-update-001 --format json`
	return NewLeafCommand(LeafSpec{
		Example: example,
		Use:     "update", Short: "修改脑图主题文本", Long: "只修改指定真实主题的文本；--text 必须显式提供，允许空字符串清空文本。相同 request-id 的恢复必须复用全部参数。NO_CHANGE 表示内容无需修改；COMMITTED_UNVERIFIED 表示已提交但尚未确认，不能当作完成。",
		Server: "mind", Tool: "update_mind_node", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "node-id", Bind: "nodeId", Usage: "脑图文件 ID，不是主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "page-id", Bind: "pageId", Usage: "真实页面 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "request-id", Bind: "requestId", Usage: "稳定幂等键；恢复必须复用（必填）", Required: true, MarkRequired: true, Trim: true, Transform: validateMindRequestID},
			{Name: "topic-id", Bind: "topicId", Usage: "真实主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "text", Bind: "patch", Usage: "新文本；允许显式空字符串（必填）", MarkRequired: true, Transform: mindTextPatch},
			{Name: "expected-version", Bind: "expectedVersion", Usage: "可选的期望版本；十进制字符串", Trim: true, OmitEmpty: true, Transform: validateMindVersion},
		},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "mind", Name: "update_node", CanonicalPath: "mind.update_node", CLIPath: "mind node update", PrimaryCLIPath: "mind node update"},
			Description: "修改脑图主题文本", Interface: mindInterface(),
			DryRun:    &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Selection: contract.SelectionSpec{Examples: []string{example}, AgentSummary: "只修改指定真实主题的文本；--text 必须显式提供，允许空字符串清空文本。相同 request-id 的恢复必须复用全部参数。NO_CHANGE 表示内容无需修改；COMMITTED_UNVERIFIED 表示已提交但尚未确认，不能当作完成。", UseWhen: []string{"修改脑图主题文本时"}, AvoidWhen: []string{"其他类型的文档内容使用对应产品命令"}},
			Parameters: []contract.ParamDecl{
				{Name: "node-id", Property: "nodeId", InterfaceType: "string"},
				{Name: "page-id", Property: "pageId", InterfaceType: "string"},
				{Name: "request-id", Property: "requestId", InterfaceType: "string"},
				{Name: "topic-id", Property: "topicId", InterfaceType: "string"},
				{Name: "text", Property: "patch", InterfaceType: "string"},
				{Name: "expected-version", Property: "expectedVersion", InterfaceType: "string"},
			},
			Result: mindMutationResultSpec(),
		},
		Validate: validateMindCommand, ResultCall: callMindResult,
	})
}

func newMindNodeDeleteCommand() *cobra.Command {
	const example = `dws mind node delete --node-id FILE_ID --page-id PAGE_ID --topic-id TOPIC_ID --request-id mind-node-delete-001 --format json`
	return NewLeafCommand(LeafSpec{
		Example: example,
		Use:     "delete", Short: "删除脑图主题及其子树", Long: "删除真实主题及其全部后代，deleteMode 固定为 subtree。根主题不可删除。仅 APPLIED 且 applied=true 表示完成；deletedCount 只有 deletedCountKnown=true 时可作为准确删除数量。",
		Server: "mind", Tool: "delete_mind_node", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "node-id", Bind: "nodeId", Usage: "脑图文件 ID，不是主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "page-id", Bind: "pageId", Usage: "真实页面 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "request-id", Bind: "requestId", Usage: "稳定幂等键；恢复必须复用（必填）", Required: true, MarkRequired: true, Trim: true, Transform: validateMindRequestID},
			{Name: "topic-id", Bind: "topicId", Usage: "真实主题 ID（必填）", Required: true, MarkRequired: true, Trim: true},
			{Name: "expected-version", Bind: "expectedVersion", Usage: "可选的期望版本；十进制字符串", Trim: true, OmitEmpty: true, Transform: validateMindVersion},
		},
		ConstParams: map[string]any{"deleteMode": "subtree"},
		Safety:      contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "mind", Name: "delete_node", CanonicalPath: "mind.delete_node", CLIPath: "mind node delete", PrimaryCLIPath: "mind node delete"},
			Description: "删除脑图主题及其子树", Interface: mindInterface(),
			DryRun:    &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Selection: contract.SelectionSpec{Examples: []string{example}, AgentSummary: "删除真实主题及其全部后代，deleteMode 固定为 subtree。根主题不可删除。仅 APPLIED 且 applied=true 表示完成；deletedCount 只有 deletedCountKnown=true 时可作为准确删除数量。", UseWhen: []string{"删除脑图主题及其子树时"}, AvoidWhen: []string{"其他类型的文档内容使用对应产品命令"}},
			Parameters: []contract.ParamDecl{
				{Name: "node-id", Property: "nodeId", InterfaceType: "string"},
				{Name: "page-id", Property: "pageId", InterfaceType: "string"},
				{Name: "request-id", Property: "requestId", InterfaceType: "string"},
				{Name: "topic-id", Property: "topicId", InterfaceType: "string"},
				{Name: "expected-version", Property: "expectedVersion", InterfaceType: "string"},
			},
			Result: mindMutationResultSpec(),
		},
		Validate: validateMindCommand, ResultCall: callMindResult,
	})
}

func validateMindRequestID(raw string) (any, error) {
	if strings.TrimSpace(raw) == "" || !utf8.ValidString(raw) || len(utf16.Encode([]rune(raw))) > 128 {
		return nil, fmt.Errorf("request-id must contain 1-128 characters")
	}
	return raw, nil
}
func validateMindName(raw string) (any, error)    { return raw, mindcore.ValidateName(raw) }
func validateMindVersion(raw string) (any, error) { return raw, mindcore.ValidateVersion(raw) }
func mindTextPatch(raw string) (any, error) {
	if err := mindcore.ValidateText(raw); err != nil {
		return nil, err
	}
	return map[string]any{"text": raw}, nil
}
func parseMindDepth(raw string) (any, error) { return parseMindInteger(raw, 8) }
func parseMindLimit(raw string) (any, error) { return parseMindInteger(raw, 200) }
func parseMindInteger(raw string, max int) (any, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		return nil, fmt.Errorf("value must be an integer within 1..%d", max)
	}
	return n, nil
}
func readMindJSON(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") {
		if len(raw) > mindcore.MaxSourceBytes {
			return nil, fmt.Errorf("JSON exceeds 2 MiB")
		}
		return []byte(raw), nil
	}
	file, err := os.Open(raw)
	if err != nil {
		return nil, fmt.Errorf("read JSON file %q: %w", raw, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, mindcore.MaxSourceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read JSON file %q: %w", raw, err)
	}
	if len(data) > mindcore.MaxSourceBytes {
		return nil, fmt.Errorf("JSON file exceeds 2 MiB")
	}
	return data, nil
}
func loadMindSource(raw string) (any, error) {
	data, err := readMindJSON(raw)
	if err != nil {
		return nil, err
	}
	if _, err = mindcore.ParseSource(data); err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	// Preserve the validated text, including sibling order. It is a JSON string
	// in the tool arguments, not an object or a checkpoint.
	return string(data), nil
}
func loadMindTopic(raw string) (any, error) {
	data, err := readMindJSON(raw)
	if err != nil {
		return nil, err
	}
	topic, err := mindcore.ParseTopic(data)
	if err != nil {
		return nil, fmt.Errorf("topic: %w", err)
	}
	return map[string]any{"id": topic.ID, "type": topic.Type, "text": topic.Text}, nil
}
func validateMindCommand(cmd *cobra.Command, _ []string) error {
	// An explicitly supplied empty optional flag must not silently disappear.
	for _, key := range []string{"node-id", "page-id", "topic-id", "parent-topic-id", "before-topic-id", "folder", "workspace", "expected-version"} {
		if flag := cmd.Flags().Lookup(key); flag != nil && flag.Changed && strings.TrimSpace(flag.Value.String()) == "" {
			return fmt.Errorf("--%s cannot be blank when supplied", key)
		}
	}
	if cmd.Name() != "list" {
		return nil
	}
	mode, _ := cmd.Flags().GetString("mode")
	page, _ := cmd.Flags().GetString("page-id")
	topic, _ := cmd.Flags().GetString("topic-id")
	token, _ := cmd.Flags().GetString("next-token")
	if err := mindcore.ValidateCursor(token); err != nil {
		return err
	}
	if mode == "ROOT" {
		if strings.TrimSpace(topic) != "" || token != "" {
			return fmt.Errorf("ROOT mode does not accept --topic-id or --next-token")
		}
		return nil
	}
	if strings.TrimSpace(page) == "" || strings.TrimSpace(topic) == "" {
		return fmt.Errorf("%s mode requires --page-id and --topic-id", mode)
	}
	return nil
}

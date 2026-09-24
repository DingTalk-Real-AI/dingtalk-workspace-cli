// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/buildversion"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/authoring"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/delivery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/preview"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/protocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/state"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/profilectx"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var cardResultSpec = &contract.ResultSpec{
	Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
	DataSchema: json.RawMessage(`{"type":"object","description":"A2UI card command result","additionalProperties":true}`),
}

var (
	cardCurrentProfileIdentity = profilectx.GetIdentity
	cardResolveProfileIdentity = profilectx.ResolveIdentity
)

func newCardCommand() *cobra.Command {
	root := newGroupCommand(&cobra.Command{Use: "card", Short: "构建、校验、预览、发送和更新 A2UI 卡片"})
	protocolGroup := newGroupCommand(&cobra.Command{Use: "protocol", Short: "查询内置 A2UI 协议包"})
	protocolGroup.AddCommand(newCardProtocolInfoCommand(), newCardProtocolVerifyCommand())
	catalogGroup := newGroupCommand(&cobra.Command{Use: "catalog", Short: "渐进查询 A2UI 组件协议"})
	catalogGroup.AddCommand(newCardCatalogListCommand(), newCardCatalogSearchCommand(), newCardCatalogGetCommand())
	blockGroup := newGroupCommand(&cobra.Command{Use: "block", Short: "查询语义化 A2UI 区块"})
	blockGroup.AddCommand(newCardBlockListCommand(), newCardBlockShowCommand())
	recipeGroup := newGroupCommand(&cobra.Command{Use: "recipe", Short: "查询内置 A2UI Recipe"})
	recipeGroup.AddCommand(newCardRecipeListCommand(), newCardRecipeShowCommand())
	guideGroup := newGroupCommand(&cobra.Command{Use: "guide", Short: "获得卡片组合建议"})
	guideGroup.AddCommand(newCardGuideRecommendCommand(), newCardGuideRulesCommand())
	root.AddCommand(protocolGroup, catalogGroup, blockGroup, recipeGroup, guideGroup)
	root.AddCommand(newCardComposeCommand(), newCardBuildCommand(), newCardLintCommand(), newCardReviewCommand(), newCardPreviewCommand())
	root.AddCommand(newCardSendCommand(), newCardUpdateCommand(), newCardFinishCommand(), newCardVerifyCommand(), newCardListCommand(), newCardShowCommand(), newCardDoctorCommand())
	return root
}

func newCardCatalogSearchCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "search", Short: "按名称或语义搜索 A2UI 组件", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "query", Bind: "query", Usage: "组件名、用途或场景关键词", Required: true, Trim: true}}, Safety: readSafety(),
		Contract: localContract("catalog_search", "card catalog search", "按组件名和内置语义指南搜索 A2UI 组件", "Agent 知道用途但不知道准确组件名时", "已知组件名时使用 card catalog get", `dws card catalog search --query "表单选择"`),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			query := strings.ToLower(fmt.Sprint(args["query"]))
			var matches []authoring.ComponentGuide
			for _, guide := range authoring.Guides() {
				haystack := strings.ToLower(guide.Name + " " + guide.Role + " " + guide.UseWhen + " " + guide.AvoidWhen)
				if strings.Contains(haystack, query) || strings.Contains(query, strings.ToLower(guide.Name)) {
					matches = append(matches, guide)
				}
			}
			return output.Success(map[string]any{"query": args["query"], "matches": matches}), nil
		},
	})
}

func newCardBlockListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "list", Short: "列出内置语义区块", OutputRollout: output.RolloutUnifiedActive, Safety: readSafety(), Contract: localContract("block_list", "card block list", "List semantic A2UI blocks for an information plan", "When composing a card from information roles and relationships", "A complete built-in Recipe is optional; custom A2UI composition is supported", "dws card block list"), ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
		return output.Success(map[string]any{"blocks": authoring.Blocks()}), nil
	}})
}

func newCardBlockShowCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "show", Short: "查看一个语义区块", OutputRollout: output.RolloutUnifiedActive, Flags: []LeafFlag{{Name: "name", Bind: "name", Usage: "区块名称", Required: true, Trim: true}}, Safety: readSafety(), Contract: localContract("block_show", "card block show", "返回一个 A2UI 语义区块的职责和推荐组件", "Agent 已选择区块并需要了解组成时", "需要组件字段时使用 card catalog get", "dws card block show --name header"), ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
		name := fmt.Sprint(args["name"])
		for _, block := range authoring.Blocks() {
			if block.Name == name {
				return output.Success(map[string]any{"block": block}), nil
			}
		}
		return nil, fmt.Errorf("unknown block %q", name)
	}})
}

func newCardProtocolInfoCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "info", Short: "显示内置协议版本和来源摘要", OutputRollout: output.RolloutUnifiedActive,
		Safety: readSafety(), Contract: localContract("protocol_info", "card protocol info", "查看 DWS 内置 A2UI 协议包版本、摘要和组件数量", "需要确认 Agent 使用的 A2UI 协议版本时", "需要某个组件完整 Schema 时使用 card catalog get", "dws card protocol info"),
		ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
			registry, err := protocol.Load()
			if err != nil {
				return nil, err
			}
			catalog := registry.Catalog()
			return output.Success(map[string]any{"bundleVersion": protocol.BundleVersion, "manifestHash": protocol.ManifestHash(), "catalogId": catalog.CatalogID, "protocolVersion": catalog.ProtocolVersion, "components": len(catalog.Components), "functions": len(catalog.Functions)}), nil
		},
	})
}

func newCardProtocolVerifyCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "verify", Short: "校验内置协议资产完整性", OutputRollout: output.RolloutUnifiedActive,
		Safety: readSafety(), Contract: localContract("protocol_verify", "card protocol verify", "校验嵌入的 A2UI Catalog 与公共类型摘要并编译 Schema", "安装、升级或排障后确认协议资产可用时", "校验业务卡片内容时使用 card lint", "dws card protocol verify"),
		ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
			registry, err := protocol.Load()
			if err != nil {
				return nil, err
			}
			return output.Success(map[string]any{"valid": true, "bundleVersion": protocol.BundleVersion, "manifestHash": protocol.ManifestHash(), "componentCount": len(registry.ComponentNames())}), nil
		},
	})
}

func newCardCatalogListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "list", Short: "列出公开 A2UI 组件", OutputRollout: output.RolloutUnifiedActive,
		Safety: readSafety(), Contract: localContract("catalog_list", "card catalog list", "列出内置 A2UI Catalog 的组件名称", "Agent 需要先发现可用组件时", "已知组件并需要完整字段时使用 card catalog get", "dws card catalog list"),
		ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
			registry, err := protocol.Load()
			if err != nil {
				return nil, err
			}
			return output.Success(map[string]any{"components": registry.ComponentNames()}), nil
		},
	})
}

func newCardCatalogGetCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "get", Short: "读取一个组件的完整 Schema 和语义指南", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "component", Bind: "component", Usage: "组件名称", Required: true, Trim: true}}, Safety: readSafety(),
		Contract: localContract("catalog_get", "card catalog get", "按名称返回单个 A2UI 组件 Schema、示例和使用建议", "Agent 要使用某个组件并需要精确字段合同时", "只需组件名称列表时使用 card catalog list", "dws card catalog get --component Button"),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			registry, err := protocol.Load()
			if err != nil {
				return nil, err
			}
			name := fmt.Sprint(args["component"])
			raw, ok := registry.Component(name)
			if !ok {
				return nil, fmt.Errorf("unknown A2UI component %q", name)
			}
			var schema any
			if err := json.Unmarshal(raw, &schema); err != nil {
				return nil, err
			}
			guide, _ := authoring.Guide(name)
			return output.Success(map[string]any{"name": name, "guide": guide, "schema": schema}), nil
		},
	})
}

func newCardRecipeListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "list", Short: "列出内置 A2UI Recipe", OutputRollout: output.RolloutUnifiedActive,
		Safety: readSafety(), Contract: localContract("recipe_list", "card recipe list", "列出可直接编译的语义化 A2UI Recipe", "Agent 要根据内容类型选择稳定布局时", "需要组件字段时使用 card catalog", "dws card recipe list"),
		ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
			return output.Success(map[string]any{"recipes": authoring.Recipes()}), nil
		},
	})
}

func newCardRecipeShowCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "show", Short: "查看一个 A2UI Recipe", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "name", Bind: "name", Usage: "Recipe 名称", Required: true, Trim: true}}, Safety: readSafety(),
		Contract: localContract("recipe_show", "card recipe show", "返回 Recipe 的使用场景和区块构成", "Agent 已选定 Recipe 并需确认信息层次时", "要生成消息时使用 card compose", "dws card recipe show --name information"),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			name := fmt.Sprint(args["name"])
			for _, recipe := range authoring.Recipes() {
				if recipe.Name == name {
					return output.Success(map[string]any{"recipe": recipe}), nil
				}
			}
			return nil, fmt.Errorf("unknown recipe %q", name)
		},
	})
}

func newCardGuideRecommendCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "recommend", Short: "Plan information hierarchy and suggest an optional Recipe", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "intent", Bind: "intent", Usage: "要表达的内容或交互意图", Required: true, Trim: true}}, Safety: readSafety(),
		Contract: localContract("guide_recommend", "card guide recommend", "Plan information roles and relationships, then suggest an optional A2UI Recipe", "Before composing a card from content, especially content outside built-in Recipes", "For exact component fields use card catalog get; raw A2UI can be validated with card lint", `dws card guide recommend --intent "需要用户审批方案"`),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			guidance := authoring.RecommendComposition(fmt.Sprint(args["intent"]))
			return output.Success(map[string]any{"recipe": guidance.Recipe, "recipeMatch": guidance.RecipeMatched, "recipeIsOptional": true, "suggestedPath": guidance.SuggestedPath, "reason": guidance.Reason, "informationPlan": authoring.InformationPlan()}), nil
		},
	})
}

func newCardGuideRulesCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "rules", Short: "列出组件语义和组合规则", OutputRollout: output.RolloutUnifiedActive,
		Safety: readSafety(), Contract: localContract("guide_rules", "card guide rules", "返回一期组件的场景语义、可靠性约束和视觉规则", "Agent 要自由组合组件或审查设计质量时", "需要精确字段合同使用 card catalog get", "dws card guide rules"),
		ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
			return output.Success(map[string]any{"components": authoring.Guides(), "surfacePolicies": authoring.SurfacePolicies(), "informationPlan": authoring.InformationPlan(), "rules": []string{"When the message root is Card, always set backgroundColor=#00FFFFFF and omit backgroundColorToken; place opaque or themed fills on child components. A root Column may omit fill to show the host bubble.", "Keep one primary action when the card needs an action.", "Use Column for the reading sequence and Row for short peers; check long content at a narrow width.", "Use a Divider only where it clarifies a real semantic boundary; spacing may be enough.", "Bind actual form inputs to DataModel and return their values through the submit action.", "Use appendDataModel for streaming text and updateDataModel for a full checkpoint after failure.", "Select a host SurfacePolicy for content width; public A2UI has no full-card minWidth field, so do not emit a private width property."}}), nil
		},
	})
}

func newCardComposeCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "compose", Short: "Compile the fixed-slot built-in Recipe Spec to A2UI", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "file", Bind: "file", Usage: "Built-in Recipe CompositionSpec JSON file; - reads stdin", Required: true, Trim: true}, {Name: "output", Bind: "output", Usage: "可选输出文件", Trim: true, OmitEmpty: true}}, Safety: readSafety(),
		Contract:   localContract("compose", "card compose", "Compile the fixed-slot built-in Recipe Spec to Schema-validated A2UI messages", "After card guide suggests review_recipe and the content fits the selected Recipe slots", "For novel layouts compose raw public A2UI and use card lint/build; for existing A2UI wire use card build", "dws card compose --file ./card.json --output ./messages.json"),
		ResultCall: composeResult,
	})
}

func newCardBuildCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "build", Short: "校验并生成确定性 A2UI wire", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "file", Bind: "file", Usage: "A2UI 对象数组、wire 数组或 JSONL，- 表示 stdin", Required: true, Trim: true}, {Name: "mode", Bind: "mode", Usage: "create 或 update", Default: "create", Enum: []string{"create", "update"}}}, Safety: readSafety(),
		Contract: localContract("build", "card build", "读取、归一化并校验 A2UI 消息，输出 MCP 所需字符串数组", "需要检查最终投递 wire 或兼容旧格式时", "要发送卡片时使用 card send", "dws card build --file ./messages.json --mode create"),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			messages, validation, err := loadAndValidate(fmt.Sprint(args["file"]), fmt.Sprint(args["mode"]))
			if err != nil {
				return nil, err
			}
			wire, err := protocol.MarshalWire(messages)
			if err != nil {
				return nil, err
			}
			return output.Success(map[string]any{"validation": validation, "messages": messages, "wire": wire}), nil
		},
	})
}

func newCardLintCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "lint", Short: "执行 A2UI 协议、引用和设计校验", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "file", Bind: "file", Usage: "A2UI 对象数组、wire 数组或 JSONL，- 表示 stdin", Required: true, Trim: true}, {Name: "mode", Bind: "mode", Usage: "create 或 update", Default: "create", Enum: []string{"create", "update"}}, {Name: "design-archetype", Bind: "designArchetype", Usage: "视觉规则的消息类型", Enum: []string{"notification", "approval", "report", "detail", "form", "progress", "information"}}, {Name: "minimum-validation-width", Bind: "minimumValidationWidth", Usage: "设计验收的最小宽度（像素）", Trim: true}, {Name: "debug-host-badge", Bind: "debugHostBadge", Usage: "目标客户端显示 Debug 标识", Kind: LeafBool}}, Safety: readSafety(),
		Contract: localContract("lint", "card lint", "离线执行 A2UI 消息、组件 Schema、引用图和更新模式校验", "生成或发送前需要快速定位协议问题时", "需要 MCP wire 输出时使用 card build", "dws card lint --file ./messages.json --mode create"),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			messages, validation, err := loadAndValidate(fmt.Sprint(args["file"]), fmt.Sprint(args["mode"]))
			if err != nil && validation.BundleVersion == "" {
				return nil, err
			}
			minimumWidth := 0
			if raw := strings.TrimSpace(fmt.Sprint(args["minimumValidationWidth"])); raw != "" && raw != "<nil>" {
				minimumWidth, err = strconv.Atoi(raw)
				if err != nil || minimumWidth < 0 {
					return nil, apperrors.NewValidation("minimum-validation-width must be a non-negative integer")
				}
			}
			visual := authoring.VisualDiagnostics(messages, authoring.VisualContext{Archetype: strings.TrimSpace(fmt.Sprint(args["designArchetype"])), MinimumValidationWidth: minimumWidth, DebugHostBadge: args["debugHostBadge"] == true})
			validation.Diagnostics = append(validation.Diagnostics, visual...)
			for _, diagnostic := range visual {
				if diagnostic.Severity == "error" {
					validation.Valid = false
					err = fmt.Errorf("A2UI visual lint failed")
				}
			}
			if err != nil {
				return output.Failure(&output.ErrorInfo{Type: "validation", Subtype: "a2ui_validation_failed", Message: err.Error(), Hint: "inspect error.details.validation.diagnostics", Details: map[string]any{"validation": validation}}), nil
			}
			return output.Success(map[string]any{"validation": validation}), nil
		},
	})
}

func newCardReviewCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "review", Short: "Review card composition before delivery", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "file", Bind: "file", Usage: "A2UI create message file", Required: true, Trim: true}, {Name: "design-archetype", Bind: "designArchetype", Usage: "Message type for visual rules", Enum: []string{"notification", "approval", "report", "detail", "form", "progress", "information"}}, {Name: "minimum-validation-width", Bind: "minimumValidationWidth", Usage: "Minimum design validation width in pixels", Trim: true}, {Name: "debug-host-badge", Bind: "debugHostBadge", Usage: "Target client shows a Debug badge", Kind: LeafBool}}, Safety: readSafety(),
		Contract: localContract("review", "card review", "Run bundled composition checks and return an author and client review rubric", "Before sending a card whose information hierarchy or layout matters", "Protocol validity and this review do not prove target-client rendering", "dws card review --file ./messages.json --design-archetype report"),
		ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
			context, err := visualContextFromArgs(args)
			if err != nil {
				return nil, err
			}
			messages, validation, err := loadAndValidate(fmt.Sprint(args["file"]), "create")
			if err != nil {
				return nil, err
			}
			review := authoring.ReviewVisual(messages, context)
			if review.StaticStatus == "fail" {
				return output.Failure(&output.ErrorInfo{Type: "validation", Subtype: "a2ui_visual_review_failed", Message: "card visual review found a deterministic error", Hint: "inspect error.details.visualReview.diagnostics", Details: map[string]any{"validation": validation, "visualReview": review}}), nil
			}
			return output.Success(map[string]any{"validation": validation, "visualReview": review}), nil
		},
	})
}

func visualContextFromArgs(args map[string]any) (authoring.VisualContext, error) {
	context := authoring.VisualContext{Archetype: strings.TrimSpace(fmt.Sprint(args["designArchetype"])), DebugHostBadge: args["debugHostBadge"] == true}
	if context.Archetype == "<nil>" {
		context.Archetype = ""
	}
	if raw := strings.TrimSpace(fmt.Sprint(args["minimumValidationWidth"])); raw != "" && raw != "<nil>" {
		width, err := strconv.Atoi(raw)
		if err != nil || width < 0 {
			return context, apperrors.NewValidation("minimum-validation-width must be a non-negative integer")
		}
		context.MinimumValidationWidth = width
	}
	return context, nil
}

func newCardPreviewCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "preview", Short: "生成无需钉钉的本地参考预览", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{{Name: "file", Bind: "file", Usage: "create 模式 A2UI 消息文件", Required: true, Trim: true}, {Name: "output", Bind: "output", Usage: "HTML 输出路径", Required: true, Trim: true}}, Safety: contract.SafetySpec{Effect: "write", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:   localContract("preview", "card preview", "离线生成确定性的 reference_preview HTML，验证层级、内容和交互控件", "希望不依赖钉钉客户端快速查看卡片结构时", "不能用它声明真实客户端像素级验收通过", "dws card preview --file ./messages.json --output ./preview.html"),
		ResultCall: previewResult,
	})
}

func newCardSendCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "send", Short: "校验并发送 A2UI 卡片", Long: "校验、冻结目标并通过 IM MCP 创建 A2UI 卡片，同时保存本地更新台账。", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "file", Bind: "file", Usage: "create 模式 A2UI 消息文件", Required: true, Trim: true},
			{Name: "conversation-id", Bind: "conversationId", Usage: "群聊 openConversationId", Trim: true, OmitEmpty: true},
			{Name: "chat-query", Bind: "chatQuery", Usage: "群名、群号或其他群线索；仅唯一解析时发送", Trim: true, OmitEmpty: true},
			{Name: "open-dingtalk-id", Bind: "openDingtalkId", Usage: "单聊接收者 openDingTalkId", Trim: true, OmitEmpty: true},
			{Name: "user-query", Bind: "userQuery", Usage: "精确 userId 或 openDingTalkId；不执行姓名模糊搜索", Trim: true, OmitEmpty: true},
			{Name: "summary", Bind: "summary", Usage: "卡片降级摘要", Trim: true, OmitEmpty: true},
			{Name: "idempotency-key", Bind: "idempotencyKey", Usage: "可选稳定业务幂等键", Trim: true, OmitEmpty: true},
			{Name: "support-forward", Bind: "supportForward", Usage: "是否允许转发", Kind: LeafBool},
			{Name: "design-archetype", Bind: "designArchetype", Usage: "Message type for bundled visual review", Enum: []string{"notification", "approval", "report", "detail", "form", "progress", "information"}},
			{Name: "minimum-validation-width", Bind: "minimumValidationWidth", Usage: "Minimum design validation width in pixels", Trim: true},
			{Name: "debug-host-badge", Bind: "debugHostBadge", Usage: "Target client shows a Debug badge", Kind: LeafBool},
		},
		Constraints: []LeafConstraint{{Kind: LeafExactlyOne, Flags: []string{"conversation-id", "chat-query", "open-dingtalk-id", "user-query"}}}, Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
		Contract: compositeContract("send", "card send", "校验、冻结目标并通过 IM MCP 创建 A2UI 卡片，同时保存本地更新台账", "A2UI create 消息已准备好并需要发送到群聊或单聊时", "普通消息使用 chat message send；发送前仅检查使用 card lint", "dws card send --conversation-id <openConversationId> --file ./messages.json"), Validate: requireExplicitCardProfile, ResultCall: sendCardResult,
	})
}

func newCardUpdateCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "update", Short: "更新已发送的 A2UI 卡片", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "handle", Bind: "handle", Usage: "card send 返回的本地句柄", Required: true, Trim: true},
			{Name: "file", Bind: "file", Usage: "update 消息或期望快照文件", Required: true, Trim: true},
			{Name: "input-mode", Bind: "inputMode", Usage: "messages 或 snapshot", Default: "messages", Enum: []string{"messages", "snapshot"}},
			{Name: "flow-status", Bind: "flowStatus", Usage: "A2UI 流状态", Default: "PROCESSING", Enum: []string{"PROCESSING", "INPUTTING", "FINISH", "EXECUTING", "ERROR", "ABORTED", "TIMEOUT", "CONFIRMING", "CONFIRMED"}},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
		Contract: compositeContract("update", "card update", "更新已发送的 A2UI 卡片；messages 为协议增量，snapshot 自动生成完整组件 upsert 和数据检查点", "已有 card send 返回的 handle，需要增量更新组件或数据时", "结束流式生成时使用 card finish", "dws card update --handle <handle> --file ./delta.json"),
		Validate: requireExplicitCardProfile, ResultCall: updateCardResult,
	})
}

func newCardFinishCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "finish", Short: "结束 A2UI 流，可选发送最后一批更新", OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "handle", Bind: "handle", Usage: "card send 返回的本地句柄", Required: true, Trim: true},
			{Name: "file", Bind: "file", Usage: "可选最后一批 update 消息或期望快照", Trim: true, OmitEmpty: true},
			{Name: "input-mode", Bind: "inputMode", Usage: "messages 或 snapshot", Default: "messages", Enum: []string{"messages", "snapshot"}},
			{Name: "flow-status", Bind: "flowStatus", Usage: "终态 A2UI 流状态", Default: "FINISH", Enum: []string{"FINISH", "ERROR", "ABORTED", "TIMEOUT"}},
		},
		Safety:   contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "unknown"},
		Contract: compositeContract("finish", "card finish", "结束 A2UI 流；可仅更新 flowStatus，也可原子发送最后一批消息", "已有 handle，需要完成、失败、中止或超时终态时", "中间增量使用 card update", "dws card finish --handle <handle>"),
		Validate: requireExplicitCardProfile, ResultCall: updateCardResult,
	})
}

func newCardListCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "list", Short: "列出本地 A2UI 投递台账", OutputRollout: output.RolloutUnifiedActive, Safety: readSafety(), Contract: localContract("list", "card list", "列出本机保存的 A2UI 卡片句柄、状态和修订号", "需要找到后续 update 使用的 handle 时", "需要服务端送达回读时使用 card verify", "dws card list"), ResultCall: func(*cobra.Command, string, map[string]any) (output.CommandResult, error) {
		records, err := (delivery.Store{Dir: delivery.DefaultDir()}).List()
		if err != nil {
			return nil, err
		}
		return output.Success(map[string]any{"cards": records}), nil
	}})
}

func newCardVerifyCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "verify", Short: "核对 A2UI 本地续操作证据", OutputRollout: output.RolloutUnifiedActive, Flags: []LeafFlag{{Name: "handle", Bind: "handle", Usage: "卡片句柄", Required: true, Trim: true}}, Safety: readSafety(), Contract: localContract("verify", "card verify", "核对 handle、bizId、surfaceId、revision 和 flowStatus；当前不把本地台账冒充服务端送达回读", "update 前需要确认本地续操作身份或区分证据层时", "需要真实送达和客户端证据时执行受控 PRE 验收", "dws card verify --handle <handle>"), ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
		record, err := (delivery.Store{Dir: delivery.DefaultDir()}).Load(fmt.Sprint(args["handle"]))
		if err != nil {
			return nil, err
		}
		consistent := record.BizID != "" && record.Surface.SurfaceID != "" && record.Revision > 0
		return output.Success(map[string]any{"handle": record.Handle, "bizId": record.BizID, "surfaceId": record.Surface.SurfaceID, "revision": record.Revision, "flowStatus": record.FlowStatus, "ledgerConsistent": consistent, "deliveryVerified": false, "evidenceLevel": "local_ledger", "nextEvidence": "server readback and client rendering"}), nil
	}})
}

func newCardDoctorCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "doctor", Short: "诊断 A2UI 本地创作与投递依赖", OutputRollout: output.RolloutUnifiedActive, Safety: readSafety(), Contract: localContract("doctor", "card doctor", "分别报告协议包、参考预览、IM 调用接缝和本地台账状态，不发送消息", "A2UI 构建、发送或更新前排查环境时", "它不证明账号授权、群可达或真实客户端渲染", "dws card doctor"), ResultCall: func(_ *cobra.Command, _ string, _ map[string]any) (output.CommandResult, error) {
		registry, err := protocol.Load()
		bundleOK := err == nil
		stateDir := delivery.DefaultDir()
		_, statErr := os.Stat(stateDir)
		stateStatus := "ready"
		if os.IsNotExist(statErr) {
			stateStatus = "not_created"
		} else if statErr != nil {
			stateStatus = "unreadable"
		}
		componentCount := 0
		if registry != nil {
			componentCount = len(registry.ComponentNames())
		}
		executable, executableErr := os.Executable()
		binary := map[string]any{"path": executable, "pathAvailable": executableErr == nil, "cliVersion": buildversion.CurrentVersion(), "vcsRevision": "unknown", "vcsModified": "unknown", "goVersion": "unknown"}
		if info, ok := debug.ReadBuildInfo(); ok {
			binary["goVersion"] = info.GoVersion
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					binary["vcsRevision"] = setting.Value
				case "vcs.modified":
					binary["vcsModified"] = setting.Value
				}
			}
		}
		capabilities := map[string]any{"a2uiSend": true, "a2uiUpdate": true, "a2uiFinish": true, "a2uiSnapshot": true, "visualLint": true, "visualReview": true, "semanticBlocks": true}
		return output.Success(map[string]any{"binary": binary, "capabilities": capabilities, "bundle": map[string]any{"ok": bundleOK, "version": protocol.BundleVersion, "components": componentCount}, "preview": map[string]any{"available": true, "kind": "reference_preview", "realRenderer": false}, "transport": map[string]any{"configured": GetCaller() != nil, "liveAuthorization": "unknown"}, "ledger": map[string]any{"path": stateDir, "status": stateStatus}, "sideEffects": "none"}), nil
	}})
}

func newCardShowCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{Use: "show", Short: "读取一个本地 A2UI 投递记录", OutputRollout: output.RolloutUnifiedActive, Flags: []LeafFlag{{Name: "handle", Bind: "handle", Usage: "卡片句柄", Required: true, Trim: true}}, Safety: readSafety(), Contract: localContract("show", "card show", "读取一个 A2UI 卡片的目标、bizId、状态、修订号和本地 Surface 快照", "更新、排障或审计前需要读取卡片状态时", "只需列表使用 card list", "dws card show --handle <handle>"), ResultCall: func(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
		record, err := (delivery.Store{Dir: delivery.DefaultDir()}).Load(fmt.Sprint(args["handle"]))
		if err != nil {
			return nil, err
		}
		return output.Success(map[string]any{"card": record}), nil
	}})
}

func readSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func requireExplicitCardProfile(cmd *cobra.Command, _ []string) error {
	flag := cmd.Flag("profile")
	if flag == nil || !flag.Changed || strings.TrimSpace(flag.Value.String()) == "" {
		return fmt.Errorf("card send/update requires an explicit --profile to freeze organization and environment identity")
	}
	return nil
}

func resolveCardProfileScope(cmd *cobra.Command) (delivery.ProfileScope, error) {
	flag := cmd.Flag("profile")
	if flag == nil {
		return delivery.ProfileScope{}, fmt.Errorf("card command requires the global --profile flag")
	}
	selector := strings.TrimSpace(flag.Value.String())
	if selector == "" {
		return delivery.ProfileScope{}, fmt.Errorf("card command requires an explicit --profile")
	}
	if runtimeSelector := strings.TrimSpace(profilectx.Get()); runtimeSelector != "" {
		selector = runtimeSelector
	}
	identity := cardCurrentProfileIdentity()
	if strings.TrimSpace(identity.CorpID) == "" || strings.TrimSpace(identity.UserID) == "" {
		resolved, err := cardResolveProfileIdentity(selector)
		if err != nil {
			return delivery.ProfileScope{}, fmt.Errorf("resolve card profile identity: %w", err)
		}
		identity = resolved
	}
	if strings.TrimSpace(identity.CorpID) == "" || strings.TrimSpace(identity.UserID) == "" {
		return delivery.ProfileScope{}, fmt.Errorf("card command requires --profile to resolve to one exact corpId:userId identity")
	}
	canonicalSelector := strings.TrimSpace(identity.CorpID) + ":" + strings.TrimSpace(identity.UserID)
	scope := delivery.ProfileScope{Selector: canonicalSelector, CorpID: strings.TrimSpace(identity.CorpID), UserID: strings.TrimSpace(identity.UserID), Environment: cardDeliveryEnvironment()}
	return scope, nil
}

func cardDeliveryEnvironment() string {
	endpoint := strings.TrimSpace(os.Getenv("DINGTALK_IM_MCP_URL"))
	if endpoint == "" {
		endpoint = strings.TrimSpace(config.GetMCPBaseURL())
	}
	sum := sha256.Sum256([]byte(endpoint))
	return fmt.Sprintf("im:%x", sum[:8])
}

func localContract(name, cliPath, description, useWhen, avoidWhen, example string) LeafContract {
	return LeafContract{Description: description, Result: cardResultSpec, Interface: &contract.InterfaceSpec{Mode: contract.InterfaceModeLocal, Availability: contract.InterfaceAvailable, Reason: "implemented by the embedded DWS A2UI authoring runtime"}, Selection: contract.SelectionSpec{AgentSummary: description, UseWhen: []string{useWhen}, AvoidWhen: []string{avoidWhen}, Examples: []string{example}}, Identity: contract.ToolIdentitySpec{ProductID: "card", Name: name, CanonicalPath: "card." + name, CLIPath: cliPath, PrimaryCLIPath: cliPath}}
}

func compositeContract(name, cliPath, description, useWhen, avoidWhen, example string) LeafContract {
	contract := localContract(name, cliPath, description, useWhen, avoidWhen, example)
	contract.Interface = &contractpkgInterfaceComposite
	return contract
}

var contractpkgInterfaceComposite = contract.InterfaceSpec{Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable, Reason: "local A2UI validation and ledger wrap the IM MCP write"}

func readCardFile(path string) ([]byte, error) {
	if path == "-" {
		return os.ReadFile("/dev/stdin")
	}
	return os.ReadFile(path)
}

func loadMessages(path string) ([]map[string]any, error) {
	raw, err := readCardFile(path)
	if err != nil {
		return nil, err
	}
	return protocol.ParseMessages(bytes.NewReader(raw))
}

func loadSnapshot(path string) (state.Surface, error) {
	raw, err := readCardFile(path)
	if err != nil {
		return state.Surface{}, err
	}
	return state.ParseSnapshot(bytes.NewReader(raw))
}

func loadAndValidate(path, mode string) ([]map[string]any, protocol.Validation, error) {
	messages, err := loadMessages(path)
	if err != nil {
		return nil, protocol.Validation{}, err
	}
	registry, err := protocol.Load()
	if err != nil {
		return nil, protocol.Validation{}, err
	}
	validation := registry.Validate(messages, mode)
	if !validation.Valid {
		return messages, validation, fmt.Errorf("A2UI validation failed with %d diagnostic(s)", len(validation.Diagnostics))
	}
	return messages, validation, nil
}

func composeResult(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
	raw, err := readCardFile(fmt.Sprint(args["file"]))
	if err != nil {
		return nil, err
	}
	var spec authoring.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("invalid CompositionSpec: %w", err)
	}
	effectiveRecipe, err := authoring.ResolveRecipe(spec)
	if err != nil {
		return nil, err
	}
	spec.Recipe = effectiveRecipe
	messages, err := authoring.Compile(spec)
	if err != nil {
		return nil, err
	}
	if path := strings.TrimSpace(fmt.Sprint(args["output"])); path != "" && path != "<nil>" {
		encoded, _ := json.MarshalIndent(messages, "", "  ")
		if err := writeAtomic(path, encoded); err != nil {
			return nil, err
		}
	}
	return output.Success(map[string]any{"recipe": spec.Recipe, "surfaceId": spec.SurfaceID, "messages": messages}), nil
}

func previewResult(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
	messages, validation, err := loadAndValidate(fmt.Sprint(args["file"]), "create")
	if err != nil {
		return nil, err
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		return nil, err
	}
	html, err := preview.Render(surface)
	if err != nil {
		return nil, err
	}
	path, err := filepath.Abs(fmt.Sprint(args["output"]))
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(path, html); err != nil {
		return nil, err
	}
	return output.Success(map[string]any{"previewKind": "reference_preview", "fidelity": "structural", "output": path, "components": preview.ComponentInventory(surface), "validation": validation}), nil
}

func sendCardResult(cmd *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
	profile, err := resolveCardProfileScope(cmd)
	if err != nil {
		return nil, err
	}
	messages, validation, err := loadAndValidate(fmt.Sprint(args["file"]), "create")
	if err != nil {
		return nil, err
	}
	visualContext, err := visualContextFromArgs(args)
	if err != nil {
		return nil, err
	}
	visualReview := authoring.ReviewVisual(messages, visualContext)
	for _, diagnostic := range visualReview.Diagnostics {
		if diagnostic.Severity == "error" {
			return nil, apperrors.NewValidation("card visual review failed: " + diagnostic.Code)
		}
	}
	if err := protocol.ValidateDeliveryResources(messages); err != nil {
		return nil, apperrors.NewValidation(err.Error())
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		return nil, err
	}
	wire, err := protocol.MarshalWire(messages)
	if err != nil {
		return nil, err
	}
	bizCardID, requestID := strings.TrimSpace(fmt.Sprint(args["idempotencyKey"])), uuid.NewString()
	if bizCardID == "" || bizCardID == "<nil>" {
		bizCardID = uuid.NewString()
	}
	params := map[string]any{"requestId": requestID, "bizCardId": bizCardID, "protocolVersion": "1.0", "supportForward": args["supportForward"] == true, "flowStatus": defaultA2UIFlowStatus, "a2uiMessages": wire, "summary": defaultSummary(args, surface)}
	conversationID := strings.TrimSpace(fmt.Sprint(args["conversationId"]))
	if conversationID == "<nil>" {
		conversationID = ""
	}
	if query := strings.TrimSpace(fmt.Sprint(args["chatQuery"])); query != "" && query != "<nil>" {
		conversationID, err = resolveNativeChatTarget(query)
		if err != nil {
			return nil, err
		}
	}
	receiverID := strings.TrimSpace(fmt.Sprint(args["openDingtalkId"]))
	if receiverID == "<nil>" {
		receiverID = ""
	}
	if query := strings.TrimSpace(fmt.Sprint(args["userQuery"])); query != "" && query != "<nil>" {
		receiverID = query
	}
	if conversationID != "" {
		params["openConversationId"] = conversationID
	} else {
		resolved, err := resolveOpenDingTalkID(cmd.Context(), receiverID)
		if err != nil {
			return nil, err
		}
		receiverID = resolved
		params["receiverOpenDingTalkId"] = receiverID
	}
	if GetCaller() != nil && GetCaller().DryRun() {
		return output.Success(map[string]any{"dryRun": true, "executed": false, "validation": validation, "visualReview": visualReview, "request": params}, output.WithDryRun()), nil
	}
	response, err := CallMCPToolDataOnServer(cmd.Context(), "im", "create_and_send_a2ui_card", params)
	if err != nil {
		return nil, err
	}
	bizID := delivery.ExtractBizID(response)
	if bizID == "" {
		bizID = bizCardID
	}
	handle := "a2ui-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	keyHash := sha256.Sum256([]byte(bizCardID))
	record := delivery.Record{Handle: handle, BizID: bizID, CardInstanceID: delivery.ExtractCardInstanceID(response), CreateRequestID: requestID, IdempotencyKeySHA256: fmt.Sprintf("%x", keyHash), Profile: profile, ConversationID: conversationID, ReceiverID: receiverID, Surface: surface, FlowStatus: defaultA2UIFlowStatus, Revision: 1}
	if err := (delivery.Store{Dir: delivery.DefaultDir()}).Save(record); err != nil {
		return nil, fmt.Errorf("card sent but local ledger save failed for bizId %s: %w", bizID, err)
	}
	return output.Success(map[string]any{"handle": handle, "bizId": bizID, "flowStatus": record.FlowStatus, "revision": record.Revision, "validation": validation, "visualReview": visualReview, "response": response}), nil
}

func updateCardResult(cmd *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
	store := delivery.Store{Dir: delivery.DefaultDir()}
	handle := strings.TrimSpace(fmt.Sprint(args["handle"]))
	profile, err := resolveCardProfileScope(cmd)
	if err != nil {
		return nil, err
	}
	var result output.CommandResult
	err = store.WithHandleLock(handle, func() error {
		record, err := store.Load(handle)
		if err != nil {
			return err
		}
		if err := record.ValidateUpdateScope(profile); err != nil {
			return err
		}
		baseRevision := record.Revision
		registry, err := protocol.Load()
		if err != nil {
			return err
		}
		file := strings.TrimSpace(fmt.Sprint(args["file"]))
		mode := fmt.Sprint(args["inputMode"])
		var messages []map[string]any
		if file != "" && file != "<nil>" {
			if mode == "snapshot" {
				desired, err := loadSnapshot(file)
				if err != nil {
					return err
				}
				if desired.CatalogID == "" {
					desired.CatalogID = record.Surface.CatalogID
				}
				if desired.CatalogID != record.Surface.CatalogID {
					return fmt.Errorf("snapshot catalogId %q does not match handle catalogId %q", desired.CatalogID, record.Surface.CatalogID)
				}
				snapshotValidation := registry.Validate(snapshotCreateMessages(desired), "create")
				if !snapshotValidation.Valid {
					return fmt.Errorf("A2UI snapshot validation failed with %d diagnostic(s): %+v", len(snapshotValidation.Diagnostics), snapshotValidation.Diagnostics)
				}
				messages, err = state.Diff(record.Surface, desired)
				if err != nil {
					return err
				}
			} else {
				messages, err = loadMessages(file)
				if err != nil {
					return err
				}
			}
		}
		validation := registry.Validate(messages, "update")
		if !validation.Valid {
			return fmt.Errorf("A2UI update validation failed with %d diagnostic(s): %+v", len(validation.Diagnostics), validation.Diagnostics)
		}
		if err := protocol.ValidateDeliveryResources(messages); err != nil {
			return apperrors.NewValidation(err.Error())
		}
		next, err := state.Reduce(record.Surface, messages)
		if err != nil {
			return err
		}
		if next.SurfaceID == "" {
			return fmt.Errorf("deleteSurface is not supported by stateful card update")
		}
		finalValidation := registry.Validate(snapshotCreateMessages(next), "create")
		if !finalValidation.Valid {
			return fmt.Errorf("A2UI update would leave an invalid surface with %d diagnostic(s): %+v", len(finalValidation.Diagnostics), finalValidation.Diagnostics)
		}
		wire, err := protocol.MarshalWire(messages)
		if err != nil {
			return err
		}
		flowStatus := fmt.Sprint(args["flowStatus"])
		params := map[string]any{"requestId": uuid.NewString(), "bizId": record.BizID, "flowStatus": flowStatus, "a2uiMessages": wire, "a2uiAnnotations": []any{}}
		if GetCaller() != nil && GetCaller().DryRun() {
			result = output.Success(map[string]any{"dryRun": true, "executed": false, "handle": record.Handle, "baseRevision": record.Revision, "validation": validation, "request": params}, output.WithDryRun())
			return nil
		}
		if len(messages) == 0 && flowStatus == record.FlowStatus {
			result = output.Success(map[string]any{"handle": record.Handle, "bizId": record.BizID, "flowStatus": flowStatus, "revision": record.Revision, "noOp": true, "validation": validation})
			return nil
		}
		response, err := CallMCPToolDataOnServer(cmd.Context(), "im", "update_a2ui_card", params)
		if err != nil {
			return err
		}
		record.Surface, record.FlowStatus, record.Revision = next, flowStatus, baseRevision+1
		if err := store.SaveIfRevision(record, baseRevision); err != nil {
			return fmt.Errorf("card updated but local ledger compare-and-save failed: %w", err)
		}
		result = output.Success(map[string]any{"handle": record.Handle, "bizId": record.BizID, "flowStatus": flowStatus, "revision": record.Revision, "validation": validation, "response": response})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func snapshotCreateMessages(surface state.Surface) []map[string]any {
	ids := make([]string, 0, len(surface.Components))
	for id := range surface.Components {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	components := make([]any, 0, len(ids))
	for _, id := range ids {
		components = append(components, surface.Components[id])
	}
	return []map[string]any{
		{"version": "v1.0", "createSurface": map[string]any{"surfaceId": surface.SurfaceID, "catalogId": surface.CatalogID}},
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": surface.SurfaceID, "path": "/", "value": surface.Data}},
		{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": surface.SurfaceID, "components": components}},
	}
}

func defaultSummary(args map[string]any, surface state.Surface) string {
	if value := strings.TrimSpace(fmt.Sprint(args["summary"])); value != "" && value != "<nil>" {
		return value
	}
	if data, ok := surface.Data.(map[string]any); ok {
		if content, ok := data["content"].(map[string]any); ok {
			if title, ok := content["title"].(string); ok {
				return title
			}
		}
	}
	return "A2UI card"
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".dws-card-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
	"github.com/spf13/cobra"
)

type nativeAitableEntityReader struct {
	ctx context.Context
}

func (r nativeAitableEntityReader) CallMCPData(product, tool string, params map[string]any) (map[string]any, error) {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := callMCPReadToolReturnTextOnServer(ctx, product, tool, params)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, apperrors.NewAPI("search_entities 返回空响应",
			apperrors.WithReason("resolution_incomplete"),
			apperrors.WithFailureStage("response_validation"),
			apperrors.WithExecutionStarted(false))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, apperrors.NewAPI("search_entities 返回的不是合法 JSON 对象",
			apperrors.WithReason("resolution_incomplete"),
			apperrors.WithFailureStage("response_validation"),
			apperrors.WithExecutionStarted(false),
			apperrors.WithHint(err.Error()))
	}
	return payload, nil
}

func newAitableEntityCommand() *cobra.Command {
	entityCmd := newGroupCommand(&cobra.Command{
		Use:   "entity",
		Short: "搜索 AI 表格人员、部门和群组实体",
		RunE:  groupRunE,
	})
	searchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索实体候选，不自动选择重名或模糊结果",
		Long: `按人员、部门或群组类型搜索 AI 表格实体候选，并完整读取分页结果。
返回候选名称、消歧描述和可用于实体字段筛选的稳定标识；本命令只读，不执行任何 View 更新。`,
		Example: `  dws aitable entity search --entity-type DEPARTMENT --keyword "客户成功部"
  dws aitable entity search --entity-type PERSON --keyword "张三"
  dws aitable entity search --entity-type GROUP --keyword "项目群"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			entityType, err := aitabletarget.ParseEntityType(mustGetFlag(cmd, "entity-type"))
			if err != nil {
				return err
			}
			keyword := strings.TrimSpace(mustGetFlag(cmd, "keyword"))
			result, err := aitabletarget.SearchEntities(
				nativeAitableEntityReader{ctx: cmd.Context()}, entityType, keyword)
			if err != nil {
				return err
			}
			return deps.Out.PrintJSON(result)
		},
	}
	searchCmd.Flags().String("entity-type", "", "实体类型：PERSON、DEPARTMENT 或 GROUP (必填)")
	searchCmd.Flags().String("keyword", "", "人员、部门或群组显示名称关键词，最长 100 个字符 (必填)")
	_ = searchCmd.MarkFlagRequired("entity-type")
	_ = searchCmd.MarkFlagRequired("keyword")
	DeclareLeafMetadata(searchCmd, LeafSpec{
		Safety: aitableSafetyRead(),
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "aitable",
				Name:           "entity_search",
				CanonicalPath:  "aitable.entity_search",
				CLIPath:        "aitable entity search",
				PrimaryCLIPath: "aitable entity search",
			},
			Description: "搜索 AI 表格人员、部门或群组候选。",
			Interface:   aitableMCPInterface("search_entities"),
			Selection: contract.SelectionSpec{
				AgentSummary: "按显示名称搜索人员、部门或群组候选，并返回稳定标识供筛选使用。",
				UseWhen:      []string{"只有实体显示名称，需要取得 userId+corpId、departmentId 或 cid 时"},
				AvoidWhen:    []string{"已经有经过验证的稳定实体标识时无需搜索；本命令不更新 View"},
				Examples:     []string{"dws aitable entity search --entity-type DEPARTMENT --keyword 客户成功部"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "entity-type", Property: "entityType", Required: boolPtr(true)},
				{Name: "keyword", Property: "keyword", Required: boolPtr(true)},
			},
			Result: &contract.ResultSpec{
				Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{
					"type":"object",
					"properties":{
						"entityType":{"type":"string","description":"实体类型"},
						"keyword":{"type":"string","description":"搜索关键词"},
						"candidates":{"type":"array","description":"完整候选列表","items":{"type":"object","description":"一个实体候选"}},
						"profile":{"type":"string","description":"当前非敏感 profile 标识"}
					}
				}`),
			},
		},
	})
	entityCmd.AddCommand(searchCmd)
	return entityCmd
}

// normalizeAitableViewFilterEntities resolves every entityName before any
// update_view call. It returns a fresh filter tree and never mutates the input.
func normalizeAitableViewFilterEntities(
	filter []any,
	fieldTypes map[string]string,
	reader aitabletarget.Reader,
) ([]any, bool, error) {
	cache := map[string]aitabletarget.EntityResolution{}
	normalized := make([]any, 0, len(filter))
	resolutionPerformed := false
	for index, raw := range filter {
		condition, ok := raw.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("filter[%d] must be an object", index)
		}
		projected, searched, err := normalizeAitableEntityCondition(condition, fieldTypes, reader, cache)
		if err != nil {
			return nil, false, fmt.Errorf("filter[%d]: %w", index, err)
		}
		normalized = append(normalized, projected)
		resolutionPerformed = resolutionPerformed || searched
	}
	return normalized, resolutionPerformed, nil
}

func normalizeAitableEntityCondition(
	condition map[string]any,
	fieldTypes map[string]string,
	reader aitabletarget.Reader,
	cache map[string]aitabletarget.EntityResolution,
) (map[string]any, bool, error) {
	projected := cloneStringAnyMap(condition)
	operator, _ := projected["operator"].(string)
	operands, ok := projected["operands"].([]any)
	if !ok {
		return nil, false, fmt.Errorf("operator %s requires an operands array", operator)
	}
	if isAitableViewLogicalOperator(operator) {
		children := make([]any, 0, len(operands))
		searchedAny := false
		for index, raw := range operands {
			child, ok := raw.(map[string]any)
			if !ok {
				return nil, false, fmt.Errorf("logical operator %s operand %d must be an object", operator, index)
			}
			normalizedChild, searched, err := normalizeAitableEntityCondition(child, fieldTypes, reader, cache)
			if err != nil {
				return nil, false, fmt.Errorf("logical operator %s operand %d: %w", operator, index, err)
			}
			children = append(children, normalizedChild)
			searchedAny = searchedAny || searched
		}
		projected["operands"] = children
		return projected, searchedAny, nil
	}
	if operator == "exist" || operator == "un_exist" || len(operands) < 2 {
		return projected, false, nil
	}
	fieldID, _ := operands[0].(string)
	entityType, isEntity := aitableEntityTypeForField(fieldTypes[strings.TrimSpace(fieldID)])
	if !isEntity {
		return projected, false, nil
	}
	value, searched, err := normalizeAitableEntityFilterValue(operands[1], entityType, reader, cache)
	if err != nil {
		return nil, false, err
	}
	projectedOperands := append([]any{}, operands...)
	projectedOperands[1] = value
	projected["operands"] = projectedOperands
	return projected, searched, nil
}

func normalizeAitableEntityFilterValue(
	value any,
	entityType aitabletarget.EntityType,
	reader aitabletarget.Reader,
	cache map[string]aitabletarget.EntityResolution,
) (any, bool, error) {
	if values, ok := value.([]any); ok {
		if len(values) == 0 {
			return nil, false, invalidAitableEntityReference(entityType, "实体值数组不能为空")
		}
		normalized := make([]any, 0, len(values))
		seen := map[string]bool{}
		searchedAny := false
		for index, item := range values {
			projected, searched, err := normalizeAitableEntityFilterValue(item, entityType, reader, cache)
			if err != nil {
				return nil, false, fmt.Errorf("entity value %d: %w", index, err)
			}
			key := compactJSON(projected)
			if !seen[key] {
				seen[key] = true
				normalized = append(normalized, projected)
			}
			searchedAny = searchedAny || searched
		}
		return normalized, searchedAny, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false, invalidAitableEntityReference(entityType,
			"实体字段不接受裸字符串；请传结构化稳定标识或 {\"entityName\":\"显示名称\"}")
	}
	entityName, _ := object["entityName"].(string)
	entityName = strings.TrimSpace(entityName)
	if entityName != "" {
		if hasAitableStableEntityReference(object) {
			return nil, false, invalidAitableEntityReference(entityType,
				"entityName 不能与稳定实体标识同时提供")
		}
		cacheKey := strings.ToLower(string(entityType) + "\x00" + entityName)
		resolved, exists := cache[cacheKey]
		if !exists {
			var err error
			resolved, err = aitabletarget.ResolveEntity(reader, entityType, entityName)
			if err != nil {
				return nil, false, err
			}
			cache[cacheKey] = resolved
		}
		return aitableReferenceMap(resolved.Selected.Reference), true, nil
	}
	stable, err := normalizeAitableStableReference(entityType, object)
	if err != nil {
		return nil, false, err
	}
	return stable, false, nil
}

func normalizeAitableStableReference(
	entityType aitabletarget.EntityType,
	value map[string]any,
) (map[string]any, error) {
	text := func(key string) string {
		raw, _ := value[key].(string)
		return strings.TrimSpace(raw)
	}
	switch entityType {
	case aitabletarget.EntityPerson:
		if text("departmentId") != "" || text("departmentKey") != "" || text("cid") != "" || text("openConversationId") != "" {
			return nil, invalidAitableEntityReference(entityType, "人员实体不能混入部门或群组标识")
		}
		userID, corpID, userRef := text("userId"), text("corpId"), text("userRef")
		if userRef != "" && (userID != "" || corpID != "") {
			return nil, invalidAitableEntityReference(entityType, "userRef 不能与 userId/corpId 同时提供")
		}
		if (userID == "") != (corpID == "") {
			return nil, invalidAitableEntityReference(entityType, "userId 和 corpId 必须同时提供")
		}
		if userRef != "" {
			return map[string]any{"userRef": userRef}, nil
		}
		if userID != "" {
			return map[string]any{"userId": userID, "corpId": corpID}, nil
		}
	case aitabletarget.EntityDepartment:
		if text("userId") != "" || text("corpId") != "" || text("userRef") != "" || text("cid") != "" || text("openConversationId") != "" {
			return nil, invalidAitableEntityReference(entityType, "部门实体不能混入人员或群组标识")
		}
		departmentID, departmentKey := text("departmentId"), text("departmentKey")
		if departmentID != "" && departmentKey != "" && departmentID != departmentKey {
			return nil, invalidAitableEntityReference(entityType,
				"departmentId 与 departmentKey 同时存在但值不同")
		}
		if departmentID != "" {
			return map[string]any{"departmentId": departmentID}, nil
		}
		if departmentKey != "" {
			return map[string]any{"departmentKey": departmentKey}, nil
		}
	case aitabletarget.EntityGroup:
		if text("userId") != "" || text("corpId") != "" || text("userRef") != "" || text("departmentId") != "" || text("departmentKey") != "" {
			return nil, invalidAitableEntityReference(entityType, "群组实体不能混入人员或部门标识")
		}
		cid, openConversationID := text("cid"), text("openConversationId")
		if cid != "" && openConversationID != "" {
			return nil, invalidAitableEntityReference(entityType, "cid 与 openConversationId 只能提供一个")
		}
		if cid != "" {
			return map[string]any{"cid": cid}, nil
		}
		if openConversationID != "" {
			// DWS only validates the public identifier shape. MCP owns the
			// openConversationId -> cid conversion and persists the cid.
			return map[string]any{"openConversationId": openConversationID}, nil
		}
	}
	return nil, invalidAitableEntityReference(entityType, "缺少该实体类型需要的稳定标识")
}

func aitableEntityTypeForField(fieldType string) (aitabletarget.EntityType, bool) {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "user", "person":
		return aitabletarget.EntityPerson, true
	case "department":
		return aitabletarget.EntityDepartment, true
	case "group":
		return aitabletarget.EntityGroup, true
	default:
		return "", false
	}
}

func aitableReferenceMap(reference aitabletarget.EntityReference) map[string]any {
	switch {
	case reference.UserID != "" && reference.CorpID != "":
		return map[string]any{"userId": reference.UserID, "corpId": reference.CorpID}
	case reference.UserRef != "":
		return map[string]any{"userRef": reference.UserRef}
	case reference.DepartmentID != "":
		return map[string]any{"departmentId": reference.DepartmentID}
	case reference.DepartmentKey != "":
		return map[string]any{"departmentKey": reference.DepartmentKey}
	case reference.CID != "":
		return map[string]any{"cid": reference.CID}
	default:
		return map[string]any{}
	}
}

func hasAitableStableEntityReference(value map[string]any) bool {
	for _, key := range []string{"userId", "corpId", "userRef", "departmentId", "departmentKey", "cid", "openConversationId"} {
		if raw, ok := value[key].(string); ok && strings.TrimSpace(raw) != "" {
			return true
		}
	}
	return false
}

func invalidAitableEntityReference(entityType aitabletarget.EntityType, hint string) error {
	return apperrors.NewValidation("AI 表格实体筛选值无效",
		apperrors.WithReason("invalid_entity_reference"),
		apperrors.WithFailureStage("target_resolution"),
		apperrors.WithExecutionStarted(false),
		apperrors.WithHint(hint),
		apperrors.WithDetails(map[string]any{"entityType": entityType}))
}

// compareAitableViewFilterReadBack compares the new external projection when
// complete, otherwise safely falls back to the proven department/group legacy
// identity equivalence. Person internal keys remain unknown without MCP proof.
func compareAitableViewFilterReadBack(
	view map[string]any,
	requested any,
	normalizedExpected any,
	fieldTypes map[string]string,
) (matched bool, unknown bool, actual any) {
	if complete, ok := view["filterExternalComplete"].(bool); ok && complete {
		actual = view["filterExternal"]
		if persistedViewFilterMatches(actual, requested) || persistedViewFilterMatches(actual, normalizedExpected) {
			return true, false, actual
		}
	}
	if actual == nil {
		actual = walkViewPath(view, "filter")
	}
	// A current MCP response already uses the same normalized internal value
	// that get_views returns, so prefer that exact comparison before applying
	// legacy external-identity projection for older responses.
	if persistedViewFilterMatches(actual, normalizedExpected) {
		return true, false, actual
	}
	projected, projectionUnknown := projectPersistedAitableEntityFilter(actual, fieldTypes)
	if projectionUnknown {
		return false, true, actual
	}
	if persistedViewFilterMatches(projected, requested) || persistedViewFilterMatches(projected, normalizedExpected) {
		return true, false, actual
	}
	return false, false, actual
}

// normalizedAitableViewFilterFromUpdateResponse returns the filter projection
// produced by MCP after field-value conversion. In particular, MCP converts a
// group openConversationId to the cid that get_views subsequently returns.
func normalizedAitableViewFilterFromUpdateResponse(raw string) (any, bool) {
	var envelope map[string]any
	if json.Unmarshal([]byte(raw), &envelope) != nil {
		return nil, false
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return nil, false
	}
	filter, exists := data["filter"]
	return filter, exists
}

func projectPersistedAitableEntityFilter(value any, fieldTypes map[string]string) (any, bool) {
	root, ok := canonicalAitableViewFilter(value)
	if !ok {
		return value, false
	}
	projected, unknown := projectPersistedAitableEntityCondition(root, fieldTypes)
	return projected, unknown
}

func projectPersistedAitableEntityCondition(
	condition map[string]any,
	fieldTypes map[string]string,
) (map[string]any, bool) {
	projected := cloneStringAnyMap(condition)
	operands, ok := condition["operands"].([]any)
	if !ok {
		return projected, false
	}
	operator, _ := condition["operator"].(string)
	if isAitableViewLogicalOperator(operator) {
		children := make([]any, 0, len(operands))
		unknown := false
		for _, raw := range operands {
			child, ok := raw.(map[string]any)
			if !ok {
				children = append(children, raw)
				continue
			}
			projectedChild, childUnknown := projectPersistedAitableEntityCondition(child, fieldTypes)
			children = append(children, projectedChild)
			unknown = unknown || childUnknown
		}
		projected["operands"] = children
		return projected, unknown
	}
	if len(operands) < 2 {
		return projected, false
	}
	fieldID, _ := operands[0].(string)
	entityType, isEntity := aitableEntityTypeForField(fieldTypes[strings.TrimSpace(fieldID)])
	if !isEntity {
		return projected, false
	}
	value, unknown := projectPersistedAitableEntityValue(operands[1], entityType)
	projectedOperands := append([]any{}, operands...)
	projectedOperands[1] = value
	projected["operands"] = projectedOperands
	return projected, unknown
}

func projectPersistedAitableEntityValue(value any, entityType aitabletarget.EntityType) (any, bool) {
	if values, ok := value.([]any); ok {
		projected := make([]any, 0, len(values))
		unknown := false
		for _, item := range values {
			converted, itemUnknown := projectPersistedAitableEntityValue(item, entityType)
			projected = append(projected, converted)
			unknown = unknown || itemUnknown
		}
		return projected, unknown
	}
	if object, ok := value.(map[string]any); ok {
		stable, err := normalizeAitableStableReference(entityType, object)
		if err == nil {
			return stable, false
		}
		return value, true
	}
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return value, true
	}
	switch entityType {
	case aitabletarget.EntityDepartment:
		return map[string]any{"departmentId": text}, false
	case aitabletarget.EntityGroup:
		return map[string]any{"cid": text}, false
	case aitabletarget.EntityPerson:
		return value, true
	default:
		return value, false
	}
}

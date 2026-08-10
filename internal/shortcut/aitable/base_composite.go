// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

var BaseSchemaSnapshot = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-schema-snapshot",
	Product:     serverMain,
	Description: "读取 Base、全部数据表、字段和视图的可复用结构快照，并严格校验每层响应",
	Intent:      "当你要审计、迁移或复制一个 Base 的完整结构但不需要记录数据时使用；明确空 tables/fields/views 合法，缺失容器或缺少请求 ID 则失败。",
	Risk:        shortcut.RiskRead,
	Safety: contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	},
	Contract: aitableCompositeContract(
		"+base-schema-snapshot",
		"读取 Base、全部数据表、字段和视图的可复用结构快照，并严格校验每层响应",
		"当你要审计、迁移或复制一个 Base 的完整结构但不需要记录数据时使用；明确空 tables/fields/views 合法，缺失容器或缺少请求 ID 则失败。",
		"需要记录数据用 record query；只看 Base 目录用 base get；快照不会创建或修改资源",
		"dws aitable +base-schema-snapshot --base-id BASE_ID",
	),
	Flags: []shortcut.Flag{{Name: "base-id", Type: shortcut.FlagString, Desc: "Base ID", Required: true}},
	Tips:  []string{`dws aitable +base-schema-snapshot --base-id BASE_ID`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeBaseSchemaSnapshot(rt)
	},
}

var BaseBootstrap = shortcut.Shortcut{
	Service:     "aitable",
	Command:     "+base-bootstrap",
	Product:     serverMain,
	Description: "一次创建 Base、数据表和字段，逐层读回验证并在中断时报告已知副作用",
	Intent:      "当你已有声明式 tables JSON、想一次搭好一套 AI 表格结构时使用；表内字段自动按 15 个拆批，每次创建都读回验证。",
	Risk:        shortcut.RiskWrite,
	Safety: contract.SafetySpec{
		Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent",
	},
	Contract: aitableCompositeContract(
		"+base-bootstrap",
		"一次创建 Base、数据表和字段，逐层读回验证并在中断时报告已知副作用",
		"当你已有声明式 tables JSON、想一次搭好一套 AI 表格结构时使用；表内字段自动按 15 个拆批，每次创建都读回验证。",
		"已有 Base 只需新增一张表时用 table create；复制现有 Base 用 base copy；不要对失败请求盲目重试",
		`dws aitable +base-bootstrap --name "项目管理" --tables '[{"name":"任务","fields":[]}]'`,
	),
	Flags: []shortcut.Flag{
		{Name: "name", Type: shortcut.FlagString, Desc: "新 Base 名称", Required: true},
		{Name: "folder-id", Type: shortcut.FlagString, Desc: "目标知识库文件夹 ID（可选）"},
		{Name: "template-id", Type: shortcut.FlagString, Desc: "模板 ID（可选）"},
		{Name: "tables", Type: shortcut.FlagString, Desc: `表结构 JSON 数组：[{'name':'任务','fields':[...]}]`, Required: true},
	},
	Tips: []string{`dws aitable +base-bootstrap --name "项目管理" --tables '[{"name":"任务","fields":[]}]'`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		return executeBaseBootstrap(rt)
	},
}

type bootstrapTable struct {
	Name   string
	Fields []any
}

func parseBootstrapTables(raw string) ([]bootstrapTable, error) {
	value, err := parseJSONAny("tables", raw)
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, apperrors.NewValidation("--tables 必须是非空 JSON 数组")
	}
	if len(items) > 100 {
		return nil, apperrors.NewValidation("--tables 最多接受 100 张表")
	}
	seen := map[string]bool{}
	out := make([]bootstrapTable, 0, len(items))
	for index, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("--tables[%d] 必须是 JSON 对象", index))
		}
		name := strings.TrimSpace(stringValue(object, "name"))
		if name == "" || seen[name] {
			return nil, apperrors.NewValidation(fmt.Sprintf("--tables[%d].name 必须非空且不能重复", index))
		}
		seen[name] = true
		fields := []any{}
		if rawFields, exists := object["fields"]; exists {
			var fieldsOK bool
			fields, fieldsOK = rawFields.([]any)
			if !fieldsOK {
				return nil, apperrors.NewValidation(fmt.Sprintf("--tables[%d].fields 必须是 JSON 数组", index))
			}
		}
		for fieldIndex, field := range fields {
			object, ok := field.(map[string]any)
			if !ok || strings.TrimSpace(stringValue(object, "fieldName", "name")) == "" || strings.TrimSpace(stringValue(object, "type")) == "" {
				return nil, apperrors.NewValidation(fmt.Sprintf("--tables[%d].fields[%d] 必须包含 fieldName 和 type", index, fieldIndex))
			}
		}
		out = append(out, bootstrapTable{Name: name, Fields: fields})
	}
	return out, nil
}

func executeBaseSchemaSnapshot(rt *shortcut.RuntimeContext) error {
	baseID := rt.Str("base-id")
	base, err := rt.CallMCPData(serverMain, "get_base", map[string]any{"baseId": baseID})
	if err != nil {
		return err
	}
	if !deepContainsString(base, baseID) {
		return fmt.Errorf("get_base response does not identify requested baseId %s", baseID)
	}
	tables, found := findNamedObjectList(base, "tables", "tableList", "sheets")
	if !found {
		return fmt.Errorf("get_base response is missing the tables collection")
	}
	result := newCompositeResult("base_schema_snapshot")
	result.RequestedCount = len(tables)
	result.Resolved = map[string]any{"baseId": baseID}
	tableSnapshots := make([]any, 0, len(tables))
	for index, table := range tables {
		tableID := stringValue(table, "tableId", "sheetId", "id")
		if tableID == "" {
			return fmt.Errorf("get_base tables[%d] is missing tableId", index)
		}
		detail, err := rt.CallMCPData(serverMain, "get_tables", map[string]any{"baseId": baseID, "tableIds": []string{tableID}})
		if err != nil || !deepContainsString(detail, tableID) {
			if err == nil {
				err = fmt.Errorf("get_tables response does not identify tableId %s", tableID)
			}
			return err
		}
		fieldsData, err := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": baseID, "tableId": tableID})
		if err != nil {
			return err
		}
		fields, found := findNamedObjectList(fieldsData, "fields", "fieldList")
		if !found {
			return fmt.Errorf("get_fields response for %s is missing the fields collection", tableID)
		}
		viewsData, err := rt.CallMCPData(serverMain, "get_views", map[string]any{"baseId": baseID, "tableId": tableID})
		if err != nil {
			return err
		}
		views, found := findNamedObjectList(viewsData, "views", "viewList")
		if !found {
			return fmt.Errorf("get_views response for %s is missing the views collection", tableID)
		}
		tableSnapshots = append(tableSnapshots, map[string]any{
			"tableId": tableID, "summary": table, "detail": detail, "fields": fields, "views": views,
		})
		result.CompletedCount = index + 1
	}
	result.Result = map[string]any{"base": base, "tables": tableSnapshots}
	result.Verification = map[string]any{"status": "verified", "tableCount": len(tables)}
	return rt.Output(result)
}

func executeBaseBootstrap(rt *shortcut.RuntimeContext) error {
	tables, err := parseBootstrapTables(rt.Str("tables"))
	if err != nil {
		return err
	}
	result := newCompositeResult("base_bootstrap")
	result.RequestedCount = len(tables)
	result.Plan = append(result.Plan, compositeStep{Index: 1, Name: "create base", Tool: "create_base", Status: "planned"})
	for index, table := range tables {
		result.Plan = append(result.Plan, compositeStep{Index: index + 2, Name: "create and verify table", Tool: "create_table", Status: "planned", Count: len(table.Fields), Arguments: map[string]any{"name": table.Name}})
	}
	if rt.DryRun() {
		result.Status = "planned"
		result.Executed = false
		return rt.Output(result)
	}
	baseArgs := map[string]any{"baseName": rt.Str("name")}
	if rt.Changed("folder-id") {
		baseArgs["folderId"] = rt.Str("folder-id")
	}
	if rt.Changed("template-id") {
		baseArgs["templateId"] = rt.Str("template-id")
	}
	baseData, err := rt.CallMCPWriteDataStrict(serverMain, "create_base", baseArgs)
	if err != nil {
		result.Status = "unknown"
		result.FailedCount = len(tables)
		result.Checkpoint = map[string]any{"nextStep": "resolve whether base was created before retrying"}
		return compositeError(result, err, false)
	}
	baseID := findStringByKeys(baseData, "baseId")
	if baseID == "" {
		result.Status = "unknown"
		result.Result = baseData
		result.Checkpoint = map[string]any{"nextStep": "locate the created Base by exact name before retrying"}
		return compositeError(result, fmt.Errorf("create_base response is missing baseId"), false)
	}
	result.Resolved = map[string]any{"baseId": baseID}
	result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "create_base", "baseId": baseID})
	result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: 1, Name: "create base", Tool: "create_base", Status: "completed", Result: baseData})
	baseRead, err := rt.CallMCPData(serverMain, "get_base", map[string]any{"baseId": baseID})
	if err != nil || !deepContainsString(baseRead, baseID) {
		if err == nil {
			err = fmt.Errorf("get_base does not identify created baseId %s", baseID)
		}
		result.Status = "partial_success"
		result.Checkpoint = map[string]any{"nextTableIndex": 0, "baseId": baseID}
		return compositeError(result, err, false)
	}

	createdTables := make([]any, 0, len(tables))
	for index, spec := range tables {
		initialEnd := minInt(15, len(spec.Fields))
		initialFields := spec.Fields[:initialEnd]
		createData, createErr := rt.CallMCPWriteDataStrict(serverMain, "create_table", map[string]any{
			"baseId": baseID, "tableName": spec.Name, "fields": initialFields,
		})
		tableID := findStringByKeys(createData, "tableId", "sheetId")
		if createErr != nil || tableID == "" {
			if createErr == nil {
				createErr = fmt.Errorf("create_table response for %q is missing tableId", spec.Name)
			}
			result.Status = "partial_success"
			result.CompletedCount = index
			result.FailedCount = len(tables) - index
			result.Checkpoint = map[string]any{"nextTableIndex": index, "baseId": baseID}
			return compositeError(result, createErr, false)
		}
		result.KnownEffects = append(result.KnownEffects, map[string]any{"tool": "create_table", "baseId": baseID, "tableId": tableID, "name": spec.Name})
		for offset := initialEnd; offset < len(spec.Fields); offset += 15 {
			end := minInt(offset+15, len(spec.Fields))
			_, fieldErr := rt.CallMCPWriteDataStrict(serverMain, "create_fields", map[string]any{
				"baseId": baseID, "tableId": tableID, "fields": spec.Fields[offset:end],
			})
			if fieldErr != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("create_fields offset %d returned an error; checking final field state: %v", offset, fieldErr))
			}
		}
		detail, verifyErr := rt.CallMCPData(serverMain, "get_tables", map[string]any{"baseId": baseID, "tableIds": []string{tableID}})
		if verifyErr != nil || !deepContainsString(detail, tableID) {
			if verifyErr == nil {
				verifyErr = fmt.Errorf("get_tables does not identify created tableId %s", tableID)
			}
			result.Status = "partial_success"
			result.CompletedCount = index
			result.FailedCount = len(tables) - index
			result.Checkpoint = map[string]any{"nextTableIndex": index, "baseId": baseID, "tableId": tableID, "step": "verify table and fields"}
			return compositeError(result, verifyErr, false)
		}
		fieldsData, verifyErr := rt.CallMCPData(serverMain, "get_fields", map[string]any{"baseId": baseID, "tableId": tableID})
		fields, found := findNamedObjectList(fieldsData, "fields", "fieldList")
		if verifyErr != nil || !found || !containsAllFieldNames(fields, spec.Fields) {
			if verifyErr == nil {
				verifyErr = fmt.Errorf("field read-back for table %s does not contain the declared field set", tableID)
			}
			result.Status = "partial_success"
			result.CompletedCount = index
			result.FailedCount = len(tables) - index
			result.Checkpoint = map[string]any{"nextTableIndex": index, "baseId": baseID, "tableId": tableID, "step": "repair fields"}
			return compositeError(result, verifyErr, false)
		}
		result.CompletedCount = index + 1
		result.CompletedSteps = append(result.CompletedSteps, compositeStep{Index: index + 2, Name: "create and verify table", Tool: "create_table", Status: "completed", Count: len(spec.Fields), Result: map[string]any{"tableId": tableID, "fieldCount": len(fields)}})
		createdTables = append(createdTables, map[string]any{"tableId": tableID, "name": spec.Name, "fieldCount": len(fields)})
	}
	result.Verification = map[string]any{"status": "verified", "baseId": baseID, "tableCount": len(createdTables)}
	result.Result = map[string]any{"baseId": baseID, "tables": createdTables}
	return rt.Output(result)
}

func findNamedObjectList(value any, names ...string) ([]map[string]any, bool) {
	nameSet := map[string]bool{}
	for _, name := range names {
		nameSet[strings.ToLower(name)] = true
	}
	var walk func(any) ([]map[string]any, bool)
	walk = func(current any) ([]map[string]any, bool) {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		for key, child := range object {
			if nameSet[strings.ToLower(key)] {
				items, ok := child.([]any)
				if !ok {
					return nil, false
				}
				out := make([]map[string]any, 0, len(items))
				for _, item := range items {
					entry, ok := item.(map[string]any)
					if !ok {
						return nil, false
					}
					out = append(out, entry)
				}
				return out, true
			}
		}
		for _, child := range object {
			if found, ok := walk(child); ok {
				return found, true
			}
		}
		return nil, false
	}
	return walk(value)
}

func stringValue(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return value
		}
	}
	return ""
}

func findStringByKeys(value any, keys ...string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(key)] = true
	}
	var walk func(any) string
	walk = func(current any) string {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if keySet[strings.ToLower(key)] {
					if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
						return strings.TrimSpace(text)
					}
				}
			}
			for _, child := range typed {
				if found := walk(child); found != "" {
					return found
				}
			}
		case []any:
			for _, child := range typed {
				if found := walk(child); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(value)
}

func deepContainsString(value any, expected string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if deepContainsString(child, expected) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if deepContainsString(child, expected) {
				return true
			}
		}
	case string:
		return typed == expected
	}
	return false
}

func containsAllFieldNames(actual []map[string]any, expected []any) bool {
	names := map[string]bool{}
	for _, field := range actual {
		names[stringValue(field, "fieldName", "name")] = true
	}
	for _, raw := range expected {
		field, _ := raw.(map[string]any)
		if !names[stringValue(field, "fieldName", "name")] {
			return false
		}
	}
	return true
}

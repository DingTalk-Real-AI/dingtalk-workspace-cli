// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// AitableNavigationGuidance keeps object delivery and URL preservation separate from business state.
const AitableNavigationGuidance = "本段仅为展示约定，不代表业务状态。" +
	"以实体名称展示返回的访问链接（如 url/links、shareFormUrl），名称缺失时使用对象类型标签。" +
	"按用户目标逐对象交付已返回的最具体链接；仅有父级入口时明确说明，不冒充目标直达。" +
	"完整保留 URL 及参数（含百分号编码和参数顺序），逐字符原样使用，不得删除、脱敏、解码、重编码或补全；已有真实链接时不按模板重建。" +
	"正文通常无需罗列裸 ID，用户索要或排障时可展示，此例外不适用于 URL。" +
	"链接有无不代表写入完成、定位成功、访问权限或分享状态。"

func aitableNavigationDryRunResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, bool) {
	if corecmd.BoolFlag(cmd, "dry-run") {
		return output.Success(map[string]any{"tool": tool, "arguments": args, "executed": false}, output.WithDryRun()), true
	}
	return aitableUnifiedDryRunResult(tool, args)
}

// AitableNavigationBusinessResultSpec describes data after unwrapping the MCP envelope.
func AitableNavigationBusinessResultSpec() *contract.ResultSpec {
	object := AitableNavigationObjectSchema()
	properties := object["properties"].(map[string]any)
	for key, description := range map[string]string{
		"baseName": "服务端返回的 Base 名称", "newBaseId": "复制生成的新 Base ID",
		"appId": "服务端返回的应用编码标识", "warningMessage": "服务端核验提示",
	} {
		properties[key] = map[string]any{"type": "string", "description": description}
	}
	properties["verificationStatus"].(map[string]any)["description"] = "原服务端核验状态；显式返回时仅 verified 成功，pending 需按真实对象 ID 只读核对；其他值或缺少恢复 ID 均失败。Base 创建、复制、更新兼容未提供此字段的服务端回执，调用成功不等于独立读回已核实"
	// Distinguish the business result from the generic dry-run preview.
	object["not"] = map[string]any{"required": []string{"executed"}, "properties": map[string]any{"executed": map[string]any{"type": "boolean", "description": "远端调用是否已执行", "const": false}}}
	schema, _ := json.Marshal(object)
	return &contract.ResultSpec{
		Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure},
		DataSchema: aitableResultSchemaWithDryRun("对象变更结果及服务端导航链接", schema),
	}
}

// ExecuteAitableNavigationResult changes presentation only: one write, no extra read or retry.
func ExecuteAitableNavigationResult(cmd *cobra.Command, tool string, args map[string]any) error {
	if preview, ok := aitableNavigationDryRunResult(cmd, tool, args); ok {
		return output.StoreResult(cmd.Context(), preview)
	}
	data, err := callAitableUnifiedDataContext(cmd.Context(), tool, args)
	if err != nil {
		return err
	}
	object, ok := data.(map[string]any)
	if !ok {
		return apperrors.NewInternal(fmt.Sprintf("aitable/%s 返回的 data 不是 JSON 对象", tool))
	}
	status, hasStatus := object["verificationStatus"]
	// These Base DTOs predate verificationStatus; omission remains a call receipt.
	if !hasStatus && (tool == "create_base" || tool == "copy_base" || tool == "update_base") {
		return output.StoreResult(cmd.Context(), output.Success(data))
	}
	switch status {
	case "verified":
		return output.StoreResult(cmd.Context(), output.Success(data))
	case "pending":
		if operation := aitableNavigationPendingOperation(tool, args, object); operation != nil {
			return output.StoreResult(cmd.Context(), output.Pending(data, operation))
		}
	}
	started := true
	return output.StoreResult(cmd.Context(), output.Failure(&output.ErrorInfo{
		Type: "api", Subtype: "projection_unknown", Message: "服务端核验状态无效或缺少恢复 ID，请保留回执并只读核对目标，不要自动重放写入",
		ExecutionStarted: &started, Details: map[string]any{"result": data},
	}))
}

func aitableNavigationPendingOperation(tool string, args, object map[string]any) *output.OperationInfo {
	baseID, _ := args["baseId"].(string)
	var id, next string
	switch tool {
	case "create_base", "copy_base":
		// A copy must recover the new Base, never the source from args.
		baseID, _ = object["baseId"].(string)
		fallthrough
	case "update_base":
		id = baseID
		next = "dws aitable base get --base-id " + ShellQuoteArg(baseID) + " --format json"
	case "update_table":
		id, _ = args["tableId"].(string)
		next = fmt.Sprintf("dws aitable table get --base-id %s --table-ids %s --format json", ShellQuoteArg(baseID), ShellQuoteArg(id))
	}
	if strings.TrimSpace(baseID) == "" || strings.TrimSpace(id) == "" {
		return nil
	}
	return &output.OperationInfo{ID: id, State: "pending", NextCommand: next}
}

// AitableNavigationObjectSchema returns a fresh business schema shared by atomic and composite results.
func AitableNavigationObjectSchema() map[string]any {
	return map[string]any{"type": "object", "description": "对象访问结果；状态保留原有语义", "additionalProperties": true,
		"properties": map[string]any{
			"baseId":             map[string]any{"type": "string", "description": "所属 Base ID；真实回显时提供"},
			"tableId":            map[string]any{"type": "string", "description": "所属数据表 ID，对应 sheetId"},
			"tableName":          map[string]any{"type": "string", "description": "服务端返回的数据表名称"},
			"dashboardId":        map[string]any{"type": "string", "description": "仪表盘真实 ID，对应 applicationId"},
			"dashboardName":      map[string]any{"type": "string", "description": "服务端返回的仪表盘名称"},
			"fieldId":            map[string]any{"type": "string", "description": "字段真实 ID，对应 fieldId"},
			"fieldName":          map[string]any{"type": "string", "description": "已知字段名称；未核实的更新值不冒充服务端状态"},
			"viewId":             map[string]any{"type": "string", "description": "视图或表单真实 ID，对应 viewId"},
			"flowId":             map[string]any{"type": "string", "description": "自动化真实 ID，对应 flowId"},
			"verificationStatus": map[string]any{"type": "string", "description": "原服务端核验状态，不由链接存在与否推导"},
			"url":                map[string]any{"type": []string{"string", "null"}, "description": "携带对象定位标识的访问链接；数据表用 sheetId，仪表盘用 applicationId；显式 null 保留，不代表写后核验完成"},
			"links": map[string]any{"type": []string{"array", "null"}, "description": "目标对象与父级上下文链接；显式 null 保留", "items": map[string]any{"type": "object", "additionalProperties": true, "properties": map[string]any{
				"objectType": map[string]any{"type": "string", "description": "对象类型，与字段业务 type 分开"},
				"objectId":   map[string]any{"type": "string", "description": "真实对象 ID"},
				"name":       map[string]any{"type": "string", "description": "已知对象名称，未知时省略"},
				"relation":   map[string]any{"type": "string", "description": "target 或 parent"},
				"url":        map[string]any{"type": "string", "description": "对象访问链接"},
			}}},
		}}
}

// aitableNavigationResultSpec describes existing legacy MCP output without changing its envelope.
func aitableNavigationResultSpec(batch bool) *contract.ResultSpec {
	object := AitableNavigationObjectSchema()
	if batch {
		object = map[string]any{"type": "object", "description": "批量字段逐项结果；pending/unknown/partial 不因链接存在而改变", "additionalProperties": true, "properties": map[string]any{"results": map[string]any{"type": "array", "description": "按请求顺序返回的逐字段回执", "items": object}}}
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "description": "MCP 原始响应信封；不重塑输出层级", "additionalProperties": true, "properties": map[string]any{"data": object}})
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: schema}
}

// AitableObjectNavigationResultSpec shares the existing MCP envelope with direct object Shortcut aliases.
func AitableObjectNavigationResultSpec() *contract.ResultSpec {
	return aitableNavigationResultSpec(false)
}

// AitableTablesNavigationResultSpec describes get_tables without changing the legacy envelope.
func AitableTablesNavigationResultSpec() *contract.ResultSpec {
	data := map[string]any{"type": "object", "description": "数据表查询结果", "additionalProperties": true,
		"properties": map[string]any{"tables": map[string]any{"type": "array", "description": "已命中的表及其访问链接", "items": AitableNavigationObjectSchema()}}}
	schema, _ := json.Marshal(map[string]any{"type": "object", "description": "get_tables 的 MCP 原始响应信封", "additionalProperties": true, "properties": map[string]any{"data": data}})
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: schema}
}

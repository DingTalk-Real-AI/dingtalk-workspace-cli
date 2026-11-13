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

// AitableNavigationGuidance publishes per-target closure checks, process links, and outcome boundaries independently of the MCP envelope.
const AitableNavigationGuidance = "收尾：生成最终回复前，按用户请求、已确认的交付范围及当前状态清点最终目标，" +
	"对有当前有效、身份与用途匹配入口的目标，按宿主规则逐目标完成登记与呈现，并核对正文链接，不以父级入口合并替代。" +
	"成功回执、relation=target 或过程对象不自动成为最终产物；正文链接不替代规定的登记。" +
	"过程已展示链接或宿主已呈现卡片，最终 Markdown 正文仍须保留最终目标的有效具名完整链接（`[对象名称](完整URL)`）；过程去重不免除此要求。缺少适用入口时说明缺口。" +
	"业务核验与宿主交付分别判断。" +
	"链接保真必经步骤：仅当存在明确识别为 JSON 格式的原始业务回执文件时，展示前必须先调用本地 JSON 解析器，从该回执文件提取每条实际待展示的业务 URL，并与实际链接严格比较；核对成功后才生成展示内容。CSV、XLS、XLSX 等非 JSON 导入附件不是业务回执，不作为 JSON 解析；没有上述 JSON 回执文件时，直接使用当前命令的结构化回执核对并展示链接，不因非 JSON 附件阻断成功结果展示。不手抄 JSON 充作源数据，不将聊天摘要当原文；源文件不可用时不伪造输入文件。只提取允许展示的业务 URL，凭据及失效入口不回显。此核对只读本地文件，不调用 DWS、网络或宿主；百分号编码中的字母大小写须原样保留，即使访问等价也不可互换。" +
	"过程与链接：对用户本次要求处理且有当前有效、适用入口的对象，必须逐对象展示具名完整链接。" +
	"不同对象不合并为单条 Base 入口，不以父级替代已有目标深链；过程内已展示的同对象同有效入口可去重。" +
	"仅有父级入口时明确说明，不冒充目标直达。" +
	"内部自动对象不自动纳入，用户要求的对象不因自动创建而排除。" +
	"入口取实际返回的 url/links 或已确认可用的 shareFormUrl；target/parent 仅表调用关系。" +
	"按链接的对象类型与对象 ID 判断入口身份，不按 relation 字面含义判断；身份匹配的 links 即使 relation=parent 仍是当前对象入口，只有链接身份确属父级时才称父级入口。" +
	"links[].name 对应同项 URL 对象；缺名用类型或任务标签，仅在仍无法区分时附 ID，不编造名称。" +
	"凭据按既有规则脱敏；其余业务对象 URL（含参数顺序及百分号编码）须逐字符完整原样保留，不重建。" +
	"先逐对象提取本条回执的完整原始 URL，直接用于该对象的链接目标；不套用其他对象的 URL 替换 ID。" +
	"直接从本条回执复制完整 URL，不重新键入、解码再编码或统一大小写；同一批链接各自保留编码，前一条的 %3d 不决定后一条的编码，如 %3D 不改为 %3d。" +
	"发送前将实际待发送链接与本条回执逐字符比对，有一处不同就修正；只声称原样保留不算核对。" +
	"缺链仅按本工具明确规则及真实标识回退，否则说明缺口、不臆造；表单管理页不得冒充填写页。" +
	"交付给填写者且填写入口未就绪时，只说明缺口，不展示或登记管理页链接；即使注明非填写入口，也不将管理页列为可交付内容。" +
	"只有用户明确要求管理操作或接收者为管理者时，才交付当前可用的管理页，并注明管理页，非填写入口。" +
	"边界：登记或展示失败、能力不可用或结果未确认时，保留业务结果并说明缺口，不为补链接或交付重放业务写入。" +
	"业务回执或展示请求提交不证明卡片已登记、已渲染或面板已打开；" +
	"链接有无不证明写入完成、定位成功、访问权限或分享状态。"

// AitableBaseListFallbackGuidance permits only the reviewed Base list/search fallback; it never rebuilds returned links.
const AitableBaseListFallbackGuidance = "仅 Base 列表／搜索结果未返回目标 url/links 时，可用本次返回的真实 baseId 按 https://alidocs.dingtalk.com/i/nodes/{baseId} 拼接 Base 入口，baseId 按路径段编码一次；已有目标链接优先原样使用，不重建。该许可不适用于表单或其他对象，不证明定位、权限、分享或写入状态。"

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

// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// AitableFormFieldUpdateConstraint keeps the conditional update rule visible in both leaves' parameter help and Schema.
const AitableFormFieldUpdateConstraint = "至少一项有效更新；设置与清除条件互斥；clear=false 不算更新"

// AitableFormFieldUpdateParameters declares the same RPC properties on both executable leaves.
func AitableFormFieldUpdateParameters() []contract.ParamDecl {
	return []contract.ParamDecl{
		{Name: "base-id", Property: "baseId"}, {Name: "table-id", Property: "tableId"},
		{Name: "view-id", Property: "viewId"}, {Name: "field-id", Property: "fieldId"},
		{Name: "required", Property: "required", Required: boolPtr(false)},
		{Name: "field-description", Property: "fieldDescription", Required: boolPtr(false)},
		{Name: "display-condition", Property: "displayCondition", InterfaceType: "object", Required: boolPtr(false)},
		{Name: "clear-display-condition", Property: "clearDisplayCondition", InterfaceType: "boolean", Required: boolPtr(false)},
	}
}

// AitableFormFieldUpdateParams preserves Changed, notably required=false and clear=false.
func AitableFormFieldUpdateParams(cmd *cobra.Command) (map[string]any, error) {
	params := map[string]any{}
	for flag, property := range map[string]string{"required": "required", "clear-display-condition": "clearDisplayCondition"} {
		if cmd.Flags().Changed(flag) {
			raw := cmd.Flags().Lookup(flag).Value.String()
			// The atomic string flag historically treats an explicit empty value as omitted.
			if flag == "required" && raw == "" {
				continue
			}
			value, err := strconv.ParseBool(strings.TrimSpace(raw))
			if err != nil {
				return nil, apperrors.NewValidation("--" + flag + " 必须是 true/false")
			}
			params[property] = value
		}
	}
	if cmd.Flags().Changed("field-description") {
		value, _ := cmd.Flags().GetString("field-description")
		// Preserve the atomic entry's empty-string omission; it must not clear the description.
		if value != "" {
			if strings.TrimSpace(value) == "" {
				return nil, apperrors.NewValidation("--field-description 不能为空")
			}
			params["fieldDescription"] = value
		}
	}
	if cmd.Flags().Changed("display-condition") {
		if params["clearDisplayCondition"] == true {
			return nil, apperrors.NewValidation("设置与清除显示条件不能同时使用")
		}
		raw, _ := cmd.Flags().GetString("display-condition")
		condition, err := parseAitableJSONObjectFlag("display-condition", raw, true)
		if err != nil {
			return nil, err
		}
		if err = validateFormDisplayCondition(condition); err != nil {
			return nil, err
		}
		params["displayCondition"] = condition
	}
	if _, required := params["required"]; !required && params["fieldDescription"] == nil && params["displayCondition"] == nil && params["clearDisplayCondition"] != true {
		return nil, apperrors.NewValidation("至少传入 required、field-description、display-condition 或 clear-display-condition=true")
	}
	return params, nil
}

func validateFormDisplayCondition(condition map[string]any) error {
	invalid := func() error {
		return apperrors.NewValidation("显示条件须为单层 and/or 根，包含 1 至 20 个 eq [字段ID,选项ID]；清除请用 --clear-display-condition")
	}
	if len(condition) != 2 || (condition["operator"] != "and" && condition["operator"] != "or") {
		return invalid()
	}
	operands, ok := condition["operands"].([]any)
	if !ok || len(operands) == 0 || len(operands) > 20 {
		return invalid()
	}
	for _, item := range operands {
		leaf, ok := item.(map[string]any)
		if !ok || len(leaf) != 2 || leaf["operator"] != "eq" {
			return invalid()
		}
		values, ok := leaf["operands"].([]any)
		if !ok || len(values) != 2 {
			return invalid()
		}
		for _, value := range values {
			if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
				return invalid()
			}
		}
	}
	return nil
}

// AitableFormFieldUpdateResultSpec describes acknowledged writes separately from verified completion.
func AitableFormFieldUpdateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
		DataSchema: aitableResultSchemaWithDryRun("字段配置写后核验；pending/unknown/旧服务缺少证明时为 partial_failure（退出码7），不自动重放", json.RawMessage(`{"oneOf":[
{"type":"object","properties":{
"baseId":{"type":"string","description":"已核验Base ID"},"tableId":{"type":"string","description":"已核验Table ID"},
"viewId":{"type":"string","description":"已核验表单视图ID"},"fieldId":{"type":"string","description":"已核验题目ID"},
"verificationStatus":{"type":"string","const":"verified","description":"独立读回已匹配，只有此状态为成功"},
"completedOperations":{"type":"array","description":"已确认完成的子操作，不表示原子回滚","items":{"type":"string"}},
"actual":{"type":"object","description":"独立读回的实际字段配置","properties":{
"fieldId":{"type":"string","description":"实际题目ID"},"required":{"type":"boolean","description":"实际必填状态"},"hidden":{"type":"boolean","description":"实际固定隐藏状态"},
"description":{"type":["string","null"],"description":"实际题目描述"},
"displayCondition":{"type":["object","null"],"description":"标准条件；仅在状态available时，null或缺失表示已确认无条件","additionalProperties":true},
"displayConditionStatus":{"type":"string","description":"available表示已确认读取；unknown/unsupported或状态缺失不能解释为条件已清除"}
},"required":["fieldId","required","hidden"],"additionalProperties":true}
},"required":["baseId","tableId","viewId","fieldId","verificationStatus","actual"],"additionalProperties":true},
{"type":"object","properties":{
"total":{"type":"integer","description":"响应接收和业务核验两个阶段","const":2},
"succeeded":{"type":"array","description":"仅确认收到回执，response保留原始MCP信封及completedOperations","items":{"type":"object","additionalProperties":true}},
"failed":{"type":"array","description":"独立核验未完成的原因；execution_started=true，禁止重放","items":{"type":"object","additionalProperties":true}},
"unknown":{"type":"array","description":"保留统一部分结果结构","items":{"type":"object","additionalProperties":true}}
},"required":["total","succeeded","failed","unknown"],"additionalProperties":false}
]}`)),
	}
}

// RunAitableFormFieldUpdate invokes once; both adapters share this boundary and never retry mutations.
func RunAitableFormFieldUpdate(cmd *cobra.Command, params map[string]any) error {
	// Honour the executable flag even when a host caller has not mirrored it yet.
	if flag := cmd.Flag("dry-run"); flag != nil && flag.Value.String() == "true" {
		return output.StoreResult(cmd.Context(), output.Success(map[string]any{
			"tool": "update_form_field", "arguments": params, "executed": false,
		}, output.WithDryRun()))
	}
	if preview, ok := aitableUnifiedDryRunResult("update_form_field", params); ok {
		return output.StoreResult(cmd.Context(), preview)
	}
	raw, err := CallMCPToolDataOnServer(cmd.Context(), "aitable-helper", "update_form_field", params)
	if err != nil {
		// The common parser preserves the original business envelope in CLIError.Message.
		// Recover only this tool's typed partial receipt, never arbitrary transport errors.
		var cliErr *CLIError
		var envelope map[string]any
		if errors.As(err, &cliErr) && json.Unmarshal([]byte(cliErr.Message), &envelope) == nil {
			if data, ok := envelope["data"].(map[string]any); ok && data["verificationStatus"] != nil {
				return output.StoreResult(cmd.Context(), AitableFormFieldUpdateResult(envelope, params))
			}
		}
		return aitableFormFieldUpdateError(err)
	}
	envelope, _ := raw.(map[string]any)
	return output.StoreResult(cmd.Context(), AitableFormFieldUpdateResult(envelope, params))
}

// Only these form mutations interpret organization errors; other MCP tools keep their existing classification.
func aitableFormFieldUpdateError(err error) error {
	var cliErr *CLIError
	var body map[string]any
	if !errors.As(err, &cliErr) || json.Unmarshal([]byte(cliErr.Message), &body) != nil || !isBusinessError(body) {
		return err
	}
	code := businessErrorCode(body)
	nested, _ := body["error"].(map[string]any)
	kind, _ := nested["type"].(string)
	if code != "601" && code != "ORG_NOT_MATCH" && kind != "ORG_DOC_TOKEN_MISMATCH" {
		return err
	}
	trace, _ := nested["traceId"].(string)
	return apperrors.NewAuth("请求组织与文档所属组织不匹配",
		apperrors.WithOrigin("mcp"), apperrors.WithReason("organization_mismatch"),
		apperrors.WithRetryable(false), apperrors.WithCause(err),
		apperrors.WithServerKey("aitable-helper"), apperrors.WithOperation("aitable-helper/update_form_field"),
		apperrors.WithServerDiag(apperrors.ServerDiagnostics{ServerErrorCode: code, TraceID: strings.TrimSpace(trace), ErrorType: kind}),
		apperrors.WithHint("先核对当前 profile 与文档所属组织，使用已有授权且组织匹配的 profile；不自动切换账号、刷新凭据、申请权限或重放写入。"))
}

// AitableFormFieldUpdateResult requires matching target and independent actual state, not just echoed IDs.
// Only an explicit available condition state can confirm a clear when the gateway omits null values.
func AitableFormFieldUpdateResult(envelope, request map[string]any) output.CommandResult {
	data, ok := envelope["data"].(map[string]any)
	status, hasStatus := envelope["status"]
	acknowledged := status == "success" || (!hasStatus && envelope["success"] == true)
	verified := ok && acknowledged && data["verificationStatus"] == "verified" && envelope["success"] != false
	for _, key := range []string{"baseId", "tableId", "viewId", "fieldId"} {
		verified = verified && reflect.DeepEqual(data[key], request[key]) && request[key] != nil
	}
	actual, actualOK := data["actual"].(map[string]any)
	_, requiredOK := actual["required"].(bool)
	_, hiddenOK := actual["hidden"].(bool)
	verified = verified && actualOK && requiredOK && hiddenOK && reflect.DeepEqual(actual["fieldId"], request["fieldId"])
	if value, exists := request["required"]; exists {
		verified = verified && reflect.DeepEqual(actual["required"], value)
	}
	if value, exists := request["fieldDescription"]; exists {
		verified = verified && reflect.DeepEqual(actual["description"], value)
	}
	if condition, exists := request["displayCondition"]; exists {
		verified = verified && actual["displayConditionStatus"] == "available" && reflect.DeepEqual(condition, actual["displayCondition"])
	}
	if request["clearDisplayCondition"] == true {
		// The gateway may omit null map values. Missing alone is not proof: keep the explicit available guard.
		verified = verified && actual["displayCondition"] == nil && actual["displayConditionStatus"] == "available"
	}
	if verified {
		return output.Success(data)
	}
	started := true
	return output.Partial(&output.PartialData{
		Total:     2,
		Succeeded: []any{map[string]any{"id": "remote_response", "response": envelope}},
		Failed: []output.PartialFailedEntry{{ID: "verification", Error: &output.ErrorInfo{
			Type: "api", Subtype: "form_field_update_unverified", Message: "字段更新尚未独立核验完成", Operation: "aitable-helper/update_form_field", Origin: "mcp", Stage: "response_validation", ExecutionStarted: &started,
			Hint: "保留已完成子操作，按原目标只读核对；不自动重放，不取消必填或永久隐藏作为替代。",
		}}}, Unknown: []output.PartialUnknownEntry{},
	})
}

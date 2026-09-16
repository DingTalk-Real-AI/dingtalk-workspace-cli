package helpers

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const aitableFormSharePartialResultSchema = `{
  "type":"object",
  "description":"终态未验证（包括响应 baseId/tableId/viewId 与本次请求不一致）：ok=false、outcome=partial_failure、退出码7。succeeded仅表示已收到远端响应，不表示配置或CP已成功；原始data保留在succeeded[0].response。failed[0].error.execution_started=true，不自动重放写入。",
  "properties":{
    "total":{"type":"integer","const":2,"description":"响应接收与终态验证两个阶段"},
    "succeeded":{"type":"array","description":"仅确认远端响应已收到；response保留原始data","minItems":1,"maxItems":1,"items":{"type":"object","properties":{"id":{"type":"string","const":"remote_response","description":"已完成的响应接收阶段"},"response":{"description":"未经修补的远端data，可为空或畸形"}},"required":["id","response"]}},
    "failed":{"type":"array","description":"终态验证失败，error含execution_started=true及诊断信息","minItems":1,"maxItems":1,"items":{"type":"object","properties":{"id":{"type":"string","const":"terminal_verification","description":"未通过的终态校验阶段"},"error":{"type":"object","description":"统一ErrorInfo，远端已开始执行，不自动重放写入","additionalProperties":true}},"required":["id","error"]}},
    "unknown":{"type":"array","description":"无额外未知阶段","maxItems":0,"items":{"type":"object","additionalProperties":true}}
  },
  "required":["total","succeeded","failed","unknown"],
  "additionalProperties":false
}`

// AitableFormShareUpdateResult is the shared terminal projection for atomic and
// shortcut writes. IDs must match the original request exactly: another form's
// remote response is not proof of CP convergence for the requested target.
func AitableFormShareUpdateResult(data any, baseID, tableID, viewID string) output.CommandResult {
	invalid := invalidFormShareStateFields(data, baseID, tableID, viewID)
	if len(invalid) == 0 {
		return output.Success(data)
	}
	started := true
	return output.Partial(&output.PartialData{
		Total: 2,
		// This stage only acknowledges receipt, never a verified business update.
		Succeeded: []any{map[string]any{"id": "remote_response", "response": data}},
		Failed: []output.PartialFailedEntry{{ID: "terminal_verification", Error: &output.ErrorInfo{
			Type: "api", Subtype: "form_share_state_unverified",
			Message:   "表单分享写调用已返回，但终态校验未通过：" + strings.Join(invalid, ", "),
			Operation: "aitable-helper/update_share_form", Origin: "mcp",
			Stage: "response_validation", ExecutionStarted: &started,
			Hint:    "远端已开始执行，不能确认分享闭环完成；先核对服务端状态，不自动重放写操作，也不调用 View 更新补偿 CP。get 仅诊断分享配置，不能证明 CP 已同步。",
			Details: map[string]any{"invalid_fields": invalid},
		}}},
		Unknown: []output.PartialUnknownEntry{},
	})
}

func invalidFormShareStateFields(raw any, baseID, tableID, viewID string) []string {
	data, ok := raw.(map[string]any)
	if !ok || data == nil {
		return []string{"data"}
	}
	var invalid []string
	for _, selector := range []struct{ key, expected string }{
		{"baseId", baseID}, {"tableId", tableID}, {"viewId", viewID},
	} {
		if value, ok := data[selector.key].(string); !ok || strings.TrimSpace(selector.expected) == "" || value != selector.expected {
			invalid = append(invalid, selector.key)
		}
	}
	if _, ok := data["enabled"].(bool); !ok {
		invalid = append(invalid, "enabled")
	}
	// MCP JSON numbers are float64; retain support for exact JSON and Go ints.
	var integral bool
	switch value := data["status"].(type) {
	case float64:
		integral = !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value
	case int, int64:
		integral = true
	case json.Number:
		_, err := value.Int64()
		integral = err == nil
	}
	if !integral {
		invalid = append(invalid, "status")
	}
	if synced, ok := data["cpSynced"].(bool); !ok || !synced {
		invalid = append(invalid, "cpSynced")
	}
	for _, key := range []string{"shareFormUuid", "formCover", "formName", "formDesc"} {
		if value := data[key]; value != nil {
			if _, ok := value.(string); !ok {
				invalid = append(invalid, key)
			}
		}
	}
	return invalid
}

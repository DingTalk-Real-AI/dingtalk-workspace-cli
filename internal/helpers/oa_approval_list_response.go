package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// normalizeOAApprovalListResponse aligns the approval list APIs' legacy string envelopes with
// the typed OA envelope. RawMessage keeps business fields (including large IDs)
// intact; only the reviewed top-level success and error-code fields change.
// It is shared by success rendering and business-error diagnostics.
func normalizeOAApprovalListResponse(text string) (map[string]json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &body); err != nil || body == nil {
		return nil, &CLIError{Code: CodeMCPToolError, Message: "审批列表返回了无效的 JSON 对象"}
	}
	if raw, ok := body["success"]; ok {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		switch value {
		case true, "true":
			body["success"] = json.RawMessage("true")
		case false, "false":
			body["success"] = json.RawMessage("false")
		default:
			return nil, &CLIError{Code: CodeMCPToolError, Message: "审批列表响应 success 必须为布尔值"}
		}
	}
	for _, key := range []string{"errorCode", "error_code"} {
		if raw, ok := body[key]; ok {
			code := string(raw)
			if len(raw) > 0 && raw[0] == '"' {
				// The outer Unmarshal validated every RawMessage; the leading
				// quote also proves this value is a JSON string.
				_ = json.Unmarshal(raw, &code)
			}
			n, err := strconv.ParseInt(code, 10, 64)
			if err != nil {
				return nil, &CLIError{Code: CodeMCPToolError, Message: fmt.Sprintf("审批列表响应 %s 必须为整数", key)}
			}
			body["errorCode"] = json.RawMessage(strconv.FormatInt(n, 10))
		}
	}
	delete(body, "error_code")
	return body, nil
}

func renderOAApprovalListResponse(text string) error {
	body, err := normalizeOAApprovalListResponse(text)
	if err != nil {
		return err
	}
	if deps.Caller.Format() == "json" {
		return deps.Out.PrintJSON(body)
	}
	// Normalization preserves validated RawMessages and only inserts valid
	// boolean/integer literals, so this map is always JSON-serializable.
	raw, _ := json.Marshal(body)
	deps.Out.PrintRaw(string(raw))
	return nil
}

// Keep success rendering and business-error diagnostics on the same RPC set.
func hasOAApprovalListEnvelope(serverID, toolName string) bool {
	if serverID != "oa" {
		return false
	}
	switch toolName {
	case "get_todo_tasks", "get_done_tasks", "get_submitted_instances", "get_noticed_instances":
		return true
	default:
		return false
	}
}

package helpers

import (
	"encoding/json"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const aitableFormSharePartialResultSchema = `{
  "type":"object",
  "description":"终态未验证（包括 enabled/status 相互矛盾，或响应 baseId/tableId/viewId 与本次请求不一致，或本次显式请求的 enabled/formName/formDesc 与响应回读值不一致，以及其他已回读配置不符合本次请求语义）：ok=false、outcome=partial_failure、退出码7。succeeded仅表示已收到远端响应，不表示配置或CP已成功；原始data保留在succeeded[0].response。failed[0].error.execution_started=true，不自动重放写入。",
  "properties":{
    "total":{"type":"integer","const":2,"description":"响应接收与终态验证两个阶段"},
    "succeeded":{"type":"array","description":"仅确认远端响应已收到；response保留原始data","minItems":1,"maxItems":1,"items":{"type":"object","properties":{"id":{"type":"string","const":"remote_response","description":"已完成的响应接收阶段"},"response":{"description":"未经修补的远端data，可为空或畸形"}},"required":["id","response"]}},
    "failed":{"type":"array","description":"终态验证失败，error含execution_started=true及诊断信息","minItems":1,"maxItems":1,"items":{"type":"object","properties":{"id":{"type":"string","const":"terminal_verification","description":"未通过的终态校验阶段"},"error":{"type":"object","description":"统一ErrorInfo，远端已开始执行，不自动重放写入","properties":{"details":{"type":"object","description":"终态诊断与可保留的资源标识，不是写入成功证明","properties":{"invalid_fields":{"type":"array","description":"未通过终态校验的字段","items":{"type":"string"}},"warnings":` + aitableFormShareWarningsSchema + `,"formCoverResourceId":{"type":"string","description":"本次请求使用的封面资源 ID（含本地上传获得的 ID）；仅在请求提供时返回，用于保留资源供后续人工核验，不自动重传或重放配置写入"}},"additionalProperties":true}},"additionalProperties":true}},"required":["id","error"]}},
    "unknown":{"type":"array","description":"无额外未知阶段","maxItems":0,"items":{"type":"object","additionalProperties":true}}
  },
  "required":["total","succeeded","failed","unknown"],
  "additionalProperties":false
}`

// Request target IDs, in the order they are reported as invalid fields.
var formShareRequestTargetFields = []string{"baseId", "tableId", "viewId"}

// These fields have reviewed read-back semantics. Older servers may omit the
// newer fields; absence remains unverified rather than proving a successful update.
var formShareProvableRequestFields = []string{
	"enabled", "formName", "formDesc", "authTypeCode", "authData",
	"submitTimesLimit", "submitTimesUserLimit", "formStartTime", "formEndTime",
	"anonymousSubmit", "loadLastSubmit", "submitRecordViewMode", "replyNotice",
	"shareUidList", "formCover",
}

// Only reviewed, external-facing suggestion fields enter the business result.
// The upstream meta object can carry other diagnostics and is never copied wholesale.
type formShareWarning struct {
	Action   string `json:"action,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// AitableFormShareGetResult keeps identity-unavailable warnings with the state
// they describe. A null member field must not be interpreted as an empty list.
func AitableFormShareGetResult(envelope map[string]any) (output.CommandResult, error) {
	data, ok := envelope["data"].(map[string]any)
	if !ok || data == nil {
		return nil, apperrors.NewInternal("get_share_form_config 返回值缺少 JSON 对象 data")
	}
	return output.Success(formShareStateWithWarnings(data, formShareWarnings(envelope["meta"]))), nil
}

// AitableFormShareUpdateResult is the shared terminal projection for atomic and
// shortcut writes. It takes the whole update request: IDs must match exactly
// because another form's remote response is not proof of CP convergence for the
// requested target, and an explicitly requested value that the response echoes
// differently is not the update that was asked for.
func AitableFormShareUpdateResult(envelope map[string]any, request map[string]any) output.CommandResult {
	data := envelope["data"]
	warnings := formShareWarnings(envelope["meta"])
	invalid, unverified := inspectFormShareState(data, request)
	if len(invalid) == 0 {
		state := formShareStateWithWarnings(data.(map[string]any), warnings)
		state["verified"] = len(unverified) == 0
		if len(unverified) > 0 {
			state["unverified"] = unverified
		}
		return output.Success(state)
	}
	started := true
	details := map[string]any{"invalid_fields": invalid}
	if len(warnings) > 0 {
		details["warnings"] = warnings
	}
	// A stored partial result is not a Go error: preserve the upload recovery ID here,
	// without rewriting the remote response or suggesting another write to verify CP.
	if resourceID, ok := request["formCoverResourceId"].(string); ok && strings.TrimSpace(resourceID) != "" {
		details["formCoverResourceId"] = resourceID
	}
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
			Details: details,
		}}},
		Unknown: []output.PartialUnknownEntry{},
	})
}

func inspectFormShareState(raw any, request map[string]any) (invalid, unverified []string) {
	data, ok := raw.(map[string]any)
	if !ok || data == nil {
		return []string{"data"}, nil
	}
	rejected := make(map[string]bool, len(request))
	reject := func(key string) {
		if rejected[key] {
			return
		}
		rejected[key] = true
		invalid = append(invalid, key)
	}
	for _, key := range formShareRequestTargetFields {
		expected, _ := request[key].(string)
		if value, ok := data[key].(string); !ok || strings.TrimSpace(expected) == "" || value != expected {
			reject(key)
		}
	}
	enabled, enabledOK := data["enabled"].(bool)
	if !enabledOK {
		reject("enabled")
	}
	status, statusOK := formShareInteger(data["status"])
	if !statusOK {
		reject("status")
	}
	// MCP derives enabled from status == 1. A contradictory pair cannot prove
	// convergence even when both fields have valid types and cpSynced is true.
	if enabledOK && statusOK && enabled != (status == 1) {
		reject("enabled")
		reject("status")
	}
	if synced, ok := data["cpSynced"].(bool); !ok || !synced {
		reject("cpSynced")
	}
	for _, key := range []string{"shareFormUuid", "shareFormUrl", "formCover", "formName", "formDesc"} {
		if value := data[key]; value != nil {
			if _, ok := value.(string); !ok {
				reject(key)
			}
		}
	}
	for _, key := range formShareProvableRequestFields {
		expected, requested := request[key]
		if !requested {
			continue
		}
		actual, present := data[key]
		if (!present || actual == nil) && key != "enabled" && key != "formName" && key != "formDesc" {
			unverified = append(unverified, key)
			continue
		}
		matches, verifiable := formShareRequestedFieldMatches(key, expected, actual)
		if !verifiable {
			unverified = append(unverified, key)
		} else if !matches {
			reject(key)
		}
	}
	for key := range request {
		if slices.Contains(formShareRequestTargetFields, key) || slices.Contains(formShareProvableRequestFields, key) {
			continue
		}
		// In particular, formCover is a public URL, not proof of which resource ID
		// produced it. Keep formCoverResourceId unverified without another write/read.
		unverified = append(unverified, key)
	}
	sort.Strings(unverified)
	return invalid, unverified
}

// Compare the public read-back representation, not Go numeric types or the
// internal member JSON. Business validation and server-side normalization remain in MCP.
func formShareRequestedFieldMatches(key string, expected, actual any) (matches, verifiable bool) {
	switch key {
	case "enabled", "anonymousSubmit", "loadLastSubmit", "replyNotice":
		want, wantOK := expected.(bool)
		got, gotOK := actual.(bool)
		return wantOK && gotOK && want == got, true
	case "formName", "formDesc", "formCover":
		want, wantOK := expected.(string)
		got, gotOK := actual.(string)
		return wantOK && gotOK && want == got, true
	case "shareUidList":
		want, wantOK := expected.(string)
		got, gotOK := actual.(string)
		return wantOK && gotOK && formShareUserListsMatch(want, got), true
	case "authData":
		return formShareAuthDataMatches(expected, actual)
	default:
		want, wantOK := formShareInteger(expected)
		got, gotOK := formShareInteger(actual)
		if !wantOK || !gotOK {
			return false, true
		}
		if (key == "authTypeCode" || key == "submitTimesUserLimit") && want == -1 {
			// The reviewed MCP protocol treats UNKNOWN as an omitted update.
			return true, true
		}
		if key == "submitTimesLimit" {
			// MCP accepts the provider's dynamic cap after a successful mutation.
			// Zero means unlimited, so it only verifies an explicit clear, never
			// a positive limit. Negative inputs normalize to that same explicit clear.
			if want <= 0 {
				return got == 0, true
			}
			return got > 0 && got <= want, true
		}
		return want == got, true
	}
}

// JSON numbers decode as float64; reject fractions, overflow and non-finite
// values before converting so distinct timestamps cannot compare equal by truncation.
func formShareInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case json.Number:
		n, err := value.Int64()
		return n, err == nil
	case float64:
		if !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value && value >= math.MinInt64 && value < -float64(math.MinInt64) {
			return int64(value), true
		}
	}
	return 0, false
}

func formShareUserSet(raw string) (map[string]bool, bool) {
	users := map[string]bool{}
	if raw == "" {
		return users, true
	}
	for _, user := range strings.Split(raw, ",") {
		user = strings.TrimSpace(user)
		if user == "" {
			return nil, false
		}
		users[user] = true
	}
	return users, true
}

func formShareUserListsMatch(expected, actual string) bool {
	want, wantOK := formShareUserSet(expected)
	got, gotOK := formShareUserSet(actual)
	return wantOK && gotOK && maps.Equal(want, got)
}

// authData writes a userId CSV and reads a public member JSON. Missing external
// identities (including legacy uid-only responses) cannot prove membership.
func formShareAuthDataMatches(expected, actual any) (matches, verifiable bool) {
	wantRaw, wantOK := expected.(string)
	gotRaw, gotOK := actual.(string)
	if !wantOK || !gotOK {
		return false, true
	}
	// MCP preserves legacy explicit clearing outside specified-member mode.
	// Only the exact empty-string read-back proves that clear, never a null or member JSON.
	if wantRaw == "" {
		return gotRaw == "", true
	}
	want, valid := formShareUserSet(wantRaw)
	if !valid || len(want) == 0 {
		return false, true
	}
	var payload struct {
		Members []struct {
			Type   string `json:"type"`
			UserID string `json:"userId"`
			CorpID string `json:"corpId"`
		} `json:"members"`
	}
	if json.Unmarshal([]byte(gotRaw), &payload) != nil || payload.Members == nil {
		return false, true
	}
	got := map[string]bool{}
	corpID := ""
	for _, member := range payload.Members {
		if member.Type != "user" && member.Type != "staff" {
			return false, true
		}
		if strings.TrimSpace(member.UserID) == "" || strings.TrimSpace(member.CorpID) == "" {
			return false, false
		}
		if corpID != "" && corpID != member.CorpID {
			return false, true
		}
		corpID = member.CorpID
		got[member.UserID] = true
	}
	return maps.Equal(want, got), true
}

func formShareWarnings(raw any) []formShareWarning {
	meta, _ := raw.(map[string]any)
	suggestions, _ := meta["suggestions"].([]any)
	var warnings []formShareWarning
	for _, raw := range suggestions {
		suggestion, _ := raw.(map[string]any)
		text := func(key string) string {
			value, _ := suggestion[key].(string)
			return strings.TrimSpace(value)
		}
		warning := formShareWarning{Action: text("action"), Reason: text("reason"), Priority: text("priority")}
		if warning.Action != "" || warning.Reason != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func formShareStateWithWarnings(data map[string]any, warnings []formShareWarning) map[string]any {
	state := maps.Clone(data)
	if len(warnings) > 0 {
		state["warnings"] = warnings
	}
	return state
}

package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// devAppSuccessNextSteps adds follow-up suggestions only after the caller has
// classified the response as a terminal success. It never executes a step.
func devAppSuccessNextSteps(tool string, params map[string]any, content map[string]any) []map[string]any {
	// Pending/failure responses retain their existing recovery instructions.
	switch strings.ToLower(devAppContentString(content, "outcome")) {
	case "pending", "failure", "unknown":
		return nil
	}
	if devAppParamBool(params, "precheckOnly") {
		switch tool {
		case devAppScopeSetTool:
			return devAppScopeSetSuccessNextSteps(params, content)
		case devAppVersionPublishTool:
			return devAppVersionCheckApprovalSuccessNextSteps(devAppSuccessIDs(params, content))
		default:
			return nil
		}
	}
	if devAppToolMayNeedVersionPublish(tool) {
		if steps := devAppVersionPublishSuccessNextSteps(params, content); len(steps) > 0 {
			return steps
		}
	}
	switch tool {
	case devAppCreateTool, devAppUpdateTool, devAppEnableTool, devAppDisableTool:
		return devAppReadBackStep(params, content, "get", "check_app", "核对应用信息和启停状态")
	case devAppDeleteTool:
		return []map[string]any{devAppNextStep(devAppStep{ID: "check_app_list", Command: "dws dev app list --format json", DoneWhen: "结合分页结果确认应用已删除"})}
	case devAppScopeSetTool:
		return devAppReadBackStep(params, content, "scope get", "check_scope", "核对当前生效的可见范围")
	case devAppWebappSetTool:
		return devAppReadBackStep(params, content, "webapp get", "check_webapp", "核对网页应用配置")
	case devAppPermissionAddTool, devAppPermissionRmTool:
		return devAppReadBackStep(params, content, "permission list", "check_permissions", "结合分页结果核对应用权限")
	case devAppMemberAddTool, devAppMemberRemoveTool:
		return devAppReadBackStep(params, content, "member list", "check_members", "核对应用成员")
	case devAppSecurityConfigTool:
		return []map[string]any{{"id": "verify_security_config", "title": "验证安全配置", "requiresUserInput": true, "blocking": false, "doneWhen": "在开发者后台核对配置，并从预期运行环境验证访问"}}
	case devAppEventSubscribeTool, devAppEventUnsubscribeTool:
		return devAppReadBackStep(params, content, "event list", "check_events", "结合分页结果核对事件订阅")
	case devAppRobotConfigUpsertTool, devAppRobotEnableTool, devAppRobotOfflineTool:
		return devAppReadBackStep(params, content, "robot get", "check_robot", "核对机器人配置和状态")
	case devAppRobotSubmitTool:
		taskID := devAppFirstContentString(content, "taskId")
		if taskID != "" {
			if quotedTaskID, ok := devAppQuoteScopeValue(taskID); ok {
				return []map[string]any{devAppNextStep(devAppStep{ID: "check_robot_result", Command: "dws dev app robot result --task-id " + quotedTaskID + " --format json", DoneWhen: "返回机器人创建结果"})}
			}
			return []map[string]any{devAppMissingParamsStep("check_robot_result", "返回机器人创建结果", "taskId")}
		}
	case devAppVersionCreateTool:
		return devAppVersionCreateSuccessNextSteps(devAppSuccessIDs(params, content))
	case devAppVersionPublishTool:
		if strings.EqualFold(devAppContentString(content, "approvalMode"), "SELECT_APPROVER") {
			return nil
		}
		return devAppVersionCreateSuccessNextSteps(devAppSuccessIDs(params, content))
	case devAppListTool, devAppGetTool, devAppCredentialsGetTool, devAppScopeGetTool,
		devAppWebappGetTool, devAppPermissionListTool, devAppMemberListTool,
		devAppEventListTool, devAppRobotConfigGetTool, devAppRobotResultTool,
		devAppVersionListTool, devAppVersionDetailTool, devAppVersionStatusTool:
		// Reads are terminal. Robot lifecycle guidance is supplied by its normalizer.
		return nil
	}
	return nil
}

// Only identifiers may fall back to invocation parameters; never echo secrets.
func devAppSuccessIDs(params, content map[string]any) map[string]any {
	merged := make(map[string]any, len(content)+2)
	for key, value := range content {
		merged[key] = value
	}
	for _, key := range []string{"unifiedAppId", "versionId"} {
		if devAppContentString(merged, key) == "" {
			merged[key] = devAppParamString(params, key)
		}
	}
	return merged
}

func devAppReadBackStep(params, content map[string]any, path, id, doneWhen string) []map[string]any {
	appID := devAppContentString(devAppSuccessIDs(params, content), "unifiedAppId")
	if appID == "" {
		return nil
	}
	quotedAppID, ok := devAppQuoteScopeValue(appID)
	if !ok {
		return []map[string]any{devAppMissingParamsStep(id, doneWhen, "unifiedAppId")}
	}
	return []map[string]any{devAppNextStep(devAppStep{ID: id, Command: "dws dev app " + path + " --unified-app-id " + quotedAppID + " --format json", DoneWhen: doneWhen})}
}

func devAppScopeSetSuccessNextSteps(params map[string]any, content map[string]any) []map[string]any {
	if !devAppParamBool(params, "precheckOnly") ||
		!strings.EqualFold(devAppFirstContentString(content, "outcome"), "planned") {
		return nil
	}

	step := map[string]any{
		"id":                "apply_scope",
		"title":             "提交可见范围变更",
		"requiresUserInput": true,
		"blocking":          true,
		"doneWhen":          "返回 outcome=success 或 outcome=pending",
	}

	appID := devAppParamString(params, "unifiedAppId")
	missing := make([]string, 0, 2)
	if appID == "" {
		missing = append(missing, "unifiedAppId")
	}
	if !devAppHasScopeValues(params) {
		missing = append(missing, "至少一个可见范围参数")
	}
	if len(missing) > 0 {
		step["missingParams"] = missing
		return []map[string]any{step}
	}

	command, unsafeParams := devAppBuildScopeApplyCommand(params)
	if len(unsafeParams) > 0 {
		step["missingParams"] = unsafeParams
		return []map[string]any{step}
	}
	step["command"] = command
	return []map[string]any{step}
}

func devAppVersionPublishSuccessNextSteps(params map[string]any, content map[string]any) []map[string]any {
	if !devAppContentBool(content, "needsPublish") || devAppContentBool(content, "published") {
		return nil
	}
	appID := devAppFirstContentString(content, "unifiedAppId")
	if appID == "" {
		appID = devAppParamString(params, "unifiedAppId")
	}
	versionID := devAppFirstContentString(content, "versionId")
	if appID == "" || versionID == "" {
		return nil
	}
	quotedAppID, appOK := devAppQuoteScopeValue(appID)
	quotedVersionID, versionOK := devAppQuoteScopeValue(versionID)
	missing := make([]string, 0, 3)
	if !appOK {
		missing = append(missing, "unifiedAppId")
	}
	if !versionOK {
		missing = append(missing, "versionId")
	}
	command := ""
	if len(missing) == 0 {
		command = fmt.Sprintf("dws dev app version publish --unified-app-id %s --version-id %s --confirmed-sensitive=false", quotedAppID, quotedVersionID)
	}
	if strings.EqualFold(devAppContentString(content, "approvalMode"), "SELECT_APPROVER") {
		approverID := devAppFirstContentString(content, "approverUserId", "approver-user-id", "approver_id")
		if approverID == "" {
			missing = append(missing, "approver-user-id")
		} else if quotedApproverID, ok := devAppQuoteScopeValue(approverID); ok {
			command += " --approver-user-id " + quotedApproverID
		} else {
			missing = append(missing, "approver-user-id")
		}
	}
	if len(missing) > 0 {
		return []map[string]any{devAppMissingParamsStep("publish_version", "返回 published=true 或 outcome=pending", missing...)}
	}
	return []map[string]any{devAppNextStep(devAppStep{
		ID:                "publish_version",
		Command:           command + " --format json",
		DoneWhen:          "返回 published=true 或 outcome=pending",
		RequiresUserInput: true,
		Blocking:          true,
	})}
}

func devAppVersionCheckApprovalSuccessNextSteps(content map[string]any) []map[string]any {
	if devAppContentBool(content, "published") || devAppContentBool(content, "approvalSubmitted") {
		return nil
	}
	appID := devAppFirstContentString(content, "unifiedAppId")
	versionID := devAppFirstContentString(content, "versionId")
	if appID == "" || versionID == "" {
		return nil
	}
	quotedAppID, appOK := devAppQuoteScopeValue(appID)
	quotedVersionID, versionOK := devAppQuoteScopeValue(versionID)
	missing := make([]string, 0, 3)
	if !appOK {
		missing = append(missing, "unifiedAppId")
	}
	if !versionOK {
		missing = append(missing, "versionId")
	}
	command := ""
	if len(missing) == 0 {
		command = fmt.Sprintf("dws dev app version publish --unified-app-id %s --version-id %s --confirmed-sensitive=false", quotedAppID, quotedVersionID)
	}
	if strings.EqualFold(devAppContentString(content, "approvalMode"), "select_approver") {
		approverID := devAppFirstContentString(content, "approverUserId", "approver-user-id", "approver_id")
		if approverID == "" {
			missing = append(missing, "approver-user-id")
		} else if quotedApproverID, ok := devAppQuoteScopeValue(approverID); ok {
			command += " --approver-user-id " + quotedApproverID
		} else {
			missing = append(missing, "approver-user-id")
		}
	}
	if devAppContentBool(content, "requiresApproval") && strings.TrimSpace(devAppContentString(content, "approvalMode")) == "" {
		missing = append(missing, "approvalMode", "approver-user-id")
	}
	if len(missing) > 0 {
		return []map[string]any{devAppMissingParamsStep("publish_version", "返回 published=true、approvalSubmitted=true 或版本进入审批状态", missing...)}
	}
	return []map[string]any{devAppNextStep(devAppStep{
		ID:                "publish_version",
		Command:           command + " --format json",
		DoneWhen:          "返回 published=true、approvalSubmitted=true 或版本进入审批状态",
		RequiresUserInput: true,
		Blocking:          true,
	})}
}

func devAppToolMayNeedVersionPublish(tool string) bool {
	switch tool {
	case devAppUpdateTool, devAppScopeSetTool, devAppEventSubscribeTool, devAppEventUnsubscribeTool,
		devAppRobotConfigUpsertTool, devAppRobotEnableTool, devAppRobotOfflineTool, devAppWebappSetTool,
		devAppPermissionAddTool, devAppPermissionRmTool, devAppMemberAddTool, devAppMemberRemoveTool,
		devAppSecurityConfigTool:
		return true
	default:
		return false
	}
}

func devAppVersionCreateSuccessNextSteps(content map[string]any) []map[string]any {
	appID := devAppFirstContentString(content, "unifiedAppId")
	versionID := devAppFirstContentString(content, "versionId")
	if appID == "" || versionID == "" {
		return nil
	}
	quotedAppID, appOK := devAppQuoteScopeValue(appID)
	quotedVersionID, versionOK := devAppQuoteScopeValue(versionID)
	if !appOK || !versionOK {
		missing := make([]string, 0, 2)
		if !appOK {
			missing = append(missing, "unifiedAppId")
		}
		if !versionOK {
			missing = append(missing, "versionId")
		}
		return []map[string]any{devAppMissingParamsStep("check_version_status", "返回版本当前状态", missing...)}
	}
	return []map[string]any{devAppNextStep(devAppStep{
		ID:       "check_version_status",
		Command:  fmt.Sprintf("dws dev app version status --unified-app-id %s --version-id %s --format json", quotedAppID, quotedVersionID),
		DoneWhen: "返回版本当前状态",
	})}
}

func devAppBuildScopeApplyCommand(params map[string]any) (string, []string) {
	quotedAppID, ok := devAppQuoteScopeValue(devAppParamString(params, "unifiedAppId"))
	if !ok {
		return "", []string{"unifiedAppId"}
	}
	parts := []string{"dws", "dev", "app", "scope", "set", "--unified-app-id", quotedAppID}
	unsafeParams := []string(nil)
	for _, item := range []struct{ key, flag string }{
		{"deptVisibleScopes", "--dept-visible-scopes"},
		{"userVisibleScopes", "--user-visible-scopes"},
		{"roleVisibleScopes", "--role-visible-scopes"},
		{"dynamicGroupScopes", "--dynamic-group-scopes"},
	} {
		if value := devAppParamString(params, item.key); value != "" {
			quotedValue, valueOK := devAppQuoteScopeValue(value)
			if !valueOK {
				unsafeParams = append(unsafeParams, item.key)
				continue
			}
			parts = append(parts, item.flag, quotedValue)
		}
	}
	if len(unsafeParams) > 0 {
		return "", unsafeParams
	}
	if hidden, ok := params["isHidden"].(bool); ok {
		parts = append(parts, "--is-hidden="+strconv.FormatBool(hidden))
	}
	parts = append(parts, "--precheck-only=false")
	if publish, ok := params["publish"].(bool); ok {
		parts = append(parts, "--publish="+strconv.FormatBool(publish))
	}
	parts = append(parts, "--format", "json")
	return strings.Join(parts, " "), nil
}

func devAppHasScopeValues(params map[string]any) bool {
	for _, key := range []string{"deptVisibleScopes", "userVisibleScopes", "roleVisibleScopes", "dynamicGroupScopes"} {
		if devAppParamString(params, key) != "" {
			return true
		}
	}
	_, hiddenProvided := params["isHidden"]
	return hiddenProvided
}

func devAppParamString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return devAppScopeListParamValue(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				values = append(values, text)
			}
		}
		return devAppScopeListParamValue(values)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// devAppScopeListParamValue renders scope list values for replay commands.
// Multi-value (or comma-bearing) lists must serialize as a JSON array: a CSV
// join would split a value like "group,a" into two on replay
// (transformDevAppListParam), replaying a different request than precheck.
func devAppScopeListParamValue(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			cleaned = append(cleaned, text)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 && !strings.ContainsAny(cleaned[0], ",[]\"") {
		return cleaned[0]
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(cleaned)
	return strings.TrimSpace(buf.String())
}

func devAppParamBool(params map[string]any, key string) bool {
	value, ok := params[key]
	if !ok {
		return false
	}
	boolean, _ := value.(bool)
	return boolean
}

func appendDevAppSuccessNextSteps(content map[string]any, additions ...map[string]any) {
	if len(additions) == 0 {
		return
	}
	steps := normalizeDevAppNextSteps(content["nextSteps"])
	for _, addition := range additions {
		if addition == nil || devAppNextStepIDExists(steps, devAppContentString(addition, "id")) {
			continue
		}
		steps = append(steps, addition)
	}
	if len(steps) > 0 {
		content["nextSteps"] = steps
	}
}

func normalizeDevAppNextSteps(raw any) []map[string]any {
	steps := make([]map[string]any, 0)
	switch typed := raw.(type) {
	case []map[string]any:
		steps = append(steps, typed...)
	case []any:
		for _, item := range typed {
			if step, ok := item.(map[string]any); ok {
				steps = append(steps, step)
			}
		}
	}
	return steps
}

func devAppNextStepIDExists(steps []map[string]any, id string) bool {
	if id == "" {
		return false
	}
	for _, step := range steps {
		if devAppContentString(step, "id") == id {
			return true
		}
	}
	return false
}

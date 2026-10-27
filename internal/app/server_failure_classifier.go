// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"regexp"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Match an explicit rejection of robotCode itself, not an unrelated invalid
// parameter elsewhere in the error or a phrase stitched across diagnostics.
var dingRobotRejectionPattern = regexp.MustCompile(`(?i)\b(?:robotcode(?:\s*:\s*|\s+(?:is\s+(?:in\s+)?)?)(?:invalid|not\s+valid|not\s+in\s+(?:the\s+)?org(?:anization)?)|invalid\s+robotcode)\b`)

type serverFailureClass struct {
	category         apperrors.Category
	message          string
	reason           string
	origin           string
	stage            string
	hint             string
	actions          []string
	operation        string
	retryable        *bool
	executionStarted *bool
}

func classifyServerFailure(message, serverKey, tool string, diag apperrors.ServerDiagnostics) (serverFailureClass, bool) {
	code := strings.ToUpper(strings.TrimSpace(diag.ServerErrorCode))
	detail := strings.ToLower(strings.TrimSpace(diag.TechnicalDetail))
	text := strings.ToLower(strings.TrimSpace(message))
	combined := text + " " + detail

	// Prefer the explicit server code over incidental dependency or parameter
	// wording. The classifier only improves recovery guidance; it does not
	// invent retryability, a retry delay, or an execution state.
	if code == "RATE_LIMIT_ERROR" {
		return serverFailureClass{
			category: apperrors.CategoryAPI,
			message:  "由于当前请求频率过高，本次请求被限流，请等待 10 秒后重试。",
			reason:   "rate_limited",
			hint:     "请降低请求频率，并等待 10 秒后重试。",
			actions: []string{
				"降低请求频率，避免短时间内发起大量请求",
				"等待 10 秒后使用相同参数重试",
				"持续失败时保留 Trace ID 和 Server Code 联系服务端排查",
			},
		}, true
	}

	if strings.EqualFold(strings.TrimSpace(serverKey), "ding") &&
		(strings.EqualFold(strings.TrimSpace(tool), "send_ding_message") ||
			strings.EqualFold(strings.TrimSpace(tool), "recall_ding_message")) &&
		(dingRobotRejectionPattern.MatchString(text) || dingRobotRejectionPattern.MatchString(detail)) {
		retryable := false
		executionStarted := false
		return serverFailureClass{
			category:         apperrors.CategoryAuth,
			message:          "企业机器人不在当前组织或未处于可用状态",
			reason:           "robot_not_in_org",
			origin:           "dingtalk_api",
			stage:            "precondition_rejected",
			operation:        "ding/" + strings.TrimSpace(tool),
			retryable:        &retryable,
			executionStarted: &executionStarted,
			hint:             "请提供当前组织内有效且已发布的 robot-code 后重新发起请求。",
			actions: []string{
				"停止本次机器人 DING；禁止尝试或替换为其他机器人（包括其他 robot-code）；不要搜索 dev/devapp、配置、其他 profile 或替换发送通道",
			},
		}, true
	}

	if code == "999" &&
		(strings.Contains(combined, "nullpointerexception") || strings.Contains(combined, "system error")) {
		classified := serverFailureClass{
			category: apperrors.CategoryInternal,
			message:  message,
			reason:   "upstream_internal_error",
			origin:   "dingtalk_api",
			stage:    "upstream_execution",
			hint:     "上游服务发生内部异常；请保留 Trace ID 和 Server Code，确认操作结果后再决定是否重试。",
			actions: []string{
				"检查目标资源的当前状态，确认本次操作是否已经生效",
				"状态未确认前不要直接重试写操作",
				"持续失败时携带 Trace ID 和 Server Code 联系服务端排查",
			},
		}
		if strings.EqualFold(strings.TrimSpace(serverKey), "todo") &&
			strings.EqualFold(strings.TrimSpace(tool), "create_personal_todo") {
			retryable := false
			classified.operation = "todo/create_personal_todo"
			classified.retryable = &retryable
			classified.hint = "待办服务发生内部异常，创建结果未知；请先查询是否已创建相同待办，再决定是否重试。"
			classified.actions = []string{
				"查询近期由自己创建的待办，核对标题、执行人和截止时间",
				"确认没有创建成功后再重新提交",
				"持续失败时携带 Trace ID 和 Server Code 联系服务端排查",
			}
		}
		return classified, true
	}

	if code == "NETWORK_ERROR" ||
		strings.Contains(detail, "statuscode.unavailable") ||
		strings.Contains(detail, "connection refused") {
		classified := serverFailureClass{
			category: apperrors.CategoryAPI,
			message:  "MCP 后端依赖暂时不可用",
			reason:   "backend_dependency_unavailable",
			origin:   "mcp_gateway",
			stage:    "backend_dependency",
			hint:     "请求参数无需修改；请使用相同参数稍后重试。持续失败时请提供 Trace ID 排查 MCP 服务。",
			actions: []string{
				"使用相同参数重试一次",
				"持续失败时保留 Trace ID 并排查 MCP 后端依赖",
			},
		}
		if strings.Contains(detail, "querytoolmeta") {
			classified.message = "MCP 后端元数据服务暂时不可用"
			classified.stage = "tool_metadata_lookup"
		}
		return classified, true
	}

	if code == "PARAM_ERROR" ||
		strings.Contains(text, "opencid or cid is required") ||
		strings.Contains(text, "openconversationid") && strings.Contains(text, "required") {
		return serverFailureClass{
			category: apperrors.CategoryAPI,
			message:  message,
			reason:   "invalid_request",
			origin:   "dingtalk_api",
			stage:    "tool_validation",
			hint:     "请求未通过后端参数校验；请核对当前 leaf Help/Schema 和稳定 ID 类型后重试。",
		}, true
	}

	return serverFailureClass{}, false
}

func newServerFailureAPIError(
	message string,
	fallbackReason string,
	fallbackHint string,
	serverKey string,
	tool string,
	diag apperrors.ServerDiagnostics,
) error {
	operation := "tools/call"
	reason := fallbackReason
	origin := ""
	stage := ""
	hint := fallbackHint
	actions := []string{"持续失败时保留 Trace ID 和 Server Code 联系服务端排查"}
	classifiedRetryable := false
	classifiedExecutionStarted := false
	classifiedReason := false
	classifiedOperation := false
	classifiedOrigin := false
	classifiedStage := false
	classifiedHint := false
	classifiedActions := false
	classified, classifiedOK := classifyServerFailure(message, serverKey, tool, diag)
	if classifiedOK {
		message = classified.message
		if classified.reason != "" {
			reason = classified.reason
			classifiedReason = true
		}
		if classified.operation != "" {
			operation = classified.operation
			classifiedOperation = true
		}
		if classified.origin != "" {
			origin = classified.origin
			classifiedOrigin = true
		}
		if classified.stage != "" {
			stage = classified.stage
			classifiedStage = true
		}
		if classified.hint != "" {
			hint = classified.hint
			classifiedHint = true
		}
		if len(classified.actions) > 0 {
			actions = append([]string(nil), classified.actions...)
			classifiedActions = true
		}
		if classified.retryable != nil {
			classifiedRetryable = true
		}
		if classified.executionStarted != nil {
			classifiedExecutionStarted = true
		}
	}
	if diag.Reason != "" && !classifiedReason {
		reason = diag.Reason
	}
	if diag.Operation != "" && !classifiedOperation {
		operation = diag.Operation
	}
	if diag.Origin != "" && !classifiedOrigin {
		origin = diag.Origin
	}
	if diag.FailureStage != "" && !classifiedStage {
		stage = diag.FailureStage
	}
	if diag.FriendlyHint != "" && !classifiedHint {
		hint = diag.FriendlyHint
	}
	if !classifiedActions {
		actions = appendUniqueServerFailureActions(actions, diag.Actions...)
		if diag.NextCommand != "" {
			actions = appendUniqueServerFailureActions(actions, diag.NextCommand)
		}
	}
	opts := []apperrors.Option{
		apperrors.WithOperation(operation),
		apperrors.WithReason(reason),
		apperrors.WithServerKey(serverKey),
		apperrors.WithOrigin(origin),
		apperrors.WithFailureStage(stage),
		apperrors.WithHint(hint),
		apperrors.WithActions(actions...),
		apperrors.WithServerDiag(diag),
	}
	if classifiedOK {
		if classified.retryable != nil {
			opts = append(opts, apperrors.WithRetryable(*classified.retryable))
		}
		if classified.executionStarted != nil {
			opts = append(opts, apperrors.WithExecutionStarted(*classified.executionStarted))
		}
	}
	if diag.ServerRetryable != nil && !classifiedRetryable {
		opts = append(opts, apperrors.WithRetryable(*diag.ServerRetryable))
	}
	if diag.ExecutionStarted != nil && !classifiedExecutionStarted {
		opts = append(opts, apperrors.WithExecutionStarted(*diag.ExecutionStarted))
	}
	if diag.NextCommand != "" || diag.Outcome != "" || diag.ContractVersion != "" || diag.Unknown != nil || len(diag.Details) > 0 {
		details := map[string]any{}
		for key, value := range diag.Details {
			details[key] = value
		}
		if diag.NextCommand != "" {
			details["nextCommand"] = diag.NextCommand
		}
		if diag.Outcome != "" {
			details["outcome"] = diag.Outcome
		}
		if diag.ContractVersion != "" {
			details["contractVersion"] = diag.ContractVersion
		}
		if diag.Unknown != nil {
			details["unknown"] = *diag.Unknown
		}
		opts = append(opts, apperrors.WithDetails(details))
	}
	category := apperrors.CategoryAPI
	if classifiedOK && classified.category != "" {
		category = classified.category
	} else {
		switch strings.ToLower(strings.TrimSpace(diag.ErrorType)) {
		case "auth", "authorization", "permission", "forbidden":
			category = apperrors.CategoryAuth
		case "validation":
			category = apperrors.CategoryValidation
		case "discovery":
			category = apperrors.CategoryDiscovery
		case "internal":
			category = apperrors.CategoryInternal
		}
	}
	newError := apperrors.NewAPI
	switch category {
	case apperrors.CategoryAuth:
		newError = apperrors.NewAuth
	case apperrors.CategoryValidation:
		newError = apperrors.NewValidation
	case apperrors.CategoryDiscovery:
		newError = apperrors.NewDiscovery
	case apperrors.CategoryInternal:
		newError = apperrors.NewInternal
	}
	return newError(message, opts...)
}

func appendUniqueServerFailureActions(actions []string, additions ...string) []string {
	for _, action := range additions {
		if action == "" || containsServerFailureAction(actions, action) {
			continue
		}
		actions = append(actions, action)
	}
	return actions
}

func containsServerFailureAction(actions []string, want string) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}

func serverFailureReason(err error, fallback string) string {
	typed, ok := err.(*apperrors.Error)
	if ok && strings.TrimSpace(typed.Reason) != "" {
		return typed.Reason
	}
	return fallback
}

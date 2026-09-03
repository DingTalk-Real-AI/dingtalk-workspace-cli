// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package responsecheck contains fail-closed response validators shared by
// built-in shortcuts. It deliberately does not guess that a missing collection
// means an empty result: callers must name the exact reviewed wire paths.
package responsecheck

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Error returns a non-retryable response-validation error. The remote request
// has completed, but the response is not sufficient evidence of success.
func Error(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

// RequireSuccess validates the common service envelope and returns it. A
// missing success field is rejected because the products using this helper
// have been observed to publish an explicit boolean success field.
func RequireSuccess(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, Error(operation, "empty_tool_response", "服务返回空响应，无法证明调用成功或结果确实为空")
	}
	envelope := data
	if content, ok := data["content"].(map[string]any); ok {
		envelope = content
	}
	raw, present := envelope["success"]
	if !present {
		return nil, Error(operation, "missing_success", "响应缺少 success 业务状态，无法证明调用成功")
	}
	success, ok := raw.(bool)
	if !ok {
		return nil, Error(operation, "malformed_success", "响应 success 字段不是布尔值")
	}
	if !success {
		message := firstString(envelope, "errorMessage", "errorMsg", "message", "error")
		if message == "" {
			message = "服务明确返回 success=false"
		}
		return nil, Error(operation, "remote_failure", message)
	}
	return envelope, nil
}

// RequireResult returns the explicit non-null result value from a successful
// response. Empty arrays and empty objects remain valid values; absence/null do
// not prove an empty business result.
func RequireResult(data map[string]any, operation string) (any, error) {
	envelope, err := RequireSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	result, present := envelope["result"]
	if !present || result == nil {
		return nil, Error(operation, "missing_result", "成功响应缺少非空 result 字段")
	}
	return result, nil
}

// RequireObjectResult returns an explicit object result.
func RequireObjectResult(data map[string]any, operation string) (map[string]any, error) {
	result, err := RequireResult(data, operation)
	if err != nil {
		return nil, err
	}
	object, ok := result.(map[string]any)
	if !ok {
		return nil, Error(operation, "malformed_result", fmt.Sprintf("响应 result 应为对象，实际为 %T", result))
	}
	return object, nil
}

// RequireSingleObjectResult returns one non-empty business object. It accepts
// the two reviewed detail shapes used by product services: result={...} and
// result=[{...}]. Empty arrays, multiple rows, empty objects, and scalar values
// cannot prove that the requested unique resource was read.
func RequireSingleObjectResult(data map[string]any, operation string) (map[string]any, error) {
	result, err := RequireResult(data, operation)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	switch typed := result.(type) {
	case map[string]any:
		object = typed
	case []any:
		if len(typed) != 1 {
			return nil, Error(operation, "unexpected_detail_count", fmt.Sprintf("详情响应应唯一，实际返回 %d 项", len(typed)))
		}
		var ok bool
		object, ok = typed[0].(map[string]any)
		if !ok {
			return nil, Error(operation, "malformed_result", fmt.Sprintf("详情响应 result[0] 应为对象，实际为 %T", typed[0]))
		}
	default:
		return nil, Error(operation, "malformed_result", fmt.Sprintf("详情响应 result 应为对象或单元素对象数组，实际为 %T", result))
	}
	if len(object) == 0 {
		return nil, Error(operation, "empty_result_object", "成功响应的详情对象为空，无法证明资源读取成功")
	}
	return object, nil
}

// RequireObjectCollection resolves the first present reviewed path, requires
// an array at that exact path, and rejects malformed elements instead of
// silently dropping them. A present empty array is a valid empty result.
func RequireObjectCollection(data map[string]any, operation string, paths ...string) ([]map[string]any, error) {
	envelope, err := RequireSuccess(data, operation)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		value, present := lookup(envelope, path)
		if !present {
			continue
		}
		raw, ok := value.([]any)
		if !ok {
			return nil, Error(operation, "malformed_collection", fmt.Sprintf("响应 %s 应为数组，实际为 %T", path, value))
		}
		items := make([]map[string]any, 0, len(raw))
		for index, item := range raw {
			object, ok := item.(map[string]any)
			if !ok || len(object) == 0 {
				return nil, Error(operation, "malformed_item", fmt.Sprintf("响应 %s[%d] 不是非空对象", path, index))
			}
			items = append(items, object)
		}
		return items, nil
	}
	return nil, Error(operation, "missing_collection", "成功响应缺少已审核的结果数组；不能把未知响应结构当作空结果")
}

// LookupObject returns the object at a reviewed dotted path.
func LookupObject(data map[string]any, operation, path string) (map[string]any, error) {
	value, present := lookup(data, path)
	if !present {
		return nil, Error(operation, "missing_object", fmt.Sprintf("响应缺少 %s 对象", path))
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, Error(operation, "malformed_object", fmt.Sprintf("响应 %s 应为对象，实际为 %T", path, value))
	}
	return object, nil
}

func lookup(data map[string]any, path string) (any, bool) {
	var current any = data
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

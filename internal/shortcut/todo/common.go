// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package todo

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const (
	todoPageSize = 20
	todoMaxPages = 40
)

func todoObjectResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"additionalProperties":true}`,
			description,
		)),
	}
}

func todoCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"结果数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"待办结果条目","additionalProperties":true}}},"required":["count",%q],"additionalProperties":true}`,
			description, collection, description, collection,
		)),
	}
}

func todoPagedCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"结果数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"待办结果条目","additionalProperties":true}},"page":{"type":"integer","description":"当前页码；全量模式省略"},"size":{"type":"integer","description":"当前页大小；全量模式省略"},"hasMore":{"type":"boolean","description":"服务端是否仍有下一页；全量模式省略"},"complete":{"type":"boolean","description":"全量模式是否确认遍历耗尽"}},"required":["count",%q],"additionalProperties":false}`,
			description, collection, description, collection,
		)),
	}
}

func requireTodoResponse(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, todoResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明请求成功")
	}
	if raw, ok := data["success"]; ok {
		success, valid := raw.(bool)
		if !valid {
			return nil, todoResponseError(operation, "malformed_success", "响应 success 字段不是布尔值")
		}
		if !success {
			return nil, todoResponseError(operation, "remote_failure", "服务明确返回 success=false")
		}
	}
	return data, nil
}

func requireTodoWriteReceipt(data map[string]any, operation string) error {
	if len(data) == 0 {
		return todoWriteResponseError(operation, "empty_tool_response", "写响应为空，远端效果未知")
	}
	raw, ok := data["success"]
	if !ok {
		return todoWriteResponseError(operation, "missing_success_receipt", "写响应没有明确的 success=true 回执")
	}
	success, valid := raw.(bool)
	if !valid {
		return todoWriteResponseError(operation, "malformed_success", "写响应 success 字段不是布尔值")
	}
	if !success {
		return todoWriteResponseError(operation, "remote_failure", "服务明确返回 success=false")
	}
	return nil
}

func requireTodoCollection(data map[string]any, operation string, keys ...string) ([]map[string]any, error) {
	data, err := requireTodoResponse(data, operation)
	if err != nil {
		return nil, err
	}
	raw, found, err := findTodoCollection(data, keys, 0)
	if err != nil {
		return nil, todoResponseError(operation, "malformed_collection", err.Error())
	}
	if !found {
		return nil, todoResponseError(operation, "missing_collection", "响应缺少预期列表容器")
	}
	out := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, todoResponseError(operation, "malformed_item", fmt.Sprintf("列表第 %d 项不是对象", i))
		}
		out = append(out, m)
	}
	return out, nil
}

func todoHasMore(data map[string]any) (bool, error) {
	return findTodoHasMore(data, 0)
}

func findTodoHasMore(m map[string]any, depth int) (bool, error) {
	if depth > 4 {
		return false, todoResponseError("todo/pagination", "missing_has_more", "响应缺少 hasMore，无法证明分页已完成")
	}
	if raw, ok := m["hasMore"]; ok {
		value, valid := raw.(bool)
		if !valid {
			return false, todoResponseError("todo/pagination", "malformed_has_more", "响应 hasMore 字段不是布尔值")
		}
		return value, nil
	}
	for _, key := range []string{"result", "data"} {
		if raw, ok := m[key]; ok {
			child, valid := raw.(map[string]any)
			if !valid {
				return false, todoResponseError("todo/pagination", "malformed_wrapper", key+" 包装层不是对象")
			}
			return findTodoHasMore(child, depth+1)
		}
	}
	return false, todoResponseError("todo/pagination", "missing_has_more", "响应缺少 hasMore，无法证明分页已完成")
}

func listAllTodoCards(rt *shortcut.RuntimeContext, base map[string]any, maxPages int) ([]map[string]any, error) {
	all := make([]map[string]any, 0)
	for page := 1; page <= maxPages; page++ {
		params := make(map[string]any, len(base)+2)
		for key, value := range base {
			params[key] = value
		}
		params["pageNum"] = strconv.Itoa(page)
		params["pageSize"] = strconv.Itoa(todoPageSize)
		data, err := rt.CallMCPReadData("todo", "get_user_todos_in_current_org", params)
		if err != nil {
			return nil, err
		}
		pageItems, err := getMyTasksProjectStrict(data)
		if err != nil {
			return nil, err
		}
		hasMore, err := todoHasMore(data)
		if err != nil {
			return nil, err
		}
		all = append(all, pageItems...)
		if !hasMore {
			return all, nil
		}
	}
	return nil, todoResponseError("todo/get_user_todos_in_current_org", "pagination_limit_reached", fmt.Sprintf("连续 %d 页仍返回 hasMore=true，拒绝把不完整结果当作完整结果", maxPages))
}

func findTodoCollection(m map[string]any, keys []string, depth int) ([]any, bool, error) {
	if depth > 4 {
		return nil, false, nil
	}
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			arr, valid := raw.([]any)
			if !valid {
				return nil, false, fmt.Errorf("%s 容器不是数组", key)
			}
			return arr, true, nil
		}
	}
	for _, key := range []string{"result", "data"} {
		raw, ok := m[key]
		if !ok {
			continue
		}
		if arr, valid := raw.([]any); valid {
			return arr, true, nil
		}
		child, valid := raw.(map[string]any)
		if !valid {
			return nil, false, fmt.Errorf("%s 包装层既不是对象也不是数组", key)
		}
		if arr, found, err := findTodoCollection(child, keys, depth+1); found || err != nil {
			return arr, found, err
		}
	}
	return nil, false, nil
}

func requireTodoDetail(data map[string]any, operation, expectedTaskID string) (map[string]any, error) {
	data, err := requireTodoResponse(data, operation)
	if err != nil {
		return nil, err
	}
	detail, found, err := findTodoObject(data, []string{"todoDetailModel", "todo", "task"}, 0)
	if err != nil {
		return nil, todoResponseError(operation, "malformed_detail", err.Error())
	}
	if !found {
		// Some deployments return the detail itself under result/data.
		if result, ok := data["result"].(map[string]any); ok && todoStableString(result, "taskId", "todoId", "id") != "" {
			detail, found = result, true
		} else if todoStableString(data, "taskId", "todoId", "id") != "" {
			detail, found = data, true
		}
	}
	if !found || len(detail) == 0 {
		return nil, todoResponseError(operation, "missing_detail", "响应缺少有效待办详情对象")
	}
	actual := todoStableString(detail, "taskId", "todoId", "id")
	if actual == "" {
		return nil, todoResponseError(operation, "missing_stable_id", "待办详情缺少稳定 taskId")
	}
	if expectedTaskID != "" && actual != expectedTaskID {
		return nil, todoResponseError(operation, "identity_mismatch", "读回的 taskId 与请求不一致")
	}
	return detail, nil
}

func findTodoObject(m map[string]any, keys []string, depth int) (map[string]any, bool, error) {
	if depth > 4 {
		return nil, false, nil
	}
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			object, valid := raw.(map[string]any)
			if !valid {
				return nil, false, fmt.Errorf("%s 容器不是对象", key)
			}
			return object, true, nil
		}
	}
	for _, key := range []string{"result", "data"} {
		raw, ok := m[key]
		if !ok {
			continue
		}
		child, valid := raw.(map[string]any)
		if !valid {
			return nil, false, fmt.Errorf("%s 包装层不是对象", key)
		}
		if object, found, err := findTodoObject(child, keys, depth+1); found || err != nil {
			return object, found, err
		}
	}
	return nil, false, nil
}

func todoCreatedTaskID(data map[string]any) string {
	return findTodoStableString(data, []string{"taskId", "todoId", "id"}, 0)
}

func findTodoStableString(m map[string]any, keys []string, depth int) string {
	if depth > 5 {
		return ""
	}
	if value := todoStableString(m, keys...); value != "" {
		return value
	}
	for _, key := range []string{"result", "data", "todoDetailModel", "todo", "task"} {
		if child, ok := m[key].(map[string]any); ok {
			if value := findTodoStableString(child, keys, depth+1); value != "" {
				return value
			}
		}
	}
	return ""
}

func todoStableString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := m[key].(type) {
		case string:
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		case json.Number:
			return value.String()
		case float64:
			if value == float64(int64(value)) {
				return strconv.FormatInt(int64(value), 10)
			}
		case int:
			return strconv.Itoa(value)
		case int64:
			return strconv.FormatInt(value, 10)
		}
	}
	return ""
}

func readTodoDetail(rt *shortcut.RuntimeContext, taskID string) (map[string]any, error) {
	data, err := rt.CallMCPReadData("todo", "get_todo_detail", map[string]any{"taskId": taskID})
	if err != nil {
		return nil, err
	}
	return requireTodoDetail(data, "todo/get_todo_detail", taskID)
}

// VerifyCreatedTodo is shared by Todo-owned smart shortcuts that create a
// personal todo. It requires a terminal receipt, a stable taskId and a detail
// read-back bound to that exact ID.
func VerifyCreatedTodo(rt *shortcut.RuntimeContext, data map[string]any, operation, expectedSubject string) (string, map[string]any, error) {
	if err := requireTodoWriteReceipt(data, operation); err != nil {
		return "", nil, err
	}
	taskID := todoCreatedTaskID(data)
	if taskID == "" {
		return "", nil, todoWriteResponseError(operation, "missing_stable_id", "创建响应缺少稳定 taskId；远端效果未知")
	}
	detail, err := readTodoDetail(rt, taskID)
	if err != nil {
		return "", nil, todoWriteVerificationError(operation, err)
	}
	if subject, _ := detail["subject"].(string); expectedSubject != "" && subject != expectedSubject {
		return "", nil, todoWriteResponseError(operation, "verification_mismatch", "创建后读回的标题不一致")
	}
	return taskID, detail, nil
}

// VerifyDoneStatus requires a terminal write receipt and verifies isDone on a
// detail read bound to the same taskId.
func VerifyDoneStatus(rt *shortcut.RuntimeContext, data map[string]any, taskID string, expected bool) error {
	if err := requireTodoWriteReceipt(data, "todo/update_todo_done_status"); err != nil {
		return err
	}
	detail, err := readTodoDetail(rt, taskID)
	if err != nil {
		return todoWriteVerificationError("todo/update_todo_done_status", err)
	}
	actual, ok := detail["isDone"].(bool)
	if !ok || actual != expected {
		return todoWriteResponseError("todo/update_todo_done_status", "verification_mismatch", "写后读回 isDone 不一致或缺失")
	}
	return nil
}

func todoResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

// todoWriteResponseError marks failures found only after a write request was
// submitted. Callers must reconcile the returned resource or receipt before
// retrying, especially for non-idempotent create/comment operations.
func todoWriteResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

// todoWriteVerificationError upgrades a read-back failure after a confirmed
// write receipt. It preserves the original reason and cause while making the
// unsafe retry boundary explicit to callers.
func todoWriteVerificationError(operation string, cause error) error {
	reason := "write_verification_failed"
	origin := "mcp"
	var typed *apperrors.Error
	if stderrors.As(cause, &typed) {
		if strings.TrimSpace(typed.Reason) != "" {
			reason = typed.Reason
		}
		if strings.TrimSpace(typed.Origin) != "" {
			origin = typed.Origin
		}
	}
	options := []apperrors.Option{
		apperrors.WithOperation(operation),
		apperrors.WithOrigin(origin),
		apperrors.WithFailureStage("write_verification"),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
		apperrors.WithCause(cause),
	}
	message := "写操作已提交，但读回核验失败；请先查询对账，禁止直接重试"
	if typed != nil && typed.Category == apperrors.CategoryAuth {
		return apperrors.NewAuth(message, options...)
	}
	return apperrors.NewAPI(message, options...)
}

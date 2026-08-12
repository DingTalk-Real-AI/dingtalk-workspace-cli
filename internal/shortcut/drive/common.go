// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package drive

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

const driveCompositeInterfaceReason = "Reviewed Drive Shortcut composite: the executable CLI owns strict response validation, optional multi-step orchestration, local I/O, read-back verification, output projection, and confirmation; no single MCP interface represents the complete command contract."

func driveContract(command, description, useWhen string, avoidWhen, examples []string, result *contract.ResultSpec, pagination *contract.PaginationSpec, params ...contract.ParamDecl) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "drive " + command
	return corecmd.ContractDecl{
		Description: description,
		Parameters:  params,
		Result:      result,
		Pagination:  pagination,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       driveCompositeInterfaceReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{useWhen},
			AvoidWhen:    avoidWhen,
			Examples:     examples,
		},
		Identity: contract.ToolIdentitySpec{
			ProductID:      "drive",
			Name:           name,
			CanonicalPath:  "drive." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
	}
}

func driveObjectResult(description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"additionalProperties":true}`,
			description,
		)),
	}
}

func driveCollectionResult(collection, description string) *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema: json.RawMessage(fmt.Sprintf(
			`{"type":"object","description":%q,"properties":{"count":{"type":"integer","description":"本页有效结果数量"},%q:{"type":"array","description":%q,"items":{"type":"object","description":"Drive 资源条目","additionalProperties":true}},"nextCursor":{"type":"string","description":"下一页游标"},"hasMore":{"type":"boolean","description":"服务端是否仍有下一页"}},"required":["count",%q],"additionalProperties":true}`,
			description, collection, description, collection,
		)),
	}
}

func driveCursorPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{
		Kind:                  contract.PaginationKindCursor,
		CursorParameter:       "cursor",
		MetaPath:              contract.PaginationMetaPath,
		EndpointExhaustedPath: contract.PaginationExhaustedPath,
		NextTokenPath:         contract.PaginationNextTokenPath,
	}
}

func requireDriveResponse(data map[string]any, operation string) (map[string]any, error) {
	if len(data) == 0 {
		return nil, driveResponseError(operation, "empty_tool_response", "服务返回空响应，无法证明操作成功")
	}
	if success, present := data["success"]; present {
		value, ok := success.(bool)
		if !ok {
			return nil, driveResponseError(operation, "malformed_success", "响应 success 字段不是布尔值")
		}
		if !value {
			message := firstString(data, "errorMsg", "message", "error")
			if message == "" {
				message = "服务明确返回 success=false"
			}
			return nil, driveResponseError(operation, "remote_failure", message)
		}
	}
	return data, nil
}

func requireDriveObject(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireDriveResponse(data, operation)
	if err != nil {
		return nil, err
	}
	if value, present := data["result"]; present {
		result, ok := value.(map[string]any)
		if !ok || len(result) == 0 {
			return nil, driveResponseError(operation, "malformed_result", "响应 result 缺失有效对象")
		}
		return result, nil
	}
	if value, present := data["data"]; present {
		result, ok := value.(map[string]any)
		if !ok || len(result) == 0 {
			return nil, driveResponseError(operation, "malformed_data", "响应 data 缺失有效对象")
		}
		return result, nil
	}
	if len(data) == 1 {
		return nil, driveResponseError(operation, "missing_business_result", "响应没有可验证的业务对象")
	}
	return data, nil
}

func requireDriveWrite(data map[string]any, operation string) (map[string]any, error) {
	data, err := requireDriveResponse(data, operation)
	if err != nil {
		return nil, err
	}
	success, ok := data["success"].(bool)
	if !ok || !success {
		return nil, driveResponseError(operation, "missing_terminal_success", "写操作响应没有 success=true 终态证据")
	}
	return data, nil
}

func requireDriveCollection(data map[string]any, operation string, keys ...string) ([]any, map[string]any, error) {
	data, err := requireDriveResponse(data, operation)
	if err != nil {
		return nil, nil, err
	}
	containers := []map[string]any{data}
	for _, wrapper := range []string{"result", "data"} {
		if value, present := data[wrapper]; present {
			inner, ok := value.(map[string]any)
			if !ok {
				return nil, nil, driveResponseError(operation, "malformed_envelope", fmt.Sprintf("响应 %s 字段不是对象", wrapper))
			}
			containers = append(containers, inner)
		}
	}
	for _, container := range containers {
		for _, key := range keys {
			value, present := container[key]
			if !present {
				continue
			}
			items, ok := value.([]any)
			if !ok {
				return nil, nil, driveResponseError(operation, "malformed_collection", fmt.Sprintf("响应 %s 字段不是数组", key))
			}
			for index, item := range items {
				if _, ok := item.(map[string]any); !ok {
					return nil, nil, driveResponseError(operation, "malformed_collection_item", fmt.Sprintf("响应 %s[%d] 不是对象", key, index))
				}
			}
			return items, container, nil
		}
	}
	return nil, nil, driveResponseError(operation, "missing_collection", "响应缺少声明的业务数组；不能把缺字段投影成空结果")
}

func projectDriveRows(items []any, aliases map[string][]string) []map[string]any {
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		source := item.(map[string]any)
		row := make(map[string]any)
		for canonical, candidates := range aliases {
			for _, candidate := range candidates {
				if value, ok := source[candidate]; ok && value != nil {
					row[canonical] = value
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func addDrivePagination(out map[string]any, container map[string]any) {
	for _, pair := range [][2]string{{"nextCursor", "nextCursor"}, {"nextToken", "nextCursor"}, {"nextPageToken", "nextCursor"}, {"hasMore", "hasMore"}} {
		if value, ok := container[pair[0]]; ok && value != nil {
			out[pair[1]] = value
		}
	}
}

func firstString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nestedString(data map[string]any, keys ...string) string {
	if value := firstString(data, keys...); value != "" {
		return value
	}
	for _, wrapper := range []string{"result", "data"} {
		if inner, ok := data[wrapper].(map[string]any); ok {
			if value := firstString(inner, keys...); value != "" {
				return value
			}
		}
	}
	return ""
}

func driveResponseError(operation, reason, message string) error {
	return apperrors.NewAPI(message,
		apperrors.WithOperation(operation),
		apperrors.WithOrigin("mcp"),
		apperrors.WithFailureStage("response_validation"),
		apperrors.WithRetryable(false),
		apperrors.WithReason(reason),
	)
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

func newStandaloneWhiteboardCreateCommand() *cobra.Command {
	createExampleIndex := 0
	return NewLeafCommand(LeafSpec{
		Use:           "create-with-content",
		Short:         "使用 OpenNodes 创建独立白板",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        whiteboardServerID,
		Tool:          standaloneWhiteboardCreateTool,
		Long: `使用 OpenNodes V1 初始内容创建独立 .adraw 白板。

	--source 接受 OpenNodes V1 JSON String，或在内容较长时接受本地 JSON 文件路径。
	内联 JSON 可直接使用 {"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[...]}；
	文件可使用该结构，也可使用 {"source":{...}} 包装结构。
	source.nodes 必须是数组，允许 []，表示创建空白独立白板。
	CLI 校验后统一向 MCP/HSF 传递 source JSON 字符串。
	--request-id 是稳定幂等键，同一次逻辑创建的网络重试必须复用相同值。`,
		Example: `  dws whiteboard create-with-content --name "项目方案白板" --source ./whiteboard.json --request-id wb-create-001 --format json
	  dws whiteboard create-with-content --name "项目方案白板" --source ./whiteboard.json --folder FOLDER_ID --request-id wb-create-002 --format json`,
		Flags: []LeafFlag{
			{Name: "name", Usage: "独立白板名称（必填）", Bind: "name", Required: true, MarkRequired: true, Trim: true},
			{Name: "source", Usage: "OpenNodes V1 JSON String 或 JSON 文件路径（必填）", Bind: "source", Required: true, MarkRequired: true, Trim: true, Transform: loadStandaloneWhiteboardCreateSource},
			{Name: "folder", Usage: "目标文件夹节点 ID/URL", Bind: "folderId", Trim: true, OmitEmpty: true},
			{Name: "workspace", Usage: "目标知识库 ID/URL", Bind: "workspaceId", Trim: true, OmitEmpty: true},
			{Name: "request-id", Usage: "1-128 字符稳定幂等请求 ID（必填）", Bind: "requestId", Required: true, MarkRequired: true, Trim: true, Transform: validateStandaloneWhiteboardRequestID},
		},
		Constraints: []LeafConstraint{{Kind: LeafMutuallyExclusive, Flags: []string{"folder", "workspace"}}},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "whiteboard", Name: "create_with_content",
				CanonicalPath: "whiteboard.create_with_content",
				CLIPath:       "whiteboard create-with-content", PrimaryCLIPath: "whiteboard create-with-content",
			},
			Description: "使用 OpenNodes V1 初始内容和稳定 requestId 创建独立白板",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode: "composite", Availability: "available",
				Reason: "CLI 在调用 create_whiteboard 前解析并校验内联或文件中的 OpenNodes，dry-run 只输出安全摘要，并校验幂等创建结果",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "使用调用方提供的 OpenNodes V1 初始内容创建独立 .adraw 白板",
				UseWhen:      []string{"需要在文件夹、知识库或我的文档中创建一份带 OpenNodes 初始内容的独立白板时"},
				AvoidWhen:    []string{"创建空白独立白板使用现有文档文件创建能力；在文档中插入白板卡片使用 doc whiteboard insert"},
				Examples:     []string{"dws whiteboard create-with-content --name \"项目方案白板\" --source ./whiteboard.json --request-id wb-create-001 --format json"},
				ExampleDispositions: []contract.ExampleDisposition{{
					Index:      &createExampleIndex,
					Mode:       contract.ExampleDispositionModeContractOnly,
					ReasonCode: contract.ExampleDispositionReasonLocalState,
					Reason:     "运行时需要用户提供可读且通过 OpenNodes V1 校验的本地 JSON 文件",
					Reviewed:   true,
				}},
			},
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "name", Required: boolPtr(true)},
				{Name: "source", Property: "source", InterfaceType: "string", Required: boolPtr(true)},
				{Name: "folder", Property: "folderId", Required: boolPtr(false)},
				{Name: "workspace", Property: "workspaceId", Required: boolPtr(false)},
				{Name: "request-id", Property: "requestId", Required: boolPtr(true)},
			},
			Result: standaloneWhiteboardCreateResultSpec(),
		},
		Validate: func(cmd *cobra.Command, _ []string) error {
			return rejectWhiteboardOutputFilters(cmd)
		},
		ResultCall: callStandaloneWhiteboardCreateResult,
	})
}

func loadStandaloneWhiteboardCreateSource(sourceValue string) (any, error) {
	sourceValue = strings.TrimSpace(sourceValue)
	if sourceValue == "" {
		return nil, invalidWhiteboardSourceParam("source is required")
	}

	var data []byte
	if strings.HasPrefix(sourceValue, "{") {
		data = []byte(sourceValue)
	} else {
		var err error
		data, err = os.ReadFile(sourceValue)
		if err != nil {
			code := CodeInvalidPath
			if os.IsNotExist(err) {
				code = CodeFileNotFound
			}
			return nil, &CLIError{
				Code:       code,
				Message:    fmt.Sprintf("--source 既不是 OpenNodes JSON，也无法读取为文件 %q", sourceValue),
				Suggestion: "直接传 OpenNodes JSON String，或确认文件路径指向可读的 UTF-8 JSON 文件",
				Cause:      err,
			}
		}
	}

	input, nodesJSON, err := parseStandaloneWhiteboardCreateJSON(data)
	if err != nil {
		return nil, err
	}
	// parseStandaloneWhiteboardCreateJSON has already validated both fixed
	// version fields and the complete nodes array. Assemble the service string
	// from those validated fragments so JSON numbers remain numbers and there is
	// no second lossy decode/encode pass.
	return fmt.Sprintf(`{"schemaVersion":%s,"catalogVersion":%s,"nodes":%s}`,
		strconv.Quote(input.Source.SchemaVersion),
		strconv.Quote(input.Source.CatalogVersion),
		nodesJSON), nil
}

func parseStandaloneWhiteboardCreateJSON(data []byte) (*whiteboardUpdateFile, string, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&fields); err != nil {
		return nil, "", invalidWhiteboardSourceJSON(err)
	}
	if err := ensureWhiteboardJSONEOF(decoder); err != nil {
		return nil, "", invalidWhiteboardSourceJSON(err)
	}
	if fields == nil {
		return nil, "", invalidWhiteboardSourceParam("source must be a JSON object")
	}
	if _, wrapped := fields["source"]; wrapped {
		return parseWhiteboardSourceJSON(data)
	}
	wrapper := make([]byte, 0, len(data)+len(`{"source":}`))
	wrapper = append(wrapper, `{"source":`...)
	wrapper = append(wrapper, data...)
	wrapper = append(wrapper, '}')
	return parseWhiteboardSourceJSON(wrapper)
}

func validateStandaloneWhiteboardRequestID(requestID string) (any, error) {
	requestID = strings.TrimSpace(requestID)
	if err := whiteboardcore.ValidateCreateRequestID(requestID); err != nil {
		return nil, err
	}
	return requestID, nil
}

func callStandaloneWhiteboardCreateResult(cmd *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
	if deps.Caller.DryRun() {
		sourceJSON, _ := args["source"].(string)
		var source struct {
			Nodes []json.RawMessage `json:"nodes"`
		}
		if err := json.Unmarshal([]byte(sourceJSON), &source); err != nil {
			return nil, invalidWhiteboardSourceJSON(err)
		}
		preview := map[string]any{
			"name": args["name"], "requestId": args["requestId"],
			"sourceBytes": len(sourceJSON), "nodeCount": len(source.Nodes),
			"executed": false, "dryRun": true,
		}
		for _, key := range []string{"folderId", "workspaceId"} {
			if value, ok := args[key]; ok {
				preview[key] = value
			}
		}
		return output.Success(preview, output.WithDryRun()), nil
	}
	response, err := callWhiteboardToolResult(cmd, standaloneWhiteboardCreateTool, args)
	if err != nil {
		return nil, err
	}
	if err := validateStandaloneWhiteboardCreateResponse(response, whiteboardString(args["requestId"])); err != nil {
		return nil, err
	}
	return output.Success(unwrapWhiteboardResult(response)), nil
}

func validateStandaloneWhiteboardCreateResponse(response map[string]any, expectedRequestID string) error {
	result := unwrapWhiteboardResult(response)
	if result == nil {
		return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf("response must be a JSON object"))
	}
	if success, exists := response["success"]; exists {
		value, ok := success.(bool)
		if !ok || !value {
			return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf("success must be true"))
		}
	}
	if strings.TrimSpace(whiteboardString(result["nodeId"])) == "" {
		return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf("response missing nodeId"))
	}
	requestID := strings.TrimSpace(whiteboardString(result["requestId"]))
	if requestID == "" {
		return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf("response missing requestId"))
	}
	if requestID != expectedRequestID {
		return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf(
			"response requestId %q does not match request %q", requestID, expectedRequestID))
	}
	if err := normalizeStandaloneWhiteboardCreateRevision(result); err != nil {
		return invalidStandaloneWhiteboardCreateReceipt(err)
	}
	// The pre-release gateway currently projects only the stable create identity
	// and links even though the reviewed HSF output contains these additional
	// verification fields. Treat omitted projection fields as unknown, but keep
	// rejecting an explicit contradiction so a future richer response cannot
	// silently claim the wrong resource or content outcome.
	if raw, exists := result["contentType"]; exists {
		contentType, ok := raw.(string)
		if !ok || strings.ToUpper(strings.TrimSpace(contentType)) != "WBD" {
			return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf("unexpected contentType %q", whiteboardString(raw)))
		}
	}
	for _, field := range []string{"requestedContentApplied", "requestMatched"} {
		if raw, exists := result[field]; exists {
			value, ok := raw.(bool)
			if !ok || !value {
				return invalidStandaloneWhiteboardCreateReceipt(fmt.Errorf("%s must be true when present", field))
			}
		}
	}
	return nil
}

func normalizeStandaloneWhiteboardCreateRevision(result map[string]any) error {
	raw, exists := result["revision"]
	if !exists {
		return fmt.Errorf("response missing revision")
	}
	var text string
	switch value := raw.(type) {
	case json.Number:
		text = value.String()
	case string:
		text = strings.TrimSpace(value)
	default:
		return fmt.Errorf("revision must be a non-negative integer, got %T", raw)
	}
	revision, err := strconv.ParseInt(text, 10, 64)
	if err != nil || revision < 0 {
		return fmt.Errorf("revision must be a non-negative integer, got %q", text)
	}
	// Publish one stable Result shape even while the gateway serializes the
	// numeric HSF revision as a JSON string.
	result["revision"] = json.Number(strconv.FormatInt(revision, 10))
	return nil
}

func invalidStandaloneWhiteboardCreateReceipt(err error) error {
	return &CLIError{
		Code:       CodeMCPToolError,
		Message:    "白板已返回创建响应，但成功回执字段不符合约定",
		Suggestion: "不要更换 request-id 盲目重试；保留原 request-id、响应和 trace 信息排查",
		Operation:  whiteboardServerID + "/" + standaloneWhiteboardCreateTool,
		Cause:      err,
	}
}

func unwrapWhiteboardResult(response map[string]any) map[string]any {
	if response == nil {
		return nil
	}
	if result, ok := response["result"].(map[string]any); ok {
		return result
	}
	return response
}

func whiteboardString(value any) string {
	text, _ := value.(string)
	return text
}

func standaloneWhiteboardCreateResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"独立白板幂等创建结果",
			"properties":{
				"nodeId":{"type":"string","description":"新建独立白板节点 ID"},
				"folderId":{"type":"string","description":"实际父目录 ID"},
				"name":{"type":"string","description":"白板名称"},
				"docUrl":{"type":"string","description":"桌面端访问链接"},
				"mobileUrl":{"type":"string","description":"移动端访问链接"},
				"contentType":{"type":"string","description":"网关投影该字段时固定为 WBD"},
				"extension":{"type":"string","description":"白板扩展名，通常为 adraw"},
				"status":{"type":"string","description":"创建状态"},
				"initializationMode":{"type":"string","description":"初始化方式"},
				"revision":{"type":"integer","description":"初始白板 revision"},
				"requestId":{"type":"string","description":"服务端回显的请求幂等 ID"},
				"requestedContentApplied":{"type":"boolean","description":"网关投影该字段时表示请求 OpenNodes 初始内容是否已应用"},
				"idempotentReplay":{"type":"boolean","description":"是否命中幂等重放"},
				"requestMatched":{"type":"boolean","description":"网关投影该字段时表示历史幂等请求是否与本次参数一致"},
				"message":{"type":"string","description":"服务端结果说明"}
			},
			"required":["requestId","nodeId","revision"],
			"additionalProperties":true
		}`),
		SensitivePaths: []string{"nodeId", "folderId"},
	}
}

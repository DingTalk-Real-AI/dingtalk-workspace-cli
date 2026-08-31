// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

const (
	whiteboardServerID   = "whiteboard"
	whiteboardQueryTool  = "read_whiteboard_content"
	whiteboardUpdateTool = "update_whiteboard"
)

type whiteboardUpdateFile struct {
	Overwrite bool                  `json:"overwrite"`
	Source    *whiteboardOpenSource `json:"source"`
}

type whiteboardOpenSource struct {
	SchemaVersion  string          `json:"schemaVersion"`
	CatalogVersion string          `json:"catalogVersion"`
	Nodes          json.RawMessage `json:"nodes"`
}

var compactWhiteboardJSON = json.Compact

func newWhiteboardCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "whiteboard",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("白板深度指南", "dingtalk-misc", "references/whiteboard.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "读取和更新钉钉在线文档中的内嵌白板",
			UseWhen:      []string{"操作已有文档内嵌白板的 OpenNodes 内容时"},
			AvoidWhen:    []string{"普通文档正文和块使用 doc；创建白板卡片先用 doc whiteboard insert"},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "whiteboard",
		Short: "钉钉文档内嵌白板管理",
		Long: `读取或更新钉钉在线文档中已经存在的内嵌白板。

当前仅支持单页白板。每次操作都必须同时提供文档 ID 或 URL 和白板 part ID；
本命令不负责创建白板（请使用 dws doc whiteboard insert），也不支持通过已有节点 ID 做局部修改。`,
		RunE: groupRunE,
	})

	queryCmd := &cobra.Command{
		Use:     "query",
		Short:   "读取白板内容",
		Example: `  dws whiteboard query --node DOC_ID_OR_URL --part-id WHITEBOARD_PART_ID --format json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := rejectWhiteboardOutputFilters(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "node", "part-id"); err != nil {
				return err
			}
			return callWhiteboardTool(cmd, whiteboardQueryTool, map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
				"partId": mustGetFlag(cmd, "part-id"),
			})
		},
	}
	queryCmd.Flags().String("node", "", "承载白板的钉钉文档 ID 或 URL（必填）")
	queryCmd.Flags().String("part-id", "", "文档内白板 part ID（必填）")
	DeclareLeafMetadata(queryCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "whiteboard",
				Name:           "query",
				CanonicalPath:  "whiteboard.query",
				CLIPath:        "whiteboard query",
				PrimaryCLIPath: "whiteboard query",
			},
			Description: "读取钉钉文档内已有白板的 OpenNodes 内容",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "白板端点通过显式服务适配器调用并解码 resultJson，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "读取钉钉文档内已有白板的 OpenNodes 内容",
				UseWhen:      []string{"已知承载文档 nodeId 和白板 partId，需要检查当前白板节点、布局或写入支持时"},
				AvoidWhen:    []string{"创建新白板卡片用 doc whiteboard insert；缺少 partId 时先从文档 card metadata.id 定位"},
				Examples:     []string{"dws whiteboard query --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "part-id", Property: "partId", Required: boolPtr(true)},
			},
		},
	})

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "追加或整页重建白板内容",
		Long: `从 JSON 文件读取 OpenNodes V1 更新请求并更新已有白板。

更新模式由文件顶层的 overwrite 字段决定。overwrite=false 表示追加，
overwrite=true 表示整页重建。两种模式都会写入远端白板，必须同时传入 --yes。`,
		Example: `  dws whiteboard update --node DOC_ID_OR_URL --part-id WHITEBOARD_PART_ID --source ./whiteboard.json --format json
  dws whiteboard update --node DOC_ID_OR_URL --part-id WHITEBOARD_PART_ID --source ./overwrite.json --yes --format json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := rejectWhiteboardOutputFilters(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "node", "part-id", "source"); err != nil {
				return err
			}

			input, nodesJSON, err := loadWhiteboardUpdateFile(mustGetFlag(cmd, "source"))
			if err != nil {
				return err
			}
			mode := "append"
			if input.Overwrite {
				mode = "overwrite"
			}
			return callWhiteboardTool(cmd, whiteboardUpdateTool, map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
				"partId": mustGetFlag(cmd, "part-id"),
				"mode":   mode,
				"nodes":  nodesJSON,
			})
		},
	}
	updateCmd.Flags().String("node", "", "承载白板的钉钉文档 ID 或 URL（必填）")
	updateCmd.Flags().String("part-id", "", "文档内白板 part ID（必填）")
	updateCmd.Flags().String("source", "", "OpenNodes V1 更新请求 JSON 文件（必填）")
	updateCmd.Flags().Bool("yes", false, "确认写入远端白板")
	updateExampleIndex := 0
	DeclareLeafMetadata(updateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "whiteboard",
				Name:           "update",
				CanonicalPath:  "whiteboard.update",
				CLIPath:        "whiteboard update",
				PrimaryCLIPath: "whiteboard update",
			},
			Description: "经用户确认后向已有白板追加 OpenNodes 或整页重建",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令包含本地 OpenNodes 校验、显式白板服务路由与结构化结果解码，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "经用户确认后向已有白板追加 OpenNodes 或整页重建",
				UseWhen:      []string{"已有 nodeId、partId 和合规 OpenNodes V1 文件，用户确认后要追加图形、文本、连接线或整页替换时"},
				AvoidWhen:    []string{"只读取内容用 whiteboard query；创建白板卡片用 doc whiteboard insert；不要用真实节点 ID 做局部修改"},
				Examples:     []string{"dws whiteboard update --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --source ./whiteboard.json --format json"},
				ExampleDispositions: []contract.ExampleDisposition{{
					Index:      &updateExampleIndex,
					Mode:       contract.ExampleDispositionModeContractOnly,
					ReasonCode: contract.ExampleDispositionReasonLocalState,
					Reason:     "运行时需要用户提供可读且通过 OpenNodes V1 校验的本地 JSON 文件",
					Reviewed:   true,
				}},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "part-id", Property: "partId", Required: boolPtr(true)},
				{Name: "source", Required: boolPtr(true)},
			},
		},
	})

	root.AddCommand(queryCmd, updateCmd)
	return root
}

func rejectWhiteboardOutputFilters(cmd *cobra.Command) error {
	for _, name := range []string{"jq", "fields"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			flag = cmd.InheritedFlags().Lookup(name)
		}
		if flag != nil && flag.Changed {
			return &CLIError{
				Code:       CodeInvalidParam,
				Message:    fmt.Sprintf("whiteboard 命令不支持 --%s", name),
				Suggestion: "直接读取命令返回的结构化 JSON",
			}
		}
	}
	return nil
}

func loadWhiteboardUpdateFile(path string) (*whiteboardUpdateFile, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		code := CodeInvalidPath
		if os.IsNotExist(err) {
			code = CodeFileNotFound
		}
		return nil, "", &CLIError{
			Code:       code,
			Message:    fmt.Sprintf("无法读取白板更新文件 %q", path),
			Suggestion: "确认 --source 指向可读的 UTF-8 JSON 文件",
			Cause:      err,
		}
	}

	var input whiteboardUpdateFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, "", invalidWhiteboardSourceJSON(err)
	}
	if err := ensureWhiteboardJSONEOF(decoder); err != nil {
		return nil, "", invalidWhiteboardSourceJSON(err)
	}
	if input.Source == nil {
		return nil, "", invalidWhiteboardSourceParam("source is required")
	}
	if input.Source.SchemaVersion != "1.0" {
		return nil, "", invalidWhiteboardSourceParam(`source.schemaVersion must be "1.0"`)
	}
	if input.Source.CatalogVersion != "dml-v1" {
		return nil, "", invalidWhiteboardSourceParam(`source.catalogVersion must be "dml-v1"`)
	}

	nodesJSON, nodeCount, err := validateWhiteboardNodes(input.Source.Nodes)
	if err != nil {
		return nil, "", err
	}
	if !input.Overwrite && nodeCount == 0 {
		return nil, "", invalidWhiteboardSourceParam("append requires at least one source.nodes item")
	}
	return &input, nodesJSON, nil
}

func ensureWhiteboardJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("multiple JSON values are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func validateWhiteboardNodes(raw json.RawMessage) (string, int, error) {
	if len(raw) == 0 || !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return "", 0, invalidWhiteboardSourceParam("source.nodes must be an array")
	}

	var nodes []json.RawMessage
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return "", 0, invalidWhiteboardSourceJSON(err)
	}
	for i, node := range nodes {
		var object map[string]any
		if err := json.Unmarshal(node, &object); err != nil || object == nil {
			return "", 0, invalidWhiteboardSourceParam(fmt.Sprintf("source.nodes[%d] must be an object", i))
		}
	}

	var compact bytes.Buffer
	if err := compactWhiteboardJSON(&compact, raw); err != nil {
		return "", 0, invalidWhiteboardSourceJSON(err)
	}
	return compact.String(), len(nodes), nil
}

func invalidWhiteboardSourceJSON(err error) error {
	return &CLIError{
		Code:       CodeInvalidJSON,
		Message:    "白板更新文件不是合法的 OpenNodes V1 JSON",
		Suggestion: "检查 JSON 语法、未知字段以及 source 对象结构",
		Cause:      err,
	}
}

func invalidWhiteboardSourceParam(message string) error {
	return &CLIError{
		Code:       CodeInvalidParam,
		Message:    message,
		Suggestion: "参考 whiteboard Skill 中的 OpenNodes V1 文件格式",
	}
}

func callWhiteboardTool(cmd *cobra.Command, toolName string, args map[string]any) error {
	if deps.Caller.DryRun() {
		return callMCPToolOnServer(whiteboardServerID, toolName, args)
	}

	text, err := callMCPToolReturnTextOnServer(cmd.Context(), whiteboardServerID, toolName, args)
	if err != nil {
		return err
	}
	if text == "" {
		return nil
	}

	var response map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return invalidWhiteboardToolResult(toolName, err)
	}
	if err := ensureWhiteboardJSONEOF(decoder); err != nil {
		return invalidWhiteboardToolResult(toolName, err)
	}
	if response == nil {
		return invalidWhiteboardToolResult(toolName, fmt.Errorf("response must be a JSON object"))
	}

	if encoded, ok := response["resultJson"].(string); ok && strings.TrimSpace(encoded) != "" {
		var result any
		resultDecoder := json.NewDecoder(strings.NewReader(encoded))
		resultDecoder.UseNumber()
		if err := resultDecoder.Decode(&result); err != nil {
			return invalidWhiteboardToolResult(toolName, fmt.Errorf("invalid resultJson: %w", err))
		}
		if err := ensureWhiteboardJSONEOF(resultDecoder); err != nil {
			return invalidWhiteboardToolResult(toolName, fmt.Errorf("invalid resultJson: %w", err))
		}
		response["resultJson"] = result
	}
	return deps.Out.PrintJSON(response)
}

func invalidWhiteboardToolResult(toolName string, err error) error {
	return &CLIError{
		Code:       CodeMCPToolError,
		Message:    "白板服务返回了无法解析的 JSON",
		Suggestion: "使用 --debug 获取调用信息并联系白板服务维护者",
		Operation:  whiteboardServerID + "/" + toolName,
		Cause:      err,
	}
}

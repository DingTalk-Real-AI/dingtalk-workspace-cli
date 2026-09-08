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
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

const (
	whiteboardServerID = whiteboardcore.ServerID

	whiteboardQueryTool  = whiteboardcore.EmbeddedQueryTool
	whiteboardUpdateTool = whiteboardcore.EmbeddedUpdateTool

	standaloneWhiteboardCreateTool = whiteboardcore.StandaloneCreateTool
	standaloneWhiteboardQueryTool  = whiteboardcore.StandaloneQueryTool
	standaloneWhiteboardUpdateTool = whiteboardcore.StandaloneUpdateTool
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
			AgentSummary: "创建独立白板，或按 partId 是否提供查询和更新独立/文档内嵌白板",
			UseWhen:      []string{"用户要读取或写入白板/画布中的 OpenNodes，或使用 OpenNodes 初始内容创建独立白板时；没有文档内嵌证据时默认独立白板"},
			AvoidWhen:    []string{"普通文档正文和块使用 doc；只创建或删除文档内白板卡片使用 doc whiteboard insert / doc block delete"},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "whiteboard",
		Short: "钉钉白板管理",
		Long: `创建独立白板，或读取和更新独立/文档内嵌白板。

	显式提供非空 --part-id 时操作文档内嵌白板；完全未提供 --part-id 时默认操作
	独立 .adraw 白板。接口失败后不会自动切换另一类白板。文档内插入白板卡片请使用
	dws doc whiteboard insert。`,
		RunE: groupRunE,
	})

	queryCmd := &cobra.Command{
		Use:   "query",
		Short: "读取白板内容",
		Long: `读取白板内容。显式提供非空 --part-id 时读取文档内嵌白板；未提供时默认读取独立白板。

	独立白板支持 summary、all 和 page 三种视图，默认 summary；view=page 时必须提供 --page-id。`,
		Example: `  dws whiteboard query --node DOC_ID_OR_URL --part-id WHITEBOARD_PART_ID --format json
  dws whiteboard query --node WHITEBOARD_NODE_ID --view all --format json
  dws whiteboard query --node WHITEBOARD_NODE_ID --view page --page-id PAGE_ID --format json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := rejectWhiteboardOutputFilters(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "node"); err != nil {
				return err
			}
			call, err := whiteboardcore.BuildQueryCall(whiteboardcore.QueryOptions{
				Target: whiteboardcore.Target{
					NodeID: mustGetFlag(cmd, "node"), PartID: mustGetFlag(cmd, "part-id"),
					PartIDChanged: cmd.Flags().Changed("part-id"),
				},
				View: mustGetFlag(cmd, "view"), ViewChanged: cmd.Flags().Changed("view"),
				PageID: mustGetFlag(cmd, "page-id"), PageIDChanged: cmd.Flags().Changed("page-id"),
			})
			if err != nil {
				return err
			}
			return callWhiteboardTool(cmd, call.Tool, call.Args)
		},
	}
	queryCmd.Flags().String("node", "", "承载文档或独立白板的节点 ID/URL（必填）")
	queryCmd.Flags().String("part-id", "", "文档内白板 part ID；显式提供时选择内嵌白板")
	queryCmd.Flags().String("view", "summary", "独立白板查询视图: summary / page / all")
	queryCmd.Flags().String("page-id", "", "独立白板页面 ID（view=page 时必填）")
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
			Description: "按 partId 是否提供读取文档内嵌或独立白板内容",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "白板端点通过显式服务适配器调用并解码 resultJson，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "显式 partId 读取内嵌白板；未提供 partId 时默认读取独立白板",
				UseWhen:      []string{"读取独立 .adraw 白板，或已知承载文档 nodeId 与白板 partId 时"},
				AvoidWhen:    []string{"创建文档内白板卡片用 doc whiteboard insert；内嵌白板缺少 partId 时先从 card metadata.id 定位"},
				Examples: []string{
					"dws whiteboard query --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --format json",
					"dws whiteboard query --node <WHITEBOARD_NODE_ID> --view all --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "part-id", Property: "partId", Required: boolPtr(false)},
				{Name: "view", Property: "view", Required: boolPtr(false), Enum: []string{"summary", "page", "all"}},
				{Name: "page-id", Property: "pageId", Required: boolPtr(false), RequiredWhen: "独立白板且 view=page 时"},
			},
		},
	})

	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "追加或整页重建白板内容",
		Long: `从 JSON 文件读取 OpenNodes V1 更新请求并更新已有白板。

	更新模式由文件顶层的 overwrite 字段决定。overwrite=false 表示追加，
	overwrite=true 表示整页重建。显式提供非空 --part-id 时更新文档内嵌白板；未提供时
	默认更新独立白板，并要求 --expected-revision 和 --request-id。两种模式都会写入
	远端白板，执行前必须取得用户确认；非交互执行仅在确认后添加 --yes。`,
		Example: `  dws whiteboard update --node DOC_ID_OR_URL --part-id WHITEBOARD_PART_ID --source ./whiteboard.json --format json
  dws whiteboard update --node WHITEBOARD_NODE_ID --source ./whiteboard.json --expected-revision 12 --request-id wb-update-001 --format json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := rejectWhiteboardOutputFilters(cmd); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "node", "source"); err != nil {
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
			expectedRevision, _ := cmd.Flags().GetInt("expected-revision")
			call, err := whiteboardcore.BuildUpdateCall(whiteboardcore.UpdateOptions{
				Target: whiteboardcore.Target{
					NodeID: mustGetFlag(cmd, "node"), PartID: mustGetFlag(cmd, "part-id"),
					PartIDChanged: cmd.Flags().Changed("part-id"),
				},
				PageID: mustGetFlag(cmd, "page-id"), PageIDChanged: cmd.Flags().Changed("page-id"),
				ExpectedRevision: expectedRevision, ExpectedRevisionChanged: cmd.Flags().Changed("expected-revision"),
				RequestID: mustGetFlag(cmd, "request-id"), RequestIDChanged: cmd.Flags().Changed("request-id"),
				Mode: mode, NodesJSON: nodesJSON,
			})
			if err != nil {
				return err
			}
			return callWhiteboardTool(cmd, call.Tool, call.Args)
		},
	}
	updateCmd.Flags().String("node", "", "承载文档或独立白板的节点 ID/URL（必填）")
	updateCmd.Flags().String("part-id", "", "文档内白板 part ID；显式提供时选择内嵌白板")
	updateCmd.Flags().String("source", "", "OpenNodes V1 更新请求 JSON 文件（必填）")
	updateCmd.Flags().String("page-id", "", "目标页面 ID；独立白板 overwrite 时必填")
	updateCmd.Flags().Int("expected-revision", 0, "独立白板最新 revision（独立分支必填）")
	updateCmd.Flags().String("request-id", "", "独立白板稳定幂等请求 ID（独立分支必填）")
	updateCmd.Flags().Bool("yes", false, "确认写入远端白板")
	updateEmbeddedExampleIndex, updateStandaloneExampleIndex := 0, 1
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
				UseWhen:      []string{"已有合规 OpenNodes V1 文件，用户确认后要更新独立白板，或已知 nodeId+partId 要更新文档内嵌白板时"},
				AvoidWhen:    []string{"只读取内容用 whiteboard query；创建文档内白板卡片用 doc whiteboard insert；不要用真实节点 ID 做局部修改"},
				Examples: []string{
					"dws whiteboard update --node <DOC_ID> --part-id <WHITEBOARD_PART_ID> --source ./whiteboard.json --format json",
					"dws whiteboard update --node <WHITEBOARD_NODE_ID> --source ./whiteboard.json --expected-revision 12 --request-id wb-update-001 --format json",
				},
				ExampleDispositions: []contract.ExampleDisposition{
					{
						Index:      &updateEmbeddedExampleIndex,
						Mode:       contract.ExampleDispositionModeContractOnly,
						ReasonCode: contract.ExampleDispositionReasonLocalState,
						Reason:     "运行时需要用户提供可读且通过 OpenNodes V1 校验的本地 JSON 文件",
						Reviewed:   true,
					},
					{
						Index:      &updateStandaloneExampleIndex,
						Mode:       contract.ExampleDispositionModeContractOnly,
						ReasonCode: contract.ExampleDispositionReasonLocalState,
						Reason:     "运行时需要用户提供可读且通过 OpenNodes V1 校验的本地 JSON 文件",
						Reviewed:   true,
					},
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "part-id", Property: "partId", Required: boolPtr(false)},
				{Name: "source", Required: boolPtr(true)},
				{Name: "page-id", Property: "pageId", Required: boolPtr(false), RequiredWhen: "独立白板 overwrite 时"},
				{Name: "expected-revision", Property: "expectedRevision", Required: boolPtr(false), RequiredWhen: "操作独立白板时"},
				{Name: "request-id", Property: "requestId", Required: boolPtr(false), RequiredWhen: "操作独立白板时"},
			},
		},
	})

	root.AddCommand(queryCmd, updateCmd, newStandaloneWhiteboardCreateCommand())
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
	input, nodesJSON, err := loadWhiteboardSourceFile(path)
	if err != nil {
		return nil, "", err
	}
	if !input.Overwrite && nodesJSON == "[]" {
		return nil, "", invalidWhiteboardSourceParam("append requires at least one source.nodes item")
	}
	return input, nodesJSON, nil
}

func loadWhiteboardSourceFile(path string) (*whiteboardUpdateFile, string, error) {
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

	return parseWhiteboardSourceJSON(data)
}

func parseWhiteboardSourceJSON(data []byte) (*whiteboardUpdateFile, string, error) {
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

	nodesJSON, _, err := validateWhiteboardNodes(input.Source.Nodes)
	if err != nil {
		return nil, "", err
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
	response, err := callWhiteboardToolResult(cmd, toolName, args)
	if err != nil {
		return err
	}
	if response == nil {
		return nil
	}
	return deps.Out.PrintJSON(response)
}

func callWhiteboardToolResult(cmd *cobra.Command, toolName string, args map[string]any) (map[string]any, error) {
	text, err := callMCPToolReturnTextOnServer(cmd.Context(), whiteboardServerID, toolName, args)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}

	var response map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, invalidWhiteboardToolResult(toolName, err)
	}
	if err := ensureWhiteboardJSONEOF(decoder); err != nil {
		return nil, invalidWhiteboardToolResult(toolName, err)
	}
	if response == nil {
		return nil, invalidWhiteboardToolResult(toolName, fmt.Errorf("response must be a JSON object"))
	}

	if raw, present := response["resultJson"]; present && raw != nil {
		result := raw
		if encoded, ok := raw.(string); ok {
			if strings.TrimSpace(encoded) == "" {
				return nil, invalidWhiteboardToolResult(toolName, fmt.Errorf("resultJson must not be blank"))
			}
			resultDecoder := json.NewDecoder(strings.NewReader(encoded))
			resultDecoder.UseNumber()
			if err := resultDecoder.Decode(&result); err != nil {
				return nil, invalidWhiteboardToolResult(toolName, fmt.Errorf("invalid resultJson: %w", err))
			}
			if err := ensureWhiteboardJSONEOF(resultDecoder); err != nil {
				return nil, invalidWhiteboardToolResult(toolName, fmt.Errorf("invalid resultJson: %w", err))
			}
		}
		if toolName == standaloneWhiteboardQueryTool {
			object, ok := result.(map[string]any)
			if !ok || len(object) == 0 {
				return nil, invalidWhiteboardToolResult(toolName, fmt.Errorf("resultJson must be a non-empty JSON object"))
			}
			// Standalone query returns the OpenNodes snapshot as an encoded
			// resultJson. Promote the decoded value to the public source field so
			// page/all callers can consume source.pages without knowing the HSF
			// transport representation. Keep the decoded resultJson alias for
			// compatibility with callers that already consume the native output.
			response["resultJson"] = object
			response["source"] = object
		} else {
			response["resultJson"] = result
		}
	}
	return response, nil
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

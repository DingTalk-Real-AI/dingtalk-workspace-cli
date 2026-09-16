// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
	whiteboardrender "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/render"
)

const maximumWhiteboardRenderSourceBytes = 16 * 1024 * 1024

var whiteboardRenderStdin io.Reader = os.Stdin

// Keep path resolution independently testable across supported operating systems.
var whiteboardRenderAbsPath = filepath.Abs

func newWhiteboardRenderCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "render",
		Short: "把 OpenNodes 预渲染为本地 SVG",
		Long: `把 OpenNodes V1 本地预渲染为确定性 SVG，供 Agent 在创建白板前展示给用户确认。

	生成预览后必须向用户展示 SVG 并停止执行，等待用户明确确认当前版本才能创建。
	用户要求修改时，修改 source、重新渲染并再次等待确认；最初的创建请求和 Agent 自检均不替代此确认。
	预览不会访问网络，也不会创建或修改远端白板。--source 支持内联 JSON、@文件、
	裸文件路径或 -（stdin）。图片、Vector、未知节点和无法安全解释的内容会显示为占位框，
	并在 warnings 中明确返回。预览用于确认内容、结构和大致布局，不承诺与钉钉白板像素级一致。`,
		Example:       "  dws whiteboard render --source @whiteboard.json --output ./preview.svg --format json",
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "source", Usage: "OpenNodes V1 JSON、@文件、文件路径或 -（必填）", Bind: "source", Required: true, MarkRequired: true, Trim: true, Transform: loadWhiteboardRenderSource},
			{Name: "output", Usage: "输出 SVG 文件路径（必填）", Bind: "output", Required: true, MarkRequired: true, Trim: true},
			{Name: "force", Usage: "允许原子替换已存在的输出文件", Kind: LeafBool, Bind: "force"},
		},
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID: "whiteboard", Name: "render", CanonicalPath: "whiteboard.render",
				CLIPath: "whiteboard render", PrimaryCLIPath: "whiteboard render",
			},
			Description: "在本地把 OpenNodes V1 预渲染为安全的确定性 SVG",
			Interface: &contract.InterfaceSpec{
				Mode: contract.InterfaceModeComposite, Availability: contract.InterfaceAvailable,
				Reason: "纯本地 OpenNodes SVG renderer，不调用 MCP、不访问外链资源",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建白板前预渲染 SVG；展示后必须等待用户确认当前版本，修改后重新渲染并再次确认；sourceDigest 只校验内容一致性",
				UseWhen:      []string{"Agent 已生成 OpenNodes，需要在 create-with-content 前向用户展示内容、结构和大致布局时"},
				AvoidWhen:    []string{"比较已有白板与 proposed 更新使用 whiteboard +diff；读取真实白板使用 whiteboard +query；预览不代表最终像素效果"},
				Examples:     []string{"dws whiteboard render --source @whiteboard.json --output ./preview.svg --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "source", Property: "source", InterfaceType: "string", Required: boolPtr(true)},
				{Name: "output", Property: "artifactPath", InterfaceType: "string", Required: boolPtr(true)},
				{Name: "force", Property: "force", InterfaceType: "boolean", Required: boolPtr(false)},
			},
			Result: whiteboardRenderResultSpec(),
		},
		ResultCall: callWhiteboardRenderResult,
	})
}

func loadWhiteboardRenderSource(raw string) (any, error) {
	data, err := readWhiteboardRenderSource(raw)
	if err != nil {
		return nil, err
	}
	source, err := opennodes.Parse(data)
	if err != nil {
		return nil, invalidWhiteboardRenderSource(err)
	}
	return source, nil
}

func readWhiteboardRenderSource(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, invalidWhiteboardRenderSource(fmt.Errorf("source is required"))
	}
	if strings.HasPrefix(raw, "{") {
		if len(raw) > maximumWhiteboardRenderSourceBytes {
			return nil, invalidWhiteboardRenderSource(fmt.Errorf("source exceeds %d bytes", maximumWhiteboardRenderSourceBytes))
		}
		return []byte(raw), nil
	}
	if raw == "-" {
		data, err := io.ReadAll(io.LimitReader(whiteboardRenderStdin, maximumWhiteboardRenderSourceBytes+1))
		if err != nil {
			return nil, invalidWhiteboardRenderSource(fmt.Errorf("read stdin: %w", err))
		}
		if len(data) > maximumWhiteboardRenderSourceBytes {
			return nil, invalidWhiteboardRenderSource(fmt.Errorf("stdin source exceeds %d bytes", maximumWhiteboardRenderSourceBytes))
		}
		return data, nil
	}
	path := strings.TrimPrefix(raw, "@")
	data, err := os.ReadFile(path)
	if err != nil {
		code := CodeInvalidPath
		if os.IsNotExist(err) {
			code = CodeFileNotFound
		}
		return nil, &CLIError{
			Code: code, Message: fmt.Sprintf("无法读取 OpenNodes 预渲染源文件 %q", path),
			Suggestion: "传入内联 JSON、@可读文件、裸文件路径或 - 从 stdin 读取", Cause: err,
		}
	}
	if len(data) > maximumWhiteboardRenderSourceBytes {
		return nil, invalidWhiteboardRenderSource(fmt.Errorf("source file exceeds %d bytes", maximumWhiteboardRenderSourceBytes))
	}
	return data, nil
}

func callWhiteboardRenderResult(_ *cobra.Command, _ string, args map[string]any) (output.CommandResult, error) {
	source, ok := args["source"].(*opennodes.Source)
	if !ok || source == nil {
		return nil, invalidWhiteboardRenderSource(fmt.Errorf("normalized source is unavailable"))
	}
	rendered, err := whiteboardrender.SVG(source)
	if err != nil {
		return nil, &CLIError{
			Code: CodeInvalidParam, Message: "OpenNodes SVG 预渲染失败",
			Suggestion: "检查 warnings 所指节点，或缩小 source.nodes 后重试", Cause: err,
		}
	}
	digest, err := opennodes.DigestSource(source)
	if err != nil {
		return nil, &CLIError{Code: CodeUnclassified, Message: "计算白板 sourceDigest 失败", Cause: err}
	}

	requestedPath := strings.TrimSpace(whiteboardString(args["output"]))
	if requestedPath == "" {
		return nil, &CLIError{Code: CodeInvalidPath, Message: "--output 不能为空"}
	}
	if !strings.EqualFold(filepath.Ext(requestedPath), ".svg") {
		return nil, &CLIError{
			Code: CodeInvalidPath, Message: "--output 必须使用 .svg 扩展名",
			Suggestion: "例如 --output ./preview.svg",
		}
	}
	absolutePath, err := whiteboardRenderAbsPath(requestedPath)
	if err != nil {
		return nil, &CLIError{Code: CodeInvalidPath, Message: "无法解析 --output 绝对路径", Cause: err}
	}
	force, _ := args["force"].(bool)
	if _, err := os.Lstat(absolutePath); err == nil && !force {
		return nil, &CLIError{
			Code: CodeInvalidPath, Message: fmt.Sprintf("输出文件已存在：%s", absolutePath),
			Suggestion: "更换 --output，或明确添加 --force 原子替换现有文件",
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, &CLIError{Code: CodeInvalidPath, Message: "无法检查输出文件", Cause: err}
	}
	if err := AtomicWrite(absolutePath, rendered.SVG, 0o644); err != nil {
		return nil, &CLIError{Code: CodeInvalidPath, Message: "无法原子写入 SVG 预览文件", Cause: err}
	}

	result := map[string]any{
		"artifactPath":            absolutePath,
		"mimeType":                "image/svg+xml",
		"sourceDigest":            digest,
		"rendererVersion":         whiteboardrender.Version,
		"fidelity":                rendered.Fidelity,
		"nodeCount":               rendered.NodeCount,
		"renderedCount":           rendered.RenderedCount,
		"exactCount":              rendered.ExactCount,
		"approximateCount":        rendered.ApproximateCount,
		"placeholderCount":        rendered.PlaceholderCount,
		"svgBytes":                len(rendered.SVG),
		"bounds":                  rendered.Bounds,
		"warnings":                rendered.Warnings,
		"warningCodes":            whiteboardrender.SortedWarningCodes(rendered.Warnings),
		"remoteReads":             false,
		"nextAction":              "await_user_confirmation",
		"confirmationInstruction": "向用户展示此 SVG、fidelity 和全部 warnings，然后停止执行并等待用户明确确认当前版本；用户要求修改时重新渲染并再次确认。最初的创建请求、Agent 自检通过和 sourceDigest 匹配均不代表用户确认。确认后才能使用同一 source 和 sourceDigest 创建，不能自行添加 --yes。",
		"disclaimer":              "预览用于确认内容、结构和大致布局；实际白板的字体、换行、图形细节及连接线路径可能略有差异。",
	}
	return output.Success(result), nil
}

func invalidWhiteboardRenderSource(err error) error {
	var textError *opennodes.TextRunValidationError
	if errors.As(err, &textError) {
		return &CLIError{Code: CodeInvalidJSON, Message: textError.Error(), Cause: err}
	}
	return &CLIError{
		Code: CodeInvalidJSON, Message: "--source 不是可预渲染的 OpenNodes V1",
		Suggestion: "使用 schemaVersion=1.0、catalogVersion=dml-v1 且 nodes 为对象数组的 source", Cause: err,
	}
}

func whiteboardRenderResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{
		Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
		DataSchema: json.RawMessage(`{
			"type":"object",
			"description":"本地 OpenNodes V1 SVG 预渲染产物与创建绑定摘要",
			"properties":{
				"artifactPath":{"type":"string","description":"已写入的 SVG 绝对路径"},
				"mimeType":{"type":"string","const":"image/svg+xml","description":"预览产物 MIME 类型"},
				"sourceDigest":{"type":"string","description":"规范化 source 的 SHA-256 摘要，可传给 create-with-content --expected-source-digest"},
				"rendererVersion":{"type":"string","description":"本地 renderer 语义版本"},
				"fidelity":{"type":"string","enum":["exact","approximate","placeholder"],"description":"整个预览的最低保真等级"},
				"nodeCount":{"type":"integer","minimum":0,"description":"输入 OpenNodes 节点总数"},
				"renderedCount":{"type":"integer","minimum":0,"description":"实际输出到 SVG 的非隐藏节点数"},
				"exactCount":{"type":"integer","minimum":0,"description":"按确定映射渲染的节点数"},
				"approximateCount":{"type":"integer","minimum":0,"description":"按近似规则渲染的节点数"},
				"placeholderCount":{"type":"integer","minimum":0,"description":"使用占位框渲染的节点数"},
				"svgBytes":{"type":"integer","minimum":0,"description":"SVG 文件字节数"},
				"bounds":{"type":"object","description":"含默认留白的 SVG viewBox","properties":{"x":{"type":"number","description":"viewBox 左上角 x"},"y":{"type":"number","description":"viewBox 左上角 y"},"width":{"type":"number","exclusiveMinimum":0,"description":"viewBox 宽度"},"height":{"type":"number","exclusiveMinimum":0,"description":"viewBox 高度"}},"required":["x","y","width","height"],"additionalProperties":false},
				"warnings":{"type":"array","description":"近似或占位渲染的逐节点说明","items":{"type":"object","properties":{"code":{"type":"string","description":"稳定 warning 代码"},"nodeId":{"type":"string","description":"关联节点 ID"},"nodeType":{"type":"string","description":"关联节点类型"},"message":{"type":"string","description":"可读说明"},"fidelity":{"type":"string","description":"该 warning 对应的保真等级"}},"required":["code","message","fidelity"],"additionalProperties":false}},
				"warningCodes":{"type":"array","description":"排序去重后的 warning 代码摘要","items":{"type":"string"}},
				"remoteReads":{"type":"boolean","const":false,"description":"确认预览没有网络读取"},
				"nextAction":{"type":"string","const":"await_user_confirmation","description":"渲染后的下一步：展示 SVG 并等待用户确认，不能自动创建"},
				"confirmationInstruction":{"type":"string","description":"用户审阅当前 SVG、修改后重新确认及创建授权的操作约束"},
				"disclaimer":{"type":"string","description":"预览与最终白板可能存在差异的用户提示"}
			},
			"required":["artifactPath","mimeType","sourceDigest","rendererVersion","fidelity","nodeCount","renderedCount","exactCount","approximateCount","placeholderCount","svgBytes","bounds","warnings","warningCodes","remoteReads","disclaimer","nextAction","confirmationInstruction"],
			"additionalProperties":false
		}`),
		SensitivePaths: []string{"artifactPath"},
	}
}

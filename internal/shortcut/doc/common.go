// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

const compositeInterfaceReason = "Reviewed Doc Shortcut composite: the executable CLI owns validation, multi-step orchestration, local I/O, output projection, and confirmation; no single MCP interface represents the complete command contract."

func docContract(command, description, intent string, examples []string, params ...contract.ParamDecl) corecmd.ContractDecl {
	name := "shortcut_" + strings.ReplaceAll(strings.TrimPrefix(command, "+"), "-", "_")
	cliPath := "doc " + command
	return corecmd.ContractDecl{
		Description: description,
		Parameters:  params,
		Interface: &contract.InterfaceSpec{
			Mode:         contract.InterfaceModeComposite,
			Availability: contract.InterfaceAvailable,
			Reason:       compositeInterfaceReason,
		},
		Selection: contract.SelectionSpec{
			AgentSummary: description,
			UseWhen:      []string{intent},
			AvoidWhen: []string{
				"需要文件树移动、复制或普通钉盘文件操作时改用 drive；非文字文档按对象类型路由到 sheet、aitable、slides 或 wiki",
			},
			Examples: examples,
		},
		Identity: contract.ToolIdentitySpec{
			ProductID:      "doc",
			Name:           name,
			CanonicalPath:  "doc." + name,
			CLIPath:        cliPath,
			PrimaryCLIPath: cliPath,
		},
	}
}

func withDryRun(decl corecmd.ContractDecl, kind string, remoteReads bool) corecmd.ContractDecl {
	decl.DryRun = &contract.DryRunSpec{PreviewKind: kind, RemoteReads: remoteReads}
	return decl
}

func readShortcutContent(rt *shortcut.RuntimeContext, flag string) (string, error) {
	raw := rt.Str(flag)
	if raw == "-" {
		data, err := io.ReadAll(rt.Command().InOrStdin())
		if err != nil {
			return "", apperrors.NewValidation(fmt.Sprintf("--%s: 读取 stdin 失败: %v", flag, err))
		}
		return string(data), nil
	}
	if !strings.HasPrefix(raw, "@") {
		return raw, nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if path == "" || filepath.IsAbs(path) {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s 的 @file 只接受工作目录内的相对路径", flag))
	}
	cwd, err := docGetwd()
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("读取工作目录失败: %v", err))
	}
	realBase, err := docEvalSymlinks(cwd)
	if err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("解析工作目录失败: %v", err))
	}
	realPath, err := docEvalSymlinks(filepath.Join(realBase, filepath.Clean(path)))
	if err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s: 读取文件 %q 失败: %v", flag, path, err))
	}
	rel, err := docRel(realBase, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s 的 @file 不能逃逸工作目录", flag))
	}
	data, err := docReadFile(realPath)
	if err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s: 读取文件 %q 失败: %v", flag, path, err))
	}
	return string(data), nil
}

func validateJSONML(raw string) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("JSONML 解析失败: %v", err))
	}
	if _, ok := value.([]any); !ok {
		return "", apperrors.NewValidation("JSONML 顶层必须是数组")
	}
	normalized, _ := json.Marshal(value) // decoded JSON trees are always marshalable
	return string(normalized), nil
}

func docEnvelope(operation string, data any, steps ...map[string]any) map[string]any {
	return map[string]any{
		"ok":           true,
		"status":       "success",
		"operation":    operation,
		"steps":        steps,
		"data":         data,
		"warnings":     []string{},
		"compensation": map[string]any{"available": false, "reason": ""},
	}
}

func docPartialWriteError(operation, reason, stage, message string, cause error, data map[string]any, steps []map[string]any, compensation map[string]any) error {
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation(operation),
		apperrors.WithReason(reason),
		apperrors.WithFailureStage(stage),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithActions("inspect the completed steps before retrying", "use the compensation details to clean up or restore the document"),
		apperrors.WithDetails(map[string]any{
			"status":       "partial_success",
			"data":         data,
			"steps":        steps,
			"compensation": compensation,
		}),
		apperrors.WithCause(cause),
	)
}

func nestedString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, wrapper := range []string{"result", "data", "content"} {
		if inner, ok := data[wrapper].(map[string]any); ok {
			if value := nestedString(inner, keys...); value != "" {
				return value
			}
		}
	}
	return ""
}

func nestedMap(data map[string]any) map[string]any {
	for _, wrapper := range []string{"result", "data"} {
		if inner, ok := data[wrapper].(map[string]any); ok {
			return nestedMap(inner)
		}
	}
	return data
}

func stringSliceNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// blockIdentity normalizes the currently observed block response shapes. The
// element API returns element.id, while JSONML and older payloads use blockId
// or uuid. Callers pass an inherited parent identity for nested text maps.
func blockIdentity(values map[string]any, inherited string) string {
	for _, key := range []string{"blockId", "id", "uuid"} {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return inherited
}

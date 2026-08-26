// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package doc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

const (
	compositeInterfaceReason           = "Reviewed Doc Shortcut composite: the executable CLI owns validation, multi-step orchestration, local I/O, output projection, and confirmation; no single MCP interface represents the complete command contract."
	docContentInputDescription         = "内容字面量、@工作目录相对文件或 - 表示 stdin"
	docRequiredContentInputDescription = docContentInputDescription + "；相关动作要求时不能为空"
)

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
		return normalizeDocInputLineEndings(string(data)), nil
	}
	if !strings.HasPrefix(raw, "@") {
		return normalizeDocInputLineEndings(raw), nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
	if path == "" || filepath.IsAbs(path) {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s 的 @file 只接受工作目录内的相对路径；请先暂存到工作目录后传 @相对路径，或改用 --%s - 从 stdin 读取", flag, flag))
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
		return "", apperrors.NewValidation(fmt.Sprintf("--%s 的 @file 不能逃逸工作目录；请先暂存到工作目录后传 @相对路径，或改用 --%s - 从 stdin 读取", flag, flag))
	}
	data, err := docReadFile(realPath)
	if err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s: 读取文件 %q 失败: %v", flag, path, err))
	}
	return normalizeDocInputLineEndings(string(data)), nil
}

func normalizeDocInputLineEndings(content string) string {
	return strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
}

func validateWorkspaceInputPath(flag, raw string) error {
	path := strings.TrimSpace(raw)
	if path == "" || filepath.IsAbs(path) {
		return apperrors.NewValidation(fmt.Sprintf("--%s 只接受工作目录内已暂存文件的相对路径", flag))
	}
	cwd, err := docGetwd()
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("读取工作目录失败: %v", err))
	}
	realBase, err := docEvalSymlinks(cwd)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("解析工作目录失败: %v", err))
	}
	realPath, err := docEvalSymlinks(filepath.Join(realBase, filepath.Clean(path)))
	if err != nil {
		return apperrors.NewValidation(fmt.Sprintf("--%s: 读取文件 %q 失败: %v", flag, path, err))
	}
	rel, err := docRel(realBase, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return apperrors.NewValidation(fmt.Sprintf("--%s 不能逃逸工作目录；请先把文件暂存到工作目录", flag))
	}
	return nil
}

func validateJSONMLBody(cmd *cobra.Command, raw string) (string, error) {
	normalized, err := helpers.PrepareDocJSONMLBody(cmd, raw)
	if err != nil {
		return "", shortcutJSONMLValidationError(err)
	}
	return validateJSONMLElement(normalized)
}

func validateJSONMLNode(cmd *cobra.Command, raw string) (string, error) {
	normalized, err := helpers.PrepareDocJSONMLNode(cmd, raw)
	if err != nil {
		return "", shortcutJSONMLValidationError(err)
	}
	return validateJSONMLElement(normalized)
}

func validateJSONMLElement(raw string) (string, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("JSONML 解析失败: %v", err))
	}
	root, ok := value.([]any)
	if !ok {
		return "", apperrors.NewValidation("JSONML 顶层必须是数组")
	}
	if len(root) == 0 {
		return "", apperrors.NewValidation("JSONML 顶层数组不能为空")
	}
	if _, nestedRoot := root[0].([]any); nestedRoot {
		return "", apperrors.NewValidation("JSONML 顶层必须是单个元素，不能使用元素数组包裹；请传 [\"root\", {...}, ...] 或单个 block 元素")
	}
	if tag, ok := root[0].(string); !ok || strings.TrimSpace(tag) == "" {
		return "", apperrors.NewValidation("JSONML 第一个元素必须是非空标签名")
	}
	return raw, nil
}

func shortcutJSONMLValidationError(err error) error {
	message := strings.ReplaceAll(err.Error(), "--content-format", "--doc-format")
	message = strings.ReplaceAll(message, "--element", "--content")
	return apperrors.NewValidation(message)
}

func docEnvelope(operation string, data any, steps ...map[string]any) map[string]any {
	return map[string]any{
		"contractVersion": "doc.operation.v1",
		"ok":              true,
		"status":          "success",
		"complete":        true,
		"operation":       operation,
		"steps":           steps,
		"data":            data,
		"warnings":        []string{},
		"compensation":    map[string]any{"available": false, "reason": ""},
	}
}

// withDocWarnings fills the envelope's warnings slot, which docEnvelope has
// always emitted empty. Written as a wrapper rather than another docEnvelope
// parameter so its ~20 existing call sites stay untouched.
func withDocWarnings(envelope map[string]any, warnings []string) map[string]any {
	if len(warnings) > 0 {
		envelope["warnings"] = warnings
	}
	return envelope
}

type docFailureState string

const (
	docFailureFailed    docFailureState = "failed"
	docFailureRetryable docFailureState = "retryable"
	docFailureUnknown   docFailureState = "unknown"
)

type docFailureTransition struct {
	state            docFailureState
	reason           string
	retryable        bool
	executionStarted bool
	message          string
	actions          []string
}

func classifyDocWriteFailure(cause error) docFailureTransition {
	transition := docFailureTransition{
		state:            docFailureUnknown,
		reason:           "doc_write_commit_unknown",
		retryable:        false,
		executionStarted: true,
		message:          "文档写入结果未知；为避免重复创建或重复追加，已停止自动重试",
		actions:          []string{"先读取目标文档确认实际写入状态", "仅在确认服务端未提交后重新执行"},
	}
	var typed *apperrors.Error
	if !errors.As(cause, &typed) {
		return transition
	}
	reason := strings.ToLower(strings.TrimSpace(typed.Reason))
	message := strings.ToLower(typed.Message)
	if typed.Category == apperrors.CategoryValidation {
		if reason == "" {
			reason = "invalid_input"
		}
		return docFailureTransition{
			state:            docFailureFailed,
			reason:           reason,
			retryable:        false,
			executionStarted: false,
			message:          typed.Message,
			actions:          []string{"根据错误中的参数约束修正输入后重新执行"},
		}
	}
	permissionDenied := strings.Contains(reason, "403") || strings.Contains(reason, "permission") ||
		strings.Contains(message, "permission") || strings.Contains(message, "forbidden") || strings.Contains(message, "权限")
	if permissionDenied {
		return docFailureTransition{
			state:            docFailureFailed,
			reason:           "permission_denied",
			retryable:        false,
			executionStarted: false,
			message:          "当前身份没有执行该文档操作的权限；已终止，不会重试或改走同义命令",
			actions:          []string{"检查当前 profile 与文档权限", "获得权限后重新发起一次原操作"},
		}
	}
	if typed.Category == apperrors.CategoryAuth {
		return docFailureTransition{
			state:            docFailureFailed,
			reason:           "authentication_required",
			retryable:        false,
			executionStarted: false,
			message:          "当前身份认证不可用；已终止文档写入",
			actions:          []string{"检查当前 profile 与登录状态", "认证恢复后重新发起一次原操作"},
		}
	}
	if typed.ExecutionStarted != nil && !*typed.ExecutionStarted {
		transition.state = docFailureFailed
		transition.executionStarted = false
		transition.retryable = typed.RetryableSet && typed.Retryable
		if transition.retryable {
			transition.state = docFailureRetryable
		}
		if reason != "" {
			transition.reason = reason
		} else {
			transition.reason = "doc_write_not_started"
		}
		transition.message = "文档写入已确认尚未开始"
		transition.actions = []string{"仅在 retryable=true 时按服务端 retry-after 有界重试一次"}
	}
	return transition
}

func docUnknownWriteError(operation, stage, nodeID string, cause error) error {
	var existing *apperrors.Error
	if errors.As(cause, &existing) {
		if existing.Category == apperrors.CategoryValidation {
			return cause
		}
		if status, ok := existing.Details["status"].(string); ok && (status == "unknown" || status == "partial_success") {
			return cause
		}
	}
	transition := classifyDocWriteFailure(cause)
	details := map[string]any{
		"contractVersion": "doc.operation.v1",
		"status":          string(transition.state),
		"state":           string(transition.state),
		"nodeId":          nodeID,
		"stage":           stage,
	}
	options := []apperrors.Option{
		apperrors.WithOperation(operation),
		apperrors.WithReason(transition.reason),
		apperrors.WithFailureStage(stage),
		apperrors.WithExecutionStarted(transition.executionStarted),
		apperrors.WithRetryable(transition.retryable),
		apperrors.WithActions(transition.actions...),
		apperrors.WithDetails(details),
		apperrors.WithCause(cause),
	}
	if transition.reason == "permission_denied" || transition.reason == "authentication_required" {
		return apperrors.NewAuth(transition.message, options...)
	}
	return apperrors.NewAPI(transition.message, options...)
}

func docVerificationError(operation, stage, nodeID string, cause error, steps []map[string]any) error {
	return docPartialWriteError(
		operation,
		"doc_write_verification_failed",
		stage,
		"文档写入已经执行，但回读验证失败；请先检查当前内容，不要直接重试写入",
		cause,
		map[string]any{"nodeId": nodeID, "verified": false},
		steps,
		map[string]any{"available": false, "reason": "inspect the current document before choosing recovery"},
	)
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
			"contractVersion": "doc.operation.v1",
			"status":          "partial_success",
			"data":            data,
			"steps":           steps,
			"compensation":    compensation,
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

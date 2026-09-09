// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

var whiteboardExportAfter = time.After

var whiteboardExportWait = func(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-whiteboardExportAfter(delay):
		return nil
	}
}
var whiteboardExportAbs = filepath.Abs
var whiteboardExportStat = os.Stat
var whiteboardExportFileStat = (*os.File).Stat

func newStandaloneWhiteboardExportCommands() (*cobra.Command, *cobra.Command) {
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出独立白板到本地",
		Long:  "提交独立白板导出任务，轮询完成后按白板名称下载为 PNG 或 PDF。--output 是目标目录。",
		Example: "  dws whiteboard export --node <WHITEBOARD_NODE_ID> --output ./exports --format json\n" +
			"  dws whiteboard export --node <WHITEBOARD_NODE_ID> --export-format pdf --output ./exports --format json",
		RunE: runStandaloneWhiteboardExport,
	}
	exportCmd.Flags().String("node", "", "独立白板节点 ID/URL（必填）")
	exportCmd.Flags().String("export-format", "png", "导出格式: png / pdf")
	exportCmd.Flags().String("output", "", "本地目标目录（必填，文件名自动使用白板名称）")

	getCmd := &cobra.Command{
		Use:   "export-get",
		Short: "查询白板导出任务并下载",
		Long:  "查询已有白板导出任务；成功后使用下载地址中的标准文件名保存到本地目标目录。",
		RunE:  runStandaloneWhiteboardExportGet,
	}
	getCmd.Flags().String("job-id", "", "导出任务 ID（必填）")
	getCmd.Flags().String("export-format", "png", "导出格式: png / pdf")
	getCmd.Flags().String("output", "", "本地目标目录（必填）")

	DeclareLeafMetadata(exportCmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "unknown"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "whiteboard", Name: "export_whiteboard", CanonicalPath: "whiteboard.export_whiteboard", CLIPath: "whiteboard export", PrimaryCLIPath: "whiteboard export"},
			Description: "导出独立白板并按白板名称下载 PNG 或 PDF 到本地目录",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "命令提交导出任务、轮询任务并使用下载地址中的标准文件名安全落盘，不能绑定为单一 interface_ref"},
			Selection:   contract.SelectionSpec{AgentSummary: "导出独立白板并按白板名称保存到本地", UseWhen: []string{"用户要把独立 .adraw 白板导出为 PNG 或 PDF 本地文件时"}, AvoidWhen: []string{"文档内嵌白板不使用本命令；只读取 OpenNodes 使用 whiteboard query"}, Examples: []string{"dws whiteboard export --node <WHITEBOARD_NODE_ID> --output ./exports --format json"}},
			Parameters:  []contract.ParamDecl{{Name: "node", Property: "nodeId", Required: boolPtr(true)}, {Name: "export-format", Property: "exportFormat", Required: boolPtr(false), Enum: []string{"png", "pdf"}}, {Name: "output", Required: boolPtr(true)}},
			Result:      whiteboardExportResultSpec(),
		},
	})
	DeclareLeafMetadata(getCmd, LeafSpec{
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "whiteboard", Name: "query_export_job", CanonicalPath: "whiteboard.query_export_job", CLIPath: "whiteboard export-get", PrimaryCLIPath: "whiteboard export-get"},
			Description: "查询已有白板导出任务并下载结果到本地目录",
			DryRun:      &contract.DryRunSpec{PreviewKind: "request", RemoteReads: false},
			Interface:   &contract.InterfaceSpec{Mode: "composite", Availability: "available", Reason: "命令查询远端任务并将签名下载地址安全落盘"},
			Selection:   contract.SelectionSpec{AgentSummary: "恢复查询已有白板导出任务并下载", UseWhen: []string{"已有 export_whiteboard 返回的 jobId，需要恢复查询和下载时"}, AvoidWhen: []string{"没有 jobId 时使用 whiteboard export 提交新任务"}, Examples: []string{"dws whiteboard export-get --job-id <JOB_ID> --output ./exports --format json"}},
			Parameters:  []contract.ParamDecl{{Name: "job-id", Property: "jobId", Required: boolPtr(true)}, {Name: "export-format", Required: boolPtr(false), Enum: []string{"png", "pdf"}}, {Name: "output", Required: boolPtr(true)}},
			Result:      whiteboardExportResultSpec(),
		},
	})
	return exportCmd, getCmd
}

func runStandaloneWhiteboardExport(cmd *cobra.Command, _ []string) error {
	node := strings.TrimSpace(mustGetFlag(cmd, "node"))
	format := strings.ToLower(strings.TrimSpace(mustGetFlag(cmd, "export-format")))
	outputDir := strings.TrimSpace(mustGetFlag(cmd, "output"))
	if node == "" || outputDir == "" {
		return fmt.Errorf("flags --node and --output are required")
	}
	if err := validateWhiteboardExportFormat(format); err != nil {
		return err
	}
	if deps.Caller.DryRun() {
		return callMCPToolOnServer(whiteboardServerID, whiteboardcore.StandaloneExportTool, map[string]any{"nodeId": node, "exportFormat": format})
	}
	if err := validateWhiteboardExportDirectory(outputDir); err != nil {
		return err
	}
	response, err := callWhiteboardToolResult(cmd, whiteboardcore.StandaloneExportTool, map[string]any{"nodeId": node, "exportFormat": format})
	if err != nil {
		return err
	}
	jobID := strings.TrimSpace(whiteboardString(unwrapWhiteboardExportResult(response)["jobId"]))
	if jobID == "" {
		return invalidWhiteboardToolResult(whiteboardcore.StandaloneExportTool, fmt.Errorf("response missing jobId"))
	}
	return pollAndDownloadWhiteboardExport(cmd, jobID, format, outputDir)
}

func runStandaloneWhiteboardExportGet(cmd *cobra.Command, _ []string) error {
	jobID := strings.TrimSpace(mustGetFlag(cmd, "job-id"))
	format := strings.ToLower(strings.TrimSpace(mustGetFlag(cmd, "export-format")))
	outputDir := strings.TrimSpace(mustGetFlag(cmd, "output"))
	if jobID == "" || outputDir == "" {
		return fmt.Errorf("flags --job-id and --output are required")
	}
	if err := validateWhiteboardExportFormat(format); err != nil {
		return err
	}
	if deps.Caller.DryRun() {
		return callMCPToolOnServer(whiteboardServerID, whiteboardcore.StandaloneExportQueryTool, map[string]any{"jobId": jobID})
	}
	if err := validateWhiteboardExportDirectory(outputDir); err != nil {
		return whiteboardExportRecoveryError(err, jobID, format, outputDir)
	}
	return pollAndDownloadWhiteboardExport(cmd, jobID, format, outputDir)
}

func validateWhiteboardExportFormat(format string) error {
	if format != "png" && format != "pdf" {
		return fmt.Errorf("--export-format must be png or pdf")
	}
	return nil
}

func pollAndDownloadWhiteboardExport(cmd *cobra.Command, jobID, format, outputDir string) (err error) {
	defer func() {
		if err != nil {
			err = whiteboardExportRecoveryError(err, jobID, format, outputDir)
		}
	}()
	const maxPolls = 30
	var result map[string]any
	for attempt := 1; attempt <= maxPolls; attempt++ {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		if attempt > 1 {
			if err := whiteboardExportWait(cmd.Context(), taskPollInterval(attempt)); err != nil {
				return fmt.Errorf("白板导出轮询被取消 (jobId=%s): %w", jobID, err)
			}
			// Cancellation and the timer may become ready together. Recheck after
			// waiting so a timer win cannot start another remote query.
			if err := cmd.Context().Err(); err != nil {
				return fmt.Errorf("白板导出轮询被取消 (jobId=%s): %w", jobID, err)
			}
		}
		response, err := callWhiteboardToolResult(cmd, whiteboardcore.StandaloneExportQueryTool, map[string]any{"jobId": jobID})
		if err != nil {
			return fmt.Errorf("查询白板导出任务失败 (jobId=%s): %w", jobID, err)
		}
		result = unwrapWhiteboardExportResult(response)
		returnedJobID := strings.TrimSpace(whiteboardString(result["jobId"]))
		if returnedJobID != "" && returnedJobID != jobID {
			return invalidWhiteboardToolResult(whiteboardcore.StandaloneExportQueryTool, fmt.Errorf("response jobId mismatch"))
		}
		status := strings.ToUpper(strings.TrimSpace(whiteboardString(result["status"])))
		switch status {
		case "SUCCESS":
			downloadURL := strings.TrimSpace(whiteboardString(result["downloadUrl"]))
			if downloadURL == "" {
				return invalidWhiteboardToolResult(whiteboardcore.StandaloneExportQueryTool, fmt.Errorf("SUCCESS response missing downloadUrl"))
			}
			return downloadWhiteboardExport(cmd.Context(), jobID, format, outputDir, downloadURL, result)
		case "INIT", "PENDING", "PROCESSING":
			continue
		case "FAILED", "CANCELLED", "TIMEOUT", "PARTIAL_FAILED":
			return fmt.Errorf("白板导出任务失败 (jobId=%s, status=%s): %s", jobID, status, whiteboardString(result["message"]))
		default:
			return fmt.Errorf("白板导出任务返回未知状态 (jobId=%s, status=%s)", jobID, status)
		}
	}
	return fmt.Errorf("白板导出任务仍在处理中 (jobId=%s)；请使用 dws whiteboard export-get 恢复查询", jobID)
}

func downloadWhiteboardExport(ctx context.Context, jobID, format, outputDir, downloadURL string, result map[string]any) error {
	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("白板下载地址无效")
	}
	rawName := filepath.Base(strings.ReplaceAll(parsedURL.Path, "\\", "/"))
	if rawName == "." || rawName == "/" || strings.HasSuffix(parsedURL.Path, "/") {
		return invalidWhiteboardToolResult(whiteboardcore.StandaloneExportQueryTool, fmt.Errorf("downloadUrl missing standard filename"))
	}
	fileName := sanitizeFileName(rawName)
	if fileName == "unnamed" {
		return invalidWhiteboardToolResult(whiteboardcore.StandaloneExportQueryTool, fmt.Errorf("downloadUrl missing standard filename"))
	}
	if ext := filepath.Ext(fileName); ext == "" {
		fileName += "." + format
	} else if !strings.EqualFold(ext, "."+format) {
		return whiteboardExportFormatMismatch(jobID, format, strings.TrimPrefix(strings.ToLower(ext), "."))
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("创建白板导出目录失败: %w", err)
	}
	outputPath, err := whiteboardExportAbs(filepath.Join(outputDir, fileName))
	if err != nil {
		return fmt.Errorf("解析白板导出路径失败: %w", err)
	}
	if err := checkDownloadConflict(outputPath, false, "whiteboard.export_whiteboard"); err != nil {
		return whiteboardExportDownloadError(err)
	}
	var size int64
	if err := downloadViaTemp(outputPath, false, func(tmpPath string) error {
		if err := whiteboardExportHTTPGet(ctx, downloadURL, nil, tmpPath); err != nil {
			return err
		}
		var err error
		size, err = validateWhiteboardExportFile(tmpPath, format)
		return err
	}); err != nil {
		return fmt.Errorf("下载白板导出文件失败: %w", whiteboardExportDownloadError(err))
	}
	return deps.Out.PrintJSON(map[string]any{"success": true, "jobId": jobID, "status": "SUCCESS", "exportFormat": format, "fileName": fileName, "outputPath": outputPath, "size": size, "logId": whiteboardString(result["logId"])})
}

// unwrapWhiteboardExportResult accepts the response shapes used by the
// whiteboard gateway during rollout. callWhiteboardToolResult has already
// decoded a string resultJson, so this function only selects the business
// object and never reparses arbitrary response text.
func unwrapWhiteboardExportResult(response map[string]any) map[string]any {
	if response == nil {
		return nil
	}
	for _, key := range []string{"result", "resultJson", "data"} {
		if result, ok := response[key].(map[string]any); ok {
			return result
		}
	}
	return response
}

func whiteboardExportFormatMismatch(jobID, requested, actual string) error {
	return &CLIError{
		Code:       CodeInvalidParam,
		Message:    fmt.Sprintf("任务格式不匹配：jobId=%s 的下载文件格式为 %s，但 --export-format 指定为 %s", jobID, actual, requested),
		Suggestion: fmt.Sprintf("请改用 --export-format %s，或按 %s 格式重新提交白板导出任务", actual, requested),
		Operation:  whiteboardServerID + "/" + whiteboardcore.StandaloneExportQueryTool,
	}
}

func whiteboardExportResultSpec() *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","description":"已下载到本地的独立白板导出结果","properties":{"jobId":{"type":"string","description":"白板导出任务 ID"},"status":{"type":"string","description":"白板导出任务终态"},"exportFormat":{"type":"string","description":"实际导出格式"},"fileName":{"type":"string","description":"下载地址提供的标准白板文件名"},"outputPath":{"type":"string","description":"本地文件绝对路径"},"size":{"type":"integer","description":"本地文件字节数"},"logId":{"type":"string","description":"服务端诊断日志 ID"}}}`)}
}

// Validate the temporary file before publishing it, so failed downloads remain retryable.
func validateWhiteboardExportFile(path, format string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	signature := "\x89PNG\r\n\x1a\n"
	if format == "pdf" {
		signature = "%PDF-"
	}
	header := make([]byte, len(signature))
	if _, err := io.ReadFull(file, header); err != nil || string(header) != signature {
		return 0, fmt.Errorf("白板导出文件为空、截断或不是有效的 %s 文件", format)
	}
	info, err := whiteboardExportFileStat(file)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func whiteboardExportDownloadError(err error) error {
	var cliErr *CLIError
	if errors.As(err, &cliErr) && cliErr.Code == CodeFileAlreadyExists {
		copy := *cliErr
		copy.Suggestion = "请使用其他 --output 目录，或先处理已有文件后重试；白板导出不会覆盖已有文件"
		return &copy
	}
	return err
}

// Inspect existing ancestors without creating anything during validation.
func validateWhiteboardExportDirectory(directory string) error {
	path, err := whiteboardExportAbs(directory)
	if err != nil {
		return err
	}
	for {
		info, err := whiteboardExportStat(path)
		if err == nil {
			if !info.IsDir() {
				return &CLIError{Code: CodeInvalidPath, Message: "--output 必须是目录，已有路径不是目录: " + path}
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("检查白板导出目录失败: %w", err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return err
		}
		path = parent
	}
}

func whiteboardExportRecoveryError(err error, jobID, format, directory string) error {
	return fmt.Errorf("%w\n任务 jobId=%s；修正问题后可恢复查询（POSIX shell）：dws whiteboard export-get --job-id %s --export-format %s --output %s --format json",
		err, jobID, ShellQuoteArg(jobID), ShellQuoteArg(format), ShellQuoteArg(directory))
}

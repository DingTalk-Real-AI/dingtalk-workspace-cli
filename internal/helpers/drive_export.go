package helpers

// drive_export.go 实现了 drive export 通用导出命令。
//
// drive export 是通用的导出命令，支持所有文档类型（adoc/axls/appt），
// 可导出为 docx/xlsx/pptx/markdown/pdf 等格式；doc export 与 sheet export
// 是分别针对在线文档和在线表格的产品级入口。三者共享统一的
// 提交→轮询→下载流程。
//
// 同步自闭源 MR 28967420（通用导出支持）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// supportedExportFormats is the global format→extension map used by the
// drive-level (universal) export command.
var supportedExportFormats = map[string]string{
	"docx":     ".docx",
	"xlsx":     ".xlsx",
	"markdown": ".md",
	"md":       ".md",
	"pdf":      ".pdf",
	"pptx":     ".pptx",
}

// docExtensionToDefaultExportFormat maps a DingTalk online document extension
// (as returned by get_document_info) to its preferred export format. Used by
// drive export's auto-detection so that, e.g., a presentation (appt) defaults
// to pptx without requiring the user to pass --export-format.
var docExtensionToDefaultExportFormat = map[string]string{
	"adoc": "docx",
	"axls": "xlsx",
	"appt": "pptx",
}

// driveExportCallTool routes submit_export_job (and any format auto-detection
// probes) to the doc MCP server. Polling always uses queryTaskCallTool
// (drive server), independent of this closure.
func driveExportCallTool(ctx context.Context, tool string, args map[string]any) (string, error) {
	return callMCPToolReturnTextOnServer(ctx, "doc", tool, args)
}

// isRetryableTaskQueryError 判断轮询中的 query_task 错误是否为临时性传输故障。
// 仅网络类错误与响应解析失败（网关临时返回非 JSON 错误页）继续重试；
// 业务/鉴权类确定性错误立即上抛，避免用户空等约 5 分钟后原始错误被掩盖。
func isRetryableTaskQueryError(err error) bool {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr.Code == CodeNetworkTimeout || cliErr.Code == CodeNetworkUnreachable
	}
	// checkTaskBusinessError 的确定性业务错误（裸 error）立即退出；
	// 解析失败更可能是网关临时故障（如 5xx 错误页），归入可重试。
	return strings.HasPrefix(err.Error(), "解析 query_task 响应失败")
}

// pollDriveExportJob polls the unified query_task tool (drive endpoint) with
// progressive backoff until the job reaches a terminal state or the 30-attempt
// cap is hit.
//
// Terminal handling:
//   - SUCCESS → returns (ResultURL, ResultName)
//   - FAILED / PARTIAL_FAILED / TIMEOUT → returns an error
//   - PENDING / PROCESSING / unknown → keep polling
//
// Query errors go through isRetryableTaskQueryError: transient network
// failures and response-parse failures keep polling (the last error is
// retained and appended to the timeout message), while deterministic
// business/auth errors abort immediately instead of burning the ~5-minute
// polling cap and masking the original error with a timeout.
func pollDriveExportJob(ctx context.Context, jobID string) (downloadURL string, fileName string, err error) {
	const maxPolls = 30

	var lastQueryErr error

	for attempt := 1; attempt <= maxPolls; attempt++ {
		interval := taskPollInterval(attempt)
		printTaskProgress(fmt.Sprintf("    第 %d/%d 次查询，等待 %v ...", attempt, maxPolls, interval))

		select {
		case <-ctx.Done():
			return "", "", fmt.Errorf("导出轮询被取消 (taskId=%s): %w", jobID, ctx.Err())
		case <-helperAfter(interval):
		}

		result, queryErr := QueryTask(ctx, jobID, "export")
		if queryErr != nil {
			if !isRetryableTaskQueryError(queryErr) {
				return "", "", fmt.Errorf("查询导出任务失败 (taskId=%s): %w", jobID, queryErr)
			}
			lastQueryErr = queryErr
			printTaskProgress(fmt.Sprintf("  [%d/%d] 查询失败，将继续轮询: %v", attempt, maxPolls, queryErr))
			continue
		}

		switch result.Status {
		case TaskStatusSuccess:
			printTaskProgress(fmt.Sprintf("  [%d/%d] 状态: SUCCESS", attempt, maxPolls))
			if result.ResultURL == "" {
				return "", "", fmt.Errorf("导出成功但 downloadUrl 为空 (taskId=%s)", jobID)
			}
			return result.ResultURL, result.ResultName, nil
		case TaskStatusFailed, TaskStatusPartialFailed:
			message := result.Message
			if message == "" {
				message = fmt.Sprintf("导出任务失败 (taskId=%s, status=%s)", jobID, result.Status)
			}
			return "", "", fmt.Errorf("%s", message)
		case TaskStatusTimeout:
			return "", "", fmt.Errorf("导出任务超时 (taskId=%s)", jobID)
		case TaskStatusPending, TaskStatusProcessing:
			// NormalizeStatus only ever produces the six enum values handled
			// by the cases above, so no default branch is reachable.
			printTaskProgress(fmt.Sprintf("  [%d/%d] 状态: PROCESSING", attempt, maxPolls))
		}
	}

	if lastQueryErr != nil {
		return "", "", fmt.Errorf("导出任务超时：已轮询 %d 次仍在处理中 (taskId=%s)，请稍后使用 dws drive task get --type export --id %s 手动查询；最后一次查询错误: %v", maxPolls, jobID, jobID, lastQueryErr)
	}
	return "", "", fmt.Errorf("导出任务超时：已轮询 %d 次仍在处理中 (taskId=%s)，请稍后使用 dws drive task get --type export --id %s 手动查询", maxPolls, jobID, jobID)
}

// inferExportFormatFromDocInfo probes the document's extension via
// get_document_info and maps it to a default export format. On any failure or
// unknown extension it falls back to "docx".
func inferExportFormatFromDocInfo(ctx context.Context, nodeID string) string {
	text, err := driveExportCallTool(ctx, "get_document_info", map[string]any{"nodeId": nodeID})
	if err != nil {
		return "docx"
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "docx"
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	ext, _ := data["extension"].(string)
	ext = strings.ToLower(strings.TrimSpace(ext))
	if format, ok := docExtensionToDefaultExportFormat[ext]; ok {
		return format
	}
	return "docx"
}

// runDriveExport is the RunE for `dws drive export`.
func runDriveExport(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	node, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if err != nil {
		return err
	}
	outputPath, _ := cmd.Flags().GetString("output")
	asyncMode, _ := cmd.Flags().GetBool("async")

	// ── Format resolution ──
	format := ""
	formatExplicit := false
	if f := cmd.Flags().Lookup("export-format"); f != nil && f.Changed {
		format = strings.TrimSpace(f.Value.String())
		formatExplicit = format != ""
	}
	if !formatExplicit {
		if f := cmd.Flags().Lookup("format"); f != nil && f.Changed {
			legacy := strings.TrimSpace(f.Value.String())
			switch strings.ToLower(legacy) {
			case "json", "table", "raw", "pretty":
				// global output-format value: not an export format
			default:
				if legacy != "" {
					format = legacy
					formatExplicit = true
				}
			}
		}
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "md" {
		format = "markdown"
	}

	// 显式指定的格式先于 dry-run 校验：dry-run 是一次忠实预览，非法格式应
	// 与真实执行同样 fail-fast（且不发起任何远端调用）。
	if formatExplicit {
		if _, ok := supportedExportFormats[format]; !ok {
			return fmt.Errorf("不支持的导出格式: %s", format)
		}
	}

	// ── DryRun preview ──
	// 必须位于格式自动探测之前：探测会真实调用远端 get_document_info，而
	// dry-run 不允许任何远端调用。未显式指定格式时只标注自动探测语义。
	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "导出通用文档（提交+轮询+下载）")
		deps.Out.PrintKeyValue("通用文档", node)
		if outputPath != "" {
			deps.Out.PrintKeyValue("输出", outputPath)
		}
		if formatExplicit {
			deps.Out.PrintKeyValue("格式", format)
		} else {
			deps.Out.PrintKeyValue("格式", "自动探测（执行时按文档类型推断）")
		}
		if asyncMode {
			deps.Out.PrintKeyValue("异步模式", "是")
		}
		return nil
	}

	// Auto-detect from document info when the format was not explicitly chosen.
	// inferExportFormatFromDocInfo never returns an empty or unsupported format
	// (it falls back to "docx" itself), and explicit formats were validated
	// above, so the resolved format needs no further validation here.
	if !formatExplicit {
		format = inferExportFormatFromDocInfo(ctx, node)
	}
	fileExt := supportedExportFormats[format]

	// ── Step 1: submit export job ──
	printTaskProgress("[1/3] 提交导出任务...")
	submitText, err := driveExportCallTool(ctx, "submit_export_job", map[string]any{
		"nodeId":       node,
		"exportFormat": format,
	})
	if err != nil {
		return fmt.Errorf("提交导出任务失败: %w", err)
	}
	jobID, err := parseExportSubmitResult(submitText)
	if err != nil {
		return err
	}
	printTaskProgress(fmt.Sprintf("    任务已提交，taskId: %s", jobID))

	// ── --async: return immediately ──
	if asyncMode {
		result := &TaskResult{
			ID:      jobID,
			Type:    "export",
			Status:  TaskStatusPending,
			Message: "任务已提交，请稍后查询",
		}
		// 后续查询提示走 stderr（printTaskProgress），不污染 stdout；结构化
		// TaskResult 打印到 stdout 并将其写错误上抛（对齐仓库
		// return deps.Out.PrintJSON(...) 惯例，避免管道破裂时静默吞错）。
		printTaskProgress("异步模式：使用以下命令查询状态：")
		printTaskProgress(fmt.Sprintf("  dws drive task get --type export --id %s", jobID))
		return deps.Out.PrintJSON(result)
	}

	// ── Step 2: progressive backoff polling ──
	printTaskProgress("[2/3] 等待导出完成...")
	downloadURL, fileName, err := pollDriveExportJob(ctx, jobID)
	if err != nil {
		return err
	}

	// ── No output path: print downloadUrl and exit ──
	// json 模式下 stdout 只输出单一 JSON 结果对象（taskId/downloadUrl，camelCase），
	// 保持机器可解析；结构对齐 sheet export 无 --output 分支的
	// {"success":true,...,"downloadUrl":...} 既有惯例。时效性提示始终走
	// stderr（printTaskProgress），不污染 stdout。
	if outputPath == "" {
		if deps.Caller.Format() == "json" {
			printTaskProgress("导出完成。downloadUrl 具有时效性，请尽快下载。")
			return deps.Out.PrintJSON(map[string]any{
				"success":     true,
				"taskId":      jobID,
				"downloadUrl": downloadURL,
			})
		}
		deps.Out.PrintKeyValue("taskId", jobID)
		deps.Out.PrintKeyValue("downloadUrl", downloadURL)
		printTaskProgress("导出完成。downloadUrl 具有时效性，请尽快下载。")
		return nil
	}

	// ── Step 3: resolve output path and download ──
	outputPath = resolveDriveExportOutputPath(outputPath, downloadURL, fileName, fileExt, jobID)
	printTaskProgress(fmt.Sprintf("[3/3] 下载文件到 %s ...", outputPath))
	if err := httpGetFile(ctx, downloadURL, nil, outputPath); err != nil {
		return fmt.Errorf("文件下载失败 (taskId=%s): %w", jobID, err)
	}
	printTaskProgress(fmt.Sprintf("导出完成: %s", outputPath))

	// json 模式下 stdout 输出单一结果对象（taskId/outputPath/downloadUrl，
	// camelCase），保持机器可解析；结构对齐 sheet export 下载完成分支的
	// {"success":true,...} 既有惯例。进度提示始终走 stderr（printTaskProgress）。
	if deps.Caller.Format() == "json" {
		return deps.Out.PrintJSON(map[string]any{
			"success":     true,
			"taskId":      jobID,
			"outputPath":  outputPath,
			"downloadUrl": downloadURL,
		})
	}
	return nil
}

// runDriveExportGet queries a previously-submitted drive export task by ID.
func runDriveExportGet(cmd *cobra.Command, _ []string) error {
	taskID, err := mustFlagOrFallback(cmd, "task-id", "job-id")
	if err != nil {
		return err
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "查询导出任务状态")
		deps.Out.PrintKeyValue("Task ID", taskID)
		return nil
	}

	// query_task is registered on the drive (dingpan) MCP server; route there
	// explicitly to avoid resolveProductID misrouting under doc/sheet context.
	result, err := QueryTask(cmd.Context(), taskID, "export")
	if err != nil {
		return err
	}
	return deps.Out.PrintJSON(result)
}

// inferExportFilename extracts a safe local filename from a download URL.
func inferExportFilename(rawURL string, fallback string) string {
	name := ""
	if idx := strings.LastIndex(rawURL, "/"); idx >= 0 && idx < len(rawURL)-1 {
		name = rawURL[idx+1:]
		if qIdx := strings.Index(name, "?"); qIdx >= 0 {
			name = name[:qIdx]
		}
	}
	if decoded, err := url.PathUnescape(name); err == nil && decoded != "" {
		name = decoded
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "" || name == "." || name == "/" {
		return fallback
	}
	return name
}

// resolveDriveExportOutputPath resolves the final local destination path. If
// outputPath points to an existing directory, a filename is inferred
// (preferring the task-supplied fileName, then the URL-derived name) and the
// extension is aligned to fileExt before joining with the directory.
// Non-directory paths are returned unchanged.
func resolveDriveExportOutputPath(outputPath, downloadURL, fileName, fileExt, jobID string) string {
	fi, statErr := os.Stat(outputPath)
	if statErr != nil || !fi.IsDir() {
		return outputPath
	}

	filename := strings.TrimSpace(fileName)
	if filename != "" {
		filename = sanitizeFileName(filename)
	}
	if filename == "" || filename == "unnamed" {
		filename = sanitizeFileName(inferExportFilename(downloadURL, ""))
	}
	if filename == "" || filename == "unnamed" {
		filename = fmt.Sprintf("drive-export-%s%s", jobID, fileExt)
		return filepath.Join(outputPath, filename)
	}
	if ext := filepath.Ext(filename); ext == "" {
		filename += fileExt
	} else if !strings.EqualFold(ext, fileExt) {
		filename = strings.TrimSuffix(filename, ext) + fileExt
	}
	return filepath.Join(outputPath, filename)
}

// newDriveExportCmd builds the `dws drive export` command tree
// (export + export get).
func newDriveExportCmd() *cobra.Command {
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出通用文档 (docx / xlsx / markdown / pdf / pptx)",
		Long: `将钉钉在线文档导出为本地文件，自动识别文档类型并选择合适的导出格式。

支持的导出格式 (--export-format):
  docx       Word 文档 (默认)
  xlsx       Excel 表格
  markdown   Markdown 文件 (.md，别名 md)
  pdf        PDF 文档
  pptx       PowerPoint 演示文稿

格式自动识别:
  未显式指定 --export-format 时，根据文档扩展名自动选择默认格式：
  adoc → docx, axls → xlsx, appt → pptx；识别失败回退 docx。

CLI 内部自动完成全部流程：
  1. 提交导出任务
  2. 渐进式退避轮询等待完成（最多约 5 分钟）
  3. 导出成功后自动下载文件到 --output 指定路径

如果轮询超时仍未完成，会输出 taskId 供后续手动查询：
  dws drive task get --type export --id <taskId>

--async 异步模式：
  仅提交任务并立即返回 taskId（TaskResult JSON），不轮询、不下载。

--output 可选：
  未指定 --output 时，导出成功后仅返回 downloadUrl（链接有时效性，请尽快下载）。`,
		Example: `  # 导出为 docx (默认)
  dws drive export --node "https://alidocs.dingtalk.com/i/nodes/xxx" --output ./exported.docx

  # 显式指定格式
  dws drive export --node <DOC_ID> --export-format pdf --output ./exported.pdf

  # --output 传入目录时，自动推断文件名
  dws drive export --node <DOC_ID> --output ~/downloads/

  # 不指定 --output，仅返回 downloadUrl
  dws drive export --node <DOC_ID>

  # 异步模式：只提交不等待，立即返回 taskId
  dws drive export --node <DOC_ID> --async`,
		RunE: runDriveExport,
	}
	exportCmd.Flags().String("node", "", "要导出的文档标识，支持 URL 或 dentryUuid (必填)")
	exportCmd.Flags().String("export-format", "docx", "导出格式：docx (默认) / xlsx / markdown (或 md) / pdf / pptx")
	exportCmd.Flags().String("output", "", "本地保存路径，文件路径或目录 (可选)")
	exportCmd.Flags().Bool("async", false, "异步模式：提交导出任务后立即返回 taskId，不等待完成")

	// --node hidden aliases (cross-product compatibility).
	for _, name := range []string{"url", "id", "node-id", "doc-id", "file-id"} {
		exportCmd.Flags().String(name, "", "--node 的别名")
		_ = exportCmd.Flags().MarkHidden(name)
	}

	// export get: manual one-shot status query (fallback after timeout/interruption).
	exportGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询导出任务状态",
		Long: `根据 taskId 查询导出任务的当前状态（仅查询一次，不轮询）。

典型场景:
  - dws drive export 超时或中断后，手动查询任务最终结果
  - 确认导出任务是否已完成
  - 获取已完成任务的下载链接

任务状态:
  PROCESSING  处理中
  SUCCESS     导出成功，返回 resultUrl（下载链接）
  FAILED      导出失败`,
		Example: `  dws drive export get --task-id <TASK_ID>`,
		RunE:    runDriveExportGet,
		Args:    cobra.NoArgs,
	}
	exportGetCmd.Flags().String("task-id", "", "导出任务 ID (必填)")
	exportGetCmd.Flags().String("job-id", "", "--task-id 的别名（向后兼容）")
	_ = exportGetCmd.Flags().MarkHidden("job-id")

	DeclareLeafMetadata(exportCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "export_document",
				CanonicalPath:  "drive.export_document",
				CLIPath:        "drive export",
				PrimaryCLIPath: "drive export",
			},
			Description: "导出通用文档（adoc/axls/appt）为 docx/xlsx/markdown/pdf/pptx，自动识别类型",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "命令提交导出任务后经统一 query_task 轮询并下载，不能绑定为单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "导出通用文档（adoc/axls/appt）为 docx/xlsx/markdown/pdf/pptx，自动识别类型",
				UseWhen: []string{
					"用户要导出文档且不确定具体类型（文档/表格/演示）或明确要求按类型导出 xlsx/pptx 时",
					"未指定格式、希望按文档类型自动选择导出格式时",
				},
				AvoidWhen: []string{
					"明确是在线文档(adoc)导出 docx/markdown/pdf 时优先 dws doc export",
					"明确是在线表格(axls)导出 xlsx 时优先 dws sheet export",
				},
				Examples: []string{
					"dws drive export --node <ID> --output ./out.docx --format json",
					"dws drive export --node <ID> --export-format pdf --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "async", Property: "async"},
				{Name: "export-format", Property: "exportFormat"},
				{Name: "node", Property: "nodeId", Required: boolPtr(true)},
				{Name: "output", Property: "output"},
			},
		},
	})

	DeclareLeafMetadata(exportGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "drive",
				Name:           "query_export_task",
				CanonicalPath:  "drive.query_export_task",
				CLIPath:        "drive export get",
				PrimaryCLIPath: "drive export get",
			},
			Description: "查询 drive 导出任务状态（TaskResult JSON）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "drive", RPCName: "query_task"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询 drive 导出任务状态（TaskResult JSON）",
				UseWhen:      []string{"dws drive export --async 提交后或轮询超时后手动查询导出结果时"},
				AvoidWhen:    []string{"非导出任务用 dws drive task get 传 --type；doc 原始响应用 dws doc export get"},
				Examples:     []string{"dws drive export get --task-id <TASK_ID> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "task-id", Property: "taskId", Required: boolPtr(true)},
			},
		},
	})

	exportCmd.AddCommand(exportGetCmd)
	newHybridGroupCommand(exportCmd)

	return exportCmd
}

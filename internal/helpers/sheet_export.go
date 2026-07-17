package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/asynctask"
)

func runSheetExport(cmd *cobra.Command, _ []string) error {
	nodeID := mustGetFlag(cmd, "node")
	if nodeID == "" {
		return fmt.Errorf("flag --node is required")
	}
	outputPath, _ := cmd.Flags().GetString("output")

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "导出钉钉表格为 xlsx")
		deps.Out.PrintKeyValue("节点", nodeID)
		if outputPath != "" {
			deps.Out.PrintKeyValue("输出", outputPath)
		}
		return nil
	}

	ctx := cmd.Context()
	// Cobra injects a context during normal Execute/ExecuteContext. Keep direct
	// handler callers (including embedders and focused unit tests) safe as well.
	if ctx == nil {
		ctx = context.Background()
	}
	asyncMode, _ := cmd.Flags().GetBool("async")

	// json 模式下进度提示会污染 stdout（PrintInfo/PrintKeyValue 都写 stdout），
	// 使得 agent 无法按 JSON 解析。故 json 模式抑制进度、末尾统一输出结果 JSON。
	jsonMode := deps.Caller.Format() == "json"

	// Step 1: submit export job
	if !jsonMode {
		deps.Out.PrintInfo("[1/3] 提交表格导出任务 (xlsx)...")
	}
	submitText, err := callMCPToolReturnText(ctx, "submit_export_job", map[string]any{
		"nodeId":       nodeID,
		"exportFormat": "xlsx",
	})
	if err != nil {
		return fmt.Errorf("提交导出任务失败: %w", err)
	}
	jobID, err := parseExportSubmitResult(submitText)
	if err != nil {
		return err
	}
	if asyncMode {
		return deps.Out.PrintJSON(asynctask.TaskResult{
			ID:      jobID,
			Type:    "export",
			Status:  asynctask.StatusPending,
			Message: "任务已提交，请稍后查询",
		})
	}
	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("导出任务已提交: jobId=%s", jobID))
		// Step 2: progressive backoff polling
		deps.Out.PrintInfo("[2/3] 轮询任务状态（渐进式退避，最多 30 次约 5 分钟）...")
	}
	downloadURL, err := pollSheetExportJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("表格导出任务未完成 (taskId=%s): %w", jobID, err)
	}

	// No output path: print the downloadUrl and exit
	if outputPath == "" {
		if jsonMode {
			return deps.Out.PrintJSON(map[string]any{
				"success":     true,
				"jobId":       jobID,
				"downloadUrl": downloadURL,
			})
		}
		deps.Out.PrintKeyValue("jobId", jobID)
		deps.Out.PrintKeyValue("downloadUrl", downloadURL)
		deps.Out.PrintInfo("导出完成。downloadUrl 具有时效性，请尽快下载。")
		return nil
	}

	// Step 3: download to local file
	// If outputPath is an existing directory, append inferred filename.
	if fi, statErr := os.Stat(outputPath); statErr == nil && fi.IsDir() {
		filename := inferSheetExportFilename(downloadURL)
		if filename == "" {
			filename = fmt.Sprintf("sheet-export-%s.xlsx", jobID)
		}
		outputPath = filepath.Join(outputPath, filename)
	}

	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("[3/3] 下载 xlsx 到 %s ...", outputPath))
	}
	if err := httpGetFile(ctx, downloadURL, map[string]string{}, outputPath); err != nil {
		return fmt.Errorf("下载 xlsx 失败 (taskId=%s): %w", jobID, err)
	}
	if jsonMode {
		return deps.Out.PrintJSON(map[string]any{
			"success":     true,
			"jobId":       jobID,
			"outputPath":  outputPath,
			"downloadUrl": downloadURL,
		})
	}
	deps.Out.PrintInfo(fmt.Sprintf("导出完成: %s", outputPath))
	return nil
}

// parseExportSubmitResult extracts jobId from submit_export_job MCP response.
func parseExportSubmitResult(text string) (string, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "", fmt.Errorf("解析 submit_export_job 响应失败: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	if success, ok := data["success"].(bool); ok && !success {
		msg, _ := data["message"].(string)
		if msg == "" {
			msg = "提交导出任务失败"
		}
		return "", fmt.Errorf("%s", msg)
	}
	jobID, _ := data["jobId"].(string)
	if jobID == "" {
		return "", fmt.Errorf("submit_export_job 未返回 jobId，响应: %s", text)
	}
	return jobID, nil
}

// exportPollIntervals returns the progressive backoff schedule defined in the
// sheet export MCP tool spec: 1~5:2s, 6~10:5s, 11~20:10s, 21~30:15s.
func exportPollIntervals() []time.Duration {
	intervals := make([]time.Duration, 0, 30)
	for i := 0; i < 5; i++ {
		intervals = append(intervals, 2*time.Second)
	}
	for i := 0; i < 5; i++ {
		intervals = append(intervals, 5*time.Second)
	}
	for i := 0; i < 10; i++ {
		intervals = append(intervals, 10*time.Second)
	}
	for i := 0; i < 10; i++ {
		intervals = append(intervals, 15*time.Second)
	}
	return intervals
}

// pollExportJob polls query_export_job per the progressive backoff schedule
// until the job completes successfully, fails, or the 30-attempt cap is hit.
func pollSheetExportJob(ctx context.Context, jobID string) (string, error) {
	// json 模式下轮询进度也要抑制，否则 [INFO] 行会混进 stdout 破坏纯 JSON 输出。
	quiet := deps.Caller.Format() == "json"
	intervals := exportPollIntervals()
	for i, wait := range intervals {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-helperAfter(wait):
		}

		text, err := callMCPToolReturnText(ctx, "query_export_job", map[string]any{
			"jobId": jobID,
		})
		if err != nil {
			if !quiet {
				deps.Out.PrintInfo(fmt.Sprintf("  [%d/30] 查询失败，将继续轮询: %v", i+1, err))
			}
			continue
		}

		status, downloadURL, message, parseErr := parseExportQueryResult(text)
		if parseErr != nil {
			return "", parseErr
		}

		// 服务端可能返回 SUCCESS / success / Success 等不同大小写，统一归一化后再比较。
		normStatus := strings.ToUpper(strings.TrimSpace(status))
		switch normStatus {
		case "SUCCESS":
			if !quiet {
				deps.Out.PrintInfo(fmt.Sprintf("  [%d/30] 状态: SUCCESS", i+1))
			}
			if downloadURL == "" {
				return "", fmt.Errorf("任务成功但未返回 downloadUrl")
			}
			return downloadURL, nil
		case "FAILED", "FAIL", "ERROR":
			if message == "" {
				message = "导出任务失败"
			}
			return "", fmt.Errorf("%s", message)
		case "PROCESSING", "RUNNING", "DOING", "PENDING", "":
			if !quiet {
				deps.Out.PrintInfo(fmt.Sprintf("  [%d/30] 状态: PROCESSING", i+1))
			}
		default:
			if !quiet {
				deps.Out.PrintInfo(fmt.Sprintf("  [%d/30] 状态: %s", i+1, status))
			}
		}
	}
	return "", fmt.Errorf("导出任务超时：已轮询 30 次（约 5 分钟）仍未完成，请稍后再试")
}

// parseExportQueryResult extracts status/downloadUrl/message from query_export_job.
func parseExportQueryResult(text string) (status, downloadURL, message string, err error) {
	var data map[string]any
	if e := json.Unmarshal([]byte(text), &data); e != nil {
		err = fmt.Errorf("解析 query_export_job 响应失败: %w", e)
		return
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}
	status, _ = data["status"].(string)
	downloadURL, _ = data["downloadUrl"].(string)
	message, _ = data["message"].(string)
	return
}

// inferSheetExportFilename extracts a safe local filename from a sheet-export download URL.
func inferSheetExportFilename(rawURL string) string {
	name := ""
	if idx := strings.LastIndex(rawURL, "/"); idx >= 0 && idx < len(rawURL)-1 {
		name = rawURL[idx+1:]
		if qIdx := strings.Index(name, "?"); qIdx >= 0 {
			name = name[:qIdx]
		}
	}
	if name == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(name); err == nil && decoded != "" {
		name = decoded
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return name
}

// ── export 命令定义 ──────────────────────────────────────────────────────────

func newExportCmd() *cobra.Command {
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出表格为 xlsx（同步轮询或异步提交）",
		Long: `将钉钉在线电子表格导出为 Office xlsx 格式（单命令一站式）。

默认同步模式会自动完成全部流程（无需 Agent 介入轮询）:
  1. 提交导出任务（submit_export_job），获取 jobId
  2. 按渐进式退避策略轮询任务状态（query_export_job）
       第 1~5 次：每次 2 秒
       第 6~10 次：每次 5 秒
       第 11~20 次：每次 10 秒
       第 21~30 次：每次 15 秒
       硬上限 30 次（约 5 分钟），超时后返回错误
  3. 任务成功后取得 downloadUrl
  4. 若指定了 --output，将 xlsx 下载到本地文件；否则直接输出 downloadUrl

异步模式传入 --async 后，在提交成功后立即输出一个 PENDING TaskResult，
不会轮询或下载。请保存 TaskResult.id，稍后查询：
  dws sheet export get --task-id <TASK_ID>

参数说明:
  --node    表格文档 ID 或链接 URL，系统自动识别（必填）
  --output  本地保存路径（可选）。可为文件路径或目录：
            - 文件路径：如 ./a.xlsx，直接按此路径保存
            - 目录路径：如 ./，自动从下载链接推断文件名
            - 未指定：仅返回 downloadUrl，链接有时效性请尽快下载

支持范围:
  仅支持钉钉在线电子表格（axls）→ xlsx；
  若需导出钉钉文字文档，请使用 dingtalkdoc 侧的导出工具。

权限要求:
  当前用户对目标表格具备可查看/下载权限。`,
		Example: `  # 仅导出，返回 downloadUrl（链接有时效性，请尽快下载）
  dws sheet export --node NODE_ID

  # 导出并自动下载为本地文件
  dws sheet export --node NODE_ID --output ./report.xlsx

  # --output 为目录时，自动按下载链接里的文件名保存
  dws sheet export --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --output ./

  # 异步提交并立即返回 TaskResult.id
  dws sheet export --node NODE_ID --async`,
		RunE: runSheetExport,
	}
	registerSheetExportFlags(exportCmd)

	// 保留历史 runnable parent，并通过 create compatibility leaf 暴露完全
	// 相同的提交/同步导出动作。
	exportCreateCmd := &cobra.Command{
		Use:     "create",
		Short:   exportCmd.Short,
		Long:    exportCmd.Long,
		Example: strings.ReplaceAll(exportCmd.Example, "dws sheet export --", "dws sheet export create --"),
		RunE:    runSheetExport,
	}
	registerSheetExportFlags(exportCreateCmd)
	RegisterCrossProductAliases(exportCreateCmd)
	cli.AnnotateRuntimeCompatibilityEquivalence(exportCmd, exportCreateCmd, cli.RuntimeCompatibilityEquivalence{
		ID:       "sheet-export-create-v1",
		Reason:   "The create compatibility leaf reuses the historical export handler and exact flag contract while preserving the stable parent Schema identity.",
		Reviewed: true,
	})

	exportGetCmd := &cobra.Command{
		Use:   "get",
		Short: "按任务 ID 查询表格导出任务结果",
		Long: `根据任务 ID 查询表格导出任务的执行结果。
用于查询 --async 返回的任务，或同步导出超时、中断后遗留的任务。

任务状态：
  PENDING     等待处理
  PROCESSING  处理中
  SUCCESS     导出成功，返回 resultUrl
  FAILED      导出失败
  TIMEOUT     查询超时`,
		Example: `  dws sheet export get --task-id <TASK_ID>`,
		RunE:    runSheetExportGet,
	}
	exportGetCmd.Flags().String("task-id", "", "导出任务 ID (必填)")
	exportGetCmd.Flags().String("job-id", "", "--task-id 的兼容别名")
	_ = exportGetCmd.Flags().MarkHidden("job-id")

	exportCmd.AddCommand(exportCreateCmd, exportGetCmd)
	return exportCmd
}

func registerSheetExportFlags(cmd *cobra.Command) {
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("output", "", "本地保存路径（可选，支持文件路径或目录）")
	cmd.Flags().Bool("async", false, "异步模式：提交导出任务后立即返回 TaskResult.id，不等待完成")
}

func runSheetExportGet(cmd *cobra.Command, _ []string) error {
	taskID, err := taskIDFromFlags(cmd)
	if err != nil {
		return err
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "查询表格导出任务结果")
		deps.Out.PrintKeyValue("任务ID", taskID)
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := queryAsyncTask(ctx, "export", taskID)
	if err != nil {
		return err
	}
	if err := deps.Out.PrintJSON(result); err != nil {
		return err
	}
	switch result.Status {
	case asynctask.StatusFailed:
		if result.Message != "" {
			return fmt.Errorf("表格导出任务失败 (status=%s): %s", result.Status, result.Message)
		}
		return fmt.Errorf("表格导出任务失败 (status=%s)", result.Status)
	case asynctask.StatusTimeout:
		if result.Message != "" {
			return fmt.Errorf("表格导出任务超时 (status=%s): %s", result.Status, result.Message)
		}
		return fmt.Errorf("表格导出任务超时 (status=%s)", result.Status)
	}
	return nil
}

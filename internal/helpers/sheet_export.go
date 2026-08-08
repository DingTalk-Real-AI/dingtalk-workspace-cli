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
)

func runSheetExport(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("export-format")
	switch format {
	case "", "xlsx":
		// xlsx 异步流程不消费 csv 专属选择器，静默忽略它们最危险的场景是自动化漏写
		// --export-format csv：用户要的 --range 被丢掉，导出的却是整篇工作簿，而命令
		// 仍报成功。显式传了就直接拒。
		if used := changedCsvOnlyExportFlags(cmd); len(used) > 0 {
			return fmt.Errorf("%s 仅在 --export-format csv 下生效，xlsx 导出会忽略它们（会导出整篇工作簿）；"+
				"请补上 --export-format csv，或去掉这些参数", strings.Join(used, " / "))
		}
	case "csv":
		return runSheetExportCsv(cmd)
	default:
		return fmt.Errorf("--export-format 仅支持 xlsx 或 csv，当前值: %s", format)
	}

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

	ctx := context.Background()

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
	if !jsonMode {
		deps.Out.PrintInfo(fmt.Sprintf("导出任务已提交: jobId=%s", jobID))
		// Step 2: progressive backoff polling
		deps.Out.PrintInfo("[2/3] 轮询任务状态（渐进式退避，最多 30 次约 5 分钟）...")
	}
	downloadURL, err := pollSheetExportJob(ctx, jobID)
	if err != nil {
		return err
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
		return fmt.Errorf("下载 xlsx 失败: %w", err)
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
		Short: "导出表格为 xlsx（异步一站式）或 CSV（单表同步）",
		Long: `将钉钉在线电子表格导出为 Office xlsx 或 CSV。

格式（--export-format）:
  xlsx（默认）  整篇表格导出为 xlsx，异步任务一站式（提交+轮询+下载，见下方流程）
  csv           导出单个工作表为纯 RFC4180 CSV（同步）。用 --sheet-id 指定工作表、
                --range 限定范围、--value-render-option 选择取值模式；--output 落盘，
                不传则打印到 stdout。超大表会截断并给出警告，请用 --range 分块或改用 xlsx。

以下为 xlsx 异步流程：

执行流程（全程自动，无需 Agent 介入轮询）:
  1. 提交导出任务（submit_export_job），获取 jobId
  2. 按渐进式退避策略轮询任务状态（query_export_job）
       第 1~5 次：每次 2 秒
       第 6~10 次：每次 5 秒
       第 11~20 次：每次 10 秒
       第 21~30 次：每次 15 秒
       硬上限 30 次（约 5 分钟），超时后返回错误
  3. 任务成功后取得 downloadUrl
  4. 若指定了 --output，将 xlsx 下载到本地文件；否则直接输出 downloadUrl

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

  # 导出单个工作表为 CSV 文件
  dws sheet export --node NODE_ID --export-format csv --sheet-id SHEET_ID --output ./data.csv

  # 导出 CSV 到 stdout（可管道处理）
  dws sheet export --node NODE_ID --export-format csv --sheet-id SHEET_ID`,
		RunE: runSheetExport,
	}
	exportCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	exportCmd.Flags().String("output", "", "本地保存路径（可选，支持文件路径或目录）")
	// 注意：不要把这个 flag 叫 --format。根级已有 persistent 的 -f/--format（输出格式），
	// 同名局部 flag 会在 cobra 的 flag 合并中把它整个挤掉，导致 -f 变成未知简写、
	// 且 --format 被这里吞掉（命名与 aitable export-data 的 --export-format 一致）。
	exportCmd.Flags().String("export-format", "xlsx", "导出格式: xlsx(默认,异步任务) / csv(单个工作表,同步)")
	exportCmd.Flags().String("sheet-id", "", "工作表 ID 或名称（--export-format csv 时指定要导出的工作表，不传则第一个）")
	exportCmd.Flags().String("range", "", "导出范围，A1 表示法（仅 --export-format csv，不传则整表；大表可用此分块导出）")
	exportCmd.Flags().String("value-render-option", "", "取值模式（仅 --export-format csv）: formatted_value(默认) / raw_value / formula")
	exportCmd.Flags().Bool("allow-truncated", false, "允许 CSV 被截断时仍然导出（仅 --export-format csv）。默认截断即报错并不写文件，避免不完整数据被当成完整导出")
	return exportCmd
}

// valueRenderOptionEnum 是 --value-render-option 的合法取值（仅 csv 路径生效）。
var valueRenderOptionEnum = map[string]bool{
	"formatted_value": true, "raw_value": true, "formula": true,
}

// csvOnlyExportFlags 是只有 --export-format csv 分支会读取的 flag。
// 新增 csv 专属 flag 必须同步登记（TestCsvOnlyExportFlagsMatchBoundFlags 会盯住漂移）。
var csvOnlyExportFlags = []string{"sheet-id", "range", "value-render-option", "allow-truncated"}

// changedCsvOnlyExportFlags 返回用户显式设置过的 csv 专属 flag（带 -- 前缀，顺序稳定）。
func changedCsvOnlyExportFlags(cmd *cobra.Command) []string {
	var out []string
	for _, name := range csvOnlyExportFlags {
		if cmd.Flags().Changed(name) {
			out = append(out, "--"+name)
		}
	}
	return out
}

// runSheetExportCsv 导出单个工作表为纯 CSV（同步，复用 get_range_as_csv，annotateRowNumbers=false）。
func runSheetExportCsv(cmd *cobra.Command) error {
	nodeID := mustGetFlag(cmd, "node")
	if nodeID == "" {
		return fmt.Errorf("flag --node is required")
	}
	sheetID, _ := cmd.Flags().GetString("sheet-id")
	rangeAddr, _ := cmd.Flags().GetString("range")
	valueRenderOption, _ := cmd.Flags().GetString("value-render-option")
	valueRenderOption = strings.ToLower(strings.TrimSpace(valueRenderOption))
	if valueRenderOption != "" && !valueRenderOptionEnum[valueRenderOption] {
		return fmt.Errorf("--value-render-option 必须为 formatted_value / raw_value / formula，当前值: %s", valueRenderOption)
	}
	outputPath, _ := cmd.Flags().GetString("output")
	allowTruncated, _ := cmd.Flags().GetBool("allow-truncated")

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "导出工作表为 CSV")
		deps.Out.PrintKeyValue("节点", nodeID)
		if sheetID != "" {
			deps.Out.PrintKeyValue("工作表", sheetID)
		}
		if outputPath != "" {
			deps.Out.PrintKeyValue("输出", outputPath)
		}
		return nil
	}

	ctx := context.Background()
	toolArgs := map[string]any{
		"nodeId":             nodeID,
		"annotateRowNumbers": false,
	}
	if sheetID != "" {
		toolArgs["sheetId"] = sheetID
	}
	if rangeAddr != "" {
		toolArgs["range"] = rangeAddr
	}
	if valueRenderOption != "" {
		toolArgs["valueRenderOption"] = valueRenderOption
	}

	// CSV 正文走 stdout，进度/警告一律不能污染它。
	text, err := callMCPToolReturnText(ctx, "get_range_as_csv", toolArgs)
	if err != nil {
		return fmt.Errorf("读取 CSV 失败: %w", err)
	}

	csvContent, hasMore, err := parseGetRangeAsCsvResult(text)
	if err != nil {
		return err
	}

	// 截断必须 fail-closed：只打 stderr 警告然后照常落盘 + 报"导出完成" + 退出码 0，
	// 会让自动化调用方（和没留意 stderr 的人）把不完整文件当成完整导出，且若目标
	// 文件已存在还会被截断数据覆盖。默认在写文件/输出之前就失败，要接受不完整结果
	// 必须显式加 --allow-truncated。
	if hasMore && !allowTruncated {
		return fmt.Errorf("表格数据超出单次读取上限，CSV 会被截断，已中止导出（未写入 %s）；"+
			"请用 --range 分块导出（如 --range A1:Z1000、A1001:Z2000 ...）、改用 --export-format xlsx 导出完整表格，"+
			"或确认可接受不完整数据后加 --allow-truncated",
			firstNonEmpty(outputPath, "stdout"))
	}
	if hasMore {
		deps.Out.PrintWarning("表格数据超出单次读取上限，CSV 已被截断（--allow-truncated 已显式放行）。" +
			"请用 --range 分块导出（如 --range A1:Z1000、A1001:Z2000 ...），或改用 --export-format xlsx 导出完整表格。")
	}

	if outputPath == "" {
		deps.Out.PrintRaw(csvContent)
		return nil
	}
	if fi, statErr := os.Stat(outputPath); statErr == nil && fi.IsDir() {
		outputPath = filepath.Join(outputPath, "sheet-export.csv")
	}
	// 必须原子替换：os.WriteFile 会先把已存在的 CSV 截断，写入中途失败（磁盘满、
	// 配额、I/O 错误）就把用户的原文件毁掉了。AtomicWrite 写同目录临时文件再 rename，
	// 失败时原文件保持不变。
	// AtomicWrite 会 MkdirAll 父目录，先探一次，保持与 xlsx 分支一致的「父目录不存在
	// 即报错」语义，避免把拼错的路径悄悄建成目录。
	if _, statErr := os.Stat(filepath.Dir(outputPath)); statErr != nil {
		return fmt.Errorf("写入 CSV 文件失败: %w", statErr)
	}
	if err := AtomicWrite(outputPath, []byte(csvContent), 0o644); err != nil {
		return fmt.Errorf("写入 CSV 文件失败: %w", err)
	}
	if hasMore {
		deps.Out.PrintInfo(fmt.Sprintf("导出完成（数据已截断，不是完整表格）: %s", outputPath))
		return nil
	}
	deps.Out.PrintInfo(fmt.Sprintf("导出完成: %s", outputPath))
	return nil
}

// parseGetRangeAsCsvResult 从 get_range_as_csv 的 MCP 响应中提取 csv 文本与 hasMore 标志。
//
// csv 字段缺失或类型不对，必须报错而不是当成空表：调用方会把空内容写进
// --output，用 0 字节覆盖已有文件并打印"导出完成"，等于静默数据丢失。
// 「字段存在且为空串」是合法的（真的空区域），与「字段缺失」区分开。
func parseGetRangeAsCsvResult(text string) (csv string, hasMore bool, err error) {
	var data map[string]any
	if e := json.Unmarshal([]byte(text), &data); e != nil {
		return "", false, fmt.Errorf("解析 get_range_as_csv 响应失败: %w", e)
	}
	if raw, wrapped := data["result"]; wrapped {
		result, ok := raw.(map[string]any)
		if !ok {
			return "", false, fmt.Errorf("解析 get_range_as_csv 响应失败: result 不是对象，响应: %s", text)
		}
		data = result
	}
	raw, exists := data["csv"]
	if !exists {
		return "", false, fmt.Errorf("解析 get_range_as_csv 响应失败: 缺少 csv 字段，响应: %s", text)
	}
	csvVal, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("解析 get_range_as_csv 响应失败: csv 字段不是字符串（%T），响应: %s", raw, text)
	}
	csv = csvVal
	if hm, ok := data["hasMore"].(bool); ok {
		hasMore = hm
	}
	return csv, hasMore, nil
}

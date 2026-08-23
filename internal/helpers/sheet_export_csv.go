package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// ============================================================================
// sheet export-csv：把单个工作表同步导出为纯 RFC4180 CSV
//
// 独立成一条命令而不是给 sheet export 加 --export-format csv：sheet export 的叶子
// 契约是 interface_mode=mcp + interface_ref=submit_export_job（xlsx 异步任务）。
// csv 是互斥分支，走的是 get_range_as_csv，submit_export_job 根本不执行；挂在同一
// 条叶子上会让 Agent 的接口审计、参数映射和后端能力判断在 csv 场景下全都拿到错误
// 信息。本叶子如实声明 interface_ref=get_range_as_csv，sheet export 则保持原样。
//
// 与 sheet csv-get 的分工：csv-get 面向 Agent 阅读，输出带 [row=N] 行号前缀并按
// -f 渲染，只回 stdout；本命令面向落盘，输出纯 CSV、可写文件，且截断时 fail-closed。
// ============================================================================

func newSheetExportCsvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export-csv",
		Short: "导出单个工作表为纯 CSV（同步）",
		Long: `将钉钉在线电子表格的单个工作表同步导出为纯 RFC4180 CSV。

整篇表格导出为 xlsx 请用 dws sheet export（异步任务：提交→轮询→可选下载）。

参数说明:
  --node                  表格文档 ID 或链接 URL，系统自动识别（必填）
  --sheet-id              要导出的工作表 ID 或名称（不传则第一个工作表）
  --range                 导出范围，A1 表示法（不传则整表；大表可用此分块导出）
  --value-render-option   取值模式: formatted_value(默认) / raw_value / formula
  --output                本地保存路径（可选）。可为文件路径或目录（目录时存为
                          sheet-export.csv）；不传则把 CSV 打到 stdout
  --allow-truncated       允许数据被截断时仍然导出

截断行为:
  表格超出单次读取上限时默认直接报错并且不写文件，避免不完整数据被当成完整导出、
  或覆盖掉已有的完整文件。要接受不完整结果须显式加 --allow-truncated。

落盘保证:
  写文件走同目录临时文件 + rename 原子替换，写入中途失败（磁盘满、配额、I/O 错误）
  不会破坏已存在的目标文件。父目录不存在时报错，不会替你创建。

权限要求:
  当前用户对目标表格具备可查看权限。`,
		Example: `  # 导出第一个工作表到 stdout（可管道处理）
  dws sheet export-csv --node NODE_ID

  # 导出指定工作表为本地 CSV 文件
  dws sheet export-csv --node NODE_ID --sheet-id SHEET_ID --output ./data.csv

  # 只导出某个范围，取原始值
  dws sheet export-csv --node NODE_ID --range A1:Z1000 --value-render-option raw_value`,
		RunE: runSheetExportCsv,
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "export_csv",
				CanonicalPath:  "sheet.export_csv",
				CLIPath:        "sheet export-csv",
				PrimaryCLIPath: "sheet export-csv",
			},
			Description: "同步导出单个工作表为纯 RFC4180 CSV（可落盘，截断即报错）。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "get_range_as_csv"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "同步导出单个工作表为纯 RFC4180 CSV（可落盘，截断即报错）。",
				UseWhen:      []string{"需要单个工作表的纯 CSV（写本地文件或管道处理）时"},
				AvoidWhen:    []string{"要整篇表格的 xlsx 用 sheet export；只是让 Agent 读内容用 sheet csv-get（带 [row=N] 行号）"},
				Examples: []string{
					"dws sheet export-csv --node <NODE_ID> --sheet-id <SHEET_ID> --output ./data.csv",
					"dws sheet export-csv --node <NODE_ID> --range A1:Z1000",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
				{Name: "range", Property: "range"},
				{Name: "sheet-id", Property: "sheetId"},
				{Name: "value-render-option", Property: "valueRenderOption"},
			},
		},
	})
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称（不传则第一个工作表）")
	cmd.Flags().String("range", "", "导出范围，A1 表示法（不传则整表；大表可用此分块导出）")
	cmd.Flags().String("value-render-option", "", "取值模式: formatted_value(默认) / raw_value / formula")
	cmd.Flags().String("output", "", "本地保存路径（可选，支持文件路径或目录）；不传则输出到 stdout")
	cmd.Flags().Bool("allow-truncated", false, "允许数据被截断时仍然导出。默认截断即报错并不写文件，避免不完整数据被当成完整导出")
	return cmd
}

// valueRenderOptionEnum 是 --value-render-option 的合法取值。
var valueRenderOptionEnum = map[string]bool{
	"formatted_value": true, "raw_value": true, "formula": true,
}

// runSheetExportCsv 导出单个工作表为纯 CSV（同步，复用 get_range_as_csv，annotateRowNumbers=false）。
func runSheetExportCsv(cmd *cobra.Command, _ []string) error {
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
			"请用 --range 分块导出（如 --range A1:Z1000、A1001:Z2000 ...）、改用 dws sheet export 导出完整表格的 xlsx，"+
			"或确认可接受不完整数据后加 --allow-truncated",
			firstNonEmpty(outputPath, "stdout"))
	}
	if hasMore {
		deps.Out.PrintWarning("表格数据超出单次读取上限，CSV 已被截断（--allow-truncated 已显式放行）。" +
			"请用 --range 分块导出（如 --range A1:Z1000、A1001:Z2000 ...），或改用 dws sheet export 导出完整表格的 xlsx。")
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
	// AtomicWrite 会 MkdirAll 父目录，先探一次，保持与 sheet export 一致的「父目录
	// 不存在即报错」语义，避免把拼错的路径悄悄建成目录。
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

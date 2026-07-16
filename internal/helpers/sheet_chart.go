package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ─── Chart properties validation ─────────────────────────────────────────────

// validChartTypes enumerates all MCP-supported chart type values (lowercase).
var validChartTypes = map[string]bool{
	"column": true, "bar": true, "columnstacked": true, "barstacked": true,
	"line": true, "linestacked": true,
	"area": true, "areastacked": true, "areapercentstacked": true,
	"pie": true, "doughnut": true,
	"scatter": true, "radar": true,
}

// validateChartProperties performs basic structural validation on the --properties JSON
// before sending to MCP. It checks required fields and types without doing any conversion.
func validateChartProperties(props map[string]any) error {
	// position (required)
	posRaw, exists := props["position"]
	if !exists || posRaw == nil {
		return fmt.Errorf("--properties 缺少必填字段 position")
	}
	posObj, ok := posRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("position 必须为对象")
	}
	if _, exists := posObj["row"]; !exists {
		return fmt.Errorf("position.row 为必填字段")
	}
	if _, exists := posObj["col"]; !exists {
		return fmt.Errorf("position.col 为必填字段")
	}

	// dimensions (required)
	dimRaw, exists := props["dimensions"]
	if !exists || dimRaw == nil {
		return fmt.Errorf("--properties 缺少必填字段 dimensions")
	}
	dimObj, ok := dimRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("dimensions 必须为对象")
	}
	if w, exists := dimObj["width"]; !exists {
		return fmt.Errorf("dimensions.width 为必填字段")
	} else if wv, ok := w.(float64); ok && wv <= 0 {
		return fmt.Errorf("dimensions.width 必须为正数，当前值: %v", wv)
	}
	if h, exists := dimObj["height"]; !exists {
		return fmt.Errorf("dimensions.height 为必填字段")
	} else if hv, ok := h.(float64); ok && hv <= 0 {
		return fmt.Errorf("dimensions.height 必须为正数，当前值: %v", hv)
	}

	// chart (required)
	chartRaw, exists := props["chart"]
	if !exists || chartRaw == nil {
		return fmt.Errorf("--properties 缺少必填字段 chart")
	}
	chartObj, ok := chartRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("chart 必须为对象")
	}

	// chart.type (required, must be valid enum)
	typeRaw, exists := chartObj["type"]
	if !exists || typeRaw == nil {
		return fmt.Errorf("chart.type 为必填字段")
	}
	typeStr, ok := typeRaw.(string)
	if !ok {
		return fmt.Errorf("chart.type 必须为字符串")
	}
	if !validChartTypes[strings.ToLower(strings.TrimSpace(typeStr))] {
		return fmt.Errorf("未知的图表类型: %q，可用类型: column/bar/columnStacked/barStacked/line/lineStacked/area/areaStacked/areaPercentStacked/pie/doughnut/scatter/radar", typeStr)
	}

	// chart.series (required, must be non-empty array)
	seriesRaw, exists := chartObj["series"]
	if !exists || seriesRaw == nil {
		return fmt.Errorf("chart.series 为必填字段")
	}
	seriesArr, ok := seriesRaw.([]any)
	if !ok {
		return fmt.Errorf("chart.series 必须为数组")
	}
	if len(seriesArr) == 0 {
		return fmt.Errorf("chart.series 必须至少包含一项")
	}
	// Validate each series item has value field
	for i, item := range seriesArr {
		obj, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("chart.series[%d] 必须为对象", i)
		}
		valueRaw, exists := obj["value"]
		if !exists || valueRaw == nil {
			return fmt.Errorf("chart.series[%d].value 为必填字段", i)
		}
		valueArr, ok := valueRaw.([]any)
		if !ok {
			return fmt.Errorf("chart.series[%d].value 必须为数组", i)
		}
		if len(valueArr) == 0 {
			return fmt.Errorf("chart.series[%d].value 不能为空", i)
		}
	}

	return nil
}

// ─── Chart subcommands ──────────────────────────────────────────────────────

func newChartCmd() *cobra.Command {
	chartCmd := &cobra.Command{Use: "chart", Short: "浮动图表管理"}

	// ── chart list ──────────────────────────────────────────────────────
	chartListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取浮动图表",
		Long: `获取钉钉电子表格指定工作表中的浮动图表。

支持两种模式：
1. 列出全部图表：不传 --chart-id，返回该工作表中所有浮动图表的列表。
2. 获取单个图表：传入 --chart-id，返回该图表的详细信息。

返回内容包含每个图表的 ID、锚点位置、尺寸坐标以及完整图表配置（类型、数据系列、分类轴、标题、图例、坐标轴等）。`,
		Example: `  # 列出所有浮动图表
  dws sheet chart list --node NODE_ID --sheet-id SHEET_ID

  # 获取单个图表详情
  dws sheet chart list --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			}
			if v, _ := cmd.Flags().GetString("chart-id"); v != "" {
				toolArgs["floatChartId"] = v
			}
			return callMCPTool("list_float_charts", toolArgs)
		},
	}
	chartListCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	chartListCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	chartListCmd.Flags().String("chart-id", "", "浮动图表 ID (可选，不传则返回全部)")

	// ── chart create ────────────────────────────────────────────────────
	chartCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建浮动图表",
		Long: `在钉钉电子表格的指定工作表中创建一个浮动图表。

--properties 为 JSON 格式，包含 position（锚点位置）、dimensions（尺寸）、chart（图表配置）三个必填字段，
以及可选的 offset（偏移量）。支持 stdin heredoc 和 @file 两种传入方式。

chart.type 支持: column（柱形图）、bar（条形图）、columnStacked（堆积柱形图）、barStacked（堆积条形图）、
line（折线图）、lineStacked（堆积折线图）、area（面积图）、areaStacked（堆积面积图）、
areaPercentStacked（百分比堆积面积图）、pie（饼图）、doughnut（环形图）、scatter（散点图）、
radar（雷达图）。
chart.series[].value / chart.series[].name / chart.category 使用 A1 区域表示法（如 "A2:B10"），
不指定 sheet 前缀时默认使用 --sheet-id 的值。
position.col 使用列字母表示法（如 "A"、"AA"），不支持数字形式。

创建成功后返回新图表的完整信息，包含系统分配的 chart-id。`,
		Example: `  # 创建柱形图
  dws sheet chart create --node NODE_ID --sheet-id SHEET_ID --properties '{
    "position": {"row": 12, "col": "A"},
    "dimensions": {"width": 600, "height": 400},
    "chart": {
      "type": "column",
      "series": [{"name": "B1", "value": ["B2:B10"]}],
      "category": ["A2:A10"],
      "title": {"show": true, "text": "销售数据"}
    }
  }'

  # 创建折线图（通过文件传入）
  dws sheet chart create --node NODE_ID --sheet-id SHEET_ID --properties @chart.json

  # 创建饼图
  dws sheet chart create --node NODE_ID --sheet-id SHEET_ID --properties '{
    "position": {"row": 0, "col": "F"},
    "dimensions": {"width": 500, "height": 400},
    "chart": {
      "type": "pie",
      "series": [{"value": ["B2:B6"]}],
      "category": ["A2:A6"],
      "title": {"show": true, "text": "各部门占比"}
    }
  }'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			propsStr := mustGetFlag(cmd, "properties")
			if propsStr == "" {
				return fmt.Errorf("--properties 为必填参数")
			}
			if strings.HasPrefix(propsStr, "@") {
				data, err := os.ReadFile(strings.TrimPrefix(propsStr, "@"))
				if err != nil {
					return fmt.Errorf("读取文件失败: %w", err)
				}
				propsStr = string(data)
			}
			var props map[string]any
			if err := json.Unmarshal([]byte(propsStr), &props); err != nil {
				return fmt.Errorf("--properties JSON 解析失败: %w", err)
			}
			if err := validateChartProperties(props); err != nil {
				return err
			}

			return callMCPTool("create_float_chart", map[string]any{
				"nodeId":     mustGetFlag(cmd, "node"),
				"sheetId":    mustGetFlag(cmd, "sheet-id"),
				"properties": props,
			})
		},
	}
	chartCreateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	chartCreateCmd.Flags().String("sheet-id", "", "工作表 ID (必填)")
	chartCreateCmd.Flags().String("properties", "", "图表完整配置 JSON (必填，含 position/dimensions/chart)")

	// ── chart update ────────────────────────────────────────────────────
	chartUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新浮动图表",
		Long: `更新钉钉电子表格中指定浮动图表的配置。

注意：本接口采用 PUT 语义（完整覆盖），非增量更新。
建议先通过 chart list（传入 --chart-id）获取当前配置，修改后整体传入。
如果只需修改部分字段（如仅修改标题），仍需传入完整的 chart 配置以避免其他字段被清空。

--properties 结构与 chart create 完全相同。`,
		Example: `  # 先获取现有配置
  dws sheet chart list --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID

  # 修改后整体回写
  dws sheet chart update --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID --properties '{
    "position": {"row": 12, "col": "A"},
    "dimensions": {"width": 800, "height": 500},
    "chart": {
      "type": "line",
      "series": [{"name": "B1", "value": ["B2:B20"]}],
      "category": ["A2:A20"],
      "title": {"show": true, "text": "月度趋势（更新）"}
    }
  }'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			propsStr := mustGetFlag(cmd, "properties")
			if propsStr == "" {
				return fmt.Errorf("--properties 为必填参数")
			}
			if strings.HasPrefix(propsStr, "@") {
				data, err := os.ReadFile(strings.TrimPrefix(propsStr, "@"))
				if err != nil {
					return fmt.Errorf("读取文件失败: %w", err)
				}
				propsStr = string(data)
			}
			var props map[string]any
			if err := json.Unmarshal([]byte(propsStr), &props); err != nil {
				return fmt.Errorf("--properties JSON 解析失败: %w", err)
			}
			if err := validateChartProperties(props); err != nil {
				return err
			}

			return callMCPTool("update_float_chart", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"floatChartId": mustGetFlag(cmd, "chart-id"),
				"properties":   props,
			})
		},
	}
	chartUpdateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	chartUpdateCmd.Flags().String("sheet-id", "", "工作表 ID (必填)")
	chartUpdateCmd.Flags().String("chart-id", "", "浮动图表 ID (必填，可通过 chart list 获取)")
	chartUpdateCmd.Flags().String("properties", "", "图表完整配置 JSON (必填，PUT 语义整体覆盖)")

	// ── chart delete ────────────────────────────────────────────────────
	chartDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除浮动图表",
		Long: `删除钉钉电子表格中指定的浮动图表。

注意: 这是一个危险操作，删除后图表不可恢复。执行前需要确认，或传入 --yes 跳过确认。
chart-id 可通过 chart list 获取。`,
		Example: `  dws sheet chart delete --node NODE_ID --sheet-id SHEET_ID --chart-id CHART_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			chartID := mustGetFlag(cmd, "chart-id")
			return callMCPTool("delete_float_chart", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"floatChartId": chartID,
			})
		},
	}
	chartDeleteCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	chartDeleteCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	chartDeleteCmd.Flags().String("chart-id", "", "浮动图表 ID (必填，可通过 chart list 获取)")

	chartCmd.AddCommand(chartListCmd, chartCreateCmd, chartUpdateCmd, chartDeleteCmd)
	return chartCmd
}

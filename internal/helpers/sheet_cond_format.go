package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newCondFormatCmd() *cobra.Command {
	condFormatCmd := &cobra.Command{Use: "cond-format", Short: "条件格式管理"}

	condFormatListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取条件格式规则",
		Long: `获取钉钉电子表格中指定工作表的所有条件格式规则，或通过 --rule-id 获取单个规则的详情。

条件格式规则定义了当单元格满足特定条件时自动应用的样式（如背景色、字体颜色等）。
返回的每条规则包含：规则 ID、应用范围、条件类型及参数、命中时的单元格样式。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。`,
		Example: `  # 获取所有条件格式规则
  dws sheet cond-format list --node NODE_ID --sheet-id SHEET_ID

  # 获取单个规则的详情
  dws sheet cond-format list --node NODE_ID --sheet-id SHEET_ID --rule-id RULE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			}
			if v, _ := cmd.Flags().GetString("rule-id"); v != "" {
				toolArgs["ruleId"] = v
			}
			return callMCPTool("get_cond_format", toolArgs)
		},
	}
	condFormatListCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	condFormatListCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	condFormatListCmd.Flags().String("rule-id", "", "条件格式规则 ID (可选，不传则返回全部)")

	condFormatCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建条件格式规则",
		Long: `在钉钉电子表格的指定工作表中创建一条条件格式规则。

条件格式规则定义了当单元格满足特定条件时自动应用的样式。
每条规则需要指定：应用范围（--ranges）、条件类型及参数（--condition）、命中时的单元格样式（--cell-style）。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。

--ranges 为 JSON 数组，A1 表示法，如 '["A1:E10"]'。
--condition 为 JSON 对象，包含条件类型及参数。每条规则只能选择一种条件类型。
--cell-style 为 JSON 对象，指定条件命中时的样式。
--data-bar-style 为 JSON 对象，仅数据条类型时使用。

支持的条件类型（condition JSON 的 key，每次只能选一种）：
  numberCondition     数值比较
    operator: equal/not-equal/greater/greater-equal/less/less-equal/between/not-between
    value1: 比较值（字符串，必填）
    value2: 第二比较值（字符串，仅 between/not-between 时必填）
  textCondition       文本匹配
    operator: contains/not-contains/starts-with/ends-with
    value: 匹配文本
  emptyCondition      空值判断（operator: is-empty/is-not-empty）
  errorCondition      错误值判断（operator: error/no-error）
  duplicateCondition  重复/唯一值（operator: duplicate/unique）
  formulaCondition    自定义公式（formula: "=A1>100"）
  rankCondition       排名（value: 数量, isPercent: 是否百分比, isBottom: 是否倒数）
  averageCondition    高于/低于平均值（isAbove: 是否高于, andEqual: 是否含等于）
  stdevCondition      标准差（value: 倍数, isAbove: 方向, andEqual: 是否含等于）
  dataBarCondition    数据条
    minPoint/maxPoint: { type: auto/maxmin/number/percent/percentile/formula, value: 端点值 }
  iconSetCondition    图标集
    iconSet: 数组，每项含 criteria({ type, value, gtOrEqual }) 和 icon({ type: "id", value: 图标ID })
    showIconOnly: 是否仅显示图标
  colorScaleCondition 色阶
    criterias: 数组（2或3项），每项含 { type: maxmin/number/percent/percentile/formula, value, color }

--cell-style JSON 字段：
  backgroundColor  背景色（十六进制如 #FFCDD2）
  fontColor        字体颜色（十六进制如 #B71C1C）
  bold             是否加粗（布尔）
  italic           是否斜体（布尔）
  strikethrough    是否删除线（布尔）

--data-bar-style JSON 字段（仅 dataBarCondition 时使用）：
  fill         填充颜色数组 [正数颜色, 负数颜色]，如 ["#4CAF50","#F44336"]
  isGradient   是否渐变填充（布尔，默认 false）

注意事项：
  - 创建后必须用 cond-format list 验证规则是否生效
  - 中文"标红/高亮/染色"默认指 backgroundColor，"字体红"才是 fontColor
  - 日期/空值公式必须防空：=AND(E1<>"", E1<=TODAY()) 而非 =E1<=TODAY()
  - 公式中用相对引用（如 =E1<=TODAY()）使公式随行变化，绝对引用只比较一个格
  - 创建前建议先 range read 读 3-5 行数据确认列对应关系`,
		Example: `  # 数值条件：大于 80 时标红
  dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID \
    --ranges '["A1:A100"]' \
    --condition '{"numberCondition":{"operator":"greater","value1":"80"}}' \
    --cell-style '{"backgroundColor":"#FFCDD2","fontColor":"#B71C1C","bold":true}'

  # 文本条件：包含"延期"时加删除线
  dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID \
    --ranges '["B1:B50"]' \
    --condition '{"textCondition":{"operator":"contains","value":"延期"}}' \
    --cell-style '{"backgroundColor":"#FFF3E0","strikethrough":true}'

  # 数据条
  dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID \
    --ranges '["C1:C20"]' \
    --condition '{"dataBarCondition":{"minPoint":{"type":"auto"},"maxPoint":{"type":"auto"}}}' \
    --data-bar-style '{"fill":["#4CAF50","#F44336"],"isGradient":true}'

  # 色阶（三色）
  dws sheet cond-format create --node NODE_ID --sheet-id SHEET_ID \
    --ranges '["D1:D50"]' \
    --condition '{"colorScaleCondition":{"criterias":[{"type":"maxmin","color":"#F44336"},{"type":"percentile","value":"50","color":"#FFEB3B"},{"type":"maxmin","color":"#4CAF50"}]}}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rangesStr := mustGetFlag(cmd, "ranges")
			var ranges []string
			if err := json.Unmarshal([]byte(rangesStr), &ranges); err != nil {
				return fmt.Errorf("--ranges JSON 解析失败: %w", err)
			}
			if len(ranges) == 0 {
				return fmt.Errorf("--ranges 至少包含 1 个范围")
			}
			conditionStr := mustGetFlag(cmd, "condition")
			var condition map[string]any
			if err := json.Unmarshal([]byte(conditionStr), &condition); err != nil {
				return fmt.Errorf("--condition JSON 解析失败: %w", err)
			}
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"ranges":  ranges,
			}
			for key, val := range condition {
				toolArgs[key] = val
			}
			if cellStyleStr, _ := cmd.Flags().GetString("cell-style"); cellStyleStr != "" {
				var cellStyle map[string]any
				if err := json.Unmarshal([]byte(cellStyleStr), &cellStyle); err != nil {
					return fmt.Errorf("--cell-style JSON 解析失败: %w", err)
				}
				toolArgs["cellStyle"] = cellStyle
			}
			if dataBarStyleStr, _ := cmd.Flags().GetString("data-bar-style"); dataBarStyleStr != "" {
				var dataBarStyle map[string]any
				if err := json.Unmarshal([]byte(dataBarStyleStr), &dataBarStyle); err != nil {
					return fmt.Errorf("--data-bar-style JSON 解析失败: %w", err)
				}
				toolArgs["dataBarStyle"] = dataBarStyle
			}
			return callMCPTool("create_cond_format", toolArgs)
		},
	}
	condFormatCreateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	condFormatCreateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	condFormatCreateCmd.Flags().String("ranges", "", `应用范围 JSON 数组 (必填)，如 '["A1:E10"]'`)
	condFormatCreateCmd.Flags().String("condition", "", `条件类型及参数 JSON 对象 (必填)，如 '{"numberCondition":{"operator":"greater","value1":"80"}}'`)
	condFormatCreateCmd.Flags().String("cell-style", "", `单元格样式 JSON 对象 (可选)，如 '{"backgroundColor":"#FF0000","bold":true}'`)
	condFormatCreateCmd.Flags().String("data-bar-style", "", `数据条样式 JSON 对象 (可选，仅数据条类型)，如 '{"fill":["#4CAF50","#F44336"],"isGradient":true}'`)

	condFormatUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新条件格式规则",
		Long: `更新钉钉电子表格中指定工作表的一条条件格式规则。

可以部分更新以下字段：
- --ranges：修改规则的应用范围
- --condition：切换或修改条件类型及参数
- --cell-style：修改命中时的单元格样式
- --data-bar-style：修改数据条样式

未传入的字段将保持原有值不变。
传入 --condition 时，新条件类型将替换原有条件类型。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --rule-id 定位条件格式规则。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
ruleId 可通过 cond-format list 获取。`,
		Example: `  # 修改规则的条件（改为大于 90）
  dws sheet cond-format update --node NODE_ID --sheet-id SHEET_ID --rule-id RULE_ID \
    --condition '{"numberCondition":{"operator":"greater","value1":"90"}}'

  # 修改规则的样式
  dws sheet cond-format update --node NODE_ID --sheet-id SHEET_ID --rule-id RULE_ID \
    --cell-style '{"backgroundColor":"#C8E6C9","fontColor":"#1B5E20"}'

  # 修改规则的应用范围
  dws sheet cond-format update --node NODE_ID --sheet-id SHEET_ID --rule-id RULE_ID \
    --ranges '["A1:F200"]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"ruleId":  mustGetFlag(cmd, "rule-id"),
			}
			rangesChanged := cmd.Flags().Changed("ranges")
			conditionChanged := cmd.Flags().Changed("condition")
			cellStyleChanged := cmd.Flags().Changed("cell-style")
			dataBarStyleChanged := cmd.Flags().Changed("data-bar-style")
			if !rangesChanged && !conditionChanged && !cellStyleChanged && !dataBarStyleChanged {
				return fmt.Errorf("--ranges、--condition、--cell-style、--data-bar-style 至少需要传入一个")
			}
			if rangesChanged {
				rangesStr, _ := cmd.Flags().GetString("ranges")
				var ranges []string
				if err := json.Unmarshal([]byte(rangesStr), &ranges); err != nil {
					return fmt.Errorf("--ranges JSON 解析失败: %w", err)
				}
				toolArgs["ranges"] = ranges
			}
			if conditionChanged {
				conditionStr, _ := cmd.Flags().GetString("condition")
				var condition map[string]any
				if err := json.Unmarshal([]byte(conditionStr), &condition); err != nil {
					return fmt.Errorf("--condition JSON 解析失败: %w", err)
				}
				for key, val := range condition {
					toolArgs[key] = val
				}
			}
			if cellStyleChanged {
				cellStyleStr, _ := cmd.Flags().GetString("cell-style")
				var cellStyle map[string]any
				if err := json.Unmarshal([]byte(cellStyleStr), &cellStyle); err != nil {
					return fmt.Errorf("--cell-style JSON 解析失败: %w", err)
				}
				toolArgs["cellStyle"] = cellStyle
			}
			if dataBarStyleChanged {
				dataBarStyleStr, _ := cmd.Flags().GetString("data-bar-style")
				var dataBarStyle map[string]any
				if err := json.Unmarshal([]byte(dataBarStyleStr), &dataBarStyle); err != nil {
					return fmt.Errorf("--data-bar-style JSON 解析失败: %w", err)
				}
				toolArgs["dataBarStyle"] = dataBarStyle
			}
			return callMCPTool("update_cond_format", toolArgs)
		},
	}
	condFormatUpdateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	condFormatUpdateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	condFormatUpdateCmd.Flags().String("rule-id", "", "条件格式规则 ID (必填)")
	condFormatUpdateCmd.Flags().String("ranges", "", `应用范围 JSON 数组 (可选)，如 '["A1:E10"]'`)
	condFormatUpdateCmd.Flags().String("condition", "", `条件类型及参数 JSON 对象 (可选)，传入后替换原有条件类型`)
	condFormatUpdateCmd.Flags().String("cell-style", "", `单元格样式 JSON 对象 (可选)`)
	condFormatUpdateCmd.Flags().String("data-bar-style", "", `数据条样式 JSON 对象 (可选，仅数据条类型)`)

	condFormatDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除条件格式规则",
		Long: `删除钉钉电子表格中指定工作表的一条条件格式规则。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --rule-id 定位要删除的规则。
删除成功后该规则将从工作表中移除，被该规则影响的单元格将不再应用对应的条件格式样式。
如果规则已不存在，操作仍然返回成功。

⚠️ 危险操作：删除不可恢复。必须先向用户确认，用户同意后才加 --yes 执行。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
ruleId 可通过 cond-format list 获取。`,
		Example: `  # 删除条件格式规则（必须加 --yes 确认）
  dws sheet cond-format delete --node NODE_ID --sheet-id SHEET_ID --rule-id RULE_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("delete_cond_format", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"ruleId":  mustGetFlag(cmd, "rule-id"),
			})
		},
	}
	condFormatDeleteCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	condFormatDeleteCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	condFormatDeleteCmd.Flags().String("rule-id", "", "条件格式规则 ID (必填)")
	condFormatDeleteCmd.Flags().Bool("yes", false, "确认删除（危险操作，必须用户同意后才加此标志）")

	condFormatCmd.AddCommand(condFormatListCmd, condFormatCreateCmd, condFormatUpdateCmd, condFormatDeleteCmd)
	return condFormatCmd
}

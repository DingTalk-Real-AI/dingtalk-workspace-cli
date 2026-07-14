package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newDimensionCmds creates dimension-related commands: insert/delete/update/move/add-dimension,
// merge-cells, unmerge-cells, and dropdown commands (set/get/delete-dropdown).
func newDimensionCmds() []*cobra.Command {
	insertDimensionCmd := &cobra.Command{
		Use:   "insert-dimension",
		Short: "在指定位置插入行或列",
		Long: `在钉钉表格指定工作表的指定位置之前（before）插入若干空行或空列。

位置参数使用 A1 表示法：
  --dimension ROWS 时，--position 为 1-based 行号字符串，如 "3" 表示在第 3 行之前插入
  --dimension COLUMNS 时，--position 为列字母，如 "A" 表示在 A 列之前插入、"AB" 表示在 AB 列之前插入

支持在 --position 中携带工作表前缀（如 "Sheet1!3" / "Sheet1!A"），此时将忽略 --sheet-id。
若需要在工作表末尾追加行/列，请使用 append 命令。`,
		Example: `  # 在第 3 行之前插入 2 行
  dws sheet insert-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --position "3" --length 2

  # 在 A 列之前插入 1 列
  dws sheet insert-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --position "A" --length 1

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet insert-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --position "Sheet1!3" --length 5

  # 在 AB 列之前插入 3 列
  dws sheet insert-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --position "AB" --length 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "dimension", "position", "length"); err != nil {
				return err
			}
			dimension := mustGetFlag(cmd, "dimension")
			if dimension != "ROWS" && dimension != "COLUMNS" {
				return fmt.Errorf("--dimension 必须为 ROWS 或 COLUMNS，当前值: %s", dimension)
			}
			lengthStr := mustGetFlag(cmd, "length")
			var length int
			if _, err := fmt.Sscanf(lengthStr, "%d", &length); err != nil || length < 1 {
				return fmt.Errorf("--length 必须为正整数（>= 1），当前值: %s", lengthStr)
			}
			if length > 5000 {
				return fmt.Errorf("--length 最大为 5000，当前值: %d", length)
			}
			return callMCPTool("insert_dimension", map[string]any{
				"nodeId":    mustGetFlag(cmd, "node"),
				"sheetId":   mustGetFlag(cmd, "sheet-id"),
				"dimension": dimension,
				"position":  mustGetFlag(cmd, "position"),
				"length":    length,
			})
		},
	}
	insertDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	insertDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	insertDimensionCmd.Flags().String("dimension", "", "插入维度: ROWS 或 COLUMNS (必填)")
	insertDimensionCmd.Flags().String("position", "", "插入位置，A1 表示法 (必填)。ROWS 时为行号如 \"3\"；COLUMNS 时为列字母如 \"A\"")
	insertDimensionCmd.Flags().String("length", "", "插入数量，正整数 (必填)，最大 5000")

	moveDimensionCmd := &cobra.Command{
		Use:   "move-dimension",
		Short: "移动行或列到指定位置/调整顺序",
		Long: `在钉钉表格的指定工作表中，将指定范围的行或列移动到目标位置。

参数说明：
  --dimension         维度类型，ROWS 表示移动行，COLUMNS 表示移动列
  --start-index       源起始位置，A1 表示法（ROWS 时为行号如 "2"，COLUMNS 时为列字母如 "B"）
  --end-index         源结束位置，A1 表示法（同 start-index 格式，必须 >= start-index）
  --destination-index 目标位置，A1 表示法，源行/列移动后将出现在该位置

注意事项：
  - destination-index 不能在源范围 [start-index, end-index] 内
  - 向下/向右移动时，destination-index 应大于 end-index
  - 向上/向左移动时，destination-index 应小于 start-index`,
		Example: `  # 将第 2 行移动到第 5 行的位置
  dws sheet move-dimension --node NODE_ID --sheet-id SHEET_ID \
    --dimension ROWS --start-index "2" --end-index "2" --destination-index "5"

  # 将第 2~4 行（共 3 行）移动到第 1 行的位置（最前面）
  dws sheet move-dimension --node NODE_ID --sheet-id SHEET_ID \
    --dimension ROWS --start-index "2" --end-index "4" --destination-index "1"

  # 将 B~C 列（共 2 列）移动到 D 列的位置
  dws sheet move-dimension --node NODE_ID --sheet-id SHEET_ID \
    --dimension COLUMNS --start-index "B" --end-index "C" --destination-index "D"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dimension := mustGetFlag(cmd, "dimension")
			switch dimension {
			case "ROW":
				dimension = "ROWS"
			case "COLUMN":
				dimension = "COLUMNS"
			}
			if dimension != "ROWS" && dimension != "COLUMNS" {
				return fmt.Errorf("--dimension 必须为 ROWS 或 COLUMNS，当前值: %s。请使用 --help 查看参数说明", dimension)
			}
			startIndex := mustGetFlag(cmd, "start-index")
			if startIndex == "" {
				return fmt.Errorf("--start-index 为必填参数，请使用 --help 查看参数说明")
			}
			endIndex := mustGetFlag(cmd, "end-index")
			if endIndex == "" {
				return fmt.Errorf("--end-index 为必填参数，请使用 --help 查看参数说明")
			}
			destinationIndex := mustGetFlag(cmd, "destination-index")
			if destinationIndex == "" {
				return fmt.Errorf("--destination-index 为必填参数，请使用 --help 查看参数说明")
			}
			return callMCPTool("move_dimension", map[string]any{
				"nodeId":           mustGetFlag(cmd, "node"),
				"sheetId":          mustGetFlag(cmd, "sheet-id"),
				"dimension":        dimension,
				"startIndex":       startIndex,
				"endIndex":         endIndex,
				"destinationIndex": destinationIndex,
			})
		},
	}
	moveDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	moveDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	moveDimensionCmd.Flags().String("dimension", "", "维度类型: ROWS 或 COLUMNS (必填)")
	moveDimensionCmd.Flags().String("start-index", "", "源起始位置，A1 表示法 (必填)")
	moveDimensionCmd.Flags().String("end-index", "", "源结束位置，A1 表示法 (必填)")
	moveDimensionCmd.Flags().String("destination-index", "", "目标位置，A1 表示法 (必填)")

	addDimensionCmd := &cobra.Command{
		Use:   "add-dimension",
		Short: "在末尾追加空行或空列",
		Long: `在钉钉表格的指定工作表末尾追加空行或空列。

参数说明：
  --dimension  维度类型，ROWS 表示追加行，COLUMNS 表示追加列
  --length     追加数量，正整数（>= 1），最多 5000`,
		Example: `  # 在末尾追加 5 行空行
  dws sheet add-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --length 5

  # 在末尾追加 3 列空列
  dws sheet add-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --length 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dimension := mustGetFlag(cmd, "dimension")
			if dimension != "ROWS" && dimension != "COLUMNS" {
				return fmt.Errorf("--dimension 必须为 ROWS 或 COLUMNS，当前值: %s", dimension)
			}
			length, err := cmd.Flags().GetInt("length")
			if err != nil {
				return fmt.Errorf("--length 解析失败: %w", err)
			}
			if length < 1 {
				return fmt.Errorf("--length 必须为正整数（>= 1），当前值: %d", length)
			}
			if length > 5000 {
				return fmt.Errorf("--length 最大为 5000，当前值: %d", length)
			}
			return callMCPTool("add_dimension", map[string]any{
				"nodeId":    mustGetFlag(cmd, "node"),
				"sheetId":   mustGetFlag(cmd, "sheet-id"),
				"dimension": dimension,
				"length":    length,
			})
		},
	}
	addDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	addDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	addDimensionCmd.Flags().String("dimension", "", "维度类型: ROWS 或 COLUMNS (必填)")
	addDimensionCmd.Flags().Int("length", 0, "追加数量，正整数 (必填)")

	mergeCellsCmd := &cobra.Command{
		Use:   "merge-cells",
		Short: "合并单元格",
		Long: `将钉钉电子表格中指定工作表的指定区域内的单元格合并为一个或多个合并区域。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
rangeAddress 也支持带工作表前缀的写法，如 Sheet1!A1:B3，此时将优先使用前缀解析出的工作表。

支持三种合并方式（通过 --merge-type 指定）：
  mergeAll（默认）    合并所有单元格，将选定区域内的所有单元格合并成一个
  mergeRows           按行合并，在选定区域内将同一行相邻的单元格合并
  mergeColumns        按列合并，在选定区域内将同一列相邻的单元格合并

注意：合并时只保留左上角单元格的值，其他单元格的值会被丢弃。
如果目标区域与其他合并单元格、锁定区域或表格区域存在交集，合并将失败。
仅限用户对文档具备"可编辑"权限时可操作。不支持跨组织操作。`,
		Example: `  # 合并所有单元格（默认）
  dws sheet merge-cells --node NODE_ID --sheet-id SHEET_ID --range "A1:B3"

  # 按行合并
  dws sheet merge-cells --node NODE_ID --sheet-id SHEET_ID --range "A1:C3" --merge-type mergeRows

  # 按列合并
  dws sheet merge-cells --node NODE_ID --sheet-id SHEET_ID --range "A1:C3" --merge-type mergeColumns

  # 使用带工作表前缀的范围（忽略 --sheet-id）
  dws sheet merge-cells --node NODE_ID --sheet-id SHEET_ID --range "Sheet1!A1:B3"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"rangeAddress": mustGetFlag(cmd, "range"),
			}
			if v, _ := cmd.Flags().GetString("merge-type"); v != "" {
				toolArgs["mergeType"] = v
			}
			return callMCPTool("merge_cells", toolArgs)
		},
	}
	mergeCellsCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	mergeCellsCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	mergeCellsCmd.Flags().String("range", "", "目标单元格区域地址，如 A1:B3 (必填)")
	mergeCellsCmd.Flags().String("merge-type", "", "合并方式: mergeAll(默认)/mergeRows/mergeColumns")

	unmergeRangeCmd := &cobra.Command{
		Use:   "unmerge-cells",
		Short: "取消合并单元格",
		Long: `取消钉钉表格指定工作表中指定范围内的合并单元格。

通过 --range 参数指定要取消合并的范围，使用 A1 表示法（如 A1:D5）。
该范围内所有合并的单元格都会被取消合并，恢复为独立的单元格。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。`,
		Example: `  # 取消 A1:D5 范围内的合并
  dws sheet unmerge-cells --node NODE_ID --sheet-id SHEET_ID --range "A1:D5"

  # 取消整个工作表的合并
  dws sheet unmerge-cells --node NODE_ID --sheet-id SHEET_ID --range "A1:ZZ10000"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("unmerge_range", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"rangeAddress": mustGetFlag(cmd, "range"),
			})
		},
	}
	unmergeRangeCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	unmergeRangeCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	unmergeRangeCmd.Flags().String("range", "", "取消合并的范围，A1 表示法，如 A1:D5 (必填)")

	deleteDimensionCmd := &cobra.Command{
		Use:   "delete-dimension",
		Short: "删除指定位置的行或列",
		Long: `在钉钉表格指定工作表中，从指定位置起删除若干连续的行或列。

位置参数使用 A1 表示法：
  --dimension ROWS 时，--position 为 1-based 行号字符串，如 "3" 表示从第 3 行开始删除
  --dimension COLUMNS 时，--position 为列字母，如 "A" 表示从 A 列开始删除、"AB" 表示从 AB 列开始删除

支持在 --position 中携带工作表前缀（如 "Sheet1!3" / "Sheet1!A"），此时将忽略 --sheet-id。
删除后后续的行/列会向前移动填补空位；若需要仅清空内容但保留行/列占位，请使用 clear_range 工具。`,
		Example: `  # 获得用户确认后从第 3 行开始删除 2 行
  dws sheet delete-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --position "3" --length 2 --yes

  # 获得用户确认后从 A 列开始删除 1 列
  dws sheet delete-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --position "A" --length 1 --yes

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet delete-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --position "Sheet1!3" --length 5 --yes

  # 获得用户确认后从 AB 列开始删除 3 列
  dws sheet delete-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --position "AB" --length 3 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "dimension", "position", "length"); err != nil {
				return err
			}
			dimension := mustGetFlag(cmd, "dimension")
			if dimension != "ROWS" && dimension != "COLUMNS" {
				return fmt.Errorf("--dimension 必须为 ROWS 或 COLUMNS，当前值: %s", dimension)
			}
			lengthStr := mustGetFlag(cmd, "length")
			var length int
			if _, err := fmt.Sscanf(lengthStr, "%d", &length); err != nil || length < 1 {
				return fmt.Errorf("--length 必须为正整数（>= 1），当前值: %s", lengthStr)
			}
			if length > 5000 {
				return fmt.Errorf("--length 最大为 5000，当前值: %d", length)
			}
			return callMCPTool("delete_dimension", map[string]any{
				"nodeId":    mustGetFlag(cmd, "node"),
				"sheetId":   mustGetFlag(cmd, "sheet-id"),
				"dimension": dimension,
				"position":  mustGetFlag(cmd, "position"),
				"length":    length,
			})
		},
	}
	deleteDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	deleteDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	deleteDimensionCmd.Flags().String("dimension", "", "删除维度: ROWS 或 COLUMNS (必填)")
	deleteDimensionCmd.Flags().String("position", "", "删除起始位置，A1 表示法 (必填)。ROWS 时为行号如 \"3\"；COLUMNS 时为列字母如 \"A\"")
	deleteDimensionCmd.Flags().String("length", "", "删除数量，正整数 (必填)，最大 5000")

	updateDimensionCmd := &cobra.Command{
		Use:   "update-dimension",
		Short: "更新指定范围行/列属性（显隐、行高/列宽）",
		Long: `批量更新钉钉表格指定工作表中连续多行/多列的属性，支持设置显隐状态（hidden）与行高/列宽（pixelSize）。

起始位置 startIndex 使用 A1 表示法：
  --dimension ROWS 时，--start-index 为 1-based 行号字符串，如 "3" 表示从第 3 行开始
  --dimension COLUMNS 时，--start-index 为列字母，如 "A" 表示从 A 列开始、"AB" 表示从 AB 列开始

支持在 --start-index 中携带工作表前缀（如 "Sheet1!3" / "Sheet1!A"），此时将忽略 --sheet-id。
--hidden 与 --pixel-size 至少必须提供一个。当同时提供时，将先应用尺寸再应用显隐，任一失败整体失败。
--pixel-size 单位为像素，dimension=ROWS 时表示行高、dimension=COLUMNS 时表示列宽。

常见场景：隐藏/显示指定连续行或列、批量调整行高/列宽、在同一次调用中同时修改尺寸与显隐。`,
		Example: `  # 隐藏第 3~4 行
  dws sheet update-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --start-index "3" --length 2 --hidden

  # 显示 A~B 列
  dws sheet update-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --start-index "A" --length 2 --hidden=false

  # 设置第 1~5 行行高为 40px
  dws sheet update-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --start-index "1" --length 5 --pixel-size 40

  # 设置 C 列列宽为 200px 并隐藏
  dws sheet update-dimension --node NODE_ID --sheet-id SHEET_ID --dimension COLUMNS --start-index "C" --length 1 --pixel-size 200 --hidden

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet update-dimension --node NODE_ID --sheet-id SHEET_ID --dimension ROWS --start-index "Sheet1!3" --length 2 --hidden`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "dimension", "start-index", "length"); err != nil {
				return err
			}
			dimension := mustGetFlag(cmd, "dimension")
			if dimension != "ROWS" && dimension != "COLUMNS" {
				return fmt.Errorf("--dimension 必须为 ROWS 或 COLUMNS，当前值: %s", dimension)
			}
			lengthStr := mustGetFlag(cmd, "length")
			var length int
			if _, err := fmt.Sscanf(lengthStr, "%d", &length); err != nil || length < 1 {
				return fmt.Errorf("--length 必须为正整数（>= 1），当前值: %s", lengthStr)
			}
			if length > 5000 {
				return fmt.Errorf("--length 最大为 5000，当前值: %d", length)
			}
			hiddenChanged := cmd.Flags().Changed("hidden")
			pixelSizeChanged := cmd.Flags().Changed("pixel-size")
			if !hiddenChanged && !pixelSizeChanged {
				return fmt.Errorf("--hidden 与 --pixel-size 至少必须提供一个")
			}
			toolArgs := map[string]any{
				"nodeId":     mustGetFlag(cmd, "node"),
				"sheetId":    mustGetFlag(cmd, "sheet-id"),
				"dimension":  dimension,
				"startIndex": mustGetFlag(cmd, "start-index"),
				"length":     length,
			}
			if hiddenChanged {
				hidden, _ := cmd.Flags().GetBool("hidden")
				toolArgs["hidden"] = hidden
			}
			if pixelSizeChanged {
				pixelSize, _ := cmd.Flags().GetInt("pixel-size")
				if pixelSize < 0 {
					return fmt.Errorf("--pixel-size 必须为非负整数，当前值: %d", pixelSize)
				}
				toolArgs["pixelSize"] = pixelSize
			}
			return callMCPTool("update_dimension", toolArgs)
		},
	}
	updateDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	updateDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	updateDimensionCmd.Flags().String("dimension", "", "更新维度: ROWS 或 COLUMNS (必填)")
	updateDimensionCmd.Flags().String("start-index", "", "起始位置，A1 表示法 (必填)。ROWS 时为行号如 \"3\"；COLUMNS 时为列字母如 \"A\"")
	updateDimensionCmd.Flags().String("length", "", "更新数量，正整数 (必填)，最大 5000")
	updateDimensionCmd.Flags().Bool("hidden", false, "是否隐藏 (true=隐藏, false=显示)")
	updateDimensionCmd.Flags().Int("pixel-size", 0, "行高或列宽（像素），ROWS 时为行高，COLUMNS 时为列宽")

	groupDimensionCmd := &cobra.Command{
		Use:   "group-dimension",
		Short: "对指定连续行/列创建分组",
		Long: `对钉钉表格指定工作表中的连续整行或整列创建分组。

--range 使用整行/整列范围：
  行分组：3:7 或 3
  列分组：C:F 或 C

支持在 --range 中携带工作表前缀（如 "Sheet1!3:7" / "Sheet1!C:F"），此时将忽略 --sheet-id。
创建后可通过 sheet info --include groups 回读 rowGroups / columnGroups。

--group-state 支持 expand / fold，默认 expand。`,
		Example: `  # 分组第 3~7 行
  dws sheet group-dimension --node NODE_ID --sheet-id SHEET_ID --range "3:7"

  # 分组并折叠 C~F 列
  dws sheet group-dimension --node NODE_ID --sheet-id SHEET_ID --range "C:F" --group-state fold

  # 使用工作表前缀（忽略 --sheet-id）
  dws sheet group-dimension --node NODE_ID --sheet-id SHEET_ID --range "Sheet1!3:7"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range"); err != nil {
				return err
			}
			groupState, _ := cmd.Flags().GetString("group-state")
			switch groupState {
			case "", "expand":
				groupState = "expand"
			case "fold":
			default:
				return fmt.Errorf("--group-state 必须为 expand 或 fold，当前值: %s", groupState)
			}
			return callMCPTool("group_dimension", map[string]any{
				"nodeId":     mustGetFlag(cmd, "node"),
				"sheetId":    mustGetFlag(cmd, "sheet-id"),
				"range":      mustGetFlag(cmd, "range"),
				"groupState": groupState,
			})
		},
	}
	groupDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	groupDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	groupDimensionCmd.Flags().String("range", "", `整行/整列范围 (必填)，如 "3:7" 或 "C:F"`)
	groupDimensionCmd.Flags().String("group-state", "expand", "创建后的分组状态: expand 或 fold")

	ungroupDimensionCmd := &cobra.Command{
		Use:   "ungroup-dimension",
		Short: "取消指定连续行/列分组",
		Long: `取消钉钉表格指定工作表中的连续整行或整列分组。

--range 使用整行/整列范围：
  行分组：3:7 或 3
  列分组：C:F 或 C

支持在 --range 中携带工作表前缀（如 "Sheet1!3:7" / "Sheet1!C:F"），此时将忽略 --sheet-id。
取消后可通过 sheet info --include groups 回读 rowGroups / columnGroups。`,
		Example: `  # 取消第 3~7 行分组
  dws sheet ungroup-dimension --node NODE_ID --sheet-id SHEET_ID --range "3:7"

  # 取消 C~F 列分组
  dws sheet ungroup-dimension --node NODE_ID --sheet-id SHEET_ID --range "C:F"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "sheet-id", "range"); err != nil {
				return err
			}
			return callMCPTool("ungroup_dimension", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			})
		},
	}
	ungroupDimensionCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	ungroupDimensionCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	ungroupDimensionCmd.Flags().String("range", "", `整行/整列范围 (必填)，如 "3:7" 或 "C:F"`)

	// ── dropdown ──────────────────────────────────────────────────
	setDropdownCmd := &cobra.Command{
		Use:   "set-dropdown",
		Short: "设置下拉列表",
		Long: `在钉钉表格的指定单元格范围内设置下拉列表。

设置后，用户可以在这些单元格中从预定义的选项列表中选择值。
支持自定义每个选项的颜色和是否允许多选。
如果目标范围已存在下拉列表，会被新的配置覆盖。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。

--options 为 JSON 数组，每个元素包含 value（必填）和 color（可选）。
选项值不能包含英文逗号。`,
		Example: `  # 设置单选下拉列表
  dws sheet set-dropdown --node NODE_ID --sheet-id SHEET_ID --range "A2:A100" \
    --options '[{"value":"选项1"},{"value":"选项2"},{"value":"选项3"}]'

  # 设置带颜色的多选下拉列表
  dws sheet set-dropdown --node NODE_ID --sheet-id SHEET_ID --range "B2:B50" \
    --options '[{"value":"高","color":"#ff0000"},{"value":"中","color":"#ffaa00"},{"value":"低","color":"#00ff00"}]' \
    --multi-select

  # 使用文档链接 URL
  dws sheet set-dropdown --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" \
    --sheet-id SHEET_ID --range "C1:C10" --options '[{"value":"是"},{"value":"否"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			optionsStr := mustGetFlag(cmd, "options")
			var options []map[string]any
			if err := json.Unmarshal([]byte(optionsStr), &options); err != nil {
				return fmt.Errorf("--options JSON 解析失败: %w", err)
			}
			if len(options) == 0 {
				return fmt.Errorf("--options 至少包含 1 个选项")
			}
			for i, opt := range options {
				val, ok := opt["value"].(string)
				if !ok || val == "" {
					return fmt.Errorf("--options[%d] 缺少必填的 value 字段或 value 为空", i)
				}
				if strings.Contains(val, ",") {
					return fmt.Errorf("--options[%d].value 不能包含英文逗号: %q", i, val)
				}
			}
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
				"options": options,
			}
			if multiSelect, _ := cmd.Flags().GetBool("multi-select"); multiSelect {
				toolArgs["enableMultiSelect"] = true
			}
			return callMCPTool("set_dropdown_lists", toolArgs)
		},
	}
	setDropdownCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	setDropdownCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	setDropdownCmd.Flags().String("range", "", "目标单元格范围，A1 表示法，如 A2:A100 (必填)")
	setDropdownCmd.Flags().String("options", "", `下拉选项 JSON 数组 (必填)，如 '[{"value":"选项1","color":"#ff0000"}]'`)
	setDropdownCmd.Flags().Bool("multi-select", false, "是否允许多选（默认单选）")

	getDropdownCmd := &cobra.Command{
		Use:   "get-dropdown",
		Short: "获取下拉列表配置",
		Long: `查询钉钉表格指定范围内的下拉列表配置。

返回范围内所有单元格的下拉列表配置信息，包括选项值和颜色。
如果范围内存在多个不同的下拉列表配置，会分别返回每组配置及其覆盖的单元格列表。
如果范围内没有设置下拉列表，返回空。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。`,
		Example: `  # 查询指定范围的下拉列表
  dws sheet get-dropdown --node NODE_ID --sheet-id SHEET_ID --range "A2:A100"

  # 查询单个单元格
  dws sheet get-dropdown --node NODE_ID --sheet-id SHEET_ID --range "A1"

  # 查询多列范围
  dws sheet get-dropdown --node NODE_ID --sheet-id SHEET_ID --range "A1:D10"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_dropdown_lists", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			})
		},
	}
	getDropdownCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	getDropdownCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	getDropdownCmd.Flags().String("range", "", "查询范围，A1 表示法，如 A1:A100 (必填)")

	deleteDropdownCmd := &cobra.Command{
		Use:   "delete-dropdown",
		Short: "删除下拉列表",
		Long: `删除钉钉表格指定单元格范围内的下拉列表配置。

删除后，单元格将恢复为普通文本格式，已填写的单元格值不会被清除。
如果目标范围不存在下拉列表，操作仍然返回成功。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。`,
		Example: `  # 获得用户确认后删除指定范围的下拉列表
  dws sheet delete-dropdown --node NODE_ID --sheet-id SHEET_ID --range "A2:A100" --yes

  # 获得用户确认后删除多列范围的下拉列表
  dws sheet delete-dropdown --node NODE_ID --sheet-id SHEET_ID --range "B1:D10" --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("delete_dropdown_lists", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			})
		},
	}
	deleteDropdownCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	deleteDropdownCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	deleteDropdownCmd.Flags().String("range", "", "要删除下拉列表的范围，A1 表示法，如 A2:A100 (必填)")

	return []*cobra.Command{
		insertDimensionCmd, moveDimensionCmd, addDimensionCmd,
		mergeCellsCmd, unmergeRangeCmd,
		deleteDimensionCmd, updateDimensionCmd, groupDimensionCmd, ungroupDimensionCmd,
		setDropdownCmd, getDropdownCmd, deleteDropdownCmd,
	}
}

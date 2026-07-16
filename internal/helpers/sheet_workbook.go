package helpers

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newWorkbookCmds() []*cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "创建钉钉表格文档",
		Long: `创建一篇新的钉钉在线电子表格，支持创建空表格。

创建位置优先级: --folder > --workspace > 默认 (我的文档根目录)`,
		Example: `  dws sheet create --name "销售数据"
  dws sheet create --name "Q1 数据" --folder FOLDER_ID
  dws sheet create --name "知识库表格" --workspace WS_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"name": mustGetFlag(cmd, "name"),
			}
			if v, _ := cmd.Flags().GetString("folder"); v != "" {
				toolArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				toolArgs["workspaceId"] = v
			}
			return callMCPTool("create_workspace_sheet", toolArgs)
		},
	}
	createCmd.Flags().String("name", "", "表格名称 (必填)")
	createCmd.Flags().String("folder", "", "目标文件夹 ID 或 URL")
	createCmd.Flags().String("workspace", "", "目标知识库 ID")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "获取全部工作表列表",
		Long: `获取钉钉电子表格中所有工作表的 ID 和名称。
返回的 sheetId 可用于 info / range read (别名 get) / range update 等后续操作。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
文档链接格式：https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`,
		Example: `  dws sheet list --node NODE_ID
  dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_all_sheets", map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
			})
		},
	}
	listCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")

	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "获取指定工作表详情",
		Long: `获取指定工作表的详细信息，包括 ID、名称、可见性、行列数、最后非空行列位置、合并单元格范围（mergedRanges）等。

非空数据边界使用 A1/UI 语义字段：
  nonEmptyRange.range              从 A1 到最后非空单元格的 A1 范围（空表为 null）
  nonEmptyRange.lastCell           最后非空单元格地址（如 J5，空表为 null）
  nonEmptyRange.lastRow            1-based 行号（空表为 null）
  nonEmptyRange.lastColumn         列字母（空表为 null）

mergedRanges 已经是 A1 表示法（如 "A1:B2"），可原样用于 range/merge 相关命令。

nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid）；
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
不传 --sheet-id 时默认返回第一个工作表。`,
		Example: `  dws sheet info --node NODE_ID
  dws sheet info --node NODE_ID --sheet-id SHEET_ID
  dws sheet info --node NODE_ID --sheet-id "Sheet1"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
			}
			if v, _ := cmd.Flags().GetString("sheet-id"); v != "" {
				toolArgs["sheetId"] = v
			}
			return callMCPToolSheetInfo(toolArgs)
		},
	}
	infoCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	infoCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (不传则返回第一个工作表)")

	newCmd := &cobra.Command{
		Use:   "new",
		Short: "新建工作表",
		Long: `在指定钉钉表格中新建一个工作表。
当指定名称与已有工作表重复时，系统会自动重命名为合法值。`,
		Example: `  dws sheet new --node NODE_ID --name "Sheet2"
  dws sheet new --node NODE_ID --name "数据汇总"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("create_sheet", map[string]any{
				"nodeId": mustGetFlag(cmd, "node"),
				"name":   mustGetFlag(cmd, "name"),
			})
		},
	}
	newCmd.Flags().String("node", "", "表格文档 ID (必填)")
	newCmd.Flags().String("name", "", "工作表名称 (必填)")

	updateSheetCmd := &cobra.Command{
		Use:   "update",
		Short: "更新工作表属性",
		Long: `更新工作表名称、位置、隐藏状态、冻结行列、标签颜色。

--name / --index / --hidden / --frozen-row-count / --frozen-column-count / --tab-color 至少提供一个；
多个属性可同时传入，将在同一次请求中更新。

注意：
  - name 不能包含 / \ ? * [ ] : 等特殊字符，最长 100 字符
  - 至少需要保留一个可见的工作表，不能将所有工作表都隐藏
  - 冻结行数/列数不能超过工作表的总行数/总列数，设为 0 表示取消冻结
  - tab-color 为 Hex 格式如 #FF0000，传空字符串清除颜色`,
		Example: `  # 改名 + 调整冻结
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --name "汇总表" --frozen-row-count 2 --frozen-column-count 1

  # 隐藏工作表
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --hidden=true

  # 显示工作表
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --hidden=false

  # 移动工作表到第一个位置
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --index 0

  # 取消冻结
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --frozen-row-count 0 --frozen-column-count 0

  # 设置工作表标签颜色
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --tab-color "#FF0000"

  # 清除标签颜色
  dws sheet update --node NODE_ID --sheet-id SHEET_ID --tab-color ""`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nameChanged := cmd.Flags().Changed("name") || cmd.Flags().Changed("title")
			indexChanged := cmd.Flags().Changed("index")
			hiddenChanged := cmd.Flags().Changed("hidden")
			frozenRowChanged := cmd.Flags().Changed("frozen-row-count")
			frozenColChanged := cmd.Flags().Changed("frozen-column-count")
			tabColorChanged := cmd.Flags().Changed("tab-color")

			if !nameChanged && !indexChanged && !hiddenChanged && !frozenRowChanged && !frozenColChanged && !tabColorChanged {
				return fmt.Errorf("--name、--index、--hidden、--frozen-row-count、--frozen-column-count、--tab-color 至少必须提供一个")
			}

			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			}

			if nameChanged {
				toolArgs["title"] = resolveSheetName(cmd)
			}
			if indexChanged {
				index, _ := cmd.Flags().GetInt("index")
				if index < 0 {
					return fmt.Errorf("--index 不能为负数，当前值: %d", index)
				}
				toolArgs["index"] = index
			}
			if hiddenChanged {
				hidden, _ := cmd.Flags().GetBool("hidden")
				toolArgs["hidden"] = hidden
			}
			if frozenRowChanged {
				frozenRowCount, _ := cmd.Flags().GetInt("frozen-row-count")
				if frozenRowCount < 0 {
					return fmt.Errorf("--frozen-row-count 不能为负数，当前值: %d", frozenRowCount)
				}
				toolArgs["frozenRowCount"] = frozenRowCount
			}
			if frozenColChanged {
				frozenColumnCount, _ := cmd.Flags().GetInt("frozen-column-count")
				if frozenColumnCount < 0 {
					return fmt.Errorf("--frozen-column-count 不能为负数，当前值: %d", frozenColumnCount)
				}
				toolArgs["frozenColumnCount"] = frozenColumnCount
			}
			if tabColorChanged {
				tabColor, _ := cmd.Flags().GetString("tab-color")
				toolArgs["tabColor"] = tabColor
			}

			return callMCPTool("update_sheet", toolArgs)
		},
	}
	updateSheetCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	updateSheetCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	updateSheetCmd.Flags().String("name", "", "工作表新名称，最长 100 字符")
	updateSheetCmd.Flags().String("title", "", "--name 的别名（兼容）")
	updateSheetCmd.Flags().Int("index", 0, "工作表新位置索引，0-based")
	updateSheetCmd.Flags().Bool("hidden", false, "是否隐藏工作表 (true=隐藏, false=显示)")
	updateSheetCmd.Flags().Int("frozen-row-count", 0, "冻结行数，0 表示取消冻结")
	updateSheetCmd.Flags().Int("frozen-column-count", 0, "冻结列数，0 表示取消冻结")
	updateSheetCmd.Flags().String("tab-color", "", "工作表标签颜色，Hex 格式如 #FF0000；传空字符串清除颜色")

	copySheetCmd := &cobra.Command{
		Use:   "copy",
		Short: "复制工作表",
		Long: `复制指定工作表，在同一表格中创建一个副本。

复制操作会将源工作表的所有内容（包括数据、格式、公式等）完整复制到新工作表中。

可选参数：
  --name   指定副本名称；不传时系统自动生成（通常为"源名称 副本"）。
           名称与已有工作表重复时系统会自动重命名。
  --index  指定副本位置（0-based）；不传时放在源工作表之后。
           传 --index 时，CLI 会先复制，再追加一次位置更新，把副本移动到目标索引。

name 不能包含 / \ ? * [ ] : 等特殊字符，最长 100 字符。`,
		Example: `  # 按默认位置复制
  dws sheet copy --node NODE_ID --sheet-id SHEET_ID

  # 指定副本名称和位置
  dws sheet copy --node NODE_ID --sheet-id SHEET_ID --name "销售副本" --index 2

  # 只指定名称
  dws sheet copy --node NODE_ID --sheet-id SHEET_ID --name "备份"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			}
			if cmd.Flags().Changed("name") || cmd.Flags().Changed("title") {
				toolArgs["title"] = resolveSheetName(cmd)
			}
			if cmd.Flags().Changed("index") {
				index, _ := cmd.Flags().GetInt("index")
				if index < 0 {
					return fmt.Errorf("--index 不能为负数，当前值: %d", index)
				}
				toolArgs["index"] = index
			}
			return callMCPTool("copy_sheet", toolArgs)
		},
	}
	copySheetCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	copySheetCmd.Flags().String("sheet-id", "", "源工作表 ID 或名称 (必填)")
	copySheetCmd.Flags().String("name", "", "副本名称，最长 100 字符 (不传则系统自动生成)")
	copySheetCmd.Flags().String("title", "", "--name 的别名（兼容）")
	copySheetCmd.Flags().Int("index", 0, "副本位置索引，0-based (不传则放在源工作表之后)")

	deleteSheetCmd := &cobra.Command{
		Use:   "delete-sheet",
		Short: "删除工作表",
		Long: `删除指定的工作表。此操作不可逆，删除后工作表及其所有数据将无法恢复。

约束：
  - 不能删除隐藏的工作表（需先取消隐藏再删除）
  - 不能删除最后一个可见工作表（至少保留一个可见工作表）`,
		Example: `  dws sheet delete-sheet --node NODE_ID --sheet-id SHEET_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("delete_sheet", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			})
		},
	}
	deleteSheetCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	deleteSheetCmd.Flags().String("sheet-id", "", "要删除的工作表 ID 或名称 (必填)")

	showGridlineCmd := newGridlineVisibilityCmd("show-gridline", "显示工作表网格线", "visible")
	hideGridlineCmd := newGridlineVisibilityCmd("hide-gridline", "隐藏工作表网格线", "hidden")

	return []*cobra.Command{createCmd, listCmd, infoCmd, newCmd, updateSheetCmd, copySheetCmd, deleteSheetCmd, showGridlineCmd, hideGridlineCmd}
}

func newGridlineVisibilityCmd(use, short, visibility string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use,
		Short:   short,
		Example: fmt.Sprintf("  dws sheet %s --node NODE_ID --sheet-id SHEET_ID", use),
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "file-id", "node-id", "doc-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "sheet-id"); err != nil {
				return err
			}
			return callMCPTool("set_gridline_visibility", map[string]any{
				"nodeId":     nodeID,
				"sheetId":    mustGetFlag(cmd, "sheet-id"),
				"visibility": visibility,
			})
		},
	}
	cmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	return cmd
}

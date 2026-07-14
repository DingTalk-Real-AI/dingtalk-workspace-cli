package helpers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func fetchFilterViews(nodeID, sheetID string) ([]map[string]any, error) {
	ctx := context.Background()
	rawText, err := callMCPToolReturnText(ctx, "get_filter_views", map[string]any{
		"nodeId":  nodeID,
		"sheetId": sheetID,
	})
	if err != nil {
		return nil, err
	}
	if rawText == "" {
		return nil, fmt.Errorf("get_filter_views 返回为空")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(rawText), &parsed); err != nil {
		return nil, fmt.Errorf("解析 get_filter_views 返回失败: %w", err)
	}

	root := parsed
	if result, ok := parsed["result"].(map[string]any); ok {
		root = result
	}
	filterViewsRaw, ok := root["filterViews"].([]any)
	if !ok {
		return nil, nil
	}

	filterViews := make([]map[string]any, 0, len(filterViewsRaw))
	for _, item := range filterViewsRaw {
		if viewMap, ok := item.(map[string]any); ok {
			filterViews = append(filterViews, viewMap)
		}
	}
	return filterViews, nil
}

func findFilterViewByID(filterViews []map[string]any, filterViewID string) (map[string]any, error) {
	for _, view := range filterViews {
		viewID, _ := view["id"].(string)
		if viewID == filterViewID {
			return view, nil
		}
		if altID, _ := view["filterViewId"].(string); altID == filterViewID {
			return view, nil
		}
	}
	return nil, fmt.Errorf("未找到筛选视图 %q，请检查 --filter-view-id 是否正确（可通过 filter-view list 查看可用 ID）", filterViewID)
}

func runFilterViewInfo(cmd *cobra.Command, _ []string) error {
	nodeID := mustGetFlag(cmd, "node")
	sheetID := mustGetFlag(cmd, "sheet-id")
	filterViewID := mustGetFlag(cmd, "filter-view-id")

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "获取单个筛选视图详情")
		deps.Out.PrintKeyValue("表格", nodeID)
		deps.Out.PrintKeyValue("工作表", sheetID)
		deps.Out.PrintKeyValue("筛选视图", filterViewID)
		return nil
	}

	filterViews, err := fetchFilterViews(nodeID, sheetID)
	if err != nil {
		return err
	}

	view, err := findFilterViewByID(filterViews, filterViewID)
	if err != nil {
		return err
	}

	return deps.Out.PrintJSON(view)
}

func runFilterViewListCriteria(cmd *cobra.Command, _ []string) error {
	nodeID := mustGetFlag(cmd, "node")
	sheetID := mustGetFlag(cmd, "sheet-id")
	filterViewID := mustGetFlag(cmd, "filter-view-id")

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "列出筛选视图所有列条件")
		deps.Out.PrintKeyValue("表格", nodeID)
		deps.Out.PrintKeyValue("工作表", sheetID)
		deps.Out.PrintKeyValue("筛选视图", filterViewID)
		return nil
	}

	filterViews, err := fetchFilterViews(nodeID, sheetID)
	if err != nil {
		return err
	}

	view, err := findFilterViewByID(filterViews, filterViewID)
	if err != nil {
		return err
	}

	criteria, ok := view["criteria"].(map[string]any)
	if !ok || len(criteria) == 0 {
		return deps.Out.PrintJSON(map[string]any{})
	}

	return deps.Out.PrintJSON(criteria)
}

func runFilterViewGetCriteria(cmd *cobra.Command, _ []string) error {
	nodeID := mustGetFlag(cmd, "node")
	sheetID := mustGetFlag(cmd, "sheet-id")
	filterViewID := mustGetFlag(cmd, "filter-view-id")

	column, err := cmd.Flags().GetInt("column")
	if err != nil {
		return fmt.Errorf("--column 解析失败: %w", err)
	}
	if column < 0 {
		return fmt.Errorf("--column 不能为负数，当前值: %d", column)
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "获取单列筛选条件")
		deps.Out.PrintKeyValue("表格", nodeID)
		deps.Out.PrintKeyValue("工作表", sheetID)
		deps.Out.PrintKeyValue("筛选视图", filterViewID)
		deps.Out.PrintKeyValue("列偏移量", fmt.Sprintf("%d", column))
		return nil
	}

	filterViews, err := fetchFilterViews(nodeID, sheetID)
	if err != nil {
		return err
	}

	view, err := findFilterViewByID(filterViews, filterViewID)
	if err != nil {
		return err
	}

	criteria, ok := view["criteria"].(map[string]any)
	if !ok || len(criteria) == 0 {
		return fmt.Errorf("筛选视图 %q 没有设置任何筛选条件", filterViewID)
	}

	columnKey := fmt.Sprintf("%d", column)
	columnCriteria, ok := criteria[columnKey]
	if !ok {
		return fmt.Errorf("筛选视图 %q 的第 %d 列没有设置筛选条件（可通过 list-criteria 查看已设置条件的列）", filterViewID, column)
	}

	return deps.Out.PrintJSON(columnCriteria)
}

// ── filter + filter-view 命令定义 ──────────────────────────────────────────────

func newFilterCmd() *cobra.Command {
	filterCmd := &cobra.Command{Use: "filter", Short: "全局筛选管理"}

	filterGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取全局筛选信息",
		Long: `获取指定工作表的全局筛选信息，返回筛选范围和各列的筛选条件详情。

如果工作表未设置筛选，返回结果中筛选信息为空。
全局筛选影响所有协作者看到的数据展示，每个工作表最多一个。`,
		Example: `  dws sheet filter get --node NODE_ID --sheet-id SHEET_ID
  dws sheet filter get --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --sheet-id "Sheet1"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_filter", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			})
		},
	}
	filterGetCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterGetCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")

	filterCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建全局筛选",
		Long: `在工作表中创建全局筛选。

每个工作表只能有一个全局筛选，已存在时会报错。
--range 必须包含表头行（如 A1:E100），不能只包含数据行。
--criteria 可选，不传则仅创建空筛选框架，后续可通过 filter update 设置条件。`,
		Example: `  # 创建筛选框架（不设条件）
  dws sheet filter create --node NODE_ID --sheet-id SHEET_ID --range "A1:E100"

  # 创建筛选并同时设置按值筛选
  dws sheet filter create --node NODE_ID --sheet-id SHEET_ID --range "A1:E100" \
    --criteria '[{"column":1,"filterType":"values","visibleValues":["北京","上海"]}]'

  # 创建筛选并设置条件筛选
  dws sheet filter create --node NODE_ID --sheet-id SHEET_ID --range "A1:E100" \
    --criteria '[{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"range":   mustGetFlag(cmd, "range"),
			}
			if v, _ := cmd.Flags().GetString("criteria"); v != "" {
				var criteria []any
				if err := json.Unmarshal([]byte(v), &criteria); err != nil {
					return fmt.Errorf("--criteria JSON 解析失败: %w", err)
				}
				toolArgs["criteria"] = criteria
			}
			return callMCPTool("create_filter", toolArgs)
		},
	}
	filterCreateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterCreateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterCreateCmd.Flags().String("range", "", "筛选范围，A1 表示法，须包含表头行 (必填)")
	filterCreateCmd.Flags().String("criteria", "", "筛选条件 JSON 数组 (可选)")

	filterDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除全局筛选",
		Long: `删除工作表的全局筛选。

删除后所有筛选条件丢失，所有被隐藏的行将重新显示。此操作不可恢复。
工作表没有筛选时调用会报错。`,
		Example: `  dws sheet filter delete --node NODE_ID --sheet-id SHEET_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("delete_filter", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			})
		},
	}
	filterDeleteCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterDeleteCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")

	filterUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "批量更新筛选条件",
		Long: `批量更新筛选条件，可同时设置多列的筛选条件。

前置条件：工作表必须已创建筛选（通过 filter create）。
覆盖式：指定列的条件会被替换，未指定的列保持不变。

--criteria JSON 数组，每个元素含 column（列偏移量，从 0 开始）和筛选条件字段。
支持三种 filterType：
  - values：按值筛选，指定 visibleValues 数组
  - condition：按条件筛选，指定 conditions 数组（最多 2 个）和可选的 conditionOperator
  - color：按颜色筛选，指定 backgroundColor 或 fontColor（二选一）`,
		Example: `  # 同时设置多列筛选条件
  dws sheet filter update --node NODE_ID --sheet-id SHEET_ID \
    --criteria '[{"column":0,"filterType":"values","visibleValues":["已完成","进行中"]},{"column":2,"filterType":"condition","conditions":[{"operator":"greater","value":"50"}]}]'

  # 按颜色筛选
  dws sheet filter update --node NODE_ID --sheet-id SHEET_ID \
    --criteria '[{"column":1,"filterType":"color","backgroundColor":"#FF0000"}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			criteriaStr := mustGetFlag(cmd, "criteria")
			var criteria []any
			if err := json.Unmarshal([]byte(criteriaStr), &criteria); err != nil {
				return fmt.Errorf("--criteria JSON 解析失败: %w", err)
			}
			return callMCPTool("update_filter", map[string]any{
				"nodeId":   mustGetFlag(cmd, "node"),
				"sheetId":  mustGetFlag(cmd, "sheet-id"),
				"criteria": criteria,
			})
		},
	}
	filterUpdateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterUpdateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterUpdateCmd.Flags().String("criteria", "", "筛选条件 JSON 数组 (必填)")

	filterClearCriteriaCmd := &cobra.Command{
		Use:   "clear-criteria",
		Short: "清除单列筛选条件",
		Long: `清除筛选中某一列的筛选条件。

清除后该列不再参与筛选计算，之前被该列条件隐藏的行将重新显示。
仅清除指定列的条件，不删除整个筛选。如需删除整个筛选，使用 filter delete。
指定列没有设置筛选条件时调用不会报错（幂等）。`,
		Example: `  # 清除第 2 列（B 列）的筛选条件
  dws sheet filter clear-criteria --node NODE_ID --sheet-id SHEET_ID --column 1

  # 清除第 1 列（A 列）的筛选条件
  dws sheet filter clear-criteria --node NODE_ID --sheet-id SHEET_ID --column 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			column, _ := cmd.Flags().GetInt("column")
			return callMCPTool("clear_filter_criteria", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"column":  column,
			})
		},
	}
	filterClearCriteriaCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterClearCriteriaCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterClearCriteriaCmd.Flags().Int("column", 0, "列偏移量，从 0 开始 (必填)")

	filterSortCmd := &cobra.Command{
		Use:   "sort",
		Short: "筛选排序",
		Long: `对筛选范围内的数据按指定列排序。

前置条件：工作表必须已创建筛选（通过 filter create）。
排序会实际改变工作表中数据行的物理顺序，不可撤销。`,
		Example: `  # 按第 1 列（A 列）升序
  dws sheet filter sort --node NODE_ID --sheet-id SHEET_ID --column 0 --ascending

  # 按第 3 列（C 列）降序
  dws sheet filter sort --node NODE_ID --sheet-id SHEET_ID --column 2 --ascending=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			column, _ := cmd.Flags().GetInt("column")
			ascending, _ := cmd.Flags().GetBool("ascending")
			return callMCPTool("sort_filter", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"field": map[string]any{
					"column":    column,
					"ascending": ascending,
				},
			})
		},
	}
	filterSortCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterSortCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterSortCmd.Flags().Int("column", 0, "排序列偏移量，从 0 开始 (必填)")
	filterSortCmd.Flags().Bool("ascending", true, "是否升序，默认 true")

	filterCmd.AddCommand(filterGetCmd, filterCreateCmd, filterDeleteCmd,
		filterUpdateCmd, filterClearCriteriaCmd, filterSortCmd)
	return filterCmd
}

func newFilterViewCmd() *cobra.Command {
	filterViewCmd := &cobra.Command{Use: "filter-view", Short: "筛选视图管理"}

	filterViewListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取所有筛选视图",
		Long: `获取钉钉电子表格中指定工作表的所有筛选视图列表。

用途：查看当前工作表上已创建的所有筛选视图，获取视图 ID、名称和范围信息。
场景：在对筛选视图进行 update / delete / update-criteria 等操作前，先用 list 获取可用的 filterViewId。
区分：筛选视图（filter-view）是个人化的数据过滤方式，与全局筛选（filter）不同。每个用户可以创建自己的筛选视图，互不影响原始数据。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。

返回该工作表下所有筛选视图的 ID、名称和范围信息。如果没有筛选视图，返回空列表。`,
		Example: `  # 查看工作表上有哪些筛选视图
  dws sheet filter-view list --node NODE_ID --sheet-id SHEET_ID

  # 使用文档链接和工作表名称
  dws sheet filter-view list --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --sheet-id "Sheet1"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_filter_views", map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
			})
		},
	}
	filterViewListCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewListCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")

	filterViewCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建筛选视图",
		Long: `在钉钉电子表格的指定工作表中创建一个筛选视图。

用途：为指定数据区域创建一个可命名的个人化筛选视图，可选同时设置筛选条件。
场景：用户需要针对某个数据区域建立固定的筛选视角（如"高绩效员工""研发部数据"），方便反复查看。
区分：
  - 与全局筛选不同，筛选视图是个人化的，不影响其他用户看到的数据。
  - 如果只需创建视图不设条件，后续可通过 update-criteria 单独设置。
  - 如果要一步到位，可通过 --criteria 在创建时直接设置筛选条件。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
同一工作表可以创建多个筛选视图。

--criteria 为可选的 JSON 数组，每个元素包含 column（列偏移量，从 0 开始）和筛选条件字段。
筛选条件支持三种类型：
  values    按值筛选，通过 visibleValues 指定允许显示的值列表
  condition 按条件筛选，通过 conditions 指定条件运算符和比较值
  color     按颜色筛选，通过 backgroundColor 或 fontColor 指定颜色`,
		Example: `  # 创建不带筛选条件的筛选视图
  dws sheet filter-view create --node NODE_ID --sheet-id SHEET_ID --name "我的视图" --range "A1:E10"

  # 创建带按值筛选条件的筛选视图
  dws sheet filter-view create --node NODE_ID --sheet-id SHEET_ID --name "销售筛选" --range "A1:E10" \
    --criteria '[{"column":0,"filterType":"values","visibleValues":["销售部"]}]'

  # 创建带按条件筛选的筛选视图（大于等于 200000）
  dws sheet filter-view create --node NODE_ID --sheet-id SHEET_ID --name "高预算" --range "A1:C10" \
    --criteria '[{"column":1,"filterType":"condition","conditions":[{"operator":"greater-equal","value":"200000"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":  mustGetFlag(cmd, "node"),
				"sheetId": mustGetFlag(cmd, "sheet-id"),
				"name":    mustGetFlag(cmd, "name"),
				"range":   mustGetFlag(cmd, "range"),
			}
			if criteriaStr, _ := cmd.Flags().GetString("criteria"); criteriaStr != "" {
				var criteria []any
				if err := json.Unmarshal([]byte(criteriaStr), &criteria); err != nil {
					return fmt.Errorf("--criteria JSON 解析失败: %w", err)
				}
				toolArgs["criteria"] = criteria
			}
			return callMCPTool("create_filter_view", toolArgs)
		},
	}
	filterViewCreateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewCreateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewCreateCmd.Flags().String("name", "", "筛选视图名称 (必填)")
	filterViewCreateCmd.Flags().String("range", "", "筛选视图范围，A1 表示法，如 A1:E10 (必填)")
	filterViewCreateCmd.Flags().String("criteria", "", "筛选条件，JSON 数组 (可选)")

	filterViewUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新筛选视图属性",
		Long: `更新钉钉电子表格中指定工作表的筛选视图属性（名称、范围和/或筛选条件）。

用途：修改已有筛选视图的名称、数据范围或筛选条件。
场景：
  - 数据区域扩展后需要扩大筛选视图范围（如从 A1:D10 扩到 A1:D100）
  - 重命名筛选视图以更好描述其用途
  - 通过 --criteria 一次性批量更新多列的筛选条件
区分：
  - update 可同时修改名称、范围和条件，适合批量更新
  - update-criteria 只能设置单列条件，适合精确控制某一列的筛选逻辑
  - update --criteria 会替换指定列的条件，未指定的列保持不变

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。

至少需要传入一个更新字段（--name / --range / --criteria）。`,
		Example: `  # 更新筛选视图名称
  dws sheet filter-view update --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --name "新名称"

  # 更新筛选视图范围
  dws sheet filter-view update --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --range "A1:F20"

  # 更新筛选条件
  dws sheet filter-view update --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --criteria '[{"column":1,"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"filterViewId": mustGetFlag(cmd, "filter-view-id"),
			}
			nameChanged := cmd.Flags().Changed("name")
			rangeChanged := cmd.Flags().Changed("range")
			criteriaChanged := cmd.Flags().Changed("criteria")
			if !nameChanged && !rangeChanged && !criteriaChanged {
				return fmt.Errorf("--name、--range、--criteria 至少需要传入一个")
			}
			if nameChanged {
				toolArgs["name"], _ = cmd.Flags().GetString("name")
			}
			if rangeChanged {
				toolArgs["range"], _ = cmd.Flags().GetString("range")
			}
			if criteriaChanged {
				criteriaStr, _ := cmd.Flags().GetString("criteria")
				var criteria []any
				if err := json.Unmarshal([]byte(criteriaStr), &criteria); err != nil {
					return fmt.Errorf("--criteria JSON 解析失败: %w", err)
				}
				toolArgs["criteria"] = criteria
			}
			return callMCPTool("update_filter_view", toolArgs)
		},
	}
	filterViewUpdateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewUpdateCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewUpdateCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")
	filterViewUpdateCmd.Flags().String("name", "", "筛选视图新名称")
	filterViewUpdateCmd.Flags().String("range", "", "筛选视图新范围，A1 表示法")
	filterViewUpdateCmd.Flags().String("criteria", "", "筛选条件，JSON 数组")

	filterViewDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除筛选视图",
		Long: `删除钉钉电子表格中指定工作表的筛选视图。

用途：永久删除一个不再需要的筛选视图及其所有筛选条件。
场景：筛选视图已过时或不再需要时，清理无用的视图。
区分：
  - delete 删除整个筛选视图（包括所有列的条件），操作不可恢复
  - delete-criteria 只删除某一列的筛选条件，视图本身保留
  - 此操作不影响全局筛选或其他筛选视图，也不影响原始数据

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。`,
		Example: `  # 获得用户确认后删除指定筛选视图
  dws sheet filter-view delete --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("delete_filter_view", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"filterViewId": mustGetFlag(cmd, "filter-view-id"),
			})
		},
	}
	filterViewDeleteCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewDeleteCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewDeleteCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")

	filterViewSetCriteriaCmd := &cobra.Command{
		Use:   "update-criteria",
		Short: "更新筛选视图列条件",
		Long: `更新钉钉电子表格中指定工作表筛选视图的某一列的筛选条件。

用途：为筛选视图的指定列创建或更新筛选条件，控制该列哪些数据行可见。
场景：
  - 只显示某些特定值的行（如"只看研发部"）→ 使用 filterType: values
  - 按数值条件筛选（如"绩效 ≥ 85"）→ 使用 filterType: condition + operator: greater-equal
  - 按文本条件筛选（如"名称包含关键字"）→ 使用 filterType: condition + operator: contains
区分：
  - update-criteria 精确控制单列条件，适合逐列设置不同的筛选逻辑
  - filter-view update --criteria 可以批量更新多列条件，适合一次性设置多列
  - delete-criteria 是 update-criteria 的逆操作，删除指定列的条件

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图，通过 --column 指定目标列。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。

--column 为列偏移量（从 0 开始），相对于筛选视图范围首列。
例如筛选视图范围为 B1:E10，则 --column 0 代表 B 列，--column 1 代表 C 列。

--filter-criteria 为该列的筛选条件 JSON 对象，支持三种筛选类型：
  values    按值筛选，通过 visibleValues 指定允许显示的值列表
  condition 按条件筛选，通过 conditions 指定条件运算符和比较值（最多 2 个）
  color     按颜色筛选，通过 backgroundColor 或 fontColor 指定颜色

condition 类型支持的 operator（必须使用 kebab-case 格式）：
  equal, not-equal, contains, not-contains,
  starts-with, not-starts-with, ends-with, not-ends-with,
  greater, greater-equal, less, less-equal

多条件之间通过 conditionOperator 指定逻辑关系："and"（且，默认）或 "or"（或）。

更新条件后会立即在该筛选视图中生效。筛选视图的条件仅影响当前视图，不影响全局筛选或其他筛选视图。`,
		Example: `  # 按值筛选：只显示"销售部"和"市场部"
  dws sheet filter-view update-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --column 0 --filter-criteria '{"filterType":"values","visibleValues":["销售部","市场部"]}'

  # 按条件筛选：大于 100
  dws sheet filter-view update-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --column 2 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater","value":"100"}]}'

  # 按条件筛选：大于等于 200000
  dws sheet filter-view update-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --column 1 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"greater-equal","value":"200000"}]}'

  # 按条件筛选：小于 100
  dws sheet filter-view update-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --column 1 --filter-criteria '{"filterType":"condition","conditions":[{"operator":"less","value":"100"}]}'

  # 多条件筛选：大于等于 60 且 小于等于 90
  dws sheet filter-view update-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --column 2 --filter-criteria '{"filterType":"condition","conditionOperator":"and","conditions":[{"operator":"greater-equal","value":"60"},{"operator":"less-equal","value":"90"}]}'

  # 按颜色筛选：背景色为红色
  dws sheet filter-view update-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID \
    --column 1 --filter-criteria '{"filterType":"color","backgroundColor":"#FF0000"}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			column, err := cmd.Flags().GetInt("column")
			if err != nil {
				return fmt.Errorf("--column 解析失败: %w", err)
			}
			if column < 0 {
				return fmt.Errorf("--column 不能为负数，当前值: %d", column)
			}
			filterCriteriaStr := mustGetFlag(cmd, "filter-criteria")
			var filterCriteria map[string]any
			if err := json.Unmarshal([]byte(filterCriteriaStr), &filterCriteria); err != nil {
				return fmt.Errorf("--filter-criteria JSON 解析失败: %w", err)
			}
			return callMCPTool("set_filter_view_criteria", map[string]any{
				"nodeId":         mustGetFlag(cmd, "node"),
				"sheetId":        mustGetFlag(cmd, "sheet-id"),
				"filterViewId":   mustGetFlag(cmd, "filter-view-id"),
				"column":         column,
				"filterCriteria": filterCriteria,
			})
		},
	}
	filterViewSetCriteriaCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewSetCriteriaCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewSetCriteriaCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")
	filterViewSetCriteriaCmd.Flags().Int("column", 0, "列偏移量，从 0 开始 (必填)")
	filterViewSetCriteriaCmd.Flags().String("filter-criteria", "", "筛选条件，JSON 对象 (必填)")

	filterViewClearCriteriaCmd := &cobra.Command{
		Use:   "delete-criteria",
		Short: "删除筛选视图列条件",
		Long: `删除钉钉电子表格中指定工作表筛选视图的某一列的筛选条件。

用途：移除筛选视图中指定列的筛选条件，使该列不再参与过滤。
场景：之前通过 update-criteria 设置了某列的筛选条件，现在需要取消该列的筛选以显示全部数据。
区分：
  - delete-criteria 只删除指定列的条件，筛选视图本身和其他列的条件保持不变
  - filter-view delete 会删除整个筛选视图（包括所有列的条件）
  - 如果指定列没有设置筛选条件，调用此命令不会报错（幂等操作）

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图，通过 --column 指定目标列。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。

--column 为列偏移量（从 0 开始），相对于筛选视图范围首列。`,
		Example: `  # 获得用户确认后删除第 1 列（A 列）的筛选条件
  dws sheet filter-view delete-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --column 0 --yes

  # 获得用户确认后删除第 3 列（C 列）的筛选条件
  dws sheet filter-view delete-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --column 2 --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			column, err := cmd.Flags().GetInt("column")
			if err != nil {
				return fmt.Errorf("--column 解析失败: %w", err)
			}
			if column < 0 {
				return fmt.Errorf("--column 不能为负数，当前值: %d", column)
			}
			return callMCPTool("clear_filter_view_criteria", map[string]any{
				"nodeId":       mustGetFlag(cmd, "node"),
				"sheetId":      mustGetFlag(cmd, "sheet-id"),
				"filterViewId": mustGetFlag(cmd, "filter-view-id"),
				"column":       column,
			})
		},
	}
	filterViewClearCriteriaCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewClearCriteriaCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewClearCriteriaCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")
	filterViewClearCriteriaCmd.Flags().Int("column", 0, "列偏移量，从 0 开始 (必填)")

	filterViewInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "获取单个筛选视图详情",
		Long: `获取钉钉电子表格中指定工作表的某一个筛选视图的详细信息。

用途：查看指定筛选视图的名称、范围和筛选条件等完整信息。
场景：在修改或删除筛选视图前，先查看其当前配置；或确认 update-criteria 后条件是否生效。
实现：内部调用 get_filter_views 获取全部列表，按 --filter-view-id 过滤出目标视图返回。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。`,
		Example: `  # 查看指定筛选视图的详情
  dws sheet filter-view info --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID`,
		RunE: runFilterViewInfo,
	}
	filterViewInfoCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewInfoCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewInfoCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")

	filterViewListCriteriaCmd := &cobra.Command{
		Use:   "list-criteria",
		Short: "列出筛选视图所有列条件",
		Long: `列出钉钉电子表格中指定筛选视图的所有列筛选条件。

用途：查看某个筛选视图当前已设置了哪些列的筛选条件，包括每列的条件类型和具体规则。
场景：在管理筛选条件（修改/删除特定列条件）前，先了解当前视图设置了哪些条件。
实现：内部调用 get_filter_views 获取视图详情，提取其中的 criteria 字段返回。

如果该视图没有设置任何筛选条件，返回空对象 {}。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。`,
		Example: `  # 列出筛选视图的所有条件
  dws sheet filter-view list-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID`,
		RunE: runFilterViewListCriteria,
	}
	filterViewListCriteriaCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewListCriteriaCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewListCriteriaCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")

	filterViewGetCriteriaCmd := &cobra.Command{
		Use:   "get-criteria",
		Short: "获取单列筛选条件",
		Long: `获取钉钉电子表格中指定筛选视图某一列的筛选条件详情。

用途：查看某个筛选视图中指定列当前设置的筛选条件。
场景：在修改某列条件前，先查看其当前配置是否符合预期。
实现：内部调用 get_filter_views 获取视图详情，提取指定列的条件返回。

--column 为列偏移量（从 0 开始），相对于筛选视图范围首列。
例如筛选视图范围为 B1:E10，则 --column 0 代表 B 列，--column 1 代表 C 列。

如果该列没有设置筛选条件，返回错误提示。

通过 --node 定位电子表格文档，通过 --sheet-id 定位工作表，通过 --filter-view-id 定位筛选视图。
nodeId 支持传入文档链接 URL 或文档 ID（dentryUuid），系统自动识别。
sheetId 支持传入工作表 ID 或工作表名称，可通过 sheet list 获取。
filterViewId 可通过 filter-view list 获取。`,
		Example: `  # 查看第 1 列（偏移量 0）的筛选条件
  dws sheet filter-view get-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --column 0

  # 查看第 3 列（偏移量 2）的筛选条件
  dws sheet filter-view get-criteria --node NODE_ID --sheet-id SHEET_ID --filter-view-id FV_ID --column 2`,
		RunE: runFilterViewGetCriteria,
	}
	filterViewGetCriteriaCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	filterViewGetCriteriaCmd.Flags().String("sheet-id", "", "工作表 ID 或名称 (必填)")
	filterViewGetCriteriaCmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")
	filterViewGetCriteriaCmd.Flags().Int("column", 0, "列偏移量，从 0 开始 (必填)")

	filterViewCmd.AddCommand(filterViewListCmd, filterViewCreateCmd, filterViewUpdateCmd,
		filterViewDeleteCmd, filterViewSetCriteriaCmd, filterViewClearCriteriaCmd,
		filterViewInfoCmd, filterViewListCriteriaCmd, filterViewGetCriteriaCmd)
	return filterViewCmd
}

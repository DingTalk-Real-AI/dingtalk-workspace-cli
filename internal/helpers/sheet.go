// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return sheetHandler{}
	})
}

type sheetHandler struct{}

func (sheetHandler) Name() string {
	return "sheet"
}

func (sheetHandler) Command(runner executor.Runner) *cobra.Command {
	root := newSheetGroup("sheet", "钉钉电子表格")
	root.Long = "管理钉钉在线电子表格：文档与工作表管理、单元格读写、CSV、行列、样式、筛选、条件格式和图片。"

	rangeCmd := newSheetGroup("range", "数据区域操作")
	rangeCmd.AddCommand(
		newSheetRangeReadCommand(runner),
		newSheetRangeUpdateCommand(runner),
		newSheetRangeClearCommand(runner),
		newSheetRangeSortCommand(runner),
		newSheetRangeFillCommand(runner),
		newSheetRangeCopyCommand(runner),
		newSheetRangeMoveCommand(runner),
		newSheetRangeSetStyleCommand(runner),
		newSheetRangeBatchSetStyleCommand(runner),
	)

	filterCmd := newSheetGroup("filter", "全局筛选管理")
	filterCmd.AddCommand(
		newSheetFilterGetCommand(runner),
		newSheetFilterCreateCommand(runner),
		newSheetFilterDeleteCommand(runner),
		newSheetFilterUpdateCommand(runner),
		newSheetFilterClearCriteriaCommand(runner),
		newSheetFilterSortCommand(runner),
	)

	filterViewCmd := newSheetGroup("filter-view", "筛选视图管理")
	filterViewCmd.AddCommand(
		newSheetFilterViewListCommand(runner),
		newSheetFilterViewCreateCommand(runner),
		newSheetFilterViewUpdateCommand(runner),
		newSheetFilterViewDeleteCommand(runner),
		newSheetFilterViewUpdateCriteriaCommand(runner),
		newSheetFilterViewDeleteCriteriaCommand(runner),
		newSheetFilterViewInfoCommand(runner),
		newSheetFilterViewListCriteriaCommand(runner),
		newSheetFilterViewGetCriteriaCommand(runner),
	)

	condFormatCmd := newSheetGroup("cond-format", "条件格式管理")
	condFormatCmd.AddCommand(
		newSheetCondFormatListCommand(runner),
		newSheetCondFormatCreateCommand(runner),
		newSheetCondFormatUpdateCommand(runner),
		newSheetCondFormatDeleteCommand(runner),
	)

	root.AddCommand(
		newSheetCreateCommand(runner),
		newSheetListCommand(runner),
		newSheetInfoCommand(runner),
		newSheetNewCommand(runner),
		newSheetUpdateCommand(runner),
		newSheetCopyCommand(runner),
		newSheetDeleteSheetCommand(runner),
		rangeCmd,
		newSheetFindCommand(runner),
		newSheetAppendCommand(runner),
		newSheetCSVPutCommand(runner),
		newSheetCSVGetCommand(runner),
		newSheetInsertDimensionCommand(runner),
		newSheetDeleteDimensionCommand(runner),
		newSheetUpdateDimensionCommand(runner),
		newSheetMoveDimensionCommand(runner),
		newSheetAddDimensionCommand(runner),
		newSheetMergeCellsCommand(runner),
		newSheetUnmergeCellsCommand(runner),
		newSheetSetDropdownCommand(runner),
		newSheetGetDropdownCommand(runner),
		newSheetDeleteDropdownCommand(runner),
		newSheetMediaUploadCommand(runner),
		newSheetWriteImageCommand(runner),
		newSheetReplaceCommand(runner),
		filterCmd,
		filterViewCmd,
		condFormatCmd,
		newSheetCreateFloatImageCommand(runner),
		newSheetGetFloatImageCommand(runner),
		newSheetListFloatImagesCommand(runner),
		newSheetUpdateFloatImageCommand(runner),
		newSheetDeleteFloatImageCommand(runner),
		newSheetExportCommand(runner),
	)
	return root
}

func newSheetCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("create", "创建钉钉表格文档", func(cmd *cobra.Command, _ []string) error {
		name, err := sheetRequiredFlag(cmd, "name")
		if err != nil {
			return err
		}
		params := map[string]any{"name": name}
		sheetAddStringParam(cmd, params, "folderId", "folder")
		sheetAddStringParam(cmd, params, "workspaceId", "workspace", "workspace-id")
		return runSheetTool(cmd, runner, "create_workspace_sheet", params)
	})
	cmd.Flags().String("name", "", "表格名称 (必填)")
	cmd.Flags().String("folder", "", "目标文件夹 ID 或 URL")
	cmd.Flags().String("workspace", "", "目标知识库 ID")
	addSheetHiddenStringFlag(cmd, "workspace-id", "--workspace alias")
	return cmd
}

func newSheetListCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("list", "获取全部工作表列表", func(cmd *cobra.Command, _ []string) error {
		node, err := sheetRequiredFlag(cmd, "node")
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "get_all_sheets", map[string]any{"nodeId": node})
	})
	addSheetNodeFlags(cmd)
	return cmd
}

func newSheetInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("info", "获取指定工作表详情", func(cmd *cobra.Command, _ []string) error {
		node, err := sheetRequiredFlag(cmd, "node")
		if err != nil {
			return err
		}
		params := map[string]any{"nodeId": node}
		sheetAddStringParam(cmd, params, "sheetId", "sheet-id")
		return runSheetTool(cmd, runner, "get_sheet", params)
	})
	addSheetNodeFlags(cmd)
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称")
	return cmd
}

func newSheetNewCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("new", "新建工作表", func(cmd *cobra.Command, _ []string) error {
		node, name, err := sheetNodeAndName(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "create_sheet", map[string]any{"nodeId": node, "name": name})
	})
	addSheetNodeFlags(cmd)
	cmd.Flags().String("name", "", "工作表名称 (必填)")
	return cmd
}

func newSheetUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update", "更新工作表属性", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		changed := false
		if cmd.Flags().Changed("name") || cmd.Flags().Changed("title") {
			params["title"] = sheetNameFlag(cmd)
			changed = true
		}
		if cmd.Flags().Changed("index") {
			index, _ := cmd.Flags().GetInt("index")
			if index < 0 {
				return apperrors.NewValidation("--index must be >= 0")
			}
			params["index"] = index
			changed = true
		}
		if cmd.Flags().Changed("hidden") {
			v, _ := cmd.Flags().GetBool("hidden")
			params["hidden"] = v
			changed = true
		}
		if cmd.Flags().Changed("frozen-row-count") {
			v, _ := cmd.Flags().GetInt("frozen-row-count")
			if v < 0 {
				return apperrors.NewValidation("--frozen-row-count must be >= 0")
			}
			params["frozenRowCount"] = v
			changed = true
		}
		if cmd.Flags().Changed("frozen-column-count") {
			v, _ := cmd.Flags().GetInt("frozen-column-count")
			if v < 0 {
				return apperrors.NewValidation("--frozen-column-count must be >= 0")
			}
			params["frozenColumnCount"] = v
			changed = true
		}
		if !changed {
			return apperrors.NewValidation("at least one of --name, --index, --hidden, --frozen-row-count or --frozen-column-count is required")
		}
		return runSheetTool(cmd, runner, "update_sheet", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("name", "", "工作表新名称")
	cmd.Flags().String("title", "", "--name alias")
	_ = cmd.Flags().MarkHidden("title")
	cmd.Flags().Int("index", 0, "工作表新位置索引，0-based")
	cmd.Flags().Bool("hidden", false, "是否隐藏工作表")
	cmd.Flags().Int("frozen-row-count", 0, "冻结行数")
	cmd.Flags().Int("frozen-column-count", 0, "冻结列数")
	return cmd
}

func newSheetCopyCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("copy", "复制工作表", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("name") || cmd.Flags().Changed("title") {
			params["title"] = sheetNameFlag(cmd)
		}
		if cmd.Flags().Changed("index") {
			index, _ := cmd.Flags().GetInt("index")
			if index < 0 {
				return apperrors.NewValidation("--index must be >= 0")
			}
			params["index"] = index
		}
		return runSheetTool(cmd, runner, "copy_sheet", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("name", "", "副本名称")
	cmd.Flags().String("title", "", "--name alias")
	_ = cmd.Flags().MarkHidden("title")
	cmd.Flags().Int("index", 0, "副本位置索引，0-based")
	return cmd
}

func newSheetDeleteSheetCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("delete-sheet", "删除工作表", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "delete_sheet", params)
	})
	addSheetBaseFlags(cmd)
	return cmd
}

func newSheetRangeReadCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("read", "读取工作表数据", func(cmd *cobra.Command, _ []string) error {
		node, err := sheetRequiredFlag(cmd, "node")
		if err != nil {
			return err
		}
		params := map[string]any{"nodeId": node}
		sheetAddStringParam(cmd, params, "sheetId", "sheet-id")
		sheetAddStringParam(cmd, params, "range", "range")
		sheetAddStringParam(cmd, params, "valueRenderOption", "value-render-option")
		return runSheetTool(cmd, runner, "get_cell_infos", params)
	})
	cmd.Aliases = []string{"get"}
	addSheetNodeFlags(cmd)
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称")
	cmd.Flags().String("range", "", "读取范围，A1 表示法")
	cmd.Flags().String("value-render-option", "", "formatted_value | raw_value | formula")
	return cmd
}

func newSheetRangeUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update", "更新工作表指定区域内容", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "range", "values"); err != nil {
			return err
		}
		var cells [][]any
		if err := sheetParseJSONFlag(cmd, "values", &cells); err != nil {
			return err
		}
		for i, row := range cells {
			for j, cell := range row {
				if cell == nil {
					return apperrors.NewValidation(fmt.Sprintf("--values[%d][%d] must be an object, not null", i, j))
				}
				if _, ok := cell.(map[string]any); !ok {
					return apperrors.NewValidation(fmt.Sprintf("--values[%d][%d] must be an object", i, j))
				}
			}
		}
		return runSheetTool(cmd, runner, "set_cell_range", map[string]any{
			"nodeId":       sheetStringFlag(cmd, "node"),
			"sheetId":      sheetStringFlag(cmd, "sheet-id"),
			"rangeAddress": sheetStringFlag(cmd, "range"),
			"cells":        cells,
		})
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "目标单元格区域地址 (必填)")
	cmd.Flags().String("values", "", "单元格内容二维 JSON 数组 (必填)")
	return cmd
}

func newSheetRangeClearCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("clear", "清除工作表指定区域", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "range"); err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"range":   sheetStringFlag(cmd, "range"),
		}
		sheetAddStringParam(cmd, params, "type", "type")
		return runSheetTool(cmd, runner, "clear_range", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "清除范围，A1 表示法 (必填)")
	cmd.Flags().String("type", "", "content | format | all")
	return cmd
}

func newSheetRangeSortCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("sort", "对工作表指定区域排序", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "range", "sort-keys"); err != nil {
			return err
		}
		var sortKeys []any
		if err := sheetParseJSONFlag(cmd, "sort-keys", &sortKeys); err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":   sheetStringFlag(cmd, "node"),
			"sheetId":  sheetStringFlag(cmd, "sheet-id"),
			"range":    sheetStringFlag(cmd, "range"),
			"sortKeys": sortKeys,
		}
		if v, _ := cmd.Flags().GetBool("has-header"); v {
			params["hasHeader"] = true
		}
		return runSheetTool(cmd, runner, "sort_range", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "排序范围，A1 表示法 (必填)")
	cmd.Flags().String("sort-keys", "", "排序规则 JSON 数组 (必填)")
	cmd.Flags().Bool("has-header", false, "首行是否为表头")
	return cmd
}

func newSheetRangeFillCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("fill", "自动填充工作表指定区域", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "source-range", "target-range"); err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":           sheetStringFlag(cmd, "node"),
			"sheetId":          sheetStringFlag(cmd, "sheet-id"),
			"sourceRange":      sheetStringFlag(cmd, "source-range"),
			"destinationRange": sheetStringFlag(cmd, "target-range"),
		}
		sheetAddStringParam(cmd, params, "fillType", "fill-type")
		return runSheetTool(cmd, runner, "fill_range", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("source-range", "", "源数据范围，A1 表示法 (必填)")
	cmd.Flags().String("target-range", "", "目标填充范围，A1 表示法 (必填)")
	cmd.Flags().String("fill-type", "", "copy | onlystyle | withoutstyle")
	return cmd
}

func newSheetRangeCopyCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("copy-to", "复制工作表指定区域到目标位置", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetRangeTransferParams(cmd)
		if err != nil {
			return err
		}
		sheetAddStringParam(cmd, params, "pasteType", "paste-type")
		return runSheetTool(cmd, runner, "copy_range", params)
	})
	addSheetRangeTransferFlags(cmd)
	cmd.Flags().String("paste-type", "", "values | formulas | formats | all")
	return cmd
}

func newSheetRangeMoveCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("move-to", "移动工作表指定区域到目标位置", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetRangeTransferParams(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "move_range", params)
	})
	addSheetRangeTransferFlags(cmd)
	return cmd
}

func newSheetFindCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("find", "在工作表中搜索单元格内容", func(cmd *cobra.Command, _ []string) error {
		query, err := sheetRequiredFlagOrFallback(cmd, "query", "find")
		if err != nil {
			return err
		}
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		params["text"] = query
		sheetAddStringParam(cmd, params, "range", "range")
		for flag, key := range map[string]string{
			"match-case":        "matchCase",
			"match-entire-cell": "matchEntireCell",
			"use-regexp":        "useRegExp",
			"match-formula":     "matchFormulaText",
			"include-hidden":    "includeHidden",
		} {
			v, _ := cmd.Flags().GetBool(flag)
			params[key] = v
		}
		return runSheetTool(cmd, runner, "find_cells", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("query", "", "搜索文本 (必填)")
	cmd.Flags().String("find", "", "--query alias")
	_ = cmd.Flags().MarkHidden("find")
	cmd.Flags().String("range", "", "搜索范围，A1 表示法")
	cmd.Flags().Bool("match-case", true, "区分大小写")
	cmd.Flags().Bool("match-entire-cell", false, "完整单元格匹配")
	cmd.Flags().Bool("use-regexp", false, "启用正则表达式搜索")
	cmd.Flags().Bool("match-formula", false, "搜索公式文本")
	cmd.Flags().Bool("include-hidden", false, "包含隐藏单元格")
	return cmd
}

func newSheetAppendCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("append", "在工作表末尾追加数据", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "values"); err != nil {
			return err
		}
		var values [][]any
		if err := sheetParseJSONFlag(cmd, "values", &values); err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "append_rows", map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"values":  values,
		})
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("values", "", "追加数据二维 JSON 数组 (必填)")
	return cmd
}

func newSheetCSVPutCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("csv-put", "将 CSV 数据写入表格指定位置", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "csv", "start-cell"); err != nil {
			return err
		}
		csvContent := sheetStringFlag(cmd, "csv")
		switch {
		case csvContent == "-":
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin failed: %w", err)
			}
			csvContent = string(data)
		case strings.HasPrefix(csvContent, "@"):
			data, err := os.ReadFile(strings.TrimPrefix(csvContent, "@"))
			if err != nil {
				return fmt.Errorf("read CSV file failed: %w", err)
			}
			csvContent = string(data)
		}
		csvContent = strings.ReplaceAll(csvContent, "\r", "")
		csvContent = strings.TrimPrefix(csvContent, "\xef\xbb\xbf")
		params := map[string]any{
			"nodeId":    sheetStringFlag(cmd, "node"),
			"sheetId":   sheetStringFlag(cmd, "sheet-id"),
			"csv":       csvContent,
			"startCell": sheetStringFlag(cmd, "start-cell"),
		}
		if cmd.Flags().Changed("allow-overwrite") {
			v, _ := cmd.Flags().GetBool("allow-overwrite")
			params["allowOverwrite"] = v
		}
		return runSheetTool(cmd, runner, "set_range_from_csv", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("csv", "", "CSV 文本、@文件路径 或 - (必填)")
	cmd.Flags().String("start-cell", "", "起始单元格，A1 表示法 (必填)")
	cmd.Flags().Bool("allow-overwrite", false, "允许覆盖已有数据")
	return cmd
}

func newSheetCSVGetCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("csv-get", "以 CSV 格式读取工作表数据", func(cmd *cobra.Command, _ []string) error {
		node, err := sheetRequiredFlag(cmd, "node")
		if err != nil {
			return err
		}
		params := map[string]any{"nodeId": node}
		sheetAddStringParam(cmd, params, "sheetId", "sheet-id")
		sheetAddStringParam(cmd, params, "range", "range")
		sheetAddStringParam(cmd, params, "valueRenderOption", "value-render-option")
		if cmd.Flags().Changed("max-chars") {
			v, _ := cmd.Flags().GetInt("max-chars")
			params["maxChars"] = v
		}
		return runSheetTool(cmd, runner, "get_range_as_csv", params)
	})
	addSheetNodeFlags(cmd)
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称")
	cmd.Flags().String("range", "", "读取范围，A1 表示法")
	cmd.Flags().String("value-render-option", "", "formatted_value | raw_value | formula")
	cmd.Flags().Int("max-chars", 0, "CSV 最大字符数")
	return cmd
}

func newSheetInsertDimensionCommand(runner executor.Runner) *cobra.Command {
	return newSheetDimensionPositionCommand(runner, "insert-dimension", "在指定位置插入行或列", "insert_dimension")
}

func newSheetDeleteDimensionCommand(runner executor.Runner) *cobra.Command {
	return newSheetDimensionPositionCommand(runner, "delete-dimension", "删除指定位置的行或列", "delete_dimension")
}

func newSheetDimensionPositionCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := newSheetLeaf(use, short, func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "dimension", "position", "length"); err != nil {
			return err
		}
		dimension, err := sheetDimension(cmd)
		if err != nil {
			return err
		}
		length, err := sheetPositiveIntStringFlag(cmd, "length", 5000)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, tool, map[string]any{
			"nodeId":    sheetStringFlag(cmd, "node"),
			"sheetId":   sheetStringFlag(cmd, "sheet-id"),
			"dimension": dimension,
			"position":  sheetStringFlag(cmd, "position"),
			"length":    length,
		})
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("dimension", "", "ROWS 或 COLUMNS (必填)")
	cmd.Flags().String("position", "", "位置，A1 表示法 (必填)")
	cmd.Flags().String("length", "", "数量，正整数 (必填)")
	return cmd
}

func newSheetUpdateDimensionCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update-dimension", "更新指定范围行/列属性", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "dimension", "start-index", "length"); err != nil {
			return err
		}
		dimension, err := sheetDimension(cmd)
		if err != nil {
			return err
		}
		length, err := sheetPositiveIntStringFlag(cmd, "length", 5000)
		if err != nil {
			return err
		}
		hiddenChanged := cmd.Flags().Changed("hidden")
		pixelChanged := cmd.Flags().Changed("pixel-size")
		if !hiddenChanged && !pixelChanged {
			return apperrors.NewValidation("at least one of --hidden or --pixel-size is required")
		}
		params := map[string]any{
			"nodeId":     sheetStringFlag(cmd, "node"),
			"sheetId":    sheetStringFlag(cmd, "sheet-id"),
			"dimension":  dimension,
			"startIndex": sheetStringFlag(cmd, "start-index"),
			"length":     length,
		}
		if hiddenChanged {
			v, _ := cmd.Flags().GetBool("hidden")
			params["hidden"] = v
		}
		if pixelChanged {
			v, _ := cmd.Flags().GetInt("pixel-size")
			if v < 0 {
				return apperrors.NewValidation("--pixel-size must be >= 0")
			}
			params["pixelSize"] = v
		}
		return runSheetTool(cmd, runner, "update_dimension", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("dimension", "", "ROWS 或 COLUMNS (必填)")
	cmd.Flags().String("start-index", "", "起始位置，A1 表示法 (必填)")
	cmd.Flags().String("length", "", "数量，正整数 (必填)")
	cmd.Flags().Bool("hidden", false, "是否隐藏")
	cmd.Flags().Int("pixel-size", 0, "行高或列宽")
	return cmd
}

func newSheetMoveDimensionCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("move-dimension", "移动行或列到指定位置", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "dimension", "start-index", "end-index", "destination-index"); err != nil {
			return err
		}
		dimension, err := sheetDimension(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "move_dimension", map[string]any{
			"nodeId":           sheetStringFlag(cmd, "node"),
			"sheetId":          sheetStringFlag(cmd, "sheet-id"),
			"dimension":        dimension,
			"startIndex":       sheetStringFlag(cmd, "start-index"),
			"endIndex":         sheetStringFlag(cmd, "end-index"),
			"destinationIndex": sheetStringFlag(cmd, "destination-index"),
		})
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("dimension", "", "ROWS 或 COLUMNS (必填)")
	cmd.Flags().String("start-index", "", "源起始位置 (必填)")
	cmd.Flags().String("end-index", "", "源结束位置 (必填)")
	cmd.Flags().String("destination-index", "", "目标位置 (必填)")
	return cmd
}

func newSheetAddDimensionCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("add-dimension", "在末尾追加空行或空列", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "dimension"); err != nil {
			return err
		}
		dimension, err := sheetDimension(cmd)
		if err != nil {
			return err
		}
		length, _ := cmd.Flags().GetInt("length")
		if length < 1 || length > 5000 {
			return apperrors.NewValidation("--length must be between 1 and 5000")
		}
		return runSheetTool(cmd, runner, "add_dimension", map[string]any{
			"nodeId":    sheetStringFlag(cmd, "node"),
			"sheetId":   sheetStringFlag(cmd, "sheet-id"),
			"dimension": dimension,
			"length":    length,
		})
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("dimension", "", "ROWS 或 COLUMNS (必填)")
	cmd.Flags().Int("length", 0, "追加数量，正整数 (必填)")
	return cmd
}

func newSheetMergeCellsCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("merge-cells", "合并单元格", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetRangeAddressParams(cmd)
		if err != nil {
			return err
		}
		sheetAddStringParam(cmd, params, "mergeType", "merge-type")
		return runSheetTool(cmd, runner, "merge_cells", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "目标单元格区域地址 (必填)")
	cmd.Flags().String("merge-type", "", "mergeAll | mergeRows | mergeColumns")
	return cmd
}

func newSheetUnmergeCellsCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("unmerge-cells", "取消合并单元格", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetRangeAddressParams(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "unmerge_range", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "取消合并的范围 (必填)")
	return cmd
}

func newSheetSetDropdownCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("set-dropdown", "设置下拉列表", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "range", "options"); err != nil {
			return err
		}
		var options []map[string]any
		if err := sheetParseJSONFlag(cmd, "options", &options); err != nil {
			return err
		}
		if len(options) == 0 {
			return apperrors.NewValidation("--options must contain at least one item")
		}
		params := map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"range":   sheetStringFlag(cmd, "range"),
			"options": options,
		}
		if v, _ := cmd.Flags().GetBool("multi-select"); v {
			params["enableMultiSelect"] = true
		}
		return runSheetTool(cmd, runner, "set_dropdown_lists", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "目标单元格范围 (必填)")
	cmd.Flags().String("options", "", "下拉选项 JSON 数组 (必填)")
	cmd.Flags().Bool("multi-select", false, "是否允许多选")
	return cmd
}

func newSheetGetDropdownCommand(runner executor.Runner) *cobra.Command {
	return newSheetRangeToolCommand(runner, "get-dropdown", "获取下拉列表配置", "get_dropdown_lists", "range")
}

func newSheetDeleteDropdownCommand(runner executor.Runner) *cobra.Command {
	return newSheetRangeToolCommand(runner, "delete-dropdown", "删除下拉列表", "delete_dropdown_lists", "range")
}

func newSheetReplaceCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("replace", "查找替换文本", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "find"); err != nil {
			return err
		}
		if !cmd.Flags().Changed("replacement") {
			return apperrors.NewValidation("--replacement is required")
		}
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		params["text"] = sheetStringFlag(cmd, "find")
		params["replaceText"] = sheetStringFlag(cmd, "replacement")
		sheetAddStringParam(cmd, params, "range", "range")
		for flag, key := range map[string]string{
			"match-case":        "matchCase",
			"match-entire-cell": "matchEntireCell",
			"use-regexp":        "useRegExp",
			"include-hidden":    "includeHidden",
		} {
			v, _ := cmd.Flags().GetBool(flag)
			params[key] = v
		}
		return runSheetTool(cmd, runner, "replace_all", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("find", "", "查找文本 (必填)")
	cmd.Flags().String("replacement", "", "替换文本 (必填，可为空)")
	cmd.Flags().String("range", "", "替换范围，A1 表示法")
	cmd.Flags().Bool("match-case", false, "区分大小写")
	cmd.Flags().Bool("match-entire-cell", false, "完整单元格匹配")
	cmd.Flags().Bool("use-regexp", false, "启用正则表达式匹配")
	cmd.Flags().Bool("include-hidden", false, "包含隐藏行/列")
	return cmd
}

func newSheetFilterGetCommand(runner executor.Runner) *cobra.Command {
	return newSheetBaseToolCommand(runner, "get", "获取全局筛选信息", "get_filter")
}

func newSheetFilterDeleteCommand(runner executor.Runner) *cobra.Command {
	return newSheetBaseToolCommand(runner, "delete", "删除全局筛选", "delete_filter")
}

func newSheetFilterCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("create", "创建全局筛选", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "range"); err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"range":   sheetStringFlag(cmd, "range"),
		}
		if v := sheetStringFlag(cmd, "criteria"); v != "" {
			var criteria []any
			if err := json.Unmarshal([]byte(v), &criteria); err != nil {
				return fmt.Errorf("--criteria JSON parse failed: %w", err)
			}
			params["criteria"] = criteria
		}
		return runSheetTool(cmd, runner, "create_filter", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "筛选范围，A1 表示法 (必填)")
	cmd.Flags().String("criteria", "", "筛选条件 JSON 数组")
	return cmd
}

func newSheetFilterUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update", "批量更新筛选条件", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "criteria"); err != nil {
			return err
		}
		var criteria []any
		if err := sheetParseJSONFlag(cmd, "criteria", &criteria); err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "update_filter", map[string]any{
			"nodeId":   sheetStringFlag(cmd, "node"),
			"sheetId":  sheetStringFlag(cmd, "sheet-id"),
			"criteria": criteria,
		})
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("criteria", "", "筛选条件 JSON 数组 (必填)")
	return cmd
}

func newSheetFilterClearCriteriaCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetColumnCommand(runner, "clear-criteria", "清除单列筛选条件", "clear_filter_criteria")
	return cmd
}

func newSheetFilterSortCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("sort", "筛选排序", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		column, _ := cmd.Flags().GetInt("column")
		ascending, _ := cmd.Flags().GetBool("ascending")
		params["field"] = map[string]any{"column": column, "ascending": ascending}
		return runSheetTool(cmd, runner, "sort_filter", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().Int("column", 0, "排序列偏移量，从 0 开始")
	cmd.Flags().Bool("ascending", true, "是否升序")
	return cmd
}

func newSheetFilterViewListCommand(runner executor.Runner) *cobra.Command {
	return newSheetBaseToolCommand(runner, "list", "获取所有筛选视图", "get_filter_views")
}

func newSheetFilterViewCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("create", "创建筛选视图", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "name", "range"); err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"name":    sheetStringFlag(cmd, "name"),
			"range":   sheetStringFlag(cmd, "range"),
		}
		if v := sheetStringFlag(cmd, "criteria"); v != "" {
			var criteria []any
			if err := json.Unmarshal([]byte(v), &criteria); err != nil {
				return fmt.Errorf("--criteria JSON parse failed: %w", err)
			}
			params["criteria"] = criteria
		}
		return runSheetTool(cmd, runner, "create_filter_view", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("name", "", "筛选视图名称 (必填)")
	cmd.Flags().String("range", "", "筛选视图范围，A1 表示法 (必填)")
	cmd.Flags().String("criteria", "", "筛选条件 JSON 数组")
	return cmd
}

func newSheetFilterViewUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update", "更新筛选视图属性", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "filter-view-id"); err != nil {
			return err
		}
		params := sheetFilterViewBaseParams(cmd)
		changed := false
		if cmd.Flags().Changed("name") {
			params["name"] = sheetStringFlag(cmd, "name")
			changed = true
		}
		if cmd.Flags().Changed("range") {
			params["range"] = sheetStringFlag(cmd, "range")
			changed = true
		}
		if cmd.Flags().Changed("criteria") {
			var criteria []any
			if err := sheetParseJSONFlag(cmd, "criteria", &criteria); err != nil {
				return err
			}
			params["criteria"] = criteria
			changed = true
		}
		if !changed {
			return apperrors.NewValidation("at least one of --name, --range or --criteria is required")
		}
		return runSheetTool(cmd, runner, "update_filter_view", params)
	})
	addSheetFilterViewBaseFlags(cmd)
	cmd.Flags().String("name", "", "筛选视图新名称")
	cmd.Flags().String("range", "", "筛选视图新范围")
	cmd.Flags().String("criteria", "", "筛选条件 JSON 数组")
	return cmd
}

func newSheetFilterViewDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("delete", "删除筛选视图", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "filter-view-id"); err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "delete_filter_view", sheetFilterViewBaseParams(cmd))
	})
	addSheetFilterViewBaseFlags(cmd)
	return cmd
}

func newSheetFilterViewUpdateCriteriaCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update-criteria", "更新筛选视图列条件", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "filter-view-id", "filter-criteria"); err != nil {
			return err
		}
		params := sheetFilterViewBaseParams(cmd)
		column, _ := cmd.Flags().GetInt("column")
		if column < 0 {
			return apperrors.NewValidation("--column must be >= 0")
		}
		var filterCriteria map[string]any
		if err := sheetParseJSONFlag(cmd, "filter-criteria", &filterCriteria); err != nil {
			return err
		}
		params["column"] = column
		params["filterCriteria"] = filterCriteria
		return runSheetTool(cmd, runner, "set_filter_view_criteria", params)
	})
	addSheetFilterViewBaseFlags(cmd)
	cmd.Flags().Int("column", 0, "列偏移量，从 0 开始")
	cmd.Flags().String("filter-criteria", "", "筛选条件 JSON 对象 (必填)")
	return cmd
}

func newSheetFilterViewDeleteCriteriaCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetColumnCommand(runner, "delete-criteria", "删除筛选视图列条件", "clear_filter_view_criteria")
	cmd.Flags().String("filter-view-id", "", "筛选视图 ID (必填)")
	return cmd
}

func newSheetFilterViewInfoCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetFilterViewReadCommand(runner, "info", "获取单个筛选视图详情", "info")
	return cmd
}

func newSheetFilterViewListCriteriaCommand(runner executor.Runner) *cobra.Command {
	return newSheetFilterViewReadCommand(runner, "list-criteria", "列出筛选视图所有列条件", "list-criteria")
}

func newSheetFilterViewGetCriteriaCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetFilterViewReadCommand(runner, "get-criteria", "获取单列筛选条件", "get-criteria")
	cmd.Flags().Int("column", 0, "列偏移量，从 0 开始")
	return cmd
}

func newSheetFilterViewReadCommand(runner executor.Runner, use, short, mode string) *cobra.Command {
	cmd := newSheetLeaf(use, short, func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "filter-view-id"); err != nil {
			return err
		}
		result, err := sheetInvocationResult(cmd, runner, "sheet", "get_filter_views", map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
		})
		if err != nil {
			return err
		}
		filterViews := sheetResultFilterViews(result.Response)
		if len(filterViews) == 0 {
			return writeCommandPayload(cmd, result)
		}
		view, err := sheetFindFilterView(filterViews, sheetStringFlag(cmd, "filter-view-id"))
		if err != nil {
			return err
		}
		switch mode {
		case "info":
			return writeCommandPayload(cmd, view)
		case "list-criteria":
			if criteria, ok := view["criteria"]; ok {
				return writeCommandPayload(cmd, criteria)
			}
			return writeCommandPayload(cmd, map[string]any{})
		case "get-criteria":
			column, _ := cmd.Flags().GetInt("column")
			if column < 0 {
				return apperrors.NewValidation("--column must be >= 0")
			}
			criteria, _ := view["criteria"].(map[string]any)
			if criteria == nil {
				return apperrors.NewValidation("filter view has no criteria")
			}
			item, ok := criteria[strconv.Itoa(column)]
			if !ok {
				return apperrors.NewValidation("filter view column criteria not found")
			}
			return writeCommandPayload(cmd, item)
		default:
			return writeCommandPayload(cmd, result)
		}
	})
	addSheetFilterViewBaseFlags(cmd)
	return cmd
}

func newSheetCondFormatListCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("list", "获取条件格式规则", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		sheetAddStringParam(cmd, params, "ruleId", "rule-id")
		return runSheetTool(cmd, runner, "get_cond_format", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("rule-id", "", "条件格式规则 ID")
	return cmd
}

func newSheetCondFormatCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("create", "创建条件格式规则", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetCondFormatMutationBase(cmd, false)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "create_cond_format", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("ranges", "", "应用范围 JSON 数组 (必填)")
	cmd.Flags().String("condition", "", "条件类型及参数 JSON 对象 (必填)")
	cmd.Flags().String("cell-style", "", "单元格样式 JSON 对象")
	cmd.Flags().String("data-bar-style", "", "数据条样式 JSON 对象")
	return cmd
}

func newSheetCondFormatUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update", "更新条件格式规则", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetCondFormatMutationBase(cmd, true)
		if err != nil {
			return err
		}
		if len(params) <= 3 {
			return apperrors.NewValidation("at least one of --ranges, --condition, --cell-style or --data-bar-style is required")
		}
		return runSheetTool(cmd, runner, "update_cond_format", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("rule-id", "", "条件格式规则 ID (必填)")
	cmd.Flags().String("ranges", "", "应用范围 JSON 数组")
	cmd.Flags().String("condition", "", "条件类型及参数 JSON 对象")
	cmd.Flags().String("cell-style", "", "单元格样式 JSON 对象")
	cmd.Flags().String("data-bar-style", "", "数据条样式 JSON 对象")
	return cmd
}

func newSheetCondFormatDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("delete", "删除条件格式规则", func(cmd *cobra.Command, _ []string) error {
		if ok, _ := cmd.Flags().GetBool("yes"); !ok && !commandDryRun(cmd) {
			return apperrors.NewValidation("delete cond-format requires --yes")
		}
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		ruleID, err := sheetRequiredFlag(cmd, "rule-id")
		if err != nil {
			return err
		}
		params["ruleId"] = ruleID
		return runSheetTool(cmd, runner, "delete_cond_format", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("rule-id", "", "条件格式规则 ID (必填)")
	cmd.Flags().Bool("yes", false, "确认删除")
	return cmd
}

func newSheetCreateFloatImageCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("create-float-image", "创建浮动图片", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "src", "range"); err != nil {
			return err
		}
		width, height, err := sheetPositiveSize(cmd, true)
		if err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"src":     sheetStringFlag(cmd, "src"),
			"range":   sheetStringFlag(cmd, "range"),
			"width":   width,
			"height":  height,
		}
		sheetAddChangedIntParam(cmd, params, "offsetX", "offset-x")
		sheetAddChangedIntParam(cmd, params, "offsetY", "offset-y")
		return runSheetTool(cmd, runner, "create_float_image", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("src", "", "图片资源路径 (必填)")
	cmd.Flags().String("range", "", "锚点单元格 (必填)")
	cmd.Flags().Int("width", 0, "图片宽度，像素 (必填)")
	cmd.Flags().Int("height", 0, "图片高度，像素 (必填)")
	cmd.Flags().Int("offset-x", 0, "水平偏移量")
	cmd.Flags().Int("offset-y", 0, "垂直偏移量")
	return cmd
}

func newSheetGetFloatImageCommand(runner executor.Runner) *cobra.Command {
	return newSheetFloatImageIDCommand(runner, "get-float-image", "获取浮动图片详情", "get_float_image")
}

func newSheetListFloatImagesCommand(runner executor.Runner) *cobra.Command {
	return newSheetBaseToolCommand(runner, "list-float-images", "列出工作表所有浮动图片", "list_float_images")
}

func newSheetUpdateFloatImageCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("update-float-image", "更新浮动图片属性", func(cmd *cobra.Command, _ []string) error {
		params, err := sheetFloatImageBaseParams(cmd)
		if err != nil {
			return err
		}
		changed := false
		for flag, key := range map[string]string{"src": "src", "range": "range"} {
			if cmd.Flags().Changed(flag) {
				params[key] = sheetStringFlag(cmd, flag)
				changed = true
			}
		}
		for flag, key := range map[string]string{"width": "width", "height": "height", "offset-x": "offsetX", "offset-y": "offsetY"} {
			if cmd.Flags().Changed(flag) {
				v, _ := cmd.Flags().GetInt(flag)
				if (flag == "width" || flag == "height") && v <= 0 {
					return apperrors.NewValidation("--" + flag + " must be > 0")
				}
				if (flag == "offset-x" || flag == "offset-y") && v < 0 {
					return apperrors.NewValidation("--" + flag + " must be >= 0")
				}
				params[key] = v
				changed = true
			}
		}
		if !changed {
			return apperrors.NewValidation("at least one float image field is required")
		}
		return runSheetTool(cmd, runner, "update_float_image", params)
	})
	addSheetFloatImageBaseFlags(cmd)
	cmd.Flags().String("src", "", "新的图片资源路径")
	cmd.Flags().String("range", "", "新的锚点单元格")
	cmd.Flags().Int("width", 0, "新的图片宽度")
	cmd.Flags().Int("height", 0, "新的图片高度")
	cmd.Flags().Int("offset-x", 0, "新的水平偏移量")
	cmd.Flags().Int("offset-y", 0, "新的垂直偏移量")
	return cmd
}

func newSheetDeleteFloatImageCommand(runner executor.Runner) *cobra.Command {
	return newSheetFloatImageIDCommand(runner, "delete-float-image", "删除浮动图片", "delete_float_image")
}

func newSheetMediaUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("media-upload", "上传附件到表格", func(cmd *cobra.Command, _ []string) error {
		return runSheetAttachmentUpload(cmd, runner, false)
	})
	addSheetNodeFlags(cmd)
	cmd.Flags().String("file", "", "本地文件路径 (必填)")
	cmd.Flags().String("name", "", "附件显示名称")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型")
	return cmd
}

func newSheetWriteImageCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("write-image", "上传图片并写入表格单元格", func(cmd *cobra.Command, _ []string) error {
		return runSheetAttachmentUpload(cmd, runner, true)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "目标单元格区域地址 (必填)")
	cmd.Flags().String("file", "", "本地图片文件路径 (必填)")
	cmd.Flags().String("name", "", "图片显示名称")
	cmd.Flags().String("mime-type", "", "文件 MIME 类型")
	cmd.Flags().Int("width", 0, "图片显示宽度")
	cmd.Flags().Int("height", 0, "图片显示高度")
	return cmd
}

func newSheetExportCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("export", "导出表格为 xlsx", func(cmd *cobra.Command, _ []string) error {
		node, err := sheetRequiredFlag(cmd, "node")
		if err != nil {
			return err
		}
		outputPath := sheetStringFlag(cmd, "output")
		if commandDryRun(cmd) {
			return runSheetTool(cmd, runner, "submit_export_job", map[string]any{"nodeId": node, "exportFormat": "xlsx"})
		}
		submit, err := sheetInvocationResult(cmd, runner, "sheet", "submit_export_job", map[string]any{"nodeId": node, "exportFormat": "xlsx"})
		if err != nil {
			return err
		}
		jobID := sheetStringFromResponse(submit.Response, "jobId")
		if jobID == "" {
			return writeCommandPayload(cmd, submit)
		}
		downloadURL, status, err := pollSheetExport(cmd, runner, jobID)
		if err != nil {
			return err
		}
		out := map[string]any{"jobId": jobID, "status": status}
		if downloadURL != "" {
			out["downloadUrl"] = downloadURL
		}
		if outputPath != "" && downloadURL != "" {
			if fi, statErr := os.Stat(outputPath); statErr == nil && fi.IsDir() {
				outputPath = filepath.Join(outputPath, sheetExportFilename(downloadURL, jobID))
			}
			if err := sheetHTTPGet(cmd, downloadURL, outputPath); err != nil {
				return err
			}
			out["output"] = outputPath
		}
		return writeCommandPayload(cmd, out)
	})
	addSheetNodeFlags(cmd)
	cmd.Flags().String("output", "", "本地保存路径")
	return cmd
}

func newSheetRangeToolCommand(runner executor.Runner, use, short, tool, rangeFlag string) *cobra.Command {
	cmd := newSheetLeaf(use, short, func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", rangeFlag); err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":  sheetStringFlag(cmd, "node"),
			"sheetId": sheetStringFlag(cmd, "sheet-id"),
			"range":   sheetStringFlag(cmd, rangeFlag),
		}
		return runSheetTool(cmd, runner, tool, params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String(rangeFlag, "", "范围，A1 表示法 (必填)")
	return cmd
}

func newSheetBaseToolCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := newSheetLeaf(use, short, func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, tool, params)
	})
	addSheetBaseFlags(cmd)
	return cmd
}

func newSheetColumnCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := newSheetLeaf(use, short, func(cmd *cobra.Command, _ []string) error {
		params, err := sheetBaseParams(cmd)
		if err != nil {
			return err
		}
		if strings.Contains(use, "criteria") && strings.Contains(tool, "filter_view") {
			filterViewID, err := sheetRequiredFlag(cmd, "filter-view-id")
			if err != nil {
				return err
			}
			params["filterViewId"] = filterViewID
		}
		column, _ := cmd.Flags().GetInt("column")
		if column < 0 {
			return apperrors.NewValidation("--column must be >= 0")
		}
		params["column"] = column
		return runSheetTool(cmd, runner, tool, params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().Int("column", 0, "列偏移量，从 0 开始")
	return cmd
}

func newSheetFloatImageIDCommand(runner executor.Runner, use, short, tool string) *cobra.Command {
	cmd := newSheetLeaf(use, short, func(cmd *cobra.Command, _ []string) error {
		params, err := sheetFloatImageBaseParams(cmd)
		if err != nil {
			return err
		}
		return runSheetTool(cmd, runner, tool, params)
	})
	addSheetFloatImageBaseFlags(cmd)
	return cmd
}

func newSheetGroup(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	preferLegacyLeaf(cmd)
	return cmd
}

func newSheetLeaf(use, short string, run func(*cobra.Command, []string) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE:              run,
	}
	preferLegacyLeaf(cmd)
	return cmd
}

func runSheetTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	result, err := sheetInvocationResult(cmd, runner, "sheet", tool, params)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func sheetInvocationResult(cmd *cobra.Command, runner executor.Runner, product, tool string, params map[string]any) (executor.Result, error) {
	invocation := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), product, tool, params)
	invocation.DryRun = commandDryRun(cmd)
	return runner.Run(cmd.Context(), invocation)
}

func addSheetNodeFlags(cmd *cobra.Command) {
	cmd.Flags().String("node", "", "表格文档 ID 或 URL")
	addSheetHiddenStringFlag(cmd, "url", "--node alias")
	addSheetHiddenStringFlag(cmd, "id", "--node alias")
	addSheetHiddenStringFlag(cmd, "node-id", "--node alias")
	addSheetHiddenStringFlag(cmd, "doc-id", "--node alias")
	addSheetHiddenStringFlag(cmd, "file-id", "--node alias")
}

func addSheetBaseFlags(cmd *cobra.Command) {
	addSheetNodeFlags(cmd)
	cmd.Flags().String("sheet-id", "", "工作表 ID 或名称")
}

func addSheetFilterViewBaseFlags(cmd *cobra.Command) {
	addSheetBaseFlags(cmd)
	cmd.Flags().String("filter-view-id", "", "筛选视图 ID")
}

func addSheetFloatImageBaseFlags(cmd *cobra.Command) {
	addSheetBaseFlags(cmd)
	cmd.Flags().String("float-image-id", "", "浮动图片 ID")
}

func addSheetRangeTransferFlags(cmd *cobra.Command) {
	addSheetBaseFlags(cmd)
	cmd.Flags().String("source-range", "", "源范围，A1 表示法 (必填)")
	cmd.Flags().String("target-range", "", "目标位置，A1 表示法 (必填)")
	cmd.Flags().String("target-sheet-id", "", "目标工作表 ID 或名称")
}

func addSheetHiddenStringFlag(cmd *cobra.Command, name, usage string) {
	cmd.Flags().String(name, "", usage)
	_ = cmd.Flags().MarkHidden(name)
}

func sheetBaseParams(cmd *cobra.Command) (map[string]any, error) {
	if err := sheetValidateRequired(cmd, "node", "sheet-id"); err != nil {
		return nil, err
	}
	return map[string]any{
		"nodeId":  sheetStringFlag(cmd, "node"),
		"sheetId": sheetStringFlag(cmd, "sheet-id"),
	}, nil
}

func sheetFilterViewBaseParams(cmd *cobra.Command) map[string]any {
	return map[string]any{
		"nodeId":       sheetStringFlag(cmd, "node"),
		"sheetId":      sheetStringFlag(cmd, "sheet-id"),
		"filterViewId": sheetStringFlag(cmd, "filter-view-id"),
	}
}

func sheetFloatImageBaseParams(cmd *cobra.Command) (map[string]any, error) {
	if err := sheetValidateRequired(cmd, "node", "sheet-id", "float-image-id"); err != nil {
		return nil, err
	}
	return map[string]any{
		"nodeId":       sheetStringFlag(cmd, "node"),
		"sheetId":      sheetStringFlag(cmd, "sheet-id"),
		"floatImageId": sheetStringFlag(cmd, "float-image-id"),
	}, nil
}

func sheetRangeAddressParams(cmd *cobra.Command) (map[string]any, error) {
	if err := sheetValidateRequired(cmd, "node", "sheet-id", "range"); err != nil {
		return nil, err
	}
	return map[string]any{
		"nodeId":       sheetStringFlag(cmd, "node"),
		"sheetId":      sheetStringFlag(cmd, "sheet-id"),
		"rangeAddress": sheetStringFlag(cmd, "range"),
	}, nil
}

func sheetRangeTransferParams(cmd *cobra.Command) (map[string]any, error) {
	if err := sheetValidateRequired(cmd, "node", "sheet-id", "source-range", "target-range"); err != nil {
		return nil, err
	}
	params := map[string]any{
		"nodeId":           sheetStringFlag(cmd, "node"),
		"sheetId":          sheetStringFlag(cmd, "sheet-id"),
		"sourceRange":      sheetStringFlag(cmd, "source-range"),
		"destinationRange": sheetStringFlag(cmd, "target-range"),
	}
	sheetAddStringParam(cmd, params, "targetSheetId", "target-sheet-id")
	return params, nil
}

func sheetNodeAndName(cmd *cobra.Command) (string, string, error) {
	node, err := sheetRequiredFlag(cmd, "node")
	if err != nil {
		return "", "", err
	}
	name, err := sheetRequiredFlag(cmd, "name")
	if err != nil {
		return "", "", err
	}
	return node, name, nil
}

func sheetNameFlag(cmd *cobra.Command) string {
	if v := sheetStringFlag(cmd, "name"); v != "" {
		return v
	}
	return sheetStringFlag(cmd, "title")
}

func sheetStringFlag(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func sheetFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	for _, name := range append([]string{primary}, aliases...) {
		if flag := cmd.Flags().Lookup(name); flag == nil {
			continue
		}
		if v := sheetStringFlag(cmd, name); v != "" {
			return v
		}
	}
	return ""
}

func sheetRequiredFlag(cmd *cobra.Command, name string) (string, error) {
	if v := sheetStringFlag(cmd, name); v != "" {
		return v, nil
	}
	return "", apperrors.NewValidation("--" + name + " is required")
}

func sheetRequiredFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) (string, error) {
	if v := sheetFlagOrFallback(cmd, primary, aliases...); v != "" {
		return v, nil
	}
	return "", apperrors.NewValidation("--" + primary + " is required")
}

func sheetValidateRequired(cmd *cobra.Command, names ...string) error {
	for _, name := range names {
		if sheetStringFlag(cmd, name) == "" {
			return apperrors.NewValidation("--" + name + " is required")
		}
	}
	return nil
}

func sheetAddStringParam(cmd *cobra.Command, params map[string]any, key string, flags ...string) {
	if v := sheetFlagOrFallback(cmd, flags[0], flags[1:]...); v != "" {
		params[key] = v
	}
}

func sheetAddChangedIntParam(cmd *cobra.Command, params map[string]any, key, flag string) {
	if cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetInt(flag)
		params[key] = v
	}
}

func sheetParseJSONFlag(cmd *cobra.Command, flag string, out any) error {
	raw := sheetStringFlag(cmd, flag)
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("--%s JSON parse failed: %w", flag, err)
	}
	return nil
}

func sheetDimension(cmd *cobra.Command) (string, error) {
	dimension := sheetStringFlag(cmd, "dimension")
	switch dimension {
	case "ROW":
		dimension = "ROWS"
	case "COLUMN":
		dimension = "COLUMNS"
	}
	if dimension != "ROWS" && dimension != "COLUMNS" {
		return "", apperrors.NewValidation("--dimension must be ROWS or COLUMNS")
	}
	return dimension, nil
}

func sheetPositiveIntStringFlag(cmd *cobra.Command, flag string, max int) (int, error) {
	raw := sheetStringFlag(cmd, flag)
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, apperrors.NewValidation("--" + flag + " must be a positive integer")
	}
	if max > 0 && n > max {
		return 0, apperrors.NewValidation(fmt.Sprintf("--%s must be <= %d", flag, max))
	}
	return n, nil
}

func sheetPositiveSize(cmd *cobra.Command, required bool) (int, int, error) {
	width, _ := cmd.Flags().GetInt("width")
	height, _ := cmd.Flags().GetInt("height")
	if required || cmd.Flags().Changed("width") {
		if width <= 0 {
			return 0, 0, apperrors.NewValidation("--width must be > 0")
		}
	}
	if required || cmd.Flags().Changed("height") {
		if height <= 0 {
			return 0, 0, apperrors.NewValidation("--height must be > 0")
		}
	}
	return width, height, nil
}

func sheetCondFormatMutationBase(cmd *cobra.Command, update bool) (map[string]any, error) {
	params, err := sheetBaseParams(cmd)
	if err != nil {
		return nil, err
	}
	if update {
		ruleID, err := sheetRequiredFlag(cmd, "rule-id")
		if err != nil {
			return nil, err
		}
		params["ruleId"] = ruleID
	}
	if cmd.Flags().Changed("ranges") || !update {
		var ranges []string
		if err := sheetParseJSONFlag(cmd, "ranges", &ranges); err != nil {
			return nil, err
		}
		if len(ranges) == 0 {
			return nil, apperrors.NewValidation("--ranges must contain at least one range")
		}
		params["ranges"] = ranges
	}
	if cmd.Flags().Changed("condition") || !update {
		var condition map[string]any
		if err := sheetParseJSONFlag(cmd, "condition", &condition); err != nil {
			return nil, err
		}
		for k, v := range condition {
			params[k] = v
		}
	}
	if cmd.Flags().Changed("cell-style") {
		var cellStyle map[string]any
		if err := sheetParseJSONFlag(cmd, "cell-style", &cellStyle); err != nil {
			return nil, err
		}
		params["cellStyle"] = cellStyle
	}
	if cmd.Flags().Changed("data-bar-style") {
		var dataBarStyle map[string]any
		if err := sheetParseJSONFlag(cmd, "data-bar-style", &dataBarStyle); err != nil {
			return nil, err
		}
		params["dataBarStyle"] = dataBarStyle
	}
	return params, nil
}

func runSheetAttachmentUpload(cmd *cobra.Command, runner executor.Runner, writeImage bool) error {
	node, err := sheetRequiredFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	if err != nil {
		return err
	}
	filePath, err := sheetRequiredFlag(cmd, "file")
	if err != nil {
		return err
	}
	if writeImage {
		if err := sheetValidateRequired(cmd, "sheet-id", "range"); err != nil {
			return err
		}
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if info.IsDir() {
		return apperrors.NewValidation(filePath + " is a directory")
	}
	fileName := sheetStringFlag(cmd, "name")
	if fileName == "" {
		fileName = filepath.Base(filePath)
	} else if filepath.Ext(fileName) == "" {
		if ext := filepath.Ext(filePath); ext != "" {
			fileName += ext
		}
	}
	mimeType := sheetStringFlag(cmd, "mime-type")
	if mimeType == "" {
		mimeType = detectMIME(fileName)
	}
	params := map[string]any{
		"nodeId":   node,
		"fileName": fileName,
		"fileSize": float64(info.Size()),
		"mimeType": mimeType,
	}
	if commandDryRun(cmd) {
		return runSheetDocTool(cmd, runner, "get_doc_attachment_upload_info", params)
	}
	credResult, err := sheetInvocationResult(cmd, runner, "doc", "get_doc_attachment_upload_info", params)
	if err != nil {
		return err
	}
	uploadURL, resourceID, resourceURL, err := extractDocAttachmentUploadInfo(credResult.Response)
	if err != nil {
		return err
	}
	if err := sheetHTTPPut(cmd, uploadURL, filePath, info.Size(), mimeType); err != nil {
		return err
	}
	if !writeImage {
		return writeCommandPayload(cmd, map[string]any{
			"resourceId":  resourceID,
			"resourceUrl": resourceURL,
			"fileName":    fileName,
			"mimeType":    mimeType,
			"fileSize":    info.Size(),
		})
	}
	writeArgs := map[string]any{
		"nodeId":       node,
		"sheetId":      sheetStringFlag(cmd, "sheet-id"),
		"rangeAddress": sheetStringFlag(cmd, "range"),
		"resourceId":   resourceID,
		"resourceUrl":  resourceURL,
	}
	sheetAddChangedIntParam(cmd, writeArgs, "width", "width")
	sheetAddChangedIntParam(cmd, writeArgs, "height", "height")
	return runSheetTool(cmd, runner, "write_image", writeArgs)
}

func runSheetDocTool(cmd *cobra.Command, runner executor.Runner, tool string, params map[string]any) error {
	result, err := sheetInvocationResult(cmd, runner, "doc", tool, params)
	if err != nil {
		return err
	}
	return writeCommandPayload(cmd, result)
}

func sheetHTTPPut(cmd *cobra.Command, uploadURL, filePath string, size int64, mimeType string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPut, uploadURL, f)
	if err != nil {
		return fmt.Errorf("build upload request failed: %w", err)
	}
	req.ContentLength = size
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("OSS upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("OSS upload failed HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func sheetHTTPGet(cmd *cobra.Command, rawURL, outputPath string) error {
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("download failed HTTP %d: %s", resp.StatusCode, string(body))
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func pollSheetExport(cmd *cobra.Command, runner executor.Runner, jobID string) (downloadURL, status string, err error) {
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
	for _, wait := range intervals {
		timer := time.NewTimer(wait)
		select {
		case <-cmd.Context().Done():
			timer.Stop()
			return "", "", cmd.Context().Err()
		case <-timer.C:
		}
		result, err := sheetInvocationResult(cmd, runner, "sheet", "query_export_job", map[string]any{"jobId": jobID})
		if err != nil {
			continue
		}
		status = strings.ToUpper(strings.TrimSpace(sheetStringFromResponse(result.Response, "status")))
		downloadURL = sheetStringFromResponse(result.Response, "downloadUrl")
		switch status {
		case "SUCCESS":
			return downloadURL, status, nil
		case "FAILED", "FAIL", "ERROR":
			msg := sheetStringFromResponse(result.Response, "message")
			if msg == "" {
				msg = "export failed"
			}
			return "", status, apperrors.NewValidation(msg)
		}
	}
	return "", status, apperrors.NewValidation("export timed out")
}

func sheetStringFromResponse(resp map[string]any, key string) string {
	data := sheetResponseData(resp)
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func sheetResponseData(resp map[string]any) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	data := resp
	for {
		next, ok := data["result"].(map[string]any)
		if !ok {
			next, ok = data["data"].(map[string]any)
		}
		if !ok {
			next, ok = data["content"].(map[string]any)
		}
		if !ok || len(next) == 0 {
			return data
		}
		data = next
	}
}

func sheetResultFilterViews(resp map[string]any) []map[string]any {
	data := sheetResponseData(resp)
	raw, _ := data["filterViews"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func sheetFindFilterView(views []map[string]any, id string) (map[string]any, error) {
	for _, view := range views {
		if v, _ := view["id"].(string); v == id {
			return view, nil
		}
		if v, _ := view["filterViewId"].(string); v == id {
			return view, nil
		}
	}
	return nil, apperrors.NewValidation("filter view not found: " + id)
}

func sheetExportFilename(downloadURL, jobID string) string {
	if parsed, err := url.Parse(downloadURL); err == nil {
		name := filepath.Base(parsed.Path)
		if decoded, decodeErr := url.PathUnescape(name); decodeErr == nil && decoded != "" {
			name = decoded
		}
		name = strings.ReplaceAll(name, "\\", "/")
		name = filepath.Base(name)
		if name != "" && name != "." && name != "/" {
			return name
		}
	}
	return "sheet-export-" + jobID + ".xlsx"
}

type sheetStyleSpec struct {
	BgColor         string `json:"bgColor,omitempty"`
	BgColorsJSON    string `json:"bgColorsJson,omitempty"`
	FontSize        int    `json:"fontSize,omitempty"`
	FontSizesJSON   string `json:"fontSizesJson,omitempty"`
	HAlign          string `json:"hAlign,omitempty"`
	HAlignsJSON     string `json:"hAlignsJson,omitempty"`
	VAlign          string `json:"vAlign,omitempty"`
	VAlignsJSON     string `json:"vAlignsJson,omitempty"`
	FontColor       string `json:"fontColor,omitempty"`
	FontColorsJSON  string `json:"fontColorsJson,omitempty"`
	FontWeight      string `json:"fontWeight,omitempty"`
	FontWeightsJSON string `json:"fontWeightsJson,omitempty"`
	WordWrap        string `json:"wordWrap,omitempty"`
	NumberFormat    string `json:"numberFormat,omitempty"`
}

type sheetBatchStyleItem struct {
	SheetID string `json:"sheetId"`
	Range   string `json:"range"`
	sheetStyleSpec
}

func newSheetRangeSetStyleCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("set-style", "设置指定单元格区域的样式", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "sheet-id", "range"); err != nil {
			return err
		}
		rows, cols, err := sheetParseA1Range(sheetStringFlag(cmd, "range"))
		if err != nil {
			return err
		}
		params := map[string]any{
			"nodeId":       sheetStringFlag(cmd, "node"),
			"sheetId":      sheetStringFlag(cmd, "sheet-id"),
			"rangeAddress": sheetStringFlag(cmd, "range"),
		}
		if err := sheetApplyStyleSpec(sheetReadStyleSpec(cmd), rows, cols, params); err != nil {
			return err
		}
		return runSheetTool(cmd, runner, "update_range", params)
	})
	addSheetBaseFlags(cmd)
	cmd.Flags().String("range", "", "目标单元格区域地址 (必填)")
	bindSheetStyleFlags(cmd)
	return cmd
}

func newSheetRangeBatchSetStyleCommand(runner executor.Runner) *cobra.Command {
	cmd := newSheetLeaf("batch-set-style", "按配置文件批量设置样式", func(cmd *cobra.Command, _ []string) error {
		if err := sheetValidateRequired(cmd, "node", "batch"); err != nil {
			return err
		}
		data, err := os.ReadFile(sheetStringFlag(cmd, "batch"))
		if err != nil {
			return err
		}
		var items []sheetBatchStyleItem
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		continueOnErr, _ := cmd.Flags().GetBool("continue-on-error")
		var outputs []executor.Result
		var firstErr error
		for _, item := range items {
			if item.SheetID == "" || item.Range == "" {
				firstErr = apperrors.NewValidation("batch item requires sheetId and range")
				if !continueOnErr {
					return firstErr
				}
				continue
			}
			rows, cols, err := sheetParseA1Range(item.Range)
			if err != nil {
				firstErr = err
				if !continueOnErr {
					return err
				}
				continue
			}
			params := map[string]any{"nodeId": sheetStringFlag(cmd, "node"), "sheetId": item.SheetID, "rangeAddress": item.Range}
			if err := sheetApplyStyleSpec(&item.sheetStyleSpec, rows, cols, params); err != nil {
				firstErr = err
				if !continueOnErr {
					return err
				}
				continue
			}
			result, err := sheetInvocationResult(cmd, runner, "sheet", "update_range", params)
			if err != nil {
				firstErr = err
				if !continueOnErr {
					return err
				}
				continue
			}
			outputs = append(outputs, result)
		}
		if firstErr != nil && !continueOnErr {
			return firstErr
		}
		return writeCommandPayload(cmd, map[string]any{"count": len(outputs), "results": outputs})
	})
	addSheetNodeFlags(cmd)
	cmd.Flags().String("batch", "", "批次配置 JSON 文件路径 (必填)")
	cmd.Flags().Bool("continue-on-error", false, "遇到失败时继续执行")
	return cmd
}

func bindSheetStyleFlags(cmd *cobra.Command) {
	cmd.Flags().String("bg-color", "", "背景色")
	cmd.Flags().String("bg-colors-json", "", "背景色二维 JSON 数组")
	cmd.Flags().Int("font-size", 0, "字号")
	cmd.Flags().String("font-sizes-json", "", "字号二维 JSON 数组")
	cmd.Flags().String("h-align", "", "水平对齐")
	cmd.Flags().String("h-aligns-json", "", "水平对齐二维 JSON 数组")
	cmd.Flags().String("v-align", "", "垂直对齐")
	cmd.Flags().String("v-aligns-json", "", "垂直对齐二维 JSON 数组")
	cmd.Flags().String("font-color", "", "字体颜色")
	cmd.Flags().String("font-colors-json", "", "字体颜色二维 JSON 数组")
	cmd.Flags().String("font-weight", "", "字体粗细")
	cmd.Flags().String("font-weights-json", "", "字体粗细二维 JSON 数组")
	cmd.Flags().String("word-wrap", "", "换行方式")
	cmd.Flags().String("number-format", "", "数字格式 code")
}

func sheetReadStyleSpec(cmd *cobra.Command) *sheetStyleSpec {
	spec := &sheetStyleSpec{}
	spec.BgColor, _ = cmd.Flags().GetString("bg-color")
	spec.BgColorsJSON, _ = cmd.Flags().GetString("bg-colors-json")
	spec.FontSize, _ = cmd.Flags().GetInt("font-size")
	spec.FontSizesJSON, _ = cmd.Flags().GetString("font-sizes-json")
	spec.HAlign, _ = cmd.Flags().GetString("h-align")
	spec.HAlignsJSON, _ = cmd.Flags().GetString("h-aligns-json")
	spec.VAlign, _ = cmd.Flags().GetString("v-align")
	spec.VAlignsJSON, _ = cmd.Flags().GetString("v-aligns-json")
	spec.FontColor, _ = cmd.Flags().GetString("font-color")
	spec.FontColorsJSON, _ = cmd.Flags().GetString("font-colors-json")
	spec.FontWeight, _ = cmd.Flags().GetString("font-weight")
	spec.FontWeightsJSON, _ = cmd.Flags().GetString("font-weights-json")
	spec.WordWrap, _ = cmd.Flags().GetString("word-wrap")
	spec.NumberFormat, _ = cmd.Flags().GetString("number-format")
	return spec
}

func sheetApplyStyleSpec(spec *sheetStyleSpec, rows, cols int, params map[string]any) error {
	if rows > 1000 || rows*cols > 30000 {
		return apperrors.NewValidation("style range is too large")
	}
	if err := sheetApply2DString(spec.BgColor, spec.BgColorsJSON, rows, cols, "bg-color", "backgroundColors", nil, params); err != nil {
		return err
	}
	if err := sheetApplyFontSize(spec, rows, cols, params); err != nil {
		return err
	}
	if err := sheetApply2DString(spec.HAlign, spec.HAlignsJSON, rows, cols, "h-align", "horizontalAlignments", map[string]bool{"left": true, "center": true, "right": true, "general": true}, params); err != nil {
		return err
	}
	if err := sheetApply2DString(spec.VAlign, spec.VAlignsJSON, rows, cols, "v-align", "verticalAlignments", map[string]bool{"top": true, "middle": true, "bottom": true}, params); err != nil {
		return err
	}
	if err := sheetApply2DString(spec.FontColor, spec.FontColorsJSON, rows, cols, "font-color", "fontColors", nil, params); err != nil {
		return err
	}
	if err := sheetApply2DString(spec.FontWeight, spec.FontWeightsJSON, rows, cols, "font-weight", "fontWeights", map[string]bool{"bold": true, "normal": true}, params); err != nil {
		return err
	}
	if spec.WordWrap != "" {
		if !map[string]bool{"overflow": true, "clip": true, "autoWrap": true}[spec.WordWrap] {
			return apperrors.NewValidation("--word-wrap has invalid value")
		}
		params["wordWrap"] = spec.WordWrap
	}
	if spec.NumberFormat != "" {
		params["numberFormat"] = spec.NumberFormat
	}
	for _, key := range []string{"backgroundColors", "fontSizes", "horizontalAlignments", "verticalAlignments", "fontColors", "fontWeights", "wordWrap", "numberFormat"} {
		if _, ok := params[key]; ok {
			return nil
		}
	}
	return apperrors.NewValidation("at least one style flag is required")
}

func sheetApplyFontSize(spec *sheetStyleSpec, rows, cols int, params map[string]any) error {
	if spec.FontSize != 0 && spec.FontSizesJSON != "" {
		return apperrors.NewValidation("--font-size and --font-sizes-json are mutually exclusive")
	}
	if spec.FontSize != 0 {
		if spec.FontSize < 0 {
			return apperrors.NewValidation("--font-size must be positive")
		}
		params["fontSizes"] = sheetFillIntMatrix(rows, cols, spec.FontSize)
		return nil
	}
	if spec.FontSizesJSON != "" {
		var m [][]int
		if err := json.Unmarshal([]byte(spec.FontSizesJSON), &m); err != nil {
			return err
		}
		if !sheetMatrixIntShape(m, rows, cols) {
			return apperrors.NewValidation("--font-sizes-json shape does not match range")
		}
		params["fontSizes"] = m
	}
	return nil
}

func sheetApply2DString(scalar, jsonStr string, rows, cols int, flagName, key string, enum map[string]bool, params map[string]any) error {
	if scalar != "" && jsonStr != "" {
		return apperrors.NewValidation("--" + flagName + " and --" + flagName + "s-json are mutually exclusive")
	}
	if scalar != "" {
		if enum != nil && !enum[scalar] {
			return apperrors.NewValidation("--" + flagName + " has invalid value")
		}
		params[key] = sheetFillStringMatrix(rows, cols, scalar)
		return nil
	}
	if jsonStr == "" {
		return nil
	}
	var m [][]string
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return err
	}
	if !sheetMatrixStringShape(m, rows, cols) {
		return apperrors.NewValidation("--" + flagName + "-json shape does not match range")
	}
	if enum != nil {
		for _, row := range m {
			for _, v := range row {
				if v != "" && !enum[v] {
					return apperrors.NewValidation("--" + flagName + "-json has invalid value")
				}
			}
		}
	}
	params[key] = m
	return nil
}

func sheetParseA1Range(addr string) (rows, cols int, err error) {
	if i := strings.Index(addr, "!"); i >= 0 {
		addr = addr[i+1:]
	}
	addr = strings.TrimSpace(strings.ToUpper(addr))
	parts := strings.SplitN(addr, ":", 2)
	c1, r1, err := sheetParseA1Cell(parts[0])
	if err != nil {
		return 0, 0, err
	}
	c2, r2 := c1, r1
	if len(parts) == 2 {
		c2, r2, err = sheetParseA1Cell(parts[1])
		if err != nil {
			return 0, 0, err
		}
	}
	if c2 < c1 {
		c1, c2 = c2, c1
	}
	if r2 < r1 {
		r1, r2 = r2, r1
	}
	return r2 - r1 + 1, c2 - c1 + 1, nil
}

func sheetParseA1Cell(s string) (col, row int, err error) {
	for len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
		col = col*26 + int(s[0]-'A'+1)
		s = s[1:]
	}
	if col == 0 || s == "" {
		return 0, 0, apperrors.NewValidation("invalid A1 cell")
	}
	row, err = strconv.Atoi(s)
	if err != nil || row <= 0 {
		return 0, 0, apperrors.NewValidation("invalid A1 cell")
	}
	return col, row, nil
}

func sheetFillStringMatrix(rows, cols int, v string) [][]string {
	out := make([][]string, rows)
	for i := range out {
		out[i] = make([]string, cols)
		for j := range out[i] {
			out[i][j] = v
		}
	}
	return out
}

func sheetFillIntMatrix(rows, cols, v int) [][]int {
	out := make([][]int, rows)
	for i := range out {
		out[i] = make([]int, cols)
		for j := range out[i] {
			out[i][j] = v
		}
	}
	return out
}

func sheetMatrixStringShape(m [][]string, rows, cols int) bool {
	if len(m) != rows {
		return false
	}
	for _, row := range m {
		if len(row) != cols {
			return false
		}
	}
	return true
}

func sheetMatrixIntShape(m [][]int, rows, cols int) bool {
	if len(m) != rows {
		return false
	}
	for _, row := range m {
		if len(row) != cols {
			return false
		}
	}
	return true
}

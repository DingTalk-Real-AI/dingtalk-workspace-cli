package helpers

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// resolveSheetName 折叠 --name / --title 别名为单个值。
// 优先 --name，未设置时回退到 --title；两者都未传返回 ""。
func resolveSheetName(cmd *cobra.Command) string {
	if v, _ := cmd.Flags().GetString("name"); v != "" {
		return v
	}
	if v, _ := cmd.Flags().GetString("title"); v != "" {
		return v
	}
	return ""
}

func newSheetCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "sheet",
		Short: "钉钉表格管理",
		Long: `管理钉钉在线电子表格：创建表格、工作表管理、数据读写、单元格搜索、查找替换、单元格合并与取消合并、行列插入删除移动追加与属性更新、附件上传、浮动图片管理、筛选视图管理、下拉列表管理。

命令结构:
  dws sheet create                              创建钉钉表格文档
  dws sheet list                                获取全部工作表列表
  dws sheet info                                获取指定工作表详情
  dws sheet new                                 新建工作表
  dws sheet update                              更新工作表属性
  dws sheet copy                                复制工作表
  dws sheet range read                          读取工作表数据
  dws sheet range update                        更新工作表指定区域内容
  dws sheet range clear                         清除工作表指定区域
  dws sheet range sort                          对工作表指定区域排序
  dws sheet range fill                          自动填充工作表指定区域
  dws sheet range copy-to                       复制区域到目标位置
  dws sheet range move-to                       移动区域到目标位置
  dws sheet range set-style                     设置指定区域的单元格样式
  dws sheet range batch-set-style               按配置文件批量设置单元格样式
  dws sheet find                                在工作表中搜索单元格内容
  dws sheet append                              在工作表末尾追加数据
  dws sheet csv-put                             将 CSV 数据写入表格指定位置
  dws sheet table-get                          读取结构化 table 数据
  dws sheet table-put                          写入结构化 table 数据
  dws sheet pivot-table [list|create|update|delete]  透视表管理
  dws sheet show-gridline                      显示工作表网格线
  dws sheet hide-gridline                      隐藏工作表网格线
  dws sheet merge-cells                         合并单元格
  dws sheet insert-dimension                    在指定位置插入行或列
  dws sheet delete-dimension                    删除指定位置的行或列
  dws sheet update-dimension                    更新指定范围行/列属性（显隐、行高/列宽）
  dws sheet group-dimension                     对指定连续行/列创建分组
  dws sheet ungroup-dimension                   取消指定连续行/列分组
  dws sheet media-upload                        上传附件到表格
  dws sheet write-image                         上传图片并写入表格单元格
  dws sheet replace                             全局查找替换文本
  dws sheet move-dimension                      移动行或列到指定位置
  dws sheet add-dimension                       在末尾追加空行或空列
  dws sheet unmerge-cells                       取消合并单元格
  dws sheet set-dropdown                        设置下拉列表
  dws sheet get-dropdown                        获取下拉列表配置
  dws sheet delete-dropdown                     删除下拉列表
  dws sheet create-float-image                  创建浮动图片
  dws sheet get-float-image                     获取浮动图片详情
  dws sheet list-float-images                   列出工作表所有浮动图片
  dws sheet update-float-image                  更新浮动图片属性
  dws sheet delete-float-image                  删除浮动图片
  dws sheet filter get                          获取全局筛选信息
  dws sheet filter create                       创建全局筛选
  dws sheet filter delete                       删除全局筛选
  dws sheet filter update                       批量更新筛选条件
  dws sheet filter clear-criteria               清除单列筛选条件
  dws sheet filter sort                         筛选排序
  dws sheet filter-view list                    获取所有筛选视图
  dws sheet filter-view create                  创建筛选视图
  dws sheet filter-view update                  更新筛选视图属性
  dws sheet filter-view delete                  删除筛选视图
  dws sheet filter-view update-criteria         更新筛选视图列条件
  dws sheet filter-view delete-criteria         删除筛选视图列条件
  dws sheet filter-view info                    获取单个筛选视图详情
  dws sheet filter-view list-criteria           列出筛选视图所有列条件
  dws sheet filter-view get-criteria            获取单列筛选条件详情
  dws sheet cond-format list                    获取条件格式规则
  dws sheet cond-format create                  创建条件格式规则
  dws sheet cond-format update                  更新条件格式规则
  dws sheet cond-format delete                  删除条件格式规则
  dws sheet chart list                           获取浮动图表
  dws sheet chart create                         创建浮动图表
  dws sheet chart update                         更新浮动图表
  dws sheet chart delete                         删除浮动图表
  dws sheet export                              导出表格为 xlsx（异步任务一站式：提交→轮询→可选下载）
  dws sheet template list                       获取表格模板列表
  dws sheet template search                     搜索表格模板
  dws sheet template apply                      应用表格模板创建新表格文档`,
	}

	// ── Build commands via factory functions ──────────────────────────
	workbookCmds := newWorkbookCmds()
	rangeCmd := newRangeCmd()
	dataCmds := newDataCmds()
	dimensionCmds := newDimensionCmds()
	mediaCmds := newMediaCmds()
	filterCmd := newFilterCmd()
	filterViewCmd := newFilterViewCmd()
	condFormatCmd := newCondFormatCmd()
	floatImageCmds := newFloatImageCmds()
	chartCmd := newChartCmd()
	exportCmd := newExportCmd()
	templateCmd := newSheetTemplateCmd()
	tableCmds := newTableCmds()
	pivotTableCmd := newPivotTableCmd()

	batchUpdateCmd := newBatchUpdateCmd()
	rangeBatchClearCmd := newRangeBatchClearCmd()
	rangeCmd.AddCommand(newRangeSetStyleCmd(), newRangeBatchSetStyleCmd(), rangeBatchClearCmd)

	// Flag registrations for batch commands
	batchUpdateCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	batchUpdateCmd.Flags().String("operations", "", "操作数组 JSON (必填，每项 {toolName, input})")
	batchUpdateCmd.Flags().Bool("continue-on-error", false, "遇失败继续执行后续操作（默认 false，严格事务）")
	rangeBatchClearCmd.Flags().String("node", "", "表格文档 ID 或 URL (必填)")
	rangeBatchClearCmd.Flags().String("ranges", "", `目标区域 JSON 数组，每项带 sheet 前缀 (必填，如 '["Sheet1!A1:B3"]')`)
	rangeBatchClearCmd.Flags().String("type", "", "清除类型: content(仅值,默认) / format(仅格式) / all(全部)")

	// Collect all standalone commands for alias registration
	standaloneCmds := []*cobra.Command{}
	standaloneCmds = append(standaloneCmds, workbookCmds...)
	standaloneCmds = append(standaloneCmds, dataCmds...)
	standaloneCmds = append(standaloneCmds, dimensionCmds...)
	standaloneCmds = append(standaloneCmds, mediaCmds...)
	standaloneCmds = append(standaloneCmds, floatImageCmds...)
	standaloneCmds = append(standaloneCmds, tableCmds...)
	standaloneCmds = append(standaloneCmds, exportCmd, batchUpdateCmd)

	// Register cross-product aliases
	for _, cmd := range standaloneCmds {
		RegisterCrossProductAliases(cmd)
	}
	for _, parent := range []*cobra.Command{rangeCmd, filterCmd, filterViewCmd, condFormatCmd} {
		for _, child := range parent.Commands() {
			RegisterCrossProductAliases(child)
		}
	}

	// Add all to root
	root.AddCommand(standaloneCmds...)
	root.AddCommand(rangeCmd, filterCmd, filterViewCmd, condFormatCmd, chartCmd, templateCmd, pivotTableCmd)

	// Guards for grouped parent commands
	attachUnknownSubcommandGuard(root)
	attachUnknownSubcommandGuard(rangeCmd)
	attachUnknownSubcommandGuard(filterCmd)
	attachUnknownSubcommandGuard(filterViewCmd)
	attachUnknownSubcommandGuard(condFormatCmd)
	attachUnknownSubcommandGuard(chartCmd)
	attachUnknownSubcommandGuard(pivotTableCmd)

	return root
}

// attachUnknownSubcommandGuard 为分组型命令挂上拼错子命令时的 did-you-mean 提示。
//
// 背景：cobra 对父命令的 Args 校验发生在 ParseFlags 之后，而 pflag 默认把未知 flag
// 当作硬错误抛出。于是 `dws sheet read --sheet-id X` 会先报 `unknown flag: --sheet-id`，
// 真正的根因（read 不是 sheet 的直接子命令）被彻底掩盖；同时 `dws sheet reead` 会被
// 当成位置参数静默吞掉、打印 help 后 exit=0，AI Agent 无法察觉命令执行失败。
//
// 本函数通过三件套让分组命令在"没匹配到子命令"时给出明确的错误与建议：
//  1. FParseErrWhitelist.UnknownFlags=true —— pflag 不再因未知 flag 中断，
//     未知 flag 连同其值一起被静默消化；
//  2. Args=ArbitraryArgs —— 允许把剩余位置参数交给 RunE 处理；
//  3. RunE —— 取 args[0] 作为拼错的子命令名，先在后代命令里查找完全同名的叶子
//     （能把 `sheet read` 精准引导到 `sheet range read`），找不到再退回 cobra
//     自带的同级编辑距离建议；最终返回 error 以保证 exit!=0。
//
// 仅挂在分组型父命令（sheet/range/filter-view）上，不会影响已在 cobra Find 阶段
// 精确匹配到的合法叶子命令。
func attachUnknownSubcommandGuard(cmd *cobra.Command) {
	cmd.Args = cobra.ArbitraryArgs
	cmd.FParseErrWhitelist = cobra.FParseErrWhitelist{UnknownFlags: true}
	cmd.SilenceUsage = true
	// cobra 仅在 root 自动把 SuggestionsMinimumDistance 兑成 2，子命令默认为 0，
	// 会导致 `sheet range reead` 这样的同级近似拼写无法触发内置建议。
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		name := args[0]
		var buf strings.Builder
		fmt.Fprintf(&buf, "unknown command %q for %q", name, c.CommandPath())
		suggestions := deepSuggestSubcommand(c, name)
		if len(suggestions) == 0 {
			suggestions = c.SuggestionsFor(name)
		}
		if len(suggestions) > 0 {
			buf.WriteString("\n\nDid you mean this?")
			for _, s := range suggestions {
				fmt.Fprintf(&buf, "\n\t%s %s", c.CommandPath(), s)
			}
		}
		fmt.Fprintf(&buf, "\n\nRun '%s --help' for usage.", c.CommandPath())
		return fmt.Errorf("%s", buf.String())
	}
}

// deepSuggestSubcommand 在所有后代命令里查找与 name 完全同名的可用子命令，
// 返回从 parent 出发的相对路径列表。用于把 `sheet read` 这样的平铺习惯引导到
// 真实的深路径 `sheet range read`。
func deepSuggestSubcommand(parent *cobra.Command, name string) []string {
	var out []string
	var walk func(c *cobra.Command, rel []string)
	walk = func(c *cobra.Command, rel []string) {
		for _, sub := range c.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			next := append(append([]string{}, rel...), sub.Name())
			if sub.Name() == name {
				out = append(out, strings.Join(next, " "))
			}
			walk(sub, next)
		}
	}
	walk(parent, nil)
	return out
}

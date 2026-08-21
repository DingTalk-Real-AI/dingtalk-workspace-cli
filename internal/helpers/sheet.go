package helpers

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
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
	// Product-level Agent routing Decl (migrated from selection/sheet.json
	// products.sheet). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "sheet",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "导入本地 Excel，或创建、读取、编辑、导出和审计钉钉在线电子表格（axls），并管理工作表、区域、历史 revision、筛选、图表、图片与格式。",
			UseWhen: []string{
				"用户要处理钉钉在线电子表格中的工作表、单元格、范围、筛选、图表、图片或格式时",
				"用户要把本地 xlsx/xls 转换为新的钉钉在线电子表格时",
				"用户要查询工作簿当前 revision 或复核两个 revision 之间的 changeset 时",
			},
			AvoidWhen: []string{
				"目标是 AI 表格 Base 的结构化记录或钉钉文档正文时不要使用 sheet",
				"只读取已上传但未转换的 xlsx/xls 节点时先用 doc download；不要把文件节点直接传给单元格命令",
			},
		},
	})
	root := &cobra.Command{
		Use:   "sheet",
		Short: "钉钉表格管理",
		Long: `管理钉钉在线电子表格：创建表格、工作表管理、数据读写、单元格搜索、查找替换、单元格合并与取消合并、行列插入删除移动追加与属性更新、附件上传、浮动图片管理、筛选视图管理、下拉列表管理。

命令结构:
  dws sheet create                              创建钉钉表格文档
  dws sheet create-with-data                     创建表格文档并写入初始数据（可选样式）
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
  dws sheet revision-get                         获取工作簿当前 revision
  dws sheet changeset-get                        获取工作簿 revision 区间内的 changeset
  dws sheet export                              导出表格为 xlsx（异步任务一站式：提交→轮询→可选下载）
  dws sheet export-csv                          导出单个工作表为纯 CSV（同步，可落盘）
  dws sheet import                              导入 xlsx/xls 为在线电子表格
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
	DeclareLeafMetadata(exportCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "submit_export_job",
				CanonicalPath:  "sheet.submit_export_job",
				CLIPath:        "sheet export",
				PrimaryCLIPath: "sheet export",
			},
			Description: "一站式导出 axls 为 xlsx（内部提交+轮询，可选下载）。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "sheet", RPCName: "submit_export_job"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "一站式导出 axls 为 xlsx（内部提交+轮询，可选下载）。",
				UseWhen:      []string{"需要把在线电子表格导出为 Excel 文件或拿到 downloadUrl 时"},
				AvoidWhen:    []string{"禁止用 range read 拼 xlsx；本地已有 xlsx 节点用 doc download；Agent 不要外层再轮询"},
				Examples:     []string{"dws sheet export --node <NODE_ID> --output ./report.xlsx"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "node", Property: "nodeId"},
			},
		},
	})
	// 建表带初始数据、csv 同步导出都是与既有叶子不同的接口种类（一条 composite
	// 编排、一条 get_range_as_csv 直连），各自独立成叶子，不挂到 create / export 上。
	createWithDataCmd := newSheetCreateWithDataCmd()
	exportCsvCmd := newSheetExportCsvCmd()
	importCmd := newSheetImportCmd()
	templateCmd := newSheetTemplateCmd()
	tableCmds := newTableCmds()
	pivotTableCmd := newPivotTableCmd()
	revisionCmds := newSheetRevisionCmds()

	batchUpdateCmd := newBatchUpdateCmd()
	DeclareLeafMetadata(batchUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "batch_update",
				CanonicalPath:  "sheet.batch_update",
				CLIPath:        "sheet batch-update",
				PrimaryCLIPath: "sheet batch-update",
			},
			Description: "批量打包多个写操作原子执行（可含清除/删除，需确认后加 --yes）。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量打包多个写操作原子执行（可含清除/删除，需确认后加 --yes）。",
				UseWhen:      []string{"用户明确要求把多个已审查写操作作为一批执行时"},
				AvoidWhen:    []string{"单操作请用对应原子命令；仅预览用 --dry-run，不要在未确认时加 --yes"},
				Examples:     []string{"dws sheet batch-update --node <NODE_ID> --operations '[{\"toolName\":\"range clear\",\"input\":{\"sheet-id\":\"Sheet1\",\"range\":\"A1:B3\",\"type\":\"content\"}}]'"},
			},
		},
	})
	rangeBatchClearCmd := newRangeBatchClearCmd()
	DeclareLeafMetadata(rangeBatchClearCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "range_batch_clear",
				CanonicalPath:  "sheet.range_batch_clear",
				CLIPath:        "sheet range batch-clear",
				PrimaryCLIPath: "sheet range batch-clear",
			},
			Description: "批量清除多个区域（原子事务，需确认后加 --yes）。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量清除多个区域（原子事务，需确认后加 --yes）。",
				UseWhen:      []string{"需要一次清除多个互不相关的区域，并希望同批提交时"},
				AvoidWhen:    []string{"只清一个区域用 range clear；混有写入/合并等操作用 batch-update"},
				Examples:     []string{"dws sheet range batch-clear --node NODE_ID --ranges '[\"Sheet1!A1:B3\",\"Sheet2!C1:D5\"]'"},
			},
		},
	})
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
	standaloneCmds = append(standaloneCmds, revisionCmds...)
	standaloneCmds = append(standaloneCmds, exportCmd, exportCsvCmd, importCmd, batchUpdateCmd, createWithDataCmd)

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
	root.AddCommand(newSheetVersionCmd(), newSheetCommentCmd(), newSheetFormulaVerifyCmd())

	// This is the reviewed runtime counterpart of the final Sheet Schema
	// confirmation=user_required set. It is intentionally command-local: there
	// is no Cobra-wide or Schema-driven interceptor. The app-level delivery gate
	// verifies this exact protected set against the final typed SchemaRegistry.
	confirmationGuards := []struct {
		path       string
		operation  string
		targetHint string
	}{
		{path: "batch-update", operation: "批量更新", targetHint: "文档、子操作及影响范围"},
		{path: "chart delete", operation: "删除浮动图表", targetHint: "文档、工作表和图表"},
		{path: "range clear", operation: "清除工作表区域", targetHint: "工作表、清除范围和清除类型"},
		{path: "cond-format delete", operation: "删除条件格式规则", targetHint: "文档、工作表和规则"},
		{path: "delete-dimension", operation: "删除行或列", targetHint: "工作表、维度、起始位置和数量"},
		{path: "delete-dropdown", operation: "删除下拉列表", targetHint: "工作表和单元格范围"},
		{path: "filter delete", operation: "删除全局筛选", targetHint: "文档和工作表"},
		{path: "filter-view delete", operation: "删除筛选视图", targetHint: "文档、工作表和筛选视图"},
		{path: "delete-float-image", operation: "删除浮动图片", targetHint: "文档、工作表和浮动图片"},
		{path: "pivot-table delete", operation: "删除透视表", targetHint: "文档、工作表和透视表"},
		{path: "delete-sheet", operation: "删除工作表", targetHint: "文档和工作表"},
		{path: "filter-view delete-criteria", operation: "删除筛选视图列条件", targetHint: "文档、工作表、筛选视图和列"},
		{path: "range batch-clear", operation: "批量清除工作表区域", targetHint: "文档、工作表、清除范围和清除类型"},
		{path: "range move-to", operation: "移动工作表区域", targetHint: "源工作表范围和目标位置"},
		{path: "version revert", operation: "回滚表格版本", targetHint: "文档和目标版本号"},
		{path: "comment delete", operation: "删除单元格评论", targetHint: "文档和评论 commentKey"},
	}
	for _, guard := range confirmationGuards {
		attachSheetConfirmationGuard(root, guard.path, guard.operation, guard.targetHint)
	}

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

func attachSheetConfirmationGuard(root *cobra.Command, path, operation, targetHint string) {
	command, remaining, err := root.Find(strings.Fields(path))
	if err != nil || command == nil || len(remaining) != 0 || command == root {
		panic(fmt.Sprintf("attach Sheet confirmation guard %q: command not found (remaining=%v, err=%v)", path, remaining, err))
	}
	protectSheetMutationCommand(command, operation, targetHint)
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

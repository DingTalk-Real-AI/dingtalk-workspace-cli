package helpers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ============================================================================
// sheet create-with-data：创建表格并写入初始数据（可选样式）的编排
//
// 这是多步流程，没有单一 MCP 工具承载：
//   create_workspace_sheet → 探活 → 定位默认工作表 → 写数据 → 回读校验 → 应用样式
// 所有结构/枚举校验都在发第一个请求之前完成，避免留下白建的空文档。
//
// 之所以独立成一条命令而不是给 sheet create 加 flag：sheet create 的叶子契约是
// interface_mode=mcp + interface_ref=create_workspace_sheet，即"一次直接 RPC"。
// 把编排挂到那条叶子上会让 Schema 消费者以为 --values / --sheets / --styles 都是
// create_workspace_sheet 的入参，接口审计、参数映射、后端能力判断全都拿到错误信息。
// 本叶子如实声明 interface_mode=composite（composite 不得带 interface_ref，必须给
// Reason），sheet create 则保持原样只做一次 RPC。
// ============================================================================

func newSheetCreateWithDataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-with-data",
		Short: "创建钉钉表格文档并写入初始数据（可选样式）",
		Long: `创建一篇新的钉钉在线电子表格，并在创建后写入初始数据、可选地应用样式。

创建空表格请用 dws sheet create（单次调用）；本命令是多步编排：
  create_workspace_sheet → 等待文档就绪 → 定位默认工作表 → 写入数据 → 回读校验 → 应用样式
create / write / style 是多个远程步骤，不是原子事务。创建后的失败会返回已创建的
nodeId、已探活的 sheetId（若有）、失败阶段和提交状态；先读回确认现状并只补失败步骤，
不要整体重跑 create-with-data，以免生成重复工作簿。

创建位置优先级: --folder > --workspace > 默认 (我的文档根目录)

初始数据（--values 与 --sheets 二选一，必须给一个）：
  --values   二维 JSON 数组，裸值写入默认工作表 A1 起（单表快速建表，无表头/类型语义，
             复用 csv-put 通道，自动识别数字/布尔）。单元格只能是字符串/数字/布尔/null；
             上限 30000 单元格、编码为 CSV 后 2000000 字符
  --sheets   typed table 数组，多工作表一次写入（复用 table-put 通道）。每个条目形如
             {"name":"表名","columns":["列1","列2"],"data":[[...]],"dtypes":{...},"formats":{...},"cellStyles":[...]}
             name、columns 必填；第一个条目写入默认工作表（自动重命名为其 name），其余按 name 自动新建。
             字段名为 camelCase，只接受 name / columns / data / dtypes / formats / cellStyles /
             startCell / mode / header / allowOverwrite（写错的键会被服务端静默丢弃，故一律拒绝；
             不接受 sheetId：文档尚未创建）。data 每行长度须等于 columns，单元格只能是
             字符串/数字/布尔/null；dtypes、formats 的键须是 columns 里的列名（按 trim 后比较）。
             单表写入上限 30000 单元格（含表头行）。

样式配置（--styles，可选；顶层键对齐飞书 snake_case，列表项内字段兼容 camelCase；
两级都拒绝未知键，避免写错的键被静默丢弃导致样式只应用一半）：
  {"styles":[{"name":"表名",
    "cell_styles":[{"range":"A1:D1","font_weight":"bold","background_color":"#FFF2CC",
                    "font_family":"微软雅黑","number_format":"@",
                    "border_styles":{"bottom":{"style":"medium"}}}],
    "row_sizes":[{"range":"1:1","type":"pixel","size":28}],
    "col_sizes":[{"range":"A:D","type":"pixel","size":120}],
    "cell_merges":[{"range":"A1:B1","merge_type":"all"}]}]}
  - 每项至少给 cell_styles / row_sizes / col_sizes / cell_merges 之一
  - 配 --sheets 时 styles 的项数/顺序/name 必须与 --sheets 子表一一对应；配 --values 时只给 1 项（name 忽略）
  - 数据写入后按 cell_styles → row_sizes → col_sizes → cell_merges 顺序执行（非原子）
  - row_sizes 的 type：pixel（需 size）/ standard（恢复默认行高）/ auto（按内容自适应）
  - col_sizes 的 type：pixel（需 size）/ standard（恢复默认列宽）——与飞书一致，列宽不提供 auto
  - merge_type 取 all/rows/columns`,
		Example: `  # 创建并写入初始数据（默认工作表，裸二维值）
  dws sheet create-with-data --name "名单" --values '[["姓名","分数"],["张三","90"]]'

  # 创建多个带数据的工作表（typed table）
  dws sheet create-with-data --name "报表" --sheets '[{"name":"一月","columns":["项目","金额"],"data":[["房租",5000]]},{"name":"二月","columns":["项目","金额"],"data":[["房租",5000]]}]'

  # 创建 + 写数据 + 一并应用样式（表头加粗黄底、行高、列宽、合并）
  dws sheet create-with-data --name "带样式" --values '[["姓名","分数"],["张三","90"]]' \
    --styles '{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:B1","font_weight":"bold","background_color":"#FFF2CC"}],"row_sizes":[{"range":"1:1","type":"pixel","size":28}],"col_sizes":[{"range":"A:B","type":"pixel","size":120}]}]}'

  # 指定创建位置
  dws sheet create-with-data --name "Q1 数据" --folder FOLDER_ID --values '[["月份","金额"],["1月",100]]'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			valuesStr, _ := cmd.Flags().GetString("values")
			sheetsStr, _ := cmd.Flags().GetString("sheets")
			stylesStr, _ := cmd.Flags().GetString("styles")
			if valuesStr == "" && sheetsStr == "" {
				return fmt.Errorf("--values 与 --sheets 必须给一个（只建空表格请用 dws sheet create）")
			}
			if valuesStr != "" && sheetsStr != "" {
				return fmt.Errorf("--values 与 --sheets 二选一，不能同时指定")
			}
			createArgs := map[string]any{
				"name": mustGetFlag(cmd, "name"),
			}
			if v, _ := cmd.Flags().GetString("folder"); v != "" {
				createArgs["folderId"] = v
			}
			if v := flagOrFallback(cmd, "workspace", "workspace-id"); v != "" {
				createArgs["workspaceId"] = v
			}
			return runCreateSheetWithData(cmd, createArgs, valuesStr, sheetsStr, stylesStr)
		},
	}
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "sheet",
				Name:           "create_with_data",
				CanonicalPath:  "sheet.create_with_data",
				CLIPath:        "sheet create-with-data",
				PrimaryCLIPath: "sheet create-with-data",
			},
			Description: "创建钉钉在线电子表格文档（axls）并写入初始数据，可选一并应用样式。",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes: []contract.ResultOutcome{
					contract.ResultOutcomeSuccess,
					contract.ResultOutcomeFailure,
				},
				DataSchema: json.RawMessage(`{"type":"object","description":"已创建并完成初始数据回读校验的在线电子表格；顶层字段供新消费者使用，旧 result 包装与外层 requestId 原样兼容","properties":{"nodeId":{"type":"string","description":"新建在线电子表格的稳定节点 ID"},"sheetId":{"type":"string","description":"探活阶段确认可写并供后续命令直接复用的默认工作表 ID"},"docUrl":{"type":"string","description":"新建在线电子表格的访问链接"},"result":{"description":"上游存在时原样兼容的旧结果字段；对象包装内包含同一 nodeId / sheetId"},"requestId":{"description":"上游响应存在时保留的请求 ID"}},"required":["nodeId","sheetId"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed composite workflow: the command calls sheet/create_workspace_sheet, waits for the new document to become writable, resolves the default worksheet, writes the initial data through sheet/set_range_from_csv or sheet/table_put, reads it back with sheet/get_range_as_csv, and optionally applies sheet/set_cell_range, sheet/update_dimension and sheet/merge_cells; no single pinned RPC represents the workflow.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建钉钉在线电子表格文档（axls）并写入初始数据，可选一并应用样式。",
				UseWhen:      []string{"需要新建一份钉钉在线电子表格并在建表时就带上初始数据（可含表头样式、行高列宽、合并）时"},
				AvoidWhen:    []string{"只要空表格用 sheet create；往已有表格写数据用 sheet csv-put / table-put；只改样式用 sheet range set-style"},
				Examples: []string{
					"dws sheet create-with-data --name \"名单\" --values '[[\"姓名\",\"分数\"],[\"张三\",90]]'",
					"dws sheet create-with-data --name \"报表\" --sheets '[{\"name\":\"一月\",\"columns\":[\"项目\",\"金额\"],\"data\":[[\"房租\",5000]]}]'",
				},
			},
			// name 由 mustGetFlag 校验；这里发布 required 供 Schema 消费者识别
			Parameters: []contract.ParamDecl{
				{Name: "name", Required: boolPtr(true)},
			},
		},
	})
	cmd.Flags().String("name", "", "表格名称 (必填)")
	cmd.Flags().String("folder", "", "目标文件夹 ID 或 URL")
	cmd.Flags().String("workspace", "", "目标知识库 ID")
	cmd.Flags().String("values", "", "初始数据，二维 JSON 数组，写入默认工作表 (与 --sheets 二选一)")
	cmd.Flags().String("sheets", "", `多工作表 typed table JSON，如 '[{"name":"表名","columns":[...],"data":[[...]]}]' (与 --values 二选一)`)
	cmd.Flags().String("styles", "", `写入数据后一并应用的视觉处理 JSON（对齐飞书）：{"styles":[{"name":"表名","cell_styles":[{"range":"A1:D1","font_weight":"bold"}],"row_sizes":[{"range":"1:1","type":"pixel","size":28}],"col_sizes":[{"range":"A:D","type":"pixel","size":120}],"cell_merges":[{"range":"A1:B1","merge_type":"all"}]}]}`)
	return cmd
}

func runCreateSheetWithData(cmd *cobra.Command, createArgs map[string]any, valuesStr, sheetsStr, stylesStr string) error {
	// 先解析初始数据，避免创建了空文档后才发现 JSON 非法
	var values [][]any
	var sheetSpecs []map[string]any
	// 分流判据统一用 valuesStr（flag 是否给了），不能用 values != nil：
	// `--values null` 是合法 JSON 且解析出 nil，两者不等价。
	useValues := valuesStr != ""
	if useValues {
		// UseNumber 保留数字字面量，不经过 float64：普通 Unmarshal 会把超过 2^53
		// 的整数舍入（雪花 ID 1234567890123456789 变成 ...768），而回读只校验
		// 单元格非空，这种篡改会被当成写入成功。订单号、雪花 ID 都是表格常见数据。
		dec := json.NewDecoder(strings.NewReader(valuesStr))
		dec.UseNumber()
		if err := dec.Decode(&values); err != nil {
			return fmt.Errorf("--values JSON 解析失败: %w", err)
		}
		if err := requireJSONEOF(dec, "--values"); err != nil {
			return err
		}
		// null / [] 都写不出任何数据，必须在建文档之前拒掉，
		// 否则会白建一个空文档再由回读校验报成"写入未生效"。
		if len(values) == 0 {
			return fmt.Errorf("--values 不能为空（需要形如 '[[\"姓名\",\"分数\"],[\"张三\",90]]' 的二维数组）")
		}
		if err := validateValuesCells(values); err != nil {
			return err
		}
		if !valuesHaveContent(values) {
			return fmt.Errorf("--values 全部单元格为空，写不出任何数据（需要至少一个非空单元格）")
		}
		if err := validateValuesBudget(values); err != nil {
			return err
		}
	} else {
		specs, err := parseCreateSheetSpecs(sheetsStr)
		if err != nil {
			return err
		}
		sheetSpecs = specs
	}

	// --styles 也先解析校验（含与 --sheets 的数量/顺序/name 一致性）
	var styleOps []sheetStyleOps
	if stylesStr != "" {
		var expected []string
		for _, s := range sheetSpecs {
			name, _ := s["name"].(string)
			expected = append(expected, name)
		}
		ops, err := parseCreateStyles(stylesStr, expected)
		if err != nil {
			return err
		}
		// 前置校验：在创建文档之前把每项的结构/枚举问题全部暴露（快速失败，避免白建空文档）
		for i, o := range ops {
			if _, err := planStyleOps("", "", o); err != nil {
				return fmt.Errorf("--styles[%d]: %w", i, err)
			}
		}
		styleOps = ops
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if deps.Caller.DryRun() {
		deps.Out.PrintKeyValue("操作", "创建表格并写入初始数据")
		deps.Out.PrintKeyValue("名称", fmt.Sprintf("%v", createArgs["name"]))
		return nil
	}

	// json 模式下进度提示会污染 stdout，末尾统一输出创建结果 JSON。
	jsonMode := deps.Caller.Format() == "json"
	progress := func(msg string) {
		if !jsonMode {
			deps.Out.PrintInfo(msg)
		}
	}

	progress("创建表格文档 ...")
	createText, err := callMCPToolReturnText(ctx, "create_workspace_sheet", createArgs)
	if err != nil {
		return fmt.Errorf("创建表格失败: %w", err)
	}
	nodeID, err := parseCreatedNodeID(createText)
	if err != nil {
		return err
	}
	progress(fmt.Sprintf("表格已创建: nodeId=%s，等待文档就绪 ...", nodeID))

	// 新建文档服务端仍在初始化，需先探活；否则写入可能返回成功但数据不落盘
	defaultSheetID, err := waitSheetWritable(ctx, nodeID)
	if err != nil {
		return sheetCreateWithDataStateError(
			nodeID, "", "wait_writable", "sheet_create_with_data_initialization_failed", "partial_success",
			fmt.Sprintf("表格已创建 (nodeId=%s)，但等待文档就绪失败", nodeID), err, nil,
		)
	}
	progress("开始写入初始数据 ...")

	if useValues {
		if err := writeValuesToSheet(ctx, nodeID, defaultSheetID, values); err != nil {
			return sheetCreateWithDataStateError(
				nodeID, defaultSheetID, "write", "sheet_create_with_data_write_commit_unknown", "unknown",
				fmt.Sprintf("表格已创建 (nodeId=%s)，但写入数据失败，服务端提交状态未知", nodeID), err, nil,
			)
		}
		// 回读校验：防"返回成功但未落盘"（新建文档初始化竞态）。
		// 必须校验输入里第一个**非空**的单元格，不能死盯 A1 ——
		// [["","姓名"],[1,"张三"]] 这类首格为空的合法数据会被误报成写入失败。
		probeCell := firstNonEmptyValuesCell(values)
		if err := verifyRangeNotEmpty(ctx, nodeID, defaultSheetID, probeCell); err != nil {
			return sheetCreateWithDataStateError(
				nodeID, defaultSheetID, "verify_write", "sheet_create_with_data_write_verification_failed", "unknown",
				fmt.Sprintf("表格已创建 (nodeId=%s)，但初始数据写入未生效", nodeID), err,
				map[string]any{"probeRange": probeCell, "writeRequestSucceeded": true},
			)
		}
	} else {
		// 复用默认工作表承载第一个 sheet：先把默认表重命名为 sheets[0].name，
		// 之后 table_put 按 name 命中它并写入；其余 sheet 由 table_put 自动创建。
		// 避免残留一张空的默认工作表。
		firstName, _ := sheetSpecs[0]["name"].(string)
		if err := callMCPToolSilent(ctx, "update_sheet", map[string]any{
			"nodeId": nodeID, "sheetId": defaultSheetID, "title": firstName,
		}); err != nil {
			return sheetCreateWithDataStateError(
				nodeID, defaultSheetID, "rename_default_sheet", "sheet_create_with_data_rename_commit_unknown", "unknown",
				fmt.Sprintf("表格已创建 (nodeId=%s)，但重命名默认工作表失败", nodeID), err,
				map[string]any{"targetSheetName": firstName},
			)
		}
		if err := callMCPToolSilent(ctx, "table_put", map[string]any{
			"nodeId": nodeID,
			"sheets": sheetSpecs,
		}); err != nil {
			return sheetCreateWithDataStateError(
				nodeID, defaultSheetID, "write", "sheet_create_with_data_write_commit_unknown", "unknown",
				fmt.Sprintf("表格已创建 (nodeId=%s)，但写入初始数据失败，服务端提交状态未知", nodeID), err,
				map[string]any{"sheetCount": len(sheetSpecs)},
			)
		}
		// 逐表回读校验：table_put 可能返回成功但在新建文档初始化竞态下数据未落盘，
		// 与 --values 分支同源的问题。table_put 会按 name 复用/新建工作表，因此
		// 重取一次 name→sheetId 映射，再按每个 spec 的首个预期非空单元格回读。
		sheetIDByName, err := resolveSheetIDsByName(ctx, nodeID)
		if err != nil {
			return sheetCreateWithDataStateError(
				nodeID, defaultSheetID, "verify_write", "sheet_create_with_data_write_verification_failed", "unknown",
				fmt.Sprintf("表格已创建 (nodeId=%s)，数据已提交但无法回读校验（获取工作表列表失败）", nodeID), err,
				map[string]any{"writeRequestSucceeded": true},
			)
		}
		for _, spec := range sheetSpecs {
			name, _ := spec["name"].(string)
			probeCell, hasContent := firstNonEmptySheetSpecCell(spec)
			if !hasContent {
				continue // 只有 name、无 columns/data 的工作表本就应为空，不回读
			}
			sid, ok := sheetIDByName[name]
			if !ok {
				return sheetCreateWithDataStateError(
					nodeID, defaultSheetID, "verify_write", "sheet_create_with_data_sheet_missing_after_write", "unknown",
					fmt.Sprintf("表格已创建 (nodeId=%s)，但写入后未找到工作表 %q，其初始数据可能未落盘", nodeID, name),
					fmt.Errorf("工作表 %q 不在写后列表中", name),
					map[string]any{"sheetName": name, "writeRequestSucceeded": true},
				)
			}
			if err := verifyRangeNotEmpty(ctx, nodeID, sid, probeCell); err != nil {
				return sheetCreateWithDataStateError(
					nodeID, defaultSheetID, "verify_write", "sheet_create_with_data_write_verification_failed", "unknown",
					fmt.Sprintf("表格已创建 (nodeId=%s)，但工作表 %q 的初始数据写入未生效", nodeID, name), err,
					map[string]any{"sheetName": name, "targetSheetId": sid, "probeRange": probeCell, "writeRequestSucceeded": true},
				)
			}
		}
	}

	progress("初始数据写入完成。")

	// --styles：数据写入后按 cell_styles → row_sizes → col_sizes → cell_merges 顺序应用
	if len(styleOps) > 0 {
		progress("应用样式配置 ...")
		for i, ops := range styleOps {
			// --values 模式只有一项，作用于默认表；--sheets 模式按顺序对应各子表（name 已校验一致）
			targetSheet := defaultSheetID
			if !useValues {
				targetSheet = ops.Name
			}
			if err := applyStyleOps(ctx, nodeID, targetSheet, ops); err != nil {
				return sheetCreateWithDataStateError(
					nodeID, defaultSheetID, "apply_styles", "sheet_create_with_data_style_commit_unknown", "partial_success",
					fmt.Sprintf("表格已创建且数据已写入 (nodeId=%s)，但 --styles[%d] 应用失败", nodeID, i), err,
					map[string]any{"dataWritten": true, "failedStyleIndex": i, "targetSheet": targetSheet},
				)
			}
		}
		progress("样式应用完成。")
	}
	// 探活已经拿到默认工作表 ID；将它并入创建回执，供后续读写直接复用。
	// 这里只投影本地已有事实，不再调用 get_all_sheets。
	createResult, err := projectCreatedSheetResult(createText, nodeID, defaultSheetID)
	if err != nil {
		return sheetCreateWithDataStateError(
			nodeID, defaultSheetID, "project_result", "sheet_create_with_data_result_projection_failed", "partial_success",
			fmt.Sprintf("表格已创建且初始数据已写入 (nodeId=%s)，但构造结果失败", nodeID), err,
			map[string]any{"dataWritten": true},
		)
	}
	deps.Out.PrintRaw(createResult)
	return nil
}

// sheetCreateWithDataStateError describes the recoverable state of this
// non-transactional workflow. Once create_workspace_sheet succeeds, replaying
// the whole command can duplicate a workbook. The error therefore carries the
// stable resource identities and explicitly disables automatic retry.
func sheetCreateWithDataStateError(nodeID, sheetID, stage, reason, status, message string, cause error, extra map[string]any) error {
	details := map[string]any{
		"contractVersion": "sheet.create_with_data.v1",
		"status":          status,
		"nodeId":          nodeID,
		"failedStage":     stage,
		"created":         true,
		"retryCreateSafe": false,
	}
	if sheetID != "" {
		details["sheetId"] = sheetID
	}
	for key, value := range extra {
		details[key] = value
	}
	if cause != nil {
		message = fmt.Sprintf("%s: %v", message, cause)
	}
	return apperrors.NewAPI(
		message,
		apperrors.WithOperation("sheet.create_with_data"),
		apperrors.WithReason(reason),
		apperrors.WithFailureStage(stage),
		apperrors.WithExecutionStarted(true),
		apperrors.WithRetryable(false),
		apperrors.WithActions(
			"使用 details.nodeId / details.sheetId 读取当前工作簿并确认实际状态",
			"只补做失败或缺失的步骤；不要重新执行 create-with-data",
		),
		apperrors.WithDetails(details),
		apperrors.WithCause(cause),
	)
}

// parseCreateSheetSpecs 解析 --sheets 为 table_put 的 sheet spec 数组，并校验每个条目带 name。
// 接受 JSON 数组、{"sheets":[...]} 或单个 spec 对象。
func parseCreateSheetSpecs(sheetsStr string) ([]map[string]any, error) {
	var payload any
	// 同 --values：UseNumber 保留数字字面量。specs 原样转发给 table_put，
	// 若经过 float64 中转，records/data 里的大整数会在发出前就被改掉
	// （1234567890123456789 变成 1234567890123456800）。
	dec := json.NewDecoder(strings.NewReader(sheetsStr))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("--sheets JSON 解析失败: %w", err)
	}
	if err := requireJSONEOF(dec, "--sheets"); err != nil {
		return nil, err
	}
	var arr []any
	switch v := payload.(type) {
	case []any:
		arr = v
	case map[string]any:
		// 只要出现 sheets 键就按包装对象处理：畸形包装（{"sheets":"bad","name":"一月"}）
		// 必须报错，不能退化成「把整个对象当单个 spec」—— 那样 sheets 字段会被当成
		// 未知字段，用户以为在传一组子表，实际只建了一张。
		if inner, wrapped := v["sheets"]; wrapped {
			s, ok := inner.([]any)
			if !ok {
				return nil, fmt.Errorf("--sheets.sheets 必须是数组，实际是 %T", inner)
			}
			arr = s
		} else {
			arr = []any{v} // 单个 spec 对象
		}
	default:
		return nil, fmt.Errorf("--sheets 必须是 JSON 数组、{\"sheets\":[...]} 或单个 sheet spec 对象")
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("--sheets 不能为空数组")
	}
	specs := make([]map[string]any, 0, len(arr))
	seen := make(map[string]int, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("--sheets[%d] 不是对象", i)
		}
		if v, present := m["name"]; present && v != nil {
			if _, isStr := v.(string); !isStr {
				return nil, fmt.Errorf("--sheets[%d].name 必须是字符串，实际是 %T", i, v)
			}
		}
		name, _ := m["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("--sheets[%d] 缺少必填的 name 字段（创建带数据时每个工作表必须命名）", i)
		}
		// 工作表名不能重复：样式接口按「ID 或名称」定位工作表，重名时命中哪一张
		// 由服务端决定，--styles 会作用到不确定的工作表上。而 --styles 又是建
		// 文档之后的非原子步骤，所以必须在建文档之前拒掉。
		if prev, dup := seen[name]; dup {
			return nil, fmt.Errorf("--sheets[%d].name=%q 与 --sheets[%d] 重复；工作表名必须唯一，否则 --styles 按名称定位时会落到不确定的工作表上", i, name, prev)
		}
		seen[name] = i
		// name 之外的字段同样要在建文档之前按 table_put 的输入契约验一遍
		if err := validateCreateSheetSpec(m); err != nil {
			return nil, fmt.Errorf("--sheets[%d]: %w", i, err)
		}
		specs = append(specs, m)
	}
	return specs, nil
}

// ── --sheets：table_put 输入契约的前置校验 ────────────────────────────────────
//
// sheet spec 原样转发给 table_put，字段契约见 MCP 的 SheetTableSpec 与引擎侧
// TablePutOptions：
//
//	name / startCell / mode / header / allowOverwrite /
//	columns（必填、非空、列名非空且不重复）/ data / dtypes / formats / cellStyles
//
// 只校验「对象类型 + name」是不够的：
//   - 类型不对的字段（data 传成对象、columns 传成字符串）要等到 table_put 才被
//     服务端拒绝，此时 create_workspace_sheet 与 update_sheet 已经执行，留下一份
//     用户并没有请求到的空/半成品文档，且无法回滚。
//   - 拼错的字段更糟：MCP 侧反序列化到 SheetTableSpec 会静默丢掉不认识的键，
//     `datas` 会变成「只写了表头、数据全丢」，而回读探针刚好落在表头上，
//     整条命令报成功。
//
// 因此把契约里的每个已提供字段都在发第一个请求之前验完。

// requireJSONEOF 确认解码器在读出第一个 JSON 值之后就到了输入末尾。
//
// json.Decoder.Decode 只读一个值就返回，`[[1]] trailing` 会被当成合法矩阵：
// 粘贴多了一段、shell 拼接残留、把两个 JSON 首尾相连，都会静默丢掉后半截并照常
// 建文档。创建不可原子回滚，所以这类输入必须在发第一个请求之前拒掉，而不是建完
// 文档只写进去半份数据。（--styles 走 json.Unmarshal，它本身就拒绝多余内容。）
func requireJSONEOF(dec *json.Decoder, flag string) error {
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s 只接受单个 JSON 值，末尾有多余内容", flag)
	}
	return nil
}

const maxTableWriteCells = 30000

// tablePutSpecFields 是 table_put 单个 sheet spec 的合法字段（契约字段名为 camelCase）。
// sheetId 虽在契约内但不在此列：见 validateCreateSheetSpec。
var tablePutSpecFields = []string{
	"name", "startCell", "mode", "header", "allowOverwrite",
	"columns", "data", "dtypes", "formats", "cellStyles",
}

// looseFieldKey 归一化字段名：去掉下划线/连字符并转小写。
func looseFieldKey(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(s))
}

// rejectUnknownFields 校验对象只含契约里的字段名。allowed 列出所有被接受的拼写
// （同一字段的多种别名都要列出，首个视为规范拼写）；归一化后能对上的键判为拼写
// 偏差并直接指向规范拼写，其余判为未知字段。
//
// 之所以要拒而不是忽略：这些 JSON 最终交给「按固定字段名取值」的下游（服务端
// DTO 或 pickStr），不认识的键会被静默丢掉——写错一个键的后果是数据/样式部分丢失
// 而命令报成功，比直接报错难发现得多。
func rejectUnknownFields(item map[string]any, what string, allowed []string) error {
	exact := make(map[string]bool, len(allowed))
	canonicalByLoose := make(map[string]string, len(allowed))
	for _, f := range allowed {
		exact[f] = true
		if _, dup := canonicalByLoose[looseFieldKey(f)]; !dup {
			canonicalByLoose[looseFieldKey(f)] = f
		}
	}
	canonical := make([]string, 0, len(canonicalByLoose))
	for _, f := range allowed {
		if canonicalByLoose[looseFieldKey(f)] == f {
			canonical = append(canonical, f)
		}
	}
	for _, key := range sortedMapKeys(item) {
		if exact[key] {
			continue
		}
		if want, ok := canonicalByLoose[looseFieldKey(key)]; ok {
			return fmt.Errorf("字段 %q 拼写不符合契约，应为 %q", key, want)
		}
		return fmt.Errorf("未知字段 %q（%s只接受 %s）", key, what, strings.Join(canonical, " / "))
	}
	return nil
}

// normalizeSpecStartCell 把 startCell 归一化成 parseA1Cell 认的形态：服务端解析
// 前会 toUpperCase，且允许 $ 绝对引用（$C$3 等价于 C3）。本地校验与回读探针必须与
// 服务端同一套解释——否则要么误拒服务端接受的写法，要么探到错位的单元格。
func normalizeSpecStartCell(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "$", ""))
}

// sortedMapKeys 返回排序后的键，保证校验顺序与错误信息稳定（map 遍历是随机的）。
func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// validateCreateSheetSpec 按 table_put 的输入契约校验单个 sheet spec 的每个已提供
// 字段。纯函数、不发请求：调用方在创建文档之前调用，非法输入不会产生任何 MCP 调用。
func validateCreateSheetSpec(spec map[string]any) error {
	// sheetId 在 table_put 契约里合法（且优先级高于 name），但对 create-with-data
	// 没有意义：文档此刻还不存在，任何 sheetId 都不可能是它的工作表。传了只会
	// 让写入落到别处或直接失败，而重命名默认表、回读校验都是按 name 定位的。
	for _, key := range sortedMapKeys(spec) {
		if looseFieldKey(key) == "sheetid" {
			return fmt.Errorf("不支持 sheetId：文档尚未创建，工作表 ID 无从得知；请只用 name 指定工作表")
		}
	}
	if err := rejectUnknownFields(spec, "sheet spec ", tablePutSpecFields); err != nil {
		return err
	}

	columns, err := validateSpecColumns(spec)
	if err != nil {
		return err
	}
	dataRows, err := validateSpecData(spec, columns)
	if err != nil {
		return err
	}
	for _, field := range []string{"dtypes", "formats"} {
		if err := validateSpecColumnMap(spec, field, columns); err != nil {
			return err
		}
	}
	if v, ok := spec["cellStyles"]; ok && v != nil {
		switch v.(type) {
		case map[string]any, []any:
		default:
			return fmt.Errorf("cellStyles 必须是对象或二维数组，实际是 %T", v)
		}
	}
	// mode / startCell 留空等价于不传（服务端按 blank 处理），非空才校验取值。
	if v, ok := spec["mode"]; ok && v != nil {
		s, isStr := v.(string)
		if !isStr {
			return fmt.Errorf("mode 必须是字符串，实际是 %T", v)
		}
		if strings.TrimSpace(s) != "" && s != "overwrite" && s != "append" {
			return fmt.Errorf("mode=%q 非法（合法值: overwrite / append）", s)
		}
	}
	for _, field := range []string{"header", "allowOverwrite"} {
		if v, ok := spec[field]; ok && v != nil {
			if _, isBool := v.(bool); !isBool {
				return fmt.Errorf("%s 必须是布尔值，实际是 %T", field, v)
			}
		}
	}
	if v, ok := spec["startCell"]; ok && v != nil {
		s, isStr := v.(string)
		if !isStr {
			return fmt.Errorf("startCell 必须是字符串，实际是 %T", v)
		}
		if normalized := normalizeSpecStartCell(s); normalized != "" {
			if _, _, err := parseA1Cell(normalized); err != nil {
				return fmt.Errorf("startCell=%q 不是合法的单元格地址（应形如 A1，单个单元格）", s)
			}
		}
	}
	// 写入规模上限是服务端硬限，超限同样只会在 table_put 阶段报错。
	rows := dataRows
	if specWritesHeader(spec) {
		rows++
	}
	if total := rows * len(columns); total > maxTableWriteCells {
		return fmt.Errorf("单个工作表写入单元格总数上限为 %d（当前 %d 行 × %d 列 = %d）",
			maxTableWriteCells, rows, len(columns), total)
	}
	return nil
}

// validateSpecColumns 校验必填的 columns 并返回服务端实际使用的列名（已 trim）。
func validateSpecColumns(spec map[string]any) ([]string, error) {
	raw, ok := spec["columns"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("缺少必填的 columns（table_put 要求每个工作表给出非空的列名数组）")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("columns 必须是字符串数组，实际是 %T", raw)
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("columns 不能为空数组")
	}
	columns := make([]string, 0, len(list))
	seen := make(map[string]int, len(list))
	for i, item := range list {
		s, isStr := item.(string)
		if !isStr {
			return nil, fmt.Errorf("columns[%d] 必须是字符串，实际是 %T", i, item)
		}
		// 服务端按 trim 后的列名判重、写表头、并以其作为 dtypes/formats 的查表键，
		// 本地校验必须用同一套归一化，否则 ["量","量 "] 会通过本地检查再被服务端拒绝。
		name := strings.TrimSpace(s)
		if name == "" {
			return nil, fmt.Errorf("columns[%d] 不能为空字符串（列名不可为空）", i)
		}
		if prev, dup := seen[name]; dup {
			return nil, fmt.Errorf("columns[%d]=%q 与 columns[%d] 重复（列名不可重复）", i, name, prev)
		}
		seen[name] = i
		columns = append(columns, name)
	}
	return columns, nil
}

// validateSpecData 校验可选的 data 形状（二维、行宽等于 columns、单元格为标量），
// 返回数据行数供写入规模校验使用。data 缺省或为 null 等价于空数据。
func validateSpecData(spec map[string]any, columns []string) (int, error) {
	raw, ok := spec["data"]
	if !ok || raw == nil {
		return 0, nil
	}
	rows, ok := raw.([]any)
	if !ok {
		return 0, fmt.Errorf("data 必须是二维数组，实际是 %T", raw)
	}
	for i, r := range rows {
		row, isRow := r.([]any)
		if !isRow {
			return 0, fmt.Errorf("data[%d] 必须是数组，实际是 %T", i, r)
		}
		if len(row) != len(columns) {
			return 0, fmt.Errorf("data[%d] 有 %d 列，与 columns 的 %d 列不一致（每行长度必须与 columns 一致）",
				i, len(row), len(columns))
		}
		for j, cell := range row {
			if !isTableScalar(cell) {
				return 0, fmt.Errorf("data[%d][%d] 必须是字符串/数字/布尔/null，实际是 %T（不支持嵌套数组或对象）", i, j, cell)
			}
		}
	}
	return len(rows), nil
}

// validateSpecColumnMap 校验 dtypes / formats：必须是「列名 → 字符串」的对象，且键
// 必须是 columns 里的列名 —— 服务端按列名查表，键写错既不报错也不生效，静默失效
// 比报错更难发现。
func validateSpecColumnMap(spec map[string]any, field string, columns []string) error {
	raw, ok := spec[field]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s 必须是对象（列名 → 字符串），实际是 %T", field, raw)
	}
	known := make(map[string]bool, len(columns))
	for _, c := range columns {
		known[c] = true
	}
	for _, key := range sortedMapKeys(m) {
		if _, isStr := m[key].(string); !isStr {
			return fmt.Errorf("%s[%q] 必须是字符串，实际是 %T", field, key, m[key])
		}
		if !known[key] {
			return fmt.Errorf("%s 的键 %q 不是 columns 中的列名（服务端按列名查表，会静默忽略）", field, key)
		}
	}
	return nil
}

// isTableScalar 判断单元格取值是否落在契约允许的标量范围（string / number /
// boolean / null）。嵌套数组或对象会被服务端拒绝，或（--values 通道）被 fmt 写成
// "map[a:1]" 这类无意义文本。UseNumber 解码出的数字是 json.Number。
func isTableScalar(v any) bool {
	switch v.(type) {
	case nil, string, bool, json.Number, float64, int:
		return true
	}
	return false
}

// specWritesHeader 判断该 spec 是否会写入表头行：显式 header 优先；缺省时写表头
// （overwrite 默认写；append 落在空表上也写，而本命令的工作表都是新建空表）。
func specWritesHeader(spec map[string]any) bool {
	if v, ok := spec["header"].(bool); ok {
		return v
	}
	return true
}

// validateValuesCells 校验 --values 的单元格取值同样是标量。嵌套对象/数组会被
// cellToString 写成 "map[a:1]"，回读校验只看非空，于是垃圾数据被报成写入成功。
func validateValuesCells(values [][]any) error {
	for i, row := range values {
		for j, cell := range row {
			if !isTableScalar(cell) {
				return fmt.Errorf("--values[%d][%d] 必须是字符串/数字/布尔/null，实际是 %T"+
					"（不支持嵌套数组或对象；富格式单元格请用 sheet range update）", i, j, cell)
			}
		}
	}
	return nil
}

// validateValuesBudget 校验 --values 不超过 set_range_from_csv 的服务端硬限
// （行 × 列 ≤ 30000，CSV ≤ 2M 字符）。超限只会在写入阶段失败，而那时文档已经建好。
func validateValuesBudget(values [][]any) error {
	cols := 0
	for _, row := range values {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if total := len(values) * cols; total > maxCSVWriteCells {
		return fmt.Errorf("--values 单元格总数上限为 %d（当前 %d 行 × %d 列 = %d）",
			maxCSVWriteCells, len(values), cols, total)
	}
	if n := utf8.RuneCountInString(valuesToCSV(values)); n > maxCSVWriteChars {
		return fmt.Errorf("--values 编码为 CSV 后长度为 %d 字符，超过上限 %d", n, maxCSVWriteChars)
	}
	return nil
}

// set_range_from_csv 的服务端硬限：解析后行 × 列 ≤ 30000，csv 文本 ≤ 2M 字符。
const (
	maxCSVWriteCells = 30000
	maxCSVWriteChars = 2 * 1000 * 1000
)

// ── create-with-data --styles：对齐飞书 +workbook-create --styles 协议 ────────
//
// {"styles":[{"name":"子表名",
//
//	"cell_styles":[{"range":"A1:D1","font_weight":"bold","background_color":"#FFF2CC",
//	                "border_styles":{"bottom":{"style":"medium"}}}],
//	"row_sizes":[{"range":"1:1","type":"pixel","size":28}],
//	"col_sizes":[{"range":"A:D","type":"pixel","size":120}],
//	"cell_merges":[{"range":"A1:B1","merge_type":"all"}]}]}
//
// 顶层四个列表键与飞书一致（snake_case）；列表项内部的字段名同时兼容 DWS 的
// camelCase 写法。两处都拒绝未知键：这些 JSON 落到固定字段名的结构体/pickStr 上，
// 写错的键会被静默丢弃，导致样式只应用了一部分而命令报成功。
//
// styleItemFields / cellStyleItemFields / sizeItemFields / mergeItemFields 必须与
// sheetStyleOps 的 json tag、cellStyleItemToSpec 与 planStyleOps 里的取值键保持一致。
var (
	styleItemFields     = []string{"name", "cell_styles", "row_sizes", "col_sizes", "cell_merges"}
	cellStyleItemFields = []string{
		"range",
		"background_color", "backgroundColor", "bgColor",
		"font_color", "fontColor",
		"font_family", "fontFamily",
		"font_style", "fontStyle",
		"font_weight", "fontWeight",
		"font_line", "fontLine",
		"font_size", "fontSize",
		"horizontal_alignment", "horizontalAlignment", "hAlign",
		"vertical_alignment", "verticalAlignment", "vAlign",
		"word_wrap", "wordWrap",
		"number_format", "numberFormat",
		"border_styles", "borderStyles",
	}
	sizeItemFields  = []string{"range", "type", "size"}
	mergeItemFields = []string{"range", "merge_type", "mergeType"}
)

// sheetStyleOps 是 --styles 数组的单项：对应一张子表的视觉处理操作。
type sheetStyleOps struct {
	Name       string           `json:"name"`
	CellStyles []map[string]any `json:"cell_styles"`
	RowSizes   []map[string]any `json:"row_sizes"`
	ColSizes   []map[string]any `json:"col_sizes"`
	CellMerges []map[string]any `json:"cell_merges"`
}

func (o sheetStyleOps) isEmpty() bool {
	return len(o.CellStyles) == 0 && len(o.RowSizes) == 0 && len(o.ColSizes) == 0 && len(o.CellMerges) == 0
}

// pickStr 按多个候选键取字符串（snake_case 优先，兼容 camelCase）。
func pickStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// pickNum 按多个候选键取整数值（JSON 数字为 float64）。
//
// JSON 解码后所有数字都是 float64，直接 int(n) 会把 12.9 悄悄变成 12、把 1e20
// 截成 math.MaxInt64。文档和错误信息都写明这些字段必须是正整数，而 --styles
// 是不可原子回滚的复合流程，静默取整会留下与配置不符的表格。因此非整数、
// 非有限值、超出 int 范围一律报错，交由调用方在建文档之前拒绝。
func pickNum(m map[string]any, keys ...string) (int, bool, error) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			if math.IsNaN(n) || math.IsInf(n, 0) {
				return 0, true, fmt.Errorf("%s=%v 不是有限数值", k, n)
			}
			if n != math.Trunc(n) {
				return 0, true, fmt.Errorf("%s=%v 必须是整数，不接受小数（静默取整会得到与配置不符的结果）", k, n)
			}
			if n > math.MaxInt32 || n < math.MinInt32 {
				return 0, true, fmt.Errorf("%s=%v 超出取值范围", k, n)
			}
			return int(n), true, nil
		case int:
			return n, true, nil
		default:
			return 0, true, fmt.Errorf("%s 必须是数字，实际是 %T", k, v)
		}
	}
	return 0, false, nil
}

// feishuWordWrap 把飞书 word_wrap 枚举映射为引擎枚举（同时接受引擎原生值）。
func feishuWordWrap(v string) string {
	switch v {
	case "auto-wrap":
		return "autoWrap"
	case "word-clip":
		return "clip"
	default:
		return v // overflow / autoWrap / clip 原样
	}
}

// mergeCellsTypeEnum 是 merge_cells 接受的原生 mergeType 取值。
var mergeCellsTypeEnum = map[string]bool{
	"mergeAll": true, "mergeRows": true, "mergeColumns": true,
}

// feishuMergeType 把飞书 merge_type 映射为 MCP mergeType（同时接受原生值）。
//
// 未知值必须报错而不能原样透传：planStyleOps 是创建文档**之前**的最后一道枚举
// 校验，放过去的后果是「文档已建 → 数据已写 → cell_styles/row_sizes/col_sizes
// 已刷 → 最后 merge_cells 被服务端拒绝」，留下一个部分修改过的新文档，正是这段
// 编排声称要避免的非原子副作用。
func feishuMergeType(v string) (string, error) {
	switch v {
	case "", "all":
		return "mergeAll", nil
	case "rows":
		return "mergeRows", nil
	case "columns":
		return "mergeColumns", nil
	}
	if mergeCellsTypeEnum[v] {
		return v, nil
	}
	return "", fmt.Errorf("merge_type 非法: %q（合法值: all / rows / columns，或原生 mergeAll / mergeRows / mergeColumns）", v)
}

// cellStyleItemToSpec 把飞书 cell_styles 单项转为 styleSpec + range，复用 buildStyleCells 的校验。
func cellStyleItemToSpec(item map[string]any) (*styleSpec, string, error) {
	if err := rejectUnknownFields(item, "cell_styles 项", cellStyleItemFields); err != nil {
		return nil, "", err
	}
	rangeAddr := pickStr(item, "range")
	if rangeAddr == "" {
		return nil, "", fmt.Errorf("cell_styles 项缺少必填的 range")
	}
	spec := &styleSpec{
		BgColor:      pickStr(item, "background_color", "backgroundColor", "bgColor"),
		FontColor:    pickStr(item, "font_color", "fontColor"),
		FontFamily:   pickStr(item, "font_family", "fontFamily"),
		FontStyle:    pickStr(item, "font_style", "fontStyle"),
		FontWeight:   pickStr(item, "font_weight", "fontWeight"),
		FontLine:     pickStr(item, "font_line", "fontLine"),
		HAlign:       pickStr(item, "horizontal_alignment", "horizontalAlignment", "hAlign"),
		VAlign:       pickStr(item, "vertical_alignment", "verticalAlignment", "vAlign"),
		WordWrap:     feishuWordWrap(pickStr(item, "word_wrap", "wordWrap")),
		NumberFormat: pickStr(item, "number_format", "numberFormat"),
	}
	if n, ok, err := pickNum(item, "font_size", "fontSize"); err != nil {
		return nil, "", fmt.Errorf("cell_styles.%w", err)
	} else if ok {
		spec.FontSize = n
	}
	// border_styles 是对象，转回 JSON 字符串交给已有的 parseBorderStyles 校验。
	// 入参来自 json.Unmarshal，不含 channel/func/NaN，所以 Marshal 不会失败。
	for _, k := range []string{"border_styles", "borderStyles"} {
		if bs, ok := item[k]; ok && bs != nil {
			raw, _ := json.Marshal(bs)
			spec.BorderStylesJSON = string(raw)
			break
		}
	}
	return spec, rangeAddr, nil
}

// isAllLetters 判断字符串是否非空且只含英文字母（大小写皆可），用于列范围
// 预检：拒绝 "A5" / "Ax" 这类带数字或尾随字符、会被 parseA1Cell 误判的列名。
func isAllLetters(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

// parseRowColRange 解析行/列范围："1:3"→(start "1", len 3)；"A:C"→(start "A", len 3)；"1"/"A"→len 1。
func parseRowColRange(addr string, isRow bool) (start string, length int, err error) {
	addr = strings.TrimSpace(addr)
	if i := strings.Index(addr, "!"); i >= 0 {
		addr = addr[i+1:]
	}
	if addr == "" {
		return "", 0, fmt.Errorf("range 不能为空")
	}
	parts := strings.SplitN(addr, ":", 2)
	a := strings.TrimSpace(parts[0])
	b := a
	if len(parts) == 2 {
		b = strings.TrimSpace(parts[1])
	}
	if isRow {
		// strconv.Atoi 要求整个字符串都是合法整数，拒绝 "1x" / "2foo" 这类带尾随
		// 字符的输入。fmt.Sscanf("%d") 只消费前缀数字，会把 "1x" 静默当成第 1 行 ——
		// 这是建文档前的校验关口，放过后 update_dimension 会改到错误的行，且此时
		// 文档与数据已创建、无法原子回滚。
		r1, e1 := strconv.Atoi(a)
		r2, e2 := strconv.Atoi(b)
		if e1 != nil || e2 != nil || r1 < 1 || r2 < 1 {
			return "", 0, fmt.Errorf("行范围非法: %s（应形如 \"1:3\"）", addr)
		}
		if r2 < r1 {
			r1, r2 = r2, r1
		}
		return fmt.Sprintf("%d", r1), r2 - r1 + 1, nil
	}
	// 列同理必须是纯字母：parseA1Cell 会给列名补 "1" 再解析，"A5" 会变成 "A51"
	// 被当成 A 列静默放过。先要求 a/b 只含字母，杜绝带数字/尾随字符的输入通过预检；
	// 纯多字母列名（如 "AX"）是合法的，正常通过。
	if !isAllLetters(a) || !isAllLetters(b) {
		return "", 0, fmt.Errorf("列范围非法: %s（应形如 \"A:C\"）", addr)
	}
	// isAllLetters 已保证 a/b 为非空纯字母，补 "1" 后 parseA1Cell 必然成功，无需再判错。
	c1, _, _ := parseA1Cell(strings.ToUpper(a) + "1")
	c2, _, _ := parseA1Cell(strings.ToUpper(b) + "1")
	// 反序范围（如 "C:A"）必须同时交换起始列，否则会静默改错目标：
	// 只交换用于算长度的索引会得到 startIndex="C"/length=3，改到 C/D/E 而非 A/B/C。
	// 行分支同样在交换后返回较小的 r1。
	start = strings.ToUpper(a)
	if c2 < c1 {
		c1, c2 = c2, c1
		start = strings.ToUpper(b)
	}
	return start, c2 - c1 + 1, nil
}

// parseCreateStyles 解析 --styles，校验结构；expectedNames 非空时校验数量/顺序/name 与 --sheets 一致。
func parseCreateStyles(stylesStr string, expectedNames []string) ([]sheetStyleOps, error) {
	var payload any
	if err := json.Unmarshal([]byte(stylesStr), &payload); err != nil {
		return nil, fmt.Errorf("--styles JSON 解析失败: %w", err)
	}
	// 接受 {"styles":[...]} 或直接数组
	var arrRaw []byte
	switch v := payload.(type) {
	case map[string]any:
		inner, ok := v["styles"]
		if !ok {
			return nil, fmt.Errorf("--styles 对象形式必须含 styles 数组")
		}
		arrRaw, _ = json.Marshal(inner)
	case []any:
		arrRaw, _ = json.Marshal(v)
	default:
		return nil, fmt.Errorf("--styles 必须是 {\"styles\":[...]} 或 JSON 数组")
	}
	// 顶层键先按原始 map 查一遍：json.Unmarshal 只认 sheetStyleOps 的 tag，写成
	// cellStyles 的那份样式会被静默丢掉，同一项里的 row_sizes 却照常生效 —— 用户拿到
	// 的是一份「命令成功但样式不全」的表格，而 --styles 是建文档之后的非原子步骤。
	var rawItems []map[string]any
	if err := json.Unmarshal(arrRaw, &rawItems); err != nil {
		return nil, fmt.Errorf("--styles 解析失败: %w", err)
	}
	for i, raw := range rawItems {
		if err := rejectUnknownFields(raw, "--styles 单项", styleItemFields); err != nil {
			return nil, fmt.Errorf("--styles[%d]: %w", i, err)
		}
	}
	var items []sheetStyleOps
	if err := json.Unmarshal(arrRaw, &items); err != nil {
		return nil, fmt.Errorf("--styles 解析失败: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("--styles 不能为空数组")
	}
	for i, it := range items {
		if it.isEmpty() {
			return nil, fmt.Errorf("--styles[%d] 至少需要 cell_styles / row_sizes / col_sizes / cell_merges 之一", i)
		}
	}
	if len(expectedNames) > 0 {
		// 与 --sheets 搭配：长度/顺序/name 必须一一对应（同飞书规则）
		if len(items) != len(expectedNames) {
			return nil, fmt.Errorf("--styles 项数(%d)必须与 --sheets 子表数(%d)一致且顺序对应", len(items), len(expectedNames))
		}
		for i, it := range items {
			if it.Name != expectedNames[i] {
				return nil, fmt.Errorf("--styles[%d].name=%q 与 --sheets[%d].name=%q 不一致（需顺序对应）", i, it.Name, i, expectedNames[i])
			}
		}
	} else if len(items) != 1 {
		// 与 --values 搭配：只接受一项（其 name 忽略）
		return nil, fmt.Errorf("--values 搭配 --styles 时只能有 1 项（当前 %d 项）", len(items))
	}
	return items, nil
}

// styleCall 是一次待执行的 MCP 调用（先规划、后执行，便于前置校验）。
type styleCall struct {
	tool string
	args map[string]any
}

// planStyleOps 把 styles 单项翻译为待执行的 MCP 调用序列；纯函数、不发请求，
// 所有结构/枚举校验都在此完成，便于在创建文档**之前**快速失败。
// 顺序：cell_styles → row_sizes → col_sizes → cell_merges（合并最后，避免干扰）。
func planStyleOps(nodeID, sheetID string, ops sheetStyleOps) ([]styleCall, error) {
	var calls []styleCall

	for i, item := range ops.CellStyles {
		spec, rangeAddr, err := cellStyleItemToSpec(item)
		if err != nil {
			return nil, fmt.Errorf("cell_styles[%d]: %w", i, err)
		}
		rows, cols, err := parseA1Range(rangeAddr)
		if err != nil {
			return nil, fmt.Errorf("cell_styles[%d].range: %w", i, err)
		}
		cells, err := buildStyleCells(spec, rows, cols)
		if err != nil {
			return nil, fmt.Errorf("cell_styles[%d]: %w", i, err)
		}
		calls = append(calls, styleCall{"set_cell_range", map[string]any{
			"nodeId": nodeID, "sheetId": sheetID, "rangeAddress": rangeAddr, "cells": cells,
		}})
	}

	planSizes := func(list []map[string]any, isRow bool, label string) error {
		for i, item := range list {
			if err := rejectUnknownFields(item, label+" 项", sizeItemFields); err != nil {
				return fmt.Errorf("%s[%d]: %w", label, i, err)
			}
			rangeAddr := pickStr(item, "range")
			if rangeAddr == "" {
				return fmt.Errorf("%s[%d] 缺少必填的 range", label, i)
			}
			// type 对齐飞书：pixel（需 size）/ standard（恢复默认尺寸）/ auto（按内容自适应，仅行支持）
			typ := strings.ToLower(pickStr(item, "type"))
			if typ == "" {
				typ = "pixel"
			}
			dim := "COLUMNS"
			if isRow {
				dim = "ROWS"
			}
			args := map[string]any{
				"nodeId": nodeID, "sheetId": sheetID, "dimension": dim, "sizeType": typ,
			}
			// type 枚举按维度区分（与飞书一致）：row_sizes 有 auto，col_sizes 只有 pixel / standard
			enumHint := "pixel / standard"
			if isRow {
				enumHint = "pixel / standard / auto"
			}
			switch {
			case typ == "pixel":
				size, ok, err := pickNum(item, "size")
				if err != nil {
					return fmt.Errorf("%s[%d].%w", label, i, err)
				}
				if !ok || size <= 0 {
					return fmt.Errorf("%s[%d] type=pixel 时必须提供正整数 size", label, i)
				}
				args["pixelSize"] = size
			case typ == "standard":
				// 尺寸由服务端读取默认行高/列宽决定，无需 size。
				// 给了 size 说明调用方预期不符，静默忽略会让人以为生效了。
				_, ok, err := pickNum(item, "size")
				if err != nil {
					return fmt.Errorf("%s[%d].%w", label, i, err)
				}
				if ok {
					return fmt.Errorf("%s[%d] type=standard 表示恢复默认尺寸，不能同时给 size；要指定固定像素请用 type=pixel", label, i)
				}
			case typ == "auto" && isRow:
				// 行高按内容自适应，无需 size
				_, ok, err := pickNum(item, "size")
				if err != nil {
					return fmt.Errorf("%s[%d].%w", label, i, err)
				}
				if ok {
					return fmt.Errorf("%s[%d] type=auto 表示按内容自适应，不能同时给 size；要指定固定像素请用 type=pixel", label, i)
				}
			default:
				return fmt.Errorf("%s[%d].type=%q 非法（合法值: %s）", label, i, typ, enumHint)
			}
			start, length, err := parseRowColRange(rangeAddr, isRow)
			if err != nil {
				return fmt.Errorf("%s[%d].range: %w", label, i, err)
			}
			args["startIndex"] = start
			args["length"] = length
			calls = append(calls, styleCall{"update_dimension", args})
		}
		return nil
	}
	if err := planSizes(ops.RowSizes, true, "row_sizes"); err != nil {
		return nil, err
	}
	if err := planSizes(ops.ColSizes, false, "col_sizes"); err != nil {
		return nil, err
	}

	for i, item := range ops.CellMerges {
		if err := rejectUnknownFields(item, "cell_merges 项", mergeItemFields); err != nil {
			return nil, fmt.Errorf("cell_merges[%d]: %w", i, err)
		}
		rangeAddr := pickStr(item, "range")
		if rangeAddr == "" {
			return nil, fmt.Errorf("cell_merges[%d] 缺少必填的 range", i)
		}
		// 严格解析合并区域地址，与 cell_styles 分支一致：planStyleOps 会在创建文档
		// **之前**被干跑一次做前置校验，非法 range（如 "not-a-range"）必须在此拦下，
		// 否则会走到建文档→写数据→应用前序样式，直到最后 merge_cells 才被服务端
		// 拒绝，留下无法回滚的部分完成文档。merge_cells 的 range 契约是 A1:B3 形态，
		// parseA1Range 正好覆盖（并自动剥离 Sheet1! 前缀）。
		if _, _, err := parseA1Range(rangeAddr); err != nil {
			return nil, fmt.Errorf("cell_merges[%d].range: %w", i, err)
		}
		mergeType, err := feishuMergeType(pickStr(item, "merge_type", "mergeType"))
		if err != nil {
			return nil, fmt.Errorf("cell_merges[%d]: %w", i, err)
		}
		calls = append(calls, styleCall{"merge_cells", map[string]any{
			"nodeId": nodeID, "sheetId": sheetID, "rangeAddress": rangeAddr,
			"mergeType": mergeType,
		}})
	}
	return calls, nil
}

// applyStyleOps 规划并顺序执行 styles 单项对应的 MCP 调用。
func applyStyleOps(ctx context.Context, nodeID, sheetID string, ops sheetStyleOps) error {
	calls, err := planStyleOps(nodeID, sheetID, ops)
	if err != nil {
		return err
	}
	for _, c := range calls {
		if err := callMCPToolSilent(ctx, c.tool, c.args); err != nil {
			return fmt.Errorf("%s 应用失败: %w", c.tool, err)
		}
	}
	return nil
}

// callMCPToolSilent 调用 MCP 工具但不打印结果（用于编排中的中间步骤）。
func callMCPToolSilent(ctx context.Context, tool string, args map[string]any) error {
	_, err := callMCPToolReturnText(ctx, tool, args)
	return err
}

// waitSheetWritable 等待新建文档进入可写状态。
// 新建表格后服务端仍在初始化，此时写入可能返回成功但数据不落盘，
// 因此先用 get_all_sheets 探活（带退避重试），确认文档已就绪再写数据。
func waitSheetWritable(ctx context.Context, nodeID string) (string, error) {
	delays := []time.Duration{0, 500 * time.Millisecond, 1 * time.Second, 2 * time.Second, 3 * time.Second}
	var lastErr error
	for _, d := range delays {
		if err := waitSheetRetryDelay(ctx, d); err != nil {
			return "", err
		}
		if sheetID, err := resolveFirstSheetID(ctx, nodeID); err == nil {
			// 探活已经取得稳定 sheetId；直接交给写入阶段复用，避免再次
			// get_all_sheets，并保证探活与首次写入指向同一张默认工作表。
			return sheetID, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}

func waitSheetRetryDelay(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-helperAfter(delay):
		return nil
	}
}

// firstNonEmptyValuesCell 返回 values 里第一个非空单元格的 A1 地址（按行优先）。
// 调用方已保证矩阵含至少一个非空格，所以不会退化到 "A1" 兜底之外的情况。
func firstNonEmptyValuesCell(values [][]any) string {
	for i, row := range values {
		for j, cell := range row {
			if cellToString(cell) != "" {
				return fmt.Sprintf("%s%d", sheetColumnLetterFromZeroBased(j), i+1)
			}
		}
	}
	return "A1"
}

// valuesHaveContent 判断二维数据里是否有任何非空单元格。
// 全空矩阵（如 [["",""]]）写不出任何内容，必须在建文档之前拒掉，
// 否则会白建一个空文档再由回读校验报成"写入未生效"。
func valuesHaveContent(values [][]any) bool {
	for _, row := range values {
		for _, cell := range row {
			if cellToString(cell) != "" {
				return true
			}
		}
	}
	return false
}

// verifyRangeNotEmpty 回读校验目标区域确实写入了数据（防"返回成功但未落盘"）。
// 返回 nil 表示已确认非空；无法确认时返回错误说明。
func verifyRangeNotEmpty(ctx context.Context, nodeID, sheetID, rangeAddr string) error {
	text, err := callMCPToolReturnText(ctx, "get_range_as_csv", map[string]any{
		"nodeId": nodeID, "sheetId": sheetID, "range": rangeAddr,
	})
	if err != nil {
		return fmt.Errorf("回读校验失败: %w", err)
	}
	data := unwrapSheetResult(text)
	if data == nil {
		return fmt.Errorf("回读校验失败：无法解析返回")
	}
	csvText, _ := data["csv"].(string)
	// 去掉 [row=N] 前缀与分隔符后若无任何实质字符，视为未落盘
	stripped := strings.NewReplacer(",", "", "\n", "", " ", "").Replace(
		regexpRowPrefix.ReplaceAllString(csvText, ""))
	if strings.TrimSpace(stripped) == "" {
		return fmt.Errorf("数据未落盘（回读为空）")
	}
	return nil
}

// regexpRowPrefix 匹配 csv-get 的 [row=N] 行号前缀。
var regexpRowPrefix = regexp.MustCompile(`\[row=\d+\]\s*`)

// writeValuesToSheet 把二维数据转 CSV 后写入指定工作表（复用 set_range_from_csv，允许覆盖）。
func writeValuesToSheet(ctx context.Context, nodeID, sheetID string, values [][]any) error {
	if len(values) == 0 {
		return nil
	}
	return callMCPToolSilent(ctx, "set_range_from_csv", map[string]any{
		"nodeId":         nodeID,
		"sheetId":        sheetID,
		"csv":            valuesToCSV(values),
		"startCell":      "A1",
		"allowOverwrite": true,
	})
}

// valuesToCSV 把二维数据编码为 RFC4180 CSV 文本（每个单元格转字符串，nil 视为空）。
func valuesToCSV(values [][]any) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, row := range values {
		rec := make([]string, len(row))
		for i, cell := range row {
			rec[i] = cellToString(cell)
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return buf.String()
}

func cellToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		// 原样输出字面量，避免任何浮点中转造成的精度损失
		return t.String()
	case float64:
		// 整数不带小数点
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// parseCreatedNodeID 从 create_workspace_sheet 响应提取 nodeId。
func parseCreatedNodeID(text string) (string, error) {
	data := unwrapSheetResult(text)
	if data == nil {
		return "", fmt.Errorf("解析创建结果失败，响应: %s", text)
	}
	if nodeID, _ := data["nodeId"].(string); nodeID != "" {
		return nodeID, nil
	}
	return "", fmt.Errorf("创建结果未返回 nodeId，响应: %s", text)
}

var (
	projectCreatedSheetResult = addCreatedSheetID
	marshalCreatedSheetJSON   = json.Marshal
)

// addCreatedSheetID 保留 create_workspace_sheet 的完整响应信封，同时发布编排
// 过程中已经探活确认的 nodeId / sheetId。对象型 result 同时保留旧的
// .result.nodeId 路径并投影顶层字段；外层 requestId 也不丢失。使用 RawMessage
// 避免大整数经 float64 中转；本函数只做结果投影，不触发远程调用。
func addCreatedSheetID(text, nodeID, sheetID string) (string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return "", fmt.Errorf("解析创建响应失败: %w", err)
	}
	if envelope == nil {
		return "", fmt.Errorf("创建响应不是 JSON 对象")
	}
	nodeJSON := json.RawMessage(strconv.Quote(nodeID))
	sheetJSON := json.RawMessage(strconv.Quote(sheetID))
	if wrapped, ok := envelope["result"]; ok {
		var business map[string]json.RawMessage
		// 与 unwrapSheetResult 保持同一层选择规则：只有 result 确实是
		// JSON object 时才把它视为 transport wrapper。为兼容已有脚本，包装
		// 本身继续保留；同时把业务字段补到顶层，供新消费者直接读取。
		if err := json.Unmarshal(wrapped, &business); err == nil && business != nil {
			business["nodeId"] = nodeJSON
			business["sheetId"] = sheetJSON
			encodedBusiness, err := marshalCreatedSheetJSON(business)
			if err != nil {
				return "", fmt.Errorf("编码创建响应 result 失败: %w", err)
			}
			envelope["result"] = encodedBusiness
			for key, value := range business {
				if _, exists := envelope[key]; !exists {
					envelope[key] = value
				}
			}
		}
	}
	// 顶层稳定 ID 以探活事实为准；非对象 result 仍按原样保留。
	envelope["nodeId"] = nodeJSON
	envelope["sheetId"] = sheetJSON
	encoded, err := marshalCreatedSheetJSON(envelope)
	// envelope 只含刚通过 Unmarshal 校验的 RawMessage 和上面的合法字符串，Marshal
	// 理论上不会失败；仍原样传播错误，避免静默输出空结果。
	return string(encoded), err
}

// resolveFirstSheetID 通过 get_all_sheets 获取第一个工作表的 sheetId。
func resolveFirstSheetID(ctx context.Context, nodeID string) (string, error) {
	text, err := callMCPToolReturnText(ctx, "get_all_sheets", map[string]any{"nodeId": nodeID})
	if err != nil {
		return "", err
	}
	data := unwrapSheetResult(text)
	if data == nil {
		return "", fmt.Errorf("解析工作表列表失败，响应: %s", text)
	}
	sheets, _ := data["sheets"].([]any)
	if len(sheets) == 0 {
		return "", fmt.Errorf("表格中未找到任何工作表")
	}
	first, _ := sheets[0].(map[string]any)
	if id, _ := first["sheetId"].(string); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("工作表列表未返回 sheetId，响应: %s", text)
}

// resolveSheetIDsByName 返回文档内 name→sheetId 映射，用于 table_put 之后按
// 工作表名逐一回读校验（table_put 会按 name 复用/新建工作表）。
func resolveSheetIDsByName(ctx context.Context, nodeID string) (map[string]string, error) {
	text, err := callMCPToolReturnText(ctx, "get_all_sheets", map[string]any{"nodeId": nodeID})
	if err != nil {
		return nil, err
	}
	data := unwrapSheetResult(text)
	if data == nil {
		return nil, fmt.Errorf("解析工作表列表失败，响应: %s", text)
	}
	sheets, _ := data["sheets"].([]any)
	m := make(map[string]string, len(sheets))
	for _, s := range sheets {
		sm, _ := s.(map[string]any)
		name, _ := sm["name"].(string)
		id, _ := sm["sheetId"].(string)
		if name != "" && id != "" {
			m[name] = id
		}
	}
	return m, nil
}

// sheetSpecGrid 把 table_put 的一个 sheet spec 还原成写入的二维逻辑网格：
// 表头行（写表头时）在前，其后接 data 行。用于定位首个预期非空单元格。
// spec 已由 validateCreateSheetSpec 校验，columns 必为字符串数组、data 行必为数组。
func sheetSpecGrid(spec map[string]any) [][]any {
	var grid [][]any
	if cols, ok := spec["columns"].([]any); ok && len(cols) > 0 && specWritesHeader(spec) {
		grid = append(grid, cols)
	}
	if data, ok := spec["data"].([]any); ok {
		for _, r := range data {
			if row, ok := r.([]any); ok {
				grid = append(grid, row)
			}
		}
	}
	return grid
}

// firstNonEmptySheetSpecCell 返回该 sheet spec 首个预期非空单元格的 A1 地址。
// 尊重起始格偏移（默认 A1）。第二返回值为 false 表示该 spec 没有任何要写入
// 的内容（例如 header=false 且没给 data），这类工作表本就应为空，调用方跳过回读，
// 避免把合法的空表误判为数据丢失。逻辑与 --values 分支的 firstNonEmptyValuesCell
// 一致：不能死盯 A1，首行/首列为空的合法数据不应被误报。
//
// 起始格键名只认 table_put 线上契约的 camelCase startCell —— snake_case 的
// start_cell 会被服务端 DTO 静默丢弃（写入实际落在 A1），探针若认它就会指到空白
// 处、把成功的写入报成「数据未落盘」；validateCreateSheetSpec 已在前置校验里拒掉。
func firstNonEmptySheetSpecCell(spec map[string]any) (string, bool) {
	col0, row0 := 1, 1
	if sc := normalizeSpecStartCell(pickStr(spec, "startCell")); sc != "" {
		if c, r, err := parseA1Cell(sc); err == nil {
			col0, row0 = c, r
		}
	}
	// mode=append 时服务端按「已有数据的下一行」定位：本命令新建的工作表
	// 都是空表，追加就从第 1 行开始，startCell 的行号被忽略（列号仍生效）。
	if mode, _ := spec["mode"].(string); mode == "append" {
		row0 = 1
	}
	for i, row := range sheetSpecGrid(spec) {
		for j, cell := range row {
			if cellToString(cell) != "" {
				return fmt.Sprintf("%s%d", sheetColumnLetterFromZeroBased(col0-1+j), row0+i), true
			}
		}
	}
	return "", false
}

// unwrapSheetResult 解析 MCP 响应 JSON，自动剥离外层 result 包装。
func unwrapSheetResult(text string) map[string]any {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil
	}
	if result, ok := data["result"].(map[string]any); ok {
		return result
	}
	return data
}

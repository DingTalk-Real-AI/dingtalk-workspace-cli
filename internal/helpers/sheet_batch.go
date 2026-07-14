package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ── batch-update: CLI 命令名 → MCP toolName 翻译层 ──────────────────────────────
//
// 设计参考 lark/cli 的 batch_op_dispatch.go：
// 每个原子命令提供 BuildXxxArgs 函数（CLI flag → MCP param），
// translateBatchOp 通过 dispatch 表引用这些函数，不重复维护映射关系。

// batchOpMapping maps a CLI command name to its MCP tool name and builder function.
type batchOpMapping struct {
	mcpTool string
	build   func(input map[string]any) map[string]any
}

// batchOpDispatch is the dispatch table for batch-update sub-operations.
// Each entry references the BuildXxxArgs function from the command's own file.
var batchOpDispatch = map[string]batchOpMapping{
	"range clear":        {"clear_range", BuildClearRangeArgs},
	"range update":       {"set_cell_range", BuildSetCellRangeArgs},
	"merge-cells":        {"merge_range", BuildMergeCellsArgs},
	"unmerge-cells":      {"unmerge_range", BuildUnmergeCellsArgs},
	"range fill":         {"fill_range", BuildFillRangeArgs},
	"range copy-to":      {"copy_range", BuildCopyRangeArgs},
	"add-dimension":      {"add_dimension", BuildAddDimensionArgs},
	"delete-dimension":   {"delete_dimension", BuildDeleteDimensionArgs},
	"move-dimension":     {"move_dimension", BuildMoveDimensionArgs},
	"set-dropdown":       {"insert_dropdown_lists", BuildSetDropdownArgs},
	"delete-dropdown":    {"delete_dropdown_lists", BuildDeleteDropdownArgs},
	"csv-put":            {"set_range_from_csv", BuildCsvPutArgs},
	"delete-float-image": {"delete_float_image", BuildDeleteFloatImageArgs},
	"update-dimension":   {"update_dimension", BuildUpdateDimensionArgs},
	"group-dimension":    {"group_dimension", BuildGroupDimensionArgs},
	"ungroup-dimension":  {"ungroup_dimension", BuildUngroupDimensionArgs},
}

// translateBatchOp translates a batch operation from CLI format to MCP format.
// toolName must be a CLI command name (e.g. "range clear", "range update").
// input keys must be CLI flag names without -- prefix (e.g. "sheet-id", "range").
// Returns error if toolName is not a recognized CLI command name.
func translateBatchOp(op map[string]any) (map[string]any, error) {
	toolName, _ := op["toolName"].(string)
	input, _ := op["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}

	// Look up CLI command name in dispatch table
	mapping, ok := batchOpDispatch[toolName]
	if !ok {
		return nil, fmt.Errorf("unsupported toolName %q: must be a CLI command name (e.g. \"range clear\", \"range update\", \"merge-cells\"). Run 'dws sheet batch-update --help' for the full list", toolName)
	}

	return map[string]any{
		"toolName": mapping.mcpTool,
		"input":    mapping.build(input),
	}, nil
}

// ── BuildXxxArgs: CLI flag → MCP param 转换函数 ──────────────────────────────────
// 每个函数接收 CLI flag 名（kebab-case）的 map，输出 MCP 参数名（camelCase）的 map。
// 目前集中放在此文件；后续拆分命令文件时可移到各命令所在文件。

// batchStr extracts a string from input map, checking multiple key aliases.
func batchStr(input map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := input[k]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func batchStrOr(input map[string]any, key, defaultVal string) string {
	if v, ok := input[key]; ok && v != nil {
		s := fmt.Sprintf("%v", v)
		if s != "" {
			return s
		}
	}
	return defaultVal
}

func batchInt(input map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := input[k]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			default:
				var i int
				fmt.Sscanf(fmt.Sprintf("%v", v), "%d", &i)
				return i
			}
		}
	}
	return 0
}

// BuildClearRangeArgs converts CLI flags to MCP params for clear_range.
func BuildClearRangeArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
	}
	t := batchStrOr(input, "type", "content")
	args["type"] = t
	return args
}

// BuildSetCellRangeArgs converts CLI flags to MCP params for set_cell_range.
func BuildSetCellRangeArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId":      batchStr(input, "sheet-id"),
		"rangeAddress": batchStr(input, "range"),
		"cells":        input["values"],
	}
}

// BuildMergeCellsArgs converts CLI flags to MCP params for merge_range.
func BuildMergeCellsArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
	}
	if mt := batchStr(input, "merge-type"); mt != "" {
		args["mergeType"] = mt
	} else {
		args["mergeType"] = "mergeAll"
	}
	return args
}

// BuildUnmergeCellsArgs converts CLI flags to MCP params for unmerge_range.
func BuildUnmergeCellsArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
	}
}

// BuildFillRangeArgs converts CLI flags to MCP params for fill_range.
func BuildFillRangeArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId":          batchStr(input, "sheet-id"),
		"sourceRange":      batchStr(input, "source-range"),
		"destinationRange": batchStr(input, "target-range"),
	}
	if ft := batchStr(input, "fill-type"); ft != "" {
		args["fillType"] = ft
	}
	return args
}

// BuildCopyRangeArgs converts CLI flags to MCP params for copy_range.
func BuildCopyRangeArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId":          batchStr(input, "sheet-id"),
		"sourceRange":      batchStr(input, "source-range"),
		"destinationRange": batchStr(input, "target-range"),
	}
	if v := batchStr(input, "target-sheet-id"); v != "" {
		args["targetSheetId"] = v
	}
	if v := batchStr(input, "paste-type"); v != "" {
		args["pasteType"] = v
	}
	return args
}

// BuildAddDimensionArgs converts CLI flags to MCP params for add_dimension.
func BuildAddDimensionArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId":   batchStr(input, "sheet-id"),
		"dimension": batchStr(input, "dimension"),
		"length":    batchInt(input, "length"),
	}
}

// BuildDeleteDimensionArgs converts CLI flags to MCP params for delete_dimension.
func BuildDeleteDimensionArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId":    batchStr(input, "sheet-id"),
		"dimension":  batchStr(input, "dimension"),
		"startIndex": batchInt(input, "position", "startIndex"),
		"count":      batchInt(input, "length", "count"),
	}
}

// BuildMoveDimensionArgs converts CLI flags to MCP params for move_dimension.
func BuildMoveDimensionArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId":          batchStr(input, "sheet-id"),
		"dimension":        batchStr(input, "dimension"),
		"startIndex":       batchInt(input, "start-index", "startIndex"),
		"endIndex":         batchInt(input, "end-index", "endIndex"),
		"destinationIndex": batchInt(input, "destination-index", "destinationIndex"),
	}
}

// BuildSetDropdownArgs converts CLI flags to MCP params for insert_dropdown_lists.
func BuildSetDropdownArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
		"options": input["options"],
	}
	if v, ok := input["multi-select"]; ok {
		args["enableMultiSelect"] = v
	}
	return args
}

// BuildDeleteDropdownArgs converts CLI flags to MCP params for delete_dropdown_lists.
func BuildDeleteDropdownArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
	}
}

// BuildCsvPutArgs converts CLI flags to MCP params for set_range_from_csv.
// Resolves @filepath and - stdin to CSV text.
func BuildCsvPutArgs(input map[string]any) map[string]any {
	csvVal := batchStr(input, "csv")
	args := map[string]any{
		"sheetId":   batchStr(input, "sheet-id"),
		"csv":       resolveCsvContent(csvVal),
		"startCell": batchStr(input, "start-cell"),
	}
	if v, ok := input["allow-overwrite"]; ok {
		args["allowOverwrite"] = v
	}
	return args
}

// BuildDeleteFloatImageArgs converts CLI flags to MCP params for delete_float_image.
func BuildDeleteFloatImageArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId":      batchStr(input, "sheet-id"),
		"floatImageId": batchStr(input, "float-image-id"),
	}
}

// BuildUpdateDimensionArgs converts CLI flags to MCP params for update_dimension.
// startIndex 传递 A1 表示法字符串（与独立工具一致），由服务端转换为 0-based 整数。
func BuildUpdateDimensionArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId":    batchStr(input, "sheet-id"),
		"dimension":  strings.ToUpper(batchStr(input, "dimension")),
		"startIndex": batchStr(input, "start-index"),
		"length":     batchInt(input, "length"),
	}
	if v := batchInt(input, "pixel-size", "pixelSize"); v != 0 {
		args["pixelSize"] = v
	}
	if v, ok := input["hidden"]; ok {
		args["hidden"] = v
	}
	return args
}

// BuildGroupDimensionArgs converts CLI flags to MCP params for group_dimension.
func BuildGroupDimensionArgs(input map[string]any) map[string]any {
	groupState := batchStr(input, "group-state", "groupState")
	if groupState == "" {
		groupState = "expand"
	}
	return map[string]any{
		"sheetId":    batchStr(input, "sheet-id"),
		"range":      batchStr(input, "range"),
		"groupState": groupState,
	}
}

// BuildUngroupDimensionArgs converts CLI flags to MCP params for ungroup_dimension.
func BuildUngroupDimensionArgs(input map[string]any) map[string]any {
	return map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
	}
}

// resolveCsvContent resolves @filepath and - stdin to CSV text, matching standalone csv-put behavior.
func resolveCsvContent(csvVal string) string {
	switch {
	case csvVal == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return csvVal
		}
		csvVal = string(data)
	case strings.HasPrefix(csvVal, "@"):
		data, err := os.ReadFile(strings.TrimPrefix(csvVal, "@"))
		if err != nil {
			return csvVal
		}
		csvVal = string(data)
	}
	csvVal = strings.ReplaceAll(csvVal, "\r", "")
	csvVal = strings.TrimPrefix(csvVal, "\xef\xbb\xbf")
	return csvVal
}

// ── batch-update / batch-clear 命令定义 ──────────────────────────────────────────

func requireSheetMutationConfirmation(cmd *cobra.Command, operation, targetHint string) error {
	// Let dry-run reach callMCPTool so it emits the exact translated preview.
	// The ToolCaller is the authoritative execution boundary and mirrors the
	// root --dry-run flag; do not bypass confirmation on a flag-only mismatch.
	if deps != nil && deps.Caller != nil && deps.Caller.DryRun() {
		return nil
	}
	if commandBoolFlag(cmd, "yes") {
		return nil
	}
	return apperrors.NewValidation(
		fmt.Sprintf("%s可能删除工作表内容或结构；获得用户确认后加 --yes 执行", operation),
		apperrors.WithReason("confirmation_required"),
		apperrors.WithHint(fmt.Sprintf("先确认%s；用户明确同意后以相同参数追加 --yes", targetHint)),
		apperrors.WithActions(fmt.Sprintf("确认%s", targetHint), "获得用户确认后使用 --yes 执行"),
	)
}

const sheetMutationConfirmationGuardAnnotation = "dws.sheet.confirmation-guard"

// protectSheetMutationCommand installs the command-local execution guard used
// by Sheet leaves whose final Schema contract declares
// confirmation=user_required. Keeping the annotation and the wrapper in the
// same function makes the Schema-to-runtime coverage gate structural: a
// command cannot advertise the marker without also running the guard.
func protectSheetMutationCommand(cmd *cobra.Command, operation, targetHint string) {
	if cmd == nil {
		panic("protect sheet mutation command: nil command")
	}
	if cmd.Annotations != nil && cmd.Annotations[sheetMutationConfirmationGuardAnnotation] == "true" {
		panic(fmt.Sprintf("protect sheet mutation command: duplicate guard on %q", cmd.CommandPath()))
	}
	originalRunE := cmd.RunE
	if originalRunE == nil {
		panic(fmt.Sprintf("protect sheet mutation command: %q has no RunE", cmd.CommandPath()))
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[sheetMutationConfirmationGuardAnnotation] = "true"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := requireSheetMutationConfirmation(cmd, operation, targetHint); err != nil {
			return err
		}
		return originalRunE(cmd, args)
	}
}

// HasSheetMutationConfirmationGuard reports whether a Sheet command is
// protected by protectSheetMutationCommand. It is exported for the app-level
// final Schema-to-runtime delivery gate; callers must not set the annotation
// directly.
func HasSheetMutationConfirmationGuard(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations != nil && cmd.Annotations[sheetMutationConfirmationGuardAnnotation] == "true"
}

func newBatchUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "batch-update",
		Short: "批量执行多个写操作（原子事务）",
		Long: `将多个写操作打包为一次原子请求执行。

默认严格事务模式：任一子操作失败则整批回滚。
传 --continue-on-error 切换为宽松模式（遇失败继续执行）。
由于子操作可包含清除、删除等破坏性操作，真实执行必须获得用户确认后加 --yes。
使用 --dry-run 可预览翻译后的完整请求，不需要 --yes，也不会调用远程接口。

toolName 使用 CLI 命令名（与原子命令一致），input 的键用 CLI flag 名去掉 --。
CLI 层自动翻译为 MCP toolName + 参数名，无需记忆 MCP 参数名。

支持的 CLI 命令名:
  range clear / range update / merge-cells / unmerge-cells / update-dimension
  range fill / range copy-to / add-dimension / delete-dimension / move-dimension
  group-dimension / ungroup-dimension
  set-dropdown / delete-dropdown / csv-put / delete-float-image

注意：batch-update 中 group-dimension 适合默认展开分组；需要 --group-state fold 时请使用独立
dws sheet group-dimension 命令。

--operations 是 JSON 数组，每项包含:
  toolName  CLI 命令名（如 "range clear", "range update"）
  input     该命令的入参（不含 --node），键用 flag 名去掉 --

完整映射表见:
  dingtalk-workspace/references/products/sheet/sheet-batch-operations.md`,
		Example: `  # 批量清除 + 写入 + 合并（获得用户确认后加 --yes）
  dws sheet batch-update --node NODE_ID --operations '[
    {"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3","type":"content"}},
    {"toolName":"range update","input":{"sheet-id":"Sheet1","range":"A1","values":[[{"type":"text","text":"hello"}]]}},
    {"toolName":"merge-cells","input":{"sheet-id":"Sheet1","range":"A1:B1","merge-type":"mergeAll"}}
  ]' --yes

  # 宽松模式
  dws sheet batch-update --node NODE_ID --continue-on-error --operations '[...]' --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "operations"); err != nil {
				return err
			}
			opsStr := mustGetFlag(cmd, "operations")
			var operations []any
			if err := json.Unmarshal([]byte(opsStr), &operations); err != nil {
				return fmt.Errorf("--operations JSON 解析失败: %w\n  hint: --operations 必须是 JSON 数组", err)
			}
			if len(operations) == 0 {
				return fmt.Errorf("--operations 不能为空数组")
			}
			translated := make([]any, 0, len(operations))
			for i, op := range operations {
				opMap, ok := op.(map[string]any)
				if !ok {
					return fmt.Errorf("operations[%d] 不是 object: %v", i, op)
				}
				top, err := translateBatchOp(opMap)
				if err != nil {
					return fmt.Errorf("operations[%d] 翻译失败: %w", i, err)
				}
				translated = append(translated, top)
			}
			toolArgs := map[string]any{
				"nodeId":     mustGetFlag(cmd, "node"),
				"operations": translated,
			}
			continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
			if continueOnError {
				toolArgs["continueOnError"] = true
			}
			err := callMCPTool("batch_update", toolArgs)
			if err == nil || continueOnError {
				return err
			}
			// 严格事务模式失败：直接透传服务端错误信息
			// flex-table-app 已在错误信息中包含失败操作索引、回滚提示和原因
			return err
		},
	}
}

func newRangeBatchClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "batch-clear",
		Short: "批量清除多个区域（原子事务）",
		Long: `批量清除多个区域，一次原子请求。任一区域清除失败则整批回滚。

每个 --ranges 项必须包含工作表前缀（格式: "SheetName!A1:B3"）。
不同区域可以属于不同工作表。
真实执行必须获得用户确认后加 --yes；--dry-run 仅预览且不调用远程接口。`,
		Example: `  dws sheet range batch-clear --node NODE_ID --ranges '["Sheet1!A1:B3","Sheet2!C1:D5"]' --yes
  dws sheet range batch-clear --node NODE_ID --ranges '["Sheet1!A1:Z1000"]' --type all --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node", "ranges"); err != nil {
				return err
			}
			rangesStr := mustGetFlag(cmd, "ranges")
			var ranges []string
			if err := json.Unmarshal([]byte(rangesStr), &ranges); err != nil {
				return fmt.Errorf("--ranges JSON 解析失败: %w\n  hint: --ranges 必须是 JSON 字符串数组，如 '[\"Sheet1!A1:B3\"]'", err)
			}
			if len(ranges) == 0 {
				return fmt.Errorf("--ranges 不能为空数组")
			}
			clearType, _ := cmd.Flags().GetString("type")
			if clearType == "" {
				clearType = "content"
			}
			operations := make([]any, 0, len(ranges))
			for i, rng := range ranges {
				idx := strings.Index(rng, "!")
				if idx <= 0 || idx == len(rng)-1 {
					return fmt.Errorf("--ranges[%d] (%q) 必须包含工作表前缀，格式为 \"SheetName!A1:B3\"", i, rng)
				}
				sheetName := strings.TrimSpace(rng[:idx])
				rangeAddr := strings.TrimSpace(rng[idx+1:])
				operations = append(operations, map[string]any{
					"toolName": "clear_range",
					"input": map[string]any{
						"sheetId": sheetName,
						"range":   rangeAddr,
						"type":    clearType,
					},
				})
			}
			return callMCPTool("batch_update", map[string]any{
				"nodeId":     mustGetFlag(cmd, "node"),
				"operations": operations,
			})
		},
	}
}

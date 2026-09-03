package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
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
	build   batchArgsBuilder
}

type batchArgsBuilder func(input map[string]any) (map[string]any, error)

func legacyBatchBuilder(build func(map[string]any) map[string]any) batchArgsBuilder {
	return func(input map[string]any) (map[string]any, error) { return build(input), nil }
}

// batchOpDispatch is the dispatch table for batch-update sub-operations.
// Each entry references the BuildXxxArgs function from the command's own file.
var batchOpDispatch = map[string]batchOpMapping{
	"range clear":        {"clear_range", legacyBatchBuilder(BuildClearRangeArgs)},
	"range update":       {"set_cell_range", legacyBatchBuilder(BuildSetCellRangeArgs)},
	"merge-cells":        {"merge_range", legacyBatchBuilder(BuildMergeCellsArgs)},
	"unmerge-cells":      {"unmerge_range", legacyBatchBuilder(BuildUnmergeCellsArgs)},
	"range fill":         {"fill_range", legacyBatchBuilder(BuildFillRangeArgs)},
	"range copy-to":      {"copy_range", legacyBatchBuilder(BuildCopyRangeArgs)},
	"add-dimension":      {"add_dimension", legacyBatchBuilder(BuildAddDimensionArgs)},
	"delete-dimension":   {"delete_dimension", BuildBatchDeleteDimensionArgs},
	"move-dimension":     {"move_dimension", BuildBatchMoveDimensionArgs},
	"set-dropdown":       {"insert_dropdown_lists", legacyBatchBuilder(BuildSetDropdownArgs)},
	"delete-dropdown":    {"delete_dropdown_lists", legacyBatchBuilder(BuildDeleteDropdownArgs)},
	"csv-put":            {"set_range_from_csv", legacyBatchBuilder(BuildCsvPutArgs)},
	"delete-float-image": {"delete_float_image", legacyBatchBuilder(BuildDeleteFloatImageArgs)},
	"update-dimension":   {"update_dimension", legacyBatchBuilder(BuildUpdateDimensionArgs)},
	"group-dimension":    {"group_dimension", legacyBatchBuilder(BuildGroupDimensionArgs)},
	"ungroup-dimension":  {"ungroup_dimension", legacyBatchBuilder(BuildUngroupDimensionArgs)},

	"range set-style":    {"set_cell_range", BuildBatchSetStyleArgs},
	"replace":            {"replace_all", BuildBatchReplaceArgs},
	"insert-dimension":   {"insert_dimension", BuildBatchInsertDimensionArgs},
	"range move-to":      {"move_range", BuildBatchMoveRangeArgs},
	"range sort":         {"sort_range", BuildBatchSortRangeArgs},
	"new":                {"create_sheet", BuildBatchCreateSheetArgs},
	"delete-sheet":       {"delete_sheet", BuildBatchDeleteSheetArgs},
	"update":             {"update_sheet", BuildBatchUpdateSheetArgs},
	"show-gridline":      {"set_gridline_visibility", buildBatchGridlineArgs("visible")},
	"hide-gridline":      {"set_gridline_visibility", buildBatchGridlineArgs("hidden")},
	"chart delete":       {"delete_float_chart", BuildBatchDeleteFloatChartArgs},
	"pivot-table delete": {"delete_pivot_table", BuildBatchDeletePivotTableArgs},
	"cond-format create": {"create_cond_format", BuildBatchCreateCondFormatArgs},
	"cond-format update": {"update_cond_format", BuildBatchUpdateCondFormatArgs},
	"cond-format delete": {"delete_cond_format", BuildBatchDeleteCondFormatArgs},
	"filter create":      {"create_filter", BuildBatchCreateFilterArgs},
	"filter update":      {"update_filter", BuildBatchUpdateFilterArgs},
	"filter delete":      {"delete_filter", BuildBatchDeleteFilterArgs},
	"filter-view create": {"create_filter_view", BuildBatchCreateFilterViewArgs},
	"filter-view update": {"update_filter_view", BuildBatchUpdateFilterViewArgs},
	"filter-view delete": {"delete_filter_view", BuildBatchDeleteFilterViewArgs},
	"create-float-image": {"create_float_image", BuildBatchCreateFloatImageArgs},
	"update-float-image": {"update_float_image", BuildBatchUpdateFloatImageArgs},
}

// translateBatchOp translates a batch operation from CLI format to MCP format.
// toolName must be a CLI command name (e.g. "range clear", "range update").
// input keys must be CLI flag names without -- prefix (e.g. "sheet-id", "range").
// Returns error if toolName is not a recognized CLI command name.
func translateBatchOp(op map[string]any) (map[string]any, error) {
	if err := rejectUnknownFields(op, "batch operation", []string{"toolName", "input"}); err != nil {
		return nil, err
	}
	toolName, ok := op["toolName"].(string)
	if !ok || strings.TrimSpace(toolName) == "" {
		return nil, fmt.Errorf("toolName 必须是非空字符串")
	}
	rawInput, exists := op["input"]
	if !exists {
		return nil, fmt.Errorf("%s: input 必须是 object", toolName)
	}
	input, ok := rawInput.(map[string]any)
	if !ok || input == nil {
		return nil, fmt.Errorf("%s: input 必须是 object，实际是 %T", toolName, rawInput)
	}

	// Look up CLI command name in dispatch table
	mapping, ok := batchOpDispatch[toolName]
	if !ok {
		return nil, fmt.Errorf("unsupported toolName %q: must be a CLI command name (e.g. \"range clear\", \"range update\", \"merge-cells\"). Run 'dws sheet batch-update --help' for the full list", toolName)
	}
	if toolName == "set-dropdown" {
		if err := validateBatchSetDropdownInput(input); err != nil {
			return nil, err
		}
	}
	if toolName == "csv-put" {
		if err := validateBatchCsvPutInput(input); err != nil {
			return nil, err
		}
	}

	args, err := mapping.build(input)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", toolName, err)
	}
	return map[string]any{
		"toolName": mapping.mcpTool,
		"input":    args,
	}, nil
}

// buildBatchUpdateToolArgs keeps the user-facing --operations contract while
// sending the translated array through the MCP string transport. Encoding the
// complete array locally preserves JSON number/boolean types across the
// platform's generic Map bridge. Callers must never add the legacy operations
// field as well or retry a write through a different transport.
func buildBatchUpdateToolArgs(nodeID string, operations []any, continueOnError bool) (map[string]any, error) {
	encoded, err := json.Marshal(operations)
	if err != nil {
		return nil, fmt.Errorf("batch_update operations JSON 编码失败: %w", err)
	}
	args := map[string]any{
		"nodeId":         nodeID,
		"operationsJson": string(encoded),
	}
	if continueOnError {
		args["continueOnError"] = true
	}
	return args, nil
}

// buildBatchUpdateToolArgsForCommand is the command-boundary seam for the
// three CLI surfaces that emit batch_update. Production always uses the pure
// builder above; tests replace this seam to prove every command propagates a
// local JSON encoding failure without attempting a remote write.
var buildBatchUpdateToolArgsForCommand = buildBatchUpdateToolArgs

func validateBatchCsvPutInput(input map[string]any) error {
	if value, exists := input["auto-convert"]; exists {
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("csv-put: auto-convert 必须是布尔值 true 或 false")
		}
	}
	return nil
}

func validateBatchSetDropdownInput(input map[string]any) error {
	if _, exists := input["source-colors"]; exists {
		return fmt.Errorf("set-dropdown: 顶层 source-colors 不受支持；Inline 颜色请写入 options[].color，SourceRange 颜色写入暂不支持")
	}
	if _, exists := input["colors"]; exists {
		return fmt.Errorf("set-dropdown: 顶层 colors 不受支持；Inline 颜色请写入 options[].color，SourceRange 颜色写入暂不支持")
	}

	options, hasOptions := input["options"]
	hasOptions = hasOptions && options != nil
	sourceRange := batchStr(input, "source-range")
	sourceSheetID := batchStr(input, "source-sheet-id")
	hasSourceRange := sourceRange != ""
	if hasOptions == hasSourceRange {
		return fmt.Errorf("set-dropdown: options 与 source-range 必须且只能指定一个")
	}
	if hasSourceRange != (sourceSheetID != "") {
		return fmt.Errorf("set-dropdown: source-range 与 source-sheet-id 必须同时指定")
	}
	if hasSourceRange {
		if err := validateDropdownSourceRangeInput(sourceSheetID, sourceRange); err != nil {
			return fmt.Errorf("set-dropdown: %w", err)
		}
	}
	return nil
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

// batchDimension converts the public CLI dimension spelling to the value used
// by the MCP batch contract. Keeping this validation in the DWS translation
// layer prevents ROWS/COLUMNS from silently taking different index paths.
func batchDimension(input map[string]any) (string, error) {
	dimension, err := requiredBatchString(input, "dimension")
	if err != nil {
		return "", err
	}
	dimension = strings.ToUpper(strings.TrimSpace(dimension))
	if dimension != "ROWS" && dimension != "COLUMNS" {
		return "", fmt.Errorf("dimension 必须为 ROWS 或 COLUMNS，当前值: %s", dimension)
	}
	return dimension, nil
}

// batchDimensionA1Index converts one public CLI A1 position to the 0-based
// integer expected by the low-level batch MCP contract. ROWS accept 1-based
// row numbers; COLUMNS accept column letters such as A/AB. delete-dimension
// additionally allows the standalone-compatible "SheetName!position" form.
func batchDimensionA1Index(value any, dimension, field string, allowSheetPrefix bool) (sheetPrefix string, index int, err error) {
	text, ok := value.(string)
	if !ok {
		if dimension != "ROWS" {
			return "", 0, fmt.Errorf("%s 在 dimension=COLUMNS 时必须是列字母字符串，实际是 %T", field, value)
		}
		parsed, _, parseErr := batchStrictInt(map[string]any{field: value}, field, 1, math.MaxInt32, true)
		if parseErr != nil {
			return "", 0, parseErr
		}
		return "", parsed - 1, nil
	}

	originalText := strings.TrimSpace(text)
	text = originalText
	if text == "" {
		return "", 0, fmt.Errorf("%s 不能为空", field)
	}
	if bang := strings.Index(text, "!"); bang >= 0 {
		if !allowSheetPrefix {
			return "", 0, fmt.Errorf("%s 不支持工作表前缀，请通过 sheet-id 指定工作表", field)
		}
		if strings.Contains(text[bang+1:], "!") {
			return "", 0, fmt.Errorf("%s 工作表前缀格式非法: %s", field, text)
		}
		sheetPrefix = strings.TrimSpace(text[:bang])
		text = strings.TrimSpace(text[bang+1:])
		if sheetPrefix == "" || text == "" {
			return "", 0, fmt.Errorf("%s 工作表前缀格式非法: %s", field, originalText)
		}
	}

	if dimension == "ROWS" {
		parsed, parseErr := strconv.ParseInt(text, 10, 32)
		if parseErr != nil || parsed < 1 {
			return "", 0, fmt.Errorf("%s 在 dimension=ROWS 时必须是 1-based 行号，当前值: %s", field, text)
		}
		return sheetPrefix, int(parsed) - 1, nil
	}

	if !isAllLetters(text) {
		return "", 0, fmt.Errorf("%s 在 dimension=COLUMNS 时必须是列字母，如 A 或 AB，当前值: %s", field, text)
	}
	column := 0
	for _, character := range strings.ToUpper(text) {
		digit := int(character-'A') + 1
		if column > (math.MaxInt32-digit)/26 {
			return "", 0, fmt.Errorf("%s 列字母超出支持范围: %s", field, text)
		}
		column = column*26 + digit
	}
	return sheetPrefix, column - 1, nil
}

func hasAnyBatchField(input map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, exists := input[key]; exists {
			return true
		}
	}
	return false
}

// BuildBatchDeleteDimensionArgs converts the documented CLI-shaped input to
// the low-level 0-based delete_dimension request. The camelCase startIndex /
// count pair remains as an undocumented compatibility path for callers that
// already supplied the old low-level payload directly.
func BuildBatchDeleteDimensionArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "dimension", "position", "length", "startIndex", "count"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	dimension, err := batchDimension(input)
	if err != nil {
		return nil, err
	}
	publicInput := hasAnyBatchField(input, "position", "length")
	rawInput := hasAnyBatchField(input, "startIndex", "count")
	if publicInput && rawInput {
		return nil, fmt.Errorf("position/length 与 startIndex/count 不能混用")
	}

	var startIndex, count int
	if publicInput {
		position, exists := input["position"]
		if !exists {
			return nil, fmt.Errorf("position 必填")
		}
		resolvedSheetID, parsedIndex, parseErr := batchDimensionA1Index(position, dimension, "position", true)
		if parseErr != nil {
			return nil, parseErr
		}
		if resolvedSheetID != "" {
			sheetID = resolvedSheetID
		}
		startIndex = parsedIndex
		count, _, err = batchStrictInt(input, "length", 1, 5000, true)
	} else if rawInput {
		startIndex, _, err = batchStrictInt(input, "startIndex", 0, math.MaxInt32, true)
		if err == nil {
			count, _, err = batchStrictInt(input, "count", 1, 5000, true)
		}
	} else {
		return nil, fmt.Errorf("position/length 必填")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"sheetId": sheetID, "dimension": dimension, "startIndex": startIndex, "count": count,
	}, nil
}

// BuildBatchMoveDimensionArgs converts the documented 1-based row numbers or
// column letters to the 0-based integer indexes consumed by batch_update. The
// camelCase fields preserve the previous low-level compatibility path.
func BuildBatchMoveDimensionArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "dimension", "start-index", "end-index", "destination-index", "startIndex", "endIndex", "destinationIndex"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	dimension, err := batchDimension(input)
	if err != nil {
		return nil, err
	}
	publicInput := hasAnyBatchField(input, "start-index", "end-index", "destination-index")
	rawInput := hasAnyBatchField(input, "startIndex", "endIndex", "destinationIndex")
	if publicInput && rawInput {
		return nil, fmt.Errorf("start-index/end-index/destination-index 与 startIndex/endIndex/destinationIndex 不能混用")
	}

	var startIndex, endIndex, destinationIndex int
	if publicInput {
		for _, key := range []string{"start-index", "end-index", "destination-index"} {
			if _, exists := input[key]; !exists {
				return nil, fmt.Errorf("%s 必填", key)
			}
		}
		_, startIndex, err = batchDimensionA1Index(input["start-index"], dimension, "start-index", false)
		if err == nil {
			_, endIndex, err = batchDimensionA1Index(input["end-index"], dimension, "end-index", false)
		}
		if err == nil {
			_, destinationIndex, err = batchDimensionA1Index(input["destination-index"], dimension, "destination-index", false)
		}
	} else if rawInput {
		startIndex, _, err = batchStrictInt(input, "startIndex", 0, math.MaxInt32, true)
		if err == nil {
			endIndex, _, err = batchStrictInt(input, "endIndex", 0, math.MaxInt32, true)
		}
		if err == nil {
			destinationIndex, _, err = batchStrictInt(input, "destinationIndex", 0, math.MaxInt32, true)
		}
	} else {
		return nil, fmt.Errorf("start-index/end-index/destination-index 必填")
	}
	if err != nil {
		return nil, err
	}
	if endIndex < startIndex {
		return nil, fmt.Errorf("end-index 必须大于等于 start-index")
	}
	if endIndex-startIndex+1 > 5000 {
		return nil, fmt.Errorf("移动范围最多包含 5000 行或列")
	}
	if destinationIndex >= startIndex && destinationIndex <= endIndex {
		return nil, fmt.Errorf("destination-index 不能落在源范围内")
	}
	return map[string]any{
		"sheetId": sheetID, "dimension": dimension,
		"startIndex": startIndex, "endIndex": endIndex, "destinationIndex": destinationIndex,
	}, nil
}

// BuildSetDropdownArgs converts CLI flags to MCP params for insert_dropdown_lists.
func BuildSetDropdownArgs(input map[string]any) map[string]any {
	args := map[string]any{
		"sheetId": batchStr(input, "sheet-id"),
		"range":   batchStr(input, "range"),
	}
	if options, ok := input["options"]; ok && options != nil {
		args["options"] = options
	}
	if sourceRange := batchStr(input, "source-range"); sourceRange != "" {
		args["sourceRange"] = map[string]any{
			"sheetId":    batchStr(input, "source-sheet-id"),
			"a1Notation": sourceRange,
		}
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
	if v, ok := input["auto-convert"]; ok {
		args["autoConvert"] = v
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

// ── Strict P0/P1 builders shared by batch-update and standalone commands ─────

func validateBatchInputFields(input map[string]any, allowed ...string) error {
	if err := rejectUnknownFields(input, "batch input", allowed); err != nil {
		return err
	}
	return nil
}

func requiredBatchString(input map[string]any, key string) (string, error) {
	value, exists := input[key]
	if !exists {
		return "", fmt.Errorf("%s 必填", key)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s 必须是字符串，实际是 %T", key, value)
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s 不能为空", key)
	}
	return text, nil
}

func optionalBatchString(input map[string]any, key string) (string, bool, error) {
	value, exists := input[key]
	if !exists {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("%s 必须是字符串，实际是 %T", key, value)
	}
	return text, true, nil
}

func optionalBatchBool(input map[string]any, key string) (bool, bool, error) {
	value, exists := input[key]
	if !exists {
		return false, false, nil
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, false, fmt.Errorf("%s 必须是布尔值 true 或 false，实际是 %T", key, value)
	}
	return boolean, true, nil
}

func batchStrictInt(input map[string]any, key string, min, max int, required bool) (int, bool, error) {
	value, exists := input[key]
	if !exists {
		if required {
			return 0, false, fmt.Errorf("%s 必填", key)
		}
		return 0, false, nil
	}
	var parsed int64
	switch number := value.(type) {
	case int:
		parsed = int64(number)
	case int8:
		parsed = int64(number)
	case int16:
		parsed = int64(number)
	case int32:
		parsed = int64(number)
	case int64:
		parsed = number
	case uint:
		if uint64(number) > math.MaxInt64 {
			return 0, false, fmt.Errorf("%s 超出整数范围", key)
		}
		parsed = int64(number)
	case uint8:
		parsed = int64(number)
	case uint16:
		parsed = int64(number)
	case uint32:
		parsed = int64(number)
	case uint64:
		if number > math.MaxInt64 {
			return 0, false, fmt.Errorf("%s 超出整数范围", key)
		}
		parsed = int64(number)
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
			return 0, false, fmt.Errorf("%s 必须是整数，当前值: %v", key, value)
		}
		parsed = int64(number)
	case float32:
		if math.Trunc(float64(number)) != float64(number) {
			return 0, false, fmt.Errorf("%s 必须是整数，当前值: %v", key, value)
		}
		parsed = int64(number)
	case json.Number:
		integer, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil {
			return 0, false, fmt.Errorf("%s 必须是整数，当前值: %v", key, value)
		}
		parsed = integer
	default:
		return 0, false, fmt.Errorf("%s 必须是整数，实际是 %T", key, value)
	}
	if parsed < int64(min) || parsed > int64(max) {
		return 0, false, fmt.Errorf("%s 必须在 %d~%d 范围内，当前值: %d", key, min, max, parsed)
	}
	return int(parsed), true, nil
}

func batchJSONArray(input map[string]any, key string, required bool) ([]any, bool, error) {
	value, exists := input[key]
	if !exists {
		if required {
			return nil, false, fmt.Errorf("%s 必填", key)
		}
		return nil, false, nil
	}
	var raw []byte
	var err error
	if text, ok := value.(string); ok {
		raw = []byte(text)
	} else {
		raw, err = json.Marshal(value)
		if err != nil {
			return nil, false, fmt.Errorf("%s JSON 编码失败: %w", key, err)
		}
	}
	var array []any
	if err := json.Unmarshal(raw, &array); err != nil {
		return nil, false, fmt.Errorf("%s 必须是 JSON 数组: %w", key, err)
	}
	if required && len(array) == 0 {
		return nil, false, fmt.Errorf("%s 不能为空数组", key)
	}
	return array, true, nil
}

func batchJSONObject(input map[string]any, key string, required bool) (map[string]any, bool, error) {
	value, exists := input[key]
	if !exists {
		if required {
			return nil, false, fmt.Errorf("%s 必填", key)
		}
		return nil, false, nil
	}
	var raw []byte
	var err error
	if text, ok := value.(string); ok {
		raw = []byte(text)
	} else {
		raw, err = json.Marshal(value)
		if err != nil {
			return nil, false, fmt.Errorf("%s JSON 编码失败: %w", key, err)
		}
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, false, fmt.Errorf("%s 必须是 JSON object: %w", key, err)
	}
	if object == nil || required && len(object) == 0 {
		return nil, false, fmt.Errorf("%s 不能为空 object", key)
	}
	return object, true, nil
}

func batchJSONText(input map[string]any, key string) (string, bool, error) {
	value, exists := input[key]
	if !exists {
		return "", false, nil
	}
	if text, ok := value.(string); ok {
		return text, true, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", false, fmt.Errorf("%s JSON 编码失败: %w", key, err)
	}
	return string(raw), true, nil
}

func BuildBatchSetStyleArgs(input map[string]any) (map[string]any, error) {
	allowed := append([]string{"sheet-id", "range"}, styleFlagNames...)
	if err := validateBatchInputFields(input, allowed...); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	rangeAddr, err := requiredBatchString(input, "range")
	if err != nil {
		return nil, err
	}
	rows, cols, err := parseA1Range(rangeAddr)
	if err != nil {
		return nil, err
	}
	spec := &styleSpec{}
	for key, target := range map[string]*string{
		"bg-color": &spec.BgColor, "h-align": &spec.HAlign, "v-align": &spec.VAlign,
		"font-color": &spec.FontColor, "font-weight": &spec.FontWeight, "word-wrap": &spec.WordWrap,
		"number-format": &spec.NumberFormat, "font-style": &spec.FontStyle, "font-line": &spec.FontLine,
		"font-family": &spec.FontFamily,
	} {
		if value, exists, valueErr := optionalBatchString(input, key); valueErr != nil {
			return nil, valueErr
		} else if exists {
			*target = value
		}
	}
	if value, exists, valueErr := batchStrictInt(input, "font-size", 1, math.MaxInt32, false); valueErr != nil {
		return nil, valueErr
	} else if exists {
		spec.FontSize = value
	}
	for key, target := range map[string]*string{
		"bg-colors-json": &spec.BgColorsJSON, "font-sizes-json": &spec.FontSizesJSON,
		"h-aligns-json": &spec.HAlignsJSON, "v-aligns-json": &spec.VAlignsJSON,
		"font-colors-json": &spec.FontColorsJSON, "font-weights-json": &spec.FontWeightsJSON,
		"border-styles-json": &spec.BorderStylesJSON,
	} {
		if value, exists, valueErr := batchJSONText(input, key); valueErr != nil {
			return nil, valueErr
		} else if exists {
			*target = value
		}
	}
	cells, err := buildStyleCells(spec, rows, cols)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sheetId": sheetID, "rangeAddress": rangeAddr, "cells": cells}, nil
}

func BuildBatchReplaceArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "find", "replacement", "range", "match-case",
		"match-entire-cell", "use-regexp", "match-formula", "include-hidden"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	find, err := requiredBatchString(input, "find")
	if err != nil {
		return nil, err
	}
	replacement, exists, err := optionalBatchString(input, "replacement")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("replacement 必填（允许空字符串）")
	}
	args := map[string]any{"sheetId": sheetID, "text": find, "replaceText": replacement}
	if value, ok, valueErr := optionalBatchString(input, "range"); valueErr != nil {
		return nil, valueErr
	} else if ok && value != "" {
		args["range"] = value
	}
	for cliKey, mcpKey := range map[string]string{
		"match-case": "matchCase", "match-entire-cell": "matchEntireCell", "use-regexp": "useRegExp",
		"match-formula": "matchFormulaText", "include-hidden": "includeHidden",
	} {
		value, _, valueErr := optionalBatchBool(input, cliKey)
		if valueErr != nil {
			return nil, valueErr
		}
		args[mcpKey] = value
	}
	return args, nil
}

func BuildBatchInsertDimensionArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "dimension", "position", "length"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	dimension, err := requiredBatchString(input, "dimension")
	if err != nil {
		return nil, err
	}
	dimension = strings.ToUpper(dimension)
	if dimension != "ROWS" && dimension != "COLUMNS" {
		return nil, fmt.Errorf("dimension 必须为 ROWS 或 COLUMNS，当前值: %s", dimension)
	}
	position, err := requiredBatchString(input, "position")
	if err != nil {
		return nil, err
	}
	length, _, err := batchStrictInt(input, "length", 1, 5000, true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sheetId": sheetID, "dimension": dimension, "position": position, "length": length}, nil
}

func BuildBatchMoveRangeArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "source-range", "target-range", "target-sheet-id"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	sourceRange, err := requiredBatchString(input, "source-range")
	if err != nil {
		return nil, err
	}
	targetRange, err := requiredBatchString(input, "target-range")
	if err != nil {
		return nil, err
	}
	args := map[string]any{"sheetId": sheetID, "sourceRange": sourceRange, "destinationRange": targetRange}
	if targetSheetID, exists, valueErr := optionalBatchString(input, "target-sheet-id"); valueErr != nil {
		return nil, valueErr
	} else if exists && targetSheetID != "" {
		args["targetSheetId"] = targetSheetID
	}
	return args, nil
}

func BuildBatchSortRangeArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "range", "sort-keys", "has-header"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	rangeAddr, err := requiredBatchString(input, "range")
	if err != nil {
		return nil, err
	}
	sortKeys, _, err := batchJSONArray(input, "sort-keys", true)
	if err != nil {
		return nil, err
	}
	for i, item := range sortKeys {
		if _, ok := item.(map[string]any); !ok {
			return nil, fmt.Errorf("sort-keys[%d] 必须是 object", i)
		}
	}
	args := map[string]any{"sheetId": sheetID, "range": rangeAddr, "sortKeys": sortKeys}
	if hasHeader, exists, valueErr := optionalBatchBool(input, "has-header"); valueErr != nil {
		return nil, valueErr
	} else if exists {
		args["hasHeader"] = hasHeader
	}
	return args, nil
}

func BuildBatchCreateSheetArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "name"); err != nil {
		return nil, err
	}
	name, err := requiredBatchString(input, "name")
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": name}, nil
}

func BuildBatchDeleteSheetArgs(input map[string]any) (map[string]any, error) {
	return buildBatchSheetIDOnly(input)
}

func buildBatchSheetIDOnly(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	return map[string]any{"sheetId": sheetID}, nil
}

func BuildBatchUpdateSheetArgs(input map[string]any) (map[string]any, error) {
	return buildUpdateSheetArgs(input, true)
}

func BuildUpdateSheetArgs(input map[string]any) (map[string]any, error) {
	return buildUpdateSheetArgs(input, false)
}

func buildUpdateSheetArgs(input map[string]any, rejectMixed bool) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "name", "title", "index", "hidden",
		"frozen-row-count", "frozen-column-count", "tab-color"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	args := map[string]any{"sheetId": sheetID}
	name, nameSet, err := optionalBatchString(input, "name")
	if err != nil {
		return nil, err
	}
	title, titleSet, err := optionalBatchString(input, "title")
	if err != nil {
		return nil, err
	}
	if nameSet && titleSet {
		return nil, fmt.Errorf("name 与 title 是别名，不能同时指定")
	}
	if nameSet {
		args["title"] = name
	}
	if titleSet {
		args["title"] = title
	}
	index, indexSet, err := batchStrictInt(input, "index", 0, math.MaxInt32, false)
	if err != nil {
		return nil, err
	}
	if indexSet {
		args["index"] = index
	}
	hidden, hiddenSet, err := optionalBatchBool(input, "hidden")
	if err != nil {
		return nil, err
	}
	if hiddenSet {
		args["hidden"] = hidden
	}
	for cliKey, mcpKey := range map[string]string{
		"frozen-row-count": "frozenRowCount", "frozen-column-count": "frozenColumnCount",
	} {
		value, exists, valueErr := batchStrictInt(input, cliKey, 0, math.MaxInt32, false)
		if valueErr != nil {
			return nil, valueErr
		}
		if exists {
			args[mcpKey] = value
		}
	}
	if tabColor, exists, valueErr := optionalBatchString(input, "tab-color"); valueErr != nil {
		return nil, valueErr
	} else if exists {
		args["tabColor"] = tabColor
	}
	hasProperty := nameSet || titleSet || hiddenSet || input["frozen-row-count"] != nil ||
		input["frozen-column-count"] != nil || input["tab-color"] != nil
	if rejectMixed && indexSet && hasProperty {
		return nil, fmt.Errorf("index 只能单独更新，不能与 name/title/hidden/frozen counts/tab-color 混用")
	}
	if !indexSet && !hasProperty {
		return nil, fmt.Errorf("name/title/index/hidden/frozen-row-count/frozen-column-count/tab-color 至少指定一个")
	}
	return args, nil
}

func buildBatchGridlineArgs(visibility string) batchArgsBuilder {
	return func(input map[string]any) (map[string]any, error) {
		args, err := buildBatchSheetIDOnly(input)
		if err != nil {
			return nil, err
		}
		args["visibility"] = visibility
		return args, nil
	}
}

func BuildBatchDeleteFloatChartArgs(input map[string]any) (map[string]any, error) {
	return buildBatchObjectDeleteArgs(input, "chart-id", "floatChartId")
}

func BuildBatchDeletePivotTableArgs(input map[string]any) (map[string]any, error) {
	return buildBatchObjectDeleteArgs(input, "pivot-table-id", "pivotTableId")
}

func BuildBatchDeleteCondFormatArgs(input map[string]any) (map[string]any, error) {
	return buildBatchObjectDeleteArgs(input, "rule-id", "ruleId")
}

func BuildBatchDeleteFilterViewArgs(input map[string]any) (map[string]any, error) {
	return buildBatchObjectDeleteArgs(input, "filter-view-id", "filterViewId")
}

func buildBatchObjectDeleteArgs(input map[string]any, cliID, mcpID string) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", cliID); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	objectID, err := requiredBatchString(input, cliID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sheetId": sheetID, mcpID: objectID}, nil
}

var batchCondKeys = []string{
	"numberCondition", "textCondition", "emptyCondition", "errorCondition", "duplicateCondition",
	"formulaCondition", "rankCondition", "averageCondition", "stdevCondition", "dataBarCondition",
	"iconSetCondition", "colorScaleCondition",
}

func buildBatchCondFormatArgs(input map[string]any, update bool) (map[string]any, error) {
	allowed := []string{"sheet-id", "ranges", "condition", "cell-style", "data-bar-style"}
	if update {
		allowed = append(allowed, "rule-id")
	}
	if err := validateBatchInputFields(input, allowed...); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	args := map[string]any{"sheetId": sheetID}
	if update {
		ruleID, ruleErr := requiredBatchString(input, "rule-id")
		if ruleErr != nil {
			return nil, ruleErr
		}
		args["ruleId"] = ruleID
	}
	ranges, rangesSet, err := batchJSONArray(input, "ranges", !update)
	if err != nil {
		return nil, err
	}
	if rangesSet {
		for i, item := range ranges {
			if text, ok := item.(string); !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("ranges[%d] 必须是非空字符串", i)
			}
		}
		args["ranges"] = ranges
	}
	condition, conditionSet, err := batchJSONObject(input, "condition", !update)
	if err != nil {
		return nil, err
	}
	if conditionSet {
		if len(condition) != 1 {
			return nil, fmt.Errorf("condition 必须且只能包含一种条件类型")
		}
		for key, value := range condition {
			known := false
			for _, allowedKey := range batchCondKeys {
				if key == allowedKey {
					known = true
					break
				}
			}
			if !known {
				return nil, fmt.Errorf("condition 包含不支持的条件类型 %q", key)
			}
			if _, ok := value.(map[string]any); !ok {
				return nil, fmt.Errorf("condition.%s 必须是 object", key)
			}
			args[key] = value
		}
	}
	for cliKey, mcpKey := range map[string]string{"cell-style": "cellStyle", "data-bar-style": "dataBarStyle"} {
		value, exists, valueErr := batchJSONObject(input, cliKey, false)
		if valueErr != nil {
			return nil, valueErr
		}
		if exists {
			args[mcpKey] = value
		}
	}
	if update && !rangesSet && !conditionSet && input["cell-style"] == nil && input["data-bar-style"] == nil {
		return nil, fmt.Errorf("ranges/condition/cell-style/data-bar-style 至少指定一个")
	}
	return args, nil
}

func BuildBatchCreateCondFormatArgs(input map[string]any) (map[string]any, error) {
	return buildBatchCondFormatArgs(input, false)
}

func BuildBatchUpdateCondFormatArgs(input map[string]any) (map[string]any, error) {
	return buildBatchCondFormatArgs(input, true)
}

func BuildBatchCreateFilterArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "range", "criteria"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	rangeAddr, err := requiredBatchString(input, "range")
	if err != nil {
		return nil, err
	}
	args := map[string]any{"sheetId": sheetID, "range": rangeAddr}
	if criteria, exists, valueErr := batchJSONArray(input, "criteria", false); valueErr != nil {
		return nil, valueErr
	} else if exists {
		args["criteria"] = criteria
	}
	return args, nil
}

func BuildBatchUpdateFilterArgs(input map[string]any) (map[string]any, error) {
	return buildUpdateFilterArgs(input, true)
}

// BuildUpdateFilterArgs converts standalone filter update flags while preserving
// the historical behavior that an empty criteria array is accepted.
func BuildUpdateFilterArgs(input map[string]any) (map[string]any, error) {
	return buildUpdateFilterArgs(input, false)
}

func buildUpdateFilterArgs(input map[string]any, rejectEmpty bool) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "criteria"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	criteria, exists, err := batchJSONArray(input, "criteria", false)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("criteria 必填")
	}
	if rejectEmpty && len(criteria) == 0 {
		return nil, fmt.Errorf("criteria 不能为空数组")
	}
	return map[string]any{"sheetId": sheetID, "criteria": criteria}, nil
}

func BuildBatchDeleteFilterArgs(input map[string]any) (map[string]any, error) {
	return buildBatchSheetIDOnly(input)
}

func BuildBatchCreateFilterViewArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "name", "range", "criteria"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	name, err := requiredBatchString(input, "name")
	if err != nil {
		return nil, err
	}
	rangeAddr, err := requiredBatchString(input, "range")
	if err != nil {
		return nil, err
	}
	args := map[string]any{"sheetId": sheetID, "name": name, "range": rangeAddr}
	if criteria, exists, valueErr := batchJSONArray(input, "criteria", false); valueErr != nil {
		return nil, valueErr
	} else if exists {
		args["criteria"] = criteria
	}
	return args, nil
}

func BuildBatchUpdateFilterViewArgs(input map[string]any) (map[string]any, error) {
	if err := validateBatchInputFields(input, "sheet-id", "filter-view-id", "name", "range", "criteria"); err != nil {
		return nil, err
	}
	base, err := buildBatchObjectDeleteArgs(map[string]any{
		"sheet-id": input["sheet-id"], "filter-view-id": input["filter-view-id"],
	}, "filter-view-id", "filterViewId")
	if err != nil {
		return nil, err
	}
	updated := false
	for key, target := range map[string]string{"name": "name", "range": "range"} {
		if value, exists, valueErr := optionalBatchString(input, key); valueErr != nil {
			return nil, valueErr
		} else if exists {
			base[target] = value
			updated = true
		}
	}
	if criteria, exists, valueErr := batchJSONArray(input, "criteria", false); valueErr != nil {
		return nil, valueErr
	} else if exists {
		base["criteria"] = criteria
		updated = true
	}
	if !updated {
		return nil, fmt.Errorf("name/range/criteria 至少指定一个")
	}
	return base, nil
}

func BuildBatchCreateFloatImageArgs(input map[string]any) (map[string]any, error) {
	if _, exists := input["file"]; exists {
		return nil, fmt.Errorf("file 不支持进入 batch；请先执行 media-upload，再把返回的 resourceUrl 作为 src")
	}
	if err := validateBatchInputFields(input, "sheet-id", "src", "range", "width", "height", "offset-x", "offset-y"); err != nil {
		return nil, err
	}
	sheetID, err := requiredBatchString(input, "sheet-id")
	if err != nil {
		return nil, err
	}
	src, err := requiredBatchString(input, "src")
	if err != nil {
		return nil, err
	}
	rangeAddr, err := requiredBatchString(input, "range")
	if err != nil {
		return nil, err
	}
	width, _, err := batchStrictInt(input, "width", 1, math.MaxInt32, true)
	if err != nil {
		return nil, err
	}
	height, _, err := batchStrictInt(input, "height", 1, math.MaxInt32, true)
	if err != nil {
		return nil, err
	}
	args := map[string]any{"sheetId": sheetID, "src": src, "range": rangeAddr, "width": width, "height": height}
	for cliKey, mcpKey := range map[string]string{"offset-x": "offsetX", "offset-y": "offsetY"} {
		value, exists, valueErr := batchStrictInt(input, cliKey, 0, math.MaxInt32, false)
		if valueErr != nil {
			return nil, valueErr
		}
		if exists {
			args[mcpKey] = value
		}
	}
	return args, nil
}

func BuildBatchUpdateFloatImageArgs(input map[string]any) (map[string]any, error) {
	if _, exists := input["file"]; exists {
		return nil, fmt.Errorf("file 不支持进入 batch；请先执行 media-upload，再把返回的 resourceUrl 作为 src")
	}
	if err := validateBatchInputFields(input, "sheet-id", "float-image-id", "src", "range", "width", "height", "offset-x", "offset-y"); err != nil {
		return nil, err
	}
	args, err := buildBatchObjectDeleteArgs(map[string]any{
		"sheet-id": input["sheet-id"], "float-image-id": input["float-image-id"],
	}, "float-image-id", "floatImageId")
	if err != nil {
		return nil, err
	}
	updated := false
	for key, target := range map[string]string{"src": "src", "range": "range"} {
		if value, exists, valueErr := optionalBatchString(input, key); valueErr != nil {
			return nil, valueErr
		} else if exists {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("%s 不能为空", key)
			}
			args[target] = value
			updated = true
		}
	}
	for cliKey, mcpKey := range map[string]string{
		"width": "width", "height": "height", "offset-x": "offsetX", "offset-y": "offsetY",
	} {
		min := 0
		if cliKey == "width" || cliKey == "height" {
			min = 1
		}
		value, exists, valueErr := batchStrictInt(input, cliKey, min, math.MaxInt32, false)
		if valueErr != nil {
			return nil, valueErr
		}
		if exists {
			args[mcpKey] = value
			updated = true
		}
	}
	if !updated {
		return nil, fmt.Errorf("src/range/width/height/offset-x/offset-y 至少指定一个")
	}
	return args, nil
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

// protectSheetMutationCommand marks a Sheet leaf as covered by the Schema→runtime
// confirmation gate and installs the Sheet --yes-only runtime guard.
//
// Sheet destructive commands intentionally do NOT honor interactive or piped
// stdin answers (unlike corecmd.ConfirmSafety). Agent/CI must pass --yes.
// When DeclareLeafMetadata already wrapped ConfirmSafety, this outer guard
// still wraps outside that pipeline. Order: ContractValidate (if any) →
// requireSheetMutationTargets (--node) → requireSheetMutationConfirmation →
// inner contract wrap. Missing --node fails before the Sheet --yes-only
// prompt. With --yes both confirmation layers bypass.
//
// Transitional dual gate: two runtime confirmation sources (outer Sheet
// --yes-only + inner ConfirmSafety). Do not remove the outer guard without
// proving ConfirmSafety alone keeps the --yes-only Sheet policy
// (see TestSheetMutationGuardRejectsPipedYesEvenWithContractConfirmSafety and
// the declare_leaf+sheet_marker assertion in
// TestUserRequiredSafetyHomologyWithRuntimeGate).
func protectSheetMutationCommand(cmd *cobra.Command, operation, targetHint string) {
	if cmd == nil {
		panic("protect sheet mutation command: nil command")
	}
	if cmd.Annotations != nil && cmd.Annotations[sheetMutationConfirmationGuardAnnotation] == "true" {
		panic(fmt.Sprintf("protect sheet mutation command: duplicate guard on %q", cmd.CommandPath()))
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[sheetMutationConfirmationGuardAnnotation] = "true"
	originalRunE := cmd.RunE
	if originalRunE == nil {
		panic(fmt.Sprintf("protect sheet mutation command: %q has no RunE", cmd.CommandPath()))
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if validate := ContractValidate(cmd); validate != nil {
			if err := validate(cmd, args); err != nil {
				return err
			}
		}
		// Local target checks before the Sheet --yes-only prompt (RFC §5.1).
		// Most destructive Sheet leaves require --node (with URL/id aliases).
		if err := requireSheetMutationTargets(cmd); err != nil {
			return err
		}
		if err := requireSheetMutationConfirmation(cmd, operation, targetHint); err != nil {
			return err
		}
		return originalRunE(cmd, args)
	}
}

// requireSheetMutationTargets fails closed on missing document identity before
// any confirmation prompt. Commands without a --node flag are skipped.
func requireSheetMutationTargets(cmd *cobra.Command) error {
	if cmd == nil || cmd.Flags().Lookup("node") == nil {
		return nil
	}
	_, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
	return err
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
source-range 的语义按 toolName 隔离：set-dropdown 中表示下拉候选项来源，
range fill/copy-to/move-to 中表示待填充、复制或移动的数据源区域。

支持的 CLI 命令名:
  range clear / range update / merge-cells / unmerge-cells / update-dimension
  range fill / range copy-to / add-dimension / delete-dimension / move-dimension
  group-dimension / ungroup-dimension
  set-dropdown / delete-dropdown / csv-put / delete-float-image
  range set-style / replace / insert-dimension / range move-to / range sort
  new / delete-sheet / update / show-gridline / hide-gridline
  chart delete / pivot-table delete
  cond-format create / cond-format update / cond-format delete
  filter create / filter update / filter delete
  filter-view create / filter-view update / filter-view delete
  create-float-image / update-float-image
其中 csv-put 与独立命令语义一致：input 可传 "auto-convert":false，关闭非公式
字段的自动类型转换；CSV 字段值以 = 开头时仍按公式解析，其他字段按普通文本
原样写入（例如 "'=1+1" 会保留前置单引号）。缺省或 true 保持现有自动转换行为。
set-dropdown 不接受顶层 colors/source-colors；Inline 颜色应写在 options[].color，
SourceRange 颜色写入暂不支持。

新增子操作执行严格本地校验：未知字段、错误 JSON 类型、把整数或布尔值写成
字符串，以及 float-image 的 file 本地上传模式都会在发请求前拒绝。
批内不支持 $ref 或引用前序结果；create 返回的服务端原始 data（含 ID 时）只能
供调用方在下一次请求使用。update 改名或 delete-sheet 后，后续操作若仍按旧名
定位会失败：严格模式整批回滚，宽松模式则保留此前已成功的操作。跨操作关联时
优先使用稳定的 sheet ID。

注意：batch-update 中 group-dimension 适合默认展开分组；需要 --group-state fold 时请使用独立
dws sheet group-dimension 命令。

--operations 是 JSON 数组，每项包含:
  toolName  CLI 命令名（如 "range clear", "range update"）
  input     该命令的入参（不含 --node），键用 flag 名去掉 --

CLI 会在本地完成翻译后把最终数组编码到 MCP operationsJson 字符串中，以保持
number/boolean 类型；--dry-run 展示的是这个带转义的实际远端参数。写失败时不会
切换到旧 operations 入口重试。

完整映射表见:
  dingtalk-workspace/references/products/sheet/sheet-batch-operations.md`,
		Example: `  dws sheet batch-update --node NODE_ID --operations '[
    {"toolName":"range clear","input":{"sheet-id":"Sheet1","range":"A1:B3","type":"content"}},
    {"toolName":"range update","input":{"sheet-id":"Sheet1","range":"A1","values":[[{"type":"text","text":"hello"}]]}},
    {"toolName":"merge-cells","input":{"sheet-id":"Sheet1","range":"A1:B1","merge-type":"mergeAll"}},
    {"toolName":"csv-put","input":{"sheet-id":"Sheet1","start-cell":"C1","csv":"001,2026/8/1,=1+1,\u0027=1+1","auto-convert":false}}
  ]'

  # 宽松模式
  dws sheet batch-update --node NODE_ID --continue-on-error --operations '[...]'`,
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
			continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
			toolArgs, err := buildBatchUpdateToolArgsForCommand(mustGetFlag(cmd, "node"), translated, continueOnError)
			if err != nil {
				return err
			}
			err = callMCPTool("batch_update", toolArgs)
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
		Example: `  dws sheet range batch-clear --node NODE_ID --ranges '["Sheet1!A1:B3","Sheet2!C1:D5"]'
  dws sheet range batch-clear --node NODE_ID --ranges '["Sheet1!A1:Z1000"]' --type all`,
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
				// 与 batch-set-style 共用同一个拆分器：此前这里自己拆，且只按原始串里
				// ! 的位置判断，" !A1:B2" 会拆出空工作表名并带着 sheetId:"" 下发。
				sheetName, rangeAddr, err := splitSheetPrefixedRange(rng, i)
				if err != nil {
					return err
				}
				operations = append(operations, map[string]any{
					"toolName": "clear_range",
					"input": map[string]any{
						"sheetId": sheetName,
						"range":   rangeAddr,
						"type":    clearType,
					},
				})
			}
			toolArgs, err := buildBatchUpdateToolArgsForCommand(mustGetFlag(cmd, "node"), operations, false)
			if err != nil {
				return err
			}
			return callMCPTool("batch_update", toolArgs)
		},
	}
}

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

// parseBoolFlag reads a string flag and parses it as a boolean, accepting the
// natural `--flag true` / `--flag false` space syntax. Registering these
// switches as String (not Bool) flags avoids cobra swallowing `false` as a
// positional arg — which silently inverted `--enabled false` into enable.
func parseBoolFlag(cmd *cobra.Command, name string) (bool, error) {
	raw := strings.TrimSpace(mustGetFlag(cmd, name))
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("--%s 只接受 true 或 false，got %q", name, raw)
	}
	return v, nil
}

// resolveAitableExportFormat keeps the historical `--format excel` spelling
// executable without advertising it as the business contract. `--format` is
// the root output-format flag, so new calls use `--export-format excel
// --format json`; only non-output values are treated as the legacy export
// format.
func resolveAitableExportFormat(cmd *cobra.Command) string {
	if value, _ := cmd.Flags().GetString("export-format"); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	legacy, _ := cmd.Flags().GetString("format")
	legacy = strings.TrimSpace(legacy)
	switch strings.ToLower(legacy) {
	case "", "json", "table", "raw", "pretty", "ndjson", "csv":
		return ""
	default:
		return legacy
	}
}

// ──────────────────────────────────────────────────────────
// dws aitable — AI 表格
// 中文 tool name 映射: CLI 英文命令 → MCP 中文 tool name
// ──────────────────────────────────────────────────────────

// resolveRecordsFlag resolves the records JSON string from either --records,
// --records-file (reads from file), or --fields (alias for --records).
// Priority: --records-file > --records > --fields
func resolveRecordsFlag(cmd *cobra.Command) (string, error) {
	// Priority 1: --records-file (read JSON from file, best for Windows/long payloads)
	if filePath, _ := cmd.Flags().GetString("records-file"); filePath != "" {
		filePath = strings.TrimSpace(filePath)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("--records-file read failed: %w\n  hint: ensure the file path is correct and the file contains valid JSON", err)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return "", fmt.Errorf("--records-file is empty: %s", filePath)
		}
		return content, nil
	}

	// Priority 2: --records (standard usage)
	if v := mustGetFlag(cmd, "records"); v != "" {
		return v, nil
	}

	// Priority 3: --fields (common LLM mistake, treat as alias for --records)
	if v, _ := cmd.Flags().GetString("fields"); v != "" {
		return v, nil
	}

	return "", fmt.Errorf("missing required flag(s): --records (example: --records '[{\"cells\":{\"fldTextId\":\"文本内容\"}}]')\n  hint: for large payloads or Windows, use --records-file ./path/to/records.json")
}

// resolveWorkflowDSL reads --dsl from inline JSON, @file, or stdin (-), then
// decodes the MCP-facing workflow-dsl/v1 object. Detailed DSL validation stays
// on the workflow service so callers receive its structured issues response.
func resolveWorkflowDSL(cmd *cobra.Command) (map[string]any, error) {
	raw := mustGetFlag(cmd, "dsl")
	switch {
	case raw == "-":
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("--dsl stdin read failed: %w", err)
		}
		raw = string(data)
	case strings.HasPrefix(raw, "@"):
		path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
		if path == "" {
			return nil, fmt.Errorf("--dsl file path must not be empty\n  hint: use --dsl @workflow.json")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--dsl file read failed: %w\n  hint: ensure the file exists and contains a workflow-dsl/v1 JSON object", err)
		}
		raw = string(data)
	}

	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("--dsl must not be empty\n  hint: pass inline JSON, @workflow.json, or - for stdin")
	}

	var dsl map[string]any
	if err := json.Unmarshal([]byte(raw), &dsl); err != nil {
		return nil, fmt.Errorf("--dsl JSON parse failed: %w\n  hint: --dsl must be a workflow-dsl/v1 JSON object", err)
	}
	if dsl == nil {
		return nil, fmt.Errorf("--dsl must be a JSON object, got null")
	}
	return dsl, nil
}

// recordQueryFetchAll implements --all auto-pagination for record query.
//   - pageLimit controls max pages (default 50, 0 = unlimited)
//   - Mid-loop errors break gracefully, outputting already-fetched data
//   - Merged result preserves last page's cursor so caller can resume
func recordQueryFetchAll(toolArgs map[string]any, pageLimit int) error {
	const pageDelayMs = 200
	const serverID = "aitable"

	ctx := context.Background()
	var allRecords []any
	page := 0
	lastCursor := ""

	for {
		page++

		if page > 1 {
			time.Sleep(time.Duration(pageDelayMs) * time.Millisecond)
		}

		fmt.Fprintf(os.Stderr, "[page %d] fetching...\n", page)

		result, err := deps.Caller.CallTool(ctx, serverID, "query_records", toolArgs)
		if err != nil {
			if page == 1 {
				return WrapError(err)
			}
			fmt.Fprintf(os.Stderr, "[page %d] error, stopping pagination: %v\n", page, err)
			break
		}

		// Extract text content from tool result
		var text string
		for _, c := range result.Content {
			if c.Type == "text" && c.Text != "" {
				text = c.Text
				break
			}
		}
		if text == "" {
			break
		}

		var resp map[string]any
		if err := json.Unmarshal([]byte(text), &resp); err != nil {
			if page == 1 {
				deps.Out.PrintRaw(text)
				return nil
			}
			break
		}

		// Extract records from response
		var pageRecords []any
		if data, ok := resp["data"].(map[string]any); ok {
			if recs, ok := data["records"].([]any); ok {
				pageRecords = recs
			}
		} else if recs, ok := resp["records"].([]any); ok {
			pageRecords = recs
		}
		allRecords = append(allRecords, pageRecords...)

		// Extract cursor for next page (MCP returns "nextCursor" or "cursor")
		cursor := ""
		if data, ok := resp["data"].(map[string]any); ok {
			if c, ok := data["nextCursor"].(string); ok && c != "" {
				cursor = c
			} else if c, ok := data["cursor"].(string); ok && c != "" {
				cursor = c
			}
		} else if c, ok := resp["nextCursor"].(string); ok && c != "" {
			cursor = c
		} else if c, ok := resp["cursor"].(string); ok && c != "" {
			cursor = c
		}

		if cursor == "" {
			lastCursor = ""
			break
		}

		// Check page limit (0 = unlimited)
		if pageLimit > 0 && page >= pageLimit {
			lastCursor = cursor
			fmt.Fprintf(os.Stderr, "[pagination] reached page limit (%d), stopping. Use --page-limit 0 to fetch all.\n", pageLimit)
			break
		}

		lastCursor = ""
		toolArgs["cursor"] = cursor
	}

	fmt.Fprintf(os.Stderr, "[pagination] done: %d pages, %d total records\n", page, len(allRecords))

	// Build merged output — preserve cursor & has_more for resume capability
	mergedData := map[string]any{
		"records":    allRecords,
		"totalCount": len(allRecords),
	}
	if lastCursor != "" {
		mergedData["cursor"] = lastCursor
		mergedData["hasMore"] = true
	} else {
		mergedData["hasMore"] = false
	}

	return deps.Out.PrintJSON(map[string]any{"data": mergedData})
}

// ─── filters 格式校验 ──────────────────────────────────────────────────────
//
// validateFiltersStructure 校验 --filters 传入的 JSON 结构是否符合规范。
// 正确格式：{"operator":"and|or","operands":[{"operator":"eq","operands":["fieldId","value"]}]}
// 常见错误：传入 MCP 格式的数组 [{"fieldId":"xxx","operator":"is","value":["xxx"]}]
//
//	或缺少根节点的 and/or 包装直接传入比较操作符。
func validateFiltersStructure(parsed any, rawJSON string) error {
	if parsed == nil {
		return nil
	}

	filterMap, ok := parsed.(map[string]any)
	if !ok {
		// 根节点不是对象（可能是数组），给出明确提示
		return fmt.Errorf(`invalid --filters structure: root must be a JSON object with "operator":"and|or", got array or non-object.
  Correct format: {"operator":"and","operands":[{"operator":"eq","operands":["<fieldId>","<value>"]}]}
  Hint: wrap your conditions in {"operator":"and","operands":[...]}`)
	}

	op, hasOp := filterMap["operator"]
	if !hasOp {
		return fmt.Errorf(`invalid --filters structure: missing "operator" field at root level.
  Correct format: {"operator":"and","operands":[{"operator":"eq","operands":["<fieldId>","<value>"]}]}`)
	}

	opStr, isStr := op.(string)
	if !isStr {
		return fmt.Errorf(`invalid --filters structure: "operator" must be a string, got %T`, op)
	}

	opLower := strings.ToLower(opStr)
	if opLower != "and" && opLower != "or" {
		// 根节点 operator 不是 and/or，可能是直接传了比较操作符如 "eq"、"is" 等
		return fmt.Errorf(`invalid --filters structure: root "operator" must be "and" or "or", got %q.
  Correct format: {"operator":"and","operands":[{"operator":"%s","operands":["<fieldId>","<value>"]}]}
  Hint: wrap your condition in {"operator":"and","operands":[<your_condition>]}`, opStr, opStr)
	}

	operands, hasOperands := filterMap["operands"]
	if !hasOperands {
		return fmt.Errorf(`invalid --filters structure: missing "operands" array at root level.
  Correct format: {"operator":"and","operands":[{"operator":"eq","operands":["<fieldId>","<value>"]}]}`)
	}

	operandsArr, isArr := operands.([]any)
	if !isArr {
		return fmt.Errorf(`invalid --filters structure: "operands" must be an array, got %T`, operands)
	}

	// 校验每个子条件的 operator 是否为合法值
	for i, item := range operandsArr {
		cond, ok := item.(map[string]any)
		if !ok {
			continue
		}
		childOp, has := cond["operator"]
		if !has {
			continue
		}
		childOpStr, isStr := childOp.(string)
		if !isStr {
			continue
		}
		if msg, bad := unsupportedFilterOperators[childOpStr]; bad {
			return fmt.Errorf("unsupported filter operator %q in operands[%d]: %s", childOpStr, i, msg)
		}
		if suggestion := suggestOperator(childOpStr); suggestion != "" {
			return fmt.Errorf(`invalid filter operator %q in operands[%d]. Did you mean %q?
  Supported operators: eq, ne, gt, lt, gte, lte, contain, exclusive, exist, un_exist, any_of, all_of, none_of, date_eq, before, after, not_before, not_after
  Hint: use "eq" for equals, "ne" for not-equals, "contain" for text contains;
        date fields use date_eq/before/after/not_before/not_after (NOT eq/gte/contain)`, childOpStr, i, suggestion)
		}
	}

	return nil
}

// normalizeFilters 将 MCP 格式的子条件（fieldId/operator/value 对象）
// 自动转换为底层 API 需要的 operands 数组格式。
// 输入: {"operator":"and","operands":[{"fieldId":"X","operator":"eq","value":"Y"}]}
// 输出: {"operator":"and","operands":[{"operator":"eq","operands":["X","Y"]}]}
// 如果子条件已经是 operands 格式则不做转换。
func normalizeFilters(parsed any) any {
	if parsed == nil {
		return nil
	}
	filterMap, ok := parsed.(map[string]any)
	if !ok {
		return parsed
	}

	operands, has := filterMap["operands"]
	if !has {
		return parsed
	}
	operandsArr, isArr := operands.([]any)
	if !isArr {
		return parsed
	}

	normalized := make([]any, 0, len(operandsArr))
	for _, item := range operandsArr {
		cond, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		// 检测是否是 MCP 格式（有 fieldId 字段）
		fieldID, hasFieldID := cond["fieldId"]
		if hasFieldID {
			// MCP 格式：{fieldId, operator, value} → {operator, operands:[fieldId, value]}
			childOp := cond["operator"]
			value, hasValue := cond["value"]
			newCond := map[string]any{
				"operator": childOp,
			}
			if hasValue {
				newCond["operands"] = []any{fieldID, value}
			} else {
				// exist/un_exist 没有 value
				newCond["operands"] = []any{fieldID}
			}
			normalized = append(normalized, newCond)
		} else if _, hasChildOperands := cond["operands"]; hasChildOperands {
			// 已经是 operands 格式，递归处理嵌套
			normalized = append(normalized, normalizeFilters(cond))
		} else {
			normalized = append(normalized, cond)
		}
	}

	return map[string]any{
		"operator": filterMap["operator"],
		"operands": normalized,
	}
}

// normalizeViewConfigFilter 将 view config 中的 filter 字段规范化为服务端要求的数组格式。
// 服务端 POJO 要求 config.filter 为 []FilterRule（JSON array）。
// 常见错误格式：
//   - 传了对象 {"operator":"and","operands":[...]} → 自动 wrap 为 [对象]
//   - 子条件使用 MCP 简写格式 {fieldId,operator,value} → 自动 normalize 为 operands 格式
func normalizeViewConfigFilter(filterVal any) any {
	switch v := filterVal.(type) {
	case []any:
		// 已经是数组格式，对每个元素做子条件 normalize
		for i, item := range v {
			v[i] = normalizeFilters(item)
		}
		return v
	case map[string]any:
		// 对象格式，先 normalize 子条件，再 wrap 为数组
		normalized := normalizeFilters(v)
		return []any{normalized}
	default:
		return filterVal
	}
}

// ensureArray 确保值为 JSON array 格式，如果是对象则 wrap 为 [对象]。
func ensureArray(val any) any {
	switch v := val.(type) {
	case []any:
		return v
	case map[string]any:
		return []any{v}
	default:
		return val
	}
}

// knownViewConfigKeys 是 update_view.config 接受的全部 key。
// 与服务端 UpdateViewConfigInput.java 1:1 对齐，更新时请同步。
var knownViewConfigKeys = map[string]bool{
	"visibleFieldIds": true,
	"filter":          true,
	"sort":            true,
	"group":           true,
	"fieldWidths":     true,
	"aggregate":       true,
	"kanbanCard":      true,
	"ganttTimebar":    true,
	"galleryCard":     true,
}

// viewConfigKeyToAttrSubcmd 列出"看似 view config 字段、但服务端走独立工具"的 key
// 映射到正确的 dws CLI 子命令路径，便于在 normalizeViewConfigBlock 给精准引导。
// 当用户错把这类 key 塞进 --config 时，CLI 立即提示替代命令而非让 server 静默忽略。
var viewConfigKeyToAttrSubcmd = map[string]string{
	"flags":              "dws aitable view lock [--off]（设置/解除锁定）",
	"frozenColCount":     "dws aitable view update frozen-cols --count N",
	"cellHeight":         "dws aitable view update row-height --cell-height N",
	"rowHeightLevel":     "dws aitable view update row-height --cell-height N（请用像素值，不是 level）",
	"conditionalFormats": "dws aitable view update fill-color-rule --json '[...]'",
}

// normalizeViewConfigBlock 校验 config map 中的 key 合法性并对 filter/sort/group
// 做服务端要求的数组格式自动修正。本函数原地修改 cfgMap，并在遇到非法格式时返回 error。
// 同时把 "可能不被支持" 的 unknown key 提示打到 stderr（不阻断）。
func normalizeViewConfigBlock(cfgMap map[string]any) error {
	var unknownKeys []string
	var routedKeys []string // unknownKeys 中"应使用独立子命令"的子集
	for k := range cfgMap {
		if !knownViewConfigKeys[k] {
			unknownKeys = append(unknownKeys, k)
			if _, ok := viewConfigKeyToAttrSubcmd[k]; ok {
				routedKeys = append(routedKeys, k)
			}
		}
	}
	if len(routedKeys) > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  以下 key 不能通过 view update --config 修改（服务端走独立工具），请改用对应子命令：\n")
		for _, k := range routedKeys {
			fmt.Fprintf(os.Stderr, "   - %s → %s\n", k, viewConfigKeyToAttrSubcmd[k])
		}
	}
	// 仍保留泛化 warning，覆盖既不在白名单也不在 routed 表里的真·未知 key
	var trulyUnknown []string
	for _, k := range unknownKeys {
		if _, routed := viewConfigKeyToAttrSubcmd[k]; !routed {
			trulyUnknown = append(trulyUnknown, k)
		}
	}
	if len(trulyUnknown) > 0 {
		fmt.Fprintf(os.Stderr, "⚠️  warning: config 中包含可能不被支持的 key: %v，服务端可能会静默忽略这些配置\n", trulyUnknown)
		fmt.Fprintf(os.Stderr, "   当前 view config 支持的 key: visibleFieldIds, filter, sort, group, fieldWidths(仅Grid视图), aggregate(仅Grid视图), kanbanCard(仅Kanban视图), ganttTimebar(仅Gantt视图), galleryCard(仅Gallery视图)\n")
	}
	if filterVal, hasFilter := cfgMap["filter"]; hasFilter && filterVal != nil {
		switch filterVal.(type) {
		case []any, map[string]any:
			cfgMap["filter"] = normalizeViewConfigFilter(filterVal)
		default:
			return fmt.Errorf("invalid config.filter: must be a JSON array or object, got %T\n"+
				"  hint: view config filter 格式为数组: [{\"operator\":\"and\",\"operands\":[{\"operator\":\"eq\",\"operands\":[\"fieldId\",\"value\"]}]}]\n"+
				"  注意: 与 record query --filters（对象格式）不同，view config filter 外层必须是数组", filterVal)
		}
	}
	if sortVal, hasSort := cfgMap["sort"]; hasSort && sortVal != nil {
		switch sortVal.(type) {
		case []any, map[string]any:
			cfgMap["sort"] = ensureArray(sortVal)
		default:
			return fmt.Errorf("invalid config.sort: must be a JSON array, got %T\n"+
				"  hint: sort 格式: [{\"fieldId\":\"fldXXX\",\"direction\":\"asc|desc\"}]", sortVal)
		}
	}
	if groupVal, hasGroup := cfgMap["group"]; hasGroup && groupVal != nil {
		switch groupVal.(type) {
		case []any, map[string]any:
			cfgMap["group"] = ensureArray(groupVal)
		default:
			return fmt.Errorf("invalid config.group: must be a JSON array, got %T\n"+
				"  hint: group 格式: [{\"fieldId\":\"fldXXX\",\"direction\":\"asc|desc\"}]", groupVal)
		}
	}
	return nil
}

// validFilterOperators 是 MCP 支持的合法过滤操作符集合
var validFilterOperators = map[string]bool{
	"eq": true, "ne": true, "gt": true, "lt": true, "gte": true, "lte": true,
	"contain": true, "exclusive": true, "exist": true, "un_exist": true,
	"any_of": true, "all_of": true, "none_of": true,
	"date_eq": true, "before": true, "after": true, "not_before": true, "not_after": true,
	"and": true, "or": true, // 嵌套逻辑组也合法
}

// unsupportedFilterOperators 列出产品层面未实现、会静默返回 0 条记录的操作符，
// 直接给出明确替代方案而非泛化的拼写建议（避免被 suggestOperator 误判为拼写错误）。
//
// 二者均经集成测试验证：后端对 date 字段静默返回 0 条，前端筛选 UI 也无对应选项。
//   - date_between：区间过滤 → 用 not_before + not_after 组合表达范围
//   - from_now：相对天数过滤 → 自行计算日期后用 not_before/not_after
var unsupportedFilterOperators = map[string]string{
	"date_between": `date 字段不支持区间(between)过滤。请用 not_before + not_after 两个条件组合表达范围，例如：` +
		`{"operator":"and","operands":[{"operator":"not_before","operands":["fldXXX","2026-05-01"]},{"operator":"not_after","operands":["fldXXX","2026-05-31"]}]}`,
	"from_now": `date 字段不支持 from_now(相对天数)过滤。请自行计算目标日期后改用 not_before / not_after，` +
		`例如"未来7天内"= not_before(今天) + not_after(今天+7)。`,
}

// operatorAliases 常见错误拼写到正确操作符的映射
var operatorAliases = map[string]string{
	"equal": "eq", "equals": "eq", "is": "eq", "==": "eq",
	"not_equal": "ne", "not_equals": "ne", "is_not": "ne", "!=": "ne",
	"like": "contain", "contains": "contain", "include": "contain",
	"greater_than": "gt", "less_than": "lt",
	"greater_than_or_equal": "gte", "less_than_or_equal": "lte",
}

// suggestOperator 检查操作符是否合法，如果不合法则返回建议值，合法返回空字符串
func suggestOperator(op string) string {
	if validFilterOperators[op] {
		return ""
	}
	if suggestion, ok := operatorAliases[op]; ok {
		return suggestion
	}
	// 未知操作符，建议 eq
	return "eq"
}

// ─── aitable 专用重试 wrapper ──────────────────────────────────────────────
//
// 仅影响 aitable 产品线，不影响其他钉钉业务（calendar/contact/chat 等）。
// 对网络瞬断、服务端 5xx、retryable:true 的错误自动指数退避重试，
// 同时解决 CI 偶发失败和线上用户关键链路被阻断的问题。

const aitableMaxRetries = 3

// callAitableTool 是 aitable 专用的 MCP 调用入口，带自动重试。
// 替代直接调用 callMCPTool，对网络抖动和服务端瞬态错误进行透明重试。
func callAitableTool(toolName string, args map[string]any) error {
	var lastErr error
	for attempt := 0; attempt <= aitableMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second // 1s, 2s, 4s
			fmt.Fprintf(os.Stderr, "[aitable retry %d/%d] %s after %v...\n", attempt, aitableMaxRetries, toolName, backoff)
			time.Sleep(backoff)
		}

		err := callMCPTool(toolName, args)
		if err == nil {
			return nil
		}

		// 判断是否为可重试错误
		if !isAitableRetryableError(err) {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// callAitableHelperTool 是 aitable-helper 专用的 MCP 调用入口，路由到 aitable-helper server，带自动重试。
func callAitableHelperTool(toolName string, args map[string]any) error {
	var lastErr error
	for attempt := 0; attempt <= aitableMaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			fmt.Fprintf(os.Stderr, "[aitable-helper retry %d/%d] %s after %v...\n", attempt, aitableMaxRetries, toolName, backoff)
			time.Sleep(backoff)
		}

		err := callMCPToolOnServer("aitable-helper", toolName, args)
		if err == nil {
			return nil
		}

		if !isAitableRetryableError(err) {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// ─── view 子命令公共 helper ──────────────────────────────────────────

// getViewRaw 调用 get_views 服务端 filter 单个 viewId，返回原始视图 map 与 viewType。
// 当响应中没有目标视图、解析失败、或服务端报错时，返回带 CLIError 的错误。
// 该函数是 view get/update <attr> 子命令共用的 preflight。
func getViewRaw(ctx context.Context, baseID, tableID, viewID string) (view map[string]any, viewType string, err error) {
	args := map[string]any{
		"baseId":  baseID,
		"tableId": tableID,
		"viewIds": []string{viewID},
	}
	raw, err := callMCPToolReturnText(ctx, "get_views", args)
	if err != nil {
		return nil, "", err
	}
	var parsed map[string]any
	if e := json.Unmarshal([]byte(raw), &parsed); e != nil {
		return nil, "", &CLIError{
			Code:    CodeMCPToolError,
			Message: fmt.Sprintf("get_views response is not valid JSON: %v", e),
		}
	}
	data, _ := parsed["data"].(map[string]any)
	views, _ := data["views"].([]any)
	if len(views) == 0 {
		return nil, "", &CLIError{
			Code:       CodeMCPToolError,
			Message:    fmt.Sprintf("view %s not found in table %s", viewID, tableID),
			Suggestion: "用 dws aitable view get --table-id <tableId> 查看可用 viewId 列表",
		}
	}
	view, _ = views[0].(map[string]any)
	if view == nil {
		return nil, "", &CLIError{
			Code:    CodeMCPToolError,
			Message: "get_views[0] is not a JSON object",
		}
	}
	vt, _ := view["viewType"].(string)
	return view, vt, nil
}

// requireViewType 校验实际 viewType 是否在白名单 expected 中，不在则返回带建议的 CLIError。
func requireViewType(actual, attr string, expected []string) error {
	for _, e := range expected {
		if actual == e {
			return nil
		}
	}
	return &CLIError{
		Code: CodeMCPToolError,
		Message: fmt.Sprintf("view type %q does not support attribute %q (only %v)",
			actual, attr, expected),
		Suggestion: fmt.Sprintf("请选择一个 %v 类型的视图 viewId 后重试，或用 view get 查看完整视图配置", expected),
	}
}

// dispatchCardKey 把 "card" 子命令分发到 kanbanCard / galleryCard 服务端字段名。
// 仅 Kanban、Gallery 两类视图支持 card；其他视图返回 CLIError。
func dispatchCardKey(viewType string) (string, error) {
	switch viewType {
	case "Kanban":
		return "kanbanCard", nil
	case "Gallery":
		return "galleryCard", nil
	default:
		return "", requireViewType(viewType, "card", []string{"Kanban", "Gallery"})
	}
}

// walkViewPath 按 dotted path（如 "custom.widthMap"）从 view map 中投影子字段；
// 中途遇到非 map 或不存在返回 nil。
func walkViewPath(view map[string]any, path string) any {
	if path == "" {
		return nil
	}
	var cur any = view
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[seg]
	}
	return cur
}

// printViewSubBlock 把已经从 view get 响应里投影出的子块包成标准 envelope
// {status, data, error} 输出，与 callAitableTool 的响应形态一致；测试 / agent /
// jq 都按 .data 取值。子块为 nil 时输出 data:{}。
func printViewSubBlock(block any) error {
	if block == nil {
		block = map[string]any{}
	}
	envelope := map[string]any{
		"status": "success",
		"data":   block,
		"error":  map[string]any{},
	}
	return deps.Out.PrintJSON(envelope)
}

// findFormViewByID 从 list_form_views 响应里递归定位 viewId 匹配的表单对象。
// 服务端的 viewIds 过滤参数当前不生效（返回全表所有表单），故 form get 在客户端
// 侧按 viewId 精确筛出单条，避免退化成等价 form list。
func findFormViewByID(node any, viewID string) (map[string]any, bool) {
	switch v := node.(type) {
	case map[string]any:
		if id, _ := v["viewId"].(string); id == viewID {
			return v, true
		}
		for _, val := range v {
			if found, ok := findFormViewByID(val, viewID); ok {
				return found, true
			}
		}
	case []any:
		for _, el := range v {
			if found, ok := findFormViewByID(el, viewID); ok {
				return found, true
			}
		}
	}
	return nil, false
}

// collectStringFlag 把 cmd 上 flagName 的字符串值（若非空）以 jsonKey 写入 out map。
func collectStringFlag(cmd *cobra.Command, flagName, jsonKey string, out map[string]any) {
	if v, _ := cmd.Flags().GetString(flagName); v != "" {
		out[jsonKey] = v
	}
}

// collectBoolFlag 在 flag 被显式设置时（cmd.Flags().Changed），把 bool 值以 jsonKey 写入 out。
// 区别于 GetBool 一定写入默认 false。
func collectBoolFlag(cmd *cobra.Command, flagName, jsonKey string, out map[string]any) {
	if !cmd.Flags().Changed(flagName) {
		return
	}
	v, _ := cmd.Flags().GetBool(flagName)
	out[jsonKey] = v
}

// mergeUpdateBlock 把 --json 解析的 baseObj 与 typed flag 收集的 typedFields 合并。
// typedFields 优先（覆盖 baseObj 同 key），冲突时在 stderr 给出提示便于用户排查。
// 当 jsonStr 为空时仅返回 typedFields；当 typedFields 为空时仅返回 jsonStr 解析结果。
func mergeUpdateBlock(jsonStr string, typedFields map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if jsonStr != "" {
		parsed := jsonStringToMap(jsonStr)
		m, ok := parsed.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid --json: expect a JSON object, got %T", parsed)
		}
		for k, v := range m {
			out[k] = v
		}
	}
	for k, v := range typedFields {
		if _, exists := out[k]; exists {
			fmt.Fprintf(os.Stderr, "⚠️  --json 与 typed flag 同时设置了 %q，使用 typed flag 的值\n", k)
		}
		out[k] = v
	}
	return out, nil
}

// callUpdateViewWithBlock 组装 update_view 的 toolArgs 并调用。
// blockKey 为 config 子块的服务端 key（如 "kanbanCard"）；当 blockKey == ""
// 时直接把 extra 合并到 toolArgs 顶层（供 name 子命令传 newViewName 用）。
func callUpdateViewWithBlock(baseID, tableID, viewID, blockKey string, blockValue any, extra map[string]any) error {
	toolArgs := map[string]any{
		"baseId":  baseID,
		"tableId": tableID,
		"viewId":  viewID,
	}
	if blockKey != "" {
		toolArgs["config"] = map[string]any{blockKey: blockValue}
	}
	for k, v := range extra {
		toolArgs[k] = v
	}
	return callAitableTool("update_view", toolArgs)
}

// isAitableRetryableError 判断 aitable MCP 调用错误是否值得重试。
// 仅对网络类错误重试，业务错误（参数/权限/资源不存在）不重试。
func isAitableRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// 网络瞬态错误
	retryablePatterns := []string{
		"timeout", "deadline exceeded", "connection reset",
		"connection refused", "broken pipe", "eof",
		"network is unreachable", "i/o timeout",
		"tls handshake", "server misbehaving", "temporary failure",
		"no such host",
	}
	for _, p := range retryablePatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}

	// 服务端 5xx（网关/内部错误）
	if strings.Contains(msg, "system_error") || strings.Contains(msg, "internal_error") ||
		strings.Contains(msg, "service_unavailable") || strings.Contains(msg, "gateway_timeout") {
		return true
	}

	// MCP 框架层返回的 retryable 标记
	if strings.Contains(msg, "retryable") && strings.Contains(msg, "true") {
		return true
	}

	return false
}

// parseFieldsJSON parses a --fields value as a JSON array, tolerating
// a {"fields": [...]} wrapper that AI Agents sometimes produce.
func parseFieldsJSON(raw string) ([]any, error) {
	var fields []any
	if err := json.Unmarshal([]byte(raw), &fields); err == nil {
		return fields, nil
	}
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil {
		if arr, ok := wrapper["fields"].([]any); ok {
			return arr, nil
		}
	}
	return nil, fmt.Errorf("--fields JSON parse failed: expect a JSON array [...]\n  hint: example: '[{\"fieldName\":\"名称\",\"type\":\"text\"}]'")
}

// resolveFormUpdateTitle 折叠 form update 的 --title / --name 别名为单个 title 值。
// 优先 --title，未设置时回退到 --name；两者都未传返回 ""。
// 抽出独立函数便于单测覆盖。
func resolveFormUpdateTitle(cmd *cobra.Command) string {
	if v, _ := cmd.Flags().GetString("title"); v != "" {
		return v
	}
	if v, _ := cmd.Flags().GetString("name"); v != "" {
		return v
	}
	return ""
}

func newAitableCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "aitable",
		Short: "AI 表格操作",
		Long: `管理钉钉 AI 表格：Base 管理、数据表、字段、记录、视图、表单、仪表盘、图表、导入导出。

命令结构:
  dws aitable base       [list|search|get|get-primary-doc-id|create|update|delete|copy]  Base 管理
  dws aitable table      [get|create|update|delete]                                     数据表管理
  dws aitable field      [get|create|update|delete|search-options]                      字段管理
  dws aitable record     [query|create|update|delete]                                   记录管理
  dws aitable view       [get|create|update|delete]                                     视图管理
  dws aitable form       [list|delete|update]                                           表单管理
  dws aitable form field [list|update|hide]                                             表单字段管理
  dws aitable form share [get|update|notify]                                            表单分享管理
  dws aitable dashboard  [get|create|update|delete|config-example]                      仪表盘管理
  dws aitable chart      [get|create|update|delete|widgets-example]                     图表管理
  dws aitable export     data                                                           数据导出
  dws aitable import     [upload|data]                                                  数据导入
  dws aitable attachment upload                                                         附件上传准备
  dws aitable template   search                                                         模板搜索
  dws aitable section    [create|rename|delete|reorder|list-empty|list-nodes|move-node]  文件夹与节点管理`,
		RunE:                       groupRunE,
		SuggestionsMinimumDistance: 2, // Enable "Did you mean ...?" for typos
	}

	// ── base: Base 管理 ─────────────────────────────────────────

	baseCmd := &cobra.Command{Use: "base", Short: "Base 管理", RunE: groupRunE}

	baseGetPrimaryDocIdCmd := &cobra.Command{
		Use:   "get-primary-doc-id",
		Short: "获取主键文档ID",
		Long: `根据 baseId、tableId 和 recordId 获取主键文档对应的文档信息。
当 AI 表格使用文档类型作为主键字段时，可以通过此命令获取对应文档的 dentryUuid，
进而利用该 uuid 去获取文档的内容以及做其他操作。`,
		Example: `  dws aitable base get-primary-doc-id --base-id BASE_ID --table-id TABLE_ID --record-id RECORD_ID
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable base get --base-id <baseId>
  # 查询 recordId: dws aitable record query --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "base-id", "table-id", "record-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("get_base_primary_doc_id", map[string]any{
				"baseId":   baseID,
				"tableId":  mustGetFlag(cmd, "table-id"),
				"recordId": mustGetFlag(cmd, "record-id"),
			})
		},
	}

	baseListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取 AI 表格列表",
		Long: `列出当前用户可访问的 AI 表格 Base。默认返回最近访问结果，支持分页游标续取。返回 baseId 与 baseName，后续可直接用于 base get。
AI 表格访问地址可按 baseId 拼接为：https://alidocs.dingtalk.com/i/nodes/{baseId}

注意: base list 仅返回最近访问过的 Base，不是全部 Base。
新创建或未在钉钉前端打开过的 Base 可能不会出现在此列表中。
如需按名称查找表格，请优先使用 base search 命令。`,
		Example: `  dws aitable base list
  dws aitable base list --limit 5 --cursor NEXT_CURSOR`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callAitableTool("list_bases", toolArgs)
		},
	}

	baseSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索 AI 表格",
		Long: `按名称关键词搜索 AI 表格 Base。返回 baseId/baseName，结果按相关性排序。返回的 baseId 可直接用于 base get 等后续工具。
AI 表格访问地址可按 baseId 拼接为：https://alidocs.dingtalk.com/i/nodes/{baseId}
不传关键词时返回最近访问的 Base 列表。`,
		Example: `  dws aitable base search --query "项目管理"
  dws aitable base search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := flagOrFallback(cmd, "query", "keyword")
			if query == "" {
				toolArgs := map[string]any{}
				if v, _ := cmd.Flags().GetString("cursor"); v != "" {
					toolArgs["cursor"] = v
				}
				return callAitableTool("list_bases", toolArgs)
			}
			toolArgs := map[string]any{
				"query": query,
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callAitableTool("search_bases", toolArgs)
		},
	}

	baseGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取 AI 表格信息",
		Long: `获取指定 Base 的资源目录级信息，返回 baseName、tables、dashboards 的 summary 信息（不含字段与记录详情）。
这是当前 Base 级目录入口：后续如需 tableId 或 dashboardId，优先从这里读取；table 详情再调用 table get，dashboard 详情再调用 get_dashboard`,
		Example: `  dws aitable base get --base-id BASE_ID  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("get_base", map[string]any{
				"baseId": baseID,
			})
		},
	}

	baseCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建 AI 表格",
		Long: `创建一个新的 AI 表格 Base。当前仅要求 baseName，服务端按默认模板创建并返回 baseId/baseName。
如果需要创建在特定的文件夹路径下，则需要传递 folderId，这个是知识库节点的 ID，调用文档的相关服务可以获取。
MCP 层会进一步兼容同字段传入的标准节点 URL，并在创建前解析出实际生效的节点 ID。`,
		Example: `  dws aitable base create --name "项目跟踪"
  dws aitable base create --name "项目跟踪" --folder-id FOLDER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseName": mustGetFlag(cmd, "name"),
			}
			if v, _ := cmd.Flags().GetString("folder-id"); v != "" {
				toolArgs["folderId"] = v
			}
			if v, _ := cmd.Flags().GetString("template-id"); v != "" {
				toolArgs["templateId"] = v
			}
			return callMCPTool("create_base", toolArgs)
		},
	}

	baseUpdateCmd := &cobra.Command{
		Use:     "update",
		Short:   "更新 AI 表格",
		Long:    `更新 Base 名称（可选备注）。当前不支持修改主题、封面等扩展属性`,
		Example: `  dws aitable base update --base-id BASE_ID --name "新名称"  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":      baseID,
				"newBaseName": mustGetFlag(cmd, "name"),
			}
			if v, _ := cmd.Flags().GetString("desc"); v != "" {
				toolArgs["description"] = v
			}
			return callAitableTool("update_base", toolArgs)
		},
	}

	baseDeleteCmd := &cobra.Command{
		Use:     "delete",
		Short:   "删除 AI 表格",
		Long:    `删除指定 Base（高风险、不可逆）。成功后应无法通过 base get / base search 读取到该 Base`,
		Example: `  dws aitable base delete --base-id BASE_ID --yes  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			if !confirmDelete("AI 表格 Base", baseID) {
				return nil
			}
			toolArgs := map[string]any{
				"baseId": baseID,
			}
			if v, _ := cmd.Flags().GetString("reason"); v != "" {
				toolArgs["reason"] = v
			}
			return callAitableTool("delete_base", toolArgs)
		},
	}

	baseCopyCmd := &cobra.Command{
		Use:   "copy",
		Short: "复制 AI 表格",
		Long: `复制 AI 表格到指定目录。支持完整复制或仅复制结构（不含数据）。
复制操作会创建一个新的 Base，包含原 Base 的表、字段、视图等配置。
如果选择仅复制结构（--only-struct），则不会复制实际的记录数据。

权限要求：需要对源 Base 有"阅读"权限，且对目标文件夹有"编辑"权限。

注意：--target-folder-id 参数如果传入的是文档/文件夹 URL（如 https://alidocs.dingtalk.com/i/nodes/xxx），
需要先调用文档的 dws 命令（如 dws doc info --node URL）获取 dentryUuid，再将 dentryUuid 传入本命令。
MCP 层不会会自动解析 URL，必须直接传入 dentryUuid 以避免报错。`,
		Example: `  dws aitable base copy --base-id BASE_ID --target-folder-id FOLDER_ID
  dws aitable base copy --base-id BASE_ID --target-folder-id FOLDER_ID --only-struct
  # 查询 baseId: dws aitable base list
  # 查询 folderId: dws doc list --folder PARENT_FOLDER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID := mustGetFlag(cmd, "base-id")
			targetFolderID := mustGetFlag(cmd, "target-folder-id")
			onlyCopyMeta, err := cmd.Flags().GetBool("only-struct")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":         baseID,
				"targetFolderId": targetFolderID,
				"onlyCopyMeta":   onlyCopyMeta,
			}
			return callAitableTool("copy_base", toolArgs)
		},
	}

	// ── table: 数据表管理 ───────────────────────────────────────

	tableCmd := &cobra.Command{Use: "table", Short: "数据表管理", RunE: groupRunE}

	tableGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取数据表",
		Long: `批量获取指定 Tables（数据表）的表级信息、字段目录与视图目录。
会返回 tables 列表；每个 table 直接包含 tableId、tableName、description、fields、views；
字段列表仅包含 fieldId、fieldName、type、description；views 仅包含 viewId、viewName、type。
若需读取字段的完整配置，请再调用 field get。`,
		Example: `  dws aitable table get --base-id BASE_ID
  dws aitable table get --base-id BASE_ID --table-ids tbl1,tbl2
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
			}
			if v, _ := cmd.Flags().GetString("table-ids"); v != "" {
				toolArgs["tableIds"] = parseCSVValues(v)
			}
			return callAitableTool("get_tables", toolArgs)
		},
	}

	tableCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建数据表",
		Long: `在指定 Base 中新建表格，并可在创建时附带初始化一批基础字段。
建表时单次最多附带 15 个字段；若 fields 为空，服务会自动补一个名为"标题"的 primaryDoc 首列。
若 tableName 与当前 Base 下已有表重名，服务会自动续号为"原名 1 / 原名 2 ..."，并在 summary 中返回当前表名。
如需添加更多字段，或在已有表中增加字段，请使用 create_fields。

--fields 参数说明：
建表时随附创建的初始字段列表，至少包含 1 个字段，单次最多 15 个。若传空数组 []，系统会自动补一个名为"标题"的 primaryDoc 首列。
建议在此处定义结构清晰的基础字段（如文本、数字、日期、单选等）；
复杂字段（关联、流转等）建议建表完成后通过 field create 单独添加。

每个字段对象包含：
  fieldName（必填）: 字段名称，最大 100 字，不支持换行
  type（必填）: 字段类型，可选值与 config 结构详见下方字段参考
  config（可选）: 字段配置，结构因 type 而异，详见下方参考

字段类型参考（括号内为 config 键，* 表示必填，无括号表示无需 config）：
  text: 文本 | number: 数字 (formatter) | singleSelect: 单选 (options*)
  multipleSelect: 多选 (options*) | date: 日期 (formatter)
  currency: 货币 (currencyType, formatter) | user: 人员 (multiple)
  department: 部门 (multiple) | group: 群组 (multiple)
  progress: 进度 (formatter, customizeRange, min, max) | rating: 评分 (min, max, icon)
  checkbox: 勾选 | attachment: 附件 | url: 链接 | richText: 富文本
  telephone: 电话 | email: 邮件 | idCard: 身份证 | barcode: 条码
  geolocation: 地理位置 | address: 行政区域 | primaryDoc: 文档 (仅限第一列) | formula: 公式
  filterUp: 查找引用 (targetSheet*, filters*, valuesField*, aggregator*) — 只读字段，不能通过 record create/update 写入
  lookup: 关联引用 (associateField*, valuesField*, aggregator*) — 只读字段，不能通过 record create/update 写入
  unidirectionalLink: 单向关联 (linkedTableId*, multiple)
  bidirectionalLink: 双向关联 (linkedTableId*, multiple)
  creator/lastModifier/createdTime/lastModifiedTime: 系统字段

config 结构参考：
  formatter（number）: INT|FLOAT_1|FLOAT_2|FLOAT_3|FLOAT_4|THOUSAND|THOUSAND_FLOAT|PERCENT|PERCENT_FLOAT
  formatter（date）: YYYY-MM-DD | YYYY-MM-DD HH:mm | YYYY-MM-DD HH:mm:ss | YYYY/MM/DD | YYYY/MM/DD HH:mm
  formatter（currency）: 可省略（默认 FLOAT_2）；若需指定小数位可用 INT|FLOAT_1|FLOAT_2|FLOAT_3|FLOAT_4
  formatter（progress）: 固定填 "PERCENT"
  currencyType（currency）: CNY|HKD|USD|EUR|GBP|MOP|VND|JPY|KRW|AED|AUD|BRL|CAD|CHF|INR|IDR|MXN|MYR|PHP|PLN|RUB|SGD|THB|TRY|TWD
  options（singleSelect/multipleSelect）: [{"name":"选项名"}, ...] — id 由系统生成，创建时只需传 name；更新时已有选项建议回传原 id
  multiple（user/department/group/unidirectionalLink/bidirectionalLink）: true（多选，默认）| false（单选）
  progress: {"formatter":"PERCENT"} | 自定义范围: {"formatter":"PERCENT","min":0,"max":1,"customizeRange":true}
  rating: {"min":1,"max":5,"icon":"star"} — max 范围 1~10
  formula: {"formula":"[单价] * [数量]"} — 使用 AI 表格公式字符串格式，方括号内填写表内字段名
  filterUp: {"targetSheet":"<目标表ID>","filters":[{"fieldId":"<目标表字段ID>","operator":"equal|contain","value":"常量值","currentSheetFieldId":"<本表字段ID>","link":"AND|OR"}],"valuesField":"<目标表字段ID>","aggregator":"SUM|AVERAGE|COUNT|MAX|MIN|CONCATENATE"}
    filters 说明：operator 仅支持 equal/contain；value 与 currentSheetFieldId 二选一（value=常量匹配，currentSheetFieldId=按本表字段动态匹配）；link 为多条件逻辑（AND/OR），单条件可省略，多条件必须统一
  lookup: {"associateField":"<关联字段ID>","valuesField":"<目标字段ID>","aggregator":"SUM|AVERAGE|COUNT|MAX|MIN|CONCATENATE"}
  unidirectionalLink/bidirectionalLink: {"linkedTableId":"<tableId>","multiple":true} — 反向关联端由系统自动创建，MCP 对外协议无需额外参数

已知边界：
  formula 字段在当前服务实例上创建会返回 not supported yet，调用前不要假设可用。
  关联字段创建对底层主键约束严格；即使已传 linkedTableId，也可能因下游返回"主键不存在不允许创建关联字段"而失败。

示例：
  [{"fieldName":"任务名称","type":"text"},
   {"fieldName":"优先级","type":"singleSelect","config":{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}},
   {"fieldName":"截止日期","type":"date","config":{"formatter":"YYYY-MM-DD"}},
   {"fieldName":"负责人","type":"user","config":{"multiple":false}}]`,
		Example: `  dws aitable table create --base-id BASE_ID --name "任务表" \
    --fields '[{"fieldName":"任务名称","type":"text"},{"fieldName":"优先级","type":"singleSelect","config":{"options":[{"name":"高"},{"name":"中"},{"name":"低"}]}}]'
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "base-id", "name"); err != nil {
				return err
			}
			fieldsStr := mustGetFlag(cmd, "fields")
			fields, err := parseFieldsJSON(fieldsStr)
			if err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callMCPTool("create_table", map[string]any{
				"baseId":    baseID,
				"tableName": flagOrFallback(cmd, "name", "table-name"),
				"fields":    fields,
			})
		},
	}

	tableUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新数据表",
		Long: `更新指定 Table（数据表）的名称、备注说明、行命名规则。
--name / --description / --record-name-key 至少传一项；三者可同时使用。

--record-name-key（行命名规则）取值是固定枚举键，不是字段 ID。常用值：
  ji_lu / project / task / event / question / customer / order /
  contact / item / issue / ticket / candidate / opportunity / meeting / member / okr 等
完整枚举集合较大，由服务端校验；传非法 key 会被拒。`,
		Example: `  dws aitable table update --base-id BASE_ID --table-id TABLE_ID --name "新表名"
  dws aitable table update --base-id BASE_ID --table-id TABLE_ID --description "项目阶段任务表"
  dws aitable table update --base-id BASE_ID --table-id TABLE_ID --record-name-key task
  dws aitable table update --base-id BASE_ID --table-id TABLE_ID --name "客户" --record-name-key customer
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			name, _ := cmd.Flags().GetString("name")
			desc, _ := cmd.Flags().GetString("description")
			rnKey, _ := cmd.Flags().GetString("record-name-key")
			if name == "" && desc == "" && rnKey == "" {
				return fmt.Errorf("--name / --description / --record-name-key 至少指定一项")
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
			}
			if name != "" {
				toolArgs["newTableName"] = name
			}
			if desc != "" {
				toolArgs["description"] = desc
			}
			if rnKey != "" {
				toolArgs["recordNameKey"] = rnKey
			}
			return callAitableTool("update_table", toolArgs)
		},
	}

	tableDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除数据表",
		Long: `删除指定 tableId 的数据表（不可逆，数据将永久丢失），该操作为高风险写入。
调用前请先通过 get_base / get_tables 确认目标表 ID 与名称。
若该表为 Base 中最后一张表，删除操作会失败（报错 "cannot delete the last sheet"）。
此时可改为调用 base delete 删除整个 Base，或先创建一张新表再删除目标表。`,
		Example: `  dws aitable table delete --base-id BASE_ID --table-id TABLE_ID --yes
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			tableID := mustGetFlag(cmd, "table-id")
			if !confirmDelete("数据表", tableID) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if v, _ := cmd.Flags().GetString("reason"); v != "" {
				toolArgs["reason"] = v
			}
			return callAitableTool("delete_table", toolArgs)
		},
	}

	// ── field: 字段管理 ─────────────────────────────────────────

	fieldCmd := &cobra.Command{Use: "field", Short: "字段管理", RunE: groupRunE}

	fieldGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取字段详情",
		Long: `批量获取指定字段的详细信息，包括 fieldId、名称、类型、description 以及类型相关完整配置（如格式化、选项等）。
传 fieldIds 时单次最多获取 10 个字段；若需更多字段，请拆分多次调用。
适用于在 table get 拿到字段目录后，按需展开少量字段的完整配置，避免大 options 字段放大 table get 返回值。

必填参数：
--table-id (必填)：表格 ID，必须先通过 dws aitable table get --base-id <baseId> 获取 tableId
--base-id (必填)：Base ID，可通过 dws aitable base list 或 dws aitable base search 获取`,
		Example: `  dws aitable field get --base-id BASE_ID --table-id TABLE_ID
  dws aitable field get --base-id BASE_ID --table-id TABLE_ID --field-ids fld1,fld2
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
			}
			if v, _ := cmd.Flags().GetString("field-ids"); v != "" {
				toolArgs["fieldIds"] = parseCSVValues(v)
			}
			return callAitableTool("get_fields", toolArgs)
		},
	}

	fieldCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建字段",
		Long: `在已有表格中批量新增字段。适用于建表后补充一批字段，或一次性添加多个关联、流转等复杂类型字段。
单次最多创建 15 个字段；若超过该数量，请拆分多次调用。
允许部分成功，返回结果会逐项说明每个字段是否创建成功；失败项会返回 reason 说明失败原因。
系统会按数组顺序依次创建，返回结果顺序与入参保持一致，并逐项标明成功/失败状态。

每个字段对象包含：
  fieldName（必填）: 字段名称，最大 100 字，不支持换行
  type（必填）: 字段类型（参考见 table create --help），新增类型包括 address（行政区域）、filterUp（查找引用）、lookup（关联引用）
  config（可选）: 字段配置（参考见 table create --help）
  aiConfig（可选）: AI 字段配置，传入后表示创建 AI 字段。
    - outputType（必填）: text|select|multiSelect|number|currency|image|video
    - prompt（必填）: [{"type":"text","value":"..."}, {"type":"fieldRef","fieldId":"..."}]
    - imageConfig（可选）: outputType=image 时可用，配置 resolution 和 aiGeneratedWatermark
    - videoConfig（可选）: outputType=video 时可用，配置 aspectRatio、resolution、duration
    - autoRecompute（可选）: 引用字段变化后是否自动重算
    - enableThinking（可选）: 是否启用深度思考
    - enableWebSearch（可选）: 是否启用联网搜索`,
		Example: `  dws aitable field create --base-id BASE_ID --table-id TABLE_ID \
    --fields '[{"fieldName":"状态","type":"singleSelect","config":{"options":[{"name":"待办"},{"name":"进行中"},{"name":"已完成"}]}}]'
  dws aitable field create --base-id BASE_ID --table-id TABLE_ID --name "优先级" --type singleSelect --config '{"options":[{"name":"高"},{"name":"低"}]}'
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}

			var fields []any
			fieldsStr, _ := cmd.Flags().GetString("fields")
			name, _ := cmd.Flags().GetString("name")
			typ, _ := cmd.Flags().GetString("type")

			// Resolve aliases: --field-name → --name, --field-type → --type
			if name == "" {
				if alias, _ := cmd.Flags().GetString("field-name"); alias != "" {
					name = alias
				}
			}
			if typ == "" {
				if alias, _ := cmd.Flags().GetString("field-type"); alias != "" {
					typ = alias
				}
			}

			if fieldsStr != "" {
				var err error
				fields, err = parseFieldsJSON(fieldsStr)
				if err != nil {
					return err
				}
			} else if name != "" && typ != "" {
				field := map[string]any{
					"fieldName": name,
					"type":      typ,
				}
				cfg, _ := cmd.Flags().GetString("config")
				optionsStr, _ := cmd.Flags().GetString("options")
				if cfg != "" {
					configMap, ok := jsonStringToMap(cfg).(map[string]any)
					if ok && optionsStr != "" {
						var optionsArr []any
						if err := json.Unmarshal([]byte(optionsStr), &optionsArr); err != nil {
							return fmt.Errorf("--options JSON parse failed: %w\n  hint: options must be a JSON array like '[{\"name\":\"高\"},{\"name\":\"低\"}]'", err)
						}
						configMap["options"] = optionsArr
						field["config"] = configMap
					} else {
						field["config"] = jsonStringToMap(cfg)
					}
				} else if optionsStr != "" {
					var optionsArr []any
					if err := json.Unmarshal([]byte(optionsStr), &optionsArr); err != nil {
						return fmt.Errorf("--options JSON parse failed: %w\n  hint: options must be a JSON array like '[{\"name\":\"高\"},{\"name\":\"低\"}]'", err)
					}
					field["config"] = map[string]any{"options": optionsArr}
				}
				if aiCfg, _ := cmd.Flags().GetString("ai-config"); aiCfg != "" {
					field["aiConfig"] = jsonStringToMap(aiCfg)
				}
				fields = []any{field}
			} else {
				return fmt.Errorf("must specify either --fields OR both --name and --type")
			}

			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callMCPTool("create_fields", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"fields":  fields,
			})
		},
	}

	fieldUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新字段",
		Long: `更新指定字段的名称、字段配置或 AI 配置。不可变更字段类型（type 不可修改）。
newFieldName、config、aiConfig 至少传入一项。

注意：更新 singleSelect/multipleSelect 的 options 时，需传入完整列表（含已有选项），
系统以新列表整体覆盖，不是追加。为避免已有单元格因 option id 变化而丢数据，
更新时已有选项应尽量回传原 id；新增选项无需传 id。
如果请求中传入的 option id 在当前字段配置中不存在，系统会丢弃该 id，并按新增选项处理；
若 id 合法但 name 改了，属于正常更新，会保留该 id。`,
		Example: `  dws aitable field update --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --name "新字段名"
  dws aitable field update --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --config '{"options":[{"name":"A"},{"name":"B"}]}'
  dws aitable field update --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --ai-config '{"outputType":"text","prompt":[{"type":"text","value":"请总结"}]}'
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>
  # 查询 fieldId: dws aitable field get --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "field-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"fieldId": mustGetFlag(cmd, "field-id"),
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["newFieldName"] = v
			}
			if v, _ := cmd.Flags().GetString("config"); v != "" {
				toolArgs["config"] = jsonStringToMap(v)
			}
			if v, _ := cmd.Flags().GetString("ai-config"); v != "" {
				toolArgs["aiConfig"] = jsonStringToMap(v)
			}
			if _, ok := toolArgs["newFieldName"]; !ok {
				if _, ok := toolArgs["config"]; !ok {
					if _, ok := toolArgs["aiConfig"]; !ok {
						return fmt.Errorf("must specify at least one of --name, --config, --ai-config")
					}
				}
			}
			return callAitableTool("update_field", toolArgs)
		},
	}

	fieldSearchOptionsCmd := &cobra.Command{
		Use:   "search-options",
		Short: "搜索单选/多选字段的选项",
		Long: `搜索指定单选/多选字段的 options 列表，支持按关键词模糊匹配（大小写不敏感、contains 模式）。
目标字段必须是 singleSelect 或 multipleSelect 类型，否则返回错误。
不传 --keyword 时返回该字段全部 options。
适用于 options 较多时按关键词缩小范围，避免通过 field get 拉取完整字段配置。
若需读取字段的完整配置（含 options 之外的属性），请使用 field get。`,
		Example: `  dws aitable field search-options --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID
  dws aitable field search-options --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --keyword 已完成
  dws aitable field search-options --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --limit 100
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>
  # 查询 fieldId: dws aitable field get --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "field-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"fieldId": mustGetFlag(cmd, "field-id"),
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			return callMCPTool("search_field_options", toolArgs)
		},
	}

	fieldDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除字段",
		Long: `删除指定 Table 中的一个字段（Field），删除操作不可逆。禁止删除主字段，且禁止删除最后一个字段

此操作不可逆，会永久删除字段及其所有数据。
必须提供准确的 baseId、tableId 和 fieldId，不得使用名称代替 ID。
若字段不存在或无权限，将返回错误。`,
		Example: `  dws aitable field delete --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --yes
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>
  # 查询 fieldId: dws aitable field get --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "field-id"); err != nil {
				return err
			}
			fieldID := mustGetFlag(cmd, "field-id")
			if !confirmDelete("字段", fieldID) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("delete_field", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"fieldId": fieldID,
			})
		},
	}

	// ── record: 记录管理 ────────────────────────────────────────

	recordCmd := &cobra.Command{Use: "record", Short: "记录管理", RunE: groupRunE}

	recordQueryCmd := &cobra.Command{
		Use:   "query",
		Short: "获取行记录",
		Long: `查询指定表格中的记录，支持两种模式：
- 按 ID 取：传入 --record-ids（单次最多 100 个），直接获取指定记录。忽略 filters 和 sort。
- 条件查：通过 --filters 过滤、--sort 排序、--cursor 分页遍历全表。
两种模式均可通过 --field-ids（单次最多 100 个）限制返回字段以节省 token。

重要说明（公式、引用、关联字段）：
默认情况下，公式字段（formula）、引用字段（lookup）、关联字段（association）不会返回数据。
如需返回这些字段的值，必须在 --field-ids 参数中显式指定这些字段的列 ID。
例如：--field-ids "fldTextId,fldFormulaId,fldLookupId,fldAssocId"

--filters 结构（最外层必须是并列关系 and 或 or）：
{"operator":"and","operands":[{"operator":"<op>","operands":["<fieldId>","<value>"]}]}
  操作符：eq(等于) ne(不等于) exist(有值) un_exist(为空)
          lt gt lte gte(数值比较) contain exclusive(文本)
          all_of any_of none_of(多选)
          date_eq before after not_before not_after(日期)
  注意：singleSelect/multipleSelect 字段过滤值 必须 传选项名称的字面量（比如 "本科"），不要传 option ID！
  注意：date 日期字段 只能用 date_eq/before/after/not_before/not_after，值传日期串(如 "2026-05-22")；
        通用 eq/gte/lte/contain 对日期字段无效会返回 0 条；不支持区间(date_between)和相对(from_now)，
        范围用 not_before+not_after 组合。

--sort 结构：[{"fieldId":"<fieldId>","direction":"asc|desc"}]
  示例：[{"fieldId":"fldPriorityId","direction":"asc"},{"fieldId":"fldDueDateId","direction":"desc"}]
  注意：排序方向字段必须使用 direction（值为 asc 或 desc）`,
		Example: `  dws aitable record query --base-id BASE_ID --table-id TABLE_ID
  dws aitable record query --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2
  dws aitable record query --base-id BASE_ID --table-id TABLE_ID --filters '{"operator":"and","operands":[{"operator":"eq","operands":["fld_xxx","本科"]}]}'
  dws aitable record query --base-id BASE_ID --table-id TABLE_ID --query "关键词" --limit 50
  dws aitable record query --base-id BASE_ID --table-id TABLE_ID --field-ids "fldTextId,fldFormulaId,fldLookupId"
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			tableID := mustGetFlag(cmd, "table-id")
			if tableID == "" {
				return fmt.Errorf("--table-id must not be empty\n  hint: first run: dws aitable table get --base-id <baseId> to find the tableId")
			}
			if v, _ := cmd.Flags().GetString("view-id"); v != "" {
				fmt.Fprintf(os.Stderr, "warning: --view-id is not supported by record query; records are queried by table, not by view. The --view-id flag has been ignored.\n")
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			}
			if v, _ := cmd.Flags().GetString("record-ids"); v != "" {
				toolArgs["recordIds"] = parseCSVValues(v)
			}
			if v, _ := cmd.Flags().GetString("field-ids"); v != "" {
				toolArgs["fieldIds"] = parseCSVValues(v)
			}
			if v, _ := cmd.Flags().GetString("filters"); v != "" {
				parsed := jsonStringToMap(v)
				if err := validateFiltersStructure(parsed, v); err != nil {
					return err
				}
				toolArgs["filters"] = normalizeFilters(parsed)
			}
			if v, _ := cmd.Flags().GetString("sort"); v != "" {
				var sortArr []map[string]any
				if err := json.Unmarshal([]byte(v), &sortArr); err == nil {
					// 兼容处理：用户传 "order" 时自动转为 "direction"
					for i := range sortArr {
						if val, ok := sortArr[i]["order"]; ok {
							if _, hasDirection := sortArr[i]["direction"]; !hasDirection {
								sortArr[i]["direction"] = val
								delete(sortArr[i], "order")
							}
						}
					}
					toolArgs["sort"] = sortArr
				}
			}
			if v, _ := cmd.Flags().GetString("query"); v != "" {
				toolArgs["keyword"] = v
			} else if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			} else if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
				// --page-size is a hidden alias for --limit
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}

			// --all: 自动翻页，合并所有页的 records 后统一输出
			fetchAll, _ := cmd.Flags().GetBool("all")
			if !fetchAll {
				return callAitableTool("query_records", toolArgs)
			}
			pageLimit, _ := cmd.Flags().GetInt("page-limit")
			if pageLimit == 0 {
				// 用户未显式传 --page-limit 时，检查是否有默认值
				if !cmd.Flags().Changed("page-limit") {
					pageLimit = 50 // 默认最多 50 页
				}
				// 显式传 --page-limit 0 表示无限制，保持 0
			}
			return recordQueryFetchAll(toolArgs, pageLimit)
		},
	}

	recordCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "新增记录",
		Long: `在指定表格中批量新增记录。

records 为待创建的记录列表 JSON 数组，单次最多 100 条。
每条记录包含 cells 字段，key 为 fieldId（通过 table get 获取），value 为写入值。

注意：filterUp（查找引用）和 lookup（关联引用）字段为只读字段，不能通过 record create/update 写入值。

各类型写入格式：
  text → "文本内容"
  number → 123 或 123.45（也接受字符串 "123"）
  singleSelect → "选项名称"，或 {"id":"opt_xxx","name":"进行中"}；id 为准
  multipleSelect → ["选项名1","选项名2"]，或 [{"id":"opt_a","name":"标签A"},...]
  date → "2026-03-15" 或 "2026-03-15T09:00+08:00"（RFC3339）；亦支持毫秒时间戳
  checkbox → true | false
  user → [{"userId":"staff_001","corpId":"dingxxxxxxxx"}]
  department → [{"deptId":"52528700"}]
  group → [{"cid":"74577067501"}] — 注意 key 是 cid，不是 openConversationId
  url → {"text":"显示文字","link":"https://..."}；不能直接传 URL 字符串
  richText → {"markdown":"**加粗**\n普通文字\n"}
  telephone/email/barcode/idCard → 字符串
  attachment → 推荐通过 prepare_attachment_upload + append_uploaded_file_to_record 写入
  unidirectionalLink/bidirectionalLink → {"linkedRecordIds":["recXXX","recYYY"]}
  creator/lastModifier/createdTime/lastModifiedTime → 系统自动回填，不建议手动写入

Windows 用户注意：如果 --records JSON 很长（超过命令行长度限制），请使用 --records-file 参数指定一个 JSON 文件路径，
CLI 会自动从文件中读取内容作为 --records 的值。这样可以避免 shell 引号转义和命令行截断问题。`,
		Example: `  dws aitable record create --base-id BASE_ID --table-id TABLE_ID --records '[{"cells":{"fldTextId":"文本内容","fldNumId":123}}]'
  dws aitable record create --base-id BASE_ID --table-id TABLE_ID --records-file ./data/records.json
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			// Hidden shortcut: --cells → auto-construct --records '[{"cells":...}]'
			if singleCells, _ := cmd.Flags().GetString("cells"); singleCells != "" {
				recs, _ := cmd.Flags().GetString("records")
				recsFile, _ := cmd.Flags().GetString("records-file")
				fieldsAlias, _ := cmd.Flags().GetString("fields")
				if recs == "" && recsFile == "" && fieldsAlias == "" {
					var cellsObj map[string]any
					if err := json.Unmarshal([]byte(singleCells), &cellsObj); err != nil {
						return fmt.Errorf("--cells JSON parse failed: %w\n  hint: cells must be a JSON object like '{\"fldId\":\"value\"}'", err)
					}
					baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
					if err != nil {
						return err
					}
					return callMCPTool("create_records", map[string]any{
						"baseId":  baseID,
						"tableId": mustGetFlag(cmd, "table-id"),
						"records": []any{map[string]any{"cells": cellsObj}},
					})
				}
			}
			recordsStr, err := resolveRecordsFlag(cmd)
			if err != nil {
				return err
			}
			var records []any
			if err := json.Unmarshal([]byte(recordsStr), &records); err != nil {
				return fmt.Errorf("--records JSON parse failed: %w\n  hint: records must be a JSON array like '[{\"cells\":{\"fldId\":\"value\"}}]'", err)
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callMCPTool("create_records", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"records": records,
			})
		},
	}

	recordUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新记录",
		Long: `批量更新指定记录的字段值，只需传入需修改的字段，未传入的字段保持原值。

records 为待更新的记录内容列表 JSON 数组，单次最多 100 条。
每条记录必须包含 recordId 和 cells 字段。cells 的写入格式与 record create 相同（详见 record create --help）。

Windows 用户注意：如果 --records JSON 很长，请使用 --records-file 参数指定一个 JSON 文件路径。`,
		Example: `  dws aitable record update --base-id BASE_ID --table-id TABLE_ID --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]'
  dws aitable record update --base-id BASE_ID --table-id TABLE_ID --records-file ./data/updates.json
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			// Hidden shortcut: --record-id + --cells → auto-construct --records
			singleRecID, _ := cmd.Flags().GetString("record-id")
			singleCells, _ := cmd.Flags().GetString("cells")
			if singleRecID != "" || singleCells != "" {
				if singleRecID == "" || singleCells == "" {
					return fmt.Errorf("--record-id and --cells must be used together\n  hint: dws aitable record update --base-id BASE --table-id TBL --record-id recXXX --cells '{\"fldId\":\"value\"}'")
				}
				var cellsObj map[string]any
				if err := json.Unmarshal([]byte(singleCells), &cellsObj); err != nil {
					return fmt.Errorf("--cells JSON parse failed: %w\n  hint: cells must be a JSON object like '{\"fldId\":\"value\"}'", err)
				}
				baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
				if err != nil {
					return err
				}
				return callAitableTool("update_records", map[string]any{
					"baseId":  baseID,
					"tableId": mustGetFlag(cmd, "table-id"),
					"records": []any{map[string]any{"recordId": singleRecID, "cells": cellsObj}},
				})
			}
			recordsStr, err := resolveRecordsFlag(cmd)
			if err != nil {
				return err
			}
			var records []any
			if err := json.Unmarshal([]byte(recordsStr), &records); err != nil {
				return fmt.Errorf("--records JSON parse failed: %w\n  hint: records must be a JSON array like '[{\"recordId\":\"recXXX\",\"cells\":{\"fldId\":\"value\"}}]'", err)
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("update_records", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"records": records,
			})
		},
	}

	recordDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除行记录",
		Long: `在指定 Table 中批量删除记录（不可逆，数据将永久丢失）。
单次最多删除 100 条；超出请拆分多次调用。
调用前建议先通过 record query 确认目标记录 ID 与内容，避免误删。`,
		Example: `  dws aitable record delete --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2 --yes
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "record-ids"); err != nil {
				return err
			}
			recordIDs := mustGetFlag(cmd, "record-ids")
			if !confirmDelete("记录", recordIDs) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("delete_records", map[string]any{
				"baseId":    baseID,
				"tableId":   mustGetFlag(cmd, "table-id"),
				"recordIds": parseCSVValues(recordIDs),
			})
		},
	}

	recordBatchUpdateCmd := &cobra.Command{
		Use:   "batch-update",
		Short: "批量更新记录（同一 cells 应用到多条 recordId）",
		Long: `将同一份 cells JSON 批量应用到多条记录上。适合"统一标记完成 / 统一改负责人 / 统一清空某字段"等场景。

与 record update 的区别：
  - record update         每条记录可有不同 cells，参数是 records JSON 数组
  - record batch-update   所有记录共享同一份 cells（CLI 客户端展开后调用相同 MCP 工具）

单次最多 100 条；超出请拆分多次调用。
cells 写入格式与 record update / record create 相同（详见 record create --help）。
只读字段（formula / filterUp / lookup / 系统字段）不能写入。

CLI 行为：客户端把 --record-ids 拆开后构造 [{recordId, cells}, ...] 调用 update_records；
当前服务端尚未提供 broadcast patch 工具，所以本命令是纯语义包装，不省网络体积，仅省 LLM 构造 prompt 的 token。`,
		Example: `  dws aitable record batch-update --base-id BASE_ID --table-id TABLE_ID \
    --record-ids rec1,rec2,rec3 --cells '{"fldStatusId":"已完成"}'
  dws aitable record batch-update --base-id BASE_ID --table-id TABLE_ID \
    --record-ids rec1,rec2 --cells '{"fldOwnerId":[{"userId":"staff_001"}]}'
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "record-ids", "cells"); err != nil {
				return err
			}
			recordIDs := parseCSVValues(mustGetFlag(cmd, "record-ids"))
			if len(recordIDs) == 0 {
				return fmt.Errorf("--record-ids must not be empty\n  hint: provide comma-separated recordIds")
			}
			if len(recordIDs) > 100 {
				return fmt.Errorf("--record-ids exceeds limit: got %d, max 100\n  hint: split into multiple batch-update calls", len(recordIDs))
			}
			cellsRaw := mustGetFlag(cmd, "cells")
			var cells map[string]any
			if err := json.Unmarshal([]byte(cellsRaw), &cells); err != nil {
				return fmt.Errorf("--cells JSON parse failed: %w\n  hint: --cells must be a JSON object like '{\"fldXXX\":\"value\"}'", err)
			}
			if len(cells) == 0 {
				return fmt.Errorf("--cells must not be empty\n  hint: provide at least one fieldId → value mapping")
			}
			records := make([]any, 0, len(recordIDs))
			for _, rid := range recordIDs {
				records = append(records, map[string]any{
					"recordId": rid,
					"cells":    cells,
				})
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("update_records", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"records": records,
			})
		},
	}

	// record query-empty：查询完全没填用户字段的空行
	recordQueryEmptyCmd := &cobra.Command{
		Use:   "query-empty",
		Short: "查询完全没填用户字段的空行",
		Long: `按表内顺序扫描一页，过滤出"完全没填用户字段"的空行。
- 空行定义：除系统字段（recordId / 创建人 / 创建时间 / 修改人 / 修改时间）外，所有 cell 都是 null、空字符串、空集合或空 Map。
- --limit 是扫描预算（不是返回数）：可能扫了 100 条但全部非空，本页返回空数组。
- 翻页：返回 nextCursor 非空时把它传回继续扫；nextCursor 为空才表示扫完全表。

返回 data: {records: [...], nextCursor: "..."}。`,
		Example: `  dws aitable record query-empty --base-id BASE_ID --table-id TABLE_ID
  dws aitable record query-empty --base-id BASE_ID --table-id TABLE_ID --limit 50
  # 翻页：第二页传上次返回的 nextCursor
  dws aitable record query-empty --base-id BASE_ID --table-id TABLE_ID --cursor <上次的nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
			}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				if limit < 1 || limit > 100 {
					return fmt.Errorf("--limit 范围 [1, 100]，got %d", limit)
				}
				toolArgs["limit"] = limit
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callAitableHelperTool("query_empty_records", toolArgs)
		},
	}

	// record history-list：按 recordId 查变更历史
	recordHistoryListCmd := &cobra.Command{
		Use:   "history-list",
		Short: "查询行记录变更历史",
		Long: `按 recordId 查询单条记录的变更历史。
返回字段（每条 history item）：
- type              变更类型（如 field_change / record_create）
- action            操作动作（create / update / delete）
- newValue          变更后的值（JSON 字符串）
- oldValue          变更前的值（JSON 字符串）
- operateTime       操作时间（毫秒级时间戳）
- typeChangedFields 类型变更的字段信息（JSON 字符串）
- version           版本号

分页用 --offset / --limit；--limit 范围 [1, 50]，默认 20。`,
		Example: `  dws aitable record history-list --base-id BASE_ID --table-id TABLE_ID --record-id REC_ID
  dws aitable record history-list --base-id BASE_ID --table-id TABLE_ID --record-id REC_ID --limit 50 --offset 0
  # 查询 recordId: dws aitable record query --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "record-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":   baseID,
				"tableId":  mustGetFlag(cmd, "table-id"),
				"recordId": mustGetFlag(cmd, "record-id"),
			}
			if cmd.Flags().Changed("offset") {
				offset, _ := cmd.Flags().GetInt("offset")
				if offset < 0 {
					return fmt.Errorf("--offset 必须 >= 0，got %d", offset)
				}
				toolArgs["offset"] = offset
			}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				if limit < 1 || limit > 50 {
					return fmt.Errorf("--limit 范围 [1, 50]，got %d", limit)
				}
				toolArgs["limit"] = limit
			}
			return callAitableHelperTool("query_record_history", toolArgs)
		},
	}

	// record share-url：批量获取 record 分享链接
	recordShareUrlCmd := &cobra.Command{
		Use:   "share-url",
		Short: "批量获取记录分享链接",
		Long: `按 recordId 批量获取记录分享链接。
单次最多 20 条；可选 --view-id 生成带视图上下文的链接。

返回 data.items 数组，每项 {recordId, shareUrl}；shareUrl 为 null 表示该条获取失败。`,
		Example: `  dws aitable record share-url --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2,rec3
  dws aitable record share-url --base-id BASE_ID --table-id TABLE_ID --record-ids rec1 --view-id viw_VIP
  # 查询 recordId: dws aitable record query --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "record-ids"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			recordIDs := parseCSVValues(mustGetFlag(cmd, "record-ids"))
			if len(recordIDs) == 0 {
				return fmt.Errorf("--record-ids 必须至少包含 1 个 ID")
			}
			if len(recordIDs) > 20 {
				return fmt.Errorf("--record-ids 单次最多 20 条，got %d", len(recordIDs))
			}
			toolArgs := map[string]any{
				"baseId":    baseID,
				"tableId":   mustGetFlag(cmd, "table-id"),
				"recordIds": recordIDs,
			}
			if v, _ := cmd.Flags().GetString("view-id"); v != "" {
				toolArgs["viewId"] = v
			}
			return callAitableHelperTool("get_record_share_url", toolArgs)
		},
	}

	// record upsert：按 recordId 是否存在自动拆分 create / update
	recordUpsertCmd := &cobra.Command{
		Use:   "upsert",
		Short: "批量创建或更新记录（Upsert）",
		Long: `按 records 列表自动判断是创建还是更新：
- 带 recordId 的项 → 走 update（按 recordId 局部更新 cells）
- 不带 recordId 的项 → 走 create（生成新 recordId）

records JSON 结构与 record update 完全一致：[{"recordId": "<可选>", "cells": {...}}, ...]
单次最多 100 条（创建 + 更新合计）。
cells 写入格式见 record create --help（key 是 fieldId，value 按字段类型）。

返回 data: {createdRecordIds: [...], updatedRecordIds: [...]}，分别按链路汇总。

Windows 用户注意：如果 --records JSON 很长，请使用 --records-file 参数指定一个 JSON 文件路径。`,
		Example: `  # 混合：第 1 条更新（带 recordId），第 2 条创建（不带）
  dws aitable record upsert --base-id BASE_ID --table-id TABLE_ID --records '[
    {"recordId":"rec1","cells":{"fldStatusId":"已完成"}},
    {"cells":{"fldTitleId":"新任务","fldStatusId":"待办"}}
  ]'
  dws aitable record upsert --base-id BASE_ID --table-id TABLE_ID --records-file ./batch.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			recordsStr, err := resolveRecordsFlag(cmd)
			if err != nil {
				return err
			}
			var records []any
			if err := json.Unmarshal([]byte(recordsStr), &records); err != nil {
				return fmt.Errorf("--records JSON parse failed: %w\n  hint: records must be a JSON array like '[{\"recordId\":\"rec1\",\"cells\":{\"fldId\":\"value\"}},{\"cells\":{\"fldId\":\"new\"}}]'", err)
			}
			if len(records) == 0 {
				return fmt.Errorf("--records 必须至少包含 1 条记录")
			}
			if len(records) > 100 {
				return fmt.Errorf("--records 单次最多 100 条，got %d", len(records))
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("record_upsert", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"records": records,
			})
		},
	}

	// record primary-doc-get：查询主键文档
	recordPrimaryDocGetCmd := &cobra.Command{
		Use:   "primary-doc-get",
		Short: "查询记录的主键文档",
		Long: `查询指定记录关联的主键文档 nodeId。
返回的 nodeId 可直接用于 dws doc read/update 的 --node 参数来读写文档内容。
若该记录尚未创建主键文档，nodeId 为 null。`,
		Example: `  dws aitable record primary-doc-get --base-id BASE_ID --table-id TABLE_ID --record-id RECORD_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "record-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_primary_doc", map[string]any{
				"baseId":   baseID,
				"tableId":  mustGetFlag(cmd, "table-id"),
				"recordId": mustGetFlag(cmd, "record-id"),
			})
		},
	}

	// record primary-doc-create：创建主键文档
	recordPrimaryDocCreateCmd := &cobra.Command{
		Use:   "primary-doc-create",
		Short: "为记录创建主键文档",
		Long: `为指定记录创建主键文档（幂等：已存在则返回已有文档的 nodeId）。
返回的 nodeId 可直接用于 dws doc read/update 的 --node 参数来读写文档内容。
fieldId 必须是 primaryDoc 类型的字段。`,
		Example: `  dws aitable record primary-doc-create --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --record-id RECORD_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "field-id", "record-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("create_primary_doc", map[string]any{
				"baseId":   baseID,
				"tableId":  mustGetFlag(cmd, "table-id"),
				"fieldId":  mustGetFlag(cmd, "field-id"),
				"recordId": mustGetFlag(cmd, "record-id"),
			})
		},
	}

	// ── template: 模板搜索 ──────────────────────────────────────

	templateCmd := &cobra.Command{Use: "template", Short: "模板搜索", RunE: groupRunE}

	templateSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索模板",
		Long: `按名称关键词搜索 AI 表格模板，支持分页。
返回每个模板的 templateId、name、description，以及分页信息 hasMore / nextCursor。
返回的 templateId 可直接用于 base create。
模板预览链接可通过 https://docs.dingtalk.com/table/template/{templateId} 拼接得到
不传关键词时返回热门模板。`,
		Example: `  dws aitable template search --query "项目管理"
  dws aitable template search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if q := flagOrFallback(cmd, "query", "keyword"); q != "" {
				toolArgs["query"] = q
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callAitableTool("search_templates", toolArgs)
		},
	}

	// ── attachment: 附件管理 ──────────────────────────────────────

	attachmentCmd := &cobra.Command{Use: "attachment", Short: "附件管理", RunE: groupRunE}

	attachmentUploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "准备附件上传",
		Long: `为 attachment 字段文件申请带容量校验的 OSS 直传地址。

该命令只负责准备上传（获取 uploadUrl 和 fileToken），不直接上传文件。
实际文件需由客户端在 MCP 外通过 PUT 请求上传到返回的 uploadUrl。

完整流程:
  1. dws aitable attachment upload --base-id <BASE_ID> --file-name photo.png --size 1024
     → 获取 uploadUrl 和 fileToken
  2. curl -X PUT "<uploadUrl>" -H "Content-Type: image/png" --data-binary @photo.png
     → 上传文件到 OSS
  3. dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
       --records '[{"cells":{"fldAttachId":[{"fileToken":"<fileToken>"}]}}]'
     → 将 fileToken 写入 attachment 字段

注意:
  上传文件到 uploadUrl 时，PUT 请求必须携带 Content-Type header，
  且其值必须是该文件的具体 MIME type（如 image/png、application/pdf）。`,
		Example: `  dws aitable attachment upload --base-id BASE_ID --file-name report.xlsx --size 204800
  dws aitable attachment upload --base-id BASE_ID --file-name photo.png --size 1024 --mime-type image/png
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 手动校验必填参数(避免 validateRequiredFlags 对 Int64 的误判)
			if !cmd.Flags().Changed("base-id") {
				return fmt.Errorf("missing required flag --base-id")
			}
			if !cmd.Flags().Changed("file-name") {
				return fmt.Errorf("missing required flag --file-name")
			}
			if !cmd.Flags().Changed("size") {
				return fmt.Errorf("missing required flag --size")
			}

			size, err := cmd.Flags().GetInt64("size")
			if err != nil {
				return fmt.Errorf("invalid --size value: %w", err)
			}

			toolArgs := map[string]any{
				"baseId":   mustGetFlag(cmd, "base-id"),
				"fileName": mustGetFlag(cmd, "file-name"),
				"size":     size,
			}
			if v, _ := cmd.Flags().GetString("mime-type"); v != "" {
				toolArgs["mimeType"] = v
			}
			return callAitableTool("prepare_attachment_upload", toolArgs)
		},
	}

	// ── view: 视图管理 ───────────────────────────────────────────

	viewCmd := &cobra.Command{Use: "view", Short: "视图管理", RunE: groupRunE}

	viewGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取视图详情",
		Long: `获取指定数据表（Table）中的视图（View）完整信息，包括列顺序、筛选、排序、分组、条件格式、自定义配置等。
支持两种模式：
- 显式选择：传入 --view-ids，按入参顺序返回这些视图；单次最多 10 个。
- 默认全量：省略 --view-ids，返回当前表下全部视图，顺序与当前表视图目录一致。`,
		Example: `  dws aitable view get --base-id BASE_ID --table-id TABLE_ID
  dws aitable view get --base-id BASE_ID --table-id TABLE_ID --view-ids view1,view2
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
			}
			if v, _ := cmd.Flags().GetString("view-ids"); v != "" {
				toolArgs["viewIds"] = parseCSVValues(v)
			}
			return callAitableTool("get_views", toolArgs)
		},
	}

	// ─── view get <attr> 子命令：按属性投影 view 响应 ──────────────
	//
	// 共用 makeViewGetSubCmd 工厂构造。card/timebar/aggregate 需要 viewType
	// 校验；filter/sort/group/visible-fields/field-widths 不需要。
	makeViewGetSubCmd := func(use, short, longExtra, blockKey string,
		viewTypeWhitelist []string,
		// 当 blockKey == "card" 时由 dispatchCardKey 动态决定取哪个字段（kanbanCard / galleryCard）
		dynamicDispatch bool,
	) *cobra.Command {
		long := fmt.Sprintf(`获取指定视图的 %s 配置。`, short)
		if longExtra != "" {
			long += "\n" + longExtra
		}
		return &cobra.Command{
			Use:     use,
			Short:   "获取视图 " + use + " 配置",
			Long:    long,
			Example: fmt.Sprintf("  dws aitable view get %s --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID", use),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
					return err
				}
				baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
				if err != nil {
					return err
				}
				view, viewType, err := getViewRaw(context.Background(), baseID, mustGetFlag(cmd, "table-id"), mustGetFlag(cmd, "view-id"))
				if err != nil {
					return err
				}
				key := blockKey
				if dynamicDispatch {
					key, err = dispatchCardKey(viewType)
					if err != nil {
						return err
					}
				} else if len(viewTypeWhitelist) > 0 {
					if err := requireViewType(viewType, use, viewTypeWhitelist); err != nil {
						return err
					}
				}
				// blockKey 支持 dotted path（如 "custom.widthMap"），供 read 路径
				// 投影到嵌套字段。服务端某些属性（fieldWidths）写入走 config.<key>，
				// 但 get_views 响应里落到 view.custom.<sub>，read/write 路径不对称。
				return printViewSubBlock(walkViewPath(view, key))
			},
		}
	}

	viewGetCardCmd := makeViewGetSubCmd("card",
		"card",
		"适用视图：Kanban / Gallery。返回对应视图的卡片配置子块（kanbanCard 或 galleryCard）。",
		"", nil, true)

	viewGetTimebarCmd := makeViewGetSubCmd("timebar",
		"timebar",
		"适用视图：Gantt。返回 ganttTimebar 子块（startField / endField / displayFieldId / timelineScale / colorConfigs / officialHoliday）。",
		"ganttTimebar", []string{"Gantt"}, false)

	viewGetAggregateCmd := makeViewGetSubCmd("aggregate",
		"aggregate",
		"适用视图：Grid。返回字段聚合统计配置（map[fieldId]→AggregateAction）。",
		"aggregate", []string{"Grid"}, false)

	viewGetFilterCmd := makeViewGetSubCmd("filter",
		"filter",
		"返回视图当前的筛选规则数组。所有视图类型都支持。",
		"filter", nil, false)

	viewGetSortCmd := makeViewGetSubCmd("sort",
		"sort",
		"返回视图当前的排序规则数组。所有视图类型都支持。",
		"sort", nil, false)

	viewGetGroupCmd := makeViewGetSubCmd("group",
		"group",
		"返回视图当前的分组规则数组。所有视图类型都支持。",
		"group", nil, false)

	viewGetVisibleFieldsCmd := makeViewGetSubCmd("visible-fields",
		"visible-fields",
		"返回视图当前可见字段 ID 列表（即 columns 字段，按显示顺序）。所有视图类型都支持。",
		"columns", nil, false)

	viewGetFieldWidthsCmd := makeViewGetSubCmd("field-widths",
		"field-widths",
		"适用视图：Grid。返回字段列宽映射（map[fieldId]→width）。注意服务端响应里落在 custom.widthMap，read 路径已自动投影到此。",
		"custom.widthMap", []string{"Grid"}, false)

	viewGetCmd.AddCommand(
		viewGetCardCmd,
		viewGetTimebarCmd,
		viewGetAggregateCmd,
		viewGetFilterCmd,
		viewGetSortCmd,
		viewGetGroupCmd,
		viewGetVisibleFieldsCmd,
		viewGetFieldWidthsCmd,
	)

	viewCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建视图",
		Long: `在指定数据表（Table）下创建一个新视图（View）。
当前稳定支持的 viewType：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery。
若未传 --name，则会按视图类型自动生成不重名名称。
首列字段是每条数据的索引，不支持删除、移动或隐藏。`,
		Example: `  dws aitable view create --base-id BASE_ID --table-id TABLE_ID --view-type Grid
  dws aitable view create --base-id BASE_ID --table-id TABLE_ID --view-type Kanban --name "看板视图"
  dws aitable view create --base-id BASE_ID --table-id TABLE_ID --view-type Grid --config '{"visibleFieldIds":["fld1","fld2"]}'
  # 查询 baseId: dws aitable base list
  # 查询 tableId: dws aitable table get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-type"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":   baseID,
				"tableId":  mustGetFlag(cmd, "table-id"),
				"viewType": mustGetFlag(cmd, "view-type"),
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["viewName"] = v
			}
			if v, _ := cmd.Flags().GetString("view-sub-type"); v != "" {
				toolArgs["viewSubType"] = v
			}
			if v, _ := cmd.Flags().GetString("desc"); v != "" {
				toolArgs["viewDescription"] = jsonStringToMap(v)
			}
			if v, _ := cmd.Flags().GetString("config"); v != "" {
				toolArgs["config"] = jsonStringToMap(v)
			}
			return callMCPTool("create_view", toolArgs)
		},
	}

	viewUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新视图",
		Long: `更新指定视图（View）的名称、描述或配置。
当前稳定支持更新：--name、--desc、--config（含 visibleFieldIds、filter、sort、group、fieldWidths）。
fieldWidths 仅支持 Grid 视图。
首列字段是每条数据的索引，不支持删除、移动或隐藏。`,
		Example: `  dws aitable view update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --name "新视图名"
  dws aitable view update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --config '{"visibleFieldIds":["fld1","fld2","fld3"]}'
  # 查询 viewId: dws aitable view get --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["newViewName"] = v
			}
			if v, _ := cmd.Flags().GetString("desc"); v != "" {
				toolArgs["viewDescription"] = jsonStringToMap(v)
			}
			if v, _ := cmd.Flags().GetString("config"); v != "" {
				cfg := jsonStringToMap(v)
				if cfgMap, ok := cfg.(map[string]any); ok {
					if err := normalizeViewConfigBlock(cfgMap); err != nil {
						return err
					}
					cfg = cfgMap
				}
				toolArgs["config"] = cfg
			}
			return callAitableTool("update_view", toolArgs)
		},
	}

	// ─── view update <attr> 子命令：按属性局部更新 ────────────────────

	// viewUpdateCommonPreflight 校验 base-id/table-id/view-id；对需要 viewType 校验
	// 的 attr，preflight 调 get_views 拿 viewType，并返回 dispatch 后的 blockKey。
	viewUpdateCommonPreflight := func(cmd *cobra.Command, attr string,
		viewTypeWhitelist []string, dynamicDispatch bool,
	) (baseID, tableID, viewID, blockKey string, err error) {
		if err = validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
			return
		}
		baseID, err = mustFlagOrFallback(cmd, "base-id", "base")
		if err != nil {
			return
		}
		tableID = mustGetFlag(cmd, "table-id")
		viewID = mustGetFlag(cmd, "view-id")
		blockKey = attr
		if dynamicDispatch || len(viewTypeWhitelist) > 0 {
			_, viewType, e := getViewRaw(context.Background(), baseID, tableID, viewID)
			if e != nil {
				err = e
				return
			}
			if dynamicDispatch {
				blockKey, err = dispatchCardKey(viewType)
				if err != nil {
					return
				}
			} else if err = requireViewType(viewType, attr, viewTypeWhitelist); err != nil {
				return
			}
		}
		return
	}

	// view update card: 同时支持 Kanban / Gallery，preflight 拿 viewType dispatch
	viewUpdateCardCmd := &cobra.Command{
		Use:   "card",
		Short: "更新视图 card 配置（Kanban / Gallery）",
		Long: `按属性局部更新视图的 card 配置。preflight 调 get_views 拿 viewType 后分发：
  - Kanban → kanbanCard {coverFieldId, coverResizeMode, hiddenFieldTitle}
  - Gallery → galleryCard {coverMode, coverFieldId, coverResizeMode, displayFieldName}
typed flag 与 --json 同时存在时，typed flag 优先。--no-cover 与 --cover-field-id 互斥。`,
		Example: `  dws aitable view update card --view-id VIEW_ID --cover-field-id fldXXX --cover-resize-mode contain
  dws aitable view update card --view-id VIEW_ID --no-cover
  dws aitable view update card --view-id VIEW_ID --cover-mode auto       # Gallery
  dws aitable view update card --view-id VIEW_ID --json '{"hiddenFieldTitle":true}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// flag 级互斥先校验，避免无效请求触发 preflight GET
			noCover, _ := cmd.Flags().GetBool("no-cover")
			coverFieldID, _ := cmd.Flags().GetString("cover-field-id")
			if noCover && coverFieldID != "" {
				return fmt.Errorf("--no-cover 与 --cover-field-id 互斥，请只指定一个")
			}
			baseID, tableID, viewID, blockKey, err := viewUpdateCommonPreflight(cmd, "card", nil, true)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			if noCover {
				typed["coverFieldId"] = "NONE"
			} else if coverFieldID != "" {
				typed["coverFieldId"] = coverFieldID
			}
			collectStringFlag(cmd, "cover-resize-mode", "coverResizeMode", typed)
			// 以下两个为 Kanban / Gallery 独占字段，但 server 端不强校验，多传也不会报错
			collectBoolFlag(cmd, "hidden-field-title", "hiddenFieldTitle", typed)
			collectStringFlag(cmd, "cover-mode", "coverMode", typed)
			collectBoolFlag(cmd, "display-field-name", "displayFieldName", typed)
			jsonStr, _ := cmd.Flags().GetString("json")
			block, err := mergeUpdateBlock(jsonStr, typed)
			if err != nil {
				return err
			}
			return callUpdateViewWithBlock(baseID, tableID, viewID, blockKey, block, nil)
		},
	}

	viewUpdateTimebarCmd := &cobra.Command{
		Use:   "timebar",
		Short: "更新视图 timebar 配置（仅 Gantt）",
		Long: `按属性局部更新 Gantt 视图的 ganttTimebar 配置。
子字段：startField / endField (date 字段) / displayFieldId / timelineScale (year|quarter|month|weeks) /
colorConfigs (JSON 数组) / officialHoliday (bool)。`,
		Example: `  dws aitable view update timebar --view-id VIEW_ID --start-field fldStart --end-field fldEnd --timeline-scale month
  dws aitable view update timebar --view-id VIEW_ID --json '{"colorConfigs":[]}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, blockKey, err := viewUpdateCommonPreflight(cmd, "ganttTimebar", []string{"Gantt"}, false)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			collectStringFlag(cmd, "start-field", "startField", typed)
			collectStringFlag(cmd, "end-field", "endField", typed)
			collectStringFlag(cmd, "display-field-id", "displayFieldId", typed)
			collectStringFlag(cmd, "timeline-scale", "timelineScale", typed)
			collectBoolFlag(cmd, "official-holiday", "officialHoliday", typed)
			if v, _ := cmd.Flags().GetString("color-configs"); v != "" {
				parsed := jsonStringToMap(v)
				if arr, ok := parsed.([]any); ok {
					typed["colorConfigs"] = arr
				} else {
					return fmt.Errorf("--color-configs 必须是 JSON 数组，got %T", parsed)
				}
			}
			jsonStr, _ := cmd.Flags().GetString("json")
			block, err := mergeUpdateBlock(jsonStr, typed)
			if err != nil {
				return err
			}
			return callUpdateViewWithBlock(baseID, tableID, viewID, blockKey, block, nil)
		},
	}

	viewUpdateAggregateCmd := &cobra.Command{
		Use:   "aggregate",
		Short: "更新视图字段聚合统计（仅 Grid）",
		Long: `更新 Grid 视图的 aggregate 配置。value 为 map[fieldId]→AggregateAction string，
传 null 清除单个字段聚合。便捷 flag：--field-id / --action 单字段写入；--clear-field-id 单/多清除。`,
		Example: `  dws aitable view update aggregate --view-id VIEW_ID --field-id fldX --action SUM
  dws aitable view update aggregate --view-id VIEW_ID --clear-field-id fldX,fldY
  dws aitable view update aggregate --view-id VIEW_ID --json '{"fldA":"AVG","fldB":"MAX"}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, blockKey, err := viewUpdateCommonPreflight(cmd, "aggregate", []string{"Grid"}, false)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			fldID, _ := cmd.Flags().GetString("field-id")
			action, _ := cmd.Flags().GetString("action")
			if (fldID == "") != (action == "") {
				return fmt.Errorf("--field-id 与 --action 必须同时指定（或同时省略）")
			}
			if fldID != "" {
				typed[fldID] = action
			}
			if v, _ := cmd.Flags().GetString("clear-field-id"); v != "" {
				for _, k := range parseCSVValues(v) {
					typed[k] = nil
				}
			}
			jsonStr, _ := cmd.Flags().GetString("json")
			block, err := mergeUpdateBlock(jsonStr, typed)
			if err != nil {
				return err
			}
			return callUpdateViewWithBlock(baseID, tableID, viewID, blockKey, block, nil)
		},
	}

	viewUpdateFieldWidthsCmd := &cobra.Command{
		Use:   "field-widths",
		Short: "更新视图字段列宽（仅 Grid）",
		Long:  `更新 Grid 视图的字段列宽。value 为 map[fieldId]→width(int)，可单字段或批量。`,
		Example: `  dws aitable view update field-widths --view-id VIEW_ID --field-id fldX --width 200
  dws aitable view update field-widths --view-id VIEW_ID --json '{"fldA":120,"fldB":200}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, blockKey, err := viewUpdateCommonPreflight(cmd, "fieldWidths", []string{"Grid"}, false)
			if err != nil {
				return err
			}
			typed := map[string]any{}
			fldID, _ := cmd.Flags().GetString("field-id")
			width, _ := cmd.Flags().GetInt("width")
			if (fldID == "") != !cmd.Flags().Changed("width") {
				return fmt.Errorf("--field-id 与 --width 必须同时指定（或同时省略）")
			}
			if fldID != "" {
				typed[fldID] = width
			}
			jsonStr, _ := cmd.Flags().GetString("json")
			block, err := mergeUpdateBlock(jsonStr, typed)
			if err != nil {
				return err
			}
			return callUpdateViewWithBlock(baseID, tableID, viewID, blockKey, block, nil)
		},
	}

	viewUpdateVisibleFieldsCmd := &cobra.Command{
		Use:   "visible-fields",
		Short: "更新视图可见字段列表",
		Long: `按属性更新视图的 visibleFieldIds（即列顺序）。传入的字段 ID 列表
完全替换原有顺序；首列字段不可隐藏。所有视图类型都支持。`,
		Example: `  dws aitable view update visible-fields --view-id VIEW_ID --field-ids fld1,fld2,fld3
  dws aitable view update visible-fields --view-id VIEW_ID --json '["fld1","fld2"]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, _, err := viewUpdateCommonPreflight(cmd, "visibleFieldIds", nil, false)
			if err != nil {
				return err
			}
			var fieldIDs []string
			if v, _ := cmd.Flags().GetString("field-ids"); v != "" {
				fieldIDs = parseCSVValues(v)
			}
			if jsonStr, _ := cmd.Flags().GetString("json"); jsonStr != "" {
				parsed := jsonStringToMap(jsonStr)
				arr, ok := parsed.([]any)
				if !ok {
					return fmt.Errorf("--json 必须是字符串数组，got %T", parsed)
				}
				// --json 优先（与其他 update 子命令"typed 优先"相反，因为 visible-fields
				// 不存在 key-level 合并；只能整组替换）。若同时设置则提示。
				if len(fieldIDs) > 0 {
					fmt.Fprintf(os.Stderr, "⚠️  --json 与 --field-ids 同时设置，使用 --json 的值\n")
				}
				fieldIDs = nil
				for _, e := range arr {
					if s, ok := e.(string); ok {
						fieldIDs = append(fieldIDs, s)
					} else {
						return fmt.Errorf("--json 数组元素必须是字符串，got %T", e)
					}
				}
			}
			if len(fieldIDs) == 0 {
				return fmt.Errorf("必须指定 --field-ids 或 --json 之一")
			}
			return callUpdateViewWithBlock(baseID, tableID, viewID, "visibleFieldIds", fieldIDs, nil)
		},
	}

	// view update filter / sort / group：纯 --json 入口，复用 normalize helper 做数组化
	makeViewUpdateArrayCmd := func(use, blockKey, shortDesc, hintExample string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: "更新视图 " + use + " 配置",
			Long: fmt.Sprintf(`按属性更新视图的 %s 配置（整组替换）。
%s
若传对象会自动 wrap 为数组；其他非法格式拒绝。`, shortDesc, hintExample),
			Example: fmt.Sprintf("  dws aitable view update %s --view-id VIEW_ID --json '%s'", use, hintExample),
			RunE: func(cmd *cobra.Command, args []string) error {
				baseID, tableID, viewID, _, err := viewUpdateCommonPreflight(cmd, blockKey, nil, false)
				if err != nil {
					return err
				}
				jsonStr, _ := cmd.Flags().GetString("json")
				if jsonStr == "" {
					return fmt.Errorf("必须指定 --json 传入 %s JSON 数组", blockKey)
				}
				// jsonStringToMap 只解 map，不解 array；这里直接 Unmarshal 到 any
				// 以支持 filter/sort/group 的数组入参。
				var parsed any
				if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
					return fmt.Errorf("--json 解析失败: %v", err)
				}
				cfgMap := map[string]any{blockKey: parsed}
				if err := normalizeViewConfigBlock(cfgMap); err != nil {
					return err
				}
				return callUpdateViewWithBlock(baseID, tableID, viewID, blockKey, cfgMap[blockKey], nil)
			},
		}
	}
	viewUpdateFilterCmd := makeViewUpdateArrayCmd("filter", "filter",
		"filter 数组每项为 {operator,operands}",
		`[{"operator":"and","operands":[{"operator":"eq","operands":["fldX","value"]}]}]`)
	viewUpdateSortCmd := makeViewUpdateArrayCmd("sort", "sort",
		"sort 数组每项为 {fieldId,direction}",
		`[{"fieldId":"fldX","direction":"asc"}]`)
	viewUpdateGroupCmd := makeViewUpdateArrayCmd("group", "group",
		"group 数组每项为 {fieldId,direction}",
		`[{"fieldId":"fldX","direction":"asc"}]`)

	viewUpdateNameCmd := &cobra.Command{
		Use:     "name",
		Short:   "重命名视图（= view update --name 的便捷子命令）",
		Long:    `重命名指定视图，等价于 dws aitable view update --name X。无 config 参数。`,
		Example: `  dws aitable view update name --view-id VIEW_ID --name "新视图名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id", "name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			newName, _ := cmd.Flags().GetString("name")
			return callUpdateViewWithBlock(baseID, mustGetFlag(cmd, "table-id"), mustGetFlag(cmd, "view-id"),
				"", nil, map[string]any{"newViewName": newName})
		},
	}

	viewUpdateCmd.AddCommand(
		viewUpdateCardCmd, viewUpdateTimebarCmd, viewUpdateAggregateCmd,
		viewUpdateFieldWidthsCmd, viewUpdateVisibleFieldsCmd,
		viewUpdateFilterCmd, viewUpdateSortCmd, viewUpdateGroupCmd,
		viewUpdateNameCmd,
	)

	// ─── 新增子命令：lock / frozen-cols / row-height / fill-color-rule / duplicate ───

	// commonViewArgs: lock/frozen-cols/row-height 等独立工具的公共三元组拼装
	commonViewArgs := func(cmd *cobra.Command) (map[string]any, error) {
		if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
			return nil, err
		}
		baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"baseId":  baseID,
			"tableId": mustGetFlag(cmd, "table-id"),
			"viewId":  mustGetFlag(cmd, "view-id"),
		}, nil
	}

	// view get lock：调 get_view_lock_status
	viewGetLockCmd := &cobra.Command{
		Use:   "lock",
		Short: "获取视图锁定状态",
		Long: `获取指定视图的锁定状态。返回 {baseId, tableId, viewId, locked: <bool>}：
locked 为 true 表示视图已锁定，false 表示未锁定。`,
		Example: `  dws aitable view get lock --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_view_lock_status", toolArgs)
		},
	}

	// view lock：调 lock_or_unlock_view（默认 action=lock，--off 时 action=unlock）
	viewLockCmd := &cobra.Command{
		Use:   "lock",
		Short: "锁定/解锁视图",
		Long: `锁定指定视图，禁止他人编辑。默认锁定；传 --off 解锁。
返回 {baseId, tableId, viewId, action, locked}。`,
		Example: `  dws aitable view lock --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID
  dws aitable view lock --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --off`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			action := "lock"
			if off, _ := cmd.Flags().GetBool("off"); off {
				action = "unlock"
			}
			toolArgs["action"] = action
			return callAitableHelperTool("lock_or_unlock_view", toolArgs)
		},
	}

	// view get frozen-cols：调 get_frozen_columns_of_view
	viewGetFrozenColsCmd := &cobra.Command{
		Use:   "frozen-cols",
		Short: "获取视图冻结列数",
		Long: `获取指定视图当前冻结的左侧列数。
返回 {baseId, tableId, viewId, count}：count 为 null 表示视图未显式设置冻结列。`,
		Example: `  dws aitable view get frozen-cols --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_frozen_columns_of_view", toolArgs)
		},
	}

	// view update frozen-cols：调 set_frozen_columns_of_view，--count int 必填
	viewUpdateFrozenColsCmd := &cobra.Command{
		Use:   "frozen-cols",
		Short: "更新视图冻结列数",
		Long: `设置视图冻结列数。--count N 表示从首列起冻结 N 列；--count 0 表示取消冻结。
返回 {baseId, tableId, viewId, count}。`,
		Example: `  dws aitable view update frozen-cols --view-id VIEW_ID --count 1
  dws aitable view update frozen-cols --view-id VIEW_ID --count 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("count") {
				return fmt.Errorf("必须指定 --count")
			}
			count, _ := cmd.Flags().GetInt("count")
			if count < 0 {
				return fmt.Errorf("--count 必须 >= 0，got %d", count)
			}
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			toolArgs["count"] = count
			return callAitableHelperTool("set_frozen_columns_of_view", toolArgs)
		},
	}

	// view get row-height：调 get_cell_height_of_view
	viewGetRowHeightCmd := &cobra.Command{
		Use:   "row-height",
		Short: "获取视图行高（单元格高度）",
		Long: `获取指定视图当前的单元格行高，单位为像素。
返回 {baseId, tableId, viewId, cellHeight}：cellHeight 为 null 表示视图未显式设置（前端实际显示回落到默认 32px）。`,
		Example: `  dws aitable view get row-height --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_cell_height_of_view", toolArgs)
		},
	}

	// view update row-height：调 set_cell_height_of_view，--cell-height int 必填（像素）
	viewUpdateRowHeightCmd := &cobra.Command{
		Use:   "row-height",
		Short: "更新视图行高（单元格高度）",
		Long: `设置视图单元格高度，单位为像素。--cell-height 必填，合法档位 32 / 56 / 88 / 128，默认 32。
返回 {baseId, tableId, viewId, cellHeight}。`,
		Example: `  dws aitable view update row-height --view-id VIEW_ID --cell-height 32
  dws aitable view update row-height --view-id VIEW_ID --cell-height 56`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("cell-height") {
				return fmt.Errorf("必须指定 --cell-height（像素整数）")
			}
			cellHeight, _ := cmd.Flags().GetInt("cell-height")
			if cellHeight <= 0 {
				return fmt.Errorf("--cell-height 必须 > 0，got %d", cellHeight)
			}
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			toolArgs["cellHeight"] = cellHeight
			return callAitableHelperTool("set_cell_height_of_view", toolArgs)
		},
	}

	// view get fill-color-rule：server 没独立 get 工具，从 view.conditionalFormats 投影
	viewGetFillColorRuleCmd := makeViewGetSubCmd("fill-color-rule",
		"fill-color-rule",
		"读取视图当前的条件填色规则（conditionalFormats 数组）。所有视图类型都支持。",
		"conditionalFormats", nil, false)

	// view update fill-color-rule：调 set_view_fill_color_rule，--json 数组必填
	viewUpdateFillColorRuleCmd := &cobra.Command{
		Use:   "fill-color-rule",
		Short: "更新视图数据高亮规则",
		Long: `全量覆盖指定 Grid 视图的条件填色规则。仅支持 --json 入口，每项规则结构：
  {type: cell|row|column|preRow, formatFieldId, format: {color}, filters: [{fieldId, symbol, value?}]}
- color 必须用 FORMAT_COLORS 代号（如 firstLine1..firstLine11），不支持 hex
- symbol 取 GT/LT/GTE/LTE/EQ/NE/CONTAIN/EXCLUSIVE/EXIST/UN_EXIST/ALL_OF/ANY_OF/NONE_OF/BEFORE/AFTER/NOT_BEFORE/NOT_AFTER/DATE_EQ/FROM_NOW/DATE_BETWEEN
- 传 --json '[]' 清空当前视图所有填色规则`,
		Example: `  dws aitable view update fill-color-rule --view-id VIEW_ID --json '[]'
  dws aitable view update fill-color-rule --view-id VIEW_ID --json '[{"type":"cell","formatFieldId":"fldX","format":{"color":"firstLine5"},"filters":[{"fieldId":"fldX","symbol":"GT","value":100}]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			jsonStr, _ := cmd.Flags().GetString("json")
			if jsonStr == "" {
				return fmt.Errorf("必须指定 --json 传入 conditionalFormats JSON 数组")
			}
			var parsed any
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
				return fmt.Errorf("--json 解析失败: %v", err)
			}
			arr, ok := parsed.([]any)
			if !ok {
				return fmt.Errorf("--json 必须是 JSON 数组，got %T", parsed)
			}
			toolArgs, err := commonViewArgs(cmd)
			if err != nil {
				return err
			}
			toolArgs["conditionalFormats"] = arr
			// set_view_fill_color_rule 部署在 aitable 主 server（不是 aitable-helper），
			// 与其他 set_view_* 工具的归属不同，单独走 callAitableTool。
			return callAitableTool("set_view_fill_color_rule", toolArgs)
		},
	}

	// view duplicate：调 duplicate_view，源视图字段名是 sourceViewId（与 server 对齐）
	viewDuplicateCmd := &cobra.Command{
		Use:   "duplicate",
		Short: "复制视图",
		Long: `复制指定视图，生成一个配置完全相同的新视图。
若不传 --new-name，由系统默认命名（一般为"原视图名 (副本)"）。`,
		Example: `  dws aitable view duplicate --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID
  dws aitable view duplicate --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --new-name "副本视图"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":       baseID,
				"tableId":      mustGetFlag(cmd, "table-id"),
				"sourceViewId": mustGetFlag(cmd, "view-id"),
			}
			if v, _ := cmd.Flags().GetString("new-name"); v != "" {
				toolArgs["newViewName"] = v
			}
			return callAitableHelperTool("duplicate_view", toolArgs)
		},
	}

	// 把上面新增的子命令补挂到对应父级（与 view get/update 已有的 AddCommand 调用各自累加）
	viewGetCmd.AddCommand(
		viewGetLockCmd,
		viewGetFrozenColsCmd,
		viewGetRowHeightCmd,
		viewGetFillColorRuleCmd,
	)
	viewUpdateCmd.AddCommand(
		viewUpdateFrozenColsCmd,
		viewUpdateRowHeightCmd,
		viewUpdateFillColorRuleCmd,
	)
	// viewLockCmd / viewDuplicateCmd 挂在 viewCmd 顶层（与 view get / view update 同级），
	// 在文件末尾的 viewCmd.AddCommand(...) 调用里追加。

	viewDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除视图",
		Long: `删除指定视图（View）。该操作不可逆。
禁止删除数据表中的最后一个视图；锁定视图不允许删除。`,
		Example: `  dws aitable view delete --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --yes
  # 查询 viewId: dws aitable view get --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			viewID := mustGetFlag(cmd, "view-id")
			if !confirmDelete("视图", viewID) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("delete_view", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  viewID,
			})
		},
	}

	// ── form: 表单管理 ──────────────────────────────────────────

	formCmd := &cobra.Command{Use: "form", Short: "表单管理", RunE: groupRunE}
	formFieldCmd := &cobra.Command{Use: "field", Short: "表单字段管理", RunE: groupRunE}
	formShareCmd := &cobra.Command{Use: "share", Short: "表单分享管理", RunE: groupRunE}

	formListCmd := &cobra.Command{
		Use:     "list",
		Short:   "列出表单视图",
		Long:    `列出指定数据表下的所有表单视图。每个表单视图返回 viewId、name、title、createdAt（shareFormUuid 不在此返回，需 form share get 单独获取）。`,
		Example: `  dws aitable form list --base-id BASE_ID --table-id TABLE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("list_form_views", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
			})
		},
	}

	formCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建表单视图",
		Long: `在指定数据表下创建一个新的表单视图（FormDesigner 类型）。
等价于 view create --view-type FormDesigner，但走 form 命令组对齐表单工作流。`,
		Example: `  dws aitable form create --base-id BASE_ID --table-id TABLE_ID --name "员工信息收集"
  dws aitable form create --base-id BASE_ID --table-id TABLE_ID --name "用户调研" --description "2026 年度调研"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":   baseID,
				"tableId":  mustGetFlag(cmd, "table-id"),
				"viewName": mustGetFlag(cmd, "name"),
				"viewType": "FormDesigner",
			}
			return callMCPTool("create_view", toolArgs)
		},
	}

	formGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取单个表单视图详情",
		Long: `按 viewId 获取单个表单视图的元信息（name/title/createdAt/shareFormUuid）。
内部拉取 list_form_views 后在客户端按 viewId 精确筛出单条（服务端的 viewIds
过滤参数当前不生效，会返回全表所有表单，故在此侧过滤）。data 即命中的表单对象。`,
		Example: `  dws aitable form get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			viewID := mustGetFlag(cmd, "view-id")
			raw, err := callMCPToolReturnTextOnServer(context.Background(), "aitable-helper", "list_form_views", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
			})
			if err != nil {
				return err
			}
			var parsed any
			if e := json.Unmarshal([]byte(raw), &parsed); e != nil {
				return &CLIError{
					Code:    CodeMCPToolError,
					Message: fmt.Sprintf("list_form_views response is not valid JSON: %v", e),
				}
			}
			form, ok := findFormViewByID(parsed, viewID)
			if !ok {
				return &CLIError{
					Code:       CodeMCPToolError,
					Message:    fmt.Sprintf("form view %s not found in table", viewID),
					Suggestion: "用 dws aitable form list --base-id <baseId> --table-id <tableId> 查看可用 viewId 列表",
				}
			}
			return printViewSubBlock(form)
		},
	}

	formDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除表单",
		Long: `删除指定数据表下的表单视图（不可逆）。
调用前建议先通过 form list 确认目标视图。`,
		Example: `  dws aitable form delete --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --yes
  # 查询 viewId: dws aitable form list --base-id <baseId> --table-id <tableId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			viewID := mustGetFlag(cmd, "view-id")
			if !confirmDelete("表单", viewID) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("delete_form_view", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  viewID,
			})
		},
	}

	formUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新表单配置",
		Long: `更新表单整体配置（标题、描述）。
--title（或其等价别名 --name）和 --description 至少传入一项。
--title 与 --name 完全等价；同时传入时以 --title 优先。`,
		Example: `  dws aitable form update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --title "员工信息收集"
  dws aitable form update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --name "员工信息收集"
  dws aitable form update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --description "请如实填写"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			title := resolveFormUpdateTitle(cmd)
			description, _ := cmd.Flags().GetString("description")
			if title == "" && description == "" {
				return fmt.Errorf("--title (or --name) and --description must specify at least one")
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
			}
			if title != "" {
				toolArgs["title"] = title
			}
			if description != "" {
				toolArgs["description"] = description
			}
			return callAitableHelperTool("update_form_info", toolArgs)
		},
	}

	// ── form questions: 表单题目（form 视角的字段管理，等价于 field create / field delete） ──

	formQuestionsCmd := &cobra.Command{Use: "questions", Short: "表单题目管理（等价于 field create / delete）", RunE: groupRunE}

	formQuestionsCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "向表单添加题目（等价于 field create）",
		Long: `在数据表中创建字段，即向表单添加题目。
入参与 ` + "`field create`" + ` 完全一致：可用 --fields 批量创建，或 --name + --type 单题模式。
若需将题目设为必填，创建后请用 form field update --required true 单独设置。`,
		Example: `  dws aitable form questions create --base-id BASE_ID --table-id TABLE_ID \
    --fields '[{"fieldName":"姓名","type":"text"},{"fieldName":"邮箱","type":"text"}]'
  dws aitable form questions create --base-id BASE_ID --table-id TABLE_ID --name "电话" --type "text"`,
		RunE: fieldCreateCmd.RunE,
	}

	formQuestionsDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "从表单删除题目（等价于 field delete，不可逆）",
		Long: `删除指定字段（即表单题目）。**不可逆**。
入参与 ` + "`field delete`" + ` 完全一致：必须传 --field-id；目前 MCP 不支持批量删除，需多次调用。`,
		Example: `  dws aitable form questions delete --base-id BASE_ID --table-id TABLE_ID --field-id fldXXX --yes`,
		RunE:    fieldDeleteCmd.RunE,
	}

	formFieldListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出表单字段",
		Long: `列出指定表单视图当前可见的字段及其配置。
返回每个字段的 fieldId、name、type、required、hidden、description。
注意：hidden=true 的字段不会出现在此返回（它们在表单中已隐藏）；如需查看全部字段请用 field get。`,
		Example: `  dws aitable form field list --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("list_form_fields", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
			})
		},
	}

	formFieldUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新表单字段",
		Long: `更新表单视图中某个字段的必填状态或描述。
--required 或 --field-description 至少传入一项。`,
		Example: `  dws aitable form field update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id FIELD_ID --required true
  dws aitable form field update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id FIELD_ID --field-description "请填写真实姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id", "field-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
				"fieldId": mustGetFlag(cmd, "field-id"),
			}
			if v, _ := cmd.Flags().GetString("required"); v != "" {
				toolArgs["required"] = v == "true"
			}
			if v, _ := cmd.Flags().GetString("field-description"); v != "" {
				toolArgs["fieldDescription"] = v
			}
			return callAitableHelperTool("update_form_field", toolArgs)
		},
	}

	formFieldHideCmd := &cobra.Command{
		Use:   "hide",
		Short: "切换表单字段隐藏",
		Long: `切换表单视图中某个字段的隐藏/显示状态。
--hidden true：在表单中隐藏该字段（填写者不可见）
--hidden false：显示该字段`,
		Example: `  dws aitable form field hide --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id FIELD_ID --hidden true
  dws aitable form field hide --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id FIELD_ID --hidden false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id", "field-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("update_form_field_hidden", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
				"fieldId": mustGetFlag(cmd, "field-id"),
				"hidden":  mustGetFlag(cmd, "hidden") == "true",
			})
		},
	}

	formShareGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取表单分享配置",
		Long: `读取指定视图当前的分享表单配置。
返回 enabled（是否开启）、status、shareFormUuid、formName 等信息。
若该视图尚未开启分享表单，enabled=false、status=0。`,
		Example: `  dws aitable form share get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_share_form_config", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
			})
		},
	}

	formShareUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "开启/关闭分享表单",
		Long: `开启或关闭指定视图的分享表单。
--enabled true 表示开启分享，--enabled false 表示关闭分享。`,
		Example: `  dws aitable form share update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --enabled true
  dws aitable form share update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --enabled false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "view-id", "enabled"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("update_share_form", map[string]any{
				"baseId":  baseID,
				"tableId": mustGetFlag(cmd, "table-id"),
				"viewId":  mustGetFlag(cmd, "view-id"),
				"enabled": mustGetFlag(cmd, "enabled"),
			})
		},
	}

	// ── workflow: 自动化工作流管理 ────────────────────────────────

	workflowCmd := &cobra.Command{
		Use:   "workflow",
		Short: "自动化工作流管理（创建 / 更新 / 启停 / 查看 / 列表）",
		RunE:  groupRunE,
	}

	workflowCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建并发布自动化工作流",
		Long: `在指定 Base 中创建并发布自动化工作流。
--dsl 必须是完整的 workflow-dsl/v1 JSON 对象；涉及数据表、字段或视图的节点应使用真实的 sheetId / fieldId / viewId。

--dsl 支持内联 JSON、@文件路径，或 - 从 stdin 读取。创建属于非幂等操作，CLI 不会自动重试。
返回 data.valid、flowId、flowSchema、stepNodeIds、referenceMap、issues；即使 status=success，
valid=false 仍表示 DSL 校验或发布未通过，必须读取 issues 修正后再调用。`,
		Example: `  dws aitable workflow create --base-id BASE_ID --dsl @workflow.json --locale zh-CN
  cat workflow.json | dws aitable workflow create --base-id BASE_ID --dsl -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dsl"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			dsl, err := resolveWorkflowDSL(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
				"dsl":    dsl,
			}
			if locale, _ := cmd.Flags().GetString("locale"); strings.TrimSpace(locale) != "" {
				toolArgs["locale"] = locale
			}
			// create_workflow is non-idempotent. Bypass the retry wrapper to
			// prevent an uncertain first response from creating a duplicate.
			return callMCPToolOnServer("aitable", "create_workflow", toolArgs)
		},
	}

	workflowUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新并发布已有自动化工作流",
		Long: `在指定 Base 中更新并发布已有自动化工作流。
建议先用 workflow get 留底当前详情；--dsl 必须是完整的 workflow-dsl/v1 JSON 对象，而不是局部 patch。

--dsl 支持内联 JSON、@文件路径，或 - 从 stdin 读取。更新会发布传入的目标 DSL；请提供完整、可独立校验的 workflow-dsl/v1 对象。
返回 data.valid、flowId、flowSchema、stepNodeIds、referenceMap、issues；即使 status=success，
valid=false 仍表示 DSL 校验或发布未通过，必须读取 issues 修正后再调用。`,
		Example: `  dws aitable workflow update --base-id BASE_ID --workflow-id WORKFLOW_ID --dsl @workflow.json --locale zh-CN
  cat workflow.json | dws aitable workflow update --base-id BASE_ID --workflow-id WORKFLOW_ID --dsl -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "workflow-id", "dsl"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			dsl, err := resolveWorkflowDSL(cmd)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":     baseID,
				"workflowId": mustGetFlag(cmd, "workflow-id"),
				"dsl":        dsl,
			}
			if locale, _ := cmd.Flags().GetString("locale"); strings.TrimSpace(locale) != "" {
				toolArgs["locale"] = locale
			}
			return callAitableTool("update_workflow", toolArgs)
		},
	}

	workflowEnableCmd := &cobra.Command{
		Use:   "enable",
		Short: "启用指定工作流",
		Long: `启用指定 Base 中的自动化工作流。启用后工作流将按配置的触发条件自动执行。
返回 {workflowId, enabled} 用于确认操作结果（enabled 为动作确认而非状态查询）。
可用 workflow create 创建工作流，或用 workflow list 获取已有 workflowId。`,
		Example: `  dws aitable workflow enable --base-id BASE_ID --workflow-id WORKFLOW_ID
  # 查询 workflowId: dws aitable workflow list --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "workflow-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("enable_workflow", map[string]any{
				"baseId":     baseID,
				"workflowId": mustGetFlag(cmd, "workflow-id"),
			})
		},
	}

	workflowDisableCmd := &cobra.Command{
		Use:   "disable",
		Short: "禁用指定工作流（高危）",
		Long: `禁用指定 Base 中的自动化工作流。禁用后工作流将不再自动触发执行。
此操作直接影响业务自动化，建议在交互场景下用 --yes 二次确认。
返回 {workflowId, disabled} 用于确认操作结果（disabled 为动作确认而非状态查询）。`,
		Example: `  dws aitable workflow disable --base-id BASE_ID --workflow-id WORKFLOW_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "workflow-id"); err != nil {
				return err
			}
			// 先校验所有必填 flag（含 base-id），再进入二次确认提示，
			// 避免对无效请求弹无意义的确认（与 advperm role-delete 同模式）。
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			workflowID := mustGetFlag(cmd, "workflow-id")
			if !confirmDelete("工作流（禁用后将停止自动触发）", workflowID) {
				return nil
			}
			return callAitableHelperTool("disable_workflow", map[string]any{
				"baseId":     baseID,
				"workflowId": workflowID,
			})
		},
	}

	workflowGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取单个工作流详情",
		Long: `获取指定 Base 中某个自动化工作流的详细信息（含触发条件、动作步骤等）。
返回结构由服务端 getFlowDetail 透传，agent 应按需读取关心字段。`,
		Example: `  dws aitable workflow get --base-id BASE_ID --workflow-id WORKFLOW_ID
  # 查询 workflowId: dws aitable workflow list --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "workflow-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_workflow", map[string]any{
				"baseId":     baseID,
				"workflowId": mustGetFlag(cmd, "workflow-id"),
			})
		},
	}

	workflowListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出 Base 下的工作流",
		Long: `列出指定 Base 中的自动化工作流，支持分页。
--limit 默认服务端 20，最大 100；--offset 默认 0。
返回结构由服务端 searchFlows 透传，含 workflowId、name 等字段。`,
		Example: `  dws aitable workflow list --base-id BASE_ID
  dws aitable workflow list --base-id BASE_ID --limit 50 --offset 100`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"baseId": baseID}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetInt("limit")
				if limit < 1 || limit > 100 {
					return fmt.Errorf("--limit 必须在 [1, 100] 范围内，got %d", limit)
				}
				toolArgs["limit"] = limit
			}
			if cmd.Flags().Changed("offset") {
				offset, _ := cmd.Flags().GetInt("offset")
				if offset < 0 {
					return fmt.Errorf("--offset 必须 >= 0，got %d", offset)
				}
				toolArgs["offset"] = offset
			}
			return callAitableHelperTool("list_workflows", toolArgs)
		},
	}

	// ── dashboard: 仪表盘管理 ────────────────────────────────────

	dashboardCmd := &cobra.Command{Use: "dashboard", Short: "仪表盘管理", RunE: groupRunE}

	dashboardConfigExampleCmd := &cobra.Command{
		Use:   "config-example",
		Short: "获取仪表盘配置示例",
		Long: `返回 dashboard config 的完整结构示例（JSONC 格式，含注释说明每个字段的含义和约束）。
可作为 dashboard create / dashboard update 的 --config 参数结构参考。`,
		Example: `  dws aitable dashboard config-example`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callAitableTool("get_dashboard_config_example", map[string]any{})
		},
	}

	dashboardGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取仪表盘信息",
		Long: `获取指定 dashboard 的详细信息。
返回 dashboardName、filters、layout，以及该 dashboard 下的 charts summary（chartId、chartName、chartType）。`,
		Example: `  dws aitable dashboard get --base-id BASE_ID --dashboard-id DASHBOARD_ID
  # 查询 dashboardId: dws aitable base get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("get_dashboard", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
			})
		},
	}

	dashboardCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建仪表盘",
		Long: `在指定 Base 下创建 dashboard。
调用前建议先调用 dashboard config-example 了解 --config 入参结构和要求。
返回新创建的 dashboard 详情。`,
		Example: `  dws aitable dashboard create --base-id BASE_ID --name "销售看板"
  dws aitable dashboard create --base-id BASE_ID --config '{"name":"销售看板",...}'
  # 先获取配置示例: dws aitable dashboard config-example`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg map[string]any
			if configStr, _ := cmd.Flags().GetString("config"); configStr != "" {
				if parsed, ok := jsonStringToMap(configStr).(map[string]any); ok {
					cfg = parsed
				} else {
					cfg = make(map[string]any)
				}
			} else {
				cfg = make(map[string]any)
			}
			if name, _ := cmd.Flags().GetString("name"); name != "" {
				cfg["name"] = name
			}
			if len(cfg) == 0 {
				return fmt.Errorf("must specify either --config or --name")
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callMCPTool("create_dashboard", map[string]any{
				"baseId": baseID,
				"config": cfg,
			})
		},
	}

	dashboardUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新仪表盘",
		Long: `更新指定 dashboard 的配置。
调用前建议先调用 dashboard config-example 了解 --config 入参结构和要求。
传入需要更新的字段，未传入的字段保持原值。`,
		Example: `  dws aitable dashboard update --base-id BASE_ID --dashboard-id DASHBOARD_ID --name "新名称"
  dws aitable dashboard update --base-id BASE_ID --dashboard-id DASHBOARD_ID --config '{"name":"新名称"}'
  # 查询 dashboardId: dws aitable base get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id"); err != nil {
				return err
			}
			var cfg map[string]any
			if configStr, _ := cmd.Flags().GetString("config"); configStr != "" {
				if parsed, ok := jsonStringToMap(configStr).(map[string]any); ok {
					cfg = parsed
				} else {
					cfg = make(map[string]any)
				}
			} else {
				cfg = make(map[string]any)
			}
			if name, _ := cmd.Flags().GetString("name"); name != "" {
				cfg["name"] = name
			}
			if len(cfg) == 0 {
				return fmt.Errorf("must specify either --config or --name")
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("update_dashboard", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"config":      cfg,
			})
		},
	}

	dashboardDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除仪表盘",
		Long:  `删除指定 dashboard，会级联删除该 dashboard 下的所有 chart；删除操作不可逆。`,
		Example: `  dws aitable dashboard delete --base-id BASE_ID --dashboard-id DASHBOARD_ID --yes
  # 查询 dashboardId: dws aitable base get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id"); err != nil {
				return err
			}
			dashboardID := mustGetFlag(cmd, "dashboard-id")
			if !confirmDelete("仪表盘", dashboardID) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":      baseID,
				"dashboardId": dashboardID,
			}
			if v, _ := cmd.Flags().GetString("reason"); v != "" {
				toolArgs["reason"] = v
			}
			return callAitableTool("delete_dashboard", toolArgs)
		},
	}

	// dashboard arrange：自动重排仪表盘内的图表布局
	dashboardArrangeCmd := &cobra.Command{
		Use:   "arrange",
		Short: "自动重排仪表盘图表布局",
		Long: `对指定仪表盘做服务端智能布局：把图表按行铺满网格，避免某行只占半幅、留下大片空白。
仪表盘网格总列数 totalColumns 通常是 12 或 48；返回字段 alignedChartCount 表示本次重排涉及的图表数。

返回 data: {baseId, dashboardId, totalColumns, alignedChartCount, layout: [...]}。
layout 数组里每项含图表的新位置（row/col/width/height）。`,
		Example: `  dws aitable dashboard arrange --base-id BASE_ID --dashboard-id DASHBOARD_ID
  # 查询 dashboardId: dws aitable base get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("align_dashboard", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
			})
		},
	}

	// ── dashboard share: 仪表盘分享管理 ────────────────────────────

	dashboardShareCmd := &cobra.Command{Use: "share", Short: "仪表盘分享管理", RunE: groupRunE}

	dashboardShareGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取仪表盘分享配置",
		Long:  `查询指定 dashboard 的分享配置，返回是否开启、分享类型（PUBLIC/ORG）以及分享链接。`,
		Example: `  dws aitable dashboard share get --base-id BASE_ID --dashboard-id DASHBOARD_ID
  # 查询 dashboardId: dws aitable base get --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("get_dashboard_share", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
			})
		},
	}

	dashboardShareUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新仪表盘分享配置",
		Long: `开启/关闭 dashboard 分享并可设置分享类型。
--enabled=true 表示开启分享，--enabled=false 表示关闭分享；
--share-type 仅在开启时生效，可选 PUBLIC 或 ORG。`,
		Example: `  dws aitable dashboard share update --base-id BASE_ID --dashboard-id DASHBOARD_ID --enabled true --share-type PUBLIC
  dws aitable dashboard share update --base-id BASE_ID --dashboard-id DASHBOARD_ID --enabled false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "enabled"); err != nil {
				return err
			}
			enabled, err := parseBoolFlag(cmd, "enabled")
			if err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"enabled":     enabled,
			}
			if v, _ := cmd.Flags().GetString("share-type"); v != "" {
				toolArgs["shareType"] = v
			}
			if cmd.Flags().Changed("allow-back-to-doc") {
				if v, err := cmd.Flags().GetBool("allow-back-to-doc"); err == nil {
					toolArgs["allowBackToDoc"] = v
				}
			}
			return callAitableTool("update_dashboard_share", toolArgs)
		},
	}

	// ── chart: 图表管理 ──────────────────────────────────────────

	chartCmd := &cobra.Command{Use: "chart", Short: "图表管理", RunE: groupRunE}

	chartWidgetsExampleCmd := &cobra.Command{
		Use:   "widgets-example",
		Short: "获取图表配置示例",
		Long: `返回所有图表类型的 widget config 示例（JSONC 格式，含注释说明每个字段的含义和约束）。
可作为 chart create / chart update 的 --config 参数结构参考，根据目标图表类型选取对应示例。`,
		Example: `  dws aitable chart widgets-example`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callAitableTool("get_dashboard_widgets_example", map[string]any{})
		},
	}

	chartGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取图表信息",
		Long: `获取指定 chart 的详细信息。
返回所属 dashboardId、chartName、chartType、widget.config 以及布局项。
返回的 config 中 sheet 为该图表引用的数据表 tableId，view 为视图 viewId。`,
		Example: `  dws aitable chart get --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID
  # 查询 chartId: dws aitable dashboard get --base-id <baseId> --dashboard-id <dashboardId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "chart-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("get_chart", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"chartId":     mustGetFlag(cmd, "chart-id"),
			})
		},
	}

	chartCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建图表",
		Long: `在指定 dashboard 下创建 chart。
调用前建议先调用 chart widgets-example 了解 --config 入参结构和要求。
--layout 为必填参数，指定图表在 dashboard 中的位置和大小（12 列网格布局）。

布局说明：
  x/y 表示横纵坐标，w/h 表示宽度/高度，单位是列数或行数。
  仪表盘是网格布局共 12 列、行数无限制。
  同一行的图表保持高度一致，每行的图表宽度相加需要正好将整行填满。`,
		Example: `  dws aitable chart create --base-id BASE_ID --dashboard-id DASHBOARD_ID \
    --config '{"chartName":"销售柱图","chartType":"bar",...}' \
    --layout '{"x":0,"y":0,"w":6,"h":4}'
  # 先获取配置示例: dws aitable chart widgets-example`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "config", "layout"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callMCPTool("create_chart", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"config":      jsonStringToMap(mustGetFlag(cmd, "config")),
				"layout":      jsonStringToMap(mustGetFlag(cmd, "layout")),
			})
		},
	}

	chartUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新图表",
		Long: `更新指定 chart 的配置或布局。
--config 为必填参数，即使只想改布局也要带上完整的图表配置（服务端会拒绝缺 config 的请求，
空对象 {} 同样被拒，至少要包含 chartName）。--layout 可选，仅调整位置/大小时追加。
调用前建议先调用 chart widgets-example 了解 --config 入参结构和要求，
或先用 chart get 拿到当前 config 再改。`,
		Example: `  dws aitable chart update --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID \
    --config '{"chartName":"新柱图名",...}'
  # 只改布局也必须带 --config（用 chart get 拿到当前 config 原样回传）：
  dws aitable chart update --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID \
    --config '{"chartName":"柱图",...}' --layout '{"x":0,"y":4,"w":12,"h":4}'
  # 查询 chartId: dws aitable dashboard get --base-id <baseId> --dashboard-id <dashboardId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "chart-id", "config"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"chartId":     mustGetFlag(cmd, "chart-id"),
				"config":      jsonStringToMap(mustGetFlag(cmd, "config")),
			}
			if v, _ := cmd.Flags().GetString("layout"); v != "" {
				toolArgs["layout"] = jsonStringToMap(v)
			}
			return callAitableTool("update_chart", toolArgs)
		},
	}

	chartDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除图表",
		Long:  `删除指定 chart，并同步删除其在 dashboard 中对应的布局项。删除操作不可逆。`,
		Example: `  dws aitable chart delete --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID --yes
  # 查询 chartId: dws aitable dashboard get --base-id <baseId> --dashboard-id <dashboardId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "chart-id"); err != nil {
				return err
			}
			chartID := mustGetFlag(cmd, "chart-id")
			if !confirmDelete("图表", chartID) {
				return nil
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"chartId":     chartID,
			}
			if v, _ := cmd.Flags().GetString("reason"); v != "" {
				toolArgs["reason"] = v
			}
			return callAitableTool("delete_chart", toolArgs)
		},
	}

	// ── chart share: 图表分享管理 ────────────────────────────────

	chartShareCmd := &cobra.Command{Use: "share", Short: "图表分享管理", RunE: groupRunE}

	chartShareGetCmd := &cobra.Command{
		Use:   "get",
		Short: "获取图表分享配置",
		Long:  `查询指定 chart 的分享配置，返回是否开启、分享类型（PUBLIC/ORG）以及分享链接。`,
		Example: `  dws aitable chart share get --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID
  # 查询 chartId: dws aitable dashboard get --base-id <baseId> --dashboard-id <dashboardId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "chart-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableTool("get_chart_share", map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"chartId":     mustGetFlag(cmd, "chart-id"),
			})
		},
	}

	chartShareUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新图表分享配置",
		Long: `开启/关闭 chart 分享并可设置分享类型。
--enabled=true 表示开启分享，--enabled=false 表示关闭分享；
--share-type 仅在开启时生效，可选 PUBLIC 或 ORG。`,
		Example: `  dws aitable chart share update --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID --enabled true --share-type ORG
  dws aitable chart share update --base-id BASE_ID --dashboard-id DASHBOARD_ID --chart-id CHART_ID --enabled false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "dashboard-id", "chart-id", "enabled"); err != nil {
				return err
			}
			enabled, err := parseBoolFlag(cmd, "enabled")
			if err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":      baseID,
				"dashboardId": mustGetFlag(cmd, "dashboard-id"),
				"chartId":     mustGetFlag(cmd, "chart-id"),
				"enabled":     enabled,
			}
			if v, _ := cmd.Flags().GetString("share-type"); v != "" {
				toolArgs["shareType"] = v
			}
			if cmd.Flags().Changed("allow-back-to-doc") {
				if v, err := cmd.Flags().GetBool("allow-back-to-doc"); err == nil {
					toolArgs["allowBackToDoc"] = v
				}
			}
			return callAitableTool("update_chart_share", toolArgs)
		},
	}

	// ── export / import: 数据导入导出 ────────────────────────────

	exportCmd := &cobra.Command{Use: "export", Short: "数据导出", RunE: groupRunE}

	exportDataCmd := &cobra.Command{
		Use:   "data",
		Short: "导出数据",
		Long: `导出 AI 表格数据的统一入口。
不传 --task-id 时，根据 --scope / --export-format 创建新的导出任务，并同步等待结果；
若在等待窗口内完成，则直接返回 downloadUrl 和 fileName。
传入 --task-id 时，继续等待该任务，不会重新创建。

scope 可选值：all（整个 Base）、table（指定数据表）、view（指定视图）。
export-format 可选值：excel、attachment、excel_and_attachment、excel_with_inline_images。`,
		Example: `  dws aitable export data --base-id BASE_ID --scope all --export-format excel --format json
  dws aitable export data --base-id BASE_ID --scope table --table-id TABLE_ID --export-format excel --format json
  dws aitable export data --base-id BASE_ID --scope view --table-id TABLE_ID --view-id VIEW_ID --export-format excel --format json
  dws aitable export data --base-id BASE_ID --task-id TASK_ID --format json
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
			}
			taskID, _ := cmd.Flags().GetString("task-id")
			taskID = strings.TrimSpace(taskID)
			exportFormat := resolveAitableExportFormat(cmd)
			if taskID != "" {
				if exportFormat != "" || cmd.Flags().Changed("scope") || cmd.Flags().Changed("table-id") || cmd.Flags().Changed("view-id") {
					return fmt.Errorf("--task-id is mutually exclusive with --scope, --export-format, --table-id, and --view-id")
				}
				toolArgs["taskId"] = taskID
			} else {
				if err := validateRequiredFlags(cmd, "scope"); err != nil {
					return err
				}
				if exportFormat == "" {
					return fmt.Errorf("missing required flag(s): --export-format")
				}
				scope := strings.ToLower(strings.TrimSpace(mustGetFlag(cmd, "scope")))
				switch scope {
				case "all":
					if cmd.Flags().Changed("table-id") || cmd.Flags().Changed("view-id") {
						return fmt.Errorf("--scope=all does not accept --table-id or --view-id")
					}
				case "table":
					if err := validateRequiredFlags(cmd, "table-id"); err != nil {
						return err
					}
					if cmd.Flags().Changed("view-id") {
						return fmt.Errorf("--scope=table does not accept --view-id")
					}
				case "view":
					if err := validateRequiredFlags(cmd, "table-id", "view-id"); err != nil {
						return err
					}
				default:
					return fmt.Errorf("--scope must be one of all, table, or view, got %q", scope)
				}
				exportFormat = strings.ToLower(strings.TrimSpace(exportFormat))
				switch exportFormat {
				case "excel", "attachment", "excel_and_attachment", "excel_with_inline_images":
				default:
					return fmt.Errorf("--export-format must be one of excel, attachment, excel_and_attachment, or excel_with_inline_images, got %q", exportFormat)
				}
				toolArgs["scope"] = scope
				toolArgs["format"] = exportFormat
				if v, _ := cmd.Flags().GetString("table-id"); v != "" {
					toolArgs["tableId"] = v
				}
				if v, _ := cmd.Flags().GetString("view-id"); v != "" {
					toolArgs["viewId"] = v
				}
			}
			if v, _ := cmd.Flags().GetInt("timeout-ms"); v > 0 {
				toolArgs["timeoutMs"] = v
			}
			return callAitableTool("export_data", toolArgs)
		},
	}

	importCmd := &cobra.Command{Use: "import", Short: "数据导入", RunE: groupRunE}

	// ── advperm: 高级权限 / 自定义角色 ────────────────────────────

	advpermCmd := &cobra.Command{Use: "advperm", Short: "高级权限管理（开关 / 角色查看与删除）", RunE: groupRunE}

	advpermEnableCmd := &cobra.Command{
		Use:   "enable",
		Short: "开启高级权限总开关",
		Long: `开启指定 Base 的高级权限总开关。
开启后，advperm role-list / role-get 等角色配置才会真正生效。`,
		Example: `  dws aitable advperm enable --base-id BASE_ID
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("set_advanced_permission", map[string]any{
				"baseId":  baseID,
				"enabled": true,
			})
		},
	}

	advpermDisableCmd := &cobra.Command{
		Use:   "disable",
		Short: "关闭高级权限总开关（高危）",
		Long: `关闭指定 Base 的高级权限总开关。
关闭后，所有自定义角色配置都不再生效，全员回退到默认权限。
此操作影响范围较大，请确认后再操作（需 --yes 确认）。
要求：调用者必须是该 AI 表格的管理员/Owner，否则返回 401 AUTH_ERROR，
message: "the current user must be a manager (administrator) of this base to manage roles or advanced permission"。`,
		Example: `  dws aitable advperm disable --base-id BASE_ID --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			// 先确认 baseID 合法再进入二次确认提示，避免对无效请求弹无意义的确认。
			if !commandDryRun(cmd) && !confirmDelete("高级权限（关闭后所有自定义角色失效）", baseID) {
				return nil
			}
			return callAitableHelperTool("set_advanced_permission", map[string]any{
				"baseId":  baseID,
				"enabled": false,
			})
		},
	}

	advpermRoleListCmd := &cobra.Command{
		Use:   "role-list",
		Short: "列出 Base 下所有角色",
		Long: `列出指定 Base 下的全部角色，返回 enabled 标志、defaultRole 与 roles 列表。
roles 同时包含自定义角色和系统角色，用 system (boolean) 字段区分（旧版本里叫 isSystem，服务端已统一为 system）。
subRoles[].display.* 提供人类可读标签（authLevelLabel / targetTypeLabel / permissionScopeNote 等）。
不返回角色成员列表。
非管理员也可调用此命令读取角色配置。`,
		Example: `  dws aitable advperm role-list --base-id BASE_ID
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("list_roles", map[string]any{
				"baseId": baseID,
			})
		},
	}

	advpermRoleGetCmd := &cobra.Command{
		Use:   "role-get",
		Short: "获取单个角色完整配置",
		Long:  `获取指定 Base 下单个角色的完整配置，含 subRoles 与字段/行级权限。`,
		Example: `  dws aitable advperm role-get --base-id BASE_ID --role-id ROLE_ID
  # 查询 roleId: dws aitable advperm role-list --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "role-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			return callAitableHelperTool("get_role", map[string]any{
				"baseId": baseID,
				"roleId": mustGetFlag(cmd, "role-id"),
			})
		},
	}

	advpermRoleCreateCmd := &cobra.Command{
		Use:   "role-create",
		Short: "创建自定义角色",
		Long: `在指定 Base 下创建一个自定义角色（系统角色禁止通过本命令创建）。
必填 --base-id 与 --name；可选 --role-type / --flow-type / --sub-roles。
--sub-roles 为 JSON 数组，每项 {targetId, targetType, authLevel, appId?, config?}。
  - targetType: sheet / dashboard / app
  - authLevel: manage / edit-own / edit-custom-field / edit-field-range / read / none
返回新建角色的完整配置（同 role-get 出参）。
要求：调用者必须是该 AI 表格的管理员/Owner；Base 须已开启高级权限。`,
		Example: `  dws aitable advperm role-create --base-id BASE_ID --name "市场可读" --sub-roles '[{"targetId":"<sheetId>","targetType":"sheet","authLevel":"read"}]'
  dws aitable advperm role-create --base-id BASE_ID --name "纯角色无权限"   # 后续可通过 role-update 增量补 subRoles`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
				"name":   mustGetFlag(cmd, "name"),
			}
			if v, _ := cmd.Flags().GetString("role-type"); v != "" {
				toolArgs["roleType"] = v
			}
			if v, _ := cmd.Flags().GetString("flow-type"); v != "" {
				toolArgs["flowType"] = v
			}
			if v, _ := cmd.Flags().GetString("sub-roles"); v != "" {
				var subRoles any
				if err := json.Unmarshal([]byte(v), &subRoles); err != nil {
					return fmt.Errorf("--sub-roles 解析失败: %v\n  hint: 需要 JSON 数组，每项 {targetId, targetType, authLevel, appId?, config?}", err)
				}
				if _, ok := subRoles.([]any); !ok {
					return fmt.Errorf("--sub-roles 必须是 JSON 数组，got %T", subRoles)
				}
				toolArgs["subRoles"] = subRoles
			}
			return callAitableHelperTool("create_role", toolArgs)
		},
	}

	advpermRoleUpdateCmd := &cobra.Command{
		Use:   "role-update",
		Short: "增量更新自定义角色配置（patch 语义）",
		Long: `按 PATCH 语义增量更新指定自定义角色。系统角色禁止更新。
必填 --base-id 与 --role-id；可选 --name / --role-type / --flow-type / --sub-roles。
未传的字段保持不变；传 --sub-roles 时服务端按 (targetId, targetType) 合并到现有
subRoles，入参中的 sub 整体替换该 sub，入参未提及的 sub 保留不变（不需要先调
role-get 自行 merge）。
要求：调用者必须是该 AI 表格的管理员/Owner；Base 须已开启高级权限。`,
		Example: `  dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID --name "新名字"
  dws aitable advperm role-update --base-id BASE_ID --role-id ROLE_ID --sub-roles '[{"targetId":"<sheetId>","targetType":"sheet","authLevel":"edit-own"}]'
  # 查询 roleId: dws aitable advperm role-list --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "role-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
				"roleId": mustGetFlag(cmd, "role-id"),
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["name"] = v
			}
			if v, _ := cmd.Flags().GetString("role-type"); v != "" {
				toolArgs["roleType"] = v
			}
			if v, _ := cmd.Flags().GetString("flow-type"); v != "" {
				toolArgs["flowType"] = v
			}
			if v, _ := cmd.Flags().GetString("sub-roles"); v != "" {
				var subRoles any
				if err := json.Unmarshal([]byte(v), &subRoles); err != nil {
					return fmt.Errorf("--sub-roles 解析失败: %v\n  hint: 需要 JSON 数组，每项 {targetId, targetType, authLevel, appId?, config?}", err)
				}
				if _, ok := subRoles.([]any); !ok {
					return fmt.Errorf("--sub-roles 必须是 JSON 数组，got %T", subRoles)
				}
				toolArgs["subRoles"] = subRoles
			}
			return callAitableHelperTool("patch_role", toolArgs)
		},
	}

	advpermRoleDeleteCmd := &cobra.Command{
		Use:   "role-delete",
		Short: "删除自定义角色（不可逆）",
		Long: `删除 Base 下指定的自定义角色（system=true 的系统角色禁删，服务端返回 600 / "Illegal argument"）。
删除操作不可逆，请先通过 advperm role-list 确认目标 roleId（必须是数字 long 字符串，传 owner / manager 等字符串 meta-id 会被服务端拒为 INVALID_PARAMS）。
要求：
  - 该 Base 已开启高级权限，否则返回 ADVANCED_PERMISSION_DISABLED / USER_ERROR；
  - 调用者必须是该 AI 表格的管理员/Owner，否则返回 401 AUTH_ERROR，
    message: "the current user must be a manager (administrator) of this base to manage roles or advanced permission"。`,
		Example: `  dws aitable advperm role-delete --base-id BASE_ID --role-id ROLE_ID --yes
  # 查询 roleId: dws aitable advperm role-list --base-id <baseId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "role-id"); err != nil {
				return err
			}
			// 先校验所有必填 flag（含 base-id），再进入二次确认提示，避免对无效请求弹无意义的确认。
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			roleID := mustGetFlag(cmd, "role-id")
			if !confirmDelete("角色", roleID) {
				return nil
			}
			return callAitableHelperTool("delete_role", map[string]any{
				"baseId": baseID,
				"roleId": roleID,
			})
		},
	}

	importUploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "准备导入文件上传",
		Long: `为导入任务申请 OSS 直传地址。返回 uploadUrl 和 importId。
客户端应通过 HTTP PUT 将原始文件字节流上传至 uploadUrl。
上传完成后将 importId 传入 import data 即可触发导入。

完整流程:
  1. dws aitable import upload --base-id BASE_ID --file-name data.xlsx --file-size 204800
     → 获取 uploadUrl 和 importId
  2. curl -X PUT "<uploadUrl>" --data-binary @data.xlsx
     → 上传文件到 OSS
  3. dws aitable import data --import-id <importId>
     → 触发导入`,
		Example: `  dws aitable import upload --base-id BASE_ID --file-name data.xlsx --file-size 204800
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "file-name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":   baseID,
				"fileName": mustGetFlag(cmd, "file-name"),
			}
			if v, _ := cmd.Flags().GetInt64("file-size"); v > 0 {
				toolArgs["fileSize"] = v
			}
			return callAitableTool("prepare_import_upload", toolArgs)
		},
	}

	importDataCmd := &cobra.Command{
		Use:   "data",
		Short: "导入数据",
		Long: `将已通过 import upload 上传完成的文件导入 AI 表格。
支持两种模式：
  1. 新建表导入（默认）：不传 --table-id，每个 Sheet 会新建为独立的数据表
  2. 追加导入：传入 --table-id，数据将作为新行追加到该已有表中

工具内部会等待导入完成，大多数情况下一次调用即可拿到最终结果。
若在 timeout 内未完成，再次传入相同 importId 继续等待，无需重新提交任务。

追加导入时的注意事项：
  - 系统按列名自动匹配字段，源文件列名须与目标表字段名一致
  - 若需自定义映射关系，使用 --field-mapping 指定（key=目标表字段名，value=源文件列名）
  - 多 Sheet 文件默认使用第一个 Sheet，可通过 --src-sheet-name 指定`,
		Example: `  # 新建表导入
  dws aitable import data --import-id IMPORT_ID
  # 追加到已有表
  dws aitable import data --import-id IMPORT_ID --table-id TABLE_ID
  # 指定表头行和源 Sheet
  dws aitable import data --import-id IMPORT_ID --table-id TABLE_ID --header-row 2 --src-sheet-name "Sheet1"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "import-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"importId": mustGetFlag(cmd, "import-id"),
			}
			if v, _ := cmd.Flags().GetString("table-id"); v != "" {
				toolArgs["tableId"] = v
			}
			if v, _ := cmd.Flags().GetInt("timeout"); v > 0 {
				toolArgs["timeout"] = v
			}
			if v, _ := cmd.Flags().GetInt("header-row"); v > 0 {
				toolArgs["headerRow"] = v
			}
			if v, _ := cmd.Flags().GetString("src-sheet-name"); v != "" {
				toolArgs["srcSheetName"] = v
			}
			if v, _ := cmd.Flags().GetString("field-mapping"); v != "" {
				var mapping map[string]string
				if err := json.Unmarshal([]byte(v), &mapping); err != nil {
					return fmt.Errorf("--field-mapping must be valid JSON object: %w", err)
				}
				toolArgs["fieldMapping"] = mapping
			}
			return callAitableTool("import_data", toolArgs)
		},
	}

	// ── section: 文件夹与节点管理（导航树组织） ──────────────────────────────

	sectionCmd := &cobra.Command{Use: "section", Short: "文件夹与节点管理", RunE: groupRunE}

	sectionCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文件夹",
		Long: `在指定 Base 下创建文件夹（用于在导航树中组织 table / dashboard，类似文件夹）。
不传 --parent-section-id 或传空字符串表示创建在 Base 根目录下。
--index 为 0-based 位置，不传则追加到末尾。
返回新建文件夹的 sectionId 与 name。`,
		Example: `  dws aitable section create --base-id BASE_ID --name 我的文件夹
  dws aitable section create --base-id BASE_ID --name 子文件夹 --parent-section-id SECTION_ID --index 0
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
				"name":   mustGetFlag(cmd, "name"),
			}
			// 父文件夹 ID：空字符串=根目录是有效语义，只要用户显式传了就透传
			if cmd.Flags().Changed("parent-section-id") {
				v, _ := cmd.Flags().GetString("parent-section-id")
				toolArgs["parentSectionId"] = v
			}
			// index：0 是合法首位，用 -1 作哨兵，>=0 才透传
			if v, _ := cmd.Flags().GetInt("index"); v >= 0 {
				toolArgs["index"] = v
			}
			return callMCPToolOnServer("aitable-helper", "create_section", toolArgs)
		},
	}

	sectionRenameCmd := &cobra.Command{
		Use:   "rename",
		Short: "重命名文件夹",
		Long: `重命名指定文件夹。
必填 --base-id、--section-id、--new-name。
返回重命名后的 sectionId 与新的 name。`,
		Example: `  dws aitable section rename --base-id BASE_ID --section-id SECTION_ID --new-name 新名称
  # 查询 sectionId: dws aitable section list-nodes --base-id BASE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "section-id", "new-name"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":    baseID,
				"sectionId": mustGetFlag(cmd, "section-id"),
				"newName":   mustGetFlag(cmd, "new-name"),
			}
			return callMCPToolOnServer("aitable-helper", "rename_section", toolArgs)
		},
	}

	sectionDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除文件夹",
		Long: `删除指定文件夹。
注意：删除操作不可逆；若文件夹下仍有 table / dashboard，下游会按既有规则处理。
返回被删除的 sectionId。`,
		Example: `  dws aitable section delete --base-id BASE_ID --section-id SECTION_ID
  # 删除前可先确认空文件夹: dws aitable section list-empty --base-id BASE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "section-id"); err != nil {
				return err
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId":    baseID,
				"sectionId": mustGetFlag(cmd, "section-id"),
			}
			return callMCPToolOnServer("aitable-helper", "delete_section", toolArgs)
		},
	}

	sectionReorderCmd := &cobra.Command{
		Use:   "reorder",
		Short: "调整文件夹顺序",
		Long: `在当前父文件夹下调整文件夹的展示顺序。
必填 --base-id、--section-id、--target-index（0-based）。
返回被调整的 sectionId。`,
		Example: `  dws aitable section reorder --base-id BASE_ID --section-id SECTION_ID --target-index 0
  # 查询 sectionId: dws aitable section list-nodes --base-id BASE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "section-id"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("target-index") {
				return fmt.Errorf("--target-index 是必填参数（0-based 目标位置）")
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			targetIndex, _ := cmd.Flags().GetInt("target-index")
			toolArgs := map[string]any{
				"baseId":      baseID,
				"sectionId":   mustGetFlag(cmd, "section-id"),
				"targetIndex": targetIndex,
			}
			return callMCPToolOnServer("aitable-helper", "reorder_section", toolArgs)
		},
	}

	sectionListEmptyCmd := &cobra.Command{
		Use:   "list-empty",
		Short: "列出空文件夹",
		Long: `列出指定 Base 下所有没有任何子节点的文件夹（空文件夹），用于清理或诊断导航树。
返回按 name 升序排列的 [{sectionId, name, parentSectionId}] 列表与总数；
parentSectionId 为空串表示该文件夹在 Base 根目录下。`,
		Example: `  dws aitable section list-empty --base-id BASE_ID
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
			}
			return callMCPToolOnServer("aitable-helper", "list_empty_sections", toolArgs)
		},
	}

	sectionListNodesCmd := &cobra.Command{
		Use:   "list-nodes",
		Short: "列出全部节点",
		Long: `列出指定 Base 当前版本下的全部 nsheet 节点，包括文件夹、AI 表格、表单视图、仪表盘、文档、查询视图等。
返回按 name 升序排列的 [{nodeId, nodeType, parentSectionId, name}] 列表与总数；
parentSectionId 为空串表示该节点在 Base 根目录下。
适合在 section move-node / section reorder 等操作前用来定位节点 ID 与父级关系。`,
		Example: `  dws aitable section list-nodes --base-id BASE_ID
  # 查询 baseId: dws aitable base list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"baseId": baseID,
			}
			return callMCPToolOnServer("aitable-helper", "list_nsheet_nodes", toolArgs)
		},
	}

	sectionMoveNodeCmd := &cobra.Command{
		Use:   "move-node",
		Short: "移动节点",
		Long: `把任意 nsheet 节点（文件夹 / 多维表 / 表单视图 / 仪表盘 / 文档 / 查询视图）移动到目标文件夹下，可选同时调整全局位置。
--new-parent-section-id 传空字符串表示移动到 Base 根目录。
--target-index 为 0-based 全局下标；对文件夹节点会先 move 再调 reorder，中间步骤失败时会返回 MOVE_OK_REORDER_FAILED，可用 section reorder 重试。
服务端会自动识别节点类型，调用方无需区分文件夹与非文件夹。
返回被移动节点的 id、type、目标父级与下标。`,
		Example: `  dws aitable section move-node --base-id BASE_ID --node-id NODE_ID --new-parent-section-id SECTION_ID
  dws aitable section move-node --base-id BASE_ID --node-id NODE_ID --new-parent-section-id "" --target-index 0
  # 查询 nodeId: dws aitable section list-nodes --base-id BASE_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "node-id"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("new-parent-section-id") {
				return fmt.Errorf("--new-parent-section-id 是必填参数（空字符串表示移到 Base 根目录）")
			}
			baseID, err := mustFlagOrFallback(cmd, "base-id", "base")
			if err != nil {
				return err
			}
			newParentSectionID, _ := cmd.Flags().GetString("new-parent-section-id")
			toolArgs := map[string]any{
				"baseId":             baseID,
				"nodeId":             mustGetFlag(cmd, "node-id"),
				"newParentSectionId": newParentSectionID,
			}
			// targetIndex：0 是合法首位，用 -1 作哨兵，>=0 才透传
			if v, _ := cmd.Flags().GetInt("target-index"); v >= 0 {
				toolArgs["targetIndex"] = v
			}
			return callMCPToolOnServer("aitable-helper", "move_nsheet_node", toolArgs)
		},
	}

	// ── 注册命令树（原 init） ─────────────────────────────────────────────

	// base
	baseListCmd.Flags().Int("limit", 0, "每页数量，默认 10，最大 10")
	baseListCmd.Flags().String("cursor", "", "首次不传；传入上次返回的游标继续获取下一页")
	baseSearchCmd.Flags().String("query", "", "Base 名称关键词，建议至少 2 个字符 (必填)")
	baseSearchCmd.Flags().String("keyword", "", "--query alias")
	_ = baseSearchCmd.Flags().MarkHidden("keyword")
	baseSearchCmd.Flags().String("cursor", "", "分页游标，首次不传")
	baseGetCmd.Flags().String("base-id", "", "Base 唯一标识。优先使用 base search / base list 返回值 (必填)")
	baseCreateCmd.Flags().String("name", "", "Base 名称，1-50 字符；会去除首尾空格后校验 (必填)")
	baseCreateCmd.Flags().String("folder-id", "", "目标父节点的 dentryUuid (知识库节点 ID)，也可传入标准节点 URL，MCP 会在创建前解析出实际生效的节点 ID")
	baseCreateCmd.Flags().String("template-id", "", "创建 Base 模板 ID，默认创建一个空 Base。可通过 template search 获取模板")
	baseUpdateCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	baseUpdateCmd.Flags().String("name", "", "新名称，1-50 字符 (必填)")
	baseUpdateCmd.Flags().String("desc", "", "备注文本")
	baseDeleteCmd.Flags().String("base-id", "", "待删除 Base ID。建议先通过 base get 确认目标 (必填)")
	baseDeleteCmd.Flags().String("reason", "", "一句话描述删除的原因")
	baseGetPrimaryDocIdCmd.Flags().String("base-id", "", "Base ID，可通过 list_bases 或 search_bases 获取 (必填)")
	baseGetPrimaryDocIdCmd.Flags().String("table-id", "", "Table ID，可通过 list_tables 或 get_base 获取 (必填)")
	baseGetPrimaryDocIdCmd.Flags().String("record-id", "", "记录 ID (必填)")
	baseCopyCmd.Flags().String("base-id", "", "源 Base ID (必填)")
	baseCopyCmd.Flags().String("target-folder-id", "", "目标文件夹 ID (必填, 不传会复制失败)")
	baseCopyCmd.Flags().Bool("only-struct", false, "是否仅复制结构（不含数据），默认 false 表示完整复制")
	baseCmd.AddCommand(
		baseListCmd, baseSearchCmd, baseGetCmd,
		baseCreateCmd, baseUpdateCmd, baseDeleteCmd,
		baseGetPrimaryDocIdCmd, baseCopyCmd,
	)

	// table
	tableGetCmd.Flags().String("base-id", "", "所属 Base ID（通过 base list / base search 获取）(必填)")
	tableGetCmd.Flags().String("table-ids", "", "待获取详情的 Table ID 列表（通过 base get 获取），逗号分隔，单次最多 10 个；不传则默认返回当前 Base 下全部表。建议优先显式传入，以控制返回体大小，避免上下文突增")
	tableCreateCmd.Flags().String("base-id", "", "目标 Base ID（通过 base list 获取）(必填)")
	tableCreateCmd.Flags().String("name", "", "表格名称，1~100 个字符；不能包含 / \\ ? * [ ] : 等字符 (必填)")
	tableCreateCmd.Flags().String("table-name", "", "--name 的别名")
	_ = tableCreateCmd.Flags().MarkHidden("table-name")
	tableCreateCmd.Flags().String("fields", "[]", "建表时随附创建的初始字段 JSON 数组，至少 1 个，单次最多 15 个。若传空数组 []，系统会自动补一个名为'标题'的 primaryDoc 首列")
	tableUpdateCmd.Flags().String("base-id", "", "所属 Base ID（用于定位目标表）(必填)")
	tableUpdateCmd.Flags().String("table-id", "", "目标 Table ID（通过 base get 获取）(必填)")
	tableUpdateCmd.Flags().String("name", "", "新表名。不能包含 / \\ ? * [ ] : 等特殊字符；与 --description / --record-name-key 三选一")
	tableUpdateCmd.Flags().String("description", "", "更新后的数据表备注说明；与 --name / --record-name-key 三选一")
	tableUpdateCmd.Flags().String("record-name-key", "", "行命名规则枚举键（如 task / project / event / customer 等固定枚举值）；与 --name / --description 三选一")
	tableDeleteCmd.Flags().String("base-id", "", "目标 Base ID（通过 base list 获取）(必填)")
	tableDeleteCmd.Flags().String("table-id", "", "将被删除的 Table ID（通过 base get / get_tables 获取）(必填)")
	tableDeleteCmd.Flags().String("reason", "", "一句话描述一下删除该数据表的原因，用于审计")
	tableCmd.AddCommand(
		tableGetCmd, tableCreateCmd,
		tableUpdateCmd, tableDeleteCmd,
	)

	// dws aitable table list → dws aitable table get (别名命令)
	tableListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取数据表信息（dws aitable table get 的别名）",
		Long: `批量获取指定 Tables（数据表）的表级信息、字段目录与视图目录。
这是 dws aitable table get 的便捷别名，两个命令完全等价。`,
		Example: `  dws aitable table list --base-id BASE_ID
  dws aitable table list --base-id BASE_ID --table-ids tbl1,tbl2`,
		RunE: tableGetCmd.RunE,
	}
	copyFlags(tableGetCmd, tableListCmd, "base-id", "table-ids")
	tableCmd.AddCommand(tableListCmd)

	// field
	fieldGetCmd.Flags().String("base-id", "", "Base ID（可通过 base list 获取）(必填)")
	fieldGetCmd.Flags().String("table-id", "", "Table ID（可通过 base get 获取）(必填)")
	fieldGetCmd.Flags().String("field-ids", "", "待获取详情的字段 ID 列表（通过 table get 获取），逗号分隔；建议只传真正需要展开完整配置的字段，单次最多 10 个；不传则返回全部字段。建议优先显式传入，以控制返回体大小，避免上下文突增")
	fieldCreateCmd.Flags().String("base-id", "", "Base ID（通过 base list 获取）(必填)")
	fieldCreateCmd.Flags().String("table-id", "", "Table ID（通过 base get 获取）(必填)")
	fieldCreateCmd.Flags().String("fields", "", "待新增字段列表 JSON 数组，至少包含 1 个字段，单次最多 15 个。系统会按数组顺序依次创建，返回结果顺序与入参保持一致，并逐项标明成功/失败状态。若是单个字段可直接使用 --name/--type/--config")
	fieldCreateCmd.Flags().String("name", "", "要创建的单字段名称（与 --type 配合使用，替代 --fields）")
	fieldCreateCmd.Flags().String("field-name", "", "--name 的别名（兼容 LLM 常见误用）")
	_ = fieldCreateCmd.Flags().MarkHidden("field-name")
	fieldCreateCmd.Flags().String("type", "", "要创建的单字段类型（需要配合 --name，参考 table create 的内置类型）")
	fieldCreateCmd.Flags().String("field-type", "", "--type 的别名（兼容 LLM 常见误用）")
	_ = fieldCreateCmd.Flags().MarkHidden("field-type")
	fieldCreateCmd.Flags().String("config", "", "单字段的额外配置 JSON（如 options，配合 --name/--type 使用）")
	fieldCreateCmd.Flags().String("ai-config", "", "单字段 AI 配置 JSON（如 outputType/prompt，配合 --name/--type 使用）")
	fieldCreateCmd.Flags().String("options", "", "选项列表 JSON 数组（自动包装为 config.options），与 --name/--type 配合使用")
	_ = fieldCreateCmd.Flags().MarkHidden("options")
	fieldUpdateCmd.Flags().String("base-id", "", "Base ID（可通过 base list 获取）(必填)")
	fieldUpdateCmd.Flags().String("table-id", "", "Table ID（可通过 base get 获取）(必填)")
	fieldUpdateCmd.Flags().String("field-id", "", "Field ID（可通过 table get 获取）(必填)")
	fieldUpdateCmd.Flags().String("name", "", "更新后的字段名称，最大100字。不修改名称时省略")
	fieldUpdateCmd.Flags().String("config", "", "更新后的字段配置 JSON，结构与 field create 的 config 完全一致。不修改配置时省略。更新 singleSelect/multipleSelect 的 options 时需传入完整列表，系统以新列表整体覆盖；已有选项应回传原 id，新增选项无需传 id")
	fieldUpdateCmd.Flags().String("ai-config", "", "更新后的 AI 配置 JSON，不修改 AI 配置时省略（与 MCP update_field.aiConfig 对齐）")
	fieldDeleteCmd.Flags().String("base-id", "", "Base ID（通过 base list 获取）(必填)")
	fieldDeleteCmd.Flags().String("table-id", "", "Table ID（通过 base get 获取）(必填)")
	fieldDeleteCmd.Flags().String("field-id", "", "待删除字段 ID（通过 table get 获取）(必填)")
	fieldSearchOptionsCmd.Flags().String("base-id", "", "Base ID（通过 base list 获取）(必填)")
	fieldSearchOptionsCmd.Flags().String("table-id", "", "Table ID（通过 base get 获取）(必填)")
	fieldSearchOptionsCmd.Flags().String("field-id", "", "目标字段 ID，必须是 singleSelect 或 multipleSelect 类型；通过 table get / field get 获取 (必填)")
	fieldSearchOptionsCmd.Flags().String("keyword", "", "模糊搜索关键词，大小写不敏感，按 contains 匹配 option name；不传则返回全部 options")
	fieldSearchOptionsCmd.Flags().Int("limit", 0, "返回的最大 option 数量，默认 3000（全量返回），最大 3000；传入较小值可减少响应体积")
	fieldCmd.AddCommand(
		fieldGetCmd, fieldCreateCmd,
		fieldUpdateCmd, fieldDeleteCmd,
		fieldSearchOptionsCmd,
	)

	// dws aitable field list → dws aitable field get (别名命令)
	fieldListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取字段信息（dws aitable field get 的别名）",
		Long: `批量获取指定字段的详细信息，包括 fieldId、名称、类型、description 以及类型相关完整配置。
这是 dws aitable field get 的便捷别名，两个命令完全等价。`,
		Example: `  dws aitable field list --base-id BASE_ID --table-id TABLE_ID
  dws aitable field list --base-id BASE_ID --table-id TABLE_ID --field-ids fld1,fld2`,
		RunE: fieldGetCmd.RunE,
	}
	copyFlags(fieldGetCmd, fieldListCmd, "base-id", "table-id", "field-ids")
	fieldCmd.AddCommand(fieldListCmd)

	// record
	recordQueryCmd.Flags().String("base-id", "", "Base ID（通过 base list / base search 获取）(必填)")
	recordQueryCmd.Flags().String("table-id", "", "Table ID（通过 base get 获取）(必填)")
	recordQueryCmd.Flags().String("record-ids", "", "指定要获取的记录 ID 列表，逗号分隔，单次最多 100 个。传入时按 ID 返回，忽略 filters 和 sort。适用于已知 recordId（如关联字段中的 linkedRecordIds）时的精准取数")
	recordQueryCmd.Flags().String("field-ids", "", "指定要返回的字段 ID 列表，逗号分隔。省略则返回所有字段。建议在字段较多时按需传入，可显著减少响应体积；单次最多 100 个")
	recordQueryCmd.Flags().String("filters", "", "结构化过滤条件 JSON，不传则返回全部记录（受 limit 限制）")
	recordQueryCmd.Flags().String("sort", "", "排序条件 JSON 数组，按数组顺序依次生效")
	recordQueryCmd.Flags().String("query", "", "全文关键词。将对整表内容做文本匹配搜索，并返回符合条件的记录")
	recordQueryCmd.Flags().String("keyword", "", "全文关键词 (--query 的别名)")
	_ = recordQueryCmd.Flags().MarkHidden("keyword")
	recordQueryCmd.Flags().Int("limit", 0, "单次返回的最大记录数，默认 100，最大 100")
	recordQueryCmd.Flags().Int("page-size", 0, "--limit 的别名（兼容 LLM 常见误用）")
	_ = recordQueryCmd.Flags().MarkHidden("page-size")
	recordQueryCmd.Flags().String("cursor", "", "分页游标，首次查询不传；cursor 为空表示已取完全部记录")
	recordQueryCmd.Flags().Bool("all", false, "自动翻页获取全部记录。传入时自动循环直到无更多数据或达到 --page-limit 上限")
	recordQueryCmd.Flags().Int("page-limit", 50, "自动翻页最大页数（仅 --all 时生效）。默认 50 页（5000 条），设为 0 表示无限制")
	recordQueryCmd.Flags().String("view-id", "", "视图 ID（record query 不支持按视图过滤，此参数会被忽略并给出提示）")
	_ = recordQueryCmd.Flags().MarkHidden("view-id")
	recordCreateCmd.Flags().String("base-id", "", "Base ID，可通过 base list 或 base search 获取 (必填)")
	recordCreateCmd.Flags().String("table-id", "", "Table ID，可通过 base get 获取 (必填)")
	recordCreateCmd.Flags().String("records", "", "待创建的记录列表 JSON 数组，单次最多 100 条 (必填)")
	recordCreateCmd.Flags().String("records-file", "", "从文件读取 records JSON（替代 --records，适合 Windows 或超长数据）")
	recordCreateCmd.Flags().String("fields", "", "--records 的别名（兼容 LLM 常见误用）")
	_ = recordCreateCmd.Flags().MarkHidden("fields")
	recordCreateCmd.Flags().String("cells", "", "单条记录的 cells JSON 对象（自动构造 --records '[{\"cells\":...}]'）")
	_ = recordCreateCmd.Flags().MarkHidden("cells")
	recordUpdateCmd.Flags().String("base-id", "", "Base ID，可通过 base list 或 base search 获取 (必填)")
	recordUpdateCmd.Flags().String("table-id", "", "Table ID，可通过 base get 获取 (必填)")
	recordUpdateCmd.Flags().String("records", "", "待更新的记录内容列表 JSON 数组，单次最多 100 条 (必填)")
	recordUpdateCmd.Flags().String("records-file", "", "从文件读取 records JSON（替代 --records，适合 Windows 或超长数据）")
	recordUpdateCmd.Flags().String("fields", "", "--records 的别名（兼容 LLM 常见误用）")
	_ = recordUpdateCmd.Flags().MarkHidden("fields")
	recordUpdateCmd.Flags().String("record-id", "", "单条记录 ID（与 --cells 配合使用，自动构造 --records）")
	_ = recordUpdateCmd.Flags().MarkHidden("record-id")
	recordUpdateCmd.Flags().String("cells", "", "单条记录的 cells JSON 对象（与 --record-id 配合使用，自动构造 --records）")
	_ = recordUpdateCmd.Flags().MarkHidden("cells")
	recordDeleteCmd.Flags().String("base-id", "", "Base ID，可通过 base list 或 base search 获取 (必填)")
	recordDeleteCmd.Flags().String("table-id", "", "Table ID，可通过 base get 获取 (必填)")
	recordDeleteCmd.Flags().String("record-ids", "", "待删除的记录 ID 列表，逗号分隔，最多 100 条 (必填)")
	recordBatchUpdateCmd.Flags().String("base-id", "", "Base ID，可通过 base list 或 base search 获取 (必填)")
	recordBatchUpdateCmd.Flags().String("table-id", "", "Table ID，可通过 base get 获取 (必填)")
	recordBatchUpdateCmd.Flags().String("record-ids", "", "待更新的记录 ID 列表，逗号分隔，单次最多 100 条 (必填)")
	recordBatchUpdateCmd.Flags().String("cells", "", "要应用到所有记录的 cells JSON 对象（共享 patch），如 '{\"fldStatusId\":\"已完成\"}' (必填)")
	// record history-list 的 flags（与 record query 不复用，因为入参不同）
	recordHistoryListCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	recordHistoryListCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	recordHistoryListCmd.Flags().String("record-id", "", "目标 Record ID (必填)")
	recordHistoryListCmd.Flags().Int("offset", 0, "分页偏移量，默认 0")
	recordHistoryListCmd.Flags().Int("limit", 0, "每页返回数量，默认 20，最大 50")

	// record query-empty
	recordQueryEmptyCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	recordQueryEmptyCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	recordQueryEmptyCmd.Flags().Int("limit", 0, "单次扫描的最大记录数（扫描预算，非返回数）；范围 [1, 100]，默认 100")
	recordQueryEmptyCmd.Flags().String("cursor", "", "分页游标。首次不传；返回 nextCursor 非空时把它传回继续扫")

	// record share-url
	recordShareUrlCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	recordShareUrlCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	recordShareUrlCmd.Flags().String("record-ids", "", "目标 Record ID 列表，逗号分隔，单次最多 20 条 (必填)")
	recordShareUrlCmd.Flags().String("view-id", "", "视图 ID（可选，用于生成带视图上下文的分享链接）")

	// record upsert：复用 record update 的 records / records-file / fields 三种入参方式
	recordUpsertCmd.Flags().String("base-id", "", "Base ID，可通过 base list 或 base search 获取 (必填)")
	recordUpsertCmd.Flags().String("table-id", "", "Table ID，可通过 base get 获取 (必填)")
	recordUpsertCmd.Flags().String("records", "", "待 upsert 的记录内容列表 JSON 数组，单次最多 100 条；带 recordId 的走更新，不带的走创建 (必填，可改用 --records-file)")
	recordUpsertCmd.Flags().String("records-file", "", "从文件读取 records JSON（避免命令行长度限制）；与 --records 互斥，优先级更高")
	recordUpsertCmd.Flags().String("fields", "", "--records 的别名 (兼容旧用法)")
	_ = recordUpsertCmd.Flags().MarkHidden("fields")

	// record primary-doc-get
	recordPrimaryDocGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	recordPrimaryDocGetCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	recordPrimaryDocGetCmd.Flags().String("record-id", "", "目标 Record ID (必填)")

	// record primary-doc-create
	recordPrimaryDocCreateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	recordPrimaryDocCreateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	recordPrimaryDocCreateCmd.Flags().String("field-id", "", "主键字段 ID，必须是 primaryDoc 类型 (必填)")
	recordPrimaryDocCreateCmd.Flags().String("record-id", "", "目标 Record ID (必填)")

	recordCmd.AddCommand(
		recordQueryCmd, recordCreateCmd,
		recordUpdateCmd, recordDeleteCmd,
		recordBatchUpdateCmd,
		recordHistoryListCmd,
		recordShareUrlCmd,
		recordUpsertCmd,
		recordQueryEmptyCmd,
		recordPrimaryDocGetCmd,
		recordPrimaryDocCreateCmd,
	)

	// dws aitable record list → dws aitable record query (别名命令)
	recordListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取行记录（dws aitable record query 的别名）",
		Long: `查询指定表格中的记录。
这是 dws aitable record query 的便捷别名,两个命令完全等价。`,
		Example: `  dws aitable record list --base-id BASE_ID --table-id TABLE_ID
  dws aitable record list --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2`,
		RunE: recordQueryCmd.RunE, // 复用 recordQueryCmd 的执行逻辑
	}
	cli.AnnotateRuntimeCompatibilityEquivalence(recordQueryCmd, recordListCmd, cli.RuntimeCompatibilityEquivalence{
		ID:       "aitable.record-query-list-v1",
		Reason:   "The compatibility leaf reuses the exact record query handler and flag surface; it only preserves the historical list spelling.",
		Reviewed: true,
	})
	// 复用 recordQueryCmd 的 flags
	copyFlags(recordQueryCmd, recordListCmd, "base-id", "base", "table-id", "record-ids", "field-ids", "filters", "sort", "query", "keyword", "limit", "cursor", "page-size", "all", "page-limit", "view-id")
	_ = recordListCmd.Flags().MarkHidden("keyword")
	recordCmd.AddCommand(recordListCmd)

	// dws aitable record get → 按 recordId 取记录（record query --record-ids 的窄别名，强制必填 record-ids）
	recordGetCmd := &cobra.Command{
		Use:   "get",
		Short: "按 ID 获取记录（record query --record-ids 的便捷别名，单次最多 100 条）",
		Long: `按 recordId 精确获取一条或多条记录。
这是 dws aitable record query --record-ids 的便捷别名；强制 record-ids 必填，未暴露 filters/sort/query/cursor/limit 等查询参数。
如需关键词搜索、筛选、排序、自动翻页，请改用 dws aitable record query 或 dws aitable record list。

--field-ids 同样支持，逗号分隔，单次最多 100 个；用于控制返回字段或显式拉取计算字段。
公式（formula）、查找引用（filterUp）、关联引用（lookup）字段默认不返回值，
需在 --field-ids 中显式指定字段 ID 才会返回。`,
		Example: `  dws aitable record get --base-id BASE_ID --table-id TABLE_ID --record-ids rec1
  dws aitable record get --base-id BASE_ID --table-id TABLE_ID --record-ids rec1,rec2,rec3
  dws aitable record get --base-id BASE_ID --table-id TABLE_ID --record-ids rec1 --field-ids fldFormulaId,fldLookupId`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "table-id", "record-ids"); err != nil {
				return err
			}
			return recordQueryCmd.RunE(cmd, args)
		},
	}
	copyFlags(recordQueryCmd, recordGetCmd, "base-id", "table-id", "record-ids", "field-ids")
	recordCmd.AddCommand(recordGetCmd)

	// template
	templateSearchCmd.Flags().String("query", "", "模板名称关键词 (必填)")
	templateSearchCmd.Flags().String("keyword", "", "--query alias")
	_ = templateSearchCmd.Flags().MarkHidden("keyword")
	templateSearchCmd.Flags().Int("limit", 0, "每页返回数量。默认 10，最大 30")
	templateSearchCmd.Flags().String("cursor", "", "分页游标。首次请求不传；后续请原样传入上次返回的 nextCursor")
	templateCmd.AddCommand(templateSearchCmd)

	// attachment
	attachmentUploadCmd.Flags().String("base-id", "", "Base ID，可通过 base list 或 base search 获取 (必填)")
	attachmentUploadCmd.Flags().String("file-name", "", "待上传的文件名，必须包含扩展名（如 report.xlsx、photo.png）(必填)")
	attachmentUploadCmd.Flags().Int64("size", 0, "文件大小（字节），必须大于 0 (必填)")
	attachmentUploadCmd.Flags().String("mime-type", "", "文件 MIME type（如 image/png），不传时根据扩展名推断")
	attachmentCmd.AddCommand(attachmentUploadCmd)

	// view
	viewGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	viewGetCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	viewGetCmd.Flags().String("view-ids", "", "待获取详情的 View ID 列表，逗号分隔，单次最多 10 个；不传则返回全部视图")
	viewCreateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	viewCreateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	viewCreateCmd.Flags().String("view-type", "", "视图类型：Grid、FormDesigner、Gantt、Calendar、Kanban、Gallery (必填)")
	viewCreateCmd.Flags().String("view-sub-type", "", "视图子类型，可选")
	viewCreateCmd.Flags().String("name", "", "视图名称，未传时自动生成")
	viewCreateCmd.Flags().String("desc", "", "视图描述 JSON，如 {\"content\":[]}")
	viewCreateCmd.Flags().String("config", "", "视图配置 JSON（含 visibleFieldIds、filter、sort、group 等）")
	viewUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	viewUpdateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	viewUpdateCmd.Flags().String("view-id", "", "目标 View ID (必填)")
	viewUpdateCmd.Flags().String("name", "", "新的视图名称")
	viewUpdateCmd.Flags().String("desc", "", "新的视图描述 JSON，不修改时省略；如需清空可传 {\"content\":[]}")
	viewUpdateCmd.Flags().String("config", "", "视图配置更新项 JSON（含 visibleFieldIds、filter、sort、group、fieldWidths 等）")
	viewDeleteCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	viewDeleteCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	viewDeleteCmd.Flags().String("view-id", "", "要删除的 View ID (必填)")

	// view get <attr> 子命令的公共 flag（base-id / table-id / view-id）
	for _, sub := range []*cobra.Command{
		viewGetCardCmd, viewGetTimebarCmd, viewGetAggregateCmd,
		viewGetFilterCmd, viewGetSortCmd, viewGetGroupCmd,
		viewGetVisibleFieldsCmd, viewGetFieldWidthsCmd,
		viewGetLockCmd, viewGetFrozenColsCmd, viewGetRowHeightCmd, viewGetFillColorRuleCmd,
	} {
		sub.Flags().String("base-id", "", "所属 Base ID (必填)")
		sub.Flags().String("table-id", "", "所属 Table ID (必填)")
		sub.Flags().String("view-id", "", "目标 View ID (必填)")
	}

	// view update <attr> 子命令的公共 flag + typed flag
	for _, sub := range []*cobra.Command{
		viewUpdateCardCmd, viewUpdateTimebarCmd, viewUpdateAggregateCmd,
		viewUpdateFieldWidthsCmd, viewUpdateVisibleFieldsCmd,
		viewUpdateFilterCmd, viewUpdateSortCmd, viewUpdateGroupCmd,
		viewUpdateNameCmd,
		viewUpdateFrozenColsCmd, viewUpdateRowHeightCmd, viewUpdateFillColorRuleCmd,
	} {
		sub.Flags().String("base-id", "", "所属 Base ID (必填)")
		sub.Flags().String("table-id", "", "所属 Table ID (必填)")
		sub.Flags().String("view-id", "", "目标 View ID (必填)")
	}
	// card: Kanban / Gallery 共享 cover-field-id / cover-resize-mode；其他独占字段也都暴露
	viewUpdateCardCmd.Flags().String("cover-field-id", "", "封面字段 ID (Kanban / Gallery 通用)")
	viewUpdateCardCmd.Flags().Bool("no-cover", false, "清除封面 (Kanban / Gallery 通用)；与 --cover-field-id 互斥")
	viewUpdateCardCmd.Flags().String("cover-resize-mode", "", "封面缩放: cover|contain|stretch")
	viewUpdateCardCmd.Flags().Bool("hidden-field-title", false, "隐藏字段名标题 (仅 Kanban)")
	viewUpdateCardCmd.Flags().String("cover-mode", "", "封面模式 (仅 Gallery): none|auto|custom")
	viewUpdateCardCmd.Flags().Bool("display-field-name", false, "是否显示字段名 (仅 Gallery)")
	viewUpdateCardCmd.Flags().String("json", "", "完整 card 子块 JSON，与 typed flag 同时存在时 typed flag 优先")
	// timebar: Gantt 专属
	viewUpdateTimebarCmd.Flags().String("start-field", "", "开始日期字段 ID")
	viewUpdateTimebarCmd.Flags().String("end-field", "", "结束日期字段 ID")
	viewUpdateTimebarCmd.Flags().String("display-field-id", "", "时间条上显示的标题字段 ID")
	viewUpdateTimebarCmd.Flags().String("timeline-scale", "", "时间尺度: year|quarter|month|weeks")
	viewUpdateTimebarCmd.Flags().String("color-configs", "", "颜色配置 JSON 数组")
	viewUpdateTimebarCmd.Flags().Bool("official-holiday", false, "是否标注法定节假日")
	viewUpdateTimebarCmd.Flags().String("json", "", "完整 ganttTimebar 子块 JSON")
	// aggregate: Grid 专属
	viewUpdateAggregateCmd.Flags().String("field-id", "", "单字段 ID（配合 --action 写入单个聚合）")
	viewUpdateAggregateCmd.Flags().String("action", "", "聚合 action: SUM|AVG|MAX|MIN|MEDIAN|RANGE|...（配合 --field-id）")
	viewUpdateAggregateCmd.Flags().String("clear-field-id", "", "清除聚合的字段 ID 列表 (CSV)")
	viewUpdateAggregateCmd.Flags().String("json", "", "完整 aggregate map JSON")
	// field-widths: Grid 专属
	viewUpdateFieldWidthsCmd.Flags().String("field-id", "", "单字段 ID（配合 --width）")
	viewUpdateFieldWidthsCmd.Flags().Int("width", 0, "字段列宽（配合 --field-id）")
	viewUpdateFieldWidthsCmd.Flags().String("json", "", "完整 fieldWidths map JSON")
	// visible-fields: 通用
	viewUpdateVisibleFieldsCmd.Flags().String("field-ids", "", "可见字段 ID 列表 (CSV)，整组替换原有顺序")
	viewUpdateVisibleFieldsCmd.Flags().String("json", "", "可见字段 ID 数组 JSON")
	// filter / sort / group: 仅 --json
	viewUpdateFilterCmd.Flags().String("json", "", "filter 数组 JSON")
	viewUpdateSortCmd.Flags().String("json", "", "sort 数组 JSON")
	viewUpdateGroupCmd.Flags().String("json", "", "group 数组 JSON")
	// name
	viewUpdateNameCmd.Flags().String("name", "", "新视图名称 (必填)")
	// frozen-cols / row-height / fill-color-rule typed flags
	viewUpdateFrozenColsCmd.Flags().Int("count", 0, "冻结列数（>=0；0 表示取消冻结）(必填)")
	viewUpdateRowHeightCmd.Flags().Int("cell-height", 0, "单元格高度（像素），合法档位 32 / 56 / 88 / 128 (必填)")
	viewUpdateFillColorRuleCmd.Flags().String("json", "", "conditionalFormats JSON 数组（整组替换；传 [] 清空）(必填)")
	// view lock / view duplicate（顶层独立子命令）
	viewLockCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	viewLockCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	viewLockCmd.Flags().String("view-id", "", "目标 View ID (必填)")
	viewLockCmd.Flags().Bool("off", false, "解锁视图（不传则锁定）")
	viewDuplicateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	viewDuplicateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	viewDuplicateCmd.Flags().String("view-id", "", "源 View ID (必填)")
	viewDuplicateCmd.Flags().String("new-name", "", "新视图名称（不传则由 server 默认命名）")

	viewCmd.AddCommand(
		viewGetCmd, viewCreateCmd,
		viewUpdateCmd, viewDeleteCmd,
		viewLockCmd, viewDuplicateCmd,
	)

	// dws aitable view list → dws aitable view get (别名命令)
	viewListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取视图信息（dws aitable view get 的别名）",
		Long: `批量获取指定 Table 下的视图配置（含 viewId、viewName、type、filter、sort、group、visibleFieldIds、fieldWidths 等）。
这是 dws aitable view get 的便捷别名，两个命令完全等价；不传 --view-ids 时返回当前表全部视图。`,
		Example: `  dws aitable view list --base-id BASE_ID --table-id TABLE_ID
  dws aitable view list --base-id BASE_ID --table-id TABLE_ID --view-ids viw1,viw2`,
		RunE: viewGetCmd.RunE,
	}
	copyFlags(viewGetCmd, viewListCmd, "base-id", "table-id", "view-ids")
	viewCmd.AddCommand(viewListCmd)

	// form
	formListCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formListCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formGetCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formGetCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formCreateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formCreateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formCreateCmd.Flags().String("name", "", "新表单名称 (必填)")
	formCreateCmd.Flags().String("description", "", "表单描述（创建后可用 form update 调整）")
	formDeleteCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formDeleteCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formDeleteCmd.Flags().String("view-id", "", "目标表单视图 ID（通过 form list 获取）(必填)")
	formUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formUpdateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formUpdateCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formUpdateCmd.Flags().String("title", "", "表单标题（与 --name 等价，二选一）")
	formUpdateCmd.Flags().String("name", "", "表单标题（与 --title 等价）")
	formUpdateCmd.Flags().String("description", "", "表单描述")
	// form questions create/delete 复用 field create/delete 的 flag（保持入参完全一致）
	copyFlags(fieldCreateCmd, formQuestionsCreateCmd, "base-id", "table-id", "fields", "name", "type", "config", "ai-config", "field-name", "field-type")
	copyFlags(fieldDeleteCmd, formQuestionsDeleteCmd, "base-id", "table-id", "field-id")
	formFieldListCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formFieldListCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formFieldListCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formFieldUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formFieldUpdateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formFieldUpdateCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formFieldUpdateCmd.Flags().String("field-id", "", "目标字段 ID（通过 form field list 获取）(必填)")
	formFieldUpdateCmd.Flags().String("required", "", "设置字段在表单中的必填状态 (true/false)")
	formFieldUpdateCmd.Flags().String("field-description", "", "设置字段在表单中的描述文案")
	formFieldHideCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formFieldHideCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formFieldHideCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formFieldHideCmd.Flags().String("field-id", "", "目标字段 ID (必填)")
	formFieldHideCmd.Flags().String("hidden", "", "true 隐藏字段，false 显示字段")
	formShareGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formShareGetCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formShareGetCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formShareUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	formShareUpdateCmd.Flags().String("table-id", "", "所属 Table ID (必填)")
	formShareUpdateCmd.Flags().String("view-id", "", "目标表单视图 ID (必填)")
	formShareUpdateCmd.Flags().String("enabled", "", "分享开关：true 开启，false 关闭 (必填)")
	formFieldCmd.AddCommand(formFieldListCmd, formFieldUpdateCmd, formFieldHideCmd)
	formShareCmd.AddCommand(formShareGetCmd, formShareUpdateCmd)
	formQuestionsCmd.AddCommand(formQuestionsCreateCmd, formQuestionsDeleteCmd)
	formCmd.AddCommand(formListCmd, formGetCmd, formCreateCmd, formDeleteCmd, formUpdateCmd, formFieldCmd, formShareCmd, formQuestionsCmd)

	// workflow
	workflowCreateCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	workflowCreateCmd.Flags().String("dsl", "", "workflow-dsl/v1 JSON 对象；支持内联 JSON、@文件路径或 - 从 stdin 读取 (必填)")
	workflowCreateCmd.Flags().String("locale", "", "请求语言，例如 zh-CN 或 zh_CN (可选)")
	workflowUpdateCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	workflowUpdateCmd.Flags().String("workflow-id", "", "目标工作流 ID (必填)")
	workflowUpdateCmd.Flags().String("dsl", "", "workflow-dsl/v1 JSON 对象；支持内联 JSON、@文件路径或 - 从 stdin 读取 (必填)")
	workflowUpdateCmd.Flags().String("locale", "", "请求语言，例如 zh-CN 或 zh_CN (可选)")
	workflowEnableCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	workflowEnableCmd.Flags().String("workflow-id", "", "目标工作流 ID (必填)")
	workflowDisableCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	workflowDisableCmd.Flags().String("workflow-id", "", "目标工作流 ID (必填)")
	workflowGetCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	workflowGetCmd.Flags().String("workflow-id", "", "目标工作流 ID (必填)")
	workflowListCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	workflowListCmd.Flags().Int("limit", 0, "分页大小 [1, 100]，不传走服务端默认 20")
	workflowListCmd.Flags().Int("offset", 0, "分页偏移量，>= 0，不传走服务端默认 0")
	workflowCmd.AddCommand(
		workflowCreateCmd, workflowUpdateCmd,
		workflowEnableCmd, workflowDisableCmd,
		workflowGetCmd, workflowListCmd,
	)

	// dashboard
	dashboardGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardGetCmd.Flags().String("dashboard-id", "", "目标 Dashboard ID（通过 base get 获取）(必填)")
	dashboardCreateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardCreateCmd.Flags().String("config", "", "Dashboard 配置 JSON，结构参考 dashboard config-example。可用 --name 替代来快速创建空看板")
	dashboardCreateCmd.Flags().String("name", "", "新仪表盘名称（替代 --config 简化版创建空看板）")
	dashboardUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardUpdateCmd.Flags().String("dashboard-id", "", "目标 Dashboard ID (必填)")
	dashboardUpdateCmd.Flags().String("config", "", "Dashboard 配置更新项 JSON。可选，若只需改名可以用 --name 代替")
	dashboardUpdateCmd.Flags().String("name", "", "要修改的新看板名称（替代 --config 简化操作）")
	dashboardDeleteCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardDeleteCmd.Flags().String("dashboard-id", "", "目标 Dashboard ID (必填)")
	dashboardDeleteCmd.Flags().String("reason", "", "删除原因")
	dashboardArrangeCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardArrangeCmd.Flags().String("dashboard-id", "", "目标 Dashboard ID (必填)")
	dashboardShareGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardShareGetCmd.Flags().String("dashboard-id", "", "目标 Dashboard ID (必填)")
	dashboardShareUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	dashboardShareUpdateCmd.Flags().String("dashboard-id", "", "目标 Dashboard ID (必填)")
	dashboardShareUpdateCmd.Flags().String("enabled", "", "分享开关：true 开启，false 关闭 (必填)")
	dashboardShareUpdateCmd.Flags().String("share-type", "", "分享类型：PUBLIC 或 ORG（enabled=true 时生效）")
	dashboardShareUpdateCmd.Flags().Bool("allow-back-to-doc", false, "是否允许从分享页返回源 AI 表格（仅在显式传参时生效）")
	dashboardShareCmd.AddCommand(dashboardShareGetCmd, dashboardShareUpdateCmd)
	dashboardCmd.AddCommand(
		dashboardConfigExampleCmd, dashboardGetCmd,
		dashboardCreateCmd, dashboardUpdateCmd, dashboardDeleteCmd,
		dashboardArrangeCmd,
		dashboardShareCmd,
	)

	// chart
	chartGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	chartGetCmd.Flags().String("dashboard-id", "", "所属 Dashboard ID (必填)")
	chartGetCmd.Flags().String("chart-id", "", "目标 Chart ID（通过 dashboard get 获取）(必填)")
	chartCreateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	chartCreateCmd.Flags().String("dashboard-id", "", "所属 Dashboard ID (必填)")
	chartCreateCmd.Flags().String("config", "", "图表配置 JSON，结构参考 chart widgets-example (必填)")
	chartCreateCmd.Flags().String("layout", "", "图表布局 JSON，如 {\"x\":0,\"y\":0,\"w\":6,\"h\":4} (必填)")
	chartUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	chartUpdateCmd.Flags().String("dashboard-id", "", "所属 Dashboard ID (必填)")
	chartUpdateCmd.Flags().String("chart-id", "", "目标 Chart ID (必填)")
	chartUpdateCmd.Flags().String("config", "", "图表配置 JSON (必填)")
	chartUpdateCmd.Flags().String("layout", "", "图表布局更新 JSON")
	chartDeleteCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	chartDeleteCmd.Flags().String("dashboard-id", "", "所属 Dashboard ID (必填)")
	chartDeleteCmd.Flags().String("chart-id", "", "目标 Chart ID (必填)")
	chartDeleteCmd.Flags().String("reason", "", "删除原因")
	chartShareGetCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	chartShareGetCmd.Flags().String("dashboard-id", "", "所属 Dashboard ID (必填)")
	chartShareGetCmd.Flags().String("chart-id", "", "目标 Chart ID (必填)")
	chartShareUpdateCmd.Flags().String("base-id", "", "所属 Base ID (必填)")
	chartShareUpdateCmd.Flags().String("dashboard-id", "", "所属 Dashboard ID (必填)")
	chartShareUpdateCmd.Flags().String("chart-id", "", "目标 Chart ID (必填)")
	chartShareUpdateCmd.Flags().String("enabled", "", "分享开关：true 开启，false 关闭 (必填)")
	chartShareUpdateCmd.Flags().String("share-type", "", "分享类型：PUBLIC 或 ORG（enabled=true 时生效）")
	chartShareUpdateCmd.Flags().Bool("allow-back-to-doc", false, "是否允许从分享页返回源 AI 表格（仅在显式传参时生效）")
	chartShareCmd.AddCommand(chartShareGetCmd, chartShareUpdateCmd)
	chartCmd.AddCommand(
		chartWidgetsExampleCmd, chartGetCmd,
		chartCreateCmd, chartUpdateCmd, chartDeleteCmd,
		chartShareCmd,
	)

	// export
	exportDataCmd.Flags().String("base-id", "", "Base ID (必填)")
	exportDataCmd.Flags().String("scope", "", "导出范围：all（整个 Base）、table（指定数据表）、view（指定视图）")
	exportDataCmd.Flags().String("export-format", "", "导出格式：excel、attachment、excel_and_attachment、excel_with_inline_images")
	exportDataCmd.Flags().String("task-id", "", "已有导出任务 ID，传入后继续等待（不要同时提供 scope/export-format/table-id/view-id）")
	exportDataCmd.Flags().String("table-id", "", "Table ID，scope=table 或 scope=view 时必填")
	exportDataCmd.Flags().String("view-id", "", "View ID，scope=view 时必填")
	exportDataCmd.Flags().Int("timeout-ms", 0, "单次等待超时（毫秒），默认 30000，最大 30000")
	exportCmd.AddCommand(exportDataCmd)

	// import
	importUploadCmd.Flags().String("base-id", "", "Base ID (必填)")
	importUploadCmd.Flags().String("file-name", "", "文件名，须带扩展名，如 data.xlsx (必填)")
	importUploadCmd.Flags().Int64("file-size", 0, "文件大小（字节数）(必填)")
	importDataCmd.Flags().String("import-id", "", "prepare_import_upload 返回的 importId (必填)")
	importDataCmd.Flags().String("table-id", "", "目标数据表 ID。传入后数据将作为新行追加到该表中；不传则默认新建表导入")
	importDataCmd.Flags().Int("timeout", 0, "最长等待时间（秒），默认且推荐使用最大值 30")
	importDataCmd.Flags().Int("header-row", 0, "表头所在行号（从 1 开始），数据从 headerRow 的下一行开始读取。不传则自动识别表头行")
	importDataCmd.Flags().String("src-sheet-name", "", "源文件中的 Sheet 名称。多 Sheet 文件时指定从哪个 Sheet 导入数据。不传则默认使用第一个 Sheet")
	importDataCmd.Flags().String("field-mapping", "", "字段映射关系 JSON 对象。key 为目标表的字段名，value 为源文件中的列名。不传则按列名自动匹配")
	importCmd.AddCommand(importUploadCmd, importDataCmd)

	// advperm: 高级权限 / 角色
	advpermEnableCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermDisableCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermRoleListCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermRoleGetCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermRoleGetCmd.Flags().String("role-id", "", "目标角色 ID (字符串形态的 long 数字) (必填)")
	advpermRoleCreateCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermRoleCreateCmd.Flags().String("name", "", "角色名称 (必填)")
	advpermRoleCreateCmd.Flags().String("role-type", "", "角色类型 (留空由下游决定默认值，如 custom)")
	advpermRoleCreateCmd.Flags().String("flow-type", "", "流程类型 (按业务需要填写，留空表示无流程绑定)")
	advpermRoleCreateCmd.Flags().String("sub-roles", "", "子角色配置 JSON 数组：[{targetId,targetType,authLevel,appId?,config?}]")
	advpermRoleUpdateCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermRoleUpdateCmd.Flags().String("role-id", "", "目标自定义角色 ID (必填，系统角色禁止更新)")
	advpermRoleUpdateCmd.Flags().String("name", "", "新角色名称 (可选，不传不修改)")
	advpermRoleUpdateCmd.Flags().String("role-type", "", "角色类型 (可选)")
	advpermRoleUpdateCmd.Flags().String("flow-type", "", "流程类型 (可选)")
	advpermRoleUpdateCmd.Flags().String("sub-roles", "", "子角色配置 JSON 数组，PATCH 语义：按 (targetId,targetType) 合并入现有 subRoles，未提及的 sub 保留")
	advpermRoleDeleteCmd.Flags().String("base-id", "", "目标 Base ID (必填)")
	advpermRoleDeleteCmd.Flags().String("role-id", "", "目标自定义角色 ID（系统角色禁删） (必填)")
	advpermCmd.AddCommand(
		advpermEnableCmd, advpermDisableCmd,
		advpermRoleListCmd, advpermRoleGetCmd,
		advpermRoleCreateCmd, advpermRoleUpdateCmd,
		advpermRoleDeleteCmd,
	)

	// section
	sectionCreateCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionCreateCmd.Flags().String("name", "", "文件夹名称 (必填)")
	sectionCreateCmd.Flags().String("parent-section-id", "", "父文件夹 ID；不传或空字符串表示创建在 Base 根目录下")
	sectionCreateCmd.Flags().Int("index", -1, "在父文件夹下的目标位置（0-based）；不传则追加到末尾")
	sectionRenameCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionRenameCmd.Flags().String("section-id", "", "目标文件夹 ID (必填)")
	sectionRenameCmd.Flags().String("new-name", "", "新的文件夹名称 (必填)")
	sectionDeleteCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionDeleteCmd.Flags().String("section-id", "", "目标文件夹 ID (必填)")
	sectionReorderCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionReorderCmd.Flags().String("section-id", "", "目标文件夹 ID (必填)")
	sectionReorderCmd.Flags().Int("target-index", -1, "目标位置（0-based）(必填)")
	sectionListEmptyCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionListNodesCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionMoveNodeCmd.Flags().String("base-id", "", "Base ID (必填)")
	sectionMoveNodeCmd.Flags().String("node-id", "", "要移动的节点 ID，可以是文件夹、AI表格、表单视图、仪表盘、文档或查询视图 (必填)")
	sectionMoveNodeCmd.Flags().String("new-parent-section-id", "", "目标父文件夹 ID；空字符串表示移到 Base 根目录 (必填)")
	sectionMoveNodeCmd.Flags().Int("target-index", -1, "Base 内节点的全局位置（0-based）；不传则不调整")
	sectionCmd.AddCommand(
		sectionCreateCmd, sectionRenameCmd, sectionDeleteCmd,
		sectionReorderCmd, sectionListEmptyCmd, sectionListNodesCmd,
		sectionMoveNodeCmd,
	)

	// 组装 aitable 命令树
	root.AddCommand(
		baseCmd, tableCmd, fieldCmd,
		recordCmd, viewCmd, formCmd,
		workflowCmd,
		dashboardCmd, chartCmd,
		exportCmd, importCmd,
		attachmentCmd, templateCmd,
		advpermCmd,
		sectionCmd,
	)

	// 批量注册 --base 作为 --base-id 的隐藏别名
	registerHiddenAlias := func(cmd *cobra.Command) {
		if cmd.Flags().Lookup("base-id") != nil && cmd.Flags().Lookup("base") == nil {
			cmd.Flags().String("base", "", "--base-id 的别名")
			_ = cmd.Flags().MarkHidden("base")
		}
	}
	for _, sub := range root.Commands() {
		registerHiddenAlias(sub)
		for _, child := range sub.Commands() {
			registerHiddenAlias(child)
			for _, grandchild := range child.Commands() {
				registerHiddenAlias(grandchild)
			}
		}
	}

	// dws aitable search → dws aitable base search (真正的别名命令)
	searchAliasCmd := &cobra.Command{
		Use:   "search",
		Short: "AI 表格搜索（dws aitable base search 的别名）",
		Long: `按名称关键词搜索 AI 表格 Base。
这是 dws aitable base search 的便捷别名,两个命令完全等价。`,
		Example: `  dws aitable search --query "项目管理"
  dws aitable search --keyword "评测项目管理"`,
		RunE: baseSearchCmd.RunE, // 直接复用 baseSearchCmd 的执行逻辑
	}
	// 复用 baseSearchCmd 的 flags
	copyFlags(baseSearchCmd, searchAliasCmd, "query", "keyword", "cursor")
	_ = searchAliasCmd.Flags().MarkHidden("keyword")
	root.AddCommand(searchAliasCmd)

	// dws aitable list → dws aitable base list (别名命令)
	listAliasCmd := &cobra.Command{
		Use:   "list",
		Short: "获取 AI 表格列表（dws aitable base list 的别名）",
		Long: `列出当前用户可访问的 AI 表格 Base。
这是 dws aitable base list 的便捷别名,两个命令完全等价。`,
		Example: `  dws aitable list
  dws aitable list --limit 5`,
		RunE: baseListCmd.RunE, // 复用 baseListCmd 的执行逻辑
	}
	// 复用 baseListCmd 的 flags
	copyFlags(baseListCmd, listAliasCmd, "limit", "cursor")
	root.AddCommand(listAliasCmd)

	// dws aitable create → dws aitable base create (别名命令)
	createAliasCmd := &cobra.Command{
		Use:   "create",
		Short: "创建 AI 表格（dws aitable base create 的别名）",
		Long: `创建一个新的 AI 表格 Base。
这是 dws aitable base create 的便捷别名,两个命令完全等价。`,
		Example: `  dws aitable create --name "项目跟踪"
  dws aitable create --name "项目跟踪" --folder-id FOLDER_ID`,
		RunE: baseCreateCmd.RunE, // 复用 baseCreateCmd 的执行逻辑
	}
	// 独立注册 flags（不能用 copyFlags 共享指针，cobra 不支持同一 flag 绑多个命令）
	createAliasCmd.Flags().String("name", "", "Base 名称，1-50 字符；会去除首尾空格后校验 (必填)")
	createAliasCmd.Flags().String("folder-id", "", "目标父节点的 dentryUuid (知识库节点 ID)，也可传入标准节点 URL，MCP 会在创建前解析出实际生效的节点 ID")
	createAliasCmd.Flags().String("template-id", "", "创建 Base 模板 ID，默认创建一个空 Base。可通过 template search 获取模板")
	root.AddCommand(createAliasCmd)

	// dws aitable info → dws aitable base get (别名命令)
	infoAliasCmd := &cobra.Command{
		Use:   "info",
		Short: "获取 AI 表格信息（dws aitable base get 的别名）",
		Long: `获取指定 Base 的资源目录级信息。
这是 dws aitable base get 的便捷别名,两个命令完全等价。`,
		Example: `  dws aitable info --base-id BASE_ID`,
		RunE:    baseGetCmd.RunE, // 复用 baseGetCmd 的执行逻辑
	}
	// 独立注册 flags（不能用 copyFlags 共享指针，cobra 不支持同一 flag 绑多个命令）
	infoAliasCmd.Flags().String("base-id", "", "Base 唯一标识。优先使用 base search / base list 返回值 (必填)")
	root.AddCommand(infoAliasCmd)
	// hint: dws aitable doc search → dws aitable base search
	root.AddCommand(hintSubCmd("doc", "use: dws aitable base search --query <关键词>"))
	// NOTE: "create" and "info" are registered as real alias commands above
	// (createAliasCmd / infoAliasCmd), do NOT add hintSubCmd with the same
	// Use name — cobra silently replaces the earlier command.

	return root
}

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// drive list --type/--start/--end 客户端过滤（CLI 侧筛）
//
// 两路由统一（拍板⑤）：钉盘与知识库均为全量拉取后在进程内筛——
// 复用 depth/latest 的 BFS 基建（钉盘单层 = depth=1 退化态，全量翻完当前目录不下钻），
// 过滤挂在 emitDriveDepthResult 管线内（pattern 名称筛之后、latest Top-N 之前）。
// 类型判定复用 route.isFolder 反判；时间比较直读 BFS 收集段注入的 sortTime 毫秒。
// ──────────────────────────────────────────────────────────

type driveListFilter struct {
	nodeType string // ""=不过滤 / "file" / "folder"
	startMs  int64
	endMs    int64
	hasStart bool
	hasEnd   bool
}

func (f driveListFilter) active() bool {
	return f.nodeType != "" || f.hasStart || f.hasEnd
}

// parseDriveListFilter 读取并校验 --type/--start/--end。
// 三 flag 均未给值时返回零值（active()=false，调用方走存量分支）。
// 非法值 / start>end / 互斥组合在此拒绝（退出码 3，风格对齐 drive_latest.go 互斥条款）。
func parseDriveListFilter(cmd *cobra.Command) (driveListFilter, error) {
	var filter driveListFilter
	nodeType, _ := cmd.Flags().GetString("type")
	startRaw, _ := cmd.Flags().GetString("start")
	endRaw, _ := cmd.Flags().GetString("end")
	nodeType = strings.TrimSpace(nodeType)
	startRaw = strings.TrimSpace(startRaw)
	endRaw = strings.TrimSpace(endRaw)
	if nodeType == "" && startRaw == "" && endRaw == "" {
		return filter, nil
	}

	if nodeType != "" {
		switch strings.ToLower(nodeType) {
		case "file", "folder":
			filter.nodeType = strings.ToLower(nodeType)
		default:
			return filter, &CLIError{
				Code:    CodeInvalidParam,
				Message: fmt.Sprintf("--type 取值非法: %q（合法值: file|folder）", nodeType),
			}
		}
	}
	if startRaw != "" {
		ms, err := parseDriveListTime(startRaw)
		if err != nil {
			return filter, &CLIError{Code: CodeInvalidParam, Message: fmt.Sprintf("--start %s", err)}
		}
		filter.startMs = ms
		filter.hasStart = true
	}
	if endRaw != "" {
		ms, err := parseDriveListTime(endRaw)
		if err != nil {
			return filter, &CLIError{Code: CodeInvalidParam, Message: fmt.Sprintf("--end %s", err)}
		}
		filter.endMs = ms
		filter.hasEnd = true
	}
	if filter.hasStart && filter.hasEnd && filter.startMs > filter.endMs {
		return filter, &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--start %q 晚于 --end %q，请修正时间范围", startRaw, endRaw),
		}
	}

	// 互斥：filter 为 CLI 侧全量扫描模式，服务端分页/排序语义无法保持。
	if cmd.Flags().Changed("versions") {
		return filter, driveListFilterExclusiveError("versions", "--versions 为独立的版本列表模式，请去掉 --versions 或过滤条件")
	}
	// 用 Changed 判定而非值非空：显式 --cursor= 空值同样视为启用游标分页。
	for _, f := range []string{"cursor", "next-token", "page-token"} {
		if fl := cmd.Flags().Lookup(f); fl != nil && fl.Changed {
			return filter, driveListFilterExclusiveError("cursor", "过滤模式为从头全量扫描，无连续游标；如需逐页翻页请去掉过滤条件")
		}
	}
	for _, f := range []string{"order-by", "order"} {
		if cmd.Flags().Changed(f) {
			return filter, driveListFilterExclusiveError(f, "过滤模式输出由客户端筛选后重排，服务端排序语义无法保持；如需服务端排序请去掉过滤条件")
		}
	}
	if cmd.Flags().Changed("limit") || cmd.Flags().Changed("max") {
		return filter, driveListFilterExclusiveError("limit", "过滤模式为全量扫描，数据量由全局上限 2000 与目录范围控制；如需控制单页条数请去掉过滤条件")
	}
	return filter, nil
}

func driveListFilterExclusiveError(flag, guidance string) error {
	return &CLIError{
		Code:    CodeInvalidParam,
		Message: fmt.Sprintf("--type/--start/--end 不能与 --%s 同时使用：%s", flag, guidance),
	}
}

// parseDriveListTime 单一时间解析入口：先试相对时间语法，失败回落 ISO8601
// （agoal.go parseISO8601ToMillis，RFC3339/无时区默认 Asia/Shanghai/仅日期）。
// 两 flag 共用同一入口；不接受毫秒时间戳（规约红线，与 search 历史遗留切割）。
func parseDriveListTime(value string) (int64, error) {
	if ms, ok := parseDriveRelativeTimeAgo(value); ok {
		return ms, nil
	}
	if ms, err := parseISO8601ToMillis(value); err == nil {
		return ms, nil
	}
	return 0, fmt.Errorf("时间格式不支持: %q（支持: 相对时间如 24h/7d/2w、RFC3339 如 2026-08-01T00:00:00+08:00、无时区 ISO8601 如 2026-08-01 08:00:00（默认 Asia/Shanghai）、仅日期 2026-08-01；不支持毫秒时间戳与 m 单位）", value)
}

// parseDriveRelativeTimeAgo 相对时间 Nh/Nd/Nw（小时/天/周），按本机时钟换算为绝对毫秒。
// 不支持 m 单位（lark 双解析器 m=分钟/月语义打架的教训，直接规避）；
// ok=false 表示非相对时间语法，由调用方回落 ISO8601 解析。
func parseDriveRelativeTimeAgo(value string) (int64, bool) {
	s := strings.TrimSpace(value)
	if len(s) < 2 {
		return 0, false
	}
	var unit time.Duration
	switch s[len(s)-1] {
	case 'h':
		unit = time.Hour
	case 'd':
		unit = 24 * time.Hour
	case 'w':
		unit = 7 * 24 * time.Hour
	default:
		return 0, false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return time.Now().Add(-time.Duration(n) * unit).UnixMilli(), true
}

// applyDriveListFilter 在 emit 管线内做类型/时间筛（pattern 之后、latest Top-N 之前）。
func applyDriveListFilter(items []map[string]any, route driveDepthRoute, filter driveListFilter) []map[string]any {
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if filter.nodeType != "" {
			folder := route.isFolder(item)
			if filter.nodeType == "folder" && !folder {
				continue
			}
			if filter.nodeType == "file" && folder {
				continue
			}
		}
		if filter.hasStart || filter.hasEnd {
			// sortTime 由 BFS 收集段无条件注入（drive_depth.go 时间戳归一）；
			// 无时间信息的条目（sortTime<=0）在时间条件下不可判定，保守滤除。
			ts, _ := item["sortTime"].(int64)
			if ts <= 0 {
				continue
			}
			if filter.hasStart && ts < filter.startMs {
				continue
			}
			if filter.hasEnd && ts > filter.endMs {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// callDriveListPageWithPattern 单层钉盘 --pattern 页内过滤（现状缺口附带修复：
// 纯透传分支此前未消费 pattern，flag 文案承诺的客户端过滤静默失效）。
// callMCPTool 透传即打印、无夹过滤的钩子，故取回解析后页内筛再输出；
// 输出保留原 body 形态（nextToken 等分页字段不动），仅条目数组被筛。
func callDriveListPageWithPattern(argsMap map[string]any, pattern string) error {
	if deps.Caller.DryRun() {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run":   true,
			"executed":  false,
			"tool":      "list_files",
			"arguments": argsMap,
			"pattern":   pattern,
			"note":      "pattern 为客户端过滤，仅作用于本页返回条目",
		})
	}
	text, err := callMCPToolReturnText(context.Background(), "list_files", argsMap)
	if err != nil {
		return err
	}
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		// 非 JSON 返回无法筛，原样输出（与透传分支的兜底形态一致）。
		deps.Out.PrintRaw(text)
		return nil
	}
	target := body
	if inner, ok := body["result"].(map[string]any); ok && driveDepthListItemsKey(inner) != "" {
		target = inner
	}
	if key := driveDepthListItemsKey(target); key != "" {
		arr, _ := target[key].([]any)
		filtered := make([]any, 0, len(arr))
		for _, entry := range arr {
			item, ok := entry.(map[string]any)
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			name, _ := item["name"].(string)
			if name == "" {
				name, _ = item["fileName"].(string)
			}
			if matchDriveNamePattern(name, pattern) {
				filtered = append(filtered, entry)
			}
		}
		target[key] = filtered
	}
	return deps.Out.PrintJSON(body)
}

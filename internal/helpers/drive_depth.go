package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// drive list --depth 递归编排（BFS + 限流补偿）
//
// 服务端无递归 API，CLI 侧 BFS 过渡方案；服务端递归 API 上线后本块整体退役。
// 钉盘（list_files）与知识库（list_nodes）共用一份路由无关骨架，差异收敛在 driveDepthRoute。
// ──────────────────────────────────────────────────────────

const (
	// 50 来源于服务端 MAX_PAGE_SIZE 硬校验（超限抛错不 clamp），服务端放宽后仅需修改此常量。
	driveDepthPageSize = 50
	// 来源与 driveDepthPageSize 不同（list_nodes pageSize 上限），数值碰巧相同但不合并。
	docDepthPageSize = 50
	// API 调用数随目录宽度指数增长，超过报错不 clamp。
	driveDepthMax      = 5
	driveDepthMaxItems = 2000
	// Sentinel 限流以 HTTP 200 + body errorCode 返回，transport 层不会自动重试，补偿只能在 BFS 层做。
	driveDepthRateLimitedCode = "invalidRequest.rateLimited"
	driveDepthCancelledMsg    = "cancelled by user, partial result emitted"
)

type driveDepthFolder struct {
	id      string // 钉盘 dentryUuid / 知识库 nodeId
	name    string
	depth   int
	relPath string
	retried bool // 已因 rate_limited 重新入队过一次，不二次补偿
	// 限流重入队时保留失败页游标：已成功页的条目已进 collected，
	// 从头重扫会产生重复条目并提前耗尽全局 2000 上限。
	pageToken string
}

type driveDepthError struct {
	Depth      int    `json:"depth"`
	FolderID   string `json:"folderId"`
	FolderName string `json:"folderName"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
}

// 取消不是错误：不记 errors[]、不复用 1~6 退出码，固定 130（128+SIGINT）。
type driveDepthCancelledError struct{}

func (e *driveDepthCancelledError) Error() string     { return driveDepthCancelledMsg }
func (e *driveDepthCancelledError) RawStderr() string { return driveDepthCancelledMsg }
func (e *driveDepthCancelledError) ExitCode() int     { return 130 }

// depth==1 走原有单层路径，仅做范围校验，不触发互斥检查。
func validateDriveListDepth(cmd *cobra.Command, depth int) error {
	if depth < 1 {
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("--depth 必须为 1~%d 的整数，当前: %d", driveDepthMax, depth),
		}
	}
	if depth > driveDepthMax {
		// 不 clamp：静默 clamp 会让用户误以为拿到了完整树。
		return &CLIError{
			Code:    CodeInvalidParam,
			Message: fmt.Sprintf("DEPTH_EXCEED_MAX: --depth 最大为 %d，当前: %d（API 调用数随目录宽度指数增长，不做静默 clamp）", driveDepthMax, depth),
		}
	}
	if depth == 1 {
		return nil
	}
	// --workspace 不互斥：depth>1 时作为路由开关切到知识库 BFS。
	if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
		return driveDepthExclusiveError("cursor", "depth>1 时多文件夹合成一份清单，无连续游标")
	}
	if cmd.Flags().Changed("limit") || cmd.Flags().Changed("max") {
		return driveDepthExclusiveError("limit", "递归模式数据量由全局上限 2000 与 --depth 层数控制")
	}
	return nil
}

func driveDepthExclusiveError(flag, reason string) error {
	return &CLIError{
		Code:    CodeInvalidParam,
		Message: fmt.Sprintf("--depth 不能与 --%s 同时使用：%s", flag, reason),
	}
}

// 路由绑定差异全部收敛在此，BFS 主循环保持路由无关。
type driveDepthRoute struct {
	serverID string // ""=钉盘自动路由 / "doc"=知识库
	toolName string
	pageSize int
	// 钉盘契约无 hasMore 字段，只信空 nextToken；知识库契约含 hasMore，显式 false 即停。
	// 钉盘的 hasMore 仅用于「hasMore=true 但 token 为空」异常检测。
	hasMoreAuthoritative bool
	buildArgs            func(base map[string]any, folderID, pageToken string) map[string]any
	parsePage            func(text string) (items []map[string]any, nextToken string, hasMore bool)
	isFolder             func(item map[string]any) bool
	itemID               func(item map[string]any) string
}

func (r driveDepthRoute) fetchPage(ctx context.Context, args map[string]any) (string, error) {
	if r.serverID == "" {
		return callMCPToolReturnText(ctx, r.toolName, args)
	}
	return callMCPToolReturnTextOnServer(ctx, r.serverID, r.toolName, args)
}

func newDrivePanDepthRoute() driveDepthRoute {
	return driveDepthRoute{
		serverID:             "",
		toolName:             "list_files",
		pageSize:             driveDepthPageSize,
		hasMoreAuthoritative: false,
		buildArgs: func(base map[string]any, folderID, pageToken string) map[string]any {
			args := map[string]any{"maxResults": float64(driveDepthPageSize)}
			for k, v := range base {
				args[k] = v
			}
			if folderID != "" {
				args["parentId"] = folderID
			}
			if pageToken != "" {
				args["nextToken"] = pageToken
			}
			return args
		},
		parsePage: parseDriveDepthPage,
		isFolder:  isDriveDepthFolder,
		itemID: func(item map[string]any) string {
			id, _ := item["fileId"].(string)
			return id
		},
	}
}

// folderId 为空 = 知识库根。
func newDocDepthRoute() driveDepthRoute {
	return driveDepthRoute{
		serverID: "doc",
		toolName: "list_nodes",
		pageSize: docDepthPageSize,
		// 字段缺失（契约违背）时按停处理——保守方向，宁可少翻页不空转。
		hasMoreAuthoritative: true,
		buildArgs: func(base map[string]any, folderID, pageToken string) map[string]any {
			args := map[string]any{"pageSize": float64(docDepthPageSize)}
			for k, v := range base {
				args[k] = v
			}
			if folderID != "" {
				args["folderId"] = folderID
			}
			if pageToken != "" {
				args["pageToken"] = pageToken
			}
			return args
		},
		parsePage: parseDocDepthPage,
		isFolder:  isDocDepthFolder,
		itemID: func(item map[string]any) string {
			id, _ := item["nodeId"].(string)
			return id
		},
	}
}

// SIGINT 检查两点（出队后发首页前 + 翻页循环发每页前），入队是纯内存操作不检查。
func runDriveListDepth(cmd *cobra.Command, route driveDepthRoute, baseArgs map[string]any, rootFolderID string, maxDepth int, pattern string, quiet bool) error {
	if deps.Caller.DryRun() {
		return printDriveDepthDryRun(route, baseArgs, maxDepth)
	}

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()

	// 兜底服务端「空页 + 非空游标」类 bug：该形态下条目不增长、全局 2000 永不触发。
	// 不用「token 不推进即停」：兜不住每页 token 都不同的空页流。
	maxPagesPerFolder := driveDepthMaxItems/route.pageSize + 1

	traceID := fmt.Sprintf("drive-depth-%d", time.Now().UnixNano())
	var (
		collected []map[string]any
		errs      = make([]driveDepthError, 0)
		truncated bool
		visited   = map[string]bool{}
	)
	queue := []driveDepthFolder{{id: rootFolderID, depth: 0}}
	if rootFolderID != "" {
		visited[rootFolderID] = true
	}

bfs:
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return emitDriveDepthCancelled(collected, errs, pattern)
		}
		folder := queue[0]
		queue = queue[1:]

		start := time.Now()
		var folderErr error
		pageToken := folder.pageToken
		pages := 0
		for {
			if ctx.Err() != nil {
				return emitDriveDepthCancelled(collected, errs, pattern)
			}
			pages++
			if pages > maxPagesPerFolder {
				folderErr = &CLIError{
					Code:    CodeMCPToolError,
					Message: fmt.Sprintf("pagination anomaly: folder exceeded %d pages with non-empty nextToken, cursor loop suspected, directory may be truncated", maxPagesPerFolder),
				}
				break
			}
			args := route.buildArgs(baseArgs, folder.id, pageToken)
			text, err := route.fetchPage(ctx, args)
			if ctx.Err() != nil {
				return emitDriveDepthCancelled(collected, errs, pattern)
			}
			if err != nil {
				folderErr = err
				break
			}
			items, next, hasMore := route.parsePage(text)
			for _, item := range items {
				name, _ := item["name"].(string)
				rel := name
				if folder.relPath != "" {
					rel = folder.relPath + "/" + name
				}
				item["depth"] = folder.depth + 1
				item["parentId"] = folder.id // 根级为空串
				item["rel_path"] = rel       // 不保证唯一，组树以 parentId 为准
				collected = append(collected, item)
				if len(collected) >= driveDepthMaxItems {
					// 未访问目录不记 errors[]（没失败只是没扫），避免 errors 数组被淹没
					truncated = true
					break
				}
				if route.isFolder(item) && folder.depth+1 < maxDepth {
					if id := route.itemID(item); id != "" && !visited[id] {
						// 重复条目静默丢弃
						visited[id] = true
						queue = append(queue, driveDepthFolder{id: id, name: name, depth: folder.depth + 1, relPath: rel})
					}
				}
			}
			if truncated {
				break bfs
			}
			if hasMore && next == "" {
				// 不静默截断——显式落 api_error，让消费方感知数据不完整。
				folderErr = &CLIError{
					Code:    CodeMCPToolError,
					Message: "pagination anomaly: hasMore=true but nextToken is empty, directory may be truncated",
				}
				break
			}
			if next == "" {
				break
			}
			if route.hasMoreAuthoritative && !hasMore {
				break
			}
			pageToken = next
		}

		slog.Info("drive list depth folder done",
			"traceId", traceID, "depth", folder.depth, "folderId", folder.id,
			"latency", time.Since(start).Milliseconds(), "failed", folderErr != nil)

		if folderErr != nil {
			if driveDepthUnrecoverable(folderErr) {
				// partial 照吐 stdout，错误详情走 stderr，非零退出
				errs = append(errs, newDriveDepthError(folder, folderErr))
				if emitErr := emitDriveDepthResult(collected, errs, truncated, pattern); emitErr != nil {
					return emitErr
				}
				return folderErr
			}
			if driveDepthErrorCode(folderErr) == driveDepthRateLimitedCode && !folder.retried {
				// 限流命中在入口处、失败页本身无副作用；但此前成功页已进 collected，
				// 必须从失败页游标续扫而非整目录重扫。不加 sleep/退避。
				folder.retried = true
				folder.pageToken = pageToken
				queue = append(queue, folder)
				continue
			}
			if folder.depth == 0 && len(collected) == 0 {
				return folderErr
			}
			// 403 / transport 重试耗尽 / 限流重入队仍失败：记 errors[] 跳过
			errs = append(errs, newDriveDepthError(folder, folderErr))
			continue
		}

		if !quiet {
			display := folder.name
			if display == "" {
				display = folder.id
			}
			if display == "" {
				display = "<root>"
			}
			fmt.Fprintf(os.Stderr, "[drive-list] depth=%d folder=%s done\n", folder.depth, display)
		}
	}

	return emitDriveDepthResult(collected, errs, truncated, pattern)
}

func emitDriveDepthCancelled(items []map[string]any, errs []driveDepthError, pattern string) error {
	if err := emitDriveDepthResult(items, errs, true, pattern); err != nil {
		return err
	}
	return &driveDepthCancelledError{}
}

// depth>1 不输出 nextToken。
func emitDriveDepthResult(items []map[string]any, errs []driveDepthError, truncated bool, pattern string) error {
	if pattern != "" {
		// 先递归后过滤，过滤仅作用于输出项，不阻止文件夹下钻
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			name, _ := item["name"].(string)
			if name == "" {
				name, _ = item["fileName"].(string)
			}
			if matchDriveNamePattern(name, pattern) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	// 排列为 rel_path 树序：BFS 只决定截断时哪些条目入选，树序决定呈现顺序。
	sort.SliceStable(items, func(i, j int) bool {
		ri, _ := items[i]["rel_path"].(string)
		rj, _ := items[j]["rel_path"].(string)
		if ri != rj {
			return ri < rj
		}
		return driveDepthItemID(items[i]) < driveDepthItemID(items[j])
	})
	maxDepth := 0
	for _, item := range items {
		if d, ok := item["depth"].(int); ok && d > maxDepth {
			maxDepth = d
		}
	}
	if items == nil {
		items = []map[string]any{}
	}
	return deps.Out.PrintJSON(map[string]any{
		"items":     items,
		"maxDepth":  maxDepth,
		"truncated": truncated,
		"errors":    errs,
	})
}

// 不输出预估调用次数：零调用下平均子文件夹数不可知，硬编码上界会被误读为真实估算。
func printDriveDepthDryRun(route driveDepthRoute, baseArgs map[string]any, maxDepth int) error {
	if deps.Caller.Format() == "json" {
		return deps.Out.PrintJSON(map[string]any{
			"dry_run":    true,
			"executed":   false,
			"tool":       route.toolName,
			"baseArgs":   baseArgs,
			"maxDepth":   maxDepth,
			"pageSize":   route.pageSize,
			"totalLimit": driveDepthMaxItems,
			"warning":    "调用数随目录宽度指数增长",
		})
	}
	bold := color.New(color.FgYellow, color.Bold)
	bold.Println("[DRY-RUN] Preview only, not executed:")
	deps.Out.PrintKeyValue("Tool", route.toolName)
	argsJSON, _ := json.MarshalIndent(baseArgs, "  ", "  ")
	deps.Out.PrintKeyValue("baseArgs", "\n  "+string(argsJSON))
	deps.Out.PrintKeyValue("maxDepth", fmt.Sprintf("%d", maxDepth))
	deps.Out.PrintKeyValue("每页条数", fmt.Sprintf("%d", route.pageSize))
	deps.Out.PrintKeyValue("全局上限", fmt.Sprintf("%d", driveDepthMaxItems))
	deps.Out.PrintWarning("调用数随目录宽度指数增长")
	return nil
}

// 钉盘契约无 hasMore 字段，hasMoreExplicit 仅供「hasMore=true 但 token 为空」异常检测。
func parseDriveDepthPage(text string) (items []map[string]any, nextToken string, hasMoreExplicit bool) {
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return nil, "", false
	}
	target := body
	if inner, ok := body["result"].(map[string]any); ok && driveDepthListItemsKey(inner) != "" {
		target = inner
	}
	key := driveDepthListItemsKey(target)
	if key == "" {
		return nil, "", false
	}
	arr, _ := target[key].([]any)
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			items = append(items, m)
		}
	}
	nextToken, _ = target["nextToken"].(string)
	hasMoreExplicit, _ = target["hasMore"].(bool)
	return items, nextToken, hasMoreExplicit
}

func driveDepthListItemsKey(m map[string]any) string {
	for _, k := range []string{"items", "dentryList"} {
		if arr, ok := m[k].([]any); ok && arr != nil {
			return k
		}
	}
	return ""
}

// 非法通配符降级为子串匹配，避免 filepath.Match 报错导致过滤静默失效。
func matchDriveNamePattern(name, pattern string) bool {
	if pattern == "" {
		return true
	}
	p := pattern
	if !strings.ContainsAny(p, "*?[") {
		p = "*" + p + "*"
	}
	ok, err := filepath.Match(p, name)
	if err != nil {
		return strings.Contains(name, pattern)
	}
	return ok
}

func isDriveDepthFolder(item map[string]any) bool {
	for _, key := range []string{"type", "dentryType"} {
		if v, ok := item[key].(string); ok && strings.EqualFold(v, "FOLDER") {
			return true
		}
	}
	return false
}

// list_nodes 游标字段叫 nextPageToken，与钉盘的 nextToken 不同名。
func parseDocDepthPage(text string) (items []map[string]any, nextToken string, hasMore bool) {
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return nil, "", false
	}
	arr, _ := body["nodes"].([]any)
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			items = append(items, m)
		}
	}
	nextToken, _ = body["nextPageToken"].(string)
	hasMore, _ = body["hasMore"].(bool)
	return items, nextToken, hasMore
}

func isDocDepthFolder(item map[string]any) bool {
	v, _ := item["nodeType"].(string)
	return strings.EqualFold(v, "folder")
}

func driveDepthItemID(item map[string]any) string {
	for _, key := range []string{"fileId", "nodeId"} {
		if s, ok := item[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func driveDepthUnrecoverable(err error) bool {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		switch cliErr.Code {
		case CodeAuthTokenExpired, CodeAuthNotConfigured, CodeNetworkTimeout, CodeNetworkUnreachable:
			return true
		}
	}
	return false
}

// CLIError.Message 保留了原始响应 JSON。
func driveDepthErrorCode(err error) string {
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		return ""
	}
	var body map[string]any
	if json.Unmarshal([]byte(cliErr.Message), &body) != nil {
		return ""
	}
	for _, key := range []string{"errorCode", "error_code", "code"} {
		if s, ok := body[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// 匹配顺序是硬约束：invalidRequest.rateLimited 自带 "invalidRequest." 前缀，
// 若先做 invalidRequest.* 前缀分类，限流会被吞进参数错误类。必须先精确匹配限流，
// 再 forbidden.* 前缀，最后兜底 api_error。未知 reason 按 api_error 兼容。
func classifyDriveDepthReason(errorCode string) string {
	switch {
	case errorCode == driveDepthRateLimitedCode:
		return "rate_limited"
	case strings.HasPrefix(errorCode, "forbidden."):
		return "permission_denied"
	default:
		return "api_error"
	}
}

func newDriveDepthError(folder driveDepthFolder, err error) driveDepthError {
	return driveDepthError{
		Depth:      folder.depth,
		FolderID:   folder.id,
		FolderName: folder.name,
		Reason:     classifyDriveDepthReason(driveDepthErrorCode(err)),
		Message:    driveDepthErrorMessage(err),
	}
}

func driveDepthErrorMessage(err error) string {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		var body map[string]any
		if json.Unmarshal([]byte(cliErr.Message), &body) == nil {
			for _, key := range []string{"errorMsg", "message"} {
				if s, ok := body[key].(string); ok && s != "" {
					return s
				}
			}
		}
		return cliErr.Message
	}
	return err.Error()
}

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
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
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
// filter 零值 = 未启用；启用时由 emitDriveDepthResult 管线在 pattern 之后、latest 之前筛。
func runDriveListDepth(cmd *cobra.Command, route driveDepthRoute, baseArgs map[string]any, rootFolderID string, maxDepth int, pattern string, quiet bool, latest int, filter driveListFilter) error {
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
			return emitDriveDepthCancelled(collected, errs, pattern, latest, maxDepth, route, filter)
		}
		folder := queue[0]
		queue = queue[1:]

		start := time.Now()
		var folderErr error
		pageToken := folder.pageToken
		pages := 0
		for {
			if ctx.Err() != nil {
				return emitDriveDepthCancelled(collected, errs, pattern, latest, maxDepth, route, filter)
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
				return emitDriveDepthCancelled(collected, errs, pattern, latest, maxDepth, route, filter)
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
				// 时间戳归一：钉盘 modifyTime / 知识库 updateTime 统一为 sortTime（毫秒 int64）
				if ms, ok := driveItemModifiedMillis(item); ok {
					item["sortTime"] = ms
				} else {
					item["sortTime"] = int64(0)
				}
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
				if latest > 0 {
					// 不完整集合上的 Top-N 会被误读为全局最新：latest 下不吐 partial，
					// 直接回根因错误（auth 过期 / 网络不可达比通用 token 更可操作）。
					return folderErr
				}
				// partial 照吐 stdout，错误详情走 stderr，非零退出
				errs = append(errs, newDriveDepthError(folder, folderErr))
				if emitErr := emitDriveDepthResult(collected, errs, truncated, pattern, latest, maxDepth, route, filter); emitErr != nil {
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

	// BFS 序与修改时间无关：截断与递归途中目录失败都让未扫区域可能含更新文件，
	// 此时的 Top-N 不是全局最新，两者同属一条防线——拒绝以成功状态产出。
	if latest > 0 && (truncated || len(errs) > 0) {
		return driveLatestIncompleteError(latest, truncated, errs, driveLatestScopeFromCmd(cmd, maxDepth, rootFolderID))
	}

	return emitDriveDepthResult(collected, errs, truncated, pattern, latest, maxDepth, route, filter)
}

// driveLatestScope 是原调用的完整候选集快照，用于生成不改变候选集的恢复命令。
//
// 恢复命令若丢掉任何一项，用户照抄后都会在**另一个集合**上拿到一份「看起来对」的 Top-N ——
// 比直接报错更难发现：丢 --workspace/--space-id 会从知识库切到普通钉盘（或反之）；丢 --folder
// 会从子树跳到空间根；丢 --pattern/--type/--start/--end 会把全部条目纳入排序基。
type driveLatestScope struct {
	// domain 是查询域 flag 串（如 "--workspace ws-1" / "--space-id sp-1"），无则空串。
	domain string
	// filters 是决定候选集的过滤 flag 串（--pattern/--type/--start/--end），无则空串。
	filters string
	// folder 是原调用实际使用的扫描根（已解析的 ID，非用户原始 URL），空则为空间根。
	folder string
	// depth 是原调用的 --depth 层数，让「去掉 --latest 重跑」的恢复命令给出确切层数。
	depth int
	// notes 收集无法安全内联进可执行命令的原值展示行。POSIX 构建下恒为空（单引号足够）；
	// Windows 构建下含元字符的值走这里，命令里只留占位符。
	notes []string
	// 以上 domain/filters/folder 里的值全部经 driveLatestScopeValue 渲染：恢复命令是给用户
	// 直接复制到 shell 执行的，而 workspace 的常见形态就是带 & 查询串的 URL，pattern 又天然
	// 含 * 与中文，裸拼接会改变命令解析。
}

// driveLatestValueRenderer 把用户值渲染成可内联的命令片段；ok 为 false 表示该值在目标 shell
// 下无法安全内联。取成参数而非直接调用平台绑定函数，是为了让任一平台的测试都能驱动另一平台
// 的降级分支 —— 否则「Windows 上降级为占位符」这条路在 POSIX 机器上永不可达、无法验证。
type driveLatestValueRenderer func(string) (string, bool)

// value 渲染单个用户值供内联。值在目标 shell 下无法安全内联时，登记一条展示行并返回占位符
// —— 宁可让用户手动粘一次，也不能给出一条粘贴即执行额外命令的「恢复命令」。
// 展示行用 strconv.Quote 包裹并显式声明非可执行，与 internal/auth 侧展示 profile 标识一致。
func (s *driveLatestScope) value(render driveLatestValueRenderer, label, v string) string {
	if inline, ok := render(v); ok {
		return inline
	}
	s.notes = append(s.notes, fmt.Sprintf("%s 原值（仅作数据展示，不是可执行命令）%s", label, strconv.Quote(v)))
	return driveLatestUnsafeValuePlaceholder
}

// driveLatestUnsafeValuePlaceholder 是不可内联值在命令中的占位符。
const driveLatestUnsafeValuePlaceholder = "<见下方原值>"

// driveLatestScopeFromCmd 从原命令抽完整候选集。--workspace 决定路由（知识库 vs 钉盘），判定与
// drive list 里的路由分支同源（同一个 flagOrFallback(cmd, "workspace", "workspace-id")）；
// 钉盘侧的 --space-id 同样必须保留。目录 flag 名无需按路由切换：--folder 两条路由都接受。
//
// rootFolder 取 runDriveListDepth 实际使用的扫描根而非重新读 flag：用户可能传的是 URL，
// 解析后的 ID 才是真正被扫描的目标，也是照抄时更精确的形态。
func driveLatestScopeFromCmd(cmd *cobra.Command, depth int, rootFolder string) driveLatestScope {
	return driveLatestScopeFrom(cmd, depth, rootFolder, driveLatestScopeValue)
}

// driveLatestScopeFrom 是 driveLatestScopeFromCmd 的可注入本体：render 决定用户值以何种形态
// 进入恢复命令。生产路径固定传平台绑定的 driveLatestScopeValue；测试可传另一平台的策略，
// 从而在单一平台上覆盖两种形态。
func driveLatestScopeFrom(cmd *cobra.Command, depth int, rootFolder string, render driveLatestValueRenderer) driveLatestScope {
	scope := driveLatestScope{depth: depth}
	if workspaceID := flagOrFallback(cmd, "workspace", "workspace-id"); workspaceID != "" {
		scope.domain = "--workspace " + scope.value(render, "--workspace", workspaceID)
	} else if spaceID, _ := cmd.Flags().GetString("space-id"); spaceID != "" {
		scope.domain = "--space-id " + scope.value(render, "--space-id", spaceID)
	}
	if rootFolder != "" {
		scope.folder = scope.value(render, "--folder", rootFolder)
	}
	// 顺序固定为注册顺序，保证同一组入参每次给出同一条恢复命令（便于用户比对与测试断言）。
	filters := make([]string, 0, 4)
	for _, name := range []string{"pattern", "type", "start", "end"} {
		if v, _ := cmd.Flags().GetString(name); v != "" {
			filters = append(filters, "--"+name+" "+scope.value(render, "--"+name, v))
		}
	}
	scope.filters = strings.Join(filters, " ")
	return scope
}

// command 拼一条保留原候选集的恢复命令：查询域 + 指定的 --folder + 原过滤条件。
// folderArg 传 "" 表示该条命令不带 --folder（原调用就在空间根时不应凭空塞一个）。
func (s driveLatestScope) command(folderArg string) string {
	parts := make([]string, 0, 4)
	parts = append(parts, "dws drive list")
	if s.domain != "" {
		parts = append(parts, s.domain)
	}
	if folderArg != "" {
		parts = append(parts, "--folder "+folderArg)
	}
	if s.filters != "" {
		parts = append(parts, s.filters)
	}
	return strings.Join(parts, " ")
}

// driveLatestIncompleteError 是排序基不完整时的拒绝产出错误。截断与目录失败共用
// CodeContentTruncated（→ ExitAPI），但 token 分开，便于消费方区分「范围太大」与「读不到」；
// 二者同真时两个 token 都带。拒绝产出后 errors[] 不再进 stdout，失败详情必须落在错误消息里，
// 否则用户完全瞎。调用点已保证 truncated 与 len(errs)>0 至少一真。
func driveLatestIncompleteError(latest int, truncated bool, errs []driveDepthError, scope driveLatestScope) error {
	// 目录失败详情排在截断之前：BFS 可以先记下可恢复目录错误、再在别的目录撞上 2000 上限，
	// 此时 permission_denied 这类 reason 是用户唯一能动手修的线索，不能被截断提示吞掉。
	causes := make([]string, 0, 2)
	if len(errs) > 0 {
		causes = append(causes, driveLatestFolderFailureCause(errs))
	}
	if truncated {
		causes = append(causes, fmt.Sprintf("LATEST_SCAN_TRUNCATED: 扫描在全局上限 %d 条处截断", driveDepthMaxItems))
	}
	return &CLIError{
		Code: CodeContentTruncated,
		Message: fmt.Sprintf("%s，未扫描区域可能含更新文件，拒绝输出不完整的 Top-%d",
			strings.Join(causes, "；同时 "), latest),
		Suggestion: driveLatestIncompleteSuggestion(latest, truncated, len(errs) > 0, scope),
	}
}

// driveLatestFolderFailureCause 组装目录失败详情，含首个失败的 folder/depth/reason。
// folderName 空回落 folderID，两者都空回落 <root>。
//
// folderName / folderID / message 三项都是远端可控内容（目录名由共享目录的创建者决定，
// message 是服务端错误文本），必须过 driveLatestSafeRemoteText。Reason 不用过：它是
// classifyDriveDepthReason 的固定三值映射，与服务端字符串无关。
func driveLatestFolderFailureCause(errs []driveDepthError) string {
	first := errs[0]
	folder := first.FolderName
	if folder == "" {
		folder = first.FolderID
	}
	if folder == "" {
		folder = "<root>"
	}
	return fmt.Sprintf("LATEST_SCAN_INCOMPLETE: %d 个目录未读全（首个失败 folder=%s depth=%d reason=%s: %s）",
		len(errs), driveLatestSafeRemoteText(folder), first.Depth, first.Reason,
		driveLatestSafeRemoteText(first.Message))
}

// driveLatestSafeRemoteText 把远端可控文本压成可安全嵌进单行 stderr 错误消息的形式。
//
// 拒绝产出后 errors[] 不再进 stdout，失败详情改走纯文本错误消息 —— 而 JSON 编码会转义的
// 控制字符在纯文本里会被终端直接执行：ANSI/OSC 序列可以伪造提示、清屏、隐藏后续内容、改窗口
// 标题，在 AI Agent 场景还会污染上下文窗口。latest=0 的既有路径仍把原值放进 errors[] JSON，
// 不受影响，也不该受影响（消费方需要原始数据）。
//
// output.SanitizeForTerminal 负责剥 ANSI/OSC、C0 控制字符与危险 Unicode，但按设计保留 \n 与
// \t；本错误消息是单行叙述，故再把这两者折成空格，避免远端换行把一条错误拆成多行伪造输出。
func driveLatestSafeRemoteText(s string) string {
	s = output.SanitizeForTerminal(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

// driveLatestIncompleteSuggestion 按实际触发的成因给恢复指引。约束两条：
//  1. 每条示例命令都带原查询域（scope.base()），照抄不会切换查询域；
//  2. 每个子句的示例命令与该子句正文一致——「去掉 --latest」的子句示例不带 --latest，
//     否则照抄复现同一错误。
func driveLatestIncompleteSuggestion(latest int, truncated, folderFailed bool, scope driveLatestScope) string {
	// 「缩小范围」类命令要求用户换一个目录，故 --folder 给占位符；查询域与原过滤条件由
	// command 一并带上，否则照抄后候选集就变了（例如丢掉 --pattern 会对全部条目取 Top-N）。
	narrowed := scope.command("<可读子目录ID>")
	clauses := make([]string, 0, 3)
	if folderFailed {
		clauses = append(clauses, "确认目录权限后重试")
	}
	switch {
	case folderFailed && truncated:
		// 两个成因都要解：既要换到可读目录，也要把范围缩到 2000 条以内。
		clauses = append(clauses, fmt.Sprintf("或用 --folder 缩小到可读子目录、并降低 --depth 层数后重取 Top-%d：%s --latest %d", latest, narrowed, latest))
	case folderFailed:
		clauses = append(clauses, fmt.Sprintf("或用 --folder 缩小到可读子目录后重取 Top-%d：%s --latest %d", latest, narrowed, latest))
	default:
		clauses = append(clauses, fmt.Sprintf("缩小扫描范围后重试：--folder 指定子目录，或降低 --depth 层数，如 %s --latest %d", narrowed, latest))
	}
	// partial+errors[] 承诺限定 --depth>1：单层去掉 --latest 会路由回普通单层 list，本就无
	// errors[] 契约，故该子句只在多层时给出，并直接带上原层数。这是唯一一条「按原范围」命令，
	// 必须原样带回原 --folder（原调用在空间根时则不带），照抄即复现同一候选集、只是不取 Top-N。
	if folderFailed && scope.depth > 1 {
		clauses = append(clauses, fmt.Sprintf("需要看失败明细请去掉 --latest 按原范围重跑（同时输出已扫到的 partial 与 errors[] 明细）：%s --depth %d", scope.command(scope.folder), scope.depth))
	}
	// Windows 构建下无法安全引用的值不会进入命令（cmd.exe 不把单引号当引号），原值改在此处以
	// 数据行给出，由用户手动替换占位符 —— 少一次复制粘贴的便利，换掉一条粘贴即执行的命令。
	if len(scope.notes) > 0 {
		clauses = append(clauses, fmt.Sprintf("命令中的 %s 请手动替换为 —— %s",
			driveLatestUnsafeValuePlaceholder, strings.Join(scope.notes, "、")))
	}
	return strings.Join(clauses, "；")
}

// emitDriveDepthCancelled 处理 SIGINT 取消：吐已扫到的 partial（truncated=true）后回退出码 130。
//
// 这里刻意**不**套用 latest 的拒绝产出防线（只有 BFS 尾部 guard 与 unrecoverable 分支走）：
// 取消是用户主动发起的，退出码 130 本身已明确告知结果不完整，此时 partial 是用户的预期产物而
// 非冒充全局最新的误导。sortTime 仍由 emitDriveDepthResult 统一剥除，取消路径不例外。
func emitDriveDepthCancelled(items []map[string]any, errs []driveDepthError, pattern string, latest, reqDepth int, route driveDepthRoute, filter driveListFilter) error {
	if err := emitDriveDepthResult(items, errs, true, pattern, latest, reqDepth, route, filter); err != nil {
		return err
	}
	return &driveDepthCancelledError{}
}

// depth>1 不输出 nextToken。
func emitDriveDepthResult(items []map[string]any, errs []driveDepthError, truncated bool, pattern string, latest, reqDepth int, route driveDepthRoute, filter driveListFilter) error {
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
	if filter.active() {
		// 类型（route.isFolder 反判）+ 时间（直读收集段注入的 sortTime 毫秒）区间筛
		items = applyDriveListFilter(items, route, filter)
	}
	if latest > 0 {
		// --type folder 时 latest 对过滤后的目录取 Top-N，避免「先留目录再剔目录」的空结果。
		items = applyDriveListLatest(items, latest, filter.nodeType == "folder")
	} else {
		// 排列为 rel_path 树序：BFS 只决定截断时哪些条目入选，树序决定呈现顺序。
		sort.SliceStable(items, func(i, j int) bool {
			ri, _ := items[i]["rel_path"].(string)
			rj, _ := items[j]["rel_path"].(string)
			if ri != rj {
				return ri < rj
			}
			return driveDepthItemID(items[i]) < driveDepthItemID(items[j])
		})
	}
	maxDepth := 0
	for _, item := range items {
		if d, ok := item["depth"].(int); ok && d > maxDepth {
			maxDepth = d
		}
	}
	// sortTime 是内部排序字段（applyDriveListLatest 排 Top-N、applyDriveListFilter 筛时间区间
	// 时都已用完），任何输出路径都不得泄露进契约。放在这里一处覆盖三条 emit 路径：正常 emit、
	// SIGINT 取消、unrecoverable partial。
	// depth/parentId/rel_path 不在此处删——它们是 depth>1 的既有输出契约，
	// stripDriveDepthDecorations 仅在单层（reqDepth==1）把这套装饰整体剥掉。
	for _, item := range items {
		delete(item, "sortTime")
	}
	if (latest > 0 || filter.active()) && reqDepth == 1 {
		stripDriveDepthDecorations(items)
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

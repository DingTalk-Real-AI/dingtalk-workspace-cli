package helpers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

// ==========================================================
// drive pull — 把钉盘文件夹镜像到本地（Drive → 本地）
// ==========================================================
// ──────────────────────────────────────────────────────────
// dws drive pull — 把钉盘文件夹单向、文件级镜像到本地（Drive → 本地）
//
// 递归列出 --remote-folder 指向的钉盘文件夹下所有 type=FILE 的文件，逐一下载到
// --local-folder 对应的相对路径。已存在的本地文件按 --if-exists 决定
// overwrite / smart / skip。结构化 summary + items 始终打印到 stdout；
// summary.failed > 0 时额外以非零退出码退出。
// ──────────────────────────────────────────────────────────

// --if-exists 的三种策略。
const (
	ifExistsOverwrite = "overwrite" // 总是下载覆盖（Drive 为权威源）
	ifExistsSmart     = "smart"     // 推荐增量：本地 mtime 已 ≥ 远端 modified_time 则跳过
	ifExistsSkip      = "skip"      // 默认：本地已存在则保持不动
)

// pull 下载路径使用 os.Root 固定根目录和目标父目录。以下 seam 只用于
// 确定性覆盖文件系统与传输失败分支；测试必须通过 testseam.Swap 替换。
var (
	pullMkdirAll    = os.MkdirAll
	pullOpenRoot    = os.OpenRoot
	pullRelPath     = filepath.Rel
	pullPathStat    = os.Stat
	pullPathLstat   = os.Lstat
	pullTargetLstat = os.Lstat
	pullRootMkdir   = func(root *os.Root, name string, mode os.FileMode) error {
		return root.Mkdir(name, mode)
	}
	pullOpenParentRoot = func(root *os.Root, name string) (*os.Root, error) {
		return root.OpenRoot(name)
	}
	pullRootStat   = func(root *os.Root, name string) (os.FileInfo, error) { return root.Stat(name) }
	pullRootLstat  = func(root *os.Root, name string) (os.FileInfo, error) { return root.Lstat(name) }
	pullCreateTemp = createPinnedPullTemp
	pullTempName   = uuid.NewString
	pullSyncTemp   = func(file *os.File) error { return file.Sync() }
	pullCloseTemp  = func(file *os.File) error { return file.Close() }
	pullRename     = func(root *os.Root, oldName, newName string) error {
		return root.Rename(oldName, newName)
	}
	pullLink         = func(root *os.Root, oldName, newName string) error { return root.Link(oldName, newName) }
	pullRemove       = func(root *os.Root, name string) error { return root.Remove(name) }
	pullDownloadFile = defaultPullDownloadFile
	pullHTTPDo       = func(request *http.Request) (*http.Response, error) {
		return (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	}
)

// pull 动作分类。
const (
	pullActionDownloaded = "downloaded"
	pullActionSkipped    = "skipped"
	pullActionFailed     = "failed"
)

// drivePullItem 是输出 items[] 中每个文件的明细。
type drivePullItem struct {
	RelPath string `json:"rel_path"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

// drivePullSummary 是各动作的计数汇总。
type drivePullSummary struct {
	Downloaded int `json:"downloaded"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

// drivePullResult 是 pull 命令的输出 schema。
type drivePullResult struct {
	Summary drivePullSummary `json:"summary"`
	Items   []drivePullItem  `json:"items"`
}

type drivePullDryRunResult struct {
	DryRun      bool            `json:"dry_run"`
	Executed    bool            `json:"executed"`
	PreviewKind string          `json:"preview_kind"`
	Operation   string          `json:"operation"`
	IfExists    string          `json:"if_exists"`
	Plan        drivePullResult `json:"plan"`
}

// drivePartialFailure 在 summary.failed > 0 时返回：结构化结果已打印到 stdout，
// 这里只负责以 exit=1 退出并向 stderr 输出一行简短说明（与 push/sync 一致）。
type drivePartialFailure struct{ failed int }

func (e *drivePartialFailure) Error() string {
	return fmt.Sprintf("drive pull: %d file(s) failed", e.failed)
}
func (e *drivePartialFailure) RawStderr() string { return e.Error() }
func (e *drivePartialFailure) ExitCode() int     { return 1 }

// pathCollisionKey 把本地目标路径归一化成「目标文件系统下的等价键」，用于探测多个
// 远端条目是否会落到同一个本地文件。caseInsensitive 为真时（Windows / 默认 macOS）
// 折叠大小写并做 Unicode NFC 规范化，从而把 A.txt/a.txt、NFC/NFD 记法视为同一目标；
// 大小写敏感文件系统（如 Linux ext4）则按精确路径区分，避免误判合法的异名文件。
func pathCollisionKey(target string, caseInsensitive bool) string {
	p := filepath.Clean(target)
	if caseInsensitive {
		p = norm.NFC.String(strings.ToLower(p))
	}
	return p
}

// isCaseInsensitiveFS 是大小写探测的注入点（与 httpGetFile / httpPutFile 同样的 seam）：
// 生产路径始终是 detectCaseInsensitiveFS，测试可替换它，以便在大小写敏感的 CI 文件系统上
// 也能走到「等价路径冲突」分支。
var isCaseInsensitiveFS = detectCaseInsensitiveFS

// caseProbePattern 是探针文件名模板。抽成变量只为可测：纯数字模板的大写与原名相同，
// 可覆盖「名称无大小写差异、无法据此判定」的回退分支。
var caseProbePattern = "dws-caseprobe-*"

// detectCaseInsensitiveFS 探测 dir 所在文件系统是否大小写不敏感：在 dir 下创建一个随机
// 小写名探针文件，再用其大写名 stat；命中同一文件即为不敏感。dir 必须已存在。
// 探测失败时回退到平台默认（Windows / macOS 视为不敏感，其它敏感）。
func detectCaseInsensitiveFS(dir string) bool {
	platformDefault := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	f, err := os.CreateTemp(dir, caseProbePattern)
	if err != nil {
		return platformDefault
	}
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)
	base := filepath.Base(name)
	upper := strings.ToUpper(base)
	if upper == base {
		return platformDefault // 名称无大小写差异，无法据此判定
	}
	_, statErr := os.Stat(filepath.Join(dir, upper))
	return statErr == nil
}

// detectTargetCollisions 按 caseInsensitive 指定的等价规则，找出会映射到同一本地目标
// 的多个远端 rel_path，返回被判定冲突的 rel_path 集合（其中每个都不应落盘）。
func detectTargetCollisions(absDir string, rels []string, caseInsensitive bool) map[string]bool {
	groups := make(map[string][]string, len(rels))
	for _, rel := range rels {
		target := filepath.Join(absDir, filepath.FromSlash(rel))
		key := pathCollisionKey(target, caseInsensitive)
		groups[key] = append(groups[key], rel)
	}
	collided := make(map[string]bool)
	for _, g := range groups {
		if len(g) > 1 {
			for _, r := range g {
				collided[r] = true
			}
		}
	}
	return collided
}

func runDrivePull(cmd *cobra.Command, _ []string) error {
	if err := validateRequiredFlags(cmd, "local-folder", "remote-folder"); err != nil {
		return err
	}
	localDir := mustGetFlag(cmd, "local-folder")
	remoteDirID := mustGetFlag(cmd, "remote-folder")
	// space-id 可选：不传则由 fetchRemoteDriveTree 使用「我的文件」对应的空间。
	spaceID := mustGetFlag(cmd, "space-id")

	ifExists, _ := cmd.Flags().GetString("if-exists")
	if ifExists == "" {
		// 安全默认：不自动覆盖本地既有文件。
		ifExists = ifExistsSkip
	}
	switch ifExists {
	case ifExistsOverwrite, ifExistsSmart, ifExistsSkip:
	default:
		return fmt.Errorf("--if-exists 取值非法: %s（可选 overwrite|smart|skip）", ifExists)
	}

	absDir, err := validateLocalDirAbs(localDir)
	if err != nil {
		return err
	}
	// 在任何远端读取之前固定本地根计划：既有根立即持有 os.Root；不存在的根只固定
	// 最近既存祖先并记录尚缺的逐层后缀。远端清单不完整或预检失败时不得创建本地根。
	rootPlan, err := planPinnedPullRoot(absDir)
	if err != nil {
		return err
	}
	defer rootPlan.close()

	ctx := cmd.Context()
	// 复用 status 的远端遍历：按 parentId 递归拿到所有 type=FILE 的文件（key 为 rel_path）。
	remote, err := fetchRemoteDriveTree(ctx, spaceID, remoteDirID, false)
	if err != nil {
		return err
	}

	// 稳定顺序输出（rel_path 升序）。
	relPaths := make([]string, 0, len(remote))
	for rel := range remote {
		relPaths = append(relPaths, rel)
	}
	sort.Strings(relPaths)
	preflight, localInfo, err := buildDrivePullPreflightFromPlan(rootPlan, remote)
	if err != nil {
		return err
	}
	if deps.Caller.DryRun() {
		return printDrivePullDryRunWithLocalInfo(ifExists, remote, relPaths, preflight, localInfo)
	}
	// 镜像契约独立于当前主机文件系统：远端 A/a 或 NFC/NFD 异写（包括目录
	// 前缀）在任何落盘前整批拒绝，避免同一远端树在不同平台得到不同镜像。
	if len(preflight) > 0 {
		res := drivePullPreflightFailureResult(relPaths, preflight)
		if perr := deps.Out.PrintJSON(res); perr != nil {
			return perr
		}
		return &drivePartialFailure{failed: res.Summary.Failed}
	}
	pinnedRoot, err := rootPlan.materialize()
	if err != nil {
		return err
	}
	defer pinnedRoot.close()

	res := drivePullResult{Items: make([]drivePullItem, 0, len(relPaths))}
	for _, rel := range relPaths {
		rf := remote[rel]
		// preflight 与真正下载之间仍需二次解析，挡住祖先目录被并发替换为软链的
		// TOCTOU 逃逸；pullRemoteFile 将该检查收口在写入入口前。
		action, perr := pullRemoteFilePinned(ctx, spaceID, rf, pinnedRoot, rel, ifExists)
		item := drivePullItem{RelPath: rel, Action: action}
		switch action {
		case pullActionDownloaded:
			res.Summary.Downloaded++
		case pullActionSkipped:
			res.Summary.Skipped++
		case pullActionFailed:
			res.Summary.Failed++
			if perr != nil {
				item.Error = perr.Error()
			}
		}
		res.Items = append(res.Items, item)
	}

	// 结构化结果始终打印到 stdout；有失败则额外以非零退出码退出。
	if perr := deps.Out.PrintJSON(res); perr != nil {
		return perr
	}
	if res.Summary.Failed > 0 {
		return &drivePartialFailure{failed: res.Summary.Failed}
	}
	return nil
}

func printDrivePullDryRun(absDir, ifExists string, remote map[string]*remoteFile, relPaths []string, caseInsensitive bool) error {
	return printDrivePullDryRunWithPreflight(absDir, ifExists, remote, relPaths, caseInsensitive,
		buildDrivePullPreflight(absDir, remote))
}

func printDrivePullDryRunWithPreflight(absDir, ifExists string, remote map[string]*remoteFile, relPaths []string, caseInsensitive bool, preflight map[string]string) error {
	localInfo := make(map[string]os.FileInfo, len(relPaths))
	for _, rel := range relPaths {
		localPath, err := resolveLocalTarget(absDir, rel)
		if err != nil {
			continue
		}
		if info, statErr := os.Stat(localPath); statErr == nil && info.Mode().IsRegular() {
			localInfo[rel] = info
		}
	}
	_ = caseInsensitive // 保留 helper 的平台预览参数兼容性。
	return printDrivePullDryRunWithLocalInfo(ifExists, remote, relPaths, preflight, localInfo)
}

func printDrivePullDryRunWithLocalInfo(ifExists string, remote map[string]*remoteFile, relPaths []string, preflight map[string]string, localInfo map[string]os.FileInfo) error {
	plan := drivePullResult{Items: make([]drivePullItem, 0, len(relPaths))}
	if len(preflight) > 0 {
		plan = drivePullPreflightFailureResult(relPaths, preflight)
		return deps.Out.PrintJSON(drivePullDryRunResult{
			DryRun: true, Executed: false, PreviewKind: "plan", Operation: "drive pull",
			IfExists: ifExists, Plan: plan,
		})
	}
	for _, rel := range relPaths {
		action := pullActionDownloaded
		if fi := localInfo[rel]; fi != nil {
			switch ifExists {
			case ifExistsSkip:
				action = pullActionSkipped
			case ifExistsSmart:
				rf := remote[rel]
				if rf.ModifiedTimeValid && fi.ModTime().UnixMilli() >= rf.ModifiedTime {
					action = pullActionSkipped
				}
			}
		}
		if action == pullActionSkipped {
			plan.Summary.Skipped++
		} else {
			plan.Summary.Downloaded++
		}
		plan.Items = append(plan.Items, drivePullItem{RelPath: rel, Action: action})
	}
	return deps.Out.PrintJSON(drivePullDryRunResult{
		DryRun: true, Executed: false, PreviewKind: "plan", Operation: "drive pull",
		IfExists: ifExists, Plan: plan,
	})
}

// buildDrivePullPreflight 在创建本地根目录、探测文件系统或下载任一文件之前，
// 一次性验证完整远端树及每个本地落盘目标。任一目标是目录、符号链接、设备文件，
// 或经既有符号链接逃逸时，整批 pull 都必须零写入中止。
func buildDrivePullPreflight(absDir string, remote map[string]*remoteFile) map[string]string {
	failures := buildMirrorPreflight(nil, nil, remote, nil)
	addLocalTargetPreflightFailures(absDir, remote, failures)
	return failures
}

// buildDrivePullPreflightFromPlan 只从 list_files 前已固定的根计划读取本地状态。
// existing 根不再按 absDir 重开路径；missing 根在 ancestor Root 下仍必须完整缺失。
func buildDrivePullPreflightFromPlan(plan *pinnedPullRootPlan, remote map[string]*remoteFile) (map[string]string, map[string]os.FileInfo, error) {
	failures := buildMirrorPreflight(nil, nil, remote, nil)
	localInfo := make(map[string]os.FileInfo, len(remote))
	if err := plan.verify(); err != nil {
		return nil, nil, err
	}
	if plan.existing != nil {
		addPinnedLocalTargetPreflightFailures(plan.existing, remote, failures)
		for rel := range remote {
			if mirrorPreflightFailureForRel(rel, failures) != "" {
				continue
			}
			info, err := pullRootLstat(plan.existing.root, filepath.FromSlash(rel))
			if err == nil && info.Mode().IsRegular() {
				localInfo[rel] = info
			}
		}
	}
	if err := plan.verify(); err != nil {
		return nil, nil, err
	}
	return failures, localInfo, nil
}

func addLocalTargetPreflightFailures(absDir string, remote map[string]*remoteFile, failures map[string]string) {
	for rel := range remote {
		// 已被名称/类型冲突根屏蔽的后代不重复计错；一个冲突根只报告一次。
		if mirrorPreflightFailureForRel(rel, failures) != "" {
			continue
		}
		localPath, err := resolveLocalTarget(absDir, rel)
		if err == nil {
			err = rejectNonRegularLocalTarget(localPath)
		}
		if err != nil {
			failures[rel] = err.Error()
		}
	}
}

func drivePullPreflightFailureResult(relPaths []string, preflight map[string]string) drivePullResult {
	res := drivePullResult{Items: make([]drivePullItem, 0, len(relPaths))}
	for _, rel := range relPaths {
		msg := mirrorPreflightFailureForRel(rel, preflight)
		if msg == "" {
			msg = "另一镜像条目未通过预检，整批拉取已中止"
		}
		res.Summary.Failed++
		res.Items = append(res.Items, drivePullItem{RelPath: rel, Action: pullActionFailed, Error: msg})
	}
	return res
}

func mirrorPreflightFailureForRel(rel string, preflight map[string]string) string {
	for rel != "" {
		if msg := preflight[rel]; msg != "" {
			return msg
		}
		rel, _ = splitRel(rel)
	}
	return ""
}

func pullRemoteFile(ctx context.Context, spaceID string, rf *remoteFile, absDir, rel, ifExists string) (string, error) {
	return pullOneFileAtRoot(ctx, spaceID, rf, absDir, rel, ifExists)
}

func pullRemoteFilePinned(ctx context.Context, spaceID string, rf *remoteFile, root *pinnedPullRoot, rel, ifExists string) (string, error) {
	target, err := root.openTarget(rel)
	if err != nil {
		return pullActionFailed, err
	}
	defer target.close()
	return pullOneFilePinned(ctx, spaceID, rf, target, ifExists)
}

// pullOneFile 处理单个远端文件：按 --if-exists 决定是否跳过，否则下载到 localPath。
// 返回动作分类（downloaded / skipped / failed）及失败时的 error。
func pullOneFile(ctx context.Context, spaceID string, rf *remoteFile, localPath, ifExists string) (string, error) {
	return pullOneFileAtRoot(ctx, spaceID, rf, filepath.Dir(localPath), filepath.Base(localPath), ifExists)
}

func pullOneFileAtRoot(ctx context.Context, spaceID string, rf *remoteFile, absDir, rel, ifExists string) (string, error) {
	root, err := openPinnedPullRoot(absDir)
	if err != nil {
		return pullActionFailed, err
	}
	defer root.close()
	target, err := root.openTarget(rel)
	if err != nil {
		return pullActionFailed, err
	}
	defer target.close()
	return pullOneFilePinned(ctx, spaceID, rf, target, ifExists)
}

func pullOneFilePinned(ctx context.Context, spaceID string, rf *remoteFile, target *pinnedPullTarget, ifExists string) (string, error) {
	// 镜像文件的目标若已存在，必须是常规文件。目录、设备、FIFO 等都不能交给
	// download_file / 临时文件写入流程，否则 dry-run 与执行结论会分叉，且目录目标
	// 会在发出远端读请求后才失败。
	initialInfo, err := target.regularTargetInfo()
	if err != nil {
		return pullActionFailed, err
	}
	// 本地已存在常规文件时，按策略判断是否跳过下载。
	if initialInfo != nil {
		switch ifExists {
		case ifExistsSkip:
			return pullActionSkipped, nil
		case ifExistsSmart:
			// 远端时间可信且本地 mtime 已 ≥ 远端 → 视为已对齐，跳过；
			// 时间缺失/非法时不盲跳，退回继续下载。
			if rf.ModifiedTimeValid && initialInfo.ModTime().UnixMilli() >= rf.ModifiedTime {
				return pullActionSkipped, nil
			}
		case ifExistsOverwrite:
			// 总是下载覆盖。
		}
	}

	// download_file → 拿到带签名的下载 URL 与请求头，再 HTTP GET 落盘。
	args := map[string]any{"fileId": rf.FileID}
	if spaceID != "" {
		args["spaceId"] = spaceID
	}
	text, err := callMCPToolReturnText(ctx, "download_file", args)
	if err != nil {
		return pullActionFailed, err
	}
	resourceURL, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		return pullActionFailed, err
	}

	// download_file 可执行任意时长的远端读。在任何 HTTP GET 或临时文件写入前，
	// 必须确认目标父目录仍是最初固定在本地根下的同一目录。
	if err := target.verifyParent(); err != nil {
		return pullActionFailed, err
	}

	// 临时文件通过已固定的 os.Root 直接打开，下载直接写已打开的句柄；
	// 生产路径不会再按可被换链的绝对路径重新打开临时文件。
	tmp, tmpName, err := pullCreateTemp(target.parentRoot)
	if err != nil {
		return pullActionFailed, fmt.Errorf("创建临时文件失败: %w", err)
	}
	// 除非成功 rename，否则始终清理临时文件，不留半成品。
	committed := false
	tmpInfo, err := tmp.Stat()
	if err != nil {
		_ = tmp.Close()
		_ = target.parentRoot.Remove(tmpName)
		return pullActionFailed, fmt.Errorf("读取临时文件身份失败: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = target.parentRoot.Remove(tmpName)
		}
	}()

	if err := pullDownloadFile(ctx, resourceURL, headers, tmp); err != nil {
		return pullActionFailed, err
	}
	// 通过仍打开的文件句柄设置 mtime，避免按 tmpName 重新解析路径产生 symlink TOCTOU。
	// 时间对齐延续既有 best-effort 语义，失败不影响已完整下载内容的发布。
	if rf.ModifiedTimeValid {
		_ = setPullFileTimes(tmp, time.UnixMilli(rf.ModifiedTime))
	}
	if err := pullSyncTemp(tmp); err != nil {
		return pullActionFailed, fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := pullCloseTemp(tmp); err != nil {
		return pullActionFailed, fmt.Errorf("关闭临时文件失败: %w", err)
	}
	currentTmp, err := pullRootLstat(target.parentRoot, tmpName)
	if err != nil || !os.SameFile(tmpInfo, currentTmp) {
		return pullActionFailed, fmt.Errorf("临时文件在下载期间被替换，已中止发布")
	}
	// 网络传输期间父目录仍可能被移走或替换。发布前再核对父目录身份和
	// 目标类型；不一致时只删除固定目录内的临时文件，绝不发布。
	if err := target.verifyParent(); err != nil {
		return pullActionFailed, err
	}
	currentTarget, err := target.regularTargetInfo()
	if err != nil {
		return pullActionFailed, err
	}
	if ifExists != ifExistsOverwrite {
		if currentTarget != nil && (initialInfo == nil || !os.SameFile(initialInfo, currentTarget)) {
			return pullActionSkipped, nil
		}
		if ifExists == ifExistsSmart && currentTarget != nil && rf.ModifiedTimeValid && currentTarget.ModTime().UnixMilli() >= rf.ModifiedTime {
			return pullActionSkipped, nil
		}
	}
	if currentTarget == nil && ifExists != ifExistsOverwrite {
		if err := pullLink(target.parentRoot, tmpName, target.name); err != nil {
			if concurrent, statErr := target.regularTargetInfo(); statErr == nil && concurrent != nil {
				return pullActionSkipped, nil
			}
			return pullActionFailed, fmt.Errorf("发布目标文件失败: %w", err)
		}
		if err := pullRemove(target.parentRoot, tmpName); err != nil {
			return pullActionFailed, fmt.Errorf("清理临时文件失败: %w", err)
		}
	} else if err := pullRename(target.parentRoot, tmpName, target.name); err != nil {
		return pullActionFailed, fmt.Errorf("替换目标文件失败: %w", err)
	}
	finalInfo, err := pullRootLstat(target.parentRoot, target.name)
	if err != nil || !os.SameFile(tmpInfo, finalInfo) {
		// 身份不匹配意味着发布后 terminal entry 可能已被并发替换；保留未知对象并报错，
		// 不做 Lstat→Remove 的第二次 check/use，以免误删用户并发创建的文件。
		return pullActionFailed, fmt.Errorf("发布后的目标文件身份不一致")
	}
	if err := target.verifyParent(); err != nil {
		// 固定目录已从命令根路径移走时，保留句柄内已完成的结果并报失败；不能无条件
		// Remove，因为发布后同名 terminal entry 仍可能被并发替换。
		return pullActionFailed, err
	}
	committed = true
	return pullActionDownloaded, nil
}

type pinnedPullRoot struct {
	absDir   string
	root     *os.Root
	rootInfo os.FileInfo
	pathInfo os.FileInfo
}

// pinnedPullRootPlan 在远端清单读取前固定 pull 的本地根状态。existing 非空表示
// absDir 当时已存在；否则 ancestor 固定最近既存祖先，missing 保存从祖先到 absDir
// 的逐层目录名。missing 只有在完整远端清单和本地预检通过后才允许物化。
type pinnedPullRootPlan struct {
	absDir   string
	existing *pinnedPullRoot
	ancestor *pinnedPullRoot
	missing  []string
}

type pinnedPullTarget struct {
	base        *pinnedPullRoot
	parentRoot  *os.Root
	parentChain []pinnedPullDirIdentity
	name        string
	ownsBase    bool
}

type pinnedPullDirIdentity struct {
	rel  string
	info os.FileInfo
}

func planPinnedPullRoot(absDir string) (*pinnedPullRootPlan, error) {
	info, err := pullPathStat(absDir)
	if err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("创建本地目录失败: %s 已存在且不是目录", absDir)
		}
		root, openErr := openExistingPinnedPullRoot(absDir)
		if openErr != nil {
			return nil, openErr
		}
		return &pinnedPullRootPlan{absDir: absDir, existing: root}, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("检查本地目录失败: %w", err)
	}

	ancestorPath := absDir
	missing := make([]string, 0, 4)
	for {
		parent := filepath.Dir(ancestorPath)
		// absDir 已是验证过的绝对路径；文件系统根目录始终存在，
		// 因此循环必然在到达 parent == ancestorPath 之前以 Stat 成功结束。
		name := filepath.Base(ancestorPath)
		missing = append([]string{name}, missing...)
		ancestorPath = parent
		info, statErr := pullPathStat(ancestorPath)
		if statErr == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("创建本地目录失败: %s 已存在且不是目录", ancestorPath)
			}
			break
		}
		if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("检查本地目录失败: %w", statErr)
		}
	}
	ancestor, openErr := openExistingPinnedPullRoot(ancestorPath)
	if openErr != nil {
		return nil, openErr
	}
	plan := &pinnedPullRootPlan{absDir: absDir, ancestor: ancestor, missing: missing}
	if verifyErr := plan.verify(); verifyErr != nil {
		plan.close()
		return nil, verifyErr
	}
	return plan, nil
}

func (plan *pinnedPullRootPlan) verify() error {
	if plan.existing != nil {
		return plan.existing.verify()
	}
	if plan.ancestor == nil {
		return fmt.Errorf("本地根目录计划无效")
	}
	if err := plan.ancestor.verify(); err != nil {
		return err
	}
	prefix := ""
	for _, segment := range plan.missing {
		prefix = filepath.Join(prefix, segment)
		_, err := pullRootLstat(plan.ancestor.root, prefix)
		if err == nil {
			return fmt.Errorf("本地根目录 %q 在远端读取期间被占用，已中止写入", plan.absDir)
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("检查本地根目录计划失败: %w", err)
		}
		// 第一层缺失后更深的路径不可能存在；物化时每层 Mkdir 仍以 no-clobber
		// 语义拒绝 verify 与创建之间的并发抢占。
		break
	}
	return nil
}

func (plan *pinnedPullRootPlan) materialize() (*pinnedPullRoot, error) {
	if err := plan.verify(); err != nil {
		return nil, err
	}
	if plan.existing != nil {
		root := plan.existing
		plan.existing = nil
		return root, nil
	}

	current, err := pullOpenParentRoot(plan.ancestor.root, ".")
	if err != nil {
		return nil, fmt.Errorf("固定本地根祖先失败: %w", err)
	}
	for _, segment := range plan.missing {
		if err := pullRootMkdir(current, segment, 0o755); err != nil {
			_ = current.Close()
			return nil, fmt.Errorf("创建本地目录失败: %w", err)
		}
		createdInfo, err := pullRootLstat(current, segment)
		if err != nil || !createdInfo.IsDir() {
			_ = current.Close()
			return nil, fmt.Errorf("读取新建本地目录身份失败")
		}
		next, err := pullOpenParentRoot(current, segment)
		if err != nil {
			_ = current.Close()
			return nil, fmt.Errorf("固定新建本地目录失败: %w", err)
		}
		nextInfo, statErr := pullRootStat(next, ".")
		if statErr != nil || !os.SameFile(createdInfo, nextInfo) {
			_ = next.Close()
			_ = current.Close()
			return nil, fmt.Errorf("新建本地目录在固定期间被替换，已中止写入")
		}
		_ = current.Close()
		current = next
	}
	rootInfo, err := pullRootStat(current, ".")
	if err != nil {
		_ = current.Close()
		return nil, fmt.Errorf("读取本地根目录身份失败: %w", err)
	}
	pathInfo, err := pullPathLstat(plan.absDir)
	if err != nil {
		_ = current.Close()
		return nil, fmt.Errorf("读取本地根目录路径身份失败: %w", err)
	}
	root := &pinnedPullRoot{absDir: plan.absDir, root: current, rootInfo: rootInfo, pathInfo: pathInfo}
	if err := root.verify(); err != nil {
		root.close()
		return nil, err
	}
	return root, nil
}

func (plan *pinnedPullRootPlan) close() {
	if plan.existing != nil {
		plan.existing.close()
		plan.existing = nil
	}
	if plan.ancestor != nil {
		plan.ancestor.close()
		plan.ancestor = nil
	}
}

func openPinnedPullRoot(absDir string) (*pinnedPullRoot, error) {
	if err := pullMkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建本地目录失败: %w", err)
	}
	return openExistingPinnedPullRoot(absDir)
}

// openExistingPinnedPullRoot 只固定已经存在的本地目录。push/sync 必须用这一入口：
// 本地源根不存在时直接失败，不能为了扫描或上传隐式创建一个空源目录。
func openExistingPinnedPullRoot(absDir string) (*pinnedPullRoot, error) {
	baseRoot, err := pullOpenRoot(absDir)
	if err != nil {
		return nil, fmt.Errorf("打开本地根目录失败: %w", err)
	}
	rootInfo, err := pullRootStat(baseRoot, ".")
	if err != nil {
		_ = baseRoot.Close()
		return nil, fmt.Errorf("读取本地根目录身份失败: %w", err)
	}
	pathInfo, err := pullPathLstat(absDir)
	if err != nil {
		_ = baseRoot.Close()
		return nil, fmt.Errorf("读取本地根目录路径身份失败: %w", err)
	}
	root := &pinnedPullRoot{absDir: absDir, root: baseRoot, rootInfo: rootInfo, pathInfo: pathInfo}
	if err := root.verify(); err != nil {
		root.close()
		return nil, err
	}
	return root, nil
}

func openPinnedPullTarget(absDir, rel string) (*pinnedPullTarget, error) {
	root, err := openPinnedPullRoot(absDir)
	if err != nil {
		return nil, err
	}
	target, err := root.openTarget(rel)
	if err != nil {
		root.close()
		return nil, err
	}
	target.ownsBase = true
	return target, nil
}

func (root *pinnedPullRoot) openTarget(rel string) (*pinnedPullTarget, error) {
	if err := root.verify(); err != nil {
		return nil, err
	}
	localPath := filepath.Join(root.absDir, filepath.FromSlash(rel))
	localRel, err := pullRelPath(root.absDir, localPath)
	if err != nil || localRel == ".." || strings.HasPrefix(localRel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("远端路径 %q 逃逸出本地根目录", rel)
	}
	parentRel := filepath.Dir(localRel)
	parentRoot, parentChain, err := root.openTargetParent(parentRel)
	if err != nil {
		return nil, err
	}
	target := &pinnedPullTarget{
		base: root, parentRoot: parentRoot, parentChain: parentChain,
		name: filepath.Base(localRel),
	}
	if err := target.verifyParent(); err != nil {
		target.close()
		return nil, err
	}
	return target, nil
}

// openTargetParent 从固定根开始逐层创建并打开目标父目录。os.Root 会安全地阻止
// 软链接逃逸根外，但允许跟随仍指向根内的相对软链接；镜像语义不能接受这种重定向，
// 否则 remote sub/a.txt 可能覆盖同一根下 victim/a.txt。因此每一层都必须：
//
//  1. 以 Lstat 拒绝软链接 / junction 及非目录；
//  2. 通过上一层已固定的 Root 打开下一层；
//  3. 再以 Lstat + SameFile 证明打开的就是目录项本身，而不是竞态中换入的链接目标。
//
// parentChain 保存每一层目录项身份，后续每次 verifyParent 都从命令根重新复核。
func (root *pinnedPullRoot) openTargetParent(parentRel string) (*os.Root, []pinnedPullDirIdentity, error) {
	current, err := pullOpenParentRoot(root.root, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("固定本地目标目录失败: %w", err)
	}
	chain := make([]pinnedPullDirIdentity, 0, 4)
	if parentRel == "." {
		info, err := pullRootLstat(root.root, ".")
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			_ = current.Close()
			return nil, nil, fmt.Errorf("本地目标目录不是可固定的真实目录")
		}
		openedInfo, statErr := pullRootStat(current, ".")
		if statErr != nil || !os.SameFile(info, openedInfo) {
			_ = current.Close()
			return nil, nil, fmt.Errorf("本地目标目录在固定期间被替换，已中止写入")
		}
		chain = append(chain, pinnedPullDirIdentity{rel: ".", info: info})
		return current, chain, nil
	}

	prefix := ""
	for _, segment := range strings.Split(parentRel, string(filepath.Separator)) {
		info, lstatErr := pullRootLstat(current, segment)
		if os.IsNotExist(lstatErr) {
			if mkdirErr := pullRootMkdir(current, segment, 0o755); mkdirErr != nil {
				_ = current.Close()
				return nil, nil, fmt.Errorf("创建本地目录失败: %w", mkdirErr)
			}
			info, lstatErr = pullRootLstat(current, segment)
		}
		if lstatErr != nil {
			_ = current.Close()
			return nil, nil, fmt.Errorf("检查本地目标目录失败: %w", lstatErr)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			_ = current.Close()
			return nil, nil, fmt.Errorf("本地目标目录 %q 已存在且不是可固定的真实目录，已拒绝镜像", filepath.Join(prefix, segment))
		}

		next, openErr := pullOpenParentRoot(current, segment)
		if openErr != nil {
			_ = current.Close()
			return nil, nil, fmt.Errorf("固定本地目标目录失败: %w", openErr)
		}
		openedInfo, statErr := pullRootStat(next, ".")
		currentInfo, currentErr := pullRootLstat(current, segment)
		if statErr != nil || currentErr != nil || !currentInfo.IsDir() || currentInfo.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(info, currentInfo) || !os.SameFile(currentInfo, openedInfo) {
			_ = next.Close()
			_ = current.Close()
			return nil, nil, fmt.Errorf("本地目标目录在固定期间被替换，已中止写入")
		}
		prefix = filepath.Join(prefix, segment)
		chain = append(chain, pinnedPullDirIdentity{rel: prefix, info: currentInfo})
		_ = current.Close()
		current = next
	}
	return current, chain, nil
}

func (root *pinnedPullRoot) verify() error {
	current, err := pullPathStat(root.absDir)
	currentPath, pathErr := pullPathLstat(root.absDir)
	if err != nil || pathErr != nil || !os.SameFile(root.rootInfo, current) || !os.SameFile(root.pathInfo, currentPath) {
		return fmt.Errorf("本地根目录在同步期间被替换，已中止写入")
	}
	return nil
}

func (root *pinnedPullRoot) close() { _ = root.root.Close() }

func (target *pinnedPullTarget) close() {
	_ = target.parentRoot.Close()
	if target.ownsBase {
		target.base.close()
	}
}

func (target *pinnedPullTarget) verifyParent() error {
	if err := target.base.verify(); err != nil {
		return err
	}
	for _, expected := range target.parentChain {
		current, err := pullRootLstat(target.base.root, expected.rel)
		if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected.info, current) {
			return fmt.Errorf("本地目标目录在下载期间被替换，已中止写入")
		}
	}
	return nil
}

func (target *pinnedPullTarget) regularTargetInfo() (os.FileInfo, error) {
	info, err := pullRootLstat(target.parentRoot, target.name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("检查本地目标失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("本地目标 %q 已存在且不是常规文件，已拒绝覆盖", target.name)
	}
	return info, nil
}

func createPinnedPullTemp(root *os.Root) (*os.File, string, error) {
	for i := 0; i < 100; i++ {
		name := ".dws-pull-" + pullTempName()
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("创建不重名临时文件失败")
}

func defaultPullDownloadFile(ctx context.Context, url string, headers map[string]string, destination *os.File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := pullHTTPDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	_, err = io.Copy(destination, resp.Body)
	return err
}

// rejectNonRegularLocalTarget 在任何下载工具调用或临时文件创建之前，拒绝已存在但
// 不是常规文件的镜像目标。不存在的目标可由后续流程安全创建。
func rejectNonRegularLocalTarget(localPath string) error {
	info, err := pullTargetLstat(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("检查本地目标失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("本地目标 %q 已存在且不是常规文件，已拒绝覆盖", localPath)
	}
	return nil
}

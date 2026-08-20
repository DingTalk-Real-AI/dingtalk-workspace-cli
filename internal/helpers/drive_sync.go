package helpers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// keep-both 的本地保留副本通过同一个固定父目录 os.Root 原子建立硬链接；以下 seam
// 仅用于文件系统失败与并发替换的确定性测试，测试必须通过 testseam.Swap 替换。
var (
	syncRootLink          = func(root *os.Root, oldName, newName string) error { return root.Link(oldName, newName) }
	syncPullOneFilePinned = pullOneFilePinned
)

// ==========================================================
// drive sync — 本地与钉盘双向同步（本地 ⇄ Drive）
// ==========================================================
// ──────────────────────────────────────────────────────────
// dws drive sync — 把 --local-folder 与 --remote-folder 做文件级双向同步。
//
// 复用 status 的差异判定（exact MD5 / --quick modified_time）先算出 diff，再按方向执行：
//   new_local   仅本地存在   → push（缺失的远端目录按需创建，再上传）
//   new_remote  仅钉盘存在   → pull（下载到本地对应路径）
//   modified    两侧都变更   → 按 --on-conflict 解决：
//                 skip       默认：两侧都不动并保留两边内容
//                 remote-wins     ：拉取远端覆盖本地
//                 local-wins       ：上传本地覆盖远端
//                 keep-both        ：本地改名保留，再拉取远端到原路径
//                 ask              ：交互式逐个询问
//   unknown     exact 模式远端无可靠 md5、内容无法核对 → 跳过（记 skipped，提示改用 --quick）
//   unchanged   两侧一致                                → 不动
//
// 文件级同步——只新增/覆盖，不删除任何一侧的多余文件。summary.failed > 0 时以非零
// 退出码退出，结构化 summary + diff + items 仍打印在 stdout 上。
// ──────────────────────────────────────────────────────────

// --on-conflict 的五种冲突解决策略。
const (
	syncConflictLocalWins  = "local-wins"  // 上传本地覆盖远端
	syncConflictRemoteWins = "remote-wins" // 拉取远端覆盖本地
	syncConflictKeepBoth   = "keep-both"   // 本地改名保留，再拉取远端到原路径
	syncConflictAsk        = "ask"         // 交互式逐个询问
	syncConflictSkip       = "skip"        // 默认：两侧都变更时什么都不做，两边内容都保留
)

// sync 动作分类（direction 标注每条记录属于哪个方向）。
const (
	syncDirectionPull     = "pull"
	syncDirectionPush     = "push"
	syncDirectionConflict = "conflict"

	syncActionDownloaded    = "downloaded"
	syncActionUploaded      = "uploaded"
	syncActionOverwritten   = "overwritten"
	syncActionFolderCreated = "folder_created"
	syncActionRenamedLocal  = "renamed_local"
	syncActionSkipped       = "skipped"
	syncActionFailed        = "failed"

	syncActionPlannedDownload     = "planned_download"
	syncActionPlannedUpload       = "planned_upload"
	syncActionPlannedOverwrite    = "planned_overwrite"
	syncActionPlannedFolderCreate = "planned_folder_create"
	syncActionPlannedRenameLocal  = "planned_rename_local"
	syncActionPlannedSkip         = "planned_skip"
	syncActionDecisionRequired    = "decision_required"
)

// driveSyncItem 是输出 items[] 中每条操作的明细。
type driveSyncItem struct {
	RelPath   string `json:"rel_path"`
	Action    string `json:"action"`
	Direction string `json:"direction,omitempty"`
	Error     string `json:"error,omitempty"`
}

// driveSyncSummary 是各方向的计数汇总。
type driveSyncSummary struct {
	Pulled         int `json:"pulled"`
	Pushed         int `json:"pushed"`
	Skipped        int `json:"skipped"`
	Failed         int `json:"failed"`
	PlannedPulls   int `json:"planned_pulls,omitempty"`
	PlannedPushes  int `json:"planned_pushes,omitempty"`
	PlannedSkips   int `json:"planned_skips,omitempty"`
	PlannedFolders int `json:"planned_folders,omitempty"`
}

// driveSyncDiff 是本次同步前算出的五类差异（与 status 同源）。
type driveSyncDiff struct {
	NewLocal  []driveStatusEntry `json:"new_local"`
	NewRemote []driveStatusEntry `json:"new_remote"`
	Modified  []driveStatusEntry `json:"modified"`
	Unchanged []driveStatusEntry `json:"unchanged"`
	Unknown   []driveStatusEntry `json:"unknown"`
}

// driveSyncResult 是 sync 命令的输出 schema。
type driveSyncResult struct {
	Detection string           `json:"detection"`
	Diff      driveSyncDiff    `json:"diff"`
	Summary   driveSyncSummary `json:"summary"`
	Items     []driveSyncItem  `json:"items"`
}

type driveSyncDryRunResult struct {
	DryRun      bool            `json:"dry_run"`
	Executed    bool            `json:"executed"`
	PreviewKind string          `json:"preview_kind"`
	Operation   string          `json:"operation"`
	Plan        driveSyncResult `json:"plan"`
}

// driveSyncFailure 在 summary.failed > 0 时返回：结构化结果已打印到 stdout，
// 这里只负责以 exit=1 退出并向 stderr 输出一行简短说明（与 drivePushFailure 一致）。
type driveSyncFailure struct{ failed int }

func (e *driveSyncFailure) Error() string {
	return fmt.Sprintf("drive sync: %d item(s) failed", e.failed)
}
func (e *driveSyncFailure) RawStderr() string { return e.Error() }
func (e *driveSyncFailure) ExitCode() int     { return 1 }

func runDriveSync(cmd *cobra.Command, _ []string) error {
	if err := validateRequiredFlags(cmd, "local-folder", "remote-folder"); err != nil {
		return err
	}
	localDir := mustGetFlag(cmd, "local-folder")
	remoteDirID := mustGetFlag(cmd, "remote-folder")
	// space-id 可选：不传则由底层使用「我的文件」对应的空间。
	spaceID := mustGetFlag(cmd, "space-id")

	onConflict, _ := cmd.Flags().GetString("on-conflict")
	if onConflict == "" {
		// 安全默认：两侧都变更时不擅自覆盖任何一侧。
		onConflict = syncConflictSkip
	}
	switch onConflict {
	case syncConflictSkip, syncConflictLocalWins, syncConflictRemoteWins, syncConflictKeepBoth, syncConflictAsk:
	default:
		return fmt.Errorf("--on-conflict 取值非法: %s（可选 skip|local-wins|remote-wins|keep-both|ask）", onConflict)
	}

	quick, _ := cmd.Flags().GetBool("quick")
	detection := "exact"
	if quick {
		detection = "quick"
	}

	absDir, err := validateLocalDirAbs(localDir)
	if err != nil {
		return err
	}
	pinnedRoot, err := openExistingPinnedPullRoot(absDir)
	if err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}
	defer pinnedRoot.close()
	// sync 的本地快照与后续 hash / upload / pull 全部绑定到同一个固定根。
	localDirs, localFiles, err := runDriveMirrorWalkLocalForPushPinned(pinnedRoot)
	if err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}

	ctx := cmd.Context()

	// 远端现状：文件（含 hash/mtime/fileId）与目录（rel_path → fileId，"" 即根本身）。
	remoteFiles, remoteFolders, err := fetchRemoteTreeForPush(ctx, spaceID, remoteDirID)
	if err != nil {
		return err
	}
	localByRel := make(map[string]localPushFile, len(localFiles))
	for _, f := range localFiles {
		localByRel[f.RelPath] = f
	}
	preflight := buildMirrorPreflight(localDirs, localFiles, remoteFiles, remoteFolders)
	addPinnedLocalTargetPreflightFailures(pinnedRoot, remoteFiles, preflight)

	// 在计算 diff 前先识别文件/目录同路径冲突。冲突根及其全部后代都不能再进入
	// new_local / new_remote 或后续执行阶段，否则会在远端制造同名异类对象，或尝试
	// 把远端目录内容下载到本地文件之下。
	typeConflicts := make(map[string]string)
	for rel, msg := range preflight {
		typeConflicts[rel] = msg
	}
	for _, dir := range localDirs {
		if msg := pushTypeConflictError(dir, true, remoteFiles, remoteFolders); msg != "" {
			typeConflicts[dir] = msg
		}
	}
	for rel := range localByRel {
		if msg := pushTypeConflictError(rel, false, remoteFiles, remoteFolders); msg != "" {
			typeConflicts[rel] = msg
		}
	}

	// 计算 diff：双端都存在的文件复用 judgeFileMatch，保持与 status 完全一致的判定。
	diff := driveSyncDiff{
		NewLocal:  []driveStatusEntry{},
		NewRemote: []driveStatusEntry{},
		Modified:  []driveStatusEntry{},
		Unchanged: []driveStatusEntry{},
		Unknown:   []driveStatusEntry{},
	}
	var newLocal, newRemote, modified, unknown []string
	for rel, pf := range localByRel {
		if syncPathBlockedByTypeConflict(rel, typeConflicts) {
			continue
		}
		rf, ok := remoteFiles[rel]
		if !ok {
			newLocal = append(newLocal, rel)
			continue
		}
		verdict, jerr := judgePinnedPushFileMatch(pinnedRoot, pf, rf, quick)
		if jerr != nil {
			return jerr
		}
		switch verdict {
		case matchUnchanged:
			diff.Unchanged = append(diff.Unchanged, driveStatusEntry{RelPath: rel})
		case matchUnknown:
			unknown = append(unknown, rel)
		default:
			modified = append(modified, rel)
		}
	}
	for rel := range remoteFiles {
		if syncPathBlockedByTypeConflict(rel, typeConflicts) {
			continue
		}
		if _, ok := localByRel[rel]; !ok {
			newRemote = append(newRemote, rel)
		}
	}
	sort.Strings(newLocal)
	sort.Strings(newRemote)
	sort.Strings(modified)
	sort.Strings(unknown)
	for _, rel := range newLocal {
		diff.NewLocal = append(diff.NewLocal, driveStatusEntry{RelPath: rel})
	}
	for _, rel := range newRemote {
		diff.NewRemote = append(diff.NewRemote, driveStatusEntry{RelPath: rel})
	}
	for _, rel := range modified {
		diff.Modified = append(diff.Modified, driveStatusEntry{RelPath: rel})
	}
	for _, rel := range unknown {
		diff.Unknown = append(diff.Unknown, driveStatusEntry{RelPath: rel})
	}
	sortEntries(diff.Unchanged)

	res := driveSyncResult{Detection: detection, Diff: diff, Items: []driveSyncItem{}}
	conflictRels := make([]string, 0, len(typeConflicts))
	for rel := range typeConflicts {
		conflictRels = append(conflictRels, rel)
	}
	sort.Strings(conflictRels)
	for _, rel := range conflictRels {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{
			RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict,
			Error: typeConflicts[rel],
		})
	}
	if err := pinnedRoot.verify(); err != nil {
		return err
	}

	// --dry-run：只算差异、不执行任何同步动作。
	if deps.Caller.DryRun() {
		if len(preflight) == 0 && res.Summary.Failed == 0 {
			appendDriveSyncDryRunPlan(&res, driveSyncDryRunPlanInput{
				LocalDirs: localDirs, LocalByRel: localByRel, RemoteFiles: remoteFiles, RemoteFolders: remoteFolders,
				NewLocal: newLocal, NewRemote: newRemote, Modified: modified, Unknown: unknown, OnConflict: onConflict,
			})
		}
		return deps.Out.PrintJSON(driveSyncDryRunResult{
			DryRun: true, Executed: false, PreviewKind: "plan", Operation: "drive sync", Plan: res,
		})
	}
	// 镜像前置检查是整批门禁：发现任一本地非法名称或跨端等价路径歧义后，
	// sync 不得执行 create/download/upload 等任一写动作。
	if len(preflight) > 0 {
		if perr := deps.Out.PrintJSON(res); perr != nil {
			return perr
		}
		return &driveSyncFailure{failed: res.Summary.Failed}
	}

	// unknown：exact 模式下远端无可靠 md5、内容无法核对，不擅自覆盖任何一侧，记 skipped。
	for _, rel := range unknown {
		res.Summary.Skipped++
		res.Items = append(res.Items, driveSyncItem{
			RelPath: rel, Action: syncActionSkipped, Direction: syncDirectionConflict,
			Error: "远端无可靠 md5，内容无法核对，已跳过（可改用 --quick 按 modified_time 比对）",
		})
	}

	// ask 模式：先把所有 modified 的解决策略问全，再统一执行，避免边执行边交互。
	resolutions := make(map[string]string, len(modified))
	for _, rel := range modified {
		strategy := onConflict
		switch strategy {
		case syncConflictAsk:
			strategy, err = driveSyncAskConflict(rel)
			if err != nil {
				return err
			}
		case syncConflictSkip:
			strategy = "" // 与 ask 选择「跳过」同一条落地路径
		}
		resolutions[rel] = strategy
	}
	// 交互决策期间本地根也可能被替换；首个双端写动作前再次核对固定身份。
	if err := pinnedRoot.verify(); err != nil {
		return err
	}

	// occupied：keep-both 生成不冲突的本地重命名目标时用，覆盖两侧全部已知路径。
	occupied := make(map[string]bool)
	for rel := range localByRel {
		occupied[rel] = true
	}
	for _, d := range localDirs {
		occupied[d] = true
	}
	for rel := range remoteFiles {
		occupied[rel] = true
	}
	for rel := range remoteFolders {
		if rel != "" {
			occupied[rel] = true
		}
	}

	// 阶段 1：镜像本地目录结构到远端（缺失则创建），保证空目录与 push 目标的父目录先存在。
	for _, dir := range localDirs {
		if _, ok := remoteFolders[dir]; ok {
			continue // 远端已存在，复用 fileId
		}
		parentRel, name := splitRel(dir)
		parentID, ok := remoteFolders[parentRel]
		if !ok || parentID == "" {
			res.Summary.Failed++
			res.Items = append(res.Items, driveSyncItem{RelPath: dir, Action: syncActionFailed, Direction: syncDirectionPush, Error: "父目录未能创建"})
			continue
		}
		if err := pinnedRoot.verify(); err != nil {
			return err
		}
		fid, cerr := pushCreateFolder(ctx, spaceID, parentID, name)
		if cerr != nil || fid == "" {
			res.Summary.Failed++
			msg := "create_folder 未返回 fileId"
			if cerr != nil {
				msg = cerr.Error()
			}
			res.Items = append(res.Items, driveSyncItem{RelPath: dir, Action: syncActionFailed, Direction: syncDirectionPush, Error: msg})
			continue
		}
		remoteFolders[dir] = fid
		res.Summary.Pushed++
		res.Items = append(res.Items, driveSyncItem{RelPath: dir, Action: syncActionFolderCreated, Direction: syncDirectionPush})
	}

	// 阶段 2：new_remote 下载到本地。
	for _, rel := range newRemote {
		syncPullFilePinned(&res, ctx, spaceID, remoteFiles[rel], pinnedRoot, rel, syncDirectionPull)
	}

	// 阶段 3：new_local 上传到远端。
	for _, rel := range newLocal {
		pf := localByRel[rel]
		syncPushFile(&res, ctx, spaceID, remoteFolders, pinnedRoot, pf, rel, "")
	}

	// 阶段 4：modified 按 --on-conflict 解决。
	for _, rel := range modified {
		pf := localByRel[rel]
		rf := remoteFiles[rel]
		switch resolutions[rel] {
		case syncConflictRemoteWins:
			syncPullFilePinned(&res, ctx, spaceID, rf, pinnedRoot, rel, syncDirectionPull)
		case syncConflictLocalWins:
			syncPushFile(&res, ctx, spaceID, remoteFolders, pinnedRoot, pf, rel, rf.FileID)
		case syncConflictKeepBoth:
			syncKeepBothPinned(&res, ctx, spaceID, rf, pinnedRoot, rel, occupied)
		default: // "" — ask 选择跳过
			res.Summary.Skipped++
			res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionSkipped, Direction: syncDirectionConflict})
		}
	}

	// 结构化结果始终打印到 stdout；有失败则额外以非零退出码退出。
	if perr := deps.Out.PrintJSON(res); perr != nil {
		return perr
	}
	if res.Summary.Failed > 0 {
		return &driveSyncFailure{failed: res.Summary.Failed}
	}
	return nil
}

type driveSyncDryRunPlanInput struct {
	LocalDirs     []string
	LocalByRel    map[string]localPushFile
	RemoteFiles   map[string]*remoteFile
	RemoteFolders map[string]string
	NewLocal      []string
	NewRemote     []string
	Modified      []string
	Unknown       []string
	OnConflict    string
}

func appendDriveSyncDryRunPlan(res *driveSyncResult, input driveSyncDryRunPlanInput) {
	for _, rel := range input.Unknown {
		res.Summary.PlannedSkips++
		res.Items = append(res.Items, driveSyncItem{
			RelPath: rel, Action: syncActionPlannedSkip, Direction: syncDirectionConflict,
			Error: "远端无可靠 md5，内容无法核对，执行时会跳过（可改用 --quick 按 modified_time 比对）",
		})
	}

	plannedFolders := make(map[string]string, len(input.RemoteFolders)+len(input.LocalDirs))
	for rel, fileID := range input.RemoteFolders {
		plannedFolders[rel] = fileID
	}
	for _, dir := range input.LocalDirs {
		if _, ok := plannedFolders[dir]; ok {
			continue
		}
		parentRel, _ := splitRel(dir)
		if plannedFolders[parentRel] == "" {
			res.Summary.Failed++
			res.Items = append(res.Items, driveSyncItem{RelPath: dir, Action: syncActionFailed, Direction: syncDirectionPush, Error: "父目录未能创建"})
			continue
		}
		plannedFolders[dir] = "dry-run-planned-folder"
		res.Summary.PlannedFolders++
		res.Items = append(res.Items, driveSyncItem{RelPath: dir, Action: syncActionPlannedFolderCreate, Direction: syncDirectionPush})
	}

	for _, rel := range input.NewRemote {
		res.Summary.PlannedPulls++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionPlannedDownload, Direction: syncDirectionPull})
	}
	for _, rel := range input.NewLocal {
		parentRel, _ := splitRel(rel)
		if plannedFolders[parentRel] == "" {
			res.Summary.Failed++
			res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionPush, Error: "父目录未能创建"})
			continue
		}
		res.Summary.PlannedPushes++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionPlannedUpload, Direction: syncDirectionPush})
	}

	occupied := make(map[string]bool, len(input.LocalByRel)+len(input.LocalDirs)+len(input.RemoteFiles)+len(input.RemoteFolders))
	for rel := range input.LocalByRel {
		occupied[rel] = true
	}
	for _, rel := range input.LocalDirs {
		occupied[rel] = true
	}
	for rel := range input.RemoteFiles {
		occupied[rel] = true
	}
	for rel := range input.RemoteFolders {
		if rel != "" {
			occupied[rel] = true
		}
	}
	for _, rel := range input.Modified {
		switch input.OnConflict {
		case syncConflictRemoteWins:
			res.Summary.PlannedPulls++
			res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionPlannedDownload, Direction: syncDirectionPull})
		case syncConflictLocalWins:
			res.Summary.PlannedPushes++
			res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionPlannedOverwrite, Direction: syncDirectionConflict})
		case syncConflictKeepBoth:
			candidate := driveSyncSuffixedRel(rel, input.RemoteFiles[rel].FileID, occupied)
			occupied[candidate] = true
			res.Summary.PlannedPulls++
			res.Items = append(res.Items,
				driveSyncItem{RelPath: candidate, Action: syncActionPlannedRenameLocal, Direction: syncDirectionConflict},
				driveSyncItem{RelPath: rel, Action: syncActionPlannedDownload, Direction: syncDirectionPull},
			)
		case syncConflictAsk:
			res.Items = append(res.Items, driveSyncItem{
				RelPath: rel, Action: syncActionDecisionRequired, Direction: syncDirectionConflict,
				Error: "执行时需要交互选择 remote-wins、local-wins、keep-both 或 skip",
			})
		default:
			res.Summary.PlannedSkips++
			res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionPlannedSkip, Direction: syncDirectionConflict})
		}
	}
}

// syncPathBlockedByTypeConflict 判断 rel 本身或任一祖先是否是文件/目录类型冲突根。
// 祖先检查保证冲突目录下的本地、远端后代不会绕过根路径的 fail-closed 结论。
func syncPathBlockedByTypeConflict(rel string, conflicts map[string]string) bool {
	for rel != "" {
		if _, blocked := conflicts[rel]; blocked {
			return true
		}
		rel, _ = splitRel(rel)
	}
	return false
}

// addPinnedLocalTargetPreflightFailures 从 sync 已固定的本地根核对所有远端落盘点。
// 每一层既有祖先都必须是真实目录，末段若存在则必须是常规文件；软链即使仍指向根内
// 也不属于可移植镜像树，必须在任何双端写之前整批拒绝。
func addPinnedLocalTargetPreflightFailures(root *pinnedPullRoot, remote map[string]*remoteFile, failures map[string]string) {
	for rel := range remote {
		if mirrorPreflightFailureForRel(rel, failures) != "" {
			continue
		}
		parts := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
		for i := range parts {
			name := filepath.Join(parts[:i+1]...)
			info, err := pullRootLstat(root.root, name)
			if err != nil {
				if os.IsNotExist(err) {
					break
				}
				failures[rel] = fmt.Sprintf("检查本地目标失败: %v", err)
				break
			}
			if i < len(parts)-1 {
				if !info.IsDir() {
					failures[rel] = fmt.Sprintf("本地目标 %q 的祖先 %q 已存在且不是目录，已拒绝镜像", rel, filepath.ToSlash(name))
					break
				}
				continue
			}
			if !info.Mode().IsRegular() {
				failures[rel] = fmt.Sprintf("本地目标 %q 已存在且不是常规文件，已拒绝覆盖", rel)
			}
		}
	}
}

// syncPullFile 下载单个远端文件到本地 rel 对应路径（总是覆盖，Drive 为该项权威源），
// 并把结果计入 res。命中大小写/规范化冲突或逃逸的路径记 failed、不落盘。
func syncPullFile(res *driveSyncResult, ctx context.Context, spaceID string, rf *remoteFile, absDir, rel, direction string) {
	root, err := openPinnedPullRoot(absDir)
	if err != nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: direction, Error: err.Error()})
		return
	}
	defer root.close()
	syncPullFilePinned(res, ctx, spaceID, rf, root, rel, direction)
}

func syncPullFilePinned(res *driveSyncResult, ctx context.Context, spaceID string, rf *remoteFile, root *pinnedPullRoot, rel string, direction string) {
	action, perr := pullRemoteFilePinned(ctx, spaceID, rf, root, rel, ifExistsOverwrite)
	if action == pullActionDownloaded {
		res.Summary.Pulled++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionDownloaded, Direction: direction})
		return
	}
	res.Summary.Failed++
	item := driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: direction}
	if perr != nil {
		item.Error = perr.Error()
	}
	res.Items = append(res.Items, item)
}

// syncPushFile 上传单个本地文件到远端；overwriteID 非空时走覆盖上传（原地覆盖同名远端文件）。
func syncPushFile(res *driveSyncResult, ctx context.Context, spaceID string, remoteFolders map[string]string, root *pinnedPullRoot, pf localPushFile, rel, overwriteID string) {
	parentRel, name := splitRel(rel)
	parentID, ok := remoteFolders[parentRel]
	if !ok || parentID == "" {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionPush, Error: "父目录未能创建"})
		return
	}
	if err := pushUploadFilePinned(ctx, spaceID, parentID, overwriteID, name, root, pf); err != nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionPush, Error: err.Error()})
		return
	}
	res.Summary.Pushed++
	action := syncActionUploaded
	direction := syncDirectionPush
	if overwriteID != "" {
		action = syncActionOverwritten
		direction = syncDirectionConflict
	}
	res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: action, Direction: direction})
}

// syncKeepBoth 解决 modified 冲突的 keep-both 策略：先为本地文件原子建立带冲突后缀
// 的 no-clobber 硬链接，再把远端文件拉取到原 rel 路径。拉取失败时保留两个本地名字，
// 不执行可能误删或覆盖并发文件的回滚。
func syncKeepBoth(res *driveSyncResult, ctx context.Context, spaceID string, rf *remoteFile, absDir, rel string, occupied map[string]bool) {
	root, err := openPinnedPullRoot(absDir)
	if err != nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: err.Error()})
		return
	}
	defer root.close()
	syncKeepBothPinned(res, ctx, spaceID, rf, root, rel, occupied)
}

func syncKeepBothPinned(res *driveSyncResult, ctx context.Context, spaceID string, rf *remoteFile, root *pinnedPullRoot, rel string, occupied map[string]bool) {
	pinned, e1 := root.openTarget(rel)
	if e1 != nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: e1.Error()})
		return
	}
	defer pinned.close()
	localInfo, err := pinned.regularTargetInfo()
	if err != nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: err.Error()})
		return
	}
	if localInfo == nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: "本地保留版本不存在"})
		return
	}
	// Root.Link(original,candidate) 在同一固定父目录中原子 no-clobber 地保留本地版本。
	// occupied 的精确字符串查重覆盖不到的文件系统等价名称，由 Link 的 EEXIST 兜底。
	suffixedRel, newName, e2 := reserveSyncKeepBothTargetPinned(pinned, rel, rf.FileID, occupied)
	if e2 != nil {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: e2.Error()})
		return
	}
	savedInfo, err := pullRootLstat(pinned.parentRoot, newName)
	if err != nil || !sameKeepBothVersion(localInfo, savedInfo) {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: "本地保留版本身份不一致"})
		return
	}
	occupied[suffixedRel] = true

	action, perr := syncPullOneFilePinned(ctx, spaceID, rf, pinned, ifExistsOverwrite)
	if action != pullActionDownloaded {
		res.Summary.Failed++
		msg := ""
		if perr != nil {
			msg = perr.Error()
		}
		if msg == "" {
			msg = "远端版本未能拉取，本地保留版本已保留"
		}
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: msg})
		return
	}
	currentSaved, err := pullRootLstat(pinned.parentRoot, newName)
	if err != nil || !sameKeepBothVersion(savedInfo, currentSaved) {
		res.Summary.Failed++
		res.Items = append(res.Items, driveSyncItem{RelPath: rel, Action: syncActionFailed, Direction: syncDirectionConflict, Error: "本地保留版本在传输期间被删除、替换或修改"})
		return
	}
	res.Summary.Pulled++
	res.Items = append(res.Items,
		driveSyncItem{RelPath: suffixedRel, Action: syncActionRenamedLocal, Direction: syncDirectionConflict},
		driveSyncItem{RelPath: rel, Action: syncActionDownloaded, Direction: syncDirectionPull},
	)
}

func sameKeepBothVersion(expected, current os.FileInfo) bool {
	return expected != nil && current != nil && current.Mode().IsRegular() &&
		os.SameFile(expected, current) && expected.Size() == current.Size() &&
		expected.ModTime().Equal(current.ModTime())
}

// syncKeepBothCandidate 生成 keep-both 的第 n 个候选相对路径（n=0 为首选）：在扩展名前
// 插入基于远端 fileId 末段的后缀（缺失时用 conflict），n>0 再追加序号消歧。
func syncKeepBothCandidate(rel, fileID string, n int) string {
	suffix := "conflict"
	if fileID != "" {
		s := fileID
		if len(s) > 8 {
			s = s[len(s)-8:]
		}
		suffix = "conflict-" + s
	}
	dir, base := splitRel(rel)
	ext := filepath.Ext(base) // base 无路径分隔符，取末尾扩展名安全
	stem := strings.TrimSuffix(base, ext)
	name := stem + "." + suffix + ext
	if n > 0 {
		name = fmt.Sprintf("%s.%s.%d%s", stem, suffix, n, ext)
	}
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// driveSyncSuffixedRel 返回首个不在 occupied（纯字符串查重）中的候选。仅用于快速路径与
// 单元测试；keep-both 实际执行走 reserveSyncKeepBothTarget，以 OS 兜底等价性。
func driveSyncSuffixedRel(rel, fileID string, occupied map[string]bool) string {
	for n := 0; ; n++ {
		if cand := syncKeepBothCandidate(rel, fileID, n); !occupied[cand] {
			return cand
		}
	}
}

// reserveSyncKeepBothTarget 生成候选并通过 Root.Link(original,candidate) 原子建立
// no-clobber 本地保留版本。EEXIST（含文件系统等价名称）时换下一个候选。
func reserveSyncKeepBothTarget(absDir, rel, fileID string, occupied map[string]bool) (string, string, error) {
	target, err := openPinnedPullTarget(absDir, rel)
	if err != nil {
		return "", "", err
	}
	defer target.close()
	candidate, _, err := reserveSyncKeepBothTargetPinned(target, rel, fileID, occupied)
	if err != nil {
		return "", "", err
	}
	return candidate, filepath.Join(absDir, filepath.FromSlash(candidate)), nil
}

func reserveSyncKeepBothTargetPinned(target *pinnedPullTarget, rel, fileID string, occupied map[string]bool) (string, string, error) {
	parentRel, _ := splitRel(rel)
	for n := 0; ; n++ {
		cand := syncKeepBothCandidate(rel, fileID, n)
		if occupied[cand] {
			continue // 本次运行内已知占用，快速跳过
		}
		candidateParent, candidateName := splitRel(cand)
		if candidateParent != parentRel || !isSafeRemoteSegment(candidateName) {
			return "", "", fmt.Errorf("keep-both 候选路径 %q 不在原目标目录内", cand)
		}
		if err := target.verifyParent(); err != nil {
			return "", "", err
		}
		oerr := syncRootLink(target.parentRoot, target.name, filepath.FromSlash(candidateName))
		if oerr == nil {
			return cand, filepath.FromSlash(candidateName), nil
		}
		if os.IsExist(oerr) {
			occupied[cand] = true // 等价目标已存在，记下避免重试，换下一个后缀
			continue
		}
		return "", "", oerr
	}
}

// syncAskStdin 是 --on-conflict=ask 交互提问的输入源，默认 os.Stdin；测试可替换它以
// 注入非交互（EOF）场景。
var syncAskStdin io.Reader = os.Stdin

// driveSyncAskConflict 在 --on-conflict=ask 下交互式询问单个冲突文件的解决策略。
// 返回四种具体策略之一，或 ""（跳过）。当 stdin 在给出选择前结束（管道/无 TTY 等非交互
// 环境）时，按文档约定等价于「跳过」，返回 ""——绝不因此中止整个同步，new_local/new_remote
// 等其余差异仍会照常处理。
func driveSyncAskConflict(rel string) (string, error) {
	fmt.Fprintf(os.Stderr, "冲突: 本地与远端都修改了 %q。请选择 [R]远端优先 / [L]本地优先 / [K]保留两者 / [S]跳过 (默认 R): ", rel)
	line, err := bufio.NewReader(syncAskStdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("读取冲突选择失败 (%s): %w", rel, err)
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "r", "remote", "remote-wins":
		return syncConflictRemoteWins, nil
	case "l", "local", "local-wins":
		return syncConflictLocalWins, nil
	case "k", "keep", "keep-both":
		return syncConflictKeepBoth, nil
	case "s", "skip":
		return "", nil
	case "":
		if errors.Is(err, io.EOF) {
			// 非交互（管道/无 TTY）：stdin 在给出选择前结束。按文档约定等价于跳过，
			// 返回 "" 而非报错——否则单个冲突会让 runDriveSync 直接中止，连
			// new_local/new_remote 都不再处理。
			return "", nil
		}
		return syncConflictRemoteWins, nil // 交互式回车 → 默认远端优先
	default:
		return "", fmt.Errorf("无效的冲突选择: %q（可选 remote/local/keep/skip）", strings.TrimSpace(line))
	}
}

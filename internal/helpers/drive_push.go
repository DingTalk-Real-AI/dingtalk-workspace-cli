package helpers

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

// ==========================================================
// drive push — 把本地文件夹镜像到钉盘（本地 → Drive）
// ==========================================================
// ──────────────────────────────────────────────────────────
// dws drive push — 把本地目录单向、文件级镜像到钉盘（本地 → Drive）
//
// 递归遍历 --local-folder 下所有常规文件与子目录（含空目录），按相对路径在
// --remote-folder 指向的钉盘文件夹里新建/覆盖/跳过。已存在的远端目录复用其
// fileId、不重建；缺失的目录按需 create_folder。文件按 --if-exists 决定
// skip / smart / overwrite。summary.failed > 0 时以非零退出码退出（结构化
// summary + items 仍打印在 stdout 上）。
// ──────────────────────────────────────────────────────────

// push 动作分类。
const (
	pushActionUploaded      = "uploaded"       // 新建上传
	pushActionOverwritten   = "overwritten"    // 覆盖已存在的远端文件
	pushActionSkipped       = "skipped"        // 按 --if-exists 跳过
	pushActionFolderCreated = "folder_created" // 新建远端目录（不计入 uploaded）
	pushActionFailed        = "failed"
)

// localPushFile 描述一个待推送的本地常规文件。
type localPushFile struct {
	RelPath       string
	AbsPath       string
	ModTimeMillis int64
	Size          int64
	scanInfo      os.FileInfo
}

// pushPutOpenedFile 把已由固定 os.Root 打开的本地文件句柄上传到 OSS。生产 push/sync
// 不再把可被并发换链的绝对路径交给 HTTP 层重新打开；测试必须通过 testseam.Swap 替换。
var (
	pushPutOpenedFile  = defaultPushPutOpenedFile
	pushStatOpenedFile = func(file *os.File) (os.FileInfo, error) {
		return file.Stat()
	}
	pushVerifyPinnedSourceRoot = func(root *pinnedPullRoot) error {
		return root.verify()
	}
	pushMD5OpenedFile = func(file *os.File) (string, error) {
		hash := md5.New()
		if _, err := io.Copy(hash, file); err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
	}
)

// drivePushItem 是输出 items[] 中每个条目的明细。
type drivePushItem struct {
	RelPath   string `json:"rel_path"`
	Action    string `json:"action"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	Error     string `json:"error,omitempty"`
}

// drivePushSummary 是各动作的计数汇总。uploaded 同时统计新建与覆盖。
type drivePushSummary struct {
	Uploaded int  `json:"uploaded"`
	Skipped  int  `json:"skipped"`
	Failed   int  `json:"failed"`
	Aborted  bool `json:"aborted"`
}

type drivePushResult struct {
	Summary drivePushSummary `json:"summary"`
	Items   []drivePushItem  `json:"items"`
}

type drivePushDryRunResult struct {
	DryRun      bool            `json:"dry_run"`
	Executed    bool            `json:"executed"`
	PreviewKind string          `json:"preview_kind"`
	Operation   string          `json:"operation"`
	IfExists    string          `json:"if_exists"`
	Plan        drivePushResult `json:"plan"`
}

// drivePushFailure 在 summary.failed > 0 时返回：结构化结果已打印到 stdout，
// 这里只负责以 exit=1 退出并向 stderr 输出一行简短说明。
type drivePushFailure struct{ failed int }

func (e *drivePushFailure) Error() string {
	return fmt.Sprintf("drive push: %d file(s) failed", e.failed)
}
func (e *drivePushFailure) RawStderr() string { return e.Error() }
func (e *drivePushFailure) ExitCode() int     { return 1 }

func runDrivePush(cmd *cobra.Command, _ []string) error {
	if err := validateRequiredFlags(cmd, "local-folder", "remote-folder"); err != nil {
		return err
	}
	localDir := mustGetFlag(cmd, "local-folder")
	remoteDirID := mustGetFlag(cmd, "remote-folder")
	spaceID := mustGetFlag(cmd, "space-id")

	// push 默认 skip（安全，只新增不覆盖）。
	ifExists, _ := cmd.Flags().GetString("if-exists")
	if ifExists == "" {
		ifExists = ifExistsSkip
	}
	switch ifExists {
	case ifExistsSkip, ifExistsSmart, ifExistsOverwrite:
	default:
		return fmt.Errorf("--if-exists 取值非法: %s（可选 skip|smart|overwrite）", ifExists)
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

	localDirs, localFiles, err := runDriveMirrorWalkLocalForPushPinned(pinnedRoot)
	if err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}

	ctx := cmd.Context()
	// 远端现状：已存在的文件（用于 --if-exists 判断）与目录（rel_path → fileId，用于复用/定位父目录）。
	remoteFiles, remoteFolders, err := fetchRemoteTreeForPush(ctx, spaceID, remoteDirID)
	if err != nil {
		return err
	}

	preflight := buildMirrorPreflight(localDirs, localFiles, remoteFiles, remoteFolders)

	res := drivePushResult{Items: make([]drivePushItem, 0, len(localDirs)+len(localFiles))}
	if deps.Caller.DryRun() {
		return printDrivePushDryRunWithPreflight(ifExists, remoteFiles, remoteFolders, localDirs, localFiles, preflight)
	}
	if len(preflight) > 0 {
		appendDrivePushPreflightFailures(&res, preflight)
		if perr := deps.Out.PrintJSON(res); perr != nil {
			return perr
		}
		return &drivePushFailure{failed: res.Summary.Failed}
	}
	// 本地扫描与远端读取都可能耗时。首个远端写之前确认命令行路径仍指向最初固定
	// 的根；若根已被移走或替换，整批中止，不能把替代目录的数据当成扫描快照上传。
	if err := pinnedRoot.verify(); err != nil {
		return err
	}

	// 第一阶段：按需创建远端目录（浅层在前，保证父目录先于子目录存在）。
	for _, dir := range localDirs {
		if _, ok := remoteFolders[dir]; ok {
			continue // 远端已存在，复用 fileId，不留痕
		}
		parentRel, name := splitRel(dir)
		parentID, ok := remoteFolders[parentRel]
		if !ok || parentID == "" {
			res.Summary.Failed++
			res.Items = append(res.Items, drivePushItem{RelPath: dir, Action: pushActionFailed, Error: "父目录未能创建"})
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
			res.Items = append(res.Items, drivePushItem{RelPath: dir, Action: pushActionFailed, Error: msg})
			continue
		}
		remoteFolders[dir] = fid
		res.Items = append(res.Items, drivePushItem{RelPath: dir, Action: pushActionFolderCreated})
	}

	// 第二阶段：上传/覆盖/跳过文件。
	for i := range localFiles {
		lf := localFiles[i]
		size := lf.Size
		parentRel, name := splitRel(lf.RelPath)
		parentID, ok := remoteFolders[parentRel]
		if !ok || parentID == "" {
			res.Summary.Failed++
			res.Items = append(res.Items, drivePushItem{RelPath: lf.RelPath, Action: pushActionFailed, SizeBytes: &size, Error: "父目录未能创建"})
			continue
		}

		rf, exists := remoteFiles[lf.RelPath]
		action := pushActionUploaded
		overwriteID := ""
		if exists {
			switch ifExists {
			case ifExistsSkip:
				res.Summary.Skipped++
				res.Items = append(res.Items, drivePushItem{RelPath: lf.RelPath, Action: pushActionSkipped, SizeBytes: &size})
				continue
			case ifExistsSmart:
				// 远端时间可信且已 ≥ 本地 → 跳过；否则走覆盖路径。
				if rf.ModifiedTimeValid && rf.ModifiedTime >= lf.ModTimeMillis {
					res.Summary.Skipped++
					res.Items = append(res.Items, drivePushItem{RelPath: lf.RelPath, Action: pushActionSkipped, SizeBytes: &size})
					continue
				}
				action = pushActionOverwritten
			case ifExistsOverwrite:
				action = pushActionOverwritten
			}
			// 覆盖分支必须走覆盖上传（传 overwriteFileId、不传 parentId），
			// 否则会在同目录新建重名副本而非原地覆盖。
			if action == pushActionOverwritten {
				overwriteID = rf.FileID
			}
		}

		if err := pushUploadFilePinned(ctx, spaceID, parentID, overwriteID, name, pinnedRoot, lf); err != nil {
			res.Summary.Failed++
			res.Items = append(res.Items, drivePushItem{RelPath: lf.RelPath, Action: pushActionFailed, SizeBytes: &size, Error: err.Error()})
			continue
		}
		res.Summary.Uploaded++ // uploaded 同时统计新建与覆盖
		res.Items = append(res.Items, drivePushItem{RelPath: lf.RelPath, Action: action, SizeBytes: &size})
	}

	// 结构化结果始终打印到 stdout；有失败则额外以非零退出码退出。
	if perr := deps.Out.PrintJSON(res); perr != nil {
		return perr
	}
	if res.Summary.Failed > 0 {
		return &drivePushFailure{failed: res.Summary.Failed}
	}
	return nil
}

// fetchRemoteTreeForPush 递归拉取 rootID 下的远端现状：files（rel_path → *remoteFile，
// 用于 --if-exists 判断）与 folders（rel_path → fileId，rel_path "" 即 rootID 本身）。
func fetchRemoteTreeForPush(ctx context.Context, spaceID, rootID string) (map[string]*remoteFile, map[string]string, error) {
	files := make(map[string]*remoteFile)
	folders := map[string]string{"": rootID}
	occupied := make(map[string]string)
	if err := walkRemoteForPush(ctx, spaceID, rootID, "", files, folders, occupied, 0); err != nil {
		return nil, nil, err
	}
	return files, folders, nil
}

func walkRemoteForPush(ctx context.Context, spaceID, parentID, relBase string, files map[string]*remoteFile, folders map[string]string, occupied map[string]string, depth int) error {
	if depth > remoteMaxDepth {
		return fmt.Errorf("drive 目录层级超过 %d 层，疑似循环引用，已中止", remoteMaxDepth)
	}
	nextToken := ""
	seenTokens := make(map[string]struct{})
	for {
		args := map[string]any{"maxResults": float64(driveListPageSize)}
		if spaceID != "" {
			args["spaceId"] = spaceID
		}
		if parentID != "" {
			args["parentId"] = parentID
		}
		if nextToken != "" {
			args["nextToken"] = nextToken
		}

		text, err := callDriveListFiles(ctx, args)
		if err != nil {
			return err
		}
		items, token, err := parseDriveList(text)
		if err != nil {
			return err
		}

		for _, it := range items {
			name, included, err := remoteMirrorEntryName(it, relBase)
			if err != nil {
				return err
			}
			if !included {
				continue
			}
			childRel := name
			if relBase != "" {
				childRel = relBase + "/" + name
			}
			if it.isFolder() {
				if err := claimRemotePath(occupied, childRel, "目录"); err != nil {
					return err
				}
				folderID := it.id()
				if folderID == "" {
					return fmt.Errorf("远端目录 %q 缺少目录 ID，已中止递归", childRel)
				}
				folders[childRel] = folderID
				if err := walkRemoteForPush(ctx, spaceID, folderID, childRel, files, folders, occupied, depth+1); err != nil {
					return err
				}
				continue
			}
			// 非文件类型已由 remoteMirrorEntryName 过滤；目录在上方完成递归后返回。
			if err := claimRemotePath(occupied, childRel, "文件"); err != nil {
				return err
			}
			fileID := it.id()
			if fileID == "" {
				return fmt.Errorf("远端文件 %q 缺少文件 ID，已中止遍历", childRel)
			}
			modMillis, modValid := it.modifiedMillis()
			files[childRel] = &remoteFile{
				RelPath:           childRel,
				FileID:            fileID,
				Hash:              it.hash(),
				ModifiedTime:      modMillis,
				ModifiedTimeValid: modValid,
			}
		}

		if token == "" {
			break
		}
		if _, exists := seenTokens[token]; exists {
			return fmt.Errorf("远端目录 %q 的分页 token 重复，疑似分页循环，已中止", relBase)
		}
		seenTokens[token] = struct{}{}
		nextToken = token
	}
	return nil
}

func printDrivePushDryRun(ifExists string, remoteFiles map[string]*remoteFile, remoteFolders map[string]string, localDirs []string, localFiles []localPushFile) error {
	return printDrivePushDryRunWithPreflight(ifExists, remoteFiles, remoteFolders, localDirs, localFiles,
		buildMirrorPreflight(localDirs, localFiles, remoteFiles, remoteFolders))
}

func printDrivePushDryRunWithPreflight(ifExists string, remoteFiles map[string]*remoteFile, remoteFolders map[string]string, localDirs []string, localFiles []localPushFile, preflight map[string]string) error {
	plan := drivePushResult{Items: make([]drivePushItem, 0, len(localDirs)+len(localFiles))}
	if len(preflight) > 0 {
		appendDrivePushPreflightFailures(&plan, preflight)
		return deps.Out.PrintJSON(drivePushDryRunResult{
			DryRun: true, Executed: false, PreviewKind: "plan", Operation: "drive push",
			IfExists: ifExists, Plan: plan,
		})
	}
	plannedFolders := make(map[string]string, len(remoteFolders)+len(localDirs))
	for rel, fileID := range remoteFolders {
		plannedFolders[rel] = fileID
	}
	for _, dir := range localDirs {
		if _, ok := plannedFolders[dir]; ok {
			continue
		}
		parentRel, _ := splitRel(dir)
		if plannedFolders[parentRel] == "" {
			plan.Summary.Failed++
			plan.Items = append(plan.Items, drivePushItem{RelPath: dir, Action: pushActionFailed, Error: "父目录未能创建"})
			continue
		}
		plannedFolders[dir] = "dry-run-planned-folder"
		plan.Items = append(plan.Items, drivePushItem{RelPath: dir, Action: pushActionFolderCreated})
	}
	for _, lf := range localFiles {
		size := lf.Size
		parentRel, _ := splitRel(lf.RelPath)
		if plannedFolders[parentRel] == "" {
			plan.Summary.Failed++
			plan.Items = append(plan.Items, drivePushItem{RelPath: lf.RelPath, Action: pushActionFailed, SizeBytes: &size, Error: "父目录未能创建"})
			continue
		}
		action := pushActionUploaded
		if rf, exists := remoteFiles[lf.RelPath]; exists {
			switch ifExists {
			case ifExistsSkip:
				action = pushActionSkipped
			case ifExistsSmart:
				if rf.ModifiedTimeValid && rf.ModifiedTime >= lf.ModTimeMillis {
					action = pushActionSkipped
				} else {
					action = pushActionOverwritten
				}
			case ifExistsOverwrite:
				action = pushActionOverwritten
			}
		}
		if action == pushActionSkipped {
			plan.Summary.Skipped++
		} else {
			plan.Summary.Uploaded++
		}
		plan.Items = append(plan.Items, drivePushItem{RelPath: lf.RelPath, Action: action, SizeBytes: &size})
	}
	return deps.Out.PrintJSON(drivePushDryRunResult{
		DryRun: true, Executed: false, PreviewKind: "plan", Operation: "drive push",
		IfExists: ifExists, Plan: plan,
	})
}

func appendDrivePushPreflightFailures(res *drivePushResult, preflight map[string]string) {
	res.Summary.Aborted = true
	for _, rel := range mirrorPreflightFailureRels(preflight) {
		res.Summary.Failed++
		res.Items = append(res.Items, drivePushItem{RelPath: rel, Action: pushActionFailed, Error: preflight[rel]})
	}
}

// pushTypeConflictError 在任何写操作或 dry-run 成功预览之前，拒绝同一路径下
// “本地目录 ↔ 远端文件”或“本地文件 ↔ 远端目录”的类型冲突。
func pushTypeConflictError(rel string, localIsFolder bool, remoteFiles map[string]*remoteFile, remoteFolders map[string]string) string {
	if localIsFolder {
		if _, exists := remoteFiles[rel]; exists {
			return "远端同路径已存在文件，无法创建目录"
		}
		return ""
	}
	if _, exists := remoteFolders[rel]; exists {
		return "远端同路径已存在目录，无法上传文件"
	}
	return ""
}

// walkLocalForPush 遍历本地根目录，返回所有子目录 rel_path（不含根本身，浅层在前）
// 与所有常规文件。dirs 升序排序保证父目录先于子目录被创建。
func walkLocalForPush(root string) ([]string, []localPushFile, error) {
	pinned, err := openExistingPinnedPullRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer pinned.close()
	return walkLocalForPushPinned(pinned)
}

// 文件系统遍历与命令入口通过显式 seam 暴露仅用于确定性覆盖 I/O 失败；测试必须用
// testseam.Swap 替换，生产仍直接使用 fs.WalkDir / walkLocalForPushPinned。
var (
	walkPinnedLocalFS                    = fs.WalkDir
	runDriveMirrorWalkLocalForPushPinned = walkLocalForPushPinned
)

// walkLocalForPushPinned 从命令开始时固定的同一个 os.Root/FS 扫描本地树。即使调用方
// 给出的路径随后被替换成软链或另一目录，遍历也只会读取原目录树。
func walkLocalForPushPinned(root *pinnedPullRoot) ([]string, []localPushFile, error) {
	if err := root.verify(); err != nil {
		return nil, nil, err
	}
	var dirs []string
	var files []localPushFile
	err := walkPinnedLocalFS(root.root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		relSlash := filepath.ToSlash(path)
		if d.IsDir() {
			dirs = append(dirs, relSlash)
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, localPushFile{
			RelPath: relSlash, AbsPath: filepath.Join(root.absDir, filepath.FromSlash(relSlash)),
			ModTimeMillis: info.ModTime().UnixMilli(), Size: info.Size(), scanInfo: info,
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if err := root.verify(); err != nil {
		return nil, nil, err
	}
	// 字典序即浅层在前（"a" < "a/b" < "a/b/c"），父目录先于子目录。
	sort.Strings(dirs)
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return dirs, files, nil
}

// judgePinnedPushFileMatch 与 judgeFileMatch 的判定保持一致，但 exact 模式只从固定
// os.Root 打开的句柄计算 MD5，绝不按可被换链的 AbsPath 重开文件。
func judgePinnedPushFileMatch(root *pinnedPullRoot, local localPushFile, remote *remoteFile, quick bool) (string, error) {
	if quick {
		if remote.ModifiedTimeValid && local.ModTimeMillis == remote.ModifiedTime {
			return matchUnchanged, nil
		}
		return matchModified, nil
	}
	if remote.Hash == "" {
		return matchUnknown, nil
	}
	file, err := root.root.Open(filepath.FromSlash(local.RelPath))
	if err != nil {
		return "", fmt.Errorf("计算 %s 的 MD5 失败: %w", local.RelPath, err)
	}
	defer file.Close()
	info, err := pushStatOpenedFile(file)
	if err != nil || !info.Mode().IsRegular() || (local.scanInfo != nil && !os.SameFile(local.scanInfo, info)) {
		return "", fmt.Errorf("计算 %s 的 MD5 失败: 本地文件在扫描后被替换", local.RelPath)
	}
	hash, err := pushMD5OpenedFile(file)
	if err != nil {
		return "", fmt.Errorf("计算 %s 的 MD5 失败: %w", local.RelPath, err)
	}
	if err := pushVerifyPinnedSourceRoot(root); err != nil {
		return "", err
	}
	if hash == remote.Hash {
		return matchUnchanged, nil
	}
	return matchModified, nil
}

// mirrorPathKey 表示 Drive 镜像契约中的等价路径：逐段 Unicode NFC + 大小写折叠。
// 该规则独立于运行命令的本地文件系统，避免 macOS/Windows 与 Drive 间把 a/A 或
// NFC/NFD 当成“双向新增”，也避免把等价目录前缀错误拼接成另一棵树。
func mirrorPathKey(rel string) string {
	parts := strings.Split(rel, "/")
	for i := range parts {
		parts[i] = norm.NFC.String(strings.ToLower(parts[i]))
	}
	return strings.Join(parts, "/")
}

func mirrorLocalRelError(rel string) string {
	for _, segment := range strings.Split(rel, "/") {
		if !isSafeRemoteSegment(segment) || strings.IndexByte(segment, 0) >= 0 {
			return fmt.Sprintf("本地路径 %q 含无法安全映射到远端的名称段 %q，已中止镜像", rel, segment)
		}
	}
	return ""
}

// buildMirrorPreflight 在任何远端写之前统一检查本地名称和双端等价路径/类型。
// 每个条目同时声明全部祖先目录，因而 local A/x 与 remote a/y 会在祖先键上冲突；
// 即便本地为空，remote A/a 也会被完整性门禁识别。调用方必须整体拒绝写入，不能只
// 跳过冲突项。
func buildMirrorPreflight(localDirs []string, localFiles []localPushFile, remoteFiles map[string]*remoteFile, remoteFolders map[string]string) map[string]string {
	type mirrorKind string
	const (
		mirrorFile mirrorKind = "文件"
		mirrorDir  mirrorKind = "目录"
	)
	type entry struct {
		rel  string
		kind mirrorKind
	}
	local := make(map[string]map[entry]struct{})
	remote := make(map[string]map[entry]struct{})
	failures := make(map[string]string)
	addClaims := func(index map[string]map[entry]struct{}, rel string, kind mirrorKind) {
		parts := strings.Split(rel, "/")
		for i := range parts {
			claimKind := mirrorDir
			if i == len(parts)-1 {
				claimKind = kind
			}
			claimRel := strings.Join(parts[:i+1], "/")
			key := mirrorPathKey(claimRel)
			if index[key] == nil {
				index[key] = make(map[entry]struct{})
			}
			index[key][entry{rel: claimRel, kind: claimKind}] = struct{}{}
		}
	}
	addLocal := func(rel string, kind mirrorKind) {
		if msg := mirrorLocalRelError(rel); msg != "" {
			failures[rel] = msg
		}
		addClaims(local, rel, kind)
	}
	for _, rel := range localDirs {
		addLocal(rel, mirrorDir)
	}
	for _, f := range localFiles {
		addLocal(f.RelPath, mirrorFile)
	}
	for rel := range remoteFolders {
		if rel != "" {
			addClaims(remote, rel, mirrorDir)
		}
	}
	for rel := range remoteFiles {
		addClaims(remote, rel, mirrorFile)
	}
	keys := make(map[string]struct{}, len(local)+len(remote))
	for key := range local {
		keys[key] = struct{}{}
	}
	for key := range remote {
		keys[key] = struct{}{}
	}
	for key := range keys {
		claims := make(map[entry]struct{}, len(local[key])+len(remote[key]))
		for claim := range local[key] {
			claims[claim] = struct{}{}
		}
		for claim := range remote[key] {
			claims[claim] = struct{}{}
		}
		if len(claims) <= 1 {
			continue
		}
		for claim := range claims {
			failures[claim.rel] = fmt.Sprintf("%s路径 %q 在镜像规则下存在大小写/Unicode NFC 等价路径拼写或类型歧义，已中止镜像", claim.kind, claim.rel)
		}
	}
	return failures
}

func mirrorPreflightFailureRels(preflight map[string]string) []string {
	rels := make([]string, 0, len(preflight))
	for rel := range preflight {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	return rels
}

// walkLocalForPushEntry 是 walkLocalForPush 的单条目处理逻辑。抽成命名函数只为可测：
// filepath.Rel / Info() 的失败在真实 WalkDir 下几乎不可复现，单测可直接注入。
func walkLocalForPushEntry(root, path string, d fs.DirEntry, err error, dirs *[]string, files *[]localPushFile) error {
	if err != nil {
		return err
	}
	rel, rerr := filepath.Rel(root, path)
	if rerr != nil {
		return rerr
	}
	if rel == "." {
		return nil // 根目录本身不处理
	}
	relSlash := filepath.ToSlash(rel)
	if d.IsDir() {
		*dirs = append(*dirs, relSlash)
		return nil
	}
	info, ierr := d.Info()
	if ierr != nil {
		return ierr
	}
	// 只推送常规文件；符号链接、设备文件等忽略。
	if !info.Mode().IsRegular() {
		return nil
	}
	*files = append(*files, localPushFile{
		RelPath:       relSlash,
		AbsPath:       path,
		ModTimeMillis: info.ModTime().UnixMilli(),
		Size:          info.Size(),
		scanInfo:      info,
	})
	return nil
}

// pushCreateFolder 在 parentID 下创建名为 name 的文件夹，返回新目录的 fileId。
func pushCreateFolder(ctx context.Context, spaceID, parentID, name string) (string, error) {
	args := map[string]any{"name": name}
	if spaceID != "" {
		args["spaceId"] = spaceID
	}
	if parentID != "" {
		args["parentId"] = parentID
	}
	text, err := callMCPToolReturnText(ctx, "create_folder", args)
	if err != nil {
		return "", err
	}
	return parseNodeID(text), nil
}

// pushUploadFile 走 get_upload_info → OSS PUT → commit_upload 三步，把本地文件
// 上传到 parentID 目录下。复用 drive upload 的凭证解析与 HTTP PUT。
//
// overwriteFileID 非空时走覆盖流程（与 uploadToDrive 一致）：get_upload_info 与
// commit_upload 两个阶段都传 overwriteFileId、都不传 parentId（服务端据此设置
// conflictStrategy=OVERWRITE，原地覆盖而非在同目录新建重名副本）。
func pushUploadFile(ctx context.Context, spaceID, parentID, overwriteFileID, fileName, filePath string, fileSize int64) error {
	return pushUploadWithTransport(ctx, spaceID, parentID, overwriteFileID, fileName, fileSize,
		func(resourceURL string, ossHeaders map[string]string) error {
			return httpPutFile(ctx, resourceURL, ossHeaders, filePath, fileSize)
		})
}

// pushUploadFilePinned 从固定本地根打开 rel_path，并在发出 get_upload_info 前核对
// 文件身份与扫描快照。HTTP PUT 只读取这个已打开句柄，绝不按 AbsPath 二次解析。
func pushUploadFilePinned(ctx context.Context, spaceID, parentID, overwriteFileID, fileName string, root *pinnedPullRoot, local localPushFile) error {
	if err := root.verify(); err != nil {
		return err
	}
	file, err := root.root.Open(filepath.FromSlash(local.RelPath))
	if err != nil {
		return fmt.Errorf("打开本地上传文件失败: %w", err)
	}
	defer file.Close()
	info, err := pushStatOpenedFile(file)
	if err != nil {
		return fmt.Errorf("读取本地上传文件身份失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("本地上传目标 %q 已不再是常规文件", local.RelPath)
	}
	if local.scanInfo != nil && !os.SameFile(local.scanInfo, info) {
		return fmt.Errorf("本地上传目标 %q 在扫描后被替换，已中止上传", local.RelPath)
	}
	if info.Size() != local.Size || info.ModTime().UnixMilli() != local.ModTimeMillis {
		return fmt.Errorf("本地上传目标 %q 在扫描后发生变化，已中止上传", local.RelPath)
	}
	if err := pushVerifyPinnedSourceRoot(root); err != nil {
		return err
	}
	return pushUploadWithTransport(ctx, spaceID, parentID, overwriteFileID, fileName, local.Size,
		func(resourceURL string, ossHeaders map[string]string) error {
			if err := pushPutOpenedFile(ctx, resourceURL, ossHeaders, file, local.Size); err != nil {
				return err
			}
			// OSS 传输期间若源文件被原地改写（编辑器覆盖、截断重写、mmap），
			// PUT 里发送的可能是新旧内容的混合体，root.verify() 又不检查文件本身。
			// commit 前对已打开句柄再取一次身份，与 PUT 前的 inode/size/mtime 逐项比对;
			// 任一变化都拒绝 commit，避免 overwrite/local-wins 场景把混合内容覆盖到远端。
			after, err := pushStatOpenedFile(file)
			if err != nil {
				return fmt.Errorf("上传后读取本地上传文件身份失败: %w", err)
			}
			if !os.SameFile(info, after) ||
				after.Size() != info.Size() ||
				after.ModTime().UnixMilli() != info.ModTime().UnixMilli() {
				return fmt.Errorf("本地上传目标 %q 在传输期间被修改，已中止提交", local.RelPath)
			}
			// OSS 传输期间若命令行根已被替换，不提交这个上传会话；已发送的字节仍来自
			// 固定根内的原文件句柄，不可能读取替代路径或根外内容。
			return root.verify()
		})
}

func pushUploadWithTransport(ctx context.Context, spaceID, parentID, overwriteFileID, fileName string, fileSize int64, put func(string, map[string]string) error) error {
	step1 := map[string]any{"fileName": fileName, "fileSize": float64(fileSize)}
	if spaceID != "" {
		step1["spaceId"] = spaceID
	}
	if overwriteFileID != "" {
		step1["overwriteFileId"] = overwriteFileID
	} else if parentID != "" {
		step1["parentId"] = parentID
	}
	text, err := callMCPToolReturnText(ctx, "get_upload_info", step1)
	if err != nil {
		return err
	}
	resourceURL, uploadID, ossHeaders, err := parseDriveUploadInfo(text)
	if err != nil {
		return err
	}
	if err := put(resourceURL, ossHeaders); err != nil {
		return err
	}
	commit := map[string]any{"fileName": fileName, "fileSize": float64(fileSize), "uploadId": uploadID}
	if spaceID != "" {
		commit["spaceId"] = spaceID
	}
	if overwriteFileID != "" {
		commit["overwriteFileId"] = overwriteFileID
	} else if parentID != "" {
		commit["parentId"] = parentID
	}
	commitText, err := callMCPToolReturnText(ctx, "commit_upload", commit)
	if err != nil {
		return err
	}
	if strings.TrimSpace(commitText) == "" {
		return fmt.Errorf("commit_upload returned no business result; remote effect is unknown")
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(commitText), &result); err != nil {
		return fmt.Errorf("parse commit_upload response: %w", err)
	}
	if len(result) == 0 {
		return fmt.Errorf("commit_upload returned an empty JSON object; remote effect is unknown")
	}
	return nil
}

func defaultPushPutOpenedFile(ctx context.Context, url string, headers map[string]string, file *os.File, fileSize int64) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek upload file: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.ContentLength = fileSize
	req.Header.Del("Content-Type")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("file upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OSS upload failed: %w", &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)})
	}
	return nil
}

// parseNodeID 从 create_folder / commit 等返回里抽出节点 fileId（带 fallback）。
func parseNodeID(text string) string {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return ""
	}
	if r, ok := data["result"].(map[string]any); ok {
		data = r
	}
	for _, k := range []string{"fileId", "dentryUuid", "dentryId", "id", "nodeId"} {
		if s, ok := data[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// splitRel 把 rel_path 拆成父路径与末段名："a/b/c" → ("a/b","c")，"c" → ("","c")。
func splitRel(rel string) (string, string) {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i], rel[i+1:]
	}
	return "", rel
}

// checkFileNotDingTalkDoc 通过 get_file_info 检查文件类型，
// 若为钉钉在线文档 (adoc/axls/amind/adraw) 则返回错误，提示使用对应服务命令。
// 探测失败（如无效 fileId）不阻断流程，让后续 MCP 工具自行报错。

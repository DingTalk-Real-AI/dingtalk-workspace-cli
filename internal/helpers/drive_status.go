package helpers

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// ==========================================================
// drive status — 比较本地文件夹与钉盘文件夹的差异
// ==========================================================
// ──────────────────────────────────────────────────────────
// dws drive status — 比较本地文件夹与钉盘文件夹的差异
//
// 只读命令：本地取 --local-folder（绝对路径），钉盘取 --remote-folder
// （文件夹 dentryUuid）指向的文件夹，按精确 MD5（默认）或快速 modified_time
// （--quick）逐文件比对，输出 new_local / new_remote / modified / unchanged /
// unknown 五类差异（exact 模式远端无可靠 md5 的文件归入 unknown）。两侧各自递归
// 遍历，rel_path 相对各自根目录。
//
// 后端交互（按 parentId 递归拉取 list_files、只保留 type=file 的二进制文件、并为
// 每个条目填充 hash 或者 modified_time），见 fetchRemoteDriveTree。
// ──────────────────────────────────────────────────────────

// driveStatusEntry 是输出 schema 中每个差异项的形态：{"rel_path": "..."}。
type driveStatusEntry struct {
	RelPath string `json:"rel_path"`
}

// driveStatusResult 是 status 命令成功时的输出 schema。
// 五个切片都初始化为非 nil，保证 JSON 序列化为 [] 而不是 null。
//
// unknown：exact 模式下双端都存在、但远端未返回可靠 md5 的文件——此时无法核对
// 内容，既不能判 unchanged（会误报未变更），也不宜硬判 modified（会误报变更），
// 如实归入 unknown，让降级模式在输出里显式可见。quick 模式不会产生 unknown。
type driveStatusResult struct {
	Detection string             `json:"detection"`
	NewLocal  []driveStatusEntry `json:"new_local"`
	NewRemote []driveStatusEntry `json:"new_remote"`
	Modified  []driveStatusEntry `json:"modified"`
	Unchanged []driveStatusEntry `json:"unchanged"`
	Unknown   []driveStatusEntry `json:"unknown"`
}

// localFile 描述一个本地常规文件。
type localFile struct {
	RelPath       string
	AbsPath       string // 绝对路径，用于按需计算 MD5
	Hash          string // MD5（base64）；仅在双端都存在且非 quick 模式时按需计算
	Size          int64  // 本地文件字节数
	ModTimeMillis int64  // 本地 mtime，Unix 毫秒
}

// remoteFile 描述一个云端 type=file 的二进制文件。
// 由 fetchRemoteDriveTree 填充。
type remoteFile struct {
	RelPath           string
	FileID            string // dentryUuid；pull 下载时作为 download_file 的 fileId
	Hash              string // 远端 md5（base64，exact 模式用；为空时该文件归入 unknown）
	Size              int64  // 远端文件字节数
	ModifiedTime      int64  // 远端 modified_time，Unix 毫秒（quick / smart 模式比较用）
	ModifiedTimeValid bool   // 远端时间戳是否可信；不可信时走保守路径
}

// 每次 list_files 请求的分页大小。后端约束 maxResults 必须在 1..50 之间。
const driveListPageSize = 50

// remoteMaxDepth 是递归遍历子文件夹的最大层级，防止云端目录出现环形引用时死循环。
const remoteMaxDepth = 64

// fetchRemoteDriveTree 递归拉取云端文件夹 parentID 下的文件树，返回所有 type=FILE 的
// 二进制文件索引（key 为 rel_path）。spaceID 为空时省略 spaceId 参数，由 MCP Server
// 默认到「我的文件」（与其它 drive 命令一致）；parentID 为待比对的云端根文件夹 dentryUuid。
//
// 实现要点：
//   - 通过 drive 的 list_files MCP 工具，按 parentId（文件夹 dentryUuid）逐层遍历；
//   - 单个目录文件过多时按 nextToken 分批拉取（maxResults ≤ 50）；
//   - 遇到子文件夹用其 dentryUuid 作为下一层的 parentId 递归进入；
//   - rel_path 由各层文件夹名逐层拼接得到（相对 parentID 根），与本地相对路径对齐；
//   - 只纳入明确 type=FILE 的条目，其余类型（在线文档、快捷方式等）一律跳过。
func fetchRemoteDriveTree(ctx context.Context, spaceID, parentID string, quick bool) (map[string]*remoteFile, error) {
	out := make(map[string]*remoteFile)
	occupied := make(map[string]string)
	if err := walkRemoteDir(ctx, spaceID, parentID, "", out, occupied, 0); err != nil {
		return nil, err
	}
	_ = quick // hash 与 modified_time 都会被填充，比对模式由 compareTrees 决定
	return out, nil
}

// walkRemoteDir 遍历 parentID 指向的文件夹，把其中的二进制文件写入 out（key 为 rel_path），
// 并递归进入子文件夹。occupied 同时记录文件和目录；精确路径被第二个条目占用时立即失败，
// 防止分页或返回顺序决定最终镜像。relBase 是当前文件夹相对遍历起点的路径前缀。
func walkRemoteDir(ctx context.Context, spaceID, parentID, relBase string, out map[string]*remoteFile, occupied map[string]string, depth int) error {
	if depth > remoteMaxDepth {
		return fmt.Errorf("drive 目录层级超过 %d 层，疑似循环引用，已中止", remoteMaxDepth)
	}

	nextToken := ""
	seenTokens := make(map[string]struct{})
	for {
		args := map[string]any{"maxResults": float64(driveListPageSize)}
		// space-id 为空则省略，交给 MCP Server 默认「我的文件」。
		if spaceID != "" {
			args["spaceId"] = spaceID
		}
		// 按 parentId（文件夹 dentryUuid）导航；空则由 MCP Server 默认列空间根目录。
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
			// rel_path 由文件夹名逐层拼接，统一用 / 分隔，与本地相对路径对齐。
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
				// 用子文件夹的 dentryUuid 作为下一层 parentId 递归进入。
				if err := walkRemoteDir(ctx, spaceID, folderID, childRel, out, occupied, depth+1); err != nil {
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
			out[childRel] = &remoteFile{
				RelPath:           childRel,
				FileID:            fileID,
				Hash:              it.hash(),
				Size:              it.size(),
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

// claimRemotePath 保证一棵远端树中的每个精确 rel_path 只对应一个文件或目录。
// 重复条目会让本地镜像不可完整表达，因此必须失败，不能任意选择其中一个 fileId。
func claimRemotePath(occupied map[string]string, rel, kind string) error {
	if previous, exists := occupied[rel]; exists {
		return fmt.Errorf("检测到重复远端路径 %q（已有%s，重复%s），无法构建确定性镜像", rel, previous, kind)
	}
	occupied[rel] = kind
	return nil
}

// remoteMirrorEntryName 只选择文件夹镜像支持的 file / folder 条目，并验证其名称
// 能安全、无歧义地表示为本地单层路径。无法表示的镜像条目必须中止整棵远端树构建；
// 静默跳过会让 status/pull/push/sync 在遗漏文件或目录子树时误报成功。
func remoteMirrorEntryName(it driveItem, relBase string) (string, bool, error) {
	kind := ""
	switch {
	case it.isFolder():
		kind = "目录"
	case it.isFile():
		kind = "文件"
	default:
		return "", false, nil
	}

	name := it.name()
	if !isSafeRemoteSegment(name) {
		return "", false, fmt.Errorf("远端%s名称 %q 无法安全映射到本地路径（父路径 %q），已中止", kind, name, relBase)
	}
	return name, true, nil
}

// callDriveListFiles 让文件夹同步族在 --dry-run 下仍可通过受控只读通道获取
// 远端现状；普通 CallTool 在 dry-run 下只会返回执行计划，不能被当作业务数据解析。
func callDriveListFiles(ctx context.Context, args map[string]any) (string, error) {
	if deps.Caller.DryRun() {
		return callMCPReadToolReturnTextOnServer(ctx, "drive", "list_files", args)
	}
	return callMCPToolReturnText(ctx, "list_files", args)
}

// driveItem 是 list_files 返回里的一个 dentry 条目。
// 后端字段命名存在多种历史写法，这里用带 fallback 的访问器做防御式解析。
type driveItem map[string]any

func (d driveItem) str(keys ...string) string {
	for _, k := range keys {
		if s, ok := d[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (d driveItem) name() string { return d.str("name") }
func (d driveItem) id() string   { return d.str("fileId", "dentryUuid", "dentryId", "id", "nodeId") }
func (d driveItem) typ() string  { return d.str("type") }
func (d driveItem) hash() string { return d.str("md5") }
func (d driveItem) path() string { return d.str("path") }

// size 返回远端文件字节数（fileSize 字段），用于 exact 无 md5 时的降级比对。
func (d driveItem) size() int64 {
	switch v := d["fileSize"].(type) {
	case float64:
		return int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n
		}
	}
	return 0
}

func (d driveItem) isFolder() bool {
	return strings.ToUpper(d.typ()) == "FOLDER"
}

func (d driveItem) isFile() bool {
	return strings.ToUpper(d.typ()) == "FILE"
}

func (d driveItem) modifiedMillis() (int64, bool) {
	for _, k := range []string{"modifiedTime", "modifyTime", "modified_time", "gmtModified", "lastModifiedTime", "updateTime"} {
		if v, ok := d[k]; ok {
			if ms, ok := toMillis(v); ok {
				return ms, true
			}
		}
	}
	return 0, false
}

// parseDriveList 解析 list_files 返回文本，抽出 items 列表与 nextToken。
// 兼容两种 result 形态：
//   - {"result":{"items":[...],"nextToken":"..."}}
//   - {"result":[...]}（result 直接是条目数组，无分页）
func parseDriveList(text string) ([]driveItem, string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return nil, "", fmt.Errorf("解析 list_files 返回失败: %w", err)
	}
	result, exists := envelope["result"]
	if !exists {
		return nil, "", fmt.Errorf("解析 list_files 返回失败: 缺少 result")
	}
	result = json.RawMessage(bytes.TrimSpace(result))
	if bytes.Equal(result, []byte("null")) {
		return nil, "", fmt.Errorf("解析 list_files 返回失败: result 为 null")
	}

	toItems := func(ms []map[string]any) ([]driveItem, error) {
		items := make([]driveItem, 0, len(ms))
		for index, m := range ms {
			item := driveItem(m)
			// FILE / FOLDER 之外的显式类型（在线文档、快捷方式等）可以由镜像
			// walker 跳过；但 null、空对象或缺少有效 type 的条目不是一个可判定的
			// 业务对象。静默跳过会把不完整清单伪装成权威目录，必须失败关闭。
			if m == nil || strings.TrimSpace(item.typ()) == "" {
				return nil, fmt.Errorf("解析 list_files 返回失败: result.items[%d] 缺少有效 type", index)
			}
			items = append(items, item)
		}
		return items, nil
	}

	switch {
	case len(result) > 0 && result[0] == '{':
		// 形态一：result 是对象 {items, nextToken}。items 必须显式存在，不能把
		// 缺失/null 的非权威响应误判为空目录。
		var obj struct {
			Items     json.RawMessage `json:"items"`
			NextToken string          `json:"nextToken"`
			HasMore   *bool           `json:"hasMore"`
		}
		if err := json.Unmarshal(result, &obj); err != nil {
			return nil, "", fmt.Errorf("解析 list_files 返回失败: %w", err)
		}
		if len(obj.Items) == 0 {
			return nil, "", fmt.Errorf("解析 list_files 返回失败: result 缺少 items")
		}
		var items []map[string]any
		if err := json.Unmarshal(obj.Items, &items); err != nil || items == nil {
			return nil, "", fmt.Errorf("解析 list_files 返回失败: result.items 不是数组")
		}
		if obj.HasMore != nil && *obj.HasMore && obj.NextToken == "" {
			return nil, "", fmt.Errorf("解析 list_files 返回失败: hasMore=true 但 nextToken 为空，远端清单可能被截断")
		}
		parsed, err := toItems(items)
		if err != nil {
			return nil, "", err
		}
		return parsed, obj.NextToken, nil
	case len(result) > 0 && result[0] == '[':
		// 形态二：result 直接是条目数组。
		var arr []map[string]any
		if err := json.Unmarshal(result, &arr); err != nil {
			return nil, "", fmt.Errorf("解析 list_files 返回失败: result 数组无效: %w", err)
		}
		parsed, err := toItems(arr)
		if err != nil {
			return nil, "", err
		}
		return parsed, "", nil
	}

	return nil, "", fmt.Errorf("解析 list_files 返回失败: result 既不是 {items:[]} 也不是数组")
}

// validateLocalDirAbs 校验 --local-folder 必须是绝对路径，返回其清理后的绝对路径。
func validateLocalDirAbs(localDir string) (string, error) {
	if localDir == "" {
		return "", fmt.Errorf("flag --local-folder is required")
	}
	if !filepath.IsAbs(localDir) {
		return "", fmt.Errorf("--local-folder 必须是绝对路径: %s", localDir)
	}
	return filepath.Clean(localDir), nil
}

// isSafeRemoteSegment 校验单个远端条目名称是否可安全用作一层相对路径段。
// 拒绝空串、"."、".."、ASCII 控制字符、含卷标（如 Windows 的 "C:"）以及任意平台
// 的路径分隔符（/ 与 \）——这些成分无法安全表示为本地文件名，或可能让下载目标逃逸
// 出本地根目录。
func isSafeRemoteSegment(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] <= 0x1f || name[i] == 0x7f {
			return false
		}
	}
	// 任意平台分隔符都不允许出现在单层名称里。
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	// 余下的卷标/规范化检查只在 Windows 上可达：Unix 的 filepath.VolumeName 恒为空，
	// 且经上面的分隔符过滤后 filepath.Clean 不会再改写名称。按平台分文件承载。
	return isSafeRemoteSegmentPlatform(name)
}

// resolveLocalTarget 把远端 rel_path 拼到本地根目录 absDir 下，并确认结果仍位于
// absDir 内。除词法检查（filepath.Rel）外，还解析已存在路径组件的符号链接，防止
// root 内的目录软链（如 root/sub -> /outside）把落盘点指到根目录外。逃逸时返回错误。
func resolveLocalTarget(absDir, rel string) (string, error) {
	target := filepath.Join(absDir, filepath.FromSlash(rel))
	back, err := filepath.Rel(absDir, target)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("远端路径 %q 逃逸出本地根目录", rel)
	}
	if err := verifyNoSymlinkEscape(absDir, target); err != nil {
		return "", err
	}
	return target, nil
}

// verifyNoSymlinkEscape 挡住「absDir 之内的目录软链指向外部」的逃逸：从 target 的父
// 目录向上、在**不越过 absDir 边界**的前提下，找到最深的已存在祖先，解析其真实路径
// （EvalSymlinks 展开整条软链），确认仍位于 absDir 真实路径之内。
//
// absDir 尚不存在时直接放行：其下不可能有已存在的组件可供逃逸，后续 MkdirAll 只会
// 创建真实目录、绝不新建软链——这也是 pull 自动创建目标根目录的正常路径。target 尚不
// 存在的末段同理无软链可谈，故只需校验 [absDir, target] 之间的现有组件。
func verifyNoSymlinkEscape(absDir, target string) error {
	absDir = filepath.Clean(absDir)
	realRoot, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		// 根目录尚不存在（无软链可解析）→ 放行，交给 MkdirAll 按需创建真实目录。
		return nil
	}

	// 循环必然终止，无需不动点兜底：每轮 ancestor 严格变短，要么越界后由下面的 ".."
	// 分支退出，要么下降到 absDir 自身——absDir 必然存在（EvalSymlinks 刚成功），于是
	// 在 Lstat 成功分支退出。即便 absDir 在此期间被删除，下一轮 ancestor 升到 absDir
	// 之上，同样由 ".." 分支退出。
	ancestor := filepath.Dir(target)
	for {
		// 越过 absDir 边界（祖先跑到根目录之上）就停止：根目录之上不属于逃逸判定范围。
		rel, relErr := filepath.Rel(absDir, ancestor)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			real, evErr := filepath.EvalSymlinks(ancestor)
			if evErr != nil {
				return fmt.Errorf("解析路径 %q 失败: %w", ancestor, evErr)
			}
			back, bErr := filepath.Rel(realRoot, real)
			if bErr != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
				return fmt.Errorf("路径 %q 经符号链接逃逸出本地根目录", target)
			}
			return nil
		}
		ancestor = filepath.Dir(ancestor)
	}
}

// md5File 计算文件内容的 MD5，并按 base64 编码返回（与钉盘 list_files 返回的
// md5 字段编码一致，便于直接字符串比较）。变量形式只用于测试确定性注入读取失败；
// 测试必须通过 testseam.Swap 替换并自动恢复。
var md5File = func(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// statusRootLstat 是根类型校验的注入 seam；测试通过 testseam.Swap 注入不同的
// FileInfo 来复现 Windows 上无法创建的符号链接场景。
var statusRootLstat = os.Lstat

// statusWalkDir 是本地目录遍历的注入 seam。改造根类型校验后，filepath.WalkDir
// 的错误分支（walkLocalTreeEntry 之外的外层错误）在 macOS/Windows 上都难以自然
// 触发，需要 seam 让该分支可以确定性回归。
var statusWalkDir = filepath.WalkDir

// walkLocalTree 递归遍历本地目录，只收集常规文件（跳过符号链接、设备文件等）。
// rel_path 统一使用 / 作为分隔符，相对于 root。
//
// 这里只记录路径与 mtime，不计算 MD5：本地 hash 采用惰性策略，仅在文件双端
// 都存在且非 quick 模式时，由 compareTrees 按需计算（见 judgeFileMatch）。
func walkLocalTree(root string) (map[string]*localFile, error) {
	// filepath.WalkDir 不会跟随根本身的符号链接：若 root 是指向目录的 symlink，
	// WalkDir 只以 symlink 身份触发一次回调，随后 !IsRegular 分支静默跳过，最终
	// 返回空索引，让 status 把所有远端文件报成 new_remote。fail-closed 于遍历前：
	// 根必须是真实目录，任何其它类型都必须报错。
	info, err := statusRootLstat(root)
	if err != nil {
		return nil, fmt.Errorf("读取本地根目录身份失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("本地根目录 %q 是符号链接，请传入真实目录路径", root)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("本地根目录 %q 不是目录", root)
	}
	files := make(map[string]*localFile)
	err = statusWalkDir(root, func(path string, d fs.DirEntry, err error) error {
		return walkLocalTreeEntry(root, path, d, err, files)
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// runDriveStatusWalkLocalTree 仅作为 runDriveStatus 扫描失败分支的确定性测试 seam；
// 测试必须通过 testseam.Swap 替换并自动恢复。
var runDriveStatusWalkLocalTree = walkLocalTree

// walkLocalTreeEntry 是 walkLocalTree 的单条目处理逻辑。抽成命名函数只为可测：
// Info() / filepath.Rel 的失败在真实 WalkDir 下几乎不可复现，单测可直接注入。
func walkLocalTreeEntry(root, path string, d fs.DirEntry, err error, files map[string]*localFile) error {
	if err != nil {
		return err
	}
	if d.IsDir() {
		return nil
	}
	info, err := d.Info()
	if err != nil {
		return err
	}
	// 只比对常规文件；符号链接、设备文件等被忽略。
	if !info.Mode().IsRegular() {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)

	files[relSlash] = &localFile{
		RelPath:       relSlash,
		AbsPath:       path,
		Size:          info.Size(),
		ModTimeMillis: info.ModTime().UnixMilli(),
	}
	return nil
}

// 双端都存在文件的比对结论。
const (
	matchUnchanged = "unchanged"
	matchModified  = "modified"
	matchUnknown   = "unknown" // exact 模式且远端无可靠 md5，内容无法核对
)

// judgeFileMatch 判定双端都存在的文件本次的比对结论：unchanged / modified / unknown。
//
// exact 模式下，本地 MD5 在此处按需计算——只有走到这里的文件才是双端都存在的，
// local-only 文件不会触发 hash 计算。若远端未返回 md5（list_files 目前通常如此），
// 无法核对内容，则返回 unknown：不判 unchanged（避免把不同内容误报为未变更），
// 也不硬判 modified（避免把相同内容误报为已变更），让降级如实反映在输出中。
func judgeFileMatch(lf *localFile, rf *remoteFile, quick bool) (string, error) {
	if quick {
		// quick：远端时间戳必须可信且与本地 mtime 相等才算 unchanged；
		// 远端时间缺失/非法时走保守路径记为 modified。
		if rf.ModifiedTimeValid && lf.ModTimeMillis == rf.ModifiedTime {
			return matchUnchanged, nil
		}
		return matchModified, nil
	}

	// exact：远端缺少可靠 md5 时无法核对内容 → unknown（既不判 unchanged 也不判 modified）。
	if rf.Hash == "" {
		return matchUnknown, nil
	}

	// exact：按需计算本地 MD5（base64），再与远端 md5（钉盘也返回 base64）直接比较。
	if lf.Hash == "" {
		h, err := md5File(lf.AbsPath)
		if err != nil {
			return "", fmt.Errorf("计算 %s 的 MD5 失败: %w", lf.RelPath, err)
		}
		lf.Hash = h
	}
	if lf.Hash == rf.Hash {
		return matchUnchanged, nil
	}
	return matchModified, nil
}

// compareTrees 比较本地与云端文件树，产出五类差异。
func compareTrees(local map[string]*localFile, remote map[string]*remoteFile, detection string, quick bool) (driveStatusResult, error) {
	res := driveStatusResult{
		Detection: detection,
		NewLocal:  []driveStatusEntry{},
		NewRemote: []driveStatusEntry{},
		Modified:  []driveStatusEntry{},
		Unchanged: []driveStatusEntry{},
		Unknown:   []driveStatusEntry{},
	}

	for rel, lf := range local {
		rf, ok := remote[rel]
		if !ok {
			res.NewLocal = append(res.NewLocal, driveStatusEntry{RelPath: rel})
			continue
		}
		verdict, err := judgeFileMatch(lf, rf, quick)
		if err != nil {
			return res, err
		}
		switch verdict {
		case matchUnchanged:
			res.Unchanged = append(res.Unchanged, driveStatusEntry{RelPath: rel})
		case matchUnknown:
			res.Unknown = append(res.Unknown, driveStatusEntry{RelPath: rel})
		default:
			res.Modified = append(res.Modified, driveStatusEntry{RelPath: rel})
		}
	}
	for rel := range remote {
		if _, ok := local[rel]; !ok {
			res.NewRemote = append(res.NewRemote, driveStatusEntry{RelPath: rel})
		}
	}

	sortEntries(res.NewLocal)
	sortEntries(res.NewRemote)
	sortEntries(res.Modified)
	sortEntries(res.Unchanged)
	sortEntries(res.Unknown)
	return res, nil
}

func sortEntries(entries []driveStatusEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })
}

func runDriveStatus(cmd *cobra.Command, _ []string) error {
	if err := validateRequiredFlags(cmd, "local-folder", "remote-folder"); err != nil {
		return err
	}
	localDir := mustGetFlag(cmd, "local-folder")
	// remote-folder 必传：待比对的云端根文件夹 dentryUuid。
	remoteDirID := mustGetFlag(cmd, "remote-folder")
	// space-id 可选：不传则由 fetchRemoteDriveTree 使用「我的文件」对应的空间。
	spaceID := mustGetFlag(cmd, "space-id")

	quick, _ := cmd.Flags().GetBool("quick")
	detection := "exact"
	if quick {
		detection = "quick"
	}

	absDir, err := validateLocalDirAbs(localDir)
	if err != nil {
		return err
	}

	local, err := runDriveStatusWalkLocalTree(absDir)
	if err != nil {
		return fmt.Errorf("扫描本地目录失败: %w", err)
	}

	ctx := cmd.Context()
	remoteIndex, err := fetchRemoteDriveTree(ctx, spaceID, remoteDirID, quick)
	if err != nil {
		return err
	}

	result, err := compareTrees(local, remoteIndex, detection, quick)
	if err != nil {
		return err
	}
	return deps.Out.PrintJSON(result)
}

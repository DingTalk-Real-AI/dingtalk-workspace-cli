package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

// errTestDownload 用于模拟 httpGetFile 下载失败。
var errTestDownload = errors.New("simulated download failure")

// swapPullDownloadPath 为存量测试保留基于路径的传输模拟写法。
// 生产 pull/sync 始终把已由 os.Root 安全打开的句柄交给 pullDownloadFile；
// 只有测试适配层才把该句柄的名称传给旧 callback。
func swapPullDownloadPath(t *testing.T, fn func(context.Context, string, map[string]string, string) error) {
	t.Helper()
	testseam.Swap(t, &pullDownloadFile, func(ctx context.Context, url string, headers map[string]string, destination *os.File) error {
		return fn(ctx, url, headers, destination.Name())
	})
}

// ──────────────────────────────────────────────────────────
// driveSyncFailure — sync 部分失败错误：exit=1
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveSyncFailure(t *testing.T) {
	e := &driveSyncFailure{failed: 2}
	if e.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", e.ExitCode())
	}
	if e.Error() != e.RawStderr() {
		t.Error("Error() 与 RawStderr() 应一致")
	}
	if !strings.Contains(e.Error(), "2") {
		t.Errorf("Error() 应包含失败数, got %q", e.Error())
	}
}

// ──────────────────────────────────────────────────────────
// driveSyncSuffixedRel — keep-both 本地重命名目标生成
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveSyncSuffixedRel(t *testing.T) {
	// 基本：扩展名前插入基于 fileId 末 8 位的后缀。
	got := driveSyncSuffixedRel("a/b.txt", "0123456789ABCDEF", map[string]bool{})
	if want := "a/b.conflict-89ABCDEF.txt"; got != want {
		t.Errorf("suffixed = %q, want %q", got, want)
	}

	// 根目录下、无扩展名。
	if got := driveSyncSuffixedRel("readme", "", map[string]bool{}); got != "readme.conflict" {
		t.Errorf("suffixed = %q, want readme.conflict", got)
	}

	// 首选目标已被占用时追加序号，直到不冲突。
	occupied := map[string]bool{
		"x.conflict-FID.log":   true,
		"x.conflict-FID.0.log": true,
	}
	got = driveSyncSuffixedRel("x.log", "FID", occupied)
	if want := "x.conflict-FID.1.log"; got != want {
		t.Errorf("suffixed with collisions = %q, want %q", got, want)
	}
}

// ──────────────────────────────────────────────────────────
// sync 双向流程 — new_local 上传 / new_remote 下载
// ──────────────────────────────────────────────────────────

// runDriveSyncTest 用 mock caller 执行 `drive sync`，注入 opened-file PUT / GET 为
// 可控 no-op，避免真实 OSS 传输。返回 root.Execute() 的结果。
func runDriveSyncTest(t *testing.T, caller *driveSyncMockCaller, localDir string, args ...string) error {
	// 默认下载：把远端内容写到目标路径，模拟成功落盘。
	return runDriveSyncTestWithGet(t, caller, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("remote-content"), 0o644)
	}, localDir, args...)
}

// runDriveSyncTestWithGet 同上，但允许自定义 httpGetFile 行为（例如模拟下载失败）。
func runDriveSyncTestWithGet(t *testing.T, caller *driveSyncMockCaller, getFn func(context.Context, string, map[string]string, string) error, localDir string, args ...string) error {
	t.Helper()

	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return nil })
	swapPullDownloadPath(t, getFn)
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().BoolP("yes", "y", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(newDriveCommand())

	// sync 是 user_required 叶子，非交互环境需 --yes 才能越过统一确认门。
	full := append([]string{"sync", "--local-folder", localDir, "--remote-folder", "ROOT", "--yes"}, args...)
	testseam.Swap(t, &os.Args, append([]string{"dws", "drive"}, full...))
	root.SetArgs(append([]string{"drive"}, full...))

	return root.Execute()
}

// 双向：本地独有 local.txt 应上传；远端独有 remote.txt 应下载到本地。
func TestCrossPlatformCoverageDriveSync_bidirectional(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "local.txt"), "local-only")

	caller := &driveSyncMockCaller{
		// 远端仅有 remote.txt（本地不存在）→ new_remote，应被下载。
		listJSON: `{"result":{"items":[{"name":"remote.txt","type":"file","fileId":"R_FID","md5":"z","modifyTime":2000}],"nextToken":""}}`,
	}
	if err := runDriveSyncTest(t, caller, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// new_local：local.txt 走 get_upload_info（新建上传，传 parentId=ROOT、不传 overwriteFileId）。
	up := caller.callsFor("get_upload_info")
	if len(up) != 1 {
		t.Fatalf("expected 1 get_upload_info call, got %d", len(up))
	}
	if got := up[0].args["parentId"]; got != "ROOT" {
		t.Errorf("get_upload_info parentId = %v, want ROOT", got)
	}
	if _, ok := up[0].args["overwriteFileId"]; ok {
		t.Error("新建上传不应传 overwriteFileId")
	}

	// new_remote：remote.txt 走 download_file，并已落盘到本地。
	dl := caller.callsFor("download_file")
	if len(dl) != 1 || dl[0].args["fileId"] != "R_FID" {
		t.Fatalf("expected 1 download_file(fileId=R_FID), got %v", dl)
	}
	if _, err := os.Stat(filepath.Join(dir, "remote.txt")); err != nil {
		t.Errorf("remote.txt 应被下载到本地: %v", err)
	}
}

// modified + remote-wins：quick 模式下两侧 mtime 不同判为 modified，并显式拉取远端覆盖本地。
func TestCrossPlatformCoverageDriveSync_modifiedRemoteWins(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-old")
	// 本地 mtime 设为与远端不同，quick 模式判为 modified。
	local := readFileMTimeMillis(t, p)

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F_FID","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	}
	if err := runDriveSyncTest(t, caller, dir, "--quick", "--on-conflict", "remote-wins"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// remote-wins → 下载覆盖本地，且不应发生上传。
	if dl := caller.callsFor("download_file"); len(dl) != 1 || dl[0].args["fileId"] != "F_FID" {
		t.Errorf("expected download_file(fileId=F_FID), got %v", dl)
	}
	if up := caller.callsFor("get_upload_info"); len(up) != 0 {
		t.Errorf("remote-wins 不应上传, got %d get_upload_info calls", len(up))
	}
	if b, _ := os.ReadFile(p); string(b) != "remote-content" {
		t.Errorf("本地文件应被远端内容覆盖, got %q", string(b))
	}
}

// modified + local-wins：改用 local-wins 时应覆盖上传远端（传 overwriteFileId、不传 parentId）。
func TestCrossPlatformCoverageDriveSync_modifiedLocalWins(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-new")
	local := readFileMTimeMillis(t, p)

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F_FID","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	}
	if err := runDriveSyncTest(t, caller, dir, "--quick", "--on-conflict", "local-wins"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	up := caller.callsFor("get_upload_info")
	if len(up) != 1 {
		t.Fatalf("expected 1 get_upload_info call, got %d", len(up))
	}
	if got := up[0].args["overwriteFileId"]; got != "F_FID" {
		t.Errorf("local-wins 覆盖上传 overwriteFileId = %v, want F_FID", got)
	}
	if _, ok := up[0].args["parentId"]; ok {
		t.Error("覆盖上传不应传 parentId")
	}
	if dl := caller.callsFor("download_file"); len(dl) != 0 {
		t.Errorf("local-wins 不应下载, got %d download_file calls", len(dl))
	}
}

func TestCrossPlatformCoverageDriveSyncRejectsUnknownCommitEffect(t *testing.T) {
	for _, response := range []string{"", `{}`} {
		t.Run(response, func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, "local.txt"), "payload")
			caller := &driveSyncMockCaller{
				listJSON:   `{"result":{"items":[]}}`,
				commitJSON: &response,
			}
			err, output, putCalls := runDriveMirrorUploadCommandCapture(t, "sync", caller, dir)
			if err == nil {
				t.Fatal("empty commit result must make sync exit non-zero")
			}
			if failure, ok := err.(*driveSyncFailure); !ok || failure.ExitCode() != 1 {
				t.Fatalf("error=%T %v", err, err)
			}
			assertMirrorUploadFailedJSON(t, output)
			if putCalls != 1 {
				t.Fatalf("OSS PUT calls=%d, want exactly 1", putCalls)
			}
			if got := len(caller.callsFor("get_upload_info")); got != 1 {
				t.Fatalf("get_upload_info calls=%d, want 1", got)
			}
			if got := len(caller.callsFor("commit_upload")); got != 1 {
				t.Fatalf("commit_upload calls=%d, want 1", got)
			}
		})
	}
}

// --dry-run：只计算差异，不触发任何 MCP 写操作与本地落盘。
func TestCrossPlatformCoverageDriveSync_dryRun(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "local.txt"), "x")

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[{"name":"remote.txt","type":"file","fileId":"R_FID","modifyTime":2000}],"nextToken":""}}`,
		dryRun:   true,
	}
	if err := runDriveSyncTest(t, caller, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(caller.callsFor("download_file")) + len(caller.callsFor("get_upload_info")); got != 0 {
		t.Errorf("dry-run 不应触发下载/上传, got %d 次写操作调用", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "remote.txt")); !os.IsNotExist(err) {
		t.Error("dry-run 不应把远端文件落盘到本地")
	}
}

// remote-wins 下载失败：本地原文件必须原样保留（pullOneFile 先写临时文件、成功才原子
// rename 覆盖），且命令以非零退出码退出。
func TestCrossPlatformCoverageDriveSync_pullFailureKeepsLocal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-original")
	local := readFileMTimeMillis(t, p)

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F_FID","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	}
	failGet := func(context.Context, string, map[string]string, string) error {
		return errTestDownload
	}
	err := runDriveSyncTestWithGet(t, caller, failGet, dir, "--quick", "--on-conflict", "remote-wins")
	if err == nil {
		t.Fatal("下载失败时应以非零退出码退出")
	}
	if ec, ok := err.(*driveSyncFailure); !ok || ec.ExitCode() != 1 {
		t.Fatalf("期望 driveSyncFailure(exit=1), got %T %v", err, err)
	}
	// 关键：原文件内容与存在性都不被破坏。
	if b, rerr := os.ReadFile(p); rerr != nil || string(b) != "local-original" {
		t.Errorf("下载失败后本地原文件应原样保留, got %q err=%v", string(b), rerr)
	}
}

// keep-both 成功路径：先以 no-clobber 硬链接保留本地版本，再把远端拉取到原路径。
func TestCrossPlatformCoverageDriveSync_keepBothSuccess(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-new")
	local := readFileMTimeMillis(t, p)

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F_FID","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	}
	if err := runDriveSyncTest(t, caller, dir, "--quick", "--on-conflict", "keep-both"); err != nil {
		t.Fatalf("keep-both 成功路径不应报错: %v", err)
	}

	// 原路径 f.txt 应为远端内容（拉取落盘）。
	if b, _ := os.ReadFile(p); string(b) != "remote-content" {
		t.Errorf("f.txt 应为远端内容, got %q", string(b))
	}
	// 本地版本改名保留为 f.conflict-F_FID.txt，内容不变。
	aside := filepath.Join(dir, "f.conflict-F_FID.txt")
	if b, err := os.ReadFile(aside); err != nil || string(b) != "local-new" {
		t.Errorf("本地版本应改名保留为 %s 且内容不变, got %q err=%v", filepath.Base(aside), string(b), err)
	}
	// 下载发生一次（拉取远端到原路径），不应有上传。
	if dl := caller.callsFor("download_file"); len(dl) != 1 {
		t.Errorf("keep-both 应拉取远端一次, got %d download_file calls", len(dl))
	}
	if up := caller.callsFor("get_upload_info"); len(up) != 0 {
		t.Errorf("keep-both 不应上传, got %d get_upload_info calls", len(up))
	}
}

// keep-both 下载失败：不做可能误删并发文件的回滚，原名与本地保留候选都应保留。
func TestCrossPlatformCoverageDriveSync_keepBothPreservesLinkOnPullFailure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-original")
	local := readFileMTimeMillis(t, p)

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F_FID","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	}
	failGet := func(context.Context, string, map[string]string, string) error {
		return errTestDownload
	}
	err := runDriveSyncTestWithGet(t, caller, failGet, dir, "--quick", "--on-conflict", "keep-both")
	if err == nil {
		t.Fatal("下载失败时应以非零退出码退出")
	}
	// 原名尚未被远端发布，保持原内容。
	if b, rerr := os.ReadFile(p); rerr != nil || string(b) != "local-original" {
		t.Errorf("keep-both 拉取失败后原文件应保留: got %q err=%v", string(b), rerr)
	}
	aside := filepath.Join(dir, "f.conflict-F_FID.txt")
	if b, rerr := os.ReadFile(aside); rerr != nil || string(b) != "local-original" {
		t.Errorf("keep-both 拉取失败后候选应保留: got %q err=%v", string(b), rerr)
	}
}

// reserveSyncKeepBothTarget 不得占用「精确同名」的既有文件（occupied 未记录时），
// 应原子占位到下一个后缀，且既有文件内容不被破坏。此断言在任何文件系统上都成立。
func TestCrossPlatformCoverageReserveSyncKeepBothTarget_noOverwriteExact(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "f.txt"), "local-data")
	// 预置一个与首选候选精确同名的既有文件（模拟未被 occupied 记录的外部残留）。
	first := "f.conflict-ABCD1234.txt"
	mustWrite(t, filepath.Join(dir, first), "existing-data")

	rel, abs, err := reserveSyncKeepBothTarget(dir, "f.txt", "0000ABCD1234", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if rel == first {
		t.Fatalf("不应占用既有文件 %s", first)
	}
	if want := "f.conflict-ABCD1234.1.txt"; rel != want {
		t.Errorf("rel = %q, want %q", rel, want)
	}
	// 既有文件内容原样保留（未被覆盖）。
	if b, _ := os.ReadFile(filepath.Join(dir, first)); string(b) != "existing-data" {
		t.Error("既有文件被覆盖，数据丢失")
	}
	// 返回的目标是原始本地版本的硬链接，不是空占位。
	if b, e := os.ReadFile(abs); e != nil || string(b) != "local-data" {
		t.Errorf("候选硬链接内容错误: content=%q err=%v", b, e)
	}
}

// reserveSyncKeepBothTarget 在大小写不敏感文件系统上，不得覆盖与候选「大小写等价」的
// 既有文件——O_EXCL 由 OS 兜底判等价性，occupied 的精确字符串键覆盖不到这种情况。
func TestCrossPlatformCoverageReserveSyncKeepBothTarget_noOverwriteCaseEquivalent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "f.txt"), "local-data")
	if !isCaseInsensitiveFS(dir) {
		t.Skip("文件系统大小写敏感，跳过大小写等价覆盖测试")
	}
	// 既有文件用大写变体，occupied 为空（模拟精确键查重覆盖不到的等价文件）。
	existing := "f.CONFLICT-ABCD1234.TXT"
	mustWrite(t, filepath.Join(dir, existing), "case-variant-data")

	rel, _, err := reserveSyncKeepBothTarget(dir, "f.txt", "0000abcd1234", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// 生成的小写候选与既有大写文件在不敏感 FS 上等价，本不应被占用。
	if strings.EqualFold(rel, "f.conflict-abcd1234.txt") {
		t.Errorf("首选候选与既有文件等价，不应被占用: %q", rel)
	}
	// 既有文件内容原样保留。
	if b, _ := os.ReadFile(filepath.Join(dir, existing)); string(b) != "case-variant-data" {
		t.Error("大小写等价的既有文件被覆盖，数据丢失")
	}
}

// 非交互 ask（stdin 立即 EOF）：冲突项应被跳过而非中止整个同步，
// new_local 仍上传、new_remote 仍下载，且被跳过的 modified 本地文件不被改动。
func TestCrossPlatformCoverageDriveSync_askNonInteractiveSkipsConflict(t *testing.T) {
	testseam.Swap(t, &syncAskStdin, io.Reader(strings.NewReader(""))) // 非交互：立即 EOF

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "local.txt"), "local-only") // new_local → 应上传
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-old") // modified → ask 非交互跳过
	local := readFileMTimeMillis(t, p)

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[` +
			`{"name":"f.txt","type":"file","fileId":"F_FID","modifyTime":` + differentMillis(local) + `},` +
			`{"name":"remote.txt","type":"file","fileId":"R_FID","modifyTime":2000}` +
			`],"nextToken":""}}`,
	}
	if err := runDriveSyncTest(t, caller, dir, "--quick", "--on-conflict", "ask"); err != nil {
		t.Fatalf("非交互 ask 不应中止同步: %v", err)
	}

	// new_local 上传、new_remote 下载都应照常发生。
	if up := caller.callsFor("get_upload_info"); len(up) != 1 {
		t.Errorf("new_local 应被上传, got %d get_upload_info calls", len(up))
	}
	if dl := caller.callsFor("download_file"); len(dl) != 1 || dl[0].args["fileId"] != "R_FID" {
		t.Errorf("new_remote 应被下载, got %v", dl)
	}
	// modified 冲突被跳过：本地 f.txt 未被覆盖。
	if b, _ := os.ReadFile(p); string(b) != "local-old" {
		t.Errorf("ask 跳过的 modified 文件不应被改动, got %q", string(b))
	}
}

// readFileMTimeMillis 返回文件的 mtime（Unix 毫秒）。
func readFileMTimeMillis(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.ModTime().UnixMilli()
}

// differentMillis 返回一个与 base 不同的毫秒数字符串，保证 quick 模式判为 modified。
func differentMillis(base int64) string {
	return strconv.FormatInt(base+60000, 10) // 差 1 分钟，足以区分
}

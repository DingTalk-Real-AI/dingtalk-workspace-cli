package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

var errTestRead = errors.New("simulated read failure")

// errReader 让 bufio 读取返回非 EOF 错误，覆盖 ask 的读取失败分支。
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errTestRead }

// findDriveSubcommand 取出 drive 下的指定子命令，便于直接调用其 RunE。
func findDriveSubcommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, c := range newDriveCommand().Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("drive subcommand %q not found", name)
	return nil
}

// mustSetFlags 设置一组 flag，任何失败都直接终止用例。
func mustSetFlags(t *testing.T, cmd *cobra.Command, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s=%s: %v", k, v, err)
		}
	}
}

// ──────────────────────────────────────────────────────────
// --if-exists 显式空值回落默认
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDrivePull_emptyIfExistsDefaultsToSkip(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	mustWrite(t, p, "local-old")
	caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":1}`)
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("remote-new"), 0o644)
	})

	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--if-exists", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "local-old" {
		t.Errorf("empty --if-exists must default to skip and keep the local file, got %q", string(b))
	}
}

func TestCrossPlatformCoverageDrivePush_emptyIfExistsDefaultsToSkip(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "local")
	withNoopPut(t)

	caller := pushOKCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A","modifyTime":1}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT", "--if-exists", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := caller.callsFor("get_upload_info"); len(got) != 0 {
		t.Errorf("empty --if-exists must behave as skip, got %v", got)
	}
}

// ──────────────────────────────────────────────────────────
// pullOneFile 的落盘失败分支
// ──────────────────────────────────────────────────────────

// 目标目录不可写 → 创建临时文件失败。
func TestCrossPlatformCoveragePullOneFile_tempFileCreationFailure(t *testing.T) {
	root := t.TempDir()
	testseam.Swap(t, &pullCreateTemp, func(*os.Root) (*os.File, string, error) {
		return nil, "", errors.New("create temp boom")
	})

	testseam.Swap(t, &deps, &Deps{Caller: pullListingCaller(""), Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	action, err := pullOneFile(context.Background(), "", &remoteFile{RelPath: "a.txt", FileID: "A"},
		filepath.Join(root, "a.txt"), ifExistsOverwrite)
	if action != pullActionFailed || err == nil {
		t.Fatalf("action=%q err=%v, want failed + error", action, err)
	}
}

// 目标路径是目录 → 最后的原子 rename 失败。
func TestCrossPlatformCoveragePullOneFile_renameOntoDirectoryFails(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	if err := os.MkdirAll(target, 0o755); err != nil { // 目标被目录占位
		t.Fatal(err)
	}

	testseam.Swap(t, &deps, &Deps{Caller: pullListingCaller(""), Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("payload"), 0o644)
	})

	action, err := pullOneFile(context.Background(), "", &remoteFile{RelPath: "a.txt", FileID: "A"},
		target, ifExistsOverwrite)
	if action != pullActionFailed || err == nil {
		t.Fatalf("action=%q err=%v, want failed + error", action, err)
	}
}

// ──────────────────────────────────────────────────────────
// PrintJSON 失败分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDrivePull_printFailurePropagates(t *testing.T) {
	dir := t.TempDir()

	testseam.Swap(t, &deps, &Deps{Caller: pullListingCaller(""), Out: &Formatter{w: failingWriter{}}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive", "pull"})

	cmd := findDriveSubcommand(t, "pull")
	mustSetFlags(t, cmd, map[string]string{"local-folder": dir, "remote-folder": "ROOT"})
	if err := runDrivePull(cmd, nil); err == nil {
		t.Fatal("expected the PrintJSON writer failure to propagate")
	}
}

func TestCrossPlatformCoverageDrivePush_printFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	withNoopPut(t)

	testseam.Swap(t, &deps, &Deps{Caller: pushOKCaller(nil), Out: &Formatter{w: failingWriter{}}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive", "push"})

	cmd := findDriveSubcommand(t, "push")
	mustSetFlags(t, cmd, map[string]string{"local-folder": dir, "remote-folder": "ROOT"})
	if err := runDrivePush(cmd, nil); err == nil {
		t.Fatal("expected the PrintJSON writer failure to propagate")
	}
}

func TestCrossPlatformCoverageDriveSync_printFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return nil })

	testseam.Swap(t, &deps, &Deps{Caller: syncCaller(nil), Out: &Formatter{w: failingWriter{}}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive", "sync"})

	cmd := findDriveSubcommand(t, "sync")
	mustSetFlags(t, cmd, map[string]string{"local-folder": dir, "remote-folder": "ROOT", "quick": "true"})
	if err := runDriveSync(cmd, nil); err == nil {
		t.Fatal("expected the PrintJSON writer failure to propagate")
	}
}

// ──────────────────────────────────────────────────────────
// walkLocalForPush / walkLocalTree 的 WalkDir 错误回调
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageWalkLocal_unreadableSubdirectoryErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable dir needs POSIX perms and a non-root user")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "locked")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(sub, "inner.txt"), "x")
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

	if _, _, err := walkLocalForPush(dir); err == nil {
		t.Error("walkLocalForPush must surface the WalkDir error")
	}
	if _, err := walkLocalTree(dir); err == nil {
		t.Error("walkLocalTree must surface the WalkDir error")
	}
}

// ──────────────────────────────────────────────────────────
// verifyNoSymlinkEscape 的剩余分支
// ──────────────────────────────────────────────────────────

// 目标位于 absDir 之外 → 走到根目录之上即放行（不是逃逸判定范围）。
func TestCrossPlatformCoverageVerifyNoSymlinkEscape_targetOutsideRootIsIgnored(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere", "x.txt")
	if err := verifyNoSymlinkEscape(root, outside); err != nil {
		t.Errorf("target above the root must be ignored, got %v", err)
	}
}

// 已存在路径含符号链接环 → EvalSymlinks 失败要如实上报。
func TestCrossPlatformCoverageVerifyNoSymlinkEscape_symlinkLoopErrors(t *testing.T) {
	root := t.TempDir()
	loop := filepath.Join(root, "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skipf("self-symlink unsupported: %v", err)
	}
	err := verifyNoSymlinkEscape(root, filepath.Join(loop, "child.txt"))
	if err == nil || !strings.Contains(err.Error(), "解析路径") {
		t.Fatalf("expected EvalSymlinks failure, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// reserveSyncKeepBothTarget 的失败分支
// ──────────────────────────────────────────────────────────

// 候选目标逃逸出本地根（父目录是指向外部的软链）→ 直接失败。
func TestCrossPlatformCoverageReserveSyncKeepBothTarget_escapeFails(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := reserveSyncKeepBothTarget(root, "sub/f.txt", "FID12345678", map[string]bool{}); err == nil {
		t.Fatal("expected escaping keep-both target to fail")
	}
}

// 候选硬链接创建返回非 EEXIST 错误时直接上报。
func TestCrossPlatformCoverageReserveSyncKeepBothTarget_unwritableDirFails(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "local")
	testseam.Swap(t, &syncRootLink, func(*os.Root, string, string) error {
		return errors.New("link boom")
	})

	if _, _, err := reserveSyncKeepBothTarget(root, "f.txt", "FID12345678", map[string]bool{}); err == nil {
		t.Fatal("expected unwritable directory to fail")
	}
}

// occupied 中已知占用的首选候选被跳过，改用下一个后缀。
func TestCrossPlatformCoverageReserveSyncKeepBothTarget_skipsKnownOccupied(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "local")
	first := syncKeepBothCandidate("f.txt", "FID12345678", 0)
	occupied := map[string]bool{first: true}

	got, _, err := reserveSyncKeepBothTarget(root, "f.txt", "FID12345678", occupied)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == first {
		t.Errorf("known-occupied candidate %q must be skipped", first)
	}
}

// ──────────────────────────────────────────────────────────
// ask 读取失败（非 EOF）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveSyncAskConflict_readErrorPropagates(t *testing.T) {
	testseam.Swap(t, &syncAskStdin, io.Reader(errReader{}))

	if _, err := driveSyncAskConflict("f.txt"); err == nil {
		t.Fatal("expected read failure to propagate")
	}
}

// ──────────────────────────────────────────────────────────
// sync：本地根不可创建
// ──────────────────────────────────────────────────────────

// --local-folder 指向一个常规文件 → MkdirAll 失败。
func TestCrossPlatformCoverageDriveSync_localRootIsFileFails(t *testing.T) {
	dir := t.TempDir()
	asFile := filepath.Join(dir, "not-a-dir")
	mustWrite(t, asFile, "x")

	if err := runDriveCmd(t, syncCaller(nil), "sync", "--local-folder", asFile, "--remote-folder", "ROOT", "--quick"); err == nil {
		t.Fatal("expected a regular-file local root to fail")
	}
}

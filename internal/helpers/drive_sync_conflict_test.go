package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// failFolderCaller 让指定名字的 create_folder 失败，其余一切正常，
// 用于构造「父目录未能创建 → 子目录/子文件连锁失败」。
func failFolderCaller(failName string) *driveScriptCaller {
	return &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
		switch tool {
		case "list_files":
			return `{"result":{"items":[],"nextToken":""}}`, nil
		case "create_folder":
			if args["name"] == failName {
				return "", errors.New("create_folder rejected: " + failName)
			}
			return `{"result":{"fileId":"NEWDIR"},"success":true}`, nil
		case "get_upload_info":
			return `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`, nil
		case "download_file":
			return `{"result":{"downloadUrl":"https://oss.example.com/get","headers":{}}}`, nil
		}
		return `{"result":{"fileId":"NEWFILE"},"success":true}`, nil
	}}
}

// mkNested 建出 a/b/deep.txt 结构。
func mkNested(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "a", "b", "deep.txt"), "d")
	return dir
}

// 顶层目录 a 建失败 → 子目录 a/b 因父目录缺失也 failed，其中文件同样 failed。
func TestCrossPlatformCoverageDrivePush_nestedDirFailsWhenParentMissing(t *testing.T) {
	dir := mkNested(t)
	withNoopPut(t)

	caller := failFolderCaller("a")
	err := runDriveCmd(t, caller, "push", "--local-folder", dir, "--remote-folder", "ROOT")
	var pf *drivePushFailure
	if !errors.As(err, &pf) {
		t.Fatalf("expected drivePushFailure, got %T %v", err, err)
	}
	// a（建失败）+ a/b（父缺失）+ a/b/deep.txt（父缺失）= 3
	if pf.failed != 3 {
		t.Errorf("failed = %d, want 3", pf.failed)
	}
	if got := caller.callsFor("get_upload_info"); len(got) != 0 {
		t.Errorf("nothing may upload without a parent folder, got %v", got)
	}
}

func TestCrossPlatformCoverageDriveSync_nestedDirFailsWhenParentMissing(t *testing.T) {
	dir := mkNested(t)
	withNoopPut(t)

	caller := failFolderCaller("a")
	err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick")
	var sf *driveSyncFailure
	if !errors.As(err, &sf) {
		t.Fatalf("expected driveSyncFailure, got %T %v", err, err)
	}
	if sf.failed != 3 {
		t.Errorf("failed = %d, want 3", sf.failed)
	}
}

// new_remote 落盘目标经软链逃逸出本地根 → 记 failed，不写到根目录之外。
func TestCrossPlatformCoverageDriveSync_newRemoteEscapeIsFailed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	withSyncTransport(t, "leak")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"sub","type":"folder","fileId":"SUB"}],"nextToken":""}}`,
		"SUB":  `{"result":{"items":[{"name":"leaked.txt","type":"file","fileId":"L","modifyTime":1}],"nextToken":""}}`,
	})
	err := runDriveCmd(t, caller, "sync", "--local-folder", root, "--remote-folder", "ROOT", "--quick")
	var sf *driveSyncFailure
	if !errors.As(err, &sf) {
		t.Fatalf("expected driveSyncFailure, got %T %v", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "leaked.txt")); !os.IsNotExist(statErr) {
		t.Error("file escaped the local root")
	}
}

// ──────────────────────────────────────────────────────────
// keep-both 的失败与回滚分支
// ──────────────────────────────────────────────────────────

// syncKeepBoth 在 rel 逃逸时直接 failed（resolveLocalTarget 报错）。
func TestCrossPlatformCoverageSyncKeepBoth_escapingRelIsFailed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	res := &driveSyncResult{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{RelPath: "sub/f.txt", FileID: "FID12345678"},
		root, "sub/f.txt", map[string]bool{})
	if res.Summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", res.Summary.Failed)
	}
}

// 原子建立本地保留硬链接失败 → failed。
func TestCrossPlatformCoverageSyncKeepBoth_reserveFailureIsFailed(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "local")
	testseam.Swap(t, &syncRootLink, func(*os.Root, string, string) error {
		return errors.New("link boom")
	})

	res := &driveSyncResult{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{RelPath: "f.txt", FileID: "FID12345678"},
		root, "f.txt", map[string]bool{})
	if res.Summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", res.Summary.Failed)
	}
}

// Link 失败不得留下空占位，也不得移动或修改原文件。
func TestCrossPlatformCoverageSyncKeepBoth_linkFailureLeavesOriginal(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "local")
	testseam.Swap(t, &syncRootLink, func(*os.Root, string, string) error {
		return errors.New("link boom")
	})
	res := &driveSyncResult{}
	occupied := map[string]bool{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{RelPath: "f.txt", FileID: "FID12345678"},
		root, "f.txt", occupied)

	if res.Summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", res.Summary.Failed)
	}
	if _, err := os.Stat(filepath.Join(root, syncKeepBothCandidate("f.txt", "FID12345678", 0))); !os.IsNotExist(err) {
		t.Error("failed link must not leave a placeholder")
	}
	if content, err := os.ReadFile(filepath.Join(root, "f.txt")); err != nil || string(content) != "local" {
		t.Fatalf("original changed after failed link: content=%q err=%v", content, err)
	}
}

// 拉取失败时不做回滚：原名尚未发布，候选硬链接也保留，避免误删并发对象。
func TestCrossPlatformCoverageSyncKeepBoth_pullFailurePreservesCandidate(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f.txt")
	mustWrite(t, p, "local-version")

	testseam.Swap(t, &deps, &Deps{Caller: syncCaller(nil), Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error { return errTestDownload })

	res := &driveSyncResult{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{RelPath: "f.txt", FileID: "FID12345678"},
		root, "f.txt", map[string]bool{})

	if res.Summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", res.Summary.Failed)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "local-version" {
		t.Fatalf("original must remain before publish: %v / %q", err, string(b))
	}
	suffixed := filepath.Join(root, syncKeepBothCandidate("f.txt", "FID12345678", 0))
	if b, err := os.ReadFile(suffixed); err != nil || string(b) != "local-version" {
		t.Fatalf("candidate must remain after pull failure: %v / %q", err, string(b))
	}
	originalInfo, _ := os.Lstat(p)
	candidateInfo, _ := os.Lstat(suffixed)
	if !os.SameFile(originalInfo, candidateInfo) {
		t.Fatal("candidate must be a hard link to the original local version")
	}
}

// 拉取失败只能上报 failed，不得产生 renamed_local 成功项。
func TestCrossPlatformCoverageSyncKeepBoth_pullFailureDoesNotReportRenamed(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f.txt")
	mustWrite(t, p, "local-version")

	testseam.Swap(t, &deps, &Deps{Caller: syncCaller(nil), Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error { return errTestDownload })

	res := &driveSyncResult{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{RelPath: "f.txt", FileID: "FID12345678"},
		root, "f.txt", map[string]bool{})

	if res.Summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", res.Summary.Failed)
	}
	if len(res.Items) != 1 || res.Items[0].Action != syncActionFailed {
		t.Fatalf("items = %+v", res.Items)
	}
	suffixed := syncKeepBothCandidate("f.txt", "FID12345678", 0)
	if b, err := os.ReadFile(filepath.Join(root, suffixed)); err != nil || string(b) != "local-version" {
		t.Fatalf("local version must survive as %s: %v / %q", suffixed, err, string(b))
	}
	if res.Items[0].Action == syncActionRenamedLocal {
		t.Fatalf("pull failure must not report renamed_local: %#v", res.Items)
	}
}

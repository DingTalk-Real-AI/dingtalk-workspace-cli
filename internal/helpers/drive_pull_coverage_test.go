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
)

// ──────────────────────────────────────────────────────────
// runDrivePull — 端到端：远端树 → 本地落盘
// ──────────────────────────────────────────────────────────

// pullListingCaller 返回固定的单层远端清单，download_file 给出预签名链接。
func pullListingCaller(items string) *driveScriptCaller {
	return &driveScriptCaller{reply: func(tool string, _ map[string]any, nth int) (string, error) {
		switch tool {
		case "list_files":
			if nth > 0 {
				return `{"result":{"items":[],"nextToken":""}}`, nil
			}
			return `{"result":{"items":[` + items + `],"nextToken":""}}`, nil
		case "download_file":
			return `{"result":{"downloadUrl":"https://oss.example.com/get","headers":{}}}`, nil
		}
		return `{"result":{},"success":true}`, nil
	}}
}

// 端到端 pull：远端两个文件都下载到本地，并自动创建缺失的本地根目录。
func TestCrossPlatformCoverageDrivePull_endToEndDownloadsAll(t *testing.T) {
	root := filepath.Join(t.TempDir(), "created-by-pull")
	caller := pullListingCaller(
		`{"name":"a.txt","type":"file","fileId":"A","modifyTime":1000},` +
			`{"name":"b.txt","type":"file","fileId":"B","modifyTime":2000}`)

	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("remote"), 0o644)
	})

	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if b, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(b) != "remote" {
			t.Errorf("%s not written: %v / %q", name, err, string(b))
		}
	}
}

// --if-exists skip：本地已存在则不下载。
func TestCrossPlatformCoverageDrivePull_ifExistsSkipKeepsLocal(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "local-original")
	caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":9000}`)

	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
		t.Error("skip must not download")
		return nil
	})

	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--if-exists", "skip"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(b) != "local-original" {
		t.Errorf("local file changed: %q", string(b))
	}
}

// --if-exists smart：本地 mtime 已 ≥ 远端 → 跳过；远端更新 → 下载。
func TestCrossPlatformCoverageDrivePull_ifExistsSmart(t *testing.T) {
	t.Run("local newer skips", func(t *testing.T) {
		root := t.TempDir()
		p := filepath.Join(root, "a.txt")
		mustWrite(t, p, "local")
		local := readFileMTimeMillis(t, p)
		caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":` + strconv.FormatInt(local-5000, 10) + `}`)

		swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
			t.Error("smart must not download when local is newer")
			return nil
		})

		if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--if-exists", "smart"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b, _ := os.ReadFile(p); string(b) != "local" {
			t.Errorf("local file changed: %q", string(b))
		}
	})

	t.Run("remote newer downloads", func(t *testing.T) {
		root := t.TempDir()
		p := filepath.Join(root, "a.txt")
		mustWrite(t, p, "local")
		local := readFileMTimeMillis(t, p)
		caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":` + strconv.FormatInt(local+60000, 10) + `}`)

		swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
			return os.WriteFile(dest, []byte("fresh"), 0o644)
		})

		if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--if-exists", "smart"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b, _ := os.ReadFile(p); string(b) != "fresh" {
			t.Errorf("expected download, got %q", string(b))
		}
	})
}

// --if-exists 非法值直接拒绝。
func TestCrossPlatformCoverageDrivePull_invalidIfExistsRejected(t *testing.T) {
	caller := pullListingCaller("")
	err := runDriveCmd(t, caller, "pull", "--local-folder", t.TempDir(), "--remote-folder", "ROOT", "--if-exists", "bogus")
	if err == nil || !strings.Contains(err.Error(), "--if-exists") {
		t.Fatalf("expected --if-exists rejection, got %v", err)
	}
}

// space-id 透传到 download_file。
func TestCrossPlatformCoverageDrivePull_passesSpaceIDToDownload(t *testing.T) {
	root := t.TempDir()
	caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":1}`)
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("x"), 0o644)
	})

	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--space-id", "SP9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := caller.callsFor("download_file")[0].args["spaceId"]; got != "SP9" {
		t.Errorf("download_file spaceId = %v, want SP9", got)
	}
}

// 下载失败 → 该项记 failed，命令以 partial_failure 非零退出。
func TestCrossPlatformCoverageDrivePull_downloadFailureIsPartialFailure(t *testing.T) {
	root := t.TempDir()
	caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A","modifyTime":1}`)
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error { return errTestDownload })

	var out strings.Builder
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: &out}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive", "pull"})

	cmd := findDriveSubcommand(t, "pull")
	mustSetFlags(t, cmd, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
	err := runDrivePull(cmd, nil)
	var pf *drivePartialFailure
	if !errors.As(err, &pf) {
		t.Fatalf("expected drivePartialFailure, got %T %v", err, err)
	}
	if pf.failed != 1 || pf.RawStderr() != "drive pull: 1 file(s) failed" {
		t.Errorf("partial failure = %#v, stderr = %q", pf, pf.RawStderr())
	}
	stdout := out.String()
	if !strings.Contains(stdout, `"failed": 1`) || !strings.Contains(stdout, `"rel_path": "a.txt"`) {
		t.Errorf("stdout must retain the structured partial result: %s", stdout)
	}
}

// download_file 未返回下载链接 → failed（不是 panic，也不落盘）。
func TestCrossPlatformCoverageDrivePull_missingDownloadURLFails(t *testing.T) {
	root := t.TempDir()
	caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, nth int) (string, error) {
		if tool == "list_files" {
			if nth > 0 {
				return `{"result":{"items":[],"nextToken":""}}`, nil
			}
			return `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}],"nextToken":""}}`, nil
		}
		return `{"result":{"fileId":"A"},"success":true}`, nil // 无 downloadUrl
	}}
	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected failure when download_file returns no URL")
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Error("nothing must be written when the download URL is missing")
	}
}

// download_file 的 MCP 调用本身失败 → failed。
func TestCrossPlatformCoverageDrivePull_downloadToolErrorFails(t *testing.T) {
	root := t.TempDir()
	caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, nth int) (string, error) {
		if tool == "list_files" {
			if nth > 0 {
				return `{"result":{"items":[],"nextToken":""}}`, nil
			}
			return `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}],"nextToken":""}}`, nil
		}
		return "", errTestDownload
	}}
	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected failure when download_file errors")
	}
}

// 远端名称逃逸出本地根 → 记 failed，不落盘（resolveLocalTarget 二次确认）。
func TestCrossPlatformCoverageDrivePull_escapingRelPathIsFailed(t *testing.T) {
	root := t.TempDir()
	// 远端目录名合法，但目录内的软链把落盘点指到根目录外。
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	caller := &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
		if tool != "list_files" {
			return `{"result":{"downloadUrl":"https://oss.example.com/get"}}`, nil
		}
		switch args["parentId"] {
		case "ROOT":
			return `{"result":{"items":[{"name":"sub","type":"folder","fileId":"SUB"}],"nextToken":""}}`, nil
		case "SUB":
			return `{"result":{"items":[{"name":"leaked.txt","type":"file","fileId":"L"}],"nextToken":""}}`, nil
		}
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("leak"), 0o644)
	})

	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected symlink escape to be reported as failure")
	}
	if _, err := os.Stat(filepath.Join(outside, "leaked.txt")); !os.IsNotExist(err) {
		t.Error("file escaped the local root")
	}
}

// 目标父目录不可创建（同名常规文件占位）→ failed，不 panic。
func TestCrossPlatformCoveragePullOneFile_mkdirFailureIsFailed(t *testing.T) {
	root := t.TempDir()
	// "sub" 是普通文件，MkdirAll("sub") 必然失败。
	mustWrite(t, filepath.Join(root, "sub"), "occupied")

	testseam.Swap(t, &deps, &Deps{Caller: pullListingCaller(""), Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	rf := &remoteFile{RelPath: "sub/x.txt", FileID: "X"}
	action, err := pullOneFile(context.Background(), "", rf, filepath.Join(root, "sub", "x.txt"), ifExistsOverwrite)
	if action != pullActionFailed || err == nil {
		t.Fatalf("action=%q err=%v, want failed + error", action, err)
	}
}

// 探测目录不存在时 isCaseInsensitiveFS 回退平台默认，不 panic。
func TestCrossPlatformCoverageIsCaseInsensitiveFS_missingDirFallsBackToPlatformDefault(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	// 只断言不 panic 且返回布尔（具体值取决于平台默认）。
	_ = isCaseInsensitiveFS(missing)
}

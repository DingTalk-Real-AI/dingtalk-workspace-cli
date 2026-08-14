package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// TestCrossPlatformCoverageDriveMirrorCoverageGaps 覆盖镜像整批 preflight 引入的
// 输出失败和防御式解析分支。测试只调用命名 helper，不依赖真实 Drive 或网络。
func TestCrossPlatformCoverageDriveMirrorCoverageGaps(t *testing.T) {
	t.Run("preflight output failures", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "a.txt"), "local")
		caller := syncCaller(map[string]string{
			"ROOT": `{"result":{"items":[{"name":"A.txt","type":"file","fileId":"REMOTE"}],"nextToken":""}}`,
		})
		testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: failingWriter{}}})
		testseam.Swap(t, &os.Args, []string{"dws", "drive"})

		pull := findDriveSubcommand(t, "pull")
		mustSetFlags(t, pull, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
		if err := runDrivePull(pull, nil); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("pull preflight output error = %v", err)
		}

		push := findDriveSubcommand(t, "push")
		mustSetFlags(t, push, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
		if err := runDrivePush(push, nil); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("push preflight output error = %v", err)
		}

		sync := findDriveSubcommand(t, "sync")
		mustSetFlags(t, sync, map[string]string{"local-folder": root, "remote-folder": "ROOT", "quick": "true"})
		if err := runDriveSync(sync, nil); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("sync preflight output error = %v", err)
		}
	})

	t.Run("pull remote-only preflight output failure", func(t *testing.T) {
		root := t.TempDir()
		caller := syncCaller(map[string]string{
			"ROOT": `{"result":{"items":[{"name":"A.txt","type":"file","fileId":"UP"},{"name":"a.txt","type":"file","fileId":"LOW"}],"nextToken":""}}`,
		})
		testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: failingWriter{}}})
		testseam.Swap(t, &os.Args, []string{"dws", "drive"})
		pull := findDriveSubcommand(t, "pull")
		mustSetFlags(t, pull, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
		if err := runDrivePull(pull, nil); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("pull remote-only preflight output error = %v", err)
		}
	})

	t.Run("dry run preflight output failures", func(t *testing.T) {
		root := t.TempDir()
		remote := map[string]*remoteFile{"A.txt": {RelPath: "A.txt"}, "a.txt": {RelPath: "a.txt"}}
		testseam.Swap(t, &deps, &Deps{Out: &Formatter{w: failingWriter{}}})
		if err := printDrivePullDryRun(root, ifExistsSkip, remote, []string{"A.txt", "a.txt"}, true); err == nil {
			t.Fatal("pull dry-run preflight must propagate output failure")
		}
		local := []localPushFile{{RelPath: "a.txt"}}
		if err := printDrivePushDryRun(ifExistsSkip,
			map[string]*remoteFile{"A.txt": {RelPath: "A.txt"}}, map[string]string{"": "ROOT"}, nil, local); err == nil {
			t.Fatal("push dry-run preflight must propagate output failure")
		}
	})

	t.Run("parser errors", func(t *testing.T) {
		for _, body := range []string{
			`{"result":`,
			`{"result":{"items":[],"hasMore":"bad"}}`,
			`{"result":{"items":null}}`,
			`{"result":{"items":"bad"}}`,
			`{"result":[`,
			`{"result":[true]}`,
			`{"result":[{}]}`,
			`{"result":true}`,
		} {
			if _, _, err := parseDriveList(body); err == nil {
				t.Errorf("parseDriveList(%q) unexpectedly succeeded", body)
			}
		}
	})

	t.Run("pull dry-run existing-file policies", func(t *testing.T) {
		root := t.TempDir()
		skipPath := filepath.Join(root, "skip.txt")
		smartPath := filepath.Join(root, "smart.txt")
		mustWrite(t, skipPath, "skip")
		mustWrite(t, smartPath, "smart")
		smartMTime := readFileMTimeMillis(t, smartPath)
		remote := map[string]*remoteFile{
			"skip.txt":  {RelPath: "skip.txt"},
			"smart.txt": {RelPath: "smart.txt", ModifiedTime: smartMTime - 1, ModifiedTimeValid: true},
		}
		testseam.Swap(t, &deps, &Deps{Out: &Formatter{w: io.Discard}})
		if err := printDrivePullDryRunWithPreflight(root, ifExistsSkip, remote, []string{"skip.txt"}, false, nil); err != nil {
			t.Fatal(err)
		}
		if err := printDrivePullDryRunWithPreflight(root, ifExistsSmart, remote, []string{"smart.txt"}, false, nil); err != nil {
			t.Fatal(err)
		}
		remote["smart.txt"] = &remoteFile{RelPath: "smart.txt"}
		if err := printDrivePullDryRunWithPreflight(root, ifExistsSmart, remote, []string{"smart.txt"}, false, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("pull mkdir and replace errors", func(t *testing.T) {
		root := t.TempDir()
		caller := pullListingCaller("")
		testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
		testseam.Swap(t, &os.Args, []string{"dws", "drive"})
		testseam.Swap(t, &pullMkdirAll, func(string, os.FileMode) error { return errTestDownload })
		if action, err := pullOneFile(context.Background(), "", &remoteFile{FileID: "F"},
			filepath.Join(root, "blocked", "x.txt"), ifExistsOverwrite); action != pullActionFailed || err == nil {
			t.Fatalf("mkdir failure = (%q, %v)", action, err)
		}

		targetDir := t.TempDir()
		mustWrite(t, filepath.Join(targetDir, "target.txt"), "local")
		testseam.Swap(t, &pullMkdirAll, os.MkdirAll)
		testseam.Swap(t, &pullRename, func(*os.Root, string, string) error { return errTestDownload })
		swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
			return os.WriteFile(dest, []byte("remote"), 0o644)
		})
		if action, err := pullOneFile(context.Background(), "", &remoteFile{FileID: "F"},
			filepath.Join(targetDir, "target.txt"), ifExistsOverwrite); action != pullActionFailed || err == nil {
			t.Fatalf("replace failure = (%q, %v)", action, err)
		}
	})

	t.Run("sync pull defensive failures", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		res := &driveSyncResult{}
		syncPullFile(res, context.Background(), "", &remoteFile{FileID: "F"}, root, "sub/a.txt", syncDirectionPull)
		if res.Summary.Failed != 1 {
			t.Fatalf("escape failed = %d", res.Summary.Failed)
		}
	})

	t.Run("pull remote file rechecks symlink escape", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}
		action, err := pullRemoteFile(context.Background(), "", &remoteFile{FileID: "F"}, root,
			"sub/a.txt", ifExistsOverwrite)
		if action != pullActionFailed || err == nil {
			t.Fatalf("pull remote TOCTOU guard = (%q, %v)", action, err)
		}
	})
}

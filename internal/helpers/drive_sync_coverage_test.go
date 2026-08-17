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

// syncCaller 组合 push 与 pull 两侧：list_files 按 parentId 返回远端现状，
// create_folder / get_upload_info / ln 走 push 路径，download_file 走 pull 路径。
func syncCaller(listing map[string]string) *driveScriptCaller {
	return &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
		switch tool {
		case "list_files":
			parent, _ := args["parentId"].(string)
			if body, ok := listing[parent]; ok {
				return body, nil
			}
			return `{"result":{"items":[],"nextToken":""}}`, nil
		case "create_folder":
			return `{"result":{"fileId":"NEWDIR"},"success":true}`, nil
		case "get_upload_info":
			return `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`, nil
		case "download_file":
			return `{"result":{"downloadUrl":"https://oss.example.com/get","headers":{}}}`, nil
		}
		return `{"result":{"fileId":"NEWFILE"},"success":true}`, nil
	}}
}

// withSyncTransport 注入成功的上传与下载。
func withSyncTransport(t *testing.T, body string) {
	t.Helper()
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return nil })
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte(body), 0o644)
	})
}

// ──────────────────────────────────────────────────────────
// --on-conflict 取值处理
// ──────────────────────────────────────────────────────────

// 显式传空 --on-conflict 时回落安全默认 skip：两侧都变更时两边内容都不动。
func TestCrossPlatformCoverageDriveSync_emptyOnConflictDefaultsToSkip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-old")
	local := readFileMTimeMillis(t, p)
	withSyncTransport(t, "remote-new")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT",
		"--quick", "--on-conflict", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "local-old" {
		t.Errorf("empty --on-conflict must default to skip and keep the local file, got %q", string(b))
	}
}

func TestCrossPlatformCoverageDriveSync_invalidOnConflictRejected(t *testing.T) {
	err := runDriveCmd(t, syncCaller(nil), "sync", "--local-folder", t.TempDir(),
		"--remote-folder", "ROOT", "--on-conflict", "bogus")
	if err == nil || !strings.Contains(err.Error(), "--on-conflict") {
		t.Fatalf("expected --on-conflict rejection, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// unchanged / unknown 分类
// ──────────────────────────────────────────────────────────

// quick 模式下两侧 mtime 相同 → unchanged，不做任何读写。
func TestCrossPlatformCoverageDriveSync_unchangedIsUntouched(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "same.txt")
	mustWrite(t, p, "identical")
	local := readFileMTimeMillis(t, p)
	withSyncTransport(t, "should-not-be-used")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"same.txt","type":"file","fileId":"S","modifyTime":` +
			strconv.FormatInt(local, 10) + `}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(caller.callsFor("download_file")) + len(caller.callsFor("get_upload_info")); got != 0 {
		t.Errorf("unchanged file must not be transferred, got %d write calls", got)
	}
	if b, _ := os.ReadFile(p); string(b) != "identical" {
		t.Errorf("unchanged file was modified: %q", string(b))
	}
}

// exact 模式远端无可靠 md5 → unknown，一律计入 skipped 且不动任何一侧。
func TestCrossPlatformCoverageDriveSync_unknownIsSkipped(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "u.txt")
	mustWrite(t, p, "local")
	withSyncTransport(t, "remote")

	// 无 md5 字段 → exact 模式无法核对内容。
	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"u.txt","type":"file","fileId":"U","modifyTime":1}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(caller.callsFor("download_file")) + len(caller.callsFor("get_upload_info")); got != 0 {
		t.Errorf("unknown must not be transferred, got %d write calls", got)
	}
	if b, _ := os.ReadFile(p); string(b) != "local" {
		t.Errorf("unknown file was modified: %q", string(b))
	}
}

// exact 模式下本地 MD5 计算失败 → 整个 sync 中止。错误通过 seam 确定性注入，
// 避免依赖 chmod：Windows 不遵循 POSIX 的 000 权限语义。
func TestCrossPlatformCoverageDriveSync_md5FailureAborts(t *testing.T) {
	testseam.Swap(t, &pushMD5OpenedFile, func(*os.File) (string, error) {
		return "", errors.New("md5 boom")
	})

	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	mustWrite(t, p, "data")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"secret.txt","type":"file","fileId":"S","md5":"abc"}],"nextToken":""}}`,
	})
	err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "MD5") {
		t.Fatalf("expected MD5 failure to abort sync, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// new_local 推送：按需建目录
// ──────────────────────────────────────────────────────────

// 本地子目录在远端缺失 → 建目录（记 folder_created + pushed）再上传其中文件。
func TestCrossPlatformCoverageDriveSync_createsRemoteFolderForNewLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "deep.txt"), "d")
	withSyncTransport(t, "unused")

	caller := syncCaller(nil)
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf := caller.callsFor("create_folder")
	if len(cf) != 1 || cf[0].args["name"] != "sub" {
		t.Fatalf("expected create_folder(sub), got %v", cf)
	}
	up := caller.callsFor("get_upload_info")
	if len(up) != 1 || up[0].args["parentId"] != "NEWDIR" {
		t.Errorf("upload parentId = %v, want NEWDIR", up[0].args["parentId"])
	}
}

// 远端已有同名目录 → 复用，不重建。
func TestCrossPlatformCoverageDriveSync_reusesExistingRemoteFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "deep.txt"), "d")
	withSyncTransport(t, "unused")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"sub","type":"folder","fileId":"EXISTING"}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := caller.callsFor("create_folder"); len(got) != 0 {
		t.Errorf("existing folder must be reused, got %v", got)
	}
}

// create_folder 失败 → 目录与其中文件都记 failed，命令非零退出。
func TestCrossPlatformCoverageDriveSync_folderCreationFailureCascades(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "deep.txt"), "d")
	withSyncTransport(t, "unused")

	caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
		if tool == "create_folder" {
			return "", errors.New("create_folder boom")
		}
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}
	err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick")
	var sf *driveSyncFailure
	if !errors.As(err, &sf) || sf.failed != 2 {
		t.Fatalf("expected 2 failures (folder + orphaned file), got %T %v", err, err)
	}
}

// new_local 上传失败 → 记 failed 并非零退出。
func TestCrossPlatformCoverageDriveSync_pushUploadFailureIsFailed(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "only-local.txt"), "x")
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return errTestUpload })

	caller := syncCaller(nil)
	err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick")
	var sf *driveSyncFailure
	if !errors.As(err, &sf) || sf.failed != 1 {
		t.Fatalf("expected 1 push failure, got %T %v", err, err)
	}
}

// ──────────────────────────────────────────────────────────
// --on-conflict ask 的每个交互答案
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveSyncAskConflict_answers(t *testing.T) {
	cases := []struct {
		input string
		want  string
		isErr bool
	}{
		{"r\n", syncConflictRemoteWins, false},
		{"remote\n", syncConflictRemoteWins, false},
		{"l\n", syncConflictLocalWins, false},
		{"local-wins\n", syncConflictLocalWins, false},
		{"k\n", syncConflictKeepBoth, false},
		{"keep\n", syncConflictKeepBoth, false},
		{"s\n", "", false},
		{"skip\n", "", false},
		{"\n", syncConflictRemoteWins, false}, // 交互式回车 → 默认远端优先
		{"nonsense\n", "", true},
	}
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.input), func(t *testing.T) {
			testseam.Swap(t, &syncAskStdin, io.Reader(strings.NewReader(tc.input)))

			got, err := driveSyncAskConflict("f.txt")
			if tc.isErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("driveSyncAskConflict(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ask 选到无效值时整个 sync 中止（错误上抛，不是静默跳过）。
func TestCrossPlatformCoverageDriveSync_askInvalidChoiceAborts(t *testing.T) {
	testseam.Swap(t, &syncAskStdin, io.Reader(strings.NewReader("nonsense\n")))

	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local")
	local := readFileMTimeMillis(t, p)
	withSyncTransport(t, "remote")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"F","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT",
		"--quick", "--on-conflict", "ask"); err == nil {
		t.Fatal("expected invalid ask choice to abort sync")
	}
}

// ask 选 keep-both：本地改名保留副本，远端拉到原名。
func TestCrossPlatformCoverageDriveSync_askKeepBothViaStdin(t *testing.T) {
	testseam.Swap(t, &syncAskStdin, io.Reader(strings.NewReader("k\n")))

	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-version")
	local := readFileMTimeMillis(t, p)
	withSyncTransport(t, "remote-version")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"FID12345678","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	})
	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT",
		"--quick", "--on-conflict", "ask"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "remote-version" {
		t.Errorf("original path must hold the remote version, got %q", string(b))
	}
	// 本地副本以 .conflict-<fileId 末 8 位> 保留。
	entries, _ := os.ReadDir(dir)
	var found bool
	for _, e := range entries {
		if strings.Contains(e.Name(), ".conflict-") {
			found = true
			if b, _ := os.ReadFile(filepath.Join(dir, e.Name())); string(b) != "local-version" {
				t.Errorf("conflict copy content = %q, want local-version", string(b))
			}
		}
	}
	if !found {
		t.Errorf("no .conflict-* copy kept, dir = %v", entries)
	}
}

// keep-both 改名失败（目标被目录占位）→ 记 failed，本地原文件保持不动。
func TestCrossPlatformCoverageDriveSync_keepBothRenameFailureKeepsLocal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	mustWrite(t, p, "local-version")
	local := readFileMTimeMillis(t, p)
	withSyncTransport(t, "remote-version")

	// 让候选改名目标 f.conflict-FID12345.txt 被一个目录占住 → O_EXCL 建占位失败。
	if err := os.MkdirAll(filepath.Join(dir, "f.conflict-ID123456.txt"), 0o755); err != nil {
		t.Fatal(err)
	}

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"f.txt","type":"file","fileId":"FID123456","modifyTime":` +
			differentMillis(local) + `}],"nextToken":""}}`,
	})
	// 无论走到哪个失败分支，本地原文件都不能被破坏。
	_ = runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT",
		"--quick", "--on-conflict", "keep-both")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("local file must still exist: %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// dry-run 与 unknown 组合
// ──────────────────────────────────────────────────────────

// dry-run 下即使有 new_local / new_remote / modified 也不产生任何写调用。
func TestCrossPlatformCoverageDriveSync_dryRunSkipsEveryWrite(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "local.txt"), "l")
	p := filepath.Join(dir, "both.txt")
	mustWrite(t, p, "local")
	local := readFileMTimeMillis(t, p)
	withSyncTransport(t, "remote")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[` +
			`{"name":"both.txt","type":"file","fileId":"B","modifyTime":` + differentMillis(local) + `},` +
			`{"name":"remote.txt","type":"file","fileId":"R","modifyTime":2000}` +
			`],"nextToken":""}}`,
	})
	caller.dryRun = true

	if err := runDriveCmd(t, caller, "sync", "--local-folder", dir, "--remote-folder", "ROOT", "--quick"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(caller.callsFor("download_file")) + len(caller.callsFor("get_upload_info")) +
		len(caller.callsFor("create_folder")); got != 0 {
		t.Errorf("dry-run must not write, got %d calls", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "remote.txt")); !os.IsNotExist(err) {
		t.Error("dry-run must not materialize remote files locally")
	}
}

// 远端列举失败 → sync 直接报错。
func TestCrossPlatformCoverageDriveSync_remoteListFailurePropagates(t *testing.T) {
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) { return "", errTestList }}
	if err := runDriveCmd(t, caller, "sync", "--local-folder", t.TempDir(), "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected remote listing failure to propagate")
	}
}

// 本地根目录不存在 → 扫描失败上抛。
func TestCrossPlatformCoverageDriveSync_missingLocalRootFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if err := runDriveCmd(t, syncCaller(nil), "sync", "--local-folder", missing, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected missing local root to fail")
	}
}

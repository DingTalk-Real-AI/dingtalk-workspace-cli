package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// driveScriptCaller — 可编程 MCP mock：按 (toolName, 第几次调用) 决定返回，
// 覆盖分页、递归、错误分支等 walkRemoteDir / runDriveStatus 路径。
// ──────────────────────────────────────────────────────────

type driveScriptCaller struct {
	calls  []driveMCPCall
	reply  func(toolName string, args map[string]any, nth int) (string, error)
	dryRun bool
}

func (m *driveScriptCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	nth := 0
	for _, c := range m.calls {
		if c.toolName == toolName {
			nth++
		}
	}
	m.calls = append(m.calls, driveMCPCall{toolName: toolName, args: args})
	text, err := m.reply(toolName, args, nth)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (m *driveScriptCaller) CallReadTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	return m.CallTool(ctx, productID, toolName, args)
}

func (m *driveScriptCaller) Format() string { return "raw" }
func (m *driveScriptCaller) DryRun() bool   { return m.dryRun }
func (m *driveScriptCaller) Fields() string { return "" }
func (m *driveScriptCaller) JQ() string     { return "" }

func (m *driveScriptCaller) callsFor(tool string) []driveMCPCall {
	var out []driveMCPCall
	for _, c := range m.calls {
		if c.toolName == tool {
			out = append(out, c)
		}
	}
	return out
}

var errTestList = errors.New("list_files boom")

// runDriveCmd 以给定 caller 执行 `dws drive <args...>`。
//
// pull / push / sync 声明了 Safety.Confirmation = user_required，非交互环境下必须带
// --yes 才能越过统一确认门；这里自动补上，让用例专注被测行为。要断言「未确认即拒绝」
// 请改用 runDriveCmdWithoutConfirm。
func runDriveCmd(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	switch {
	case len(args) == 0:
	case args[0] == "pull", args[0] == "push", args[0] == "sync":
		args = append(args, "--yes")
	}
	return runDriveCmdWithoutConfirm(t, caller, args...)
}

// runDriveCmdWithoutConfirm 原样执行，不补 --yes，用于验证确认门本身。
func runDriveCmdWithoutConfirm(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()

	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().BoolP("yes", "y", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(newDriveCommand())

	testseam.Swap(t, &os.Args, append([]string{"dws", "drive"}, args...))
	root.SetArgs(append([]string{"drive"}, args...))
	return root.Execute()
}

// ──────────────────────────────────────────────────────────
// runDriveStatus — 端到端：本地 + 远端树 → diff
// ──────────────────────────────────────────────────────────

// 端到端 status：递归进入子文件夹、拼接 rel_path、跳过非 file 类型。
func TestCrossPlatformCoverageDriveStatus_endToEndNestedTree(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "root.txt"), "r")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "sub", "nested.txt"), "n")

	caller := &driveScriptCaller{reply: func(_ string, args map[string]any, _ int) (string, error) {
		switch args["parentId"] {
		case "ROOT":
			return `{"result":{"items":[` +
				`{"name":"root.txt","type":"file","fileId":"R1","modifyTime":1000},` +
				`{"name":"sub","type":"folder","fileId":"SUB"},` +
				`{"name":"online.adoc","type":"document","fileId":"D1"}` +
				`],"nextToken":""}}`, nil
		case "SUB":
			return `{"result":{"items":[{"name":"nested.txt","type":"file","fileId":"N1","modifyTime":2000}],"nextToken":""}}`, nil
		}
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}

	if err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT", "--quick"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 递归进入 sub 一次；不属于文件夹镜像范围的在线文档不产生额外调用。
	if got := len(caller.callsFor("list_files")); got != 2 {
		t.Errorf("expected 2 list_files calls (ROOT + SUB), got %d", got)
	}
}

// 分页 token 未推进意味着远端清单可能被截断，必须失败而不是把当前页当作 EOF。
func TestCrossPlatformCoverageDriveStatus_stalledPaginationTokenFailsClosed(t *testing.T) {
	dir := t.TempDir()

	caller := &driveScriptCaller{reply: func(_ string, args map[string]any, nth int) (string, error) {
		switch nth {
		case 0:
			if _, ok := args["nextToken"]; ok {
				t.Error("first page must not carry nextToken")
			}
			return `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}],"nextToken":"T1"}}`, nil
		case 1:
			if args["nextToken"] != "T1" {
				t.Errorf("second page nextToken = %v, want T1", args["nextToken"])
			}
			// 返回同一个 token → 未推进，必须 fail closed。
			return `{"result":{"items":[{"name":"b.txt","type":"file","fileId":"B"}],"nextToken":"T1"}}`, nil
		}
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}

	err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT", "--quick")
	if err == nil || !strings.Contains(err.Error(), "分页 token") {
		t.Fatalf("error = %v, want stalled pagination failure", err)
	}
	if got := len(caller.callsFor("list_files")); got != 2 {
		t.Fatalf("stalled token must fail after 2 calls, got %d", got)
	}
}

// space-id 显式传入时必须透传到 list_files。
func TestCrossPlatformCoverageDriveStatus_passesSpaceID(t *testing.T) {
	dir := t.TempDir()
	caller := &driveScriptCaller{reply: func(_ string, _ map[string]any, _ int) (string, error) {
		return `{"result":{"items":[],"nextToken":""}}`, nil
	}}
	if err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT", "--space-id", "SP1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := caller.callsFor("list_files")[0].args["spaceId"]; got != "SP1" {
		t.Errorf("spaceId = %v, want SP1", got)
	}
}

// list_files 失败 → status 直接报错。
func TestCrossPlatformCoverageDriveStatus_listFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) { return "", errTestList }}
	if err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected list_files error to propagate")
	}
}

// 返回体无法解析 → status 报解析失败。
func TestCrossPlatformCoverageDriveStatus_unparsableListPropagates(t *testing.T) {
	dir := t.TempDir()
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) {
		return `{"result":"not-a-list"}`, nil
	}}
	err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "list_files") {
		t.Fatalf("expected list_files parse error, got %v", err)
	}
}

// exact 模式下本地文件不可读 → MD5 计算失败要作为错误上抛，不能静默当成一致。
func TestCrossPlatformCoverageDriveStatus_unreadableLocalFileFailsExactCompare(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	mustWrite(t, p, "data")
	testseam.Swap(t, &md5File, func(string) (string, error) {
		return "", errors.New("MD5 boom")
	})

	caller := &driveScriptCaller{reply: func(_ string, _ map[string]any, _ int) (string, error) {
		return `{"result":{"items":[{"name":"secret.txt","type":"file","fileId":"S","md5":"abc"}],"nextToken":""}}`, nil
	}}
	err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "MD5") {
		t.Fatalf("expected MD5 failure, got %v", err)
	}
}

func TestCrossPlatformCoverageDriveStatus_walkLocalFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	testseam.Swap(t, &runDriveStatusWalkLocalTree, func(string) (map[string]*localFile, error) {
		return nil, errors.New("walk boom")
	})
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) {
		t.Error("local scan failure must abort before MCP calls")
		return "", nil
	}}
	err := runDriveCmd(t, caller, "status", "--local-folder", dir, "--remote-folder", "ROOT")
	if err == nil || !strings.Contains(err.Error(), "扫描本地目录失败") {
		t.Fatalf("expected local scan failure, got %v", err)
	}
}

// --local-folder 不是绝对路径 → 校验拦下，不发起任何 MCP 调用。
func TestCrossPlatformCoverageDriveStatus_relativeLocalFolderRejected(t *testing.T) {
	caller := &driveScriptCaller{reply: func(string, map[string]any, int) (string, error) {
		t.Error("must not call MCP when --local-folder is rejected")
		return "", nil
	}}
	if err := runDriveCmd(t, caller, "status", "--local-folder", "relative/path", "--remote-folder", "ROOT"); err == nil {
		t.Fatal("expected relative --local-folder to be rejected")
	}
}

// ──────────────────────────────────────────────────────────
// walkRemoteDir — 深度上限
// ──────────────────────────────────────────────────────────

// 远端目录自引用时深度上限必须中止遍历，避免无限递归。
func TestCrossPlatformCoverageWalkRemoteDir_depthLimitAborts(t *testing.T) {
	caller := &driveScriptCaller{reply: func(_ string, _ map[string]any, _ int) (string, error) {
		// 每层都返回一个同名子文件夹 → 无限深。
		return `{"result":{"items":[{"name":"loop","type":"folder","fileId":"LOOP"}],"nextToken":""}}`, nil
	}}
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"}) // 工具路由需要命令上下文

	_, err := fetchRemoteDriveTree(context.Background(), "", "ROOT", true)
	if err == nil || !strings.Contains(err.Error(), "循环引用") {
		t.Fatalf("expected depth-limit abort, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────
// isSafeRemoteSegment / resolveLocalTarget 的剩余分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageIsSafeRemoteSegment_volumeAndNonCanonical(t *testing.T) {
	if runtime.GOOS == "windows" {
		if isSafeRemoteSegment(`C:name.txt`) {
			t.Error("volume-qualified name must be rejected")
		}
	}
	// 非规范形式（Clean 后不等于自身）必须拒绝。
	for _, name := range []string{"./a.txt", "a/../b.txt", "a//b.txt"} {
		if isSafeRemoteSegment(name) {
			t.Errorf("non-canonical %q must be rejected", name)
		}
	}
}

// ──────────────────────────────────────────────────────────
// md5File / walkLocalTree 错误分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageMD5File_missingFile(t *testing.T) {
	if _, err := md5File(filepath.Join(t.TempDir(), "nope.bin")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestCrossPlatformCoverageMD5File_directoryIsNotReadable(t *testing.T) {
	// 目录不是常规文件：io.Copy 读取时必然报错。
	if _, err := md5File(t.TempDir()); err == nil {
		t.Fatal("expected error when hashing a directory")
	}
}

func TestCrossPlatformCoverageWalkLocalTree_missingRootErrors(t *testing.T) {
	if _, err := walkLocalTree(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected error for missing local root")
	}
}

// 根路径是指向目录的符号链接时，filepath.WalkDir 只把根本身当成一次 !IsRegular
// 回调静默跳过，结果是空索引，而 status 会把所有远端文件误报为 new_remote。
// 必须在遍历前 fail-closed。这里通过 seam 注入，兼顾无法真实创建 symlink 的平台。
func TestCrossPlatformCoverageWalkLocalTree_rejectsSymlinkRootViaSeam(t *testing.T) {
	testseam.Swap(t, &statusRootLstat, func(string) (os.FileInfo, error) {
		return fakeSymlinkFileInfo{}, nil
	})
	if _, err := walkLocalTree(t.TempDir()); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("symlink root must fail closed, got %v", err)
	}
}

// 真实 symlink 复现：非 Windows 平台可以直接创建，进一步保证 seam 与真实语义一致。
func TestCrossPlatformCoverageWalkLocalTree_rejectsSymlinkRootReal(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := walkLocalTree(link); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("symlink root must fail closed, got %v", err)
	}
}

// 根类型校验通过后 WalkDir 内部返回的错误必须原样上抛，不能被吞掉；根消失或
// 权限变更之类的情形在 Linux/Windows runner 上都难以稳定复现，用 seam 注入。
func TestCrossPlatformCoverageWalkLocalTree_propagatesWalkDirError(t *testing.T) {
	testseam.Swap(t, &statusWalkDir, func(string, fs.WalkDirFunc) error {
		return errWalkDirSentinel
	})
	dir := t.TempDir()
	if _, err := walkLocalTree(dir); !errors.Is(err, errWalkDirSentinel) {
		t.Fatalf("walk dir error must be surfaced, got %v", err)
	}
}

var errWalkDirSentinel = errors.New("simulated walk dir failure")

// 根路径既非目录也非符号链接（例如普通文件被误传）时，也必须报错而非空索引。
func TestCrossPlatformCoverageWalkLocalTree_rejectsNonDirectoryRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	mustWrite(t, file, "x")
	if _, err := walkLocalTree(file); err == nil || !strings.Contains(err.Error(), "不是目录") {
		t.Fatalf("file root must fail closed, got %v", err)
	}
}

// fakeSymlinkFileInfo 是一个 Mode()&os.ModeSymlink != 0 的 os.FileInfo。Windows
// 上创建目录符号链接需要管理员权限，测试通过它绕过创建步骤。
type fakeSymlinkFileInfo struct{}

func (fakeSymlinkFileInfo) Name() string       { return "root-link" }
func (fakeSymlinkFileInfo) Size() int64        { return 0 }
func (fakeSymlinkFileInfo) Mode() os.FileMode  { return os.ModeSymlink | 0o755 }
func (fakeSymlinkFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeSymlinkFileInfo) IsDir() bool        { return false }
func (fakeSymlinkFileInfo) Sys() any           { return nil }

// 非常规文件（FIFO/符号链接目标缺失等）不计入本地索引。
func TestCrossPlatformCoverageWalkLocalTree_skipsIrregularEntries(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "real.txt"), "x")
	if err := os.Symlink(filepath.Join(dir, "absent"), filepath.Join(dir, "dangling")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	got, err := walkLocalTree(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["dangling"]; ok {
		t.Error("dangling symlink must not be indexed")
	}
	if _, ok := got["real.txt"]; !ok {
		t.Error("regular file must be indexed")
	}
}

// ──────────────────────────────────────────────────────────
// toMillis 的 json.Number / float64 分支
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDriveItem_modifiedMillisFromNumericForms(t *testing.T) {
	if got, ok := (driveItem{"modifyTime": float64(1500)}).modifiedMillis(); !ok || got != 1500 {
		t.Errorf("float64 modifyTime = (%d,%v), want (1500,true)", got, ok)
	}
	if got, ok := (driveItem{"modifyTime": json.Number("2500")}).modifiedMillis(); !ok || got != 2500 {
		t.Errorf("json.Number modifyTime = (%d,%v), want (2500,true)", got, ok)
	}
	if _, ok := (driveItem{"modifyTime": json.Number("nope")}).modifiedMillis(); ok {
		t.Error("malformed json.Number modifyTime must be invalid")
	}
	if _, ok := (driveItem{"modifyTime": float64(0)}).modifiedMillis(); ok {
		t.Error("non-positive modifyTime must be invalid")
	}
	if _, ok := (driveItem{}).modifiedMillis(); ok {
		t.Error("missing modifyTime must be invalid")
	}
}

// fileSize 的 float64 / json.Number 两种形态，以及缺失时的兜底。
func TestCrossPlatformCoverageDriveItem_sizeForms(t *testing.T) {
	if got := (driveItem{"fileSize": float64(42)}).size(); got != 42 {
		t.Errorf("float fileSize = %d, want 42", got)
	}
	if got := (driveItem{"fileSize": json.Number("77")}).size(); got != 77 {
		t.Errorf("json.Number fileSize = %d, want 77", got)
	}
	if got := (driveItem{"fileSize": json.Number("not-a-number")}).size(); got != 0 {
		t.Errorf("malformed json.Number fileSize = %d, want 0", got)
	}
	if got := (driveItem{}).size(); got != 0 {
		t.Errorf("missing fileSize = %d, want 0", got)
	}
}

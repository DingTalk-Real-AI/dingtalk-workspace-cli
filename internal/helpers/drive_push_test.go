package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// splitRel — rel_path 拆父路径 / 末段名
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageSplitRel(t *testing.T) {
	cases := []struct {
		in           string
		parent, base string
	}{
		{"a/b/c", "a/b", "c"},
		{"a/b", "a", "b"},
		{"c", "", "c"},
		{"", "", ""},
	}
	for _, c := range cases {
		p, b := splitRel(c.in)
		if p != c.parent || b != c.base {
			t.Errorf("splitRel(%q) = (%q,%q), want (%q,%q)", c.in, p, b, c.parent, c.base)
		}
	}
}

// ──────────────────────────────────────────────────────────
// parseNodeID — create_folder / commit 返回里抽 fileId
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageParseNodeID(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"result.fileId", `{"result":{"fileId":"F1","name":"x"},"success":true}`, "F1"},
		{"top-level fileId", `{"fileId":"F2"}`, "F2"},
		{"dentryUuid fallback", `{"result":{"dentryUuid":"D1"}}`, "D1"},
		{"dentryId fallback", `{"result":{"dentryId":"D2"}}`, "D2"},
		{"missing", `{"result":{"name":"x"}}`, ""},
		{"invalid json", `not json`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseNodeID(c.text); got != c.want {
				t.Errorf("parseNodeID = %q, want %q", got, c.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────
// walkLocalForPush — 本地遍历（含空目录，父目录先于子目录）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageWalkLocalForPush(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")        // 5 bytes
	mustWrite(t, filepath.Join(root, "sub1", "b.txt"), "worl") // 4 bytes, 建出 sub1
	if err := os.MkdirAll(filepath.Join(root, "sub1", "sub2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, files, err := walkLocalForPush(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 目录：含空目录，不含根本身，且父目录先于子目录（字典序即可保证）。
	wantDirs := []string{"empty", "sub1", "sub1/sub2"}
	if !sort.StringsAreSorted(dirs) {
		t.Errorf("dirs should be sorted (parent before child): %v", dirs)
	}
	if !equalStrings(dirs, wantDirs) {
		t.Errorf("dirs = %v, want %v", dirs, wantDirs)
	}

	// 文件：rel_path 用 /，记录 size 与 mtime。
	byRel := map[string]localPushFile{}
	for _, f := range files {
		byRel[f.RelPath] = f
	}
	if len(byRel) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(byRel), files)
	}
	a, ok := byRel["a.txt"]
	if !ok || a.Size != 5 {
		t.Errorf("a.txt size = %d, want 5 (present=%v)", a.Size, ok)
	}
	if a.AbsPath == "" || a.ModTimeMillis == 0 {
		t.Error("file should record AbsPath and mtime")
	}
	b, ok := byRel["sub1/b.txt"]
	if !ok || b.Size != 4 {
		t.Errorf("sub1/b.txt size = %d, want 4 (present=%v)", b.Size, ok)
	}
}

// ──────────────────────────────────────────────────────────
// drivePushFailure — push 部分失败错误：exit=1
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDrivePushFailure(t *testing.T) {
	e := &drivePushFailure{failed: 3}
	if e.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", e.ExitCode())
	}
	if e.Error() != e.RawStderr() {
		t.Error("Error() 与 RawStderr() 应一致")
	}
	if !strings.Contains(e.Error(), "3") {
		t.Errorf("Error() 应包含失败数, got %q", e.Error())
	}
}

// ──────────────────────────────────────────────────────────
// push 覆盖流程 — get_upload_info / commit_upload 两阶段的 MCP 参数
// ──────────────────────────────────────────────────────────

// driveMCPCall 记录一次 MCP 工具调用。
type driveMCPCall struct {
	toolName string
	args     map[string]any
}

// driveSyncMockCaller 按工具名分发返回，并记录全部调用，供断言 MCP 参数。
type driveSyncMockCaller struct {
	calls      []driveMCPCall
	listJSON   string  // list_files 返回体
	commitJSON *string // 非 nil 时覆盖 commit_upload 返回体（包括空响应）
	dryRun     bool    // --dry-run：只算差异，不执行任何写操作
}

func (m *driveSyncMockCaller) CallTool(_ context.Context, _ string, toolName string, args map[string]any) (*edition.ToolResult, error) {
	m.calls = append(m.calls, driveMCPCall{toolName: toolName, args: args})
	var text string
	switch toolName {
	case "list_files":
		text = m.listJSON
	case "get_upload_info":
		// 返回可被 parseDriveUploadInfo 解析的凭证。
		text = `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`
	case "download_file":
		// 返回可被 parseDriveDownloadInfo 解析的预签名下载链接。
		text = `{"result":{"downloadUrl":"https://oss.example.com/get","headers":{}}}`
	case "commit_upload":
		if m.commitJSON != nil {
			text = *m.commitJSON
		} else {
			text = `{"result":{"fileId":"NEW_FID"},"success":true}`
		}
	default:
		text = `{"result":{"fileId":"NEW_FID"},"success":true}`
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}

func (m *driveSyncMockCaller) CallReadTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	return m.CallTool(ctx, productID, toolName, args)
}

func (m *driveSyncMockCaller) Format() string { return "raw" }
func (m *driveSyncMockCaller) DryRun() bool   { return m.dryRun }
func (m *driveSyncMockCaller) Fields() string { return "" }
func (m *driveSyncMockCaller) JQ() string     { return "" }

func (m *driveSyncMockCaller) callsFor(tool string) []driveMCPCall {
	var out []driveMCPCall
	for _, c := range m.calls {
		if c.toolName == tool {
			out = append(out, c)
		}
	}
	return out
}

// runDrivePushTest 用 mock caller 执行 `drive push`，返回捕获的调用。
// 注入已打开文件句柄的 PUT seam 为 no-op，避免真实 OSS 上传。
func runDrivePushTest(t *testing.T, caller *driveSyncMockCaller, localDir string, args ...string) error {
	t.Helper()

	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return nil })
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().BoolP("yes", "y", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(newDriveCommand())

	// pull/push 是 user_required 叶子，非交互环境需 --yes 才能越过统一确认门。
	full := append([]string{"push", "--local-folder", localDir, "--remote-folder", "ROOT", "--yes"}, args...)
	testseam.Swap(t, &os.Args, append([]string{"dws", "drive"}, full...))
	root.SetArgs(append([]string{"drive"}, full...))

	return root.Execute()
}

func runDriveMirrorUploadCommandCapture(t *testing.T, command string, caller *driveSyncMockCaller, localDir string) (error, string, int) {
	t.Helper()
	putCalls := 0
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
		putCalls++
		return nil
	})
	var out bytes.Buffer
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: &out}})

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().BoolP("yes", "y", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(newDriveCommand())
	full := []string{command, "--local-folder", localDir, "--remote-folder", "ROOT", "--yes"}
	testseam.Swap(t, &os.Args, append([]string{"dws", "drive"}, full...))
	root.SetArgs(append([]string{"drive"}, full...))
	return root.Execute(), out.String(), putCalls
}

func assertMirrorUploadFailedJSON(t *testing.T, output string) {
	t.Helper()
	var payload struct {
		Summary struct {
			Failed int `json:"failed"`
		} `json:"summary"`
		Items []struct {
			Action string `json:"action"`
			Error  string `json:"error"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output is not structured JSON: %q: %v", output, err)
	}
	if payload.Summary.Failed != 1 || len(payload.Items) != 1 || payload.Items[0].Action != pushActionFailed || payload.Items[0].Error == "" {
		t.Fatalf("structured failure=%#v", payload)
	}
}

// 覆盖分支：get_upload_info 与 commit_upload 两阶段都必须传 overwriteFileId、都不传 parentId。
func TestCrossPlatformCoverageDrivePush_overwriteUsesOverwriteFileId(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "new-content")

	caller := &driveSyncMockCaller{
		// 远端根目录下已存在同名 a.txt，fileId=REMOTE_FID。
		listJSON: `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"REMOTE_FID","md5":"x","modifyTime":1000}],"nextToken":""}}`,
	}
	if err := runDrivePushTest(t, caller, dir, "--if-exists", "overwrite"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	up := caller.callsFor("get_upload_info")
	if len(up) != 1 {
		t.Fatalf("expected 1 get_upload_info call, got %d", len(up))
	}
	if got := up[0].args["overwriteFileId"]; got != "REMOTE_FID" {
		t.Errorf("get_upload_info overwriteFileId = %v, want REMOTE_FID", got)
	}
	if _, ok := up[0].args["parentId"]; ok {
		t.Error("覆盖时 get_upload_info 不应传 parentId")
	}

	commit := caller.callsFor("commit_upload")
	if len(commit) != 1 {
		t.Fatalf("expected 1 commit_upload call, got %d", len(commit))
	}
	if got := commit[0].args["overwriteFileId"]; got != "REMOTE_FID" {
		t.Errorf("commit_upload overwriteFileId = %v, want REMOTE_FID", got)
	}
	if _, ok := commit[0].args["parentId"]; ok {
		t.Error("覆盖时 commit_upload 不应传 parentId")
	}
}

// 新建分支：远端不存在同名文件时走普通上传——两阶段都传 parentId、都不传 overwriteFileId。
func TestCrossPlatformCoverageDrivePush_newUploadUsesParentId(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "b.txt"), "brand-new")

	caller := &driveSyncMockCaller{
		listJSON: `{"result":{"items":[],"nextToken":""}}`, // 远端空
	}
	if err := runDrivePushTest(t, caller, dir, "--if-exists", "overwrite"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	up := caller.callsFor("get_upload_info")
	if len(up) != 1 {
		t.Fatalf("expected 1 get_upload_info call, got %d", len(up))
	}
	if _, ok := up[0].args["overwriteFileId"]; ok {
		t.Error("新建上传不应传 overwriteFileId")
	}
	if got := up[0].args["parentId"]; got != "ROOT" {
		t.Errorf("get_upload_info parentId = %v, want ROOT", got)
	}
	commit := caller.callsFor("commit_upload")
	if len(commit) != 1 || commit[0].args["parentId"] != "ROOT" {
		t.Errorf("commit_upload should carry parentId=ROOT, got %v", commit)
	}
}

func TestCrossPlatformCoverageDrivePushRejectsUnknownCommitEffect(t *testing.T) {
	for _, response := range []string{"", `{}`} {
		t.Run(fmt.Sprintf("response-%q", response), func(t *testing.T) {
			dir := t.TempDir()
			mustWrite(t, filepath.Join(dir, "a.txt"), "payload")
			caller := &driveSyncMockCaller{
				listJSON:   `{"result":{"items":[]}}`,
				commitJSON: &response,
			}
			err, output, putCalls := runDriveMirrorUploadCommandCapture(t, "push", caller, dir)
			if err == nil {
				t.Fatal("empty commit result must make push exit non-zero")
			}
			if failure, ok := err.(*drivePushFailure); !ok || failure.ExitCode() != 1 {
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

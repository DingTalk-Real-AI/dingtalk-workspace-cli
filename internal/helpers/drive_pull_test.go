package helpers

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// drivePullMockCaller 按工具名分发：list_files 返回可配置的树，download_file 返回预签名 URL。
type drivePullMockCaller struct{ listJSON string }

func (m *drivePullMockCaller) CallTool(_ context.Context, _ string, tool string, _ map[string]any) (*edition.ToolResult, error) {
	text := `{"result":{"downloadUrl":"https://oss.example.com/dl"}}`
	if tool == "list_files" {
		text = m.listJSON
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}, nil
}
func (m *drivePullMockCaller) Format() string { return "raw" }
func (m *drivePullMockCaller) DryRun() bool   { return false }
func (m *drivePullMockCaller) Fields() string { return "" }
func (m *drivePullMockCaller) JQ() string     { return "" }

// ──────────────────────────────────────────────────────────
// isSafeRemoteSegment — 远端名称逐段安全校验（拒绝逃逸成分）
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageIsSafeRemoteSegment(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"report.pdf", true},
		{"报告 2024.xlsx", true},
		{"a.b.c", true},
		{"", false},
		{".", false},
		{"..", false},
		{"a/b", false},    // 正斜杠分隔符
		{`a\b`, false},    // 反斜杠分隔符（Windows）
		{"../etc", false}, // 相对上跳
		{"foo/../bar", false},
		{"/abs", false}, // 绝对路径成分
	}
	for _, c := range cases {
		if got := isSafeRemoteSegment(c.name); got != c.ok {
			t.Errorf("isSafeRemoteSegment(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}

// ──────────────────────────────────────────────────────────
// resolveLocalTarget — 拼接后必须仍位于本地根目录内
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageResolveLocalTarget_withinRoot(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	got, err := resolveLocalTarget(root, "sub/a.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "sub", "a.txt")
	if got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

// 即便 isSafeRemoteSegment 被绕过，逐段拼出的逃逸路径也必须被 resolveLocalTarget 兜住。
func TestCrossPlatformCoverageResolveLocalTarget_escapeRejected(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	for _, rel := range []string{"../evil.txt", "../../etc/passwd", "sub/../../out.txt"} {
		if _, err := resolveLocalTarget(root, rel); err == nil {
			t.Errorf("resolveLocalTarget(%q) should reject escaping path", rel)
		}
	}
}

// 根目录内的目录符号链接指向外部时，词法检查会放过（root/sub 仍在 root 下），
// 必须靠符号链接解析挡住：sub -> /outside 时 sub/a.txt 应被拒绝。
func TestCrossPlatformCoverageResolveLocalTarget_dirSymlinkEscapeRejected(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	outside := filepath.Clean(t.TempDir())

	link := filepath.Join(root, "sub")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("平台不支持创建符号链接: %v", err)
	}

	if _, err := resolveLocalTarget(root, "sub/a.txt"); err == nil {
		t.Fatal("root/sub -> 外部目录时，sub/a.txt 应被拒绝（防符号链接逃逸）")
	}

	// 对照：根目录内的真实子目录不应被误伤。
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveLocalTarget(root, "real/a.txt"); err != nil {
		t.Errorf("真实子目录不应被拒绝: %v", err)
	}
}

// 不存在的本地根目录必须放行（不误判逃逸），由后续 MkdirAll 创建。
func TestCrossPlatformCoverageResolveLocalTarget_nonexistentRootAllowed(t *testing.T) {
	parent := filepath.Clean(t.TempDir())
	absDir := filepath.Join(parent, "not", "created", "yet") // 尚不存在
	got, err := resolveLocalTarget(absDir, "sub/a.txt")
	if err != nil {
		t.Fatalf("不存在的根目录不应被误判为逃逸: %v", err)
	}
	if want := filepath.Join(absDir, "sub", "a.txt"); got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

// 回归：--local-folder 指向尚不存在的绝对路径时，pull 应自动创建目录并下载，
// 而不是把每个远端文件都误判为符号链接逃逸而 failed。
func TestCrossPlatformCoverageDrivePull_nonexistentLocalRoot(t *testing.T) {
	parent := t.TempDir()
	absDir := filepath.Join(parent, "new", "repo") // 尚不存在

	caller := &drivePullMockCaller{
		listJSON: `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"F1","modifyTime":1000}],"nextToken":""}}`,
	}

	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("REMOTE"), 0o644)
	})

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().BoolP("yes", "y", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(newDriveCommand())

	// pull/push 是 user_required 叶子，非交互环境需 --yes 才能越过统一确认门。
	full := []string{"pull", "--local-folder", absDir, "--remote-folder", "ROOT", "--yes"}
	testseam.Swap(t, &os.Args, append([]string{"dws", "drive"}, full...))
	root.SetArgs(append([]string{"drive"}, full...))

	// 全部下载成功时 runDrivePull 返回 nil；若误判逃逸则会有 failed 并返回 partial_failure。
	if err := root.Execute(); err != nil {
		t.Fatalf("pull 到不存在的本地根目录不应失败: %v", err)
	}
	got, rerr := os.ReadFile(filepath.Join(absDir, "a.txt"))
	if rerr != nil {
		t.Fatalf("目标文件应被创建: %v", rerr)
	}
	if string(got) != "REMOTE" {
		t.Errorf("目标内容 = %q, want REMOTE", got)
	}
}

// ──────────────────────────────────────────────────────────
// detectTargetCollisions — 多个远端条目映射到同一本地目标
// ──────────────────────────────────────────────────────────

// 大小写不敏感 FS：A.txt 与 a.txt 落到同一目标 → 两者都判冲突；无关文件不受影响。
func TestCrossPlatformCoverageDetectTargetCollisions_caseInsensitive(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	rels := []string{"A.txt", "a.txt", "b.txt", "sub/C.md", "sub/c.md"}
	collided := detectTargetCollisions(root, rels, true)

	for _, r := range []string{"A.txt", "a.txt", "sub/C.md", "sub/c.md"} {
		if !collided[r] {
			t.Errorf("%q 应被判为大小写冲突", r)
		}
	}
	if collided["b.txt"] {
		t.Error("b.txt 无冲突，不应被标记")
	}
}

// 大小写敏感 FS：A.txt 与 a.txt 是两个合法文件 → 不应误判冲突。
func TestCrossPlatformCoverageDetectTargetCollisions_caseSensitive(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	rels := []string{"A.txt", "a.txt", "b.txt"}
	collided := detectTargetCollisions(root, rels, false)
	if len(collided) != 0 {
		t.Errorf("大小写敏感 FS 不应有冲突, got %v", collided)
	}
}

// 平台名称规范化冲突：同名的 NFC 与 NFD 记法在不敏感 FS 上落到同一目标 → 冲突。
func TestCrossPlatformCoverageDetectTargetCollisions_unicodeNormalization(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	nfc := "café.txt"  // café（预组合 é）
	nfd := "café.txt" // café（e + 组合尖音符）
	if nfc == nfd {
		t.Fatal("测试数据应为不同字节序列")
	}

	ci := detectTargetCollisions(root, []string{nfc, nfd}, true)
	if !ci[nfc] || !ci[nfd] {
		t.Errorf("NFC/NFD 异写在大小写不敏感 FS 上应判冲突, got %v", ci)
	}
	// 大小写/规范化敏感 FS 上视为两个不同文件，不冲突。
	cs := detectTargetCollisions(root, []string{nfc, nfd}, false)
	if len(cs) != 0 {
		t.Errorf("敏感 FS 上 NFC/NFD 不应冲突, got %v", cs)
	}
}

// isCaseInsensitiveFS 应能对临时目录给出与实际探针一致的判定（本机自洽）。
func TestCrossPlatformCoverageIsCaseInsensitiveFS_selfConsistent(t *testing.T) {
	dir := t.TempDir()
	got := isCaseInsensitiveFS(dir)
	// 实测：写一个小写名，再用大写名 stat。
	lower := filepath.Join(dir, "probe_case_xyz.txt")
	if err := os.WriteFile(lower, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := os.Stat(filepath.Join(dir, "PROBE_CASE_XYZ.TXT"))
	actualInsensitive := err == nil
	if got != actualInsensitive {
		t.Errorf("isCaseInsensitiveFS=%v，但实测大写名可 stat=%v", got, actualInsensitive)
	}
}

// ──────────────────────────────────────────────────────────
// pullOneFile — 覆盖下载中断不得破坏原文件（原子替换）
// ──────────────────────────────────────────────────────────

// 下载中途失败时：命令报告 failed，原有本地文件内容必须原封不动，且不留临时残file。
func TestCrossPlatformCoveragePullOneFile_downloadFailureKeepsOriginal(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	const original = "ORIGINAL-CONTENT-DO-NOT-LOSE"
	mustWrite(t, target, original)

	// download_file 的 MCP 调用走 mock；httpGetFile 注入为“写入部分内容后报错”。
	testseam.Swap(t, &deps, &Deps{Caller: &drivePullMockCaller{}, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		// 模拟传输中断：先截断/写半截，再返回错误。
		_ = os.WriteFile(dest, []byte("HALF"), 0o644)
		return errors.New("connection reset")
	})

	rf := &remoteFile{RelPath: "a.txt", FileID: "F1"}
	action, err := pullOneFile(context.Background(), "", rf, target, ifExistsOverwrite)
	if action != pullActionFailed || err == nil {
		t.Fatalf("expected failed action with error, got action=%q err=%v", action, err)
	}

	// 原文件必须保持原内容。
	got, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatalf("原文件应仍存在: %v", rerr)
	}
	if string(got) != original {
		t.Errorf("原文件被破坏: got %q, want %q", got, original)
	}

	// 不应残留临时文件。
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != "a.txt" {
			t.Errorf("下载失败后残留了临时文件: %s", e.Name())
		}
	}
}

// 下载成功时：临时文件被原子重命名为目标，内容与远端一致。
func TestCrossPlatformCoveragePullOneFile_downloadSuccessReplacesTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	mustWrite(t, target, "OLD")

	testseam.Swap(t, &deps, &Deps{Caller: &drivePullMockCaller{}, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	swapPullDownloadPath(t, func(_ context.Context, _ string, _ map[string]string, dest string) error {
		return os.WriteFile(dest, []byte("NEW-CONTENT"), 0o644)
	})

	rf := &remoteFile{RelPath: "a.txt", FileID: "F1"}
	action, err := pullOneFile(context.Background(), "", rf, target, ifExistsOverwrite)
	if err != nil || action != pullActionDownloaded {
		t.Fatalf("expected downloaded, got action=%q err=%v", action, err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "NEW-CONTENT" {
		t.Errorf("目标未被替换为新内容: got %q", got)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Errorf("成功后目录应只有目标文件，got %d 项", len(entries))
	}
}

// ──────────────────────────────────────────────────────────
// parseDriveDownloadInfo — drive download_file 返回解析
// ──────────────────────────────────────────────────────────

// drive 的正常返回：result.downloadUrl 是预签名 URL，无需额外 header。
func TestCrossPlatformCoverageParseDriveDownloadInfo_downloadUrl(t *testing.T) {
	text := `{"result":{"downloadType":"urlPreSignature","downloadUrl":"https://oss.example.com/f.file?Expires=1&Signature=xyz","fileName":"a.txt"},"success":true}`
	url, headers, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://oss.example.com/f.file?Expires=1&Signature=xyz" {
		t.Errorf("url = %q", url)
	}
	if len(headers) != 0 {
		t.Errorf("预签名 URL 不应带 header, got %v", headers)
	}
}

// 兼容 flat resourceUrl 字段。
func TestCrossPlatformCoverageParseDriveDownloadInfo_resourceUrlFallback(t *testing.T) {
	text := `{"resourceUrl":"https://flat.example.com/dl"}`
	url, _, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://flat.example.com/dl" {
		t.Errorf("url = %q", url)
	}
}

// 兼容 resourceUrls[].url 数组格式（在 result 包裹下）。
func TestCrossPlatformCoverageParseDriveDownloadInfo_resourceUrlsArrayFallback(t *testing.T) {
	text := `{"result":{"resourceUrls":[{"url":"https://arr.example.com/dl","headers":{}}]}}`
	url, _, err := parseDriveDownloadInfo(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://arr.example.com/dl" {
		t.Errorf("url = %q", url)
	}
}

func TestCrossPlatformCoverageParseDriveDownloadInfo_missingURL(t *testing.T) {
	text := `{"result":{"fileName":"a.txt"}}`
	if _, _, err := parseDriveDownloadInfo(text); err == nil {
		t.Fatal("expected error when downloadUrl is empty")
	}
}

func TestCrossPlatformCoverageParseDriveDownloadInfo_invalidJSON(t *testing.T) {
	if _, _, err := parseDriveDownloadInfo("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ──────────────────────────────────────────────────────────
// drivePartialFailure — pull 部分失败错误：exit=1，stderr 仅保留简短说明
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDrivePartialFailure(t *testing.T) {
	e := &drivePartialFailure{failed: 2}
	if e.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", e.ExitCode())
	}
	want := "drive pull: 2 file(s) failed"
	if e.Error() != want || e.RawStderr() != want {
		t.Errorf("Error()/RawStderr() = %q / %q, want %q", e.Error(), e.RawStderr(), want)
	}
}

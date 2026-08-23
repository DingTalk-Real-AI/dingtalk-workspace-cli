package helpers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// forceCaseInsensitiveFS 在当前用例内把大小写探测钉成不敏感，使等价路径冲突分支
// 在大小写敏感的 CI 文件系统（ext4）上同样可达。
func forceCaseInsensitiveFS(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &isCaseInsensitiveFS, func(string) bool { return true })
}

// ──────────────────────────────────────────────────────────
// 等价路径冲突：pull / sync 都必须全部记 failed 且一个都不落盘
// ──────────────────────────────────────────────────────────

// 远端 A.txt 与 a.txt 在大小写不敏感目标上映射到同一本地文件 → 两项都 failed，
// 绝不顺序覆盖导致静默丢文件。
func TestCrossPlatformCoverageDrivePull_equivalentPathCollisionFailsBoth(t *testing.T) {
	forceCaseInsensitiveFS(t)
	root := t.TempDir()

	caller := pullListingCaller(
		`{"name":"A.txt","type":"file","fileId":"UP","modifyTime":1},` +
			`{"name":"a.txt","type":"file","fileId":"LOW","modifyTime":2}`)
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
		t.Error("collided entries must not be downloaded")
		return nil
	})

	err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT")
	var pf *drivePartialFailure
	if !errors.As(err, &pf) {
		t.Fatalf("expected drivePartialFailure, got %T %v", err, err)
	}
	for _, name := range []string{"A.txt", "a.txt"} {
		if _, statErr := os.Stat(filepath.Join(root, name)); !os.IsNotExist(statErr) {
			t.Errorf("%s must not be written on collision", name)
		}
	}
}

// sync 的 new_remote 阶段同样拒绝等价冲突项。
func TestCrossPlatformCoverageDriveSync_equivalentPathCollisionFailsBoth(t *testing.T) {
	forceCaseInsensitiveFS(t)
	root := t.TempDir()
	withSyncTransport(t, "remote")

	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[` +
			`{"name":"A.txt","type":"file","fileId":"UP","modifyTime":1},` +
			`{"name":"a.txt","type":"file","fileId":"LOW","modifyTime":2}` +
			`],"nextToken":""}}`,
	})
	err := runDriveCmd(t, caller, "sync", "--local-folder", root, "--remote-folder", "ROOT", "--quick")
	var sf *driveSyncFailure
	if !errors.As(err, &sf) {
		t.Fatalf("expected driveSyncFailure, got %T %v", err, err)
	}
	if sf.failed != 2 {
		t.Errorf("failed = %d, want 2 (both collided entries)", sf.failed)
	}
}

// ──────────────────────────────────────────────────────────
// detectCaseInsensitiveFS：探针名无大小写差异时回退平台默认
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageDetectCaseInsensitiveFS_caselessProbeFallsBackToPlatformDefault(t *testing.T) {
	// 纯数字模板：ToUpper 与原名相同，无法据此判定。
	testseam.Swap(t, &caseProbePattern, "1234567890*")

	dir := t.TempDir()
	got := detectCaseInsensitiveFS(dir)
	// 回退值即平台默认；这里只断言它与平台默认一致，不写死具体布尔。
	want := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	if got != want {
		t.Errorf("detectCaseInsensitiveFS = %v, want platform default %v", got, want)
	}
}

// ──────────────────────────────────────────────────────────
// isSafeRemoteSegmentPlatform：非 Windows 平台无额外约束
// ──────────────────────────────────────────────────────────

func TestCrossPlatformCoverageIsSafeRemoteSegmentPlatform_allowsPlainNames(t *testing.T) {
	for _, name := range []string{"a.txt", "报告.pdf", "with space.md"} {
		if !isSafeRemoteSegmentPlatform(name) {
			t.Errorf("isSafeRemoteSegmentPlatform(%q) = false, want true", name)
		}
	}
}

func TestCrossPlatformCoverageIsSafeRemoteSegmentPlatform_rejectsNonCanonicalWindowsName(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	for _, name := range []string{
		`C:relative`, `a?.txt`, `a*.txt`, `a:stream`, `a<.txt`, `a>.txt`, `a|.txt`, `a".txt`,
		"name.", "name ", "...", "CON", "con.txt", "PRN", "AUX", "NUL", "COM1", "LPT9", "COM¹", "CONIN$",
		// 调用方 isSafeRemoteSegment 已先过滤分隔符，所以只有直接调用才会走到
		// filepath.Clean 改写这条防御分支；平台函数本身仍必须 fail closed。
		`a/b`, `.\a`,
	} {
		if isSafeRemoteSegmentPlatform(name) {
			t.Errorf("Windows unsafe segment %q must be rejected", name)
		}
	}
	for _, name := range []string{"report.txt", "报告 2026.pdf", "COM10", "LPT0"} {
		if !isSafeRemoteSegmentPlatform(name) {
			t.Errorf("Windows ordinary segment %q must be accepted", name)
		}
	}
}

// ──────────────────────────────────────────────────────────
// WalkDir 单条目处理：Info() 与 filepath.Rel 的失败分支
// ──────────────────────────────────────────────────────────

// errDirEntry 是一个 Info() 必然报错的 fs.DirEntry。
type errDirEntry struct{ dir bool }

func (e errDirEntry) Name() string               { return "entry" }
func (e errDirEntry) IsDir() bool                { return e.dir }
func (e errDirEntry) Type() fs.FileMode          { return 0 }
func (e errDirEntry) Info() (fs.FileInfo, error) { return nil, errTestInfo }

var errTestInfo = errors.New("simulated DirEntry.Info failure")

// irregularDirEntry 的 Info() 成功，但报告为非常规文件（如设备文件）。
type irregularDirEntry struct{}

func (irregularDirEntry) Name() string               { return "dev" }
func (irregularDirEntry) IsDir() bool                { return false }
func (irregularDirEntry) Type() fs.FileMode          { return fs.ModeDevice }
func (irregularDirEntry) Info() (fs.FileInfo, error) { return irregularFileInfo{}, nil }

type irregularFileInfo struct{}

func (irregularFileInfo) Name() string       { return "dev" }
func (irregularFileInfo) Size() int64        { return 0 }
func (irregularFileInfo) Mode() fs.FileMode  { return fs.ModeDevice }
func (irregularFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (irregularFileInfo) IsDir() bool        { return false }
func (irregularFileInfo) Sys() any           { return nil }

func TestCrossPlatformCoverageWalkLocalTreeEntry_errorBranches(t *testing.T) {
	files := map[string]*localFile{}

	// WalkDir 自身报错原样上抛。
	if err := walkLocalTreeEntry("/root", "/root/a", errDirEntry{}, errTestInfo, files); !errors.Is(err, errTestInfo) {
		t.Errorf("incoming error must propagate, got %v", err)
	}
	// 目录条目直接跳过。
	if err := walkLocalTreeEntry("/root", "/root/sub", errDirEntry{dir: true}, nil, files); err != nil {
		t.Errorf("directory entry must be skipped, got %v", err)
	}
	// Info() 失败上抛。
	if err := walkLocalTreeEntry("/root", "/root/a", errDirEntry{}, nil, files); !errors.Is(err, errTestInfo) {
		t.Errorf("Info() error must propagate, got %v", err)
	}
	// 非常规文件忽略。
	if err := walkLocalTreeEntry("/root", "/root/dev", irregularDirEntry{}, nil, files); err != nil {
		t.Errorf("irregular entry must be skipped, got %v", err)
	}
	// filepath.Rel 失败（root 绝对、path 相对）上抛。
	if err := walkLocalTreeEntry("/root", "relative/path", regularDirEntry{}, nil, files); err == nil {
		t.Error("filepath.Rel error must propagate")
	}
	if len(files) != 0 {
		t.Errorf("no entry should have been indexed, got %v", files)
	}
}

// regularDirEntry 的 Info() 返回一个常规文件，用于走到 filepath.Rel。
type regularDirEntry struct{}

func (regularDirEntry) Name() string               { return "a" }
func (regularDirEntry) IsDir() bool                { return false }
func (regularDirEntry) Type() fs.FileMode          { return 0 }
func (regularDirEntry) Info() (fs.FileInfo, error) { return regularFileInfo{}, nil }

type regularFileInfo struct{}

func (regularFileInfo) Name() string       { return "a" }
func (regularFileInfo) Size() int64        { return 1 }
func (regularFileInfo) Mode() fs.FileMode  { return 0o644 }
func (regularFileInfo) ModTime() time.Time { return time.Unix(1, 0) }
func (regularFileInfo) IsDir() bool        { return false }
func (regularFileInfo) Sys() any           { return nil }

func TestCrossPlatformCoverageWalkLocalForPushEntry_errorBranches(t *testing.T) {
	var dirs []string
	var files []localPushFile

	// WalkDir 自身报错原样上抛。
	if err := walkLocalForPushEntry("/root", "/root/a", regularDirEntry{}, errTestInfo, &dirs, &files); !errors.Is(err, errTestInfo) {
		t.Errorf("incoming error must propagate, got %v", err)
	}
	// filepath.Rel 失败上抛。
	if err := walkLocalForPushEntry("/root", "relative/path", regularDirEntry{}, nil, &dirs, &files); err == nil {
		t.Error("filepath.Rel error must propagate")
	}
	// 根目录本身跳过。
	if err := walkLocalForPushEntry("/root", "/root", regularDirEntry{}, nil, &dirs, &files); err != nil {
		t.Errorf("root itself must be skipped, got %v", err)
	}
	// Info() 失败上抛。
	if err := walkLocalForPushEntry("/root", "/root/a", errDirEntry{}, nil, &dirs, &files); !errors.Is(err, errTestInfo) {
		t.Errorf("Info() error must propagate, got %v", err)
	}
	// 非常规文件忽略。
	if err := walkLocalForPushEntry("/root", "/root/dev", irregularDirEntry{}, nil, &dirs, &files); err != nil {
		t.Errorf("irregular entry must be skipped, got %v", err)
	}
	// 目录条目进 dirs。
	if err := walkLocalForPushEntry("/root", "/root/sub", errDirEntry{dir: true}, nil, &dirs, &files); err != nil {
		t.Errorf("directory entry must be recorded, got %v", err)
	}
	if len(dirs) != 1 || dirs[0] != "sub" {
		t.Errorf("dirs = %v, want [sub]", dirs)
	}
	if len(files) != 0 {
		t.Errorf("files = %v, want empty", files)
	}
}

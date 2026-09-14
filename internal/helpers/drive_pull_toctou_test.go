package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func installPinnedPullSuccess(t *testing.T) {
	t.Helper()
	testseam.Swap(t, &deps, &Deps{Caller: &driveSyncMockCaller{}, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
		_, err := destination.WriteString("remote")
		return err
	})
}

// replacePullAncestorWithOutsideLink 把已被 pull 固定的 sub 目录移走，
// 再在原名处放置指向根外的符号链接，确定性复现远端调用/下载期间的换链。
func replacePullAncestorWithOutsideLink(t *testing.T, root, outside string) string {
	t.Helper()
	sub := filepath.Join(root, "sub")
	parked := filepath.Join(root, "sub-pinned")
	if forcePinnedFallbackForTest {
		swapPinnedParentIdentity(t, "sub")
		return sub
	}
	if err := os.Rename(sub, parked); err != nil {
		// Windows 在目录内有已打开的句柄（如 pull 的临时文件）时会锁住该目录，
		// 移走再换链于该平台物理不可达。退化为等价的父目录身份注入，命中
		// verifyParent 同一条 fail-closed 分支；原目录未移动，故返回 sub。
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		swapPinnedParentIdentity(t, "sub")
		return sub
	}
	if err := os.Symlink(outside, sub); err != nil {
		t.Skipf("平台不支持创建符号链接: %v", err)
	}
	return parked
}

// replacePullCommandRootWithOutsideLink 把命令根整体移走，并在原名处放置指向根外的
// 符号链接，复现「固定后命令根被换掉」的场景。Windows 在根目录持有已打开句柄时会
// 锁住该目录，移走于该平台物理不可达，此时退化为等价的根身份注入。
func replacePullCommandRootWithOutsideLink(t *testing.T, root, parked, outside string) {
	t.Helper()
	if forcePinnedFallbackForTest {
		swapPinnedRootIdentity(t, root)
		return
	}
	if err := os.Rename(root, parked); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		swapPinnedRootIdentity(t, root)
		return
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runPullTOCTOUCase(t *testing.T, command string, root string) error {
	t.Helper()
	rf := &remoteFile{RelPath: "sub/a.txt", FileID: "F"}
	if command == "pull" {
		_, err := pullOneFileAtRoot(context.Background(), "", rf, root, rf.RelPath, ifExistsOverwrite)
		return err
	}
	res := &driveSyncResult{}
	syncPullFile(res, context.Background(), "", rf, root, rf.RelPath, syncDirectionPull)
	if res.Summary.Failed != 1 || len(res.Items) != 1 {
		t.Fatalf("sync TOCTOU result = %#v", res)
	}
	if res.Items[0].Error == "" {
		t.Fatal("sync TOCTOU failure must retain the cause")
	}
	return &drivePullTestError{message: res.Items[0].Error}
}

type drivePullTestError struct{ message string }

func (e *drivePullTestError) Error() string { return e.message }

// download_file 返回期间父目录被换成指向根外的软链时，pull/sync
// 必须在创建临时文件和发出 HTTP GET 前失败。
func TestCrossPlatformCoverageDrivePullSyncRejectAncestorSwapBeforeTransfer(t *testing.T) {
	for _, command := range []string{"pull", "sync"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}

			var parked string
			caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
				if tool != "download_file" {
					t.Fatalf("unexpected MCP tool %q", tool)
				}
				parked = replacePullAncestorWithOutsideLink(t, root, outside)
				return `{"result":{"downloadUrl":"https://oss.example.com/get"}}`, nil
			}}
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})

			tempCalls, getCalls := 0, 0
			testseam.Swap(t, &pullCreateTemp, func(root *os.Root) (*os.File, string, error) {
				tempCalls++
				return createPinnedPullTemp(root)
			})
			testseam.Swap(t, &pullDownloadFile, func(context.Context, string, map[string]string, *os.File) error {
				getCalls++
				return nil
			})

			err := runPullTOCTOUCase(t, command, root)
			if err == nil {
				t.Fatal("换链后不应继续下载")
			}
			if tempCalls != 0 || getCalls != 0 {
				t.Fatalf("换链后发生了落盘: temp=%d HTTP_GET=%d", tempCalls, getCalls)
			}
			if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
				t.Fatalf("根外目录被写入: entries=%v err=%v", entries, readErr)
			}
			if entries, readErr := os.ReadDir(parked); readErr != nil || len(entries) != 0 {
				t.Fatalf("已移走的固定目录被写入: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

// HTTP 传输完成前父目录被换链时，已打开临时文件可以安全清理，
// 但最终发布必须在再次核对父目录身份后失败，不得沿新软链写到根外。
func TestCrossPlatformCoverageDrivePullSyncRechecksAncestorBeforePublish(t *testing.T) {
	for _, command := range []string{"pull", "sync"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
				return `{"result":{"downloadUrl":"https://oss.example.com/get"}}`, nil
			}}
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})

			parked := ""
			testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
				if _, err := destination.WriteString("remote"); err != nil {
					return err
				}
				parked = replacePullAncestorWithOutsideLink(t, root, outside)
				return nil
			})

			err := runPullTOCTOUCase(t, command, root)
			if err == nil {
				t.Fatal("发布前换链未被拒绝")
			}
			if command == "pull" && !strings.Contains(err.Error(), "目标目录在下载期间被替换") {
				t.Fatalf("pull error = %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(outside, "a.txt")); !os.IsNotExist(statErr) {
				t.Fatalf("根外目标被发布: %v", statErr)
			}
			if entries, readErr := os.ReadDir(parked); readErr != nil || len(entries) != 0 {
				t.Fatalf("失败后应清理临时文件: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveSyncKeepBothRejectsAncestorSwapWithoutRollback(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &deps, &Deps{Caller: &driveSyncMockCaller{}, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	parked := ""
	testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
		if _, err := destination.WriteString("remote"); err != nil {
			return err
		}
		parked = replacePullAncestorWithOutsideLink(t, root, outside)
		return nil
	})

	res := &driveSyncResult{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{FileID: "FID"}, root, "sub/a.txt", map[string]bool{})

	if res.Summary.Failed != 1 || len(res.Items) != 1 {
		t.Fatalf("keep-both TOCTOU result = %#v", res)
	}
	if !strings.Contains(res.Items[0].Error, "目标目录在下载期间被替换") {
		t.Fatalf("error = %q", res.Items[0].Error)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("不得沿换链路径触碰根外目录: entries=%v err=%v", entries, err)
	}
	kept := filepath.Join(parked, "a.conflict-FID.txt")
	if content, err := os.ReadFile(kept); err != nil || string(content) != "local" {
		t.Fatalf("本地保留硬链接应留在原固定目录: content=%q err=%v", content, err)
	}
}

func TestCrossPlatformCoveragePinnedPullFilesystemErrorBranches(t *testing.T) {
	t.Run("root mkdir", func(t *testing.T) {
		testseam.Swap(t, &pullMkdirAll, func(string, os.FileMode) error { return errTestDownload })
		if _, err := openPinnedPullTarget(filepath.Join(t.TempDir(), "root"), "a.txt"); err == nil {
			t.Fatal("expected root mkdir failure")
		}
	})

	t.Run("open root", func(t *testing.T) {
		root := t.TempDir()
		testseam.Swap(t, &pullOpenRoot, func(string) (*os.Root, error) { return nil, errTestDownload })
		if _, err := openPinnedPullTarget(root, "a.txt"); err == nil {
			t.Fatal("expected root open failure")
		}
	})

	t.Run("relative path", func(t *testing.T) {
		root := t.TempDir()
		for name, relFn := range map[string]func(string, string) (string, error){
			"error":  func(string, string) (string, error) { return "", errTestDownload },
			"escape": func(string, string) (string, error) { return "../escape", nil },
		} {
			t.Run(name, func(t *testing.T) {
				testseam.Swap(t, &pullRelPath, relFn)
				if _, err := openPinnedPullTarget(root, "a.txt"); err == nil {
					t.Fatal("expected relative-path failure")
				}
			})
		}
	})

	t.Run("parent mkdir", func(t *testing.T) {
		root := t.TempDir()
		testseam.Swap(t, &pullRootMkdir, func(*os.Root, string, os.FileMode) error { return errTestDownload })
		if _, err := openPinnedPullTarget(root, "sub/a.txt"); err == nil {
			t.Fatal("expected parent mkdir failure")
		}
	})

	t.Run("parent mkdir success", func(t *testing.T) {
		root := t.TempDir()
		target, err := openPinnedPullTarget(root, "sub/a.txt")
		if err != nil {
			t.Fatal(err)
		}
		target.close()
		if info, err := os.Lstat(filepath.Join(root, "sub")); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("created parent is not a real directory: info=%v err=%v", info, err)
		}
	})

	t.Run("open root clone", func(t *testing.T) {
		root := t.TempDir()
		testseam.Swap(t, &pullOpenParentRoot, func(*os.Root, string) (*os.Root, error) { return nil, errTestDownload })
		if _, err := openPinnedPullTarget(root, "sub/a.txt"); err == nil {
			t.Fatal("expected root clone failure")
		}
	})

	t.Run("open parent segment", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &pullOpenParentRoot, func(root *os.Root, name string) (*os.Root, error) {
			if name == "sub" {
				return nil, errTestDownload
			}
			return root.OpenRoot(name)
		})
		if _, err := openPinnedPullTarget(root, "sub/a.txt"); err == nil {
			t.Fatal("expected parent segment open failure")
		}
	})

	t.Run("lstat root parent", func(t *testing.T) {
		root := t.TempDir()
		testseam.Swap(t, &pullRootLstat, func(*os.Root, string) (os.FileInfo, error) { return nil, errTestDownload })
		if _, err := openPinnedPullTarget(root, "a.txt"); err == nil {
			t.Fatal("expected root parent lstat failure")
		}
	})

	t.Run("lstat parent segment", func(t *testing.T) {
		root := t.TempDir()
		testseam.Swap(t, &pullRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == "sub" {
				return nil, errTestDownload
			}
			return root.Lstat(name)
		})
		if _, err := openPinnedPullTarget(root, "sub/a.txt"); err == nil {
			t.Fatal("expected parent segment lstat failure")
		}
	})

	t.Run("stat parent", func(t *testing.T) {
		root := t.TempDir()
		testseam.Swap(t, &pullRootStat, func(*os.Root, string) (os.FileInfo, error) { return nil, errTestDownload })
		if _, err := openPinnedPullTarget(root, "a.txt"); err == nil {
			t.Fatal("expected parent stat failure")
		}
	})

	t.Run("parent changes while target opens", func(t *testing.T) {
		root := t.TempDir()
		calls := 0
		testseam.Swap(t, &pullRootStat, func(root *os.Root, name string) (os.FileInfo, error) {
			calls++
			if calls == 2 {
				return nil, errTestDownload
			}
			return root.Stat(name)
		})
		if _, err := openPinnedPullTarget(root, "a.txt"); err == nil {
			t.Fatal("expected construction-time parent identity failure")
		}
	})

	t.Run("lstat target", func(t *testing.T) {
		root := t.TempDir()
		target, err := openPinnedPullTarget(root, "a.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer target.close()
		testseam.Swap(t, &pullRootLstat, func(*os.Root, string) (os.FileInfo, error) { return nil, errTestDownload })
		if _, err := target.regularTargetInfo(); err == nil {
			t.Fatal("expected target lstat failure")
		}
	})

	t.Run("create temp root error and collision exhaustion", func(t *testing.T) {
		closed, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		_ = closed.Close()
		if _, _, err := createPinnedPullTemp(closed); err == nil {
			t.Fatal("expected closed root failure")
		}

		rootDir := t.TempDir()
		root, err := os.OpenRoot(rootDir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		const collision = "collision"
		if err := os.WriteFile(filepath.Join(rootDir, ".dws-pull-"+collision), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &pullTempName, func() string { return collision })
		if _, _, err := createPinnedPullTemp(root); err == nil {
			t.Fatal("expected collision exhaustion")
		}
	})
}

func TestCrossPlatformCoveragePinnedPullPublicationErrorBranches(t *testing.T) {
	call := func(t *testing.T, root string) error {
		t.Helper()
		_, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsOverwrite)
		return err
	}

	t.Run("temp stat", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullCreateTemp, func(root *os.Root) (*os.File, string, error) {
			file, name, err := createPinnedPullTemp(root)
			if err == nil {
				_ = file.Close()
			}
			return file, name, err
		})
		if err := call(t, root); err == nil || !strings.Contains(err.Error(), "读取临时文件身份失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temp sync", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullSyncTemp, func(*os.File) error { return errTestDownload })
		if err := call(t, root); err == nil || !strings.Contains(err.Error(), "同步临时文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temp close", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullCloseTemp, func(*os.File) error { return errTestDownload })
		if err := call(t, root); err == nil || !strings.Contains(err.Error(), "关闭临时文件失败") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("temp identity", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		other := filepath.Join(t.TempDir(), "other")
		if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
			t.Fatal(err)
		}
		otherInfo, err := os.Lstat(other)
		if err != nil {
			t.Fatal(err)
		}
		downloaded := false
		testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
			downloaded = true
			_, err := destination.WriteString("remote")
			return err
		})
		testseam.Swap(t, &pullRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if downloaded && strings.HasPrefix(name, ".dws-pull-") {
				return otherInfo, nil
			}
			return root.Lstat(name)
		})
		if err := call(t, root); err == nil || !strings.Contains(err.Error(), "临时文件在下载期间被替换") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("target becomes directory", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
			if _, err := destination.WriteString("remote"); err != nil {
				return err
			}
			return os.Mkdir(filepath.Join(root, "a.txt"), 0o755)
		})
		if err := call(t, root); err == nil || !strings.Contains(err.Error(), "不是常规文件") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCrossPlatformCoverageDefaultPullDownloadFileBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Test") != "yes" {
			t.Errorf("header = %q", request.Header.Get("X-Test"))
		}
		if request.URL.Path == "/status" {
			http.Error(w, "denied", http.StatusTeapot)
			return
		}
		_, _ = io.WriteString(w, "payload")
	}))
	defer server.Close()

	destination, err := os.Create(filepath.Join(t.TempDir(), "payload"))
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err := defaultPullDownloadFile(context.Background(), ":", nil, destination); err == nil {
		t.Fatal("expected invalid request URL")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultPullDownloadFile(cancelled, server.URL, nil, destination); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
	if err := defaultPullDownloadFile(context.Background(), server.URL+"/status", map[string]string{"X-Test": "yes"}, destination); err == nil {
		t.Fatal("expected HTTP status failure")
	}
	if err := defaultPullDownloadFile(context.Background(), server.URL+"/ok", map[string]string{"X-Test": "yes"}, destination); err != nil {
		t.Fatal(err)
	}

	closed, err := os.Create(filepath.Join(t.TempDir(), "closed"))
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if err := defaultPullDownloadFile(context.Background(), server.URL+"/ok", map[string]string{"X-Test": "yes"}, closed); err == nil {
		t.Fatal("expected destination copy failure")
	}
}

func TestCrossPlatformCoverageDriveSyncKeepBothLinkRemainsPinnedWhenParentMoves(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"a.txt":              "attacker-destination",
		"a.conflict-FID.txt": "attacker-source",
	} {
		if err := os.WriteFile(filepath.Join(outside, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	testseam.Swap(t, &deps, &Deps{Caller: &driveSyncMockCaller{}, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})

	links := 0
	parked := ""
	testseam.Swap(t, &syncRootLink, func(parent *os.Root, oldName, newName string) error {
		links++
		// reserve 已完成 verifyParent；在真正 no-clobber Link 前换链，验证操作
		// 仍只落在已固定的 parentRoot，而不会沿新软链进入根外目录。
		parked = replacePullAncestorWithOutsideLink(t, root, outside)
		return parent.Link(oldName, newName)
	})

	res := &driveSyncResult{}
	syncKeepBoth(res, context.Background(), "", &remoteFile{FileID: "FID"}, root, "sub/a.txt", map[string]bool{})
	if res.Summary.Failed != 1 || links != 1 {
		t.Fatalf("result=%#v links=%d", res, links)
	}
	for _, name := range []string{"a.txt", "a.conflict-FID.txt"} {
		if content, err := os.ReadFile(filepath.Join(parked, name)); err != nil || string(content) != "local" {
			t.Fatalf("固定目录内的 %s 未安全保留: content=%q err=%v", name, content, err)
		}
	}
	for name, want := range map[string]string{
		"a.txt":              "attacker-destination",
		"a.conflict-FID.txt": "attacker-source",
	} {
		if content, err := os.ReadFile(filepath.Join(outside, name)); err != nil || string(content) != want {
			t.Fatalf("根外 %s 被硬链接触碰: content=%q err=%v", name, content, err)
		}
	}
}

func TestCrossPlatformCoverageDriveSyncKeepBothDetectsCandidateMutationDuringTransfer(t *testing.T) {
	for _, mutation := range []string{"deleted", "replaced", "modified"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("local"), 0o600); err != nil {
				t.Fatal(err)
			}
			candidate := filepath.Join(root, "a.conflict-FID.txt")
			testseam.Swap(t, &deps, &Deps{Caller: &driveSyncMockCaller{}, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})
			testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
				if _, err := destination.WriteString("remote"); err != nil {
					return err
				}
				switch mutation {
				case "deleted":
					return os.Remove(candidate)
				case "replaced":
					if err := os.Remove(candidate); err != nil {
						return err
					}
					return os.WriteFile(candidate, []byte("attacker"), 0o600)
				default:
					return os.WriteFile(candidate, []byte("modified-local-version"), 0o600)
				}
			})

			res := &driveSyncResult{}
			syncKeepBoth(res, context.Background(), "", &remoteFile{FileID: "FID"}, root, "a.txt", map[string]bool{})
			if res.Summary.Failed != 1 || res.Summary.Pulled != 0 || len(res.Items) != 1 {
				t.Fatalf("result=%#v", res)
			}
			if res.Items[0].Action != syncActionFailed || !strings.Contains(res.Items[0].Error, "本地保留版本在传输期间") {
				t.Fatalf("candidate mutation must be failed, got %#v", res.Items)
			}
			if content, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(content) != "remote" {
				t.Fatalf("remote publish result changed: content=%q err=%v", content, err)
			}
		})
	}
}

func TestCrossPlatformCoverageDrivePullCommandPinsRootAcrossAllFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	caller := pullListingCaller(
		`{"name":"a.txt","type":"file","fileId":"A"},` +
			`{"name":"b.txt","type":"file","fileId":"B"}`)
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive", "pull"})

	calls := 0
	parked := filepath.Join(t.TempDir(), "original-root")
	testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
		calls++
		if _, err := destination.WriteString("remote"); err != nil {
			return err
		}
		if calls == 1 {
			replacePullCommandRootWithOutsideLink(t, root, parked, outside)
		}
		return nil
	})

	cmd := findDriveSubcommand(t, "pull")
	mustSetFlags(t, cmd, map[string]string{"local-folder": root, "remote-folder": "ROOT", "if-exists": "overwrite"})
	if err := runDrivePull(cmd, nil); err == nil {
		t.Fatal("root replacement must fail the command")
	}
	if calls != 1 {
		t.Fatalf("second file must fail before transfer when command root changed: calls=%d", calls)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside root was touched: entries=%v err=%v", entries, err)
	}
}

func TestCrossPlatformCoverageDrivePullPlansExistingRootBeforeRemoteRead(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "mirror")
	outside := t.TempDir()
	parked := filepath.Join(parent, "mirror-pinned")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
		if tool != "list_files" {
			t.Fatalf("root replacement must stop before %s", tool)
		}
		replacePullCommandRootWithOutsideLink(t, root, parked, outside)
		return `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}]}}`, nil
	}}
	getCalls := 0
	testseam.Swap(t, &pullDownloadFile, func(context.Context, string, map[string]string, *os.File) error {
		getCalls++
		return nil
	})

	err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--if-exists", "overwrite")
	if err == nil || !strings.Contains(err.Error(), "本地根目录在同步期间被替换") {
		t.Fatalf("error=%v", err)
	}
	if len(caller.callsFor("download_file")) != 0 || getCalls != 0 {
		t.Fatalf("root replacement reached transfer: MCP=%d GET=%d", len(caller.callsFor("download_file")), getCalls)
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("outside root was touched: entries=%v err=%v", entries, readErr)
	}
}

func TestCrossPlatformCoverageDrivePullPlansMissingRootBeforeRemoteRead(t *testing.T) {
	for _, takeover := range []string{"directory", "symlink"} {
		t.Run(takeover, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "missing", "mirror")
			outside := t.TempDir()
			caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
				if tool != "list_files" {
					t.Fatalf("missing-root takeover must stop before %s", tool)
				}
				if takeover == "directory" {
					if err := os.MkdirAll(root, 0o755); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, root); err != nil {
						t.Skipf("symlink unsupported: %v", err)
					}
				}
				return `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}]}}`, nil
			}}
			getCalls := 0
			testseam.Swap(t, &pullDownloadFile, func(context.Context, string, map[string]string, *os.File) error {
				getCalls++
				return nil
			})

			err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--if-exists", "overwrite")
			if err == nil || !strings.Contains(err.Error(), "在远端读取期间被占用") {
				t.Fatalf("error=%v", err)
			}
			if len(caller.callsFor("download_file")) != 0 || getCalls != 0 {
				t.Fatalf("missing-root takeover reached transfer: MCP=%d GET=%d", len(caller.callsFor("download_file")), getCalls)
			}
			if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
				t.Fatalf("outside root was touched: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDrivePullDryRunDoesNotMaterializeMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "mirror")
	caller := &driveScriptCaller{dryRun: true, reply: func(tool string, _ map[string]any, _ int) (string, error) {
		if tool != "list_files" {
			t.Fatalf("dry-run must stop before %s", tool)
		}
		return `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"A"}]}}`, nil
	}}
	if err := runDriveCmd(t, caller, "pull", "--local-folder", root, "--remote-folder", "ROOT", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("dry-run materialized missing root: %v", err)
	}
}

func TestCrossPlatformCoverageDriveSyncCommandPinsRootAcrossAllFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	caller := &driveSyncMockCaller{listJSON: `{"result":{"items":[` +
		`{"name":"a.txt","type":"file","fileId":"A"},` +
		`{"name":"b.txt","type":"file","fileId":"B"}` +
		`],"nextToken":""}}`}
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive", "sync"})
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error { return nil })

	calls := 0
	parked := filepath.Join(t.TempDir(), "original-root")
	testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
		calls++
		if _, err := destination.WriteString("remote"); err != nil {
			return err
		}
		if calls == 1 {
			replacePullCommandRootWithOutsideLink(t, root, parked, outside)
		}
		return nil
	})

	cmd := findDriveSubcommand(t, "sync")
	mustSetFlags(t, cmd, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
	if err := runDriveSync(cmd, nil); err == nil {
		t.Fatal("root replacement must fail sync")
	}
	if calls != 1 {
		t.Fatalf("second sync file must fail before transfer when command root changed: calls=%d", calls)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside root was touched: entries=%v err=%v", entries, err)
	}
}

func TestCrossPlatformCoverageDrivePullReappliesSkipAndSmartAtPublish(t *testing.T) {
	for _, policy := range []string{ifExistsSkip, ifExistsSmart} {
		t.Run(policy, func(t *testing.T) {
			root := t.TempDir()
			installPinnedPullSuccess(t)
			testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
				if _, err := destination.WriteString("remote"); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(root, "a.txt"), []byte("concurrent"), 0o600)
			})
			action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", policy)
			if err != nil || action != pullActionSkipped {
				t.Fatalf("action=%q err=%v", action, err)
			}
			if content, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(content) != "concurrent" {
				t.Fatalf("concurrent target was overwritten: content=%q err=%v", content, err)
			}
		})
	}
}

func TestCrossPlatformCoverageDrivePullPublicationIdentityAndLinkFailures(t *testing.T) {
	t.Run("link failure without target is not skipped", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullLink, func(*os.Root, string, string) error { return errTestDownload })
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsSkip)
		if err == nil || action != pullActionFailed || !strings.Contains(err.Error(), "发布目标文件失败") {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("post publish mismatch preserves unknown terminal", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		other := filepath.Join(t.TempDir(), "other")
		if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
			t.Fatal(err)
		}
		otherInfo, _ := os.Lstat(other)
		published := false
		testseam.Swap(t, &pullRename, func(root *os.Root, oldName, newName string) error {
			if err := root.Rename(oldName, newName); err != nil {
				return err
			}
			published = true
			return nil
		})
		testseam.Swap(t, &pullRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if published && name == "a.txt" {
				return otherInfo, nil
			}
			return root.Lstat(name)
		})
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsOverwrite)
		if err == nil || action != pullActionFailed {
			t.Fatalf("action=%q err=%v", action, err)
		}
		if content, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(content) != "remote" {
			t.Fatalf("未知 terminal entry 应保留: content=%q err=%v", content, err)
		}
	})
}

// os.Root 只保证路径不能逃逸根目录，仍会跟随指向根内目录的相对软链接。若 sub 在
// Lstat 与 OpenRoot 之间被换成 sub -> victim，pull/sync/keep-both 都必须在任何远端
// 读取或本地写入前拒绝，不能把 remote sub/a.txt 重定向到 victim/a.txt。
func TestCrossPlatformCoverageDrivePullSyncKeepBothRejectRootInternalRedirect(t *testing.T) {
	for _, redirect := range []string{"symlink", "directory"} {
		for _, command := range []string{"pull", "sync", "keep-both"} {
			t.Run(redirect+"/"+command, func(t *testing.T) {
				root := t.TempDir()
				sub := filepath.Join(root, "sub")
				parked := filepath.Join(root, "sub-pinned")
				victim := filepath.Join(root, "victim")
				if err := os.Mkdir(sub, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(victim, 0o755); err != nil {
					t.Fatal(err)
				}
				mustWrite(t, filepath.Join(sub, "a.txt"), "local")
				mustWrite(t, filepath.Join(victim, "a.txt"), "victim")

				caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
					if tool != "download_file" {
						t.Fatalf("unexpected tool %q", tool)
					}
					return `{"result":{"downloadUrl":"https://oss.example.com/get"}}`, nil
				}}
				testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
				testseam.Swap(t, &os.Args, []string{"dws", "drive"})
				testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
					_, err := destination.WriteString("remote")
					return err
				})

				redirectedVictim := filepath.Join(victim, "a.txt")
				swapped := false
				testseam.Swap(t, &pullOpenParentRoot, func(parent *os.Root, name string) (*os.Root, error) {
					if name == "sub" && !swapped {
						swapped = true
						if err := os.Rename(sub, parked); err != nil {
							t.Fatal(err)
						}
						if redirect == "symlink" {
							if err := os.Symlink("victim", sub); err != nil {
								t.Skipf("平台不支持创建符号链接: %v", err)
							}
						} else {
							if err := os.Rename(victim, sub); err != nil {
								t.Fatal(err)
							}
							redirectedVictim = filepath.Join(sub, "a.txt")
						}
					}
					return parent.OpenRoot(name)
				})

				rf := &remoteFile{RelPath: "sub/a.txt", FileID: "F"}
				switch command {
				case "pull":
					action, err := pullOneFileAtRoot(context.Background(), "", rf, root, rf.RelPath, ifExistsOverwrite)
					if err == nil || action != pullActionFailed {
						t.Fatalf("action=%q err=%v", action, err)
					}
				case "sync":
					res := &driveSyncResult{}
					syncPullFile(res, context.Background(), "", rf, root, rf.RelPath, syncDirectionPull)
					if res.Summary.Failed != 1 || res.Summary.Pulled != 0 {
						t.Fatalf("sync result=%#v", res)
					}
				default:
					res := &driveSyncResult{}
					syncKeepBoth(res, context.Background(), "", rf, root, rf.RelPath, map[string]bool{})
					if res.Summary.Failed != 1 || res.Summary.Pulled != 0 {
						t.Fatalf("keep-both result=%#v", res)
					}
				}

				if !swapped {
					t.Fatal("test did not install the root-internal redirect")
				}
				if got := len(caller.callsFor("download_file")); got != 0 {
					t.Fatalf("root-internal redirect reached download_file: %d", got)
				}
				for path, want := range map[string]string{
					filepath.Join(parked, "a.txt"): "local",
					redirectedVictim:               "victim",
				} {
					content, err := os.ReadFile(path)
					if err != nil || string(content) != want {
						t.Fatalf("%s changed: content=%q err=%v", path, content, err)
					}
				}
				if matches, err := filepath.Glob(filepath.Join(filepath.Dir(redirectedVictim), "a.conflict-*.txt")); err != nil || len(matches) != 0 {
					t.Fatalf("keep-both wrote into redirected victim: matches=%v err=%v", matches, err)
				}
			})
		}
	}
}

package helpers

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

var errDriveFinalCoverage = errors.New("drive final coverage failure")

func openDriveFinalCoverageRoot(t *testing.T) (string, *pinnedPullRoot, localPushFile) {
	t.Helper()
	rootPath := t.TempDir()
	path := filepath.Join(rootPath, "a.txt")
	mustWrite(t, path, "payload")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openExistingPinnedPullRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.close)
	return rootPath, root, localPushFile{
		RelPath: "a.txt", AbsPath: path, Size: info.Size(),
		ModTimeMillis: info.ModTime().UnixMilli(), scanInfo: info,
	}
}

func TestCrossPlatformCoverageDrivePushFinalMD5AndPinnedMatchEdges(t *testing.T) {
	t.Run("default md5 success and read failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "payload")
		mustWrite(t, path, "payload")
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pushMD5OpenedFile(file)
		_ = file.Close()
		wantSum := md5.Sum([]byte("payload"))
		want := base64.StdEncoding.EncodeToString(wantSum[:])
		if err != nil || got != want {
			t.Fatalf("md5=(%q,%v), want %q", got, err, want)
		}
		if _, err := pushMD5OpenedFile(file); err == nil {
			t.Fatal("closed file must fail MD5 reading")
		}
	})

	t.Run("open failure", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		local.RelPath = "missing.txt"
		if _, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: "x"}, false); err == nil {
			t.Fatal("missing file must fail exact matching")
		}
	})

	t.Run("stat and identity failures", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		testseam.Swap(t, &pushStatOpenedFile, func(*os.File) (os.FileInfo, error) {
			return nil, errDriveFinalCoverage
		})
		if _, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: "x"}, false); err == nil {
			t.Fatal("opened file stat failure must abort matching")
		}
	})

	t.Run("directory and replaced scan identity", func(t *testing.T) {
		rootPath, root, local := openDriveFinalCoverageRoot(t)
		if _, err := judgePinnedPushFileMatch(root, localPushFile{RelPath: "."}, &remoteFile{Hash: "x"}, false); err == nil {
			t.Fatal("directory must not be hashed as a regular file")
		}
		other := filepath.Join(rootPath, "other.txt")
		mustWrite(t, other, "other")
		otherInfo, _ := os.Lstat(other)
		local.scanInfo = otherInfo
		if _, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: "x"}, false); err == nil {
			t.Fatal("scan identity mismatch must abort matching")
		}
	})

	t.Run("hash failure and root verification", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		testseam.Swap(t, &pushMD5OpenedFile, func(*os.File) (string, error) {
			return "", errDriveFinalCoverage
		})
		if _, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: "x"}, false); err == nil {
			t.Fatal("hash read failure must abort matching")
		}
	})

	t.Run("root verification and both verdicts", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		sum := md5.Sum([]byte("payload"))
		hash := base64.StdEncoding.EncodeToString(sum[:])
		testseam.Swap(t, &pushVerifyPinnedSourceRoot, func(*pinnedPullRoot) error {
			return errDriveFinalCoverage
		})
		if _, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: hash}, false); !errors.Is(err, errDriveFinalCoverage) {
			t.Fatalf("root verification error=%v", err)
		}
	})

	t.Run("unchanged and modified", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		sum := md5.Sum([]byte("payload"))
		hash := base64.StdEncoding.EncodeToString(sum[:])
		if got, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: hash}, false); err != nil || got != matchUnchanged {
			t.Fatalf("equal hash=(%q,%v)", got, err)
		}
		if got, err := judgePinnedPushFileMatch(root, local, &remoteFile{Hash: "different"}, false); err != nil || got != matchModified {
			t.Fatalf("different hash=(%q,%v)", got, err)
		}
	})
}

func TestCrossPlatformCoverageDrivePushFinalPinnedUploadGuards(t *testing.T) {
	t.Run("initial root replacement", func(t *testing.T) {
		rootPath, root, local := openDriveFinalCoverageRoot(t)
		moved := filepath.Join(filepath.Dir(rootPath), "moved-root")
		replacePinnedMirrorRoot(t, rootPath, moved)
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, local); err == nil {
			t.Fatal("replaced root must abort before opening upload")
		}
	})

	t.Run("open and stat failures", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		_ = root.root.Close()
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, local); err == nil {
			t.Fatal("closed pinned root must fail file open")
		}
	})

	t.Run("opened file stat failure", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		testseam.Swap(t, &pushStatOpenedFile, func(*os.File) (os.FileInfo, error) {
			return nil, errDriveFinalCoverage
		})
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, local); err == nil {
			t.Fatal("stat failure must abort upload")
		}
	})

	t.Run("non regular replaced and changed", func(t *testing.T) {
		rootPath, root, local := openDriveFinalCoverageRoot(t)
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "root", root, localPushFile{RelPath: "."}); err == nil {
			t.Fatal("directory upload must fail")
		}
		other := filepath.Join(rootPath, "other")
		mustWrite(t, other, "other")
		otherInfo, _ := os.Lstat(other)
		replaced := local
		replaced.scanInfo = otherInfo
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, replaced); err == nil {
			t.Fatal("replaced scan identity must fail")
		}
		changed := local
		changed.Size++
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, changed); err == nil {
			t.Fatal("changed size must fail")
		}
	})

	t.Run("second root verification", func(t *testing.T) {
		_, root, local := openDriveFinalCoverageRoot(t)
		testseam.Swap(t, &pushVerifyPinnedSourceRoot, func(*pinnedPullRoot) error {
			return errDriveFinalCoverage
		})
		if err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, local); !errors.Is(err, errDriveFinalCoverage) {
			t.Fatalf("second verification error=%v", err)
		}
	})

	// PUT 期间源文件被原地改写（编辑器覆盖、截断重写、mmap）时，PUT 已把混合内容
	// 送到 OSS；如果 commit 只看根身份而不复核文件本身，overwrite/local-wins 会把
	// 半新半旧的字节流覆盖到远端。commit 前必须再取一次源身份，size/mtime/inode
	// 任一变化都要拒绝提交。
	for _, mutation := range []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"size grows during PUT", func(t *testing.T, dir string) {
			path := filepath.Join(dir, "a.txt")
			if err := os.WriteFile(path, []byte("payload-mutated-mid-transfer"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"mtime advances during PUT", func(t *testing.T, dir string) {
			path := filepath.Join(dir, "a.txt")
			future := time.Now().Add(2 * time.Second)
			if err := os.Chtimes(path, future, future); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run("mid transfer source modification: "+mutation.name, func(t *testing.T) {
			rootPath, root, local := openDriveFinalCoverageRoot(t)
			caller := pushOKCaller(nil)
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})

			putCalls := 0
			testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
				putCalls++
				mutation.mutate(t, rootPath)
				return nil
			})

			err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, local)
			if err == nil || !strings.Contains(err.Error(), "在传输期间被修改") {
				t.Fatalf("PUT-time mutation must abort commit, got err=%v", err)
			}
			if putCalls != 1 {
				t.Fatalf("PUT must run exactly once, got %d", putCalls)
			}
			if commits := caller.callsFor("commit_upload"); len(commits) != 0 {
				t.Fatalf("commit_upload must not run after mid-transfer mutation, got %v", commits)
			}
		})
		// PUT 之后再取源身份可能因文件被移动/删除失败：那也必须拒绝 commit，防止
		// 一个不能被验证的传输被提交到远端。
		t.Run("mid transfer stat failure aborts commit", func(t *testing.T) {
			_, root, local := openDriveFinalCoverageRoot(t)
			caller := pushOKCaller(nil)
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})

			calls := 0
			testseam.Swap(t, &pushStatOpenedFile, func(file *os.File) (os.FileInfo, error) {
				calls++
				if calls == 1 {
					return file.Stat()
				}
				return nil, errDriveFinalCoverage
			})
			testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
				return nil
			})

			err := pushUploadFilePinned(context.Background(), "", "P", "", "a.txt", root, local)
			if err == nil || !strings.Contains(err.Error(), "上传后读取本地上传文件身份失败") {
				t.Fatalf("post-PUT stat failure must abort commit, got %v", err)
			}
			if commits := caller.callsFor("commit_upload"); len(commits) != 0 {
				t.Fatalf("commit_upload must not run after post-PUT stat failure, got %v", commits)
			}
		})
	}
	t.Run("command scan errors", func(t *testing.T) {
		for _, command := range []string{"push", "sync"} {
			t.Run(command, func(t *testing.T) {
				testseam.Swap(t, &runDriveMirrorWalkLocalForPushPinned, func(*pinnedPullRoot) ([]string, []localPushFile, error) {
					return nil, nil, errDriveFinalCoverage
				})
				err := runDriveCmd(t, pushOKCaller(nil), command, "--local-folder", t.TempDir(), "--remote-folder", "ROOT")
				if !errors.Is(err, errDriveFinalCoverage) {
					t.Fatalf("scan error=%v", err)
				}
			})
		}
	})

	t.Run("walk initial and callback errors", func(t *testing.T) {
		rootPath, root, _ := openDriveFinalCoverageRoot(t)
		moved := filepath.Join(filepath.Dir(rootPath), "moved-walk-root")
		replacePinnedMirrorRoot(t, rootPath, moved)
		if _, _, err := walkLocalForPushPinned(root); err == nil {
			t.Fatal("initial root replacement must fail walk")
		}
	})

	t.Run("walk entry info error", func(t *testing.T) {
		_, root, _ := openDriveFinalCoverageRoot(t)
		testseam.Swap(t, &walkPinnedLocalFS, func(_ fs.FS, _ string, fn fs.WalkDirFunc) error {
			return fn("bad", errDirEntry{}, nil)
		})
		if _, _, err := walkLocalForPushPinned(root); !errors.Is(err, errTestInfo) {
			t.Fatalf("walk info error=%v", err)
		}
	})

	// fs.WalkDir 允许把中途的 lstat 失败通过第三个参数传给回调而不是 DirEntry.Info；
	// 该短路分支必须直接原样上抛，不能吞掉底层错误。macOS 上并非每次遍历都会经过它，
	// 因此需要一个显式测试固定这条路径。
	t.Run("walk callback receives error", func(t *testing.T) {
		_, root, _ := openDriveFinalCoverageRoot(t)
		testseam.Swap(t, &walkPinnedLocalFS, func(_ fs.FS, _ string, fn fs.WalkDirFunc) error {
			return fn("bad", nil, errTestInfo)
		})
		if _, _, err := walkLocalForPushPinned(root); !errors.Is(err, errTestInfo) {
			t.Fatalf("walk callback error = %v", err)
		}
	})

	t.Run("walk final root verification", func(t *testing.T) {
		rootPath, root, _ := openDriveFinalCoverageRoot(t)
		moved := filepath.Join(filepath.Dir(rootPath), "moved-after-walk")
		testseam.Swap(t, &walkPinnedLocalFS, func(_ fs.FS, _ string, _ fs.WalkDirFunc) error {
			replacePinnedMirrorRoot(t, rootPath, moved)
			return nil
		})
		if _, _, err := walkLocalForPushPinned(root); err == nil {
			t.Fatal("post-walk root replacement must fail")
		}
	})

	t.Run("regular entry", func(t *testing.T) {
		var dirs []string
		var files []localPushFile
		if err := walkLocalForPushEntry("/root", "/root/a", regularDirEntry{}, nil, &dirs, &files); err != nil || len(files) != 1 {
			t.Fatalf("files=%v err=%v", files, err)
		}
	})

	t.Run("root replacement between folder writes", func(t *testing.T) {
		for _, command := range []string{"push", "sync"} {
			t.Run(command, func(t *testing.T) {
				parent := t.TempDir()
				rootPath := filepath.Join(parent, "root")
				if err := os.MkdirAll(filepath.Join(rootPath, "a"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(rootPath, "b"), 0o755); err != nil {
					t.Fatal(err)
				}
				moved := filepath.Join(parent, "moved")
				caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, nth int) (string, error) {
					switch tool {
					case "list_files":
						return `{"result":{"items":[]}}`, nil
					case "create_folder":
						if nth == 0 {
							replacePinnedMirrorRoot(t, rootPath, moved)
						}
						return `{"result":{"fileId":"DIR"}}`, nil
					default:
						return `{"result":{"fileId":"FILE"}}`, nil
					}
				}}
				args := []string{command, "--local-folder", rootPath, "--remote-folder", "ROOT"}
				if command == "sync" {
					args = append(args, "--quick")
				}
				if err := runDriveCmd(t, caller, args...); err == nil || !strings.Contains(err.Error(), "本地根目录") {
					t.Fatalf("root replacement error=%v", err)
				}
			})
		}
	})
}

func TestCrossPlatformCoverageDrivePushFinalDefaultOpenedPUTAndCommit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Test") != "yes" {
			t.Errorf("header=%q", request.Header.Get("X-Test"))
		}
		if request.URL.Path == "/status" {
			http.Error(w, "denied", http.StatusTeapot)
			return
		}
		_, _ = io.Copy(io.Discard, request.Body)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "payload")
	mustWrite(t, path, "payload")
	put := func(ctx context.Context, url string, headers map[string]string) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		return defaultPushPutOpenedFile(ctx, url, headers, file, 7)
	}
	if err := put(context.Background(), ":", nil); err == nil {
		t.Fatal("invalid PUT URL must fail")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := put(cancelled, server.URL, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled PUT error=%v", err)
	}
	if err := put(context.Background(), server.URL+"/status", map[string]string{"X-Test": "yes"}); err == nil {
		t.Fatal("non-200 PUT must fail")
	}
	if err := put(context.Background(), server.URL+"/ok", map[string]string{"X-Test": "yes"}); err != nil {
		t.Fatal(err)
	}
	closed, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if err := defaultPushPutOpenedFile(context.Background(), server.URL, nil, closed, 7); err == nil {
		t.Fatal("closed upload file must fail seek")
	}

	caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
		if tool == "get_upload_info" {
			return `{"result":{"resourceUrls":[{"url":"https://upload.invalid","headers":{}}],"uploadId":"U"}}`, nil
		}
		return "{", nil
	}}
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
	testseam.Swap(t, &os.Args, []string{"dws", "drive"})
	if err := pushUploadWithTransport(context.Background(), "", "P", "", "a.txt", 7, func(string, map[string]string) error { return nil }); err == nil || !strings.Contains(err.Error(), "parse commit_upload") {
		t.Fatalf("malformed commit error=%v", err)
	}
}

func TestCrossPlatformCoverageDriveSyncFinalPreflightAndKeepBothErrors(t *testing.T) {
	t.Run("local target preflight lstat error and directory", func(t *testing.T) {
		_, root, _ := openDriveFinalCoverageRoot(t)
		failures := map[string]string{}
		testseam.Swap(t, &pullRootLstat, func(*os.Root, string) (os.FileInfo, error) {
			return nil, errDriveFinalCoverage
		})
		addPinnedLocalTargetPreflightFailures(root, map[string]*remoteFile{"remote.txt": {}}, failures)
		if !strings.Contains(failures["remote.txt"], "检查本地目标失败") {
			t.Fatalf("failures=%v", failures)
		}
	})

	t.Run("local target preflight walks existing directory", func(t *testing.T) {
		rootPath, root, _ := openDriveFinalCoverageRoot(t)
		if err := os.Mkdir(filepath.Join(rootPath, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		failures := map[string]string{}
		addPinnedLocalTargetPreflightFailures(root, map[string]*remoteFile{"sub/remote.txt": {}}, failures)
		if len(failures) != 0 {
			t.Fatalf("failures=%v", failures)
		}
		addPinnedLocalTargetPreflightFailures(root, map[string]*remoteFile{"sub": {}}, failures)
		if failures["sub"] == "" {
			t.Fatal("terminal directory must fail file target preflight")
		}
	})

	t.Run("wrapper root failures", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		mustWrite(t, path, "x")
		pullResult := &driveSyncResult{}
		syncPullFile(pullResult, context.Background(), "", &remoteFile{}, path, "a", syncDirectionPull)
		if pullResult.Summary.Failed != 1 {
			t.Fatalf("pull result=%#v", pullResult)
		}
		keepResult := &driveSyncResult{}
		syncKeepBoth(keepResult, context.Background(), "", &remoteFile{}, path, "a", map[string]bool{})
		if keepResult.Summary.Failed != 1 {
			t.Fatalf("keep result=%#v", keepResult)
		}
	})

	t.Run("missing and non-regular local versions", func(t *testing.T) {
		rootPath := t.TempDir()
		missing := &driveSyncResult{}
		syncKeepBoth(missing, context.Background(), "", &remoteFile{FileID: "F"}, rootPath, "missing.txt", map[string]bool{})
		if missing.Summary.Failed != 1 || !strings.Contains(missing.Items[0].Error, "不存在") {
			t.Fatalf("missing result=%#v", missing)
		}
		if err := os.Mkdir(filepath.Join(rootPath, "dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		nonRegular := &driveSyncResult{}
		syncKeepBoth(nonRegular, context.Background(), "", &remoteFile{FileID: "F"}, rootPath, "dir", map[string]bool{})
		if nonRegular.Summary.Failed != 1 || !strings.Contains(nonRegular.Items[0].Error, "不是常规文件") {
			t.Fatalf("non-regular result=%#v", nonRegular)
		}
	})

	t.Run("saved identity mismatch", func(t *testing.T) {
		rootPath := t.TempDir()
		mustWrite(t, filepath.Join(rootPath, "a.txt"), "local")
		other := filepath.Join(t.TempDir(), "other")
		mustWrite(t, other, "other")
		otherInfo, _ := os.Lstat(other)
		candidate := filepath.FromSlash(syncKeepBothCandidate("a.txt", "F", 0))
		testseam.Swap(t, &pullRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			if name == candidate {
				return otherInfo, nil
			}
			return root.Lstat(name)
		})
		res := &driveSyncResult{}
		syncKeepBoth(res, context.Background(), "", &remoteFile{FileID: "F"}, rootPath, "a.txt", map[string]bool{})
		if res.Summary.Failed != 1 || !strings.Contains(res.Items[0].Error, "身份不一致") {
			t.Fatalf("result=%#v", res)
		}
	})

	t.Run("nil transfer error fallback", func(t *testing.T) {
		rootPath := t.TempDir()
		mustWrite(t, filepath.Join(rootPath, "a.txt"), "local")
		testseam.Swap(t, &syncPullOneFilePinned, func(context.Context, string, *remoteFile, *pinnedPullTarget, string) (string, error) {
			return pullActionFailed, nil
		})
		res := &driveSyncResult{}
		syncKeepBoth(res, context.Background(), "", &remoteFile{FileID: "F"}, rootPath, "a.txt", map[string]bool{})
		if res.Summary.Failed != 1 || !strings.Contains(res.Items[0].Error, "未能拉取") {
			t.Fatalf("result=%#v", res)
		}
	})

	t.Run("unsafe candidate and parent verification", func(t *testing.T) {
		rootPath := t.TempDir()
		mustWrite(t, filepath.Join(rootPath, "a.txt"), "local")
		target, err := openPinnedPullTarget(rootPath, "a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := reserveSyncKeepBothTargetPinned(target, "\x00", "", map[string]bool{}); err == nil {
			t.Fatal("unsafe candidate must fail")
		}
		target.close()

		target, err = openPinnedPullTarget(rootPath, "a.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer target.close()
		moved := filepath.Join(filepath.Dir(rootPath), "moved-keep-root")
		replacePinnedMirrorRoot(t, rootPath, moved)
		if _, _, err := reserveSyncKeepBothTargetPinned(target, "a.txt", "", map[string]bool{}); err == nil {
			t.Fatal("replaced parent must fail before link")
		}
	})
}

type driveFinalCallbackReader struct{ callback func() }

func (reader driveFinalCallbackReader) Read(buffer []byte) (int, error) {
	reader.callback()
	return copy(buffer, "s\n"), nil
}

func TestCrossPlatformCoverageDriveSyncFinalPostDecisionRootVerification(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "root")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rootPath, "a.txt")
	mustWrite(t, path, "local")
	mtime := readFileMTimeMillis(t, path)
	moved := filepath.Join(parent, "moved")
	testseam.Swap(t, &syncAskStdin, io.Reader(driveFinalCallbackReader{callback: func() {
		replacePinnedMirrorRoot(t, rootPath, moved)
	}}))
	caller := syncCaller(map[string]string{
		"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"F","modifyTime":` + differentMillis(mtime) + `}]}}`,
	})
	err := runDriveCmd(t, caller, "sync", "--local-folder", rootPath, "--remote-folder", "ROOT", "--quick", "--on-conflict", "ask")
	if err == nil || !strings.Contains(err.Error(), "本地根目录") {
		t.Fatalf("post-decision root verification error=%v", err)
	}
}

func TestCrossPlatformCoverageDrivePushSyncFinalCommandValidationAndPrintErrors(t *testing.T) {
	for _, command := range []string{"push", "sync"} {
		t.Run(command+" required", func(t *testing.T) {
			cmd := findDriveSubcommand(t, command)
			var err error
			if command == "push" {
				err = runDrivePush(cmd, nil)
			} else {
				err = runDriveSync(cmd, nil)
			}
			if err == nil || !strings.Contains(err.Error(), "required") {
				t.Fatalf("required error=%v", err)
			}
		})

		t.Run(command+" relative", func(t *testing.T) {
			cmd := findDriveSubcommand(t, command)
			mustSetFlags(t, cmd, map[string]string{"local-folder": "relative", "remote-folder": "ROOT"})
			var err error
			if command == "push" {
				err = runDrivePush(cmd, nil)
			} else {
				err = runDriveSync(cmd, nil)
			}
			if err == nil || !strings.Contains(err.Error(), "绝对路径") {
				t.Fatalf("relative error=%v", err)
			}
		})

		t.Run(command+" preflight print", func(t *testing.T) {
			rootPath := t.TempDir()
			mustWrite(t, filepath.Join(rootPath, "A.txt"), "local")
			caller := pushOKCaller(map[string]string{
				"ROOT": `{"result":{"items":[{"name":"a.txt","type":"file","fileId":"F"}]}}`,
			})
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: failingWriter{}}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive", command})
			cmd := findDriveSubcommand(t, command)
			flags := map[string]string{"local-folder": rootPath, "remote-folder": "ROOT"}
			if command == "sync" {
				flags["quick"] = "true"
			}
			mustSetFlags(t, cmd, flags)
			var err error
			if command == "push" {
				err = runDrivePush(cmd, nil)
			} else {
				err = runDriveSync(cmd, nil)
			}
			if err == nil || !strings.Contains(err.Error(), "write failed") {
				t.Fatalf("PrintJSON error=%v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveSyncFinalPullWrapperSuccess(t *testing.T) {
	rootPath := t.TempDir()
	installPinnedPullSuccess(t)
	res := &driveSyncResult{}
	syncPullFile(res, context.Background(), "", &remoteFile{FileID: "F"}, rootPath, "a.txt", syncDirectionPull)
	if res.Summary.Pulled != 1 || res.Summary.Failed != 0 {
		t.Fatalf("result=%#v", res)
	}
}

func TestCrossPlatformCoverageDrivePushFinalPureRemainingEdges(t *testing.T) {
	if got := mirrorLocalRelError("good/../bad"); got == "" {
		t.Fatal("unsafe local mirror segment must fail")
	}
	failures := buildMirrorPreflight(nil, []localPushFile{{RelPath: "bad\x00name"}}, nil, nil)
	if failures["bad\x00name"] == "" {
		t.Fatalf("failures=%v", failures)
	}
	if got := parseNodeID("{"); got != "" {
		t.Fatalf("malformed node ID=%q", got)
	}
}

func TestCrossPlatformCoverageDrivePushFinalRemoteWalkerFailClosedEdges(t *testing.T) {
	cases := []struct {
		name  string
		reply func(map[string]any) string
		want  string
	}{
		{
			name: "unsafe entry",
			reply: func(map[string]any) string {
				return `{"result":{"items":[{"name":"../escape","type":"file","fileId":"F"}]}}`
			},
			want: "无法安全映射",
		},
		{
			name: "duplicate folder",
			reply: func(args map[string]any) string {
				if args["parentId"] == "D" {
					return `{"result":{"items":[]}}`
				}
				return `{"result":{"items":[{"name":"dir","type":"folder","fileId":"D"},{"name":"dir","type":"folder","fileId":"D"}]}}`
			},
			want: "重复",
		},
		{
			name: "folder without ID",
			reply: func(map[string]any) string {
				return `{"result":{"items":[{"name":"dir","type":"folder"}]}}`
			},
			want: "缺少目录 ID",
		},
		{
			name: "duplicate file",
			reply: func(map[string]any) string {
				return `{"result":{"items":[{"name":"a","type":"file","fileId":"A"},{"name":"a","type":"file","fileId":"A"}]}}`
			},
			want: "重复",
		},
		{
			name: "file without ID",
			reply: func(map[string]any) string {
				return `{"result":{"items":[{"name":"a","type":"file"}]}}`
			},
			want: "缺少文件 ID",
		},
		{
			name: "pagination cycle",
			reply: func(map[string]any) string {
				return `{"result":{"items":[],"nextToken":"P"}}`
			},
			want: "分页 token 重复",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &driveScriptCaller{reply: func(tool string, args map[string]any, _ int) (string, error) {
				if tool != "list_files" {
					t.Fatalf("unexpected tool %s", tool)
				}
				return tc.reply(args), nil
			}}
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})
			_, _, err := fetchRemoteTreeForPush(context.Background(), "", "ROOT")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

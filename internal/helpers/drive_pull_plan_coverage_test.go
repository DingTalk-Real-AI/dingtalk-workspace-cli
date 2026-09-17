package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func newPinnedPullRootForCoverage(t *testing.T, path string) *pinnedPullRoot {
	t.Helper()
	root, err := openExistingPinnedPullRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.close)
	return root
}

func newMissingPinnedPullPlanForCoverage(t *testing.T) *pinnedPullRootPlan {
	t.Helper()
	root := filepath.Join(t.TempDir(), "missing", "mirror")
	plan, err := planPinnedPullRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(plan.close)
	return plan
}

func TestCrossPlatformCoverageDrivePullRootPlanFailures(t *testing.T) {
	t.Run("existing root open", func(t *testing.T) {
		testseam.Swap(t, &pullOpenRoot, func(string) (*os.Root, error) { return nil, errTestDownload })
		if _, err := planPinnedPullRoot(t.TempDir()); err == nil {
			t.Fatal("expected existing-root open failure")
		}
	})

	t.Run("ancestor is a file", func(t *testing.T) {
		parent := t.TempDir()
		file := filepath.Join(parent, "file")
		mustWrite(t, file, "x")
		if _, err := planPinnedPullRoot(filepath.Join(file, "child")); err == nil ||
			(!strings.Contains(err.Error(), "不是目录") && !strings.Contains(err.Error(), "not a directory")) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("ancestor discovered as file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		mustWrite(t, file, "x")
		calls := 0
		testseam.Swap(t, &pullPathStat, func(string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, os.ErrNotExist
			}
			return os.Stat(file)
		})
		if _, err := planPinnedPullRoot(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "不是目录") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("ancestor open", func(t *testing.T) {
		testseam.Swap(t, &pullOpenRoot, func(string) (*os.Root, error) { return nil, errTestDownload })
		if _, err := planPinnedPullRoot(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("expected ancestor open failure")
		}
	})

	t.Run("path stat errors", func(t *testing.T) {
		for _, initial := range []bool{true, false} {
			t.Run(map[bool]string{true: "initial", false: "ancestor"}[initial], func(t *testing.T) {
				calls := 0
				testseam.Swap(t, &pullPathStat, func(path string) (os.FileInfo, error) {
					calls++
					if initial || calls > 1 {
						return nil, errTestDownload
					}
					return nil, os.ErrNotExist
				})
				if _, err := planPinnedPullRoot(filepath.Join(t.TempDir(), "missing")); err == nil {
					t.Fatal("expected path stat failure")
				}
			})
		}
	})

	t.Run("initial verification", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &pullRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, errTestDownload
			}
			return root.Lstat(name)
		})
		if _, err := planPinnedPullRoot(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("expected initial plan verification failure")
		}
	})

	t.Run("invalid and changed plan", func(t *testing.T) {
		if err := (&pinnedPullRootPlan{}).verify(); err == nil {
			t.Fatal("expected invalid plan failure")
		}
		root := newPinnedPullRootForCoverage(t, t.TempDir())
		bad := *root
		bad.absDir = filepath.Join(t.TempDir(), "gone")
		if err := (&pinnedPullRootPlan{ancestor: &bad}).verify(); err == nil {
			t.Fatal("expected changed ancestor failure")
		}
	})

	t.Run("unexpected missing-path lstat", func(t *testing.T) {
		plan := newMissingPinnedPullPlanForCoverage(t)
		testseam.Swap(t, &pullRootLstat, func(*os.Root, string) (os.FileInfo, error) { return nil, errTestDownload })
		if err := plan.verify(); err == nil {
			t.Fatal("expected missing-path lstat failure")
		}
	})
}

func TestCrossPlatformCoverageDrivePullRootMaterializeFailures(t *testing.T) {
	t.Run("verification", func(t *testing.T) {
		if _, err := (&pinnedPullRootPlan{}).materialize(); err == nil {
			t.Fatal("expected materialize verification failure")
		}
	})

	for _, tc := range []struct {
		name string
		stub func(*testing.T)
	}{
		{"open ancestor", func(t *testing.T) {
			testseam.Swap(t, &pullOpenParentRoot, func(*os.Root, string) (*os.Root, error) { return nil, errTestDownload })
		}},
		{"mkdir", func(t *testing.T) {
			testseam.Swap(t, &pullRootMkdir, func(*os.Root, string, os.FileMode) error { return errTestDownload })
		}},
		{"created lstat", func(t *testing.T) {
			testseam.Swap(t, &pullRootLstat, func(*os.Root, string) (os.FileInfo, error) { return nil, errTestDownload })
		}},
		{"created not directory", func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "file")
			mustWrite(t, file, "x")
			info, err := os.Lstat(file)
			if err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &pullRootLstat, func(*os.Root, string) (os.FileInfo, error) { return info, nil })
		}},
		{"created changes before root identity", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &pullRootLstat, func(root *os.Root, name string) (os.FileInfo, error) {
				calls++
				if calls == 2 {
					return nil, errTestDownload
				}
				return root.Lstat(name)
			})
		}},
		{"open created", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &pullOpenParentRoot, func(root *os.Root, name string) (*os.Root, error) {
				calls++
				if calls > 1 {
					return nil, errTestDownload
				}
				return root.OpenRoot(name)
			})
		}},
		{"created identity", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &pullRootStat, func(root *os.Root, name string) (os.FileInfo, error) {
				calls++
				if calls == 1 {
					return nil, errTestDownload
				}
				return root.Stat(name)
			})
		}},
		{"final root stat", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &pullRootStat, func(root *os.Root, name string) (os.FileInfo, error) {
				calls++
				if calls == 3 {
					return nil, errTestDownload
				}
				return root.Stat(name)
			})
		}},
		{"final verification", func(t *testing.T) {
			calls := 0
			testseam.Swap(t, &pullPathLstat, func(path string) (os.FileInfo, error) {
				calls++
				if calls >= 2 {
					return nil, errTestDownload
				}
				return os.Lstat(path)
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := newMissingPinnedPullPlanForCoverage(t)
			tc.stub(t)
			if root, err := plan.materialize(); err == nil {
				root.close()
				t.Fatal("expected materialize failure")
			}
		})
	}

	t.Run("final lstat and verify", func(t *testing.T) {
		for _, failAt := range []int{1, 2} {
			t.Run(string(rune('0'+failAt)), func(t *testing.T) {
				plan := newMissingPinnedPullPlanForCoverage(t)
				calls := 0
				if failAt == 1 {
					testseam.Swap(t, &pullPathLstat, func(string) (os.FileInfo, error) { return nil, errTestDownload })
				} else {
					testseam.Swap(t, &pullPathStat, func(path string) (os.FileInfo, error) {
						calls++
						if calls >= 2 {
							return nil, errTestDownload
						}
						return os.Stat(path)
					})
				}
				if root, err := plan.materialize(); err == nil {
					root.close()
					t.Fatal("expected final materialize failure")
				}
			})
		}
	})

	t.Run("open existing path identity and verification", func(t *testing.T) {
		for _, fail := range []string{"lstat", "verify"} {
			t.Run(fail, func(t *testing.T) {
				dir := t.TempDir()
				if fail == "lstat" {
					testseam.Swap(t, &pullPathLstat, func(string) (os.FileInfo, error) { return nil, errTestDownload })
				} else {
					pathCalls := 0
					testseam.Swap(t, &pullPathLstat, func(path string) (os.FileInfo, error) {
						pathCalls++
						if pathCalls >= 2 {
							return nil, errTestDownload
						}
						return os.Lstat(path)
					})
				}
				if _, err := openExistingPinnedPullRoot(dir); err == nil {
					t.Fatal("expected existing-root identity failure")
				}
			})
		}
	})
}

func TestCrossPlatformCoverageDrivePullPublishRaces(t *testing.T) {
	t.Run("smart concurrent target", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		mustWrite(t, filepath.Join(root, "a.txt"), "older")
		testseam.Swap(t, &pullDownloadFile, func(_ context.Context, _ string, _ map[string]string, destination *os.File) error {
			if _, err := destination.WriteString("remote"); err != nil {
				return err
			}
			return os.Chtimes(filepath.Join(root, "a.txt"), time.Now(), time.Now().Add(time.Hour))
		})
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F", ModifiedTimeValid: true}, root, "a.txt", ifExistsSmart)
		if err != nil || action != pullActionSkipped {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("link loses to regular target", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullLink, func(parent *os.Root, _, target string) error {
			file, err := parent.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0o600)
			if err == nil {
				_ = file.Close()
			}
			return os.ErrExist
		})
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsSkip)
		if err != nil || action != pullActionSkipped {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("link cleanup", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullRemove, func(*os.Root, string) error { return errTestDownload })
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsSkip)
		if err == nil || action != pullActionFailed || !strings.Contains(err.Error(), "清理临时文件") {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("post publish parent", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		statCalls := 0
		testseam.Swap(t, &pullPathStat, func(path string) (os.FileInfo, error) {
			statCalls++
			if statCalls >= 5 {
				return nil, errTestDownload
			}
			return os.Stat(path)
		})
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsOverwrite)
		if err == nil || action != pullActionFailed {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("overwrite rename", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		testseam.Swap(t, &pullRename, func(*os.Root, string, string) error { return errTestDownload })
		action, err := pullOneFileAtRoot(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsOverwrite)
		if err == nil || action != pullActionFailed {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})
}

func TestCrossPlatformCoverageDrivePullPlanCallers(t *testing.T) {
	t.Run("second preflight verification", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "a.txt"), "local")
		root := newPinnedPullRootForCoverage(t, dir)
		plan := &pinnedPullRootPlan{absDir: root.absDir, existing: root}
		moved := false
		testseam.Swap(t, &pullRootLstat, func(parent *os.Root, name string) (os.FileInfo, error) {
			info, err := parent.Lstat(name)
			if name == "a.txt" && !moved {
				moved = true
				if renameErr := os.Rename(dir, dir+"-pinned"); renameErr != nil {
					// Windows 在固定目录上持有句柄时会锁住它，移走于该平台物理
					// 不可达；注入等价的根身份变化，命中同一条二次校验分支。
					if runtime.GOOS != "windows" {
						t.Fatal(renameErr)
					}
					swapPinnedRootIdentity(t, dir)
				} else if mkdirErr := os.Mkdir(dir, 0o755); mkdirErr != nil {
					t.Fatal(mkdirErr)
				}
			}
			return info, err
		})
		if _, _, err := buildDrivePullPreflightFromPlan(plan, map[string]*remoteFile{"a.txt": {RelPath: "a.txt"}}); err == nil {
			t.Fatal("expected second verification failure")
		}
	})

	t.Run("command materialize", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "missing")
		caller := pullListingCaller(`{"name":"a.txt","type":"file","fileId":"A"}`)
		testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
		testseam.Swap(t, &os.Args, []string{"dws", "drive"})
		testseam.Swap(t, &pullOpenParentRoot, func(*os.Root, string) (*os.Root, error) { return nil, errTestDownload })
		cmd := findDriveSubcommand(t, "pull")
		mustSetFlags(t, cmd, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
		if err := runDrivePull(cmd, nil); err == nil || !errors.Is(err, errTestDownload) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("preflight output", func(t *testing.T) {
		root := t.TempDir()
		caller := pullListingCaller(`{"name":"A.txt","type":"file","fileId":"A"},{"name":"a.txt","type":"file","fileId":"B"}`)
		testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: failingWriter{}}})
		testseam.Swap(t, &os.Args, []string{"dws", "drive"})
		cmd := findDriveSubcommand(t, "pull")
		mustSetFlags(t, cmd, map[string]string{"local-folder": root, "remote-folder": "ROOT"})
		if err := runDrivePull(cmd, nil); err == nil {
			t.Fatal("expected preflight output failure")
		}
	})

	t.Run("remote helper", func(t *testing.T) {
		root := t.TempDir()
		installPinnedPullSuccess(t)
		action, err := pullRemoteFile(context.Background(), "", &remoteFile{FileID: "F"}, root, "a.txt", ifExistsOverwrite)
		if err != nil || action != pullActionDownloaded {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("target construction final verification", func(t *testing.T) {
		root := newPinnedPullRootForCoverage(t, t.TempDir())
		calls := 0
		testseam.Swap(t, &pullRootLstat, func(parent *os.Root, name string) (os.FileInfo, error) {
			calls++
			if calls >= 2 && name == "." {
				return nil, errTestDownload
			}
			return parent.Lstat(name)
		})
		if _, err := root.openTarget("a.txt"); err == nil {
			t.Fatal("expected target verification failure")
		}
	})
}

func TestCrossPlatformCoverageDrivePullDefaultTransportWithoutListener(t *testing.T) {
	destination, err := os.Create(filepath.Join(t.TempDir(), "payload"))
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()

	if err := defaultPullDownloadFile(context.Background(), ":", nil, destination); err == nil {
		t.Fatal("expected invalid request URL")
	}

	t.Run("transport error", func(t *testing.T) {
		testseam.Swap(t, &pullHTTPDo, func(*http.Request) (*http.Response, error) { return nil, errTestDownload })
		if err := defaultPullDownloadFile(context.Background(), "https://example.invalid", nil, destination); !errors.Is(err, errTestDownload) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("status", func(t *testing.T) {
		testseam.Swap(t, &pullHTTPDo, func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("X-Test") != "yes" {
				t.Fatalf("header = %q", request.Header.Get("X-Test"))
			}
			return &http.Response{StatusCode: http.StatusTeapot, Body: io.NopCloser(strings.NewReader("denied"))}, nil
		})
		if err := defaultPullDownloadFile(context.Background(), "https://example.invalid", map[string]string{"X-Test": "yes"}, destination); err == nil {
			t.Fatal("expected status failure")
		}
	})

	t.Run("success and copy", func(t *testing.T) {
		testseam.Swap(t, &pullHTTPDo, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("payload"))}, nil
		})
		if err := defaultPullDownloadFile(context.Background(), "https://example.invalid", nil, destination); err != nil {
			t.Fatal(err)
		}
		closed, createErr := os.Create(filepath.Join(t.TempDir(), "closed"))
		if createErr != nil {
			t.Fatal(createErr)
		}
		_ = closed.Close()
		if err := defaultPullDownloadFile(context.Background(), "https://example.invalid", nil, closed); err == nil {
			t.Fatal("expected copy failure")
		}
	})
}

func TestCrossPlatformCoverageDrivePullMTimeHandle(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "mtime"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := setPullFileTimes(file, time.Unix(123, 0)); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDrivePullDryRunLocalPolicies(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	mustWrite(t, path, "local")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &deps, &Deps{Out: &Formatter{w: io.Discard}})
	remote := map[string]*remoteFile{"a.txt": {RelPath: "a.txt", ModifiedTimeValid: true, ModifiedTime: info.ModTime().UnixMilli()}}
	for _, policy := range []string{ifExistsSkip, ifExistsSmart, ifExistsOverwrite} {
		if err := printDrivePullDryRunWithLocalInfo(policy, remote, []string{"a.txt"}, nil, map[string]os.FileInfo{"a.txt": info}); err != nil {
			t.Fatalf("policy %s: %v", policy, err)
		}
	}
	if err := printDrivePullDryRunWithLocalInfo(ifExistsSkip, remote, []string{"missing.txt"}, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageDrivePullRejectNonRegular(t *testing.T) {
	if err := rejectNonRegularLocalTarget(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := rejectNonRegularLocalTarget(dir); err == nil {
		t.Fatal("expected directory rejection")
	}
	testseam.Swap(t, &pullTargetLstat, func(string) (os.FileInfo, error) { return nil, errTestDownload })
	if err := rejectNonRegularLocalTarget("target"); err == nil {
		t.Fatal("expected lstat failure")
	}
}

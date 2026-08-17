package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func runDriveMirrorCommand(t *testing.T, caller *driveScriptCaller, dryRun bool, args ...string) ([]byte, error) {
	t.Helper()
	caller.dryRun = dryRun
	var out bytes.Buffer
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: &out}})

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().BoolP("yes", "y", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(newDriveCommand())
	full := append(append([]string{}, args...), "--yes")
	testseam.Swap(t, &os.Args, append([]string{"dws", "drive"}, full...))
	root.SetArgs(append([]string{"drive"}, full...))
	err := root.Execute()
	return append([]byte(nil), out.Bytes()...), err
}

func captureMirrorHTTP(t *testing.T) (getCalls, putCalls *int) {
	t.Helper()
	gets, puts := 0, 0
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
		gets++
		return nil
	})
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
		puts++
		return nil
	})
	return &gets, &puts
}

func assertMirrorPreflightNoWrites(t *testing.T, caller *driveScriptCaller, getCalls, putCalls int) {
	t.Helper()
	for _, tool := range []string{"create_folder", "download_file", "get_upload_info", "commit_upload"} {
		if calls := caller.callsFor(tool); len(calls) != 0 {
			t.Errorf("mirror preflight must not call %s: %v", tool, calls)
		}
	}
	if getCalls != 0 || putCalls != 0 {
		t.Errorf("mirror preflight must not transfer content: GET=%d PUT=%d", getCalls, putCalls)
	}
}

func TestCrossPlatformCoverageDrivePull_nonRegularTargetFailsBeforeDownload(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		name := "execute"
		if dryRun {
			name = "dry-run"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "blocked.txt"), 0o755); err != nil {
				t.Fatal(err)
			}
			caller := syncCaller(map[string]string{
				"ROOT": `{"result":{"items":[{"name":"blocked.txt","type":"file","fileId":"REMOTE"}],"nextToken":""}}`,
			})
			gets, puts := captureMirrorHTTP(t)
			out, err := runDriveMirrorCommand(t, caller, dryRun, "pull", "--local-folder", root, "--remote-folder", "ROOT")
			if dryRun {
				if err != nil {
					t.Fatalf("pull dry-run: %v", err)
				}
				var got drivePullDryRunResult
				if jsonErr := json.Unmarshal(out, &got); jsonErr != nil {
					t.Fatalf("pull dry-run output is not JSON: %v\n%s", jsonErr, out)
				}
				if got.Plan.Summary.Failed != 1 || len(got.Plan.Items) != 1 || !strings.Contains(got.Plan.Items[0].Error, "不是常规文件") {
					t.Fatalf("pull dry-run plan = %+v, want one non-regular target failure", got.Plan)
				}
			} else {
				var failure *drivePartialFailure
				if !errors.As(err, &failure) || failure.failed != 1 {
					t.Fatalf("pull error = %v, want one failed item", err)
				}
			}
			assertMirrorPreflightNoWrites(t, caller, *gets, *puts)
		})
	}
}

func TestCrossPlatformCoverageDrivePull_nonRegularTargetAbortsWholeBatchBeforeDownload(t *testing.T) {
	for _, command := range []string{"pull", "sync"} {
		for _, dryRun := range []bool{false, true} {
			name := command + "/execute"
			if dryRun {
				name = command + "/dry-run"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				if err := os.Mkdir(filepath.Join(root, "z-blocked.txt"), 0o755); err != nil {
					t.Fatal(err)
				}
				caller := syncCaller(map[string]string{
					"ROOT": `{"result":{"items":[{"name":"a-first.txt","type":"file","fileId":"FIRST"},{"name":"z-blocked.txt","type":"file","fileId":"BLOCKED"}],"nextToken":""}}`,
				})
				gets, puts := captureMirrorHTTP(t)
				args := []string{command, "--local-folder", root, "--remote-folder", "ROOT"}
				if command == "sync" {
					args = append(args, "--quick")
				}
				out, err := runDriveMirrorCommand(t, caller, dryRun, args...)
				if dryRun {
					if err != nil {
						t.Fatalf("%s dry-run: %v", command, err)
					}
					if command == "pull" {
						var got drivePullDryRunResult
						if jsonErr := json.Unmarshal(out, &got); jsonErr != nil || got.Plan.Summary.Failed != 2 {
							t.Fatalf("pull dry-run preflight = %+v, json error %v", got, jsonErr)
						}
					} else {
						assertMirrorDryRunFailureJSON(t, command, out)
					}
				} else if err == nil {
					t.Fatalf("%s must abort the whole batch", command)
				}
				if _, statErr := os.Stat(filepath.Join(root, "a-first.txt")); !os.IsNotExist(statErr) {
					t.Fatalf("%s wrote an earlier file before discovering the conflict: %v", command, statErr)
				}
				assertMirrorPreflightNoWrites(t, caller, *gets, *puts)
			})
		}
	}
}

func TestCrossPlatformCoverageDrivePull_remoteEquivalentTreeFailsBeforeDownload(t *testing.T) {
	tests := []struct {
		name     string
		listings map[string]string
		wantFail int
	}{
		{
			name: "equivalent files",
			listings: map[string]string{
				"ROOT": `{"result":{"items":[{"name":"A.txt","type":"file","fileId":"UP"},{"name":"a.txt","type":"file","fileId":"LOW"}],"nextToken":""}}`,
			},
			wantFail: 2,
		},
		{
			name: "equivalent folder prefixes",
			listings: map[string]string{
				"ROOT":    `{"result":{"items":[{"name":"Folder","type":"folder","fileId":"UP_DIR"},{"name":"folder","type":"folder","fileId":"LOW_DIR"}],"nextToken":""}}`,
				"UP_DIR":  `{"result":{"items":[{"name":"up.txt","type":"file","fileId":"UP"}],"nextToken":""}}`,
				"LOW_DIR": `{"result":{"items":[{"name":"low.txt","type":"file","fileId":"LOW"}],"nextToken":""}}`,
			},
			wantFail: 2,
		},
	}
	for _, tt := range tests {
		for _, dryRun := range []bool{false, true} {
			name := tt.name + "/execute"
			if dryRun {
				name = tt.name + "/dry-run"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				caller := syncCaller(tt.listings)
				gets, puts := captureMirrorHTTP(t)
				out, err := runDriveMirrorCommand(t, caller, dryRun, "pull", "--local-folder", root, "--remote-folder", "ROOT")
				if dryRun {
					if err != nil {
						t.Fatalf("pull dry-run: %v", err)
					}
					var got drivePullDryRunResult
					if jsonErr := json.Unmarshal(out, &got); jsonErr != nil {
						t.Fatalf("pull dry-run output is not JSON: %v\n%s", jsonErr, out)
					}
					if got.Plan.Summary.Failed != tt.wantFail || got.Plan.Summary.Downloaded != 0 {
						t.Fatalf("pull dry-run plan = %+v", got.Plan)
					}
				} else {
					var failure *drivePartialFailure
					if !errors.As(err, &failure) || failure.failed != tt.wantFail {
						t.Fatalf("pull error = %v, want %d failed items", err, tt.wantFail)
					}
				}
				if !strings.Contains(string(out), "等价路径") {
					t.Fatalf("pull output must explain equivalent path ambiguity: %s", out)
				}
				assertMirrorPreflightNoWrites(t, caller, *gets, *puts)
			})
		}
	}
}

func TestCrossPlatformCoverageDriveMirror_unsafeLocalNamesFailClosed(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("current platform cannot create a local name containing a backslash")
	}
	for _, command := range []string{"push", "sync"} {
		for _, dryRun := range []bool{false, true} {
			name := command + "/execute"
			if dryRun {
				name = command + "/dry-run"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				mustWrite(t, filepath.Join(root, `unsafe\name.txt`), "local")
				caller := syncCaller(nil)
				gets, puts := captureMirrorHTTP(t)
				args := []string{command, "--local-folder", root, "--remote-folder", "ROOT"}
				if command == "sync" {
					args = append(args, "--quick")
				}
				out, err := runDriveMirrorCommand(t, caller, dryRun, args...)
				if dryRun {
					if err != nil {
						t.Fatalf("%s dry-run: %v", command, err)
					}
					assertMirrorDryRunFailureJSON(t, command, out)
				} else if err == nil {
					t.Fatalf("%s must fail an unsafe local name", command)
				}
				if !strings.Contains(string(out), "无法安全映射到远端") {
					t.Fatalf("%s output must explain unsafe local name: %s", command, out)
				}
				assertMirrorPreflightNoWrites(t, caller, *gets, *puts)
			})
		}
	}

	// NUL 不能由真实文件系统创建，直接覆盖契约 helper，避免该分支成为死角。
	preflight := buildMirrorPreflight(nil, []localPushFile{{RelPath: "bad\x00name.txt"}}, nil, map[string]string{"": "ROOT"})
	if !strings.Contains(preflight["bad\x00name.txt"], "无法安全映射到远端") {
		t.Fatalf("NUL name preflight = %v", preflight)
	}
	if err := rejectNonRegularLocalTarget("bad\x00target"); err == nil || !strings.Contains(err.Error(), "检查本地目标失败") {
		t.Fatalf("invalid local target error = %v", err)
	}

	localCollision := buildMirrorPreflight(nil, []localPushFile{{RelPath: "A.txt"}, {RelPath: "a.txt"}}, nil, map[string]string{"": "ROOT"})
	for _, rel := range []string{"A.txt", "a.txt"} {
		if !strings.Contains(localCollision[rel], "等价路径") {
			t.Fatalf("local equivalent collision %q = %q", rel, localCollision[rel])
		}
	}
	prefixCollision := buildMirrorPreflight(nil, []localPushFile{{RelPath: "A/x.txt"}},
		map[string]*remoteFile{"a/y.txt": {RelPath: "a/y.txt"}}, map[string]string{"": "ROOT"})
	for _, rel := range []string{"A", "a"} {
		if !strings.Contains(prefixCollision[rel], "等价路径") {
			t.Fatalf("equivalent ancestor collision %q = %q", rel, prefixCollision[rel])
		}
	}
}

func TestCrossPlatformCoverageDriveMirror_crossSideEquivalentPathsFailBeforeWrites(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, string)
		listings map[string]string
	}{
		{
			name:    "case-equivalent files",
			prepare: func(t *testing.T, root string) { mustWrite(t, filepath.Join(root, "a.txt"), "local") },
			listings: map[string]string{
				"ROOT": `{"result":{"items":[{"name":"A.txt","type":"file","fileId":"REMOTE"}],"nextToken":""}}`,
			},
		},
		{
			name: "case-equivalent folder prefixes",
			prepare: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(root, "Folder", "local.txt"), "local")
			},
			listings: map[string]string{
				"ROOT":       `{"result":{"items":[{"name":"folder","type":"folder","fileId":"REMOTE_DIR"}],"nextToken":""}}`,
				"REMOTE_DIR": `{"result":{"items":[{"name":"remote.txt","type":"file","fileId":"REMOTE_FILE"}],"nextToken":""}}`,
			},
		},
		{
			name:    "unicode-equivalent files",
			prepare: func(t *testing.T, root string) { mustWrite(t, filepath.Join(root, "caf\u00e9.txt"), "local") },
			listings: map[string]string{
				"ROOT": `{"result":{"items":[{"name":"cafe\u0301.txt","type":"file","fileId":"REMOTE"}],"nextToken":""}}`,
			},
		},
		{
			name:    "equivalent path type ambiguity",
			prepare: func(t *testing.T, root string) { mustWrite(t, filepath.Join(root, "item"), "local") },
			listings: map[string]string{
				"ROOT":       `{"result":{"items":[{"name":"ITEM","type":"folder","fileId":"REMOTE_DIR"}],"nextToken":""}}`,
				"REMOTE_DIR": `{"result":{"items":[],"nextToken":""}}`,
			},
		},
		{
			name:    "remote-only equivalent files",
			prepare: func(t *testing.T, root string) { mustWrite(t, filepath.Join(root, "safe-local.txt"), "local") },
			listings: map[string]string{
				"ROOT": `{"result":{"items":[{"name":"A.txt","type":"file","fileId":"UP"},{"name":"a.txt","type":"file","fileId":"LOW"}],"nextToken":""}}`,
			},
		},
		{
			name:    "remote-only equivalent folders",
			prepare: func(t *testing.T, root string) { mustWrite(t, filepath.Join(root, "safe-local.txt"), "local") },
			listings: map[string]string{
				"ROOT":    `{"result":{"items":[{"name":"Folder","type":"folder","fileId":"UP_DIR"},{"name":"folder","type":"folder","fileId":"LOW_DIR"}],"nextToken":""}}`,
				"UP_DIR":  `{"result":{"items":[],"nextToken":""}}`,
				"LOW_DIR": `{"result":{"items":[],"nextToken":""}}`,
			},
		},
	}

	for _, tt := range tests {
		for _, command := range []string{"push", "sync"} {
			for _, dryRun := range []bool{false, true} {
				name := tt.name + "/" + command + "/execute"
				if dryRun {
					name = tt.name + "/" + command + "/dry-run"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					tt.prepare(t, root)
					caller := syncCaller(tt.listings)
					gets, puts := captureMirrorHTTP(t)
					args := []string{command, "--local-folder", root, "--remote-folder", "ROOT"}
					if command == "sync" {
						args = append(args, "--quick")
					}
					out, err := runDriveMirrorCommand(t, caller, dryRun, args...)
					if dryRun {
						if err != nil {
							t.Fatalf("%s dry-run: %v", command, err)
						}
						assertMirrorDryRunFailureJSON(t, command, out)
					} else if err == nil {
						t.Fatalf("%s must reject an equivalent cross-side path", command)
					}
					if !strings.Contains(string(out), "等价路径") {
						t.Fatalf("%s output must explain equivalent path ambiguity: %s", command, out)
					}
					assertMirrorPreflightNoWrites(t, caller, *gets, *puts)
				})
			}
		}
	}
}

func assertMirrorDryRunFailureJSON(t *testing.T, command string, out []byte) {
	t.Helper()
	switch command {
	case "push":
		var got drivePushDryRunResult
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("push dry-run output is not JSON: %v\n%s", err, out)
		}
		if !got.DryRun || got.Executed || got.Operation != "drive push" || got.Plan.Summary.Failed == 0 || !got.Plan.Summary.Aborted {
			t.Fatalf("push dry-run result = %+v", got)
		}
	case "sync":
		var got driveSyncDryRunResult
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("sync dry-run output is not standalone JSON: %v\n%s", err, out)
		}
		if !got.DryRun || got.Executed || got.PreviewKind != "plan" || got.Operation != "drive sync" || got.Plan.Summary.Failed == 0 {
			t.Fatalf("sync dry-run result = %+v", got)
		}
	default:
		t.Fatalf("unsupported command %q", command)
	}
}

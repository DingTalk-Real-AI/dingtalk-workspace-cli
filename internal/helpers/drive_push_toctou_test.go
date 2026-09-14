package helpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// replacePinnedMirrorRoot 返回 true 表示原目录被真实移走、原名处已换成另一个目录；
// 返回 false 表示走了 Windows 身份注入降级，原目录仍留在原处。依赖「替代目录内容」
// 的断言必须按返回值分流。
func replacePinnedMirrorRoot(t *testing.T, root, moved string) bool {
	t.Helper()
	if forcePinnedFallbackForTest {
		swapPinnedRootIdentity(t, root)
		return false
	}
	if err := os.Rename(root, moved); err != nil {
		// Windows 在目录内有已打开的句柄（如上传读取中的源文件）时会锁住该目录，
		// 移走固定根于该平台物理不可达。退化为等价的根身份注入，命中同一条
		// fail-closed 分支。
		if runtime.GOOS != "windows" {
			t.Fatalf("move pinned root: %v", err)
		}
		swapPinnedRootIdentity(t, root)
		return false
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("create replacement root: %v", err)
	}
	mustWrite(t, filepath.Join(root, "a.txt"), "outside-replacement")
	return true
}

// 本地扫描完成后、任何 create_folder/get_upload_info/commit_upload 前若命令行根已
// 被另一目录替换，push 与 sync 都必须整批失败，不能把替代目录当成扫描快照继续写远端。
func TestCrossPlatformCoverageDrivePushSyncRejectRootReplacementBeforeRemoteWrite(t *testing.T) {
	for _, command := range []string{"push", "sync"} {
		t.Run(command, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "root")
			moved := filepath.Join(parent, "moved-original")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(root, "a.txt"), "pinned-original")

			replaced := false
			realMove := false
			caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
				if tool == "list_files" {
					if !replaced {
						realMove = replacePinnedMirrorRoot(t, root, moved)
						replaced = true
					}
					return `{"result":{"items":[],"nextToken":""}}`, nil
				}
				return `{"result":{"fileId":"UNEXPECTED"},"success":true}`, nil
			}}
			putCalls := 0
			testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
				putCalls++
				return nil
			})

			args := []string{command, "--local-folder", root, "--remote-folder", "ROOT"}
			if command == "sync" {
				args = append(args, "--quick")
			}
			err := runDriveCmd(t, caller, args...)
			if err == nil || !strings.Contains(err.Error(), "本地根目录在同步期间被替换") {
				t.Fatalf("expected root replacement rejection, got %v", err)
			}
			for _, tool := range []string{"create_folder", "get_upload_info", "commit_upload"} {
				if got := caller.callsFor(tool); len(got) != 0 {
					t.Errorf("%s must not run after root replacement: %v", tool, got)
				}
			}
			if putCalls != 0 {
				t.Fatalf("OSS PUT must not run after root replacement, got %d", putCalls)
			}
			want := "outside-replacement"
			if !realMove {
				// 身份注入降级：原目录仍在原处，内容同样不得被镜像流程改写。
				want = "pinned-original"
			}
			if b, err := os.ReadFile(filepath.Join(root, "a.txt")); err != nil || string(b) != want {
				t.Fatalf("replacement tree changed: %q err=%v", string(b), err)
			}
		})
	}
}

// 即使根在 get_upload_info 之后、OSS PUT 读取前被替换，PUT 也只能读取扫描时固定根
// 打开的文件句柄。传输后身份复核会拒绝 commit，替代目录内容绝不能泄漏到上传流。
func TestCrossPlatformCoverageDrivePushSyncUploadReadsPinnedFileHandle(t *testing.T) {
	for _, command := range []string{"push", "sync"} {
		t.Run(command, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "root")
			moved := filepath.Join(parent, "moved-original")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(root, "a.txt"), "pinned-original")

			caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
				switch tool {
				case "list_files":
					return `{"result":{"items":[],"nextToken":""}}`, nil
				case "get_upload_info":
					return `{"result":{"resourceUrls":[{"url":"https://oss.example.com/put","headers":{}}],"uploadId":"U1"}}`, nil
				}
				return `{"result":{"fileId":"NEW"},"success":true}`, nil
			}}

			var uploaded string
			putCalls := 0
			realMove := false
			testseam.Swap(t, &pushPutOpenedFile, func(_ context.Context, _ string, _ map[string]string, file *os.File, _ int64) error {
				putCalls++
				realMove = replacePinnedMirrorRoot(t, root, moved)
				body, err := io.ReadAll(file)
				if err != nil {
					return err
				}
				uploaded = string(body)
				return nil
			})

			args := []string{command, "--local-folder", root, "--remote-folder", "ROOT"}
			if command == "sync" {
				args = append(args, "--quick")
			}
			err := runDriveCmd(t, caller, args...)
			if err == nil {
				t.Fatal("root replacement during PUT must prevent commit")
			}
			if putCalls != 1 || uploaded != "pinned-original" {
				t.Fatalf("PUT read %q in %d call(s), want pinned-original once", uploaded, putCalls)
			}
			if commits := caller.callsFor("commit_upload"); len(commits) != 0 {
				t.Fatalf("root replacement must prevent commit_upload: %v", commits)
			}
			if realMove {
				if b, readErr := os.ReadFile(filepath.Join(root, "a.txt")); readErr != nil || string(b) != "outside-replacement" {
					t.Fatalf("replacement file was read or changed: %q err=%v", string(b), readErr)
				}
				if b, readErr := os.ReadFile(filepath.Join(moved, "a.txt")); readErr != nil || string(b) != "pinned-original" {
					t.Fatalf("pinned original changed: %q err=%v", string(b), readErr)
				}
			} else if b, readErr := os.ReadFile(filepath.Join(root, "a.txt")); readErr != nil || string(b) != "pinned-original" {
				// 身份注入降级：原目录仍在原处，内容同样不得被上传路径改写。
				t.Fatalf("pinned original changed: %q err=%v", string(b), readErr)
			}
		})
	}
}

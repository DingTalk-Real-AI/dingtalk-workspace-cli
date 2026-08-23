package helpers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDriveRemoteTree_unsafeNamesFailClosed(t *testing.T) {
	tests := []struct {
		name     string
		listing  string
		wantKind string
	}{
		{
			name:     "file name containing a backslash",
			listing:  `{"result":{"items":[{"name":"a\\b.txt","type":"file","fileId":"UNSAFE_FILE"}],"nextToken":""}}`,
			wantKind: "文件",
		},
		{
			name:     "folder name escaping its parent",
			listing:  `{"result":{"items":[{"name":"..","type":"folder","fileId":"UNSAFE_FOLDER"}],"nextToken":""}}`,
			wantKind: "目录",
		},
	}

	commands := []struct {
		name    string
		args    func(localRoot string) []string
		prepare func(t *testing.T, localRoot string)
	}{
		{
			name: "status",
			args: func(localRoot string) []string {
				return []string{"status", "--local-folder", localRoot, "--remote-folder", "ROOT", "--quick"}
			},
			prepare: func(t *testing.T, localRoot string) {
				t.Helper()
				if err := os.MkdirAll(localRoot, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "pull",
			args: func(localRoot string) []string {
				return []string{"pull", "--local-folder", localRoot, "--remote-folder", "ROOT"}
			},
		},
		{
			name: "push",
			args: func(localRoot string) []string {
				return []string{"push", "--local-folder", localRoot, "--remote-folder", "ROOT"}
			},
			prepare: prepareUnsafeNameWriteSource,
		},
		{
			name: "sync",
			args: func(localRoot string) []string {
				return []string{"sync", "--local-folder", localRoot, "--remote-folder", "ROOT", "--quick"}
			},
			prepare: prepareUnsafeNameWriteSource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, command := range commands {
				t.Run(command.name, func(t *testing.T) {
					localRoot := filepath.Join(t.TempDir(), "mirror")
					if command.prepare != nil {
						command.prepare(t, localRoot)
					}

					caller := &driveScriptCaller{reply: func(tool string, args map[string]any, nth int) (string, error) {
						if tool != "list_files" || nth != 0 || args["parentId"] != "ROOT" {
							t.Fatalf("unsafe remote entry must stop before recursion: tool=%s nth=%d args=%v", tool, nth, args)
						}
						return tt.listing, nil
					}}
					getCalls, putCalls := 0, 0
					swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
						getCalls++
						return nil
					})
					testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
						putCalls++
						return nil
					})

					err := runDriveCmd(t, caller, command.args(localRoot)...)
					if err == nil || !strings.Contains(err.Error(), "无法安全映射到本地路径") ||
						!strings.Contains(err.Error(), tt.wantKind) {
						t.Fatalf("error = %v, want unsafe remote %s name failure", err, tt.wantKind)
					}
					if got := len(caller.callsFor("list_files")); got != 1 {
						t.Fatalf("list_files calls = %d, want only the root query", got)
					}
					for _, tool := range []string{"create_folder", "download_file", "get_upload_info", "commit_upload"} {
						if calls := caller.callsFor(tool); len(calls) != 0 {
							t.Fatalf("unsafe remote name must not call %s: %v", tool, calls)
						}
					}
					if getCalls != 0 || putCalls != 0 {
						t.Fatalf("unsafe remote name must not transfer content: GET=%d PUT=%d", getCalls, putCalls)
					}
					if command.name == "pull" {
						if _, statErr := os.Stat(localRoot); !os.IsNotExist(statErr) {
							t.Fatalf("pull must fail before creating its local root: %v", statErr)
						}
					}
				})
			}
		})
	}
}

func prepareUnsafeNameWriteSource(t *testing.T, localRoot string) {
	t.Helper()
	mustWrite(t, filepath.Join(localRoot, "local.txt"), "local")
}

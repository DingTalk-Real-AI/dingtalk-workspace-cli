package helpers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageParseDriveList_requiresAuthoritativeResult(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
		wantItems int
	}{
		{name: "missing result", body: `{}`, wantError: "缺少 result"},
		{name: "null result", body: `{"result":null}`, wantError: "result 为 null"},
		{name: "empty result object", body: `{"result":{}}`, wantError: "缺少 items"},
		{name: "null item", body: `{"result":{"items":[null]}}`, wantError: "缺少有效 type"},
		{name: "empty item object", body: `{"result":{"items":[{}]}}`, wantError: "缺少有效 type"},
		{name: "item missing type", body: `{"result":{"items":[{"name":"unknown"}]}}`, wantError: "缺少有效 type"},
		{name: "item empty type", body: `{"result":{"items":[{"name":"unknown","type":"  "}]}}`, wantError: "缺少有效 type"},
		{name: "has more without token", body: `{"result":{"items":[],"hasMore":true,"nextToken":""}}`, wantError: "hasMore=true"},
		{name: "empty items", body: `{"result":{"items":[]}}`},
		{name: "empty result array", body: `{"result":[]}`},
		{name: "explicit unsupported type remains parseable", body: `{"result":{"items":[{"name":"online.adoc","type":"document"}]}}`, wantItems: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, token, err := parseDriveList(tt.body)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("parseDriveList error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDriveList(%s): %v", tt.body, err)
			}
			if len(items) != tt.wantItems || token != "" {
				t.Fatalf("parseDriveList(%s) = (%v, %q), want %d authoritative item(s) with no token", tt.body, items, token, tt.wantItems)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveRemoteTree_integrityFailuresStopFourCommandsBeforeWrites(t *testing.T) {
	failures := []struct {
		name          string
		wantError     string
		wantListCalls int
		reply         func(int) string
	}{
		{
			name:          "missing result",
			wantError:     "缺少 result",
			wantListCalls: 1,
			reply:         func(int) string { return `{}` },
		},
		{
			name:          "null result",
			wantError:     "result 为 null",
			wantListCalls: 1,
			reply:         func(int) string { return `{"result":null}` },
		},
		{
			name:          "empty result object",
			wantError:     "缺少 items",
			wantListCalls: 1,
			reply:         func(int) string { return `{"result":{}}` },
		},
		{
			name:          "null item",
			wantError:     "缺少有效 type",
			wantListCalls: 1,
			reply:         func(int) string { return `{"result":{"items":[null],"nextToken":""}}` },
		},
		{
			name:          "empty item object",
			wantError:     "缺少有效 type",
			wantListCalls: 1,
			reply:         func(int) string { return `{"result":{"items":[{}],"nextToken":""}}` },
		},
		{
			name:          "item missing type",
			wantError:     "缺少有效 type",
			wantListCalls: 1,
			reply: func(int) string {
				return `{"result":{"items":[{"name":"unknown"}],"nextToken":""}}`
			},
		},
		{
			name:          "item empty type",
			wantError:     "缺少有效 type",
			wantListCalls: 1,
			reply: func(int) string {
				return `{"result":{"items":[{"name":"unknown","type":""}],"nextToken":""}}`
			},
		},
		{
			name:          "has more without token",
			wantError:     "hasMore=true",
			wantListCalls: 1,
			reply: func(int) string {
				return `{"result":{"items":[],"hasMore":true,"nextToken":""}}`
			},
		},
		{
			name:          "file without an ID",
			wantError:     "缺少文件 ID",
			wantListCalls: 1,
			reply: func(int) string {
				return `{"result":{"items":[{"name":"remote.txt","type":"file"}],"nextToken":""}}`
			},
		},
		{
			name:          "file name with an ASCII control",
			wantError:     "无法安全映射到本地路径",
			wantListCalls: 1,
			reply: func(int) string {
				return `{"result":{"items":[{"name":"remote\u0000.txt","type":"file","fileId":"REMOTE"}],"nextToken":""}}`
			},
		},
		{
			name:          "pagination token cycle",
			wantError:     "分页 token",
			wantListCalls: 3,
			reply: func(nth int) string {
				tokens := []string{"PAGE_A", "PAGE_B", "PAGE_A"}
				if nth >= len(tokens) {
					return `{"result":{"items":[],"nextToken":""}}`
				}
				return fmt.Sprintf(`{"result":{"items":[],"nextToken":%q}}`, tokens[nth])
			},
		},
	}

	commands := []struct {
		name    string
		args    func(string) []string
		prepare func(*testing.T, string)
	}{
		{
			name: "status",
			args: func(localRoot string) []string {
				return []string{"status", "--local-folder", localRoot, "--remote-folder", "ROOT", "--quick"}
			},
			prepare: prepareRemoteTreeReadRoot,
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
			prepare: prepareRemoteTreeWriteSource,
		},
		{
			name: "sync",
			args: func(localRoot string) []string {
				return []string{"sync", "--local-folder", localRoot, "--remote-folder", "ROOT", "--quick"}
			},
			prepare: prepareRemoteTreeWriteSource,
		},
	}

	for _, failure := range failures {
		t.Run(failure.name, func(t *testing.T) {
			for _, command := range commands {
				t.Run(command.name, func(t *testing.T) {
					localRoot := filepath.Join(t.TempDir(), "mirror")
					if command.prepare != nil {
						command.prepare(t, localRoot)
					}

					caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, nth int) (string, error) {
						if tool != "list_files" {
							return "", fmt.Errorf("远端清单预检完成前调用了写工具 %s", tool)
						}
						return failure.reply(nth), nil
					}}
					getCalls, putCalls := swapRemoteTreeTransferCounters(t)

					err := runDriveCmd(t, caller, command.args(localRoot)...)
					if err == nil || !strings.Contains(err.Error(), failure.wantError) {
						t.Fatalf("error = %v, want remote tree integrity failure containing %q", err, failure.wantError)
					}
					if got := len(caller.callsFor("list_files")); got != failure.wantListCalls {
						t.Fatalf("list_files calls = %d, want %d", got, failure.wantListCalls)
					}
					for _, call := range caller.calls {
						if call.toolName != "list_files" {
							t.Fatalf("remote tree integrity failure must stop before MCP writes, got %s", call.toolName)
						}
					}
					if *getCalls != 0 || *putCalls != 0 {
						t.Fatalf("remote tree integrity failure must stop before transfers: GET=%d PUT=%d", *getCalls, *putCalls)
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

func TestCrossPlatformCoverageDriveRemoteTree_validEmptyListsRemainSupported(t *testing.T) {
	for _, listing := range []struct {
		name string
		body string
	}{
		{name: "items object", body: `{"result":{"items":[]}}`},
		{name: "result array", body: `{"result":[]}`},
	} {
		t.Run(listing.name, func(t *testing.T) {
			for _, fetcher := range []struct {
				name  string
				fetch func(context.Context) error
			}{
				{
					name: "status and pull walker",
					fetch: func(ctx context.Context) error {
						files, err := fetchRemoteDriveTree(ctx, "", "ROOT", true)
						if len(files) != 0 {
							return fmt.Errorf("remote files = %v, want empty", files)
						}
						return err
					},
				},
				{
					name: "push and sync walker",
					fetch: func(ctx context.Context) error {
						files, folders, err := fetchRemoteTreeForPush(ctx, "", "ROOT")
						if len(files) != 0 || len(folders) != 1 || folders[""] != "ROOT" {
							return fmt.Errorf("remote tree = files %v folders %v, want empty root", files, folders)
						}
						return err
					},
				},
			} {
				t.Run(fetcher.name, func(t *testing.T) {
					caller := &driveScriptCaller{reply: func(tool string, _ map[string]any, _ int) (string, error) {
						if tool != "list_files" {
							t.Fatalf("unexpected tool %s", tool)
						}
						return listing.body, nil
					}}
					testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
					testseam.Swap(t, &os.Args, []string{"dws", "drive"})
					if err := fetcher.fetch(context.Background()); err != nil {
						t.Fatalf("valid empty listing: %v", err)
					}
					if got := len(caller.callsFor("list_files")); got != 1 {
						t.Fatalf("list_files calls = %d, want 1", got)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageIsSafeRemoteSegment_rejectsASCIIControls(t *testing.T) {
	for _, name := range []string{"nul\x00name", "unit\x1fseparator", "delete\x7fcharacter"} {
		if isSafeRemoteSegment(name) {
			t.Errorf("isSafeRemoteSegment(%q) = true, want false", name)
		}
	}
}

func prepareRemoteTreeReadRoot(t *testing.T, localRoot string) {
	t.Helper()
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
}

func prepareRemoteTreeWriteSource(t *testing.T, localRoot string) {
	t.Helper()
	mustWrite(t, filepath.Join(localRoot, "local.txt"), "local")
}

func swapRemoteTreeTransferCounters(t *testing.T) (*int, *int) {
	t.Helper()
	getCalls, putCalls := 0, 0
	swapPullDownloadPath(t, func(context.Context, string, map[string]string, string) error {
		getCalls++
		return nil
	})
	testseam.Swap(t, &pushPutOpenedFile, func(context.Context, string, map[string]string, *os.File, int64) error {
		putCalls++
		return nil
	})
	return &getCalls, &putCalls
}

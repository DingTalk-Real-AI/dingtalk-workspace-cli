package helpers

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDriveRemoteTree_folderIDFallbacks(t *testing.T) {
	for _, key := range []string{"fileId", "dentryUuid", "dentryId", "id", "nodeId"} {
		t.Run(key, func(t *testing.T) {
			caller := &driveScriptCaller{reply: func(_ string, args map[string]any, _ int) (string, error) {
				if args["parentId"] == "ROOT" {
					return `{"result":{"items":[{"name":"sub","type":"folder","` + key + `":"SUB"}],"nextToken":""}}`, nil
				}
				if args["parentId"] != "SUB" {
					t.Fatalf("nested parentId = %v, want SUB", args["parentId"])
				}
				return `{"result":{"items":[],"nextToken":""}}`, nil
			}}
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})

			if _, err := fetchRemoteDriveTree(context.Background(), "", "ROOT", true); err != nil {
				t.Fatalf("fetch with %s: %v", key, err)
			}
			if got := len(caller.callsFor("list_files")); got != 2 {
				t.Fatalf("list_files calls = %d, want root + nested", got)
			}
		})
	}
}

func TestCrossPlatformCoverageDriveRemoteTree_missingFolderIDFailsClosed(t *testing.T) {
	for _, fetcher := range []struct {
		name  string
		fetch func(context.Context) error
	}{
		{
			name: "status and pull tree",
			fetch: func(ctx context.Context) error {
				_, err := fetchRemoteDriveTree(ctx, "", "ROOT", true)
				return err
			},
		},
		{
			name: "push and sync tree",
			fetch: func(ctx context.Context) error {
				_, _, err := fetchRemoteTreeForPush(ctx, "", "ROOT")
				return err
			},
		},
	} {
		t.Run(fetcher.name, func(t *testing.T) {
			caller := &driveScriptCaller{reply: func(_ string, args map[string]any, nth int) (string, error) {
				if nth > 0 || args["parentId"] != "ROOT" {
					t.Fatalf("missing folder ID must not trigger another list_files call: nth=%d args=%v", nth, args)
				}
				return `{"result":{"items":[{"name":"sub","type":"folder"}],"nextToken":""}}`, nil
			}}
			testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
			testseam.Swap(t, &os.Args, []string{"dws", "drive"})

			err := fetcher.fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), "目录 ID") || !strings.Contains(err.Error(), "sub") {
				t.Fatalf("error = %v, want missing folder ID for sub", err)
			}
			if got := len(caller.callsFor("list_files")); got != 1 {
				t.Fatalf("list_files calls = %d, want only root query", got)
			}
		})
	}
}

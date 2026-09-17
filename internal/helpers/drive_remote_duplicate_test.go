package helpers

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDriveRemoteTree_duplicatePathsFail(t *testing.T) {
	tests := []struct {
		name  string
		reply func(string, map[string]any, int) (string, error)
		path  string
	}{
		{
			name: "same file path across pages",
			path: "duplicate.txt",
			reply: func(_ string, args map[string]any, _ int) (string, error) {
				if _, ok := args["nextToken"]; !ok {
					return `{"result":{"items":[{"name":"duplicate.txt","type":"file","fileId":"F1"}],"nextToken":"PAGE2"}}`, nil
				}
				return `{"result":{"items":[{"name":"duplicate.txt","type":"file","fileId":"F2"}],"nextToken":""}}`, nil
			},
		},
		{
			name: "same folder path with duplicate descendants",
			path: "duplicate",
			reply: func(_ string, args map[string]any, _ int) (string, error) {
				switch args["parentId"] {
				case "ROOT":
					return `{"result":{"items":[` +
						`{"name":"duplicate","type":"folder","fileId":"D1"},` +
						`{"name":"duplicate","type":"folder","fileId":"D2"}` +
						`],"nextToken":""}}`, nil
				case "D1":
					return `{"result":{"items":[{"name":"child.txt","type":"file","fileId":"C1"}],"nextToken":""}}`, nil
				case "D2":
					return `{"result":{"items":[{"name":"child.txt","type":"file","fileId":"C2"}],"nextToken":""}}`, nil
				default:
					return `{"result":{"items":[],"nextToken":""}}`, nil
				}
			},
		},
	}

	fetchers := []struct {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, fetcher := range fetchers {
				t.Run(fetcher.name, func(t *testing.T) {
					caller := &driveScriptCaller{reply: tt.reply}
					testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard}})
					testseam.Swap(t, &os.Args, []string{"dws", "drive"})

					err := fetcher.fetch(context.Background())
					if err == nil || !strings.Contains(err.Error(), "重复远端路径") || !strings.Contains(err.Error(), tt.path) {
						t.Fatalf("error = %v, want duplicate remote path %q", err, tt.path)
					}
				})
			}
		})
	}
}

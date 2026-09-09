package helpers

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageDriveSearchPaginationBoundary(t *testing.T) {
	for _, target := range []string{"", "all", "file", "space"} {
		for _, flag := range []string{"", "cursor", "page-token"} {
			t.Run(target+"/"+flag, func(t *testing.T) {
				caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"items":[]}`}, {text: `{"documents":[]}`}}}
				testseam.Swap(t, &deps, &Deps{Caller: caller, Out: &Formatter{w: io.Discard, errW: io.Discard}})
				testseam.Swap(t, &os.Args, []string{"dws", "drive"})
				root := newDriveCommand()
				installExampleGlobalFlags(root)
				root.SilenceErrors, root.SilenceUsage = true, true
				args := []string{"search", "--query", "fixture"}
				if target != "" {
					args = append(args, "--target", target)
				}
				if flag != "" {
					args = append(args, "--"+flag, "own-cursor")
				}
				root.SetArgs(args)
				err := root.Execute()
				aggregate := target == "" || target == "all"
				if aggregate && flag != "" {
					if err == nil || !strings.Contains(err.Error(), "聚合搜索不支持续页") {
						t.Fatalf("error=%v", err)
					}
					if caller.calls != 0 {
						t.Fatalf("rejected request made %d calls", caller.calls)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				wantCalls := 1
				if aggregate {
					wantCalls = 2
				}
				if caller.calls != wantCalls {
					t.Fatalf("calls=%d want=%d", caller.calls, wantCalls)
				}
				if flag != "" && caller.argsLog[0]["pageToken"] != "own-cursor" {
					t.Fatalf("cursor lost: %v", caller.argsLog)
				}
				if flag == "" {
					for _, params := range caller.argsLog {
						if _, found := params["pageToken"]; found {
							t.Fatal("unexpected cursor")
						}
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDriveSearchPaginationHelp(t *testing.T) {
	root := newDriveCommand()
	cmd, _, err := root.Find([]string{"search"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd.Long, "默认/all 仅支持首页聚合") {
		t.Fatal("missing aggregation boundary")
	}
	if !strings.Contains(cmd.Flags().Lookup("cursor").Usage, "--target file/space") {
		t.Fatal("missing cursor scope")
	}
	if !strings.Contains(cmd.Example, `--target file --limit 30 --cursor`) {
		t.Fatal("unsafe pagination example")
	}
}

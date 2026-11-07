package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
)

func TestCrossPlatformCoverageDeapAgentVisibilityValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     []string
		wantError string
	}{
		{"all", []string{"--visibility", "ALL"}, ""},
		{"users", []string{"--visibility", "PARTIAL", "--user-ids", "u1,u2"}, ""},
		{"departments", []string{"--visibility", "PARTIAL", "--dept-ids", "100"}, ""},
		{"mixed", []string{"--visibility", "PARTIAL", "--user-ids", "u1", "--dept-ids", "100"}, ""},
		{"legacy_users", []string{"--visibility", "PARTIAL", "--staff-ids", "u1"}, ""},
		{"invalid_enum", []string{"--visibility", "invalid"}, "visibility"},
		{"empty_enum", []string{"--visibility", " "}, "visibility"},
		{"partial_empty", []string{"--visibility", "PARTIAL"}, "至少包含一个非空 ID"},
		{"partial_blank_users", []string{"--visibility", "PARTIAL", "--user-ids", " , "}, "至少包含一个非空 ID"},
		{"partial_blank_both", []string{"--visibility", "PARTIAL", "--user-ids", "", "--dept-ids", " "}, "至少包含一个非空 ID"},
		{"all_users", []string{"--visibility", "ALL", "--user-ids", "u1"}, "请移除"},
		{"all_departments", []string{"--visibility", "ALL", "--dept-ids", "100"}, "请移除"},
	} {
		for _, dryRun := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/dry_run=%t", tc.name, dryRun), func(t *testing.T) {
				caller, out := newDeapAgentTestTree(t, dryRun)
				root := deapHandler{}.Command(&captureRunner{})
				root.PersistentFlags().Bool("yes", false, "test confirmation")
				root.PersistentFlags().Bool("dry-run", false, "test preview")
				argv := append([]string{"manage", "set-visibility", "--agent-uuid", "agent-1"}, tc.flags...)
				if dryRun {
					argv = append(argv, "--dry-run")
				} else {
					argv = append(argv, "--yes")
				}
				root.SetArgs(argv)
				err := corecmd.ExecuteForTest(root)
				if tc.wantError != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("error = %v; want %q", err, tc.wantError)
					}
					if len(caller.calls) != 0 {
						t.Fatalf("invalid input made %d remote calls", len(caller.calls))
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if dryRun {
					if len(caller.calls) != 0 {
						t.Fatal("dry-run made remote calls")
					}
					var result map[string]any
					if err := json.Unmarshal(out.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if result["dry_run"] != true || result["executed"] != false {
						t.Fatalf("preview = %v", result)
					}
				} else if len(caller.calls) != 1 || caller.calls[0].toolName != deapAgentSetVisibilityTool {
					t.Fatalf("unexpected calls = %v", caller.calls)
				}
			})
		}
	}
}

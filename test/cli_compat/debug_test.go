package cli_compat_test

import (
	"bytes"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

func TestDebugAitableTableCreate(t *testing.T) {
	_ = setupTestDeps(t, "aitable")
	root := app.NewRootCommand()

	cliArgs := []string{"-f", "json", "aitable", "table", "create",
		"--base-id", "B1", "--name", "任务表",
		"--fields", `[{"fieldName":"名称","type":"text"}]`,
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(cliArgs)

	err := root.Execute()
	t.Logf("Execute error: %v", err)
	t.Logf("Stdout: [%s]", out.String())
	t.Logf("Stderr: [%s]", errOut.String())

	// Check all aitable subcommands
	aitableCmd, _, _ := root.Find([]string{"aitable"})
	if aitableCmd != nil {
		t.Logf("aitable subcommands:")
		for _, grp := range aitableCmd.Commands() {
			t.Logf("  %s:", grp.Use)
			for _, sub := range grp.Commands() {
				t.Logf("    %s (hidden=%v)", sub.Use, sub.Hidden)
			}
		}
	}
}

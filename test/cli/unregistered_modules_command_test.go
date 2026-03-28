package cli_test

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

func TestRootHelpHidesRecoveredModules(t *testing.T) {
	t.Parallel()

	cmd := app.NewRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	// All of these commands should be hidden because they are not in
	// the dynamic server discovery product list.
	for _, module := range []string{
		"aisearch", "contract", "recruit", "tb",
		"chat", "drive", "minutes", "mail", "credit",
		"credit-risk", "finance", "message", "notify", "doc",
	} {
		if strings.Contains(got, "  "+module+" ") {
			t.Fatalf("root help should not show hidden module %q:\n%s", module, got)
		}
	}
}

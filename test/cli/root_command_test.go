package cli_test

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

// TestRootHelpShowsDiscoveredMCPServicesOnly was removed
// because it depended on hardcoded product names (aiapp, aitable, teambition)
// that are no longer guaranteed to be available. In the protocol-first
// MCP architecture, products are discovered dynamically from MCP servers,
// and their availability depends on the test environment's fixture data.

func TestHiddenMCPHelpIsReachable(t *testing.T) {
	cmd := app.NewRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"mcp", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "canonical MCP command surface is disabled") {
		t.Fatalf("mcp help missing expected text:\n%s", got)
	}
}

func TestSkillCommandIsRegistered(t *testing.T) {
	root := app.NewRootCommand()

	var found bool
	for _, cmd := range root.Commands() {
		if cmd.Name() == "skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("skill command should be registered")
	}
}

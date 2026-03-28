package cli_test

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

func TestMCPSubcommandIsHidden(t *testing.T) {
	t.Parallel()

	root := app.NewRootCommand()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "mcp" {
			if !cmd.Hidden {
				t.Fatal("mcp sub-command should remain hidden on root")
			}
			return
		}
	}
	t.Fatal("mcp sub-command should remain registered for hidden/internal flows")
}

func TestSkillCommandIsNotRegisteredInPublicOSSBuild(t *testing.T) {
	t.Parallel()

	root := app.NewRootCommand()

	for _, cmd := range root.Commands() {
		if cmd.Name() == "skill" {
			t.Fatalf("skill command should not be registered in OSS build")
		}
	}
}

func TestAuthLoginCommandIsRegisteredInPublicOSSBuild(t *testing.T) {
	t.Parallel()

	root := app.NewRootCommand()

	for _, cmd := range root.Commands() {
		if cmd.Name() != "auth" {
			continue
		}
		for _, sub := range cmd.Commands() {
			if sub.Name() == "login" {
				return
			}
		}
		t.Fatal("auth login command should be registered in OSS build")
	}

	t.Fatal("auth command should be registered on root")
}

func TestCacheStatusJSONBootstrapOutput(t *testing.T) {
	t.Parallel()

	cmd := app.NewRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"cache", "status", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\"kind\": \"cache_status\"") {
		t.Fatalf("cache status output missing JSON payload:\n%s", got)
	}
}

package helpers

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoveragePublicProductCommandsBuildCompleteUniqueTrees(t *testing.T) {
	commands := NewPublicCommands(&captureRunner{})
	if len(commands) == 0 {
		t.Fatal("NewPublicCommands() returned no commands")
	}

	seenProducts := make(map[string]bool, len(commands))
	for _, command := range commands {
		if command == nil {
			t.Fatal("NewPublicCommands() returned a nil command")
		}
		name := command.Name()
		if name == "" {
			t.Fatal("public command has an empty name")
		}
		if seenProducts[name] {
			t.Fatalf("duplicate public command %q", name)
		}
		seenProducts[name] = true
		assertCommandTree(t, command, make(map[*cobra.Command]bool))
	}

	for _, want := range []string{
		"agoal", "aisearch", "aitable", "attendance", "calendar", "chat",
		"contact", "devdoc", "ding", "doc", "drive", "live", "mail",
		"minutes", "oa", "report", "sheet", "todo", "wiki",
	} {
		if !seenProducts[want] {
			t.Errorf("public product %q was not registered", want)
		}
	}
}

func assertCommandTree(t *testing.T, command *cobra.Command, seen map[*cobra.Command]bool) {
	t.Helper()
	if seen[command] {
		t.Fatalf("command tree contains a cycle at %q", command.CommandPath())
	}
	seen[command] = true

	seenNames := make(map[string]bool, len(command.Commands()))
	for _, child := range command.Commands() {
		if child.Parent() != command {
			t.Errorf("command %q has parent %q, want %q", child.Name(), child.Parent().Name(), command.Name())
		}
		if seenNames[child.Name()] {
			t.Errorf("command %q contains duplicate child %q", command.CommandPath(), child.Name())
		}
		seenNames[child.Name()] = true
		assertCommandTree(t, child, seen)
	}
}

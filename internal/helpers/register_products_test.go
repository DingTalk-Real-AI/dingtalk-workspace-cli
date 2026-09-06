package helpers

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type countedHandler struct {
	name string
}

func TestPublicRouteIndexMatchesRegisteredFactoryAuthority(t *testing.T) {
	want := make(map[string]string)
	for _, registered := range publicFactories {
		if _, exists := want[registered.name]; !exists {
			want[registered.name] = registered.name
		}
		for _, alias := range registered.aliases {
			if _, exists := want[alias]; !exists {
				want[alias] = registered.name
			}
		}
	}
	if !reflect.DeepEqual(publicIndex, want) {
		t.Fatalf("public route index = %#v, want %#v", publicIndex, want)
	}
}

func BenchmarkResolvePublicCommand(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if canonical, ok := ResolvePublicCommand("im"); !ok || canonical != "chat" {
			b.Fatal("public route index lost chat alias")
		}
	}
}

func BenchmarkResolvePublicCommandLinear(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		canonical := ""
		for _, registered := range publicFactories {
			if registered.name == "im" {
				canonical = registered.name
				break
			}
			for _, alias := range registered.aliases {
				if alias == "im" {
					canonical = registered.name
					break
				}
			}
			if canonical != "" {
				break
			}
		}
		if canonical != "chat" {
			b.Fatal("linear registry lost chat alias")
		}
	}
}

func (h countedHandler) Name() string { return h.name }

func (h countedHandler) Command(executor.Runner) *cobra.Command {
	return &cobra.Command{Use: h.name}
}

func TestSelectedPublicProductDoesNotConstructUnrelatedFactories(t *testing.T) {
	calls := map[string]int{}
	factory := func(name string) Factory {
		return func() Handler {
			calls[name]++
			return countedHandler{name: name}
		}
	}
	factories := []registeredFactory{
		{name: "calendar", factory: factory("calendar")},
		{name: "drive", factory: factory("drive")},
	}

	commands := buildCommands(factories, &captureRunner{}, "calendar")
	if len(commands) != 1 || commands[0].Name() != "calendar" {
		t.Fatalf("selected commands = %#v, want calendar only", commands)
	}
	if calls["calendar"] != 1 || calls["drive"] != 0 {
		t.Fatalf("factory calls = %#v, want calendar=1 and drive=0", calls)
	}
}

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
		resolved, ok := ResolvePublicCommand(name)
		if !ok || resolved != name {
			t.Fatalf("registered product %q does not resolve to itself", name)
		}
		for _, alias := range command.Aliases {
			resolved, ok := ResolvePublicCommand(alias)
			if !ok || resolved != name {
				t.Fatalf("top-level alias %q resolves to %q, %v; want %q", alias, resolved, ok, name)
			}
		}
		assertCommandTree(t, command, make(map[*cobra.Command]bool))
	}

	for _, want := range []string{
		"agoal", "aisearch", "aitable", "attendance", "calendar", "chat",
		"contact", "devdoc", "ding", "doc", "drive", "html", "live", "mail",
		"markdown", "minutes", "oa", "recruit", "report", "sheet", "todo", "wiki", "whiteboard",
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

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/interfacesnapshot"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestInspectRuntimeCommandSurfaceUsesBoundedEligibilityIO(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "/bounded-config")
	var lstatCalls, readDirCalls, readFileCalls int
	testseam.Swap(t, &startupRouteLstat, func(name string) (os.FileInfo, error) {
		lstatCalls++
		if name != "/bounded-config/shortcuts" {
			t.Fatalf("Lstat path = %q", name)
		}
		return nil, fs.ErrNotExist
	})
	testseam.Swap(t, &startupRouteReadDir, func(name string) ([]os.DirEntry, error) {
		readDirCalls++
		if name != "/bounded-config/plugins/user" {
			t.Fatalf("ReadDir path = %q", name)
		}
		return nil, fs.ErrNotExist
	})
	testseam.Swap(t, &startupRouteReadFile, func(name string) ([]byte, error) {
		readFileCalls++
		if name != "/bounded-config/settings.json" {
			t.Fatalf("ReadFile path = %q", name)
		}
		return nil, fs.ErrNotExist
	})

	surface := inspectRuntimeCommandSurface()
	if !surface.selectiveStartup || !surface.pluginsProvenAbsent {
		t.Fatalf("clean surface = %#v", surface)
	}
	if lstatCalls != 1 || readDirCalls != 1 || readFileCalls != 1 {
		t.Fatalf("eligibility I/O = lstat:%d readdir:%d readfile:%d, want 1 each", lstatCalls, readDirCalls, readFileCalls)
	}
}

func TestResolveStartupRouteHandlesRootFlagsWithoutGuessingUnknownSyntax(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().StringP("profile", "p", "", "profile")
	root.PersistentFlags().BoolP("verbose", "v", false, "verbose")
	root.Flags().Bool("version", false, "version")

	for _, tc := range []struct {
		name string
		args []string
		want startupRoute
	}{
		{name: "plain", args: []string{"calendar", "book", "list"}, want: startupRoute{topLevel: "calendar", selective: true}},
		{name: "long value", args: []string{"--profile", "corp:user", "calendar", "book", "list"}, want: startupRoute{topLevel: "calendar", selective: true}},
		{name: "short attached", args: []string{"-pcorp:user", "calendar"}, want: startupRoute{topLevel: "calendar", selective: true}},
		{name: "boolean cluster", args: []string{"-v", "calendar"}, want: startupRoute{topLevel: "calendar", selective: true}},
		{name: "version", args: []string{"--version"}, want: startupRoute{selective: true}},
		{name: "help needs full surface", args: []string{"--help"}},
		{name: "completion script needs full surface", args: []string{"completion", "zsh"}},
		{name: "completion request needs full surface", args: []string{"__complete", "calendar"}},
		{name: "completion request without descriptions needs full surface", args: []string{"__completeNoDesc", "calendar"}},
		{name: "unknown flag falls back", args: []string{"--future", "calendar"}},
		{name: "separator falls back", args: []string{"--", "calendar"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveStartupRoute(root, tc.args); got != tc.want {
				t.Fatalf("route = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestProcessRootSkipsPluginLoaderOnlyAfterAbsenceProof(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("DO_NOT_TRACK", "1")
	testseam.Swap(t, &os.Args, []string{"dws", "calendar", "book", "list", "--help"})
	loads := 0
	testseam.Swap(t, &rootLoadPlugins, func(*cobra.Command, *pipeline.Engine, executor.Runner) []*cobra.Command {
		loads++
		return nil
	})

	_ = newProcessRootCommandWithEngine(context.Background(), nil)
	if loads != 0 {
		t.Fatalf("clean command surface loaded plugins %d times", loads)
	}

	pluginDir := filepath.Join(configDir, "plugins", "user", "example")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = newProcessRootCommandWithEngine(context.Background(), nil)
	if loads != 1 {
		t.Fatalf("extension-bearing command surface loaded plugins %d times, want 1", loads)
	}
}

func TestProcessCompletionKeepsCompleteProductSurface(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "1")
	for _, args := range [][]string{
		{"dws", "completion", "zsh"},
		{"dws", cobra.ShellCompRequestCmd, "calendar"},
		{"dws", cobra.ShellCompNoDescRequestCmd, "calendar"},
	} {
		testseam.Swap(t, &os.Args, args)
		root := newProcessRootCommandWithEngine(context.Background(), nil)
		if !hasTopLevelCommand(root, "calendar") || !hasTopLevelCommand(root, "drive") {
			t.Fatalf("completion route %q must retain the complete product tree", args[1])
		}
	}
}

func TestProcessRootMountsOnlySelectedProductAndKeepsLibraryRootComplete(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "1")
	testseam.Swap(t, &os.Args, []string{"dws", "calendar", "book", "list", "--help"})

	processRoot := newProcessRootCommandWithEngine(context.Background(), nil)
	if !hasTopLevelCommand(processRoot, "calendar") || hasTopLevelCommand(processRoot, "drive") {
		t.Fatalf("selective process root has calendar=%v drive=%v", hasTopLevelCommand(processRoot, "calendar"), hasTopLevelCommand(processRoot, "drive"))
	}
	if !hasTopLevelCommand(processRoot, "config") {
		t.Fatal("selective process root dropped utility commands")
	}

	os.Args = []string{"dws", "event", "+listen-im", "--dry-run"}
	eventRoot := newProcessRootCommandWithEngine(context.Background(), nil)
	event := findDirectChild(eventRoot, "event")
	if event == nil || findDirectChild(event, "+listen-im") == nil {
		t.Fatal("utility/product name collision dropped event shortcuts")
	}

	libraryRoot := NewRootCommand(context.Background())
	if !hasTopLevelCommand(libraryRoot, "calendar") || !hasTopLevelCommand(libraryRoot, "drive") {
		t.Fatal("reusable library root must retain the complete command surface")
	}

	os.Args = []string{"dws", "im", "message", "list", "--help"}
	aliasRoot := newProcessRootCommandWithEngine(context.Background(), nil)
	if !hasTopLevelCommand(aliasRoot, "chat") || hasTopLevelCommand(aliasRoot, "drive") {
		t.Fatal("top-level alias did not select its canonical product only")
	}

	os.Args = []string{"dws", "calendr", "book", "list"}
	unknownRoot := newProcessRootCommandWithEngine(context.Background(), nil)
	if !hasTopLevelCommand(unknownRoot, "calendar") || !hasTopLevelCommand(unknownRoot, "drive") {
		t.Fatal("unknown command must retain the full tree for suggestions")
	}

	pluginDir := filepath.Join(os.Getenv("DWS_CONFIG_DIR"), "plugins", "user", "example")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"dws", "calendar", "book", "list", "--help"}
	extensionRoot := newProcessRootCommandWithEngine(context.Background(), nil)
	if !hasTopLevelCommand(extensionRoot, "drive") {
		t.Fatal("runtime extension state must force complete-tree fallback")
	}
}

func TestProcessRootSelectedCommandsMatchCompleteTreeContracts(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "1")
	testseam.Protect(t, &os.Args)
	complete := interfacesnapshot.Capture(NewRootCommand(context.Background()))
	completeByPath := make(map[string]interfacesnapshot.Command, len(complete.Commands))
	for _, command := range complete.Commands {
		completeByPath[command.Path] = command
	}
	for _, args := range [][]string{
		{"dws", "calendar", "book", "list", "--dry-run", "-f", "json"},
		{"dws", "im", "message", "list", "--dry-run", "-f", "json"},
	} {
		os.Args = args
		selected := interfacesnapshot.Capture(newProcessRootCommandWithEngine(context.Background(), nil))
		for _, command := range selected.Commands {
			want, ok := completeByPath[command.Path]
			if !ok {
				t.Fatalf("selected tree for %q added command outside complete tree: %s", args[1], command.Path)
			}
			if !reflect.DeepEqual(command, want) {
				t.Fatalf("selected command contract for %q differs from complete tree: %s", args[1], command.Path)
			}
		}
	}
}

func TestProcessRootUserShortcutStateForcesCompleteTree(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("DO_NOT_TRACK", "1")
	if err := os.Mkdir(filepath.Join(configDir, "shortcuts"), 0o700); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &os.Args, []string{"dws", "schema", "--compact"})

	root := newProcessRootCommandWithEngine(context.Background(), nil)
	if !hasTopLevelCommand(root, "calendar") || !hasTopLevelCommand(root, "drive") {
		t.Fatal("user shortcut state must retain the complete tree and its startup diagnostics")
	}
}

func hasTopLevelCommand(root *cobra.Command, name string) bool {
	for _, command := range root.Commands() {
		if commandMatchesTopLevel(command, name) {
			return true
		}
	}
	return false
}

package helpers

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

func runDingCoverageCommand(t *testing.T, caller *productExampleCaller, args ...string) error {
	t.Helper()
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newDingCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func TestCrossPlatformCoverageDingOpenDingIDFlagsUseDingID(t *testing.T) {
	for _, path := range [][]string{
		{"message", "recall-personal"},
		{"message", "recall"},
	} {
		command, _, err := newDingCommand().Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if command.Flags().Lookup("ding-id") == nil {
			t.Fatalf("%s missing --ding-id", strings.Join(path, " "))
		}
		legacy := command.Flags().Lookup("id")
		if legacy == nil || !legacy.Hidden {
			t.Fatalf("%s --id = %#v, want hidden alias", strings.Join(path, " "), legacy)
		}
		if got := legacy.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != "ding-id" {
			t.Fatalf("%s --id alias_of = %#v", strings.Join(path, " "), got)
		}
	}
}

func TestCrossPlatformCoverageDingHelpHidesLegacyID(t *testing.T) {
	for _, path := range [][]string{
		{"message", "recall-personal"},
		{"message", "recall"},
	} {
		command, _, err := newDingCommand().Find(path)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		command.SetOut(&out)
		command.SetErr(io.Discard)
		command.SetArgs([]string{"--help"})
		if err := command.Help(); err != nil {
			t.Fatalf("%s help: %v", strings.Join(path, " "), err)
		}
		help := out.String()
		if !strings.Contains(help, "--ding-id") {
			t.Fatalf("%s help missing --ding-id:\n%s", strings.Join(path, " "), help)
		}
		if strings.Contains(help, "--id") {
			t.Fatalf("%s help exposed legacy --id:\n%s", strings.Join(path, " "), help)
		}
	}
}

func TestCrossPlatformCoverageDingIDAliasCompatibilityAndConflict(t *testing.T) {
	caller := &productExampleCaller{}
	if err := runDingCoverageCommand(t, caller, "message", "recall-personal", "--id=ding-legacy"); err != nil {
		t.Fatalf("legacy --id failed: %v", err)
	}
	if caller.calls != 1 {
		t.Fatalf("legacy calls = %d, want 1", caller.calls)
	}

	caller = &productExampleCaller{}
	err := runDingCoverageCommand(t, caller,
		"message", "recall-personal",
		"--ding-id=ding-new",
		"--id=ding-old")
	if err == nil || !strings.Contains(err.Error(), "--ding-id conflicts with --id") {
		t.Fatalf("conflict err = %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("conflict calls = %d, want 0", caller.calls)
	}
}

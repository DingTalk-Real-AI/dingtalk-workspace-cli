package pipeline

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageFlagInfoFromCommandIncludesLocalInheritedAndAnnotations(t *testing.T) {
	if FlagInfoFromCommand(nil) != nil {
		t.Fatal("FlagInfoFromCommand(nil) != nil")
	}
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("profile", "", "")
	child := &cobra.Command{Use: "child"}
	child.Flags().String("start-time", "", "")
	child.Flags().Lookup("start-time").Annotations = map[string][]string{
		"x-cli-format": {"date-time"},
		"x-cli-enum":   {"one", "two"},
	}
	root.AddCommand(child)

	infos := FlagInfoFromCommand(child)
	if len(infos) != 2 {
		t.Fatalf("FlagInfoFromCommand() = %#v", infos)
	}
	byName := make(map[string]FlagInfo)
	for _, info := range infos {
		byName[info.Name] = info
	}
	if byName["profile"].Type != "string" || byName["start-time"].Format != "date-time" ||
		!reflect.DeepEqual(byName["start-time"].Enum, []string{"one", "two"}) {
		t.Fatalf("flag infos = %#v", infos)
	}

	var deduplicated []FlagInfo
	seen := make(map[string]bool)
	flag := child.Flags().Lookup("start-time")
	appendFlagInfo(&deduplicated, seen, flag)
	appendFlagInfo(&deduplicated, seen, flag)
	if len(deduplicated) != 1 {
		t.Fatalf("appendFlagInfo duplicate result = %#v", deduplicated)
	}
}

func TestCrossPlatformCoverageRunPreParseGuardAndTraversalBranches(t *testing.T) {
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "flagless"})

	RunPreParse(root, nil)
	RunPreParse(root, NewEngine())

	engine := NewEngine()
	engine.Register(newStub("noop", PreParse, nil))
	os.Args = []string{"root"}
	RunPreParse(root, engine)
	os.Args = []string{"root", "missing"}
	RunPreParse(root, engine)
	os.Args = []string{"root", "--unknown", "value", "flagless"}
	RunPreParse(root, engine)
	os.Args = []string{"root", "flagless"}
	RunPreParse(root, engine)
}

func TestCrossPlatformCoverageRunPreParseAppliesCorrectionsOnlyOnSuccess(t *testing.T) {
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })

	buildRoot := func() (*cobra.Command, *string) {
		root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
		child := &cobra.Command{Use: "child"}
		value := ""
		child.Flags().StringVar(&value, "name", "", "")
		root.AddCommand(child)
		return root, &value
	}

	root, value := buildRoot()
	engine := NewEngine()
	engine.Register(newStub("correct", PreParse, func(ctx *Context) error {
		ctx.Args[len(ctx.Args)-1] = "corrected"
		ctx.AddCorrection("correct", PreParse, "name", "wrong", "corrected", "test")
		return nil
	}))
	os.Args = []string{"root", "child", "--name", "wrong"}
	RunPreParse(root, engine)
	if err := root.Execute(); err != nil || *value != "corrected" {
		t.Fatalf("corrected execute = %q, %v", *value, err)
	}

	root, value = buildRoot()
	noCorrection := NewEngine()
	noCorrection.Register(newStub("inspect", PreParse, func(*Context) error { return nil }))
	os.Args = []string{"root", "child", "--name", "original"}
	RunPreParse(root, noCorrection)
	if err := root.Execute(); err != nil || *value != "original" {
		t.Fatalf("uncorrected execute = %q, %v", *value, err)
	}

	root, value = buildRoot()
	failing := NewEngine()
	failing.Register(newStub("fail", PreParse, func(*Context) error { return errors.New("boom") }))
	os.Args = []string{"root", "child", "--name", "original"}
	if err := RunPreParse(root, failing); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("failed preparse error = %v, want boom", err)
	}
	if err := root.Execute(); err != nil || *value != "original" {
		t.Fatalf("failed preparse execute = %q, %v", *value, err)
	}
}

func TestRunPreParseResolvesCommandPastLeadingPersistentFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "boolean long flag", args: []string{"--dry-run", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "valued long flag", args: []string{"--profile", "corp:user", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "valued shorthand", args: []string{"-f", "json", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "attached shorthand", args: []string{"-fjson", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "clustered attached shorthand", args: []string{"-vfjson", "calendar", "event", "list", "--date", "2026-03-10"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().Bool("dry-run", false, "")
			root.PersistentFlags().String("profile", "", "")
			root.PersistentFlags().StringP("format", "f", "json", "")
			root.PersistentFlags().BoolP("verbose", "v", false, "")

			// This similarly named root path makes the old traversal failure
			// deterministic: `--dry-run` consumed "calendar" as a value and
			// incorrectly selected `dws event list`.
			misleadingEvent := &cobra.Command{Use: "event"}
			misleadingEvent.AddCommand(&cobra.Command{Use: "list"})
			root.AddCommand(misleadingEvent)

			calendar := &cobra.Command{Use: "calendar"}
			event := &cobra.Command{Use: "event"}
			value := ""
			list := &cobra.Command{Use: "list"}
			list.Flags().StringVar(&value, "start", "", "")
			event.AddCommand(list)
			calendar.AddCommand(event)
			root.AddCommand(calendar)

			engine := NewEngine()
			engine.Register(newStub("calendar-date-alias", PreParse, func(ctx *Context) error {
				if ctx.Command != "dws calendar event list" {
					t.Fatalf("resolved command = %q, want dws calendar event list", ctx.Command)
				}
				for index, argument := range ctx.Args {
					if argument == "--date" {
						ctx.Args[index] = "--start"
						ctx.AddCorrection("calendar-date-alias", PreParse, "start", "--date", "--start", "test")
					}
				}
				return nil
			}))

			root.SetArgs(test.args)
			ctx, err := RunPreParseArgs(root, engine, test.args)
			if err != nil {
				t.Fatalf("RunPreParseArgs() error = %v", err)
			}
			if ctx == nil || len(ctx.Corrections) != 1 {
				t.Fatalf("RunPreParseArgs() context = %#v", ctx)
			}
			if err := root.Execute(); err != nil {
				t.Fatalf("corrected command failed: %v", err)
			}
			if value != "2026-03-10" {
				t.Fatalf("canonical --start value = %q", value)
			}
		})
	}
}

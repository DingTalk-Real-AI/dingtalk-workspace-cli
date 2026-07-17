package pipeline

import (
	"errors"
	"os"
	"reflect"
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
	RunPreParse(root, failing)
	if err := root.Execute(); err != nil || *value != "original" {
		t.Fatalf("failed preparse execute = %q, %v", *value, err)
	}
}

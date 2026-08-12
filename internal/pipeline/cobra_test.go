package pipeline

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageFlagInfoFromCommandIncludesLocalInheritedAndAnnotations(t *testing.T) {
	if FlagInfoFromCommand(nil) != nil {
		t.Fatal("FlagInfoFromCommand(nil) != nil")
	}
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("profile", "", "")
	child := &cobra.Command{Use: "child"}
	child.Flags().StringP("start-time", "s", "", "")
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
	if byName["profile"].Type != "string" || byName["start-time"].Shorthand != "s" || byName["start-time"].Format != "date-time" ||
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
	testseam.Protect(t, &os.Args)
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
	testseam.Protect(t, &os.Args)

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
		name       string
		args       []string
		executable bool
	}{
		{name: "boolean long flag", args: []string{"--dry-run", "calendar", "event", "list", "--date", "2026-03-10"}, executable: true},
		{name: "boolean long flag with detached false", args: []string{"--dry-run", "false", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "camel-case boolean long flag", args: []string{"--dryRun", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "camel-case boolean with detached false", args: []string{"--dryRun", "false", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "fuzzy boolean long flag", args: []string{"--dry-rnu", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "fuzzy boolean with detached false", args: []string{"--dry-rnu", "false", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "valued long flag", args: []string{"--profile", "corp:user", "calendar", "event", "list", "--date", "2026-03-10"}, executable: true},
		{name: "fuzzy valued long flag", args: []string{"--profle", "corp:user", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "sticky valued long flag", args: []string{"--timeout30", "calendar", "event", "list", "--date", "2026-03-10"}},
		{name: "valued shorthand", args: []string{"-f", "json", "calendar", "event", "list", "--date", "2026-03-10"}, executable: true},
		{name: "attached shorthand", args: []string{"-fjson", "calendar", "event", "list", "--date", "2026-03-10"}, executable: true},
		{name: "clustered attached shorthand", args: []string{"-vfjson", "calendar", "event", "list", "--date", "2026-03-10"}, executable: true},
		{name: "boolean shorthand with detached false", args: []string{"-v", "false", "calendar", "event", "list", "--date", "2026-03-10"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().Bool("dry-run", false, "")
			root.PersistentFlags().String("profile", "", "")
			root.PersistentFlags().Int("timeout", 0, "")
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
			if test.executable {
				if err := root.Execute(); err != nil {
					t.Fatalf("corrected command failed: %v", err)
				}
				if value != "2026-03-10" {
					t.Fatalf("canonical --start value = %q", value)
				}
			}
		})
	}
}

func TestRunPreParsePrimesPresentationFlagsForEarlyErrors(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantFormat  string
		wantDebug   bool
		wantVerbose bool
	}{
		{
			name:       "canonical flags after command",
			args:       []string{"child", "--name", "demo", "--format", "table", "--debug"},
			wantFormat: "table",
			wantDebug:  true,
		},
		{
			name:        "normalized presentation names",
			args:        []string{"--dryRun", "--FORMAT=pretty", "--Verbose", "child", "--name", "demo"},
			wantFormat:  "pretty",
			wantVerbose: true,
		},
		{
			name:        "clustered shorthands",
			args:        []string{"-vfraw", "child", "--name", "demo"},
			wantFormat:  "raw",
			wantVerbose: true,
		},
		{
			name:        "explicit boolean presentation values",
			args:        []string{"--debug=true", "-v=false", "child", "--name", "demo"},
			wantFormat:  "json",
			wantDebug:   true,
			wantVerbose: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
			root.PersistentFlags().Bool("dry-run", false, "")
			root.PersistentFlags().StringP("format", "f", "json", "")
			root.PersistentFlags().Bool("debug", false, "")
			root.PersistentFlags().BoolP("verbose", "v", false, "")
			child := &cobra.Command{Use: "child"}
			child.Flags().String("name", "", "")
			root.AddCommand(child)

			engine := NewEngine()
			engine.Register(newStub("fail", PreParse, func(*Context) error { return errors.New("early") }))
			ctx, err := RunPreParseArgs(root, engine, test.args)
			if err == nil || ctx == nil {
				t.Fatalf("RunPreParseArgs() = %#v, %v; want early error with context", ctx, err)
			}
			format, _ := root.PersistentFlags().GetString("format")
			debug, _ := root.PersistentFlags().GetBool("debug")
			verbose, _ := root.PersistentFlags().GetBool("verbose")
			if format != test.wantFormat || debug != test.wantDebug || verbose != test.wantVerbose {
				t.Fatalf("presentation flags = format:%q debug:%v verbose:%v; want %q/%v/%v", format, debug, verbose, test.wantFormat, test.wantDebug, test.wantVerbose)
			}
		})
	}
}

func TestRunPreParseKeepsPrimaryErrorWhenPresentationParsingFails(t *testing.T) {
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringP("format", "f", "json", "")
	root.PersistentFlags().Bool("debug", false, "")
	root.PersistentFlags().BoolP("verbose", "v", false, "")
	child := &cobra.Command{Use: "child"}
	child.Flags().String("name", "", "")
	root.AddCommand(child)

	primaryErr := errors.New("primary pre-parse failure")
	engine := NewEngine()
	engine.Register(newStub("fail", PreParse, func(*Context) error { return primaryErr }))

	ctx, err := RunPreParseArgs(root, engine, []string{
		"child", "--name", "demo", "--format", "table", "--debug=maybe",
	})
	if ctx == nil || !errors.Is(err, primaryErr) {
		t.Fatalf("RunPreParseArgs() = %#v, %v; want primary error", ctx, err)
	}
	format, _ := root.PersistentFlags().GetString("format")
	debug, _ := root.PersistentFlags().GetBool("debug")
	if format != "table" || debug {
		t.Fatalf("partially valid presentation values = format:%q debug:%v", format, debug)
	}
}

func TestCommandTraversalFlagTokenEdges(t *testing.T) {
	raw := []string{"child"}
	if got := argsForCommandTraversal(nil, raw); !reflect.DeepEqual(got, raw) {
		t.Fatalf("nil-root traversal args = %v", got)
	}
	root := &cobra.Command{Use: "root"}
	if got := argsForCommandTraversal(root, nil); got != nil {
		t.Fatalf("empty traversal args = %v", got)
	}
	root.PersistentFlags().BoolP("verbose", "v", false, "")
	root.PersistentFlags().StringP("format", "f", "", "")
	if got := argsForCommandTraversal(root, []string{"--", "--verbose", "child"}); !reflect.DeepEqual(got, []string{"--", "--verbose", "child"}) {
		t.Fatalf("double-dash traversal args = %v", got)
	}

	if flag, inline, matched := newFlagTokenMatcher(nil).matchTraversalToken("--verbose"); flag != nil || inline || matched {
		t.Fatalf("nil flag set matched: %#v, %v, %v", flag, inline, matched)
	}
	if flag, inline, matched := (*flagTokenMatcher)(nil).matchTraversalToken(""); flag != nil || inline || matched {
		t.Fatalf("nil matcher matched: %#v, %v, %v", flag, inline, matched)
	}
	if match := (*flagTokenMatcher)(nil).matchLongToken("--verbose"); match.recognized {
		t.Fatalf("nil long matcher matched: %#v", match)
	}
	if flag, inline, matched := newFlagTokenMatcher(root.PersistentFlags()).matchTraversalToken("-x"); flag != nil || inline || matched {
		t.Fatalf("unknown shorthand matched: %#v, %v, %v", flag, inline, matched)
	}
	flag, inline, matched := newFlagTokenMatcher(root.PersistentFlags()).matchTraversalToken("-vv")
	if !matched || !inline || flag == nil || flag.Name != "verbose" {
		t.Fatalf("boolean shorthand cluster = %#v, %v, %v", flag, inline, matched)
	}

	duplicate := pflag.NewFlagSet("duplicate", pflag.ContinueOnError)
	duplicate.Bool("verbose", false, "")
	matcher := newFlagTokenMatcher(root.PersistentFlags(), duplicate)
	if len(matcher.byName) != 2 || matcher.byName["verbose"] != root.PersistentFlags().Lookup("verbose") {
		t.Fatalf("duplicate flag precedence = %#v", matcher.byName)
	}
}

func TestSeparatedBoolValueRecognition(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.BoolP("verbose", "v", false, "")
	flags.String("format", "", "")
	verbose := flags.Lookup("verbose")
	format := flags.Lookup("format")

	tests := []struct {
		name      string
		argument  string
		following string
		flag      *pflag.Flag
		inline    bool
		want      string
		ok        bool
	}{
		{name: "nil flag", argument: "--verbose", following: "false"},
		{name: "non bool", argument: "--format", following: "false", flag: format},
		{name: "long false", argument: "--verbose", following: "false", flag: verbose, want: "false", ok: true},
		{name: "long synonym", argument: "--verbose", following: "on", flag: verbose, want: "true", ok: true},
		{name: "inline long", argument: "--verbosefalse", following: "false", flag: verbose, inline: true},
		{name: "equals long", argument: "--verbose=false", following: "true", flag: verbose},
		{name: "exact shorthand", argument: "-v", following: "0", flag: verbose, want: "false", ok: true},
		{name: "shorthand cluster", argument: "-vv", following: "false", flag: verbose},
		{name: "invalid literal", argument: "--verbose", following: "maybe", flag: verbose},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := separatedBoolValue(test.argument, test.following, test.flag, test.inline)
			if got != test.want || ok != test.ok {
				t.Fatalf("separatedBoolValue() = %q, %v; want %q, %v", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestPrimeEarlyErrorPresentationEdges(t *testing.T) {
	if err := primeEarlyErrorPresentation(nil, nil, nil); err != nil {
		t.Fatalf("nil presentation priming error = %v", err)
	}

	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().StringP("format", "f", "json", "")
	root.PersistentFlags().Bool("debug", false, "")
	root.PersistentFlags().BoolP("verbose", "v", false, "")
	child := &cobra.Command{Use: "child"}
	child.Flags().String("name", "", "")
	root.AddCommand(child)

	if err := primeEarlyErrorPresentation(root, child, []string{
		"child", "--unknown", "value", "-x", "--name", "demo",
		"-f", "table", "-v", "maybe", "--", "--debug",
	}); err != nil {
		t.Fatalf("presentation priming error = %v", err)
	}
	format, _ := root.PersistentFlags().GetString("format")
	debug, _ := root.PersistentFlags().GetBool("debug")
	verbose, _ := root.PersistentFlags().GetBool("verbose")
	if format != "table" || debug || !verbose {
		t.Fatalf("presentation after edge argv = format:%q debug:%v verbose:%v", format, debug, verbose)
	}
}

func TestPrimeEarlyErrorPresentationReportsParseAndContractErrors(t *testing.T) {
	t.Run("invalid presentation value is reported after applying valid values", func(t *testing.T) {
		root := &cobra.Command{Use: "root"}
		root.PersistentFlags().StringP("format", "f", "json", "")
		root.PersistentFlags().Bool("debug", false, "")
		root.PersistentFlags().BoolP("verbose", "v", false, "")
		child := &cobra.Command{Use: "child"}
		root.AddCommand(child)

		err := primeEarlyErrorPresentation(root, child, []string{"child", "--format", "table", "--debug=maybe"})
		if err == nil || !strings.Contains(err.Error(), "invalid argument") {
			t.Fatalf("invalid presentation error = %v", err)
		}
		format, _ := root.PersistentFlags().GetString("format")
		debug, _ := root.PersistentFlags().GetBool("debug")
		if format != "table" || debug {
			t.Fatalf("partially valid presentation values = format:%q debug:%v", format, debug)
		}
	})

	for _, test := range []struct {
		name string
		add  func(*pflag.FlagSet)
		want string
	}{
		{name: "format type drift", add: func(flags *pflag.FlagSet) { flags.Bool("format", false, "") }, want: "read presentation flag --format"},
		{name: "debug type drift", add: func(flags *pflag.FlagSet) {
			flags.String("format", "json", "")
			flags.String("debug", "", "")
		}, want: "read presentation flag --debug"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := &cobra.Command{Use: "root"}
			test.add(root.PersistentFlags())
			err := primeEarlyErrorPresentation(root, root, []string{"--format", "table"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("presentation contract error = %v, want %q", err, test.want)
			}
		})
	}

	t.Run("apply error is returned", func(t *testing.T) {
		root := &cobra.Command{Use: "root"}
		value := &rejectingPresentationString{value: "json"}
		root.PersistentFlags().VarP(value, "format", "f", "")
		err := primeEarlyErrorPresentation(root, root, []string{"--format", "table"})
		if err == nil || !strings.Contains(err.Error(), "apply presentation flag --format") {
			t.Fatalf("presentation apply error = %v", err)
		}
	})

	t.Run("missing presentation flags are optional", func(t *testing.T) {
		root := &cobra.Command{Use: "root"}
		if err := primeEarlyErrorPresentation(root, root, []string{"--unknown", "value"}); err != nil {
			t.Fatalf("optional presentation flags error = %v", err)
		}
	})
}

type rejectingPresentationString struct {
	value string
}

func (v *rejectingPresentationString) Set(string) error { return errors.New("rejected") }
func (v *rejectingPresentationString) String() string   { return v.value }
func (*rejectingPresentationString) Type() string       { return "string" }

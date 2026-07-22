// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

type pluginFailRunner struct{}

func (pluginFailRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	return executor.Result{}, errors.New("runner failed")
}

type pluginWrongFlagValue struct{}

func (pluginWrongFlagValue) String() string   { return "" }
func (pluginWrongFlagValue) Set(string) error { return nil }
func (pluginWrongFlagValue) Type() string     { return "wrong" }

func TestPluginCompilerRejectsInvalidDuplicateAndEmptyDefinitions(t *testing.T) {
	invalidRoot := conferencePluginDescriptor()
	invalidRoot.CLI.Command = "Invalid Root"
	if commands := buildPluginCommands([]mcptypes.ServerDescriptor{invalidRoot}, executor.EchoRunner{}, nil); len(commands) != 0 {
		t.Fatalf("invalid root produced commands %#v", commands)
	}

	descriptor := conferencePluginDescriptor()
	descriptor.CLI.Groups = map[string]mcptypes.CLIGroupDef{
		"empty": {Description: "removed when no leaf survives"},
	}
	descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"":        {CLIName: "blank-tool"},
		"hidden":  {CLIName: "hidden", Hidden: true},
		"invalid": {CLIName: "Invalid Leaf"},
		"first":   {CLIName: "same"},
		"second":  {CLIName: "same"},
	}
	commands := buildPluginCommands([]mcptypes.ServerDescriptor{descriptor}, executor.EchoRunner{}, nil)
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	if requireOptionalPluginChild(commands[0], "same") == nil {
		t.Fatal("valid leaf was not retained")
	}
	if requireOptionalPluginChild(commands[0], "empty") != nil {
		t.Fatal("empty group was not pruned")
	}

	empty := conferencePluginDescriptor()
	empty.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"hidden": {CLIName: "hidden", Hidden: true},
	}
	if commands := buildPluginCommands([]mcptypes.ServerDescriptor{empty}, executor.EchoRunner{}, nil); len(commands) != 0 {
		t.Fatalf("empty overlay produced commands %#v", commands)
	}
}

func TestPluginLeafExecutionErrorsAndBodyWrapper(t *testing.T) {
	base := conferencePluginDescriptor()
	base.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"wrapped": {
			CLIName:     "wrapped",
			BodyWrapper: "body",
			Flags: map[string]mcptypes.CLIFlagOverride{
				"value": {Required: true},
			},
		},
	}
	runner := &pluginCaptureRunner{}
	root := pluginTestRoot(buildPluginCommands([]mcptypes.ServerDescriptor{base}, runner, nil)...)
	root.SetArgs([]string{"conference", "wrapped", "--value", "ok", "--params", `{"body":{"old":1},"_meta":"kept"}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("wrapped command: %v", err)
	}
	want := map[string]any{
		"_meta": "kept",
		"body":  map[string]any{"old": float64(1), "value": "ok"},
	}
	if !reflect.DeepEqual(runner.invocations[0].Params, want) {
		t.Fatalf("wrapped params = %#v, want %#v", runner.invocations[0].Params, want)
	}

	for _, testCase := range []struct {
		name   string
		runner executor.Runner
		args   []string
	}{
		{name: "invalid json", runner: executor.EchoRunner{}, args: []string{"conference", "wrapped", "--json", "["}},
		{name: "missing required", runner: executor.EchoRunner{}, args: []string{"conference", "wrapped"}},
		{name: "missing runner", runner: nil, args: []string{"conference", "wrapped", "--value", "ok"}},
		{name: "runner error", runner: pluginFailRunner{}, args: []string{"conference", "wrapped", "--value", "ok"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			commandRoot := pluginTestRoot(buildPluginCommands([]mcptypes.ServerDescriptor{base}, testCase.runner, nil)...)
			commandRoot.SetArgs(testCase.args)
			if err := commandRoot.Execute(); err == nil {
				t.Fatal("expected command error")
			}
		})
	}

	for _, flagName := range []string{"json", "params"} {
		t.Run("unreadable "+flagName, func(t *testing.T) {
			commands := buildPluginCommands([]mcptypes.ServerDescriptor{base}, executor.EchoRunner{}, nil)
			leaf := requirePluginChild(t, commands[0], "wrapped")
			leaf.Flags().Lookup(flagName).Value = pluginWrongFlagValue{}
			commandRoot := pluginTestRoot(commands...)
			commandRoot.SetArgs([]string{"conference", "wrapped", "--value", "ok"})
			if err := commandRoot.Execute(); err == nil {
				t.Fatal("expected unreadable flag error")
			}
		})
	}
}

func TestPluginBindingCompilerCoversAliasesAndPositionalValidators(t *testing.T) {
	reservations := pluginFlagReservations{
		names:      map[string]bool{"reserved": true},
		shorthands: map[string]bool{},
	}
	bindings, _, _, ok := registerPluginBindings("alias", mcptypes.CLIToolOverride{
		Flags: map[string]mcptypes.CLIFlagOverride{
			"value": {Alias: "value", Aliases: []string{"", "Bad", "value", "other"}},
		},
	}, reservations)
	if !ok || !reflect.DeepEqual(bindings[0].names, []string{"value", "other"}) {
		t.Fatalf("alias bindings = (%#v, %v)", bindings, ok)
	}
	if _, _, _, ok := registerPluginBindings("conflict", mcptypes.CLIToolOverride{
		Flags: map[string]mcptypes.CLIFlagOverride{"value": {Alias: "reserved"}},
	}, reservations); ok {
		t.Fatal("reserved flag was accepted")
	}
	if _, _, _, ok := registerPluginBindings("negative", mcptypes.CLIToolOverride{
		Flags: map[string]mcptypes.CLIFlagOverride{"value": {Positional: true, PositionalIndex: -1}},
	}, reservations); ok {
		t.Fatal("negative positional index was accepted")
	}
	if _, _, _, ok := registerPluginBindings("duplicate", mcptypes.CLIToolOverride{
		Flags: map[string]mcptypes.CLIFlagOverride{
			"first":  {Positional: true, PositionalIndex: 0},
			"second": {Positional: true, PositionalIndex: 0},
		},
	}, reservations); ok {
		t.Fatal("duplicate positional index was accepted")
	}
	if _, _, _, ok := registerPluginBindings("gap", mcptypes.CLIToolOverride{
		Flags: map[string]mcptypes.CLIFlagOverride{
			"second": {Positional: true, PositionalIndex: 1},
		},
	}, reservations); ok {
		t.Fatal("non-contiguous positional indexes were accepted")
	}

	for _, testCase := range []struct {
		name    string
		flags   map[string]mcptypes.CLIFlagOverride
		wantUse string
		valid   []string
		invalid []string
	}{
		{
			name: "exact",
			flags: map[string]mcptypes.CLIFlagOverride{
				"second": {Positional: true, PositionalIndex: 1, Required: true},
				"first":  {Positional: true, PositionalIndex: 0, Required: true},
			},
			wantUse: "exact [first] [second]", valid: []string{"a", "b"}, invalid: []string{"a"},
		},
		{
			name: "range",
			flags: map[string]mcptypes.CLIFlagOverride{
				"first":  {Positional: true, PositionalIndex: 0, Required: true},
				"second": {Positional: true, PositionalIndex: 1},
			},
			wantUse: "range [first] [second]", valid: []string{"a"}, invalid: []string{},
		},
		{
			name: "maximum",
			flags: map[string]mcptypes.CLIFlagOverride{
				"first":  {Positional: true, PositionalIndex: 0},
				"second": {Positional: true, PositionalIndex: 1},
			},
			wantUse: "maximum [first] [second]", valid: []string{}, invalid: []string{"a", "b", "c"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, use, validator, ok := registerPluginBindings(testCase.name, mcptypes.CLIToolOverride{Flags: testCase.flags}, reservations)
			if !ok || use != testCase.wantUse {
				t.Fatalf("binding contract = (%q, %v)", use, ok)
			}
			cmd := &cobra.Command{Use: testCase.name}
			if err := validator(cmd, testCase.valid); err != nil {
				t.Fatalf("valid args: %v", err)
			}
			if err := validator(cmd, testCase.invalid); err == nil {
				t.Fatal("invalid args were accepted")
			}
		})
	}
}

func TestPluginFlagRegistrationAndReadingCoversAllKinds(t *testing.T) {
	cmd := &cobra.Command{Use: "leaf"}
	override := mcptypes.CLIToolOverride{Flags: map[string]mcptypes.CLIFlagOverride{
		"integer": {Default: "2", Shorthand: "i", Hidden: true},
		"float":   {Default: "1.5"},
		"boolean": {Default: "true"},
		"slice":   {Default: "one, ,two"},
		"json":    {Default: `{"old":true}`},
		"string":  {Default: "text"},
	}}
	bindings := []pluginFlagBinding{
		{property: "integer", names: []string{"integer", "integer-alias"}, kind: pluginFlagInt},
		{property: "float", names: []string{"float"}, kind: pluginFlagFloat},
		{property: "boolean", names: []string{"boolean"}, kind: pluginFlagBool},
		{property: "slice", names: []string{"slice"}, kind: pluginFlagStringSlice},
		{property: "json", names: []string{"json-value"}, kind: pluginFlagJSON},
		{property: "string", names: []string{"string"}, kind: pluginFlagString},
	}
	registerPluginFlags(cmd, bindings, override, pluginFlagReservations{shorthands: map[string]bool{}})
	for name, raw := range map[string]string{
		"integer": "3", "float": "2.5", "boolean": "false",
		"slice": "three,four", "json-value": `{"ok":true}`, "string": "changed",
	} {
		if err := cmd.Flags().Set(name, raw); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	wants := map[string]any{
		"integer": 3,
		"float":   2.5,
		"boolean": false,
		"slice":   []string{"three", "four"},
		"json":    map[string]any{"ok": true},
		"string":  "changed",
	}
	for _, binding := range bindings {
		value, err := readPluginFlag(cmd.Flags(), binding.names[0], binding.kind)
		if err != nil || !reflect.DeepEqual(value, wants[binding.property]) {
			t.Fatalf("read %s = (%#v, %v), want %#v", binding.property, value, err, wants[binding.property])
		}
	}
	if !cmd.Flags().Lookup("integer").Hidden || !cmd.Flags().Lookup("integer-alias").Hidden {
		t.Fatal("hidden primary or alias flag was exposed")
	}
	if err := cmd.Flags().Set("json-value", "{"); err != nil {
		t.Fatal(err)
	}
	if _, err := readPluginFlag(cmd.Flags(), "json-value", pluginFlagJSON); err == nil {
		t.Fatal("invalid JSON flag was accepted")
	}
	cmd.Flags().Lookup("json-value").Value = pluginWrongFlagValue{}
	if _, err := readPluginFlag(cmd.Flags(), "json-value", pluginFlagJSON); err == nil {
		t.Fatal("wrong JSON flag type was accepted")
	}
}

func TestCollectPluginBindingsCoversEveryValueSourceAndFailure(t *testing.T) {
	t.Run("sources", func(t *testing.T) {
		cmd := &cobra.Command{Use: "leaf"}
		registerPluginFlag(cmd.Flags(), "flag", "", "", pluginFlagString, "")
		if err := cmd.Flags().Set("flag", "from-flag"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PLUGIN_COVERAGE_ENV", "7")
		params := map[string]any{"existing": "from-json"}
		bindings := []pluginFlagBinding{
			{property: "flag", names: []string{"flag"}, kind: pluginFlagString},
			{property: "existing", kind: pluginFlagString},
			{property: "positional", kind: pluginFlagBool, positional: true, positionalIndex: 0},
			{property: "default", kind: pluginFlagFloat, defaultProvided: true, defaultValue: "1.5"},
			{property: "env", kind: pluginFlagInt, envDefault: "PLUGIN_COVERAGE_ENV"},
			{property: "optional", kind: pluginFlagString},
		}
		if err := collectPluginBindings(cmd, []string{"true"}, bindings, params); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"flag": "from-flag", "existing": "from-json", "positional": true,
			"default": 1.5, "env": 7,
		}
		if !reflect.DeepEqual(params, want) {
			t.Fatalf("params = %#v, want %#v", params, want)
		}
	})

	for _, testCase := range []struct {
		name    string
		prepare func(t *testing.T, cmd *cobra.Command)
		args    []string
		binding pluginFlagBinding
		params  map[string]any
	}{
		{
			name: "wrong flag type",
			prepare: func(t *testing.T, cmd *cobra.Command) {
				cmd.Flags().String("value", "", "")
				if err := cmd.Flags().Set("value", "x"); err != nil {
					t.Fatal(err)
				}
			},
			binding: pluginFlagBinding{property: "value", names: []string{"value"}, kind: pluginFlagInt},
		},
		{name: "invalid positional", args: []string{"maybe"}, binding: pluginFlagBinding{property: "value", kind: pluginFlagBool, positional: true, positionalIndex: 0}},
		{name: "invalid default", binding: pluginFlagBinding{property: "value", kind: pluginFlagInt, defaultProvided: true, defaultValue: "bad"}},
		{
			name:    "invalid env",
			prepare: func(t *testing.T, _ *cobra.Command) { t.Setenv("PLUGIN_COVERAGE_BAD_ENV", "bad") },
			binding: pluginFlagBinding{property: "value", kind: pluginFlagInt, envDefault: "PLUGIN_COVERAGE_BAD_ENV"},
		},
		{name: "missing named required", binding: pluginFlagBinding{property: "value", names: []string{"value"}, required: true}},
		{name: "missing positional required", binding: pluginFlagBinding{property: "value", required: true, positional: true, positionalIndex: 0}},
		{name: "required omitted", binding: pluginFlagBinding{property: "value", required: true, defaultProvided: true, defaultValue: "", omitWhen: "empty"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "leaf"}
			if testCase.prepare != nil {
				testCase.prepare(t, cmd)
			}
			if err := collectPluginBindings(cmd, testCase.args, []pluginFlagBinding{testCase.binding}, testCase.params); err == nil {
				t.Fatal("expected binding error")
			}
		})
	}

	params := map[string]any{"value": ""}
	if err := collectPluginBindings(&cobra.Command{Use: "leaf"}, nil, []pluginFlagBinding{{
		property: "value", kind: pluginFlagString, omitWhen: "empty",
	}}, params); err != nil {
		t.Fatal(err)
	}
	if _, exists := params["value"]; exists {
		t.Fatal("optional empty value was not omitted")
	}
}

func TestPluginValueAndNamingHelpers(t *testing.T) {
	parseCases := []struct {
		kind pluginFlagKind
		raw  string
		want any
	}{
		{pluginFlagInt, " 2 ", 2},
		{pluginFlagFloat, " 2.5 ", 2.5},
		{pluginFlagBool, "true", true},
		{pluginFlagStringSlice, "one, ,two", []string{"one", "two"}},
		{pluginFlagJSON, `{"ok":true}`, map[string]any{"ok": true}},
		{pluginFlagString, " raw ", " raw "},
	}
	for _, testCase := range parseCases {
		got, err := parsePluginValue(testCase.raw, testCase.kind)
		if err != nil || !reflect.DeepEqual(got, testCase.want) {
			t.Fatalf("parse %q = (%#v, %v), want %#v", testCase.raw, got, err, testCase.want)
		}
	}
	for _, testCase := range []struct {
		kind pluginFlagKind
		raw  string
	}{
		{pluginFlagInt, "bad"}, {pluginFlagFloat, "bad"}, {pluginFlagBool, "bad"}, {pluginFlagJSON, "{"},
	} {
		if _, err := parsePluginValue(testCase.raw, testCase.kind); err == nil {
			t.Fatalf("invalid %q was accepted", testCase.raw)
		}
	}

	omitCases := []struct {
		value any
		mode  string
		want  bool
	}{
		{nil, "", true}, {" ", "", true}, {[]string{}, "", true},
		{"", "never", false}, {false, "zero", true}, {0, "zero", true},
		{float64(0), "zero", true}, {true, "zero", false}, {1, "zero", false},
		{float64(1), "zero", false}, {[]any{}, "zero", true}, {map[string]any{}, "zero", true},
		{[]any{"value"}, "zero", false}, {map[string]any{"value": true}, "zero", false},
		{struct{}{}, "zero", false}, {false, "", false},
	}
	for _, testCase := range omitCases {
		if got := shouldOmitPluginValue(testCase.value, testCase.mode); got != testCase.want {
			t.Fatalf("omit (%#v, %q) = %v, want %v", testCase.value, testCase.mode, got, testCase.want)
		}
	}

	wrapPluginParams(nil, "body")
	untouched := map[string]any{"value": 1}
	wrapPluginParams(untouched, " ")
	wrapped := map[string]any{"body": map[string]any{"old": 1}, "value": 2, "_meta": 3}
	wrapPluginParams(wrapped, "body")
	wantWrapped := map[string]any{"body": map[string]any{"old": 1, "value": 2}, "_meta": 3}
	if !reflect.DeepEqual(wrapped, wantWrapped) {
		t.Fatalf("wrapped = %#v, want %#v", wrapped, wantWrapped)
	}

	kinds := map[string]pluginFlagKind{
		"int": pluginFlagInt, "integer": pluginFlagInt,
		"float": pluginFlagFloat, "float64": pluginFlagFloat, "number": pluginFlagFloat,
		"bool": pluginFlagBool, "boolean": pluginFlagBool,
		"stringSlice": pluginFlagStringSlice, "string_slice": pluginFlagStringSlice,
		"array": pluginFlagStringSlice, "[]string": pluginFlagStringSlice,
		"json": pluginFlagJSON, "object": pluginFlagJSON, "unknown": pluginFlagString,
	}
	for raw, want := range kinds {
		if got := pluginFlagKindFromString(raw); got != want {
			t.Fatalf("kind %q = %v, want %v", raw, got, want)
		}
	}

	used := map[string]bool{}
	reserved := map[string]bool{"r": true}
	if got := safePluginShorthand(" x ", used, reserved); got != "x" || !used["x"] {
		t.Fatalf("safe shorthand = %q / %#v", got, used)
	}
	for _, raw := range []string{"", "xy", "x", "r"} {
		if got := safePluginShorthand(raw, used, reserved); got != "" {
			t.Fatalf("unsafe shorthand %q = %q", raw, got)
		}
	}

	baseReservations := pluginReservedFlags(nil)
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().StringP("custom", "c", "", "")
	rootReservations := pluginReservedFlags(root)
	if !baseReservations.names["yes"] || !rootReservations.names["custom"] || !rootReservations.shorthands["c"] {
		t.Fatalf("reservations = %#v / %#v", baseReservations, rootReservations)
	}

	if got := safePluginAliases([]string{"", "help", "auth", "cmd", "cmd", "ok", "Bad"}, "cmd"); !reflect.DeepEqual(got, []string{"ok"}) {
		t.Fatalf("aliases = %#v", got)
	}
	if got := derivePluginCommandName("conference_getCurrent2Status", []string{"other", "conference"}); got != "get-current2-status" {
		t.Fatalf("derived name = %q", got)
	}
	if got := pluginKebabName(" HTTP2.Foo_bar baz@ "); got != "http2-foo-bar-baz@" {
		t.Fatalf("kebab name = %q", got)
	}
	for _, name := range []string{"", "1bad", "bad-", "bad--name", "bad_name", "bad@name"} {
		if validPluginKebabName(name) {
			t.Fatalf("invalid kebab name %q was accepted", name)
		}
	}
	if !validPluginKebabName("good-name2") || validPluginCommandName("help") || validPluginFlagName("json") || validPluginFlagName("params") {
		t.Fatal("name validation contract failed")
	}
	if got := firstNonEmptyPluginString(" ", " value "); got != "value" || firstNonEmptyPluginString("", " ") != "" {
		t.Fatal("first non-empty string contract failed")
	}
}

func TestPluginConstraintGroupAndRootHelpers(t *testing.T) {
	cmd := &cobra.Command{Use: "leaf"}
	for _, name := range []string{"a", "b", "c"} {
		cmd.Flags().String(name, "", "")
	}
	applyPluginFlagConstraints(cmd, mcptypes.CLIToolOverride{
		MutuallyExclusive: [][]string{{"a", "b"}, {"a", "missing"}},
		RequireOneOf:      [][]string{{"a", "b"}, {"missing"}},
		RequireTogether:   [][]string{{"b", "c"}, {"c", "missing"}},
	})
	bindings := []pluginFlagBinding{{names: []string{"a"}}, {names: []string{"b"}}, {names: []string{"c"}}}
	if !validPluginFlagConstraints(bindings, mcptypes.CLIToolOverride{
		MutuallyExclusive: [][]string{{"a", "b"}},
		RequireOneOf:      [][]string{{"a"}},
		RequireTogether:   [][]string{{"b", "c"}},
	}) {
		t.Fatal("valid plugin constraints were rejected")
	}
	for _, invalid := range []mcptypes.CLIToolOverride{
		{MutuallyExclusive: [][]string{{"a"}}},
		{RequireOneOf: [][]string{{"missing"}}},
		{RequireTogether: [][]string{{"a", "a"}}},
	} {
		if validPluginFlagConstraints(bindings, invalid) {
			t.Fatalf("invalid plugin constraints were accepted: %#v", invalid)
		}
	}

	groups := map[string]*cobra.Command{}
	root := &cobra.Command{Use: "root"}
	group := ensurePluginGroup(root, "parent.child", "child description", groups)
	if group.Name() != "child" || group.Short != "child description" || !cmdutil.IsPluginSourced(group) {
		t.Fatalf("group = %#v", group)
	}
	if again := ensurePluginGroup(root, "parent.child", "ignored", groups); again != group {
		t.Fatal("existing group was not reused")
	}
	for _, invalid := range []string{"safe.bad_name", "_bad", ".parent", "parent."} {
		if got := ensurePluginGroup(root, invalid, "invalid", groups); got != nil {
			t.Fatalf("invalid group path %q produced %#v", invalid, got)
		}
	}

	mergePluginRoot(nil, root)
	mergePluginRoot(root, nil)
	destination := &cobra.Command{Use: "plugin", Aliases: []string{"one"}}
	source := &cobra.Command{Use: "plugin", Aliases: []string{"one", "two"}}
	source.AddCommand(&cobra.Command{Use: "leaf"})
	mergePluginRoot(destination, source)
	if !reflect.DeepEqual(destination.Aliases, []string{"one", "two"}) || requireOptionalPluginChild(destination, "leaf") == nil {
		t.Fatalf("merged root = %#v", destination)
	}

	pruneEmptyPluginGroups(nil)
	pruneRoot := &cobra.Command{Use: "root"}
	empty := cobracmd.NewGroupCommand("empty", "empty")
	nonEmpty := cobracmd.NewGroupCommand("non-empty", "non-empty")
	nonEmpty.AddCommand(&cobra.Command{Use: "leaf"})
	pruneRoot.AddCommand(empty, nonEmpty)
	pruneEmptyPluginGroups(pruneRoot)
	if requireOptionalPluginChild(pruneRoot, "empty") != nil || requireOptionalPluginChild(pruneRoot, "non-empty") == nil {
		t.Fatal("empty plugin groups were not pruned correctly")
	}

	if pluginRootBoolFlag(nil, "yes") {
		t.Fatal("nil command reported a root flag")
	}
	noFlag := &cobra.Command{Use: "root"}
	if pluginRootBoolFlag(noFlag, "yes") {
		t.Fatal("missing flag reported true")
	}
	wrongType := &cobra.Command{Use: "root"}
	wrongType.PersistentFlags().String("yes", "true", "")
	if pluginRootBoolFlag(wrongType, "yes") {
		t.Fatal("wrong flag type reported true")
	}
	boolRoot := &cobra.Command{Use: "root"}
	boolRoot.PersistentFlags().Bool("yes", false, "")
	if err := boolRoot.PersistentFlags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if !pluginRootBoolFlag(boolRoot, "yes") {
		t.Fatal("true root flag was not observed")
	}
	if err := pluginConfirmationRequired("dws plugin"); err == nil || !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("confirmation error = %v", err)
	}
}

func TestUnsupportedPluginSemanticsReportEveryField(t *testing.T) {
	overlays := []struct {
		value mcptypes.CLIOverlay
		want  string
	}{
		{mcptypes.CLIOverlay{Parent: "root"}, "parent"},
		{mcptypes.CLIOverlay{Group: "group"}, "group"},
		{mcptypes.CLIOverlay{ServerDeps: []string{"other"}}, "serverDeps"},
		{mcptypes.CLIOverlay{Hints: map[string]json.RawMessage{"x": json.RawMessage(`{}`)}}, "hintCommands"},
		{mcptypes.CLIOverlay{RedirectTo: "other"}, "redirectTo"},
		{mcptypes.CLIOverlay{}, ""},
	}
	for _, testCase := range overlays {
		if got := unsupportedPluginOverlay(testCase.value); got != testCase.want {
			t.Fatalf("unsupported overlay = %q, want %q", got, testCase.want)
		}
	}

	tools := []struct {
		value mcptypes.CLIToolOverride
		want  string
	}{
		{mcptypes.CLIToolOverride{CLIAliases: []string{"x"}}, "cliAliases"},
		{mcptypes.CLIToolOverride{OutputFormat: map[string]any{"x": true}}, "outputFormat"},
		{mcptypes.CLIToolOverride{ServerOverride: "other"}, "serverOverride"},
		{mcptypes.CLIToolOverride{RedirectTo: "x"}, "redirectTo"},
		{mcptypes.CLIToolOverride{Pipeline: []json.RawMessage{json.RawMessage(`{}`)}}, "pipeline"},
		{mcptypes.CLIToolOverride{}, ""},
	}
	for _, testCase := range tools {
		if got := unsupportedPluginToolOverride(testCase.value); got != testCase.want {
			t.Fatalf("unsupported tool = %q, want %q", got, testCase.want)
		}
	}

	flags := []struct {
		value mcptypes.CLIFlagOverride
		want  string
	}{
		{mcptypes.CLIFlagOverride{MapsTo: "x"}, "mapsTo"},
		{mcptypes.CLIFlagOverride{Transform: "x"}, "transform"},
		{mcptypes.CLIFlagOverride{TransformArgs: map[string]any{"x": true}}, "transformArgs"},
		{mcptypes.CLIFlagOverride{RuntimeDefault: "x"}, "runtimeDefault"},
		{mcptypes.CLIFlagOverride{PipelineLocal: true}, "pipelineLocal"},
		{mcptypes.CLIFlagOverride{Type: "mystery"}, "type"},
		{mcptypes.CLIFlagOverride{OmitWhen: "sometimes"}, "omitWhen"},
		{mcptypes.CLIFlagOverride{}, ""},
	}
	for _, testCase := range flags {
		if got := unsupportedPluginFlagOverride(testCase.value); got != testCase.want {
			t.Fatalf("unsupported flag = %q, want %q", got, testCase.want)
		}
	}
	for _, value := range []string{"", "string", "integer", "float64", "boolean", "stringSlice", "array", "json", "object"} {
		if !supportedPluginFlagType(value) {
			t.Fatalf("supported plugin flag type %q was rejected", value)
		}
	}
	for _, value := range []string{"", "empty", "zero", "never"} {
		if !supportedPluginOmitMode(value) {
			t.Fatalf("supported plugin omit mode %q was rejected", value)
		}
	}
}

func TestUnsupportedPluginDescriptorRejectsEveryInvalidLayer(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*mcptypes.ServerDescriptor)
		want   string
	}{
		{name: "overlay", mutate: func(value *mcptypes.ServerDescriptor) { value.CLI.Parent = "root" }, want: "parent"},
		{name: "no tools", mutate: func(value *mcptypes.ServerDescriptor) { value.CLI.ToolOverrides = nil }, want: ""},
		{name: "root", mutate: func(value *mcptypes.ServerDescriptor) { value.CLI.Command = "Bad" }, want: "command"},
		{name: "declared group", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.Groups = map[string]mcptypes.CLIGroupDef{"bad_name": {}}
		}, want: "groups"},
		{name: "blank tool", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"": {}}
		}, want: "tool"},
		{name: "tool semantics", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"tool": {ServerOverride: "drive"}}
		}, want: "serverOverride"},
		{name: "hidden tool", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"tool": {Hidden: true, ServerOverride: "drive"}}
		}, want: ""},
		{name: "derived leaf", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"conference_derived_tool": {}}
		}, want: ""},
		{name: "leaf", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"tool": {CLIName: "Bad"}}
		}, want: "cliName"},
		{name: "leaf group", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"tool": {CLIName: "leaf", Group: "bad_name"}}
		}, want: "group"},
		{name: "flags", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"tool": {
				CLIName: "leaf",
				Flags:   map[string]mcptypes.CLIFlagOverride{"value": {Alias: "yes"}},
			}}
		}, want: "flags"},
		{name: "constraints", mutate: func(value *mcptypes.ServerDescriptor) {
			value.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{"tool": {
				CLIName:         "leaf",
				Flags:           map[string]mcptypes.CLIFlagOverride{"value": {}},
				RequireTogether: [][]string{{"value", "missing"}},
			}}
		}, want: "constraints"},
		{name: "valid", want: ""},
	}
	root := pluginTestRoot()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			descriptor := conferencePluginDescriptor()
			if testCase.mutate != nil {
				testCase.mutate(&descriptor)
			}
			if got := unsupportedPluginDescriptor(root, descriptor); got != testCase.want {
				t.Fatalf("unsupported descriptor = %q, want %q", got, testCase.want)
			}
		})
	}
}

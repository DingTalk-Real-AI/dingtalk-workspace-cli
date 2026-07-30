// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"context"
	"errors"
	"flag"
	"go/format"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestParamAliasGeneratorUsesDistributionOwnedCommandTree(t *testing.T) {
	got := reflect.ValueOf(newParamAliasRoot).Pointer()
	want := reflect.ValueOf(app.NewSchemaSourceRootCommand).Pointer()
	if got != want {
		t.Fatal("parameter alias generator must use the distribution-owned Schema source command tree")
	}
}

func preserveParamAliasGeneratorGlobals(t *testing.T) {
	t.Helper()
	oldArgs, oldFlags := os.Args, flag.CommandLine
	oldRoot := newParamAliasRoot
	oldReduce := reduceParamAliasEntries
	oldFormat := formatParamAliasSource
	oldWrite := writeParamAliasFile
	oldExit := exitParamAliasProcess
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
		newParamAliasRoot = oldRoot
		reduceParamAliasEntries = oldReduce
		formatParamAliasSource = oldFormat
		writeParamAliasFile = oldWrite
		exitParamAliasProcess = oldExit
	})
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCrossPlatformCoverageParamAliasMainWritesGeneratedFile(t *testing.T) {
	preserveParamAliasGeneratorGlobals(t)
	output := filepath.Join(t.TempDir(), "param_aliases_generated.go")
	flag.CommandLine = flag.NewFlagSet("param-alias-success", flag.ContinueOnError)
	os.Args = []string{"cmd_param_aliases", "-root", repositoryRoot(t), "-output", output}
	newParamAliasRoot = func(...context.Context) *cobra.Command { return &cobra.Command{Use: "dws"} }
	reduceParamAliasEntries = func(*cobra.Command) ([]cli.ParamAliasEntry, error) {
		return []cli.ParamAliasEntry{{CLIPath: "demo get", Aliases: map[string]string{"uid": "user"}}}, nil
	}

	main()
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "func loadGeneratedParamAliases() []ParamAliasEntry") ||
		!strings.Contains(string(data), `"uid": "user"`) {
		t.Fatalf("generated source missing runtime table:\n%s", data)
	}
}

func TestCrossPlatformCoverageParamAliasMainReportsFailure(t *testing.T) {
	preserveParamAliasGeneratorGlobals(t)
	flag.CommandLine = flag.NewFlagSet("param-alias-failure", flag.ContinueOnError)
	os.Args = []string{
		"cmd_param_aliases",
		"-root", repositoryRoot(t),
		"-output", filepath.Join(repositoryRoot(t), "internal", "cli", "param_concepts.json"),
	}
	exitParamAliasProcess = func(code int) { panic(code) }
	defer func() {
		if recovered := recover(); recovered != 1 {
			t.Fatalf("main exit = %#v, want 1", recovered)
		}
	}()
	main()
}

func TestGenerateParamAliasesFailurePaths(t *testing.T) {
	preserveParamAliasGeneratorGlobals(t)
	root := repositoryRoot(t)
	output := filepath.Join(t.TempDir(), "param_aliases_generated.go")
	newParamAliasRoot = func(...context.Context) *cobra.Command { return &cobra.Command{Use: "dws"} }

	if err := generateParamAliases(root, filepath.Join(root, "internal", "cli", "param_concepts.json")); err == nil {
		t.Fatal("generateParamAliases accepted an output that overlaps a reviewed input")
	}

	reduceParamAliasEntries = func(*cobra.Command) ([]cli.ParamAliasEntry, error) {
		return nil, errors.New("reduce")
	}
	if err := generateParamAliases(root, output); err == nil || !strings.Contains(err.Error(), "reduce") {
		t.Fatalf("reduction error = %v", err)
	}

	reduceParamAliasEntries = func(*cobra.Command) ([]cli.ParamAliasEntry, error) { return nil, nil }
	formatParamAliasSource = func([]byte) ([]byte, error) { return nil, errors.New("format") }
	if err := generateParamAliases(root, output); err == nil || !strings.Contains(err.Error(), "format generated") {
		t.Fatalf("format error = %v", err)
	}

	formatParamAliasSource = format.Source
	writeParamAliasFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := generateParamAliases(root, output); err == nil || !strings.Contains(err.Error(), "write generated") {
		t.Fatalf("write error = %v", err)
	}
}

func TestValidateParamAliasOutputIsolation(t *testing.T) {
	root := repositoryRoot(t)
	if err := validateOutputIsolation(root, filepath.Join(t.TempDir(), "aliases.go")); err != nil {
		t.Fatalf("temporary output rejected: %v", err)
	}
	if err := validateOutputIsolation(root, filepath.Join(root, "internal", "cli", "not_a_delivery_target.go")); err == nil ||
		!strings.Contains(err.Error(), "not a canonical generated delivery target") {
		t.Fatalf("non-canonical repository output error = %v", err)
	}
}

func TestRenderParamAliasesDeterministicShape(t *testing.T) {
	preserveParamAliasGeneratorGlobals(t)
	entries := []cli.ParamAliasEntry{
		{
			CLIPath:   "demo get",
			Aliases:   map[string]string{"z-name": "name", "a-name": "name"},
			Blocked:   []string{"page", "count"},
			Ambiguous: []string{"user-id"},
		},
		{CLIPath: "demo empty"},
	}
	data, err := renderParamAliases(entries)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Index(text, `"a-name": "name"`) > strings.Index(text, `"z-name": "name"`) {
		t.Fatalf("alias keys are not sorted:\n%s", text)
	}
	for _, want := range []string{
		"func loadGeneratedParamAliases() []ParamAliasEntry",
		`Blocked:   []string{"page", "count"}`,
		`Ambiguous: []string{"user-id"}`,
		`CLIPath: "demo empty"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated source missing %q:\n%s", want, text)
		}
	}

	empty, err := renderParamAliases(nil)
	if err != nil || !strings.Contains(string(empty), "var generatedParamAliases = []ParamAliasEntry{") {
		t.Fatalf("empty render = %q, error = %v", empty, err)
	}
	if got := renderStringSlice(nil); got != "[]string{}" {
		t.Fatalf("renderStringSlice(nil) = %q", got)
	}
	if got := sortedKeys(nil); len(got) != 0 {
		t.Fatalf("sortedKeys(nil) = %v", got)
	}
	if got := sortedKeys(map[string]string{"b": "2", "a": "1"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("sortedKeys() = %v", got)
	}

	formatParamAliasSource = func([]byte) ([]byte, error) { return nil, errors.New("broken formatter") }
	if _, err := renderParamAliases(entries); err == nil || !strings.Contains(err.Error(), "broken formatter") {
		t.Fatalf("formatter error = %v", err)
	}
}

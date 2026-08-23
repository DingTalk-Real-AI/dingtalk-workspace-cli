// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

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

func TestCrossPlatformCoverageCommandFallbackGeneratorUsesDistributionOwnedCommandTree(t *testing.T) {
	got := reflect.ValueOf(newCommandFallbackRoot).Pointer()
	want := reflect.ValueOf(app.NewSchemaSourceRootCommand).Pointer()
	if got != want {
		t.Fatal("command fallback generator must use the distribution-owned Schema source command tree")
	}
}

func preserveCommandFallbackGeneratorGlobals(t *testing.T) {
	t.Helper()
	oldArgs, oldFlags := os.Args, flag.CommandLine
	oldRoot := newCommandFallbackRoot
	oldReduce := reduceCommandFallbackEntries
	oldFormat := formatCommandFallbackSource
	oldWrite := writeCommandFallbackFile
	oldExit := exitCommandFallbackProcess
	t.Cleanup(func() {
		os.Args, flag.CommandLine = oldArgs, oldFlags
		newCommandFallbackRoot = oldRoot
		reduceCommandFallbackEntries = oldReduce
		formatCommandFallbackSource = oldFormat
		writeCommandFallbackFile = oldWrite
		exitCommandFallbackProcess = oldExit
	})
}

func commandFallbackRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCrossPlatformCoverageCommandFallbackMainWritesGeneratedFile(t *testing.T) {
	preserveCommandFallbackGeneratorGlobals(t)
	output := filepath.Join(t.TempDir(), "command_path_fallbacks_generated.go")
	flag.CommandLine = flag.NewFlagSet("command-fallback-success", flag.ContinueOnError)
	os.Args = []string{"cmd_command_path_fallbacks", "-root", commandFallbackRepositoryRoot(t), "-output", output}
	newCommandFallbackRoot = func(...context.Context) *cobra.Command { return &cobra.Command{Use: "dws"} }
	reduceCommandFallbackEntries = func(*cobra.Command) ([]cli.CommandPathFallback, error) {
		return []cli.CommandPathFallback{{
			From:         "demo +bad",
			Mode:         cli.CommandPathFallbackRewrite,
			To:           "demo +good",
			Reviewed:     true,
			ReviewReason: "fixture",
		}}, nil
	}

	main()
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"func loadGeneratedCommandPathFallbacks() []CommandPathFallback",
		`From:         "demo +bad"`,
		`To:           "demo +good"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated source missing %q:\n%s", want, text)
		}
	}
}

func TestCrossPlatformCoverageGenerateCommandPathFallbackFailurePaths(t *testing.T) {
	preserveCommandFallbackGeneratorGlobals(t)
	root := commandFallbackRepositoryRoot(t)
	output := filepath.Join(t.TempDir(), "command_path_fallbacks_generated.go")
	newCommandFallbackRoot = func(...context.Context) *cobra.Command { return &cobra.Command{Use: "dws"} }

	if err := generateCommandPathFallbacks(root, filepath.Join(root, "internal", "cli", "command_path_fallbacks.json")); err == nil {
		t.Fatal("generator accepted output overlapping reviewed input")
	}
	reduceCommandFallbackEntries = func(*cobra.Command) ([]cli.CommandPathFallback, error) {
		return nil, errors.New("reduce")
	}
	if err := generateCommandPathFallbacks(root, output); err == nil || !strings.Contains(err.Error(), "reduce") {
		t.Fatalf("reduction error = %v", err)
	}
	reduceCommandFallbackEntries = func(*cobra.Command) ([]cli.CommandPathFallback, error) { return nil, nil }
	formatCommandFallbackSource = func([]byte) ([]byte, error) { return nil, errors.New("format") }
	if err := generateCommandPathFallbacks(root, output); err == nil || !strings.Contains(err.Error(), "format generated") {
		t.Fatalf("format error = %v", err)
	}
	formatCommandFallbackSource = format.Source
	writeCommandFallbackFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := generateCommandPathFallbacks(root, output); err == nil || !strings.Contains(err.Error(), "write generated") {
		t.Fatalf("write error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateCommandFallbackOutputIsolation(t *testing.T) {
	root := commandFallbackRepositoryRoot(t)
	if err := validateOutputIsolation(root, filepath.Join(t.TempDir(), "fallbacks.go")); err != nil {
		t.Fatalf("temporary output rejected: %v", err)
	}
	if err := validateOutputIsolation(root, filepath.Join(root, "internal", "cli", "not_a_delivery_target.go")); err == nil ||
		!strings.Contains(err.Error(), "not a canonical generated delivery target") {
		t.Fatalf("non-canonical repository output error = %v", err)
	}
}

func TestCrossPlatformCoverageRenderCommandPathFallbacksShape(t *testing.T) {
	preserveCommandFallbackGeneratorGlobals(t)
	entries := []cli.CommandPathFallback{
		{
			From:         "demo +bad",
			Mode:         cli.CommandPathFallbackRewrite,
			To:           "demo +good",
			Reviewed:     true,
			ReviewReason: "rewrite fixture",
		},
		{
			From:         "demo +choose",
			Mode:         cli.CommandPathFallbackAmbiguous,
			Candidates:   []string{"demo +one", "demo +two"},
			Reviewed:     true,
			ReviewReason: "ambiguous fixture",
		},
	}
	data, err := renderCommandPathFallbacks(entries)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`Mode:         "rewrite"`,
		`Candidates:   []string{"demo +one", "demo +two"}`,
		`ReviewReason: "ambiguous fixture"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("render missing %q:\n%s", want, text)
		}
	}
	if got := renderStringSlice(nil); got != "[]string{}" {
		t.Fatalf("renderStringSlice(nil) = %q", got)
	}
	formatCommandFallbackSource = func([]byte) ([]byte, error) { return nil, errors.New("broken formatter") }
	if _, err := renderCommandPathFallbacks(entries); err == nil || !strings.Contains(err.Error(), "broken formatter") {
		t.Fatalf("formatter error = %v", err)
	}
}

func TestCrossPlatformCoverageCommandFallbackMainReportsFailure(t *testing.T) {
	preserveCommandFallbackGeneratorGlobals(t)
	flag.CommandLine = flag.NewFlagSet("command-fallback-failure", flag.ContinueOnError)
	os.Args = []string{
		"cmd_command_path_fallbacks",
		"-root", commandFallbackRepositoryRoot(t),
		"-output", filepath.Join(commandFallbackRepositoryRoot(t), "internal", "cli", "command_path_fallbacks.json"),
	}
	exitCommandFallbackProcess = func(code int) { panic(code) }
	defer func() {
		if recovered := recover(); recovered != 1 {
			t.Fatalf("main exit = %#v, want 1", recovered)
		}
	}()
	main()
}

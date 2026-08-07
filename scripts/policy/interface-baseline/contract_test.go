// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunModes(t *testing.T) {
	var generated, stderr bytes.Buffer
	if code := run(nil, testRoot(), &generated, &stderr); code != 0 {
		t.Fatalf("generate code=%d stderr=%s", code, stderr.String())
	}
	baseline := filepath.Join(t.TempDir(), "baseline.txt")
	if err := os.WriteFile(baseline, generated.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	stderr.Reset()
	if code := run([]string{"--check", baseline}, testRoot(), &stdout, &stderr); code != 0 {
		t.Fatalf("check code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "compatibility check: ok") {
		t.Fatalf("unexpected check output %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--merge", baseline}, testRoot(), &stdout, &stderr); code != 0 {
		t.Fatalf("merge code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[old]") {
		t.Fatalf("unexpected merge output %q", stdout.String())
	}

	stderr.Reset()
	if code := run([]string{"--check", baseline, "--merge", baseline}, testRoot(), &stdout, &stderr); code != 2 {
		t.Fatalf("conflicting modes code=%d, want 2", code)
	}

	stderr.Reset()
	missingRoot := &cobra.Command{Use: "dws"}
	missingRoot.InitDefaultHelpCmd()
	if code := run([]string{"--check", baseline}, missingRoot, &stdout, &stderr); code != 1 {
		t.Fatalf("incompatible check code=%d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "historical command") {
		t.Fatalf("unexpected incompatible output %q", stderr.String())
	}

	stderr.Reset()
	if code := run([]string{"--check", filepath.Join(t.TempDir(), "missing")}, testRoot(), &stdout, &stderr); code != 2 {
		t.Fatalf("missing baseline code=%d, want 2", code)
	}
	stderr.Reset()
	if code := run([]string{"--unknown"}, testRoot(), &stdout, &stderr); code != 2 {
		t.Fatalf("unknown flag code=%d, want 2", code)
	}
}

func TestCompatibilityAllowsAdditions(t *testing.T) {
	root := testRoot()
	baseline, err := parseContract([]byte("[root]\n  commands: old\n\n[old]\n  flags: -n/--name:string, -h/--help:bool\n"))
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(root, baseline); len(failures) != 0 {
		t.Fatalf("additions should be compatible: %v", failures)
	}
}

func TestCompatibilityTreatsLegacyMetadataAsUnknown(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	old := &cobra.Command{Use: "old", Run: func(*cobra.Command, []string) {}}
	old.Flags().String("required", "", "required")
	if err := old.MarkFlagRequired("required"); err != nil {
		t.Fatal(err)
	}
	root.AddCommand(old)
	root.InitDefaultHelpCmd()
	baseline, err := parseContract([]byte("[root]\n  commands: old\n\n[old]\n  flags: --required:string\n"))
	if err != nil {
		t.Fatal(err)
	}
	if failures := checkCompatibility(root, baseline); len(failures) != 0 {
		t.Fatalf("legacy metadata should be unknown: %v", failures)
	}
}

func TestCompatibilityRejectsMissingCommandAndFlag(t *testing.T) {
	root := testRoot()
	baseline, err := parseContract([]byte("[root]\n\n[removed]\n  flags: --gone:string\n\n[old]\n  flags: --gone:string\n"))
	if err != nil {
		t.Fatal(err)
	}
	failures := checkCompatibility(root, baseline)
	if len(failures) != 2 {
		t.Fatalf("got %d failures, want 2: %v", len(failures), failures)
	}
}

func TestCompatibilityAllowsNewShorthandButRejectsRemovedShorthand(t *testing.T) {
	root := testRoot()
	baseline, _ := parseContract([]byte("[root]\n\n[old]\n  flags: --name:string\n"))
	if failures := checkCompatibility(root, baseline); len(failures) != 0 {
		t.Fatalf("new shorthand should be compatible: %v", failures)
	}

	baseline, _ = parseContract([]byte("[root]\n\n[old]\n  flags: -x/--name:string\n"))
	if failures := checkCompatibility(root, baseline); len(failures) != 1 {
		t.Fatalf("removed shorthand should fail: %v", failures)
	}
}

func TestCompatibilityRejectsCommandContractRegressions(t *testing.T) {
	baselineRoot := &cobra.Command{Use: "dws"}
	baselineRoot.AddCommand(&cobra.Command{Use: "old", Run: func(*cobra.Command, []string) {}})
	baselineRoot.InitDefaultHelpCmd()
	baseline := snapshot(baselineRoot)

	tests := []struct {
		name   string
		mutate func(*cobra.Command)
		want   string
	}{
		{
			name: "runnable to non-runnable",
			mutate: func(command *cobra.Command) {
				command.Run = nil
			},
			want: "became non-runnable",
		},
		{
			name: "visible to hidden",
			mutate: func(command *cobra.Command) {
				command.Hidden = true
			},
			want: "became hidden",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			currentRoot := &cobra.Command{Use: "dws"}
			current := &cobra.Command{Use: "old", Run: func(*cobra.Command, []string) {}}
			test.mutate(current)
			currentRoot.AddCommand(current)
			currentRoot.InitDefaultHelpCmd()
			assertFailureContains(t, checkCompatibility(currentRoot, baseline), test.want)
		})
	}
}

func TestCompatibilityRejectsFlagContractRegressions(t *testing.T) {
	newRoot := func(persistent bool) (*cobra.Command, *cobra.Command) {
		root := &cobra.Command{Use: "dws"}
		old := &cobra.Command{Use: "old"}
		if persistent {
			old.PersistentFlags().Bool("toggle", false, "toggle")
		} else {
			old.Flags().Bool("toggle", false, "toggle")
		}
		root.AddCommand(old)
		root.InitDefaultHelpCmd()
		return root, old
	}

	baselineRoot, _ := newRoot(true)
	baseline := snapshot(baselineRoot)
	tests := []struct {
		name   string
		mutate func(*cobra.Command)
		want   string
	}{
		{
			name: "optional to required",
			mutate: func(command *cobra.Command) {
				_ = command.MarkPersistentFlagRequired("toggle")
			},
			want: "became required",
		},
		{
			name: "visible to hidden",
			mutate: func(command *cobra.Command) {
				_ = command.PersistentFlags().MarkHidden("toggle")
			},
			want: "became hidden",
		},
		{
			name: "no-opt changed",
			mutate: func(command *cobra.Command) {
				command.PersistentFlags().Lookup("toggle").NoOptDefVal = "false"
			},
			want: "changed no-opt value",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			currentRoot, current := newRoot(true)
			test.mutate(current)
			assertFailureContains(t, checkCompatibility(currentRoot, baseline), test.want)
		})
	}

	currentRoot, _ := newRoot(false)
	assertFailureContains(t, checkCompatibility(currentRoot, baseline), "narrowed persistent scope")
}

func TestCompatibilityRejectsNewRequiredFlag(t *testing.T) {
	baselineRoot := testRoot()
	baseline := snapshot(baselineRoot)
	currentRoot := testRoot()
	old, _, err := currentRoot.Find([]string{"old"})
	if err != nil {
		t.Fatal(err)
	}
	old.Flags().String("required-new", "", "required")
	if err := old.MarkFlagRequired("required-new"); err != nil {
		t.Fatal(err)
	}
	assertFailureContains(t, checkCompatibility(currentRoot, baseline), "added required flag")
}

func assertFailureContains(t *testing.T, failures []string, want string) {
	t.Helper()
	for _, failure := range failures {
		if strings.Contains(failure, want) {
			return
		}
	}
	t.Fatalf("failures %v do not contain %q", failures, want)
}

func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	old := &cobra.Command{Use: "old"}
	old.Flags().StringP("name", "n", "", "name")
	old.Flags().String("extra", "", "addition")
	root.AddCommand(old, &cobra.Command{Use: "new"})
	root.InitDefaultHelpCmd()
	return root
}

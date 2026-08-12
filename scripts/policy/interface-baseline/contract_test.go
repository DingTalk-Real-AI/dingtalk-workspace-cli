// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
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

// policyRoot builds "dws old" with a single --policy flag declared by declare,
// so a baseline and a candidate can differ only in how that flag is typed.
func policyRoot(declare func(*cobra.Command)) *cobra.Command {
	root := &cobra.Command{Use: "dws"}
	old := &cobra.Command{Use: "old", Run: func(*cobra.Command, []string) {}}
	declare(old)
	root.AddCommand(old)
	root.InitDefaultHelpCmd()
	return root
}

// registerReviewedFixture puts a fixture command in the reviewed table for the
// duration of one test, so the behaviour tests exercise the real lookup instead
// of depending on whichever production entries happen to exist.
func registerReviewedFixture(t *testing.T, change flagTypeChange) {
	t.Helper()
	if _, exists := reviewedFlagTypeChanges[change]; exists {
		t.Fatalf("fixture %+v collides with a production entry", change)
	}
	reviewedFlagTypeChanges[change] = struct{}{}
	t.Cleanup(func() { delete(reviewedFlagTypeChanges, change) })
}

const typeChangedFailure = "changed type from string to int"

func TestReviewedFlagTypeChangeAcceptsRegisteredMigration(t *testing.T) {
	baseline := snapshot(policyRoot(func(command *cobra.Command) {
		command.Flags().String("policy", "", "policy")
	}))
	current := policyRoot(func(command *cobra.Command) {
		command.Flags().Int("policy", 0, "policy")
	})

	// Without an entry the migration is still a blocking change.
	assertFailureContains(t, checkCompatibility(current, baseline), typeChangedFailure)

	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws old", Flag: "policy", From: "string", To: "int"})
	if failures := checkCompatibility(current, baseline); len(failures) != 0 {
		t.Fatalf("a reviewed string->int migration should pass: %v", failures)
	}
}

func TestReviewedFlagTypeChangeIsDirectionSensitive(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws old", Flag: "policy", From: "string", To: "int"})

	// The reverse migration shares command and flag but not direction, so the
	// string->int entry must not admit it.
	baseline := snapshot(policyRoot(func(command *cobra.Command) {
		command.Flags().Int("policy", 0, "policy")
	}))
	current := policyRoot(func(command *cobra.Command) {
		command.Flags().String("policy", "", "policy")
	})
	assertFailureContains(t, checkCompatibility(current, baseline), "changed type from int to string")
}

func TestReviewedFlagTypeChangeRejectsOtherCommandsAndFlags(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws other", Flag: "policy", From: "string", To: "int"})
	baseline := snapshot(policyRoot(func(command *cobra.Command) {
		command.Flags().String("policy", "", "policy")
	}))
	current := policyRoot(func(command *cobra.Command) {
		command.Flags().Int("policy", 0, "policy")
	})
	// Same flag and direction, different command.
	assertFailureContains(t, checkCompatibility(current, baseline), typeChangedFailure)

	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws old", Flag: "elsewhere", From: "string", To: "int"})
	// Same command and direction, different flag.
	assertFailureContains(t, checkCompatibility(current, baseline), typeChangedFailure)
}

// A reviewed migration is only accepted when the rest of the flag's contract
// held still. Each case bundles one unrelated regression with the reviewed type
// change and requires the type failure to reappear, which also anchors
// flagContractOtherwiseChanged against the checks it mirrors: a condition that
// drifts out of sync fails here instead of silently widening the exemption.
func TestReviewedFlagTypeChangeRejectsBundledRegression(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws old", Flag: "policy", From: "string", To: "int"})

	tests := []struct {
		name     string
		baseline func(*cobra.Command)
		current  func(*cobra.Command)
		want     string
	}{
		{
			name:     "became required",
			baseline: func(command *cobra.Command) { command.Flags().String("policy", "", "policy") },
			current: func(command *cobra.Command) {
				command.Flags().Int("policy", 0, "policy")
				_ = command.MarkFlagRequired("policy")
			},
			want: "became required",
		},
		{
			name:     "became hidden",
			baseline: func(command *cobra.Command) { command.Flags().String("policy", "", "policy") },
			current: func(command *cobra.Command) {
				command.Flags().Int("policy", 0, "policy")
				_ = command.Flags().MarkHidden("policy")
			},
			want: "became hidden",
		},
		{
			name:     "lost shorthand",
			baseline: func(command *cobra.Command) { command.Flags().StringP("policy", "p", "", "policy") },
			current:  func(command *cobra.Command) { command.Flags().Int("policy", 0, "policy") },
			want:     "lost shorthand",
		},
		{
			name:     "changed no-opt value",
			baseline: func(command *cobra.Command) { command.Flags().String("policy", "", "policy") },
			current: func(command *cobra.Command) {
				command.Flags().Int("policy", 0, "policy")
				command.Flags().Lookup("policy").NoOptDefVal = "4"
			},
			want: "changed no-opt value",
		},
		{
			name:     "narrowed persistent scope",
			baseline: func(command *cobra.Command) { command.PersistentFlags().String("policy", "", "policy") },
			current:  func(command *cobra.Command) { command.Flags().Int("policy", 0, "policy") },
			want:     "narrowed persistent scope",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failures := checkCompatibility(policyRoot(test.current), snapshot(policyRoot(test.baseline)))
			assertFailureContains(t, failures, test.want)
			assertFailureContains(t, failures, typeChangedFailure)
		})
	}
}

// mergeContracts runs the same admission decision on --merge. A reviewed
// migration keeps merging and the merged entry keeps the historical type, so a
// later --check still resolves the flag through the reviewed table.
func TestMergeAcceptsReviewedFlagTypeChangeAndKeepsHistoricalType(t *testing.T) {
	historical := snapshot(policyRoot(func(command *cobra.Command) {
		command.Flags().String("policy", "", "policy")
	}))
	current := snapshot(policyRoot(func(command *cobra.Command) {
		command.Flags().Int("policy", 0, "policy")
	}))

	if _, failures := mergeContracts(historical, current); len(failures) == 0 {
		t.Fatal("an unreviewed merge should report the type change")
	}

	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws old", Flag: "policy", From: "string", To: "int"})
	merged, failures := mergeContracts(historical, current)
	if len(failures) != 0 {
		t.Fatalf("a reviewed merge should pass: %v", failures)
	}
	if got := merged.Commands["old"].Flags["policy"].Type; got != "string" {
		t.Fatalf("merged type = %q, want the historical %q", got, "string")
	}
}

// mergeContracts applies the same "nothing else moved" condition as
// checkCompatibility, but against two recorded contracts rather than a live
// pflag definition. Each case bundles one unrelated regression with the reviewed
// type change and requires the type failure to reappear, anchoring
// mergedFlagContractOtherwiseChanged against the checks it mirrors.
func TestMergeRejectsReviewedFlagTypeChangeWithBundledRegression(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws old", Flag: "policy", From: "string", To: "int"})

	tests := []struct {
		name       string
		historical func(*cobra.Command)
		current    func(*cobra.Command)
	}{
		{
			name:       "lost shorthand",
			historical: func(command *cobra.Command) { command.Flags().StringP("policy", "p", "", "policy") },
			current:    func(command *cobra.Command) { command.Flags().Int("policy", 0, "policy") },
		},
		{
			name:       "became required",
			historical: func(command *cobra.Command) { command.Flags().String("policy", "", "policy") },
			current: func(command *cobra.Command) {
				command.Flags().Int("policy", 0, "policy")
				_ = command.MarkFlagRequired("policy")
			},
		},
		{
			name:       "became hidden",
			historical: func(command *cobra.Command) { command.Flags().String("policy", "", "policy") },
			current: func(command *cobra.Command) {
				command.Flags().Int("policy", 0, "policy")
				_ = command.Flags().MarkHidden("policy")
			},
		},
		{
			name:       "changed no-opt value",
			historical: func(command *cobra.Command) { command.Flags().String("policy", "", "policy") },
			current: func(command *cobra.Command) {
				command.Flags().Int("policy", 0, "policy")
				command.Flags().Lookup("policy").NoOptDefVal = "4"
			},
		},
		{
			name:       "narrowed persistent scope",
			historical: func(command *cobra.Command) { command.PersistentFlags().String("policy", "", "policy") },
			current:    func(command *cobra.Command) { command.Flags().Int("policy", 0, "policy") },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, failures := mergeContracts(
				snapshot(policyRoot(test.historical)),
				snapshot(policyRoot(test.current)),
			)
			assertFailureContains(t, failures, typeChangedFailure)
		})
	}
}

// The behaviour tests above register their own fixtures, so they cannot catch a
// production entry whose key is spelled in the wrong form — that mistake
// disables the exemption silently and only a real gate run reports it. Recompute
// every key through displayPath instead of trusting the spelling in the table.
func TestReviewedFlagTypeChangeKeysUseDisplayPaths(t *testing.T) {
	if len(reviewedFlagTypeChanges) == 0 {
		t.Fatal("豁免表为空：若确已清空，请连这条守卫一起删除")
	}
	for change := range reviewedFlagTypeChanges {
		if got := displayPath(internalCommandPath(change.CommandPath)); got != change.CommandPath {
			t.Errorf("命令路径不是 displayPath 的规范形态\n  登记: %s\n  规范: %s", change.CommandPath, got)
		}
		if change.Flag == "" || strings.HasPrefix(change.Flag, "-") {
			t.Errorf("%s: flag 名应是裸名（lookupFlag 用裸名查找），登记为 %q", change.CommandPath, change.Flag)
		}
		if change.From == "" || change.To == "" || change.From == change.To {
			t.Errorf("%s --%s: %q -> %q 不构成类型迁移", change.CommandPath, change.Flag, change.From, change.To)
		}
	}
}

// Anchor every production entry to the real command tree. A misspelled command
// or flag name would leave the exemption permanently inert, and the behaviour
// tests cannot see that because they register their own paths.
func TestReviewedFlagTypeChangeEntriesResolveInTheRealCommandTree(t *testing.T) {
	if len(reviewedFlagTypeChanges) == 0 {
		t.Fatal("豁免表为空：若确已清空，请连这条守卫一起删除")
	}
	root := app.NewRootCommand()
	for change := range reviewedFlagTypeChanges {
		command, resolved := resolveCommand(root, internalCommandPath(change.CommandPath))
		if !resolved {
			t.Errorf("%q 在真实命令树中不存在", change.CommandPath)
			continue
		}
		flag, _ := lookupFlag(command, change.Flag)
		if flag == nil {
			t.Errorf("%q 上不存在 --%s", change.CommandPath, change.Flag)
			continue
		}
		// Exactly one side holds depending on whether the migration has landed:
		// From before it does, To afterwards. Asserting membership catches a
		// misspelled type name without failing while the migration is pending.
		if actual := flag.Value.Type(); actual != change.From && actual != change.To {
			t.Errorf("%q --%s 当前类型 %q 既不是 %q 也不是 %q",
				change.CommandPath, change.Flag, actual, change.From, change.To)
		}
	}
}

// internalCommandPath inverts displayPath so a table key can be fed back into
// resolveCommand and displayPath.
func internalCommandPath(displayed string) string {
	if displayed == "dws" {
		return "root"
	}
	return strings.ReplaceAll(strings.TrimPrefix(displayed, "dws "), " ", ".")
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

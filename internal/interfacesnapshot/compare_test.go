// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package interfacesnapshot

import (
	"os"
	"regexp"
	"testing"
)

func TestCrossPlatformCoverageCompareAdmissionPolicy(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "string", false)),
	)

	tests := []struct {
		name       string
		current    Snapshot
		compatible bool
		kind       string
	}{
		{
			name: "command addition allowed",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws search", testFlag("query", "string", false)),
				testCommand("dws status"),
			),
			compatible: true,
		},
		{
			name: "flag addition allowed",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws search",
					testFlag("limit", "int", false),
					testFlag("query", "string", false),
				),
			),
			compatible: true,
		},
		{
			name: "required flag addition blocked on existing command",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws search",
					testFlag("query", "string", false),
					testFlag("tenant", "string", true),
				),
			),
			kind: "required_flag_added",
		},
		{
			name: "command deletion blocked",
			current: testSnapshot(
				testCommand("dws"),
			),
			kind: "command_removed",
		},
		{
			name: "rename without alias blocked",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws find", testFlag("query", "string", false)),
			),
			kind: "command_removed",
		},
		{
			name: "flag deletion blocked",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws search"),
			),
			kind: "flag_removed",
		},
		{
			name: "optional becoming required blocked",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws search", testFlag("query", "string", true)),
			),
			kind: "flag_became_required",
		},
		{
			name: "flag type change blocked",
			current: testSnapshot(
				testCommand("dws"),
				testCommand("dws search", testFlag("query", "stringSlice", false)),
			),
			kind: "flag_type_changed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			comparison := Compare(test.current, base, "base")
			if comparison.Compatible != test.compatible {
				t.Fatalf("Compatible = %v, want %v; blocking=%#v", comparison.Compatible, test.compatible, comparison.Blocking)
			}
			if test.kind == "" {
				return
			}
			if !hasChangeKind(comparison.Blocking, test.kind) {
				t.Fatalf("blocking=%#v, want kind %q", comparison.Blocking, test.kind)
			}
		})
	}
}

func TestCrossPlatformCoverageCompareAllowsRenameWhenOldPathIsAlias(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "string", false)),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommandWithAliases("dws find", []string{"search"}, testFlag("query", "string", false)),
	)

	comparison := Compare(current, base, "base")
	if !comparison.Compatible {
		t.Fatalf("rename with compatibility alias was blocked: %#v", comparison.Blocking)
	}
}

func TestCrossPlatformCoverageCompareBlocksRemovedAlias(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommandWithAliases("dws search", []string{"find"}),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommand("dws search"),
	)

	comparison := Compare(current, base, "base")
	if comparison.Compatible || !hasChangeKind(comparison.Blocking, "command_alias_removed") {
		t.Fatalf("removed alias was not blocked: %#v", comparison)
	}
}

func TestCrossPlatformCoverageCompareBlocksAliasRetargetedToIncompatibleCommand(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommandWithAliases("dws search", []string{"find"}, testFlag("query", "string", false)),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "string", false)),
		testCommand("dws find"),
	)

	comparison := Compare(current, base, "base")
	if comparison.Compatible || !hasFlagChange(comparison.Blocking, "flag_removed", "dws find", "query") {
		t.Fatalf("alias retarget was not checked against its old contract: %#v", comparison)
	}
}

func TestCrossPlatformCoverageCompareBlocksSnapshotRuleChanges(t *testing.T) {
	base := testSnapshot(testCommand("dws"))
	current := testSnapshot(testCommand("dws"))
	current.Rules.ExcludedFlags = append(current.Rules.ExcludedFlags, "legacy")

	comparison := Compare(current, base, "base")
	if comparison.Compatible || !hasChangeKind(comparison.Blocking, "snapshot_rules_changed") {
		t.Fatalf("snapshot rule change was not blocked: %#v", comparison)
	}
}

func TestCrossPlatformCoverageCompareBlocksCallableMetadataRegressions(t *testing.T) {
	baseFlag := testFlag("format", "string", false)
	baseFlag.Shorthand = "f"
	baseFlag.NoOpt = "json"
	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws export", baseFlag),
	)

	tests := []struct {
		name    string
		command Command
		kind    string
	}{
		{
			name: "command became non-runnable",
			command: func() Command {
				command := testCommand("dws export", baseFlag)
				command.Runnable = false
				return command
			}(),
			kind: "command_became_non_runnable",
		},
		{
			name: "command became hidden",
			command: func() Command {
				command := testCommand("dws export", baseFlag)
				command.Hidden = true
				return command
			}(),
			kind: "command_became_hidden",
		},
		{
			name: "flag shorthand removed",
			command: func() Command {
				flag := baseFlag
				flag.Shorthand = ""
				return testCommand("dws export", flag)
			}(),
			kind: "flag_shorthand_changed",
		},
		{
			name: "flag no-opt behavior removed",
			command: func() Command {
				flag := baseFlag
				flag.NoOpt = ""
				return testCommand("dws export", flag)
			}(),
			kind: "flag_no_opt_changed",
		},
		{
			name: "flag became hidden",
			command: func() Command {
				flag := baseFlag
				flag.Hidden = true
				return testCommand("dws export", flag)
			}(),
			kind: "flag_became_hidden",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := testSnapshot(testCommand("dws"), test.command)
			comparison := Compare(current, base, "base")
			if comparison.Compatible || !hasChangeKind(comparison.Blocking, test.kind) {
				t.Fatalf("metadata regression %q was not blocked: %#v", test.kind, comparison)
			}
		})
	}
}

func TestCrossPlatformCoverageCompareUsesEffectiveFlagsAtEveryCommandPath(t *testing.T) {
	base := testSnapshot(
		testCommand("dws", testFlag("profile", "string", false)),
		testCommandWithFlagScopes(
			"dws search",
			[]Flag{testFlag("query", "string", false)},
			[]Flag{testFlag("profile", "string", false)},
		),
	)

	t.Run("persistent flag scope narrowing is blocked", func(t *testing.T) {
		current := testSnapshot(
			testCommand("dws", testFlag("profile", "string", false)),
			testCommand("dws search", testFlag("query", "string", false)),
		)
		comparison := Compare(current, base, "base")
		if comparison.Compatible || !hasFlagChange(comparison.Blocking, "flag_removed", "dws search", "profile") {
			t.Fatalf("lost inherited flag was not blocked: %#v", comparison)
		}
	})

	t.Run("local flag moved to inherited remains compatible", func(t *testing.T) {
		current := testSnapshot(
			testCommand("dws",
				testFlag("profile", "string", false),
				testFlag("query", "string", false),
			),
			testCommandWithFlagScopes(
				"dws search",
				nil,
				[]Flag{
					testFlag("profile", "string", false),
					testFlag("query", "string", false),
				},
			),
		)
		comparison := Compare(current, base, "base")
		if !comparison.Compatible {
			t.Fatalf("flag moved to an inherited scope was blocked: %#v", comparison.Blocking)
		}
	})
}

func TestCrossPlatformCoverageCompareAllRequiresBothReferencesToPass(t *testing.T) {
	current := testSnapshot(testCommand("dws"), testCommand("dws status"))
	mergeBase := testSnapshot(testCommand("dws"))
	stable := testSnapshot(testCommand("dws"), testCommand("dws legacy"))

	report := CompareAll(current, map[string]Snapshot{
		"stable":     stable,
		"merge-base": mergeBase,
	})
	if report.Compatible {
		t.Fatal("aggregate report passed even though stable comparison removed a command")
	}
	if len(report.Comparisons) != 2 || report.Comparisons[0].Reference != "merge-base" || report.Comparisons[1].Reference != "stable" {
		t.Fatalf("comparisons are not deterministic: %#v", report.Comparisons)
	}
	if !report.Comparisons[0].Compatible || report.Comparisons[1].Compatible {
		t.Fatalf("unexpected per-reference results: %#v", report.Comparisons)
	}
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

func TestCrossPlatformCoverageCompareAcceptsReviewedFlagTypeChange(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "string", false)),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "int", false)),
	)

	// Without an entry the migration is still a blocking change.
	if comparison := Compare(current, base, "base"); !hasChangeKind(comparison.Blocking, "flag_type_changed") {
		t.Fatalf("an unreviewed type change should block: %#v", comparison.Blocking)
	}

	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws search", Flag: "query", From: "string", To: "int"})
	if comparison := Compare(current, base, "base"); !comparison.Compatible {
		t.Fatalf("a reviewed string->int migration should pass: %#v", comparison.Blocking)
	}
}

func TestCrossPlatformCoverageCompareReviewedFlagTypeChangeIsDirectionSensitive(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws search", Flag: "query", From: "string", To: "int"})

	// The reverse migration shares command and flag but not direction, so the
	// string->int entry must not admit it.
	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "int", false)),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "string", false)),
	)
	if comparison := Compare(current, base, "base"); comparison.Compatible {
		t.Fatal("int->string must not be admitted by a string->int entry")
	}
}

func TestCrossPlatformCoverageCompareReviewedFlagTypeChangeRejectsOtherCommandsAndFlags(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "string", false)),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", testFlag("query", "int", false)),
	)

	// Same flag and direction, different command.
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws status", Flag: "query", From: "string", To: "int"})
	if comparison := Compare(current, base, "base"); comparison.Compatible {
		t.Fatal("an entry for another command must not admit this migration")
	}

	// Same command and direction, different flag.
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws search", Flag: "elsewhere", From: "string", To: "int"})
	if comparison := Compare(current, base, "base"); comparison.Compatible {
		t.Fatal("an entry for another flag must not admit this migration")
	}
}

// A reviewed migration is only accepted when the rest of the flag's contract
// held still. Each case bundles one unrelated regression with the reviewed type
// change and requires the type failure to reappear, which also anchors
// flagContractOtherwiseChanged against the checks it mirrors: a condition that
// drifts out of sync fails here instead of silently widening the exemption.
func TestCrossPlatformCoverageCompareReviewedFlagTypeChangeRejectsBundledRegression(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws search", Flag: "query", From: "string", To: "int"})

	tests := []struct {
		name string
		base Flag
		next Flag
		kind string
	}{
		{
			name: "became required",
			base: Flag{Name: "query", Type: "string"},
			next: Flag{Name: "query", Type: "int", Required: true},
			kind: "flag_became_required",
		},
		{
			name: "lost shorthand",
			base: Flag{Name: "query", Type: "string", Shorthand: "q"},
			next: Flag{Name: "query", Type: "int"},
			kind: "flag_shorthand_changed",
		},
		{
			name: "changed no-opt",
			base: Flag{Name: "query", Type: "string", NoOpt: "all"},
			next: Flag{Name: "query", Type: "int", NoOpt: "1"},
			kind: "flag_no_opt_changed",
		},
		{
			name: "became hidden",
			base: Flag{Name: "query", Type: "string"},
			next: Flag{Name: "query", Type: "int", Hidden: true},
			kind: "flag_became_hidden",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := testSnapshot(testCommand("dws"), testCommand("dws search", test.base))
			current := testSnapshot(testCommand("dws"), testCommand("dws search", test.next))
			comparison := Compare(current, base, "base")
			if !hasChangeKind(comparison.Blocking, test.kind) {
				t.Fatalf("blocking=%#v, want kind %q", comparison.Blocking, test.kind)
			}
			if !hasChangeKind(comparison.Blocking, "flag_type_changed") {
				t.Fatalf("a bundled regression must re-report the type change: %#v", comparison.Blocking)
			}
		})
	}
}

// The exemption is resolved against the canonical Command.Path, never the
// alias-expanded accepted path. An aliased command enters compareEffectiveFlags
// once per accepted spelling, so keying on the accepted path would let every
// alias spelling re-report a reviewed migration.
func TestCrossPlatformCoverageCompareResolvesReviewedFlagTypeChangeByCanonicalPath(t *testing.T) {
	base := testSnapshot(
		testCommand("dws"),
		testCommandWithAliases("dws search", []string{"find"}, testFlag("query", "string", false)),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommandWithAliases("dws search", []string{"find"}, testFlag("query", "int", false)),
	)

	// The canonical path admits the migration for every accepted spelling.
	canonical := flagTypeChange{CommandPath: "dws search", Flag: "query", From: "string", To: "int"}
	registerReviewedFixture(t, canonical)
	if comparison := Compare(current, base, "base"); !comparison.Compatible {
		t.Fatalf("canonical path should admit the migration on every alias: %#v", comparison.Blocking)
	}
	delete(reviewedFlagTypeChanges, canonical)

	// An entry spelled with the alias path matches nothing, because the lookup
	// never sees the accepted spelling.
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws find", Flag: "query", From: "string", To: "int"})
	if comparison := Compare(current, base, "base"); comparison.Compatible {
		t.Fatal("an alias-spelled entry must not admit the migration")
	}
}

// A string -> int migration always changes the recorded default ("" to "0").
// Defaults are not part of what this gate enforces, and folding them into
// flagContractOtherwiseChanged would make every table entry dead on arrival.
func TestCrossPlatformCoverageCompareReviewedFlagTypeChangeToleratesDefaultDrift(t *testing.T) {
	registerReviewedFixture(t, flagTypeChange{CommandPath: "dws search", Flag: "query", From: "string", To: "int"})

	base := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", Flag{Name: "query", Type: "string", Default: ""}),
	)
	current := testSnapshot(
		testCommand("dws"),
		testCommand("dws search", Flag{Name: "query", Type: "int", Default: "0"}),
	)
	if comparison := Compare(current, base, "base"); !comparison.Compatible {
		t.Fatalf("default drift must not defeat a reviewed migration: %#v", comparison.Blocking)
	}
}

// reviewedEntryPattern matches one table entry as gofmt writes it on a single
// line. It is used to read the sibling copy of the table out of the policy
// helper, which cannot be imported: that directory is copied into a worktree at
// a historical revision and built there, so it must not depend on this package.
var reviewedEntryPattern = regexp.MustCompile(
	`\{CommandPath:\s*"([^"]*)",\s*Flag:\s*"([^"]*)",\s*From:\s*"([^"]*)",\s*To:\s*"([^"]*)"\}`)

const interfaceBaselineReviewedPath = "../../scripts/policy/interface-baseline/reviewed.go"

// The two copies of the table must stay identical, otherwise a migration passes
// one gate and is blocked by the other in the same CI job.
func TestCrossPlatformCoverageReviewedFlagTypeTableMatchesInterfaceBaseline(t *testing.T) {
	source, err := os.ReadFile(interfaceBaselineReviewedPath)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", interfaceBaselineReviewedPath, err)
	}

	sibling := make(map[flagTypeChange]struct{})
	for _, match := range reviewedEntryPattern.FindAllStringSubmatch(string(source), -1) {
		sibling[flagTypeChange{CommandPath: match[1], Flag: match[2], From: match[3], To: match[4]}] = struct{}{}
	}

	// Without this the whole guard degrades into comparing two empty sets the
	// moment the pattern stops matching, which is exactly how a duplicated
	// allowlist goes stale unnoticed.
	if len(sibling) == 0 {
		t.Fatalf("未能从 %s 提取到任何条目：正则或表的书写形态已漂移，这条守卫已失效", interfaceBaselineReviewedPath)
	}

	for change := range reviewedFlagTypeChanges {
		if _, ok := sibling[change]; !ok {
			t.Errorf("%+v 只存在于 interfacesnapshot，缺在 %s", change, interfaceBaselineReviewedPath)
		}
	}
	for change := range sibling {
		if _, ok := reviewedFlagTypeChanges[change]; !ok {
			t.Errorf("%+v 只存在于 %s，缺在 interfacesnapshot", change, interfaceBaselineReviewedPath)
		}
	}
}

func testSnapshot(commands ...Command) Snapshot {
	return Snapshot{
		SchemaVersion: SchemaVersion,
		Rules: Rules{
			ExcludedCommandSubtrees: append([]string(nil), excludedCommandSubtrees...),
			ExcludedFlags:           []string{"help"},
		},
		Commands: commands,
	}
}

func testCommand(path string, flags ...Flag) Command {
	return testCommandWithAliases(path, nil, flags...)
}

func testCommandWithAliases(path string, aliases []string, flags ...Flag) Command {
	return Command{
		Path:           path,
		Runnable:       true,
		Aliases:        aliases,
		LocalFlags:     flags,
		InheritedFlags: []Flag{},
	}
}

func testCommandWithFlagScopes(path string, local, inherited []Flag) Command {
	return Command{
		Path:           path,
		Aliases:        []string{},
		LocalFlags:     local,
		InheritedFlags: inherited,
	}
}

func testFlag(name, flagType string, required bool) Flag {
	return Flag{Name: name, Type: flagType, Required: required}
}

func hasChangeKind(changes []Change, kind string) bool {
	for _, change := range changes {
		if change.Kind == kind {
			return true
		}
	}
	return false
}

func hasFlagChange(changes []Change, kind, path, flag string) bool {
	for _, change := range changes {
		if change.Kind == kind && change.Path == path && change.Flag == flag {
			return true
		}
	}
	return false
}

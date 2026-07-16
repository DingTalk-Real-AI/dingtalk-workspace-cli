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

import "testing"

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

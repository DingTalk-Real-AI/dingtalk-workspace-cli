// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package interfacesnapshot

import (
	"reflect"
	"testing"
)

func recruitRetirementSnapshot() Snapshot {
	return testSnapshot(
		testCommand("dws"),
		testCommand("dws recruit"),
		testCommand("dws recruit job"),
		testCommand("dws recruit job list"),
		testCommand("dws recruit job get"),
		testCommand("dws recruit job create"),
	)
}

func TestCrossPlatformCoverageReviewedRecruitRetirement(t *testing.T) {
	baseline := recruitRetirementSnapshot()
	current := testSnapshot(testCommand("dws"))
	result := Compare(current, baseline, "stable")
	if !result.Compatible {
		t.Fatalf("PR #1485 retirement still blocks unrelated PRs: %+v", result.Blocking)
	}
	if !reflect.DeepEqual(result.Reviewed, []Change{{Kind: "product_retirement", Path: "dws recruit", After: "PR #1485"}}) {
		t.Fatalf("missing retirement audit receipt: %+v", result.Reviewed)
	}
	emptyFlags := FlagMigrationManifest{Version: FlagMigrationManifestVersion, Migrations: []FlagMigration{}}
	emptyCommands := CommandMigrationManifest{Version: CommandMigrationManifestVersion, Migrations: []CommandMigration{}}
	report, err := CompareAllWithInterfaceMigrations(current, map[string]Snapshot{"main": current, "stable": baseline}, emptyFlags, emptyFlags, emptyCommands, emptyCommands)
	if err != nil || !report.Compatible || len(report.Comparisons[0].Reviewed) != 0 || len(report.Comparisons[1].Reviewed) != 1 {
		t.Fatalf("dual-baseline migration entry: %+v, %v", report, err)
	}
}

func TestCrossPlatformCoverageReviewedRecruitRetirementBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot, *Snapshot)
	}{
		{"extra historical command", func(b, c *Snapshot) { b.Commands = append(b.Commands, testCommand("dws recruit candidate list")) }},
		{"missing historical command", func(b, c *Snapshot) { b.Commands = b.Commands[:5] }},
		{"historical alias", func(b, c *Snapshot) { b.Commands[1].Aliases = []string{"hiring"} }},
		{"historical hidden command", func(b, c *Snapshot) { b.Commands[1].Hidden = true }},
		{"historical non-runnable command", func(b, c *Snapshot) { b.Commands[1].Runnable = false }},
		{"recruit retained as alias", func(b, c *Snapshot) {
			c.Commands = append(c.Commands, testCommandWithAliases("dws hiring", []string{"recruit"}))
		}},
		{"other product removed", func(b, c *Snapshot) { b.Commands = append(b.Commands, testCommand("dws contact")) }},
		{"similar product removed", func(b, c *Snapshot) { b.Commands = append(b.Commands, testCommand("dws recruiter")) }},
		{"unrelated flag removed", func(b, c *Snapshot) {
			b.Commands = append(b.Commands, testCommand("dws contact", testFlag("user-id", "string", false)))
			c.Commands = append(c.Commands, testCommand("dws contact"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline, current := recruitRetirementSnapshot(), testSnapshot(testCommand("dws"))
			tt.mutate(&baseline, &current)
			if result := Compare(current, baseline, "stable"); result.Compatible {
				t.Fatalf("retirement hid unrelated or unapproved change: %+v", result)
			}
		})
	}
	// 逐个保留节点，证明不能用整体退役回执放行部分删除。
	for _, retained := range recruitRetirementSnapshot().Commands[1:] {
		t.Run("retained "+retained.Path, func(t *testing.T) {
			current := testSnapshot(testCommand("dws"), retained)
			if reviewedRecruitRetirement(current, recruitRetirementSnapshot()) {
				t.Fatal("partial retirement accepted")
			}
		})
	}
	for _, tt := range []struct {
		id, path string
		want     bool
	}{
		{"recruit.list_jobs", "recruit job list", true},
		{"recruit.get_job_detail", "recruit job get", true},
		{"recruit.create_job", "recruit job create", true},
		{"recruit.create_job", "recruit job get", false},
		{"recruit.other", "recruit job get", false},
	} {
		if got := IsReviewedRecruitTool(tt.id, tt.path); got != tt.want {
			t.Fatalf("%s / %s: %v", tt.id, tt.path, got)
		}
	}
}

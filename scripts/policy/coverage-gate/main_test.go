// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "coverage.out")
	body := "mode: atomic\nexample.com/project/internal/a.go:10.1,12.2 5 1\n"
	if err := os.WriteFile(profile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--overall-profile", profile,
		"--diff-profile", profile,
		"--base-ref", "base",
		"--module", "example.com/project",
		"--baseline-overall", "100",
		"--target", "80",
		"--enforce-overall-target=true",
	}
	loader := func(ref string) (map[string][]lineRange, error) {
		if ref != "base" {
			t.Fatalf("base ref=%q", ref)
		}
		return map[string][]lineRange{"internal/a.go": {{Start: 10, End: 12}}}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, loader, nil); code != 0 {
		t.Fatalf("run code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "overall coverage: 100.0%") || !strings.Contains(stdout.String(), "changed code coverage: 100.0%") {
		t.Fatalf("unexpected output %q", stdout.String())
	}

	stderr.Reset()
	if code := run(nil, &stdout, &stderr, loader, nil); code != 2 {
		t.Fatalf("missing arguments code=%d, want 2", code)
	}
	stderr.Reset()
	badLoader := func(string) (map[string][]lineRange, error) { return nil, errors.New("diff failed") }
	if code := run(args, &stdout, &stderr, badLoader, nil); code != 2 || !strings.Contains(stderr.String(), "diff failed") {
		t.Fatalf("loader failure code=%d stderr=%q", code, stderr.String())
	}
}

func TestCrossPlatformCoverageRunUnionsRepeatedOverallProfiles(t *testing.T) {
	dir := t.TempDir()
	uncovered := filepath.Join(dir, "uncovered.out")
	covered := filepath.Join(dir, "covered.out")
	if err := os.WriteFile(uncovered, []byte("mode: atomic\nexample.com/project/internal/a.go:10.1,12.2 5 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(covered, []byte("mode: atomic\nexample.com/project/internal/a.go:10.1,12.2 5 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--overall-profile", uncovered,
		"--overall-profile", covered,
		"--diff-profile", uncovered,
		"--diff-profile", covered,
		"--base-ref", "base",
		"--module", "example.com/project",
		"--baseline-overall", "100",
		"--target", "80",
	}
	loader := func(string) (map[string][]lineRange, error) {
		return map[string][]lineRange{"internal/a.go": {{Start: 10, End: 12}}}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, loader, nil); code != 0 {
		t.Fatalf("run code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "overall coverage: 100.0%") || !strings.Contains(stdout.String(), "changed code coverage: 100.0%") {
		t.Fatalf("repeated profiles were not unioned: %q", stdout.String())
	}
}

func TestRunChangedOnlyWithBuildableScope(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "coverage.out")
	body := "mode: atomic\nexample.com/project/internal/a.go:10.1,12.2 5 1\n"
	if err := os.WriteFile(profile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--changed-only",
		"--scope-buildable",
		"--diff-profile", profile,
		"--base-ref", "base",
		"--module", "example.com/project",
		"--target", "80",
	}
	loader := func(string) (map[string][]lineRange, error) {
		return map[string][]lineRange{
			"internal/a.go":         {{Start: 10, End: 12}},
			"internal/a_windows.go": {{Start: 1, End: 3}},
		}, nil
	}
	buildableLoader := func() (map[string]bool, error) {
		return map[string]bool{"internal/a.go": true}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, loader, buildableLoader); code != 0 {
		t.Fatalf("run code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "overall coverage") || !strings.Contains(stdout.String(), "changed code coverage: 100.0%") {
		t.Fatalf("unexpected changed-only output %q", stdout.String())
	}

	missingProfileScope := func() (map[string]bool, error) {
		return map[string]bool{"internal/a.go": true, "internal/a_windows.go": true}, nil
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr, loader, missingProfileScope); code != 1 || !strings.Contains(stderr.String(), "internal/a_windows.go") {
		t.Fatalf("missing native profile code=%d stderr=%q", code, stderr.String())
	}

	brokenScope := func() (map[string]bool, error) { return nil, errors.New("go list failed") }
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr, loader, brokenScope); code != 2 || !strings.Contains(stderr.String(), "go list failed") {
		t.Fatalf("scope failure code=%d stderr=%q", code, stderr.String())
	}
}

func TestReadProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte("mode: set\nexample.com/project/internal/a.go:1.1,2.2 3 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocks, err := readProfiles([]string{path}, "example.com/project")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].File != "internal/a.go" || blocks[0].Statements != 3 {
		t.Fatalf("blocks=%v", blocks)
	}
	if _, err := readProfiles([]string{filepath.Join(t.TempDir(), "missing")}, "example.com/project"); err == nil {
		t.Fatal("missing profile should fail")
	}
}

func TestParseChangedLinesRejectsInvalidHunk(t *testing.T) {
	_, err := parseChangedLines([]byte("+++ b/internal/a.go\n@@ invalid @@\n"))
	if err == nil {
		t.Fatal("invalid hunk should fail")
	}
}

func TestGitChangedLines(t *testing.T) {
	repository := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repository
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	runGit("init")
	runGit("config", "user.email", "coverage@example.com")
	runGit("config", "user.name", "Coverage Test")
	path := filepath.Join(repository, "a.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a.go")
	runGit("commit", "-m", "base")
	base := runGit("rev-parse", "HEAD")
	if err := os.WriteFile(path, []byte("package sample\n\nfunc added() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a.go")
	runGit("commit", "-m", "change")
	if err := os.WriteFile(path, []byte("package sample\n\nfunc added() int { return 1 }\nfunc workingTree() int { return 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	changed, err := gitChangedLines(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed["a.go"]) == 0 {
		t.Fatalf("changed lines=%v", changed)
	}
	last := changed["a.go"][len(changed["a.go"])-1]
	if last.End < 4 {
		t.Fatalf("working-tree line was not included: %v", changed["a.go"])
	}
}

func TestParseChangedLines(t *testing.T) {
	diff := []byte("diff --git a/internal/a.go b/internal/a.go\n+++ b/internal/a.go\n@@ -1,0 +2,3 @@\n+one\n+two\n+three\n" +
		"diff --git a/internal/a_test.go b/internal/a_test.go\n+++ b/internal/a_test.go\n@@ -0,0 +1,2 @@\n+test\n")
	got, err := parseChangedLines(diff)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]lineRange{"internal/a.go": {{Start: 2, End: 4}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseChangedLines() = %#v, want %#v", got, want)
	}
}

func TestFilterChangedFiles(t *testing.T) {
	changed := map[string][]lineRange{
		"internal/a.go":         {{Start: 1, End: 2}},
		"internal/a_windows.go": {{Start: 3, End: 4}},
	}
	got := filterChangedFiles(changed, map[string]bool{"internal/a_windows.go": true})
	want := map[string][]lineRange{"internal/a_windows.go": {{Start: 3, End: 4}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterChangedFiles() = %#v, want %#v", got, want)
	}
}

func TestEvaluateAllowsNonRegressionAndCoveredChanges(t *testing.T) {
	result := evaluate(gateInput{
		Overall: []coverageBlock{
			{File: "internal/a.go", Statements: 8, Count: 1},
			{File: "internal/a.go", Statements: 2, Count: 0},
		},
		Diff: []coverageBlock{
			{File: "internal/a.go", StartLine: 10, EndLine: 12, Statements: 4, Count: 1},
			{File: "internal/a.go", StartLine: 20, EndLine: 22, Statements: 1, Count: 0},
		},
		Changed:         map[string][]lineRange{"internal/a.go": {{Start: 10, End: 22}}},
		BaselineOverall: 80,
		Target:          80,
	})
	if len(result.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", result.Failures)
	}
	if result.ChangedCoverage != 80 {
		t.Fatalf("changed coverage = %.1f, want 80", result.ChangedCoverage)
	}
}

func TestCrossPlatformCoverageUnionsDuplicateCrossPackageCoverageBlocks(t *testing.T) {
	duplicate := coverageBlock{File: "internal/a.go", StartLine: 10, EndLine: 12, Statements: 4}
	covered := duplicate
	covered.Count = 3
	result := evaluate(gateInput{
		Overall: []coverageBlock{duplicate, covered},
		Diff:    []coverageBlock{duplicate, covered},
		Changed: map[string][]lineRange{"internal/a.go": {{Start: 10, End: 12}}},
		Target:  80,
	})
	if len(result.Failures) != 0 {
		t.Fatalf("duplicate blocks should be unioned: %v", result.Failures)
	}
	if result.Overall != 100 || result.ChangedCoverage != 100 || result.ChangedStatements != 4 {
		t.Fatalf("result = %#v, want one fully covered four-statement block", result)
	}
}

func TestEvaluateAllowsMeasurementTolerance(t *testing.T) {
	result := evaluate(gateInput{
		Overall:          []coverageBlock{{Statements: 403, Count: 1}, {Statements: 597, Count: 0}},
		Changed:          map[string][]lineRange{},
		BaselineOverall:  40.4,
		OverallTolerance: 0.1,
		Target:           80,
	})
	if len(result.Failures) != 0 {
		t.Fatalf("measurement tolerance should allow 40.3%% vs 40.4%%: %v", result.Failures)
	}
}

func TestEvaluateFailsClosed(t *testing.T) {
	result := evaluate(gateInput{
		Overall: []coverageBlock{
			{File: "internal/base.go", StartLine: 1, EndLine: 2, Statements: 10, Count: 1},
			{File: "internal/base.go", StartLine: 3, EndLine: 4, Statements: 10, Count: 0},
		},
		Diff:            []coverageBlock{{File: "internal/a.go", StartLine: 1, EndLine: 2, Statements: 2, Count: 0}},
		Changed:         map[string][]lineRange{"internal/a.go": {{Start: 1, End: 2}}, "internal/missing.go": {{Start: 1, End: 1}}},
		BaselineOverall: 54.2,
		Target:          80,
	})
	if len(result.Failures) != 3 {
		t.Fatalf("got failures %v, want overall, changed-code, and missing-profile failures", result.Failures)
	}
}

func TestEvaluateOverallTargetCanBeEnabled(t *testing.T) {
	result := evaluate(gateInput{
		Overall:         []coverageBlock{{Statements: 60, Count: 1}, {Statements: 40, Count: 0}},
		Diff:            []coverageBlock{},
		Changed:         map[string][]lineRange{},
		BaselineOverall: 54.2,
		Target:          80,
		EnforceOverall:  true,
	})
	if len(result.Failures) != 1 {
		t.Fatalf("failures = %v, want overall target failure", result.Failures)
	}
}

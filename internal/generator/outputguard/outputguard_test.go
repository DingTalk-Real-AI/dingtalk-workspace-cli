// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package outputguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageFilesystemSeamFailures(t *testing.T) {
	originalAbs, originalEval, originalRel := guardAbs, guardEvalSymlinks, guardRel
	originalStat, originalWalk := guardStat, guardWalk
	t.Cleanup(func() {
		guardAbs, guardEvalSymlinks, guardRel = originalAbs, originalEval, originalRel
		guardStat, guardWalk = originalStat, originalWalk
	})
	wantErr := errors.New("injected")
	guardAbs = func(string) (string, error) { return "", wantErr }
	if _, err := canonicalPath("x"); !errors.Is(err, wantErr) {
		t.Fatalf("canonicalPath(abs error) = %v", err)
	}
	guardAbs = originalAbs
	guardEvalSymlinks = func(string) (string, error) { return "", os.ErrNotExist }
	if got, err := canonicalPath(string(filepath.Separator)); err != nil || got == "" {
		t.Fatalf("canonicalPath(no existing ancestor) = %q, %v", got, err)
	}
	guardEvalSymlinks = originalEval
	guardRel = func(string, string) (string, error) { return "", wantErr }
	if pathContains("a", "b") {
		t.Fatal("pathContains(rel error) = true")
	}
	guardRel = originalRel

	root := t.TempDir()
	input := filepath.Join(root, "input")
	target := filepath.Join(root, "target")
	if err := os.WriteFile(input, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	statCalls := 0
	guardStat = func(path string) (os.FileInfo, error) {
		statCalls++
		if statCalls == 2 {
			return nil, wantErr
		}
		return originalStat(path)
	}
	if err := Validate(root, []Input{{Path: input}}, []Target{{Path: target}}); !errors.Is(err, wantErr) {
		t.Fatalf("Validate(target stat error) = %v", err)
	}
	guardStat = originalStat
	inputDir := filepath.Join(root, "dir")
	if err := os.Mkdir(inputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	guardWalk = func(root string, visit filepath.WalkFunc) error { return visit(root, nil, wantErr) }
	if err := Validate(root, []Input{{Path: inputDir}}, []Target{{Path: target}}); !errors.Is(err, wantErr) {
		t.Fatalf("Validate(walk error) = %v", err)
	}
}

func TestCrossPlatformCoverageValidateCoverageEdges(t *testing.T) {
	root := t.TempDir()
	inputFile := filepath.Join(root, "input.json")
	if err := os.WriteFile(inputFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []Input{{Path: ""}}, nil); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty input error = %v", err)
	}
	if err := Validate(root, []Input{{Path: "missing"}}, nil); err == nil || !strings.Contains(err.Error(), "stat") {
		t.Fatalf("missing input error = %v", err)
	}
	if err := Validate(root, []Input{{Path: "input.json"}}, []Target{{Path: ""}}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []Input{{Path: "input.json"}}, []Target{{Path: inputFile}}); err == nil || !strings.Contains(err.Error(), "resolves to") {
		t.Fatalf("same target error = %v", err)
	}
	hardLink := filepath.Join(root, "hard.json")
	if err := os.Link(inputFile, hardLink); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []Input{{Path: "input.json"}}, []Target{{Path: hardLink}}); err == nil || !strings.Contains(err.Error(), "resolves to") {
		t.Fatalf("hard-link target error = %v", err)
	}
	if err := Validate(root, []Input{{Path: "input.json"}}, []Target{{Path: root, Directory: true}}); err == nil || !strings.Contains(err.Error(), "contains") {
		t.Fatalf("directory target error = %v", err)
	}
	inputDir := filepath.Join(root, "inputs")
	if err := os.Mkdir(inputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(inputDir, "output.json")
	if err := Validate(root, []Input{{Path: "inputs"}}, []Target{{Path: inside}}); err == nil || !strings.Contains(err.Error(), "is inside") {
		t.Fatalf("inside target error = %v", err)
	}
	targetDir := filepath.Join(root, "target")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	containedInput := filepath.Join(targetDir, "reviewed.json")
	if err := os.WriteFile(containedInput, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []Input{{Path: containedInput}}, []Target{{Path: targetDir}}); err == nil || !strings.Contains(err.Error(), "contains") {
		t.Fatalf("existing directory contains error = %v", err)
	}

	loop := filepath.Join(root, "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, []Input{{Path: loop}}, nil); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("input symlink loop error = %v", err)
	}
	if err := Validate(root, []Input{{Path: inputFile}}, []Target{{Path: loop}}); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("target symlink loop error = %v", err)
	}

	if err := ValidateRepoTargetAllowlist(root, Target{}, "allowed"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRepoTargetAllowlist(loop, Target{Path: inputFile}, "allowed"); err == nil || !strings.Contains(err.Error(), "repository root") {
		t.Fatalf("allowlist root loop error = %v", err)
	}
	if err := ValidateRepoTargetAllowlist(root, Target{Path: loop}, "allowed"); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("allowlist target loop error = %v", err)
	}
	if err := ValidateRepoTargetAllowlist(root, Target{Path: inputFile}, "loop"); err == nil || !strings.Contains(err.Error(), "allowed repository output") {
		t.Fatalf("allowlist allowed loop error = %v", err)
	}
	if got := resolveRootPath(root, inputFile); got != inputFile {
		t.Fatalf("resolveRootPath(abs) = %q", got)
	}
	if !pathContains(root, root) || pathContains(root, filepath.Dir(root)) {
		t.Fatal("pathContains boundary mismatch")
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("firstNonEmpty() = %q", got)
	}
}

func TestCrossPlatformCoverageValidateRejectsHardLinkToProtectedDirectoryMember(t *testing.T) {
	root := t.TempDir()
	protectedDir := filepath.Join(root, "inputs")
	if err := os.MkdirAll(protectedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	member := filepath.Join(protectedDir, "reviewed.json")
	if err := os.WriteFile(member, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	hardLink := filepath.Join(root, "outside-link.json")
	if err := os.Link(member, hardLink); err != nil {
		t.Fatal(err)
	}
	err := Validate(root,
		[]Input{{Name: "reviewed directory", Path: "inputs"}},
		[]Target{{Name: "--output", Path: hardLink}},
	)
	if err == nil || !strings.Contains(err.Error(), "hard link to a member") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCrossPlatformCoverageValidateRepoTargetAllowlist(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "internal/cli/schema_catalog.json")
	if err := os.MkdirAll(filepath.Dir(allowed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRepoTargetAllowlist(root, Target{Name: "--output", Path: allowed}, "internal/cli/schema_catalog.json"); err != nil {
		t.Fatalf("canonical output rejected: %v", err)
	}
	err := ValidateRepoTargetAllowlist(root, Target{Name: "--output", Path: filepath.Join(root, "skills/mono/SKILL.md")}, "internal/cli/schema_catalog.json")
	if err == nil || !strings.Contains(err.Error(), "not a canonical generated delivery target") {
		t.Fatalf("repository source output error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "catalog.json")
	if err := ValidateRepoTargetAllowlist(root, Target{Name: "--output", Path: outside}, "internal/cli/schema_catalog.json"); err != nil {
		t.Fatalf("external temporary output rejected: %v", err)
	}
}

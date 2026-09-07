// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreCommitChecksOnlyStagedContent(t *testing.T) {
	repository := newPreCommitFixture(t)
	script := preCommitScriptPath(t)

	writePreCommitFixture(t, filepath.Join(repository, "legacy.go"), "package fixture\nfunc legacy( ){ }\n")
	writePreCommitFixture(t, filepath.Join(repository, "README.md"), "staged documentation change\n")
	runPreCommitGit(t, repository, "add", "README.md")

	output, err := runPreCommitCheck(repository, script)
	if err != nil {
		t.Fatalf("staged-only check rejected an unrelated unstaged Go file: %v\n%s", err, output)
	}
}

func TestPreCommitRejectsStagedUnformattedGoWithoutModifyingIt(t *testing.T) {
	repository := newPreCommitFixture(t)
	script := preCommitScriptPath(t)
	path := filepath.Join(repository, "feature.go")
	content := "package fixture\nfunc feature( ){ }\n"
	writePreCommitFixture(t, path, content)
	runPreCommitGit(t, repository, "add", "feature.go")

	output, err := runPreCommitCheck(repository, script)
	if err == nil {
		t.Fatalf("staged-only check accepted unformatted Go source:\n%s", output)
	}
	if !strings.Contains(output, "feature.go") {
		t.Fatalf("failure does not identify the staged file:\n%s", output)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read staged Go source: %v", readErr)
	}
	if string(got) != content {
		t.Fatalf("pre-commit check modified the working tree:\nwant: %q\n got: %q", content, got)
	}
}

func TestPreCommitUsesIndexForPartiallyStagedGoFile(t *testing.T) {
	repository := newPreCommitFixture(t)
	script := preCommitScriptPath(t)
	path := filepath.Join(repository, "feature.go")
	writePreCommitFixture(t, path, "package fixture\n\nfunc feature() {}\n")
	runPreCommitGit(t, repository, "add", "feature.go")
	writePreCommitFixture(t, path, "package fixture\nfunc feature( ){ }\n")

	output, err := runPreCommitCheck(repository, script)
	if err != nil {
		t.Fatalf("check inspected unstaged content instead of the index: %v\n%s", err, output)
	}
}

func TestPreCommitRejectsStagedWhitespaceErrors(t *testing.T) {
	repository := newPreCommitFixture(t)
	script := preCommitScriptPath(t)
	writePreCommitFixture(t, filepath.Join(repository, "notes.txt"), "trailing whitespace   \n")
	runPreCommitGit(t, repository, "add", "notes.txt")

	output, err := runPreCommitCheck(repository, script)
	if err == nil {
		t.Fatalf("staged-only check accepted a whitespace error:\n%s", output)
	}
	if !strings.Contains(output, "trailing whitespace") {
		t.Fatalf("failure does not explain the whitespace error:\n%s", output)
	}
}

func TestPreCommitHookIsOptInAndDoesNotRunMake(t *testing.T) {
	root := preCommitRepositoryRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	for _, line := range strings.Split(string(makefile), "\n") {
		if strings.HasPrefix(line, "all:") && strings.Contains(line, "setup-hooks") {
			t.Fatalf("default make target must not install repository hooks: %s", line)
		}
	}

	hook, err := os.ReadFile(filepath.Join(root, "scripts", "hooks", "pre-commit"))
	if err != nil {
		t.Fatalf("read pre-commit hook: %v", err)
	}
	if strings.Contains(string(hook), "\nmake") {
		t.Fatalf("pre-commit hook must not run the repository-wide make target:\n%s", hook)
	}
	if !strings.Contains(string(hook), "check-staged-commit.sh") {
		t.Fatalf("pre-commit hook does not delegate to the staged-only check:\n%s", hook)
	}
}

func newPreCommitFixture(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runPreCommitGit(t, repository, "init", "--quiet")
	runPreCommitGit(t, repository, "config", "user.name", "Pre-commit Test")
	runPreCommitGit(t, repository, "config", "user.email", "pre-commit@example.com")
	writePreCommitFixture(t, filepath.Join(repository, "legacy.go"), "package fixture\n\nfunc legacy() {}\n")
	writePreCommitFixture(t, filepath.Join(repository, "README.md"), "initial\n")
	runPreCommitGit(t, repository, "add", ".")
	runPreCommitGit(t, repository, "commit", "--quiet", "-m", "initial")
	return repository
}

func preCommitScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(preCommitRepositoryRoot(t), "scripts", "dev", "check-staged-commit.sh")
}

func preCommitRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func writePreCommitFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runPreCommitGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func runPreCommitCheck(repository, script string) (string, error) {
	command := exec.Command("sh", script)
	command.Dir = repository
	output, err := command.CombinedOutput()
	return string(output), err
}

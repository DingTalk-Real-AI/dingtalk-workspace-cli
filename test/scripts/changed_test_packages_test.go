// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestChangedTestPackagesPlansProductionReverseDependencies(t *testing.T) {
	repository := newChangedTestPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")

	writeChangedTestPackagesFixture(t, repository, "base/base.go", `package base

func Value() string {
	return "changed"
}
`)
	headRef := commitChangedTestPackagesFixture(t, repository, "change base")

	changed := runChangedTestPackages(t, repository, "changed", baseRef, headRef)
	if want := []string{"example.com/fixture/base"}; !slices.Equal(changed, want) {
		t.Fatalf("changed packages = %q, want %q", changed, want)
	}

	impacted := runChangedTestPackages(t, repository, "list", baseRef, headRef)
	want := []string{
		"example.com/fixture/base",
		"example.com/fixture/dependent",
		"example.com/fixture/testconsumer",
	}
	if !slices.Equal(impacted, want) {
		t.Fatalf("impacted packages = %q, want %q", impacted, want)
	}
	if slices.Contains(impacted, "example.com/fixture/unrelated") {
		t.Fatalf("impacted packages unexpectedly contain unrelated package: %q", impacted)
	}
}

func TestChangedTestPackagesMapsEmbeddedAssetsToTheirOwner(t *testing.T) {
	repository := newChangedTestPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")

	writeChangedTestPackagesFixture(
		t,
		repository,
		"assetowner/assets/message.json",
		"{\"message\":\"updated\"}\n",
	)
	headRef := commitChangedTestPackagesFixture(t, repository, "update embedded asset")

	want := []string{"example.com/fixture/assetowner"}
	if changed := runChangedTestPackages(t, repository, "changed", baseRef, headRef); !slices.Equal(changed, want) {
		t.Fatalf("changed packages = %q, want embedded owner %q", changed, want)
	}
	if impacted := runChangedTestPackages(t, repository, "list", baseRef, headRef); !slices.Equal(impacted, want) {
		t.Fatalf("impacted packages = %q, want embedded owner %q", impacted, want)
	}
}

func TestChangedTestPackagesIgnoresDocumentationOnlyDiff(t *testing.T) {
	repository := newChangedTestPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")

	writeChangedTestPackagesFixture(t, repository, "docs/guide.md", "# Updated guide\n")
	headRef := commitChangedTestPackagesFixture(t, repository, "update documentation")

	for _, mode := range []string{"changed", "list"} {
		if packages := runChangedTestPackages(t, repository, mode, baseRef, headRef); len(packages) != 0 {
			t.Errorf("%s packages = %q, want no packages", mode, packages)
		}
	}
}

func TestChangedTestPackagesRejectsMismatchedOrDirtyCheckout(t *testing.T) {
	t.Run("mismatched head", func(t *testing.T) {
		repository := newChangedTestPackagesRepository(t)
		baseRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")
		writeChangedTestPackagesFixture(t, repository, "base/base.go", "package base\n\nconst Value = \"changed\"\n")
		commitChangedTestPackagesFixture(t, repository, "change base")

		output, err := runChangedTestPackagesFailure(repository, "changed", baseRef, baseRef)
		if err == nil || !strings.Contains(output, "head does not match the checked-out revision") {
			t.Fatalf("mismatched HEAD failure = %v, output = %q", err, output)
		}
	})

	t.Run("dirty tracked file", func(t *testing.T) {
		repository := newChangedTestPackagesRepository(t)
		headRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")
		writeChangedTestPackagesFixture(t, repository, "base/base.go", "package base\n\nconst Value = \"dirty\"\n")

		output, err := runChangedTestPackagesFailure(repository, "changed", headRef, headRef)
		if err == nil || !strings.Contains(output, "requires a clean tracked worktree") {
			t.Fatalf("dirty worktree failure = %v, output = %q", err, output)
		}
	})
}

func TestChangedTestPackagesFailsClosedWhenPackageGraphIsInvalid(t *testing.T) {
	repository := newChangedTestPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")

	writeChangedTestPackagesFixture(t, repository, "dependent/dependent.go", `package dependent

import "example.com/fixture/missing"

func Value() string {
	return missing.Value()
}
`)
	headRef := commitChangedTestPackagesFixture(t, repository, "break package graph")

	script := filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh")
	command := exec.Command(script, "list", baseRef, headRef)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid package graph unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "failed to resolve the module package graph") {
		t.Fatalf("invalid package graph failure = %q, want fail-closed diagnostic", output)
	}
}

func newChangedTestPackagesRepository(t *testing.T) string {
	t.Helper()

	repository := t.TempDir()
	for path, contents := range map[string]string{
		"go.mod": `module example.com/fixture

go 1.22
`,
		"base/base.go": `package base

func Value() string {
	return "base"
}
`,
		"dependent/dependent.go": `package dependent

import "example.com/fixture/base"

func Value() string {
	return base.Value()
}
`,
		"testconsumer/consumer.go": `package testconsumer

func Value() string {
	return "consumer"
}
`,
		"testconsumer/consumer_test.go": `package testconsumer

import (
	"testing"

	"example.com/fixture/base"
)

func TestBaseValue(t *testing.T) {
	if base.Value() == "" {
		t.Fatal("base value is empty")
	}
}
`,
		"assetowner/asset.go": `package assetowner

import _ "embed"

//go:embed assets/message.json
var message []byte

func Message() []byte {
	return message
}
`,
		"assetowner/assets/message.json": "{\"message\":\"initial\"}\n",
		"unrelated/unrelated.go": `package unrelated

func Value() string {
	return "unrelated"
}
`,
		"docs/guide.md": "# Guide\n",
	} {
		writeChangedTestPackagesFixture(t, repository, path, contents)
	}

	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(projectRoot, "scripts", "ci", "changed-test-packages.sh"))
	if err != nil {
		t.Fatalf("read changed package planner: %v", err)
	}
	scriptPath := filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("create script directory: %v", err)
	}
	if err := os.WriteFile(scriptPath, script, 0o755); err != nil {
		t.Fatalf("copy changed package planner: %v", err)
	}

	runChangedTestPackagesCommand(t, repository, "git", "init", "-q", "-b", "main")
	runChangedTestPackagesCommand(t, repository, "git", "config", "user.name", "DWS CI")
	runChangedTestPackagesCommand(t, repository, "git", "config", "user.email", "dws-ci@example.invalid")
	return repository
}

func writeChangedTestPackagesFixture(t *testing.T, repository, path, contents string) {
	t.Helper()

	fullPath := filepath.Join(repository, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func commitChangedTestPackagesFixture(t *testing.T, repository, message string) string {
	t.Helper()

	runChangedTestPackagesCommand(t, repository, "git", "add", ".")
	runChangedTestPackagesCommand(t, repository, "git", "commit", "-q", "-m", message)
	return strings.TrimSpace(runChangedTestPackagesCommand(t, repository, "git", "rev-parse", "HEAD"))
}

func runChangedTestPackages(t *testing.T, repository, mode, baseRef, headRef string) []string {
	t.Helper()

	output := runChangedTestPackagesCommand(
		t,
		repository,
		filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh"),
		mode,
		baseRef,
		headRef,
	)
	return strings.Fields(output)
}

func runChangedTestPackagesFailure(repository, mode, baseRef, headRef string) (string, error) {
	script := filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh")
	command := exec.Command(script, mode, baseRef, headRef)
	command.Dir = repository
	output, err := command.CombinedOutput()
	return string(output), err
}

func runChangedTestPackagesCommand(t *testing.T, directory, name string, args ...string) string {
	t.Helper()

	command := exec.Command(name, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

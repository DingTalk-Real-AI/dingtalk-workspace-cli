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

func TestChangedTestPackagesRoutesSmokeOnlyChangeToSmokeShard(t *testing.T) {
	repository := newShardChangedPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial fixture")

	writeChangedTestPackagesFixture(t, repository, "test/smoke/smoke.go", `package smoke

func Value() string { return "changed smoke" }
`)
	headRef := commitChangedTestPackagesFixture(t, repository, "change smoke test")

	output := runChangedTestPackagesCommand(
		t,
		repository,
		filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh"),
		"list-shard",
		"smoke",
		baseRef,
		headRef,
	)
	if packages := strings.Fields(output); !slices.Equal(packages, []string{"example.com/shardfixture/test/smoke"}) {
		t.Fatalf("smoke shard packages = %q, want smoke package", packages)
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

// shardChangedPackagesShards mirrors the shard names scripts/ci/test-packages.sh
// asserts complete coverage for in its verify mode. Keeping the list here in
// full is deliberate: the union assertion below fails if a shard is added
// upstream without being selected, and an unknown-shard error fails if one is
// removed, so shard-plan drift cannot silently shrink focused-test coverage.
var shardChangedPackagesShards = []string{
	"app",
	"generators",
	"helpers",
	"cli",
	"smoke",
	"remaining",
	"release-scripts",
}

func TestChangedTestPackagesShardSelectionCoversEveryImpactedPackageExactlyOnce(t *testing.T) {
	repository := newShardChangedPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial shard fixture")

	writeChangedTestPackagesFixture(t, repository, "core/core.go", `package core

func Value() string {
	return "changed"
}
`)
	headRef := commitChangedTestPackagesFixture(t, repository, "change core")

	impacted := runChangedTestPackages(t, repository, "list", baseRef, headRef)
	if len(impacted) == 0 {
		t.Fatal("shard fixture produced no impacted packages")
	}

	owner := map[string]string{}
	union := make([]string, 0, len(impacted))
	for _, shard := range shardChangedPackagesShards {
		for _, pkg := range runChangedTestPackagesShard(t, repository, shard, baseRef, headRef) {
			if previous, ok := owner[pkg]; ok {
				t.Fatalf("package %q selected by both shard %q and %q", pkg, previous, shard)
			}
			owner[pkg] = shard
			union = append(union, pkg)
		}
	}

	slices.Sort(union)
	if !slices.Equal(union, impacted) {
		t.Fatalf("shard selection union = %q, want every impacted package %q", union, impacted)
	}
}

func TestChangedTestPackagesShardSelectionFailsClosedOnUnknownShard(t *testing.T) {
	repository := newShardChangedPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial shard fixture")

	writeChangedTestPackagesFixture(t, repository, "core/core.go", `package core

func Value() string {
	return "changed"
}
`)
	headRef := commitChangedTestPackagesFixture(t, repository, "change core")

	script := filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh")
	command := exec.Command(script, "list-shard", "nonexistent", baseRef, headRef)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("unknown shard unexpectedly succeeded:\n%s", output)
	}
	// An unknown shard must abort rather than report an empty selection: a
	// silently empty shard would let a mistyped CI shard name skip every test
	// while still reporting success.
	if !strings.Contains(string(output), "unknown test package shard") {
		t.Fatalf("unknown shard failure = %q, want fail-closed diagnostic", output)
	}
	if strings.TrimSpace(string(output)) == "" {
		t.Fatal("unknown shard produced empty output instead of a diagnostic")
	}
}

func TestChangedTestPackagesShardSelectionIsEmptyForUnaffectedShard(t *testing.T) {
	repository := newShardChangedPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial shard fixture")

	writeChangedTestPackagesFixture(t, repository, "internal/helpers/helpers.go", `package helpers

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " helpers changed"
}
`)
	headRef := commitChangedTestPackagesFixture(t, repository, "change helpers only")

	if selected := runChangedTestPackagesShard(t, repository, "helpers", baseRef, headRef); len(selected) == 0 {
		t.Fatal("helpers shard selected nothing for a helpers-only change")
	}
	// An unaffected shard is a normal outcome and must exit zero with no
	// output so the CI step can no-op instead of failing the job.
	if selected := runChangedTestPackagesShard(t, repository, "app", baseRef, headRef); len(selected) != 0 {
		t.Fatalf("app shard = %q, want empty for a helpers-only change", selected)
	}
}

func TestChangedTestPackagesShardSelectionRejectsMissingShardArgument(t *testing.T) {
	repository := newShardChangedPackagesRepository(t)
	baseRef := commitChangedTestPackagesFixture(t, repository, "initial shard fixture")

	script := filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh")
	command := exec.Command(script, "list-shard", baseRef, baseRef)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("list-shard without a shard argument unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "usage:") {
		t.Fatalf("missing shard argument failure = %q, want usage diagnostic", output)
	}
}

func runChangedTestPackagesShard(t *testing.T, repository, shard, baseRef, headRef string) []string {
	t.Helper()

	output := runChangedTestPackagesCommand(
		t,
		repository,
		filepath.Join(repository, "scripts", "ci", "changed-test-packages.sh"),
		"list-shard",
		shard,
		baseRef,
		headRef,
	)
	return strings.Fields(output)
}

// newShardChangedPackagesRepository builds a fixture whose directory layout
// matches every shard scripts/ci/test-packages.sh knows about, so shard
// selection can be exercised without depending on the real repository's working
// tree state. Every package imports core, letting a single core edit mark the
// whole module impacted.
func newShardChangedPackagesRepository(t *testing.T) string {
	t.Helper()

	repository := t.TempDir()
	for path, contents := range map[string]string{
		"go.mod": `module example.com/shardfixture

go 1.22
`,
		"core/core.go": `package core

func Value() string {
	return "core"
}
`,
		"internal/app/app.go": `package app

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " app"
}
`,
		"internal/generator/gen/gen.go": `package gen

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " gen"
}
`,
		"internal/helpers/helpers.go": `package helpers

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " helpers"
}
`,
		"internal/cli/cli.go": `package cli

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " cli"
}
`,
		"test/smoke/smoke.go": `package smoke

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " smoke"
}
`,
		"test/scripts/scripts.go": `package scripts

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " scripts"
}
`,
		"pkg/extra/extra.go": `package extra

import "example.com/shardfixture/core"

func Value() string {
	return core.Value() + " extra"
}
`,
	} {
		writeChangedTestPackagesFixture(t, repository, path, contents)
	}

	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	for _, name := range []string{"changed-test-packages.sh", "test-packages.sh"} {
		script, err := os.ReadFile(filepath.Join(projectRoot, "scripts", "ci", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scriptPath := filepath.Join(repository, "scripts", "ci", name)
		if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
			t.Fatalf("create script directory: %v", err)
		}
		if err := os.WriteFile(scriptPath, script, 0o755); err != nil {
			t.Fatalf("copy %s: %v", name, err)
		}
	}

	runChangedTestPackagesCommand(t, repository, "git", "init", "-q", "-b", "main")
	runChangedTestPackagesCommand(t, repository, "git", "config", "user.name", "DWS CI")
	runChangedTestPackagesCommand(t, repository, "git", "config", "user.email", "dws-ci@example.invalid")
	return repository
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

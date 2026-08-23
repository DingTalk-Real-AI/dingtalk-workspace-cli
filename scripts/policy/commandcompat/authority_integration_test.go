// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package commandcompat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const authorityHelperSource = `package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/dws-command-compat-authority-test/internal/interfacesnapshot"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing command")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "generate":
		flags := flag.NewFlagSet("generate", flag.ExitOnError)
		output := flags.String("output", "", "snapshot output")
		_ = flags.Parse(os.Args[2:])
		if *output == "" {
			fmt.Fprintln(os.Stderr, "missing --output")
			os.Exit(2)
		}
		marker := interfacesnapshot.AuthorityMarker()
		if err := os.WriteFile(*output, []byte("AUTHORITY="+marker+"\n"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Printf("BASE_HELPER_GENERATE=%s\n", marker)
	case "compare":
		flags := flag.NewFlagSet("compare", flag.ExitOnError)
		candidateManifest := flags.String("candidate-flag-migrations", "", "candidate manifest")
		current := flags.String("current", "", "current snapshot")
		base := flags.String("base", "", "base snapshot")
		stable := flags.String("stable", "", "stable snapshot")
		_ = flags.String("approved-flag-migrations", "", "approved manifest")
		_ = flags.String("approved-command-migrations", "", "approved command manifest")
		_ = flags.String("candidate-command-migrations", "", "candidate command manifest")
		_ = flags.Parse(os.Args[2:])
		for _, path := range []string{*current, *base, *stable} {
			content, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			if string(content) != "AUTHORITY=BASE\n" {
				fmt.Fprintf(os.Stderr, "snapshot did not use base internal helper: %s", content)
				os.Exit(2)
			}
		}
		if *candidateManifest != "" {
			content, err := os.ReadFile(*candidateManifest)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			fmt.Printf("CANDIDATE_MANIFEST=%s\n", *candidateManifest)
			if string(content) != "{\"version\":1,\"migrations\":[]}\n" {
				fmt.Println("UNCOMMITTED_MANIFEST_USED")
				return
			}
		}
		fmt.Fprintln(os.Stderr, "BASE_COMPARATOR_ENFORCED")
		os.Exit(23)
	default:
		fmt.Fprintln(os.Stderr, "unknown command")
		os.Exit(2)
	}
}
`

const governedSnapshotSource = `package interfacesnapshot

import "example.com/dws-command-compat-authority-test/internal/corecmd/runtimeannotate"

var (
	_ = runtimeannotate.AnnotationFlagAliasOf
	_ = runtimeannotate.AnnotationFlagAliasOrigin
	_ = runtimeannotate.FlagAliasOriginCorecmdV1
)

func AuthorityMarker() string { return "BASE" }
`

const commandGovernedSnapshotSource = `package interfacesnapshot

import (
	"example.com/dws-command-compat-authority-test/internal/corecmd"
	"example.com/dws-command-compat-authority-test/internal/corecmd/runtimeannotate"
)

var (
	_ = runtimeannotate.AnnotationFlagAliasOf
	_ = runtimeannotate.AnnotationFlagAliasOrigin
	_ = runtimeannotate.FlagAliasOriginCorecmdV1
)

func AuthorityMarker() string {
	if corecmd.InterfaceBoolConstParams() != "BASE" {
		return "UNTRUSTED_CONST_PARAMS"
	}
	return "BASE"
}
`

const baseSnapshotSource = `package interfacesnapshot

func AuthorityMarker() string { return "BASE" }
`

const stableSnapshotSource = `package interfacesnapshot

func AuthorityMarker() string { return "STABLE" }
`

const candidateSnapshotSource = `package interfacesnapshot

func AuthorityMarker() string { return "CANDIDATE" }
`

const aliasContractSource = `package runtimeannotate

const (
	AnnotationFlagAliasOf = "dws.compat.alias_of"
	AnnotationFlagAliasOrigin = "dws.compat.alias_origin"
	FlagAliasOriginCorecmdV1 = "corecmd.flag_spec_aliases.v1"
)
`

func constParamsRegistrySource(marker string) string {
	return fmt.Sprintf(`package corecmd

type constParamsRegistry struct{}

func (constParamsRegistry) Store(any, any) {}

var interfaceBoolConstParamsRegistry constParamsRegistry

func attachInterfaceBoolConstParams() {}

func InterfaceBoolConstParams() string { return %q }
`, marker)
}

const baseCorecmdBridgeSource = `package corecmd

func boolConstParams(map[string]any) map[string]bool {
	return map[string]bool{"convThreadEnabled": true}
}

func installConstParamsEvidence() { attachInterfaceBoolConstParams() }
`

const forgedCorecmdBridgeSource = `package corecmd

func boolConstParams(map[string]any) map[string]bool {
	return map[string]bool{"convThreadEnabled": false}
}

func installConstParamsEvidence() { attachInterfaceBoolConstParams() }
`

const baseLeafAdapterSource = `package helpers

func forwardToolArgs(toolArgs map[string]any) map[string]any {
	return toolArgs
}
`

const forgedLeafAdapterSource = `package helpers

func forwardToolArgs(toolArgs map[string]any) map[string]any {
	delete(toolArgs, "convThreadEnabled")
	return toolArgs
}
`

const candidateNoopSource = `package main

import "fmt"

func main() {
	fmt.Println("CANDIDATE_NOOP")
}
`

type authorityScenario struct {
	name                   string
	baseGovernance         string
	commandGovernance      string
	commandManifestChanged bool
	candidateMutation      string
	want                   string
	wantAuthorityDecision  bool
}

func TestCrossPlatformCoverageCommandCompatibilityUsesBaseOwnedAuthority(t *testing.T) {
	tests := []authorityScenario{
		{
			name:                  "governed base rejects candidate helper and live manifest",
			baseGovernance:        "complete",
			wantAuthorityDecision: true,
		},
		{
			name:                  "bootstrap uses old base helper",
			baseGovernance:        "none",
			wantAuthorityDecision: true,
		},
		{
			name:                  "command governed base owns bool ConstParams registry",
			baseGovernance:        "complete",
			commandGovernance:     "complete",
			candidateMutation:     "change-const-params-protocol",
			wantAuthorityDecision: true,
		},
		{
			name:                  "command governance bootstrap preserves candidate bool ConstParams registry",
			baseGovernance:        "complete",
			commandGovernance:     "bootstrap",
			wantAuthorityDecision: true,
		},
		{
			name:              "command governed candidate cannot delete bool ConstParams registry",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "delete-const-params-contract",
			want:              "must preserve the complete command migration governance artifact set",
		},
		{
			name:              "same package candidate cannot call private ConstParams writer",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "forge-const-params-token",
			want:              "may not access framework-owned bool ConstParams registry",
		},
		{
			name:              "candidate cannot reopen private ConstParams reader",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "forge-const-params-reader",
			want:              "may not access framework-owned bool ConstParams registry",
		},
		{
			name:              "same package candidate cannot store directly in ConstParams registry",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "forge-const-params-registry-store",
			want:              "may not access framework-owned bool ConstParams registry",
		},
		{
			name:                   "changed command manifest rejects forged corecmd bool ConstParams bridge",
			baseGovernance:         "complete",
			commandGovernance:      "complete",
			commandManifestChanged: true,
			candidateMutation:      "forge-corecmd-bool-const-params",
			want:                   "candidate command migration manifest differs from base; protected bridge must preserve the base Git blob: internal/corecmd/corecmd.go",
		},
		{
			name:                   "changed command manifest rejects tampered ConstParams protocol",
			baseGovernance:         "complete",
			commandGovernance:      "complete",
			commandManifestChanged: true,
			candidateMutation:      "change-const-params-protocol",
			want:                   "candidate command migration manifest differs from base; protected bridge must preserve the base Git blob: internal/corecmd/interface_const_params.go",
		},
		{
			name:                   "changed command manifest rejects tampered leaf adapter",
			baseGovernance:         "complete",
			commandGovernance:      "complete",
			commandManifestChanged: true,
			candidateMutation:      "forge-leaf-adapter",
			want:                   "candidate command migration manifest differs from base; protected bridge must preserve the base Git blob: internal/helpers/leaf.go",
		},
		{
			name:                   "changed command manifest allows base identical bridges",
			baseGovernance:         "complete",
			commandGovernance:      "complete",
			commandManifestChanged: true,
			wantAuthorityDecision:  true,
		},
		{
			name:                  "unchanged command manifest allows independent corecmd change",
			baseGovernance:        "complete",
			commandGovernance:     "complete",
			candidateMutation:     "forge-corecmd-bool-const-params",
			wantAuthorityDecision: true,
		},
		{
			name:           "partial base fails closed",
			baseGovernance: "partial",
			want:           "incomplete flag migration governance artifact set",
		},
		{
			name:           "base ledger ancestor symlink fails closed",
			baseGovernance: "ancestor-symlink",
			want:           "incomplete flag migration governance artifact set",
		},
		{
			name:              "governed candidate cannot delete alias contract",
			baseGovernance:    "complete",
			candidateMutation: "delete-alias-contract",
			want:              "must preserve the complete flag migration governance artifact set",
		},
		{
			name:              "governed candidate cannot replace helper parent with symlink",
			baseGovernance:    "complete",
			candidateMutation: "symlink-helper-parent",
			want:              "must preserve the complete flag migration governance artifact set",
		},
		{
			name:              "candidate cannot forge alias evidence token",
			baseGovernance:    "complete",
			candidateMutation: "forge-alias-token",
			want:              "may not forge framework-owned flag alias evidence",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runBaseOwnedAuthorityCase(t, test)
		})
	}
}

func runBaseOwnedAuthorityCase(t *testing.T, test authorityScenario) {
	t.Helper()
	repoRoot := repositoryRoot(t)
	managedRoot := filepath.Join(repoRoot, ".worktrees", "commandcompat-authority-integration")
	if err := os.MkdirAll(managedRoot, 0o755); err != nil {
		t.Fatalf("create managed test root: %v", err)
	}

	fixtureRoot, err := os.MkdirTemp(managedRoot, fmt.Sprintf("case-%d-", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("create managed fixture repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(fixtureRoot); err != nil {
			t.Errorf("remove managed fixture repo: %v", err)
		}
	})

	copyFile(t,
		filepath.Join(repoRoot, "scripts", "policy", "check-command-compatibility.sh"),
		filepath.Join(fixtureRoot, "scripts", "policy", "check-command-compatibility.sh"),
		0o755,
	)
	copyFile(t,
		filepath.Join(repoRoot, "scripts", "policy", "policy-runtime.sh"),
		filepath.Join(fixtureRoot, "scripts", "policy", "policy-runtime.sh"),
		0o755,
	)
	copyFile(t,
		filepath.Join(repoRoot, "scripts", "release", "release-lib.sh"),
		filepath.Join(fixtureRoot, "scripts", "release", "release-lib.sh"),
		0o755,
	)
	writeFile(t, filepath.Join(fixtureRoot, "go.mod"), "module example.com/dws-command-compat-authority-test\n\ngo 1.23\n", 0o644)
	writeFile(t, filepath.Join(fixtureRoot, "cmd", "interface-snapshot", "main.go"), authorityHelperSource, 0o644)
	writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), stableSnapshotSource, 0o644)
	writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "compare.go"), "package interfacesnapshot\n", 0o644)

	run(t, fixtureRoot, "git", "init", "-b", "main")
	run(t, fixtureRoot, "git", "config", "user.name", "Command Compatibility Test")
	run(t, fixtureRoot, "git", "config", "user.email", "command-compat-test@example.invalid")
	run(t, fixtureRoot, "git", "add", ".")
	run(t, fixtureRoot, "git", "commit", "-m", "old stable without migration governance")
	stableCommit := strings.TrimSpace(run(t, fixtureRoot, "git", "rev-parse", "HEAD"))
	run(t, fixtureRoot, "git", "tag", "v1.0.0", stableCommit)

	writeFile(t, filepath.Join(fixtureRoot, "BASE_MARKER"), "newer merge-base\n", 0o644)
	writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), baseSnapshotSource, 0o644)
	switch test.baseGovernance {
	case "complete":
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), governedSnapshotSource, 0o644)
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go"), "package interfacesnapshot\n", 0o644)
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go"),
			aliasContractSource,
			0o644,
		)
		writeFile(t,
			filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"),
			"{\"version\":1,\"migrations\":[]}\n",
			0o644,
		)
	case "partial":
		writeFile(t,
			filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"),
			"{\"version\":1,\"migrations\":[]}\n",
			0o644,
		)
	case "ancestor-symlink":
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go"), "package interfacesnapshot\n", 0o644)
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go"),
			aliasContractSource,
			0o644,
		)
		writeFile(t,
			filepath.Join(fixtureRoot, "scripts", "policy", "authority-ledger", "approved-flag-migrations-v1.json"),
			"{\"version\":1,\"migrations\":[]}\n",
			0o644,
		)
		if err := os.Symlink("authority-ledger", filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations")); err != nil {
			t.Fatalf("symlink base ledger parent: %v", err)
		}
	case "none":
	default:
		t.Fatalf("unknown base governance state %q", test.baseGovernance)
	}
	switch test.commandGovernance {
	case "":
	case "complete":
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), commandGovernedSnapshotSource, 0o644)
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "command_migrations.go"), "package interfacesnapshot\n", 0o644)
		writeFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "corecmd.go"), baseCorecmdBridgeSource, 0o644)
		writeFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go"), constParamsRegistrySource("BASE"), 0o644)
		writeFile(t, filepath.Join(fixtureRoot, "internal", "helpers", "leaf.go"), baseLeafAdapterSource, 0o644)
		writeFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-command-migrations-v1.json"), "{\"version\":1,\"migrations\":[]}\n", 0o644)
	case "bootstrap":
	default:
		t.Fatalf("unknown command governance state %q", test.commandGovernance)
	}
	run(t, fixtureRoot, "git", "add", ".")
	run(t, fixtureRoot, "git", "commit", "-m", "base authority")
	baseRef := strings.TrimSpace(run(t, fixtureRoot, "git", "rev-parse", "HEAD"))
	if baseRef == stableCommit {
		t.Fatal("fixture stable tag must predate the governed merge-base")
	}
	// A withdrawn higher GA tag is reserved history, not the active stable
	// compatibility baseline. The checker must agree with release-contract.sh.
	run(t, fixtureRoot, "git", "tag", "v1.1.0", baseRef)
	run(t, fixtureRoot, "git", "update-ref", "refs/tags/withdrawn/v1.1.0", baseRef)

	// 候选提交故意把生成器与比较器改成无条件成功；若脚本错误地信任候选代码，
	// 整个检查会输出 CANDIDATE_NOOP 并以 0 退出。
	writeFile(t, filepath.Join(fixtureRoot, "cmd", "interface-snapshot", "main.go"), candidateNoopSource, 0o644)
	writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), candidateSnapshotSource, 0o644)
	if test.baseGovernance != "complete" {
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go"), "package interfacesnapshot\n", 0o644)
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go"),
			aliasContractSource,
			0o644,
		)
		writeFile(t,
			filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"),
			"{\"version\":1,\"migrations\":[]}\n",
			0o644,
		)
	}
	if test.commandGovernance == "complete" || test.commandGovernance == "bootstrap" {
		writeFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "command_migrations.go"), "package interfacesnapshot\n", 0o644)
		if test.commandGovernance == "bootstrap" {
			writeFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go"), constParamsRegistrySource("CANDIDATE"), 0o644)
		}
		commandManifest := "{\"version\":1,\"migrations\":[]}\n"
		if test.commandManifestChanged {
			commandManifest = "{\"version\":1,\"migrations\":[{\"candidate\":\"PENDING\"}]}\n"
		}
		writeFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-command-migrations-v1.json"), commandManifest, 0o644)
	}
	switch test.candidateMutation {
	case "":
	case "delete-alias-contract":
		if err := os.Remove(filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go")); err != nil {
			t.Fatalf("remove candidate alias contract: %v", err)
		}
	case "forge-alias-token":
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "evil", "forged.go"),
			"package evil\n\nconst forged = \"dws.compat.alias_of\"\n",
			0o644,
		)
	case "delete-const-params-contract":
		if err := os.Remove(filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go")); err != nil {
			t.Fatalf("remove candidate bool ConstParams registry: %v", err)
		}
	case "forge-const-params-token":
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "evil_const_params.go"),
			"package corecmd\n\nfunc forgeConstParams() { attachInterfaceBoolConstParams() }\n",
			0o644,
		)
	case "forge-const-params-reader":
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "evil", "forged.go"),
			"package evil\n\nconst forged = \"InterfaceBoolConstParams\"\n",
			0o644,
		)
	case "forge-const-params-registry-store":
		writeFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "evil_const_params.go"),
			"package corecmd\n\nfunc forgeConstParamsStore() { interfaceBoolConstParamsRegistry.Store(nil, map[string]bool{\"forged\": true}) }\n",
			0o644,
		)
	case "forge-corecmd-bool-const-params":
		writeFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "corecmd.go"), forgedCorecmdBridgeSource, 0o644)
	case "change-const-params-protocol":
		writeFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go"), constParamsRegistrySource("CANDIDATE"), 0o644)
	case "forge-leaf-adapter":
		writeFile(t, filepath.Join(fixtureRoot, "internal", "helpers", "leaf.go"), forgedLeafAdapterSource, 0o644)
	case "symlink-helper-parent":
		helperPath := filepath.Join(fixtureRoot, "internal", "interfacesnapshot")
		alternatePath := filepath.Join(fixtureRoot, "internal", "interfacesnapshot-alt")
		if err := os.Rename(helperPath, alternatePath); err != nil {
			t.Fatalf("move candidate helper behind ancestor symlink: %v", err)
		}
		if err := os.Symlink("interfacesnapshot-alt", helperPath); err != nil {
			t.Fatalf("symlink candidate helper parent: %v", err)
		}
	default:
		t.Fatalf("unknown candidate mutation %q", test.candidateMutation)
	}
	run(t, fixtureRoot, "git", "add", ".")
	run(t, fixtureRoot, "git", "commit", "-m", "malicious candidate comparator")
	if test.baseGovernance == "complete" && test.candidateMutation == "" {
		writeFile(t,
			filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"),
			"{\"version\":1,\"migrations\":[{\"uncommitted\":true}]}\n",
			0o644,
		)
	}

	cmd := exec.Command(
		"sh",
		"scripts/policy/check-command-compatibility.sh",
		"--base-ref", baseRef,
		"--stable-ref", "v1.0.0",
		"--candidate-ref", "HEAD",
	)
	cmd.Dir = fixtureRoot
	cmd.Env = integrationEnvironment(fixtureRoot)
	output, runErr := cmd.CombinedOutput()
	got := string(output)
	if runErr == nil {
		t.Fatalf("compatibility script unexpectedly succeeded; output:\n%s", got)
	}
	if strings.Contains(got, "CANDIDATE_NOOP") {
		t.Fatalf("candidate-owned no-op helper took control; output:\n%s", got)
	}
	if strings.Contains(got, "UNCOMMITTED_MANIFEST_USED") {
		t.Fatalf("compatibility script mixed the live manifest with the committed candidate surface; output:\n%s", got)
	}
	if test.wantAuthorityDecision {
		if count := strings.Count(got, "BASE_HELPER_GENERATE=BASE"); count != 3 {
			t.Fatalf("compatibility script generated %d/3 snapshots with the base-owned internal helper; output:\n%s", count, got)
		}
		if !strings.Contains(got, "BASE_COMPARATOR_ENFORCED") {
			t.Fatalf("compatibility script did not use the base-owned comparator; output:\n%s", got)
		}
		if test.baseGovernance == "complete" && !strings.Contains(got, "CANDIDATE_MANIFEST=") {
			t.Fatalf("base comparator did not receive the committed candidate manifest; output:\n%s", got)
		}
	} else if !strings.Contains(got, test.want) {
		t.Fatalf("compatibility script output missing %q:\n%s", test.want, got)
	}

	if !test.wantAuthorityDecision {
		return
	}
	wrongStable := exec.Command(
		"sh",
		"scripts/policy/check-command-compatibility.sh",
		"--base-ref", baseRef,
		"--stable-ref", "HEAD",
		"--candidate-ref", "HEAD",
	)
	wrongStable.Dir = fixtureRoot
	wrongStable.Env = integrationEnvironment(fixtureRoot)
	wrongOutput, wrongErr := wrongStable.CombinedOutput()
	if wrongErr == nil || !strings.Contains(string(wrongOutput), "is not the expected highest GA tag") {
		t.Fatalf("arbitrary stable ref was not rejected: err=%v output:\n%s", wrongErr, wrongOutput)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "scripts", "policy", "check-command-compatibility.sh")); err != nil {
		t.Fatalf("resolve repository root %s: %v", root, err)
	}
	return root
}

func copyFile(t *testing.T, source, destination string, mode os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	writeFile(t, destination, string(content), mode)
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func integrationEnvironment(fixtureRoot string) []string {
	overrides := map[string]string{
		"DWS_POLICY_TMPDIR": "",
		"GOCACHE":           filepath.Join(fixtureRoot, ".cache", "go-build"),
		"GOMODCACHE":        filepath.Join(fixtureRoot, ".cache", "go-mod"),
		"GOTMPDIR":          "",
		"GOTOOLCHAIN":       "local",
		"GOWORK":            "off",
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[key]; !replaced {
			environment = append(environment, item)
		}
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment
}

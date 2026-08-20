// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package schemacompat

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

const governedSchemaCheckerSource = `package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	normalize := flag.String("normalize", "", "raw schema")
	check := flag.String("check", "", "base schema")
	current := flag.String("current", "", "candidate schema")
	approved := flag.String("approved-flag-migrations", "", "base ledger")
	candidate := flag.String("candidate-flag-migrations", "", "candidate ledger")
	currentSnapshot := flag.String("migration-current-snapshot", "", "candidate interface snapshot")
	baseSnapshot := flag.String("migration-base-snapshot", "", "base interface snapshot")
	stableSnapshot := flag.String("migration-stable-snapshot", "", "stable interface snapshot")
	_ = flag.String("approved-command-migrations", "", "base command ledger")
	_ = flag.String("candidate-command-migrations", "", "candidate command ledger")
	flag.Parse()

	if *normalize != "" {
		data, err := os.ReadFile(*normalize)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(string(data))
		return
	}
	if *check == "" || *current == "" {
		fmt.Fprintln(os.Stderr, "missing check inputs")
		os.Exit(2)
	}
	currentData, err := os.ReadFile(*current)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if !strings.Contains(string(currentData), "COMMITTED_CANDIDATE") {
		fmt.Fprintf(os.Stderr, "candidate schema did not come from detached commit: %s\n", currentData)
		os.Exit(2)
	}
	paths := []string{*approved, *candidate, *currentSnapshot, *baseSnapshot, *stableSnapshot}
	for _, path := range paths {
		if path == "" {
			fmt.Fprintln(os.Stderr, "governed checker did not receive all migration inputs")
			os.Exit(2)
		}
	}
	for _, fixture := range []struct {
		path string
		want string
	}{
		{*approved, "{\"version\":1,\"migrations\":[{\"authority\":\"BASE\"}]}\n"},
		{*candidate, "{\"version\":1,\"migrations\":[{\"candidate\":\"COMMITTED\"}]}\n"},
		{*currentSnapshot, "COMMITTED_CANDIDATE"},
		{*baseSnapshot, "BASE_AUTHORITY"},
		{*stableSnapshot, "STABLE_RELEASE"},
	} {
		data, err := os.ReadFile(fixture.path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if !strings.Contains(string(data), fixture.want) {
			fmt.Fprintf(os.Stderr, "migration input %s does not contain %q: %s\n", fixture.path, fixture.want, data)
			os.Exit(2)
		}
	}
	for _, path := range []string{*currentSnapshot, *baseSnapshot, *stableSnapshot} {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if !strings.Contains(string(data), "INTERFACE_HELPER=BASE") {
			fmt.Fprintf(os.Stderr, "interface snapshot did not use base internal helper: %s\n", data)
			os.Exit(2)
		}
	}
	fmt.Fprintln(os.Stderr, "BASE_SCHEMA_CHECKER_ENFORCED")
	os.Exit(23)
}
`

const bootstrapSchemaCheckerSource = `package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	normalize := flag.String("normalize", "", "raw schema")
	check := flag.String("check", "", "base schema")
	current := flag.String("current", "", "candidate schema")
	flag.Parse()
	if *normalize != "" {
		data, err := os.ReadFile(*normalize)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(string(data))
		return
	}
	if *check == "" || *current == "" {
		fmt.Fprintln(os.Stderr, "missing check inputs")
		os.Exit(2)
	}
	data, err := os.ReadFile(*current)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if !strings.Contains(string(data), "COMMITTED_CANDIDATE") {
		fmt.Fprintf(os.Stderr, "candidate schema did not come from detached commit: %s\n", data)
		os.Exit(2)
	}
	baselineData, err := os.ReadFile(*check)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	switch {
	case strings.Contains(string(baselineData), "BASE_AUTHORITY"):
		fmt.Fprintln(os.Stdout, "OLD_BASE_SCHEMA_CONTRACT_CHECKED")
	case strings.Contains(string(baselineData), "STABLE_RELEASE"):
		fmt.Fprintln(os.Stderr, "OLD_BASE_STABLE_SCHEMA_CHECKER_ENFORCED")
		os.Exit(23)
	default:
		fmt.Fprintf(os.Stderr, "unexpected historical Schema contract: %s\n", baselineData)
		os.Exit(2)
	}
}
`

const stableSchemaGuardCheckerSource = `package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	normalize := flag.String("normalize", "", "raw schema")
	check := flag.String("check", "", "historical schema")
	current := flag.String("current", "", "candidate schema")
	approved := flag.String("approved-flag-migrations", "", "base ledger")
	candidate := flag.String("candidate-flag-migrations", "", "candidate ledger")
	currentSnapshot := flag.String("migration-current-snapshot", "", "candidate interface snapshot")
	baseSnapshot := flag.String("migration-base-snapshot", "", "base interface snapshot")
	stableSnapshot := flag.String("migration-stable-snapshot", "", "stable interface snapshot")
	_ = flag.String("approved-command-migrations", "", "base command ledger")
	_ = flag.String("candidate-command-migrations", "", "candidate command ledger")
	flag.Parse()

	if *normalize != "" {
		data, err := os.ReadFile(*normalize)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Print(string(data))
		return
	}
	if *check == "" || *current == "" {
		fmt.Fprintln(os.Stderr, "missing check inputs")
		os.Exit(2)
	}
	for _, fixture := range []struct {
		path string
		want string
	}{
		{*approved, "\"authority\":\"BASE\""},
		{*candidate, "\"candidate\":\"COMMITTED\""},
		{*currentSnapshot, "COMMITTED_CANDIDATE"},
		{*baseSnapshot, "BASE_AUTHORITY"},
		{*stableSnapshot, "STABLE_RELEASE"},
	} {
		if fixture.path == "" {
			fmt.Fprintln(os.Stderr, "stable Schema guard did not receive all migration inputs")
			os.Exit(2)
		}
		data, err := os.ReadFile(fixture.path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if !strings.Contains(string(data), fixture.want) {
			fmt.Fprintf(os.Stderr, "stable Schema guard input %s does not contain %q: %s\n", fixture.path, fixture.want, data)
			os.Exit(2)
		}
	}
	currentData, err := os.ReadFile(*current)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if !strings.Contains(string(currentData), "COMMITTED_CANDIDATE") {
		fmt.Fprintf(os.Stderr, "candidate schema did not come from detached commit: %s\n", currentData)
		os.Exit(2)
	}
	baselineData, err := os.ReadFile(*check)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	switch {
	case strings.Contains(string(baselineData), "BASE_AUTHORITY"):
		fmt.Fprintln(os.Stdout, "BASE_SCHEMA_CONTRACT_CHECKED")
	case strings.Contains(string(baselineData), "STABLE_RELEASE"):
		fmt.Fprintln(os.Stderr, "STABLE_SCHEMA_CONTRACT_ENFORCED")
		os.Exit(23)
	default:
		fmt.Fprintf(os.Stderr, "unexpected historical Schema contract: %s\n", baselineData)
		os.Exit(2)
	}
}
`

const authorityInterfaceHelperSource = `package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/dws-schema-compat-authority-test/internal/interfacesnapshot"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "generate" {
		fmt.Fprintln(os.Stderr, "expected generate")
		os.Exit(2)
	}
	flags := flag.NewFlagSet("generate", flag.ExitOnError)
	output := flags.String("output", "", "snapshot output")
	_ = flags.Parse(os.Args[2:])
	marker, err := os.ReadFile("SNAPSHOT_MARKER")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	marker = append(marker, []byte("INTERFACE_HELPER="+interfacesnapshot.AuthorityMarker()+"\n")...)
	if err := os.WriteFile(*output, marker, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("BASE_INTERFACE_HELPER_GENERATE=%s\n", interfacesnapshot.AuthorityMarker())
}
`

const schemaGovernedSnapshotSource = `package interfacesnapshot

import "example.com/dws-schema-compat-authority-test/internal/corecmd/runtimeannotate"

var (
	_ = runtimeannotate.AnnotationFlagAliasOf
	_ = runtimeannotate.AnnotationFlagAliasOrigin
	_ = runtimeannotate.FlagAliasOriginCorecmdV1
)

func AuthorityMarker() string { return "BASE" }
`

const schemaCommandGovernedSnapshotSource = `package interfacesnapshot

import (
	"example.com/dws-schema-compat-authority-test/internal/corecmd"
	"example.com/dws-schema-compat-authority-test/internal/corecmd/runtimeannotate"
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

const schemaBaseSnapshotSource = `package interfacesnapshot

func AuthorityMarker() string { return "BASE" }
`

const schemaStableSnapshotSource = `package interfacesnapshot

func AuthorityMarker() string { return "STABLE" }
`

const schemaCandidateSnapshotSource = `package interfacesnapshot

func AuthorityMarker() string { return "CANDIDATE" }
`

const schemaAliasContractSource = `package runtimeannotate

const (
	AnnotationFlagAliasOf = "dws.compat.alias_of"
	AnnotationFlagAliasOrigin = "dws.compat.alias_origin"
	FlagAliasOriginCorecmdV1 = "corecmd.flag_spec_aliases.v1"
)
`

func schemaConstParamsRegistrySource(marker string) string {
	return fmt.Sprintf(`package corecmd

type constParamsRegistry struct{}

func (constParamsRegistry) Store(any, any) {}

var interfaceBoolConstParamsRegistry constParamsRegistry

func attachInterfaceBoolConstParams() {}

func InterfaceBoolConstParams() string { return %q }
`, marker)
}

const schemaBaseCorecmdBridgeSource = `package corecmd

func boolConstParams(map[string]any) map[string]bool {
	return map[string]bool{"convThreadEnabled": true}
}

func installConstParamsEvidence() { attachInterfaceBoolConstParams() }
`

const schemaForgedCorecmdBridgeSource = `package corecmd

func boolConstParams(map[string]any) map[string]bool {
	return map[string]bool{"convThreadEnabled": false}
}

func installConstParamsEvidence() { attachInterfaceBoolConstParams() }
`

const schemaBaseLeafAdapterSource = `package helpers

func forwardToolArgs(toolArgs map[string]any) map[string]any {
	return toolArgs
}
`

const schemaForgedLeafAdapterSource = `package helpers

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

func schemaBinarySource(marker string) string {
	return fmt.Sprintf(`package main

import "fmt"

func main() {
	fmt.Println(%q)
}
`, `{"kind":"schema","marker":"`+marker+`"}`)
}

type authorityScenario struct {
	name                   string
	baseGovernance         string
	commandGovernance      string
	commandManifestChanged bool
	candidateMutation      string
	checkerMode            string
	want                   string
	authorityMarker        string
}

func TestCrossPlatformCoverageSchemaCompatibilityUsesBaseOwnedAuthority(t *testing.T) {
	tests := []authorityScenario{
		{
			name:            "governed base rejects candidate checker and live workspace",
			baseGovernance:  "complete",
			authorityMarker: "BASE_SCHEMA_CHECKER_ENFORCED",
		},
		{
			name:            "governed checker protects stable Schema contract",
			baseGovernance:  "complete",
			checkerMode:     "stable-schema-guard",
			authorityMarker: "STABLE_SCHEMA_CONTRACT_ENFORCED",
		},
		{
			name:            "bootstrap uses old base checker and committed canonical empty ledger",
			baseGovernance:  "none",
			authorityMarker: "OLD_BASE_STABLE_SCHEMA_CHECKER_ENFORCED",
		},
		{
			name:              "command governed base owns bool ConstParams contract",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "change-const-params-protocol",
			authorityMarker:   "BASE_SCHEMA_CHECKER_ENFORCED",
		},
		{
			name:              "command governance bootstrap preserves candidate bool ConstParams contract",
			baseGovernance:    "complete",
			commandGovernance: "bootstrap",
			authorityMarker:   "BASE_SCHEMA_CHECKER_ENFORCED",
		},
		{
			name:              "command governed candidate cannot delete bool ConstParams contract",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "delete-const-params-contract",
			want:              "must preserve the complete Schema command migration governance artifact set",
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
			authorityMarker:        "BASE_SCHEMA_CHECKER_ENFORCED",
		},
		{
			name:              "unchanged command manifest allows independent corecmd change",
			baseGovernance:    "complete",
			commandGovernance: "complete",
			candidateMutation: "forge-corecmd-bool-const-params",
			authorityMarker:   "BASE_SCHEMA_CHECKER_ENFORCED",
		},
		{
			name:           "partial base fails closed",
			baseGovernance: "partial",
			want:           "incomplete Schema flag migration governance artifact set",
		},
		{
			name:           "base ledger ancestor symlink fails closed",
			baseGovernance: "ancestor-symlink",
			want:           "incomplete Schema flag migration governance artifact set",
		},
		{
			name:              "governed candidate must preserve schema checker",
			baseGovernance:    "complete",
			candidateMutation: "delete-checker",
			want:              "must preserve the complete Schema flag migration governance artifact set",
		},
		{
			name:              "governed candidate must preserve migrations implementation",
			baseGovernance:    "complete",
			candidateMutation: "delete-migrations",
			want:              "must preserve the complete Schema flag migration governance artifact set",
		},
		{
			name:              "governed candidate ledger must be a regular file",
			baseGovernance:    "complete",
			candidateMutation: "symlink-ledger",
			want:              "must preserve the complete Schema flag migration governance artifact set",
		},
		{
			name:              "governed candidate helper parent must be a Git tree",
			baseGovernance:    "complete",
			candidateMutation: "symlink-helper-parent",
			want:              "must preserve the complete Schema flag migration governance artifact set",
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
			runSchemaAuthorityCase(t, test)
		})
	}
}

func runSchemaAuthorityCase(t *testing.T, test authorityScenario) {
	t.Helper()
	repoRoot := schemaRepositoryRoot(t)
	managedRoot := filepath.Join(repoRoot, ".worktrees", "schemacompat-authority-integration")
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

	schemaCopyFile(t,
		filepath.Join(repoRoot, "scripts", "policy", "check-authoritative-schema-compatibility.sh"),
		filepath.Join(fixtureRoot, "scripts", "policy", "check-authoritative-schema-compatibility.sh"),
		0o755,
	)
	schemaCopyFile(t,
		filepath.Join(repoRoot, "scripts", "policy", "policy-runtime.sh"),
		filepath.Join(fixtureRoot, "scripts", "policy", "policy-runtime.sh"),
		0o755,
	)
	schemaCopyFile(t,
		filepath.Join(repoRoot, "scripts", "release", "release-lib.sh"),
		filepath.Join(fixtureRoot, "scripts", "release", "release-lib.sh"),
		0o755,
	)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "go.mod"), "module example.com/dws-schema-compat-authority-test\n\ngo 1.23\n", 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "main.go"), schemaBinarySource("STABLE_RELEASE"), 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "interface-snapshot", "main.go"), candidateNoopSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), schemaStableSnapshotSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "compare.go"), "package interfacesnapshot\n", 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "schema-compat", "main.go"), bootstrapSchemaCheckerSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "SNAPSHOT_MARKER"), "STABLE_RELEASE\n", 0o644)

	schemaRun(t, fixtureRoot, "git", "init", "-b", "main")
	schemaRun(t, fixtureRoot, "git", "config", "user.name", "Schema Compatibility Test")
	schemaRun(t, fixtureRoot, "git", "config", "user.email", "schema-compat-test@example.invalid")
	schemaRun(t, fixtureRoot, "git", "add", ".")
	schemaRun(t, fixtureRoot, "git", "commit", "-m", "stable release")
	stableCommit := strings.TrimSpace(schemaRun(t, fixtureRoot, "git", "rev-parse", "HEAD"))
	schemaRun(t, fixtureRoot, "git", "tag", "v1.0.0", stableCommit)

	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "main.go"), schemaBinarySource("BASE_AUTHORITY"), 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "interface-snapshot", "main.go"), authorityInterfaceHelperSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "SNAPSHOT_MARKER"), "BASE_AUTHORITY\n", 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), schemaBaseSnapshotSource, 0o644)
	switch test.baseGovernance {
	case "complete":
		checkerSource := governedSchemaCheckerSource
		if test.checkerMode == "stable-schema-guard" {
			checkerSource = stableSchemaGuardCheckerSource
		}
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "schema-compat", "main.go"), checkerSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), schemaGovernedSnapshotSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go"), "package interfacesnapshot\n", 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go"), schemaAliasContractSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"), "{\"version\":1,\"migrations\":[{\"authority\":\"BASE\"}]}\n", 0o644)
	case "partial":
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"), "{\"version\":1,\"migrations\":[]}\n", 0o644)
	case "ancestor-symlink":
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "schema-compat", "main.go"), governedSchemaCheckerSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go"), "package interfacesnapshot\n", 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go"), "package runtimeannotate\n", 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "authority-ledger", "approved-flag-migrations-v1.json"), "{\"version\":1,\"migrations\":[]}\n", 0o644)
		if err := os.Symlink("authority-ledger", filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations")); err != nil {
			t.Fatalf("symlink base Schema ledger parent: %v", err)
		}
	case "none":
	default:
		t.Fatalf("unknown base governance state %q", test.baseGovernance)
	}
	switch test.commandGovernance {
	case "":
	case "complete":
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), schemaCommandGovernedSnapshotSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "command_migrations.go"), "package interfacesnapshot\n", 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "corecmd.go"), schemaBaseCorecmdBridgeSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go"), schemaConstParamsRegistrySource("BASE"), 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "helpers", "leaf.go"), schemaBaseLeafAdapterSource, 0o644)
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-command-migrations-v1.json"), "{\"version\":1,\"migrations\":[]}\n", 0o644)
	case "bootstrap":
	default:
		t.Fatalf("unknown command governance state %q", test.commandGovernance)
	}
	schemaRun(t, fixtureRoot, "git", "add", ".")
	schemaRun(t, fixtureRoot, "git", "commit", "-m", "base authority")
	baseRef := strings.TrimSpace(schemaRun(t, fixtureRoot, "git", "rev-parse", "HEAD"))
	schemaRun(t, fixtureRoot, "git", "tag", "v1.1.0", baseRef)
	schemaRun(t, fixtureRoot, "git", "update-ref", "refs/tags/withdrawn/v1.1.0", baseRef)
	if test.baseGovernance == "ancestor-symlink" {
		ledgerPath := filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations")
		if err := os.Remove(ledgerPath); err != nil {
			t.Fatalf("remove inherited base ledger parent symlink: %v", err)
		}
		if err := os.Rename(
			filepath.Join(fixtureRoot, "scripts", "policy", "authority-ledger"),
			ledgerPath,
		); err != nil {
			t.Fatalf("restore regular candidate ledger tree: %v", err)
		}
	}

	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "main.go"), schemaBinarySource("COMMITTED_CANDIDATE"), 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "interface-snapshot", "main.go"), candidateNoopSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "schema-compat", "main.go"), candidateNoopSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "snapshot.go"), schemaCandidateSnapshotSource, 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go"), "package interfacesnapshot\n", 0o644)
	schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "runtimeannotate", "interface_alias.go"), "package runtimeannotate\n", 0o644)
	candidateLedger := "{\"version\":1,\"migrations\":[]}\n"
	if test.baseGovernance == "complete" {
		candidateLedger = "{\"version\":1,\"migrations\":[{\"candidate\":\"COMMITTED\"}]}\n"
	}
	schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json"), candidateLedger, 0o644)
	if test.commandGovernance == "complete" || test.commandGovernance == "bootstrap" {
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "command_migrations.go"), "package interfacesnapshot\n", 0o644)
		if test.commandGovernance == "bootstrap" {
			schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go"), schemaConstParamsRegistrySource("CANDIDATE"), 0o644)
		}
		commandManifest := "{\"version\":1,\"migrations\":[]}\n"
		if test.commandManifestChanged {
			commandManifest = "{\"version\":1,\"migrations\":[{\"candidate\":\"PENDING\"}]}\n"
		}
		schemaWriteFile(t, filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-command-migrations-v1.json"), commandManifest, 0o644)
	}
	schemaWriteFile(t, filepath.Join(fixtureRoot, "SNAPSHOT_MARKER"), "COMMITTED_CANDIDATE\n", 0o644)
	switch test.candidateMutation {
	case "":
	case "delete-checker":
		if err := os.Remove(filepath.Join(fixtureRoot, "scripts", "policy", "schema-compat", "main.go")); err != nil {
			t.Fatalf("remove candidate schema checker: %v", err)
		}
	case "delete-migrations":
		if err := os.Remove(filepath.Join(fixtureRoot, "internal", "interfacesnapshot", "migrations.go")); err != nil {
			t.Fatalf("remove candidate migrations implementation: %v", err)
		}
	case "symlink-ledger":
		ledger := filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json")
		if err := os.Remove(ledger); err != nil {
			t.Fatalf("remove candidate ledger: %v", err)
		}
		if err := os.Symlink("../../../CANONICAL_EMPTY", ledger); err != nil {
			t.Fatalf("symlink candidate ledger: %v", err)
		}
		schemaWriteFile(t, filepath.Join(fixtureRoot, "CANONICAL_EMPTY"), "{\"version\":1,\"migrations\":[]}\n", 0o644)
	case "forge-alias-token":
		schemaWriteFile(t,
			filepath.Join(fixtureRoot, "internal", "evil", "forged.go"),
			"package evil\n\nconst forged = \"dws.compat.alias_of\"\n",
			0o644,
		)
	case "delete-const-params-contract":
		if err := os.Remove(filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go")); err != nil {
			t.Fatalf("remove candidate bool ConstParams registry: %v", err)
		}
	case "forge-const-params-token":
		schemaWriteFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "evil_const_params.go"),
			"package corecmd\n\nfunc forgeConstParams() { attachInterfaceBoolConstParams() }\n",
			0o644,
		)
	case "forge-const-params-reader":
		schemaWriteFile(t,
			filepath.Join(fixtureRoot, "internal", "evil", "forged.go"),
			"package evil\n\nconst forged = \"InterfaceBoolConstParams\"\n",
			0o644,
		)
	case "forge-const-params-registry-store":
		schemaWriteFile(t,
			filepath.Join(fixtureRoot, "internal", "corecmd", "evil_const_params.go"),
			"package corecmd\n\nfunc forgeConstParamsStore() { interfaceBoolConstParamsRegistry.Store(nil, map[string]bool{\"forged\": true}) }\n",
			0o644,
		)
	case "forge-corecmd-bool-const-params":
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "corecmd.go"), schemaForgedCorecmdBridgeSource, 0o644)
	case "change-const-params-protocol":
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "corecmd", "interface_const_params.go"), schemaConstParamsRegistrySource("CANDIDATE"), 0o644)
	case "forge-leaf-adapter":
		schemaWriteFile(t, filepath.Join(fixtureRoot, "internal", "helpers", "leaf.go"), schemaForgedLeafAdapterSource, 0o644)
	case "symlink-helper-parent":
		helperPath := filepath.Join(fixtureRoot, "internal", "interfacesnapshot")
		alternatePath := filepath.Join(fixtureRoot, "internal", "interfacesnapshot-alt")
		if err := os.Rename(helperPath, alternatePath); err != nil {
			t.Fatalf("move candidate Schema helper behind ancestor symlink: %v", err)
		}
		if err := os.Symlink("interfacesnapshot-alt", helperPath); err != nil {
			t.Fatalf("symlink candidate Schema helper parent: %v", err)
		}
	default:
		t.Fatalf("unknown candidate mutation %q", test.candidateMutation)
	}
	schemaRun(t, fixtureRoot, "git", "add", ".")
	schemaRun(t, fixtureRoot, "git", "commit", "-m", "candidate revision")
	candidateRef := strings.TrimSpace(schemaRun(t, fixtureRoot, "git", "rev-parse", "HEAD"))

	// 让调用者工作区前进到另一提交，并放入可自批的二进制与 ledger；显式
	// candidate ref 必须隔离这些内容。
	schemaWriteFile(t, filepath.Join(fixtureRoot, "cmd", "main.go"), schemaBinarySource("LIVE_WORKTREE_BINARY"), 0o644)
	liveLedger := filepath.Join(fixtureRoot, "scripts", "policy", "interface-migrations", "approved-flag-migrations-v1.json")
	if err := os.Remove(liveLedger); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove selected candidate ledger from live revision: %v", err)
	}
	schemaWriteFile(t, liveLedger, "{\"version\":1,\"migrations\":[{\"live\":true}]}\n", 0o644)
	schemaRun(t, fixtureRoot, "git", "add", ".")
	schemaRun(t, fixtureRoot, "git", "commit", "-m", "unselected live revision")
	schemaWriteFile(t, filepath.Join(fixtureRoot, "dws"), "#!/bin/sh\nprintf '%s\\n' 'LIVE_WORKTREE_BINARY'\n", 0o755)

	cmd := exec.Command(
		"sh",
		"scripts/policy/check-authoritative-schema-compatibility.sh",
		"--base-ref", baseRef,
		"--stable-ref", "v1.0.0",
		"--candidate-ref", candidateRef,
	)
	cmd.Dir = fixtureRoot
	cmd.Env = schemaIntegrationEnvironment(fixtureRoot)
	output, runErr := cmd.CombinedOutput()
	got := string(output)
	if runErr == nil {
		t.Fatalf("Schema compatibility script unexpectedly succeeded; output:\n%s", got)
	}
	for _, forbidden := range []string{"CANDIDATE_NOOP", "LIVE_WORKTREE_BINARY"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("candidate-controlled authority %q took effect; output:\n%s", forbidden, got)
		}
	}
	if test.authorityMarker != "" {
		if !strings.Contains(got, test.authorityMarker) {
			t.Fatalf("Schema compatibility script did not use base authority %q; output:\n%s", test.authorityMarker, got)
		}
		if test.baseGovernance == "complete" && strings.Count(got, "BASE_INTERFACE_HELPER_GENERATE=BASE") != 3 {
			t.Fatalf("governed Schema check did not generate three base-owned interface snapshots; output:\n%s", got)
		}

		wrongStable := exec.Command(
			"sh",
			"scripts/policy/check-authoritative-schema-compatibility.sh",
			"--base-ref", baseRef,
			"--stable-ref", "HEAD",
			"--candidate-ref", candidateRef,
		)
		wrongStable.Dir = fixtureRoot
		wrongStable.Env = schemaIntegrationEnvironment(fixtureRoot)
		wrongOutput, wrongErr := wrongStable.CombinedOutput()
		if wrongErr == nil || !strings.Contains(string(wrongOutput), "is not the expected highest GA tag") {
			t.Fatalf("arbitrary Schema stable ref was not rejected: err=%v output:\n%s", wrongErr, wrongOutput)
		}
		return
	}
	if !strings.Contains(got, test.want) {
		t.Fatalf("Schema compatibility script output missing %q:\n%s", test.want, got)
	}
}

func schemaRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "scripts", "policy", "check-authoritative-schema-compatibility.sh")); err != nil {
		t.Fatalf("resolve repository root %s: %v", root, err)
	}
	return root
}

func schemaCopyFile(t *testing.T, source, destination string, mode os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	schemaWriteFile(t, destination, string(content), mode)
}

func schemaWriteFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func schemaRun(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func schemaIntegrationEnvironment(fixtureRoot string) []string {
	overrides := map[string]string{
		"DWS_BIN":           filepath.Join(fixtureRoot, "dws"),
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

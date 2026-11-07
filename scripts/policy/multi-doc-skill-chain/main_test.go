// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func TestValidateProtectedReferences(t *testing.T) {
	root := t.TempDir()
	relative := "skills/multi/dingtalk-doc/references/doc/style/example.md"
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("stable\n")
	if err := os.WriteFile(absolute, content, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := routeManifest{
		ProtectedReferenceRoots: []string{"skills/multi/dingtalk-doc/references/doc/style"},
		ProtectedReferenceSHA256: map[string]string{
			relative: fmt.Sprintf("%x", sha256.Sum256(content)),
		},
	}
	if failures := validateProtectedReferences(root, manifest); len(failures) != 0 {
		t.Fatalf("valid protected references failed: %v", failures)
	}

	if err := os.WriteFile(absolute, []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failures := validateProtectedReferences(root, manifest)
	if !containsFailure(failures, "sha256") {
		t.Fatalf("hash drift failures = %v", failures)
	}

	extra := filepath.Join(filepath.Dir(absolute), "unreviewed.md")
	if err := os.WriteFile(extra, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failures = validateProtectedReferences(root, manifest)
	if !containsFailure(failures, "unreviewed file") {
		t.Fatalf("unreviewed file failures = %v", failures)
	}
}

func containsFailure(failures []string, needle string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, needle) {
			return true
		}
	}
	return false
}

func TestMainReportsWorkingDirectoryFailure(t *testing.T) {
	originalGetwd, originalExit := mainGetwd, mainExit
	t.Cleanup(func() { mainGetwd, mainExit = originalGetwd, originalExit })
	mainGetwd = func() (string, error) { return "", errors.New("getwd failed") }
	exitCode := -1
	mainExit = func(code int) { exitCode = code }
	main()
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
}

func TestMainRunsRepositoryCheck(t *testing.T) {
	originalGetwd, originalExit := mainGetwd, mainExit
	t.Cleanup(func() { mainGetwd, mainExit = originalGetwd, originalExit })
	mainGetwd = func() (string, error) { return repositoryRoot(t), nil }
	exitCode := -1
	mainExit = func(code int) { exitCode = code }
	main()
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}

func TestRunCoversTopLevelOutcomes(t *testing.T) {
	originalBuild, originalBind := buildEffective, bindEffective
	t.Cleanup(func() { buildEffective, bindEffective = originalBuild, originalBind })

	t.Run("missing manifest", func(t *testing.T) {
		var stderr strings.Builder
		if code := run(t.TempDir(), &cobra.Command{}, io.Discard, &stderr); code != 2 || !strings.Contains(stderr.String(), "read doc intent route manifest") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("protected reference failure", func(t *testing.T) {
		root := writeManifestFixture(t, routeManifest{Version: 1})
		var stderr strings.Builder
		if code := run(root, &cobra.Command{}, io.Discard, &stderr); code != 1 || !strings.Contains(stderr.String(), "protected doc Reference check failed") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("manifest contract failure", func(t *testing.T) {
		root := t.TempDir()
		reference := "protected/stable.md"
		absolute := filepath.Join(root, filepath.FromSlash(reference))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte("stable\n")
		if err := os.WriteFile(absolute, content, 0o644); err != nil {
			t.Fatal(err)
		}
		manifest := routeManifest{
			Version:                 2,
			ProtectedReferenceRoots: []string{"protected"},
			ProtectedReferenceSHA256: map[string]string{
				reference: fmt.Sprintf("%x", sha256.Sum256(content)),
			},
		}
		writeManifestAtRoot(t, root, manifest)
		var stderr strings.Builder
		if code := run(root, app.NewRootCommand(), io.Discard, &stderr); code != 1 || !strings.Contains(stderr.String(), "multi doc Skill chain check failed") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	root := repositoryRoot(t)
	t.Run("effective registry failure", func(t *testing.T) {
		buildEffective = func(*cobra.Command) (cli.EffectiveCommandRegistry, error) {
			return cli.EffectiveCommandRegistry{}, errors.New("build failed")
		}
		var stderr strings.Builder
		if code := run(root, app.NewRootCommand(), io.Discard, &stderr); code != 2 || !strings.Contains(stderr.String(), "build effective CommandRegistry") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("bound registry failure", func(t *testing.T) {
		buildEffective = originalBuild
		bindEffective = func(*cobra.Command, cli.EffectiveCommandRegistry) (cli.BoundCommandRegistry, error) {
			return cli.BoundCommandRegistry{}, errors.New("bind failed")
		}
		var stderr strings.Builder
		if code := run(root, app.NewRootCommand(), io.Discard, &stderr); code != 2 || !strings.Contains(stderr.String(), "bind effective CommandRegistry") {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	})

	t.Run("repository manifest succeeds", func(t *testing.T) {
		buildEffective, bindEffective = originalBuild, originalBind
		var stdout, stderr strings.Builder
		if code := run(root, app.NewRootCommand(), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "chain check: ok") {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})
}

func TestLoadManifestRejectsUnknownAndInvalidJSON(t *testing.T) {
	root := t.TempDir()
	unknown := filepath.Join(root, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"version":1,"unknown":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	invalid := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifest(invalid); err == nil {
		t.Fatal("invalid JSON unexpectedly succeeded")
	}
}

func TestValidateProtectedReferencesRejectsInvalidDeclarations(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name     string
		manifest routeManifest
		needle   string
	}{
		{"empty roots", routeManifest{}, "roots must not be empty"},
		{"empty hashes", routeManifest{ProtectedReferenceRoots: []string{"safe"}}, "sha256 must not be empty"},
		{"unsafe root", routeManifest{ProtectedReferenceRoots: []string{"../bad"}, ProtectedReferenceSHA256: map[string]string{"safe/a.md": strings.Repeat("0", 64)}}, "invalid protected reference root"},
		{"unsafe hash path", routeManifest{ProtectedReferenceRoots: []string{"safe"}, ProtectedReferenceSHA256: map[string]string{"../bad.md": strings.Repeat("0", 64)}}, "invalid protected reference path"},
		{"outside root", routeManifest{ProtectedReferenceRoots: []string{"safe"}, ProtectedReferenceSHA256: map[string]string{"other/a.md": strings.Repeat("0", 64)}}, "outside declared roots"},
		{"invalid hash", routeManifest{ProtectedReferenceRoots: []string{"safe"}, ProtectedReferenceSHA256: map[string]string{"safe/a.md": "bad"}}, "invalid sha256"},
		{"missing file", routeManifest{ProtectedReferenceRoots: []string{"safe"}, ProtectedReferenceSHA256: map[string]string{"safe/a.md": strings.Repeat("0", 64)}}, "read protected reference"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if failures := validateProtectedReferences(root, test.manifest); !containsFailure(failures, test.needle) {
				t.Fatalf("failures = %v, want %q", failures, test.needle)
			}
		})
	}
}

func TestValidateProtectedReferencesReportsRelativePathFailure(t *testing.T) {
	original := repositoryRelative
	t.Cleanup(func() { repositoryRelative = original })
	root := t.TempDir()
	absolute := filepath.Join(root, "safe", "a.md")
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	repositoryRelative = func(string, string) (string, error) { return "", errors.New("relative failed") }
	manifest := routeManifest{
		ProtectedReferenceRoots: []string{"safe"},
		ProtectedReferenceSHA256: map[string]string{
			"safe/a.md": fmt.Sprintf("%x", sha256.Sum256([]byte("a"))),
		},
	}
	if failures := validateProtectedReferences(root, manifest); !containsFailure(failures, "relative failed") {
		t.Fatalf("failures = %v", failures)
	}
}

func TestValidateManifestReportsContractFailures(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "refs", "valid.md"), []byte("<!-- dws-intent: contract --><!-- dws-intent: contract -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := routeManifest{
		Version:     2,
		MarkerRoots: []string{"refs"},
		Intents: []intentRoute{
			{ID: "BAD ID", PreferredTool: "missing"},
			{ID: "duplicate", PreferredTool: "missing"},
			{ID: "duplicate", PreferredTool: "missing"},
			{
				ID: "contract", PreferredTool: "preferred",
				AllowedFallbacks:      []routeFallback{{}, {Tool: "fallback", ReasonCode: "BAD"}, {Tool: "fallback", ReasonCode: "ok"}, {Tool: "missing-fallback", ReasonCode: "ok"}},
				ForbiddenDefaultTools: []string{"", "forbidden", "forbidden", "missing-forbidden"},
				References:            []string{"../bad.md", "refs/valid.md", "refs/valid.md", "refs/missing.md"},
			},
		},
	}
	tools := map[string]toolFact{
		"preferred": {Canonical: "preferred"},
		"fallback":  {Canonical: "fallback"},
		"forbidden": {Canonical: "forbidden"},
	}
	failures := validateManifest(root, manifest, tools)
	for _, needle := range []string{
		"version = 2", "intent id", "duplicate intent", "absent from BoundCommandRegistry",
		"absent from ResolveMeta delivery", "needs non-empty use_when", "empty or duplicate allowed fallback",
		"invalid reason_code", "fallback tool", "empty or duplicate forbidden default", "forbidden default tool",
		"invalid reference", "repeats reference", "does not exist", "missing its dws-intent marker",
	} {
		if !containsFailure(failures, needle) {
			t.Errorf("failures missing %q: %v", needle, failures)
		}
	}
	if failures := validateManifest(root, routeManifest{Version: 1}, tools); !containsFailure(failures, "marker_roots must not be empty") {
		t.Fatalf("empty marker roots failures = %v", failures)
	}
	if failures := validateManifest(root, routeManifest{Version: 1, MarkerRoots: []string{"refs"}}, tools); !containsFailure(failures, "intents must not be empty") {
		t.Fatalf("empty intents failures = %v", failures)
	}
}

func TestScanMarkersReportsLinksRoutesAndOrphans(t *testing.T) {
	root := t.TempDir()
	refs := filepath.Join(root, "refs")
	if err := os.MkdirAll(refs, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"SKILL.md":   "[known](known.md) [remote](https://example.com/remote.md)\n",
		"known.md":   "<!-- dws-intent: known --><!-- dws-intent: known --> use `dws doc +wrong` and `dws drive +upload`\n[dead](missing.md)\n",
		"unknown.md": "<!-- dws-intent: unknown -->\n",
		"orphan.md":  "plain\n",
		"allowed.md": "plain\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(refs, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	intents := map[string]intentRoute{
		"known": {ID: "known", PreferredTool: "preferred", ForbiddenDefaultTools: []string{"forbidden"}, References: []string{"different.md"}},
	}
	tools := map[string]toolFact{
		"preferred": {PrimaryPath: "doc +read"},
		"forbidden": {PrimaryPath: "drive +upload"},
	}
	failures, markers := scanMarkers(root, []string{"../bad", "refs", "missing-root"}, []string{"refs/allowed.md", "refs/unknown.md"}, intents, tools)
	for _, needle := range []string{"invalid marker root", "dead reference link", "unknown dws-intent", "undeclared dws-intent", "must contain preferred path", "uses forbidden default", "scan marker root", "orphan reference"} {
		if !containsFailure(failures, needle) {
			t.Errorf("failures missing %q: %v", needle, failures)
		}
	}
	if markers["known\x00refs/known.md"] != 2 {
		t.Fatalf("markers = %v", markers)
	}
}

func TestScanMarkersReportsOpenFailureAndMissingPreferredTool(t *testing.T) {
	originalOpen := markerOpen
	t.Cleanup(func() { markerOpen = originalOpen })
	root := t.TempDir()
	refs := filepath.Join(root, "refs")
	if err := os.MkdirAll(refs, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(refs, "route.md")
	if err := os.WriteFile(path, []byte("<!-- dws-intent: known -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markerOpen = func(string) (*os.File, error) { return nil, errors.New("open failed") }
	if failures, _ := scanMarkers(root, []string{"refs"}, nil, nil, nil); !containsFailure(failures, "open failed") {
		t.Fatalf("open failures = %v", failures)
	}
	markerOpen = originalOpen
	intents := map[string]intentRoute{"known": {ID: "known", PreferredTool: "missing", References: []string{"refs/route.md"}}}
	if failures, markers := scanMarkers(root, []string{"refs"}, []string{"refs/route.md"}, intents, nil); len(failures) != 0 || markers["known\x00refs/route.md"] != 1 {
		t.Fatalf("failures=%v markers=%v", failures, markers)
	}
}

func TestPathHelpers(t *testing.T) {
	for _, test := range []struct {
		line, path string
		want       bool
	}{
		{"use dws doc +read", "doc +read", true},
		{"use dws doc +read `now`", "doc +read", true},
		{"use dws doc +reader", "doc +read", false},
		{"nothing", "doc +read", false},
	} {
		if got := containsCLIPath(test.line, test.path); got != test.want {
			t.Errorf("containsCLIPath(%q, %q) = %v, want %v", test.line, test.path, got, test.want)
		}
	}
	if !stringSliceContains([]string{"refs/a.md"}, "refs/a.md") || stringSliceContains([]string{"refs/a.md"}, "refs/b.md") {
		t.Fatal("stringSliceContains returned an unexpected result")
	}
}

func writeManifestFixture(t *testing.T, manifest routeManifest) string {
	t.Helper()
	root := t.TempDir()
	writeManifestAtRoot(t, root, manifest)
	return root
}

func writeManifestAtRoot(t *testing.T, root string, manifest routeManifest) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(manifestRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunSkillCommandCheck(t *testing.T) {
	root := testCommandRoot()
	directory := t.TempDir()
	skills := filepath.Join(directory, "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skills, "SKILL.md")
	body := "Use `dws auth login`.\nUse `dws auth login` again.\nUse `dws auth bogus-command`.\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(directory, root, &stdout, &stderr); code != 1 {
		t.Fatalf("invalid command code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "SKILL.md:3") || !strings.Contains(stderr.String(), "command path does not exist") {
		t.Fatalf("unexpected failure %q", stderr.String())
	}

	body = "Use `dws auth login`.\n占位写法 `dws <cmd> --help`.\n禁止使用 `dws auth bogus-command`.\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(directory, testCommandRoot(), &stdout, &stderr); code != 0 {
		t.Fatalf("valid commands code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ok (1 executable command paths)") {
		t.Fatalf("unexpected success output %q", stdout.String())
	}

	stderr.Reset()
	if code := run(filepath.Join(directory, "missing"), testCommandRoot(), &stdout, &stderr); code != 2 {
		t.Fatalf("missing skills code=%d, want 2", code)
	}
}

func TestExtractReferences(t *testing.T) {
	directory := t.TempDir()
	markdown := filepath.Join(directory, "sample.md")
	body := "Inline `dws auth login`.\n```sh\n$ dws schema \"dev app create\"\nnot a command\n```\n❌ `dws bogus`\n"
	if err := os.WriteFile(markdown, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ignored.txt"), []byte("dws bogus"), 0o600); err != nil {
		t.Fatal(err)
	}
	refs, err := extractReferences(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0].Line != 1 || refs[1].Line != 3 {
		t.Fatalf("references=%v", refs)
	}
}

func TestShellFieldsAndFailureFormatting(t *testing.T) {
	got := shellFields(`dws schema "dev app create" 'single value' escaped\ value`)
	want := []string{"dws", "schema", "dev app create", "single value", "escaped value"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("shellFields()=%v, want %v", got, want)
	}
	root := t.TempDir()
	ref := commandRef{File: filepath.Join(root, "skills", "SKILL.md"), Line: 7, Text: "dws bogus"}
	wantFailure := filepath.Join("skills", "SKILL.md") + ":7: `dws bogus`: missing"
	if got := formatFailure(root, ref, "missing"); got != wantFailure {
		t.Fatalf("formatFailure()=%q, want %q", got, wantFailure)
	}
	if !isAntiPatternLine("这是错误命令 dws bogus") || isAntiPatternLine("dws auth login") {
		t.Fatal("anti-pattern marker detection is incorrect")
	}
}

func testCommandRoot() *cobra.Command {
	root := &cobra.Command{Use: "dws", Args: cobra.NoArgs}
	auth := &cobra.Command{Use: "auth", Args: cobra.NoArgs}
	auth.AddCommand(&cobra.Command{Use: "login", Aliases: []string{"signin"}})
	root.AddCommand(auth, &cobra.Command{Use: "schema [path]", Args: cobra.MaximumNArgs(1)})
	return root
}

func TestParseReferenceExtractsCommandPath(t *testing.T) {
	path, flags, skip := parseReference(`dws doc read --node <DOC_ID> --format json`)
	if skip || path != "dws doc read" {
		t.Fatalf("parseReference() = %q, skip=%v", path, skip)
	}
	if len(flags) != 2 || flags[0] != "format" || flags[1] != "node" {
		t.Fatalf("flags = %v", flags)
	}
}

func TestParseReferenceSkipsCombinedDocumentationNotation(t *testing.T) {
	if _, _, skip := parseReference(`dws doc create/update --content-format jsonml`); !skip {
		t.Fatal("combined create/update notation should not be treated as an executable command")
	}
}

func TestParseReferenceSkipsShellComposition(t *testing.T) {
	if _, _, skip := parseReference(`dws A & dws B & wait`); !skip {
		t.Fatal("shell composition should not be treated as an executable command")
	}
}

func TestResolveCommandReference(t *testing.T) {
	root := &cobra.Command{Use: "dws", Args: cobra.NoArgs}
	auth := &cobra.Command{Use: "auth", Args: cobra.NoArgs}
	auth.AddCommand(&cobra.Command{Use: "login", Aliases: []string{"signin"}})
	sheet := &cobra.Command{Use: "sheet", Args: cobra.NoArgs}
	sheet.AddCommand(&cobra.Command{Use: "read"})
	root.AddCommand(
		auth,
		sheet,
		&cobra.Command{Use: "schema [path]", Args: cobra.MaximumNArgs(1)},
		&cobra.Command{Use: "plugin-info <name>", Args: cobra.ExactArgs(1)},
	)

	tests := []struct {
		name string
		path string
		want commandResolution
	}{
		{name: "existing leaf", path: "dws auth login", want: resolutionValid},
		{name: "existing group", path: "dws auth", want: resolutionValid},
		{name: "top-level alias", path: "dws auth signin", want: resolutionValid},
		{name: "missing top-level command", path: "dws bogus-command", want: resolutionInvalid},
		{name: "missing nested command", path: "dws auth bogus-command", want: resolutionInvalid},
		{name: "top-level placeholder", path: "dws <cmd>", want: resolutionSkip},
		{name: "nested placeholder", path: "dws sheet <command>", want: resolutionSkip},
		{name: "quoted positional argument", path: "dws schema dev app create", want: resolutionValid},
		{name: "leaf positional argument", path: "dws plugin-info example", want: resolutionValid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveCommandReference(root, test.path); got != test.want {
				t.Fatalf("resolveCommandReference(%q) = %d, want %d", test.path, got, test.want)
			}
		})
	}
}

func TestIsPlaceholder(t *testing.T) {
	for _, token := range []string{"<cmd>", "<子命令>", "[optional]"} {
		if !isPlaceholder(token) {
			t.Errorf("isPlaceholder(%q) = false, want true", token)
		}
	}
	for _, token := range []string{"cmd", "<cmd", "cmd>", "[optional"} {
		if isPlaceholder(token) {
			t.Errorf("isPlaceholder(%q) = true, want false", token)
		}
	}
}

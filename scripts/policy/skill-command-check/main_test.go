// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

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

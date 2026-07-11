// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import "testing"

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

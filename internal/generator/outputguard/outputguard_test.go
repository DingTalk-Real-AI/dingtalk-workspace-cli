// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package outputguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectsHardLinkToProtectedDirectoryMember(t *testing.T) {
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

func TestValidateRepoTargetAllowlist(t *testing.T) {
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

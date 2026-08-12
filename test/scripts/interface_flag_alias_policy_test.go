// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameworkOwnsInterfaceFlagAliasEvidence(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	allowed := map[string]bool{
		"internal/corecmd/corecmd.go":                         true,
		"internal/corecmd/runtimeannotate/interface_alias.go": true,
		"internal/interfacesnapshot/snapshot.go":              true,
	}
	protectedTokens := []string{
		"dws.compat.alias_of",
		"dws.compat.alias_origin",
		"corecmd.flag_spec_aliases.v1",
		"AnnotationFlagAliasOf",
		"AnnotationFlagAliasOrigin",
		"FlagAliasOriginCorecmdV1",
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".worktrees", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if allowed[relative] {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range protectedTokens {
			if strings.Contains(string(content), token) {
				t.Errorf("production file %s uses framework-owned flag alias evidence token %q", relative, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production Go files: %v", err)
	}
}

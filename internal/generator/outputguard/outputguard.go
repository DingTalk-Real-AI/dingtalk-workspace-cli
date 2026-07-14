// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package outputguard prevents generators from overwriting reviewed or
// generated inputs that participate in the same one-way build graph.
package outputguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Input is a protected generator input. Relative paths are resolved against
// the generator's explicit repository root.
type Input struct {
	Name string
	Path string
}

// Target is a prospective generator output. Relative paths retain CLI
// semantics and are resolved against the current working directory.
type Target struct {
	Name      string
	Path      string
	Directory bool
}

// Validate rejects any target that is identical to, contains, or is contained
// by a protected input. Existing symlinks and hard links are resolved before
// comparison; paths that do not exist resolve through their longest existing
// ancestor.
func Validate(root string, inputs []Input, targets []Target) error {
	type resolvedInput struct {
		input Input
		path  string
		info  os.FileInfo
	}
	resolvedInputs := make([]resolvedInput, 0, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input.Path) == "" {
			return fmt.Errorf("%s path cannot be empty", firstNonEmpty(input.Name, "protected input"))
		}
		path, err := canonicalPath(resolveRootPath(root, input.Path))
		if err != nil {
			return fmt.Errorf("resolve %s %q: %w", firstNonEmpty(input.Name, "protected input"), input.Path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s %q: %w", firstNonEmpty(input.Name, "protected input"), input.Path, err)
		}
		resolvedInputs = append(resolvedInputs, resolvedInput{input: input, path: path, info: info})
	}

	for _, target := range targets {
		if strings.TrimSpace(target.Path) == "" {
			continue
		}
		targetPath, err := canonicalPath(target.Path)
		if err != nil {
			return fmt.Errorf("resolve %s %q: %w", firstNonEmpty(target.Name, "output"), target.Path, err)
		}
		targetInfo, statErr := os.Stat(targetPath)
		if statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("stat %s %q: %w", firstNonEmpty(target.Name, "output"), target.Path, statErr)
		}
		for _, input := range resolvedInputs {
			if samePath(input.path, targetPath) || (statErr == nil && os.SameFile(input.info, targetInfo)) {
				return overlapError(target, input.input, "resolves to")
			}
			if statErr == nil && input.info.IsDir() {
				sameMember, walkErr := directoryContainsSameFile(input.path, targetInfo)
				if walkErr != nil {
					return fmt.Errorf("inspect protected %s %q: %w", firstNonEmpty(input.input.Name, "input"), input.input.Path, walkErr)
				}
				if sameMember {
					return overlapError(target, input.input, "is a hard link to a member of")
				}
			}
			if target.Directory && pathContains(targetPath, input.path) {
				return overlapError(target, input.input, "contains")
			}
			if input.info.IsDir() && pathContains(input.path, targetPath) {
				return overlapError(target, input.input, "is inside")
			}
			if statErr == nil && targetInfo.IsDir() && pathContains(targetPath, input.path) {
				return overlapError(target, input.input, "contains")
			}
		}
	}
	return nil
}

func directoryContainsSameFile(root string, target os.FileInfo) (bool, error) {
	found := false
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && os.SameFile(info, target) {
			found = true
		}
		return nil
	})
	return found, err
}

// ValidateRepoTargetAllowlist permits arbitrary temporary outputs outside the
// repository, but restricts in-repository writes to explicit canonical
// delivery targets. This closes transitive source-graph gaps without requiring
// every future input file to be rediscovered by each generator guard.
func ValidateRepoTargetAllowlist(root string, target Target, allowedRelativePaths ...string) error {
	if strings.TrimSpace(target.Path) == "" {
		return nil
	}
	rootPath, err := canonicalPath(root)
	if err != nil {
		return fmt.Errorf("resolve repository root %q: %w", root, err)
	}
	targetPath, err := canonicalPath(target.Path)
	if err != nil {
		return fmt.Errorf("resolve %s %q: %w", firstNonEmpty(target.Name, "output"), target.Path, err)
	}
	if !samePath(rootPath, targetPath) && !pathContains(rootPath, targetPath) {
		return nil
	}
	for _, allowed := range allowedRelativePaths {
		allowedPath, resolveErr := canonicalPath(resolveRootPath(root, allowed))
		if resolveErr != nil {
			return fmt.Errorf("resolve allowed repository output %q: %w", allowed, resolveErr)
		}
		if samePath(targetPath, allowedPath) {
			return nil
		}
	}
	return fmt.Errorf("%s %q is inside the repository but is not a canonical generated delivery target", firstNonEmpty(target.Name, "output"), target.Path)
}

func overlapError(target Target, input Input, relation string) error {
	return fmt.Errorf("%s %q %s %s %q",
		firstNonEmpty(target.Name, "output"), target.Path, relation,
		firstNonEmpty(input.Name, "input"), input.Path)
}

func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return "", err
	}
	current := absPath
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absPath, nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func resolveRootPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func pathContains(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." {
		return rel == "."
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

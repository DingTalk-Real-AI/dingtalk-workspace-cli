// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package skillprovenance builds the per-Skill provenance records stored in
// the single ~/.dws/skills-state.json metadata file.
package skillprovenance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DigestScope = "skill-directory-v1"

	SourceSkillSetup = "dws-skill-setup"
	SourceUpgrade    = "dws-upgrade"
)

// Record identifies one DWS-managed Skill. Ownership is derived from these
// records, never from files placed inside an Agent's Skill directory.
type Record struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	Digest      string `json:"digest"`
	DigestScope string `json:"digest_scope"`
}

var (
	provenanceWalkDir  = filepath.WalkDir
	provenanceRelative = filepath.Rel
	provenanceReadFile = os.ReadFile
)

// Build returns a deterministic provenance record for a Skill directory.
func Build(name, dir, version, source string) (Record, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Record{}, fmt.Errorf("provenance Skill name is empty")
	}
	digest, err := DigestDir(dir)
	if err != nil {
		return Record{}, err
	}
	version = strings.TrimSpace(version)
	if version == "" {
		version = "unknown"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return Record{}, fmt.Errorf("provenance source is empty")
	}
	return Record{
		Name:        name,
		Version:     version,
		Source:      source,
		Digest:      digest,
		DigestScope: DigestScope,
	}, nil
}

// DigestDir hashes every regular file ordered by slash-normalized relative
// path. Paths and contents are NUL-delimited.
func DigestDir(root string) (string, error) {
	var paths []string
	err := provenanceWalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := provenanceRelative(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported non-regular Skill file: %s", path)
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	hash := sha256.New()
	for _, rel := range paths {
		_, _ = hash.Write([]byte(rel))
		_, _ = hash.Write([]byte{0})
		content, err := provenanceReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// Merge replaces records with matching names and returns a name-sorted set.
func Merge(existing, updates []Record) []Record {
	byName := make(map[string]Record, len(existing)+len(updates))
	for _, record := range existing {
		if strings.TrimSpace(record.Name) != "" {
			byName[record.Name] = record
		}
	}
	for _, record := range updates {
		if strings.TrimSpace(record.Name) != "" {
			byName[record.Name] = record
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Record, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

func Names(records []Record) map[string]bool {
	names := make(map[string]bool, len(records))
	for _, record := range records {
		if record.Name != "" {
			names[record.Name] = true
		}
	}
	return names
}

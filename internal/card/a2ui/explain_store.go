// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

//go:embed explain.json
var bundledExplain embed.FS

type explainEntry struct {
	Kind       string          `json:"kind"`
	Definition json.RawMessage `json:"definition"`
}

type explainIndex struct {
	FormatVersion  int                     `json:"formatVersion"`
	ManifestSHA256 string                  `json:"manifestSha256"`
	Entries        map[string]explainEntry `json:"entries"`
}

// ExplainStore parses one bundled JSON document, retaining definitions as raw JSON.
// Named lookup decodes only the requested definition and never compiles Schema.
type ExplainStore struct {
	index        explainIndex
	sourceSHA256 string
}

func NewExplainStore(source fs.FS) (*ExplainStore, error) {
	data, err := fs.ReadFile(source, "explain.json")
	if err != nil {
		return nil, err
	}
	var index explainIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.FormatVersion != 1 || len(index.Entries) == 0 || len(index.ManifestSHA256) != 64 {
		return nil, fmt.Errorf("invalid explain index")
	}
	for name, entry := range index.Entries {
		if name == "" || entry.Kind == "" || len(entry.Definition) == 0 {
			return nil, fmt.Errorf("invalid explain index entry: %s", name)
		}
	}
	return &ExplainStore{index: index, sourceSHA256: fmt.Sprintf("%x", sha256.Sum256(data))}, nil
}

var explainOnce sync.Once
var explainCached *ExplainStore
var explainCachedError error

func BundledExplain() (*ExplainStore, error) {
	explainOnce.Do(func() {
		explainCached, explainCachedError = NewExplainStore(bundledExplain)
	})
	return explainCached, explainCachedError
}

func (s *ExplainStore) Names() []string {
	names := make([]string, 0, len(s.index.Entries))
	for name := range s.index.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *ExplainStore) lookupCanonical(name string) (map[string]any, error) {
	entry := s.index.Entries[name]
	value, err := decode(entry.Definition)
	if err != nil {
		return nil, err
	}
	result := object(value)
	if result == nil || result["kind"] != entry.Kind || result["name"] != name {
		return nil, fmt.Errorf("explain contract mismatch: %s", name)
	}
	return result, nil
}

func (s *ExplainStore) Lookup(name string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if _, ok := s.index.Entries[name]; ok {
		return s.lookupCanonical(name)
	}
	names := s.Names()
	for _, key := range names {
		entry := s.index.Entries[key]
		if entry.Kind != "token-item" && strings.EqualFold(key, name) {
			return s.lookupCanonical(key)
		}
	}
	type candidate struct {
		name     string
		distance int
	}
	candidates := []candidate{}
	for _, key := range names {
		if s.index.Entries[key].Kind == "token-item" {
			continue
		}
		distance := editDistance(strings.ToLower(name), strings.ToLower(key))
		if distance <= len([]rune(key))/2+1 {
			candidates = append(candidates, candidate{key, distance})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].name < candidates[j].name
	})
	suggestions := []string{}
	for i, c := range candidates {
		if i == 5 {
			break
		}
		suggestions = append(suggestions, c.name)
	}
	return map[string]any{"kind": "unknown", "name": name, "suggestions": suggestions}, nil
}

func (s *ExplainStore) LookupMany(names []string, compact bool) (map[string]any, error) {
	contracts := []any{}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		result, err := s.Lookup(name)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, result)
		seen[name] = true
	}
	return bundleContracts(contracts, compact), nil
}

func (s *ExplainStore) CheckManifest(manifest map[string]string) error {
	// A string-to-string map has no unsupported values or cycles.
	data, _ := json.Marshal(manifest)
	data = append(data, '\n')
	if fmt.Sprintf("%x", sha256.Sum256(data)) != s.index.ManifestSHA256 {
		return fmt.Errorf("explain index and protocol manifest have drifted")
	}
	return nil
}

// CheckAll verifies the exact bundle against the independently generated validator
// assets, then checks the manifest and every decoded definition. A manifest hash
// alone cannot detect removed, added, or incomplete definitions.
func (s *ExplainStore) CheckAll(manifest map[string]string, expectedSHA256 string) error {
	if len(expectedSHA256) != 64 || s.sourceSHA256 != expectedSHA256 {
		return fmt.Errorf("explain bundle content differs from validator assets")
	}
	if err := s.CheckManifest(manifest); err != nil {
		return err
	}
	for _, name := range s.Names() {
		if _, err := s.lookupCanonical(name); err != nil {
			return err
		}
	}
	return nil
}

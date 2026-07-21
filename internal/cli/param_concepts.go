// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const paramConceptsSchemaRef = "./param_concepts.schema.json"

// param_concepts.json is the reviewed, typed parameter concept dictionary and
// the sole source of equivalent flag spellings ("concepts") plus per-command
// overrides. Build-time generators reduce these concepts against each command's
// real Cobra flags; generated alias tables are downstream views and must never
// be read back here.

//go:embed param_concepts.json
var embeddedParamConceptsJSON []byte

//go:embed param_concepts.schema.json
var embeddedParamConceptsSchemaJSON []byte

var (
	paramConceptIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	paramFlagTokenPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	paramCommandPathPattern = regexp.MustCompile(`^[A-Za-z0-9+][A-Za-z0-9._:+-]*$`)
)

// didYouMean sentinels are the only non-flag values a fixture case may expect.
const (
	paramDidYouMeanAmbiguous = "did-you-mean:ambiguous"
	paramDidYouMeanBlocked   = "did-you-mean:blocked"
)

type paramConceptsSnapshot struct {
	Schema     string                          `json:"$schema"`
	Version    int                             `json:"version"`
	MorphRules map[string]ParamMorphRule       `json:"morphological_rules,omitempty"`
	Concepts   map[string]paramConceptSpec     `json:"concepts"`
	Overrides  map[string]paramCommandOverride `json:"command_overrides,omitempty"`
	Fixture    *paramValidationFixtureSpec     `json:"validation_fixture,omitempty"`
}

type paramConceptSpec struct {
	Denotes       string   `json:"denotes"`
	CanonicalHint string   `json:"canonical_hint"`
	Members       []string `json:"members"`
	Excludes      []string `json:"excludes,omitempty"`
	Risk          string   `json:"risk"`
}

type paramCommandOverride struct {
	Bind          map[string]string `json:"bind,omitempty"`
	ScopedAliases map[string]string `json:"scoped_aliases,omitempty"`
	Block         []string          `json:"block,omitempty"`
	Ambiguous     []string          `json:"ambiguous,omitempty"`
	Confirm       bool              `json:"confirm,omitempty"`
	ScopeStrict   bool              `json:"scope_strict,omitempty"`
	Investigate   bool              `json:"investigate,omitempty"`
	Note          string            `json:"note,omitempty"`
}

type paramValidationFixtureSpec struct {
	Cases []paramFixtureCaseSpec `json:"cases"`
}

type paramFixtureCaseSpec struct {
	Command string `json:"command"`
	Emitted string `json:"emitted"`
	Expect  string `json:"expect"`
	Via     string `json:"via,omitempty"`
	Occ     int    `json:"occ,omitempty"`
}

// ParamMorphRule documents one table-free name normalization behavior. It is
// evidence for the shared Morph function; it is not a per-command alias.
type ParamMorphRule struct {
	Desc    string `json:"desc"`
	Enabled bool   `json:"enabled"`
	Guard   string `json:"guard,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Concept is one reviewed set of equivalent flag spellings that all denote a
// single entity. Members reduce onto the command's real flag; Excludes lists
// spellings that denote a different entity and must never be reduced in.
type Concept struct {
	ID            string
	Denotes       string
	CanonicalHint string
	Members       []string
	Excludes      []string
	Risk          string
}

// CommandOverride is one reviewed per-command adjustment: binding a generic
// real flag to a concept, command-scoped aliases, blocks, and the reviewed
// co-occurrence whitelist.
type CommandOverride struct {
	CommandPath   string
	Bind          map[string]string
	ScopedAliases map[string]string
	Block         []string
	Ambiguous     []string
	Confirm       bool
	ScopeStrict   bool
	Investigate   bool
	Note          string
}

// ParamFixtureCase is one reviewed regression assertion derived from evaluation
// bad cases: the emitted name on Command must reduce to Expect (a real flag) or
// route to a did-you-mean sentinel.
type ParamFixtureCase struct {
	Command string
	Emitted string
	Expect  string
	Via     string
	Occ     int
}

// ParamConcepts is the decoded, validated reviewed concept dictionary.
type ParamConcepts struct {
	Version   int
	Morph     map[string]ParamMorphRule
	Concepts  []Concept
	ByConcept map[string]Concept
	Overrides []CommandOverride
	Fixture   []ParamFixtureCase
}

var (
	embeddedParamConceptsOnce sync.Once
	embeddedParamConceptsData ParamConcepts
	embeddedParamConceptsErr  error
	loadReviewedParamConcepts = loadEmbeddedParamConcepts
)

// LoadParamConcepts decodes and validates the embedded reviewed concept
// dictionary exactly once.
func LoadParamConcepts() (ParamConcepts, error) {
	return loadReviewedParamConcepts()
}

func loadEmbeddedParamConcepts() (ParamConcepts, error) {
	embeddedParamConceptsOnce.Do(func() {
		embeddedParamConceptsData, embeddedParamConceptsErr = decodeParamConcepts(embeddedParamConceptsJSON)
	})
	return cloneParamConcepts(embeddedParamConceptsData), embeddedParamConceptsErr
}

func decodeParamConcepts(data []byte) (ParamConcepts, error) {
	var snapshot paramConceptsSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return ParamConcepts{}, fmt.Errorf("decode reviewed parameter concepts: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return ParamConcepts{}, fmt.Errorf("decode reviewed parameter concepts: %w", err)
	}
	if snapshot.Version != 1 {
		return ParamConcepts{}, fmt.Errorf("unsupported parameter concepts version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.Schema) != paramConceptsSchemaRef {
		return ParamConcepts{}, fmt.Errorf("parameter concepts must declare $schema=%q", paramConceptsSchemaRef)
	}
	if len(snapshot.Concepts) == 0 {
		return ParamConcepts{}, fmt.Errorf("parameter concepts declares no concepts")
	}

	concepts, byConcept, err := decodeParamConceptSpecs(snapshot.Concepts)
	if err != nil {
		return ParamConcepts{}, err
	}

	overrides, err := decodeParamCommandOverrides(snapshot.Overrides, byConcept)
	if err != nil {
		return ParamConcepts{}, err
	}

	fixture, err := decodeParamFixtureCases(snapshot.Fixture)
	if err != nil {
		return ParamConcepts{}, err
	}

	morph := make(map[string]ParamMorphRule, len(snapshot.MorphRules))
	for name, rule := range snapshot.MorphRules {
		if strings.TrimSpace(rule.Desc) == "" {
			return ParamConcepts{}, fmt.Errorf("parameter concepts morph rule %q has empty desc", name)
		}
		morph[name] = rule
	}

	return ParamConcepts{
		Version:   snapshot.Version,
		Morph:     morph,
		Concepts:  concepts,
		ByConcept: byConcept,
		Overrides: overrides,
		Fixture:   fixture,
	}, nil
}

// decodeParamConceptSpecs validates every concept and enforces two purity
// invariants: members are unique across all concepts, and no member appears in
// its own excludes list.
func decodeParamConceptSpecs(specs map[string]paramConceptSpec) ([]Concept, map[string]Concept, error) {
	ids := make([]string, 0, len(specs))
	for id := range specs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	concepts := make([]Concept, 0, len(ids))
	byConcept := make(map[string]Concept, len(ids))
	memberOwner := make(map[string]string)
	for _, id := range ids {
		if !paramConceptIDPattern.MatchString(id) {
			return nil, nil, fmt.Errorf("parameter concepts contains invalid concept id %q", id)
		}
		spec := specs[id]
		if strings.TrimSpace(spec.Denotes) == "" {
			return nil, nil, fmt.Errorf("concept %s has empty denotes", id)
		}
		if !paramFlagTokenPattern.MatchString(spec.CanonicalHint) {
			return nil, nil, fmt.Errorf("concept %s has invalid canonical_hint %q", id, spec.CanonicalHint)
		}
		switch spec.Risk {
		case "green", "yellow":
		default:
			return nil, nil, fmt.Errorf("concept %s has invalid risk %q", id, spec.Risk)
		}
		if len(spec.Members) == 0 {
			return nil, nil, fmt.Errorf("concept %s has no members", id)
		}
		members := make([]string, 0, len(spec.Members))
		memberSet := make(map[string]bool, len(spec.Members))
		for _, member := range spec.Members {
			if !paramFlagTokenPattern.MatchString(member) {
				return nil, nil, fmt.Errorf("concept %s has invalid member %q", id, member)
			}
			if memberSet[member] {
				return nil, nil, fmt.Errorf("concept %s repeats member %q", id, member)
			}
			memberSet[member] = true
			if owner, exists := memberOwner[member]; exists {
				return nil, nil, fmt.Errorf("member %q belongs to both concept %s and %s", member, owner, id)
			}
			memberOwner[member] = id
			members = append(members, member)
		}
		excludes := make([]string, 0, len(spec.Excludes))
		excludeSet := make(map[string]bool, len(spec.Excludes))
		for _, exclude := range spec.Excludes {
			if !paramFlagTokenPattern.MatchString(exclude) {
				return nil, nil, fmt.Errorf("concept %s has invalid exclude %q", id, exclude)
			}
			if excludeSet[exclude] {
				return nil, nil, fmt.Errorf("concept %s repeats exclude %q", id, exclude)
			}
			excludeSet[exclude] = true
			if memberSet[exclude] {
				return nil, nil, fmt.Errorf("concept %s lists %q as both member and exclude", id, exclude)
			}
			excludes = append(excludes, exclude)
		}
		concept := Concept{
			ID:            id,
			Denotes:       strings.TrimSpace(spec.Denotes),
			CanonicalHint: spec.CanonicalHint,
			Members:       members,
			Excludes:      excludes,
			Risk:          spec.Risk,
		}
		concepts = append(concepts, concept)
		byConcept[id] = concept
	}
	return concepts, byConcept, nil
}

func decodeParamCommandOverrides(specs map[string]paramCommandOverride, byConcept map[string]Concept) ([]CommandOverride, error) {
	paths := make([]string, 0, len(specs))
	for path := range specs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	overrides := make([]CommandOverride, 0, len(paths))
	for _, path := range paths {
		if !validParamCommandPath(path) {
			return nil, fmt.Errorf("command_overrides contains invalid command path %q", path)
		}
		spec := specs[path]
		if len(spec.Bind) == 0 && len(spec.ScopedAliases) == 0 && len(spec.Block) == 0 && len(spec.Ambiguous) == 0 {
			return nil, fmt.Errorf("command_override %q declares no bind/scoped_aliases/block/ambiguous", path)
		}
		for flag, conceptID := range spec.Bind {
			if !paramFlagTokenPattern.MatchString(flag) {
				return nil, fmt.Errorf("command_override %q bind has invalid flag %q", path, flag)
			}
			if _, ok := byConcept[conceptID]; !ok {
				return nil, fmt.Errorf("command_override %q binds %q to undeclared concept %q", path, flag, conceptID)
			}
		}
		for emitted, realFlag := range spec.ScopedAliases {
			if !paramFlagTokenPattern.MatchString(emitted) {
				return nil, fmt.Errorf("command_override %q scoped_aliases has invalid emitted %q", path, emitted)
			}
			if !paramFlagTokenPattern.MatchString(realFlag) {
				return nil, fmt.Errorf("command_override %q scoped_aliases has invalid target %q", path, realFlag)
			}
		}
		if err := validParamTokenList(path, "block", spec.Block); err != nil {
			return nil, err
		}
		if err := validParamTokenList(path, "ambiguous", spec.Ambiguous); err != nil {
			return nil, err
		}
		overrides = append(overrides, CommandOverride{
			CommandPath:   path,
			Bind:          cloneStringMap(spec.Bind),
			ScopedAliases: cloneStringMap(spec.ScopedAliases),
			Block:         append([]string(nil), spec.Block...),
			Ambiguous:     append([]string(nil), spec.Ambiguous...),
			Confirm:       spec.Confirm,
			ScopeStrict:   spec.ScopeStrict,
			Investigate:   spec.Investigate,
			Note:          strings.TrimSpace(spec.Note),
		})
	}
	return overrides, nil
}

func decodeParamFixtureCases(spec *paramValidationFixtureSpec) ([]ParamFixtureCase, error) {
	if spec == nil {
		return nil, nil
	}
	if len(spec.Cases) == 0 {
		return nil, fmt.Errorf("validation_fixture declares no cases")
	}
	cases := make([]ParamFixtureCase, 0, len(spec.Cases))
	for i, c := range spec.Cases {
		if !validParamCommandPath(c.Command) {
			return nil, fmt.Errorf("validation_fixture case %d has invalid command %q", i, c.Command)
		}
		if !paramFlagTokenPattern.MatchString(c.Emitted) {
			return nil, fmt.Errorf("validation_fixture case %d has invalid emitted %q", i, c.Emitted)
		}
		expect := strings.TrimSpace(c.Expect)
		if expect == "" {
			return nil, fmt.Errorf("validation_fixture case %d has empty expect", i)
		}
		if strings.HasPrefix(expect, "did-you-mean:") {
			if expect != paramDidYouMeanAmbiguous && expect != paramDidYouMeanBlocked {
				return nil, fmt.Errorf("validation_fixture case %d has unknown did-you-mean sentinel %q", i, expect)
			}
		} else if !paramFlagTokenPattern.MatchString(expect) {
			return nil, fmt.Errorf("validation_fixture case %d has invalid expect %q", i, expect)
		}
		if c.Occ < 0 {
			return nil, fmt.Errorf("validation_fixture case %d has negative occ %d", i, c.Occ)
		}
		cases = append(cases, ParamFixtureCase{
			Command: c.Command,
			Emitted: c.Emitted,
			Expect:  expect,
			Via:     strings.TrimSpace(c.Via),
			Occ:     c.Occ,
		})
	}
	return cases, nil
}

func validParamCommandPath(path string) bool {
	if strings.TrimSpace(path) != path || path == "" {
		return false
	}
	for _, token := range strings.Split(path, " ") {
		if !paramCommandPathPattern.MatchString(token) {
			return false
		}
	}
	return true
}

func validParamTokenList(path, field string, tokens []string) error {
	seen := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		if !paramFlagTokenPattern.MatchString(token) {
			return fmt.Errorf("command_override %q %s has invalid token %q", path, field, token)
		}
		if seen[token] {
			return fmt.Errorf("command_override %q %s repeats token %q", path, field, token)
		}
		seen[token] = true
	}
	return nil
}

func cloneParamConcepts(src ParamConcepts) ParamConcepts {
	dst := ParamConcepts{Version: src.Version}
	if src.Morph != nil {
		dst.Morph = make(map[string]ParamMorphRule, len(src.Morph))
		for k, v := range src.Morph {
			dst.Morph[k] = v
		}
	}
	if src.Concepts != nil {
		dst.Concepts = make([]Concept, 0, len(src.Concepts))
		for _, c := range src.Concepts {
			dst.Concepts = append(dst.Concepts, cloneConcept(c))
		}
	}
	if src.ByConcept != nil {
		dst.ByConcept = make(map[string]Concept, len(src.ByConcept))
		for k, v := range src.ByConcept {
			dst.ByConcept[k] = cloneConcept(v)
		}
	}
	if src.Overrides != nil {
		dst.Overrides = make([]CommandOverride, 0, len(src.Overrides))
		for _, o := range src.Overrides {
			dst.Overrides = append(dst.Overrides, cloneCommandOverride(o))
		}
	}
	if src.Fixture != nil {
		dst.Fixture = append([]ParamFixtureCase(nil), src.Fixture...)
	}
	return dst
}

func cloneConcept(c Concept) Concept {
	c.Members = append([]string(nil), c.Members...)
	c.Excludes = append([]string(nil), c.Excludes...)
	return c
}

func cloneCommandOverride(o CommandOverride) CommandOverride {
	o.Bind = cloneStringMap(o.Bind)
	o.ScopedAliases = cloneStringMap(o.ScopedAliases)
	o.Block = append([]string(nil), o.Block...)
	o.Ambiguous = append([]string(nil), o.Ambiguous...)
	return o
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

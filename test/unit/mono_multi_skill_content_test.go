// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
//
// Mono↔multi skill content QA (G1–G4). Spec: docs/skill-mono-multi-qa.md
// Contract: skills/content-qa/mono-multi-coverage.yaml

package unit_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type monoMultiQAContract struct {
	SchemaVersion int `yaml:"schema_version"`
	Coverage      []struct {
		Mono       string   `yaml:"mono"`
		MultiSkill string   `yaml:"multi_skill"`
		MultiRefs  []string `yaml:"multi_refs"`
	} `yaml:"coverage"`
	OmitCoverage []struct {
		Mono        string `yaml:"mono"`
		Disposition string `yaml:"disposition"`
		Via         string `yaml:"via"`
		Reason      string `yaml:"reason"`
	} `yaml:"omit_coverage"`
	GlobalProtocols []struct {
		ID        string `yaml:"id"`
		MultiPath string `yaml:"multi_path"`
	} `yaml:"global_protocols"`
	OmitGlobal []struct {
		ID            string `yaml:"id"`
		Disposition   string `yaml:"disposition"`
		Reason        string `yaml:"reason"`
		ExpectedMulti string `yaml:"expected_multi"`
	} `yaml:"omit_global"`
	PairedFiles []struct {
		Mono  string `yaml:"mono"`
		Multi string `yaml:"multi"`
		Mode  string `yaml:"mode"`
	} `yaml:"paired_files"`
	PairedTrees []struct {
		Mono  string `yaml:"mono"`
		Multi string `yaml:"multi"`
		Mode  string `yaml:"mode"`
	} `yaml:"paired_trees"`
	OrphanScriptsAllowlist []struct {
		Path        string `yaml:"path"`
		Disposition string `yaml:"disposition"`
		Reason      string `yaml:"reason"`
	} `yaml:"orphan_scripts_allowlist"`
	SkillsWithoutReferencesAllowlist []string `yaml:"skills_without_references_allowlist"`
}

func loadMonoMultiQAContract(t *testing.T) (string, monoMultiQAContract) {
	t.Helper()
	root := repoRoot(t)
	path := filepath.Join(root, "skills", "content-qa", "mono-multi-coverage.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c monoMultiQAContract
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	if c.SchemaVersion != 1 {
		t.Fatalf("unsupported schema_version %d", c.SchemaVersion)
	}
	return root, c
}

func TestMonoMultiSkillContentG1Shape(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")
	entries, err := os.ReadDir(multiRoot)
	if err != nil {
		t.Fatal(err)
	}
	foundShared := false
	noRefs := map[string]bool{}
	for _, name := range c.SkillsWithoutReferencesAllowlist {
		noRefs[name] = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("G1: unexpected non-directory %s under skills/multi", e.Name())
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "dingtalk-") {
			t.Errorf("G1: invalid skill directory name %q (want dingtalk-*)", name)
		}
		if name == "dingtalk-shared" {
			foundShared = true
		}
		skillMD := filepath.Join(multiRoot, name, "SKILL.md")
		if _, err := os.Stat(skillMD); err != nil {
			t.Errorf("G1: missing SKILL.md for %s: %v", name, err)
		}
		refs := filepath.Join(multiRoot, name, "references")
		if _, err := os.Stat(refs); err != nil && !noRefs[name] {
			t.Errorf("G1: missing references/ for %s (add dir or skills_without_references_allowlist)", name)
		}
		if noRefs[name] {
			if _, err := os.Stat(refs); err == nil {
				t.Errorf("G1: %s is on skills_without_references_allowlist but references/ exists; remove allowlist entry", name)
			}
		}
	}
	if !foundShared {
		t.Error("G1: skills/multi must contain dingtalk-shared")
	}
}

func TestMonoMultiSkillContentG2Frontmatter(t *testing.T) {
	root, _ := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")
	entries, err := os.ReadDir(multiRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(multiRoot, name, "SKILL.md")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		fm, err := parseSkillFrontmatter(body)
		if err != nil {
			t.Errorf("G2: %s: %v", name, err)
			continue
		}
		if fm.Name != name {
			t.Errorf("G2: %s: frontmatter name %q != directory", name, fm.Name)
		}
		if strings.TrimSpace(fm.Description) == "" {
			t.Errorf("G2: %s: empty description", name)
		}
		category := fm.Metadata.Category
		if category == "" {
			t.Errorf("G2: %s: missing metadata.category", name)
		} else if category != "product" && category != "shared" {
			t.Errorf("G2: %s: metadata.category %q want product|shared", name, category)
		}
		wantCat := "product"
		if name == "dingtalk-shared" {
			wantCat = "shared"
		}
		if category != "" && category != wantCat {
			t.Errorf("G2: %s: metadata.category %q want %q", name, category, wantCat)
		}
		bins := fm.Metadata.Requires.Bins
		hasDWS := false
		for _, b := range bins {
			if b == "dws" {
				hasDWS = true
				break
			}
		}
		if !hasDWS {
			t.Errorf("G2: %s: metadata.requires.bins must include dws", name)
		}
	}
}

func TestMonoMultiSkillContentG3Coverage(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	products := filepath.Join(root, "skills", "mono", "references", "products")
	stems, err := monoProductStems(products)
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, row := range c.Coverage {
		if row.Mono == "" || row.MultiSkill == "" {
			t.Fatalf("G3: coverage row missing mono/multi_skill: %+v", row)
		}
		if covered[row.Mono] {
			t.Errorf("G3: duplicate coverage for mono %q", row.Mono)
		}
		covered[row.Mono] = true
		skillDir := filepath.Join(root, "skills", "multi", row.MultiSkill)
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			t.Errorf("G3: coverage %s → %s missing SKILL.md: %v", row.Mono, row.MultiSkill, err)
		}
		for _, rel := range row.MultiRefs {
			p := filepath.Join(skillDir, filepath.FromSlash(rel))
			if _, err := os.Stat(p); err != nil {
				t.Errorf("G3: coverage %s → %s missing %s: %v", row.Mono, row.MultiSkill, rel, err)
			}
		}
	}
	omitted := map[string]bool{}
	for _, row := range c.OmitCoverage {
		if row.Mono == "" || strings.TrimSpace(row.Reason) == "" || strings.TrimSpace(row.Disposition) == "" {
			t.Fatalf("G3: omit_coverage requires mono, disposition, reason: %+v", row)
		}
		omitted[row.Mono] = true
	}
	for stem := range stems {
		if covered[stem] || omitted[stem] {
			continue
		}
		t.Errorf("G3: mono product stem %q not in coverage or omit_coverage", stem)
	}
	for stem := range covered {
		if !stems[stem] {
			t.Errorf("G3: coverage mono %q has no matching skills/mono/references/products entry", stem)
		}
	}
	for stem := range omitted {
		if !stems[stem] {
			t.Errorf("G3: omit_coverage mono %q has no matching products entry", stem)
		}
	}
}

func TestMonoMultiSkillContentG4Drift(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")

	for _, g := range c.GlobalProtocols {
		p := filepath.Join(multiRoot, filepath.FromSlash(g.MultiPath))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("G4: global protocol %s missing at %s: %v", g.ID, g.MultiPath, err)
		}
	}
	for _, o := range c.OmitGlobal {
		if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.Disposition) == "" || strings.TrimSpace(o.Reason) == "" {
			t.Fatalf("G4: omit_global requires id, disposition, reason: %+v", o)
		}
	}

	for _, pair := range c.PairedFiles {
		monoPath := filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono))
		multiPath := filepath.Join(multiRoot, filepath.FromSlash(pair.Multi))
		monoData, err := os.ReadFile(monoPath)
		if err != nil {
			t.Errorf("G4: paired mono %s: %v", pair.Mono, err)
			continue
		}
		multiData, err := os.ReadFile(multiPath)
		if err != nil {
			t.Errorf("G4: paired multi %s: %v", pair.Multi, err)
			continue
		}
		mode := pair.Mode
		if mode == "" {
			mode = "identical"
		}
		switch mode {
		case "identical":
			if !bytes.Equal(monoData, multiData) {
				t.Errorf("G4: paired files differ: mono %s vs multi %s", pair.Mono, pair.Multi)
			}
		case "shared-link-normalized":
			monoLink := []byte("../url-patterns.md")
			multiLink := []byte("../../dingtalk-shared/references/url-patterns.md")
			if bytes.Count(monoData, monoLink) != 1 || bytes.Count(multiData, multiLink) != 1 {
				t.Errorf("G4: normalized shared-link pair must contain exactly one layout-specific URL-pattern link: mono %s vs multi %s", pair.Mono, pair.Multi)
				continue
			}
			marker := []byte("__DWS_SHARED_URL_PATTERNS__")
			monoNormalized := bytes.Replace(monoData, monoLink, marker, 1)
			multiNormalized := bytes.Replace(multiData, multiLink, marker, 1)
			if !bytes.Equal(monoNormalized, multiNormalized) {
				t.Errorf("G4: paired files differ after shared-link normalization: mono %s vs multi %s", pair.Mono, pair.Multi)
			}
		default:
			t.Errorf("G4: unknown paired mode %q", mode)
		}
	}

	for _, pair := range c.PairedTrees {
		if pair.Mono == "" || pair.Multi == "" {
			t.Fatalf("G4: paired tree requires mono and multi paths: %+v", pair)
		}
		mode := pair.Mode
		if mode == "" {
			mode = "identical"
		}
		if mode != "identical" {
			t.Errorf("G4: unknown paired tree mode %q", mode)
			continue
		}

		monoDir := filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono))
		multiDir := filepath.Join(multiRoot, filepath.FromSlash(pair.Multi))
		monoFiles, err := readRelativeFileTree(monoDir)
		if err != nil {
			t.Errorf("G4: paired mono tree %s: %v", pair.Mono, err)
			continue
		}
		multiFiles, err := readRelativeFileTree(multiDir)
		if err != nil {
			t.Errorf("G4: paired multi tree %s: %v", pair.Multi, err)
			continue
		}
		for rel, monoData := range monoFiles {
			multiData, ok := multiFiles[rel]
			if !ok {
				t.Errorf("G4: paired multi tree %s missing %s from mono %s", pair.Multi, rel, pair.Mono)
				continue
			}
			if !bytes.Equal(monoData, multiData) {
				t.Errorf("G4: paired tree file differs: mono %s/%s vs multi %s/%s", pair.Mono, rel, pair.Multi, rel)
			}
		}
		for rel := range multiFiles {
			if _, ok := monoFiles[rel]; !ok {
				t.Errorf("G4: paired mono tree %s missing %s from multi %s", pair.Mono, rel, pair.Multi)
			}
		}
	}

	allow := map[string]bool{}
	for _, row := range c.OrphanScriptsAllowlist {
		if row.Path == "" || row.Disposition == "" || row.Reason == "" {
			t.Fatalf("G4: orphan allowlist requires path, disposition, reason: %+v", row)
		}
		allow[filepath.ToSlash(row.Path)] = true
	}
	var orphans []string
	entries, err := os.ReadDir(multiRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skill := e.Name()
		scriptsDir := filepath.Join(multiRoot, skill, "scripts")
		if st, err := os.Stat(scriptsDir); err != nil || !st.IsDir() {
			continue
		}
		blob := skillMarkdownBlob(t, filepath.Join(multiRoot, skill))
		_ = filepath.WalkDir(scriptsDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			rel, err := filepath.Rel(multiRoot, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			base := filepath.Base(path)
			if strings.Contains(blob, base) || strings.Contains(blob, relSlash) {
				return nil
			}
			if allow[relSlash] {
				return nil
			}
			orphans = append(orphans, relSlash)
			return nil
		})
	}
	for _, o := range orphans {
		t.Errorf("G4: orphan script %s (reference from skill markdown or add orphan_scripts_allowlist)", o)
	}
	for path := range allow {
		full := filepath.Join(multiRoot, filepath.FromSlash(path))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("G4: orphan allowlist path missing on disk: %s", path)
		}
	}
}

func readRelativeFileTree(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	return files, err
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		Category string `yaml:"category"`
		Requires struct {
			Bins []string `yaml:"bins"`
		} `yaml:"requires"`
	} `yaml:"metadata"`
}

func parseSkillFrontmatter(body []byte) (skillFrontmatter, error) {
	var out skillFrontmatter
	s := string(body)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return out, fmt.Errorf("missing YAML frontmatter opener")
	}
	rest := s[3:]
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return out, fmt.Errorf("missing YAML frontmatter closer")
	}
	block := rest[:end]
	if err := yaml.Unmarshal([]byte(block), &out); err != nil {
		return out, err
	}
	return out, nil
}

func monoProductStems(productsDir string) (map[string]bool, error) {
	entries, err := os.ReadDir(productsDir)
	if err != nil {
		return nil, err
	}
	stems := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			stems[name] = true
			continue
		}
		if strings.HasSuffix(name, ".md") {
			stems[strings.TrimSuffix(name, ".md")] = true
		}
	}
	return stems, nil
}

func skillMarkdownBlob(t *testing.T, skillDir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}

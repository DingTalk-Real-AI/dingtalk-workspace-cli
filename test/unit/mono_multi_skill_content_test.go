// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
//
// Mono↔multi skill content QA (G1–G5). Spec: docs/skill-mono-multi-qa.md
// Contract: skills/content-qa/mono-multi-coverage.yaml

package unit_test

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
		Mono              string                      `yaml:"mono"`
		Multi             string                      `yaml:"multi"`
		Mode              string                      `yaml:"mode"`
		LinkSubstitutions []monoMultiLinkSubstitution `yaml:"link_substitutions"`
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
	RetiredScripts []struct {
		Path   string `yaml:"path"`
		Reason string `yaml:"reason"`
	} `yaml:"retired_scripts"`
	SkillsWithoutReferencesAllowlist []string `yaml:"skills_without_references_allowlist"`
}

type monoMultiLinkSubstitution struct {
	Mono  string `yaml:"mono"`
	Multi string `yaml:"multi"`
}

type markdownLink struct {
	Line   int
	Target string
}

type markdownLinkIssue struct {
	Link     markdownLink
	Resolved string
	Err      error
}

var inlineMarkdownLinkPattern = regexp.MustCompile(`!?\[[^\]\n]*\]\(\s*(<[^>\n]+>|[^)\s\n]+)(?:\s+[^)\n]+)?\)`)

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
		if err := comparePairedFileContent(monoData, multiData, mode, pair.LinkSubstitutions); err != nil {
			t.Errorf("G4: paired file mono %s vs multi %s: %v", pair.Mono, pair.Multi, err)
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
			if (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")) && strings.HasSuffix(base, ".py") {
				return nil
			}
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

	retired := map[string]bool{}
	for _, row := range c.RetiredScripts {
		path := filepath.ToSlash(filepath.Clean(row.Path))
		if row.Path == "" || strings.TrimSpace(row.Reason) == "" {
			t.Fatalf("G4: retired_scripts requires path and reason: %+v", row)
		}
		if filepath.IsAbs(row.Path) || path == "." || path == ".." || strings.HasPrefix(path, "../") {
			t.Fatalf("G4: invalid retired script path %q", row.Path)
		}
		if retired[path] {
			t.Fatalf("G4: duplicate retired script path %q", path)
		}
		retired[path] = true
		full := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Stat(full); err == nil {
			t.Errorf("G4: retired script was republished: %s", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("G4: inspect retired script %s: %v", path, err)
		}
	}
}

func TestMonoMultiSkillContentG5RelativeLinks(t *testing.T) {
	root, c := loadMonoMultiQAContract(t)
	multiRoot := filepath.Join(root, "skills", "multi")
	sources := map[string]bool{}

	for _, pair := range c.PairedFiles {
		if pair.Mono == "" || pair.Multi == "" {
			t.Fatalf("G5: paired file requires mono and multi paths: %+v", pair)
		}
		sources[filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono))] = true
		sources[filepath.Join(multiRoot, filepath.FromSlash(pair.Multi))] = true
	}
	for _, pair := range c.PairedTrees {
		if pair.Mono == "" || pair.Multi == "" {
			t.Fatalf("G5: paired tree requires mono and multi paths: %+v", pair)
		}
		trees := []string{
			filepath.Join(root, "skills", "mono", filepath.FromSlash(pair.Mono)),
			filepath.Join(multiRoot, filepath.FromSlash(pair.Multi)),
		}
		for _, tree := range trees {
			files, err := readRelativeFileTree(tree)
			if err != nil {
				t.Errorf("G5: read paired Markdown tree %s: %v", tree, err)
				continue
			}
			for rel := range files {
				sources[filepath.Join(tree, filepath.FromSlash(rel))] = true
			}
		}
	}

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, source := range paths {
		if !strings.EqualFold(filepath.Ext(source), ".md") {
			continue
		}
		data, err := os.ReadFile(source)
		if err != nil {
			t.Errorf("G5: read %s: %v", source, err)
			continue
		}
		rel, relErr := filepath.Rel(root, source)
		if relErr != nil {
			rel = source
		}
		for _, issue := range unresolvedRelativeMarkdownLinks(root, source, data) {
			resolved := issue.Resolved
			if resolved != "" {
				if resolvedRel, err := filepath.Rel(root, resolved); err == nil {
					resolved = filepath.ToSlash(resolvedRel)
				}
			}
			t.Errorf("G5: %s:%d relative link %q resolves to %q: %v", filepath.ToSlash(rel), issue.Link.Line, issue.Link.Target, resolved, issue.Err)
		}
	}
}

func TestMonoMultiSkillContentLinkNormalization(t *testing.T) {
	rules := []monoMultiLinkSubstitution{
		{Mono: "../url-patterns.md", Multi: "../../dingtalk-shared/references/url-patterns.md"},
		{Mono: "../intent-guide.md", Multi: "sheet-intent-guide.md"},
	}
	mono := []byte("url=../url-patterns.md\nintent=../intent-guide.md\n")
	multi := []byte("url=../../dingtalk-shared/references/url-patterns.md\nintent=sheet-intent-guide.md\n")
	if err := comparePairedFileContent(mono, multi, "link-normalized", rules); err != nil {
		t.Fatalf("valid substitutions rejected: %v", err)
	}
	if err := comparePairedFileContent(mono, append(multi, []byte("drift\n")...), "link-normalized", rules); err == nil {
		t.Fatal("unknown content drift must not be normalized away")
	}
	if err := comparePairedFileContent(mono, multi, "identical", rules); err == nil {
		t.Fatal("identical mode must reject link substitutions")
	}
	if err := comparePairedFileContent(mono, multi, "unknown", nil); err == nil {
		t.Fatal("unknown paired-file mode must fail closed")
	}
}

func TestMonoMultiSkillContentLinkNormalizationRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name  string
		mono  string
		multi string
		rules []monoMultiLinkSubstitution
	}{
		{name: "no rules", mono: "m1", multi: "x1"},
		{name: "empty mono", mono: "m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Multi: "x1"}}},
		{name: "empty multi", mono: "m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m1"}}},
		{name: "same sides", mono: "m1", multi: "m1", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "m1"}}},
		{name: "duplicate mono", mono: "m1", multi: "x1 x2", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "x1"}, {Mono: "m1", Multi: "x2"}}},
		{name: "duplicate multi", mono: "m1 m2", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "x1"}, {Mono: "m2", Multi: "x1"}}},
		{name: "overlapping mono", mono: "m1 m", multi: "x1 x2", rules: []monoMultiLinkSubstitution{{Mono: "m", Multi: "x1"}, {Mono: "m1", Multi: "x2"}}},
		{name: "missing occurrence", mono: "m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m2", Multi: "x1"}}},
		{name: "multiple occurrences", mono: "m1 m1", multi: "x1", rules: []monoMultiLinkSubstitution{{Mono: "m1", Multi: "x1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := normalizePairedFileLinks([]byte(tt.mono), []byte(tt.multi), tt.rules); err == nil {
				t.Fatal("invalid substitution rules must fail closed")
			}
		})
	}
}

func TestMonoMultiSkillContentRelativeLinkScanner(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "sheet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "topic.md"), []byte("# Topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte("[topic](./topic.md#section)\n[directory](./sheet/)\n[external](https://example.com/a)\n[email](mailto:test@example.com)\n[anchor](#section)\n`[inline](missing-inline.md)`\n```text\n[fenced](missing-fenced.md)\n```\n[missing](missing.md)\n[escape](../../escape.md)\n")
	issues := unresolvedRelativeMarkdownLinks(root, filepath.Join(docs, "source.md"), body)
	if len(issues) != 2 {
		t.Fatalf("got %d unresolved links, want 2: %+v", len(issues), issues)
	}
	if issues[0].Link.Target != "missing.md" || issues[0].Link.Line != 10 {
		t.Fatalf("unexpected unresolved link: %+v", issues[0])
	}
	if issues[1].Link.Target != "../../escape.md" || issues[1].Link.Line != 11 || !strings.Contains(issues[1].Err.Error(), "escapes repository") {
		t.Fatalf("unexpected escaping link: %+v", issues[1])
	}
}

func TestMonoMultiSkillContentReadRelativeFileTreeFiltersNonMarkdown(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"nested", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"guide.md":          "visible",
		"nested/topic.md":   "nested",
		"notes.txt":         "ignored",
		".DS_Store":         "ignored",
		".draft.md":         "ignored",
		".hidden/topic.md":  "ignored",
		"nested/README.MD":  "accepted",
		"nested/.hidden.md": "ignored",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readRelativeFileTree(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"guide.md", "nested/README.MD", "nested/topic.md"}
	gotNames := make([]string, 0, len(got))
	for rel := range got {
		gotNames = append(gotNames, rel)
	}
	sort.Strings(gotNames)
	if strings.Join(gotNames, "\n") != strings.Join(want, "\n") {
		t.Fatalf("visible Markdown tree = %v, want %v", gotNames, want)
	}
}

func comparePairedFileContent(monoData, multiData []byte, mode string, substitutions []monoMultiLinkSubstitution) error {
	switch mode {
	case "identical":
		if len(substitutions) != 0 {
			return fmt.Errorf("identical mode does not allow link_substitutions")
		}
		if !bytes.Equal(monoData, multiData) {
			return fmt.Errorf("contents differ")
		}
		return nil
	case "link-normalized":
		monoNormalized, multiNormalized, err := normalizePairedFileLinks(monoData, multiData, substitutions)
		if err != nil {
			return err
		}
		if !bytes.Equal(monoNormalized, multiNormalized) {
			return fmt.Errorf("contents differ after declared link substitutions")
		}
		return nil
	default:
		return fmt.Errorf("unknown paired mode %q", mode)
	}
}

func normalizePairedFileLinks(monoData, multiData []byte, substitutions []monoMultiLinkSubstitution) ([]byte, []byte, error) {
	if len(substitutions) == 0 {
		return nil, nil, fmt.Errorf("link-normalized mode requires link_substitutions")
	}
	for i, substitution := range substitutions {
		if substitution.Mono == "" || substitution.Multi == "" {
			return nil, nil, fmt.Errorf("link_substitutions[%d] requires non-empty mono and multi values", i)
		}
		if substitution.Mono == substitution.Multi {
			return nil, nil, fmt.Errorf("link_substitutions[%d] must describe a layout difference", i)
		}
		for j := 0; j < i; j++ {
			previous := substitutions[j]
			if strings.Contains(substitution.Mono, previous.Mono) || strings.Contains(previous.Mono, substitution.Mono) {
				return nil, nil, fmt.Errorf("link_substitutions[%d].mono overlaps link_substitutions[%d].mono", i, j)
			}
			if strings.Contains(substitution.Multi, previous.Multi) || strings.Contains(previous.Multi, substitution.Multi) {
				return nil, nil, fmt.Errorf("link_substitutions[%d].multi overlaps link_substitutions[%d].multi", i, j)
			}
		}
		if count := bytes.Count(monoData, []byte(substitution.Mono)); count != 1 {
			return nil, nil, fmt.Errorf("link_substitutions[%d].mono occurs %d times, want exactly 1", i, count)
		}
		if count := bytes.Count(multiData, []byte(substitution.Multi)); count != 1 {
			return nil, nil, fmt.Errorf("link_substitutions[%d].multi occurs %d times, want exactly 1", i, count)
		}
	}

	monoNormalized := bytes.Clone(monoData)
	multiNormalized := bytes.Clone(multiData)
	for i, substitution := range substitutions {
		marker := []byte(fmt.Sprintf("\x00DWS_LINK_SUBSTITUTION_%d\x00", i))
		if bytes.Contains(monoNormalized, marker) || bytes.Contains(multiNormalized, marker) {
			return nil, nil, fmt.Errorf("link_substitutions[%d] normalization marker already exists in content", i)
		}
		monoNormalized = bytes.Replace(monoNormalized, []byte(substitution.Mono), marker, 1)
		multiNormalized = bytes.Replace(multiNormalized, []byte(substitution.Multi), marker, 1)
	}
	return monoNormalized, multiNormalized, nil
}

func unresolvedRelativeMarkdownLinks(repoRoot, source string, body []byte) []markdownLinkIssue {
	var issues []markdownLinkIssue
	for _, link := range extractMarkdownLinks(body) {
		targetPath, check, err := relativeMarkdownTargetPath(link.Target)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Err: err})
			continue
		}
		if !check {
			continue
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(targetPath)))
		rel, err := filepath.Rel(repoRoot, resolved)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: err})
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: fmt.Errorf("target escapes repository")})
			continue
		}
		if _, err := os.Stat(resolved); err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: err})
			continue
		}
		realRoot, err := filepath.EvalSymlinks(repoRoot)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: fmt.Errorf("resolve repository path: %w", err)})
			continue
		}
		realTarget, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: fmt.Errorf("resolve target path: %w", err)})
			continue
		}
		realRel, err := filepath.Rel(realRoot, realTarget)
		if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(os.PathSeparator)) {
			if err == nil {
				err = fmt.Errorf("target escapes repository through a symbolic link")
			}
			issues = append(issues, markdownLinkIssue{Link: link, Resolved: resolved, Err: err})
		}
	}
	return issues
}

func extractMarkdownLinks(body []byte) []markdownLink {
	var links []markdownLink
	var fence byte
	for i, line := range strings.Split(string(body), "\n") {
		if marker := markdownFenceMarker(line); marker != 0 {
			if fence == 0 {
				fence = marker
			} else if fence == marker {
				fence = 0
			}
			continue
		}
		if fence != 0 {
			continue
		}
		masked := maskInlineMarkdownCode(line)
		for _, match := range inlineMarkdownLinkPattern.FindAllStringSubmatch(masked, -1) {
			target := match[1]
			if strings.HasPrefix(target, "<") && strings.HasSuffix(target, ">") {
				target = strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">")
			}
			links = append(links, markdownLink{Line: i + 1, Target: target})
		}
	}
	return links
}

func markdownFenceMarker(line string) byte {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "```") {
		return '`'
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return '~'
	}
	return 0
}

func maskInlineMarkdownCode(line string) string {
	masked := []byte(line)
	for i := 0; i < len(masked); {
		if masked[i] != '`' {
			i++
			continue
		}
		endOfOpener := i
		for endOfOpener < len(masked) && masked[endOfOpener] == '`' {
			endOfOpener++
		}
		marker := masked[i:endOfOpener]
		closingOffset := bytes.Index(masked[endOfOpener:], marker)
		if closingOffset < 0 {
			i = endOfOpener
			continue
		}
		end := endOfOpener + closingOffset + len(marker)
		for j := i; j < end; j++ {
			masked[j] = ' '
		}
		i = end
	}
	return string(masked)
}

func relativeMarkdownTargetPath(target string) (string, bool, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", false, fmt.Errorf("parse target: %w", err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || strings.HasPrefix(target, "//") || strings.HasPrefix(parsed.Path, "/") {
		return "", false, nil
	}
	if parsed.Path == "" {
		return "", false, nil
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false, fmt.Errorf("decode target path: %w", err)
	}
	return path, true, nil
}

func readRelativeFileTree(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
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

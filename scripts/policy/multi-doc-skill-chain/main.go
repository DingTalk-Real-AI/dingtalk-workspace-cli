// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command multi-doc-skill-chain verifies that reviewed high-frequency doc
// intents keep one default route across Skill references and Agent selection.
//
// It reads the runtime CommandRegistry (not source text) so a command that is
// renamed at runtime — e.g. canonicalizeShareDoc turning +share-doc into
// +share — is judged by its real canonical identity.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

const manifestRelativePath = "scripts/policy/multi-doc-skill-chain/testdata/intent_routes.json"

var (
	intentIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	reasonCodePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	intentMarker       = regexp.MustCompile(`<!--\s*dws-intent:\s*([a-z0-9][a-z0-9._-]*)\s*-->`)
	markdownLink       = regexp.MustCompile(`\]\(([^)#\s]+\.md)(?:#[^)]*)?\)`)
	mainGetwd          = os.Getwd
	mainExit           = os.Exit
	markerOpen         = os.Open
	repositoryRelative = filepath.Rel
	buildEffective     = cli.BuildEffectiveCommandRegistry
	bindEffective      = cli.BindEffectiveCommandRegistry
)

type routeManifest struct {
	Version                  int               `json:"version"`
	MarkerRoots              []string          `json:"marker_roots"`
	OrphanAllowlist          []string          `json:"orphan_allowlist,omitempty"`
	ProtectedReferenceRoots  []string          `json:"protected_reference_roots"`
	ProtectedReferenceSHA256 map[string]string `json:"protected_reference_sha256"`
	Intents                  []intentRoute     `json:"intents"`
}

type intentRoute struct {
	ID                    string          `json:"id"`
	PreferredTool         string          `json:"preferred_tool"`
	AllowedFallbacks      []routeFallback `json:"allowed_fallbacks,omitempty"`
	ForbiddenDefaultTools []string        `json:"forbidden_default_tools,omitempty"`
	References            []string        `json:"references"`
}

type routeFallback struct {
	Tool       string `json:"tool"`
	ReasonCode string `json:"reason_code"`
}

type toolFact struct {
	Canonical    string
	PrimaryPath  string
	Confirmation string
	UseWhen      []string
	AvoidWhen    []string
	HasMeta      bool
}

func main() {
	rootPath, err := mainGetwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		mainExit(2)
		return
	}
	mainExit(run(rootPath, app.NewRootCommand(), os.Stdout, os.Stderr))
}

func run(rootPath string, root *cobra.Command, stdout, stderr io.Writer) int {
	manifest, err := loadManifest(filepath.Join(rootPath, manifestRelativePath))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	protectedFailures := validateProtectedReferences(rootPath, manifest)
	sort.Strings(protectedFailures)
	if len(protectedFailures) > 0 {
		fmt.Fprintf(stderr, "protected doc Reference check failed (%d problems):\n", len(protectedFailures))
		for _, failure := range protectedFailures {
			fmt.Fprintf(stderr, "  - %s\n", failure)
		}
		return 1
	}
	effective, err := buildEffective(root)
	if err != nil {
		fmt.Fprintf(stderr, "build effective CommandRegistry: %v\n", err)
		return 2
	}
	bound, err := bindEffective(root, effective)
	if err != nil {
		fmt.Fprintf(stderr, "bind effective CommandRegistry: %v\n", err)
		return 2
	}

	tools := make(map[string]toolFact, len(bound.ByCanonical))
	for canonical, item := range bound.ByCanonical {
		meta, ok := cli.ResolveMeta(item.PrimaryCLIPath)
		tools[canonical] = toolFact{
			Canonical:    canonical,
			PrimaryPath:  item.PrimaryCLIPath,
			Confirmation: meta.Safety.Confirmation,
			UseWhen:      append([]string(nil), meta.Selection.UseWhen...),
			AvoidWhen:    append([]string(nil), meta.Selection.AvoidWhen...),
			HasMeta:      ok,
		}
	}

	failures := validateManifest(rootPath, manifest, tools)
	sort.Strings(failures)
	if len(failures) > 0 {
		fmt.Fprintf(stderr, "multi doc Skill chain check failed (%d problems):\n", len(failures))
		for _, failure := range failures {
			fmt.Fprintf(stderr, "  - %s\n", failure)
		}
		return 1
	}
	fmt.Fprintf(stdout, "multi doc Skill chain check: ok (%d intents)\n", len(manifest.Intents))
	return 0
}

func loadManifest(path string) (routeManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return routeManifest{}, fmt.Errorf("read doc intent route manifest: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest routeManifest
	if err := decoder.Decode(&manifest); err != nil {
		return routeManifest{}, fmt.Errorf("decode doc intent route manifest: %w", err)
	}
	return manifest, nil
}

func validateProtectedReferences(rootPath string, manifest routeManifest) []string {
	var failures []string
	if len(manifest.ProtectedReferenceRoots) == 0 {
		return append(failures, "protected_reference_roots must not be empty")
	}
	if len(manifest.ProtectedReferenceSHA256) == 0 {
		return append(failures, "protected_reference_sha256 must not be empty")
	}

	rootPrefixes := make([]string, 0, len(manifest.ProtectedReferenceRoots))
	for _, root := range manifest.ProtectedReferenceRoots {
		root = filepath.ToSlash(strings.TrimSpace(root))
		if !safeRepositoryPath(root) {
			failures = append(failures, fmt.Sprintf("invalid protected reference root %q", root))
			continue
		}
		rootPrefixes = append(rootPrefixes, strings.TrimSuffix(root, "/")+"/")
		absoluteRoot := filepath.Join(rootPath, filepath.FromSlash(root))
		err := filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, relErr := repositoryRelative(rootPath, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			if _, reviewed := manifest.ProtectedReferenceSHA256[relative]; !reviewed {
				failures = append(failures, fmt.Sprintf("unreviewed file in protected reference root: %s", relative))
			}
			return nil
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("scan protected reference root %s: %v", root, err))
		}
	}

	for relative, expected := range manifest.ProtectedReferenceSHA256 {
		relative = filepath.ToSlash(strings.TrimSpace(relative))
		if !safeRepositoryPath(relative) {
			failures = append(failures, fmt.Sprintf("invalid protected reference path %q", relative))
			continue
		}
		insideRoot := false
		for _, prefix := range rootPrefixes {
			if strings.HasPrefix(relative, prefix) {
				insideRoot = true
				break
			}
		}
		if !insideRoot {
			failures = append(failures, fmt.Sprintf("protected hash path is outside declared roots: %s", relative))
			continue
		}
		if !sha256Pattern.MatchString(expected) {
			failures = append(failures, fmt.Sprintf("protected reference %s has invalid sha256 %q", relative, expected))
			continue
		}
		data, err := os.ReadFile(filepath.Join(rootPath, filepath.FromSlash(relative)))
		if err != nil {
			failures = append(failures, fmt.Sprintf("read protected reference %s: %v", relative, err))
			continue
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(data))
		if actual != expected {
			failures = append(failures, fmt.Sprintf("protected reference %s sha256 = %s, want %s", relative, actual, expected))
		}
	}
	return failures
}

func validateManifest(rootPath string, manifest routeManifest, tools map[string]toolFact) []string {
	var failures []string
	if manifest.Version != 1 {
		failures = append(failures, fmt.Sprintf("manifest version = %d, want 1", manifest.Version))
	}
	if len(manifest.MarkerRoots) == 0 {
		failures = append(failures, "manifest marker_roots must not be empty")
	}
	if len(manifest.Intents) == 0 {
		failures = append(failures, "manifest intents must not be empty")
	}

	intentByID := make(map[string]intentRoute, len(manifest.Intents))
	for _, route := range manifest.Intents {
		if !intentIDPattern.MatchString(route.ID) {
			failures = append(failures, fmt.Sprintf("intent id %q is invalid", route.ID))
			continue
		}
		if _, exists := intentByID[route.ID]; exists {
			failures = append(failures, fmt.Sprintf("duplicate intent id %q", route.ID))
			continue
		}
		intentByID[route.ID] = route
		preferred, ok := tools[route.PreferredTool]
		if !ok {
			failures = append(failures, fmt.Sprintf("intent %s preferred tool %q is absent from BoundCommandRegistry", route.ID, route.PreferredTool))
			continue
		}
		if !preferred.HasMeta || preferred.Confirmation == "" {
			failures = append(failures, fmt.Sprintf("intent %s preferred tool %q is absent from ResolveMeta delivery", route.ID, route.PreferredTool))
		}
		if len(preferred.UseWhen) == 0 || len(preferred.AvoidWhen) == 0 {
			failures = append(failures, fmt.Sprintf("intent %s preferred tool %q needs non-empty use_when and avoid_when", route.ID, route.PreferredTool))
		}

		seenFallbacks := map[string]bool{}
		for _, fallback := range route.AllowedFallbacks {
			if fallback.Tool == "" || seenFallbacks[fallback.Tool] {
				failures = append(failures, fmt.Sprintf("intent %s has empty or duplicate allowed fallback %q", route.ID, fallback.Tool))
				continue
			}
			seenFallbacks[fallback.Tool] = true
			if !reasonCodePattern.MatchString(fallback.ReasonCode) {
				failures = append(failures, fmt.Sprintf("intent %s fallback %q has invalid reason_code %q", route.ID, fallback.Tool, fallback.ReasonCode))
			}
			fact, exists := tools[fallback.Tool]
			if !exists {
				failures = append(failures, fmt.Sprintf("intent %s fallback tool %q is absent from BoundCommandRegistry", route.ID, fallback.Tool))
				continue
			}
			if !fact.HasMeta || fact.Confirmation == "" {
				failures = append(failures, fmt.Sprintf("intent %s fallback tool %q is absent from ResolveMeta delivery", route.ID, fallback.Tool))
			}
		}

		seenForbidden := map[string]bool{}
		for _, canonical := range route.ForbiddenDefaultTools {
			if canonical == "" || seenForbidden[canonical] {
				failures = append(failures, fmt.Sprintf("intent %s has empty or duplicate forbidden default %q", route.ID, canonical))
				continue
			}
			seenForbidden[canonical] = true
			entry, exists := tools[canonical]
			if !exists {
				failures = append(failures, fmt.Sprintf("intent %s forbidden default tool %q is absent from BoundCommandRegistry", route.ID, canonical))
				continue
			}
			if !entry.HasMeta || entry.Confirmation == "" {
				failures = append(failures, fmt.Sprintf("intent %s forbidden default tool %q is absent from ResolveMeta delivery", route.ID, canonical))
			}
		}

		seenReferences := map[string]bool{}
		for _, reference := range route.References {
			if !safeRepositoryPath(reference) || filepath.Ext(reference) != ".md" {
				failures = append(failures, fmt.Sprintf("intent %s has invalid reference %q", route.ID, reference))
				continue
			}
			if seenReferences[reference] {
				failures = append(failures, fmt.Sprintf("intent %s repeats reference %q", route.ID, reference))
				continue
			}
			seenReferences[reference] = true
			if _, err := os.Stat(filepath.Join(rootPath, filepath.FromSlash(reference))); err != nil {
				failures = append(failures, fmt.Sprintf("intent %s reference %q does not exist", route.ID, reference))
			}
		}
	}

	markerFailures, markers := scanMarkers(rootPath, manifest.MarkerRoots, manifest.OrphanAllowlist, intentByID, tools)
	failures = append(failures, markerFailures...)
	for _, route := range manifest.Intents {
		for _, reference := range route.References {
			key := route.ID + "\x00" + filepath.ToSlash(reference)
			switch markers[key] {
			case 0:
				failures = append(failures, fmt.Sprintf("intent %s reference %s is missing its dws-intent marker", route.ID, reference))
			case 1:
			default:
				failures = append(failures, fmt.Sprintf("intent %s reference %s has %d dws-intent markers, want 1", route.ID, reference, markers[key]))
			}
		}
	}
	return failures
}

func scanMarkers(rootPath string, roots []string, orphanAllowlist []string, intents map[string]intentRoute, tools map[string]toolFact) ([]string, map[string]int) {
	var failures []string
	markers := map[string]int{}
	inbound := map[string]int{}
	var treeFiles []string
	for _, relativeRoot := range roots {
		if !safeRepositoryPath(relativeRoot) {
			failures = append(failures, fmt.Sprintf("invalid marker root %q", relativeRoot))
			continue
		}
		absoluteRoot := filepath.Join(rootPath, filepath.FromSlash(relativeRoot))
		err := filepath.WalkDir(absoluteRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			file, err := markerOpen(path)
			if err != nil {
				return err
			}
			defer file.Close()
			relative, _ := filepath.Rel(rootPath, path)
			relative = filepath.ToSlash(relative)
			treeFiles = append(treeFiles, relative)
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
			lineNumber := 0
			for scanner.Scan() {
				lineNumber++
				line := scanner.Text()
				for _, link := range markdownLink.FindAllStringSubmatch(line, -1) {
					target := link[1]
					if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
						continue
					}
					resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
					if _, statErr := os.Stat(resolved); statErr != nil {
						failures = append(failures, fmt.Sprintf("%s:%d dead reference link %q", relative, lineNumber, target))
						continue
					}
					if resolvedRelative, relErr := filepath.Rel(rootPath, resolved); relErr == nil {
						inbound[filepath.ToSlash(resolvedRelative)]++
					}
				}
				for _, match := range intentMarker.FindAllStringSubmatch(line, -1) {
					id := match[1]
					route, ok := intents[id]
					if !ok {
						failures = append(failures, fmt.Sprintf("%s:%d uses unknown dws-intent %q", relative, lineNumber, id))
						continue
					}
					if !stringSliceContains(route.References, relative) {
						failures = append(failures, fmt.Sprintf("%s:%d uses undeclared dws-intent %q", relative, lineNumber, id))
					}
					markers[id+"\x00"+relative]++
					preferred, exists := tools[route.PreferredTool]
					if !exists {
						continue
					}
					if !containsCLIPath(line, preferred.PrimaryPath) {
						failures = append(failures, fmt.Sprintf("%s:%d intent %s must contain preferred path `dws %s` on the marker line", relative, lineNumber, id, preferred.PrimaryPath))
					}
					for _, canonical := range route.ForbiddenDefaultTools {
						if fact, ok := tools[canonical]; ok && containsCLIPath(line, fact.PrimaryPath) {
							failures = append(failures, fmt.Sprintf("%s:%d intent %s uses forbidden default `dws %s`", relative, lineNumber, id, fact.PrimaryPath))
						}
					}
				}
			}
			return scanner.Err()
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("scan marker root %s: %v", relativeRoot, err))
		}
	}
	for _, relative := range treeFiles {
		if filepath.Base(relative) == "SKILL.md" {
			continue
		}
		if inbound[relative] > 0 || stringSliceContains(orphanAllowlist, relative) {
			continue
		}
		failures = append(failures, fmt.Sprintf("orphan reference %s (no inbound links; add a link or list it in orphan_allowlist)", relative))
	}
	return failures, markers
}

func safeRepositoryPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return path != "" && path != "." && !filepath.IsAbs(path) && path != ".." && !strings.HasPrefix(path, "../") && !strings.Contains(path, "/../")
}

func containsCLIPath(line, path string) bool {
	needle := "dws " + strings.TrimSpace(path)
	index := strings.Index(line, needle)
	if index < 0 {
		return false
	}
	end := index + len(needle)
	if end == len(line) {
		return true
	}
	next := line[end]
	return next == ' ' || next == '`' || next == '<' || next == '|' || next == ',' || next == '/'
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if filepath.ToSlash(value) == target {
			return true
		}
	}
	return false
}

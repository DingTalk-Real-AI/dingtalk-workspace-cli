// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Command multi-im-skill-chain verifies that reviewed high-frequency IM and
// personal-event intents keep one default route across Skill references and
// Agent selection.
package main

import (
	"bufio"
	"bytes"
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
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/spf13/cobra"
)

const manifestRelativePath = "scripts/policy/multi-im-skill-chain/testdata/intent_routes.json"

var (
	intentIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	reasonCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	intentMarker      = regexp.MustCompile(`<!--\s*dws-intent:\s*([a-z0-9][a-z0-9._-]*)\s*-->`)
	mainGetwd         = os.Getwd
	mainExit          = os.Exit
	markerOpen        = os.Open
	buildEffective    = cli.BuildEffectiveCommandRegistry
	bindEffective     = cli.BindEffectiveCommandRegistry
)

type routeManifest struct {
	Version           int           `json:"version"`
	MarkerRoots       []string      `json:"marker_roots"`
	Intents           []intentRoute `json:"intents"`
	RetiredScripts    []string      `json:"retired_scripts"`
	ContractReference string        `json:"contract_reference"`
	HandoffReference  string        `json:"handoff_reference"`
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
		fmt.Fprintf(stderr, "multi IM Skill chain check failed (%d problems):\n", len(failures))
		for _, failure := range failures {
			fmt.Fprintf(stderr, "  - %s\n", failure)
		}
		return 1
	}
	fmt.Fprintf(stdout, "multi IM Skill chain check: ok (%d intents)\n", len(manifest.Intents))
	return 0
}

func loadManifest(path string) (routeManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return routeManifest{}, fmt.Errorf("read IM intent route manifest: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest routeManifest
	if err := decoder.Decode(&manifest); err != nil {
		return routeManifest{}, fmt.Errorf("decode IM intent route manifest: %w", err)
	}
	return manifest, nil
}

func validateManifest(rootPath string, manifest routeManifest, tools map[string]toolFact) []string {
	var failures []string
	if manifest.Version != 3 {
		failures = append(failures, fmt.Sprintf("manifest version = %d, want 3", manifest.Version))
	}
	if len(manifest.MarkerRoots) == 0 {
		failures = append(failures, "manifest marker_roots must not be empty")
	}
	if len(manifest.Intents) == 0 {
		failures = append(failures, "manifest intents must not be empty")
	}
	failures = append(failures, validateRetiredScripts(rootPath, manifest.RetiredScripts)...)
	failures = append(failures, validateTypedContractReference(rootPath, manifest.ContractReference)...)
	failures = append(failures, validateEventHandoffReference(rootPath, manifest.HandoffReference)...)

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
			} else if fact.Confirmation != preferred.Confirmation {
				failures = append(failures, fmt.Sprintf("intent %s fallback %q confirmation %q differs from preferred %q confirmation %q", route.ID, fallback.Tool, fact.Confirmation, route.PreferredTool, preferred.Confirmation))
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
			if len(entry.UseWhen) == 0 || len(entry.AvoidWhen) == 0 {
				failures = append(failures, fmt.Sprintf("intent %s forbidden default tool %q needs non-empty use_when and avoid_when", route.ID, canonical))
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

	markerFailures, markers := scanMarkers(rootPath, manifest.MarkerRoots, intentByID, tools)
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

func validateRetiredScripts(rootPath string, paths []string) []string {
	var failures []string
	if len(paths) == 0 {
		return []string{"manifest retired_scripts must not be empty"}
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if !safeRepositoryPath(path) || filepath.Ext(path) != ".py" {
			failures = append(failures, fmt.Sprintf("invalid retired script path %q", path))
			continue
		}
		if seen[path] {
			failures = append(failures, fmt.Sprintf("duplicate retired script path %q", path))
			continue
		}
		seen[path] = true
		if _, err := os.Stat(filepath.Join(rootPath, filepath.FromSlash(path))); err == nil {
			failures = append(failures, fmt.Sprintf("retired script %s was republished", path))
		} else if !os.IsNotExist(err) {
			failures = append(failures, fmt.Sprintf("inspect retired script %s: %v", path, err))
		}
	}
	return failures
}

func validateTypedContractReference(rootPath, relative string) []string {
	if !safeRepositoryPath(relative) || filepath.Ext(relative) != ".md" {
		return []string{fmt.Sprintf("invalid contract_reference %q", relative)}
	}
	data, err := os.ReadFile(filepath.Join(rootPath, filepath.FromSlash(relative)))
	if err != nil {
		return []string{fmt.Sprintf("read typed contract reference %s: %v", relative, err)}
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	blocks := []struct {
		name     string
		expected string
	}{
		{name: "MESSAGE_RESULT", expected: renderMessageResultContract()},
		{name: "IDENTITY_CAPABILITY", expected: renderIdentityCapabilityContract()},
		{name: "CARD_WORKFLOW", expected: renderCardWorkflowContract()},
		{name: "CAPABILITY_BOUNDARY", expected: renderCapabilityBoundaryContract()},
	}
	var failures []string
	for _, block := range blocks {
		start := "<!-- DWS_" + block.name + "_CONTRACT_START -->"
		end := "<!-- DWS_" + block.name + "_CONTRACT_END -->"
		expected := start + "\n" + block.expected + "\n" + end
		if bytes.Count(data, []byte(start)) != 1 || bytes.Count(data, []byte(end)) != 1 {
			failures = append(failures, fmt.Sprintf("%s must contain exactly one %s contract marker pair", relative, block.name))
			continue
		}
		startAt := bytes.Index(data, []byte(start))
		endAt := bytes.Index(data[startAt:], []byte(end))
		if endAt < 0 {
			failures = append(failures, fmt.Sprintf("%s has malformed %s contract markers", relative, block.name))
			continue
		}
		actual := string(data[startAt : startAt+endAt+len(end)])
		if actual != expected {
			failures = append(failures, fmt.Sprintf("%s %s contract differs from Runtime typed descriptor", relative, block.name))
		}
	}
	return failures
}

func validateEventHandoffReference(rootPath, relative string) []string {
	if !safeRepositoryPath(relative) || filepath.Ext(relative) != ".md" {
		return []string{fmt.Sprintf("invalid handoff_reference %q", relative)}
	}
	data, err := os.ReadFile(filepath.Join(rootPath, filepath.FromSlash(relative)))
	if err != nil {
		return []string{fmt.Sprintf("read event handoff reference %s: %v", relative, err)}
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	const expected = `<!-- DWS_EVENT_CHAT_HANDOFF_START -->
| event field | exact chat target |
|---|---|
| ` + "`conversation_id`" + ` | ` + "`dws chat +messages-send --as user --group <conversation_id>`" + ` |
| ` + "`sender_open_dingtalk_id`" + ` | ` + "`dws chat +messages-send --as user --open-dingtalk-id <sender_open_dingtalk_id>`" + ` |
<!-- DWS_EVENT_CHAT_HANDOFF_END -->`
	start := bytes.Index(data, []byte("<!-- DWS_EVENT_CHAT_HANDOFF_START -->"))
	endMarker := "<!-- DWS_EVENT_CHAT_HANDOFF_END -->"
	if start < 0 || bytes.Count(data, []byte("<!-- DWS_EVENT_CHAT_HANDOFF_START -->")) != 1 ||
		bytes.Count(data, []byte(endMarker)) != 1 {
		return []string{fmt.Sprintf("%s must contain exactly one event-to-chat handoff marker pair", relative)}
	}
	end := bytes.Index(data[start:], []byte(endMarker))
	if end < 0 || string(data[start:start+end+len(endMarker)]) != expected {
		return []string{fmt.Sprintf("%s event-to-chat handoff differs from exact stable-ID mapping", relative)}
	}
	return nil
}

func renderMessageResultContract() string {
	contract := chatmsg.CurrentMessageResultContract()
	return fmt.Sprintf("- `version`: `%s`\n- `message_fields`: %s\n- `envelope_fields`: %s",
		contract.Version, markdownCodeList(contract.MessageFields), markdownCodeList(contract.EnvelopeFields))
}

func renderIdentityCapabilityContract() string {
	var rows []string
	for _, capability := range chatshortcut.MessageIdentityCapabilities() {
		rows = append(rows, fmt.Sprintf("| `%s` | %s | %s | %s | %s | `%t` | `%t` |",
			capability.Identity, markdownBreakList(capability.Targets), markdownBreakList(capability.ContentTypes),
			markdownBreakList(capability.NaturalTargets), markdownBreakList(capability.MentionTargets),
			capability.IdempotencyKeys, capability.BatchLedger))
	}
	return "| identity | targets | content types | natural targets | mention targets | idempotency keys | batch ledger |\n" +
		"|---|---|---|---|---|---:|---:|\n" + strings.Join(rows, "\n")
}

func renderCardWorkflowContract() string {
	contract := chatshortcut.CurrentCardWorkflowContract()
	statuses := make([]string, 0, len(contract.FlowStatuses))
	for _, status := range contract.FlowStatuses {
		statuses = append(statuses, fmt.Sprintf("%d=%s", status.Value, status.Name))
	}
	return fmt.Sprintf("- `version`: `%s`\n- `targets`: %s\n- `content_types`: %s\n- `flow_statuses`: %s\n- `callback_supported`: `%t`",
		contract.Version, markdownCodeList(contract.Targets), markdownCodeList(contract.ContentTypes),
		markdownCodeList(statuses), contract.CallbackSupported)
}

func renderCapabilityBoundaryContract() string {
	rows := make([]string, 0)
	for _, boundary := range chatshortcut.CurrentIMCapabilityBoundaries() {
		rows = append(rows, fmt.Sprintf("| `%s` | `%t` | %s |", boundary.Capability, boundary.Supported, boundary.Alternative))
	}
	return "| capability | supported | current route / boundary |\n|---|---:|---|\n" + strings.Join(rows, "\n")
}

func markdownCodeList(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = "`" + value + "`"
	}
	return strings.Join(quoted, ", ")
}

func markdownBreakList(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = "`" + value + "`"
	}
	return strings.Join(quoted, "<br>")
}

func scanMarkers(rootPath string, roots []string, intents map[string]intentRoute, tools map[string]toolFact) ([]string, map[string]int) {
	var failures []string
	markers := map[string]int{}
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
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
			lineNumber := 0
			for scanner.Scan() {
				lineNumber++
				line := scanner.Text()
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
	return next == ' ' || next == '`' || next == '<' || next == '|' || next == ','
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if filepath.ToSlash(value) == target {
			return true
		}
	}
	return false
}

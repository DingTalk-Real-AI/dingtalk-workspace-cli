// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wukonghints

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator/agentmetadata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
)

const auditVersion = 1

type Options struct {
	EnvelopeDir string
	SurfacePath string
	Repository  string
	Revision    string
	Channel     string
	MaxExamples int
	SourceName  string
}

type Result struct {
	Hints agentmetadata.HintFile
	Audit Audit
}

type Audit struct {
	Version   int                        `json:"version"`
	Source    agentmetadata.HintSource   `json:"source"`
	Coverage  agentmetadata.HintCoverage `json:"coverage"`
	Skipped   []AuditEntry               `json:"skipped,omitempty"`
	Unmatched []AuditEntry               `json:"unmatched,omitempty"`
	Conflicts []Conflict                 `json:"conflicts,omitempty"`
}

type AuditEntry struct {
	ProductID  string   `json:"product_id"`
	RPCName    string   `json:"rpc_name"`
	CLIPath    string   `json:"cli_path,omitempty"`
	Source     string   `json:"source"`
	Reason     string   `json:"reason"`
	Candidates []string `json:"candidates,omitempty"`
}

type Conflict struct {
	CanonicalPath string `json:"canonical_path"`
	Kept          string `json:"kept"`
	Ignored       string `json:"ignored"`
	Source        string `json:"source"`
}

type surfaceSnapshot struct {
	Version  int              `json:"version"`
	Products []surfaceProduct `json:"products"`
}

type surfaceProduct struct {
	ID    string        `json:"id"`
	Tools []surfaceTool `json:"tools"`
}

type surfaceTool struct {
	CanonicalPath string   `json:"canonical_path,omitempty"`
	CLIPath       string   `json:"cli_path"`
	Aliases       []string `json:"aliases,omitempty"`
}

type surfaceTarget struct {
	ProductID     string
	CanonicalPath string
	CLIPath       string
}

type sourceEnvelope struct {
	path    string
	display string
	data    []byte
	value   market.ServerEnvelope
}

func Import(opts Options) (Result, error) {
	if strings.TrimSpace(opts.EnvelopeDir) == "" {
		return Result{}, fmt.Errorf("envelope directory is required")
	}
	if strings.TrimSpace(opts.SurfacePath) == "" {
		return Result{}, fmt.Errorf("command surface path is required")
	}
	if strings.TrimSpace(opts.Revision) == "" {
		return Result{}, fmt.Errorf("source revision is required")
	}
	if opts.MaxExamples <= 0 {
		opts.MaxExamples = 3
	}
	if strings.TrimSpace(opts.SourceName) == "" {
		opts.SourceName = "dws-wukong-envelope"
	}
	if strings.TrimSpace(opts.Channel) == "" {
		opts.Channel = filepath.Base(filepath.Clean(opts.EnvelopeDir))
	}

	paths, surfaceProducts, targets, err := loadSurface(opts.SurfacePath)
	if err != nil {
		return Result{}, err
	}
	envelopes, sourceHash, err := loadEnvelopes(opts.EnvelopeDir)
	if err != nil {
		return Result{}, err
	}
	source := agentmetadata.HintSource{
		Kind:       "imported",
		Name:       strings.TrimSpace(opts.SourceName),
		Repository: strings.TrimSpace(opts.Repository),
		Revision:   strings.TrimSpace(opts.Revision),
		Channel:    strings.TrimSpace(opts.Channel),
		SourceHash: sourceHash,
	}
	result := Result{
		Hints: agentmetadata.HintFile{
			Version:  agentmetadata.HintFileVersion,
			Source:   source,
			Products: map[string]agentmetadata.HintProduct{},
			Tools:    map[string]agentmetadata.HintTool{},
		},
		Audit: Audit{Version: auditVersion, Source: source},
	}
	sourceProducts := map[string]bool{}
	matchedProducts := map[string]bool{}
	matchedTools := map[string]bool{}
	eligibleTools := 0
	sourceTools := 0

	for _, envelope := range envelopes {
		cli := envelope.value.Meta.CLI
		status := strings.ToLower(strings.TrimSpace(envelope.value.Meta.Registry.Status))
		if status != "" && status != "active" {
			continue
		}
		root := firstNonEmpty(cli.Command, cli.ID, envelope.value.Server.Name)
		if root == "" {
			continue
		}
		sourceProducts[root] = true
		if surfaceProducts[root] {
			result.Hints.Products[root] = productHint(source, envelope, root)
			matchedProducts[root] = true
		}
		toolNames := make([]string, 0, len(cli.ToolOverrides))
		for toolName := range cli.ToolOverrides {
			toolNames = append(toolNames, toolName)
		}
		sort.Strings(toolNames)
		for _, toolName := range toolNames {
			override := cli.ToolOverrides[toolName]
			sourceTools++
			entry := AuditEntry{
				ProductID: root,
				RPCName:   toolName,
				Source:    envelope.display,
			}
			switch {
			case override.Hidden:
				entry.Reason = "hidden"
				result.Audit.Skipped = append(result.Audit.Skipped, entry)
				continue
			case strings.TrimSpace(override.RedirectTo) != "":
				entry.Reason = "redirect"
				result.Audit.Skipped = append(result.Audit.Skipped, entry)
				continue
			case strings.TrimSpace(override.CLIName) == "":
				entry.Reason = "missing_cli_name"
				result.Audit.Skipped = append(result.Audit.Skipped, entry)
				continue
			}
			eligibleTools++
			cliPaths := envelopeCLIPaths(root, override)
			entry.CLIPath = cliPaths[0]
			target, ok := firstSurfaceTarget(cliPaths, paths)
			if !ok {
				entry.Reason = "not_in_public_command_surface"
				entry.Candidates = candidatePaths(cliPaths[0], targets, 3)
				result.Audit.Unmatched = append(result.Audit.Unmatched, entry)
				continue
			}
			key := firstNonEmpty(target.CanonicalPath, target.CLIPath)
			incoming := agentmetadata.HintTool{
				AgentSummary: strings.TrimSpace(override.Description),
				Examples:     extractExamples(override.Example, cliPaths, opts.MaxExamples),
				SourceRefs:   []string{sourceReference(source, envelope.display, root, toolName)},
				InterfaceRef: &agentmetadata.HintInterfaceRef{
					ProductID: root,
					RPCName:   toolName,
				},
			}
			if override.IsSensitive {
				incoming.Risk = "high"
				incoming.Confirmation = "user_required"
			}
			existing := result.Hints.Tools[key]
			if existing.AgentSummary != "" && incoming.AgentSummary != "" && existing.AgentSummary != incoming.AgentSummary {
				result.Audit.Conflicts = append(result.Audit.Conflicts, Conflict{
					CanonicalPath: key,
					Kept:          existing.AgentSummary,
					Ignored:       incoming.AgentSummary,
					Source:        envelope.display,
				})
			}
			result.Hints.Tools[key] = mergeHintTool(existing, incoming)
			matchedTools[key] = true
			matchedProducts[target.ProductID] = true
			if _, exists := result.Hints.Products[target.ProductID]; !exists {
				result.Hints.Products[target.ProductID] = productHint(source, envelope, root)
			}
		}
	}

	coverage := agentmetadata.HintCoverage{
		SourceProducts:  len(sourceProducts),
		MatchedProducts: len(matchedProducts),
		SourceTools:     sourceTools,
		EligibleTools:   eligibleTools,
		MatchedTools:    len(matchedTools),
		UnmatchedTools:  len(result.Audit.Unmatched),
	}
	result.Hints.Coverage = coverage
	result.Audit.Coverage = coverage
	sortAudit(&result.Audit)
	return result, nil
}

func productHint(source agentmetadata.HintSource, envelope sourceEnvelope, productID string) agentmetadata.HintProduct {
	summary := strings.TrimSpace(envelope.value.Meta.CLI.Description)
	if summary == "" {
		summary = strings.TrimSpace(envelope.value.Server.Description)
	}
	return agentmetadata.HintProduct{
		AgentSummary: summary,
		SourceRefs:   []string{sourceReference(source, envelope.display, productID, "")},
	}
}

func loadSurface(path string) (map[string]surfaceTarget, map[string]bool, []surfaceTarget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read command surface: %w", err)
	}
	var snapshot surfaceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, nil, nil, fmt.Errorf("decode command surface: %w", err)
	}
	if snapshot.Version != 1 {
		return nil, nil, nil, fmt.Errorf("unsupported command surface version %d", snapshot.Version)
	}
	paths := map[string]surfaceTarget{}
	products := map[string]bool{}
	targets := []surfaceTarget{}
	for _, product := range snapshot.Products {
		productID := strings.TrimSpace(product.ID)
		if productID == "" {
			continue
		}
		products[productID] = true
		for _, tool := range product.Tools {
			target := surfaceTarget{
				ProductID:     productID,
				CanonicalPath: strings.TrimSpace(tool.CanonicalPath),
				CLIPath:       normalizePath(tool.CLIPath),
			}
			if target.CLIPath == "" {
				continue
			}
			targets = append(targets, target)
			paths[target.CLIPath] = target
			for _, alias := range tool.Aliases {
				if alias = normalizePath(alias); alias != "" {
					paths[alias] = target
				}
			}
		}
	}
	return paths, products, targets, nil
}

func loadEnvelopes(root string) ([]sourceEnvelope, string, error) {
	root = filepath.Clean(root)
	paths := []string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return nil, "", fmt.Errorf("walk Wukong envelopes: %w", err)
	}
	sort.Strings(paths)
	hash := sha256.New()
	envelopes := []sourceEnvelope{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("read Wukong envelope %s: %w", path, err)
		}
		display, err := filepath.Rel(root, path)
		if err != nil {
			display = filepath.Base(path)
		}
		display = filepath.ToSlash(display)
		_, _ = hash.Write([]byte(display))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
		var envelope market.ServerEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, "", fmt.Errorf("decode Wukong envelope %s: %w", path, err)
		}
		if firstNonEmpty(envelope.Meta.CLI.ID, envelope.Meta.CLI.Command, envelope.Server.Name) == "" {
			continue
		}
		envelopes = append(envelopes, sourceEnvelope{path: path, display: display, data: data, value: envelope})
	}
	return envelopes, "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func envelopeCLIPaths(root string, override market.CLIToolOverride) []string {
	group := strings.ReplaceAll(strings.TrimSpace(override.Group), ".", " ")
	base := normalizePath(strings.Join([]string{root, group}, " "))
	names := append([]string{strings.TrimSpace(override.CLIName)}, override.CLIAliases...)
	paths := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		path := normalizePath(strings.Join([]string{base, name}, " "))
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

func firstSurfaceTarget(candidates []string, paths map[string]surfaceTarget) (surfaceTarget, bool) {
	for _, path := range candidates {
		if target, ok := paths[normalizePath(path)]; ok {
			return target, true
		}
	}
	return surfaceTarget{}, false
}

func mergeHintTool(existing, incoming agentmetadata.HintTool) agentmetadata.HintTool {
	if existing.AgentSummary == "" {
		existing.AgentSummary = incoming.AgentSummary
	}
	if existing.Risk == "" {
		existing.Risk = incoming.Risk
	}
	if existing.Confirmation == "" {
		existing.Confirmation = incoming.Confirmation
	}
	if existing.InterfaceRef == nil && incoming.InterfaceRef != nil {
		ref := *incoming.InterfaceRef
		existing.InterfaceRef = &ref
	}
	existing.Examples = uniqueStrings(append(existing.Examples, incoming.Examples...))
	existing.SourceRefs = uniqueStrings(append(existing.SourceRefs, incoming.SourceRefs...))
	return existing
}

func extractExamples(raw string, paths []string, limit int) []string {
	lines := strings.Split(raw, "\n")
	commands := []string{}
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		parts := []string{}
		for {
			continued := strings.HasSuffix(line, "\\")
			line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
			if line != "" {
				parts = append(parts, line)
			}
			if !continued || index+1 >= len(lines) {
				break
			}
			index++
			line = strings.TrimSpace(lines[index])
		}
		command := strings.TrimSpace(strings.Join(parts, " "))
		if !strings.HasPrefix(command, "dws ") || len(command) > 500 || !exampleMatchesPath(command, paths) {
			continue
		}
		commands = append(commands, command)
		if len(commands) >= limit {
			break
		}
	}
	return uniqueStrings(commands)
}

func exampleMatchesPath(command string, paths []string) bool {
	command = strings.TrimSpace(strings.TrimPrefix(command, "dws "))
	for _, path := range paths {
		path = normalizePath(path)
		if command == path || strings.HasPrefix(command, path+" ") {
			return true
		}
	}
	return false
}

func candidatePaths(path string, targets []surfaceTarget, limit int) []string {
	parts := strings.Fields(path)
	if len(parts) == 0 || limit <= 0 {
		return nil
	}
	product, verb := parts[0], parts[len(parts)-1]
	values := []string{}
	for _, target := range targets {
		candidateParts := strings.Fields(target.CLIPath)
		if len(candidateParts) == 0 || candidateParts[0] != product {
			continue
		}
		if candidateParts[len(candidateParts)-1] == verb {
			values = append(values, target.CLIPath)
		}
	}
	sort.Strings(values)
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}

func sourceReference(source agentmetadata.HintSource, file, productID, toolName string) string {
	revision := strings.TrimSpace(source.Revision)
	if len(revision) > 12 {
		revision = revision[:12]
	}
	ref := strings.TrimSpace(source.Name)
	if revision != "" {
		ref += "@" + revision
	}
	ref += ":" + filepath.ToSlash(file)
	if toolName != "" {
		ref += "#" + productID + "." + toolName
	}
	return ref
}

func sortAudit(audit *Audit) {
	sort.Slice(audit.Skipped, func(i, j int) bool { return auditEntryLess(audit.Skipped[i], audit.Skipped[j]) })
	sort.Slice(audit.Unmatched, func(i, j int) bool { return auditEntryLess(audit.Unmatched[i], audit.Unmatched[j]) })
	sort.Slice(audit.Conflicts, func(i, j int) bool { return audit.Conflicts[i].CanonicalPath < audit.Conflicts[j].CanonicalPath })
}

func auditEntryLess(left, right AuditEntry) bool {
	if left.ProductID != right.ProductID {
		return left.ProductID < right.ProductID
	}
	if left.CLIPath != right.CLIPath {
		return left.CLIPath < right.CLIPath
	}
	return left.RPCName < right.RPCName
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizePath(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

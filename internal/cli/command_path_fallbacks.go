// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

const commandPathFallbacksSchemaRef = "./command_path_fallbacks.schema.json"

// command_path_fallbacks.json is a reviewed recovery policy for invalid paths
// observed in evaluation. It is deliberately not a command identity source:
// Help, Skill, Schema, and normal navigation continue to publish canonical
// paths from the CommandRegistry and the real Cobra tree only.

//go:embed command_path_fallbacks.json
var embeddedCommandPathFallbacksJSON []byte

//go:embed command_path_fallbacks.schema.json
var embeddedCommandPathFallbacksSchemaJSON []byte

var commandFallbackPathPattern = regexp.MustCompile(`^[A-Za-z0-9+][A-Za-z0-9._:+-]*(?: [A-Za-z0-9+][A-Za-z0-9._:+-]*)*$`)

// CommandPathFallbackMode determines whether an invalid reviewed path can be
// normalized exactly or must stop and surface reviewed candidates.
type CommandPathFallbackMode string

const (
	CommandPathFallbackRewrite   CommandPathFallbackMode = "rewrite"
	CommandPathFallbackAmbiguous CommandPathFallbackMode = "ambiguous"
)

// CommandPathFallback is one validated, generated runtime recovery record.
// From is never advertised as a stable alias. To and Candidates are canonical
// real Cobra paths validated at generation time.
type CommandPathFallback struct {
	From         string                  `json:"from"`
	Mode         CommandPathFallbackMode `json:"mode"`
	To           string                  `json:"to,omitempty"`
	Candidates   []string                `json:"candidates,omitempty"`
	Reviewed     bool                    `json:"reviewed"`
	ReviewReason string                  `json:"review_reason"`
}

type commandPathFallbackSnapshot struct {
	Schema  string                         `json:"$schema"`
	Version int                            `json:"version"`
	Entries []commandPathFallbackEntrySpec `json:"entries"`
}

type commandPathFallbackEntrySpec struct {
	From         string                  `json:"from"`
	Mode         CommandPathFallbackMode `json:"mode"`
	To           string                  `json:"to,omitempty"`
	Candidates   []string                `json:"candidates,omitempty"`
	Reviewed     bool                    `json:"reviewed"`
	ReviewReason string                  `json:"review_reason"`
}

var (
	embeddedCommandPathFallbacksOnce sync.Once
	embeddedCommandPathFallbacksData []CommandPathFallback
	embeddedCommandPathFallbacksErr  error
	loadReviewedCommandPathFallbacks = loadEmbeddedCommandPathFallbacks
)

// LoadCommandPathFallbacks decodes and validates the authored recovery table.
// Callers receive a clone so generation and tests cannot mutate shared data.
func LoadCommandPathFallbacks() ([]CommandPathFallback, error) {
	return loadReviewedCommandPathFallbacks()
}

func loadEmbeddedCommandPathFallbacks() ([]CommandPathFallback, error) {
	embeddedCommandPathFallbacksOnce.Do(func() {
		embeddedCommandPathFallbacksData, embeddedCommandPathFallbacksErr = decodeCommandPathFallbacks(embeddedCommandPathFallbacksJSON)
	})
	return cloneCommandPathFallbacks(embeddedCommandPathFallbacksData), embeddedCommandPathFallbacksErr
}

func decodeCommandPathFallbacks(data []byte) ([]CommandPathFallback, error) {
	var snapshot commandPathFallbackSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode reviewed command path fallbacks: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("decode reviewed command path fallbacks: %w", err)
	}
	if snapshot.Version != 1 {
		return nil, fmt.Errorf("unsupported command path fallbacks version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.Schema) != commandPathFallbacksSchemaRef {
		return nil, fmt.Errorf("command path fallbacks must declare $schema=%q", commandPathFallbacksSchemaRef)
	}
	if len(snapshot.Entries) == 0 {
		return nil, fmt.Errorf("command path fallbacks declares no entries")
	}

	entries := make([]CommandPathFallback, 0, len(snapshot.Entries))
	seenFrom := make(map[string]bool, len(snapshot.Entries))
	for index, spec := range snapshot.Entries {
		from, err := validateAuthoredCommandFallbackPath(spec.From)
		if err != nil {
			return nil, fmt.Errorf("command path fallback entry %d from: %w", index, err)
		}
		if seenFrom[from] {
			return nil, fmt.Errorf("command path fallbacks contains duplicate from path %q", from)
		}
		seenFrom[from] = true
		reason := strings.TrimSpace(spec.ReviewReason)
		if !spec.Reviewed || reason == "" {
			return nil, fmt.Errorf("command path fallback %q requires reviewed=true and non-empty review_reason", from)
		}

		entry := CommandPathFallback{
			From:         from,
			Mode:         spec.Mode,
			Reviewed:     true,
			ReviewReason: reason,
		}
		switch spec.Mode {
		case CommandPathFallbackRewrite:
			if len(spec.Candidates) > 0 {
				return nil, fmt.Errorf("rewrite command path fallback %q must not declare candidates", from)
			}
			to, pathErr := validateAuthoredCommandFallbackPath(spec.To)
			if pathErr != nil {
				return nil, fmt.Errorf("rewrite command path fallback %q to: %w", from, pathErr)
			}
			entry.To = to
		case CommandPathFallbackAmbiguous:
			if strings.TrimSpace(spec.To) != "" {
				return nil, fmt.Errorf("ambiguous command path fallback %q must not declare to", from)
			}
			if len(spec.Candidates) < 2 {
				return nil, fmt.Errorf("ambiguous command path fallback %q requires at least two candidates", from)
			}
			seenCandidates := make(map[string]bool, len(spec.Candidates))
			for _, rawCandidate := range spec.Candidates {
				candidate, pathErr := validateAuthoredCommandFallbackPath(rawCandidate)
				if pathErr != nil {
					return nil, fmt.Errorf("ambiguous command path fallback %q candidate: %w", from, pathErr)
				}
				if seenCandidates[candidate] {
					return nil, fmt.Errorf("ambiguous command path fallback %q repeats candidate %q", from, candidate)
				}
				seenCandidates[candidate] = true
				entry.Candidates = append(entry.Candidates, candidate)
			}
		default:
			return nil, fmt.Errorf("command path fallback %q has invalid mode %q", from, spec.Mode)
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].From < entries[j].From })
	return entries, nil
}

func validateAuthoredCommandFallbackPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path != raw || normalizeSchemaCLIPath(path) != path || !commandFallbackPathPattern.MatchString(path) {
		return "", fmt.Errorf("path %q is not a normalized command path without leading dws or flags", raw)
	}
	return path, nil
}

// ReduceCommandPathFallbacks validates the reviewed recovery table against the
// live distribution-owned Cobra tree. It refuses to turn an existing command
// or stable alias into a hidden rewrite. The sole exception is an explicitly
// annotated hint-only compatibility node, which contains no business action.
func ReduceCommandPathFallbacks(root *cobra.Command) ([]CommandPathFallback, error) {
	if root == nil {
		return nil, fmt.Errorf("command path fallback source root is nil")
	}
	entries, err := LoadCommandPathFallbacks()
	if err != nil {
		return nil, err
	}

	byFrom := make(map[string]CommandPathFallback, len(entries))
	for _, entry := range entries {
		byFrom[entry.From] = entry
	}
	var problems []string
	for _, entry := range entries {
		match, resolveErr := resolveExactCobraPath(root, entry.From)
		if resolveErr != nil {
			problems = append(problems, fmt.Sprintf("from %q cannot be resolved safely: %v", entry.From, resolveErr))
		} else if match.Command != nil && !cmdutil.IsHintOnlyCommand(match.Command) {
			problems = append(problems, fmt.Sprintf("from %q collides with a real Cobra command or alias", entry.From))
		}

		switch entry.Mode {
		case CommandPathFallbackRewrite:
			if _, chained := byFrom[entry.To]; chained {
				problems = append(problems, fmt.Sprintf("rewrite %q targets fallback source %q; chained fallbacks are forbidden", entry.From, entry.To))
			}
			problems = append(problems, validateCommandFallbackTarget(root, entry.From, entry.To, true)...)
		case CommandPathFallbackAmbiguous:
			for _, candidate := range entry.Candidates {
				if _, chained := byFrom[candidate]; chained {
					problems = append(problems, fmt.Sprintf("ambiguous fallback %q candidate %q is another fallback source", entry.From, candidate))
				}
				// Ambiguous recovery never dispatches. It may therefore point the
				// caller at both canonical shortcuts and native leaves, while exact
				// rewrites keep the stricter +shortcut identity boundary.
				problems = append(problems, validateCommandFallbackTarget(root, entry.From, candidate, false)...)
			}
		default:
			problems = append(problems, fmt.Sprintf("fallback %q has unsupported mode %q", entry.From, entry.Mode))
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("command path fallback reduction failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cloneCommandPathFallbacks(entries), nil
}

func validateCommandFallbackTarget(root *cobra.Command, from, target string, requireShortcutParity bool) []string {
	var problems []string
	if commandFallbackService(from) != commandFallbackService(target) {
		problems = append(problems, fmt.Sprintf("fallback %q crosses service boundary to %q", from, target))
	}
	if requireShortcutParity && commandFallbackHasShortcut(from) != commandFallbackHasShortcut(target) {
		problems = append(problems, fmt.Sprintf("fallback %q and target %q disagree on +shortcut identity", from, target))
	}
	match, err := resolveExactCobraPath(root, target)
	if err != nil {
		return append(problems, fmt.Sprintf("fallback %q target %q cannot be resolved safely: %v", from, target, err))
	}
	if match.Command == nil {
		return append(problems, fmt.Sprintf("fallback %q target %q does not exist", from, target))
	}
	if match.UsedAlias {
		problems = append(problems, fmt.Sprintf("fallback %q target %q must use canonical Cobra names, not an alias", from, target))
	}
	if !runnableSchemaLeaf(match.Command) || !match.Command.IsAvailableCommand() {
		problems = append(problems, fmt.Sprintf("fallback %q target %q is not a public runnable Cobra leaf", from, target))
	}
	return problems
}

func commandFallbackService(path string) string {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func commandFallbackHasShortcut(path string) bool {
	for _, part := range strings.Fields(path) {
		if strings.HasPrefix(part, "+") {
			return true
		}
	}
	return false
}

func cloneCommandPathFallbacks(entries []CommandPathFallback) []CommandPathFallback {
	cloned := make([]CommandPathFallback, len(entries))
	for index, entry := range entries {
		cloned[index] = entry
		cloned[index].Candidates = append([]string(nil), entry.Candidates...)
	}
	return cloned
}

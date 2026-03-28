package surface

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/mcp-probe/runner"
)

type Page struct {
	Path       []string
	Summary    string
	Aliases    []string
	Available  []CommandEntry
	LocalFlags []FlagEntry
	Raw        string
}

type CommandEntry struct {
	Name        string
	Description string
}

type FlagEntry struct {
	Names []string
}

type Diff struct {
	Equal   bool
	Details []string
}

var multiSpacePattern = regexp.MustCompile(`\s{2,}`)

func Crawl(ctx context.Context, r *runner.Runner) ([]Page, error) {
	if r == nil {
		return nil, fmt.Errorf("runner is nil")
	}

	type node struct {
		path []string
	}

	queue := []node{{path: nil}}
	seen := make(map[string]struct{})
	var pages []Page

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		key := strings.Join(current.path, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		result, err := r.Run(ctx, helpArgs(current.path))
		if err != nil {
			return nil, fmt.Errorf("run help for %q: %w", strings.Join(current.path, " "), err)
		}
		if result.ExitCode != 0 {
			return nil, fmt.Errorf("help for %q exited with %d: %s", strings.Join(current.path, " "), result.ExitCode, result.Stderr)
		}

		page := ParsePage(current.path, result.Stdout)
		pages = append(pages, page)

		for _, child := range page.Available {
			nextPath := append(append([]string{}, current.path...), child.Name)
			childKey := strings.Join(nextPath, "\x00")
			if _, ok := seen[childKey]; ok {
				continue
			}
			queue = append(queue, node{path: nextPath})
		}
	}

	return pages, nil
}

func ParsePage(path []string, raw string) Page {
	var pagePath []string
	if len(path) > 0 {
		pagePath = append([]string{}, path...)
	}
	page := Page{
		Path: pagePath,
		Raw:  raw,
	}

	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	section := ""
	seenUsage := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case "":
			continue
		case "Usage:":
			seenUsage = true
			section = ""
			continue
		case "Aliases:":
			section = "aliases"
			continue
		case "Available Commands:":
			section = "commands"
			continue
		case "Flags:":
			section = "flags"
			continue
		case "Global Flags:", "Examples:":
			section = ""
			continue
		}

		if !seenUsage && page.Summary == "" {
			page.Summary = trimmed
			continue
		}

		switch section {
		case "aliases":
			page.Aliases = append(page.Aliases, normalizeAliases(trimmed)...)
		case "commands":
			left, right, ok := splitColumns(line)
			if !ok {
				continue
			}
			page.Available = append(page.Available, CommandEntry{
				Name:        left,
				Description: right,
			})
		case "flags":
			left, _, ok := splitColumns(line)
			if !ok {
				continue
			}
			if !strings.HasPrefix(strings.TrimSpace(left), "-") {
				continue
			}
			page.LocalFlags = append(page.LocalFlags, FlagEntry{
				Names: normalizeFlagNames(left),
			})
		}
	}

	return page
}

func ComparePage(expected, actual Page) Diff {
	var details []string
	if !equalCommandNames(expected.Available, actual.Available) {
		details = append(details, "available commands mismatch")
	}
	if !equalFlagNames(expected.LocalFlags, actual.LocalFlags) {
		details = append(details, "local flags mismatch")
	}
	return Diff{
		Equal:   len(details) == 0,
		Details: details,
	}
}

func splitColumns(line string) (left string, right string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", false
	}
	idx := multiSpacePattern.FindStringIndex(trimmed)
	if idx == nil {
		return "", "", false
	}
	left = strings.TrimSpace(trimmed[:idx[0]])
	right = strings.TrimSpace(trimmed[idx[1]:])
	if left == "" {
		return "", "", false
	}
	return left, right, true
}

func normalizeAliases(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.Split(text, ",")
	aliases := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		aliases = append(aliases, part)
	}
	return aliases
}

func normalizeFlagNames(text string) []string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		names = append(names, fields[0])
	}
	return names
}

func equalCommandNames(expected, actual []CommandEntry) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i].Name != actual[i].Name {
			return false
		}
	}
	return true
}

func equalFlagNames(expected, actual []FlagEntry) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if !equalStringSlices(expected[i].Names, actual[i].Names) {
			return false
		}
	}
	return true
}

func equalStringSlices(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return false
		}
	}
	return true
}

func helpArgs(path []string) []string {
	args := append([]string{}, path...)
	args = append(args, "--help")
	return args
}

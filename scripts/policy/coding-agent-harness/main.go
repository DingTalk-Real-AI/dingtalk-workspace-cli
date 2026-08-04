package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var guideLineLimits = map[string]int{
	"AGENTS.md":                  80,
	"internal/cli/AGENTS.md":     80,
	"internal/helpers/AGENTS.md": 100,
	"skills/AGENTS.md":           80,
}

var markdownLinkPattern = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)

type fileContract struct {
	path     string
	required []string
}

type taskField struct {
	label        string
	allowNone    bool
	allowedValue map[string]bool
}

var taskFields = []taskField{
	{label: "Task kind", allowedValue: map[string]bool{"bug": true, "feature": true, "refactor": true, "docs": true, "policy": true, "release": true}},
	{label: "Goal (one primary outcome)"},
	{label: "Current behavior and evidence"},
	{label: "Acceptance criteria"},
	{label: "In scope (packages/files/surfaces)"},
	{label: "Out of scope", allowNone: true},
	{label: "Compatibility constraints", allowNone: true},
	{label: "Interface impact (commands/flags/output/errors/exit codes/Schema)", allowNone: true},
	{label: "Safety or data-mutation constraints", allowNone: true},
	{label: "Expected validation"},
	{label: "Known environment limitations", allowNone: true},
}

var guideContracts = []fileContract{
	{
		path: "AGENTS.md",
		required: []string{
			"docs/coding-agent-guide.md",
			"docs/coding-agent-task-template.md",
			"docs/schema-contributor-guide.md",
			"docs/helpers-structure-guide.md",
			"docs/architecture.md",
			"docs/skill-authoring-guide.md",
			"skills/AGENTS.md",
			"internal/helpers/AGENTS.md",
			"docs/automation.md",
			"docs/agent-code.md",
			"Do not depend on generated Wiki or CodeWiki content.",
		},
	},
	{
		path: "CONTRIBUTING.md",
		required: []string{
			"docs/coding-agent-guide.md",
			"docs/schema-contributor-guide.md",
		},
	},
	{
		path: "docs/coding-agent-guide.md",
		required: []string{
			"## 1. Normalize the task input",
			"coding-agent-task-template.md",
			"make coding-agent-task TASK=",
			"primary outcome per task",
			"helpers-structure-guide.md",
			"not wired into CI",
			"github.com/larksuite/cli/blob/",
			"github.com/WecomTeam/wecom-cli/blob/",
			"## 2. Establish the baseline",
			"## 3. Implement from authoritative inputs",
			"## 4. Select validation by change surface",
			"Documentation only",
			"Go implementation",
			"CLI paths or flags",
			"Schema registry, hints, or generators",
			"CI or test sharding",
			"Packaging or installers",
			"Authentication, transport, or OS-specific code",
			"## 5. Pre-handoff self-check",
			"Outcome: what is now true",
			"Validation: exact commands and results",
		},
	},
	{
		path: "docs/architecture.md",
		required: []string{
			"## Change Rules",
			"helpers-structure-guide.md",
			"skill-authoring-guide.md",
			"schema-contributor-guide.md",
			"## Repository Structure",
			"internal/helpers",
			"skills/",
		},
	},
	{
		path: "docs/skill-authoring-guide.md",
		required: []string{
			"skills/mono/",
			"skills/multi/dingtalk-<product>/",
			"dws-shared",
			"SAFETY_PREAMBLE_INJECT",
			"Dual-write rule",
			"make skill-command-integrity",
			"schema-contributor-guide.md",
		},
	},
	{
		path: "skills/AGENTS.md",
		required: []string{
			"../docs/coding-agent-guide.md",
			"../docs/skill-authoring-guide.md",
			"../docs/schema-contributor-guide.md",
			"SAFETY_PREAMBLE_INJECT",
			"make skill-command-integrity",
			"Dual-write",
		},
	},
	{
		path: "docs/helpers-structure-guide.md",
		required: []string{
			"package helpers",
			"{product}.go",
			"{product}_{resource}.go",
			"sheet.go",
			"register_products.go",
			"Anti-pattern to stop growing",
			"Mechanical splits",
			"make coding-agent-harness",
			"not part of `make policy` or CI",
		},
	},
	{
		path: "docs/coding-agent-task-template.md",
		required: []string{
			"Goal (one primary outcome):",
			"Current behavior and evidence:",
			"Acceptance criteria:",
			"In scope (packages/files/surfaces):",
			"Out of scope:",
			"Compatibility constraints:",
			"Interface impact (commands/flags/output/errors/exit codes/Schema):",
			"Safety or data-mutation constraints:",
			"Expected validation:",
			"Known environment limitations:",
			"Expected stdout/stderr and exit behavior:",
			"Mutation preview/confirmation behavior:",
		},
	},
	{
		path: "docs/schema-contributor-guide.md",
		required: []string{
			"internal/cli/schema_command_registry.json",
			"internal/cli/schema_command_registry.schema.json",
			"internal/cli/schema_hints/metadata/<product>.json",
			"internal/cli/schema_hints/selection/<product>.json",
			"internal/cli/schema_parameter_bindings.json",
			"internal/cli/schema_mcp_metadata.json",
			"internal/cli/schema_command_exclusions.json",
			"internal/cli/schema_catalog.json",
			"make generate-schema",
			"make test-schema-agent-examples",
		},
	},
	{
		path: "internal/helpers/AGENTS.md",
		required: []string{
			"../../CONTRIBUTING.md",
			"../../docs/coding-agent-guide.md",
			"../../docs/schema-contributor-guide.md",
			"../../docs/helpers-structure-guide.md",
			"File layout (hard rules)",
			"Do not grow megafiles",
			"Keep stdout machine-readable business data.",
			"structured error category",
			"preview/confirmation",
			"Do not claim a live",
		},
	},
	{
		path: "internal/cli/AGENTS.md",
		required: []string{
			"../../docs/schema-contributor-guide.md",
			"schema_command_registry.json",
			"schema_parameter_bindings.json",
			"schema_hints/metadata/<product>.json",
			"schema_hints/selection/<product>.json",
			"schema_command_exclusions.json",
			"schema_catalog.json",
			"make generate-schema",
		},
	},
}

var requiredPaths = []string{
	"docs/automation.md",
	"docs/agent-code.md",
	"scripts/policy/check-runtime-confirmation-truth.sh",
	"scripts/policy/check-coding-agent-harness.sh",
	"scripts/policy/check-generated-drift.sh",
	"scripts/policy/check-schema-catalog.sh",
	"scripts/policy/check-command-surface.sh",
	"scripts/release/verify-package-managers.sh",
}

var requiredMakeTargets = []string{
	"build",
	"coding-agent-harness",
	"coding-agent-task",
	"format-check",
	"test",
	"policy",
	"interface-integrity",
	"skill-command-integrity",
	"test-schema-agent-examples",
	"generate-schema",
	"package",
}

func main() {
	root := flag.String("root", ".", "repository root")
	task := flag.String("task", "", "optional filled coding-agent task file to validate")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "coding agent harness: unexpected positional arguments")
		os.Exit(2)
	}

	problems := validate(*root)
	if strings.TrimSpace(*task) != "" {
		taskPath := *task
		if !filepath.IsAbs(taskPath) {
			taskPath = filepath.Join(*root, taskPath)
		}
		problems = append(problems, validateTask(taskPath)...)
		sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	}
	if len(problems) != 0 {
		fmt.Fprintln(os.Stderr, "coding agent harness: failed")
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "- %v\n", problem)
		}
		os.Exit(1)
	}
	fmt.Println("coding agent harness: ok")
}

func validateTask(path string) []error {
	content, err := os.ReadFile(path)
	if err != nil {
		return []error{fmt.Errorf("read task contract %s: %w", path, err)}
	}

	values := make(map[string][]string)
	occurrences := make(map[string]int)
	current := ""
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matched := false
		for _, field := range taskFields {
			prefix := field.label + ":"
			if strings.HasPrefix(line, prefix) {
				current = field.label
				occurrences[current]++
				inline := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if inline != "" {
					values[current] = append(values[current], inline)
				}
				matched = true
				break
			}
		}
		if matched || current == "" || line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		values[current] = append(values[current], line)
	}
	if err := scanner.Err(); err != nil {
		return []error{fmt.Errorf("scan task contract %s: %w", path, err)}
	}

	var problems []error
	for _, field := range taskFields {
		if occurrences[field.label] > 1 {
			problems = append(problems, fmt.Errorf("task contract field %q appears %d times; each field must be unique", field.label, occurrences[field.label]))
		}
		value := strings.TrimSpace(strings.Join(values[field.label], "\n"))
		if value == "" {
			problems = append(problems, fmt.Errorf("task contract is missing a value for %q", field.label))
			continue
		}
		normalized := strings.ToLower(value)
		if len(field.allowedValue) != 0 && !field.allowedValue[normalized] {
			problems = append(problems, fmt.Errorf("task contract field %q has unsupported value %q", field.label, value))
		}
		if isPlaceholderTaskValue(normalized) {
			problems = append(problems, fmt.Errorf("task contract field %q still contains placeholder value %q", field.label, value))
		}
		if !field.allowNone && (normalized == "none" || normalized == "n/a" || normalized == "not applicable") {
			problems = append(problems, fmt.Errorf("task contract field %q requires concrete evidence", field.label))
		}
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	return problems
}

func isPlaceholderTaskValue(value string) bool {
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimLeft(trimmed, "-*0123456789.) ")
		switch trimmed {
		case "todo", "tbd", "unknown", "fill me", "to be decided":
			return true
		}
		if strings.HasPrefix(trimmed, "todo:") || strings.HasPrefix(trimmed, "tbd:") || strings.Contains(trimmed, "<fill") || strings.Contains(trimmed, "<todo") {
			return true
		}
	}
	return false
}

func validate(root string) []error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return []error{fmt.Errorf("resolve root: %w", err)}
	}

	var problems []error
	for _, contract := range guideContracts {
		content, readErr := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(contract.path)))
		if readErr != nil {
			problems = append(problems, fmt.Errorf("read %s: %w", contract.path, readErr))
			continue
		}
		text := string(content)
		for _, required := range contract.required {
			if !strings.Contains(text, required) {
				problems = append(problems, fmt.Errorf("%s is missing required contract text %q", contract.path, required))
			}
		}
		problems = append(problems, validateLocalLinks(absRoot, contract.path, text)...)
	}

	for path, limit := range guideLineLimits {
		if lineCount, countErr := countLines(filepath.Join(absRoot, filepath.FromSlash(path))); countErr != nil {
			problems = append(problems, fmt.Errorf("count %s lines: %w", path, countErr))
		} else if lineCount > limit {
			problems = append(problems, fmt.Errorf("%s has %d lines; scoped guide limit is %d", path, lineCount, limit))
		}
	}

	for _, path := range requiredPaths {
		info, statErr := os.Stat(filepath.Join(absRoot, filepath.FromSlash(path)))
		if statErr != nil {
			problems = append(problems, fmt.Errorf("required referenced path %s: %w", path, statErr))
		} else if strings.HasSuffix(path, ".sh") && info.Mode().Perm()&0o111 == 0 {
			problems = append(problems, fmt.Errorf("required referenced script %s is not executable", path))
		}
	}

	makeTargets, makeErr := loadMakeTargets(filepath.Join(absRoot, "Makefile"))
	if makeErr != nil {
		problems = append(problems, makeErr)
	} else {
		for _, target := range requiredMakeTargets {
			if !makeTargets[target] {
				problems = append(problems, fmt.Errorf("makefile is missing referenced target %q", target))
			}
		}
	}

	sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	return problems
}

func validateLocalLinks(root, document, content string) []error {
	var problems []error
	documentDir := filepath.Join(root, filepath.Dir(filepath.FromSlash(document)))
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(content, -1) {
		target := strings.TrimSpace(match[1])
		if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		if cut, _, ok := strings.Cut(target, "#"); ok {
			target = cut
		}
		if target == "" {
			continue
		}
		if filepath.IsAbs(target) {
			problems = append(problems, fmt.Errorf("%s contains absolute local link %q", document, match[1]))
			continue
		}
		resolved := filepath.Clean(filepath.Join(documentDir, filepath.FromSlash(target)))
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			problems = append(problems, fmt.Errorf("%s link %q escapes repository root", document, match[1]))
			continue
		}
		if _, err := os.Stat(resolved); err != nil {
			problems = append(problems, fmt.Errorf("%s has broken local link %q: %w", document, match[1], err))
		}
	}
	return problems
}

func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func loadMakeTargets(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read Makefile targets: %w", err)
	}
	defer file.Close()

	targets := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '\t' || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok || strings.ContainsAny(name, "= ") || strings.HasPrefix(name, ".") {
			continue
		}
		targets[name] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("scan Makefile targets: " + err.Error())
	}
	return targets, nil
}

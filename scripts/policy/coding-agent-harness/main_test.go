package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryCodingAgentHarness(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if problems := validate(root); len(problems) != 0 {
		t.Fatalf("repository harness problems:\n%s", formatProblems(problems))
	}
}

func TestCodingAgentHarnessFixturePasses(t *testing.T) {
	root := newHarnessFixture(t)
	if problems := validate(root); len(problems) != 0 {
		t.Fatalf("valid fixture problems:\n%s", formatProblems(problems))
	}
}

func TestCodingAgentTaskContractPasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.md")
	writeTaskFixture(t, path, validTaskContract)
	if problems := validateTask(path); len(problems) != 0 {
		t.Fatalf("valid task problems:\n%s", formatProblems(problems))
	}
}

func TestCodingAgentTaskContractFailsClosed(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantProblem string
	}{
		{
			name:        "missing goal",
			content:     strings.Replace(validTaskContract, "Goal (one primary outcome): Fix retry handling\n", "", 1),
			wantProblem: `missing a value for "Goal (one primary outcome)"`,
		},
		{
			name:        "blank acceptance criteria",
			content:     strings.Replace(validTaskContract, "Acceptance criteria:\n- The retry succeeds once and then stops.\n", "Acceptance criteria:\n", 1),
			wantProblem: `missing a value for "Acceptance criteria"`,
		},
		{
			name:        "unsupported task kind",
			content:     strings.Replace(validTaskContract, "Task kind: bug", "Task kind: experiment", 1),
			wantProblem: `unsupported value "experiment"`,
		},
		{
			name:        "placeholder acceptance bullet",
			content:     strings.Replace(validTaskContract, "- The retry succeeds once and then stops.", "- TBD: decide expected retry result", 1),
			wantProblem: "still contains placeholder value",
		},
		{
			name:        "duplicate goal",
			content:     strings.Replace(validTaskContract, "Goal (one primary outcome): Fix retry handling", "Goal (one primary outcome): Fix retry handling\nGoal (one primary outcome): Refactor transport", 1),
			wantProblem: `field "Goal (one primary outcome)" appears 2 times`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "task.md")
			writeTaskFixture(t, path, test.content)
			problems := formatProblems(validateTask(path))
			if !strings.Contains(problems, test.wantProblem) {
				t.Fatalf("problems:\n%s\nwant substring %q", problems, test.wantProblem)
			}
		})
	}
}

func TestCodingAgentHarnessFailsClosed(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, root string)
		wantProblem string
	}{
		{
			name: "root guide is no longer thin",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "AGENTS.md")
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				for i := 0; i < guideLineLimits["AGENTS.md"]; i++ {
					if _, err := file.WriteString("extra line\n"); err != nil {
						t.Fatal(err)
					}
				}
			},
			wantProblem: "scoped guide limit",
		},
		{
			name: "task input field is removed",
			mutate: func(t *testing.T, root string) {
				replaceFixtureText(t, root, "docs/coding-agent-task-template.md", "Acceptance criteria:", "")
			},
			wantProblem: `docs/coding-agent-task-template.md is missing required contract text "Acceptance criteria:"`,
		},
		{
			name: "route link is broken",
			mutate: func(t *testing.T, root string) {
				replaceFixtureText(t, root, "AGENTS.md", "[automation](docs/automation.md)", "[automation](docs/missing.md)")
			},
			wantProblem: "broken local link",
		},
		{
			name: "referenced script is absent",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "scripts/policy/check-schema-catalog.sh")); err != nil {
					t.Fatal(err)
				}
			},
			wantProblem: "required referenced path scripts/policy/check-schema-catalog.sh",
		},
		{
			name: "make target is absent",
			mutate: func(t *testing.T, root string) {
				replaceFixtureText(t, root, "Makefile", "package:\n", "removed-package:\n")
			},
			wantProblem: `makefile is missing referenced target "package"`,
		},
		{
			name: "harness script is not executable",
			mutate: func(t *testing.T, root string) {
				path := filepath.Join(root, "scripts/policy/check-coding-agent-harness.sh")
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantProblem: "required referenced script scripts/policy/check-coding-agent-harness.sh is not executable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newHarnessFixture(t)
			test.mutate(t, root)
			problems := formatProblems(validate(root))
			if !strings.Contains(problems, test.wantProblem) {
				t.Fatalf("problems:\n%s\nwant substring %q", problems, test.wantProblem)
			}
		})
	}
}

func newHarnessFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	for _, contract := range guideContracts {
		var content strings.Builder
		content.WriteString("# Fixture\n\n")
		for _, required := range contract.required {
			content.WriteString(required)
			content.WriteByte('\n')
		}
		if contract.path == "AGENTS.md" {
			content.WriteString("[coding](docs/coding-agent-guide.md)\n")
			content.WriteString("[schema](docs/schema-contributor-guide.md)\n")
			content.WriteString("[automation](docs/automation.md)\n")
			content.WriteString("[agent code](docs/agent-code.md)\n")
		}
		writeFixtureFile(t, root, contract.path, content.String())
	}

	for _, path := range requiredPaths {
		writeFixtureFileMode(t, root, path, "fixture\n", 0o755)
	}
	var makefile strings.Builder
	for _, target := range requiredMakeTargets {
		makefile.WriteString(target)
		makefile.WriteString(":\n\t@true\n")
	}
	writeFixtureFile(t, root, "Makefile", makefile.String())
	return root
}

func writeFixtureFile(t *testing.T, root, path, content string) {
	t.Helper()
	writeFixtureFileMode(t, root, path, content, 0o644)
}

func writeFixtureFileMode(t *testing.T, root, path, content string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func replaceFixtureText(t *testing.T, root, path, old, replacement string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	content, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	updated := strings.Replace(string(content), old, replacement, 1)
	if updated == string(content) {
		t.Fatalf("fixture %s does not contain %q", path, old)
	}
	if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
		t.Fatalf("update fixture %s: %v", path, err)
	}
}

func formatProblems(problems []error) string {
	var lines []string
	for _, problem := range problems {
		lines = append(lines, problem.Error())
	}
	return strings.Join(lines, "\n")
}

func writeTaskFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write task fixture: %v", err)
	}
}

const validTaskContract = `# Task

Task kind: bug
Goal (one primary outcome): Fix retry handling
Current behavior and evidence: The focused test reproduces two retries.
Acceptance criteria:
- The retry succeeds once and then stops.
In scope (packages/files/surfaces): internal/transport
Out of scope: Authentication behavior
Compatibility constraints: Preserve existing flags and output
Interface impact (commands/flags/output/errors/exit codes/Schema): None
Safety or data-mutation constraints: No external writes
Expected validation: Focused unit test and make test
Known environment limitations: Live service unavailable
`

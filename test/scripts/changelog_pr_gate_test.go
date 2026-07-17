package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type changelogGateRepo struct {
	root string
	gate string
	base string
}

const changelogGateBase = `# Changelog

## [Unreleased]

## [1.0.0] - 2026-07-01

### Added

- Initial release.
`

func newChangelogGateRepo(t *testing.T) *changelogGateRepo {
	t.Helper()

	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	root := t.TempDir()

	for _, path := range []string{
		"LICENSE",
		"NOTICE",
		"README.md",
		"CONTRIBUTING.md",
		"SECURITY.md",
		"CODE_OF_CONDUCT.md",
		".env.example",
		".github/workflows/ci.yml",
		".github/PULL_REQUEST_TEMPLATE.md",
		"docs/architecture.md",
		"scripts/README.md",
		"build/README.md",
	} {
		changelogGateWrite(t, root, path, "fixture\n", 0o644)
	}
	for _, path := range []string{
		"scripts/policy/check-changelog-pr.sh",
		"scripts/policy/open-source-audit.sh",
		"scripts/release/release-lib.sh",
	} {
		data, err := os.ReadFile(filepath.Join(sourceRoot, path))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		changelogGateWrite(t, root, path, string(data), mode)
	}
	changelogGateWrite(t, root, "CHANGELOG.md", changelogGateBase, 0o644)

	changelogGateGit(t, root, "init", "-b", "main")
	changelogGateGit(t, root, "config", "user.name", "Changelog Gate Test")
	changelogGateGit(t, root, "config", "user.email", "changelog-gate@example.com")
	changelogGateGit(t, root, "add", ".")
	changelogGateGit(t, root, "commit", "-m", "seed repository")

	return &changelogGateRepo{
		root: root,
		gate: filepath.Join(root, "scripts", "policy", "check-changelog-pr.sh"),
		base: strings.TrimSpace(changelogGateGit(t, root, "rev-parse", "HEAD")),
	}
}

func changelogGateWrite(t *testing.T, root, path, content string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", full, err)
	}
}

func changelogGateGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s error = %v\noutput:\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func (r *changelogGateRepo) commit(t *testing.T, message string) {
	t.Helper()
	changelogGateGit(t, r.root, "add", "-A")
	changelogGateGit(t, r.root, "commit", "-m", message)
}

func (r *changelogGateRepo) run(t *testing.T) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", r.gate, r.base, "HEAD")
	cmd.Dir = r.root
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestChangelogPRGateAcceptsTargetedChanges(t *testing.T) {
	tests := []struct {
		name      string
		changelog string
	}{
		{
			name: "new release section with lowercase todo product",
			changelog: `# Changelog

## [Unreleased]

## [1.0.1-beta.1] - 2026-07-17

### Changed

- Improve the lowercase todo command family without leaving a placeholder.

## [1.0.0] - 2026-07-01

### Added

- Initial release.
`,
		},
		{
			name: "unreleased note",
			changelog: `# Changelog

## [Unreleased]

### Changed

- Document the next release candidate.

## [1.0.0] - 2026-07-01

### Added

- Initial release.
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newChangelogGateRepo(t)
			changelogGateWrite(t, repo.root, "CHANGELOG.md", test.changelog, 0o644)
			repo.commit(t, test.name)

			output, err := repo.run(t)
			if err != nil {
				t.Fatalf("gate error = %v\noutput:\n%s", err, output)
			}
			if !strings.Contains(output, "CHANGELOG PR check: ok") {
				t.Fatalf("gate output missing success marker:\n%s", output)
			}
		})
	}
}

func TestReleaseChangelogExtractionAllowsLowercaseTodoProductName(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	changelog := filepath.Join(t.TempDir(), "CHANGELOG.md")
	changelogGateWrite(t, filepath.Dir(changelog), filepath.Base(changelog), `# Changelog

## [1.0.1-beta.1] - 2026-07-17

### Changed

- Improve the lowercase todo command family.
`, 0o644)

	cmd := exec.Command(
		"sh",
		"-c",
		`. "$1"; release_extract_changelog "$2" 1.0.1-beta.1 -`,
		"sh",
		filepath.Join(sourceRoot, "scripts", "release", "release-lib.sh"),
		changelog,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release_extract_changelog error = %v\noutput:\n%s", err, output)
	}
}

func TestChangelogPRFastPathWorkflowContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	workflows := map[string][]string{
		".github/workflows/ci.yml": {
			"name: Classify PR changes",
			"name: Changelog Check",
			`run: ./scripts/policy/check-changelog-pr.sh "$PR_BASE_SHA" "$PR_HEAD_SHA"`,
			"expected_heavy_result=skipped",
			"name: CI Gate",
		},
		".github/workflows/multi-profile-e2e.yml": {
			"branches:",
			"- main",
			"Record changelog-only fast path",
			"Full E2E: runs after merge",
		},
	}
	classifierContract := []string{
		"files.length === 1",
		"files[0].filename === 'CHANGELOG.md'",
		"files[0].status === 'modified'",
		"!files[0].previous_filename",
	}

	for path, required := range workflows {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		workflow := string(data)
		for _, want := range append(classifierContract, required...) {
			if !strings.Contains(workflow, want) {
				t.Errorf("%s missing fast-path contract %q", path, want)
			}
		}
		if strings.Contains(workflow, "paths-ignore:") {
			t.Errorf("%s must not skip the required workflow with paths-ignore", path)
		}
	}
}

func TestChangelogPRGateRejectsUnsafeChanges(t *testing.T) {
	validRelease := `# Changelog

## [Unreleased]

## [1.0.1-beta.1] - 2026-07-17

### Changed

- Valid release note.

## [1.0.0] - 2026-07-01

### Added

- Initial release.
`
	tests := []struct {
		name       string
		changelog  string
		mutate     func(*testing.T, *changelogGateRepo)
		wantOutput string
	}{
		{
			name:      "second changed file",
			changelog: validRelease,
			mutate: func(t *testing.T, repo *changelogGateRepo) {
				changelogGateWrite(t, repo.root, "extra.txt", "extra\n", 0o644)
			},
			wantOutput: "exactly one in-place modification",
		},
		{
			name: "invalid calendar date",
			changelog: strings.Replace(
				validRelease,
				"2026-07-17",
				"2026-02-30",
				1,
			),
			wantOutput: "invalid calendar date",
		},
		{
			name: "duplicate release heading",
			changelog: strings.Replace(
				validRelease,
				"## [1.0.0] - 2026-07-01",
				"## [1.0.1-beta.1] - 2026-07-17\n\n- Duplicate.\n\n## [1.0.0] - 2026-07-01",
				1,
			),
			wantOutput: "exactly one well-formed section",
		},
		{
			name: "missing bullet",
			changelog: strings.Replace(
				validRelease,
				"- Valid release note.",
				"Valid release note.",
				1,
			),
			wantOutput: "at least one bullet",
		},
		{
			name: "placeholder",
			changelog: strings.Replace(
				validRelease,
				"- Valid release note.",
				"- TODO: write release notes.",
				1,
			),
			wantOutput: "must not contain TODO/TBD",
		},
		{
			name: "malformed duplicate unreleased heading",
			changelog: strings.Replace(
				validRelease,
				"## [Unreleased]",
				"## [Unreleased]\n\n## [Unreleased] junk",
				1,
			),
			wantOutput: "exactly one heading",
		},
		{
			name: "preamble change",
			changelog: strings.Replace(
				validRelease,
				"# Changelog",
				"# Release history",
				1,
			),
			wantOutput: "only permits notes inside",
		},
		{
			name:      "rename changelog",
			changelog: changelogGateBase,
			mutate: func(t *testing.T, repo *changelogGateRepo) {
				if err := os.Rename(
					filepath.Join(repo.root, "CHANGELOG.md"),
					filepath.Join(repo.root, "CHANGES.md"),
				); err != nil {
					t.Fatalf("Rename CHANGELOG.md error = %v", err)
				}
			},
			wantOutput: "exactly one in-place modification",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newChangelogGateRepo(t)
			changelogGateWrite(t, repo.root, "CHANGELOG.md", test.changelog, 0o644)
			if test.mutate != nil {
				test.mutate(t, repo)
			}
			repo.commit(t, test.name)

			output, err := repo.run(t)
			if err == nil {
				t.Fatalf("unsafe change unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(output, test.wantOutput) {
				t.Fatalf("gate output missing %q:\n%s", test.wantOutput, output)
			}
		})
	}
}

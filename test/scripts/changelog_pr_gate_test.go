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

const changelogGateValidRelease = `# Changelog

## [Unreleased]

## [1.0.1-beta.1] - 2026-07-17

### Changed

- Valid release note.

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
	return r.runMode(t, "--fast-path")
}

func (r *changelogGateRepo) runMode(t *testing.T, mode string) (string, error) {
	t.Helper()
	return r.runRefs(t, mode, r.base, "HEAD")
}

func (r *changelogGateRepo) runRefs(t *testing.T, mode, base, head string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", r.gate, mode, base, head)
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

func TestChangelogPRContentOnlyAllowsOtherFiles(t *testing.T) {
	repo := newChangelogGateRepo(t)
	changelogGateWrite(t, repo.root, "CHANGELOG.md", changelogGateValidRelease, 0o644)
	changelogGateWrite(t, repo.root, "internal/change.go", "package internal\n", 0o644)
	repo.commit(t, "change code with release notes")

	output, err := repo.runMode(t, "--content-only")
	if err != nil {
		t.Fatalf("content-only gate error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "CHANGELOG PR check: ok (mode=content-only") {
		t.Fatalf("content-only gate output missing success marker:\n%s", output)
	}
}

func TestChangelogPRContentOnlyStillValidatesContentWithOtherFiles(t *testing.T) {
	tests := []struct {
		name       string
		changelog  string
		wantOutput string
	}{
		{
			name: "invalid calendar date",
			changelog: strings.Replace(
				changelogGateValidRelease,
				"2026-07-17",
				"2026-02-30",
				1,
			),
			wantOutput: "invalid calendar date",
		},
		{
			name: "placeholder",
			changelog: strings.Replace(
				changelogGateValidRelease,
				"- Valid release note.",
				"- TODO: write release notes.",
				1,
			),
			wantOutput: "must not contain TODO/TBD",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newChangelogGateRepo(t)
			changelogGateWrite(t, repo.root, "CHANGELOG.md", test.changelog, 0o644)
			changelogGateWrite(t, repo.root, "internal/change.go", "package internal\n", 0o644)
			repo.commit(t, test.name)

			output, err := repo.runMode(t, "--content-only")
			if err == nil {
				t.Fatalf("unsafe content-only change unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(output, test.wantOutput) {
				t.Fatalf("content-only gate output missing %q:\n%s", test.wantOutput, output)
			}
		})
	}
}

func TestChangelogPRGateValidatesSyntheticMergeTree(t *testing.T) {
	repo := newChangelogGateRepo(t)
	commonChangelog := `# Changelog

## [Unreleased]

## [1.0.0] - 2026-07-01

### Added

- Initial release.

## [0.9.0] - 2026-06-01

### Added

- Earlier release.
`
	changelogGateWrite(t, repo.root, "CHANGELOG.md", commonChangelog, 0o644)
	repo.commit(t, "expand changelog history")
	common := strings.TrimSpace(changelogGateGit(t, repo.root, "rev-parse", "HEAD"))
	repo.base = common

	changelogGateGit(t, repo.root, "switch", "-c", "feature")
	featureChangelog := strings.Replace(
		commonChangelog,
		"## [1.0.0] - 2026-07-01",
		`## [1.0.1-beta.1] - 2026-07-17

### Changed

- Feature branch release note.

## [1.0.0] - 2026-07-01`,
		1,
	)
	changelogGateWrite(t, repo.root, "CHANGELOG.md", featureChangelog, 0o644)
	repo.commit(t, "add feature release note")
	featureHead := strings.TrimSpace(changelogGateGit(t, repo.root, "rev-parse", "HEAD"))
	if output, err := repo.runRefs(t, "--fast-path", common, featureHead); err != nil {
		t.Fatalf("feature head should be valid before merging: %v\n%s", err, output)
	}

	changelogGateGit(t, repo.root, "switch", "main")
	mainChangelog := strings.Replace(
		commonChangelog,
		"## [0.9.0] - 2026-06-01",
		`## [1.0.1-beta.1] - 2026-07-17

### Changed

- Main branch release note.

## [0.9.0] - 2026-06-01`,
		1,
	)
	changelogGateWrite(t, repo.root, "CHANGELOG.md", mainChangelog, 0o644)
	repo.commit(t, "add main release note")
	mergeBase := strings.TrimSpace(changelogGateGit(t, repo.root, "rev-parse", "HEAD"))
	changelogGateGit(t, repo.root, "merge", "--no-ff", "feature", "-m", "merge feature")
	mergeHead := strings.TrimSpace(changelogGateGit(t, repo.root, "rev-parse", "HEAD"))

	output, err := repo.runRefs(t, "--fast-path", mergeBase, mergeHead)
	if err == nil {
		t.Fatalf("synthetic merge with duplicate release heading unexpectedly passed:\n%s", output)
	}
	if !strings.Contains(output, "exactly one well-formed section") {
		t.Fatalf("synthetic merge rejection missing duplicate-section evidence:\n%s", output)
	}
}

func TestChangelogPRGateRejectsExecutableModeInBothModes(t *testing.T) {
	for _, mode := range []string{"--fast-path", "--content-only"} {
		t.Run(strings.TrimPrefix(mode, "--"), func(t *testing.T) {
			repo := newChangelogGateRepo(t)
			changelogGateWrite(t, repo.root, "CHANGELOG.md", changelogGateValidRelease, 0o644)
			changelogGateGit(t, repo.root, "add", "CHANGELOG.md")
			changelogGateGit(t, repo.root, "update-index", "--chmod=+x", "CHANGELOG.md")
			changelogGateGit(t, repo.root, "commit", "-m", "make changelog executable")

			output, err := repo.runMode(t, mode)
			if err == nil {
				t.Fatalf("executable CHANGELOG unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(output, "regular 100644 blob at head") {
				t.Fatalf("gate output missing regular-file rejection:\n%s", output)
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

func TestInterfaceIntegrityWorkflowContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	workflow := string(data)
	interfaceStart := strings.Index(workflow, "\n  interface-integrity:\n")
	interfaceEnd := strings.Index(workflow, "\n  cli-smoke:\n")
	if interfaceStart < 0 || interfaceEnd <= interfaceStart {
		t.Fatal("Code Admission workflow missing Interface Integrity job boundaries")
	}
	interfaceJob := workflow[interfaceStart:interfaceEnd]
	for _, want := range []string{
		"name: Check historical commands, help, and complete CLI compatibility",
		"make authoritative-interface-integrity",
		`BASE_REF="$COMPATIBILITY_BASE_REF"`,
		`STABLE_REF="$COMPATIBILITY_STABLE_REF"`,
		`CANDIDATE_REF="$COMPATIBILITY_CANDIDATE_REF"`,
	} {
		if !strings.Contains(interfaceJob, want) {
			t.Errorf("Interface Integrity job missing release-equivalent compatibility contract %q", want)
		}
	}
	for _, want := range []string{
		`candidate_ref="$(git rev-parse 'HEAD^{commit}')"`,
		`[ "$candidate_ref" != "$PR_HEAD_SHA" ]`,
		`"COMPATIBILITY_BASE_REF=$base_ref"`,
		`"COMPATIBILITY_STABLE_REF=$stable_ref"`,
		`"COMPATIBILITY_CANDIDATE_REF=$candidate_ref" >> "$GITHUB_ENV"`,
	} {
		if got := strings.Count(interfaceJob, want); got != 1 {
			t.Errorf("Interface Integrity job contract %q count = %d, want exactly one definition", want, got)
		}
	}
	if got := strings.Count(interfaceJob, `>> "$GITHUB_ENV"`); got != 1 {
		t.Errorf("Interface Integrity job must append its compatibility refs to GITHUB_ENV once, got %d writes", got)
	}
	if strings.Count(interfaceJob, "make authoritative-interface-integrity") != 1 {
		t.Errorf("Interface Integrity job must have exactly one compatibility decision seam")
	}
	if strings.Contains(interfaceJob, "./scripts/policy/check-command-compatibility.sh") {
		t.Error("Interface Integrity job must not bypass the authoritative Make seam")
	}
	if strings.Contains(interfaceJob, "check-interface-baseline.sh") {
		t.Error("Interface Integrity job must not reintroduce the legacy fixture checker")
	}

	schemaStart := strings.Index(interfaceJob, "\n      - name: Check complete Schema compatibility\n")
	if schemaStart < 0 {
		t.Fatal("Interface Integrity job missing complete Schema compatibility step")
	}
	schemaRemainder := interfaceJob[schemaStart+1:]
	schemaEnd := strings.Index(schemaRemainder, "\n      - name:")
	if schemaEnd < 0 {
		t.Fatal("complete Schema compatibility step has no workflow boundary")
	}
	schemaStep := schemaRemainder[:schemaEnd]
	for _, want := range []string{
		"make schema-compatibility",
		`BASE_REF="$COMPATIBILITY_BASE_REF"`,
		`STABLE_REF="$COMPATIBILITY_STABLE_REF"`,
		`CANDIDATE_REF="$COMPATIBILITY_CANDIDATE_REF"`,
	} {
		if !strings.Contains(schemaStep, want) {
			t.Errorf("Schema compatibility step missing authoritative contract %q", want)
		}
	}
}

func TestLocalInterfaceIntegrityUsesAuthoritativeSeam(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("ReadFile(Makefile) error = %v", err)
	}
	makefile := string(data)
	interfaceStart := strings.Index(makefile, "\ninterface-integrity:\n")
	authoritativeStart := strings.Index(makefile, "\nauthoritative-interface-integrity:\n")
	coverageStart := strings.Index(makefile, "\ncoverage-gate:\n")
	if interfaceStart < 0 || authoritativeStart <= interfaceStart || coverageStart <= authoritativeStart {
		t.Fatal("Makefile missing interface-integrity target boundaries")
	}
	interfaceTarget := makefile[interfaceStart:authoritativeStart]
	for _, want := range []string{
		"./scripts/policy/check-authoritative-interface-baselines.sh",
		`base_ref="origin/main"`,
		`candidate_ref="HEAD"`,
		`--base-ref "$$base_ref"`,
		`--stable-ref "$(STABLE_REF)"`,
		`--candidate-ref "$$candidate_ref"`,
	} {
		if !strings.Contains(interfaceTarget, want) {
			t.Errorf("interface-integrity target missing authoritative contract %q", want)
		}
	}
	if strings.Contains(interfaceTarget, "check-interface-baseline.sh") {
		t.Error("interface-integrity target still invokes the legacy fixture checker")
	}
	authoritativeTarget := makefile[authoritativeStart:coverageStart]
	for _, want := range []string{
		"./scripts/policy/check-authoritative-interface-baselines.sh",
		`candidate_ref="HEAD"`,
		`--base-ref "$(BASE_REF)"`,
		`--stable-ref "$(STABLE_REF)"`,
		`--candidate-ref "$$candidate_ref"`,
	} {
		if !strings.Contains(authoritativeTarget, want) {
			t.Errorf("authoritative-interface-integrity target missing contract %q", want)
		}
	}

	if got := strings.Count(makefile, "./scripts/policy/check-interface-baseline.sh"); got != 2 {
		t.Fatalf("legacy fixture helper invocation count = %d, want update/reset only", got)
	}
	for _, target := range []string{"update-interface-baseline", "reset-interface-baseline"} {
		marker := "\n" + target + ":\n"
		start := strings.Index(makefile, marker)
		if start < 0 {
			t.Fatalf("Makefile missing %s target", target)
		}
		end := strings.Index(makefile[start+len(marker):], "\n\n")
		if end < 0 {
			t.Fatalf("Makefile %s target has no boundary", target)
		}
		block := makefile[start : start+len(marker)+end]
		if !strings.Contains(block, "./scripts/policy/check-interface-baseline.sh") {
			t.Errorf("%s target must retain the legacy fixture helper", target)
		}
	}
}

func TestLocalSchemaCompatibilityUsesAuthoritativeCandidateSeam(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("ReadFile(Makefile) error = %v", err)
	}
	makefile := string(data)
	schemaStart := strings.Index(makefile, "\nschema-compatibility:\n")
	skillStart := strings.Index(makefile, "\nskill-command-integrity:\n")
	if schemaStart < 0 || skillStart <= schemaStart {
		t.Fatal("Makefile missing schema-compatibility target boundaries")
	}
	schemaTarget := makefile[schemaStart:skillStart]
	for _, want := range []string{
		"./scripts/policy/check-authoritative-schema-compatibility.sh",
		`candidate_ref="HEAD"`,
		`--base-ref "$(BASE_REF)"`,
		`--stable-ref "$(STABLE_REF)"`,
		`--candidate-ref "$$candidate_ref"`,
	} {
		if !strings.Contains(schemaTarget, want) {
			t.Errorf("schema-compatibility target missing authoritative contract %q", want)
		}
	}
}

func TestInterfaceSensitiveClassificationCoversCoreCommandFramework(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	workflow := string(data)
	start := strings.Index(workflow, "            const isInterfaceSensitive =")
	end := strings.Index(workflow, "            const isMCPSensitive =")
	if start < 0 || end <= start {
		t.Fatal("Code Admission workflow missing interface-sensitive classifier boundaries")
	}
	classifier := strings.TrimSpace(workflow[start:end])
	probe := classifier + `
if (!isInterfaceSensitive("internal/corecmd/corecmd.go")) {
  throw new Error("existing corecmd files must trigger Interface Integrity");
}
`
	cmd := exec.Command("node", "-e", probe)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("interface-sensitive classifier rejected corecmd change: %v\n%s", err, output)
	}
}

func TestHighRiskClassificationShardsHelperChanges(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	workflow := string(data)
	start := strings.Index(workflow, "            const isDocsOnly =")
	end := strings.Index(workflow, "            const classifyFiles =")
	if start < 0 || end <= start {
		t.Fatal("Code Admission workflow missing high-risk classifier boundaries")
	}
	classifier := strings.TrimSpace(workflow[start:end])
	probe := classifier + `
if (!isHighRisk("internal/helpers/minutes.go")) {
  throw new Error("helper changes must use the sharded full suite");
}
if (isHighRisk("internal/helpersx/minutes.go")) {
  throw new Error("helper high-risk classification must respect the path boundary");
}
`
	cmd := exec.Command("node", "-e", probe)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("high-risk classifier rejected helper sharding contract: %v\n%s", err, output)
	}
}

func TestChangelogPRFastPathWorkflowContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	readWorkflow := func(path string) string {
		t.Helper()
		data, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, readErr)
		}
		return string(data)
	}

	admission := readWorkflow(".github/workflows/ci.yml")
	for _, want := range []string{
		"name: CI",
		"files.length === 1",
		"files[0].filename === 'CHANGELOG.md'",
		"files[0].status === 'modified'",
		"!files[0].previous_filename",
		"pre-classification",
		"post-classification",
		"before.changed_files !== files.length",
		"after.changed_files !== files.length",
		"context.ref !== 'refs/heads/main'",
		"context.payload.ref !== 'refs/heads/main'",
		"context.payload.created !== false",
		"context.payload.deleted !== false",
		"context.payload.forced !== false",
		"expectedAfter !== context.sha",
		"github.rest.repos.compareCommitsWithBasehead",
		"comparison.merge_base_commit?.sha === expectedBefore",
		"comparison.behind_by === 0",
		"comparison.total_commits === comparison.ahead_by",
		"run.app?.slug !== 'github-actions'",
		"predecessor Code Admission is not fully successful",
		`test "$(git rev-parse HEAD^1)" = "$PR_BASE_SHA"`,
		`test "$(git rev-parse HEAD^2)" = "$PR_HEAD_SHA"`,
		"Files API and synthetic merge tree disagree on CHANGELOG scope",
		"mode=--content-only",
		"mode=--fast-path",
		`"$mode" "$PR_BASE_SHA" HEAD`,
		`test "$(git rev-parse HEAD)" = "$PUSH_AFTER_SHA"`,
		`--fast-path "$PUSH_BEFORE_SHA" "$PUSH_AFTER_SHA"`,
		"needs.lint.outputs.platform_sensitive == 'true'",
		`COVERAGE_TARGET: "100"`,
		`COVERAGE_ENFORCE_OVERALL: "false"`,
		`COVERAGE_OVERALL_TOLERANCE: "0.1"`,
		`additional_profile=coverage-shortcut.txt`,
		"run: make test-plan",
		"run: make format-check",
		"./scripts/ci/test-packages.sh list",
		`package_output="$(./scripts/ci/test-packages.sh list`,
		"docs_only: ${{ steps.classify.outputs.docs_only }}",
		"full_suite: ${{ steps.classify.outputs.full_suite }}",
		"classifyFiles(files.length < 300)",
		"platformSensitive =",
		"files.some(isNativeGoChange)",
		"paths.some(isEditionSensitive)",
		`/_(darwin|windows|linux|unix)(?:_[a-z0-9]+)*\.go$/`,
		"filename.startsWith('build/')",
		"filename === '.github/actionlint.yaml'",
		"filename.startsWith('scripts/')",
		"filename.startsWith('verify/')",
		"filename.startsWith('internal/helpers/')",
		"filename === 'test/fixtures/cli-interface-baseline.txt'",
		"filename.startsWith('internal/interfacesnapshot/')",
		"filename.startsWith('internal/cobracmd/')",
		"filename.startsWith('internal/i18n/')",
		"filename.startsWith('pkg/cmdutil/')",
		"filename === 'skills_embed.go'",
		"filename.startsWith('test/mock_mcp/')",
		"name: Test (changed packages)",
		"changed-test-packages.sh",
		"Verify authoritative synthetic merge",
		`test "$(git rev-parse HEAD^1)" = "$PR_BASE_SHA"`,
		`test "$(git rev-parse HEAD^2)" = "$PR_HEAD_SHA"`,
		`echo "TEST_HEAD_REF=$(git rev-parse HEAD)"`,
		"list \"$TEST_BASE_REF\" \"$TEST_HEAD_REF\"",
		"needs.lint.outputs.full_suite != 'true'",
		`name: "Test (race: ${{ matrix.shard }})"`,
		"name: Test (workflow and release contracts)",
		"needs.lint.outputs.full_suite == 'true' || needs.lint.outputs.release_sensitive == 'true'",
		"needs.lint.outputs.full_suite == 'true'",
		"-coverpkg=\"$coverpkg\"",
		"changed \"$COVERAGE_BASE_REF\" \"$COVERAGE_HEAD_REF\"",
		"list \"$COVERAGE_BASE_REF\" \"$COVERAGE_HEAD_REF\"",
		"current_expected=success",
		"supporting_expected=skipped",
		"baseline_expected=success",
		"Validate scoped policy",
		"Interface Integrity is unaffected by this classified revision.",
		"CLI Smoke is unaffected by this classified revision.",
		"Mock MCP is unaffected by this classified revision.",
		"Edition is unaffected by this classified revision.",
	} {
		if !strings.Contains(admission, want) {
			t.Errorf("Code Admission workflow missing contract %q", want)
		}
	}
	for _, context := range []string{
		"Lint",
		"Test",
		"Coverage",
		"Policy",
		"Edition",
		"Interface Integrity",
		"CLI Smoke",
		"Mock MCP",
	} {
		if !strings.Contains(admission, "\n    name: "+context+"\n") {
			t.Errorf("Code Admission workflow missing exact context %q", context)
		}
	}
	for _, forbidden := range []string{
		"name: CI Gate",
		"name: Changelog Check",
		"name: Policy Check",
		"name: Edition Contract Tests",
		"name: Mock MCP Smoke",
	} {
		if strings.Contains(admission, forbidden) {
			t.Errorf("Code Admission workflow retains legacy context %q", forbidden)
		}
	}
	if strings.Contains(admission, "paths-ignore:") {
		t.Error("Code Admission must not suppress required contexts with paths-ignore")
	}

	focusedStart := strings.Index(admission, "\n  test-focused:\n")
	focusedEnd := strings.Index(admission, "\n  test-race:\n")
	if focusedStart < 0 || focusedEnd <= focusedStart {
		t.Fatal("Code Admission workflow missing focused test job boundaries")
	}
	focusedJob := admission[focusedStart:focusedEnd]
	if !strings.Contains(focusedJob, "timeout-minutes: 20") {
		t.Error("focused test job must allow the scoped race suite up to 20 minutes")
	}
	if !strings.Contains(focusedJob, `go test -v -race -count=1 -timeout=15m "${packages[@]}"`) {
		t.Error("focused race tests must retain enough package-level time for internal/app")
	}

	raceStart := focusedEnd
	raceEnd := strings.Index(admission, "\n  test-release-scripts:\n")
	if raceEnd <= raceStart {
		t.Fatal("Code Admission workflow missing race test job boundaries")
	}
	raceJob := admission[raceStart:raceEnd]
	// Full race shards use a dynamic package-level timeout: default/floor 12m,
	// with cli/smoke raised to 15m for NewRootCommand / Schema assembly under -race.
	for _, want := range []string{
		"timeout_budget=12m",
		`if [ "$TEST_SHARD" = "cli" ] || [ "$TEST_SHARD" = "smoke" ]; then`,
		"timeout_budget=15m",
		`go test -v -race -count=1 -timeout="$timeout_budget" "${packages[@]}"`,
		"- smoke",
	} {
		if !strings.Contains(raceJob, want) {
			t.Errorf("full race shards must retain dynamic timeout budget contract %q", want)
		}
	}

	darwinStart := strings.Index(admission, "\n  test-darwin:\n")
	darwinEnd := strings.Index(admission, "\n  test-windows:\n")
	if darwinStart < 0 || darwinEnd <= darwinStart {
		t.Fatal("Code Admission workflow missing macOS test job boundaries")
	}
	darwinJob := admission[darwinStart:darwinEnd]
	// The macOS job no longer runs ./internal/app as a whole package, so it does
	// not need the package-level race budget the Ubuntu shard gets — the Ubuntu
	// "race: app" shard already covers everything except the natively-gated
	// tests. Pinning the two focused commands replaces that budget assertion:
	// it locks the per-step timeouts and blocks a whole-package regression.
	for _, want := range []string{
		`go test -v -race -count=1 -timeout=6m ./internal/keychain ./internal/auth`,
		`go test -v -race -count=1 -timeout=5m ./internal/app -run '^(TestValidateNewBinary_RecoversFromUnsignedDarwin|Test(CrossPlatformCoverage)?Auth(MigrateKeychain|StatusDiagnosticReportsCiphertextKeyMismatch))'`,
	} {
		if !strings.Contains(darwinJob, want) {
			t.Errorf("macOS native test job missing focused auth contract %q", want)
		}
	}
	if strings.Contains(darwinJob, "./internal/keychain ./internal/auth ./internal/app") {
		t.Error("macOS native test job must not repeat the complete internal/app race shard")
	}
	if count := strings.Count(darwinJob, "./internal/app"); count != 1 {
		t.Errorf("macOS native test job internal/app invocation count = %d, want 1 focused invocation", count)
	}

	coverageStart := strings.Index(admission, "\n  coverage:\n")
	coverageEnd := strings.Index(admission, "\n  policy:\n")
	if coverageStart < 0 || coverageEnd <= coverageStart {
		t.Fatal("Code Admission workflow missing coverage job boundaries")
	}
	coverageJob := admission[coverageStart:coverageEnd]
	for _, want := range []string{
		`FULL_SUITE: ${{ needs.lint.outputs.full_suite }}`,
		"policy_profile=coverage-policy.txt\n" +
			`          if [ "$FULL_SUITE" != true ]; then` + "\n" +
			"            policy_profile=\n" +
			"          fi",
		`COVERAGE_DIFF_PROFILE="$policy_profile"`,
	} {
		if !strings.Contains(coverageJob, want) {
			t.Errorf("Code Admission workflow missing scoped coverage contract %q", want)
		}
	}

	notification := readWorkflow(".github/workflows/notify-wukong.yml")
	if !strings.Contains(notification, "- CI") {
		t.Error("Wukong notification must follow the renamed CI workflow")
	}
	if strings.Contains(notification, "Code Admission — PR 合入门禁") {
		t.Error("Wukong notification still follows the retired workflow display name")
	}

	coverageGate := readWorkflow("scripts/policy/check-coverage-gate.sh")
	if !strings.Contains(coverageGate, `TARGET="${COVERAGE_TARGET:-100}"`) {
		t.Error("coverage gate must default to 100% changed-code coverage")
	}
	if !strings.Contains(coverageGate, `OVERALL_TOLERANCE="${COVERAGE_OVERALL_TOLERANCE:-0}"`) {
		t.Error("coverage gate must reject any reported overall regression")
	}
	if !strings.Contains(coverageGate, `DIFF_PROFILE="${COVERAGE_DIFF_PROFILE-coverage-policy.txt}"`) {
		t.Error("coverage gate must allow scoped CI to explicitly omit the supporting policy profile")
	}
	if !strings.Contains(coverageGate, `if [ -n "$DIFF_PROFILE" ]; then`) {
		t.Error("coverage gate must add the supporting policy profile only when configured")
	}
	if !strings.Contains(coverageGate, `--baseline-profile "$BASELINE_PROFILE"`) {
		t.Error("coverage gate must evaluate the merge-base profile with the candidate checker")
	}
	if strings.Contains(coverageGate, `--overall-profile "$ADDITIONAL_DIFF_PROFILE"`) {
		t.Error("supporting changed-code coverage must not inflate candidate overall coverage")
	}
	if strings.Contains(coverageGate, `go tool cover -func="$BASELINE_PROFILE"`) {
		t.Error("coverage baseline must not use a different coverage calculator")
	}

	aiBehavior := readWorkflow(".github/workflows/ai-behavior-check.yml")
	for _, want := range []string{
		"name: Code Admission — AI Behavior",
		"\n    name: AI Behavior\n",
		"context: 'AI Behavior'",
		"pull_request_target:",
		"pull.head.sha !== expectedHead",
		"pull.base.sha !== expectedBase",
	} {
		if !strings.Contains(aiBehavior, want) {
			t.Errorf("AI Behavior workflow missing contract %q", want)
		}
	}

	integration := readWorkflow(".github/workflows/multi-profile-e2e.yml")
	for _, want := range []string{
		"name: Main Integration — 主干集成",
		"\n    name: Multi-profile E2E\n",
		"branches:",
		"- main",
		"workflow_dispatch:",
	} {
		if !strings.Contains(integration, want) {
			t.Errorf("main integration workflow missing contract %q", want)
		}
	}
	if strings.Contains(integration, "pull_request:") {
		t.Error("complete Multi-profile E2E must not run as a pull-request admission context")
	}
	if !strings.Contains(integration, "include-hidden-files: true") {
		t.Error("main integration must upload diagnostics stored below the hidden .tmp-bin directory")
	}

	integrationScript := readWorkflow("scripts/dev/test-multi-profile-e2e.sh")
	if !strings.Contains(integrationScript, `GO_TEST_TIMEOUT="${MULTI_PROFILE_GO_TEST_TIMEOUT:-10m}"`) {
		t.Error("multi-profile E2E must allow enough time for the complete internal/app regression suite")
	}
	if strings.Contains(integrationScript, "go test -timeout 180s") {
		t.Error("multi-profile E2E must not retain the obsolete three-minute Go test budget")
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
			wantOutput: "regular 100644 blob at head",
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

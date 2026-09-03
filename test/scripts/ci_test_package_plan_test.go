package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCITestPackagePlanCoversDefaultPackagesExactlyOnce(t *testing.T) {
	root := testPackagePlanRoot(t)
	output := runTestPackagePlan(t, root, "verify")
	if !strings.Contains(output, "default packages exactly once") {
		t.Fatalf("verify output = %q, want coverage summary", output)
	}
	if !strings.Contains(output, "full-suite packages exactly once") {
		t.Fatalf("verify output = %q, want coverage shard plan summary", output)
	}
}

func TestCICoveragePackagePlanRoutesFullSuiteScope(t *testing.T) {
	root := testPackagePlanRoot(t)
	remaining := strings.Fields(runTestPackagePlan(t, root, "list-coverage", "remaining"))

	for _, suffix := range []string{
		"/cmd",
		"/internal/output",
		"/skills",
		"/scripts/build/runtime-payload",
	} {
		if !containsPackageSuffix(remaining, suffix) {
			t.Errorf("coverage remaining shard does not contain package ending in %q", suffix)
		}
	}
	for _, suffix := range []string{
		"/internal/app",
		"/internal/cli",
		"/internal/generator",
		"/internal/helpers",
		"/test/smoke",
		"/test/scripts",
		"/pkg/cmdutil",
		"/scripts/policy/coverage-gate",
	} {
		if containsPackageSuffix(remaining, suffix) {
			t.Errorf("coverage remaining shard unexpectedly contains package ending in %q", suffix)
		}
	}

	app := strings.Fields(runTestPackagePlan(t, root, "list-coverage", "app"))
	if !containsPackageSuffix(app, "/internal/app") {
		t.Error("coverage app shard does not contain /internal/app")
	}
}

func TestCITestPackagePlanRoutesPublicTestSuites(t *testing.T) {
	root := testPackagePlanRoot(t)
	remaining := strings.Fields(runTestPackagePlan(t, root, "list", "remaining"))
	smoke := strings.Fields(runTestPackagePlan(t, root, "list", "smoke"))
	releaseScripts := strings.Fields(runTestPackagePlan(t, root, "list", "release-scripts"))

	for _, suffix := range []string{
		"/test/cli",
		"/test/contract",
		"/test/integration/extensions",
		"/test/mock_mcp",
		"/test/unit",
	} {
		if !containsPackageSuffix(remaining, suffix) {
			t.Errorf("remaining shard does not contain package ending in %q", suffix)
		}
	}
	if containsPackageSuffix(remaining, "/test/smoke") {
		t.Error("remaining shard unexpectedly contains /test/smoke")
	}
	if containsPackageSuffix(remaining, "/test/scripts") {
		t.Error("remaining shard unexpectedly contains /test/scripts")
	}
	if !containsPackageSuffix(smoke, "/test/smoke") {
		t.Error("smoke shard does not contain /test/smoke")
	}
	if !containsPackageSuffix(releaseScripts, "/test/scripts") {
		t.Error("release-scripts shard does not contain /test/scripts")
	}
}

func TestCIAppRacePartitionsCoverTopLevelTestsExactlyOnce(t *testing.T) {
	root := testPackagePlanRoot(t)
	packages := strings.Fields(runTestPackagePlan(t, root, "list", "app"))
	if len(packages) != 1 {
		t.Fatalf("app package shard = %v, want exactly one package", packages)
	}

	script := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	cmd := exec.Command("sh", script, "verify", packages[0])
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s verify %s failed: %v\n%s", script, packages[0], err, output)
	}
	if !strings.Contains(string(output), "top-level tests exactly once") {
		t.Fatalf("verify output = %q, want exact coverage summary", output)
	}
}

func TestCIAppCoverageModeUsesOneNonRacePartition(t *testing.T) {
	root := testPackagePlanRoot(t)
	fakeBin := t.TempDir()
	writeFakeCoverageGo(t, fakeBin)
	argsLog := filepath.Join(t.TempDir(), "go-args.log")
	profile := filepath.Join(t.TempDir(), "coverage.txt")

	script := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	cmd := exec.Command(
		"sh",
		script,
		"coverage",
		"example.com/project/internal/app",
		"a-b",
		profile,
	)
	cmd.Dir = root
	cmd.Env = []string{
		"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMPDIR=" + t.TempDir(),
		"DWS_TEST_ROOT=" + root,
		"GO_ARGS_LOG=" + argsLog,
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s coverage failed: %v\n%s", script, err, output)
	}

	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read fake go args: %v", err)
	}
	command := string(args)
	for _, want := range []string{
		"-v",
		"-run ^Test[A-B]",
		"-skip ^Test.*Schema",
		"-coverprofile=" + profile,
		"-covermode=atomic",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("coverage command %q missing %q", command, want)
		}
	}
	if strings.Contains(command, "-race") {
		t.Errorf("coverage command must not enable race instrumentation: %q", command)
	}
}

func TestCIFullCoverageRunnerUsesBoundedAppProcesses(t *testing.T) {
	root := testPackagePlanRoot(t)
	targetRoot := t.TempDir()
	fakeBin := t.TempDir()
	writeFakeCoverageGo(t, fakeBin)
	argsLog := filepath.Join(t.TempDir(), "go-args.log")
	profile := filepath.Join(t.TempDir(), "coverage.txt")

	partitionScript := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	partitionCmd := exec.Command("sh", partitionScript, "list-partitions")
	partitionCmd.Dir = root
	partitionOutput, err := partitionCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list app partitions: %v\n%s", err, partitionOutput)
	}
	partitions := strings.Fields(string(partitionOutput))

	script := filepath.Join(root, "scripts", "ci", "run-full-coverage.sh")
	cmd := exec.Command("sh", script, "--root", targetRoot, "--output", profile)
	cmd.Dir = root
	cmd.Env = []string{
		"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMPDIR=" + t.TempDir(),
		"RUNNER_TEMP=" + t.TempDir(),
		"GO_ARGS_LOG=" + argsLog,
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", script, err, output)
	}

	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read fake go args: %v", err)
	}
	commands := strings.FieldsFunc(strings.TrimSpace(string(args)), func(r rune) bool { return r == '\n' })
	wantCommands := len(partitions) + 4
	if len(commands) != wantCommands {
		t.Fatalf("coverage test process count = %d, want %d; commands:\n%s", len(commands), wantCommands, args)
	}
	for _, command := range commands {
		if !strings.Contains(command, "-v") || strings.Contains(command, "-race") {
			t.Errorf("coverage command must be verbose and non-race: %q", command)
		}
	}

	profileData, err := os.ReadFile(profile)
	if err != nil {
		t.Fatalf("read merged profile: %v", err)
	}
	if got := strings.Count(string(profileData), "mode: atomic"); got != 1 {
		t.Errorf("merged profile mode headers = %d, want 1", got)
	}
	if got := strings.Count(string(profileData), "example.com/project/file.go:"); got != wantCommands {
		t.Errorf("merged profile blocks = %d, want %d", got, wantCommands)
	}
}

// TestCIAppPartitionMatricesMatchHelper pins the workflow's app test and
// coverage shards to the partition set the helper actually runs. The
// partitions are separate CI jobs, so the helper's own "covered exactly once"
// check cannot prove the whole package ran: a partition the helper knows about
// but no matrix shard dispatches would silently stop running while every job
// stays green. Both directions are asserted so a stale matrix shard fails too.
func TestCIAppPartitionMatricesMatchHelper(t *testing.T) {
	root := testPackagePlanRoot(t)
	script := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	cmd := exec.Command("sh", script, "list-partitions")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s list-partitions failed: %v\n%s", script, err, output)
	}
	partitions := strings.Fields(string(output))
	if len(partitions) == 0 {
		t.Fatalf("list-partitions returned no partitions: %q", output)
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	admission := string(workflow)

	for _, job := range []struct {
		name      string
		startMark string
		endMark   string
	}{
		{"test-focused", "\n  test-focused:\n", "\n  test-race:\n"},
		{"test-race", "\n  test-race:\n", "\n  test-release-scripts:\n"},
		{"coverage-current-full", "\n  coverage-current-full:\n", "\n  coverage-supporting:\n"},
	} {
		start := strings.Index(admission, job.startMark)
		end := strings.Index(admission, job.endMark)
		if start < 0 || end <= start {
			t.Fatalf("ci.yml is missing %s job boundaries", job.name)
		}
		body := admission[start:end]

		for _, partition := range partitions {
			want := "- app-" + partition
			if !strings.Contains(body, want) {
				t.Errorf("%s matrix is missing shard %q for a partition the helper runs", job.name, want)
			}
		}

		for _, line := range strings.Split(body, "\n") {
			shard := strings.TrimSpace(line)
			if !strings.HasPrefix(shard, "- app-") {
				continue
			}
			name := strings.TrimPrefix(shard, "- app-")
			matched := false
			for _, partition := range partitions {
				if partition == name {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("%s matrix shard %q has no matching helper partition", job.name, shard)
			}
		}
	}
}

func TestCITestPackagePlanFailsClosedWhenGoListFails(t *testing.T) {
	root := testPackagePlanRoot(t)
	fakeBin := t.TempDir()
	fakeGo := filepath.Join(fakeBin, "go")
	err := os.WriteFile(fakeGo, []byte(`#!/bin/sh
if [ "$1" = "list" ] && [ "$2" = "-m" ]; then
  printf '%s\n' 'github.com/DingTalk-Real-AI/dingtalk-workspace-cli'
  exit 0
fi
printf '%s\n' 'injected go list failure' >&2
exit 42
`), 0o755)
	if err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	script := filepath.Join(root, "scripts", "ci", "test-packages.sh")
	for _, args := range [][]string{{"list", "remaining"}, {"verify"}} {
		cmd := exec.Command("sh", append([]string{script}, args...)...)
		cmd.Dir = root
		cmd.Env = []string{
			"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
			"TMPDIR=" + t.TempDir(),
		}
		output, runErr := cmd.CombinedOutput()
		if runErr == nil {
			t.Fatalf("%s unexpectedly succeeded with failing go list:\n%s", strings.Join(args, " "), output)
		}
		if !strings.Contains(string(output), "injected go list failure") {
			t.Fatalf("%s failure output = %q, want injected failure", strings.Join(args, " "), output)
		}
	}
}

func testPackagePlanRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func runTestPackagePlan(t *testing.T, root string, args ...string) string {
	t.Helper()
	script := filepath.Join(root, "scripts", "ci", "test-packages.sh")
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", script, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func containsPackageSuffix(packages []string, suffix string) bool {
	for _, packagePath := range packages {
		if strings.HasSuffix(packagePath, suffix) {
			return true
		}
	}
	return false
}

func writeFakeCoverageGo(t *testing.T, binDir string) {
	t.Helper()

	const fakeGo = `#!/bin/sh
set -eu

case "${1:-}" in
  list)
    shift
    if [ "${1:-}" = "-m" ]; then
      printf '%s\n' 'example.com/project'
      exit 0
    fi
    case "$*" in
      './internal/app/...') printf '%s\n' 'example.com/project/internal/app' ;;
      './internal/cli/...') printf '%s\n' 'example.com/project/internal/cli' ;;
      './internal/generator/...') printf '%s\n' 'example.com/project/internal/generator' ;;
      './internal/helpers/...') printf '%s\n' 'example.com/project/internal/helpers' ;;
      './ ./cmd/... ./internal/... ./skills/... ./scripts/build/runtime-payload')
        printf '%s\n' \
          'example.com/project' \
          'example.com/project/cmd' \
          'example.com/project/internal/app' \
          'example.com/project/internal/cli' \
          'example.com/project/internal/generator' \
          'example.com/project/internal/helpers' \
          'example.com/project/skills' \
          'example.com/project/scripts/build/runtime-payload'
        ;;
      *)
        printf 'unexpected fake go list arguments: %s\n' "$*" >&2
        exit 2
        ;;
    esac
    ;;
  test)
    shift
    list=false
    profile=
    for argument in "$@"; do
      case "$argument" in
        -list) list=true ;;
        -coverprofile=*) profile="${argument#-coverprofile=}" ;;
      esac
    done
    if [ "$list" = true ]; then
      printf '%s\n' \
        TestSchemaContract \
        TestAlpha \
        TestCrossPlatformCoverageAlpha \
        TestCrossPlatformCoverageMike \
        TestCrossPlatformCoveragePapa \
        TestCrossPlatformCoverageZulu \
        TestCommand \
        TestDelta \
        TestZulu \
        'ok example.com/project/internal/app'
      exit 0
    fi
    [ -n "$profile" ]
    printf '%s\n' "$*" >> "${GO_ARGS_LOG:?}"
    printf '%s\n' \
      'mode: atomic' \
      'example.com/project/file.go:1.1,1.2 1 1' > "$profile"
    ;;
  tool)
    [ "${2:-}" = cover ]
    printf '%s\n' 'total: (statements) 100.0%'
    ;;
  *)
    printf 'unexpected fake go command: %s\n' "${1:-}" >&2
    exit 2
    ;;
esac
`
	fakeGoPath := filepath.Join(binDir, "go")
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}
}

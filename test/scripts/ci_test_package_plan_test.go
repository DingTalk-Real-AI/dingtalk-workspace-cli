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

	for _, suffix := range []string{"/cmd", "/internal/output", "/skills"} {
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

// TestCIAppRacePartitionMatrixMatchesHelper pins the workflow's app partition
// shards to the partition set the helper actually runs. The partitions are
// separate CI jobs now, so the helper's own "covered exactly once" check can no
// longer prove the whole package ran: a partition the helper knows about but no
// matrix shard dispatches would silently stop running while every job stays
// green. Both directions are asserted so a stale matrix shard fails too.
func TestCIAppRacePartitionMatrixMatchesHelper(t *testing.T) {
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

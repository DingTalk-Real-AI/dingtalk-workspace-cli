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
}

func TestCITestPackagePlanRoutesPublicTestSuites(t *testing.T) {
	root := testPackagePlanRoot(t)
	remaining := strings.Fields(runTestPackagePlan(t, root, "list", "remaining"))
	releaseScripts := strings.Fields(runTestPackagePlan(t, root, "list", "release-scripts"))

	for _, suffix := range []string{
		"/test/cli",
		"/test/contract",
		"/test/integration/extensions",
		"/test/mock_mcp",
		"/test/smoke",
		"/test/unit",
	} {
		if !containsPackageSuffix(remaining, suffix) {
			t.Errorf("remaining shard does not contain package ending in %q", suffix)
		}
	}
	if containsPackageSuffix(remaining, "/test/scripts") {
		t.Error("remaining shard unexpectedly contains /test/scripts")
	}
	if !containsPackageSuffix(releaseScripts, "/test/scripts") {
		t.Error("release-scripts shard does not contain /test/scripts")
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

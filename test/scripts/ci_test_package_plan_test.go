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

// TestCIAppRaceLaneMatrixMatchesHelper pins both workflow matrices to the three
// physical lanes declared by the helper and proves that those lanes contain
// every logical partition exactly once. A stale lane, dropped partition, or
// duplicate assignment must fail rather than silently weakening the race suite.
func TestCIAppRaceLaneMatrixMatchesHelper(t *testing.T) {
	root := testPackagePlanRoot(t)
	script := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	partitionsOutput := runAppRaceHelper(t, root, script, "list-partitions")
	partitions := strings.Fields(partitionsOutput)
	if len(partitions) == 0 {
		t.Fatalf("list-partitions returned no partitions: %q", partitionsOutput)
	}
	partitionSet := make(map[string]struct{}, len(partitions))
	for _, partition := range partitions {
		partitionSet[partition] = struct{}{}
	}

	lanesOutput := runAppRaceHelper(t, root, script, "list-lanes")
	lanes := strings.Fields(lanesOutput)
	if len(lanes) != 3 {
		t.Fatalf("list-lanes returned %d lanes, want 3: %q", len(lanes), lanesOutput)
	}
	assigned := make(map[string]string, len(partitions))
	for _, lane := range lanes {
		laneOutput := runAppRaceHelper(t, root, script, "list-lane-partitions", lane)
		lanePartitions := strings.Fields(laneOutput)
		if len(lanePartitions) == 0 {
			t.Fatalf("lane %q returned no partitions", lane)
		}
		for _, partition := range lanePartitions {
			if _, ok := partitionSet[partition]; !ok {
				t.Errorf("lane %q contains unknown partition %q", lane, partition)
				continue
			}
			if previous, ok := assigned[partition]; ok {
				t.Errorf("partition %q appears in lanes %q and %q", partition, previous, lane)
				continue
			}
			assigned[partition] = lane
		}
	}
	for _, partition := range partitions {
		if _, ok := assigned[partition]; !ok {
			t.Errorf("partition %q is not assigned to an app race lane", partition)
		}
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

		for _, lane := range lanes {
			want := "- app-" + lane
			if !strings.Contains(body, want) {
				t.Errorf("%s matrix is missing helper lane shard %q", job.name, want)
			}
		}

		for _, line := range strings.Split(body, "\n") {
			shard := strings.TrimSpace(line)
			if !strings.HasPrefix(shard, "- app-") {
				continue
			}
			name := strings.TrimPrefix(shard, "- app-")
			matched := false
			for _, lane := range lanes {
				if lane == name {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("%s matrix shard %q has no matching helper lane", job.name, shard)
			}
		}
	}
}

func TestCIAppRaceLaneRunsEachPartitionInFreshGoTestProcess(t *testing.T) {
	root := testPackagePlanRoot(t)
	fakeBin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "go-test.log")
	fakeGo := filepath.Join(fakeBin, "go")
	const fakeGoScript = `#!/bin/sh
case " $* " in
  *" -list "*)
    printf '%s\n' \
      TestAgentSchema \
      TestAlpha \
      TestCrossPlatformCoverageApple \
      TestCrossPlatformCoverageMail \
      TestCrossPlatformCoveragePolicy \
      TestCrossPlatformCoverageSheet \
      TestCreate \
      TestDownload \
      TestSend
    exit 0
    ;;
esac
printf '%s\n' "$*" >> "$FAKE_GO_LOG"
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	script := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	cmd := exec.Command("sh", script, "run-lane", "./internal/app", "lane-2")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GO_LOG="+logPath,
		"TMPDIR="+t.TempDir(),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run lane-2 failed: %v\n%s", err, output)
	}
	for _, partition := range []string{"c-p-r", "c-m-o", "c-s-z"} {
		want := "running internal/app race partition " + partition
		if !strings.Contains(string(output), want) {
			t.Errorf("run-lane output missing %q:\n%s", want, output)
		}
	}

	invocations, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake go invocation log: %v", err)
	}
	lines := strings.FieldsFunc(strings.TrimSpace(string(invocations)), func(r rune) bool { return r == '\n' })
	if len(lines) != 3 {
		t.Fatalf("lane-2 launched %d go test processes, want 3:\n%s", len(lines), invocations)
	}
}

func runAppRaceHelper(t *testing.T, root, script string, args ...string) string {
	t.Helper()
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", script, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func TestCIMergeCoverageProfilesUnionsDuplicateBlocks(t *testing.T) {
	root := testPackagePlanRoot(t)
	workdir := t.TempDir()
	first := filepath.Join(workdir, "first.txt")
	second := filepath.Join(workdir, "second.txt")
	output := filepath.Join(workdir, "merged.txt")

	const blockA = "example.com/project/a.go:10.2,12.3 2"
	const blockB = "example.com/project/b.go:20.1,20.8 1"
	if err := os.WriteFile(first, []byte("mode: atomic\n"+blockA+" 0\n"+blockB+" 3\n"), 0o600); err != nil {
		t.Fatalf("write first coverage profile: %v", err)
	}
	if err := os.WriteFile(second, []byte("mode: atomic\n"+blockA+" 7\n"+blockB+" 1\n"), 0o600); err != nil {
		t.Fatalf("write second coverage profile: %v", err)
	}

	script := filepath.Join(root, "scripts", "ci", "merge-coverage-profiles.sh")
	cmd := exec.Command("sh", script, output, first, second)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TMPDIR="+t.TempDir())
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("merge coverage profiles failed: %v\n%s", err, combined)
	}
	merged, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read merged coverage profile: %v", err)
	}
	want := "mode: atomic\n" + blockA + " 7\n" + blockB + " 3\n"
	if string(merged) != want {
		t.Fatalf("merged profile = %q, want %q", merged, want)
	}
}

func TestCIFullCoverageRunnerKeepsOneJobAndPartitionsApp(t *testing.T) {
	root := testPackagePlanRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "ci", "run-full-coverage.sh"))
	if err != nil {
		t.Fatalf("read full coverage runner: %v", err)
	}
	script := string(data)
	for _, want := range []string{
		"for shard in app cli generators helpers remaining; do",
		`"$TOOLS_ROOT/scripts/ci/run-coverage-shard.sh" run "$shard" "$profile"`,
		`"$TOOLS_ROOT/scripts/ci/merge-coverage-profiles.sh"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("full coverage runner missing %q", want)
		}
	}
	for _, forbidden := range []string{"xargs -P", "parallel "} {
		if strings.Contains(script, forbidden) {
			t.Errorf("full coverage runner unexpectedly adds in-job fan-out %q", forbidden)
		}
	}
}

func TestCICoverageShardRunnerBoundsPackageParallelism(t *testing.T) {
	root := testPackagePlanRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "ci", "run-coverage-shard.sh"))
	if err != nil {
		t.Fatalf("read coverage shard runner: %v", err)
	}
	script := string(data)
	for _, want := range []string{
		"package_parallelism=1",
		`if [ "$shard" = remaining ]; then`,
		"package_parallelism=2",
		`go test -count=1 -p "$package_parallelism"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("coverage shard runner missing bounded parallelism contract %q", want)
		}
	}
}

func TestCICoverageShardsOwnEveryAppPartitionExactlyOnce(t *testing.T) {
	root := testPackagePlanRoot(t)
	appScript := filepath.Join(root, "scripts", "ci", "run-app-race-tests.sh")
	list := exec.Command("sh", appScript, "list-partitions")
	list.Dir = root
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("list app partitions failed: %v\n%s", err, output)
	}
	want := strings.Fields(string(output))
	if len(want) == 0 {
		t.Fatal("app partition list is empty")
	}

	shardScript := filepath.Join(root, "scripts", "ci", "run-coverage-shard.sh")
	counts := map[string]int{}
	for _, shard := range []string{"app", "cli", "generators", "helpers", "remaining"} {
		cmd := exec.Command("sh", shardScript, "list-app-partitions", shard)
		cmd.Dir = root
		shardOutput, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("list app partitions for %s failed: %v\n%s", shard, runErr, shardOutput)
		}
		for _, partition := range strings.Fields(string(shardOutput)) {
			counts[partition]++
		}
	}

	wanted := map[string]bool{}
	for _, partition := range want {
		wanted[partition] = true
		if counts[partition] != 1 {
			t.Errorf("app partition %q assigned %d times, want exactly once", partition, counts[partition])
		}
	}
	for partition := range counts {
		if !wanted[partition] {
			t.Errorf("coverage shard owns unknown app partition %q", partition)
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

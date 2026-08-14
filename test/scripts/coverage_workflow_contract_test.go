package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageGatePolicyProfileCanBeExplicitlyOmitted(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	binDir := t.TempDir()
	fakeGoPath := filepath.Join(binDir, "go")
	const fakeGo = `#!/bin/sh
set -eu
case "$1" in
  build)
    shift
    output=
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -o)
          output="$2"
          shift 2
          ;;
        *)
          shift
          ;;
      esac
    done
    cat > "$output" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" > "$COVERAGE_ARGS_LOG"
EOF
    chmod +x "$output"
    ;;
  list)
    printf '%s\n' "example.com/coverage-fixture"
    ;;
  *)
    printf 'unexpected fake go command: %s\n' "$1" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		t.Fatalf("WriteFile(fake go) error = %v", err)
	}

	baseEnv := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "PATH=") ||
			strings.HasPrefix(value, "COVERAGE_DIFF_PROFILE=") ||
			strings.HasPrefix(value, "COVERAGE_ARGS_LOG=") {
			continue
		}
		baseEnv = append(baseEnv, value)
	}
	baseEnv = append(baseEnv, "PATH="+binDir+":"+os.Getenv("PATH"))

	runGate := func(t *testing.T, diffProfile *string) []string {
		t.Helper()

		argsLog := filepath.Join(t.TempDir(), "args.log")
		cmd := exec.Command(
			"sh",
			"./scripts/policy/check-coverage-gate.sh",
			"--base-ref",
			"HEAD",
		)
		cmd.Dir = root
		cmd.Env = append(append([]string{}, baseEnv...), "COVERAGE_ARGS_LOG="+argsLog)
		if diffProfile != nil {
			cmd.Env = append(cmd.Env, "COVERAGE_DIFF_PROFILE="+*diffProfile)
		}
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("coverage gate error = %v\noutput:\n%s", runErr, output)
		}
		data, readErr := os.ReadFile(argsLog)
		if readErr != nil {
			t.Fatalf("ReadFile(args log) error = %v", readErr)
		}
		return strings.Fields(string(data))
	}

	assertDiffProfiles := func(t *testing.T, args []string, want ...string) {
		t.Helper()

		var got []string
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--diff-profile" {
				got = append(got, args[i+1])
				i++
			}
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("diff profiles = %q, want %q; args = %q", got, want, args)
		}
	}

	t.Run("unset keeps strict policy profile", func(t *testing.T) {
		assertDiffProfiles(
			t,
			runGate(t, nil),
			"coverage-policy.txt",
			"coverage.txt",
		)
	})

	t.Run("explicit empty omits only policy profile", func(t *testing.T) {
		empty := ""
		assertDiffProfiles(t, runGate(t, &empty), "coverage.txt")
	})
}

// TestCoverageWorkflowShardsAndBaselineCache pins the full-suite coverage
// architecture: the candidate profile is produced by disjoint per-shard
// helper jobs and reassembled before enforcement, and the merge-base profile
// is reused only through an exact-key cache written by a green main push of
// that same commit. Near-miss reuse (restore-keys) would compare the
// candidate against the wrong commit and must never appear.
func TestCoverageWorkflowShardsAndBaselineCache(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	admission := string(data)

	currentStart := strings.Index(admission, "\n  coverage-current:\n")
	fullStart := strings.Index(admission, "\n  coverage-current-full:\n")
	supportingStart := strings.Index(admission, "\n  coverage-supporting:\n")
	baselineStart := strings.Index(admission, "\n  coverage-baseline:\n")
	gateStart := strings.Index(admission, "\n  coverage:\n")
	policyStart := strings.Index(admission, "\n  policy:\n")
	if currentStart < 0 || fullStart <= currentStart || supportingStart <= fullStart ||
		baselineStart <= supportingStart || gateStart <= baselineStart || policyStart <= gateStart {
		t.Fatal("CI workflow missing ordered coverage job boundaries")
	}

	currentJob := admission[currentStart:fullStart]
	if !strings.Contains(currentJob, "needs.lint.outputs.full_suite != 'true'") {
		t.Error("coverage-current must be scoped-tier only; the full suite belongs to the shard matrix")
	}
	if strings.Contains(currentJob, "./ ./cmd/... ./internal/... ./skills/...") {
		t.Error("coverage-current must not retain the retired single serial full-suite run")
	}

	fullJob := admission[fullStart:supportingStart]
	for _, want := range []string{
		"needs.lint.outputs.full_suite == 'true'",
		"fail-fast: false",
		"          - app",
		"          - cli",
		"          - generators",
		"          - helpers",
		"          - remaining",
		`./scripts/ci/test-packages.sh list-coverage "$COVERAGE_SHARD"`,
		"go test -count=1 -p 1",
		`-coverprofile="coverage-shard-$COVERAGE_SHARD.txt"`,
		"-covermode=atomic",
		"name: coverage-current-shard-${{ matrix.shard }}",
	} {
		if !strings.Contains(fullJob, want) {
			t.Errorf("coverage-current-full missing shard contract %q", want)
		}
	}

	baselineJob := admission[baselineStart:gateStart]
	cachePath := "coverage-cache.txt"
	baselineKey := "dws-coverage-full-v2-${{ env.COVERAGE_BASE_REF }}-go${{ steps.setup-go.outputs.go-version }}"
	for _, want := range []string{
		"uses: actions/cache/restore@v4",
		"uses: actions/cache/save@v4",
		"path: " + cachePath,
		"key: " + baselineKey,
		"if: steps.baseline-cache.outputs.cache-hit != 'true'",
		"cp coverage-cache.txt coverage-base.txt",
		"cp coverage-base.txt coverage-cache.txt",
	} {
		if !strings.Contains(baselineJob, want) {
			t.Errorf("coverage-baseline missing cache contract %q", want)
		}
	}
	if strings.Count(baselineJob, "key: "+baselineKey) != 2 {
		t.Error("coverage-baseline restore and save must use the identical exact cache key")
	}
	if strings.Count(baselineJob, "path: "+cachePath) != 2 {
		t.Error("coverage-baseline restore and save must use the identical cache path/version")
	}
	if strings.Contains(baselineJob, "restore-keys") {
		t.Error("coverage baseline cache must stay exact-key; prefix restore-keys can resurrect a wrong-commit baseline")
	}

	gateJob := admission[gateStart:policyStart]
	for _, want := range []string{
		"pattern: coverage-current-*",
		"merge-multiple: true",
		"for shard in app cli generators helpers remaining; do",
		"test ! -f coverage.txt",
		`test "$(head -n 1 "$profile")" = "mode: atomic"`,
		"github.event_name == 'push'",
		"cp coverage.txt coverage-cache.txt",
		"path: " + cachePath,
		"key: dws-coverage-full-v2-${{ github.sha }}-go${{ steps.setup-go.outputs.go-version }}",
		`"current shards:$CURRENT_FULL_RESULT:$current_full_expected"`,
	} {
		if !strings.Contains(gateJob, want) {
			t.Errorf("coverage gate missing shard assembly contract %q", want)
		}
	}

	if strings.Count(gateJob, "path: "+cachePath) != 1 {
		t.Error("green main push must save the candidate profile through the same cache path/version as baseline restore")
	}
}

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

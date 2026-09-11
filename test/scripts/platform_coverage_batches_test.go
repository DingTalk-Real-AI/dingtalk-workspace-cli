package scripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformCoverageBatchesPreserveSelectionAndCrossPackageUnion(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(fixture, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/platform-fixture\n\ngo 1.25\n")
	write("internal/helper/helper.go", "package helper\nfunc FromApp() int { return 1 }\nfunc FromHelper() int { return 2 }\nfunc Uncovered() int { return 3 }\n")
	write("internal/helper/helper_test.go", `package helper
import "testing"
func TestAllShortcutsHelper(t *testing.T) { FromHelper() }
func TestNotSelected(t *testing.T) { t.Fatal("non-platform test ran") }
`)
	write("internal/app/app.go", "package app\nfunc Value() int { return 1 }\n")
	appTests := `package app
import (
 "fmt"
 "os"
 "testing"
 "example.com/platform-fixture/internal/helper"
)
func record(t *testing.T) {
 t.Helper()
 helper.FromApp()
 Value()
 f, err := os.OpenFile(os.Getenv("PLATFORM_TEST_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
 if err != nil { t.Fatal(err) }
 defer f.Close()
 fmt.Fprintf(f, "%s %d\n", t.Name(), os.Getpid())
}
func TestNotSelected(t *testing.T) { t.Fatal("non-platform test ran") }
`
	for i := 0; i < 25; i++ {
		appTests += fmt.Sprintf("func TestCrossPlatformCoverageCase%d(t *testing.T) { record(t) }\n", i)
	}
	appTests += "func TestAllShortcutsApp(t *testing.T) { record(t) }\n"
	write("internal/app/app_test.go", appTests)
	logPath := filepath.Join(fixture, "executed.log")
	profile := filepath.Join(fixture, "coverage.txt")
	run := func() ([]byte, error) {
		cmd := exec.Command("sh", filepath.Join(root, "scripts/ci/run-platform-coverage-tests.sh"), profile,
			"./internal/app,./internal/helper", "1m", "./internal/app", "./internal/helper")
		cmd.Dir = fixture
		cmd.Env = append(os.Environ(), "GOWORK=off", "PLATFORM_TEST_LOG="+logPath)
		return cmd.CombinedOutput()
	}
	output, err := run()
	if err != nil {
		t.Fatalf("platform coverage failed: %v\n%s", err, output)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	processes := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || seen[fields[0]] {
			t.Fatalf("duplicate or malformed execution: %q", line)
		}
		seen[fields[0]] = true
		processes[fields[1]]++
	}
	if len(seen) != 26 || !seen["TestAllShortcutsApp"] || len(processes) != 3 {
		t.Fatalf("executed tests = %d, processes = %v; log:\n%s", len(seen), processes, log)
	}
	for pid, count := range processes {
		if count > 12 {
			t.Errorf("process %s ran %d tests, want at most 12", pid, count)
		}
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	for line, covered := range map[string]bool{"helper.go:2.": true, "helper.go:3.": true, "helper.go:4.": false} {
		matches := 0
		for _, block := range strings.Split(string(data), "\n") {
			if strings.Contains(block, line) {
				matches++
				fields := strings.Fields(block)
				if len(fields) != 3 || (fields[2] != "0") != covered {
					t.Errorf("coverage block = %q, want covered=%v", block, covered)
				}
			}
		}
		if matches != 1 {
			t.Errorf("block %q occurs %d times, want once", line, matches)
		}
	}

	// A failure in a later app batch must stop publication, even after other
	// packages and earlier batches have already produced valid profiles.
	if err := os.Remove(profile); err != nil {
		t.Fatal(err)
	}
	write("internal/app/app_test.go", appTests+"func TestCrossPlatformCoverageZFailure(t *testing.T) { t.Fatal(\"injected failure\") }\n")
	if output, err := run(); err == nil || !strings.Contains(string(output), "injected failure") {
		t.Fatalf("expected batch failure: %v\n%s", err, output)
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("failed run published a profile: %v", err)
	}

	// Discovery errors must not become an empty successful batch.
	write("internal/app/broken_test.go", "package app\nthis is invalid Go\n")
	if output, err := run(); err == nil {
		t.Fatalf("expected discovery failure:\n%s", output)
	}
}

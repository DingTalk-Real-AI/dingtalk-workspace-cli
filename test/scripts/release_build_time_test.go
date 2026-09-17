package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageReleaseBuildTimeUsesUTCCommitterDate(t *testing.T) {
	repo := t.TempDir()
	script, err := os.ReadFile("../../scripts/build/release-build-time.sh")
	if err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(repo, "scripts", "build", "release-build-time.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, script, 0o644); err != nil {
		t.Fatal(err)
	}
	// Different author/committer times and offsets catch both wrong-date and
	// machine-timezone dependence. Signing/global Git configuration is excluded.
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=2026-01-01T01:00:00-07:00", "GIT_COMMITTER_DATE=2026-09-06T01:02:03+08:00")
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = repo, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "fixture")
	commit := git("rev-parse", "HEAD")
	for _, zone := range []string{"UTC", "Asia/Shanghai", "America/Los_Angeles"} {
		t.Run(zone, func(t *testing.T) {
			cmd := exec.Command("sh", scriptPath, commit)
			cmd.Dir = t.TempDir() // the helper resolves its repository independently of cwd
			cmd.Env = append(env, "TZ="+zone)
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != "2026-09-05T17:02:03Z\n" {
				t.Fatalf("build time = %q, err = %v", out, err)
			}
		})
	}
	for _, invalid := range []string{"HEAD", "--help", commit[:12], strings.Repeat("0", 40)} {
		t.Run(invalid, func(t *testing.T) {
			cmd := exec.Command("sh", scriptPath, invalid)
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("accepted unproven commit %q: %s", invalid, out)
			}
		})
	}
}

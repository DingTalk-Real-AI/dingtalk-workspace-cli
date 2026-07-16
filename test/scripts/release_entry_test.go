package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type releaseEntryFixture struct {
	repo  *releaseTestRepo
	entry string
	log   string
}

func newReleaseEntryFixture(t *testing.T) *releaseEntryFixture {
	t.Helper()
	r := newReleaseTestRepo(t)
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	entry := filepath.Join(r.root, "scripts", "release", "dws-release.sh")
	releaseCopyFile(t, r.lib, filepath.Join(r.root, "scripts", "release", "release-lib.sh"), 0o644)
	releaseCopyFile(t, filepath.Join(sourceRoot, "scripts", "release", "dws-release.sh"), entry, 0o755)
	fakeDelegate := func(name string) []byte {
		return []byte("#!/bin/sh\nset -eu\nprintf '" + name + "' >> \"$DWS_RELEASE_CALL_LOG\"\nfor arg in \"$@\"; do printf '\\t%s' \"$arg\" >> \"$DWS_RELEASE_CALL_LOG\"; done\nprintf '\\n' >> \"$DWS_RELEASE_CALL_LOG\"\nexit \"${DWS_RELEASE_FAKE_EXIT:-0}\"\n")
	}
	mustWriteFile(t, filepath.Join(r.root, "scripts", "release", "prepare-changelog.sh"), fakeDelegate("prepare"), 0o755)
	mustWriteFile(t, filepath.Join(r.root, "scripts", "release", "release.sh"), fakeDelegate("release"), 0o755)
	r.commitAndPush(t, "install unified release entry fixture")
	return &releaseEntryFixture{repo: r, entry: entry, log: filepath.Join(t.TempDir(), "calls.log")}
}

func (f *releaseEntryFixture) run(t *testing.T, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{f.entry}, args...)...)
	cmd.Dir = f.repo.root
	cmd.Env = append(os.Environ(), "DWS_RELEASE_CALL_LOG="+f.log)
	cmd.Env = append(cmd.Env, extraEnv...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (f *releaseEntryFixture) configureRemote(t *testing.T, remote string) {
	t.Helper()
	output, err := f.run(t, nil, "config", "--remote", remote)
	if err != nil {
		t.Fatalf("config remote error = %v\noutput:\n%s", err, output)
	}
}

func (f *releaseEntryFixture) callLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.log)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatalf("ReadFile(call log) error = %v", err)
	}
	return string(data)
}

func TestDWSReleaseEntryPreparesMissingChangelogAndStops(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "prerelease",
			args: []string{"v1.0.1-beta.1", "--remote", "origin", "--publish"},
			want: "prepare\tprerelease\tv1.0.1-beta.1\n",
		},
		{
			name: "stable",
			args: []string{"v1.0.1", "--from-beta", "v1.0.1-beta.1", "--remote", "origin"},
			want: "prepare\tstable\tv1.0.1\t--from-beta\tv1.0.1-beta.1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseEntryFixture(t)
			f.configureRemote(t, "origin")
			output, err := f.run(t, nil, test.args...)
			if err != nil {
				t.Fatalf("dws-release prepare error = %v\noutput:\n%s", err, output)
			}
			if got := f.callLog(t); got != test.want {
				t.Fatalf("delegate calls = %q, want %q", got, test.want)
			}
			if !strings.Contains(output, "publishing intentionally stopped") {
				t.Fatalf("prepare output did not make the stop boundary clear:\n%s", output)
			}
		})
	}
}

func TestDWSReleaseEntryRoutesOneGuardedCommand(t *testing.T) {
	tests := []struct {
		name     string
		section  string
		args     []string
		wantCall string
	}{
		{
			name:     "check prerelease",
			section:  betaSection(),
			args:     []string{"v1.0.1-beta.1", "--remote", "origin"},
			wantCall: "release\tprerelease\tv1.0.1-beta.1\t--remote\torigin\n",
		},
		{
			name:     "publish prerelease keeps confirmation",
			section:  betaSection(),
			args:     []string{"v1.0.1-beta.1", "--remote", "origin", "--publish"},
			wantCall: "release\tprerelease\tv1.0.1-beta.1\t--remote\torigin\t--publish\n",
		},
		{
			name:     "check stable",
			section:  stableSection(),
			args:     []string{"v1.0.1", "--from-beta", "v1.0.1-beta.1", "--remote", "origin"},
			wantCall: "release\tstable\tv1.0.1\t--remote\torigin\t--from-beta\tv1.0.1-beta.1\n",
		},
		{
			name:     "publish stable keeps confirmation",
			section:  stableSection(),
			args:     []string{"v1.0.1", "--from-beta", "v1.0.1-beta.1", "--remote", "origin", "--publish"},
			wantCall: "release\tstable\tv1.0.1\t--remote\torigin\t--from-beta\tv1.0.1-beta.1\t--publish\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseEntryFixture(t)
			f.configureRemote(t, "origin")
			mustWriteFile(t, filepath.Join(f.repo.root, "CHANGELOG.md"), []byte(releaseChangelog(test.section)), 0o644)
			f.repo.commitAndPush(t, "add candidate changelog")
			output, err := f.run(t, nil, test.args...)
			if err != nil {
				t.Fatalf("dws-release route error = %v\noutput:\n%s", err, output)
			}
			if got := f.callLog(t); got != test.wantCall {
				t.Fatalf("delegate calls = %q, want %q\noutput:\n%s", got, test.wantCall, output)
			}
		})
	}
}

func TestDWSReleaseEntryRejectsInvalidInvocationBeforeDelegation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid version", args: []string{"1.0.1"}, want: "invalid release version"},
		{name: "stable missing beta", args: []string{"v1.0.1"}, want: "requires --from-beta"},
		{name: "prerelease with beta", args: []string{"v1.0.1-beta.1", "--from-beta", "v1.0.1-beta.1"}, want: "only valid for stable"},
		{name: "conflicting mode", args: []string{"v1.0.1-beta.1", "--check", "--publish"}, want: "cannot be used together"},
		{name: "yes bypass", args: []string{"v1.0.1-beta.1", "--publish", "--yes"}, want: "unknown argument"},
		{name: "unknown flag", args: []string{"v1.0.1-beta.1", "--force"}, want: "unknown argument"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseEntryFixture(t)
			output, err := f.run(t, nil, test.args...)
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("invalid invocation: err=%v, want=%q\noutput:\n%s", err, test.want, output)
			}
			if got := f.callLog(t); got != "" {
				t.Fatalf("invalid invocation delegated work: %q", got)
			}
		})
	}
}

func TestDWSReleaseEntryUsesConfiguredRemote(t *testing.T) {
	f := newReleaseEntryFixture(t)
	f.configureRemote(t, "origin")
	mustWriteFile(t, filepath.Join(f.repo.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	f.repo.commitAndPush(t, "add beta changelog")
	output, err := f.run(t, nil, "v1.0.1-beta.1")
	if err != nil {
		t.Fatalf("configured remote route error = %v\noutput:\n%s", err, output)
	}
	want := "release\tprerelease\tv1.0.1-beta.1\t--remote\torigin\n"
	if got := f.callLog(t); got != want {
		t.Fatalf("delegate calls = %q, want %q", got, want)
	}
}

func TestDWSReleaseEntryFailsClosedWithoutRemote(t *testing.T) {
	f := newReleaseEntryFixture(t)
	mustWriteFile(t, filepath.Join(f.repo.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	f.repo.commitAndPush(t, "add beta changelog")
	output, err := f.run(t, nil, "v1.0.1-beta.1")
	if err == nil || !strings.Contains(output, "release remote is not configured") {
		t.Fatalf("missing remote did not fail closed: err=%v\noutput:\n%s", err, output)
	}
	if got := f.callLog(t); got != "" {
		t.Fatalf("missing remote delegated work: %q", got)
	}
}

func TestDWSReleaseEntryPropagatesDelegateFailure(t *testing.T) {
	f := newReleaseEntryFixture(t)
	f.configureRemote(t, "origin")
	mustWriteFile(t, filepath.Join(f.repo.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	f.repo.commitAndPush(t, "add beta changelog")
	output, err := f.run(t, []string{"DWS_RELEASE_FAKE_EXIT=23"}, "v1.0.1-beta.1", "--remote", "origin")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("delegate error was not propagated: err=%v\noutput:\n%s", err, output)
	}
}

func TestDWSReleaseEntryFastForwardsCleanMain(t *testing.T) {
	f := newReleaseEntryFixture(t)
	f.configureRemote(t, "origin")
	other := filepath.Join(t.TempDir(), "other")
	mustRun(t, filepath.Dir(other), "git", "clone", "-b", "main", f.repo.remote, other)
	mustRun(t, other, "git", "config", "user.name", "Release Test")
	mustRun(t, other, "git", "config", "user.email", "release-test@example.com")
	mustWriteFile(t, filepath.Join(other, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	mustRun(t, other, "git", "add", "CHANGELOG.md")
	mustRun(t, other, "git", "commit", "-m", "add remote beta changelog")
	mustRun(t, other, "git", "push", "origin", "main")

	output, err := f.run(t, nil, "v1.0.1-beta.1", "--remote", "origin")
	if err != nil {
		t.Fatalf("fast-forward route error = %v\noutput:\n%s", err, output)
	}
	localHead := strings.TrimSpace(mustOutput(t, f.repo.root, "git", "rev-parse", "HEAD"))
	remoteHead := strings.TrimSpace(mustOutput(t, f.repo.root, "git", "rev-parse", "refs/remotes/origin/main"))
	if localHead != remoteHead {
		t.Fatalf("local main was not fast-forwarded: local=%s remote=%s", localHead, remoteHead)
	}
	if got := f.callLog(t); !strings.HasPrefix(got, "release\tprerelease\tv1.0.1-beta.1") {
		t.Fatalf("fast-forward did not reach guarded delegate: %q", got)
	}
}

func TestDWSReleaseEntryRejectsRetargetedRemote(t *testing.T) {
	f := newReleaseEntryFixture(t)
	f.configureRemote(t, "origin")
	mustWriteFile(t, filepath.Join(f.repo.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	f.repo.commitAndPush(t, "add beta changelog")
	otherRemote := filepath.Join(t.TempDir(), "other.git")
	mustRun(t, f.repo.root, "git", "init", "--bare", otherRemote)
	mustRun(t, f.repo.root, "git", "remote", "set-url", "origin", otherRemote)

	output, err := f.run(t, nil, "v1.0.1-beta.1")
	if err == nil || !strings.Contains(output, "changed repository identity") {
		t.Fatalf("retargeted remote was not rejected: err=%v\noutput:\n%s", err, output)
	}
	if got := f.callLog(t); got != "" {
		t.Fatalf("retargeted remote delegated work: %q", got)
	}
}

func TestDWSReleaseEntryRejectsAuthorityOverrides(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "non github remote", env: "DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE=1", want: "test-only"},
		{name: "official tags URL", env: "DWS_RELEASE_OFFICIAL_TAGS_URL=https://example.com/tags.git", want: "cannot override"},
		{name: "official repository", env: "DWS_RELEASE_OFFICIAL_REPOSITORY=other/repo", want: "cannot override"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newReleaseEntryFixture(t)
			output, err := f.run(t, []string{test.env}, "v1.0.1-beta.1")
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("authority override was not rejected: err=%v, want=%q\noutput:\n%s", err, test.want, output)
			}
			if got := f.callLog(t); got != "" {
				t.Fatalf("authority override delegated work: %q", got)
			}
		})
	}
}

func TestReleaseCommandRejectsNonInteractivePublishWithoutYes(t *testing.T) {
	r := newReleaseTestRepo(t)
	installReleaseCommandFixture(t, r)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "install release automation")

	output, err := runReleaseScript(t, r.root, filepath.Join(r.root, "scripts", "release", "release.sh"),
		"prerelease", "v1.0.1-beta.1", "--remote", "origin", "--publish",
	)
	if err == nil || !strings.Contains(output, "interactive confirmation is unavailable") {
		t.Fatalf("non-interactive publish did not require confirmation: err=%v\noutput:\n%s", err, output)
	}
	if got := mustOutput(t, r.root, "git", "tag", "--list", "v1.0.1-beta.1"); got != "" {
		t.Fatalf("rejected non-interactive publish created a tag: %s", got)
	}
}

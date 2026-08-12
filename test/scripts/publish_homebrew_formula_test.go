package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishHomebrewFormulaPublishesDirectlyWithoutPR(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish-homebrew-formula.sh"))
	if err != nil {
		t.Fatalf("Abs(publish-homebrew-formula.sh) error = %v", err)
	}

	root := t.TempDir()
	remoteDir := filepath.Join(root, "tap.git")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	seedTapRepo(t, remoteDir, "main", "class OldFormula < Formula\nend\n")

	sourceFormula := filepath.Join(root, "dingtalk-workspace-cli.rb")
	mustWriteFile(t, sourceFormula, []byte("class DingtalkWorkspaceCli < Formula\n  desc \"DingTalk Workspace CLI\"\nend\n"), 0o644)
	publishOutput := filepath.Join(root, "publish-output")

	fakeBin := filepath.Join(root, "bin")
	ghLog := filepath.Join(root, "gh.log")
	mustWriteFile(t, filepath.Join(fakeBin, "gh"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$GH_LOG"
exit 97
`), 0o755)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_LOG="+ghLog,
		"DWS_TAP_REPO_URL="+remoteDir,
		"DWS_FORMULA_SOURCE="+sourceFormula,
		"GITHUB_OUTPUT="+publishOutput,
		"DWS_GIT_NAME=DWS Bot",
		"DWS_GIT_EMAIL=dws@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish-homebrew-formula.sh error = %v\noutput:\n%s", err, string(output))
	}

	if !strings.Contains(string(output), "Published Homebrew formula") {
		t.Fatalf("publish output missing success message:\n%s", string(output))
	}

	cloneDir := filepath.Join(root, "check")
	mustRun(t, root, "git", "clone", "--branch", "main", remoteDir, cloneDir)
	got, err := os.ReadFile(filepath.Join(cloneDir, "Formula", "dingtalk-workspace-cli.rb"))
	if err != nil {
		t.Fatalf("ReadFile(published formula) error = %v", err)
	}
	if string(got) != "class DingtalkWorkspaceCli < Formula\n  desc \"DingTalk Workspace CLI\"\nend\n" {
		t.Fatalf("published formula = %q", string(got))
	}
	publishedCommit := strings.TrimSpace(mustOutput(t, cloneDir, "git", "rev-parse", "HEAD"))
	result, err := os.ReadFile(publishOutput)
	if err != nil {
		t.Fatalf("ReadFile(publish output) error = %v", err)
	}
	wantResult := "formula_changed=true\npublished_commit=" + publishedCommit + "\n"
	if string(result) != wantResult {
		t.Fatalf("publish result = %q, want %q", result, wantResult)
	}
	parentLine := strings.Fields(mustOutput(t, cloneDir, "git", "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parentLine) != 2 {
		t.Fatalf("published Formula commit parent fields = %v, want exactly one parent", parentLine)
	}
	changedPaths := strings.TrimSpace(mustOutput(t, cloneDir, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"))
	if changedPaths != "Formula/dingtalk-workspace-cli.rb" {
		t.Fatalf("published Formula commit changed %q", changedPaths)
	}
	if calls, err := os.ReadFile(ghLog); err == nil {
		t.Fatalf("direct Homebrew publication unexpectedly invoked gh:\n%s", calls)
	} else if !os.IsNotExist(err) {
		t.Fatalf("ReadFile(gh log) error = %v", err)
	}
}

func TestPublishHomebrewFormulaSkipsWhenFormulaUnchanged(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish-homebrew-formula.sh"))
	if err != nil {
		t.Fatalf("Abs(publish-homebrew-formula.sh) error = %v", err)
	}

	root := t.TempDir()
	remoteDir := filepath.Join(root, "tap.git")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	seedTapRepo(t, remoteDir, "main", "class OldFormula < Formula\nend\n")
	initialFormula := "class DingtalkWorkspaceCli < Formula\n  desc \"DingTalk Workspace CLI\"\nend\n"

	historyDir := filepath.Join(root, "history")
	mustRun(t, root, "git", "clone", "--branch", "main", remoteDir, historyDir)
	mustRun(t, historyDir, "git", "config", "user.name", "DWS Bot")
	mustRun(t, historyDir, "git", "config", "user.email", "dws@example.com")
	mustWriteFile(t, filepath.Join(historyDir, "Formula", "dingtalk-workspace-cli.rb"), []byte(initialFormula), 0o644)
	mustRun(t, historyDir, "git", "add", "Formula/dingtalk-workspace-cli.rb")
	mustRun(t, historyDir, "git", "commit", "-m", "chore: update formula for v1.2.3 [skip ci]")
	formulaCommit := strings.TrimSpace(mustOutput(t, historyDir, "git", "rev-parse", "HEAD"))
	mustRun(t, historyDir, "git", "config", "user.name", "Another Maintainer")
	mustRun(t, historyDir, "git", "config", "user.email", "maintainer@example.com")
	mustWriteFile(t, filepath.Join(historyDir, "README.md"), []byte("unrelated main update\n"), 0o644)
	mustRun(t, historyDir, "git", "add", "README.md")
	mustRun(t, historyDir, "git", "commit", "-m", "docs: unrelated main update")
	mustRun(t, historyDir, "git", "push", "origin", "main")

	sourceFormula := filepath.Join(root, "dingtalk-workspace-cli.rb")
	mustWriteFile(t, sourceFormula, []byte(initialFormula), 0o644)
	publishOutput := filepath.Join(root, "publish-output")

	beforeHead := strings.TrimSpace(mustOutput(t, root, "git", "ls-remote", remoteDir, "refs/heads/main"))

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_TAP_REPO_URL="+remoteDir,
		"DWS_TAP_BRANCH=main",
		"DWS_FORMULA_SOURCE="+sourceFormula,
		"DWS_PUBLISH_OUTPUT="+publishOutput,
		"DWS_GIT_NAME=DWS Bot",
		"DWS_GIT_EMAIL=dws@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish-homebrew-formula.sh error = %v\noutput:\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "No formula changes to publish.") {
		t.Fatalf("publish output missing no-op message:\n%s", string(output))
	}

	afterHead := strings.TrimSpace(mustOutput(t, root, "git", "ls-remote", remoteDir, "refs/heads/main"))
	if beforeHead != afterHead {
		t.Fatalf("remote head changed unexpectedly:\nbefore: %s\nafter:  %s", beforeHead, afterHead)
	}
	result, err := os.ReadFile(publishOutput)
	if err != nil {
		t.Fatalf("ReadFile(publish output) error = %v", err)
	}
	wantResult := "formula_changed=false\npublished_commit=" + formulaCommit + "\n"
	if string(result) != wantResult {
		t.Fatalf("no-op publish result = %q, want last matching Formula-only bot commit %q", result, wantResult)
	}
}

func TestPublishHomebrewFormulaRetriesNonFastForwardFromFreshMain(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish-homebrew-formula.sh"))
	if err != nil {
		t.Fatalf("Abs(publish-homebrew-formula.sh) error = %v", err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) error = %v", err)
	}

	root := t.TempDir()
	remoteDir := filepath.Join(root, "tap.git")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	seedTapRepo(t, remoteDir, "main", "class OldFormula < Formula\nend\n")

	raceDir := filepath.Join(root, "racer")
	mustRun(t, root, "git", "clone", "--branch", "main", remoteDir, raceDir)
	mustRun(t, raceDir, "git", "config", "user.name", "Concurrent Maintainer")
	mustRun(t, raceDir, "git", "config", "user.email", "concurrent@example.com")
	mustWriteFile(t, filepath.Join(raceDir, "README.md"), []byte("concurrent main update\n"), 0o644)
	mustRun(t, raceDir, "git", "add", "README.md")
	mustRun(t, raceDir, "git", "commit", "-m", "docs: concurrent main update")
	raceCommit := strings.TrimSpace(mustOutput(t, raceDir, "git", "rev-parse", "HEAD"))

	sourceFormula := filepath.Join(root, "dingtalk-workspace-cli.rb")
	newFormula := "class DingtalkWorkspaceCli < Formula\n  desc \"DingTalk Workspace CLI\"\nend\n"
	mustWriteFile(t, sourceFormula, []byte(newFormula), 0o644)
	publishOutput := filepath.Join(root, "publish-output")

	fakeBin := filepath.Join(root, "bin")
	gitLog := filepath.Join(root, "git.log")
	raceOnce := filepath.Join(root, "race-once")
	mustWriteFile(t, filepath.Join(fakeBin, "git"), []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$GIT_LOG"
if [ "${1:-}" = push ] && [ "${2:-}" = origin ] &&
   [ "${3:-}" = "HEAD:main" ] && [ ! -e "$GIT_RACE_ONCE" ]; then
  : > "$GIT_RACE_ONCE"
  "$REAL_GIT" -C "$RACE_REPO" push origin main >/dev/null
  exit 1
fi
exec "$REAL_GIT" "$@"
`), 0o755)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"REAL_GIT="+realGit,
		"RACE_REPO="+raceDir,
		"GIT_LOG="+gitLog,
		"GIT_RACE_ONCE="+raceOnce,
		"DWS_TAP_REPO_URL="+remoteDir,
		"DWS_TAP_BRANCH=main",
		"DWS_FORMULA_SOURCE="+sourceFormula,
		"DWS_PUBLISH_OUTPUT="+publishOutput,
		"DWS_GIT_NAME=DWS Bot",
		"DWS_GIT_EMAIL=dws@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish-homebrew-formula.sh race recovery error = %v\noutput:\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "retrying from a fresh clone") ||
		!strings.Contains(string(output), "Published Homebrew formula") {
		t.Fatalf("publisher did not report bounded fresh-clone recovery:\n%s", output)
	}

	checkDir := filepath.Join(root, "check")
	mustRun(t, root, "git", "clone", "--branch", "main", remoteDir, checkDir)
	publishedCommit := strings.TrimSpace(mustOutput(t, checkDir, "git", "rev-parse", "HEAD"))
	parent := strings.TrimSpace(mustOutput(t, checkDir, "git", "rev-parse", "HEAD^"))
	if parent != raceCommit {
		t.Fatalf("retried Formula commit parent = %s, want concurrent main %s", parent, raceCommit)
	}
	parentLine := strings.Fields(mustOutput(t, checkDir, "git", "rev-list", "--parents", "-n", "1", "HEAD"))
	if len(parentLine) != 2 {
		t.Fatalf("retried Formula commit parent fields = %v, want exactly one parent", parentLine)
	}
	changedPaths := strings.TrimSpace(mustOutput(t, checkDir, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"))
	if changedPaths != "Formula/dingtalk-workspace-cli.rb" {
		t.Fatalf("retried Formula commit changed %q", changedPaths)
	}
	formula, err := os.ReadFile(filepath.Join(checkDir, "Formula", "dingtalk-workspace-cli.rb"))
	if err != nil {
		t.Fatalf("ReadFile(published formula) error = %v", err)
	}
	if string(formula) != newFormula {
		t.Fatalf("published formula = %q, want %q", formula, newFormula)
	}
	result, err := os.ReadFile(publishOutput)
	if err != nil {
		t.Fatalf("ReadFile(publish output) error = %v", err)
	}
	wantResult := "formula_changed=true\npublished_commit=" + publishedCommit + "\n"
	if string(result) != wantResult {
		t.Fatalf("race-recovered publish result = %q, want %q", result, wantResult)
	}
	gitCalls, err := os.ReadFile(gitLog)
	if err != nil {
		t.Fatalf("ReadFile(git log) error = %v", err)
	}
	if strings.Contains(string(gitCalls), "--force") {
		t.Fatalf("direct Formula publication must never force push:\n%s", gitCalls)
	}
	if got := strings.Count(string(gitCalls), "push origin HEAD:main"); got != 2 {
		t.Fatalf("direct Formula push attempts = %d, want one rejected attempt and one retry\n%s", got, gitCalls)
	}
}

func TestPublishHomebrewFormulaOpensPRWithoutWritingMain(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish-homebrew-formula.sh"))
	if err != nil {
		t.Fatalf("Abs(publish-homebrew-formula.sh) error = %v", err)
	}

	root := t.TempDir()
	remoteDir := filepath.Join(root, "repo.git")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	oldFormula := "class OldFormula < Formula\nend\n"
	seedTapRepo(t, remoteDir, "main", oldFormula)

	sourceFormula := filepath.Join(root, "dingtalk-workspace-cli.rb")
	newFormula := "class DingtalkWorkspaceCli < Formula\n  desc \"DingTalk Workspace CLI\"\nend\n"
	mustWriteFile(t, sourceFormula, []byte(newFormula), 0o644)

	fakeBin := filepath.Join(root, "bin")
	ghLog := filepath.Join(root, "gh.log")
	mustWriteFile(t, filepath.Join(fakeBin, "gh"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$GH_LOG"
if [ "$1 $2" = "pr create" ]; then
  printf '%s\n' 'https://github.example/pr/1'
fi
`), 0o755)

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_LOG="+ghLog,
		"DWS_TAP_REPO_URL="+remoteDir,
		"DWS_TAP_BRANCH=main",
		"DWS_FORMULA_SOURCE="+sourceFormula,
		"DWS_TAP_GITHUB_TOKEN=test-token",
		"DWS_TAP_PR_REPOSITORY=DingTalk-Real-AI/dingtalk-workspace-cli",
		"DWS_TAP_PR_BRANCH=automation/homebrew-v1.2.3",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish-homebrew-formula.sh error = %v\noutput:\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "Opened Homebrew formula PR: https://github.example/pr/1") {
		t.Fatalf("publish output missing PR URL:\n%s", string(output))
	}

	mainClone := filepath.Join(root, "main-check")
	mustRun(t, root, "git", "clone", "--branch", "main", remoteDir, mainClone)
	mainFormula, err := os.ReadFile(filepath.Join(mainClone, "Formula", "dingtalk-workspace-cli.rb"))
	if err != nil {
		t.Fatalf("ReadFile(main formula) error = %v", err)
	}
	if string(mainFormula) != oldFormula {
		t.Fatalf("publisher wrote main directly: %q", string(mainFormula))
	}

	prClone := filepath.Join(root, "pr-check")
	mustRun(t, root, "git", "clone", "--branch", "automation/homebrew-v1.2.3", remoteDir, prClone)
	prFormula, err := os.ReadFile(filepath.Join(prClone, "Formula", "dingtalk-workspace-cli.rb"))
	if err != nil {
		t.Fatalf("ReadFile(PR formula) error = %v", err)
	}
	if string(prFormula) != newFormula {
		t.Fatalf("PR formula = %q, want %q", string(prFormula), newFormula)
	}

	ghCalls, err := os.ReadFile(ghLog)
	if err != nil {
		t.Fatalf("ReadFile(gh log) error = %v", err)
	}
	for _, want := range []string{"pr list", "pr create"} {
		if !strings.Contains(string(ghCalls), want) {
			t.Errorf("gh calls missing %q:\n%s", want, ghCalls)
		}
	}
}

func seedTapRepo(t *testing.T, remoteDir, branch, formulaContent string) {
	t.Helper()

	workDir := t.TempDir()
	mustRun(t, t.TempDir(), "git", "clone", remoteDir, workDir)
	mustRun(t, workDir, "git", "config", "user.name", "Seed User")
	mustRun(t, workDir, "git", "config", "user.email", "seed@example.com")
	mustWriteFile(t, filepath.Join(workDir, "Formula", "dingtalk-workspace-cli.rb"), []byte(formulaContent), 0o644)
	mustRun(t, workDir, "git", "add", "Formula/dingtalk-workspace-cli.rb")
	mustRun(t, workDir, "git", "commit", "-m", "seed")
	mustRun(t, workDir, "git", "branch", "-M", branch)
	mustRun(t, workDir, "git", "push", "origin", branch)
}

func mustRun(t *testing.T, workdir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v error = %v\noutput:\n%s", name, args, err, string(output))
	}
}

func mustOutput(t *testing.T, workdir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v error = %v\noutput:\n%s", name, args, err, string(output))
	}
	return string(output)
}

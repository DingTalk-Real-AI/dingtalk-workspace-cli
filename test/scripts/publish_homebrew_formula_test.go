package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishHomebrewFormulaPushesUpdatedFormula(t *testing.T) {
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

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_TAP_REPO_URL="+remoteDir,
		"DWS_TAP_BRANCH=main",
		"DWS_FORMULA_SOURCE="+sourceFormula,
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
	initialFormula := "class DingtalkWorkspaceCli < Formula\n  desc \"DingTalk Workspace CLI\"\nend\n"
	seedTapRepo(t, remoteDir, "main", initialFormula)

	sourceFormula := filepath.Join(root, "dingtalk-workspace-cli.rb")
	mustWriteFile(t, sourceFormula, []byte(initialFormula), 0o644)

	beforeHead := strings.TrimSpace(mustOutput(t, root, "git", "ls-remote", remoteDir, "refs/heads/main"))

	cmd := exec.Command("sh", scriptPath)
	cmd.Env = append(os.Environ(),
		"DWS_TAP_REPO_URL="+remoteDir,
		"DWS_TAP_BRANCH=main",
		"DWS_FORMULA_SOURCE="+sourceFormula,
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

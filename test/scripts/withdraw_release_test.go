package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithdrawReleaseWorkflowIsProtectedAndFailClosed(t *testing.T) {
	t.Parallel()

	workflowPath, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "withdraw-release.yml"))
	if err != nil {
		t.Fatalf("Abs(withdraw workflow) error = %v", err)
	}
	content, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", workflowPath, err)
	}
	workflow := string(content)

	for _, want := range []string{
		"workflow_dispatch:",
		"version:",
		"reason:",
		"confirmation:",
		"group: dws-release-publication",
		"cancel-in-progress: false",
		"environment: release-withdrawal",
		"prevent_self_review !== true",
		"deployment_branch_policy?.protected_branches !== true",
		"can_admins_bypass !== false",
		`const expectedRepository = "DingTalk-Real-AI/dingtalk-workspace-cli"`,
		"context.ref !== `refs/heads/${defaultBranch}`",
		"branch.data.object.sha !== context.sha",
		"contents: write",
		"persist-credentials: false",
		"NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}",
		"DWS_GITEE_ENABLED:",
		"HOMEBREW_PR_TOKEN: ${{ secrets.HOMEBREW_PR_TOKEN }}",
		"./scripts/release/withdraw-release.sh",
		"already-installed clients cannot be remotely downgraded.",
		"this run remains failed until that PR is independently reviewed",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("withdraw workflow missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"\n  push:",
		"\n  schedule:",
		"npm unpublish",
		"cancel-in-progress: true",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("withdraw workflow contains forbidden trigger/action %q", forbidden)
		}
	}
}

func TestWithdrawReleaseScriptDeletesProblemReleaseLastAndUsesPermanentTombstone(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "withdraw-release.sh"))
	if err != nil {
		t.Fatalf("Abs(withdraw script) error = %v", err)
	}
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", scriptPath, err)
	}
	script := string(content)

	for _, want := range []string{
		`TOMBSTONE="withdrawn/${VERSION}"`,
		`github_api --method POST "repos/${OFFICIAL_REPOSITORY}/git/tags"`,
		`github_api --method POST "repos/${OFFICIAL_REPOSITORY}/git/refs"`,
		"existing tombstone $TOMBSTONE has different immutable withdrawal metadata",
		`github_api --method PATCH`,
		`npm deprecate "${PACKAGE_NAME}@${VERSION#v}"`,
		`npm dist-tag add "${PACKAGE_NAME}@${ROLLBACK_VERSION#v}"`,
		`TARGET_OSS_MODE="$("$SCRIPT_DIR/release-tag-oss-mode.sh" "$VERSION")"`,
		`printf 'OSS-Mirror: %s\n' "$TARGET_OSS_MODE"`,
		`oss_mode = fields.get("OSS-Mirror", "enabled")`,
		`if [ "$OSS_ENABLED" = true ]; then`,
		`*) err "could not resolve immutable OSS policy for $VERSION" ;;`,
		`"$OSSUTIL" rm -rf`,
		`curl -fsS -X DELETE`,
		`git push "$GITEE_GIT_REMOTE" ":refs/tags/${VERSION}"`,
		`)" || return 1`,
		`err "could not verify Gitee tag deletion for $VERSION"`,
		`disabled_gitee_release_id="$(gitee_release_id "$VERSION")"`,
		`DWS_TAP_PR_TITLE="revert: withdraw ${VERSION} and restore ${ROLLBACK_VERSION}"`,
		"Homebrew rollback PR requires independent review and merge",
		`"repos/${OFFICIAL_REPOSITORY}/releases/${TARGET_RELEASE_ID}"`,
		`"repos/${OFFICIAL_REPOSITORY}/git/refs/tags/${VERSION}"`,
		`github_expect_404 "repos/${OFFICIAL_REPOSITORY}/releases/tags/${VERSION}"`,
		`github_expect_404 "repos/${OFFICIAL_REPOSITORY}/git/ref/tags/${VERSION}"`,
		"Already-installed clients cannot be remotely downgraded",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("withdraw script missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"npm unpublish",
		`"repos/${OFFICIAL_REPOSITORY}/git/refs/tags/${TOMBSTONE}"`,
		`--force "refs/tags/${TOMBSTONE}`,
		`make_latest`,
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("withdraw script contains evidence-destroying operation %q", forbidden)
		}
	}

	tombstone := strings.LastIndex(script, "\ncreate_tombstone\n")
	githubMutation := strings.LastIndex(script, "\nupdate_github_release\n")
	npmMutation := strings.LastIndex(script, "\nupdate_npm_channel\n")
	ossMutation := strings.LastIndex(script, `update_oss_channel "$OSS_POINTER_NAME"`)
	homebrewGate := strings.LastIndex(script, "\nupdate_homebrew\n")
	githubDelete := strings.LastIndex(script, "\ndelete_github_release_and_tag\n")
	if tombstone < 0 || githubMutation < 0 || npmMutation < 0 || ossMutation < 0 ||
		homebrewGate < 0 || githubDelete < 0 {
		t.Fatalf("could not locate withdrawal mutation sequence")
	}
	if !(tombstone < homebrewGate && homebrewGate < githubMutation &&
		githubMutation < npmMutation && npmMutation < ossMutation && ossMutation < githubDelete) {
		t.Fatalf("unsafe withdrawal order: tombstone=%d github-mark=%d npm=%d oss=%d homebrew=%d github-delete=%d",
			tombstone, githubMutation, npmMutation, ossMutation, homebrewGate, githubDelete)
	}
}

func TestReleaseTagOSSModeIsImmutableAndFailClosed(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "release-tag-oss-mode.sh"))
	if err != nil {
		t.Fatalf("Abs(OSS mode script) error = %v", err)
	}
	repo := t.TempDir()
	mustRun(t, repo, "git", "init", "-b", "main")
	mustRun(t, repo, "git", "config", "user.name", "OSS Mode Test")
	mustRun(t, repo, "git", "config", "user.email", "oss-mode@example.com")
	mustWriteFile(t, filepath.Join(repo, "tracked"), []byte("fixture\n"), 0o644)
	mustRun(t, repo, "git", "add", "tracked")
	mustRun(t, repo, "git", "commit", "-m", "fixture")

	tag := func(version, message string) {
		t.Helper()
		messagePath := filepath.Join(repo, version+".message")
		mustWriteFile(t, messagePath, []byte(message), 0o644)
		mustRun(t, repo, "git", "tag", "-a", version, "-F", messagePath)
	}
	tag("v1.0.1", "Release v1.0.1\n")
	tag("v1.0.2", "Release v1.0.2\n\nOSS-Mirror: enabled\n")
	tag("v1.0.3", "Release v1.0.3\n\nOSS-Mirror: deferred\n")
	tag("v1.0.4", "Release v1.0.4\n\nOSS-Mirror: invalid\n")
	tag("v1.0.5", "Release v1.0.5\n\nOSS-Mirror: enabled\nOSS-Mirror: deferred\n")
	mustRun(t, repo, "git", "tag", "v1.0.6")
	rawTag := func(version, message string) {
		t.Helper()
		commit := strings.TrimSpace(mustOutput(t, repo, "git", "rev-parse", "HEAD"))
		payload := strings.Join([]string{
			"object " + commit,
			"type commit",
			"tag " + version,
			"tagger OSS Mode Test <oss-mode@example.com> 1700000000 +0000",
			"",
			message,
		}, "\n")
		cmd := exec.Command("git", "mktag")
		cmd.Dir = repo
		cmd.Stdin = strings.NewReader(payload)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git mktag %s error = %v\noutput:\n%s", version, err, output)
		}
		mustRun(t, repo, "git", "update-ref", "refs/tags/"+version, strings.TrimSpace(string(output)))
	}
	rawTag("v1.0.7", "Release v1.0.7\n\nOSS-Mirror: \n")
	rawTag("v1.0.8", "Release v1.0.8\r\n\r\nOSS-Mirror: deferred\r\n")

	for _, test := range []struct {
		version string
		want    string
		ok      bool
	}{
		{version: "v1.0.1", want: "enabled", ok: true},
		{version: "v1.0.2", want: "enabled", ok: true},
		{version: "v1.0.3", want: "deferred", ok: true},
		{version: "v1.0.4", want: "invalid OSS-Mirror metadata", ok: false},
		{version: "v1.0.5", want: "duplicate OSS-Mirror metadata", ok: false},
		{version: "v1.0.6", want: "must be annotated", ok: false},
		{version: "v1.0.7", want: "invalid OSS-Mirror metadata", ok: false},
		{version: "v1.0.8", want: "deferred", ok: true},
	} {
		t.Run(test.version, func(t *testing.T) {
			cmd := exec.Command(scriptPath, test.version)
			cmd.Dir = repo
			output, err := cmd.CombinedOutput()
			if test.ok && err != nil {
				t.Fatalf("release-tag-oss-mode.sh error = %v\noutput:\n%s", err, output)
			}
			if !test.ok && err == nil {
				t.Fatalf("release-tag-oss-mode.sh unexpectedly accepted %s", test.version)
			}
			if !strings.Contains(string(output), test.want) {
				t.Fatalf("release-tag-oss-mode.sh output missing %q:\n%s", test.want, output)
			}
		})
	}
}

func TestWithdrawReleaseDefersOSSRequirementUntilImmutablePolicyResolution(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "withdraw-release.sh"))
	if err != nil {
		t.Fatalf("Abs(withdraw script) error = %v", err)
	}
	cmd := exec.Command("bash", scriptPath,
		"v1.2.3",
		"critical startup regression",
		"WITHDRAW v1.2.3",
	)
	cmd.Env = append(os.Environ(),
		"GITHUB_TOKEN=test-github-token",
		"NODE_AUTH_TOKEN=test-npm-token",
		"GITHUB_REPOSITORY=DingTalk-Real-AI/dingtalk-workspace-cli",
		"GITHUB_REF_NAME=main",
		"GITHUB_SHA="+strings.Repeat("a", 40),
		"GITHUB_RUN_ID=1",
		"GITHUB_ACTOR=test-operator",
		"GITHUB_EVENT_DEFAULT_BRANCH=main",
		"GITHUB_ACTIONS=false",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("withdraw-release.sh unexpectedly ran outside GitHub Actions:\n%s", output)
	}
	if !strings.Contains(string(output), "withdrawal may run only inside GitHub Actions") {
		t.Fatalf("withdrawal did not reach the protected execution gate before resolving OSS policy:\n%s", output)
	}
	if strings.Contains(string(output), "missing required environment variable OSS_") {
		t.Fatalf("withdrawal required OSS credentials before resolving immutable tag policy:\n%s", output)
	}
}

func TestWithdrawReleaseRejectsInvalidInputsBeforeMutation(t *testing.T) {
	t.Parallel()

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "withdraw-release.sh"))
	if err != nil {
		t.Fatalf("Abs(withdraw script) error = %v", err)
	}
	tests := []struct {
		name         string
		version      string
		reason       string
		confirmation string
		want         string
	}{
		{
			name:         "invalid version",
			version:      "1.2.3",
			reason:       "critical startup regression",
			confirmation: "WITHDRAW 1.2.3",
			want:         "version must be exactly",
		},
		{
			name:         "wrong confirmation",
			version:      "v1.2.3",
			reason:       "critical startup regression",
			confirmation: "v1.2.3",
			want:         "confirmation must be exactly: WITHDRAW v1.2.3",
		},
		{
			name:         "multiline reason",
			version:      "v1.2.3",
			reason:       "critical\nregression",
			confirmation: "WITHDRAW v1.2.3",
			want:         "reason must be a trimmed, printable single line",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("bash", scriptPath, test.version, test.reason, test.confirmation)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("withdraw-release.sh unexpectedly accepted invalid input:\n%s", string(output))
			}
			if !strings.Contains(string(output), test.want) {
				t.Fatalf("withdraw-release.sh output missing %q:\n%s", test.want, string(output))
			}
		})
	}
}

func TestWithdrawReleaseRollsBackConfiguredChannelsAndStopsForHomebrewReview(t *testing.T) {
	for _, test := range []struct {
		name        string
		ossDeferred bool
	}{
		{name: "legacy tag defaults OSS to enabled"},
		{name: "deferred OSS survives tombstone retry", ossDeferred: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			testWithdrawReleaseRollsBackConfiguredChannelsAndStopsForHomebrewReview(t, test.ossDeferred)
		})
	}
}

func testWithdrawReleaseRollsBackConfiguredChannelsAndStopsForHomebrewReview(t *testing.T, ossDeferred bool) {
	root := t.TempDir()
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(source root) error = %v", err)
	}

	for _, rel := range []string{
		"scripts/release/withdraw-release.sh",
		"scripts/release/release-tag-oss-mode.sh",
		"scripts/release/release-lib.sh",
		"build/homebrew-release.rb.tmpl",
	} {
		copyTestFile(t, filepath.Join(sourceRoot, rel), filepath.Join(root, rel), 0o755)
	}
	mustWriteFile(t, filepath.Join(root, "scripts", "release", "verify-delivery"), []byte(`#!/bin/sh
printf 'delivery %s %s\n' "$1" "$2" >> "$CALL_LOG"
exit 0
`), 0o755)
	mustWriteFile(t, filepath.Join(root, "scripts", "release", "download-release-assets"), []byte(`#!/bin/sh
set -eu
version="$1"
dist="$2"
mkdir -p "$dist"
for asset in \
  dws-darwin-amd64.tar.gz dws-darwin-arm64.tar.gz \
  dws-linux-amd64.tar.gz dws-linux-arm64.tar.gz \
  dws-windows-amd64.zip dws-windows-arm64.zip \
  dws-skills.zip; do
  printf '%s %s\n' "$version" "$asset" > "$dist/$asset"
done
{
  for asset in \
    dws-darwin-amd64.tar.gz dws-darwin-arm64.tar.gz \
    dws-linux-amd64.tar.gz dws-linux-arm64.tar.gz \
    dws-windows-amd64.zip dws-windows-arm64.zip \
    dws-skills.zip; do
    printf '%064d  %s\n' 0 "$asset"
  done
} > "$dist/checksums.txt"
printf 'download-assets %s\n' "$version" >> "$CALL_LOG"
`), 0o755)
	mustWriteFile(t, filepath.Join(root, "scripts", "release", "verify-release-assets"), []byte(`#!/bin/sh
set -eu
test -f "$DWS_PACKAGE_DIST_DIR/checksums.txt"
printf 'verify-assets %s\n' "$1" >> "$CALL_LOG"
`), 0o755)
	mustWriteFile(t, filepath.Join(root, "scripts", "release", "sync-gitee"), []byte(`#!/bin/sh
set -eu
git push "$GITEE_GIT_REMOTE" "refs/tags/$VERSION:refs/tags/$VERSION" >/dev/null
printf '%s\n' "$VERSION" > "$MOCK_STATE/gitee-release-$VERSION"
printf 'gitee-sync %s\n' "$VERSION" >> "$CALL_LOG"
`), 0o755)
	mustWriteFile(t, filepath.Join(root, "scripts", "release", "publish-homebrew-formula.sh"), []byte(`#!/bin/sh
cp "$DWS_FORMULA_SOURCE" "$MOCK_STATE/homebrew-formula"
printf 'homebrew-pr %s %s\n' "$DWS_TAP_PR_TITLE" "$DWS_TAP_PR_BRANCH" >> "$CALL_LOG"
exit 0
`), 0o755)
	mustWriteFile(t, filepath.Join(root, "scripts", "release", "unexpected-ossutil"), []byte(`#!/bin/sh
printf 'unexpected-oss-call\n' >> "$CALL_LOG"
exit 99
`), 0o755)

	stateDir := filepath.Join(root, "state")
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(state) error = %v", err)
	}
	callLog := filepath.Join(stateDir, "calls.log")
	writeWithdrawMocks(t, fakeBin)

	mustWriteFile(t, filepath.Join(root, "Formula", "dingtalk-workspace-cli.rb"), []byte(`class DingtalkWorkspaceCli < Formula
  version "1.0.51"
end
`), 0o644)
	mustRun(t, root, "git", "init", "-b", "main")
	mustRun(t, root, "git", "config", "user.name", "Withdrawal Test")
	mustRun(t, root, "git", "config", "user.email", "withdrawal@example.com")
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-m", "candidate")
	mustRun(t, root, "git", "tag", "-a", "v1.0.51", "-m", "Release v1.0.51")

	mustWriteFile(t, filepath.Join(root, "Formula", "dingtalk-workspace-cli.rb"), []byte(`class DingtalkWorkspaceCli < Formula
  version "1.0.52"
end
`), 0o644)
	mustRun(t, root, "git", "add", "Formula/dingtalk-workspace-cli.rb")
	mustRun(t, root, "git", "commit", "-m", "target")
	tagArgs := []string{"tag", "-a", "v1.0.52", "-m", "Release v1.0.52"}
	if ossDeferred {
		tagArgs = append(tagArgs, "-m", "OSS-Mirror: deferred")
	}
	mustRun(t, root, "git", tagArgs...)
	targetCommit := strings.TrimSpace(mustOutput(t, root, "git", "rev-parse", "HEAD"))
	targetTagObject := strings.TrimSpace(mustOutput(t, root, "git", "rev-parse", "refs/tags/v1.0.52"))

	origin := filepath.Join(root, "origin.git")
	gitee := filepath.Join(root, "gitee.git")
	mustRun(t, root, "git", "init", "--bare", origin)
	mustRun(t, root, "git", "init", "--bare", gitee)
	mustRun(t, root, "git", "remote", "add", "origin", origin)
	mustRun(t, root, "git", "push", "origin", "main", "v1.0.51", "v1.0.52")
	mustRun(t, root, "git", "push", gitee, "v1.0.52")

	writeJSONFile(t, filepath.Join(stateDir, "release-v1.0.51.json"), map[string]any{
		"id": 51, "tag_name": "v1.0.51", "name": "v1.0.51", "body": "candidate",
		"draft": false, "prerelease": false, "immutable": true,
	})
	writeJSONFile(t, filepath.Join(stateDir, "release-v1.0.52.json"), map[string]any{
		"id": 52, "tag_name": "v1.0.52", "name": "v1.0.52", "body": "target",
		"draft": false, "prerelease": false, "immutable": true,
	})
	mustWriteFile(t, filepath.Join(stateDir, "latest"), []byte("v1.0.52\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "npm-latest"), []byte("1.0.52\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "oss-latest"), []byte("v1.0.52\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "gitee-release-v1.0.52"), []byte("752\n"), 0o644)
	mustWriteFile(t, filepath.Join(stateDir, "github-tag-v1.0.52"), []byte(targetTagObject+"\n"), 0o644)

	scriptPath := filepath.Join(root, "scripts", "release", "withdraw-release.sh")
	cmd := exec.Command("bash", scriptPath,
		"v1.0.52",
		"critical startup regression",
		"WITHDRAW v1.0.52",
	)
	cmd.Dir = root
	withdrawEnv := append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_STATE="+stateDir,
		"CALL_LOG="+callLog,
		"GITHUB_ACTIONS=true",
		"GITHUB_REPOSITORY=DingTalk-Real-AI/dingtalk-workspace-cli",
		"GITHUB_REF_NAME=main",
		"GITHUB_SHA="+targetCommit,
		"GITHUB_RUN_ID=12345",
		"GITHUB_ACTOR=release-operator",
		"GITHUB_EVENT_DEFAULT_BRANCH=main",
		"GITHUB_TOKEN=github-token",
		"NODE_AUTH_TOKEN=npm-token",
		"DWS_GITEE_ENABLED=true",
		"GITEE_TOKEN=gitee-token",
		"GITEE_USER=gitee-user",
		"GITEE_REPO=DingTalk-Real-AI/dingtalk-workspace-cli",
		"GITEE_GIT_REMOTE="+gitee,
		"GITEE_PUBLIC_GIT_REMOTE="+gitee,
		"HOMEBREW_PR_TOKEN=homebrew-token",
		"DWS_DELIVERY_VERIFIER="+filepath.Join(root, "scripts", "release", "verify-delivery"),
		"DWS_GITHUB_DOWNLOAD_HELPER="+filepath.Join(root, "scripts", "release", "download-release-assets"),
		"DWS_ARTIFACT_VERIFY_HELPER="+filepath.Join(root, "scripts", "release", "verify-release-assets"),
		"DWS_GITEE_SYNC_HELPER="+filepath.Join(root, "scripts", "release", "sync-gitee"),
		"ORIGIN_GIT="+origin,
	)
	if ossDeferred {
		withdrawEnv = append(withdrawEnv,
			"OSS_ACCESS_KEY_ID=",
			"OSS_ACCESS_KEY_SECRET=",
			"OSS_ENDPOINT=",
			"OSS_BUCKET=",
			"OSSUTIL="+filepath.Join(root, "scripts", "release", "unexpected-ossutil"),
		)
	} else {
		withdrawEnv = append(withdrawEnv,
			"OSS_ACCESS_KEY_ID=oss-id",
			"OSS_ACCESS_KEY_SECRET=oss-secret",
			"OSS_ENDPOINT=https://oss-cn-hangzhou.aliyuncs.com",
			"OSS_BUCKET=dws-test",
			"OSSUTIL="+filepath.Join(fakeBin, "ossutil"),
		)
	}
	cmd.Env = withdrawEnv
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("withdrawal must remain failed while Homebrew PR is pending:\n%s", string(output))
	}
	if !strings.Contains(string(output), "Homebrew rollback PR requires independent review and merge") {
		t.Fatalf("withdrawal did not report the Homebrew manual gate:\n%s", string(output))
	}

	assertFileEquals(t, filepath.Join(stateDir, "latest"), "v1.0.51")
	assertFileEquals(t, filepath.Join(stateDir, "npm-latest"), "1.0.51")
	assertFileContains(t, filepath.Join(stateDir, "npm-deprecated"), "WITHDRAWN v1.0.52")
	if ossDeferred {
		assertFileEquals(t, filepath.Join(stateDir, "oss-latest"), "v1.0.52")
	} else {
		assertFileEquals(t, filepath.Join(stateDir, "oss-latest"), "v1.0.51")
		if _, err := os.Stat(filepath.Join(stateDir, "oss-removed")); err != nil {
			t.Fatalf("OSS withdrawn prefix was not removed: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "gitee-release-v1.0.52")); !os.IsNotExist(err) {
		t.Fatalf("Gitee release still exists, stat error = %v", err)
	}
	if refs := mustOutput(t, root, "git", "ls-remote", gitee, "refs/tags/v1.0.52"); strings.TrimSpace(refs) != "" {
		t.Fatalf("Gitee still exposes withdrawn tag:\n%s", refs)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "release-v1.0.52.json")); !os.IsNotExist(err) {
		t.Fatalf("GitHub problem release still exists while Homebrew review is pending, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "github-tag-v1.0.52")); !os.IsNotExist(err) {
		t.Fatalf("GitHub problem tag still exists while Homebrew review is pending, stat error = %v", err)
	}
	assertFileContains(t, filepath.Join(stateDir, "tombstone-message"), "Original-Commit: "+targetCommit)
	assertFileContains(t, filepath.Join(stateDir, "tombstone-message"), "Original-Tag-Object: "+targetTagObject)
	assertFileContains(t, filepath.Join(stateDir, "tombstone-message"), "Original-Release-ID: 52")
	ossMode := "enabled"
	if ossDeferred {
		ossMode = "deferred"
	}
	assertFileContains(t, filepath.Join(stateDir, "tombstone-message"), "OSS-Mirror: "+ossMode)
	assertFileContains(t, filepath.Join(stateDir, "tombstone-message"), "Reason: critical startup regression")
	assertFileContains(t, callLog, "homebrew-pr revert: withdraw v1.0.52 and restore v1.0.51")

	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("ReadFile(call log) error = %v", err)
	}
	logText := string(calls)
	tombstoneIndex := strings.Index(logText, "tombstone-ref")
	githubIndex := strings.Index(logText, "github-withdraw")
	npmIndex := strings.Index(logText, "npm-deprecate")
	ossIndex := strings.Index(logText, "oss-remove")
	if tombstoneIndex < 0 || githubIndex < 0 || npmIndex < 0 || (!ossDeferred && ossIndex < 0) {
		t.Fatalf("missing mutation audit entries:\n%s", logText)
	}
	homebrewIndex := strings.Index(logText, "homebrew-pr")
	if !(tombstoneIndex < homebrewIndex && homebrewIndex < githubIndex && githubIndex < npmIndex) ||
		(!ossDeferred && npmIndex >= ossIndex) {
		t.Fatalf("tombstone was not durable before channel mutations:\n%s", logText)
	}
	deleteReleaseIndex := strings.Index(logText, "github-delete-release")
	deleteTagIndex := strings.Index(logText, "github-delete-tag")
	channelsCompleteIndex := npmIndex
	if !ossDeferred {
		channelsCompleteIndex = ossIndex
	}
	if deleteReleaseIndex < 0 || deleteTagIndex < 0 || homebrewIndex < 0 ||
		!(homebrewIndex < channelsCompleteIndex && channelsCompleteIndex < deleteReleaseIndex && deleteReleaseIndex < deleteTagIndex) {
		t.Fatalf("GitHub problem release was not removed before the Homebrew review pause:\n%s", logText)
	}
	if ossDeferred {
		if strings.Contains(logText, "unexpected-oss-call") || ossIndex >= 0 {
			t.Fatalf("deferred withdrawal invoked OSS tooling:\n%s", logText)
		}
		for _, want := range []string{
			"OSS mirroring is disabled; no OSS rollback is required.",
			"OSS did not contain v1.0.52 because mirroring was disabled",
		} {
			if !strings.Contains(string(output), want) {
				t.Fatalf("deferred withdrawal output missing %q:\n%s", want, output)
			}
		}
	}
	if !ossDeferred {
		tombstonePath := filepath.Join(stateDir, "tombstone-message")
		tombstoneMessage, err := os.ReadFile(tombstonePath)
		if err != nil {
			t.Fatalf("ReadFile(legacy tombstone) error = %v", err)
		}
		legacyMessage := strings.Replace(string(tombstoneMessage), "OSS-Mirror: enabled\n", "", 1)
		if legacyMessage == string(tombstoneMessage) {
			t.Fatal("could not convert tombstone fixture to the legacy format")
		}
		mustWriteFile(t, tombstonePath, []byte(legacyMessage), 0o644)
	}

	mergedFormula, err := os.ReadFile(filepath.Join(stateDir, "homebrew-formula"))
	if err != nil {
		t.Fatalf("ReadFile(rendered Homebrew rollback) error = %v", err)
	}
	mustWriteFile(t, filepath.Join(root, "Formula", "dingtalk-workspace-cli.rb"), mergedFormula, 0o644)
	mustRun(t, root, "git", "add", "Formula/dingtalk-workspace-cli.rb")
	mustRun(t, root, "git", "commit", "-m", "merge Homebrew rollback")
	mustRun(t, root, "git", "push", "origin", "main")
	retryCommit := strings.TrimSpace(mustOutput(t, root, "git", "rev-parse", "HEAD"))
	retryEnv := replaceTestEnv(withdrawEnv,
		"GITHUB_SHA", retryCommit,
		"GITHUB_RUN_ID", "12346",
	)
	retry := exec.Command("bash", scriptPath,
		"v1.0.52",
		"critical startup regression",
		"WITHDRAW v1.0.52",
	)
	retry.Dir = root
	retry.Env = retryEnv
	retryOutput, retryErr := retry.CombinedOutput()
	if retryErr != nil {
		t.Fatalf("withdrawal retry after Homebrew merge error = %v\noutput:\n%s", retryErr, string(retryOutput))
	}
	for _, want := range []string{
		"Resuming withdrawal for v1.0.52 from exact permanent tombstone metadata.",
		"Permanent tombstone withdrawn/v1.0.52 already exists",
		"GitHub Release v1.0.52 was already absent.",
		"GitHub tag v1.0.52 was already absent.",
		"Withdrawal completed for all configured distribution channels.",
		"Already-installed clients cannot be remotely downgraded",
	} {
		if !strings.Contains(string(retryOutput), want) {
			t.Fatalf("withdrawal retry output missing %q:\n%s", want, string(retryOutput))
		}
	}
	if ossDeferred && !strings.Contains(string(retryOutput), "OSS mirroring is disabled; no OSS rollback is required.") {
		t.Fatalf("deferred tombstone retry did not restore the sealed OSS policy:\n%s", retryOutput)
	}
	assertFileEquals(t, filepath.Join(stateDir, "latest"), "v1.0.51")
	if _, err := os.Stat(filepath.Join(stateDir, "release-v1.0.52.json")); !os.IsNotExist(err) {
		t.Fatalf("GitHub problem release still exists, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "github-tag-v1.0.52")); !os.IsNotExist(err) {
		t.Fatalf("GitHub problem tag still exists, stat error = %v", err)
	}
	if refs := mustOutput(t, root, "git", "ls-remote", origin, "refs/tags/v1.0.52"); strings.TrimSpace(refs) != "" {
		t.Fatalf("GitHub origin still exposes withdrawn tag:\n%s", refs)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "tombstone-ref")); err != nil {
		t.Fatalf("permanent withdrawal tombstone is missing: %v", err)
	}
	assertFileContains(t, callLog, "github-delete-release")
	assertFileContains(t, callLog, "github-delete-tag")
}

func copyTestFile(t *testing.T, source, destination string, mode os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", source, err)
	}
	mustWriteFile(t, destination, content, mode)
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%s) error = %v", path, err)
	}
	mustWriteFile(t, path, append(content, '\n'), 0o644)
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if got := strings.TrimSpace(string(content)); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("%s missing %q:\n%s", path, want, string(content))
	}
}

func replaceTestEnv(env []string, replacements ...string) []string {
	result := append([]string{}, env...)
	for index := 0; index < len(replacements); index += 2 {
		key := replacements[index]
		value := replacements[index+1]
		prefix := key + "="
		replaced := false
		for envIndex, entry := range result {
			if strings.HasPrefix(entry, prefix) {
				result[envIndex] = prefix + value
				replaced = true
			}
		}
		if !replaced {
			result = append(result, prefix+value)
		}
	}
	return result
}

func writeWithdrawMocks(t *testing.T, fakeBin string) {
	t.Helper()

	mustWriteFile(t, filepath.Join(fakeBin, "gh"), []byte(`#!/usr/bin/env python3
import json
import os
import pathlib
import subprocess
import sys

state = pathlib.Path(os.environ["MOCK_STATE"])
log = pathlib.Path(os.environ["CALL_LOG"])
args = sys.argv[1:]

def record(value):
    with log.open("a", encoding="utf-8") as handle:
        handle.write(value + "\n")

def value_after(flag, default=""):
    try:
        return args[args.index(flag) + 1]
    except (ValueError, IndexError):
        return default

def not_found():
    print("gh: Not Found (HTTP 404)", file=sys.stderr)
    raise SystemExit(1)

if args[:2] == ["release", "download"]:
    output = pathlib.Path(value_after("--dir"))
    output.mkdir(parents=True, exist_ok=True)
    assets = [
        "dws-darwin-amd64.tar.gz",
        "dws-darwin-arm64.tar.gz",
        "dws-linux-amd64.tar.gz",
        "dws-linux-arm64.tar.gz",
        "dws-skills.zip",
    ]
    (output / "checksums.txt").write_text(
        "".join(("a" * 64) + "  " + asset + "\n" for asset in assets),
        encoding="utf-8",
    )
    record("github-download-checksums")
    raise SystemExit(0)

if not args or args[0] != "api":
    raise SystemExit("unsupported gh invocation: " + " ".join(args))

method = value_after("--method", "GET")
skip = {"-H", "--method", "-f", "-F", "--input", "--jq"}
endpoint = ""
i = 1
while i < len(args):
    if args[i] in skip:
        i += 2
        continue
    if args[i].startswith("repos/"):
        endpoint = args[i]
        break
    i += 1
jq = value_after("--jq")

if endpoint.endswith("/git/ref/heads/main"):
    payload = {"object": {"sha": os.environ["GITHUB_SHA"]}}
elif "/releases/tags/" in endpoint:
    version = endpoint.rsplit("/", 1)[1]
    path = state / f"release-{version}.json"
    if not path.exists():
        not_found()
    payload = json.loads(path.read_text(encoding="utf-8"))
elif endpoint.endswith("/releases/latest"):
    payload = {"tag_name": (state / "latest").read_text(encoding="utf-8").strip()}
elif "/git/ref/tags/withdrawn/" in endpoint:
    ref_path = state / "tombstone-ref"
    if not ref_path.exists():
        not_found()
    payload = {"object": {"sha": ref_path.read_text(encoding="utf-8").strip()}}
elif endpoint.endswith("/git/ref/tags/v1.0.52") and method == "GET":
    tag_path = state / "github-tag-v1.0.52"
    if not tag_path.exists():
        not_found()
    payload = {"object": {"sha": tag_path.read_text(encoding="utf-8").strip()}}
elif "/git/tags/" in endpoint and method == "GET":
    payload = {
        "tag": "withdrawn/v1.0.52",
        "message": (state / "tombstone-message").read_text(encoding="utf-8"),
        "object": {
            "type": "commit",
            "sha": (state / "tombstone-target").read_text(encoding="utf-8").strip(),
        },
    }
elif endpoint.endswith("/git/tags") and method == "POST":
    fields = {}
    for index, arg in enumerate(args):
        if arg == "-f":
            key, value = args[index + 1].split("=", 1)
            fields[key] = value
    (state / "tombstone-message").write_text(fields["message"], encoding="utf-8")
    (state / "tombstone-target").write_text(fields["object"] + "\n", encoding="utf-8")
    payload = {"sha": "a" * 40}
    record("tombstone-object")
elif endpoint.endswith("/git/refs") and method == "POST":
    (state / "tombstone-ref").write_text("a" * 40 + "\n", encoding="utf-8")
    payload = {"ref": "refs/tags/withdrawn/v1.0.52", "object": {"sha": "a" * 40}}
    record("tombstone-ref")
elif endpoint.endswith("/releases/52") and method == "DELETE":
    release_path = state / "release-v1.0.52.json"
    if not release_path.exists():
        not_found()
    release_path.unlink()
    (state / "latest").write_text("v1.0.51\n", encoding="utf-8")
    payload = {}
    record("github-delete-release")
elif endpoint.endswith("/git/refs/tags/v1.0.52") and method == "DELETE":
    tag_path = state / "github-tag-v1.0.52"
    if not tag_path.exists():
        not_found()
    tag_path.unlink()
    subprocess.run(
        ["git", f"--git-dir={os.environ['ORIGIN_GIT']}", "update-ref", "-d", "refs/tags/v1.0.52"],
        check=True,
    )
    payload = {}
    record("github-delete-tag")
elif "/releases/" in endpoint and method == "PATCH":
    release_id = endpoint.rsplit("/", 1)[1]
    if release_id == "52":
        path = state / "release-v1.0.52.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        input_path = value_after("--input")
        patch = json.loads(pathlib.Path(input_path).read_text(encoding="utf-8"))
        data.update({"name": patch["name"], "body": patch["body"]})
        path.write_text(json.dumps(data), encoding="utf-8")
        record("github-withdraw")
    payload = {}
else:
    raise SystemExit("unsupported gh api endpoint: " + endpoint + " " + method)

if jq == ".object.sha":
    print(payload["object"]["sha"])
elif jq == ".tag_name":
    print(payload["tag_name"])
elif jq == ".sha":
    print(payload["sha"])
else:
    print(json.dumps(payload))
`), 0o755)

	mustWriteFile(t, filepath.Join(fakeBin, "npm"), []byte(`#!/usr/bin/env python3
import os
import pathlib
import sys

state = pathlib.Path(os.environ["MOCK_STATE"])
log = pathlib.Path(os.environ["CALL_LOG"])
args = sys.argv[1:]

def record(value):
    with log.open("a", encoding="utf-8") as handle:
        handle.write(value + "\n")

if args[0] == "view":
    spec = args[1]
    field = args[2]
    if field == "version":
        print(spec.rsplit("@", 1)[1])
    elif field == "deprecated":
        path = state / "npm-deprecated"
        if path.exists() and spec.endswith("@1.0.52"):
            print(path.read_text(encoding="utf-8").strip())
    elif field == "dist-tags.latest":
        print((state / "npm-latest").read_text(encoding="utf-8").strip())
    else:
        raise SystemExit("unsupported npm view field: " + field)
elif args[0] == "deprecate":
    (state / "npm-deprecated").write_text(args[2] + "\n", encoding="utf-8")
    record("npm-deprecate")
elif args[:2] == ["dist-tag", "add"]:
    version = args[2].rsplit("@", 1)[1]
    (state / "npm-latest").write_text(version + "\n", encoding="utf-8")
    record("npm-pointer")
else:
    raise SystemExit("unsupported npm invocation: " + " ".join(args))
`), 0o755)

	mustWriteFile(t, filepath.Join(fakeBin, "ossutil"), []byte(`#!/usr/bin/env python3
import os
import pathlib
import sys

state = pathlib.Path(os.environ["MOCK_STATE"])
log = pathlib.Path(os.environ["CALL_LOG"])
args = sys.argv[1:]

if args[0] == "cp":
    source, target = args[-2:]
    if source.startswith("oss://"):
        if source.endswith("/latest.txt"):
            stored = state / "oss-latest"
        else:
            stored = state / ("oss-object-" + source.rsplit("/", 1)[1])
        pathlib.Path(target).write_bytes(stored.read_bytes())
    else:
        if target.endswith("/latest.txt"):
            stored = state / "oss-latest"
        else:
            stored = state / ("oss-object-" + target.rsplit("/", 1)[1])
        stored.write_bytes(pathlib.Path(source).read_bytes())
elif args[0] == "rm":
    (state / "oss-removed").write_text("yes\n", encoding="utf-8")
    with log.open("a", encoding="utf-8") as handle:
        handle.write("oss-remove\n")
elif args[0] == "ls":
    raise SystemExit(0)
else:
    raise SystemExit("unsupported ossutil invocation: " + " ".join(args))
`), 0o755)

	mustWriteFile(t, filepath.Join(fakeBin, "curl"), []byte(`#!/usr/bin/env python3
import json
import os
import pathlib
import sys

state = pathlib.Path(os.environ["MOCK_STATE"])
log = pathlib.Path(os.environ["CALL_LOG"])
args = sys.argv[1:]
url = args[-1]
version = "v1.0.51" if "v1.0.51" in url else "v1.0.52"
release = state / ("gitee-release-" + version)
if "-X" in args and args[args.index("-X") + 1] == "DELETE":
    (state / "gitee-release-v1.0.52").unlink(missing_ok=True)
    with log.open("a", encoding="utf-8") as handle:
        handle.write("gitee-delete-release\n")
elif "/releases/tags/" in url:
    output = pathlib.Path(args[args.index("-o") + 1])
    if release.exists():
        release_id = 751 if version == "v1.0.51" else 752
        output.write_text(json.dumps({"id": release_id}), encoding="utf-8")
        print("200", end="")
    else:
        output.write_text('{"message":"Not Found"}', encoding="utf-8")
        print("404", end="")
else:
    raise SystemExit(22)
`), 0o755)
}

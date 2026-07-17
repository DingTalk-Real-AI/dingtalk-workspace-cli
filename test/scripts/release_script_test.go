package scripts_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var releasePlatformAssets = []string{
	"dws-darwin-amd64.tar.gz",
	"dws-darwin-arm64.tar.gz",
	"dws-linux-amd64.tar.gz",
	"dws-linux-arm64.tar.gz",
	"dws-windows-amd64.zip",
	"dws-windows-arm64.zip",
}

func writeVersionedReleaseArchive(t *testing.T, dist, asset, version string) {
	t.Helper()
	stage := t.TempDir()
	binary := "dws"
	if strings.HasSuffix(asset, ".zip") {
		binary = "dws.exe"
	}
	mustWriteFile(t, filepath.Join(stage, binary), []byte("fake release binary\n"+version+"\n"), 0o755)
	if strings.HasSuffix(asset, ".zip") {
		mustRun(t, stage, "zip", "-q", filepath.Join(dist, asset), binary)
		return
	}
	mustRun(t, stage, "tar", "-czf", filepath.Join(dist, asset), binary)
}

func writeReleaseChecksums(t *testing.T, dist string, includeSkills bool) {
	t.Helper()
	assets := append([]string{}, releasePlatformAssets...)
	if includeSkills {
		assets = append(assets, "dws-skills.zip")
	}
	sort.Strings(assets)
	var lines []string
	for _, asset := range assets {
		data, err := os.ReadFile(filepath.Join(dist, asset))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", asset, err)
		}
		lines = append(lines, fmt.Sprintf("%x  %s", sha256.Sum256(data), asset))
	}
	mustWriteFile(t, filepath.Join(dist, "checksums.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func seedVersionedReleaseArtifacts(t *testing.T, dist, version string) {
	t.Helper()
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dist, err)
	}
	for _, asset := range releasePlatformAssets {
		writeVersionedReleaseArchive(t, dist, asset, version)
	}
	mustWriteFile(t, filepath.Join(dist, "dws-skills.zip"), []byte("fake skills\n"), 0o644)
	writeReleaseChecksums(t, dist, true)
}

type releaseTestRepo struct {
	root       string
	remote     string
	contract   string
	prepare    string
	releaseCmd string
	lib        string
	verify     string
}

func newReleaseTestRepo(t *testing.T) *releaseTestRepo {
	t.Helper()
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	base := t.TempDir()
	root := filepath.Join(base, "work")
	remote := filepath.Join(base, "remote.git")
	mustRun(t, base, "git", "init", "--bare", remote)
	mustRun(t, base, "git", "init", "-b", "main", root)
	mustRun(t, root, "git", "config", "user.name", "Release Test")
	mustRun(t, root, "git", "config", "user.email", "release-test@example.com")

	mustWriteFile(t, filepath.Join(root, "CHANGELOG.md"), []byte(releaseChangelog()), 0o644)
	mustWriteFile(t, filepath.Join(root, "seed.txt"), []byte("initial\n"), 0o644)
	mustRun(t, root, "git", "add", ".")
	mustRun(t, root, "git", "commit", "-m", "initial stable")
	mustRun(t, root, "git", "tag", "-a", "v1.0.0", "-m", "Release v1.0.0")
	mustRun(t, root, "git", "remote", "add", "origin", remote)
	mustRun(t, root, "git", "push", "-u", "origin", "main")
	mustRun(t, root, "git", "push", "origin", "v1.0.0")

	return &releaseTestRepo{
		root:       root,
		remote:     remote,
		contract:   filepath.Join(sourceRoot, "scripts", "release", "release-contract.sh"),
		prepare:    filepath.Join(sourceRoot, "scripts", "release", "prepare-changelog.sh"),
		releaseCmd: filepath.Join(sourceRoot, "scripts", "release", "release.sh"),
		lib:        filepath.Join(sourceRoot, "scripts", "release", "release-lib.sh"),
		verify:     filepath.Join(sourceRoot, "scripts", "release", "verify-release-artifacts.sh"),
	}
}

func TestReleaseVersionOrdering(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	lib := filepath.Join(sourceRoot, "scripts", "release", "release-lib.sh")
	tests := []struct {
		candidate string
		baseline  string
		greater   bool
	}{
		{candidate: "v1.0.2", baseline: "v1.0.1", greater: true},
		{candidate: "v1.1.0-beta.1", baseline: "v1.0.9-beta.9", greater: true},
		{candidate: "v1.0.1-beta.2", baseline: "v1.0.1-beta.1", greater: true},
		{candidate: "v1.0.1", baseline: "v1.0.1-beta.9", greater: true},
		{candidate: "v1.0.1-beta.1", baseline: "v1.0.1-beta.2", greater: false},
		{candidate: "v1.0.1-beta.1", baseline: "v1.0.1", greater: false},
		{candidate: "v1.0.1", baseline: "v1.0.1", greater: false},
	}
	for _, test := range tests {
		cmd := exec.Command("sh", "-c", `. "$1"; release_version_is_greater "$2" "$3"`, "sh", lib, test.candidate, test.baseline)
		err := cmd.Run()
		if (err == nil) != test.greater {
			t.Fatalf("release_version_is_greater(%s, %s) error = %v, want greater=%v", test.candidate, test.baseline, err, test.greater)
		}
	}
}

func TestReleaseLibHelpersPreserveCallerVariables(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	lib := filepath.Join(sourceRoot, "scripts", "release", "release-lib.sh")
	changelogPath := filepath.Join(t.TempDir(), "CHANGELOG.md")
	mustWriteFile(t, changelogPath, []byte("## [1.2.3] - 2026-07-16\n\n- Test release.\n"), 0o644)

	script := `
. "$1"
version=caller-version
expected=caller-expected
actual=caller-actual
candidate=caller-candidate
baseline=caller-baseline
changelog=caller-changelog
semver=caller-semver
output=caller-output
tmp=caller-tmp
status=caller-status

release_channel_for_version v1.2.3 >/dev/null
release_validate_version_channel stable v1.2.3
release_core_tag v1.2.3-beta.1 >/dev/null
release_core_is_greater v1.2.4 v1.2.3
release_extract_changelog "$2" 1.2.3 - >/dev/null

printf '%s\n' "$version" "$expected" "$actual" "$candidate" "$baseline" \
  "$changelog" "$semver" "$output" "$tmp" "$status"
`
	output, err := exec.Command("sh", "-c", script, "sh", lib, changelogPath).CombinedOutput()
	if err != nil {
		t.Fatalf("release helper variable isolation error = %v\noutput:\n%s", err, output)
	}
	want := strings.Join([]string{
		"caller-version",
		"caller-expected",
		"caller-actual",
		"caller-candidate",
		"caller-baseline",
		"caller-changelog",
		"caller-semver",
		"caller-output",
		"caller-tmp",
		"caller-status",
	}, "\n") + "\n"
	if string(output) != want {
		t.Fatalf("release helpers changed caller variables:\ngot:\n%s\nwant:\n%s", output, want)
	}
}

func TestReleaseNpmPackingIgnoresLifecycleScripts(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	packageDir := filepath.Join(t.TempDir(), "package")
	marker := filepath.Join(t.TempDir(), "prepack-ran")
	manifest := fmt.Sprintf(`{
  "name": "dingtalk-workspace-cli",
  "version": "1.2.3",
  "scripts": {"prepack": "touch %s"},
  "files": ["README.md"]
}
`, marker)
	mustWriteFile(t, filepath.Join(packageDir, "package.json"), []byte(manifest), 0o644)
	mustWriteFile(t, filepath.Join(packageDir, "README.md"), []byte("test package\n"), 0o644)
	outputTarball := filepath.Join(t.TempDir(), "package.tgz")
	cmd := exec.Command("sh", filepath.Join(sourceRoot, "scripts", "release", "pack-npm-package.sh"), packageDir, outputTarball)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pack-npm-package error = %v\noutput:\n%s", err, output)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "sha512-") {
		t.Fatalf("pack output is not an integrity value: %s", output)
	}
	if _, err := os.Stat(outputTarball); err != nil {
		t.Fatalf("packed tarball missing: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("npm prepack lifecycle unexpectedly ran: %v", err)
	}
}

func TestReleaseNpmStagingRejectsUnexpectedLifecycleScripts(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	dist := filepath.Join(root, "dist")
	mustWriteFile(t, filepath.Join(source, "build", "npm", "install.js"), []byte("\n"), 0o644)
	mustWriteFile(t, filepath.Join(source, "build", "npm", "bin", "dws.js"), []byte("\n"), 0o755)
	mustWriteFile(t, filepath.Join(source, "build", "npm", "README.md"), []byte("test\n"), 0o644)
	mustWriteFile(t, filepath.Join(source, "build", "npm", "package.json.tmpl"), []byte(`{
  "name": "dingtalk-workspace-cli",
  "version": "__VERSION__",
  "bin": {"dws": "./bin/dws.js"},
  "scripts": {"postinstall": "node install.js", "prepublishOnly": "node steal.js"}
}
`), 0o644)
	cmd := exec.Command("sh", filepath.Join(sourceRoot, "scripts", "release", "stage-npm-package.sh"), "v1.2.3")
	cmd.Env = append(os.Environ(), "DWS_PACKAGE_SOURCE_ROOT="+source, "DWS_PACKAGE_DIST_DIR="+dist)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unexpected npm lifecycle scripts") {
		t.Fatalf("unexpected lifecycle script was not rejected: err=%v\noutput:\n%s", err, output)
	}
}

func TestReleaseGitHubAssetSetIsExact(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	binDir := t.TempDir()
	fakeGH := filepath.Join(binDir, "gh")
	mustWriteFile(t, fakeGH, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$GH_ASSETS\"\n"), 0o755)
	exact := strings.Join([]string{
		"dws-windows-arm64.zip",
		"dws-linux-amd64.tar.gz",
		"checksums.txt",
		"dws-skills.zip",
		"dws-darwin-amd64.tar.gz",
		"dws-windows-amd64.zip",
		"dws-linux-arm64.tar.gz",
		"dws-darwin-arm64.tar.gz",
	}, "\n")
	script := filepath.Join(sourceRoot, "scripts", "release", "verify-github-release-assets.sh")
	run := func(assets string) (string, error) {
		cmd := exec.Command("sh", script, "v1.2.3")
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"GITHUB_REPOSITORY=owner/repo",
			"GH_ASSETS=" + assets,
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	if output, err := run(exact); err != nil {
		t.Fatalf("exact GitHub assets rejected: %v\n%s", err, output)
	}
	if output, err := run(exact + "\nmalware.exe"); err == nil || !strings.Contains(output, "exactly the supported assets") {
		t.Fatalf("extra GitHub asset was not rejected: err=%v\n%s", err, output)
	}
}

func TestReleaseDeliveredStableRequiresSuccessfulPublicDelivery(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	r := newReleaseTestRepo(t)
	commit := strings.TrimSpace(mustOutput(t, r.root, "git", "rev-parse", "v1.0.0^{commit}"))
	binDir := t.TempDir()
	fakeCurl := filepath.Join(binDir, "curl")
	mustWriteFile(t, fakeCurl, []byte(`#!/bin/sh
set -eu
for arg in "$@"; do last="$arg"; done
case "$last" in
  */releases/tags/*)
    printf '{"tag_name":"v1.0.0","draft":false,"prerelease":false}\n'
    ;;
  */actions/workflows/release.yml/runs*)
    printf '{"workflow_runs":[{"head_sha":"%s","head_branch":"v1.0.0","conclusion":"%s"}]}\n' "$EXPECTED_COMMIT" "$RUN_CONCLUSION"
    ;;
  *) exit 1 ;;
esac
`), 0o755)
	script := filepath.Join(sourceRoot, "scripts", "release", "verify-delivered-stable.sh")
	run := func(conclusion string) (string, error) {
		cmd := exec.Command("sh", script, "v1.0.0", commit)
		cmd.Dir = r.root
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"DWS_RELEASE_OFFICIAL_REPOSITORY=owner/repo",
			"EXPECTED_COMMIT=" + commit,
			"RUN_CONCLUSION=" + conclusion,
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	if output, err := run("success"); err != nil {
		t.Fatalf("delivered stable was rejected: %v\n%s", err, output)
	}
	if output, err := run("failure"); err == nil || !strings.Contains(output, "did not complete successfully") {
		t.Fatalf("orphan stable tag was not rejected: err=%v\n%s", err, output)
	}
}

func TestReleaseDeliveredStableAcceptsOnlyPinnedSuccessfulRecovery(t *testing.T) {
	const (
		tag         = "v1.0.52"
		commit      = "4e59f9aa7ab057da8d5512ae9818fb66d4c6a045"
		runID       = "29380892754"
		workflowSHA = "434251695c19ebbfe2a2240d2eddce2d56af07b7"
	)
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	root := t.TempDir()
	mustRun(t, root, "git", "init")
	binDir := t.TempDir()
	fakeCurl := filepath.Join(binDir, "curl")
	mustWriteFile(t, fakeCurl, []byte(`#!/bin/sh
set -eu
for arg in "$@"; do last="$arg"; done
case "$last" in
  */releases/tags/*)
    printf '{"tag_name":"v1.0.52","draft":false,"prerelease":false}\n'
    ;;
  */actions/workflows/release.yml/runs*)
    printf '{"workflow_runs":[]}\n'
    ;;
  */actions/runs/29380892754/jobs*)
    printf '{"jobs":[{"name":"release","status":"completed","conclusion":"success","head_sha":"%s"},{"name":"verify-darwin-signatures","status":"completed","conclusion":"success","head_sha":"%s"},{"name":"publish-release","status":"completed","conclusion":"%s","head_sha":"%s"}]}\n' "$WORKFLOW_SHA" "$WORKFLOW_SHA" "$PUBLISH_CONCLUSION" "$WORKFLOW_SHA"
    ;;
  */actions/runs/29380892754)
    printf '{"event":"workflow_dispatch","status":"completed","conclusion":"success","head_sha":"%s","head_branch":"codex/recover-v1.0.52","run_attempt":1,"path":".github/workflows/release.yml","repository":{"full_name":"owner/repo"}}\n' "$RUN_HEAD_SHA"
    ;;
  *) exit 1 ;;
esac
`), 0o755)
	script := filepath.Join(sourceRoot, "scripts", "release", "verify-delivered-stable.sh")
	run := func(runHeadSHA, publishConclusion string) (string, error) {
		cmd := exec.Command("sh", script, tag, commit)
		cmd.Dir = root
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"DWS_RELEASE_OFFICIAL_REPOSITORY=owner/repo",
			"WORKFLOW_SHA=" + workflowSHA,
			"RUN_HEAD_SHA=" + runHeadSHA,
			"PUBLISH_CONCLUSION=" + publishConclusion,
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	if output, err := run(workflowSHA, "success"); err != nil || !strings.Contains(output, "reviewed recovery run "+runID) {
		t.Fatalf("pinned stable recovery was rejected: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(strings.Repeat("0", 40), "success"); err == nil || !strings.Contains(output, "does not match the pinned delivery proof") {
		t.Fatalf("mismatched recovery workflow passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(workflowSHA, "failure"); err == nil || !strings.Contains(output, "missing successful job publish-release") {
		t.Fatalf("failed recovery delivery job passed: err=%v\noutput:\n%s", err, output)
	}
}

func releaseChangelog(sections ...string) string {
	text := "# Changelog\n\n## [Unreleased]\n\n"
	for _, section := range sections {
		text += section
		if !strings.HasSuffix(text, "\n\n") {
			text += "\n"
		}
	}
	text += "## [1.0.0] - 2026-07-01\n\n### Changed\n\n- Initial release.\n"
	return text
}

func betaSection() string {
	return "## [1.0.1-beta.1] - 2026-07-11\n\n### Changed\n\n- Validate the sealed beta candidate.\n\n"
}

func stableSection() string {
	return "## [1.0.1] - 2026-07-11\n\nThis release promotes the sealed `v1.0.1-beta.1` contents to stable.\n\n### Changed\n\n- Publish the validated candidate to the stable channel.\n\n"
}

func (r *releaseTestRepo) commitAndPush(t *testing.T, message string) {
	t.Helper()
	mustRun(t, r.root, "git", "add", ".")
	mustRun(t, r.root, "git", "commit", "-m", message)
	mustRun(t, r.root, "git", "push", "origin", "main")
}

func (r *releaseTestRepo) seedBeta(t *testing.T) {
	t.Helper()
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	mustWriteFile(t, filepath.Join(r.root, "feature.txt"), []byte("sealed candidate\n"), 0o644)
	r.commitAndPush(t, "prepare beta")
	mustRun(t, r.root, "git", "tag", "-a", "v1.0.1-beta.1", "-m", "Release v1.0.1-beta.1", "-m", "Channel: prerelease")
	mustRun(t, r.root, "git", "push", "origin", "v1.0.1-beta.1")
}

func runReleaseScript(t *testing.T, workdir, script string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE=1")
	if remote, err := exec.Command("git", "-C", workdir, "remote", "get-url", "origin").Output(); err == nil {
		cmd.Env = append(cmd.Env, "DWS_RELEASE_OFFICIAL_TAGS_URL="+strings.TrimSpace(string(remote)))
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestReleaseContractAcceptsPrereleaseAndWritesNotes(t *testing.T) {
	r := newReleaseTestRepo(t)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "prepare beta changelog")

	notes := filepath.Join(t.TempDir(), "notes.md")
	output, err := runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "prerelease",
		"--version", "v1.0.1-beta.1",
		"--context", "local",
		"--remote", "origin",
		"--notes-output", notes,
	)
	if err != nil {
		t.Fatalf("release contract error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "Release contract passed") {
		t.Fatalf("contract output missing success:\n%s", output)
	}
	notesData, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("ReadFile(notes) error = %v", err)
	}
	if !strings.Contains(string(notesData), "Validate the sealed beta candidate") {
		t.Fatalf("notes did not come from exact changelog section:\n%s", notesData)
	}
}

func TestReleaseContractRejectsInvalidVersionChannelPairs(t *testing.T) {
	r := newReleaseTestRepo(t)
	tests := []struct {
		channel string
		version string
	}{
		{channel: "prerelease", version: "v1.0.1"},
		{channel: "stable", version: "v1.0.1-beta.1"},
		{channel: "prerelease", version: "v1.0.1-rc.1"},
		{channel: "prerelease", version: "v1.0.1-preview"},
		{channel: "stable", version: "1.0.1"},
		{channel: "stable", version: "v01.0.1"},
		{channel: "prerelease", version: "v1.0.1-beta.0"},
		{channel: "prerelease", version: "v1.0.1-beta.2"},
	}
	for _, test := range tests {
		t.Run(test.channel+"_"+strings.ReplaceAll(test.version, ".", "_"), func(t *testing.T) {
			output, err := runReleaseScript(t, r.root, r.contract,
				"--repo-root", r.root,
				"--channel", test.channel,
				"--version", test.version,
			)
			if err == nil {
				t.Fatalf("invalid pair unexpectedly passed:\n%s", output)
			}
		})
	}
}

func TestReleaseContractRejectsBadChangelogSections(t *testing.T) {
	tests := []struct {
		name     string
		sections string
		want     string
	}{
		{name: "missing", sections: "", want: "exactly one section"},
		{name: "empty", sections: "## [1.0.1-beta.1] - 2026-07-11\n\n", want: "must contain release notes"},
		{name: "placeholder", sections: "## [1.0.1-beta.1] - 2026-07-11\n\n### Changed\n\n- TODO: write this.\n\n", want: "TODO/TBD"},
		{name: "duplicate", sections: betaSection() + betaSection(), want: "exactly one section"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := newReleaseTestRepo(t)
			mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(test.sections)), 0o644)
			mustWriteFile(t, filepath.Join(r.root, "candidate.txt"), []byte(test.name+"\n"), 0o644)
			r.commitAndPush(t, "write candidate changelog")
			output, err := runReleaseScript(t, r.root, r.contract,
				"--repo-root", r.root,
				"--channel", "prerelease",
				"--version", "v1.0.1-beta.1",
				"--remote", "origin",
			)
			if err == nil {
				t.Fatalf("bad changelog unexpectedly passed:\n%s", output)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("output missing %q:\n%s", test.want, output)
			}
		})
	}
}

func TestReleaseContractRejectsDirtyOrUnsyncedMain(t *testing.T) {
	r := newReleaseTestRepo(t)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "prepare beta")

	mustWriteFile(t, filepath.Join(r.root, "dirty.txt"), []byte("dirty\n"), 0o644)
	output, err := runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "prerelease",
		"--version", "v1.0.1-beta.1",
		"--remote", "origin",
	)
	if err == nil || !strings.Contains(output, "worktree must be clean") {
		t.Fatalf("dirty worktree was not blocked: err=%v\noutput:\n%s", err, output)
	}

	mustRun(t, r.root, "git", "add", "dirty.txt")
	mustRun(t, r.root, "git", "commit", "-m", "local commit not pushed")
	output, err = runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "prerelease",
		"--version", "v1.0.1-beta.1",
		"--remote", "origin",
	)
	if err == nil || !strings.Contains(output, "must exactly match origin/main") {
		t.Fatalf("unsynced main was not blocked: err=%v\noutput:\n%s", err, output)
	}
}

func TestReleaseContractStablePromotionAllowsOnlyChangelogDiff(t *testing.T) {
	t.Run("sealed", func(t *testing.T) {
		r := newReleaseTestRepo(t)
		r.seedBeta(t)
		mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(stableSection(), betaSection())), 0o644)
		r.commitAndPush(t, "prepare stable changelog")

		output, err := runReleaseScript(t, r.root, r.contract,
			"--repo-root", r.root,
			"--channel", "stable",
			"--version", "v1.0.1",
			"--from-beta", "v1.0.1-beta.1",
			"--remote", "origin",
		)
		if err != nil {
			t.Fatalf("sealed stable promotion error = %v\noutput:\n%s", err, output)
		}
	})

	t.Run("source drift", func(t *testing.T) {
		r := newReleaseTestRepo(t)
		r.seedBeta(t)
		mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(stableSection(), betaSection())), 0o644)
		mustWriteFile(t, filepath.Join(r.root, "drift.txt"), []byte("untested change\n"), 0o644)
		r.commitAndPush(t, "drift after beta")

		output, err := runReleaseScript(t, r.root, r.contract,
			"--repo-root", r.root,
			"--channel", "stable",
			"--version", "v1.0.1",
			"--from-beta", "v1.0.1-beta.1",
			"--remote", "origin",
		)
		if err == nil {
			t.Fatalf("drifted stable promotion unexpectedly passed:\n%s", output)
		}
		if !strings.Contains(output, "only CHANGELOG.md may differ") || !strings.Contains(output, "drift.txt") {
			t.Fatalf("drift output is not actionable:\n%s", output)
		}
	})
}

func TestReleaseContractCIReadsStableBaselineFromAnnotatedTag(t *testing.T) {
	r := newReleaseTestRepo(t)
	r.seedBeta(t)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(stableSection(), betaSection())), 0o644)
	r.commitAndPush(t, "prepare stable changelog")
	mustRun(t, r.root, "git", "tag", "-a", "v1.0.1", "-m", "Release v1.0.1", "-m", "Channel: stable", "-m", "From-Beta: v1.0.1-beta.1")
	mustRun(t, r.root, "git", "push", "origin", "v1.0.1")
	mustRun(t, r.root, "git", "checkout", "--detach", "v1.0.1")

	metadata := filepath.Join(t.TempDir(), "metadata")
	output, err := runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "stable",
		"--version", "v1.0.1",
		"--context", "ci",
		"--remote", "origin",
		"--metadata-output", metadata,
	)
	if err != nil {
		t.Fatalf("CI stable contract error = %v\noutput:\n%s", err, output)
	}
	metadataData, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatalf("ReadFile(metadata) error = %v", err)
	}
	if !strings.Contains(string(metadataData), "from_beta=v1.0.1-beta.1") {
		t.Fatalf("metadata missing annotated tag baseline:\n%s", metadataData)
	}
}

func TestReleaseContractCIRejectsLightweightTag(t *testing.T) {
	r := newReleaseTestRepo(t)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "prepare beta")
	mustRun(t, r.root, "git", "tag", "v1.0.1-beta.1")
	mustRun(t, r.root, "git", "push", "origin", "v1.0.1-beta.1")

	output, err := runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "prerelease",
		"--version", "v1.0.1-beta.1",
		"--context", "ci",
		"--remote", "origin",
	)
	if err == nil {
		t.Fatalf("lightweight release tag unexpectedly passed:\n%s", output)
	}
	if !strings.Contains(output, "must be annotated") {
		t.Fatalf("output missing annotated-tag guidance:\n%s", output)
	}
}

func TestReleaseContractCIAllowsMainToAdvanceAfterTagSeal(t *testing.T) {
	r := newReleaseTestRepo(t)
	r.seedBeta(t)
	mustWriteFile(t, filepath.Join(r.root, "after-seal.txt"), []byte("main advanced\n"), 0o644)
	r.commitAndPush(t, "advance main after beta seal")
	mustRun(t, r.root, "git", "checkout", "--detach", "v1.0.1-beta.1")

	output, err := runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "prerelease",
		"--version", "v1.0.1-beta.1",
		"--context", "ci",
		"--remote", "origin",
	)
	if err != nil {
		t.Fatalf("sealed tag should remain valid after main advances: %v\noutput:\n%s", err, output)
	}
}

func TestReleasePrepareChangelogCreatesGuardedTemplate(t *testing.T) {
	r := newReleaseTestRepo(t)
	releaseCopyFile(t, r.lib, filepath.Join(r.root, "scripts", "release", "release-lib.sh"), 0o644)
	releaseCopyFile(t, r.prepare, filepath.Join(r.root, "scripts", "release", "prepare-changelog.sh"), 0o755)
	r.commitAndPush(t, "install changelog preparation")

	cmd := exec.Command("sh", filepath.Join(r.root, "scripts", "release", "prepare-changelog.sh"), "prerelease", "v1.0.1-beta.1")
	cmd.Dir = r.root
	cmd.Env = append(os.Environ(), "DWS_RELEASE_DATE=2026-07-11")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare changelog error = %v\noutput:\n%s", err, output)
	}
	changelog, err := os.ReadFile(filepath.Join(r.root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("ReadFile(CHANGELOG.md) error = %v", err)
	}
	if !strings.Contains(string(changelog), "## [1.0.1-beta.1] - 2026-07-11") || !strings.Contains(string(changelog), "TODO") {
		t.Fatalf("prepared changelog missing guarded template:\n%s", changelog)
	}

	r.commitAndPush(t, "commit unfinished release notes")
	contractOutput, contractErr := runReleaseScript(t, r.root, r.contract,
		"--repo-root", r.root,
		"--channel", "prerelease",
		"--version", "v1.0.1-beta.1",
		"--remote", "origin",
	)
	if contractErr == nil || !strings.Contains(contractOutput, "TODO/TBD") {
		t.Fatalf("unfinished template was not blocked: err=%v\noutput:\n%s", contractErr, contractOutput)
	}
}

func TestReleasePrepareChangelogKeepsUnreleasedContentAboveNewVersion(t *testing.T) {
	r := newReleaseTestRepo(t)
	releaseCopyFile(t, r.lib, filepath.Join(r.root, "scripts", "release", "release-lib.sh"), 0o644)
	releaseCopyFile(t, r.prepare, filepath.Join(r.root, "scripts", "release", "prepare-changelog.sh"), 0o755)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte("# Changelog\n\n## [Unreleased]\n\n### Changed\n\n- Keep this unreleased note.\n\n## [1.0.0] - 2026-07-01\n\n### Changed\n\n- Initial release.\n"), 0o644)
	r.commitAndPush(t, "add unreleased changelog note")

	cmd := exec.Command("sh", filepath.Join(r.root, "scripts", "release", "prepare-changelog.sh"), "prerelease", "v1.0.1-beta.1")
	cmd.Dir = r.root
	cmd.Env = append(os.Environ(), "DWS_RELEASE_DATE=2026-07-11")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare changelog error = %v\noutput:\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(r.root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("ReadFile(CHANGELOG.md) error = %v", err)
	}
	content := string(data)
	positions := []int{
		strings.Index(content, "## [Unreleased]"),
		strings.Index(content, "- Keep this unreleased note."),
		strings.Index(content, "## [1.0.1-beta.1] - 2026-07-11"),
		strings.Index(content, "## [1.0.0] - 2026-07-01"),
	}
	for index, position := range positions {
		if position < 0 {
			t.Fatalf("prepared changelog is missing expected marker %d:\n%s", index, content)
		}
	}
	for index := 1; index < len(positions); index++ {
		if positions[index-1] >= positions[index] {
			t.Fatalf("prepared changelog order is invalid: %v\n%s", positions, content)
		}
	}
}

func TestReleaseMirrorUsesChannelSpecificPointer(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	script := filepath.Join(sourceRoot, "scripts", "release", "sync-to-oss.sh")

	for _, test := range []struct {
		name      string
		version   string
		channel   string
		want      string
		doNotWant string
		installer bool
	}{
		{name: "prerelease", version: "v1.2.3-beta.1", channel: "prerelease", want: "/beta.txt", doNotWant: "/latest.txt", installer: false},
		{name: "stable", version: "v1.2.3", channel: "stable", want: "/latest.txt", doNotWant: "/beta.txt", installer: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			dist := filepath.Join(root, "dist")
			seedVersionedReleaseArtifacts(t, dist, test.version)
			logPath := filepath.Join(root, "ossutil.log")
			argsLogPath := filepath.Join(root, "ossutil-args.log")
			fakeOSSUtil := filepath.Join(root, "ossutil")
			mustWriteFile(t, fakeOSSUtil, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" >> \"$OSSUTIL_ARGS_LOG\"\nprevious=\npenultimate=\nfor arg in \"$@\"; do penultimate=\"$previous\"; previous=\"$arg\"; done\nlast=\"$previous\"\ncase \"$penultimate\" in oss://*) echo 'ErrorCode=NoSuchKey' >&2; exit 1 ;; esac\nprintf '%s\\n' \"$last\" >> \"$OSSUTIL_LOG\"\n"), 0o755)

			cmd := exec.Command("bash", script)
			cmd.Dir = sourceRoot
			cmd.Env = append(os.Environ(),
				"DIST_DIR="+dist,
				"VERSION="+test.version,
				"DWS_RELEASE_CHANNEL="+test.channel,
				"OSS_ACCESS_KEY_ID=test-key",
				"OSS_ACCESS_KEY_SECRET=test-secret",
				"OSS_ENDPOINT=https://oss.example.com",
				"OSS_BUCKET=test-bucket",
				"OSS_PREFIX=dws",
				"OSSUTIL="+fakeOSSUtil,
				"OSSUTIL_LOG="+logPath,
				"OSSUTIL_ARGS_LOG="+argsLogPath,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sync-to-oss error = %v\noutput:\n%s", err, output)
			}
			logData, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("ReadFile(ossutil log) error = %v", err)
			}
			if !strings.Contains(string(logData), test.want) {
				t.Fatalf("mirror log missing %q:\n%s", test.want, logData)
			}
			if strings.Contains(string(logData), test.doNotWant) {
				t.Fatalf("mirror log unexpectedly contains %q:\n%s", test.doNotWant, logData)
			}
			hasInstaller := strings.Contains(string(logData), "/dws/install.sh")
			if hasInstaller != test.installer {
				t.Fatalf("installer upload = %v, want %v:\n%s", hasInstaller, test.installer, logData)
			}
			argsData, err := os.ReadFile(argsLogPath)
			if err != nil {
				t.Fatalf("ReadFile(ossutil args log) error = %v", err)
			}
			for _, forbidden := range []string{"test-key", "test-secret", "--access-key-id", "--access-key-secret"} {
				if strings.Contains(string(argsData), forbidden) {
					t.Fatalf("ossutil argv exposed %q:\n%s", forbidden, argsData)
				}
			}
		})
	}
}

func TestReleaseMirrorFailsClosedWhenPointerCannotBeRead(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	script := filepath.Join(sourceRoot, "scripts", "release", "sync-to-oss.sh")

	for _, test := range []struct {
		name       string
		readAction string
		want       string
	}{
		{name: "transport error", readAction: "echo 'connection timed out' >&2; exit 7", want: "Could not read OSS beta.txt"},
		{name: "empty pointer", readAction: ": > \"$last\"; exit 0", want: "read successfully but is empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			dist := filepath.Join(root, "dist")
			seedVersionedReleaseArtifacts(t, dist, "v1.2.3-beta.1")
			logPath := filepath.Join(root, "ossutil.log")
			fakeOSSUtil := filepath.Join(root, "ossutil")
			scriptText := "#!/bin/sh\nset -eu\nprevious=\npenultimate=\nfor arg in \"$@\"; do penultimate=\"$previous\"; previous=\"$arg\"; done\nlast=\"$previous\"\ncase \"$penultimate\" in oss://*) " + test.readAction + " ;; esac\nprintf '%s\\n' \"$last\" >> \"$OSSUTIL_LOG\"\n"
			mustWriteFile(t, fakeOSSUtil, []byte(scriptText), 0o755)

			cmd := exec.Command("bash", script)
			cmd.Dir = sourceRoot
			cmd.Env = append(os.Environ(),
				"DIST_DIR="+dist,
				"VERSION=v1.2.3-beta.1",
				"DWS_RELEASE_CHANNEL=prerelease",
				"OSS_ACCESS_KEY_ID=test-key",
				"OSS_ACCESS_KEY_SECRET=test-secret",
				"OSS_ENDPOINT=https://oss.example.com",
				"OSS_BUCKET=test-bucket",
				"OSS_PREFIX=dws",
				"OSSUTIL="+fakeOSSUtil,
				"OSSUTIL_LOG="+logPath,
			)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("pointer failure was not closed: err=%v, want=%q\noutput:\n%s", err, test.want, output)
			}
			if logData, readErr := os.ReadFile(logPath); readErr == nil && strings.Contains(string(logData), "/beta.txt") {
				t.Fatalf("failed pointer read still published beta pointer:\n%s", logData)
			}
		})
	}
}

func TestReleaseMirrorRepairsHistoricalAssetsWithoutMovingNewerPointer(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	seedVersionedReleaseArtifacts(t, dist, "v1.2.3-beta.1")
	logPath := filepath.Join(root, "ossutil.log")
	fakeOSSUtil := filepath.Join(root, "ossutil")
	mustWriteFile(t, fakeOSSUtil, []byte("#!/bin/sh\nset -eu\nprevious=\npenultimate=\nfor arg in \"$@\"; do penultimate=\"$previous\"; previous=\"$arg\"; done\nlast=\"$previous\"\ncase \"$penultimate\" in oss://*) printf 'v1.2.4-beta.1\\n' > \"$last\"; exit 0 ;; esac\nprintf '%s\\n' \"$last\" >> \"$OSSUTIL_LOG\"\n"), 0o755)
	cmd := exec.Command("bash", filepath.Join(sourceRoot, "scripts", "release", "sync-to-oss.sh"))
	cmd.Dir = sourceRoot
	cmd.Env = append(os.Environ(),
		"DIST_DIR="+dist,
		"VERSION=v1.2.3-beta.1",
		"DWS_RELEASE_CHANNEL=prerelease",
		"OSS_ACCESS_KEY_ID=test-key",
		"OSS_ACCESS_KEY_SECRET=test-secret",
		"OSS_ENDPOINT=https://oss.example.com",
		"OSS_BUCKET=test-bucket",
		"OSS_PREFIX=dws",
		"OSSUTIL="+fakeOSSUtil,
		"OSSUTIL_LOG="+logPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "assets will be repaired without moving it") {
		t.Fatalf("historical OSS repair failed: err=%v\noutput:\n%s", err, output)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(ossutil log) error = %v", err)
	}
	if strings.Contains(string(logData), "/beta.txt") {
		t.Fatalf("historical repair moved beta pointer:\n%s", logData)
	}
	if !strings.Contains(string(logData), "/download/v1.2.3-beta.1/") {
		t.Fatalf("historical repair did not upload target assets:\n%s", logData)
	}
}

func TestReleaseRequiredMirrorsFailWithoutCredentials(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	for _, test := range []struct {
		name    string
		script  string
		require string
		want    string
	}{
		{name: "oss", script: "sync-to-oss.sh", require: "DWS_REQUIRE_OSS=1", want: "OSS mirror sync is required"},
		{name: "gitee", script: "sync-to-gitee.sh", require: "DWS_REQUIRE_GITEE=1", want: "Gitee mirror sync is enabled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("bash", filepath.Join(sourceRoot, "scripts", "release", test.script))
			cmd.Dir = sourceRoot
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), test.require}
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("required mirror did not fail closed: err=%v, want=%q\noutput:\n%s", err, test.want, output)
			}
		})
	}
}

func TestReleaseArtifactVerificationRequiresEveryChecksum(t *testing.T) {
	r := newReleaseTestRepo(t)
	dist := t.TempDir()
	seedVersionedReleaseArtifacts(t, dist, "v1.2.3")
	cmd := exec.Command("sh", r.verify, "v1.2.3")
	cmd.Env = append(os.Environ(), "DWS_PACKAGE_DIST_DIR="+dist)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("artifact verification error = %v\noutput:\n%s", err, output)
	}

	writeReleaseChecksums(t, dist, false)
	cmd = exec.Command("sh", r.verify, "v1.2.3")
	cmd.Env = append(os.Environ(), "DWS_PACKAGE_DIST_DIR="+dist)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "dws-skills.zip exactly once") {
		t.Fatalf("missing checksum was not blocked: err=%v\noutput:\n%s", err, output)
	}

	writeReleaseChecksums(t, dist, true)
	mustWriteFile(t, filepath.Join(dist, "dws-linux-riscv64.tar.gz"), []byte("unexpected\n"), 0o644)
	cmd = exec.Command("sh", r.verify, "v1.2.3")
	cmd.Env = append(os.Environ(), "DWS_PACKAGE_DIST_DIR="+dist)
	if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "public release assets") {
		t.Fatalf("extra public archive was not rejected: err=%v\noutput:\n%s", err, output)
	}
	if err := os.Remove(filepath.Join(dist, "dws-linux-riscv64.tar.gz")); err != nil {
		t.Fatalf("Remove(extra archive) error = %v", err)
	}

	writeVersionedReleaseArchive(t, dist, "dws-windows-arm64.zip", "v1.2.2")
	writeReleaseChecksums(t, dist, true)
	cmd = exec.Command("sh", r.verify, "v1.2.3")
	cmd.Env = append(os.Environ(), "DWS_PACKAGE_DIST_DIR="+dist)
	if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "dws-windows-arm64.zip binary") {
		t.Fatalf("mixed-version archive was not rejected: err=%v\noutput:\n%s", err, output)
	}
}

func TestReleaseCommandValidatesThenPushesAnnotatedTag(t *testing.T) {
	r := newReleaseTestRepo(t)
	installReleaseCommandFixture(t, r)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "install release automation")

	output, err := runReleaseScript(t, r.root, filepath.Join(r.root, "scripts", "release", "release.sh"),
		"prerelease", "v1.0.1-beta.1", "--remote", "origin",
	)
	if err != nil {
		t.Fatalf("release validation error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "No tag was created") {
		t.Fatalf("validation output missing dry-run result:\n%s", output)
	}
	if mustOutput(t, r.root, "git", "tag", "--list", "v1.0.1-beta.1") != "" {
		t.Fatal("validation-only release created a tag")
	}

	output, err = runReleaseScript(t, r.root, filepath.Join(r.root, "scripts", "release", "release.sh"),
		"prerelease", "v1.0.1-beta.1", "--remote", "origin", "--publish", "--yes",
	)
	if err != nil {
		t.Fatalf("release publish error = %v\noutput:\n%s", err, output)
	}
	if got := strings.TrimSpace(mustOutput(t, r.root, "git", "cat-file", "-t", "v1.0.1-beta.1")); got != "tag" {
		t.Fatalf("release tag type = %q, want annotated tag", got)
	}
	if got := mustOutput(t, r.root, "git", "ls-remote", "--tags", "origin", "refs/tags/v1.0.1-beta.1"); !strings.Contains(got, "refs/tags/v1.0.1-beta.1") {
		t.Fatalf("remote release tag missing:\n%s", got)
	}
}

func TestReleaseCommandCleansLocalTagWhenPushFails(t *testing.T) {
	r := newReleaseTestRepo(t)
	installReleaseCommandFixture(t, r)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "install release automation")

	hook := filepath.Join(r.remote, "hooks", "pre-receive")
	mustWriteFile(t, hook, []byte("#!/bin/sh\nset -eu\nwhile read -r old new ref; do\n  case \"$ref\" in refs/tags/*) exit 1 ;; esac\ndone\nexit 0\n"), 0o755)

	output, err := runReleaseScript(t, r.root, filepath.Join(r.root, "scripts", "release", "release.sh"),
		"prerelease", "v1.0.1-beta.1", "--remote", "origin", "--publish", "--yes",
	)
	if err == nil {
		t.Fatalf("rejected tag push unexpectedly passed:\n%s", output)
	}
	if !strings.Contains(output, "new local tag was removed") {
		t.Fatalf("push failure output missing cleanup result:\n%s", output)
	}
	if mustOutput(t, r.root, "git", "tag", "--list", "v1.0.1-beta.1") != "" {
		t.Fatal("failed push left the local release tag behind")
	}
}

func TestReleaseCommandRejectsDifferentFetchAndPushRepositories(t *testing.T) {
	r := newReleaseTestRepo(t)
	installReleaseCommandFixture(t, r)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	r.commitAndPush(t, "install release automation")
	otherRemote := filepath.Join(t.TempDir(), "other.git")
	mustRun(t, r.root, "git", "init", "--bare", otherRemote)
	mustRun(t, r.root, "git", "remote", "set-url", "--push", "origin", otherRemote)

	output, err := runReleaseScript(t, r.root, filepath.Join(r.root, "scripts", "release", "release.sh"),
		"prerelease", "v1.0.1-beta.1", "--remote", "origin",
	)
	if err == nil || !strings.Contains(output, "fetch and push URLs target different repositories") {
		t.Fatalf("split release authority was not rejected: err=%v\noutput:\n%s", err, output)
	}
}

func TestReleaseCommandRechecksAdvancedStableAuthority(t *testing.T) {
	r := newReleaseTestRepo(t)
	installReleaseCommandFixture(t, r)
	section := "## [1.0.2-beta.1] - 2026-07-11\n\n### Changed\n\n- Validate a candidate after stable authority advances.\n\n"
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(section)), 0o644)
	mustWriteFile(t, filepath.Join(r.root, "scripts", "policy", "check-command-compatibility.sh"), []byte("#!/bin/sh\nset -eu\nprintf 'compatibility %s\\n' \"$*\"\n"), 0o755)
	mustWriteFile(t, filepath.Join(r.root, "Makefile"), []byte("test:\n\t@:\npolicy:\n\t@:\npackage:\n\t@git tag -a v1.0.1 -m 'Release v1.0.1'\n\t@git push origin refs/tags/v1.0.1\n"), 0o644)
	r.commitAndPush(t, "install advancing release fixture")

	output, err := runReleaseScript(t, r.root, filepath.Join(r.root, "scripts", "release", "release.sh"),
		"prerelease", "v1.0.2-beta.1", "--remote", "origin",
	)
	if err != nil {
		t.Fatalf("release validation error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "Stable authority advanced from v1.0.0 to v1.0.1") ||
		!strings.Contains(output, "--stable-ref v1.0.1") {
		t.Fatalf("advanced stable command tree was not rechecked:\n%s", output)
	}
}

func installReleaseCommandFixture(t *testing.T, r *releaseTestRepo) {
	t.Helper()
	for _, source := range []string{r.lib, r.contract, r.releaseCmd} {
		releaseCopyFile(t, source, filepath.Join(r.root, "scripts", "release", filepath.Base(source)), 0o755)
	}
	mustWriteFile(t, filepath.Join(r.root, "scripts", "release", "verify-package-managers.sh"), []byte("#!/bin/sh\nset -eu\nexit 0\n"), 0o755)
	mustWriteFile(t, filepath.Join(r.root, "scripts", "release", "verify-release-artifacts.sh"), []byte("#!/bin/sh\nset -eu\nexit 0\n"), 0o755)
	mustWriteFile(t, filepath.Join(r.root, "scripts", "policy", "check-command-compatibility.sh"), []byte("#!/bin/sh\nset -eu\nexit 0\n"), 0o755)
	mustWriteFile(t, filepath.Join(r.root, "Makefile"), []byte("test:\n\t@:\npolicy:\n\t@:\npackage:\n\t@:\n"), 0o644)
}

func releaseCopyFile(t *testing.T, source, target string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", source, err)
	}
	mustWriteFile(t, target, data, mode)
}

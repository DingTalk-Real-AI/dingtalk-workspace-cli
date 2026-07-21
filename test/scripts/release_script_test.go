package scripts_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
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
	mustWriteFile(t, fakeGH, []byte(`#!/bin/sh
set -eu
case "${1:-}" in
  release)
    [ "${2:-}" = "view" ] || exit 2
    [ "${GH_DRAFT:-false}" = "true" ] || exit 1
    printf '123\n'
    ;;
  api)
    printf '%s\n' "$*" >> "$GH_LOG"
    case "$*" in
      *tag_name*) printf '%s\n' "$GH_RELEASE_TAG" ;;
      *assets*name*) printf '%s\n' "$GH_ASSETS" ;;
      *) exit 2 ;;
    esac
    ;;
  *) exit 2 ;;
esac
`), 0o755)
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
	run := func(assets string, draft bool, releaseTag, releaseID string) (string, string, error) {
		draftValue := "false"
		if draft {
			draftValue = "true"
		}
		logPath := filepath.Join(t.TempDir(), "gh.log")
		cmd := exec.Command("sh", script, "v1.2.3")
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"GITHUB_REPOSITORY=owner/repo",
			"GH_ASSETS=" + assets,
			"GH_DRAFT=" + draftValue,
			"GH_LOG=" + logPath,
			"GH_RELEASE_TAG=" + releaseTag,
			"DWS_GITHUB_RELEASE_ID=" + releaseID,
		}
		output, err := cmd.CombinedOutput()
		log, readErr := os.ReadFile(logPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatalf("ReadFile(%s) error = %v", logPath, readErr)
		}
		return string(output), string(log), err
	}
	if output, _, err := run(exact, false, "v1.2.3", ""); err != nil {
		t.Fatalf("exact GitHub assets rejected: %v\n%s", err, output)
	}
	if output, log, err := run(exact, true, "v1.2.3", ""); err != nil {
		t.Fatalf("exact Draft GitHub assets rejected: %v\n%s", err, output)
	} else if !strings.Contains(log, "repos/owner/repo/releases/123") ||
		strings.Contains(log, "repos/owner/repo/releases/tags/v1.2.3") {
		t.Fatalf("Draft verification did not use the release ID endpoint:\n%s", log)
	}
	if output, _, err := run(exact+"\nmalware.exe", false, "v1.2.3", ""); err == nil || !strings.Contains(output, "exactly the supported assets") {
		t.Fatalf("extra GitHub asset was not rejected: err=%v\n%s", err, output)
	}
	if output, _, err := run(exact, true, "v9.9.9", ""); err == nil || !strings.Contains(output, "ID/tag mismatch") {
		t.Fatalf("wrong Draft release ID target was not rejected: err=%v\n%s", err, output)
	}
	if output, _, err := run(exact, false, "v1.2.3", "invalid"); err == nil || !strings.Contains(output, "invalid GitHub Release ID") {
		t.Fatalf("invalid explicit release ID was not rejected: err=%v\n%s", err, output)
	}
}

func TestReleaseGitHubAssetsDownloadByExactReleaseID(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	binDir := t.TempDir()
	fakeGH := filepath.Join(binDir, "gh")
	mustWriteFile(t, fakeGH, []byte(`#!/bin/sh
set -eu
[ "${1:-}" = "api" ] || exit 2
shift

accept=""
endpoint=""
query=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -H)
      accept="${2:-}"
      shift 2
      ;;
    --jq)
      query="${2:-}"
      shift 2
      ;;
    repos/*)
      endpoint="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s|%s|%s\n' "$accept" "$endpoint" "$query" >> "$GH_LOG"

case "$accept" in
  *application/octet-stream*)
    case "$endpoint" in
      repos/owner/repo/releases/assets/*) ;;
      *) exit 31 ;;
    esac
    asset_id="${endpoint##*/}"
    asset_name="$(
      awk -F '	' -v id="$asset_id" '
        $1 == id { print $2; found = 1 }
        END { if (!found) exit 1 }
      ' "$GH_ASSET_ROWS"
    )" || exit 32
    printf 'asset-bytes:%s:%s\n' "$asset_id" "$asset_name"
    exit 0
    ;;
esac

[ "$endpoint" = "repos/owner/repo/releases/$GH_EXPECTED_RELEASE_ID" ] || exit 33
case "$query" in
  '.tag_name')
    printf '%s\n' "$GH_RELEASE_TAG"
    ;;
  '.assets[].name')
    cut -f 2 "$GH_ASSET_ROWS"
    ;;
  '.assets[] | [.id, .name] | @tsv')
    cat "$GH_ASSET_ROWS"
    ;;
  *)
    exit 34
    ;;
esac
`), 0o755)

	type releaseAsset struct {
		id   string
		name string
	}
	assets := []releaseAsset{
		{id: "1008", name: "dws-windows-arm64.zip"},
		{id: "1003", name: "dws-darwin-arm64.tar.gz"},
		{id: "1001", name: "checksums.txt"},
		{id: "1005", name: "dws-linux-arm64.tar.gz"},
		{id: "1002", name: "dws-darwin-amd64.tar.gz"},
		{id: "1007", name: "dws-windows-amd64.zip"},
		{id: "1004", name: "dws-linux-amd64.tar.gz"},
		{id: "1006", name: "dws-skills.zip"},
	}
	rowsFor := func(items []releaseAsset) string {
		var rows []string
		for _, asset := range items {
			rows = append(rows, asset.id+"\t"+asset.name)
		}
		return strings.Join(rows, "\n") + "\n"
	}
	script := filepath.Join(sourceRoot, "scripts", "release", "download-github-release-assets.sh")
	type runResult struct {
		output string
		log    string
		dest   string
		err    error
	}
	run := func(items []releaseAsset, releaseTag, releaseID string) runResult {
		runDir := t.TempDir()
		rowsPath := filepath.Join(runDir, "assets.tsv")
		logPath := filepath.Join(runDir, "gh.log")
		dest := filepath.Join(runDir, "download")
		mustWriteFile(t, rowsPath, []byte(rowsFor(items)), 0o644)
		cmd := exec.Command("sh", script, "v1.2.3", dest)
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"GITHUB_REPOSITORY=owner/repo",
			"DWS_GITHUB_RELEASE_ID=" + releaseID,
			"GH_ASSET_ROWS=" + rowsPath,
			"GH_EXPECTED_RELEASE_ID=456",
			"GH_LOG=" + logPath,
			"GH_RELEASE_TAG=" + releaseTag,
		}
		output, runErr := cmd.CombinedOutput()
		log, readErr := os.ReadFile(logPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatalf("ReadFile(%s) error = %v", logPath, readErr)
		}
		return runResult{
			output: string(output),
			log:    string(log),
			dest:   dest,
			err:    runErr,
		}
	}

	success := run(assets, "v1.2.3", "456")
	if success.err != nil {
		t.Fatalf("exact GitHub Release download was rejected: %v\n%s", success.err, success.output)
	}
	if strings.Contains(success.log, "releases/tags/") {
		t.Fatalf("download fell back to a tag endpoint:\n%s", success.log)
	}
	if got := strings.Count(success.log, "application/octet-stream"); got != len(assets) {
		t.Fatalf("asset download count = %d, want %d\nlog:\n%s", got, len(assets), success.log)
	}
	for _, asset := range assets {
		wantEndpoint := "repos/owner/repo/releases/assets/" + asset.id
		if !strings.Contains(success.log, wantEndpoint) {
			t.Errorf("asset %s was not downloaded by ID %s\nlog:\n%s", asset.name, asset.id, success.log)
		}
		data, readErr := os.ReadFile(filepath.Join(success.dest, asset.name))
		if readErr != nil {
			t.Errorf("ReadFile(%s) error = %v", asset.name, readErr)
			continue
		}
		want := fmt.Sprintf("asset-bytes:%s:%s\n", asset.id, asset.name)
		if string(data) != want {
			t.Errorf("%s bytes = %q, want %q", asset.name, data, want)
		}
	}

	extra := append(append([]releaseAsset{}, assets...), releaseAsset{id: "1009", name: "malware.exe"})
	if result := run(extra, "v1.2.3", "456"); result.err == nil || !strings.Contains(result.output, "exactly the supported assets") {
		t.Fatalf("extra GitHub Release asset was not rejected: err=%v\n%s", result.err, result.output)
	}
	if result := run(assets[:len(assets)-1], "v1.2.3", "456"); result.err == nil || !strings.Contains(result.output, "exactly the supported assets") {
		t.Fatalf("missing GitHub Release asset was not rejected: err=%v\n%s", result.err, result.output)
	}
	if result := run(assets, "v9.9.9", "456"); result.err == nil || !strings.Contains(result.output, "ID/tag mismatch") {
		t.Fatalf("wrong GitHub Release tag was not rejected: err=%v\n%s", result.err, result.output)
	}
	if result := run(assets, "v1.2.3", "invalid"); result.err == nil || !strings.Contains(result.output, "invalid GitHub Release ID") {
		t.Fatalf("invalid GitHub Release ID was not rejected: err=%v\n%s", result.err, result.output)
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

func TestReleaseWorkflowDeliveryAcceptsOnlySharedProtectedRecovery(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	binDir := t.TempDir()
	fakeCurl := filepath.Join(binDir, "curl")
	mustWriteFile(t, fakeCurl, []byte(`#!/bin/sh
set -eu
for argument in "$@"; do endpoint="$argument"; done
case "$endpoint" in
  *event=push*)
    printf '{"workflow_runs":[]}\n'
    ;;
  *event=workflow_dispatch*)
    printf '{"workflow_runs":[{"id":42,"display_title":"Release recovery %s at %s %s-1700000000-42","event":"workflow_dispatch","status":"completed","conclusion":"success","head_branch":"main","head_sha":"%s","path":".github/workflows/release.yml","repository":{"full_name":"owner/repo"}}]}\n' "$TAG" "$RELEASE_COMMIT" "$RELEASE_COMMIT" "$WORKFLOW_SHA"
    ;;
  */compare/*...main)
    printf '{"status":"ahead"}\n'
    ;;
  */actions/runs/42/jobs*)
    printf '{"jobs":['
    printf '{"name":"Build signed release artifacts","status":"completed","conclusion":"success","head_sha":"%s"},' "$WORKFLOW_SHA"
    printf '{"name":"Verify Apple Developer ID signatures","status":"completed","conclusion":"success","head_sha":"%s"},' "$WORKFLOW_SHA"
    printf '{"name":"Publish immutable GitHub Release","status":"completed","conclusion":"success","head_sha":"%s"}' "$WORKFLOW_SHA"
    if [ "${MISSING_CHANNELS:-0}" != 1 ]; then
      printf ',{"name":"Publish npm and mirrors","status":"completed","conclusion":"success","head_sha":"%s"}' "$WORKFLOW_SHA"
    fi
    printf ']}\n'
    ;;
  *) exit 1 ;;
esac
`), 0o755)
	script := filepath.Join(sourceRoot, "scripts", "release", "verify-release-workflow-delivery.sh")
	tag := "v1.2.3-beta.1"
	commit := strings.Repeat("a", 40)
	workflowSHA := strings.Repeat("b", 40)
	run := func(missingChannels bool) (string, error) {
		cmd := exec.Command("sh", script, tag, commit)
		missing := "0"
		if missingChannels {
			missing = "1"
		}
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"DWS_RELEASE_OFFICIAL_REPOSITORY=owner/repo",
			"TAG=" + tag,
			"RELEASE_COMMIT=" + commit,
			"WORKFLOW_SHA=" + workflowSHA,
			"MISSING_CHANNELS=" + missing,
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := run(false)
	if err != nil || !strings.Contains(output, "protected recovery run 42") {
		t.Fatalf("protected shared recovery was rejected: err=%v\noutput:\n%s", err, output)
	}
	output, err = run(true)
	if err == nil || !strings.Contains(output, "did not complete the shared release job graph") {
		t.Fatalf("incomplete recovery job graph passed: err=%v\noutput:\n%s", err, output)
	}
}

func TestReleaseWorkflowDeliveryChannelRepairRequiresLatestAttemptCoreDelivery(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	binDir := t.TempDir()
	fakeCurl := filepath.Join(binDir, "curl")
	mustWriteFile(t, fakeCurl, []byte(`#!/bin/sh
set -eu
for argument in "$@"; do endpoint="$argument"; done
case "$endpoint" in
  *event=push*)
    python3 - <<'PY'
import json
import os
run = {
    "id": 77,
    "event": "push",
    "status": "completed",
    "conclusion": "failure",
    "head_branch": os.environ["TAG"],
    "head_sha": os.environ["RELEASE_COMMIT"],
    "path": os.environ.get("RUN_PATH", ".github/workflows/release.yml"),
    "repository": {"full_name": os.environ.get("RUN_REPOSITORY", "owner/repo")},
    "run_attempt": int(os.environ.get("RUN_ATTEMPT", "2")),
}
runs = [run]
if os.environ.get("DUPLICATE_RUN") == "1":
    duplicate = dict(run)
    duplicate["id"] = 78
    runs.append(duplicate)
print(json.dumps({"workflow_runs": runs}))
PY
    ;;
  *event=workflow_dispatch*)
    printf '{"workflow_runs":[]}\n'
    ;;
  */actions/runs/77/attempts/2/jobs*)
    python3 - <<'PY'
import json
import os

commit = os.environ.get("JOB_SHA", os.environ["RELEASE_COMMIT"])
core = [
    "release-contract",
    "Build signed release artifacts",
    "Verify Apple Developer ID signatures",
    "Publish immutable GitHub Release",
]
jobs = [{
    "name": name,
    "status": "completed",
    "conclusion": (
        os.environ.get("CORE_CONCLUSION", "success")
        if name == os.environ.get("CORE_JOB", "Build signed release artifacts")
        else "success"
    ),
    "head_sha": commit,
    "steps": (
        [{
            "name": "Require immutable published GitHub Release",
            "status": "completed",
            "conclusion": os.environ.get("IMMUTABLE_STEP_CONCLUSION", "success"),
        }]
        if name == "Publish immutable GitHub Release"
        else []
    ),
} for name in core]
if os.environ.get("DUPLICATE_CORE") == "1":
    jobs.append(dict(jobs[1]))
required_steps = [
    "Download and verify immutable GitHub Release",
    "Verify immutable npm package without publication credentials",
    "Inspect npm channel state",
    "Verify npm channel delivery",
]
jobs.append({
    "name": "Publish npm and mirrors",
    "status": "completed",
    "conclusion": os.environ.get("CHANNEL_JOB_CONCLUSION", "failure"),
    "head_sha": commit,
    "steps": [{
        "name": name,
        "status": "completed",
        "conclusion": (
            os.environ.get("CHANNEL_STEP_CONCLUSION", "success")
            if name == os.environ.get("CHANNEL_STEP", "Verify npm channel delivery")
            else "success"
        ),
    } for name in required_steps] + [{
        "name": "Sync release artifacts to China OSS mirror",
        "status": "completed",
        "conclusion": os.environ.get("OSS_STEP_CONCLUSION", "failure"),
    }],
})
jobs.extend([
    {
        "name": "Mirror immutable release to Gitee",
        "status": "completed",
        "conclusion": os.environ.get("GITEE_CONCLUSION", "skipped"),
        "head_sha": commit,
        "steps": [],
    },
    {
        "name": "Release delivery gate",
        "status": "completed",
        "conclusion": os.environ.get("DELIVERY_GATE_CONCLUSION", "failure"),
        "head_sha": commit,
        "steps": [],
    },
])
if os.environ.get("UNRELATED_FAILURE") == "1":
    jobs.append({
        "name": "Unrelated release job",
        "status": "completed",
        "conclusion": "failure",
        "head_sha": commit,
        "steps": [],
    })
print(json.dumps({"jobs": jobs}))
PY
    ;;
  */actions/runs/77/*/jobs*)
    echo "channel repair must inspect the exact latest run attempt" >&2
    exit 91
    ;;
  *) exit 1 ;;
esac
`), 0o755)
	script := filepath.Join(sourceRoot, "scripts", "release", "verify-release-workflow-delivery.sh")
	tag := "v1.2.3-beta.1"
	commit := strings.Repeat("a", 40)
	run := func(args []string, overrides ...string) (string, error) {
		cmd := exec.Command("sh", append([]string{script}, args...)...)
		cmd.Env = append([]string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"DWS_RELEASE_OFFICIAL_REPOSITORY=owner/repo",
			"TAG=" + tag,
			"RELEASE_COMMIT=" + commit,
		}, overrides...)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	repairArgs := func(target string) []string {
		return []string{"--channel-repair", target, tag, commit}
	}

	if output, err := run([]string{tag, commit}); err == nil ||
		!strings.Contains(output, "did not deliver") {
		t.Fatalf("strict delivery accepted a failed tag run: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(repairArgs("oss")); err != nil ||
		!strings.Contains(output, "failed exact-tag push run 77") {
		t.Fatalf("safe channel repair delivery was rejected: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"CORE_CONCLUSION=failure",
	); err == nil || !strings.Contains(output, "required job 'Build signed release artifacts' did not succeed") {
		t.Fatalf("failed core release job passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"CHANNEL_STEP_CONCLUSION=failure",
	); err == nil || !strings.Contains(output, "required channel step 'Verify npm channel delivery' did not succeed") {
		t.Fatalf("failed npm delivery proof passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"DUPLICATE_CORE=1",
	); err == nil || !strings.Contains(output, "expected exactly one latest-attempt job 'Build signed release artifacts'") {
		t.Fatalf("duplicate latest-attempt core job passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"JOB_SHA="+strings.Repeat("b", 40),
	); err == nil || !strings.Contains(output, "is not bound to "+commit) {
		t.Fatalf("wrong-sha release jobs passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"UNRELATED_FAILURE=1",
	); err == nil || !strings.Contains(output, "unrelated job 'Unrelated release job' failed") {
		t.Fatalf("unrelated failed job passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"IMMUTABLE_STEP_CONCLUSION=failure",
	); err == nil || !strings.Contains(output, "immutable GitHub Release verification did not succeed") {
		t.Fatalf("failed immutable release verification passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("gitee"),
		"CHANNEL_JOB_CONCLUSION=success",
		"OSS_STEP_CONCLUSION=success",
		"GITEE_CONCLUSION=failure",
	); err != nil || !strings.Contains(output, "channel-repair authority verified") {
		t.Fatalf("single failed Gitee mirror was rejected: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"CHANNEL_JOB_CONCLUSION=success",
		"OSS_STEP_CONCLUSION=success",
		"GITEE_CONCLUSION=failure",
	); err == nil || !strings.Contains(output, "OSS repair requires") {
		t.Fatalf("Gitee-only failure was accepted as OSS evidence: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("gitee"),
	); err != nil || !strings.Contains(output, "channel-repair authority verified") {
		t.Fatalf("skipped Gitee backfill was rejected after upstream OSS failure: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"GITEE_CONCLUSION=failure",
	); err == nil || !strings.Contains(output, "expected exactly one failed downstream channel job") {
		t.Fatalf("two failed downstream channels passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"DELIVERY_GATE_CONCLUSION=success",
	); err == nil || !strings.Contains(output, "must end in a failed delivery gate") {
		t.Fatalf("successful terminal gate on a failed run passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"RUN_ATTEMPT=1",
	); err == nil || strings.Contains(output, "channel-repair authority verified") {
		t.Fatalf("non-latest run attempt passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"RUN_REPOSITORY=other/repo",
	); err == nil || strings.Contains(output, "channel-repair authority verified") {
		t.Fatalf("wrong-repository release run passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"RUN_PATH=.github/workflows/other.yml",
	); err == nil || strings.Contains(output, "channel-repair authority verified") {
		t.Fatalf("wrong-workflow release run passed: err=%v\noutput:\n%s", err, output)
	}
	if output, err := run(
		repairArgs("oss"),
		"DUPLICATE_RUN=1",
	); err == nil || strings.Contains(output, "channel-repair authority verified") {
		t.Fatalf("ambiguous failed release runs passed: err=%v\noutput:\n%s", err, output)
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
				"OSS_REGION=cn-test",
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
				"OSS_REGION=cn-test",
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
		"OSS_REGION=cn-test",
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

func TestReleaseCommandReusesExactPreflightProof(t *testing.T) {
	r := newReleaseTestRepo(t)
	installReleaseCommandFixture(t, r)
	mustWriteFile(t, filepath.Join(r.root, ".gitignore"), []byte("/dws\n"), 0o644)
	mustWriteFile(t, filepath.Join(r.root, "CHANGELOG.md"), []byte(releaseChangelog(betaSection())), 0o644)
	mustWriteFile(t, filepath.Join(r.root, "Makefile"), []byte(`test:
	@rm -f dws
	@printf 'test\n' >> "$$(git rev-parse --git-path release-phases)"
build:
	@printf '#!/bin/sh\nexit 0\n' > dws
	@chmod +x dws
	@printf 'build\n' >> "$$(git rev-parse --git-path release-phases)"
policy:
	@test -x dws
	@printf 'policy\n' >> "$$(git rev-parse --git-path release-phases)"
package:
	@printf 'package\n' >> "$$(git rev-parse --git-path release-phases)"
`), 0o644)
	r.commitAndPush(t, "install proof-aware release automation")

	command := filepath.Join(r.root, "scripts", "release", "release.sh")
	output, err := runReleaseScript(t, r.root, command,
		"prerelease", "v1.0.1-beta.1", "--remote", "origin",
	)
	if err != nil {
		t.Fatalf("release validation error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "Publish within six hours") {
		t.Fatalf("validation output missing reusable-proof guidance:\n%s", output)
	}
	phasePath := filepath.Join(r.root, ".git", "release-phases")
	phaseData, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatalf("ReadFile(release phases) error = %v", err)
	}
	wantPhases := "test\nbuild\npolicy\npackage\n"
	if string(phaseData) != wantPhases {
		t.Fatalf("release phase order = %q, want %q", phaseData, wantPhases)
	}
	proofPath := strings.TrimSpace(mustOutput(t, r.root, "git", "rev-parse", "--git-path", "dws-release-preflight/v1.0.1-beta.1.proof"))
	if !filepath.IsAbs(proofPath) {
		proofPath = filepath.Join(r.root, proofPath)
	}
	proofBefore, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatalf("ReadFile(preflight proof) error = %v", err)
	}
	hook := filepath.Join(r.remote, "hooks", "pre-receive")
	mustWriteFile(t, hook, []byte("#!/bin/sh\nset -eu\nwhile read -r old new ref; do\n  case \"$ref\" in refs/tags/*) exit 1 ;; esac\ndone\nexit 0\n"), 0o755)
	failedOutput, failedErr := runReleaseScript(t, r.root, command,
		"prerelease", "v1.0.1-beta.1", "--remote", "origin", "--publish", "--yes",
	)
	if failedErr == nil || !strings.Contains(failedOutput, "new local tag was removed") {
		t.Fatalf("rejected proof-based publish did not fail safely: err=%v\noutput:\n%s", failedErr, failedOutput)
	}
	proofAfterFailure, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatalf("ReadFile(preflight proof after failure) error = %v", err)
	}
	if string(proofAfterFailure) != string(proofBefore) {
		t.Fatalf("failed publish refreshed reusable proof:\nbefore:\n%s\nafter:\n%s", proofBefore, proofAfterFailure)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatalf("Remove(rejecting hook) error = %v", err)
	}

	output, err = runReleaseScript(t, r.root, command,
		"prerelease", "v1.0.1-beta.1", "--remote", "origin", "--publish", "--yes",
	)
	if err != nil {
		t.Fatalf("release publish error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(output, "Reusing exact preflight proof") {
		t.Fatalf("publish did not reuse the exact preflight proof:\n%s", output)
	}
	phaseData, err = os.ReadFile(phasePath)
	if err != nil {
		t.Fatalf("ReadFile(release phases after publish) error = %v", err)
	}
	if string(phaseData) != wantPhases {
		t.Fatalf("publish reran expensive phases: %q", phaseData)
	}
}

func TestReleaseGovernanceCIDispatchBindsExactRunIdentity(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	binDir := t.TempDir()
	stateDir := t.TempDir()
	fakeGH := filepath.Join(binDir, "gh")
	mustWriteFile(t, fakeGH, []byte(`#!/bin/sh
set -eu
command="$1"
shift
case "$command" in
  workflow)
    test "$1" = run
    shift
    printf '%s\n' "$@" > "$GH_STATE/workflow-args"
    for argument in "$@"; do
      case "$argument" in
        governance_preflight_nonce=*)
          printf 'Release governance preflight %s\n' "${argument#governance_preflight_nonce=}" > "$GH_STATE/title"
          ;;
        governance_preflight_commit=*)
          printf '%s\n' "${argument#governance_preflight_commit=}" > "$GH_STATE/commit"
          ;;
      esac
    done
    ;;
  api)
    for argument in "$@"; do
      case "$argument" in repos/*/actions/*) endpoint="$argument" ;; esac
    done
    case "$endpoint" in
      */actions/workflows/release.yml/runs*) printf '42\n' ;;
      */actions/runs/42)
        title="$(cat "$GH_STATE/title")"
        commit="$(cat "$GH_STATE/commit")"
        repository=owner/repo
        if [ "${GH_BAD_STATE:-0}" = 1 ]; then repository=attacker/repo; fi
        printf '%s\tworkflow_dispatch\tcompleted\tsuccess\tmain\t%s\t.github/workflows/release.yml\t%s\n' "$title" "$commit" "$repository"
        ;;
      *) exit 1 ;;
    esac
    ;;
  run)
    test "$1" = watch
    ;;
  *) exit 1 ;;
esac
`), 0o755)
	script := filepath.Join(sourceRoot, "scripts", "release", "verify-release-governance-ci.sh")
	commit := strings.Repeat("a", 40)
	run := func(badState bool) (string, error) {
		cmd := exec.Command("sh", script, "owner/repo", commit)
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"GH_STATE=" + stateDir,
			fmt.Sprintf("GH_BAD_STATE=%d", map[bool]int{false: 0, true: 1}[badState]),
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := run(false)
	if err != nil || !strings.Contains(output, "Release governance preflight passed") {
		t.Fatalf("governance CI preflight error = %v\noutput:\n%s", err, output)
	}
	args, err := os.ReadFile(filepath.Join(stateDir, "workflow-args"))
	if err != nil {
		t.Fatalf("ReadFile(workflow args) error = %v", err)
	}
	for _, required := range []string{
		"--ref\nmain\n",
		"governance_preflight_commit=" + commit,
		"governance_preflight_nonce=" + commit + "-",
	} {
		if !strings.Contains(string(args), required) {
			t.Errorf("workflow dispatch args are missing %q:\n%s", required, args)
		}
	}

	output, err = run(true)
	if err == nil || !strings.Contains(output, "run identity mismatch") {
		t.Fatalf("mismatched governance run identity passed: err=%v\noutput:\n%s", err, output)
	}
}

func TestVerifyHomebrewPRTokenRequiresMinimalWorkingIdentity(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	binDir := t.TempDir()
	fakeGH := filepath.Join(binDir, "gh")
	mustWriteFile(t, fakeGH, []byte(`#!/bin/sh
set -eu
test "${GH_TOKEN:-}" = valid-token
command="$1"
shift
test "$command" = api
endpoint=""
for argument in "$@"; do
  case "$argument" in
    repos/*|user) endpoint="$argument" ;;
  esac
done
case "$endpoint" in
  repos/owner/repo)
    printf 'HTTP/2.0 200 OK\nX-OAuth-Scopes: %s\n\nowner/repo\t%s\tmain\n' \
      "${GH_SCOPES-public_repo}" "${GH_CAN_PUSH:-true}"
    ;;
  repos/owner/repo/pulls?state=open\&per_page=1) ;;
  user) printf '%s\n' release-bot ;;
  *) exit 1 ;;
esac
`), 0o755)

	script := filepath.Join(sourceRoot, "scripts", "release", "verify-homebrew-pr-token.sh")
	run := func(token, scopes, canPush string) (string, error) {
		cmd := exec.Command("sh", script, "owner/repo")
		cmd.Env = []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"HOMEBREW_PR_TOKEN=" + token,
			"GH_SCOPES=" + scopes,
			"GH_CAN_PUSH=" + canPush,
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := run("valid-token", "public_repo", "true")
	if err != nil || !strings.Contains(output, "Homebrew PR token capability passed for owner/repo as release-bot (classic-public_repo)") {
		t.Fatalf("valid Homebrew PR token rejected: err=%v\noutput:\n%s", err, output)
	}

	output, err = run("valid-token", "repo", "true")
	if err == nil || !strings.Contains(output, "must have only public_repo scope") {
		t.Fatalf("overprivileged Homebrew PR token accepted: err=%v\noutput:\n%s", err, output)
	}

	output, err = run("valid-token", "", "true")
	if err != nil || !strings.Contains(output, "(fine-grained)") {
		t.Fatalf("fine-grained Homebrew PR token rejected: err=%v\noutput:\n%s", err, output)
	}

	output, err = run("valid-token", "public_repo", "false")
	if err == nil || !strings.Contains(output, "does not have push permission") {
		t.Fatalf("non-pushing Homebrew identity accepted: err=%v\noutput:\n%s", err, output)
	}

	output, err = run("", "public_repo", "true")
	if err == nil || !strings.Contains(output, "HOMEBREW_PR_TOKEN is required") {
		t.Fatalf("missing Homebrew token accepted: err=%v\noutput:\n%s", err, output)
	}
}

func TestVerifyHomebrewPRTokenCanaryCreatesAndCleans(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	r := newReleaseTestRepo(t)
	binDir := t.TempDir()
	stateDir := t.TempDir()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) error = %v", err)
	}
	fakeGit := filepath.Join(binDir, "git")
	mustWriteFile(t, fakeGit, []byte(`#!/bin/sh
set -eu
network=false
for argument in "$@"; do
  case "$argument" in
    clone|push|ls-remote) network=true ;;
  esac
done
if [ "$network" = true ]; then
  printf '%s\n' "$*" >> "$GH_STATE/git-network-args"
  case " $* " in
    *" https://github.com/owner/repo.git "*) ;;
    *) exit 91 ;;
  esac
  test "${GIT_CONFIG_NOSYSTEM:-}" = 1
  test "${GIT_CONFIG_GLOBAL:-}" = /dev/null
  test "${GIT_CONFIG_COUNT:-}" = 0
  test -x "${GIT_ASKPASS:-}"
  test "$("$GIT_ASKPASS" Username)" = x-access-token
  test "$("$GIT_ASKPASS" Password)" = valid-token
  case " $* " in
    *" HEAD:refs/heads/"*)
      if [ "${BLOCK_PUSH:-false}" = true ]; then
        "$REAL_GIT" \
          -c "url.$TEST_REMOTE.insteadOf=https://github.com/owner/repo.git" \
          "$@"
        : > "$GH_STATE/push-complete-${GITHUB_RUN_ID:-unknown}"
        sleep 2
        exit 0
      fi
      ;;
  esac
  case " $* " in
    *" :refs/heads/"*)
      if [ "${FAIL_DELETE:-false}" = true ]; then
        exit 92
      fi
      ;;
  esac
  exec "$REAL_GIT" \
    -c "url.$TEST_REMOTE.insteadOf=https://github.com/owner/repo.git" \
    "$@"
fi
exec "$REAL_GIT" "$@"
`), 0o755)
	fakeGH := filepath.Join(binDir, "gh")
	mustWriteFile(t, fakeGH, []byte(`#!/bin/sh
set -eu
test "${GH_TOKEN:-}" = valid-token
command="$1"
shift
case "$command" in
  api)
    endpoint=""
    for argument in "$@"; do
      case "$argument" in
        repos/*|user) endpoint="$argument" ;;
      esac
    done
    case "$endpoint" in
      repos/owner/repo)
        printf 'HTTP/2.0 200 OK\nX-OAuth-Scopes: public_repo\n\nowner/repo\ttrue\tmain\n'
        ;;
      repos/owner/repo/pulls?state=open\&per_page=1) ;;
      repos/owner/repo/pulls?state=open\&head=owner:*\&base=main\&per_page=2)
        if [ -f "$GH_STATE/pr-open-${GITHUB_RUN_ID:-unknown}" ]; then
          printf '%s\n' 42
        fi
        ;;
      repos/owner/repo/pulls/42)
        if [ "${FAIL_CLOSE:-false}" = true ]; then
          exit 93
        fi
        rm -f "$GH_STATE/pr-open-${GITHUB_RUN_ID:-unknown}"
        printf '%s\n' closed > "$GH_STATE/pr-closed-${GITHUB_RUN_ID:-unknown}"
        ;;
      user) printf '%s\n' release-bot ;;
      *) exit 1 ;;
    esac
    ;;
  pr)
    test "$1" = create
    shift
    printf '%s\n' "$@" > "$GH_STATE/pr-args"
    : > "$GH_STATE/pr-create-started-${GITHUB_RUN_ID:-unknown}"
    if [ "${FAIL_PR_CREATE:-false}" = true ]; then
      exit 94
    fi
    : > "$GH_STATE/pr-open-${GITHUB_RUN_ID:-unknown}"
    if [ "${BLOCK_PR_CREATE:-false}" = true ]; then
      sleep 2
    fi
    printf '%s\n' 'https://github.com/owner/repo/pull/42'
    ;;
  *) exit 1 ;;
esac
`), 0o755)

	script := filepath.Join(sourceRoot, "scripts", "release", "verify-homebrew-pr-token.sh")
	baseEnv := []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"GH_STATE=" + stateDir,
		"HOMEBREW_PR_TOKEN=valid-token",
		"GITHUB_RUN_ATTEMPT=2",
		"REAL_GIT=" + realGit,
		"TEST_REMOTE=" + r.remote,
	}
	runCanary := func(runID string, extraEnv ...string) ([]byte, error) {
		cmd := exec.Command("sh", script, "owner/repo", "--canary")
		cmd.Dir = r.root
		cmd.Env = append(append([]string{}, baseEnv...), "GITHUB_RUN_ID="+runID)
		cmd.Env = append(cmd.Env, extraEnv...)
		return cmd.CombinedOutput()
	}
	remoteHasBranch := func(branch string) bool {
		cmd := exec.Command(
			"git", "--git-dir", r.remote, "show-ref", "--verify",
			"refs/heads/"+branch,
		)
		return cmd.Run() == nil
	}

	output, err := runCanary("12345")
	if err != nil || !strings.Contains(string(output), "Homebrew PR token capability passed") {
		t.Fatalf("Homebrew token canary error = %v\noutput:\n%s", err, output)
	}

	prArgs, err := os.ReadFile(filepath.Join(stateDir, "pr-args"))
	if err != nil {
		t.Fatalf("ReadFile(PR args) error = %v", err)
	}
	for _, required := range []string{
		"--draft",
		"--base\nmain",
		"--head\nautomation/homebrew-token-canary-12345-2",
	} {
		if !strings.Contains(string(prArgs), required) {
			t.Errorf("Homebrew token canary PR args are missing %q:\n%s", required, prArgs)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "pr-closed-12345")); err != nil {
		t.Fatalf("Homebrew token canary did not close its PR: %v", err)
	}
	gitNetworkArgs, err := os.ReadFile(filepath.Join(stateDir, "git-network-args"))
	if err != nil {
		t.Fatalf("ReadFile(git network args) error = %v", err)
	}
	for _, required := range []string{
		"clone",
		"https://github.com/owner/repo.git",
		"HEAD:refs/heads/automation/homebrew-token-canary-12345-2",
		"--force-with-lease=refs/heads/automation/homebrew-token-canary-12345-2:",
		":refs/heads/automation/homebrew-token-canary-12345-2",
	} {
		if !strings.Contains(string(gitNetworkArgs), required) {
			t.Errorf("Homebrew token canary Git operations are missing %q:\n%s", required, gitNetworkArgs)
		}
	}

	if remoteHasBranch("automation/homebrew-token-canary-12345-2") {
		t.Fatal("Homebrew token canary left its remote branch behind")
	}

	output, err = runCanary("20001", "FAIL_PR_CREATE=true")
	if err == nil || !strings.Contains(string(output), "cannot create a canary pull request") {
		t.Fatalf("PR creation failure was not fail-closed: err=%v\noutput:\n%s", err, output)
	}
	if remoteHasBranch("automation/homebrew-token-canary-20001-2") {
		t.Fatal("PR creation failure left its canary branch behind")
	}

	output, err = runCanary("20002", "FAIL_CLOSE=true")
	if err == nil ||
		!strings.Contains(string(output), "could not close canary PR") ||
		!strings.Contains(string(output), "could not clean up Homebrew token canary PR") {
		t.Fatalf("PR cleanup failure was not fail-closed: err=%v\noutput:\n%s", err, output)
	}
	if remoteHasBranch("automation/homebrew-token-canary-20002-2") {
		t.Fatal("PR cleanup failure left its canary branch behind")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "pr-open-20002")); err != nil {
		t.Fatalf("PR cleanup failure did not leave an observable open canary PR: %v", err)
	}

	output, err = runCanary("20003", "FAIL_DELETE=true")
	if err == nil ||
		!strings.Contains(string(output), "could not delete canary branch") ||
		!strings.Contains(string(output), "could not clean up Homebrew token canary branch") {
		t.Fatalf("branch cleanup failure was not fail-closed: err=%v\noutput:\n%s", err, output)
	}
	if !remoteHasBranch("automation/homebrew-token-canary-20003-2") {
		t.Fatal("branch deletion failure was hidden instead of leaving an observable failed canary")
	}

	termCmd := exec.Command("sh", script, "owner/repo", "--canary")
	termCmd.Dir = r.root
	termCmd.Env = append(append([]string{}, baseEnv...),
		"GITHUB_RUN_ID=20004",
		"BLOCK_PR_CREATE=true",
	)
	var termStdout bytes.Buffer
	var termStderr bytes.Buffer
	termCmd.Stdout = &termStdout
	termCmd.Stderr = &termStderr
	if err := termCmd.Start(); err != nil {
		t.Fatalf("Start(TERM canary) error = %v", err)
	}
	marker := filepath.Join(stateDir, "pr-create-started-20004")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = termCmd.Process.Kill()
			_ = termCmd.Wait()
			t.Fatalf("TERM canary did not reach PR creation:\n%s%s", termStdout.String(), termStderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := termCmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal(TERM canary) error = %v", err)
	}
	err = termCmd.Wait()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 143 {
		t.Fatalf("TERM canary exit = %v, want 143\nstdout:\n%s\nstderr:\n%s",
			err, termStdout.String(), termStderr.String())
	}
	if remoteHasBranch("automation/homebrew-token-canary-20004-2") {
		t.Fatal("TERM canary left its remote branch behind")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "pr-closed-20004")); err != nil {
		t.Fatalf("TERM canary did not close the PR created during the signal race: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "pr-open-20004")); !os.IsNotExist(err) {
		t.Fatalf("TERM canary left an open PR marker: %v", err)
	}

	pushTermCmd := exec.Command("sh", script, "owner/repo", "--canary")
	pushTermCmd.Dir = r.root
	pushTermCmd.Env = append(append([]string{}, baseEnv...),
		"GITHUB_RUN_ID=20005",
		"BLOCK_PUSH=true",
	)
	var pushTermStdout bytes.Buffer
	var pushTermStderr bytes.Buffer
	pushTermCmd.Stdout = &pushTermStdout
	pushTermCmd.Stderr = &pushTermStderr
	if err := pushTermCmd.Start(); err != nil {
		t.Fatalf("Start(push TERM canary) error = %v", err)
	}
	pushMarker := filepath.Join(stateDir, "push-complete-20005")
	deadline = time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(pushMarker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = pushTermCmd.Process.Kill()
			_ = pushTermCmd.Wait()
			t.Fatalf("push TERM canary did not complete its remote push:\n%s%s",
				pushTermStdout.String(), pushTermStderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := pushTermCmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal(push TERM canary) error = %v", err)
	}
	err = pushTermCmd.Wait()
	exitErr, ok = err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 143 {
		t.Fatalf("push TERM canary exit = %v, want 143\nstdout:\n%s\nstderr:\n%s",
			err, pushTermStdout.String(), pushTermStderr.String())
	}
	if remoteHasBranch("automation/homebrew-token-canary-20005-2") {
		t.Fatal("push TERM canary left its remote branch behind")
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
	mustWriteFile(t, filepath.Join(r.root, "Makefile"), []byte("test:\n\t@:\nbuild:\n\t@:\npolicy:\n\t@:\npackage:\n\t@git tag -a v1.0.1 -m 'Release v1.0.1'\n\t@git push origin refs/tags/v1.0.1\n"), 0o644)
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
	mustWriteFile(t, filepath.Join(r.root, "Makefile"), []byte("test:\n\t@:\nbuild:\n\t@:\npolicy:\n\t@:\npackage:\n\t@:\n"), 0o644)
}

func releaseCopyFile(t *testing.T, source, target string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", source, err)
	}
	mustWriteFile(t, target, data, mode)
}

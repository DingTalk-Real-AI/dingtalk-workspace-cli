package scripts_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var requiredGiteeAssets = []string{
	"dws-darwin-amd64.tar.gz",
	"dws-darwin-arm64.tar.gz",
	"dws-linux-amd64.tar.gz",
	"dws-linux-arm64.tar.gz",
	"dws-windows-amd64.zip",
	"dws-windows-arm64.zip",
	"dws-skills.zip",
	"checksums.txt",
}

func TestSyncGiteeTagSkipsAnAlreadyAlignedImmutableTag(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)

	firstOutput := runGiteeTagSync(t, scriptPath, workDir, remoteDir, "v1.2.3", true)
	if !strings.Contains(firstOutput, "Gitee tag v1.2.3 is aligned") {
		t.Fatalf("first tag sync did not report alignment:\n%s", firstOutput)
	}

	// Reject every subsequent push. A truly idempotent second run succeeds only
	// if it observes the aligned peeled commit and skips git push entirely.
	mustWriteFile(t, filepath.Join(remoteDir, "hooks", "pre-receive"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	secondOutput := runGiteeTagSync(t, scriptPath, workDir, remoteDir, "v1.2.3", true)
	if !strings.Contains(secondOutput, "already aligned") || !strings.Contains(secondOutput, "skip push") {
		t.Fatalf("second tag sync did not take the idempotent path:\n%s", secondOutput)
	}
}

func TestSyncGiteeTagRefusesToMoveAnExistingTag(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	mustRun(t, workDir, "git", "push", remoteDir, "refs/tags/v1.2.3:refs/tags/v1.2.3")
	originalCommit := peeledRemoteTag(t, workDir, remoteDir, "v1.2.3")

	mustRun(t, workDir, "git", "tag", "-d", "v1.2.3")
	mustWriteFile(t, filepath.Join(workDir, "payload.txt"), []byte("new release bytes\n"), 0o644)
	mustRun(t, workDir, "git", "add", "payload.txt")
	mustRun(t, workDir, "git", "commit", "-m", "new release commit")
	mustRun(t, workDir, "git", "tag", "-a", "v1.2.3", "-m", "v1.2.3 moved locally")

	output := runGiteeTagSync(t, scriptPath, workDir, remoteDir, "v1.2.3", false)
	if !strings.Contains(output, "refusing to move it") {
		t.Fatalf("conflicting tag sync did not fail closed:\n%s", output)
	}
	if got := peeledRemoteTag(t, workDir, remoteDir, "v1.2.3"); got != originalCommit {
		t.Fatalf("remote tag moved from %s to %s", originalCommit, got)
	}
}

func TestSyncGiteeTagRejectsAMissingLocalTagWithoutInventingACommit(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)

	output := runGiteeTagSync(t, scriptPath, workDir, remoteDir, "v9.9.9", false)
	if !strings.Contains(output, "could not resolve local release tag v9.9.9") {
		t.Fatalf("missing tag did not report a local resolution failure:\n%s", output)
	}
	for _, misleading := range []string{"refusing to move it", "v9.9.9^{commit}"} {
		if strings.Contains(output, misleading) {
			t.Fatalf("missing tag output contains misleading commit information %q:\n%s", misleading, output)
		}
	}
}

func TestSyncGiteeTagReportsFetchFailureWhenItCannotRecoverAMissingTag(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)

	output := runGiteeTagSync(
		t, scriptPath, workDir, remoteDir, "v9.9.9", false,
		"GITEE_SOURCE_REMOTE=missing-source-remote",
	)
	if !strings.Contains(output, "source tag fetch failed") ||
		!strings.Contains(output, "local release tag v9.9.9 could not be resolved") {
		t.Fatalf("fetch failure did not preserve the actionable cause:\n%s", output)
	}
}

func TestSyncGiteeTagHonorsItsDeadlineDuringFetch(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) error = %v", err)
	}
	wrapperDir := t.TempDir()
	mustWriteFile(t, filepath.Join(wrapperDir, "git"), []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" = "fetch" ]; then
  sleep 5
  exit 1
fi
exec %q "$@"
`, realGit)), 0o755)

	started := time.Now()
	output := runGiteeTagSync(
		t, scriptPath, workDir, remoteDir, "v1.2.3", false,
		"PATH="+wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GITEE_SOURCE_REMOTE=origin",
		"GITEE_TAG_TIMEOUT_SECONDS=1",
		"GITEE_GIT_TIMEOUT_SECONDS=10",
	)
	elapsed := time.Since(started)
	if elapsed > 6*time.Second {
		t.Fatalf("tag deadline took %s, want no more than 6s\noutput:\n%s", elapsed, output)
	}
	if !strings.Contains(output, "deadline exhausted") {
		t.Fatalf("tag deadline failure was not reported clearly:\n%s", output)
	}
}

func TestSyncGiteeTagUsesAskpassWithoutEmbeddingCredentials(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	seedTaggedRepository(t, workDir, "v1.2.3")
	expectedCommit := strings.TrimSpace(mustOutput(t, workDir, "git", "rev-parse", "v1.2.3^{commit}"))

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("LookPath(git) error = %v", err)
	}
	wrapperDir := t.TempDir()
	logPath := filepath.Join(root, "git.log")
	pushMarker := filepath.Join(root, "pushed")
	wrapper := fmt.Sprintf(`#!/bin/sh
set -eu
case "$1" in
  ls-remote)
    printf '%%s\n' "$*" >>"$GIT_TEST_LOG"
    case " $* " in *"$DWS_GITEE_TOKEN"*) exit 91 ;; esac
    [ "${GIT_TERMINAL_PROMPT:-}" = "0" ]
    [ -x "${GIT_ASKPASS:-}" ]
    [ "$("$GIT_ASKPASS" "Username for https://gitee.com")" = "$DWS_GITEE_USER" ]
    [ "$("$GIT_ASKPASS" "Password for https://gitee.com")" = "$DWS_GITEE_TOKEN" ]
    if [ -f "$GIT_PUSH_MARKER" ]; then
      printf '%%s\trefs/tags/v1.2.3^{}\n' "$EXPECTED_COMMIT"
    fi
    ;;
  push)
    printf '%%s\n' "$*" >>"$GIT_TEST_LOG"
    case " $* " in *"$DWS_GITEE_TOKEN"*) exit 92 ;; esac
    [ "$2" = "https://gitee.com/owner/repo.git" ]
    : >"$GIT_PUSH_MARKER"
    ;;
  *) exec %q "$@" ;;
esac
`, realGit)
	mustWriteFile(t, filepath.Join(wrapperDir, "git"), []byte(wrapper), 0o755)

	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"PATH="+wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"VERSION=v1.2.3",
		"GITEE_USER=test-user",
		"GITEE_TOKEN=sensitive-token",
		"GITEE_REPO=owner/repo",
		"GITEE_SOURCE_REMOTE=",
		"GITEE_GIT_REMOTE=",
		"GITEE_PUBLIC_GIT_REMOTE=",
		"GITEE_TAG_VERIFY_ATTEMPTS=1",
		"GITEE_TAG_VERIFY_DELAY=0",
		"GITEE_GIT_TIMEOUT_SECONDS=10",
		"GIT_TEST_LOG="+logPath,
		"GIT_PUSH_MARKER="+pushMarker,
		"EXPECTED_COMMIT="+expectedCommit,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync-gitee-tag.sh error = %v\noutput:\n%s", err, output)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(git log) error = %v", err)
	}
	if strings.Contains(string(logData), "sensitive-token") {
		t.Fatalf("Gitee token leaked into git argv:\n%s", logData)
	}
	if !strings.Contains(string(logData), "push https://gitee.com/owner/repo.git") {
		t.Fatalf("tag push did not use the credential-free remote:\n%s", logData)
	}
}

func TestSyncGiteeTagAndReleasePathShareOneOverallDeadline(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-to-gitee.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	mustRun(t, workDir, "git", "push", remoteDir, "refs/tags/v1.2.3:refs/tags/v1.2.3")
	distDir := seedGiteeDist(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Second)
		http.Error(w, "slow Gitee API", http.StatusGatewayTimeout)
	}))
	defer server.Close()

	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"VERSION=v1.2.3",
		"DIST_DIR="+distDir,
		"GITEE_API="+server.URL,
		"GITEE_TOKEN=test-token",
		"GITEE_USER=test-user",
		"GITEE_REPO=owner/repo",
		"GITEE_SOURCE_REMOTE=",
		"GITEE_GIT_REMOTE="+remoteDir,
		"GITEE_PUBLIC_GIT_REMOTE="+remoteDir,
		"GITEE_SYNC_TIMEOUT_SECONDS=6",
		"GITEE_TAG_TIMEOUT_SECONDS=3",
		"GITEE_GIT_TIMEOUT_SECONDS=3",
		"GITEE_RELEASE_LOOKUP_MAX_TIME=10",
		"GITEE_RELEASE_CREATE_MAX_TIME=10",
		"GITEE_RECONCILE_TIMEOUT_SECONDS=10",
	)
	started := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(started)
	if err == nil {
		t.Fatalf("full Gitee sync unexpectedly succeeded after its deadline:\n%s", output)
	}
	if elapsed > 12*time.Second {
		t.Fatalf("full Gitee sync deadline took %s, want no more than 12s\noutput:\n%s", elapsed, output)
	}
	if !strings.Contains(string(output), "overall Gitee sync deadline exhausted") {
		t.Fatalf("full-path deadline failure was not reported clearly:\n%s", output)
	}
}

func TestSyncToGiteeRunsTagReleaseCreationAndAssetReconciliationWithinOneBudget(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-to-gitee.sh"))
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "gitee.git")
	seedTaggedRepository(t, workDir, "v1.2.3")
	mustRun(t, root, "git", "init", "--bare", remoteDir)
	mustRun(t, workDir, "git", "push", remoteDir, "refs/tags/v1.2.3:refs/tags/v1.2.3")
	distDir := seedGiteeDist(t)

	fake := newFakeGiteeRelease(false, false)
	var releaseMu sync.Mutex
	releaseLookups := 0
	releaseCreates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			if got := r.Header.Get("Authorization"); got != "token test-token" {
				t.Errorf("Authorization header = %q, want token authentication", got)
			}
			if got := r.URL.Query().Get("access_token"); got != "" {
				t.Errorf("Gitee token leaked in query string: %q", got)
			}
		} else if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("token forwarded to asset download URL: %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/releases/tags/v1.2.3":
			releaseMu.Lock()
			releaseLookups++
			lookupAttempt := releaseLookups
			releaseMu.Unlock()
			if lookupAttempt == 1 {
				http.Error(w, "temporary Gitee outage", http.StatusServiceUnavailable)
				return
			}
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/releases":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("ParseMultipartForm(release create) error = %v", err)
			}
			if got := r.FormValue("access_token"); got != "" {
				t.Errorf("Gitee token leaked in release form: %q", got)
			}
			releaseMu.Lock()
			releaseCreates++
			releaseMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"metadata":{"id":999},"id":1}`)
		default:
			fake.ServeHTTP(w, r)
		}
	}))
	defer server.Close()

	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"VERSION=v1.2.3",
		"DIST_DIR="+distDir,
		"GITEE_API="+server.URL,
		"GITEE_TOKEN=test-token",
		"GITEE_USER=test-user",
		"GITEE_REPO=owner/repo",
		"GITEE_SOURCE_REMOTE=",
		"GITEE_GIT_REMOTE="+remoteDir,
		"GITEE_PUBLIC_GIT_REMOTE="+remoteDir,
		"GITEE_SYNC_TIMEOUT_SECONDS=30",
		"GITEE_TAG_TIMEOUT_SECONDS=5",
		"GITEE_GIT_TIMEOUT_SECONDS=3",
		"GITEE_RELEASE_LOOKUP_MAX_TIME=2",
		"GITEE_RELEASE_LOOKUP_RETRIES=2",
		"GITEE_RELEASE_LOOKUP_RETRY_DELAY=0",
		"GITEE_RELEASE_CREATE_MAX_TIME=2",
		"GITEE_RECONCILE_TIMEOUT_SECONDS=20",
		"GITEE_CHILD_DEADLINE_RESERVE_SECONDS=1",
		"GITEE_CURL_CONNECT_TIMEOUT=2",
		"GITEE_CURL_MAX_TIME=2",
		"GITEE_UPLOAD_MAX_TIME=2",
		"GITEE_UPLOAD_RETRIES=1",
		"GITEE_UPLOAD_RETRY_DELAY=0",
		"GITEE_EXISTING_VERIFY_ATTEMPTS=1",
		"GITEE_POST_UPLOAD_VERIFY_ATTEMPTS=1",
		"GITEE_VERIFY_RETRY_DELAY=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("full Gitee sync error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(string(output), "all 8 verified") {
		t.Fatalf("full Gitee sync did not reconcile every release asset:\n%s", output)
	}

	releaseMu.Lock()
	if releaseLookups != 2 || releaseCreates != 1 {
		t.Errorf("release lookup/create calls = %d/%d, want 2/1", releaseLookups, releaseCreates)
	}
	releaseMu.Unlock()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, name := range requiredGiteeAssets {
		if got := fake.uploadCalls[name]; got != 1 {
			t.Errorf("upload calls for %s = %d, want 1", name, got)
		}
	}
}

func TestReconcileGiteeAssetsRecoversACommittedUploadWithLostResponse(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	distDir := seedGiteeDist(t)
	fake := newFakeGiteeRelease(true, false)
	server := httptest.NewServer(fake)
	defer server.Close()

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = giteeAssetEnv(distDir, server.URL, "2")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reconcile-gitee-assets.sh error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(string(output), "appeared with the expected SHA after a lost upload response") {
		t.Fatalf("sync did not recognize the committed upload after the response was lost:\n%s", output)
	}
	if !strings.Contains(string(output), "all 8 verified") {
		t.Fatalf("sync did not report complete final verification:\n%s", output)
	}

	secondCmd := exec.Command("bash", scriptPath)
	secondCmd.Env = giteeAssetEnv(distDir, server.URL, "2")
	secondOutput, err := secondCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("idempotent reconcile error = %v\noutput:\n%s", err, secondOutput)
	}
	if !strings.Contains(string(secondOutput), "uploaded 0, replaced 0, skipped 8") {
		t.Fatalf("second reconcile did not skip all verified assets:\n%s", secondOutput)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, name := range requiredGiteeAssets {
		if got := fake.uploadCalls[name]; got != 1 {
			t.Errorf("upload calls for %s = %d, want 1", name, got)
		}
	}
}

func TestReconcileGiteeAssetsDisablesExpectContinueForLargeUploads(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	distDir := seedGiteeDist(t)
	mustWriteFile(
		t,
		filepath.Join(distDir, requiredGiteeAssets[0]),
		[]byte(strings.Repeat("x", 2<<20)),
		0o644,
	)
	fake := newFakeGiteeRelease(false, false)
	fake.rejectExpectContinue = true
	server := httptest.NewServer(fake)
	defer server.Close()

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = giteeAssetEnv(distDir, server.URL, "1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("large-asset reconciliation error = %v\noutput:\n%s", err, output)
	}
	if !strings.Contains(string(output), "all 8 verified") {
		t.Fatalf("large-asset reconciliation did not verify every asset:\n%s", output)
	}
}

func TestReconcileGiteeAssetsFailsWhenAnyUploadIsMissing(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	distDir := seedGiteeDist(t)
	fake := newFakeGiteeRelease(false, true)
	server := httptest.NewServer(fake)
	defer server.Close()

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = giteeAssetEnv(distDir, server.URL, "1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("reconcile-gitee-assets.sh unexpectedly succeeded with failed uploads:\n%s", output)
	}
	if !strings.Contains(string(output), "reconciliation finished with") {
		t.Fatalf("failed reconciliation did not report a hard final error:\n%s", output)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, name := range requiredGiteeAssets {
		if got := fake.uploadCalls[name]; got != 1 {
			t.Errorf("upload calls for %s = %d, want exactly the configured single attempt", name, got)
		}
	}
}

func TestReconcileGiteeAssetsHonorsTheOverallDeadline(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	distDir := seedGiteeDist(t)
	fake := newFakeGiteeRelease(false, false)
	fake.uploadDelay = 5 * time.Second
	server := httptest.NewServer(fake)
	defer server.Close()

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(giteeAssetEnv(distDir, server.URL, "2"),
		"GITEE_ASSET_TIMEOUT_SECONDS=1",
		"GITEE_OVERALL_TIMEOUT_SECONDS=3",
		"GITEE_UPLOAD_MAX_TIME=10",
	)
	started := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(started)
	if err == nil {
		t.Fatalf("reconcile unexpectedly succeeded after the overall deadline:\n%s", output)
	}
	if elapsed > 8*time.Second {
		t.Fatalf("overall deadline took %s, want no more than 8s\noutput:\n%s", elapsed, output)
	}
	if !strings.Contains(string(output), "deadline") {
		t.Fatalf("deadline failure was not reported clearly:\n%s", output)
	}
}

func TestGiteeReleaseWorkflowUsesImmutableTagsAndBoundedRetryBudget(t *testing.T) {
	mirrorPath := mustAbs(t, filepath.Join("..", "..", ".github", "workflows", "mirror-to-gitee.yml"))
	mirrorData, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", mirrorPath, err)
	}
	mirror := string(mirrorData)
	if strings.Contains(mirror, "tags:") || strings.Contains(mirror, "sync-gitee-tag.sh") {
		t.Fatal("code mirror must not publish release tags outside the guarded release workflow")
	}
	for _, forbidden := range []string{
		`git push --force "$REMOTE" "refs/tags/`,
		`git push --force --tags`,
	} {
		if strings.Contains(mirror, forbidden) {
			t.Fatalf("tag workflow still contains unsafe force push %q", forbidden)
		}
	}

	manualPath := mustAbs(t, filepath.Join("..", "..", ".github", "workflows", "sync-release-to-gitee.yml"))
	if _, err := os.Stat(manualPath); !os.IsNotExist(err) {
		t.Fatalf("manual Gitee release workflow must be removed, stat error = %v", err)
	}

	syncPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-to-gitee.sh"))
	syncData, err := os.ReadFile(syncPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", syncPath, err)
	}
	syncScript := string(syncData)
	syncSeconds := shellDefaultInt(t, syncScript, "GITEE_SYNC_TIMEOUT_SECONDS")
	tagSeconds := shellDefaultInt(t, syncScript, "GITEE_TAG_TIMEOUT_SECONDS")
	lookupSeconds := shellDefaultInt(t, syncScript, "GITEE_RELEASE_LOOKUP_MAX_TIME")
	createSeconds := shellDefaultInt(t, syncScript, "GITEE_RELEASE_CREATE_MAX_TIME")
	reconcileSeconds := shellDefaultInt(t, syncScript, "GITEE_RECONCILE_TIMEOUT_SECONDS")

	reconcilerPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	reconcilerData, err := os.ReadFile(reconcilerPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", reconcilerPath, err)
	}
	reconciler := string(reconcilerData)
	for _, forbidden := range []string{"?access_token=", `-F "access_token=`} {
		if strings.Contains(syncScript, forbidden) || strings.Contains(reconciler, forbidden) {
			t.Fatalf("Gitee API scripts still expose tokens via %q", forbidden)
		}
	}
	if !strings.Contains(syncScript, "Authorization: token") || !strings.Contains(reconciler, "Authorization: token") {
		t.Fatal("Gitee API scripts must authenticate with an Authorization header")
	}
	perAssetSeconds := shellDefaultInt(t, reconciler, "GITEE_ASSET_TIMEOUT_SECONDS")
	overallSeconds := shellDefaultInt(t, reconciler, "GITEE_OVERALL_TIMEOUT_SECONDS")
	listSeconds := shellDefaultInt(t, reconciler, "GITEE_LIST_MAX_TIME")
	uploadSeconds := shellDefaultInt(t, reconciler, "GITEE_UPLOAD_MAX_TIME")
	verifySeconds := shellDefaultInt(t, reconciler, "GITEE_VERIFY_MAX_TIME")
	postUploadVerifyAttempts := shellDefaultInt(t, reconciler, "GITEE_POST_UPLOAD_VERIFY_ATTEMPTS")
	verifyRetryDelay := shellDefaultInt(t, reconciler, "GITEE_VERIFY_RETRY_DELAY")
	uploadRetryDelay := shellDefaultInt(t, reconciler, "GITEE_UPLOAD_RETRY_DELAY")
	const minimumLargeAssetUploadSeconds = 1200
	if uploadSeconds < minimumLargeAssetUploadSeconds {
		t.Fatalf(
			"Gitee upload deadline = %ds, want at least %ds for near-10 MiB release assets",
			uploadSeconds, minimumLargeAssetUploadSeconds,
		)
	}
	oneSlowSuccessBudget := 2*listSeconds + uploadSeconds + verifySeconds
	if oneSlowSuccessBudget > perAssetSeconds {
		t.Fatalf(
			"one complete slow upload budget = %ds, exceeds per-asset deadline %ds",
			oneSlowSuccessBudget, perAssetSeconds,
		)
	}
	fastFailureRetryBudget := uploadSeconds + (postUploadVerifyAttempts+3)*listSeconds +
		verifySeconds + (postUploadVerifyAttempts-1)*verifyRetryDelay + uploadRetryDelay
	if fastFailureRetryBudget > perAssetSeconds {
		t.Fatalf(
			"fast-failure retry budget = %ds, exceeds per-asset deadline %ds",
			fastFailureRetryBudget, perAssetSeconds,
		)
	}
	finalListSeconds := listSeconds
	completeAssetBudget := perAssetSeconds*len(requiredGiteeAssets) + finalListSeconds
	if completeAssetBudget > overallSeconds {
		t.Fatalf(
			"complete asset budget = %ds (%d assets x %ds + %ds final list), exceeds overall deadline %ds",
			completeAssetBudget, len(requiredGiteeAssets), perAssetSeconds, finalListSeconds, overallSeconds,
		)
	}
	if completeAssetBudget > reconcileSeconds || reconcileSeconds > overallSeconds {
		t.Fatalf(
			"asset budget nesting invalid: complete=%ds configured-reconcile=%ds reconciler-overall=%ds",
			completeAssetBudget, reconcileSeconds, overallSeconds,
		)
	}

	childDeadlineReserveSeconds := shellDefaultInt(t, syncScript, "GITEE_CHILD_DEADLINE_RESERVE_SECONDS")
	completeSyncBudget := tagSeconds + lookupSeconds + createSeconds + reconcileSeconds + childDeadlineReserveSeconds
	if completeSyncBudget > syncSeconds {
		t.Fatalf(
			"complete sync budget = %ds (tag=%d + lookup=%d + create=%d + reconcile=%d + child reserve=%d), exceeds sync deadline %ds",
			completeSyncBudget, tagSeconds, lookupSeconds, createSeconds, reconcileSeconds, childDeadlineReserveSeconds, syncSeconds,
		)
	}

	const workflowReserveMinutes = 5
	releasePath := mustAbs(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	releaseData, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", releasePath, err)
	}
	assertWorkflowBudget(
		t,
		string(releaseData),
		"mirror-gitee-release:",
		[]string{
			"name: Check out sealed release source",
			"name: Restore finalized distribution files",
			"name: Mirror release to Gitee (China)",
		},
		workflowReserveMinutes,
	)
	releaseSyncStepSeconds := workflowTimeoutMinutesAfter(
		t, string(releaseData), "mirror-gitee-release:", "name: Mirror release to Gitee (China)",
	) * 60
	if syncSeconds+workflowReserveMinutes*60 > releaseSyncStepSeconds {
		t.Fatalf(
			"sync deadline %ds plus reserve exceeds release fallback step %ds",
			syncSeconds, releaseSyncStepSeconds,
		)
	}

	const repairWorkflowReserveMinutes = 20
	assertWorkflowBudget(
		t,
		string(releaseData),
		"repair-channel:",
		[]string{"name: Mirror release to Gitee (China)"},
		repairWorkflowReserveMinutes,
	)
	repairSyncStepSeconds := workflowTimeoutMinutesAfter(
		t, string(releaseData), "repair-channel:", "name: Mirror release to Gitee (China)",
	) * 60
	if syncSeconds+workflowReserveMinutes*60 > repairSyncStepSeconds {
		t.Fatalf(
			"sync deadline %ds plus reserve exceeds repair step %ds",
			syncSeconds, repairSyncStepSeconds,
		)
	}

	localBuildPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "build-and-publish-gitee.sh"))
	localBuildData, err := os.ReadFile(localBuildPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", localBuildPath, err)
	}
	if !strings.Contains(string(localBuildData), "Direct Gitee release builds are disabled") {
		t.Fatal("local Gitee builds must stay disabled so only immutable GitHub assets are mirrored")
	}
	localPublishPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "publish-gitee-local.sh"))
	localPublishData, err := os.ReadFile(localPublishPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", localPublishPath, err)
	}
	if !strings.Contains(string(localPublishData), "Direct Gitee artifact publication is disabled") {
		t.Fatal("direct local Gitee publication must stay disabled")
	}

	tagSyncPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "sync-gitee-tag.sh"))
	tagSyncData, err := os.ReadFile(tagSyncPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", tagSyncPath, err)
	}
	tagSync := string(tagSyncData)
	if strings.Contains(tagSync, `https://${GITEE_USER}:${GITEE_TOKEN}@`) {
		t.Fatal("Gitee git remote must not embed credentials")
	}
	for _, required := range []string{"GIT_ASKPASS", "GIT_TERMINAL_PROMPT", "DWS_GITEE_TOKEN"} {
		if !strings.Contains(tagSync, required) {
			t.Fatalf("Gitee tag sync missing askpass safeguard %q", required)
		}
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%s) error = %v", path, err)
	}
	return abs
}

func seedTaggedRepository(t *testing.T, workDir, tag string) {
	t.Helper()
	mustRun(t, t.TempDir(), "git", "init", workDir)
	mustRun(t, workDir, "git", "config", "user.name", "Release Test")
	mustRun(t, workDir, "git", "config", "user.email", "release-test@example.com")
	mustWriteFile(t, filepath.Join(workDir, "payload.txt"), []byte("release bytes\n"), 0o644)
	mustRun(t, workDir, "git", "add", "payload.txt")
	mustRun(t, workDir, "git", "commit", "-m", "release commit")
	mustRun(t, workDir, "git", "tag", "-a", tag, "-m", tag)
}

func runGiteeTagSync(t *testing.T, scriptPath, workDir, remoteDir, tag string, wantSuccess bool, extraEnv ...string) string {
	t.Helper()
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"VERSION="+tag,
		"GITEE_SOURCE_REMOTE=",
		"GITEE_GIT_REMOTE="+remoteDir,
		"GITEE_PUBLIC_GIT_REMOTE="+remoteDir,
		"GITEE_TAG_VERIFY_ATTEMPTS=1",
		"GITEE_TAG_VERIFY_DELAY=0",
		"GITEE_GIT_TIMEOUT_SECONDS=10",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	output, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("sync-gitee-tag.sh error = %v\noutput:\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("sync-gitee-tag.sh unexpectedly succeeded:\n%s", output)
	}
	return string(output)
}

func shellDefaultInt(t *testing.T, content, name string) int {
	t.Helper()
	prefix := name + `="${` + name + `:-`
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, `}"`) {
			continue
		}
		raw := strings.TrimSuffix(strings.TrimPrefix(line, prefix), `}"`)
		value, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("parse %s default %q: %v", name, raw, err)
		}
		return value
	}
	t.Fatalf("shell default %s not found", name)
	return 0
}

func workflowTimeoutMinutesAfter(t *testing.T, content string, markers ...string) int {
	t.Helper()
	for _, marker := range markers {
		index := strings.Index(content, marker)
		if index < 0 {
			t.Fatalf("workflow marker %q not found", marker)
		}
		content = content[index+len(marker):]
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		const prefix = "timeout-minutes:"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		value, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("parse workflow timeout %q: %v", raw, err)
		}
		return value
	}
	t.Fatal("workflow timeout-minutes not found")
	return 0
}

func assertWorkflowBudget(t *testing.T, content, jobMarker string, stepMarkers []string, reserveMinutes int) {
	t.Helper()
	jobMinutes := workflowTimeoutMinutesAfter(t, content, jobMarker)
	stepMinutes := 0
	jobContent := content[strings.Index(content, jobMarker)+len(jobMarker):]
	for _, marker := range stepMarkers {
		stepMinutes += workflowTimeoutMinutesAfter(t, jobContent, marker)
	}
	if stepMinutes+reserveMinutes > jobMinutes {
		t.Fatalf(
			"workflow budget for %s = %d step minutes + %d reserve, exceeds %d-minute job",
			jobMarker, stepMinutes, reserveMinutes, jobMinutes,
		)
	}
}

func peeledRemoteTag(t *testing.T, workDir, remoteDir, tag string) string {
	t.Helper()
	output := mustOutput(t, workDir, "git", "ls-remote", remoteDir, "refs/tags/"+tag+"^{}")
	fields := strings.Fields(output)
	if len(fields) != 2 {
		t.Fatalf("unexpected ls-remote output for %s: %q", tag, output)
	}
	return fields[0]
}

func seedGiteeDist(t *testing.T) string {
	t.Helper()
	distDir := t.TempDir()
	for _, name := range requiredGiteeAssets {
		mustWriteFile(t, filepath.Join(distDir, name), []byte("payload for "+name+"\n"), 0o644)
	}
	return distDir
}

func giteeAssetEnv(distDir, apiURL, retries string) []string {
	return append(os.Environ(),
		"DIST_DIR="+distDir,
		"GITEE_API="+apiURL,
		"GITEE_TOKEN=test-token",
		"GITEE_REPO=owner/repo",
		"GITEE_RELEASE_ID=1",
		"GITEE_CURL_CONNECT_TIMEOUT=2",
		"GITEE_CURL_MAX_TIME=2",
		"GITEE_UPLOAD_MAX_TIME=2",
		"GITEE_UPLOAD_RETRIES="+retries,
		"GITEE_UPLOAD_RETRY_DELAY=0",
		"GITEE_EXISTING_VERIFY_ATTEMPTS=1",
		"GITEE_POST_UPLOAD_VERIFY_ATTEMPTS=1",
		"GITEE_VERIFY_RETRY_DELAY=0",
	)
}

type fakeGiteeAsset struct {
	id   int
	name string
	data []byte
}

type fakeGiteeRelease struct {
	mu                   sync.Mutex
	nextID               int
	assets               map[int]fakeGiteeAsset
	uploadCalls          map[string]int
	dropFirstResponse    bool
	droppedResponse      bool
	failUploads          bool
	uploadDelay          time.Duration
	rejectExpectContinue bool
}

func newFakeGiteeRelease(dropFirstResponse, failUploads bool) *fakeGiteeRelease {
	return &fakeGiteeRelease{
		nextID:            1,
		assets:            make(map[int]fakeGiteeAsset),
		uploadCalls:       make(map[string]int),
		dropFirstResponse: dropFirstResponse,
		failUploads:       failUploads,
	}
}

func (f *fakeGiteeRelease) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const attachPath = "/repos/owner/repo/releases/1/attach_files"
	if strings.HasPrefix(r.URL.Path, "/repos/") {
		if got := r.Header.Get("Authorization"); got != "token test-token" {
			http.Error(w, "missing token authorization header", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("access_token") != "" {
			http.Error(w, "token leaked in query string", http.StatusBadRequest)
			return
		}
	} else if r.Header.Get("Authorization") != "" {
		http.Error(w, "token forwarded to asset download URL", http.StatusBadRequest)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == attachPath:
		f.list(w, r)
	case r.Method == http.MethodPost && r.URL.Path == attachPath:
		f.upload(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, attachPath+"/"):
		f.delete(w, strings.TrimPrefix(r.URL.Path, attachPath+"/"))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/download/"):
		f.download(w, strings.TrimPrefix(r.URL.Path, "/download/"))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGiteeRelease) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	baseURL := requestBaseURL(r)
	rows := make([]map[string]any, 0, len(f.assets))
	for _, asset := range f.assets {
		rows = append(rows, map[string]any{
			"id":                   asset.id,
			"name":                 asset.name,
			"browser_download_url": fmt.Sprintf("%s/download/%d", baseURL, asset.id),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

func (f *fakeGiteeRelease) upload(w http.ResponseWriter, r *http.Request) {
	if f.rejectExpectContinue && r.Header.Get("Expect") != "" {
		http.Error(w, "Expect: 100-continue is not supported", http.StatusExpectationFailed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.FormValue("access_token") != "" {
		http.Error(w, "token leaked in upload form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if f.uploadDelay > 0 {
		time.Sleep(f.uploadDelay)
	}
	baseURL := requestBaseURL(r)

	f.mu.Lock()
	f.uploadCalls[header.Filename]++
	if f.failUploads {
		f.mu.Unlock()
		http.Error(w, "temporary Gitee failure", http.StatusServiceUnavailable)
		return
	}
	id := f.nextID
	f.nextID++
	f.assets[id] = fakeGiteeAsset{id: id, name: header.Filename, data: data}
	drop := f.dropFirstResponse && !f.droppedResponse
	if drop {
		f.droppedResponse = true
	}
	f.mu.Unlock()

	if drop {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking unavailable", http.StatusInternalServerError)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                   id,
		"name":                 header.Filename,
		"browser_download_url": fmt.Sprintf("%s/download/%d", baseURL, id),
	})
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (f *fakeGiteeRelease) delete(w http.ResponseWriter, rawID string) {
	id, err := strconv.Atoi(rawID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	delete(f.assets, id)
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeGiteeRelease) download(w http.ResponseWriter, rawID string) {
	id, err := strconv.Atoi(rawID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	asset, ok := f.assets[id]
	f.mu.Unlock()
	if !ok {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}
	_, _ = w.Write(asset.data)
}

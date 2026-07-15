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

func TestReconcileGiteeAssetsRecoversACommittedUploadWithLostResponse(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	distDir := seedGiteeDist(t)
	fake := newFakeGiteeRelease(true, false)
	server := httptest.NewServer(fake)
	defer server.Close()
	fake.baseURL = server.URL

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

func TestReconcileGiteeAssetsFailsWhenAnyUploadIsMissing(t *testing.T) {
	scriptPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "reconcile-gitee-assets.sh"))
	distDir := seedGiteeDist(t)
	fake := newFakeGiteeRelease(false, true)
	server := httptest.NewServer(fake)
	defer server.Close()
	fake.baseURL = server.URL

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

func TestGiteeWorkflowsUseImmutableTagsAndBoundedRetryBudget(t *testing.T) {
	mirrorPath := mustAbs(t, filepath.Join("..", "..", ".github", "workflows", "mirror-to-gitee.yml"))
	mirrorData, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", mirrorPath, err)
	}
	mirror := string(mirrorData)
	if !strings.Contains(mirror, `VERSION="$GITHUB_REF_NAME" ./scripts/release/sync-gitee-tag.sh`) {
		t.Fatal("tag workflow must delegate immutable tag reconciliation to sync-gitee-tag.sh")
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
	manualData, err := os.ReadFile(manualPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", manualPath, err)
	}
	if !strings.Contains(string(manualData), "timeout-minutes: 90") {
		t.Fatal("manual Gitee repair workflow must exceed the bounded per-file retry budget")
	}

	localBuildPath := mustAbs(t, filepath.Join("..", "..", "scripts", "release", "build-and-publish-gitee.sh"))
	localBuildData, err := os.ReadFile(localBuildPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", localBuildPath, err)
	}
	if !strings.Contains(string(localBuildData), "publish-gitee-local.sh reconcile-gitee-assets.sh") {
		t.Fatal("detached-tag Gitee builds must copy the shared asset reconciler with the publisher")
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

func runGiteeTagSync(t *testing.T, scriptPath, workDir, remoteDir, tag string, wantSuccess bool) string {
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
	output, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("sync-gitee-tag.sh error = %v\noutput:\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("sync-gitee-tag.sh unexpectedly succeeded:\n%s", output)
	}
	return string(output)
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
	mu                sync.Mutex
	baseURL           string
	nextID            int
	assets            map[int]fakeGiteeAsset
	uploadCalls       map[string]int
	dropFirstResponse bool
	droppedResponse   bool
	failUploads       bool
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
	switch {
	case r.Method == http.MethodGet && r.URL.Path == attachPath:
		f.list(w)
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

func (f *fakeGiteeRelease) list(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := make([]map[string]any, 0, len(f.assets))
	for _, asset := range f.assets {
		rows = append(rows, map[string]any{
			"id":                   asset.id,
			"name":                 asset.name,
			"browser_download_url": fmt.Sprintf("%s/download/%d", f.baseURL, asset.id),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

func (f *fakeGiteeRelease) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		"browser_download_url": fmt.Sprintf("%s/download/%d", f.baseURL, id),
	})
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

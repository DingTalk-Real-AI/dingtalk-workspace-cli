// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

type driveCommandRunner struct {
	calls  int
	all    []executor.Invocation
	last   executor.Invocation
	result executor.Result
	err    error
}

func (r *driveCommandRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.calls++
	r.last = invocation
	r.all = append(r.all, invocation)
	if r.err != nil {
		return executor.Result{}, r.err
	}
	if r.result.Response != nil {
		r.result.Invocation = invocation
		return r.result, nil
	}
	return executor.Result{Invocation: invocation}, nil
}

func TestDriveListPageSizeAliasMapsMaxResults(t *testing.T) {
	t.Parallel()

	runner := &driveCommandRunner{}
	cmd := newDriveListCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--page-size", "20"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.Tool != "list_files" {
		t.Fatalf("tool = %q, want list_files", runner.last.Tool)
	}
	if got := runner.last.Params["maxResults"]; got != float64(20) {
		t.Fatalf("maxResults = %#v, want 20", got)
	}
}

func TestDriveListWorkspaceRoutesToDocListNodes(t *testing.T) {
	t.Parallel()

	runner := &driveCommandRunner{}
	cmd := newDriveListCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--workspace-id", "WS_001", "--folder", "FOLDER_001", "--limit", "10"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.CanonicalProduct != "doc" {
		t.Fatalf("product = %q, want doc", runner.last.CanonicalProduct)
	}
	if runner.last.Tool != "list_nodes" {
		t.Fatalf("tool = %q, want list_nodes", runner.last.Tool)
	}
	if got := runner.last.Params["workspaceId"]; got != "WS_001" {
		t.Fatalf("workspaceId = %#v, want WS_001", got)
	}
	if got := runner.last.Params["folderId"]; got != "FOLDER_001" {
		t.Fatalf("folderId = %#v, want FOLDER_001", got)
	}
	if got := runner.last.Params["pageSize"]; got != 10 {
		t.Fatalf("pageSize = %#v, want 10", got)
	}
}

func TestDriveCopyAliasesRouteToDoc(t *testing.T) {
	t.Parallel()

	runner := &driveCommandRunner{}
	cmd := newDriveCopyCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--file-id", "NODE_001", "--parent-id", "FOLDER_001", "--workspace-id", "WS_001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.CanonicalProduct != "doc" || runner.last.Tool != "copy_document" {
		t.Fatalf("invocation = %#v, want doc copy_document", runner.last)
	}
	if got := runner.last.Params["nodeId"]; got != "NODE_001" {
		t.Fatalf("nodeId = %#v, want NODE_001", got)
	}
	if got := runner.last.Params["targetFolderId"]; got != "FOLDER_001" {
		t.Fatalf("targetFolderId = %#v, want FOLDER_001", got)
	}
	if got := runner.last.Params["workspaceId"]; got != "WS_001" {
		t.Fatalf("workspaceId = %#v, want WS_001", got)
	}
}

func TestDrivePermissionRemoveRoutesToDoc(t *testing.T) {
	t.Parallel()

	runner := &driveCommandRunner{}
	cmd := newDrivePermissionCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"remove", "--node", "NODE_001", "--users", "uid1,uid2"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if runner.last.CanonicalProduct != "doc" || runner.last.Tool != "remove_permission" {
		t.Fatalf("invocation = %#v, want doc remove_permission", runner.last)
	}
	users, ok := runner.last.Params["userIds"].([]string)
	if !ok || strings.Join(users, ",") != "uid1,uid2" {
		t.Fatalf("userIds = %#v, want uid1,uid2", runner.last.Params["userIds"])
	}
}

func TestDriveSearchAggregatesDriveAndDoc(t *testing.T) {
	t.Parallel()

	runner := &driveCommandRunner{}
	cmd := newDriveSearchCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--query", "报告", "--extensions", "pdf,docx", "--limit", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	if len(runner.all) != 2 {
		t.Fatalf("calls = %d, want 2", len(runner.all))
	}
	if runner.all[0].CanonicalProduct != "drive" || runner.all[0].Tool != "search_files" {
		t.Fatalf("first invocation = %#v, want drive search_files", runner.all[0])
	}
	if runner.all[1].CanonicalProduct != "doc" || runner.all[1].Tool != "search_documents" {
		t.Fatalf("second invocation = %#v, want doc search_documents", runner.all[1])
	}
}

func TestDriveDownloadOutputDirectoryUsesServerFileName(t *testing.T) {
	t.Parallel()

	wantBody := []byte("download body")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write(wantBody)
	}))
	defer server.Close()

	runner := &driveCommandRunner{
		result: executor.Result{
			Response: map[string]any{
				"content": map[string]any{
					"result": map[string]any{
						"resourceUrl": server.URL + "/url-derived.bin",
						"fileName":    "server-name.txt",
					},
				},
			},
		},
	}
	outputDir := t.TempDir()
	cmd := newDriveDownloadCommand(runner)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--file-id", "FILE_001", "--output", outputDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	gotPath := filepath.Join(outputDir, "server-name.txt")
	gotBody, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", gotPath, err)
	}
	if string(gotBody) != string(wantBody) {
		t.Fatalf("downloaded body = %q, want %q", string(gotBody), string(wantBody))
	}
	if _, err := os.Stat(filepath.Join(outputDir, "url-derived.bin")); !os.IsNotExist(err) {
		t.Fatalf("URL-derived filename should not be used, stat error = %v", err)
	}
}

func TestParseDriveDownloadInfoUsesNameWhenFileNameAbsent(t *testing.T) {
	t.Parallel()

	resourceURL, filename, _, err := parseDriveDownloadInfo(map[string]any{
		"result": map[string]any{
			"downloadUrl": "https://example.com/fallback.bin",
			"name":        "server-name-from-name.txt",
		},
	})
	if err != nil {
		t.Fatalf("parseDriveDownloadInfo() error = %v", err)
	}
	if resourceURL != "https://example.com/fallback.bin" {
		t.Fatalf("resourceURL = %q, want fallback URL", resourceURL)
	}
	if filename != "server-name-from-name.txt" {
		t.Fatalf("filename = %q, want server-name-from-name.txt", filename)
	}
}

// TestHttpPutDriveFile_NoContentTypeWhenServerHeadersEmpty guards the fix for
// the SignatureDoesNotMatch bug on DingTalk drive presigned OSS uploads.
//
// DingTalk drive returns an OSS presigned URL (signature in the URL query
// string) and signs the upload with Content-Type left empty. Any client-side
// Content-Type makes the signature OSS computes at PUT time differ from the
// server presignature → 403 SignatureDoesNotMatch.
//
// Previous behavior: httpPutDriveFile fell back to a client-inferred mime when
// the server's `headers` map was empty, which is the normal case for DingTalk
// drive (`{"headers": {}}`). That fallback broke every PNG / image / typed-mime
// upload in production.
//
// This test asserts the PUT request body contains no Content-Type header when
// the server returns an empty headers map. If a future change reintroduces
// client-side Content-Type fallback this test will fail loudly.
func TestHttpPutDriveFile_NoContentTypeWhenServerHeadersEmpty(t *testing.T) {
	var receivedContentType string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmp := filepath.Join(t.TempDir(), "test.png")
	wantBody := []byte("fake-png-bytes")
	if err := os.WriteFile(tmp, wantBody, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := httpPutDriveFile(context.Background(), server.URL, map[string]string{}, tmp, int64(len(wantBody)))
	if err != nil {
		t.Fatalf("httpPutDriveFile() error = %v", err)
	}
	if receivedContentType != "" {
		t.Fatalf("Content-Type = %q, want empty (presigned URL signing requires no client-inferred headers)", receivedContentType)
	}
	if string(receivedBody) != string(wantBody) {
		t.Fatalf("uploaded body = %q, want %q", string(receivedBody), string(wantBody))
	}
}

// TestHttpPutDriveFile_PassthroughServerHeaders verifies that any header the
// server returns in its prepare response is forwarded verbatim to the PUT
// request. This is the symmetric guarantee to the test above: clients must
// neither add nor drop headers — they pass through exactly what the server
// declared.
func TestHttpPutDriveFile_PassthroughServerHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmp := filepath.Join(t.TempDir(), "test.bin")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	headers := map[string]string{
		"Content-Type":        "application/octet-stream",
		"x-oss-storage-class": "Standard",
	}
	err := httpPutDriveFile(context.Background(), server.URL, headers, tmp, 1)
	if err != nil {
		t.Fatalf("httpPutDriveFile() error = %v", err)
	}
	if got := receivedHeaders.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := receivedHeaders.Get("x-oss-storage-class"); got != "Standard" {
		t.Fatalf("x-oss-storage-class = %q, want Standard", got)
	}
}

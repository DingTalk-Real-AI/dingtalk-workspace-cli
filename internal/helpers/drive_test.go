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
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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

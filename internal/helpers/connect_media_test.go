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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPictureDownloadCode(t *testing.T) {
	cases := []struct {
		name    string
		content interface{}
		want    string
	}{
		{"downloadCode", map[string]interface{}{"downloadCode": "dc-1"}, "dc-1"},
		{"pictureDownloadCode fallback", map[string]interface{}{"pictureDownloadCode": "dc-2"}, "dc-2"},
		{"blank ignored", map[string]interface{}{"downloadCode": "  "}, ""},
		{"wrong type", "not-a-map", ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := pictureDownloadCode(tc.content); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestDownloadMessageFile drives the full resolve-then-fetch flow against a
// fake API: token → messageFiles/download (must carry robotCode+downloadCode)
// → presigned GET → local temp file.
func TestDownloadMessageFile(t *testing.T) {
	var gotDownloadReq map[string]any
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-acs-dingtalk-access-token") != "tok-1" {
			t.Error("missing access token header")
		}
		_ = json.NewDecoder(r.Body).Decode(&gotDownloadReq)
		_ = json.NewEncoder(w).Encode(map[string]any{"downloadUrl": srv.URL + "/file"})
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	path, err := c.downloadMessageFile(context.Background(), "ding-client", "dc-1")
	if err != nil {
		t.Fatalf("downloadMessageFile: %v", err)
	}
	defer os.Remove(path)

	if gotDownloadReq["robotCode"] != "ding-client" || gotDownloadReq["downloadCode"] != "dc-1" {
		t.Fatalf("download request payload = %v", gotDownloadReq)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("path = %q, want .png suffix from content type", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "PNGDATA" {
		t.Fatalf("saved file = %q, %v", raw, err)
	}
}

func TestDownloadMessageFileNoURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	if _, err := c.downloadMessageFile(context.Background(), "ding-client", "dc-1"); err == nil {
		t.Fatal("missing downloadUrl should error")
	}
}

func TestMediaExt(t *testing.T) {
	cases := []struct{ url, ct, want string }{
		{"https://x/y", "image/png", ".png"},
		{"https://x/y", "image/jpeg", ".jpg"},
		{"https://x/y.webp?sig=1", "", ".webp"},
		{"https://x/y", "", ".png"},
	}
	for _, tc := range cases {
		if got := mediaExt(tc.url, tc.ct); got != tc.want {
			t.Fatalf("mediaExt(%q, %q) = %q, want %q", tc.url, tc.ct, got, tc.want)
		}
	}
}

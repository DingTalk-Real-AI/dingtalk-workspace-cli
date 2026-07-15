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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiChannelUsesAPIWithoutLocalCLI(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_API_BASE_URL", "https://gemini-proxy.example/v1beta")

	fwd, err := forwarderForChannel("gemini", "", connectAgentOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("forwarderForChannel(gemini): %v", err)
	}
	gf, ok := fwd.(*geminiAPIForwarder)
	if !ok {
		t.Fatalf("gemini forwarder = %T, want *geminiAPIForwarder", fwd)
	}
	if gf.model != "gemini-test" {
		t.Fatalf("model = %q, want gemini-test", gf.model)
	}
	if gf.baseURL != "https://gemini-proxy.example/v1beta" {
		t.Fatalf("baseURL = %q", gf.baseURL)
	}
}

func TestGeminiAPIForwarderSendsSmallAttachmentInline(t *testing.T) {
	clearChannelEnv(t)
	path := filepath.Join(t.TempDir(), "voice.mp3")
	wantBytes := []byte("real-audio-bytes")
	if err := os.WriteFile(path, wantBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var got geminiPart
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Contents) == 1 && len(req.Contents[0].Parts) == 2 {
			got = req.Contents[0].Parts[1]
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	}))
	defer ts.Close()
	f := &geminiAPIForwarder{model: "gemini-test", apiKey: "key", baseURL: ts.URL, httpClient: ts.Client()}
	_, err := f.forwardWithAttachments(context.Background(), "conv", "转写", []connectMediaAttachment{{LocalPath: path, FileName: "voice.mp3", MediaType: "audio"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.InlineData == nil || got.InlineData.MIMEType != "audio/mpeg" {
		t.Fatalf("inlineData = %#v", got.InlineData)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.InlineData.Data)
	if err != nil || string(decoded) != string(wantBytes) {
		t.Fatalf("inline bytes = %q, err=%v", decoded, err)
	}
}

func TestGeminiAPIForwarderUploadsLargeAttachment(t *testing.T) {
	clearChannelEnv(t)
	path := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(path, make([]byte, geminiInlineRawLimit+1), 0o600); err != nil {
		t.Fatal(err)
	}
	var started, uploaded, generated bool
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload/v1beta/files":
			started = r.Header.Get("X-Goog-Upload-Command") == "start"
			w.Header().Set("X-Goog-Upload-URL", ts.URL+"/upload-session")
		case "/upload-session":
			uploaded = r.Header.Get("X-Goog-Upload-Command") == "upload, finalize"
			_, _ = w.Write([]byte(`{"file":{"name":"files/1","uri":"https://files.example/1","mimeType":"video/mp4","state":"ACTIVE"}}`))
		case "/v1beta/models/gemini-test:generateContent":
			var req geminiGenerateContentRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			generated = len(req.Contents) == 1 && len(req.Contents[0].Parts) == 2 && req.Contents[0].Parts[1].FileData != nil && req.Contents[0].Parts[1].FileData.FileURI == "https://files.example/1"
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	f := &geminiAPIForwarder{model: "gemini-test", apiKey: "key", baseURL: ts.URL + "/v1beta", httpClient: ts.Client()}
	_, err := f.forwardWithAttachments(context.Background(), "conv", "分析", []connectMediaAttachment{{LocalPath: path, FileName: "video.mp4", MediaType: "video"}})
	if err != nil {
		t.Fatal(err)
	}
	if !started || !uploaded || !generated {
		t.Fatalf("started=%v uploaded=%v generated=%v", started, uploaded, generated)
	}
}

func TestGeminiAPIForwarderForward(t *testing.T) {
	clearChannelEnv(t)
	var gotPath, gotKey, gotText string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-goog-api-key")
		var req geminiGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Contents) > 0 && len(req.Contents[0].Parts) > 0 {
			gotText = req.Contents[0].Parts[0].Text
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"gemini-api-ok"}]}}]}`))
	}))
	defer ts.Close()
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_API_BASE_URL", ts.URL)

	fwd, err := forwarderForChannel("gemini", "", connectAgentOptions{Model: "models/gemini-test"})
	if err != nil {
		t.Fatalf("forwarderForChannel(gemini): %v", err)
	}
	reply, err := fwd.forward(context.Background(), "conv-1", "你好")
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if reply != "gemini-api-ok" {
		t.Fatalf("reply = %q, want gemini-api-ok", reply)
	}
	if gotPath != "/models/gemini-test:generateContent" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "test-key" {
		t.Fatalf("x-goog-api-key = %q", gotKey)
	}
	if gotText != "你好" {
		t.Fatalf("prompt text = %q", gotText)
	}
}

func TestGeminiChannelRequiresAPIKey(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	_, err := forwarderForChannel("gemini", "", connectAgentOptions{})
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("err = %v, want GEMINI_API_KEY validation", err)
	}
}

func TestGeminiCLIStatusUsesAPIKey(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	if got := connectCliStatus("gemini")["installed"]; got != false {
		t.Fatalf("installed without key = %v, want false", got)
	}
	t.Setenv("GOOGLE_API_KEY", "google-key")
	if got := connectCliStatus("gemini")["installed"]; got != true {
		t.Fatalf("installed with key = %v, want true", got)
	}
	if got := connectCliStatus("gemini")["autoInstall"]; got != false {
		t.Fatalf("autoInstall = %v, want false", got)
	}
}

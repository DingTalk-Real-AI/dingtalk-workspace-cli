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

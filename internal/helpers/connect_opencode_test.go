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
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpencodeForwarderUsesServerSessionAPI(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "opencode-sessions.json")
	var calls []string
	var prompts []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true,"version":"test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"id":"ses_server"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_server/message":
			var req struct {
				Parts []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"parts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode message request: %v", err)
			}
			if len(req.Parts) != 1 || req.Parts[0].Type != "text" {
				t.Fatalf("message parts = %#v, want one text part", req.Parts)
			}
			prompts = append(prompts, req.Parts[0].Text)
			_, _ = w.Write([]byte(`{"info":{"id":"msg_1"},"parts":[{"type":"text","text":"reply ` + req.Parts[0].Text + `"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := &opencodeForwarder{
		bin:      "opencode",
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(storePath),
		server:   &opencodeServer{baseURL: ts.URL, httpClient: ts.Client()},
	}

	reply, err := f.forwardStream(context.Background(), "conv-1", "hi", func(string) {
		t.Fatal("opencode server mode must not stream deltas")
	})
	if err != nil {
		t.Fatalf("first forward: %v", err)
	}
	if reply != "reply hi" {
		t.Fatalf("first reply = %q, want reply hi", reply)
	}
	if got := f.sessions.id("conv-1"); got != "ses_server" {
		t.Fatalf("captured session = %q, want ses_server", got)
	}

	reply, err = f.forwardStream(context.Background(), "conv-1", "again", nil)
	if err != nil {
		t.Fatalf("second forward: %v", err)
	}
	if reply != "reply again" {
		t.Fatalf("second reply = %q, want reply again", reply)
	}
	if got := strings.Join(calls, "\n"); strings.Count(got, "POST /session\n") != 1 && strings.Count(got, "POST /session") != 1 {
		t.Fatalf("POST /session should be called once, calls:\n%s", got)
	}
	if got := strings.Join(prompts, ","); got != "hi,again" {
		t.Fatalf("prompts = %q, want hi,again", got)
	}
	if f.canStream() {
		t.Fatal("opencode server mode should be one-shot for group chat replies")
	}
}

func TestOpencodeForwarderRecreatesMissingServerSession(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "opencode-sessions.json")
	var created int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true,"version":"test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			created++
			_, _ = w.Write([]byte(`{"id":"ses_new"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_stale/message":
			http.Error(w, `{"error":"missing"}`, http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_new/message":
			_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"fresh"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := &opencodeForwarder{
		bin:      "opencode",
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(storePath),
		server:   &opencodeServer{baseURL: ts.URL, httpClient: ts.Client()},
	}
	f.sessions.set("conv-1", "ses_stale")

	reply, err := f.forwardStream(context.Background(), "conv-1", "again", nil)
	if err != nil {
		t.Fatalf("forward after missing session: %v", err)
	}
	if reply != "fresh" {
		t.Fatalf("reply = %q, want fresh", reply)
	}
	if got := f.sessions.id("conv-1"); got != "ses_new" {
		t.Fatalf("session after retry = %q, want ses_new", got)
	}
	if created != 1 {
		t.Fatalf("created sessions = %d, want 1", created)
	}
}

func TestOpencodeForwarderKeepsSessionWhenServerReturnsNoText(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "opencode-sessions.json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true,"version":"test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_keep/message":
			_, _ = w.Write([]byte(`{"parts":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := &opencodeForwarder{
		bin:      "opencode",
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(storePath),
		server:   &opencodeServer{baseURL: ts.URL, httpClient: ts.Client()},
	}
	f.sessions.set("conv-1", "ses_keep")

	reply, err := f.forwardStream(context.Background(), "conv-1", "question", nil)
	if err != nil {
		t.Fatalf("forward with empty response: %v", err)
	}
	if reply != "（本地 agent 无文本输出）" {
		t.Fatalf("reply = %q, want no-text hint", reply)
	}
	if got := f.sessions.id("conv-1"); got != "ses_keep" {
		t.Fatalf("session after no-text response = %q, want ses_keep", got)
	}
}

func TestOpencodeForwarderOnlyReturnsTextParts(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "opencode-sessions.json")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true,"version":"test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"id":"ses_parts"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_parts/message":
			_, _ = w.Write([]byte(`{"parts":[{"type":"reasoning","text":"hidden reasoning"},{"type":"text","text":"visible"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := &opencodeForwarder{
		bin:      "opencode",
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(storePath),
		server:   &opencodeServer{baseURL: ts.URL, httpClient: ts.Client()},
	}

	reply, err := f.forwardStream(context.Background(), "conv-1", "question", nil)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if reply != "visible" {
		t.Fatalf("reply = %q, want visible", reply)
	}
}

// TestOpencodeSessionsPersist verifies the convID→sessionID store survives a
// restart and that reset is persisted.
func TestOpencodeSessionsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode-sessions.json")

	s := newOpencodeSessions(path)
	s.set("conv-1", "ses_a")
	s.set("conv-2", "ses_b")

	restarted := newOpencodeSessions(path)
	if got := restarted.id("conv-1"); got != "ses_a" {
		t.Errorf("after restart conv-1 = %q, want ses_a", got)
	}
	if got := restarted.id("conv-2"); got != "ses_b" {
		t.Errorf("after restart conv-2 = %q, want ses_b", got)
	}

	restarted.reset("conv-1")
	again := newOpencodeSessions(path)
	if got := again.id("conv-1"); got != "" {
		t.Errorf("after reset+restart conv-1 = %q, want empty", got)
	}
	if got := again.id("conv-2"); got != "ses_b" {
		t.Errorf("after reset+restart conv-2 = %q, want ses_b (untouched)", got)
	}
}

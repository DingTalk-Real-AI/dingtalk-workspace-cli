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

// TestNewOpencodeServerHasNoClientTimeout pins the fix: the shared HTTP client
// must not carry an overall deadline, otherwise long agent turns get aborted
// with "Client.Timeout exceeded while awaiting headers". The per-turn ctx is
// the only thing allowed to bound a message round-trip.
func TestNewOpencodeServerHasNoClientTimeout(t *testing.T) {
	s := newOpencodeServer("opencode", nil, "")
	if s.httpClient == nil {
		t.Fatal("httpClient must be initialized")
	}
	if s.httpClient.Timeout != 0 {
		t.Fatalf("opencode http client Timeout = %s, want 0 (per-turn ctx governs long agent replies)", s.httpClient.Timeout)
	}
}

// TestOpencodeForwarderMessageGovernedByTurnCtx verifies a slow reply is not
// cut by a fixed client timeout (the old 30s bug) and that the per-turn ctx is
// the real governor instead.
func TestOpencodeForwarderMessageGovernedByTurnCtx(t *testing.T) {
	dir := t.TempDir()
	const replyDelay = 200 * time.Millisecond

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true}`))
		case r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"id":"ses_slow"}`))
		case r.URL.Path == "/session/ses_slow/message":
			select {
			case <-time.After(replyDelay):
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"slow ok"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	newForwarder := func(turn time.Duration) *opencodeForwarder {
		return &opencodeForwarder{
			bin:      "opencode",
			timeout:  turn,
			workDir:  dir,
			sessions: newOpencodeSessions(filepath.Join(dir, "s.json")),
			// &http.Client{} mirrors the fixed production default (no overall cap).
			server: &opencodeServer{baseURL: ts.URL, httpClient: &http.Client{}},
		}
	}

	// Generous turn budget: the slow reply must come back, not be cut by a cap.
	reply, err := newForwarder(5*time.Second).forwardStream(context.Background(), "conv-ok", "hi", nil)
	if err != nil {
		t.Fatalf("slow reply within the turn budget should succeed, got: %v", err)
	}
	if !strings.Contains(reply, "slow ok") {
		t.Fatalf("reply = %q, want it to contain slow ok", reply)
	}

	// Turn budget shorter than the reply delay: the per-turn ctx is what bounds
	// the round-trip, so this must error out (proving ctx, not a client cap, wins).
	if _, err := newForwarder(50*time.Millisecond).forwardStream(context.Background(), "conv-cut", "hi", nil); err == nil {
		t.Fatal("a reply slower than the turn ctx should fail")
	}
}

// opencodeForwarder must satisfy sessionClearer so /clear gets a real delete.
var _ sessionClearer = (*opencodeForwarder)(nil)

// TestOpencodeForwarderClearDeletesServerSession pins /clear semantics: it issues
// a real DELETE /session/:id on the opencode server and drops the local mapping.
func TestOpencodeForwarderClearDeletesServerSession(t *testing.T) {
	dir := t.TempDir()
	var deleted []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"id":"ses_clear"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_clear/message":
			_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"hi"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/session/ses_clear":
			deleted = append(deleted, r.URL.Path)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := &opencodeForwarder{
		bin:      "opencode",
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(filepath.Join(dir, "s.json")),
		server:   &opencodeServer{baseURL: ts.URL, httpClient: ts.Client()},
	}

	if _, err := f.forwardStream(context.Background(), "conv-1", "hi", nil); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if got := f.sessions.id("conv-1"); got != "ses_clear" {
		t.Fatalf("session = %q, want ses_clear", got)
	}

	if err := f.clearSession(context.Background(), "conv-1"); err != nil {
		t.Fatalf("clearSession: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "/session/ses_clear" {
		t.Fatalf("server deletes = %v, want one DELETE /session/ses_clear", deleted)
	}
	if got := f.sessions.id("conv-1"); got != "" {
		t.Fatalf("session after /clear = %q, want empty", got)
	}
}

// TestOpencodeForwarderResetKeepsServerSession pins /new semantics: resetSession
// only drops the local mapping and must NOT delete the server-side session, so
// the old session stays resumable (the /new vs /clear distinction).
func TestOpencodeForwarderResetKeepsServerSession(t *testing.T) {
	dir := t.TempDir()
	deletes := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/global/health":
			_, _ = w.Write([]byte(`{"healthy":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"id":"ses_keep"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/ses_keep/message":
			_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"hi"}]}`))
		case r.Method == http.MethodDelete:
			deletes++
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	f := &opencodeForwarder{
		bin:      "opencode",
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(filepath.Join(dir, "s.json")),
		server:   &opencodeServer{baseURL: ts.URL, httpClient: ts.Client()},
	}

	if _, err := f.forwardStream(context.Background(), "conv-1", "hi", nil); err != nil {
		t.Fatalf("forward: %v", err)
	}
	f.resetSession("conv-1")
	if deletes != 0 {
		t.Fatalf("resetSession issued %d DELETE(s), want 0 (old session must stay resumable)", deletes)
	}
	if got := f.sessions.id("conv-1"); got != "" {
		t.Fatalf("session after /new = %q, want empty", got)
	}
}

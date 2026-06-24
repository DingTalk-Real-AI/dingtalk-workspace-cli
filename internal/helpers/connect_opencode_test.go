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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOpencodeForwarderCapturesAndResumesSession verifies the capture-based
// session flow: the first turn passes no --session and captures the id from the
// JSON stream (persisting it), and the second turn replays it with --session.
func TestOpencodeForwarderCapturesAndResumesSession(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	storePath := filepath.Join(dir, "opencode-sessions.json")
	opencode := writeShellExecutable(t, dir, "opencode", `
echo "$@" >> "$OPENCODE_STUB_LOG"
sid=ses_stub
printf '%s\n' '{"type":"step_start","sessionID":"'"$sid"'","part":{}}'
printf '%s\n' '{"type":"text","sessionID":"'"$sid"'","part":{"text":"hello"}}'
printf '%s\n' '{"type":"step_finish","sessionID":"'"$sid"'","part":{}}'
`)
	f := &opencodeForwarder{
		bin:      opencode,
		env:      []string{"OPENCODE_STUB_LOG=" + logPath},
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newOpencodeSessions(storePath),
	}

	// First turn: captures and persists ses_stub; reply is the accumulated text.
	reply, err := f.forwardStream(context.Background(), "conv-1", "hi", nil)
	if err != nil {
		t.Fatalf("first forward: %v", err)
	}
	if reply != "hello" {
		t.Fatalf("first reply = %q, want hello", reply)
	}
	if got := f.sessions.id("conv-1"); got != "ses_stub" {
		t.Fatalf("captured session = %q, want ses_stub", got)
	}

	// Second turn: must replay the captured session via --session.
	if _, err := f.forwardStream(context.Background(), "conv-1", "again", nil); err != nil {
		t.Fatalf("second forward: %v", err)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 invocations, got %d:\n%s", len(lines), string(logBytes))
	}
	if strings.Contains(lines[0], "--session") {
		t.Errorf("first invocation should not pass --session, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "--session ses_stub") {
		t.Errorf("second invocation should resume --session ses_stub, got: %s", lines[1])
	}

	// reset drops the mapping: the next turn starts fresh (no --session).
	f.resetSession("conv-1")
	if got := f.sessions.id("conv-1"); got != "" {
		t.Errorf("after reset session = %q, want empty", got)
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

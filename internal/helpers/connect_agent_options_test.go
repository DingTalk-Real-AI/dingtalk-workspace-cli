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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExecStub drops an executable shell stub named name into dir so PATH
// lookups resolve without the real CLI installed.
func writeExecStub(dir, name string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755)
}

// TestConvSessions covers the per-conversation session contract: first message
// creates (--session-id), follow-ups resume (--resume), reset re-mints.
func TestConvSessions(t *testing.T) {
	s := newConvSessions("")

	first := s.args("conv-1")
	if len(first) != 2 || first[0] != "--session-id" || first[1] == "" {
		t.Fatalf("first args = %v, want [--session-id <uuid>]", first)
	}
	second := s.args("conv-1")
	if len(second) != 2 || second[0] != "--resume" || second[1] != first[1] {
		t.Fatalf("second args = %v, want [--resume %s]", second, first[1])
	}

	// A different conversation gets its own session.
	other := s.args("conv-2")
	if other[0] != "--session-id" || other[1] == first[1] {
		t.Fatalf("conv-2 args = %v, want a fresh --session-id distinct from %s", other, first[1])
	}

	// reset self-heals a broken session: next message starts fresh with a NEW
	// uuid (the old one may or may not exist agent-side; a new one is safe).
	s.reset("conv-1")
	again := s.args("conv-1")
	if again[0] != "--session-id" || again[1] == first[1] {
		t.Fatalf("post-reset args = %v, want fresh --session-id != %s", again, first[1])
	}
}

// TestApplyModelArg covers both shapes: replacing an existing model pin
// (claudecode's built-in haiku) and inserting before the tail (gemini-style
// tails that end with -p and need the prompt to stay trailing).
func TestApplyModelArg(t *testing.T) {
	replaced := applyModelArg(
		[]string{"claude", "-p", "--model", "claude-haiku-4-5-20251001", "--strict-mcp-config"},
		"--model", "claude-sonnet-4-6")
	want := []string{"claude", "-p", "--model", "claude-sonnet-4-6", "--strict-mcp-config"}
	if strings.Join(replaced, " ") != strings.Join(want, " ") {
		t.Fatalf("replace: got %v, want %v", replaced, want)
	}

	inserted := applyModelArg([]string{"gemini", "-p"}, "-m", "gemini-2.5-pro")
	wantIns := []string{"gemini", "-m", "gemini-2.5-pro", "-p"}
	if strings.Join(inserted, " ") != strings.Join(wantIns, " ") {
		t.Fatalf("insert: got %v, want %v", inserted, wantIns)
	}
}

// TestForwarderSessionAndModelWiring checks forwarderForChannel applies the
// options: memory only on ccSessions channels, model only via the spec's flag.
func TestForwarderSessionAndModelWiring(t *testing.T) {
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	t.Setenv("DWS_AGENT_CMD", "") // ensure no override
	// Use DWS_AGENT_CMD-free resolution; claudecode requires the binary on
	// PATH, which CI may lack — fake it via DWS_AGENT_CMD is wrong (disables
	// extras by design), so point PATH at a stub.
	stub := t.TempDir()
	for _, name := range []string{"claude", "qodercli"} {
		if err := writeExecStub(stub, name); err != nil {
			t.Fatalf("stub %s: %v", name, err)
		}
	}
	t.Setenv("PATH", stub)

	fwd, err := forwarderForChannel("claudecode", "", connectAgentOptions{Memory: true, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("claudecode forwarder: %v", err)
	}
	ef := fwd.(*execForwarder)
	if ef.sessions == nil {
		t.Fatal("claudecode with Memory=true should have sessions enabled")
	}
	if !strings.Contains(strings.Join(ef.argv, " "), "--model claude-sonnet-4-6") {
		t.Fatalf("model not applied: %v", ef.argv)
	}
	if strings.Contains(strings.Join(ef.argv, " "), "haiku") {
		t.Fatalf("built-in haiku pin should be replaced: %v", ef.argv)
	}

	// qoder family runs a persistent stream-json subprocess and carries the
	// addressable Qoder session id inside each JSON user message. DWS persists
	// the mapping so a connector restart can resume the same conversation.
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	qf, err := forwarderForChannel("qoder", "qoder-client", connectAgentOptions{Memory: true})
	if err != nil {
		t.Fatalf("qoder forwarder: %v", err)
	}
	qsf := qf.(*qoderStreamForwarder)
	if qsf.sessions == nil {
		t.Fatal("qoder with Memory=true should have sessions")
	}
	if qsf.sessions.path == "" {
		t.Fatal("qoder sessions should persist to disk")
	}
	if got := strings.Join(qsf.commandArgs(), " "); !strings.Contains(got, "--input-format stream-json") || !strings.Contains(got, "--output-format stream-json") {
		t.Fatalf("qoder stream-json args mismatch: argv = %v", qsf.commandArgs())
	}

	qwf, err := forwarderForChannel("qoderwork", "robot-client", connectAgentOptions{Memory: true})
	if err != nil {
		t.Fatalf("qoderwork forwarder: %v", err)
	}
	qwsf := qwf.(*qoderStreamForwarder)
	if qwsf.sessions == nil {
		t.Fatal("qoderwork with Memory=true should have sessions")
	}
	if qwsf.sessions.path == "" {
		t.Fatal("qoderwork sessions should persist to disk")
	}

	// Memory off on a supporting channel.
	off, err := forwarderForChannel("claudecode", "", connectAgentOptions{Memory: false})
	if err != nil {
		t.Fatalf("claudecode memory-off forwarder: %v", err)
	}
	if off.(*execForwarder).sessions != nil {
		t.Fatal("Memory=false must disable sessions")
	}
}

// TestRobotConnectAgentFlagsInDryRun checks the new flags surface in the
// dry-run preview so callers can see the effective agent tuning.
func TestRobotConnectAgentFlagsInDryRun(t *testing.T) {
	root := newDevAppTestRoot(&captureRunner{})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "connect",
		"--channel", "claudecode",
		"--robot-client-id", "id1", "--robot-client-secret", "sec1",
		"--agent-model", "claude-sonnet-4-6", "--agent-workdir", "/tmp/kb",
		"--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	for _, sub := range []string{"claude-sonnet-4-6", "/tmp/kb", "per-conversation"} {
		if !strings.Contains(out.String(), sub) {
			t.Fatalf("dry-run output missing %q:\n%s", sub, out.String())
		}
	}

	// memory=false shows as disabled.
	out.Reset()
	root = newDevAppTestRoot(&captureRunner{})
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"dev", "connect",
		"--channel", "claudecode",
		"--robot-client-id", "id1", "--robot-client-secret", "sec1",
		"--agent-memory=false", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"memory": "disabled"`) {
		t.Fatalf("dry-run output missing disabled memory:\n%s", out.String())
	}
}

func TestQoderAgentMemoryPayloadIsPerConversation(t *testing.T) {
	payload := connectAgentOptionsPayload("qoder", connectAgentOptions{Memory: true})
	if got := payload["memory"]; got != "per-conversation-qoder-stream" {
		t.Fatalf("qoder memory = %v, want per-conversation-qoder-stream", got)
	}
	payload = connectAgentOptionsPayload("qoderwork", connectAgentOptions{Memory: true})
	if got := payload["memory"]; got != "per-conversation-qoder-stream" {
		t.Fatalf("qoderwork memory = %v, want per-conversation-qoder-stream", got)
	}
	payload = connectAgentOptionsPayload("qoder", connectAgentOptions{Memory: false})
	if got := payload["memory"]; got != "disabled" {
		t.Fatalf("qoder memory disabled = %v, want disabled", got)
	}
}

// TestRobotConnectDryRunShowsCliStatus checks the dependency preflight agents
// rely on: dry-run reports whether the channel CLI is installed, with the
// install hint when missing.
func TestRobotConnectDryRunShowsCliStatus(t *testing.T) {
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	stub := t.TempDir()
	if err := writeExecStub(stub, "claude"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stub)

	run := func(channel string) string {
		root := newDevAppTestRoot(&captureRunner{})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"dev", "connect", "--channel", channel,
			"--robot-client-id", "a", "--robot-client-secret", "b", "--dry-run"})
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute(%s): %v\n%s", channel, err, out.String())
		}
		return out.String()
	}

	// claude on PATH → installed true with path
	if got := run("claudecode"); !strings.Contains(got, `"installed": true`) {
		t.Fatalf("claudecode should be installed:\n%s", got)
	}
	// codex NOT on PATH → installed false + hint
	got := run("codex")
	if !strings.Contains(got, `"installed": false`) || !strings.Contains(got, "@openai/codex") {
		t.Fatalf("codex should be missing with hint:\n%s", got)
	}
	if !strings.Contains(got, "per-conversation-app-server") {
		t.Fatalf("codex dry-run should advertise app-server memory:\n%s", got)
	}
}

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

func TestExecForwarderRetriesMissingSessionOnce(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	logPath := filepath.Join(dir, "calls.log")
	if err := os.WriteFile(bin, []byte(`#!/bin/sh
echo "$@" >> "$DWS_STALE_LOG"
case " $* " in
  *" --resume "*)
    echo "No conversation found with session ID: stale" >&2
    exit 1
    ;;
esac
echo fresh-ok
`), 0o755); err != nil {
		t.Fatal(err)
	}

	sessions := newConvSessions("")
	if got := sessions.args("conv-1"); len(got) != 2 || got[0] != "--session-id" {
		t.Fatalf("seed session args = %v, want --session-id", got)
	}
	f := &execForwarder{
		name:     "claudecode",
		argv:     []string{bin, "-p"},
		env:      []string{"DWS_STALE_LOG=" + logPath},
		timeout:  5 * time.Second,
		sessions: sessions,
	}

	reply, err := f.forward(context.Background(), "conv-1", "hello")
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if reply != "fresh-ok" {
		t.Fatalf("reply = %q, want fresh-ok", reply)
	}
	calls := readCallLog(t, logPath)
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want stale resume then fresh session", calls)
	}
	if !strings.Contains(calls[0], "--resume") || !strings.Contains(calls[1], "--session-id") {
		t.Fatalf("calls = %v, want --resume then --session-id", calls)
	}
}

func TestExecForwarderStreamRetriesMissingSessionOnce(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent-stream")
	logPath := filepath.Join(dir, "calls.log")
	if err := os.WriteFile(bin, []byte(`#!/bin/sh
echo "$@" >> "$DWS_STALE_LOG"
case " $* " in
  *" --resume "*)
    echo "session not found: stale" >&2
    exit 1
    ;;
esac
printf '%s\n' '{"type":"result","result":"fresh-stream-ok"}'
`), 0o755); err != nil {
		t.Fatal(err)
	}

	sessions := newConvSessions("")
	_ = sessions.args("conv-1")
	f := &execForwarder{
		name:       "claudecode",
		argv:       []string{bin, "-p"},
		streamArgv: []string{bin, "--output-format", "stream-json"},
		env:        []string{"DWS_STALE_LOG=" + logPath},
		parser:     "cc",
		timeout:    5 * time.Second,
		sessions:   sessions,
	}

	reply, err := f.forwardStream(context.Background(), "conv-1", "hello", func(string) {})
	if err != nil {
		t.Fatalf("forwardStream: %v", err)
	}
	if reply != "fresh-stream-ok" {
		t.Fatalf("reply = %q, want fresh-stream-ok", reply)
	}
	calls := readCallLog(t, logPath)
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want stale resume then fresh session", calls)
	}
	if !strings.Contains(calls[0], "--resume") || !strings.Contains(calls[1], "--session-id") {
		t.Fatalf("calls = %v, want --resume then --session-id", calls)
	}
}

func TestExecForwarderDoesNotRetryNonSessionErrors(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	logPath := filepath.Join(dir, "calls.log")
	if err := os.WriteFile(bin, []byte(`#!/bin/sh
echo "$@" >> "$DWS_STALE_LOG"
echo "plain failure" >&2
exit 1
`), 0o755); err != nil {
		t.Fatal(err)
	}

	sessions := newConvSessions("")
	_ = sessions.args("conv-1")
	f := &execForwarder{
		name:     "claudecode",
		argv:     []string{bin, "-p"},
		env:      []string{"DWS_STALE_LOG=" + logPath},
		timeout:  5 * time.Second,
		sessions: sessions,
	}

	if _, err := f.forward(context.Background(), "conv-1", "hello"); err == nil {
		t.Fatal("forward error = nil, want error")
	}
	calls := readCallLog(t, logPath)
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want one non-session failure without retry", calls)
	}
	if !strings.Contains(calls[0], "--resume") {
		t.Fatalf("call = %q, want stale --resume attempt", calls[0])
	}
}

func readCallLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

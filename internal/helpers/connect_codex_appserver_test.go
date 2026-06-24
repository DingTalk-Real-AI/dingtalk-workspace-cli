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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeShellExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", name, err)
	}
	return path
}

func TestForwarderForChannelCodexPrefersAppServer(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_CONNECT_NO_INSTALL", "1")
	stub := t.TempDir()
	writeShellExecutable(t, stub, "codex", "exit 0\n")
	t.Setenv("PATH", stub)

	fwd, err := forwarderForChannel("codex", "", connectAgentOptions{Memory: true})
	if err != nil {
		t.Fatalf("codex forwarder: %v", err)
	}
	cf, ok := fwd.(*codexAppServerForwarder)
	if !ok {
		t.Fatalf("codex should prefer app-server forwarder, got %T", fwd)
	}
	if cf.sessions == nil {
		t.Fatal("codex app-server should keep per-conversation thread memory by default")
	}

	t.Setenv("DWS_CODEX_APP_SERVER", "0")
	fwd, err = forwarderForChannel("codex", "", connectAgentOptions{Memory: true})
	if err != nil {
		t.Fatalf("codex forwarder with deprecated app-server env: %v", err)
	}
	if _, ok := fwd.(*codexAppServerForwarder); !ok {
		t.Fatalf("DWS_CODEX_APP_SERVER=0 should be ignored and keep app-server, got %T", fwd)
	}
}

func TestCodexConnectPlanIgnoresAppServerEnv(t *testing.T) {
	clearChannelEnv(t)
	plan := buildConnectPlan("codex", "cid", "")
	if got := plan["method"]; got != "stream-bridge-codex-app-server" {
		t.Fatalf("default codex plan method = %v", got)
	}
	payload := connectAgentOptionsPayload("codex", connectAgentOptions{Memory: true})
	if got := payload["memory"]; got != "per-conversation-app-server" {
		t.Fatalf("default codex memory = %v", got)
	}

	t.Setenv("DWS_CODEX_APP_SERVER", "0")
	plan = buildConnectPlan("codex", "cid", "")
	if got := plan["method"]; got != "stream-bridge-codex-app-server" {
		t.Fatalf("deprecated app-server env should be ignored, method = %v", got)
	}
	payload = connectAgentOptionsPayload("codex", connectAgentOptions{Memory: true})
	if got := payload["memory"]; got != "per-conversation-app-server" {
		t.Fatalf("deprecated app-server env should keep codex memory, got %v", got)
	}
}

func TestCodexConnectPlanIgnoresAgentCmdOverride(t *testing.T) {
	clearChannelEnv(t)
	t.Setenv("DWS_AGENT_CMD", "my-codex exec")
	plan := buildConnectPlan("codex", "cid", "")
	if got := plan["method"]; got != "stream-bridge-codex-app-server" {
		t.Fatalf("codex should ignore DWS_AGENT_CMD and keep app-server plan, method = %v", got)
	}
	payload := connectAgentOptionsPayload("codex", connectAgentOptions{Memory: true})
	if got := payload["memory"]; got != "per-conversation-app-server" {
		t.Fatalf("codex should ignore DWS_AGENT_CMD and keep app-server memory, got %v", got)
	}
}

func TestCodexAppServerForwarderStreamsAndRemembersThread(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "requests.log")
	codex := writeShellExecutable(t, dir, "codex", `
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$CODEX_STUB_LOG"
  case "$line" in
    *\"method\":\"initialize\"*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *\"method\":\"thread/start\"*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"thr_stub"}}}' ;;
    *\"method\":\"thread/resume\"*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"thr_stub"}}}' ;;
    *\"method\":\"turn/start\"*)
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thr_stub","turnId":"turn_stub","itemId":"item_1","delta":"你"}}'
      printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thr_stub","turnId":"turn_stub","itemId":"item_1","delta":"好"}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thr_stub","turn":{"id":"turn_stub","status":"completed","items":[{"id":"item_1","type":"agentMessage","text":"你好"}]}}}'
      exit 0
      ;;
  esac
done
`)
	fwd := &codexAppServerForwarder{
		bin:      codex,
		env:      []string{"CODEX_STUB_LOG=" + logPath},
		timeout:  5 * time.Second,
		workDir:  dir,
		sessions: newCodexThreadSessions(""),
	}

	var deltas []string
	reply, err := fwd.forwardStream(context.Background(), "conv-1", "第一问", func(s string) {
		deltas = append(deltas, s)
	})
	if err != nil {
		t.Fatalf("first forward: %v", err)
	}
	if reply != "你好" {
		t.Fatalf("first reply = %q, want 你好", reply)
	}
	if strings.Join(deltas, "|") != "你|你好" {
		t.Fatalf("deltas = %v, want [你 你好]", deltas)
	}

	if _, err := fwd.forwardStream(context.Background(), "conv-1", "第二问", nil); err != nil {
		t.Fatalf("second forward: %v", err)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	if strings.Count(log, `"method":"thread/start"`) != 1 {
		t.Fatalf("expected one thread/start, log:\n%s", log)
	}
	if strings.Count(log, `"method":"thread/resume"`) != 1 {
		t.Fatalf("expected one thread/resume, log:\n%s", log)
	}
}

func TestCodexAppServerForwarderReturnsAppServerError(t *testing.T) {
	dir := t.TempDir()
	codex := writeShellExecutable(t, dir, "codex", `
while IFS= read -r line; do
  case "$line" in
    *\"method\":\"initialize\"*)
      printf '%s\n' '{"id":1,"error":{"code":123,"message":"app-server-broken"}}'
      exit 0
      ;;
  esac
done
`)
	fwd := &codexAppServerForwarder{
		bin:     codex,
		timeout: 10 * time.Second,
		workDir: dir,
	}

	reply, err := fwd.forward(context.Background(), "conv-1", "hello")
	if err == nil {
		t.Fatal("forward should return the app-server error instead of falling back to exec")
	}
	if reply != "" {
		t.Fatalf("reply = %q, want empty reply on app-server error", reply)
	}
	if !strings.Contains(err.Error(), "app-server-broken") {
		t.Fatalf("error = %v, want app-server-broken", err)
	}
}

func TestCodexAppServerReadLoopStopsWhenClosedWithFullQueue(t *testing.T) {
	reader, writer := io.Pipe()
	c := &codexAppServerClient{
		msgs:    make(chan codexRPCMessage, 1),
		readErr: make(chan error, 1),
		done:    make(chan struct{}),
	}

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		c.readLoop(reader)
	}()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := 0; i < 200; i++ {
			if _, err := fmt.Fprintf(writer, `{"method":"event/%d","params":{}}`+"\n", i); err != nil {
				return
			}
		}
	}()

	deadline := time.Now().Add(time.Second)
	for len(c.msgs) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(c.msgs) == 0 {
		t.Fatal("readLoop did not enqueue the first message")
	}

	close(c.done)
	_ = reader.Close()
	_ = writer.Close()
	select {
	case <-loopDone:
	case <-time.After(time.Second):
		t.Fatal("readLoop did not stop after client close")
	}
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("writer stayed blocked after pipe close")
	}
}

func TestLockedBufferConcurrentReadWrite(t *testing.T) {
	var b lockedBuffer
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = b.Write([]byte("stderr\n"))
			}
		}
	}()

	for i := 0; i < 1000; i++ {
		_ = b.String()
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("writer did not stop")
	}
}

func TestCodexTurnCompletedText(t *testing.T) {
	final, status, errMsg, ok := codexTurnCompletedText([]byte(`{"threadId":"thr","turn":{"status":"completed","items":[{"type":"agentMessage","text":"早"},{"type":"agentMessage","text":"最终"}]}}`), "thr")
	if !ok || status != "completed" || final != "最终" || errMsg != "" {
		t.Fatalf("completed = (%q,%q,%q,%v)", final, status, errMsg, ok)
	}

	_, status, errMsg, ok = codexTurnCompletedText([]byte(`{"threadId":"thr","turn":{"status":"failed","error":{"message":"boom","additionalDetails":"detail"},"items":[]}}`), "thr")
	if !ok || status != "failed" || errMsg != "boom: detail" {
		t.Fatalf("failed = (%q,%q,%v)", status, errMsg, ok)
	}
}

package helpers

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeStreamingAgent(t *testing.T, body string) string {
	t.Helper()
	return writeShellExecutable(t, t.TempDir(), "agent", body+"\n")
}

func streamingExecForwarder(bin string) *execForwarder {
	return &execForwarder{
		name:       "claudecode",
		argv:       []string{bin},
		streamArgv: []string{bin},
		parser:     "cc",
		timeout:    5 * time.Second,
	}
}

func TestCrossPlatformCoverageExecForwarderStreamingRemainingCoverage(t *testing.T) {
	// Streaming-disabled forwarders use their ordinary one-shot command.
	oneShot := writeStreamingAgent(t, `printf 'one-shot\n'`)
	f := streamingExecForwarder(oneShot)
	f.streamArgv = nil
	if reply, err := f.forwardStream(context.Background(), "", "prompt", func(string) {}); err != nil || reply != "one-shot" {
		t.Fatalf("fallback reply=%q err=%v", reply, err)
	}

	// StdoutPipe failures are surfaced as invocation failures.
	origCommand := connectStreamCommandContext
	connectStreamCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdout = io.Discard
		return cmd
	}
	if _, err := streamingExecForwarder(oneShot).forwardStream(context.Background(), "", "prompt", func(string) {}); err == nil {
		t.Fatal("StdoutPipe failure returned nil")
	}
	connectStreamCommandContext = origCommand
	t.Cleanup(func() { connectStreamCommandContext = origCommand })

	// A missing executable reaches the process-start failure path.
	if _, err := streamingExecForwarder(filepath.Join(t.TempDir(), "missing")).forwardStream(context.Background(), "", "prompt", func(string) {}); err == nil {
		t.Fatal("start failure returned nil")
	}

	// Ignore blank/non-JSON lines, publish deltas, and turn a bare backend
	// error into an actionable reply while clearing the stale session.
	backend := writeStreamingAgent(t, `
printf '\nnoise\n'
printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"partial"}}}'
printf '%s\n' '{"type":"result","result":"API Error: failed"}'`)
	sessions := newConvSessions("")
	oldID := sessions.id("conv")
	f = streamingExecForwarder(backend)
	f.sessions = sessions
	var deltas []string
	reply, err := f.forwardStream(context.Background(), "conv", "prompt", func(s string) { deltas = append(deltas, s) })
	if err != nil || !strings.Contains(reply, "failed") || len(deltas) != 1 || !strings.Contains(deltas[0], "partial") {
		t.Fatalf("backend reply=%q deltas=%v err=%v", reply, deltas, err)
	}
	if sessions.id("conv") == oldID {
		t.Fatal("backend error did not reset session")
	}

	// A successful command without visible events returns the no-text hint.
	empty := writeStreamingAgent(t, `printf 'metadata\n'`)
	if reply, err := streamingExecForwarder(empty).forwardStream(context.Background(), "", "prompt", func(string) {}); err != nil || reply != "（本地 agent 无文本输出）" {
		t.Fatalf("empty reply=%q err=%v", reply, err)
	}

	// A non-session process failure prefers stderr and reports the final error.
	failure := writeStreamingAgent(t, `echo 'plain failure' >&2; exit 1`)
	if _, err := streamingExecForwarder(failure).forwardStream(context.Background(), "", "prompt", func(string) {}); err == nil || !strings.Contains(err.Error(), "plain failure") {
		t.Fatalf("plain failure err=%v", err)
	}

	// A failing remembered session is cleared even when it is not retryable.
	sessions = newConvSessions("")
	oldID = sessions.id("conv")
	f = streamingExecForwarder(failure)
	f.sessions = sessions
	if _, err := f.forwardStream(context.Background(), "conv", "prompt", func(string) {}); err == nil {
		t.Fatal("session failure returned nil")
	}
	if sessions.id("conv") == oldID {
		t.Fatal("terminal stream failure did not reset session")
	}
}

func TestCrossPlatformCoverageParseStreamLineCCInvalidJSON(t *testing.T) {
	if delta, final := parseStreamLine("cc", `{not json`); delta != "" || final != "" {
		t.Fatalf("invalid cc JSON delta=%q final=%q", delta, final)
	}
}

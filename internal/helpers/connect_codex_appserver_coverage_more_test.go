package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type codexNthFailWriter struct {
	writes int
	failAt int
}

func (w *codexNthFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, errors.New("write")
	}
	return len(p), nil
}
func (*codexNthFailWriter) Close() error { return nil }

type codexErrorReader struct{ err error }

func (r codexErrorReader) Read([]byte) (int, error) { return 0, r.err }

func codexIntPtr(v int) *int { return &v }

func unitCodexClient(stdin io.WriteCloser, messages ...codexRPCMessage) *codexAppServerClient {
	msgs := make(chan codexRPCMessage, len(messages)+1)
	for _, msg := range messages {
		msgs <- msg
	}
	return &codexAppServerClient{
		cmd: &exec.Cmd{}, stdin: stdin, msgs: msgs, readErr: make(chan error, 1),
		done: make(chan struct{}), stderr: &lockedBuffer{}, nextID: 1,
	}
}

func TestCrossPlatformCoverageCodexForwardAppServerFailureAndResumeEdges(t *testing.T) {
	originalFactory := codexNewAppServerClient
	defer func() { codexNewAppServerClient = originalFactory }()
	boom := errors.New("factory")
	codexNewAppServerClient = func(context.Context, string, []string, string) (*codexAppServerClient, error) {
		return nil, boom
	}
	if _, err := (&codexAppServerForwarder{bin: "codex"}).forwardAppServer(context.Background(), "conv", "text", nil, nil); !errors.Is(err, boom) {
		t.Fatalf("factory error=%v", err)
	}

	turnDone := json.RawMessage(`{"threadId":"new-thread","turn":{"status":"completed","items":[{"type":"agentMessage","text":"done"}]}}`)
	client := unitCodexClient(&bufferWriteCloser{},
		codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)},
		codexRPCMessage{ID: codexIntPtr(2), Error: &codexRPCError{Message: "resume"}},
		codexRPCMessage{ID: codexIntPtr(3), Result: json.RawMessage(`{"thread":{"id":"new-thread"}}`)},
		codexRPCMessage{Method: "turn/completed", Params: turnDone},
	)
	codexNewAppServerClient = func(context.Context, string, []string, string) (*codexAppServerClient, error) { return client, nil }
	sessions := newCodexThreadSessions("")
	sessions.setThreadID("conv", "old-thread")
	fwd := &codexAppServerForwarder{bin: "codex", sessions: sessions}
	if reply, err := fwd.forwardAppServer(context.Background(), "conv", "text", nil, nil); err != nil || reply != "done" {
		t.Fatalf("resume recovery reply=%q err=%v", reply, err)
	}
	if sessions.threadID("conv") != "new-thread" {
		t.Fatalf("thread=%q", sessions.threadID("conv"))
	}

	for _, tc := range []struct {
		name     string
		messages []codexRPCMessage
	}{
		{"start", []codexRPCMessage{{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)}, {ID: codexIntPtr(2), Error: &codexRPCError{Message: "start"}}}},
		{"turn", []codexRPCMessage{{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)}, {ID: codexIntPtr(2), Result: json.RawMessage(`{"thread":{"id":"thread"}}`)}, {ID: codexIntPtr(3), Error: &codexRPCError{Message: "turn"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := unitCodexClient(&bufferWriteCloser{}, tc.messages...)
			codexNewAppServerClient = func(context.Context, string, []string, string) (*codexAppServerClient, error) { return client, nil }
			if _, err := (&codexAppServerForwarder{bin: "codex"}).forwardAppServer(context.Background(), "conv", "text", nil, nil); err == nil {
				t.Fatal("failure returned nil")
			}
		})
	}
}

func TestCrossPlatformCoverageCodexCwdConstructorAndReadLoopEdges(t *testing.T) {
	originalAbs := codexAbsPath
	originalExec := codexExecCommandContext
	defer func() {
		codexAbsPath = originalAbs
		codexExecCommandContext = originalExec
	}()
	codexAbsPath = func(string) (string, error) { return "", errors.New("abs") }
	if got := (&codexAppServerForwarder{workDir: "relative"}).cwd(); got != "relative" {
		t.Fatalf("cwd=%q", got)
	}
	codexAbsPath = filepath.Abs

	cases := []func() *exec.Cmd{
		func() *exec.Cmd {
			cmd := exec.Command("sh", "-c", "exit 0")
			cmd.Stdin = strings.NewReader("")
			return cmd
		},
		func() *exec.Cmd { cmd := exec.Command("sh", "-c", "exit 0"); cmd.Stdout = io.Discard; return cmd },
		func() *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing")) },
	}
	for _, makeCmd := range cases {
		codexExecCommandContext = func(context.Context, string, ...string) *exec.Cmd { return makeCmd() }
		if client, err := newCodexAppServerClient(context.Background(), "codex", []string{"A=B"}, t.TempDir()); err == nil || client != nil {
			t.Fatalf("constructor client=%v err=%v", client, err)
		}
	}

	c := unitCodexClient(&bufferWriteCloser{})
	c.readLoop(strings.NewReader("\n{\n"))
	if err := <-c.readErr; err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("parse read error=%v", err)
	}
	c = unitCodexClient(&bufferWriteCloser{})
	c.readLoop(codexErrorReader{err: errors.New("scan")})
	if err := <-c.readErr; err == nil || !strings.Contains(err.Error(), "scan") {
		t.Fatalf("scanner error=%v", err)
	}
}

func TestCrossPlatformCoverageCodexClientSendInitializeAndRequestEdges(t *testing.T) {
	if err := unitCodexClient(&codexNthFailWriter{failAt: 1}).initialize(context.Background()); err == nil {
		t.Fatal("initialize first send error returned nil")
	}
	c := unitCodexClient(&codexNthFailWriter{failAt: 2}, codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)})
	if err := c.initialize(context.Background()); err == nil {
		t.Fatal("initialized notification error returned nil")
	}
	if err := unitCodexClient(&codexNthFailWriter{failAt: 1}).send(make(chan int)); err == nil {
		t.Fatal("marshal error returned nil")
	}
	for _, call := range []func(*codexAppServerClient) error{
		func(c *codexAppServerClient) error { _, err := c.startThread(context.Background(), nil); return err },
		func(c *codexAppServerClient) error { _, err := c.resumeThread(context.Background(), nil); return err },
		func(c *codexAppServerClient) error {
			_, err := c.runTurn(context.Background(), "thread", "text", nil, nil)
			return err
		},
	} {
		if err := call(unitCodexClient(&codexNthFailWriter{failAt: 1})); err == nil {
			t.Fatal("request send error returned nil")
		}
	}
}

func TestCrossPlatformCoverageCodexRunTurnRemainingEdges(t *testing.T) {
	serverID := 99
	for _, tc := range []struct {
		name     string
		messages []codexRPCMessage
		close    bool
		want     string
		wantErr  bool
	}{
		{"closed", nil, true, "", true},
		{"server-request-and-turn-error", []codexRPCMessage{{ID: &serverID, Method: "request/input"}, {ID: codexIntPtr(1), Error: &codexRPCError{Message: "turn failed"}}}, false, "", true},
		{"wrong-completion-then-notification", []codexRPCMessage{{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"other","turn":{}}`)}, {Method: "error", Params: json.RawMessage(`{"message":"bad"}`)}}, false, "", true},
		{"failed-without-message", []codexRPCMessage{{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"failed"}}`)}}, false, "", true},
		{"delta-fallback", []codexRPCMessage{{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"threadId":"thread","delta":"partial"}`)}, {Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed"}}`)}}, false, "partial", false},
		{"empty", []codexRPCMessage{{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"thread","turn":{"status":"completed"}}`)}}, false, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := unitCodexClient(&bufferWriteCloser{}, tc.messages...)
			if tc.close {
				close(client.msgs)
			}
			var delta string
			got, err := client.runTurn(context.Background(), "thread", "text", nil, func(s string) { delta = s })
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got=%q delta=%q err=%v", got, delta, err)
			}
		})
	}
}

func TestCrossPlatformCoverageCodexWaitResponseAndNextEdges(t *testing.T) {
	serverID, otherID, wantID := 99, 2, 1
	c := unitCodexClient(&bufferWriteCloser{},
		codexRPCMessage{ID: &serverID, Method: "request/input"},
		codexRPCMessage{},
		codexRPCMessage{ID: &otherID},
		codexRPCMessage{ID: &wantID, Error: &codexRPCError{Message: "response"}},
	)
	if _, err := c.waitResponse(context.Background(), wantID); err == nil {
		t.Fatal("response error returned nil")
	}
	c = unitCodexClient(&bufferWriteCloser{})
	close(c.msgs)
	if _, err := c.waitResponse(context.Background(), wantID); err == nil {
		t.Fatal("closed response returned nil")
	}

	closed := unitCodexClient(&bufferWriteCloser{})
	close(closed.msgs)
	if _, err := closed.next(context.Background()); err == nil {
		t.Fatal("closed buffered channel returned nil")
	}
	delayed := unitCodexClient(&bufferWriteCloser{})
	go func() {
		time.Sleep(time.Millisecond)
		delayed.msgs <- codexRPCMessage{Method: "delayed"}
	}()
	if msg, err := delayed.next(context.Background()); err != nil || msg.Method != "delayed" {
		t.Fatalf("delayed msg=%+v err=%v", msg, err)
	}
	delayedClose := unitCodexClient(&bufferWriteCloser{})
	go func() {
		time.Sleep(time.Millisecond)
		close(delayedClose.msgs)
	}()
	if _, err := delayedClose.next(context.Background()); err == nil {
		t.Fatal("delayed close returned nil")
	}
	readFailure := unitCodexClient(&bufferWriteCloser{})
	readFailure.readErr <- errors.New("read")
	if _, err := readFailure.next(context.Background()); err == nil {
		t.Fatal("read error returned nil")
	}
	done := unitCodexClient(&bufferWriteCloser{})
	close(done.done)
	if _, err := done.next(context.Background()); err == nil {
		t.Fatal("done returned nil")
	}
	cancelled := unitCodexClient(&bufferWriteCloser{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cancelled.next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestCrossPlatformCoverageCodexResultParsingAndDefaultKeyEdges(t *testing.T) {
	if got := codexConvKey("  "); got != "_default" {
		t.Fatalf("default key=%q", got)
	}
	for _, tc := range []struct {
		result json.RawMessage
		err    error
	}{
		{nil, errors.New("upstream")},
		{json.RawMessage(`{`), nil},
		{json.RawMessage(`{}`), nil},
	} {
		if _, err := codexThreadIDFromResult(tc.result, tc.err); err == nil {
			t.Fatalf("result=%s upstream=%v returned nil", tc.result, tc.err)
		}
	}
	if _, _, _, ok := codexTurnCompletedText(json.RawMessage(`{`), "thread"); ok {
		t.Fatal("invalid completed payload accepted")
	}
}

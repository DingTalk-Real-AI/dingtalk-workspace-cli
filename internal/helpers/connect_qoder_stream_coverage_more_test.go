package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type qoderErrorWriteCloser struct{ err error }

func (w qoderErrorWriteCloser) Write([]byte) (int, error) { return 0, w.err }
func (qoderErrorWriteCloser) Close() error                { return nil }

type qoderCloseRecorder struct{ closed bool }

func (*qoderCloseRecorder) Write(p []byte) (int, error) { return len(p), nil }
func (w *qoderCloseRecorder) Close() error              { w.closed = true; return nil }

func activeQoderForwarder(stdin io.WriteCloser) *qoderStreamForwarder {
	return &qoderStreamForwarder{
		name: "qoder", stdin: stdin, cmd: &exec.Cmd{},
		lines: make(chan string, 10), done: make(chan error), stderr: &lockedStringBuffer{},
	}
}

func installImmediateQoderAfter(t *testing.T) {
	t.Helper()
	orig := helperAfter
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() { helperAfter = orig })
}

func TestCrossPlatformCoverageQoderForwardStreamRemainingEdges(t *testing.T) {
	installImmediateQoderAfter(t)
	boom := errors.New("write failed")
	f := activeQoderForwarder(qoderErrorWriteCloser{err: boom})
	if _, err := f.forwardStream(context.Background(), "conv", "text", nil); err == nil {
		t.Fatal("write error returned nil")
	}

	// Control request, assistant delta and empty final fall back to accumulated text.
	stdin := &qoderTestWriteCloser{}
	f = activeQoderForwarder(stdin)
	f.lines <- `{"type":"control_request","request_id":"req","request":{"type":"permission"}}`
	f.lines <- `{"type":"assistant","message":{"content":[{"type":"text","text":"delta"}]}}`
	f.lines <- `{"type":"result","subtype":"success"}`
	var deltas []string
	reply, err := f.forwardStream(context.Background(), "conv", "text", func(delta string) { deltas = append(deltas, delta) })
	if err != nil || !strings.Contains(reply, "delta") || len(deltas) != 1 {
		t.Fatalf("reply=%q deltas=%v err=%v", reply, deltas, err)
	}

	// Backend error resets a remembered conversation.
	sessions := newConvSessions("")
	oldID := sessions.id("conv")
	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.sessions = sessions
	f.lines <- `{"type":"result","subtype":"error","error":"API Error: failed"}`
	if reply, err := f.forwardStream(context.Background(), "conv", "text", nil); err != nil || !strings.Contains(reply, "failed") {
		t.Fatalf("backend reply=%q err=%v", reply, err)
	}
	if sessions.id("conv") == oldID {
		t.Fatal("backend error did not reset session")
	}

	// A completed result with no result text and no deltas returns the no-text hint.
	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.lines <- `{"type":"result","subtype":"success"}`
	if reply, err := f.forwardStream(context.Background(), "conv", "text", nil); err != nil || reply != "（本地 agent 无文本输出）" {
		t.Fatalf("empty reply=%q err=%v", reply, err)
	}

	// Closed output is a read failure and clears remembered state.
	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.sessions = sessions
	close(f.lines)
	if _, err := f.forwardStream(context.Background(), "conv", "text", nil); err == nil {
		t.Fatal("closed output returned nil")
	}
}

func TestCrossPlatformCoverageQoderEnsureLockedRemainingEdges(t *testing.T) {
	origExec := qoderExecCommand
	origAbs := qoderAbsPath
	t.Cleanup(func() {
		qoderExecCommand = origExec
		qoderAbsPath = origAbs
	})
	boom := errors.New("abs")
	qoderAbsPath = func(string) (string, error) { return "", boom }
	if got := (&qoderStreamForwarder{workDir: "relative"}).cwd(); got != "relative" {
		t.Fatalf("cwd fallback=%q", got)
	}
	qoderAbsPath = filepath.Abs
	if got := (&qoderStreamForwarder{workDir: "."}).cwd(); !filepath.IsAbs(got) {
		t.Fatalf("absolute cwd=%q", got)
	}
	if args := (&qoderStreamForwarder{model: "model"}).commandArgs(); !strings.Contains(strings.Join(args, " "), "--model model") {
		t.Fatalf("model args=%v", args)
	}

	active := activeQoderForwarder(&qoderTestWriteCloser{})
	if err := active.ensureLocked(context.Background()); err != nil {
		t.Fatalf("active ensure: %v", err)
	}

	// A finished process is cleared and restart is attempted. Cover error/no-error
	// exits and stderr/no-stderr diagnostics with a deterministic start failure.
	for _, tc := range []struct {
		err    error
		stderr string
	}{{errors.New("exit"), "detail"}, {errors.New("exit"), ""}, {nil, ""}} {
		f := activeQoderForwarder(&qoderTestWriteCloser{})
		f.done = make(chan error, 1)
		f.done <- tc.err
		_, _ = f.stderr.Write([]byte(tc.stderr))
		qoderExecCommand = func(string, ...string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing")) }
		if err := f.ensureLocked(context.Background()); err == nil {
			t.Fatal("restart failure returned nil")
		}
	}

	// StdinPipe and StdoutPipe reject commands whose streams are preconfigured.
	qoderExecCommand = func(string, ...string) *exec.Cmd {
		cmd := exec.Command("sh", "-c", "exit 0")
		cmd.Stdin = strings.NewReader("")
		return cmd
	}
	if err := (&qoderStreamForwarder{name: "qoder"}).ensureLocked(context.Background()); err == nil {
		t.Fatal("stdin pipe error returned nil")
	}
	qoderExecCommand = func(string, ...string) *exec.Cmd {
		cmd := exec.Command("sh", "-c", "exit 0")
		cmd.Stdout = io.Discard
		return cmd
	}
	if err := (&qoderStreamForwarder{name: "qoder"}).ensureLocked(context.Background()); err == nil {
		t.Fatal("stdout pipe error returned nil")
	}
}

func TestCrossPlatformCoverageQoderInitializeAndIOEdges(t *testing.T) {
	installImmediateQoderAfter(t)
	origUUID := qoderUUIDString
	qoderUUIDString = func() string { return "fixed" }
	t.Cleanup(func() { qoderUUIDString = origUUID })
	boom := errors.New("write")
	f := &qoderStreamForwarder{name: "qoder", stdin: qoderErrorWriteCloser{err: boom}}
	if err := f.initializeLocked(context.Background()); err == nil {
		t.Fatal("initialize write error returned nil")
	}

	f = activeQoderForwarder(&qoderTestWriteCloser{})
	close(f.lines)
	if err := f.initializeLocked(context.Background()); err == nil {
		t.Fatal("initialize read error returned nil")
	}

	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.lines <- `{"type":"control_request","request_id":"req","request":{"subtype":"permission"}}`
	f.lines <- `{"type":"control_response","response":{"subtype":"success","request_id":"dws_init_fixed"}}`
	if err := f.initializeLocked(context.Background()); err != nil {
		t.Fatalf("initialize after control request: %v", err)
	}

	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.lines <- `{"type":"control_response","response":{"subtype":"error","request_id":"wrong","error":"ignored"}}`
	// The generated request ID is unknown, so cancel after covering the control and
	// unmatched-response paths.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.initializeLocked(ctx); err == nil {
		t.Fatal("cancelled initialize returned nil")
	}

	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.lines <- `{"type":"control_response","response":{"subtype":"error","request_id":"dws_init_fixed","error":"initialize failed"}}`
	if err := f.initializeLocked(context.Background()); err == nil {
		t.Fatal("initialize response error returned nil")
	}

	if err := (&qoderStreamForwarder{stdin: &qoderTestWriteCloser{}}).writeJSONLocked(make(chan int)); err == nil {
		t.Fatal("marshal error returned nil")
	}

	// readLine skips noise, handles closed output with/without stderr, process
	// exits with/without errors, and context cancellation.
	f = activeQoderForwarder(&qoderTestWriteCloser{})
	f.lines <- ""
	f.lines <- "noise"
	f.lines <- `{"ok":true}`
	if line, err := f.readLineLocked(context.Background()); err != nil || line == "" {
		t.Fatalf("read JSON line=%q err=%v", line, err)
	}
	for _, stderr := range []string{"", "stderr detail"} {
		f = activeQoderForwarder(&qoderTestWriteCloser{})
		_, _ = f.stderr.Write([]byte(stderr))
		close(f.lines)
		if _, err := f.readLineLocked(context.Background()); err == nil {
			t.Fatal("closed lines returned nil")
		}
	}
	for _, processErr := range []error{errors.New("exit"), nil} {
		f = activeQoderForwarder(&qoderTestWriteCloser{})
		f.done = make(chan error, 1)
		f.done <- processErr
		if _, err := f.readLineLocked(context.Background()); err == nil {
			t.Fatal("process exit returned nil")
		}
	}
	f = activeQoderForwarder(&qoderTestWriteCloser{})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.readLineLocked(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestCrossPlatformCoverageQoderControlAndParseRemainingEdges(t *testing.T) {
	f := &qoderStreamForwarder{stdin: &qoderTestWriteCloser{}}
	for _, line := range []string{
		`{`, `{"type":"other"}`, `{"type":"control_request"}`,
	} {
		if f.handleControlRequestLocked(line) {
			t.Fatalf("unexpected handled line %q", line)
		}
	}
	if !f.handleControlRequestLocked(`{"type":"control_request","request_id":"req","request":{"type":"permission"}}`) {
		t.Fatal("type fallback control not handled")
	}

	for _, tc := range []struct {
		line, request string
		matched       bool
		err           bool
	}{
		{`{`, "req", false, false},
		{`{"type":"other"}`, "req", false, false},
		{`{"type":"control_response","response":{"request_id":"other"}}`, "req", false, false},
		{`{"type":"control_response","response":{"request_id":"req","subtype":"error"}}`, "req", true, true},
		{`{"type":"control_response","response":{"request_id":"req","subtype":"error","error":"failed"}}`, "req", true, true},
		{`{"type":"control_response","response":{"request_id":"req","subtype":"success"}}`, "req", true, false},
	} {
		matched, err := qoderControlResponse(tc.line, tc.request)
		if matched != tc.matched || (err != nil) != tc.err {
			t.Fatalf("control(%q)=(%v,%v)", tc.line, matched, err)
		}
	}

	for _, line := range []string{
		`{`,
		`{"type":"assistant","message":{"content":[{"type":"tool","text":"hidden"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"visible"}]}}`,
		`{"type":"result","subtype":"error","error":"failed"}`,
		`{"type":"result","subtype":"error","message":{"content":[{"type":"text","text":"fallback"}]}}`,
		`{"type":"result","subtype":"success","message":{"content":[{"type":"text","text":"done"}]}}`,
		`{"type":"result","subtype":"success","result":"result"}`,
		`{"type":"error","error":"failed"}`,
		`{"type":"error"}`,
		`{"type":"other"}`,
	} {
		_, _, _ = parseQoderPersistentLine(line)
	}
}

func TestCrossPlatformCoverageQoderCloseAndScanEdges(t *testing.T) {
	installImmediateQoderAfter(t)
	closer := &qoderCloseRecorder{}
	f := &qoderStreamForwarder{stdin: closer, done: make(chan error)}
	if err := f.close(); err != nil || !closer.closed {
		t.Fatalf("close err=%v closed=%v", err, closer.closed)
	}
	lines := make(chan string, 2)
	scanQoderLines(bytes.NewBufferString("one\ntwo\n"), lines)
	var got []string
	for line := range lines {
		got = append(got, line)
	}
	if strings.Join(got, ",") != "one,two" {
		t.Fatalf("scanned=%v", got)
	}
}

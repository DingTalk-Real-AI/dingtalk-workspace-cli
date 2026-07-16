package logging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func TestCrossPlatformCoverageLoggerFailureAndRecoveryEdges(t *testing.T) {
	if (*FileLogger)(nil).Writer() != io.Discard || (&FileLogger{}).Writer() != io.Discard {
		t.Fatal("uninitialized Writer should discard")
	}

	dir := t.TempDir()
	logPath := filepath.Join(dir, "direct.log")
	file, err := NewRotatingFile(logPath)
	if err != nil {
		t.Fatalf("NewRotatingFile: %v", err)
	}
	if _, err := file.Write([]byte("entry")); err != nil {
		t.Fatalf("write rotating file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close rotating file: %v", err)
	}
	configured := Setup(t.TempDir())
	if configured.Writer() == io.Discard {
		t.Fatal("configured Writer should expose rotating writer")
	}
	_ = configured.Close()
	if _, err := NewRotatingFile(filepath.Join(dir, "missing", "file")); err == nil {
		t.Fatal("rotating file in missing directory should fail")
	}

	blocked := filepath.Join(t.TempDir(), logSubDir, logFileName)
	if err := os.MkdirAll(blocked, config.DirPerm); err != nil {
		t.Fatalf("create blocking directory: %v", err)
	}
	nop := Setup(filepath.Dir(filepath.Dir(blocked)))
	if nop.writer != nil {
		t.Fatal("Setup with directory at log path should return no-op logger")
	}

	oldOpen := openLogFile
	t.Cleanup(func() { openLogFile = oldOpen })
	wantErr := errors.New("open failed")
	openLogFile = func(string, int, os.FileMode) (*os.File, error) { return nil, wantErr }
	w := newRotatingWriter(logPath, 1, 1)
	if _, err := w.Write([]byte("x")); !errors.Is(err, wantErr) {
		t.Fatalf("reopen error = %v", err)
	}

	closedFile := func() *os.File {
		f, err := os.CreateTemp(t.TempDir(), "closed")
		if err != nil {
			t.Fatalf("create temp file: %v", err)
		}
		_ = f.Close()
		return f
	}
	openLogFile = func(string, int, os.FileMode) (*os.File, error) { return closedFile(), nil }
	if err := newRotatingWriter(logPath, 1, 1).open(); err == nil {
		t.Fatal("open should report stat failure")
	}
	w = newRotatingWriter(logPath, 1, 1)
	if err := w.reopenLocked(); err == nil {
		t.Fatal("reopen should report stat failure")
	}

	openLogFile = oldOpen
	w = newRotatingWriter(logPath, 1, 1)
	if err := w.open(); err != nil {
		t.Fatalf("open before rotation failure: %v", err)
	}
	openLogFile = func(string, int, os.FileMode) (*os.File, error) { return nil, wantErr }
	if _, err := w.Write([]byte("oversized")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("rotation reopen error = %v", err)
	}

	openLogFile = oldOpen
	w = newRotatingWriter(filepath.Join(t.TempDir(), "reopened.log"), 100, 1)
	if _, err := w.Write([]byte("reopened")); err != nil {
		t.Fatalf("successful reopen write: %v", err)
	}
	_ = w.close()

	w = newRotatingWriter(filepath.Join(t.TempDir(), "closed-write.log"), 100, 1)
	if err := w.open(); err != nil {
		t.Fatalf("open for closed write: %v", err)
	}
	_ = w.file.Close()
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("write to externally closed file should fail")
	}
	if err := w.close(); err == nil {
		t.Fatal("close of externally closed file should fail")
	}
	if err := w.close(); err != nil {
		t.Fatalf("second close = %v", err)
	}
}

func TestCrossPlatformCoverageLoggingTransportOptionalAndNilBranches(t *testing.T) {
	LogRequest(nil, "", "", "", 0)
	LogRequestBody(nil, "tools/call", "", "", nil)
	LogResponse(nil, "", "", "", 0, 0, 0, nil)
	LogResponseBody(nil, "", "", 0, nil, "")
	LogRetryAttempt(nil, "", "", 0, 0, 0, 0, nil)
	LogErrorClassified(nil, "", "", "", "", 0, 0, false, "")
	LogCommandStart(nil, "", "", "", "", "", false, 0)
	LogCommandEnd(nil, "", "", "", true, 0, "", "")

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	LogResponseBody(logger, "method", "exec", 200, []byte("ok"), "")
	LogRetryAttempt(logger, "method", "exec", 1, 2, 503, time.Second, nil)
	LogErrorClassified(logger, "method", "exec", "category", "reason", 0, 0, false, "")
	LogErrorClassified(logger, "method", "exec", "category", "reason", 418, -32000, false, "trace")
	LogCommandStart(logger, "exec", "product", "tool", "https://example.test?q=secret", "v1", true, 0)
	LogCommandStart(logger, "exec", "product", "tool", "https://example.test", "v1", true, 30)
	LogCommandEnd(logger, "exec", "product", "tool", true, time.Second, "", "")
	LogCommandEnd(logger, "exec", "product", "tool", false, time.Second, "category", "reason")
	if !strings.Contains(buf.String(), "command_start") || !strings.Contains(buf.String(), "command_end") {
		t.Fatalf("optional log records = %q", buf.String())
	}
}

func TestCrossPlatformCoverageRedactionAndMultiHandlerEdges(t *testing.T) {
	if got := TruncateBody([]byte{0xff, 0xfe, 'a'}, 2); !strings.Contains(got, "truncated") {
		t.Fatalf("invalid UTF-8 truncation = %q", got)
	}
	if got := SanitizeArguments(map[string]any{"unsupported": make(chan int)}, 100); got != "{}" {
		t.Fatalf("marshal failure sanitization = %q", got)
	}
	if RedactHeaders(http.Header{}) != nil {
		t.Fatal("empty headers should return nil")
	}

	wantErr := errors.New("handler failure")
	h := &failingLogHandler{err: wantErr, enabled: true}
	multi := NewMultiHandler(&failingLogHandler{}, h)
	if err := multi.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)); !errors.Is(err, wantErr) {
		t.Fatalf("multi handler error = %v", err)
	}
	if !multi.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("multi handler should be enabled")
	}
	if NewMultiHandler(&failingLogHandler{}).Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("disabled multi handler should not be enabled")
	}
}

type failingLogHandler struct {
	err     error
	enabled bool
}

func (h *failingLogHandler) Enabled(context.Context, slog.Level) bool { return h.enabled }
func (h *failingLogHandler) Handle(context.Context, slog.Record) error {
	return h.err
}
func (h *failingLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *failingLogHandler) WithGroup(string) slog.Handler      { return h }

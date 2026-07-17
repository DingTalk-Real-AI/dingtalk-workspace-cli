package app

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
)

type auditCoverageSink struct {
	events   []*audit.Event
	emitErr  error
	closeErr error
}

func (sink *auditCoverageSink) Emit(event *audit.Event) error {
	sink.events = append(sink.events, event)
	return sink.emitErr
}

func (sink *auditCoverageSink) Close() error { return sink.closeErr }

func TestCrossPlatformCoverageAuditCommandsAndFileHelpersCoverage(t *testing.T) {
	originalExit, originalVerify := auditExit, auditVerify
	t.Cleanup(func() { auditExit, auditVerify = originalExit, originalVerify })
	dir := t.TempDir()
	t.Setenv(audit.EnvAuditDir, dir)
	if auditDir() != dir {
		t.Fatalf("auditDir() = %q", auditDir())
	}

	tail := newAuditTailCommand()
	tail.SetArgs([]string{"--lines", "1"})
	if err := tail.Execute(); err == nil || !strings.Contains(err.Error(), "无审计记录") {
		t.Fatalf("audit tail(empty) error = %v", err)
	}
	path := filepath.Join(dir, "audit-20260101.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tail = newAuditTailCommand()
	tail.SetArgs([]string{"--lines", "1"})
	if err := tail.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := tailFile(filepath.Join(dir, "missing"), 1); err == nil {
		t.Fatal("tailFile(missing) error = nil")
	}
	tailErrorDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tailErrorDir, "audit-20260101.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(audit.EnvAuditDir, tailErrorDir)
	tail = newAuditTailCommand()
	if err := tail.Execute(); err == nil {
		t.Fatal("audit tail(directory record) error = nil")
	}
	oversize := filepath.Join(dir, "oversize")
	if err := os.WriteFile(oversize, []byte(strings.Repeat("x", 2*1024*1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := tailFile(oversize, 1); err == nil {
		t.Fatal("tailFile(oversize) error = nil")
	}

	exportDir := t.TempDir()
	t.Setenv(audit.EnvAuditDir, exportDir)
	export := newAuditExportCommand()
	if err := export.Execute(); err == nil || !strings.Contains(err.Error(), "无审计文件") {
		t.Fatalf("audit export(empty) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(exportDir, "audit-20260102.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"jsonl", "csv"} {
		export = newAuditExportCommand()
		export.SetArgs([]string{"--since", "2026-01-01", "--until", "2026-01-03", "--format", format})
		if err := export.Execute(); err != nil {
			t.Fatalf("audit export(%s) error = %v", format, err)
		}
	}
	export = newAuditExportCommand()
	export.SetArgs([]string{"--format", "xml"})
	if err := export.Execute(); err == nil || !strings.Contains(err.Error(), "不支持的格式") {
		t.Fatalf("audit export(xml) error = %v", err)
	}
	t.Setenv(audit.EnvAuditDir, filepath.Join(exportDir, "missing"))
	export = newAuditExportCommand()
	if err := export.Execute(); err == nil || !strings.Contains(err.Error(), "查找审计文件失败") {
		t.Fatalf("audit export(missing dir) error = %v", err)
	}

	if err := exportJSONL([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("exportJSONL(missing) error = nil")
	}
	if err := exportJSONL([]string{oversize}); err == nil {
		t.Fatal("exportJSONL(oversize) error = nil")
	}
	if err := exportCSV([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatal("exportCSV(missing) error = nil")
	}
	blank := filepath.Join(dir, "blank")
	if err := os.WriteFile(blank, []byte("\n \n{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exportCSV([]string{blank}); err != nil {
		t.Fatal(err)
	}
	if err := exportCSV([]string{oversize}); err == nil {
		t.Fatal("exportCSV(oversize) error = nil")
	}

	originalWrite, originalFlush, originalError := auditCSVWrite, auditCSVFlush, auditCSVError
	t.Cleanup(func() { auditCSVWrite, auditCSVFlush, auditCSVError = originalWrite, originalFlush, originalError })
	auditCSVWrite = func(*csv.Writer, []string) error { return errors.New("write") }
	if err := exportCSV(nil); err == nil || !strings.Contains(err.Error(), "表头") {
		t.Fatalf("exportCSV(header error) = %v", err)
	}
	calls := 0
	auditCSVWrite = func(writer *csv.Writer, row []string) error {
		calls++
		if calls > 1 {
			return errors.New("row")
		}
		return originalWrite(writer, row)
	}
	if err := exportCSV([]string{blank}); err == nil || !strings.Contains(err.Error(), "记录") {
		t.Fatalf("exportCSV(row error) = %v", err)
	}
	auditCSVWrite = originalWrite
	auditCSVError = func(*csv.Writer) error { return errors.New("flush") }
	if err := exportCSV(nil); err == nil || !strings.Contains(err.Error(), "刷新") {
		t.Fatalf("exportCSV(flush error) = %v", err)
	}

	t.Setenv(audit.EnvAuditDir, t.TempDir())
	verify := newAuditVerifyCommand()
	if err := verify.Execute(); err == nil || !strings.Contains(err.Error(), "无审计文件") {
		t.Fatalf("audit verify(empty) error = %v", err)
	}
	verify = newAuditVerifyCommand()
	verify.SetArgs([]string{"--file", filepath.Join(dir, "missing")})
	if err := verify.Execute(); err == nil || !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("audit verify(missing) error = %v", err)
	}
	validDir := t.TempDir()
	writer, err := audit.NewDateRotatingWriter(validDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	sink := audit.NewFileSink(writer, audit.NewChain(validDir), nil)
	if err := sink.Emit(&audit.Event{Timestamp: time.Now(), Product: "test", Command: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	validFile, err := audit.LatestAuditFile(validDir)
	if err != nil {
		t.Fatal(err)
	}
	verify = newAuditVerifyCommand()
	verify.SetArgs([]string{"--file", validFile})
	if err := verify.Execute(); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(t.TempDir(), "audit-broken.jsonl")
	if err := os.WriteFile(broken, []byte(`{"prev_hash":"wrong","hash":"wrong"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	auditExit = func(code int) { exitCode = code }
	auditVerify = func(string) (bool, int, error) { return false, 1, nil }
	verify = newAuditVerifyCommand()
	verify.SetArgs([]string{"--file", broken})
	if err := verify.Execute(); err != nil || exitCode != 1 {
		t.Fatalf("audit verify(broken) = %v, exit=%d", err, exitCode)
	}
	t.Setenv(audit.EnvAuditDir, "")
	if auditDir() == "" {
		t.Fatal("default auditDir() is empty")
	}
}

func TestCrossPlatformCoverageAuditRuntimeCoverage(t *testing.T) {
	previousSink, previousLoader := sharedAuditSink, loadTokenForProfile
	t.Cleanup(func() {
		sharedAuditSink = previousSink
		loadTokenForProfile = previousLoader
		auditSinkOnce, auditCloseOnce = sync.Once{}, sync.Once{}
		resetAuditIdentityCache()
	})

	sharedAuditSink = nil
	auditCloseOnce = sync.Once{}
	CloseAuditSink()
	failedClose := &auditCoverageSink{closeErr: errors.New("close")}
	sharedAuditSink = failedClose
	auditCloseOnce = sync.Once{}
	CloseAuditSink()

	bad := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(audit.EnvAudit, "1")
	t.Setenv(audit.EnvAuditDir, filepath.Join(bad, "child"))
	auditSinkOnce = sync.Once{}
	sharedAuditSink = nil
	if _, ok := setupAuditSink().(audit.NopSink); !ok {
		t.Fatalf("setupAuditSink(error) = %T", sharedAuditSink)
	}

	t.Setenv(audit.EnvAuditDebug, "1")
	fileLogger = logging.Setup(t.TempDir())
	t.Cleanup(func() {
		if fileLogger != nil {
			fileLogger.Close()
			fileLogger = nil
		}
	})
	auditReport("coverage %d", 1)
	loadTokenForProfile = func(string, string) (*auth.TokenData, error) { return nil, errors.New("identity") }
	resetAuditIdentityCache()
	if actor, _ := auditIdentity(); actor.UserID != "" {
		t.Fatalf("auditIdentity(error) = %+v", actor)
	}

	invocation := executor.Invocation{CanonicalProduct: "calendar", Tool: "list", Params: map[string]any{"token": "secret"}}
	emitAudit(nil, "nil", time.Now(), invocation, "https://example.com?token=secret", nil, "test")
	emitAudit(audit.NopSink{}, "nop", time.Now(), invocation, "", nil, "test")
	recording := &auditCoverageSink{}
	emitAudit(recording, "ok", time.Now(), invocation, "https://example.com?token=secret", nil, "test")
	if len(recording.events) != 1 || recording.events[0].Result != "success" {
		t.Fatalf("successful audit events = %#v", recording.events)
	}
	typed := &apperrors.Error{Category: apperrors.CategoryAuth, Reason: "expired"}
	emitAudit(recording, "typed", time.Now(), invocation, "", typed, "test")
	if recording.events[1].ErrReason != "expired" {
		t.Fatalf("typed audit event = %#v", recording.events[1])
	}
	recording.emitErr = errors.New("emit")
	emitAudit(recording, "failed", time.Now(), invocation, "", errors.New("plain"), "test")
	if category, reason := classifyAuditError(nil); category != "" || reason != "" {
		t.Fatalf("classifyAuditError(nil) = %q, %q", category, reason)
	}
	if category, reason := classifyAuditError(fmt.Errorf("wrapped: %w", typed)); category != string(apperrors.CategoryAuth) || reason != "expired" {
		t.Fatalf("classifyAuditError(typed) = %q, %q", category, reason)
	}
	if category, reason := classifyAuditError(errors.New("plain")); category != "unknown" || reason != "plain" {
		t.Fatalf("classifyAuditError(plain) = %q, %q", category, reason)
	}
}

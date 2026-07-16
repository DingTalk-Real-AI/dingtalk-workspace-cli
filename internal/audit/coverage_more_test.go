package audit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageChainErrorAndLargeRecordCoverage(t *testing.T) {
	dir := t.TempDir()
	closed, err := os.Create(filepath.Join(dir, "closed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewChain(dir).SealFromFile(closed, []byte("{}")); err == nil {
		t.Fatal("SealFromFile on a closed file succeeded")
	}

	newFile := func(name, content string) *os.File {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		return file
	}
	if hash, err := lastRecordHash(newFile("newlines", "\n\r\n")); err != nil || hash != "" {
		t.Fatalf("lastRecordHash(newlines) = %q, %v", hash, err)
	}
	if _, err := lastRecordHash(newFile("invalid", "not-json\n")); err == nil {
		t.Fatal("lastRecordHash(invalid) error = nil")
	}
	large := `{"padding":"` + strings.Repeat("x", 70*1024) + `","hash":"large-hash"}` + "\n"
	if hash, err := lastRecordHash(newFile("large", large)); err != nil || hash != "large-hash" {
		t.Fatalf("lastRecordHash(large) = %q, %v", hash, err)
	}
	if hash, err := lastRecordHashFullScan(newFile("scan-empty", "\n\n")); err != nil || hash != "" {
		t.Fatalf("lastRecordHashFullScan(empty) = %q, %v", hash, err)
	}
	if _, err := lastRecordHashFullScan(newFile("scan-invalid", "not-json\n")); err == nil {
		t.Fatal("lastRecordHashFullScan(invalid) error = nil")
	}
	tooLarge := strings.Repeat("x", 9*1024*1024)
	if _, err := lastRecordHashFullScan(newFile("scan-too-large", tooLarge)); err == nil {
		t.Fatal("lastRecordHashFullScan(oversize) error = nil")
	}

	if _, _, err := VerifyFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("VerifyFile(missing) error = nil")
	}
	if valid, line, err := VerifyFile(filepath.Join(dir, "invalid")); err == nil || valid || line != 1 {
		t.Fatalf("VerifyFile(invalid) = %v, %d, %v", valid, line, err)
	}
	prevMismatch := filepath.Join(dir, "prev-mismatch")
	if err := os.WriteFile(prevMismatch, []byte(`{"prev_hash":"wrong","hash":"wrong"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if valid, line, err := VerifyFile(prevMismatch); err == nil || valid || line != 1 {
		t.Fatalf("VerifyFile(prev mismatch) = %v, %d, %v", valid, line, err)
	}
	oversize := filepath.Join(dir, "verify-too-large")
	if err := os.WriteFile(oversize, []byte(tooLarge), 0o600); err != nil {
		t.Fatal(err)
	}
	if valid, _, err := VerifyFile(oversize); err == nil || valid {
		t.Fatalf("VerifyFile(oversize) = %v, %v", valid, err)
	}
	invalidBody := []byte("not-json")
	if got := stripHashFields(invalidBody); string(got) != string(invalidBody) {
		t.Fatalf("stripHashFields(invalid) = %q", got)
	}
}

func TestCrossPlatformCoverageCollectionAndRotationCoverage(t *testing.T) {
	for _, value := range []string{"1", "true", "on", "yes", "y"} {
		t.Setenv(EnvAuditDebug, value)
		if !DebugEnabled() {
			t.Fatalf("DebugEnabled(%q) = false", value)
		}
	}
	t.Setenv(EnvAuditDebug, "off")
	if DebugEnabled() {
		t.Fatal("DebugEnabled(off) = true")
	}
	for _, value := range []string{"0", "false", "off", "no", "n"} {
		t.Setenv(EnvAudit, value)
		if IsEnabled() {
			t.Fatalf("IsEnabled(%q) = true", value)
		}
	}
	t.Setenv(EnvAudit, "unexpected")
	if !IsEnabled() {
		t.Fatal("IsEnabled(unexpected) = false")
	}
	t.Setenv(EnvAudit, "0")
	sink, err := BuildSink(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sink.(NopSink); !ok {
		t.Fatalf("disabled BuildSink() = %T", sink)
	}

	t.Setenv(EnvAudit, "1")
	t.Setenv(EnvRetentionDays, "invalid")
	t.Setenv(EnvForwardURL, "http://127.0.0.1:1")
	t.Setenv(EnvForwardToken, "secret")
	t.Setenv(EnvForwardRedact, "unexpected")
	sink, err = BuildSink(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRetentionDays, "0")
	t.Setenv(EnvForwardRedact, string(RedactHashed))
	sink, err = BuildSink(t.TempDir(), func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	badConfig := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badConfig, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAuditDir, filepath.Join(badConfig, "child"))
	if _, err := BuildSink(badConfig, nil); err == nil {
		t.Fatal("BuildSink(unwritable path) error = nil")
	}

	if _, err := NewDateRotatingWriter(filepath.Join(badConfig, "child"), 1); err == nil {
		t.Fatal("NewDateRotatingWriter(invalid dir) error = nil")
	}
	lockDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(lockDir, auditLockFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDateRotatingWriter(lockDir, 1); err == nil {
		t.Fatal("NewDateRotatingWriter(lock directory) error = nil")
	}

	dir := t.TempDir()
	w, err := NewDateRotatingWriter(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if w.Dir() != dir {
		t.Fatalf("Dir() = %q", w.Dir())
	}
	w.pruneOldFiles()
	file, release, err := w.beginAppend()
	if err != nil {
		t.Fatal(err)
	}
	release()
	w.curDate = "old"
	if _, release, err = w.beginAppend(); err != nil {
		t.Fatal(err)
	}
	release()
	if file == nil {
		t.Fatal("beginAppend returned nil file")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	openFailureDir := t.TempDir()
	openFailureWriter, err := NewDateRotatingWriter(openFailureDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(openFailureDir, "audit-"+time.Now().Format("20060102")+".jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openFailureWriter.beginAppend(); err == nil {
		t.Fatal("beginAppend(open failure) error = nil")
	}
	if err := openFailureWriter.Close(); err != nil {
		t.Fatal(err)
	}
	fileErrorWriter, err := NewDateRotatingWriter(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	fileErrorWriter.file, err = os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileErrorWriter.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fileErrorWriter.Close(); err == nil {
		t.Fatal("Close(closed data file) error = nil")
	}
	lockErrorWriter, err := NewDateRotatingWriter(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockErrorWriter.lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lockErrorWriter.Close(); err == nil {
		t.Fatal("Close(closed lock file) error = nil")
	}
	sinkCloseWriter, err := NewDateRotatingWriter(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := sinkCloseWriter.lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := NewFileSink(sinkCloseWriter, NewChain(""), nil).Close(); err == nil {
		t.Fatal("FileSink.Close(closed writer) error = nil")
	}
	(&DateRotatingWriter{dir: filepath.Join(t.TempDir(), "missing"), retention: 1}).pruneOldFiles()
	pruneDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pruneDir, "audit-bad.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	(&DateRotatingWriter{dir: pruneDir, retention: 1}).pruneOldFiles()

	if _, err := LatestAuditFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("LatestAuditFile(missing) error = nil")
	}
	empty := t.TempDir()
	if _, err := LatestAuditFile(empty); err == nil {
		t.Fatal("LatestAuditFile(empty) error = nil")
	}
	if _, err := AuditFilesInRange(filepath.Join(empty, "missing"), "", ""); err == nil {
		t.Fatal("AuditFilesInRange(missing) error = nil")
	}
	for name := range map[string]bool{
		"audit-20260101.jsonl": true,
		"audit-20260102.jsonl": true,
		"audit-bad.jsonl":      true,
		"other":                true,
	} {
		if err := os.WriteFile(filepath.Join(empty, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := AuditFilesInRange(empty, "20260102", "20260103")
	if err != nil || len(files) != 1 || !strings.HasSuffix(files[0], "20260102.jsonl") {
		t.Fatalf("AuditFilesInRange() = %v, %v", files, err)
	}
}

func TestCrossPlatformCoverageSinkAndForwarderFailureCoverage(t *testing.T) {
	originalMarshal, originalWrite := sinkMarshal, sinkWrite
	originalForwardMarshal, originalRedact := forwardMarshal, forwardRedactEventJSON
	originalReadAt, originalSeek, originalChainMarshal := chainReadAt, chainSeek, chainMarshal
	originalLock, originalLockTimeout := rotateLockFile, rotateLockTimeout
	t.Cleanup(func() {
		sinkMarshal, sinkWrite = originalMarshal, originalWrite
		forwardMarshal, forwardRedactEventJSON = originalForwardMarshal, originalRedact
		chainReadAt, chainSeek, chainMarshal = originalReadAt, originalSeek, originalChainMarshal
		rotateLockFile, rotateLockTimeout = originalLock, originalLockTimeout
	})
	chainReadAt = func(*os.File, []byte, int64) (int, error) { return 0, errors.New("read") }
	readFailure, err := os.CreateTemp(t.TempDir(), "read-failure")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readFailure.Close() })
	if _, err := readFailure.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := lastRecordHash(readFailure); err == nil {
		t.Fatal("lastRecordHash(read failure) error = nil")
	}
	chainReadAt = originalReadAt
	chainSeek = func(*os.File, int64, int) (int64, error) { return 0, errors.New("seek") }
	if _, err := lastRecordHashFullScan(readFailure); err == nil {
		t.Fatal("lastRecordHashFullScan(seek failure) error = nil")
	}
	chainSeek = originalSeek
	chainMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	validEvent, err := originalMarshal(Event{})
	if err != nil {
		t.Fatal(err)
	}
	if got := stripHashFields(validEvent); string(got) != string(validEvent) {
		t.Fatalf("stripHashFields(marshal failure) = %q", got)
	}
	chainMarshal = originalChainMarshal

	writer, err := NewDateRotatingWriter(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	sink := NewFileSink(writer, NewChain(""), nil)
	sinkMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	if err := sink.Emit(&Event{}); err == nil || !strings.Contains(err.Error(), "marshal event") {
		t.Fatalf("Emit(first marshal) error = %v", err)
	}
	calls := 0
	sinkMarshal = func(value any) ([]byte, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("final marshal")
		}
		return originalMarshal(value)
	}
	if err := sink.Emit(&Event{}); err == nil || !strings.Contains(err.Error(), "marshal final event") {
		t.Fatalf("Emit(final marshal) error = %v", err)
	}
	sinkMarshal = originalMarshal
	sinkWrite = func(*os.File, []byte) (int, error) { return 0, errors.New("write") }
	if err := sink.Emit(&Event{}); err == nil || !strings.Contains(err.Error(), "write event") {
		t.Fatalf("Emit(write) error = %v", err)
	}
	sinkWrite = originalWrite
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	rotateLockFile = func(*os.File) error { return errors.New("locked") }
	rotateLockTimeout = 0
	blockedWriter, err := NewDateRotatingWriter(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	blockedSink := NewFileSink(blockedWriter, NewChain(""), nil)
	if err := blockedSink.Emit(&Event{}); err == nil || !strings.Contains(err.Error(), "acquire writer") {
		t.Fatalf("Emit(lock failure) error = %v", err)
	}
	rotateLockFile, rotateLockTimeout = originalLock, originalLockTimeout
	if err := blockedWriter.Close(); err != nil {
		t.Fatal(err)
	}

	sealDir := t.TempDir()
	sealWriter, err := NewDateRotatingWriter(sealDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	todayFile := filepath.Join(sealDir, "audit-"+time.Now().Format("20060102")+".jsonl")
	if err := os.WriteFile(todayFile, []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewFileSink(sealWriter, NewChain(""), nil).Emit(&Event{}); err == nil || !strings.Contains(err.Error(), "seal event") {
		t.Fatalf("Emit(seal failure) error = %v", err)
	}
	if err := sealWriter.Close(); err != nil {
		t.Fatal(err)
	}

	forwardServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	forwarding := NewHTTPForwarder(forwardServer.URL, "", RedactNone, nil)
	forwardWriter, err := NewDateRotatingWriter(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	forwardSink := NewFileSink(forwardWriter, NewChain(""), forwarding)
	if err := forwardSink.Emit(&Event{}); err != nil {
		t.Fatal(err)
	}
	if err := forwardSink.Close(); err != nil {
		t.Fatal(err)
	}
	forwardServer.Close()

	var reports []string
	report := func(format string, args ...any) { reports = append(reports, fmt.Sprintf(format, args...)) }
	forwarder := NewHTTPForwarder(":", "", RedactNone, report)
	forwarder.send(Event{})
	forwardMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	forwarder.send(Event{})
	forwardMarshal = originalForwardMarshal
	forwardRedactEventJSON = func(Event, RedactLevel) ([]byte, error) { return nil, errors.New("redact") }
	forwarder.redact = RedactHashed
	forwarder.send(Event{})
	forwardRedactEventJSON = originalRedact

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	forwarder = NewHTTPForwarder(server.URL, "token", RedactMinimal, report)
	forwarder.send(Event{Actor: Actor{Name: "name"}})
	forwarder = NewHTTPForwarder("http://127.0.0.1:1", "", RedactNone, report)
	forwarder.timeout = 20 * time.Millisecond
	forwarder.send(Event{})
	if err := forwarder.Close(nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	forwarder.wg.Add(1)
	if err := forwarder.Close(ctx); err == nil {
		t.Fatal("Close(cancelled) error = nil")
	}
	forwarder.wg.Done()
	if len(reports) < 5 {
		t.Fatalf("forward reports = %v", reports)
	}
	if _, err := RedactEventJSON(Event{Actor: Actor{Name: "name"}}, RedactHashed); err != nil {
		t.Fatal(err)
	}
}

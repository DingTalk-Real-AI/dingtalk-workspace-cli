package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCrossPlatformCoverageConnectHealthRemainingIOAndListingBranches(t *testing.T) {
	origOverride := connectDaemonDirOverride
	origReadDir := connectHealthReadDir
	t.Cleanup(func() {
		connectDaemonDirOverride = origOverride
		connectHealthReadDir = origReadDir
	})

	blockingFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	connectDaemonDirOverride = blockingFile
	if h := newConnectHealth("client", "codex"); h != nil {
		t.Fatalf("health with invalid base=%+v", h)
	}
	connectHealthReadDir = func(string) ([]os.DirEntry, error) {
		return nil, errors.New("read directory")
	}
	if _, err := listConnectors(time.Now()); err == nil {
		t.Fatal("list against non-directory base returned nil")
	}
	connectHealthReadDir = origReadDir

	h := &connectHealth{dir: filepath.Join(t.TempDir(), "missing"), hb: connectHeartbeat{UpdatedUnix: 1}}
	h.onError(nil)
	if err := h.flush(); err == nil {
		t.Fatal("heartbeat write failure returned nil")
	}

	renameDir := t.TempDir()
	if err := os.Mkdir(connectHeartbeatPath(renameDir), 0o700); err != nil {
		t.Fatal(err)
	}
	h = &connectHealth{dir: renameDir, hb: connectHeartbeat{UpdatedUnix: 1}}
	if err := h.flush(); err == nil {
		t.Fatal("heartbeat rename failure returned nil")
	}
	if _, err := readConnectHeartbeat(renameDir); err == nil {
		t.Fatal("heartbeat read I/O failure returned nil")
	}

	root := t.TempDir()
	connectDaemonDirOverride = root
	base := connectBaseDir()
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "not-a-connector"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(base, "bad-heartbeat")
	if err := os.MkdirAll(connectHeartbeatPath(badDir), 0o700); err != nil {
		t.Fatal(err)
	}
	daemonDir := filepath.Join(base, "daemon-only")
	if err := os.MkdirAll(daemonDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeDaemonState(daemonDir, daemonState{Pid: os.Getpid(), UnifiedAppID: "app-id"}); err != nil {
		t.Fatal(err)
	}
	reports, err := listConnectors(time.Now())
	if err != nil || len(reports) != 1 || reports[0].ClientID != "daemon-only" || reports[0].UnifiedAppID != "app-id" {
		t.Fatalf("reports=%+v err=%v", reports, err)
	}

	dead := &connectHeartbeat{Pid: -1}
	if got := deriveConnectHealth(dead, true, time.Now()); got.Detail != "connector process not alive; supervisor should restart it" {
		t.Fatalf("supervised dead detail=%q", got.Detail)
	}

	connectDaemonDirOverride = ""
	if got := connectBaseDir(); got == "" {
		t.Fatal("default connect base is empty")
	}
}

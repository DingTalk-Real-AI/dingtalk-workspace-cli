package busctl

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

var errBusctlInjected = errors.New("injected busctl failure")

func TestCrossPlatformCoverageDiscoverValidationAndFailures(t *testing.T) {
	for name, cfg := range map[string]DiscoverConfig{
		"workdir":  {IPCEndpoint: "endpoint", ClientID: "id"},
		"endpoint": {WorkDir: t.TempDir(), ClientID: "id"},
		"client":   {WorkDir: t.TempDir(), IPCEndpoint: "endpoint"},
	} {
		if _, err := Discover(cfg); err == nil {
			t.Fatalf("%s validation error expected", name)
		}
	}

	origDial := discoverDial
	origMkdir := discoverMkdirAll
	origSpawn := discoverSpawn
	t.Cleanup(func() {
		discoverDial = origDial
		discoverMkdirAll = origMkdir
		discoverSpawn = origSpawn
	})
	discoverDial = func(string) (net.Conn, error) { return nil, errBusctlInjected }
	discoverMkdirAll = func(string, os.FileMode) error { return errBusctlInjected }
	if _, err := Discover(DiscoverConfig{WorkDir: t.TempDir(), IPCEndpoint: "x", ClientID: "id"}); err == nil {
		t.Fatal("mkdir error expected")
	}
	discoverMkdirAll = func(string, os.FileMode) error { return nil }
	discoverSpawn = func(SpawnConfig) (int, error) { return 0, ErrSpawnFailed }
	if _, err := Discover(DiscoverConfig{
		WorkDir: t.TempDir(), IPCEndpoint: "x", ClientID: "id",
		DialDeadline: time.Millisecond, DialBackoff: time.Millisecond, DialMaxBackoff: time.Nanosecond,
	}); err == nil {
		t.Fatal("deadline error expected")
	}
	if got := MetaPath("dir"); got != filepath.Join("dir", "bus.meta") {
		t.Fatalf("MetaPath = %q", got)
	}
}

func TestCrossPlatformCoverageWaitReadyRejectsShortSuccessfulRead(t *testing.T) {
	original := busctlReadFull
	t.Cleanup(func() { busctlReadFull = original })
	busctlReadFull = func(io.Reader, []byte) (int, error) { return 0, nil }
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	if err := waitReady(read); !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("waitReady(short read) error = %v", err)
	}
}

func TestCrossPlatformCoverageSpawnAndReadyFailureEdges(t *testing.T) {
	origExecutable := spawnExecutable
	origPipe := spawnPipe
	origTimeout := ReadyTimeout
	t.Cleanup(func() {
		spawnExecutable = origExecutable
		spawnPipe = origPipe
		ReadyTimeout = origTimeout
	})
	if _, err := Spawn(SpawnConfig{}); err == nil {
		t.Fatal("client ID validation expected")
	}
	spawnExecutable = func() (string, error) { return "", errBusctlInjected }
	if _, err := Spawn(SpawnConfig{ClientID: "id"}); err == nil {
		t.Fatal("executable error expected")
	}
	spawnExecutable = origExecutable
	spawnPipe = func() (*os.File, *os.File, error) { return nil, nil, errBusctlInjected }
	if _, err := Spawn(SpawnConfig{ClientID: "id"}); err == nil {
		t.Fatal("pipe error expected")
	}
	spawnPipe = origPipe
	if _, err := Spawn(SpawnConfig{ClientID: "id", ExecPath: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("start error expected")
	}

	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"ready", []byte{'R'}, nil},
		{"failed", []byte{'E'}, ErrSpawnFailed},
		{"unexpected", []byte{'X'}, errBusctlInjected},
		{"eof", nil, ErrSpawnFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if len(tc.data) > 0 {
				_, _ = pw.Write(tc.data)
			}
			_ = pw.Close()
			err = waitReady(pr)
			_ = pr.Close()
			if tc.want == nil && err != nil {
				t.Fatal(err)
			}
			if tc.want != nil && tc.want == ErrSpawnFailed && !errors.Is(err, tc.want) {
				t.Fatalf("error = %v", err)
			}
			if tc.name == "unexpected" && err == nil {
				t.Fatal("unexpected byte error expected")
			}
		})
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = pr.Close()
	if err := waitReady(pr); err == nil || errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("closed read error = %v", err)
	}
	_ = pw.Close()
	pr, pw, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	ReadyTimeout = time.Millisecond
	if err := waitReady(pr); !errors.Is(err, ErrSpawnTimeout) {
		t.Fatalf("timeout error = %v", err)
	}
	_ = pr.Close()
	_ = pw.Close()
}

func TestCrossPlatformCoverageStatusFilesystemEdges(t *testing.T) {
	origReadDir := statusReadDir
	origStat := statusStat
	t.Cleanup(func() { statusReadDir = origReadDir; statusStat = origStat })
	configDir := t.TempDir()
	eventsDir := filepath.Join(configDir, "events")
	statusReadDir = func(path string) ([]os.DirEntry, error) {
		if path == eventsDir {
			return nil, errBusctlInjected
		}
		return origReadDir(path)
	}
	if _, err := EnumerateBuses(configDir, ""); !errors.Is(err, errBusctlInjected) {
		t.Fatalf("events scan error = %v", err)
	}
	statusReadDir = origReadDir

	configDir = t.TempDir()
	legacy := makeBusDir(t, configDir, "open", "legacy", true, 0)
	_ = legacy
	sourceDir := filepath.Join(configDir, "events", "open", string(dwsevent.SourceKindPersonalStream), "identity")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := bus.WriteMeta(sourceDir, bus.Meta{
		ClientID: "client", Edition: "open", SourceKind: dwsevent.SourceKindPersonalStream,
		IdentityHash: "meta-identity",
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := EnumerateBuses(configDir, "")
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
	var personal BusEntry
	for _, entry := range entries {
		_ = entry.IPCEndpoint()
		if entry.SourceKind == dwsevent.SourceKindPersonalStream {
			personal = entry
		}
	}
	if personal.IdentityHash != "meta-identity" || personal.ClientIDHash != "meta-identity" {
		t.Fatalf("personal meta = %#v", personal)
	}
	if got := FindBusByIdentity(configDir, "open", dwsevent.SourceKindPersonalStream, "identity"); got == nil {
		t.Fatal("personal identity not found")
	}
	if got := FindBusByIdentity(configDir, "open", "", "missing"); got != nil {
		t.Fatalf("missing identity = %#v", got)
	}

	root := filepath.Join(configDir, "events")
	statusReadDir = func(path string) ([]os.DirEntry, error) {
		if path != root {
			return nil, errBusctlInjected
		}
		return origReadDir(path)
	}
	if got, err := EnumerateBuses(configDir, ""); err != nil || len(got) != 0 {
		t.Fatalf("edition scan skip = %#v, %v", got, err)
	}
	statusReadDir = func(path string) ([]os.DirEntry, error) {
		if filepath.Base(path) == string(dwsevent.SourceKindPersonalStream) {
			return nil, errBusctlInjected
		}
		return origReadDir(path)
	}
	if _, err := EnumerateBuses(configDir, ""); err != nil {
		t.Fatalf("identity scan should be skipped: %v", err)
	}

	statusStat = func(string) (os.FileInfo, error) { return nil, nil }
	if got := FindBusByClientID(configDir, "open", "virtual"); got == nil {
		t.Fatal("virtual primary path should be inspected")
	}
}

type queryErrorConn struct {
	writes int
	failAt int
}

func (c *queryErrorConn) Read([]byte) (int, error) { return 0, errBusctlInjected }
func (c *queryErrorConn) Write(p []byte) (int, error) {
	c.writes++
	if c.failAt > 0 && c.writes >= c.failAt {
		return 0, errBusctlInjected
	}
	return len(p), nil
}
func (*queryErrorConn) Close() error                     { return nil }
func (*queryErrorConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (*queryErrorConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (*queryErrorConn) SetDeadline(time.Time) error      { return errBusctlInjected }
func (*queryErrorConn) SetReadDeadline(time.Time) error  { return nil }
func (*queryErrorConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return string(a) }
func (a dummyAddr) String() string  { return string(a) }

func TestCrossPlatformCoverageQueryStatusAndEntryEdges(t *testing.T) {
	origDial := statusDial
	origQuery := queryStatus
	t.Cleanup(func() { statusDial = origDial; queryStatus = origQuery })
	statusDial = func(string) (net.Conn, error) { return nil, errBusctlInjected }
	if _, err := QueryStatus("x"); err == nil {
		t.Fatal("dial error expected")
	}
	for failAt := 1; failAt <= 4; failAt++ {
		statusDial = func(string) (net.Conn, error) { return &queryErrorConn{failAt: failAt}, nil }
		_, _ = QueryStatus("x")
	}
	statusDial = func(string) (net.Conn, error) { return &queryErrorConn{}, nil }
	if _, err := QueryStatus("x"); err == nil {
		t.Fatal("read error expected")
	}

	entry := BusEntry{State: BusStateNotRunning}
	if got := QueryEntry(entry); got.Live != nil {
		t.Fatalf("not-running live = %#v", got.Live)
	}
	entry.State = BusStateRunning
	queryStatus = func(string) (*transport.StatusResp, error) { return nil, errBusctlInjected }
	if got := QueryEntry(entry); got.Live != nil {
		t.Fatalf("failed live = %#v", got.Live)
	}
	want := &transport.StatusResp{Type: transport.FrameTypeStatusResp}
	queryStatus = func(string) (*transport.StatusResp, error) { return want, nil }
	if got := QueryEntry(entry); got.Live != want {
		t.Fatalf("live = %#v", got.Live)
	}
}

func TestCrossPlatformCoverageStopInjectedEdges(t *testing.T) {
	origRead := stopReadHolderPID
	origAlive := stopAlive
	origFind := stopFindProcess
	origSignal := stopSignalProcess
	t.Cleanup(func() {
		stopReadHolderPID = origRead
		stopAlive = origAlive
		stopFindProcess = origFind
		stopSignalProcess = origSignal
	})
	if err := Stop(StopConfig{}); err == nil {
		t.Fatal("workdir validation expected")
	}
	stopReadHolderPID = func(string) int { return 123 }
	stopAlive = func(int) bool { return true }
	stopFindProcess = func(int) (*os.Process, error) { return nil, errBusctlInjected }
	if err := Stop(StopConfig{WorkDir: "x"}); err == nil {
		t.Fatal("find error expected")
	}
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	stopFindProcess = func(int) (*os.Process, error) { return proc, nil }
	stopSignalProcess = func(*os.Process, os.Signal) error { return os.ErrProcessDone }
	if err := Stop(StopConfig{WorkDir: "x"}); err != nil {
		t.Fatalf("done signal = %v", err)
	}
	stopSignalProcess = func(*os.Process, os.Signal) error { return errBusctlInjected }
	if err := Stop(StopConfig{WorkDir: "x"}); err == nil {
		t.Fatal("signal error expected")
	}
	stopSignalProcess = func(*os.Process, os.Signal) error { return nil }
	calls := 0
	stopAlive = func(int) bool { calls++; return calls < 3 }
	if err := Stop(StopConfig{WorkDir: "x", Timeout: time.Second}); err != nil {
		t.Fatalf("poll exit = %v", err)
	}
	stopAlive = func(int) bool { return true }
	if err := Stop(StopConfig{WorkDir: "x", Timeout: time.Millisecond}); err == nil {
		t.Fatal("timeout expected")
	}
}

var _ io.Reader = (*queryErrorConn)(nil)

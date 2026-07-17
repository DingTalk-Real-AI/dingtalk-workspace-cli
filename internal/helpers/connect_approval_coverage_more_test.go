package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installApprovalIODefaults(t *testing.T) func() {
	t.Helper()
	originalMkdirAll := approvalMkdirAll
	originalCreateTemp := approvalCreateTemp
	originalChmod := approvalFileChmod
	originalWrite := approvalFileWrite
	originalClose := approvalFileClose
	originalRename := approvalRename
	originalRemove := approvalRemove
	originalReadDir := approvalReadDir
	originalReadFile := approvalReadFile
	t.Cleanup(func() {
		approvalMkdirAll = originalMkdirAll
		approvalCreateTemp = originalCreateTemp
		approvalFileChmod = originalChmod
		approvalFileWrite = originalWrite
		approvalFileClose = originalClose
		approvalRename = originalRename
		approvalRemove = originalRemove
		approvalReadDir = originalReadDir
		approvalReadFile = originalReadFile
	})
	return func() {
		approvalMkdirAll = os.MkdirAll
		approvalCreateTemp = os.CreateTemp
		approvalFileChmod = func(f *os.File, mode os.FileMode) error { return f.Chmod(mode) }
		approvalFileWrite = func(f *os.File, data []byte) (int, error) { return f.Write(data) }
		approvalFileClose = func(f *os.File) error { return f.Close() }
		approvalRename = os.Rename
		approvalRemove = os.Remove
		approvalReadDir = os.ReadDir
		approvalReadFile = os.ReadFile
	}
}

func TestCrossPlatformCoverageApprovalGateLookupMutationAndAwaitEdges(t *testing.T) {
	g := newApprovalGate("")
	g.setOutTrackID("missing", "track")
	if got := g.findByOutTrackID("  "); got != nil {
		t.Fatalf("blank track=%v", got)
	}
	if got := g.pendingForConv(" "); got != nil {
		t.Fatalf("blank conversation=%v", got)
	}
	if got := g.latestPending(); got != nil {
		t.Fatalf("empty latest=%v", got)
	}
	if got := g.Get("missing"); got != nil {
		t.Fatalf("missing get=%v", got)
	}
	if got, ok := g.Await(context.Background(), "missing", time.Millisecond); ok || got != nil {
		t.Fatalf("missing await=(%v,%v)", got, ok)
	}

	first := g.Submit(ApprovalRequest{ID: "first", CreatedAt: time.Unix(1, 0)})
	g.Decide(first.ID, true, "owner")
	if got, ok := g.Await(context.Background(), first.ID, 0); !ok || got == nil {
		t.Fatalf("decided await=(%v,%v)", got, ok)
	}
	g.markFailed(first.ID, strings.Repeat("failure", 100))
	if got := g.Get(first.ID); got.State != approvalFailed || got.ExecErr == "" {
		t.Fatalf("failed state=%+v", got)
	}

	pending := g.Submit(ApprovalRequest{ID: "pending"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, ok := g.Await(ctx, pending.ID, 0); ok || got == nil {
		t.Fatalf("cancelled await=(%v,%v)", got, ok)
	}

	older := g.Submit(ApprovalRequest{ID: "older", CreatedAt: time.Unix(2, 0)})
	newer := g.Submit(ApprovalRequest{ID: "newer", CreatedAt: time.Unix(3, 0)})
	g.Decide(older.ID, true, "owner")
	g.Decide(newer.ID, true, "owner")
	g.markDeferred(newer.ID, "x")
	g.markDeferred(older.ID, "x")
	deferred := g.allDeferred()
	if len(deferred) != 2 || deferred[0].ID != "older" {
		t.Fatalf("deferred order=%v", deferred)
	}
}

func TestCrossPlatformCoverageApprovalGatePersistFailureEdges(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	defaults := installApprovalIODefaults(t)
	g := &approvalGate{clientID: "io"}
	req := &ApprovalRequest{ID: "id", CreatedAt: time.Now()}
	boom := errors.New("io")

	approvalMkdirAll = func(string, os.FileMode) error { return boom }
	g.persist(req)
	defaults()

	bad := *req
	bad.ID = "marshal"
	bad.Action.Params = map[string]any{"bad": make(chan int)}
	g.persist(&bad)

	approvalCreateTemp = func(string, string) (*os.File, error) { return nil, boom }
	g.persist(req)
	defaults()

	approvalFileChmod = func(*os.File, os.FileMode) error { return boom }
	g.persist(req)
	defaults()

	approvalFileWrite = func(*os.File, []byte) (int, error) { return 0, boom }
	g.persist(req)
	defaults()

	approvalFileClose = func(f *os.File) error { _ = f.Close(); return boom }
	g.persist(req)
	defaults()

	approvalRename = func(string, string) error { return boom }
	g.persist(req)
}

func TestCrossPlatformCoverageApprovalGateLoadRemainingEdges(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	defaults := installApprovalIODefaults(t)
	g := &approvalGate{clientID: "load", reqs: make(map[string]*ApprovalRequest), waiters: make(map[string]chan struct{})}
	boom := errors.New("read dir")
	approvalReadDir = func(string) ([]os.DirEntry, error) { return nil, boom }
	g.loadAll()
	defaults()

	dir := g.approvalDir()
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"note.txt":        "ignored",
		"corrupt.json":    "{",
		"blank.json":      `{"id":" "}`,
		"read-error.json": `{"id":"unread"}`,
		"valid.json":      `{"id":"valid","state":"pending"}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	approvalReadFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "read-error.json") {
			return nil, errors.New("read")
		}
		return os.ReadFile(path)
	}
	g.loadAll()
	if g.Get("valid") == nil || g.Get("unread") != nil {
		t.Fatalf("loaded=%v", g.reqs)
	}
}

func TestCrossPlatformCoverageApprovalMarkerAndDueRemainingEdges(t *testing.T) {
	act, cleaned, found := parseActionMarker(`before [[ACTION:todo.create title='single quoted']] after`)
	if !found || act.Args["title"] != "single quoted" || strings.Contains(cleaned, "ACTION") {
		t.Fatalf("action=%+v cleaned=%q found=%v", act, cleaned, found)
	}
	if _, err := parseDueToMillis("not-a-date"); err == nil {
		t.Fatal("invalid due returned nil")
	}
}

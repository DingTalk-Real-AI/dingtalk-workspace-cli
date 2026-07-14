package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeEvent seals and appends one event through the writer+chain, mirroring
// FileSink.Emit's locking discipline, so tests exercise the real tail-derived
// hash chain path.
func writeEvent(t *testing.T, w *DateRotatingWriter, chain *Chain, evt *Event) {
	t.Helper()
	body, err := marshalWithoutHash(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f, release, err := w.beginAppend()
	if err != nil {
		t.Fatalf("beginAppend: %v", err)
	}
	prev, hash, err := chain.SealFromFile(f, body)
	if err != nil {
		release()
		t.Fatalf("seal: %v", err)
	}
	evt.PrevHash, evt.Hash = prev, hash
	line, err := json.Marshal(evt)
	if err != nil {
		release()
		t.Fatalf("marshal final: %v", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		release()
		t.Fatalf("write: %v", err)
	}
	release()
}

func sampleEvent(id string) *Event {
	return &Event{
		Timestamp:   time.Now(),
		ExecutionID: id,
		Actor:       Actor{UserID: "u1", CorpID: "c1"},
		Product:     "calendar",
		Command:     "event_list",
		Result:      "success",
		DurationMs:  10,
		CLIVersion:  "1.0.0",
		OS:          "darwin",
		Arch:        "arm64",
	}
}

// TestCrossDateChainIndependentPerFile verifies each day's file starts a fresh
// chain (prev_hash="") and verifies independently — the bug the removed global
// .chain sidecar introduced.
func TestCrossDateChainIndependentPerFile(t *testing.T) {
	dir := t.TempDir()
	chain := NewChain(dir)

	// Simulate two calendar days by writing files directly with the same chain
	// semantics: each file's first record must chain from "".
	for _, day := range []string{"20260101", "20260102"} {
		path := filepath.Join(dir, "audit-"+day+".jsonl")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			evt := sampleEvent(fmt.Sprintf("%s-%d", day, i))
			body, _ := marshalWithoutHash(evt)
			prev, hash, err := chain.SealFromFile(f, body)
			if err != nil {
				t.Fatal(err)
			}
			evt.PrevHash, evt.Hash = prev, hash
			line, _ := json.Marshal(evt)
			if _, err := f.Write(append(line, '\n')); err != nil {
				t.Fatal(err)
			}
		}
		f.Close()

		valid, brokenAt, err := VerifyFile(path)
		if err != nil {
			t.Fatalf("verify %s: %v", day, err)
		}
		if !valid {
			t.Fatalf("file %s chain broken at line %d", day, brokenAt)
		}
	}
}

// TestCrossProcessChainSharedFile simulates two independent writers (as two
// processes would) appending to the same day's file. Because each seal derives
// prev_hash from the file tail under the inter-process lock, the resulting chain
// must remain valid with no fork.
func TestCrossProcessChainSharedFile(t *testing.T) {
	dir := t.TempDir()

	w1, err := NewDateRotatingWriter(dir, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer w1.Close()
	w2, err := NewDateRotatingWriter(dir, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	chain := NewChain(dir)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		w := w1
		if i%2 == 1 {
			w = w2
		}
		go func(w *DateRotatingWriter, i int) {
			defer wg.Done()
			writeEvent(t, w, chain, sampleEvent(fmt.Sprintf("exec-%d", i)))
		}(w, i)
	}
	wg.Wait()

	file, err := LatestAuditFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	valid, brokenAt, err := VerifyFile(file)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !valid {
		t.Fatalf("cross-process chain broken at line %d", brokenAt)
	}
}

// TestForwarderCloseWaitsForDelivery ensures Close blocks until every async
// forward has been delivered to the remote endpoint.
func TestForwarderCloseWaitsForDelivery(t *testing.T) {
	var received int64
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hold the handler so delivery is still in flight at Close time
		atomic.AddInt64(&received, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fwd := NewHTTPForwarder(srv.URL, "", RedactNone, nil)
	const n = 5
	for i := 0; i < n; i++ {
		fwd.Forward(*sampleEvent(fmt.Sprintf("e-%d", i)))
	}

	// Nothing delivered yet because handlers are blocked.
	if got := atomic.LoadInt64(&received); got != 0 {
		t.Fatalf("expected 0 delivered before release, got %d", got)
	}
	close(release)

	if err := fwd.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := atomic.LoadInt64(&received); got != n {
		t.Fatalf("expected %d delivered after Close, got %d", n, got)
	}
}

// TestForwarderCloseTimeoutReports verifies Close honors the ctx deadline and
// reports instead of blocking forever when the endpoint never responds.
func TestForwarderCloseTimeoutReports(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	// Defers run LIFO: close(block) first unblocks the handler so srv.Close can
	// join its connection goroutine without deadlocking.
	defer srv.Close()
	defer close(block)

	var reported int64
	report := func(string, ...any) { atomic.AddInt64(&reported, 1) }
	fwd := NewHTTPForwarder(srv.URL, "", RedactNone, report)
	fwd.timeout = 10 * time.Second // keep the in-flight send alive past ctx
	fwd.Forward(*sampleEvent("stuck"))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := fwd.Close(ctx); err == nil {
		t.Fatal("expected timeout error from Close")
	}
	if atomic.LoadInt64(&reported) == 0 {
		t.Fatal("expected Close timeout to be reported")
	}
}

// TestBuildSinkInitFailureObservable verifies BuildSink surfaces an error (and
// the caller can fall back) when the audit directory cannot be created.
func TestBuildSinkInitFailureObservable(t *testing.T) {
	dir := t.TempDir()
	// Make a file where the audit subdir is expected so MkdirAll fails.
	clash := filepath.Join(dir, "audit")
	if err := os.WriteFile(clash, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAuditDir, filepath.Join(clash, "sub"))

	var reported int64
	_, err := BuildSink(dir, func(string, ...any) { atomic.AddInt64(&reported, 1) })
	if err == nil {
		t.Fatal("expected BuildSink to fail when audit dir is unusable")
	}
}

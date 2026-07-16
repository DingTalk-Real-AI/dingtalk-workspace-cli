package helpers

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func opencodeTestClient(t *testing.T, handler http.HandlerFunc) (*opencodeHTTPClient, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	return &opencodeHTTPClient{baseURL: server.URL, username: "user", password: "pass", httpClient: server.Client()}, server.Close
}

func TestCrossPlatformCoverageOpencodeForwarderRemainingEdges(t *testing.T) {
	t.Run("create session error", func(t *testing.T) {
		client, closeServer := opencodeTestClient(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "failed", http.StatusInternalServerError) })
		defer closeServer()
		f := &opencodeForwarder{server: &opencodeServer{}, sessions: newOpencodeSessions("")}
		if _, err := f.forwardWithClient(context.Background(), client, "conv", "text", nil); err == nil {
			t.Fatal("create error returned nil")
		}
	})
	t.Run("message error and backend reply", func(t *testing.T) {
		response := "error"
		client, closeServer := opencodeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/session" {
				_, _ = io.WriteString(w, `{"id":"session"}`)
				return
			}
			if response == "error" {
				http.Error(w, "failed", http.StatusInternalServerError)
			} else {
				_, _ = io.WriteString(w, `{"parts":[{"type":"text","text":"API Error: backend"}]}`)
			}
		})
		defer closeServer()
		f := &opencodeForwarder{server: &opencodeServer{}}
		if _, err := f.forwardWithClient(context.Background(), client, "conv", "text", nil); err == nil {
			t.Fatal("message error returned nil")
		}
		response = "backend"
		if reply, err := f.forwardWithClient(context.Background(), client, "conv", "text", nil); err != nil || !strings.Contains(reply, "backend") {
			t.Fatalf("backend reply=%q error=%v", reply, err)
		}
	})

	origAbs := opencodeAbsPath
	t.Cleanup(func() { opencodeAbsPath = origAbs })
	opencodeAbsPath = func(string) (string, error) { return "", errors.New("abs") }
	f := &opencodeForwarder{workDir: "relative"}
	if f.cwd() != "relative" {
		t.Fatalf("cwd fallback = %q", f.cwd())
	}
	opencodeAbsPath = filepath.Abs
	if got := (&opencodeForwarder{workDir: "."}).cwd(); !filepath.IsAbs(got) {
		t.Fatalf("absolute cwd = %q", got)
	}

	if err := (&opencodeForwarder{}).clearSession(context.Background(), "conv"); err != nil {
		t.Fatal(err)
	}
	f.sessions = newOpencodeSessions("")
	if err := f.clearSession(context.Background(), "conv"); err != nil {
		t.Fatal(err)
	}
	f.sessions.set("conv", "session")
	f.server = newOpencodeServer(filepath.Join(t.TempDir(), "missing"), nil, "", false)
	if err := f.clearSession(context.Background(), "conv"); err == nil {
		t.Fatal("server ensure error returned nil")
	}
}

func TestCrossPlatformCoverageOpencodeServerEnsureAndCloseEdges(t *testing.T) {
	origPort := opencodeFreeLocalPort
	origCommand := opencodeExecCommand
	origWait := opencodeServerStartupWait
	origHealthy := opencodeWaitHealthy
	t.Cleanup(func() {
		opencodeFreeLocalPort = origPort
		opencodeExecCommand = origCommand
		opencodeServerStartupWait = origWait
		opencodeWaitHealthy = origHealthy
	})

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"healthy":true}`)
	}))
	defer healthy.Close()
	s := &opencodeServer{baseURL: healthy.URL}
	if _, err := s.ensure(context.Background()); err != nil || s.httpClient == nil {
		t.Fatalf("existing server ensure: %v", err)
	}

	opencodeFreeLocalPort = func() (int, error) { return 0, errors.New("port") }
	if _, err := newOpencodeServer("bin", nil, "", false).ensure(context.Background()); err == nil {
		t.Fatal("port error returned nil")
	}
	opencodeFreeLocalPort = origPort
	if _, err := newOpencodeServer(filepath.Join(t.TempDir(), "missing"), nil, "", false).ensure(context.Background()); err == nil {
		t.Fatal("start error returned nil")
	}

	opencodeExecCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "sleep 5") }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s = newOpencodeServer("ignored", nil, t.TempDir(), false)
	if _, err := s.ensure(ctx); err == nil {
		t.Fatal("cancelled startup returned nil")
	}
	if s.cmd != nil || s.baseURL != "" {
		t.Fatalf("failed startup not cleaned: %#v", s)
	}

	// A successful injected readiness check covers the final state return; the
	// real waitHealthy state machine is exercised independently below.
	opencodeWaitHealthy = func(*opencodeServer, context.Context, *opencodeHTTPClient) error { return nil }
	opencodeExecCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "sleep 5") }
	s = newOpencodeServer("ignored", nil, "", false)
	if client, err := s.ensure(context.Background()); err != nil || client == nil {
		t.Fatalf("successful startup: client=%v err=%v", client, err)
	}
	_ = s.close()

	// Existing unhealthy process is closed before attempting a replacement.
	proc := exec.Command("sh", "-c", "sleep 5")
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func(cmd *exec.Cmd) { done <- cmd.Wait() }(proc)
	s = &opencodeServer{cmd: proc, done: done, baseURL: "http://127.0.0.1:1", httpClient: &http.Client{}}
	opencodeFreeLocalPort = func() (int, error) { return 0, errors.New("after-close") }
	if _, err := s.ensure(context.Background()); err == nil {
		t.Fatal("replacement error returned nil")
	}
	opencodeFreeLocalPort = origPort

	proc = exec.Command("sh", "-c", "sleep 5")
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	done = make(chan error, 1)
	go func(cmd *exec.Cmd) { done <- cmd.Wait() }(proc)
	s = &opencodeServer{cmd: proc, done: done, baseURL: "url", password: "pw"}
	if err := s.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestCrossPlatformCoverageOpencodeWaitHealthyRemainingEdges(t *testing.T) {
	origWait := opencodeServerStartupWait
	origSleep := helperSleep
	t.Cleanup(func() {
		opencodeServerStartupWait = origWait
		helperSleep = origSleep
	})
	helperSleep = func(time.Duration) { time.Sleep(time.Millisecond) }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &opencodeServer{}
	if err := s.waitHealthy(ctx, &opencodeHTTPClient{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}

	for _, processErr := range []error{errors.New("exit"), nil} {
		s = &opencodeServer{done: make(chan error, 1)}
		s.done <- processErr
		if err := s.waitHealthy(context.Background(), &opencodeHTTPClient{}); err == nil {
			t.Fatal("process exit returned nil")
		}
	}

	opencodeServerStartupWait = 3 * time.Millisecond
	bad := &opencodeHTTPClient{baseURL: "http://127.0.0.1:1", httpClient: &http.Client{}}
	if err := s.waitHealthy(context.Background(), bad); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("health timeout=%v", err)
	}
	opencodeServerStartupWait = 0
	if err := s.waitHealthy(context.Background(), bad); err == nil {
		t.Fatal("empty timeout returned nil")
	}
}

func TestCrossPlatformCoverageOpencodeHTTPClientRemainingEdges(t *testing.T) {
	ctx := context.Background()
	empty := &opencodeHTTPClient{httpClient: &http.Client{}}
	if err := empty.health(ctx); err == nil {
		t.Fatal("empty URL health returned nil")
	}
	if err := empty.deleteSession(ctx, "  "); err != nil {
		t.Fatal(err)
	}

	client, closeServer := opencodeTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/global/health":
			_, _ = io.WriteString(w, `{"healthy":false}`)
		case "/session-empty":
			_, _ = io.WriteString(w, `{}`)
		case "/session/missing":
			http.NotFound(w, r)
		case "/session/fail":
			http.Error(w, "delete failed", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})
	defer closeServer()
	if err := client.health(ctx); err == nil {
		t.Fatal("health=false returned nil")
	}
	// Point createSession at a server whose /session response has no id.
	missingID, closeMissing := opencodeTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{}`) })
	defer closeMissing()
	if _, err := missingID.createSession(ctx); err == nil {
		t.Fatal("missing session id returned nil")
	}
	messageClient, closeMessage := opencodeTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"parts":[{"type":"text","text":"reply"}]}`)
	})
	defer closeMessage()
	if reply, err := messageClient.sendMessageWithAttachments(ctx, "session", "text", "provider/model", nil); err != nil || reply != "reply" {
		t.Fatalf("modeled message reply=%q err=%v", reply, err)
	}
	if err := client.deleteSession(ctx, "missing"); err != nil {
		t.Fatalf("missing delete: %v", err)
	}
	if err := client.deleteSession(ctx, "fail"); err == nil {
		t.Fatal("failed delete returned nil")
	}
}

func TestCrossPlatformCoverageOpencodeDoJSONAndPartsEdges(t *testing.T) {
	ctx := context.Background()
	client := &opencodeHTTPClient{baseURL: "http://example.invalid", httpClient: &http.Client{}}
	if err := client.doJSON(ctx, http.MethodPost, "/x", make(chan int), nil); err == nil {
		t.Fatal("marshal error returned nil")
	}
	client.baseURL = "%"
	if err := client.doJSON(ctx, http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("request error returned nil")
	}

	responses := []struct {
		status int
		body   string
		out    any
		err    bool
	}{
		{http.StatusInternalServerError, "", nil, true},
		{http.StatusInternalServerError, "failure", nil, true},
		{http.StatusNoContent, "", &map[string]any{}, false},
		{http.StatusOK, "{", &map[string]any{}, true},
		{http.StatusOK, `{}`, nil, false},
	}
	for _, tc := range responses {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		c := &opencodeHTTPClient{baseURL: server.URL, username: "u", password: "p", httpClient: server.Client()}
		err := c.doJSON(ctx, http.MethodPost, "/x", map[string]any{"x": 1}, tc.out)
		server.Close()
		if (err != nil) != tc.err {
			t.Fatalf("status=%d body=%q error=%v wantErr=%v", tc.status, tc.body, err, tc.err)
		}
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closedClient := closed.Client()
	closed.Close()
	if err := (&opencodeHTTPClient{baseURL: closedURL, httpClient: closedClient}).doJSON(ctx, http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("transport error returned nil")
	}

	parts := []opencodePart{
		{Type: "text", Delta: "delta", Data: []byte(`{"text":"data"}`)},
		{Type: "text", Data: []byte(`{`)},
		{Parts: []opencodePart{{Type: "text", Text: "nested"}}},
	}
	if got := opencodePartsText(parts); !strings.Contains(got, "delta") || !strings.Contains(got, "data") || !strings.Contains(got, "nested") {
		t.Fatalf("parts text=%q", got)
	}
	for _, model := range []string{"", "model", "/missing", "provider/", "provider/model"} {
		_ = opencodeModelRef(model)
	}
}

func TestCrossPlatformCoverageOpencodeSystemHelperEdges(t *testing.T) {
	origPort := opencodeFreeLocalPort
	origRand := opencodeRandRead
	origListen := freeLocalPortListen
	t.Cleanup(func() {
		opencodeFreeLocalPort = origPort
		opencodeRandRead = origRand
		freeLocalPortListen = origListen
	})
	opencodeRandRead = func([]byte) (int, error) { return 0, errors.New("rand") }
	if randomHex(4) == "" {
		t.Fatal("random fallback empty")
	}
	freeLocalPortListen = func(string, string) (net.Listener, error) { return nil, errors.New("listen") }
	if _, err := freeLocalPort(); err == nil {
		t.Fatal("listen error returned nil")
	}
	if opencodeConvKey(" ") != "_default" {
		t.Fatal("blank conversation key")
	}
	sessions := newOpencodeSessions("")
	sessions.set("conv", "session")
	sessions.set("conv", "")
}

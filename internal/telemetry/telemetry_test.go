package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"gitlab.alibaba-inc.com/aes/aem-go-sdk/clitrack"
)

// The subprocess transport override exists only in the test binary. Production
// workers accept neither an endpoint nor an arbitrary field map in their IPC.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == workerArgument {
		if os.Getenv("DWS_TELEMETRY_TEST_MODE") == "hold" {
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}
		if os.Getenv("DWS_TELEMETRY_TEST_MODE") == "wire" {
			err := runWorker(os.Stdin, acquireSlot, func(event Event) error {
				cfg := sdkConfig(event)
				cfg.Endpoint = os.Getenv("DWS_TELEMETRY_TEST_ENDPOINT")
				return sendWithConfig(event, cfg)
			})
			if err != nil {
				os.Exit(2)
			}
			os.Exit(0)
		}
		RunWorker(os.Args[1:], os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func sampleEvent() Event {
	return Event{Protocol: protocolVersion, Version: "0.0.0-test", Command: "dws", Path: "sheet read", ExitCode: 3,
		DurationMillis: 234, CompletedAtMillis: 1700000000123, ErrorSummary: "sanitized failure 错误",
		Identity: Identity{UserID: "user-1", UserName: "测试用户", CorpID: "corp-1"}}
}

func TestCrossPlatformCoverageSubmitBoundsLocalHandoff(t *testing.T) {
	for _, mode := range []string{"success", "error", "panic", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			called := make(chan []byte, 1)
			returned := make(chan struct{})
			start := time.Now()
			submit(sampleEvent(), 10*time.Millisecond, func(ctx context.Context, payload []byte) error {
				defer close(returned)
				called <- payload
				switch mode {
				case "error":
					return errors.New("private transport error")
				case "panic":
					panic("private SDK error")
				case "blocked":
					<-ctx.Done()
				}
				return nil
			})
			if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
				t.Fatalf("handoff blocked for %s", elapsed)
			}
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("launch did not cancel")
			}
			var got Event
			if err := json.Unmarshal(<-called, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, sampleEvent()) {
				t.Fatalf("event changed: %+v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageSubmitRejectsOversizedOrInvalidEvents(t *testing.T) {
	for _, mutation := range []func(*Event){
		func(e *Event) { e.Command = "" },
		func(e *Event) { e.Path = "" },
		func(e *Event) { e.ExitCode = -1 },
		func(e *Event) { e.ExitCode = 256 },
		func(e *Event) { e.DurationMillis = -1 },
		func(e *Event) { e.DurationMillis = 1 << 62 },
		func(e *Event) { e.CompletedAtMillis = 0 },
		func(e *Event) { e.ErrorSummary = strings.Repeat("x", maxEventBytes) },
		func(e *Event) { e.Identity.UserName = strings.Repeat("x", maxEventBytes) },
		func(e *Event) { e.ErrorSummary = strings.Repeat("\x00", maxEventBytes/2) },
	} {
		event := sampleEvent()
		mutation(&event)
		submit(event, time.Second, func(context.Context, []byte) error { t.Error("invalid event launched worker"); return nil })
	}
	t.Setenv("DO_NOT_TRACK", "1")
	Submit(sampleEvent())
	if !OptedOut() {
		t.Fatal("opt out ignored")
	}
	t.Setenv("DO_NOT_TRACK", "  ")
	if OptedOut() {
		t.Fatal("empty opt out enabled")
	}
	Submit(Event{}) // Invalid input must not create a sender even when enabled.
}

type closeFunc func() error

func (f closeFunc) Close() error { return f() }

func TestCrossPlatformCoverageWorkerValidatesBeforeSending(t *testing.T) {
	payload, _ := json.Marshal(sampleEvent())
	for name, input := range map[string][]byte{
		"truncated": payload[:len(payload)-1], "oversized": bytes.Repeat([]byte("x"), maxEventBytes+1),
		"unknown field": []byte(strings.Replace(string(payload), `"protocol":1`, `"protocol":1,"p2":"Codex"`, 1)),
		"wrong version": []byte(strings.Replace(string(payload), `"protocol":1`, `"protocol":2`, 1)),
		"extra object":  append(slices.Clone(payload), []byte(" {}")...), "empty": nil,
	} {
		t.Run(name, func(t *testing.T) {
			err := runWorker(bytes.NewReader(input), func() (io.Closer, error) { t.Error("invalid payload acquired slot"); return nil, nil }, func(Event) error { t.Error("invalid payload sent"); return nil })
			if err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	for _, sendError := range []bool{false, true} {
		closed, sends := false, 0
		err := runWorker(bytes.NewReader(payload), func() (io.Closer, error) { return closeFunc(func() error { closed = true; return nil }), nil }, func(event Event) error {
			sends++
			if !reflect.DeepEqual(event, sampleEvent()) {
				t.Error("event changed")
			}
			if sendError {
				return errors.New("send failed")
			}
			return nil
		})
		if closed != true || sends != 1 || (err != nil) != sendError {
			t.Fatalf("closed=%v sends=%d err=%v", closed, sends, err)
		}
	}
	if err := runWorker(bytes.NewReader(payload), func() (io.Closer, error) { return nil, errors.New("busy") }, func(Event) error { t.Error("sent without slot"); return nil }); err == nil {
		t.Fatal("busy slot ignored")
	}
}

func TestCrossPlatformCoverageWorkerLimitsConcurrentSends(t *testing.T) {
	dir := t.TempDir()
	var slots []io.Closer
	for i := 0; i < maxSenders; i++ {
		slot, err := acquireSlotIn(dir)
		if err != nil {
			t.Fatal(err)
		}
		slots = append(slots, slot)
	}
	defer func() {
		for _, slot := range slots {
			slot.Close()
		}
	}()
	if slot, err := acquireSlotIn(dir); err == nil {
		slot.Close()
		t.Fatal("accepted ninth sender")
	}
	slots[0].Close()
	reused, err := acquireSlotIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reused.Close()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != maxSenders {
		t.Fatalf("slot files=%d", len(entries))
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil || len(data) != 0 {
			t.Fatalf("persisted telemetry: size=%d err=%v", len(data), err)
		}
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireSlotIn(filepath.Join(file, "slots")); err == nil {
		t.Fatal("invalid cache accepted")
	}
}

func TestCrossPlatformCoverageSDKWireUsesExistingWhitelist(t *testing.T) {
	for _, code := range []int{0, 3} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			event := sampleEvent()
			event.ExitCode = code
			if code == 0 {
				event.ErrorSummary = ""
				event.Identity = Identity{}
			}
			request := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/aes.1.1" {
					t.Error("incorrect request target")
				}
				if r.UserAgent() != "dws/0.0.0-test" {
					t.Error("incorrect user agent")
				}
				body, _ := io.ReadAll(r.Body)
				request <- body
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			cfg := sdkConfig(event)
			cfg.Endpoint = server.URL
			if !cfg.NoAttribution || !cfg.NoAutomaticDimensions || !cfg.NoCommandLine || !cfg.NoCwd || cfg.CaptureOutput {
				t.Fatal("privacy controls disabled")
			}
			if err := sendWithConfig(event, cfg); err != nil {
				t.Fatal(err)
			}
			common, fields := decodeEvent(t, <-request)
			keys := []string{"app_name", "app_version", "env", "msg", "pid", "platform", "version"}
			eventKeys := []string{"c1", "c3", "c4", "c9", "p1", "p4", "ts", "type"}
			if code != 0 {
				keys = append(keys, "uid", "username")
				eventKeys = append(eventKeys, "c5", "c10")
			}
			assertKeys(t, common, keys)
			assertKeys(t, fields, eventKeys)
			for key, want := range map[string]string{"c1": "dws", "c3": fmt.Sprint(code), "c4": "234", "c9": "sheet read", "p1": "cli.exec", "p4": "SYS", "ts": "1700000000123", "c5": event.ErrorSummary, "c10": event.Identity.CorpID} {
				if fields.Get(key) != want {
					t.Fatalf("%s=%q want %q", key, fields.Get(key), want)
				}
			}
			if common.Get("uid") != event.Identity.UserID || common.Get("username") != event.Identity.UserName {
				t.Fatal("identity changed")
			}
		})
	}
}

func decodeEvent(t *testing.T, body []byte) (url.Values, url.Values) {
	t.Helper()
	var packet map[string]string
	if err := json.Unmarshal(body, &packet); err != nil {
		t.Fatal(err)
	}
	decoded, err := url.QueryUnescape(packet["gokey"])
	if err != nil {
		t.Fatal(err)
	}
	common, err := url.ParseQuery(decoded)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := url.ParseQuery(common.Get("msg"))
	if err != nil {
		t.Fatal(err)
	}
	return common, fields
}

func assertKeys(t *testing.T, values url.Values, want []string) {
	t.Helper()
	got := make([]string, 0, len(values))
	for key := range values {
		got = append(got, key)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("fields=%v want %v", got, want)
	}
}

func TestCrossPlatformCoverageDetachedSenderPreservesEOF(t *testing.T) {
	request := make(chan []byte, 1)
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request <- body
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer func() { once.Do(func() { close(release) }); server.Close() }()
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("DWS_TELEMETRY_TEST_MODE", "wire")
	t.Setenv("DWS_TELEMETRY_TEST_ENDPOINT", server.URL)
	t.Setenv("DWS_TELEMETRY_TEST_PARENT", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestCrossPlatformCoverageTelemetryParentHelper$")
	cmd.WaitDelay = 100 * time.Millisecond
	output, err := cmd.CombinedOutput()
	if err != nil {
		once.Do(func() { close(release) })
		t.Fatalf("parent/pipe waited for sender: %v %s", err, output)
	}
	if !bytes.Contains(output, []byte("business stdout")) || !bytes.Contains(output, []byte("business stderr")) {
		t.Fatalf("output=%q", output)
	}
	select {
	case body := <-request:
		_, fields := decodeEvent(t, body)
		if fields.Get("c4") != "234" || fields.Get("c3") != "3" {
			t.Fatal("worker reported its own execution")
		}
	case <-time.After(3 * time.Second):
		once.Do(func() { close(release) })
		t.Fatal("detached event did not arrive after parent exit")
	}
	once.Do(func() { close(release) })
}

func TestCrossPlatformCoverageLaunchDetachedDirectly(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("DWS_TELEMETRY_TEST_MODE", "wire")
	request := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv("DWS_TELEMETRY_TEST_ENDPOINT", server.URL)
	payload, _ := json.Marshal(sampleEvent())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := launchDetached(ctx, payload); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel() // Completion must detach the worker from the handoff cancellation.
	select {
	case <-request:
	case <-time.After(3 * time.Second):
		t.Fatal("completed handoff was cancelled")
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := launchDetached(ctx, payload); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled launch=%v", err)
	}
}

func TestCrossPlatformCoverageTelemetryParentHelper(t *testing.T) {
	if os.Getenv("DWS_TELEMETRY_TEST_PARENT") != "1" {
		t.Skip("subprocess helper")
	}
	fmt.Fprintln(os.Stdout, "business stdout")
	fmt.Fprintln(os.Stderr, "business stderr")
	// This test requires a completed handoff to exercise detached delivery and
	// pipe EOF. Production's shorter best-effort budget may legitimately drop
	// the event on a busy runner; its deadline is tested separately.
	submit(sampleEvent(), time.Second, launchDetached)
}

func TestCrossPlatformCoverageWorkerDispatchAndDeadline(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	file, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if RunWorker([]string{"version"}, file) {
		t.Fatal("business command intercepted")
	}
	if !RunWorker([]string{workerArgument}, file) {
		t.Fatal("private invocation entered business")
	}
	badRead, badWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer badRead.Close()
	_, _ = badWrite.Write([]byte("invalid json"))
	_ = badWrite.Close()
	if !RunWorker([]string{workerArgument}, badRead) {
		t.Fatal("malformed worker input entered business")
	}
	t.Setenv("DO_NOT_TRACK", "1")
	if !RunWorker([]string{workerArgument}, file) {
		t.Fatal("opted-out worker entered business")
	}
	t.Setenv("DO_NOT_TRACK", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, workerArgument)
	cmd.Stdin = read
	cmd.Env = append(os.Environ(), "DO_NOT_TRACK=", "DWS_TELEMETRY_TEST_MODE=deadline")
	if err := cmd.Run(); err != nil {
		t.Fatalf("worker failed to terminate on incomplete input: %v", err)
	}
}

func BenchmarkTelemetryEncode(b *testing.B) {
	event := sampleEvent()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(event); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCrossPlatformCoverageWorkerFaults(t *testing.T) {
	errRead := errors.New("read failed")
	if err := runWorker(iotest.ErrReader(errRead), nil, nil); !errors.Is(err, errRead) {
		t.Fatalf("read error=%v", err)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("LOCALAPPDATA", "")
	if slot, err := acquireSlot(); err == nil {
		slot.Close()
		t.Fatal("missing cache accepted")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "0.lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if slot, err := acquireSlotIn(dir); err == nil {
		slot.Close()
		t.Fatal("lock collision accepted")
	}
}

type fakeTracker struct {
	reportErr, closeErr error
	reported            clitrack.Execution
	closes              int
}

func (f *fakeTracker) ReportExecution(e clitrack.Execution) error { f.reported = e; return f.reportErr }
func (f *fakeTracker) Close() error                               { f.closes++; return f.closeErr }

func TestCrossPlatformCoverageSenderPreservesSDKFailures(t *testing.T) {
	for _, fails := range []string{"track", "close", "both", "none"} {
		t.Run(fails, func(t *testing.T) {
			fake := &fakeTracker{}
			if fails == "track" || fails == "both" {
				fake.reportErr = errors.New("track failed")
			}
			if fails == "close" || fails == "both" {
				fake.closeErr = errors.New("send failed")
			}
			testseam.Swap(t, &newTracker, func(cfg clitrack.Config) executionTracker {
				if !cfg.NoAttribution || !cfg.NoAutomaticDimensions || cfg.PID != "wcCRwZ" {
					t.Error("incorrect SDK config")
				}
				return fake
			})
			err := sendEvent(sampleEvent())
			want := fake.reportErr
			if want == nil {
				want = fake.closeErr
			}
			if err != want || fake.closes != 1 || fake.reported.Duration != 234*time.Millisecond {
				t.Fatalf("err=%v closes=%d execution=%+v", err, fake.closes, fake.reported)
			}
		})
	}
}

func TestCrossPlatformCoverageWorkerRecoversAndArmsDeadline(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	testseam.Swap(t, &workerSend, func(Event) error { panic("private SDK error") })
	exited := -1
	testseam.Swap(t, &workerExit, func(code int) { exited = code })
	testseam.Swap(t, &workerAfterFunc, func(d time.Duration, f func()) *time.Timer {
		if d != 6*time.Second {
			t.Error("incorrect lifetime")
		}
		f()
		return time.NewTimer(time.Hour)
	})
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	payload, _ := json.Marshal(sampleEvent())
	_, _ = write.Write(payload)
	write.Close()
	if !RunWorker([]string{workerArgument}, read) || exited != 0 {
		t.Fatalf("private worker dispatch/exit=%d", exited)
	}
}

func TestCrossPlatformCoverageLaunchFailures(t *testing.T) {
	t.Run("executable", func(t *testing.T) {
		testseam.Swap(t, &findExecutable, func() (string, error) { return "", errors.New("unavailable") })
		if err := launchDetached(context.Background(), nil); err == nil {
			t.Fatal("lookup failure ignored")
		}
	})
	t.Run("pipe", func(t *testing.T) {
		testseam.Swap(t, &openPipe, func() (*os.File, *os.File, error) { return nil, nil, errors.New("no descriptors") })
		if err := launchDetached(context.Background(), nil); err == nil {
			t.Fatal("pipe failure ignored")
		}
	})
	t.Run("start", func(t *testing.T) {
		testseam.Swap(t, &findExecutable, func() (string, error) { return filepath.Join(t.TempDir(), "missing"), nil })
		if err := launchDetached(context.Background(), nil); err == nil {
			t.Fatal("spawn failure ignored")
		}
	})
	t.Run("cancel after start", func(t *testing.T) {
		t.Setenv("DWS_TELEMETRY_TEST_MODE", "hold")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		testseam.Swap(t, &startCommand, func(cmd *exec.Cmd) error { err := cmd.Start(); cancel(); return err })
		if err := launchDetached(ctx, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled spawn=%v", err)
		}
	})
	t.Run("closed writer", func(t *testing.T) {
		testseam.Swap(t, &openPipe, func() (*os.File, *os.File, error) {
			r, w, err := os.Pipe()
			if err == nil {
				w.Close()
			}
			return r, w, err
		})
		if err := launchDetached(context.Background(), []byte("event")); err == nil {
			t.Fatal("closed writer accepted")
		}
	})
	t.Run("blocked writer", func(t *testing.T) {
		t.Setenv("DWS_TELEMETRY_TEST_MODE", "hold")
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := launchDetached(ctx, make([]byte, 4<<20)); err == nil {
			t.Fatal("blocked writer succeeded")
		}
		if time.Since(start) > time.Second {
			t.Fatal("blocked pipe exceeded cancellation budget")
		}
	})
}

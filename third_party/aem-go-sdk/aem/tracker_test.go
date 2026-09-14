package aem

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// extractGokeyFromBody 从 server 收到的 JSON body 中提取并解码 gokey。
//
// 流程：JSON parse 拿到 gokey 字段 → URL decode 一次（因为 sender 做了 EncodeURIComponent）。
// 注意 gokey 内部的 msg=... 部分仍是 urlencoded（双重编码协议），所以 strings.Contains
// 用 "type%3Dapi" 这样的 encoded 形式匹配。
func extractGokeyFromBody(t *testing.T, body string) string {
	t.Helper()
	var parsed map[string]string
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("body is not JSON: %v\nbody=%s", err, body)
	}
	encoded, ok := parsed["gokey"]
	if !ok || encoded == "" {
		t.Fatalf("body has no gokey field: %s", body)
	}
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		t.Fatalf("url.QueryUnescape gokey failed: %v", err)
	}
	return decoded
}

func TestNewTracker_AppliesDefaults(t *testing.T) {
	tr := NewTracker(Config{"pid": "Hp8rK6"})
	defer tr.Close()
	if tr.config["app_name"] != "unknown" || tr.config["env"] != "prod" || tr.config["endpoint"] != "gm.mmstat.com" {
		t.Errorf("NewTracker did not apply defaults: %+v", tr.config)
	}
	if tr.sendCfg["pid"] != "Hp8rK6" {
		t.Errorf("sendCfg pid = %q, want Hp8rK6", tr.sendCfg["pid"])
	}
	if !tr.async || tr.queue == nil {
		t.Errorf("NewTracker should default to async queue")
	}
}

func TestTracker_Track_SendsExpectedBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": server.URL, "env": "dev"})
	err := tr.Track(Event{
		Type: "api",
		Fields: map[string]string{
			"url":    "/api/user",
			"status": "200",
		},
	})
	if err != nil {
		t.Fatalf("Track failed: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	gokey := extractGokeyFromBody(t, receivedBody)
	if !strings.Contains(gokey, "type%3Dapi") {
		t.Errorf("gokey should contain encoded type=api, got: %s", gokey)
	}
	if !strings.Contains(gokey, "url%3D") {
		t.Errorf("gokey should contain encoded url=, got: %s", gokey)
	}
}

func TestTracker_Track_AutoFillsTimestamp(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": server.URL})
	// 不在 Fields 里给 ts，应自动补
	_ = tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "login", "p4": "SYS"}})
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	gokey := extractGokeyFromBody(t, receivedBody)
	if !strings.Contains(gokey, "ts%3D") {
		t.Errorf("Track should auto-fill ts, gokey: %s", gokey)
	}
}

func TestTracker_Track_PreservesUserTimestamp(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": server.URL})
	_ = tr.Track(Event{
		Type:   "event",
		Fields: map[string]string{"ts": "1234567890000", "p1": "x", "p4": "SYS"},
	})
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	gokey := extractGokeyFromBody(t, receivedBody)
	// 用户传了 ts=1234567890000 应该被保留
	if !strings.Contains(gokey, "ts%3D1234567890000") {
		t.Errorf("user-provided ts should be preserved, gokey: %s", gokey)
	}
}

func TestTracker_Track_NetworkErrorReturnsError(t *testing.T) {
	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": "http://127.0.0.1:1", "async": false})

	err := tr.Track(Event{Type: "test", Fields: map[string]string{"k": "v"}})
	if err == nil {
		t.Fatal("Track should return error on unreachable endpoint")
	}
}

func TestTracker_Track_AsyncReturnsBeforeNetworkResult(t *testing.T) {
	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": "http://127.0.0.1:1"})

	err := tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "test", "p4": "SYS"}})
	if err != nil {
		t.Fatalf("async Track should only enqueue, got error: %v", err)
	}
	if err := tr.Close(); err == nil {
		t.Fatal("Close should return async send error")
	}
}

func TestTracker_Track_ReturnsQueueFull(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.CompareAndSwapInt32(&startedOnce, 0, 1) {
			close(started)
		}
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": server.URL, "queue_size": 1})
	if err := tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "first", "p4": "SYS"}}); err != nil {
		t.Fatalf("first Track failed: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start sending first event")
	}
	if err := tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "second", "p4": "SYS"}}); err != nil {
		t.Fatalf("second Track failed: %v", err)
	}
	err := tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "third", "p4": "SYS"}})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("third Track error = %v, want ErrQueueFull", err)
	}

	close(release)
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestTracker_Track_RequiresPID(t *testing.T) {
	tr := NewTracker(Config{"endpoint": "http://127.0.0.1:1"})
	defer tr.Close()

	err := tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "test", "p4": "SYS"}})
	if err == nil || !strings.Contains(err.Error(), `"pid"`) {
		t.Fatalf("Track error = %v, want pid required error", err)
	}
}

func TestTracker_Track_RequiresEventType(t *testing.T) {
	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": "http://127.0.0.1:1"})
	defer tr.Close()

	err := tr.Track(Event{Fields: map[string]string{"k": "v"}})
	if err == nil || !strings.Contains(err.Error(), "Event.Type") {
		t.Fatalf("Track error = %v, want Event.Type required error", err)
	}
}

func TestTracker_Track_ConcurrentSafe(t *testing.T) {
	var count int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tr := NewTracker(Config{"pid": "Hp8rK6", "endpoint": server.URL})

	const N = 20
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func() {
			_ = tr.Track(Event{Type: "concurrent", Fields: map[string]string{"k": "v"}})
			done <- struct{}{}
		}()
	}
	for i := 0; i < N; i++ {
		<-done
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if got := atomic.LoadInt32(&count); got != N {
		t.Errorf("concurrent calls received %d, want %d", got, N)
	}
}

func TestTracker_Close_ReturnsNil(t *testing.T) {
	tr := NewTracker(Config{"pid": "test"})
	if err := tr.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
	if err := tr.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestTracker_Track_ReturnsErrorAfterClose(t *testing.T) {
	tr := NewTracker(Config{"pid": "test"})
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	err := tr.Track(Event{Type: "event", Fields: map[string]string{"p1": "test", "p4": "SYS"}})
	if !errors.Is(err, ErrTrackerClosed) {
		t.Fatalf("Track error = %v, want ErrTrackerClosed", err)
	}
}

func TestTracker_Config_ReturnsCopy(t *testing.T) {
	tr := NewTracker(Config{"pid": "test", "version": "1.0"})
	defer tr.Close()
	got := tr.Config()

	if got["version"] != "1.0" {
		t.Errorf("Config().version = %q, want %q", got["version"], "1.0")
	}
	got["version"] = "2.0"
	if tr.config["version"] != "1.0" {
		t.Error("Config mutated tracker internal config")
	}
}

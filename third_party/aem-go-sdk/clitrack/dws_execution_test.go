package clitrack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDWSExecutionOptOutAndReservedFields(t *testing.T) {
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv("CODEX_TEST_SECRET", "private-value")
	tr := New(Config{PID: "test", App: "dws", Endpoint: server.URL,
		NoAutomaticDimensions: true, NoAttribution: true, NoCommandLine: true, NoCwd: true,
		ExtraFields: func() map[string]string {
			return map[string]string{"p2": "injected", "p3": "injected", "c5": "incorrect override", "c9": "version", "c10": "corp-1"}
		},
	})
	if tr.ancestryCh != nil || tr.executor != "" {
		t.Fatal("disabled attribution started a probe")
	}
	when := time.UnixMilli(1700000000123)
	if err := tr.ReportExecution(Execution{Command: "dws-original", ExitCode: 3, Duration: 234 * time.Millisecond, CompletedAt: when, ErrorSummary: strings.Repeat("错", 220)}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	if err := json.Unmarshal(<-requests, &body); err != nil {
		t.Fatal(err)
	}
	outer, _ := url.QueryUnescape(body["gokey"])
	common, _ := url.ParseQuery(outer)
	fields, _ := url.ParseQuery(common.Get("msg"))
	for key, want := range map[string]string{"c1": "dws-original", "c3": "3", "c4": "234", "c9": "version", "c10": "corp-1", "ts": "1700000000123"} {
		if fields.Get(key) != want {
			t.Fatalf("%s=%q want %q", key, fields.Get(key), want)
		}
	}
	if got := fields.Get("c5"); len([]rune(got)) != 200 || !strings.HasSuffix(got, "...") {
		t.Fatalf("error summary=%q", got)
	}
	for _, key := range []string{"p2", "p3", "c2", "c6", "c7", "c8"} {
		if fields.Has(key) {
			t.Fatalf("unexpected %s", key)
		}
	}
}

func TestDWSExecutionNoopAndClosed(t *testing.T) {
	tr := New(Config{})
	if err := tr.ReportExecution(Execution{}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	tr = New(Config{PID: "test", NoAttribution: true, NoAutomaticDimensions: true})
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tr.ReportExecution(Execution{}); err == nil {
		t.Fatal("closed tracker accepted an event")
	}
}

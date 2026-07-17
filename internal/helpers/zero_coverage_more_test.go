package helpers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCrossPlatformCoverageLookupOpenDingTalkIDsByAisearchPersonCoverage(t *testing.T) {
	caller := &helpersCoreCaller{result: textToolResult(`{"result":[{"userId":"u1","openDingTalkId":"open1","name":"Alice"}]}`)}
	installHelpersCoreDeps(t, caller)
	openIDs := map[string]string{}
	names := map[string]string{}
	if err := lookupOpenDingTalkIDsByAisearchPerson(context.Background(), "Alice", openIDs, names); err != nil {
		t.Fatal(err)
	}
	if openIDs["u1"] != "open1" || names["u1"] != "Alice" {
		t.Fatalf("person mappings = %#v / %#v", openIDs, names)
	}
	caller.result = textToolResult(`{`)
	if err := lookupOpenDingTalkIDsByAisearchPerson(context.Background(), "Alice", openIDs, names); err == nil {
		t.Fatal("invalid person response succeeded")
	}
	caller.err = errors.New("search failed")
	if err := lookupOpenDingTalkIDsByAisearchPerson(context.Background(), "Alice", openIDs, names); err == nil {
		t.Fatal("person search error was ignored")
	}
}

func TestCrossPlatformCoverageOpencodeWaitHealthyCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != opencodeServerHealthPath {
			t.Fatalf("health path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"healthy":true}`))
	}))
	defer server.Close()
	client := &opencodeHTTPClient{baseURL: server.URL, httpClient: server.Client()}
	if err := (&opencodeServer{}).waitHealthy(context.Background(), client); err != nil {
		t.Fatalf("healthy server: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&opencodeServer{}).waitHealthy(cancelled, client); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v", err)
	}
	for _, doneErr := range []error{errors.New("exited"), nil} {
		done := make(chan error, 1)
		done <- doneErr
		bad := &opencodeHTTPClient{baseURL: "http://127.0.0.1:1", httpClient: http.DefaultClient}
		if err := (&opencodeServer{done: done}).waitHealthy(context.Background(), bad); err == nil {
			t.Fatal("exited server wait succeeded")
		}
	}
}

package clitrack

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBuildFieldsKeepsOrganizationDimension(t *testing.T) {
	tracker := &Tracker{
		noCommandLine: true,
		noCwd:         true,
		extraFields: func() map[string]string {
			return map[string]string{"c9": "version", "c10": "corp-1"}
		},
	}

	fields := tracker.buildFields(0, time.Millisecond, "", "")
	if fields["c9"] != "version" || fields["c10"] != "corp-1" {
		t.Fatalf("custom telemetry fields = %#v", fields)
	}
}

func TestNoFlushWaitReturnsBeforeBlockedSend(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		started <- struct{}{}
		<-release
	}))
	defer server.Close()

	tracker := New(Config{
		PID: "test", Endpoint: server.URL, NoAutomaticDimensions: true,
		NoFlushWait: true, FlushTimeout: 2 * time.Second,
	})
	returned := make(chan struct{})
	go func() {
		tracker.Run(func() error { return nil }, nil)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(500 * time.Millisecond):
		close(release)
		t.Fatal("NoFlushWait blocked command exit on telemetry delivery")
	}
	select {
	case <-started:
		close(release)
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("queued telemetry was never attempted")
	}
}

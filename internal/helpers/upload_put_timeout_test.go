package helpers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageFilePutClientTimeoutFloorAndRaise(t *testing.T) {
	testseam.Swap(t, &filePutClientTimeout, DefaultFilePutTimeout)

	SetFilePutClientTimeout(10 * time.Minute)
	if got := FilePutClientTimeout(); got != 10*time.Minute {
		t.Fatalf("raised PUT budget = %v, want 10m", got)
	}

	SetFilePutClientTimeout(30 * time.Second)
	if got := FilePutClientTimeout(); got != DefaultFilePutTimeout {
		t.Fatalf("below-floor PUT budget = %v, want default %v", got, DefaultFilePutTimeout)
	}

	SetFilePutClientTimeout(DefaultFilePutTimeout)
	if got := FilePutClientTimeout(); got != DefaultFilePutTimeout {
		t.Fatalf("default PUT budget = %v, want %v", got, DefaultFilePutTimeout)
	}
}

func TestCrossPlatformCoverageFilePutFlowTimeoutFollowsBudget(t *testing.T) {
	testseam.Swap(t, &filePutClientTimeout, DefaultFilePutTimeout)

	if got := filePutFlowTimeout(); got != 10*time.Minute {
		t.Fatalf("default flow budget = %v, want 10m", got)
	}

	// Raised budgets keep the historical 5-minute RPC headroom beyond the PUT.
	testseam.Swap(t, &filePutClientTimeout, time.Hour)
	if got := filePutFlowTimeout(); got != 65*time.Minute {
		t.Fatalf("raised flow budget = %v, want 65m", got)
	}

	if got := mailHTTPClient().Timeout; got != time.Hour {
		t.Fatalf("mail attachment client timeout = %v, want raised budget", got)
	}
	if got := newFloatImageUploadClient().Timeout; got != time.Hour {
		t.Fatalf("float image client timeout = %v, want raised budget", got)
	}
}

func TestCrossPlatformCoverageDefaultHTTPPutFileUsesTransferBudget(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()

	file := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(file, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := defaultHTTPPutFile(context.Background(), slow.URL, nil, file, 7); err != nil {
		t.Fatal(err)
	}

	testseam.Swap(t, &filePutClientTimeout, 50*time.Millisecond)
	if err := defaultHTTPPutFile(context.Background(), slow.URL, nil, file, 7); err == nil {
		t.Fatal("short transfer budget must abort the direct PUT")
	}
}

func TestCrossPlatformCoverageDefaultPushPutOpenedFileUsesTransferBudget(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()

	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := defaultPushPutOpenedFile(context.Background(), slow.URL, nil, file, 7); err != nil {
		t.Fatal(err)
	}

	testseam.Swap(t, &filePutClientTimeout, 50*time.Millisecond)
	if err := defaultPushPutOpenedFile(context.Background(), slow.URL, nil, file, 7); err == nil {
		t.Fatal("short transfer budget must abort the push PUT")
	}
}

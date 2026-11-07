//go:build windows

package helpers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestCrossPlatformCoverageFixtureCleanupWaitsForSharingHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.exe")
	if err := os.WriteFile(path, []byte("locked fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err == nil {
		windows.CloseHandle(handle)
		t.Fatal("exclusive handle unexpectedly allowed deletion")
	}
	closed := make(chan error, 1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		closed <- windows.CloseHandle(handle)
	}()
	err = retryHelpersFixtureCleanup(func() error { return os.Remove(path) })
	if closeErr := <-closed; closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatalf("cleanup did not recover after handle release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("fixture remains after cleanup: %v", err)
	}
}

//go:build windows

package helpers

import (
	"testing"
	"time"
)

func TestCrossPlatformCoverageProcessAliveExitedChildWithRetainedHandle(t *testing.T) {
	child := nativeExitCommand(t)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()

	pid := child.Process.Pid
	deadline := time.Now().Add(2 * time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processAlive(pid) {
		t.Fatalf("exited process %d was reported live while its handle was retained", pid)
	}
}

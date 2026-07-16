package helpers

import (
	"os"
	"os/exec"
	"testing"
)

func TestCrossPlatformCoverageProcessAlivePlatformEdges(t *testing.T) {
	if processAlive(0) {
		t.Fatal("pid 0 must not be treated as a managed live process")
	}
	if !processAlive(os.Getpid()) {
		t.Fatal("current process was reported dead")
	}

	exited := nativeExitCommand(t)
	if err := exited.Start(); err != nil {
		t.Fatal(err)
	}
	pid := exited.Process.Pid
	if err := exited.Wait(); err != nil {
		t.Fatal(err)
	}
	if processAlive(pid) {
		t.Fatalf("exited process %d was reported live", pid)
	}
}

func nativeExitCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exec.Command(executable, "-test.run=^$")
}

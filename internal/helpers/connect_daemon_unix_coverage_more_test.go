//go:build darwin || linux

package helpers

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageDaemonUnixStatusAndStopEdges(t *testing.T) {
	t.Run("stale supervisor stops orphan", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		helperSleep = func(time.Duration) {}
		worker := exec.Command("sh", "-c", "sleep 5")
		if err := worker.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { _ = worker.Wait(); close(done) }()
		dir, _ := connectDaemonDir("orphan")
		deadSupervisorPID := deadPid(t)
		if err := writeDaemonState(dir, daemonState{Pid: deadSupervisorPID, DirKey: "orphan"}); err != nil {
			t.Fatal(err)
		}
		seedHeartbeat(t, "orphan", connectHeartbeat{Pid: worker.Process.Pid})
		aliveChecks := 0
		daemonProcessAlive = func(pid int) bool {
			if pid == deadSupervisorPID {
				return false
			}
			aliveChecks++
			return aliveChecks < 3
		}
		var out bytes.Buffer
		if err := daemonStop(&out, "orphan"); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = worker.Process.Kill()
			t.Fatal("orphan worker did not stop")
		}
	})

	t.Run("stale supervisor force kills orphan", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		dir, _ := connectDaemonDir("orphan-force")
		if err := writeDaemonState(dir, daemonState{Pid: 999, DirKey: "orphan-force"}); err != nil {
			t.Fatal(err)
		}
		seedHeartbeat(t, "orphan-force", connectHeartbeat{Pid: 123})
		daemonProcessAlive = func(pid int) bool { return pid == 123 }
		daemonFindProcess = func(int) (*os.Process, error) { return os.FindProcess(-1) }
		base := time.Now()
		var calls int
		daemonNow = func() time.Time {
			calls++
			return base.Add(time.Duration(calls) * (daemonStopTimeout + time.Second))
		}
		helperSleep = func(time.Duration) {}
		if err := daemonStop(&bytes.Buffer{}, "orphan-force"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("live graceful stop", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		worker := exec.Command("sh", "-c", "sleep 5")
		if err := worker.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { _ = worker.Wait() }()
		dir, _ := connectDaemonDir("live")
		if err := writeDaemonState(dir, daemonState{Pid: worker.Process.Pid, DirKey: "live"}); err != nil {
			t.Fatal(err)
		}
		var aliveCalls int
		daemonProcessAlive = func(int) bool {
			aliveCalls++
			return aliveCalls == 1
		}
		if err := daemonStop(&bytes.Buffer{}, "live"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("signal error", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		dir, _ := connectDaemonDir("signal-error")
		if err := writeDaemonState(dir, daemonState{Pid: 456, DirKey: "signal-error"}); err != nil {
			t.Fatal(err)
		}
		daemonProcessAlive = func(int) bool { return true }
		daemonFindProcess = func(int) (*os.Process, error) { return os.FindProcess(-1) }
		if err := daemonStop(&bytes.Buffer{}, "signal-error"); err == nil {
			t.Fatal("signal process error was ignored")
		}
	})

	t.Run("live force kill", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		worker := exec.Command("sh", "-c", "trap '' TERM; sleep 5")
		if err := worker.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
		done := make(chan struct{})
		go func() { _ = worker.Wait(); close(done) }()
		dir, _ := connectDaemonDir("force")
		if err := writeDaemonState(dir, daemonState{Pid: worker.Process.Pid, DirKey: "force"}); err != nil {
			t.Fatal(err)
		}
		base := time.Now()
		advanced := false
		daemonNow = func() time.Time {
			if advanced {
				return base.Add(daemonStopTimeout + time.Second)
			}
			return base
		}
		helperSleep = func(time.Duration) { advanced = true }
		var out bytes.Buffer
		if err := daemonStop(&out, "force"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "SIGKILL") {
			t.Fatalf("force stop output = %q", out.String())
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = worker.Process.Kill()
			t.Fatal("forced worker did not stop")
		}
	})
}

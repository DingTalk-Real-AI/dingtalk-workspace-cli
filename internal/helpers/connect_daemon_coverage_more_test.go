package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func preserveDaemonHooks(t *testing.T) {
	t.Helper()
	oldDetach := daemonDetachEnabled
	oldExecutable := daemonExecutable
	oldCommand := daemonCommand
	oldNow := daemonNow
	oldCreateTemp := daemonCreateTemp
	oldFileChmod := daemonFileChmod
	oldCopy := daemonCopy
	oldFileSync := daemonFileSync
	oldFileClose := daemonFileClose
	oldRename := daemonRename
	oldFindProcess := daemonFindProcess
	oldProcessAlive := daemonProcessAlive
	oldSignalProcess := daemonSignalProcess
	oldSignalContext := daemonSignalContext
	oldReadDir := connectHealthReadDir
	oldDir := connectDaemonDirOverride
	oldAfter := helperAfter
	oldSleep := helperSleep
	t.Cleanup(func() {
		daemonDetachEnabled = oldDetach
		daemonExecutable = oldExecutable
		daemonCommand = oldCommand
		daemonNow = oldNow
		daemonCreateTemp = oldCreateTemp
		daemonFileChmod = oldFileChmod
		daemonCopy = oldCopy
		daemonFileSync = oldFileSync
		daemonFileClose = oldFileClose
		daemonRename = oldRename
		daemonFindProcess = oldFindProcess
		daemonProcessAlive = oldProcessAlive
		daemonSignalProcess = oldSignalProcess
		daemonSignalContext = oldSignalContext
		connectHealthReadDir = oldReadDir
		connectDaemonDirOverride = oldDir
		helperAfter = oldAfter
		helperSleep = oldSleep
	})
	// Tests exercise the platform-independent lifecycle below the public
	// Windows guard. The dedicated unsupported case resets this to false.
	daemonDetachEnabled = true
}

func daemonTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "connect"}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func instantAfter(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Now()
	return ch
}

func TestCrossPlatformCoverageStartDaemonLifecycleEdges(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		preserveDaemonHooks(t)
		daemonDetachEnabled = false
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("unsupported daemon start succeeded")
		}
	})

	t.Run("directory error", func(t *testing.T) {
		preserveDaemonHooks(t)
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		connectDaemonDirOverride = blocked
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("daemon start with blocked directory succeeded")
		}
	})

	t.Run("already running", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		dir, err := connectDaemonDir("key")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeDaemonState(dir, daemonState{Pid: os.Getpid(), DirKey: "key"}); err != nil {
			t.Fatal(err)
		}
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("duplicate daemon start succeeded")
		}
	})

	t.Run("executable error", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		daemonExecutable = func() (string, error) { return "", errors.New("executable") }
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("daemon start without executable succeeded")
		}
	})

	t.Run("stage error", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		daemonExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "missing"), nil }
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("daemon start with missing source succeeded")
		}
	})

	t.Run("log error", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		dir, err := connectDaemonDir("key")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(daemonLogPath(dir), 0o700); err != nil {
			t.Fatal(err)
		}
		daemonExecutable = os.Executable
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("daemon start with directory log succeeded")
		}
	})

	t.Run("child start error", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		daemonExecutable = os.Executable
		daemonCommand = func(string, ...string) *exec.Cmd {
			return exec.Command(filepath.Join(t.TempDir(), "missing"))
		}
		if err := startDaemon(daemonTestCommand(), "key", "client", "", "custom", "", "", false); err == nil {
			t.Fatal("daemon start with invalid command succeeded")
		}
	})

	t.Run("success", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		daemonExecutable = os.Executable
		fixture := writeShellExecutable(t, t.TempDir(), "daemon-success", "exit 0\n")
		daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command(fixture) }
		cmd := daemonTestCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := startDaemon(cmd, "key", "client", "app", "custom", "staff", "profile", true); err != nil {
			t.Fatalf("startDaemon() error = %v", err)
		}
		if !strings.Contains(out.String(), "daemon started") {
			t.Fatalf("start output = %q", out.String())
		}

		// startDaemon intentionally releases its detached child. On Windows the
		// child keeps daemon.log locked until it exits, so wait for that handle
		// to close before TempDir cleanup.
		dir, err := connectDaemonDir("key")
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			err = os.Remove(daemonLogPath(dir))
			if err == nil || os.IsNotExist(err) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("daemon log remained locked: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func TestCrossPlatformCoverageDaemonFileOperationEdges(t *testing.T) {
	t.Run("default config directory", func(t *testing.T) {
		preserveDaemonHooks(t)
		t.Setenv("HOME", t.TempDir())
		connectDaemonDirOverride = ""
		if _, err := connectDaemonDir("default"); err != nil {
			t.Fatal(err)
		}
	})

	for _, tc := range []struct {
		name      string
		configure func()
	}{
		{"create temp", func() {
			daemonCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("create") }
		}},
		{"chmod", func() {
			daemonFileChmod = func(*os.File, os.FileMode) error { return errors.New("chmod") }
		}},
		{"copy", func() {
			daemonCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
		}},
		{"sync", func() {
			daemonFileSync = func(*os.File) error { return errors.New("sync") }
		}},
		{"close", func() {
			daemonFileClose = func(file *os.File) error {
				_ = file.Close()
				return errors.New("close")
			}
		}},
		{"rename", func() {
			daemonRename = func(string, string) error { return errors.New("rename") }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveDaemonHooks(t)
			dir := t.TempDir()
			src := filepath.Join(dir, "source")
			if err := os.WriteFile(src, []byte("binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			tc.configure()
			if _, err := stageDaemonExecutable(src, dir); err == nil {
				t.Fatalf("stage with %s failure succeeded", tc.name)
			}
		})
	}

	t.Run("state write and rename errors", func(t *testing.T) {
		preserveDaemonHooks(t)
		if err := writeDaemonState(filepath.Join(t.TempDir(), "missing"), daemonState{}); err == nil {
			t.Fatal("state write to missing directory succeeded")
		}
		dir := t.TempDir()
		daemonRename = func(string, string) error { return errors.New("rename") }
		if err := writeDaemonState(dir, daemonState{}); err == nil {
			t.Fatal("state rename failure succeeded")
		}
	})

	t.Run("state read non-not-exist error", func(t *testing.T) {
		preserveDaemonHooks(t)
		dir := t.TempDir()
		if err := os.Mkdir(daemonStatePath(dir), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := readDaemonState(dir); err == nil {
			t.Fatal("reading directory as state succeeded")
		}
	})

	if backoffDelay(1, 2*time.Second, time.Second) != time.Second {
		t.Fatal("backoff base larger than cap was not capped")
	}
	if statusHintArgs("client", "app-id") != " --robot-client-id client" ||
		statusHintArgs("", "app-id") != " --unified-app-id id" || statusHintArgs("", "plain") != "" {
		t.Fatal("status hint variants mismatch")
	}
	workerArgs := buildWorkerArgs([]string{"keep", "--daemon=true", "--daemon-supervise=false", "--daemon-worker=true"})
	if strings.Contains(strings.Join(workerArgs, " "), "=true") || workerArgs[0] != "keep" {
		t.Fatalf("worker args with assigned daemon flags = %#v", workerArgs)
	}
}

func TestCrossPlatformCoverageRunSupervisorLifecycleEdges(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		preserveDaemonHooks(t)
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "")
		if err := runSupervisor(daemonTestCommand()); err == nil {
			t.Fatal("supervisor without key succeeded")
		}
	})

	t.Run("directory error", func(t *testing.T) {
		preserveDaemonHooks(t)
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", blocked)
		if err := runSupervisor(daemonTestCommand()); err == nil {
			t.Fatal("supervisor with blocked directory succeeded")
		}
	})

	t.Run("state write error", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		connectDaemonDirOverride = base
		dir, err := connectDaemonDir("key")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(daemonStatePath(dir), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		if err := runSupervisor(daemonTestCommand()); err == nil {
			t.Fatal("supervisor with unwritable state succeeded")
		}
	})

	t.Run("executable error", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		daemonExecutable = func() (string, error) { return "", errors.New("executable") }
		if err := runSupervisor(daemonTestCommand()); err == nil {
			t.Fatal("supervisor without executable succeeded")
		}
	})

	t.Run("cancel before worker", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		daemonSignalContext = func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, func() {}
		}
		if err := runSupervisor(daemonTestCommand()); err != nil {
			t.Fatalf("cancelled supervisor = %v", err)
		}
	})

	t.Run("single worker without always-on", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		t.Setenv("DWS_CONNECT_DAEMON_CLIENTID", "client")
		t.Setenv("DWS_CONNECT_DAEMON_UNIFIEDAPPID", "app")
		t.Setenv("DWS_CONNECT_DAEMON_CHANNEL", "custom")
		t.Setenv("DWS_CONNECT_DAEMON_PROFILE", "profile")
		t.Setenv("DWS_CONNECT_DAEMON_ALWAYSON", "")
		daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "exit 0") }
		if err := runSupervisor(daemonTestCommand()); err != nil {
			t.Fatalf("single-worker supervisor = %v", err)
		}
	})

	t.Run("repeated start failures", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		t.Setenv("DWS_CONNECT_DAEMON_ALWAYSON", "true")
		daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command(filepath.Join(base, "missing")) }
		helperAfter = instantAfter
		if err := runSupervisor(daemonTestCommand()); err == nil {
			t.Fatal("supervisor did not give up after start failures")
		}
	})

	t.Run("repeated fast crashes", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		t.Setenv("DWS_CONNECT_DAEMON_ALWAYSON", "true")
		daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "exit 1") }
		helperAfter = instantAfter
		if err := runSupervisor(daemonTestCommand()); err == nil {
			t.Fatal("supervisor did not give up after fast crashes")
		}
	})

	t.Run("healthy crash resets failures then cancellation", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		t.Setenv("DWS_CONNECT_DAEMON_ALWAYSON", "true")
		secondWorker := make(chan struct{})
		var workers int
		daemonCommand = func(string, ...string) *exec.Cmd {
			workers++
			if workers == 1 {
				return exec.Command("sh", "-c", "exit 0")
			}
			select {
			case <-secondWorker:
			default:
				close(secondWorker)
			}
			return exec.Command("sh", "-c", "sleep 5")
		}
		baseTime := time.Now()
		var tick int
		daemonNow = func() time.Time {
			tick++
			if tick >= 3 {
				return baseTime.Add(daemonHealthyAfter)
			}
			return baseTime
		}
		daemonSignalContext = func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				<-secondWorker
				cancel()
			}()
			return ctx, cancel
		}
		if err := runSupervisor(daemonTestCommand()); err != nil {
			t.Fatalf("healthy cancellation supervisor = %v", err)
		}
	})

	t.Run("cancel during backoff", func(t *testing.T) {
		preserveDaemonHooks(t)
		base := t.TempDir()
		t.Setenv("DWS_CONNECT_DAEMON_DIRKEY", "key")
		t.Setenv("DWS_CONNECT_DAEMON_DIR", base)
		t.Setenv("DWS_CONNECT_DAEMON_ALWAYSON", "true")
		daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "exit 1") }
		never := make(chan time.Time)
		enteredBackoff := make(chan struct{})
		helperAfter = func(time.Duration) <-chan time.Time {
			select {
			case <-enteredBackoff:
			default:
				close(enteredBackoff)
			}
			return never
		}
		daemonSignalContext = func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				<-enteredBackoff
				cancel()
			}()
			return ctx, cancel
		}
		if err := runSupervisor(daemonTestCommand()); err != nil {
			t.Fatalf("backoff cancellation supervisor = %v", err)
		}
	})
}

func TestCrossPlatformCoverageSuperviseWaitForcedKill(t *testing.T) {
	preserveDaemonHooks(t)
	worker := exec.Command("sh", "-c", "trap '' TERM; sleep 5")
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	helperAfter = instantAfter
	_ = superviseWait(ctx, worker)
}

func TestCrossPlatformCoverageDaemonNotifyStateChangeEdges(t *testing.T) {
	preserveDaemonHooks(t)
	daemonExecutable = func() (string, error) { return "", errors.New("missing") }
	daemonNotifyStateChange("staff", "custom", "client", "started", "")

	daemonExecutable = func() (string, error) { return "/bin/sh", nil }
	done := make(chan struct{}, 4)
	daemonCommand = func(string, ...string) *exec.Cmd {
		done <- struct{}{}
		return exec.Command("sh", "-c", "exit 0")
	}
	for _, event := range []string{"started", "stopped", "crashed", "gave_up"} {
		daemonNotifyStateChange("staff", "custom", "client", event, "detail")
	}
	for range 4 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("notification subprocess was not created")
		}
	}
	time.Sleep(50 * time.Millisecond)
}

func TestCrossPlatformCoverageDaemonStatusAndStopEdges(t *testing.T) {
	t.Run("status directory and corrupt files", func(t *testing.T) {
		preserveDaemonHooks(t)
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		connectDaemonDirOverride = blocked
		if err := daemonStatus(&bytes.Buffer{}, "key", false); err == nil {
			t.Fatal("status with blocked directory succeeded")
		}

		connectDaemonDirOverride = t.TempDir()
		dir, _ := connectDaemonDir("corrupt-state")
		if err := os.WriteFile(daemonStatePath(dir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := daemonStatus(&bytes.Buffer{}, "corrupt-state", false); err == nil {
			t.Fatal("status with corrupt state succeeded")
		}

		dir, _ = connectDaemonDir("corrupt-heartbeat")
		if err := os.WriteFile(connectHeartbeatPath(dir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := daemonStatus(&bytes.Buffer{}, "corrupt-heartbeat", false); err == nil {
			t.Fatal("status with corrupt heartbeat succeeded")
		}
	})

	t.Run("status detailed plain and json", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		base := time.Now()
		daemonNow = func() time.Time { return base }
		dir, _ := connectDaemonDir("detailed")
		if err := writeDaemonState(dir, daemonState{Pid: os.Getpid(), DirKey: "detailed"}); err != nil {
			t.Fatal(err)
		}
		seedHeartbeat(t, "detailed", connectHeartbeat{
			Pid: os.Getpid(), ClientID: "client", Channel: "custom",
			StartUnix: base.Add(-time.Minute).Unix(), ConnectedUnix: base.Add(-time.Minute).Unix(),
			LastPushUnix: base.Add(-time.Second).Unix(), LastError: "last error",
		})
		var plain bytes.Buffer
		if err := daemonStatus(&plain, "detailed", false); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"state", "detail", "pid", "channel", "client", "uptime", "recv", "error", "logs"} {
			if !strings.Contains(plain.String(), want) {
				t.Errorf("plain status missing %q: %s", want, plain.String())
			}
		}
		var jsonOut bytes.Buffer
		if err := daemonStatus(&jsonOut, "detailed", true); err != nil || !strings.Contains(jsonOut.String(), `"state"`) {
			t.Fatalf("json status = %q, %v", jsonOut.String(), err)
		}
	})

	t.Run("stop directory and corrupt state", func(t *testing.T) {
		preserveDaemonHooks(t)
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		connectDaemonDirOverride = blocked
		if err := daemonStop(&bytes.Buffer{}, "key"); err == nil {
			t.Fatal("stop with blocked directory succeeded")
		}
		connectDaemonDirOverride = t.TempDir()
		dir, _ := connectDaemonDir("corrupt")
		if err := os.WriteFile(daemonStatePath(dir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := daemonStop(&bytes.Buffer{}, "corrupt"); err == nil {
			t.Fatal("stop with corrupt state succeeded")
		}
	})

	t.Run("find error", func(t *testing.T) {
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		dir, _ := connectDaemonDir("find-error")
		if err := writeDaemonState(dir, daemonState{Pid: 123, DirKey: "find-error"}); err != nil {
			t.Fatal(err)
		}
		daemonProcessAlive = func(int) bool { return true }
		daemonFindProcess = func(int) (*os.Process, error) { return nil, errors.New("find") }
		if err := daemonStop(&bytes.Buffer{}, "find-error"); err == nil {
			t.Fatal("find process error was ignored")
		}
	})
}

func TestCrossPlatformCoverageDaemonStopHookedLifecycleEdges(t *testing.T) {
	const (
		supervisorPID = 101
		workerPID     = 202
	)

	prepare := func(t *testing.T, key string, withWorker bool) {
		t.Helper()
		preserveDaemonHooks(t)
		connectDaemonDirOverride = t.TempDir()
		dir, err := connectDaemonDir(key)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeDaemonState(dir, daemonState{Pid: supervisorPID, DirKey: key}); err != nil {
			t.Fatal(err)
		}
		if withWorker {
			seedHeartbeat(t, key, connectHeartbeat{Pid: workerPID})
		}
		daemonFindProcess = func(int) (*os.Process, error) { return new(os.Process), nil }
	}

	t.Run("orphan worker exits gracefully", func(t *testing.T) {
		prepare(t, "orphan-graceful", true)
		workerChecks := 0
		daemonProcessAlive = func(pid int) bool {
			if pid == supervisorPID {
				return false
			}
			workerChecks++
			return workerChecks <= 2
		}
		base := time.Now()
		daemonNow = func() time.Time { return base }
		var signals []os.Signal
		daemonSignalProcess = func(_ *os.Process, signal os.Signal) error {
			signals = append(signals, signal)
			return nil
		}
		sleeps := 0
		helperSleep = func(time.Duration) { sleeps++ }
		if err := daemonStop(&bytes.Buffer{}, "orphan-graceful"); err != nil {
			t.Fatal(err)
		}
		if len(signals) != 1 || signals[0] != syscall.SIGTERM || sleeps != 1 {
			t.Fatalf("signals=%v sleeps=%d", signals, sleeps)
		}
	})

	t.Run("orphan worker is force killed", func(t *testing.T) {
		prepare(t, "orphan-force-hooked", true)
		daemonProcessAlive = func(pid int) bool { return pid == workerPID }
		base := time.Now()
		nowCalls := 0
		daemonNow = func() time.Time {
			nowCalls++
			if nowCalls <= 2 {
				return base
			}
			return base.Add(daemonStopTimeout + time.Second)
		}
		var signals []os.Signal
		daemonSignalProcess = func(_ *os.Process, signal os.Signal) error {
			signals = append(signals, signal)
			return nil
		}
		helperSleep = func(time.Duration) {}
		if err := daemonStop(&bytes.Buffer{}, "orphan-force-hooked"); err != nil {
			t.Fatal(err)
		}
		if len(signals) != 2 || signals[0] != syscall.SIGTERM || signals[1] != syscall.SIGKILL {
			t.Fatalf("signals=%v", signals)
		}
	})

	t.Run("live supervisor exits gracefully", func(t *testing.T) {
		prepare(t, "live-graceful-hooked", false)
		aliveCalls := 0
		daemonProcessAlive = func(int) bool {
			aliveCalls++
			return aliveCalls == 1
		}
		base := time.Now()
		daemonNow = func() time.Time { return base }
		daemonSignalProcess = func(*os.Process, os.Signal) error { return nil }
		if err := daemonStop(&bytes.Buffer{}, "live-graceful-hooked"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("live supervisor signal fails", func(t *testing.T) {
		prepare(t, "live-signal-error-hooked", false)
		daemonProcessAlive = func(int) bool { return true }
		daemonSignalProcess = func(*os.Process, os.Signal) error { return errors.New("signal") }
		if err := daemonStop(&bytes.Buffer{}, "live-signal-error-hooked"); err == nil {
			t.Fatal("signal error was ignored")
		}
	})

	t.Run("live supervisor is force killed", func(t *testing.T) {
		prepare(t, "live-force-hooked", false)
		daemonProcessAlive = func(int) bool { return true }
		base := time.Now()
		nowCalls := 0
		daemonNow = func() time.Time {
			nowCalls++
			if nowCalls <= 2 {
				return base
			}
			return base.Add(daemonStopTimeout + time.Second)
		}
		var signals []os.Signal
		daemonSignalProcess = func(_ *os.Process, signal os.Signal) error {
			signals = append(signals, signal)
			return nil
		}
		helperSleep = func(time.Duration) {}
		if err := daemonStop(&bytes.Buffer{}, "live-force-hooked"); err != nil {
			t.Fatal(err)
		}
		if len(signals) != 2 || signals[0] != syscall.SIGTERM || signals[1] != syscall.SIGKILL {
			t.Fatalf("signals=%v", signals)
		}
	})
}

type daemonSequenceRunner struct {
	responses []map[string]any
	calls     int
}

func (r *daemonSequenceRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	index := r.calls
	r.calls++
	response := map[string]any{}
	if index < len(r.responses) {
		response = r.responses[index]
	}
	return executor.Result{Invocation: invocation, Response: response}, nil
}

func TestCrossPlatformCoverageDaemonListAndNamePaginationEdges(t *testing.T) {
	preserveDaemonHooks(t)
	connectDaemonDirOverride = t.TempDir()
	cmd := &cobra.Command{Use: "list"}

	runner := &daemonSequenceRunner{responses: []map[string]any{
		{"items": []any{map[string]any{"id": "u-1", "appName": "App One"}}, "hasMore": true, "nextCursor": "next"},
		{"items": []any{map[string]any{"unifiedAppId": "u-2", "name": "App Two"}}, "hasMore": false},
	}}
	names, err := devAppNameMap(cmd, runner)
	if err != nil || names["u-1"] != "App One" || names["u-2"] != "App Two" || runner.calls != 2 {
		t.Fatalf("paginated names = %#v calls=%d err=%v", names, runner.calls, err)
	}

	runner = &daemonSequenceRunner{responses: []map[string]any{{"hasMore": true}}}
	if _, err := devAppNameMap(cmd, runner); err != nil || runner.calls != 1 {
		t.Fatalf("empty cursor pagination calls=%d err=%v", runner.calls, err)
	}

	reports := []connectHealthReport{{UnifiedAppID: "u-1"}, {UnifiedAppID: "missing"}, {ClientID: "client"}}
	runner = &daemonSequenceRunner{responses: []map[string]any{{
		"items": []any{map[string]any{"unifiedAppId": "u-1", "name": "Resolved"}}, "hasMore": false,
	}}}
	resolveAppNames(cmd, runner, reports)
	if reports[0].AppName != "Resolved" || reports[1].AppName != "" {
		t.Fatalf("resolved reports = %#v", reports)
	}
	resolveAppNames(cmd, connectResponseRunner{err: errors.New("offline")}, []connectHealthReport{{UnifiedAppID: "u-1"}})

	list := newDevAppRobotConnectListCommand(runner)
	var out bytes.Buffer
	list.SetOut(&out)
	if err := list.Execute(); err != nil || !strings.Contains(out.String(), "no connectors") {
		t.Fatalf("empty list = %q, %v", out.String(), err)
	}
	list = newDevAppRobotConnectListCommand(runner)
	out.Reset()
	list.SetOut(&out)
	list.SetArgs([]string{"--json"})
	if err := list.Execute(); err != nil || !strings.Contains(out.String(), "null") {
		t.Fatalf("json list = %q, %v", out.String(), err)
	}

	seedHeartbeat(t, "listed", connectHeartbeat{
		Pid: os.Getpid(), ClientID: strings.Repeat("c", 80), Channel: strings.Repeat("x", 80),
		StartUnix: time.Now().Add(-time.Minute).Unix(), ConnectedUnix: time.Now().Add(-time.Minute).Unix(),
	})
	list = newDevAppRobotConnectListCommand(runner)
	out.Reset()
	list.SetOut(&out)
	if err := list.Execute(); err != nil || !strings.Contains(out.String(), "STATE") {
		t.Fatalf("table list = %q, %v", out.String(), err)
	}

	connectHealthReadDir = func(string) ([]os.DirEntry, error) {
		return nil, errors.New("read directory")
	}
	list = newDevAppRobotConnectListCommand(runner)
	list.SetOut(&bytes.Buffer{})
	if err := list.Execute(); err == nil {
		t.Fatal("list with blocked directory succeeded")
	}
}

func TestCrossPlatformCoverageDaemonControlCommandEdges(t *testing.T) {
	preserveDaemonHooks(t)
	connectDaemonDirOverride = t.TempDir()
	baseDir := connectDaemonDirOverride
	defaultProcessAlive := daemonProcessAlive
	defaultFindProcess := daemonFindProcess

	for _, command := range []*cobra.Command{newDevAppRobotConnectStatusCommand(), newDevAppRobotConnectStopCommand(), newDevAppRobotConnectRestartCommand()} {
		command.SetArgs(nil)
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		if err := command.Execute(); err == nil {
			t.Errorf("%s without identity succeeded", command.Name())
		}
	}

	status := newDevAppRobotConnectStatusCommand()
	status.SetArgs([]string{"--robot-client-id", "missing", "--json"})
	status.SetOut(&bytes.Buffer{})
	if err := status.Execute(); err != nil {
		t.Fatalf("status command = %v", err)
	}
	stop := newDevAppRobotConnectStopCommand()
	stop.SetArgs([]string{"--unified-app-id", "missing"})
	stop.SetOut(&bytes.Buffer{})
	if err := stop.Execute(); err != nil {
		t.Fatalf("stop command = %v", err)
	}

	restart := newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "missing"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err == nil {
		t.Fatal("restart without state succeeded")
	}

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	connectDaemonDirOverride = blocked
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "blocked"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err == nil {
		t.Fatal("restart with blocked directory succeeded")
	}
	connectDaemonDirOverride = baseDir
	corruptDir, _ := connectDaemonDir("corrupt-restart")
	if err := os.WriteFile(daemonStatePath(corruptDir), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "corrupt-restart"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err == nil {
		t.Fatal("restart with corrupt state succeeded")
	}

	dir, _ := connectDaemonDir("no-unified")
	if err := writeDaemonState(dir, daemonState{Pid: deadPid(t), DirKey: "no-unified"}); err != nil {
		t.Fatal(err)
	}
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "no-unified"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err == nil {
		t.Fatal("restart without unified app ID succeeded")
	}

	dir, _ = connectDaemonDir("restart")
	state := daemonState{Pid: deadPid(t), DirKey: "restart", UnifiedAppID: "app", Channel: "custom", NotifyStaffID: "staff", Profile: "saved", AlwaysOn: true}
	if err := writeDaemonState(dir, state); err != nil {
		t.Fatal(err)
	}
	daemonExecutable = func() (string, error) { return "/bin/sh", nil }
	daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "exit 0") }
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "restart"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err != nil {
		t.Fatalf("restart command = %v", err)
	}

	if err := writeDaemonState(dir, state); err != nil {
		t.Fatal(err)
	}
	daemonProcessAlive = func(int) bool { return true }
	daemonFindProcess = func(int) (*os.Process, error) { return nil, errors.New("find") }
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "restart"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err != nil {
		t.Fatalf("restart after stop warning = %v", err)
	}
	daemonProcessAlive = defaultProcessAlive
	daemonFindProcess = defaultFindProcess

	if err := writeDaemonState(dir, state); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("profile", "", "")
	restart = newDevAppRobotConnectRestartCommand()
	root.AddCommand(restart)
	root.SetArgs([]string{"restart", "--robot-client-id", "restart", "--profile", "override"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatalf("restart with profile override = %v", err)
	}

	if err := writeDaemonState(dir, state); err != nil {
		t.Fatal(err)
	}
	daemonExecutable = func() (string, error) { return "", errors.New("missing") }
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "restart"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err == nil {
		t.Fatal("restart without executable succeeded")
	}

	daemonExecutable = func() (string, error) { return "/bin/sh", nil }
	daemonCommand = func(string, ...string) *exec.Cmd { return exec.Command("sh", "-c", "exit 1") }
	if err := writeDaemonState(dir, state); err != nil {
		t.Fatal(err)
	}
	restart = newDevAppRobotConnectRestartCommand()
	restart.SetArgs([]string{"--robot-client-id", "restart"})
	restart.SetOut(&bytes.Buffer{})
	restart.SetErr(&bytes.Buffer{})
	if err := restart.Execute(); err == nil {
		t.Fatal("failing restart subprocess succeeded")
	}
}

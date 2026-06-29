// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/spf13/cobra"
)

// connect_daemon turns the foreground `robot connect` connector into a 7x24
// background service. Three responsibilities live here and nowhere else (the
// forwarding/session/knowledge logic in devapp_connect.go and connect_stream.go
// is untouched):
//
//  1. detach: `connect --daemon` re-execs dws in supervisor mode in a new
//     session (POSIX setsid), prints pid + log path, and exits.
//  2. supervise: the supervisor process runs the real connector as a worker
//     child and restarts it with exponential backoff when it crashes.
//  3. status/stop: read the daemon pid file, probe liveness (reusing
//     processAlive from connect_lock.go), and signal a graceful stop.
//
// Two hidden internal flags select the mode of a re-exec:
//
//	--daemon-supervise : run the supervisor loop (set by the --daemon parent)
//	--daemon-worker    : run a single foreground connector (set by the supervisor)
const (
	daemonSuperviseFlag = "daemon-supervise"
	daemonWorkerFlag    = "daemon-worker"
	daemonFlag          = "daemon"
)

// daemonState is the JSON persisted to the daemon pid file. It records the
// supervisor pid plus enough context for `status` to report without re-deriving
// it (start time for uptime, log path, the dir key it was filed under).
type daemonState struct {
	Pid       int    `json:"pid"`
	StartUnix int64  `json:"startUnix"`
	LogPath   string `json:"logPath"`
	DirKey    string `json:"dirKey"`
	ClientID  string `json:"clientId,omitempty"`
}

// connectDaemonDirOverride lets tests redirect the per-client daemon directory
// away from the real ~/.dws tree. Empty means use config.DefaultConfigDir.
var connectDaemonDirOverride string

// daemonDirKey derives a filesystem-safe directory key identifying a connector.
// Priority: clientId (the robot's AppKey, the natural identity) > unifiedAppID.
// Reuses sanitizeLockID (connect_lock.go) so the key matches the lock naming
// convention. Returns "" when neither is available.
func daemonDirKey(clientID, unifiedAppID string) string {
	if v := strings.TrimSpace(clientID); v != "" {
		return sanitizeLockID(v)
	}
	if v := strings.TrimSpace(unifiedAppID); v != "" {
		return "app-" + sanitizeLockID(v)
	}
	return ""
}

// connectDaemonDir returns <configDir>/connect/<dirKey>, creating it. This holds
// daemon.pid and daemon.log for one connector.
func connectDaemonDir(dirKey string) (string, error) {
	base := connectDaemonDirOverride
	if base == "" {
		base = config.DefaultConfigDir()
	}
	dir := filepath.Join(base, "connect", dirKey)
	if err := os.MkdirAll(dir, config.DirPerm); err != nil {
		return "", err
	}
	return dir, nil
}

func daemonPidPath(dir string) string { return filepath.Join(dir, "daemon.pid") }
func daemonLogPath(dir string) string { return filepath.Join(dir, "daemon.log") }

// writeDaemonState atomically persists the daemon pid file (write temp + rename)
// so a reader never sees a half-written file.
func writeDaemonState(dir string, st daemonState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := daemonPidPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, data, config.FilePerm); err != nil {
		return err
	}
	return os.Rename(tmp, daemonPidPath(dir))
}

// readDaemonState loads the daemon pid file. A missing file yields (nil, nil) so
// callers can treat "not running" distinctly from a real I/O error.
func readDaemonState(dir string) (*daemonState, error) {
	data, err := os.ReadFile(daemonPidPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st daemonState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("daemon pid file %s is corrupt: %w", daemonPidPath(dir), err)
	}
	return &st, nil
}

// backoffDelay computes the restart delay for the Nth consecutive worker
// failure: base * 2^(n-1) capped at cap. Pure and unit-tested. n<=0 returns 0
// (first start is immediate). Caller resets n when a worker stays healthy.
func backoffDelay(consecutiveFailures int, base, maxDelay time.Duration) time.Duration {
	if consecutiveFailures <= 0 {
		return 0
	}
	d := base
	for i := 1; i < consecutiveFailures; i++ {
		d *= 2
		if d >= maxDelay {
			return maxDelay
		}
	}
	if d > maxDelay {
		return maxDelay
	}
	return d
}

const (
	daemonBackoffBase = time.Second
	daemonBackoffCap  = 60 * time.Second
	// daemonMaxFastFailures is the consecutive fast-failure ceiling: after this
	// many crashes that each happened within daemonHealthyAfter, the supervisor
	// gives up rather than spin forever (e.g. bad credentials).
	daemonMaxFastFailures = 10
	// daemonHealthyAfter is how long a worker must run before the supervisor
	// considers it healthy and resets the failure counter.
	daemonHealthyAfter = 60 * time.Second
	// daemonStopTimeout bounds the graceful wait in `stop` before SIGKILL.
	daemonStopTimeout = 10 * time.Second
)

// buildWorkerArgs rewrites the supervisor's own argv into a worker argv: it
// strips the daemon-control flags (--daemon / --daemon-supervise) and appends
// --daemon-worker, preserving every other flag (credentials, channel, knowledge,
// etc.) so the worker connects exactly as the foreground command would. Pure for
// unit testing.
func buildWorkerArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	for _, a := range args {
		switch {
		case a == "--"+daemonFlag, a == "--"+daemonSuperviseFlag, a == "--"+daemonWorkerFlag:
			continue
		case strings.HasPrefix(a, "--"+daemonFlag+"="),
			strings.HasPrefix(a, "--"+daemonSuperviseFlag+"="),
			strings.HasPrefix(a, "--"+daemonWorkerFlag+"="):
			continue
		}
		out = append(out, a)
	}
	out = append(out, "--"+daemonWorkerFlag)
	return out
}

// startDaemon implements `connect --daemon`: it re-execs dws in supervisor mode
// detached from the terminal, writes nothing itself to the worker log (the
// supervisor does), prints the pid + log path, and returns so the parent exits.
func startDaemon(cmd *cobra.Command, dirKey, clientID string) error {
	if !daemonDetachSupported {
		return apperrors.NewValidation("--daemon is not supported on this OS; run the foreground connector under a service manager instead")
	}
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("create daemon dir: " + err.Error())
	}
	// Refuse to start a second supervisor for the same connector. The Stream
	// single-instance lock would also catch this at the worker layer, but a
	// pre-flight check gives a clearer message and avoids an orphaned supervisor.
	if st, _ := readDaemonState(dir); st != nil && st.Pid > 0 && processAlive(st.Pid) {
		return apperrors.NewValidation(fmt.Sprintf("a connect daemon is already running for %s (pid %d); use `robot connect status`/`stop`", dirKey, st.Pid))
	}

	exe, err := os.Executable()
	if err != nil {
		return apperrors.NewInternal("resolve executable: " + err.Error())
	}
	superviseArgs := buildSuperviseArgs(os.Args[1:])

	logPath := daemonLogPath(dir)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePerm)
	if err != nil {
		return apperrors.NewInternal("open daemon log: " + err.Error())
	}
	defer logFile.Close()

	child := exec.Command(exe, superviseArgs...)
	child.Stdout = logFile
	child.Stderr = logFile
	child.Stdin = nil
	// Pass the resolved dir key + clientId through the environment so the
	// supervisor files its pid under the same key the parent computed (rather
	// than re-deriving and risking a mismatch).
	child.Env = append(os.Environ(),
		"DWS_CONNECT_DAEMON_DIRKEY="+dirKey,
		"DWS_CONNECT_DAEMON_CLIENTID="+clientID,
	)
	if connectDaemonDirOverride != "" {
		child.Env = append(child.Env, "DWS_CONNECT_DAEMON_DIR="+connectDaemonDirOverride)
	}
	applyDetach(child)

	if err := child.Start(); err != nil {
		return apperrors.NewInternal("start daemon: " + err.Error())
	}
	// Release the child so the parent can exit without leaving a zombie.
	pid := child.Process.Pid
	_ = child.Process.Release()

	writeConnectDaemonStarted(cmd.OutOrStdout(), pid, logPath, clientID, dirKey)
	return nil
}

func writeConnectDaemonStarted(w io.Writer, pid int, logPath, clientID, dirKey string) {
	fmt.Fprintf(w, "connect daemon started (pid %d)\n", pid)
	fmt.Fprintf(w, "  logs:   %s\n", logPath)
	fmt.Fprintf(w, "  status: dws devapp robot connect status%s\n", statusHintArgs(clientID, dirKey))
	fmt.Fprintf(w, "  stop:   dws devapp robot connect stop%s\n", statusHintArgs(clientID, dirKey))
	fmt.Fprint(w, connectLocalDebugNotice())
}

// buildSuperviseArgs rewrites argv to run the supervisor: strip --daemon, append
// --daemon-supervise. Pure for testing.
func buildSuperviseArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	for _, a := range args {
		if a == "--"+daemonFlag || strings.HasPrefix(a, "--"+daemonFlag+"=") {
			continue
		}
		out = append(out, a)
	}
	out = append(out, "--"+daemonSuperviseFlag)
	return out
}

func statusHintArgs(clientID, dirKey string) string {
	if strings.TrimSpace(clientID) != "" {
		return " --robot-client-id " + clientID
	}
	if strings.HasPrefix(dirKey, "app-") {
		return " --unified-app-id " + strings.TrimPrefix(dirKey, "app-")
	}
	return ""
}

// runSupervisor is the entry point when dws is started with --daemon-supervise.
// It writes the daemon pid file, then loops launching the worker child and
// restarting it with backoff until told to stop (SIGTERM/SIGINT) or it exhausts
// the fast-failure budget. On stop it forwards the signal to the worker, waits,
// and removes its pid file.
func runSupervisor(cmd *cobra.Command) error {
	dirKey := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_DIRKEY"))
	if dirKey == "" {
		return apperrors.NewInternal("supervisor started without DWS_CONNECT_DAEMON_DIRKEY")
	}
	if v := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_DIR")); v != "" {
		connectDaemonDirOverride = v
	}
	clientID := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_CLIENTID"))
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("create daemon dir: " + err.Error())
	}
	st := daemonState{
		Pid:       os.Getpid(),
		StartUnix: time.Now().Unix(),
		LogPath:   daemonLogPath(dir),
		DirKey:    dirKey,
		ClientID:  clientID,
	}
	if err := writeDaemonState(dir, st); err != nil {
		return apperrors.NewInternal("write daemon pid file: " + err.Error())
	}
	defer os.Remove(daemonPidPath(dir))

	// Route worker stdout/stderr through the shared size-rotating writer
	// (logging.NewRotatingFile) so a long-running connector's logs don't grow
	// unbounded. The detached parent already redirected our own fds to the same
	// daemon.log; from here we append through the rotator. Fall back to stderr if
	// the rotator can't be opened.
	var out io.Writer = cmd.ErrOrStderr()
	if rot, rerr := logging.NewRotatingFile(daemonLogPath(dir)); rerr == nil {
		defer rot.Close()
		out = rot
	}

	// The supervisor reacts to SIGTERM/SIGINT itself (cmd.Context from root is
	// already wired to these). We use an independent NotifyContext so cancelling
	// it stops the loop and lets us forward the signal to the worker explicitly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	workerArgs := buildWorkerArgs(os.Args[1:])
	exe, err := os.Executable()
	if err != nil {
		return apperrors.NewInternal("resolve executable: " + err.Error())
	}

	failures := 0
	for {
		if delay := backoffDelay(failures, daemonBackoffBase, daemonBackoffCap); delay > 0 {
			fmt.Fprintf(out, "[daemon] restarting worker in %s (consecutive failures: %d)\n", delay, failures)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
		}
		if ctx.Err() != nil {
			return nil
		}

		started := time.Now()
		worker := exec.Command(exe, workerArgs...)
		worker.Stdout = out
		worker.Stderr = out
		worker.Env = os.Environ()
		if err := worker.Start(); err != nil {
			fmt.Fprintf(out, "[daemon] failed to start worker: %v\n", err)
			failures++
			if failures >= daemonMaxFastFailures {
				return apperrors.NewInternal("daemon worker failed to start too many times; giving up")
			}
			continue
		}
		fmt.Fprintf(out, "[daemon] worker started (pid %d)\n", worker.Process.Pid)

		waitErr := superviseWait(ctx, worker)
		ran := time.Since(started)

		if ctx.Err() != nil {
			// We were asked to stop; the worker has been (or is being) signalled.
			fmt.Fprintln(out, "[daemon] stop requested, worker shut down; exiting supervisor")
			return nil
		}

		if ran >= daemonHealthyAfter {
			failures = 0
		} else {
			failures++
		}
		fmt.Fprintf(out, "[daemon] worker exited after %s (err=%v); consecutive failures: %d\n", ran.Round(time.Second), waitErr, failures)
		if failures >= daemonMaxFastFailures {
			return apperrors.NewInternal(fmt.Sprintf("daemon worker crashed %d times in a row; giving up (check %s)", failures, daemonLogPath(dir)))
		}
	}
}

// superviseWait waits for the worker to exit, but if the supervisor's ctx is
// cancelled first it forwards SIGTERM to the worker for a graceful shutdown
// (releasing the Stream single-instance lock) and then waits.
func superviseWait(ctx context.Context, worker *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- worker.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = worker.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			return err
		case <-time.After(daemonStopTimeout):
			_ = worker.Process.Kill()
			return <-done
		}
	}
}

// daemonStatus reports the state of the connector daemon to w.
func daemonStatus(w io.Writer, dirKey string) error {
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("resolve daemon dir: " + err.Error())
	}
	st, err := readDaemonState(dir)
	if err != nil {
		return apperrors.NewInternal(err.Error())
	}
	if st == nil {
		fmt.Fprintf(w, "connect daemon: not running (no pid file under %s)\n", dir)
		return nil
	}
	if st.Pid <= 0 || !processAlive(st.Pid) {
		fmt.Fprintf(w, "connect daemon: not running (stale pid file for pid %d at %s)\n", st.Pid, daemonPidPath(dir))
		return nil
	}
	uptime := time.Since(time.Unix(st.StartUnix, 0)).Round(time.Second)
	fmt.Fprintf(w, "connect daemon: running\n")
	fmt.Fprintf(w, "  pid:     %d\n", st.Pid)
	fmt.Fprintf(w, "  uptime:  %s\n", uptime)
	fmt.Fprintf(w, "  logs:    %s\n", st.LogPath)
	if st.ClientID != "" {
		fmt.Fprintf(w, "  client:  %s\n", st.ClientID)
	}
	return nil
}

// daemonStop gracefully stops the connector daemon: SIGTERM the supervisor (it
// forwards to the worker, which releases the lock and Stream connection), poll
// until it exits, escalate to SIGKILL on timeout, and clean up the pid file.
func daemonStop(w io.Writer, dirKey string) error {
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("resolve daemon dir: " + err.Error())
	}
	st, err := readDaemonState(dir)
	if err != nil {
		return apperrors.NewInternal(err.Error())
	}
	if st == nil || st.Pid <= 0 {
		fmt.Fprintf(w, "connect daemon: not running (nothing to stop)\n")
		return nil
	}
	if !processAlive(st.Pid) {
		_ = os.Remove(daemonPidPath(dir))
		fmt.Fprintf(w, "connect daemon: was not running (cleaned up stale pid file for pid %d)\n", st.Pid)
		return nil
	}
	proc, err := os.FindProcess(st.Pid)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("find daemon process %d: %v", st.Pid, err))
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("signal daemon %d: %v", st.Pid, err))
	}
	fmt.Fprintf(w, "sent SIGTERM to connect daemon (pid %d), waiting for graceful stop...\n", st.Pid)

	deadline := time.Now().Add(daemonStopTimeout)
	for time.Now().Before(deadline) {
		if !processAlive(st.Pid) {
			_ = os.Remove(daemonPidPath(dir))
			fmt.Fprintf(w, "connect daemon stopped (pid %d)\n", st.Pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Graceful window elapsed; force kill.
	_ = proc.Signal(syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
	_ = os.Remove(daemonPidPath(dir))
	fmt.Fprintf(w, "connect daemon did not stop in %s; sent SIGKILL (pid %d)\n", daemonStopTimeout, st.Pid)
	return nil
}

// newDevAppRobotConnectStatusCommand implements `dws devapp robot connect
// status`: report whether a background connector daemon is running for the
// robot identified by --robot-client-id (or --unified-app-id).
func newDevAppRobotConnectStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "status",
		Short:             "查看后台连接器守护进程状态（pid、运行时长、日志路径）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirKey, err := connectDaemonDirKeyFromFlags(cmd)
			if err != nil {
				return err
			}
			return daemonStatus(cmd.OutOrStdout(), dirKey)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("robot-client-id", "", "机器人 clientId（定位守护进程）")
	cmd.Flags().String("unified-app-id", "", "统一应用 ID（当未用 clientId 起守护进程时定位）")
	return cmd
}

// newDevAppRobotConnectStopCommand implements `dws devapp robot connect stop`:
// gracefully stop the background connector daemon (SIGTERM, escalate to SIGKILL
// on timeout) and clean up its pid file.
func newDevAppRobotConnectStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "stop",
		Short:             "优雅停止后台连接器守护进程（释放单实例锁与 Stream 连接）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirKey, err := connectDaemonDirKeyFromFlags(cmd)
			if err != nil {
				return err
			}
			return daemonStop(cmd.OutOrStdout(), dirKey)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("robot-client-id", "", "机器人 clientId（定位守护进程）")
	cmd.Flags().String("unified-app-id", "", "统一应用 ID（当未用 clientId 起守护进程时定位）")
	return cmd
}

// connectDaemonDirKeyFromFlags resolves the daemon directory key from the
// status/stop flags, requiring at least one identifier.
func connectDaemonDirKeyFromFlags(cmd *cobra.Command) (string, error) {
	clientID := devAppStringFlag(cmd, "robot-client-id")
	unifiedAppID := devAppStringFlag(cmd, "unified-app-id")
	dirKey := daemonDirKey(clientID, unifiedAppID)
	if dirKey == "" {
		return "", apperrors.NewValidation("需要 --robot-client-id 或 --unified-app-id 以定位守护进程")
	}
	return dirKey, nil
}

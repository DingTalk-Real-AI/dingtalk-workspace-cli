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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/tui"
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
	Pid           int    `json:"pid"`
	StartUnix     int64  `json:"startUnix"`
	LogPath       string `json:"logPath"`
	DirKey        string `json:"dirKey"`
	ClientID      string `json:"clientId,omitempty"`
	UnifiedAppID  string `json:"unifiedAppId,omitempty"`
	Channel       string `json:"channel,omitempty"`
	NotifyStaffID string `json:"notifyStaffId,omitempty"`
	// Profile records the --profile selector the connector was started with,
	// so `restart` re-fetches credentials against the same org instead of the
	// default profile (which may not know the unifiedAppId at all).
	Profile string `json:"profile,omitempty"`
	// AlwaysOn controls whether the supervisor auto-restarts the worker on
	// crash. Without it the supervisor exits after the first worker exit.
	AlwaysOn bool `json:"alwaysOn,omitempty"`
}

// connectDaemonDirOverride lets tests redirect the per-client daemon directory
// away from the real ~/.dws tree. Empty means use config.DefaultConfigDir.
var connectDaemonDirOverride string

// Process and clock hooks keep daemon lifecycle tests deterministic while the
// production defaults remain the operating-system implementations.
var (
	daemonDetachEnabled = daemonDetachSupported
	daemonExecutable    = os.Executable
	daemonCommand       = exec.Command
	daemonNow           = time.Now
	daemonCreateTemp    = os.CreateTemp
	daemonFileChmod     = func(file *os.File, mode os.FileMode) error { return file.Chmod(mode) }
	daemonCopy          = io.Copy
	daemonFileSync      = func(file *os.File) error { return file.Sync() }
	daemonFileClose     = func(file *os.File) error { return file.Close() }
	daemonRename        = os.Rename
	daemonFindProcess   = os.FindProcess
	daemonProcessAlive  = processAlive
	daemonSignalProcess = func(process *os.Process, signal os.Signal) error {
		return process.Signal(signal)
	}
	daemonSignalContext = func() (context.Context, context.CancelFunc) {
		return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
)

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

func daemonPidPath(dir string) string   { return filepath.Join(dir, "daemon.pid") }
func daemonStatePath(dir string) string { return filepath.Join(dir, "daemon-state.json") }
func daemonLogPath(dir string) string   { return filepath.Join(dir, "daemon.log") }
func daemonExecutablePath(dir string) string {
	return filepath.Join(dir, "dws-daemon")
}

// stageDaemonExecutable snapshots the current dws binary into the connector's
// persistent directory. One-click launchers may execute a temporary download
// (for example dws-latest) and remove it after connect --daemon returns; the
// supervisor must not depend on that transient path for future worker restarts.
func stageDaemonExecutable(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	tmp, err := daemonCreateTemp(dir, ".dws-daemon-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := daemonFileChmod(tmp, daemonExecutablePerm); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := daemonCopy(tmp, in); err != nil {
		tmp.Close()
		return "", err
	}
	if err := daemonFileSync(tmp); err != nil {
		tmp.Close()
		return "", err
	}
	if err := daemonFileClose(tmp); err != nil {
		return "", err
	}

	dst := daemonExecutablePath(dir)
	if err := daemonRename(tmpPath, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// writeDaemonState atomically persists the daemon state to daemon-state.json
// (persistent, survives supervisor exit) so restart/list can recover config.
func writeDaemonState(dir string, st daemonState) error {
	data, _ := json.MarshalIndent(st, "", "  ")
	tmp := daemonStatePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, data, config.FilePerm); err != nil {
		return err
	}
	return daemonRename(tmp, daemonStatePath(dir))
}

// readDaemonState loads the daemon state. Reads daemon-state.json (persistent)
// first, falls back to daemon.pid for backward compat with old connectors.
func readDaemonState(dir string) (*daemonState, error) {
	for _, p := range []string{daemonStatePath(dir), daemonPidPath(dir)} {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var st daemonState
		if err := json.Unmarshal(data, &st); err != nil {
			return nil, fmt.Errorf("daemon state file %s is corrupt: %w", p, err)
		}
		return &st, nil
	}
	return nil, nil
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
	daemonBackoffBase                = time.Second
	daemonBackoffCap                 = 60 * time.Second
	daemonExecutablePerm os.FileMode = 0o700
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
func startDaemon(cmd *cobra.Command, dirKey, clientID, unifiedAppID, channel, notifyStaffID, profile string, alwaysOn bool) error {
	if !daemonDetachEnabled {
		return apperrors.NewValidation("--daemon is not supported on this OS; run the foreground connector under a service manager instead")
	}
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("create daemon dir: " + err.Error())
	}
	// Refuse to start a second supervisor for the same connector. The Stream
	// single-instance lock would also catch this at the worker layer, but a
	// pre-flight check gives a clearer message and avoids an orphaned supervisor.
	if st, _ := readDaemonState(dir); st != nil && st.Pid > 0 && daemonProcessAlive(st.Pid) {
		return apperrors.NewValidation(fmt.Sprintf("a connect daemon is already running for %s (pid %d); use `robot connect status`/`stop`", dirKey, st.Pid))
	}

	exe, err := daemonExecutable()
	if err != nil {
		return apperrors.NewInternal("resolve executable: " + err.Error())
	}
	exe, err = stageDaemonExecutable(exe, dir)
	if err != nil {
		return apperrors.NewInternal("stage daemon executable: " + err.Error())
	}
	superviseArgs := buildSuperviseArgs(os.Args[1:])

	logPath := daemonLogPath(dir)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePerm)
	if err != nil {
		return apperrors.NewInternal("open daemon log: " + err.Error())
	}
	defer logFile.Close()

	child := daemonCommand(exe, superviseArgs...)
	child.Stdout = logFile
	child.Stderr = logFile
	child.Stdin = nil
	// Pass the resolved dir key + clientId through the environment so the
	// supervisor files its pid under the same key the parent computed (rather
	// than re-deriving and risking a mismatch).
	child.Env = append(os.Environ(),
		"DWS_CONNECT_DAEMON_DIRKEY="+dirKey,
		"DWS_CONNECT_DAEMON_CLIENTID="+clientID,
		"DWS_CONNECT_DAEMON_UNIFIEDAPPID="+unifiedAppID,
		"DWS_CONNECT_DAEMON_CHANNEL="+channel,
		"DWS_CONNECT_DAEMON_NOTIFY_STAFF_ID="+notifyStaffID,
		"DWS_CONNECT_DAEMON_PROFILE="+profile,
	)
	if alwaysOn {
		child.Env = append(child.Env, "DWS_CONNECT_DAEMON_ALWAYSON=true")
	}
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
	fmt.Fprintf(w, "  status: dws dev connect status%s\n", statusHintArgs(clientID, dirKey))
	fmt.Fprintf(w, "  stop:   dws dev connect stop%s\n", statusHintArgs(clientID, dirKey))
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
	unifiedAppID := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_UNIFIEDAPPID"))
	channel := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_CHANNEL"))
	notifyStaffID := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_NOTIFY_STAFF_ID"))
	profile := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_PROFILE"))
	alwaysOn := strings.TrimSpace(os.Getenv("DWS_CONNECT_DAEMON_ALWAYSON")) == "true"
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("create daemon dir: " + err.Error())
	}
	st := daemonState{
		Pid:           os.Getpid(),
		StartUnix:     daemonNow().Unix(),
		LogPath:       daemonLogPath(dir),
		DirKey:        dirKey,
		ClientID:      clientID,
		UnifiedAppID:  unifiedAppID,
		Channel:       channel,
		NotifyStaffID: notifyStaffID,
		Profile:       profile,
		AlwaysOn:      alwaysOn,
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
	ctx, stop := daemonSignalContext()
	defer stop()

	workerArgs := buildWorkerArgs(os.Args[1:])
	exe, err := daemonExecutable()
	if err != nil {
		return apperrors.NewInternal("resolve executable: " + err.Error())
	}

	failures := 0
	daemonNotifyStateChange(notifyStaffID, channel, clientID, "started", "")
	for {
		if delay := backoffDelay(failures, daemonBackoffBase, daemonBackoffCap); delay > 0 {
			fmt.Fprintf(out, "[daemon] restarting worker in %s (consecutive failures: %d)\n", delay, failures)
			select {
			case <-ctx.Done():
				daemonNotifyStateChange(notifyStaffID, channel, clientID, "stopped", "")
				return nil
			case <-helperAfter(delay):
			}
		}
		if ctx.Err() != nil {
			daemonNotifyStateChange(notifyStaffID, channel, clientID, "stopped", "")
			return nil
		}

		started := daemonNow()
		worker := daemonCommand(exe, workerArgs...)
		worker.Stdout = out
		worker.Stderr = out
		worker.Env = os.Environ()
		configureWorkerProcessGroup(worker)
		if err := worker.Start(); err != nil {
			fmt.Fprintf(out, "[daemon] failed to start worker: %v\n", err)
			failures++
			if failures >= daemonMaxFastFailures {
				daemonNotifyStateChange(notifyStaffID, channel, clientID, "gave_up", fmt.Sprintf("worker 启动失败 %d 次", failures))
				return apperrors.NewInternal("daemon worker failed to start too many times; giving up")
			}
			continue
		}
		fmt.Fprintf(out, "[daemon] worker started (pid %d)\n", worker.Process.Pid)

		waitErr := superviseWait(ctx, worker)
		cleanupWorkerProcessGroup(worker.Process.Pid)
		ran := daemonNow().Sub(started)

		if ctx.Err() != nil {
			// We were asked to stop; the worker has been (or is being) signalled.
			fmt.Fprintln(out, "[daemon] stop requested, worker shut down; exiting supervisor")
			daemonNotifyStateChange(notifyStaffID, channel, clientID, "stopped", "")
			return nil
		}

		if !alwaysOn {
			fmt.Fprintln(out, "[daemon] worker exited; not restarting (--alwayson not set)")
			daemonNotifyStateChange(notifyStaffID, channel, clientID, "stopped", "worker 退出，未启用 --alwayson")
			return nil
		}

		if ran >= daemonHealthyAfter {
			failures = 0
		} else {
			failures++
		}
		fmt.Fprintf(out, "[daemon] worker exited after %s (err=%v); consecutive failures: %d\n", ran.Round(time.Second), waitErr, failures)
		if failures >= daemonMaxFastFailures {
			daemonNotifyStateChange(notifyStaffID, channel, clientID, "gave_up", fmt.Sprintf("连续崩溃 %d 次", failures))
			return apperrors.NewInternal(fmt.Sprintf("daemon worker crashed %d times in a row; giving up (check %s)", failures, daemonLogPath(dir)))
		}
		daemonNotifyStateChange(notifyStaffID, channel, clientID, "crashed", fmt.Sprintf("worker 退出 (%s)，正在重启", ran.Round(time.Second)))
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
		_ = daemonSignalProcess(worker.Process, syscall.SIGTERM)
		select {
		case err := <-done:
			return err
		case <-helperAfter(daemonStopTimeout):
			_ = worker.Process.Kill()
			return <-done
		}
	}
}

// daemonStatus reports connector health to w. It combines two independent
// signals: the daemon supervisor pid file (is a supervisor alive) and the
// connector heartbeat (is the connection live and receiving — see
// connect_health.go). jsonOut emits the machine-readable health report an
// external supervisor (launchd/systemd/pm2/cron) consumes to decide restarts.
func daemonStatus(w io.Writer, dirKey string, jsonOut bool) error {
	dir, err := connectDaemonDir(dirKey)
	if err != nil {
		return apperrors.NewInternal("resolve daemon dir: " + err.Error())
	}
	st, err := readDaemonState(dir)
	if err != nil {
		return apperrors.NewInternal(err.Error())
	}
	supervised := st != nil && st.Pid > 0 && daemonProcessAlive(st.Pid)

	hb, err := readConnectHeartbeat(dir)
	if err != nil {
		return apperrors.NewInternal("read connector heartbeat: " + err.Error())
	}
	report := deriveConnectHealth(hb, supervised, daemonNow())

	if jsonOut {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(w, string(data))
		return nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s  %s", tui.Key("state"), colorConnectState(report.State)))
	if report.Detail != "" {
		lines = append(lines, fmt.Sprintf("%s %s", tui.Key("detail"), report.Detail))
	}
	if report.Pid > 0 {
		lines = append(lines, fmt.Sprintf("%s    %s", tui.Key("pid"), tui.White(fmt.Sprintf("%d", report.Pid))))
	}
	if report.Channel != "" {
		lines = append(lines, fmt.Sprintf("%s %s", tui.Key("channel"), tui.White(report.Channel)))
	}
	if report.ClientID != "" {
		lines = append(lines, fmt.Sprintf("%s %s", tui.Key("client"), tui.White(report.ClientID)))
	}
	if report.UptimeSec > 0 {
		lines = append(lines, fmt.Sprintf("%s %s", tui.Key("uptime"), tui.White((time.Duration(report.UptimeSec)*time.Second).Round(time.Second).String())))
	}
	lines = append(lines, fmt.Sprintf("%s  %s", tui.Key("super"), supervisedLabel(supervised)))
	if hb != nil {
		if report.LastPushAgoSec > 0 {
			lines = append(lines, fmt.Sprintf("%s  %s ago", tui.Key("recv"), tui.White((time.Duration(report.LastPushAgoSec)*time.Second).Round(time.Second).String())))
		} else {
			lines = append(lines, fmt.Sprintf("%s  %s", tui.Key("recv"), tui.Dim("(none since start)")))
		}
		if report.LastError != "" {
			lines = append(lines, fmt.Sprintf("%s  %s", tui.Key("error"), tui.Danger(report.LastError)))
		}
		lines = append(lines, fmt.Sprintf("%s   %s", tui.Key("logs"), tui.Dim(daemonLogPath(dir))))
	}
	return tui.Panel(w, tui.Bold("connect status"), lines)
}

func supervisedLabel(supervised bool) string {
	if supervised {
		return "running (--daemon)"
	}
	return "none (foreground or external)"
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
	if !daemonProcessAlive(st.Pid) {
		_ = os.Remove(daemonPidPath(dir))
		// The supervisor is dead, but its worker may still be alive (e.g. the
		// supervisor was kill -9'd). Check the heartbeat for the worker pid and
		// stop it so we don't leave an orphan.
		if hb, _ := readConnectHeartbeat(dir); hb != nil && hb.Pid > 0 && daemonProcessAlive(hb.Pid) {
			fmt.Fprintf(w, "connect daemon: supervisor (pid %d) was dead, stopping orphan worker (pid %d)...\n", st.Pid, hb.Pid)
			if proc, perr := daemonFindProcess(hb.Pid); perr == nil {
				_ = daemonSignalProcess(proc, syscall.SIGTERM)
				deadline := daemonNow().Add(daemonStopTimeout)
				for daemonNow().Before(deadline) {
					if !daemonProcessAlive(hb.Pid) {
						break
					}
					helperSleep(200 * time.Millisecond)
				}
				if daemonProcessAlive(hb.Pid) {
					_ = daemonSignalProcess(proc, syscall.SIGKILL)
					helperSleep(200 * time.Millisecond)
				}
			}
			fmt.Fprintf(w, "connect daemon: orphan worker stopped (pid %d)\n", hb.Pid)
			return nil
		}
		fmt.Fprintf(w, "connect daemon: was not running (cleaned up stale pid file for pid %d)\n", st.Pid)
		return nil
	}
	proc, err := daemonFindProcess(st.Pid)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("find daemon process %d: %v", st.Pid, err))
	}
	if err := daemonSignalProcess(proc, syscall.SIGTERM); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("signal daemon %d: %v", st.Pid, err))
	}
	fmt.Fprintf(w, "sent SIGTERM to connect daemon (pid %d), waiting for graceful stop...\n", st.Pid)

	deadline := daemonNow().Add(daemonStopTimeout)
	for daemonNow().Before(deadline) {
		if !daemonProcessAlive(st.Pid) {
			_ = os.Remove(daemonPidPath(dir))
			fmt.Fprintf(w, "connect daemon stopped (pid %d)\n", st.Pid)
			return nil
		}
		helperSleep(200 * time.Millisecond)
	}
	// Graceful window elapsed; force kill.
	_ = daemonSignalProcess(proc, syscall.SIGKILL)
	helperSleep(200 * time.Millisecond)
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
		Short:             "查看连接器健康状态（healthy/degraded/down，pid、收发活动、日志路径；--json 供外部托管消费）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirKey, err := connectDaemonDirKeyFromFlags(cmd)
			if err != nil {
				return err
			}
			jsonOut, _ := cmd.Flags().GetBool("json")
			return daemonStatus(cmd.OutOrStdout(), dirKey, jsonOut)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("robot-client-id", "", "机器人 clientId（定位守护进程）")
	cmd.Flags().String("unified-app-id", "", "统一应用 ID（当未用 clientId 起守护进程时定位）")
	cmd.Flags().Bool("json", false, "以 JSON 输出健康报告（供 launchd/systemd/pm2/cron 判断是否重启）")
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

// newDevAppRobotConnectRestartCommand implements `dws devapp robot connect
// restart`: stop the running daemon (if any) then re-launch it using the
// persisted unifiedAppId so credentials are freshly fetched from the dev
// platform — no secrets stored on disk.
func newDevAppRobotConnectRestartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "restart",
		Short:             "重启连接器守护进程（通过持久化的 unifiedAppId 重新拉取密钥，无需本地存密钥）",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirKey, err := connectDaemonDirKeyFromFlags(cmd)
			if err != nil {
				return err
			}
			dir, err := connectDaemonDir(dirKey)
			if err != nil {
				return apperrors.NewInternal("resolve daemon dir: " + err.Error())
			}
			st, err := readDaemonState(dir)
			if err != nil {
				return apperrors.NewInternal(err.Error())
			}
			if st == nil {
				return apperrors.NewValidation("未找到连接器记录（没有 daemon.pid）；请用 `dws dev connect --daemon` 首次启动")
			}
			unifiedAppID := st.UnifiedAppID
			if unifiedAppID == "" {
				return apperrors.NewValidation("该连接器未持久化 unifiedAppId（可能是用 --robot-client-id/--robot-client-secret 直接启动的，无法安全重启：clientSecret 不落盘）；请停掉后用 `dws dev connect --daemon --unified-app-id <uappid>` 重新启动，之后 restart 就能自动从 credentials get 拉密钥、命令行不出现 secret")
			}
			// Stop the running daemon first (ignore "not running" — that's fine).
			fmt.Fprintln(cmd.OutOrStdout(), "stopping existing daemon...")
			if err := daemonStop(cmd.OutOrStdout(), dirKey); err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "warning: stop returned %v (continuing with restart)\n", err)
			}
			// Re-exec dws dev connect --daemon with the stored flags. An explicit
			// --profile on this invocation overrides the persisted one.
			exe, err := daemonExecutable()
			if err != nil {
				return apperrors.NewInternal("resolve executable: " + err.Error())
			}
			profile := st.Profile
			if v, _ := cmd.Root().PersistentFlags().GetString("profile"); strings.TrimSpace(v) != "" {
				profile = strings.TrimSpace(v)
			}
			args := []string{"dev", "connect", "--daemon", "--unified-app-id", unifiedAppID}
			if st.Channel != "" {
				args = append(args, "--channel", st.Channel)
			}
			if st.NotifyStaffID != "" {
				args = append(args, "--notify-staff-id", st.NotifyStaffID)
			}
			if profile != "" {
				args = append(args, "--profile", profile)
			}
			if st.AlwaysOn {
				args = append(args, "--alwayson")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "restarting connector: dws %s\n", strings.Join(args, " "))
			// Run synchronously: `--daemon` itself detaches the supervisor and
			// returns quickly, so waiting here costs nothing and lets a failed
			// relaunch (e.g. credential fetch error) surface as a non-zero exit
			// instead of a silent success.
			restartCmd := daemonCommand(exe, args...)
			restartCmd.Stdout = cmd.OutOrStdout()
			restartCmd.Stderr = cmd.OutOrStderr()
			restartCmd.Stdin = nil
			if err := restartCmd.Run(); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("重启失败（旧守护进程已停止，连接器记录已清除）；恢复请手动执行: dws %s", strings.Join(args, " ")))
			}
			return nil
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("robot-client-id", "", "机器人 clientId（定位守护进程）")
	cmd.Flags().String("unified-app-id", "", "统一应用 ID（当未用 clientId 起守护进程时定位）")
	return cmd
}

// newDevAppRobotConnectListCommand implements `dws dev connect list`: enumerate
// every connector on this machine and its health, so a developer running
// several robots sees at a glance which are alive/degraded/down without
// querying each clientId. `--json` emits the array for scripts.
func newDevAppRobotConnectListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "列出本机所有连接器及健康状态（healthy/degraded/down）；--json 供脚本消费",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reports, err := listConnectors(daemonNow())
			if err != nil {
				return apperrors.NewInternal(err.Error())
			}
			resolveAppNames(cmd, runner, reports)
			w := cmd.OutOrStdout()
			if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
				data, _ := json.MarshalIndent(reports, "", "  ")
				fmt.Fprintln(w, string(data))
				return nil
			}
			if len(reports) == 0 {
				fmt.Fprintln(w, "no connectors found")
				return nil
			}
			return writeConnectListTable(w, reports)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().Bool("json", false, "以 JSON 数组输出（供脚本消费）")
	return cmd
}

// resolveAppNames calls list_dev_app once to build a unifiedAppId→name map,
// then fills in AppName on each report. Failures are silent (name stays empty)
// so the list still works offline or when the API is unreachable.
func resolveAppNames(cmd *cobra.Command, runner executor.Runner, reports []connectHealthReport) {
	need := false
	for i := range reports {
		if reports[i].UnifiedAppID != "" {
			need = true
			break
		}
	}
	if !need {
		return
	}
	nameMap, err := devAppNameMap(cmd, runner)
	if err != nil || nameMap == nil {
		return
	}
	for i := range reports {
		if reports[i].UnifiedAppID == "" {
			continue
		}
		if name, ok := nameMap[reports[i].UnifiedAppID]; ok && name != "" {
			reports[i].AppName = name
		}
	}
}

// devAppNameMap calls list_dev_app with pagination to build a full
// unifiedAppId→appName map. It is best-effort: any error returns an empty map.
func devAppNameMap(cmd *cobra.Command, runner executor.Runner) (map[string]string, error) {
	out := make(map[string]string)
	cursor := ""
	for page := 0; page < 20; page++ {
		params := map[string]any{"pageSize": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), devAppProduct, devAppListTool, params)
		res, err := runner.Run(cmd.Context(), inv)
		if err != nil {
			return out, err
		}
		payload := devAppConnectUnwrap(res.Response)
		items := devAppConnectList(payload)
		for _, item := range items {
			uid := devAppConnectFirst(item, "unifiedAppId", "id")
			name := devAppConnectFirst(item, "name", "appName")
			if uid != "" && name != "" {
				out[uid] = name
			}
		}
		hasMore := false
		if v, ok := payload["hasMore"].(bool); ok {
			hasMore = v
		}
		if !hasMore {
			break
		}
		cursor = devAppConnectFirst(payload, "nextCursor", "cursor")
		if cursor == "" {
			break
		}
	}
	return out, nil
}

// devAppConnectList extracts the array of app items from a list_dev_app payload,
// tolerating various wrapper shapes.
func devAppConnectList(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	for _, key := range []string{"items", "list", "data"} {
		if arr, ok := payload[key].([]any); ok {
			out := make([]map[string]any, 0, len(arr))
			for _, e := range arr {
				if m, ok := e.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return out
		}
	}
	return nil
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

func colorConnectState(state string) string {
	switch state {
	case healthHealthy:
		return tui.Success(state)
	case healthDegraded:
		return tui.Warning(state)
	case healthDown, healthNotRunning:
		return tui.Danger(state)
	default:
		return tui.Cyan(state)
	}
}

func writeConnectListTable(w io.Writer, reports []connectHealthReport) error {
	type col struct {
		header string
		width  int
	}
	cols := []col{
		{"STATE", 11},
		{"APP NAME", 8},
		{"CLIENT", 8},
		{"PID", 6},
		{"CHANNEL", 7},
		{"UPTIME", 6},
	}
	// compute column widths from data
	for _, r := range reports {
		if w := tui.PlainRuneWidth(r.AppName); w > cols[1].width {
			cols[1].width = w
		}
		if w := tui.PlainRuneWidth(r.ClientID); w > cols[2].width {
			cols[2].width = w
		}
		if w := tui.PlainRuneWidth(r.Channel); w > cols[4].width {
			cols[4].width = w
		}
	}
	for i := range cols {
		if cols[i].width > tui.MaxTableColumnWidth {
			cols[i].width = tui.MaxTableColumnWidth
		}
	}

	writeBorder := func(left, mid, right string, colorFn func(string) string) {
		fmt.Fprint(w, colorFn(left))
		for i, c := range cols {
			if i > 0 {
				fmt.Fprint(w, colorFn(mid))
			}
			fmt.Fprint(w, colorFn(strings.Repeat("─", c.width+2)))
		}
		fmt.Fprintln(w, colorFn(right))
	}
	writeRowCells := func(cells []string) {
		fmt.Fprint(w, tui.Gray("│"))
		for i, c := range cols {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			fmt.Fprintf(w, " %s ", tui.PadRightANSI(cell, c.width))
			fmt.Fprint(w, tui.Gray("│"))
		}
		fmt.Fprintln(w)
	}

	// header
	writeBorder("╭", "┬", "╮", tui.Blue)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = tui.Brand(c.header)
	}
	writeRowCells(headers)
	writeBorder("├", "┼", "┤", tui.Gray)

	// rows
	for _, r := range reports {
		uptime := tui.Dim("-")
		if r.UptimeSec > 0 {
			uptime = tui.White((time.Duration(r.UptimeSec) * time.Second).Round(time.Second).String())
		}
		pid := tui.Dim("-")
		if r.Pid > 0 {
			pid = tui.White(fmt.Sprintf("%d", r.Pid))
		}
		channel := tui.Dim("-")
		if r.Channel != "" {
			channel = tui.White(r.Channel)
		}
		appName := tui.Dim("-")
		if r.AppName != "" {
			appName = tui.White(r.AppName)
		}
		writeRowCells([]string{
			colorConnectState(r.State),
			appName,
			tui.White(r.ClientID),
			pid,
			channel,
			uptime,
		})
	}

	writeBorder("╰", "┴", "╯", tui.Blue)
	return nil
}

// daemonNotifyStateChange sends a DingTalk message to the configured staff
// when the connector state changes. No-op when notifyStaffID is empty. It
// execs `dws chat message send` as a subprocess (fire-and-forget) so the
// supervisor is never blocked by notification delivery.
func daemonNotifyStateChange(notifyStaffID, channel, clientID, event, detail string) {
	if notifyStaffID == "" {
		return
	}
	var msg string
	switch event {
	case "started":
		msg = fmt.Sprintf("机器人已启动 ✅\n渠道: %s\nclientId: %s", channel, clientID)
	case "stopped":
		msg = fmt.Sprintf("机器人已停止 ⏹️\n渠道: %s\nclientId: %s", channel, clientID)
	case "crashed":
		msg = fmt.Sprintf("机器人已崩溃 ⚠️\n渠道: %s\n%s\n正在自动重启...", channel, detail)
	case "gave_up":
		msg = fmt.Sprintf("机器人重启失败 ❌\n渠道: %s\n%s\n请检查日志后手动重启", channel, detail)
	default:
		return
	}
	exe, err := daemonExecutable()
	if err != nil {
		return
	}
	go func() {
		cmd := daemonCommand(exe, "chat", "message", "send",
			"--staff-id", notifyStaffID,
			"--text", msg,
			"--ai-tag=false",
			"--yes",
			"--format", "json",
		)
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		_ = cmd.Run()
	}()
}

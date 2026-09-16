package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

func employeeDaemonFixtureCommand(ctx context.Context, bin string, args ...string) *exec.Cmd {
	mode := "worker"
	if strings.Contains(strings.Join(args, " "), "--local-supervise") {
		mode = "supervisor"
	}
	return exec.CommandContext(ctx, bin, "-test.run=^TestEmployeeDaemonSubprocessFixture$", "--", "employee-daemon-"+mode)
}

func TestEmployeeDaemonSubprocessFixture(t *testing.T) {
	mode := os.Args[len(os.Args)-1]
	if mode != "employee-daemon-supervisor" && mode != "employee-daemon-worker" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(config.DefaultConfigDir(), "fixture.json"))
	if err != nil {
		os.Exit(3)
	}
	var cfg digitalEmployeeAdapterConfig
	if json.Unmarshal(raw, &cfg) != nil {
		os.Exit(3)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if mode == "employee-daemon-supervisor" {
		testseam.Swap(t, &employeeExecCommand, employeeDaemonFixtureCommand)
		if superviseDigitalEmployee(ctx, cfg) != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
	pid, _ := strconv.Atoi(os.Getenv("DWS_EMPLOYEE_SUPERVISOR_PID"))
	state := digitalEmployeeRunState{Status: "running", PID: os.Getpid(), SupervisorPID: pid, RunID: os.Getenv("DWS_EMPLOYEE_RUN_ID"), AgentUUID: cfg.Binding.AgentUUID, Profile: cfg.Binding.DWSProfile, Channel: cfg.Binding.Channel}
	dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	if persistEmployeeState(dir, state) != nil {
		os.Exit(5)
	}
	<-ctx.Done()
	state.Status = "stopped"
	if persistEmployeeState(dir, state) != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func TestEmployeeDaemonLifecycleUsesSavedProfileAndStopsOnlyOwnProcess(t *testing.T) {
	if !daemonDetachSupported {
		t.Skip("平台不支持后台")
	}
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	testseam.Swap(t, &deapConnectConfigDir, func() string { return dir })
	testseam.Swap(t, &employeeExecCommand, employeeDaemonFixtureCommand)
	cfg := digitalEmployeeAdapterConfig{Binding: digitalEmployeeBinding{SchemaVersion: 1, AgentUUID: "agent", DWSProfile: "corp:employee", Channel: "custom", OperatorOpenDingTalkID: "owner"}, SelfOpenDingTalkID: "employee-open", AlwaysOn: true}
	if err := saveDigitalEmployeeBinding(dir, cfg.Binding); err != nil {
		t.Fatal(err)
	}
	if err := writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json"), cfg); err != nil {
		t.Fatal(err)
	}
	if err := writeEmployeeJSON(filepath.Join(dir, "fixture.json"), cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := newDeapConnectCommand()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	t.Cleanup(func() {
		state, err := readDigitalEmployeeState(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile))
		if err == nil {
			_ = writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "stop.json"), map[string]string{"runId": state.RunID})
			deadline := time.Now().Add(3 * time.Second)
			for processAlive(state.SupervisorPID) && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
		}
	})
	if err := startDigitalEmployeeDaemon(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	var result struct {
		OK   bool                    `json:"ok"`
		Data digitalEmployeeRunState `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.OK || result.Data.Status != "running" || result.Data.Profile != cfg.Binding.DWSProfile {
		t.Fatalf("unexpected startup: %s (%v)", out.String(), err)
	}
	t.Cleanup(func() {
		_ = writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "stop.json"), map[string]string{"runId": result.Data.RunID})
	})
	stop := newDigitalEmployeeStopCommand()
	stop.SetContext(ctx)
	stop.SetOut(&bytes.Buffer{})
	_ = stop.Flags().Set("agent-uuid", "agent")
	if err := stop.RunE(stop, nil); err != nil {
		t.Fatal(err)
	}
	if processAlive(result.Data.SupervisorPID) || processAlive(result.Data.PID) {
		t.Fatal("owned processes survived stop")
	}
	if !processAlive(os.Getpid()) {
		t.Fatal("stop affected test host")
	}
	restart := newDigitalEmployeeRestartCommand()
	restart.SetContext(ctx)
	out.Reset()
	restart.SetOut(&out)
	_ = restart.Flags().Set("agent-uuid", "agent")
	if err := restart.RunE(restart, nil); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.OK || result.Data.Status != "running" {
		t.Fatalf("restart failed: %s (%v)", out.String(), err)
	}
	stop.SetContext(ctx)
	if err := stop.RunE(stop, nil); err != nil {
		t.Fatal(err)
	}
}

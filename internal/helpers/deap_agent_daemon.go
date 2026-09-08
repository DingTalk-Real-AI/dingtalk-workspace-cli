// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func startDigitalEmployeeDaemon(cmd *cobra.Command, cfg digitalEmployeeAdapterConfig) error {
	if !daemonDetachSupported {
		return fmt.Errorf("当前平台不支持后台运行")
	}
	dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	proc, err := employeeCommand(context.Background(), cfg.Binding.DWSProfile, "dingtalk-tag", "connect", "--agent-uuid", cfg.Binding.AgentUUID, "--channel", cfg.Binding.Channel, "--local-supervise", "--yes", "--format", "json")
	if err != nil {
		return err
	}
	// 保留可执行快照，后台重启不依赖可能被覆盖的开发构建。
	bin, err := stageDaemonExecutable(proc.Path, dir)
	if err != nil {
		return err
	}
	proc.Path = bin
	proc.Args[0] = bin
	log, err := os.OpenFile(filepath.Join(dir, "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	// 运行日志只由我们写稳定状态，不转储 Agent 或 MCP stderr。
	proc.Stdout = nil
	proc.Stderr = nil
	applyDetach(proc)
	if err = proc.Start(); err != nil {
		return err
	}
	pid := proc.Process.Pid
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	timer := time.NewTimer(digitalEmployeeReadyTimeout + 15*time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s, e := readDigitalEmployeeState(dir)
			if e == nil && s.SupervisorPID == pid && s.Status == "running" {
				return writeDWSMachineEnvelope(cmd, s)
			}
			if e == nil && s.SupervisorPID == pid && s.Status == "blocked" {
				return fmt.Errorf("数字员工后台启动失败: %s", s.Code)
			}
		case exitErr := <-done:
			return fmt.Errorf("数字员工后台进程退出（%v）；Profile 已保留，请检查 connection status；若被系统终止，请检查主机执行策略", exitErr)
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-timer.C:
			return fmt.Errorf("数字员工尚未 ready；后台可能在等待服务端重试窗口，请检查 connection status 或 stop")
		}
	}
}

func runDigitalEmployeeSaved(cmd *cobra.Command) error {
	profile := auth.RuntimeProfile()
	cfg, err := loadDigitalEmployeeConfig(profile)
	if err != nil {
		return err
	}
	if cfg.Binding.AgentUUID != devAppStringFlag(cmd, "agent-uuid") || cfg.Binding.Channel != devAppStringFlag(cmd, "channel") {
		return fmt.Errorf("saved adapter identity mismatch")
	}
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if commandBoolFlag(cmd, "local-supervise") {
		return superviseDigitalEmployee(ctx, cfg)
	}
	supervisor, _ := strconv.Atoi(os.Getenv("DWS_EMPLOYEE_SUPERVISOR_PID"))
	err = runEmployeeWorker(ctx, cfg, supervisor)
	if err != nil {
		failure, ok := err.(*employeeRunError)
		if !ok {
			failure = &employeeRunError{Code: "worker_failed"}
		}
		_ = writeEmployeeJSON(filepath.Join(digitalEmployeeRuntimeDir(profile), "failure.json"), failure)
	}
	return err
}

func superviseDigitalEmployee(parent context.Context, cfg digitalEmployeeAdapterConfig) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
	lock, err := auth.AcquireDualLock(ctx, filepath.Join(dir, "supervisor"))
	if err != nil {
		return err
	}
	defer lock.Release()
	runID := uuid.NewString()
	go employeeWatchStop(ctx, cancel, dir, runID)
	supervisorState := digitalEmployeeRunState{Status: "starting", PID: os.Getpid(), SupervisorPID: os.Getpid(), RunID: runID, AgentUUID: cfg.Binding.AgentUUID, Profile: cfg.Binding.DWSProfile, Channel: cfg.Binding.Channel, UpdatedAt: time.Now()}
	if err := persistEmployeeState(dir, supervisorState); err != nil {
		return err
	}
	defer func() {
		if ctx.Err() != nil {
			supervisorState.Status = "stopped"
			supervisorState.UpdatedAt = time.Now()
			_ = persistEmployeeState(dir, supervisorState)
		}
	}()
	retry := employeeRetryState{}
	if data, e := os.ReadFile(filepath.Join(dir, "retry.json")); e == nil {
		if json.Unmarshal(data, &retry) != nil {
			return employeeTerminal("invalid_retry_state")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	blocked := func(code string) {
		_ = writeEmployeeJSON(filepath.Join(dir, "state.json"), digitalEmployeeRunState{Status: "blocked", PID: os.Getpid(), SupervisorPID: os.Getpid(), RunID: runID, AgentUUID: cfg.Binding.AgentUUID, Profile: cfg.Binding.DWSProfile, Channel: cfg.Binding.Channel, Code: code, UpdatedAt: time.Now()})
	}
	if retry.Terminal {
		blocked("retry_budget_exhausted")
		return nil
	}
	for {
		if wait := time.Until(retry.NextAttempt); wait > 0 {
			supervisorState.Status = "retry_wait"
			supervisorState.UpdatedAt = time.Now()
			if err := persistEmployeeState(dir, supervisorState); err != nil {
				return err
			}
			if err := waitForDigitalEmployeeReceipt(ctx, wait); err != nil {
				return nil
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		// 清除的仅是本员工上一次 worker 的错误分类；重试预算保留。
		if err := os.Remove(filepath.Join(dir, "failure.json")); err != nil && !os.IsNotExist(err) {
			return err
		}
		proc, err := employeeCommand(ctx, cfg.Binding.DWSProfile, "dingtalk-tag", "connect", "--agent-uuid", cfg.Binding.AgentUUID, "--channel", cfg.Binding.Channel, "--local-worker", "--yes", "--format", "json")
		if err != nil {
			return err
		}
		proc.Env = append(proc.Env, "DWS_EMPLOYEE_SUPERVISOR_PID="+strconv.Itoa(os.Getpid()), "DWS_EMPLOYEE_RUN_ID="+runID)
		configureWorkerProcessGroup(proc)
		proc.Cancel = func() error { return proc.Process.Signal(syscall.SIGTERM) }
		proc.WaitDelay = 10 * time.Second
		err = proc.Start()
		if err == nil {
			err = proc.Wait()
			cleanupWorkerProcessGroup(proc.Process.Pid)
		}
		if ctx.Err() != nil || err == nil {
			return nil
		}
		failure := &employeeRunError{Code: "worker_exit"}
		if raw, e := os.ReadFile(filepath.Join(dir, "failure.json")); e == nil {
			_ = json.Unmarshal(raw, failure)
		}
		var allowed bool
		retry, allowed = employeeNextRetry(retry, failure, time.Now())
		if e := writeEmployeeJSON(filepath.Join(dir, "retry.json"), retry); e != nil {
			return e
		}
		if !cfg.AlwaysOn || !allowed {
			blocked(failure.Code)
			return nil
		}
	}
}

func employeeWatchStop(ctx context.Context, cancel context.CancelFunc, dir, runID string) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var request struct {
				RunID string `json:"runId"`
			}
			if data, err := os.ReadFile(filepath.Join(dir, "stop.json")); err == nil && json.Unmarshal(data, &request) == nil && request.RunID == runID {
				cancel()
				return
			}
		}
	}
}

func employeeFindBindings(agentUUID string) ([]digitalEmployeeBinding, error) {
	root := filepath.Join(deapConnectConfigDir(), "digital-employees")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var bindings []digitalEmployeeBinding
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var b digitalEmployeeBinding
		raw, e := os.ReadFile(filepath.Join(root, entry.Name()))
		if e != nil {
			return nil, e
		}
		if json.Unmarshal(raw, &b) != nil {
			return nil, fmt.Errorf("invalid employee binding")
		}
		if agentUUID != "" && b.AgentUUID != agentUUID {
			continue
		}
		validated, e := loadDigitalEmployeeBinding(deapConnectConfigDir(), b.DWSProfile)
		if e != nil {
			return nil, e
		}
		if filepath.Base(digitalEmployeeBindingPath("", b.DWSProfile)) != entry.Name() {
			return nil, fmt.Errorf("employee binding path mismatch")
		}
		bindings = append(bindings, validated)
	}
	return bindings, nil
}

func runDigitalEmployeeLifecycle(cmd *cobra.Command, action string) error {
	bindings, err := employeeFindBindings(devAppStringFlag(cmd, "agent-uuid"))
	if err != nil {
		return err
	}
	if action != "list" && len(bindings) != 1 {
		return fmt.Errorf("agent-uuid 必须对应唯一的本地员工绑定")
	}
	items := []digitalEmployeeRunState{}
	for _, b := range bindings {
		s := digitalEmployeeRunState{Status: "stopped", AgentUUID: b.AgentUUID, Profile: b.DWSProfile, Channel: bindingChannel(b)}
		if bindingChannel(b) == "dsh" {
			s.Status = "external_managed"
			if action == "stop" || action == "restart" {
				return fmt.Errorf("DSH 由外部宿主管理，请使用 DSH 的停止或重启操作")
			}
		} else {
			dir := digitalEmployeeRuntimeDir(b.DWSProfile)
			if saved, e := readDigitalEmployeeState(dir); e == nil {
				if saved.Profile != b.DWSProfile || saved.AgentUUID != b.AgentUUID {
					return fmt.Errorf("employee runtime identity mismatch")
				}
				s = saved
				pid := s.PID
				if s.SupervisorPID > 0 {
					pid = s.SupervisorPID
				}
				if !processAlive(pid) && s.Status != "blocked" {
					s.Status = "stopped"
				}
			} else if !os.IsNotExist(e) {
				return e
			}
			if action == "stop" || action == "restart" {
				if employeeStateAlive(s) {
					if s.RunID == "" {
						return fmt.Errorf("运行状态缺少实例 ID，拒绝停止不确定的进程")
					}
					if err = writeEmployeeJSON(filepath.Join(dir, "stop.json"), map[string]string{"runId": s.RunID}); err != nil {
						return err
					}
					deadline := time.Now().Add(15 * time.Second)
					for time.Now().Before(deadline) {
						current, e := readDigitalEmployeeState(dir)
						if e == nil && current.Status == "stopped" && (s.SupervisorPID == 0 || !processAlive(s.SupervisorPID)) {
							break
						}
						if err = waitForDigitalEmployeeReceipt(cmd.Context(), 100*time.Millisecond); err != nil {
							return err
						}
					}
					current, e := readDigitalEmployeeState(dir)
					if e != nil || current.Status != "stopped" {
						return fmt.Errorf("等待员工进程优雅退出超时")
					}
				}
				s.Status = "stopped"
				if action == "restart" {
					cfg, e := loadDigitalEmployeeConfig(b.DWSProfile)
					if e != nil {
						return e
					}
					return startDigitalEmployeeDaemon(cmd, cfg)
				}
			}
		}
		items = append(items, s)
	}
	if action == "list" {
		return writeDWSMachineEnvelope(cmd, map[string]any{"items": items})
	}
	return writeDWSMachineEnvelope(cmd, items[0])
}

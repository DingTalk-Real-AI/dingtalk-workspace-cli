// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// DSH 与原生 worker 使用同一把本机 Profile 锁。stdin 由宿主持有，只有
// Consumer、任务及回复全部释放后才关闭；不是跨机器 Presence 或消息 ACK。
func runEmployeeLease(cmd *cobra.Command) error {
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	profile := auth.RuntimeProfile()
	revision, err := strconv.ParseUint(devAppStringFlag(cmd, "binding-revision"), 10, 64)
	instance := devAppStringFlag(cmd, "runtime-instance-id")
	_, instanceErr := uuid.Parse(instance)
	if err != nil || profile == "" || instanceErr != nil {
		return fmt.Errorf("invalid lease identity")
	}
	waitCtx, stopWait := context.WithTimeout(ctx, 10*time.Second)
	defer stopWait()
	lock, err := auth.AcquireDualLock(waitCtx, filepath.Join(digitalEmployeeRuntimeDir(profile), "worker"))
	if err != nil {
		return fmt.Errorf("employee runtime lock unavailable")
	}
	defer lock.Release()
	if err := checkEmployeeLeaseGuard(profile); err != nil {
		return err
	}
	b, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), profile)
	if err != nil || b.AgentUUID != devAppStringFlag(cmd, "agent-uuid") || b.BindingRevision != revision || bindingChannel(b) != "dsh" || employeeBindingState(b) != "bound" || employeeDesiredState(b) != "running" {
		return fmt.Errorf("employee lease binding not authorized")
	}
	// 锁进程异常退出后仍保留隔离标记；文件锁消失不是旧宿主已释放的证明。
	guard := employeeLeaseGuard{AgentUUID: b.AgentUUID, Revision: revision, InstanceID: instance}
	if err := writeEmployeeJSON(employeeLeaseGuardPath(profile), guard); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "[employee] leased"); err != nil {
		return err
	}
	done := make(chan bool, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 64))
		done <- err == nil && string(data) == "released\n"
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case released := <-done:
		if !released {
			return fmt.Errorf("employee release unconfirmed; runtime remains quarantined")
		}
		return os.Remove(employeeLeaseGuardPath(profile))
	}
}

type employeeLeaseGuard struct {
	AgentUUID  string `json:"agentUuid"`
	Revision   uint64 `json:"bindingRevision"`
	InstanceID string `json:"runtimeInstanceId"`
}

func employeeLeaseGuardPath(profile string) string {
	return filepath.Join(digitalEmployeeRuntimeDir(profile), "lease-guard.json")
}

// 调用方必须持有 worker 锁；不按 PID 或超时猜测旧宿主是否已释放。
func checkEmployeeLeaseGuard(profile string) error {
	if _, err := os.Lstat(employeeLeaseGuardPath(profile)); os.IsNotExist(err) {
		return nil
	}
	return employeeTerminal("previous_host_release_unconfirmed")
}

func clearEmployeeLeaseGuard(b digitalEmployeeBinding, instance string) error {
	data, err := os.ReadFile(employeeLeaseGuardPath(b.DWSProfile))
	if os.IsNotExist(err) {
		return nil
	}
	var guard employeeLeaseGuard
	if err != nil || json.Unmarshal(data, &guard) != nil || instance == "" || guard.AgentUUID != b.AgentUUID || guard.Revision != b.BindingRevision || guard.InstanceID != instance {
		return fmt.Errorf("旧宿主实例未精确确认释放，保留隔离标记")
	}
	return os.Remove(employeeLeaseGuardPath(b.DWSProfile))
}

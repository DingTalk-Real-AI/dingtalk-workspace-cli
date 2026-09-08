// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
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
	if err != nil || profile == "" {
		return fmt.Errorf("invalid lease identity")
	}
	waitCtx, stopWait := context.WithTimeout(ctx, 10*time.Second)
	defer stopWait()
	lock, err := auth.AcquireDualLock(waitCtx, filepath.Join(digitalEmployeeRuntimeDir(profile), "worker"))
	if err != nil {
		return fmt.Errorf("employee runtime lock unavailable")
	}
	defer lock.Release()
	b, err := loadDigitalEmployeeBinding(deapConnectConfigDir(), profile)
	if err != nil || b.AgentUUID != devAppStringFlag(cmd, "agent-uuid") || b.BindingRevision != revision || bindingChannel(b) != "dsh" || employeeBindingState(b) != "bound" || employeeDesiredState(b) != "running" {
		return fmt.Errorf("employee lease binding not authorized")
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "[employee] leased"); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, cmd.InOrStdin()); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

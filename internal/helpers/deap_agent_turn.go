// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// 初始化本地协议但不发送模型请求；模型授权仍由真实首轮验证。
func prepareEmployeeForwarder(parent context.Context, fwd forwarder) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	switch f := fwd.(type) {
	case *codexAppServerForwarder:
		client, err := codexNewAppServerClient(ctx, f.bin, f.env, f.cwd())
		if err != nil {
			return err
		}
		defer client.close()
		return client.initialize(ctx)
	case *qoderStreamForwarder:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.ensureLocked(ctx)
	case *opencodeForwarder:
		_, err := f.server.ensure(ctx)
		return err
	case *execForwarder:
		if len(f.argv) == 0 {
			return fmt.Errorf("missing agent executable")
		}
		_, err := exec.LookPath(f.argv[0])
		return err
	default:
		return nil
	}
}

func forwardEmployeeTurn(ctx context.Context, fwd forwarder, conversation, text string) (string, error) {
	action, control := parseConnectControlCommand(text)
	if !control {
		return fwd.forward(ctx, conversation, text)
	}
	if action.name == "clear" {
		if clearer, ok := fwd.(sessionClearer); ok {
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if err := clearer.clearSession(ctx, conversation); err != nil {
				return "", fmt.Errorf("session_clear_failed")
			}
			return action.ack, nil
		}
	}
	if resetter, ok := fwd.(sessionResetter); ok {
		resetter.resetSession(conversation)
		return action.ack, nil
	}
	return "当前渠道暂不支持会话指令（/new、/clear）。", nil
}

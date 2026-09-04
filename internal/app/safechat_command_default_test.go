// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build cgo && (darwin || linux || windows) && (amd64 || arm64)

package app

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
)

func TestCrossPlatformCoverageSafeChatIsInDefaultCGOBuild(t *testing.T) {
	if !msgcrypto.Available() {
		t.Fatal("default supported-platform CGO build must include SafeChat")
	}
	if got := newSafeChatCommand(); got == nil {
		t.Fatal("newSafeChatCommand() = nil in default CGO build")
	}
	cmd, _, err := NewRootCommand(context.Background()).Find([]string{"safechat"})
	if err != nil || cmd == nil || cmd.Name() != "safechat" {
		t.Fatalf("default root SafeChat command = %#v, %v", cmd, err)
	}
}

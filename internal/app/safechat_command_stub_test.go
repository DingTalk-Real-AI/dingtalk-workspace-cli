// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !(cgo && (darwin || linux || windows) && (amd64 || arm64))

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSafeChatStubRemainsFailClosed(t *testing.T) {
	if got := newSafeChatCommand(); got != nil {
		t.Fatalf("newSafeChatCommand() = %#v, want nil", got)
	}
	base := []*cobra.Command{{Use: "base"}}
	if got := appendOptionalCommand(base, nil); len(got) != 1 || got[0].Name() != "base" {
		t.Fatalf("append nil = %#v", got)
	}
	if got := appendOptionalCommand(base, &cobra.Command{Use: "extra"}); len(got) != 2 || got[1].Name() != "extra" {
		t.Fatalf("append command = %#v", got)
	}
	if cmd, _, err := NewRootCommand(context.Background()).Find([]string{"safechat"}); err == nil && cmd != nil && cmd.Name() == "safechat" {
		t.Fatalf("safechat command should be excluded from CGO-disabled root: %#v", cmd)
	}

	var out bytes.Buffer
	cmd := newSafeChatDecryptCommand()
	cmd.SetOut(&out)
	err := emitUnavailableSafeChatError(cmd, false, "plain unavailable")
	if !errors.Is(err, errors.New("plain unavailable")) && !strings.Contains(err.Error(), "plain unavailable") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "plain unavailable") {
		t.Fatalf("output = %q", out.String())
	}
	if err := validateSafeChatDecryptInput(nil, "cipher.txt", ""); err != nil {
		t.Fatalf("file-only input should be accepted: %v", err)
	}
}

//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"strings"
	"syscall"
	"testing"
)

func TestCrossPlatformCoverageEmployeeSkillFIFORejectedBeforeOpen(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := syscall.Mkfifo("blocked.zip", 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := deapAgentValidateSkillPackage("blocked.zip"); err == nil || !strings.Contains(err.Error(), "普通文件") {
		t.Fatalf("FIFO error = %v", err)
	}
}

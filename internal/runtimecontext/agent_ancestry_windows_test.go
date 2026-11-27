// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build windows

package runtimecontext

import (
	"errors"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"golang.org/x/sys/windows"
)

func TestCrossPlatformCoverageWindowsAncestryHelpers(t *testing.T) {
	if _, ok := windowsLookupProcess(map[int]windowsProcEntry{}, 999999); ok {
		t.Fatal("expected missing pid lookup to fail")
	}
	if got := windowsProcessStartTime(0); got != 0 {
		t.Fatalf("expected 0 start time for pid 0, got %d", got)
	}
	testseam.Swap(t, &windowsGetProcessTimes, func(windows.Handle, *windows.Filetime, *windows.Filetime, *windows.Filetime, *windows.Filetime) error {
		return errors.New("times failed")
	})
	if got := windowsProcessStartTime(os.Getpid()); got != 0 {
		t.Fatalf("expected 0 start time on GetProcessTimes error, got %d", got)
	}
	testseam.Swap(t, &windowsCreateProcessSnapshot, func(uint32, uint32) (windows.Handle, error) {
		return 0, errors.New("snapshot failed")
	})
	if _, ok := collectProcessAncestry(4); ok {
		t.Fatal("expected snapshot failure to return false")
	}
}

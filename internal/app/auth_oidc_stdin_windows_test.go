// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// CI runners have no attached console, so GetConsoleMode is stubbed to reach
// the console cancellation path deterministically.
func TestCrossPlatformCoverageSplitOIDCStdinWindowsConsoleCancel(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	wantHandle := windows.Handle(input.Fd())
	testseam.Swap(t, &oidcInputGetConsoleMode, func(handle windows.Handle, mode *uint32) error {
		if handle != wantHandle || mode == nil {
			t.Errorf("GetConsoleMode(%v, %v), want handle %v", handle, mode, wantHandle)
		}
		return nil
	})
	reader, cancelRead, cleanup, err := newOIDCFileInput(input)
	if err != nil || reader != input || cancelRead == nil || cleanup != nil {
		t.Fatalf("console input = %v, cancel=%t, cleanup=%t, %v", reader, cancelRead != nil, cleanup != nil, err)
	}

	t.Run("completed read", func(t *testing.T) {
		testseam.Swap(t, &oidcInputCancelIoEx, func(windows.Handle, *windows.Overlapped) error {
			t.Error("completed read was canceled")
			return nil
		})
		done := make(chan struct{})
		close(done)
		cancelRead(done)
	})

	t.Run("read returns after cancel", func(t *testing.T) {
		done := make(chan struct{})
		calls := 0
		testseam.Swap(t, &oidcInputCancelIoEx, func(windows.Handle, *windows.Overlapped) error {
			calls++
			close(done)
			return nil
		})
		cancelRead(done)
		if calls != 1 {
			t.Fatalf("CancelIoEx calls = %d, want 1", calls)
		}
	})

	t.Run("retry until read returns", func(t *testing.T) {
		done := make(chan struct{})
		calls := 0
		testseam.Swap(t, &oidcInputCancelIoEx, func(handle windows.Handle, overlapped *windows.Overlapped) error {
			calls++
			if handle != wantHandle || overlapped != nil {
				t.Errorf("CancelIoEx(%v, %v), want handle %v and nil overlapped", handle, overlapped, wantHandle)
			}
			if calls == 1 {
				return windows.ERROR_NOT_FOUND
			}
			close(done)
			return nil
		})
		cancelRead(done)
		if calls != 2 {
			t.Fatalf("CancelIoEx calls = %d, want 2", calls)
		}
	})

	t.Run("unexpected cancel error", func(t *testing.T) {
		calls := 0
		testseam.Swap(t, &oidcInputCancelIoEx, func(windows.Handle, *windows.Overlapped) error {
			calls++
			return windows.ERROR_INVALID_HANDLE
		})
		cancelRead(make(chan struct{}))
		if calls != 1 {
			t.Fatalf("CancelIoEx calls = %d, want 1", calls)
		}
	})
}

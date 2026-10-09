// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"golang.org/x/sys/windows"
)

func TestCrossPlatformCoverageEmployeeSnapshotWindowsReaderContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeEmployeeJSON(path, map[string]string{"status": "starting"}); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	attempts := 0
	testseam.Swap(t, &atomicRename, func(src, dst string) error {
		attempts++
		err := os.Rename(src, dst)
		if attempts == 1 {
			if !platformEmployeeRenameBusy(err) {
				t.Fatalf("expected real Windows reader contention, got %v", err)
			}
			// Release only after observing the actual OS error, not after a timer.
			if closeErr := reader.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		return err
	})
	if err := writeEmployeeJSON(path, map[string]string{"status": "running"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"status":"running"}` || attempts != 2 {
		t.Fatalf("snapshot=%s attempts=%d err=%v", data, attempts, err)
	}
}

func TestCrossPlatformCoverageEmployeeSnapshotWindowsBusyErrors(t *testing.T) {
	for _, err := range []error{windows.ERROR_SHARING_VIOLATION, windows.ERROR_ACCESS_DENIED} {
		if !platformEmployeeRenameBusy(&os.LinkError{Op: "rename", Err: err}) {
			t.Fatalf("Windows contention not classified: %v", err)
		}
	}
	if platformEmployeeRenameBusy(errors.New("storage failed")) {
		t.Fatal("permanent error classified as contention")
	}
}

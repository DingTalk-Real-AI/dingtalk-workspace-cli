// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"golang.org/x/sys/windows"
)

func TestCrossPlatformCoverageTokenMarkerWindowsReaderContention(t *testing.T) {
	dir := t.TempDir()
	if err := WriteTokenMarker(dir); err != nil {
		t.Fatal(err)
	}
	previous, _, err := ReadTokenMarkerRevision(dir)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(filepath.Join(dir, tokenJSONFile))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	attempts := 0
	testseam.Swap(t, &tokenRename, func(src, dst string) error {
		attempts++
		err := os.Rename(src, dst)
		if attempts == 1 {
			if !platformAuthFileRenameBusy(err) {
				t.Fatalf("expected real Windows reader contention, got %v", err)
			}
			// 观察到真实系统错误后释放读句柄，避免依赖定时器制造竞态。
			if closeErr := reader.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
		}
		return err
	})
	if err := WriteTokenMarker(dir); err != nil {
		t.Fatal(err)
	}
	current, present, err := ReadTokenMarkerRevision(dir)
	if err != nil || !present || current == previous || attempts != 2 {
		t.Fatalf("marker revision unchanged or retry failed: present=%v attempts=%d err=%v", present, attempts, err)
	}
}

func TestCrossPlatformCoverageTokenMarkerWindowsBusyErrors(t *testing.T) {
	for _, err := range []error{windows.ERROR_SHARING_VIOLATION, windows.ERROR_ACCESS_DENIED} {
		if !platformAuthFileRenameBusy(&os.LinkError{Op: "rename", Err: err}) {
			t.Fatalf("Windows contention not classified: %v", err)
		}
	}
	if platformAuthFileRenameBusy(errors.New("storage failed")) {
		t.Fatal("unrelated error classified as contention")
	}
}

//go:build unix

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageReadTextInputUsesOpenedDescriptorAfterPathReplacementE2E(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	path := filepath.Join(dir, "input.txt")
	replacement := filepath.Join(dir, "replacement.txt")
	if err := os.WriteFile(path, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("replacement-exceeds-limit"), 0o600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &openTextInputFile, func(candidate string) (*os.File, error) {
		file, openErr := os.Open(candidate)
		if openErr != nil {
			return nil, openErr
		}
		if renameErr := os.Rename(replacement, candidate); renameErr != nil {
			_ = file.Close()
			return nil, renameErr
		}
		return file, nil
	})
	got, err := ReadTextInput("@input.txt", nil, 10)
	if err != nil || got != "safe" {
		t.Fatalf("replacement read = %q, %v", got, err)
	}
}

func TestCrossPlatformCoverageReadTextInputRejectsUnconnectedFIFOWithoutBlockingE2E(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	fifo := filepath.Join(dir, "input.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, readErr := ReadTextInput("@input.fifo", nil, 1024)
		result <- readErr
	}()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "LOCAL_INPUT_INVALID") {
			t.Fatalf("FIFO rejection error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO without a writer blocked text input validation")
	}
}

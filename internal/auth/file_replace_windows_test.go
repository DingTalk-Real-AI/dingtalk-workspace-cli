// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAuthFileWritersWindowsReaderContention(t *testing.T) {
	for _, writer := range authFileWritersForTest() {
		t.Run(writer.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := writer.write(dir, "old"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, writer.file)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			attempts := 0
			testseam.Swap(t, writer.rename, func(src, dst string) error {
				attempts++
				err := os.Rename(src, dst)
				if attempts == 1 {
					if !platformAuthFileRenameBusy(err) {
						t.Fatalf("expected real Windows reader contention, got %v", err)
					}
					if closeErr := reader.Close(); closeErr != nil {
						t.Fatal(closeErr)
					}
				}
				return err
			})
			if err := writer.write(dir, "new"); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || bytes.Equal(after, before) || attempts != 2 {
				t.Fatalf("replacement failed: attempts=%d err=%v", attempts, err)
			}
		})
	}
}

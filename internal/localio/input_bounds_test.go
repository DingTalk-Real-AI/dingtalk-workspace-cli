// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package localio

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageReadTextInputUsesOpenedDescriptorAndBoundedReadE2E(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	t.Run("file grows after stat", func(t *testing.T) {
		path := filepath.Join(dir, "grow.txt")
		if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &readTextInputAll, func(reader io.Reader) ([]byte, error) {
			file, openErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if openErr != nil {
				return nil, openErr
			}
			if _, writeErr := file.WriteString("-too-large"); writeErr != nil {
				_ = file.Close()
				return nil, writeErr
			}
			if closeErr := file.Close(); closeErr != nil {
				return nil, closeErr
			}
			return io.ReadAll(reader)
		})
		if _, err := ReadTextInput("@grow.txt", nil, 4); err == nil {
			t.Fatal("file growth bypassed size limit")
		}
	})

	t.Run("path becomes directory before descriptor check", func(t *testing.T) {
		path := filepath.Join(dir, "becomes-directory.txt")
		if err := os.WriteFile(path, []byte("safe"), 0o600); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &openTextInputFile, func(candidate string) (*os.File, error) {
			if removeErr := os.Remove(candidate); removeErr != nil {
				return nil, removeErr
			}
			if mkdirErr := os.Mkdir(candidate, 0o700); mkdirErr != nil {
				return nil, mkdirErr
			}
			return os.Open(candidate)
		})
		if _, err := ReadTextInput("@becomes-directory.txt", nil, 10); err == nil {
			t.Fatal("path type replacement bypassed descriptor validation")
		}
	})

	t.Run("file grows before descriptor check", func(t *testing.T) {
		path := filepath.Join(dir, "grows-before-stat.txt")
		if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &openTextInputFile, func(candidate string) (*os.File, error) {
			file, openErr := os.Open(candidate)
			if openErr != nil {
				return nil, openErr
			}
			writer, writeOpenErr := os.OpenFile(candidate, os.O_APPEND|os.O_WRONLY, 0)
			if writeOpenErr != nil {
				_ = file.Close()
				return nil, writeOpenErr
			}
			if _, writeErr := writer.WriteString("-too-large"); writeErr != nil {
				_ = writer.Close()
				_ = file.Close()
				return nil, writeErr
			}
			if closeErr := writer.Close(); closeErr != nil {
				_ = file.Close()
				return nil, closeErr
			}
			return file, nil
		})
		if _, err := ReadTextInput("@grows-before-stat.txt", nil, 4); err == nil {
			t.Fatal("pre-open growth bypassed descriptor size validation")
		}
	})
}

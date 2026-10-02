// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageTokenMarkerReplacementRetry(t *testing.T) {
	busy := errors.New("reader holds marker")
	permanent := errors.New("storage unavailable")
	for _, tc := range []struct {
		name     string
		failure  error
		failures int
		attempts int
		wantErr  error
	}{
		{"released reader", busy, 1, 2, nil},
		{"persistent reader", busy, 11, 11, busy},
		{"unrelated failure", permanent, 1, 1, permanent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tokenJSONFile)
			before := []byte(`{"revision":"old"}`)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &authFileRenameBusy, func(err error) bool { return errors.Is(err, busy) })
			attempts := 0
			var staged string
			testseam.Swap(t, &tokenRename, func(src, dst string) error {
				attempts++
				if staged != "" && staged != src {
					t.Fatal("retry replaced the staged file")
				}
				staged = src
				current, err := os.ReadFile(dst)
				if err != nil || !bytes.Equal(current, before) {
					t.Fatalf("failed replacement changed old marker: %v", err)
				}
				if attempts <= tc.failures {
					return tc.failure
				}
				return os.Rename(src, dst)
			})
			err := WriteTokenMarker(dir)
			if !errors.Is(err, tc.wantErr) || attempts != tc.attempts {
				t.Fatalf("error=%v attempts=%d, want %v/%d", err, attempts, tc.wantErr, tc.attempts)
			}
			after, err := os.ReadFile(path)
			if err != nil || (tc.wantErr != nil) != bytes.Equal(after, before) {
				t.Fatalf("marker publication did not match result: %v", err)
			}
			files, err := filepath.Glob(filepath.Join(dir, tokenJSONFile+".*.tmp"))
			if err != nil || len(files) != 0 {
				t.Fatalf("staged marker leaked: %v %v", files, err)
			}
		})
	}
	if runtime.GOOS != "windows" && platformAuthFileRenameBusy(os.ErrPermission) {
		t.Fatal("non-Windows permission error must not be retried")
	}
}

func TestCrossPlatformCoverageTokenMarkerPartialWriteCleanup(t *testing.T) {
	dir := t.TempDir()
	failure := errors.New("partial write")
	testseam.Swap(t, &tokenWriteFile, func(path string, data []byte, mode os.FileMode) error {
		if err := os.WriteFile(path, data[:1], mode); err != nil {
			t.Fatal(err)
		}
		return failure
	})
	if err := WriteTokenMarker(dir); !errors.Is(err, failure) {
		t.Fatalf("partial write error = %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("partial marker leaked: %v %v", files, err)
	}
}

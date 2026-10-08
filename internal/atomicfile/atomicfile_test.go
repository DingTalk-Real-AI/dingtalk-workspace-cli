// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type failingTemp struct {
	stage  string
	closed int
}

func (f *failingTemp) Name() string { return "temp" }
func (f *failingTemp) failure(stage string) error {
	if f.stage == stage {
		return errors.New(stage)
	}
	return nil
}
func (f *failingTemp) Write(p []byte) (int, error) { return len(p), f.failure("write") }
func (f *failingTemp) Chmod(os.FileMode) error     { return f.failure("chmod") }
func (f *failingTemp) Sync() error                 { return f.failure("sync") }
func (f *failingTemp) Close() error                { f.closed++; return f.failure("close") }

func TestCrossPlatformCoverageAtomicWriteFailureCleanup(t *testing.T) {
	for _, stage := range []string{"mkdir", "create", "chmod", "write", "sync", "close", "rename", "success"} {
		t.Run(stage, func(t *testing.T) {
			f := &failingTemp{stage: stage}
			removed, renamed := false, false
			ops := Ops{
				MkdirAll:   func(string, os.FileMode) error { return f.failure("mkdir") },
				CreateTemp: func(string, string) (TempFile, error) { return f, f.failure("create") },
				Remove: func(name string) error {
					removed = true
					if name != "temp" {
						t.Fatal(name)
					}
					return nil
				},
				Rename: func(from, to string) error {
					renamed = true
					if from != "temp" || to != "target" {
						t.Fatalf("rename %s %s", from, to)
					}
					return f.failure("rename")
				},
			}
			err := WriteWithOps("target", 0600, ops, func(tmp TempFile) error { _, err := tmp.Write([]byte("data")); return err })
			if stage == "success" {
				if err != nil || removed || !renamed || f.closed != 1 {
					t.Fatalf("err=%v removed=%v renamed=%v closed=%d", err, removed, renamed, f.closed)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), stage) {
					t.Fatalf("stage=%s err=%v", stage, err)
				}
				if stage != "mkdir" && stage != "create" && (!removed || f.closed == 0) {
					t.Fatal("failed temporary file was not cleaned")
				}
				if stage != "rename" && renamed {
					t.Fatal("renamed after failed write")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAtomicWritePrivateReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	for _, data := range []string{`{"version":1}`, `{"version":2}`} {
		if err := WriteJSON(path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != data {
			t.Fatalf("data=%q err=%v", got, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", info.Mode())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files: %v %v", entries, err)
	}
}

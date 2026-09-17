// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageEmployeeSnapshotAllowsAtomicReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := AtomicWriteJSON(path, []byte(`{"status":"starting"}`)); err != nil {
		t.Fatal(err)
	}
	reader, err := openEmployeeSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// Keep the old handle open during replacement: this deterministically
	// reproduces Windows sharing violations instead of depending on polling.
	if err := AtomicWriteJSON(path, []byte(`{"status":"running"}`)); err != nil {
		t.Fatalf("snapshot reader blocked atomic replacement: %v", err)
	}
	old, err := io.ReadAll(reader)
	if err != nil || string(old) != `{"status":"starting"}` {
		t.Fatalf("old snapshot changed: %q, %v", old, err)
	}
	current, err := employeeReadFile(path)
	if err != nil || string(current) != `{"status":"running"}` {
		t.Fatalf("new snapshot unavailable: %q, %v", current, err)
	}
}

func TestCrossPlatformCoverageEmployeeSnapshotReadErrors(t *testing.T) {
	if _, err := readEmployeeSnapshot(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing snapshot error: %v", err)
	}
	if _, err := readEmployeeSnapshot("invalid\x00path"); err == nil {
		t.Fatal("invalid path accepted")
	}
}

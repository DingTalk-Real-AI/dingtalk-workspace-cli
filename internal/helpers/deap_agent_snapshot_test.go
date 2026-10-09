// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeSnapshotRenameBoundaries(t *testing.T) {
	for _, scenario := range []string{"success", "transient", "exhausted", "permanent"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			old := []byte(`{"status":"starting"}`)
			if err := AtomicWriteJSON(path, old); err != nil {
				t.Fatal(err)
			}
			busy, permanent := errors.New("busy snapshot"), errors.New("permanent failure")
			testseam.Swap(t, &employeeRenameBusy, func(err error) bool { return errors.Is(err, busy) })
			attempts := 0
			testseam.Swap(t, &atomicRename, func(src, dst string) error {
				attempts++
				if scenario == "exhausted" || (scenario == "transient" && attempts < 3) {
					return busy
				}
				if scenario == "permanent" {
					return permanent
				}
				return os.Rename(src, dst)
			})
			err := writeEmployeeJSON(path, map[string]string{"status": "running"})
			want, calls := `{"status":"running"}`, 1
			switch scenario {
			case "transient":
				calls = 3
			case "exhausted":
				want, calls = string(old), 11
				if !errors.Is(err, busy) {
					t.Fatalf("lost contention error: %v", err)
				}
			case "permanent":
				want = string(old)
				if !errors.Is(err, permanent) {
					t.Fatalf("lost permanent error: %v", err)
				}
			}
			if (scenario == "success" || scenario == "transient") && err != nil {
				t.Fatal(err)
			}
			if attempts != calls {
				t.Fatalf("rename attempts=%d, want %d", attempts, calls)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != want {
				t.Fatalf("snapshot=%q, want %q, err=%v", data, want, readErr)
			}
			files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
			if len(files) != 0 {
				t.Fatalf("temporary snapshots leaked: %v", files)
			}
		})
	}
}

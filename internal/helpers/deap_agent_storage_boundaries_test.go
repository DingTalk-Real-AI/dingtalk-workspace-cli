// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeDurableStorageFailures(t *testing.T) {
	for _, scenario := range []string{"audit-json", "audit-write", "state-json", "state-write", "task-stat", "task-list", "task-read", "task-audit", "task-write", "binding-json", "binding-stat", "binding-list", "binding-read"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			r, e := employeeLedgerFixture(t)
			if err := writeEmployeeJSON(r.recordPath(e), employeeTaskRecord{Status: "sending"}); err != nil {
				t.Fatal(err)
			}
			boom := errors.New("storage unavailable")
			switch scenario {
			case "audit-json":
				testseam.Swap(t, &employeeMarshalJSON, func(any) ([]byte, error) { return nil, boom })
				if err := r.audit(employeeTaskRecord{}); err == nil {
					t.Fatal("audit marshal error ignored")
				}
			case "audit-write", "state-write":
				testseam.Swap(t, &employeeOpenFile, func(name string, _ int, _ os.FileMode) (*os.File, error) { return os.Open(name) })
				if err := os.WriteFile(filepath.Join(r.dir, "audit.jsonl"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(r.dir, "runtime.log"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if scenario == "audit-write" {
					err = r.audit(employeeTaskRecord{})
				} else {
					err = persistEmployeeState(r.dir, digitalEmployeeRunState{})
				}
				if err == nil {
					t.Fatal("read-only descriptor accepted")
				}
			case "state-json":
				testseam.Swap(t, &employeeMarshalJSON, func(any) ([]byte, error) { return nil, boom })
				if err := persistEmployeeState(r.dir, digitalEmployeeRunState{}); err == nil {
					t.Fatal("state marshal error ignored")
				}
			case "task-stat":
				testseam.Swap(t, &employeeStat, func(string) (os.FileInfo, error) { return nil, boom })
				if err := r.enqueue(e); err == nil {
					t.Fatal("task stat failure ignored")
				}
			case "task-list", "task-read", "task-audit", "task-write":
				switch scenario {
				case "task-list":
					testseam.Swap(t, &employeeReadDir, func(string) ([]os.DirEntry, error) { return nil, boom })
				case "task-read":
					testseam.Swap(t, &employeeReadFile, func(string) ([]byte, error) { return nil, boom })
				case "task-audit":
					testseam.Swap(t, &employeeOpenFile, func(string, int, os.FileMode) (*os.File, error) { return nil, boom })
				case "task-write":
					testseam.Swap(t, &atomicRename, func(string, string) error { return boom })
				}
				if err := r.recoverInterruptedTasks(); err == nil {
					t.Fatal("recovery failure ignored")
				}
			case "binding-json":
				testseam.Swap(t, &employeeMarshalIndent, func(any, string, string) ([]byte, error) { return nil, boom })
				if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding); err == nil {
					t.Fatal("binding marshal error ignored")
				}
			case "binding-stat", "binding-list", "binding-read":
				if scenario == "binding-stat" {
					testseam.Swap(t, &employeeStat, func(string) (os.FileInfo, error) { return nil, boom })
				}
				if scenario == "binding-list" {
					testseam.Swap(t, &employeeReadDir, func(string) ([]os.DirEntry, error) { return nil, boom })
				}
				if scenario == "binding-read" {
					testseam.Swap(t, &employeeReadFile, func(string) ([]byte, error) { return nil, boom })
				}
				if _, err := employeeFindBindings(cfg.Binding.AgentUUID); err == nil {
					t.Fatal("binding store failure ignored")
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeLocksCannotBeStolen(t *testing.T) {
	for _, kind := range []string{"worker", "activation", "supervisor", "binding"} {
		t.Run(kind, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			path := filepath.Join(dir, kind)
			if kind == "binding" {
				path = filepath.Join(deapConnectConfigDir(), "digital-employees", digitalEmployeeScope(cfg.Binding.DWSProfile), "binding-lock")
			}
			// A regular file as parent makes lock persistence fail immediately,
			// rather than waiting for the default lock acquisition timeout.
			if kind == "binding" {
				// Test an unwritable lock root without changing the existing binding.
				blocked := filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(blocked, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := saveDigitalEmployeeBinding(blocked, cfg.Binding); err == nil {
					t.Fatal("unavailable binding lock accepted")
				}
				return
			}
			lock, err := auth.AcquireDualLock(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Release()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if kind == "supervisor" {
				err = superviseDigitalEmployee(ctx, cfg)
			} else {
				err = runEmployeeWorker(ctx, cfg, 0)
			}
			if err == nil {
				t.Fatal("occupied runtime lock was stolen")
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeLifecycleReloadFailures(t *testing.T) {
	for _, action := range []string{"stop", "unbind", "binding"} {
		t.Run(action, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			reads := 0
			old := employeeReadFile
			testseam.Swap(t, &employeeReadFile, func(path string) ([]byte, error) {
				if path == digitalEmployeeBindingPath(deapConnectConfigDir(), b.DWSProfile) {
					reads++
					if reads >= 3 {
						return nil, errors.New("binding changed")
					}
				}
				return old(path)
			})
			cmd := lifecycleCmd(t, "unbind", b.AgentUUID)
			if action == "binding" {
				auth.SetRuntimeProfile(b.DWSProfile)
				cmd = newEmployeeBindingCommand()
				cmd.SetContext(context.Background())
				_ = cmd.Flags().Set("channel", "dsh")
				_ = cmd.Flags().Set("stdin", "true")
				cmd.SetIn(strings.NewReader(`{"agentUuid":"` + b.AgentUUID + `","bindingRevision":7}`))
				if err := cmd.RunE(cmd, nil); err == nil {
					t.Fatal("changed binding accepted")
				}
			} else {
				var err error
				if action == "unbind" {
					err = runEmployeeUnbind(cmd)
				} else {
					err = runDigitalEmployeeLifecycle(cmd, action)
				}
				if err == nil {
					t.Fatal("changed binding accepted")
				}
			}
		})
	}
}

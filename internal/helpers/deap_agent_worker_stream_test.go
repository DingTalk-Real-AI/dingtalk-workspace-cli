// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestEmployeeStreamBoundaryFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "employee-stream-boundary" {
		return
	}
	mode := os.Getenv("DWS_STREAM_BOUNDARY")
	if mode == "hang-exit" {
		signal.Ignore(syscall.SIGTERM)
	}
	if mode == "cancel-start" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if mode == "exit-before-ready" {
		fmt.Fprintln(os.Stderr, `{"retryable":false}`)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[event] ready")
	fmt.Fprintln(os.Stderr, `[event] transport {"state":"connected","source":"inferred","observed":true}`)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := readDigitalEmployeeState(os.Getenv("DWS_STREAM_BOUNDARY_DIR"))
		if err == nil && state.Status == "running" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	switch mode {
	case "stderr-oversized":
		fmt.Fprint(os.Stderr, strings.Repeat("x", digitalEmployeeStdinLimit+1))
	case "stdout-oversized":
		fmt.Print(strings.Repeat("x", digitalEmployeeStdinLimit+1))
	case "exit-after-ready":
		os.Exit(1)
	case "unobserved":
		fmt.Fprintln(os.Stderr, `[event] transport {"state":"connected","source":"unknown","observed":false}`)
	case "transport-write":
		fmt.Fprintln(os.Stderr, `[event] transport {"state":"reconnecting","source":"inferred","observed":true}`)
	case "enqueue", "fatal":
		fmt.Println(`{"type":"user_im_message_receive_o2o_all","event_id":"event","message_id":"message","conversation_id":"conversation","sender_open_dingtalk_id":"owner","content":"private-input"}`)
	case "hang-exit":
		time.Sleep(30 * time.Second)
	case "flood-stdout":
		for i := 0; i < 10000; i++ {
			fmt.Println(`{"type":"ignored","sender_open_dingtalk_id":"self"}`)
		}
	case "flood-stderr":
		for i := 0; i < 10000; i++ {
			fmt.Fprintln(os.Stderr, `[event] transport {"state":"connected","source":"inferred","observed":true}`)
		}
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestCrossPlatformCoverageEmployeeWorkerStreamFailures(t *testing.T) {
	for _, scenario := range []string{"prepare", "executor-state", "audit", "ledger", "command", "transport-state", "running-state", "exit-before-ready", "stderr-oversized", "stdout-oversized", "exit-after-ready", "unobserved", "transport-write", "enqueue", "fatal", "cancel-start", "cancel-running", "hang-exit", "flood-stdout", "flood-stderr"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := employeeBoundaryConfig(t)
			cfg.Options.AllowedUsers = []string{"owner"}
			cfg.Options.AllowedGroups = []string{"group"}
			dir := digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile)
			closed := false
			testseam.Swap(t, &digitalEmployeeNewForwarder, func(context.Context, digitalEmployeeAdapterConfig) (forwarder, error) {
				if scenario == "prepare" {
					return &execForwarder{}, nil
				}
				return &employeeClosingForwarder{closed: &closed}, nil
			})
			t.Setenv("DWS_STREAM_BOUNDARY", scenario)
			t.Setenv("DWS_STREAM_BOUNDARY_DIR", dir)
			testseam.Swap(t, &employeeExecCommand, func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmployeeStreamBoundaryFixture$", "--", "employee-stream-boundary")
				if scenario == "hang-exit" {
					cmd.Cancel = func() error { return nil }
				}
				return cmd
			})
			if scenario == "command" {
				testseam.Swap(t, &daemonExecutable, func() (string, error) { return "", errors.New("no executable") })
			}
			if scenario == "audit" {
				if err := os.MkdirAll(filepath.Join(dir, "audit.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "ledger" {
				if err := os.WriteFile(filepath.Join(dir, "tasks"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			writes, tasks := 0, 0
			testseam.Swap(t, &atomicRename, func(src, dst string) error {
				if filepath.Base(dst) == "state.json" {
					writes++
				}
				if filepath.Dir(dst) == filepath.Join(dir, "tasks") {
					tasks++
				}
				fail := (scenario == "executor-state" && writes == 2) || (scenario == "transport-state" && writes == 3) || (scenario == "running-state" && writes == 4) || (scenario == "transport-write" && writes == 5) || (scenario == "enqueue" && tasks == 1) || (scenario == "fatal" && tasks == 2)
				if fail {
					return errors.New("storage failed")
				}
				return os.Rename(src, dst)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if scenario == "hang-exit" {
				testseam.Swap(t, &employeeConsumerStopTimeout, time.Millisecond)
			}
			graceful := scenario == "cancel-start" || scenario == "cancel-running" || scenario == "hang-exit" || strings.HasPrefix(scenario, "flood-")
			if graceful {
				done := make(chan struct{})
				go func() {
					defer close(done)
					for ctx.Err() == nil {
						s, e := readDigitalEmployeeState(dir)
						if e == nil && ((scenario == "cancel-start" && s.ExecutorReady) || s.Status == "running") {
							time.Sleep(30 * time.Millisecond)
							cancel()
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
				}()
				defer func() { cancel(); <-done }()
			}
			var err error
			if scenario == "cancel-running" {
				cmd := newDeapConnectCommand()
				cmd.SetContext(ctx)
				cmd.SetOut(io.Discard)
				err = runDigitalEmployeeForeground(cmd, cfg)
			} else {
				err = runEmployeeWorker(ctx, cfg, 0)
			}
			if graceful {
				if err != nil {
					t.Fatal(err)
				}
				if !closed {
					t.Fatal("forwarder not closed")
				}
				return
			}
			if err == nil {
				t.Fatal("failed worker reported graceful stop")
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("private details escaped")
			}
		})
	}
}

type employeeClosingForwarder struct {
	employeeBoundaryForwarder
	closed *bool
}

func (f *employeeClosingForwarder) close() error { *f.closed = true; return nil }

func TestCrossPlatformCoverageEmployeeConsumerExitDistinguishesStop(t *testing.T) {
	failure := &employeeRunError{Code: "consumer_exit"}
	if err := employeeConsumerExit(context.Background(), failure); err != failure {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := employeeConsumerExit(ctx, failure); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeeBlockedStdoutReaderCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lines := make(chan []byte) // No consumer: cancellation is the only ready case.
	done := make(chan struct{})
	go func() {
		defer close(done)
		employeeReadEvents(ctx, strings.NewReader("event\n"), lines, make(chan error, 1))
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stdout backpressure blocked cancellation")
	}
}

func TestCrossPlatformCoverageEmployeeRetryHintAndRecoveryDetails(t *testing.T) {
	var failure employeeRunError
	raw, _ := json.Marshal([]any{map[string]any{"next_retry_at": time.Now().Add(time.Hour).Format(time.RFC3339), "retryable": true}})
	employeeReadRetryHint(string(raw), &failure)
	if failure.RetryAfter < 59*time.Minute || failure.Retryable == nil || !*failure.Retryable {
		t.Fatalf("retry hint=%+v", failure)
	}
	if _, ok := employeeNextRetry(employeeRetryState{}, errors.New("unknown exit"), time.Now()); !ok {
		t.Fatal("first unknown failure cannot retry")
	}
	r, _ := employeeLedgerFixture(t)
	if err := os.MkdirAll(filepath.Join(r.dir, "tasks", "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, "tasks", "other.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverInterruptedTasks(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, "tasks", "invalid.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.recoverInterruptedTasks(); err == nil {
		t.Fatal("corrupt ledger accepted")
	}
}

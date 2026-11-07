// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeePrepareProtocols(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("unavailable")
	testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) { return nil, boom })
	if err := prepareEmployeeForwarder(ctx, &codexAppServerForwarder{}); !errors.Is(err, boom) {
		t.Fatalf("codex initialization: %v", err)
	}
	testseam.Swap(t, &codexNewAppServerClient, func(context.Context, string, []string, string) (*codexAppServerClient, error) {
		return unitCodexClient(&bufferWriteCloser{}, codexRPCMessage{ID: codexIntPtr(1), Result: json.RawMessage(`{}`)}), nil
	})
	if err := prepareEmployeeForwarder(ctx, &codexAppServerForwarder{}); err != nil {
		t.Fatal(err)
	}
	if err := prepareEmployeeForwarder(ctx, activeQoderForwarder(&qoderTestWriteCloser{})); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &opencodeFreeLocalPort, func() (int, error) { return 0, boom })
	if err := prepareEmployeeForwarder(ctx, &opencodeForwarder{server: newOpencodeServer("missing", nil, "", false)}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	for _, argv := range [][]string{nil, {filepath.Join(t.TempDir(), "missing")}} {
		if err := prepareEmployeeForwarder(ctx, &execForwarder{argv: argv}); err == nil {
			t.Fatal("missing executable accepted")
		}
	}
	if err := prepareEmployeeForwarder(ctx, &employeeTestForwarder{}); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageEmployeePrivateAgentStartup(t *testing.T) {
	for _, exitErr := range []error{nil, errors.New("private process detail")} {
		t.Run("qoder", func(t *testing.T) {
			f := activeQoderForwarder(&qoderTestWriteCloser{})
			f.privateDiagnostics = true
			f.done = make(chan error, 1)
			f.done <- exitErr
			testseam.Swap(t, &qoderExecCommand, func(string, ...string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing")) })
			if err := prepareEmployeeForwarder(context.Background(), f); err == nil {
				t.Fatal("missing restart executable accepted")
			}
		})
	}
	testseam.Swap(t, &opencodeExecCommand, func(string, ...string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing")) })
	s := newOpencodeServer("missing", nil, "", false)
	s.privateDiagnostics = true
	if _, err := s.ensure(context.Background()); err == nil {
		t.Fatal("missing private opencode accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := digitalEmployeeNewForwarder(ctx, digitalEmployeeAdapterConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

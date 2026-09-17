// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

const (
	runtimeTokenDetachedChildEnv     = "DWS_EVENT_RUNTIME_TOKEN_E2E_CHILD"
	runtimeTokenDetachedWorkDirEnv   = "DWS_EVENT_RUNTIME_TOKEN_E2E_WORKDIR"
	runtimeTokenDetachedEndpointEnv  = "DWS_EVENT_RUNTIME_TOKEN_E2E_ENDPOINT"
	runtimeTokenDetachedEvidenceEnv  = "DWS_EVENT_RUNTIME_TOKEN_E2E_EVIDENCE"
	runtimeTokenDetachedCanaryA      = "dws-runtime-e2e-A-9f34c8d10b7e"
	runtimeTokenDetachedCanaryB      = "dws-runtime-e2e-B-2ad761e5c490"
	runtimeTokenDetachedClientID     = "runtime-e2e-client"
	runtimeTokenDetachedIdentityHash = "90abcdef12345678"
	runtimeTokenDetachedSourceID     = "runtime-e2e-source"
)

// runRuntimeTokenDetachedE2EChild is called at the very start of TestMain.
// busctl.Spawn executes this test binary with production-style `event _bus`
// arguments; the env marker lets the child run a real bus daemon before the Go
// test runner attempts to parse those CLI arguments.
func runRuntimeTokenDetachedE2EChild() (int, bool) {
	if os.Getenv(runtimeTokenDetachedChildEnv) != "1" {
		return 0, false
	}
	workDir := strings.TrimSpace(os.Getenv(runtimeTokenDetachedWorkDirEnv))
	endpoint := strings.TrimSpace(os.Getenv(runtimeTokenDetachedEndpointEnv))
	evidence := strings.TrimSpace(os.Getenv(runtimeTokenDetachedEvidenceEnv))
	if workDir == "" || endpoint == "" || evidence == "" {
		return 91, true
	}

	argvClean := !runtimeTokenDetachedContainsCanary(strings.Join(os.Args, "\x00"))
	envClean := !runtimeTokenDetachedContainsCanary(strings.Join(os.Environ(), "\x00"))
	if err := appendRuntimeTokenDetachedEvidence(evidence,
		fmt.Sprintf("child_start argv_clean=%t env_clean=%t", argvClean, envClean)); err != nil {
		return 92, true
	}
	if !argvClean || !envClean {
		return 93, true
	}

	logFile, err := os.OpenFile(filepath.Join(workDir, "bus.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 94, true
	}
	defer logFile.Close()

	broker := runtimecred.New(runtimecred.Config{RequireSeed: true, RequireActivation: true})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = bus.Run(ctx, bus.Config{
		WorkDir:          workDir,
		IPCEndpoint:      endpoint,
		ClientID:         runtimeTokenDetachedClientID,
		SourceKind:       dwsevent.SourceKindPersonalStream,
		IdentityHash:     runtimeTokenDetachedIdentityHash,
		SourceID:         runtimeTokenDetachedSourceID,
		Edition:          "open",
		SDKVersion:       "runtime-e2e",
		Source:           &runtimeTokenDetachedSource{broker: broker, evidence: evidence},
		CredentialBroker: broker,
		ReadyPipe:        busctl.ReadyFDFromEnv(),
		Logger:           slog.New(slog.NewTextHandler(logFile, nil)),
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		_ = appendRuntimeTokenDetachedEvidence(evidence, "bus_exit clean=false")
		return 95, true
	}
	_ = appendRuntimeTokenDetachedEvidence(evidence, "bus_exit clean=true")
	return 0, true
}

type runtimeTokenDetachedSource struct {
	broker   *runtimecred.Broker
	evidence string
}

// Start models the credential-sensitive part of a reconnecting Stream source
// without network access. It resolves A for the first connection, waits until a
// second consumer rotates the broker to B, then exercises the exact 401 path:
// RefreshRejected(A) must return B and must not fall back to local OAuth.
func (s *runtimeTokenDetachedSource) Start(ctx context.Context, _ dwsevent.EmitFn) error {
	first, err := s.broker.Resolve(ctx)
	if err != nil {
		return errors.New("runtime e2e: initial credential unavailable")
	}
	if first != runtimeTokenDetachedCanaryA || s.broker.Generation() != 1 {
		return errors.New("runtime e2e: initial credential mismatch")
	}
	if err := appendRuntimeTokenDetachedEvidence(s.evidence, "resolved_a=true generation=1"); err != nil {
		return errors.New("runtime e2e: record initial connection")
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for s.broker.Generation() < 2 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	rotated, err := s.broker.RefreshRejected(ctx, first)
	if err != nil || rotated != runtimeTokenDetachedCanaryB {
		return errors.New("runtime e2e: rotated credential unavailable")
	}
	if err := appendRuntimeTokenDetachedEvidence(s.evidence, "rejected_a=true resolved_b=true reconnect=true generation=2"); err != nil {
		return errors.New("runtime e2e: record reconnect")
	}
	<-ctx.Done()
	return ctx.Err()
}

func appendRuntimeTokenDetachedEvidence(path, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}

func runtimeTokenDetachedContainsCanary(value string) bool {
	return strings.Contains(value, runtimeTokenDetachedCanaryA) ||
		strings.Contains(value, runtimeTokenDetachedCanaryB)
}

func TestCrossPlatformCoverageUnixDetachedRuntimeTokenLifecycleAndCanaryLeakScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("real detached-process lifecycle is Unix-only; Windows named-pipe code is cross-compiled separately")
	}

	root, err := os.MkdirTemp("/tmp", "dws-runtime-token-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	workDir := filepath.Join(root, "events", "open", string(dwsevent.SourceKindPersonalStream), runtimeTokenDetachedIdentityHash)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	endpoint := dwsevent.IPCEndpoint(workDir, "open", dwsevent.SourceKindPersonalStream, runtimeTokenDetachedIdentityHash)
	evidencePath := filepath.Join(workDir, "runtime-e2e.evidence")

	identity := personal.Identity{
		ClientID: runtimeTokenDetachedClientID,
		SourceID: runtimeTokenDetachedSourceID,
		CorpID:   "runtime-e2e-corp",
		UserID:   "runtime-e2e-user",
	}
	spawnArgs := personalBusSpawnArgsForToken(identity, runtimeTokenDetachedIdentityHash, "", "", "corp:user", runtimeTokenDetachedCanaryA)
	assertRuntimeTokenDetachedClean(t, "spawn argv", []byte(strings.Join(spawnArgs, "\x00")))

	childEnv := append([]string{}, os.Environ()...)
	childEnv = append(childEnv,
		runtimeTokenDetachedChildEnv+"=1",
		runtimeTokenDetachedWorkDirEnv+"="+workDir,
		runtimeTokenDetachedEndpointEnv+"="+endpoint,
		runtimeTokenDetachedEvidenceEnv+"="+evidencePath,
	)
	assertRuntimeTokenDetachedClean(t, "spawn environment", []byte(strings.Join(childEnv, "\x00")))

	pid, err := busctl.Spawn(busctl.SpawnConfig{
		ExecPath:  os.Args[0],
		ClientID:  runtimeTokenDetachedClientID,
		ExtraArgs: spawnArgs,
		Env:       childEnv,
	})
	if err != nil {
		failRuntimeTokenDetachedError(t, "spawn detached runtime bus", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = busctl.Stop(busctl.StopConfig{WorkDir: workDir, Timeout: 2 * time.Second})
			if proc, findErr := os.FindProcess(pid); findErr == nil {
				_ = proc.Kill()
			}
		}
	})

	waitRuntimeTokenDetachedFile(t, evidencePath, "child_start argv_clean=true env_clean=true", 3*time.Second)

	var stdoutA, stderrA bytes.Buffer
	err = consume.Run(context.Background(), runtimeTokenDetachedConsumeConfig(
		workDir, endpoint, "sub-runtime-a", runtimeTokenDetachedCanaryA, 500*time.Millisecond, &stdoutA, &stderrA,
	))
	if err != nil {
		failRuntimeTokenDetachedError(t, "consume token A", err)
	}
	waitRuntimeTokenDetachedFile(t, evidencePath, "resolved_a=true generation=1", 3*time.Second)

	if err := personal.UpsertRunState(workDir, personal.RunState{
		SubscribeID:  "sub-runtime-b",
		EventKey:     personal.EventMention,
		ClientID:     runtimeTokenDetachedClientID,
		SourceID:     runtimeTokenDetachedSourceID,
		IdentityHash: runtimeTokenDetachedIdentityHash,
	}); err != nil {
		failRuntimeTokenDetachedError(t, "persist non-sensitive run state", err)
	}

	var stdoutB, stderrB bytes.Buffer
	consumeDone := make(chan error, 1)
	go func() {
		consumeDone <- consume.Run(context.Background(), runtimeTokenDetachedConsumeConfig(
			workDir, endpoint, "sub-runtime-b", runtimeTokenDetachedCanaryB, 5*time.Second, &stdoutB, &stderrB,
		))
	}()

	status := waitRuntimeTokenDetachedStatus(t, endpoint, "sub-runtime-b", 3*time.Second)
	if status.Bus.PID != pid || status.Bus.IdentityHash != runtimeTokenDetachedIdentityHash {
		t.Fatalf("status bus identity = %#v, want pid=%d identity=%s", status.Bus, pid, runtimeTokenDetachedIdentityHash)
	}
	waitRuntimeTokenDetachedFile(t, evidencePath, "rejected_a=true resolved_b=true reconnect=true generation=2", 3*time.Second)

	stopResp, err := busctl.StopConsumers(endpoint, []string{"sub-runtime-b"})
	if err != nil {
		failRuntimeTokenDetachedError(t, "targeted consumer stop", err)
	}
	if len(stopResp.Stopped) != 1 || stopResp.Stopped[0] != "sub-runtime-b" {
		t.Fatalf("targeted stop response = %#v", stopResp)
	}
	select {
	case err := <-consumeDone:
		if err != nil {
			failRuntimeTokenDetachedError(t, "consume token B after targeted stop", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("token B consumer did not exit after targeted stop")
	}
	status = waitRuntimeTokenDetachedStatus(t, endpoint, "", 3*time.Second)
	if len(status.Consumers) != 0 {
		t.Fatalf("status consumers after stop = %#v", status.Consumers)
	}

	if err := busctl.Stop(busctl.StopConfig{WorkDir: workDir, Timeout: 4 * time.Second}); err != nil {
		failRuntimeTokenDetachedError(t, "stop detached bus", err)
	}
	stopped = true
	waitRuntimeTokenDetachedFile(t, evidencePath, "bus_exit clean=true", 3*time.Second)

	statusJSON, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	stopJSON, err := json.Marshal(stopResp)
	if err != nil {
		t.Fatal(err)
	}
	for name, artifact := range map[string][]byte{
		"consume A stdout": stdoutA.Bytes(),
		"consume A stderr": stderrA.Bytes(),
		"consume B stdout": stdoutB.Bytes(),
		"consume B stderr": stderrB.Bytes(),
		"status response":  statusJSON,
		"stop response":    stopJSON,
	} {
		assertRuntimeTokenDetachedClean(t, name, artifact)
	}
	assertRuntimeTokenDetachedTreeClean(t, root)

	for _, required := range []string{
		filepath.Join(workDir, bus.MetaFileName),
		filepath.Join(workDir, "bus.log"),
		filepath.Join(workDir, personal.StateFileName),
		evidencePath,
	} {
		if info, statErr := os.Stat(required); statErr != nil || !info.Mode().IsRegular() {
			t.Fatalf("expected runtime artifact %s: info=%v err=%v", required, info, statErr)
		}
	}
}

func runtimeTokenDetachedConsumeConfig(workDir, endpoint, subscribeID, token string, duration time.Duration, stdout, stderr *bytes.Buffer) consume.Config {
	return consume.Config{
		WorkDir:          workDir,
		IPCEndpoint:      endpoint,
		ClientID:         runtimeTokenDetachedClientID,
		RuntimeToken:     token,
		EventTypes:       []string{personal.EventMention},
		EventKey:         personal.EventMention,
		SubscribeID:      subscribeID,
		ReadySubscribeID: subscribeID,
		Duration:         duration,
		Format:           consume.FormatNDJSON,
		Stdout:           stdout,
		Stderr:           stderr,
	}
}

func waitRuntimeTokenDetachedFile(t *testing.T, path, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), want) {
			return string(data)
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, err := os.ReadFile(path)
	if runtimeTokenDetachedContainsCanary(string(data)) {
		t.Fatalf("runtime credential leaked into child evidence while waiting for %q", want)
	}
	t.Fatalf("evidence %s missing %q: data=%q err=%v", path, want, data, err)
	return ""
}

func waitRuntimeTokenDetachedStatus(t *testing.T, endpoint, subscribeID string, timeout time.Duration) *transport.StatusResp {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		status, err := busctl.QueryStatus(endpoint)
		if err == nil {
			if subscribeID == "" && len(status.Consumers) == 0 {
				return status
			}
			for _, consumer := range status.Consumers {
				if consumer.SubscribeID == subscribeID {
					return status
				}
			}
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("status never reached subscribe_id=%q: %v", subscribeID, lastErr)
	return nil
}

func assertRuntimeTokenDetachedTreeClean(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assertRuntimeTokenDetachedClean(t, path, data)
		return nil
	})
	if err != nil {
		t.Fatalf("scan runtime artifacts: %v", err)
	}
}

func assertRuntimeTokenDetachedClean(t *testing.T, name string, artifact []byte) {
	t.Helper()
	if runtimeTokenDetachedContainsCanary(string(artifact)) {
		t.Fatalf("runtime credential leaked into %s", name)
	}
}

func failRuntimeTokenDetachedError(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil && runtimeTokenDetachedContainsCanary(err.Error()) {
		t.Fatalf("%s failed and exposed a runtime credential", step)
	}
	t.Fatalf("%s: %v", step, err)
}

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package consume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func installRuntimeCredentialDiscover(t *testing.T, serve func(net.Conn)) {
	t.Helper()
	oldDiscover := discoverBus
	done := make(chan struct{})
	discoverBus = func(busctl.DiscoverConfig) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer close(done)
			defer server.Close()
			serve(server)
		}()
		return client, nil
	}
	t.Cleanup(func() {
		discoverBus = oldDiscover
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("fake runtime credential bus did not stop")
		}
	})
}

func TestCrossPlatformCoverageRunRuntimeTokenNegotiatesBeforeReady(t *testing.T) {
	const canary = "consume-runtime-canary"
	helloSeen := make(chan transport.Hello, 1)
	updateSeen := make(chan transport.CredentialUpdate, 1)
	installRuntimeCredentialDiscover(t, func(conn net.Conn) {
		r, w := transport.NewReader(conn), transport.NewWriter(conn)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			return
		}
		helloSeen <- hello
		_ = w.WriteJSON(transport.HelloAck{
			Type:                 transport.FrameTypeHelloAck,
			BusPID:               71,
			Capabilities:         []string{transport.CapabilityRuntimeTokenV1},
			CredentialGeneration: 3,
		})
		var update transport.CredentialUpdate
		if err := r.ReadJSON(&update); err != nil {
			return
		}
		updateSeen <- update
		_ = w.WriteJSON(transport.CredentialUpdateAck{
			Type:                 transport.FrameTypeCredentialUpdateAck,
			Accepted:             true,
			CredentialGeneration: 4,
		})
		_ = w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: "done"})
	})

	var stderr bytes.Buffer
	cfg := validRunConfig()
	cfg.RuntimeToken = "  " + canary + "  "
	cfg.Stderr = &stderr
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	hello := <-helloSeen
	if hello.CredentialMode != transport.CredentialModeRuntimeToken {
		t.Fatalf("credential mode = %q", hello.CredentialMode)
	}
	update := <-updateSeen
	if update.ExpectedGeneration != 3 || update.Token != canary {
		t.Fatal("credential update used the wrong token or generation")
	}
	if !strings.Contains(stderr.String(), "[event] ready bus_pid=71") {
		t.Fatalf("ready marker missing: %s", stderr.String())
	}
}

func TestCrossPlatformCoverageRunRuntimeTokenMissingCapabilityFailsBeforeSendingSecretOrReady(t *testing.T) {
	const canary = "unsupported-canary-secret"
	peerBytes := make(chan string, 1)
	installRuntimeCredentialDiscover(t, func(conn net.Conn) {
		r, w := transport.NewReader(conn), transport.NewWriter(conn)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			return
		}
		_ = w.WriteJSON(transport.HelloAck{Type: transport.FrameTypeHelloAck, BusPID: 72})
		raw, _ := io.ReadAll(conn)
		peerBytes <- string(raw)
	})

	var stderr bytes.Buffer
	cfg := validRunConfig()
	cfg.RuntimeToken = canary
	cfg.Stderr = &stderr
	err := Run(context.Background(), cfg)
	if !errors.Is(err, ErrRuntimeTokenUnsupported) {
		t.Fatalf("Run error = %v", err)
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatal("unsupported-bus error contained runtime token")
	}
	for _, recoveryStep := range []string{
		"dws event status --as user --format json",
		"dws event stop --as user --all --dry-run",
		"dws event stop --as user --all --yes",
	} {
		if !strings.Contains(err.Error(), recoveryStep) {
			t.Fatalf("unsupported-bus error missing recovery step %q: %v", recoveryStep, err)
		}
	}
	if strings.Contains(stderr.String(), "[event] ready") {
		t.Fatal("ready marker was written before capability negotiation succeeded")
	}
	if got := <-peerBytes; strings.Contains(got, canary) || got != "" {
		t.Fatal("client sent data after unsupported capability acknowledgement")
	}
}

func TestCrossPlatformCoverageRunRuntimeTokenRejectedDoesNotSurfacePeerText(t *testing.T) {
	const canary = "rejected-canary-secret"
	installRuntimeCredentialDiscover(t, func(conn net.Conn) {
		r, w := transport.NewReader(conn), transport.NewWriter(conn)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			return
		}
		_ = w.WriteJSON(transport.HelloAck{
			Type:         transport.FrameTypeHelloAck,
			Capabilities: []string{transport.CapabilityRuntimeTokenV1},
		})
		var update transport.CredentialUpdate
		if err := r.ReadJSON(&update); err != nil {
			return
		}
		_ = w.WriteJSON(transport.CredentialUpdateAck{
			Type:      transport.FrameTypeCredentialUpdateAck,
			Accepted:  false,
			ErrorCode: "malicious-code-" + update.Token,
			Error:     "malicious echo " + update.Token,
		})
	})

	cfg := validRunConfig()
	cfg.RuntimeToken = canary
	err := Run(context.Background(), cfg)
	if !errors.Is(err, ErrRuntimeTokenUpdate) {
		t.Fatalf("Run error = %v", err)
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatal("credential update error contained peer-provided token text")
	}
}

func TestCrossPlatformCoverageRuntimeTokenNeverAppearsInDryRun(t *testing.T) {
	const canary = "dry-run-canary-secret"
	var output bytes.Buffer
	cfg := validRunConfig()
	cfg.RuntimeToken = canary
	PrintDryRun(&output, cfg)
	if strings.Contains(output.String(), canary) || strings.Contains(output.String(), "RuntimeToken") {
		t.Fatal("dry-run output contained runtime-token data")
	}
}

func TestCrossPlatformCoverageRunWhitespaceRuntimeTokenUsesLegacyHandshake(t *testing.T) {
	helloSeen := make(chan transport.Hello, 1)
	installRuntimeCredentialDiscover(t, func(conn net.Conn) {
		r, w := transport.NewReader(conn), transport.NewWriter(conn)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			return
		}
		helloSeen <- hello
		_ = w.WriteJSON(transport.HelloAck{Type: transport.FrameTypeHelloAck})
		_ = w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: "done"})
	})
	cfg := validRunConfig()
	cfg.RuntimeToken = "   "
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if hello := <-helloSeen; hello.CredentialMode != "" {
		t.Fatalf("whitespace token enabled mode %q", hello.CredentialMode)
	}
}

func TestCrossPlatformCoverageRunRuntimeTokenRejectedByeReturnsTypedError(t *testing.T) {
	installRuntimeCredentialDiscover(t, func(conn net.Conn) {
		r, w := transport.NewReader(conn), transport.NewWriter(conn)
		var hello transport.Hello
		if err := r.ReadJSON(&hello); err != nil {
			return
		}
		_ = w.WriteJSON(transport.HelloAck{
			Type:         transport.FrameTypeHelloAck,
			BusPID:       73,
			Capabilities: []string{transport.CapabilityRuntimeTokenV1},
		})
		var update transport.CredentialUpdate
		if err := r.ReadJSON(&update); err != nil {
			return
		}
		_ = w.WriteJSON(transport.CredentialUpdateAck{
			Type:                 transport.FrameTypeCredentialUpdateAck,
			Accepted:             true,
			CredentialGeneration: 1,
		})
		_ = w.WriteJSON(transport.Bye{
			Type:   transport.FrameTypeBye,
			Reason: transport.ByeReasonRuntimeTokenRejected,
		})
	})

	var stderr bytes.Buffer
	cfg := validRunConfig()
	cfg.RuntimeToken = "runtime-rejected-canary"
	cfg.Stderr = &stderr
	err := Run(context.Background(), cfg)
	if !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(stderr.String(), "reason: bus_shutdown") {
		t.Fatalf("runtime rejection was reported as a successful exit: %s", stderr.String())
	}
}

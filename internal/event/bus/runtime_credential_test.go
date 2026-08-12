// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package bus

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

type runtimeCredentialRejectSource struct {
	broker *runtimecred.Broker
}

func (s *runtimeCredentialRejectSource) Start(ctx context.Context, _ dwsevent.EmitFn) error {
	if _, err := s.broker.Resolve(ctx); err != nil {
		return err
	}
	return runtimecred.ErrRuntimeTokenRejected
}

func runtimeCredentialDaemon(broker *runtimecred.Broker, logOutput io.Writer) *daemon {
	if logOutput == nil {
		logOutput = io.Discard
	}
	return &daemon{
		cfg:      Config{CredentialBroker: broker},
		log:      slog.New(slog.NewTextHandler(logOutput, nil)),
		hub:      NewHub(2),
		started:  time.Now(),
		idleStop: make(chan struct{}),
	}
}

func runRuntimeCredentialConnection(t *testing.T, d *daemon) (net.Conn, *transport.Writer, *transport.Reader, <-chan struct{}) {
	t.Helper()
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		d.handleConnection(context.Background(), server)
		close(done)
	}()
	return client, transport.NewWriter(client), transport.NewReader(client), done
}

func TestCrossPlatformCoverageDaemonRuntimeCredentialHandshakeBeforeRegister(t *testing.T) {
	broker := runtimecred.New(runtimecred.Config{RequireSeed: true, RequireActivation: true})
	d := runtimeCredentialDaemon(broker, nil)
	client, w, r, done := runRuntimeCredentialConnection(t, d)
	defer client.Close()

	if err := w.WriteJSON(transport.Hello{
		Type:           transport.FrameTypeHello,
		ConsumerPID:    42,
		CredentialMode: transport.CredentialModeRuntimeToken,
	}); err != nil {
		t.Fatal(err)
	}
	var helloAck transport.HelloAck
	if err := r.ReadJSON(&helloAck); err != nil {
		t.Fatal(err)
	}
	if !hasTransportCapability(helloAck.Capabilities, transport.CapabilityRuntimeTokenV1) || helloAck.CredentialGeneration != 0 {
		t.Fatalf("hello ack = %#v", helloAck)
	}
	if d.hub.Len() != 0 {
		t.Fatalf("consumer registered before credential update: %d", d.hub.Len())
	}

	const canary = "ipc-canary-runtime-token"
	if err := w.WriteJSON(transport.CredentialUpdate{
		Type:               transport.FrameTypeCredentialUpdate,
		ExpectedGeneration: helloAck.CredentialGeneration,
		Token:              canary,
	}); err != nil {
		t.Fatal(err)
	}
	var updateAck transport.CredentialUpdateAck
	if err := r.ReadJSON(&updateAck); err != nil {
		t.Fatal(err)
	}
	if !updateAck.Accepted || updateAck.CredentialGeneration != 1 {
		t.Fatalf("credential ack = %#v", updateAck)
	}
	deadline := time.Now().Add(time.Second)
	for d.hub.Len() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if d.hub.Len() != 1 {
		t.Fatal("consumer was not registered after credential ack")
	}
	if token, err := broker.Resolve(context.Background()); err != nil || token != canary {
		t.Fatalf("broker did not resolve installed runtime token: %v", err)
	}

	if err := w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: "done"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("connection did not close")
	}
}

func TestCrossPlatformCoverageDaemonCompatibleBusRotatesRuntimeCredentialAcrossConnections(t *testing.T) {
	broker := runtimecred.New(runtimecred.Config{RequireSeed: true, RequireActivation: true})
	d := runtimeCredentialDaemon(broker, nil)

	handshake := func(token string, wantHelloGeneration, wantAckGeneration uint64) {
		t.Helper()
		client, w, r, done := runRuntimeCredentialConnection(t, d)
		if err := w.WriteJSON(transport.Hello{
			Type:           transport.FrameTypeHello,
			CredentialMode: transport.CredentialModeRuntimeToken,
		}); err != nil {
			t.Fatal(err)
		}
		var helloAck transport.HelloAck
		if err := r.ReadJSON(&helloAck); err != nil {
			t.Fatal(err)
		}
		if helloAck.CredentialGeneration != wantHelloGeneration {
			t.Fatalf("hello generation = %d, want %d", helloAck.CredentialGeneration, wantHelloGeneration)
		}
		if err := w.WriteJSON(transport.CredentialUpdate{
			Type:               transport.FrameTypeCredentialUpdate,
			ExpectedGeneration: helloAck.CredentialGeneration,
			Token:              token,
		}); err != nil {
			t.Fatal(err)
		}
		var updateAck transport.CredentialUpdateAck
		if err := r.ReadJSON(&updateAck); err != nil {
			t.Fatal(err)
		}
		if !updateAck.Accepted || updateAck.CredentialGeneration != wantAckGeneration {
			t.Fatalf("credential ack accepted=%v generation=%d, want true/%d", updateAck.Accepted, updateAck.CredentialGeneration, wantAckGeneration)
		}
		if err := w.WriteJSON(transport.Bye{Type: transport.FrameTypeBye, Reason: "done"}); err != nil {
			t.Fatal(err)
		}
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("connection did not close")
		}
	}

	handshake("runtime-token-a", 0, 1)
	if token, err := broker.Resolve(context.Background()); err != nil || token != "runtime-token-a" {
		t.Fatalf("broker did not retain first token: %v", err)
	}
	handshake("runtime-token-b", 1, 2)
	if token, err := broker.Resolve(context.Background()); err != nil || token != "runtime-token-b" {
		t.Fatalf("broker did not rotate to second token: %v", err)
	}
}

func TestCrossPlatformCoverageDaemonRuntimeCredentialMissingCapabilityDoesNotRegister(t *testing.T) {
	d := runtimeCredentialDaemon(nil, nil)
	client, w, r, done := runRuntimeCredentialConnection(t, d)
	defer client.Close()
	if err := w.WriteJSON(transport.Hello{
		Type:           transport.FrameTypeHello,
		CredentialMode: transport.CredentialModeRuntimeToken,
	}); err != nil {
		t.Fatal(err)
	}
	var ack transport.HelloAck
	if err := r.ReadJSON(&ack); err != nil {
		t.Fatal(err)
	}
	if hasTransportCapability(ack.Capabilities, transport.CapabilityRuntimeTokenV1) {
		t.Fatalf("unsupported daemon advertised capability: %#v", ack)
	}
	if d.hub.Len() != 0 {
		t.Fatalf("unsupported daemon registered consumer: %d", d.hub.Len())
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("unsupported connection did not close")
	}
}

func TestCrossPlatformCoverageDaemonTerminalStateRejectsLateLegacyConsumer(t *testing.T) {
	d := runtimeCredentialDaemon(nil, nil)
	d.setTerminalReason(transport.ByeReasonRuntimeTokenRejected)
	client, w, r, done := runRuntimeCredentialConnection(t, d)
	defer client.Close()

	if err := w.WriteJSON(transport.Hello{Type: transport.FrameTypeHello, ConsumerPID: 7}); err != nil {
		t.Fatal(err)
	}
	var bye transport.Bye
	if err := r.ReadJSON(&bye); err != nil {
		t.Fatal(err)
	}
	if bye.Type != transport.FrameTypeBye || bye.Reason != transport.ByeReasonRuntimeTokenRejected {
		t.Fatalf("late consumer frame = %#v", bye)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("late consumer connection did not close")
	}
	if d.hub.Len() != 0 {
		t.Fatalf("late terminal consumer remained registered: %d", d.hub.Len())
	}
}

func TestCrossPlatformCoverageDaemonRuntimeCredentialConflictDoesNotLeakOrRegister(t *testing.T) {
	broker := runtimecred.New(runtimecred.Config{})
	if _, err := broker.Update(0, "installed-secret"); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	d := runtimeCredentialDaemon(broker, &logs)
	client, w, r, done := runRuntimeCredentialConnection(t, d)
	defer client.Close()
	if err := w.WriteJSON(transport.Hello{
		Type:           transport.FrameTypeHello,
		CredentialMode: transport.CredentialModeRuntimeToken,
	}); err != nil {
		t.Fatal(err)
	}
	var helloAck transport.HelloAck
	if err := r.ReadJSON(&helloAck); err != nil {
		t.Fatal(err)
	}
	const canary = "rejected-canary-secret"
	if err := w.WriteJSON(transport.CredentialUpdate{
		Type:               transport.FrameTypeCredentialUpdate,
		ExpectedGeneration: 0,
		Token:              canary,
	}); err != nil {
		t.Fatal(err)
	}
	var updateAck transport.CredentialUpdateAck
	if err := r.ReadJSON(&updateAck); err != nil {
		t.Fatal(err)
	}
	if updateAck.Accepted || updateAck.ErrorCode != transport.CredentialErrorGenerationConflict || updateAck.CredentialGeneration != 1 {
		t.Fatalf("conflict ack = %#v", updateAck)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rejected connection did not close")
	}
	if strings.Contains(updateAck.Error, canary) || strings.Contains(logs.String(), canary) {
		t.Fatal("credential appeared in acknowledgement or logs")
	}
	if d.hub.Len() != 0 {
		t.Fatalf("rejected consumer registered: %d", d.hub.Len())
	}
}

func TestCrossPlatformCoverageDaemonRuntimeCredentialInvalidFilterRejectedBeforeUpdate(t *testing.T) {
	broker := runtimecred.New(runtimecred.Config{})
	d := runtimeCredentialDaemon(broker, nil)
	client, w, r, done := runRuntimeCredentialConnection(t, d)
	defer client.Close()
	if err := w.WriteJSON(transport.Hello{
		Type:           transport.FrameTypeHello,
		CredentialMode: transport.CredentialModeRuntimeToken,
		Filter:         "[",
	}); err != nil {
		t.Fatal(err)
	}
	var helloAck transport.HelloAck
	if err := r.ReadJSON(&helloAck); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteJSON(transport.CredentialUpdate{
		Type:               transport.FrameTypeCredentialUpdate,
		ExpectedGeneration: helloAck.CredentialGeneration,
		Token:              "filter-canary-secret",
	}); err != nil {
		t.Fatal(err)
	}
	var updateAck transport.CredentialUpdateAck
	if err := r.ReadJSON(&updateAck); err != nil {
		t.Fatal(err)
	}
	if updateAck.Accepted || updateAck.ErrorCode != transport.CredentialErrorRegistration {
		t.Fatalf("invalid filter ack = %#v", updateAck)
	}
	if broker.Generation() != 0 || d.hub.Len() != 0 {
		t.Fatalf("invalid filter mutated state: generation=%d consumers=%d", broker.Generation(), d.hub.Len())
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("invalid filter connection did not close")
	}
}

func TestCrossPlatformCoverageRuntimeCredentialShutdownReason(t *testing.T) {
	if got := sourceShutdownReason(runtimecred.ErrRuntimeTokenRejected); got != transport.ByeReasonRuntimeTokenRejected {
		t.Fatalf("runtime source shutdown reason = %q", got)
	}
	if got := sourceShutdownReason(errors.New("local source failed")); got != "shutdown" {
		t.Fatalf("local source shutdown reason = %q", got)
	}
	if got := normalizedShutdownReason(transport.ByeReasonRuntimeTokenRejected); got != transport.ByeReasonRuntimeTokenRejected {
		t.Fatalf("normalized runtime shutdown reason = %q", got)
	}
	for _, reasons := range [][]string{nil, {"peer-controlled"}} {
		if got := normalizedShutdownReason(reasons...); got != "shutdown" {
			t.Fatalf("normalized untrusted shutdown reason = %q", got)
		}
	}
}

func TestCrossPlatformCoverageDaemonRuntimeCredentialSourceRejectionBroadcastsTypedBye(t *testing.T) {
	skipOnWindows(t, "uses Unix socket dial")
	workDir := shortTempDir(t)
	sockPath := filepath.Join(workDir, "bus.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker := runtimecred.New(runtimecred.Config{RequireSeed: true, RequireActivation: true})
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(ctx, Config{
			WorkDir:          workDir,
			IPCEndpoint:      sockPath,
			ClientID:         "runtime-client",
			Edition:          "open",
			SourceKind:       dwsevent.SourceKindPersonalStream,
			IdentityHash:     "0123456789abcdef",
			SourceID:         "open",
			Source:           &runtimeCredentialRejectSource{broker: broker},
			CredentialBroker: broker,
		})
	}()
	waitForFile(t, sockPath, 2*time.Second)

	conn, err := transport.Dial(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	r, w := transport.NewReader(conn), transport.NewWriter(conn)
	if err := w.WriteJSON(transport.Hello{
		Type:           transport.FrameTypeHello,
		CredentialMode: transport.CredentialModeRuntimeToken,
	}); err != nil {
		t.Fatal(err)
	}
	var helloAck transport.HelloAck
	if err := r.ReadJSON(&helloAck); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteJSON(transport.CredentialUpdate{
		Type:               transport.FrameTypeCredentialUpdate,
		ExpectedGeneration: helloAck.CredentialGeneration,
		Token:              "runtime-rejected-canary",
	}); err != nil {
		t.Fatal(err)
	}
	var updateAck transport.CredentialUpdateAck
	if err := r.ReadJSON(&updateAck); err != nil || !updateAck.Accepted {
		t.Fatalf("credential update ack = %#v, %v", updateAck, err)
	}

	var bye transport.Bye
	if err := r.ReadJSON(&bye); err != nil {
		t.Fatalf("read typed shutdown: %v", err)
	}
	if bye.Type != transport.FrameTypeBye || bye.Reason != transport.ByeReasonRuntimeTokenRejected {
		t.Fatalf("shutdown frame = %#v", bye)
	}
	_ = conn.Close()
	select {
	case err := <-runDone:
		if !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runtime credential bus did not stop")
	}
}

func hasTransportCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

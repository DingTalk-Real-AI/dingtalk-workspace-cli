package consume

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func eventCoreCredentialReader(t *testing.T, frame any) *transport.Reader {
	t.Helper()
	var buffer bytes.Buffer
	if err := transport.NewWriter(&buffer).WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	return transport.NewReader(&buffer)
}

func TestCrossPlatformCoverageEventCoreRuntimeNegotiationEdges(t *testing.T) {
	capable := transport.HelloAck{Capabilities: []string{transport.CapabilityRuntimeTokenV1}}
	if err := negotiateRuntimeToken(
		transport.NewWriter(io.Discard),
		transport.NewReader(strings.NewReader("")),
		transport.HelloAck{TerminalReason: transport.ByeReasonRuntimeTokenRejected},
		"token",
	); !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) {
		t.Fatalf("terminal hello error = %v", err)
	}

	wantWriteErr := errors.New("write failed")
	if err := negotiateRuntimeToken(
		transport.NewWriter(errorWriter{err: wantWriteErr}),
		transport.NewReader(strings.NewReader("")),
		capable,
		"token",
	); !errors.Is(err, wantWriteErr) {
		t.Fatalf("credential write error = %v", err)
	}

	if err := negotiateRuntimeToken(
		transport.NewWriter(io.Discard),
		transport.NewReader(strings.NewReader("")),
		capable,
		"token",
	); !errors.Is(err, io.EOF) {
		t.Fatalf("credential ack read error = %v", err)
	}

	if err := negotiateRuntimeToken(
		transport.NewWriter(io.Discard),
		eventCoreCredentialReader(t, transport.Heartbeat{Type: transport.FrameTypeHeartbeat}),
		capable,
		"token",
	); err == nil || !strings.Contains(err.Error(), "unexpected runtime credential response") {
		t.Fatalf("unexpected credential frame error = %v", err)
	}

	if err := negotiateRuntimeToken(
		transport.NewWriter(io.Discard),
		eventCoreCredentialReader(t, transport.CredentialUpdateAck{
			Type:      transport.FrameTypeCredentialUpdateAck,
			Accepted:  false,
			ErrorCode: transport.CredentialErrorRuntimeRejected,
		}),
		capable,
		"token",
	); !errors.Is(err, runtimecred.ErrRuntimeTokenRejected) {
		t.Fatalf("runtime rejected ack error = %v", err)
	}

	for _, code := range []string{
		transport.CredentialErrorGenerationConflict,
		transport.CredentialErrorInvalid,
		transport.CredentialErrorRegistration,
		transport.CredentialErrorRuntimeRejected,
		transport.CredentialErrorInternal,
	} {
		if got := safeCredentialErrorCode(code); got != code {
			t.Fatalf("safeCredentialErrorCode(%q) = %q", code, got)
		}
	}
	if got := safeCredentialErrorCode("peer-controlled"); got != transport.CredentialErrorInternal {
		t.Fatalf("unknown credential error code = %q", got)
	}
}

func TestCrossPlatformCoverageEventCoreRunManyHandshakeFailure(t *testing.T) {
	bus := newManyFakeBus(901, nil)
	installManyDiscover(t, bus)
	cfg := manyTestConfig(io.Discard, io.Discard)
	cfg.RuntimeToken = "runtime-token"
	err := RunMany(context.Background(), cfg, manyTestSpecs())
	if !errors.Is(err, ErrRuntimeTokenUnsupported) || !strings.Contains(err.Error(), "runtime credential handshake") {
		t.Fatalf("RunMany handshake error = %v", err)
	}
}

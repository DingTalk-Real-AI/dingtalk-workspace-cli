package transport

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestHTTPStatusErrorIncludesCallMetadata(t *testing.T) {
	err := httpStatusError("tools/call", "https://mcp.dingtalk.com/server", http.StatusTooManyRequests, "")

	var callErr *CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("expected CallError in chain, got %T", err)
	}
	if callErr.Stage != CallStageHTTP {
		t.Fatalf("Stage = %q, want %q", callErr.Stage, CallStageHTTP)
	}
	if callErr.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("HTTPStatus = %d, want %d", callErr.HTTPStatus, http.StatusTooManyRequests)
	}

	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected structured errors.Error, got %T", err)
	}
	if typed.Reason != "http_429" {
		t.Fatalf("Reason = %q, want http_429", typed.Reason)
	}
}

func TestJSONRPCEnvelopeErrorIncludesCallMetadata(t *testing.T) {
	err := jsonrpcEnvelopeError("tools/call", &RPCError{Code: -32602, Message: "invalid params"}, "")

	var callErr *CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("expected CallError in chain, got %T", err)
	}
	if callErr.Stage != CallStageJSONRPC {
		t.Fatalf("Stage = %q, want %q", callErr.Stage, CallStageJSONRPC)
	}
	if callErr.RPCCode != -32602 {
		t.Fatalf("RPCCode = %d, want -32602", callErr.RPCCode)
	}
}

func TestDoWithRetryRequestFailureIncludesCallMetadata(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})})
	client.MaxRetries = 0

	_, err := client.doWithRetry(context.Background(), "https://mcp.dingtalk.com/server", []byte(`{}`))
	if err == nil {
		t.Fatal("doWithRetry() error = nil, want request failure")
	}

	var callErr *CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("expected CallError in chain, got %T", err)
	}
	if callErr.Stage != CallStageRequest {
		t.Fatalf("Stage = %q, want %q", callErr.Stage, CallStageRequest)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

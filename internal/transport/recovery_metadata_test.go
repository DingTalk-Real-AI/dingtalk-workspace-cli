package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestHTTPStatusErrorIncludesCallMetadata(t *testing.T) {
	err := httpStatusError("tools/call", "https://mcp.dingtalk.com/server", http.StatusTooManyRequests, "", "")

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
	err := jsonrpcEnvelopeError("tools/call", &RPCError{Code: -32602, Message: "invalid params"}, "", "")

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

	_, err := client.doWithRetry(context.Background(), "https://mcp.dingtalk.com/server", []byte(`{}`), "tools/call")
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

func TestCallToolRequestFailureUsesAPIClassification(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})})
	client.MaxRetries = 0

	_, err := client.CallTool(context.Background(), "https://mcp.dingtalk.com/server", "search_messages", map[string]any{"keyword": "marker"})
	if err == nil {
		t.Fatal("CallTool() error = nil, want request failure")
	}

	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryAPI {
		t.Fatalf("Category = %q, want %q", typed.Category, apperrors.CategoryAPI)
	}
	if typed.Operation != "tools/call" {
		t.Fatalf("Operation = %q, want tools/call", typed.Operation)
	}
	if typed.Reason != "connection_refused" {
		t.Fatalf("Reason = %q, want connection_refused", typed.Reason)
	}
	actions := strings.Join(typed.Actions, "\n")
	if strings.Contains(actions, "internal/syncdata") || strings.Contains(actions, "sync-oss") {
		t.Fatalf("runtime network failure contains discovery-only actions: %q", actions)
	}
}

func TestCallToolRetryCancellationUsesAPIClassification(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil
	})})
	client.MaxRetries = 1
	client.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	_, err := client.CallTool(context.Background(), "https://mcp.dingtalk.com/server", "search_messages", nil)
	if err == nil {
		t.Fatal("CallTool() error = nil, want retry cancellation")
	}

	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("CallTool() error = %T, want *errors.Error", err)
	}
	if typed.Category != apperrors.CategoryAPI {
		t.Fatalf("Category = %q, want %q", typed.Category, apperrors.CategoryAPI)
	}
	if typed.Operation != "tools/call" {
		t.Fatalf("Operation = %q, want tools/call", typed.Operation)
	}
	if typed.Reason != "request_cancelled" {
		t.Fatalf("Reason = %q, want request_cancelled", typed.Reason)
	}
	actions := strings.Join(typed.Actions, "\n")
	if strings.Contains(actions, "internal/syncdata") || strings.Contains(actions, "sync-oss") {
		t.Fatalf("runtime retry cancellation contains discovery-only actions: %q", actions)
	}
}

func TestDiscoveryRequestFailuresKeepDiscoveryClassification(t *testing.T) {
	tests := []struct {
		name       string
		failure    error
		operation  string
		wantReason string
		call       func(*Client) error
	}{
		{
			name:       "initialize connection refused",
			failure:    errors.New("dial tcp: connection refused"),
			operation:  "initialize",
			wantReason: "connection_refused",
			call: func(client *Client) error {
				_, err := client.Initialize(context.Background(), "https://mcp.dingtalk.com/server")
				return err
			},
		},
		{
			name:       "tools list dns failure",
			failure:    errors.New("dial tcp: lookup mcp.dingtalk.com: no such host"),
			operation:  "tools/list",
			wantReason: "dns_resolution_failed",
			call: func(client *Client) error {
				_, err := client.ListTools(context.Background(), "https://mcp.dingtalk.com/server")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, tt.failure
			})})
			client.MaxRetries = 0

			err := tt.call(client)
			if err == nil {
				t.Fatal("request error = nil, want failure")
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) {
				t.Fatalf("request error = %T, want *errors.Error", err)
			}
			if typed.Category != apperrors.CategoryDiscovery {
				t.Fatalf("Category = %q, want %q", typed.Category, apperrors.CategoryDiscovery)
			}
			if typed.Operation != tt.operation {
				t.Fatalf("Operation = %q, want %q", typed.Operation, tt.operation)
			}
			if typed.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", typed.Reason, tt.wantReason)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

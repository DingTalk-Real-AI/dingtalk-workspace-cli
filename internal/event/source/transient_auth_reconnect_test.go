// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

func transientRefreshError() error {
	return &authpkg.HTTPStatusError{StatusCode: http.StatusServiceUnavailable}
}

func terminalRefreshError() error {
	return &authpkg.HTTPStatusError{StatusCode: http.StatusUnauthorized}
}

func TestCrossPlatformCoveragePersonalRetryLogErrorReportsOnlySafeTransientStatus(t *testing.T) {
	cause := errors.New("personal source: ticket HTTP 401 secret detail")
	_, err := refreshRejectedSourceToken(context.Background(), func(context.Context, string) (string, error) {
		return "", transientRefreshError()
	}, "rejected", "personal source", cause)
	if got, want := personalRetryLogError(retryPersonal(err)), "personal source: token refresh HTTP 503"; got != want {
		t.Fatalf("personalRetryLogError() = %q, want %q", got, want)
	}
}

func TestCrossPlatformCoveragePersonalSourceRetriesTransientTokenResolutionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	src, err := NewPersonal(PersonalConfig{
		AccessTokenProvider: func(context.Context) (string, error) {
			if calls.Add(1) == 2 {
				cancel()
			}
			return "", transientRefreshError()
		},
		ClientID:     "client",
		SourceID:     "open",
		TicketURL:    "https://ticket.invalid",
		ReconnectMin: time.Millisecond,
		ReconnectMax: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(ctx, func(*dwsevent.RawEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context canceled after retry", err)
	}
	if calls.Load() != 2 || src.State().ReconnectCount != 1 {
		t.Fatalf("provider calls=%d reconnects=%d, want 2 calls and 1 reconnect", calls.Load(), src.State().ReconnectCount)
	}
}

func TestCrossPlatformCoveragePersonalSourceDoesNotRetryTerminalTokenResolutionFailure(t *testing.T) {
	var calls atomic.Int32
	src, err := NewPersonal(PersonalConfig{
		AccessTokenProvider: func(context.Context) (string, error) {
			calls.Add(1)
			return "", terminalRefreshError()
		},
		ClientID:  "client",
		SourceID:  "open",
		TicketURL: "https://ticket.invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(context.Background(), func(*dwsevent.RawEvent) {})
	if authpkg.ClassifyRefreshFailure(err) != authpkg.RefreshFailureTerminal {
		t.Fatalf("Start() error = %v, want terminal refresh failure", err)
	}
	if calls.Load() != 1 || src.State().ReconnectCount != 0 {
		t.Fatalf("provider calls=%d reconnects=%d, want 1 call and no reconnect", calls.Load(), src.State().ReconnectCount)
	}
}

func TestCrossPlatformCoveragePersonalSourceRetriesTransientRejectedTokenRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var ticketCalls atomic.Int32
	var refreshCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ticketCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	src, err := NewPersonal(PersonalConfig{
		AccessTokenProvider: func(context.Context) (string, error) { return "old-token", nil },
		ForceRefreshToken: func(_ context.Context, rejected string) (string, error) {
			if rejected != "old-token" {
				t.Fatalf("rejected token = %q, want old-token", rejected)
			}
			if refreshCalls.Add(1) == 2 {
				cancel()
			}
			return "", transientRefreshError()
		},
		ClientID:     "client",
		SourceID:     "open",
		TicketURL:    srv.URL,
		HTTPClient:   srv.Client(),
		ReconnectMin: time.Millisecond,
		ReconnectMax: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(ctx, func(*dwsevent.RawEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context canceled after retry", err)
	}
	if ticketCalls.Load() != 2 || refreshCalls.Load() != 2 || src.State().ReconnectCount != 1 {
		t.Fatalf("ticket calls=%d refresh calls=%d reconnects=%d", ticketCalls.Load(), refreshCalls.Load(), src.State().ReconnectCount)
	}
}

func TestCrossPlatformCoveragePersonalRetryLogErrorFallsBackToNetworkMessage(t *testing.T) {
	err := retryPersonal(fmt.Errorf("personal source: resolve access token: %w", errors.New("dial tcp: lookup oauth.invalid")))
	if got, want := personalRetryLogError(err), "personal source: token refresh: temporary network error"; got != want {
		t.Fatalf("personalRetryLogError() = %q, want %q", got, want)
	}
}

func TestCrossPlatformCoveragePortalStageErrorNilAndUnwrap(t *testing.T) {
	var nilErr *portalStageError
	if got, want := nilErr.Error(), "source: portal stream failed"; got != want {
		t.Fatalf("nil stage error = %q, want %q", got, want)
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("nil stage error should unwrap to nil")
	}
	cause := errors.New("cause")
	stageErr := &portalStageError{stage: "stream_read", retryable: true, cause: cause}
	if !errors.Is(stageErr, cause) {
		t.Fatalf("stage error should unwrap to cause: %v", stageErr)
	}
}

func TestCrossPlatformCoveragePortalSourceRetriesTransientTokenResolutionFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	src, err := New(Config{PortalTicket: &PortalTicketConfig{
		TicketURL: "https://ticket.invalid",
		AccessTokenProvider: func(context.Context) (string, error) {
			if calls.Add(1) == 2 {
				cancel()
			}
			return "", transientRefreshError()
		},
		SourceID: "open",
		// Min above max exercises the reconnect clamp.
		ReconnectMin: 2 * time.Millisecond,
		ReconnectMax: time.Millisecond,
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(ctx, func(*dwsevent.RawEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context canceled after retry", err)
	}
	if calls.Load() != 2 || src.State().ReconnectCount != 1 {
		t.Fatalf("provider calls=%d reconnects=%d, want 2 calls and 1 reconnect", calls.Load(), src.State().ReconnectCount)
	}
}

func TestCrossPlatformCoveragePortalSourceResetsBackoffAfterAckedAttempt(t *testing.T) {
	var ticketCalls atomic.Int32
	upgrader := websocket.Upgrader{}
	var wsURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/ticket", func(w http.ResponseWriter, _ *http.Request) {
		if ticketCalls.Add(1) > 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"endpoint":`+strconvQuote(wsURL)+`,"ticket":"t"}`)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		df := payload.DataFrame{Type: "event", Headers: payload.DataFrameHeader{payload.DataFrameHeaderKMessageId: "m"}, Data: `{}`}
		_ = conn.WriteJSON(df)
		// Wait for the ACK, then close so the read fails retryably with an
		// acked attempt behind it, which resets the reconnect backoff.
		_, _, _ = conn.ReadMessage()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	src, err := New(Config{PortalTicket: &PortalTicketConfig{
		TicketURL:    srv.URL + "/ticket",
		AccessToken:  "t",
		SourceID:     "open",
		HTTPClient:   srv.Client(),
		ReconnectMin: time.Millisecond,
		ReconnectMax: time.Millisecond,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var events atomic.Int32
	err = src.Start(context.Background(), func(*dwsevent.RawEvent) { events.Add(1) })
	if err == nil || !strings.Contains(err.Error(), "portal ticket HTTP 400") {
		t.Fatalf("Start() error = %v, want fatal ticket HTTP 400 after reconnect", err)
	}
	if events.Load() != 1 || ticketCalls.Load() != 2 || src.State().ReconnectCount != 1 {
		t.Fatalf("events=%d ticket calls=%d reconnects=%d, want 1/2/1", events.Load(), ticketCalls.Load(), src.State().ReconnectCount)
	}
}

func TestCrossPlatformCoveragePortalSourceDoesNotRetryTerminalTokenResolutionFailure(t *testing.T) {
	var calls atomic.Int32
	src, err := New(Config{PortalTicket: &PortalTicketConfig{
		TicketURL: "https://ticket.invalid",
		AccessTokenProvider: func(context.Context) (string, error) {
			calls.Add(1)
			return "", terminalRefreshError()
		},
		SourceID: "open",
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(context.Background(), func(*dwsevent.RawEvent) {})
	if authpkg.ClassifyRefreshFailure(err) != authpkg.RefreshFailureTerminal {
		t.Fatalf("Start() error = %v, want terminal refresh failure", err)
	}
	if calls.Load() != 1 || src.State().ReconnectCount != 0 {
		t.Fatalf("provider calls=%d reconnects=%d, want 1 call and no reconnect", calls.Load(), src.State().ReconnectCount)
	}
}

func TestCrossPlatformCoveragePortalSourceRetriesTransientRejectedTokenRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var ticketCalls atomic.Int32
	var refreshCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ticketCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	src, err := New(Config{PortalTicket: &PortalTicketConfig{
		TicketURL:           srv.URL,
		AccessTokenProvider: func(context.Context) (string, error) { return "old-token", nil },
		ForceRefreshToken: func(_ context.Context, rejected string) (string, error) {
			if rejected != "old-token" {
				t.Fatalf("rejected token = %q, want old-token", rejected)
			}
			if refreshCalls.Add(1) == 2 {
				cancel()
			}
			return "", transientRefreshError()
		},
		SourceID:     "open",
		HTTPClient:   srv.Client(),
		ReconnectMin: time.Millisecond,
		ReconnectMax: time.Millisecond,
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = src.Start(ctx, func(*dwsevent.RawEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context canceled after retry", err)
	}
	if ticketCalls.Load() != 2 || refreshCalls.Load() != 2 || src.State().ReconnectCount != 1 {
		t.Fatalf("ticket calls=%d refresh calls=%d reconnects=%d", ticketCalls.Load(), refreshCalls.Load(), src.State().ReconnectCount)
	}
}

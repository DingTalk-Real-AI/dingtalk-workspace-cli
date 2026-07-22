package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

// TestCrossPlatformCoveragePortalStart401RefreshRetryEndToEnd drives the full production chain
// DingtalkSource.Start → startPortalTicket → requestPortalTicket: the first
// ticket request is rejected with 401, ForceRefreshToken rotates the token,
// the in-chain retry succeeds with the fresh token and a WebSocket event is
// delivered to emit.
func TestCrossPlatformCoveragePortalStart401RefreshRetryEndToEnd(t *testing.T) {
	var ticketCalls, refreshCalls atomic.Int64
	var rejectedSeen atomic.Value

	upgrader := websocket.Upgrader{}
	var wsURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/ticket", func(w http.ResponseWriter, r *http.Request) {
		ticketCalls.Add(1)
		switch r.Header.Get("x-user-access-token") {
		case "fresh-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  map[string]string{"endpoint": wsURL, "ticket": "ticket-1"},
			})
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "token expired")
		}
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		df := payload.DataFrame{Type: "event", Headers: payload.DataFrameHeader{payload.DataFrameHeaderKMessageId: "msg-1"}, Data: `{}`}
		_ = conn.WriteJSON(df)
		_, _, _ = conn.ReadMessage()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	s, err := New(Config{PortalTicket: &PortalTicketConfig{
		TicketURL: srv.URL + "/ticket",
		AccessTokenProvider: func(context.Context) (string, error) {
			return "stale-token", nil
		},
		ForceRefreshToken: func(_ context.Context, rejectedToken string) (string, error) {
			refreshCalls.Add(1)
			rejectedSeen.Store(rejectedToken)
			return "fresh-token", nil
		},
		SourceID:   "source",
		HTTPClient: srv.Client(),
	}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	emitted := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx, func(*dwsevent.RawEvent) { emitted <- struct{}{} }) }()
	select {
	case <-emitted:
	case <-time.After(2 * time.Second):
		t.Fatal("portal event timeout after 401 refresh retry")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("portal stop = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("portal stop timeout")
	}

	if got := ticketCalls.Load(); got != 2 {
		t.Fatalf("ticket calls = %d, want 2", got)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got, _ := rejectedSeen.Load().(string); got != "stale-token" {
		t.Fatalf("rejected token = %q, want %q", got, "stale-token")
	}
}

// TestCrossPlatformCoverageRequestPortalTicketRetryUsesRotatedTokenDirectly asserts the in-chain
// retry sends the token returned by ForceRefreshToken instead of re-invoking
// the provider (which could still serve the stale token).
func TestCrossPlatformCoverageRequestPortalTicketRetryUsesRotatedTokenDirectly(t *testing.T) {
	providerCalls := 0
	var attemptTokens []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		token := req.Header.Get("x-user-access-token")
		attemptTokens = append(attemptTokens, token)
		if token != "rotated" {
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"endpoint":"wss://x","ticket":"t"}`)), Header: make(http.Header)}, nil
	})}
	ticket, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL: "https://x",
		AccessTokenProvider: func(context.Context) (string, error) {
			providerCalls++
			return "stale", nil
		},
		ForceRefreshToken: func(_ context.Context, rejectedToken string) (string, error) {
			if rejectedToken != "stale" {
				t.Fatalf("rejected token = %q, want %q", rejectedToken, "stale")
			}
			return "rotated", nil
		},
		SourceID:   "s",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("requestPortalTicket = %v", err)
	}
	if ticket.Endpoint != "wss://x" || ticket.Ticket != "t" {
		t.Fatalf("ticket = %#v", ticket)
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls)
	}
	if len(attemptTokens) != 2 || attemptTokens[0] != "stale" || attemptTokens[1] != "rotated" {
		t.Fatalf("attempt tokens = %v", attemptTokens)
	}
}

// TestCrossPlatformCoverageRequestPortalTicketRefreshFailureKeepsBothErrors asserts a failing
// refresh neither retries nor drops the refresh error or the original 401.
func TestCrossPlatformCoverageRequestPortalTicketRefreshFailureKeepsBothErrors(t *testing.T) {
	refreshErr := errors.New("refresh_token exchange failed")
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
	})}
	_, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL:   "https://x",
		AccessToken: "stale",
		ForceRefreshToken: func(context.Context, string) (string, error) {
			return "", refreshErr
		},
		SourceID:   "s",
		HTTPClient: client,
	})
	if !errors.Is(err, refreshErr) {
		t.Fatalf("error should wrap refresh error, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("error should keep original 401, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry after failed refresh)", attempts)
	}
}

// TestCrossPlatformCoverageRequestPortalTicketWithoutRefreshCallback401StaysFatal covers backward
// compatibility: nil ForceRefreshToken keeps the single-attempt fatal 401.
func TestCrossPlatformCoverageRequestPortalTicketWithoutRefreshCallback401StaysFatal(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
	})}
	_, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL: "https://x", AccessToken: "stale", SourceID: "s", HTTPClient: client,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("fatal 401 expected, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// TestCrossPlatformCoverageRequestPortalTicketSecond401IsFatal guards against refresh loops: the
// controlled retry happens exactly once even if the rotated token is also
// rejected.
func TestCrossPlatformCoverageRequestPortalTicketSecond401IsFatal(t *testing.T) {
	attempts := 0
	refreshCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
	})}
	_, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL:   "https://x",
		AccessToken: "stale",
		ForceRefreshToken: func(context.Context, string) (string, error) {
			refreshCalls++
			return "rotated-but-still-rejected", nil
		},
		SourceID:   "s",
		HTTPClient: client,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("fatal 401 expected after single retry, got %v", err)
	}
	if attempts != 2 || refreshCalls != 1 {
		t.Fatalf("attempts = %d refreshCalls = %d, want 2/1", attempts, refreshCalls)
	}
}

// TestCrossPlatformCoverageRequestPortalTicketRefreshEmptyTokenIsFatal asserts an empty rotated
// token is rejected instead of being sent to the server.
func TestCrossPlatformCoverageRequestPortalTicketRefreshEmptyTokenIsFatal(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
	})}
	_, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL:   "https://x",
		AccessToken: "stale",
		ForceRefreshToken: func(context.Context, string) (string, error) {
			return "  ", nil
		},
		SourceID:   "s",
		HTTPClient: client,
	})
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("empty rotated token error expected, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// TestCrossPlatformCoveragePersonalFetchTicket401RefreshRetry mirrors the portal behavior for the
// personal stream ticket path.
func TestCrossPlatformCoveragePersonalFetchTicket401RefreshRetry(t *testing.T) {
	var attemptTokens []string
	src, err := NewPersonal(PersonalConfig{
		AccessTokenProvider: func(context.Context) (string, error) { return "stale", nil },
		ForceRefreshToken: func(_ context.Context, rejectedToken string) (string, error) {
			if rejectedToken != "stale" {
				t.Fatalf("rejected token = %q, want %q", rejectedToken, "stale")
			}
			return "rotated", nil
		},
		ClientID:  "client",
		SourceID:  "source",
		TicketURL: "https://ticket.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			token := req.Header.Get("x-user-access-token")
			attemptTokens = append(attemptTokens, token)
			if token != "rotated" {
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"endpoint":"wss://stream.test","ticket":"ticket"}`)), Header: make(http.Header)}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := src.fetchTicket(context.Background())
	if err != nil {
		t.Fatalf("fetchTicket = %v", err)
	}
	if ticket.Endpoint != "wss://stream.test" || ticket.Ticket != "ticket" {
		t.Fatalf("ticket = %#v", ticket)
	}
	if len(attemptTokens) != 2 || attemptTokens[0] != "stale" || attemptTokens[1] != "rotated" {
		t.Fatalf("attempt tokens = %v", attemptTokens)
	}
}

// TestCrossPlatformCoveragePersonalFetchTicket401RefreshFailureStaysFatal asserts a failed refresh
// keeps the 401 fatal (not retryable) and wraps the refresh error.
func TestCrossPlatformCoveragePersonalFetchTicket401RefreshFailureStaysFatal(t *testing.T) {
	refreshErr := errors.New("refresh_token exchange failed")
	src, err := NewPersonal(PersonalConfig{
		AccessToken: "stale",
		ForceRefreshToken: func(context.Context, string) (string, error) {
			return "", refreshErr
		},
		ClientID:  "client",
		SourceID:  "source",
		TicketURL: "https://ticket.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header)}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.fetchTicket(context.Background())
	if !errors.Is(err, refreshErr) {
		t.Fatalf("error should wrap refresh error, got %v", err)
	}
	if isRetryablePersonalError(err) {
		t.Fatalf("failed refresh should stay fatal, got retryable %v", err)
	}
}

// brokenBody simulates a response body that fails mid-read, e.g. the server
// closing the connection before the error payload is fully written.
type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (brokenBody) Close() error             { return nil }

func TestCrossPlatformCoveragePortalTicketNon2xxTruncatedBodyKeepsStatus(t *testing.T) {
	for _, tc := range []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusUnauthorized},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusServiceUnavailable, retryable: true},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: brokenBody{}, Header: make(http.Header)}, nil
			})}

			_, status, err := requestPortalTicketAttempt(context.Background(), &PortalTicketConfig{
				TicketURL: "https://ticket.test",
				SourceID:  "source",
			}, client, "token")
			if status != tc.status || err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", tc.status)) {
				t.Fatalf("requestPortalTicketAttempt() status=%d err=%v, want HTTP %d", status, err, tc.status)
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("non-2xx status error should not expose diagnostic body read failure: %v", err)
			}
			var stageErr *portalStageError
			if tc.retryable {
				if !errors.As(err, &stageErr) || stageErr.status != tc.status || !stageErr.retryable {
					t.Fatalf("retryable status should return retryable portalStageError, got %T %v", err, err)
				}
			} else if errors.As(err, &stageErr) && stageErr.retryable {
				t.Fatalf("fatal status should not become retryable stage error: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoveragePortalTicket200TruncatedBodyIsRetryable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: brokenBody{}, Header: make(http.Header)}, nil
	})}

	_, status, err := requestPortalTicketAttempt(context.Background(), &PortalTicketConfig{
		TicketURL: "https://ticket.test",
		SourceID:  "source",
	}, client, "token")
	if status != http.StatusOK || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("requestPortalTicketAttempt() status=%d err=%v, want 200 + io.ErrUnexpectedEOF", status, err)
	}
	var stageErr *portalStageError
	if !errors.As(err, &stageErr) || !stageErr.retryable || stageErr.stage != "ticket_request" {
		t.Fatalf("2xx truncated body should be retryable ticket_request stage, got %T %v", err, err)
	}
}

// TestCrossPlatformCoveragePersonalFetchTicket401TruncatedBodyStaysFatal guards the single
// refresh-retry protection: a 401 whose body fails with unexpected EOF must
// be classified by status (fatal) and never wrapped as retryable, otherwise
// the outer reconnect loop would refresh again on every iteration.
func TestCrossPlatformCoveragePersonalFetchTicket401TruncatedBodyStaysFatal(t *testing.T) {
	attempts := 0
	refreshCalls := 0
	src, err := NewPersonal(PersonalConfig{
		AccessToken: "stale",
		ForceRefreshToken: func(context.Context, string) (string, error) {
			refreshCalls++
			return "rotated", nil
		},
		ClientID:  "client",
		SourceID:  "source",
		TicketURL: "https://ticket.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: brokenBody{}, Header: make(http.Header)}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.fetchTicket(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("fatal 401 error expected, got %v", err)
	}
	if isRetryablePersonalError(err) {
		t.Fatalf("401 with truncated body must stay fatal, got retryable %v", err)
	}
	if refreshCalls != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

// TestCrossPlatformCoveragePersonalFetchTicket200TruncatedBodyStaysRetryable pins the existing
// behavior for success responses: a body read failure on 2xx is a transient
// transport problem and remains retryable.
func TestCrossPlatformCoveragePersonalFetchTicket200TruncatedBodyStaysRetryable(t *testing.T) {
	src, err := NewPersonal(PersonalConfig{
		AccessToken: "token",
		ClientID:    "client",
		SourceID:    "source",
		TicketURL:   "https://ticket.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: brokenBody{}, Header: make(http.Header)}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.fetchTicket(context.Background())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error should wrap io.ErrUnexpectedEOF, got %v", err)
	}
	if !isRetryablePersonalError(err) {
		t.Fatalf("2xx body read failure should stay retryable, got %v", err)
	}
}

// TestCrossPlatformCoveragePersonalFetchTicketAttemptTransportAndPayloadEdges covers the
// remaining fetchTicketAttempt branches: transport failures and retryable
// statuses stay retryable, while a well-formed response missing the endpoint
// or ticket fields stays fatal.
func TestCrossPlatformCoveragePersonalFetchTicketAttemptTransportAndPayloadEdges(t *testing.T) {
	newSource := func(rt roundTripFunc) *PersonalSource {
		src, err := NewPersonal(PersonalConfig{
			AccessToken: "token",
			ClientID:    "client",
			SourceID:    "source",
			TicketURL:   "https://ticket.test",
			HTTPClient:  &http.Client{Transport: rt},
		})
		if err != nil {
			t.Fatal(err)
		}
		return src
	}

	dialErr := errors.New("dial tcp: connection refused")
	src := newSource(func(*http.Request) (*http.Response, error) { return nil, dialErr })
	_, _, err := src.fetchTicketAttempt(context.Background(), "token")
	if !errors.Is(err, dialErr) || !isRetryablePersonalError(err) {
		t.Fatalf("transport failure should stay retryable and wrap cause, got %v", err)
	}

	src = newSource(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("busy")), Header: make(http.Header)}, nil
	})
	_, status, err := src.fetchTicketAttempt(context.Background(), "token")
	if status != http.StatusServiceUnavailable || !isRetryablePersonalError(err) {
		t.Fatalf("503 should stay retryable, got status %d err %v", status, err)
	}

	src = newSource(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"endpoint":"","ticket":""}`)), Header: make(http.Header)}, nil
	})
	_, _, err = src.fetchTicketAttempt(context.Background(), "token")
	if err == nil || isRetryablePersonalError(err) || !strings.Contains(err.Error(), "missing endpoint or ticket") {
		t.Fatalf("missing ticket fields should stay fatal, got %v", err)
	}
}

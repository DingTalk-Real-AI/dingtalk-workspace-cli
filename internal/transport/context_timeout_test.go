package transport

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNewClientMovesHTTPTimeoutToRequestContext(t *testing.T) {
	seenDeadline := make(chan bool, 1)
	httpClient := &http.Client{
		Timeout: 20 * time.Millisecond,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			_, ok := req.Context().Deadline()
			seenDeadline <- ok
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}
	client := NewClient(httpClient)
	if client.HTTPClient.Timeout != 0 {
		t.Fatalf("http.Client.Timeout=%s, want 0", client.HTTPClient.Timeout)
	}
	if client.RequestTimeout != 20*time.Millisecond {
		t.Fatalf("RequestTimeout=%s", client.RequestTimeout)
	}
	_, _ = client.Initialize(context.Background(), "https://mcp.example.test")
	if !<-seenDeadline {
		t.Fatal("request context has no deadline")
	}
	if httpClient.Timeout != 20*time.Millisecond {
		t.Fatal("NewClient mutated caller-owned http.Client")
	}
}

func TestExistingContextDeadlineIsNotExtended(t *testing.T) {
	want := 5 * time.Millisecond
	seen := make(chan time.Duration, 1)
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, _ := req.Context().Deadline()
		seen <- time.Until(deadline)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})})
	ctx, cancel := context.WithTimeout(context.Background(), want)
	defer cancel()
	_, _ = client.Initialize(ctx, "https://mcp.example.test")
	if remaining := <-seen; remaining > 15*time.Millisecond {
		t.Fatalf("existing deadline was extended: remaining=%s", remaining)
	}
}

func TestClientTimeoutWinsOverLongerCallerDeadline(t *testing.T) {
	requestTimeout := 10 * time.Minute
	callerDeadline := time.Now().Add(time.Hour)
	seen := make(chan time.Time, len(supportedProtocolVersions))
	httpClient := &http.Client{
		Timeout: requestTimeout,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			deadline, ok := req.Context().Deadline()
			if !ok {
				t.Fatal("request context has no deadline")
			}
			seen <- deadline
			return nil, errors.New("stop after observing deadline")
		}),
	}
	client := NewClient(httpClient)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()

	_, _ = client.Initialize(ctx, "https://mcp.example.test")
	requestDeadline := <-seen
	if !requestDeadline.Before(callerDeadline.Add(-time.Minute)) {
		t.Fatalf("request deadline=%s, want client timeout earlier than caller deadline=%s", requestDeadline, callerDeadline)
	}
	remaining := time.Until(requestDeadline)
	if remaining < requestTimeout-time.Second || remaining > requestTimeout {
		t.Fatalf("request deadline remaining=%s, want about %s", remaining, requestTimeout)
	}
}

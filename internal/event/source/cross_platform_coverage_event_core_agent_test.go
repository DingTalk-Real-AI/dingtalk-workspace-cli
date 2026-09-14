package source

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageEventCorePersonalSupersededRetry(t *testing.T) {
	source, err := NewPersonal(PersonalConfig{
		AccessToken: "stale",
		ForceRefreshToken: func(context.Context, string) (string, error) {
			return "rotated", nil
		},
		ClassifyRetryReject: func(token string) (bool, error) {
			if token != "rotated" {
				t.Fatalf("classified token = %q", token)
			}
			return true, nil
		},
		ClientID:  "client",
		SourceID:  "source",
		TicketURL: "https://ticket.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("rejected")),
				Header:     make(http.Header),
			}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.fetchTicket(context.Background()); !isRetryablePersonalError(err) {
		t.Fatalf("superseded retry error = %v", err)
	}
}

package personal

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type accessTokenRoundTripper func(*http.Request) (*http.Response, error)

func (f accessTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCrossPlatformCoverageClientResolvesAccessTokenPerRequest(t *testing.T) {
	tokens := []string{"token-a", "token-b"}
	calls := 0
	client := NewClient("https://control.test", Identity{AccessToken: "stale", ClientID: "client", SourceID: "source"})
	client.AccessTokenProvider = func(context.Context) (string, error) {
		token := tokens[calls]
		calls++
		return token, nil
	}
	client.HTTPClient = &http.Client{Transport: accessTokenRoundTripper(func(req *http.Request) (*http.Response, error) {
		want := tokens[calls-1]
		if got := req.Header.Get("Authorization"); got != "Bearer "+want {
			t.Fatalf("Authorization = %q, want Bearer %s", got, want)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true,"result":{"items":[]}}`)), Header: make(http.Header)}, nil
	})}
	for range 2 {
		if _, err := client.ListSubscriptions(context.Background(), ListOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
}

func TestCrossPlatformCoverageClientDoesNotFallBackAfterProviderFailure(t *testing.T) {
	want := errors.New("keychain failed")
	client := NewClient("https://control.test", Identity{AccessToken: "stale", ClientID: "client", SourceID: "source"})
	client.AccessTokenProvider = func(context.Context) (string, error) { return "", want }
	client.HTTPClient = &http.Client{Transport: accessTokenRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP must not run after token provider failure")
		return nil, nil
	})}
	_, err := client.ListSubscriptions(context.Background(), ListOptions{})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

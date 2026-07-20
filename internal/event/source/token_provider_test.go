package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type tokenProviderRoundTripper func(*http.Request) (*http.Response, error)

func (f tokenProviderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCrossPlatformCoveragePersonalSourceResolvesTokenForEveryTicketRequest(t *testing.T) {
	tokens := []string{"token-a", "token-b"}
	calls := 0
	source, err := NewPersonal(PersonalConfig{
		AccessTokenProvider: func(context.Context) (string, error) {
			token := tokens[calls]
			calls++
			return token, nil
		},
		ClientID:  "client",
		SourceID:  "source",
		TicketURL: "https://ticket.test",
		HTTPClient: &http.Client{Transport: tokenProviderRoundTripper(func(req *http.Request) (*http.Response, error) {
			want := tokens[calls-1]
			if got := req.Header.Get("x-user-access-token"); got != want {
				t.Fatalf("token header = %q, want %q", got, want)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"endpoint":"wss://stream.test","ticket":"ticket"}`)), Header: make(http.Header)}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := source.fetchTicket(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
}

func TestCrossPlatformCoveragePortalTicketProviderFailureStopsBeforeHTTP(t *testing.T) {
	want := errors.New("token store failed")
	httpCalled := false
	_, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL:           "https://ticket.test",
		AccessTokenProvider: func(context.Context) (string, error) { return "", want },
		SourceID:            "source",
		HTTPClient: &http.Client{Transport: tokenProviderRoundTripper(func(*http.Request) (*http.Response, error) {
			httpCalled = true
			return nil, errors.New("unexpected HTTP")
		})},
	})
	if !errors.Is(err, want) || httpCalled {
		t.Fatalf("request error = %v, httpCalled=%v", err, httpCalled)
	}
}

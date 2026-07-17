package helpers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

func TestCrossPlatformCoverageAICardAccessTokenRemainingEdges(t *testing.T) {
	originalBase := dingtalkCardAPIBase
	defer func() { dingtalkCardAPIBase = originalBase }()

	client := newAICardClient("client", "secret", "template")
	client.httpClient = &http.Client{Transport: mailRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport")
	})}
	dingtalkCardAPIBase = "https://api.invalid"
	if _, err := client.accessToken(context.Background()); err == nil {
		t.Fatal("token transport error returned nil")
	}

	for _, tc := range []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"http", http.StatusInternalServerError, "failure", false},
		{"invalid-json", http.StatusOK, "{", false},
		{"empty-token", http.StatusOK, `{}`, false},
		{"default-expiry", http.StatusOK, `{"accessToken":"token","expireIn":0}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			dingtalkCardAPIBase = server.URL
			client := newAICardClient("client", "secret", "template")
			got, err := client.accessToken(context.Background())
			if tc.ok && (err != nil || got != "token") {
				t.Fatalf("token=%q err=%v", got, err)
			}
			if !tc.ok && err == nil {
				t.Fatal("error case returned nil")
			}
		})
	}
}

func TestCrossPlatformCoverageAICardCallRawAndCheckedRemainingEdges(t *testing.T) {
	originalBase := dingtalkCardAPIBase
	defer func() { dingtalkCardAPIBase = originalBase }()
	cached := func() *aiCardClient {
		c := newAICardClient("client", "secret", "template")
		c.token = "token"
		c.tokenExp = time.Now().Add(time.Hour)
		return c
	}

	c := cached()
	if _, err := c.callRaw(context.Background(), http.MethodPost, "/path", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("payload marshal error returned nil")
	}
	dingtalkCardAPIBase = "%"
	if _, err := newAICardClient("client", "secret", "template").callRaw(context.Background(), http.MethodGet, "/path", nil); err == nil {
		t.Fatal("access token error returned nil")
	}
	if _, err := cached().callRaw(context.Background(), http.MethodGet, "/path", nil); err == nil {
		t.Fatal("request construction error returned nil")
	}
	if err := cached().callChecked(context.Background(), http.MethodGet, "/path", nil); err == nil {
		t.Fatal("checked transport error returned nil")
	}

	dingtalkCardAPIBase = "https://api.invalid"
	c = cached()
	c.httpClient = &http.Client{Transport: mailRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport")
	})}
	if _, err := c.callRaw(context.Background(), http.MethodGet, "/path", nil); err == nil {
		t.Fatal("call transport error returned nil")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"errorMsg":"bad"}`))
	}))
	defer server.Close()
	dingtalkCardAPIBase = server.URL
	if err := cached().callChecked(context.Background(), http.MethodPost, "/deliver", nil); err == nil {
		t.Fatal("business failure returned nil")
	}
}

func TestCrossPlatformCoverageAICardDeliveryStreamingAndSleepEdges(t *testing.T) {
	recorder, server := newCardAPIServer(t)
	withCardAPIBase(t, server.URL)
	c := newAICardClient("client", "secret", defaultAICardTemplateID)
	recorder.fail["POST /v1.0/card/instances/deliver"] = 500
	if _, err := c.createAndDeliver(context.Background(), groupCallback()); err == nil {
		t.Fatal("deliver error returned nil")
	}
	recorder.fail = map[string]int{}
	if _, err := c.createAndDeliver(context.Background(), &chatbot.BotCallbackDataModel{}); err == nil {
		t.Fatal("one-to-one missing sender returned nil")
	}

	long := strings.Repeat("界", aiCardMaxContent+1)
	if err := c.streamingUpdate(context.Background(), &aiCardInstance{outTrackID: "track"}, long, false, false); err != nil {
		t.Fatalf("long streaming update: %v", err)
	}
	if err := sleepCtx(context.Background(), time.Nanosecond); err != nil {
		t.Fatalf("timer sleep: %v", err)
	}

	originalGap := aiCardFrameGap
	aiCardFrameGap = time.Hour
	defer func() { aiCardFrameGap = originalGap }()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.finalize(cancelled, &aiCardInstance{outTrackID: "track"}, "content"); !errors.Is(err, context.Canceled) {
		t.Fatalf("finalize cancel=%v", err)
	}

	finishCtx := context.Background()
	finishServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer finishServer.Close()
	dingtalkCardAPIBase = finishServer.URL
	finishClient := newAICardClient("client", "secret", "template")
	finishClient.token, finishClient.tokenExp = "token", time.Now().Add(time.Hour)
	originalSleep := aiCardSleepCtx
	aiCardSleepCtx = func(context.Context, time.Duration) error { return context.Canceled }
	defer func() { aiCardSleepCtx = originalSleep }()
	if err := finishClient.finish(finishCtx, &aiCardInstance{outTrackID: "track", inputing: true}, "content"); !errors.Is(err, context.Canceled) {
		t.Fatalf("finish cancel=%v", err)
	}
}

func TestCrossPlatformCoverageCardMarkdownRemainingEdges(t *testing.T) {
	table := "intro\n| A | B |\n|---|---|\nend"
	if got := ensureCardTableBlankLines(table); !strings.Contains(got, "intro\n\n| A") {
		t.Fatalf("table normalization=%q", got)
	}
	input := "plain\nnext\n> quote one\n> quote two\nafter\n```go\ncode\n```\n\n- item"
	got := fixCardNewlines(input)
	for _, want := range []string{"plain<br>next", "> quote one<br>quote two", "```go\ncode\n```", "\n- item"} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized output missing %q: %q", want, got)
		}
	}
}

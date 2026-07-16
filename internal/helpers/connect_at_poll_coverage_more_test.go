package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"
)

type atPollSignalForwarder struct {
	mu      sync.Mutex
	reply   string
	err     error
	delay   time.Duration
	prompts []string
	done    chan struct{}
}

func (f *atPollSignalForwarder) forward(_ context.Context, _ string, text string) (string, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	f.prompts = append(f.prompts, text)
	f.mu.Unlock()
	select {
	case f.done <- struct{}{}:
	default:
	}
	return f.reply, f.err
}

func (*atPollSignalForwarder) label() string { return "poll-test" }

type atPollStreamingForwarder struct{ *atPollSignalForwarder }

func (*atPollStreamingForwarder) canStream() bool { return true }
func (f *atPollStreamingForwarder) forwardStream(ctx context.Context, convID, text string, _ func(string)) (string, error) {
	return f.forward(ctx, convID, text)
}

func waitAtPollForward(t *testing.T, f *atPollSignalForwarder) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for at-poll forward")
	}
}

func waitAtPollStop(t *testing.T, stopped <-chan struct{}) {
	t.Helper()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for at-poll shutdown")
	}
}

func newAtPollForTest(fwd forwarder, extras *connectExtras) *atMentionPoller {
	return &atMentionPoller{
		clientID: "client", botClient: newAICardClient("client", "secret", ""),
		fwd: fwd, queue: newConvQueue(), dedup: newMsgDedup(20),
		health: nil, extras: extras, channel: "test",
	}
}

func TestCrossPlatformCoverageAtMentionPollerStartAndPollCoverage(t *testing.T) {
	origAfter := helperAfter
	origExecutable := atPollExecutable
	origCommand := atPollCommandContext
	origTicker := atPollTicker
	ticks, stopTicker := origTicker(time.Hour)
	_ = ticks
	stopTicker()
	t.Cleanup(func() {
		helperAfter = origAfter
		atPollExecutable = origExecutable
		atPollCommandContext = origCommand
		atPollTicker = origTicker
	})
	helperAfter = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	tickerStarted := make(chan struct{})
	atPollTicker = func(time.Duration) (<-chan time.Time, func()) {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		close(tickerStarted)
		return ch, func() {}
	}
	atPollExecutable = func() (string, error) { return "", errors.New("executable") }
	ctx, cancel := context.WithCancel(context.Background())
	p := newAtPollForTest(&atPollSignalForwarder{done: make(chan struct{}, 1)}, &connectExtras{})
	stopped := p.start(ctx)
	select {
	case <-tickerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for at-poll startup")
	}
	cancel()
	waitAtPollStop(t, stopped)

	p.poll(context.Background())
	atPollExecutable = func() (string, error) { return "test", nil }
	atPollCommandContext = func(context.Context, string, ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 3")
	}
	p.poll(context.Background())

	outputs := []string{
		`{`,
		`{"success":false}`,
		`{"success":true,"result":{"items":[{"msgId":""},{"msgId":"dup","conversationType":"2"},{"msgId":"dup","conversationType":"2"},{"msgId":"other","conversationType":"1"},{"msgId":"valid","conversationType":"2","content":""}]}}`,
	}
	p.dedup.first("dup")
	for _, output := range outputs {
		value := output
		atPollCommandContext = func(context.Context, string, ...string) *exec.Cmd {
			cmd := exec.Command("sh", "-c", "printf %s \"$AT_POLL_OUTPUT\"")
			cmd.Env = append(cmd.Environ(), "AT_POLL_OUTPUT="+value)
			return cmd
		}
		p.poll(context.Background())
	}
}

func TestCrossPlatformCoverageAtMentionPollerHandleCoverage(t *testing.T) {
	base := atMentionMessage{MsgID: "m", OpenConversationID: "conv", SenderStaffID: "user", SenderNick: "nick", Content: `{"text":"hello"}`, ConversationType: "2"}

	p := newAtPollForTest(&atPollSignalForwarder{done: make(chan struct{}, 1)}, &connectExtras{})
	empty := base
	empty.Content = ""
	p.handleMessage(context.Background(), empty)

	denied := &atPollSignalForwarder{done: make(chan struct{}, 1)}
	p = newAtPollForTest(denied, &connectExtras{gate: newConnectGate([]string{"other"}, nil, 0)})
	p.handleMessage(context.Background(), base)
	time.Sleep(20 * time.Millisecond)
	select {
	case <-denied.done:
		t.Fatal("denied message reached forwarder")
	default:
	}

	plain := &atPollSignalForwarder{done: make(chan struct{}, 2)}
	kb := &knowledgeBase{chunks: []knowledgeChunk{{text: "hello handbook", terms: map[string]int{"hello": 1}}}}
	p = newAtPollForTest(plain, &connectExtras{kb: kb, persona: " persona "})
	fallback := base
	fallback.SenderNick = ""
	fallback.OpenConversationID = ""
	p.handleMessage(context.Background(), fallback)
	waitAtPollForward(t, plain)

	failing := &atPollSignalForwarder{err: errors.New("agent failed"), done: make(chan struct{}, 1)}
	p = newAtPollForTest(failing, &connectExtras{})
	p.handleMessage(context.Background(), fallback)
	waitAtPollForward(t, failing)

	origBase := dingtalkCardAPIBase
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_, _ = io.WriteString(w, `{"accessToken":"token","expireIn":7200}`)
			return
		}
		http.Error(w, "send failed", http.StatusBadRequest)
	}))
	dingtalkCardAPIBase = server.URL
	t.Cleanup(func() {
		dingtalkCardAPIBase = origBase
		server.Close()
	})
	streamBase := &atPollSignalForwarder{reply: "reply", done: make(chan struct{}, 2)}
	stream := &atPollStreamingForwarder{atPollSignalForwarder: streamBase}
	p = newAtPollForTest(stream, &connectExtras{})
	p.handleMessage(context.Background(), base)
	waitAtPollForward(t, streamBase)

	slow := &atPollSignalForwarder{delay: 30 * time.Millisecond, done: make(chan struct{}, 4)}
	p = newAtPollForTest(slow, &connectExtras{})
	p.handleMessage(context.Background(), base)
	second := base
	second.MsgID = "m2"
	second.Content = `{"text":"second"}`
	third := base
	third.MsgID = "m3"
	third.Content = `{"text":"third"}`
	p.handleMessage(context.Background(), second)
	p.handleMessage(context.Background(), third)
	waitAtPollForward(t, slow)
	waitAtPollForward(t, slow)
}

func TestCrossPlatformCoverageAtMentionPollerSendGroupReplyCoverage(t *testing.T) {
	origBase := dingtalkCardAPIBase
	t.Cleanup(func() { dingtalkCardAPIBase = origBase })
	p := newAtPollForTest(&atPollSignalForwarder{done: make(chan struct{}, 1)}, &connectExtras{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_, _ = io.WriteString(w, `{"accessToken":"token","expireIn":7200}`)
		case "/v1.0/robot/groupMessages/send":
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	dingtalkCardAPIBase = server.URL
	if err := p.sendGroupReply(context.Background(), "conv", "reply"); err != nil {
		t.Fatalf("send success: %v", err)
	}
	server.Close()
	if err := p.sendGroupReply(context.Background(), "conv", "reply"); err == nil {
		t.Fatal("transport error returned nil")
	}

	p.botClient.token = "token"
	p.botClient.tokenExp = time.Now().Add(time.Hour)
	dingtalkCardAPIBase = "%"
	if err := p.sendGroupReply(context.Background(), "conv", "reply"); err == nil {
		t.Fatal("invalid request URL returned nil")
	}

	p.botClient.token = ""
	p.botClient.tokenExp = time.Time{}
	if err := p.sendGroupReply(context.Background(), "conv", "reply"); err == nil {
		t.Fatal("token request error returned nil")
	}
}

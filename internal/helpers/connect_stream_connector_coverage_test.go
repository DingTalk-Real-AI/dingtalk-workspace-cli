package helpers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/card"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

type fakeConnectStreamClient struct {
	chatHandler chatbot.IChatBotMessageHandler
	cardHandler card.ICardCallbackHandler
	start       func(context.Context, chatbot.IChatBotMessageHandler) error
	closed      bool
}

func (c *fakeConnectStreamClient) RegisterChatBotCallbackRouter(handler chatbot.IChatBotMessageHandler) {
	c.chatHandler = handler
}

func (c *fakeConnectStreamClient) RegisterCardCallbackRouter(handler card.ICardCallbackHandler) {
	c.cardHandler = handler
}

func (c *fakeConnectStreamClient) Start(ctx context.Context) error {
	if c.start != nil {
		return c.start(ctx, c.chatHandler)
	}
	return nil
}

func (c *fakeConnectStreamClient) Close() { c.closed = true }

type fakeConnectReplier struct {
	mu       sync.Mutex
	errors   []error
	calls    int
	kinds    []string
	payloads []string
	events   chan struct{}
}

func (r *fakeConnectReplier) next(kind, payload string) error {
	r.mu.Lock()
	index := r.calls
	r.calls++
	r.kinds = append(r.kinds, kind)
	r.payloads = append(r.payloads, payload)
	var err error
	if index < len(r.errors) {
		err = r.errors[index]
	}
	r.mu.Unlock()
	select {
	case r.events <- struct{}{}:
	default:
	}
	return err
}

func (r *fakeConnectReplier) SimpleReplyMarkdown(_ context.Context, _ string, _, content []byte) error {
	return r.next("markdown", string(content))
}

func (r *fakeConnectReplier) SimpleReplyText(_ context.Context, _ string, content []byte) error {
	return r.next("text", string(content))
}

type fakeConnectMedia struct {
	messagePath string
	messageErr  error
	unionID     string
	unionErr    error
	dentryPath  string
	dentryErr   error
}

func (m *fakeConnectMedia) downloadMessageFile(context.Context, string, string) (string, error) {
	return m.messagePath, m.messageErr
}

func (m *fakeConnectMedia) downloadMessageFileNamed(context.Context, string, string, string) (string, error) {
	return m.messagePath, m.messageErr
}

func (m *fakeConnectMedia) downloadRecoveredChatRecordFile(context.Context, fileInboundInfo) (string, error) {
	return m.messagePath, m.messageErr
}

func (m *fakeConnectMedia) getUserUnionID(context.Context, string) (string, error) {
	return m.unionID, m.unionErr
}

func (m *fakeConnectMedia) downloadDentryFile(context.Context, int64, int64, string, string) (string, error) {
	return m.dentryPath, m.dentryErr
}

type basicConnectorForwarder struct {
	mu      sync.Mutex
	reply   string
	err     error
	prompts []string
}

func (f *basicConnectorForwarder) forward(_ context.Context, _ string, text string) (string, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, text)
	f.mu.Unlock()
	return f.reply, f.err
}

func (f *basicConnectorForwarder) promptSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

func (*basicConnectorForwarder) label() string { return "fake" }

type resetConnectorForwarder struct {
	*basicConnectorForwarder
	resets   []string
	clears   []string
	clearErr error
}

func (f *resetConnectorForwarder) resetSession(convID string) { f.resets = append(f.resets, convID) }

func (f *resetConnectorForwarder) clearSession(_ context.Context, convID string) error {
	f.clears = append(f.clears, convID)
	return f.clearErr
}

type streamingConnectorForwarder struct {
	*basicConnectorForwarder
	streamed bool
}

func (*streamingConnectorForwarder) canStream() bool { return true }

func (f *streamingConnectorForwarder) forwardStream(_ context.Context, _ string, text string, onDelta func(string)) (string, error) {
	f.streamed = true
	f.mu.Lock()
	f.prompts = append(f.prompts, text)
	f.mu.Unlock()
	if onDelta != nil {
		onDelta("partial")
	}
	return f.reply, f.err
}

type closingConnectorForwarder struct {
	*basicConnectorForwarder
	closed   bool
	closeErr error
}

type slowConnectorForwarder struct {
	*basicConnectorForwarder
	delay time.Duration
}

func (f *slowConnectorForwarder) forward(ctx context.Context, convID, text string) (string, error) {
	time.Sleep(f.delay)
	return f.basicConnectorForwarder.forward(ctx, convID, text)
}

func (f *closingConnectorForwarder) close() error {
	f.closed = true
	return f.closeErr
}

type fakeOwnerNotifier struct{}

func (fakeOwnerNotifier) sendOTOText(context.Context, []string, string) error { return nil }

func preserveConnectStreamHooks(t *testing.T) {
	t.Helper()
	oldStream := newConnectStreamClient
	oldReplier := newConnectChatReplier
	oldMedia := newConnectMediaClient
	oldLockDir := connectLockDir
	oldDaemonDir := connectDaemonDirOverride
	oldSleep := helperSleep
	oldCardRepair := runConnectCardRepair
	t.Cleanup(func() {
		newConnectStreamClient = oldStream
		newConnectChatReplier = oldReplier
		newConnectMediaClient = oldMedia
		connectLockDir = oldLockDir
		connectDaemonDirOverride = oldDaemonDir
		helperSleep = oldSleep
		runConnectCardRepair = oldCardRepair
	})
}

func connectorMessage(id, text string) *chatbot.BotCallbackDataModel {
	return &chatbot.BotCallbackDataModel{
		ConversationId:   "conv-" + id,
		MsgId:            id,
		SenderNick:       "Sender",
		SenderStaffId:    "staff-" + id,
		ConversationType: "2",
		SessionWebhook:   "https://webhook.test/" + id,
		Text:             chatbot.BotCallbackDataTextModel{Content: text},
		Msgtype:          "text",
	}
}

func runConnectorScenario(t *testing.T, messages []*chatbot.BotCallbackDataModel, fwd forwarder, media connectMediaClient, extras *connectExtras, replyErrors []error, wantReplyCalls int, wantTurnCalls ...int) (*fakeConnectReplier, *fakeConnectStreamClient) {
	return runConnectorScenarioWithCard(t, messages, fwd, media, nil, extras, replyErrors, wantReplyCalls, wantTurnCalls...)
}

func runConnectorScenarioWithCard(t *testing.T, messages []*chatbot.BotCallbackDataModel, fwd forwarder, media connectMediaClient, cardCli *aiCardClient, extras *connectExtras, replyErrors []error, wantReplyCalls int, wantTurnCalls ...int) (*fakeConnectReplier, *fakeConnectStreamClient) {
	t.Helper()
	preserveConnectStreamHooks(t)
	connectLockDir = t.TempDir()
	connectDaemonDirOverride = t.TempDir()
	helperSleep = func(time.Duration) {}
	runConnectCardRepair = func(repair func()) { repair() }
	replier := &fakeConnectReplier{errors: replyErrors, events: make(chan struct{}, 16)}
	stream := &fakeConnectStreamClient{}
	turnEvents := make(chan struct{}, len(messages))
	if extras == nil {
		extras = &connectExtras{}
	} else {
		copy := *extras
		extras = &copy
	}
	extras.onTurnDone = func() { turnEvents <- struct{}{} }
	wantTurns := 0
	if len(messages) > 0 {
		wantTurns = 1
	}
	if len(wantTurnCalls) > 0 {
		wantTurns = wantTurnCalls[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream.start = func(_ context.Context, handler chatbot.IChatBotMessageHandler) error {
		if handler == nil {
			return errors.New("chat handler was not registered")
		}
		for _, message := range messages {
			if _, err := handler(context.Background(), message); err != nil {
				return err
			}
		}
		for range wantReplyCalls {
			select {
			case <-replier.events:
			case <-time.After(2 * time.Second):
				return errors.New("timed out waiting for reply")
			}
		}
		for range wantTurns {
			select {
			case <-turnEvents:
			case <-time.After(2 * time.Second):
				return errors.New("timed out waiting for queued turn")
			}
		}
		cancel()
		return nil
	}
	newConnectStreamClient = func(string, string, time.Duration) connectStreamClient { return stream }
	newConnectChatReplier = func() connectChatReplier { return replier }
	if media == nil {
		media = &fakeConnectMedia{}
	}
	newConnectMediaClient = func(string, string) connectMediaClient { return media }
	if err := runStreamConnector(ctx, "custom", "client-"+strings.ReplaceAll(t.Name(), "/", "-"), "secret", fwd, cardCli, extras); err != nil {
		t.Fatalf("runStreamConnector() error = %v", err)
	}
	return replier, stream
}

func TestCrossPlatformCoverageRunStreamConnectorBasicMessageEdges(t *testing.T) {
	t.Run("blank fallback and missing webhook drop", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: "unused"}
		blank := connectorMessage("blank", "")
		noWebhook := connectorMessage("webhook", "hello")
		noWebhook.SessionWebhook = ""
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{blank, noWebhook}, fwd, nil, nil, nil, 0)
		prompts := fwd.promptSnapshot()
		if len(prompts) != 1 || !strings.Contains(prompts[0], "原始消息 JSON") {
			t.Fatalf("fallback messages forwarded: %#v", prompts)
		}
	})

	t.Run("text reply and duplicate", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: "answer"}
		message := connectorMessage("duplicate", "question")
		replier, stream := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{message, message}, fwd, nil, nil, nil, 1)
		if replier.calls != 1 || replier.kinds[0] != "text" || !stream.closed {
			t.Fatalf("reply calls=%d kinds=%#v closed=%v", replier.calls, replier.kinds, stream.closed)
		}
	})

	t.Run("structured fallbacks and sender fallback", func(t *testing.T) {
		interactive := connectorMessage("interactive", "")
		interactive.Msgtype = "interactiveCard"
		interactive.SenderNick = ""
		interactive.Content = map[string]any{"cardContent": []any{map[string]any{"children": []any{
			map[string]any{"elementType": "TEXT", "value": "@bot"},
			map[string]any{"elementType": "TEXT", "value": " instruction"},
		}}}}
		markdown := connectorMessage("markdown", "")
		markdown.Msgtype = "markdown"
		markdown.Content = map[string]any{"text": " markdown body "}
		fwd := &basicConnectorForwarder{reply: "ok"}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{interactive, markdown}, fwd, nil, nil, nil, 2, 2)
		prompts := fwd.promptSnapshot()
		if len(prompts) != 2 {
			t.Fatalf("structured prompts = %#v", prompts)
		}
	})

	t.Run("conversation id fallback and queued merge", func(t *testing.T) {
		fallback := connectorMessage("fallback", "question")
		fallback.ConversationId = ""
		first := connectorMessage("merge-1", "first")
		second := connectorMessage("merge-2", "second")
		third := connectorMessage("merge-3", "third")
		second.ConversationId = first.ConversationId
		third.ConversationId = first.ConversationId
		fwd := &slowConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "ok"}, delay: 30 * time.Millisecond}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{fallback, first, second, third}, fwd, nil, nil, nil, 3, 3)
		foundMerged := false
		prompts := fwd.promptSnapshot()
		for _, prompt := range prompts {
			if strings.Contains(prompt, "连续发送") {
				foundMerged = true
			}
		}
		if !foundMerged {
			t.Fatalf("queued prompts were not merged: %#v", prompts)
		}
	})

	t.Run("access gate denial", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: "unused"}
		extras := &connectExtras{gate: newConnectGate([]string{"allowed"}, nil, 0)}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("denied", "question")}, fwd, nil, extras, nil, 0, 0)
		if len(fwd.promptSnapshot()) != 0 {
			t.Fatal("denied message reached forwarder")
		}
	})
}

func TestCrossPlatformCoverageRunStreamConnectorForwardAndReplyEdges(t *testing.T) {
	t.Run("long markdown and retry success", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: strings.Repeat("long", 80)}
		replier, _ := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("long", "question")}, fwd, nil, &connectExtras{persona: "Persona"}, []error{errors.New("one"), errors.New("two"), nil}, 3)
		prompts := fwd.promptSnapshot()
		if replier.calls != 3 || replier.kinds[0] != "markdown" || !strings.Contains(prompts[0], "Persona") {
			t.Fatalf("markdown retry = calls %d kinds %#v prompts %#v", replier.calls, replier.kinds, prompts)
		}
	})

	t.Run("reply retry exhaustion", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: "answer"}
		errs := []error{errors.New("1"), errors.New("2"), errors.New("3"), errors.New("4")}
		replier, _ := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("exhaust", "question")}, fwd, nil, nil, errs, 4)
		if replier.calls != 4 {
			t.Fatalf("exhausted reply calls = %d", replier.calls)
		}
	})

	t.Run("ordinary forward error", func(t *testing.T) {
		fwd := &basicConnectorForwarder{err: errors.New("backend")}
		replier, _ := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("error", "question")}, fwd, nil, nil, nil, 1)
		if !strings.Contains(replier.payloads[0], "调用失败") {
			t.Fatalf("error reply = %q", replier.payloads[0])
		}
	})

	t.Run("deadline resets session", func(t *testing.T) {
		fwd := &resetConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{err: context.DeadlineExceeded}}
		replier, _ := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("deadline", "question")}, fwd, nil, nil, nil, 1)
		if len(fwd.resets) != 1 || !strings.Contains(replier.payloads[0], "自动重置") {
			t.Fatalf("deadline resets=%#v reply=%#v", fwd.resets, replier.payloads)
		}
	})

	t.Run("streaming forwarder", func(t *testing.T) {
		fwd := &streamingConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "stream answer"}}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("stream", "question")}, fwd, nil, nil, nil, 1)
		if !fwd.streamed {
			t.Fatal("streaming forwarder was not used")
		}
	})

	t.Run("knowledge augmentation", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: "answer"}
		kb := &knowledgeBase{chunks: []knowledgeChunk{{source: "guide.md", text: "alpha guidance", terms: knowledgeTerms("alpha guidance")}}}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("knowledge", "alpha question")}, fwd, nil, &connectExtras{kb: kb}, nil, 1)
		prompts := fwd.promptSnapshot()
		if !strings.Contains(prompts[0], "guide.md") {
			t.Fatalf("knowledge prompt = %q", prompts[0])
		}
	})
}

func TestCrossPlatformCoverageRunStreamConnectorControlAndLifecycleEdges(t *testing.T) {
	t.Run("asynchronous card repair runner", func(t *testing.T) {
		done := make(chan struct{})
		runConnectCardRepair(func() { close(done) })
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("card repair runner did not start")
		}
	})

	for _, tc := range []struct {
		name     string
		text     string
		clearErr error
	}{
		{"new", "/new", nil},
		{"clear", "/clear", nil},
		{"clear error", "/clear", errors.New("clear")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fwd := &resetConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "unused"}, clearErr: tc.clearErr}
			runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("control", tc.text)}, fwd, nil, nil, nil, 1)
			if tc.text == "/new" && len(fwd.resets) != 1 {
				t.Fatalf("new resets = %#v", fwd.resets)
			}
			if tc.text == "/clear" && len(fwd.clears) != 1 {
				t.Fatalf("clear calls = %#v", fwd.clears)
			}
		})
	}

	t.Run("unsupported control", func(t *testing.T) {
		fwd := &basicConnectorForwarder{reply: "unused"}
		replier, _ := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("unsupported", "/new")}, fwd, nil, nil, nil, 1)
		if !strings.Contains(replier.payloads[0], "暂不支持") {
			t.Fatalf("unsupported control reply = %q", replier.payloads[0])
		}
	})

	t.Run("control reply error", func(t *testing.T) {
		fwd := &resetConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{}}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("control-error", "/new")}, fwd, nil, nil, []error{errors.New("send")}, 1)
	})

	for _, closeErr := range []error{nil, errors.New("close")} {
		name := "close success"
		if closeErr != nil {
			name = "close error"
		}
		t.Run(name, func(t *testing.T) {
			fwd := &closingConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{}, closeErr: closeErr}
			runConnectorScenario(t, nil, fwd, nil, nil, nil, 0)
			if !fwd.closed {
				t.Fatal("forwarder was not closed")
			}
		})
	}

	t.Run("stream start error", func(t *testing.T) {
		preserveConnectStreamHooks(t)
		connectLockDir = t.TempDir()
		connectDaemonDirOverride = t.TempDir()
		stream := &fakeConnectStreamClient{start: func(context.Context, chatbot.IChatBotMessageHandler) error { return errors.New("start") }}
		newConnectStreamClient = func(string, string, time.Duration) connectStreamClient { return stream }
		newConnectChatReplier = func() connectChatReplier { return &fakeConnectReplier{events: make(chan struct{}, 1)} }
		newConnectMediaClient = func(string, string) connectMediaClient { return &fakeConnectMedia{} }
		if err := runStreamConnector(context.Background(), "custom", "start-error", "secret", &basicConnectorForwarder{}, nil, nil); err == nil {
			t.Fatal("stream start error was ignored")
		}
	})

	t.Run("duplicate connector lock", func(t *testing.T) {
		preserveConnectStreamHooks(t)
		connectLockDir = t.TempDir()
		release, err := acquireConnectLock("duplicate-client")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := runStreamConnector(context.Background(), "custom", "duplicate-client", "secret", &basicConnectorForwarder{}, nil, nil); err == nil {
			t.Fatal("duplicate connector lock was ignored")
		}
	})

	t.Run("approval registration and ordinary response", func(t *testing.T) {
		gate := newApprovalGate("")
		approval := newTextApprovalOrchestrator(gate, nil, "owner", fakeOwnerNotifier{})
		extras := &connectExtras{approval: approval}
		fwd := &basicConnectorForwarder{reply: "ordinary answer"}
		_, stream := runConnectorScenario(t, []*chatbot.BotCallbackDataModel{connectorMessage("approval", "question")}, fwd, nil, extras, nil, 1)
		prompts := fwd.promptSnapshot()
		if stream.cardHandler == nil || !strings.Contains(prompts[0], "ACTION") {
			t.Fatalf("approval registration=%v prompt=%q", stream.cardHandler != nil, prompts[0])
		}
		_, _ = stream.cardHandler(context.Background(), &card.CardRequest{})
	})

	t.Run("owner decision consumed", func(t *testing.T) {
		gate := newApprovalGate("")
		_ = gate.Submit(ApprovalRequest{Requester: "requester", Summary: "pending"})
		approval := newTextApprovalOrchestrator(gate, nil, "owner", fakeOwnerNotifier{})
		message := connectorMessage("decision", "拒绝")
		message.SenderStaffId = "owner"
		fwd := &basicConnectorForwarder{reply: "unused"}
		runConnectorScenario(t, []*chatbot.BotCallbackDataModel{message}, fwd, nil, &connectExtras{approval: approval}, nil, 0)
		if len(fwd.promptSnapshot()) != 0 {
			t.Fatal("owner decision reached forwarder")
		}
	})

	t.Run("approval group reply helper", func(t *testing.T) {
		replier := &fakeConnectReplier{events: make(chan struct{}, 2)}
		groupReply := connectApprovalGroupReply(replier, "webhook", "custom")
		if err := groupReply(context.Background(), "conv", "short"); err != nil {
			t.Fatal(err)
		}
		if err := groupReply(context.Background(), "conv", strings.Repeat("x", 201)); err != nil {
			t.Fatal(err)
		}
		if len(replier.kinds) != 2 || replier.kinds[0] != "text" || replier.kinds[1] != "markdown" {
			t.Fatalf("group reply kinds = %#v", replier.kinds)
		}
	})
}

func TestCrossPlatformCoverageRunStreamConnectorMediaEdges(t *testing.T) {
	cases := []struct {
		name    string
		message *chatbot.BotCallbackDataModel
		media   *fakeConnectMedia
		want    string
	}{
		{
			name: "picture success",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("pic-ok", "")
				m.Msgtype = "picture"
				m.Content = map[string]any{"downloadCode": "code"}
				return m
			}(),
			media: &fakeConnectMedia{messagePath: "/tmp/picture.png"}, want: "/tmp/picture.png",
		},
		{
			name: "picture failure",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("pic-fail", "")
				m.Msgtype = "picture"
				m.Content = map[string]any{"downloadCode": "code"}
				return m
			}(),
			media: &fakeConnectMedia{messageErr: errors.New("download")}, want: "图片下载失败",
		},
		{
			name: "picture success with text",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("pic-text", "also text")
				m.Msgtype = "picture"
				m.Content = map[string]any{"downloadCode": "code"}
				return m
			}(),
			media: &fakeConnectMedia{messagePath: "/tmp/picture.png"}, want: "同时附了一张图片",
		},
		{
			name: "client file success with text",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("file-ok", "please inspect")
				m.Msgtype = "file"
				m.Content = map[string]any{"downloadCode": "code", "fileName": "report.txt"}
				return m
			}(),
			media: &fakeConnectMedia{messagePath: "/tmp/report.txt"}, want: "/tmp/report.txt",
		},
		{
			name: "client file success without text",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("file-empty", "")
				m.Msgtype = "file"
				m.Content = map[string]any{"downloadCode": "code", "fileName": "report.txt"}
				return m
			}(),
			media: &fakeConnectMedia{messagePath: "/tmp/report.txt"}, want: "/tmp/report.txt",
		},
		{
			name: "client file failure",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("file-fail", "")
				m.Msgtype = "file"
				m.Content = map[string]any{"downloadCode": "code", "fileName": "report.txt"}
				return m
			}(),
			media: &fakeConnectMedia{messageErr: errors.New("download")}, want: "原始内容下载失败",
		},
		{
			name: "dentry success",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("dentry-ok", "")
				m.Msgtype = "file"
				m.Content = map[string]any{"dentryId": float64(1), "spaceId": float64(2), "fileName": "data.csv"}
				return m
			}(),
			media: &fakeConnectMedia{unionID: "union", dentryPath: "/tmp/data.csv"}, want: "/tmp/data.csv",
		},
		{
			name: "dentry download failure",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("dentry-download-fail", "")
				m.Msgtype = "file"
				m.Content = map[string]any{"dentryId": float64(1), "spaceId": float64(2), "fileName": "data.csv"}
				return m
			}(),
			media: &fakeConnectMedia{unionID: "union", dentryErr: errors.New("download")}, want: "dentryId=1",
		},
		{
			name: "dentry metadata fallback",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("dentry-fail", "")
				m.Msgtype = "file"
				m.Content = map[string]any{"dentryId": float64(1), "spaceId": float64(2), "fileName": "data.csv", "fileType": "csv", "fileSize": float64(42)}
				return m
			}(),
			media: &fakeConnectMedia{unionErr: errors.New("union")}, want: "dentryId=1",
		},
		{
			name: "dentry metadata fallback with text",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("dentry-text", "use context")
				m.Msgtype = "file"
				m.Content = map[string]any{"dentryId": float64(1), "spaceId": float64(2), "fileName": "data.csv"}
				return m
			}(),
			media: &fakeConnectMedia{unionErr: errors.New("union")}, want: "用户同时附了一个文件",
		},
		{
			name: "dentry success with text",
			message: func() *chatbot.BotCallbackDataModel {
				m := connectorMessage("dentry-success-text", "use context")
				m.Msgtype = "file"
				m.Content = map[string]any{"dentryId": float64(1), "spaceId": float64(2), "fileName": "data.csv"}
				return m
			}(),
			media: &fakeConnectMedia{unionID: "union", dentryPath: "/tmp/data.csv"}, want: "用户同时附了一个文件",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fwd := &basicConnectorForwarder{reply: "done"}
			runConnectorScenario(t, []*chatbot.BotCallbackDataModel{tc.message}, fwd, tc.media, nil, nil, 1)
			prompts := fwd.promptSnapshot()
			if len(prompts) != 1 || !strings.Contains(prompts[0], tc.want) {
				t.Fatalf("media prompt = %#v, want %q", prompts, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageRunStreamConnectorCardEdges(t *testing.T) {
	t.Run("card client without template", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		cardCli := newAICardClient("client", "secret", "")
		fwd := &basicConnectorForwarder{reply: "plain"}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("no-template", "question")}, fwd, nil, cardCli, nil, nil, 1)
		if len(rec.calls) == 0 {
			t.Fatal("card client did not mark thinking")
		}
	})

	t.Run("reaction and card creation failures fall back", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		rec.fail["POST /v1.0/robot/emotion/reply"] = 500
		rec.fail["POST /v1.0/card/instances"] = 500
		cardCli := newAICardClient("client", "secret", defaultAICardTemplateID)
		fwd := &basicConnectorForwarder{reply: "plain"}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("card-create-fail", "question")}, fwd, nil, cardCli, nil, nil, 1)
	})

	t.Run("stream card frame and finish failures", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		rec.fail["PUT /v1.0/card/streaming"] = 500
		cardCli := newAICardClient("client", "secret", defaultAICardTemplateID)
		fwd := &streamingConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "streamed"}}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("card-stream-fail", "question")}, fwd, nil, cardCli, nil, nil, 1)
		if !fwd.streamed {
			t.Fatal("streaming card forwarder was not called")
		}
	})

	t.Run("stream card precreate failure", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		rec.fail["POST /v1.0/card/instances"] = 500
		cardCli := newAICardClient("client", "secret", defaultAICardTemplateID)
		fwd := &streamingConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "streamed"}}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("card-precreate-fail", "question")}, fwd, nil, cardCli, nil, nil, 1)
	})

	t.Run("stream delta status failure", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		rec.fail["PUT /v1.0/card/instances"] = 500
		cardCli := newAICardClient("client", "secret", defaultAICardTemplateID)
		fwd := &streamingConnectorForwarder{basicConnectorForwarder: &basicConnectorForwarder{reply: "streamed"}}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("card-delta-fail", "question")}, fwd, nil, cardCli, nil, nil, 1)
	})

	t.Run("successful one-shot card", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		cardCli := newAICardClient("client", "secret", defaultAICardTemplateID)
		fwd := &basicConnectorForwarder{reply: "card answer"}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("card-ok", "question")}, fwd, nil, cardCli, nil, nil, 0)
		rec.mu.Lock()
		calls := strings.Join(rec.calls, ",")
		rec.mu.Unlock()
		if !strings.Contains(calls, "PUT /v1.0/card/streaming") {
			t.Fatalf("card calls = %s", calls)
		}
	})

	t.Run("thinking remains when every fallback reply fails", func(t *testing.T) {
		rec, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		rec.fail["POST /v1.0/card/instances"] = 500
		cardCli := newAICardClient("client", "secret", defaultAICardTemplateID)
		fwd := &basicConnectorForwarder{reply: "plain"}
		errs := []error{errors.New("1"), errors.New("2"), errors.New("3"), errors.New("4")}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("card-send-fail", "question")}, fwd, nil, cardCli, nil, errs, 4)
	})

	t.Run("handled approval swaps thinking", func(t *testing.T) {
		_, srv := newCardAPIServer(t)
		withCardAPIBase(t, srv.URL)
		cardCli := newAICardClient("client", "secret", "")
		approval := newTextApprovalOrchestrator(newApprovalGate(""), nil, "owner", fakeOwnerNotifier{})
		fwd := &basicConnectorForwarder{reply: `准备创建 [[ACTION:doc.create name="Plan"]]`}
		runConnectorScenarioWithCard(t, []*chatbot.BotCallbackDataModel{connectorMessage("approval-handled", "create doc")}, fwd, nil, cardCli, &connectExtras{approval: approval}, nil, 0)
	})
}

package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/gorilla/websocket"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/handler"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
)

var errSourceInjected = errors.New("injected source failure")

type fakeStreamClient struct {
	handler  handler.IFrameHandler
	startErr error
	closed   bool
}

func (c *fakeStreamClient) RegisterAllEventRouter(h handler.IFrameHandler) { c.handler = h }
func (c *fakeStreamClient) Start(context.Context) error                    { return c.startErr }
func (c *fakeStreamClient) Close()                                         { c.closed = true }

func TestCrossPlatformCoverageDingtalkStartEdges(t *testing.T) {
	s, err := New(Config{ClientID: "id", ClientSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	s.cli = &fakeStreamClient{}
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("second Start error expected")
	}
}

func TestCrossPlatformCoverageDingtalkStartInjected(t *testing.T) {
	origNew := newStreamClient
	t.Cleanup(func() { newStreamClient = origNew })
	_ = origNew()
	failing := &fakeStreamClient{startErr: errSourceInjected}
	newStreamClient = func(_ ...client.ClientOption) streamClient { return failing }
	s, _ := New(Config{ClientID: "id", ClientSecret: "secret"})
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); !errors.Is(err, errSourceInjected) {
		t.Fatalf("Start failure = %v", err)
	}
	if s.State().State != StateStopped || failing.handler == nil {
		t.Fatal("failed start state/handler")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	success := &fakeStreamClient{}
	newStreamClient = func(_ ...client.ClientOption) streamClient { return success }
	s, _ = New(Config{ClientID: "id", ClientSecret: "secret"})
	if err := s.Start(ctx, func(*dwsevent.RawEvent) {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start cancel = %v", err)
	}
	if !success.closed {
		t.Fatal("stream client not closed")
	}
}

func staticPersonalTicketClient(endpoint, body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if body == "" {
			body = `{"endpoint":` + strconvQuote(endpoint) + `,"ticket":"t"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestCrossPlatformCoveragePersonalRunAttemptAndFrameFailures(t *testing.T) {
	base := PersonalConfig{
		AccessToken: "token", ClientID: "id", SourceID: "source", TicketURL: "https://ticket",
		ReconnectMin: time.Millisecond, ReconnectMax: time.Millisecond,
	}
	base.HTTPClient = staticPersonalTicketClient("http://bad", "")
	s, _ := NewPersonal(base)
	if _, err := s.runAttempt(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("endpoint error expected")
	}
	base.HTTPClient = staticPersonalTicketClient("ws://127.0.0.1:1", "")
	s, _ = NewPersonal(base)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := s.runAttempt(ctx, func(*dwsevent.RawEvent) {}); !isRetryablePersonalError(err) {
		t.Fatalf("dial error = %v", err)
	}
	base.HTTPClient = staticPersonalTicketClient("wss://x", `{`)
	s, _ = NewPersonal(base)
	if _, err := s.fetchTicket(context.Background()); err == nil {
		t.Fatal("ticket decode error expected")
	}

	if err := s.handleFrame(nil, []byte("invalid"), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("dataframe decode error expected")
	}
	upgrader := websocket.Upgrader{}
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			serverConn <- conn
		}
	}))
	defer srv.Close()
	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	peer := <-serverConn
	_ = clientConn.UnderlyingConn().Close()
	dfRaw, _ := json.Marshal(payload.DataFrame{Type: "event", Data: `{}`})
	if err := s.handleFrame(clientConn, dfRaw, func(*dwsevent.RawEvent) {}); !isRetryablePersonalError(err) {
		t.Fatalf("ack write error = %v", err)
	}
	_ = peer.Close()

	wsEndpoint := ""
	mux := http.NewServeMux()
	mux.HandleFunc("/ticket", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"endpoint":`+strconvQuote(wsEndpoint)+`,"ticket":"t"}`)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("ignored"))
	})
	wsSrv := httptest.NewServer(mux)
	defer wsSrv.Close()
	wsEndpoint = "ws" + strings.TrimPrefix(wsSrv.URL, "http") + "/ws"
	base.HTTPClient = wsSrv.Client()
	base.TicketURL = wsSrv.URL + "/ticket"
	s, _ = NewPersonal(base)
	if _, err := s.runAttempt(context.Background(), func(*dwsevent.RawEvent) {}); !isRetryablePersonalError(err) {
		t.Fatalf("binary/read error = %v", err)
	}
	textSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("invalid"))
	}))
	defer textSrv.Close()
	base.HTTPClient = staticPersonalTicketClient("ws"+strings.TrimPrefix(textSrv.URL, "http"), "")
	base.TicketURL = "https://ticket"
	s, _ = NewPersonal(base)
	if _, err := s.runAttempt(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("text dataframe error expected")
	}
}

func TestCrossPlatformCoverageNewAndPersonalValidationEdges(t *testing.T) {
	invalidPortal := &PortalTicketConfig{}
	if _, err := New(Config{PortalTicket: invalidPortal}); err == nil {
		t.Fatal("invalid portal expected")
	}
	validCustom := &PortalTicketConfig{
		TicketURL: "https://ticket", AccessToken: "token", SourceID: "source",
		Mode: PortalTicketModeCustom, ClientID: "portal-id", ClientSecret: "portal-secret",
	}
	if _, err := New(Config{PortalTicket: validCustom}); err == nil {
		t.Fatal("custom ClientID expected")
	}

	base := PersonalConfig{AccessToken: "token", ClientID: "id", SourceID: "source", TicketURL: "https://ticket"}
	for name, mutate := range map[string]func(*PersonalConfig){
		"token":         func(c *PersonalConfig) { c.AccessToken = "" },
		"client":        func(c *PersonalConfig) { c.ClientID = "" },
		"source":        func(c *PersonalConfig) { c.SourceID = "" },
		"ticket":        func(c *PersonalConfig) { c.TicketURL = "" },
		"custom-secret": func(c *PersonalConfig) { c.TicketMode = "custom" },
	} {
		cfg := base
		mutate(&cfg)
		if _, err := NewPersonal(cfg); err == nil {
			t.Fatalf("%s validation expected", name)
		}
	}
	base.ReconnectMin = 5 * time.Second
	base.ReconnectMax = time.Second
	s, err := NewPersonal(base)
	if err != nil {
		t.Fatal(err)
	}
	if s.cfg.HTTPClient == nil || s.cfg.WebSocketDialer == nil || s.cfg.Now == nil || s.cfg.ReconnectMax != s.cfg.ReconnectMin {
		t.Fatalf("personal defaults = %#v", s.cfg)
	}
	if err := s.Start(context.Background(), nil); err == nil {
		t.Fatal("nil emit expected")
	}
	s.started.Store(true)
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("second Start expected")
	}
	re := retryPersonal(errSourceInjected)
	if !errors.Is(re, errSourceInjected) || re.(*retryablePersonalError).Unwrap() != errSourceInjected {
		t.Fatal("retryable error contract")
	}
	if retryPersonal(nil) != nil {
		t.Fatal("retryPersonal(nil)")
	}
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) { return 0, errSourceInjected }
func (errorReadCloser) Close() error             { return nil }

func TestCrossPlatformCoveragePersonalTicketAndHelperEdges(t *testing.T) {
	clientWith := func(fn func(*http.Request) (*http.Response, error)) *http.Client {
		return &http.Client{Transport: roundTripFunc(fn)}
	}
	base := PersonalConfig{
		AccessToken: "token", ClientID: "id", ClientSecret: "secret", SourceID: "source",
		TicketURL: "https://ticket", TicketMode: "custom",
	}
	base.HTTPClient = clientWith(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: errorReadCloser{}}, nil
	})
	s, _ := NewPersonal(base)
	if _, err := s.fetchTicket(context.Background()); !isRetryablePersonalError(err) {
		t.Fatalf("read error = %v", err)
	}
	base.TicketURL = ":"
	base.HTTPClient = http.DefaultClient
	s, _ = NewPersonal(base)
	if _, err := s.fetchTicket(context.Background()); err == nil {
		t.Fatal("request creation error expected")
	}

	for _, tc := range []struct {
		data    string
		wantErr bool
	}{
		{`{"result":{"endpoint":"wss://x","ticket":"t"}}`, false},
		{`{"result":null,"data":{"endpoint":"wss://x","ticket":"t"}}`, false},
		{`{"result":"bad"}`, true},
		{`{"endpoint":"wss://x","ticket":"t"}`, false},
		{`{`, true},
	} {
		_, err := decodeTicket([]byte(tc.data))
		if (err != nil) != tc.wantErr {
			t.Fatalf("decodeTicket(%q) = %v", tc.data, err)
		}
	}
	for _, endpoint := range []string{"%", "http://host", "ws://", "ws://host/path?ticket=old", "wss://host/path"} {
		_, _ = endpointWithTicket(endpoint, "new")
	}
	for _, message := range []string{
		"ticket HTTP 500", "fetch ticket x", "read ticket response x", "dial websocket x",
		"read websocket x", "write ack x", "something else",
	} {
		_ = personalRetryLogError(retryPersonal(errors.New(message)))
	}
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, 500, 400} {
		_ = retryableTicketStatus(status)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitPersonalReconnect(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := waitPersonalReconnect(context.Background(), time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	_ = nextPersonalBackoff(30*time.Second, 30*time.Second)
	_ = nextPersonalBackoff(20*time.Second, 30*time.Second)

	if headerValue(nil, "x") != "" {
		t.Fatal("nil header")
	}
	h := payload.DataFrameHeader{"Mixed": " value "}
	if headerValue(h, "mixed") == "" || headerAny(h, "none", "mixed") == "" {
		t.Fatal("case-insensitive header")
	}
	for _, raw := range []string{"", "not-json", `[]`, `"not-json"`, `"{\"Name\":\" value \"}"`} {
		_ = parsePersonalDataFields(raw)
	}
	f := personalDataFields{"Name": " value ", "bad": 1, "nested": map[string]any{"Value": " ok "}}
	if f.string("name") != "value" || f.string("bad") != "" || f.nestedString([]string{"NESTED", "value"}) != "ok" {
		t.Fatalf("personal fields = %#v", f)
	}
	_ = personalDataFields(nil).string("x")
	_ = personalDataFields(nil).nestedString([]string{"x"})
	for _, path := range [][]string{{"missing"}, {"bad", "x"}, {"nested"}, {"nested", "missing"}} {
		_ = nestedStringCI(map[string]any(f), path...)
	}
	_ = firstNonEmpty("", " value ")
	_ = firstInt64(0, 2)
	_ = firstInt64(0)

	logPersonalDataFrame(nil, "")
	if redactPersonalRawStringMap(nil) != nil {
		t.Fatal("nil redaction")
	}
	_ = redactPersonalRawStringMap(map[string]string{"token": "secret", "safe": "value"})
	for _, data := range [][]byte{nil, []byte("not-json"), []byte(`{"token":"x","nested":[{"safe":1}]}`)} {
		_ = sanitizePersonalRawPayload(data)
	}
	_ = redactPersonalRawJSONValue("scalar")
	if _, err := marshalPersonalRawJSON(make(chan int)); err == nil {
		t.Fatal("marshal error expected")
	}
	_ = personalRawSensitiveKey(" Authorization ")
	_ = truncatePersonalRawLog(strings.Repeat("x", personalRawDebugPayloadLimit+1))

	var nilSource *PersonalSource
	raw := nilSource.rawEventFromDataFrame(&payload.DataFrame{Data: `{}`})
	if raw.SourceID != "" || raw.ReceivedAt.IsZero() {
		t.Fatalf("nil source event = %#v", raw)
	}
}

func TestCrossPlatformCoveragePortalConfigAndRequestEdges(t *testing.T) {
	for name, cfg := range map[string]*PortalTicketConfig{
		"nil":    nil,
		"url":    {AccessToken: "t", SourceID: "s"},
		"token":  {TicketURL: "https://x", SourceID: "s"},
		"source": {TicketURL: "https://x", AccessToken: "t"},
		"mode":   {TicketURL: "https://x", AccessToken: "t", SourceID: "s", Mode: "bad"},
		"custom": {TicketURL: "https://x", AccessToken: "t", SourceID: "s", Mode: "custom"},
	} {
		if err := cfg.Valid(); err == nil {
			t.Fatalf("%s validation expected", name)
		}
	}
	valid := &PortalTicketConfig{TicketURL: "https://x", AccessToken: "t", SourceID: "s", Mode: " CUSTOM ", ClientID: "id", ClientSecret: "secret"}
	if err := valid.Valid(); err != nil || valid.normalizedMode() != PortalTicketModeCustom {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "normal", "custom", "bad"} {
		_ = normalizePortalTicketMode(mode)
	}

	if _, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL: ":", AccessToken: "t", SourceID: "s",
	}); err == nil {
		t.Fatal("request URL error expected")
	}
	network := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errSourceInjected })}
	if _, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
		TicketURL: "https://x", AccessToken: "t", SourceID: "s", HTTPClient: network,
	}); err == nil {
		t.Fatal("network error expected")
	}

	for name, response := range map[string]struct {
		status  int
		body    string
		wantErr bool
	}{
		"http":     {500, "failure", true},
		"direct":   {200, `{"endpoint":"wss://x","ticket":"t"}`, false},
		"parse":    {200, `{`, true},
		"failed":   {200, `{"success":false,"errorCode":"E","errorMsg":"bad"}`, true},
		"missing":  {200, `{"success":true,"result":{}}`, true},
		"envelope": {200, `{"success":true,"result":{"endpoint":"wss://x","ticket":"t"}}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if name == "direct" && req.Header.Get("User-Agent") != "agent" {
					t.Fatal("user agent missing")
				}
				return &http.Response{StatusCode: response.status, Body: io.NopCloser(strings.NewReader(response.body)), Header: make(http.Header)}, nil
			})}
			_, err := requestPortalTicket(context.Background(), &PortalTicketConfig{
				TicketURL: "https://x", AccessToken: "t", SourceID: "s", UserAgent: "agent", HTTPClient: client,
			})
			if (err != nil) != response.wantErr {
				t.Fatalf("request error = %v", err)
			}
		})
	}
	for _, ticket := range []portalStreamTicket{{Endpoint: "%", Ticket: "t"}, {Endpoint: "wss://host/path?x=1", Ticket: "t"}} {
		_, _ = websocketURL(ticket)
	}
	resp := payload.NewSuccessDataFrameResponse()
	df := &payload.DataFrame{Headers: payload.DataFrameHeader{payload.DataFrameHeaderKMessageId: "msg"}}
	ensurePortalAckHeaders(resp, df)
	ensurePortalAckHeaders(resp, df)
	ctx, cancel := context.WithCancel(context.Background())
	if isContextDone(ctx) {
		t.Fatal("fresh context done")
	}
	cancel()
	if !isContextDone(ctx) {
		t.Fatal("canceled context not done")
	}
	_ = truncatePortalTicketLog(" short ", 10)
	_ = truncatePortalTicketLog(" long value ", 4)
}

func TestCrossPlatformCoveragePortalStartEndToEndAndFailures(t *testing.T) {
	makeSource := func(cfg *PortalTicketConfig) *DingtalkSource {
		s, err := New(Config{PortalTicket: cfg})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	network := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errSourceInjected })}
	s := makeSource(&PortalTicketConfig{TicketURL: "https://x", AccessToken: "t", SourceID: "s", HTTPClient: network, DisableReconnect: true})
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("ticket failure expected")
	}

	returnTicket := func(endpoint string) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			body := `{"endpoint":` + strconvQuote(endpoint) + `,"ticket":"t"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
	}
	s = makeSource(&PortalTicketConfig{TicketURL: "https://x", AccessToken: "t", SourceID: "s", HTTPClient: returnTicket("%")})
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("websocket URL error expected")
	}
	s = makeSource(&PortalTicketConfig{TicketURL: "https://x", AccessToken: "t", SourceID: "s", HTTPClient: returnTicket("ws://127.0.0.1:1")})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := s.Start(ctx, func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("dial error expected")
	}

	upgrader := websocket.Upgrader{}
	var wsURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/ticket", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"endpoint":`+strconvQuote(wsURL)+`,"ticket":"ticket"}`)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("invalid"))
		df := payload.DataFrame{Type: "event", Headers: payload.DataFrameHeader{payload.DataFrameHeaderKMessageId: "msg"}, Data: `{}`}
		_ = conn.WriteJSON(df)
		_, _, _ = conn.ReadMessage()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	s = makeSource(&PortalTicketConfig{TicketURL: srv.URL + "/ticket", AccessToken: "t", SourceID: "s", HTTPClient: srv.Client()})
	ctx, cancel = context.WithCancel(context.Background())
	emitted := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx, func(*dwsevent.RawEvent) { emitted <- struct{}{} }) }()
	select {
	case <-emitted:
	case <-time.After(time.Second):
		t.Fatal("portal event timeout")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("portal stop = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("portal stop timeout")
	}
}

func TestCrossPlatformCoveragePortalStartHandshakeReadAndAckErrors(t *testing.T) {
	origWriteMessage := portalWriteMessage
	t.Cleanup(func() { portalWriteMessage = origWriteMessage })
	makeSource := func(endpoint string) *DingtalkSource {
		s, err := New(Config{PortalTicket: &PortalTicketConfig{
			TicketURL: "https://ticket", AccessToken: "token", SourceID: "source",
			HTTPClient: staticPersonalTicketClient(endpoint, ""), DisableReconnect: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "forbidden")
	}))
	defer forbidden.Close()
	endpoint := "ws" + strings.TrimPrefix(forbidden.URL, "http")
	s := makeSource(endpoint)
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("handshake error = %v", err)
	}

	upgrader := websocket.Upgrader{}
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer closedSrv.Close()
	endpoint = "ws" + strings.TrimPrefix(closedSrv.URL, "http")
	s = makeSource(endpoint)
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil || !strings.Contains(err.Error(), "stream read") {
		t.Fatalf("read error = %v", err)
	}

	// Keep the websocket open with no frames so cancellation can only unblock
	// the client's ReadMessage call. This deterministically covers the
	// context-cancelled read path instead of racing cancellation against ACK.
	readReady := make(chan struct{})
	releaseRead := make(chan struct{})
	cancelReadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		close(readReady)
		<-releaseRead
		_ = conn.Close()
	}))
	defer cancelReadSrv.Close()
	endpoint = "ws" + strings.TrimPrefix(cancelReadSrv.URL, "http")
	s = makeSource(endpoint)
	readCtx, cancelRead := context.WithCancel(context.Background())
	readErr := make(chan error, 1)
	go func() { readErr <- s.Start(readCtx, func(*dwsevent.RawEvent) {}) }()
	select {
	case <-readReady:
	case <-time.After(time.Second):
		close(releaseRead)
		t.Fatal("cancelled read connection timeout")
	}
	cancelRead()
	select {
	case err := <-readErr:
		close(releaseRead)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled read = %v", err)
		}
	case <-time.After(time.Second):
		close(releaseRead)
		t.Fatal("cancelled read timeout")
	}

	frame := payload.DataFrame{Type: "event", Headers: payload.DataFrameHeader{payload.DataFrameHeaderKMessageId: "m"}, Data: `{}`}
	abruptSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.WriteJSON(frame)
		_ = conn.UnderlyingConn().Close()
	}))
	defer abruptSrv.Close()
	endpoint = "ws" + strings.TrimPrefix(abruptSrv.URL, "http")
	s = makeSource(endpoint)
	portalWriteMessage = func(*websocket.Conn, int, []byte) error { return errSourceInjected }
	if err := s.Start(context.Background(), func(*dwsevent.RawEvent) {}); err == nil {
		t.Fatal("abrupt ack/read error expected")
	}

	cancelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(frame)
		time.Sleep(100 * time.Millisecond)
	}))
	defer cancelSrv.Close()
	endpoint = "ws" + strings.TrimPrefix(cancelSrv.URL, "http")
	s = makeSource(endpoint)
	ctx, cancel := context.WithCancel(context.Background())
	portalWriteMessage = func(*websocket.Conn, int, []byte) error {
		cancel()
		return errSourceInjected
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start(ctx, func(*dwsevent.RawEvent) {}) }()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ack cancellation = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ack cancellation timeout")
	}
}

func strconvQuote(s string) string {
	var b bytes.Buffer
	_, _ = io.WriteString(&b, `"`+strings.ReplaceAll(s, `"`, `\"`)+`"`)
	return b.String()
}

package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type edgeWriteCloser struct {
	bytes.Buffer
	err       error
	closeErr  error
	closeCall int
}

func (w *edgeWriteCloser) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func (w *edgeWriteCloser) Close() error {
	w.closeCall++
	return w.closeErr
}

type edgeReadCloser struct {
	io.Reader
	closeErr error
}

func (r edgeReadCloser) Close() error { return r.closeErr }

type edgeErrorReader struct{}

func (edgeErrorReader) Read([]byte) (int, error) { return 0, errors.New("read") }

func edgeResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func edgeClient(fn roundTripFunc) *Client {
	c := NewClient(&http.Client{Transport: fn})
	c.MaxRetries = 0
	c.RetryDelay = time.Millisecond
	c.RetryMaxDelay = 2 * time.Millisecond
	return c
}

func TestCrossPlatformCoverageCallErrorAndRedirectEdges(t *testing.T) {
	httpClient := &http.Client{}
	if got := NewClient(httpClient); got.HTTPClient.Transport == nil || got.HTTPClient.CheckRedirect == nil {
		t.Fatalf("NewClient did not fill HTTP defaults: %#v", got.HTTPClient)
	}
	var nilErr *CallError
	if nilErr.Error() != "" || nilErr.Unwrap() != nil {
		t.Fatal("nil call error did not stay empty")
	}
	cause := errors.New("cause")
	if got := (&CallError{Cause: cause}).Error(); got != "cause" {
		t.Fatalf("cause error = %q", got)
	}
	if !errors.Is(&CallError{Cause: cause}, cause) {
		t.Fatal("call error did not unwrap")
	}
	for _, tc := range []struct {
		err  *CallError
		want string
	}{
		{&CallError{Stage: CallStageHTTP, HTTPStatus: 503}, "http failure: http 503"},
		{&CallError{Stage: CallStageJSONRPC, RPCCode: -32601}, "jsonrpc failure: rpc -32601"},
		{&CallError{Stage: CallStageRequest}, "request failure"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Fatalf("Error() = %q, want %q", got, tc.want)
		}
	}

	request := func(raw string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "secret")
		req.Header.Set("x-user-access-token", "secret")
		return req
	}
	if err := safeRedirectPolicy(request("https://one.test"), nil); err != nil {
		t.Fatal(err)
	}
	same := request("https://one.test/next")
	if err := safeRedirectPolicy(same, []*http.Request{request("https://one.test/start")}); err != nil || same.Header.Get("Authorization") == "" {
		t.Fatalf("same-host redirect = %v, %#v", err, same.Header)
	}
	cross := request("https://two.test")
	if err := safeRedirectPolicy(cross, []*http.Request{request("https://one.test")}); err != nil || cross.Header.Get("Authorization") != "" || cross.Header.Get("x-user-access-token") != "" {
		t.Fatalf("cross-host redirect = %v, %#v", err, cross.Header)
	}
	via := make([]*http.Request, 10)
	if err := safeRedirectPolicy(request("https://one.test"), via); err == nil {
		t.Fatal("redirect limit succeeded")
	}

	base := NewClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return edgeResponse(http.StatusOK, "{}"), nil
	})})
	base.AuthToken = "token"
	base.ExtraHeaders = map[string]string{"X": "Y"}
	copy := base.WithExecutionId("exec-1")
	if copy == base || copy.ExecutionId != "exec-1" || copy.AuthToken != "token" || copy.ExtraHeaders["X"] != "Y" {
		t.Fatalf("WithExecutionId = %#v", copy)
	}
}

func TestCrossPlatformCoverageToolCallResultUnmarshalEdges(t *testing.T) {
	for _, raw := range []string{"[]", `{"content":42}`} {
		var result ToolCallResult
		if err := json.Unmarshal([]byte(raw), &result); err == nil {
			t.Fatalf("invalid result %q succeeded", raw)
		}
	}
	checks := []struct {
		raw   string
		check func(ToolCallResult) bool
	}{
		{`{"structuredContent":{"x":1},"isError":true}`, func(r ToolCallResult) bool {
			return r.IsError && r.Content["x"] != nil && r.StructuredContent["x"] != nil
		}},
		{`{}`, func(r ToolCallResult) bool { return r.Content == nil && r.Blocks == nil }},
		{`{"content":{"x":1}}`, func(r ToolCallResult) bool { return r.Content["x"] != nil }},
		{`{"content":[{"type":"text","text":"ignored"}],"structuredContent":{"x":1}}`, func(r ToolCallResult) bool { return len(r.Blocks) == 1 && r.Content["x"] != nil }},
		{`{"content":[{"type":"text","text":" "},{"type":"text","text":"bad"},{"type":"text","text":"{\"x\":1}"}]}`, func(r ToolCallResult) bool { return r.Content["x"] != nil }},
		{`{"content":[{"type":"text","text":"plain"}]}`, func(r ToolCallResult) bool { return len(r.Blocks) == 1 && r.Content == nil }},
	}
	for _, tc := range checks {
		var result ToolCallResult
		if err := json.Unmarshal([]byte(tc.raw), &result); err != nil || !tc.check(result) {
			t.Fatalf("UnmarshalJSON(%s) = %#v, %v", tc.raw, result, err)
		}
	}
}

func TestCrossPlatformCoverageClientProtocolAndValidationEdges(t *testing.T) {
	original := supportedProtocolVersions
	supportedProtocolVersions = nil
	if _, err := edgeClient(func(*http.Request) (*http.Response, error) { return edgeResponse(http.StatusOK, "{}"), nil }).Initialize(context.Background(), "https://x.test"); err == nil {
		t.Fatal("empty protocol versions succeeded")
	}
	supportedProtocolVersions = original
	t.Cleanup(func() { supportedProtocolVersions = original })

	rpcFailure := edgeClient(func(*http.Request) (*http.Response, error) {
		return edgeResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"unsupported"}}`), nil
	})
	if _, err := rpcFailure.Initialize(context.Background(), "https://x.test"); err == nil {
		t.Fatal("all-protocol failure succeeded")
	}

	c := edgeClient(func(*http.Request) (*http.Response, error) {
		return edgeResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`), nil
	})
	got, err := c.Initialize(context.Background(), "https://x.test")
	if err != nil || got.ProtocolVersion == "" || got.RequestedProtocolVersion == "" {
		t.Fatalf("Initialize fallback version = %#v, %v", got, err)
	}

	for _, args := range []map[string]any{
		{"nested": map[string]any{"bad": "x\x00"}},
		{"list": []any{"ok", "x\x00"}},
		{"list": []any{map[string]any{"bad": "x\x00"}}},
	} {
		if err := validateCallArguments(args); err == nil {
			t.Fatalf("invalid args %#v succeeded", args)
		}
	}
	if err := validateCallArguments(map[string]any{"ok": []any{1, map[string]any{"v": "safe"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CallTool(context.Background(), "https://x.test", "bad\x00name", nil); err == nil {
		t.Fatal("control-character tool succeeded")
	}
	if _, err := c.CallTool(context.Background(), "https://x.test", "ok", map[string]any{"bad": "x\x00"}); err == nil {
		t.Fatal("control-character argument succeeded")
	}
}

func TestCrossPlatformCoverageCallJSONRPCEdges(t *testing.T) {
	c := edgeClient(func(*http.Request) (*http.Response, error) { return edgeResponse(http.StatusOK, "{}"), nil })
	if err := c.callJSONRPC(context.Background(), "https://x.test", requestEnvelope{Method: "x", Params: func() {}}, true, nil); err == nil {
		t.Fatal("unencodable request succeeded")
	}

	c = edgeClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Trace-Id": {"trace"}}, Body: edgeReadCloser{Reader: edgeErrorReader{}}}, nil
	})
	if err := c.callJSONRPC(context.Background(), "https://x.test", requestEnvelope{Method: "tools/list"}, true, nil); err == nil {
		t.Fatal("response read error succeeded")
	}

	for _, tc := range []struct {
		body string
		out  any
	}{
		{"not-json", &map[string]any{}},
		{`{"jsonrpc":"2.0","id":1}`, &map[string]any{}},
		{`{"jsonrpc":"2.0","id":1,"result":"bad"}`, &map[string]any{}},
	} {
		c = edgeClient(func(*http.Request) (*http.Response, error) { return edgeResponse(http.StatusOK, tc.body), nil })
		if err := c.callJSONRPC(context.Background(), "https://x.test", requestEnvelope{Method: "tools/list"}, true, tc.out); err == nil {
			t.Fatalf("bad response %q succeeded", tc.body)
		}
	}

	c = edgeClient(func(*http.Request) (*http.Response, error) {
		return edgeResponse(http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`), nil
	})
	if err := c.callJSONRPC(context.Background(), "https://x.test", requestEnvelope{Method: "x"}, true, nil); err != nil {
		t.Fatalf("nil output: %v", err)
	}
	if err := c.callJSONRPC(context.Background(), "https://x.test", requestEnvelope{Method: "notify"}, false, nil); err != nil {
		t.Fatalf("notification: %v", err)
	}
}

func TestCrossPlatformCoverageRetryAndFailureEdges(t *testing.T) {
	if _, err := edgeClient(func(*http.Request) (*http.Response, error) { return nil, nil }).doWithRetry(context.Background(), "://", nil, ""); err == nil {
		t.Fatal("invalid endpoint succeeded")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	seen := false
	c := edgeClient(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get(HeaderExecutionId) == "exec" && req.Header.Get("X-Test") == "value" && req.Header.Get("Authorization") != ""
		return edgeResponse(http.StatusOK, "{}"), nil
	})
	c.ExecutionId = "exec"
	c.AuthToken = " token "
	c.ExtraHeaders = map[string]string{"": "ignored", "X-Empty": "", "X-Test": "value"}
	c.TrustedDomains = []string{"x.test"}
	c.FileLogger = logger
	if resp, err := c.doWithRetry(context.Background(), "https://x.test", nil, " "); err != nil || resp == nil || !seen {
		t.Fatalf("header request = %#v, %v, seen=%v", resp, err, seen)
	}

	closeCalls := 0
	attempts := 0
	c = edgeClient(func(*http.Request) (*http.Response, error) {
		attempts++
		resp := edgeResponse(http.StatusServiceUnavailable, "busy")
		resp.Header.Set("Retry-After", "1")
		resp.Body = edgeReadCloser{Reader: strings.NewReader("busy"), closeErr: nil}
		if attempts == 2 {
			return edgeResponse(http.StatusOK, "{}"), nil
		}
		return resp, nil
	})
	c.MaxRetries = 1
	c.RetryMaxDelay = time.Millisecond
	c.sleep = func(context.Context, time.Duration) error { closeCalls++; return nil }
	if resp, err := c.doWithRetry(context.Background(), "https://x.test", nil, "tools/list"); err != nil || resp.StatusCode != http.StatusOK || attempts != 2 || closeCalls != 1 {
		t.Fatalf("retry response = %#v, %v attempts=%d sleeps=%d", resp, err, attempts, closeCalls)
	}

	for _, method := range []string{"tools/call", "tools/list"} {
		c = edgeClient(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
		c.MaxRetries = 1
		c.sleep = func(context.Context, time.Duration) error { return context.Canceled }
		if _, err := c.doWithRetry(context.Background(), "https://x.test", nil, method); err == nil {
			t.Fatalf("cancelled retry for %s succeeded", method)
		}
	}

	for _, method := range []string{"tools/call", "tools/list"} {
		c = edgeClient(func(*http.Request) (*http.Response, error) { return nil, errors.New("no such host") })
		if _, err := c.doWithRetry(context.Background(), "https://x.test", nil, method); err == nil {
			t.Fatalf("request failure for %s succeeded", method)
		}
	}
	c = edgeClient(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	c.MaxRetries = 3
	if _, err := c.doWithRetry(context.Background(), "https://x.test", nil, "tools/list"); err == nil {
		t.Fatal("timeout request succeeded")
	}
}

func TestCrossPlatformCoverageRetryHelperAndTrustEdges(t *testing.T) {
	for _, err := range []error{nil, context.DeadlineExceeded, context.Canceled, os.ErrDeadlineExceeded, errors.New("Client.Timeout exceeded"), errors.New("TLS handshake timeout"), errors.New("other")} {
		_ = isTimeoutError(err)
		_, _ = classifyRequestFailure(err)
	}
	for _, err := range []error{errors.New("connection refused"), errors.New("no such host"), errors.New("i/o timeout")} {
		reason, _ := classifyRequestFailure(err)
		if reason == "request_failed" {
			t.Fatalf("unclassified error %v", err)
		}
	}
	if respRetryAfter(nil) != "" {
		t.Fatal("nil retry-after was not empty")
	}
	resp := edgeResponse(http.StatusTooManyRequests, "")
	resp.Header.Set("Retry-After", " 2 ")
	if respRetryAfter(resp) != "2" {
		t.Fatal("retry-after was not trimmed")
	}
	c := &Client{}
	if got := c.retryDelayForAttempt(0, "1"); got != 80*time.Millisecond {
		// With defaults derived from an empty client, Retry-After is capped.
		if got != 80*time.Millisecond {
			t.Fatalf("retry delay = %v", got)
		}
	}
	c.RetryDelay = time.Second
	c.RetryMaxDelay = 2 * time.Second
	if got := c.retryDelayForAttempt(5, ""); got != 2*time.Second {
		t.Fatalf("capped delay = %v", got)
	}
	if got := c.retryDelayForAttempt(0, "0"); got != time.Second {
		t.Fatalf("zero retry delay = %v", got)
	}
	if _, ok := parseRetryAfter("bad"); ok {
		t.Fatal("bad retry-after parsed")
	}
	if got, ok := parseRetryAfter(time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)); !ok || got != 0 {
		t.Fatalf("past retry-after = %v, %v", got, ok)
	}
	if got, ok := parseRetryAfter(time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)); !ok || got <= 0 {
		t.Fatalf("future retry-after = %v, %v", got, ok)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Client{}).sleepForRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("sleep cancellation = %v", err)
	}
	if err := (&Client{}).sleepForRetry(context.Background(), time.Nanosecond); err != nil {
		t.Fatalf("short sleep = %v", err)
	}

	for _, endpoint := range []string{"%", "ftp://x.test", "http://remote.test"} {
		t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
		if (&Client{TrustedDomains: []string{"*"}}).isEndpointTrusted(endpoint) {
			t.Fatalf("endpoint %q trusted", endpoint)
		}
	}
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "")
	if (&Client{TrustedDomains: []string{"*"}}).isEndpointTrusted("http://localhost") {
		t.Fatal("HTTP endpoint trusted without opt-in")
	}
	t.Setenv("DWS_ALLOW_HTTP_ENDPOINTS", "1")
	if !(&Client{TrustedDomains: []string{"localhost"}}).isEndpointTrusted("http://localhost") {
		t.Fatal("loopback endpoint not trusted")
	}
	var warning bytes.Buffer
	wild := &Client{TrustedDomains: []string{"*"}, Stderr: &warning}
	if !wild.isEndpointTrusted("https://anything.test") || !wild.isEndpointTrusted("https://anything.test") || strings.Count(warning.String(), "WARN") != 1 {
		t.Fatalf("wildcard warning = %q", warning.String())
	}
	(&Client{TrustedDomains: []string{"*"}}).warnWildcardDomains()
	if (&Client{TrustedDomains: []string{"none.test"}}).isEndpointTrusted("https://x.test") {
		t.Fatal("unlisted domain trusted")
	}
	if matchDomain("x.test", "") {
		t.Fatal("empty domain pattern matched")
	}
	if shouldPreserveEndpointQuery(nil) || shouldPreserveEndpointQuery(&url.URL{Scheme: "http", Host: "mcp-gw.dingtalk.com"}) {
		t.Fatal("invalid gateway preserved query")
	}
}

func TestCrossPlatformCoverageHTTPStatusAndDiagnosticsEdges(t *testing.T) {
	for _, tc := range []struct {
		method string
		code   int
	}{
		{"tools/list", http.StatusBadRequest},
		{"tools/call", http.StatusBadRequest},
		{"tools/call", http.StatusServiceUnavailable},
		{"tools/list", http.StatusServiceUnavailable},
	} {
		if err := httpStatusError(tc.method, "https://x.test?q=secret", tc.code, "snap", "trace"); err == nil {
			t.Fatalf("HTTP %d returned nil", tc.code)
		}
	}
	if got := networkActions("snap"); len(got) != 3 {
		t.Fatalf("network actions = %#v", got)
	}
	if err := jsonrpcEnvelopeError("tools/list", &RPCError{Code: -1, Message: "failed"}, "", "header-trace"); err == nil {
		t.Fatal("JSON-RPC trace fallback returned nil")
	}

	deep := map[string]any{"errorCode": "ERROR", "data": []any{"skip", map[string]any{"result": map[string]any{"code": "REAL"}}}}
	if got := serverErrorCodeFromMap(deep, 0); got != "REAL" {
		t.Fatalf("nested code = %q", got)
	}
	if got := serverErrorCodeFromMap(map[string]any{"errorCode": "ERROR"}, 0); got != "ERROR" {
		t.Fatalf("wrapper fallback = %q", got)
	}
	if got := serverErrorCodeFromMap(nil, 0); got != "" {
		t.Fatalf("nil code = %q", got)
	}
	if got := serverErrorCodeFromMap(map[string]any{"code": "x"}, 9); got != "" {
		t.Fatalf("depth-limited code = %q", got)
	}
	if got := nestedServerErrorCode(map[string]any{"content": []any{map[string]any{"code": "NEST"}}}, 0); got != "NEST" {
		t.Fatalf("array nested code = %q", got)
	}
	if coalesceStr("", "x") != "x" || coalesceStr("", "") != "" || stringFromMap(map[string]any{"x": 1}, "x") != "" {
		t.Fatal("string helpers failed")
	}
}

func TestCrossPlatformCoverageStdioCallAndStartEdges(t *testing.T) {
	makeClient := func(response string) (*StdioClient, *edgeWriteCloser) {
		stdin := &edgeWriteCloser{}
		return &StdioClient{started: true, stdin: stdin, stdout: bufio.NewReader(strings.NewReader(response))}, stdin
	}

	client, stdin := makeClient(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}` + "\n")
	if _, err := client.ListTools(context.Background()); err != nil || stdin.Len() == 0 {
		t.Fatalf("ListTools = %v, request=%q", err, stdin.String())
	}
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	client, _ = makeClient(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"v"}}` + "\n")
	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	client, _ = makeClient(`{"jsonrpc":"2.0","id":1,"result":{"content":[]}}` + "\n")
	if _, err := client.CallTool(context.Background(), "x", nil); err != nil {
		t.Fatal(err)
	}

	client, _ = makeClient("")
	if err := client.call(context.Background(), "x", nil, nil); err == nil {
		t.Fatal("EOF response succeeded")
	}
	if _, err := (&StdioClient{}).Initialize(context.Background()); err == nil {
		t.Fatal("unstarted Initialize succeeded")
	}
	if _, err := (&StdioClient{}).ListTools(context.Background()); err == nil {
		t.Fatal("unstarted ListTools succeeded")
	}
	client, _ = makeClient("bad\n")
	if err := client.call(context.Background(), "x", nil, nil); err == nil {
		t.Fatal("invalid response succeeded")
	}
	client, _ = makeClient(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"bad"}}` + "\n")
	if err := client.call(context.Background(), "x", nil, nil); err == nil {
		t.Fatal("RPC error succeeded")
	}
	client, _ = makeClient(`{"jsonrpc":"2.0","id":1,"result":"bad"}` + "\n")
	if err := client.call(context.Background(), "x", nil, &map[string]any{}); err == nil {
		t.Fatal("invalid result succeeded")
	}
	client, _ = makeClient(`{"jsonrpc":"2.0","id":1,"result":null}` + "\n")
	if err := client.call(context.Background(), "x", nil, nil); err != nil {
		t.Fatalf("nil result target: %v", err)
	}
	client, _ = makeClient("")
	client.stdin = &edgeWriteCloser{err: errors.New("write")}
	if err := client.call(context.Background(), "x", nil, nil); err == nil {
		t.Fatal("write error succeeded")
	}
	client, _ = makeClient("")
	if err := client.call(context.Background(), "x", func() {}, nil); err == nil {
		t.Fatal("marshal error succeeded")
	}
	client, _ = makeClient("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.call(ctx, "x", nil, nil); err == nil {
		t.Fatal("cancelled call succeeded")
	}

	stopped := &StdioClient{started: true, cmd: &exec.Cmd{}, stdin: &edgeWriteCloser{}}
	if err := stopped.Stop(); err == nil || stopped.started {
		t.Fatalf("Stop without process = %v", err)
	}
	if err := (&StdioClient{}).Stop(); err != nil {
		t.Fatal(err)
	}

	oldCommand := stdioCommandContext
	t.Cleanup(func() { stdioCommandContext = oldCommand })
	for _, stream := range []string{"stdin", "stdout", "stderr"} {
		stdioCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
			switch stream {
			case "stdin":
				cmd.Stdin = strings.NewReader("")
			case "stdout":
				cmd.Stdout = io.Discard
			case "stderr":
				cmd.Stderr = io.Discard
			}
			return cmd
		}
		if err := NewStdioClient("ignored", nil, map[string]string{"EDGE": "1"}).Start(context.Background()); err == nil {
			t.Fatalf("%s pipe failure succeeded", stream)
		}
	}

	stderr := &StdioClient{command: "edge", stderr: io.NopCloser(strings.NewReader("\nline\n"))}
	stderr.drainStderr()
}

func TestCrossPlatformCoverageDiagnosticsRecursiveArraysAndStdioInitializationCoverage(t *testing.T) {
	content := map[string]any{
		"errorCode": "ERROR",
		"content": []any{
			"skip",
			map[string]any{"data": []any{map[string]any{"code": "REAL", "friendlyHint": "hint"}}},
		},
	}
	diag := ExtractServerDiagnosticsFromMap(content)
	if diag.ServerErrorCode != "REAL" || diag.FriendlyHint != "hint" {
		t.Fatalf("recursive diagnostics = %#v", diag)
	}
	deep := map[string]any{"friendlyHint": "too deep"}
	for range 10 {
		deep = map[string]any{"data": deep}
	}
	if got := stringFromMapRecursive(deep, 0, "friendlyHint"); got != "" {
		t.Fatalf("depth-limited diagnostic = %q", got)
	}

	client := NewStdioClient("unused", nil, nil)
	client.initialized = true
	client.initResult = InitializeResult{ProtocolVersion: "test"}
	if err := client.EnsureInitialized(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := client.initialize(context.Background()); err != nil || got.ProtocolVersion != "test" {
		t.Fatalf("cached initialize = %#v, %v", got, err)
	}
}

func TestCrossPlatformCoverageEnsureInitializedReturnsStartError(t *testing.T) {
	client := NewStdioClient(filepath.Join(t.TempDir(), "missing"), nil, nil)
	if err := client.EnsureInitialized(context.Background()); err == nil {
		t.Fatal("EnsureInitialized() error = nil")
	}
}

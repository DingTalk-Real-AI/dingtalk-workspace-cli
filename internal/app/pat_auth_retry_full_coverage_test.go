package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

func TestCrossPlatformCoveragePATRetryRemainingPureAndWaitCoverage(t *testing.T) {
	typed := apperrors.NewAPI("ordinary", apperrors.WithHint("insufficient_scope"))
	if !isPatScopeError(typed) {
		t.Fatal("typed insufficient_scope was not recognized")
	}
	typed = apperrors.NewAPI("ordinary", apperrors.WithReason("reason"))
	if got := extractPatScopeError(typed); !strings.Contains(got.Message, "reason") {
		t.Fatalf("typed scope message = %q", got.Message)
	}

	var out bytes.Buffer
	PrintPatAuthError(&out, &PatScopeError{Identity: "user", ErrorType: "missing_scope", Message: "missing", Hint: "login"})
	if !strings.Contains(out.String(), "dws auth login") {
		t.Fatalf("PAT output = %q", out.String())
	}
	if wantsStructuredPATOutputFromRunner(runnerCoverageFallback{}) {
		t.Fatal("non-runtime runner requested structured PAT output")
	}
	if got := enrichPATErrorWithOpenBrowser("", true); got != "" {
		t.Fatalf("empty enriched PAT = %q", got)
	}
	if got := enrichPATErrorWithOpenBrowser("not-json", true); got != "not-json" {
		t.Fatalf("malformed enriched PAT = %q", got)
	}
	if got := enrichPATErrorWithOpenBrowser(`{"code":"x"}`, true); !strings.Contains(got, "openBrowser") {
		t.Fatalf("missing data enrichment = %q", got)
	}
	if got := patAuthorizationURIFromData(map[string]any{"authorizationUrl": " final "}); got != "final" {
		t.Fatalf("authorization URI = %q", got)
	}
	if err := openPATAuthorizationURI(""); err != nil {
		t.Fatal(err)
	}

	oldTimeout := patAuthorizationTimeout
	oldInterval := patAuthorizationPollInterval
	oldResolve := patResolveAccessToken
	t.Cleanup(func() {
		patAuthorizationTimeout = oldTimeout
		patAuthorizationPollInterval = oldInterval
		patResolveAccessToken = oldResolve
	})
	patAuthorizationTimeout = 50 * time.Millisecond
	patAuthorizationPollInterval = time.Millisecond
	patResolveAccessToken = func(context.Context, string, string) (string, error) {
		return "token", nil
	}
	out.Reset()
	if ok, err := WaitForPatAuthorization(context.Background(), "", &out); err != nil || !ok {
		t.Fatal("valid token did not authorize")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if ok, err := WaitForPatAuthorization(ctx, "", &out); ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled authorization = %v, %v", ok, err)
	}
	patAuthorizationTimeout = time.Millisecond
	patAuthorizationPollInterval = time.Hour
	out.Reset()
	if ok, err := WaitForPatAuthorization(context.Background(), "", &out); err != nil || ok {
		t.Fatalf("timed out authorization = %v, %v", ok, err)
	}
	patAuthorizationTimeout = 5 * time.Millisecond
	patAuthorizationPollInterval = time.Millisecond
	patResolveAccessToken = func(context.Context, string, string) (string, error) {
		return "", authpkg.ErrTokenDataNotFound
	}
	out.Reset()
	if ok, err := WaitForPatAuthorization(context.Background(), "", &out); err != nil || ok || !strings.Contains(out.String(), "等待授权中") {
		t.Fatalf("invalid-token polling = %v, %v, output %q", ok, err, out.String())
	}
}

func TestCrossPlatformCoveragePATRetryRemainingOrchestrationCoverage(t *testing.T) {
	oldWait := patWaitForAuthorization
	oldPoll := patPollDeviceFlowWithInterval
	oldSaveApp := patSaveAppConfig
	oldExchange := patExchangeCodeForToken
	oldSaveToken := patSaveTokenData
	oldSleep := patSleep
	oldOpen := openBrowserFunc
	t.Cleanup(func() {
		patWaitForAuthorization = oldWait
		patPollDeviceFlowWithInterval = oldPoll
		patSaveAppConfig = oldSaveApp
		patExchangeCodeForToken = oldExchange
		patSaveTokenData = oldSaveToken
		patSleep = oldSleep
		openBrowserFunc = oldOpen
	})
	t.Setenv(authpkg.AgentCodeEnv, "")
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())

	scope := &PatScopeError{OriginalError: "missing", Identity: "user", ErrorType: "missing_scope", Message: "missing", Hint: "login", MissingScope: "calendar:read"}
	patWaitForAuthorization = func(context.Context, string, io.Writer) (bool, error) { return false, nil }
	if _, err := retryWithPatAuthRetry(context.Background(), runnerCoverageFallback{}, executor.Invocation{}, scope, t.TempDir(), io.Discard); err == nil {
		t.Fatal("PAT retry timeout succeeded")
	}
	wantErr := errors.New("runner failed")
	patWaitForAuthorization = func(context.Context, string, io.Writer) (bool, error) { return true, nil }
	if _, err := retryWithPatAuthRetry(context.Background(), runnerCoverageFallback{err: wantErr}, executor.Invocation{}, scope, t.TempDir(), io.Discard); !errors.Is(err, wantErr) {
		t.Fatalf("authorized retry = %v", err)
	}

	patErr := &apperrors.PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
	if err := runDirectPATAuthCheck(context.Background(), nil, patErr, nil, io.Discard); !errors.Is(err, patErr) {
		t.Fatalf("nil direct retry = %v", err)
	}
	if err := runDirectPATAuthCheckWithMode(context.Background(), nil, patErr, nil, io.Discard, true); !errors.Is(err, patErr) {
		t.Fatalf("nil retry mode = %v", err)
	}

	badRetry := errors.New("retry callback")
	patPollDeviceFlowWithInterval = func(context.Context, string, string, io.Writer, time.Duration) (string, string, error) {
		return authpkg.StatusApproved, "", nil
	}
	patSleep = func(time.Duration) {}
	if err := runDirectPATAuthCheck(context.Background(), nil, &apperrors.PATError{RawJSON: patRaw("f", "", "")}, func(context.Context) error { return badRetry }, io.Discard); !errors.Is(err, badRetry) {
		t.Fatalf("direct retry callback = %v", err)
	}

	malformedPAT := &apperrors.PATError{RawJSON: `{`}
	if _, err := handlePatAuthCheck(context.Background(), &runtimeRunner{}, executor.Invocation{}, malformedPAT, t.TempDir(), io.Discard); !errors.Is(err, malformedPAT) {
		t.Fatalf("malformed PAT handler = %v", err)
	}

	openBrowserFunc = func(string) error { return nil }
	for _, raw := range []string{
		`{"code":"x","data":{"flowId":"f","authUrl":"https://auth.test","desc":"authorize"}}`,
		`{"code":"x","data":{"flowId":"f","authorizationUrl":"https://auth2.test"}}`,
	} {
		patPollDeviceFlowWithInterval = func(context.Context, string, string, io.Writer, time.Duration) (string, string, error) {
			return "", "", wantErr
		}
		ctx := context.WithValue(context.Background(), patSuppressBrowserOpenKey, true)
		if _, err := handlePatAuthCheck(ctx, &runtimeRunner{}, executor.Invocation{}, &apperrors.PATError{RawJSON: raw}, t.TempDir(), io.Discard); err == nil {
			t.Fatal("poll failure returned nil")
		}
	}

	statuses := []string{authpkg.StatusRejected, authpkg.StatusExpired, authpkg.StatusCancelled, "UNKNOWN"}
	for _, status := range statuses {
		patPollDeviceFlowWithInterval = func(context.Context, string, string, io.Writer, time.Duration) (string, string, error) {
			return status, "", nil
		}
		if _, err := handlePatAuthCheck(context.WithValue(context.Background(), patSuppressBrowserOpenKey, true), &runtimeRunner{}, executor.Invocation{}, &apperrors.PATError{RawJSON: patRaw("f", "", "")}, t.TempDir(), io.Discard); err == nil {
			t.Fatalf("status %s returned nil", status)
		}
	}

	patPollDeviceFlowWithInterval = func(context.Context, string, string, io.Writer, time.Duration) (string, string, error) {
		return authpkg.StatusApproved, "code", nil
	}
	patSaveAppConfig = func(string, *authpkg.AppConfig) error { return wantErr }
	patExchangeCodeForToken = func(context.Context, string, string) (*authpkg.TokenData, error) { return nil, wantErr }
	skip := executor.Invocation{Params: map[string]any{"retryAfterApproval": false}}
	if got, err := handlePatAuthCheck(context.Background(), &runtimeRunner{}, skip, &apperrors.PATError{RawJSON: patRaw("f", "client", "secret")}, t.TempDir(), io.Discard); err != nil || !got.Invocation.Implemented {
		t.Fatalf("approved skip = %#v, %v", got, err)
	}

	patSaveAppConfig = func(string, *authpkg.AppConfig) error { return nil }
	patExchangeCodeForToken = func(context.Context, string, string) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{AccessToken: "token"}, nil
	}
	patSaveTokenData = func(string, *authpkg.TokenData) error { return wantErr }
	r := &runtimeRunner{fallback: runnerCoverageFallback{result: executor.Result{Response: map[string]any{"ok": true}}}}
	if _, err := handlePatAuthCheck(context.Background(), r, executor.Invocation{}, &apperrors.PATError{RawJSON: patRaw("f", "client", "")}, t.TempDir(), io.Discard); err != nil {
		t.Fatalf("approved retry with save failure = %v", err)
	}
	patSaveTokenData = func(string, *authpkg.TokenData) error { return nil }
	if _, err := handlePatAuthCheck(context.Background(), r, executor.Invocation{}, &apperrors.PATError{RawJSON: patRaw("f", "", "")}, t.TempDir(), io.Discard); err != nil {
		t.Fatalf("approved retry = %v", err)
	}

	for _, inv := range []executor.Invocation{{}, {Params: map[string]any{}}, {Params: map[string]any{"retryAfterApproval": "no"}}, {Params: map[string]any{"retryAfterApproval": true}}} {
		if shouldSkipPATRetryAfterApproval(inv) {
			t.Fatalf("unexpected skip for %#v", inv.Params)
		}
	}
	if got := enrichPATErrorForHostControl(""); got != "" {
		t.Fatalf("empty host PAT = %q", got)
	}
	if got := enrichPATErrorForHostControl("bad"); got != "bad" {
		t.Fatalf("bad host PAT = %q", got)
	}
	if got := enrichPATErrorForHostControl(`{"value":1}`); !strings.Contains(got, "value") {
		t.Fatalf("generic host PAT = %q", got)
	}
	if _, err := marshalSingleLineJSONNoHTMLEscape(map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("unsupported JSON value succeeded")
	}
}

func patRaw(flowID, clientID, secret string) string {
	return `{"code":"x","data":{"desc":"authorize","flowId":"` + flowID + `","uri":"https://auth.test","clientId":"` + clientID + `","clientSecret":"` + secret + `"}}`
}

func TestCrossPlatformCoveragePATRetryRemainingPollAndBrowserCoverage(t *testing.T) {
	oldDo := patPollHTTPDo
	oldRequest := patPollNewRequest
	oldResolve := patResolveAccessToken
	oldBrowser := patBrowserOpenCommand
	t.Cleanup(func() {
		patPollHTTPDo = oldDo
		patPollNewRequest = oldRequest
		patResolveAccessToken = oldResolve
		patBrowserOpenCommand = oldBrowser
	})
	patResolveAccessToken = func(context.Context, string, string) (string, error) { return "token", nil }
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if status, _, err := pollPatDeviceFlowWithInterval(cancelled, "flow", t.TempDir(), io.Discard, 0); err != nil || status != authpkg.StatusCancelled {
		t.Fatalf("zero-interval cancelled poll = %q, %v", status, err)
	}
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if status, _, err := pollPatDeviceFlowWithInterval(expired, "flow", t.TempDir(), io.Discard, time.Millisecond); err != nil || status != authpkg.StatusExpired {
		t.Fatalf("expired-context poll = %q, %v", status, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	patPollHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		cancel()
		return nil, errors.New("network")
	}
	if status, _, err := pollPatDeviceFlowWithInterval(ctx, "flow", t.TempDir(), io.Discard, time.Millisecond); err != nil || status != authpkg.StatusCancelled {
		t.Fatalf("network poll = %q, %v", status, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	patPollHTTPDo = func(*http.Client, *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{"))}, nil
	}
	if status, _, err := pollPatDeviceFlowWithInterval(ctx, "flow", t.TempDir(), io.Discard, time.Millisecond); err != nil || status != authpkg.StatusCancelled {
		t.Fatalf("malformed poll = %q, %v", status, err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	patPollNewRequest = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		cancel()
		return nil, errors.New("request")
	}
	if status, _, err := pollPatDeviceFlowWithInterval(ctx, "flow", t.TempDir(), io.Discard, time.Millisecond); err != nil || status != authpkg.StatusCancelled {
		t.Fatalf("invalid request poll = %q, %v", status, err)
	}
	patPollNewRequest = oldRequest

	var out bytes.Buffer
	t.Setenv("DWS_DEBUG_PAT_POLL", "1")
	printPATPollDebugResponse(&out, 500, nil)
	if !strings.Contains(out.String(), "empty body") {
		t.Fatalf("empty debug response = %q", out.String())
	}
	for _, goos := range []string{"darwin", "linux", "windows", "plan9"} {
		_ = browserOpenCommand(goos, "https://example.test")
	}
	patBrowserOpenCommand = func(string, string) *exec.Cmd { return nil }
	if err := tryOpenBrowser("https://example.test"); err != nil {
		t.Fatal(err)
	}
	patBrowserOpenCommand = func(string, string) *exec.Cmd { return exec.Command("definitely-not-a-real-dws-command") }
	if err := tryOpenBrowser("https://example.test"); err == nil {
		t.Fatal("missing browser command started")
	}
}

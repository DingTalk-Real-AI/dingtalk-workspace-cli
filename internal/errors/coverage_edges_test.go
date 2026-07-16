package errors

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDiagnosticsAndErrorRenderingEdges(t *testing.T) {
	retryable := false
	err := NewAPI("message",
		nil,
		WithAvailableFlags(),
		WithActions("", "retry"),
		WithServerDiag(ServerDiagnostics{}),
		WithTraceID(" "),
		WithServerDiag(ServerDiagnostics{
			TraceID: "trace", ServerErrorCode: "TOKEN_VERIFIED_FAILED",
			TechnicalDetail: "detail", ServerRetryable: &retryable,
		}),
	)
	typed := err.(*Error)
	if typed.Retryable || typed.ServerDiag.TraceID != "trace" {
		t.Fatalf("diagnostics options = %#v", typed)
	}
	if (ServerDiagnostics{TraceID: "trace"}).IsEmpty() {
		t.Fatal("populated diagnostics should not be empty")
	}
	traceOnly := NewAPI("trace", WithTraceID(" trace-only ")).(*Error)
	if traceOnly.ServerDiag.TraceID != "trace-only" {
		t.Fatalf("trimmed trace ID = %q", traceOnly.ServerDiag.TraceID)
	}

	var out bytes.Buffer
	if err := PrintJSON(&out, err); err != nil || !strings.Contains(out.String(), "friendly_hint") {
		t.Fatalf("PrintJSON friendly diagnostics = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := PrintHumanAt(&out, err, VerbosityVerbose); err != nil || !strings.Contains(out.String(), "开启地址") {
		t.Fatalf("PrintHuman friendly diagnostics = %q, %v", out.String(), err)
	}
	out.Reset()
	flagsErr := NewAPI("flags", WithActions(" ", "act"), WithAvailableFlags("one", "two"))
	if err := PrintHuman(&out, flagsErr); err != nil || !strings.Contains(out.String(), "Flags: one, two") {
		t.Fatalf("PrintHuman flags/actions = %q, %v", out.String(), err)
	}

	oldMarshal := marshalErrorJSON
	t.Cleanup(func() { marshalErrorJSON = oldMarshal })
	marshalErrorJSON = func(any, string, string) ([]byte, error) { return nil, stderrors.New("encode") }
	out.Reset()
	if err := PrintJSON(&out, err); err != nil || !strings.Contains(out.String(), "failed to encode") {
		t.Fatalf("PrintJSON fallback = %q, %v", out.String(), err)
	}

	if got := formatAvailableFlagsHumanLine(nil); got != "" {
		t.Fatalf("empty flags line = %q", got)
	}
	if got := formatAvailableFlagsHumanLine([]string{strings.Repeat("x", 201)}); !strings.HasSuffix(got, "...") {
		t.Fatalf("long first flag = %q", got)
	}
	if got := formatAvailableFlagsHumanLine([]string{strings.Repeat("x", 200), "y"}); !strings.HasSuffix(got, "...") {
		t.Fatalf("long separator flags = %q", got)
	}
	if got := formatAvailableFlagsHumanLine([]string{"one", "two"}); got != "Flags: one, two" {
		t.Fatalf("normal flags line = %q", got)
	}
}

func TestCrossPlatformCoverageValidationCoverageEdges(t *testing.T) {
	for _, name := range []string{"", strings.Repeat("a", 129), "1startsWithDigit", "bad name"} {
		if ResourceName(name) == nil {
			t.Errorf("ResourceName(%q) should fail", name)
		}
	}
	for _, path := range []string{"safe\u200bname", "safe\u202ename", "safe\u2066name"} {
		if SafePath(path) == nil {
			t.Errorf("SafePath(%q) should fail", path)
		}
	}
	if _, err := SafeOutputPath("\x00"); err == nil {
		t.Fatal("control character output path should fail")
	}
	if _, err := SafeInputPath(filepath.Join(t.TempDir(), "absolute")); err == nil {
		t.Fatal("absolute input path should fail")
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
	outside := t.TempDir()
	if err := os.Symlink(outside, "outside"); err != nil {
		t.Fatalf("outside symlink: %v", err)
	}
	if _, err := SafeOutputPath("outside/file"); err == nil {
		t.Fatal("outside symlink should fail containment")
	}
	if err := os.Symlink("loop", "loop"); err != nil {
		t.Fatalf("loop symlink: %v", err)
	}
	if _, err := SafeInputPath("loop/child"); err == nil {
		t.Fatal("symlink loop should fail resolution")
	}
	if _, err := SafeLocalFlagPath("--file", "outside/file"); err == nil {
		t.Fatal("unsafe local flag path should fail")
	}
	for _, value := range []string{"", "http://example.test/file", "https://example.test/file"} {
		if got, err := SafeLocalFlagPath("--file", value); err != nil || got != value {
			t.Errorf("SafeLocalFlagPath(%q) = %q, %v", value, got, err)
		}
	}

	oldGetwd, oldLstat, oldEval, oldRel := getWorkingDir, lstatPath, evalSymlinks, relPath
	t.Cleanup(func() { getWorkingDir, lstatPath, evalSymlinks, relPath = oldGetwd, oldLstat, oldEval, oldRel })
	wantErr := stderrors.New("filesystem failure")
	getWorkingDir = func() (string, error) { return "", wantErr }
	if _, err := SafeOutputPath("file"); !stderrors.Is(err, wantErr) {
		t.Fatalf("getwd error = %v", err)
	}
	getWorkingDir = oldGetwd
	lstatPath = func(string) (os.FileInfo, error) { return nil, nil }
	evalSymlinks = func(string) (string, error) { return "", wantErr }
	if _, err := SafeOutputPath("file"); !stderrors.Is(err, wantErr) {
		t.Fatalf("existing symlink error = %v", err)
	}
	if _, err := resolveNearestAncestor("file"); !stderrors.Is(err, wantErr) {
		t.Fatalf("ancestor symlink error = %v", err)
	}
	lstatPath = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	if got, err := resolveNearestAncestor(string(filepath.Separator)); err != nil || got != string(filepath.Separator) {
		t.Fatalf("root ancestor = %q, %v", got, err)
	}
	relPath = func(string, string) (string, error) { return "", wantErr }
	if isUnderDir("child", "parent") {
		t.Fatal("relative-path failure should not be under parent")
	}
}

func TestCrossPlatformCoveragePATURLAndMutationCoverageEdges(t *testing.T) {
	oldHost := hostControlProvider
	oldBrowser := patBrowserProvider
	t.Cleanup(func() {
		SetHostControlProvider(oldHost)
		SetPATOpenBrowserProvider(oldBrowser)
	})
	SetHostControlProvider(nil)
	SetPATOpenBrowserProvider(nil)
	for _, initial := range []any{nil, "wrong-type"} {
		out := map[string]any{"data": initial}
		ApplyHostMutations(out)
		if _, ok := out["data"].(map[string]any); !ok {
			t.Fatalf("ApplyHostMutations(%#v) = %#v", initial, out)
		}
	}

	for _, data := range []map[string]any{
		{"authUrl": " https://example.test/auth "},
		{"authorizationUrl": "https://example.test/auth"},
		{"uri": 1},
	} {
		out := map[string]any{"data": data}
		ApplyHostMutations(out)
	}

	for _, raw := range []string{
		"",
		"not a url",
		"https://example.test/other",
		"https://example.test/fe/old#personalAuthorization?flowId=only",
		"https://example.test/fe/old?hash=done#personalAuthorization?flowId=f&userCode=u",
	} {
		if got := PATAuthorizationURL(raw); raw != "" && strings.TrimSpace(raw) != got {
			t.Errorf("PATAuthorizationURL(%q) = %q", raw, got)
		}
	}
	parsed := &url.URL{Scheme: "https", Host: "example.test", Path: "/fe/old", Fragment: "%2FpersonalAuthorization%3FflowId%3Df%26userCode%3Du"}
	if values := patAuthorizationRouteQuery(parsed); values.Get("flowId") != "f" {
		t.Fatalf("decoded route values = %v", values)
	}
	if values := patAuthorizationRouteQuery(mustParseURLForTest(t, "https://example.test/fe/old")); values != nil {
		t.Fatalf("missing route values = %v", values)
	}
	if parsePersonalAuthorizationRouteQuery("no-route") != nil || parsePersonalAuthorizationRouteQuery("personalAuthorization?bad=%zz") != nil {
		t.Fatal("invalid routes should not parse")
	}
	values := parsePersonalAuthorizationRouteQuery("#personalAuthorization?flowId=f&userCode=u#ignored")
	if values.Get("userCode") != "u" {
		t.Fatalf("cut route values = %v", values)
	}

	if _, err := marshalSingleLineJSONNoHTMLEscape(make(chan int)); err == nil {
		t.Fatal("unsupported PAT JSON value should fail")
	}
	if got := cleanPATJSON(map[string]any{"data": map[string]any{"unsupported": make(chan int)}}, "PAT_NO_PERMISSION"); !strings.Contains(got, "PAT_NO_PERMISSION") {
		t.Fatalf("PAT JSON fallback = %q", got)
	}
}

func mustParseURLForTest(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return parsed
}

func TestCrossPlatformCoverageInvalidRPCDataIsOmitted(t *testing.T) {
	err := NewAPI("bad rpc", WithRPCData(json.RawMessage(`{`)))
	var out bytes.Buffer
	if printErr := PrintJSON(&out, err); printErr != nil {
		t.Fatalf("PrintJSON: %v", printErr)
	}
	if strings.Contains(out.String(), "rpc_data") {
		t.Fatalf("invalid RPC data should be omitted: %q", out.String())
	}
}

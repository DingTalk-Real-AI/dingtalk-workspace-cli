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
	if typed.Retryable || !typed.RetryableSet || typed.ServerDiag.TraceID != "trace" {
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

func TestCrossPlatformCoveragePATClassification(t *testing.T) {
	patErr := &PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
	if patErr.Error() != patErr.RawJSON || patErr.RawStderr() != patErr.RawJSON || patErr.ExitCode() != ExitCodePermission {
		t.Fatalf("PATError contract changed: %#v", patErr)
	}
	if !IsPATError(patErr) || IsPATError(stderrors.New("plain")) {
		t.Fatal("PAT error classification changed")
	}
	if !IsPATNoPermissionCode("PAT_NO_PERMISSION") || IsPATNoPermissionCode("UNKNOWN") {
		t.Fatal("PAT permission code classification changed")
	}

	if code, ok := lookupCodeIn(map[string]any{
		"code":      1,
		"errorCode": "PAT_NO_PERMISSION",
	}, patNoPermissionCodes); !ok || code != "PAT_NO_PERMISSION" {
		t.Fatalf("lookupCodeIn fallback = %q, %v", code, ok)
	}
	if code, ok := lookupCodeIn(map[string]any{"code": "UNKNOWN"}, patNoPermissionCodes); ok || code != "" {
		t.Fatalf("lookupCodeIn unknown = %q, %v", code, ok)
	}
	for _, tc := range []struct {
		body map[string]any
		want string
		ok   bool
	}{
		{body: map[string]any{"code": "PAT_NO_PERMISSION"}, want: "PAT_NO_PERMISSION", ok: true},
		{body: map[string]any{"error_code": "PAT_SCOPE_AUTH_REQUIRED"}, want: "PAT_SCOPE_AUTH_REQUIRED", ok: true},
		{body: map[string]any{"code": "UNKNOWN"}},
	} {
		code, ok := getPATErrorCode(tc.body)
		if code != tc.want || ok != tc.ok {
			t.Fatalf("getPATErrorCode(%v) = %q, %v", tc.body, code, ok)
		}
	}
	if code, ok := getDWSGatewayErrorCode(map[string]any{"errorCode": "DWS_AUTH_SERVICE_FAILED"}); !ok || code != "DWS_AUTH_SERVICE_FAILED" {
		t.Fatalf("gateway code = %q, %v", code, ok)
	}

	if !isNotLoggedInError(map[string]any{
		"error":   1,
		"message": "Missing service_id or access_key",
	}) {
		t.Fatal("missing-login response was not recognized")
	}
	if isNotLoggedInError(map[string]any{"message": "other"}) {
		t.Fatal("ordinary response was recognized as missing login")
	}
	for _, body := range []map[string]any{
		{"error": "failure"},
		{"success": false},
		{"success": "FALSE"},
	} {
		if !isBusinessError(body) {
			t.Fatalf("business error was not recognized: %#v", body)
		}
	}
	for _, body := range []map[string]any{
		{},
		{"success": true},
		{"success": "true"},
	} {
		if isBusinessError(body) {
			t.Fatalf("successful response was classified as a business error: %#v", body)
		}
	}

	if err := ClassifyToolResultContent(map[string]any{"code": "DWS_SERVICE_UNAUTHORIZED"}); err == nil {
		t.Fatal("gateway tool result was not classified")
	}
	if err := ClassifyToolResultContent(map[string]any{"code": "PAT_BATCH_AUTH_PENDING"}); !IsPATError(err) {
		t.Fatalf("PAT tool result = %T %v", err, err)
	}
	if err := ClassifyToolResultContent(map[string]any{"success": true}); err != nil {
		t.Fatalf("successful tool result = %v", err)
	}

	responseCases := []struct {
		name string
		text string
		kind string
	}{
		{name: "invalid json", text: "not-json", kind: "nil"},
		{name: "gateway", text: `{"code":"DWS_AUTH_SERVICE_FAILED"}`, kind: "error"},
		{name: "not logged in", text: `{"message":"Missing service_id or access_key"}`, kind: "error"},
		{name: "pat", text: `{"code":"PAT_NO_PERMISSION"}`, kind: "pat"},
		{name: "business", text: `{"success":false,"message":"参数错误"}`, kind: "error"},
		{name: "success", text: `{"success":true}`, kind: "nil"},
	}
	for _, tc := range responseCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ClassifyMCPResponseText(tc.text)
			switch tc.kind {
			case "nil":
				if err != nil {
					t.Fatalf("ClassifyMCPResponseText() = %v", err)
				}
			case "pat":
				if !IsPATError(err) {
					t.Fatalf("ClassifyMCPResponseText() = %T %v", err, err)
				}
			default:
				if err == nil {
					t.Fatal("ClassifyMCPResponseText() returned nil")
				}
			}
		})
	}
	if !strings.Contains(authExpiredHint(), "auth login") || !strings.Contains(notLoggedInHint(), "auth login") {
		t.Fatal("authentication recovery hints lost the login command")
	}

	if got := ClassifyPatAuthCheck(map[string]any{"code": "PAT_LOW_RISK_NO_PERMISSION"}); got == nil {
		t.Fatal("ClassifyPatAuthCheck() returned nil")
	}
	if got := ClassifyPatAuthCheck(map[string]any{"code": "UNKNOWN"}); got != nil {
		t.Fatalf("ClassifyPatAuthCheck() = %#v", got)
	}
	wrapped := stderrors.Join(stderrors.New("outer"), patErr)
	if got := AsPatAuthCheckError(wrapped); got != patErr {
		t.Fatalf("AsPatAuthCheckError() = %#v", got)
	}
	if got := AsPatAuthCheckError(stderrors.New("plain")); got != nil {
		t.Fatalf("AsPatAuthCheckError() = %#v", got)
	}
}

func TestSuggestBusinessHintChatRecovery(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
		want string
	}{
		{
			name: "missing group role context",
			body: map[string]any{
				"error":   map[string]any{"code": "IM_ERROR", "message": "listRoles null"},
				"summary": "context",
				"code":    "TOP_LEVEL",
			},
			want: "list-my-groups",
		},
		{name: "legacy open id spelling", body: map[string]any{"message": "OpendId is not in conversation"}, want: "实际加入"},
		{name: "open id outside conversation", body: map[string]any{"message": "OpenId is not in conversation"}, want: "实际加入"},
		{name: "operator outside source group", body: map[string]any{"message": "The operator is not in this group chat"}, want: "源群"},
		{name: "missing invitation receiver", body: map[string]any{"message": "targetOpenConversationId和receiverUid不能同时为空"}, want: "--receiver"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SuggestBusinessHint(tc.body); !strings.Contains(got, tc.want) {
				t.Errorf("SuggestBusinessHint(%v) = %q, want containing %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoveragePATSerializationAndPolicyEdges(t *testing.T) {
	oldHost := hostControlProvider
	oldBrowser := patBrowserProvider
	t.Cleanup(func() {
		SetHostControlProvider(oldHost)
		SetPATOpenBrowserProvider(oldBrowser)
	})

	SetHostControlProvider(func() string { return "" })
	if block := HostControlBlock(); block != nil {
		t.Fatalf("empty host provider returned %#v", block)
	}
	SetHostControlProvider(func() string { return "codex" })
	SetPATOpenBrowserProvider(func() bool { return false })

	out := map[string]any{
		"data": map[string]any{
			"authUrl":   " https://example.test/fe/old#%2FpersonalAuthorization%3FflowId%3Df%26userCode%3Du ",
			"callbacks": map[string]any{"owner": "cli"},
		},
	}
	ApplyHostMutations(out)
	data := out["data"].(map[string]any)
	if data["openBrowser"] != false || data["hostControl"] == nil || data["uri"] == "" {
		t.Fatalf("host mutations = %#v", data)
	}
	if _, ok := data["authUrl"]; ok {
		t.Fatalf("authUrl alias was not removed: %#v", data)
	}
	if _, ok := data["callbacks"]; ok {
		t.Fatalf("legacy callbacks were not removed: %#v", data)
	}

	rawPolicy := cleanPATJSON(map[string]any{
		"message": "organization denied",
		"scope":   "chat.read",
	}, "PAT_ORG_POLICY_DENIED")
	var policyPayload map[string]any
	if err := json.Unmarshal([]byte(rawPolicy), &policyPayload); err != nil {
		t.Fatalf("decode policy PAT JSON: %v", err)
	}
	policyData := policyPayload["data"].(map[string]any)
	for key, want := range map[string]any{
		"policy":      "OPEN_SOURCE_ORG_SCOPE_FORBIDDEN",
		"message":     "organization denied",
		"action":      "contact_org_admin",
		"openBrowser": false,
		"retryable":   false,
	} {
		if got := policyData[key]; got != want {
			t.Fatalf("policy data %s = %#v, want %#v", key, got, want)
		}
	}
	if !strings.Contains(policyData["hint"].(string), "organization denied") {
		t.Fatalf("policy hint = %#v", policyData["hint"])
	}

	rawDefault := cleanPATJSON(map[string]any{}, "PAT_ORG_POLICY_DENIED")
	var defaultPayload map[string]any
	if err := json.Unmarshal([]byte(rawDefault), &defaultPayload); err != nil {
		t.Fatalf("decode default policy PAT JSON: %v", err)
	}
	defaultData := defaultPayload["data"].(map[string]any)
	if !strings.Contains(defaultData["hint"].(string), "组织策略") {
		t.Fatalf("default policy hint = %#v", defaultData["hint"])
	}

	prepopulated := map[string]any{"data": map[string]any{
		"policy":  "CUSTOM",
		"message": "existing",
		"hint":    "existing hint",
	}}
	applyOrgPolicyDeniedHint(prepopulated, map[string]any{"message": "ignored"})
	prepopulatedData := prepopulated["data"].(map[string]any)
	if prepopulatedData["policy"] != "CUSTOM" ||
		prepopulatedData["message"] != "existing" ||
		prepopulatedData["hint"] != "existing hint" {
		t.Fatalf("prepopulated policy fields changed: %#v", prepopulatedData)
	}

	if got := stringValue(map[string]any{
		"number": 1,
		"blank":  " ",
		"value":  " kept ",
	}, "number", "blank", "value"); got != "kept" {
		t.Fatalf("stringValue fallback = %q", got)
	}
	if got := stringValue(map[string]any{"blank": " "}, "blank", "missing"); got != "" {
		t.Fatalf("stringValue empty = %q", got)
	}

	cleaned := stripClassFields(map[string]any{
		"class": "top",
		"items": []any{
			map[string]any{"class": "nested", "keep": "yes"},
			"scalar",
		},
	}).(map[string]any)
	if _, ok := cleaned["class"]; ok {
		t.Fatalf("top-level class was retained: %#v", cleaned)
	}
	items := cleaned["items"].([]any)
	nested := items[0].(map[string]any)
	if _, ok := nested["class"]; ok || nested["keep"] != "yes" || items[1] != "scalar" {
		t.Fatalf("nested class cleanup = %#v", cleaned)
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

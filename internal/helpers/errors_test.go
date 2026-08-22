package helpers

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageCLIErrorFormattingExitCodesAndJSON(t *testing.T) {
	cause := errors.New("root cause")
	err := &CLIError{Code: CodeInvalidParam, Message: "bad input", Suggestion: "fix it", Operation: "doc/read", Cause: cause}
	if got := err.Error(); !strings.Contains(got, "doc/read") || !strings.Contains(got, "fix it") {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(err, cause) || err.ExitCode() != ExitValidation {
		t.Fatalf("Unwrap/ExitCode = %v/%d", err.Unwrap(), err.ExitCode())
	}
	encoded := err.ToJSON()["error"].(map[string]any)
	for _, key := range []string{"code", "message", "exit_code", "operation", "suggestion", "cause"} {
		if _, ok := encoded[key]; !ok {
			t.Errorf("ToJSON() missing %q: %#v", key, encoded)
		}
	}
	minimal := (&CLIError{Code: CodeUnclassified, Message: "bad"}).ToJSON()["error"].(map[string]any)
	if len(minimal) != 3 {
		t.Fatalf("minimal ToJSON() = %#v", minimal)
	}

	exitCases := map[string]int{
		CodeAuthNotConfigured: ExitAuth, CodeAuthTokenExpired: ExitAuth,
		CodeAuthPermission: ExitPermission, CodeMissingParam: ExitValidation,
		CodeInvalidParam: ExitValidation, CodeInvalidJSON: ExitValidation,
		CodeInvalidPath: ExitValidation, CodeInputTooLarge: ExitValidation,
		CodeFileNotFound: ExitValidation, CodeContentTruncated: ExitAPI,
		CodeMCPServerError: ExitAPI, CodeMCPToolError: ExitAPI,
		CodeNetworkTimeout: ExitAPI, CodeNetworkUnreachable: ExitAPI,
		CodeResourceNotFound: ExitAPI, CodeTableNotFound: ExitAPI,
		CodeSheetNotFound: ExitAPI, CodeFieldNotFound: ExitAPI,
		CodeRecordNotFound: ExitAPI, CodeLockTimeout: ExitInternal,
		CodeUnclassified: ExitInternal, "unknown": ExitInternal,
	}
	for code, want := range exitCases {
		if got := (&CLIError{Code: code}).ExitCode(); got != want {
			t.Errorf("ExitCode(%s) = %d, want %d", code, got, want)
		}
	}

	pat := &PATError{RawJSON: `{"code":"PAT_NO_PERMISSION"}`}
	if pat.Error() != pat.RawJSON || pat.RawStderr() != pat.RawJSON || pat.ExitCode() != ExitPermission {
		t.Fatalf("PATError methods changed: %#v", pat)
	}
}

func TestCrossPlatformCoverageWrapErrorClassifiesEveryErrorFamily(t *testing.T) {
	if WrapError(nil) != nil {
		t.Fatal("WrapError(nil) != nil")
	}
	existing := &CLIError{Code: CodeInvalidParam}
	if WrapError(existing) != existing {
		t.Fatal("WrapError changed an existing CLIError")
	}
	pat := &PATError{RawJSON: `{}`}
	if WrapError(pat) != pat {
		t.Fatal("WrapError changed an existing PATError")
	}

	tests := []struct {
		message string
		op      string
		code    string
		hint    string
	}{
		{"timeout acquiring data lock", "", CodeLockTimeout, "file lock"},
		{"lookup: no such host", "", CodeNetworkUnreachable, "DNS"},
		{"TLS handshake failed", "", CodeNetworkUnreachable, "TLS"},
		{"connection refused", "", CodeNetworkUnreachable, "connect"},
		{"context deadline exceeded", "", CodeNetworkTimeout, "timed out"},
		{"USER_TOKEN_ILLEGAL", "", CodeAuthTokenExpired, "Token"},
		{"api_key_expired", "", CodeAuthTokenExpired, "API Key"},
		{"Missing service_id or access_key", "", CodeAuthNotConfigured, "未登录"},
		{"403 forbidden", "", CodeAuthPermission, "Permission"},
		{"permission denied for doc", "", CodeAuthPermission, "document owner"},
		{"permission denied for aitable base", "", CodeAuthPermission, "base list"},
		{"permission denied for chat group", "", CodeAuthPermission, "group member"},
		{"server preserved detail", "sheet/batch_update", CodeMCPServerError, "preserved detail"},
		{"resource not found", "", CodeResourceNotFound, "resource"},
		{"table not found", "", CodeTableNotFound, "doc search"},
		{"群不存在", "", CodeResourceNotFound, "disbanded"},
		{"文档不存在", "", CodeResourceNotFound, "deleted"},
		{"MCP不存在 PARAM_ERROR", "", CodeMCPServerError, "未注册"},
		{"invalid character in JSON", "", CodeInvalidJSON, "JSON"},
		{"add_base_record json: bad", "", CodeInvalidJSON, "--data"},
		{"字段 fieldId json: bad", "", CodeInvalidJSON, "fieldId"},
		{"HTTP 503", "", CodeMCPServerError, "internal"},
		{"操作失败", "", CodeMCPToolError, "verbose"},
		{"字段类型操作失败", "", CodeMCPToolError, "field get"},
		{"tool 调用失败", "", CodeMCPToolError, "verbose"},
		{"搜索内容不能为空", "", CodeMCPToolError, "doc search"},
		{"totally unknown", "trace/op", CodeUnclassified, "totally unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.message, func(t *testing.T) {
			wrapped := WrapErrorWithOperation(errors.New(tc.message), tc.op)
			cli, ok := wrapped.(*CLIError)
			if !ok || cli.Code != tc.code || !strings.Contains(cli.Error(), tc.hint) {
				t.Fatalf("WrapErrorWithOperation() = %#v (%v)", wrapped, wrapped)
			}
			if tc.op != "" && cli.Operation != tc.op {
				t.Fatalf("operation = %q, want %q", cli.Operation, tc.op)
			}
		})
	}

	if !IsAuthError(&CLIError{Code: CodeAuthTokenExpired}) || IsAuthError(errors.New("AUTH_TOKEN_EXPIRED")) || IsAuthError(&CLIError{Code: CodeAuthPermission}) {
		t.Fatal("IsAuthError classification changed")
	}
}

func TestCrossPlatformCoverageBusinessSuggestionsAndResponseClassification(t *testing.T) {
	suggestions := map[string]string{
		"搜索内容不能为空":                                    "doc search",
		"User has no permission to access this email": "mailbox list",
		"频率超限":                                        "rate limit",
		"rate limit":                                  "rate limit",
		"用户不存在":                                       "--members",
		"不属于当前组织":                                     "--members",
		"成员不存在":                                       "--members",
		"未找到指定工具":                                     "未注册",
		"MCP不存在":                                      "未注册",
		"参数错误":                                        "input parameters",
		"param error":                                 "input parameters",
		"无权限访问":                                       "权限申请",
		"权限不足":                                        "权限申请",
	}
	for input, want := range suggestions {
		if got := suggestForBusinessErrorText(input); !strings.Contains(got, want) {
			t.Errorf("suggestForBusinessErrorText(%q) = %q", input, got)
		}
	}
	if suggestForBusinessErrorText("ok") != "" {
		t.Fatal("unknown business text produced a suggestion")
	}

	if ClassifyToolResultContent(map[string]any{"ok": true}) != nil {
		t.Fatal("successful tool content was classified as an error")
	}
	for _, key := range []string{"errorCode", "error_code", "code"} {
		err := ClassifyToolResultContent(map[string]any{key: "USER_TOKEN_ILLEGAL"})
		if cli, ok := err.(*CLIError); !ok || cli.Code != CodeAuthTokenExpired {
			t.Errorf("gateway key %s classified as %#v", key, err)
		}
	}
	for _, key := range []string{"code", "errorCode"} {
		err := ClassifyToolResultContent(map[string]any{key: "PAT_NO_PERMISSION", "data": map[string]any{"class": "secret", "allowed": true}})
		if _, ok := err.(*PATError); !ok {
			t.Errorf("PAT key %s classified as %#v", key, err)
		}
	}
	// forbidden.accessDenied（新接口权限不足错误码）应分类为 AUTH_PERMISSION_DENIED
	// 并携带权限申请指引，而不是被 generic business-error 渲染吞掉。
	for _, key := range []string{"code", "errorCode", "server_error_code"} {
		err := ClassifyToolResultContent(map[string]any{key: "forbidden.accessDenied", "success": false, "errorMsg": "需要您具备 MANAGER 及以上角色"})
		cli, ok := err.(*CLIError)
		if !ok || cli.Code != CodeAuthPermission {
			t.Errorf("accessDenied key %s classified as %#v, want AUTH_PERMISSION_DENIED", key, err)
			continue
		}
		if !strings.Contains(cli.Message, "需要您具备") || !strings.Contains(cli.Suggestion, "permission apply") {
			t.Errorf("accessDenied guidance missing: message=%q suggestion=%q", cli.Message, cli.Suggestion)
		}
	}

	for _, tc := range []struct {
		text string
		kind string
	}{
		{"not-json", "nil"},
		{"null", "nil"},
		{`{"errorCode":"USER_TOKEN_ILLEGAL"}`, "cli"},
		{`{"error":"Missing service_id or access_key"}`, "cli"},
		{`{"code":"PAT_HIGH_RISK_NO_PERMISSION","extra":{"class":"x","keep":1}}`, "pat"},
		{`{"success":false,"errorMsg":"rate limit"}`, "cli"},
		{`{"success":false,"code":"forbidden.accessDenied","errorMsg":"需要您具备 MANAGER 及以上角色","logId":"abc123"}`, "authperm"},
		{`{"success":false,"errorMsg":"members[0].id 对应的用户不存在"}`, "cli"},
		{`{"success":true}`, "nil"},
	} {
		err := ClassifyMCPResponseText(tc.text)
		switch tc.kind {
		case "nil":
			if err != nil {
				t.Errorf("ClassifyMCPResponseText(%s) = %v", tc.text, err)
			}
		case "cli":
			if _, ok := err.(*CLIError); !ok {
				t.Errorf("ClassifyMCPResponseText(%s) = %#v", tc.text, err)
			}
		case "authperm":
			cli, ok := err.(*CLIError)
			if !ok || cli.Code != CodeAuthPermission {
				t.Errorf("ClassifyMCPResponseText(%s) = %#v, want AUTH_PERMISSION_DENIED", tc.text, err)
			}
		case "pat":
			if _, ok := err.(*PATError); !ok {
				t.Errorf("ClassifyMCPResponseText(%s) = %#v", tc.text, err)
			}
		}
	}
}

func TestCrossPlatformCoveragePATCleanupAndMatchingHelpers(t *testing.T) {
	cleaned := cleanPATJSON(map[string]any{
		"code": "PAT_NO_PERMISSION",
		"data": map[string]any{"class": "secret", "items": []any{map[string]any{"class": "hidden", "keep": true}}},
	}, "PAT_NO_PERMISSION")
	if strings.Contains(cleaned, "class") || !strings.Contains(cleaned, "keep") {
		t.Fatalf("cleanPATJSON(data) = %s", cleaned)
	}
	cleaned = cleanPATJSON(map[string]any{"code": "x", "extra": map[string]any{"class": "hidden", "value": 1}}, "x")
	if strings.Contains(cleaned, "class") || !strings.Contains(cleaned, "value") {
		t.Fatalf("cleanPATJSON(fallback) = %s", cleaned)
	}
	cleaned = cleanPATJSON(map[string]any{"code": "x"}, "x")
	if strings.Contains(cleaned, "data") {
		t.Fatalf("cleanPATJSON(empty fallback) = %s", cleaned)
	}
	cleaned = cleanPATJSON(map[string]any{"data": make(chan int)}, "x")
	if cleaned != `{"success":false,"code":"x"}` {
		t.Fatalf("cleanPATJSON(marshal error) = %s", cleaned)
	}
	if stripClassFields("value") != "value" {
		t.Fatal("stripClassFields scalar changed")
	}

	if !errContainsAny("TLS Handshake", "tls") || errContainsAny("ok", "bad") {
		t.Fatal("errContainsAny matching changed")
	}
	for _, tc := range []struct {
		input string
		word  string
		want  bool
	}{
		{"HTTP 500 error", "500", true}, {"x500y", "500", false},
		{"before 500", "500", true}, {"500after", "500", false}, {"none", "500", false},
		{"aaa", "a", false},
	} {
		if got := errContainsWord(tc.input, tc.word); got != tc.want {
			t.Errorf("errContainsWord(%q, %q) = %v", tc.input, tc.word, got)
		}
	}
	for _, value := range []byte{'0', '9', 'a', 'z', 'A', 'Z'} {
		if !isAlnum(value) {
			t.Errorf("isAlnum(%q) = false", value)
		}
	}
	if isAlnum('-') {
		t.Fatal("isAlnum('-') = true")
	}

	if got := fmt.Sprint(stripClassFields([]any{map[string]any{"class": "x", "v": 1}})); strings.Contains(got, "class") {
		t.Fatalf("stripClassFields slice = %s", got)
	}
}

// TestCrossPlatformCoverageBusinessErrorMessagePriority 同步自内部 MR 28965577：
// errorMessage 是新接口返回的用户/成员校验错误的字段，优先级介于 errorMsg 与 message 之间。
func TestCrossPlatformCoverageBusinessErrorMessagePriority(t *testing.T) {
	if msg := businessErrorMessage(map[string]any{"errorMsg": "first", "errorMessage": "second", "message": "third", "error": "fourth"}); msg != "first" {
		t.Errorf("businessErrorMessage = %q, want %q (errorMsg has priority)", msg, "first")
	}
	want := "members[0].id 对应的用户不存在"
	if msg := businessErrorMessage(map[string]any{"errorMessage": want, "message": "third", "error": "fourth"}); msg != want {
		t.Errorf("businessErrorMessage = %q, want %q (errorMessage fallback)", msg, want)
	}
	if msg := businessErrorMessage(map[string]any{"message": "second", "error": "third"}); msg != "second" {
		t.Errorf("businessErrorMessage = %q, want %q (message fallback)", msg, "second")
	}
	if msg := businessErrorMessage(map[string]any{}); msg != "" {
		t.Errorf("businessErrorMessage(empty) = %q, want empty", msg)
	}
}

// TestCrossPlatformCoverageBusinessErrorDisplayMessage 验证 code/logId 附加以便排查。
func TestCrossPlatformCoverageBusinessErrorDisplayMessage(t *testing.T) {
	got := businessErrorDisplayMessage(map[string]any{"errorMessage": "权限不足", "logId": "abc123"}, "raw")
	if got != "权限不足 (logId: abc123)" {
		t.Errorf("businessErrorDisplayMessage = %q, want logId appended", got)
	}
	// errorCode/code 存在时附加在后，保证后端错误码可见
	got = businessErrorDisplayMessage(map[string]any{"errorMessage": "失败", "errorCode": "forbidden.document.sizeOverLimit"}, "raw")
	if got != "失败 (code: forbidden.document.sizeOverLimit)" {
		t.Errorf("businessErrorDisplayMessage = %q, want code appended", got)
	}
	got = businessErrorDisplayMessage(map[string]any{"message": "失败", "code": "forbidden.accessDenied", "logId": "l1"}, "raw")
	if got != "失败 (code: forbidden.accessDenied, logId: l1)" {
		t.Errorf("businessErrorDisplayMessage = %q, want code+logId appended", got)
	}
	// logId 已包含在 message 中时不重复追加
	got = businessErrorDisplayMessage(map[string]any{"errorMessage": "失败 logId:abc123", "logId": "abc123"}, "raw")
	if got != "失败 logId:abc123" {
		t.Errorf("businessErrorDisplayMessage = %q, want no duplicate logId", got)
	}
	// 无任何 message 字段时回退 rawText
	if got := businessErrorDisplayMessage(map[string]any{"success": false}, "raw-fallback"); got != "raw-fallback" {
		t.Errorf("businessErrorDisplayMessage = %q, want rawText fallback", got)
	}
}

// TestCrossPlatformCoverageUserNotInOrgErrors 验证「用户不存在/不属于当前组织/成员不存在」
// 在 RESOURCE_NOT_FOUND 之前被拦截为 MCP_TOOL_ERROR 并携带 --members corpId 建议。
func TestCrossPlatformCoverageUserNotInOrgErrors(t *testing.T) {
	suggestion := suggestForBusinessError(map[string]any{"errorMessage": "members[0].id 对应的用户不存在或不属于当前组织（实际值：209499），请检查后重试。"})
	if !strings.Contains(suggestion, "--members") {
		t.Errorf("suggestForBusinessError should mention --members for user/org error, got: %q", suggestion)
	}

	for _, msg := range []string{
		"用户不存在",
		"不属于当前组织",
		"成员不存在",
	} {
		cli, ok := WrapErrorWithOperation(errors.New(msg), "wiki/add_member").(*CLIError)
		if !ok {
			t.Fatalf("WrapErrorWithOperation(%q) not a CLIError", msg)
		}
		if cli.Code != CodeMCPToolError {
			t.Errorf("WrapErrorWithOperation(%q).Code = %s, want %s (not RESOURCE_NOT_FOUND)", msg, cli.Code, CodeMCPToolError)
		}
		if !strings.Contains(cli.Suggestion, "corpId") {
			t.Errorf("WrapErrorWithOperation(%q).Suggestion should mention corpId, got: %q", msg, cli.Suggestion)
		}
	}

	// JSON 体形式：透传 errorMessage 且附 logId
	cli, ok := WrapErrorWithOperation(errors.New(`{"errorMessage":"用户不存在","logId":"lid-1"}`), "wiki/add_member").(*CLIError)
	if !ok {
		t.Fatal("WrapErrorWithOperation(json) not a CLIError")
	}
	if cli.Message != "用户不存在 (logId: lid-1)" {
		t.Errorf("WrapErrorWithOperation(json).Message = %q, want errorMessage + logId", cli.Message)
	}
}

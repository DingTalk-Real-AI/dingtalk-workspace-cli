package runtimecontext

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAttachToURL(t *testing.T) {
	token := "private+runtime/&=测试 value"
	ready := ReadyResultForTest(token)
	allowed := []string{"login.dingtalk.com"}
	raw := "https://login.dingtalk.com/oauth2/auth?client_id=client&callerUmt=old&callerUmt=duplicate&caller=old&caller=again&redirect_uri=http://127.0.0.1:9/callback#fragment"
	got, ok := ready.AttachToURL(raw, allowed)
	parsed, err := url.Parse(got)
	if err != nil || !ok {
		t.Fatal("URL attachment failed")
	}
	q := parsed.Query()
	if q.Get("callerUmt") != token || q.Get("caller") != "dws" || len(q["callerUmt"]) != 1 || len(q["caller"]) != 1 || q.Get("client_id") != "client" || parsed.Fragment != "fragment" {
		t.Fatal("URL contract mismatch")
	}
	if second, _ := ready.AttachToURL(got, allowed); second != got {
		t.Fatal("attachment is not idempotent")
	}
	trusted := raw
	for _, caseRaw := range []string{
		"https://LOGIN.DINGTALK.COM/oauth2/auth?redirect_uri=https://login.dingtalk.com/cb",
		"https://login.dingtalk.com./oauth2/auth?redirect=http://localhost:9/cb",
		"https://login.dingtalk.com/oauth2/auth?redirect_uri=http://[::1]/cb",
	} {
		if got, ok := ready.AttachToURL(caseRaw, allowed); !ok || !strings.Contains(got, "callerUmt=") {
			t.Fatalf("trusted URL %q did not attach", caseRaw)
		}
	}
	for _, raw := range []string{
		"%",
		"https://login.dingtalk.com/?bad=%zz",
		"https://login.dingtalk.com/?a=1;b=2",
		"file:///tmp/file",
		"/relative",
		"https://user:secret@login.dingtalk.com/",
		"http://login.dingtalk.com/oauth2/auth",
		"https://example.test/login",
		"https://login.dingtalk.io/oauth2/auth",
		"https://login.dingtalk.com/oauth2/auth?redirect_uri=https://evil.test/cb",
		"https://login.dingtalk.com/oauth2/auth?redirect=https://evil.test/cb",
	} {
		if got, ok := ready.AttachToURL(raw, allowed); ok || got != raw {
			t.Fatalf("untrusted URL %q did not fail open", raw)
		}
	}
	if got, ok := ready.AttachToURL(trusted, nil); ok || got != trusted {
		t.Fatal("empty allowlist must fail open")
	}
	for _, result := range []Result{{}, {State: StateTimeout}, {State: StateError}, ReadyResultForTest(""), ReadyResultForTest("bad\nvalue")} {
		if got, ok := result.AttachToURL(trusted, allowed); ok || got != trusted {
			t.Fatal("unavailable context did not fail open")
		}
	}
	for _, value := range []any{ready, ready.DiagnosticDetail()} {
		data, _ := json.Marshal(value)
		if strings.Contains(string(data), token) || strings.Contains(fmt.Sprintf("%v %#v", value, value), token) {
			t.Fatal("runtime context leaked")
		}
	}
}

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
	raw := "https://example.test/login?client_id=client&callerUmt=old&callerUmt=duplicate&caller=old&caller=again#fragment"
	got, ok := ready.AttachToURL(raw)
	parsed, err := url.Parse(got)
	if err != nil || !ok {
		t.Fatal("URL attachment failed")
	}
	q := parsed.Query()
	if q.Get("callerUmt") != token || q.Get("caller") != "dws" || len(q["callerUmt"]) != 1 || len(q["caller"]) != 1 || q.Get("client_id") != "client" || parsed.Fragment != "fragment" {
		t.Fatal("URL contract mismatch")
	}
	if second, _ := ready.AttachToURL(got); second != got {
		t.Fatal("attachment is not idempotent")
	}
	for _, raw := range []string{"%", "https://example.test/?bad=%zz", "https://example.test/?a=1;b=2", "file:///tmp/file", "/relative", "https://user:secret@example.test/"} {
		if got, ok := ready.AttachToURL(raw); ok || got != raw {
			t.Fatal("invalid URL did not fail open")
		}
	}
	for _, result := range []Result{{}, {State: StateTimeout}, {State: StateError}, ReadyResultForTest(""), ReadyResultForTest("bad\nvalue")} {
		if got, ok := result.AttachToURL(raw); ok || got != raw {
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

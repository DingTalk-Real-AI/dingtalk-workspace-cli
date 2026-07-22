// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chatmsg

import (
	"strings"
	"testing"
)

func TestSender(t *testing.T) {
	// The display name lives under the bare "sender" key.
	if got := Sender(map[string]any{"sender": "念晨", "senderOpenDingTalkId": "D1"}); got != "念晨" {
		t.Fatalf("sender = %v, want 念晨", got)
	}
	// Falls back to the open id when no display name is present.
	if got := Sender(map[string]any{"senderOpenDingTalkId": "DXYZ"}); got != "DXYZ" {
		t.Fatalf("sender fallback = %v, want DXYZ", got)
	}
	// forwardMessages entries carry the literal string "null" — treat as absent.
	if got := Sender(map[string]any{"sender": "null"}); got != nil {
		t.Fatalf("sender \"null\" = %v, want nil", got)
	}
	if got := Sender(map[string]any{"sender": "null", "senderName": "念晨"}); got != "念晨" {
		t.Fatalf("sender \"null\" fallthrough = %v, want 念晨", got)
	}
	// A nested {name:…} sender object yields its display name, not the raw map.
	if got := Sender(map[string]any{"sender": map[string]any{"name": "Alice"}}); got != "Alice" {
		t.Fatalf("nested sender = %v, want Alice", got)
	}
	// A nested sender object with no usable name must not block the fallback.
	if got := Sender(map[string]any{"sender": map[string]any{"foo": "bar"}, "senderName": "Bob"}); got != "Bob" {
		t.Fatalf("nested-no-name fallthrough = %v, want Bob", got)
	}
	// A scalar numeric id is returned as-is.
	if got := Sender(map[string]any{"senderId": float64(42)}); got != float64(42) {
		t.Fatalf("numeric sender id = %v", got)
	}
}

func TestCleanText(t *testing.T) {
	// Out-of-office auto-reply: readable body lives in items[].data.text; the
	// decorative preview/config JSON lines and "empty" placeholder are dropped.
	autoReply := "* 仅你和对方可见\n" +
		`[{"text":{"minSupportVersion":"1.1","translateMap":{},"version":"1.2","items":[{"fallbackKey":"","data":{"text":"你好，我在出差中，消息回复可能不及时。"},"style":{"size":15,"bold":0},"type":"text"}]},"type":"markdown"}]` + "\n" +
		`{"previewUrl":"dingtalk://x","title":{"text":"自动回复","type":"text"}}` + "\n" +
		"empty\n" +
		`{"autoLayout":false,"enableForward":false}`
	if got, want := CleanText(autoReply), "* 仅你和对方可见\n你好，我在出差中，消息回复可能不及时。"; got != want {
		t.Fatalf("auto-reply cleaned = %q, want %q", got, want)
	}

	// P1 regression: ordinary text whose middle line is a JSON fragment (no
	// rich-content block anywhere) must be returned VERBATIM, not rewritten.
	mixed := "payload:\n{\"approved\":false}\nplease check"
	if got := CleanText(mixed); got != mixed {
		t.Fatalf("mixed text was rewritten: got %q, want %q", got, mixed)
	}

	// An ordinary JSON line must also survive when a different line contains a
	// recognised rich-content block. Card mode is not permission to discard
	// unrelated user-authored JSON.
	richAndPlain := `[{"items":[{"data":{"text":"卡片正文"}}]}]` + "\n" +
		`{"approved":false}`
	if got, want := CleanText(richAndPlain), "卡片正文\n{\"approved\":false}"; got != want {
		t.Fatalf("mixed rich/plain JSON was rewritten: got %q, want %q", got, want)
	}

	// Malformed items (non-map item, item whose "data" isn't a map) are skipped;
	// only the well-formed item's text is extracted.
	blob := `[{"items":["notmap",{"data":"notmap"},{"data":{"text":"有效正文"}}]}]`
	if got := CleanText(blob); got != "有效正文" {
		t.Fatalf("CleanText rich edge = %q, want 有效正文", got)
	}

	tests := map[string]string{
		"上周五 7.1 KW": "上周五 7.1 KW",
		"上周客户统计的[图片消息](mediaId=@lQ)":                              "上周客户统计的[图片消息](mediaId=@lQ)",
		"[文件] 简历.pdf fileId: qnY 注意：如需下载使用dws drive download命令下载": "[文件] 简历.pdf fileId: qnY 注意：如需下载使用dws drive download命令下载",
		"[讨论] 排期\n明天开会":                                           "[讨论] 排期\n明天开会",
		// a lone JSON object that isn't a rich-content block is left untouched
		`{"autoLayout":false,"enableForward":false}`: `{"autoLayout":false,"enableForward":false}`,
	}
	for in, want := range tests {
		if got := CleanText(in); got != want {
			t.Errorf("CleanText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsEncryptedAndMarker(t *testing.T) {
	cipher := "SwzNkAraDE6lUHUNlVT3mjFdbxL6dWvmt77XtjACdpJx9VFibzTbW9KtDbkzGOYP\n" +
		"7oDptklFO+YzDltH+myErV6rkc8URHYykpeSDsMP6kznFa9E320NsIntfY771dx+\n" +
		"||2||1||196"
	if !IsEncrypted(cipher) {
		t.Fatalf("ciphertext not detected: %q", cipher)
	}
	if got := CleanText(cipher); !strings.Contains(got, "加密消息") || strings.Contains(got, "||2||1||") {
		t.Fatalf("encrypted cleaned = %q, want marker not ciphertext", got)
	}
	for _, s := range []string{
		"上周五 7.1 KW",
		"价格 100||2||1||3",
		"[图片消息](mediaId=@lQLPJwDw3VmNDcfMos0DhLB3OHPQeTBlzgov2Oi1ly4A)",
		"大哥，我看了一下我觉得有几个点可以关注一下",
		strings.Repeat("好", 20) + "||2||1||1", // long CJK body + trailer, not base64
	} {
		if IsEncrypted(s) {
			t.Errorf("false positive: %q flagged as encrypted", s)
		}
	}
}

func TestText(t *testing.T) {
	if got := Text(map[string]any{"content": "你好"}); got != "你好" {
		t.Errorf("Text string = %v", got)
	}
	if got := Text(map[string]any{"content": map[string]any{"text": "嵌套"}}); got != "嵌套" {
		t.Errorf("Text nested = %v", got)
	}
	if got := Text(map[string]any{"plainText": "纯文本"}); got != "纯文本" {
		t.Errorf("Text plainText = %v", got)
	}
	if got := Text(map[string]any{"foo": 1}); got != nil {
		t.Errorf("Text none = %v, want nil", got)
	}
}

func TestCreateTime(t *testing.T) {
	if got := CreateTime(map[string]any{"sendTime": "2026-07-19 13:37:03"}); got != "2026-07-19 13:37:03" {
		t.Errorf("CreateTime = %v", got)
	}
	if got := CreateTime(map[string]any{}); got != nil {
		t.Errorf("CreateTime empty = %v, want nil", got)
	}
}

func TestForwarded(t *testing.T) {
	var project func(m map[string]any) map[string]any
	project = func(m map[string]any) map[string]any {
		row := map[string]any{"text": Text(m)}
		if fwd := Forwarded(m, project); len(fwd) > 0 { // recurse
			row["forwarded"] = fwd
		}
		return row
	}
	if got := Forwarded(map[string]any{"content": "x"}, project); got != nil {
		t.Errorf("Forwarded none = %v", got)
	}
	fwd := Forwarded(map[string]any{
		"forwardMessages": []any{
			map[string]any{"content": "a"},
			"not-a-map",
			map[string]any{"content": "b", "forwardMessages": []any{
				map[string]any{"content": "nested"},
			}},
		},
	}, project)
	if len(fwd) != 2 || fwd[0]["text"] != "a" || fwd[1]["text"] != "b" {
		t.Fatalf("Forwarded = %#v", fwd)
	}
	nested, ok := fwd[1]["forwarded"].([]map[string]any)
	if !ok || len(nested) != 1 || nested[0]["text"] != "nested" {
		t.Errorf("nested forwarded = %#v", fwd[1]["forwarded"])
	}
}

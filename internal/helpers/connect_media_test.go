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

package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPictureDownloadCode(t *testing.T) {
	cases := []struct {
		name    string
		content interface{}
		want    string
	}{
		{"downloadCode", map[string]interface{}{"downloadCode": "dc-1"}, "dc-1"},
		{"pictureDownloadCode fallback", map[string]interface{}{"pictureDownloadCode": "dc-2"}, "dc-2"},
		{"blank ignored", map[string]interface{}{"downloadCode": "  "}, ""},
		{"wrong type", "not-a-map", ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := pictureDownloadCode(tc.content); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRichTextPictureDownloadCodes(t *testing.T) {
	content := map[string]interface{}{"richText": []interface{}{
		map[string]interface{}{"text": "这个问题示例图如下："},
		map[string]interface{}{
			"pictureDownloadCode": "picture-fallback",
			"downloadCode":        "image-1",
			"type":                "picture",
		},
		map[string]interface{}{"text": "中间文字"},
		map[string]interface{}{"pictureDownloadCode": "image-2", "type": "picture"},
		map[string]interface{}{"downloadCode": "not-a-picture", "type": "file"},
	}}

	got := richTextPictureDownloadCodes(content)
	want := []string{"image-1", "image-2"}
	if len(got) != len(want) {
		t.Fatalf("codes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("codes = %v, want %v", got, want)
		}
	}
}

func TestRichTextPictureDownloadCodesUnknownShape(t *testing.T) {
	for _, content := range []interface{}{
		nil,
		"plain text",
		map[string]interface{}{"richText": "not-an-array"},
		map[string]interface{}{"richText": []interface{}{map[string]interface{}{"text": "only text"}}},
	} {
		if got := richTextPictureDownloadCodes(content); len(got) != 0 {
			t.Fatalf("content %v: codes = %v, want none", content, got)
		}
	}
}

func TestCallbackInboundMediaPreservesFutureRichTextAttachment(t *testing.T) {
	pictures, files, unknown := callbackInboundMedia("renamedRichEnvelope", map[string]interface{}{
		"richText": []interface{}{
			map[string]interface{}{"type": "text", "text": "附件如下"},
			map[string]interface{}{"type": "futureInlineBinary", "downloadCode": "inline-1", "fileName": "demo.mp4"},
		},
	})
	if len(pictures) != 0 || len(files) != 1 || files[0].DownloadCode != "inline-1" || files[0].MediaType != "video" || unknown != 0 {
		t.Fatalf("pictures=%v files=%#v unknown=%d", pictures, files, unknown)
	}
}

// TestExtractCallbackText covers the markdown / richText fallback path used
// when SDK data.Text.Content is empty. This is the recovery path for
// `dws chat message send --group ... --text ...` (defaults to msgType=markdown)
// which otherwise gets silently dropped by the connector.
func TestExtractCallbackText(t *testing.T) {
	cases := []struct {
		name    string
		content interface{}
		want    string
	}{
		{"raw string", "hello", "hello"},
		{"text field", map[string]interface{}{"text": "hi"}, "hi"},
		{"text preferred over title", map[string]interface{}{"title": "标题", "text": "body"}, "body"},
		{"content field", map[string]interface{}{"content": "raw"}, "raw"},
		{"markdown field", map[string]interface{}{"markdown": "**bold**"}, "**bold**"},
		{"title only", map[string]interface{}{"title": "只有标题"}, "只有标题"},
		{"richText array", map[string]interface{}{"richText": []interface{}{
			map[string]interface{}{"text": "part1 "},
			map[string]interface{}{"text": "part2"},
		}}, "part1 part2"},
		{"whitespace trimmed", map[string]interface{}{"text": "  spaced  "}, "spaced"},
		{"nil returns empty", nil, ""},
		{"unknown shape returns empty", map[string]interface{}{"foo": "bar"}, ""},
		{"empty text falls through", map[string]interface{}{"text": "", "content": "backup"}, "backup"},
		// interactiveCard (bot @-mentioning this bot): the real payload shape
		// captured live — body nested in cardContent[].children[].value.
		{"interactiveCard cardContent", map[string]interface{}{"cardContent": []interface{}{
			map[string]interface{}{"elementType": "RICHTEXT", "children": []interface{}{
				map[string]interface{}{"elementType": "TEXT", "value": "@claudecode 助手"},
				map[string]interface{}{"elementType": "TEXT", "value": " 请从 1 数到 10"},
			}},
		}}, "@claudecode 助手 请从 1 数到 10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractCallbackText(tc.content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRawCallbackPromptPreservesChatRecordPayload(t *testing.T) {
	content := map[string]interface{}{
		"title": "转发的聊天记录",
		"contents": []interface{}{
			map[string]interface{}{"senderName": "张三", "text": "请汇总本周风险"},
			map[string]interface{}{"senderName": "李四", "text": "发布窗口需要延期"},
		},
	}

	got := rawCallbackPrompt(" chatRecord ", content)
	for _, want := range []string{
		`msgtype="chatRecord"`,
		`"senderName":"张三"`,
		`"text":"请汇总本周风险"`,
		`"text":"发布窗口需要延期"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rawCallbackPrompt() missing %q:\n%s", want, got)
		}
	}
}

func TestRawCallbackPromptForUnknownEmptyPayload(t *testing.T) {
	got := rawCallbackPrompt("futureMessageType", nil)
	if !strings.Contains(got, `msgtype="futureMessageType"`) || !strings.HasSuffix(got, "\nnull") {
		t.Fatalf("rawCallbackPrompt() = %q, want message type and null JSON payload", got)
	}
}

func TestChatRecordInboundMediaExtractsEveryRecoverableAttachment(t *testing.T) {
	record := []interface{}{
		map[string]interface{}{"msgType": "picture", "downloadCode": "pic-1"},
		map[string]interface{}{"msgType": "picture", "downloadCode": "pic-1"}, // duplicate
		map[string]interface{}{"msgType": "audio", "downloadCode": "audio-1", "recognition": "语音转写"},
		map[string]interface{}{"msgType": "video", "downloadCode": "video-1", "fileName": "demo.mov"},
		map[string]interface{}{"msgType": "file", "downloadCode": "file-1", "fileName": "report.md"},
		map[string]interface{}{"msgType": "file", "dentryId": "123", "spaceId": "456", "fileName": "spec.pdf"},
		map[string]interface{}{"msgType": "unknownMsgType"},
		map[string]interface{}{"msgType": "unknownMsgType"},
		map[string]interface{}{"msgType": "text", "content": "请分析这些附件"},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}

	pictures, files, unknown := chatRecordInboundMedia(map[string]interface{}{"chatRecord": string(raw)})
	if len(pictures) != 1 || pictures[0] != "pic-1" {
		t.Fatalf("pictures = %v, want [pic-1]", pictures)
	}
	if unknown != 2 {
		t.Fatalf("unknownCount = %d, want 2", unknown)
	}
	if len(files) != 4 {
		t.Fatalf("files = %#v, want 4 attachments", files)
	}
	wants := []struct {
		mediaType string
		code      string
		name      string
	}{
		{"audio", "audio-1", "语音消息"},
		{"video", "video-1", "demo.mov"},
		{"file", "file-1", "report.md"},
		{"file", "", "spec.pdf"},
	}
	for i, want := range wants {
		if files[i].MediaType != want.mediaType || files[i].DownloadCode != want.code || files[i].FileName != want.name {
			t.Fatalf("files[%d] = %#v, want type=%q code=%q name=%q", i, files[i], want.mediaType, want.code, want.name)
		}
	}
	if files[3].DentryID != 123 || files[3].SpaceID != 456 {
		t.Fatalf("dentry attachment = %#v, want dentry=123 space=456", files[3])
	}
}

func TestChatRecordInboundMediaMatchesObservedDegradedCallback(t *testing.T) {
	record := `[{"msgType":"picture","downloadCode":"pic-live"},{"msgType":"unknownMsgType"},{"msgType":"unknownMsgType"},{"msgType":"text","content":"[合并的聊天记录]"},{"msgType":"unknownMsgType"}]`
	pictures, files, unknown := chatRecordInboundMedia(map[string]interface{}{"chatRecord": record})
	if len(pictures) != 1 || pictures[0] != "pic-live" || len(files) != 0 || unknown != 3 {
		t.Fatalf("pictures=%v files=%v unknown=%d, want one picture, no recoverable files, three unknowns", pictures, files, unknown)
	}
}

func TestChatRecordInboundMediaRecoversUnknownTypeWhenLocatorSurvives(t *testing.T) {
	record := `[{"msgType":"unknownMsgType","downloadCode":"opaque-1","fileName":"payload.bin"}]`
	pictures, files, unknown := chatRecordInboundMedia(map[string]interface{}{"chatRecord": record})
	if len(pictures) != 0 || len(files) != 1 || files[0].DownloadCode != "opaque-1" || files[0].FileName != "payload.bin" || unknown != 0 {
		t.Fatalf("pictures=%v files=%#v unknown=%d", pictures, files, unknown)
	}
}

func TestChatRecordInboundMediaPreservesFutureTypeWithLocator(t *testing.T) {
	record := `[{"msgType":"futureBinaryEnvelope","downloadCode":"future-1","fileName":"clip.webm"}]`
	pictures, files, unknown := chatRecordInboundMedia(map[string]interface{}{"chatRecord": record})
	if len(pictures) != 0 || len(files) != 1 || files[0].DownloadCode != "future-1" || files[0].MediaType != "video" || unknown != 0 {
		t.Fatalf("pictures=%v files=%#v unknown=%d", pictures, files, unknown)
	}
}

func TestCallbackInboundMediaDoesNotGateOnOuterMessageType(t *testing.T) {
	pictures, files, unknown := callbackInboundMedia("futureAttachmentV2", map[string]interface{}{
		"downloadCode": "future-2",
		"fileName":     "voice.ogg",
	})
	if len(pictures) != 0 || len(files) != 1 || files[0].DownloadCode != "future-2" || files[0].MediaType != "audio" || unknown != 0 {
		t.Fatalf("pictures=%v files=%#v unknown=%d", pictures, files, unknown)
	}
}

func TestCallbackInboundMediaFindsChatRecordByShape(t *testing.T) {
	content := map[string]interface{}{
		"chatRecord": `[{"msgType":"picture","downloadCode":"nested-picture"},{"msgType":"futureFile","downloadCode":"nested-file","fileName":"notes.md"},{"msgType":"unknownMsgType"}]`,
	}
	pictures, files, unknown := callbackInboundMedia("renamedForwardEnvelope", content)
	if len(pictures) != 1 || pictures[0] != "nested-picture" {
		t.Fatalf("pictures=%v, want [nested-picture]", pictures)
	}
	if len(files) != 1 || files[0].DownloadCode != "nested-file" || files[0].FileName != "notes.md" {
		t.Fatalf("files=%#v, want nested-file", files)
	}
	if unknown != 1 {
		t.Fatalf("unknown=%d, want 1", unknown)
	}
}

func TestHasChatRecordPayloadUsesShapeNotMessageType(t *testing.T) {
	if !hasChatRecordPayload(map[string]interface{}{
		"title":      "转发记录",
		"chatRecord": `[{"msgType":"text","content":"hello"}]`,
	}) {
		t.Fatal("chatRecord JSON string should be detected by payload shape")
	}
	if hasChatRecordPayload(map[string]interface{}{"title": "普通卡片"}) {
		t.Fatal("ordinary title payload must not be detected as a chat record")
	}
}

func TestChatRecordInboundMediaAcceptsDecodedContents(t *testing.T) {
	pictures, files, unknown := chatRecordInboundMedia(map[string]interface{}{
		"contents": []interface{}{
			map[string]interface{}{"type": "image", "pictureDownloadCode": "pic-2"},
			map[string]interface{}{"type": "voice", "downloadCode": "voice-2"},
		},
	})
	if len(pictures) != 1 || pictures[0] != "pic-2" || len(files) != 1 || files[0].MediaType != "audio" || unknown != 0 {
		t.Fatalf("pictures=%v files=%#v unknown=%d", pictures, files, unknown)
	}
}

// TestExtractInteractiveCardText covers the bot→bot @ card: the leading
// mention leaf (whose display name may contain spaces) is dropped by leaf
// boundary, leaving the clean instruction.
func TestExtractInteractiveCardText(t *testing.T) {
	card := func(leaves ...string) interface{} {
		kids := make([]interface{}, 0, len(leaves))
		for _, l := range leaves {
			kids = append(kids, map[string]interface{}{"elementType": "TEXT", "value": l})
		}
		return map[string]interface{}{"cardContent": []interface{}{
			map[string]interface{}{"elementType": "RICHTEXT", "children": kids},
		}}
	}
	cases := []struct {
		name    string
		content interface{}
		want    string
	}{
		{"mention leaf dropped", card("@claudecode 助手", " 请从 1 数到 10"), "请从 1 数到 10"},
		{"no mention", card("直接说的话"), "直接说的话"},
		{"only mention", card("@claudecode 助手"), ""},
		{"non-map content", "plain", ""},
		{"nil content", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractInteractiveCardText(tc.content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDownloadMessageFile drives the full resolve-then-fetch flow against a
// fake API: token → messageFiles/download (must carry robotCode+downloadCode)
// → presigned GET → local temp file.
func TestDownloadMessageFile(t *testing.T) {
	var gotDownloadReq map[string]any
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-acs-dingtalk-access-token") != "tok-1" {
			t.Error("missing access token header")
		}
		_ = json.NewDecoder(r.Body).Decode(&gotDownloadReq)
		_ = json.NewEncoder(w).Encode(map[string]any{"downloadUrl": srv.URL + "/file"})
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	path, err := c.downloadMessageFile(context.Background(), "ding-client", "dc-1")
	if err != nil {
		t.Fatalf("downloadMessageFile: %v", err)
	}
	defer os.Remove(path)

	if gotDownloadReq["robotCode"] != "ding-client" || gotDownloadReq["downloadCode"] != "dc-1" {
		t.Fatalf("download request payload = %v", gotDownloadReq)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("path = %q, want .png suffix from content type", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "PNGDATA" {
		t.Fatalf("saved file = %q, %v", raw, err)
	}
}

func TestDownloadMessageFileNamedPreservesOriginalExtension(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"downloadUrl": srv.URL + "/opaque.file"})
	})
	mux.HandleFunc("/opaque.file", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("MOVDATA"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	path, err := c.downloadMessageFileNamed(context.Background(), "ding-client", "video-code", "screen.mov")
	if err != nil {
		t.Fatalf("downloadMessageFileNamed: %v", err)
	}
	defer os.Remove(path)
	if !strings.HasSuffix(path, ".mov") {
		t.Fatalf("path = %q, want original .mov extension", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "MOVDATA" {
		t.Fatalf("saved file = %q, %v", raw, err)
	}
}

func TestDownloadMessageFileRejectsKnownOversizePayload(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"downloadUrl": srv.URL + "/too-large"})
	})
	mux.HandleFunc("/too-large", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", mediaMaxDownloadBytes+1))
		w.WriteHeader(http.StatusOK)
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	if _, err := c.downloadMessageFileNamed(context.Background(), "ding-client", "large-code", "large.mov"); err == nil || !strings.Contains(err.Error(), "文件过大") {
		t.Fatalf("oversize download error = %v, want explicit size rejection", err)
	}
}

func TestDownloadMessageFileNoURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/robot/messageFiles/download", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	if _, err := c.downloadMessageFile(context.Background(), "ding-client", "dc-1"); err == nil {
		t.Fatal("missing downloadUrl should error")
	}
}

func TestMediaExt(t *testing.T) {
	cases := []struct{ url, ct, want string }{
		{"https://x/y", "image/png", ".png"},
		{"https://x/y", "image/jpeg", ".jpg"},
		{"https://x/y.webp?sig=1", "", ".webp"},
		{"https://x/y", "", ".png"},
	}
	for _, tc := range cases {
		if got := mediaExt(tc.url, tc.ct); got != tc.want {
			t.Fatalf("mediaExt(%q, %q) = %q, want %q", tc.url, tc.ct, got, tc.want)
		}
	}
}

func TestConnectAttachmentMIMESniffsGenericVoiceFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "voice.bin")
	// Ogg capture pattern plus a minimal header is sufficient for
	// http.DetectContentType to identify the real container.
	if err := os.WriteFile(path, append([]byte("OggS\x00\x02"), make([]byte, 506)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := connectAttachmentMIME(path); got != "application/ogg" {
		t.Fatalf("connectAttachmentMIME() = %q, want application/ogg", got)
	}
}

// TestParseFileInbound covers both callback shapes so the regression that
// dropped every API-sent file (dentryId/spaceId, no downloadCode) can't
// silently return: (a) client-sent shape carrying downloadCode + fileName,
// (b) API-sent shape carrying dentryId + spaceId as JSON string OR number,
// (c) mixed / unknown / nil shapes must degrade to hasActionable()=false.
func TestParseFileInbound(t *testing.T) {
	cases := []struct {
		name           string
		content        interface{}
		wantCode       string
		wantName       string
		wantDentry     int64
		wantSpace      int64
		wantActionable bool
	}{
		{
			name:           "client-sent downloadCode",
			content:        map[string]interface{}{"downloadCode": "dc-1", "fileName": "screenshot.png"},
			wantCode:       "dc-1",
			wantName:       "screenshot.png",
			wantActionable: true,
		},
		{
			name:           "client-sent fileDownloadCode alias",
			content:        map[string]interface{}{"fileDownloadCode": "dc-2", "fileName": "log.txt"},
			wantCode:       "dc-2",
			wantName:       "log.txt",
			wantActionable: true,
		},
		{
			name: "media locator fields",
			content: map[string]interface{}{
				"mediaId":            "media-1",
				"openMessageId":      "message-1",
				"openConversationId": "conversation-1",
				"fileName":           "recording.m4a",
			},
			wantName:       "recording.m4a",
			wantActionable: true,
		},
		{
			name: "API-sent dentryId/spaceId as numbers",
			content: map[string]interface{}{
				"dentryId": float64(123456789),
				"spaceId":  float64(987654321),
				"fileName": "report.pdf",
			},
			wantName:       "report.pdf",
			wantDentry:     123456789,
			wantSpace:      987654321,
			wantActionable: true,
		},
		{
			name: "API-sent dentryId/spaceId as strings (real callback shape)",
			content: map[string]interface{}{
				"dentryId": "11111",
				"spaceId":  "22222",
				"fileName": "spec.docx",
			},
			wantName:       "spec.docx",
			wantDentry:     11111,
			wantSpace:      22222,
			wantActionable: true,
		},
		{
			name:           "default fileName when missing",
			content:        map[string]interface{}{"downloadCode": "dc-3"},
			wantCode:       "dc-3",
			wantName:       "未知文件",
			wantActionable: true,
		},
		{
			name:           "no downloadCode + only dentry (missing space) is NOT actionable",
			content:        map[string]interface{}{"dentryId": float64(1), "fileName": "x"},
			wantName:       "x",
			wantDentry:     1,
			wantActionable: false,
		},
		{
			name:           "wrong type returns empty",
			content:        "not-a-map",
			wantName:       "",
			wantActionable: false,
		},
		{
			name:           "nil returns empty",
			content:        nil,
			wantName:       "",
			wantActionable: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFileInbound(tc.content)
			if got.DownloadCode != tc.wantCode {
				t.Errorf("DownloadCode = %q, want %q", got.DownloadCode, tc.wantCode)
			}
			if got.FileName != tc.wantName {
				t.Errorf("FileName = %q, want %q", got.FileName, tc.wantName)
			}
			if got.DentryID != tc.wantDentry {
				t.Errorf("DentryID = %d, want %d", got.DentryID, tc.wantDentry)
			}
			if got.SpaceID != tc.wantSpace {
				t.Errorf("SpaceID = %d, want %d", got.SpaceID, tc.wantSpace)
			}
			if got.hasActionable() != tc.wantActionable {
				t.Errorf("hasActionable() = %v, want %v", got.hasActionable(), tc.wantActionable)
			}
		})
	}
}

// TestFileDownloadInfoBackCompat verifies the legacy two-value wrapper still
// works so any other caller relying on it isn't broken by the refactor.
func TestFileDownloadInfoBackCompat(t *testing.T) {
	code, name := fileDownloadInfo(map[string]interface{}{
		"downloadCode": "dc-legacy",
		"fileName":     "legacy.doc",
	})
	if code != "dc-legacy" || name != "legacy.doc" {
		t.Fatalf("legacy wrapper = %q/%q, want dc-legacy/legacy.doc", code, name)
	}
}

func TestSummarizeContent(t *testing.T) {
	if got := summarizeContent(nil); got != "<nil>" {
		t.Fatalf("nil summary = %q", got)
	}
	got := summarizeContent(map[string]interface{}{"dentryId": "1", "fileName": "x"})
	if !strings.Contains(got, "dentryId") || !strings.Contains(got, "fileName") {
		t.Fatalf("summary %q missing keys", got)
	}
}

func TestGetUserUnionID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v1.0/contact/users/user-123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("want GET, got %s", r.Method)
		}
		if r.Header.Get("x-acs-dingtalk-access-token") != "tok-1" {
			t.Error("missing access token")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"unionId": "union-abc"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	uid, err := c.getUserUnionID(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("getUserUnionID: %v", err)
	}
	if uid != "union-abc" {
		t.Fatalf("unionId = %q, want union-abc", uid)
	}
}

func TestDownloadDentryFile(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v1.0/oauth2/accessToken", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-1", "expireIn": 7200})
	})
	mux.HandleFunc("/v2.0/storage/spaces/999/dentries/123/getDownloadInfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["unionId"] != "union-abc" {
			t.Errorf("unionId = %v, want union-abc", body["unionId"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceUrl": srv.URL + "/dl/report.pdf",
		})
	})
	mux.HandleFunc("/dl/report.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-fake"))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()
	withCardAPIBase(t, srv.URL)

	c := newAICardClient("ding-client", "ding-secret", "")
	path, err := c.downloadDentryFile(context.Background(), 999, 123, "union-abc", "report.pdf")
	if err != nil {
		t.Fatalf("downloadDentryFile: %v", err)
	}
	defer os.Remove(path)

	if !strings.HasSuffix(path, ".pdf") {
		t.Fatalf("path = %q, want .pdf suffix", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "%PDF-fake" {
		t.Fatalf("file content = %q, %v", raw, err)
	}
}

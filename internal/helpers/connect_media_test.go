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
	"net/http"
	"net/http/httptest"
	"os"
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

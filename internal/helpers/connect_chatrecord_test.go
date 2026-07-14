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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestChatRecordUnknownIndexesMatchesObservedCallback(t *testing.T) {
	content := map[string]interface{}{
		"chatRecord": `[{"msgType":"picture","downloadCode":"pic-live"},{"msgType":"unknownMsgType"},{"msgType":"unknownMsgType"},{"msgType":"text","content":"[合并的聊天记录]"},{"msgType":"unknownMsgType"}]`,
	}
	if got, want := chatRecordUnknownIndexes(content), []int{1, 2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown indexes = %v, want %v", got, want)
	}
}

func TestRecoverChatRecordUnknownsRestoresAudioVideoAndFile(t *testing.T) {
	outer := `{
  "result": {"messages": [{
    "openMessageId": "outer-1",
    "openConversationId": "outer-conv",
    "createTime": "2026-07-14 16:16:44",
    "forwardMessages": [
      {"openMessageId":"pic-1","openConversationId":"source-conv","createTime":"2026-07-14 14:33:15","content":"[图片消息](mediaId=picture-media) 注意：如需下载使用dws chat message download-media命令下载"},
      {"openMessageId":"audio-1","openConversationId":"source-conv","createTime":"2026-07-14 14:33:19","content":"你是谁？你是谁\n[语音消息](mediaId=audio-media) 注意：如需下载使用dws chat message download-media命令下载"},
      {"openMessageId":"video-1","openConversationId":"outer-conv","createTime":"2026-07-14 14:33:36","content":"[文件] 录屏.mov"},
      {"openMessageId":"text-1","openConversationId":"source-conv","createTime":"2026-07-14 14:34:47","content":"普通文本"},
      {"openMessageId":"file-1","openConversationId":"outer-conv","createTime":"2026-07-14 14:34:57","content":"[文件] 巡检报告(1)(1).md"}
    ]
  }]}
}`
	sourceConversation := `{
  "result": {"messages": [
    {"openMessageId":"video-1","openConversationId":"source-conv","createTime":"2026-07-14 14:33:36","content":"[文件] 录屏.mov fileId: video-file-id 注意：如需下载使用dws drive download命令下载"},
    {"openMessageId":"file-1","openConversationId":"source-conv","createTime":"2026-07-14 14:34:57","content":"[文件] 巡检报告.md fileId: markdown-file-id 注意：如需下载使用dws drive download命令下载"}
  ]}
}`

	var calls []string
	call := func(_ context.Context, server, tool string, args map[string]any) (string, error) {
		calls = append(calls, server+"."+tool)
		switch tool {
		case "list_messages_by_ids":
			if server != "im" || !reflect.DeepEqual(args["openMsgIds"], []string{"outer-1"}) {
				t.Fatalf("list_messages_by_ids route/args = %s %#v", server, args)
			}
			return outer, nil
		case "list_conversation_message_v2":
			if server != "chat" || args["forward"] != true || args["limit"] != 50 {
				t.Fatalf("list_conversation_message_v2 route/args = %s %#v", server, args)
			}
			if args["openconversation_id"] == "source-conv" {
				return sourceConversation, nil
			}
			return `{"result":{"messages":[]}}`, nil
		default:
			return "", fmt.Errorf("unexpected tool %s", tool)
		}
	}

	enrichment, err := recoverChatRecordUnknowns(context.Background(), chatRecordLookup{
		MsgID:          "outer-1",
		UnknownIndexes: []int{1, 2, 4},
	}, call)
	if err != nil {
		t.Fatal(err)
	}
	if enrichment.MissingCount != 0 || len(enrichment.Files) != 3 {
		t.Fatalf("enrichment = %#v, want 3 recovered files and no missing attachment", enrichment)
	}
	wants := []fileInboundInfo{
		{MediaID: "audio-media", OpenMessageID: "audio-1", OpenConversationID: "source-conv", FileName: "转发语音.bin", MediaType: "audio"},
		{FileID: "video-file-id", OpenMessageID: "video-1", OpenConversationID: "source-conv", FileName: "录屏.mov", MediaType: "video"},
		{FileID: "markdown-file-id", OpenMessageID: "file-1", OpenConversationID: "source-conv", FileName: "巡检报告.md", MediaType: "file"},
	}
	for i, want := range wants {
		if !reflect.DeepEqual(enrichment.Files[i], want) {
			t.Fatalf("files[%d] = %#v, want %#v", i, enrichment.Files[i], want)
		}
	}
	if !strings.Contains(enrichment.Prompt, "你是谁？你是谁") || !strings.Contains(enrichment.Prompt, "录屏.mov") || !strings.Contains(enrichment.Prompt, "巡检报告.md") {
		t.Fatalf("prompt did not preserve recovered user content: %q", enrichment.Prompt)
	}
	if strings.Contains(enrichment.Prompt, "mediaId") || strings.Contains(enrichment.Prompt, "fileId") || strings.Contains(enrichment.Prompt, "dws drive") {
		t.Fatalf("prompt leaked transport locators/instructions: %q", enrichment.Prompt)
	}
	if len(calls) != 3 || calls[0] != "im.list_messages_by_ids" {
		t.Fatalf("calls = %v, want outer lookup plus both candidate conversations", calls)
	}
}

func TestRecoverChatRecordUnknownsKeepsMetadataWhenFileLocatorMissing(t *testing.T) {
	outer := `{"result":{"messages":[{"openMessageId":"outer-1","forwardMessages":[{"openMessageId":"file-1","openConversationId":"wrong-conv","createTime":"2026-07-14 14:34:57","content":"[文件] report.md"}]}]}}`
	call := func(_ context.Context, _, tool string, _ map[string]any) (string, error) {
		if tool == "list_messages_by_ids" {
			return outer, nil
		}
		return `{"result":{"messages":[]}}`, nil
	}
	enrichment, err := recoverChatRecordUnknowns(context.Background(), chatRecordLookup{MsgID: "outer-1", UnknownIndexes: []int{0}}, call)
	if err != nil {
		t.Fatal(err)
	}
	if enrichment.MissingCount != 1 || len(enrichment.Files) != 0 || !strings.Contains(enrichment.Prompt, "report.md") {
		t.Fatalf("enrichment = %#v, want metadata prompt plus one honestly missing attachment", enrichment)
	}
}

func TestDownloadRecoveredChatRecordFileRoutesAndPreservesOriginalBytes(t *testing.T) {
	wantBody := []byte("original attachment bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Signed-Test") != "yes" {
			http.Error(w, "missing signed header", http.StatusForbidden)
			return
		}
		_, _ = w.Write(wantBody)
	}))
	defer server.Close()

	tests := []struct {
		name       string
		info       fileInboundInfo
		wantServer string
		wantTool   string
		wantArg    string
		wantValue  string
	}{
		{
			name:       "mediaId audio",
			info:       fileInboundInfo{MediaID: "media-1", OpenMessageID: "msg-1", OpenConversationID: "conv-1", FileName: "voice.amr", MediaType: "audio"},
			wantServer: "im", wantTool: "get_resource_download_url", wantArg: "resourceId", wantValue: "media-1",
		},
		{
			name:       "drive fileId video",
			info:       fileInboundInfo{FileID: "file-1", FileName: "video.mov", MediaType: "video"},
			wantServer: "drive", wantTool: "download_file", wantArg: "fileId", wantValue: "file-1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			call := func(_ context.Context, serverID, tool string, args map[string]any) (string, error) {
				if serverID != tc.wantServer || tool != tc.wantTool || args[tc.wantArg] != tc.wantValue {
					t.Fatalf("call = %s.%s %#v", serverID, tool, args)
				}
				return fmt.Sprintf(`{"result":{"resourceUrl":%q,"headers":{"X-Signed-Test":"yes"}}}`, server.URL), nil
			}
			client := &aiCardClient{httpClient: server.Client()}
			path, err := client.downloadRecoveredChatRecordFileWithCall(context.Background(), tc.info, call)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(path)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, wantBody) {
				t.Fatalf("downloaded bytes = %q, want %q", got, wantBody)
			}
			if filepath.Ext(path) != filepath.Ext(tc.info.FileName) {
				t.Fatalf("downloaded path = %q, want original extension from %q", path, tc.info.FileName)
			}
		})
	}
}

package helpers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatRecordParsingAndRecoveryEdges(t *testing.T) {
	if chatRecordEntries("not-map") != nil {
		t.Fatal("chatRecordEntries(non-map) != nil")
	}
	if got := chatRecordEntries(map[string]any{"chatRecord": "bad-json"}); got != nil {
		t.Fatalf("chatRecordEntries(bad JSON) = %v", got)
	}
	wantContents := []any{"x"}
	if got := chatRecordEntries(map[string]any{"contents": wantContents}); !reflect.DeepEqual(got, wantContents) {
		t.Fatalf("chatRecordEntries(contents) = %v", got)
	}
	content := map[string]any{"chatRecord": []any{
		"not-map",
		map[string]any{"type": "unknownMsgType", "downloadCode": "already-actionable"},
		map[string]any{"msgtype": "unknownMsgType"},
	}}
	if got := chatRecordUnknownIndexes(content); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("chatRecordUnknownIndexes() = %v", got)
	}

	empty, err := recoverChatRecordUnknowns(context.Background(), chatRecordLookup{}, nil)
	if err != nil || empty.Prompt != "" {
		t.Fatalf("recover(empty) = %#v, %v", empty, err)
	}
	lookup := chatRecordLookup{MsgID: "outer", UnknownIndexes: []int{0}}
	if _, err := recoverChatRecordUnknowns(context.Background(), lookup, func(context.Context, string, string, map[string]any) (string, error) {
		return "", errors.New("call")
	}); err == nil || !strings.Contains(err.Error(), "查询") {
		t.Fatalf("recover(call error) = %v", err)
	}
	if _, err := recoverChatRecordUnknowns(context.Background(), lookup, func(context.Context, string, string, map[string]any) (string, error) {
		return "bad", nil
	}); err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("recover(parse error) = %v", err)
	}
	if _, err := recoverChatRecordUnknowns(context.Background(), lookup, func(context.Context, string, string, map[string]any) (string, error) {
		return `{"result":{"messages":[]}}`, nil
	}); err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("recover(no outer) = %v", err)
	}
	if _, err := recoverChatRecordUnknowns(context.Background(), chatRecordLookup{MsgID: "different", UnknownIndexes: []int{4}}, func(context.Context, string, string, map[string]any) (string, error) {
		return `{"result":{"messages":[{"openMessageId":"only","forwardMessages":[]}]}}`, nil
	}); err == nil || !strings.Contains(err.Error(), "索引") {
		t.Fatalf("recover(index error) = %v", err)
	}

	if got := uniqueValidIndexes([]int{2, -1, 2, 0, 9}, 3); !reflect.DeepEqual(got, []int{0, 2}) {
		t.Fatalf("uniqueValidIndexes() = %v", got)
	}
	resolved := map[string]chatRecordMessage{"wanted": {OpenMessageID: "wanted", Content: "[文件] old"}}
	enrichForwardedFileLocators(context.Background(), []chatRecordMessage{{CreateTime: "invalid"}}, resolved, nil)
	all := []chatRecordMessage{{OpenMessageID: "wanted", OpenConversationID: "conv", CreateTime: "2026-01-01 00:00:00"}}
	callCount := 0
	enrichForwardedFileLocators(context.Background(), all, resolved, func(context.Context, string, string, map[string]any) (string, error) {
		callCount++
		if callCount == 1 {
			return "", errors.New("call")
		}
		return "bad", nil
	})
	if callCount != 1 {
		t.Fatalf("enrichment calls = %d", callCount)
	}
	enrichForwardedFileLocators(context.Background(), all, resolved, func(context.Context, string, string, map[string]any) (string, error) {
		return `{"result":{"messages":[{"openMessageId":"other","content":"[文件] x fileId: id"},{"openMessageId":"wanted","content":"plain"},{"openMessageId":"wanted","openConversationId":"source","createTime":"2026-01-01 00:01:00","content":"[文件] x fileId: id"}]}}`, nil
	})
	if chatRecordFileID(resolved["wanted"].Content) != "id" || resolved["wanted"].CreateTime == "" {
		t.Fatalf("resolved message = %#v", resolved["wanted"])
	}

	for _, test := range []struct {
		content string
		media   string
		name    string
	}{
		{"[图片消息](mediaId=p)", "image", "转发图片"},
		{"[语音消息](mediaId=a)", "audio", "转发语音.bin"},
		{"[视频消息](mediaId=v)", "video", "转发视频.bin"},
		{"[其他](mediaId=f)", "file", "转发媒体.bin"},
		{"[文件] file.mp3 fileId: x", "audio", "file.mp3"},
		{"[文件] file.jpg fileId: x", "image", "file.jpg"},
		{"[文件] file.mp4 fileId: x", "video", "file.mp4"},
		{"[文件] file.bin fileId: x", "file", "file.bin"},
	} {
		info, ok := recoveredForwardAttachment(chatRecordMessage{Content: test.content})
		if !ok || info.MediaType != test.media || info.FileName != test.name {
			t.Fatalf("recoveredForwardAttachment(%q) = %#v, %v", test.content, info, ok)
		}
	}
	if _, ok := recoveredForwardAttachment(chatRecordMessage{Content: "plain"}); ok {
		t.Fatal("plain text recovered as attachment")
	}
	if info, ok := recoveredForwardAttachment(chatRecordMessage{Content: "[文件]"}); !ok || info.FileName != "转发文件" {
		t.Fatalf("fallback filename = %#v, %v", info, ok)
	}
	if chatRecordMediaID("plain") != "" || chatRecordFileID("plain") != "" || chatRecordFileName("plain") != "" {
		t.Fatal("plain content produced a locator")
	}
	if got := humanChatRecordContent("(mediaId=x) fileId: y"); got != "[无法提取文字内容]" {
		t.Fatalf("humanChatRecordContent(locator only) = %q", got)
	}
}

func TestCrossPlatformCoverageChatRecordDownloadFailureEdges(t *testing.T) {
	client := &aiCardClient{httpClient: http.DefaultClient}
	if _, err := client.downloadRecoveredChatRecordFileWithCall(context.Background(), fileInboundInfo{}, nil); err == nil {
		t.Fatal("download without locator succeeded")
	}
	callErr := func(context.Context, string, string, map[string]any) (string, error) { return "", errors.New("call") }
	if _, err := client.downloadRecoveredChatRecordFileWithCall(context.Background(), fileInboundInfo{MediaID: "m"}, callErr); err == nil {
		t.Fatal("download call error = nil")
	}
	badInfo := func(context.Context, string, string, map[string]any) (string, error) { return "bad", nil }
	if _, err := client.downloadRecoveredChatRecordFileWithCall(context.Background(), fileInboundInfo{FileID: "f"}, badInfo); err == nil {
		t.Fatal("download parse error = nil")
	}
	if _, err := downloadConnectURLToTemp(context.Background(), nil, ":", nil, "x"); err == nil {
		t.Fatal("invalid download URL succeeded")
	}
	if _, err := downloadConnectURLToTemp(context.Background(), nil, "http://127.0.0.1:1", nil, "x"); err == nil {
		t.Fatal("failed download request succeeded")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			http.Error(w, "no", http.StatusBadRequest)
		case "/large":
			w.Header().Set("Content-Length", "999999999")
		default:
			_, _ = w.Write([]byte("body"))
		}
	}))
	defer server.Close()
	if _, err := downloadConnectURLToTemp(context.Background(), server.Client(), server.URL+"/status", nil, "x"); err == nil {
		t.Fatal("HTTP failure succeeded")
	}
	if _, err := downloadConnectURLToTemp(context.Background(), server.Client(), server.URL+"/large", nil, "x"); err == nil {
		t.Fatal("large download succeeded")
	}
	originalMkdir := chatRecordMkdirAll
	t.Cleanup(func() { chatRecordMkdirAll = originalMkdir })
	chatRecordMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if _, err := downloadConnectURLToTemp(context.Background(), server.Client(), server.URL, nil, "x"); err == nil {
		t.Fatal("download mkdir error = nil")
	}
	chatRecordMkdirAll = originalMkdir
	path, err := downloadConnectURLToTemp(context.Background(), server.Client(), server.URL, map[string]string{"X-Test": "yes"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
}

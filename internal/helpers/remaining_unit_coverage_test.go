// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/spf13/cobra"
)

type coverageFailingReader struct{}

func (coverageFailingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type coverageMediaClient struct {
	downloadPicture func(string) (string, error)
	downloadNamed   func(fileInboundInfo) (string, error)
	downloadRecover func(fileInboundInfo) (string, error)
	unionID         func(string) (string, error)
	downloadDentry  func(fileInboundInfo, string) (string, error)
}

type coverageStreamingAttachmentForwarder struct{ called bool }

func (*coverageStreamingAttachmentForwarder) label() string { return "streaming-attachment" }
func (*coverageStreamingAttachmentForwarder) forward(context.Context, string, string) (string, error) {
	return "plain", nil
}

type coverageErrorCaller struct {
	err    error
	result *edition.ToolResult
	dryRun bool
}

func (c *coverageErrorCaller) CallTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	return c.result, c.err
}
func (*coverageErrorCaller) Format() string { return "json" }
func (c *coverageErrorCaller) DryRun() bool { return c.dryRun }
func (*coverageErrorCaller) Fields() string { return "" }
func (*coverageErrorCaller) JQ() string     { return "" }
func (*coverageStreamingAttachmentForwarder) forwardWithAttachments(context.Context, string, string, []connectMediaAttachment) (string, error) {
	return "attachment", nil
}
func (*coverageStreamingAttachmentForwarder) canStream() bool { return true }
func (f *coverageStreamingAttachmentForwarder) forwardStreamWithAttachments(_ context.Context, _, _ string, _ []connectMediaAttachment, onDelta func(string)) (string, error) {
	f.called = true
	if onDelta != nil {
		onDelta("delta")
	}
	return "streamed", nil
}

func (c *coverageMediaClient) downloadMessageFile(_ context.Context, _, code string) (string, error) {
	return c.downloadPicture(code)
}

func (c *coverageMediaClient) downloadMessageFileNamed(_ context.Context, _, code, name string) (string, error) {
	return c.downloadNamed(fileInboundInfo{DownloadCode: code, FileName: name})
}

func (c *coverageMediaClient) downloadRecoveredChatRecordFile(_ context.Context, info fileInboundInfo) (string, error) {
	return c.downloadRecover(info)
}

func (c *coverageMediaClient) getUserUnionID(_ context.Context, userID string) (string, error) {
	return c.unionID(userID)
}

func (c *coverageMediaClient) downloadDentryFile(_ context.Context, spaceID, dentryID int64, unionID, name string) (string, error) {
	return c.downloadDentry(fileInboundInfo{SpaceID: spaceID, DentryID: dentryID, FileName: name}, unionID)
}

func TestCrossPlatformCoverageConnectMediaParsingRemainingEdges(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := connectAttachmentMIME(empty); got != "application/octet-stream" {
		t.Fatalf("empty MIME = %q", got)
	}

	pictures, files := richTextInboundMedia(map[string]any{"richText": []any{"not-an-object"}})
	if len(pictures) != 0 || len(files) != 0 {
		t.Fatalf("non-object rich text = %v, %v", pictures, files)
	}
	pictures, files, unknown := callbackInboundMedia("richText", map[string]any{"richText": []any{
		map[string]any{"type": "picture", "pictureDownloadCode": "inline-picture"},
	}})
	if len(pictures) != 1 || pictures[0] != "inline-picture" || len(files) != 0 || unknown != 0 {
		t.Fatalf("rich callback media = %v, %v, %d", pictures, files, unknown)
	}
	if got := parseTypedFileInbound("video", map[string]any{"downloadCode": "v"}); got.FileName != "视频消息" {
		t.Fatalf("video fallback = %#v", got)
	}
	for _, tc := range []struct {
		info fileInboundInfo
		want string
	}{
		{fileInboundInfo{MediaID: "m", OpenMessageID: "msg", OpenConversationID: "conv"}, "media:m:msg:conv"},
		{fileInboundInfo{FileID: "f"}, "file:f"},
		{fileInboundInfo{DentryID: 2, SpaceID: 1}, "dentry:1:2"},
		{fileInboundInfo{}, ""},
	} {
		if got := fileInboundKey(tc.info); got != tc.want {
			t.Fatalf("fileInboundKey(%#v) = %q", tc.info, got)
		}
	}

	if p, f, n := chatRecordInboundMedia(map[string]any{"chatRecord": "{"}); len(p) != 0 || len(f) != 0 || n != 0 {
		t.Fatalf("bad chat record = %v, %v, %d", p, f, n)
	}
	content := map[string]any{"chatRecord": []any{
		"not-an-object",
		map[string]any{"msgType": "file", "downloadCode": "dup", "fileName": "a.txt"},
		map[string]any{"msgType": "file", "downloadCode": "dup", "fileName": "a.txt"},
	}}
	_, files, _ = chatRecordInboundMedia(content)
	if len(files) != 1 {
		t.Fatalf("decoded chat record files = %#v", files)
	}
	if !hasChatRecordPayload(map[string]any{"chatRecord": []any{}}) {
		t.Fatal("decoded chatRecord payload was not detected")
	}

	pictures, files, _ = callbackInboundMedia("picture", map[string]any{
		"pictureDownloadCode": "same",
		"downloadCode":        "same",
		"fileName":            "same.png",
		"chatRecord": []any{
			map[string]any{"msgType": "picture", "pictureDownloadCode": "same"},
			map[string]any{"msgType": "file", "downloadCode": "file", "fileName": "a.txt"},
			map[string]any{"msgType": "file", "downloadCode": "file", "fileName": "a.txt"},
		},
	})
	if len(pictures) != 1 || len(files) != 1 {
		t.Fatalf("deduplicated callback media = %v, %#v", pictures, files)
	}
	if got := rawCallbackPrompt("custom", make(chan int)); !strings.Contains(got, "msgtype=\"custom\"") {
		t.Fatalf("raw callback fallback = %q", got)
	}
}

func TestCrossPlatformCoverageWriteCompleteMediaFileRemainingEdges(t *testing.T) {
	origCreate, origCopy, origRemove := mediaCreate, mediaCopy, mediaRemove
	t.Cleanup(func() {
		mediaCreate, mediaCopy, mediaRemove = origCreate, origCopy, origRemove
	})

	removed := 0
	mediaRemove = func(string) error { removed++; return nil }
	mediaCopy = func(io.Writer, io.Reader) (int64, error) { return 0, nil }
	mediaCreate = func(string) (*os.File, error) {
		f, err := os.CreateTemp(t.TempDir(), "closed")
		if err == nil {
			err = f.Close()
		}
		return f, err
	}
	if err := writeCompleteMediaFile("unused", strings.NewReader("x")); err == nil {
		t.Fatal("close error expected")
	}
	mediaCreate = origCreate
	mediaCopy = func(io.Writer, io.Reader) (int64, error) { return mediaMaxDownloadBytes + 1, nil }
	if err := writeCompleteMediaFile(filepath.Join(t.TempDir(), "large"), strings.NewReader("x")); err == nil {
		t.Fatal("oversized copy expected")
	}
	if removed < 2 {
		t.Fatalf("remove calls = %d", removed)
	}

	root := filepath.Join(os.TempDir(), "dws-connect-media")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "cleanup-test")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	mediaRemove = origRemove
	cleanupConnectMediaAttachments([]connectMediaAttachment{{LocalPath: path}})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleanup stat = %v", err)
	}
}

func TestCrossPlatformCoveragePivotAndInputValidationRemainingEdges(t *testing.T) {
	cases := []map[string]any{
		{"values": "not-array"},
		{"values": []any{map[string]any{}}},
		{"values": []any{map[string]any{"field": "x", "summarize_by": 1}}},
		{"rows": "not-array"},
		{"rows": []any{"not-object"}},
		{"collapse": "invalid"},
	}
	for i, props := range cases {
		if err := validatePivotTableProperties(props, false); err == nil {
			t.Fatalf("pivot case %d succeeded: %#v", i, props)
		}
	}
	if _, err := validatePivotField("bad", "field"); err == nil {
		t.Fatal("non-object pivot field succeeded")
	}
	if _, err := validatePivotField(map[string]any{"field": 1}, "field"); err == nil {
		t.Fatal("non-string pivot field succeeded")
	}
	if _, err := readPivotProperties("@"+filepath.Join(t.TempDir(), "missing"), false); err == nil {
		t.Fatal("missing pivot properties file succeeded")
	}

	cmd := &cobra.Command{}
	cmd.SetIn(coverageFailingReader{})
	cmd.Flags().String("table", "-", "")
	if _, err := readTableJSONFlag(cmd, "table"); err == nil {
		t.Fatal("stdin read failure succeeded")
	}
}

func TestCrossPlatformCoverageDangerousConfirmationRemainingEdges(t *testing.T) {
	if confirmDangerousAction(nil, "publish", "resource") {
		t.Fatal("nil command confirmed")
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.SetIn(strings.NewReader("y\n"))
	if !confirmDangerousAction(cmd, "publish", "resource") {
		t.Fatal("interactive y was rejected")
	}
	cmd = &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.SetIn(strings.NewReader("no\n"))
	if confirmDangerousAction(cmd, "publish", "resource") {
		t.Fatal("interactive no was accepted")
	}
}

func TestCrossPlatformCoverageProtectSheetMutationCommandPanics(t *testing.T) {
	assertPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		fn()
	}
	assertPanic("nil", func() { protectSheetMutationCommand(nil, "delete", "target") })
	assertPanic("duplicate", func() {
		protectSheetMutationCommand(&cobra.Command{Annotations: map[string]string{sheetMutationConfirmationGuardAnnotation: "true"}, RunE: func(*cobra.Command, []string) error { return nil }}, "delete", "target")
	})
	assertPanic("missing RunE", func() { protectSheetMutationCommand(&cobra.Command{Use: "leaf"}, "delete", "target") })
}

func TestCrossPlatformCoverageOpenCodeAttachmentFallbackNameAndStat(t *testing.T) {
	orig := generateConnectVideoStoryboard
	t.Cleanup(func() { generateConnectVideoStoryboard = orig })
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	storyboard := filepath.Join(t.TempDir(), "storyboard.jpg")
	generateConnectVideoStoryboard = func(context.Context, string) (string, error) { return storyboard, nil }
	prompt, attachments := prepareOpenCodeAttachments(context.Background(), video, []connectMediaAttachment{{LocalPath: video, MediaType: "video"}})
	if len(attachments) != 1 || attachments[0].FileName != "转发视频.storyboard.jpg" || !strings.Contains(prompt, storyboard) {
		t.Fatalf("prepared = %q, %#v", prompt, attachments)
	}
}

func TestCrossPlatformCoverageAssembleConnectTurnMediaBranches(t *testing.T) {
	origRecover := recoverChatRecordUnknownsForConnect
	t.Cleanup(func() { recoverChatRecordUnknownsForConnect = origRecover })
	fail := errors.New("download failed")
	client := &coverageMediaClient{
		downloadPicture: func(code string) (string, error) {
			if strings.HasPrefix(code, "ok") {
				return "/tmp/" + code + ".png", nil
			}
			return "", fail
		},
		downloadNamed: func(info fileInboundInfo) (string, error) {
			if strings.HasPrefix(info.DownloadCode, "ok") {
				return "/tmp/" + info.FileName, nil
			}
			return "", fail
		},
		downloadRecover: func(info fileInboundInfo) (string, error) {
			if info.MediaID == "ok" || info.FileID == "ok" {
				return "/tmp/" + info.FileName, nil
			}
			return "", fail
		},
		unionID: func(userID string) (string, error) {
			if userID == "union-fail" {
				return "", fail
			}
			return "union", nil
		},
		downloadDentry: func(info fileInboundInfo, _ string) (string, error) {
			switch info.DentryID {
			case 1:
				return "/tmp/" + info.FileName, nil
			case 2:
				return "", fail
			default:
				return "", nil
			}
		},
	}

	recoverChatRecordUnknownsForConnect = func(context.Context, chatRecordLookup, chatRecordToolCall) (chatRecordEnrichment, error) {
		return chatRecordEnrichment{}, fail
	}
	prompt, attachments := assembleConnectTurnMedia("", "client", "sender", client, []string{"bad"}, nil, []chatRecordLookup{{MsgID: "bad"}})
	if prompt == "" || len(attachments) != 0 {
		t.Fatalf("failed recovery/picture = %q, %#v", prompt, attachments)
	}
	assembleConnectTurnMedia("text", "client", "sender", client, []string{"bad"}, nil, nil)

	recoverChatRecordUnknownsForConnect = func(context.Context, chatRecordLookup, chatRecordToolCall) (chatRecordEnrichment, error) {
		return chatRecordEnrichment{Prompt: "recovered", Files: []fileInboundInfo{{DownloadCode: "ok-file", FileName: "recovered.txt"}}, MissingCount: 1}, nil
	}
	prompt, attachments = assembleConnectTurnMedia("", "client", "sender", client, []string{"ok-picture"}, []fileInboundInfo{{}}, []chatRecordLookup{{MsgID: "ok"}})
	if !strings.Contains(prompt, "recovered") || len(attachments) != 2 {
		t.Fatalf("successful recovery = %q, %#v", prompt, attachments)
	}
	assembleConnectTurnMedia("existing", "client", "sender", client, nil, nil, []chatRecordLookup{{MsgID: "ok"}})
	assembleConnectTurnMedia("", "client", "sender", client, []string{"ok-picture"}, nil, nil)

	files := []fileInboundInfo{
		{DownloadCode: "ok-image", FileName: "image.png", MediaType: "image"},
		{DownloadCode: "ok-audio", FileName: "audio.mp3", MediaType: "audio"},
		{DownloadCode: "ok-video", FileName: "video.mp4", MediaType: "video"},
		{DownloadCode: "bad", FileName: "failed.txt", MediaType: "file"},
		{MediaID: "ok", OpenMessageID: "message", OpenConversationID: "conversation", FileName: "recovered.bin", MediaType: "file"},
		{FileID: "bad", FileName: "missing.bin", MediaType: "file"},
	}
	prompt, attachments = assembleConnectTurnMedia("", "client", "sender", client, nil, files, nil)
	if prompt == "" || len(attachments) != 4 {
		t.Fatalf("file variants = %q, %#v", prompt, attachments)
	}

	dentry := func(id int64) fileInboundInfo {
		return fileInboundInfo{DentryID: id, SpaceID: 9, FileName: "disk.txt", FileType: "text", FileSize: 5}
	}
	prompt, attachments = assembleConnectTurnMedia("", "client", "sender", client, nil, []fileInboundInfo{dentry(1)}, nil)
	if prompt == "" || len(attachments) != 1 {
		t.Fatalf("dentry success = %q, %#v", prompt, attachments)
	}
	assembleConnectTurnMedia("existing", "client", "sender", client, nil, []fileInboundInfo{dentry(1)}, nil)
	for _, sender := range []string{"union-fail", "sender"} {
		for _, initial := range []string{"", "existing"} {
			prompt, attachments = assembleConnectTurnMedia(initial, "client", sender, client, nil, []fileInboundInfo{dentry(2)}, nil)
			if prompt == "" || len(attachments) != 0 {
				t.Fatalf("dentry fallback sender=%q initial=%q: %q, %#v", sender, initial, prompt, attachments)
			}
		}
	}
	for _, initial := range []string{"", "existing"} {
		prompt, attachments = assembleConnectTurnMedia(initial, "client", "sender", client, nil, []fileInboundInfo{{DownloadCode: "bad", FileName: "lost.txt"}}, nil)
		if prompt == "" || len(attachments) != 0 {
			t.Fatalf("ordinary fallback = %q, %#v", prompt, attachments)
		}
	}
}

func TestCrossPlatformCoverageChatRecordDownloadWrapperAndWriteEdges(t *testing.T) {
	origCall := chatRecordDownloadToolCall
	origCreate := mediaCreate
	t.Cleanup(func() {
		chatRecordDownloadToolCall = origCall
		mediaCreate = origCreate
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("image"))
	}))
	defer server.Close()
	chatRecordDownloadToolCall = func(context.Context, string, string, map[string]any) (string, error) {
		return `{"downloadUrl":"` + server.URL + `"}`, nil
	}
	client := &aiCardClient{httpClient: server.Client()}
	path, err := client.downloadRecoveredChatRecordFile(context.Background(), fileInboundInfo{FileID: "file"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if filepath.Ext(path) != ".png" {
		t.Fatalf("download extension = %q", path)
	}

	mediaCreate = func(string) (*os.File, error) { return nil, errors.New("create failed") }
	if _, err := downloadConnectURLToTemp(context.Background(), server.Client(), server.URL, nil, "file.txt"); err == nil {
		t.Fatal("media create failure succeeded")
	}
}

func TestCrossPlatformCoverageChatRecordEnrichmentInvalidConversationResponse(t *testing.T) {
	resolved := map[string]chatRecordMessage{"wanted": {OpenMessageID: "wanted", Content: "[文件] old"}}
	all := []chatRecordMessage{{OpenMessageID: "wanted", OpenConversationID: "conv", CreateTime: "2026-01-01 00:00:00"}}
	enrichForwardedFileLocators(context.Background(), all, resolved, func(context.Context, string, string, map[string]any) (string, error) {
		return "invalid", nil
	})
}

func TestCrossPlatformCoverageGeminiRemainingAttachmentEdges(t *testing.T) {
	origPollInterval := geminiFilePollInterval
	t.Cleanup(func() { geminiFilePollInterval = origPollInterval })
	f := &geminiAPIForwarder{model: "model", apiKey: "key", baseURL: "https://example.test/v1beta", timeout: 1, httpClient: http.DefaultClient}
	if _, err := f.forwardWithAttachments(context.Background(), "", "text", []connectMediaAttachment{{LocalPath: filepath.Join(t.TempDir(), "missing")}}); err == nil {
		t.Fatal("missing attachment forward succeeded")
	}

	large := filepath.Join(t.TempDir(), "large.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(geminiInlineRawLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	invalid := *f
	invalid.baseURL = "%"
	if _, err := invalid.partsWithAttachments(context.Background(), "text", []connectMediaAttachment{{LocalPath: large}}); err == nil {
		t.Fatal("large attachment upload failure succeeded")
	}
	if endpoint, err := (&geminiAPIForwarder{}).filesEndpoint(false); err != nil || endpoint == "" {
		t.Fatalf("default files endpoint = %q, %v", endpoint, err)
	}
	invalidName := *f
	invalidName.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("request should not run")
	})}
	geminiFilePollInterval = 0
	if _, err := invalidName.waitForUploadedFile(context.Background(), geminiUploadedFile{Name: "%", State: "PROCESSING"}); err == nil {
		t.Fatal("invalid poll file name succeeded")
	}
}

func TestCrossPlatformCoverageCallbackMediaCrossSourceDeduplication(t *testing.T) {
	content := map[string]any{
		"downloadCode": "same",
		"fileName":     "top.txt",
		"richText": []any{
			map[string]any{"type": "file", "downloadCode": "same", "fileName": "nested.txt"},
		},
	}
	_, files, _ := callbackInboundMedia("file", content)
	if len(files) != 1 {
		t.Fatalf("cross-source duplicate files = %#v", files)
	}
	pictures, files, _ := callbackInboundMedia("picture", map[string]any{"pictureDownloadCode": "picture", "downloadCode": "picture"})
	if len(pictures) != 1 || len(files) != 0 {
		t.Fatalf("picture/file duplicate = %v, %#v", pictures, files)
	}
}

func TestCrossPlatformCoverageForwarderAttachmentDispatchRemainingEdges(t *testing.T) {
	streaming := &coverageStreamingAttachmentForwarder{}
	if reply, err := forwardConnectTurn(context.Background(), streaming, "conv", "prompt", nil, func(string) {}); err != nil || reply != "streamed" || !streaming.called {
		t.Fatalf("streaming attachment dispatch = %q, %v, called=%v", reply, err, streaming.called)
	}

	bin := writeShellExecutable(t, t.TempDir(), "agent", "printf 'ok\\n'\n")
	for _, fwd := range []*execForwarder{
		{name: "custom", argv: []string{bin}},
		{name: "claudecode", argv: []string{bin}},
		{name: "workbuddy", argv: []string{bin}},
	} {
		attachments := []connectMediaAttachment(nil)
		if fwd.name == "workbuddy" {
			path := filepath.Join(t.TempDir(), "file.txt")
			attachments = []connectMediaAttachment{{LocalPath: ""}, {LocalPath: path}, {LocalPath: path}}
		}
		if reply, err := fwd.forwardWithAttachments(context.Background(), "conv", "prompt", attachments); err != nil || reply != "ok" {
			t.Fatalf("exec %s = %q, %v", fwd.name, reply, err)
		}
	}

	plainPrompt, plainAttachments := prepareConnectForwarderAttachments(&basicConnectorForwarder{}, "plain", []connectMediaAttachment{{LocalPath: "x"}})
	if plainPrompt != "plain" || len(plainAttachments) != 1 {
		t.Fatalf("plain prepare = %q, %#v", plainPrompt, plainAttachments)
	}
	origStoryboard := generateConnectVideoStoryboard
	t.Cleanup(func() { generateConnectVideoStoryboard = origStoryboard })
	generateConnectVideoStoryboard = func(context.Context, string) (string, error) { return "storyboard.jpg", nil }
	prompt, attachments := prepareConnectForwarderAttachments(&opencodeForwarder{}, "video.mp4", []connectMediaAttachment{{LocalPath: "video.mp4", FileName: "video.mp4", MediaType: "video"}})
	if !strings.Contains(prompt, "storyboard.jpg") || len(attachments) != 1 {
		t.Fatalf("opencode prepare = %q, %#v", prompt, attachments)
	}
}

func TestCrossPlatformCoverageQoderAttachmentForwardingRemainingEdges(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	fwd := &qoderStreamForwarder{name: "qoder", bin: "missing-qoder", timeout: 1}
	if _, err := fwd.forwardWithAttachments(cancelled, "conv", "text", nil); err == nil {
		t.Fatal("cancelled attachment-free qoder forward succeeded")
	}

	success := writeShellExecutable(t, t.TempDir(), "qoder-ok", "printf 'answer\\n'\n")
	sessions := newConvSessions("")
	fwd = &qoderStreamForwarder{name: "qoderwork", bin: success, timeout: 30 * time.Second, yolo: true, model: "model", sessions: sessions}
	if reply, err := fwd.forwardWithAttachments(context.Background(), "conv", "text", []connectMediaAttachment{{LocalPath: ""}, {LocalPath: "/tmp/file"}}); err != nil || reply == "" {
		t.Fatalf("qoder success = %q, %v", reply, err)
	}

	backend := writeShellExecutable(t, t.TempDir(), "qoder-backend", "printf 'API Error: rejected\\n'\n")
	fwd = &qoderStreamForwarder{name: "qoder", bin: backend, timeout: 30 * time.Second}
	if reply, err := fwd.forwardWithAttachments(context.Background(), "conv", "text", []connectMediaAttachment{{LocalPath: "/tmp/file"}}); err != nil || reply == "" {
		t.Fatalf("qoder backend error reply = %q, %v", reply, err)
	}

	failure := writeShellExecutable(t, t.TempDir(), "qoder-fail", "exit 1\n")
	fwd = &qoderStreamForwarder{name: "qoder", bin: failure, timeout: 30 * time.Second, sessions: sessions}
	if _, err := fwd.forwardWithAttachments(context.Background(), "conv", "text", []connectMediaAttachment{{LocalPath: "/tmp/file"}}); err == nil {
		t.Fatal("qoder process failure succeeded")
	}

	empty := writeShellExecutable(t, t.TempDir(), "qoder-empty", "exit 0\n")
	fwd = &qoderStreamForwarder{name: "qoder", bin: empty, timeout: 30 * time.Second}
	if reply, err := fwd.forwardWithAttachments(context.Background(), "conv", "text", []connectMediaAttachment{{LocalPath: "/tmp/file"}}); err != nil || reply == "" {
		t.Fatalf("qoder empty = %q, %v", reply, err)
	}
}

func TestCrossPlatformCoverageOpenCodeAndCodexAttachmentWrappers(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requestBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"ok"}]}`))
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &opencodeHTTPClient{baseURL: server.URL, httpClient: server.Client()}
	if reply, err := client.sendMessageWithAttachments(context.Background(), "session", "text", "", []connectMediaAttachment{{LocalPath: ""}, {LocalPath: file}}); err != nil || reply != "ok" {
		t.Fatalf("opencode attachments = %q, %v", reply, err)
	}
	if !strings.Contains(requestBody, filepath.Base(file)) {
		t.Fatalf("opencode request = %s", requestBody)
	}

	codex := &codexAppServerForwarder{bin: filepath.Join(t.TempDir(), "missing"), timeout: 1}
	if _, err := codex.forwardWithAttachments(context.Background(), "conv", "text", nil); err == nil {
		t.Fatal("codex wrapper unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageRunStreamConnectorChatRecordLookupBranches(t *testing.T) {
	origRecover := recoverChatRecordUnknownsForConnect
	t.Cleanup(func() { recoverChatRecordUnknownsForConnect = origRecover })
	recoverChatRecordUnknownsForConnect = func(context.Context, chatRecordLookup, chatRecordToolCall) (chatRecordEnrichment, error) {
		return chatRecordEnrichment{}, errors.New("lookup failed")
	}
	message := func(id string) *chatbot.BotCallbackDataModel {
		m := connectorMessage(id, "")
		m.MsgId = id
		m.Msgtype = "chatRecord"
		m.Content = map[string]any{"chatRecord": []any{map[string]any{"msgType": "unknownMsgType"}}}
		return m
	}
	fwd := &basicConnectorForwarder{reply: "done"}
	runConnectorScenario(t, []*chatbot.BotCallbackDataModel{message("outer"), message("")}, fwd, nil, nil, nil, 2)
	if len(fwd.prompts) != 2 {
		t.Fatalf("chat record prompts = %#v", fwd.prompts)
	}
}

func TestCrossPlatformCoverageRemainingCommandExecutionBranches(t *testing.T) {
	t.Run("aitable view with timeout", func(t *testing.T) {
		caller, err := runAitableExportCommand(t,
			"--base-id", "base", "--scope", "view", "--table-id", "table", "--view-id", "view",
			"--export-format", "excel", "--timeout-ms", "250",
		)
		if err != nil || len(caller.calls) != 1 || caller.calls[0].args["timeoutMs"] != 250 {
			t.Fatalf("aitable export = %#v, %v", caller.calls, err)
		}
	})

	t.Run("doc version confirmed revert", func(t *testing.T) {
		previous := deps
		t.Cleanup(func() { deps = previous })
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"versions":[{"version":3}]}`}, {text: `{}`}}}
		if err := runDocCoverageCommand(t, caller, "--yes", "version", "revert", "--node=node", "--version=3"); err != nil {
			t.Fatal(err)
		}
		if caller.index != 2 {
			t.Fatalf("doc caller index = %d", caller.index)
		}
	})

	t.Run("mail invalid parameter suggestion", func(t *testing.T) {
		previous := deps
		t.Cleanup(func() { deps = previous })
		caller := &coverageErrorCaller{err: &CLIError{Code: CodeMCPToolError, Message: "Invalid parameter"}}
		InitDeps(caller)
		cmd := newMailCommand()
		cmd.SetArgs([]string{"template", "update", "--email", "user@example.com", "--id", "id", "--subject", "updated"})
		err := cmd.Execute()
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || !strings.Contains(cliErr.Suggestion, "草稿") {
			t.Fatalf("mail update error = %#v", err)
		}
	})

	t.Run("sheet style dry run", func(t *testing.T) {
		previous := deps
		t.Cleanup(func() { deps = previous })
		batchPath := filepath.Join(t.TempDir(), "styles.json")
		if err := os.WriteFile(batchPath, []byte(`[{"sheetId":"Sheet1","range":"A1:B2","fontWeight":"bold"}]`), 0o600); err != nil {
			t.Fatal(err)
		}
		InitDeps(&coverageErrorCaller{err: errors.New("dry run failed"), dryRun: true})
		cmd := newRangeBatchSetStyleCmd()
		cmd.SetArgs([]string{"--node", "node", "--batch", batchPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("sheet style error = %v", err)
		}
	})

	t.Run("business error classification", func(t *testing.T) {
		previous := deps
		t.Cleanup(func() { deps = previous })
		InitDeps(&coverageErrorCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"success":false,"message":"bad"}`}}}})
		if err := callMCPTool("business", nil); err == nil {
			t.Fatal("business error was not classified")
		}
	})
}

func TestCrossPlatformCoverageAttachSheetConfirmationGuardMissingPath(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("missing Sheet path did not panic")
		}
	}()
	attachSheetConfirmationGuard(&cobra.Command{Use: "sheet"}, "missing leaf", "delete", "target")
}

func TestCrossPlatformCoverageDentryDownloadRejectsDeclaredOversize(t *testing.T) {
	orig := connectMediaCallRaw
	t.Cleanup(func() { connectMediaCallRaw = orig })
	connectMediaCallRaw = func(*aiCardClient, context.Context, string, string, map[string]any) (string, error) {
		return `{"resourceUrl":"https://download.test/file"}`, nil
	}
	client := &aiCardClient{httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: mediaMaxDownloadBytes + 1,
			Body:          io.NopCloser(strings.NewReader("")),
			Header:        make(http.Header),
		}, nil
	})}}
	if _, err := client.downloadDentryFile(context.Background(), 1, 2, "union", "file"); err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("oversize dentry error = %v", err)
	}
}

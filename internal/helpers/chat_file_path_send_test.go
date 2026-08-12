package helpers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type chatFilePathCall struct {
	server string
	tool   string
	args   map[string]any
}

type chatFilePathCaller struct {
	sequence []string
	calls    []chatFilePathCall
}

func (c *chatFilePathCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.sequence = append(c.sequence, tool)
	copied := make(map[string]any, len(args))
	for key, value := range args {
		copied[key] = value
	}
	c.calls = append(c.calls, chatFilePathCall{server: server, tool: tool, args: copied})

	switch tool {
	case "init_conversation_file_upload":
		return textToolResult(`{"resourceUrl":"https://upload.example/file","uploadKey":"upload-key","headers":{"x-upload":"yes"}}`), nil
	case "commit_conversation_file_upload":
		return textToolResult(`{"result":{"dentryId":123,"spaceId":456}}`), nil
	case "send_personal_message":
		return textToolResult(`{"success":true}`), nil
	default:
		return nil, fmt.Errorf("unexpected tool call %s/%s", server, tool)
	}
}

func (*chatFilePathCaller) Format() string { return "json" }
func (*chatFilePathCaller) DryRun() bool   { return false }
func (*chatFilePathCaller) Fields() string { return "" }
func (*chatFilePathCaller) JQ() string     { return "" }

func TestChatMessageSendFilePathUsesWukongUploadSequence(t *testing.T) {
	previousDeps, previousPut, previousArgs := deps, httpPutFile, os.Args
	t.Cleanup(func() {
		deps = previousDeps
		httpPutFile = previousPut
		os.Args = previousArgs
	})

	filePath := filepath.Join(t.TempDir(), "image.png")
	payload := []byte("png payload")
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	caller := &chatFilePathCaller{}
	commandArgs := []string{
		"message", "send",
		"--group=cid",
		"--msg-type=file",
		"--file-path=" + filePath,
	}
	os.Args = append([]string{"dws", "chat"}, commandArgs...)
	httpPutFile = func(_ context.Context, resourceURL string, headers map[string]string, localPath string, fileSize int64) error {
		caller.sequence = append(caller.sequence, "HTTP PUT")
		if resourceURL != "https://upload.example/file" {
			t.Fatalf("resourceURL = %q", resourceURL)
		}
		if headers["x-upload"] != "yes" {
			t.Fatalf("headers = %#v", headers)
		}
		if localPath != filePath || fileSize != int64(len(payload)) {
			t.Fatalf("upload file = %q (%d), want %q (%d)", localPath, fileSize, filePath, len(payload))
		}
		return nil
	}

	err := runChatCoverageCommand(t, caller, commandArgs...)
	if err != nil {
		t.Fatalf("chat message send --file-path: %v", err)
	}

	wantSequence := []string{
		"init_conversation_file_upload",
		"HTTP PUT",
		"commit_conversation_file_upload",
		"send_personal_message",
	}
	if !reflect.DeepEqual(caller.sequence, wantSequence) {
		t.Fatalf("call sequence = %#v, want %#v", caller.sequence, wantSequence)
	}
	if len(caller.calls) != 3 {
		t.Fatalf("tool calls = %#v, want init, commit, send", caller.calls)
	}

	fileMD5 := fmt.Sprintf("%x", md5.Sum(payload))
	wantInit := chatFilePathCall{
		server: "im",
		tool:   "init_conversation_file_upload",
		args: map[string]any{
			"openConversationId": "cid",
			"fileName":           "image.png",
			"fileSize":           int64(len(payload)),
			"md5":                fileMD5,
		},
	}
	if !reflect.DeepEqual(caller.calls[0], wantInit) {
		t.Fatalf("init call = %#v, want %#v", caller.calls[0], wantInit)
	}
	wantCommit := chatFilePathCall{
		server: "im",
		tool:   "commit_conversation_file_upload",
		args: map[string]any{
			"openConversationId": "cid",
			"uploadKey":          "upload-key",
			"fileName":           "image.png",
			"fileSize":           int64(len(payload)),
			"md5":                fileMD5,
		},
	}
	if !reflect.DeepEqual(caller.calls[1], wantCommit) {
		t.Fatalf("commit call = %#v, want %#v", caller.calls[1], wantCommit)
	}

	send := caller.calls[len(caller.calls)-1]
	if send.server != "chat" || send.tool != "send_personal_message" || send.args["msgType"] != "file" {
		t.Fatalf("send call = %#v", send)
	}
	if send.args["openConversationId"] != "cid" {
		t.Fatalf("send target = %#v", send.args["openConversationId"])
	}
	content, ok := send.args["content"].(string)
	if !ok {
		t.Fatalf("send content = %#v", send.args["content"])
	}
	var parsed struct {
		DentryID int64  `json:"dentryId"`
		SpaceID  int64  `json:"spaceId"`
		FileName string `json:"fileName"`
		FileType string `json:"fileType"`
		FilePath string `json:"filePath"`
		FileSize int64  `json:"fileSize"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("decode send content %q: %v", content, err)
	}
	if parsed.DentryID != 123 || parsed.SpaceID != 456 ||
		parsed.FileName != "image.png" || parsed.FileType != "png" ||
		parsed.FilePath != "/image.png" || parsed.FileSize != int64(len(payload)) {
		t.Fatalf("send content = %#v", parsed)
	}
}

func TestChatMessageSendFilePathUsesOpenDingTalkIDTarget(t *testing.T) {
	previousDeps, previousPut, previousArgs := deps, httpPutFile, os.Args
	t.Cleanup(func() {
		deps = previousDeps
		httpPutFile = previousPut
		os.Args = previousArgs
	})

	filePath := filepath.Join(t.TempDir(), "report.pdf")
	payload := []byte("pdf payload")
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	caller := &chatFilePathCaller{}
	commandArgs := []string{
		"message", "send",
		"--open-dingtalk-id=D-target",
		"--msg-type=file",
		"--file-path=" + filePath,
	}
	os.Args = append([]string{"dws", "chat"}, commandArgs...)
	httpPutFile = func(_ context.Context, resourceURL string, _ map[string]string, localPath string, fileSize int64) error {
		caller.sequence = append(caller.sequence, "HTTP PUT")
		if resourceURL != "https://upload.example/file" || localPath != filePath || fileSize != int64(len(payload)) {
			t.Fatalf("upload = %q, %q (%d)", resourceURL, localPath, fileSize)
		}
		return nil
	}

	if err := runChatCoverageCommand(t, caller, commandArgs...); err != nil {
		t.Fatalf("chat message send --open-dingtalk-id --file-path: %v", err)
	}
	if len(caller.calls) != 3 {
		t.Fatalf("tool calls = %#v, want init, commit, send", caller.calls)
	}

	fileMD5 := fmt.Sprintf("%x", md5.Sum(payload))
	wantInit := chatFilePathCall{
		server: "im",
		tool:   "init_conversation_file_upload",
		args: map[string]any{
			"openDingTalkId": "D-target",
			"fileName":       "report.pdf",
			"fileSize":       int64(len(payload)),
			"md5":            fileMD5,
		},
	}
	if !reflect.DeepEqual(caller.calls[0], wantInit) {
		t.Fatalf("init direct target call = %#v, want %#v", caller.calls[0], wantInit)
	}
	wantCommit := chatFilePathCall{
		server: "im",
		tool:   "commit_conversation_file_upload",
		args: map[string]any{
			"openDingTalkId": "D-target",
			"uploadKey":      "upload-key",
			"fileName":       "report.pdf",
			"fileSize":       int64(len(payload)),
			"md5":            fileMD5,
		},
	}
	if !reflect.DeepEqual(caller.calls[1], wantCommit) {
		t.Fatalf("commit direct target call = %#v, want %#v", caller.calls[1], wantCommit)
	}

	send := caller.calls[2]
	if send.server != "chat" || send.tool != "send_personal_message" || send.args["msgType"] != "file" {
		t.Fatalf("send direct target call = %#v", send)
	}
	if send.args["receiverOpenDingTalkId"] != "D-target" {
		t.Fatalf("send direct target = %#v", send.args)
	}
	if _, ok := send.args["openDingTalkId"]; ok {
		t.Fatalf("send direct target leaked upload target key: %#v", send.args)
	}
}

func TestChatMessageSendFilePathRequiresFileMessageType(t *testing.T) {
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })

	filePath := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(filePath, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := &chatFilePathCaller{}
	err := runChatCoverageCommand(t, caller,
		"message", "send",
		"--group=cid",
		"--file-path="+filePath,
	)
	if err == nil {
		t.Fatal("bare --file-path succeeded without --msg-type=file")
	}
	if len(caller.calls) != 0 {
		t.Fatalf("bare --file-path made remote calls: %#v", caller.calls)
	}
}

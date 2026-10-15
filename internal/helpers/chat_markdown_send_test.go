package helpers

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TC-101: Local and existing V2 images upload once, then send one Markdown message.
func TestCrossPlatformCoverageShouldSendOneMessageWhenChatMessageSendGivenLocalImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	writeMarkdownTestPNG(t, path)
	caller := &chatFilePathCaller{}
	_, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--conversation-id=cid", "--content=前![图]("+path+")后 ![已有]($existing-image)", "--title=标题", "--uuid=key", "--format=json")
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[0].tool != "upload_media" || caller.calls[1].tool != "send_personal_message" {
		t.Fatalf("calls=%#v", caller.calls)
	}
	if got := caller.calls[0].args["bizType"]; got != "CHAT_IMAGE" {
		t.Fatalf("upload bizType=%v, want CHAT_IMAGE", got)
	}
	send := caller.calls[1].args
	if send["msgType"] != "markdown" || send["uuid"] != "key" || send["openConversationId"] != "cid" {
		t.Fatalf("send=%#v", send)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(send["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["text"] != "前![图]($uploaded-image)后 ![已有]($existing-image)" || content["title"] != "标题" {
		t.Fatalf("content=%#v", content)
	}
}

func TestCrossPlatformCoverageChatMessageSendReusesV2Images(t *testing.T) {
	for _, destination := range []string{"$iwElAqNqcGcDAQTR", "mediaid:$iwElAqNqcGcDAQTR"} {
		t.Run(destination, func(t *testing.T) {
			caller := &chatFilePathCaller{}
			_, err := executeChatFilePathCommand(t, caller,
				"message", "send", "--conversation-id=cid", "--content=前![图]("+destination+")后", "--format=json")
			if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "send_personal_message" {
				t.Fatalf("err=%v calls=%#v", err, caller.calls)
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(caller.calls[0].args["content"].(string)), &content); err != nil {
				t.Fatal(err)
			}
			if content["text"] != "前![图]($iwElAqNqcGcDAQTR)后" {
				t.Fatalf("content=%#v", content)
			}
		})
	}
}

// TC-105: Direct messages use the same single-send Markdown payload.
func TestCrossPlatformCoverageShouldSendOneDirectMessageWhenChatMessageSendGivenLocalImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	writeMarkdownTestPNG(t, path)
	caller := &chatFilePathCaller{}
	_, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--open-dingtalk-id="+helperCurrentDOpenID,
		"--content=前![图]("+path+")后", "--format=json")
	if err != nil || len(caller.calls) != 2 {
		t.Fatalf("err=%v calls=%#v", err, caller.calls)
	}
	send := caller.calls[1].args
	if send["receiverOpenDingTalkId"] != helperCurrentDOpenID || send["openConversationId"] != nil || send["msgType"] != "markdown" {
		t.Fatalf("send=%#v", send)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(send["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["text"] != "前![图]($uploaded-image)后" {
		t.Fatalf("content=%#v", content)
	}
}

// TC-106: Existing remote-only Markdown must not enter the upload flow.
func TestCrossPlatformCoverageShouldPreservePayloadWhenChatMessageSendGivenRemoteImage(t *testing.T) {
	caller := &chatFilePathCaller{}
	markdown := "前![图](https://example.com/image.png)后"
	_, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--conversation-id=cid", "--content="+markdown, "--title=标题", "--format=json")
	if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "send_personal_message" {
		t.Fatalf("err=%v calls=%#v", err, caller.calls)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(caller.calls[0].args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["text"] != markdown || content["title"] != "标题" {
		t.Fatalf("content=%#v", content)
	}
}

// TC-102: Failed upload must never call the message-send interface.
func TestCrossPlatformCoverageShouldNotSendWhenChatMessageSendGivenUploadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	writeMarkdownTestPNG(t, path)
	caller := &chatFilePathCaller{failTool: "upload_media"}
	_, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--conversation-id=cid", "--content=![图]("+path+")", "--format=json")
	if err == nil || len(caller.calls) != 1 || caller.calls[0].tool != "upload_media" {
		t.Fatalf("err=%v calls=%#v", err, caller.calls)
	}
}

// TC-103: A local-image dry-run contains no path, body, or network operation.
func TestCrossPlatformCoverageShouldRedactWhenChatMessageSendGivenLocalImageDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.png")
	caller := &chatFilePathCaller{dryRun: true}
	output, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--conversation-id=cid", "--content=秘密![图]("+path+")", "--format=json")
	if err != nil || len(caller.calls) != 0 || strings.Contains(output, path) || strings.Contains(output, "秘密") {
		t.Fatalf("output=%q err=%v calls=%#v", output, err, caller.calls)
	}
}

func TestCrossPlatformCoverageRedactDirectTargetWhenChatMessageSendGivenLocalImageDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.png")
	caller := &chatFilePathCaller{dryRun: true}
	output, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--open-dingtalk-id="+helperCurrentDOpenID,
		"--content=秘密![图]("+path+")", "--format=json")
	if err != nil || len(caller.calls) != 0 || strings.Contains(output, path) || strings.Contains(output, "秘密") || !strings.Contains(output, "receiverOpenDingTalkId") {
		t.Fatalf("output=%q err=%v calls=%#v", output, err, caller.calls)
	}
}

// TC-104: An inferred title never exposes the sender's local image path.
func TestCrossPlatformCoverageShouldHidePathWhenChatMessageSendGivenImageOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.png")
	writeMarkdownTestPNG(t, path)
	caller := &chatFilePathCaller{}
	_, err := executeChatFilePathCommand(t, caller,
		"message", "send", "--conversation-id=cid", "--content=![图]("+path+")", "--format=json")
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(caller.calls[1].args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["title"] != "图文消息" {
		t.Fatalf("title=%q", content["title"])
	}
}

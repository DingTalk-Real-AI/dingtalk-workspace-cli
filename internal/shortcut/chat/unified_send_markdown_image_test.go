package chat

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func writeShortcutImage(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
}

// TC-201: Mixed local and existing V2 images upload once before one personal send.
func TestCrossPlatformCoverageShouldSendOneMessageWhenMessagesSendGivenLocalMarkdownImage(t *testing.T) {
	t.Chdir(t.TempDir())
	writeShortcutImage(t, "image.png")
	fake := &larkAlignmentCaller{responses: map[string]string{
		"dingtalk-file/upload_media": `{"success":true,"mediaIdV1":"@legacy","mediaIdV2":"$mixed"}`,
	}}
	rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{
		"as": "user", "group": "cid", "markdown": "前![图](./image.png)后 ![已有]($existing-image)", "uuid": "key",
	})
	if err := executeMessagesSend(rt); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 2 || fake.calls[0].tool != "upload_media" || fake.calls[1].tool != "send_personal_message" {
		t.Fatalf("calls=%#v", fake.calls)
	}
	if got := fake.calls[0].args["bizType"]; got != "CHAT_IMAGE" {
		t.Fatalf("upload bizType=%v, want CHAT_IMAGE", got)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(fake.calls[1].args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["text"] != "前![图]($mixed)后 ![已有]($existing-image)" || fake.calls[1].args["uuid"] != "key" {
		t.Fatalf("content=%#v args=%#v", content, fake.calls[1].args)
	}
	if content["title"] != "前" {
		t.Fatalf("title=%q", content["title"])
	}
}

func TestCrossPlatformCoverageMessagesSendReusesV2Images(t *testing.T) {
	for _, destination := range []string{"$iwElAqNqcGcDAQTR", "mediaid:$iwElAqNqcGcDAQTR"} {
		t.Run(destination, func(t *testing.T) {
			fake := &larkAlignmentCaller{}
			rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{
				"as": "user", "group": "cid", "markdown": "前![图](" + destination + ")后",
			})
			if err := executeMessagesSend(rt); err != nil || len(fake.calls) != 1 || fake.calls[0].tool != "send_personal_message" {
				t.Fatalf("err=%v calls=%#v", err, fake.calls)
			}
			var content map[string]string
			if err := json.Unmarshal([]byte(fake.calls[0].args["content"].(string)), &content); err != nil {
				t.Fatal(err)
			}
			if content["text"] != "前![图]($iwElAqNqcGcDAQTR)后" {
				t.Fatalf("content=%#v", content)
			}
		})
	}
}

// TC-202: Shortcut images cannot escape the current working directory.
func TestCrossPlatformCoverageShouldRejectWhenMessagesSendGivenOutsideMarkdownImage(t *testing.T) {
	parent := t.TempDir()
	work := filepath.Join(parent, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	fake := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{
		"as": "user", "group": "cid", "markdown": "![图](../outside.png)",
	})
	if err := executeMessagesSend(rt); err == nil || len(fake.calls) != 0 {
		t.Fatalf("err=%v calls=%#v", err, fake.calls)
	}
}

// TC-203: Other identities reject local Markdown images without a write.
func TestCrossPlatformCoverageShouldRejectWhenMessagesSendGivenBotLocalMarkdownImage(t *testing.T) {
	fake := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{
		"as": "bot", "group": "cid", "robot-code": "robot", "markdown": "![图](./image.png)",
	})
	if err := executeMessagesSend(rt); err == nil || !strings.Contains(err.Error(), "仅支持 --as user") || len(fake.calls) != 0 {
		t.Fatalf("err=%v calls=%#v", err, fake.calls)
	}
}

// TC-204: The resolver itself never uploads during a dry-run.
func TestCrossPlatformCoverageShouldNotUploadWhenMessagesSendGivenDryRun(t *testing.T) {
	fake := &larkAlignmentCaller{dryRun: true}
	rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{
		"as": "user", "group": "cid", "markdown": "secret![图](./private.png)",
	})
	_, report, err := helpers.ResolveMarkdownLocalImages(t.Context(), "![图](./private.png)", helpers.MarkdownImageOptions{DryRun: true})
	if err != nil || report.LocalCount != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if err := executeMessagesSend(rt); err != nil || len(fake.calls) != 0 {
		t.Fatalf("err=%v calls=%#v", err, fake.calls)
	}
}

func TestCrossPlatformCoverageUseSafeTitleWhenMessagesSendGivenImageOnly(t *testing.T) {
	t.Chdir(t.TempDir())
	writeShortcutImage(t, "image.png")
	fake := &larkAlignmentCaller{responses: map[string]string{
		"dingtalk-file/upload_media": `{"success":true,"mediaIdV1":"@legacy","mediaIdV2":"$mixed"}`,
	}}
	rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{
		"as": "user", "group": "cid", "markdown": "![图](./image.png)",
	})
	if err := executeMessagesSend(rt); err != nil || len(fake.calls) != 2 {
		t.Fatalf("err=%v calls=%#v", err, fake.calls)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(fake.calls[1].args["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["title"] != "图文消息" {
		t.Fatalf("title=%q", content["title"])
	}
}

func TestCrossPlatformCoveragePreserveMentionWhenMessagesSendGivenBotBatchAndWebhook(t *testing.T) {
	for _, tc := range []struct {
		flags map[string]string
		want  any
	}{
		{map[string]string{"as": "bot", "robot-code": "robot", "groups": "cid1,cid2", "markdown": "hello", "at-all": "true"}, "true"},
		{map[string]string{"as": "webhook", "webhook-token": "token", "markdown": "hello", "at-all": "true"}, true},
	} {
		fake := &larkAlignmentCaller{dryRun: true}
		rt := optimizationRuntime(t, fake, MessagesSend, tc.flags)
		if err := executeMessagesSend(rt); err != nil || len(fake.calls) != 0 {
			t.Fatalf("flags=%v err=%v calls=%#v", tc.flags, err, fake.calls)
		}
		if _, _, err := output.EmitStoredResult(rt.Command()); err != nil {
			t.Fatal(err)
		}
		var preview struct {
			Actions []struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"actions"`
		}
		if err := json.Unmarshal(rt.Command().OutOrStdout().(*bytes.Buffer).Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if len(preview.Actions) == 0 || preview.Actions[0].Arguments["isAtAll"] != tc.want {
			t.Fatalf("flags=%v preview=%+v", tc.flags, preview)
		}
	}
}

func TestCrossPlatformCoverageRejectWhenMessagesSendGivenUnknownIdentity(t *testing.T) {
	fake := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, fake, MessagesSend, map[string]string{"as": "unknown", "text": "hello"})
	if err := executeMessagesSend(rt); err == nil || !strings.Contains(err.Error(), "unsupported identity") {
		t.Fatalf("err=%v", err)
	}
}

func TestCrossPlatformCoverageRejectOrPassThroughNonUserMarkdownImageBoundary(t *testing.T) {
	for _, tc := range []struct {
		identity, body string
		wantError      bool
	}{
		{"webhook", "![图](./local.png)", true},
		{"bot", "![图](ftp://example.com/a.png)", true},
		{"bot", "![图](https://example.com/a.png)", false},
	} {
		fake := &larkAlignmentCaller{dryRun: true}
		flags := map[string]string{"as": tc.identity, "markdown": tc.body, "at-all": "true"}
		if tc.identity == "bot" {
			flags["robot-code"] = "robot"
			flags["group"] = "cid"
		} else {
			flags["webhook-token"] = "token"
		}
		rt := optimizationRuntime(t, fake, MessagesSend, flags)
		err := executeMessagesSend(rt)
		if (err != nil) != tc.wantError || len(fake.calls) != 0 {
			t.Fatalf("identity=%s body=%q err=%v calls=%#v", tc.identity, tc.body, err, fake.calls)
		}
	}
}

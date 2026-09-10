package chat

import (
	"bytes"
	"context"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"strings"
	"testing"
)

func optimizationRuntime(t *testing.T, f *larkAlignmentCaller, s shortcut.Shortcut, flags map[string]string) *shortcut.RuntimeContext {
	t.Helper()
	helpers.InitDeps(f)
	root := newPlatformCoverageRoot()
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	root.SetOut(&bytes.Buffer{})
	cmd, _, err := root.Find([]string{"chat", s.Command})
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetContext(ctx)
	args := []string{"--yes"}
	if f.dryRun {
		args = append(args, "--dry-run")
	}
	for k, v := range flags {
		args = append(args, "--"+k+"="+v)
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	return shortcut.RuntimeContextForTest(cmd, s)
}
func TestCrossPlatformCoverageOptimizationEditValidationAndReceipts(t *testing.T) {
	for _, flags := range []map[string]string{{}, {"text": "proof", "at-open-dingtalk-ids": "bad"}, {"content": `{"text":"proof"}`, "title": "bad"}, {"content": "[1]"}, {"content": `{"text":"proof","attachments":"x"}`}, {"text": "<@all>"}} {
		f := &larkAlignmentCaller{}
		s := newMessagesEdit()
		rt := optimizationRuntime(t, f, s, flags)
		if s.Execute(rt) == nil || len(f.calls) != 0 {
			t.Fatalf("invalid edit accepted %v", flags)
		}
	}
	for _, tc := range []struct {
		name     string
		dry      bool
		response string
		fail     string
		wrong    bool
	}{{"dry", true, `{"success":true}`, "", false}, {"write", false, `{"success":true}`, "", false}, {"unknown", false, `{"result":{}}`, "", false}, {"failed", false, "", "im/edit_message", false}, {"wrong-context", false, "", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{dryRun: tc.dry, failProductTool: tc.fail, responses: map[string]string{"im/list_messages_by_ids": `{"result":{"messages":[{"openMessageId":"m","openConversationId":"cid"}]}}`, "im/edit_message": tc.response}}
			s := newMessagesEdit()
			flags := map[string]string{"message-id": "m", "text": "proof <@all> <@" + optimizationOpenID + ">", "at-all": "true", "at-open-dingtalk-ids": optimizationOpenID}
			if tc.wrong {
				flags["group"] = "wrong"
			}
			rt := optimizationRuntime(t, f, s, flags)
			err := s.Execute(rt)
			if tc.fail != "" || tc.wrong {
				if err == nil {
					t.Fatal("failed edit success")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "unknown" {
				code, _, _ := output.EmitStoredResult(rt.Command())
				if code == 0 {
					t.Fatal("unknown write was success")
				}
			}
			if tc.dry && len(f.calls) != 1 {
				t.Fatal("dry run mutated")
			}
		})
	}
}
func TestCrossPlatformCoverageOptimizationReplyIdentityMatrix(t *testing.T) {
	cases := []map[string]string{{"content": ""}, {"as": "bot"}, {"as": "bot", "robot-code": "robot", "uuid": "key"}, {"robot-code": "robot"}, {"thread-id": "t"}, {"reply-in-thread": "true", "open-dingtalk-id": optimizationOpenID}, {"reply-in-thread": "true", "ref-sender": "user"}, {"open-dingtalk-id": "bad"}}
	for _, flags := range cases {
		if _, ok := flags["content"]; !ok {
			flags["content"] = "proof"
		}
		f := &larkAlignmentCaller{}
		rt := optimizationRuntime(t, f, MessagesReply, flags)
		if validateReplyExtensions(rt) == nil || len(f.calls) != 0 {
			t.Fatalf("identity mismatch accepted %#v", flags)
		}
	}
}
func TestCrossPlatformCoverageOptimizationSendExtensionContractMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, identity, kind string
		flags                map[string]string
	}{
		{"a2ui-idempotency", "user", "a2ui", map[string]string{"uuid": "key"}},
		{"a2ui-json", "user", "a2ui", map[string]string{"a2ui-messages": "bad"}},
		{"media-bot", "bot", "image", map[string]string{"media-id": "m"}},
		{"expiry-kind", "user", "text", map[string]string{"expires-seconds": "1"}},
		{"expiry-negative", "user", "share-chat", map[string]string{"expires-seconds": "-1"}},
		{"profile-bot", "bot", "profile", nil},
		{"bad-contact", "user", "profile", map[string]string{"contact-id": "bad"}},
		{"bot-media-openIDs", "bot", "image", map[string]string{"open-dingtalk-ids": optimizationOpenID}},
		{"bot-media-mention", "bot", "image", map[string]string{"at-all": "true"}},
		{"bot-file-groupfile", "bot", "file", map[string]string{"groups-file": "../bad"}},
		{"bot-file-count", "bot", "file", map[string]string{"users": "a,b"}},
		{"bot-file-path", "bot", "file", map[string]string{"users": "a", "file": "../bad"}},
		{"bot-file-missing", "bot", "file", map[string]string{"users": "a", "file": "no-such-fixture-file"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{}
			rt := optimizationRuntime(t, f, MessagesSend, tc.flags)
			if validateSendExtensions(rt, tc.identity, tc.kind) == nil || len(f.calls) != 0 {
				t.Fatal("invalid media contract was accepted")
			}
		})
	}
	f := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, f, MessagesSend, map[string]string{"as": "bot", "group": "cid", "robot-code": "robot", "text": "proof", "msg-type": "audio"})
	if validateMessagesSend(rt) == nil {
		t.Fatal("unsupported Bot audio accepted")
	}
	rt = optimizationRuntime(t, f, MessagesSend, map[string]string{"as": "bot", "text": "proof", "msg-type": "text", "image-url": "https://files.example.com/x.png"})
	if executeMessagesSend(rt) == nil || len(f.calls) != 0 {
		t.Fatal("invalid execute content sent")
	}
	for _, kind := range []string{"a2ui", "profile", "share-chat"} {
		f := &larkAlignmentCaller{}
		rt := optimizationRuntime(t, f, MessagesSend, map[string]string{"uuid": "key", "contact-id": optimizationOpenID, "share-chat-id": "cid", "a2ui-messages": "bad"})
		err := executeMessagesSendUserShare(rt, "", optimizationOpenID, kind)
		if kind == "a2ui" {
			if err == nil {
				t.Fatal("bad A2UI sent")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}
func TestCrossPlatformCoverageOptimizationCreatedBotsRecovery(t *testing.T) {
	for _, fail := range []bool{false, true} {
		f := &larkAlignmentCaller{}
		if fail {
			f.failProductTool = "bot/add_robot_to_group"
		}
		rt := optimizationRuntime(t, f, ChatCreate, nil)
		err := addCreatedGroupBots(rt, map[string]any{"openConversationId": "cid"}, []string{"robot"})
		if (err != nil) != fail {
			t.Fatal(err)
		}
	}
	for _, failOutput := range []bool{false, true} {
		f := &larkAlignmentCaller{}
		rt := optimizationRuntime(t, f, ChatCreate, nil)
		if failOutput {
			rt.Command().SetOut(chatOutputErrorWriter{err: errors.New("fixture output failure")})
		}
		if addCreatedGroupBots(rt, map[string]any{}, []string{"robot"}) == nil || len(f.calls) != 0 {
			t.Fatal("missing group ID retried/mutated")
		}
	}
}

func TestCrossPlatformCoverageOptimizationPartialResultValidationFailure(t *testing.T) {
	testseam.Swap(t, &newFeedPartialData, func(int, []any, []output.PartialFailedEntry, []output.PartialUnknownEntry) (*output.PartialData, error) {
		return nil, errors.New("framework rejected partial result")
	})
	f := &larkAlignmentCaller{sequenceResponses: map[string][]string{"im/set_top_conversation": {`{"success":true}`, `{"result":{}}`}}}
	_, err := runChatParity(t, f, "+feed-shortcut-create", "--chat-ids", "cid-a,cid-b", "--yes")
	if err == nil {
		t.Fatal("framework result validation failure ignored")
	}
	f = &larkAlignmentCaller{}
	rt := optimizationRuntime(t, f, ChatCreate, nil)
	rt.Command().SetOut(chatOutputErrorWriter{err: errors.New("output failed")})
	if addCreatedGroupBots(rt, map[string]any{"openConversationId": "cid"}, []string{"robot"}) == nil {
		t.Fatal("success output failure ignored")
	}
}

func TestCrossPlatformCoverageOptimizationFinalScopeGuards(t *testing.T) {
	f := &larkAlignmentCaller{responses: map[string]string{"chat/get_conversation_info": `{"result":{"conversationInfo":{"openConversationId":"other"}}}`}}
	rt := optimizationRuntime(t, f, MessagesReply, map[string]string{"message-id": "m", "content": "proof", "open-dingtalk-id": optimizationOpenID})
	err := executeReplyExtensions(rt, replyTarget{message: map[string]any{}, conversationID: "cid", sender: optimizationOpenID})
	if err == nil || len(f.calls) != 1 {
		t.Fatal("mismatched direct recipient was used")
	}
	f = &larkAlignmentCaller{}
	rt = optimizationRuntime(t, f, MessagesSend, map[string]string{"card-summary": "summary"})
	if validateSendExtensions(rt, "user", "text") == nil || len(f.calls) != 0 {
		t.Fatal("card-only metadata accepted for text")
	}
	rt = optimizationRuntime(t, f, MessagesSend, map[string]string{"as": "bot", "group": "cid", "robot-code": "robot", "msg-type": "audio", "file": "fixture.mp3"})
	err = validateMessagesSend(rt)
	if err == nil || !strings.Contains(err.Error(), "不支持原生音视频") || len(f.calls) != 0 {
		t.Fatal("unsupported native Bot media reached transport", err)
	}
}

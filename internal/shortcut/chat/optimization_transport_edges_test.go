package chat

import (
	"context"
	"errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"os"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOptimizationThreadCreationRecovery(t *testing.T) {
	for _, tc := range []struct {
		name           string
		dry            bool
		response, fail string
		wantErr        bool
	}{{"dry", true, "", "", false}, {"failed", false, "", "im/convert_message_to_thread", true}, {"unknown", false, `{"success":true}`, "", true}, {"success", false, `{"result":{"openConvThreadId":"child"}}`, "", false}, {"reply-failed", false, `{"result":{"openConvThreadId":"child"}}`, "chat/send_personal_message", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{dryRun: tc.dry, failProductTool: tc.fail, responses: map[string]string{"im/convert_message_to_thread": tc.response}}
			rt := optimizationRuntime(t, f, MessagesReply, map[string]string{"message-id": "m", "content": "proof", "create-thread": "true", "reply-in-thread": "true"})
			err := executeReplyExtensions(rt, replyTarget{message: map[string]any{}, conversationID: "cid"})
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
			if tc.name == "reply-failed" && !strings.Contains(err.Error(), "Thread已转换") {
				t.Fatal("recovery context lost", err)
			}
			if tc.dry && len(f.calls) != 0 {
				t.Fatal("preview wrote")
			}
		})
	}
	for _, tc := range []struct {
		name            string
		flags           map[string]string
		message         map[string]any
		cid, info, fail string
		wantErr         bool
	}{
		{"bot-thread", map[string]string{"as": "bot", "robot-code": "robot"}, map[string]any{"openConvThreadId": "child"}, "cid", "", "", true},
		{"bot-bad-quote", map[string]string{"as": "bot", "robot-code": "robot"}, map[string]any{}, "", "", "", true},
		{"direct-error", map[string]string{"open-dingtalk-id": optimizationOpenID}, map[string]any{}, "cid", "", "chat/get_conversation_info", true},
		{"direct-nested", map[string]string{"open-dingtalk-id": optimizationOpenID, "uuid": "key"}, map[string]any{}, "cid", `{"result":{"conversationInfo":{"openCid":"cid"}}}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{responses: map[string]string{"chat/get_conversation_info": tc.info}, failProductTool: tc.fail}
			tc.flags["message-id"] = "m"
			tc.flags["content"] = "proof"
			rt := optimizationRuntime(t, f, MessagesReply, tc.flags)
			err := executeReplyExtensions(rt, replyTarget{message: tc.message, conversationID: tc.cid, sender: optimizationOpenID})
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
		})
	}
	f := &larkAlignmentCaller{dryRun: true}
	rt := optimizationRuntime(t, f, MessagesReply, map[string]string{"content": "proof"})
	if executeReplyTransport(rt, "chat", "send_personal_message", nil, "cid", "quote") != nil || len(f.calls) != 0 {
		t.Fatal("transport preview mutated")
	}
}
func TestCrossPlatformCoverageOptimizationMgetAbortAndEnrichmentShapes(t *testing.T) {
	for _, mode := range []string{"cancelled", "systemic-left", "unknown-shape"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			f := &mgetTestCaller{call: func(_ string, args map[string]any) (map[string]any, error) {
				calls++
				if mode == "systemic-left" {
					if len(args["openMsgIds"].([]string)) > 1 {
						return nil, invalidMgetIDError()
					}
					return nil, errors.New("systemic")
				}
				return map[string]any{"result": map[string]any{}}, nil
			}}
			helpers.InitDeps(f)
			root := newPlatformCoverageRoot()
			cmd, _, _ := root.Find([]string{"chat", "+messages-mget"})
			cmd.SetContext(context.Background())
			if mode == "cancelled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				cmd.SetContext(ctx)
			}
			rt := shortcut.RuntimeContextForTest(cmd, MessagesMget)
			_, err := readMessagesMget(rt, []string{"m1", "m2"})
			if err == nil {
				t.Fatal("bad batch succeeded")
			}
			if mode == "cancelled" && calls != 0 || mode == "systemic-left" && calls != 2 {
				t.Fatal("unbounded/late calls", calls)
			}
		})
	}
	if _, known := mgetCollection(map[string]any{"messages": []any{1}}); known {
		t.Fatal("invalid collection shape")
	}
	for _, tc := range []struct {
		name, response, fail string
		missingCID           bool
	}{{"missing-cid", "", "", true}, {"lookup-fail", "", "chat/list_topic_replies", false}, {"shape", `{"result":{}}`, "", false}, {"wrong-reply", `{"result":{"messages":[{"openMessageId":"r","openConversationId":"other"}],"hasMore":false}}`, "", false}, {"duplicate", `{"result":{"messages":[{"openMessageId":"r"},{"openMessageId":"r"}],"hasMore":false}}`, "", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &larkAlignmentCaller{failProductTool: tc.fail, responses: map[string]string{"chat/list_topic_replies": tc.response}}
			rt := optimizationRuntime(t, f, MessagesMget, map[string]string{"no-reactions": "true"})
			m := map[string]any{"openMessageId": "m", "openConvThreadId": "t"}
			if !tc.missingCID {
				m["openConversationId"] = "cid"
			}
			ledger, _, _ := EnrichMessageDetails(rt, []map[string]any{m})
			if tc.name != "duplicate" && ledger["complete"] != false {
				t.Fatal("bad Thread marked complete", ledger)
			}
		})
	}
	f := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, f, MessagesMget, nil)
	requests, _, fs := EnrichMessageReactions(rt, []map[string]any{{}})
	if requests != 0 || len(fs) != 0 {
		t.Fatal("missing message identity caused lookup")
	}
}
func TestCrossPlatformCoverageOptimizationThreadDecryptFailureIsIncomplete(t *testing.T) {
	testseam.Swap(t, &messageReadCryptoClient, &messagecrypto.Client{BackendReady: func() bool { return true }, PolicyCache: messagecrypto.NewPolicyCache(nil), Identity: func(context.Context, string) (messagecrypto.Identity, error) {
		return messagecrypto.Identity{}, errors.New("fixture identity unavailable")
	}})
	f := &larkAlignmentCaller{responses: map[string]string{"chat/list_topic_replies": `{"result":{"messages":[{"openMessageId":"r","openConversationId":"cid","content":"` + testCipher + `"}],"hasMore":false}}`, "im/get_message_crypto_policy": `{"result":{"mode":"required"}}`}}
	rt := optimizationRuntime(t, f, MessagesMget, map[string]string{"no-reactions": "true"})
	ledger, _, _ := EnrichMessageDetails(rt, []map[string]any{{"openMessageId": "m", "openConversationId": "cid", "openConvThreadId": "t"}})
	if ledger["complete"] != false {
		t.Fatal("decrypt failure claimed complete", ledger)
	}
}

func TestCrossPlatformCoverageOptimizationBotFileAndA2UIGuards(t *testing.T) {
	f := &larkAlignmentCaller{}
	rt := optimizationRuntime(t, f, MessagesSend, map[string]string{"a2ui-messages": `["proof"]`})
	if executeMessagesSendUserShare(rt, "cid", "", "a2ui") != nil {
		t.Fatal("A2UI defaults failed")
	}
	for _, flags := range []map[string]string{{"groups-file": "../bad"}, {"file": "../bad"}, {"file": "no-such-upload-fixture"}} {
		f := &larkAlignmentCaller{}
		rt := optimizationRuntime(t, f, MessagesSend, flags)
		if executeMessagesSendBotMedia(rt, "file") == nil || len(f.calls) != 0 {
			t.Fatal("invalid Bot file uploaded")
		}
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("fixture.txt", []byte("proof"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"dry-openid", "failed-upload", "missing-url", "image-openid"} {
		t.Run(mode, func(t *testing.T) {
			f := &larkAlignmentCaller{dryRun: mode == "dry-openid"}
			flags := map[string]string{"file": "fixture.txt", "users": "u"}
			kind := "file"
			if mode == "dry-openid" {
				delete(flags, "users")
				flags["open-dingtalk-ids"] = optimizationOpenID
			}
			if mode == "image-openid" {
				kind = "image"
				delete(flags, "users")
				flags["open-dingtalk-ids"] = optimizationOpenID
			}
			testseam.Swap(t, &uploadBotMessageFile, func(context.Context, map[string]any, helpers.ConversationLocalFileMeta, string) (string, error) {
				if mode == "failed-upload" {
					return "", errors.New("upload unavailable")
				}
				return `{"result":{"fileId":"f"}}`, nil
			})
			rt := optimizationRuntime(t, f, MessagesSend, flags)
			err := executeMessagesSendBotMedia(rt, kind)
			wantErr := mode == "failed-upload" || mode == "missing-url"
			if (err != nil) != wantErr {
				t.Fatal(err)
			}
		})
	}
}

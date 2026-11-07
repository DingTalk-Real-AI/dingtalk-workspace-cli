// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package smart

import (
	"context"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// The smart runtime satisfies the message crypto Runtime interface directly —
// the decrypt pipeline takes rt without an adapter.
var _ messagecrypto.Runtime = (*shortcut.RuntimeContext)(nil)

const chatDecryptGateCiphertext = "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcQ==||2||1||196"

type chatDecryptGateCaller struct {
	calls []string
}

func chatDecryptGateResult(text string) *edition.ToolResult {
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: text}}}
}

func (c *chatDecryptGateCaller) CallTool(_ context.Context, product, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, product+"/"+tool)
	if tool == "get_message_crypto_policy" {
		return chatDecryptGateResult(`{"result":{"mode":"required","provider":"safechat","keyServer":"https://keys.example.test","allowedRedirectHost":"auth.example.test","staffIdTransform":"raw","ttlSeconds":60,"reason":"admin_enabled"}}`), nil
	}
	if tool == "batch_ding_decrypt_messages" {
		return chatDecryptGateResult(`{"result":{"items":[{"messageId":"m1","conversationId":"cid-1","status":"success","plaintextContent":"秘密内容","keyVersion":3}]}}`), nil
	}
	return chatDecryptGateResult(`{}`), nil
}

func (c *chatDecryptGateCaller) CallReadTool(ctx context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, "read:"+product+"/"+tool)
	return c.CallTool(ctx, product, tool, args)
}

func (*chatDecryptGateCaller) Format() string { return "json" }
func (*chatDecryptGateCaller) DryRun() bool   { return false }
func (*chatDecryptGateCaller) Fields() string { return "" }
func (*chatDecryptGateCaller) JQ() string     { return "" }

func chatDecryptGateClient() *messagecrypto.Client {
	return &messagecrypto.Client{
		Identity: func(context.Context, string) (messagecrypto.Identity, error) {
			return messagecrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		},
		OpenSession: func(context.Context, messagecrypto.SessionOptions) (*messagecrypto.Session, error) {
			return &messagecrypto.Session{
				Cipher:  &chatDecryptGateCipher{},
				CorpID:  "corp-1",
				StaffID: "staff-1",
				Close:   func() error { return nil },
			}, nil
		},
		BackendReady: func() bool { return true },
		PolicyCache:  messagecrypto.NewPolicyCache(nil),
	}
}

type chatDecryptGateCipher struct{}

func (*chatDecryptGateCipher) EncryptMessage(_ context.Context, _, _ string, plaintext []byte) ([]byte, error) {
	return plaintext, nil
}

func (*chatDecryptGateCipher) DecryptMessage(_ context.Context, _, _ string, _ []byte) ([]byte, error) {
	return []byte("ding-cipher"), nil
}

// TestCrossPlatformCoverageChatShortcutDecryptNamingGate proves the shared
// decrypt pipeline works through a real RuntimeContext: the policy read passes
// the fail-closed read naming gate, batch decrypt dispatches via the strict
// write channel, and --dry-run short-circuits before any MCP call.
func TestCrossPlatformCoverageChatShortcutDecryptNamingGate(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptGateCaller{}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "chat-decrypt-gate-test"}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	rt := shortcut.RuntimeContextForTest(cmd, shortcut.Shortcut{Service: "chat", Product: "chat"})

	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": chatDecryptGateCiphertext}
	ledger := chatmsg.DecryptChatMessageItems(cmd.Context(), rt, []map[string]any{item})
	if ledger == nil {
		t.Fatal("ledger = nil, want decrypt to run")
	}
	if ledger["decryptedCount"] != 1 || ledger["decryptFailedCount"] != 0 {
		t.Fatalf("ledger = %#v", ledger)
	}
	if item["content"] != "秘密内容" || item["contentDecrypted"] != true || item["dingKeyVersion"] != 3 {
		t.Fatalf("item = %#v", item)
	}
	sawPolicy, sawBatch := false, false
	for _, call := range caller.calls {
		switch call {
		case "im/get_message_crypto_policy":
			sawPolicy = true
		case "im/batch_ding_decrypt_messages":
			sawBatch = true
		}
	}
	if !sawPolicy {
		t.Fatalf("policy read never passed the naming gate: calls = %v", caller.calls)
	}
	if !sawBatch {
		t.Fatalf("batch decrypt never dispatched: calls = %v", caller.calls)
	}
}

func TestCrossPlatformCoverageChatShortcutDecryptDryRunShortCircuit(t *testing.T) {
	chatmsg.SwapMessageDecryptClientForTest(t, chatDecryptGateClient())
	caller := &chatDecryptGateCaller{}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "chat-decrypt-gate-test"}
	cmd.Flags().Bool("dry-run", false, "")
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatal(err)
	}
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	rt := shortcut.RuntimeContextForTest(cmd, shortcut.Shortcut{Service: "chat", Product: "chat"})

	item := map[string]any{"openMessageId": "m1", "openConversationId": "cid-1", "content": chatDecryptGateCiphertext}
	if ledger := chatmsg.DecryptChatMessageItems(cmd.Context(), rt, []map[string]any{item}); ledger != nil {
		t.Fatalf("dry-run ledger = %#v, want nil", ledger)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("dry-run must not dispatch MCP calls: %v", caller.calls)
	}
	if item["contentDecrypted"] == true {
		t.Fatalf("dry-run must not rewrite items: %#v", item)
	}
}

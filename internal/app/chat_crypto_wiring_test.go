// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	shortcutchat "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestSafeChatBackendIsInternalToMessageCrypto(t *testing.T) {
	root := NewRootCommand(context.Background())
	for _, command := range root.Commands() {
		if command.Name() == "safechat" {
			t.Fatal("SafeChat backend must not be exposed as a top-level command")
		}
	}
	command, _, err := root.Find([]string{"chat", "crypto", "decrypt"})
	if err != nil || command == nil || command.CommandPath() != "dws chat crypto decrypt" {
		t.Fatalf("message crypto command = %#v, %v", command, err)
	}
}

func TestCrossPlatformCoverageMessageCryptoWiring(t *testing.T) {
	t.Run("app_crypto_client_defaults", func(t *testing.T) {
		oldIdentity := appMessageCryptoCurrentIdentity
		oldOpen := appMessageCryptoOpenSession
		oldAvailable := appMessageCryptoAvailable
		t.Cleanup(func() {
			appMessageCryptoCurrentIdentity = oldIdentity
			appMessageCryptoOpenSession = oldOpen
			appMessageCryptoAvailable = oldAvailable
		})
		appMessageCryptoCurrentIdentity = func(context.Context, string) (msgcrypto.Identity, error) {
			return msgcrypto.Identity{CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		appMessageCryptoOpenSession = func(context.Context, msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			return &msgcrypto.Session{Cipher: appSafeChatFakeCipher{}, CorpID: "corp-1", StaffID: "staff-1"}, nil
		}
		appMessageCryptoAvailable = func() bool { return true }
		client := newAppMessageCryptoClient()
		if client == nil || client.PolicyCache == nil {
			t.Fatalf("client = %#v", client)
		}
		if !client.BackendReady() {
			t.Fatal("BackendReady() = false")
		}
		_, _ = client.Identity(context.Background(), t.TempDir())
		session, err := client.OpenSession(context.Background(), messagecrypto.SessionOptions{
			ConfigDir:           t.TempDir(),
			KeyServer:           " https://key.example.test ",
			AllowedRedirectHost: " redirect.example.test ",
		})
		if err != nil {
			t.Fatal(err)
		}
		if session.CorpID != "corp-1" || session.StaffID != "staff-1" || session.Cipher == nil {
			t.Fatalf("session = %#v", session)
		}
	})
	t.Run("app_crypto_client_open_error", func(t *testing.T) {
		oldOpen := appMessageCryptoOpenSession
		t.Cleanup(func() { appMessageCryptoOpenSession = oldOpen })
		appMessageCryptoOpenSession = func(context.Context, msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			return nil, errors.New("open failed")
		}
		client := newAppMessageCryptoClient()
		if _, err := client.OpenSession(context.Background(), messagecrypto.SessionOptions{}); err == nil || !strings.Contains(err.Error(), "open failed") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("first_non_empty", func(t *testing.T) {
		if got := firstNonEmptyAppCrypto("", "  cli-version  ", "fallback"); got != "cli-version" {
			t.Fatalf("firstNonEmptyAppCrypto() = %q", got)
		}
		if got := firstNonEmptyAppCrypto("", " "); got != "" {
			t.Fatalf("firstNonEmptyAppCrypto(empty) = %q", got)
		}
	})
	t.Run("stateless_token_resolves_identity_via_mcp_and_caches", func(t *testing.T) {
		testseam.Swap(t, &appMessageCryptoCurrentIdentity, func(context.Context, string) (msgcrypto.Identity, error) {
			t.Fatal("stateless --token must not read local OAuth identity")
			return msgcrypto.Identity{}, nil
		})
		var gotOpenOpts msgcrypto.SessionOptions
		testseam.Swap(t, &appMessageCryptoOpenSession, func(_ context.Context, opts msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			gotOpenOpts = opts
			return &msgcrypto.Session{
				Cipher:  appSafeChatFakeCipher{},
				CorpID:  opts.CorpID,
				StaffID: opts.StaffID,
			}, nil
		})
		testseam.Swap(t, &appMessageCryptoAvailable, func() bool { return true })

		rt := &appCryptoFakeRuntime{
			readData: map[string]any{
				"result": []any{
					map[string]any{
						"orgEmployeeModel": map[string]any{
							"corpId": "ding-stateless-corp",
							"userId": "staff-stateless-1",
						},
					},
				},
			},
		}
		client := newAppMessageCryptoClient()
		ctx := withMessageCryptoAuthContext(context.Background(), "explicit-bearer-token", "explicit-client-id")

		id1, err := client.IdentityWithRuntime(ctx, rt, "")
		if err != nil {
			t.Fatalf("IdentityWithRuntime(1) = %v", err)
		}
		if id1.CorpID != "ding-stateless-corp" || id1.StaffID != "staff-stateless-1" {
			t.Fatalf("id1 = %#v", id1)
		}

		// Second call with the same token hits the in-memory cache.
		id2, err := client.IdentityWithRuntime(ctx, rt, "")
		if err != nil || id2 != id1 {
			t.Fatalf("IdentityWithRuntime(2) = %#v, %v", id2, err)
		}
		if rt.readCalls.Load() != 1 {
			t.Fatalf("contact/get_current_user_profile calls = %d, want 1", rt.readCalls.Load())
		}

		session, err := client.OpenSession(ctx, messagecrypto.SessionOptions{Runtime: rt})
		if err != nil {
			t.Fatalf("OpenSession() = %v", err)
		}
		if session.CorpID != "ding-stateless-corp" || session.StaffID != "staff-stateless-1" {
			t.Fatalf("session = %#v", session)
		}
		if gotOpenOpts.AccessToken != "explicit-bearer-token" ||
			gotOpenOpts.ClientID != "explicit-client-id" ||
			gotOpenOpts.CorpID != "ding-stateless-corp" ||
			gotOpenOpts.StaffID != "staff-stateless-1" {
			t.Fatalf("open opts = %#v", gotOpenOpts)
		}
	})
	t.Run("stateless_token_uses_runtime_defaults_without_mcp", func(t *testing.T) {
		prevEdition := edition.Get()
		t.Cleanup(func() { edition.Override(prevEdition) })
		nextEdition := *prevEdition
		nextEdition.RuntimeDefaults = func() map[string]edition.RuntimeDefaultFn {
			return map[string]edition.RuntimeDefaultFn{
				"$corpId":        func(context.Context) (string, bool) { return "corp-from-runtime", true },
				"$currentUserId": func(context.Context) (string, bool) { return "staff-from-runtime", true },
			}
		}
		edition.Override(&nextEdition)

		rt := &appCryptoFakeRuntime{readErr: errors.New("must not call mcp")}
		client := newAppMessageCryptoClient()
		ctx := withMessageCryptoAuthContext(context.Background(), "token-with-defaults", "")
		id, err := client.IdentityWithRuntime(ctx, rt, "")
		if err != nil {
			t.Fatalf("IdentityWithRuntime() = %v", err)
		}
		if id.CorpID != "corp-from-runtime" || id.StaffID != "staff-from-runtime" {
			t.Fatalf("id = %#v", id)
		}
		if rt.readCalls.Load() != 0 {
			t.Fatalf("expected 0 MCP calls, got %d", rt.readCalls.Load())
		}
	})
	t.Run("stateless_token_identity_errors_never_fallback_to_oauth", func(t *testing.T) {
		testseam.Swap(t, &appMessageCryptoCurrentIdentity, func(context.Context, string) (msgcrypto.Identity, error) {
			t.Fatal("must not fall back to local OAuth")
			return msgcrypto.Identity{}, nil
		})
		client := newAppMessageCryptoClient()
		ctx := withMessageCryptoAuthContext(context.Background(), "token-no-corp", "")

		if _, err := client.Identity(ctx, ""); err == nil || !strings.Contains(err.Error(), "无可用 MCP 运行时") {
			t.Fatalf("nil runtime err = %v", err)
		}
		if _, err := client.IdentityWithRuntime(ctx, &appCryptoFakeRuntime{readErr: errors.New("mcp down")}, ""); err == nil || !strings.Contains(err.Error(), "mcp down") {
			t.Fatalf("mcp error = %v", err)
		}
		if _, err := client.IdentityWithRuntime(ctx, &appCryptoFakeRuntime{readData: map[string]any{"userId": "u1"}}, ""); err == nil || !strings.Contains(err.Error(), "未返回组织 corpId") {
			t.Fatalf("missing corpId err = %v", err)
		}
		if _, err := client.OpenSession(ctx, messagecrypto.SessionOptions{}); err == nil {
			t.Fatal("OpenSession should fail when identity resolution fails")
		}
	})
	t.Run("root_command_propagates_token_to_chat_crypto_decrypt", func(t *testing.T) {
		var gotOpenOpts msgcrypto.SessionOptions
		testseam.Swap(t, &appMessageCryptoCurrentIdentity, func(context.Context, string) (msgcrypto.Identity, error) {
			t.Fatal("chat crypto decrypt with --token must not read local OAuth")
			return msgcrypto.Identity{}, nil
		})
		testseam.Swap(t, &appMessageCryptoOpenSession, func(_ context.Context, opts msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			gotOpenOpts = opts
			return &msgcrypto.Session{
				Cipher:  appSafeChatStubDecryptCipher{plain: []byte("ding-layer-cipher")},
				CorpID:  opts.CorpID,
				StaffID: opts.StaffID,
			}, nil
		})
		testseam.Swap(t, &appMessageCryptoAvailable, func() bool { return true })

		prevEdition := edition.Get()
		t.Cleanup(func() { edition.Override(prevEdition) })
		nextEdition := *prevEdition
		nextEdition.RuntimeDefaults = func() map[string]edition.RuntimeDefaultFn {
			return map[string]edition.RuntimeDefaultFn{
				"$corpId":        func(context.Context) (string, bool) { return "corp-root-e2e", true },
				"$currentUserId": func(context.Context) (string, bool) { return "staff-root-e2e", true },
			}
		}
		edition.Override(&nextEdition)

		root := NewRootCommand(context.Background())
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs([]string{
			"--token", "root-explicit-token",
			"--client-id", "root-explicit-client",
			"chat", "crypto", "decrypt",
			"--layer", "safechat",
			"--text", "fake-safechat-ciphertext",
			"--format", "json",
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("root.Execute() = %v (stderr=%s)", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "ding-layer-cipher") {
			t.Fatalf("stdout = %s", stdout.String())
		}
		if gotOpenOpts.AccessToken != "root-explicit-token" ||
			gotOpenOpts.ClientID != "root-explicit-client" ||
			gotOpenOpts.CorpID != "corp-root-e2e" ||
			gotOpenOpts.StaffID != "staff-root-e2e" {
			t.Fatalf("gotOpenOpts = %#v", gotOpenOpts)
		}

		// Verify shortcutchat setter accepts nil reset and re-injection cleanly.
		shortcutchat.SetMessageReadCryptoClient(nil)
		shortcutchat.SetMessageReadCryptoClient(newAppMessageCryptoClient())
	})
}

type appCryptoFakeRuntime struct {
	readData  map[string]any
	readErr   error
	readCalls atomic.Int32
}

func (r *appCryptoFakeRuntime) CallMCPReadData(product, tool string, _ map[string]any) (map[string]any, error) {
	r.readCalls.Add(1)
	if product != "contact" || tool != "get_current_user_profile" {
		return nil, errors.New("unexpected mcp call: " + product + "/" + tool)
	}
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.readData, nil
}

func (r *appCryptoFakeRuntime) CallMCPWriteDataStrict(string, string, map[string]any) (map[string]any, error) {
	return nil, errors.New("not used")
}

func (r *appCryptoFakeRuntime) DryRun() bool { return false }

type appSafeChatStubDecryptCipher struct {
	plain []byte
}

func (appSafeChatStubDecryptCipher) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (c appSafeChatStubDecryptCipher) DecryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return c.plain, nil
}

func (appSafeChatStubDecryptCipher) Close() error { return nil }

type appSafeChatFakeCipher struct{}

func (appSafeChatFakeCipher) EncryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (appSafeChatFakeCipher) DecryptMessage(context.Context, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not used")
}

func (appSafeChatFakeCipher) Close() error { return nil }

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto"
	messagecrypto "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/msgcrypto/message"
	shortcutchat "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
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
		testseam.Swap(t, &appMessageCryptoFetchProfileWithToken, func(context.Context, string) (*edition.ToolResult, bool, error) {
			return nil, false, nil
		})
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
		testseam.Swap(t, &appMessageCryptoFetchProfileWithToken, func(context.Context, string) (*edition.ToolResult, bool, error) {
			return nil, false, nil
		})
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
	t.Run("stateless_context_cache_and_profile_fallback_edges", func(t *testing.T) {
		testseam.Swap(t, &appMessageCryptoFetchProfileWithToken, func(context.Context, string) (*edition.ToolResult, bool, error) {
			return nil, false, nil
		})
		// nil context handling for both context setter and getter.
		if _, ok := messageCryptoAuthFromContext(nil); ok {
			t.Fatal("messageCryptoAuthFromContext(nil) should be false")
		}
		//lint:ignore SA1012 intentionally exercising nil-context defensive guard
		ctxFromNil := withMessageCryptoAuthContext(nil, "tok-nil", "cid-nil")
		gotCtx, ok := messageCryptoAuthFromContext(ctxFromNil)
		if !ok || gotCtx.ExplicitToken != "tok-nil" || gotCtx.ClientID != "cid-nil" {
			t.Fatalf("gotCtx = %#v, ok=%v", gotCtx, ok)
		}

		// nil/empty cache guards and capacity eviction.
		var nilCache *statelessTokenIdentityCache
		if _, ok := nilCache.Get("tok"); ok {
			t.Fatal("nil cache Get should return false")
		}
		nilCache.Set("tok", messagecrypto.Identity{CorpID: "corp"})

		cache := newStatelessTokenIdentityCache()
		if _, ok := cache.Get(""); ok {
			t.Fatal("empty token Get should return false")
		}
		cache.Set("", messagecrypto.Identity{CorpID: "corp"})
		cache.Set("tok", messagecrypto.Identity{CorpID: " "})
		for i := 0; i < maxStatelessTokenIdentityCacheEntries; i++ {
			cache.Set(fmt.Sprintf("tok-%d", i), messagecrypto.Identity{CorpID: "corp"})
		}
		// Inserting one more entry triggers eviction via clear(c.entries).
		cache.Set("tok-overflow", messagecrypto.Identity{CorpID: "corp-overflow"})
		if _, ok := cache.Get("tok-0"); ok {
			t.Fatal("expected earlier entry to be evicted on capacity overflow")
		}
		if got, ok := cache.Get("tok-overflow"); !ok || got.CorpID != "corp-overflow" {
			t.Fatalf("overflow entry = %#v, %v", got, ok)
		}

		// json.Marshal failure branch and nested result map fallback branch.
		client := newAppMessageCryptoClient()
		ctxMarshalErr := withMessageCryptoAuthContext(context.Background(), "tok-marshal-err", "")
		if _, err := client.IdentityWithRuntime(ctxMarshalErr, &appCryptoFakeRuntime{
			readData: map[string]any{"bad": make(chan int)},
		}, ""); err == nil || !strings.Contains(err.Error(), "解析当前用户身份响应失败") {
			t.Fatalf("marshal error = %v", err)
		}

		ctxNested := withMessageCryptoAuthContext(context.Background(), "tok-nested", "")
		nestedID, err := client.IdentityWithRuntime(ctxNested, &appCryptoFakeRuntime{
			readData: map[string]any{
				"result": map[string]any{
					"corpId":  "corp-nested",
					"staffId": "staff-nested",
				},
			},
		}, "")
		if err != nil || nestedID.CorpID != "corp-nested" || nestedID.StaffID != "staff-nested" {
			t.Fatalf("nested identity = %#v, %v", nestedID, err)
		}
	})
	t.Run("stateless_token_uses_real_runner_scoped_token_without_touching_local_oauth", func(t *testing.T) {
		configDir := t.TempDir()
		t.Setenv("DWS_CONFIG_DIR", configDir)
		testseam.Swap(t, &dynamicEndpoints, map[string]string{})
		testseam.Swap(t, &dynamicProducts, map[string]bool{})
		testseam.Swap(t, &dynamicAliases, map[string]string{})
		testseam.Swap(t, &dynamicToolEndpoints, map[string]string{})
		testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{})
		SetDynamicServers([]mcptypes.ServerDescriptor{{
			Key: "contact", Endpoint: "https://mcp-gw.dingtalk.com/server/contact",
			CLI: mcptypes.CLIOverlay{ID: "contact", Command: "contact"},
		}})

		prevProfile := authpkg.RuntimeProfile()
		authpkg.SetRuntimeProfile("stale-profile-must-not-be-read")
		t.Cleanup(func() { authpkg.SetRuntimeProfile(prevProfile) })

		testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
			t.Fatal("scoped token identity lookup must not resolve local OAuth snapshot")
			return AccessTokenSnapshot{}, errors.New("forbidden")
		})
		testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) {
			t.Fatal("scoped token identity lookup must not prefetch local keychain token")
			return "", errors.New("forbidden")
		})
		testseam.Swap(t, &runnerHandlePatAuthCheck, func(context.Context, *runtimeRunner, executor.Invocation, *apperrors.PATError, string, io.Writer) (executor.Result, error) {
			t.Fatal("scoped token identity lookup must not enter interactive PAT auth flow")
			return executor.Result{}, errors.New("forbidden")
		})
		testseam.Swap(t, &runnerRetryWithPatAuthRetry, func(context.Context, executor.Runner, executor.Invocation, *PatScopeError, string, io.Writer) (executor.Result, error) {
			t.Fatal("scoped token identity lookup must not enter PAT scope retry flow")
			return executor.Result{}, errors.New("forbidden")
		})

		flags := &GlobalFlags{}
		runner := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: flags, auditSink: audit.NopSink{}}
		prevCaller := helpers.GetCaller()
		t.Cleanup(func() { helpers.InitDeps(prevCaller) })
		helpers.InitDeps(newRecordingToolCaller(newToolCallerAdapter(runner, flags)))

		// Valid token succeeds through RunWithToken without touching local OAuth or config dir.
		testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, _, tool string, _ map[string]any) (transport.ToolCallResult, error) {
			if tool != "get_current_user_profile" || client.AuthToken != "real-runner-valid-token" {
				t.Fatalf("tool=%q authToken=%q", tool, client.AuthToken)
			}
			return transport.ToolCallResult{
				Content: map[string]any{
					"result": []any{
						map[string]any{
							"orgEmployeeModel": map[string]any{
								"corpId": "corp-real-runner",
								"userId": "staff-real-runner",
							},
						},
					},
				},
			}, nil
		})

		client := newAppMessageCryptoClient()
		ctxValid := withMessageCryptoAuthContext(context.Background(), "real-runner-valid-token", "")
		id, err := client.IdentityWithRuntime(ctxValid, nil, configDir)
		if err != nil {
			t.Fatalf("IdentityWithRuntime(valid) = %v", err)
		}
		if id.CorpID != "corp-real-runner" || id.StaffID != "staff-real-runner" {
			t.Fatalf("identity = %#v", id)
		}

		// PAT auth check rejection returns error directly without interactive auth or local writes.
		testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
			if client.AuthToken != "real-runner-pat-rejected" {
				t.Fatalf("authToken=%q", client.AuthToken)
			}
			return transport.ToolCallResult{
				IsError: true,
				Content: map[string]any{
					"success":    false,
					"errorCode":  "PAT_AUTH_REQUIRED",
					"errorMsg":   "need pat auth",
					"patAuthUrl": "https://auth.example.test",
				},
			}, nil
		})
		ctxPAT := withMessageCryptoAuthContext(context.Background(), "real-runner-pat-rejected", "")
		if _, err := client.IdentityWithRuntime(ctxPAT, nil, configDir); err == nil || !strings.Contains(err.Error(), "查询当前用户身份失败") {
			t.Fatalf("IdentityWithRuntime(PAT rejected) = %v", err)
		}

		// Verify no local credential or app config files were written to configDir.
		entries, err := os.ReadDir(configDir)
		if err != nil {
			t.Fatalf("ReadDir(%s) = %v", configDir, err)
		}
		for _, entry := range entries {
			// logs/ and identity.json (anonymous device id) are non-credential runtime metadata.
			if entry.Name() == "logs" || entry.Name() == "identity.json" {
				continue
			}
			t.Fatalf("configDir must not receive credential/config writes, found %q", entry.Name())
		}

		// Exercise non-tokenOverrideToolCaller fallback and tool result parser edge branches.
		helpers.InitDeps(nil)
		if res, used, err := fetchMessageCryptoProfileWithScopedToken(context.Background(), "tok"); res != nil || used || err != nil {
			t.Fatalf("nil caller fetch = %#v, %v, %v", res, used, err)
		}
		if c, u := extractContactProfileIdentityFromToolResult(nil); c != "" || u != "" {
			t.Fatalf("nil result = %q, %q", c, u)
		}
		if c, u := extractContactProfileIdentityFromToolResult(&edition.ToolResult{
			Content: []edition.ContentBlock{
				{Type: "text", Text: "   "},
				{Type: "text", Text: "not-json"},
				{Type: "text", Text: `{"result":{"corpId":"corp-block","staffId":"staff-block"}}`},
			},
		}); c != "corp-block" || u != "staff-block" {
			t.Fatalf("fallback block parse = %q, %q", c, u)
		}
		if c, u := extractContactProfileIdentityFromToolResult(&edition.ToolResult{
			Content: []edition.ContentBlock{{Type: "text", Text: `{"userId":"only-user"}`}},
		}); c != "" || u != "" {
			t.Fatalf("missing corpId block parse = %q, %q", c, u)
		}
	})
	t.Run("reusable_root_does_not_leak_explicit_token_to_subsequent_invocations", func(t *testing.T) {
		consumeTokenInvocationFlag(nil, nil)

		var openCalls []msgcrypto.SessionOptions
		testseam.Swap(t, &appMessageCryptoOpenSession, func(_ context.Context, opts msgcrypto.SessionOptions) (*msgcrypto.Session, error) {
			openCalls = append(openCalls, opts)
			return &msgcrypto.Session{
				Cipher:  appSafeChatStubDecryptCipher{plain: []byte("plain-" + opts.AccessToken)},
				CorpID:  firstNonEmptyAppCrypto(opts.CorpID, "oauth-corp"),
				StaffID: firstNonEmptyAppCrypto(opts.StaffID, "oauth-staff"),
			}, nil
		})
		testseam.Swap(t, &appMessageCryptoAvailable, func() bool { return true })

		prevEdition := edition.Get()
		t.Cleanup(func() { edition.Override(prevEdition) })
		nextEdition := *prevEdition
		nextEdition.RuntimeDefaults = func() map[string]edition.RuntimeDefaultFn {
			return map[string]edition.RuntimeDefaultFn{
				"$corpId":        func(context.Context) (string, bool) { return "corp-reuse", true },
				"$currentUserId": func(context.Context) (string, bool) { return "staff-reuse", true },
			}
		}
		edition.Override(&nextEdition)

		root := NewRootCommand(context.Background())
		var stdout, stderr bytes.Buffer
		root.SetOut(&stdout)
		root.SetErr(&stderr)

		// First invocation passes explicit --token A and --client-id cid-A.
		root.SetArgs([]string{
			"--token", "tok-A",
			"--client-id", "cid-A",
			"chat", "crypto", "decrypt",
			"--layer", "safechat",
			"--text", "cipher-1",
			"--format", "json",
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("first Execute() = %v (stderr=%s)", err, stderr.String())
		}

		// Second invocation omits --token and must NOT reuse tok-A.
		stdout.Reset()
		stderr.Reset()
		root.SetArgs([]string{
			"chat", "crypto", "decrypt",
			"--layer", "safechat",
			"--text", "cipher-2",
			"--format", "json",
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("second Execute() = %v (stderr=%s)", err, stderr.String())
		}

		// Help invocation with --token must also discard the flag on exit.
		stdout.Reset()
		stderr.Reset()
		root.SetArgs([]string{"--token", "tok-help", "chat", "crypto", "--help"})
		if err := root.Execute(); err != nil {
			t.Fatalf("help Execute() = %v", err)
		}

		stdout.Reset()
		stderr.Reset()
		root.SetArgs([]string{
			"chat", "crypto", "decrypt",
			"--layer", "safechat",
			"--text", "cipher-3",
			"--format", "json",
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("third Execute() = %v (stderr=%s)", err, stderr.String())
		}

		if len(openCalls) != 3 {
			t.Fatalf("openCalls = %d, want 3", len(openCalls))
		}
		if openCalls[0].AccessToken != "tok-A" || openCalls[0].ClientID != "cid-A" {
			t.Fatalf("first open opts = %#v", openCalls[0])
		}
		if openCalls[1].AccessToken != "" || openCalls[1].ClientID != "" {
			t.Fatalf("second invocation leaked prior token/clientId: %#v", openCalls[1])
		}
		if openCalls[2].AccessToken != "" || openCalls[2].ClientID != "" {
			t.Fatalf("invocation after --help leaked prior token: %#v", openCalls[2])
		}
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

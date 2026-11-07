// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageExternalExchangeIntegrationOutputFailureClearsCode(t *testing.T) {
	root := newExternalExchangeIntegrationRoot(t)
	testseam.Swap(t, &rootCreateTemp, func(string, string) (*os.File, error) { return nil, errors.New("injected output failure") })
	testseam.Swap(t, &authExternalExchange, func(context.Context, string, authpkg.ExternalExchangeRequest) (*authpkg.TokenData, error) {
		t.Fatal("reused code after output initialization failure")
		return nil, nil
	})
	root.SetArgs([]string{"auth", "exchange", "--code", "first-code", "--output", filepath.Join(t.TempDir(), "result.json")})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("expected output initialization failure")
	}
	root.SetArgs([]string{"auth", "exchange", "--output", ""})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("missing code accepted")
	}
}

// 使用真实可复用 root；换票由 seam 截断，不读真实凭据、不访问网络。
func newExternalExchangeIntegrationRoot(t *testing.T) *cobra.Command {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(authpkg.EnvClientID, "configured-client")
	t.Setenv(authpkg.EnvClientSecret, "configured-secret")
	authpkg.SetClientCredentials("", "")
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() {
		authpkg.SetClientCredentials("", "")
		authpkg.SetRuntimeProfile("")
		CloseFileLogger()
	})
	root := NewRootCommand(t.Context())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

func TestCrossPlatformCoverageExternalExchangeIntegrationRootCredentialIsolation(t *testing.T) {
	t.Run("exchange_then_ordinary_command", func(t *testing.T) {
		root := newExternalExchangeIntegrationRoot(t)
		observed := addCredentialCaptureCommand(root)
		root.SetArgs([]string{"--client-id", "exchange-client", "--client-secret", "exchange-secret", "auth", "exchange", "--code-stdin", "--dry-run"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatal(err)
		}
		root.SetArgs([]string{"capture-credentials"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatal(err)
		}
		if observed.clientID != "" || observed.clientSecret != "" {
			t.Fatalf("exchange flags leaked into next command: id=%q secret_set=%t", observed.clientID, observed.clientSecret != "")
		}
	})
	t.Run("ordinary_command_then_exchange", func(t *testing.T) {
		root := newExternalExchangeIntegrationRoot(t)
		addCredentialCaptureCommand(root)
		root.SetArgs([]string{"capture-credentials", "--client-id", "previous-client", "--client-secret", "previous-secret"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatal(err)
		}
		called := false
		testseam.Swap(t, &authExternalExchange, func(context.Context, string, authpkg.ExternalExchangeRequest) (*authpkg.TokenData, error) {
			called = true
			if authpkg.ClientID() == "previous-client" || authpkg.ClientSecret() == "previous-secret" {
				t.Error("exchange inherited the previous command's process-wide credential pair")
			}
			return nil, errors.New("fixture stops before network and persistence")
		})
		root.SetArgs([]string{"auth", "exchange", "--code", "second-code"})
		_, _ = root.ExecuteC()
		if !called {
			t.Fatal("exchange did not reach the isolated capture seam")
		}
	})
}

func TestCrossPlatformCoverageExternalExchangeIntegrationLocalFlagIsolation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		first      []string
		second     []string
		input      string
		wantCalls  int
		wantCode   string
		wantClient string
	}{
		{
			name:   "omitted_application_and_assertions",
			first:  []string{"--code", "first-code", "--client-id", "first-client", "--expected-corp-id", "first-corp", "--expected-user-id", "first-user"},
			second: []string{"--code", "second-code"}, wantCalls: 2, wantCode: "second-code",
		},
		{
			name:  "inline_code_then_stdin",
			first: []string{"--code", "first-code"}, second: []string{"--code-stdin"},
			input: "second-code\n", wantCalls: 2, wantCode: "second-code",
		},
		{
			name:  "stdin_then_inline_code",
			first: []string{"--code-stdin"}, second: []string{"--code", "second-code"},
			input: "first-code\n", wantCalls: 2, wantCode: "second-code",
		},
		{
			name:  "omitted_code_is_rejected",
			first: []string{"--code", "first-code"}, wantCalls: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newExternalExchangeIntegrationRoot(t)
			var requests []authpkg.ExternalExchangeRequest
			testseam.Swap(t, &authExternalExchange, func(_ context.Context, _ string, request authpkg.ExternalExchangeRequest) (*authpkg.TokenData, error) {
				requests = append(requests, request)
				return nil, errors.New("fixture stops before network and persistence")
			})
			root.SetIn(strings.NewReader(tc.input))
			root.SetArgs(append([]string{"auth", "exchange"}, tc.first...))
			_, _ = root.ExecuteC()
			if len(requests) != 1 {
				t.Fatal("first exchange did not reach the capture seam")
			}
			root.SetArgs(append([]string{"auth", "exchange"}, tc.second...))
			_, secondErr := root.ExecuteC()
			if len(requests) != tc.wantCalls {
				t.Fatalf("exchange calls=%d, want %d; second error=%v", len(requests), tc.wantCalls, secondErr)
			}
			if tc.wantCalls == 1 {
				if secondErr == nil {
					t.Fatal("missing code was accepted")
				}
				return
			}
			request := requests[1]
			if request.ClientID != tc.wantClient || request.AuthCode != tc.wantCode || request.ExpectedCorpID != "" || request.ExpectedUserID != "" {
				t.Fatalf("second exchange reused local flags: client=%q code_matches=%t corp_assertion_set=%t user_assertion_set=%t", request.ClientID, request.AuthCode == tc.wantCode, request.ExpectedCorpID != "", request.ExpectedUserID != "")
			}
		})
	}
}

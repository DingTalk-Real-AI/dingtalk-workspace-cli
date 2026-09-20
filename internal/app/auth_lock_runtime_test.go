// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageAuthStatusExplicitHistoricalIdentityMigration(t *testing.T) {
	for _, byName := range []bool{false, true} {
		t.Run(fmt.Sprintf("select-by-userName=%v", byName), func(t *testing.T) {
			configDir := setupAuthLogoutProfiles(t)
			previousHooks := edition.Get()
			edition.Override(&edition.Hooks{})
			t.Cleanup(func() { edition.Override(previousHooks) })
			data := authLogoutTestToken("corp-status-migration")
			if err := authpkg.SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
				t.Fatal(err)
			}
			if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{
				Version: 2, CurrentProfile: data.CorpID,
				Profiles: []authpkg.Profile{{Name: "historical", CorpID: data.CorpID, CorpName: data.CorpName}},
			}); err != nil {
				t.Fatal(err)
			}
			account := data.UserID
			if byName {
				account = data.UserName
			}
			// Exercise the real status leaf without root startup's best-effort
			// default token read masking the explicit-selector regression.
			cmd := newAuthStatusCommand()
			cmd.PersistentFlags().String("format", "json", "output format")
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"--profile", data.CorpID + ":" + account})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("auth status explicit historical identity: %v", err)
			}
			var response authStatusResponse
			if err := json.Unmarshal(out.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !response.Authenticated || response.CorpID != data.CorpID {
				t.Fatalf("auth status response = %+v", response)
			}
			loaded, err := authpkg.LoadTokenDataKeychainForIdentity(data.CorpID, data.UserID)
			if err != nil || loaded == nil || loaded.AccessToken != data.AccessToken {
				t.Fatalf("auth status did not migrate the selected identity: %#v, %v", loaded, err)
			}
		})
	}
}

func TestCrossPlatformCoverageAuxiliaryHistoricalIdentityMigration(t *testing.T) {
	for _, byName := range []bool{false, true} {
		t.Run(fmt.Sprintf("select-by-userName=%v", byName), func(t *testing.T) {
			configDir := setupAuthLogoutProfiles(t)
			previousHooks := edition.Get()
			edition.Override(&edition.Hooks{})
			t.Cleanup(func() { edition.Override(previousHooks) })
			testseam.Swap(t, &runtimeTokenManager, NewTokenManager())
			data := authLogoutTestToken("corp-auxiliary-migration")
			if err := authpkg.SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
				t.Fatal(err)
			}
			if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{
				Version: 2, CurrentProfile: data.CorpID,
				Profiles: []authpkg.Profile{{Name: "historical", CorpID: data.CorpID, CorpName: data.CorpName}},
			}); err != nil {
				t.Fatal(err)
			}
			account := data.UserID
			if byName {
				account = data.UserName
			}
			authpkg.SetRuntimeProfile(data.CorpID + ":" + account)
			for attempt := 0; attempt < 2; attempt++ {
				token, err := ResolveAuxiliaryAccessToken(context.Background(), configDir, "")
				if err != nil || token != data.AccessToken {
					t.Fatalf("auxiliary historical account attempt %d = %q, %v", attempt, token, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageBusinessInvocationAuthLockBoundary(t *testing.T) {
	token := authLogoutTestToken("corp-business-lock")
	configDir := setupAuthLogoutProfiles(t, token)
	previousHooks := edition.Get()
	edition.Override(&edition.Hooks{})
	t.Cleanup(func() { edition.Override(previousHooks) })
	testseam.Swap(t, &runtimeTokenManager, NewTokenManager())
	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
	// Real credential resolution must release the auth lock before the RPC.
	calls := 0
	testseam.Swap(t, &runnerCallTool, func(client *transport.Client, ctx context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		calls++
		if client.AuthToken != token.AccessToken {
			return transport.ToolCallResult{}, fmt.Errorf("business request received the wrong token")
		}
		lockCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		lock, err := authpkg.AcquireDualLock(lockCtx, configDir)
		if err != nil {
			return transport.ToolCallResult{}, fmt.Errorf("auth lock remained held during business request: %w", err)
		}
		lock.Release()
		return transport.ToolCallResult{Content: map[string]any{"success": true}}, nil
	})
	runner := &runtimeRunner{transport: transport.NewClient(nil), auditSink: audit.NopSink{}}
	_, err := runner.executeInvocation(context.Background(), "https://auth-lock-test.invalid", executor.Invocation{CanonicalProduct: "drive", Tool: "list_documents"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("business calls=%d, want 1", calls)
	}
}

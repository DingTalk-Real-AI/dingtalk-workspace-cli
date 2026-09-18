// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAuthStatusReadOnlyLocalStates(t *testing.T) {
	for _, state := range []string{"valid", "expired", "repair_required", "unreadable", "not_logged_in"} {
		t.Run(state, func(t *testing.T) {
			token := authLogoutTestToken("corp-readonly")
			if state == "expired" {
				token.ExpiresAt = time.Now().Add(-time.Hour)
			}
			dir := setupAuthLogoutProfiles(t, token)
			testseam.Swap(t, &authOAuthStatus, func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) {
				t.Fatal("readonly entered the legacy status reader")
				return nil, nil
			})
			testseam.Swap(t, &authOAuthAccessToken, func(*authpkg.OAuthProvider, context.Context) (string, error) {
				t.Fatal("readonly attempted an OAuth refresh")
				return "", nil
			})
			switch state {
			case "repair_required":
				if err := authpkg.SaveProfiles(dir, &authpkg.ProfilesConfig{Version: 1, Profiles: []authpkg.Profile{{CorpID: token.CorpID}}}); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.WriteFile(authpkg.ProfilesPath(dir), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "not_logged_in":
				if err := authpkg.SaveProfiles(dir, &authpkg.ProfilesConfig{Version: 2}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(authpkg.ProfilesPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			cmd := newAuthStatusCommand()
			cmd.SetArgs([]string{"--readonly"})
			cmd.PersistentFlags().String("format", "json", "")
			var out bytes.Buffer
			cmd.SetOut(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var response authStatusResponse
			if err := json.Unmarshal(out.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			wantReason := map[string]string{"repair_required": "local_state_requires_repair", "unreadable": "local_state_unreadable"}[state]
			if !response.Success || response.Reason != wantReason {
				t.Fatalf("reason=%q, want %q: %s", response.Reason, wantReason, out.String())
			}
			if response.Authenticated != (state == "valid" || state == "expired") {
				t.Fatal("readonly did not preserve local login semantics")
			}
			if state == "expired" && response.TokenValid {
				t.Fatal("readonly refreshed the expired access token")
			}
			if response.Refreshed {
				t.Fatal("readonly reported a refresh")
			}
			if strings.Contains(out.String(), token.AccessToken) || strings.Contains(out.String(), token.RefreshToken) {
				t.Fatal("readonly exposed credentials")
			}
			after, err := os.ReadFile(authpkg.ProfilesPath(dir))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("readonly changed profile storage")
			}
			stored, err := authpkg.LoadTokenDataKeychainForIdentity(token.CorpID, token.UserID)
			if err != nil || stored.AccessToken != token.AccessToken || !stored.ExpiresAt.Equal(token.ExpiresAt) {
				t.Fatalf("readonly refreshed or mutated credentials: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageAuthStatusReadOnlySkipsRuntimeCredentialHooks(t *testing.T) {
	setupAuthLogoutProfiles(t, authLogoutTestToken("corp-readonly-hooks"))
	previousArgs := os.Args
	os.Args = []string{"dws", "--profile", "corp-readonly-hooks", "auth", "status", "--readonly", "--format", "json"}
	t.Cleanup(func() { os.Args = previousArgs })
	previousHooks := edition.Get()
	hooks := *previousHooks
	hooks.RegisterExtraCommands = func(*cobra.Command, edition.ToolCaller) { t.Fatal("readonly registered runtime extensions") }
	hooks.AfterPersistentPreRun = func(*cobra.Command, []string) error { t.Fatal("readonly invoked credential hooks"); return nil }
	edition.Override(&hooks)
	t.Cleanup(func() { edition.Override(previousHooks) })
	testseam.Swap(t, &rootAuthLoadTokenData, func(string) (*authpkg.TokenData, error) {
		t.Fatal("readonly used the public token preload")
		return nil, nil
	})
	cmd := NewRootCommand(context.WithValue(context.Background(), authStatusProcessStartupKey{}, true))
	target, _, err := cmd.Find(os.Args[1:])
	if err != nil {
		t.Fatal(err)
	}
	if target.Flags().Changed("readonly") || cmd.PersistentFlags().Changed("format") {
		t.Fatal("startup detection mutated flags before command execution")
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"authenticated": true`) {
		t.Fatalf("readonly output=%s", out.String())
	}
}

func TestCrossPlatformCoverageAuthStatusRetainsLocking(t *testing.T) {
	dir := setupAuthLogoutProfiles(t, authLogoutTestToken("corp-status-lock"))
	cmd := newAuthStatusCommand()
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(&bytes.Buffer{})
	lock, err := authpkg.AcquireDualLock(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case err := <-done:
		t.Fatalf("status bypassed the auth lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	lock.Release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageAuthStatusReadOnlyStatusRepairWorkflow(t *testing.T) {
	token := authLogoutTestToken("corp-readonly-repair")
	dir := setupAuthLogoutProfiles(t, token)
	if err := authpkg.SaveProfiles(dir, &authpkg.ProfilesConfig{Version: 1, CurrentProfile: token.CorpID, Profiles: []authpkg.Profile{{CorpID: token.CorpID}}}); err != nil {
		t.Fatal(err)
	}
	for step, readonlyMode := range []bool{true, false, true} {
		cmd := newAuthStatusCommand()
		cmd.SetArgs([]string{})
		if readonlyMode {
			cmd.SetArgs([]string{"--readonly"})
		}
		cmd.PersistentFlags().String("format", "json", "")
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var response authStatusResponse
		if err := json.Unmarshal(out.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if step == 0 {
			if response.Reason != "local_state_requires_repair" || response.Authenticated {
				t.Fatalf("readonly did not report pending repair: %s", out.String())
			}
		} else if !response.Authenticated || response.Reason != "" {
			t.Fatalf("status did not repair login: %s", out.String())
		}
	}
	data, err := authpkg.ReadTokenDataForProfile(dir, "")
	if err != nil || data == nil || data.UserID != token.UserID {
		t.Fatalf("snapshot after status repair: %#v, %v", data, err)
	}
}

// Optional black-box release check against the signed, assembled executable.
// Run with DWS_AUTH_STATUS_READONLY_BINARY=/absolute/path/dws after make build.
func TestCrossPlatformCoverageAuthStatusReadOnlyBuiltProcessWhileLocked(t *testing.T) {
	binary := os.Getenv("DWS_AUTH_STATUS_READONLY_BINARY")
	if binary == "" {
		t.Skip("set DWS_AUTH_STATUS_READONLY_BINARY to test the built CLI")
	}
	t.Setenv("DO_NOT_TRACK", "1")
	token := authLogoutTestToken("corp-readonly-process")
	dir := setupAuthLogoutProfiles(t, token)
	lock, err := authpkg.AcquireDualLock(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "auth", "status", "--readonly", "--format", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("built readonly failed while locked: %v; stderr=%s", err, stderr.String())
	}
	var response authStatusResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.CorpID != token.CorpID || !response.Authenticated || !response.TokenValid {
		t.Fatalf("built readonly response=%+v", response)
	}
	if strings.Contains(stdout.String(), token.AccessToken) || strings.Contains(stdout.String(), token.RefreshToken) {
		t.Fatal("built readonly exposed credentials")
	}
}

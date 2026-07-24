// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageAuthLoginUsesStableBlankProfileForPostLoginAuthorization(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	oldOAuth := authOAuthLogin
	oldLoadProfiles := authLoadProfiles
	oldRecommend := authRunLoginRecommend
	oldInteractive := authLoginInteractiveTerminal
	oldResolve := authResolveProfile
	t.Cleanup(func() {
		authOAuthLogin = oldOAuth
		authLoadProfiles = oldLoadProfiles
		authRunLoginRecommend = oldRecommend
		authLoginInteractiveTerminal = oldInteractive
		authResolveProfile = oldResolve
	})

	const corpID = "corp_post_login_blank"
	cfg := &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{
		{Name: "Fixture Organization", CorpID: corpID, CorpName: "Fixture Organization"},
		{Name: "Exact Fixture", CorpID: corpID, CorpName: "Fixture Organization", UserID: "identity_exact"},
	}}
	wantSelector := authpkg.ProfileSelectionSelector(cfg.Profiles[0], cfg)
	if wantSelector == "" || wantSelector == corpID {
		t.Fatalf("blank selector = %q, want a stable account selector", wantSelector)
	}
	authResolveProfile = func(string, string) (*authpkg.Profile, error) {
		return nil, errors.New("no implicit profile")
	}
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return cfg, nil }
	authOAuthLogin = func(*authpkg.OAuthProvider, context.Context, bool) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{
			AccessToken: "new-access",
			ExpiresAt:   time.Now().Add(time.Hour),
			CorpID:      corpID,
		}, nil
	}
	authLoginInteractiveTerminal = func() bool { return false }
	seenSelector := ""
	authRunLoginRecommend = func(context.Context, edition.ToolCaller, io.Writer, pat.LoginRecommendOptions) error {
		seenSelector = authpkg.RuntimeProfile()
		return nil
	}
	if _, _, err := authCoverageRunLogin(t, nil, "table", true, map[string]string{"recommend": "true"}); err != nil {
		t.Fatalf("blank-profile login error = %v", err)
	}
	if seenSelector != wantSelector {
		t.Fatalf("post-login runtime selector = %q, want %q", seenSelector, wantSelector)
	}
}

func TestCrossPlatformCoverageAuthStatusAndLogoutPreserveExactSelectors(t *testing.T) {
	t.Run("status canonicalizes a known identity", func(t *testing.T) {
		configDir := t.TempDir()
		t.Setenv("DWS_CONFIG_DIR", configDir)
		const exactSelector = "corp_status_fixture:identity_status_fixture"
		if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{
			Version: 2,
			Profiles: []authpkg.Profile{{
				Name:   "Status Fixture",
				CorpID: "corp_status_fixture",
				UserID: "identity_status_fixture",
			}},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}

		oldStatus := authOAuthStatus
		t.Cleanup(func() { authOAuthStatus = oldStatus })
		seenSelector := ""
		authOAuthStatus = func(*authpkg.OAuthProvider) (*authpkg.TokenData, error) {
			seenSelector = authpkg.RuntimeProfile()
			return &authpkg.TokenData{
				AccessToken: "access",
				ExpiresAt:   time.Now().Add(time.Hour),
				CorpID:      "corp_status_fixture",
				UserID:      "identity_status_fixture",
			}, nil
		}
		cmd := newAuthStatusCommand()
		_, _, _ = authCoverageRoot(cmd, "table", false)
		if err := cmd.Flags().Set("profile", " Status Fixture "); err != nil {
			t.Fatal(err)
		}
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("auth status error = %v", err)
		}
		if seenSelector != exactSelector {
			t.Fatalf("status runtime selector = %q, want %q", seenSelector, exactSelector)
		}
	})

	t.Run("logout keeps a blank local selector", func(t *testing.T) {
		oldResolve := authResolveProfileDeletion
		oldLoad := authLoadTokenForProfile
		oldRevoke := authRevokeTokenForData
		oldDelete := authDeleteProfileToken
		t.Cleanup(func() {
			authResolveProfileDeletion = oldResolve
			authLoadTokenForProfile = oldLoad
			authRevokeTokenForData = oldRevoke
			authDeleteProfileToken = oldDelete
		})

		const selector = "legacy-external-worker"
		authResolveProfileDeletion = func(string, string) (*authpkg.Profile, bool, error) {
			return &authpkg.Profile{CorpID: "corp_logout_blank"}, true, nil
		}
		loadedSelector := ""
		authLoadTokenForProfile = func(_ string, got string) (*authpkg.TokenData, error) {
			loadedSelector = got
			return &authpkg.TokenData{CorpID: "corp_logout_blank"}, nil
		}
		authRevokeTokenForData = func(context.Context, *authpkg.TokenData) error { return nil }
		deletedSelector := ""
		authDeleteProfileToken = func(_ string, got string) error {
			deletedSelector = got
			return nil
		}

		if err := logoutOneProfile(nil, context.Background(), "cfg", "  "+selector+"  "); err != nil {
			t.Fatalf("logoutOneProfile() error = %v", err)
		}
		if loadedSelector != selector || deletedSelector != selector {
			t.Fatalf("blank logout selectors = load %q delete %q, want %q", loadedSelector, deletedSelector, selector)
		}
	})
}

func TestCrossPlatformCoverageAuthHistorySelectorRemainingBranches(t *testing.T) {
	if got := authLoginHistorySelector("cfg", nil); got != "" {
		t.Fatalf("nil history selector = %q", got)
	}

	oldLoad := authLoadProfiles
	t.Cleanup(func() { authLoadProfiles = oldLoad })
	authLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return nil, errors.New("profiles unavailable")
	}
	profile := &authpkg.Profile{CorpID: "corp_history", UserID: "identity_history"}
	if got := authLoginHistorySelector("cfg", profile); got != "corp_history:identity_history" {
		t.Fatalf("history selector fallback = %q", got)
	}

	duplicateA := &authpkg.Profile{CorpID: "corp_history", UserID: "duplicate_identity"}
	duplicateB := &authpkg.Profile{CorpID: "corp_history", UserID: "duplicate_identity"}
	if got := historicalProfileForSelector(
		"corp_history",
		"corp_history:duplicate_identity",
		[]*authpkg.Profile{duplicateA, duplicateB},
	); got != nil {
		t.Fatalf("duplicate stable identity selected %#v", got)
	}

	// Whitespace keeps the raw selector from matching the stable string while
	// ParseIdentitySelector still resolves its components.
	exactFallback := &authpkg.Profile{CorpID: "corp_history", UserID: "fallback_identity"}
	if got := historicalProfileForSelector(
		"corp_history",
		"corp_history : fallback_identity",
		[]*authpkg.Profile{exactFallback},
	); got != exactFallback {
		t.Fatalf("exact history fallback = %#v, want %#v", got, exactFallback)
	}
}

func TestCrossPlatformCoverageProfileSwitchLegacyBlankAndNormalizedIdentityPointers(t *testing.T) {
	t.Run("one legacy blank name", func(t *testing.T) {
		profiles := []authpkg.Profile{
			{Name: "Fixture Organization", CorpID: "corp_profile_fixture", CorpName: "Fixture Organization"},
			{Name: "Exact Fixture", CorpID: "corp_profile_fixture", CorpName: "Fixture Organization", UserID: "identity_exact"},
		}
		cfg := &authpkg.ProfilesConfig{Profiles: profiles}
		if got := profileSwitchProfileIndex(profiles, "Fixture Organization", cfg); got != 0 {
			t.Fatalf("legacy blank profile index = %d, want 0", got)
		}
	})

	t.Run("duplicate legacy names fall through to blank-name compatibility", func(t *testing.T) {
		profiles := []authpkg.Profile{
			{Name: "duplicate-legacy", CorpID: "corp_profile_fixture"},
			{Name: "duplicate-legacy", CorpID: "corp_profile_fixture"},
		}
		cfg := &authpkg.ProfilesConfig{Profiles: profiles}
		if got := profileSwitchProfileIndex(profiles, "duplicate-legacy", cfg); got != 0 {
			t.Fatalf("duplicate legacy fallback index = %d, want 0", got)
		}
	})

	t.Run("normalized exact identity", func(t *testing.T) {
		profiles := []authpkg.Profile{{CorpID: "corp_profile_fixture", UserID: "identity_exact"}}
		cfg := &authpkg.ProfilesConfig{Profiles: profiles}
		if got := profileSwitchProfileIndex(profiles, "corp_profile_fixture : identity_exact", cfg); got != 0 {
			t.Fatalf("normalized exact profile index = %d, want 0", got)
		}
		if got := profileSwitchProfileIndex(profiles, "corp_profile_fixture : missing", cfg); got != -1 {
			t.Fatalf("missing normalized exact profile index = %d, want -1", got)
		}
	})
}

func TestCrossPlatformCoverageRuntimeRunnerPreservesBlankSelectorInSingleAndMultiRuns(t *testing.T) {
	exact := authLogoutTestToken("corp_runner_blank")
	exact.UserID = "identity_exact_runner"
	other := authLogoutTestToken("corp_runner_other")
	configDir := setupAuthLogoutProfiles(t, exact, other)
	blank := authLogoutTestToken("corp_runner_blank")
	blank.AccessToken = "access-unresolved-runner"
	blank.RefreshToken = "refresh-unresolved-runner"
	blank.UserID = ""
	blank.UserName = ""
	if err := authpkg.SaveTokenData(configDir, blank); err != nil {
		t.Fatalf("SaveTokenData(blank) error = %v", err)
	}
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	blankSelector := ""
	for _, profile := range cfg.Profiles {
		if profile.CorpID == blank.CorpID && profile.UserID == "" {
			blankSelector = authpkg.ProfileSelectionSelector(profile, cfg)
			break
		}
	}
	if blankSelector == "" || blankSelector == blank.CorpID {
		t.Fatalf("blank runner selector = %q, want exact local selector", blankSelector)
	}

	runner := &runtimeRunner{fallback: multiProfileFallbackRunner{}}
	invocation := executor.Invocation{
		Kind:             "helper_invocation",
		CanonicalProduct: "contact",
		Tool:             "get_current_user_profile",
	}
	authpkg.SetRuntimeProfile(blankSelector)
	result, err := runner.Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("single blank Run() error = %v", err)
	}
	content := result.Response["content"].(map[string]any)
	if got := content["runtimeProfile"]; got != blankSelector {
		t.Fatalf("single blank runtime profile = %#v, want %q", got, blankSelector)
	}
	if got := authpkg.RuntimeProfile(); got != blankSelector {
		t.Fatalf("single blank runtime restoration = %q, want %q", got, blankSelector)
	}

	authpkg.SetRuntimeProfile(blankSelector + ",corp_runner_other")
	result, err = runner.Run(context.Background(), invocation)
	if err != nil {
		t.Fatalf("multi blank Run() error = %v", err)
	}
	entries := result.Response["content"].(map[string]any)["profiles"].([]any)
	if len(entries) != 2 {
		t.Fatalf("multi blank profiles = %#v, want two", entries)
	}
	first := entries[0].(map[string]any)
	if first["selector"] != blankSelector || first["profile"] != blankSelector || first["userId"] != "" {
		t.Fatalf("multi blank first entry = %#v", first)
	}
}

func TestCrossPlatformCoveragePersonalBusSelectorCanonicalFallback(t *testing.T) {
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })
	identity := personal.Identity{
		CorpID:   "corp_event_fallback",
		UserID:   "identity_event_fallback",
		SourceID: "open",
	}
	if got := personalBusProfileSelector(t.TempDir(), identity); got != "corp_event_fallback:identity_event_fallback" {
		t.Fatalf("personal bus fallback selector = %q", got)
	}
	args := personalBusSpawnArgs(identity, "", "", "   ")
	if got := strings.Join(args, " "); !strings.Contains(got, "--profile corp_event_fallback:identity_event_fallback") {
		t.Fatalf("personal bus default profile args = %q", got)
	}
}

package app

import (
	"context"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
)

func TestRuntimeRunnerAggregatesCommaSeparatedProfiles(t *testing.T) {
	setupAuthLogoutProfiles(t,
		authLogoutTestToken("corp_a"),
		authLogoutTestToken("corp_b"),
	)
	authpkg.SetRuntimeProfile("corp_a, corp_b")

	runner := &runtimeRunner{fallback: multiProfileFallbackRunner{}}
	result, err := runner.Run(context.Background(), executor.Invocation{
		Kind:             "helper_invocation",
		CanonicalProduct: "contact",
		Tool:             "get_current_user_profile",
		Params:           map[string]any{"limit": 10},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := authpkg.RuntimeProfile(); got != "corp_a, corp_b" {
		t.Fatalf("runtime profile after Run = %q, want restored raw selector", got)
	}

	content := result.Response["content"].(map[string]any)
	if content["multiProfile"] != true {
		t.Fatalf("multiProfile = %#v, want true", content["multiProfile"])
	}
	if content["success"] != true {
		t.Fatalf("success = %#v, want true", content["success"])
	}
	profiles := content["profiles"].([]any)
	if len(profiles) != 2 {
		t.Fatalf("profiles len = %d, want 2", len(profiles))
	}
	for i, wantCorpID := range []string{"corp_a", "corp_b"} {
		entry := profiles[i].(map[string]any)
		if entry["corpId"] != wantCorpID {
			t.Fatalf("profiles[%d].corpId = %#v, want %q", i, entry["corpId"], wantCorpID)
		}
		if entry["ok"] != true {
			t.Fatalf("profiles[%d].ok = %#v, want true", i, entry["ok"])
		}
		resultPayload := entry["result"].(map[string]any)
		wantProfile := wantCorpID + ":user-" + wantCorpID
		if resultPayload["runtimeProfile"] != wantProfile {
			t.Fatalf("profiles[%d].result.runtimeProfile = %#v, want %q", i, resultPayload["runtimeProfile"], wantProfile)
		}
	}
}

func TestRuntimeRunnerDeduplicatesCommaSeparatedProfilesByCorpID(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t, authLogoutTestToken("corp_a"), authLogoutTestToken("corp_b"))
	authpkg.SetRuntimeProfile("corp_a, corp_a org,corp_b")

	selections, multi, err := resolveMultiProfileSelections(configDir, authpkg.RuntimeProfile())
	if err != nil {
		t.Fatalf("resolveMultiProfileSelections() error = %v", err)
	}
	if !multi {
		t.Fatal("multi = false, want true")
	}
	if len(selections) != 2 {
		t.Fatalf("selections len = %d, want 2", len(selections))
	}
	if selections[0].Profile.CorpID != "corp_a" || selections[1].Profile.CorpID != "corp_b" {
		t.Fatalf("resolved corp IDs = %q, %q; want corp_a, corp_b", selections[0].Profile.CorpID, selections[1].Profile.CorpID)
	}
}

func TestRuntimeRunnerDeduplicatesByResolvedIdentityInSameCorp(t *testing.T) {
	first := authLogoutTestToken("corp_same")
	first.UserID = "user_1"
	second := authLogoutTestToken("corp_same")
	second.AccessToken = "access-second"
	second.RefreshToken = "refresh-second"
	second.UserID = "user_2"
	configDir := setupAuthLogoutProfiles(t, first, second)

	selections, multi, err := resolveMultiProfileSelections(
		configDir,
		"corp_same,corp_same:user_1,corp_same:user_2",
	)
	if err != nil {
		t.Fatalf("resolveMultiProfileSelections() error = %v", err)
	}
	if !multi {
		t.Fatal("multi = false, want true")
	}
	if len(selections) != 2 {
		t.Fatalf("selections len = %d, want 2: %#v", len(selections), selections)
	}
	got := []string{
		authpkg.ProfileSelector(selections[0].Profile),
		authpkg.ProfileSelector(selections[1].Profile),
	}
	if strings.Join(got, ",") != "corp_same:user_2,corp_same:user_1" {
		t.Fatalf("resolved identities = %v, want current user_2 then user_1", got)
	}
}

func TestRuntimeRunnerKeepsSingleProfileBehavior(t *testing.T) {
	setupAuthLogoutProfiles(t, authLogoutTestToken("corp_a"), authLogoutTestToken("corp_b"))
	authpkg.SetRuntimeProfile("corp_a")

	runner := &runtimeRunner{fallback: multiProfileFallbackRunner{}}
	result, err := runner.Run(context.Background(), executor.Invocation{
		Kind:             "helper_invocation",
		CanonicalProduct: "contact",
		Tool:             "get_current_user_profile",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, ok := result.Response["content"].(map[string]any)["multiProfile"]; ok {
		t.Fatalf("single profile unexpectedly returned aggregate content: %#v", result.Response)
	}
	if got := result.Response["content"].(map[string]any)["runtimeProfile"]; got != "corp_a:user-corp_a" {
		t.Fatalf("fallback runtime profile = %#v, want exact identity selector", got)
	}
	if got := authpkg.RuntimeProfile(); got != "corp_a" {
		t.Fatalf("runtime profile after Run = %q, want corp_a", got)
	}
}

func TestRuntimeRunnerRejectsAmbiguousSingleProfile(t *testing.T) {
	first := authLogoutTestToken("corp_first")
	first.CorpName = "Shared Org"
	second := authLogoutTestToken("corp_second")
	second.CorpName = "Shared Org"
	setupAuthLogoutProfiles(t, first, second)
	authpkg.SetRuntimeProfile("Shared Org")

	runner := &runtimeRunner{fallback: multiProfileFallbackRunner{}}
	_, err := runner.Run(context.Background(), executor.Invocation{
		Kind:             "helper_invocation",
		CanonicalProduct: "contact",
		Tool:             "get_current_user_profile",
	})
	if err == nil {
		t.Fatal("Run() accepted ambiguous single profile selector")
	}
	for _, candidate := range []string{"corp_first", "corp_second"} {
		if !strings.Contains(err.Error(), candidate) {
			t.Fatalf("error = %q, want candidate %q", err.Error(), candidate)
		}
	}
}

func TestCommaNamedProfileStillResolvesAsSingleProfile(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t, authLogoutTestToken("corp_comma"), authLogoutTestToken("corp_other"))
	cfg, err := authpkg.LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	for i := range cfg.Profiles {
		if cfg.Profiles[i].CorpID == "corp_comma" {
			cfg.Profiles[i].Name = "alpha,beta"
		}
	}
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	selections, multi, err := resolveMultiProfileSelections(configDir, "alpha,beta")
	if err != nil {
		t.Fatalf("resolveMultiProfileSelections() error = %v", err)
	}
	if multi {
		t.Fatalf("multi = true, want false; selections=%#v", selections)
	}
}

func TestCommaSeparatedProfileRejectsEmptySelector(t *testing.T) {
	configDir := setupAuthLogoutProfiles(t, authLogoutTestToken("corp_a"), authLogoutTestToken("corp_b"))

	_, _, err := resolveMultiProfileSelections(configDir, "corp_a,,corp_b")
	if err == nil {
		t.Fatal("resolveMultiProfileSelections() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "empty profile selector") {
		t.Fatalf("error = %q, want empty profile selector", err.Error())
	}
}

type multiProfileFallbackRunner struct{}

func (multiProfileFallbackRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	invocation.Implemented = true
	return executor.Result{
		Invocation: invocation,
		Response: map[string]any{
			"content": map[string]any{
				"runtimeProfile": authpkg.RuntimeProfile(),
				"tool":           invocation.Tool,
			},
		},
	}, nil
}

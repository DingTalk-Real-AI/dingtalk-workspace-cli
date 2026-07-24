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
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// A bounded run never arms the stdin-EOF watcher, regardless of stdin
// shape: --max-events / --duration are the lifecycle control.
func TestShouldWatchStdinEOF_BoundedIsNeverArmed(t *testing.T) {
	if shouldWatchStdinEOF(1, 0) {
		t.Error("--max-events set should not arm stdin watcher")
	}
	if shouldWatchStdinEOF(0, 5*time.Second) {
		t.Error("--duration set should not arm stdin watcher")
	}
	if shouldWatchStdinEOF(3, 2*time.Second) {
		t.Error("both bounds set should not arm stdin watcher")
	}
}

// Regression: the detached _bus child must receive --profile so it resolves
// credentials for the same organization as the parent. Missing it made a
// non-default `--profile` consume fail with "bus child reported startup
// failure on ready pipe" (no bus.log).
func TestPersonalBusSpawnArgs_ForwardsProfile(t *testing.T) {
	args := personalBusSpawnArgs(personal.Identity{
		CorpID:   "dinga626d60c1128d449",
		UserID:   "user_123",
		SourceID: "open",
	}, "", "")
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--profile" && args[i+1] == "dinga626d60c1128d449:user_123" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("spawn args must forward --profile <corpId>:<userId>; got %v", args)
	}

	// No CorpID → no --profile appended (avoid an empty flag value).
	bare := personalBusSpawnArgs(personal.Identity{SourceID: "open"}, "", "")
	for _, a := range bare {
		if a == "--profile" {
			t.Errorf("must not append --profile when CorpID is empty; got %v", bare)
		}
	}
}

func TestCrossPlatformCoveragePersonalBusSpawnArgsPreservesReservedBlankProfile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	cfg := &authpkg.ProfilesConfig{
		Version: 2,
		OrgCurrentProfiles: map[string]string{
			"corp_event_fixture": "corp_event_fixture:identity_exact_fixture",
		},
		Profiles: []authpkg.Profile{
			{
				Name:     "Fixture Organization",
				CorpID:   "corp_event_fixture",
				CorpName: "Fixture Organization",
			},
			{
				Name:     "Exact Fixture Account",
				CorpID:   "corp_event_fixture",
				CorpName: "Fixture Organization",
				UserID:   "identity_exact_fixture",
			},
		},
	}
	blankSelector := authpkg.ProfileSelectionSelector(cfg.Profiles[0], cfg)
	cfg.PrimaryProfile = blankSelector
	cfg.CurrentProfile = blankSelector
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	blankToken := &authpkg.TokenData{
		AccessToken: "parent-blank-token",
		CorpID:      "corp_event_fixture",
		CorpName:    "Fixture Organization",
	}
	exactToken := &authpkg.TokenData{
		AccessToken: "other-exact-token",
		CorpID:      "corp_event_fixture",
		CorpName:    "Fixture Organization",
		UserID:      "identity_exact_fixture",
	}
	if err := authpkg.SaveTokenDataKeychainForCorpID(blankToken.CorpID, blankToken); err != nil {
		t.Fatalf("save parent blank token: %v", err)
	}
	if err := authpkg.SaveTokenDataKeychainForIdentity(exactToken.CorpID, exactToken.UserID, exactToken); err != nil {
		t.Fatalf("save other exact token: %v", err)
	}

	// Runtime identity enrichment points at the exact sibling, but the parent
	// already loaded the persisted blank current profile.
	identity := personal.Identity{
		CorpID:   "corp_event_fixture",
		UserID:   "identity_exact_fixture",
		SourceID: "open",
	}
	selector := personalBusProfileSelector(configDir, identity)
	want := blankSelector
	if selector != want || selector == identity.CorpID {
		t.Fatalf("personalBusProfileSelector(blank) = %q, want reserved %q", selector, want)
	}
	args := personalBusSpawnArgs(identity, "", "", selector)
	forwardedSelector := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--profile" && args[i+1] == want {
			forwardedSelector = args[i+1]
			break
		}
	}
	if forwardedSelector == "" {
		t.Fatalf("spawn args did not preserve reserved blank selector: %v", args)
	}
	parentToken, err := authpkg.LoadTokenDataForProfile(configDir, selector)
	if err != nil {
		t.Fatalf("load parent token: %v", err)
	}
	authpkg.SetRuntimeProfile(forwardedSelector)
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })
	childToken, err := authpkg.LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("load detached child token: %v", err)
	}
	if parentToken.AccessToken != blankToken.AccessToken ||
		childToken.AccessToken != parentToken.AccessToken ||
		childToken.UserID != "" {
		t.Fatalf("parent/child token drift: parent=%#v child=%#v", parentToken, childToken)
	}

	authpkg.SetRuntimeProfile(want)
	inferredExact := personal.Identity{
		CorpID:   "corp_event_fixture",
		UserID:   "identity_exact_fixture",
		SourceID: "open",
	}
	if got := personalBusProfileSelector(configDir, inferredExact); got != want {
		t.Fatalf("runtime blank selector changed after inferred userId: got %q, want %q", got, want)
	}
}

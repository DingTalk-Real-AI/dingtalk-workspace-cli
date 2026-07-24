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

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
)

func TestPersonalBusProfileSelectorUsesDefaultBlankCurrentBeforeRuntimeEnrichedIdentity(t *testing.T) {
	configDir, cfg, blankSelector, exactSelector := seedPersonalBusProfileSelectorConfig(t)
	cfg.CurrentProfile = blankSelector
	cfg.OrgCurrentProfiles = map[string]string{cfg.Profiles[0].CorpID: exactSelector}
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	identityAfterRuntimeEnrichment := personal.Identity{
		CorpID: cfg.Profiles[0].CorpID,
		UserID: cfg.Profiles[1].UserID,
	}
	if got := personalBusProfileSelector(configDir, identityAfterRuntimeEnrichment); got != blankSelector {
		t.Fatalf("personalBusProfileSelector() = %q, want default blank selector %q", got, blankSelector)
	}
}

func TestPersonalBusProfileSelectorUsesDefaultExactCurrent(t *testing.T) {
	configDir, cfg, _, exactSelector := seedPersonalBusProfileSelectorConfig(t)
	cfg.CurrentProfile = exactSelector
	cfg.OrgCurrentProfiles = map[string]string{cfg.Profiles[0].CorpID: exactSelector}
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	identity := personal.Identity{CorpID: cfg.Profiles[1].CorpID, UserID: cfg.Profiles[1].UserID}
	if got := personalBusProfileSelector(configDir, identity); got != exactSelector {
		t.Fatalf("personalBusProfileSelector() = %q, want default exact selector %q", got, exactSelector)
	}
}

func TestPersonalBusProfileSelectorPrefersExplicitRuntimeSelector(t *testing.T) {
	configDir, cfg, blankSelector, _ := seedPersonalBusProfileSelectorConfig(t)
	cfg.CurrentProfile = blankSelector
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}

	const explicitSelector = "corp_explicit:user_explicit"
	authpkg.SetRuntimeProfile(explicitSelector)
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	identity := personal.Identity{CorpID: cfg.Profiles[1].CorpID, UserID: cfg.Profiles[1].UserID}
	if got := personalBusProfileSelector(configDir, identity); got != explicitSelector {
		t.Fatalf("personalBusProfileSelector() = %q, want explicit selector %q", got, explicitSelector)
	}
}

func TestCrossPlatformCoveragePersonalBusProfileSelectorFallsBackToMatchingIdentity(t *testing.T) {
	configDir, cfg, _, exactSelector := seedPersonalBusProfileSelectorConfig(t)
	cfg.Profiles = append(cfg.Profiles, authpkg.Profile{
		Name:     "Other Current",
		CorpID:   "corp_event_other_fixture",
		CorpName: "Other Fixture Organization",
		UserID:   "identity_event_other_fixture",
	})
	cfg.CurrentProfile = authpkg.ProfileSelectionSelector(cfg.Profiles[2], cfg)
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })

	identity := personal.Identity{CorpID: cfg.Profiles[1].CorpID, UserID: cfg.Profiles[1].UserID}
	if got := personalBusProfileSelector(configDir, identity); got != exactSelector {
		t.Fatalf("personalBusProfileSelector() = %q, want identity fallback %q", got, exactSelector)
	}
}

func seedPersonalBusProfileSelectorConfig(t *testing.T) (string, *authpkg.ProfilesConfig, string, string) {
	t.Helper()
	configDir := t.TempDir()
	cfg := &authpkg.ProfilesConfig{
		Version: 2,
		Profiles: []authpkg.Profile{
			{
				Name:     "External Fixture",
				CorpID:   "corp_event_current_fixture",
				CorpName: "Fixture Organization",
			},
			{
				Name:     "Exact Fixture",
				CorpID:   "corp_event_current_fixture",
				CorpName: "Fixture Organization",
				UserID:   "identity_runtime_enriched_fixture",
			},
		},
	}
	blankSelector := authpkg.ProfileSelectionSelector(cfg.Profiles[0], cfg)
	exactSelector := authpkg.ProfileSelectionSelector(cfg.Profiles[1], cfg)
	if blankSelector == "" || blankSelector == cfg.Profiles[0].CorpID {
		t.Fatalf("blank selector = %q, want stable account selector", blankSelector)
	}
	return configDir, cfg, blankSelector, exactSelector
}

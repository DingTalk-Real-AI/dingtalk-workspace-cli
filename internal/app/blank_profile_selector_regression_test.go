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
)

func blankProfileSelectorAppFixture(blankName, corpName string) *authpkg.ProfilesConfig {
	const (
		corpID      = "corp_selector_fixture"
		exactUserID = "identity_exact_fixture"
	)
	exactSelector := corpID + ":" + exactUserID
	cfg := &authpkg.ProfilesConfig{
		Version:         2,
		PrimaryProfile:  exactSelector,
		PreviousProfile: exactSelector,
		OrgCurrentProfiles: map[string]string{
			corpID: exactSelector,
		},
		Profiles: []authpkg.Profile{
			{
				Name:     "Exact Fixture Account",
				CorpID:   corpID,
				CorpName: corpName,
				UserID:   exactUserID,
				UserName: "Exact Fixture Account",
				Status:   authpkg.ProfileStatusActive,
			},
			{
				Name:     blankName,
				CorpID:   corpID,
				CorpName: corpName,
				Status:   authpkg.ProfileStatusActive,
			},
		},
	}
	cfg.CurrentProfile = authpkg.ProfileSelectionSelector(cfg.Profiles[1], cfg)
	return cfg
}

func captureProfileListSelectors(t *testing.T, cfg *authpkg.ProfilesConfig) ([]string, []profileView) {
	t.Helper()
	originalLoadToken := profileLoadTokenData
	selectors := make([]string, 0, len(cfg.Profiles))
	profileLoadTokenData = func(_ string, selector string) (*authpkg.TokenData, error) {
		selectors = append(selectors, selector)
		return nil, authpkg.ErrTokenDataNotFound
	}
	t.Cleanup(func() { profileLoadTokenData = originalLoadToken })
	views := profileViews("unused-config-dir", cfg)
	return selectors, views
}

func TestCrossPlatformCoverageBlankProfileNameMatchingCorpNameRoundTripsThroughListAndTUI(t *testing.T) {
	cfg := blankProfileSelectorAppFixture("Fixture Organization", "Fixture Organization")
	blank := cfg.Profiles[1]
	blankSelector := authpkg.ProfileSelectionSelector(blank, cfg)

	if blankSelector == blank.Name || blankSelector == blank.CorpID {
		t.Fatalf("unsafe blank selector = %q, want reserved exact selector", blankSelector)
	}
	if got := profileCLISelector(blank, cfg); got != blankSelector {
		t.Errorf("profileCLISelector(blank) = %q, want %q", got, blankSelector)
	}
	if got := profileSwitchProfileIndex(cfg.Profiles, cfg.CurrentProfile, cfg); got != 1 {
		t.Errorf("profileSwitchProfileIndex(blank current) = %d, want 1", got)
	}
	model := newProfileSwitchTUIModel(cfg, cfg.CurrentProfile)
	if model.selected != 1 {
		t.Errorf("TUI selected index = %d, want blank profile index 1", model.selected)
	}
	if got := model.selectedCorpID(); got != blankSelector {
		t.Errorf("TUI selected selector = %q, want %q", got, blankSelector)
	}

	selectors, views := captureProfileListSelectors(t, cfg)
	if len(selectors) != 2 || selectors[0] != cfg.PreviousProfile || selectors[1] != blankSelector {
		t.Errorf("profile list token selectors = %#v, want exact then %q", selectors, blankSelector)
	}
	if len(views) != 2 {
		t.Fatalf("profile list views = %#v, want two entries", views)
	}
	if views[0].IsCurrent {
		t.Error("exact account should not be marked current when blank local selector is current")
	}
	if views[1].Profile != blankSelector || !views[1].IsCurrent {
		t.Errorf("blank list view = %#v, want local selector marked current", views[1])
	}
}

func TestCrossPlatformCoverageBlankProfileNameContainingColonWinsOverIdentityParsingInListAndTUI(t *testing.T) {
	cfg := blankProfileSelectorAppFixture("legacy:outsourced", "Fixture Organization")
	blank := cfg.Profiles[1]
	blankSelector := authpkg.ProfileSelectionSelector(blank, cfg)

	if blankSelector == blank.Name {
		t.Fatalf("colon-containing name leaked as selector %q", blankSelector)
	}
	if _, _, parsedAsIdentity := authpkg.ParseIdentitySelector(blankSelector); parsedAsIdentity {
		t.Fatalf("stable blank selector %q was parsed as an identity", blankSelector)
	}
	if got := profileCLISelector(blank, cfg); got != blankSelector {
		t.Errorf("profileCLISelector(colon blank) = %q, want %q", got, blankSelector)
	}
	if got := profileSwitchProfileIndex(cfg.Profiles, cfg.CurrentProfile, cfg); got != 1 {
		t.Errorf("profileSwitchProfileIndex(colon blank current) = %d, want 1", got)
	}
	model := newProfileSwitchTUIModel(cfg, cfg.CurrentProfile)
	if model.selected != 1 {
		t.Errorf("TUI selected index = %d, want colon-name blank profile index 1", model.selected)
	}
	if got := model.selectedCorpID(); got != blankSelector {
		t.Errorf("TUI selected selector = %q, want %q", got, blankSelector)
	}

	selectors, views := captureProfileListSelectors(t, cfg)
	if len(selectors) != 2 || selectors[0] != cfg.PreviousProfile || selectors[1] != blankSelector {
		t.Errorf("profile list token selectors = %#v, want exact then %q", selectors, blankSelector)
	}
	if len(views) != 2 {
		t.Fatalf("profile list views = %#v, want two entries", views)
	}
	if views[0].IsCurrent {
		t.Error("exact account should not be marked current when colon-name blank selector is current")
	}
	if views[1].Profile != blankSelector || !views[1].IsCurrent {
		t.Errorf("colon-name blank list view = %#v, want local selector marked current", views[1])
	}
}

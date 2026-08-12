// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

func TestPATFreshAuthorizationSaveUsesLoginIsolationBoundary(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	const (
		corpID = "corp_pat_login_boundary"
		userID = "exact-user"
	)
	cfg := &authpkg.ProfilesConfig{
		Version: 2,
		Profiles: []authpkg.Profile{
			{Name: "External Account", CorpID: corpID, CorpName: "PAT Boundary Organization"},
			{Name: "Exact Account", CorpID: corpID, CorpName: "PAT Boundary Organization", UserID: userID},
		},
	}
	blankSelector := authpkg.ProfileSelectionSelector(cfg.Profiles[0], cfg)
	cfg.CurrentProfile = blankSelector
	cfg.PrimaryProfile = blankSelector
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	blank := &authpkg.TokenData{AccessToken: "existing-unresolved", CorpID: corpID, CorpName: "PAT Boundary Organization"}
	exact := &authpkg.TokenData{AccessToken: "existing-exact", CorpID: corpID, CorpName: "PAT Boundary Organization", UserID: userID}
	if err := authpkg.SaveTokenDataKeychainForCorpID(corpID, blank); err != nil {
		t.Fatalf("save unresolved token: %v", err)
	}
	if err := authpkg.SaveTokenDataKeychainForIdentity(corpID, userID, exact); err != nil {
		t.Fatalf("save exact token: %v", err)
	}
	previousRuntimeProfile := authpkg.RuntimeProfile()
	authpkg.SetRuntimeProfile("")
	t.Cleanup(func() { authpkg.SetRuntimeProfile(previousRuntimeProfile) })

	fresh := &authpkg.TokenData{AccessToken: "pat-fresh-unknown", CorpID: corpID, CorpName: "PAT Boundary Organization"}
	err := patSaveTokenData(configDir, fresh)
	if err == nil || !strings.Contains(err.Error(), "fresh UID-less token") {
		t.Fatalf("patSaveTokenData() error = %v, want unresolved-sibling protection", err)
	}
	persisted, loadErr := authpkg.LoadTokenDataKeychainForCorpID(corpID)
	if loadErr != nil || persisted.AccessToken != blank.AccessToken || persisted.UserID != "" {
		t.Fatalf("PAT save changed unresolved sibling: token=%#v err=%v", persisted, loadErr)
	}
}

func TestManualLoginSaveRepairsHalfMigratedGlobalBeforeOverwrite(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	t.Setenv(keychain.StorageDirEnv, t.TempDir())
	const (
		corpID = "corp_manual_login_boundary"
		userID = "legacy-user"
	)
	selector := corpID + ":" + userID
	if err := authpkg.SaveProfiles(configDir, &authpkg.ProfilesConfig{
		Version:        2,
		CurrentProfile: selector,
		Profiles: []authpkg.Profile{{
			Name: "Legacy Exact Account", CorpID: corpID, CorpName: "Manual Boundary Organization", UserID: userID,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	legacy := &authpkg.TokenData{AccessToken: "only-legacy-copy", CorpID: corpID, CorpName: "Manual Boundary Organization"}
	if err := authpkg.SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("save half-migrated global: %v", err)
	}
	manual := &authpkg.TokenData{AccessToken: "manual-default", ExpiresAt: time.Now().Add(time.Hour)}
	if err := authSaveTokenData(configDir, manual); err != nil {
		t.Fatalf("authSaveTokenData(manual) error = %v", err)
	}
	org, err := authpkg.LoadTokenDataKeychainForCorpID(corpID)
	if err != nil || org.AccessToken != legacy.AccessToken || org.UserID != "" {
		t.Fatalf("organization repair = %#v, %v", org, err)
	}
	identity, err := authpkg.LoadTokenDataKeychainForIdentity(corpID, userID)
	if err != nil || identity.AccessToken != legacy.AccessToken || identity.UserID != userID {
		t.Fatalf("identity repair = %#v, %v", identity, err)
	}
	global, err := authpkg.LoadTokenDataKeychain()
	if err != nil || global.AccessToken != manual.AccessToken || global.CorpID != "" {
		t.Fatalf("manual global = %#v, %v", global, err)
	}
}

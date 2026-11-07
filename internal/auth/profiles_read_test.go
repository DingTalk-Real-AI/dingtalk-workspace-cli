// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func setupUnresolvedProfileSelection(t *testing.T, version int) (string, *TokenData) {
	t.Helper()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	dir := t.TempDir()
	data := testToken("profile-selection-fixture", "corp-selection", "Selection Org")
	if err := SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
		t.Fatal(err)
	}
	cfg := &ProfilesConfig{
		Version:        version,
		CurrentProfile: data.CorpID,
		Profiles: []Profile{{
			Name: "historical", CorpID: data.CorpID, CorpName: data.CorpName,
		}},
	}
	// Preserve the historical on-disk version; SaveProfiles may downgrade a
	// v3 registry that no longer contains reserved selectors.
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProfilesPath(dir), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return dir, data
}

func TestCrossPlatformCoverageProfileSelectionIdentityMigration(t *testing.T) {
	for _, version := range []int{profilesVersion, profilesUnresolvedSelectorVersion} {
		for _, selectorKind := range []string{"corp:userId", "corp:userName", "org:userId", "org:userName"} {
			t.Run(fmt.Sprintf("v%d/%s", version, selectorKind), func(t *testing.T) {
				dir, data := setupUnresolvedProfileSelection(t, version)
				selectors := map[string]string{
					"corp:userId":   data.CorpID + ":" + data.UserID,
					"corp:userName": data.CorpID + ":" + data.UserName,
					"org:userId":    data.CorpName + ":" + data.UserID,
					"org:userName":  data.CorpName + ":" + data.UserName,
				}
				selector := selectors[selectorKind]
				acquire := profilesAcquireDualLock
				locks := 0
				testseam.Swap(t, &profilesAcquireDualLock, func(ctx context.Context, dir string) (*DualLock, error) {
					locks++
					return acquire(ctx, dir)
				})
				profile, exact, err := ResolveProfileWithScope(dir, selector)
				if err != nil || profile == nil || profile.UserID != data.UserID || profile.UserName != data.UserName || !exact || locks != 1 {
					t.Fatalf("resolve %q = %#v, exact=%v, error=%v, locks=%d", selector, profile, exact, err, locks)
				}
				loaded, err := LoadTokenDataKeychainForIdentity(data.CorpID, data.UserID)
				if err != nil || loaded == nil || loaded.AccessToken != data.AccessToken {
					t.Fatalf("migrated identity = %#v, %v", loaded, err)
				}
				for index, readSelector := range []string{selector, data.CorpID, ""} {
					if profile, err := ResolveProfile(dir, readSelector); err != nil || profile == nil || profile.UserID != data.UserID || locks != index+2 {
						t.Fatalf("normal resolve %q = %#v, %v, locks=%d", readSelector, profile, err, locks)
					}
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDuplicateHistoricalAccountNameMigration(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprintf("direct-token-read=%v", direct), func(t *testing.T) {
			dir, data := setupUnresolvedProfileSelection(t, profilesVersion)
			cfg, err := LoadProfiles(dir)
			if err != nil {
				t.Fatal(err)
			}
			cfg.CurrentProfile = "historical"
			cfg.Profiles[0].UserName = data.UserName
			cfg.Profiles = append(cfg.Profiles, Profile{Name: "exact", CorpID: data.CorpID, UserID: data.UserID, UserName: data.UserName})
			if err := SaveProfiles(dir, cfg); err != nil {
				t.Fatal(err)
			}
			selector := data.CorpID + ":" + data.UserName
			acquire := profilesAcquireDualLock
			locks := 0
			testseam.Swap(t, &profilesAcquireDualLock, func(ctx context.Context, dir string) (*DualLock, error) {
				locks++
				return acquire(ctx, dir)
			})
			if direct {
				got, err := LoadTokenDataForProfile(dir, selector)
				if err != nil || got == nil || got.AccessToken != data.AccessToken {
					t.Fatalf("duplicate historical identity read = %#v, %v", got, err)
				}
			} else {
				profile, exact, err := ResolveProfileWithScope(dir, selector)
				if err != nil || profile == nil || profile.UserID != data.UserID || !exact {
					t.Fatalf("duplicate historical identity resolution = %#v, exact=%v, %v", profile, exact, err)
				}
			}
			cfg, err = LoadProfiles(dir)
			if err != nil || len(cfg.Profiles) != 1 || cfg.Profiles[0].UserID != data.UserID || locks != 1 {
				t.Fatalf("duplicate placeholder migration = %#v, %v, locks=%d", cfg, err, locks)
			}
		})
	}
}

func TestCrossPlatformCoverageTokenSnapshotSelectionProbeFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*ProfilesConfig, *TokenData)
		load      func(*TokenData) (*TokenData, error)
		selector  string
		noProbe   bool
	}{
		{name: "different account", selector: "corp-selection:other-user"},
		{name: "missing token", load: func(*TokenData) (*TokenData, error) { return nil, ErrTokenDataNotFound }},
		{name: "nil token", load: func(*TokenData) (*TokenData, error) { return nil, nil }},
		{name: "missing token userId", configure: func(_ *ProfilesConfig, data *TokenData) { data.UserID = "" }},
		{name: "wrong organization", configure: func(_ *ProfilesConfig, data *TokenData) { data.CorpID = "other-corp" }},
		{name: "unavailable token store", load: func(*TokenData) (*TokenData, error) {
			return nil, keychain.NewUnavailableError("read profile token", errors.New("unavailable"))
		}},
		{name: "unknown local name", selector: "unknown", noProbe: true},
		{name: "unknown organization", selector: "unknown:user", noProbe: true},
		{name: "ambiguous organization", selector: "Selection Org:user_corp-selection", noProbe: true, configure: func(cfg *ProfilesConfig, _ *TokenData) {
			cfg.Profiles = append(cfg.Profiles, Profile{Name: "other", CorpID: "other-corp", CorpName: "Selection Org"})
		}},
		{name: "no unresolved profile", noProbe: true, configure: func(cfg *ProfilesConfig, _ *TokenData) {
			cfg.Profiles[0].UserID = "other-user"
			cfg.CurrentProfile = "corp-selection:other-user"
		}},
		{name: "historical username is retained", selector: "corp-selection:User corp-selection", configure: func(cfg *ProfilesConfig, _ *TokenData) {
			cfg.Profiles[0].UserName = "Existing Name"
		}},
		{name: "duplicate historical placeholder", selector: "corp-selection:User corp-selection", configure: func(cfg *ProfilesConfig, data *TokenData) {
			cfg.CurrentProfile = "historical"
			cfg.Profiles = append(cfg.Profiles, Profile{Name: "exact", CorpID: data.CorpID, UserID: data.UserID, UserName: "Existing Exact Name"})
		}},
		{name: "ambiguous account name", selector: "corp-selection:Existing Name", configure: func(cfg *ProfilesConfig, data *TokenData) {
			cfg.CurrentProfile = "historical"
			cfg.Profiles[0].UserName = "Existing Name"
			cfg.Profiles = append(cfg.Profiles, Profile{Name: "exact", CorpID: data.CorpID, UserID: "other-user", UserName: "Existing Name"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, data := setupUnresolvedProfileSelection(t, profilesVersion)
			cfg, err := LoadProfiles(dir)
			if err != nil {
				t.Fatal(err)
			}
			selector := tc.selector
			if selector == "" {
				selector = data.CorpID + ":" + data.UserID
			}
			if tc.configure != nil {
				tc.configure(cfg, data)
				if err := SaveProfiles(dir, cfg); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(ProfilesPath(dir))
			if err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
				t.Fatal("unrepairable profile selection acquired the auth lock")
				return nil, nil
			})
			testseam.Swap(t, &profilesLoadCorp, func(corpID string) (*TokenData, error) {
				if tc.noProbe || corpID != "corp-selection" {
					t.Fatalf("unexpected organization token probe: %q", corpID)
				}
				if tc.load != nil {
					return tc.load(data)
				}
				return data, nil
			})
			loaded, readErr := ReadTokenDataForProfile(dir, selector)
			if readErr == nil || loaded != nil {
				t.Fatalf("unrepairable direct token selection = %#v, %v", loaded, readErr)
			}
			if tc.name == "unavailable token store" && !keychain.IsUnavailable(readErr) {
				t.Fatalf("direct token read discarded token store error: %v", readErr)
			}
			after, err := os.ReadFile(ProfilesPath(dir))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("read probe changed profile metadata: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageProfileSelectionMigrationRechecksUnderLock(t *testing.T) {
	for _, action := range []string{"logout", "completed migration", "lock failure"} {
		t.Run(action, func(t *testing.T) {
			dir, data := setupUnresolvedProfileSelection(t, profilesVersion)
			acquire := profilesAcquireDualLock
			locks := 0
			lockFailure := errors.New("fixture lock failure")
			testseam.Swap(t, &profilesAcquireDualLock, func(ctx context.Context, dir string) (*DualLock, error) {
				locks++
				if action == "lock failure" {
					return nil, lockFailure
				}
				lock, err := acquire(ctx, dir)
				if err != nil {
					return nil, err
				}
				if action == "logout" {
					err = SaveProfiles(dir, &ProfilesConfig{Version: profilesVersion})
				} else {
					fresh := *data
					fresh.UserName = "Fresh Name"
					fresh.AccessToken = "fresh-profile-selection-fixture"
					err = SaveTokenDataKeychainForCorpID(data.CorpID, &fresh)
					if err == nil {
						err = ensureProfilesMigrationLocked(dir)
					}
				}
				if err != nil {
					lock.Release()
					return nil, err
				}
				return lock, nil
			})
			profile, err := ResolveProfile(dir, data.CorpID+":"+data.UserID)
			if locks != 1 {
				t.Fatalf("migration locks=%d, want 1", locks)
			}
			if action == "completed migration" {
				if err != nil || profile == nil || profile.UserName != "Fresh Name" {
					t.Fatalf("completed migration resolution = %#v, %v", profile, err)
				}
				loaded, err := LoadTokenDataKeychainForIdentity(data.CorpID, data.UserID)
				if err != nil || loaded == nil || loaded.AccessToken != "fresh-profile-selection-fixture" {
					t.Fatalf("new identity was overwritten: %#v, %v", loaded, err)
				}
				return
			}
			if err == nil || profile != nil {
				t.Fatalf("concurrent %s resolution = %#v, %v", action, profile, err)
			}
			if action == "lock failure" && !errors.Is(err, lockFailure) {
				t.Fatalf("lock failure was discarded: %v", err)
			}
			if _, err := LoadTokenDataKeychainForIdentity(data.CorpID, data.UserID); !errors.Is(err, ErrTokenDataNotFound) {
				t.Fatalf("failed migration resurrected an identity: %v", err)
			}
		})
	}
}

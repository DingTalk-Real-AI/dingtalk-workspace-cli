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

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageTokenReadDoesNotAcquireLock(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("read-snapshot", "corp-read", "Read Org")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
		return nil, errors.New("pure read attempted to acquire the auth lock")
	})
	testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) {
		t.Fatal("already-migrated identity read accessed the organization mirror")
		return nil, nil
	})
	testseam.Swap(t, &profilesLoadCorp, func(string) (*TokenData, error) {
		t.Fatal("already-migrated identity read ran migration inventory")
		return nil, nil
	})
	for _, selector := range []string{"", data.CorpID, profileSelector(data.CorpID, data.UserID)} {
		loaded, err := ReadTokenDataForProfile(configDir, selector)
		if err != nil || loaded == nil || loaded.AccessToken != data.AccessToken {
			t.Fatalf("read selector %q = %#v, %v", selector, loaded, err)
		}
	}
	if _, err := ResolveProfileMetadataReadOnly(configDir, data.CorpID); err != nil {
		t.Fatalf("read-only profile resolution error = %v", err)
	}
}

func TestCrossPlatformCoverageTokenSnapshotIdentityBoundaries(t *testing.T) {
	storeErr := errors.New("snapshot store failure")
	for _, tc := range []struct {
		name     string
		userID   string
		data     *TokenData
		storeErr error
		wantErr  error
		wantData bool
	}{
		{name: "exact identity", userID: "user", data: &TokenData{CorpID: "corp", UserID: "user"}, wantData: true},
		{name: "wrong identity", userID: "user", data: &TokenData{CorpID: "corp", UserID: "other"}},
		{name: "wrong identity organization", userID: "user", data: &TokenData{CorpID: "other", UserID: "user"}},
		{name: "nil identity", userID: "user", wantErr: ErrTokenDataNotFound},
		{name: "identity read failure", userID: "user", storeErr: storeErr, wantErr: storeErr},
		{name: "unresolved identity", data: &TokenData{CorpID: "corp"}, wantData: true},
		{name: "unresolved identity requires repair", data: &TokenData{CorpID: "corp", UserID: "user"}, wantErr: ErrTokenMigrationRequired},
		{name: "wrong unresolved organization", data: &TokenData{CorpID: "other"}},
		{name: "nil organization", wantErr: ErrTokenDataNotFound},
		{name: "organization read failure", storeErr: storeErr, wantErr: storeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			profile := Profile{CorpID: "corp", UserID: tc.userID}
			if err := SaveProfiles(dir, &ProfilesConfig{Version: profilesVersion, CurrentProfile: ProfileSelector(profile), Profiles: []Profile{profile}}); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
				t.Fatal("snapshot acquired auth lock")
				return nil, nil
			})
			testseam.Swap(t, &tokenSaveKeychainForIdentity, func(string, string, *TokenData) error { t.Fatal("snapshot repaired an identity slot"); return nil })
			testseam.Swap(t, &tokenLoadKeychainIdentity, func(string, string) (*TokenData, error) { return tc.data, tc.storeErr })
			testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) {
				if tc.userID != "" {
					return nil, ErrTokenDataNotFound
				}
				return tc.data, tc.storeErr
			})
			testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return nil, ErrTokenDataNotFound })
			got, err := ReadTokenDataForProfile(dir, ProfileSelector(profile))
			if tc.wantData {
				if err != nil || got != tc.data {
					t.Fatalf("snapshot=%#v, %v", got, err)
				}
				return
			}
			if err == nil || got != nil || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
				t.Fatalf("snapshot=%#v, error=%v, want=%v", got, err, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageDirectTokenReadHistoricalIdentityMigration(t *testing.T) {
	for _, version := range []int{profilesVersion, profilesUnresolvedSelectorVersion} {
		for _, api := range []string{"loader", "runtime", "oauth", "status"} {
			for _, byName := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%d/%s/userName=%v", version, api, byName), func(t *testing.T) {
					dir, data := setupUnresolvedProfileSelection(t, version)
					account := data.UserID
					if byName {
						account = data.UserName
					}
					selector := data.CorpID + ":" + account
					read := func() (*TokenData, error) {
						switch api {
						case "runtime":
							SetRuntimeProfile(selector)
							return LoadTokenData(dir)
						case "oauth":
							return NewOAuthProvider(dir, nil).GetTokenSnapshotForProfile(context.Background(), selector)
						case "status":
							SetRuntimeProfile(selector)
							return NewOAuthProvider(dir, nil).Status()
						default:
							return LoadTokenDataForProfile(dir, selector)
						}
					}
					acquire := profilesAcquireDualLock
					locks := 0
					testseam.Swap(t, &profilesAcquireDualLock, func(ctx context.Context, dir string) (*DualLock, error) {
						locks++
						return acquire(ctx, dir)
					})
					got, err := read()
					if err != nil || got == nil || got.AccessToken != data.AccessToken || got.UserID != data.UserID || locks != 1 {
						t.Fatalf("historical %s read = %#v, %v, locks=%d", api, got, err, locks)
					}
					got, err = read()
					if err != nil || got == nil || got.AccessToken != data.AccessToken || locks != 2 {
						t.Fatalf("migrated %s read = %#v, %v, locks=%d; want compatibility read lock", api, got, err, locks)
					}
				})
			}
		}
	}
}

func TestCrossPlatformCoverageDirectTokenIdentityMigrationRechecksUnderLock(t *testing.T) {
	for _, action := range []string{"logout", "completed migration", "lock failure"} {
		t.Run(action, func(t *testing.T) {
			dir, data := setupUnresolvedProfileSelection(t, profilesVersion)
			acquire := profilesAcquireDualLock
			lockFailure := errors.New("direct-read fixture lock failure")
			locks := 0
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
					fresh.AccessToken = "fresh-direct-read-fixture"
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
			got, err := LoadTokenDataForProfile(dir, data.CorpID+":"+data.UserID)
			if locks != 1 {
				t.Fatalf("direct migration locks=%d, want 1", locks)
			}
			if action == "completed migration" {
				if err != nil || got == nil || got.AccessToken != "fresh-direct-read-fixture" {
					t.Fatalf("direct read did not respect completed migration: %#v, %v", got, err)
				}
				return
			}
			if err == nil || got != nil {
				t.Fatalf("direct %s read = %#v, %v", action, got, err)
			}
			if action == "lock failure" && !errors.Is(err, lockFailure) {
				t.Fatalf("lock failure was discarded: %v", err)
			}
			if _, err := LoadTokenDataKeychainForIdentity(data.CorpID, data.UserID); !errors.Is(err, ErrTokenDataNotFound) {
				t.Fatalf("failed direct migration resurrected identity: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageManualAndLoggedOutTokenReadsDoNotAcquireLock(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	if err := SaveTokenData(configDir, &TokenData{AccessToken: "manual-read"}); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
		return nil, errors.New("read attempted to acquire the auth lock")
	})
	loaded, err := ReadTokenDataForProfile(configDir, "")
	if err != nil || loaded == nil || loaded.AccessToken != "manual-read" {
		t.Fatalf("manual token read = %#v, %v", loaded, err)
	}
	if err := SaveProfiles(configDir, &ProfilesConfig{Version: profilesVersion}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteTokenMarker(configDir); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTokenDataForProfile(configDir, ""); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("logged-out tombstone read error = %v", err)
	}
}

func TestCrossPlatformCoverageCorruptTokenReadDoesNotQuarantineProfiles(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	if err := os.WriteFile(ProfilesPath(configDir), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
		t.Fatal("corrupt read acquired auth lock")
		return nil, nil
	})
	testseam.Swap(t, &profilesRename, func(string, string) error {
		t.Fatal("read quarantined profiles.json")
		return nil
	})
	if loaded, err := ReadTokenDataForProfile(configDir, ""); err == nil || loaded != nil {
		t.Fatalf("corrupt profiles read = %#v, %v", loaded, err)
	}
}

func TestCrossPlatformCoverageMissingTokenStorageReadsDoNotAcquireLock(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
		t.Fatal("empty storage read acquired the auth lock")
		return nil, nil
	})
	if _, err := ReadTokenDataForProfile(configDir, ""); !errors.Is(err, ErrTokenDataNotFound) {
		t.Fatalf("missing storage Status() error = %v", err)
	}
	if p, err := ResolveProfileMetadataReadOnly(configDir, ""); err != nil || p != nil {
		t.Fatalf("missing current profile = %#v, %v", p, err)
	}
	if p, err := ReadTokenDataForProfile(configDir, "unknown"); !errors.Is(err, ErrTokenDataNotFound) || p != nil {
		t.Fatalf("missing explicit token = %#v, %v", p, err)
	}
}

func TestCrossPlatformCoverageTokenMigrationAcquiresLockAndRechecks(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("legacy-read", "corp-migrate-read", "Migration Org")
	if err := SaveTokenDataKeychain(data); err != nil {
		t.Fatal(err)
	}
	acquire := profilesAcquireDualLock
	locks := 0
	testseam.Swap(t, &profilesAcquireDualLock, func(ctx context.Context, dir string) (*DualLock, error) {
		locks++
		return acquire(ctx, dir)
	})
	loaded, err := LoadTokenData(configDir)
	if err != nil || loaded == nil || loaded.AccessToken != data.AccessToken || locks != 1 {
		t.Fatalf("legacy read = %#v, %v, locks=%d", loaded, err, locks)
	}
	if _, err := LoadTokenData(configDir); err != nil || locks != 2 {
		t.Fatalf("migrated read error=%v, locks=%d; want compatibility read lock", err, locks)
	}

	// A migration candidate observed before locking must not resurrect a login
	// removed by another process while the reader waited for the lock.
	if err := DeleteTokenDataKeychainForIdentity(data.CorpID, data.UserID); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &profilesAcquireDualLock, func(ctx context.Context, dir string) (*DualLock, error) {
		lock, err := acquire(ctx, dir)
		if err != nil {
			return nil, err
		}
		if err := SaveProfiles(dir, &ProfilesConfig{Version: profilesVersion}); err != nil {
			lock.Release()
			return nil, err
		}
		return lock, nil
	})
	if loaded, err := LoadTokenData(configDir); !errors.Is(err, ErrTokenDataNotFound) || loaded != nil {
		t.Fatalf("concurrent logout migration read = %#v, %v", loaded, err)
	}
}

func TestCrossPlatformCoverageTokenReadErrorsDoNotAcquireLock(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("read-error-snapshot", "corp-read-errors", "Read Error Org")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		load func(string, string) (*TokenData, error)
	}{
		{"wrong organization", func(string, string) (*TokenData, error) {
			return &TokenData{AccessToken: "wrong-corp", CorpID: "other-corp", UserID: data.UserID}, nil
		}},
		{"wrong account", func(string, string) (*TokenData, error) {
			return &TokenData{AccessToken: "wrong-user", CorpID: data.CorpID, UserID: "other-user"}, nil
		}},
		{"unavailable keychain", func(string, string) (*TokenData, error) {
			return nil, keychain.NewUnavailableError("read token", errors.New("unavailable"))
		}},
		{"missing credential", func(string, string) (*TokenData, error) {
			return nil, ErrTokenDataNotFound
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
				t.Fatal("read error attempted migration without a migration candidate")
				return nil, nil
			})
			testseam.Swap(t, &tokenLoadKeychainIdentity, tc.load)
			testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
			testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return nil, ErrTokenDataNotFound })
			if loaded, err := ReadTokenDataForProfile(configDir, ""); err == nil || loaded != nil {
				t.Fatalf("invalid credential read = %#v, %v", loaded, err)
			}
		})
	}
}

func TestCrossPlatformCoverageTokenReadRefreshStillAcquiresLock(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("expired-read-snapshot", "corp-refresh-read", "Refresh Org")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatal(err)
	}
	acquire := oauthAcquireLock
	locks := 0
	testseam.Swap(t, &oauthAcquireLock, func(ctx context.Context, dir string) (*DualLock, error) {
		locks++
		return acquire(ctx, dir)
	})
	refreshed := false
	testseam.Swap(t, &oauthRefreshToken, func(p *OAuthProvider, _ context.Context, loaded *TokenData) (*TokenData, error) {
		// A second process must not be able to begin another refresh while the
		// remote exchange and credential persistence are in progress.
		if _, ok := processLocks.Load(processLockKey(configDir)); !ok {
			t.Fatal("refresh did not retain the auth lock")
		}
		updated := *loaded
		updated.AccessToken = "refreshed-read-snapshot"
		updated.ExpiresAt = time.Now().Add(time.Hour)
		if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
			return nil, err
		}
		refreshed = true
		return &updated, nil
	})
	snapshot, err := NewOAuthProvider(configDir, nil).GetTokenSnapshot(context.Background())
	if err != nil || snapshot == nil || snapshot.AccessToken != "refreshed-read-snapshot" || !refreshed || locks != 1 {
		t.Fatalf("refresh read = %#v, %v, refreshed=%v locks=%d", snapshot, err, refreshed, locks)
	}
}

func TestCrossPlatformCoverageConcurrentTokenReadRefreshPublishesOnce(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	configDir := t.TempDir()
	data := testToken("concurrent-expired-read", "corp-concurrent-read", "Concurrent Org")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatal(err)
	}
	selector := profileSelector(data.CorpID, data.UserID)
	// Observe both expired snapshots without blocking on the public read lock,
	// then verify the refresh double-check still publishes only once.
	testseam.Swap(t, &oauthLoadTokenForProfile, ReadTokenDataForProfile)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered, secondWaiting, publish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseRefresh := func() { releaseOnce.Do(func() { close(publish) }) }
	var callers sync.WaitGroup
	// Drain all callers before test seams and isolated credential state restore.
	defer func() { releaseRefresh(); callers.Wait() }()
	var lockCalls, refreshCalls atomic.Int32
	acquire := oauthAcquireLock
	testseam.Swap(t, &oauthAcquireLock, func(ctx context.Context, dir string) (*DualLock, error) {
		if lockCalls.Add(1) == 2 {
			close(secondWaiting)
		}
		return acquire(ctx, dir)
	})
	testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
		return nil, errors.New("initial token read acquired a migration lock")
	})
	testseam.Swap(t, &oauthRefreshToken, func(p *OAuthProvider, ctx context.Context, loaded *TokenData) (*TokenData, error) {
		if refreshCalls.Add(1) != 1 {
			return nil, errors.New("refresh token was consumed more than once")
		}
		close(entered)
		select {
		case <-publish:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		updated := *loaded
		updated.AccessToken = "concurrent-fresh-read"
		updated.RefreshToken = "concurrent-rotated-refresh"
		updated.ExpiresAt = time.Now().Add(time.Hour)
		if err := saveTokenDataLocked(p.configDir, &updated); err != nil {
			return nil, err
		}
		return &updated, nil
	})
	type result struct {
		data *TokenData
		err  error
	}
	results := make(chan result, 2)
	start := func() {
		callers.Add(1)
		go func() {
			defer callers.Done()
			data, err := NewOAuthProvider(configDir, nil).GetTokenSnapshotForProfile(ctx, selector)
			results <- result{data, err}
		}()
	}
	start()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first caller did not enter refresh")
	}
	start()
	select {
	case <-secondWaiting:
	case <-ctx.Done():
		t.Fatal("second caller did not reach the refresh lock")
	}
	// A snapshot-only reader may still observe the old token while refresh is
	// in flight; it must not join the refresh queue or mutate the credential.
	snapshot, err := ReadTokenDataForProfile(configDir, selector)
	if err != nil || snapshot == nil || snapshot.AccessToken != data.AccessToken || snapshot.RefreshToken != data.RefreshToken {
		t.Fatalf("read during refresh = %#v, %v", snapshot, err)
	}
	releaseRefresh()
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil || got.data == nil || got.data.AccessToken != "concurrent-fresh-read" || got.data.RefreshToken != "concurrent-rotated-refresh" {
				t.Fatalf("concurrent refresh result = %#v, %v", got.data, got.err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent refresh did not finish")
		}
	}
	if lockCalls.Load() != 2 || refreshCalls.Load() != 1 {
		t.Fatalf("lock calls=%d refresh calls=%d; want 2 locks and one refresh", lockCalls.Load(), refreshCalls.Load())
	}
	if _, held := processLocks.Load(processLockKey(configDir)); held {
		t.Fatal("refresh retained the auth lock after returning")
	}
}

func TestCrossPlatformCoverageMissingTokenSlotMigrationCandidates(t *testing.T) {
	const corpID, userID = "corp-candidate", "user-candidate"
	matching := &TokenData{AccessToken: "matching-candidate", CorpID: corpID, UserID: userID}
	blank := &TokenData{AccessToken: "blank-candidate", CorpID: corpID}
	otherUser := &TokenData{AccessToken: "other-user-candidate", CorpID: corpID, UserID: "other-user"}
	otherCorp := &TokenData{AccessToken: "other-corp-candidate", CorpID: "other-corp", UserID: userID}
	readErr := errors.New("compatibility token read failure")
	for _, tc := range []struct {
		name              string
		org, global       *TokenData
		orgErr, globalErr error
		multi             bool
		explicit          bool
		wantRepair        bool
		wantData          bool
	}{
		{name: "exact organization candidate", org: matching, wantRepair: true},
		{name: "sole identity enrichment", org: blank, wantRepair: true},
		{name: "wrong organization", org: otherCorp},
		{name: "global candidate for sole profile", global: matching, explicit: true, wantRepair: true},
		{name: "global enrichment for sole profile", global: blank, wantRepair: true},
		{name: "wrong global identity", global: otherUser},
		{name: "wrong global organization", global: otherCorp},
		{name: "multi-account explicit rejects global repair", global: matching, multi: true, explicit: true},
		{name: "multi-account default same-identity compatibility", global: matching, multi: true, wantData: true},
		{name: "other organization identity default compatibility", org: otherUser, global: matching, wantData: true},
		{name: "other organization identity explicit rejects fallback", org: otherUser, global: matching, explicit: true},
		{name: "ambiguous identity enrichment", org: blank, multi: true},
		{name: "organization read error", orgErr: readErr},
		{name: "other organization identity rejects wrong global identity", org: otherUser, global: otherUser},
		{name: "other organization identity rejects missing global token", org: otherUser},
		{name: "other organization identity preserves global read error", org: otherUser, globalErr: readErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			cfg := &ProfilesConfig{Version: profilesVersion, CurrentProfile: profileSelector(corpID, userID), Profiles: []Profile{{CorpID: corpID, UserID: userID}}}
			if tc.multi {
				cfg.Profiles = append(cfg.Profiles, Profile{CorpID: corpID, UserID: "other-user"})
			}
			if err := SaveProfiles(configDir, cfg); err != nil {
				t.Fatal(err)
			}
			testseam.Swap(t, &tokenLoadKeychainIdentity, func(string, string) (*TokenData, error) { return nil, ErrTokenDataNotFound })
			testseam.Swap(t, &tokenLoadKeychainForCorpID, func(string) (*TokenData, error) {
				if tc.orgErr != nil {
					return nil, tc.orgErr
				}
				if tc.org == nil {
					return nil, ErrTokenDataNotFound
				}
				return tc.org, nil
			})
			testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) {
				if tc.globalErr != nil {
					return nil, tc.globalErr
				}
				if tc.global == nil {
					return nil, ErrTokenDataNotFound
				}
				return tc.global, nil
			})
			locks := 0
			testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
				locks++
				return nil, errors.New("unexpected snapshot auth lock acquisition")
			})
			selector := ""
			if tc.explicit {
				selector = profileSelector(corpID, userID)
			}
			loaded, err := ReadTokenDataForProfile(configDir, selector)
			if (tc.orgErr != nil || tc.globalErr != nil) && !errors.Is(err, readErr) {
				t.Fatalf("compatibility read lost original error: %v", err)
			}
			if locks != 0 || tc.wantRepair != errors.Is(err, ErrTokenMigrationRequired) {
				t.Fatalf("locks=%d, wantRepair=%v; error=%v", locks, tc.wantRepair, err)
			}
			if tc.wantData {
				if err != nil || loaded != matching {
					t.Fatalf("compatibility read = %#v, %v", loaded, err)
				}
			} else if err == nil || loaded != nil {
				t.Fatalf("candidate read = %#v, %v", loaded, err)
			}
		})
	}
}

func TestCrossPlatformCoverageLegacyTokenReadProbeEdges(t *testing.T) {
	probeErr := errors.New("read probe failure")
	for _, tc := range []struct {
		name       string
		manual     bool
		markerErr  error
		global     *TokenData
		globalErr  error
		statErr    error
		wantErr    error
		wantRepair bool
	}{
		{name: "marker error", markerErr: probeErr, wantErr: probeErr},
		{name: "manual credential error", manual: true, globalErr: probeErr, wantErr: probeErr},
		{name: "legacy credential error", globalErr: probeErr, wantErr: probeErr},
		{name: "unbound legacy token", global: &TokenData{AccessToken: "unbound-read"}},
		{name: "legacy secure file candidate", globalErr: ErrTokenDataNotFound, wantRepair: true},
		{name: "legacy secure file stat error", globalErr: ErrTokenDataNotFound, statErr: probeErr, wantErr: probeErr},
		{name: "stale manual marker", manual: true, globalErr: ErrTokenDataNotFound, statErr: os.ErrNotExist, wantErr: ErrTokenDataNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			testseam.Swap(t, &tokenReadFile, func(string) ([]byte, error) {
				if tc.markerErr != nil {
					return nil, tc.markerErr
				}
				if tc.manual {
					return []byte(`{"manual_token":true}`), nil
				}
				return nil, os.ErrNotExist
			})
			testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return tc.global, tc.globalErr })
			testseam.Swap(t, &secureStat, func(string) (os.FileInfo, error) { return nil, tc.statErr })
			locks := 0
			testseam.Swap(t, &profilesAcquireDualLock, func(context.Context, string) (*DualLock, error) {
				locks++
				return nil, ErrTokenMigrationRequired
			})
			loaded, err := ReadTokenDataForProfile(configDir, "")
			if tc.wantRepair {
				if locks != 0 || !errors.Is(err, ErrTokenMigrationRequired) {
					t.Fatalf("migration locks=%d, error=%v", locks, err)
				}
				return
			}
			if locks != 0 || !errors.Is(err, tc.wantErr) || loaded != tc.global {
				t.Fatalf("read locks=%d, data=%#v, error=%v", locks, loaded, err)
			}
		})
	}
}

func BenchmarkTokenReadSnapshot(b *testing.B) {
	for _, count := range []int{1, 100} {
		b.Run(fmt.Sprintf("profiles=%d", count), func(b *testing.B) {
			b.Setenv(keychain.DisableKeychainEnv, "1")
			b.Setenv(keychain.StorageDirEnv, b.TempDir())
			configDir := b.TempDir()
			data := &TokenData{AccessToken: "benchmark-token", CorpID: "bench-corp", UserID: "bench-user"}
			if err := SaveTokenDataKeychainForIdentity(data.CorpID, data.UserID, data); err != nil {
				b.Fatal(err)
			}
			cfg := &ProfilesConfig{Version: profilesVersion, CurrentProfile: "bench-corp:bench-user", Profiles: []Profile{{CorpID: data.CorpID, UserID: data.UserID}}}
			for i := 1; i < count; i++ {
				cfg.Profiles = append(cfg.Profiles, Profile{CorpID: fmt.Sprintf("bench-corp-%d", i), UserID: "bench-user"})
			}
			if err := SaveProfiles(configDir, cfg); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := ReadTokenDataForProfile(configDir, "bench-corp:bench-user"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageReadTokenOpaqueEditionBackend(t *testing.T) {
	previous := edition.Get()
	hooks := *previous
	hooks.LoadToken = func(string) ([]byte, error) { return nil, nil }
	edition.Override(&hooks)
	t.Cleanup(func() { edition.Override(previous) })
	if _, err := ReadTokenDataForProfile(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "read-only inspection is not supported") {
		t.Fatalf("opaque edition backend error = %v", err)
	}
}

func TestCrossPlatformCoverageReadTokenLegacyMirrorRequiresMigration(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	dir := t.TempDir()
	legacy := testToken("legacy-snapshot", "legacy-corp", "Legacy Org")
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) { return legacy, nil })
	if _, err := ReadTokenDataForProfile(dir, ""); !errors.Is(err, ErrTokenMigrationRequired) {
		t.Fatalf("legacy mirror error = %v, want ErrTokenMigrationRequired", err)
	}
}

func TestCrossPlatformCoverageReadTokenCanonicalMigrationModernRegistry(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	dir := t.TempDir()
	cfg := &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: unresolvedProfileSelector("canon-corp"),
		Profiles:       []Profile{{Name: "historical", CorpID: "canon-corp"}},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProfilesPath(dir), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTokenDataForProfile(dir, ""); !errors.Is(err, ErrTokenMigrationRequired) {
		t.Fatalf("canonical migration error = %v, want ErrTokenMigrationRequired", err)
	}
}

func TestCrossPlatformCoverageReadTokenCompoundSelectorNeedsIdentityMigration(t *testing.T) {
	dir, data := setupUnresolvedProfileSelection(t, profilesVersion)
	if _, err := ReadTokenDataForProfile(dir, data.CorpID+":"+data.UserID); !errors.Is(err, ErrTokenMigrationRequired) {
		t.Fatalf("compound selector probe error = %v, want ErrTokenMigrationRequired", err)
	}
}

func TestCrossPlatformCoverageReadTokenUnboundLegacyMirror(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	dir := t.TempDir()
	testseam.Swap(t, &tokenLoadKeychain, func() (*TokenData, error) {
		return &TokenData{AccessToken: "unbound-legacy"}, nil
	})
	data, err := ReadTokenDataForProfile(dir, "")
	if err != nil || data == nil || data.AccessToken != "unbound-legacy" {
		t.Fatalf("unbound legacy mirror read = %#v, %v", data, err)
	}
}

func TestCrossPlatformCoverageReadTokenLegacyRegistryWithProfilesNeedsMigration(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	cleanupKeychain(t)
	dir := t.TempDir()
	cfg := &ProfilesConfig{
		Version:  profilesVersion - 1,
		Profiles: []Profile{{Name: "historical", CorpID: "legacy-corp"}},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProfilesPath(dir), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTokenDataForProfile(dir, ""); !errors.Is(err, ErrTokenMigrationRequired) {
		t.Fatalf("legacy registry with profiles error = %v, want ErrTokenMigrationRequired", err)
	}
}

func TestCrossPlatformCoverageProfileMetadataNeedsMigrationVersionGate(t *testing.T) {
	if !profileMetadataNeedsMigration(nil) {
		t.Fatal("nil registry must require migration")
	}
	if !profileMetadataNeedsMigration(&ProfilesConfig{Version: profilesVersion - 1}) {
		t.Fatal("registry below profilesVersion must require migration")
	}
}

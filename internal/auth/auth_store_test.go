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
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func withAuthStoreEdition(t *testing.T, hooks *edition.Hooks) {
	t.Helper()
	previous := edition.Get()
	edition.Override(hooks)
	t.Cleanup(func() { edition.Override(previous) })
}

func TestLegacyAndCustomConfigDirRemainZeroWriteUntilExplicitOptIn(t *testing.T) {
	for _, name := range []string{"default-like", "custom-dws-config-dir"} {
		t.Run(name, func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "not-created", name)
			DeactivateAuthStore(base)
			t.Cleanup(func() { DeactivateAuthStore(base) })

			store := ResolveAuthStore(base)
			if store.ConfigDir != base || store.KeychainService != keychain.Service || store.InstanceID != "" {
				t.Fatalf("ResolveAuthStore() = %#v, want exact legacy coordinate", store)
			}
			activated, err := ActivateCurrentInstance(base)
			if err != nil {
				t.Fatalf("ActivateCurrentInstance() error = %v", err)
			}
			if activated != store {
				t.Fatalf("ActivateCurrentInstance() = %#v, want %#v", activated, store)
			}
			instances, currentID, err := ListAuthInstances(base)
			if err != nil {
				t.Fatalf("ListAuthInstances() error = %v", err)
			}
			if len(instances) != 1 || instances[0].Kind != AuthInstanceKindLegacy || currentID != "" {
				t.Fatalf("ListAuthInstances() = %#v, %q", instances, currentID)
			}
			if _, err := os.Stat(base); !os.IsNotExist(err) {
				t.Fatalf("ordinary auth lookup wrote config home: stat error = %v", err)
			}
		})
	}
}

func TestBeginNewInstanceRegistersLegacyInPlaceWithoutCopy(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })
	cleanupAuthServiceForTest(t, keychain.Service)

	legacyProfiles := []byte(`{"version":1,"currentProfile":"legacy-corp","profiles":[{"name":"legacy","corpId":"legacy-corp","userId":"legacy-user"}]}`)
	if err := os.WriteFile(filepath.Join(base, profilesJSONFile), legacyProfiles, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, tokenJSONFile), []byte(`{"updated_at":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyToken := testAccountToken("legacy-corp", "legacy-user", "legacy-access")
	if err := SaveTokenDataKeychainAt(base, legacyToken); err != nil {
		t.Fatalf("save legacy keychain token: %v", err)
	}
	t.Cleanup(func() { _ = keychain.Remove(keychain.Service, keychain.AccountToken) })

	tx, err := BeginNewInstance(base, "personal")
	if err != nil {
		t.Fatalf("BeginNewInstance() error = %v", err)
	}
	store := tx.Store()
	cleanupAuthServiceForTest(t, store.KeychainService)
	if !store.Isolated() || store.ConfigDir == base || store.KeychainService == keychain.Service {
		t.Fatalf("pending Store() = %#v, want isolated coordinate", store)
	}
	if store.InstanceID != tx.Instance().ID ||
		store.ConfigDir != filepath.Join(base, authRegistryDirName, authInstancesDirName, tx.Instance().ID) ||
		store.KeychainService != authInstanceServicePrefix+tx.Instance().ID {
		t.Fatalf("pending Store() = %#v, want one direct coordinate for instance %q", store, tx.Instance().ID)
	}
	for _, name := range []string{profilesJSONFile, tokenJSONFile, secureDataFile} {
		if _, err := os.Stat(filepath.Join(store.ConfigDir, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy file %s was copied before commit: %v", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	for _, name := range []string{profilesJSONFile, tokenJSONFile, secureDataFile} {
		if _, err := os.Stat(filepath.Join(store.ConfigDir, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy file %s was copied at commit: %v", name, err)
		}
	}
	if got, err := LoadTokenDataKeychainAt(base); !errors.Is(err, ErrTokenDataNotFound) || got != nil {
		t.Fatalf("isolated account unexpectedly copied legacy token: %#v, %v", got, err)
	}
	if got, err := keychain.Get(keychain.Service, keychain.AccountToken); err != nil || got == "" {
		t.Fatalf("legacy keychain token changed: value=%q err=%v", got, err)
	}
	instances, currentID, err := ListAuthInstances(base)
	if err != nil || len(instances) != 2 || currentID != tx.Instance().ID {
		t.Fatalf("registry = %#v, current=%q, err=%v", instances, currentID, err)
	}
	if instances[0].Kind != AuthInstanceKindLegacy || instances[0].Alias != DefaultAuthInstanceAlias {
		t.Fatalf("first instance = %#v, want legacy-backed default", instances[0])
	}
}

func TestIsolatedInstancesKeepProfilesAndTokensSeparateAndUsePrevious(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })
	cleanupAuthServiceForTest(t, keychain.Service)

	if err := SaveTokenData(base, testAccountToken("shared-corp", "legacy-user", "legacy-access")); err != nil {
		t.Fatalf("SaveTokenData(legacy) error = %v", err)
	}
	txA, err := BeginNewInstance(base, "work")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenData(base, testAccountToken("shared-corp", "user-a", "access-a")); err != nil {
		t.Fatalf("SaveTokenData(account A) error = %v", err)
	}
	if err := txA.Commit(); err != nil {
		t.Fatal(err)
	}
	cleanupAuthServiceForTest(t, txA.Store().KeychainService)

	txB, err := BeginNewInstance(base, "personal")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenData(base, testAccountToken("shared-corp", "user-b", "access-b")); err != nil {
		t.Fatalf("SaveTokenData(account B) error = %v", err)
	}
	if err := txB.Commit(); err != nil {
		t.Fatal(err)
	}
	cleanupAuthServiceForTest(t, txB.Store().KeychainService)

	if txA.Store().InstanceID == txB.Store().InstanceID {
		t.Fatalf("auth instances share generated ID %q", txA.Store().InstanceID)
	}
	if txA.Store().KeychainService == txB.Store().KeychainService {
		t.Fatalf("accounts share keychain service %q", txA.Store().KeychainService)
	}
	assertActiveToken(t, base, "access-b", "shared-corp")
	selected, err := UseAuthInstance(base, "-")
	if err != nil || selected.ID != txA.Instance().ID {
		t.Fatalf("UseAuthInstance(-) = %#v, %v; want account A", selected, err)
	}
	assertActiveToken(t, base, "access-a", "shared-corp")
	if profile, err := ResolveProfileByUserID(base, "user-a"); err != nil || profile.CorpID != "shared-corp" {
		t.Fatalf("ResolveProfileByUserID(user-a) = %#v, %v", profile, err)
	}

	selected, err = UseAuthInstance(base, "-")
	if err != nil || selected.ID != txB.Instance().ID {
		t.Fatalf("second UseAuthInstance(-) = %#v, %v; want account B", selected, err)
	}
	assertActiveToken(t, base, "access-b", "shared-corp")
	if _, err := ResolveProfileByUserID(base, "user-a"); err == nil {
		t.Fatal("account B resolved account A's userId")
	}

	selected, err = UseAuthInstance(base, DefaultAuthInstanceAlias)
	if err != nil || selected.Kind != AuthInstanceKindLegacy {
		t.Fatalf("UseAuthInstance(default) = %#v, %v", selected, err)
	}
	assertActiveToken(t, base, "legacy-access", "shared-corp")

	instances, _, err := ListAuthInstances(base)
	if err != nil || len(instances) != 3 {
		t.Fatalf("ListAuthInstances() = %#v, %v; want 3 instances", instances, err)
	}
}

func TestAuthInstanceIsLoginSlotAndAllowsReplacingIdentity(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })

	tx, err := BeginNewInstance(base, "reusable-slot")
	if err != nil {
		t.Fatal(err)
	}
	cleanupAuthServiceForTest(t, tx.Store().KeychainService)
	if err := SaveTokenData(base, testAccountToken("same-corp", "user-a", "access-a")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// An auth instance is an isolated login-state location, not a permanent
	// person identity. Re-login there keeps the online overwrite semantics.
	if err := SaveTokenData(base, testAccountToken("same-corp", "user-b", "access-b")); err != nil {
		t.Fatalf("SaveTokenData(replacement identity) error = %v", err)
	}
	loaded, err := LoadTokenDataForProfile(base, "same-corp")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UserID != "user-b" || loaded.AccessToken != "access-b" {
		t.Fatalf("login slot did not adopt replacement identity: %#v", loaded)
	}
}

func TestRollbackDoesNotPublishFailedLogin(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })
	cleanupAuthServiceForTest(t, keychain.Service)

	legacy := testAccountToken("legacy-corp", "legacy-user", "legacy-access")
	if err := SaveTokenData(base, legacy); err != nil {
		t.Fatalf("save legacy token: %v", err)
	}
	legacyProfiles, err := os.ReadFile(ProfilesPath(base))
	if err != nil {
		t.Fatal(err)
	}
	legacyKeychain, err := keychain.Get(keychain.Service, keychain.AccountToken)
	if err != nil || legacyKeychain == "" {
		t.Fatalf("read legacy keychain: value=%q err=%v", legacyKeychain, err)
	}

	tx, err := BeginNewInstance(base, "failed")
	if err != nil {
		t.Fatal(err)
	}
	store := tx.Store()
	if err := SaveTokenData(base, testAccountToken("corp-failed", "user-failed", "access-failed")); err != nil {
		t.Fatal(err)
	}
	// Model a partial failure after a corp-scoped Keychain write but before
	// profiles.json records that corp. Rollback must still remove this slot,
	// including on the Windows registry backend where removing files is not
	// sufficient.
	orphanCorpID := "corp-unindexed"
	if err := SaveTokenDataKeychainForCorpIDAt(base, orphanCorpID, testAccountToken(orphanCorpID, "user-unindexed", "access-unindexed")); err != nil {
		t.Fatal(err)
	}
	if !keychain.Exists(store.KeychainService, TokenAccountForCorpID(orphanCorpID)) {
		t.Fatal("unindexed pending token was not written")
	}
	if _, err := os.Stat(AuthRegistryPath(base)); !os.IsNotExist(err) {
		t.Fatalf("pending login published registry: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := os.Stat(AuthRegistryPath(base)); !os.IsNotExist(err) {
		t.Fatalf("rollback published registry: %v", err)
	}
	if keychain.Exists(store.KeychainService, TokenAccountForCorpID(orphanCorpID)) {
		t.Fatal("rollback left an unindexed pending token behind")
	}
	if _, err := os.Stat(store.ConfigDir); !os.IsNotExist(err) {
		t.Fatalf("rollback left account data dir: %v", err)
	}
	resolved := ResolveAuthStore(base)
	if resolved.ConfigDir != base || resolved.KeychainService != keychain.Service || resolved.InstanceID != "" {
		t.Fatalf("store after rollback = %#v, want legacy", resolved)
	}
	if got, _ := keychain.Get(store.KeychainService, keychain.AccountToken); got != "" {
		t.Fatalf("rollback left isolated keychain token: %q", got)
	}
	if got, err := os.ReadFile(ProfilesPath(base)); err != nil || string(got) != string(legacyProfiles) {
		t.Fatalf("rollback changed legacy profiles: err=%v\ngot=%q\nwant=%q", err, got, legacyProfiles)
	}
	if got, err := keychain.Get(keychain.Service, keychain.AccountToken); err != nil || got != legacyKeychain {
		t.Fatalf("rollback changed legacy keychain: value=%q err=%v", got, err)
	}
}

func TestConcurrentNewInstanceCommitDetectsRegistryConflict(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })

	txA, err := BeginNewInstance(base, "account-a")
	if err != nil {
		t.Fatal(err)
	}
	txB, err := BeginNewInstance(base, "account-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := txA.Commit(); err != nil {
		t.Fatalf("first Commit() error = %v", err)
	}
	if err := txB.Commit(); !errors.Is(err, ErrAuthRegistryConflict) {
		t.Fatalf("second Commit() error = %v, want ErrAuthRegistryConflict", err)
	}
	if err := txB.Rollback(); err != nil {
		t.Fatalf("second Rollback() error = %v", err)
	}

	instances, currentID, err := ListAuthInstances(base)
	if err != nil {
		t.Fatal(err)
	}
	registry := authRegistry{Instances: instances}
	if len(instances) != 2 || currentID != txA.Instance().ID || findAuthInstanceByID(&registry, txB.Instance().ID) != nil {
		t.Fatalf("registry after conflict = %#v current=%q, want only default + instance-a", instances, currentID)
	}
}

func TestRegistryCorruptionFailsClosedAndHooksRejectOptIn(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })
	if err := os.MkdirAll(filepath.Dir(AuthRegistryPath(base)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(AuthRegistryPath(base), []byte(`{"version":1,"broken":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateCurrentInstance(base); err == nil {
		t.Fatal("ActivateCurrentInstance() accepted corrupt registry")
	}
	if got := ResolveAuthStore(base); got.ConfigDir != base || got.KeychainService != keychain.Service {
		t.Fatalf("corrupt registry activated a credential store: %#v", got)
	}

	for _, tc := range []struct {
		name  string
		hooks *edition.Hooks
	}{
		{name: "save", hooks: &edition.Hooks{Name: "hooked-save", SaveToken: func(string, []byte) error { return nil }}},
		{name: "load", hooks: &edition.Hooks{Name: "hooked-load", LoadToken: func(string) ([]byte, error) { return nil, nil }}},
		{name: "delete", hooks: &edition.Hooks{Name: "hooked-delete", DeleteToken: func(string) error { return nil }}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withAuthStoreEdition(t, tc.hooks)
			hookBase := filepath.Join(t.TempDir(), "not-created")
			if _, err := BeginNewInstance(hookBase, "blocked"); !errors.Is(err, ErrAuthInstanceIsolationUnsupported) {
				t.Fatalf("BeginNewInstance() error = %v, want unsupported", err)
			}
			if _, err := os.Stat(hookBase); !os.IsNotExist(err) {
				t.Fatalf("unsupported BeginNewInstance wrote base: %v", err)
			}
		})
	}
}

func TestRegistryAndAccountDirectoriesUsePrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })
	tx, err := BeginNewInstance(base, "private")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(AuthRegistryPath(base)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("registry permission = %v, err=%v; want 0600", permissionOf(info), err)
	}
	for path := tx.Store().ConfigDir; strings.HasPrefix(path, filepath.Join(base, authRegistryDirName)); path = filepath.Dir(path) {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("directory %s permission = %v, err=%v; want 0700", path, permissionOf(info), err)
		}
		if path == filepath.Join(base, authRegistryDirName) {
			break
		}
	}
}

func TestExplicitInstanceSurvivesConfigDirMove(t *testing.T) {
	root := t.TempDir()
	baseA := filepath.Join(root, "config-a")
	baseB := filepath.Join(root, "config-b")
	if err := os.MkdirAll(baseA, 0o700); err != nil {
		t.Fatal(err)
	}
	DeactivateAuthStore(baseA)
	DeactivateAuthStore(baseB)
	t.Cleanup(func() {
		DeactivateAuthStore(baseA)
		DeactivateAuthStore(baseB)
	})

	tx, err := BeginNewInstance(baseA, "portable-path")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenData(baseA, testAccountToken("corp-move", "user-move", "access-move")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	service := tx.Store().KeychainService
	cleanupAuthServiceForTest(t, service)
	DeactivateAuthStore(baseA)

	if err := os.Rename(baseA, baseB); err != nil {
		t.Fatalf("move config directory: %v", err)
	}
	store, err := ActivateCurrentInstance(baseB)
	if err != nil {
		t.Fatalf("ActivateCurrentInstance(moved) error = %v", err)
	}
	if store.KeychainService != service || !strings.HasPrefix(store.ConfigDir, baseB+string(filepath.Separator)) {
		t.Fatalf("moved store = %#v, want service %q under %q", store, service, baseB)
	}
	registryData, err := os.ReadFile(AuthRegistryPath(baseB))
	if err != nil {
		t.Fatalf("read moved registry: %v", err)
	}
	for _, secret := range []string{"access-move", "refresh-access-move", "corp-move", "user-move"} {
		if bytes.Contains(registryData, []byte(secret)) {
			t.Fatalf("registry leaked login identity or token %q: %s", secret, registryData)
		}
	}
	assertActiveToken(t, baseB, "access-move", "corp-move")
}

func TestResolveProfileByUserIDRejectsAmbiguousOrganization(t *testing.T) {
	base := t.TempDir()
	DeactivateAuthStore(base)
	t.Cleanup(func() { DeactivateAuthStore(base) })
	cleanupAuthServiceForTest(t, keychain.Service)
	if err := SaveTokenData(base, testAccountToken("corp-one", "same-user", "access-one")); err != nil {
		t.Fatal(err)
	}
	if err := SaveTokenData(base, testAccountToken("corp-two", "same-user", "access-two")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProfileByUserID(base, "same-user"); err == nil || !strings.Contains(err.Error(), "multiple organizations") {
		t.Fatalf("ResolveProfileByUserID() error = %v, want ambiguity", err)
	}
}

func permissionOf(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
}

func cleanupAuthServiceForTest(t *testing.T, service string) {
	t.Helper()
	t.Cleanup(func() { _ = os.RemoveAll(keychain.StorageDir(service)) })
}

func testAccountToken(corpID, userID, accessToken string) *TokenData {
	return &TokenData{
		AccessToken:  accessToken,
		RefreshToken: "refresh-" + accessToken,
		ExpiresAt:    time.Now().Add(time.Hour),
		RefreshExpAt: time.Now().Add(24 * time.Hour),
		CorpID:       corpID,
		CorpName:     corpID + "-name",
		UserID:       userID,
		UserName:     userID + "-name",
	}
}

func assertActiveToken(t *testing.T, base, wantAccess, wantCorp string) {
	t.Helper()
	token, err := LoadTokenData(base)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if token.AccessToken != wantAccess || token.CorpID != wantCorp {
		t.Fatalf("LoadTokenData() = %#v, want access=%q corp=%q", token, wantAccess, wantCorp)
	}
}

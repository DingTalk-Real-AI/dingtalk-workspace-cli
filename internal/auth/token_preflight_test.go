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

//go:build darwin

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type preflightRoundTripFunc func(*http.Request) (*http.Response, error)

func (f preflightRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func seedUnreadableTokenStorage(t *testing.T, configDir string, data *TokenData) {
	t.Helper()
	t.Setenv(keychain.DisableKeychainEnv, "1")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x7f}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement DEK) error = %v", err)
	}
}

func setPreflightTestCredentials(t *testing.T) {
	t.Helper()
	SetClientID("preflight-client-id")
	SetClientSecret("preflight-client-secret")
	resetClientIDFromMCP()
	t.Cleanup(func() {
		SetClientID("")
		SetClientSecret("")
		resetClientIDFromMCP()
	})
}

func profileCiphertextPathForTest(corpID string) string {
	account := strings.ReplaceAll(TokenAccountForCorpID(corpID), ":", "_")
	return filepath.Join(keychain.StorageDir(keychain.Service), account+".enc")
}

func TestLoadTokenDataFallsBackToLegacyOnlyWhenCurrentSlotIsMissing(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_fallback", "corp_fallback", "Fallback Org")

	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := DeleteTokenDataKeychainForCorpID(data.CorpID); err != nil {
		t.Fatalf("DeleteTokenDataKeychainForCorpID() error = %v", err)
	}
	if err := preflightTokenPersistence(configDir); err != nil {
		t.Fatalf("preflightTokenPersistence() with missing profile slot error = %v", err)
	}

	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded.AccessToken != data.AccessToken {
		t.Fatalf("fallback access token = %q, want %q", loaded.AccessToken, data.AccessToken)
	}
}

func TestLoadTokenDataUsesIdentitySlotWhenOrganizationMirrorIsUnreadable(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_unreadable", "corp_unreadable", "Unreadable Org")

	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(data.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	loaded, err := LoadTokenData(configDir)
	if err != nil {
		t.Fatalf("LoadTokenData() error = %v", err)
	}
	if loaded == nil || loaded.AccessToken != data.AccessToken || loaded.UserID != data.UserID {
		t.Fatalf("LoadTokenData() = %#v, want identity token %#v", loaded, data)
	}
}

func TestPreflightTokenPersistenceAllowsEmptyStorageWithoutCreatingDEK(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	if err := preflightTokenPersistence(configDir); err != nil {
		t.Fatalf("preflightTokenPersistence() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if _, err := os.Stat(dekPath); !os.IsNotExist(err) {
		t.Fatalf("preflight created a DEK at %q; stat error = %v", dekPath, err)
	}
}

func TestPreflightTokenPersistenceRejectsUnreadableProfileSlot(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_preflight", "corp_preflight", "Preflight Org")

	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(data.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	err := preflightTokenPersistence(configDir)
	if err == nil || !strings.Contains(err.Error(), "profile token slot") {
		t.Fatalf("preflightTokenPersistence() error = %v, want unreadable profile slot", err)
	}
	if !strings.Contains(err.Error(), "dws auth logout --profile \""+data.CorpID+"\"") {
		t.Fatalf("preflightTokenPersistence() error = %v, want per-profile recovery hint", err)
	}
}

func TestExactOrgCurrentRefreshRejectsUnreadableOrgMirror(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_exact_refresh", "corp_exact", "Exact Org")
	data.UserID = "user_exact"
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(data.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	SetRuntimeProfile("corp_exact:user_exact")
	defer SetRuntimeProfile("")
	if err := preflightTokenRefreshPersistence(configDir, data); err == nil ||
		!strings.Contains(err.Error(), "profile token slot") {
		t.Fatalf("preflightTokenRefreshPersistence(exact current) error = %v, want unreadable org mirror", err)
	}
}

func TestSaveLoginTokenDataRepairsTargetOrgCiphertextMismatch(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-readable", "corp_login_repair", "Login Repair Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy) error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	primaryDEK, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(primary DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6d}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(alternate DEK) error = %v", err)
	}
	staleOrg := testToken("stale-unreadable", legacy.CorpID, legacy.CorpName)
	if err := SaveTokenDataKeychainForCorpID(staleOrg.CorpID, staleOrg); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID(stale) error = %v", err)
	}
	if err := os.WriteFile(dekPath, primaryDEK, 0o600); err != nil {
		t.Fatalf("restore primary DEK error = %v", err)
	}
	if _, err := LoadTokenDataKeychainForCorpID(legacy.CorpID); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v, want ciphertext mismatch", err)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err != nil {
		t.Fatalf("SaveLoginTokenData() error = %v", err)
	}
	assertOrganizationTokenAccessForTest(t, incoming.CorpID, incoming.AccessToken, "")
	global, err := LoadTokenDataKeychain()
	if err != nil {
		t.Fatalf("LoadTokenDataKeychain() error = %v", err)
	}
	if global.AccessToken != incoming.AccessToken {
		t.Fatalf("global token = %q, want %q", global.AccessToken, incoming.AccessToken)
	}
}

func TestSaveLoginTokenDataRepairsGlobalAndOrgSlotsOnCiphertextMismatch(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_global_gate", "Global Gate Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	// Seed the keychain storage so the primary DEK exists before it is read,
	// then remove the seeded slot so both slots below are fresh writes under
	// the alternate DEK (overwriting an existing slot re-validates its DEK).
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy) error = %v", err)
	}
	if err := DeleteTokenDataKeychain(); err != nil {
		t.Fatalf("DeleteTokenDataKeychain() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	primaryDEK, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(primary DEK) error = %v", err)
	}
	// Encrypt both the global mirror and the target org slot with an alternate
	// DEK so neither can be decrypted once the primary DEK is restored.
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6d}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(alternate DEK) error = %v", err)
	}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy, alternate DEK) error = %v", err)
	}
	staleOrg := testToken("stale-unreadable", legacy.CorpID, legacy.CorpName)
	if err := SaveTokenDataKeychainForCorpID(staleOrg.CorpID, staleOrg); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID(stale) error = %v", err)
	}
	if err := os.WriteFile(dekPath, primaryDEK, 0o600); err != nil {
		t.Fatalf("restore primary DEK error = %v", err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychain() error = %v, want ciphertext mismatch", err)
	}
	if _, err := LoadTokenDataKeychainForCorpID(legacy.CorpID); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v, want ciphertext mismatch", err)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err != nil {
		t.Fatalf("SaveLoginTokenData() error = %v", err)
	}
	// The repair must clear both mismatch slots so the new token lands.
	global, err := LoadTokenDataKeychain()
	if err != nil {
		t.Fatalf("LoadTokenDataKeychain() error = %v", err)
	}
	if global.AccessToken != incoming.AccessToken {
		t.Fatalf("global token = %q, want %q", global.AccessToken, incoming.AccessToken)
	}
	assertOrganizationTokenAccessForTest(t, incoming.CorpID, incoming.AccessToken, "")
}

func TestSaveLoginTokenDataRepairAbortsUntouchedOnTransientReadError(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-readable", "corp_transient_guard", "Transient Guard Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	// Seed the keychain storage so the primary DEK exists, then re-encrypt the
	// global slot under an alternate DEK so the check phase flags it for removal.
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy) error = %v", err)
	}
	if err := DeleteTokenDataKeychain(); err != nil {
		t.Fatalf("DeleteTokenDataKeychain() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	primaryDEK, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(primary DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6d}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(alternate DEK) error = %v", err)
	}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy, alternate DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, primaryDEK, 0o600); err != nil {
		t.Fatalf("restore primary DEK error = %v", err)
	}

	// A transient read error on the org slot must abort the repair before the
	// mismatch global slot is removed.
	origGet := authKeychainGet
	defer func() { authKeychainGet = origGet }()
	transientErr := errors.New("transient keychain I/O failure")
	authKeychainGet = func(service, account string) (string, error) {
		if account == TokenAccountForCorpID(legacy.CorpID) {
			return "", transientErr
		}
		return origGet(service, account)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err == nil {
		t.Fatal("SaveLoginTokenData() error = nil, want transient read failure")
	}
	// No removal may have run: the global slot keeps its mismatch ciphertext.
	if _, err := LoadTokenDataKeychain(); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychain() after abort error = %v, want preserved ciphertext mismatch", err)
	}
}

// seedMismatchedGlobalSlot writes the global mirror under an alternate DEK and
// then restores the primary DEK, leaving a slot that LoadTokenDataKeychain
// reports as a ciphertext mismatch until a writer replaces it.
func seedMismatchedGlobalSlot(t *testing.T, legacy *TokenData) {
	t.Helper()
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy) error = %v", err)
	}
	if err := DeleteTokenDataKeychain(); err != nil {
		t.Fatalf("DeleteTokenDataKeychain() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	primaryDEK, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(primary DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6d}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(alternate DEK) error = %v", err)
	}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy, alternate DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, primaryDEK, 0o600); err != nil {
		t.Fatalf("restore primary DEK error = %v", err)
	}
}

func TestSaveLoginTokenDataRepairsAllThreeSlotsOnCiphertextMismatch(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_identity_gate", "Identity Gate Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	// Seed the primary DEK, then encrypt the global, org, and identity slots
	// under an alternate DEK so all three become unreadable once restored.
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy) error = %v", err)
	}
	if err := DeleteTokenDataKeychain(); err != nil {
		t.Fatalf("DeleteTokenDataKeychain() error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	primaryDEK, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(primary DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6d}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(alternate DEK) error = %v", err)
	}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain(legacy, alternate DEK) error = %v", err)
	}
	if err := SaveTokenDataKeychainForCorpID(legacy.CorpID, legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID(legacy, alternate DEK) error = %v", err)
	}
	staleIdentity := testToken("stale-identity", legacy.CorpID, legacy.CorpName)
	if err := SaveTokenDataKeychainForIdentity(staleIdentity.CorpID, staleIdentity.UserID, staleIdentity); err != nil {
		t.Fatalf("SaveTokenDataKeychainForIdentity(stale, alternate DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, primaryDEK, 0o600); err != nil {
		t.Fatalf("restore primary DEK error = %v", err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychain() error = %v, want ciphertext mismatch", err)
	}
	if _, err := LoadTokenDataKeychainForCorpID(legacy.CorpID); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v, want ciphertext mismatch", err)
	}
	if _, err := LoadTokenDataKeychainForIdentity(staleIdentity.CorpID, staleIdentity.UserID); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychainForIdentity() error = %v, want ciphertext mismatch", err)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	if err := SaveLoginTokenData(configDir, incoming); err != nil {
		t.Fatalf("SaveLoginTokenData() error = %v", err)
	}
	// The repair must clear all three mismatch slots so the new token lands.
	global, err := LoadTokenDataKeychain()
	if err != nil {
		t.Fatalf("LoadTokenDataKeychain() error = %v", err)
	}
	if global.AccessToken != incoming.AccessToken {
		t.Fatalf("global token = %q, want %q", global.AccessToken, incoming.AccessToken)
	}
	assertOrganizationTokenAccessForTest(t, incoming.CorpID, incoming.AccessToken, incoming.UserID)
	assertIdentityTokenAccessForTest(t, incoming.CorpID, incoming.UserID, incoming.AccessToken)
}

func TestSaveLoginTokenDataRepairFailsWhenSlotRemovalFails(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_remove_fail", "Remove Fail Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	seedMismatchedGlobalSlot(t, legacy)

	// A removal I/O failure must surface to the caller instead of silently
	// continuing: only the removal phase itself can leave a partial repair.
	origRemove := authKeychainRemove
	defer func() { authKeychainRemove = origRemove }()
	removeErr := errors.New("keychain remove I/O failure")
	authKeychainRemove = func(service, account string) error {
		if account == keychain.AccountToken {
			return removeErr
		}
		return origRemove(service, account)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "remove unreadable legacy token slot") {
		t.Fatalf("SaveLoginTokenData() error = %v, want remove unreadable legacy token slot", err)
	}
}

func TestSaveLoginTokenDataRepairAbortsUntouchedOnGlobalReadError(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_global_read_guard", "Global Read Guard Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	seedMismatchedGlobalSlot(t, legacy)

	// A transient read error on the global slot must abort the repair before
	// any mismatch slot is removed.
	origGet := authKeychainGet
	defer func() { authKeychainGet = origGet }()
	transientErr := errors.New("transient keychain I/O failure")
	authKeychainGet = func(service, account string) (string, error) {
		if account == keychain.AccountToken {
			return "", transientErr
		}
		return origGet(service, account)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err == nil {
		t.Fatal("SaveLoginTokenData() error = nil, want transient read failure")
	}
	// Restore reads so the untouched slot can be verified as still mismatched.
	authKeychainGet = origGet
	if _, err := LoadTokenDataKeychain(); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychain() after abort error = %v, want preserved ciphertext mismatch", err)
	}
}

func TestRepairLoginCiphertextMismatchTargetsSkipsUnderEditionSaveTokenHook(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_edition_hook", "Edition Hook Org")
	seedMismatchedGlobalSlot(t, legacy)

	// Editions that overlay token persistence must not touch the local
	// keychain slots, so the repair is a no-op when SaveToken is hooked.
	orig := edition.Get()
	defer edition.Override(orig)
	edition.Override(&edition.Hooks{SaveToken: func(configDir string, data []byte) error { return nil }})

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.FreshAuthorization = true
	if err := repairLoginCiphertextMismatchTargets(configDir, incoming); err != nil {
		t.Fatalf("repairLoginCiphertextMismatchTargets() error = %v, want skip under edition SaveToken hook", err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychain() error = %v, want mismatch slot preserved by edition skip", err)
	}
}

func TestRepairLoginCiphertextMismatchTargetsIgnoresNonFreshAuthorization(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_non_fresh", "Non Fresh Org")
	seedMismatchedGlobalSlot(t, legacy)

	// Only a freshly exchanged login may repair slots; refresh/reuse paths
	// leave the mismatched ciphertext untouched.
	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	if err := repairLoginCiphertextMismatchTargets(configDir, incoming); err != nil {
		t.Fatalf("repairLoginCiphertextMismatchTargets() error = %v, want no-op for non-fresh data", err)
	}
	if _, err := LoadTokenDataKeychain(); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychain() error = %v, want mismatch slot preserved by non-fresh skip", err)
	}
}

func TestExactNonOrgCurrentRefreshIgnoresUnreadableOrgMirror(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	first := testToken("at_first", "corp_exact", "Exact Org")
	first.UserID = "user_first"
	second := testToken("at_second", "corp_exact", "Exact Org")
	second.UserID = "user_second"
	if err := SaveTokenData(configDir, first); err != nil {
		t.Fatalf("SaveTokenData(first) error = %v", err)
	}
	if err := SaveTokenData(configDir, second); err != nil {
		t.Fatalf("SaveTokenData(second) error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(first.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile ciphertext) error = %v", err)
	}

	SetRuntimeProfile("corp_exact:user_first")
	defer SetRuntimeProfile("")
	if err := preflightTokenRefreshPersistence(configDir, first); err != nil {
		t.Fatalf("preflightTokenRefreshPersistence(exact non-current) error = %v", err)
	}
	updated := *first
	updated.AccessToken = "at_first_refreshed"
	updated.RefreshToken = "rt_first_refreshed"
	if err := SaveTokenData(configDir, &updated); err != nil {
		t.Fatalf("SaveTokenData(exact non-current refresh) error = %v", err)
	}
	loaded, err := LoadTokenDataForProfile(configDir, "corp_exact:user_first")
	if err != nil {
		t.Fatalf("LoadTokenDataForProfile(refreshed) error = %v", err)
	}
	if loaded.AccessToken != updated.AccessToken || loaded.RefreshToken != updated.RefreshToken {
		t.Fatalf("refreshed exact token = %#v, want %#v", loaded, updated)
	}
}

func TestCrossPlatformCoverageExactRefreshAndSwitchIgnoreUnreadableReservedBlankOrgSlot(t *testing.T) {
	t.Setenv(keychain.DisableKeychainEnv, "1")
	fixture := seedLegacyBlankAndExactIdentitySlots(t)
	if err := os.WriteFile(profileCiphertextPathForTest(fixture.corpID), []byte("corrupt reserved blank slot"), 0o600); err != nil {
		t.Fatalf("WriteFile(reserved blank ciphertext) error = %v", err)
	}

	SetRuntimeProfile("")
	if err := preflightTokenRefreshPersistence(fixture.configDir, fixture.beta); err != nil {
		t.Fatalf("preflightTokenRefreshPersistence(current exact with reserved blank) error = %v", err)
	}
	refreshed := *fixture.beta
	refreshed.AccessToken = "at_identity_beta_refreshed"
	if err := SaveTokenData(fixture.configDir, &refreshed); err != nil {
		t.Fatalf("SaveTokenData(current exact with reserved blank) error = %v", err)
	}
	if raw, err := os.ReadFile(profileCiphertextPathForTest(fixture.corpID)); err != nil || string(raw) != "corrupt reserved blank slot" {
		t.Fatalf("reserved blank slot changed during exact refresh: %q, %v", raw, err)
	}

	if selected, err := SetCurrentProfile(fixture.configDir, profileSelector(fixture.alpha.CorpID, fixture.alpha.UserID)); err != nil || selected.UserID != fixture.alpha.UserID {
		t.Fatalf("SetCurrentProfile(exact with reserved blank) = %#v, %v", selected, err)
	}
	if selected, err := UsePreviousProfile(fixture.configDir); err != nil || selected.UserID != fixture.beta.UserID {
		t.Fatalf("UsePreviousProfile(exact with reserved blank) = %#v, %v", selected, err)
	}
	if raw, err := os.ReadFile(profileCiphertextPathForTest(fixture.corpID)); err != nil || string(raw) != "corrupt reserved blank slot" {
		t.Fatalf("reserved blank slot changed during exact switches: %q, %v", raw, err)
	}

	blankRefresh := *fixture.blank
	blankRefresh.LegacyOrgScopedProfile = fixture.blankName
	SetRuntimeProfile(fixture.blankName)
	if err := preflightTokenRefreshPersistence(fixture.configDir, &blankRefresh); err == nil ||
		!strings.Contains(err.Error(), "profile token slot") {
		t.Fatalf("blank refresh preflight error = %v, want unreadable reserved slot", err)
	}
}

func TestFullTokenPersistenceInventoryDetectsOrphanProfileCiphertext(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_orphan", "corp_orphan", "Orphan Org")

	// Simulate interruption after the profile ciphertext rename but before
	// profiles.json is updated by saveTokenDataLocked.
	if err := SaveTokenDataKeychainForCorpID(data.CorpID, data); err != nil {
		t.Fatalf("SaveTokenDataKeychainForCorpID() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, profilesJSONFile)); !os.IsNotExist(err) {
		t.Fatalf("profiles.json stat error = %v, want missing metadata", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6f}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement DEK) error = %v", err)
	}

	err := preflightTokenPersistence(configDir)
	if err == nil || !strings.Contains(err.Error(), "auth token ciphertext inventory") {
		t.Fatalf("preflightTokenPersistence() error = %v, want orphan ciphertext inventory error", err)
	}
	if !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("preflightTokenPersistence() error = %v, want ciphertext key mismatch in error chain", err)
	}
}

func TestPortableAuthExportRejectsCiphertextFromAnotherDEK(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_portable", "", "")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if !PortableAuthSourceReady() {
		t.Fatal("PortableAuthSourceReady() = false before replacing DEK")
	}

	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x7f}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(replacement DEK) error = %v", err)
	}
	if PortableAuthSourceReady() {
		t.Fatal("PortableAuthSourceReady() = true for ciphertext from another DEK")
	}
	var bundle bytes.Buffer
	if err := ExportPortableAuthBundle(configDir, &bundle); err == nil {
		t.Fatal("ExportPortableAuthBundle() error = nil for ciphertext from another DEK")
	}
	if bundle.Len() != 0 {
		t.Fatalf("ExportPortableAuthBundle() wrote %d bytes, want 0", bundle.Len())
	}
}

func TestRefreshPreflightIgnoresUnreadableUnrelatedProfile(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	dataA := testToken("at_a", "corp_a", "A Org")
	dataB := testToken("at_b", "corp_b", "B Org")
	if err := SaveTokenData(configDir, dataA); err != nil {
		t.Fatalf("SaveTokenData(A) error = %v", err)
	}
	if err := SaveTokenData(configDir, dataB); err != nil {
		t.Fatalf("SaveTokenData(B) error = %v", err)
	}
	if err := os.WriteFile(profileCiphertextPathForTest(dataA.CorpID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(A profile ciphertext) error = %v", err)
	}

	if err := preflightTokenRefreshPersistence(configDir, dataB); err != nil {
		t.Fatalf("preflightTokenRefreshPersistence(B) error = %v", err)
	}
	loaded, err := NewOAuthProvider(configDir, nil).Login(context.Background(), false)
	if err != nil {
		t.Fatalf("Login() with valid B and unreadable A error = %v", err)
	}
	if loaded.AccessToken != dataB.AccessToken {
		t.Fatalf("Login() access token = %q, want %q", loaded.AccessToken, dataB.AccessToken)
	}
}

func TestOAuthLoginUnreadableGlobalFailsClosedBeforeAuthorizationStart(t *testing.T) {
	setPreflightTestCredentials(t)
	for _, force := range []bool{false, true} {
		t.Run("force="+map[bool]string{false: "false", true: "true"}[force], func(t *testing.T) {
			cleanupKeychain(t)
			configDir := t.TempDir()
			seedUnreadableTokenStorage(t, configDir, testToken("at_login", "corp_login", "Login Org"))

			listenErr := errors.New("authorization listener reached")
			var calls atomic.Int32
			oldListen := oauthListen
			oauthListen = func(string, string) (net.Listener, error) {
				calls.Add(1)
				return nil, listenErr
			}
			t.Cleanup(func() { oauthListen = oldListen })

			provider := NewOAuthProvider(configDir, nil)
			provider.NoBrowser = true
			_, err := provider.Login(context.Background(), force)
			if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
				t.Fatalf("Login(force=%v) error = %v, want unreadable-global protection", force, err)
			}
			if errors.Is(err, listenErr) {
				t.Fatalf("Login(force=%v) reached authorization listener: %v", force, err)
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("Login(force=%v) listener calls = %d, want 0", force, got)
			}
		})
	}
}

func TestExchangeAuthCodeRejectsUnreadableGlobalBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	existing := testToken("at_exchange", "corp_exchange", "Exchange Org")
	seedUnreadableTokenStorage(t, configDir, existing)

	var calls atomic.Int32
	var saveCalls atomic.Int32
	oldSave := oauthSaveToken
	oauthSaveToken = func(string, *TokenData) error {
		saveCalls.Add(1)
		return nil
	}
	t.Cleanup(func() { oauthSaveToken = oldSave })
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp_exchange"}`,
			)),
		}, nil
	})}
	_, err := provider.ExchangeAuthCode(context.Background(), "auth-code", existing.UserID)
	if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
		t.Fatalf("ExchangeAuthCode() error = %v, want target token persistence error", err)
	}
	if !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("ExchangeAuthCode() error = %v, want ciphertext key mismatch in error chain", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls = %d, want 0", got)
	}
	if got := saveCalls.Load(); got != 0 {
		t.Fatalf("SaveTokenData calls = %d, want 0", got)
	}
}

func TestDeviceFlowLoginRejectsUnreadableGlobalBeforeDeviceCodeRequest(t *testing.T) {
	cleanupKeychain(t)
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	seedUnreadableTokenStorage(t, configDir, testToken("at_device", "corp_device", "Device Org"))

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected device code request", http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewDeviceFlowProvider(configDir, nil)
	provider.Output = io.Discard
	provider.SetBaseURL(server.URL)
	_, err := provider.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
		t.Fatalf("DeviceFlowProvider.Login() error = %v, want unreadable-global protection", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("device code requests = %d, want 0", got)
	}
}

func TestLockedRefreshPreflightsLegacyMirrorBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	data := testToken("at_refresh", "corp_refresh", "Refresh Org")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	legacyPath := filepath.Join(keychain.StorageDir(keychain.Service), keychain.AccountToken+".enc")
	if err := os.WriteFile(legacyPath, []byte("corrupt legacy ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy ciphertext) error = %v", err)
	}

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected refresh request")
	})}
	_, err := provider.lockedRefresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
		t.Fatalf("lockedRefresh() error = %v, want token persistence preflight error", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("refresh HTTP calls = %d, want 0", got)
	}
}

func TestLockedRefreshRejectsFutureProfilesVersionBeforeHTTP(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()
	data := testToken("at_future_refresh", "corp_future", "Future Org")
	data.ExpiresAt = time.Now().Add(-time.Hour)
	data.RefreshExpAt = time.Now().Add(time.Hour)
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		t.Fatalf("LoadProfiles() error = %v", err)
	}
	cfg.Version = profilesMaxVersion + 1
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), raw, 0o600); err != nil {
		t.Fatalf("write future profiles: %v", err)
	}

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected refresh request")
	})}
	_, err = provider.Login(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("Login() error = %v, want future profiles rejection", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("refresh HTTP calls = %d, want 0", got)
	}
}

func TestExchangeAuthCodeAllowsFirstLogin(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()

	var calls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.Output = io.Discard
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp_new"}`,
			)),
		}, nil
	})}

	data, err := provider.ExchangeAuthCode(context.Background(), "new-code", "user-new")
	if err != nil {
		t.Fatalf("ExchangeAuthCode() error = %v", err)
	}
	if data.AccessToken != "new-access" || data.UserID != "user-new" {
		t.Fatalf("ExchangeAuthCode() data = %#v", data)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestExchangeAuthCodeExplicitUIDSkipsIdentityOverride(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	setPreflightTestCredentials(t)
	configDir := t.TempDir()

	var identityCalls atomic.Int32
	provider := NewOAuthProvider(configDir, nil)
	provider.IdentityEnricher = func(context.Context, *TokenData) error {
		identityCalls.Add(1)
		return errors.New("identity lookup should not run for explicit uid")
	}
	provider.httpClient = &http.Client{Transport: preflightRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":7200,"corpId":"corp_new"}`,
			)),
		}, nil
	})}

	data, err := provider.ExchangeAuthCode(context.Background(), "new-code", "explicit-user")
	if err != nil {
		t.Fatalf("ExchangeAuthCode() error = %v", err)
	}
	if data.UserID != "explicit-user" {
		t.Fatalf("ExchangeAuthCode() userId = %q, want explicit-user", data.UserID)
	}
	if got := identityCalls.Load(); got != 0 {
		t.Fatalf("IdentityEnricher calls = %d, want 0", got)
	}
}

// seedMismatchedTargetSlot writes one credential slot under an alternate DEK
// and then restores the primary DEK, leaving a slot that loads as a ciphertext
// mismatch until a writer replaces it.
func seedMismatchedTargetSlot(t *testing.T, seed *TokenData, save func() error) {
	t.Helper()
	if err := SaveTokenDataKeychain(seed); err != nil {
		t.Fatalf("SaveTokenDataKeychain(seed) error = %v", err)
	}
	if err := DeleteTokenDataKeychain(); err != nil {
		t.Fatalf("DeleteTokenDataKeychain(seed) error = %v", err)
	}
	dekPath := filepath.Join(keychain.StorageDir(keychain.Service), "dek")
	primaryDEK, err := os.ReadFile(dekPath)
	if err != nil {
		t.Fatalf("ReadFile(primary DEK) error = %v", err)
	}
	if err := os.WriteFile(dekPath, bytes.Repeat([]byte{0x6d}, 32), 0o600); err != nil {
		t.Fatalf("WriteFile(alternate DEK) error = %v", err)
	}
	if err := save(); err != nil {
		t.Fatalf("save target slot under alternate DEK error = %v", err)
	}
	if err := os.WriteFile(dekPath, primaryDEK, 0o600); err != nil {
		t.Fatalf("restore primary DEK error = %v", err)
	}
}

func identityCiphertextPathForTest(corpID, userID string) string {
	account := strings.ReplaceAll(TokenAccountForIdentity(corpID, userID), ":", "_")
	return filepath.Join(keychain.StorageDir(keychain.Service), account+".enc")
}

func TestRepairLoginCiphertextMismatchTargetsAbortsOnProfilesLoadError(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	// A registry read failure must abort the repair before any slot is touched.
	origLoad := profilesLoad
	defer func() { profilesLoad = origLoad }()
	profilesLoad = func(string) (*ProfilesConfig, error) {
		return nil, errors.New("profiles registry read failure")
	}

	incoming := testToken("fresh-login", "corp_load_err", "Load Err Org")
	incoming.FreshAuthorization = true
	if err := repairLoginCiphertextMismatchTargets(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "profiles registry read failure") {
		t.Fatalf("repairLoginCiphertextMismatchTargets() error = %v, want profiles registry read failure", err)
	}
}

func TestSaveLoginTokenDataRepairFailsWhenIdentitySlotRemovalFails(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_identity_remove", "Identity Remove Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	const userID = "user_identity_remove"
	seedMismatchedTargetSlot(t, legacy, func() error {
		return SaveTokenDataKeychainForIdentity(legacy.CorpID, userID, legacy)
	})
	if _, err := LoadTokenDataKeychainForIdentity(legacy.CorpID, userID); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychainForIdentity() error = %v, want ciphertext mismatch", err)
	}

	// A removal I/O failure on the identity slot must surface to the caller
	// instead of silently continuing.
	origRemove := authKeychainRemove
	defer func() { authKeychainRemove = origRemove }()
	removeErr := errors.New("keychain remove I/O failure")
	authKeychainRemove = func(service, account string) error {
		if account == TokenAccountForIdentity(legacy.CorpID, userID) {
			return removeErr
		}
		return origRemove(service, account)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = userID
	if err := SaveLoginTokenData(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "remove unreadable identity token slot") {
		t.Fatalf("SaveLoginTokenData() error = %v, want remove unreadable identity token slot", err)
	}
}

func TestSaveLoginTokenDataRepairFailsWhenOrgSlotRemovalFails(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := testToken("legacy-unreadable", "corp_org_remove", "Org Remove Org")
	legacy.UserID = ""
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	seedMismatchedTargetSlot(t, legacy, func() error {
		return SaveTokenDataKeychainForCorpID(legacy.CorpID, legacy)
	})
	if _, err := LoadTokenDataKeychainForCorpID(legacy.CorpID); !keychain.IsCiphertextKeyMismatch(err) {
		t.Fatalf("LoadTokenDataKeychainForCorpID() error = %v, want ciphertext mismatch", err)
	}

	// A removal I/O failure on the org slot must surface to the caller instead
	// of silently continuing.
	origRemove := authKeychainRemove
	defer func() { authKeychainRemove = origRemove }()
	removeErr := errors.New("keychain remove I/O failure")
	authKeychainRemove = func(service, account string) error {
		if account == TokenAccountForCorpID(legacy.CorpID) {
			return removeErr
		}
		return origRemove(service, account)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "remove unreadable profile token slot") {
		t.Fatalf("SaveLoginTokenData() error = %v, want remove unreadable profile token slot", err)
	}
}

func TestPreflightTokenRefreshPersistenceRejectsUnreadableIdentitySlot(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	data := testToken("at_identity_preflight", "corp_identity_preflight", "Identity Preflight Org")
	if err := SaveTokenData(configDir, data); err != nil {
		t.Fatalf("SaveTokenData() error = %v", err)
	}
	if err := os.WriteFile(identityCiphertextPathForTest(data.CorpID, data.UserID), []byte("corrupt ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile(identity ciphertext) error = %v", err)
	}

	err := preflightTokenRefreshPersistence(configDir, data)
	if err == nil || !strings.Contains(err.Error(), "identity token slot") {
		t.Fatalf("preflightTokenRefreshPersistence() error = %v, want unreadable identity slot", err)
	}
}

func TestSaveLoginTokenDataRejectsCredentiallessLegacyGlobalBeforeSave(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()
	legacy := &TokenData{CorpID: "corp_credentialless", CorpName: "Credentialless Org"}
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        profilesVersion,
		CurrentProfile: legacy.CorpID,
		Profiles: []Profile{{
			Name:     legacy.CorpName,
			CorpID:   legacy.CorpID,
			CorpName: legacy.CorpName,
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	if err := SaveTokenDataKeychain(legacy); err != nil {
		t.Fatalf("SaveTokenDataKeychain() error = %v", err)
	}

	incoming := testToken("fresh-login", legacy.CorpID, legacy.CorpName)
	incoming.UserID = ""
	if err := SaveLoginTokenData(configDir, incoming); err == nil ||
		!strings.Contains(err.Error(), "no recoverable credential material") {
		t.Fatalf("SaveLoginTokenData() error = %v, want credentialless legacy global rejection", err)
	}
}

func TestSaveLoginTokenDataWrapsWritePreflightFailureAfterRepairPasses(t *testing.T) {
	cleanupKeychain(t)
	t.Setenv(keychain.DisableKeychainEnv, "1")
	configDir := t.TempDir()

	// The repair and prepare phases read the registry through the profilesLoad
	// seam; the on-disk registry carries a future version so only the write
	// preflight, which reads LoadProfiles directly, rejects the login.
	origLoad := profilesLoad
	defer func() { profilesLoad = origLoad }()
	profilesLoad = func(string) (*ProfilesConfig, error) {
		return &ProfilesConfig{Version: profilesVersion}, nil
	}
	raw, err := json.Marshal(&ProfilesConfig{Version: profilesMaxVersion + 1})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(ProfilesPath(configDir), raw, 0o600); err != nil {
		t.Fatalf("write future profiles: %v", err)
	}

	incoming := testToken("fresh-login", "corp_preflight_wrap", "Preflight Wrap Org")
	err = SaveLoginTokenData(configDir, incoming)
	if err == nil ||
		!strings.Contains(err.Error(), "local login state cannot be safely updated") {
		t.Fatalf("SaveLoginTokenData() error = %v, want write preflight failure", err)
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("SaveLoginTokenData() error = %v, want future profiles rejection in chain", err)
	}
}

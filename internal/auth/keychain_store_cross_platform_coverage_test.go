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
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// stubbedKeychain stands in for the platform keychain backend so the
// ciphertext-repair coverage below is identical on macOS and Windows. The
// platform gate only runs TestCrossPlatformCoverage* tests, and these paths
// must pass on every runner, so the tests may not depend on the file-DEK
// backend that only exists on darwin/linux.
type stubbedKeychain struct {
	values     map[string]string
	errs       map[string]error
	removeErrs map[string]error
	removed    []string
}

func newStubbedKeychain() *stubbedKeychain {
	return &stubbedKeychain{
		values:     map[string]string{},
		errs:       map[string]error{},
		removeErrs: map[string]error{},
	}
}

func (s *stubbedKeychain) get(service, account string) (string, error) {
	if err, ok := s.errs[account]; ok {
		return "", err
	}
	return s.values[account], nil
}

func (s *stubbedKeychain) set(service, account, data string) error {
	if err, ok := s.errs[account]; ok {
		return err
	}
	s.values[account] = data
	return nil
}

func (s *stubbedKeychain) remove(service, account string) error {
	s.removed = append(s.removed, account)
	return s.removeErrs[account]
}

func swapKeychainStub(t *testing.T, stub *stubbedKeychain) {
	t.Helper()
	testseam.Swap(t, &authKeychainGet, stub.get)
	testseam.Swap(t, &authKeychainSet, stub.set)
	testseam.Swap(t, &authKeychainRemove, stub.remove)
}

func freshLoginDataForRepairTest(corpID string) *TokenData {
	data := testToken("at_fresh_"+corpID, corpID, "Repair Org")
	data.FreshAuthorization = true
	return data
}

func TestCrossPlatformCoverageCiphertextMismatchRepairHelpers(t *testing.T) {
	if got := authDebugHash(""); got != "" {
		t.Fatalf("authDebugHash(\"\") = %q, want empty", got)
	}
	if got := authDebugHash("   "); got != "" {
		t.Fatalf("authDebugHash(whitespace) = %q, want empty", got)
	}
	if got := authDebugHash("corp-login-value"); len(got) != 16 {
		t.Fatalf("authDebugHash(non-empty) = %q, want 16 hex chars", got)
	}

	if got := authTokenAccountKind(keychain.AccountToken); got != "legacy" {
		t.Fatalf("authTokenAccountKind(legacy) = %q, want legacy", got)
	}
	if got := authTokenAccountKind(TokenAccountForIdentity("corp_kind", "user_kind")); got != "identity" {
		t.Fatalf("authTokenAccountKind(identity) = %q, want identity", got)
	}
	if got := authTokenAccountKind(TokenAccountForCorpID("corp_kind")); got != "corp" {
		t.Fatalf("authTokenAccountKind(corp) = %q, want corp", got)
	}
	if got := authTokenAccountKind("unrelated-account"); got != "other" {
		t.Fatalf("authTokenAccountKind(other) = %q, want other", got)
	}

	// Non-mismatch errors are deliberately not logged.
	logAuthTokenCiphertextMismatch("test.event.not_mismatch", keychain.AccountToken, errors.New("plain error"))
	// Mismatch errors reach the AuthDebug logging path.
	logAuthTokenCiphertextMismatch("test.event.mismatch", keychain.AccountToken, keychain.ErrCiphertextKeyMismatch)
}

func TestCrossPlatformCoverageKeychainSaveLoadLogCiphertextMismatch(t *testing.T) {
	t.Run("save logs ciphertext mismatch", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		testseam.Swap(t, &authKeychainSet, stub.set)

		err := SaveTokenDataKeychain(testToken("at_save_log", "corp_save_log", "Save Log Org"))
		if err == nil || !strings.Contains(err.Error(), "save to keychain") {
			t.Fatalf("SaveTokenDataKeychain() error = %v, want save to keychain", err)
		}
		if !keychain.IsCiphertextKeyMismatch(err) {
			t.Fatalf("SaveTokenDataKeychain() error = %v, want ciphertext key mismatch in chain", err)
		}
	})

	t.Run("load logs ciphertext mismatch", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		testseam.Swap(t, &authKeychainGet, stub.get)

		_, err := LoadTokenDataKeychain()
		if err == nil || !strings.Contains(err.Error(), "load from keychain") {
			t.Fatalf("LoadTokenDataKeychain() error = %v, want load from keychain", err)
		}
		if !keychain.IsCiphertextKeyMismatch(err) {
			t.Fatalf("LoadTokenDataKeychain() error = %v, want ciphertext key mismatch in chain", err)
		}
	})
}

func TestCrossPlatformCoveragePreflightInventoryLogsCiphertextMismatch(t *testing.T) {
	SetRuntimeProfile("")
	defer SetRuntimeProfile("")

	t.Run("legacy slot mismatch", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)

		err := preflightTokenPersistence(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
			t.Fatalf("preflightTokenPersistence() error = %v, want legacy token slot", err)
		}
	})

	t.Run("corp slot mismatch", func(t *testing.T) {
		configDir := t.TempDir()
		corpID := "corp_inventory_log"
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version: profilesVersion,
			Profiles: []Profile{{
				Name:     "Inventory Org",
				CorpID:   corpID,
				CorpName: "Inventory Org",
			}},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		stub := newStubbedKeychain()
		stub.errs[TokenAccountForCorpID(corpID)] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)

		err := preflightTokenPersistence(configDir)
		if err == nil || !strings.Contains(err.Error(), "profile token slot") {
			t.Fatalf("preflightTokenPersistence() error = %v, want profile token slot", err)
		}
	})

	t.Run("identity slot mismatch", func(t *testing.T) {
		configDir := t.TempDir()
		corpID := "corp_inventory_id_log"
		userID := "user_inventory_id_log"
		if err := SaveProfiles(configDir, &ProfilesConfig{
			Version: profilesVersion,
			Profiles: []Profile{{
				Name:     "Inventory Id Org",
				CorpID:   corpID,
				CorpName: "Inventory Id Org",
				UserID:   userID,
			}},
		}); err != nil {
			t.Fatalf("SaveProfiles() error = %v", err)
		}
		stub := newStubbedKeychain()
		stub.errs[TokenAccountForIdentity(corpID, userID)] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)

		err := preflightTokenPersistence(configDir)
		if err == nil || !strings.Contains(err.Error(), "identity token slot") {
			t.Fatalf("preflightTokenPersistence() error = %v, want identity token slot", err)
		}
	})
}

func TestCrossPlatformCoveragePreflightWriteLogsCiphertextMismatch(t *testing.T) {
	SetRuntimeProfile("")
	defer SetRuntimeProfile("")

	data := freshLoginDataForRepairTest("corp_write_log")
	identityAccount := TokenAccountForIdentity(data.CorpID, data.UserID)
	orgAccount := TokenAccountForCorpID(data.CorpID)

	t.Run("global legacy slot mismatch", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)

		err := preflightTokenWritePersistence(t.TempDir(), data)
		if err == nil || !strings.Contains(err.Error(), "legacy token slot") {
			t.Fatalf("preflightTokenWritePersistence() error = %v, want legacy token slot", err)
		}
	})

	t.Run("identity slot mismatch", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[identityAccount] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)

		err := preflightTokenWritePersistence(t.TempDir(), data)
		if err == nil || !strings.Contains(err.Error(), "identity token slot") {
			t.Fatalf("preflightTokenWritePersistence() error = %v, want identity token slot", err)
		}
	})

	t.Run("organization slot mismatch", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[orgAccount] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)

		err := preflightTokenWritePersistence(t.TempDir(), data)
		if err == nil || !strings.Contains(err.Error(), "profile token slot") {
			t.Fatalf("preflightTokenWritePersistence() error = %v, want profile token slot", err)
		}
	})
}

func TestCrossPlatformCoverageRepairCiphertextMismatchEarlyExits(t *testing.T) {
	SetRuntimeProfile("")
	defer SetRuntimeProfile("")

	prev := edition.Get()
	edition.Override(&edition.Hooks{SaveToken: func(string, []byte) error { return nil }})
	if err := repairLoginCiphertextMismatchTargets(t.TempDir(), freshLoginDataForRepairTest("corp_hook")); err != nil {
		t.Fatalf("repair under edition SaveToken hook = %v, want nil", err)
	}
	edition.Override(prev)
	defer edition.Override(prev)

	if err := repairLoginCiphertextMismatchTargets(t.TempDir(), nil); err != nil {
		t.Fatalf("repair(nil data) = %v, want nil", err)
	}
	stale := testToken("at_stale", "corp_stale", "Stale Org")
	if err := repairLoginCiphertextMismatchTargets(t.TempDir(), stale); err != nil {
		t.Fatalf("repair(non-fresh data) = %v, want nil", err)
	}

	testseam.Swap(t, &profilesLoad, func(string) (*ProfilesConfig, error) {
		return nil, errors.New("profiles load boom")
	})
	if err := repairLoginCiphertextMismatchTargets(t.TempDir(), freshLoginDataForRepairTest("corp_load_err")); err == nil ||
		!strings.Contains(err.Error(), "profiles load boom") {
		t.Fatalf("repair(profilesLoad error) = %v, want profiles load boom", err)
	}
}

func TestCrossPlatformCoverageRepairRemovesMismatchedLoginSlots(t *testing.T) {
	SetRuntimeProfile("")
	defer SetRuntimeProfile("")

	data := freshLoginDataForRepairTest("corp_repair")
	identityAccount := TokenAccountForIdentity(data.CorpID, data.UserID)
	orgAccount := TokenAccountForCorpID(data.CorpID)
	emptyProfiles := func(string) (*ProfilesConfig, error) { return &ProfilesConfig{}, nil }

	t.Run("all three mismatch slots are removed", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		stub.errs[identityAccount] = keychain.ErrCiphertextKeyMismatch
		stub.errs[orgAccount] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)
		testseam.Swap(t, &profilesLoad, emptyProfiles)

		if err := repairLoginCiphertextMismatchTargets(t.TempDir(), data); err != nil {
			t.Fatalf("repair(all mismatch) = %v, want nil", err)
		}
		want := map[string]bool{
			keychain.AccountToken: true,
			identityAccount:       true,
			orgAccount:            true,
		}
		if len(stub.removed) != len(want) {
			t.Fatalf("removed %v, want %v", stub.removed, want)
		}
		for _, account := range stub.removed {
			if !want[account] {
				t.Fatalf("removed unexpected account %q", account)
			}
		}
	})

	t.Run("identity mismatch only removes identity slot", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[identityAccount] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)
		testseam.Swap(t, &profilesLoad, emptyProfiles)

		if err := repairLoginCiphertextMismatchTargets(t.TempDir(), data); err != nil {
			t.Fatalf("repair(identity mismatch) = %v, want nil", err)
		}
		if len(stub.removed) != 1 || stub.removed[0] != identityAccount {
			t.Fatalf("removed %v, want only %q", stub.removed, identityAccount)
		}
	})

	t.Run("organization mismatch only removes org slot", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[orgAccount] = keychain.ErrCiphertextKeyMismatch
		swapKeychainStub(t, stub)
		testseam.Swap(t, &profilesLoad, emptyProfiles)

		if err := repairLoginCiphertextMismatchTargets(t.TempDir(), data); err != nil {
			t.Fatalf("repair(org mismatch) = %v, want nil", err)
		}
		if len(stub.removed) != 1 || stub.removed[0] != orgAccount {
			t.Fatalf("removed %v, want only %q", stub.removed, orgAccount)
		}
	})

	t.Run("legacy removal failure aborts repair", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		stub.removeErrs[keychain.AccountToken] = errors.New("remove boom")
		swapKeychainStub(t, stub)
		testseam.Swap(t, &profilesLoad, emptyProfiles)

		err := repairLoginCiphertextMismatchTargets(t.TempDir(), data)
		if err == nil || !strings.Contains(err.Error(), "remove unreadable legacy token slot") {
			t.Fatalf("repair(legacy removal failure) = %v, want legacy removal wrap", err)
		}
	})

	t.Run("identity removal failure aborts repair", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		stub.errs[identityAccount] = keychain.ErrCiphertextKeyMismatch
		stub.removeErrs[identityAccount] = errors.New("remove boom")
		swapKeychainStub(t, stub)
		testseam.Swap(t, &profilesLoad, emptyProfiles)

		err := repairLoginCiphertextMismatchTargets(t.TempDir(), data)
		if err == nil || !strings.Contains(err.Error(), "remove unreadable identity token slot") {
			t.Fatalf("repair(identity removal failure) = %v, want identity removal wrap", err)
		}
	})

	t.Run("organization removal failure aborts repair", func(t *testing.T) {
		stub := newStubbedKeychain()
		stub.errs[keychain.AccountToken] = keychain.ErrCiphertextKeyMismatch
		stub.errs[identityAccount] = keychain.ErrCiphertextKeyMismatch
		stub.errs[orgAccount] = keychain.ErrCiphertextKeyMismatch
		stub.removeErrs[orgAccount] = errors.New("remove boom")
		swapKeychainStub(t, stub)
		testseam.Swap(t, &profilesLoad, emptyProfiles)

		err := repairLoginCiphertextMismatchTargets(t.TempDir(), data)
		if err == nil || !strings.Contains(err.Error(), "remove unreadable profile token slot") {
			t.Fatalf("repair(org removal failure) = %v, want org removal wrap", err)
		}
	})
}

func TestCrossPlatformCoverageProfileMigrationLoginRetryErrorNilUnwrap(t *testing.T) {
	// A nil receiver must stay safe when the retry error is unwrapped through
	// errors.Unwrap or errors.As on an absent target.
	var retryErr *profileMigrationLoginRetryError
	if got := retryErr.Unwrap(); got != nil {
		t.Fatalf("nil retryErr.Unwrap() = %v, want nil", got)
	}
	if got := errors.Unwrap(retryErr); got != nil {
		t.Fatalf("errors.Unwrap(nil retryErr) = %v, want nil", got)
	}
}

func TestCrossPlatformCoverageLoginRetryGuidanceErrorForTestConstructor(t *testing.T) {
	// The app-package guidance test that constructs this error is partitioned
	// outside the CI coverage shards, so this in-package call keeps the
	// constructor statement covered on every platform.
	cause := errors.New("underlying retry cause")
	got := NewLoginRetryGuidanceErrorForTest(cause)
	if got == nil {
		t.Fatal("NewLoginRetryGuidanceErrorForTest() = nil, want retry error")
	}
	if got.Error() != profileLoginRetryGuidance {
		t.Fatalf("NewLoginRetryGuidanceErrorForTest() = %q, want %q", got.Error(), profileLoginRetryGuidance)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("errors.Is(NewLoginRetryGuidanceErrorForTest(cause), cause) = false, want true")
	}
}

func TestCrossPlatformCoverageV1ExplicitProfileMissingDEKRetryOnStubbedKeychain(t *testing.T) {
	configDir := t.TempDir()
	first := testToken("old-first", "corp_v1_stub_retry", "First Org")
	second := testToken("old-second", "corp_v1_stub_retry_second", "Second Org")
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version:        1,
		CurrentProfile: first.CorpID,
		Profiles: []Profile{
			{Name: first.CorpName, CorpID: first.CorpID, CorpName: first.CorpName, UserID: first.UserID},
			{Name: second.CorpName, CorpID: second.CorpID, CorpName: second.CorpName, UserID: second.UserID},
		},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	SetRuntimeProfile(TokenProfileSelector(first))
	t.Cleanup(func() { SetRuntimeProfile("") })

	// The organization slot is unreadable because the DEK is gone. The v1
	// migration must skip it, and the first org-slot write after the registry
	// reached v2 must surface the profile-stable retry guidance instead of a
	// raw technical error. This mirrors the file-DEK scenario without depending
	// on the darwin/linux-only backend, so the branch stays covered on Windows.
	stub := newStubbedKeychain()
	stub.errs[TokenAccountForCorpID(first.CorpID)] = keychain.ErrDEKMissing
	swapKeychainStub(t, stub)

	fresh := *first
	fresh.AccessToken = "new-first"
	err := SaveLoginTokenData(configDir, &fresh)
	if err == nil {
		t.Fatal("SaveLoginTokenData() error = nil, want DEK-missing retry error")
	}
	if !keychain.IsDEKMissing(err) {
		t.Fatalf("SaveLoginTokenData() error = %v, want DEK missing in chain", err)
	}
	const wantGuidance = "请保持 --profile 参数不变，并重新执行 dws auth login"
	if err.Error() != wantGuidance {
		t.Fatalf("SaveLoginTokenData() error = %q, want %q", err, wantGuidance)
	}
	if got, ok := LoginRetryGuidance(err); !ok || got != wantGuidance {
		t.Fatalf("LoginRetryGuidance() = %q, %v; want %q, true", got, ok, wantGuidance)
	}
}

func TestCrossPlatformCoverageV1MigrationHardFailsOnNonDEKOrgSlotError(t *testing.T) {
	configDir := t.TempDir()
	corpID := "corp_v1_hardfail"
	if err := SaveProfiles(configDir, &ProfilesConfig{
		Version: 1,
		Profiles: []Profile{{
			Name:     "V1 Hardfail Org",
			CorpID:   corpID,
			CorpName: "V1 Hardfail Org",
		}},
	}); err != nil {
		t.Fatalf("SaveProfiles() error = %v", err)
	}
	// A v1 migration must keep failing closed on an org-slot read error that is
	// neither NotFound nor a lost DEK (the lost-DEK case is skipped so a fresh
	// login for another profile can proceed).
	stub := newStubbedKeychain()
	stub.errs[TokenAccountForCorpID(corpID)] = errors.New("keychain read boom")
	swapKeychainStub(t, stub)

	err := EnsureProfilesMigration(configDir)
	if err == nil || !strings.Contains(err.Error(), "keychain read boom") {
		t.Fatalf("EnsureProfilesMigration() error = %v, want keychain read boom", err)
	}
}

func TestCrossPlatformCoverageRepairAbortsOnTransientReadError(t *testing.T) {
	SetRuntimeProfile("")
	defer SetRuntimeProfile("")

	data := freshLoginDataForRepairTest("corp_transient")
	identityAccount := TokenAccountForIdentity(data.CorpID, data.UserID)
	orgAccount := TokenAccountForCorpID(data.CorpID)
	emptyProfiles := func(string) (*ProfilesConfig, error) { return &ProfilesConfig{}, nil }

	cases := []struct {
		name       string
		errAccount string
	}{
		{"legacy transient read error", keychain.AccountToken},
		{"identity transient read error", identityAccount},
		{"organization transient read error", orgAccount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStubbedKeychain()
			stub.errs[tc.errAccount] = errors.New("keychain read boom")
			swapKeychainStub(t, stub)
			testseam.Swap(t, &profilesLoad, emptyProfiles)

			err := repairLoginCiphertextMismatchTargets(t.TempDir(), data)
			if err == nil || !strings.Contains(err.Error(), "load from keychain: keychain read boom") {
				t.Fatalf("repair(transient read error) = %v, want load wrap", err)
			}
			if len(stub.removed) != 0 {
				t.Fatalf("removed slots on transient read error: %v", stub.removed)
			}
		})
	}
}

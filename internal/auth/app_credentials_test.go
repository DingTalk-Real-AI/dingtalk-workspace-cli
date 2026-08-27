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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func isolateAppCredentialKeychain(t *testing.T) map[string]string {
	t.Helper()
	entries := map[string]string{}
	oldSecretGet, oldSecretSet := secretKeychainGet, secretKeychainSet
	oldAuthGet, oldAuthSet, oldAuthRemove := authKeychainGet, authKeychainSet, authKeychainRemove
	secretKeychainGet = func(_, account string) (string, error) { return entries[account], nil }
	secretKeychainSet = func(_, account, value string) error { entries[account] = value; return nil }
	authKeychainGet = func(_, account string) (string, error) { return entries[account], nil }
	authKeychainSet = func(_, account, value string) error { entries[account] = value; return nil }
	authKeychainRemove = func(_, account string) error { delete(entries, account); return nil }
	t.Cleanup(func() {
		secretKeychainGet, secretKeychainSet = oldSecretGet, oldSecretSet
		authKeychainGet, authKeychainSet, authKeychainRemove = oldAuthGet, oldAuthSet, oldAuthRemove
		resetAppConfigCache()
		SetClientID("")
		SetClientSecret("")
	})
	resetAppConfigCache()
	return entries
}

func writeCredentialConfig(t *testing.T, dir, clientID string, secret SecretInput) {
	t.Helper()
	cfg := AppConfig{ClientID: clientID, ClientSecret: secret, CreatedAt: time.Now()}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GetAppConfigPath(dir), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageResolveAppCredentialPairAtomicPriority(t *testing.T) {
	isolateAppCredentialKeychain(t)
	dir := t.TempDir()
	writeCredentialConfig(t, dir, "config-id", PlainSecret("config-secret"))

	t.Setenv(EnvClientID, "env-only-id")
	t.Setenv(EnvClientSecret, "")
	pair, err := ResolveAppCredentialPair(dir, "flag-id", "flag-secret")
	if err != nil {
		t.Fatal(err)
	}
	if pair.ClientID != "flag-id" || pair.ClientSecret != "flag-secret" || pair.Source != "flag" {
		t.Fatalf("flag pair = %#v", pair)
	}
	pair, err = ResolveAppCredentialPair(dir, "  trimmed-id  ", "  trimmed-secret  ")
	if err != nil || pair.ClientID != "trimmed-id" || pair.ClientSecret != "trimmed-secret" {
		t.Fatalf("trimmed flag pair = %#v, %v", pair, err)
	}
	if _, err := ResolveAppCredentialPair(dir, "<client-id>", "<client-secret>"); !errors.Is(err, ErrCredentialPlaceholders) {
		t.Fatalf("placeholder error = %v", err)
	}

	if _, err := ResolveAppCredentialPair(dir, "flag-only", ""); !errors.Is(err, ErrFlagCredentialPairIncomplete) {
		t.Fatalf("half flag error = %v", err)
	}
	if _, err := ResolveAppCredentialPair(dir, "", ""); !errors.Is(err, ErrEnvCredentialPairIncomplete) {
		t.Fatalf("half env error = %v", err)
	}

	t.Setenv(EnvClientID, "env-id")
	t.Setenv(EnvClientSecret, "env-secret")
	pair, err = ResolveAppCredentialPair(dir, "", "")
	if err != nil || pair.ClientID != "env-id" || pair.ClientSecret != "env-secret" || pair.Source != "env" {
		t.Fatalf("env pair = %#v, %v", pair, err)
	}
}

func TestCrossPlatformCoverageSaveClientSecretRepairsLegacyConflict(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	entries[secretAccountKey("repair-id")] = "stale-canonical"
	entries[legacyClientSecretAccountKey("repair-id")] = "stale-legacy"
	if err := SaveClientSecret(" repair-id ", " fresh-secret "); err != nil {
		t.Fatal(err)
	}
	if entries[secretAccountKey("repair-id")] != "fresh-secret" || entries[legacyClientSecretAccountKey("repair-id")] != "" {
		t.Fatalf("repaired secret slots = %#v", entries)
	}
}

func TestCrossPlatformCoverageResolveAppConfigCredentialPairMigratesDerivedSlots(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")

	t.Run("canonical slot fills empty config", func(t *testing.T) {
		dir := t.TempDir()
		entries[secretAccountKey("canonical-id")] = "canonical-secret"
		writeCredentialConfig(t, dir, "canonical-id", SecretInput{})
		pair, err := ResolveAppConfigCredentialPair(dir)
		if err != nil || pair.ClientSecret != "canonical-secret" {
			t.Fatalf("pair = %#v, %v", pair, err)
		}
		cfg, _ := LoadAppConfig(dir)
		if cfg.ClientSecret.Ref == nil || cfg.ClientSecret.Ref.ID != secretAccountKey("canonical-id") {
			t.Fatalf("canonical config = %#v", cfg)
		}
	})

	t.Run("legacy slot migrates", func(t *testing.T) {
		dir := t.TempDir()
		entries[legacyClientSecretAccountKey("legacy-id")] = "legacy-secret"
		writeCredentialConfig(t, dir, "legacy-id", SecretInput{})
		pair, err := ResolveAppConfigCredentialPair(dir)
		if err != nil || pair.ClientSecret != "legacy-secret" {
			t.Fatalf("pair = %#v, %v", pair, err)
		}
		if entries[secretAccountKey("legacy-id")] != "legacy-secret" || entries[legacyClientSecretAccountKey("legacy-id")] != "" {
			t.Fatalf("migrated slots = %#v", entries)
		}
		// Repeated resolution is idempotent.
		if _, err := ResolveAppConfigCredentialPair(dir); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("plain config migrates", func(t *testing.T) {
		dir := t.TempDir()
		writeCredentialConfig(t, dir, "plain-id", PlainSecret("plain-secret"))
		pair, err := ResolveAppConfigCredentialPair(dir)
		if err != nil || pair.ClientSecret != "plain-secret" {
			t.Fatalf("pair = %#v, %v", pair, err)
		}
		cfg, _ := LoadAppConfig(dir)
		if cfg.ClientSecret.Ref == nil || cfg.ClientSecret.Ref.ID != secretAccountKey("plain-id") {
			t.Fatalf("plain config was not migrated: %#v", cfg)
		}
	})
}

func TestCrossPlatformCoverageResolveAppConfigCredentialPairFailsClosed(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")

	dir := t.TempDir()
	entries[secretAccountKey("conflict-id")] = "new-secret"
	entries[legacyClientSecretAccountKey("conflict-id")] = "old-secret"
	writeCredentialConfig(t, dir, "conflict-id", SecretInput{})
	if _, err := ResolveAppConfigCredentialPair(dir); !errors.Is(err, ErrClientSecretConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	dir = t.TempDir()
	entries[secretAccountKey("broken-id")] = "fallback-must-not-run"
	missing := GetAppConfigPath(dir) + ".missing"
	writeCredentialConfig(t, dir, "broken-id", SecretInput{Ref: &SecretRef{Source: "file", ID: missing}})
	_, err := ResolveAppConfigCredentialPair(dir)
	if !errors.Is(err, ErrSecretResolve) {
		t.Fatalf("broken explicit ref error = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "fallback-must-not-run") {
		t.Fatalf("error leaked secret: %v", err)
	}

	dir = t.TempDir()
	entries[secretAccountKey("other-id")] = "other-secret"
	writeCredentialConfig(t, dir, "expected-id", SecretInput{Ref: &SecretRef{Source: "keychain", ID: secretAccountKey("other-id")}})
	_, err = ResolveAppConfigCredentialPair(dir)
	if !errors.Is(err, ErrClientSecretRefMismatch) {
		t.Fatalf("mismatched keychain ref error = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "other-secret") {
		t.Fatalf("mismatched keychain ref error leaked secret: %v", err)
	}
}

func TestCrossPlatformCoverageExplicitFileSecretDoesNotRequireKeychain(t *testing.T) {
	isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "secret")
	if err := os.WriteFile(secretPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCredentialConfig(t, dir, "file-id", SecretInput{Ref: &SecretRef{Source: "file", ID: secretPath}})
	failure := errors.New("keychain unavailable")
	secretKeychainGet = func(string, string) (string, error) { return "", failure }
	authKeychainGet = func(string, string) (string, error) { return "", failure }
	pair, err := ResolveAppConfigCredentialPair(dir)
	if err != nil || pair.ClientSecret != "file-secret" {
		t.Fatalf("file pair = %#v, %v", pair, err)
	}
}

func TestCrossPlatformCoverageLegacyCleanupFailureKeepsResolvedPairUsable(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	dir := t.TempDir()
	entries[legacyClientSecretAccountKey("cleanup-id")] = "cleanup-secret"
	writeCredentialConfig(t, dir, "cleanup-id", SecretInput{})
	authKeychainRemove = func(string, string) error { return errors.New("cleanup unavailable") }
	pair, err := ResolveAppConfigCredentialPair(dir)
	if err != nil || pair.ClientSecret != "cleanup-secret" {
		t.Fatalf("pair after cleanup failure = %#v, %v", pair, err)
	}
	if entries[secretAccountKey("cleanup-id")] != "cleanup-secret" {
		t.Fatal("canonical write did not complete before cleanup failure")
	}
}

func TestCrossPlatformCoverageClientSecretMigrationDoesNotOverwriteConcurrentLoginPair(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	dir := t.TempDir()
	entries[legacyClientSecretAccountKey("same-id")] = "old-secret"
	writeCredentialConfig(t, dir, "same-id", SecretInput{})

	migrationReached := make(chan struct{})
	allowMigration := make(chan struct{})
	testseam.Swap(t, &appCredentialsBeforeMigrationLock, func() {
		close(migrationReached)
		<-allowMigration
	})
	type result struct {
		pair AppCredentialPair
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		pair, err := ResolveAppConfigCredentialPair(dir)
		resultCh <- result{pair: pair, err: err}
	}()
	<-migrationReached

	if err := SaveAppConfig(dir, &AppConfig{ClientID: "same-id", ClientSecret: PlainSecret("new-secret")}); err != nil {
		t.Fatal(err)
	}
	close(allowMigration)
	got := <-resultCh
	if got.err != nil || got.pair.ClientSecret != "old-secret" {
		t.Fatalf("in-flight resolver pair = %#v, %v", got.pair, got.err)
	}

	cfg, err := LoadAppConfig(dir)
	if err != nil || cfg == nil || !isCanonicalClientSecretRef(cfg.ClientSecret, "same-id") {
		t.Fatalf("concurrent login config = %#v, %v", cfg, err)
	}
	if entries[secretAccountKey("same-id")] != "new-secret" {
		t.Fatal("stale migration overwrote the newly logged-in Client Secret")
	}
}

func TestCrossPlatformCoverageCredentialResolutionWithoutMigrationLeavesLegacyStateUntouched(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	dir := t.TempDir()
	entries[legacyClientSecretAccountKey("legacy-id")] = "legacy-secret"
	writeCredentialConfig(t, dir, "legacy-id", SecretInput{})

	pair, err := resolveAppCredentialPairWithoutMigration(dir, "", "")
	if err != nil || pair.ClientSecret != "legacy-secret" {
		t.Fatalf("non-migrating pair = %#v, %v", pair, err)
	}
	if entries[secretAccountKey("legacy-id")] != "" {
		t.Fatal("non-migrating resolver wrote the canonical slot")
	}
	cfg, err := LoadAppConfig(dir)
	if err != nil || cfg == nil || !cfg.ClientSecret.IsZero() {
		t.Fatalf("non-migrating resolver changed app config: %#v, %v", cfg, err)
	}
}

func TestCrossPlatformCoverageOAuthCredentialSnapshotPersistsFlagsAndEnvOnlyAfterSuccessHook(t *testing.T) {
	entries := isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "env-id")
	t.Setenv(EnvClientSecret, "env-secret")
	dir := t.TempDir()
	p := NewOAuthProvider(dir, nil)
	if cfg, err := LoadAppConfig(dir); err != nil || cfg != nil {
		t.Fatalf("constructor persisted config: %#v, %v", cfg, err)
	}
	p.persistAppConfigIfNeeded()
	cfg, err := LoadAppConfig(dir)
	if err != nil || cfg == nil || cfg.ClientID != "env-id" || cfg.ClientSecret.Ref == nil {
		t.Fatalf("persisted env config = %#v, %v", cfg, err)
	}
	if entries[secretAccountKey("env-id")] != "env-secret" {
		t.Fatal("env secret was not stored in the canonical slot")
	}

	SetClientID("flag-id")
	SetClientSecret("flag-secret")
	entries[legacyClientSecretAccountKey("flag-id")] = "stale-legacy-secret"
	flagDir := t.TempDir()
	flagProvider := NewOAuthProvider(flagDir, nil)
	flagProvider.persistAppConfigIfNeeded()
	flagCfg, err := LoadAppConfig(flagDir)
	if err != nil || flagCfg == nil || flagCfg.ClientID != "flag-id" || entries[secretAccountKey("flag-id")] != "flag-secret" {
		t.Fatalf("persisted flag config = %#v, %v", flagCfg, err)
	}
	if entries[legacyClientSecretAccountKey("flag-id")] != "" {
		t.Fatal("successful pair persistence did not remove the stale legacy slot")
	}
}

func TestCrossPlatformCoverageOAuthSilentLoginDoesNotPersistUnvalidatedReplacementPair(t *testing.T) {
	isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "replacement-id")
	t.Setenv(EnvClientSecret, "replacement-secret")
	dir := t.TempDir()
	oldLoad := oauthLoadToken
	oauthLoadToken = func(string) (*TokenData, error) {
		return &TokenData{AccessToken: "still-valid", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	t.Cleanup(func() { oauthLoadToken = oldLoad })
	p := NewOAuthProvider(dir, nil)
	if _, err := p.Login(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadAppConfig(dir)
	if err != nil || cfg != nil {
		t.Fatalf("silent login persisted an unvalidated replacement pair: %#v, %v", cfg, err)
	}
}

func TestCrossPlatformCoverageOAuthConstructorsDoNotMutateCredentialEnvironment(t *testing.T) {
	isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	dir := t.TempDir()
	writeCredentialConfig(t, dir, "config-id", PlainSecret("config-secret"))
	_ = NewOAuthProvider(dir, nil)
	_ = NewDeviceFlowProvider(dir, nil)
	if os.Getenv(EnvClientID) != "" || os.Getenv(EnvClientSecret) != "" {
		t.Fatalf("constructors mutated env: id=%q secret_set=%t", os.Getenv(EnvClientID), os.Getenv(EnvClientSecret) != "")
	}
}

func TestCrossPlatformCoverageExplicitRuntimePairClearsStaleMCPMarker(t *testing.T) {
	isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	SetClientIDFromMCP("managed-client")
	SetClientID("explicit-client")
	SetClientSecret("explicit-secret")

	pair, err := resolveOAuthCredentialPair(t.TempDir())
	if err != nil || pair == nil || pair.ClientID != "explicit-client" || pair.ClientSecret != "explicit-secret" {
		t.Fatalf("explicit pair after MCP marker = %#v, %v", pair, err)
	}
	if IsClientIDFromMCP() {
		t.Fatal("explicit runtime Client ID retained the stale MCP marker")
	}
}

func TestCrossPlatformCoverageMCPRuntimeTupleCannotPolluteLaterAppConfigPair(t *testing.T) {
	isolateAppCredentialKeychain(t)
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	dir := t.TempDir()
	writeCredentialConfig(t, dir, "config-client", PlainSecret("config-secret"))

	SetClientCredentials("previous-direct-client", "previous-direct-secret")
	SetClientIDFromMCP("managed-client")
	if _, secret := getRuntimeCredentials(); secret != "" {
		t.Fatal("MCP runtime Client ID retained a previous direct-mode Client Secret")
	}

	pair, err := resolveOAuthCredentialPair(dir)
	if err != nil || pair == nil || pair.ClientID != "config-client" || pair.ClientSecret != "config-secret" {
		t.Fatalf("app config pair after MCP = %#v, %v", pair, err)
	}
	if id, secret := getRuntimeCredentials(); id != "" || secret != "" || IsClientIDFromMCP() {
		t.Fatalf("stale MCP runtime tuple remains: id=%q has_secret=%t from_mcp=%t", id, secret != "", IsClientIDFromMCP())
	}

	oauthProvider := NewOAuthProvider(dir, nil)
	deviceProvider := NewDeviceFlowProvider(dir, nil)
	if oauthProvider.credentials == nil || oauthProvider.credentials.ClientID != "config-client" {
		t.Fatalf("second OAuth provider credentials = %#v", oauthProvider.credentials)
	}
	if deviceProvider.credentials == nil || deviceProvider.credentials.ClientID != "config-client" {
		t.Fatalf("device provider credentials = %#v", deviceProvider.credentials)
	}
}

func TestCrossPlatformCoverageDeleteAppConfigSweepsAllApplicationCredentialNamespaces(t *testing.T) {
	oldCleanup := appConfigRemoveCredentialEntries
	var gotService string
	var gotPrefixes []string
	appConfigRemoveCredentialEntries = func(service string, prefixes ...string) error {
		gotService = service
		gotPrefixes = append([]string(nil), prefixes...)
		return nil
	}
	t.Cleanup(func() { appConfigRemoveCredentialEntries = oldCleanup })
	if err := DeleteAppConfig(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if gotService != keychain.Service {
		t.Fatalf("cleanup service = %q", gotService)
	}
	joined := strings.Join(gotPrefixes, ",")
	for _, prefix := range []string{secretKeyPrefix, clientSecretPrefix, appTokenPrefix} {
		if !strings.Contains(joined, prefix) {
			t.Fatalf("cleanup prefixes = %q; missing %q", joined, prefix)
		}
	}
}

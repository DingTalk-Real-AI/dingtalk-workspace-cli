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
	"testing"
	"time"
)

// resetStrictResolverState clears caches the strict resolver shares with
// the existing legacy resolver. Tests must call this between scenarios
// because GetCachedAppConfig and the resolved-credential cache outlive
// individual t.TempDir setups.
func resetStrictResolverState(t *testing.T) {
	t.Helper()
	cachedAppConfigMu.Lock()
	cachedAppConfig = nil
	cachedAppConfigMu.Unlock()
	cachedResolvedMu.Lock()
	cachedResolvedValid = false
	cachedResolvedID = ""
	cachedResolvedSecret = ""
	cachedResolvedMu.Unlock()
}

// writeAppConfig drops a config JSON into dir. clientSecret == "" produces
// the legacy "no SecretInput field" shape (treated as empty).
func writeAppConfig(t *testing.T, dir, clientID, clientSecret string) {
	t.Helper()
	cfg := AppConfig{
		ClientID:  clientID,
		CreatedAt: time.Now(),
	}
	if clientSecret != "" {
		cfg.ClientSecret = PlainSecret(clientSecret)
	}
	path := GetAppConfigPath(dir)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func unsetEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")
	_ = os.Unsetenv(EnvClientID)
	_ = os.Unsetenv(EnvClientSecret)
}

func TestResolveStrict_AppConfigMissing(t *testing.T) {
	resetStrictResolverState(t)
	unsetEnv(t)
	dir := t.TempDir()

	_, _, _, _, err := ResolveAppCredentialsStrict(dir)
	if !errors.Is(err, ErrAppConfigMissing) {
		t.Fatalf("err = %v, want ErrAppConfigMissing", err)
	}
}

func TestResolveStrict_ClientIDEmpty(t *testing.T) {
	resetStrictResolverState(t)
	unsetEnv(t)
	dir := t.TempDir()
	writeAppConfig(t, dir, "", "some-secret")

	_, _, _, _, err := ResolveAppCredentialsStrict(dir)
	if !errors.Is(err, ErrClientIDEmpty) {
		t.Fatalf("err = %v, want ErrClientIDEmpty", err)
	}
}

func TestResolveStrict_ClientSecretEmpty(t *testing.T) {
	resetStrictResolverState(t)
	unsetEnv(t)
	dir := t.TempDir()
	writeAppConfig(t, dir, "ding_abc", "")

	_, _, _, _, err := ResolveAppCredentialsStrict(dir)
	if !errors.Is(err, ErrClientSecretEmpty) {
		t.Fatalf("err = %v, want ErrClientSecretEmpty", err)
	}
}

func TestResolveStrict_PlainConfigSuccess(t *testing.T) {
	resetStrictResolverState(t)
	unsetEnv(t)
	dir := t.TempDir()
	writeAppConfig(t, dir, "ding_abc", "supersecret123")

	id, secret, idSrc, secretSrc, err := ResolveAppCredentialsStrict(dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id != "ding_abc" {
		t.Errorf("id = %q", id)
	}
	if secret != "supersecret123" {
		t.Errorf("secret = %q", secret)
	}
	if idSrc != CredentialSourceAppConfig {
		t.Errorf("idSrc = %s, want app_config", idSrc)
	}
	if secretSrc != CredentialSourcePlainConfig {
		t.Errorf("secretSrc = %s, want plain_config (PlainSecret was used)", secretSrc)
	}
}

func TestResolveStrict_SecretRefFileSuccess(t *testing.T) {
	resetStrictResolverState(t)
	unsetEnv(t)
	dir := t.TempDir()
	// Write secret file
	secretPath := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("via-file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Write app config with file SecretRef
	cfg := AppConfig{
		ClientID: "ding_abc",
		ClientSecret: SecretInput{
			Ref: &SecretRef{Source: "file", ID: secretPath},
		},
		CreatedAt: time.Now(),
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(GetAppConfigPath(dir), b, 0o600); err != nil {
		t.Fatal(err)
	}

	id, secret, idSrc, secretSrc, err := ResolveAppCredentialsStrict(dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id != "ding_abc" || secret != "via-file-secret" {
		t.Errorf("id/secret = %q/%q", id, secret)
	}
	if idSrc != CredentialSourceAppConfig {
		t.Errorf("idSrc = %s", idSrc)
	}
	// File-backed secrets are reported as plain_config (not in keychain).
	if secretSrc != CredentialSourcePlainConfig {
		t.Errorf("secretSrc = %s, want plain_config for file-backed secret", secretSrc)
	}
}

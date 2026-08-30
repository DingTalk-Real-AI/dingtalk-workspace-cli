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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

var appCredentialsBeforeMigrationLock = func() {}

// AppCredentialPair is an atomic application credential. ClientID and
// ClientSecret are always selected from the same source.
type AppCredentialPair struct {
	ClientID     string
	ClientSecret string
	Source       string // flag | env | app_config
}

const CredentialSourceFlag CredentialSource = "flag"

var (
	ErrFlagCredentialPairIncomplete = errors.New("--client-id and --client-secret must be provided together")
	ErrEnvCredentialPairIncomplete  = errors.New("DWS_CLIENT_ID and DWS_CLIENT_SECRET must be set together")
	ErrClientSecretConflict         = errors.New("canonical and legacy Client Secret slots conflict; log in again")
	ErrClientSecretRefMismatch      = errors.New("app config Client Secret reference does not match Client ID")
	ErrCredentialPlaceholders       = errors.New("credentials contain placeholders")
)

// ResolveAppCredentialPair resolves one complete pair without mixing sources.
// A caller-provided temporary App Token must be handled before calling this
// function so it never touches env, app config, or keychain.
func ResolveAppCredentialPair(configDir, flagClientID, flagClientSecret string) (AppCredentialPair, error) {
	if pair, selected, err := credentialPairFromValues(flagClientID, flagClientSecret, string(CredentialSourceFlag), ErrFlagCredentialPairIncomplete); selected || err != nil {
		return pair, err
	}
	if pair, selected, err := credentialPairFromValues(os.Getenv(EnvClientID), os.Getenv(EnvClientSecret), string(CredentialSourceEnv), ErrEnvCredentialPairIncomplete); selected || err != nil {
		return pair, err
	}
	return ResolveAppConfigCredentialPair(configDir)
}

// ResolveAppConfigCredentialPair resolves only app.json and its corresponding
// secret. It is exported so OAuth can distinguish "no custom application" from
// an explicit, damaged custom-application configuration before falling back to
// managed MCP credentials.
func ResolveAppConfigCredentialPair(configDir string) (AppCredentialPair, error) {
	return resolveAppConfigCredentialPair(configDir, true)
}

func resolveAppConfigCredentialPair(configDir string, migrate bool) (AppCredentialPair, error) {
	id, secret, _, _, err := resolveAppConfigCredentialsMode(configDir, migrate)
	if err != nil {
		return AppCredentialPair{}, err
	}
	return validateAppCredentialPair(id, secret, string(CredentialSourceAppConfig))
}

func resolveAppCredentialPairWithoutMigration(configDir, flagClientID, flagClientSecret string) (AppCredentialPair, error) {
	if pair, selected, err := credentialPairFromValues(flagClientID, flagClientSecret, string(CredentialSourceFlag), ErrFlagCredentialPairIncomplete); selected || err != nil {
		return pair, err
	}
	if pair, selected, err := credentialPairFromValues(os.Getenv(EnvClientID), os.Getenv(EnvClientSecret), string(CredentialSourceEnv), ErrEnvCredentialPairIncomplete); selected || err != nil {
		return pair, err
	}
	return resolveAppConfigCredentialPair(configDir, false)
}

func credentialPairFromValues(clientID, clientSecret, source string, incompleteErr error) (AppCredentialPair, bool, error) {
	idSet := strings.TrimSpace(clientID) != ""
	secretSet := strings.TrimSpace(clientSecret) != ""
	if idSet != secretSet {
		return AppCredentialPair{}, false, incompleteErr
	}
	if !idSet {
		return AppCredentialPair{}, false, nil
	}
	pair, err := validateAppCredentialPair(clientID, clientSecret, source)
	return pair, true, err
}

func validateAppCredentialPair(clientID, clientSecret, source string) (AppCredentialPair, error) {
	id := strings.TrimSpace(clientID)
	secret := strings.TrimSpace(clientSecret)
	if id == "" || secret == "" || strings.HasPrefix(id, "<") || strings.HasPrefix(secret, "<") {
		return AppCredentialPair{}, fmt.Errorf("%s credentials are incomplete: %w", source, ErrCredentialPlaceholders)
	}
	return AppCredentialPair{ClientID: id, ClientSecret: secret, Source: source}, nil
}

func resolveAppConfigCredentials(configDir string) (
	clientID, secret string,
	clientIDSource, secretSource CredentialSource,
	err error,
) {
	return resolveAppConfigCredentialsMode(configDir, true)
}

func resolveAppConfigCredentialsMode(configDir string, migrate bool) (
	clientID, secret string,
	clientIDSource, secretSource CredentialSource,
	err error,
) {
	cfg, err := LoadAppConfig(configDir)
	if err != nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, fmt.Errorf("load app config: %w", err)
	}
	if cfg == nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrAppConfigMissing
	}
	clientID = strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientIDEmpty
	}
	clientIDSource = CredentialSourceAppConfig

	// An explicit value is authoritative. If it is damaged, do not silently
	// fall back to a derived keychain slot and hide the broken app.json.
	if !cfg.ClientSecret.IsZero() {
		if cfg.ClientSecret.Ref != nil && cfg.ClientSecret.Ref.Source == "keychain" &&
			!isCanonicalClientSecretRef(cfg.ClientSecret, clientID) &&
			!isLegacyClientSecretRef(cfg.ClientSecret, clientID) {
			return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientSecretRefMismatch
		}
		wasPlain := cfg.ClientSecret.IsPlain()
		resolved, resolveErr := ResolveSecret(cfg.ClientSecret)
		if resolveErr != nil {
			return "", "", CredentialSourceUnknown, CredentialSourceUnknown, fmt.Errorf("%w: explicit app config secret cannot be resolved", ErrSecretResolve)
		}
		resolved = strings.TrimSpace(resolved)
		if resolved == "" {
			return "", "", clientIDSource, CredentialSourceUnknown, ErrClientSecretEmpty
		}

		secretSource = CredentialSourcePlainConfig
		if cfg.ClientSecret.Ref != nil && cfg.ClientSecret.Ref.Source == "keychain" {
			secretSource = CredentialSourceKeychain
		}
		switch {
		case wasPlain:
			// Plaintext remains usable if keychain is unavailable; migration is
			// best-effort. When both historical slots can be read, still refuse an
			// existing disagreement before replacing either value.
			canonical, canonicalErr, legacy, legacyErr := readDerivedSecretSlots(clientID)
			if canonicalErr == nil && legacyErr == nil && canonical != "" && legacy != "" && canonical != legacy {
				return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientSecretConflict
			}
			if migrate {
				migrateAppConfigSecret(configDir, cfg, resolved)
			}
		case isCanonicalClientSecretRef(cfg.ClientSecret, clientID):
			legacy, legacyErr := authKeychainGet(keychain.Service, legacyClientSecretAccountKey(clientID))
			if legacyErr != nil {
				return "", "", CredentialSourceUnknown, CredentialSourceUnknown, fmt.Errorf("%w: legacy secret slot unavailable", ErrSecretResolve)
			}
			if legacy != "" && legacy != resolved {
				return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientSecretConflict
			}
			if legacy != "" && migrate {
				migrateAppConfigSecret(configDir, cfg, resolved)
			}
		case isLegacyClientSecretRef(cfg.ClientSecret, clientID):
			canonical, canonicalErr := secretKeychainGet(keychain.Service, secretAccountKey(clientID))
			if canonicalErr != nil {
				return "", "", CredentialSourceUnknown, CredentialSourceUnknown, fmt.Errorf("%w: canonical secret slot unavailable", ErrSecretResolve)
			}
			if canonical != "" && canonical != resolved {
				return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientSecretConflict
			}
			if migrate {
				migrateAppConfigSecret(configDir, cfg, resolved)
			}
		}
		return clientID, resolved, clientIDSource, secretSource, nil
	}

	canonical, canonicalErr, legacy, legacyErr := readDerivedSecretSlots(clientID)
	if canonicalErr != nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, fmt.Errorf("%w: canonical secret slot unavailable", ErrSecretResolve)
	}
	if legacyErr != nil {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, fmt.Errorf("%w: legacy secret slot unavailable", ErrSecretResolve)
	}
	if canonical != "" && legacy != "" && canonical != legacy {
		return "", "", CredentialSourceUnknown, CredentialSourceUnknown, ErrClientSecretConflict
	}

	switch {
	case canonical != "":
		secret = canonical
		if migrate {
			migrateAppConfigSecret(configDir, cfg, canonical)
		}
	case legacy != "":
		secret = legacy
		if migrate {
			migrateAppConfigSecret(configDir, cfg, legacy)
		}
	default:
		return "", "", clientIDSource, CredentialSourceUnknown, ErrClientSecretEmpty
	}
	return clientID, secret, clientIDSource, CredentialSourceKeychain, nil
}

func readDerivedSecretSlots(clientID string) (canonical string, canonicalErr error, legacy string, legacyErr error) {
	canonical, canonicalErr = secretKeychainGet(keychain.Service, secretAccountKey(clientID))
	legacy, legacyErr = authKeychainGet(keychain.Service, legacyClientSecretAccountKey(clientID))
	return canonical, canonicalErr, legacy, legacyErr
}

func isCanonicalClientSecretRef(input SecretInput, clientID string) bool {
	return input.Ref != nil && input.Ref.Source == "keychain" && input.Ref.ID == secretAccountKey(clientID)
}

func isLegacyClientSecretRef(input SecretInput, clientID string) bool {
	return input.Ref != nil && input.Ref.Source == "keychain" && input.Ref.ID == legacyClientSecretAccountKey(clientID)
}

// migrateAppConfigSecret deliberately keeps a resolved credential usable when
// best-effort cleanup fails. The warning contains identifiers and error causes,
// never the secret value.
func migrateAppConfigSecret(configDir string, cfg *AppConfig, secret string) {
	if cfg == nil || strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(secret) == "" {
		return
	}
	appCredentialsBeforeMigrationLock()
	lock, err := appConfigAcquireDualLock(context.Background(), configDir)
	if err != nil {
		slog.Warn("auth: failed to lock Client Secret migration", "client_id", cfg.ClientID, "error", err)
		return
	}
	defer lock.Release()

	// Another login may have replaced app.json while this resolver waited for
	// the lock. Re-resolve under the lock and never write an observed old secret
	// over a newer complete pair.
	current, loadErr := LoadAppConfig(configDir)
	if loadErr != nil || current == nil || strings.TrimSpace(current.ClientID) != strings.TrimSpace(cfg.ClientID) {
		if loadErr != nil {
			slog.Warn("auth: failed to re-read app config before Client Secret migration", "client_id", cfg.ClientID, "error", loadErr)
		}
		return
	}
	currentSecret, resolveErr := resolveConfigSecretWithoutMigration(current)
	if resolveErr != nil || currentSecret != secret {
		if resolveErr != nil {
			slog.Warn("auth: failed to revalidate Client Secret migration", "client_id", cfg.ClientID, "error", resolveErr)
		}
		return
	}
	if err := secretKeychainSet(keychain.Service, secretAccountKey(cfg.ClientID), secret); err != nil {
		slog.Warn("auth: failed to migrate Client Secret to canonical slot", "client_id", cfg.ClientID, "error", err)
		return
	}
	updated := *current
	updated.ClientSecret = SecretInput{Ref: &SecretRef{Source: "keychain", ID: secretAccountKey(cfg.ClientID)}}
	if err := saveAppConfigLocked(configDir, &updated); err != nil {
		slog.Warn("auth: failed to update app config after Client Secret migration", "client_id", cfg.ClientID, "error", err)
	}
}

func resolveConfigSecretWithoutMigration(cfg *AppConfig) (string, error) {
	if cfg == nil || strings.TrimSpace(cfg.ClientID) == "" {
		return "", ErrClientIDEmpty
	}
	clientID := strings.TrimSpace(cfg.ClientID)
	if !cfg.ClientSecret.IsZero() {
		if cfg.ClientSecret.Ref != nil && cfg.ClientSecret.Ref.Source == "keychain" &&
			!isCanonicalClientSecretRef(cfg.ClientSecret, clientID) &&
			!isLegacyClientSecretRef(cfg.ClientSecret, clientID) {
			return "", ErrClientSecretRefMismatch
		}
		secret, err := ResolveSecret(cfg.ClientSecret)
		if err != nil {
			return "", fmt.Errorf("%w: explicit app config secret cannot be resolved", ErrSecretResolve)
		}
		return secret, nil
	}
	canonical, canonicalErr, legacy, legacyErr := readDerivedSecretSlots(clientID)
	if canonicalErr != nil || legacyErr != nil {
		return "", ErrSecretResolve
	}
	if canonical != "" && legacy != "" && canonical != legacy {
		return "", ErrClientSecretConflict
	}
	if canonical != "" {
		return canonical, nil
	}
	return legacy, nil
}

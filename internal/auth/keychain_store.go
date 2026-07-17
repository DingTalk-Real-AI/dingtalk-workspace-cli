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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var (
	migrationOnce         sync.Once
	migrationDone         bool
	authKeychainMarshal   = json.MarshalIndent
	authKeychainUnmarshal = json.Unmarshal
	authKeychainSet       = keychain.Set
	authKeychainGet       = keychain.Get
	authKeychainRemove    = keychain.Remove
	authKeychainExists    = keychain.Exists
	authKeychainMigrate   = keychain.MigrateFromLegacy
	authValidateEntries   = keychain.ValidateAuthTokenEntries
)

// ErrTokenDataNotFound means the requested keychain slot does not exist.
var ErrTokenDataNotFound = errors.New("token data not found")

// SaveTokenDataKeychain saves TokenData to the platform keychain.
// This is the new secure storage method using random master key.
func SaveTokenDataKeychain(data *TokenData) error {
	return saveTokenDataKeychainAccount(keychain.AccountToken, data)
}

// TokenAccountForCorpID returns the keychain account used for a corp-bound token.
func TokenAccountForCorpID(corpID string) string {
	return keychain.AccountToken + ":" + strings.TrimSpace(corpID)
}

// TokenAccountForIdentity returns the stable keychain account used for one
// DingTalk identity. The hash avoids collisions caused by delimiter escaping
// or keychain/file-name restrictions.
func TokenAccountForIdentity(corpID, userID string) string {
	identity := strings.TrimSpace(corpID) + "\x00" + strings.TrimSpace(userID)
	return fmt.Sprintf("%s:id:%x", keychain.AccountToken, sha256.Sum256([]byte(identity)))
}

// SaveTokenDataKeychainForCorpID saves TokenData to a corp-scoped keychain slot.
func SaveTokenDataKeychainForCorpID(corpID string, data *TokenData) error {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return fmt.Errorf("corpId is required for profile token storage")
	}
	return saveTokenDataKeychainAccount(TokenAccountForCorpID(corpID), data)
}

// SaveTokenDataKeychainForIdentity saves TokenData to an identity-scoped slot.
func SaveTokenDataKeychainForIdentity(corpID, userID string, data *TokenData) error {
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	if corpID == "" || userID == "" {
		return fmt.Errorf("corpId and userId are required for identity token storage")
	}
	return saveTokenDataKeychainAccount(TokenAccountForIdentity(corpID, userID), data)
}

func saveTokenDataKeychainAccount(account string, data *TokenData) error {
	jsonData, err := authKeychainMarshal(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token data: %w", err)
	}
	// Zero sensitive data after use
	defer func() {
		for i := range jsonData {
			jsonData[i] = 0
		}
	}()

	if err := authKeychainSet(keychain.Service, account, string(jsonData)); err != nil {
		return fmt.Errorf("save to keychain: %w", err)
	}
	return nil
}

// LoadTokenDataKeychain loads TokenData from the platform keychain.
func LoadTokenDataKeychain() (*TokenData, error) {
	return loadTokenDataKeychainAccount(keychain.AccountToken)
}

// LoadTokenDataKeychainForCorpID loads TokenData from a corp-scoped keychain slot.
func LoadTokenDataKeychainForCorpID(corpID string) (*TokenData, error) {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return nil, fmt.Errorf("corpId is required for profile token storage")
	}
	return loadTokenDataKeychainAccount(TokenAccountForCorpID(corpID))
}

// LoadTokenDataKeychainForIdentity loads TokenData from an identity-scoped slot.
func LoadTokenDataKeychainForIdentity(corpID, userID string) (*TokenData, error) {
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	if corpID == "" || userID == "" {
		return nil, fmt.Errorf("corpId and userId are required for identity token storage")
	}
	return loadTokenDataKeychainAccount(TokenAccountForIdentity(corpID, userID))
}

func loadTokenDataKeychainAccount(account string) (*TokenData, error) {
	jsonStr, err := authKeychainGet(keychain.Service, account)
	if err != nil {
		return nil, fmt.Errorf("load from keychain: %w", err)
	}
	if jsonStr == "" {
		return nil, fmt.Errorf("%w in keychain account %q", ErrTokenDataNotFound, account)
	}

	var data TokenData
	if err := authKeychainUnmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("parse token data: %w", err)
	}
	return &data, nil
}

// preflightTokenPersistence verifies that every registered token slot can be
// read before an OAuth login or exchange can target any profile.
// A missing slot is safe (first login or a legacy fallback); any other error
// stops the remote operation when existing ciphertext is already known to be
// unreadable and therefore unsafe to update.
func preflightTokenPersistence(configDir string) error {
	if h := edition.Get(); h.SaveToken != nil {
		return nil
	}

	if _, err := LoadTokenDataKeychain(); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
		return fmt.Errorf("legacy token slot %q is unreadable: %w", keychain.AccountToken, err)
	}

	cfg, err := LoadProfiles(configDir)
	if err != nil {
		return fmt.Errorf("load token profiles: %w", err)
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return err
	}
	for _, profile := range cfg.Profiles {
		// LoadProfiles normalizes away blank and duplicate corp IDs.
		corpID := profile.CorpID
		if _, err := LoadTokenDataKeychainForCorpID(corpID); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf(
				"profile token slot %q is unreadable; on macOS first try `env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run`; if the ciphertext is damaged, remove only this profile with `dws auth logout --profile %q`, or use `dws auth reset` only when discarding all local profiles: %w",
				TokenAccountForCorpID(corpID), corpID, err,
			)
		}
	}
	for _, profile := range cfg.Profiles {
		corpID := strings.TrimSpace(profile.CorpID)
		userID := strings.TrimSpace(profile.UserID)
		if corpID == "" || userID == "" {
			continue
		}
		if _, err := LoadTokenDataKeychainForIdentity(corpID, userID); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf(
				"identity token slot %q is unreadable; remove only this account with `dws auth logout --profile %q`, or use `dws auth reset` only when discarding all local profiles: %w",
				TokenAccountForIdentity(corpID, userID), ProfileSelector(profile), err,
			)
		}
	}
	if err := authValidateEntries(keychain.Service); err != nil {
		return fmt.Errorf(
			"auth token ciphertext inventory is unreadable; on macOS first try `env -u DWS_DISABLE_KEYCHAIN dws auth migrate-keychain --to file-dek --dry-run`; if the ciphertext is damaged, use `dws auth reset` only when discarding all local profiles: %w",
			err,
		)
	}
	return nil
}

// preflightTokenRefreshPersistence checks only the slots a refresh can write.
// An unrelated broken profile must not prevent the current profile from using
// its still-valid credentials.
func preflightTokenRefreshPersistence(configDir string, data *TokenData) error {
	if h := edition.Get(); h.SaveToken != nil {
		return nil
	}
	cfg, err := LoadProfiles(configDir)
	if err != nil {
		return err
	}
	if err := ensureProfilesWritable(cfg); err != nil {
		return err
	}

	if _, err := LoadTokenDataKeychain(); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
		return fmt.Errorf("legacy token slot %q is unreadable: %w", keychain.AccountToken, err)
	}
	if data == nil || strings.TrimSpace(data.CorpID) == "" {
		return nil
	}
	corpID := strings.TrimSpace(data.CorpID)
	userID := strings.TrimSpace(data.UserID)
	if userID != "" {
		if _, err := LoadTokenDataKeychainForIdentity(corpID, userID); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf("identity token slot %q is unreadable: %w", TokenAccountForIdentity(corpID, userID), err)
		}
	}
	checkOrganizationMirror := true
	if _, _, exact := ParseIdentitySelector(RuntimeProfile()); exact {
		checkOrganizationMirror =
			exactProfileSelectorForCorp(cfg, corpID, cfg.OrgCurrentProfiles[corpID]) ==
				profileSelector(corpID, userID)
	}
	if checkOrganizationMirror {
		if _, err := LoadTokenDataKeychainForCorpID(corpID); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf("profile token slot %q is unreadable: %w", TokenAccountForCorpID(corpID), err)
		}
	}
	return nil
}

// DeleteTokenDataKeychain removes TokenData from the platform keychain.
func DeleteTokenDataKeychain() error {
	return authKeychainRemove(keychain.Service, keychain.AccountToken)
}

// DeleteTokenDataKeychainForCorpID removes TokenData from a corp-scoped keychain slot.
func DeleteTokenDataKeychainForCorpID(corpID string) error {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return fmt.Errorf("corpId is required for profile token storage")
	}
	return authKeychainRemove(keychain.Service, TokenAccountForCorpID(corpID))
}

// DeleteTokenDataKeychainForIdentity removes one identity-scoped token.
func DeleteTokenDataKeychainForIdentity(corpID, userID string) error {
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	if corpID == "" || userID == "" {
		return fmt.Errorf("corpId and userId are required for identity token storage")
	}
	return authKeychainRemove(keychain.Service, TokenAccountForIdentity(corpID, userID))
}

// TokenDataExistsKeychain checks if token data exists in keychain.
func TokenDataExistsKeychain() bool {
	return authKeychainExists(keychain.Service, keychain.AccountToken)
}

// TokenDataExistsKeychainForCorpID checks if a corp-scoped token exists.
func TokenDataExistsKeychainForCorpID(corpID string) bool {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return false
	}
	return authKeychainExists(keychain.Service, TokenAccountForCorpID(corpID))
}

// TokenDataExistsKeychainForIdentity checks if an identity-scoped token exists.
func TokenDataExistsKeychainForIdentity(corpID, userID string) bool {
	corpID = strings.TrimSpace(corpID)
	userID = strings.TrimSpace(userID)
	if corpID == "" || userID == "" {
		return false
	}
	return authKeychainExists(keychain.Service, TokenAccountForIdentity(corpID, userID))
}

// EnsureMigration performs one-time migration from legacy .data to keychain.
// This should be called early in the auth flow (e.g., during GetAccessToken).
// The migration is idempotent and thread-safe.
func EnsureMigration(configDir string, logger *slog.Logger) {
	migrationOnce.Do(func() {
		result := authKeychainMigrate(configDir)
		migrationDone = true

		if result.Migrated {
			if logger != nil {
				logger.Info("migrated token data to secure keychain storage",
					"from", result.FromPath,
					"backup", result.BackupPath)
			}
		} else if result.NeedRelogin {
			if logger != nil {
				logger.Warn("cannot migrate legacy token data, please re-login",
					"error", result.Error)
			}
		} else if result.Error != nil {
			if logger != nil {
				logger.Error("migration failed", "error", result.Error)
			}
		}
	})
}

// IsMigrationDone returns true if migration has been attempted.
func IsMigrationDone() bool {
	return migrationDone
}

// Client credential storage functions.
// These store the clientSecret associated with a specific clientId,
// allowing token refresh to work even if environment variables change.

const clientSecretPrefix = "client-secret:"

// SaveClientSecret stores the client secret for a specific client ID.
// This is called during login to snapshot the credentials used.
func SaveClientSecret(clientID, clientSecret string) error {
	if clientID == "" || clientSecret == "" {
		return nil // Nothing to save
	}
	account := clientSecretPrefix + clientID
	if err := authKeychainSet(keychain.Service, account, clientSecret); err != nil {
		return fmt.Errorf("save client secret: %w", err)
	}
	return nil
}

// LoadClientSecret retrieves the stored client secret for a specific client ID.
// Returns empty string if not found.
func LoadClientSecret(clientID string) string {
	if clientID == "" {
		return ""
	}
	account := clientSecretPrefix + clientID
	secret, err := authKeychainGet(keychain.Service, account)
	if err != nil {
		return ""
	}
	return secret
}

// DeleteClientSecret removes the stored client secret for a specific client ID.
func DeleteClientSecret(clientID string) error {
	if clientID == "" {
		return nil
	}
	account := clientSecretPrefix + clientID
	return authKeychainRemove(keychain.Service, account)
}

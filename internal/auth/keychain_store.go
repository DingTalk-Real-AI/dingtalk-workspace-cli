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

// prepareLoginPersistence rejects profile registries written by a newer client
// and protects the legacy global mirror before a new authorization flow
// performs remote work. Version-1 registries keep using the existing full
// migration in saveTokenDataLocked before any compatibility mirror is
// overwritten.
//
// For v2/v3, only the organization referenced by the readable global mirror is
// inspected. A uniquely matching half-migrated profile is repaired from that
// mirror under the profiles lock. Missing or damaged slots in unrelated
// organizations and orphan inventory are deliberately not scanned.
func prepareLoginPersistence(configDir string) error {
	if h := edition.Get(); h.SaveToken != nil {
		return nil
	}
	return withProfilesLock(configDir, func() error {
		cfg, err := profilesLoad(configDir)
		if err != nil {
			return fmt.Errorf("load token profiles: %w", err)
		}
		if err := ensureProfilesWritable(cfg); err != nil {
			return err
		}
		return repairHalfMigratedGlobalTokenLocked(cfg)
	})
}

// repairHalfMigratedGlobalTokenLocked preserves the only readable copy left by
// an interrupted v1.0.53 migration. The caller must hold the profiles lock.
func repairHalfMigratedGlobalTokenLocked(cfg *ProfilesConfig) error {
	if cfg == nil || cfg.Version < profilesVersion || len(cfg.Profiles) == 0 {
		return nil
	}

	global, err := profilesLoadLegacy()
	if errors.Is(err, ErrTokenDataNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"legacy token slot %q is unreadable; refusing to overwrite a potentially unique old login: %w",
			keychain.AccountToken,
			err,
		)
	}
	if global == nil {
		return nil
	}
	corpID := strings.TrimSpace(global.CorpID)
	if corpID == "" {
		return nil
	}
	profiles := profilesForCorpID(cfg, corpID)
	if len(profiles) == 0 {
		// A readable global token for an unregistered organization is an orphan,
		// not a profile credential that this registry still promises to retain.
		return nil
	}

	orgToken, orgErr := profilesLoadCorp(corpID)
	allCanonical := true
	for _, profile := range profiles {
		if !loginProfileHasUsableCanonicalToken(profile, orgToken, orgErr) {
			allCanonical = false
			break
		}
	}
	if allCanonical {
		return nil
	}

	profile := uniqueV2GlobalRepairProfile(cfg, corpID)
	if profile == nil {
		return fmt.Errorf(
			"legacy token slot %q may be the only recoverable login for one of %d accounts in organization %q; refusing to overwrite it until each account has a usable identity slot",
			keychain.AccountToken,
			len(profiles),
			corpID,
		)
	}
	userID := strings.TrimSpace(profile.UserID)
	if userID != "" &&
		orgErr == nil &&
		loginTokenHasCredentialMaterial(orgToken) &&
		legacyTokenMatchesV2RepairProfile(orgToken, profile) {
		if err := repairLoginIdentityToken(profile, orgToken); err != nil {
			return err
		}
		return nil
	}
	if !legacyTokenMatchesV2RepairProfile(global, profile) {
		return fmt.Errorf(
			"legacy token slot %q does not safely match the only profile in organization %q; refusing to overwrite a potentially unique old login",
			keychain.AccountToken,
			corpID,
		)
	}
	if !loginTokenHasCredentialMaterial(global) {
		return fmt.Errorf(
			"legacy token slot %q has no recoverable credential material for organization %q; refusing to overwrite a potentially unique old login",
			keychain.AccountToken,
			corpID,
		)
	}

	// The matching global token is the only recoverable copy. Overwrite a
	// damaged organization slot as well as filling a missing one.
	if err := profilesSaveCorp(corpID, global); err != nil {
		return fmt.Errorf("repair organization token slot %q: %w", TokenAccountForCorpID(corpID), err)
	}
	if userID == "" {
		return nil
	}
	return repairLoginIdentityToken(profile, global)
}

func loginProfileHasUsableCanonicalToken(
	profile *Profile,
	orgToken *TokenData,
	orgErr error,
) bool {
	if profile == nil {
		return false
	}
	corpID := strings.TrimSpace(profile.CorpID)
	userID := strings.TrimSpace(profile.UserID)
	if userID == "" {
		return orgErr == nil &&
			loginTokenHasCredentialMaterial(orgToken) &&
			strings.TrimSpace(orgToken.CorpID) == corpID &&
			strings.TrimSpace(orgToken.UserID) == ""
	}
	identity, err := profilesLoadIdentity(corpID, userID)
	if err == nil &&
		loginTokenHasCredentialMaterial(identity) &&
		strings.TrimSpace(identity.CorpID) == corpID &&
		strings.TrimSpace(identity.UserID) == userID {
		return true
	}
	return false
}

func loginTokenHasCredentialMaterial(data *TokenData) bool {
	return data != nil &&
		(strings.TrimSpace(data.AccessToken) != "" ||
			strings.TrimSpace(data.RefreshToken) != "" ||
			strings.TrimSpace(data.PersistentCode) != "")
}

func repairLoginIdentityToken(profile *Profile, source *TokenData) error {
	corpID := strings.TrimSpace(profile.CorpID)
	userID := strings.TrimSpace(profile.UserID)
	identityToken := source
	if strings.TrimSpace(source.UserID) == "" {
		enriched := *source
		enriched.UserID = userID
		if strings.TrimSpace(enriched.UserName) == "" {
			enriched.UserName = strings.TrimSpace(profile.UserName)
		}
		identityToken = &enriched
	}
	if err := profilesSaveIdentity(corpID, userID, identityToken); err != nil {
		return fmt.Errorf("repair identity token slot %q: %w", TokenAccountForIdentity(corpID, userID), err)
	}
	return nil
}

// preflightTokenPersistence verifies every persisted token slot, including
// unregistered/orphan ciphertext. Keep this full-inventory validator for
// migration, export and explicit storage diagnostics; login must use the
// schema-only and target-only preflights instead so an unrelated damaged
// account cannot block reauthorization.
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

// preflightTokenWritePersistence checks only the slots SaveTokenData can write
// for data under the current runtime selector. It is shared by login and
// refresh so both paths stay aligned with the same identity/org/global mirror
// isolation rules.
func preflightTokenWritePersistence(configDir string, data *TokenData) error {
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

	plan := planTokenPersistenceWrites(cfg, data, RuntimeProfile())
	if err := validateTokenPersistenceWritePlan(cfg, data, plan); err != nil {
		return err
	}
	if plan.WriteGlobal {
		if _, err := LoadTokenDataKeychain(); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf("legacy token slot %q is unreadable: %w", keychain.AccountToken, err)
		}
	}
	if plan.CorpID == "" {
		return nil
	}
	if plan.WriteIdentity {
		if _, err := LoadTokenDataKeychainForIdentity(plan.CorpID, plan.UserID); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf(
				"identity token slot %q is unreadable: %w",
				TokenAccountForIdentity(plan.CorpID, plan.UserID),
				err,
			)
		}
	}
	if plan.WriteOrganization {
		if _, err := LoadTokenDataKeychainForCorpID(plan.CorpID); err != nil && !errors.Is(err, ErrTokenDataNotFound) {
			return fmt.Errorf(
				"profile token slot %q is unreadable: %w",
				TokenAccountForCorpID(plan.CorpID),
				err,
			)
		}
	}
	return nil
}

// preflightTokenRefreshPersistence checks only the slots a refresh can write.
// An unrelated broken profile must not prevent the current profile from using
// its still-valid credentials.
func preflightTokenRefreshPersistence(configDir string, data *TokenData) error {
	return preflightTokenWritePersistence(configDir, data)
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

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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

var (
	migrationOnce sync.Once
	migrationDone bool
)

// SaveTokenDataKeychain saves TokenData to the platform keychain.
// This is the new secure storage method using random master key.
func SaveTokenDataKeychain(configDir string, data *TokenData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token data: %w", err)
	}
	// Zero sensitive data after use
	defer func() {
		for i := range jsonData {
			jsonData[i] = 0
		}
	}()

	if err := keychain.Set(keychain.Service, tokenAccount(configDir), string(jsonData)); err != nil {
		return fmt.Errorf("save to keychain: %w", err)
	}
	return nil
}

// LoadTokenDataKeychain loads TokenData from the platform keychain.
func LoadTokenDataKeychain(configDir string) (*TokenData, error) {
	return loadTokenDataFromKeychainAccount(tokenAccount(configDir))
}

func loadTokenDataFromKeychainAccount(account string) (*TokenData, error) {
	jsonStr, err := keychain.Get(keychain.Service, account)
	if err != nil {
		return nil, fmt.Errorf("load from keychain: %w", err)
	}
	if jsonStr == "" {
		return nil, fmt.Errorf("no token data in keychain")
	}

	var data TokenData
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("parse token data: %w", err)
	}
	return &data, nil
}

// DeleteTokenDataKeychain removes TokenData from the platform keychain.
func DeleteTokenDataKeychain(configDir string) error {
	return keychain.Remove(keychain.Service, tokenAccount(configDir))
}

// TokenDataExistsKeychain checks if token data exists in keychain.
func TokenDataExistsKeychain(configDir string) bool {
	return keychain.Exists(keychain.Service, tokenAccount(configDir))
}

func LoadLegacyTokenDataKeychain() (*TokenData, error) {
	return loadTokenDataFromKeychainAccount(keychain.AccountToken)
}

func DeleteLegacyTokenDataKeychain() error {
	return keychain.Remove(keychain.Service, keychain.AccountToken)
}

func LegacyTokenDataExistsKeychain() bool {
	return keychain.Exists(keychain.Service, keychain.AccountToken)
}

func tokenAccount(configDir string) string {
	normalized := strings.TrimSpace(configDir)
	if normalized == "" {
		return keychain.AccountToken
	}
	if abs, err := filepath.Abs(normalized); err == nil {
		normalized = abs
	}
	normalized = filepath.Clean(normalized)
	sum := sha256.Sum256([]byte(normalized))
	return keychain.AccountToken + ":" + hex.EncodeToString(sum[:16])
}

// EnsureMigration performs one-time migration from legacy .data to keychain.
// This should be called early in the auth flow (e.g., during GetAccessToken).
// The migration is idempotent and thread-safe.
func EnsureMigration(configDir string, logger *slog.Logger) {
	migrationOnce.Do(func() {
		result := keychain.MigrateFromLegacy(configDir)
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

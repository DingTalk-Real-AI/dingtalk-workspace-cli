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

package keychain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/security"
)

const (
	legacyDataFile   = ".data"
	legacyBackupFile = ".data.migrated.bak"
)

// MigrationResult contains the result of a migration attempt.
type MigrationResult struct {
	Migrated    bool   // true if migration was performed
	FromPath    string // source file path
	BackupPath  string // backup file path (if migrated)
	Error       error  // error if migration failed
	NeedRelogin bool   // true if user needs to re-login
}

// MigrateFromLegacy attempts to migrate from the legacy MAC-based encryption
// to the new keychain-based storage. It:
// 1. Checks if legacy .data file exists
// 2. Tries to decrypt with MAC address
// 3. Re-encrypts and stores in keychain
// 4. Backs up the old file
//
// If the keychain already has data, migration is skipped.
// If the legacy file doesn't exist, migration is skipped.
// If decryption fails (wrong MAC/corrupted), returns NeedRelogin=true.
func MigrateFromLegacy(configDir string) *MigrationResult {
	result := &MigrationResult{}

	// Check if keychain already has data - skip migration
	if Exists(Service, AccountToken) {
		return result // Already migrated or fresh install
	}

	// Check if legacy .data file exists
	legacyPath := filepath.Join(configDir, legacyDataFile)
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return result // No legacy data to migrate
	}

	result.FromPath = legacyPath

	// Try to decrypt legacy data using MAC address
	legacyData, err := loadLegacyData(configDir)
	if err != nil {
		// Decryption failed - likely different machine or corrupted
		result.Error = fmt.Errorf("cannot decrypt legacy data: %w", err)
		result.NeedRelogin = true
		return result
	}

	// Store in new keychain
	jsonData, err := json.Marshal(legacyData)
	if err != nil {
		result.Error = fmt.Errorf("marshal token data: %w", err)
		return result
	}

	if err := Set(Service, AccountToken, string(jsonData)); err != nil {
		result.Error = fmt.Errorf("store in keychain: %w", err)
		return result
	}

	// Backup old file instead of deleting
	backupPath := filepath.Join(configDir, legacyBackupFile)
	if err := os.Rename(legacyPath, backupPath); err != nil {
		// Non-fatal - data is already migrated
		_ = os.Remove(legacyPath)
	} else {
		result.BackupPath = backupPath
	}

	result.Migrated = true
	return result
}

// loadLegacyData loads and decrypts the legacy .data file using MAC address.
func loadLegacyData(configDir string) (map[string]interface{}, error) {
	// Get MAC address for decryption
	mac, err := security.GetMACAddress()
	if err != nil {
		return nil, fmt.Errorf("get MAC address: %w", err)
	}

	// Read encrypted file
	path := filepath.Join(configDir, legacyDataFile)
	ciphertext, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy file: %w", err)
	}

	// Decrypt using legacy method
	plaintext, err := security.Decrypt(ciphertext, []byte(mac))
	if err != nil {
		return nil, fmt.Errorf("decrypt legacy data: %w", err)
	}

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("parse legacy JSON: %w", err)
	}

	return data, nil
}

// CleanupLegacyBackup removes the backup file created during migration.
// Call this after confirming the new keychain storage works correctly.
func CleanupLegacyBackup(configDir string) error {
	backupPath := filepath.Join(configDir, legacyBackupFile)
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasLegacyData checks if legacy .data file exists.
func HasLegacyData(configDir string) bool {
	legacyPath := filepath.Join(configDir, legacyDataFile)
	_, err := os.Stat(legacyPath)
	return err == nil
}

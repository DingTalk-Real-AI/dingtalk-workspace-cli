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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

const (
	// appConfigFile is the filename for the open-source edition's app
	// credentials store. Sibling editions get a name-suffixed file via
	// editionAppConfigFile() so two dws binaries sharing the same config
	// directory (~/.dws or DWS_CONFIG_DIR) cannot read/write each other's
	// credentials. See GetAppConfigPath for the path derivation contract.
	appConfigFile = "app.json"
)

// editionAppConfigFile returns the app.json filename for the currently-active
// edition. Open-source ("" or "open") keeps the historical "app.json" so
// existing users see no migration; every other edition (e.g. "wukong",
// "dev") gets its own "app-<edition>.json" file. This mirrors the
// pkg/config.EditionPartition strategy used by the disk cache loader and
// gives us end-to-end physical isolation without any read-time heuristics.
func editionAppConfigFile() string {
	name := edition.Get().Name
	if name == "" || name == "open" {
		return appConfigFile
	}
	return "app-" + name + ".json"
}

// AppConfig represents the application credentials configuration.
// This is stored in ~/.dws/app.json with the client secret securely stored in keychain.
type AppConfig struct {
	ClientID     string      `json:"clientId"`
	ClientSecret SecretInput `json:"clientSecret"`
	CreatedAt    time.Time   `json:"createdAt"`
	UpdatedAt    time.Time   `json:"updatedAt,omitempty"`
}

// Cached app config for performance (avoid repeated file reads).
var (
	cachedAppConfig     *AppConfig
	cachedAppConfigOnce sync.Once
	cachedAppConfigMu   sync.RWMutex
)

// Cached resolved credentials (avoid repeated keychain access).
var (
	cachedResolvedID     string
	cachedResolvedSecret string
	cachedResolvedValid  bool
	cachedResolvedMu     sync.RWMutex
)

// GetAppConfigPath returns the path to the app config file for the
// currently-active edition. The filename is partitioned by edition so that
// two dws binaries from different editions sharing the same configDir
// (typically ~/.dws or DWS_CONFIG_DIR) cannot read or overwrite each
// other's credentials. Open-source stays on "app.json" for backwards
// compatibility; sibling editions land on "app-<edition>.json".
func GetAppConfigPath(configDir string) string {
	return filepath.Join(configDir, editionAppConfigFile())
}

// LoadAppConfig loads the app configuration from disk.
// Returns nil, nil if the config file does not exist.
func LoadAppConfig(configDir string) (*AppConfig, error) {
	path := GetAppConfigPath(configDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading app config: %w", err)
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing app config: %w", err)
	}
	return &config, nil
}

// SaveAppConfig saves the app configuration to disk.
// If the client secret is a plain string, it will be stored in keychain
// and the config file will contain a reference to it.
func SaveAppConfig(configDir string, config *AppConfig) error {
	// Store plain secret in keychain, convert to reference
	if config.ClientSecret.IsPlain() && config.ClientID != "" {
		storedRef, err := StoreSecret(config.ClientID, config.ClientSecret)
		if err != nil {
			return fmt.Errorf("storing client secret: %w", err)
		}
		config.ClientSecret = storedRef
	}

	// Update timestamps
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}
	config.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling app config: %w", err)
	}

	path := GetAppConfigPath(configDir)
	if err := helpers.AtomicWriteJSON(path, append(data, '\n')); err != nil {
		return fmt.Errorf("writing app config: %w", err)
	}

	// Update cache
	cachedAppConfigMu.Lock()
	cachedAppConfig = config
	cachedAppConfigMu.Unlock()

	// Invalidate resolved credentials cache so next access re-resolves
	cachedResolvedMu.Lock()
	cachedResolvedValid = false
	cachedResolvedID = ""
	cachedResolvedSecret = ""
	cachedResolvedMu.Unlock()

	return nil
}

// DeleteAppConfig removes the app configuration and associated keychain secrets.
func DeleteAppConfig(configDir string) error {
	// Load existing config to clean up keychain
	existing, _ := LoadAppConfig(configDir)
	if existing != nil {
		RemoveSecretStore(existing.ClientSecret)
	}

	// Remove config file
	path := GetAppConfigPath(configDir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing app config: %w", err)
	}

	// Clear cache
	cachedAppConfigMu.Lock()
	cachedAppConfig = nil
	cachedAppConfigMu.Unlock()

	// Clear resolved credentials cache
	cachedResolvedMu.Lock()
	cachedResolvedValid = false
	cachedResolvedID = ""
	cachedResolvedSecret = ""
	cachedResolvedMu.Unlock()

	return nil
}

// GetCachedAppConfig returns the cached app configuration.
// It loads from disk on first call and caches the result.
// Returns nil if no configuration exists or loading fails.
func GetCachedAppConfig(configDir string) *AppConfig {
	cachedAppConfigOnce.Do(func() {
		cfg, err := LoadAppConfig(configDir)
		if err == nil && cfg != nil {
			cachedAppConfigMu.Lock()
			cachedAppConfig = cfg
			cachedAppConfigMu.Unlock()
		}
	})

	cachedAppConfigMu.RLock()
	defer cachedAppConfigMu.RUnlock()
	return cachedAppConfig
}

// ReloadAppConfig forces a reload of the app configuration from disk.
// This should be called after SaveAppConfig to ensure the cache is updated.
func ReloadAppConfig(configDir string) (*AppConfig, error) {
	cfg, err := LoadAppConfig(configDir)
	if err != nil {
		return nil, err
	}

	cachedAppConfigMu.Lock()
	cachedAppConfig = cfg
	cachedAppConfigMu.Unlock()

	return cfg, nil
}

// HasAppConfig returns true if an app configuration file exists.
func HasAppConfig(configDir string) bool {
	path := GetAppConfigPath(configDir)
	_, err := os.Stat(path)
	return err == nil
}

// ResolveAppCredentials resolves the client ID and secret from the app config.
// Results are cached to avoid repeated keychain access.
// Returns empty strings if the config doesn't exist or resolution fails.
func ResolveAppCredentials(configDir string) (clientID, clientSecret string) {
	// Fast path: check cache first
	cachedResolvedMu.RLock()
	if cachedResolvedValid {
		id, secret := cachedResolvedID, cachedResolvedSecret
		cachedResolvedMu.RUnlock()
		return id, secret
	}
	cachedResolvedMu.RUnlock()

	// Slow path: load and cache
	cachedResolvedMu.Lock()
	defer cachedResolvedMu.Unlock()
	// Double-check after acquiring write lock
	if cachedResolvedValid {
		return cachedResolvedID, cachedResolvedSecret
	}

	cfg := GetCachedAppConfig(configDir)
	if cfg != nil {
		cachedResolvedID = cfg.ClientID
		if secret, err := ResolveSecret(cfg.ClientSecret); err == nil {
			cachedResolvedSecret = secret
		}
	}
	cachedResolvedValid = true
	return cachedResolvedID, cachedResolvedSecret
}

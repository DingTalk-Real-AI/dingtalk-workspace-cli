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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	tokenFileName = "token"

	// LegacyPlainTokenEnv is the feature flag that re-enables the legacy
	// plaintext token file at <configDir>/token. Default (unset or any value
	// other than "1") disables all plaintext token reads and writes.
	//
	// Rationale: the Manager type predates the encrypted keychain + .data
	// storage backends and remains only for backward-compatibility tooling.
	// Leaving the plaintext path always-on silently encourages insecure
	// token storage, so the open-source default MUST be off.
	LegacyPlainTokenEnv = "DWS_LEGACY_PLAIN_TOKEN"
)

// ErrLegacyPlainTokenDisabled is returned from Manager methods when the
// legacy plaintext token path is disabled (the default). Callers should
// fall back to the keychain / encrypted storage path instead.
var ErrLegacyPlainTokenDisabled = fmt.Errorf(
	"legacy plaintext token path is disabled; set %s=1 to re-enable "+
		"(strongly discouraged; use the encrypted storage backend instead)",
	LegacyPlainTokenEnv,
)

// legacyPlainTokenEnabled reports whether the legacy plaintext token
// path is explicitly enabled for the current process.
func legacyPlainTokenEnabled() bool {
	return os.Getenv(LegacyPlainTokenEnv) == "1"
}

// Manager persists OAuth tokens and MCP URL under a config directory.
//
// NOTE: this type predates the encrypted keychain / secure_store storage
// backends. By default (i.e. when DWS_LEGACY_PLAIN_TOKEN != "1") every
// read and write method returns a disabled-path sentinel without touching
// disk. Callers should prefer the secure_store path.
type Manager struct {
	configDir string
	logger    *slog.Logger
}

// NewManager constructs a Manager bound to configDir. It does not create
// the directory or touch any file until a (plaintext-enabled) write
// method is called.
func NewManager(configDir string, logger *slog.Logger) *Manager {
	return &Manager{
		configDir: configDir,
		logger:    logger,
	}
}

// GetToken returns the plaintext token persisted at <configDir>/token.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// ("", "", ErrLegacyPlainTokenDisabled) without reading disk.
func (m *Manager) GetToken() (string, string, error) {
	if !legacyPlainTokenEnabled() {
		return "", "", ErrLegacyPlainTokenDisabled
	}
	token, err := m.loadFromFile()
	if err == nil && token != "" {
		if m.logger != nil {
			m.logger.Debug("using token from config file")
		}
		return token, "file", nil
	}

	return "", "", fmt.Errorf("%s", i18n.T("未找到认证信息，请运行 dws auth login"))
}

// GetMCPURL returns the MCP URL persisted at <configDir>/mcp_url.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// ("", ErrLegacyPlainTokenDisabled) without reading disk.
func (m *Manager) GetMCPURL() (string, error) {
	if !legacyPlainTokenEnabled() {
		return "", ErrLegacyPlainTokenDisabled
	}
	raw, err := m.loadStringFromFile("mcp_url")
	if err == nil && raw != "" {
		return raw, nil
	}
	return "", fmt.Errorf("%s", i18n.T("未找到 MCP Server URL"))
}

// SaveToken writes token to <configDir>/token in plaintext.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// ErrLegacyPlainTokenDisabled without creating or writing any file.
func (m *Manager) SaveToken(token string) error {
	if !legacyPlainTokenEnabled() {
		return ErrLegacyPlainTokenDisabled
	}
	if err := os.MkdirAll(m.configDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	path := filepath.Join(m.configDir, tokenFileName)
	if err := os.WriteFile(path, []byte(token), config.FilePerm); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}
	return nil
}

// SaveMCPURL writes url to <configDir>/mcp_url.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// ErrLegacyPlainTokenDisabled without creating or writing any file.
func (m *Manager) SaveMCPURL(url string) error {
	if !legacyPlainTokenEnabled() {
		return ErrLegacyPlainTokenDisabled
	}
	if err := os.MkdirAll(m.configDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	path := filepath.Join(m.configDir, "mcp_url")
	if err := os.WriteFile(path, []byte(url), config.FilePerm); err != nil {
		return fmt.Errorf("saving MCP URL: %w", err)
	}
	return nil
}

// DeleteToken removes <configDir>/token if present.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// nil (idempotent no-op, matching the semantics of deleting an absent
// file); no disk I/O is performed.
func (m *Manager) DeleteToken() error {
	if !legacyPlainTokenEnabled() {
		if m.logger != nil {
			m.logger.Debug("legacy plaintext token path disabled; DeleteToken no-op")
		}
		return nil
	}
	path := filepath.Join(m.configDir, tokenFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting token: %w", err)
	}
	return nil
}

// IsAuthenticated reports whether a usable plaintext token is persisted.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// false without reading disk.
func (m *Manager) IsAuthenticated() bool {
	if !legacyPlainTokenEnabled() {
		return false
	}
	token, _, err := m.GetToken()
	return err == nil && token != ""
}

// Status returns a tuple describing the persisted plaintext auth state.
//
// When DWS_LEGACY_PLAIN_TOKEN != "1" this method is disabled and returns
// (false, "", "") without reading disk.
func (m *Manager) Status() (authenticated bool, source string, maskedToken string) {
	if !legacyPlainTokenEnabled() {
		return false, "", ""
	}
	token, source, err := m.GetToken()
	if err != nil {
		return false, "", ""
	}
	return true, source, maskToken(token)
}

func (m *Manager) loadFromFile() (string, error) {
	return m.loadStringFromFile(tokenFileName)
}

func (m *Manager) loadStringFromFile(name string) (string, error) {
	path := filepath.Join(m.configDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	tokenFileName = "token"
)

type Manager struct {
	configDir string
	logger    *slog.Logger
}

func NewManager(configDir string, logger *slog.Logger) *Manager {
	return &Manager{
		configDir: configDir,
		logger:    logger,
	}
}

func (m *Manager) GetToken() (string, string, error) {
	token, err := m.loadFromFile()
	if err == nil && token != "" {
		if m.logger != nil {
			m.logger.Debug("using token from config file")
		}
		return token, "file", nil
	}

	return "", "", fmt.Errorf("%s", i18n.T("未找到认证信息，请运行 dws auth login"))
}

func (m *Manager) GetMCPURL() (string, error) {
	raw, err := m.loadStringFromFile("mcp_url")
	if err == nil && raw != "" {
		return raw, nil
	}
	return "", fmt.Errorf("%s", i18n.T("未找到 MCP Server URL"))
}

func (m *Manager) SaveToken(token string) error {
	if err := os.MkdirAll(m.configDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	path := filepath.Join(m.configDir, tokenFileName)
	if err := os.WriteFile(path, []byte(token), config.FilePerm); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}
	return nil
}

func (m *Manager) SaveMCPURL(url string) error {
	if err := os.MkdirAll(m.configDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	path := filepath.Join(m.configDir, "mcp_url")
	if err := os.WriteFile(path, []byte(url), config.FilePerm); err != nil {
		return fmt.Errorf("saving MCP URL: %w", err)
	}
	return nil
}

func (m *Manager) DeleteToken() error {
	path := filepath.Join(m.configDir, tokenFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting token: %w", err)
	}
	return nil
}

func (m *Manager) IsAuthenticated() bool {
	token, _, err := m.GetToken()
	return err == nil && token != ""
}

func (m *Manager) Status() (authenticated bool, source string, maskedToken string) {
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

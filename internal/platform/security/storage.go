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

package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/config"
)

// DataFileName is the encrypted token data file name.
const DataFileName = ".data"

// TokenData mirrors auth.TokenData for the security layer.
type TokenData struct {
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token"`
	PersistentCode string    `json:"persistent_code"`
	ExpiresAt      time.Time `json:"expires_at"`
	RefreshExpAt   time.Time `json:"refresh_expires_at"`
	CorpID         string    `json:"corp_id"`
	UserID         string    `json:"user_id,omitempty"`
	UserName       string    `json:"user_name,omitempty"`
	CorpName       string    `json:"corp_name,omitempty"`
	UpdatedAt      string    `json:"updated_at,omitempty"`
	Source         string    `json:"source,omitempty"`
}

// SecureTokenStorage provides encrypted token persistence using MAC.
type SecureTokenStorage struct {
	configDir   string
	fallbackDir string
	macAddr     string
}

// NewSecureTokenStorage creates a new secure storage instance.
// fallbackDir may be empty.
func NewSecureTokenStorage(configDir, fallbackDir, macAddr string) *SecureTokenStorage {
	return &SecureTokenStorage{
		configDir:   configDir,
		fallbackDir: fallbackDir,
		macAddr:     macAddr,
	}
}

func (s *SecureTokenStorage) password() []byte {
	return []byte(s.macAddr)
}

// DataDirs returns all configured data directories.
func (s *SecureTokenStorage) DataDirs() []string {
	var out []string
	if s.configDir != "" {
		out = append(out, s.configDir)
	}
	if s.fallbackDir != "" {
		out = append(out, s.fallbackDir)
	}
	return out
}

func dataPath(dir string) string {
	return filepath.Join(dir, DataFileName)
}

// Exists checks whether an encrypted data file exists in any configured dir.
func (s *SecureTokenStorage) Exists() bool {
	if st, err := os.Stat(dataPath(s.configDir)); err == nil && !st.IsDir() {
		return true
	}
	if s.fallbackDir != "" {
		if st, err := os.Stat(dataPath(s.fallbackDir)); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// DataFileExistsInAny checks whether .data exists in any of the given dirs.
func DataFileExistsInAny(dirs ...string) bool {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if st, err := os.Stat(dataPath(dir)); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// SaveToken encrypts and persists token data using atomic write with fsync.
func (s *SecureTokenStorage) SaveToken(data *TokenData) error {
	if err := os.MkdirAll(s.configDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}
	enc, err := Encrypt(b, s.password())
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}
	finalPath := dataPath(s.configDir)
	tmpPath := finalPath + ".tmp"

	// Atomic write with fsync to ensure data durability
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, config.FilePerm)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	writeSuccess := false
	defer func() {
		if !writeSuccess {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(enc); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming to final: %w", err)
	}
	writeSuccess = true
	return nil
}

// LoadToken reads and decrypts .data; tries configDir first, then fallbackDir.
func (s *SecureTokenStorage) LoadToken() (*TokenData, error) {
	var raw []byte
	var err error
	raw, err = os.ReadFile(dataPath(s.configDir))
	if err != nil && s.fallbackDir != "" {
		raw, err = os.ReadFile(dataPath(s.fallbackDir))
	}
	if err != nil {
		return nil, fmt.Errorf("reading encrypted file: %w", err)
	}
	plain, err := Decrypt(raw, s.password())
	if err != nil {
		return nil, err
	}
	var data TokenData
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, fmt.Errorf("parsing token JSON: %w", err)
	}
	return &data, nil
}

// DeleteToken removes encrypted data files from all configured dirs.
func (s *SecureTokenStorage) DeleteToken() error {
	var firstErr error
	for _, dir := range []string{s.configDir, s.fallbackDir} {
		if dir == "" {
			continue
		}
		p := dataPath(dir)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("deleting %s: %w", p, err)
		}
		_ = os.Remove(p + ".tmp")
	}
	return firstErr
}

// DeleteEncryptedData removes .data files from the given directories.
func DeleteEncryptedData(configDir string, fallbackDirs ...string) error {
	var firstErr error
	dirs := append([]string{configDir}, fallbackDirs...)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		p := dataPath(dir)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("deleting %s: %w", p, err)
		}
		_ = os.Remove(p + ".tmp")
	}
	return firstErr
}

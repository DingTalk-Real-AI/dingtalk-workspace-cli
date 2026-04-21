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
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// DataFileName is the encrypted token data file name.
const DataFileName = ".data"

// cachedMAC memoises the device MAC so that repeated token reads/writes do
// not re-scan network interfaces.
var (
	cachedMAC     string
	cachedMACOnce sync.Once
	cachedMACErr  error
)

// CachedMACAddress returns the process-cached MAC address, fetching it once
// via GetMACAddress on first call. It is safe for concurrent use.
func CachedMACAddress() (string, error) {
	cachedMACOnce.Do(func() {
		cachedMAC, cachedMACErr = GetMACAddress()
	})
	return cachedMAC, cachedMACErr
}

// DerivePassword returns the encryption password derived from the device
// MAC address. Callers MUST NOT log or persist the returned value.
func DerivePassword() ([]byte, error) {
	mac, err := CachedMACAddress()
	if err != nil {
		return nil, fmt.Errorf("getting MAC address for encryption: %w", err)
	}
	return []byte(mac), nil
}

// SecureTokenStorage provides encrypted raw-byte persistence at
// <configDir>/.data using AES-256-GCM with a caller-supplied password
// (typically the device MAC). It is the single implementation; higher-level
// token marshalling lives in internal/auth's wrapper functions.
type SecureTokenStorage struct {
	configDir   string
	fallbackDir string
	password    []byte
}

// NewSecureTokenStorage creates a storage bound to configDir with an optional
// fallbackDir consulted on reads only. Pass macAddr (or any key material) as
// the password; for the CLI default, use NewDefaultStorage which derives
// the key from the cached device MAC.
func NewSecureTokenStorage(configDir, fallbackDir, macAddr string) *SecureTokenStorage {
	return &SecureTokenStorage{
		configDir:   configDir,
		fallbackDir: fallbackDir,
		password:    []byte(macAddr),
	}
}

// NewDefaultStorage returns a SecureTokenStorage bound to configDir using the
// cached device-MAC-derived password. Useful for callers that do not need a
// fallback directory.
func NewDefaultStorage(configDir string) (*SecureTokenStorage, error) {
	pw, err := DerivePassword()
	if err != nil {
		return nil, err
	}
	return &SecureTokenStorage{configDir: configDir, password: pw}, nil
}

// DataDirs returns all configured data directories (primary first, then
// fallback if set).
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

// Exists reports whether an encrypted .data file exists in either the primary
// or fallback configured directory.
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

// SaveEncryptedBytes encrypts plaintext and persists it to <configDir>/.data
// via atomic-rename. The caller is responsible for zeroing plaintext on its
// own side once this call returns; the storage does not retain a reference
// to the buffer.
//
// The directory is created with 0700 permissions; if it already exists with
// unsafe permissions, they are tightened before writing.
//
// Concurrency: callers that race on the same configDir MUST coordinate
// externally (e.g. via a business-level file lock) to avoid tmp-file
// collisions. One of the concurrent writers will win the rename.
func (s *SecureTokenStorage) SaveEncryptedBytes(plaintext []byte) error {
	if err := os.MkdirAll(s.configDir, config.DirPerm); err != nil {
		return fmt.Errorf("creating config dir %s: %w", s.configDir, err)
	}
	if info, statErr := os.Stat(s.configDir); statErr == nil {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			if chErr := os.Chmod(s.configDir, config.DirPerm); chErr != nil {
				return fmt.Errorf("config dir %s has unsafe permissions %o and chmod failed: %w", s.configDir, perm, chErr)
			}
		}
	}

	ciphertext, err := Encrypt(plaintext, s.password)
	if err != nil {
		return fmt.Errorf("encrypting token data: %w", err)
	}

	finalPath := dataPath(s.configDir)
	tmpPath := finalPath + ".tmp"

	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, config.FilePerm)
	if err != nil {
		return fmt.Errorf("creating tmp file: %w", err)
	}

	writeSuccess := false
	defer func() {
		if !writeSuccess {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(ciphertext); err != nil {
		return fmt.Errorf("writing tmp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("syncing tmp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing tmp file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming tmp to final: %w", err)
	}
	writeSuccess = true
	return nil
}

// LoadEncryptedBytes reads and decrypts <configDir>/.data; falls back to
// <fallbackDir>/.data when the primary read fails.
//
// The caller is responsible for zeroing the returned plaintext buffer once
// it has been parsed.
func (s *SecureTokenStorage) LoadEncryptedBytes() ([]byte, error) {
	raw, err := os.ReadFile(dataPath(s.configDir))
	if err != nil && s.fallbackDir != "" {
		raw, err = os.ReadFile(dataPath(s.fallbackDir))
	}
	if err != nil {
		return nil, fmt.Errorf("reading encrypted file: %w", err)
	}
	plain, err := Decrypt(raw, s.password)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// DeleteToken removes the encrypted .data file (and any leftover .tmp) from
// every configured directory.
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
// Unlike SecureTokenStorage.DeleteToken, this is a free function usable
// by callers that do not hold a storage instance.
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

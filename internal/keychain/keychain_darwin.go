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

//go:build darwin

package keychain

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const (
	keychainTimeout = 5 * time.Second
	dekBytes        = 32 // DEK = Data Encryption Key (AES-256)
	ivBytes         = 12
	tagBytes        = 16
)

// StorageDir returns the storage directory for a given service name on macOS.
// Uses ~/Library/Application Support/<service> following Apple conventions.
// When the DWS_KEYCHAIN_DIR environment variable is set (used by tests for
// isolation), the storage root is taken from that env var instead.
func StorageDir(service string) string {
	if override := os.Getenv(StorageDirEnv); override != "" {
		return filepath.Join(override, service)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".dws", "keychain", service)
	}
	return filepath.Join(home, "Library", "Application Support", service)
}

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

var readDefaultKeychain = func() ([]byte, error) {
	return exec.Command("security", "default-keychain", "-d", "user").Output()
}

var (
	keyringGet = keyring.Get
	keyringSet = keyring.Set
)

func defaultKeychainPathFromSecurityOutput(output []byte) string {
	value := strings.TrimSpace(string(output))
	if value == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, `"`)
}

func checkDefaultKeychainAvailable() error {
	output, err := readDefaultKeychain()
	if err != nil {
		return nil
	}
	path := defaultKeychainPathFromSecurityOutput(output)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return NewUnavailableError("read macOS default Keychain", fmt.Errorf("default keychain %q does not exist", path))
	}
	return nil
}

func platformDiagnose() Diagnostic {
	detail := map[string]string{
		"platform": "darwin",
		"service":  Service,
		"account":  "dek",
	}
	if os.Getenv(DisableKeychainEnv) != "" {
		detail["mode"] = "file_dek"
		detail["storage_dir"] = StorageDir(Service)
		return Diagnostic{
			OK:      true,
			Message: "macOS Keychain 已禁用, 当前使用 file-DEK 测试模式",
			Detail:  detail,
		}
	}

	output, err := readDefaultKeychain()
	if err != nil {
		detail["error"] = err.Error()
		return Diagnostic{
			OK:      false,
			Reason:  "keychain_check_failed",
			Message: "无法读取 macOS 默认钥匙串配置",
			Hint:    "检查 /usr/bin/security 是否可用, 并确认当前用户钥匙串配置正常。",
			Detail:  detail,
		}
	}

	path := defaultKeychainPathFromSecurityOutput(output)
	if path != "" {
		detail["default_keychain"] = path
	}
	if path == "" {
		return Diagnostic{
			OK:      false,
			Reason:  "keychain_unavailable",
			Message: "macOS 默认钥匙串未配置",
			Hint:    "恢复默认钥匙串后重试；测试环境可设置 DWS_DISABLE_KEYCHAIN=1 后重新登录。",
			Detail:  detail,
		}
	}
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return Diagnostic{
			OK:      false,
			Reason:  "keychain_unavailable",
			Message: "macOS 默认钥匙串不存在",
			Hint:    "恢复默认钥匙串后重试；测试环境可设置 DWS_DISABLE_KEYCHAIN=1 后重新登录。",
			Detail:  detail,
		}
	} else if err != nil {
		detail["error"] = err.Error()
		return Diagnostic{
			OK:      false,
			Reason:  "keychain_check_failed",
			Message: "无法访问 macOS 默认钥匙串",
			Hint:    "检查默认钥匙串路径权限与挂载状态。",
			Detail:  detail,
		}
	}

	return Diagnostic{
		OK:      true,
		Message: "macOS 默认钥匙串可用",
		Detail:  detail,
	}
}

// getDEK retrieves or generates the Data Encryption Key.
// When DWS_DISABLE_KEYCHAIN=1 (set in sandboxed runtimes like Codex App
// where Keychain APIs are blocked), falls back to a file-based DEK
// identical to the Linux scheme. See DisableKeychainEnv docs for the
// security tradeoff.
func getDEK(service string) ([]byte, error) {
	return getOrCreateDEK(service)
}

func getDEKReadOnly(service string) ([]byte, error) {
	if os.Getenv(DisableKeychainEnv) != "" {
		return fileDEKReadOnly(service)
	}
	if err := checkDefaultKeychainAvailable(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	type result struct {
		key []byte
		err error
	}
	resCh := make(chan result, 1)

	go func() {
		defer func() { recover() }()

		encodedKey, err := keyringGet(service, "dek")
		if err == nil {
			key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
			if decodeErr == nil && len(key) == dekBytes {
				resCh <- result{key: key, err: nil}
				return
			}
			resCh <- result{key: nil, err: fmt.Errorf("read DEK from macOS Keychain: %w", ErrDEKMissing)}
			return
		}
		if errors.Is(err, keyring.ErrNotFound) {
			resCh <- result{key: nil, err: fmt.Errorf("read DEK from macOS Keychain: %w", ErrDEKMissing)}
			return
		}
		resCh <- result{key: nil, err: NewUnavailableError("read DEK from macOS Keychain", err)}
	}()

	select {
	case res := <-resCh:
		return res.key, res.err
	case <-ctx.Done():
		return nil, NewUnavailableError("read DEK from macOS Keychain", ctx.Err())
	}
}

func getOrCreateDEK(service string) ([]byte, error) {
	if os.Getenv(DisableKeychainEnv) != "" {
		return fileDEK(service)
	}
	if err := checkDefaultKeychainAvailable(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	type result struct {
		key []byte
		err error
	}
	resCh := make(chan result, 1)

	go func() {
		defer func() { recover() }()

		// Try to get existing DEK from system Keychain
		encodedKey, err := keyringGet(service, "dek")
		if err == nil {
			key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
			if decodeErr == nil && len(key) == dekBytes {
				resCh <- result{key: key, err: nil}
				return
			}
		} else if !errors.Is(err, keyring.ErrNotFound) {
			resCh <- result{key: nil, err: NewUnavailableError("read DEK from macOS Keychain", err)}
			return
		}

		// Generate new DEK if not found or invalid
		key := make([]byte, dekBytes)
		if _, randErr := rand.Read(key); randErr != nil {
			resCh <- result{key: nil, err: randErr}
			return
		}

		// Store in system Keychain
		encodedKey = base64.StdEncoding.EncodeToString(key)
		if setErr := keyringSet(service, "dek", encodedKey); setErr != nil {
			resCh <- result{key: nil, err: NewUnavailableError("store DEK in macOS Keychain", setErr)}
			return
		}
		resCh <- result{key: key, err: nil}
	}()

	select {
	case res := <-resCh:
		return res.key, res.err
	case <-ctx.Done():
		return nil, NewUnavailableError("read DEK from macOS Keychain", ctx.Err())
	}
}

func encryptData(plaintext string, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, ivBytes)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	ciphertext := aesGCM.Seal(nil, iv, []byte(plaintext), nil)
	result := make([]byte, 0, ivBytes+len(ciphertext))
	result = append(result, iv...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptData(data []byte, key []byte) (string, error) {
	if len(data) < ivBytes+tagBytes {
		return "", fmt.Errorf("ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	iv := data[:ivBytes]
	ciphertext := data[ivBytes:]
	plaintext, err := aesGCM.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}
	return string(plaintext), nil
}

func platformGet(service, account string) (string, error) {
	data, err := os.ReadFile(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Not found is not an error
		}
		return "", err
	}
	key, err := getDEKReadOnly(service)
	if err != nil {
		return "", err
	}
	plaintext, err := decryptData(data, key)
	if err != nil {
		return "", err
	}
	return plaintext, nil
}

func platformSet(service, account, data string) error {
	key, err := getOrCreateDEK(service)
	if err != nil {
		return err
	}
	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encrypted, err := encryptData(data, key)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(dir, safeFileName(account))
	tmpPath := filepath.Join(dir, safeFileName(account)+"."+uuid.New().String()+".tmp")
	defer os.Remove(tmpPath)

	if err := os.WriteFile(tmpPath, encrypted, 0600); err != nil {
		return err
	}

	// Atomic rename to prevent file corruption during multi-process writes
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	return nil
}

func platformRemove(service, account string) error {
	err := os.Remove(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

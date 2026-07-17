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
	dekBytes = 32 // DEK = Data Encryption Key (AES-256)
	ivBytes  = 12
	tagBytes = 16
)

var keychainTimeout = 5 * time.Second

// StorageDir returns the storage directory for a given service name on macOS.
// Uses ~/Library/Application Support/<service> following Apple conventions.
// When the DWS_KEYCHAIN_DIR environment variable is set (used by tests for
// isolation), the storage root is taken from that env var instead.
func StorageDir(service string) string {
	if override := os.Getenv(StorageDirEnv); override != "" {
		return filepath.Join(override, service)
	}
	home, err := keychainUserHomeDir()
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
	keyringGet                       = keyring.Get
	keyringSet                       = keyring.Set
	keychainAuthTokenCiphertextPaths = authTokenCiphertextPaths
	keychainEntryReadFile            = os.ReadFile
)

type darwinKeychainRuntime struct {
	checkAvailable func() error
	get            func(service, account string) (string, error)
	set            func(service, account, value string) error
	randRead       func([]byte) (int, error)
	timeout        time.Duration
}

func snapshotDarwinKeychainRuntime() darwinKeychainRuntime {
	return darwinKeychainRuntime{
		checkAvailable: checkDefaultKeychainAvailable,
		get:            keyringGet,
		set:            keyringSet,
		randRead:       keychainRandRead,
		timeout:        keychainTimeout,
	}
}

type darwinKeychainResult struct {
	key []byte
	err error
}

type darwinKeychainWorker struct {
	result <-chan darwinKeychainResult
	done   <-chan struct{}
}

func startDarwinKeychainWorker(operation string, work func() ([]byte, error)) darwinKeychainWorker {
	result := make(chan darwinKeychainResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				result <- darwinKeychainResult{
					err: NewUnavailableError(operation, fmt.Errorf("panic: %v", recovered)),
				}
			}
		}()
		key, err := work()
		result <- darwinKeychainResult{key: key, err: err}
	}()
	return darwinKeychainWorker{result: result, done: done}
}

func waitDarwinKeychainWorker(timeout time.Duration, operation string, worker darwinKeychainWorker) ([]byte, error, <-chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case result := <-worker.result:
		<-worker.done
		return result.key, result.err, worker.done
	case <-ctx.Done():
		return nil, NewUnavailableError(operation, ctx.Err()), worker.done
	}
}

func finishedDarwinKeychainWorker() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

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
	if _, err := keychainStat(path); err != nil && os.IsNotExist(err) {
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
	if _, err := keychainStat(path); err != nil && os.IsNotExist(err) {
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

func getSystemDEKReadOnly(service string) ([]byte, error) {
	key, err, _ := getSystemDEKReadOnlyWithRuntime(service, snapshotDarwinKeychainRuntime())
	return key, err
}

func getSystemDEKReadOnlyWithRuntime(service string, runtime darwinKeychainRuntime) ([]byte, error, <-chan struct{}) {
	if err := runtime.checkAvailable(); err != nil {
		return nil, err, finishedDarwinKeychainWorker()
	}

	const operation = "read DEK from macOS Keychain"
	worker := startDarwinKeychainWorker(operation, func() ([]byte, error) {
		encodedKey, err := runtime.get(service, "dek")
		if err == nil {
			key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
			if decodeErr == nil && len(key) == dekBytes {
				return key, nil
			}
			return nil, fmt.Errorf("read DEK from macOS Keychain: %w", ErrDEKMissing)
		}
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("read DEK from macOS Keychain: %w", ErrDEKMissing)
		}
		return nil, NewUnavailableError("read DEK from macOS Keychain", err)
	})

	return waitDarwinKeychainWorker(runtime.timeout, operation, worker)
}

func getOrCreateDEK(service string) ([]byte, error) {
	if os.Getenv(DisableKeychainEnv) != "" {
		return fileDEK(service)
	}
	key, err, _ := getOrCreateDEKWithRuntime(service, snapshotDarwinKeychainRuntime())
	return key, err
}

func getOrCreateDEKWithRuntime(service string, runtime darwinKeychainRuntime) ([]byte, error, <-chan struct{}) {
	if err := runtime.checkAvailable(); err != nil {
		return nil, err, finishedDarwinKeychainWorker()
	}

	const operation = "read or create DEK in macOS Keychain"
	worker := startDarwinKeychainWorker(operation, func() ([]byte, error) {
		// Try to get existing DEK from system Keychain
		encodedKey, err := runtime.get(service, "dek")
		if err == nil {
			key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
			if decodeErr == nil && len(key) == dekBytes {
				return key, nil
			}
		} else if !errors.Is(err, keyring.ErrNotFound) {
			return nil, NewUnavailableError("read DEK from macOS Keychain", err)
		}

		// Generate new DEK if not found or invalid
		key := make([]byte, dekBytes)
		if _, randErr := runtime.randRead(key); randErr != nil {
			return nil, randErr
		}

		// Store in system Keychain
		encodedKey = base64.StdEncoding.EncodeToString(key)
		if setErr := runtime.set(service, "dek", encodedKey); setErr != nil {
			return nil, NewUnavailableError("store DEK in macOS Keychain", setErr)
		}
		return key, nil
	})

	return waitDarwinKeychainWorker(runtime.timeout, operation, worker)
}

func encryptData(plaintext string, key []byte) ([]byte, error) {
	return encryptDataWithGCM(plaintext, key, cipher.NewGCM)
}

func encryptDataWithGCM(plaintext string, key []byte, newGCM keychainGCMFactory) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := newGCM(block)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, ivBytes)
	if _, err := keychainRandRead(iv); err != nil {
		return nil, err
	}

	ciphertext := aesGCM.Seal(nil, iv, []byte(plaintext), nil)
	result := make([]byte, 0, ivBytes+len(ciphertext))
	result = append(result, iv...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptData(data []byte, key []byte) (string, error) {
	return decryptDataWithGCM(data, key, cipher.NewGCM)
}

func decryptDataWithGCM(data []byte, key []byte, newGCM keychainGCMFactory) (string, error) {
	if len(data) < ivBytes+tagBytes {
		return "", fmt.Errorf("ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := newGCM(block)
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

// decryptWithAvailableDEK decrypts an existing entry without creating or
// migrating key material. In normal macOS mode, entries previously written by
// the explicit file-DEK fallback remain readable so another local process
// cannot replace them with ciphertext encrypted by a different key.
func decryptWithAvailableDEK(service string, data []byte) (string, []byte, error) {
	if os.Getenv(DisableKeychainEnv) != "" {
		key, err := fileDEKReadOnly(service)
		if err != nil {
			return "", nil, err
		}
		plaintext, err := decryptData(data, key)
		if err != nil {
			return "", nil, fmt.Errorf("%w: file-DEK cannot decrypt existing entry", ErrCiphertextKeyMismatch)
		}
		return plaintext, key, nil
	}

	systemKey, systemKeyErr := getSystemDEKReadOnly(service)
	if IsUnavailable(systemKeyErr) {
		return "", nil, systemKeyErr
	}
	if systemKeyErr == nil {
		if plaintext, err := decryptData(data, systemKey); err == nil {
			return plaintext, systemKey, nil
		}
	}

	fileKey, fileKeyErr := fileDEKReadOnly(service)
	if fileKeyErr == nil {
		if plaintext, err := decryptData(data, fileKey); err == nil {
			return plaintext, fileKey, nil
		}
	}

	if systemKeyErr != nil && fileKeyErr != nil {
		return "", nil, systemKeyErr
	}
	return "", nil, fmt.Errorf("%w: available DEKs cannot decrypt existing entry", ErrCiphertextKeyMismatch)
}

// keyForNewEntry preserves the backend used by the canonical auth-token entry
// when a related account is added from a normal macOS process. This prevents
// profile-scoped token slots from mixing DEK backends without allowing an
// unrelated file-backed secret to downgrade a system-Keychain-backed login.
func keyForNewEntry(service, account string) ([]byte, error) {
	if os.Getenv(DisableKeychainEnv) != "" {
		return getOrCreateDEK(service)
	}
	if account != AccountToken && !strings.HasPrefix(account, AccountToken+":") {
		return getOrCreateDEK(service)
	}

	anchorPath := filepath.Join(StorageDir(service), safeFileName(AccountToken))
	anchor, err := keychainEntryReadFile(anchorPath)
	if err == nil {
		_, key, decryptErr := decryptWithAvailableDEK(service, anchor)
		return key, decryptErr
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	return getOrCreateDEK(service)
}

func platformGet(service, account string) (string, error) {
	data, err := keychainReadFile(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Not found is not an error
		}
		return "", err
	}
	plaintext, _, err := decryptWithAvailableDEK(service, data)
	if err != nil {
		return "", err
	}
	return plaintext, nil
}

func platformSet(service, account, data string) error {
	dir := StorageDir(service)
	targetPath := filepath.Join(dir, safeFileName(account))

	var (
		key []byte
		err error
	)
	existing, readErr := keychainEntryReadFile(targetPath)
	switch {
	case readErr == nil:
		_, key, err = decryptWithAvailableDEK(service, existing)
	case os.IsNotExist(readErr):
		key, err = keyForNewEntry(service, account)
	default:
		return readErr
	}
	if err != nil {
		return err
	}
	if err := keychainMkdirAll(dir, 0700); err != nil {
		return err
	}
	encrypted, err := encryptData(data, key)
	if err != nil {
		return err
	}

	tmpPath := filepath.Join(dir, safeFileName(account)+"."+uuid.New().String()+".tmp")
	defer keychainRemove(tmpPath)

	if err := keychainWriteFile(tmpPath, encrypted, 0600); err != nil {
		return err
	}

	// Atomic rename to prevent file corruption during multi-process writes
	if err := keychainRename(tmpPath, targetPath); err != nil {
		return err
	}
	return nil
}

func platformValidateAuthTokenEntries(service string) error {
	paths, err := keychainAuthTokenCiphertextPaths(service)
	if err != nil {
		return err
	}
	for _, path := range paths {
		ciphertext, err := keychainEntryReadFile(path)
		if err != nil {
			return fmt.Errorf("read keychain entry %q: %w", filepath.Base(path), err)
		}
		if _, _, err := decryptWithAvailableDEK(service, ciphertext); err != nil {
			return fmt.Errorf("validate keychain entry %q: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func platformRemove(service, account string) error {
	err := keychainRemove(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

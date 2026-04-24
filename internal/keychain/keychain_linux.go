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

//go:build linux

package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/uuid"
)

const (
	dekBytes = 32 // DEK = Data Encryption Key (AES-256)
	ivBytes  = 12
	tagBytes = 16
)

// StorageDir returns the storage directory for a given service name.
// Follows XDG Base Directory Specification: ~/.local/share/<service>.
// When the DWS_KEYCHAIN_DIR environment variable is set (used by tests for
// isolation), the storage root is taken from that env var instead.
func StorageDir(service string) string {
	if override := os.Getenv(StorageDirEnv); override != "" {
		return filepath.Join(override, service)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
		return filepath.Join(".dws", "keychain", service)
	}
	xdgData := filepath.Join(home, ".local", "share")
	return filepath.Join(xdgData, service)
}

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

// getDEK retrieves or generates the Data Encryption Key from local file.
func getDEK(service string) ([]byte, error) {
	dir := StorageDir(service)
	keyPath := filepath.Join(dir, "dek")

	// Try to read existing DEK
	key, err := os.ReadFile(keyPath)
	if err == nil && len(key) == dekBytes {
		return key, nil
	}

	// Create directory if needed
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create keychain dir: %w", err)
	}

	// Generate new random DEK
	key = make([]byte, dekBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}

	// Atomic write to prevent multi-process initialization collision
	tmpKeyPath := filepath.Join(dir, "dek."+uuid.New().String()+".tmp")
	defer os.Remove(tmpKeyPath)

	if err := os.WriteFile(tmpKeyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("write dek: %w", err)
	}

	if err := os.Rename(tmpKeyPath, keyPath); err != nil {
		// If rename fails, another process might have created it. Try reading again.
		existingKey, readErr := os.ReadFile(keyPath)
		if readErr == nil && len(existingKey) == dekBytes {
			return existingKey, nil
		}
		return nil, fmt.Errorf("save dek: %w", err)
	}

	return key, nil
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
	key, err := getDEK(service)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Not found is not an error
		}
		return "", err
	}
	plaintext, err := decryptData(data, key)
	if err != nil {
		return "", err
	}
	return plaintext, nil
}

func platformSet(service, account, data string) error {
	key, err := getDEK(service)
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

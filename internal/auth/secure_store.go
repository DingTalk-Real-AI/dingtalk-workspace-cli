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
	"errors"
	"fmt"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/security"
)

// secureDataFile mirrors the constant used by the security package. It is
// kept here so existing tests that reference it continue to compile without
// reaching across packages.
const secureDataFile = security.DataFileName

// ErrTokenDecryption indicates that token decryption failed, typically
// due to a device mismatch or corrupted data file. Callers can check
// this with errors.Is to distinguish decryption failures from other
// I/O or parsing errors.
var ErrTokenDecryption = errors.New("token decryption failed")

// SaveSecureTokenData encrypts and saves TokenData to the legacy .data file.
// It is a thin wrapper over internal/security's byte-oriented storage; the
// AES-256-GCM encryption and MAC-derived key continue to live there.
//
// Concurrency: callers that involve token refresh MUST hold the business-level
// file lock (via acquireTokenLock) to prevent two processes from refreshing
// simultaneously. See OAuthProvider.lockedRefresh().
func SaveSecureTokenData(configDir string, data *TokenData) error {
	plaintext, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling token data: %w", err)
	}
	defer zeroBytes(plaintext)

	storage, err := security.NewDefaultStorage(configDir)
	if err != nil {
		return err
	}
	return storage.SaveEncryptedBytes(plaintext)
}

// LoadSecureTokenData decrypts and loads TokenData from the legacy .data
// file. Reads are safe without locking because SaveSecureTokenData uses
// atomic rename. Returns a wrapped ErrTokenDecryption when the ciphertext
// cannot be decrypted (device mismatch or corruption).
func LoadSecureTokenData(configDir string) (*TokenData, error) {
	storage, err := security.NewDefaultStorage(configDir)
	if err != nil {
		return nil, err
	}
	plaintext, err := storage.LoadEncryptedBytes()
	if err != nil {
		// If the backing file exists but decryption failed, the returned
		// error originates from the Decrypt call. Distinguish it from
		// file-not-found / MAC-fetch errors by checking for the
		// "reading encrypted file" prefix used by the storage layer.
		if storage.Exists() {
			return nil, fmt.Errorf("%w: %v", ErrTokenDecryption, err)
		}
		return nil, err
	}
	defer zeroBytes(plaintext)

	var data TokenData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("parsing decrypted token data: %w", err)
	}
	return &data, nil
}

// DeleteSecureData removes the legacy .data file from configDir (idempotent).
func DeleteSecureData(configDir string) error {
	return security.DeleteEncryptedData(configDir)
}

// SecureDataExists reports whether the legacy .data file exists in configDir.
func SecureDataExists(configDir string) bool {
	return security.DataFileExistsInAny(configDir)
}

// zeroBytes wipes a sensitive plaintext buffer. Kept package-private so
// the wrapper can clear data derived from json.Marshal independently of
// the security layer's own zeroing semantics.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

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

//go:build darwin || linux

package keychain

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// fileDEK retrieves or generates a Data Encryption Key stored as a plain
// file under the platform storage directory. Shared by Linux (default) and
// the macOS sandbox fallback path (DWS_DISABLE_KEYCHAIN=1).
func fileDEK(service string) ([]byte, error) {
	dir := StorageDir(service)
	keyPath := filepath.Join(dir, "dek")

	key, err := os.ReadFile(keyPath)
	if err == nil && len(key) == dekBytes {
		return key, nil
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create keychain dir: %w", err)
	}

	key = make([]byte, dekBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}

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

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build darwin

package keychain

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type fileDEKMigrationEntry struct {
	path      string
	plaintext string
	encrypted []byte
}

func platformMigrateToFileDEK(service string, dryRun bool) (int, error) {
	if os.Getenv(DisableKeychainEnv) != "" {
		return 0, fmt.Errorf("file-DEK migration requires system Keychain mode; unset %s and retry", DisableKeychainEnv)
	}

	paths, err := authTokenCiphertextPaths(service)
	if err != nil {
		return 0, err
	}

	entries := make([]fileDEKMigrationEntry, 0, len(paths))
	for _, path := range paths {
		ciphertext, err := os.ReadFile(path)
		if err != nil {
			return 0, fmt.Errorf("read keychain entry %q: %w", filepath.Base(path), err)
		}
		plaintext, _, err := decryptWithAvailableDEK(service, ciphertext)
		if err != nil {
			return 0, fmt.Errorf("validate keychain entry %q before migration: %w", filepath.Base(path), err)
		}
		entries = append(entries, fileDEKMigrationEntry{path: path, plaintext: plaintext})
	}
	if dryRun || len(entries) == 0 {
		return len(entries), nil
	}

	fileKey, err := fileDEK(service)
	if err != nil {
		return 0, fmt.Errorf("prepare file DEK: %w", err)
	}
	for i := range entries {
		entries[i].encrypted, err = encryptData(entries[i].plaintext, fileKey)
		if err != nil {
			return 0, fmt.Errorf("encrypt keychain entry %q: %w", filepath.Base(entries[i].path), err)
		}
		if _, err := decryptData(entries[i].encrypted, fileKey); err != nil {
			return 0, fmt.Errorf("verify migrated keychain entry %q: %w", filepath.Base(entries[i].path), err)
		}
	}

	tempPaths := make([]string, 0, len(entries))
	defer func() {
		for _, path := range tempPaths {
			_ = os.Remove(path)
		}
	}()
	for _, entry := range entries {
		tmpPath := entry.path + "." + uuid.New().String() + ".migrate.tmp"
		if err := os.WriteFile(tmpPath, entry.encrypted, 0600); err != nil {
			return 0, fmt.Errorf("stage keychain entry %q: %w", filepath.Base(entry.path), err)
		}
		tempPaths = append(tempPaths, tmpPath)
	}
	for i, entry := range entries {
		if err := os.Rename(tempPaths[i], entry.path); err != nil {
			return 0, fmt.Errorf("commit keychain entry %q: %w; rerun the migration to finish", filepath.Base(entry.path), err)
		}
		tempPaths[i] = ""
	}
	return len(entries), nil
}

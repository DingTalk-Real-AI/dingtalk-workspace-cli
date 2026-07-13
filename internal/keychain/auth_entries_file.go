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

//go:build darwin || linux

package keychain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func authTokenCiphertextPaths(service string) ([]string, error) {
	dir := StorageDir(service)
	dirEntries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read keychain storage: %w", err)
	}

	paths := make([]string, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if !isAuthTokenCiphertextFile(dirEntry.Name()) {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect keychain entry %q: %w", dirEntry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("keychain entry %q is not a regular file", dirEntry.Name())
		}
		paths = append(paths, filepath.Join(dir, dirEntry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func isAuthTokenCiphertextFile(name string) bool {
	legacyName := safeFileName(AccountToken)
	return name == legacyName ||
		(strings.HasPrefix(name, strings.TrimSuffix(legacyName, ".enc")+"_") && strings.HasSuffix(name, ".enc"))
}

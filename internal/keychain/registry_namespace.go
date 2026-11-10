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

package keychain

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
)

// registryNamespacePath is the Windows registryPathForService derivation. It has
// no build tag so the cross-platform coverage gate exercises the shipped logic.
// Overrides compose as test-<digest>\dir-<digest>, so a test process cannot leave
// its namespace by changing StorageDirEnv; empty overrides keep basePath.
func registryNamespacePath(basePath, storageDir, testNamespace string) string {
	path := basePath
	if namespace := strings.TrimSpace(testNamespace); namespace != "" {
		path += `\test-` + registryNamespaceDigest(namespace)
	}
	// Used verbatim like StorageDir on every platform, so a whitespace-only
	// value can never collapse into the shared historical key.
	if storageDir != "" {
		path += `\dir-` + registryNamespaceDigest(storageDir)
	}
	return path
}

// registryNamespaceDigest does not require the directory to exist: credentials stay in HKCU.
func registryNamespaceDigest(directory string) string {
	directory = filepath.Clean(directory)
	if absolute, err := filepath.Abs(directory); err == nil {
		directory = absolute
	}
	sum := sha256.Sum256([]byte(strings.ToLower(directory)))
	return fmt.Sprintf("%x", sum[:16])
}

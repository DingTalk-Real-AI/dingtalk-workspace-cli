// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package keychain

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
)

// registryNamespacePath selects a Windows registry namespace without accessing
// the registry. A storage-directory override selects a fresh credential space;
// an empty override preserves the historical path, with no cross-scope fallback.
// The test-only override retains its original path and precedence so a test
// process cannot escape its isolated namespace by changing StorageDirEnv.
func registryNamespacePath(basePath, storageDir, testNamespace string) string {
	directory, prefix := storageDir, "dir-"
	if namespace := strings.TrimSpace(testNamespace); namespace != "" {
		directory, prefix = namespace, "test-"
	}
	if directory == "" {
		return basePath
	}

	// Production callers run on Windows, so filepath handles drive letters,
	// separators and relative paths with native Windows semantics. Do not
	// require the directory to exist: the credentials themselves stay in HKCU.
	directory = filepath.Clean(directory)
	if absolute, err := filepath.Abs(directory); err == nil {
		directory = absolute
	}
	sum := sha256.Sum256([]byte(strings.ToLower(directory)))
	return fmt.Sprintf(`%s\%s%x`, basePath, prefix, sum[:16])
}

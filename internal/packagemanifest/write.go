// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package packagemanifest

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes a canonical 0644 manifest, syncs it, renames it into
// place, and syncs the package root directory. It never opens either binary.
func WriteAtomic(root string, manifest Manifest) (returnErr error) {
	data, err := MarshalCanonical(manifest)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(root, ".package-manifest-*")
	if err != nil {
		return fmt.Errorf("create temporary manifest: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set manifest mode: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	destination := filepath.Join(root, ManifestName)
	if err := os.Rename(temporaryName, destination); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
	}
	directory, err := os.Open(root)
	if err != nil {
		return fmt.Errorf("open package root for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync package root: %w", err)
	}
	return nil
}

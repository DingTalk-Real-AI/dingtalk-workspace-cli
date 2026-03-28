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

package helpers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically by creating a temp file in the
// same directory, writing and fsyncing the data, then renaming over the target.
// It replaces os.WriteFile for all config and download file writes.
//
// os.WriteFile truncates the target before writing, so a process kill (CI timeout,
// OOM, Ctrl+C) between truncate and completion leaves the file empty or partial.
// AtomicWrite avoids this: on any failure the temp file is cleaned up and the
// original file remains untouched.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWrite(path, perm, func(tmp *os.File) error {
		_, err := tmp.Write(data)
		return err
	})
}

// AtomicWriteFromReader atomically copies reader contents into path.
func AtomicWriteFromReader(path string, reader io.Reader, perm os.FileMode) (int64, error) {
	var copied int64
	err := atomicWrite(path, perm, func(tmp *os.File) error {
		n, err := io.Copy(tmp, reader)
		copied = n
		return err
	})
	if err != nil {
		return 0, err
	}
	return copied, nil
}

// AtomicWriteJSON is a convenience wrapper for writing JSON data atomically.
// It uses 0600 permissions by default for sensitive data.
func AtomicWriteJSON(path string, data []byte) error {
	return AtomicWrite(path, data, 0600)
}

func atomicWrite(path string, perm os.FileMode, writeFn func(tmp *os.File) error) error {
	dir := filepath.Dir(path)

	// Ensure directory exists with secure permissions
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err := writeFn(tmp); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync to disk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to final: %w", err)
	}
	success = true
	return nil
}

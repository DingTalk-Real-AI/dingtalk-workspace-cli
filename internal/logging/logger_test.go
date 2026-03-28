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

package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupCreatesLogFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fl := Setup(dir)
	defer fl.Close()

	fl.Logger.Info("test message", "key", "value")
	fl.Close()

	logPath := filepath.Join(dir, logSubDir, logFileName)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "test message") {
		t.Errorf("log file does not contain 'test message': %s", data)
	}
	if !strings.Contains(string(data), `"key":"value"`) {
		t.Errorf("log file does not contain structured key: %s", data)
	}
}

func TestSetupInvalidDirReturnsNopLogger(t *testing.T) {
	t.Parallel()

	fl := Setup("/nonexistent/path/that/cannot/be/created")
	defer fl.Close()

	// Should not panic
	fl.Logger.Info("should be discarded")
}

func TestRotatingWriterRotatesOnSize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	w := newRotatingWriter(logPath, 100, 2) // 100 bytes max
	if err := w.open(); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Write enough data to trigger rotation
	payload := strings.Repeat("x", 60) + "\n"
	w.Write([]byte(payload)) // 61 bytes
	w.Write([]byte(payload)) // triggers rotate at 122 bytes

	// Should have created a backup
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Errorf("backup .1 not created: %v", err)
	}

	// Write more to trigger second rotation
	w.Write([]byte(payload))
	w.Write([]byte(payload))
	w.close()

	if _, err := os.Stat(logPath + ".2"); err != nil {
		t.Errorf("backup .2 not created: %v", err)
	}

	// Active log should be small (last write only)
	info, _ := os.Stat(logPath)
	if info.Size() > 100 {
		t.Errorf("active log too large after rotation: %d bytes", info.Size())
	}
}

func TestRotatingWriterMaxBackups(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	w := newRotatingWriter(logPath, 50, 2)
	if err := w.open(); err != nil {
		t.Fatalf("open: %v", err)
	}

	payload := strings.Repeat("a", 51) + "\n"

	// Force multiple rotations
	for i := 0; i < 5; i++ {
		w.Write([]byte(payload))
	}
	w.close()

	// .3 should NOT exist (maxBackups=2)
	if _, err := os.Stat(logPath + ".3"); err == nil {
		t.Error("backup .3 should not exist with maxBackups=2")
	}
}

func TestCloseOnNilLogger(t *testing.T) {
	t.Parallel()

	var fl *FileLogger
	if err := fl.Close(); err != nil {
		t.Errorf("Close on nil should not error: %v", err)
	}
}

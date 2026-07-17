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

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		// os.UserHomeDir uses USERPROFILE on Windows rather than HOME.
		t.Setenv("USERPROFILE", home)
	}
}

func TestCrossPlatformCoverageConfigureLogLevelTerminatesReplacedFileLoggers(t *testing.T) {
	firstConfig := t.TempDir()
	secondConfig := t.TempDir()
	t.Cleanup(CloseFileLogger)

	t.Setenv("DWS_CONFIG_DIR", firstConfig)
	configureLogLevel(&GlobalFlags{})
	firstLogger := FileLoggerInstance()
	t.Setenv("DWS_CONFIG_DIR", secondConfig)
	configureLogLevel(&GlobalFlags{})

	firstLogPath := filepath.Join(firstConfig, "logs", "dws.log")
	if err := os.Remove(firstLogPath); err != nil {
		t.Fatalf("remove previous logger file: %v", err)
	}
	firstLogger.Info("late write to replaced logger")
	if _, err := os.Stat(firstLogPath); !os.IsNotExist(err) {
		t.Fatalf("replaced logger recreated its log file: %v", err)
	}

	previousSameDirLogger := FileLoggerInstance()
	configureLogLevel(&GlobalFlags{})
	currentLogger := FileLoggerInstance()
	CloseFileLogger()
	secondLogPath := filepath.Join(secondConfig, "logs", "dws.log")
	if err := os.Remove(secondLogPath); err != nil {
		t.Fatalf("remove current logger file: %v", err)
	}
	previousSameDirLogger.Info("late write to same-directory replaced logger")
	currentLogger.Info("late write to closed current logger")
	if _, err := os.Stat(secondLogPath); !os.IsNotExist(err) {
		t.Fatalf("closed logger recreated same-directory log file: %v", err)
	}
}

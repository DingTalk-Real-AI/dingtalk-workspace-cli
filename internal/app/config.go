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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Build-time variables injected via ldflags when available.
var (
	buildTime = "unknown"
	gitCommit = "unknown"
)

func defaultConfigDir() string {
	if envDir := os.Getenv("DWS_CONFIG_DIR"); envDir != "" {
		return envDir
	}
	if fn := edition.Get().ConfigDir; fn != nil {
		return fn()
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return exeRelativeConfigDir()
	}
	return filepath.Join(homeDir, ".dws")
}

func exeRelativeConfigDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return ".dws"
	}
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		realPath = exePath
	}
	return filepath.Join(filepath.Dir(realPath), ".dws")
}

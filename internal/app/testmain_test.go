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
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// TestMain isolates the on-disk keychain storage to a process-wide
// temporary directory for the entire internal/app test binary.
//
// Background: getCachedRuntimeToken caches the auth token via sync.Once
// for the process lifetime. Whichever test triggers it first locks in the
// cached value. Several tests in this package (e.g. TestSkillInstallInvalidTarget)
// call SaveTokenData and then exec a CLI command that triggers Once.Do; if
// keychain storage points at the developer's real ~/Library/Application
// Support/dws-cli (or ~/.local/share/dws-cli on Linux), a real token can be
// written there and cached process-wide, breaking later tests that assume
// "no auth" — most notably TestRuntimeRunnerRejectsUnauthenticatedRequest.
//
// Setting keychain.StorageDirEnv here forces every keychain read/write in
// this binary into a per-process tempdir, eliminating that contamination
// without touching production code.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "dws-app-test-keychain-")
	if err != nil {
		panic("create test keychain tempdir: " + err.Error())
	}
	if err := os.Setenv(keychain.StorageDirEnv, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		panic("set " + keychain.StorageDirEnv + ": " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

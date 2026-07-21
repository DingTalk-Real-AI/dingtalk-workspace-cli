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

package auth

import (
	"fmt"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

// TestMain isolates the on-disk keychain storage to a process-wide
// temporary directory for the entire internal/auth test binary so that
// SaveTokenData/DeleteTokenData calls in these tests can never write to
// the developer's real keychain location, preventing cross-package leaks
// when go test runs packages in parallel.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "dws-auth-test-keychain-")
	if err != nil {
		panic("create test keychain tempdir: " + err.Error())
	}
	if err := os.Setenv(keychain.StorageDirEnv, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		panic("set " + keychain.StorageDirEnv + ": " + err.Error())
	}
	if err := os.Setenv(keychain.TestNamespaceEnv, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		panic("set " + keychain.TestNamespaceEnv + ": " + err.Error())
	}
	code := m.Run()
	if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
		fmt.Fprintf(os.Stderr, "internal/auth keychain cleanup: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

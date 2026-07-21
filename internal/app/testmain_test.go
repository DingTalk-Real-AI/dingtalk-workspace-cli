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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
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
//
// PAT authorization tests also exercise code paths that normally open the
// system browser. Keep the package-wide default opener inert so running the
// test binary never launches a page on the developer's machine; tests that
// need to assert the URL can still replace openBrowserFunc locally.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "dws-app-test-keychain-")
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
	if err := os.Setenv("DWS_CONFIG_DIR", filepath.Join(tmpDir, "config")); err != nil {
		_ = os.RemoveAll(tmpDir)
		panic("set DWS_CONFIG_DIR: " + err.Error())
	}
	for key, value := range map[string]string{
		audit.EnvAudit:         "1",
		audit.EnvAuditDir:      filepath.Join(tmpDir, "audit"),
		audit.EnvRetentionDays: "1",
		audit.EnvForwardURL:    "",
		audit.EnvForwardToken:  "",
		audit.EnvForwardRedact: "none",
		audit.EnvAuditDebug:    "",
	} {
		if err := os.Setenv(key, value); err != nil {
			_ = os.RemoveAll(tmpDir)
			panic("set " + key + ": " + err.Error())
		}
	}
	// Keep the process-wide audit sink outside per-test TempDir trees. Cobra
	// skips PersistentPostRunE on expected command errors, and Windows cannot
	// remove a TempDir while the audit lock is still open.
	setupAuditSink()
	openBrowserFunc = func(string) error { return nil }
	code := m.Run()
	StopAllStdioClients()
	CloseAuditSink()
	CloseFileLogger()
	if err := keychain.RemoveAuthTokenEntries(keychain.Service); err != nil {
		fmt.Fprintf(os.Stderr, "internal/app keychain cleanup: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	if err := os.RemoveAll(tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "internal/app test cleanup %s: %v\n", tmpDir, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

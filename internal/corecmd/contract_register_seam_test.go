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

package corecmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Production framework code must register ContractFinal through the
// corecmd/contractfinal annotate+store seam, and must never import any
// internal/cli package (root or subpackage).
func TestAttachContractUsesContractFinalRegisterSeam(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src := filepath.Join(filepath.Dir(thisFile), "corecmd.go")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read corecmd.go: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "contractfinal.RegisterRuntimeContractFinal(") {
		t.Fatal("AttachContract/New must call contractfinal.RegisterRuntimeContractFinal")
	}
	// Registration goes through contractfinal directly; no cli-root wrapper
	// exists anymore, and the import-prefix check below forbids corecmd → cli.
	// Build the forbidden import prefix without embedding it as a contiguous
	// literal in this test file (the walker below must not self-match).
	forbidden := strings.Join([]string{
		`"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/`,
		`internal/cli`,
	}, "")
	if strings.Contains(body, forbidden) {
		t.Fatal("corecmd must not import any internal/cli package")
	}
}

func TestCorecmdPackageImportsForbidCLI(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(thisFile)
	forbidden := strings.Join([]string{
		`"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/`,
		`internal/cli`,
	}, "")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(raw), forbidden) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s must not import internal/cli", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corecmd: %v", err)
	}
}

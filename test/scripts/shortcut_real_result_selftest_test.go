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

package scripts_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestShortcutRealResultSelfTest wires scripts/shortcut_real_result_test.py into
// `go test ./...` so the upper/lower projection-audit rules (and the end-to-end
// record_real_shortcut_run.py integration) are enforced in CI, not manual-only.
func TestShortcutRealResultSelfTest(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	cmd := exec.Command(py, filepath.Join("scripts", "shortcut_real_result_test.py"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shortcut_real_result_test.py failed: %v\n%s", err, out)
	}
}

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

// Pins RFC §5.0 so the command-framework meaning of "declare" cannot drift
// away from the homology docs / Spec godoc without failing CI.
func TestRFCPinsFrameworkDeclareDefinition(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	doc := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "rfc-command-framework-convergence.md"))
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("read RFC: %v", err)
	}
	body := string(raw)
	for _, needle := range []string{
		"### 5.0 框架上的「声明」定义",
		"声明 = 最终源，Schema = 透传",
		"**声明（declare）**",
		"**框架转换**",
		"**Schema 透传**",
		"禁止** JSON 注解桥",
		"RegisterRuntimeContractFinal",
		"今日契约字段（`corecmd.Spec` / `LeafSpec`）",
		"Schema 全覆盖：`ToolSpec` 字段权威",
		"**Identity**",
		"**Parameters**",
		"**Constraints**",
		"**Positionals**",
		"**Safety**",
		"**DryRun**",
		"**Interface**",
		"**Selection**",
		"ConstParams",
		"与目标 `Contract` 的对应",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("RFC missing framework declare text %q", needle)
		}
	}
}

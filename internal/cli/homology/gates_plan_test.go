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

package homology

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Planned homology gates (docs/flag-help-schema-homology.md §4). Implementations
// land incrementally; this test pins the gate ID vocabulary so CI docs and
// future policy scripts share one checklist.
var plannedHomologyGateIDs = []string{
	"HOM-P1", // schema parameters names ≡ cobra local flags
	"HOM-P2", // type/required/default ≡ Contract/cobra; hints cannot win those
	"HOM-P3", // schema constraints ≡ Contract Constraints projection
	"HOM-S1", // Risk write/high-risk → confirmation=user_required + help Safety
	"HOM-S2", // write-guard path must record honest provenance
	"HOM-S3", // read Risk must not project user_required without exclusion
	"HOM-I1", // interface_ref/bindings ⊆ Contract flags; MCP meta creates no flags
	"HOM-D1", // --help Flags ≡ schema leaf parameters
}

// ciHomologyEntrypoints are executable gates already wired into
// scripts/policy/check-schema-catalog.sh (and confirmation-truth helper).
// Vocabulary-only pinning is not enough: these names must stay on the policy
// -run whitelist.
var ciHomologyEntrypoints = []string{
	"TestUserRequiredSafetyHomologyWithRuntimeGate",
	"TestHomologyDecisionDocPinsPathAAndGateIDs",
	"TestMCPPassthroughAdmissionExcludesLeafAndShortcut",
	"TestHomologyCIEntrypointsPinned",
	"TestFinalSchemaParametersMatchExecutableHelpFlags",
	"TestDeliverySchemaParametersMatchExecutableHelpFlags",
	"TestSheetFinalSchemaConfirmationMatchesRuntimeGuards",
}

func TestHomologyDecisionDocPinsPathAAndGateIDs(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	doc := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "docs", "flag-help-schema-homology.md"))
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("read homology doc: %v", err)
	}
	body := string(raw)
	for _, needle := range []string{
		"路径 A",
		"Contract / LeafSpec 为 CLI 表面权威",
		"必须嵌入进 Schema",
		"embedContractIntoSchema",
		"声明 = 最终数据源",
		"不得：",
		"把声明序列化成 JSON 注解再解析",
		"声明体系里再挂「评审字段」并行权威",
		"声明（declare）：写什么、写在哪、投影到哪",
		"契约字段（算声明）",
		"编排 / 执行字段（不算声明）",
		"Schema 全覆盖（`ToolSpec` 无空洞）",
		"dws.schema.runtime_gate",
		"mcp_passthrough",
		"不得覆盖 LeafSpec / Shortcut 主路径",
		"已进 CI",
		"check-schema-catalog.sh",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("homology doc missing %q", needle)
		}
	}
	for _, id := range plannedHomologyGateIDs {
		if !strings.Contains(body, id) {
			t.Fatalf("homology doc missing planned gate ID %s", id)
		}
	}
}

func TestMCPPassthroughAdmissionExcludesLeafAndShortcut(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	doc := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "docs", "flag-help-schema-homology.md"))
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("read homology doc: %v", err)
	}
	body := string(raw)
	for _, needle := range []string{
		"严格 1:1",
		"全部 `LeafSpec` 命令",
		"全部 `+shortcut`",
		"surface_kind=mcp_passthrough",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("passthrough admission text missing %q", needle)
		}
	}
}

func TestHomologyCIEntrypointsPinned(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	script := filepath.Join(root, "scripts", "policy", "check-schema-catalog.sh")
	raw, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read check-schema-catalog.sh: %v", err)
	}
	body := string(raw)
	for _, name := range ciHomologyEntrypoints {
		if !strings.Contains(body, name) {
			t.Fatalf("check-schema-catalog.sh missing homology CI entrypoint %q", name)
		}
	}
	if !strings.Contains(body, "./internal/cli/homology") {
		t.Fatal("check-schema-catalog.sh must run ./internal/cli/homology for HOM gate tests")
	}
	doc := filepath.Join(root, "docs", "flag-help-schema-homology.md")
	docRaw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("read homology doc: %v", err)
	}
	docBody := string(docRaw)
	for _, name := range []string{
		"TestUserRequiredSafetyHomologyWithRuntimeGate",
		"TestFinalSchemaParametersMatchExecutableHelpFlags",
		"TestHomologyCIEntrypointsPinned",
	} {
		if !strings.Contains(docBody, name) {
			t.Fatalf("homology doc §3 CI table missing %q", name)
		}
	}
}

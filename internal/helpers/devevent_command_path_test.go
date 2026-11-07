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

package helpers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/spf13/cobra"
)

// devEventLeafExpectation 是一条叶子在 ContractFinal 上必须成立的全部事实。
type devEventLeafExpectation struct {
	use         string
	tool        string
	effect      string
	risk        string
	confirm     string
	idempotency string
	paginated   bool
	// local 为 true 时该叶子是纯本地执行：无 DryRun 预览语义、Interface.Mode
	// 为 local（lint 是唯一先例；其余叶子都是 composite 远端适配器）。
	local         bool
	requiredFlags []string
}

func devEventLeafExpectations() []devEventLeafExpectation {
	return []devEventLeafExpectation{
		{use: "list", tool: devEventListTool, effect: "read", risk: "low", confirm: "not_required", idempotency: "idempotent", paginated: true, requiredFlags: []string{"unified-app-id"}},
		{use: "get", tool: devEventGetTool, effect: "read", risk: "low", confirm: "not_required", idempotency: "idempotent", requiredFlags: []string{"unified-app-id", "event-key"}},
		{use: "create", tool: devEventCreateTool, effect: "write", risk: "high", confirm: "user_required", idempotency: "idempotent", requiredFlags: []string{"unified-app-id", "file"}},
		{use: "update", tool: devEventUpdateTool, effect: "write", risk: "high", confirm: "user_required", idempotency: "unknown", requiredFlags: []string{"unified-app-id", "event-key", "file", "expected-revision"}},
		{use: "validate", tool: devEventValidateTool, effect: "read", risk: "low", confirm: "not_required", idempotency: "idempotent", requiredFlags: []string{"unified-app-id", "event-key"}},
		{use: "lint", tool: devEventLintTool, effect: "read", risk: "low", confirm: "not_required", idempotency: "idempotent", local: true, requiredFlags: []string{"file"}},
		{use: "debug", tool: devEventDebugTool, effect: "write", risk: "high", confirm: "user_required", idempotency: "unknown", requiredFlags: []string{"unified-app-id", "event-key", "expected-revision"}},
		{use: "publish", tool: devEventPublishTool, effect: "write", risk: "high", confirm: "user_required", idempotency: "unknown", requiredFlags: []string{"unified-app-id", "event-key", "expected-revision"}},
		{use: "offline", tool: devEventOfflineTool, effect: "write", risk: "high", confirm: "user_required", idempotency: "unknown", requiredFlags: []string{"unified-app-id", "event-key", "expected-revision"}},
	}
}

func devEventContractFinal(t *testing.T, leaf *cobra.Command) contract.ContractFinalPayload {
	t.Helper()
	final, ok := contractfinal.RuntimeContractFinal(leaf)
	if !ok {
		t.Fatalf("leaf %q did not reach ContractFinal", leaf.Use)
	}
	return final
}

// TestDevEventLeavesReachContractFinal 是「声明即 Catalog」的正向断言：九条叶子
// 的 Identity、Safety、DryRun、Result、Pagination、Interface 与 Selection 都必须
// 完整落到 ContractFinal，任何一项缺席都会让 Agent 侧的 Schema 与路由失真。
func TestDevEventLeavesReachContractFinal(t *testing.T) {
	group := newDevEventCommand(&fakeDevEventRunner{})
	byUse := make(map[string]*cobra.Command, len(group.Commands()))
	for _, leaf := range group.Commands() {
		byUse[leaf.Use] = leaf
	}

	for _, want := range devEventLeafExpectations() {
		t.Run(want.use, func(t *testing.T) {
			leaf, ok := byUse[want.use]
			if !ok {
				t.Fatalf("dev event %s not mounted; got %v", want.use, keysOfCommandMap(byUse))
			}
			final := devEventContractFinal(t, leaf)

			identity := final.Identity
			if identity == nil {
				t.Fatal("Identity == nil")
			}
			if identity.ProductID != "dev" {
				t.Fatalf("ProductID = %q, want dev", identity.ProductID)
			}
			if identity.Name != want.tool {
				t.Fatalf("Name = %q, want %q", identity.Name, want.tool)
			}
			wantCanonical := "dev." + want.tool
			if identity.CanonicalPath != wantCanonical {
				t.Fatalf("CanonicalPath = %q, want %q", identity.CanonicalPath, wantCanonical)
			}
			wantCLIPath := "dev event " + want.use
			if identity.CLIPath != wantCLIPath || identity.PrimaryCLIPath != wantCLIPath {
				t.Fatalf("CLIPath = %q / PrimaryCLIPath = %q, want both %q", identity.CLIPath, identity.PrimaryCLIPath, wantCLIPath)
			}

			safety := final.Safety
			if safety == nil {
				t.Fatal("Safety == nil")
			}
			// 四个字段彼此独立，不允许由 effect 机械推导。
			if safety.Effect != want.effect || safety.Risk != want.risk ||
				safety.Confirmation != want.confirm || safety.Idempotency != want.idempotency {
				t.Fatalf("Safety = %#v, want effect=%s risk=%s confirmation=%s idempotency=%s",
					safety, want.effect, want.risk, want.confirm, want.idempotency)
			}

			dryRun := final.DryRun
			if want.local {
				if dryRun != nil {
					t.Fatalf("local leaf DryRun = %#v, want nil（本地执行没有远端预览语义）", dryRun)
				}
			} else {
				if dryRun == nil {
					t.Fatal("DryRun == nil")
				}
				if dryRun.PreviewKind != "invocation" || dryRun.RemoteReads {
					t.Fatalf("DryRun = %#v, want invocation preview without remote reads", dryRun)
				}
			}

			iface := final.Interface
			if iface == nil {
				t.Fatal("Interface == nil")
			}
			wantMode := contract.InterfaceModeComposite
			if want.local {
				wantMode = contract.InterfaceModeLocal
			}
			if iface.Mode != wantMode || iface.Availability != contract.InterfaceAvailable {
				t.Fatalf("Interface = %#v, want %s/available", iface, wantMode)
			}
			if strings.TrimSpace(iface.Reason) == "" {
				t.Fatal("Interface.Reason is empty; an unpinned remote adapter must state why")
			}

			result := final.Result
			if result == nil {
				t.Fatal("Result == nil")
			}
			wantOutcomes := []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}
			if len(result.Outcomes) != 2 || result.Outcomes[0] != wantOutcomes[0] || result.Outcomes[1] != wantOutcomes[1] {
				t.Fatalf("Outcomes = %#v, want %#v (this surface has no non-terminal pending)", result.Outcomes, wantOutcomes)
			}
			devEventAssertDataSchema(t, want.use, result.DataSchema)

			if want.paginated {
				pagination := final.Pagination
				if pagination == nil {
					t.Fatal("Pagination == nil, want cursor pagination on list")
				}
				if pagination.Kind != contract.PaginationKindCursor || pagination.CursorParameter != "cursor" {
					t.Fatalf("Pagination = %#v, want cursor/cursor", pagination)
				}
			} else if final.Pagination != nil {
				t.Fatalf("Pagination = %#v, want nil: only list declares paging", final.Pagination)
			}

			selection := final.Selection
			if selection == nil {
				t.Fatal("Selection == nil")
			}
			if strings.TrimSpace(selection.AgentSummary) == "" || len(selection.UseWhen) == 0 || len(selection.Examples) == 0 {
				t.Fatalf("Selection = %#v, want non-empty summary/useWhen/examples", selection)
			}
			// 三条路由边界是防混淆的硬要求：每条叶子都必须重复声明。
			for _, boundary := range devEventRoutingAvoidWhen {
				if !containsString(selection.AvoidWhen, boundary) {
					t.Fatalf("AvoidWhen = %#v, want containing %q", selection.AvoidWhen, boundary)
				}
			}
			// AvoidWhen 的第一项是叶子特有边界，不能被路由边界挤掉。
			if len(selection.AvoidWhen) != len(devEventRoutingAvoidWhen)+1 {
				t.Fatalf("AvoidWhen has %d entries, want %d (one leaf-specific plus three routing boundaries)",
					len(selection.AvoidWhen), len(devEventRoutingAvoidWhen)+1)
			}

			// 必填事实的 Schema 出口是 flag 上的 dws.schema.required 注解
			// （corecmd 从 FlagSpec.Required 投影，Agent Runtime Schema 读它），
			// 不是 ContractFinal.Parameters —— 后者是可选的补充声明面。
			for _, name := range want.requiredFlags {
				flag := leaf.Flags().Lookup(name)
				if flag == nil {
					t.Fatalf("flag --%s not registered", name)
				}
				if got := flag.Annotations[runtimeannotate.AnnotationFlagRequired]; len(got) != 1 || got[0] != "true" {
					t.Fatalf("--%s required annotation = %v, want [true]: 必填必须投影到 Runtime Schema", name, got)
				}
				if got := flag.Annotations[runtimeannotate.AnnotationFlagProperty]; len(got) != 1 || strings.TrimSpace(got[0]) == "" {
					t.Fatalf("--%s property binding = %v, want exactly one non-empty interface property", name, got)
				}
				for _, param := range final.Parameters {
					if param.Name == name && param.Required != nil && !*param.Required {
						t.Fatalf("ContractFinal declares --%s required=false, contradicting the flag", name)
					}
				}
			}
			if strings.TrimSpace(final.Description) == "" {
				t.Fatal("Description is empty")
			}
		})
	}
}

// devEventAssertDataSchema 校验 Result.DataSchema 是可解析的 JSON 对象，且每个
// 命名 properties 子项都带非空 description —— Agent 只能靠这些描述理解字段。
func devEventAssertDataSchema(t *testing.T, use string, raw json.RawMessage) {
	t.Helper()
	if len(raw) == 0 {
		t.Fatalf("%s: DataSchema is empty", use)
	}
	var schema struct {
		Type        string `json:"type"`
		Description string `json:"description"`
		Properties  map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("%s: DataSchema is not parseable: %v", use, err)
	}
	if schema.Type != "object" || strings.TrimSpace(schema.Description) == "" {
		t.Fatalf("%s: DataSchema = %#v, want an object with a description", use, schema)
	}
	if len(schema.Properties) == 0 {
		t.Fatalf("%s: DataSchema declares no properties", use)
	}
	for name, property := range schema.Properties {
		if strings.TrimSpace(property.Type) == "" {
			t.Fatalf("%s: property %q has no type", use, name)
		}
		if strings.TrimSpace(property.Description) == "" {
			t.Fatalf("%s: property %q has no description", use, name)
		}
	}
}

func keysOfCommandMap(in map[string]*cobra.Command) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	return out
}

// TestDevEventCanonicalPathsAppearInSchemaDump 是反向完整性断言：九条叶子必须
// 全部出现在 dev schema --all 的装配 dump 中，且不需要任何新的排除项。
// 缺席意味着声明没有真正进入 Catalog，Agent 侧看不到这批命令。
func TestDevEventCanonicalPathsAppearInSchemaDump(t *testing.T) {
	dump := assembleDevSchemaAllDump(t)
	observed := map[string]string{}
	found := false
	for _, product := range dump.Products {
		if product.ID != "dev" {
			continue
		}
		found = true
		for _, tool := range product.Tools {
			observed[tool.CanonicalPath] = tool.CLIPath
		}
	}
	if !found {
		t.Fatal("dev product not found in schema --all dump")
	}
	for _, want := range devEventLeafExpectations() {
		canonical := "dev." + want.tool
		cliPath, ok := observed[canonical]
		if !ok {
			t.Errorf("schema --all dump missing canonical tool %q", canonical)
			continue
		}
		if wantCLIPath := "dev event " + want.use; cliPath != wantCLIPath {
			t.Errorf("canonical %q cli_path = %q, want %q", canonical, cliPath, wantCLIPath)
		}
	}
}

// TestDevEventRevisionConstraintIsDeclared 验证乐观锁规则被投影到 --help 与
// Runtime Schema：Custom 约束只负责声明，实际拦截由 Validate 钩子执行。
func TestDevEventRevisionConstraintIsDeclared(t *testing.T) {
	constraint := devEventRevisionConstraint()
	if constraint.Kind != "custom" || len(constraint.Flags) != 1 || constraint.Flags[0] != "expected-revision" {
		t.Fatalf("constraint = %#v, want custom on expected-revision", constraint)
	}
	if !strings.Contains(constraint.Description, "≥ 1") {
		t.Fatalf("constraint description = %q, want it to state the ≥ 1 rule", constraint.Description)
	}

	for _, use := range []string{"update", "debug", "publish", "offline"} {
		group := newDevEventCommand(&fakeDevEventRunner{})
		var leaf *cobra.Command
		for _, candidate := range group.Commands() {
			if candidate.Use == use {
				leaf = candidate
			}
		}
		if leaf == nil {
			t.Fatalf("dev event %s not mounted", use)
		}
		flag := leaf.Flags().Lookup("expected-revision")
		if flag == nil {
			t.Fatalf("%s: --expected-revision not registered", use)
		}
		// 刻意不设默认值：缺省必须被必填校验拦下，不能静默按 0/1 提交。
		if flag.DefValue != "" && flag.DefValue != "0" {
			t.Fatalf("%s: --expected-revision default = %q, want none", use, flag.DefValue)
		}
		if !strings.Contains(flag.Usage, "≥1") {
			t.Fatalf("%s: --expected-revision usage = %q, want it to state ≥1", use, flag.Usage)
		}
	}
}

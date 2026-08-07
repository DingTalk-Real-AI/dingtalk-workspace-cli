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
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
)

// collectDevLeafCommands 递归收集命令树中所有叶子命令（无子命令者）。
func collectDevLeafCommands(cmd *cobra.Command, out *[]*cobra.Command) {
	if cmd == nil {
		return
	}
	if len(cmd.Commands()) == 0 {
		*out = append(*out, cmd)
		return
	}
	for _, sub := range cmd.Commands() {
		collectDevLeafCommands(sub, out)
	}
}

func TestDevRepresentativeResultContractsReachContractFinal(t *testing.T) {
	root := newDevAppTestRoot(&captureRunner{})
	var leaves []*cobra.Command
	collectDevLeafCommands(root, &leaves)
	finals := make(map[string]contract.ContractFinalPayload)
	for _, leaf := range leaves {
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if ok && final.Identity != nil {
			finals[final.Identity.CanonicalPath] = final
		}
	}

	tests := []struct {
		canonical string
		outcomes  []contract.ResultOutcome
		check     func(*testing.T, *contract.ResultSpec)
	}{
		{
			canonical: "dev.list_dev_app",
			outcomes:  []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			check: func(t *testing.T, result *contract.ResultSpec) {
				if result.NDJSON == nil || result.NDJSON.RecordPath != "items" || result.Pagination == nil ||
					result.Pagination.CursorPath != "nextCursor" || result.Pagination.ExhaustionPath != "hasMore" || result.Pagination.ExhaustedWhen {
					t.Fatalf("paginated list result = %#v", result)
				}
			},
		},
		{canonical: "dev.get_dev_app", outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}},
		{
			canonical: "dev.get_dev_app_credentials",
			outcomes:  []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			check: func(t *testing.T, result *contract.ResultSpec) {
				if want := []string{"appSecret", "clientSecret"}; !reflect.DeepEqual(result.SensitivePaths, want) {
					t.Fatalf("sensitive paths = %#v, want %#v", result.SensitivePaths, want)
				}
			},
		},
		{canonical: "dev.get_dev_app_version_status", outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePending, contract.ResultOutcomeFailure}},
		{canonical: "dev.connect_status", outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}},
		{canonical: "dev.connect_list", outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}},
	}
	for _, test := range tests {
		t.Run(test.canonical, func(t *testing.T) {
			final, ok := finals[test.canonical]
			if !ok || final.Result == nil {
				t.Fatalf("representative leaf %s has no final Result", test.canonical)
			}
			if !reflect.DeepEqual(final.Result.Outcomes, test.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v", final.Result.Outcomes, test.outcomes)
			}
			if len(final.Result.DataSchema) == 0 {
				t.Fatal("data_schema is empty")
			}
			if test.check != nil {
				test.check(t, final.Result)
			}
		})
	}
}

// TestDevLeafDeclarationCompleteness 是队列 B190 的 dev 叶子声明完整性自查：
// 每一个**注册了 ContractFinal** 的 dev 叶子都必须满足 Identity+Safety 最小集
// （无半壳：有注册但字段缺失）。对未注册 ContractFinal 的叶子（裸 Cobra 迁移态
// 或 NewLeafCommand 漏声明）不硬失败，而是汇总为 findings 清单打印，供人工判断
// 是否属「空壳入册」。
func TestDevLeafDeclarationCompleteness(t *testing.T) {
	root := newDevAppTestRoot(&captureRunner{})
	var leaves []*cobra.Command
	collectDevLeafCommands(root, &leaves)

	if len(leaves) == 0 {
		t.Fatalf("no dev leaves collected under test root")
	}
	t.Logf("dev domain leaves under test: %d", len(leaves))

	var noFinal []string
	for _, leaf := range leaves {
		path := leaf.CommandPath()
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok {
			noFinal = append(noFinal, path)
			continue
		}
		t.Run(path, func(t *testing.T) {
			if final.Identity == nil {
				t.Fatalf("leaf %q ContractFinal.Identity is nil", path)
			}
			if final.Identity.ProductID == "" {
				t.Fatalf("leaf %q Identity.ProductID empty", path)
			}
			if final.Identity.Name == "" {
				t.Fatalf("leaf %q Identity.Name empty", path)
			}
			if final.Identity.CanonicalPath == "" {
				t.Fatalf("leaf %q Identity.CanonicalPath empty", path)
			}
			if final.Safety == nil {
				t.Fatalf("leaf %q ContractFinal.Safety is nil (Safety minimal set missing)", path)
			}
			if final.Description == "" {
				t.Fatalf("leaf %q ContractFinal.Description empty (no description declared)", path)
			}
		})
	}

	if len(noFinal) > 0 {
		sort.Strings(noFinal)
		t.Logf("B190 findings: %d dev leaves without ContractFinal (may be intentional bare-Cobra migration or empty-shell NewLeafCommand):", len(noFinal))
		for _, p := range noFinal {
			t.Logf("  - %s", p)
		}
	}
}

// TestDevLeafDeclarationNoEmptyShellNewLeaf 是队列 B190 的核心断言：NewLeafCommand
// 完全托管的叶子必须注册 ContractFinal——声明了 LeafSpec 却无 ContractFinal 就是
// 「空壳入册」。本测试通过 Verify 一个已知的既存漏洞（version check-approval 用
// NewLeafCommand 但 LeafSpec 未声明 Contract）来固化当前现状，并断言这类「声明了
// Shell 却无最终契约」的问题在受控已知集中，而非蔓延到其它托管叶子。
func TestDevLeafDeclarationNoEmptyShellNewLeaf(t *testing.T) {
	root := newDevAppTestRoot(&captureRunner{})
	var leaves []*cobra.Command
	collectDevLeafCommands(root, &leaves)

	knownEmptyShell := map[string]bool{}

	var unexpected []string
	for _, leaf := range leaves {
		path := leaf.CommandPath()
		if _, ok := contractfinal.RuntimeContractFinal(leaf); ok {
			continue // 正常托管叶子已有 ContractFinal。
		}
		// 无 ContractFinal：裸 Cobra 迁移态（connect 系列）或已知空壳。
		if knownEmptyShell[path] {
			continue
		}
		// 裸 Cobra 命令（不经 NewLeafCommand）不要求 ContractFinal，排除。
		if isBareDevCobraLeaf(leaf) {
			continue
		}
		unexpected = append(unexpected, path)
	}
	if len(unexpected) > 0 {
		t.Fatalf("unexpected empty-shell NewLeafCommand leaves without ContractFinal: %v", unexpected)
	}
}

// isBareDevCobraLeaf 判断某 dev 叶子是否为裸 Cobra 命令（不经 NewLeafCommand，
// 不注册 ContractFinal）。通过多次构造命令并检查其是否有本地 flags 来间接识别
// 不可靠，改用命令名白名单：connect 本地运维子命令为已知裸 Cobra 迁移态。
func isBareDevCobraLeaf(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "list", "status", "restart", "stop":
		// 仅当命令位于 dev connect 子树下才算裸 Cobra（避免误伤 dev app 的
		// version list 等托管叶子）。
		if strings.Contains(cmd.CommandPath(), "dev connect") {
			return true
		}
	}
	return false
}

var (
	_ = contractfinal.HasRuntimeContractFinal
	_ = sort.Strings
)

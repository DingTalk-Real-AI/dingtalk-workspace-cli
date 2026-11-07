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

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

// waitPollCommandTestRegistry builds one registry carrying a wait-declaring
// tool (sample.waitrun) plus a read poll target (oa.approval_instance_get).
func waitPollCommandTestRegistry(t *testing.T, pollCommand string) SchemaRegistry {
	t.Helper()
	registry := schemaDeliveryTestRegistry(
		schemaDeliveryTestTool{Canonical: "sample.waitrun", CLIPath: "sample waitrun"},
		schemaDeliveryTestTool{Canonical: "oa.approval_instance_get", CLIPath: "oa approval-instance get"},
	)
	waitTool := waitPollCommandTestTool(t, &registry, "sample.waitrun")
	waitTool.Wait = &contract.WaitSpec{
		Mode:          contract.WaitModePoll,
		PollCommand:   pollCommand,
		StatusQuery:   "result.status",
		Terminal:      map[string]contract.ResultOutcome{"COMPLETED": contract.ResultOutcomeSuccess},
		PendingValues: []string{"RUNNING"},
	}
	setWaitPollCommandTestEffect(t, &registry, "oa.approval_instance_get", "read")
	return registry
}

// waitPollCommandTestTool locates one mutable tool by canonical path.
func waitPollCommandTestTool(t *testing.T, registry *SchemaRegistry, canonical string) *ToolSpec {
	t.Helper()
	for productIndex := range registry.Products {
		for toolIndex := range registry.Products[productIndex].Tools {
			tool := &registry.Products[productIndex].Tools[toolIndex]
			if tool.Identity.CanonicalPath == canonical {
				return tool
			}
		}
	}
	t.Fatalf("tool %s missing from test registry", canonical)
	return nil
}

// setWaitPollCommandTestEffect rewrites the delivered safety effect together
// with its provenance winner so Index() revalidation stays consistent.
func setWaitPollCommandTestEffect(t *testing.T, registry *SchemaRegistry, canonical, effect string) {
	t.Helper()
	tool := waitPollCommandTestTool(t, registry, canonical)
	tool.Safety.Effect = effect
	encoded, err := json.Marshal(effect)
	if err != nil {
		t.Fatalf("encode effect provenance: %v", err)
	}
	provenance := tool.FieldProvenance["effect"]
	provenance.Value = encoded
	for i := range provenance.Candidates {
		provenance.Candidates[i].Value = append(json.RawMessage(nil), encoded...)
	}
	tool.FieldProvenance["effect"] = provenance
}

func waitPollCommandTestBound(visibility SchemaVisibility) BoundCommandRegistry {
	target := CommandSpec{
		CanonicalPath:  "oa.approval_instance_get",
		PrimaryCLIPath: "oa approval-instance get",
		Visibility:     visibility,
	}
	boundTarget := BoundCommandSpec{CommandSpec: target}
	return BoundCommandRegistry{
		Commands:    []BoundCommandSpec{boundTarget},
		ByCanonical: map[string]BoundCommandSpec{target.CanonicalPath: boundTarget},
		ByCLIPath:   map[string]BoundCommandSpec{target.PrimaryCLIPath: boundTarget},
	}
}

func TestValidateWaitPollCommandsAcceptsResolvedReadTarget(t *testing.T) {
	for _, pollCommand := range []string{
		"oa.approval_instance_get",
		"oa approval-instance get",
	} {
		registry := waitPollCommandTestRegistry(t, pollCommand)
		if err := validateWaitPollCommands(registry, waitPollCommandTestBound(SchemaVisibilityPublic)); err != nil {
			t.Fatalf("validateWaitPollCommands(%q) error = %v", pollCommand, err)
		}
	}
}

func TestValidateWaitPollCommandsRejectsMissingTarget(t *testing.T) {
	registry := waitPollCommandTestRegistry(t, "oa.typo_get")
	err := validateWaitPollCommands(registry, waitPollCommandTestBound(SchemaVisibilityPublic))
	if err == nil || !strings.Contains(err.Error(), "does not resolve to a bound catalog command") {
		t.Fatalf("missing poll target error = %v", err)
	}
}

func TestValidateWaitPollCommandsRejectsExcludedTarget(t *testing.T) {
	registry := waitPollCommandTestRegistry(t, "oa.approval_instance_get")
	err := validateWaitPollCommands(registry, waitPollCommandTestBound(SchemaVisibilityInternal))
	if err == nil || !strings.Contains(err.Error(), "non-public command") {
		t.Fatalf("excluded poll target error = %v", err)
	}
}

func TestValidateWaitPollCommandsRejectsUndeliveredTarget(t *testing.T) {
	registry := waitPollCommandTestRegistry(t, "oa.approval_instance_get")
	// Public and bound, but absent from the delivered Schema (as if delivery
	// dropped the product): the manual resume path would point agents nowhere.
	for productIndex, product := range registry.Products {
		if product.ID == "oa" {
			registry.Products = append(registry.Products[:productIndex], registry.Products[productIndex+1:]...)
			break
		}
	}
	err := validateWaitPollCommands(registry, waitPollCommandTestBound(SchemaVisibilityPublic))
	if err == nil || !strings.Contains(err.Error(), "missing from the delivered Schema") {
		t.Fatalf("undelivered poll target error = %v", err)
	}
}

func TestValidateWaitPollCommandsRejectsNonReadTarget(t *testing.T) {
	registry := waitPollCommandTestRegistry(t, "oa.approval_instance_get")
	setWaitPollCommandTestEffect(t, &registry, "oa.approval_instance_get", "write")

	err := validateWaitPollCommands(registry, waitPollCommandTestBound(SchemaVisibilityPublic))
	if err == nil || !strings.Contains(err.Error(), "must be a read command") {
		t.Fatalf("non-read poll target error = %v", err)
	}
}

func TestValidateWaitPollCommandsIgnoresEventModeWithoutPoll(t *testing.T) {
	registry := waitPollCommandTestRegistry(t, "")
	waitPollCommandTestTool(t, &registry, "sample.waitrun").Wait = &contract.WaitSpec{
		Mode:          contract.WaitModeEvent,
		EventKey:      "approval.events",
		MatchField:    "instance_id",
		ResourceQuery: "id",
		StatusQuery:   "result.status",
		Terminal:      map[string]contract.ResultOutcome{"COMPLETED": contract.ResultOutcomeSuccess},
	}
	if err := validateWaitPollCommands(registry, waitPollCommandTestBound(SchemaVisibilityPublic)); err != nil {
		t.Fatalf("event mode must not require poll_command resolution: %v", err)
	}
}

func TestValidateWaitPollCommandsPropagatesIndexErrors(t *testing.T) {
	// A registry Index() cannot build (duplicate product) must surface as a
	// validation error instead of being silently skipped.
	registry := SchemaRegistry{Products: []ProductSpec{{ID: "dup"}, {ID: "dup"}}}
	err := validateWaitPollCommands(registry, BoundCommandRegistry{})
	if err == nil || !strings.Contains(err.Error(), "duplicate schema product") {
		t.Fatalf("index failure must propagate, got %v", err)
	}
}

// assembleWaitPollTestRoot builds a production-shaped tree: one wait-declaring
// leaf plus its poll target under a registered synthetic product.
func assembleWaitPollTestRoot(t *testing.T, pollCommand string) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "dws"}
	product := &cobra.Command{Use: "waitqa"}
	root.AddCommand(product)

	waitLeaf := &cobra.Command{Use: "launch", Short: "Launch", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(waitLeaf, "waitqa", "launch", "test")
	contractfinal.RegisterRuntimeContractFinal(waitLeaf, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "waitqa", Name: "launch", CanonicalPath: "waitqa.launch",
			CLIPath: "waitqa launch", PrimaryCLIPath: "waitqa launch",
		},
		Title:       "Launch",
		Description: "Async launch",
		Safety:      &contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required"},
		Wait: &contract.WaitSpec{
			Mode:          contract.WaitModePoll,
			PollCommand:   pollCommand,
			StatusQuery:   "result.status",
			Terminal:      map[string]contract.ResultOutcome{"COMPLETED": contract.ResultOutcomeSuccess},
			PendingValues: []string{"RUNNING"},
		},
		Selection: &contract.SelectionSpec{AgentSummary: "launch", UseWhen: []string{"launch"}},
	})
	product.AddCommand(waitLeaf)

	statusLeaf := &cobra.Command{Use: "status", Short: "Status", Run: func(*cobra.Command, []string) {}}
	AttachRuntimeSchema(statusLeaf, "waitqa", "status", "test")
	contractfinal.RegisterRuntimeContractFinal(statusLeaf, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{
			ProductID: "waitqa", Name: "status", CanonicalPath: "waitqa.status",
			CLIPath: "waitqa status", PrimaryCLIPath: "waitqa status",
		},
		Title:       "Status",
		Description: "Read status",
		Safety:      &contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "none"},
		Selection:   &contract.SelectionSpec{AgentSummary: "status", UseWhen: []string{"status"}},
	})
	product.AddCommand(statusLeaf)

	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "waitqa",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "Wait QA product",
			UseWhen:      []string{"waitqa routing"},
			AvoidWhen:    []string{"not waitqa"},
		},
	})
	t.Cleanup(func() {
		contractfinal.ClearRuntimeContractFinalForTest(waitLeaf)
		contractfinal.ClearRuntimeContractFinalForTest(statusLeaf)
		contract.ClearProductDeclForTest("waitqa")
	})
	return root
}

func TestAssembleSchemaRegistryRejectsUnresolvableWaitPollCommand(t *testing.T) {
	root := assembleWaitPollTestRoot(t, "waitqa.typo_get")
	_, err := schemaRegistryForTest(root)
	if err == nil || !strings.Contains(err.Error(), "does not resolve to a bound catalog command") {
		t.Fatalf("assembly must reject an unresolvable poll_command, got %v", err)
	}
}

func TestAssembleSchemaRegistryAcceptsResolvedWaitPollCommand(t *testing.T) {
	root := assembleWaitPollTestRoot(t, "waitqa.status")
	registry, err := schemaRegistryForTest(root)
	if err != nil {
		t.Fatalf("assembly with a resolved poll target failed: %v", err)
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := index.Resolve("waitqa.launch")
	if !ok || tool.Wait == nil || tool.Wait.PollCommand != "waitqa.status" {
		t.Fatalf("wait capability must survive assembly, tool=%#v ok=%v", tool.Wait, ok)
	}
}

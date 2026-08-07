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

package contractfinal

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func TestContractFinalTypedRegistryNoJSON(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(cmd) })
	result := &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeSuccess},
		DataSchema:     []byte(`{"type":"object"}`),
		SensitivePaths: []string{"token"},
	}

	RegisterRuntimeContractFinal(cmd, contract.ContractFinalPayload{
		Title: "T",
		Safety: &contract.SafetySpec{
			Effect: "write", Confirmation: "user_required", Idempotency: "retryable",
		},
		Selection: &contract.SelectionSpec{AgentSummary: "sum", UseWhen: []string{"u"}},
		Identity:  &contract.ToolIdentitySpec{ProductID: "p", Name: "n"},
		Result:    result,
	})
	result.Outcomes[0] = contract.ResultOutcomeFailure
	result.DataSchema[0] = '['
	result.SensitivePaths[0] = "changed"
	if cmd.Annotations != nil {
		if _, ok := cmd.Annotations["dws.schema.final"]; ok {
			t.Fatal("must not write JSON annotation dws.schema.final")
		}
	}
	got, ok := RuntimeContractFinal(cmd)
	if !ok || got.Title != "T" || got.Safety == nil || got.Safety.Idempotency != "retryable" {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	if got.Selection == nil || got.Selection.Reviewed != nil {
		t.Fatalf("selection must not carry reviewed fields: %#v", got.Selection)
	}
	if got.Result == nil || got.Result.Outcomes[0] != contract.ResultOutcomeSuccess || got.Result.DataSchema[0] != '{' || got.Result.SensitivePaths[0] != "token" {
		t.Fatalf("stored result aliases registration input: %#v", got.Result)
	}
	got.Result.Outcomes[0] = contract.ResultOutcomeFailure
	again, _ := RuntimeContractFinal(cmd)
	if again.Result.Outcomes[0] != contract.ResultOutcomeSuccess {
		t.Fatal("RuntimeContractFinal result aliases stored payload")
	}
}

func TestCrossPlatformCoverageContractFinalNilCommandGuards(t *testing.T) {
	// Nil command registration/lookup must be inert no-ops.
	RegisterRuntimeContractFinal(nil, contract.ContractFinalPayload{Title: "ignored"})
	if _, ok := RuntimeContractFinal(nil); ok {
		t.Fatal("RuntimeContractFinal(nil) must report no payload")
	}
	if HasRuntimeContractFinal(nil) {
		t.Fatal("HasRuntimeContractFinal(nil) must be false")
	}
	registered := &cobra.Command{Use: "registered"}
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(registered) })
	RegisterRuntimeContractFinal(registered, contract.ContractFinalPayload{Title: "T"})
	if !HasRuntimeContractFinal(registered) {
		t.Fatal("HasRuntimeContractFinal must be true after registration")
	}
	ClearRuntimeContractFinalForTest(nil)
}

func TestCrossPlatformCoverageApplyParamDeclsSkipsBlankAndAnnotatesEnum(t *testing.T) {
	cmd := &cobra.Command{Use: "apply-params"}
	cmd.Flags().String("mode", "", "mode")
	required := false
	if err := ApplyParamDecls(cmd, []contract.ParamDecl{
		{Name: "  "}, // blank names are skipped
		{
			Name: "mode", Property: "mode", Required: &required,
			InterfaceType: "string", Description: "mode desc",
			RequiredWhen: "when create", Enum: []string{"a", "b"},
		},
	}); err != nil {
		t.Fatalf("ApplyParamDecls() error = %v", err)
	}
	flag := cmd.Flags().Lookup("mode")
	if flag == nil {
		t.Fatal("missing mode flag")
	}
	if got := flag.Annotations["dws.schema.property"]; len(got) == 0 || got[0] != "mode" {
		t.Fatalf("property = %#v", flag.Annotations["dws.schema.property"])
	}
	if got := flag.Annotations["x-cli-enum"]; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("enum = %#v", flag.Annotations["x-cli-enum"])
	}
	if err := ApplyParamDecls(nil, []contract.ParamDecl{{Name: "mode"}}); err != nil {
		t.Fatalf("ApplyParamDecls(nil) error = %v", err)
	}
	if err := ApplyParamDecls(cmd, nil); err != nil {
		t.Fatalf("ApplyParamDecls(nil decls) error = %v", err)
	}
}

func TestCrossPlatformCoverageApplyParamDeclsRejectsUnknownFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "apply-params"}
	cmd.Flags().String("mode", "", "mode")
	err := ApplyParamDecls(cmd, []contract.ParamDecl{
		{Name: "mode", Property: "mode"},
		{Name: "missing-flag", Property: "missing"},
	})
	if err == nil {
		t.Fatal("ApplyParamDecls() error = nil, want unknown flag")
	}
	if !strings.Contains(err.Error(), "missing-flag") {
		t.Fatalf("ApplyParamDecls() error = %v, want missing-flag", err)
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("ApplyParamDecls() error = %v, want unknown flag", err)
	}
	flag := cmd.Flags().Lookup("mode")
	if flag == nil {
		t.Fatal("missing mode flag")
	}
	if got := flag.Annotations["dws.schema.property"]; len(got) != 0 {
		t.Fatalf("fail-closed must not annotate before unknown ParamDecl; property = %#v", got)
	}
}

func TestCrossPlatformCoverageRuntimeContractFinalRejectsForeignStoredValue(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(cmd) })

	// Defensive branch: a stored value that is not a *contract.ContractFinalPayload
	// (or a typed nil) must fail the read instead of panicking.
	StoreRuntimeContractFinalRawForTest(cmd, "not-a-payload")
	if _, ok := RuntimeContractFinal(cmd); ok {
		t.Fatal("foreign stored value must not decode as contract.ContractFinalPayload")
	}
	StoreRuntimeContractFinalRawForTest(cmd, (*contract.ContractFinalPayload)(nil))
	if _, ok := RuntimeContractFinal(cmd); ok {
		t.Fatal("typed nil payload must not decode as contract.ContractFinalPayload")
	}
}

func TestResolveRuntimeSafetyUsesCanonicalOrCLIIdentityAndRejectsUnavailable(t *testing.T) {
	read := &cobra.Command{Use: "read"}
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(read) })
	RegisterRuntimeContractFinal(read, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{CanonicalPath: "sample.read", PrimaryCLIPath: "sample get"},
		Safety:   &contract.SafetySpec{Effect: " read ", Idempotency: " idempotent "},
	})

	for _, lookup := range []struct {
		canonical string
		cli       string
	}{
		{canonical: "sample.read"},
		{canonical: "different.rpc", cli: "dws sample get"},
	} {
		safety, declared, ok := ResolveRuntimeSafety(lookup.canonical, lookup.cli)
		if !declared || !ok || safety.Effect != "read" || safety.Idempotency != "idempotent" {
			t.Fatalf("ResolveRuntimeSafety(%q, %q) = %#v, %v, %v", lookup.canonical, lookup.cli, safety, declared, ok)
		}
	}

	missingSafety := &cobra.Command{Use: "write"}
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(missingSafety) })
	RegisterRuntimeContractFinal(missingSafety, contract.ContractFinalPayload{
		Identity: &contract.ToolIdentitySpec{CanonicalPath: "sample.write"},
	})
	if _, declared, ok := ResolveRuntimeSafety("sample.write", ""); !declared || ok {
		t.Fatalf("missing safety = declared %v ok %v, want true false", declared, ok)
	}
	if _, declared, ok := ResolveRuntimeSafety("legacy.call", "legacy call"); declared || ok {
		t.Fatalf("legacy lookup = declared %v ok %v, want false false", declared, ok)
	}
}

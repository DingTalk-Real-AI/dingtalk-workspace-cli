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
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
)

func TestCrossPlatformCoverageHasDeclaredOrAnnotatedConfirmationBranches(t *testing.T) {
	bare := &cobra.Command{Use: "bare"}
	if HasDeclaredOrAnnotatedConfirmation(bare) {
		t.Fatal("bare command must not claim confirmation coverage")
	}

	gated := &cobra.Command{Use: "gated"}
	gated.Annotations = map[string]string{runtimeannotate.AnnotationRuntimeGate: "devAppRequireWriteGuard"}
	if !HasDeclaredOrAnnotatedConfirmation(gated) {
		t.Fatal("annotated gate must satisfy declare-OR-annotate")
	}

	declared := &cobra.Command{Use: "declared"}
	t.Cleanup(func() { ClearRuntimeContractFinalForTest(declared) })
	RegisterRuntimeContractFinal(declared, contract.ContractFinalPayload{
		Safety: &contract.SafetySpec{Confirmation: "not_required"},
	})
	if !HasDeclaredOrAnnotatedConfirmation(declared) {
		t.Fatal("typed Contract SafetySpec confirmation must satisfy declare-OR-annotate")
	}

	risky := &cobra.Command{Use: "risky"}
	risky.Annotations = map[string]string{runtimeannotate.AnnotationRisk: "read"}
	if !HasDeclaredOrAnnotatedConfirmation(risky) {
		t.Fatal("Contract Risk annotation must satisfy declare-OR-annotate")
	}
}

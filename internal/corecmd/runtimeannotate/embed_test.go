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

package runtimeannotate

import (
	"testing"

	"github.com/spf13/cobra"
)

// The Risk/Gate writers are retired; assembly still reads the residual
// annotations through the accessors as a ContractFinal-less fallback, so the
// accessors keep coverage with directly constructed annotation maps.

func TestCrossPlatformCoverageRuntimeContractRiskAccessor(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Annotations: map[string]string{
		AnnotationRisk: "write",
	}}
	got, ok := RuntimeContractRisk(cmd)
	if !ok || got != "write" {
		t.Fatalf("RuntimeContractRisk = %q %v", got, ok)
	}
	if _, ok := RuntimeContractRisk(nil); ok {
		t.Fatal("RuntimeContractRisk(nil) must miss")
	}
	if _, ok := RuntimeContractRisk(&cobra.Command{}); ok {
		t.Fatal("RuntimeContractRisk empty annotations must miss")
	}
	blank := &cobra.Command{Use: "y", Annotations: map[string]string{AnnotationRisk: "   "}}
	if _, ok := RuntimeContractRisk(blank); ok {
		t.Fatal("blank risk annotation must not report a risk")
	}
}

func TestCrossPlatformCoverageRuntimeContractGateAccessor(t *testing.T) {
	cmd := &cobra.Command{Use: "x", Annotations: map[string]string{
		AnnotationRuntimeGate: "devAppRequireWriteGuard",
	}}
	got, ok := RuntimeContractGate(cmd)
	if !ok || got != "devAppRequireWriteGuard" {
		t.Fatalf("RuntimeContractGate = %q %v", got, ok)
	}
	if _, ok := RuntimeContractGate(nil); ok {
		t.Fatal("RuntimeContractGate(nil) must miss")
	}
	blank := &cobra.Command{Use: "y", Annotations: map[string]string{
		AnnotationRuntimeGate: "   ",
	}}
	if _, ok := RuntimeContractGate(blank); ok {
		t.Fatal("blank runtime_gate annotation must not report a gate")
	}
}

func TestCrossPlatformCoverageRuntimeContractAnnotationNilGuards(t *testing.T) {
	AnnotateRuntimeFlagDescription(nil, "flag", "description")
	AnnotateRuntimeContract(nil)
}

func TestSetCommandAnnotationInitializesNilMap(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	SetCommandAnnotation(cmd, "key", "value")
	if cmd.Annotations == nil || cmd.Annotations["key"] != "value" {
		t.Fatalf("annotations = %#v", cmd.Annotations)
	}
}

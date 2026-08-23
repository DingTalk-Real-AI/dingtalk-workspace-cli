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
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// contractCoverageSchema returns a minimal LeafContract accepted by
// corecmd.AttachContract's declaration completeness rules (test fixture).
func contractCoverageSchema(desc string) LeafContract {
	return LeafContract{
		Identity: contract.ToolIdentitySpec{
			ProductID: "dev", Name: "create_thing", CanonicalPath: "dev.create_thing",
			CLIPath: "dev create", PrimaryCLIPath: "dev create",
		},
		Description: desc,
		Interface: &contract.InterfaceSpec{
			Mode:         "composite",
			Availability: "available",
			Reason:       "unit test fixture",
		},
		Selection: contract.SelectionSpec{
			AgentSummary: desc,
			UseWhen:      []string{"unit test"},
			AvoidWhen:    []string{"unit test"},
			Examples:     []string{"dws test"},
		},
	}
}

// TestCrossPlatformCoverageContractRuntimeStoreLoadNilGuards covers the nil/miss guards of the
// contractRuntime registry (store nil args, load nil cmd, load miss) and the
// ContractValidate not-registered fallback.
func TestCrossPlatformCoverageContractRuntimeStoreLoadNilGuards(t *testing.T) {
	// Nil guards: none of these may panic or register anything.
	storeContractRuntime(nil, &contractRuntime{})
	storeContractRuntime(&cobra.Command{Use: "nil-rt"}, nil)
	storeContractRuntime(nil, nil)

	if rt, ok := loadContractRuntime(nil); ok || rt != nil {
		t.Fatalf("loadContractRuntime(nil) = (%v, %v), want (nil, false)", rt, ok)
	}
	fresh := &cobra.Command{Use: "fresh"}
	if rt, ok := loadContractRuntime(fresh); ok || rt != nil {
		t.Fatalf("loadContractRuntime(fresh) = (%v, %v), want (nil, false)", rt, ok)
	}
	if ContractValidate(fresh) != nil {
		t.Fatal("ContractValidate on unregistered command must return nil")
	}
	if ContractValidate(nil) != nil {
		t.Fatal("ContractValidate(nil) must return nil")
	}

	// Positive path: stored runtime is returned as-is.
	stored := &contractRuntime{validate: func(*cobra.Command, []string) error { return nil }}
	cmd := &cobra.Command{Use: "stored"}
	storeContractRuntime(cmd, stored)
	rt, ok := loadContractRuntime(cmd)
	if !ok || rt != stored {
		t.Fatalf("loadContractRuntime(stored) = (%v, %v), want stored runtime", rt, ok)
	}
	if ContractValidate(cmd) == nil {
		t.Fatal("ContractValidate must return the stored Validate hook")
	}
}

// TestCrossPlatformCoverageDeclareLeafMetadataPanicsOnExecutionFields covers every metadata-only
// misuse guard in DeclareLeafMetadata (nil cmd + all forbidden LeafSpec
// fields + missing Schema).
func TestCrossPlatformCoverageDeclareLeafMetadataPanicsOnExecutionFields(t *testing.T) {
	cases := []struct {
		name string
		cmd  *cobra.Command
		spec LeafSpec
		want string
	}{
		{"nil cmd", nil, LeafSpec{}, "cmd is nil"},
		{"flags", &cobra.Command{Use: "leaf"}, LeafSpec{Flags: []LeafFlag{{Name: "x", Usage: "x"}}}, "Flags must be empty"},
		{"constraints", &cobra.Command{Use: "leaf"}, LeafSpec{Constraints: []LeafConstraint{{Kind: LeafAtLeastOne, Flags: []string{"a", "b"}}}}, "Constraints must be empty"},
		{"const params", &cobra.Command{Use: "leaf"}, LeafSpec{ConstParams: map[string]any{"k": true}}, "ConstParams must be empty"},
		{"call", &cobra.Command{Use: "leaf"}, LeafSpec{Call: func(*cobra.Command, string, map[string]any) error { return nil }}, "Call must be nil"},
		{"runE", &cobra.Command{Use: "leaf"}, LeafSpec{RunE: func(*cobra.Command, []string) error { return nil }}, "RunE must be nil"},
		{"postMount", &cobra.Command{Use: "leaf"}, LeafSpec{PostMount: func(*cobra.Command) {}}, "PostMount must be nil"},
		{"confirmFirst", &cobra.Command{Use: "leaf"}, LeafSpec{ConfirmFirst: true}, "ConfirmFirst must be false"},
		{"server", &cobra.Command{Use: "leaf"}, LeafSpec{Server: "srv"}, "Server/Tool must be empty"},
		{"tool", &cobra.Command{Use: "leaf"}, LeafSpec{Tool: "tool"}, "Server/Tool must be empty"},
		{"empty contract", &cobra.Command{Use: "leaf"}, LeafSpec{}, "Contract is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("DeclareLeafMetadata did not panic, want %q", c.want)
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, c.want) {
					t.Fatalf("panic = %q, want contain %q", msg, c.want)
				}
			}()
			DeclareLeafMetadata(c.cmd, c.spec)
		})
	}
}

// TestCrossPlatformCoverageInstallContractRunEPipelinePanics covers the fail-closed guards of the
// pipeline installer: nil cmd/runtime and a leaf without RunE.
func TestCrossPlatformCoverageInstallContractRunEPipelinePanics(t *testing.T) {
	assertPanic := func(name, want string, fn func()) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s did not panic", name)
				return
			}
			if msg := fmt.Sprint(r); !strings.Contains(msg, want) {
				t.Errorf("%s panic = %q, want contain %q", name, msg, want)
			}
		}()
		fn()
	}
	assertPanic("nil cmd/runtime", "nil cmd/runtime", func() {
		installContractRunEPipeline(nil, nil)
	})
	assertPanic("nil RunE", "RunE is nil", func() {
		installContractRunEPipeline(&cobra.Command{Use: "leaf"}, &contractRuntime{})
	})
}

// TestCrossPlatformCoverageDeclareLeafMetadataValidateWithoutConfirmRunsInner covers the
// non-confirm pipeline: Validate runs first, then the inner RunE executes
// directly (no ConfirmSafety layer for not_required leaves).
func TestCrossPlatformCoverageDeclareLeafMetadataValidateWithoutConfirmRunsInner(t *testing.T) {
	ran := false
	cmd := &cobra.Command{
		Use: "lookup",
		RunE: func(*cobra.Command, []string) error {
			ran = true
			return nil
		},
	}
	cmd.Flags().String("id", "", "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Validate: func(c *cobra.Command, args []string) error {
			id, _ := c.Flags().GetString("id")
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("flag --id is required")
			}
			return nil
		},
		Contract: contractCoverageSchema("test lookup"),
	})
	if !HasContractValidate(cmd) {
		t.Fatal("expected contract Validate annotation")
	}
	if HasContractConfirmSafety(cmd) || HasContractConfirmDeferred(cmd) {
		t.Fatal("not_required leaf must not carry confirm annotations")
	}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	// Validate failure short-circuits before the inner RunE.
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "flag --id is required") {
		t.Fatalf("Execute() error = %v, want Validate failure", err)
	}
	if ran {
		t.Fatal("inner RunE must not run when Validate fails")
	}

	// Validate pass: inner RunE runs without any confirmation gate.
	if err := cmd.Flags().Set("id", "x"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil for not_required leaf", err)
	}
	if !ran {
		t.Fatal("inner RunE must run after Validate passes")
	}
}

// TestCrossPlatformCoverageDeclareLeafMetadataConfirmFallbackWithoutCaller covers the
// no-Validate/no-Caller fallback: without deps the deferred CallTool gate has
// nothing to hook, so ConfirmSafety runs before the inner RunE (fail closed).
func TestCrossPlatformCoverageDeclareLeafMetadataConfirmFallbackWithoutCaller(t *testing.T) {
	testseam.Swap(t, &deps, nil)

	ran := false
	cmd := &cobra.Command{
		Use: "purge",
		RunE: func(*cobra.Command, []string) error {
			ran = true
			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: contractCoverageSchema("test purge"),
	})
	if !HasContractConfirmSafety(cmd) || !HasContractConfirmDeferred(cmd) {
		t.Fatal("expected confirm + deferred-confirm annotations")
	}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || (!strings.Contains(err.Error(), "confirmation_required") && !strings.Contains(err.Error(), "需要用户确认")) {
		t.Fatalf("Execute() without deps/--yes error = %v, want confirmation_required", err)
	}
	if ran {
		t.Fatal("inner RunE must not run before the fallback confirmation")
	}

	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() with --yes error = %v", err)
	}
	if !ran {
		t.Fatal("inner RunE must run once --yes bypasses the fallback confirmation")
	}
}

// TestCrossPlatformCoverageDeclareLeafMetadataDeferredConfirmAfterRunEWithoutCallTool
// covers the deferred-confirm fail-closed path: with a deps.Caller present and
// an inner RunE that never dispatches through CallTool, success-without-confirm
// is rejected. Post-RunE ConfirmSafety is intentionally not used — local side
// effects may already have run.
func TestCrossPlatformCoverageDeclareLeafMetadataDeferredConfirmAfterRunEWithoutCallTool(t *testing.T) {
	caller := &deferConfirmTestCaller{}
	testseam.Swap(t, &deps, &Deps{Caller: caller, Out: NewFormatter()})

	ran := 0
	cmd := &cobra.Command{
		Use: "archive",
		RunE: func(*cobra.Command, []string) error {
			ran++
			return nil // never calls deps.Caller.CallTool
		},
	}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: contractCoverageSchema("test archive"),
	})
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "never obtained via CallTool") {
		t.Fatalf("Execute() without CallTool error = %v, want fail-closed contract error", err)
	}
	if ran != 1 {
		t.Fatalf("inner RunE ran %d times, want 1 (fail-closed after successful RunE)", ran)
	}
	if caller.calls != 0 {
		t.Fatalf("CallTool calls = %d, want 0", caller.calls)
	}

	cmd.Flags().Set("dry-run", "true")
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() with --dry-run error = %v, want nil dry-run bypass", err)
	}
	if ran != 2 {
		t.Fatalf("inner RunE ran %d times, want 2", ran)
	}
	if err := cmd.Flags().Set("dry-run", "false"); err != nil {
		t.Fatal(err)
	}

	// --yes must not green-light a post-RunE confirmation: the contract error
	// remains even when ConfirmSafety would have passed.
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "never obtained via CallTool") {
		t.Fatalf("Execute() with --yes but no CallTool error = %v, want fail-closed contract error", err)
	}
	if ran != 3 {
		t.Fatalf("inner RunE ran %d times, want 3", ran)
	}
}

// readCapableLeafTestCaller is a ToolCaller that also implements the optional
// ReadToolCaller extension (pre-confirm reads).
type readCapableLeafTestCaller struct {
	deferConfirmTestCaller
	readCalls int
}

func (c *readCapableLeafTestCaller) CallReadTool(context.Context, string, string, map[string]any) (*edition.ToolResult, error) {
	c.readCalls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{}`}}}, nil
}

// TestCrossPlatformCoverageContractConfirmReadCallerDelegatesReads covers the ReadToolCaller
// preservation branch of wrapContractConfirmCaller and the ungated
// CallReadTool delegation of contractConfirmReadCaller.
func TestCrossPlatformCoverageContractConfirmReadCallerDelegatesReads(t *testing.T) {
	rc := &readCapableLeafTestCaller{}
	cmd := &cobra.Command{Use: "read"}
	gate := &contractConfirmCaller{
		inner: rc, cmd: cmd,
		safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
	}
	wrapped := wrapContractConfirmCaller(gate, rc)
	readWrapped, ok := wrapped.(*contractConfirmReadCaller)
	if !ok {
		t.Fatalf("wrapContractConfirmCaller = %T, want *contractConfirmReadCaller", wrapped)
	}
	res, err := readWrapped.CallReadTool(context.Background(), "product", "read_tool", nil)
	if err != nil || res == nil {
		t.Fatalf("CallReadTool() = (%v, %v), want ungated success", res, err)
	}
	if rc.readCalls != 1 {
		t.Fatalf("readCalls = %d, want 1", rc.readCalls)
	}
	if gate.confirmed {
		t.Fatal("reads must not flip the confirmation gate")
	}

	// A caller without ReadToolCaller keeps the plain gate.
	plain := &deferConfirmTestCaller{}
	gate2 := &contractConfirmCaller{inner: plain, cmd: cmd}
	if wrapped2 := wrapContractConfirmCaller(gate2, plain); wrapped2 != edition.ToolCaller(gate2) {
		t.Fatalf("wrapContractConfirmCaller(plain) = %T, want the gate itself", wrapped2)
	}
}

// TestCrossPlatformCoverageContractAnnotationHelpersNilAndMarkedCommands covers the nil-safety of
// the exported contract annotation predicates (incl. HasContractConfirmDeferred).
func TestCrossPlatformCoverageContractAnnotationHelpersNilAndMarkedCommands(t *testing.T) {
	if HasContractConfirmSafety(nil) || HasContractValidate(nil) || HasContractConfirmDeferred(nil) {
		t.Fatal("nil command must not report contract annotations")
	}
	bare := &cobra.Command{Use: "bare"}
	if HasContractConfirmSafety(bare) || HasContractValidate(bare) || HasContractConfirmDeferred(bare) {
		t.Fatal("command without annotations must report false")
	}
	marked := &cobra.Command{Use: "marked", Annotations: map[string]string{
		contractConfirmSafetyAnnotation:   "true",
		contractValidateAnnotation:        "true",
		contractConfirmDeferredAnnotation: "true",
	}}
	if !HasContractConfirmSafety(marked) || !HasContractValidate(marked) || !HasContractConfirmDeferred(marked) {
		t.Fatal("marked command must report all contract annotations")
	}
}

// TestCrossPlatformCoverageDevAppMemberMutationValidateRejectsSeparatorOnlyUserIDs covers the
// member add/remove Validate error branch: a separator-only --user-ids value
// passes the required check but parses to an empty list and must be rejected.
func TestCrossPlatformCoverageDevAppMemberMutationValidateRejectsSeparatorOnlyUserIDs(t *testing.T) {
	cases := []struct {
		name  string
		build func(*fakeDevAppRunner) *cobra.Command
	}{
		{"add", func(r *fakeDevAppRunner) *cobra.Command { return newDevAppMemberAddCommand(r) }},
		{"remove", func(r *fakeDevAppRunner) *cobra.Command { return newDevAppMemberRemoveCommand(r) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeDevAppRunner{}
			cmd := c.build(r)
			cmd.Flags().Bool("yes", false, "")
			if err := cmd.Flags().Set("yes", "true"); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Flags().Set("unified-app-id", "APP-1")
			_ = cmd.Flags().Set("user-ids", " , ; ")
			_ = cmd.Flags().Set("member-type", "DEVELOPER")
			err := cmd.RunE(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), "--user-ids 至少包含一个 userId") {
				t.Fatalf("err = %v, want --user-ids 至少包含一个 userId", err)
			}
			if r.got.Tool != "" {
				t.Fatalf("dispatched tool = %q, want no dispatch on Validate failure", r.got.Tool)
			}
		})
	}
}

// TestCrossPlatformCoverageDevAppListTransformsOmitEmptyLists covers the empty-result branches of
// transformDevAppListParam and transformDevAppScopeValues (nil → key omitted).
func TestCrossPlatformCoverageDevAppListTransformsOmitEmptyLists(t *testing.T) {
	for _, raw := range []string{"", "   ", ",", " ; , "} {
		v, err := transformDevAppListParam(raw)
		if err != nil || v != nil {
			t.Fatalf("transformDevAppListParam(%q) = (%#v, %v), want (nil, nil)", raw, v, err)
		}
		v, err = transformDevAppScopeValues(raw)
		if err != nil || v != nil {
			t.Fatalf("transformDevAppScopeValues(%q) = (%#v, %v), want (nil, nil)", raw, v, err)
		}
	}
	v, err := transformDevAppListParam("a, b")
	if err != nil || !reflect.DeepEqual(v, []string{"a", "b"}) {
		t.Fatalf("transformDevAppListParam(a, b) = (%#v, %v), want [a b]", v, err)
	}
	v, err = transformDevAppScopeValues("a,b; c")
	if err != nil || !reflect.DeepEqual(v, []string{"a", "b", "c"}) {
		t.Fatalf("transformDevAppScopeValues(a,b; c) = (%#v, %v), want [a b c]", v, err)
	}
}

// TestCrossPlatformCoverageDevDocSearchRequiresQueryFlag covers the required-query error branch of
// `dws dev doc search` (no positional keyword and no --query/--keyword).
func TestCrossPlatformCoverageDevDocSearchRequiresQueryFlag(t *testing.T) {
	cmd := newDevDocSearchCommand()
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "query") {
		t.Fatalf("RunE() without query error = %v, want required --query error", err)
	}
}

// TestCrossPlatformCoverageSheetMutationGuardRunsContractValidateBeforeTargets covers the outer
// Sheet guard's ContractValidate branch: a DeclareLeafMetadata Validate hook
// must run (and fail) before the --node target check and the --yes-only gate.
func TestCrossPlatformCoverageSheetMutationGuardRunsContractValidateBeforeTargets(t *testing.T) {
	ran := false
	cmd := &cobra.Command{
		Use: "clear-range",
		RunE: func(*cobra.Command, []string) error {
			ran = true
			return nil
		},
	}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("node", "", "")
	cmd.Flags().String("range", "", "")
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Validate: func(c *cobra.Command, args []string) error {
			rng, _ := c.Flags().GetString("range")
			if strings.TrimSpace(rng) == "" {
				return fmt.Errorf("flag --range is required")
			}
			return nil
		},
		Contract: contractCoverageSchema("test clear range"),
	})
	protectSheetMutationCommand(cmd, "清除范围", "文档和目标范围")
	if !HasContractValidate(cmd) || !HasSheetMutationConfirmationGuard(cmd) {
		t.Fatal("expected contract Validate + sheet mutation guard markers")
	}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})

	// Validate failure must win over missing --node and the --yes-only prompt.
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "flag --range is required") {
		t.Fatalf("Execute() error = %v, want ContractValidate failure first", err)
	}
	if ran {
		t.Fatal("inner RunE must not run when ContractValidate fails")
	}

	// Full pass: Validate + --node + --yes reach the inner RunE.
	if err := cmd.Flags().Set("range", "A1:B2"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("node", "node-1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() with valid flags error = %v", err)
	}
	if !ran {
		t.Fatal("inner RunE must run after Validate/--node/--yes all pass")
	}
}

// TestCrossPlatformCoverageRequireSheetMutationTargetsSkipsCommandsWithoutNodeFlag covers the skip
// path for nil commands and leaves that do not declare a --node flag.
func TestCrossPlatformCoverageRequireSheetMutationTargetsSkipsCommandsWithoutNodeFlag(t *testing.T) {
	if err := requireSheetMutationTargets(nil); err != nil {
		t.Fatalf("requireSheetMutationTargets(nil) = %v, want nil", err)
	}
	cmd := &cobra.Command{Use: "no-node"}
	if err := requireSheetMutationTargets(cmd); err != nil {
		t.Fatalf("requireSheetMutationTargets(no --node) = %v, want nil", err)
	}
}

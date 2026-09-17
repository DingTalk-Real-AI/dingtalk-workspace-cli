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
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageValidateGroupPolicy(t *testing.T) {
	valid := []GroupPolicy{
		{},
		{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling},
		{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoveryDeep},
		{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoveryDisabled},
		{Mode: GroupHybrid, Positionals: PositionalsReject, Recovery: RecoverySibling},
		{Mode: GroupHybrid, Positionals: PositionalsReject, Recovery: RecoveryDeep},
		{Mode: GroupHybrid, Positionals: PositionalsReject, Recovery: RecoveryDisabled},
		{Mode: GroupHybrid, Positionals: PositionalsAllow, Recovery: RecoveryDisabled},
	}
	for _, policy := range valid {
		if err := ValidateGroupPolicy(policy); err != nil {
			t.Fatalf("ValidateGroupPolicy(%+v) = %v", policy, err)
		}
	}

	invalid := []struct {
		name   string
		policy GroupPolicy
		needle string
	}{
		{name: "partial", policy: GroupPolicy{Mode: GroupHybrid}, needle: "positionals"},
		{name: "unknown mode", policy: GroupPolicy{Mode: "leafish", Positionals: PositionalsReject, Recovery: RecoveryDisabled}, needle: "mode"},
		{name: "unknown positionals", policy: GroupPolicy{Mode: GroupHybrid, Positionals: "maybe", Recovery: RecoveryDisabled}, needle: "positionals"},
		{name: "unknown recovery", policy: GroupPolicy{Mode: GroupHybrid, Positionals: PositionalsReject, Recovery: "global"}, needle: "recovery"},
		{name: "navigation allows args", policy: GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsAllow, Recovery: RecoveryDisabled}, needle: "navigation-only"},
		{name: "ambiguous recovery", policy: GroupPolicy{Mode: GroupHybrid, Positionals: PositionalsAllow, Recovery: RecoverySibling}, needle: "requires recovery"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateGroupPolicy(tc.policy); err == nil || !strings.Contains(err.Error(), tc.needle) {
				t.Fatalf("ValidateGroupPolicy(%+v) = %v, want %q", tc.policy, err, tc.needle)
			}
		})
	}
}

func TestCrossPlatformCoverageApplyAndReadGroupPolicy(t *testing.T) {
	policy := GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling}
	cmd := &cobra.Command{Use: "parent"}
	ApplyGroupPolicy(cmd, policy)

	got, ok, err := GroupPolicyFor(cmd)
	if err != nil || !ok || got != policy {
		t.Fatalf("GroupPolicyFor() = %+v, %v, %v; want %+v, true, nil", got, ok, err, policy)
	}
	if !cmd.Runnable() || cmd.TraverseChildren {
		t.Fatalf("compiled navigation command Runnable=%v TraverseChildren=%v; policy must preserve the flag-traversal surface", cmd.Runnable(), cmd.TraverseChildren)
	}
	if cmd.Args == nil || cmd.Args(cmd, []string{"extra"}) != nil {
		t.Fatal("PositionalsReject must let command resolution inspect unmatched args")
	}
	if err := cmd.RunE(cmd, []string{"extra"}); err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("navigation recovery error = %v", err)
	}
	var help strings.Builder
	cmd.SetOut(&help)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("navigation help error = %v", err)
	}
	if output := help.String(); !strings.Contains(output, "Usage:") {
		t.Fatalf("navigation help output = %q", output)
	}
	// Re-applying the same declaration is idempotent.
	ApplyGroupPolicy(cmd, policy)

	traversing := &cobra.Command{Use: "traversing", TraverseChildren: true}
	ApplyGroupPolicy(traversing, policy)
	if !traversing.TraverseChildren {
		t.Fatal("ApplyGroupPolicy changed an explicitly declared TraverseChildren surface")
	}

	leaf := &cobra.Command{Use: "leaf"}
	if got, ok, err := GroupPolicyFor(leaf); err != nil || ok || !got.IsZero() {
		t.Fatalf("leaf GroupPolicyFor() = %+v, %v, %v", got, ok, err)
	}
	annotatedLeaf := &cobra.Command{Use: "annotated-leaf", Annotations: map[string]string{"unrelated": "metadata"}}
	if got, ok, err := GroupPolicyFor(annotatedLeaf); err != nil || ok || !got.IsZero() {
		t.Fatalf("annotated leaf GroupPolicyFor() = %+v, %v, %v", got, ok, err)
	}
}

func TestCrossPlatformCoverageApplyGroupPolicyDoesNotLeakParentLocalFlags(t *testing.T) {
	root := &cobra.Command{Use: "dws", SilenceUsage: true, SilenceErrors: true}
	ApplyGroupPolicy(root, GroupPolicy{
		Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling,
	})
	parent := &cobra.Command{
		Use:  "search",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	parent.Flags().String("dimension", "", "parent-only search dimension")
	ApplyGroupPolicy(parent, GroupPolicy{
		Mode: GroupHybrid, Positionals: PositionalsReject, Recovery: RecoverySibling,
	})
	childCalled := false
	child := &cobra.Command{
		Use: "enterprise",
		RunE: func(*cobra.Command, []string) error {
			childCalled = true
			return nil
		},
	}
	parent.AddCommand(child)
	root.AddCommand(parent)
	root.SetArgs([]string{"search", "--dimension", "name", "enterprise"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --dimension") {
		t.Fatalf("Execute() error = %v, want parent local flag rejected by child", err)
	}
	if childCalled {
		t.Fatal("parent local flag leaked into child command execution")
	}
}

func TestCrossPlatformCoverageApplyGroupPolicyHybridPreservesExecution(t *testing.T) {
	called := false
	cmd := &cobra.Command{
		Use: "hybrid",
		RunE: func(*cobra.Command, []string) error {
			called = true
			return nil
		},
	}
	ApplyGroupPolicy(cmd, GroupPolicy{
		Mode:        GroupHybrid,
		Positionals: PositionalsAllow,
		Recovery:    RecoveryDisabled,
	})
	if err := cmd.RunE(cmd, []string{"business-id"}); err != nil || !called {
		t.Fatalf("hybrid RunE was not preserved: called=%v err=%v", called, err)
	}
}

func TestCrossPlatformCoverageApplyGroupPolicyHybridRejectRoutesUnknownArgs(t *testing.T) {
	called := false
	wrapperFrames := 0
	cmd := &cobra.Command{
		Use: "hybrid",
		RunE: func(*cobra.Command, []string) error {
			called = true
			pcs := make([]uintptr, 32)
			frames := runtime.CallersFrames(pcs[:runtime.Callers(0, pcs)])
			for {
				frame, more := frames.Next()
				if strings.Contains(frame.Function, "corecmd.ApplyGroupPolicy.func") {
					wrapperFrames++
				}
				if !more {
					break
				}
			}
			return nil
		},
	}
	policy := GroupPolicy{
		Mode:        GroupHybrid,
		Positionals: PositionalsReject,
		Recovery:    RecoverySibling,
	}
	ApplyGroupPolicy(cmd, policy)
	// Applying the identical declaration must be a no-op. In particular, it
	// must not wrap the already wrapped Hybrid RunE a second time.
	ApplyGroupPolicy(cmd, policy)
	if err := cmd.RunE(cmd, []string{"typo"}); err == nil || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("hybrid typo error = %v", err)
	}
	if called {
		t.Fatal("unknown positional must not reach hybrid business RunE")
	}
	if err := cmd.RunE(cmd, nil); err != nil || !called {
		t.Fatalf("hybrid empty-args execution called=%v err=%v", called, err)
	}
	if wrapperFrames != 1 {
		t.Fatalf("Hybrid RunE wrapper depth = %d, want exactly 1 after idempotent re-apply", wrapperFrames)
	}
}

func TestCrossPlatformCoverageApplyGroupPolicyDeepRecoveryUsesDescendantPath(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	sheet := &cobra.Command{Use: "sheet"}
	rangeGroup := &cobra.Command{Use: "range"}
	rangeGroup.AddCommand(&cobra.Command{Use: "read", Run: func(*cobra.Command, []string) {}})
	ApplyGroupPolicy(rangeGroup, GroupPolicy{
		Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling,
	})
	sheet.AddCommand(
		rangeGroup,
		&cobra.Command{Use: "+list-sheets", Run: func(*cobra.Command, []string) {}},
	)
	ApplyGroupPolicy(sheet, GroupPolicy{
		Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoveryDeep,
	})
	root.AddCommand(sheet)

	err := sheet.RunE(sheet, []string{"read"})
	var structured *apperrors.Error
	if !errors.As(err, &structured) {
		t.Fatalf("deep recovery error = %T %v", err, err)
	}
	if structured.Reason != string(cmdutil.ResolutionUnknownSubcommand) ||
		structured.Hint != `Did you mean "dws sheet range read"? (Run 'dws sheet --help' for the full list)` {
		t.Fatalf("deep recovery = %#v", structured)
	}
	if got, ok := structured.Details["suggestions"].([]string); !ok || !slices.Equal(got, []string{"range read"}) {
		t.Fatalf("deep suggestions = %#v", structured.Details["suggestions"])
	}

	err = sheet.RunE(sheet, []string{"+list-sheet"})
	structured = nil
	if !errors.As(err, &structured) || structured.Reason != string(cmdutil.ResolutionUnknownShortcut) {
		t.Fatalf("direct shortcut recovery = %#v, err=%v", structured, err)
	}
	if !slices.Equal(structured.Actions, []string{
		"Run 'dws sheet --help' for the full list",
		"Run 'dws shortcut list --service sheet --format json'",
	}) {
		t.Fatalf("direct shortcut actions = %#v", structured.Actions)
	}
}

func TestCrossPlatformCoverageApplyGroupPolicyRejectWithoutRecoveryUsesCobraArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "parent"}
	ApplyGroupPolicy(cmd, GroupPolicy{
		Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoveryDisabled,
	})
	if err := cmd.Args(cmd, []string{"extra"}); err == nil {
		t.Fatal("RecoveryDisabled must leave rejected positionals to Cobra")
	}
}

func TestCrossPlatformCoverageApplyGroupPolicyFailsClosed(t *testing.T) {
	mustPanic := func(name, needle string, fn func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				got := recover()
				if got == nil || !strings.Contains(got.(string), needle) {
					t.Fatalf("panic = %v, want %q", got, needle)
				}
			}()
			fn()
		})
	}
	mustPanic("nil command", "nil command", func() { ApplyGroupPolicy(nil, GroupPolicy{}) })
	mustPanic("zero policy", "zero GroupPolicy", func() { ApplyGroupPolicy(&cobra.Command{Use: "leaf"}, GroupPolicy{}) })
	mustPanic("invalid policy", "invalid GroupPolicy", func() {
		ApplyGroupPolicy(&cobra.Command{Use: "broken"}, GroupPolicy{
			Mode: GroupHybrid, Positionals: "unexpected", Recovery: RecoveryDisabled,
		})
	})
	mustPanic("hybrid must run", "must declare RunE", func() {
		ApplyGroupPolicy(&cobra.Command{Use: "hybrid"}, GroupPolicy{
			Mode: GroupHybrid, Positionals: PositionalsReject, Recovery: RecoverySibling,
		})
	})
	mustPanic("conflicting redeclaration", "redeclares GroupPolicy", func() {
		cmd := &cobra.Command{Use: "parent"}
		ApplyGroupPolicy(cmd, GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling})
		ApplyGroupPolicy(cmd, GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoveryDisabled})
	})
	mustPanic("invalid existing metadata", "invalid GroupPolicy metadata", func() {
		cmd := &cobra.Command{
			Use: "broken",
			Annotations: map[string]string{
				groupPolicyAnnotation: "hybrid|reject|unexpected",
			},
		}
		ApplyGroupPolicy(cmd, GroupPolicy{Mode: GroupNavigationOnly, Positionals: PositionalsReject, Recovery: RecoverySibling})
	})
}

func TestCrossPlatformCoverageGroupPolicyForRejectsMalformedPrivateMetadata(t *testing.T) {
	for name, test := range map[string]struct {
		encoded string
		needle  string
	}{
		"malformed":      {encoded: "hybrid|reject", needle: "malformed"},
		"invalid policy": {encoded: "hybrid|reject|unexpected", needle: "recovery"},
		"zero policy":    {encoded: "||", needle: "must not be zero"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: "broken",
				Annotations: map[string]string{
					groupPolicyAnnotation: test.encoded,
				},
			}
			if _, ok, err := GroupPolicyFor(cmd); err == nil || ok || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("GroupPolicyFor(%q) = ok %v, err %v; want %q", test.encoded, ok, err, test.needle)
			}
		})
	}
}

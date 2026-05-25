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

package compat

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTodoCreateSubStub mirrors the leaf command shape emitted by
// BuildDynamicCommands for `todo task create-sub` (envelope:
// create_personal_sub_todo). Only the flags the hook touches are
// registered; the others are irrelevant to the validation.
func newTodoCreateSubStub() *cobra.Command {
	cmd := &cobra.Command{Use: "create-sub"}
	cmd.Flags().String("parent-id", "", "parent todo id")
	cmd.Flags().String("title", "", "title")
	cmd.Flags().String("executors", "", "executors")
	return cmd
}

func TestValidateTodoParentIdNumeric_AcceptsPureDigits(t *testing.T) {
	cmd := newTodoCreateSubStub()
	if err := cmd.Flags().Set("parent-id", "53340859882"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := validateTodoParentIdNumeric(cmd); err != nil {
		t.Fatalf("expected nil for numeric parent-id, got %v", err)
	}
}

func TestValidateTodoParentIdNumeric_RejectsAlphanumeric(t *testing.T) {
	cmd := newTodoCreateSubStub()
	if err := cmd.Flags().Set("parent-id", "INVALID_PARENT_ID_99999"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	err := validateTodoParentIdNumeric(cmd)
	if err == nil {
		t.Fatal("expected validation error for non-numeric parent-id")
	}
	if !strings.Contains(err.Error(), "纯数字") {
		t.Fatalf("expected '纯数字' in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "INVALID_PARENT_ID_99999") {
		t.Fatalf("expected offending value in error, got %v", err)
	}
}

func TestValidateTodoParentIdNumeric_RejectsLeadingZeroPaddedHex(t *testing.T) {
	// "0xdeadbeef" should fail strconv.ParseInt base 10, ensuring we are
	// not silently accepting hex-shaped IDs.
	cmd := newTodoCreateSubStub()
	if err := cmd.Flags().Set("parent-id", "0xdeadbeef"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := validateTodoParentIdNumeric(cmd); err == nil {
		t.Fatal("expected validation error for hex-shaped parent-id")
	}
}

func TestValidateTodoParentIdNumeric_RejectsWhitespacePadded(t *testing.T) {
	// Trimmed value is "abc" — must still reject; equally guards against
	// "  123  " false-positive once trimmed (which we DO accept as 123).
	cmd := newTodoCreateSubStub()
	if err := cmd.Flags().Set("parent-id", "  abc  "); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := validateTodoParentIdNumeric(cmd); err == nil {
		t.Fatal("expected validation error for non-numeric (whitespace-padded) parent-id")
	}
}

func TestValidateTodoParentIdNumeric_AcceptsWhitespacePaddedDigits(t *testing.T) {
	cmd := newTodoCreateSubStub()
	if err := cmd.Flags().Set("parent-id", "  53340859882  "); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := validateTodoParentIdNumeric(cmd); err != nil {
		t.Fatalf("expected whitespace-padded digits to pass after trim, got %v", err)
	}
}

func TestValidateTodoParentIdNumeric_EmptyPassesThrough(t *testing.T) {
	// Envelope marks parent-id required, so cobra produces the missing-flag
	// error itself. We must not preempt that with a confusing message.
	cmd := newTodoCreateSubStub()
	if err := validateTodoParentIdNumeric(cmd); err != nil {
		t.Fatalf("expected nil for empty parent-id (cobra owns required-check), got %v", err)
	}
}

func TestValidateTodoParentIdNumeric_NoFlagRegistered(t *testing.T) {
	// Defensive: a command without the flag must not panic / error.
	cmd := &cobra.Command{Use: "noop"}
	if err := validateTodoParentIdNumeric(cmd); err != nil {
		t.Fatalf("expected nil when --parent-id absent, got %v", err)
	}
}

// ── installTodoHook composition ────────────────────────────────

func TestInstallTodoHook_NoOpForOtherProduct(t *testing.T) {
	cmd := newTodoCreateSubStub()
	originalCalled := false
	cmd.PreRunE = func(*cobra.Command, []string) error { originalCalled = true; return nil }
	installTodoHook(cmd, "chat", "create_personal_sub_todo")
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !originalCalled {
		t.Fatal("original PreRunE should still run when hook skips")
	}
	// Bad parent-id must NOT fail since hook is no-op for non-todo product.
	if err := cmd.Flags().Set("parent-id", "INVALID"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("non-todo product must not validate parent-id: %v", err)
	}
}

func TestInstallTodoHook_NoOpForOtherTodoTool(t *testing.T) {
	// e.g. `todo task get` reuses parent-id-less plumbing — make sure we do
	// not blanket-validate every todo leaf.
	cmd := newTodoCreateSubStub()
	installTodoHook(cmd, "todo", "get_personal_todo_detail")
	if err := cmd.Flags().Set("parent-id", "INVALID"); err != nil {
		t.Fatal(err)
	}
	if cmd.PreRunE != nil {
		if err := cmd.PreRunE(cmd, nil); err != nil {
			t.Fatalf("non-target todo tool must not validate parent-id: %v", err)
		}
	}
}

func TestInstallTodoHook_TargetToolRejectsInvalid(t *testing.T) {
	cmd := newTodoCreateSubStub()
	installTodoHook(cmd, "todo", "create_personal_sub_todo")
	if cmd.PreRunE == nil {
		t.Fatal("installTodoHook should install a PreRunE for the target tool")
	}
	if err := cmd.Flags().Set("parent-id", "INVALID_PARENT_ID_99999"); err != nil {
		t.Fatal(err)
	}
	err := cmd.PreRunE(cmd, nil)
	if err == nil {
		t.Fatal("expected hook to reject non-numeric parent-id")
	}
	if !strings.Contains(err.Error(), "纯数字") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestInstallTodoHook_TargetToolAcceptsValid(t *testing.T) {
	cmd := newTodoCreateSubStub()
	installTodoHook(cmd, "todo", "create_personal_sub_todo")
	if err := cmd.Flags().Set("parent-id", "53340859882"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("numeric parent-id must pass: %v", err)
	}
}

func TestInstallTodoHook_ChainsExistingPreRunE(t *testing.T) {
	cmd := newTodoCreateSubStub()
	originalCalled := false
	cmd.PreRunE = func(*cobra.Command, []string) error {
		originalCalled = true
		return nil
	}
	installTodoHook(cmd, "todo", "create_personal_sub_todo")
	if err := cmd.Flags().Set("parent-id", "1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !originalCalled {
		t.Fatal("original PreRunE was dropped")
	}
}

func TestInstallTodoHook_BailsIfChainedPreRunEFails(t *testing.T) {
	cmd := newTodoCreateSubStub()
	cmd.PreRunE = func(*cobra.Command, []string) error { return errors.New("original boom") }
	installTodoHook(cmd, "todo", "create_personal_sub_todo")
	// Even with a VALID parent-id, the chained original error must bubble up
	// before our validation runs.
	if err := cmd.Flags().Set("parent-id", "1"); err != nil {
		t.Fatal(err)
	}
	err := cmd.PreRunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "original boom") {
		t.Fatalf("expected original PreRunE error to bubble, got %v", err)
	}
}

func TestInstallTodoHook_NilCmdSafe(t *testing.T) {
	// Defensive: should not panic.
	installTodoHook(nil, "todo", "create_personal_sub_todo")
}

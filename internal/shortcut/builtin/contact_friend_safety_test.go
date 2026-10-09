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

package builtin_test

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func findContactFriendLeaf(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, service := range root.Commands() {
		if service.Name() != "contact" {
			continue
		}
		for _, leaf := range service.Commands() {
			if leaf.Name() == name {
				return leaf
			}
		}
	}
	t.Fatalf("leaf %s not found under contact", name)
	return nil
}

// TestContactFriendShortcutsKeepDeclaredSafety pins the final mount path for
// the friend link: the shared contact finalizer must not overwrite a
// shortcut's declared Safety with the contact read default, so the four
// write shortcuts keep their user_required confirmation gates and the two
// list shortcuts publish the cursor pagination contract.
func TestContactFriendShortcutsKeepDeclaredSafety(t *testing.T) {
	root := newCoverageRoot()
	writeSafety := map[string]contract.SafetySpec{
		"+friend-request-send":   {Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		"+friend-request-accept": {Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		"+friend-request-reject": {Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"},
		"+friend-remove":         {Effect: "write", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"},
	}
	for command, want := range writeSafety {
		leaf := findContactFriendLeaf(t, root, command)
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Safety == nil {
			t.Fatalf("%s: mounted leaf must publish ContractFinal Safety", command)
		}
		if got := *final.Safety; got.Effect != want.Effect || got.Risk != want.Risk ||
			got.Confirmation != want.Confirmation || got.Idempotency != want.Idempotency {
			t.Fatalf("%s ContractFinal Safety = %#v, want declared %#v", command, got, want)
		}
	}
	for command := range map[string]bool{"+friend-list": true, "+friend-request-list": true} {
		leaf := findContactFriendLeaf(t, root, command)
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Safety == nil {
			t.Fatalf("%s: mounted leaf must publish ContractFinal Safety", command)
		}
		if got := *final.Safety; got.Effect != "read" || got.Risk != "low" || got.Confirmation != "not_required" {
			t.Fatalf("%s ContractFinal Safety = %#v, want read/low/not_required", command, got)
		}
		if final.Pagination == nil || final.Pagination.Kind != contract.PaginationKindCursor ||
			final.Pagination.CursorParameter != "cursor" ||
			final.Pagination.MetaPath != contract.PaginationMetaPath {
			t.Fatalf("%s ContractFinal Pagination = %#v, want cursor pagination on --cursor", command, final.Pagination)
		}
	}
}

// TestContactFriendRemoveRequiresRuntimeConfirmation drives the runtime
// confirmation gate end to end: without --yes the high-risk remove must fail
// closed with confirmation_required before the Execute closure runs.
func TestContactFriendRemoveRequiresRuntimeConfirmation(t *testing.T) {
	caller := &fakeCaller{}
	helpers.InitDepsForTest(t, caller)
	root := newCoverageRoot()
	root.SetArgs([]string{"contact", "+friend-remove", "--friend", "open-dt-fixture"})
	err := corecmd.ExecuteForTest(root)
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "confirmation_required" {
		t.Fatalf("friend-remove without --yes must fail confirmation_required; err = %#v", err)
	}
	if caller.called {
		t.Fatal("confirmation gate must run before Execute")
	}
}

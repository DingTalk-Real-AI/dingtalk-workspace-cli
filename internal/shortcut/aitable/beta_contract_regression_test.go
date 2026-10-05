// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageRecordQueryShorthandFilterWiring(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"records":[],"hasMore":false,"nextCursor":""}`}}}
	_, err := runRecordQueryShortcutCLI(t, caller, 10, `--filters={"operator":"and","operands":[{"fieldId":"f","operator":"gt","value":5}]}`)
	if err != nil || len(caller.calls) != 1 {
		t.Fatalf("query: %v / %#v", err, caller.calls)
	}
	filters := caller.calls[0].args["filters"].(map[string]any)
	want := map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "gt", "operands": []any{"f", float64(5)}}}}
	if !reflect.DeepEqual(filters, want) {
		t.Fatalf("shorthand was not normalized: %#v", filters)
	}
}

func TestCrossPlatformCoverageAitableAppIdempotencyDeclarations(t *testing.T) {
	want := map[string]string{
		"+app-get": "idempotent", "+app-page-list": "idempotent", "+app-page-get": "idempotent",
		"+app-page-create": "non_idempotent", "+app-page-update": "idempotent", "+app-page-delete": "unknown",
		"+app-block-list": "idempotent", "+app-block-get": "idempotent", "+app-block-create": "non_idempotent",
		"+app-block-update": "idempotent", "+app-block-delete": "unknown",
	}
	for _, command := range parityAppShortcuts() {
		if command.Safety.Idempotency != want[command.Command] {
			t.Fatalf("%s: %#v", command.Command, command.Safety)
		}
		if (command.Command == "+app-get" || command.Command == "+app-page-list") && (command.Safety.Effect != "write" || command.Safety.Confirmation != "user_required") {
			t.Fatalf("conditional write guard lost: %#v", command.Safety)
		}
		delete(want, command.Command)
	}
	if len(want) != 0 {
		t.Fatalf("missing commands: %#v", want)
	}
}

func TestCrossPlatformCoverageAitableAppIdempotencyFallback(t *testing.T) {
	for _, operation := range []struct {
		command string
		write   bool
		safety  contract.SafetySpec
	}{
		{"+app-page-get", false, contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required"}},
		{"+app-page-create", true, contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required"}},
		{"+app-page-delete", true, contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required"}},
	} {
		for _, idempotency := range []struct{ declared, want string }{
			{"", "unknown"},
			{"unknown", "unknown"},
			{"idempotent", "idempotent"},
			{"non_idempotent", "non_idempotent"},
		} {
			t.Run(operation.command+"/"+idempotency.declared, func(t *testing.T) {
				commands := buildParityAppShortcuts([]parityAppOperation{{
					command: operation.command, description: "Test App operation",
					write: operation.write, idempotency: idempotency.declared,
				}})
				if len(commands) != 1 {
					t.Fatalf("got %d commands, want 1", len(commands))
				}
				// Exercise real command construction: a missing idempotency must
				// not panic or infer replay safety from the operation's write risk.
				cmd := corecmd.New(shortcut.FromShortcut(commands[0]))
				final, ok := contractfinal.RuntimeContractFinal(cmd)
				want := operation.safety
				want.EffectSource = "corecmd.contract"
				want.Idempotency = idempotency.want
				if !ok || final.Safety == nil || *final.Safety != want {
					t.Fatalf("final safety = %#v (declared=%v), want %#v", final.Safety, ok, want)
				}
			})
		}
	}
}

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
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/wait"
)

func waitTestDecl() ContractDecl {
	return ContractDecl{
		Title:       "Wait Title",
		Description: "Wait Desc",
		Wait: &contract.WaitSpec{
			Mode:               contract.WaitModePoll,
			PollCommand:        "oa approval-instance get",
			StatusQuery:        "result.status",
			Terminal:           map[string]contract.ResultOutcome{"COMPLETED": contract.ResultOutcomeSuccess, "REJECTED": contract.ResultOutcomeFailure},
			PendingValues:      []string{"NEW", "RUNNING"},
			DefaultTimeoutSecs: 60,
		},
		Interface: &contract.InterfaceSpec{Mode: "local", Availability: "available"},
		Selection: contract.SelectionSpec{
			AgentSummary: "summary",
			UseWhen:      []string{"when wait"},
			AvoidWhen:    []string{"when nowait"},
			Examples:     []string{"dws wait-sample --wait"},
		},
		Identity: contract.ToolIdentitySpec{ProductID: "sample", Name: "waitsample", CanonicalPath: "sample.waitsample", CLIPath: "wait-sample", PrimaryCLIPath: "wait-sample"},
	}
}

func baseWaitSpec(decl ContractDecl, poll func(*Ctx) (wait.PollDoc, error)) Spec {
	return Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      decl,
		WaitPoll:      poll,
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending(map[string]any{"id": "job-1"}, &output.OperationInfo{
				ID:          "job-1",
				State:       "NEW",
				NextCommand: "dws wait-sample --id job-1",
			}), nil
		},
	}
}

func TestWaitFlagsOnlyRegisteredWhenDeclared(t *testing.T) {
	declared := New(baseWaitSpec(waitTestDecl(), func(*Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	if flag := declared.Flags().Lookup(waitFlagName); flag == nil {
		t.Fatal("declared command missing --wait flag")
	}
	if flag := declared.Flags().Lookup(waitTimeoutFlagName); flag == nil {
		t.Fatal("declared command missing --wait-timeout flag")
	}

	undeclared := New(Spec{
		Use:    "nowait",
		Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	if flag := undeclared.Flags().Lookup(waitFlagName); flag != nil {
		t.Fatal("undeclared command registered --wait")
	}
	undeclared.SetArgs([]string{"--wait"})
	if err := undeclared.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("err=%v want unknown-flag", err)
	}
}

func TestValidateWaitDeclPairsDeclarationWithImplementation(t *testing.T) {
	decl := waitTestDecl()
	spec := baseWaitSpec(decl, nil)
	expectPanic(t, func() { New(spec) }, "WaitPoll")

	spec.WaitPoll = func(*Ctx) (wait.PollDoc, error) { return nil, nil }
	expectPanic(t, func() {
		New(Spec{
			Use:      "hook-only",
			Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
			Invoke:   func(*Ctx, map[string]any) error { return nil },
			WaitPoll: func(*Ctx) (wait.PollDoc, error) { return nil, nil },
		})
	}, "Contract.Wait")
}

func expectPanic(t *testing.T, fn func(), want string) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if message, ok := recovered.(string); !ok || !strings.Contains(message, want) {
			t.Fatalf("panic=%v want containing %q", recovered, want)
		}
	}()
	fn()
}

func TestResultInvokeWaitClosesEnvelopeOutcome(t *testing.T) {
	wait.SwapSleepForTest(t, func(time.Duration) {})
	cmd := New(baseWaitSpec(waitTestDecl(), func(*Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "REJECTED"}}, nil
	}))
	cmd.SetArgs([]string{"--wait"})
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code == 0 {
		t.Fatalf("stored code/emitted=%d/%v, want non-zero for terminal failure", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "failure"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestResultInvokeWaitTimeoutKeepsPending(t *testing.T) {
	wait.SwapSleepForTest(t, func(time.Duration) {})
	polls := 0
	cmd := New(baseWaitSpec(waitTestDecl(), func(*Ctx) (wait.PollDoc, error) {
		polls++
		return wait.PollDoc{"result": map[string]any{"status": "RUNNING"}}, nil
	}))
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "1"})
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("timeout wait must exit 0 (pending is not failure): %v", err)
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "pending"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
	if polls == 0 {
		t.Fatal("wait phase never polled")
	}
}

func TestInvokeWithoutWaitFlagSkipsPhase(t *testing.T) {
	polled := false
	cmd := New(Spec{
		Use:      "wait-sample",
		Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract: waitTestDecl(),
		WaitPoll: func(*Ctx) (wait.PollDoc, error) {
			polled = true
			return nil, nil
		},
		Invoke: func(*Ctx, map[string]any) error { return nil },
	})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if polled {
		t.Fatal("wait phase ran without --wait")
	}
}

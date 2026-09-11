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
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
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

func baseWaitSpec(decl ContractDecl, poll func(context.Context, *Ctx) (wait.PollDoc, error)) Spec {
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
	declared := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
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

	spec.WaitPoll = func(context.Context, *Ctx) (wait.PollDoc, error) { return nil, nil }
	expectPanic(t, func() {
		New(Spec{
			Use:      "hook-only",
			Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
			Invoke:   func(*Ctx, map[string]any) error { return nil },
			WaitPoll: func(context.Context, *Ctx) (wait.PollDoc, error) { return nil, nil },
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

func TestWaitTimeoutFlagDefaultsComeFromDeclaration(t *testing.T) {
	stub := func(context.Context, *Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}
	declared := New(baseWaitSpec(waitTestDecl(), stub))
	if value, err := declared.Flags().GetInt(waitTimeoutFlagName); err != nil || value != 60 {
		t.Fatalf("declared default=%d/%v, want reviewed 60", value, err)
	}
	decl := waitTestDecl()
	decl.Wait.DefaultTimeoutSecs = 0
	fallback := New(baseWaitSpec(decl, stub))
	if value, err := fallback.Flags().GetInt(waitTimeoutFlagName); err != nil || value != DefaultWaitTimeoutSecs {
		t.Fatalf("fallback default=%d/%v, want framework %d", value, err, DefaultWaitTimeoutSecs)
	}
	// A non-positive explicit value falls back to the framework default.
	fallback.SetArgs([]string{"--wait-timeout", "0", "--wait"})
	if err := fallback.Flags().Set(waitTimeoutFlagName, "0"); err != nil {
		t.Fatal(err)
	}
	if got := waitTimeoutSecs(fallback); got != DefaultWaitTimeoutSecs {
		t.Fatalf("waitTimeoutSecs=%d, want %d", got, DefaultWaitTimeoutSecs)
	}
}

func TestWaitTimeoutDurationRejectsOverflowingSeconds(t *testing.T) {
	// math.MaxInt64 (9223372036854775807) is a legal pflag int on 64-bit
	// platforms and overflows time.Duration(secs)*time.Second to a negative
	// value, which would disable the wait deadline.
	if _, err := waitTimeoutDuration(math.MaxInt64); err == nil || !strings.Contains(err.Error(), "超出可表示范围") {
		t.Fatalf("err=%v, want overflow validation", err)
	}
	d, err := waitTimeoutDuration(maxWaitTimeoutSecs)
	if err != nil {
		t.Fatal(err)
	}
	if d <= 0 || d != time.Duration(maxWaitTimeoutSecs)*time.Second {
		t.Fatalf("duration=%d, want the largest representable timeout", d)
	}
	// Non-positive second counts fall back to the framework default instead
	// of disabling the deadline (waitTimeoutSecs already maps a zero/negative
	// flag to the default; this keeps the conversion itself fail-safe).
	if got, err := waitTimeoutDuration(0); err != nil || got != time.Duration(DefaultWaitTimeoutSecs)*time.Second {
		t.Fatalf("duration/err=%d/%v, want framework default", got, err)
	}
	if got, err := waitTimeoutDuration(-5); err != nil || got != time.Duration(DefaultWaitTimeoutSecs)*time.Second {
		t.Fatalf("duration/err=%d/%v, want framework default", got, err)
	}
}

func TestResultInvokeWaitTimeoutOverflowIsValidationError(t *testing.T) {
	if int64(math.MaxInt) <= maxWaitTimeoutSecs {
		t.Skip("platform int cannot overflow time.Duration")
	}
	polled := false
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		polled = true
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	cmd.SetArgs([]string{"--wait", "--wait-timeout", strconv.Itoa(math.MaxInt)})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "超出可表示范围") {
		t.Fatalf("err=%v, want overflow validation", err)
	}
	if polled {
		t.Fatal("overflowing --wait-timeout must not start the wait loop")
	}
}

func TestResultInvokeWaitPollErrorFailsTheCommand(t *testing.T) {
	boom := errors.New("rpc down")
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		return nil, boom
	}))
	cmd.SetArgs([]string{"--wait"})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "rpc down") {
		t.Fatalf("err=%v, want poll error surfaced", err)
	}
}

func TestResultInvokeWaitUnknownStatusFailsClosed(t *testing.T) {
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "Mystery"}}, nil
	}))
	cmd.SetArgs([]string{"--wait"})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err == nil || !wait.IsUnknownStatus(err) {
		t.Fatalf("err=%v, want unknown-status", err)
	}
}

func TestWaitCtxAccessorsExposeDeclaredCapability(t *testing.T) {
	var gotWait bool
	var gotTimeout int
	cmd := New(baseWaitSpec(waitTestDecl(), func(_ context.Context, c *Ctx) (wait.PollDoc, error) {
		gotWait = c.Wait()
		gotTimeout = c.WaitTimeoutSecs()
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "90"})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !gotWait || gotTimeout != 90 {
		t.Fatalf("ctx accessors=%v/%d", gotWait, gotTimeout)
	}
}

func eventTestDecl(mode string) ContractDecl {
	decl := waitTestDecl()
	decl.Wait.Mode = mode
	decl.Wait.EventKey = "bpms_instance_change"
	decl.Wait.MatchField = "process_instance_id"
	decl.Wait.ResourceQuery = "id"
	return decl
}

type scriptedStream struct {
	events []wait.PollDoc
	err    error
}

func (s *scriptedStream) Recv(context.Context) (wait.PollDoc, error) {
	if len(s.events) > 0 {
		doc := s.events[0]
		s.events = s.events[1:]
		return doc, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

func TestValidateWaitDeclPairsModeWithHooks(t *testing.T) {
	poll := func(context.Context, *Ctx) (wait.PollDoc, error) { return nil, nil }
	events := func(context.Context, *Ctx) (wait.EventStream, error) { return nil, nil }
	cases := []struct {
		name       string
		mode       string
		waitPoll   bool
		waitEvents bool
		want       string
	}{
		{"event without WaitEvents", contract.WaitModeEvent, false, false, "WaitEvents"},
		{"auto without WaitPoll", contract.WaitModeAuto, false, true, "WaitPoll"},
		{"poll with WaitEvents", contract.WaitModePoll, true, true, "WaitEvents"},
		{"event with WaitPoll", contract.WaitModeEvent, true, true, "WaitPoll"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectPanic(t, func() {
				New(Spec{
					Use:           "wait-sample",
					OutputRollout: output.RolloutUnifiedActive,
					Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
					Contract:      eventTestDecl(tc.mode),
					ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
						return output.Pending(map[string]any{}, nil), nil
					},
					WaitPoll:   hookOrNil(tc.waitPoll, poll),
					WaitEvents: eventHookOrNil(tc.waitEvents, events),
				})
			}, tc.want)
		})
	}
}

func hookOrNil(set bool, hook func(context.Context, *Ctx) (wait.PollDoc, error)) func(context.Context, *Ctx) (wait.PollDoc, error) {
	if !set {
		return nil
	}
	return hook
}

func eventHookOrNil(set bool, hook func(context.Context, *Ctx) (wait.EventStream, error)) func(context.Context, *Ctx) (wait.EventStream, error) {
	if !set {
		return nil
	}
	return hook
}

func runWaitModeCommand(t *testing.T, decl ContractDecl, poll func(context.Context, *Ctx) (wait.PollDoc, error), events func(context.Context, *Ctx) (wait.EventStream, error), args ...string) (string, error) {
	t.Helper()
	cmd := New(Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      decl,
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending(map[string]any{"id": "job-1"}, &output.OperationInfo{
				ID: "job-1", State: "NEW", NextCommand: "dws wait-sample --id job-1",
			}), nil
		},
		WaitPoll:   poll,
		WaitEvents: events,
	})
	cmd.SetArgs(append([]string{"--wait"}, args...))
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	err := cmd.Execute()
	return stdout.String(), err
}

func TestEventModeClosesEnvelopeFromCorrelatedEvent(t *testing.T) {
	stream := &scriptedStream{events: []wait.PollDoc{
		{"process_instance_id": "other", "result": map[string]any{"status": "COMPLETED"}},
		{"process_instance_id": "job-1", "result": map[string]any{"status": "REJECTED"}},
	}}
	stdout, err := runWaitModeCommand(t, eventTestDecl(contract.WaitModeEvent), nil, func(context.Context, *Ctx) (wait.EventStream, error) {
		return stream, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"outcome": "failure"`) || !strings.Contains(stdout, `"type": "wait"`) {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestEventModeSurfacesStreamEndAsError(t *testing.T) {
	_, err := runWaitModeCommand(t, eventTestDecl(contract.WaitModeEvent), nil, func(context.Context, *Ctx) (wait.EventStream, error) {
		return &scriptedStream{}, nil
	})
	if err == nil || !errors.Is(err, wait.ErrEventStreamEnded) {
		t.Fatalf("err=%v, want stream-ended", err)
	}
}

func TestEventModeRejectsUnresolvableResource(t *testing.T) {
	decl := eventTestDecl(contract.WaitModeEvent)
	decl.Wait.ResourceQuery = "missing"
	_, err := runWaitModeCommand(t, decl, nil, func(context.Context, *Ctx) (wait.EventStream, error) {
		return &scriptedStream{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "resource query") {
		t.Fatalf("err=%v", err)
	}
}

func TestAutoModeFallsBackToPollOnStreamEnd(t *testing.T) {
	polled := false
	stdout, err := runWaitModeCommand(t, eventTestDecl(contract.WaitModeAuto),
		func(context.Context, *Ctx) (wait.PollDoc, error) {
			polled = true
			return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
		},
		func(context.Context, *Ctx) (wait.EventStream, error) {
			return &scriptedStream{}, nil // ends immediately
		})
	if err != nil {
		t.Fatal(err)
	}
	if !polled {
		t.Fatal("auto mode did not fall back to polling")
	}
	if !strings.Contains(stdout, `"outcome": "success"`) {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestAutoModeFallsBackToPollOnSubscriptionFailure(t *testing.T) {
	polled := false
	_, err := runWaitModeCommand(t, eventTestDecl(contract.WaitModeAuto),
		func(context.Context, *Ctx) (wait.PollDoc, error) {
			polled = true
			return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
		},
		func(context.Context, *Ctx) (wait.EventStream, error) {
			return nil, errors.New("no subscriber credential")
		})
	if err != nil {
		t.Fatal(err)
	}
	if !polled {
		t.Fatal("auto mode did not fall back to polling on subscription failure")
	}
}

func TestResultInvokeWaitClosesEnvelopeOutcome(t *testing.T) {
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
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
	if code, emitted := output.StoredExitCode(store); !emitted || code != 8 {
		t.Fatalf("stored code/emitted=%d/%v, want dedicated wait-terminal-failure code 8", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"type": "wait"`) {
		t.Fatalf("stdout=%s, want error.type wait", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"outcome": "failure"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
	// The final emitted envelope must carry the observed terminal status in
	// meta.operation.state — the acceptance-phase state ("NEW") must not
	// survive the close (P1 regression guard).
	if !strings.Contains(stdout.String(), `"state": "REJECTED"`) {
		t.Fatalf("stdout=%s, want operation.state synced to the terminal status", stdout.String())
	}
	if strings.Contains(stdout.String(), `"state": "NEW"`) {
		t.Fatalf("stdout=%s, acceptance-phase operation.state leaked into the terminal envelope", stdout.String())
	}
	if strings.Contains(stdout.String(), `"timed_out": true`) {
		t.Fatalf("stdout=%s, terminal close must not claim timed_out", stdout.String())
	}
}

func TestResultInvokeWaitSuccessCloseSyncsOperationState(t *testing.T) {
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
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
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v, want success exit 0", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "success"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
	// Success close must publish the terminal status, never the stale
	// acceptance-phase state (no outcome=success with state=processing/NEW).
	if !strings.Contains(stdout.String(), `"state": "COMPLETED"`) {
		t.Fatalf("stdout=%s, want operation.state synced to the terminal status", stdout.String())
	}
	if strings.Contains(stdout.String(), `"state": "NEW"`) {
		t.Fatalf("stdout=%s, acceptance-phase operation.state leaked into the success envelope", stdout.String())
	}
	// Operation identity (id / next_command) survives the terminal close.
	if !strings.Contains(stdout.String(), `"id": "job-1"`) || !strings.Contains(stdout.String(), `"next_command"`) {
		t.Fatalf("stdout=%s, want operation id/next_command preserved", stdout.String())
	}
}

func TestResultInvokeWaitTimeoutKeepsPending(t *testing.T) {
	polls := 0
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
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

func TestResultInvokeWaitParentCancellationPropagates(t *testing.T) {
	// Ctrl-C (parent context cancellation) during a wait must propagate the
	// cancellation error, not be swallowed as a timed-out pending with exit 0.
	ctx, cancel := context.WithCancel(context.Background())
	polls := 0
	cmd := New(baseWaitSpec(waitTestDecl(), func(pollCtx context.Context, _ *Ctx) (wait.PollDoc, error) {
		polls++
		if polls == 1 {
			// Cancel the parent context after the first poll to simulate Ctrl-C
			// arriving during the wait phase.
			cancel()
		}
		return wait.PollDoc{"result": map[string]any{"status": "RUNNING"}}, nil
	}))
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "60"})
	storeCtx, _ := output.WithResultStore(ctx)
	cmd.SetContext(storeCtx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	err := cmd.Execute()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled (cancellation must not be swallowed as timeout)", err)
	}
	if strings.Contains(stdout.String(), `"outcome": "pending"`) {
		t.Fatalf("cancellation must not produce a pending envelope; stdout=%s", stdout.String())
	}
}

func TestEventModeSubscriptionFailureWithParentCancellationPropagates(t *testing.T) {
	// Ctrl-C arriving during a failed event subscription must propagate the
	// cancellation, not be wrapped as a subscription-failure error.
	ctx, cancel := context.WithCancel(context.Background())
	cmd := New(Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      eventTestDecl(contract.WaitModeEvent),
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending(map[string]any{"id": "job-1"}, &output.OperationInfo{
				ID: "job-1", State: "NEW", NextCommand: "dws wait-sample --id job-1",
			}), nil
		},
		WaitEvents: func(context.Context, *Ctx) (wait.EventStream, error) {
			cancel()
			return nil, errors.New("subscribe failed")
		},
	})
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "60"})
	storeCtx, _ := output.WithResultStore(ctx)
	cmd.SetContext(storeCtx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	err := cmd.Execute()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestWaitDeclRequiresResultInvokeDispatcher(t *testing.T) {
	// A declared wait on the legacy Invoke path would observe a failure
	// terminal while still exiting 0 — construction must reject it.
	expectPanic(t, func() {
		New(Spec{
			Use:      "wait-sample",
			Safety:   contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
			Contract: waitTestDecl(),
			WaitPoll: func(context.Context, *Ctx) (wait.PollDoc, error) { return nil, nil },
			Invoke:   func(*Ctx, map[string]any) error { return nil },
		})
	}, "ResultInvoke")
}

func TestResultInvokeWithoutWaitFlagSkipsPhase(t *testing.T) {
	polled := false
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		polled = true
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	cmd.SetArgs(nil)
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if polled {
		t.Fatal("wait phase ran without --wait")
	}
}

func TestWaitTimeoutWithoutWaitFlagIsRejected(t *testing.T) {
	// --wait-timeout without --wait must be rejected before ResultInvoke,
	// not silently ignored (which would mislead callers into thinking the
	// command waited for a terminal state).
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		t.Fatal("WaitPoll must not run when flag validation fails")
		return nil, nil
	}))
	cmd.SetArgs([]string{"--wait-timeout", "60"})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --wait-timeout without --wait")
	}
	if !strings.Contains(err.Error(), "--wait-timeout requires --wait") {
		t.Fatalf("err=%v, want flag combination error", err)
	}
}

func TestWaitTimeoutWithWaitFlagProceeds(t *testing.T) {
	// Valid combination: --wait --wait-timeout should proceed normally.
	polled := false
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		polled = true
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "60"})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !polled {
		t.Fatal("wait phase did not poll with valid flag combination")
	}
}

func TestAttachContractPanicsOnInvalidWaitDeclaration(t *testing.T) {
	decl := waitTestDecl()
	decl.Wait.Mode = "event" // not implemented
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic on invalid Contract.Wait")
		}
		if message, ok := recovered.(string); !ok || !strings.Contains(message, "Contract.Wait") {
			t.Fatalf("panic=%v", recovered)
		}
	}()
	AttachContract(&cobra.Command{Use: "wait-sample"}, contract.SafetySpec{
		Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
	}, decl, "", "")
}

func TestWaitDeclPaddedStatusValuesAreNormalized(t *testing.T) {
	// A declaration whose status values carry surrounding whitespace must be
	// canonicalized at construction so the runtime wait engine and the
	// published Schema agree: the backend returns "COMPLETED" verbatim, and
	// a padded terminal key would fail closed as an unknown status.
	decl := waitTestDecl()
	decl.Wait.Terminal = map[string]contract.ResultOutcome{
		" COMPLETED ": contract.ResultOutcomeSuccess,
		"\tREJECTED":  contract.ResultOutcomeFailure,
	}
	decl.Wait.PendingValues = []string{" NEW ", "RUNNING "}
	cmd := New(baseWaitSpec(decl, func(context.Context, *Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	cmd.SetArgs([]string{"--wait"})
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("padded declaration must still reach the terminal status: %v", err)
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v, want success", code, emitted)
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Wait == nil {
		t.Fatal("registered ContractFinal lost the wait capability")
	}
	if _, ok := final.Wait.Terminal["COMPLETED"]; !ok {
		t.Fatalf("registered terminal table not trimmed: %#v", final.Wait.Terminal)
	}
	for _, value := range final.Wait.PendingValues {
		if strings.TrimSpace(value) != value {
			t.Fatalf("registered pending value %q not trimmed", value)
		}
	}
}

func TestNewPanicsOnDuplicateOrConflictingWaitStatusesAfterTrim(t *testing.T) {
	// Values that collapse onto one status after trimming are programming
	// errors: silently merging them would pick one outcome for two authored
	// declarations.
	dupTerminal := waitTestDecl()
	dupTerminal.Wait.Terminal = map[string]contract.ResultOutcome{
		"COMPLETED":  contract.ResultOutcomeSuccess,
		" COMPLETED": contract.ResultOutcomeFailure,
	}
	expectPanic(t, func() { New(baseWaitSpec(dupTerminal, nil)) }, "Contract.Wait")

	conflict := waitTestDecl()
	conflict.Wait.PendingValues = []string{" COMPLETED "}
	expectPanic(t, func() { New(baseWaitSpec(conflict, nil)) }, "Contract.Wait")
}

func TestContractDeclEmptyTreatsWaitAsAuthored(t *testing.T) {
	// Only Wait is authored: empty() must report non-empty through the Wait
	// branch (before validateContractDecl then fails on the missing prose).
	decl := ContractDecl{Wait: &contract.WaitSpec{
		Mode:        contract.WaitModePoll,
		PollCommand: "oa approval-instance get",
		StatusQuery: "result.status",
		Terminal:    map[string]contract.ResultOutcome{"COMPLETED": contract.ResultOutcomeSuccess},
	}}
	if decl.Empty() {
		t.Fatal("Wait-only declaration must count as authored")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected validateContractDecl to reject the missing prose")
		}
	}()
	validateContractDecl(Spec{Use: "wait-only", Contract: decl})
}

func TestEventModeRejectsNonObjectResultData(t *testing.T) {
	cmd := New(Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      eventTestDecl(contract.WaitModeEvent),
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending([]any{"not", "an", "object"}, &output.OperationInfo{
				ID: "job-1", State: "NEW", NextCommand: "dws wait-sample",
			}), nil
		},
		WaitEvents: func(context.Context, *Ctx) (wait.EventStream, error) {
			return &scriptedStream{}, nil
		},
	})
	cmd.SetArgs([]string{"--wait"})
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not an object") {
		t.Fatalf("err=%v, want non-object data rejection", err)
	}
}

func TestEventModeSubscriptionFailureSurfacesInStrictMode(t *testing.T) {
	decl := eventTestDecl(contract.WaitModeEvent)
	_, err := runWaitModeCommand(t, decl, nil, func(context.Context, *Ctx) (wait.EventStream, error) {
		return nil, errors.New("no subscriber credential")
	})
	if err == nil || !strings.Contains(err.Error(), "subscription failed") {
		t.Fatalf("err=%v, want subscription failure surfaced", err)
	}
}

func TestResultInvokeNonPendingSkipsWaitPhase(t *testing.T) {
	partial, err := output.NewPartialData(2,
		[]any{map[string]any{"id": "ok"}},
		[]output.PartialFailedEntry{{ID: "bad", Error: &output.ErrorInfo{Type: "api", Message: "item failed"}}},
		nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		result output.CommandResult
		want   string
	}{
		{"failure", output.Failure(&output.ErrorInfo{Type: "api", Message: "business failed"}), "failure"},
		{"success", output.Success(map[string]any{"id": "job-1"}), "success"},
		{"partial", output.Partial(partial), "partial_failure"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			polled := false
			subscribed := false
			cmd := New(Spec{
				Use:           "wait-sample",
				OutputRollout: output.RolloutUnifiedActive,
				Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
				Contract:      eventTestDecl(contract.WaitModeAuto),
				ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
					return tc.result, nil
				},
				WaitPoll: func(context.Context, *Ctx) (wait.PollDoc, error) {
					polled = true
					return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
				},
				WaitEvents: func(context.Context, *Ctx) (wait.EventStream, error) {
					subscribed = true
					return &scriptedStream{}, nil
				},
			})
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
			if polled || subscribed {
				t.Fatal("wait phase must not call WaitPoll/WaitEvents for a non-pending initial result")
			}
			if _, emitted := output.StoredExitCode(store); !emitted {
				t.Fatal("initial result was not stored")
			}
			if !strings.Contains(stdout.String(), `"outcome": "`+tc.want+`"`) {
				t.Fatalf("stdout=%s, want outcome %s preserved", stdout.String(), tc.want)
			}
			if strings.Contains(stdout.String(), `"type": "wait"`) {
				t.Fatalf("stdout=%s, wait phase overwrote the original envelope", stdout.String())
			}
		})
	}
}

func TestWaitTimeoutCancelsBlockingPoll(t *testing.T) {
	started := make(chan struct{})
	cmd := New(baseWaitSpec(waitTestDecl(), func(ctx context.Context, c *Ctx) (wait.PollDoc, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.Command().Context().Done():
			return nil, c.Command().Context().Err()
		}
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
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking poll never started")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("timeout wait must exit 0 (pending is not failure): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked poll was not cancelled by --wait-timeout")
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "pending"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestWaitTimeoutCancelsBlockingSubscribe(t *testing.T) {
	started := make(chan struct{})
	cmd := New(Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      eventTestDecl(contract.WaitModeEvent),
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending(map[string]any{"id": "job-1"}, &output.OperationInfo{
				ID: "job-1", State: "NEW", NextCommand: "dws wait-sample --id job-1",
			}), nil
		},
		WaitEvents: func(ctx context.Context, c *Ctx) (wait.EventStream, error) {
			close(started)
			// Leaf subscribe may wait on either the hook ctx or the cobra
			// command context; both must carry the wait-timeout deadline.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-c.Command().Context().Done():
				return nil, c.Command().Context().Err()
			}
		},
	})
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "1"})
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking subscribe never started")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("timeout wait must exit 0 (pending is not failure): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked subscribe was not cancelled by --wait-timeout")
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "pending"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestWaitTimeoutCancelsBlockingPollAfterAutoFallback(t *testing.T) {
	started := make(chan struct{})
	cmd := New(Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      eventTestDecl(contract.WaitModeAuto),
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending(map[string]any{"id": "job-1"}, &output.OperationInfo{
				ID: "job-1", State: "NEW", NextCommand: "dws wait-sample --id job-1",
			}), nil
		},
		WaitEvents: func(context.Context, *Ctx) (wait.EventStream, error) {
			return &scriptedStream{}, nil // ends immediately → poll fallback
		},
		WaitPoll: func(ctx context.Context, _ *Ctx) (wait.PollDoc, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "1"})
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("auto-fallback blocking poll never started")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("timeout wait must exit 0 (pending is not failure): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked auto-fallback poll was not cancelled by --wait-timeout")
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "pending"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestAttachContractRejectsWaitDeclaration(t *testing.T) {
	// The Tier2 overlay path (AttachContract / DeclareLeafMetadata) has no
	// Spec: it cannot pair WaitPoll/WaitEvents, register --wait flags, or run
	// the wait phase. Publishing the declaration there would advertise a
	// catalog capability the CLI rejects at flag parse.
	expectPanic(t, func() {
		AttachContract(newTestCommand(), testWriteSafety(), waitTestDecl(), "s", "l")
	}, "Contract.Wait on AttachContract")
	expectPanic(t, func() {
		AttachContract(nil, testWriteSafety(), waitTestDecl(), "s", "l")
	}, "Contract.Wait on AttachContract")

	// A wait declaration without a mode authors nothing (empty() ignores it)
	// and must not trip the rejection.
	decl := waitTestDecl()
	decl.Wait = &contract.WaitSpec{}
	AttachContract(newTestCommand(), testWriteSafety(), decl, "s", "l")
}

func TestNewManagedPathStillPublishesWaitCapability(t *testing.T) {
	// The managed New construction keeps publishing Contract.Wait into the
	// ContractFinal store: validateWaitDecl proved the hook pairing and
	// registerWaitFlags bound --wait before embedContractDecl runs.
	cmd := New(baseWaitSpec(waitTestDecl(), func(context.Context, *Ctx) (wait.PollDoc, error) {
		return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
	}))
	if cmd.Flags().Lookup("wait") == nil {
		t.Fatal("managed wait construction must register --wait")
	}
	final, ok := contractfinal.RuntimeContractFinal(cmd)
	if !ok || final.Wait == nil || final.Wait.Mode != contract.WaitModePoll {
		t.Fatalf("managed construction must publish the wait capability, final=%#v ok=%v", final.Wait, ok)
	}
}

func TestEventModeRejectsNilStreamWithoutError(t *testing.T) {
	// A leaf or subscription adapter returning (nil, nil) is a broken
	// subscription: strict event mode must fail through the unified error
	// envelope instead of panicking inside RunEvent's first Recv.
	_, err := runWaitModeCommand(t, eventTestDecl(contract.WaitModeEvent), nil,
		func(context.Context, *Ctx) (wait.EventStream, error) {
			return nil, nil
		})
	if err == nil || !strings.Contains(err.Error(), "subscription returned no stream") {
		t.Fatalf("nil stream without error must fail closed, got err=%v", err)
	}
}

func TestAutoModeFallsBackToPollOnNilStream(t *testing.T) {
	// Auto mode treats a (nil, nil) subscription like any other
	// subscription failure: fall back to polling under the same deadline.
	polled := false
	_, err := runWaitModeCommand(t, eventTestDecl(contract.WaitModeAuto),
		func(context.Context, *Ctx) (wait.PollDoc, error) {
			polled = true
			return wait.PollDoc{"result": map[string]any{"status": "COMPLETED"}}, nil
		},
		func(context.Context, *Ctx) (wait.EventStream, error) {
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !polled {
		t.Fatal("auto mode did not fall back to polling on nil stream")
	}
}

func TestAutoFallbackTimeoutKeepsEventPhaseLastStatus(t *testing.T) {
	// The event phase observes a pending status (RUNNING) before the stream
	// ends; the auto fallback poll then blocks until the shared deadline.
	// The timed-out envelope must report the event phase's RUNNING — not
	// regress to the accepted result's NEW.
	started := make(chan struct{})
	cmd := New(Spec{
		Use:           "wait-sample",
		OutputRollout: output.RolloutUnifiedActive,
		Safety:        contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"},
		Contract:      eventTestDecl(contract.WaitModeAuto),
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			return output.Pending(map[string]any{"id": "job-1"}, &output.OperationInfo{
				ID: "job-1", State: "NEW", NextCommand: "dws wait-sample --id job-1",
			}), nil
		},
		WaitEvents: func(context.Context, *Ctx) (wait.EventStream, error) {
			return &scriptedStream{events: []wait.PollDoc{
				{"process_instance_id": "job-1", "result": map[string]any{"status": "RUNNING"}},
			}}, nil // one pending event, then stream end → poll fallback
		},
		WaitPoll: func(ctx context.Context, _ *Ctx) (wait.PollDoc, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	cmd.SetArgs([]string{"--wait", "--wait-timeout", "1"})
	ctx, store := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("auto-fallback blocking poll never started")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("timeout wait must exit 0 (pending is not failure): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked auto-fallback poll was not cancelled by --wait-timeout")
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v", code, emitted)
	}
	emitted := stdout.String()
	if !strings.Contains(emitted, `"outcome": "pending"`) {
		t.Fatalf("stdout=%s", emitted)
	}
	if !strings.Contains(emitted, `"state": "RUNNING"`) || strings.Contains(emitted, `"state": "NEW"`) {
		t.Fatalf("timed-out envelope must keep the event phase's last status RUNNING, stdout=%s", emitted)
	}
	if !strings.Contains(emitted, `"timed_out": true`) {
		t.Fatalf("timed-out envelope must mark meta.operation.timed_out, stdout=%s", emitted)
	}
}

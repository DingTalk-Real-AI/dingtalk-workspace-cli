package corecmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestResultInvokeCarriesOneFrameworkResult(t *testing.T) {
	calls := 0
	ctx, store := output.WithResultStore(context.Background())
	cmd := New(Spec{
		Use:           "result",
		OutputRollout: output.RolloutUnifiedActive,
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent",
		},
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			calls++
			return output.Success(map[string]any{"id": "a"}), nil
		},
	})
	cmd.SetContext(ctx)
	cmd.PersistentFlags().String("format", "json", "")
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.PersistentPostRunE = func(executed *cobra.Command, _ []string) error {
		_, _, err := output.EmitStoredResult(executed)
		return err
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
	if code, emitted := output.StoredExitCode(store); !emitted || code != 0 {
		t.Fatalf("stored code/emitted=%d/%v", code, emitted)
	}
	if !strings.Contains(stdout.String(), `"outcome": "success"`) || strings.Contains(stdout.String(), `"contract_version"`) {
		t.Fatalf("stdout=%s", stdout.String())
	}
	if output.CommandRollout(cmd) != output.RolloutUnifiedActive {
		t.Fatalf("rollout=%s", output.CommandRollout(cmd))
	}
}

func TestFrameworkResultInvokeErrorLegacyAndStoreEdges(t *testing.T) {
	wantErr := errors.New("invoke failed")
	cases := []struct {
		name    string
		rollout output.RolloutState
		invoke  func(*Ctx, map[string]any) (output.CommandResult, error)
		want    string
	}{
		{"invoke error", output.RolloutUnifiedActive, func(*Ctx, map[string]any) (output.CommandResult, error) { return nil, wantErr }, "invoke failed"},
		{"legacy guard", output.RolloutLegacyOnly, func(*Ctx, map[string]any) (output.CommandResult, error) { return output.Success(nil), nil }, "without an active unified-result rollout"},
		{"missing store", output.RolloutUnifiedActive, func(*Ctx, map[string]any) (output.CommandResult, error) { return output.Success(nil), nil }, "no result store"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := New(Spec{Use: "result", OutputRollout: tc.rollout, Safety: contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}, ResultInvoke: tc.invoke})
			cmd.SetArgs(nil)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestLegacyResultInvokeIsRejectedBeforeBusinessDispatch(t *testing.T) {
	calls := 0
	cmd := New(Spec{
		Use:           "result",
		OutputRollout: output.RolloutLegacyOnly,
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high", Confirmation: "not_required", Idempotency: "unknown",
		},
		ResultInvoke: func(*Ctx, map[string]any) (output.CommandResult, error) {
			calls++
			return output.Success(map[string]any{"changed": true}), nil
		},
	})
	cmd.SetArgs(nil)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "without an active unified-result rollout") {
		t.Fatalf("Execute error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("business dispatcher ran %d time(s), want 0", calls)
	}
}

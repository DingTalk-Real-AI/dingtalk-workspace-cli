package corecmd

import (
	"bytes"
	"context"
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

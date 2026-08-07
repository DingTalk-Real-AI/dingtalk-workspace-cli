package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateRolloutTransition(t *testing.T) {
	cases := []struct {
		from, to RolloutState
		rollback bool
		wantErr  bool
	}{
		{RolloutLegacyOnly, RolloutDualValidate, false, false},
		{RolloutDualValidate, RolloutV2Active, false, false},
		{RolloutV2Active, RolloutV2Stable, false, false},
		{RolloutV2Stable, RolloutV2Only, false, false},
		{RolloutLegacyOnly, RolloutV2Active, false, true},
		{RolloutV2Stable, RolloutV2Active, false, true},
		{RolloutV2Stable, RolloutV2Active, true, false},
	}
	for _, tc := range cases {
		if err := ValidateRolloutTransition(tc.from, tc.to, tc.rollback); (err != nil) != tc.wantErr {
			t.Fatalf("ValidateRolloutTransition(%s,%s,rollback=%v) err=%v, wantErr=%v", tc.from, tc.to, tc.rollback, err, tc.wantErr)
		}
	}
}

func TestAdaptMCPUsesSameEnvelope(t *testing.T) {
	result := Success(map[string]any{"id": "a"})
	mcp, err := AdaptMCP(result)
	if err != nil {
		t.Fatal(err)
	}
	if mcp.IsError {
		t.Fatal("success must not set MCP isError")
	}
	if got := mcp.StructuredContent["contract_version"]; got != ContractVersionV2 {
		t.Fatalf("contract_version=%v", got)
	}
	if got := mcp.StructuredContent["outcome"]; got != string(OutcomeSuccess) {
		t.Fatalf("outcome=%v", got)
	}
}

func TestCommandResultDetachesMutableFrameworkPayload(t *testing.T) {
	payload := map[string]any{"nested": map[string]any{"value": "before"}}
	result := Success(payload)
	payload["nested"].(map[string]any)["value"] = "after"
	env, err := EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	got := env.Data.(map[string]any)["nested"].(map[string]any)["value"]
	if got != "before" {
		t.Fatalf("detached payload value=%v, want before", got)
	}
}

func TestFailureExitCodeIsFrameworkDerived(t *testing.T) {
	result := Failure(&ErrorInfo{Type: "validation", ExitCode: 99, Message: "bad input"})
	if result.ExitCode() != 3 {
		t.Fatalf("normal failure exit code=%d, want framework validation rc=3", result.ExitCode())
	}
	env, err := EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.ExitCode != 3 {
		t.Fatalf("wire error=%+v, want exit_code=3", env.Error)
	}
}

func TestRootCompatibilityAdapterCanPreserveSignalExitCode(t *testing.T) {
	result := FailureWithExitCode(&ErrorInfo{Type: "internal", Message: "cancelled"}, 130)
	if err := ValidateResult(result); err != nil {
		t.Fatalf("ValidateResult: %v", err)
	}
	if result.ExitCode() != 130 {
		t.Fatalf("compatibility exit code=%d, want 130", result.ExitCode())
	}
	env, _ := EnvelopeFromResult(result)
	if env.Error.ExitCode != 130 {
		t.Fatalf("wire exit_code=%d, want 130", env.Error.ExitCode)
	}
}

func TestValidateResultRejectsMalformedPendingPartialAndPagination(t *testing.T) {
	cases := []struct {
		name   string
		result CommandResult
	}{
		{"pending nil operation", Pending(map[string]any{"id": "x"}, nil)},
		{"pending empty operation", Pending(nil, &OperationInfo{})},
		{"partial nil", Partial(nil)},
		{"pagination exhausted with token", Success([]any{}, WithMeta(&Meta{Pagination: &Pagination{EndpointExhausted: true, NextToken: "next"}}))},
		{"pagination open without token", Success([]any{}, WithMeta(&Meta{Pagination: &Pagination{EndpointExhausted: false}}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateResult(tc.result); err == nil {
				t.Fatalf("ValidateResult(%s) succeeded", tc.name)
			}
		})
	}
}

func TestPartialRequiresTypedPerItemError(t *testing.T) {
	for _, entry := range []PartialFailedEntry{
		{ID: "b"},
		{ID: "b", Error: &ErrorInfo{}},
	} {
		if _, err := NewPartialData(2, []any{map[string]any{"id": "a"}}, []PartialFailedEntry{entry}, nil); err == nil {
			t.Fatalf("NewPartialData accepted malformed failed entry: %+v", entry)
		}
	}
}

func TestEmitResultRejectsUnknownFormatAndKeepsOutputEmpty(t *testing.T) {
	cmd := &cobra.Command{Use: "sample"}
	output := new(bytes.Buffer)
	cmd.SetOut(output)
	cmd.SetErr(new(bytes.Buffer))
	cmd.Flags().String("format", "bogus", "")
	SetCommandRollout(cmd, RolloutV2Active)
	code, err := EmitResult(cmd, Success(map[string]any{"id": "a"}))
	if err == nil || code != 3 || !strings.Contains(err.Error(), "unsupported --format") {
		t.Fatalf("EmitResult code=%d err=%v, want validation", code, err)
	}
	if output.Len() != 0 {
		t.Fatalf("invalid format leaked output: %q", output.String())
	}
}

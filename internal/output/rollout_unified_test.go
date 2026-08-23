package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		{RolloutDualValidate, RolloutUnifiedActive, false, false},
		{RolloutUnifiedActive, RolloutUnifiedStable, false, false},
		{RolloutUnifiedStable, RolloutUnifiedOnly, false, false},
		{RolloutLegacyOnly, RolloutUnifiedActive, false, true},
		{RolloutUnifiedStable, RolloutUnifiedActive, false, true},
		{RolloutUnifiedStable, RolloutUnifiedActive, true, false},
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
	if _, exists := mcp.StructuredContent["contract_version"]; exists {
		t.Fatalf("structured content must not expose contract_version: %#v", mcp.StructuredContent)
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

func TestEmitResultUnknownFormatDegradesToJSONWithWarning(t *testing.T) {
	cmd := &cobra.Command{Use: "sample"}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().String("format", "bogus", "")
	SetCommandRollout(cmd, RolloutUnifiedActive)
	code, err := EmitResult(cmd, Success(map[string]any{"id": "a"}))
	if err != nil || code != 0 {
		t.Fatalf("EmitResult code=%d err=%v, want successful JSON fallback", code, err)
	}
	if !strings.Contains(stdout.String(), `"outcome": "success"`) {
		t.Fatalf("fallback output is not a success envelope: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[WARN]") || !strings.Contains(stderr.String(), "bogus") {
		t.Fatalf("fallback warning missing: %q", stderr.String())
	}
}

func TestPendingWithMetaDoesNotMutateCallerMetadata(t *testing.T) {
	callerOperation := &OperationInfo{ID: "original", State: "waiting", NextCommand: "dws original"}
	meta := &Meta{Operation: callerOperation}
	want := &OperationInfo{ID: "new", State: "processing", NextCommand: "dws status"}
	result := Pending(nil, want, WithMeta(meta))

	if meta.Operation != callerOperation || meta.Operation.ID != "original" {
		t.Fatalf("Pending mutated caller metadata: %+v", meta.Operation)
	}
	env, err := EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Meta.Operation.ID != "new" {
		t.Fatalf("result operation=%+v, want new operation", env.Meta.Operation)
	}
}

type writeThenError struct {
	accepted int
	writes   int
	full     bool
}

func (w *writeThenError) Write(p []byte) (int, error) {
	w.writes++
	n := len(p)
	if !w.full {
		n /= 2
	}
	w.accepted += n
	return n, io.ErrClosedPipe
}

func TestEmitStoredResultRecordsAttemptAndByteRiskOnWriteError(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprintf("full=%t", full), func(t *testing.T) {
			ctx, store := WithResultStore(context.Background())
			cmd := &cobra.Command{Use: "sample"}
			SetCommandRollout(cmd, RolloutUnifiedActive)
			writer := &writeThenError{full: full}
			cmd.SetOut(writer)
			cmd.SetErr(io.Discard)
			cmd.SetContext(ctx)
			if err := StoreResult(ctx, Success(map[string]any{"id": "a"})); err != nil {
				t.Fatal(err)
			}
			if _, _, err := EmitStoredResult(cmd); err == nil {
				t.Fatal("write error was not returned")
			}
			code, attempted, emitted, bytesRisk := StoredEmissionState(store)
			if code != exitCodeInternal || !attempted || emitted || !bytesRisk {
				t.Fatalf("state=(%d,%t,%t,%t), want (%d,true,false,true)", code, attempted, emitted, bytesRisk, exitCodeInternal)
			}
			if _, _, err := EmitStoredResult(cmd); err != nil {
				t.Fatalf("second call should report existing state without writing: %v", err)
			}
			if writer.writes != 1 {
				t.Fatalf("writer called %d times, want one emission attempt", writer.writes)
			}
		})
	}
}

func TestEmitStoredResultFallsBackToTypedFailureBeforeAnyBytesAreWritten(t *testing.T) {
	ctx, store := WithResultStore(context.Background())
	cmd := &cobra.Command{Use: "sample"}
	SetCommandRollout(cmd, RolloutUnifiedActive)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	cmd.SetContext(ctx)
	if err := StoreResult(ctx, Success(map[string]any{"unsupported": make(chan int)})); err != nil {
		t.Fatal(err)
	}

	code, emitted, err := EmitStoredResult(cmd)
	if err != nil || code != exitCodeInternal || !emitted {
		t.Fatalf("EmitStoredResult=(%d,%t,%v), want (%d,true,nil)", code, emitted, err, exitCodeInternal)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("fallback stdout is not one JSON envelope: %v\n%s", err, stdout.String())
	}
	if env.Outcome != OutcomeFailure || env.Error == nil || env.Error.Type != "internal" ||
		env.Error.Subtype != "result_encoding_failed" {
		t.Fatalf("fallback envelope=%+v", env)
	}
	if code, attempted, emitted, bytesRisk := StoredEmissionState(store); code != exitCodeInternal || !attempted || !emitted || !bytesRisk {
		t.Fatalf("state=(%d,%t,%t,%t)", code, attempted, emitted, bytesRisk)
	}
}

func TestWithResultStoreIsIdempotent(t *testing.T) {
	ctx, first := WithResultStore(context.Background())
	ctxAgain, second := WithResultStore(ctx)
	if ctxAgain != ctx || second != first {
		t.Fatal("WithResultStore replaced an existing store")
	}
}

func TestStoreResultWithoutExecutionBoundaryHasActionableDiagnostic(t *testing.T) {
	err := StoreResult(context.Background(), Success(map[string]any{"id": "a"}))
	if err == nil || !strings.Contains(err.Error(), "output.WithResultStore") ||
		!strings.Contains(err.Error(), "root execution boundary") {
		t.Fatalf("StoreResult diagnostic = %v, want execution-boundary repair hint", err)
	}
}

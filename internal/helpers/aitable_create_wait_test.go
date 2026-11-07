// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func aitableWaitCommand(t *testing.T, flags ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	declareAitableCreateWaitFlags(cmd)
	if err := cmd.ParseFlags(flags); err != nil {
		t.Fatal(err)
	}
	return cmd
}

type aitableNilCreateCaller struct{ aitableTestCaller }

func (c *aitableNilCreateCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, aitableTestCall{server: server, tool: tool, args: args})
	return nil, nil
}

func TestCrossPlatformCoverageAitableCreateWaitUnknownOutcome(t *testing.T) {
	args := map[string]any{"baseId": "b", "tableId": "t"}
	for _, original := range []error{
		context.DeadlineExceeded, errors.New("connection reset"),
		apperrors.NewAPI("timeout", apperrors.WithRetryable(true), apperrors.WithHint("retry")),
		apperrors.NewInternal("decode failed"),
		apperrors.NewValidation("post-dispatch failure", apperrors.WithExecutionStarted(true)),
	} {
		t.Run(original.Error(), func(t *testing.T) {
			caller := &aitableTestCaller{errors: []error{original}}
			installAitableDeps(t, caller)
			err := callAitableCreateWithWait(aitableWaitCommand(t, "--wait"), "create_fields", args, nil)
			assertAitableCreateUnknown(t, err)
			var structured *apperrors.Error
			errors.As(err, &structured)
			if len(caller.calls) != 1 || structured.Details["baseId"] != "b" || structured.Details["tableId"] != "t" || !errors.Is(err, original) {
				t.Fatalf("lost scope/cause or replayed: %#v / %v", caller.calls, err)
			}
		})
	}
	t.Run("nil response", func(t *testing.T) {
		installAitableDeps(t, &aitableTestCaller{})
		caller := &aitableNilCreateCaller{}
		testseam.Swap(t, &deps.Caller, edition.ToolCaller(caller))
		err := callAitableCreateWithWait(aitableWaitCommand(t, "--wait"), "create_fields", args, nil)
		assertAitableCreateUnknown(t, err)
		if len(caller.calls) != 1 {
			t.Fatalf("calls: %#v", caller.calls)
		}
	})
	for _, original := range []error{
		apperrors.NewAPI("not sent", apperrors.WithExecutionStarted(false)),
		apperrors.NewAuth("not logged in"), apperrors.NewValidation("invalid"),
		apperrors.NewDiscovery("not discovered"),
		&PATError{RawJSON: `{"error":"denied"}`},
		&CLIError{Code: CodeAuthTokenExpired}, &CLIError{Code: CodeInvalidParam},
	} {
		t.Run("pre-dispatch/"+original.Error(), func(t *testing.T) {
			if got := aitableCreateCallError("create_fields", args, original); got != original {
				t.Fatalf("pre-dispatch error reclassified: %v", got)
			}
		})
	}
	t.Run("cancelled before dispatch", func(t *testing.T) {
		caller := &aitableTestCaller{}
		installAitableDeps(t, caller)
		cmd := aitableWaitCommand(t, "--wait")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd.SetContext(ctx)
		err := callAitableCreateWithWait(cmd, "create_table", args, nil)
		var structured *apperrors.Error
		if !errors.As(err, &structured) || structured.ExecutionStarted == nil || *structured.ExecutionStarted || len(caller.calls) != 0 || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled creation dispatched: %v / %#v", err, caller.calls)
		}
	})
}

func assertAitableCreateUnknown(t *testing.T, err error) {
	t.Helper()
	var structured *apperrors.Error
	if !errors.As(err, &structured) || structured.Reason != "create_outcome_unknown" || structured.Retryable || !structured.RetryableSet || structured.ExecutionStarted == nil || !*structured.ExecutionStarted || !strings.Contains(structured.Hint, "只读") || strings.Contains(structured.Hint, "Retry later") {
		t.Fatalf("unsafe unknown-write recovery: %#v / %v", structured, err)
	}
}

func TestCrossPlatformCoverageAitableCreateWaitMinimumTimeoutAndEmptyConfig(t *testing.T) {
	// Exercise the real timer: a no-op wait stub masks the one-second regression.
	caller := &aitableTestCaller{responses: []string{
		`{"results":[{"success":true,"fieldId":"f"}]}`,
		`{"fields":[{"fieldId":"f","fieldName":"N","type":"text","config":null}]}`,
		`{"fields":[{"fieldId":"f","fieldName":"N","type":"text"}]}`,
	}}
	out := installAitableDeps(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SetArgs([]string{"field", "create", "--base-id=b", "--table-id=t", "--name=N", "--type=text", "--config={}", "--wait", "--wait-timeout=1"})
	if err := corecmd.ExecuteForTest(root); err != nil || len(caller.calls) != 3 || !strings.Contains(out.String(), `"observed"`) {
		t.Fatalf("minimum wait/empty config: %v / %s / %#v", err, out, caller.calls)
	}
}

func TestCrossPlatformCoverageAitableCreateWaitNormalizedExpectations(t *testing.T) {
	for _, optional := range []map[string]any{
		{"config": map[string]any{}, "description": ""}, {"config": nil, "description": nil},
	} {
		field := map[string]any{"fieldName": "N", "type": "text"}
		for key, value := range optional {
			field[key] = value
		}
		actual := []any{map[string]any{"fieldId": "f", "fieldName": "N", "type": "text"}}
		if err := matchAitableCreatedFields(actual, []string{"f"}, []any{field}); err != nil {
			t.Fatal(err)
		}
		for key, value := range optional {
			if got, exists := field[key]; !exists || !reflect.DeepEqual(got, value) {
				t.Fatalf("request mutated: %#v", field)
			}
		}
	}
	// Nonempty/invalid configs and meaningful zero/false values stay strict.
	for _, property := range []map[string]any{
		{"config": "invalid"}, {"description": "required"},
		{"config": map[string]any{"min": float64(0), "multiple": false}},
		{"config": map[string]any{"options": []any{map[string]any{"name": "A"}}}},
		{"aiConfig": map[string]any{"autoRecompute": false}},
	} {
		if !reflect.DeepEqual(aitableCreatedFieldExpectation(property), property) {
			t.Fatalf("meaningful expectation erased: %#v", property)
		}
		property["fieldName"] = "N"
		if err := matchAitableCreatedFields([]any{map[string]any{"fieldId": "f", "fieldName": "N"}}, nil, []any{property}); err == nil {
			t.Fatalf("missing properties accepted: %#v", property)
		}
	}
}

func TestCrossPlatformCoverageAitableCreateWaitCLI(t *testing.T) {
	testseam.Swap(t, &aitableCreateReadbackWait, func(context.Context, time.Duration) error { return nil })
	for _, kind := range []string{"table", "field"} {
		t.Run(kind, func(t *testing.T) {
			receipt := `{"status":"success","data":{"tableId":"t"}}`
			args := []string{kind, "create", "--base-id=b", `--fields=[{"fieldName":"N","type":"text"}]`, "--wait"}
			if kind == "field" {
				receipt = `{"status":"success","data":{"results":[{"success":true,"fieldId":"f","verificationStatus":"pending"}]}}`
				args = append(args, "--table-id=t")
			} else {
				args = append(args, "--name=T")
			}
			visible := `{"data":{"fields":[{"fieldId":"f","fieldName":"N","type":"text"}]}}`
			caller := &aitableTestCaller{responses: []string{receipt, visible, `{"data":{"fields":[]}}`, visible, visible}}
			out := installAitableDeps(t, caller)
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			root.SetArgs(args)
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			if len(caller.calls) != 5 {
				t.Fatalf("calls: %#v", caller.calls)
			}
			for i, call := range caller.calls {
				if i > 0 && call.tool != "get_fields" {
					t.Fatalf("replayed write: %#v", call)
				}
				if _, ok := call.args["wait"]; ok {
					t.Fatal("local flag leaked to MCP")
				}
			}
			if !strings.Contains(out.String(), `"observed"`) {
				t.Fatalf("missing readiness: %s", out)
			}
			if kind == "field" && !strings.Contains(out.String(), `"pending"`) {
				t.Fatalf("upstream receipt overwritten: %s", out)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableCreateWaitFailures(t *testing.T) {
	fields := []any{map[string]any{"fieldName": "N", "type": "text"}}
	for _, flags := range [][]string{{"--wait-timeout=0"}, {"--wait-timeout=121", "--wait"}, {"--wait-timeout=1"}} {
		caller := &aitableTestCaller{}
		installAitableDeps(t, caller)
		if err := callAitableCreateWithWait(aitableWaitCommand(t, flags...), "create_table", nil, fields); err == nil || len(caller.calls) != 0 {
			t.Fatalf("validation: %v / %#v", err, caller.calls)
		}
	}
	for _, tc := range []struct {
		tool, receipt string
		args          map[string]any
	}{
		{"create_table", `{}`, map[string]any{"baseId": "b"}},
		{"create_fields", `{"results":[]}`, map[string]any{"baseId": "b", "tableId": "t"}},
		{"create_fields", `{"results":[{"success":true}]}`, map[string]any{"baseId": "b", "tableId": "t"}},
	} {
		caller := &aitableTestCaller{responses: []string{tc.receipt}}
		installAitableDeps(t, caller)
		err := callAitableCreateWithWait(aitableWaitCommand(t, "--wait"), tc.tool, tc.args, fields)
		var structured *apperrors.Error
		if !errors.As(err, &structured) || structured.Details["receipt"] == nil || structured.Reason != "create_readback_unconfirmed" || len(caller.calls) != 1 {
			t.Fatalf("receipt failure: %v / %#v", err, caller.calls)
		}
		if structured.Retryable || !structured.RetryableSet || structured.ExecutionStarted == nil || !*structured.ExecutionStarted {
			t.Fatalf("unconfirmed creation must not advertise replay: %#v", structured)
		}
	}
	testseam.Swap(t, &aitableCreateReadbackWait, func(context.Context, time.Duration) error { return context.DeadlineExceeded })
	caller := &aitableTestCaller{responses: []string{`{"tableId":"t"}`, `{"fields":[]}`}}
	installAitableDeps(t, caller)
	err := callAitableCreateWithWait(aitableWaitCommand(t, "--wait"), "create_table", map[string]any{"baseId": "b"}, fields)
	var structured *apperrors.Error
	if !errors.As(err, &structured) || structured.Details["tableId"] != "t" || len(caller.calls) != 2 {
		t.Fatalf("timeout lost receipt: %v", err)
	}
	caller = &aitableTestCaller{errors: []error{errors.New("connection lost")}}
	installAitableDeps(t, caller)
	if err := callAitableCreateWithWait(aitableWaitCommand(t, "--wait"), "create_table", nil, fields); err == nil || len(caller.calls) != 1 {
		t.Fatalf("write error: %v", err)
	}
	for _, dry := range []bool{false, true} {
		caller := &aitableTestCaller{dryRun: dry}
		installAitableDeps(t, caller)
		flags := []string{}
		if dry {
			flags = append(flags, "--wait")
		}
		if err := callAitableCreateWithWait(aitableWaitCommand(t, flags...), "create_table", map[string]any{"baseId": "b"}, nil); err != nil {
			t.Fatal(err)
		}
		if (!dry && len(caller.calls) != 1) || (dry && len(caller.calls) != 0) {
			t.Fatalf("dry/nonwait calls: %#v", caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableCreateWaitReadbackPrimitives(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := aitableCreateReadbackWait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := aitableCreateReadbackWait(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := waitAitableCreatedFields(ctx, "b", "t", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, data := range []map[string]any{
		{}, {"results": []any{}}, {"results": []any{nil}},
		{"results": []any{map[string]any{"success": false, "fieldId": "f"}}},
		{"results": []any{map[string]any{"success": true, "fieldId": "f"}, map[string]any{"success": true, "fieldId": "f"}}},
	} {
		count := 1
		if results, ok := data["results"].([]any); ok {
			count = len(results)
		}
		if _, err := aitableCreatedFieldIDs(data, count); err == nil {
			t.Fatalf("invalid receipt accepted: %#v", data)
		}
	}
	ids := make([]string, 11)
	first, second := []any{}, []any{}
	for i := range ids {
		ids[i] = fmt.Sprintf("f%d", i)
		field := map[string]any{"fieldId": ids[i]}
		if i < 10 {
			first = append(first, field)
		} else {
			second = append(second, field)
		}
	}
	encode := func(fields []any) string {
		raw, _ := json.Marshal(map[string]any{"fields": fields})
		return string(raw)
	}
	caller := &aitableTestCaller{responses: []string{encode(first), encode(second)}}
	installAitableDeps(t, caller)
	got, err := readAitableCreatedFields(context.Background(), "b", "t", ids)
	if err != nil || len(got) != 11 || len(caller.calls[0].args["fieldIds"].([]string)) != 10 || len(caller.calls[1].args["fieldIds"].([]string)) != 1 {
		t.Fatalf("readback batching: %#v / %v", caller.calls, err)
	}
	for _, caller := range []*aitableTestCaller{{responses: []string{`{}`}}, {errors: []error{errors.New("read failed")}}} {
		installAitableDeps(t, caller)
		if _, err := readAitableCreatedFields(context.Background(), "b", "t", nil); err == nil {
			t.Fatal("invalid readback accepted")
		}
	}
	want := []any{map[string]any{"fieldName": "N", "type": "text"}}
	if err := matchAitableCreatedFields(nil, nil, want); err == nil {
		t.Fatal("empty readback accepted")
	}
	if err := matchAitableCreatedFields([]any{map[string]any{"fieldId": "f", "fieldName": "N", "type": "number"}}, nil, want); err == nil {
		t.Fatal("wrong type accepted")
	}
	if err := matchAitableCreatedFields([]any{map[string]any{"fieldId": "f", "fieldName": "N", "type": "text"}}, nil, want); err != nil {
		t.Fatal(err)
	}
	for _, actual := range [][]any{{nil}, {map[string]any{"fieldId": "f"}, map[string]any{"fieldId": "f"}}} {
		if err := matchAitableCreatedFields(actual, nil, nil); err == nil {
			t.Fatalf("invalid field directory accepted: %#v", actual)
		}
	}
	for _, tc := range []struct {
		got, want any
		matches   bool
	}{
		{nil, map[string]any{}, false}, {map[string]any{}, map[string]any{"x": 1}, false},
		{map[string]any{"x": 1, "y": 2}, map[string]any{"x": 1}, true},
		{nil, []any{}, false}, {[]any{1}, []any{}, false}, {[]any{1}, []any{2}, false},
		{[]any{map[string]any{"x": 1, "id": "a"}}, []any{map[string]any{"x": 1}}, true},
	} {
		if got := aitableDeclaredSubset(tc.got, tc.want); got != tc.matches {
			t.Fatalf("subset %#v / %#v = %v", tc.got, tc.want, got)
		}
	}
	if err := renderAitableReceipt("test", make(chan int)); err == nil {
		t.Fatal("invalid JSON should fail")
	}
}

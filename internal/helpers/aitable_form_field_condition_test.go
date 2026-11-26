// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

const testFormCondition = `{"operator":"and","operands":[{"operator":"eq","operands":["source","yes"]}]}`

func TestCrossPlatformCoverageFormFieldAtomicValidationAndFalse(t *testing.T) {
	installAitableDeps(t, &aitableTestCaller{})
	for _, flags := range [][]string{nil, {"--clear-display-condition=false"}, {"--display-condition={}"},
		{"--field-description="}, {"--field-description= "},
		{"--required="}, {"--required= ", "--field-description=example"},
		{"--display-condition=" + testFormCondition, "--clear-display-condition"},
		{`--display-condition={"operator":"eq","operands":["source","yes"]}`},
		{`--display-condition={"operator":"and","operands":[]}`},
		{`--display-condition={"operator":"and","operands":[{"operator":"eq","operands":["source"]}]}`},
		{`--display-condition={"operator":"and","operands":[{"operator":"eq","operands":[" ","yes"]}]}`},
		{`--display-condition={"operator":"and","operands":[{"operator":"ne","operands":["f","o"]}]}`}} {
		caller := &aitableTestCaller{}
		args := append([]string{"form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f"}, flags...)
		if err := runAitableCoverageCommand(t, caller, args...); err == nil {
			t.Fatalf("accepted invalid flags: %v", flags)
		}
		if len(caller.calls) != 0 {
			t.Fatalf("invalid input called MCP: %#v", caller.calls)
		}
	}
	caller := &aitableTestCaller{}
	if err := runAitableCoverageCommand(t, caller, "form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--required=false"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].args["required"] != false {
		t.Fatalf("explicit false lost: %#v", caller.calls)
	}
}

func TestCrossPlatformCoverageFormFieldAtomicLegacyRequiredValues(t *testing.T) {
	installAitableDeps(t, &aitableTestCaller{})
	for _, tc := range []struct {
		name    string
		flags   []string
		updates map[string]any
	}{
		{"trim true", []string{"--required= true "}, map[string]any{"required": true}},
		{"trim false", []string{"--required=\tFalse\n"}, map[string]any{"required": false}},
		{"empty omitted", []string{"--required=", "--field-description=example"}, map[string]any{"fieldDescription": "example"}},
		{"empty description omitted", []string{"--field-description=", "--required=false"}, map[string]any{"required": false}},
		{"description whitespace preserved", []string{"--field-description= example ", "--required=false"}, map[string]any{"fieldDescription": " example ", "required": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableTestCaller{}
			args := append([]string{"form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f"}, tc.flags...)
			if err := runAitableCoverageCommand(t, caller, args...); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f"}
			for key, value := range tc.updates {
				want[key] = value
			}
			if len(caller.calls) != 1 || !reflect.DeepEqual(caller.calls[0].args, want) {
				t.Fatalf("legacy required value changed the request: calls=%#v want=%#v", caller.calls, want)
			}
		})
	}
}

func TestCrossPlatformCoverageFormFieldCallerPreviewDoesNotCallMCP(t *testing.T) {
	caller := &aitableTestCaller{dryRun: true}
	installAitableDeps(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--required=false", "--format=json"})
	cmd, err := corecmd.ExecuteCForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted || code != 0 || len(caller.calls) != 0 {
		t.Fatalf("code=%d emitted=%v err=%v calls=%d", code, emitted, err, len(caller.calls))
	}
	var envelope struct {
		DryRun bool `json:"dry_run"`
		Data   struct {
			Executed  bool           `json:"executed"`
			Arguments map[string]any `json:"arguments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.DryRun || envelope.Data.Executed || envelope.Data.Arguments["required"] != false {
		t.Fatalf("preview lost explicit false or execution state: %s", stdout)
	}
}

func TestCrossPlatformCoverageFormFieldOrganizationErrorKeepsCause(t *testing.T) {
	for _, signal := range []map[string]any{
		{"code": "601"}, {"code": "ORG_NOT_MATCH"}, {"code": "FORBIDDEN", "type": "ORG_DOC_TOKEN_MISMATCH"},
	} {
		signal["traceId"] = " trace-form-rejection "
		signal["retryable"] = true
		raw, err := json.Marshal(map[string]any{"status": "error", "error": signal})
		if err != nil {
			t.Fatal(err)
		}
		original := &CLIError{Code: CodeMCPToolError, Message: string(raw)}
		got := aitableFormFieldUpdateError(original)
		var typed *apperrors.Error
		if !errors.As(got, &typed) || typed.Reason != "organization_mismatch" || typed.Retryable || typed.ExitCode() != ExitAuth {
			t.Fatalf("unexpected form classification: %#v", got)
		}
		if !errors.Is(got, original) || typed.ServerKey != "aitable-helper" || typed.ServerDiag.TraceID != "trace-form-rejection" || typed.ServerDiag.ServerErrorCode != signal["code"] || typed.Operation != "aitable-helper/update_form_field" {
			t.Fatalf("form error lost original receipt or route: %#v", typed)
		}
	}
	for _, original := range []error{
		errors.New("transport failure"),
		&CLIError{Code: CodeMCPToolError, Message: "not JSON"},
		&CLIError{Code: CodeMCPToolError, Message: `{"status":"success","data":{"code":"601"}}`},
		&CLIError{Code: CodeMCPToolError, Message: `{"status":"error","error":{"code":"403","message":"org mismatch"}}`},
	} {
		if got := aitableFormFieldUpdateError(original); got != original {
			t.Fatalf("form path changed an unrelated error: %#v", got)
		}
	}
}

func TestCrossPlatformCoverageFormFieldOrganizationRejectionCallsOnce(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"status":"error","error":{"code":"601","type":"ORG_DOC_TOKEN_MISMATCH","retryable":true}}`}}
	installAitableDeps(t, caller)
	err := runAitableCoverageCommand(t, caller, "form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--clear-display-condition")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Reason != "organization_mismatch" || typed.Retryable || typed.ExitCode() != ExitAuth || len(caller.calls) != 1 {
		t.Fatalf("err=%#v calls=%d", err, len(caller.calls))
	}
}

func TestCrossPlatformCoverageFormFieldAtomicTerminalResults(t *testing.T) {
	for _, state := range []string{"verified", "pending", "unknown", "wrong-target", "missing-condition", "invalid-status", "missing-status", "partial-error", "partial-error-601", "legacy"} {
		t.Run(state, func(t *testing.T) {
			var condition map[string]any
			if err := json.Unmarshal([]byte(testFormCondition), &condition); err != nil {
				t.Fatal(err)
			}
			actual := map[string]any{"fieldId": "f", "required": true, "hidden": false, "displayConditionStatus": "available", "displayCondition": condition}
			data := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f", "verificationStatus": "verified", "actual": actual, "completedOperations": []any{"DISPLAY_FILTERS"}}
			envelope := map[string]any{"status": "success", "data": data}
			switch state {
			case "pending", "unknown":
				data["verificationStatus"] = state
			case "invalid-status":
				envelope["status"] = "pending"
			case "missing-status":
				delete(envelope, "status")
			case "wrong-target":
				data["fieldId"] = map[string]any{"bad": true}
			case "missing-condition":
				delete(actual, "displayCondition")
			case "partial-error":
				data["verificationStatus"] = "unknown"
				envelope["status"] = "error"
				envelope["error"] = map[string]any{"type": "SYSTEM_ERROR", "code": "FORM_FIELD_UPDATE_OUTCOME_UNKNOWN", "message": "unknown", "retryable": false}
			case "partial-error-601":
				data["verificationStatus"] = "unknown"
				envelope["status"] = "error"
				envelope["error"] = map[string]any{"type": "ORG_DOC_TOKEN_MISMATCH", "code": "601", "retryable": false}
			case "legacy":
				delete(data, "verificationStatus")
			}
			raw, _ := json.Marshal(envelope)
			caller := &aitableTestCaller{responses: []string{string(raw)}}
			installAitableDeps(t, caller)
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			stdout := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"form", "field", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--display-condition=" + testFormCondition, "--format=json"})
			cmd, err := corecmd.ExecuteCForTest(root)
			if err != nil {
				t.Fatal(err)
			}
			code, emitted, err := output.EmitStoredResult(cmd)
			wantCode := 7
			if state == "verified" {
				wantCode = 0
			}
			if err != nil || !emitted || code != wantCode || len(caller.calls) != 1 {
				t.Fatalf("code=%d want=%d emitted=%v err=%v calls=%d output=%s", code, wantCode, emitted, err, len(caller.calls), stdout)
			}
			var emittedEnvelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &emittedEnvelope); err != nil {
				t.Fatal(err)
			}
			if emittedEnvelope["ok"] != (state == "verified") {
				t.Fatalf("wrong success: %s", stdout)
			}
		})
	}
}

// A missing condition confirms a clear only with an available state and a verified, matching target.
func TestCrossPlatformCoverageFormFieldClearWithOmittedNull(t *testing.T) {
	for _, state := range []string{"available-missing", "available-null", "available-condition", "unknown-missing", "unknown-null", "unsupported", "missing-state", "wrong-target", "pending", "error", "missing-required", "missing-hidden"} {
		t.Run(state, func(t *testing.T) {
			actual := map[string]any{"fieldId": "f", "required": true, "hidden": false, "displayConditionStatus": "available"}
			data := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f", "verificationStatus": "verified", "actual": actual}
			envelope := map[string]any{"status": "success", "data": data}
			switch state {
			case "available-null":
				actual["displayCondition"] = nil
			case "available-condition":
				actual["displayCondition"] = map[string]any{"operator": "and"}
			case "unknown-missing", "unknown-null":
				actual["displayConditionStatus"] = "unknown"
				if state == "unknown-null" {
					actual["displayCondition"] = nil
				}
			case "unsupported":
				actual["displayConditionStatus"] = "unsupported"
			case "missing-state":
				delete(actual, "displayConditionStatus")
			case "wrong-target":
				data["fieldId"] = "other"
			case "pending":
				data["verificationStatus"] = "pending"
			case "error":
				envelope["status"] = "error"
			case "missing-required":
				delete(actual, "required")
			case "missing-hidden":
				delete(actual, "hidden")
			}
			request := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f", "clearDisplayCondition": true}
			result := AitableFormFieldUpdateResult(envelope, request)
			wantCode := 7
			if state == "available-missing" || state == "available-null" {
				wantCode = 0
			}
			if result.ExitCode() != wantCode {
				t.Fatalf("exit=%d want=%d outcome=%s", result.ExitCode(), wantCode, result.Outcome())
			}
		})
	}
}

package aitable

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageFormFieldShortcutClearAndTerminalResult(t *testing.T) {
	for _, state := range []string{"verified", "pending", "unknown"} {
		t.Run(state, func(t *testing.T) {
			data := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f", "verificationStatus": state, "completedOperations": []any{"DISPLAY_FILTERS"},
				"actual": map[string]any{"fieldId": "f", "required": true, "hidden": false, "displayConditionStatus": "available", "displayCondition": nil}}
			raw, _ := json.Marshal(map[string]any{"status": "success", "data": data})
			caller := &platformCoverageCaller{response: string(raw)}
			helpers.InitDepsForTest(t, caller)
			root := newPlatformCoverageRoot()
			stdout := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"aitable", "+form-field-update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--clear-display-condition", "--yes", "--format=json"})
			cmd, err := corecmd.ExecuteCForTest(root)
			if err != nil {
				t.Fatal(err)
			}
			code, emitted, err := output.EmitStoredResult(cmd)
			want := 7
			if state == "verified" {
				want = 0
			}
			if err != nil || !emitted || code != want || caller.callCount != 1 || caller.args["clearDisplayCondition"] != true {
				t.Fatalf("code=%d err=%v calls=%d params=%#v output=%s", code, err, caller.callCount, caller.args, stdout)
			}
			if _, exists := caller.args["required"]; exists {
				t.Fatalf("unexpected required mutation: %#v", caller.args)
			}
		})
	}
}

func TestCrossPlatformCoverageFormFieldShortcutOptionalUpdates(t *testing.T) {
	condition := `{"operator":"and","operands":[{"operator":"eq","operands":["source","yes"]}]}`
	for _, flag := range []string{"--required=false", "--field-description=description", "--display-condition=" + condition} {
		t.Run(flag, func(t *testing.T) {
			caller := &platformCoverageCaller{}
			helpers.InitDepsForTest(t, caller)
			root := newPlatformCoverageRoot()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"aitable", "+form-field-update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", flag, "--dry-run", "--yes", "--format=json"})
			cmd, err := corecmd.ExecuteCForTest(root)
			if err != nil {
				t.Fatal(err)
			}
			code, emitted, err := output.EmitStoredResult(cmd)
			if err != nil || !emitted || code != 0 || caller.callCount != 0 {
				t.Fatalf("code=%d emitted=%v calls=%d err=%v", code, emitted, caller.callCount, err)
			}
		})
	}
}

func TestCrossPlatformCoverageFormFieldShortcutEmptyDescriptionOmitted(t *testing.T) {
	for _, preview := range []bool{false, true} {
		t.Run(map[bool]string{false: "execute", true: "preview"}[preview], func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"status": "success", "data": map[string]any{
				"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f", "verificationStatus": "verified",
				"actual": map[string]any{"fieldId": "f", "required": false, "hidden": false},
			}})
			if err != nil {
				t.Fatal(err)
			}
			caller := &platformCoverageCaller{response: string(raw)}
			helpers.InitDepsForTest(t, caller)
			root := newPlatformCoverageRoot()
			stdout := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(&bytes.Buffer{})
			args := []string{"aitable", "+form-field-update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--field-description=", "--required=false", "--yes", "--format=json"}
			if preview {
				args = append(args, "--dry-run")
			}
			root.SetArgs(args)
			cmd, err := corecmd.ExecuteCForTest(root)
			if err != nil {
				t.Fatal(err)
			}
			code, emitted, err := output.EmitStoredResult(cmd)
			if err != nil || !emitted || code != 0 {
				t.Fatalf("code=%d emitted=%v err=%v output=%s", code, emitted, err, stdout)
			}
			want := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "fieldId": "f", "required": false}
			if preview {
				var payload struct {
					Data struct{ Arguments map[string]any }
				}
				if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if caller.callCount != 0 || !reflect.DeepEqual(payload.Data.Arguments, want) {
					t.Fatalf("preview called MCP or changed request: calls=%d data=%#v", caller.callCount, payload.Data.Arguments)
				}
			} else if caller.callCount != 1 || !reflect.DeepEqual(caller.args, want) {
				t.Fatalf("empty description changed request: calls=%d args=%#v", caller.callCount, caller.args)
			}
		})
	}
}

func TestCrossPlatformCoverageFormFieldShortcutDryRunNeverCallsMCP(t *testing.T) {
	caller := &platformCoverageCaller{}
	helpers.InitDepsForTest(t, caller)
	root := newPlatformCoverageRoot()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"aitable", "+form-field-update", "--base-id=b", "--table-id=t", "--view-id=v", "--field-id=f", "--clear-display-condition", "--dry-run", "--yes", "--format=json"})
	cmd, err := corecmd.ExecuteCForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	code, emitted, err := output.EmitStoredResult(cmd)
	if err != nil || !emitted || code != 0 || caller.callCount != 0 {
		t.Fatalf("code=%d emitted=%v calls=%d err=%v", code, emitted, caller.callCount, err)
	}
}

func TestCrossPlatformCoverageFormFieldShortcutDirectExecutionRejectsNoUpdate(t *testing.T) {
	caller := &upsertByKeyCaller{}
	rt := boundaryRuntime(t, FormFieldUpdate, map[string]string{
		"base-id": "b", "table-id": "t", "view-id": "v", "field-id": "f",
	}, nil, caller)
	if err := FormFieldUpdate.Execute(rt); err == nil || len(caller.calls) != 0 {
		t.Fatalf("direct execution without an update: err=%v calls=%d", err, len(caller.calls))
	}
}

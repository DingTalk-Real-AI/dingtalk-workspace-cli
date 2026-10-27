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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func runAitableSFCommand(t *testing.T, caller *aitableTestCaller, args ...string) (map[string]any, error) {
	t.Helper()
	buf := installAitableDeps(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	leaf, err := corecmd.ExecuteContextCForTest(root, context.Background())
	if err != nil {
		return nil, err
	}
	if _, _, err := output.EmitStoredResult(leaf); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v / %s", err, buf.String())
	}
	return envelope, nil
}

func TestCrossPlatformCoverageAitableSFDispatchAndDryRun(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		tool   string
		params map[string]any
	}{
		{[]string{"field", "ai-plugins", "--base-id", "B", "--table-id", "T"}, "list_builtin_ai_field_plugins", map[string]any{"baseId": "B", "tableId": "T"}},
		{[]string{"field-group", "list", "--base-id", "B", "--table-id", "T"}, "list_field_groups", map[string]any{"baseId": "B", "tableId": "T"}},
		{[]string{"field-group", "create", "--base-id", "B", "--table-id", "T", "--name", "地址", "--field-ids", " F1,F2 "}, "create_field_group", map[string]any{"baseId": "B", "tableId": "T", "name": "地址", "fieldIds": []string{"F1", "F2"}}},
		{[]string{"field-group", "create", "--base-id", "B", "--table-id", "T", "--name", "新组", "--field-ids", "F1", "--confirm-empty-group=true"}, "create_field_group", map[string]any{"baseId": "B", "tableId": "T", "name": "新组", "fieldIds": []string{"F1"}, "confirmEmptyGroup": true}},
		{[]string{"field-group", "update", "--base-id", "B", "--table-id", "T", "--group-id", "G", "--description", ""}, "update_field_group", map[string]any{"baseId": "B", "tableId": "T", "groupId": "G", "description": ""}},
		{[]string{"field-group", "update", "--base-id", "B", "--table-id", "T", "--group-id", "G", "--name", "收件地址"}, "update_field_group", map[string]any{"baseId": "B", "tableId": "T", "groupId": "G", "name": "收件地址"}},
		{[]string{"field-group", "ungroup", "--base-id", "B", "--table-id", "T", "--group-id", "G"}, "ungroup_field_group", map[string]any{"baseId": "B", "tableId": "T", "groupId": "G"}},
		{[]string{"field-group", "move-fields", "--base-id", "B", "--table-id", "T", "--field-ids", "F1", "--target-group-id", "G", "--confirm-empty-group=true"}, "move_field_group_fields", map[string]any{"baseId": "B", "tableId": "T", "fieldIds": []string{"F1"}, "targetGroupId": "G", "confirmEmptyGroup": true}},
		{[]string{"field-group", "move-fields", "--base-id", "B", "--table-id", "T", "--field-ids", "F1", "--confirm-empty-group=false"}, "move_field_group_fields", map[string]any{"baseId": "B", "tableId": "T", "fieldIds": []string{"F1"}, "confirmEmptyGroup": false}},
	} {
		t.Run(strings.Join(tc.args[:2], " ")+tc.tool, func(t *testing.T) {
			for _, mode := range []string{"actual", "dry-run", "failure", "confirmation"} {
				t.Run(mode, func(t *testing.T) {
					response := `{"success":true,"data":{"baseId":"B","tableId":"T","groups":[],"verificationStatus":"verified"}}`
					caller := &aitableTestCaller{responses: []string{response}, dryRun: mode == "dry-run"}
					args := append([]string(nil), tc.args...)
					if mode == "failure" {
						caller.errors = []error{errors.New("MCP offline")}
					}
					if mode == "dry-run" {
						args = append(args, "--dry-run")
					} else if mode != "confirmation" {
						args = append(args, "--yes")
					}
					envelope, err := runAitableSFCommand(t, caller, args...)
					read := tc.tool == "list_field_groups" || tc.tool == "list_builtin_ai_field_plugins"
					if mode == "confirmation" && !read {
						if !apperrors.IsConfirmationRequired(err) || len(caller.calls) != 0 {
							t.Fatalf("confirmation: %v / %#v", err, caller.calls)
						}
						return
					}
					if mode == "failure" {
						if err == nil || !strings.Contains(err.Error(), "MCP offline") || len(caller.calls) != 1 {
							t.Fatalf("error=%v calls=%#v", err, caller.calls)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if mode == "dry-run" {
						if len(caller.calls) != 0 || envelope["dry_run"] != true {
							t.Fatalf("preview=%#v calls=%#v", envelope, caller.calls)
						}
						return
					}
					if len(caller.calls) != 1 || caller.calls[0].server != "aitable" || caller.calls[0].tool != tc.tool || !reflect.DeepEqual(caller.calls[0].args, tc.params) {
						t.Fatalf("calls=%#v want %s %#v", caller.calls, tc.tool, tc.params)
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageAitableSFValidation(t *testing.T) {
	for _, args := range [][]string{
		{"field-group", "list", "--base-id", "B"},
		{"field-group", "create", "--base-id", "B", "--table-id", "T", "--name", "组", "--field-ids", "F1,,F2"},
		{"field-group", "create", "--base-id", "B", "--table-id", "T", "--name", "组", "--field-ids", strings.Repeat("F,", 100) + "F"},
		{"field-group", "update", "--base-id", "B", "--table-id", "T", "--group-id", "G"},
		{"field-group", "update", "--table-id", "T", "--group-id", "G", "--name", "组"},
	} {
		caller := &aitableTestCaller{}
		_, err := runAitableSFCommand(t, caller, append(args, "--yes")...)
		var typed *apperrors.Error
		if err == nil || !errors.As(err, &typed) || typed.ExitCode() != 3 || len(caller.calls) != 0 {
			t.Fatalf("args=%#v err=%v calls=%#v", args, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableSFGroupInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"create-duplicate", []string{"create", "--name", "组", "--field-ids", "F1,F1"}, "重复字段 ID"},
		{"create-trimmed-duplicate", []string{"create", "--name", "组", "--field-ids", " F1,F2, F1 "}, "重复字段 ID"},
		{"move-duplicate", []string{"move-fields", "--field-ids", "F1,F1"}, "重复字段 ID"},
		{"move-trimmed-duplicate", []string{"move-fields", "--field-ids", " F1,F2, F1 "}, "重复字段 ID"},
		{"update-empty-name", []string{"update", "--group-id", "G", "--name", ""}, "--name"},
		{"update-whitespace-name", []string{"update", "--group-id", "G", "--name", " \t\n\u3000"}, "--name"},
		{"update-empty-name-with-description", []string{"update", "--group-id", "G", "--name", "", "--description", "说明"}, "--name"},
	} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry-run=%t", tc.name, dryRun), func(t *testing.T) {
				caller := &aitableTestCaller{dryRun: dryRun}
				args := append([]string{"field-group"}, tc.args...)
				args = append(args, "--base-id", "B", "--table-id", "T")
				if dryRun {
					args = append(args, "--dry-run")
				} else {
					args = append(args, "--yes")
				}
				_, err := runAitableSFCommand(t, caller, args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 || !strings.Contains(typed.Message, tc.want) || len(caller.calls) != 0 {
					t.Fatalf("invalid input must fail validation before dispatch: err=%v calls=%#v", err, caller.calls)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableSFMoveTargetPresence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      []string
		wantTarget string
		wantError  bool
	}{
		{name: "omitted"},
		{name: "empty", flags: []string{"--target-group-id", ""}, wantError: true},
		{name: "whitespace", flags: []string{"--target-group-id", " \t\n\u3000"}, wantError: true},
		{name: "valid", flags: []string{"--target-group-id", "G"}, wantTarget: "G"},
		{name: "trimmed-valid", flags: []string{"--target-group-id", " G "}, wantTarget: "G"},
	} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry-run=%t", tc.name, dryRun), func(t *testing.T) {
				caller := &aitableTestCaller{
					responses: []string{`{"status":"success","data":{"baseId":"B","tableId":"T","groups":[],"verificationStatus":"verified"}}`},
					dryRun:    dryRun,
				}
				args := []string{"field-group", "move-fields", "--base-id", "B", "--table-id", "T", "--field-ids", "F1"}
				args = append(args, tc.flags...)
				if dryRun {
					args = append(args, "--dry-run")
				} else {
					args = append(args, "--yes")
				}
				envelope, err := runAitableSFCommand(t, caller, args...)
				if tc.wantError {
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 || !strings.Contains(typed.Message, "--target-group-id") || len(caller.calls) != 0 {
						t.Fatalf("empty target must fail validation before dispatch: err=%v calls=%#v", err, caller.calls)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var sent map[string]any
				if dryRun {
					if len(caller.calls) != 0 || envelope["dry_run"] != true {
						t.Fatalf("dry-run called remote: %#v / %#v", envelope, caller.calls)
					}
					sent = envelope["data"].(map[string]any)["arguments"].(map[string]any)
				} else {
					if len(caller.calls) != 1 || caller.calls[0].tool != "move_field_group_fields" {
						t.Fatalf("expected one move request: %#v", caller.calls)
					}
					sent = caller.calls[0].args
				}
				target, provided := sent["targetGroupId"]
				if tc.wantTarget == "" {
					if provided {
						t.Fatalf("omitted target must stay omitted: %#v", sent)
					}
				} else if !provided || target != tc.wantTarget {
					t.Fatalf("valid target changed: %#v, want %q", sent, tc.wantTarget)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableSFPending(t *testing.T) {
	caller := &aitableTestCaller{responses: []string{`{"success":true,"data":{"baseId":"B","tableId":"T","groups":[],"verificationStatus":"pending"}}`}}
	envelope, err := runAitableSFCommand(t, caller, "field-group", "ungroup", "--base-id", "B", "--table-id", "T", "--group-id", "G", "--yes")
	if err != nil || envelope["outcome"] != "pending" || len(caller.calls) != 1 {
		t.Fatalf("pending=%#v err=%v calls=%#v", envelope, err, caller.calls)
	}
	operation := envelope["meta"].(map[string]any)["operation"].(map[string]any)
	if !strings.Contains(operation["next_command"].(string), "--format json") {
		t.Fatal(operation)
	}
}

func mustSFObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCrossPlatformCoverageAitableSFIdentityAndResultContracts(t *testing.T) {
	root := newAitableCommand()
	for path, tool := range map[string]string{"aitable field ai-plugins": "list_builtin_ai_field_plugins", "aitable field-group list": "list_field_groups", "aitable field-group create": "create_field_group", "aitable field-group update": "update_field_group", "aitable field-group ungroup": "ungroup_field_group", "aitable field-group move-fields": "move_field_group_fields"} {
		leaf := findCLIPath(root, path)
		final, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || final.Identity.CLIPath != path || final.Interface.Ref.RPCName != tool || final.Interface.Ref.ProductID != "aitable" || final.Result == nil || final.DryRun == nil || !output.UsesUnifiedResult(leaf) {
			t.Fatalf("%s contract=%#v", path, final)
		}
	}
}

func TestCrossPlatformCoverageAitableSFBuiltinAIPassThrough(t *testing.T) {
	const config = `{"type":"builtin","pluginId":"KNOWN_PLUGIN","formData":{"sourceField":"F"},"referFieldIds":["F"],"autoRecompute":false}`
	for _, args := range [][]string{
		{"field", "create", "--base-id", "B", "--table-id", "T", "--name", "AI", "--type", "object", "--ai-config", config},
		{"field", "update", "--base-id", "B", "--table-id", "T", "--field-id", "A", "--ai-config", config},
	} {
		caller := &aitableTestCaller{}
		if err := runAitableCoverageCommand(t, caller, args...); err != nil {
			t.Fatal(err)
		}
		if len(caller.calls) != 1 {
			t.Fatal(caller.calls)
		}
		actual := caller.calls[0].args["aiConfig"]
		if caller.calls[0].tool == "create_fields" {
			actual = caller.calls[0].args["fields"].([]any)[0].(map[string]any)["aiConfig"]
		}
		if !reflect.DeepEqual(actual, mustSFObject(t, config)) {
			t.Fatalf("config changed: %#v", actual)
		}
	}
}

func TestCrossPlatformCoverageAitableSFMinimalPendingPreservesAbsentCollections(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		data       string
		collection string
	}{
		{[]string{"field-group", "ungroup", "--base-id", "B", "--table-id", "T", "--group-id", "G"}, `{"baseId":"B","tableId":"T","groupId":"G","verificationStatus":"pending","warningMessage":"readback unavailable"}`, "groups"},
	} {
		t.Run(strings.Join(tc.args[:2], " "), func(t *testing.T) {
			caller := &aitableTestCaller{responses: []string{`{"success":true,"data":` + tc.data + `}`}}
			envelope, err := runAitableSFCommand(t, caller, append(tc.args, "--yes")...)
			if err != nil || envelope["outcome"] != "pending" || len(caller.calls) != 1 {
				t.Fatalf("envelope=%#v err=%v calls=%#v", envelope, err, caller.calls)
			}
			data := envelope["data"].(map[string]any)
			if _, exists := data[tc.collection]; exists {
				t.Fatalf("fabricated %s in pending receipt: %#v", tc.collection, data)
			}
			if !reflect.DeepEqual(data, mustSFObject(t, tc.data)) {
				t.Fatalf("pending receipt changed: %#v", data)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableSFRejectsMalformedWriteReceipts(t *testing.T) {
	for _, path := range [][]string{
		{"create", "--name", "组", "--field-ids", "F1"},
		{"update", "--group-id", "G", "--description", ""},
		{"ungroup", "--group-id", "G"}, {"move-fields", "--field-ids", "F1"},
	} {
		for _, data := range []string{`[]`, `"invalid"`, `42`, `true`, `{}`, `{"verificationStatus":"unknown"}`, `{"verificationStatus":42}`, `{"verificationStatus":null}`} {
			t.Run(path[0]+"/"+data, func(t *testing.T) {
				caller := &aitableTestCaller{responses: []string{`{"status":"success","data":` + data + `}`}}
				args := append([]string{"field-group"}, path...)
				args = append(args, "--base-id", "B", "--table-id", "T", "--yes")
				envelope, err := runAitableSFCommand(t, caller, args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.Category != apperrors.CategoryAPI || typed.Reason != "invalid_field_group_receipt" || typed.ExitCode() != 1 || typed.Retryable || typed.ExecutionStarted != nil || typed.Details != nil || len(caller.calls) != 1 || envelope != nil {
					t.Fatalf("malformed receipt became accepted: envelope=%#v err=%#v calls=%#v", envelope, err, caller.calls)
				}
			})
		}
	}
}

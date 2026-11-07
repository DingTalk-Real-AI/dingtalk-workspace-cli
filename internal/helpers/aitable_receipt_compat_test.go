// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAitableReceiptCompatibility(t *testing.T) {
	for _, tc := range []struct {
		tool, response string
		wantErr        bool
	}{
		{"create_records", `{"status":"success","data":{"newRecordIds":["r"],"clientToken":"token"}}`, false},
		{"create_records", `{"newRecordIds":["r"],"createdRecordIds":["already-present"]}`, false},
		{"create_records", `{}`, false},
		{"list_bases", `{"data":{"bases":[],"nextCursor":"next"}}`, false},
		{"list_bases", `{"bases":[{"baseId":"b"}],"nextCursor":""}`, false},
		{"list_bases", `{"bases":[],"nextCursor":null}`, false},
		{"list_bases", `{"bases":[]}`, false},
		{"list_bases", `{"bases":[],"nextCursor":1}`, true},
		{"list_bases", `{}`, false},
	} {
		t.Run(tc.response, func(t *testing.T) {
			caller := &aitableTestCaller{responses: []string{tc.response}}
			out := installAitableDeps(t, caller)
			err := callAitableCompatibleReceipt(context.Background(), tc.tool, nil)
			if (err != nil) != tc.wantErr || len(caller.calls) != 1 {
				t.Fatalf("%v / %#v", err, caller.calls)
			}
			if tc.wantErr {
				return
			}
			var receipt any
			if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}
			data := aitableReceiptData(receipt)
			if ids, exists := data["newRecordIds"]; exists && !strings.Contains(tc.response, "already-present") && !reflect.DeepEqual(ids, data["createdRecordIds"]) {
				t.Fatalf("alias lost IDs: %#v", data)
			}
			if strings.Contains(tc.response, "already-present") && !reflect.DeepEqual(data["createdRecordIds"], []any{"already-present"}) {
				t.Fatalf("existing key overwritten: %#v", data)
			}
			if bases, ok := data["bases"].([]any); ok {
				if data["returnedCount"] != float64(len(bases)) || data["hasMore"] != (data["nextCursor"] == "next") {
					t.Fatalf("pagination: %#v", data)
				}
			}
		})
	}
	caller := &aitableTestCaller{dryRun: true}
	installAitableDeps(t, caller)
	if err := callAitableCompatibleReceipt(context.Background(), "create_records", nil); err != nil || len(caller.calls) != 0 {
		t.Fatalf("dry run: %v / %#v", err, caller.calls)
	}
	caller = &aitableTestCaller{errors: []error{errors.New("disconnected")}}
	installAitableDeps(t, caller)
	if err := callAitableCompatibleReceipt(context.Background(), "create_records", nil); err == nil || len(caller.calls) != 1 {
		t.Fatalf("uncertain write retried: %v / %#v", err, caller.calls)
	}
}

func TestCrossPlatformCoverageAitableReceiptReadRetryOnly(t *testing.T) {
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		ready <- time.Time{}
		return ready
	})
	caller := &aitableTestCaller{
		errors:    []error{errors.New("connection reset"), nil},
		responses: []string{"", `{"data":{"bases":[],"nextCursor":"next"}}`},
	}
	out := installAitableDeps(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	root.SetArgs([]string{"base", "list"})
	if err := corecmd.ExecuteForTest(root); err != nil || len(caller.calls) != 2 || !strings.Contains(out.String(), `"hasMore": true`) {
		t.Fatalf("base list lost retry/projection: %v / %#v / %s", err, caller.calls, out)
	}
	for _, tool := range []string{"list_bases", "create_records"} {
		caller := &aitableTestCaller{errors: []error{errors.New("connection reset"), errors.New("connection reset"), errors.New("connection reset"), errors.New("connection reset")}}
		installAitableDeps(t, caller)
		err := callAitableCompatibleReceipt(context.Background(), tool, nil)
		wantCalls := 1
		if tool == "list_bases" {
			wantCalls = aitableMaxRetries + 1
		}
		if err == nil || len(caller.calls) != wantCalls {
			t.Fatalf("%s retry boundary: %v / %#v", tool, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableViewTypeAndFilterWiring(t *testing.T) {
	for _, value := range []string{"Grid", "Kanban", "Gantt", "Calendar", "Gallery", "FormDesigner"} {
		if err := validateAitableViewType(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ value, hint string }{{"grid", "did you mean"}, {"bad", "one of"}} {
		caller := &aitableTestCaller{}
		err := runAitableCoverageCommand(t, caller, "view", "create", "--base-id=b", "--table-id=t", "--view-type="+tc.value)
		if err == nil || !strings.Contains(err.Error(), tc.hint) || len(caller.calls) != 0 {
			t.Fatalf("invalid view: %v / %#v", err, caller.calls)
		}
	}
	caller := &aitableTestCaller{responses: []string{`{"records":[]}`}}
	err := runAitableCoverageCommand(t, caller, "record", "query", "--base-id=b", "--table-id=t", `--filters={"operator":"and","operands":[{"fieldId":"f","operator":"gt","value":5}]}`)
	if err != nil || len(caller.calls) != 1 {
		t.Fatalf("filter query: %v / %#v", err, caller.calls)
	}
	filters := caller.calls[0].args["filters"].(map[string]any)
	want := map[string]any{"operator": "and", "operands": []any{map[string]any{"operator": "gt", "operands": []any{"f", float64(5)}}}}
	if !reflect.DeepEqual(filters, want) {
		t.Fatalf("filter normalization: %#v", filters)
	}
}

// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageDatasourceUpdateCompatibleSourceConfig(t *testing.T) {
	raw := " \n" + `{"processCode":"P","unknown":{"id":9007199254740993}}` + " \n"
	response, err := json.Marshal(map[string]any{"status": "success", "data": map[string]any{"datasourceType": "OA", "sourceConfig": raw}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		flags    []string
		changes  map[string]any
		explicit bool
	}{
		{name: "enable auto", flags: []string{"--auto"}, changes: map[string]any{"auto": true}},
		{name: "disable auto", flags: []string{"--auto=false"}, changes: map[string]any{"auto": false}},
		{name: "frequency only", flags: []string{"--auto-sync-setting", `{"syncType":"hourly","hourlyInterval":2}`}, changes: map[string]any{"autoSyncSetting": `{"syncType":"hourly","hourlyInterval":2}`}},
		{name: "auto and frequency", flags: []string{"--auto", "--auto-sync-setting", `{"syncType":"hourly","hourlyInterval":2}`}, changes: map[string]any{"auto": true, "autoSyncSetting": `{"syncType":"hourly","hourlyInterval":2}`}},
		{name: "explicit replacement", flags: []string{"--source-config", `{"replacement":9007199254740993}`}, changes: map[string]any{"sourceConfig": `{"replacement":9007199254740993}`}, explicit: true},
		{name: "explicit empty object", flags: []string{"--source-config", `{}`, "--auto=false"}, changes: map[string]any{"sourceConfig": `{}`, "auto": false}, explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type contextKey struct{}
			ctx := context.WithValue(context.Background(), contextKey{}, "selected-profile-context")
			caller := &aitableDatasourceCaller{respond: func(gotCtx context.Context, tool string) (string, error) {
				if gotCtx.Value(contextKey{}) != "selected-profile-context" {
					t.Fatal("lost command context")
				}
				if tool == "get_datasource_config" {
					return string(response), nil
				}
				return `{"status":"success","data":{"taskId":"task"}}`, nil
			}}
			argv := append([]string{"update", "--base-id", "B", "--table-id", "T"}, tc.flags...)
			if err := runAitableDatasourceCommandWithCaller(t, ctx, caller, argv...); err != nil {
				t.Fatal(err)
			}
			wantTools := []string{"get_datasource_config", "update_datasource_config"}
			if tc.explicit {
				wantTools = wantTools[1:]
			}
			if len(caller.calls) != len(wantTools) {
				t.Fatalf("calls = %v", caller.calls)
			}
			for i, wantTool := range wantTools {
				if caller.calls[i].tool != wantTool || caller.calls[i].server != "aitable" {
					t.Fatalf("wrong target: %v", caller.calls)
				}
				want := map[string]any{"baseId": "B", "tableId": "T"}
				if wantTool == "update_datasource_config" {
					want["sourceConfig"] = raw
					for k, v := range tc.changes {
						want[k] = v
					}
				}
				if !reflect.DeepEqual(caller.calls[i].args, want) {
					t.Fatalf("%s args = %#v, want %#v", wantTool, caller.calls[i].args, want)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDatasourceUpdateReadFailureNeverWrites(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		readErr        error
	}{
		{name: "transport", readErr: errors.New("permission denied")},
		{name: "empty", response: ""},
		{name: "not JSON", response: "invalid"},
		{name: "business error", response: `{"status":"error","error":{"code":"DENIED","message":"denied"}}`},
		{name: "incomplete", response: `{"status":"success","data":{}}`},
		{name: "missing success", response: `{"data":{"datasourceType":"OA","sourceConfig":"{\"processCode\":\"P\"}"}}`},
		{name: "wrong config type", response: `{"status":"success","data":{"datasourceType":"OA","sourceConfig":{}}}`},
		{name: "empty config", response: `{"status":"success","data":{"datasourceType":"OA","sourceConfig":"{}"}}`},
		{name: "invalid config", response: `{"status":"success","data":{"datasourceType":"OA","sourceConfig":"null"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &aitableDatasourceCaller{respond: func(context.Context, string) (string, error) { return tc.response, tc.readErr }}
			err := runAitableDatasourceCommandWithCaller(t, context.Background(), caller, "update", "--base-id", "B", "--table-id", "T", "--auto")
			if err == nil || len(caller.calls) != 1 {
				t.Fatalf("err=%v calls=%v", err, caller.calls)
			}
			i := 0
			if caller.calls[i].tool != "get_datasource_config" {
				t.Fatalf("unexpected write: %v", caller.calls)
			}
		})
	}
}

func TestCrossPlatformCoverageDatasourceUpdateValidatesBeforeRead(t *testing.T) {
	for _, flags := range [][]string{
		{}, {"--source-config", ""}, {"--source-config", "null"},
		{"--auto", "--auto-sync-setting", "not-json"}, {"--auto", "--auto-sync-setting", ""},
		{"--auto", "--field-ids", "f"},
	} {
		caller := &aitableDatasourceCaller{}
		argv := append([]string{"update", "--base-id", "B", "--table-id", "T"}, flags...)
		if err := runAitableDatasourceCommandWithCaller(t, context.Background(), caller, argv...); err == nil || len(caller.calls) != 0 {
			t.Fatalf("flags=%v err=%v calls=%v", flags, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageDatasourceUpdateCancellationAndWriteFailure(t *testing.T) {
	for _, cancelAfterRead := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		caller := &aitableDatasourceCaller{respond: func(_ context.Context, stringTool string) (string, error) {
			if stringTool == "get_datasource_config" {
				if cancelAfterRead {
					cancel()
				}
				return `{"status":"success","data":{"datasourceType":"OA","sourceConfig":"{\"processCode\":\"P\"}"}}`, nil
			}
			return "", context.DeadlineExceeded
		}}
		err := runAitableDatasourceCommandWithCaller(t, ctx, caller, "update", "--base-id", "B", "--table-id", "T", "--auto")
		wantCalls := 2
		if cancelAfterRead {
			wantCalls = 1
		}
		if err == nil || len(caller.calls) != wantCalls {
			t.Fatalf("cancel=%v err=%v calls=%v", cancelAfterRead, err, caller.calls)
		}
		// A write timeout must never cause a second write.
	}
}

func TestCrossPlatformCoverageDatasourceUpdateDryRunNeverWrites(t *testing.T) {
	caller := &aitableDatasourceCaller{dryRun: true, respond: func(context.Context, string) (string, error) {
		return `{"status":"success","data":{"datasourceType":"OA","sourceConfig":"{\"processCode\":\"P\"}"}}`, nil
	}}
	argv := []string{"update", "--base-id", "B", "--table-id", "T", "--auto"}

	_ = runAitableDatasourceCommandWithCaller(t, context.Background(), caller, argv...)
	for i := range caller.calls {
		if caller.calls[i].tool != "get_datasource_config" {
			t.Fatalf("dry run wrote: %v", caller.calls)
		}
	}
}

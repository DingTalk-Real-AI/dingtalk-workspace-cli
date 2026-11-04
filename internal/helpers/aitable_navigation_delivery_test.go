// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func executeNavigationLeaf(t *testing.T, caller *aitableTestCaller, args ...string) (map[string]any, int, error) {
	t.Helper()
	InitDepsForTest(t, caller)
	root := newAitableCommand()
	installExampleGlobalFlags(root)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetArgs(args)
	ctx, _ := output.WithResultStore(context.Background())
	root.SetContext(ctx)
	leaf, err := corecmd.ExecuteCForTest(root)
	if err != nil {
		return nil, 0, err
	}
	code, emitted, err := output.EmitStoredResult(leaf)
	if err != nil || !emitted {
		t.Fatalf("missing unified emission: %t %v", emitted, err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err, stdout.String())
	}
	if result["outcome"] == "success" || result["outcome"] == "pending" {
		declaration, ok := contractfinal.RuntimeContractFinal(leaf)
		if !ok || declaration.Result == nil {
			t.Fatal("missing declared Result")
		}
		var schema any
		if err := json.Unmarshal(declaration.Result.DataSchema, &schema); err != nil {
			t.Fatal(err)
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("https://dws.test/result.json", schema); err != nil {
			t.Fatal(err)
		}
		compiled, err := compiler.Compile("https://dws.test/result.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := compiled.Validate(result["data"]); err != nil {
			t.Fatal(err, result)
		}
	}
	return result, code, nil
}

func TestCrossPlatformCoverageAitableNavigationUnifiedDelivery(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--name", "Name"},
		{"base", "create", "--name", "Name"}, {"base", "update", "--base-id", "b", "--name", "Name"},
		{"base", "copy", "--base-id", "source"},
		{"app", "widget", "create", "--base-id", "b", "--page-id", "p", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":48,"h":8}`},
		{"app", "widget", "update", "--base-id", "b", "--page-id", "p", "--widget-id", "w", "--name", "Name"},
		{"import", "data", "--import-id", "imp"},
	} {
		for _, nav := range []map[string]any{nil, {"url": nil, "links": nil}, {"url": "", "links": []any{}},
			{"url": "https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3Dapps%26appId%3Da%26pageId%3Dp", "links": []any{map[string]any{"objectType": "base", "url": "https://alidocs.dingtalk.com/i/nodes/b"}}},
		} {
			body := map[string]any{"status": "success", "baseId": "b", "appId": "a", "untouched": map[string]any{"value": 42.0}}
			if args[0] == "app" {
				body["config"] = map[string]any{"chartType": "AI_ANALYZE"}
				body["layout"] = map[string]any{"x": 0.0, "y": 0.0, "w": 48.0, "h": 8.0}
				body["pageId"], body["widgetId"], body["widgetName"], body["widgetType"] = "p", "w", "Name", "AI_ANALYZE"
			}
			for key, value := range nav {
				body[key] = value
			}
			encoded, _ := json.Marshal(map[string]any{"status": "success", "data": body})
			caller := &aitableTestCaller{responses: []string{string(encoded)}}
			result, code, err := executeNavigationLeaf(t, caller, append(append([]string{}, args...), "--yes", "--format=json")...)
			if err != nil || code != 0 || result["ok"] != true || result["outcome"] != "success" || !reflect.DeepEqual(result["data"], body) || len(caller.calls) != 1 {
				t.Fatalf("%v: code=%d result=%#v err=%v calls=%#v", args, code, result, err, caller.calls)
			}
		}
	}
	for _, args := range [][]string{
		{"base", "create", "--name", "Name"}, {"import", "data", "--import-id", "imp"},
		{"app", "widget", "create", "--base-id", "b", "--page-id", "p", "--config", `{"chartType":"AI_ANALYZE"}`, "--layout", `{"x":0,"y":0,"w":48,"h":8}`},
		{"app", "widget", "update", "--base-id", "b", "--page-id", "p", "--widget-id", "w", "--name", "Name"},
	} {
		caller := &aitableTestCaller{}
		result, code, err := executeNavigationLeaf(t, caller, append(args, "--dry-run", "--format=json")...)
		if err != nil || code != 0 || len(caller.calls) != 0 || result["data"].(map[string]any)["executed"] != false {
			t.Fatal(result, code, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableNavigationPendingAndUnknown(t *testing.T) {
	for _, status := range []string{"pending", "unknown", "failed", "all_fail", "timeout", "partial_failure"} {
		caller := &aitableTestCaller{responses: []string{`{"status":"success","data":{"status":"` + status + `","url":"receipt"}}`}}
		result, code, err := executeNavigationLeaf(t, caller, "import", "data", "--import-id", "imp", "--yes", "--format=json")
		if err != nil || len(caller.calls) != 1 {
			t.Fatal(err, caller.calls)
		}
		if status == "pending" {
			if code != 0 || result["outcome"] != "pending" || result["data"].(map[string]any)["url"] != "receipt" {
				t.Fatal(result, code)
			}
			operation := result["meta"].(map[string]any)["operation"].(map[string]any)
			if operation["id"] != "imp" || operation["next_command"] != "dws aitable import data --import-id imp --format json" {
				t.Fatal(operation)
			}
		} else if code == 0 || result["ok"] != false || result["outcome"] != "failure" || result["data"] != nil {
			t.Fatal(result, code)
		}
		if status == "partial_failure" {
			failure := result["error"].(map[string]any)
			if failure["subtype"] != "import_task_partial_failure" || failure["execution_started"] != true || !strings.Contains(failure["message"].(string), "部分失败") || failure["details"].(map[string]any)["result"].(map[string]any)["status"] != status {
				t.Fatal(failure)
			}
		}
	}
	for _, dryRun := range []bool{false, true} {
		caller := &aitableTestCaller{dryRun: dryRun}
		if !dryRun {
			caller.responses = []string{`{"status":"success","data":[]}`}
		}
		result, code, err := executeNavigationLeaf(t, caller, "base", "create", "--name", "Name", "--yes", "--format=json")
		if dryRun {
			if err != nil || code != 0 || len(caller.calls) != 0 || result["data"].(map[string]any)["executed"] != false {
				t.Fatal(result, code, err, caller.calls)
			}
		} else if err == nil {
			t.Fatal("invalid business object accepted")
		}
	}
	for _, args := range [][]string{{"base", "create", "--name", "Name"}, {"import", "data", "--import-id", "imp"}} {
		caller := &aitableTestCaller{errors: []error{errors.New("transport unavailable")}}
		if _, _, err := executeNavigationLeaf(t, caller, append(args, "--yes", "--format=json")...); err == nil || len(caller.calls) != 1 {
			t.Fatal("write error was hidden or replayed", err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageAitableBaseNavigationVerification(t *testing.T) {
	for _, args := range [][]string{
		{"create", "--name", "Name"}, {"base", "create", "--name", "Name"},
		{"base", "update", "--base-id", "target'base", "--name", "Name"},
		{"base", "copy", "--base-id", "source"},
	} {
		for _, tc := range []struct {
			name, outcome string
			status        any
		}{
			{"omitted", "success", nil}, {"verified", "success", "verified"}, {"pending", "pending", "pending"},
			{"unknown", "failure", "unknown"}, {"failed", "failure", "failed"}, {"future", "failure", "future_state"},
			{"empty", "failure", ""}, {"null", "failure", nil}, {"number", "failure", 42},
			{"array", "failure", []any{}}, {"object", "failure", map[string]any{}},
		} {
			t.Run(strings.Join(args, "/")+"/"+tc.name, func(t *testing.T) {
				body := map[string]any{"baseId": "new'base", "url": "receipt"}
				if tc.name != "omitted" {
					body["verificationStatus"] = tc.status
				}
				encoded, _ := json.Marshal(map[string]any{"status": "success", "data": body})
				caller := &aitableTestCaller{responses: []string{string(encoded)}}
				result, code, err := executeNavigationLeaf(t, caller, append(append([]string{}, args...), "--yes", "--format=json")...)
				if err != nil || len(caller.calls) != 1 || result["outcome"] != tc.outcome || (code == 0) != (tc.outcome != "failure") {
					t.Fatal(result, code, err, caller.calls)
				}
				if tc.outcome == "failure" {
					failure := result["error"].(map[string]any)
					if failure["subtype"] != "projection_unknown" || failure["execution_started"] != true || failure["details"].(map[string]any)["result"].(map[string]any)["url"] != "receipt" {
						t.Fatal(failure)
					}
					return
				}
				if !reflect.DeepEqual(result["data"], body) {
					t.Fatal(result)
				}
				if tc.outcome == "pending" {
					id := "new'base"
					if args[1] == "update" {
						id = "target'base"
					}
					operation := result["meta"].(map[string]any)["operation"].(map[string]any)
					if operation["id"] != id || operation["next_command"] != "dws aitable base get --base-id "+ShellQuoteArg(id)+" --format json" {
						t.Fatal(operation)
					}
				}
			})
		}
	}
	for _, action := range []string{"create", "copy"} {
		for _, id := range []any{nil, "", " ", 42} {
			body := map[string]any{"baseId": id, "verificationStatus": "pending", "url": "receipt"}
			encoded, _ := json.Marshal(map[string]any{"status": "success", "data": body})
			caller := &aitableTestCaller{responses: []string{string(encoded)}}
			args := []string{"base", "create", "--name", "Name"}
			if action == "copy" {
				args = []string{"base", "copy", "--base-id", "source"}
			}
			result, code, err := executeNavigationLeaf(t, caller, append(args, "--yes", "--format=json")...)
			if err != nil || code == 0 || result["outcome"] != "failure" || len(caller.calls) != 1 {
				t.Fatal(result, code, err, caller.calls)
			}
		}
	}
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"update_base", nil}, {"update_table", map[string]any{"baseId": "b"}},
		{"update_table", map[string]any{"tableId": "t"}}, {"future_tool", map[string]any{"baseId": "b"}},
	} {
		if operation := aitableNavigationPendingOperation(tc.tool, tc.args, nil); operation != nil {
			t.Fatal(operation)
		}
	}
}

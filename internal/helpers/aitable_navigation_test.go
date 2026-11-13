// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCrossPlatformCoverageAitableNavigationNullableSchemas(t *testing.T) {
	for name, spec := range map[string]*contract.ResultSpec{
		"object": AitableObjectNavigationResultSpec(), "batch": aitableNavigationResultSpec(true),
		"tables": AitableTablesNavigationResultSpec(), "dashboard": aitableDashboardGetResultSpec(), "form": aitableCreateFormResultSpec(),
	} {
		t.Run(name, func(t *testing.T) {
			var schema any
			if err := json.Unmarshal(spec.DataSchema, &schema); err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("https://dws.test/navigation.json", schema); err != nil {
				t.Fatal(err)
			}
			compiled, err := compiler.Compile("https://dws.test/navigation.json")
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				nav   map[string]any
				valid bool
			}{
				{nil, true}, {map[string]any{"url": nil, "links": nil}, true},
				{map[string]any{"url": "", "links": []any{}}, true},
				{map[string]any{"url": "https://example.test/object", "links": []any{map[string]any{"url": "https://example.test/object"}}}, true},
				{map[string]any{"url": true}, false}, {map[string]any{"links": map[string]any{}}, false},
			} {
				object := map[string]any{"baseId": "b", "tableId": "s", "viewId": "v", "meta": nil}
				maps.Copy(object, tc.nav)
				var data any = object
				if name == "tables" {
					data = map[string]any{"tables": []any{object}}
				}
				if name == "batch" {
					data = map[string]any{"results": []any{object}}
				}
				if err := compiled.Validate(map[string]any{"data": data}); (err == nil) != tc.valid {
					t.Fatalf("navigation %#v: %v", tc.nav, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAitableGetNullableNavigationRuntime(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		for _, nav := range []map[string]any{nil, {"url": nil, "links": nil}, {"url": "", "links": []any{}}} {
			object := map[string]any{"tableId": "s", "dashboardId": "d", "meta": nil}
			maps.Copy(object, nav)
			data, args, tool := any(object), []string{"dashboard", "get", "--base-id=b", "--dashboard-id=d", "--format=json"}, "get_dashboard"
			if !dashboard {
				data, args, tool = map[string]any{"tables": []any{object}}, []string{"table", "get", "--base-id=b", "--format=json"}, "get_tables"
			}
			raw, err := json.Marshal(map[string]any{"status": "success", "data": data})
			if err != nil {
				t.Fatal(err)
			}
			caller := &aitableTestCaller{responses: []string{string(raw)}}
			InitDepsForTest(t, caller)
			out := &bytes.Buffer{}
			deps.Out.SetWriters(out, out)
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			root.SetArgs(args)
			if err := corecmd.ExecuteContextForTest(root, context.Background()); err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err, out)
			}
			var want map[string]any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, want) || len(caller.calls) != 1 || caller.calls[0].tool != tool {
				t.Fatalf("get navigation changed: %s calls=%#v", out, caller.calls)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableNavigationResultContracts(t *testing.T) {
	for _, batch := range []bool{true, false} {
		spec := aitableNavigationResultSpec(batch)
		if !batch {
			spec = AitableObjectNavigationResultSpec()
		}
		var shape map[string]any
		if err := json.Unmarshal(spec.DataSchema, &shape); err != nil {
			t.Fatal(err)
		}
		object := shape["properties"].(map[string]any)["data"].(map[string]any)
		if batch {
			object = object["properties"].(map[string]any)["results"].(map[string]any)["items"].(map[string]any)
		}
		props := object["properties"].(map[string]any)
		if props["url"] == nil || props["links"] == nil {
			t.Fatal(object)
		}
	}
}

// TestCrossPlatformCoverageAitableTableListNavigationContract checks the list envelope separately from object responses.
func TestCrossPlatformCoverageAitableTableListNavigationContract(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(AitableTablesNavigationResultSpec().DataSchema, &schema); err != nil {
		t.Fatal(err)
	}
	data := schema["properties"].(map[string]any)["data"].(map[string]any)
	table := data["properties"].(map[string]any)["tables"].(map[string]any)["items"].(map[string]any)
	properties := table["properties"].(map[string]any)
	for _, key := range []string{"tableId", "url", "links"} {
		if properties[key] == nil {
			t.Fatalf("missing %s: %#v", key, properties)
		}
	}
}

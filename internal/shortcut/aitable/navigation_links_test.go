// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0
package aitable

import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/aitabletarget"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// navigationViewFixture keeps verified view facts separate from receipt-only navigation.
func navigationViewFixture(viewType string, nav map[string]any) map[string]any {
	view := map[string]any{"viewId": "v", "viewName": "Name", "viewType": viewType, "config": map[string]any{"visibleFieldIds": []any{"f1"}}}
	maps.Copy(view, nav)
	return view
}

// assertViewNavigation checks the final payload rather than an intermediate receipt.
func assertViewNavigation(t *testing.T, final, readback, want map[string]any) {
	t.Helper()
	for _, key := range []string{"url", "links"} {
		got, present := final[key]
		expected, wanted := want[key]
		if present != wanted || !reflect.DeepEqual(got, expected) {
			t.Fatalf("navigation %s: got %#v (present=%v), want %#v (present=%v)", key, got, present, expected, wanted)
		}
	}
	if !reflect.DeepEqual(final["view"], readback) {
		t.Fatalf("receipt changed independently verified view: %#v", final)
	}
}

// TestCrossPlatformCoverageNavigationViewPresetReceiptFallback covers create/update and per-key precedence.
func TestCrossPlatformCoverageNavigationViewPresetReceiptFallback(t *testing.T) {
	write := map[string]any{"url": "https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3Ddata%26sheetId%3Ds%26viewId%3Dv", "links": []any{map[string]any{"objectId": "v", "name": "receipt"}}}
	read := map[string]any{"url": "https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3ddata%26sheetId%3ds%26viewId%3dv", "links": []any{map[string]any{"objectId": "v", "name": "readback"}}}
	cases := []struct {
		name                    string
		writeNav, readNav, want map[string]any
	}{
		{"write only", write, nil, write},
		{"readback wins", write, read, read},
		{"neither", nil, nil, nil},
		{"readback URL only", write, map[string]any{"url": read["url"]}, map[string]any{"url": read["url"], "links": write["links"]}},
		{"readback links only", write, map[string]any{"links": read["links"]}, map[string]any{"url": write["url"], "links": read["links"]}},
	}
	for _, action := range []string{"create", "update"} {
		for _, tc := range cases {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				var before []any
				if action == "update" {
					before = []any{map[string]any{"viewId": "v", "viewName": "Name", "viewType": "Grid", "config": map[string]any{"visibleFieldIds": []any{"old"}}}}
				} else {
					before = []any{}
				}
				receipt := map[string]any{"viewId": "v", "verificationStatus": "receipt_only"}
				maps.Copy(receipt, tc.writeNav)
				readback := navigationViewFixture("Grid", tc.readNav)
				caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
					parityStep(map[string]any{"views": before}),
					parityStep(map[string]any{"status": "success", "data": receipt}),
					parityStep(map[string]any{"views": []any{readback}}),
				}}
				out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply", "--base-id", "b", "--table-id", "s", "--name", "Name", "--view-type", "Grid", "--config", `{"visibleFieldIds":["f1"]}`, "--yes")
				if err != nil {
					t.Fatal(err, out)
				}
				final := navigationFinalResult(t, out)
				assertViewNavigation(t, final, readback, tc.want)
				if _, copied := final["verificationStatus"]; copied || len(caller.calls) != 3 || caller.calls[1].tool != action+"_view" {
					t.Fatalf("unexpected state or writes: result=%#v calls=%#v", final, caller.calls)
				}
			})
		}
	}
}

// TestCrossPlatformCoverageNavigationGanttReceiptFallback covers the separate timebar write as well.
func TestCrossPlatformCoverageNavigationGanttReceiptFallback(t *testing.T) {
	preset := map[string]any{"url": "preset-url", "links": []any{map[string]any{"name": "preset", "objectId": "v"}}}
	timebar := map[string]any{"url": "timebar-url", "links": []any{map[string]any{"name": "timebar", "objectId": "v"}}}
	read := map[string]any{"url": "readback-url", "links": []any{map[string]any{"name": "readback", "objectId": "v"}}}
	cases := []struct {
		name                                 string
		onlyTimebar                          bool
		presetNav, timebarNav, readNav, want map[string]any
	}{
		{"latest write fallback", false, preset, timebar, nil, timebar},
		{"readback wins both writes", false, preset, timebar, read, read},
		{"preset survives old timebar receipt", false, preset, nil, nil, preset},
		{"timebar only", true, nil, timebar, nil, timebar},
		{"no navigation", false, nil, nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps := []upsertByKeyStep{}
			if tc.onlyTimebar {
				before := navigationViewFixture("Gantt", nil)
				before["ganttTimebar"] = map[string]any{"startField": "old"}
				steps = append(steps, parityStep(map[string]any{"views": []any{before}}))
			} else {
				receipt := map[string]any{"viewId": "v"}
				maps.Copy(receipt, tc.presetNav)
				steps = append(steps, parityStep(map[string]any{"views": []any{}}), parityStep(map[string]any{"status": "success", "data": receipt}), parityStep(map[string]any{"views": []any{navigationViewFixture("Gantt", nil)}}))
			}
			readback := navigationViewFixture("Gantt", tc.readNav)
			readback["ganttTimebar"] = map[string]any{"startField": "date1"}
			receipt := map[string]any{"viewId": "v"}
			maps.Copy(receipt, tc.timebarNav)
			steps = append(steps, parityStep(map[string]any{"status": "success", "data": receipt}), parityStep(map[string]any{"views": []any{readback}}))
			caller := &upsertByKeyCaller{steps: steps}
			out, err := runAITableCompositeCLI(t, caller, "+view-preset-apply", "--base-id", "b", "--table-id", "s", "--name", "Name", "--view-type", "Gantt", "--config", `{"visibleFieldIds":["f1"]}`, "--timebar", `{"startField":"date1"}`, "--yes")
			if err != nil {
				t.Fatal(err, out)
			}
			assertViewNavigation(t, navigationFinalResult(t, out), readback, tc.want)
			if len(caller.calls) != len(steps) || !strings.Contains(out, `"timebarVerified": true`) {
				t.Fatal(out, caller.calls)
			}
		})
	}
}

// Inspect the final result, so keeping a URL only in an intermediate step cannot pass.
func navigationFinalResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatal(err, text)
	}
	if data, ok := decoded["data"].(map[string]any); ok {
		decoded = data
	}
	result, ok := decoded["result"].(map[string]any)
	if !ok {
		t.Fatal(decoded)
	}
	return result
}

func TestCrossPlatformCoverageNavigationWorkflowDeployFinalResult(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"valid":true,"flowId":"w1","url":"https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3Dautomation%26flowId%3Dw1%26autoOpenAutomation%3Dtrue","links":[{"objectType":"workflow","objectId":"w1"}]}`},
		{text: `{"flowId":"w1"}`}, {text: `{"list":[{"flowId":"w1","status":"STOP"}]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+workflow-deploy", "--base-id", "b", "--dsl", workflowDSLFixture, "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	final := navigationFinalResult(t, out)
	if !strings.Contains(final["url"].(string), "flowId%3Dw1%26autoOpenAutomation%3Dtrue") || final["links"] == nil {
		t.Fatal(final)
	}
}

func TestCrossPlatformCoverageNavigationAppReadbackRetainsReceipt(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"pageId":"p","url":"page-url","appId":"encoded","links":[{"objectId":"p"}]}`},
		{text: `{"pageId":"p","pageName":"Name"}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+app-page-create", "--base-id", "b", "--name", "Name", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	final := navigationFinalResult(t, out)
	if final["url"] != "page-url" || final["appId"] != "encoded" || final["links"] == nil {
		t.Fatal(final)
	}
}

func TestCrossPlatformCoverageNavigationFormReadbackRetainsReceipt(t *testing.T) {
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"viewId":"v","url":"form-url","links":[{"objectId":"v"}]}`},
		{text: `{"forms":[{"viewId":"v","name":"Name"}]}`},
		{text: `{"fields":[]}`},
	}}
	out, err := runAITableCompositeCLI(t, caller, "+form-create", "--base-id", "b", "--table-id", "s", "--name", "Name", "--yes")
	if err != nil {
		t.Fatal(err, out)
	}
	final := navigationFinalResult(t, out)
	if final["url"] != "form-url" || final["links"] == nil {
		t.Fatal(final)
	}
}

func TestCrossPlatformCoverageNavigationFieldCreationAndResumeReturnCompleteTargets(t *testing.T) {
	readback := map[string]any{"fields": []any{map[string]any{"fieldId": "f", "fieldName": "Title", "type": "text"}}}
	for _, resume := range []bool{false, true} {
		name := "create"
		if resume {
			name = "resume only"
		}
		t.Run(name, func(t *testing.T) {
			steps := []upsertByKeyStep{parityStep(readback)}
			args := []string{"--base-id", "b", "--table-id", "s", "--fields", `[{"fieldName":"Title","type":"text"}]`, "--yes"}
			if resume {
				args = append(args, "--resume-field-ids", "f")
			} else {
				steps = append([]upsertByKeyStep{
					parityStep(map[string]any{"fields": []any{}}),
					parityStep(map[string]any{"results": []any{map[string]any{"fieldId": "f", "fieldName": "Title", "success": true}}}),
				}, steps...)
			}
			caller := &upsertByKeyCaller{steps: steps}
			out, err := runAITableCompositeCLI(t, caller, "+field-create", args...)
			if err != nil {
				t.Fatal(err, out)
			}
			final := navigationFinalResult(t, out)
			field := final["fields"].([]any)[0].(map[string]any)
			target, err := aitabletarget.ParseURL(field["url"].(string))
			if err != nil || target.Kind != "field" || target.BaseID != "b" || target.TableID != "s" || target.FieldID != "f" {
				t.Fatalf("field link cannot resolve to its verified field: %#v, %v", target, err)
			}
			links := final["links"].([]any)
			if links[0].(map[string]any)["url"] != field["url"] {
				t.Fatal(final)
			}
			parent, err := aitabletarget.ParseURL(links[1].(map[string]any)["url"].(string))
			if err != nil || parent.Kind != "table" || parent.BaseID != "b" || parent.TableID != "s" || len(caller.calls) != len(steps) {
				t.Fatal(parent, err, caller.calls)
			}
		})
	}
}

// TestCrossPlatformCoverageNavigationCreatedObjects verifies byte-preserving
// navigation on the real CLI path. Explicit null/empty readback values remain
// authoritative; only missing keys may fall back to the creation receipt. The
// actual business output must also validate against the declared Result schema.
func TestCrossPlatformCoverageNavigationCreatedObjects(t *testing.T) {
	for _, command := range []string{"+table-bootstrap", "+table-create", "+dashboard-create"} {
		resultSpec := TableBootstrap.Contract.Result
		if command == "+dashboard-create" {
			resultSpec = dashboardCreateShortcut(false).Contract.Result
		}
		var resultSchema any
		if err := json.Unmarshal(resultSpec.DataSchema, &resultSchema); err != nil {
			t.Fatal(err)
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		const resource = "https://dws.test/created-object-navigation.json"
		if err := compiler.AddResource(resource, resultSchema); err != nil {
			t.Fatal(err)
		}
		compiled, err := compiler.Compile(resource)
		if err != nil {
			t.Fatal(err)
		}
		// Use each object's actual MCP navigation key; Dashboard links must not
		// accidentally be represented by a table URL in these fixtures.
		targetKey := "sheetId"
		if command == "+dashboard-create" {
			targetKey = "applicationId"
		}
		write := map[string]any{"url": "https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3Ddata%26" + targetKey + "%3Dnew", "links": []any{map[string]any{"objectId": "new", "name": "receipt"}}}
		read := map[string]any{"url": "https://alidocs.dingtalk.com/i/nodes/b?iframeQuery=entrance%3ddata%26" + targetKey + "%3dnew", "links": []any{map[string]any{"objectId": "new", "name": "readback"}}}
		cases := []struct {
			name              string
			write, read, want map[string]any
		}{
			{"receipt fallback", write, nil, write},
			{"readback wins", write, read, read},
			{"readback only", nil, read, read},
			{"old server", nil, nil, nil},
			{"partial readback URL", write, map[string]any{"url": read["url"]}, map[string]any{"url": read["url"], "links": write["links"]}},
			{"partial readback links", write, map[string]any{"links": read["links"]}, map[string]any{"url": write["url"], "links": read["links"]}},
			{"explicit null", write, map[string]any{"url": nil, "links": nil}, map[string]any{"url": nil, "links": nil}},
			{"explicit empty", write, map[string]any{"url": "", "links": []any{}}, map[string]any{"url": "", "links": []any{}}},
		}
		for _, tc := range cases {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				receipt := map[string]any{"tableId": "new"}
				actual := map[string]any{"tableId": "new", "tableName": "Name"}
				args := []string{"--base-id", "b", "--name", "Name", "--yes"}
				steps := []upsertByKeyStep{}
				if command == "+dashboard-create" {
					receipt = map[string]any{"dashboardId": "new"}
					actual = map[string]any{"dashboardId": "new", "dashboardName": "Name"}
				} else {
					args = append(args, "--fields", mustJSON(t, bootstrapFields(1)))
				}
				maps.Copy(receipt, tc.write)
				maps.Copy(actual, tc.read)
				// A navigation receipt is not independent verification evidence.
				receipt["verificationStatus"] = "receipt_only"
				steps = append(steps, parityStep(map[string]any{"status": "success", "data": receipt}))
				if command == "+dashboard-create" {
					steps = append(steps, parityStep(map[string]any{"data": actual}))
				} else {
					steps = append(steps, parityStep(map[string]any{"data": map[string]any{"tables": []any{actual}}}), upsertByKeyStep{text: fieldReadBackJSON(t, bootstrapFields(1))})
				}
				caller := &upsertByKeyCaller{steps: steps}
				out, err := runAITableCompositeCLI(t, caller, command, args...)
				if err != nil {
					t.Fatal(err, out)
				}
				var business map[string]any
				if err := json.Unmarshal([]byte(out), &business); err != nil {
					t.Fatal(err)
				}
				if data, ok := business["data"].(map[string]any); ok {
					business = data
				}
				if err := compiled.Validate(business); err != nil {
					t.Fatalf("actual output contradicts Result schema: %v\n%s", err, out)
				}
				final := navigationFinalResult(t, out)
				if tc.name == "explicit null" {
					// Nullable navigation must still reject unrelated JSON types.
					for key, value := range map[string]any{"url": true, "links": map[string]any{}} {
						invalid, invalidResult := maps.Clone(business), maps.Clone(final)
						invalidResult[key] = value
						invalid["result"] = invalidResult
						if compiled.Validate(invalid) == nil {
							t.Fatalf("Result schema accepted invalid %s: %#v", key, value)
						}
					}
				}
				for _, key := range []string{"url", "links"} {
					got, exists := final[key]
					want, present := tc.want[key]
					if exists != present || !reflect.DeepEqual(got, want) {
						t.Fatalf("%s: got %#v, want %#v", key, final, tc.want)
					}
				}
				if _, exists := final["verificationStatus"]; exists || len(caller.calls) != len(steps) || !strings.Contains(out, `"status": "verified"`) {
					t.Fatal(out, caller.calls)
				}
			})
		}
	}
}

// TestCrossPlatformCoverageNavigationReceiptCannotVerifyCreation protects the
// write/read boundary: a usable receipt URL cannot turn failed independent
// verification into success, nor trigger a second non-idempotent create.
func TestCrossPlatformCoverageNavigationReceiptCannotVerifyCreation(t *testing.T) {
	for _, command := range []string{"+table-bootstrap", "+table-create", "+dashboard-create"} {
		for _, failure := range []string{"read error", "wrong ID", "content mismatch"} {
			t.Run(command+"/"+failure, func(t *testing.T) {
				idKey := "tableId"
				if command == "+dashboard-create" {
					idKey = "dashboardId"
				}
				receipt := map[string]any{idKey: "new", "url": "receipt-url", "links": []any{map[string]any{"objectId": "new"}}, "verificationStatus": "verified"}
				args := []string{"--base-id", "b", "--name", "Name", "--yes"}
				steps := []upsertByKeyStep{parityStep(map[string]any{"status": "success", "data": receipt})}
				actual := map[string]any{idKey: "new"}
				if failure == "wrong ID" {
					actual[idKey] = "other"
				}
				if command == "+dashboard-create" {
					actual["dashboardName"] = "Name"
					if failure == "content mismatch" {
						actual["dashboardName"] = "Wrong name"
					}
					steps = append(steps, parityStep(map[string]any{"data": actual}))
				} else {
					args = append(args, "--fields", mustJSON(t, bootstrapFields(1)))
					steps = append(steps, parityStep(map[string]any{"data": map[string]any{"tables": []any{actual}}}))
					if failure == "content mismatch" {
						steps = append(steps, parityStep(map[string]any{"fields": []any{}}))
					}
				}
				if failure == "read error" {
					steps[1] = upsertByKeyStep{err: errors.New("read unavailable")}
				}
				caller := &upsertByKeyCaller{steps: steps}
				out, err := runAITableCompositeCLI(t, caller, command, args...)
				var typed *apperrors.Error
				if out != "" || !errors.As(err, &typed) || typed.Retryable || len(caller.calls) != len(steps) {
					t.Fatalf("out=%s err=%v calls=%#v", out, err, caller.calls)
				}
				for _, call := range caller.calls[1:] {
					if !strings.HasPrefix(call.tool, "get_") {
						t.Fatalf("verification repeated a write: %#v", caller.calls)
					}
				}
				recovery, ok := typed.Details["result"].(compositeResult)
				if !ok || recovery.CompletedCount != 0 || recovery.Verification != nil || recovery.Result != nil || recovery.Resolved[idKey] != "new" {
					t.Fatalf("receipt promoted to verified success or lost recovery ID: %#v", typed.Details)
				}
			})
		}
	}
}

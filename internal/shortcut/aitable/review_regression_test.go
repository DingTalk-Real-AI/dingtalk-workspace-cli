// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChartBatchRejectsDuplicateCreatedIDs(t *testing.T) {
	charts := `[{"config":{"name":"n","chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}},{"config":{"name":"n","chartType":"LINE"},"layout":{"x":0,"y":0,"w":12,"h":4}}]`
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"baseId":"b","dashboardId":"d","meta":{"schemaVersion":2,"schemaVersionTypeVerified":true}}}`},
		{text: `{"data":{"baseId":"b","dashboardId":"d","totalCreated":2,"totalVerified":0,"results":[{"index":0,"chartId":"c","status":"accepted","success":true},{"index":1,"chartId":"c","status":"accepted","success":true}]}}`},
	}}
	if _, err := runAITableCompositeCLI(t, caller, "+charts-create", "--base-id", "b", "--dashboard-id", "d", "--charts", charts, "--server-batch", "--yes"); err == nil {
		t.Fatal("two inputs verified against the same created object")
	}
	if len(caller.calls) != 2 {
		t.Fatal("duplicate response triggered more calls", caller.calls)
	}
}

func TestCrossPlatformCoverageAIWaitRejectsIncompleteOrContradictoryRowEvidence(t *testing.T) {
	targets, err := parseAITaskTargets(`[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]`)
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"tasks":[{"taskId":"TASK","fieldId":"F","recordIds":["R"],"terminalVerified":true,"rows":[{"taskId":"TASK","fieldId":"F","recordId":"R","state":"succeeded","terminal":true,"resultVersion":7}]}],"terminalVerified":true,"resultReadableVerified":false,"resultVersionAvailable":true}`
	var body map[string]any
	if err := json.Unmarshal([]byte(valid), &body); err != nil {
		t.Fatal(err)
	}
	if err := validateAITaskStatusReceipt(body, targets); err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []struct{ before, after string }{
		{`"recordId":"R"`, `"recordId":"other"`},
		{`"state":"succeeded"`, `"state":"writing"`},
		{`"taskId":"TASK","fieldId":"F","recordId"`, `"taskId":"old","fieldId":"F","recordId"`},
		{`"resultReadableVerified":false`, `"resultReadableVerified":true`},
		{`"rows":[{"taskId":"TASK","fieldId":"F","recordId":"R","state":"succeeded","terminal":true,"resultVersion":7}]`, `"rows":[]`},
	} {
		var invalid map[string]any
		if err := json.Unmarshal([]byte(strings.Replace(valid, replacement.before, replacement.after, 1)), &invalid); err != nil {
			t.Fatal(err)
		}
		if err := validateAITaskStatusReceipt(invalid, targets); err == nil {
			t.Fatalf("invalid AI evidence accepted: %s", replacement.after)
		}
	}
}

func TestCrossPlatformCoverageAIWaitRetainsUnknownAndOptionalProviderCapability(t *testing.T) {
	request := `[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]`
	response := `{"data":{"baseId":"B","tableId":"T","tasks":[{"taskId":"TASK","fieldId":"F","recordIds":["R"],"terminalVerified":false,"rows":[{"taskId":"TASK","fieldId":"F","recordId":"R","state":"unknown","terminal":false}]}],"terminalVerified":false,"successTerminalVerifiable":false,"resultReadableVerified":false,"resultVersionAvailable":false}}`
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: response}}}
	out, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", "--base-id", "B", "--table-id", "T", "--tasks", request, "--timeout", "0s")
	if err != nil || len(caller.calls) != 1 || caller.calls[0].tool != "get_ai_field_task_status" || !strings.Contains(out, `"outcome": "pending"`) || !strings.Contains(out, `"successTerminalVerifiable": false`) {
		t.Fatalf("unknown task capability changed result or retriggered AI: %s %v %#v", out, err, caller.calls)
	}
	targets, err := parseAITaskTargets(request)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data map[string]any }
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Data["successTerminalVerifiable"] = "false"
	if err := validateAITaskStatusReceipt(envelope.Data, targets); err == nil {
		t.Fatal("string capability flag accepted as a reviewed boolean")
	}
	delete(envelope.Data, "successTerminalVerifiable")
	if err := validateAITaskStatusReceipt(envelope.Data, targets); err != nil {
		t.Fatalf("legacy response without optional capability rejected: %v", err)
	}
}

func TestCrossPlatformCoverageAIWaitAcceptsNewHistoryVersionsWithoutClaimingReadability(t *testing.T) {
	target := `[{"taskId":"TASK","fieldId":"F","recordIds":["R"]}]`
	response := `{"data":{"baseId":"B","tableId":"T","tasks":[{"taskId":"TASK","fieldId":"F","recordIds":["R"],"terminalVerified":true,"rows":[{"taskId":"TASK","fieldId":"F","recordId":"R","state":"succeeded","terminal":true,"resultVersion":7}]}],"terminalVerified":true,"successTerminalVerifiable":true,"resultReadableVerified":false,"resultVersionAvailable":true}}`
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: response}}}
	out, err := runAITableCompositeCLI(t, caller, "+field-wait-ai", "--base-id", "B", "--table-id", "T", "--tasks", target, "--timeout", "0s")
	if err != nil || len(caller.calls) != 1 || !strings.Contains(out, `"outcome": "pending"`) || !strings.Contains(out, `"resultVersionAvailable": true`) {
		t.Fatalf("new history rejected or upgraded to readable success: %s %v", out, err)
	}
	targets, err := parseAITaskTargets(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []struct{ before, after string }{
		{`"resultVersion":7`, `"resultVersion":-1`},
		{`"resultVersion":7`, `"resultVersion":1.5`},
		{`,"resultVersion":7`, ``},
		{`"resultVersionAvailable":true`, `"resultVersionAvailable":false`},
	} {
		var envelope struct{ Data map[string]any }
		if err := json.Unmarshal([]byte(strings.Replace(response, replacement.before, replacement.after, 1)), &envelope); err != nil {
			t.Fatal(err)
		}
		if err := validateAITaskStatusReceipt(envelope.Data, targets); err == nil {
			t.Fatalf("inconsistent provider version accepted: %s", replacement.after)
		}
	}
}

func TestCrossPlatformCoverageBuildPlanRejectsWrongFieldReadbackScope(t *testing.T) {
	plan, checkpoint := setupBuildPlan(t)
	value := `{"version":1,"baseId":"b","steps":[{"id":"table","tool":"create_table","arguments":{"baseId":"b","tableName":"T","fields":[{"fieldName":"F","type":"text"}]},"verify":{"tool":"get_tables","arguments":{"baseId":"b","tableIds":[{"$ref":"table.tableId"}]},"expected":{"data":{"tables":[{"tableId":{"$ref":"table.tableId"},"tableName":"T"}]}}}}]}`
	if err := os.WriteFile(plan, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"data":{"tableId":"t"}}`},
		{text: `{"data":{"tables":[{"tableId":"t","tableName":"T"}]}}`},
		{text: `{"data":{"baseId":"b","tableId":"other","fields":[{"fieldId":"wrong-field","fieldName":"F","type":"text"}]}}`},
	}}
	if _, err := runAITableCompositeCLI(t, caller, "+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes"); err == nil {
		t.Fatal("field IDs bound from wrong table")
	}
	raw, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fieldsByName") {
		t.Fatal("wrong-table field IDs persisted as verified bindings")
	}
}

func TestCrossPlatformCoverageBuildPlanCreationRequiresReceiptIdentity(t *testing.T) {
	state := buildPlanState{BaseID: "b"}
	for _, tool := range []string{"create_chart", "create_dashboard"} {
		current := buildPlanStepState{Arguments: map[string]any{
			"baseId": "b", "dashboardId": "d", "config": map[string]any{"name": "n", "chartType": "LINE"}, "layout": map[string]any{},
		}, Resources: map[string]any{}}
		read := map[string]any{"data": map[string]any{
			"baseId": "b", "dashboardId": "d", "dashboardName": "n", "chartName": "n", "chartType": "LINE", "config": map[string]any{"chartType": "LINE"}, "layout": map[string]any{},
		}}
		if tool == "create_dashboard" {
			delete(read["data"].(map[string]any), "dashboardId")
		}
		if err := verifyBuildPlanIntent(nil, &state, buildPlanStep{Tool: tool}, &current, read); err == nil {
			t.Fatalf("%s verified without a created ID", tool)
		}
	}
}

func TestCrossPlatformCoverageBuildPlanFieldsRequireAcceptedReceiptIDs(t *testing.T) {
	state := buildPlanState{BaseID: "b"}
	current := buildPlanStepState{Arguments: map[string]any{"baseId": "b", "tableId": "t", "fields": []any{map[string]any{"fieldName": "F", "type": "text"}}}, Resources: map[string]any{}}
	read := map[string]any{"data": map[string]any{"fields": []any{map[string]any{"fieldId": "existing", "fieldName": "F", "type": "text"}}}}
	for _, receipt := range []string{
		`{"data":{"results":[{"fieldName":"F","success":false}]}}`,
		`{"data":{"results":[{"fieldName":"F","success":true,"fieldId":"new"}]}}`,
	} {
		if err := json.Unmarshal([]byte(receipt), &current.Receipt); err != nil {
			t.Fatal(err)
		}
		if err := verifyBuildPlanIntent(nil, &state, buildPlanStep{Tool: "create_fields"}, &current, read); err == nil {
			t.Fatal("old same-name field verified as newly created")
		}
	}
	current.Receipt = map[string]any{"data": map[string]any{"results": []any{map[string]any{"fieldName": "F", "success": true, "fieldId": "existing"}}}}
	if err := verifyBuildPlanIntent(nil, &state, buildPlanStep{Tool: "create_fields"}, &current, read); err != nil {
		t.Fatal(err)
	}
}
